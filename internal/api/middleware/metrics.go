package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	metricsinfra "MKK-Luna/internal/infra/metrics"
)

func Metrics(m *metricsinfra.Metrics) func(http.Handler) http.Handler {
	if m == nil {
		return func(next http.Handler) http.Handler { return next }
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			m.HTTPInFlight.Inc()
			defer m.HTTPInFlight.Dec()

			rr := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rr, r)

			path := chi.RouteContext(r.Context()).RoutePattern()
			if path == "" {
				path = "unmatched"
			}

			code := strconv.Itoa(rr.status)
			m.HTTPRequests.WithLabelValues(r.Method, path, code).Inc()
			m.HTTPDuration.WithLabelValues(r.Method, path).Observe(time.Since(start).Seconds())
			if rr.status >= 500 {
				m.HTTPErrors.WithLabelValues(r.Method, path, code).Inc()
			}
		})
	}
}

type responseRecorder struct {
	http.ResponseWriter
	status int
}

func (r *responseRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}
