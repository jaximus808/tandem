package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agentcanvas/api/internal/auth"
	"github.com/agentcanvas/api/internal/store"
	"github.com/agentcanvas/api/internal/transcripts"
	"github.com/google/uuid"
)

// tdm18TranscriptStore knows exactly one agent, on exactly one canvas. Anything
// else the router reaches panics via the embedded nil interface.
type tdm18TranscriptStore struct {
	store.Store
	canvasID uuid.UUID
	agentID  uuid.UUID
}

func (f *tdm18TranscriptStore) GetAgent(_ context.Context, canvasID, id uuid.UUID) (*store.Agent, error) {
	if canvasID == f.canvasID && id == f.agentID {
		return &store.Agent{ID: id, Kind: "agent", Name: "demo-worker", Role: "executor"}, nil
	}
	return nil, fmt.Errorf("agent %s not found", id)
}

// tdm18WriteClaudeDir lays out a synthetic ~/.claude with one subagent
// transcript whose canvas_connect answer names agentID, and points
// TANDEM_CLAUDE_DIR at it.
func tdm18WriteClaudeDir(t *testing.T, agentID uuid.UUID) string {
	t.Helper()
	claude := t.TempDir()
	dir := filepath.Join(claude, "projects", "-Users-demo-repo", "s-1", "subagents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	lines := []string{
		`{"type":"user","timestamp":"2026-01-02T10:00:00Z","message":{"role":"user","content":"Handoff: do DEMO-1"}}`,
		`{"type":"assistant","timestamp":"2026-01-02T10:00:01Z","message":{"role":"assistant","model":"m","content":[{"type":"tool_use","id":"toolu_c","name":"mcp__tandem__canvas_connect","input":{"code":"DEMOCODE"}}]}}`,
		`{"type":"user","timestamp":"2026-01-02T10:00:02Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_c","content":[{"type":"text","text":"{\"connected\":true,\"agentId\":\"` + agentID.String() + `\"}"}]}]}}`,
		`{"type":"assistant","timestamp":"2026-01-02T10:00:03Z","message":{"role":"assistant","model":"m","content":[{"type":"text","text":"Working on it."}]}}`,
	}
	var body string
	for _, l := range lines {
		body += l + "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "agent-x1.jsonl"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "agent-x1.meta.json"), []byte(`{"agentType":"general-purpose","description":"Work DEMO-1","finished":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TANDEM_CLAUDE_DIR", claude)
	t.Setenv("TANDEM_TRANSCRIPTS", "")
	return claude
}

type tdm18Fixture struct {
	router   http.Handler
	token    string
	canvasID uuid.UUID
	agentID  uuid.UUID
}

func tdm18NewFixture(t *testing.T, opts ...RouterOption) *tdm18Fixture {
	t.Helper()
	f := &tdm18Fixture{canvasID: uuid.New(), agentID: uuid.New()}
	tdm18WriteClaudeDir(t, f.agentID)
	return f.build(t, opts...)
}

// build (re)creates the router — after changing the environment, since the
// index is built from it at router construction.
func (f *tdm18Fixture) build(t *testing.T, opts ...RouterOption) *tdm18Fixture {
	t.Helper()
	authSvc := auth.NewService("test-secret-for-tdm-18", time.Hour)
	token, err := authSvc.Issue(f.canvasID, "read", nil)
	if err != nil {
		t.Fatal(err)
	}
	f.token = token
	f.router = NewRouter(&tdm18TranscriptStore{canvasID: f.canvasID, agentID: f.agentID}, nil, authSvc, nil, false, "", t.TempDir(), "", nil, nil, opts...)
	return f
}

func (f *tdm18Fixture) get(t *testing.T, path string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer "+f.token)
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, req)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func (f *tdm18Fixture) path(agentID uuid.UUID, query string) string {
	return "/api/canvas/agents/" + agentID.String() + "/transcript" + query
}

func (f *tdm18Fixture) configTranscripts(t *testing.T) any {
	t.Helper()
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	var cfg map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &cfg)
	return cfg["transcripts"]
}

func TestTDM18TranscriptServesLocalAgent(t *testing.T) {
	f := tdm18NewFixture(t)
	if got := f.configTranscripts(t); got != true {
		t.Fatalf("/api/config transcripts = %v, want true", got)
	}

	code, body := f.get(t, f.path(f.agentID, ""))
	if code != http.StatusOK || body["found"] != true || body["type"] != "agent.transcript" {
		t.Fatalf("GET = %d %v", code, body)
	}
	tr, _ := body["transcript"].(map[string]any)
	if tr["kind"] != "subagent" || tr["id"] != "s-1/agent-x1" || tr["sessionId"] != "s-1" {
		t.Fatalf("transcript = %v", tr)
	}
	if meta, _ := tr["subagent"].(map[string]any); meta["description"] != "Work DEMO-1" || meta["finished"] != true {
		t.Fatalf("subagent meta = %v", tr["subagent"])
	}
	events, _ := body["events"].([]any)
	if len(events) != 4 {
		t.Fatalf("events = %v", events)
	}
	last, _ := events[3].(map[string]any)
	if last["kind"] != "text" || last["text"] != "Working on it." {
		t.Fatalf("last event = %v", last)
	}
	next := int64(body["next"].(float64))
	if next == 0 || body["size"].(float64) != float64(next) {
		t.Fatalf("next = %v size = %v", body["next"], body["size"])
	}

	// Resume from next: nothing new.
	code, body = f.get(t, f.path(f.agentID, fmt.Sprintf("?after=%d&transcript=s-1/agent-x1", next)))
	if code != http.StatusOK || len(body["events"].([]any)) != 0 || body["reset"] != false {
		t.Fatalf("resume = %d %v", code, body)
	}
	// A stale transcript id restarts from 0 and says reset.
	code, body = f.get(t, f.path(f.agentID, fmt.Sprintf("?after=%d&transcript=other", next)))
	if code != http.StatusOK || len(body["events"].([]any)) != 4 || body["reset"] != true {
		t.Fatalf("stale transcript id = %d %v", code, body)
	}
	// tail=1 is the last event.
	code, body = f.get(t, f.path(f.agentID, "?tail=1"))
	if code != http.StatusOK || len(body["events"].([]any)) != 1 {
		t.Fatalf("tail = %d %v", code, body)
	}
}

func TestTDM18TranscriptScopedToCanvasAgents(t *testing.T) {
	f := tdm18NewFixture(t)
	// An agent this canvas doesn't know — even one whose transcript exists on
	// disk under another canvas — is a 404, not a file read.
	code, body := f.get(t, f.path(uuid.New(), ""))
	if code != http.StatusNotFound || body["error"] != "agent_not_found" {
		t.Fatalf("foreign agent = %d %v", code, body)
	}
	code, body = f.get(t, "/api/canvas/agents/not-a-uuid/transcript")
	if code != http.StatusBadRequest || body["error"] != "invalid_agent_id" {
		t.Fatalf("bad id = %d %v", code, body)
	}
	for _, q := range []string{"?after=-1", "?after=x", "?limit=0", "?limit=5000", "?tail=0", "?tail=3&after=10"} {
		if code, body := f.get(t, f.path(f.agentID, q)); code != http.StatusBadRequest || body["error"] != "invalid_query" {
			t.Fatalf("%s = %d %v", q, code, body)
		}
	}
	// No token: the same 401 every canvas read gives.
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, f.path(f.agentID, ""), nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("no token = %d", w.Code)
	}
}

func TestTDM18TranscriptNoMatchIsQuietNotFound(t *testing.T) {
	f := &tdm18Fixture{canvasID: uuid.New(), agentID: uuid.New()}
	tdm18WriteClaudeDir(t, uuid.New()) // the file on disk names someone else
	f.build(t)
	code, body := f.get(t, f.path(f.agentID, ""))
	if code != http.StatusOK || body["found"] != false || len(body["events"].([]any)) != 0 {
		t.Fatalf("no transcript = %d %v", code, body)
	}
}

// Self-hosted servers must never expose the host's files: under
// TANDEM_AUTH=google the route refuses before touching the store or the disk
// (the fake store would panic on anything but GetAgent; the index is never
// built), and /api/config says transcripts are off.
func TestTDM18TranscriptRefusedUnderGoogleAuth(t *testing.T) {
	f := tdm18NewFixture(t, WithAuthMode(AuthModeGoogle))
	code, body := f.get(t, f.path(f.agentID, ""))
	if code != http.StatusForbidden || body["error"] != "transcripts_local_only" {
		t.Fatalf("google mode = %d %v", code, body)
	}
	if got := f.configTranscripts(t); got != false {
		t.Fatalf("/api/config transcripts = %v, want false", got)
	}
	// Direct handler check too: no index + not local still refuses as local-only.
	th := &transcriptHandler{index: transcripts.New(transcripts.Options{Root: t.TempDir()}), local: false}
	w := httptest.NewRecorder()
	th.GetAgentTranscript(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("handler with an index but not local = %d", w.Code)
	}
}

func TestTDM18TranscriptDisabledByEnv(t *testing.T) {
	f := tdm18NewFixture(t)
	t.Setenv("TANDEM_TRANSCRIPTS", "off")
	f.build(t)
	code, body := f.get(t, f.path(f.agentID, ""))
	if code != http.StatusNotFound || body["error"] != "transcripts_disabled" {
		t.Fatalf("TANDEM_TRANSCRIPTS=off = %d %v", code, body)
	}
	if got := f.configTranscripts(t); got != false {
		t.Fatalf("/api/config transcripts = %v, want false", got)
	}
}
