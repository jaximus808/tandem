# Dev workflows

Tandem is one Go binary with the web bundle embedded and a SQLite file under
`~/.tandem`. There is no Docker, no Supabase and no container to rebuild — the
dev loop is "run the Go server, point it at a web build".

Three ways to run it, fastest first.

## 1. Mock mode — frontend only, no backend (fastest UI iteration)

Pure frontend with a hardcoded canvas fixture. No API process at all. Hot-reloads
on every save.

```bash
pnpm dev:mock
```

- Opens http://localhost:5173, auto-routes to the mock canvas (`/c/MOCKCNV1`).
- All ops (`note.update`, `mode.set`, task moves, …) apply to in-memory state —
  refreshing resets to the fixture.

Use for: testing layout, markdown rendering, the docs editor, the board — anything
that's pure UI.

Limitations: no MCP gateway, refresh wipes changes.

## 2. Go server + Vite HMR (real data, fast iteration)

The Go server serves the API, the WebSocket hub and (if you built one) the web
bundle; Vite proxies `/api`, `/canvas-images` and `/ws` to it on `localhost:7891`.

```bash
# terminal 1 — the server, local mode, data in ~/.tandem
cd apps/api && go run ./cmd/server

# terminal 2 — the frontend with HMR
pnpm dev:web
```

Open http://localhost:5173. Real SQLite canvases, real WS sync, instant frontend
reload on save. After editing Go code, restart terminal 1 (`go run` recompiles).

Want a throwaway database instead of your real boards? `TANDEM_DATA_DIR=/tmp/tandem-dev`
(or `TANDEM_DB=:memory:` for one that vanishes on exit).

To test the MCP side against this server, the gateway already defaults to
`http://127.0.0.1:7891`, so `claude mcp add tandem -- npx -y @jaximus/tandem-mcp`
(or `pnpm dev:gateway` for the source checkout) just works. Set
`TANDEM_AUTOSTART=0` while iterating if you don't want the gateway to spawn a
second server when yours is down.

## 3. The real binary (what ships)

`scripts/build.sh` builds the web bundle, copies it into
`apps/api/internal/webdist/bundle/` for `//go:embed`, and compiles
`dist/tandem` for this machine (`--all` for every release target, `--skip-web`
to reuse the last web build).

```bash
scripts/build.sh
./dist/tandem            # http://localhost:7891 — exactly what `tandem-mcp up` runs
```

Or point a plain `go run` at a fresh web build without embedding it:

```bash
pnpm --filter @agentcanvas/shared build && pnpm --filter web build
cd apps/api && WEB_DIST_PATH=../../apps/web/dist go run ./cmd/server
```

`WEB_DIST_PATH` (a directory) wins over the embedded bundle when set.

---

## Cheat sheet

| Goal | Command | URL |
|---|---|---|
| UI work, no backend | `pnpm dev:mock` | http://localhost:5173 |
| UI work, real DB | `cd apps/api && go run ./cmd/server` then `pnpm dev:web` | http://localhost:5173 |
| End-to-end binary | `scripts/build.sh && ./dist/tandem` | http://localhost:7891 |
| Server logs (when started by `tandem-mcp up`) | `tail -f ~/.tandem/logs/tandem.log` | — |
| Stop a server `up` started | `npx @jaximus/tandem-mcp down` | — |
| Verify before commit | `cd apps/api && go build ./... && go test ./...`; `cd apps/web && pnpm build`; `cd apps/mcp-gateway && pnpm build && pnpm test` | — |

## Configuration

Nothing is required. Every knob — `TANDEM_AUTH`, `TANDEM_DATA_DIR`, `TANDEM_DB`,
`PORT`, `WEB_DIST_PATH`, `JWT_SECRET`, `CLAIM_TTL_MINUTES`, the webhook and
metrics switches, and the `TANDEM_AUTH=google` self-hosting set — is documented
in `apps/api/.env.example`. `GET /api/config` reports what the running server
decided; `GET /healthz` is the readiness probe `tandem-mcp up` polls.

## Schema changes

The schema is `apps/api/internal/store/sqlite/schema.sql`; `migrate.go` applies
it under a `schema_version` table at boot. To change it: edit `schema.sql` (the
current shape), bump `schemaVersion`, add a step that upgrades an existing
database, and cover it in the package's tests (they run on `:memory:`). A restart
migrates a real database.

## The mock fixture

Edit `apps/web/src/lib/mockFixture.ts` to change what `dev:mock` shows. The
fixture is plain TypeScript — add notes, tasks, roadmap items — and Vite HMR will
reload immediately. If you add a feature that needs new fixture data, drop it in
there with a comment so the next person knows what it's exercising.
