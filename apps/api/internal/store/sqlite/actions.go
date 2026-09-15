package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/agentcanvas/api/internal/store"
)

const actionCols = `id, type, state, payload, proposed_by, approved_by, claimed_by, claimed_at, result, error, ticket, authored_by, created_at, updated_at`

// actionRow is one actions row as read; claimedAtRaw keeps the exact stored
// text so a follow-up predicate can match it byte for byte.
type actionRow struct {
	action       *store.Action
	claimedAtRaw sql.NullString
}

func scanActionRow(r rowScanner) (*actionRow, error) {
	var (
		a                                                             store.Action
		id, payload, createdAt, updatedAt                             string
		approvedBy, claimedBy, claimedAt, result, errText, authoredBy sql.NullString
		ticket                                                        sql.NullInt64
	)
	if err := r.Scan(&id, &a.Type, &a.State, &payload, &a.ProposedBy, &approvedBy, &claimedBy, &claimedAt, &result, &errText, &ticket, &authoredBy, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	a.ID = mustUUID(id)
	a.Kind = "action"
	a.Payload = json.RawMessage(payload)
	if len(a.Payload) == 0 {
		a.Payload = json.RawMessage("{}")
	}
	a.ApprovedBy = strPtr(approvedBy)
	a.ClaimedBy = strPtr(claimedBy)
	a.ClaimedAt = parseTimePtr(claimedAt)
	a.Result = strPtr(result)
	a.Error = strPtr(errText)
	a.Ticket = intPtr(ticket)
	a.AuthoredBy = strPtr(authoredBy)
	a.CreatedAt = parseTime(createdAt)
	a.UpdatedAt = parseTime(updatedAt)
	return &actionRow{action: &a, claimedAtRaw: claimedAt}, nil
}

func scanAction(r rowScanner) (*store.Action, error) {
	row, err := scanActionRow(r)
	if err != nil {
		return nil, err
	}
	return row.action, nil
}

func getActionRow(ctx context.Context, q dbtx, canvasID, id uuid.UUID) (*actionRow, error) {
	row, err := scanActionRow(q.QueryRowContext(ctx, `SELECT `+actionCols+` FROM actions WHERE id = ? AND canvas_id = ?`, id.String(), canvasID.String()))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("action %s not found in canvas %s", id, canvasID)
	}
	return row, err
}

func getAction(ctx context.Context, q dbtx, canvasID, id uuid.UUID) (*store.Action, error) {
	row, err := getActionRow(ctx, q, canvasID, id)
	if err != nil {
		return nil, err
	}
	return row.action, nil
}

func listActions(ctx context.Context, q dbtx, canvasID uuid.UUID, stateFilter, typeFilter, assigneeFilter string) ([]*store.Action, error) {
	query := `SELECT ` + actionCols + ` FROM actions WHERE canvas_id = ?`
	args := []any{canvasID.String()}
	if stateFilter != "" {
		query += ` AND state = ?`
		args = append(args, stateFilter)
	}
	if typeFilter != "" {
		query += ` AND type = ?`
		args = append(args, typeFilter)
	}
	switch assigneeFilter {
	case "":
	case "agent":
		// Tasks created before the assignee field existed are agent tasks.
		query += ` AND (json_extract(payload, '$.assignee') IS NULL OR json_extract(payload, '$.assignee') = 'agent')`
	default:
		query += ` AND json_extract(payload, '$.assignee') = ?`
		args = append(args, assigneeFilter)
	}
	query += ` ORDER BY created_at, id`
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return scanActions(rows)
}

func scanActions(rows *sql.Rows) ([]*store.Action, error) {
	defer rows.Close()
	out := []*store.Action{}
	for rows.Next() {
		a, err := scanAction(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func insertAction(ctx context.Context, q dbtx, canvasID uuid.UUID, a *store.Action, now string) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	if len(a.Payload) == 0 {
		a.Payload = json.RawMessage("{}")
	}
	if a.State == "" {
		a.State = "proposed"
	}
	if a.Type == "" {
		a.Type = "navigate"
	}
	if a.ProposedBy == "" {
		a.ProposedBy = "agent"
	}
	_, err := q.ExecContext(ctx, `INSERT INTO actions
		(id, canvas_id, type, state, payload, proposed_by, approved_by, ticket, authored_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID.String(), canvasID.String(), a.Type, a.State, string(a.Payload), a.ProposedBy, strArg(a.ApprovedBy),
		intArg(a.Ticket), strArg(a.AuthoredBy), now, now)
	return err
}

// ReserveTaskTickets is the reserve_task_tickets RPC (migration 0034) as one
// statement: a single UPDATE on the canvas row, so concurrent callers can never
// receive overlapping ranges. Returns the first ticket of the range.
func (s *Store) ReserveTaskTickets(ctx context.Context, canvasID uuid.UUID, n int) (int, error) {
	if n < 1 {
		return 0, fmt.Errorf("reserveTaskTickets: n must be >= 1 (got %d)", n)
	}
	var first int
	err := s.db.QueryRowContext(ctx, `UPDATE canvases SET next_ticket = next_ticket + ?1 WHERE id = ?2 RETURNING next_ticket - ?1`,
		n, canvasID.String()).Scan(&first)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("reserveTaskTickets: canvas not found: %s", canvasID)
	}
	return first, err
}

func (s *Store) CreateAction(ctx context.Context, canvasID uuid.UUID, a *store.Action) (int, error) {
	return s.CreateActions(ctx, canvasID, []*store.Action{a})
}

// CreateActions inserts many actions in ONE transaction with ONE bump.
func (s *Store) CreateActions(ctx context.Context, canvasID uuid.UUID, actions []*store.Action) (int, error) {
	if len(actions) == 0 {
		return 0, nil
	}
	v := 0
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		now := nowStr()
		t := parseTime(now)
		for _, a := range actions {
			a.CreatedAt, a.UpdatedAt = t, t
			if err := insertAction(ctx, tx, canvasID, a, now); err != nil {
				return err
			}
		}
		var err error
		v, err = s.bumpVersion(ctx, tx, canvasID)
		return err
	})
	return v, err
}

func (s *Store) GetAction(ctx context.Context, canvasID, id uuid.UUID) (*store.Action, error) {
	return getAction(ctx, s.db, canvasID, id)
}

func (s *Store) GetActionByTicket(ctx context.Context, canvasID uuid.UUID, ticket int) (*store.Action, error) {
	a, err := scanAction(s.db.QueryRowContext(ctx, `SELECT `+actionCols+` FROM actions WHERE canvas_id = ? AND ticket = ?`, canvasID.String(), ticket))
	if errors.Is(err, sql.ErrNoRows) {
		// Wrapped so the ticket-ref middleware can answer 404 for "no such
		// ticket" and NOT for "the lookup failed" (TDM-95).
		return nil, fmt.Errorf("no task %s in canvas %s: %w", store.TicketID(ticket), canvasID, store.ErrActionNotFound)
	}
	return a, err
}

func (s *Store) ListActions(ctx context.Context, canvasID uuid.UUID, stateFilter, typeFilter, assigneeFilter string) ([]*store.Action, error) {
	return listActions(ctx, s.db, canvasID, stateFilter, typeFilter, assigneeFilter)
}

// UpdateActionState applies a single transition; the caller validates it.
func (s *Store) UpdateActionState(ctx context.Context, canvasID, id uuid.UUID, patch store.ActionStatePatch) (int, error) {
	var c setClause
	c.add("state", patch.State)
	if patch.Result != nil {
		c.add("result", *patch.Result)
	}
	if patch.Error != nil {
		c.add("error", *patch.Error)
	}
	if patch.ApprovedBy != nil {
		c.add("approved_by", *patch.ApprovedBy)
	}
	if len(patch.Payload) > 0 {
		c.add("payload", string(patch.Payload))
	}
	c.add("updated_at", nowStr())
	return s.execThenBump(ctx, canvasID, `UPDATE actions SET `+c.sql()+` WHERE id = ? AND canvas_id = ?`,
		append(c.args, id.String(), canvasID.String())...)
}

// ── Claims ────────────────────────────────────────────────────────────────────

// ClaimAction atomically claims an approved task: the conditional UPDATE
// (… WHERE state = 'approved') is the race guard — inside the store's single
// serialized transaction, exactly one of two racing claims can match. On
// no-match the action is re-read to report WHY (not found / already claimed /
// wrong state / wrong type).
//
// Lazy claim expiry (TDM-7): an executing claim older than claimTTL is taken
// over by a second conditional UPDATE (… AND claimed_at < cutoff) — the only
// moment a lapsed claim is ever observed, and the source of ExpiredClaimBy. A
// NAMED claimant retrying its own live claim gets it back idempotently; its own
// EXPIRED claim is restamped as a self-takeover (new lease, new generation, no
// ExpiredClaimBy).
func (s *Store) ClaimAction(ctx context.Context, canvasID, id uuid.UUID, claimedBy string) (*store.Action, store.ClaimOutcome, error) {
	var (
		claimed *store.Action
		out     store.ClaimOutcome
	)
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		now := time.Now().UTC()
		nowRaw := fmtTime(now)
		res, err := tx.ExecContext(ctx, `UPDATE actions SET state = 'executing', claimed_by = ?, claimed_at = ?, updated_at = ?
			WHERE id = ? AND canvas_id = ? AND state = 'approved' AND type = 'task'`,
			claimedBy, nowRaw, nowRaw, id.String(), canvasID.String())
		if err != nil {
			return err
		}
		if affected(res) == 1 {
			row, err := getActionRow(ctx, tx, canvasID, id)
			if err != nil {
				return err
			}
			gen := stampClaimRecord(ctx, tx, canvasID, row)
			v, err := s.bumpVersion(ctx, tx, canvasID)
			if err != nil {
				return err
			}
			claimed, out = row.action, store.ClaimOutcome{Version: v, ClaimGeneration: gen}
			return nil
		}
		existing, gerr := getAction(ctx, tx, canvasID, id)
		if gerr != nil {
			return store.ErrActionNotFound
		}
		if existing.Type != "task" {
			return fmt.Errorf("%w: only tasks can be claimed (this is a %q)", store.ErrIllegalActionState, existing.Type)
		}
		if existing.State != "executing" {
			return fmt.Errorf("%w: cannot claim task in state %q", store.ErrIllegalActionState, existing.State)
		}
		holder := ""
		if existing.ClaimedBy != nil {
			holder = *existing.ClaimedBy
		}
		expired := s.claimTTL > 0 && existing.ClaimedAt != nil && now.Sub(*existing.ClaimedAt) >= s.claimTTL
		if holder != "" && holder != "agent" && holder == claimedBy {
			if expired {
				a, o, ok, terr := s.takeOverExpiredClaim(ctx, tx, canvasID, id, claimedBy, now)
				if terr != nil {
					return terr
				}
				if ok {
					// Self-takeover: new lease, new generation, no handoff to report.
					claimed, out = a, o
					return nil
				}
			}
			// Idempotent retry of a live claim: same lease, same generation.
			claimed, out = existing, store.ClaimOutcome{ClaimGeneration: store.ReadClaimRecord(existing.Payload).Generation}
			return nil
		}
		if expired {
			a, o, ok, terr := s.takeOverExpiredClaim(ctx, tx, canvasID, id, claimedBy, now)
			if terr != nil {
				return terr
			}
			if ok {
				o.ExpiredClaimBy = holder
				if existing.ClaimedAt != nil {
					o.ExpiredClaimAt = *existing.ClaimedAt
				}
				claimed, out = a, o
				return nil
			}
		}
		return &store.AlreadyClaimedError{ClaimedBy: holder}
	})
	if err != nil {
		return nil, store.ClaimOutcome{}, err
	}
	return claimed, out, nil
}

// takeOverExpiredClaim runs the takeover UPDATE. ok=false with a nil error means
// "no takeover happened"; a lost race returns *AlreadyClaimedError naming the
// current holder.
func (s *Store) takeOverExpiredClaim(ctx context.Context, tx *sql.Tx, canvasID, id uuid.UUID, claimedBy string, now time.Time) (*store.Action, store.ClaimOutcome, bool, error) {
	cutoff := fmtTime(now.Add(-s.claimTTL))
	nowRaw := fmtTime(now)
	res, err := tx.ExecContext(ctx, `UPDATE actions SET state = 'executing', claimed_by = ?, claimed_at = ?, updated_at = ?
		WHERE id = ? AND canvas_id = ? AND state = 'executing' AND type = 'task' AND claimed_at < ?`,
		claimedBy, nowRaw, nowRaw, id.String(), canvasID.String(), cutoff)
	if err != nil {
		return nil, store.ClaimOutcome{}, false, err
	}
	if affected(res) == 1 {
		row, err := getActionRow(ctx, tx, canvasID, id)
		if err != nil {
			return nil, store.ClaimOutcome{}, false, err
		}
		gen := stampClaimRecord(ctx, tx, canvasID, row)
		v, err := s.bumpVersion(ctx, tx, canvasID)
		if err != nil {
			return nil, store.ClaimOutcome{}, false, err
		}
		return row.action, store.ClaimOutcome{Version: v, ClaimGeneration: gen}, true, nil
	}
	fresh, gerr := getAction(ctx, tx, canvasID, id)
	if gerr == nil && fresh.State == "executing" && fresh.ClaimedBy != nil {
		return nil, store.ClaimOutcome{}, false, &store.AlreadyClaimedError{ClaimedBy: *fresh.ClaimedBy}
	}
	return nil, store.ClaimOutcome{}, false, nil
}

// stampClaimRecord mints the fencing token (TDM-98) for the lease that just
// landed and writes it onto the payload, returning the new generation. Best
// effort by contract: any failure returns 0 (no token; the fence falls back to
// holder identity). The predicate matches the exact lease just stamped, so a
// takeover that landed in between leaves the winner's record standing.
func stampClaimRecord(ctx context.Context, q dbtx, canvasID uuid.UUID, row *actionRow) int {
	a := row.action
	if a.ClaimedBy == nil || !row.claimedAtRaw.Valid {
		return 0
	}
	at := time.Now().UTC()
	if a.ClaimedAt != nil {
		at = *a.ClaimedAt
	}
	rec := store.NextClaimRecord(a.Payload, *a.ClaimedBy, at)
	next, err := store.WithClaimRecord(a.Payload, rec)
	if err != nil {
		return 0
	}
	res, err := q.ExecContext(ctx, `UPDATE actions SET payload = ? WHERE id = ? AND canvas_id = ? AND type = 'task'
		AND state = 'executing' AND claimed_by = ? AND claimed_at = ?`,
		string(next), a.ID.String(), canvasID.String(), *a.ClaimedBy, row.claimedAtRaw.String)
	if err != nil || affected(res) != 1 {
		return 0
	}
	a.Payload = next
	return rec.Generation
}

// TouchActionClaim refreshes the lease on an executing task (a progress report
// is a heartbeat): holder-only, enforced in SQL. No version bump.
func (s *Store) TouchActionClaim(ctx context.Context, canvasID, id uuid.UUID, claimedBy string) (*store.Action, bool, error) {
	if claimedBy == "" {
		return nil, false, nil
	}
	var a *store.Action
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE actions SET claimed_at = ? WHERE id = ? AND canvas_id = ? AND type = 'task' AND state = 'executing' AND claimed_by = ?`,
			nowStr(), id.String(), canvasID.String(), claimedBy)
		if err != nil {
			return err
		}
		if affected(res) != 1 {
			return nil
		}
		a, err = getAction(ctx, tx, canvasID, id)
		return err
	})
	if err != nil {
		return nil, false, err
	}
	return a, a != nil, nil
}

// conditionalMove runs a conditional UPDATE on one task, then either returns
// the fresh row + bumped version, or re-reads to explain why nothing matched:
// missing → ErrActionNotFound; wrong type → typeErr; already in `to` → the
// existing row with version 0 (idempotent retry); otherwise → stateErr.
func (s *Store) conditionalMove(ctx context.Context, canvasID, id uuid.UUID, query string, args []any,
	wantType, to string, typeErr func(*store.Action) error, stateErr func(*store.Action) error) (*store.Action, int, error) {
	var (
		a *store.Action
		v int
	)
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return err
		}
		if affected(res) == 1 {
			if a, err = getAction(ctx, tx, canvasID, id); err != nil {
				return err
			}
			v, err = s.bumpVersion(ctx, tx, canvasID)
			return err
		}
		existing, gerr := getAction(ctx, tx, canvasID, id)
		if gerr != nil {
			return store.ErrActionNotFound
		}
		if existing.Type != wantType {
			return typeErr(existing)
		}
		if existing.State == to {
			a, v = existing, 0
			return nil
		}
		return stateErr(existing)
	})
	if err != nil {
		return nil, 0, err
	}
	return a, v, nil
}

// ReleaseAction frees a stuck claim (executing → approved, claim + error
// cleared). Releasing an already-released task is an idempotent success.
func (s *Store) ReleaseAction(ctx context.Context, canvasID, id uuid.UUID) (*store.Action, int, error) {
	return s.conditionalMove(ctx, canvasID, id,
		`UPDATE actions SET state = 'approved', claimed_by = NULL, claimed_at = NULL, error = NULL, updated_at = ?
		 WHERE id = ? AND canvas_id = ? AND state = 'executing' AND type = 'task'`,
		[]any{nowStr(), id.String(), canvasID.String()}, "task", "approved",
		func(e *store.Action) error {
			return fmt.Errorf("%w: only tasks can be released (this is a %q)", store.ErrIllegalActionState, e.Type)
		},
		func(e *store.Action) error {
			return fmt.Errorf("%w: cannot release task in state %q", store.ErrIllegalActionState, e.State)
		})
}

// RequeueAction sends a failed task back to the queue (failed → approved, claim,
// error AND result cleared). Idempotent on a task already in 'approved'.
func (s *Store) RequeueAction(ctx context.Context, canvasID, id uuid.UUID) (*store.Action, int, error) {
	return s.conditionalMove(ctx, canvasID, id,
		`UPDATE actions SET state = 'approved', claimed_by = NULL, claimed_at = NULL, error = NULL, result = NULL, updated_at = ?
		 WHERE id = ? AND canvas_id = ? AND state = 'failed' AND type = 'task'`,
		[]any{nowStr(), id.String(), canvasID.String()}, "task", "approved",
		func(e *store.Action) error {
			return fmt.Errorf("%w: only tasks can be re-queued (this is a %q)", store.ErrIllegalActionState, e.Type)
		},
		func(e *store.Action) error {
			return fmt.Errorf("%w: cannot re-queue task in state %q", store.ErrIllegalActionState, e.State)
		})
}

// ReopenAction is the human rewind out of a terminal-ish state (done →
// approved, rejected → proposed). (from, to) is the caller's validated pair.
func (s *Store) ReopenAction(ctx context.Context, canvasID, id uuid.UUID, from, to string) (*store.Action, int, error) {
	set := `state = ?, claimed_by = NULL, claimed_at = NULL, result = NULL, error = NULL, updated_at = ?`
	if to == "proposed" {
		set += `, approved_by = NULL`
	}
	return s.conditionalMove(ctx, canvasID, id,
		`UPDATE actions SET `+set+` WHERE id = ? AND canvas_id = ? AND state = ? AND type = 'task'`,
		[]any{to, nowStr(), id.String(), canvasID.String(), from}, "task", to,
		func(e *store.Action) error {
			return fmt.Errorf("%w: only tasks can be reopened (this is a %q)", store.ErrIllegalActionState, e.Type)
		},
		func(e *store.Action) error {
			return fmt.Errorf("%w: cannot move task from %q to %q (it is %q now)", store.ErrIllegalActionState, from, to, e.State)
		})
}

// MoveEpic is the owner's rewind of a whole BATCH (TDM-189): rejected →
// proposed, approved → proposed, done → proposed | rejected. `error` is cleared
// on every move and re-set only when this move IS the rejection.
func (s *Store) MoveEpic(ctx context.Context, canvasID, id uuid.UUID, from, to, reason string) (*store.Action, int, error) {
	var c setClause
	c.add("state", to)
	if to == "rejected" && strings.TrimSpace(reason) != "" {
		c.add("error", reason)
	} else {
		c.add("error", nil)
	}
	if to == "proposed" {
		c.add("approved_by", nil)
	}
	c.add("updated_at", nowStr())
	return s.conditionalMove(ctx, canvasID, id,
		`UPDATE actions SET `+c.sql()+` WHERE id = ? AND canvas_id = ? AND state = ? AND type = 'epic'`,
		append(c.args, id.String(), canvasID.String(), from), "epic", to,
		func(e *store.Action) error {
			return fmt.Errorf("%w: only epics can be moved this way (this is a %q)", store.ErrIllegalActionState, e.Type)
		},
		func(e *store.Action) error {
			return fmt.Errorf("%w: cannot move epic from %q to %q (it is %q now)", store.ErrIllegalActionState, from, to, e.State)
		})
}

// ResubmitAction sends a rejected task back to 'proposed' with the caller's
// (or the stored) payload, the audit entry appended to the STORED history, and
// approval/claim/error/result cleared.
func (s *Store) ResubmitAction(ctx context.Context, canvasID, id uuid.UUID, payload json.RawMessage, entry store.ContentAudit) (*store.Action, int, error) {
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
			return fmt.Errorf("%w: only tasks can be resubmitted (this is a %q)", store.ErrIllegalActionState, current.Type)
		}
		base := payload
		if len(base) == 0 {
			base = current.Payload
		}
		next, err := carryContentAudit(base, current.Payload, &entry)
		if err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `UPDATE actions SET state = 'proposed', payload = ?, approved_by = NULL, claimed_by = NULL,
			claimed_at = NULL, error = NULL, result = NULL, updated_at = ?
			WHERE id = ? AND canvas_id = ? AND state = 'rejected' AND type = 'task'`,
			string(next), nowStr(), id.String(), canvasID.String())
		if err != nil {
			return err
		}
		if affected(res) != 1 {
			return fmt.Errorf("%w: only a rejected task can be resubmitted (it is %q now)", store.ErrIllegalActionState, current.State)
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

// AppendActionAudit records one server-authored audit entry on an action's
// payload. No version bump: every caller is mid-move and broadcasts after.
func (s *Store) AppendActionAudit(ctx context.Context, canvasID, id uuid.UUID, entry store.ContentAudit) (json.RawMessage, error) {
	var next json.RawMessage
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		current, err := getAction(ctx, tx, canvasID, id)
		if err != nil {
			return store.ErrActionNotFound
		}
		if next, err = store.AppendAudit(current.Payload, entry); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE actions SET payload = ? WHERE id = ? AND canvas_id = ?`, string(next), id.String(), canvasID.String())
		return err
	})
	if err != nil {
		return nil, err
	}
	return next, nil
}

// AppendContentionEvent records one collision on an action's payload (TDM-100)
// and returns the fresh row. No version bump.
func (s *Store) AppendContentionEvent(ctx context.Context, canvasID, id uuid.UUID, ev store.ContentionEvent) (*store.Action, error) {
	var fresh *store.Action
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		current, err := getAction(ctx, tx, canvasID, id)
		if err != nil {
			return store.ErrActionNotFound
		}
		if ev.Generation == 0 {
			ev.Generation = store.ReadClaimRecord(current.Payload).Generation
		}
		next, err := store.AppendContention(current.Payload, ev)
		if err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `UPDATE actions SET payload = ? WHERE id = ? AND canvas_id = ?`, string(next), id.String(), canvasID.String())
		if err != nil {
			return err
		}
		if affected(res) == 0 {
			return store.ErrActionNotFound
		}
		fresh, err = getAction(ctx, tx, canvasID, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return fresh, nil
}

// UpdateActionPayload replaces an action's payload under the content-mutation
// gate (TDM-41): store.DecideContentUpdate IS the rule; this is the read, the
// write and the concurrency guard. A reverting edit drops the task back to
// 'proposed' with claim and approval cleared in the SAME statement, guarded by
// a state predicate so a concurrent move is reported instead of clobbered.
func (s *Store) UpdateActionPayload(ctx context.Context, canvasID, id uuid.UUID, payload json.RawMessage, actor string) (*store.ContentUpdate, error) {
	var out *store.ContentUpdate
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		current, err := getAction(ctx, tx, canvasID, id)
		if err != nil {
			return store.ErrActionNotFound
		}
		next, decided, err := store.DecideContentUpdate(current, payload, actor, time.Now().UTC())
		if err != nil {
			return err
		}
		out = decided
		if !out.Reverted {
			if _, err := tx.ExecContext(ctx, `UPDATE actions SET payload = ?, updated_at = ? WHERE id = ? AND canvas_id = ?`,
				string(next), nowStr(), id.String(), canvasID.String()); err != nil {
				return err
			}
			out.Version, err = s.bumpVersion(ctx, tx, canvasID)
			return err
		}
		res, err := tx.ExecContext(ctx, `UPDATE actions SET payload = ?, state = 'proposed', claimed_by = NULL, claimed_at = NULL,
			approved_by = NULL, updated_at = ? WHERE id = ? AND canvas_id = ? AND state = ?`,
			string(next), nowStr(), id.String(), canvasID.String(), current.State)
		if err != nil {
			return err
		}
		if affected(res) != 1 {
			now := "another state"
			if fresh, gerr := getAction(ctx, tx, canvasID, id); gerr == nil {
				now = fmt.Sprintf("%q", fresh.State)
			}
			return fmt.Errorf("%w: the task moved to %s while the edit was in flight — re-read it and retry", store.ErrIllegalActionState, now)
		}
		if out.Version, err = s.bumpVersion(ctx, tx, canvasID); err != nil {
			return err
		}
		out.Action, err = getAction(ctx, tx, canvasID, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) DeleteAction(ctx context.Context, canvasID, id uuid.UUID) (int, error) {
	return s.affectedThenBump(ctx, canvasID, `DELETE FROM actions WHERE id = ? AND canvas_id = ?`, id.String(), canvasID.String())
}

// bulkMove runs one UPDATE ... RETURNING over many actions, bumping the version
// only when something changed. Returns the rows that actually flipped.
func (s *Store) bulkMove(ctx context.Context, canvasID uuid.UUID, query string, args ...any) ([]*store.Action, error) {
	out := []*store.Action{}
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, query, args...)
		if err != nil {
			return err
		}
		if out, err = scanActions(rows); err != nil {
			return err
		}
		if len(out) == 0 {
			return nil
		}
		_, err = s.bumpVersion(ctx, tx, canvasID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// requiresApprovalOff keeps tasks whose payload.requiresApproval is absent,
// null, or anything other than true (JSON true or the string "true").
const requiresApprovalOff = `COALESCE(json_extract(payload, '$.requiresApproval') IN (1, 'true'), 0) = 0`

// ApproveEpicTasks flips every currently-proposed task under an epic to
// approved in ONE statement, stamping approved_by with the policy provenance.
// Tasks self-flagged requiresApproval:true keep their individual gate.
func (s *Store) ApproveEpicTasks(ctx context.Context, canvasID, epicID uuid.UUID, approvedBy string) ([]*store.Action, error) {
	return s.bulkMove(ctx, canvasID, `UPDATE actions SET state = 'approved', approved_by = ?, updated_at = ?
		WHERE canvas_id = ? AND type = 'task' AND state = 'proposed'
		  AND json_extract(payload, '$.epicId') = ? AND `+requiresApprovalOff+`
		RETURNING `+actionCols,
		approvedBy, nowStr(), canvasID.String(), epicID.String())
}

// UnapproveEpicTasks is the inverse: tickets in the ready queue only because
// their epic was approved (approved_by = onlyApprovedBy, unclaimed) go back to
// 'proposed' in ONE statement. Never takes a ticket off a worker.
func (s *Store) UnapproveEpicTasks(ctx context.Context, canvasID, epicID uuid.UUID, onlyApprovedBy string) ([]*store.Action, error) {
	if onlyApprovedBy == "" {
		return []*store.Action{}, nil
	}
	return s.bulkMove(ctx, canvasID, `UPDATE actions SET state = 'proposed', approved_by = NULL, updated_at = ?
		WHERE canvas_id = ? AND type = 'task' AND state = 'approved' AND approved_by = ? AND claimed_by IS NULL
		  AND json_extract(payload, '$.epicId') = ?
		RETURNING `+actionCols,
		nowStr(), canvasID.String(), onlyApprovedBy, epicID.String())
}

// ApproveActionsBatch flips every listed action still in 'proposed' to approved
// in ONE statement; ids that already moved on simply stop matching.
func (s *Store) ApproveActionsBatch(ctx context.Context, canvasID uuid.UUID, ids []uuid.UUID, approvedBy string) ([]*store.Action, error) {
	if len(ids) == 0 {
		return []*store.Action{}, nil
	}
	placeholders, inArgs := inArgs(ids)
	args := append([]any{approvedBy, nowStr(), canvasID.String()}, inArgs...)
	return s.bulkMove(ctx, canvasID, `UPDATE actions SET state = 'approved', approved_by = ?, updated_at = ?
		WHERE canvas_id = ? AND state = 'proposed' AND id IN (`+placeholders+`)
		RETURNING `+actionCols, args...)
}
