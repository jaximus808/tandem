package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/agentcanvas/api/internal/store"
	"github.com/agentcanvas/api/internal/webhooks"
	"github.com/google/uuid"
)

// ── Rejecting APPROVED work, and rejecting in bulk (TDM-14) ───────────────────
//
// Until this existed, reject was the other exit from the approval gate and
// nothing else: proposed → rejected. Once a batch was approved, pulling a chunk
// of it back meant un-approving and then rejecting each ticket, one at a time.
//
// THE NEW DOOR is approved → rejected, for a task NOBODY HOLDS. 'approved' is
// the ready queue, and a task in it is by construction unclaimed (a claim is
// what moves it to 'executing'), so the rule is simply the state — enforced as
// a predicate on the write itself (store RejectTask), so an agent that claims
// the task between our read and our write wins and the reject is refused,
// rather than dragging live work out from under it.
//
// WHAT STAYS REFUSED, with a coded error each, because each needs a different
// next step from the person:
//
//	executing  → reject_claimed   someone is working it: release it first
//	done/failed→ reject_finished  it already ran: reopen / re-queue instead
//
// HUMAN-ONLY, exactly like rejecting a proposal (RejectAction's gate): killing
// work is a person's decision on every approval policy, 'peer' included.
//
// AND UNDO PUTS IT BACK WHERE IT WAS. The board's undo is POST …/move
// {to:"proposed"} — the reconsider rewind. For a task rejected out of 'approved'
// that would send already-approved work back through the gate, so the move
// resolves its real target off the audit trail (store.RejectedFrom) and restores
// 'approved' instead, approved_by intact. See resolveRejectedRewind.

// taskRejectStore is the conditional reject, asserted as an OPTIONAL capability
// rather than added to store.Store — the contention.go pattern: a test double
// that does not implement it degrades to a refusal instead of nil-dereferencing
// through its embedded interface. The production SQLite store implements it.
type taskRejectStore interface {
	RejectTask(ctx context.Context, canvasID, id uuid.UUID, from string, reason *string, entry store.ContentAudit) (*store.Action, int, error)
}

// moveRestore is the undo of a rejection that came out of 'approved': rejected
// → approved, through store.ReopenAction (which keeps approved_by on a rewind
// that does not go back into the gate). Deliberately NOT in humanMoveTargets:
// the board cannot ask for it by name — it is reached only by resolving a
// reconsider (or an explicit to:"approved") against the trail, so it can never
// become a second door into the ready queue for work that was never approved.
var moveRestore = humanMove{"rejected", "approved", moveKindRewind, activityRequeued, "undo reject"}

// resolveRejectedRewind maps a rewind requested on a REJECTED task to the move
// that actually undoes the rejection. ok=false means "not my case, use the
// matrix as usual".
//
//	rejected out of approved, to:"proposed" or to:"approved" → moveRestore
//	anything else                                            → the matrix
//
// An unknown origin (no audit entry) resolves to the matrix, i.e. into the gate
// — the safe direction. to:"approved" on a task that was rejected as a PROPOSAL
// falls through to the matrix too, which refuses it: that would be approval by
// the back door.
func resolveRejectedRewind(current *store.Action, to string) (humanMove, bool) {
	if current == nil || current.Type != "task" || current.State != "rejected" {
		return humanMove{}, false
	}
	if to != "proposed" && to != "approved" {
		return humanMove{}, false
	}
	if store.RejectedFrom(current.Payload) != "approved" {
		return humanMove{}, false
	}
	return moveRestore, true
}

// rejectRefusal is why one task could not be rejected: an HTTP status for the
// single endpoint, plus the coded error both endpoints report.
type rejectRefusal struct {
	status    int
	code      string
	message   string
	state     string
	claimedBy string
}

// rejectRefusalFor says why `a` cannot be rejected from its current state, or
// nil when it can (proposed or approved). Pure: the store's predicate is what
// actually decides under concurrency; this is the explanation.
func rejectRefusalFor(a *store.Action) *rejectRefusal {
	switch a.State {
	case "proposed", "approved", "rejected":
		return nil
	case "executing":
		ref := &rejectRefusal{
			status: http.StatusConflict, code: "reject_claimed", state: a.State,
			message: "this task is being worked — release its claim first, then reject it",
		}
		if a.ClaimedBy != nil {
			ref.claimedBy = *a.ClaimedBy
			ref.message = fmt.Sprintf("this task is claimed by %q and being worked — release its claim first, then reject it", *a.ClaimedBy)
		}
		return ref
	case "done", "failed":
		return &rejectRefusal{
			status: http.StatusConflict, code: "reject_finished", state: a.State,
			message: fmt.Sprintf("this task already ran (%s) — reopen or re-queue it instead of rejecting it", a.State),
		}
	}
	return &rejectRefusal{
		status: http.StatusBadRequest, code: "reject_illegal_state", state: a.State,
		message: fmt.Sprintf("a task in %q cannot be rejected", a.State),
	}
}

func writeRejectRefusal(w http.ResponseWriter, ref *rejectRefusal) {
	extra := map[string]string{"state": ref.state}
	if ref.claimedBy != "" {
		extra["claimedBy"] = ref.claimedBy
	}
	writeCodedError(w, ref.status, ref.code, ref.message, extra)
}

// rejectTaskTo runs one conditional reject and its follow-ups (event, queue_wait
// wake, fleet activity). moved=false on the idempotent already-rejected retry,
// which emits nothing. The caller broadcasts state (once, for a batch).
func (h *Handler) rejectTaskTo(ctx context.Context, st taskRejectStore, canvasID uuid.UUID, current *store.Action, reason, actor string) (*store.Action, bool, error) {
	var reasonPtr *string
	if reason != "" {
		reasonPtr = &reason
	}
	from := current.State
	entry := store.NewStateAudit(actor, from, "rejected", reason, time.Now().UTC())
	fresh, version, err := st.RejectTask(ctx, canvasID, current.ID, from, reasonPtr, entry)
	if err != nil {
		return nil, false, err
	}
	if version == 0 {
		// The store's idempotent answer: somebody rejected it first. Nothing moved
		// in this call, so nothing is announced twice.
		return fresh, false, nil
	}
	h.emitTaskEvent(canvasID, webhooks.EventTaskRejected, fresh, withRejected(from, actor, reason))
	broadcastActionActivityAs(h.hub, canvasID, activityRejected, fresh, actor)
	return fresh, true, nil
}

// rejectApprovedTask is RejectAction's branch for a task in the ready queue.
// Same response shape as the proposal reject ({"action": …}).
func (h *Handler) rejectApprovedTask(w http.ResponseWriter, r *http.Request, current *store.Action, reason string) {
	st, ok := h.store.(taskRejectStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, "this store cannot reject approved tasks")
		return
	}
	canvasID := CanvasIDFromCtx(r.Context())
	actor := orString(moveActor(r), AuthorHuman)
	fresh, _, err := h.rejectTaskTo(r.Context(), st, canvasID, current, reason, actor)
	if err != nil {
		h.writeRejectStoreError(w, r, canvasID, current.ID, err)
		return
	}
	broadcastStateAsync(r.Context(), h.store, h.hub, canvasID)
	writeJSON(w, http.StatusOK, map[string]any{"action": fresh})
}

// writeRejectStoreError maps a RejectTask failure. A lost race (the task was
// claimed between the read and the write) is re-explained off a fresh read, so
// the person is told "someone claimed it" rather than a raw state error.
func (h *Handler) writeRejectStoreError(w http.ResponseWriter, r *http.Request, canvasID, id uuid.UUID, err error) {
	switch {
	case errors.Is(err, store.ErrActionNotFound):
		writeError(w, http.StatusNotFound, "action not found")
	case errors.Is(err, store.ErrIllegalActionState):
		if now, gerr := h.store.GetAction(r.Context(), canvasID, id); gerr == nil {
			if ref := rejectRefusalFor(now); ref != nil {
				writeRejectRefusal(w, ref)
				return
			}
		}
		writeError(w, http.StatusConflict, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

// rejectBatchResult is one id's outcome in POST …/actions/reject-batch.
type rejectBatchResult struct {
	ID       string `json:"id"`
	TicketID string `json:"ticketId,omitempty"`
	// Outcome: "rejected" (moved in this call), "unchanged" (already rejected —
	// a retry), or "refused" (see Error).
	Outcome string `json:"outcome"`
	// From is the state the task was rejected out of, on "rejected".
	From string `json:"from,omitempty"`
	// State is the task's state after this call (absent when it was not found).
	State string `json:"state,omitempty"`
	// Error / Message, on "refused": invalid_id, not_found, reject_not_task,
	// reject_claimed, reject_finished, reject_illegal_state, or reject_failed.
	Error     string `json:"error,omitempty"`
	Message   string `json:"message,omitempty"`
	ClaimedBy string `json:"claimedBy,omitempty"`
}

// POST /api/canvas/actions/reject-batch — reject many tasks in ONE request.
//
//	{ "ids": ["<uuid>", …], "reason": "optional, shared by every task" }
//
// Each id is judged on its own: proposed and approved tasks are rejected (the
// conditional write per task, so a task claimed mid-batch is refused, not
// dragged out of 'executing'), everything else is reported back with a coded
// reason. 200 whenever the request itself was well-formed, even if every id was
// refused — the per-id `results` are the answer:
//
//	{ "rejected": ["<uuid>", …],             // moved to 'rejected' by THIS call
//	  "results":  [ { "id", "ticketId", "outcome", "from", "state",
//	                  "error", "message", "claimedBy" }, … ] }   // input order, deduped
//
// Human-only, like every reject. Tasks only — an epic is rejected through its
// own …/{id}/reject (or the owner's epic move), never swept up in a selection.
func (h *Handler) RejectActionsBatch(w http.ResponseWriter, r *http.Request) {
	if !callerIsHuman(r.Context()) {
		writeError(w, http.StatusForbidden, "only a signed-in human can reject an action")
		return
	}
	var body struct {
		IDs    []string `json:"ids"`
		Reason string   `json:"reason"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(body.IDs) == 0 {
		writeError(w, http.StatusBadRequest, "ids: at least one task id is required")
		return
	}
	if tooManyBatchItems(w, len(body.IDs)) {
		return
	}
	reason := strings.TrimSpace(body.Reason)
	if !checkMoveNote(w, reason) {
		return
	}
	st, ok := h.store.(taskRejectStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, "this store cannot reject tasks in bulk")
		return
	}
	canvasID := CanvasIDFromCtx(r.Context())
	actor := orString(moveActor(r), AuthorHuman)

	rejected := make([]string, 0, len(body.IDs))
	results := make([]rejectBatchResult, 0, len(body.IDs))
	seen := make(map[string]bool, len(body.IDs))
	for _, raw := range body.IDs {
		id, err := uuid.Parse(strings.TrimSpace(raw))
		if err != nil {
			results = append(results, rejectBatchResult{ID: raw, Outcome: "refused",
				Error: "invalid_id", Message: "not a task uuid"})
			continue
		}
		key := id.String()
		if seen[key] {
			continue
		}
		seen[key] = true
		res := h.rejectOneInBatch(r.Context(), st, canvasID, id, reason, actor)
		if res.Outcome == "rejected" {
			rejected = append(rejected, key)
		}
		results = append(results, res)
	}
	if len(rejected) > 0 {
		broadcastStateAsync(r.Context(), h.store, h.hub, canvasID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"rejected": rejected, "results": results})
}

func (h *Handler) rejectOneInBatch(ctx context.Context, st taskRejectStore, canvasID, id uuid.UUID, reason, actor string) rejectBatchResult {
	res := rejectBatchResult{ID: id.String()}
	current, err := h.store.GetAction(ctx, canvasID, id)
	if err != nil {
		res.Outcome, res.Error, res.Message = "refused", "not_found", "no such task on this canvas"
		return res
	}
	if current.Ticket != nil {
		res.TicketID = store.TicketID(*current.Ticket)
	}
	res.State = current.State
	if current.Type != "task" {
		res.Outcome, res.Error = "refused", "reject_not_task"
		res.Message = fmt.Sprintf("only tasks can be bulk-rejected (this is a %q)", current.Type)
		return res
	}
	if current.State == "rejected" {
		res.Outcome = "unchanged"
		return res
	}
	if ref := rejectRefusalFor(current); ref != nil {
		res.Outcome, res.Error, res.Message, res.ClaimedBy = "refused", ref.code, ref.message, ref.claimedBy
		return res
	}
	fresh, _, err := h.rejectTaskTo(ctx, st, canvasID, current, reason, actor)
	if err != nil {
		res.Outcome = "refused"
		if errors.Is(err, store.ErrIllegalActionState) {
			// Lost a race: explain off a fresh read (most likely: claimed mid-batch).
			if now, gerr := h.store.GetAction(ctx, canvasID, id); gerr == nil {
				res.State = now.State
				if ref := rejectRefusalFor(now); ref != nil {
					res.Error, res.Message, res.ClaimedBy = ref.code, ref.message, ref.claimedBy
					return res
				}
			}
			res.Error, res.Message = "reject_illegal_state", err.Error()
			return res
		}
		res.Error, res.Message = "reject_failed", err.Error()
		return res
	}
	res.Outcome, res.From, res.State = "rejected", current.State, fresh.State
	return res
}
