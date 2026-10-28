package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/agentcanvas/api/internal/store"
)

const agentCols = `id, name, role, model, parent_agent_id, status, created_at, last_seen_at`

func scanAgent(r rowScanner) (*store.Agent, error) {
	var (
		a                       store.Agent
		id, createdAt, lastSeen string
		model, parent           sql.NullString
	)
	if err := r.Scan(&id, &a.Name, &a.Role, &model, &parent, &a.Status, &createdAt, &lastSeen); err != nil {
		return nil, err
	}
	a.ID = mustUUID(id)
	a.Kind = "agent"
	a.Model = strPtr(model)
	a.ParentAgentID = uuidPtr(parent)
	a.CreatedAt = parseTime(createdAt)
	a.LastSeenAt = parseTime(lastSeen)
	return &a, nil
}

func listAgents(ctx context.Context, q dbtx, canvasID uuid.UUID) ([]*store.Agent, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+agentCols+` FROM agents WHERE canvas_id = ? ORDER BY created_at, id`, canvasID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*store.Agent{}
	for rows.Next() {
		a, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// upsertAgentByName is the single agent write path (RegisterAgent and the claim
// path's TouchOrCreateAgent): INSERT ... ON CONFLICT (canvas_id, name) DO UPDATE.
// A fresh insert mints an id; a conflict keeps the existing id, so
// re-registering never re-keys an agent out from under its children or
// provenance. model / parent_agent_id are updated ONLY when the caller supplied
// them — an absent value means "leave unchanged", so a subagent re-registering
// without re-threading its orchestrator id is not silently unparented.
func upsertAgentByName(ctx context.Context, q dbtx, canvasID uuid.UUID, a *store.Agent) (*store.Agent, error) {
	now := nowStr()
	set := `role = excluded.role, status = 'online', last_seen_at = excluded.last_seen_at`
	if a.Model != nil {
		set += `, model = excluded.model`
	}
	if a.ParentAgentID != nil {
		set += `, parent_agent_id = excluded.parent_agent_id`
	}
	saved, err := scanAgent(q.QueryRowContext(ctx, `INSERT INTO agents
		(id, canvas_id, name, role, model, parent_agent_id, status, created_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?, ?, 'online', ?, ?)
		ON CONFLICT (canvas_id, name) DO UPDATE SET `+set+`
		RETURNING `+agentCols,
		uuid.New().String(), canvasID.String(), a.Name, a.Role, strArg(a.Model), uuidArg(a.ParentAgentID), now, now))
	if err != nil {
		return nil, fmt.Errorf("agent upsert %q: %w", a.Name, err)
	}
	return saved, nil
}

// RegisterAgent upserts on the (canvas_id, name) identity; on return a carries
// the surviving row's REAL id. Bumps the version.
func (s *Store) RegisterAgent(ctx context.Context, canvasID uuid.UUID, a *store.Agent) (int, error) {
	v := 0
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		saved, err := upsertAgentByName(ctx, tx, canvasID, a)
		if err != nil {
			return err
		}
		*a = *saved
		v, err = s.bumpVersion(ctx, tx, canvasID)
		return err
	})
	return v, err
}

// maxAgentNameAttempts bounds the fresh-name search. 32^4 ≈ 1M suffixes per
// base name, so needing more than a couple of tries means something is wrong.
const maxAgentNameAttempts = 16

// RegisterAgentDistinct: see store.Store. Looks up the row holding the
// requested name (and the session's own row), applies store.ResolveAgentName,
// then either runs the ordinary upsert or inserts under the first unused
// suffixed name, all inside one transaction.
func (s *Store) RegisterAgentDistinct(ctx context.Context, canvasID uuid.UUID, a *store.Agent, in store.AgentRegisterIntent) (int, error) {
	if in.LiveAfter.IsZero() && s.claimTTL > 0 {
		in.LiveAfter = time.Now().Add(-s.claimTTL)
	}
	v := 0
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		var existing, sessionAgent *store.Agent
		var err error
		if a.Name != "" {
			if existing, err = agentByName(ctx, tx, canvasID, a.Name); err != nil {
				return err
			}
		}
		if !in.NameGiven && in.SessionAgentID != nil {
			sessionAgent, err = scanAgent(tx.QueryRowContext(ctx, `SELECT `+agentCols+` FROM agents WHERE id = ? AND canvas_id = ?`,
				in.SessionAgentID.String(), canvasID.String()))
			if errors.Is(err, sql.ErrNoRows) {
				sessionAgent, err = nil, nil
			}
			if err != nil {
				return err
			}
		}
		name, decision := store.ResolveAgentName(a, in, existing, sessionAgent)
		a.Name = name
		if decision == store.DistinctAgent {
			fresh, err := unusedAgentName(ctx, tx, canvasID, name)
			if err != nil {
				return err
			}
			a.Name = fresh
		}
		saved, err := upsertAgentByName(ctx, tx, canvasID, a)
		if err != nil {
			return err
		}
		*a = *saved
		v, err = s.bumpVersion(ctx, tx, canvasID)
		return err
	})
	return v, err
}

// agentByName returns the row holding name on the canvas, or nil.
func agentByName(ctx context.Context, q dbtx, canvasID uuid.UUID, name string) (*store.Agent, error) {
	a, err := scanAgent(q.QueryRowContext(ctx, `SELECT `+agentCols+` FROM agents WHERE canvas_id = ? AND name = ?`, canvasID.String(), name))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return a, err
}

// unusedAgentName returns the first SuffixedAgentName(base) no agent on the
// canvas holds.
func unusedAgentName(ctx context.Context, q dbtx, canvasID uuid.UUID, base string) (string, error) {
	for i := 0; i < maxAgentNameAttempts; i++ {
		candidate := store.SuffixedAgentName(base)
		held, err := agentByName(ctx, q, canvasID, candidate)
		if err != nil {
			return "", err
		}
		if held == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no unused agent name derived from %q after %d attempts", base, maxAgentNameAttempts)
}

func (s *Store) GetAgent(ctx context.Context, canvasID, id uuid.UUID) (*store.Agent, error) {
	a, err := scanAgent(s.db.QueryRowContext(ctx, `SELECT `+agentCols+` FROM agents WHERE id = ? AND canvas_id = ?`, id.String(), canvasID.String()))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("agent %s not found", id)
	}
	return a, err
}

func (s *Store) ListAgents(ctx context.Context, canvasID uuid.UUID) ([]*store.Agent, error) {
	return listAgents(ctx, s.db, canvasID)
}

// TouchAgentLastSeen bumps last_seen_at for the agent the claimant names (a
// registered NAME or an id). The generic "agent" identity names nobody and is
// skipped. Presence-only: no version bump.
func (s *Store) TouchAgentLastSeen(ctx context.Context, canvasID uuid.UUID, claimant string) error {
	_, err := touchAgent(ctx, s.db, canvasID, claimant)
	return err
}

func touchAgent(ctx context.Context, q dbtx, canvasID uuid.UUID, claimant string) (int, error) {
	if claimant == "" || claimant == "agent" {
		return 0, nil
	}
	col := "name"
	if _, err := uuid.Parse(claimant); err == nil {
		col = "id"
	}
	res, err := q.ExecContext(ctx, `UPDATE agents SET last_seen_at = ?, status = 'online' WHERE canvas_id = ? AND `+col+` = ?`,
		nowStr(), canvasID.String(), claimant)
	if err != nil {
		return 0, err
	}
	return affected(res), nil
}

// TouchOrCreateAgent is the claim-path presence refresh: touch the claimant's
// row, and when a NAME claimant has no row, create a minimal executor row via
// the same upsert RegisterAgent uses. An id claimant is only touched. No bump.
func (s *Store) TouchOrCreateAgent(ctx context.Context, canvasID uuid.UUID, claimant string) error {
	if claimant == "" || claimant == "agent" {
		return nil
	}
	if _, err := uuid.Parse(claimant); err == nil {
		return s.TouchAgentLastSeen(ctx, canvasID, claimant)
	}
	return s.withTx(ctx, func(tx *sql.Tx) error {
		n, err := touchAgent(ctx, tx, canvasID, claimant)
		if err != nil || n > 0 {
			return err
		}
		_, err = upsertAgentByName(ctx, tx, canvasID, &store.Agent{Name: claimant, Role: "executor"})
		return err
	})
}
