//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestE2ECriticalFlowsAndMetrics(t *testing.T) {
	if os.Getenv("E2E") != "1" {
		t.Skip("set E2E=1 to run")
	}

	baseURL := getenv("E2E_BASE_URL", "http://localhost:8080")
	promURL := strings.TrimSpace(os.Getenv("E2E_PROM_URL"))
	startService(t, "redis")
	startService(t, "email-mock")
	waitAPIReady(t, baseURL)
	defer startService(t, "redis")
	defer startService(t, "email-mock")

	sfx := strconv.FormatInt(time.Now().UnixNano(), 10)
	ownerEmail := "e2e-owner-" + sfx + "@test.com"
	memberEmail := "e2e-member-" + sfx + "@test.com"
	failEmail := "e2e-fail-" + sfx + "@test.com"
	redisEmail := "e2e-redis-" + sfx + "@test.com"

	ownerToken := registerAndLogin(t, baseURL, ownerEmail, "e2eowner"+sfx, "Password123")
	memberToken := registerAndLogin(t, baseURL, memberEmail, "e2emember"+sfx, "Password123")
	_ = memberToken

	teamID := createTeam(t, baseURL, ownerToken, "e2e-team-"+sfx)
	invite(t, baseURL, ownerToken, teamID, memberEmail, "member", http.StatusOK, http.StatusConflict)
	invite(t, baseURL, ownerToken, teamID, memberEmail, "member", http.StatusConflict)

	_ = registerAndLogin(t, baseURL, failEmail, "e2efail"+sfx, "Password123")

	taskID := createTask(t, baseURL, ownerToken, teamID, "e2e-task")
	updateTask(t, baseURL, ownerToken, taskID, map[string]any{"status": "done"}, http.StatusOK)
	createComment(t, baseURL, ownerToken, taskID, "hello", http.StatusCreated)
	getHistory(t, baseURL, ownerToken, taskID, http.StatusOK)

	// auth failures for metrics
	loginFail(t, baseURL, ownerEmail, "wrong")

	// email failure + circuit open metrics
	stopService(t, "email-mock")
	for i := 0; i < 6; i++ {
		invite(t, baseURL, ownerToken, teamID, failEmail, "member", http.StatusServiceUnavailable)
	}
	startService(t, "email-mock")

	// redis degraded metric
	_ = registerAndLogin(t, baseURL, redisEmail, "e2eredis"+sfx, "Password123")
	stopService(t, "redis")
	for i := 0; i < 3; i++ {
		_, _, _ = doJSONBestEffort(http.MethodPost, baseURL+"/api/v1/login", "", map[string]any{
			"login":    redisEmail,
			"password": "Password123",
		})
		time.Sleep(300 * time.Millisecond)
	}
	startService(t, "redis")
	waitAPIReady(t, baseURL)

	if promURL != "" {
		waitPrometheus(t, promURL)
		assertMetric(t, promURL, "http_requests_total")
		assertMetric(t, promURL, "http_request_duration_seconds_bucket")
		assertMetric(t, promURL, "http_in_flight_requests")
		assertMetric(t, promURL, "http_errors_total")
		assertMetric(t, promURL, "auth_events_total")
		assertMetric(t, promURL, "auth_event_reasons_total")
		assertMetric(t, promURL, "redis_degraded_total")
		assertMetric(t, promURL, "email_send_errors_total")
		assertMetric(t, promURL, "email_circuit_open_total")
		assertMetric(t, promURL, "email_circuit_state")
	}
}

func registerAndLogin(t *testing.T, baseURL, email, username, password string) string {
	t.Helper()
	status, _ := doJSON(t, http.MethodPost, baseURL+"/api/v1/register", "", map[string]any{
		"email":    email,
		"username": username,
		"password": password,
	})
	if status != http.StatusCreated && status != http.StatusConflict {
		t.Fatalf("register status=%d", status)
	}
	deadline := time.Now().Add(25 * time.Second)
	for time.Now().Before(deadline) {
		status, body := doJSON(t, http.MethodPost, baseURL+"/api/v1/login", "", map[string]any{
			"login":    email,
			"password": password,
		})
		if status == http.StatusOK {
			var tok struct {
				AccessToken string `json:"access_token"`
			}
			if err := json.Unmarshal(body, &tok); err != nil {
				t.Fatalf("unmarshal token: %v", err)
			}
			return tok.AccessToken
		}
		if status == http.StatusUnauthorized || status == http.StatusTooManyRequests || status == http.StatusServiceUnavailable {
			time.Sleep(700 * time.Millisecond)
			continue
		}
		t.Fatalf("login status=%d body=%s", status, body)
	}
	t.Fatalf("login did not succeed for %s within retry window", email)
	return ""
}

func loginFail(t *testing.T, baseURL, login, password string) {
	t.Helper()
	status, _ := doJSON(t, http.MethodPost, baseURL+"/api/v1/login", "", map[string]any{
		"login":    login,
		"password": password,
	})
	if status != http.StatusUnauthorized {
		t.Fatalf("expected 401 login fail, got %d", status)
	}
}

func createTeam(t *testing.T, baseURL, token, name string) int64 {
	t.Helper()
	status, body := doJSON(t, http.MethodPost, baseURL+"/api/v1/teams", token, map[string]any{"name": name})
	if status != http.StatusCreated {
		t.Fatalf("create team status=%d body=%s", status, body)
	}
	var resp struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal(body, &resp)
	return resp.ID
}

func invite(t *testing.T, baseURL, token string, teamID int64, email, role string, want ...int) {
	t.Helper()
	status, _ := doJSON(t, http.MethodPost, baseURL+"/api/v1/teams/"+itoa(teamID)+"/invite", token, map[string]any{
		"email": email,
		"role":  role,
	})
	for _, w := range want {
		if status == w {
			return
		}
	}
	t.Fatalf("invite status=%d want=%v", status, want)
}

func createTask(t *testing.T, baseURL, token string, teamID int64, title string) int64 {
	t.Helper()
	status, body := doJSON(t, http.MethodPost, baseURL+"/api/v1/tasks", token, map[string]any{
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

func updateTask(t *testing.T, baseURL, token string, taskID int64, payload map[string]any, want int) {
	t.Helper()
	status, _ := doJSON(t, http.MethodPut, baseURL+"/api/v1/tasks/"+itoa(taskID), token, payload)
	if status != want {
		t.Fatalf("update task status=%d want=%d", status, want)
	}
}

func createComment(t *testing.T, baseURL, token string, taskID int64, body string, want int) {
	t.Helper()
	status, _ := doJSON(t, http.MethodPost, baseURL+"/api/v1/tasks/"+itoa(taskID)+"/comments", token, map[string]any{"body": body})
	if status != want {
		t.Fatalf("create comment status=%d want=%d", status, want)
	}
}

func getHistory(t *testing.T, baseURL, token string, taskID int64, want int) {
	t.Helper()
	status, _ := doJSON(t, http.MethodGet, baseURL+"/api/v1/tasks/"+itoa(taskID)+"/history?limit=20&offset=0", token, nil)
	if status != want {
		t.Fatalf("history status=%d want=%d", status, want)
	}
}

func doJSON(t *testing.T, method, url, token string, payload any) (int, []byte) {
	t.Helper()
	var raw []byte
	if payload != nil {
		var err error
		raw, err = json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
	}
	var resp *http.Response
	var err error
	var req *http.Request
	for i := 0; i < 5; i++ {
		var body *bytes.Reader
		if raw != nil {
			body = bytes.NewReader(raw)
		} else {
			body = bytes.NewReader(nil)
		}
		req, err = http.NewRequest(method, url, body)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err = http.DefaultClient.Do(req)
		if err == nil && resp == nil {
			err = errors.New("nil response")
		}
		if err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("http request: %v", err)
	}
	if resp == nil || resp.Body == nil {
		t.Fatalf("http request: invalid response")
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

func doJSONBestEffort(method, url, token string, payload any) (int, []byte, error) {
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return 0, nil, err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return 0, nil, err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b, nil
}

func assertMetric(t *testing.T, promURL, metric string) {
	t.Helper()
	deadline := time.Now().Add(40 * time.Second)
	for time.Now().Before(deadline) {
		query := promURL + "/api/v1/query?query=" + metric
		resp, err := http.Get(query)
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		var out struct {
			Status string `json:"status"`
			Data   struct {
				Result []any `json:"result"`
			} `json:"data"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&out)
		resp.Body.Close()
		if out.Status == "success" && len(out.Data.Result) > 0 {
			return
		}
		time.Sleep(1 * time.Second)
	}
	t.Fatalf("metric %s not found in prometheus", metric)
}

func waitPrometheus(t *testing.T, promURL string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(promURL + "/-/ready")
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("prometheus not ready")
}

func waitAPIReady(t *testing.T, baseURL string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(baseURL + "/health")
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatalf("api not ready")
}

func stopService(t *testing.T, service string) {
	t.Helper()
	runCompose(t, "stop", service)
	time.Sleep(1 * time.Second)
}

func startService(t *testing.T, service string) {
	t.Helper()
	runCompose(t, "start", service)
	time.Sleep(2 * time.Second)
}

func runCompose(t *testing.T, args ...string) {
	t.Helper()
	compose, err := findCompose()
	if err != nil {
		t.Fatalf("docker-compose not found: %v", err)
	}
	cmd := exec.Command(compose[0], append(compose[1:], args...)...)
	cmd.Dir = repoRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("compose %v failed: %v, out=%s", args, err, out)
	}
}

func findCompose() ([]string, error) {
	if _, err := exec.LookPath("docker-compose"); err == nil {
		return []string{"docker-compose"}, nil
	}
	if _, err := exec.LookPath("docker"); err == nil {
		return []string{"docker", "compose"}, nil
	}
	return nil, errors.New("docker-compose or docker not found")
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("wd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			break
		}
		wd = parent
	}
	t.Fatalf("repo root not found")
	return ""
}

func getenv(k, def string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	return v
}

func itoa(v int64) string {
	return strconv.FormatInt(v, 10)
}
