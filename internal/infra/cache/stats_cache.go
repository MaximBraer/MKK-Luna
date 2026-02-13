package cache

import (
	"context"
	"encoding/json"
	"log/slog"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	metricsinfra "MKK-Luna/internal/infra/metrics"
	"MKK-Luna/internal/repository"
)

type StatsCache struct {
	client  *redis.Client
	ttl     time.Duration
	enabled bool
	logger  *slog.Logger
	metrics *metricsinfra.Metrics
}

func NewStatsCache(client *redis.Client, ttl time.Duration, enabled bool, logger *slog.Logger, metrics *metricsinfra.Metrics) *StatsCache {
	return &StatsCache{client: client, ttl: ttl, enabled: enabled, logger: logger, metrics: metrics}
}

func (c *StatsCache) GetDone(ctx context.Context, userID int64, from, to time.Time) ([]repository.TeamDoneStat, bool, error) {
	if !c.enabled || c.client == nil {
		return nil, false, nil
	}
	raw, err := c.client.Get(ctx, doneKey(userID, from, to)).Bytes()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		c.onRedisError(err)
		return nil, false, err
	}
	var items []repository.TeamDoneStat
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, false, err
	}
	return items, true, nil
}

func (c *StatsCache) SetDone(ctx context.Context, userID int64, from, to time.Time, items []repository.TeamDoneStat) error {
	if !c.enabled || c.client == nil {
		return nil
	}
	raw, err := json.Marshal(items)
	if err != nil {
		return err
	}
	if err := c.client.Set(ctx, doneKey(userID, from, to), raw, c.ttl).Err(); err != nil {
		c.onRedisError(err)
		return err
	}
	return nil
}

func (c *StatsCache) GetTop(ctx context.Context, userID int64, from, to time.Time, limit int) ([]repository.TeamTopCreator, bool, error) {
	if !c.enabled || c.client == nil {
		return nil, false, nil
	}
	raw, err := c.client.Get(ctx, topKey(userID, from, to, limit)).Bytes()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		c.onRedisError(err)
		return nil, false, err
	}
	var items []repository.TeamTopCreator
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, false, err
	}
	return items, true, nil
}

func (c *StatsCache) SetTop(ctx context.Context, userID int64, from, to time.Time, limit int, items []repository.TeamTopCreator) error {
	if !c.enabled || c.client == nil {
		return nil
	}
	raw, err := json.Marshal(items)
	if err != nil {
		return err
	}
	if err := c.client.Set(ctx, topKey(userID, from, to, limit), raw, c.ttl).Err(); err != nil {
		c.onRedisError(err)
		return err
	}
	return nil
}

func doneKey(userID int64, from, to time.Time) string {
	return "stats:done:u:" + strconv.FormatInt(userID, 10) + ":f:" + dayKey(from) + ":t:" + dayKey(to)
}

func topKey(userID int64, from, to time.Time, limit int) string {
	return "stats:top:u:" + strconv.FormatInt(userID, 10) + ":f:" + dayKey(from) + ":t:" + dayKey(to) + ":l:" + strconv.Itoa(limit)
}

func dayKey(tm time.Time) string {
	return tm.UTC().Format("20060102")
}

func (c *StatsCache) onRedisError(err error) {
	if c.logger != nil {
		c.logger.Warn("stats cache redis error", "err", err)
	}
	if c.metrics != nil {
		c.metrics.RedisDegraded.WithLabelValues("stats_cache").Inc()
	}
}
