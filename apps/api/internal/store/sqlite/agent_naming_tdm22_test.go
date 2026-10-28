package sqlite

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/agentcanvas/api/internal/store"
)

// TDM-22: RegisterAgentDistinct against the real (canvas_id, name) unique
// index — nameless and sibling registrations get distinct rows, the session's
// own re-register and plain named registration still refresh.

func registerDistinctTDM22(t *testing.T, s *Store, canvasID uuid.UUID, a *store.Agent, in store.AgentRegisterIntent) *store.Agent {
	t.Helper()
	in.NameGiven = a.Name != ""
	if _, err := s.RegisterAgentDistinct(context.Background(), canvasID, a, in); err != nil {
		t.Fatalf("RegisterAgentDistinct %+v: %v", a, err)
	}
	return a
}

func TestTDM22StoreNamelessDistinct(t *testing.T) {
	s := openTest(t)
	c := newCanvas(t, s)

	a := registerDistinctTDM22(t, s, c.ID, &store.Agent{Role: "executor"}, store.AgentRegisterIntent{})
	b := registerDistinctTDM22(t, s, c.ID, &store.Agent{Role: "executor"}, store.AgentRegisterIntent{})
	if a.ID == b.ID || a.Name == b.Name {
		t.Fatalf("nameless executors merged: %+v / %+v", a, b)
	}
	for _, x := range []*store.Agent{a, b} {
		if !strings.HasPrefix(x.Name, "executor-") || len(x.Name) != len("executor-")+4 {
			t.Fatalf("minted name %q, want executor-<4>", x.Name)
		}
	}

	// The same session re-registering nameless keeps its row and name.
	again := registerDistinctTDM22(t, s, c.ID, &store.Agent{Role: "executor"}, store.AgentRegisterIntent{SessionAgentID: &a.ID})
	if again.ID != a.ID || again.Name != a.Name {
		t.Fatalf("session re-register moved: %+v -> %+v", a, again)
	}
	agents, _ := s.ListAgents(context.Background(), c.ID)
	if len(agents) != 2 {
		t.Fatalf("agents = %d, want 2", len(agents))
	}
}

func TestTDM22StoreSiblingsAndParents(t *testing.T) {
	s := openTest(t)
	c := newCanvas(t, s)
	ctx := context.Background()

	p1 := registerDistinctTDM22(t, s, c.ID, &store.Agent{Name: "planner-a", Role: "planner"}, store.AgentRegisterIntent{})
	p2 := registerDistinctTDM22(t, s, c.ID, &store.Agent{Name: "planner-b", Role: "planner"}, store.AgentRegisterIntent{})

	w1 := registerDistinctTDM22(t, s, c.ID, &store.Agent{Name: "worker", Role: "executor", ParentAgentID: &p1.ID},
		store.AgentRegisterIntent{SessionIdentity: "session-1"})
	// A live sibling under the same parent: suffixed.
	w2 := registerDistinctTDM22(t, s, c.ID, &store.Agent{Name: "worker", Role: "executor", ParentAgentID: &p1.ID},
		store.AgentRegisterIntent{SessionIdentity: "session-2"})
	if w2.ID == w1.ID || !strings.HasPrefix(w2.Name, "worker-") {
		t.Fatalf("live sibling merged: %+v / %+v", w1, w2)
	}
	// Under a different parent: suffixed.
	w3 := registerDistinctTDM22(t, s, c.ID, &store.Agent{Name: "worker", Role: "executor", ParentAgentID: &p2.ID},
		store.AgentRegisterIntent{SessionIdentity: "session-3"})
	if w3.ID == w1.ID || w3.ID == w2.ID || !strings.HasPrefix(w3.Name, "worker-") {
		t.Fatalf("different-parent collision merged: %+v", w3)
	}
	// The holder's own session: refreshed.
	self := registerDistinctTDM22(t, s, c.ID, &store.Agent{Name: "worker", Role: "executor", ParentAgentID: &p1.ID},
		store.AgentRegisterIntent{SessionIdentity: "worker"})
	if self.ID != w1.ID {
		t.Fatalf("holder's own re-register moved: %+v -> %+v", w1, self)
	}
	// Plain named registration (no parent) is unchanged: refresh.
	plain := registerDistinctTDM22(t, s, c.ID, &store.Agent{Name: "worker", Role: "executor"},
		store.AgentRegisterIntent{SessionIdentity: "session-9"})
	if plain.ID != w1.ID || plain.ParentAgentID == nil || *plain.ParentAgentID != p1.ID {
		t.Fatalf("plain named re-register changed: %+v", plain)
	}
	// Stale holder under the same parent: reused.
	if _, err := s.db.ExecContext(ctx, `UPDATE agents SET last_seen_at = ? WHERE id = ?`,
		fmtTime(time.Now().Add(-store.DefaultClaimTTL-time.Minute)), w1.ID.String()); err != nil {
		t.Fatal(err)
	}
	back := registerDistinctTDM22(t, s, c.ID, &store.Agent{Name: "worker", Role: "executor", ParentAgentID: &p1.ID},
		store.AgentRegisterIntent{SessionIdentity: "session-4"})
	if back.ID != w1.ID {
		t.Fatalf("stale same-parent holder not reused: %+v -> %+v", w1, back)
	}
}
