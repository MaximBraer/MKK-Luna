//go:build integration

package integration

import (
	"context"
	"os"
	"testing"
	"time"
	"strings"

	"github.com/redis/go-redis/v9"
	redisTC "github.com/testcontainers/testcontainers-go/modules/redis"

	ratelimitinfra "MKK-Luna/internal/infra/ratelimit"
)

func TestRedisRateLimit(t *testing.T) {
	if !integrationEnabled() {
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
	lim := ratelimitinfra.NewRedis(client, 2, time.Minute, ratelimitinfra.NewMemory(2, time.Minute), nil)

	key := "rl:login:ip:1.2.3.4"
	if ok, _ := lim.Allow(ctx, key); !ok {
		t.Fatalf("expected allowed")
	}
	if ok, _ := lim.Allow(ctx, key); !ok {
		t.Fatalf("expected allowed")
	}
	if ok, _ := lim.Allow(ctx, key); ok {
		t.Fatalf("expected rate limited")
	}
}

func integrationEnabled() bool {
	return testcontainersDockerOK() && os.Getenv("INTEGRATION") == "1"
}

func testcontainersDockerOK() bool {
	return true
}
