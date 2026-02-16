package ratelimit

import (
	"context"
	"log/slog"
	"math"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"

	metricsinfra "MKK-Luna/internal/infra/metrics"
)

type redisLimiter struct {
	client   *redis.Client
	limit    int
	window   time.Duration
	fallback *memoryLimiter
	logger   *slog.Logger
	metrics  *metricsinfra.Metrics
	errCount uint64
}

func NewRedis(client *redis.Client, limit int, window time.Duration, fallback *memoryLimiter, logger *slog.Logger, metrics *metricsinfra.Metrics) *redisLimiter {
	return &redisLimiter{client: client, limit: limit, window: window, fallback: fallback, logger: logger, metrics: metrics}
}

func (l *redisLimiter) Allow(ctx context.Context, key string) (bool, time.Duration) {
	if key == "" || l.limit <= 0 {
		return true, 0
	}
	if l.client == nil {
		return l.fallback.Allow(ctx, key)
	}

	count, err := l.client.Incr(ctx, key).Result()
	if err != nil {
		l.onRedisError(err)
		return l.fallback.Allow(ctx, key)
	}

	if count == 1 {
		if err := l.client.Expire(ctx, key, l.window).Err(); err != nil {
			l.onRedisError(err)
		}
	}

	if int(count) > l.limit {
		ttl, err := l.client.TTL(ctx, key).Result()
		if err != nil {
			l.onRedisError(err)
			return false, remainingDuration(l.window, time.Now().UTC())
		}
		if ttl <= 0 {
			if l.logger != nil {
				l.logger.Warn("redis limiter ttl missing", "key", key, "ttl", ttl)
			}
			return false, remainingDuration(l.window, time.Now().UTC())
		}
		return false, ceilDuration(ttl)
	}

	return true, 0
}

func (l *redisLimiter) onRedisError(err error) {
	atomic.AddUint64(&l.errCount, 1)
	if l.logger != nil {
		l.logger.Warn("redis limiter error", "err", err)
	}
	if l.metrics != nil {
		l.metrics.RedisDegraded.WithLabelValues("ratelimit").Inc()
	}
}

func ceilDuration(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}
	secs := math.Ceil(d.Seconds())
	return time.Duration(secs) * time.Second
}

func remainingDuration(window time.Duration, now time.Time) time.Duration {
	ws := int64(window.Seconds())
	if ws <= 0 {
		return time.Second
	}
	rem := ws - (now.Unix() % ws)
	if rem <= 0 {
		rem = 1
	}
	return time.Duration(rem) * time.Second
}
