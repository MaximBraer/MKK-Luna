package auth

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	metricsinfra "MKK-Luna/internal/infra/metrics"
)

type Lockout struct {
	client      *redis.Client
	maxAttempts int
	lockTTL     time.Duration
	keyMaxLen   int
	logger      *slog.Logger
	metrics     *metricsinfra.Metrics
}

func NewLockout(client *redis.Client, maxAttempts int, lockTTL time.Duration, keyMaxLen int, logger *slog.Logger, metrics *metricsinfra.Metrics) *Lockout {
	return &Lockout{
		client:      client,
		maxAttempts: maxAttempts,
		lockTTL:     lockTTL,
		keyMaxLen:   keyMaxLen,
		logger:      logger,
		metrics:     metrics,
	}
}

func (l *Lockout) Normalize(login string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(login))
	if s == "" {
		return "", errors.New("empty login")
	}
	max := l.keyMaxLen
	if max <= 0 {
		max = 128
	}
	if len(s) > max {
		return "", errors.New("login too long")
	}
	return s, nil
}

func (l *Lockout) IsLocked(ctx context.Context, normalized string) (bool, time.Duration, error) {
	if l == nil || l.client == nil || l.maxAttempts <= 0 || l.lockTTL <= 0 {
		return false, 0, nil
	}
	ttl, err := l.client.TTL(ctx, l.lockKey(normalized)).Result()
	if err != nil {
		l.onRedisError(err)
		return false, 0, err
	}
	if ttl > 0 {
		return true, ttl, nil
	}
	return false, 0, nil
}

func (l *Lockout) OnFailure(ctx context.Context, normalized string) (bool, time.Duration, error) {
	if l == nil || l.client == nil || l.maxAttempts <= 0 || l.lockTTL <= 0 {
		return false, 0, nil
	}

	failKey := l.failKey(normalized)
	count, err := l.client.Incr(ctx, failKey).Result()
	if err != nil {
		l.onRedisError(err)
		return false, 0, err
	}
	if count == 1 {
		if err := l.client.Expire(ctx, failKey, l.lockTTL).Err(); err != nil {
			l.onRedisError(err)
		}
	}
	if int(count) < l.maxAttempts {
		return false, 0, nil
	}
	if err := l.client.Set(ctx, l.lockKey(normalized), "1", l.lockTTL).Err(); err != nil {
		l.onRedisError(err)
		return false, 0, err
	}
	if l.metrics != nil {
		l.metrics.LoginLockouts.Inc()
	}
	return true, l.lockTTL, nil
}

func (l *Lockout) OnSuccess(ctx context.Context, normalized string) error {
	if l == nil || l.client == nil {
		return nil
	}
	_, err := l.client.Del(ctx, l.failKey(normalized), l.lockKey(normalized)).Result()
	if err != nil {
		l.onRedisError(err)
		return err
	}
	return nil
}

func (l *Lockout) failKey(login string) string {
	return "auth:fail:" + login
}

func (l *Lockout) lockKey(login string) string {
	return "auth:lock:" + login
}

func (l *Lockout) onRedisError(err error) {
	if l.logger != nil {
		l.logger.Warn("login lockout redis error", "err", err)
	}
	if l.metrics != nil {
		l.metrics.RedisDegraded.WithLabelValues("lockout").Inc()
	}
}

func FormatRetryAfter(d time.Duration) string {
	secs := int64(d.Seconds())
	if d > 0 && secs == 0 {
		secs = 1
	}
	if secs < 1 {
		secs = 1
	}
	return strconv.FormatInt(secs, 10)
}
