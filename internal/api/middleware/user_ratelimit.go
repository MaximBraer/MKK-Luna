package middleware

import (
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"time"

	"MKK-Luna/internal/domain/ratelimit"
	"MKK-Luna/pkg/api/response"
)

func UserRateLimit(limiter ratelimit.Limiter, windowSeconds int, logger *slog.Logger) func(http.Handler) http.Handler {
	if limiter == nil || windowSeconds <= 0 {
		return func(next http.Handler) http.Handler { return next }
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/health" || r.URL.Path == "/ready" {
				next.ServeHTTP(w, r)
				return
			}

			userID, ok := UserIDFromContext(r.Context())
			if !ok {
				if logger != nil {
					logger.Error("internal auth context missing")
				}
				response.Error(w, http.StatusInternalServerError, "internal error")
				return
			}

			key := userRateLimitKey(userID, windowSeconds, time.Now().UTC())
			allowed, retryAfter := limiter.Allow(r.Context(), key)
			if !allowed {
				setRetryAfterHeader(w, retryAfter)
				response.Error(w, http.StatusTooManyRequests, "too many requests")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func userRateLimitKey(userID int64, windowSeconds int, now time.Time) string {
	epoch := now.Unix() / int64(windowSeconds)
	return "rl:user:" + itoa(userID) + ":" + itoa(epoch)
}

func setRetryAfterHeader(w http.ResponseWriter, d time.Duration) {
	if d <= 0 {
		return
	}
	secs := int(math.Ceil(d.Seconds()))
	if secs < 1 {
		secs = 1
	}
	w.Header().Set("Retry-After", itoa(int64(secs)))
}

func itoa(v int64) string {
	return strconv.FormatInt(v, 10)
}
