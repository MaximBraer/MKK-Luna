package middleware

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	ideminfra "MKK-Luna/internal/infra/idempotency"
	metricsinfra "MKK-Luna/internal/infra/metrics"
	redislock "MKK-Luna/internal/infra/redislock"
	"MKK-Luna/pkg/api/response"
)

type IdempotencyMiddleware struct {
	enabled     bool
	store       idemStore
	locker      distLocker
	lockTTL     time.Duration
	responseTTL time.Duration
	logger      *slog.Logger
	metrics     *metricsinfra.Metrics
}

type idemStore interface {
	Get(ctx context.Context, key string) (*ideminfra.StoredResponse, bool, error)
	Set(ctx context.Context, key string, ttl time.Duration, v ideminfra.StoredResponse) error
}

type distLocker interface {
	Acquire(ctx context.Context, key string, ttl time.Duration) (string, bool, error)
	Release(ctx context.Context, key, token string) error
}

func NewIdempotencyMiddleware(enabled bool, store *ideminfra.Store, locker *redislock.Locker, lockTTL, responseTTL time.Duration, logger *slog.Logger, metrics *metricsinfra.Metrics) *IdempotencyMiddleware {
	return &IdempotencyMiddleware{
		enabled:     enabled,
		store:       store,
		locker:      locker,
		lockTTL:     lockTTL,
		responseTTL: responseTTL,
		logger:      logger,
		metrics:     metrics,
	}
}

func (m *IdempotencyMiddleware) Handler(next http.Handler) http.Handler {
	if m == nil || !m.enabled {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isMutatingMethod(r.Method) {
			next.ServeHTTP(w, r)
			return
		}

		idemKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		if idemKey == "" {
			next.ServeHTTP(w, r)
			return
		}

		userID, ok := UserIDFromContext(r.Context())
		if !ok {
			next.ServeHTTP(w, r)
			return
		}

		routePattern := routePatternFromRequest(r)
		if routePattern == "" {
			if m.logger != nil {
				m.logger.Warn("idempotency bypass: empty route pattern")
			}
			if m.metrics != nil {
				m.metrics.IdempotencyBypass.WithLabelValues("empty_route_pattern").Inc()
			}
			next.ServeHTTP(w, r)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			response.Error(w, http.StatusBadRequest, "invalid request")
			return
		}
		_ = r.Body.Close()
		r.Body = io.NopCloser(bytes.NewReader(body))

		reqHash := ideminfra.BuildRequestHash(r.Method, routePattern, r.Header.Get("Content-Type"), r.URL.Query(), body)
		routeHash := ideminfra.BuildRouteHash(routePattern)
		responseKey := "idem:resp:" + strconv.FormatInt(userID, 10) + ":" + routeHash + ":" + idemKey
		if m.store == nil || m.locker == nil {
			if m.metrics != nil {
				m.metrics.IdempotencyBypass.WithLabelValues("unavailable").Inc()
			}
			next.ServeHTTP(w, r)
			return
		}

		cached, found, err := m.store.Get(r.Context(), responseKey)
		if err != nil {
			m.onRedisUnavailable(err, "store_get_error")
			next.ServeHTTP(w, r)
			return
		}
		if found {
			if cached.RequestHash != reqHash {
				if m.metrics != nil {
					m.metrics.IdempotencyConflict.Inc()
				}
				response.Error(w, http.StatusConflict, "idempotency key reused with different payload")
				return
			}
			replayStoredResponse(w, cached)
			if m.metrics != nil {
				m.metrics.IdempotencyHits.Inc()
			}
			return
		}

		lockKey := "lock:idem:" + strconv.FormatInt(userID, 10) + ":" + routeHash + ":" + idemKey
		lockToken, ok, err := m.locker.Acquire(r.Context(), lockKey, m.lockTTL)
		if err != nil {
			m.onRedisUnavailable(err, "lock_acquire_error")
			next.ServeHTTP(w, r)
			return
		}
		if !ok {
			response.Error(w, http.StatusConflict, "request already in progress")
			return
		}
		defer func() {
			if err := m.locker.Release(context.Background(), lockKey, lockToken); err != nil {
				if m.logger != nil {
					m.logger.Warn("idempotency lock release failed", "err", err)
				}
				if m.metrics != nil {
					m.metrics.LockReleaseErrors.Inc()
				}
			}
		}()

		rr := &idempotencyResponseRecorder{
			ResponseWriter: w,
			status:         http.StatusOK,
			headers:        make(http.Header),
		}
		next.ServeHTTP(rr, r)

		if !ideminfra.IsCacheableStatus(rr.status) {
			return
		}

		stored := ideminfra.StoredResponse{
			Status:      rr.status,
			Body:        rr.body.Bytes(),
			ContentType: rr.contentType,
			Headers:     extractReplayHeaders(rr.headers),
			RequestHash: reqHash,
			CreatedAt:   time.Now().UTC().Unix(),
		}
		if err := m.store.Set(r.Context(), responseKey, m.responseTTL, stored); err != nil {
			m.onRedisUnavailable(err, "store_set_error")
		}
	})
}

func (m *IdempotencyMiddleware) onRedisUnavailable(err error, reason string) {
	if m.logger != nil {
		m.logger.Warn("idempotency redis unavailable, bypassing", "err", err, "reason", reason)
	}
	if m.metrics != nil {
		m.metrics.RedisDegraded.WithLabelValues("idempotency").Inc()
		m.metrics.IdempotencyBypass.WithLabelValues(reason).Inc()
	}
}

func routePatternFromRequest(r *http.Request) string {
	rc := chi.RouteContext(r.Context())
	if rc == nil {
		return ""
	}
	if p := rc.RoutePattern(); p != "" {
		return p
	}
	if len(rc.RoutePatterns) > 0 {
		return strings.Join(rc.RoutePatterns, "")
	}
	if rc.RoutePath != "" {
		return rc.RoutePath
	}
	return strings.TrimSpace(r.URL.Path)
}

func isMutatingMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete:
		return true
	default:
		return false
	}
}

func replayStoredResponse(w http.ResponseWriter, sr *ideminfra.StoredResponse) {
	if sr == nil {
		response.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if sr.ContentType != "" {
		w.Header().Set("Content-Type", sr.ContentType)
	}
	for k, v := range sr.Headers {
		if v != "" {
			w.Header().Set(k, v)
		}
	}
	w.WriteHeader(sr.Status)
	if len(sr.Body) > 0 {
		_, _ = w.Write(sr.Body)
	}
}

func extractReplayHeaders(headers http.Header) map[string]string {
	out := map[string]string{}
	if headers == nil {
		return out
	}
	if v := headers.Get("Location"); v != "" {
		out["Location"] = v
	}
	return out
}

type idempotencyResponseRecorder struct {
	http.ResponseWriter
	status      int
	body        bytes.Buffer
	headers     http.Header
	wroteHeader bool
	contentType string
}

func (r *idempotencyResponseRecorder) Header() http.Header {
	return r.ResponseWriter.Header()
}

func (r *idempotencyResponseRecorder) WriteHeader(code int) {
	if r.wroteHeader {
		return
	}
	r.status = code
	r.wroteHeader = true
	r.contentType = r.ResponseWriter.Header().Get("Content-Type")
	for k, v := range r.ResponseWriter.Header() {
		r.headers[k] = append([]string(nil), v...)
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *idempotencyResponseRecorder) Write(p []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	r.body.Write(p)
	return r.ResponseWriter.Write(p)
}
