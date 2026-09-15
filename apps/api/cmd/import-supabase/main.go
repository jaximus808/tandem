// import-supabase copies the boards from the old hosted Tandem (a Supabase
// project) into a local SQLite database, so the move to the local-first build
// loses nothing: canvases keep their codes, tasks keep their ticket numbers,
// documents and notes keep their ids.
//
//	cd apps/api && SUPABASE_URL=https://<ref>.supabase.co SUPABASE_KEY=<service_role> \
//	  go run ./cmd/import-supabase [--db ~/.tandem/tandem.db] [--canvas CODE] [--dry-run]
//
// It talks plain PostgREST (no Supabase client), pages through each surviving
// table, and inserts rows column-by-column where the local schema has a column
// of the same name — the two schemas were written to match, so the only work is
// type conversion (timestamps, jsonb, arrays, booleans) and the local-mode
// remaps: every canvas becomes owned by the local user, and legacy modes
// (map / itinerary / sheets / charts) collapse to what the local build has.
//
// Idempotent: rows are INSERT OR REPLACEd by primary key, so re-running after a
// partial import, or after the hosted board moved on, just brings the local copy
// up to date. Multi-user tables (users, canvas_access, notifications, PATs,
// OAuth) and delivery history are deliberately not imported.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/agentcanvas/api/internal/api"
	"github.com/agentcanvas/api/internal/store/sqlite"
)

// tables lists what is imported, in foreign-key order.
var tables = []string{"canvases", "documents", "notes", "roadmap_items", "agents", "actions", "webhooks"}

// legacyModes are canvas modes the local build no longer has.
var allowedModes = map[string]bool{"welcome": true, "docs": true, "roadmap": true}

func main() {
	var (
		dbPath  = flag.String("db", "", "SQLite file to import into (default $TANDEM_DATA_DIR/tandem.db, i.e. ~/.tandem/tandem.db)")
		code    = flag.String("canvas", "", "import only the canvas with this 8-char code (default: every canvas)")
		dryRun  = flag.Bool("dry-run", false, "fetch and convert everything, report counts, write nothing")
		pageSz  = flag.Int("page", 1000, "PostgREST page size")
		timeout = flag.Duration("timeout", 60*time.Second, "per-request timeout")
	)
	flag.Parse()

	src := strings.TrimRight(os.Getenv("SUPABASE_URL"), "/")
	key := os.Getenv("SUPABASE_KEY")
	if src == "" || key == "" {
		fail("SUPABASE_URL and SUPABASE_KEY (the service_role key) must be set — they are the OLD hosted project's credentials, found in its dashboard under Project Settings → API")
	}
	if *dbPath == "" {
		*dbPath = defaultDBPath()
	}

	imp := &importer{
		src:    &postgrest{base: src, key: key, http: &http.Client{Timeout: *timeout}, page: *pageSz},
		code:   strings.ToUpper(strings.TrimSpace(*code)),
		dryRun: *dryRun,
		log:    func(format string, a ...any) { fmt.Fprintf(os.Stderr, format+"\n", a...) },
	}
	st, err := sqlite.Open(*dbPath)
	if err != nil {
		fail("open %s: %v", *dbPath, err)
	}
	defer st.Close()

	report, err := imp.run(context.Background(), st)
	if err != nil {
		fail("%v", err)
	}
	for _, t := range tables {
		fmt.Printf("%-14s %6d rows\n", t, report[t])
	}
	if *dryRun {
		fmt.Println("dry run — nothing written")
	} else {
		fmt.Printf("imported into %s\n", *dbPath)
	}
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "import-supabase: "+format+"\n", a...)
	os.Exit(1)
}

func defaultDBPath() string {
	dir := os.Getenv("TANDEM_DATA_DIR")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fail("no home directory: set --db")
		}
		dir = filepath.Join(home, ".tandem")
	}
	return filepath.Join(dir, "tandem.db")
}

// ── PostgREST source ─────────────────────────────────────────────────────────

type rowSource interface {
	// rows fetches every row of table matching the optional filter
	// ("canvas_id=eq.<uuid>"), as generic JSON objects.
	rows(ctx context.Context, table, filter string) ([]map[string]any, error)
}

type postgrest struct {
	base string
	key  string
	http *http.Client
	page int
}

func (p *postgrest) rows(ctx context.Context, table, filter string) ([]map[string]any, error) {
	var all []map[string]any
	for from := 0; ; from += p.page {
		q := url.Values{}
		q.Set("select", "*")
		q.Set("order", "id.asc")
		u := p.base + "/rest/v1/" + table + "?" + q.Encode()
		if filter != "" {
			u += "&" + filter
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("apikey", p.key)
		req.Header.Set("Authorization", "Bearer "+p.key)
		req.Header.Set("Range-Unit", "items")
		req.Header.Set("Range", fmt.Sprintf("%d-%d", from, from+p.page-1))
		res, err := p.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", table, err)
		}
		body, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", table, err)
		}
		if res.StatusCode == http.StatusRequestedRangeNotSatisfiable {
			break // past the end
		}
		if res.StatusCode < 200 || res.StatusCode > 299 {
			return nil, fmt.Errorf("%s: HTTP %d: %s", table, res.StatusCode, strings.TrimSpace(string(body)))
		}
		var page []map[string]any
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("%s: decode: %w", table, err)
		}
		all = append(all, page...)
		if len(page) < p.page {
			break
		}
	}
	return all, nil
}

// ── Importer ─────────────────────────────────────────────────────────────────

type importer struct {
	src    rowSource
	code   string
	dryRun bool
	log    func(string, ...any)
}

// run copies every table in order inside ONE transaction (rolled back on
// --dry-run), with foreign-key checks deferred to commit so a child row that
// pages in before its parent is not an error.
func (imp *importer) run(ctx context.Context, st *sqlite.Store) (map[string]int, error) {
	// The local owner must exist before canvases point at it.
	if _, err := st.UpsertUserByGoogleSub(ctx, api.LocalUserTemplate()); err != nil {
		return nil, fmt.Errorf("local user: %w", err)
	}

	canvases, err := imp.src.rows(ctx, "canvases", canvasFilter(imp.code))
	if err != nil {
		return nil, err
	}
	if len(canvases) == 0 {
		if imp.code != "" {
			return nil, fmt.Errorf("no canvas with code %s on the source", imp.code)
		}
		return nil, errors.New("the source has no canvases")
	}
	canvasIDs := make([]string, 0, len(canvases))
	for _, c := range canvases {
		id, _ := c["id"].(string)
		if id == "" {
			return nil, errors.New("canvas row without id")
		}
		canvasIDs = append(canvasIDs, id)
	}
	imp.log("source: %d canvas(es)", len(canvases))

	db := st.DB()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `PRAGMA defer_foreign_keys = ON`); err != nil {
		return nil, err
	}

	report := map[string]int{}
	for _, table := range tables {
		cols, err := localColumns(ctx, tx, table)
		if err != nil {
			return nil, err
		}
		var rows []map[string]any
		if table == "canvases" {
			rows = canvases
		} else {
			// Child tables are fetched per canvas so a --canvas import never
			// pulls the whole hosted database across the wire.
			for _, id := range canvasIDs {
				part, err := imp.src.rows(ctx, table, "canvas_id=eq."+id)
				if err != nil {
					return nil, err
				}
				rows = append(rows, part...)
			}
		}
		dropped := map[string]bool{}
		for _, row := range rows {
			if table == "canvases" {
				remapCanvas(row)
			}
			if err := upsert(ctx, tx, table, cols, row, dropped); err != nil {
				return nil, fmt.Errorf("%s %v: %w", table, row["id"], err)
			}
		}
		if len(dropped) > 0 {
			names := make([]string, 0, len(dropped))
			for k := range dropped {
				names = append(names, k)
			}
			sort.Strings(names)
			imp.log("%s: ignoring source columns the local schema does not have: %s", table, strings.Join(names, ", "))
		}
		report[table] = len(rows)
		imp.log("%s: %d row(s)", table, len(rows))
	}

	if imp.dryRun {
		return report, nil
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return report, nil
}

func canvasFilter(code string) string {
	if code == "" {
		return ""
	}
	return "code=eq." + url.QueryEscape(code)
}

// remapCanvas applies the local-mode rules to a hosted canvas row.
func remapCanvas(row map[string]any) {
	row["owner_user_id"] = api.LocalUserID.String()
	// A private hosted board is private because of OTHER accounts; locally
	// there are none, and a bare code must still let agents in.
	row["visibility"] = "public"
	if row["public_role"] == nil || row["public_role"] == "" {
		row["public_role"] = "write"
	}
	var enabled []string
	if raw, ok := row["enabled_modes"].([]any); ok {
		for _, m := range raw {
			if s, ok := m.(string); ok && allowedModes[s] {
				enabled = append(enabled, s)
			}
		}
	}
	if enabled == nil {
		enabled = []string{}
	}
	row["enabled_modes"] = enabled
	if mode, _ := row["mode"].(string); !allowedModes[mode] {
		switch {
		case contains(enabled, "docs"):
			row["mode"] = "docs"
		case contains(enabled, "roadmap"):
			row["mode"] = "roadmap"
		default:
			row["mode"] = "welcome"
		}
	}
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

type column struct {
	name string
	typ  string // declared type, upper-cased
}

func localColumns(ctx context.Context, tx *sql.Tx, table string) ([]column, error) {
	rows, err := tx.QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cols []column
	for rows.Next() {
		var (
			cid, notnull, pk int
			name, typ        string
			dflt             sql.NullString
		)
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		cols = append(cols, column{name: name, typ: strings.ToUpper(typ)})
	}
	if len(cols) == 0 {
		return nil, fmt.Errorf("local schema has no table %s", table)
	}
	return cols, rows.Err()
}

// upsert writes one source row into table using the columns both sides have.
func upsert(ctx context.Context, tx *sql.Tx, table string, cols []column, row map[string]any, dropped map[string]bool) error {
	have := map[string]bool{}
	names := make([]string, 0, len(cols))
	args := make([]any, 0, len(cols))
	for _, c := range cols {
		v, ok := row[c.name]
		if !ok {
			continue
		}
		have[c.name] = true
		conv, err := convert(c, v)
		if err != nil {
			return fmt.Errorf("column %s: %w", c.name, err)
		}
		names = append(names, c.name)
		args = append(args, conv)
	}
	for k := range row {
		if !have[k] {
			dropped[k] = true
		}
	}
	if len(names) == 0 {
		return errors.New("no importable columns")
	}
	marks := strings.TrimSuffix(strings.Repeat("?,", len(names)), ",")
	_, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO `+table+` (`+strings.Join(names, ",")+`) VALUES (`+marks+`)`, args...)
	return err
}

// convert turns a PostgREST JSON value into what the local column stores:
// timestamps → the schema's fixed-width UTC text, jsonb/arrays → JSON text,
// booleans → 0/1, numbers into INTEGER columns → int64, uuids/text as-is.
func convert(c column, v any) (any, error) {
	if v == nil {
		return nil, nil
	}
	switch x := v.(type) {
	case bool:
		if x {
			return int64(1), nil
		}
		return int64(0), nil
	case float64:
		if c.typ == "INTEGER" || c.typ == "INT" {
			return int64(x), nil
		}
		return x, nil
	case string:
		if strings.HasSuffix(c.name, "_at") {
			t, err := time.Parse(time.RFC3339Nano, x)
			if err != nil {
				// Postgres may omit the colon in the offset ("+00"); try that.
				t, err = time.Parse("2006-01-02T15:04:05.999999Z07", x)
			}
			if err != nil {
				return nil, fmt.Errorf("timestamp %q: %w", x, err)
			}
			return sqlite.FormatTime(t), nil
		}
		if _, err := uuid.Parse(x); err == nil {
			return strings.ToLower(x), nil
		}
		return x, nil
	case map[string]any, []any, []string:
		b, err := json.Marshal(x)
		if err != nil {
			return nil, err
		}
		return string(b), nil
	default:
		return nil, fmt.Errorf("unsupported value %T", v)
	}
}
