package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agentcanvas/api/internal/auth"
)

// A router built with no options is a LOCAL one: /api/config says so, there is
// no Google sign-in route, and serving index.html mints the local user's
// session so the page's fetches arrive as the owner.
func TestLocalModeRouter(t *testing.T) {
	dist := t.TempDir()
	if err := os.WriteFile(filepath.Join(dist, "index.html"), []byte("<title>Tandem</title>"), 0o644); err != nil {
		t.Fatal(err)
	}
	authSvc := auth.NewService("test-secret-local-mode", time.Hour)
	r := NewRouter(nil, nil, authSvc, nil, false, dist, t.TempDir(), "", nil, nil, WithVersion("9.9.9"))

	// /api/config
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/api/config = %d", rec.Code)
	}
	var cfg struct {
		Auth    string `json:"auth"`
		Local   bool   `json:"local"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Auth != "none" || !cfg.Local || cfg.Version != "9.9.9" {
		t.Fatalf("/api/config = %+v, want auth=none local=true version=9.9.9", cfg)
	}

	// /healthz
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/healthz = %d", rec.Code)
	}

	// No sign-in route in local mode.
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/auth/google", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("POST /api/auth/google in local mode = %d, want 404", rec.Code)
	}

	// index.html mints the local session cookie…
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/c/ABCDEFGH", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("SPA route = %d", rec.Code)
	}
	var session *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName {
			session = c
		}
	}
	if session == nil {
		t.Fatal("index.html did not set the local session cookie")
	}
	claims, err := authSvc.ValidateSession(session.Value)
	if err != nil {
		t.Fatalf("minted session invalid: %v", err)
	}
	if claims.UserID != LocalUserID {
		t.Fatalf("session user = %s, want LocalUserID", claims.UserID)
	}

	// …and a request that already carries a valid one gets no new cookie.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(session)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if len(rec.Result().Cookies()) != 0 {
		t.Fatalf("re-minted a session for a request that had one")
	}

	// Static assets are served, not rewritten to index.html.
	if err := os.WriteFile(filepath.Join(dist, "app.js"), []byte("js"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/app.js", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "js" {
		t.Fatalf("/app.js = %d %q", rec.Code, rec.Body.String())
	}
}

// Under TANDEM_AUTH=google nothing about the hosted posture changes: sign-in
// is routed, /api/config says google, and index.html mints no session.
func TestGoogleModeRouter(t *testing.T) {
	dist := t.TempDir()
	if err := os.WriteFile(filepath.Join(dist, "index.html"), []byte("<title>Tandem</title>"), 0o644); err != nil {
		t.Fatal(err)
	}
	authSvc := auth.NewService("test-secret-google-mode", time.Hour)
	r := NewRouter(nil, nil, authSvc, nil, false, dist, t.TempDir(), "", nil, nil, WithAuthMode(AuthModeGoogle))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	var cfg struct {
		Auth  string `json:"auth"`
		Local bool   `json:"local"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &cfg)
	if cfg.Auth != "google" || cfg.Local {
		t.Fatalf("/api/config = %+v, want auth=google local=false", cfg)
	}

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/auth/google", nil))
	if rec.Code == http.StatusNotFound {
		t.Fatalf("POST /api/auth/google in google mode = 404; the route must exist")
	}

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if len(rec.Result().Cookies()) != 0 {
		t.Fatalf("google mode minted a session on index.html")
	}
}
