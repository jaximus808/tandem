package api

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/agentcanvas/api/internal/store"
)

// TDM-16: the roster attributes a claim by the agent id it stored, before
// falling back to the claimedBy name, and one agent's several claims all land
// on it.
func TestTDM16RosterPrefersStoredAgentID(t *testing.T) {
	now := time.Now().UTC()
	worker := &store.Agent{ID: uuid.New(), Name: "executor", Role: "executor", Status: "online"}
	other := &store.Agent{ID: uuid.New(), Name: "opus-worker", Role: "executor", Status: "online"}
	workerID := worker.ID.String()
	task := func(claimedBy string, agentID *string) *store.Action {
		return &store.Action{ID: uuid.New(), Type: "task", State: "executing",
			Payload: []byte(`{"title":"t"}`), ClaimedBy: &claimedBy, ClaimedByAgentID: agentID, ClaimedAt: &now}
	}
	executing := []*store.Action{
		// The stored id is the authority: it wins over a name that points at a
		// different agent.
		task("opus-worker", &workerID),
		task("executor", &workerID),
		// Legacy row with no stored id: name fallback.
		task("opus-worker", nil),
	}
	got := buildRoster([]*store.Agent{worker, other}, executing, nil, 0, now)
	tasks := map[string]int{}
	for _, a := range got.Agents {
		tasks[a.Name] = len(a.Tasks)
	}
	if len(got.Agents) != 2 || tasks["executor"] != 2 || tasks["opus-worker"] != 1 {
		t.Fatalf("roster tasks by agent = %v (%d agents), want executor:2 opus-worker:1", tasks, len(got.Agents))
	}
}
