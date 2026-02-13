//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"MKK-Luna/internal/api"
	"MKK-Luna/internal/infra/ratelimit"
	"MKK-Luna/internal/repository"
	"MKK-Luna/internal/service"
)

func TestStatsUserScopedTeams(t *testing.T) {
	if !integrationEnabled() {
		t.Skip("set INTEGRATION=1 to run")
	}

	ctx := context.Background()
	db := setupMySQLDB(t, ctx)
	defer db.Close()

	cfg := baseConfig()

	users := repository.NewUserRepository(db)
	sessions := repository.NewSessionRepository(db)
	teams := repository.NewTeamRepository(db)
	members := repository.NewTeamMemberRepository(db)
	tasks := repository.NewTaskRepository(db)
	comments := repository.NewTaskCommentRepository(db)
	history := repository.NewTaskHistoryRepository(db)
	analytics := repository.NewAnalyticsRepository(db)

	authSvc, err := service.NewAuthService(users, sessions, *cfg, slog.Default(), nil)
	if err != nil {
		t.Fatalf("auth service: %v", err)
	}
	teamSvc := service.NewTeamService(db, teams, members, users, emailOKSender{})
	taskSvc := service.NewTaskService(db, tasks, teams, members, comments, history)
	statsSvc := service.NewStatsService(analytics, nil, slog.Default())

	router := api.New(cfg, slog.Default(), authSvc, teamSvc, taskSvc, statsSvc, nil,
		ratelimit.NewMemory(1000, time.Minute),
		ratelimit.NewMemory(1000, time.Minute),
		ratelimit.NewMemory(1000, time.Minute),
		nil,
	)
	srv := httptest.NewServer(router)
	defer srv.Close()

	user1Token := registerAndLogin(t, srv.URL, "stats1@test.com", "stats1", "Password123")
	user2Token := registerAndLogin(t, srv.URL, "stats2@test.com", "stats2", "Password123")

	teamID1 := createTeamHTTP(t, srv.URL, user1Token, "team-stats-1")
	teamID2 := createTeamHTTP(t, srv.URL, user2Token, "team-stats-2")

	from := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 2, 10, 0, 0, 0, 0, time.UTC)
	inRange := time.Date(2026, 2, 2, 12, 0, 0, 0, time.UTC)

	u1, _ := users.GetByEmail(ctx, "stats1@test.com")
	u2, _ := users.GetByEmail(ctx, "stats2@test.com")
	if u1 == nil || u2 == nil {
		t.Fatalf("expected users to exist")
	}
	insertTaskWithTimes(t, ctx, db, teamID1, "done-1", "done", "medium", inRange, inRange, &u1.ID)
	insertTaskWithTimes(t, ctx, db, teamID2, "done-2", "done", "medium", inRange, inRange, &u2.ID)

	status, body := doJSONRequest(t, http.MethodGet, srv.URL+"/api/v1/stats/teams/done?from="+from.Format(time.RFC3339)+"&to="+to.Format(time.RFC3339), user1Token, nil)
	if status != http.StatusOK {
		t.Fatalf("stats done status=%d body=%s", status, body)
	}
	var doneResp struct {
		Items []struct {
			TeamID    int64 `json:"team_id"`
			DoneCount int64 `json:"done_count"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &doneResp); err != nil {
		t.Fatalf("unmarshal stats done: %v", err)
	}
	if len(doneResp.Items) != 1 || doneResp.Items[0].TeamID != teamID1 || doneResp.Items[0].DoneCount != 1 {
		t.Fatalf("unexpected stats done items: %+v", doneResp.Items)
	}

	status, body = doJSONRequest(t, http.MethodGet, srv.URL+"/api/v1/stats/teams/top-creators?from="+from.Format(time.RFC3339)+"&to="+to.Format(time.RFC3339)+"&limit=3", user1Token, nil)
	if status != http.StatusOK {
		t.Fatalf("stats top creators status=%d body=%s", status, body)
	}
	var topResp struct {
		Items []struct {
			TeamID int64 `json:"team_id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &topResp); err != nil {
		t.Fatalf("unmarshal stats top: %v", err)
	}
	for _, item := range topResp.Items {
		if item.TeamID != teamID1 {
			t.Fatalf("unexpected team id in stats top creators: %d", item.TeamID)
		}
	}
}

func TestAdminIntegrityAllowlist(t *testing.T) {
	if !integrationEnabled() {
		t.Skip("set INTEGRATION=1 to run")
	}

	ctx := context.Background()
	db := setupMySQLDB(t, ctx)
	defer db.Close()

	cfg := baseConfig()

	users := repository.NewUserRepository(db)
	sessions := repository.NewSessionRepository(db)
	teams := repository.NewTeamRepository(db)
	members := repository.NewTeamMemberRepository(db)
	tasks := repository.NewTaskRepository(db)
	comments := repository.NewTaskCommentRepository(db)
	history := repository.NewTaskHistoryRepository(db)
	analytics := repository.NewAnalyticsRepository(db)

	pass := "Password123"
	hash, err := bcrypt.GenerateFromPassword([]byte(pass), 12)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	adminID, err := users.Create(ctx, "admin@test.com", "admin", string(hash))
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	_, err = users.Create(ctx, "user@test.com", "user", string(hash))
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	cfg.Admin.UserIDs = []int64{adminID}

	authSvc, err := service.NewAuthService(users, sessions, *cfg, slog.Default(), nil)
	if err != nil {
		t.Fatalf("auth service: %v", err)
	}
	teamSvc := service.NewTeamService(db, teams, members, users, emailOKSender{})
	taskSvc := service.NewTaskService(db, tasks, teams, members, comments, history)
	statsSvc := service.NewStatsService(analytics, cfg.Admin.UserIDs, slog.Default())

	router := api.New(cfg, slog.Default(), authSvc, teamSvc, taskSvc, statsSvc, nil,
		ratelimit.NewMemory(1000, time.Minute),
		ratelimit.NewMemory(1000, time.Minute),
		ratelimit.NewMemory(1000, time.Minute),
		nil,
	)
	srv := httptest.NewServer(router)
	defer srv.Close()

	adminToken := loginOnly(t, srv.URL, "admin@test.com", pass)
	userToken := loginOnly(t, srv.URL, "user@test.com", pass)

	status, _ := doJSONRequest(t, http.MethodGet, srv.URL+"/api/v1/admin/integrity/tasks", userToken, nil)
	if status != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin, got %d", status)
	}
	status, _ = doJSONRequest(t, http.MethodGet, srv.URL+"/api/v1/admin/integrity/tasks", adminToken, nil)
	if status != http.StatusOK {
		t.Fatalf("expected 200 for admin, got %d", status)
	}
}

func loginOnly(t *testing.T, baseURL, email, password string) string {
	t.Helper()
	status, body := doJSONRequest(t, http.MethodPost, baseURL+"/api/v1/login", "", map[string]any{
		"login":    email,
		"password": password,
	})
	if status != http.StatusOK {
		t.Fatalf("login status=%d body=%s", status, body)
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tok); err != nil {
		t.Fatalf("unmarshal token: %v", err)
	}
	return tok.AccessToken
}
