package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/agentcanvas/api/internal/store"
	"github.com/google/uuid"
)

// TDM-22: same-role / same-name subagents must not merge into one agent.

// resolveAgentNameTDM22 is the handler fakes' half of RegisterAgentDistinct:
// apply store.ResolveAgentName over rows (LiveAfter defaulting from the default
// claim TTL, as the real store fills it from its configured one), set a.Name to
// the final name, and stamp a.LastSeenAt so later registrations see it live.
// The caller holds its fake's lock and then runs its ordinary upsert.
func resolveAgentNameTDM22(a *store.Agent, in store.AgentRegisterIntent, rows []*store.Agent) {
	if in.LiveAfter.IsZero() {
		in.LiveAfter = time.Now().Add(-store.DefaultClaimTTL)
	}
	find := func(match func(*store.Agent) bool) *store.Agent {
		for _, r := range rows {
			if match(r) {
				return r
			}
		}
		return nil
	}
	var existing, sessionAgent *store.Agent
	if a.Name != "" {
		existing = find(func(r *store.Agent) bool { return r.Name == a.Name })
	}
	if !in.NameGiven && in.SessionAgentID != nil {
		sessionAgent = find(func(r *store.Agent) bool { return r.ID == *in.SessionAgentID })
	}
	name, decision := store.ResolveAgentName(a, in, existing, sessionAgent)
	a.Name = name
	for decision == store.DistinctAgent {
		candidate := store.SuffixedAgentName(name)
		if find(func(r *store.Agent) bool { return r.Name == candidate }) == nil {
			a.Name = candidate
			break
		}
	}
	if a.LastSeenAt.IsZero() {
		a.LastSeenAt = time.Now().UTC()
	}
}

var suffixedExecutorTDM22 = regexp.MustCompile(`^executor-[a-z2-9]{4}$`)

// registerTDM22 registers via the handler (optionally asserting an
// X-Tandem-Agent identity) and returns the decoded response.
func registerTDM22(t *testing.T, h *Handler, canvasID uuid.UUID, body map[string]any, identity string) map[string]string {
	t.Helper()
	w := httptest.NewRecorder()
	r := canvasRequest(t, "POST", "/api/canvas/agents", body, canvasID, "")
	if identity != "" {
		r.Header.Set(AgentIdentityHeader, identity)
	}
	h.RegisterAgent(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("register status = %d, body %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal register response: %v", err)
	}
	if resp["agentId"] == "" || resp["name"] == "" {
		t.Fatalf("register response missing agentId/name: %v", resp)
	}
	return resp
}

func plannerFakeTDM22() (*agentFakeStore, uuid.UUID, uuid.UUID) {
	p1, p2 := uuid.New(), uuid.New()
	return &agentFakeStore{agents: map[uuid.UUID]*store.Agent{
		p1: {ID: p1, Name: "planner-one", Role: "planner"},
		p2: {ID: p2, Name: "planner-two", Role: "planner"},
	}}, p1, p2
}

// (a) Two nameless executors are two agents, each with a minted name.
func TestTDM22NamelessExecutorsAreDistinct(t *testing.T) {
	canvasID := uuid.New()
	fake := &agentFakeStore{}
	h := NewHandler(fake, nil)

	a := registerTDM22(t, h, canvasID, map[string]any{"role": "executor"}, "session-aaaa")
	b := registerTDM22(t, h, canvasID, map[string]any{"role": "executor"}, "session-bbbb")
	if a["agentId"] == b["agentId"] {
		t.Fatalf("nameless executors merged into %s", a["agentId"])
	}
	if a["name"] == b["name"] {
		t.Fatalf("nameless executors share a name %q", a["name"])
	}
	for _, r := range []map[string]string{a, b} {
		if !suffixedExecutorTDM22.MatchString(r["name"]) {
			t.Fatalf("minted name %q, want executor-<4 chars>", r["name"])
		}
		if _, ok := r["renamedFrom"]; ok {
			t.Fatalf("nameless register reported renamedFrom: %v", r)
		}
	}
	if n := fake.registeredCount(); n != 2 {
		t.Fatalf("registered rows = %d, want 2", n)
	}
}

// (b) One session re-registering keeps its agent: nameless with its agentId,
// and named while a live sibling-shaped row (itself) holds the name.
func TestTDM22SameSessionReRegisterRefreshes(t *testing.T) {
	canvasID := uuid.New()
	fake, parent, _ := plannerFakeTDM22()
	h := NewHandler(fake, nil)

	// Nameless: the session's agentId selects the row; the name is kept.
	first := registerTDM22(t, h, canvasID, map[string]any{"role": "executor"}, "session-aaaa")
	again := registerTDM22(t, h, canvasID, map[string]any{
		"role": "executor", "agentId": first["agentId"],
	}, first["name"])
	if again["agentId"] != first["agentId"] || again["name"] != first["name"] {
		t.Fatalf("nameless re-register by the same session moved: %v -> %v", first, again)
	}

	// Named + parented: the holder is live and under the same parent, but it IS
	// this session (X-Tandem-Agent names it), so it refreshes.
	w1 := registerTDM22(t, h, canvasID, map[string]any{
		"name": "worker", "role": "executor", "parentAgentId": parent.String(),
	}, "session-cccc")
	w2 := registerTDM22(t, h, canvasID, map[string]any{
		"name": "worker", "role": "executor", "parentAgentId": parent.String(),
	}, "worker")
	if w2["agentId"] != w1["agentId"] || w2["name"] != "worker" {
		t.Fatalf("same-session named re-register moved: %v -> %v", w1, w2)
	}
	// The body agentId alone is enough too.
	w3 := registerTDM22(t, h, canvasID, map[string]any{
		"name": "worker", "role": "executor", "parentAgentId": parent.String(),
		"agentId": w1["agentId"],
	}, "")
	if w3["agentId"] != w1["agentId"] {
		t.Fatalf("re-register carrying agentId moved: %v -> %v", w1, w3)
	}
	if n := fake.registeredCount(); n != 2 {
		t.Fatalf("registered rows = %d, want 2", n)
	}
}

// (c) Named registration without a parent is unchanged: same name, same agent,
// from any session.
func TestTDM22NamedWithoutParentUnchanged(t *testing.T) {
	canvasID := uuid.New()
	fake := &agentFakeStore{}
	h := NewHandler(fake, nil)

	a := registerTDM22(t, h, canvasID, map[string]any{"name": "opus-1", "role": "executor"}, "session-aaaa")
	b := registerTDM22(t, h, canvasID, map[string]any{"name": "opus-1", "role": "executor"}, "session-bbbb")
	if a["agentId"] != b["agentId"] || b["name"] != "opus-1" {
		t.Fatalf("plain named re-register changed: %v -> %v", a, b)
	}
	if _, ok := b["renamedFrom"]; ok {
		t.Fatalf("plain named re-register reported renamedFrom: %v", b)
	}
}

// Siblings under one planner sharing a name are suffixed, not merged.
func TestTDM22LiveSiblingSameNameSuffixed(t *testing.T) {
	canvasID := uuid.New()
	fake, parent, _ := plannerFakeTDM22()
	h := NewHandler(fake, nil)

	body := map[string]any{"name": "executor", "role": "executor", "parentAgentId": parent.String()}
	a := registerTDM22(t, h, canvasID, body, "session-aaaa")
	b := registerTDM22(t, h, canvasID, body, "session-bbbb")
	if a["agentId"] == b["agentId"] {
		t.Fatalf("live siblings merged into %s", a["agentId"])
	}
	if a["name"] != "executor" || !suffixedExecutorTDM22.MatchString(b["name"]) {
		t.Fatalf("names = %q, %q; want executor and executor-<4>", a["name"], b["name"])
	}
	if b["renamedFrom"] != "executor" {
		t.Fatalf("renamedFrom = %q, want executor", b["renamedFrom"])
	}
}

// A name held under a DIFFERENT parent is suffixed, even when that holder is
// stale.
func TestTDM22DifferentParentSuffixed(t *testing.T) {
	canvasID := uuid.New()
	fake, p1, p2 := plannerFakeTDM22()
	h := NewHandler(fake, nil)

	a := registerTDM22(t, h, canvasID, map[string]any{
		"name": "worker", "role": "executor", "parentAgentId": p1.String(),
	}, "session-aaaa")
	fake.agentNamed("worker").LastSeenAt = time.Now().Add(-24 * time.Hour)
	b := registerTDM22(t, h, canvasID, map[string]any{
		"name": "worker", "role": "executor", "parentAgentId": p2.String(),
	}, "session-bbbb")
	if a["agentId"] == b["agentId"] || b["name"] == "worker" {
		t.Fatalf("different-parent collision merged: %v / %v", a, b)
	}
	if got := fake.agentNamed("worker"); got.ParentAgentID == nil || *got.ParentAgentID != p1 {
		t.Fatalf("original holder was re-parented: %+v", got)
	}
}

// A stale holder under the same parent is reused (the worker coming back after
// its claim would have lapsed anyway).
func TestTDM22StaleSameParentRefreshes(t *testing.T) {
	canvasID := uuid.New()
	fake, parent, _ := plannerFakeTDM22()
	h := NewHandler(fake, nil)

	body := map[string]any{"name": "worker", "role": "executor", "parentAgentId": parent.String()}
	a := registerTDM22(t, h, canvasID, body, "session-aaaa")
	fake.agentNamed("worker").LastSeenAt = time.Now().Add(-store.DefaultClaimTTL - time.Minute)
	b := registerTDM22(t, h, canvasID, body, "session-bbbb")
	if a["agentId"] != b["agentId"] || b["name"] != "worker" {
		t.Fatalf("stale same-parent holder not reused: %v -> %v", a, b)
	}
}
