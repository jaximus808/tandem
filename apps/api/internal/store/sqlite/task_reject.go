package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/agentcanvas/api/internal/store"
	"github.com/google/uuid"
)

// RejectTask is the conditional reject of ONE task (TDM-14): `from` → rejected,
// predicated on (state = from, type = 'task') inside the store's serialized
// transaction, so a task an agent claimed between the caller's read and this
// write is NOT dragged out of 'executing' — the predicate misses and the re-read
// reports the state it is really in.
//
// `from` is the caller's validated source state ('proposed' or 'approved' —
// see api.rejectableFrom); this method must never be reachable with an
// arbitrary string from a request body.
//
// The state audit entry rides the SAME write, appended to the stored audit log
// via store.AppendAudit. That matters more here than on most moves: the entry's
// fromState is the only durable record of where the task was rejected FROM, and
// the board's undo (rejected → back where it was) reads it. A best-effort entry
// written after the transition could be lost and silently turn "undo" into
// "send an approved task back through the gate".
//
// The rejection reason lands in `error` (cleared when empty, so an approved task
// carrying a stale error from an earlier life does not advertise it as the
// verdict). The claim columns (claimed_by, claimed_by_agent_id, claimed_at) are
// cleared too — an approved task should hold no claim, and a rejected one
// certainly must not. approved_by is left alone: it is
// what lets the undo restore the approval as it was.
//
// Returns the fresh row and the bumped version; an already-rejected task is the
// idempotent retry (existing row, version 0). Anything else is
// ErrIllegalActionState naming the current state, or ErrActionNotFound.
func (s *Store) RejectTask(ctx context.Context, canvasID, id uuid.UUID, from string, reason *string, entry store.ContentAudit) (*store.Action, int, error) {
	var (
		a *store.Action
		v int
	)
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		current, err := getAction(ctx, tx, canvasID, id)
		if err != nil {
			return store.ErrActionNotFound
		}
		if current.Type != "task" {
			return fmt.Errorf("%w: only tasks can be rejected this way (this is a %q)", store.ErrIllegalActionState, current.Type)
		}
		if current.State == "rejected" {
			a, v = current, 0
			return nil
		}
		if current.State != from {
			return fmt.Errorf("%w: cannot reject task from %q (it is %q now)", store.ErrIllegalActionState, from, current.State)
		}
		payload, err := store.AppendAudit(current.Payload, entry)
		if err != nil {
			return err
		}
		var errCol any
		if reason != nil && *reason != "" {
			errCol = *reason
		}
		res, err := tx.ExecContext(ctx, `UPDATE actions SET state = 'rejected', error = ?, payload = ?,
			claimed_by = NULL, claimed_by_agent_id = NULL, claimed_at = NULL, updated_at = ?
			WHERE id = ? AND canvas_id = ? AND state = ? AND type = 'task'`,
			errCol, string(payload), nowStr(), id.String(), canvasID.String(), from)
		if err != nil {
			return err
		}
		if affected(res) != 1 {
			// Unreachable inside one serialized transaction after the read above,
			// and guarded anyway: a miss is a lost race, never a lost update.
			return fmt.Errorf("%w: cannot reject task from %q (it moved during the write)", store.ErrIllegalActionState, from)
		}
		if a, err = getAction(ctx, tx, canvasID, id); err != nil {
			return err
		}
		v, err = s.bumpVersion(ctx, tx, canvasID)
		return err
	})
	if err != nil {
		return nil, 0, err
	}
	return a, v, nil
}
