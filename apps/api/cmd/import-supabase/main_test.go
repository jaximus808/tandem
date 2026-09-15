package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/agentcanvas/api/internal/api"
	"github.com/agentcanvas/api/internal/store/sqlite"
)

// fakeSource serves hosted-shaped rows: PostgREST JSON, Postgres timestamps
// with offsets, jsonb as objects, text[] as arrays, booleans, a foreign owner.
type fakeSource struct {
	data  map[string][]map[string]any
	calls []string
}

func (f *fakeSource) rows(_ context.Context, table, filter string) ([]map[string]any, error) {
	f.calls = append(f.calls, table+"?"+filter)
	var out []map[string]any
	for _, r := range f.data[table] {
		if filter == "" || matches(r, filter) {
			out = append(out, r)
		}
	}
	return out, nil
}

func matches(row map[string]any, filter string) bool {
	// "col=eq.value"
	var col, val string
	for i := 0; i < len(filter); i++ {
		if filter[i] == '=' {
			col, val = filter[:i], filter[i+4:]
			break
		}
	}
	s, _ := row[col].(string)
	return s == val
}

func hostedFixture() (*fakeSource, string, string) {
	canvasID := uuid.New().String()
	otherOwner := uuid.New().String()
	docID := uuid.New().String()
	taskID := uuid.New().String()
	return &fakeSource{data: map[string][]map[string]any{
		"canvases": {{
			"id": canvasID, "code": "TEGLQFXR", "name": "tandem planning", "mode": "map",
			"enabled_modes": []any{"map", "docs", "roadmap"}, "owner_user_id": otherOwner,
			"claim_token": "clm_" + uuid.New().String(), "visibility": "private", "public_role": "read",
			"approval_policy": "epic", "require_cross_model_review": false, "briefing_doc_id": docID,
			"next_ticket": float64(215), "version": float64(9001), "map_id": "world",
			"created_at": "2026-05-01T10:00:00.123456+00:00", "updated_at": "2026-09-01T10:00:00+00:00",
		}},
		"documents": {{
			"id": docID, "canvas_id": canvasID, "type": "notes", "name": "Thesis", "sort_order": float64(0),
			"config": map[string]any{"k": "v"}, "created_by": "human", "authored_by": "human",
			"created_at": "2026-05-01T10:00:00+00:00", "updated_at": "2026-05-01T10:00:00+00:00",
		}},
		"notes": {{
			"id": uuid.New().String(), "canvas_id": canvasID, "document_id": docID, "body": "# Thesis\nhello",
			"image_refs": []any{}, "sort_order": float64(0), "created_by": "human",
			"created_at": "2026-05-01T10:00:00+00:00", "updated_at": "2026-05-01T10:00:00+00:00",
		}},
		"roadmap_items": {},
		"agents": {{
			"id": uuid.New().String(), "canvas_id": canvasID, "name": "planner-1", "role": "planner", "status": "offline",
			"created_at": "2026-05-01T10:00:00+00:00", "last_seen_at": "2026-05-01T10:00:00+00:00",
		}},
		"actions": {{
			"id": taskID, "canvas_id": canvasID, "type": "task", "state": "done",
			"payload": map[string]any{"title": "Ship it", "body": "b", "assignee": "agent"},
			"proposed_by": "agent", "ticket": float64(214), "linked_pin_ids": []any{}, "authored_by": "agent:x",
			"result": "done in abc123", "claimed_by": nil,
			"created_at": "2026-08-01T10:00:00+00:00", "updated_at": "2026-08-02T10:00:00+00:00",
		}},
		"webhooks": {{
			"id": uuid.New().String(), "canvas_id": canvasID, "url": "http://127.0.0.1:8787/webhook",
			"events": []any{"task.approved"}, "enabled": true, "name": "listen", "description": "", "secret": "whsec_x",
			"created_by": "human", "created_at": "2026-08-01T10:00:00+00:00", "updated_at": "2026-08-01T10:00:00+00:00",
		}},
	}}, canvasID, taskID
}

func TestImportRemapsAndPreservesIdentity(t *testing.T) {
	src, canvasID, taskID := hostedFixture()
	st, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	imp := &importer{src: src, log: func(string, ...any) {}}
	report, err := imp.run(context.Background(), st)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if report["canvases"] != 1 || report["actions"] != 1 || report["documents"] != 1 || report["notes"] != 1 {
		t.Fatalf("report = %v", report)
	}

	ctx := context.Background()
	c, err := st.GetCanvasByCode(ctx, "TEGLQFXR")
	if err != nil {
		t.Fatalf("canvas not imported: %v", err)
	}
	if c.ID.String() != canvasID {
		t.Errorf("canvas id changed: %s", c.ID)
	}
	if c.OwnerUserID == nil || *c.OwnerUserID != api.LocalUserID {
		t.Errorf("owner = %v, want the local user", c.OwnerUserID)
	}
	if c.Mode != "docs" {
		t.Errorf("legacy mode 'map' → %q, want docs", c.Mode)
	}
	if len(c.EnabledModes) != 2 || c.EnabledModes[0] != "docs" || c.EnabledModes[1] != "roadmap" {
		t.Errorf("enabledModes = %v, want [docs roadmap]", c.EnabledModes)
	}
	if c.Visibility != "public" || c.Version != 9001 {
		t.Errorf("visibility/version = %s/%d", c.Visibility, c.Version)
	}

	a, err := st.GetActionByTicket(ctx, c.ID, 214)
	if err != nil {
		t.Fatalf("ticket TDM-214 not importable: %v", err)
	}
	if a.ID.String() != taskID || a.State != "done" || a.Result == nil || *a.Result != "done in abc123" {
		t.Errorf("action = %+v", a)
	}
	var payload map[string]any
	if err := json.Unmarshal(a.Payload, &payload); err != nil || payload["title"] != "Ship it" {
		t.Errorf("payload = %s (%v)", a.Payload, err)
	}
	// The next ticket keeps counting from where the hosted board left off.
	next, err := st.ReserveTaskTickets(ctx, c.ID, 1)
	if err != nil || next != 215 {
		t.Errorf("next ticket = %d (%v), want 215", next, err)
	}

	_, state, _, err := st.GetCanvasState(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Documents) != 1 || len(state.Notes) != 1 || len(state.Agents) != 1 {
		t.Errorf("state counts docs=%d notes=%d agents=%d", len(state.Documents), len(state.Notes), len(state.Agents))
	}
	for _, d := range state.Documents {
		if d.Config["k"] != "v" || d.Name != "Thesis" {
			t.Errorf("document round trip: %+v", d)
		}
	}
	hooks, err := st.ListWebhooks(ctx, c.ID)
	if err != nil || len(hooks) != 1 || !hooks[0].Enabled || len(hooks[0].Events) != 1 {
		t.Errorf("webhooks = %+v (%v)", hooks, err)
	}

	// Re-running is an upsert, not a duplicate.
	if _, err := imp.run(context.Background(), st); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if _, state, _, _ = st.GetCanvasState(ctx, c.ID); len(state.Notes) != 1 {
		t.Errorf("second run duplicated notes: %d", len(state.Notes))
	}
}

func TestImportDryRunWritesNothing(t *testing.T) {
	src, _, _ := hostedFixture()
	st, _ := sqlite.Open(":memory:")
	defer st.Close()
	imp := &importer{src: src, dryRun: true, log: func(string, ...any) {}}
	if _, err := imp.run(context.Background(), st); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetCanvasByCode(context.Background(), "TEGLQFXR"); err == nil {
		t.Fatal("dry run wrote the canvas")
	}
}

func TestImportOneCanvasByCode(t *testing.T) {
	src, _, _ := hostedFixture()
	st, _ := sqlite.Open(":memory:")
	defer st.Close()
	imp := &importer{src: src, code: "nope1234", log: func(string, ...any) {}}
	if _, err := imp.run(context.Background(), st); err == nil {
		t.Fatal("unknown code must fail loudly")
	}
	imp.code = "TEGLQFXR"
	if _, err := imp.run(context.Background(), st); err != nil {
		t.Fatal(err)
	}
	if src.calls[0] != "canvases?code=eq.TEGLQFXR" && src.calls[len(src.calls)-7] != "canvases?code=eq.TEGLQFXR" {
		t.Errorf("canvas filter not applied: %v", src.calls)
	}
}
