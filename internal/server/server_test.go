package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/en9inerd/gitreal/internal/config"
	"github.com/en9inerd/gitreal/internal/github"
)

func setupTestServer(t *testing.T, apiEnabled bool, ghServer *httptest.Server) http.Handler {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := &config.Config{
		Port:       "8080",
		Verbose:    false,
		APIEnabled: apiEnabled,
	}

	if ghServer != nil {
		github.SetBaseURL(ghServer.URL)
		t.Cleanup(func() { github.SetBaseURL("https://api.github.com") })
	}

	ghClient := github.NewClient("")
	handler, err := NewServer(logger, cfg, ghClient)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	return handler
}

func mockGitHubAPI() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /users/testuser", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(github.UserProfile{
			Login: "testuser", Name: "Test User",
			Followers: 10, Following: 5,
		})
	})
	mux.HandleFunc("GET /users/testuser/repos", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]github.Repo{{Name: "repo1", Language: "Go", Size: 100, Description: "test"}})
	})
	mux.HandleFunc("GET /users/testuser/events", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]github.Event{})
	})
	mux.HandleFunc("GET /users/testuser/orgs", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]github.Org{})
	})
	mux.HandleFunc("GET /users/testuser/starred", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]json.RawMessage{})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	return httptest.NewServer(mux)
}

func TestHomePage(t *testing.T) {
	handler := setupTestServer(t, false, nil)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status=%d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("content-type=%q, want text/html", ct)
	}
	if !strings.Contains(w.Body.String(), "GitReal") {
		t.Error("home page should contain 'GitReal'")
	}
}

func TestNotFoundPage(t *testing.T) {
	handler := setupTestServer(t, false, nil)

	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status=%d, want 404", w.Code)
	}
}

func TestSecurityHeaders(t *testing.T) {
	handler := setupTestServer(t, false, nil)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	headers := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
	}
	for k, v := range headers {
		if got := w.Header().Get(k); got != v {
			t.Errorf("%s=%q, want %q", k, got, v)
		}
	}

	csp := w.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "default-src 'self'") {
		t.Errorf("CSP missing default-src: %q", csp)
	}
	if !strings.Contains(csp, "avatars.githubusercontent.com") {
		t.Errorf("CSP missing avatar domain: %q", csp)
	}
}

func TestWebScoreSuccess(t *testing.T) {
	ghSrv := mockGitHubAPI()
	defer ghSrv.Close()

	handler := setupTestServer(t, false, ghSrv)

	form := url.Values{"username": {"testuser"}}
	req := httptest.NewRequest(http.MethodPost, "/web/score", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status=%d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "testuser") {
		t.Error("response should contain username")
	}
	if !strings.Contains(body, "score-card") {
		t.Error("response should contain score card")
	}
}

func TestWebScoreEmptyUsername(t *testing.T) {
	handler := setupTestServer(t, false, nil)

	form := url.Values{"username": {""}}
	req := httptest.NewRequest(http.MethodPost, "/web/score", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status=%d, want 200 (error rendered in HTML)", w.Code)
	}
	if !strings.Contains(w.Body.String(), "required") {
		t.Error("should show required error")
	}
}

func TestWebScoreInvalidUsername(t *testing.T) {
	handler := setupTestServer(t, false, nil)

	form := url.Values{"username": {"!!!invalid!!!"}}
	req := httptest.NewRequest(http.MethodPost, "/web/score", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if !strings.Contains(w.Body.String(), "Invalid") {
		t.Error("should show invalid username error")
	}
}

func TestWebScoreStripsAtPrefix(t *testing.T) {
	ghSrv := mockGitHubAPI()
	defer ghSrv.Close()

	handler := setupTestServer(t, false, ghSrv)

	form := url.Values{"username": {"@testuser"}}
	req := httptest.NewRequest(http.MethodPost, "/web/score", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status=%d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "testuser") {
		t.Error("should resolve @testuser to testuser")
	}
}

func TestAPIDisabledByDefault(t *testing.T) {
	handler := setupTestServer(t, false, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/score", strings.NewReader(`{"username":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("API should return 404 when disabled, got %d", w.Code)
	}
}

func TestAPIEnabled(t *testing.T) {
	ghSrv := mockGitHubAPI()
	defer ghSrv.Close()

	handler := setupTestServer(t, true, ghSrv)

	req := httptest.NewRequest(http.MethodPost, "/api/score", strings.NewReader(`{"username":"testuser"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status=%d, want 200", w.Code)
	}

	var result map[string]any
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}
	if result["username"] != "testuser" {
		t.Errorf("username=%v, want testuser", result["username"])
	}
	if _, ok := result["total"]; !ok {
		t.Error("response should contain 'total'")
	}
	if _, ok := result["verdict"]; !ok {
		t.Error("response should contain 'verdict'")
	}
}

func TestAPIInvalidUsername(t *testing.T) {
	handler := setupTestServer(t, true, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/score", strings.NewReader(`{"username":"--bad--"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", w.Code)
	}
}

func TestAPIEmptyBody(t *testing.T) {
	handler := setupTestServer(t, true, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/score", strings.NewReader(`{"username":""}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", w.Code)
	}
}

func TestStaticFiles(t *testing.T) {
	handler := setupTestServer(t, false, nil)

	req := httptest.NewRequest(http.MethodGet, "/static/css/style.css", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status=%d, want 200", w.Code)
	}
	if !strings.Contains(w.Header().Get("Content-Type"), "css") {
		t.Errorf("expected CSS content-type, got %q", w.Header().Get("Content-Type"))
	}
}

func TestHealthEndpoint(t *testing.T) {
	handler := setupTestServer(t, false, nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status=%d, want 200", w.Code)
	}
}

func TestTemplateFuncs(t *testing.T) {
	fns := templateFuncs()

	scoreClass := fns["scoreClass"].(func(int, int) string)
	if scoreClass(80, 100) != "high" {
		t.Error("80/100 should be high")
	}
	if scoreClass(50, 100) != "mid" {
		t.Error("50/100 should be mid")
	}
	if scoreClass(20, 100) != "low" {
		t.Error("20/100 should be low")
	}
	if scoreClass(0, 0) != "low" {
		t.Error("0/0 should be low (div by zero guard)")
	}

	pct := fns["pct"].(func(int, int) int)
	if pct(50, 100) != 50 {
		t.Errorf("pct(50,100)=%d, want 50", pct(50, 100))
	}
	if pct(0, 0) != 0 {
		t.Errorf("pct(0,0)=%d, want 0", pct(0, 0))
	}
}
