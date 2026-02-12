package ratelimit

import (
	"context"
	"log/slog"
	"math"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

type redisLimiter struct {
	client   *redis.Client
	limit    int
	window   time.Duration
	fallback *memoryLimiter
	logger   *slog.Logger
	errCount uint64
}

func NewRedis(client *redis.Client, limit int, window time.Duration, fallback *memoryLimiter, logger *slog.Logger) *redisLimiter {
	return &redisLimiter{client: client, limit: limit, window: window, fallback: fallback, logger: logger}
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
			return false, l.window
		}
		if ttl <= 0 {
			return false, l.window
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
}

func ceilDuration(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}
	secs := math.Ceil(d.Seconds())
	return time.Duration(secs) * time.Second
}
