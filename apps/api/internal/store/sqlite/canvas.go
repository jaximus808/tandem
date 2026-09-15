package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/agentcanvas/api/internal/store"
)

const canvasCols = `id, code, name, mode, enabled_modes, owner_user_id, visibility, public_role,
	approval_policy, require_cross_model_review, briefing_doc_id, version, created_at, updated_at`

type rowScanner interface{ Scan(dest ...any) error }

// scanCanvas reads one canvases row. claim_token is deliberately never
// selected: it is surfaced ONLY on the create response (CreateCanvas sets it),
// so every other read path omits it — same rule the PostgREST store enforced in
// toCanvas.
func scanCanvas(r rowScanner) (*store.Canvas, error) {
	var (
		c                       store.Canvas
		id, enabledModes        string
		owner, briefing         sql.NullString
		createdAt, updatedAt    string
		requireCrossModelReview int
	)
	if err := r.Scan(&id, &c.Code, &c.Name, &c.Mode, &enabledModes, &owner, &c.Visibility, &c.PublicRole,
		&c.ApprovalPolicy, &requireCrossModelReview, &briefing, &c.Version, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	c.ID = mustUUID(id)
	c.EnabledModes = jsonStrings(enabledModes)
	c.OwnerUserID = uuidPtr(owner)
	c.BriefingDocID = uuidPtr(briefing)
	c.RequireCrossModelReview = requireCrossModelReview != 0
	c.CreatedAt = parseTime(createdAt)
	c.UpdatedAt = parseTime(updatedAt)
	return &c, nil
}

func scanCanvases(rows *sql.Rows) ([]*store.Canvas, error) {
	defer rows.Close()
	out := []*store.Canvas{}
	for rows.Next() {
		c, err := scanCanvas(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func getCanvasByID(ctx context.Context, q dbtx, id uuid.UUID) (*store.Canvas, error) {
	c, err := scanCanvas(q.QueryRowContext(ctx, `SELECT `+canvasCols+` FROM canvases WHERE id = ?`, id.String()))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("canvas not found: %s", id)
	}
	return c, err
}

func getCanvasByCode(ctx context.Context, q dbtx, code string) (*store.Canvas, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	c, err := scanCanvas(q.QueryRowContext(ctx, `SELECT `+canvasCols+` FROM canvases WHERE code = ?`, code))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("canvas not found: %s", code)
	}
	return c, err
}

// ── Canvas ────────────────────────────────────────────────────────────────────

func (s *Store) CreateCanvas(ctx context.Context, name string, ownerUserID *uuid.UUID, visibility, publicRole string) (*store.Canvas, error) {
	// An anonymous create (no owner — the MCP/gateway path) gets a claim token so
	// a human can later make it theirs; an owned create needs none.
	var claimToken any
	token := ""
	if ownerUserID == nil {
		token = store.GenerateClaimToken()
		claimToken = token
	}
	if visibility == "" {
		visibility = "public"
	}
	if publicRole == "" {
		publicRole = "write"
	}
	for range 10 {
		id := uuid.New()
		code := store.GenerateCode()
		now := nowStr()
		_, err := s.db.ExecContext(ctx, `INSERT INTO canvases
			(id, code, name, mode, enabled_modes, owner_user_id, claim_token, visibility, public_role, created_at, updated_at)
			VALUES (?, ?, ?, 'welcome', '[]', ?, ?, ?, ?, ?, ?)`,
			id.String(), code, name, uuidArg(ownerUserID), claimToken, visibility, publicRole, now, now)
		if err != nil {
			if isUniqueViolation(err, "canvases.code") {
				continue
			}
			return nil, err
		}
		c, err := getCanvasByID(ctx, s.db, id)
		if err != nil {
			return nil, err
		}
		c.ClaimToken = token
		return c, nil
	}
	return nil, fmt.Errorf("failed to generate unique canvas code after 10 attempts")
}

// ListCanvasesByOwner returns a user's owned canvases, newest-edited first.
func (s *Store) ListCanvasesByOwner(ctx context.Context, ownerUserID uuid.UUID) ([]*store.Canvas, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+canvasCols+` FROM canvases WHERE owner_user_id = ? ORDER BY updated_at DESC`, ownerUserID.String())
	if err != nil {
		return nil, err
	}
	return scanCanvases(rows)
}

// ListAllCanvases returns every canvas, newest-edited first — the local-mode
// dashboard read, where there is one human and every canvas is theirs.
func (s *Store) ListAllCanvases(ctx context.Context) ([]*store.Canvas, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+canvasCols+` FROM canvases ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	return scanCanvases(rows)
}

// CopyCanvas is the copy_canvas RPC (migration 0018) as one transaction: a new
// canvas owned by ownerUserID carrying the source's content (documents, notes,
// roadmap items) under fresh ids, with internal references remapped. Runtime
// rows (actions, agents, pending edits, webhooks) are deliberately not copied.
//
// One improvement over the RPC: documents ARE copied (the RPC predated them and
// left copied notes/items pointing at nothing), so a copied canvas keeps its
// tabs and its briefing designation.
func (s *Store) CopyCanvas(ctx context.Context, srcID, ownerUserID uuid.UUID, name string) (*store.Canvas, error) {
	for range 10 {
		code := store.GenerateCode()
		newID := uuid.New()
		err := s.withTx(ctx, func(tx *sql.Tx) error {
			src, err := getCanvasByID(ctx, tx, srcID)
			if err != nil {
				return err
			}
			now := nowStr()
			if _, err := tx.ExecContext(ctx, `INSERT INTO canvases
				(id, code, name, mode, enabled_modes, owner_user_id, visibility, public_role, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, 'public', 'write', ?, ?)`,
				newID.String(), code, name, src.Mode, jsonText(src.EnabledModes), ownerUserID.String(), now, now); err != nil {
				return err
			}

			// documents: fresh ids, parent (folder) links remapped in a second pass.
			docs, err := listDocuments(ctx, tx, srcID)
			if err != nil {
				return err
			}
			docMap := map[uuid.UUID]uuid.UUID{}
			for _, d := range docs {
				docMap[d.ID] = uuid.New()
			}
			for _, d := range docs {
				if _, err := tx.ExecContext(ctx, `INSERT INTO documents
					(id, canvas_id, type, name, parent_id, sort_order, config, created_by, authored_by, verified_at, stale_after_seconds, created_at, updated_at)
					VALUES (?, ?, ?, ?, NULL, ?, ?, ?, ?, ?, ?, ?, ?)`,
					docMap[d.ID].String(), newID.String(), d.Type, d.Name, d.SortOrder, jsonText(d.Config), d.CreatedBy,
					strArg(d.AuthoredBy), timeArg(d.VerifiedAt), intArg(d.StaleAfterSeconds), now, now); err != nil {
					return err
				}
			}
			for _, d := range docs {
				if d.ParentID == nil {
					continue
				}
				if p, ok := docMap[*d.ParentID]; ok {
					if _, err := tx.ExecContext(ctx, `UPDATE documents SET parent_id = ? WHERE id = ?`, p.String(), docMap[d.ID].String()); err != nil {
						return err
					}
				}
			}
			if src.BriefingDocID != nil {
				if b, ok := docMap[*src.BriefingDocID]; ok {
					if _, err := tx.ExecContext(ctx, `UPDATE canvases SET briefing_doc_id = ? WHERE id = ?`, b.String(), newID.String()); err != nil {
						return err
					}
				}
			}
			remapDoc := func(id *uuid.UUID) any {
				if id == nil {
					return nil
				}
				if n, ok := docMap[*id]; ok {
					return n.String()
				}
				return nil
			}

			// notes: parent_id pointed at pins/events, which no longer exist → NULL.
			notes, err := listNotes(ctx, tx, srcID, nil)
			if err != nil {
				return err
			}
			for _, n := range notes {
				if _, err := tx.ExecContext(ctx, `INSERT INTO notes
					(id, canvas_id, document_id, body, image_refs, parent_id, parent_kind, sort_order, created_by, authored_by, verified_at, stale_after_seconds, created_at, updated_at)
					VALUES (?, ?, ?, ?, ?, NULL, NULL, ?, ?, ?, ?, ?, ?, ?)`,
					uuid.New().String(), newID.String(), remapDoc(n.DocumentID), n.Body, jsonText(n.ImageRefs), n.SortOrder, n.CreatedBy,
					strArg(n.AuthoredBy), timeArg(n.VerifiedAt), intArg(n.StaleAfterSeconds), now, now); err != nil {
					return err
				}
			}

			// roadmap items: insert with NULL parent (self-referencing FK), then wire.
			items, err := listRoadmapItems(ctx, tx, srcID, "")
			if err != nil {
				return err
			}
			rmMap := map[uuid.UUID]uuid.UUID{}
			for _, r := range items {
				rmMap[r.ID] = uuid.New()
			}
			for _, r := range items {
				if _, err := tx.ExecContext(ctx, `INSERT INTO roadmap_items
					(id, canvas_id, document_id, parent_id, title, body, status, stage, assignee, sort_order, created_by, verified_at, stale_after_seconds, created_at, updated_at)
					VALUES (?, ?, ?, NULL, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
					rmMap[r.ID].String(), newID.String(), remapDoc(r.DocumentID), r.Title, r.Body, r.Status,
					nullIfEmpty(r.Stage), nullIfEmpty(r.Assignee), r.SortOrder, r.CreatedBy,
					timeArg(r.VerifiedAt), intArg(r.StaleAfterSeconds), now, now); err != nil {
					return err
				}
			}
			for _, r := range items {
				if r.ParentID == nil {
					continue
				}
				if p, ok := rmMap[*r.ParentID]; ok {
					if _, err := tx.ExecContext(ctx, `UPDATE roadmap_items SET parent_id = ? WHERE id = ?`, p.String(), rmMap[r.ID].String()); err != nil {
						return err
					}
				}
			}
			return nil
		})
		if err != nil {
			if isUniqueViolation(err, "canvases.code") {
				continue
			}
			return nil, err
		}
		return getCanvasByID(ctx, s.db, newID)
	}
	return nil, fmt.Errorf("failed to generate unique canvas code after 10 attempts")
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// ClaimCanvas atomically transfers an unowned canvas to ownerUserID: the single
// conditional UPDATE is the whole race guard, and the token is voided in the
// same statement so it is single-use. On no-match the canvas is read back to
// say WHY (not found / already claimed / wrong token).
func (s *Store) ClaimCanvas(ctx context.Context, code, claimToken string, ownerUserID uuid.UUID) (*store.Canvas, error) {
	code = strings.ToUpper(code)
	res, err := s.db.ExecContext(ctx, `UPDATE canvases SET owner_user_id = ?, claim_token = NULL, updated_at = ?
		WHERE code = ? AND claim_token = ? AND owner_user_id IS NULL`,
		ownerUserID.String(), nowStr(), code, claimToken)
	if err != nil {
		return nil, err
	}
	if affected(res) == 1 {
		return getCanvasByCode(ctx, s.db, code)
	}
	existing, gerr := getCanvasByCode(ctx, s.db, code)
	if gerr != nil {
		return nil, store.ErrCanvasNotFound
	}
	if existing.OwnerUserID != nil {
		return nil, store.ErrAlreadyClaimed
	}
	return nil, store.ErrInvalidClaimToken
}

// DeleteCanvas removes the canvas row; every table that FKs canvases does so
// with ON DELETE CASCADE, so all content, access rows and notifications go with
// it in the same statement. Owner-only checks live in the handler.
func (s *Store) DeleteCanvas(ctx context.Context, canvasID uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM canvases WHERE id = ?`, canvasID.String())
	return err
}

func (s *Store) GetCanvasByCode(ctx context.Context, code string) (*store.Canvas, error) {
	return getCanvasByCode(ctx, s.db, code)
}

func (s *Store) GetCanvasByID(ctx context.Context, id uuid.UUID) (*store.Canvas, error) {
	return getCanvasByID(ctx, s.db, id)
}

// ── Access control (migration 0021) ───────────────────────────────────────────

// ResolveCanvasRole — the single resolver behind both write paths. owner→write,
// explicit access row→its role, else public→public_role, else none.
func (s *Store) ResolveCanvasRole(ctx context.Context, canvas *store.Canvas, userID *uuid.UUID) (string, error) {
	if canvas == nil {
		return "none", nil
	}
	if userID != nil {
		if canvas.OwnerUserID != nil && *canvas.OwnerUserID == *userID {
			return "write", nil
		}
		var role string
		err := s.db.QueryRowContext(ctx, `SELECT role FROM canvas_access WHERE canvas_id = ? AND user_id = ?`,
			canvas.ID.String(), userID.String()).Scan(&role)
		if err == nil {
			return role, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
	}
	if canvas.Visibility == "private" {
		return "none", nil
	}
	role := canvas.PublicRole
	if role == "" {
		role = "write"
	}
	return role, nil
}

func (s *Store) SetCanvasVisibility(ctx context.Context, canvasID uuid.UUID, visibility, publicRole string) (int, error) {
	return s.execThenBump(ctx, canvasID, `UPDATE canvases SET visibility = ?, public_role = ?, updated_at = ? WHERE id = ?`,
		visibility, publicRole, nowStr(), canvasID.String())
}

func (s *Store) SetCanvasName(ctx context.Context, canvasID uuid.UUID, name string) (int, error) {
	return s.execThenBump(ctx, canvasID, `UPDATE canvases SET name = ?, updated_at = ? WHERE id = ?`,
		name, nowStr(), canvasID.String())
}

func (s *Store) SetCanvasApprovalPolicy(ctx context.Context, canvasID uuid.UUID, policy string) (int, error) {
	return s.execThenBump(ctx, canvasID, `UPDATE canvases SET approval_policy = ?, updated_at = ? WHERE id = ?`,
		policy, nowStr(), canvasID.String())
}

func (s *Store) SetCanvasCrossModelReview(ctx context.Context, canvasID uuid.UUID, required bool) (int, error) {
	return s.execThenBump(ctx, canvasID, `UPDATE canvases SET require_cross_model_review = ?, updated_at = ? WHERE id = ?`,
		boolInt(required), nowStr(), canvasID.String())
}

func (s *Store) SetCanvasBriefingDoc(ctx context.Context, canvasID uuid.UUID, docID *uuid.UUID) (int, error) {
	return s.execThenBump(ctx, canvasID, `UPDATE canvases SET briefing_doc_id = ?, updated_at = ? WHERE id = ?`,
		uuidArg(docID), nowStr(), canvasID.String())
}

func (s *Store) ListCanvasAccess(ctx context.Context, canvasID uuid.UUID) ([]*store.CanvasAccess, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT a.user_id, a.role, u.email, u.display_name, u.avatar_url
		FROM canvas_access a JOIN users u ON u.id = a.user_id WHERE a.canvas_id = ? ORDER BY a.created_at`, canvasID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*store.CanvasAccess{}
	for rows.Next() {
		var uid string
		ca := &store.CanvasAccess{}
		if err := rows.Scan(&uid, &ca.Role, &ca.Email, &ca.DisplayName, &ca.AvatarURL); err != nil {
			return nil, err
		}
		ca.UserID = mustUUID(uid)
		out = append(out, ca)
	}
	return out, rows.Err()
}

func (s *Store) UpsertCanvasAccess(ctx context.Context, canvasID, userID uuid.UUID, role string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO canvas_access (canvas_id, user_id, role, created_at) VALUES (?, ?, ?, ?)
		ON CONFLICT (canvas_id, user_id) DO UPDATE SET role = excluded.role`,
		canvasID.String(), userID.String(), role, nowStr())
	return err
}

func (s *Store) DeleteCanvasAccess(ctx context.Context, canvasID, userID uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM canvas_access WHERE canvas_id = ? AND user_id = ?`, canvasID.String(), userID.String())
	return err
}

// ListCanvasesSharedWithUser is the recipient's "shared with me" view, each
// canvas stamped with the role this user was granted. Newest-edited first.
func (s *Store) ListCanvasesSharedWithUser(ctx context.Context, userID uuid.UUID) ([]*store.Canvas, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+prefixCols("c.", canvasCols)+`, a.role
		FROM canvas_access a JOIN canvases c ON c.id = a.canvas_id
		WHERE a.user_id = ? ORDER BY c.updated_at DESC`, userID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*store.Canvas{}
	for rows.Next() {
		var (
			c                       store.Canvas
			id, enabledModes        string
			owner, briefing         sql.NullString
			createdAt, updatedAt    string
			requireCrossModelReview int
			role                    string
		)
		if err := rows.Scan(&id, &c.Code, &c.Name, &c.Mode, &enabledModes, &owner, &c.Visibility, &c.PublicRole,
			&c.ApprovalPolicy, &requireCrossModelReview, &briefing, &c.Version, &createdAt, &updatedAt, &role); err != nil {
			return nil, err
		}
		c.ID = mustUUID(id)
		c.EnabledModes = jsonStrings(enabledModes)
		c.OwnerUserID = uuidPtr(owner)
		c.BriefingDocID = uuidPtr(briefing)
		c.RequireCrossModelReview = requireCrossModelReview != 0
		c.CreatedAt = parseTime(createdAt)
		c.UpdatedAt = parseTime(updatedAt)
		c.YourRole = role
		out = append(out, &c)
	}
	return out, rows.Err()
}

// prefixCols turns "a, b,\n c" into "p.a, p.b, p.c" for joined selects.
func prefixCols(prefix, cols string) string {
	parts := strings.Split(cols, ",")
	for i, p := range parts {
		parts[i] = prefix + strings.TrimSpace(p)
	}
	return strings.Join(parts, ", ")
}

// ── Notifications (migration 0022) ────────────────────────────────────────────

func (s *Store) CreateNotification(ctx context.Context, n *store.Notification) error {
	recipient, actor := n.RecipientID(), n.ActorID()
	if n.ID == uuid.Nil {
		n.ID = uuid.New()
	}
	if n.CreatedAt.IsZero() {
		n.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO notifications (id, user_id, kind, canvas_id, actor_user_id, role, read_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, NULL, ?)`,
		n.ID.String(), recipient.String(), n.Kind, uuidArg(n.CanvasID), uuidArg(actor), nullIfEmpty(n.Role), fmtTime(n.CreatedAt))
	return err
}

func (s *Store) ListNotifications(ctx context.Context, userID uuid.UUID, limit int) ([]*store.Notification, error) {
	q := `SELECT n.id, n.kind, n.canvas_id, n.role, n.read_at, n.created_at, c.code, c.name, u.display_name
		FROM notifications n
		LEFT JOIN canvases c ON c.id = n.canvas_id
		LEFT JOIN users u ON u.id = n.actor_user_id
		WHERE n.user_id = ? ORDER BY n.created_at DESC`
	args := []any{userID.String()}
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*store.Notification{}
	for rows.Next() {
		var (
			id, kind, createdAt                     string
			canvasID, role, readAt, code, name, act sql.NullString
		)
		if err := rows.Scan(&id, &kind, &canvasID, &role, &readAt, &createdAt, &code, &name, &act); err != nil {
			return nil, err
		}
		n := &store.Notification{ID: mustUUID(id), Kind: kind, Read: readAt.Valid, CreatedAt: parseTime(createdAt),
			CanvasID: uuidPtr(canvasID), Role: role.String, CanvasCode: code.String, CanvasName: name.String, ActorName: act.String}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) CountUnreadNotifications(ctx context.Context, userID uuid.UUID) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM notifications WHERE user_id = ? AND read_at IS NULL`, userID.String()).Scan(&n)
	return n, err
}

func (s *Store) MarkNotificationsRead(ctx context.Context, userID uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, `UPDATE notifications SET read_at = ? WHERE user_id = ? AND read_at IS NULL`, nowStr(), userID.String())
	return err
}

// ── State reads ───────────────────────────────────────────────────────────────

// emptyCanvasState builds a CanvasState carrying only the bearings, with every
// surviving kind map allocated empty.
func emptyCanvasState(version int, mode string, enabledModes []string) *store.CanvasState {
	return &store.CanvasState{
		Version:      version,
		Mode:         mode,
		EnabledModes: enabledModes,
		Documents:    map[string]*store.Document{},
		Notes:        map[string]*store.Note{},
		RoadmapItems: map[string]*store.RoadmapItem{},
		Actions:      map[string]*store.Action{},
		Agents:       map[string]*store.Agent{},
	}
}

// stateKinds is every kind GetCanvasState loads, in load order.
var stateKinds = []string{"documents", "notes", "roadmapItems", "actions", "agents"}

// GetCanvasState is the full snapshot: the canvas row plus every child table.
func (s *Store) GetCanvasState(ctx context.Context, canvasID uuid.UUID) (*store.Canvas, *store.CanvasState, []*store.PendingEdit, error) {
	return s.GetCanvasKinds(ctx, canvasID, stateKinds)
}

// GetCanvasKinds loads only the requested kinds. Kinds not requested are left as
// nil maps so they serialize as absent, matching the wire contract; unknown kind
// names are ignored.
func (s *Store) GetCanvasKinds(ctx context.Context, canvasID uuid.UUID, kinds []string) (*store.Canvas, *store.CanvasState, []*store.PendingEdit, error) {
	canvas, err := getCanvasByID(ctx, s.db, canvasID)
	if err != nil {
		return nil, nil, nil, err
	}
	state := emptyCanvasState(canvas.Version, canvas.Mode, canvas.EnabledModes)
	want := map[string]bool{}
	for _, k := range kinds {
		want[k] = true
	}
	if !want["documents"] {
		state.Documents = nil
	}
	if !want["notes"] {
		state.Notes = nil
	}
	if !want["roadmapItems"] {
		state.RoadmapItems = nil
	}
	if !want["actions"] {
		state.Actions = nil
	}
	if !want["agents"] {
		state.Agents = nil
	}
	for _, k := range kinds {
		if err := s.loadKind(ctx, canvasID, k, state); err != nil {
			return nil, nil, nil, fmt.Errorf("GetCanvasKinds(%s): %w", k, err)
		}
	}
	edits, err := s.ListPendingEdits(ctx, canvasID)
	if err != nil {
		return nil, nil, nil, err
	}
	return canvas, state, edits, nil
}

func (s *Store) loadKind(ctx context.Context, canvasID uuid.UUID, kind string, state *store.CanvasState) error {
	switch kind {
	case "documents":
		docs, err := listDocuments(ctx, s.db, canvasID)
		if err != nil {
			return err
		}
		for _, d := range docs {
			state.Documents[d.ID.String()] = d
		}
	case "notes":
		notes, err := listNotes(ctx, s.db, canvasID, nil)
		if err != nil {
			return err
		}
		for _, n := range notes {
			state.Notes[n.ID.String()] = n
		}
	case "roadmapItems":
		items, err := listRoadmapItems(ctx, s.db, canvasID, "")
		if err != nil {
			return err
		}
		for _, r := range items {
			state.RoadmapItems[r.ID.String()] = r
		}
	case "actions":
		actions, err := listActions(ctx, s.db, canvasID, "", "", "")
		if err != nil {
			return err
		}
		for _, a := range actions {
			state.Actions[a.ID.String()] = a
		}
	case "agents":
		agents, err := listAgents(ctx, s.db, canvasID)
		if err != nil {
			return err
		}
		for _, a := range agents {
			state.Agents[a.ID.String()] = a
		}
	}
	return nil
}

// GetCanvasSummary loads the exact per-kind counts plus a capped, name-column
// Sample for the default canvas_state_read. Counts are authoritative; Sample is
// capped at sampleLimit and carries only the columns a name is rendered from.
func (s *Store) GetCanvasSummary(ctx context.Context, canvasID uuid.UUID, sampleLimit int) (*store.Canvas, *store.CanvasSummary, []*store.PendingEdit, error) {
	canvas, err := getCanvasByID(ctx, s.db, canvasID)
	if err != nil {
		return nil, nil, nil, err
	}
	sum := &store.CanvasSummary{
		Version:      canvas.Version,
		Mode:         canvas.Mode,
		EnabledModes: canvas.EnabledModes,
		Counts:       map[string]int{},
		Sample:       emptyCanvasState(canvas.Version, canvas.Mode, canvas.EnabledModes),
	}
	id := canvasID.String()
	count := func(key, table string) error {
		var n int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table+` WHERE canvas_id = ?`, id).Scan(&n); err != nil {
			return err
		}
		sum.Counts[key] = n
		return nil
	}
	for _, kt := range [][2]string{{"documents", "documents"}, {"notes", "notes"}, {"roadmapItems", "roadmap_items"}, {"actions", "actions"}, {"agents", "agents"}} {
		if err := count(kt[0], kt[1]); err != nil {
			return nil, nil, nil, err
		}
	}
	if sampleLimit < 0 {
		sampleLimit = 0
	}
	// documents: id, name, type
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, type FROM documents WHERE canvas_id = ? ORDER BY id LIMIT ?`, id, sampleLimit)
	if err != nil {
		return nil, nil, nil, err
	}
	for rows.Next() {
		var did, name, typ string
		if err := rows.Scan(&did, &name, &typ); err != nil {
			rows.Close()
			return nil, nil, nil, err
		}
		sum.Sample.Documents[did] = &store.Document{ID: mustUUID(did), Kind: "document", Name: name, Type: typ, Config: map[string]any{}}
	}
	rows.Close()
	// notes: id, body
	rows, err = s.db.QueryContext(ctx, `SELECT id, body FROM notes WHERE canvas_id = ? ORDER BY id LIMIT ?`, id, sampleLimit)
	if err != nil {
		return nil, nil, nil, err
	}
	for rows.Next() {
		var nid, body string
		if err := rows.Scan(&nid, &body); err != nil {
			rows.Close()
			return nil, nil, nil, err
		}
		sum.Sample.Notes[nid] = &store.Note{ID: mustUUID(nid), Kind: "note", Body: body, ImageRefs: []string{}}
	}
	rows.Close()
	// roadmap items: id, title
	rows, err = s.db.QueryContext(ctx, `SELECT id, title FROM roadmap_items WHERE canvas_id = ? ORDER BY id LIMIT ?`, id, sampleLimit)
	if err != nil {
		return nil, nil, nil, err
	}
	for rows.Next() {
		var rid, title string
		if err := rows.Scan(&rid, &title); err != nil {
			rows.Close()
			return nil, nil, nil, err
		}
		sum.Sample.RoadmapItems[rid] = &store.RoadmapItem{ID: mustUUID(rid), Kind: "roadmap", Title: title}
	}
	rows.Close()
	// agents: id, name
	rows, err = s.db.QueryContext(ctx, `SELECT id, name FROM agents WHERE canvas_id = ? ORDER BY id LIMIT ?`, id, sampleLimit)
	if err != nil {
		return nil, nil, nil, err
	}
	for rows.Next() {
		var aid, name string
		if err := rows.Scan(&aid, &name); err != nil {
			rows.Close()
			return nil, nil, nil, err
		}
		sum.Sample.Agents[aid] = &store.Agent{ID: mustUUID(aid), Kind: "agent", Name: name}
	}
	rows.Close()
	edits, err := s.ListPendingEdits(ctx, canvasID)
	if err != nil {
		return nil, nil, nil, err
	}
	return canvas, sum, edits, nil
}

// ── Modes ─────────────────────────────────────────────────────────────────────

func (s *Store) SetMode(ctx context.Context, canvasID uuid.UUID, mode string) (int, error) {
	return s.execThenBump(ctx, canvasID, `UPDATE canvases SET mode = ?, updated_at = ? WHERE id = ?`, mode, nowStr(), canvasID.String())
}

// EnableMode adds a mode to enabled_modes. Idempotent: already enabled → no
// write, no bump, current version returned.
func (s *Store) EnableMode(ctx context.Context, canvasID uuid.UUID, mode string) (int, error) {
	v := 0
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		var raw string
		var version int
		err := tx.QueryRowContext(ctx, `SELECT enabled_modes, version FROM canvases WHERE id = ?`, canvasID.String()).Scan(&raw, &version)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("canvas not found: %s", canvasID)
		}
		if err != nil {
			return err
		}
		modes := jsonStrings(raw)
		for _, m := range modes {
			if m == mode {
				v = version
				return nil
			}
		}
		modes = append(modes, mode)
		if _, err := tx.ExecContext(ctx, `UPDATE canvases SET enabled_modes = ?, updated_at = ? WHERE id = ?`, jsonText(modes), nowStr(), canvasID.String()); err != nil {
			return err
		}
		v, err = s.bumpVersion(ctx, tx, canvasID)
		return err
	})
	return v, err
}

// ApplyTemplate sets the active mode (the template picker's write).
func (s *Store) ApplyTemplate(ctx context.Context, canvasID uuid.UUID, mode string) (int, error) {
	return s.SetMode(ctx, canvasID, mode)
}

// LeaveWelcomeIfNeeded moves a canvas out of `welcome` after its first content
// write. Idempotent, no version bump — the caller's own bump carries it.
func (s *Store) LeaveWelcomeIfNeeded(ctx context.Context, canvasID uuid.UUID, fallbackMode string) error {
	return leaveWelcome(ctx, s.db, canvasID, fallbackMode)
}

func leaveWelcome(ctx context.Context, q dbtx, canvasID uuid.UUID, fallbackMode string) error {
	_, err := q.ExecContext(ctx, `UPDATE canvases SET mode = ?, updated_at = ? WHERE id = ? AND mode = 'welcome'`, fallbackMode, nowStr(), canvasID.String())
	return err
}
