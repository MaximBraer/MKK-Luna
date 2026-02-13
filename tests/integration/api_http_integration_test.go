//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"MKK-Luna/internal/api"
	"MKK-Luna/internal/config"
	"MKK-Luna/internal/infra/ratelimit"
	"MKK-Luna/internal/repository"
	"MKK-Luna/internal/service"
)

func TestAPIErrorBranches_400_403_404_409(t *testing.T) {
	if !integrationEnabled() {
		t.Skip("set INTEGRATION=1 to run")
	}

	ctx := context.Background()
	db := setupMySQLDB(t, ctx)
	defer db.Close()

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

	users := repository.NewUserRepository(db)
	sessions := repository.NewSessionRepository(db)
	teams := repository.NewTeamRepository(db)
	members := repository.NewTeamMemberRepository(db)
	tasks := repository.NewTaskRepository(db)
	comments := repository.NewTaskCommentRepository(db)
	history := repository.NewTaskHistoryRepository(db)
	analytics := repository.NewAnalyticsRepository(db)

	authSvc, err := service.NewAuthService(users, sessions, *cfg, slog.Default(), nil, nil)
	if err != nil {
		t.Fatalf("auth service: %v", err)
	}
	teamSvc := service.NewTeamService(db, teams, members, users, emailOKSender{}, nil, 0, nil, nil)
	taskSvc := service.NewTaskService(db, tasks, teams, members, comments, history)
	statsSvc := service.NewStatsService(analytics, nil, nil, slog.Default())

	router := api.New(cfg, slog.Default(), authSvc, teamSvc, taskSvc, statsSvc, nil, ratelimit.NewMemory(1000, time.Minute), ratelimit.NewMemory(1000, time.Minute), ratelimit.NewMemory(1000, time.Minute), nil, nil, nil, nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	ownerToken := registerAndLogin(t, srv.URL, "owner-http@test.com", "ownerhttp", "Password123")
	memberToken := registerAndLogin(t, srv.URL, "member-http@test.com", "memberhttp", "Password123")
	outsiderToken := registerAndLogin(t, srv.URL, "outsider-http@test.com", "outsiderhttp", "Password123")

	teamID := createTeamHTTP(t, srv.URL, ownerToken, "http-team")
	inviteHTTP(t, srv.URL, ownerToken, teamID, "member-http@test.com", "member", http.StatusOK)
	inviteHTTP(t, srv.URL, ownerToken, teamID, "member-http@test.com", "member", http.StatusConflict)
	inviteHTTP(t, srv.URL, outsiderToken, teamID, "member-http@test.com", "member", http.StatusForbidden)
	inviteHTTP(t, srv.URL, ownerToken, 999999, "member-http@test.com", "member", http.StatusNotFound)

	taskID := createTaskHTTP(t, srv.URL, ownerToken, teamID, "http-task")

	// 400: missing team_id
	status, _ := doJSONRequest(t, http.MethodGet, srv.URL+"/api/v1/tasks", ownerToken, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 list without team_id, got %d", status)
	}

	// 403: member patch forbidden field
	status, _ = doJSONRequest(t, http.MethodPatch, srv.URL+"/api/v1/tasks/"+itoa(taskID), memberToken, map[string]any{"title": "hack"})
	if status != http.StatusForbidden {
		t.Fatalf("expected 403 member patch forbidden field, got %d", status)
	}

	// 404: task not found
	status, _ = doJSONRequest(t, http.MethodGet, srv.URL+"/api/v1/tasks/999999", ownerToken, nil)
	if status != http.StatusNotFound {
		t.Fatalf("expected 404 task not found, got %d", status)
	}

	// 403: outsider comment update
	commentID := createCommentHTTP(t, srv.URL, memberToken, taskID, "hello")
	status, _ = doJSONRequest(t, http.MethodPatch, srv.URL+"/api/v1/comments/"+itoa(commentID), outsiderToken, map[string]any{"body": "hack"})
	if status != http.StatusForbidden {
		t.Fatalf("expected 403 outsider comment patch, got %d", status)
	}

	// create one history entry
	status, _ = doJSONRequest(t, http.MethodPatch, srv.URL+"/api/v1/tasks/"+itoa(taskID), ownerToken, map[string]any{"status": "done"})
	if status != http.StatusOK {
		t.Fatalf("expected 200 on patch for history, got %d", status)
	}

	// 400: invalid limit
	status, _ = doJSONRequest(t, http.MethodGet, srv.URL+"/api/v1/tasks/"+itoa(taskID)+"/history?limit=101", ownerToken, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 invalid history limit, got %d", status)
	}

	// 404: history task not found
	status, _ = doJSONRequest(t, http.MethodGet, srv.URL+"/api/v1/tasks/999999/history", ownerToken, nil)
	if status != http.StatusNotFound {
		t.Fatalf("expected 404 history task not found, got %d", status)
	}

	// 403: outsider history access
	status, _ = doJSONRequest(t, http.MethodGet, srv.URL+"/api/v1/tasks/"+itoa(taskID)+"/history", outsiderToken, nil)
	if status != http.StatusForbidden {
		t.Fatalf("expected 403 history for outsider, got %d", status)
	}

	// 200: history read
	status, body := doJSONRequest(t, http.MethodGet, srv.URL+"/api/v1/tasks/"+itoa(taskID)+"/history?limit=20&offset=0", ownerToken, nil)
	if status != http.StatusOK {
		t.Fatalf("expected 200 history read, got %d body=%s", status, body)
	}
}

func TestInviteEmailFailureReturns503(t *testing.T) {
	if !integrationEnabled() {
		t.Skip("set INTEGRATION=1 to run")
	}

	ctx := context.Background()
	db := setupMySQLDB(t, ctx)
	defer db.Close()

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

	users := repository.NewUserRepository(db)
	sessions := repository.NewSessionRepository(db)
	teams := repository.NewTeamRepository(db)
	members := repository.NewTeamMemberRepository(db)
	tasks := repository.NewTaskRepository(db)
	comments := repository.NewTaskCommentRepository(db)
	history := repository.NewTaskHistoryRepository(db)
	analytics := repository.NewAnalyticsRepository(db)

	authSvc, err := service.NewAuthService(users, sessions, *cfg, slog.Default(), nil, nil)
	if err != nil {
		t.Fatalf("auth service: %v", err)
	}
	teamSvc := service.NewTeamService(db, teams, members, users, emailFailSender{}, nil, 0, nil, nil)
	taskSvc := service.NewTaskService(db, tasks, teams, members, comments, history)
	statsSvc := service.NewStatsService(analytics, nil, nil, slog.Default())

	router := api.New(cfg, slog.Default(), authSvc, teamSvc, taskSvc, statsSvc, nil, ratelimit.NewMemory(1000, time.Minute), ratelimit.NewMemory(1000, time.Minute), ratelimit.NewMemory(1000, time.Minute), nil, nil, nil, nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	ownerToken := registerAndLogin(t, srv.URL, "owner-fail@test.com", "ownerfail", "Password123")
	_ = registerAndLogin(t, srv.URL, "member-fail@test.com", "memberfail", "Password123")
	teamID := createTeamHTTP(t, srv.URL, ownerToken, "http-team-fail")

	status, _ := doJSONRequest(t, http.MethodPost, srv.URL+"/api/v1/teams/"+itoa(teamID)+"/invite", ownerToken, map[string]any{
		"email": "member-fail@test.com",
		"role":  "member",
	})
	if status != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 on email failure, got %d", status)
	}
}

func TestGlobalUserRateLimit(t *testing.T) {
	if !integrationEnabled() {
		t.Skip("set INTEGRATION=1 to run")
	}

	ctx := context.Background()
	db := setupMySQLDB(t, ctx)
	defer db.Close()

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
	cfg.RateLimit.WindowSeconds = 2

	users := repository.NewUserRepository(db)
	sessions := repository.NewSessionRepository(db)
	teams := repository.NewTeamRepository(db)
	members := repository.NewTeamMemberRepository(db)
	tasks := repository.NewTaskRepository(db)
	comments := repository.NewTaskCommentRepository(db)
	history := repository.NewTaskHistoryRepository(db)
	analytics := repository.NewAnalyticsRepository(db)

	authSvc, err := service.NewAuthService(users, sessions, *cfg, slog.Default(), nil, nil)
	if err != nil {
		t.Fatalf("auth service: %v", err)
	}
	teamSvc := service.NewTeamService(db, teams, members, users, emailOKSender{}, nil, 0, nil, nil)
	taskSvc := service.NewTaskService(db, tasks, teams, members, comments, history)
	statsSvc := service.NewStatsService(analytics, nil, nil, slog.Default())

	userLimiter := ratelimit.NewMemory(5, 2*time.Second)
	router := api.New(cfg, slog.Default(), authSvc, teamSvc, taskSvc, statsSvc, nil, ratelimit.NewMemory(1000, time.Minute), ratelimit.NewMemory(1000, time.Minute), userLimiter, nil, nil, nil, nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	token := registerAndLogin(t, srv.URL, "rl-user@test.com", "rluser", "Password123")

	for i := 0; i < 5; i++ {
		status, _ := doJSONRequest(t, http.MethodGet, srv.URL+"/api/v1/teams", token, nil)
		if status != http.StatusOK {
			t.Fatalf("expected 200 before limit, got %d", status)
		}
	}
	status, _ := doJSONRequest(t, http.MethodGet, srv.URL+"/api/v1/teams", token, nil)
	if status != http.StatusTooManyRequests {
		t.Fatalf("expected 429 on limit, got %d", status)
	}
}

func registerAndLogin(t *testing.T, baseURL, email, username, password string) string {
	t.Helper()
	status, _ := doJSONRequest(t, http.MethodPost, baseURL+"/api/v1/register", "", map[string]any{
		"email":    email,
		"username": username,
		"password": password,
	})
	if status != http.StatusCreated {
		t.Fatalf("register status=%d", status)
	}

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

func createTeamHTTP(t *testing.T, baseURL, token, name string) int64 {
	t.Helper()
	status, body := doJSONRequest(t, http.MethodPost, baseURL+"/api/v1/teams", token, map[string]any{"name": name})
	if status != http.StatusCreated {
		t.Fatalf("create team status=%d body=%s", status, body)
	}
	var resp struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal(body, &resp)
	return resp.ID
}

func inviteHTTP(t *testing.T, baseURL, token string, teamID int64, email, role string, wantStatus int) {
	t.Helper()
	status, _ := doJSONRequest(t, http.MethodPost, baseURL+"/api/v1/teams/"+itoa(teamID)+"/invite", token, map[string]any{
		"email": email,
		"role":  role,
	})
	if status != wantStatus {
		t.Fatalf("invite status=%d want=%d", status, wantStatus)
	}
}

func createTaskHTTP(t *testing.T, baseURL, token string, teamID int64, title string) int64 {
	t.Helper()
	status, body := doJSONRequest(t, http.MethodPost, baseURL+"/api/v1/tasks", token, map[string]any{
		"team_id": teamID,
		"title":   title,
	})
	if status != http.StatusCreated {
		t.Fatalf("create task status=%d body=%s", status, body)
	}
	var resp struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal(body, &resp)
	return resp.ID
}

func createCommentHTTP(t *testing.T, baseURL, token string, taskID int64, body string) int64 {
	t.Helper()
	status, respBody := doJSONRequest(t, http.MethodPost, baseURL+"/api/v1/tasks/"+itoa(taskID)+"/comments", token, map[string]any{"body": body})
	if status != http.StatusCreated {
		t.Fatalf("create comment status=%d body=%s", status, respBody)
	}
	var resp struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal(respBody, &resp)
	return resp.ID
}

func doJSONRequest(t *testing.T, method, url, token string, payload any) (int, []byte) {
	t.Helper()
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("http request: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, respBody
}

func itoa(v int64) string {
	return strconv.FormatInt(v, 10)
}
