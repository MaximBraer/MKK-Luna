package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/testutil"

	ideminfra "MKK-Luna/internal/infra/idempotency"
	metricsinfra "MKK-Luna/internal/infra/metrics"
)

type fakeIdemStore struct {
	get func(ctx context.Context, key string) (*ideminfra.StoredResponse, bool, error)
	set func(ctx context.Context, key string, ttl time.Duration, v ideminfra.StoredResponse) error
}

type memIdemStore struct {
	mu   sync.Mutex
	data map[string]ideminfra.StoredResponse
}

func (m *memIdemStore) Get(_ context.Context, key string) (*ideminfra.StoredResponse, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data == nil {
		return nil, false, nil
	}
	v, ok := m.data[key]
	if !ok {
		return nil, false, nil
	}
	cpy := v
	return &cpy, true, nil
}

func (m *memIdemStore) Set(_ context.Context, key string, _ time.Duration, v ideminfra.StoredResponse) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data == nil {
		m.data = map[string]ideminfra.StoredResponse{}
	}
	m.data[key] = v
	return nil
}

func (f *fakeIdemStore) Get(ctx context.Context, key string) (*ideminfra.StoredResponse, bool, error) {
	if f.get != nil {
		return f.get(ctx, key)
	}
	return nil, false, nil
}

func (f *fakeIdemStore) Set(ctx context.Context, key string, ttl time.Duration, v ideminfra.StoredResponse) error {
	if f.set != nil {
		return f.set(ctx, key, ttl, v)
	}
	return nil
}

type fakeDistLocker struct {
	acquire func(ctx context.Context, key string, ttl time.Duration) (string, bool, error)
	release func(ctx context.Context, key, token string) error
}

func (f *fakeDistLocker) Acquire(ctx context.Context, key string, ttl time.Duration) (string, bool, error) {
	if f.acquire != nil {
		return f.acquire(ctx, key, ttl)
	}
	return "token", true, nil
}

func (f *fakeDistLocker) Release(ctx context.Context, key, token string) error {
	if f.release != nil {
		return f.release(ctx, key, token)
	}
	return nil
}

func TestIdempotency_HashMismatchReturnsConflict(t *testing.T) {
	metrics := metricsinfra.New()
	mw := &IdempotencyMiddleware{
		enabled: true,
		store: &fakeIdemStore{
			get: func(context.Context, string) (*ideminfra.StoredResponse, bool, error) {
				return &ideminfra.StoredResponse{RequestHash: "other"}, true, nil
			},
		},
		locker:      &fakeDistLocker{},
		lockTTL:     time.Second,
		responseTTL: time.Minute,
		metrics:     metrics,
	}

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), ctxUserID, int64(1))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	r.Use(mw.Handler)
	r.Post("/resource", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	req := httptest.NewRequest(http.MethodPost, "/resource", http.NoBody)
	req.Header.Set("Idempotency-Key", "abc")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("status=%d want=%d", rr.Code, http.StatusConflict)
	}
	if got := testutil.ToFloat64(metrics.IdempotencyConflict); got != 1 {
		t.Fatalf("idempotency conflict metric = %v, want 1", got)
	}
}

func TestIdempotency_EmptyRoutePatternBypass(t *testing.T) {
	metrics := metricsinfra.New()
	called := 0
	mw := &IdempotencyMiddleware{
		enabled: true,
		store:   &fakeIdemStore{},
		locker:  &fakeDistLocker{},
		metrics: metrics,
	}

	next := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/raw", http.NoBody)
	req.Header.Set("Idempotency-Key", "abc")
	req = req.WithContext(context.WithValue(req.Context(), ctxUserID, int64(1)))
	req.URL.Path = ""
	rr := httptest.NewRecorder()
	next.ServeHTTP(rr, req)

	if called != 1 {
		t.Fatalf("next handler not called")
	}
	if got := testutil.ToFloat64(metrics.IdempotencyBypass.WithLabelValues("empty_route_pattern")); got != 1 {
		t.Fatalf("bypass metric=%v want=1", got)
	}
}

func TestIsCacheableStatusMatrix(t *testing.T) {
	tests := []struct {
		status int
		want   bool
	}{
		{status: http.StatusCreated, want: true},
		{status: http.StatusConflict, want: true},
		{status: http.StatusUnauthorized, want: false},
		{status: http.StatusTooManyRequests, want: false},
		{status: http.StatusInternalServerError, want: false},
	}

	for _, tt := range tests {
		if got := ideminfra.IsCacheableStatus(tt.status); got != tt.want {
			t.Fatalf("status=%d got=%v want=%v", tt.status, got, tt.want)
		}
	}
}

func TestIdempotency_LockErrorBypasses(t *testing.T) {
	mw := &IdempotencyMiddleware{
		enabled: true,
		store:   &fakeIdemStore{},
		locker: &fakeDistLocker{
			acquire: func(context.Context, string, time.Duration) (string, bool, error) {
				return "", false, errors.New("redis down")
			},
		},
		lockTTL:     time.Second,
		responseTTL: time.Minute,
	}

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), ctxUserID, int64(1))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	r.Use(mw.Handler)
	r.Post("/resource", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	req := httptest.NewRequest(http.MethodPost, "/resource", http.NoBody)
	req.Header.Set("Idempotency-Key", "abc")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d want=%d", rr.Code, http.StatusCreated)
	}
}

func TestIdempotency_ParallelSameKeySingleExecution(t *testing.T) {
	store := &memIdemStore{data: map[string]ideminfra.StoredResponse{}}
	mw := &IdempotencyMiddleware{
		enabled:     true,
		store:       store,
		locker:      &fakeDistLocker{},
		lockTTL:     time.Second,
		responseTTL: time.Minute,
	}

	var calls int32
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), ctxUserID, int64(1))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	r.Use(mw.Handler)
	r.Post("/resource", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	req1 := httptest.NewRequest(http.MethodPost, "/resource", http.NoBody)
	req1.Header.Set("Idempotency-Key", "abc")
	rr1 := httptest.NewRecorder()
	r.ServeHTTP(rr1, req1)

	req2 := httptest.NewRequest(http.MethodPost, "/resource", http.NoBody)
	req2.Header.Set("Idempotency-Key", "abc")
	rr2 := httptest.NewRecorder()
	r.ServeHTTP(rr2, req2)

	if rr1.Code != http.StatusCreated || rr2.Code != http.StatusCreated {
		t.Fatalf("unexpected statuses: first=%d second=%d", rr1.Code, rr2.Code)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("handler executed %d times, want 1", calls)
	}
}
