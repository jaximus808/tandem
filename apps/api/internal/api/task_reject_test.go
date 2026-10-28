package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/agentcanvas/api/internal/store"
	"github.com/google/uuid"
)

// TDM-14 — rejecting APPROVED (unclaimed) work, the batch reject endpoint, and
// the undo that puts a rejected task back where it was.

// tdm14RejectStore is the move fake plus the conditional reject, modelled the
// way the SQLite store does it: predicated on `from`, audit entry in the same
// write (via the real store.AppendAudit), approved_by left alone.
type tdm14RejectStore struct {
	*moveFakeStore
}

func newTDM14Store(actions ...*store.Action) *tdm14RejectStore {
	return &tdm14RejectStore{newMoveStore(actions...)}
}

func (f *tdm14RejectStore) RejectTask(_ context.Context, _ uuid.UUID, id uuid.UUID, from string, reason *string, entry store.ContentAudit) (*store.Action, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.actions[id]
	if !ok {
		return nil, 0, store.ErrActionNotFound
	}
	if a.State == "rejected" {
		cp := *a
		return &cp, 0, nil
	}
	if a.Type != "task" || a.State != from {
		return nil, 0, fmt.Errorf("%w: cannot reject task from %q (it is %q now)", store.ErrIllegalActionState, from, a.State)
	}
	next, err := store.AppendAudit(a.Payload, entry)
	if err != nil {
		return nil, 0, err
	}
	a.State, a.Payload, a.Error, a.ClaimedBy, a.ClaimedByAgentID, a.ClaimedAt = "rejected", next, reason, nil, nil, nil
	cp := *a
	return &cp, 1, nil
}

func tdm14Task(state string) *store.Action {
	a := humanTask(state)
	a.Payload = json.RawMessage(`{"title":"Ship the thing","assignee":"agent"}`)
	if state == "proposed" {
		a.ApprovedBy = nil
	}
	if state == "executing" {
		a.ClaimedBy = ptr("worker-7")
	}
	return a
}

func tdm14Reject(t *testing.T, h *Handler, canvasID uuid.UUID, id uuid.UUID, reason string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	h.RejectAction(w, canvasRequest(t, "POST", "/api/canvas/actions/"+id.String()+"/reject",
		map[string]any{"reason": reason}, canvasID, id.String()))
	return w
}

func TestTDM14RejectApprovedTask(t *testing.T) {
	canvasID := uuid.New()
	task := tdm14Task("approved")
	f := newTDM14Store(task)
	h := NewHandler(f, nil)

	w := tdm14Reject(t, h, canvasID, task.ID, "wrong priority")
	if w.Code != http.StatusOK {
		t.Fatalf("reject approved: %d, want 200 (body %s)", w.Code, w.Body)
	}
	stored := f.get(t, task.ID)
	if stored.State != "rejected" {
		t.Fatalf("state %q, want rejected", stored.State)
	}
	if stored.Error == nil || *stored.Error != "wrong priority" {
		t.Fatalf("reason not stored: %v", stored.Error)
	}
	if stored.ApprovedBy == nil {
		t.Fatalf("approved_by was cleared; the undo needs it")
	}
	e := lastAudit(t, stored)
	if e.FromState != "approved" || e.ToState != "rejected" || e.Actor != AuthorHuman {
		t.Fatalf("audit entry %+v, want approved → rejected by human", e)
	}
	var body struct {
		Action store.Action `json:"action"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Action.State != "rejected" {
		t.Fatalf("response %s: %v", w.Body, err)
	}
}

func TestTDM14RejectRefusesWorkInFlightOrFinished(t *testing.T) {
	for _, tc := range []struct {
		state, code string
	}{
		{"executing", "reject_claimed"},
		{"done", "reject_finished"},
		{"failed", "reject_finished"},
	} {
		t.Run(tc.state, func(t *testing.T) {
			canvasID := uuid.New()
			task := tdm14Task(tc.state)
			f := newTDM14Store(task)
			h := NewHandler(f, nil)

			w := tdm14Reject(t, h, canvasID, task.ID, "nope")
			if w.Code != http.StatusConflict {
				t.Fatalf("reject %s: %d, want 409 (body %s)", tc.state, w.Code, w.Body)
			}
			var body map[string]string
			_ = json.Unmarshal(w.Body.Bytes(), &body)
			if body["error"] != tc.code || body["state"] != tc.state {
				t.Fatalf("body %v, want error=%s state=%s", body, tc.code, tc.state)
			}
			if tc.state == "executing" && body["claimedBy"] != "worker-7" {
				t.Fatalf("refusal should name the holder: %v", body)
			}
			if got := f.get(t, task.ID).State; got != tc.state {
				t.Fatalf("refused reject moved the task to %q", got)
			}
		})
	}
}

func TestTDM14RejectStillHumanOnlyForApproved(t *testing.T) {
	canvasID := uuid.New()
	task := tdm14Task("approved")
	f := newTDM14Store(task)
	h := NewHandler(f, nil)
	for _, tc := range nonHumanAuthors {
		w := httptest.NewRecorder()
		h.RejectAction(w, authoredRequest(t, "POST", "/api/canvas/actions/"+task.ID.String()+"/reject",
			map[string]any{}, canvasID, task.ID.String(), tc.author))
		if w.Code != http.StatusForbidden {
			t.Fatalf("reject by %s = %d, want 403", tc.name, w.Code)
		}
		w = httptest.NewRecorder()
		h.RejectActionsBatch(w, authoredRequest(t, "POST", "/api/canvas/actions/reject-batch",
			map[string]any{"ids": []string{task.ID.String()}}, canvasID, "", tc.author))
		if w.Code != http.StatusForbidden {
			t.Fatalf("reject-batch by %s = %d, want 403", tc.name, w.Code)
		}
	}
	if got := f.get(t, task.ID).State; got != "approved" {
		t.Fatalf("non-human reject moved the task to %q", got)
	}
}

func TestTDM14RejectBatchMixedOutcomes(t *testing.T) {
	canvasID := uuid.New()
	proposed := tdm14Task("proposed")
	approved := tdm14Task("approved")
	executing := tdm14Task("executing")
	done := tdm14Task("done")
	already := tdm14Task("rejected")
	epic := &store.Action{ID: uuid.New(), Type: "epic", State: "approved", Payload: json.RawMessage(`{"title":"E"}`)}
	f := newTDM14Store(proposed, approved, executing, done, already, epic)
	h := NewHandler(f, nil)

	missing := uuid.New()
	ids := []string{
		proposed.ID.String(), approved.ID.String(), executing.ID.String(), done.ID.String(),
		already.ID.String(), epic.ID.String(), missing.String(), "not-a-uuid",
		approved.ID.String(), // duplicate: reported once
	}
	w := httptest.NewRecorder()
	h.RejectActionsBatch(w, canvasRequest(t, "POST", "/api/canvas/actions/reject-batch",
		map[string]any{"ids": ids, "reason": "descoped"}, canvasID, ""))
	if w.Code != http.StatusOK {
		t.Fatalf("reject-batch: %d (body %s)", w.Code, w.Body)
	}
	var body struct {
		Rejected []string            `json:"rejected"`
		Results  []rejectBatchResult `json:"results"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Rejected) != 2 || body.Rejected[0] != proposed.ID.String() || body.Rejected[1] != approved.ID.String() {
		t.Fatalf("rejected = %v, want [proposed, approved]", body.Rejected)
	}
	want := []struct{ outcome, errCode, from, state string }{
		{"rejected", "", "proposed", "rejected"},
		{"rejected", "", "approved", "rejected"},
		{"refused", "reject_claimed", "", "executing"},
		{"refused", "reject_finished", "", "done"},
		{"unchanged", "", "", "rejected"},
		{"refused", "reject_not_task", "", "approved"},
		{"refused", "not_found", "", ""},
		{"refused", "invalid_id", "", ""},
	}
	if len(body.Results) != len(want) {
		t.Fatalf("got %d results, want %d: %+v", len(body.Results), len(want), body.Results)
	}
	for i, wr := range want {
		got := body.Results[i]
		if got.Outcome != wr.outcome || got.Error != wr.errCode || got.From != wr.from || got.State != wr.state {
			t.Fatalf("result %d = %+v, want %+v", i, got, wr)
		}
	}
	if body.Results[2].ClaimedBy != "worker-7" {
		t.Fatalf("claimed refusal should name the holder: %+v", body.Results[2])
	}
	// State on disk matches the report.
	for id, st := range map[uuid.UUID]string{
		proposed.ID: "rejected", approved.ID: "rejected", executing.ID: "executing",
		done.ID: "done", already.ID: "rejected", epic.ID: "approved",
	} {
		if got := f.get(t, id).State; got != st {
			t.Fatalf("%s state %q, want %q", id, got, st)
		}
	}
	if e := lastAudit(t, f.get(t, approved.ID)); e.FromState != "approved" || e.Note != "descoped" {
		t.Fatalf("batch audit entry %+v", e)
	}
}

func TestTDM14RejectBatchValidation(t *testing.T) {
	h := NewHandler(newTDM14Store(), nil)
	w := httptest.NewRecorder()
	h.RejectActionsBatch(w, canvasRequest(t, "POST", "/api/canvas/actions/reject-batch",
		map[string]any{"ids": []string{}}, uuid.New(), ""))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("empty ids: %d, want 400", w.Code)
	}
}

// The undo restores the PRIOR state: approved-then-rejected goes back to
// 'approved' (approval intact), proposed-then-rejected back to 'proposed'.
func TestTDM14UndoRestoresPriorState(t *testing.T) {
	for _, tc := range []struct{ prior, undoTo string }{
		{"approved", "proposed"}, // the board's Undo sends to:"proposed"
		{"approved", "approved"}, // naming the state works too
		{"proposed", "proposed"},
	} {
		t.Run(tc.prior+"_to_"+tc.undoTo, func(t *testing.T) {
			canvasID := uuid.New()
			task := tdm14Task(tc.prior)
			f := newTDM14Store(task)
			h := NewHandler(f, nil)

			if w := tdm14Reject(t, h, canvasID, task.ID, "oops"); w.Code != http.StatusOK {
				t.Fatalf("reject: %d %s", w.Code, w.Body)
			}
			w := httptest.NewRecorder()
			h.MoveAction(w, moveRequest(t, canvasID, task.ID, map[string]any{"to": tc.undoTo}))
			if w.Code != http.StatusOK {
				t.Fatalf("undo: %d %s", w.Code, w.Body)
			}
			stored := f.get(t, task.ID)
			if stored.State != tc.prior {
				t.Fatalf("undo landed in %q, want %q", stored.State, tc.prior)
			}
			if tc.prior == "approved" && stored.ApprovedBy == nil {
				t.Fatalf("restored task lost its approval stamp")
			}
			if stored.Error != nil {
				t.Fatalf("restored task still carries the rejection reason: %q", *stored.Error)
			}
			e := lastAudit(t, stored)
			if e.FromState != "rejected" || e.ToState != tc.prior {
				t.Fatalf("undo audit %+v", e)
			}
		})
	}
}

// to:"approved" on a task that was rejected as a PROPOSAL is approval by the
// back door, and stays refused.
func TestTDM14UndoCannotApproveAProposal(t *testing.T) {
	canvasID := uuid.New()
	task := tdm14Task("proposed")
	f := newTDM14Store(task)
	h := NewHandler(f, nil)
	if w := tdm14Reject(t, h, canvasID, task.ID, ""); w.Code != http.StatusOK {
		t.Fatalf("reject: %d %s", w.Code, w.Body)
	}
	w := httptest.NewRecorder()
	h.MoveAction(w, moveRequest(t, canvasID, task.ID, map[string]any{"to": "approved"}))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("rejected proposal → approved: %d, want 400 (body %s)", w.Code, w.Body)
	}
	if got := f.get(t, task.ID).State; got != "rejected" {
		t.Fatalf("state %q, want rejected", got)
	}
}

func TestTDM14RejectedFromReadsTheNewestMove(t *testing.T) {
	p := json.RawMessage(`{"title":"x"}`)
	add := func(from, to string) {
		var err error
		p, err = store.AppendAudit(p, store.NewStateAudit("human", from, to, "", time.Now()))
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := store.RejectedFrom(p); got != "" {
		t.Fatalf("no trail: %q", got)
	}
	add("approved", "rejected")
	if got := store.RejectedFrom(p); got != "approved" {
		t.Fatalf("got %q, want approved", got)
	}
	add("rejected", "approved")
	add("approved", "proposed")
	// The newest move is not a rejection: the old rejection's origin must not leak.
	if got := store.RejectedFrom(p); got != "" {
		t.Fatalf("stale origin leaked: %q", got)
	}
	add("proposed", "rejected")
	if got := store.RejectedFrom(p); got != "proposed" {
		t.Fatalf("got %q, want proposed", got)
	}
}
