// Package sqlite is the local-first store.Store: one SQLite file (or :memory:)
// holding the board-core tables, via the pure-Go modernc.org/sqlite driver so
// the server cross-compiles with CGO_ENABLED=0.
//
// It reproduces the semantics of the PostgREST-backed store it replaces —
// version bumps, audit trails, lazy claim expiry with fencing generations, the
// content gate, rows-affected error mapping — with the four Postgres RPCs
// (bump_canvas_version, copy_canvas, reserve_task_tickets, …) rewritten as Go
// transactions.
//
// CONCURRENCY MODEL. The pool is pinned to ONE connection (SetMaxOpenConns(1)).
// SQLite has a single writer anyway; forcing every reader through the same
// connection means SQLITE_BUSY can never surface and every transaction is
// trivially serializable, which is what makes the conditional-UPDATE race
// guards below (claims, leases, takeovers) exact rather than merely likely.
// The one rule this imposes on the code: inside a transaction, use the *sql.Tx
// for every statement (a bare s.db call would wait forever for the connection
// the transaction holds), and always drain+close a *sql.Rows before issuing
// the next statement.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"github.com/agentcanvas/api/internal/store"
)

// timeLayout is the fixed-width timestamp format every column uses. RFC3339 with
// exactly nine fractional digits, always UTC, so that string comparison in SQL
// (claimed_at < cutoff, next_attempt_at <= now) orders exactly like time does.
// time.RFC3339Nano trims trailing zeros and would break that.
const timeLayout = "2006-01-02T15:04:05.000000000Z"

func fmtTime(t time.Time) string { return t.UTC().Format(timeLayout) }

func nowStr() string { return fmtTime(time.Now()) }

// parseTime accepts what this store writes plus the RFC3339 shapes an imported
// Postgres row may carry. Unparseable → zero time (callers map that to nil).
func parseTime(s string) time.Time {
	for _, layout := range []string{timeLayout, time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05.999999-07", "2006-01-02 15:04:05.999999"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

func parseTimePtr(ns sql.NullString) *time.Time {
	if !ns.Valid || ns.String == "" {
		return nil
	}
	t := parseTime(ns.String)
	if t.IsZero() {
		return nil
	}
	return &t
}

func timeArg(t *time.Time) any {
	if t == nil {
		return nil
	}
	return fmtTime(*t)
}

func strPtr(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	s := ns.String
	return &s
}

func strArg(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}

func intPtr(ni sql.NullInt64) *int {
	if !ni.Valid {
		return nil
	}
	n := int(ni.Int64)
	return &n
}

func intArg(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

func uuidPtr(ns sql.NullString) *uuid.UUID {
	if !ns.Valid || ns.String == "" {
		return nil
	}
	id, err := uuid.Parse(ns.String)
	if err != nil {
		return nil
	}
	return &id
}

func uuidArg(p *uuid.UUID) any {
	if p == nil {
		return nil
	}
	return p.String()
}

func mustUUID(s string) uuid.UUID {
	id, _ := uuid.Parse(s)
	return id
}

// jsonText renders v as the TEXT a JSON column stores; nil slices/maps render as
// their empty container rather than "null" so readers never see a JSON null.
func jsonText(v any) string {
	b, err := json.Marshal(v)
	if err != nil || string(b) == "null" {
		switch v.(type) {
		case []string, []uuid.UUID:
			return "[]"
		default:
			return "{}"
		}
	}
	return string(b)
}

func jsonStrings(raw string) []string {
	out := []string{}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &out)
	}
	if out == nil {
		out = []string{}
	}
	return out
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// isUniqueViolation detects SQLITE_CONSTRAINT_UNIQUE through the driver's error
// text — modernc surfaces it as "constraint failed: UNIQUE constraint failed:
// <table>.<column> (2067)". Optionally narrowed to one column.
func isUniqueViolation(err error, column string) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	if !strings.Contains(msg, "UNIQUE constraint failed") {
		return false
	}
	return column == "" || strings.Contains(msg, column)
}

// ── Store ─────────────────────────────────────────────────────────────────────

// Store is the SQLite-backed store.Store.
type Store struct {
	db *sql.DB
	// claimTTL: see store.DefaultClaimTTL. 0 disables claim expiry entirely.
	claimTTL time.Duration
}

// Option customizes Open.
type Option func(*Store)

// WithClaimTTL sets how long an executing claim is honored before a new claimer
// may take it over. 0 disables takeover.
func WithClaimTTL(d time.Duration) Option {
	return func(s *Store) { s.claimTTL = d }
}

// Open opens (creating if needed) the SQLite database at path and brings it to
// the current schema. ":memory:" gives a private in-memory database, which is
// what the tests use.
func Open(path string, opts ...Option) (*Store, error) {
	db, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		return nil, fmt.Errorf("sqlite open: %w", err)
	}
	// One connection, kept forever: see the package doc. MaxIdle=1 with no idle
	// timeout is what keeps a :memory: database alive between calls — a dropped
	// connection would be a dropped database.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	db.SetConnMaxIdleTime(0)

	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite ping: %w", err)
	}
	if err := migrate(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite migrate: %w", err)
	}
	s := &Store{db: db, claimTTL: store.DefaultClaimTTL}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

// dsn builds the driver DSN: the file (or :memory:) plus the per-connection
// pragmas. journal_mode=WAL is a no-op on :memory: (reports "memory"), so one
// DSN shape serves both.
func dsn(path string) string {
	pragmas := []string{
		"_pragma=busy_timeout(5000)",
		"_pragma=foreign_keys(1)",
		"_pragma=journal_mode(WAL)",
		"_pragma=synchronous(NORMAL)",
	}
	if path == ":memory:" || path == "" {
		return "file::memory:?" + strings.Join(pragmas, "&")
	}
	return "file:" + url.PathEscape(path) + "?" + strings.Join(pragmas, "&")
}

// Close releases the connection. Safe to call once; the store is unusable after.
func (s *Store) Close() { _ = s.db.Close() }

// DB exposes the underlying handle for tooling that needs raw access (the
// Supabase → local import command). Application code goes through Store.
func (s *Store) DB() *sql.DB { return s.db }

// dbtx is what both *sql.DB and *sql.Tx satisfy; every internal helper takes
// one so the same code runs inside and outside a transaction.
type dbtx interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// withTx runs fn in a transaction, committing on nil and rolling back on error
// or panic.
func (s *Store) withTx(ctx context.Context, fn func(tx *sql.Tx) error) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if r := recover(); r != nil {
			_ = tx.Rollback()
			panic(r)
		}
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func affected(res sql.Result) int {
	n, err := res.RowsAffected()
	if err != nil {
		return 0
	}
	return int(n)
}

// ── Version bumps ─────────────────────────────────────────────────────────────

// BumpCanvasVersion increments the canvas version once. It ALWAYS bumps (never
// consults the skip flag), so the batch path can suppress per-item bumps and
// still bump once at the end.
func (s *Store) BumpCanvasVersion(ctx context.Context, canvasID uuid.UUID) (int, error) {
	return doBump(ctx, s.db, canvasID)
}

// bumpVersion is the per-write bump: a no-op (0, nil) on a WithoutVersionBump
// context, the real thing otherwise.
func (s *Store) bumpVersion(ctx context.Context, q dbtx, canvasID uuid.UUID) (int, error) {
	if store.ShouldSkipVersionBump(ctx) {
		return 0, nil
	}
	return doBump(ctx, q, canvasID)
}

// doBump is the bump_canvas_version RPC (migration 0003) as one statement.
func doBump(ctx context.Context, q dbtx, canvasID uuid.UUID) (int, error) {
	var v int
	err := q.QueryRowContext(ctx,
		`UPDATE canvases SET version = version + 1, updated_at = ? WHERE id = ? RETURNING version`,
		nowStr(), canvasID.String()).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("bumpVersion: canvas not found: %s", canvasID)
	}
	if err != nil {
		return 0, fmt.Errorf("bumpVersion: %w", err)
	}
	return v, nil
}

// affectedThenBump runs one scoped UPDATE/DELETE, bumps the canvas version, and
// returns the TRUE rows-affected — 0 means the id matched nothing IN THIS CANVAS.
func (s *Store) affectedThenBump(ctx context.Context, canvasID uuid.UUID, query string, args ...any) (int, error) {
	n := 0
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return err
		}
		n = affected(res)
		_, err = s.bumpVersion(ctx, tx, canvasID)
		return err
	})
	return n, err
}

// execThenBump runs one statement (no rows-affected contract) and bumps,
// returning the new version.
func (s *Store) execThenBump(ctx context.Context, canvasID uuid.UUID, query string, args ...any) (int, error) {
	v := 0
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			return err
		}
		var err error
		v, err = s.bumpVersion(ctx, tx, canvasID)
		return err
	})
	return v, err
}

// ── Audit carry (mirror of the store package's unexported carryContentAudit) ──

// storedAudit reads the audit log off a stored payload.
func storedAudit(stored json.RawMessage) []store.ContentAudit {
	var p struct {
		Audit []store.ContentAudit `json:"audit"`
	}
	if len(stored) == 0 || json.Unmarshal(stored, &p) != nil {
		return nil
	}
	return p.Audit
}

// carryContentAudit builds the payload to store from an incoming payload: the
// three server-owned keys (claim record, contention trail, audit log) are taken
// from the STORED row whatever the caller sent, entry (if any) is appended to
// the audit log, and the log is capped at store.MaxContentAudit. Same rules as
// the store package's unexported helper, which ResubmitAction needs with a
// payload that differs from the stored one (store.AppendAudit only covers the
// incoming == stored case).
func carryContentAudit(incoming, stored json.RawMessage, entry *store.ContentAudit) (json.RawMessage, error) {
	p := map[string]any{}
	if len(incoming) > 0 {
		if err := json.Unmarshal(incoming, &p); err != nil {
			return incoming, nil
		}
	}
	if rec := store.ReadClaimRecord(stored); rec.Generation > 0 {
		p[store.ClaimRecordKey] = rec
	} else {
		delete(p, store.ClaimRecordKey)
	}
	if trail := store.ReadContention(stored); len(trail) > 0 {
		p[store.ContentionKey] = trail
	} else {
		delete(p, store.ContentionKey)
	}
	history := storedAudit(stored)
	if entry != nil {
		history = append(history, *entry)
	}
	if len(history) > store.MaxContentAudit {
		history = history[len(history)-store.MaxContentAudit:]
	}
	if len(history) == 0 {
		delete(p, "audit")
	} else {
		p["audit"] = history
	}
	return json.Marshal(p)
}

// Compile-time proof that *Store implements the full store.Store surface.
var _ store.Store = (*Store)(nil)
