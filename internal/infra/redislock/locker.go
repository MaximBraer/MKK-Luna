package redislock

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	metricsinfra "MKK-Luna/internal/infra/metrics"
)

var releaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0
`)

type Locker struct {
	client  *redis.Client
	logger  *slog.Logger
	metrics *metricsinfra.Metrics
}

func New(client *redis.Client, logger *slog.Logger, metrics *metricsinfra.Metrics) *Locker {
	return &Locker{client: client, logger: logger, metrics: metrics}
}

func (l *Locker) Acquire(ctx context.Context, key string, ttl time.Duration) (string, bool, error) {
	if l == nil || l.client == nil {
		return "", false, errors.New("redis lock unavailable")
	}
	if key == "" {
		return "", false, errors.New("empty lock key")
	}
	if ttl <= 0 {
		ttl = 15 * time.Second
	}

	token := uuid.NewString()
	ok, err := l.client.SetNX(ctx, key, token, ttl).Result()
	if err != nil {
		l.onRedisError(err)
		return "", false, err
	}
	if !ok {
		return "", false, nil
	}
	return token, true, nil
}

func (l *Locker) Release(ctx context.Context, key, token string) error {
	if l == nil || l.client == nil || key == "" || token == "" {
		return nil
	}
	if _, err := releaseScript.Run(ctx, l.client, []string{key}, token).Result(); err != nil {
		l.onRedisError(err)
		return err
	}
	return nil
}

func (l *Locker) onRedisError(err error) {
	if l.logger != nil {
		l.logger.Warn("redis lock error", "err", err)
	}
	if l.metrics != nil {
		l.metrics.RedisDegraded.WithLabelValues("lock").Inc()
	}
}
