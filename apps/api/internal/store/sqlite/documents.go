package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/agentcanvas/api/internal/store"
)

// ── Documents (migration 0024) ────────────────────────────────────────────────

const documentCols = `id, type, name, parent_id, sort_order, config, created_by, authored_by, updated_at, verified_at, stale_after_seconds`

func scanDocument(r rowScanner) (*store.Document, error) {
	var (
		d                              store.Document
		id, config, updatedAt          string
		parent, authoredBy, verifiedAt sql.NullString
		staleAfter                     sql.NullInt64
	)
	if err := r.Scan(&id, &d.Type, &d.Name, &parent, &d.SortOrder, &config, &d.CreatedBy, &authoredBy, &updatedAt, &verifiedAt, &staleAfter); err != nil {
		return nil, err
	}
	d.ID = mustUUID(id)
	d.Kind = "document"
	d.ParentID = uuidPtr(parent)
	d.AuthoredBy = strPtr(authoredBy)
	d.UpdatedAt = parseTime(updatedAt)
	d.VerifiedAt = parseTimePtr(verifiedAt)
	d.StaleAfterSeconds = intPtr(staleAfter)
	d.Config = map[string]any{}
	if config != "" {
		_ = json.Unmarshal([]byte(config), &d.Config)
	}
	if d.Config == nil {
		d.Config = map[string]any{}
	}
	return &d, nil
}

func listDocuments(ctx context.Context, q dbtx, canvasID uuid.UUID) ([]*store.Document, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+documentCols+` FROM documents WHERE canvas_id = ? ORDER BY sort_order, created_at, id`, canvasID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*store.Document{}
	for rows.Next() {
		d, err := scanDocument(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func insertDocument(ctx context.Context, q dbtx, canvasID uuid.UUID, d *store.Document, now string) error {
	if d.ID == uuid.Nil {
		d.ID = uuid.New()
	}
	if d.Config == nil {
		d.Config = map[string]any{}
	}
	if d.CreatedBy == "" {
		d.CreatedBy = "agent"
	}
	_, err := q.ExecContext(ctx, `INSERT INTO documents
		(id, canvas_id, type, name, parent_id, sort_order, config, created_by, authored_by, verified_at, stale_after_seconds, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID.String(), canvasID.String(), d.Type, d.Name, uuidArg(d.ParentID), d.SortOrder, jsonText(d.Config), d.CreatedBy,
		strArg(d.AuthoredBy), timeArg(d.VerifiedAt), intArg(d.StaleAfterSeconds), now, now)
	return err
}

func (s *Store) CreateDocument(ctx context.Context, canvasID uuid.UUID, d *store.Document) (int, error) {
	return s.CreateDocuments(ctx, canvasID, []*store.Document{d})
}

// CreateDocuments inserts many documents in ONE transaction with ONE version
// bump. Returns the new canvas version.
func (s *Store) CreateDocuments(ctx context.Context, canvasID uuid.UUID, docs []*store.Document) (int, error) {
	if len(docs) == 0 {
		return 0, nil
	}
	v := 0
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		now := nowStr()
		for _, d := range docs {
			d.UpdatedAt = parseTime(now)
			if err := insertDocument(ctx, tx, canvasID, d, now); err != nil {
				return err
			}
		}
		var err error
		v, err = s.bumpVersion(ctx, tx, canvasID)
		return err
	})
	return v, err
}

// setClause accumulates "col = ?" pairs for a dynamic UPDATE.
type setClause struct {
	cols []string
	args []any
}

func (c *setClause) add(col string, v any) {
	c.cols = append(c.cols, col+" = ?")
	c.args = append(c.args, v)
}

func (c *setClause) empty() bool { return len(c.cols) == 0 }

func (c *setClause) sql() string { return strings.Join(c.cols, ", ") }

// applyFreshnessPatch folds a FreshnessPatch into the SET clause. A Clear flag
// wins over a value; absent both, the column is left alone.
func applyFreshnessPatch(c *setClause, f store.FreshnessPatch) {
	switch {
	case f.ClearVerifiedAt:
		c.add("verified_at", nil)
	case f.VerifiedAt != nil:
		c.add("verified_at", fmtTime(*f.VerifiedAt))
	}
	switch {
	case f.ClearStaleAfterSeconds:
		c.add("stale_after_seconds", nil)
	case f.StaleAfterSeconds != nil:
		c.add("stale_after_seconds", *f.StaleAfterSeconds)
	}
}

func (s *Store) UpdateDocument(ctx context.Context, canvasID uuid.UUID, id uuid.UUID, patch store.DocumentPatch) (int, error) {
	var c setClause
	if patch.Name != nil {
		c.add("name", *patch.Name)
	}
	if patch.SortOrder != nil {
		c.add("sort_order", *patch.SortOrder)
	}
	if patch.Config != nil {
		c.add("config", jsonText(patch.Config))
	}
	if patch.SetParent {
		c.add("parent_id", uuidArg(patch.ParentID))
	}
	applyFreshnessPatch(&c, patch.FreshnessPatch)
	if c.empty() {
		return 0, nil
	}
	c.add("updated_at", nowStr())
	return s.affectedThenBump(ctx, canvasID, `UPDATE documents SET `+c.sql()+` WHERE id = ? AND canvas_id = ?`,
		append(c.args, id.String(), canvasID.String())...)
}

func (s *Store) DeleteDocument(ctx context.Context, canvasID uuid.UUID, id uuid.UUID) (int, error) {
	return s.affectedThenBump(ctx, canvasID, `DELETE FROM documents WHERE id = ? AND canvas_id = ?`, id.String(), canvasID.String())
}

func (s *Store) ReorderDocuments(ctx context.Context, canvasID uuid.UUID, updates []store.DocumentReorder) (int, error) {
	v := 0
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		now := nowStr()
		for _, u := range updates {
			if _, err := tx.ExecContext(ctx, `UPDATE documents SET sort_order = ?, parent_id = ?, updated_at = ? WHERE id = ? AND canvas_id = ?`,
				u.SortOrder, uuidArg(u.ParentID), now, u.ID.String(), canvasID.String()); err != nil {
				return err
			}
		}
		var err error
		v, err = s.bumpVersion(ctx, tx, canvasID)
		return err
	})
	return v, err
}

func (s *Store) ListDocuments(ctx context.Context, canvasID uuid.UUID) ([]*store.Document, error) {
	return listDocuments(ctx, s.db, canvasID)
}

func (s *Store) GetDocument(ctx context.Context, canvasID, id uuid.UUID) (*store.Document, error) {
	d, err := scanDocument(s.db.QueryRowContext(ctx, `SELECT `+documentCols+` FROM documents WHERE id = ? AND canvas_id = ?`, id.String(), canvasID.String()))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("document %s not found in canvas %s", id, canvasID)
	}
	return d, err
}

// ── Notes ─────────────────────────────────────────────────────────────────────

const noteCols = `id, document_id, body, image_refs, parent_id, parent_kind, sort_order, created_by, authored_by, updated_at, verified_at, stale_after_seconds`

func scanNote(r rowScanner) (*store.Note, error) {
	var (
		n                                                   store.Note
		id, imageRefs, updatedAt                            string
		docID, parentID, parentKind, authoredBy, verifiedAt sql.NullString
		staleAfter                                          sql.NullInt64
	)
	if err := r.Scan(&id, &docID, &n.Body, &imageRefs, &parentID, &parentKind, &n.SortOrder, &n.CreatedBy, &authoredBy, &updatedAt, &verifiedAt, &staleAfter); err != nil {
		return nil, err
	}
	n.ID = mustUUID(id)
	n.Kind = "note"
	n.DocumentID = uuidPtr(docID)
	n.ImageRefs = jsonStrings(imageRefs)
	n.ParentID = uuidPtr(parentID)
	n.ParentKind = strPtr(parentKind)
	n.AuthoredBy = strPtr(authoredBy)
	n.UpdatedAt = parseTime(updatedAt)
	n.VerifiedAt = parseTimePtr(verifiedAt)
	n.StaleAfterSeconds = intPtr(staleAfter)
	return &n, nil
}

// listNotes lists a canvas's notes, optionally scoped to one document, in
// sort_order.
func listNotes(ctx context.Context, q dbtx, canvasID uuid.UUID, documentID *uuid.UUID) ([]*store.Note, error) {
	query := `SELECT ` + noteCols + ` FROM notes WHERE canvas_id = ?`
	args := []any{canvasID.String()}
	if documentID != nil {
		query += ` AND document_id = ?`
		args = append(args, documentID.String())
	}
	query += ` ORDER BY sort_order, created_at, id`
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*store.Note{}
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// nextNoteSortOrder is max(sort_order)+1 among the notes of one document (or
// the document-less bucket), so every creation path appends.
func nextNoteSortOrder(ctx context.Context, q dbtx, canvasID uuid.UUID, documentID *uuid.UUID) (int, error) {
	var maxSort sql.NullInt64
	var err error
	if documentID != nil {
		err = q.QueryRowContext(ctx, `SELECT MAX(sort_order) FROM notes WHERE canvas_id = ? AND document_id = ?`, canvasID.String(), documentID.String()).Scan(&maxSort)
	} else {
		err = q.QueryRowContext(ctx, `SELECT MAX(sort_order) FROM notes WHERE canvas_id = ? AND document_id IS NULL`, canvasID.String()).Scan(&maxSort)
	}
	if err != nil {
		return 0, err
	}
	if !maxSort.Valid {
		return 0, nil
	}
	return int(maxSort.Int64) + 1, nil
}

func insertNote(ctx context.Context, q dbtx, canvasID uuid.UUID, n *store.Note, now string) error {
	if n.ID == uuid.Nil {
		n.ID = uuid.New()
	}
	if n.ImageRefs == nil {
		n.ImageRefs = []string{}
	}
	if n.CreatedBy == "" {
		n.CreatedBy = "agent"
	}
	_, err := q.ExecContext(ctx, `INSERT INTO notes
		(id, canvas_id, document_id, body, image_refs, parent_id, parent_kind, sort_order, created_by, authored_by, verified_at, stale_after_seconds, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		n.ID.String(), canvasID.String(), uuidArg(n.DocumentID), n.Body, jsonText(n.ImageRefs), uuidArg(n.ParentID), strArg(n.ParentKind),
		n.SortOrder, n.CreatedBy, strArg(n.AuthoredBy), timeArg(n.VerifiedAt), intArg(n.StaleAfterSeconds), now, now)
	return err
}

func (s *Store) CreateNote(ctx context.Context, canvasID uuid.UUID, n *store.Note) (int, error) {
	return s.CreateNotes(ctx, canvasID, []*store.Note{n})
}

// CreateNotes inserts many notes in ONE transaction: sort orders append after
// the current max per target document, one welcome-transition, one bump.
func (s *Store) CreateNotes(ctx context.Context, canvasID uuid.UUID, notes []*store.Note) (int, error) {
	if len(notes) == 0 {
		return 0, nil
	}
	v := 0
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		now := nowStr()
		nextSort := map[string]int{}
		for _, n := range notes {
			n.UpdatedAt = parseTime(now)
			key := ""
			if n.DocumentID != nil {
				key = n.DocumentID.String()
			}
			start, ok := nextSort[key]
			if !ok {
				var err error
				if start, err = nextNoteSortOrder(ctx, tx, canvasID, n.DocumentID); err != nil {
					return err
				}
			}
			n.SortOrder = start
			nextSort[key] = start + 1
			if err := insertNote(ctx, tx, canvasID, n, now); err != nil {
				return err
			}
		}
		if err := leaveWelcome(ctx, tx, canvasID, "docs"); err != nil {
			return err
		}
		var err error
		v, err = s.bumpVersion(ctx, tx, canvasID)
		return err
	})
	return v, err
}

func (s *Store) UpdateNote(ctx context.Context, canvasID uuid.UUID, id uuid.UUID, patch store.NotePatch) (int, error) {
	var c setClause
	if patch.Body != nil {
		c.add("body", *patch.Body)
	}
	if patch.ImageRefs != nil {
		c.add("image_refs", jsonText(patch.ImageRefs))
	}
	if patch.ParentKind != nil {
		c.add("parent_kind", *patch.ParentKind)
	}
	if patch.ParentID != nil {
		c.add("parent_id", patch.ParentID.String())
	}
	if patch.SortOrder != nil {
		c.add("sort_order", *patch.SortOrder)
	}
	applyFreshnessPatch(&c, patch.FreshnessPatch)
	if c.empty() {
		return 0, nil
	}
	c.add("updated_at", nowStr())
	return s.affectedThenBump(ctx, canvasID, `UPDATE notes SET `+c.sql()+` WHERE id = ? AND canvas_id = ?`,
		append(c.args, id.String(), canvasID.String())...)
}

func (s *Store) DeleteNote(ctx context.Context, canvasID uuid.UUID, id uuid.UUID) (int, error) {
	return s.affectedThenBump(ctx, canvasID, `DELETE FROM notes WHERE id = ? AND canvas_id = ?`, id.String(), canvasID.String())
}

func (s *Store) ListNotesByDocument(ctx context.Context, canvasID, documentID uuid.UUID) ([]*store.Note, error) {
	return listNotes(ctx, s.db, canvasID, &documentID)
}

// ── Roadmap items ─────────────────────────────────────────────────────────────

const roadmapCols = `id, document_id, parent_id, title, body, status, stage, assignee, sort_order, created_by, updated_at, verified_at, stale_after_seconds`

func scanRoadmapItem(r rowScanner) (*store.RoadmapItem, error) {
	var (
		it                                           store.RoadmapItem
		id, updatedAt                                string
		docID, parentID, stage, assignee, verifiedAt sql.NullString
		staleAfter                                   sql.NullInt64
	)
	if err := r.Scan(&id, &docID, &parentID, &it.Title, &it.Body, &it.Status, &stage, &assignee, &it.SortOrder, &it.CreatedBy, &updatedAt, &verifiedAt, &staleAfter); err != nil {
		return nil, err
	}
	it.ID = mustUUID(id)
	it.Kind = "roadmap"
	it.DocumentID = uuidPtr(docID)
	it.ParentID = uuidPtr(parentID)
	it.Stage = stage.String
	it.Assignee = assignee.String
	it.UpdatedAt = parseTime(updatedAt)
	it.VerifiedAt = parseTimePtr(verifiedAt)
	it.StaleAfterSeconds = intPtr(staleAfter)
	return &it, nil
}

func listRoadmapItems(ctx context.Context, q dbtx, canvasID uuid.UUID, assignee string) ([]*store.RoadmapItem, error) {
	query := `SELECT ` + roadmapCols + ` FROM roadmap_items WHERE canvas_id = ?`
	args := []any{canvasID.String()}
	if assignee != "" {
		query += ` AND assignee = ?`
		args = append(args, assignee)
	}
	query += ` ORDER BY sort_order, created_at, id`
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*store.RoadmapItem{}
	for rows.Next() {
		it, err := scanRoadmapItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func insertRoadmapItem(ctx context.Context, q dbtx, canvasID uuid.UUID, r *store.RoadmapItem, now string) error {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	if r.Status == "" {
		r.Status = "todo"
	}
	if r.CreatedBy == "" {
		r.CreatedBy = "agent"
	}
	_, err := q.ExecContext(ctx, `INSERT INTO roadmap_items
		(id, canvas_id, document_id, parent_id, title, body, status, stage, assignee, sort_order, created_by, verified_at, stale_after_seconds, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID.String(), canvasID.String(), uuidArg(r.DocumentID), uuidArg(r.ParentID), r.Title, r.Body, r.Status,
		nullIfEmpty(r.Stage), nullIfEmpty(r.Assignee), r.SortOrder, r.CreatedBy, timeArg(r.VerifiedAt), intArg(r.StaleAfterSeconds), now, now)
	return err
}

func (s *Store) CreateRoadmapItem(ctx context.Context, canvasID uuid.UUID, r *store.RoadmapItem) (int, error) {
	return s.CreateRoadmapItems(ctx, canvasID, []*store.RoadmapItem{r})
}

func (s *Store) CreateRoadmapItems(ctx context.Context, canvasID uuid.UUID, items []*store.RoadmapItem) (int, error) {
	if len(items) == 0 {
		return 0, nil
	}
	v := 0
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		now := nowStr()
		for _, r := range items {
			r.UpdatedAt = parseTime(now)
			if err := insertRoadmapItem(ctx, tx, canvasID, r, now); err != nil {
				return err
			}
		}
		if err := leaveWelcome(ctx, tx, canvasID, "roadmap"); err != nil {
			return err
		}
		var err error
		v, err = s.bumpVersion(ctx, tx, canvasID)
		return err
	})
	return v, err
}

func (s *Store) UpdateRoadmapItem(ctx context.Context, canvasID uuid.UUID, id uuid.UUID, patch store.RoadmapItemPatch) (int, error) {
	var c setClause
	if patch.Title != nil {
		c.add("title", *patch.Title)
	}
	if patch.Body != nil {
		c.add("body", *patch.Body)
	}
	if patch.Status != nil {
		c.add("status", *patch.Status)
	}
	if patch.SortOrder != nil {
		c.add("sort_order", *patch.SortOrder)
	}
	if patch.ParentID != nil {
		c.add("parent_id", patch.ParentID.String())
	}
	if patch.Stage != nil {
		c.add("stage", nullIfEmpty(*patch.Stage))
	}
	if patch.Assignee != nil {
		if *patch.Assignee == "agent" {
			c.add("assignee", "agent")
		} else {
			c.add("assignee", nil)
		}
	}
	applyFreshnessPatch(&c, patch.FreshnessPatch)
	if c.empty() {
		return 0, nil
	}
	c.add("updated_at", nowStr())
	return s.affectedThenBump(ctx, canvasID, `UPDATE roadmap_items SET `+c.sql()+` WHERE id = ? AND canvas_id = ?`,
		append(c.args, id.String(), canvasID.String())...)
}

func (s *Store) DeleteRoadmapItem(ctx context.Context, canvasID uuid.UUID, id uuid.UUID) (int, error) {
	return s.affectedThenBump(ctx, canvasID, `DELETE FROM roadmap_items WHERE id = ? AND canvas_id = ?`, id.String(), canvasID.String())
}

func (s *Store) ReorderRoadmapItems(ctx context.Context, canvasID uuid.UUID, updates []store.RoadmapReorder) (int, error) {
	v := 0
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		now := nowStr()
		for _, u := range updates {
			if _, err := tx.ExecContext(ctx, `UPDATE roadmap_items SET sort_order = ?, parent_id = ?, updated_at = ? WHERE id = ? AND canvas_id = ?`,
				u.SortOrder, uuidArg(u.ParentID), now, u.ID.String(), canvasID.String()); err != nil {
				return err
			}
		}
		var err error
		v, err = s.bumpVersion(ctx, tx, canvasID)
		return err
	})
	return v, err
}

func (s *Store) ListRoadmapItems(ctx context.Context, canvasID uuid.UUID, assignee string) ([]*store.RoadmapItem, error) {
	return listRoadmapItems(ctx, s.db, canvasID, assignee)
}

// GetLinkedEntities resolves a task's payload.linkedIds against roadmap items
// and notes. Unknown ids are skipped, not errors.
func (s *Store) GetLinkedEntities(ctx context.Context, canvasID uuid.UUID, ids []uuid.UUID) ([]store.TaskLink, error) {
	if len(ids) == 0 {
		return []store.TaskLink{}, nil
	}
	placeholders, args := inArgs(ids)
	links := []store.TaskLink{}

	rows, err := s.db.QueryContext(ctx, `SELECT id, title, body, status, verified_at, stale_after_seconds FROM roadmap_items
		WHERE canvas_id = ? AND id IN (`+placeholders+`)`, append([]any{canvasID.String()}, args...)...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, title, body, status string
		var verifiedAt sql.NullString
		var stale sql.NullInt64
		if err := rows.Scan(&id, &title, &body, &status, &verifiedAt, &stale); err != nil {
			rows.Close()
			return nil, err
		}
		links = append(links, store.TaskLink{ID: mustUUID(id), Kind: "roadmap", Title: title, Body: body, Status: status,
			VerifiedAt: parseTimePtr(verifiedAt), StaleAfterSeconds: intPtr(stale)})
	}
	rows.Close()

	rows, err = s.db.QueryContext(ctx, `SELECT id, body, verified_at, stale_after_seconds FROM notes
		WHERE canvas_id = ? AND id IN (`+placeholders+`)`, append([]any{canvasID.String()}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, body string
		var verifiedAt sql.NullString
		var stale sql.NullInt64
		if err := rows.Scan(&id, &body, &verifiedAt, &stale); err != nil {
			return nil, err
		}
		links = append(links, store.TaskLink{ID: mustUUID(id), Kind: "note", Body: body,
			VerifiedAt: parseTimePtr(verifiedAt), StaleAfterSeconds: intPtr(stale)})
	}
	return links, rows.Err()
}

// inArgs renders "?, ?, ?" plus the matching args for an IN (...) list.
func inArgs(ids []uuid.UUID) (string, []any) {
	ph := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		ph[i] = "?"
		args[i] = id.String()
	}
	return strings.Join(ph, ", "), args
}

// ── Pending edits ─────────────────────────────────────────────────────────────

func (s *Store) CreatePendingEdit(ctx context.Context, canvasID, entityID uuid.UUID, instruction string) (*store.PendingEdit, error) {
	pe := &store.PendingEdit{ID: uuid.New(), EntityID: entityID, Instruction: instruction, CreatedAt: parseTime(nowStr())}
	_, err := s.db.ExecContext(ctx, `INSERT INTO pending_edits (id, canvas_id, entity_id, instruction, created_at) VALUES (?, ?, ?, ?, ?)`,
		pe.ID.String(), canvasID.String(), entityID.String(), instruction, fmtTime(pe.CreatedAt))
	if err != nil {
		return nil, err
	}
	return pe, nil
}

func (s *Store) DeletePendingEdit(ctx context.Context, canvasID, id uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM pending_edits WHERE id = ? AND canvas_id = ?`, id.String(), canvasID.String())
	return err
}

func (s *Store) ListPendingEdits(ctx context.Context, canvasID uuid.UUID) ([]*store.PendingEdit, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, entity_id, instruction, created_at FROM pending_edits WHERE canvas_id = ? ORDER BY created_at, id`, canvasID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*store.PendingEdit{}
	for rows.Next() {
		var id, entityID, instruction, createdAt string
		if err := rows.Scan(&id, &entityID, &instruction, &createdAt); err != nil {
			return nil, err
		}
		out = append(out, &store.PendingEdit{ID: mustUUID(id), EntityID: mustUUID(entityID), Instruction: instruction, CreatedAt: parseTime(createdAt)})
	}
	return out, rows.Err()
}
