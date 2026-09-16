# Local-first Tandem — transition tracker

The planning canvas (TEGLQFXR) is unreachable while the hosted site is down, so
this file is the task queue for the transition. One commit per task, prefixed
`local-first:`. Nothing is pushed.

## Goal

Tandem becomes a locally hosted tool: one Go binary with the web bundle embedded
and a SQLite database in `~/.tandem`, started from the terminal or by an agent
(`npx @jaximus/tandem-mcp up`), which opens the board in the browser. No
Supabase, no GCP, no Google sign-in unless a self-hoster turns it on.

## Decisions (reversible — each lives in its own commit)

- **Database: SQLite via `modernc.org/sqlite`** (pure Go, `CGO_ENABLED=0`, so the
  cross-compiled release binaries stay trivial). Schema is one embedded
  `schema.sql` plus a tiny versioned migrator; the 44 Postgres migrations are
  history (git keeps them). PostgREST RPCs become Go transactions.
- **Auth is a deploy flag.** `TANDEM_AUTH=none` (default) = local mode: every
  browser request is the single local owner, sign-in never renders, `/` is the
  dashboard (canvas list), no landing/marketing pages. `TANDEM_AUTH=google` =
  today's Google sign-in + PATs + OAuth step-up + sharing + notifications, for
  someone self-hosting. That code stays; it is ported to SQLite, not deleted.
- **Cut the artifact surfaces**: maps/pins/events (itinerary), sheets, charts,
  forms, itinerary/sheets exports, plus metrics history, loadtest and the stats
  console. The local product is the board + documents/notes + roadmap. This is
  the demoted travel/artifact wedge (pivot decision, 2026-07); porting it would
  be ~45 store methods of dead weight. Reverting is `git revert` of two commits.
- **Kept**: canvases, documents, notes, roadmap items, agents, actions
  (tasks/epics, tickets, claims, audit), webhooks + deliveries (needed by
  `tandem-mcp listen`), users/PATs/OAuth/access/notifications (behind the flag),
  live `/api/metrics`, WebSocket hub, pending_edits.
- **Distribution**: tag-triggered GitHub Release with darwin/linux/windows ×
  amd64/arm64 binaries + npm publish of the gateway. `deploy.yml`,
  `deploy-migrations.yml`, Caddy, compose, the MCP docker sidecar and the dead
  `apps/server` Fastify app are deleted. One slim Dockerfile stays for
  self-hosters.
- **Saving the existing boards**: `cmd/import-supabase` copies the surviving
  tables from the old Supabase project straight into the local SQLite file
  (plain PostgREST HTTP, needs only `SUPABASE_URL` + service key), so TEGLQFXR
  and its ticket numbers survive the move.

## Shared contracts

- `GET /api/config` → `{ "auth": "none" | "google", "local": true|false, "version": "x.y.z" }`.
  The web reads it once at boot and gates sign-in, the landing page and the
  account surfaces on it.
- Local identity: a fixed user row (`id = 00000000-0000-4000-8000-000000000001`,
  email `local@tandem`) created at boot in local mode; middleware resolves every
  browser request without a session cookie to it; every canvas created in local
  mode is owned by it.
- Data dir: `TANDEM_DATA_DIR` (default `~/.tandem`): `tandem.db`, `images/`,
  `secret` (auto-generated JWT secret), `logs/`.
- Gateway default `API_URL` → `http://127.0.0.1:7891`.

## Tasks

Phase 1 — cut (parallel, file-disjoint)
- [x] T1 Go: delete pins/events/maps, sheets, charts, forms, itinerary + sheets
      exports from `apps/api` (handlers, routes, store interface + supabase impl,
      `internal/maps`, `internal/forms`, tests). Server still boots on Supabase.
- [x] T2 Go: delete metrics history persistence, `cmd/loadtest`, `/api/stats`,
      `/api/metrics/history|loadtest`, `metrics_snapshots` + `loadtest_runs` store
      methods. Live `/api/metrics` stays.
- [x] T3 TS: delete the same surfaces from `internal/shared`, the gateway
      (`tools.ts`, `facade.ts`, tests) and the web (modes Map/Itinerary/Sheets/
      Charts, StatsPage, MetricsPage, leaflet, posthog-js).
- [x] T4 Go: new package `internal/store/sqlite` implementing the surviving
      `store.Store` on `modernc.org/sqlite` — schema + migrator, canvases,
      documents, notes, roadmap, agents, actions (atomic claim, ticket
      reservation, claim fence, audit), users/PATs/OAuth/access/notifications,
      webhooks + deliveries, pending_edits. Tests run on `:memory:`.

Phase 2 — wire (sequential)
- [x] T5 Go: `cmd/server` boots on SQLite; `TANDEM_DATA_DIR`, auto JWT secret,
      delete `supabase.go`, `supabase-go` dep, `supabase/`, `.env.example` rewrite.
- [x] T6 Go: `TANDEM_AUTH` flag, local identity middleware, `GET /api/config`,
      auth routes 404 in local mode, webhook SSRF guard off in local mode.

Phase 3 — product (parallel)
- [x] T7 Web: read `/api/config`; local mode = dashboard at `/`, no sign-in,
      no landing/marketing routes, no account/notification/share-by-account UI.
- [x] T8 Gateway: `tandem-mcp up` / `open` (resolve or download the binary,
      spawn detached, wait for `/healthz`, open browser), local `API_URL`
      default, auto-start on `canvas_connect`/`canvas_create` when the local
      server is unreachable, `init` against local.
- [x] T9 Go: embed the web dist (`//go:embed`), `/healthz`, `scripts/build.sh`
      + `make dist` cross-compile matrix.
- [x] T10 Go: `cmd/import-supabase` — copy surviving tables from a Supabase
      project into the local SQLite file.

Phase 4 — ship
- [x] T11 Delete hosted deploy (workflows, Caddyfile, compose, Dockerfile.mcp,
      scripts/rebuild*, `apps/server`); add `release.yml` on `v*` tags; slim
      Dockerfile for self-hosters.
- [x] T12 Docs: README, CLAUDE.md (build/verify, env, no Supabase), `docs/DEV.md`,
      `docs/ORCHESTRATION.md` local notes, gateway README/CHANGELOG.

Phase 5 — polish + site (after v0.1.1 / gateway 3.0.1 shipped)
- [x] T13 README: logo, mermaid architecture/loop/state diagrams, one-line
      onboarding (`tandem-mcp setup`), commands table.
- [x] T14 Gateway 3.1.0: `tandem-mcp setup` — user-scope `claude mcp add`,
      idempotent `~/.claude/CLAUDE.md` snippet ("sprint board" → Tandem), `up`.
- [x] T15 `apps/tandem-next`: Next.js marketing site for Vercel (Root Directory
      `apps/tandem-next`), landing/features/why/about/mcp ported, local-first CTAs.

## Status (2026-09-15)

Every task above is committed on `main` (nothing pushed). Verified: `go build
./... && go test ./...` green (api, config, metrics, store, store/sqlite,
webhooks, ws, import-supabase); shared/web/gateway build, gateway 344/344
tests; `scripts/build.sh` produced `dist/tandem` (0.1.0, darwin/arm64, web
bundle embedded) and a scratch-dir boot served the dashboard with the local
session cookie, created a canvas, minted an agent token and filed TDM-1 on
SQLite with zero environment.

## Left for Jaxon after the code lands

- `pnpm install` (lockfile changed), then `scripts/build.sh` and run the binary.
- Run `go run ./cmd/import-supabase` once with the old `SUPABASE_URL`/`SUPABASE_KEY`
  to bring TEGLQFXR and the other boards over.
- Tag `v0.1.0` to cut the first binary release (until it exists, `tandem-mcp up`
  cannot download anything — point `TANDEM_BIN` at `dist/tandem`).
- Publish the gateway 3.0.0 (`mcp-v3.0.0` tag → release-mcp-gateway.yml) so
  `npx @jaximus/tandem-mcp up` exists outside this repo.
