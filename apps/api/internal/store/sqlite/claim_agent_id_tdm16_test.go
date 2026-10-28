package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentcanvas/api/internal/store"
)

// TDM-16: a claim carries the registered agent id it resolved to, so presence
// views key claims by id instead of by a display name that can be shared.

func TestTDM16ClaimStampsAgentID(t *testing.T) {
	s := openTest(t, WithClaimTTL(50*time.Millisecond))
	ctx := context.Background()
	c := newCanvas(t, s)
	alpha := &store.Agent{Name: "executor", Role: "executor"}
	if _, err := s.RegisterAgent(ctx, c.ID, alpha); err != nil {
		t.Fatal(err)
	}
	beta := &store.Agent{Name: "opus-worker", Role: "executor"}
	if _, err := s.RegisterAgent(ctx, c.ID, beta); err != nil {
		t.Fatal(err)
	}
	idOf := func(a *store.Action) string {
		if a.ClaimedByAgentID == nil {
			return ""
		}
		return *a.ClaimedByAgentID
	}

	// Claim by registered NAME → the agent's id rides along.
	t1 := newTask(t, s, c.ID, "approved", `{"title":"one"}`)
	got, _, err := s.ClaimAction(ctx, c.ID, t1.ID, "executor")
	if err != nil || idOf(got) != alpha.ID.String() {
		t.Fatalf("name claim: id=%q err=%v, want %s", idOf(got), err, alpha.ID)
	}
	// One agent may hold several claims; each row carries the same id.
	t2 := newTask(t, s, c.ID, "approved", `{"title":"two"}`)
	if got, _, err = s.ClaimAction(ctx, c.ID, t2.ID, "executor"); err != nil || idOf(got) != alpha.ID.String() {
		t.Fatalf("second claim: id=%q err=%v", idOf(got), err)
	}
	// Claim by agent ID (a nameless session's identity) resolves too.
	t3 := newTask(t, s, c.ID, "approved", `{"title":"three"}`)
	if got, _, err = s.ClaimAction(ctx, c.ID, t3.ID, beta.ID.String()); err != nil || idOf(got) != beta.ID.String() {
		t.Fatalf("id claim: id=%q err=%v", idOf(got), err)
	}
	// Unregistered and generic claimants store NULL (readers fall back to name).
	t4 := newTask(t, s, c.ID, "approved", `{"title":"four"}`)
	if got, _, err = s.ClaimAction(ctx, c.ID, t4.ID, "ci-bot"); err != nil || got.ClaimedByAgentID != nil {
		t.Fatalf("unregistered claim: %v err=%v", got.ClaimedByAgentID, err)
	}
	t5 := newTask(t, s, c.ID, "approved", `{"title":"five"}`)
	if got, _, err = s.ClaimAction(ctx, c.ID, t5.ID, "agent"); err != nil || got.ClaimedByAgentID != nil {
		t.Fatalf("generic claim: %v err=%v", got.ClaimedByAgentID, err)
	}
	// Agent ids are canvas-scoped: another canvas's agent of the same name is not us.
	other := newCanvas(t, s)
	if _, err := s.RegisterAgent(ctx, other.ID, &store.Agent{Name: "elsewhere", Role: "executor"}); err != nil {
		t.Fatal(err)
	}
	t6 := newTask(t, s, c.ID, "approved", `{"title":"six"}`)
	if got, _, err = s.ClaimAction(ctx, c.ID, t6.ID, "elsewhere"); err != nil || got.ClaimedByAgentID != nil {
		t.Fatalf("cross-canvas name resolved: %v err=%v", got.ClaimedByAgentID, err)
	}

	// The stored value round-trips through the read paths the board uses.
	listed, err := s.ListActions(ctx, c.ID, "executing", "task", "")
	if err != nil {
		t.Fatal(err)
	}
	byAgent := map[string]int{}
	for _, a := range listed {
		byAgent[idOf(a)]++
	}
	if byAgent[alpha.ID.String()] != 2 || byAgent[beta.ID.String()] != 1 {
		t.Fatalf("listed claims by agent id: %v", byAgent)
	}

	// An expired-claim takeover restamps the id to the new holder.
	time.Sleep(80 * time.Millisecond)
	taken, out, err := s.ClaimAction(ctx, c.ID, t1.ID, "opus-worker")
	if err != nil || out.ExpiredClaimBy != "executor" || idOf(taken) != beta.ID.String() {
		t.Fatalf("takeover: id=%q out=%+v err=%v", idOf(taken), out, err)
	}

	// Releasing clears it with the name.
	released, _, err := s.ReleaseAction(ctx, c.ID, t1.ID)
	if err != nil || released.ClaimedBy != nil || released.ClaimedByAgentID != nil {
		t.Fatalf("release left a claimant: %+v err=%v", released, err)
	}
	reread, err := s.GetAction(ctx, c.ID, t1.ID)
	if err != nil || reread.ClaimedByAgentID != nil {
		t.Fatalf("reread after release: %+v err=%v", reread, err)
	}
	// …and a later claim by an unregistered name doesn't inherit the old id.
	if got, _, err = s.ClaimAction(ctx, c.ID, t1.ID, "ci-bot"); err != nil || got.ClaimedByAgentID != nil {
		t.Fatalf("re-claim inherited a stale agent id: %v err=%v", got.ClaimedByAgentID, err)
	}
}

// TestTDM16MigrateV1AddsClaimedByAgentID upgrades a database written by the v1
// binary (actions without the column) and checks the step lands it at v2 with a
// working column, without disturbing existing rows.
func TestTDM16MigrateV1AddsClaimedByAgentID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v1.db")
	v1Schema := strings.Replace(schemaSQL, "  claimed_by_agent_id TEXT,\n", "", 1)
	if v1Schema == schemaSQL {
		t.Fatal("test setup: could not derive the v1 schema from schema.sql")
	}
	raw, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		v1Schema,
		`CREATE TABLE schema_version (version INTEGER NOT NULL, applied_at TEXT NOT NULL)`,
		`INSERT INTO schema_version (version, applied_at) VALUES (1, '2026-10-01T00:00:00Z')`,
	} {
		if _, err := raw.Exec(stmt); err != nil {
			raw.Close()
			t.Fatalf("seed v1: %v", err)
		}
	}
	var cols int
	_ = raw.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('actions') WHERE name = 'claimed_by_agent_id'`).Scan(&cols)
	raw.Close()
	if cols != 0 {
		t.Fatal("test setup: v1 database already has the column")
	}

	s, err := Open(path)
	if err != nil {
		t.Fatalf("open v1 database: %v", err)
	}
	defer s.Close()
	var maxV int
	if err := s.DB().QueryRow(`SELECT MAX(version) FROM schema_version`).Scan(&maxV); err != nil || maxV != schemaVersion {
		t.Fatalf("schema version after upgrade = %d (err %v), want %d", maxV, err, schemaVersion)
	}
	ctx := context.Background()
	c := newCanvas(t, s)
	a := &store.Agent{Name: "upgraded", Role: "executor"}
	if _, err := s.RegisterAgent(ctx, c.ID, a); err != nil {
		t.Fatal(err)
	}
	task := newTask(t, s, c.ID, "approved", `{"title":"after upgrade"}`)
	got, _, err := s.ClaimAction(ctx, c.ID, task.ID, "upgraded")
	if err != nil || got.ClaimedByAgentID == nil || *got.ClaimedByAgentID != a.ID.String() {
		t.Fatalf("claim on upgraded db: %+v err=%v", got, err)
	}
}
