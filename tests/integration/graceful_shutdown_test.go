//go:build integration

package integration

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"log/slog"

	"MKK-Luna/internal/api"
	"MKK-Luna/internal/config"
	metricsinfra "MKK-Luna/internal/infra/metrics"
	"MKK-Luna/internal/infra/ratelimit"
	"MKK-Luna/internal/repository"
	"MKK-Luna/internal/service"
	"MKK-Luna/pkg/nethttp/runner"
)

func TestGracefulShutdownE2E(t *testing.T) {
	if !integrationEnabled() {
		t.Skip("set INTEGRATION=1 to run")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db := setupMySQLDB(t, ctx)
	defer db.Close()

	cfg := baseConfig()
	metrics := metricsinfra.New()

	authSvc, teamSvc, taskSvc, statsSvc := buildServices(t, db, cfg)

	apiPort := freePort(t)
	metricsPort := freePort(t)

	apiRouter := api.New(cfg, nilLogger(), authSvc, teamSvc, taskSvc, statsSvc, nil, ratelimit.NewMemory(1000, time.Minute), ratelimit.NewMemory(1000, time.Minute), ratelimit.NewMemory(1000, time.Minute), nil, nil, nil, metrics)
	apiServer := &http.Server{
		Addr:    ":" + strconv.Itoa(apiPort),
		Handler: apiRouter,
	}

	metricsServer := &http.Server{
		Addr:              ":" + strconv.Itoa(metricsPort),
		Handler:           promhttp.HandlerFor(metrics.Registry, promhttp.HandlerOpts{}),
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	errChan := make(chan error, 2)
	var wg sync.WaitGroup

	if err := runner.RunServer(ctx, apiServer, strconv.Itoa(apiPort), errChan, &wg, 5*time.Second); err != nil {
		t.Fatalf("run api server: %v", err)
	}
	if err := runner.RunServer(ctx, metricsServer, strconv.Itoa(metricsPort), errChan, &wg, 5*time.Second); err != nil {
		t.Fatalf("run metrics server: %v", err)
	}

	waitHTTP(t, "http://127.0.0.1:"+strconv.Itoa(apiPort)+"/health")
	waitHTTP(t, "http://127.0.0.1:"+strconv.Itoa(metricsPort)+"/metrics")

	cancel()
	wg.Wait()

	if _, err := http.Get("http://127.0.0.1:" + strconv.Itoa(apiPort) + "/health"); err == nil {
		t.Fatalf("expected api server to be down")
	}
}

func baseConfig() *config.Config {
	cfg := &config.Config{}
	cfg.JWT.Secret = "change-me-please-change-me-please-32"
	cfg.JWT.AccessTTL = 15 * time.Minute
	cfg.JWT.RefreshTTL = 30 * 24 * time.Hour
	cfg.JWT.Issuer = "task-service"
	cfg.JWT.ClockSkew = time.Minute
	cfg.Auth.BcryptCost = 12
	cfg.HTTP.Addr = ":8080"
	cfg.HTTP.ReadTimeout = 10 * time.Second
	cfg.HTTP.WriteTimeout = 10 * time.Second
	cfg.HTTP.IdleTimeout = 60 * time.Second
	cfg.HTTP.ShutdownTimeout = 5 * time.Second
	cfg.RateLimit.WindowSeconds = 60
	return cfg
}

func buildServices(t *testing.T, db *sqlx.DB, cfg *config.Config) (*service.AuthService, *service.TeamService, *service.TaskService, *service.StatsService) {
	users := repository.NewUserRepository(db)
	sessions := repository.NewSessionRepository(db)
	teams := repository.NewTeamRepository(db)
	members := repository.NewTeamMemberRepository(db)
	tasks := repository.NewTaskRepository(db)
	comments := repository.NewTaskCommentRepository(db)
	history := repository.NewTaskHistoryRepository(db)
	analytics := repository.NewAnalyticsRepository(db)

	authSvc, err := service.NewAuthService(users, sessions, *cfg, nilLogger(), nil, nil)
	if err != nil {
		t.Fatalf("auth service: %v", err)
	}
	teamSvc := service.NewTeamService(db, teams, members, users, emailOKSender{}, nil, 0, nil, nil)
	taskSvc := service.NewTaskService(db, tasks, teams, members, comments, history)
	statsSvc := service.NewStatsService(analytics, nil, nil, nilLogger())
	return authSvc, teamSvc, taskSvc, statsSvc
}

func waitHTTP(t *testing.T, url string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", url)
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func nilLogger() *slog.Logger {
	return slog.Default()
}
