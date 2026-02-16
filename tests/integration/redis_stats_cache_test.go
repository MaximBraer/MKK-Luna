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
	"MKK-Luna/internal/repository"
)

func TestRedisStatsCache(t *testing.T) {
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

	cache := cacheinfra.NewStatsCache(client, time.Minute, true, nil, nil)
	from := time.Date(2026, 2, 1, 1, 2, 3, 0, time.UTC)
	to := time.Date(2026, 2, 10, 1, 2, 3, 0, time.UTC)

	done := []repository.TeamDoneStat{{TeamID: 1, TeamName: "x", MembersCount: 2, DoneCount: 3}}
	if err := cache.SetDone(ctx, 7, from, to, done); err != nil {
		t.Fatalf("set done: %v", err)
	}
	gotDone, ok, err := cache.GetDone(ctx, 7, from, to)
	if err != nil || !ok || len(gotDone) != 1 || gotDone[0].DoneCount != 3 {
		t.Fatalf("get done mismatch ok=%v err=%v got=%v", ok, err, gotDone)
	}

	top := []repository.TeamTopCreator{{TeamID: 1, UserID: 7, CreatedCount: 4, Rank: 1}}
	if err := cache.SetTop(ctx, 7, from, to, 5, top); err != nil {
		t.Fatalf("set top: %v", err)
	}
	gotTop, ok, err := cache.GetTop(ctx, 7, from, to, 5)
	if err != nil || !ok || len(gotTop) != 1 || gotTop[0].CreatedCount != 4 {
		t.Fatalf("get top mismatch ok=%v err=%v got=%v", ok, err, gotTop)
	}
}
