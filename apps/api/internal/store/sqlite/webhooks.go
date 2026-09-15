package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/agentcanvas/api/internal/store"
)

// Outbound webhooks (migration 0038). SECRET HANDLING mirrors the PostgREST
// store: the read paths that back the UI select the secret only to derive
// SecretLastFour and scrub it before the row leaves; ListWebhooksForEvent and
// GetWebhookWithSecret — the emit/deliver path — are the only reads that return
// key material.

const webhookCols = `id, canvas_id, url, secret, events, enabled, name, description, created_by, created_at, updated_at`

func scanWebhook(r rowScanner) (*store.Webhook, error) {
	var w store.Webhook
	var id, canvasID, events, createdAt, updatedAt string
	var enabled int
	if err := r.Scan(&id, &canvasID, &w.URL, &w.Secret, &events, &enabled, &w.Name, &w.Description, &w.CreatedBy, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	w.ID = mustUUID(id)
	w.CanvasID = mustUUID(canvasID)
	w.SecretLastFour = store.LastFour(w.Secret)
	w.Events = jsonStrings(events)
	w.Enabled = enabled != 0
	w.CreatedAt = parseTime(createdAt)
	w.UpdatedAt = parseTime(updatedAt)
	return &w, nil
}

func scrubSecret(w *store.Webhook) *store.Webhook {
	if w != nil {
		w.Secret = ""
	}
	return w
}

func getWebhook(ctx context.Context, q dbtx, canvasID *uuid.UUID, id uuid.UUID) (*store.Webhook, error) {
	query := `SELECT ` + webhookCols + ` FROM webhooks WHERE id = ?`
	args := []any{id.String()}
	if canvasID != nil {
		query += ` AND canvas_id = ?`
		args = append(args, canvasID.String())
	}
	w, err := scanWebhook(q.QueryRowContext(ctx, query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, store.ErrWebhookNotFound
	}
	return w, err
}

func (s *Store) CreateWebhook(ctx context.Context, canvasID uuid.UUID, w *store.Webhook) (*store.Webhook, error) {
	events, err := store.NormalizeWebhookEvents(w.Events)
	if err != nil {
		return nil, err
	}
	createdBy := w.CreatedBy
	if createdBy == "" {
		createdBy = "user"
	}
	id := w.ID
	if id == uuid.Nil {
		id = uuid.New()
	}
	now := nowStr()
	if _, err := s.db.ExecContext(ctx, `INSERT INTO webhooks (id, canvas_id, url, secret, events, enabled, name, description, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id.String(), canvasID.String(), w.URL, w.Secret, jsonText(events), boolInt(w.Enabled), w.Name, w.Description, createdBy, now, now); err != nil {
		return nil, err
	}
	saved, err := getWebhook(ctx, s.db, &canvasID, id)
	if err != nil {
		return nil, err
	}
	return scrubSecret(saved), nil
}

func (s *Store) ListWebhooks(ctx context.Context, canvasID uuid.UUID) ([]*store.Webhook, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+webhookCols+` FROM webhooks WHERE canvas_id = ? ORDER BY created_at, id`, canvasID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*store.Webhook{}
	for rows.Next() {
		w, err := scanWebhook(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, scrubSecret(w))
	}
	return out, rows.Err()
}

func (s *Store) GetWebhook(ctx context.Context, canvasID, id uuid.UUID) (*store.Webhook, error) {
	w, err := getWebhook(ctx, s.db, &canvasID, id)
	if err != nil {
		return nil, err
	}
	return scrubSecret(w), nil
}

func (s *Store) UpdateWebhook(ctx context.Context, canvasID, id uuid.UUID, patch store.WebhookPatch) (*store.Webhook, error) {
	var c setClause
	if patch.URL != nil {
		c.add("url", *patch.URL)
	}
	if patch.Secret != nil {
		c.add("secret", *patch.Secret)
	}
	if patch.Events != nil {
		events, err := store.NormalizeWebhookEvents(*patch.Events)
		if err != nil {
			return nil, err
		}
		c.add("events", jsonText(events))
	}
	if patch.Enabled != nil {
		c.add("enabled", boolInt(*patch.Enabled))
	}
	if patch.Name != nil {
		c.add("name", *patch.Name)
	}
	if patch.Description != nil {
		c.add("description", *patch.Description)
	}
	if c.empty() {
		return s.GetWebhook(ctx, canvasID, id)
	}
	c.add("updated_at", nowStr())
	res, err := s.db.ExecContext(ctx, `UPDATE webhooks SET `+c.sql()+` WHERE id = ? AND canvas_id = ?`, append(c.args, id.String(), canvasID.String())...)
	if err != nil {
		return nil, err
	}
	if affected(res) == 0 {
		return nil, store.ErrWebhookNotFound
	}
	return s.GetWebhook(ctx, canvasID, id)
}

// DeleteWebhook removes the config; deliveries cascade with it.
func (s *Store) DeleteWebhook(ctx context.Context, canvasID, id uuid.UUID) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM webhooks WHERE id = ? AND canvas_id = ?`, id.String(), canvasID.String())
	if err != nil {
		return err
	}
	if affected(res) == 0 {
		return store.ErrWebhookNotFound
	}
	return nil
}

func (s *Store) CountWebhooks(ctx context.Context, canvasID uuid.UUID) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM webhooks WHERE canvas_id = ?`, canvasID.String()).Scan(&n)
	return n, err
}

// ListWebhooksForEvent is the emit-path read: enabled configs on the canvas
// whose events filter contains eventType. Returns key material.
func (s *Store) ListWebhooksForEvent(ctx context.Context, canvasID uuid.UUID, eventType string) ([]*store.Webhook, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+webhookCols+` FROM webhooks
		WHERE canvas_id = ? AND enabled = 1
		  AND EXISTS (SELECT 1 FROM json_each(webhooks.events) WHERE json_each.value = ?)
		ORDER BY created_at, id`, canvasID.String(), eventType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*store.Webhook{}
	for rows.Next() {
		w, err := scanWebhook(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// GetWebhookWithSecret is the delivery worker's lookup. Returns key material.
func (s *Store) GetWebhookWithSecret(ctx context.Context, id uuid.UUID) (*store.Webhook, error) {
	return getWebhook(ctx, s.db, nil, id)
}

// ── Delivery queue ────────────────────────────────────────────────────────────

const deliveryCols = `id, webhook_id, canvas_id, event_id, event_type, payload, status, attempt_count, last_attempt_at, next_attempt_at, response_status, response_body, error, created_at`

func scanDelivery(r rowScanner) (*store.WebhookDelivery, error) {
	var d store.WebhookDelivery
	var id, webhookID, canvasID, eventID, payload, nextAt, createdAt string
	var lastAt, body, errText sql.NullString
	var responseStatus sql.NullInt64
	if err := r.Scan(&id, &webhookID, &canvasID, &eventID, &d.EventType, &payload, &d.Status, &d.AttemptCount, &lastAt, &nextAt, &responseStatus, &body, &errText, &createdAt); err != nil {
		return nil, err
	}
	d.ID = mustUUID(id)
	d.WebhookID = mustUUID(webhookID)
	d.CanvasID = mustUUID(canvasID)
	d.EventID = mustUUID(eventID)
	d.Payload = json.RawMessage(payload)
	if len(d.Payload) == 0 {
		d.Payload = json.RawMessage("{}")
	}
	d.LastAttemptAt = parseTimePtr(lastAt)
	d.NextAttemptAt = parseTime(nextAt)
	d.ResponseStatus = intPtr(responseStatus)
	d.ResponseBody = body.String
	d.Error = errText.String
	d.CreatedAt = parseTime(createdAt)
	return &d, nil
}

func getDelivery(ctx context.Context, q dbtx, id uuid.UUID) (*store.WebhookDelivery, error) {
	return scanDelivery(q.QueryRowContext(ctx, `SELECT `+deliveryCols+` FROM webhook_deliveries WHERE id = ?`, id.String()))
}

// CreateWebhookDeliveries inserts the fan-out. UNIQUE(webhook_id, event_id) is
// the producer-side idempotency guarantee: a re-emit is DO NOTHING, so the count
// returned is the number of rows that were genuinely new.
func (s *Store) CreateWebhookDeliveries(ctx context.Context, deliveries []*store.WebhookDelivery) (int, error) {
	if len(deliveries) == 0 {
		return 0, nil
	}
	inserted := 0
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		now := nowStr()
		for _, d := range deliveries {
			payload := d.Payload
			if len(payload) == 0 {
				payload = json.RawMessage("{}")
			}
			id := d.ID
			if id == uuid.Nil {
				id = uuid.New()
			}
			res, err := tx.ExecContext(ctx, `INSERT INTO webhook_deliveries
				(id, webhook_id, canvas_id, event_id, event_type, payload, status, attempt_count, next_attempt_at, created_at)
				VALUES (?, ?, ?, ?, ?, ?, 'pending', 0, ?, ?)
				ON CONFLICT (webhook_id, event_id) DO NOTHING`,
				id.String(), d.WebhookID.String(), d.CanvasID.String(), d.EventID.String(), d.EventType, string(payload), now, now)
			if err != nil {
				return err
			}
			inserted += affected(res)
		}
		return nil
	})
	return inserted, err
}

// LeaseWebhookDeliveries claims up to limit due rows: the read picks the due
// ids oldest-first, the per-row UPDATE is the compare-and-swap on
// attempt_count, and the whole thing runs in one transaction on the store's
// single connection — so no two leasers can ever see the same row. Stamps
// last_attempt_at (the lease clock) and attempt_count+1.
func (s *Store) LeaseWebhookDeliveries(ctx context.Context, limit int) ([]*store.WebhookDelivery, error) {
	if limit <= 0 {
		return nil, nil
	}
	leased := []*store.WebhookDelivery{}
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		now := nowStr()
		rows, err := tx.QueryContext(ctx, `SELECT id, attempt_count FROM webhook_deliveries
			WHERE status IN ('pending', 'failed') AND next_attempt_at <= ? ORDER BY next_attempt_at, id LIMIT ?`, now, limit)
		if err != nil {
			return err
		}
		type due struct {
			id       string
			attempts int
		}
		var dues []due
		for rows.Next() {
			var d due
			if err := rows.Scan(&d.id, &d.attempts); err != nil {
				rows.Close()
				return err
			}
			dues = append(dues, d)
		}
		rows.Close()
		for _, d := range dues {
			res, err := tx.ExecContext(ctx, `UPDATE webhook_deliveries SET status = 'delivering', last_attempt_at = ?, attempt_count = ?
				WHERE id = ? AND status IN ('pending', 'failed') AND next_attempt_at <= ? AND attempt_count = ?`,
				now, d.attempts+1, d.id, now, d.attempts)
			if err != nil {
				return err
			}
			if affected(res) != 1 {
				continue
			}
			row, err := getDelivery(ctx, tx, mustUUID(d.id))
			if err != nil {
				return err
			}
			leased = append(leased, row)
		}
		return nil
	})
	if err != nil {
		return leased, err
	}
	return leased, nil
}

func (s *Store) MarkWebhookDeliveryOK(ctx context.Context, id uuid.UUID, res store.WebhookDeliveryResult) error {
	return s.finishDelivery(ctx, id, "ok", nil, res)
}

func (s *Store) MarkWebhookDeliveryFailed(ctx context.Context, id uuid.UUID, nextAttemptAt time.Time, res store.WebhookDeliveryResult) error {
	return s.finishDelivery(ctx, id, "failed", &nextAttemptAt, res)
}

func (s *Store) MarkWebhookDeliveryDead(ctx context.Context, id uuid.UUID, res store.WebhookDeliveryResult) error {
	return s.finishDelivery(ctx, id, "dead", nil, res)
}

// finishDelivery closes out a leased attempt. Scoped to status='delivering' so
// a reaped-and-relet row can't be stomped by the corpse of the attempt that
// abandoned it.
func (s *Store) finishDelivery(ctx context.Context, id uuid.UUID, status string, nextAttemptAt *time.Time, res store.WebhookDeliveryResult) error {
	var c setClause
	c.add("status", status)
	c.add("response_status", intArg(res.ResponseStatus))
	c.add("response_body", nullIfEmpty(res.ResponseBody))
	c.add("error", nullIfEmpty(res.Error))
	if nextAttemptAt != nil {
		c.add("next_attempt_at", fmtTime(*nextAttemptAt))
	}
	_, err := s.db.ExecContext(ctx, `UPDATE webhook_deliveries SET `+c.sql()+` WHERE id = ? AND status = 'delivering'`, append(c.args, id.String())...)
	return err
}

// ReapStuckWebhookDeliveries returns 'delivering' rows whose lease clock is
// older than olderThan to 'failed', immediately retryable.
func (s *Store) ReapStuckWebhookDeliveries(ctx context.Context, olderThan time.Duration) (int, error) {
	if olderThan <= 0 {
		return 0, nil
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `UPDATE webhook_deliveries SET status = 'failed', next_attempt_at = ?, error = ?
		WHERE status = 'delivering' AND last_attempt_at < ?`,
		fmtTime(now), "worker lease expired (no result recorded)", fmtTime(now.Add(-olderThan)))
	if err != nil {
		return 0, err
	}
	return affected(res), nil
}

func (s *Store) ListWebhookDeliveries(ctx context.Context, canvasID uuid.UUID, webhookID *uuid.UUID, status string, limit int) ([]*store.WebhookDelivery, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `SELECT ` + deliveryCols + ` FROM webhook_deliveries WHERE canvas_id = ?`
	args := []any{canvasID.String()}
	if webhookID != nil {
		query += ` AND webhook_id = ?`
		args = append(args, webhookID.String())
	}
	if status != "" {
		query += ` AND status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY created_at DESC, id LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*store.WebhookDelivery{}
	for rows.Next() {
		d, err := scanDelivery(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// RetryWebhookDelivery is the human "Retry" button: a failed or dead delivery
// goes back to 'pending' with a fresh attempt budget and the previous attempt's
// result cleared. Same delivery id, so receivers still dedupe it.
func (s *Store) RetryWebhookDelivery(ctx context.Context, canvasID, id uuid.UUID) (*store.WebhookDelivery, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE webhook_deliveries SET status = 'pending', attempt_count = 0, next_attempt_at = ?,
		error = NULL, response_status = NULL, response_body = NULL
		WHERE id = ? AND canvas_id = ? AND status IN ('failed', 'dead')`, nowStr(), id.String(), canvasID.String())
	if err != nil {
		return nil, err
	}
	if affected(res) == 0 {
		return nil, store.ErrWebhookDeliveryNotRetryable
	}
	return getDelivery(ctx, s.db, id)
}
