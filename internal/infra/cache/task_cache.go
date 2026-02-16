package cache

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	metricsinfra "MKK-Luna/internal/infra/metrics"
)

type TaskCache struct {
	client  *redis.Client
	ttl     time.Duration
	enabled bool
	logger  *slog.Logger
	metrics *metricsinfra.Metrics
}

func NewTaskCache(client *redis.Client, ttl time.Duration, enabled bool, logger *slog.Logger, metrics *metricsinfra.Metrics) *TaskCache {
	return &TaskCache{client: client, ttl: ttl, enabled: enabled, logger: logger, metrics: metrics}
}

func (c *TaskCache) GetList(ctx context.Context, teamID int64, filters map[string]string) ([]byte, bool, error) {
	if !c.enabled || c.client == nil {
		return nil, false, nil
	}
	ver, err := c.getVersion(ctx, teamID)
	if err != nil {
		return nil, false, err
	}
	key := cacheKey(teamID, ver, filters)
	val, err := c.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		c.onRedisError(err)
		return nil, false, err
	}
	return val, true, nil
}

func (c *TaskCache) SetList(ctx context.Context, teamID int64, filters map[string]string, data []byte) error {
	if !c.enabled || c.client == nil {
		return nil
	}
	ver, err := c.getVersion(ctx, teamID)
	if err != nil {
		c.onRedisError(err)
		return err
	}
	key := cacheKey(teamID, ver, filters)
	if err := c.client.Set(ctx, key, data, c.ttl).Err(); err != nil {
		c.onRedisError(err)
		return err
	}
	return nil
}

func (c *TaskCache) InvalidateTeam(ctx context.Context, teamID int64) error {
	if !c.enabled || c.client == nil {
		return nil
	}
	_, err := c.client.Incr(ctx, versionKey(teamID)).Result()
	if err != nil {
		c.onRedisError(err)
	}
	return err
}

func (c *TaskCache) getVersion(ctx context.Context, teamID int64) (int64, error) {
	val, err := c.client.Get(ctx, versionKey(teamID)).Result()
	if err == redis.Nil {
		_, _ = c.client.SetNX(ctx, versionKey(teamID), "1", 0).Result()
		return 1, nil
	}
	if err != nil {
		c.onRedisError(err)
		return 0, err
	}
	ver, err := parseInt64(val)
	if err != nil {
		if c.logger != nil {
			c.logger.Debug("cache version parse failed, fallback to 1", "err", err, "team_id", teamID)
		}
		return 1, nil
	}
	return ver, nil
}

func (c *TaskCache) onRedisError(err error) {
	if c.logger != nil {
		c.logger.Warn("cache redis error", "err", err)
	}
	if c.metrics != nil {
		c.metrics.RedisDegraded.WithLabelValues("cache").Inc()
	}
}

func versionKey(teamID int64) string {
	return "tasks:team:" + itoa(teamID) + ":ver"
}

func cacheKey(teamID, ver int64, filters map[string]string) string {
	return "tasks:team:" + itoa(teamID) + ":v:" + itoa(ver) + ":" + filtersHash(filters)
}

func filtersHash(filters map[string]string) string {
	qs := canonicalQueryString(filters)
	sum := sha1.Sum([]byte(qs))
	return hex.EncodeToString(sum[:])
}

func canonicalQueryString(filters map[string]string) string {
	if len(filters) == 0 {
		return ""
	}
	keys := make([]string, 0, len(filters))
	for k := range filters {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+filters[k])
	}
	return strings.Join(parts, "&")
}

func parseInt64(s string) (int64, error) {
	var n int64
	_, err := fmt.Sscan(s, &n)
	return n, err
}

func itoa(v int64) string {
	return strconv.FormatInt(v, 10)
}
