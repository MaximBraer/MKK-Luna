//go:build integration

package integration

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	redisTC "github.com/testcontainers/testcontainers-go/modules/redis"

	cacheinfra "MKK-Luna/internal/infra/cache"
)

func TestRedisTaskCache(t *testing.T) {
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("set INTEGRATION=1 to run")
	}

	ctx := context.Background()
	redisC, err := redisTC.RunContainer(ctx)
	if err != nil {
		t.Fatalf("redis container: %v", err)
	}
	defer redisC.Terminate(ctx)

	endpoint, err := redisC.Endpoint(ctx, "tcp")
	if err != nil {
		t.Fatalf("redis endpoint: %v", err)
	}
	endpoint = strings.TrimPrefix(endpoint, "tcp://")

	client := redis.NewClient(&redis.Options{Addr: endpoint})
	cache := cacheinfra.NewTaskCache(client, 5*time.Minute, true, nil, nil)

	if err := client.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("flushdb: %v", err)
	}

	filters := map[string]string{"status": "todo", "limit": "10"}
	payload := []byte(`{"items":[]}`)

	ok := false
	if _, ok, err = cache.GetList(ctx, 1, filters); err != nil || ok {
		t.Fatalf("expected miss")
	}

	if err := cache.SetList(ctx, 1, filters, payload); err != nil {
		t.Fatalf("set: %v", err)
	}

	if data, ok, err := cache.GetList(ctx, 1, filters); err != nil || !ok || string(data) != string(payload) {
		t.Fatalf("expected hit")
	}

	if err := cache.InvalidateTeam(ctx, 1); err != nil {
		t.Fatalf("invalidate: %v", err)
	}

	if _, ok, _ = cache.GetList(ctx, 1, filters); ok {
		t.Fatalf("expected miss after invalidate")
	}
}
