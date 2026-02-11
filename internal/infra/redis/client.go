package redis

import (
	"context"
	"log/slog"

	"github.com/redis/go-redis/v9"

	"MKK-Luna/internal/config"
)

type Client struct {
	Redis *redis.Client
}

func New(cfg config.RedisConfig) *Client {
	cli := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
	return &Client{Redis: cli}
}

func (c *Client) Ping(ctx context.Context, logger *slog.Logger) bool {
	if c == nil || c.Redis == nil {
		return false
	}
	if err := c.Redis.Ping(ctx).Err(); err != nil {
		if logger != nil {
			logger.Warn("redis ping failed", "err", err)
		}
		return false
	}
	return true
}
