//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"MKK-Luna/internal/api"
	authinfra "MKK-Luna/internal/infra/auth"
	ideminfra "MKK-Luna/internal/infra/idempotency"
	metricsinfra "MKK-Luna/internal/infra/metrics"
	"MKK-Luna/internal/infra/ratelimit"
	redislock "MKK-Luna/internal/infra/redislock"
	"MKK-Luna/internal/repository"
	"MKK-Luna/internal/service"
)

func TestRedisDown_IdempotencyBypass(t *testing.T) {
	if !integrationEnabled() {
		t.Skip("set INTEGRATION=1 to run")
	}

	ctx := context.Background()
	db := setupMySQLDB(t, ctx)
	defer db.Close()

	cfg := baseConfig()
	cfg.Idem.Enabled = true
	cfg.Idem.LockTTL = 30 * time.Second
	cfg.Idem.ResponseTTL = 10 * time.Minute
	cfg.JWT.Blacklist.Enabled = false

	redisClient := newDownRedisClient()
	defer redisClient.Close()

	users := repository.NewUserRepository(db)
	sessions := repository.NewSessionRepository(db)
	teams := repository.NewTeamRepository(db)
	members := repository.NewTeamMemberRepository(db)
	tasks := repository.NewTaskRepository(db)
	comments := repository.NewTaskCommentRepository(db)
	history := repository.NewTaskHistoryRepository(db)
	analytics := repository.NewAnalyticsRepository(db)

	metrics := metricsinfra.New()
	locker := redislock.New(redisClient, slog.Default(), metrics)
	idemStore := ideminfra.NewStore(redisClient)

	authSvc, err := service.NewAuthService(users, sessions, *cfg, slog.Default(), metrics, nil)
	if err != nil {
		t.Fatalf("auth service: %v", err)
	}
	teamSvc := service.NewTeamService(db, teams, members, users, emailOKSender{}, locker, cfg.Idem.LockTTL, slog.Default(), metrics)
	taskSvc := service.NewTaskService(db, tasks, teams, members, comments, history)
	statsSvc := service.NewStatsService(analytics, nil, nil, slog.Default())

	router := api.New(
		cfg,
		slog.Default(),
		authSvc,
		teamSvc,
		taskSvc,
		statsSvc,
		nil,
		ratelimit.NewMemory(1000, time.Minute),
		ratelimit.NewMemory(1000, time.Minute),
		ratelimit.NewMemory(1000, time.Minute),
		nil,
		idemStore,
		locker,
		metrics,
	)
	srv := httptest.NewServer(router)
	defer srv.Close()

	token := registerAndLogin(t, srv.URL, "idem-down@test.com", "idemdown", "Password123")
	status, _ := doJSONRequestWithHeaders(t, http.MethodPost, srv.URL+"/api/v1/teams", token, map[string]string{"Idempotency-Key": "same-key"}, map[string]any{"name": "team-idem-down-1"})
	if status != http.StatusCreated {
		t.Fatalf("create team status=%d want=%d", status, http.StatusCreated)
	}
}

func TestRedisDown_LockoutBypass(t *testing.T) {
	if !integrationEnabled() {
		t.Skip("set INTEGRATION=1 to run")
	}

	ctx := context.Background()
	db := setupMySQLDB(t, ctx)
	defer db.Close()

	cfg := baseConfig()
	cfg.Auth.Lockout.MaxAttempts = 1
	cfg.Auth.Lockout.LockTTL = 10 * time.Minute
	cfg.Auth.Lockout.KeyMaxLen = 128
	cfg.JWT.Blacklist.Enabled = false
	cfg.Idem.Enabled = false

	redisClient := newDownRedisClient()
	defer redisClient.Close()

	users := repository.NewUserRepository(db)
	sessions := repository.NewSessionRepository(db)
	teams := repository.NewTeamRepository(db)
	members := repository.NewTeamMemberRepository(db)
	tasks := repository.NewTaskRepository(db)
	comments := repository.NewTaskCommentRepository(db)
	history := repository.NewTaskHistoryRepository(db)
	analytics := repository.NewAnalyticsRepository(db)

	lockout := authinfra.NewLockout(redisClient, cfg.Auth.Lockout.MaxAttempts, cfg.Auth.Lockout.LockTTL, cfg.Auth.Lockout.KeyMaxLen, slog.Default(), nil)
	authSvc, err := service.NewAuthService(users, sessions, *cfg, slog.Default(), nil, nil)
	if err != nil {
		t.Fatalf("auth service: %v", err)
	}
	teamSvc := service.NewTeamService(db, teams, members, users, emailOKSender{}, nil, 0, nil, nil)
	taskSvc := service.NewTaskService(db, tasks, teams, members, comments, history)
	statsSvc := service.NewStatsService(analytics, nil, nil, slog.Default())

	router := api.New(
		cfg,
		slog.Default(),
		authSvc,
		teamSvc,
		taskSvc,
		statsSvc,
		nil,
		ratelimit.NewMemory(1000, time.Minute),
		ratelimit.NewMemory(1000, time.Minute),
		ratelimit.NewMemory(1000, time.Minute),
		lockout,
		nil,
		nil,
		nil,
	)
	srv := httptest.NewServer(router)
	defer srv.Close()

	status, _ := doJSONRequest(t, http.MethodPost, srv.URL+"/api/v1/register", "", map[string]any{
		"email":    "lockout-down@test.com",
		"username": "lockdown",
		"password": "Password123",
	})
	if status != http.StatusCreated {
		t.Fatalf("register status=%d", status)
	}

	for i := 0; i < 2; i++ {
		status, _ = doJSONRequest(t, http.MethodPost, srv.URL+"/api/v1/login", "", map[string]any{
			"login":    "lockout-down@test.com",
			"password": "WrongPassword123",
		})
		if status != http.StatusUnauthorized {
			t.Fatalf("login bad password status=%d want=%d", status, http.StatusUnauthorized)
		}
	}
}

func TestRedisDown_BlacklistFailOpenAndClosed(t *testing.T) {
	if !integrationEnabled() {
		t.Skip("set INTEGRATION=1 to run")
	}

	cases := []struct {
		name       string
		failOpen   bool
		wantStatus int
	}{
		{name: "fail-open", failOpen: true, wantStatus: http.StatusOK},
		{name: "fail-closed", failOpen: false, wantStatus: http.StatusUnauthorized},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			db := setupMySQLDB(t, ctx)
			defer db.Close()

			cfg := baseConfig()
			cfg.JWT.Blacklist.Enabled = true
			cfg.JWT.Blacklist.FailOpen = tt.failOpen
			cfg.Idem.Enabled = false

			redisClient := newDownRedisClient()
			defer redisClient.Close()

			users := repository.NewUserRepository(db)
			sessions := repository.NewSessionRepository(db)
			teams := repository.NewTeamRepository(db)
			members := repository.NewTeamMemberRepository(db)
			tasks := repository.NewTaskRepository(db)
			comments := repository.NewTaskCommentRepository(db)
			history := repository.NewTaskHistoryRepository(db)
			analytics := repository.NewAnalyticsRepository(db)

			metrics := metricsinfra.New()
			authSvc, err := service.NewAuthService(users, sessions, *cfg, slog.Default(), metrics, authinfra.NewJWTBlacklist(redisClient))
			if err != nil {
				t.Fatalf("auth service: %v", err)
			}
			teamSvc := service.NewTeamService(db, teams, members, users, emailOKSender{}, nil, 0, nil, nil)
			taskSvc := service.NewTaskService(db, tasks, teams, members, comments, history)
			statsSvc := service.NewStatsService(analytics, nil, nil, slog.Default())

			router := api.New(
				cfg,
				slog.Default(),
				authSvc,
				teamSvc,
				taskSvc,
				statsSvc,
				nil,
				ratelimit.NewMemory(1000, time.Minute),
				ratelimit.NewMemory(1000, time.Minute),
				ratelimit.NewMemory(1000, time.Minute),
				nil,
				nil,
				nil,
				metrics,
			)
			srv := httptest.NewServer(router)
			defer srv.Close()

			username := "blacklistopen"
			email := "blacklist-open@test.com"
			if !tt.failOpen {
				username = "blacklistclosed"
				email = "blacklist-closed@test.com"
			}
			token := registerAndLogin(t, srv.URL, email, username, "Password123")
			status, _ := doJSONRequest(t, http.MethodGet, srv.URL+"/api/v1/teams", token, nil)
			if status != tt.wantStatus {
				t.Fatalf("teams status=%d want=%d", status, tt.wantStatus)
			}
		})
	}
}

func doJSONRequestWithHeaders(t *testing.T, method, url, token string, headers map[string]string, payload any) (int, []byte) {
	t.Helper()
	var bodyReader *strings.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		bodyReader = strings.NewReader(string(b))
	} else {
		bodyReader = strings.NewReader("")
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, data
}

func newDownRedisClient() *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:         "127.0.0.1:1",
		DialTimeout:  30 * time.Millisecond,
		ReadTimeout:  30 * time.Millisecond,
		WriteTimeout: 30 * time.Millisecond,
		PoolTimeout:  30 * time.Millisecond,
		MaxRetries:   0,
	})
}
