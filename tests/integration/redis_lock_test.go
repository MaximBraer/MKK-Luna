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

	redislock "MKK-Luna/internal/infra/redislock"
)

func TestRedisLockOwnerRelease(t *testing.T) {
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
	defer client.Close()

	locker := redislock.New(client, nil, nil)
	key := "lock:test:ownership"

	token1, ok, err := locker.Acquire(ctx, key, 100*time.Millisecond)
	if err != nil || !ok {
		t.Fatalf("acquire first ok=%v err=%v", ok, err)
	}

	time.Sleep(150 * time.Millisecond)

	token2, ok, err := locker.Acquire(ctx, key, time.Second)
	if err != nil || !ok {
		t.Fatalf("acquire second ok=%v err=%v", ok, err)
	}

	if err := locker.Release(ctx, key, token1); err != nil {
		t.Fatalf("release stale token: %v", err)
	}
	val, err := client.Get(ctx, key).Result()
	if err != nil {
		t.Fatalf("read key: %v", err)
	}
	if val != token2 {
		t.Fatalf("stale release deleted/replaced new lock value")
	}
}
