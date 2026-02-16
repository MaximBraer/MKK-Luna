package auth

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

type JWTBlacklist struct {
	client *redis.Client
}

func NewJWTBlacklist(client *redis.Client) *JWTBlacklist {
	return &JWTBlacklist{client: client}
}

func (b *JWTBlacklist) IsRevoked(ctx context.Context, jti string) (bool, error) {
	if b == nil || b.client == nil {
		return false, nil
	}
	if jti == "" {
		return false, errors.New("empty jti")
	}
	n, err := b.client.Exists(ctx, b.key(jti)).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (b *JWTBlacklist) Revoke(ctx context.Context, jti string, ttl time.Duration) error {
	if b == nil || b.client == nil || jti == "" {
		return nil
	}
	if ttl <= 0 {
		ttl = time.Second
	}
	return b.client.Set(ctx, b.key(jti), "1", ttl).Err()
}

func (b *JWTBlacklist) key(jti string) string {
	return "blacklist:jti:" + jti
}
