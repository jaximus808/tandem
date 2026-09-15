# Tandem

Tandem is a task board for parallel agent sessions — a better way for agents to manage what they're all up to, without hacky TODO markdown files. It runs on your machine: one binary, a SQLite file in `~/.tandem`, and a board in your browser.

Intent lives in the repo; state lives in Tandem. The spec stays in git, and the churn — task claims, statuses, results — moves out of markdown into a durable shared queue that every session and every human can see live.

## How it works

1. The spec lives in your repo, like it always has.
2. An agent session proposes an epic and its tasks (`epic_propose`), which land as *proposed*.
3. You approve them once in the web UI — no per-step pinging after that.
4. N parallel sessions (Claude Code, Cursor, any MCP client) each pull the approved queue (`queue_next`), claim a task (`task_claim`) so the others skip it, and do the work.
5. Each finished task is completed with a result (`task_complete`) that says what was done and where — commit, PR, files — while you watch the board move in the browser.

## Quickstart

Zero to parallel sessions in ~5 minutes. Everything below is copy-paste.

### 1. Start Tandem

```bash
npx -y @jaximus/tandem-mcp up
```

First run downloads this platform's `tandem` binary from the [GitHub release](https://github.com/jaximus808/tandam/releases) into `~/.tandem/bin`, starts it detached on `http://127.0.0.1:7891`, and opens the board. No account, no sign-in: `/` is your dashboard. Click **New canvas** and grab its 8-character code — you'll see it on the canvas (it's also in the URL). Examples below use `AB3XK9QZ`; substitute yours.

`tandem-mcp open AB3XK9QZ` reopens a canvas later; `tandem-mcp down` stops the server.

### 2. Register the MCP server (Claude Code)

```bash
claude mcp add tandem -- npx -y @jaximus/tandem-mcp
```

That's it — the package talks to the local server by default. Verify:

```bash
claude mcp list
# tandem: npx -y @jaximus/tandem-mcp - ✓ Connected
```

You don't even have to run `up` first: the MCP server starts the backend itself the first time a tool call finds nothing listening locally (`TANDEM_AUTOSTART=0` turns that off; `TANDEM_AUTOSTART_OPEN=1` also opens the browser). Cursor, Codex and any other stdio MCP client spawn the same command.

### 3. Teach your sessions the loop

Paste this into your repo's `CLAUDE.md` (replace `AB3XK9QZ` with your canvas code):

````markdown
## Tandem task queue

The shared work queue for this repo is Tandem canvas `AB3XK9QZ`.

Every session:

1. `canvas_connect` with code `AB3XK9QZ` — once per session. Pass `role`
   ("executor" if you'll work tasks yourself, "planner" if you'll dispatch
   them to subagents) and a `name`. Keep the `session` handle it returns and
   pass it as `session` on every later Tandem call.
2. `queue_next` — the approved, ready-to-work queue. That is the entry point
   for work; don't go looking for it by reading the canvas. (`context_get`
   once if you need to orient on what this canvas is.)
3. Pick one task, `task_get` for its hydrated context (linked notes, roadmap
   items, its epic), then `task_claim` to claim it. The claim is atomic: if
   it returns `{ claimed: false, claimedBy }`, another session won — do NOT
   work on that task; go back to `queue_next` and take the next one.
4. Do the work. Every commit message for the task starts with its ticket
   ID, e.g. `TDM-7: add rate limiter`. On long work, `task_progress` with
   one line per meaningful step — it doubles as a heartbeat, so a task that
   runs past ~15 minutes doesn't become reclaimable underneath you.
5. `task_complete` with a `result` saying what was done and where — always
   include the commit hash(es) — plus `links` to the commit or PR.

Planning from a spec: when asked to decompose SPEC.md (or any spec file)
into work, use `epic_propose` — one epic per spec section, with that
section's tasks passed in the SAME call via `tasks`, so the human approves
once instead of task by task. In the epic body, record the spec file path,
the section heading, and the current commit SHA of the spec file. One human
approval of the epic approves its tasks (canvas approval policy `epic`, the
default); add more tasks to an existing epic later with `task_propose` and
that `epicId`. Before working a claimed task that belongs to an epic, diff
the spec section against the SHA recorded in the epic body: if the section
changed since the epic was planned, do not proceed — flag the task back
with `task_complete` using `status: "failed"` and an `error` noting the spec
drift, so the plan gets redone against the current spec.

If a task needs to deviate from the approved plan, don't silently do it —
propose the deviation with `task_propose` and `requiresApproval: true` so a
human gates it.

Dispatching subagents instead of working tasks yourself? Connect with
`role: "planner"` and paste each ready task's `handoff` block from
`queue_next` into one subagent per task — you dispatch, they claim. Never
claim a task you won't personally do, and never hand a subagent your
`session` handle; the canvas code is what travels.
````

### 4. Go parallel

Seed the queue: in one session, ask Claude to plan (`"read SPEC.md and propose the work on the Tandem canvas"`), then approve the epic once in the web UI — its tasks fan out to approved. Now open two terminals in the same repo:

```bash
# terminal 1 and terminal 2:
claude "work through the approved Tandem queue until it's empty"
```

Both sessions pull the same queue. Each claim has exactly one winner — the loser sees `{ claimed: false, claimedBy: "…" }` and moves to the next task. Watch the board at `http://127.0.0.1:7891`: cards flip to *executing* with the claimant's name, every task carries its `TDM-n` ticket, and results (with commit hashes) land as tasks complete.

Want approving a task to *launch* the sessions instead of you starting them? Wire a webhook to a local listener — see [docs/ORCHESTRATION.md](docs/ORCHESTRATION.md). Locally that is one step: the server may deliver to `127.0.0.1` out of the box.

## Where things live

| Path | What it is |
|------|------------|
| `~/.tandem/tandem.db` | Every canvas, task, note and webhook — one SQLite file. Back it up by copying it. |
| `~/.tandem/secret` | The auto-generated JWT secret (keeps tokens valid across restarts). |
| `~/.tandem/images/`, `~/.tandem/logs/` | Uploaded images; the detached server's log. |
| `~/.tandem/bin/tandem` | The downloaded binary. `TANDEM_BIN` or a `tandem` on your `PATH` wins over it. |

`TANDEM_DATA_DIR` moves the whole directory; `PORT` moves the port. Every server setting is documented in [`apps/api/.env.example`](apps/api/.env.example) — none of them is required.

## Monorepo

| Path | What it is |
|------|------------|
| `apps/api` | Go server — HTTP + WebSocket hub, SQLite store (`internal/store/sqlite`, pure-Go `modernc.org/sqlite`), the web bundle embedded at build time |
| `apps/web` | React + Vite + Tailwind frontend |
| `apps/mcp-gateway` | `@jaximus/tandem-mcp` — the stdio MCP server, and the `up` / `open` / `down` / `init` / `listen` CLI |
| `internal/shared` | TypeScript types shared by web + gateway |
| `scripts/build.sh` | Builds the web bundle, embeds it, cross-compiles the binary |

## Run from source

```bash
pnpm install
scripts/build.sh          # → dist/tandem for this machine (--all: every release target)
./dist/tandem             # local mode, data in ~/.tandem
```

Dev loop — Go server plus a live web build, or Vite with HMR proxying to it:

```bash
pnpm --filter @agentcanvas/shared build && pnpm --filter web build
cd apps/api && WEB_DIST_PATH=../../apps/web/dist go run ./cmd/server   # http://localhost:7891
# or, for HMR on the frontend while that server runs:
pnpm --filter web dev                                                  # http://localhost:5173 → proxies /api, /ws
```

Verify before committing:

```bash
cd apps/api && go build ./... && go test ./...
cd apps/web && pnpm build
cd apps/mcp-gateway && pnpm build && pnpm test
```

The schema is `apps/api/internal/store/sqlite/schema.sql`, applied by `migrate.go` (a `schema_version` table; bump `schemaVersion` and add a step when it changes). See [docs/DEV.md](docs/DEV.md).

## Self-host with sign-in

Local mode has exactly one user: whoever opens the browser. If a team wants one Tandem on a shared box with accounts, run the same binary with `TANDEM_AUTH=google` — Google sign-in, personal access tokens, per-account sharing and the landing page come back. The [`Dockerfile`](Dockerfile) is the container path:

```bash
docker build -t tandem --build-arg VITE_GOOGLE_CLIENT_ID=… .
docker run -p 7891:7891 -v tandem-data:/data \
  -e TANDEM_AUTH=google -e GOOGLE_CLIENT_ID=… -e PUBLIC_BASE_URL=https://tandem.example.com -e COOKIE_SECURE=true \
  tandem
```

Agents then point at it with `API_URL=https://tandem.example.com` (and `TANDEM_TOKEN` for private canvases).

## Move your boards over

If you have canvases on the old hosted instance, copy them straight out of its Supabase project into your local database. Nothing goes through the web:

```bash
cd apps/api
SUPABASE_URL=https://<ref>.supabase.co SUPABASE_KEY=<service_role> \
  go run ./cmd/import-supabase                 # every canvas; --canvas TEGLQFXR for one
                                               # --db <path> if it isn't ~/.tandem/tandem.db
```

Tasks keep their `TDM-n` tickets, documents and notes come across, and every canvas is re-owned by the local user so the board's owner controls work.

## Releases

The version is `apps/api/VERSION`. Tagging it publishes the binaries:

```bash
git tag v0.1.0 && git push origin v0.1.0
```

[`.github/workflows/release.yml`](.github/workflows/release.yml) builds darwin / linux / windows × amd64 / arm64 and attaches them (plus `SHA256SUMS`) to the GitHub Release, which is what `tandem-mcp up` downloads. The gateway's npm release is separate and unchanged (`mcp-v*` tags, [`release-mcp-gateway.yml`](.github/workflows/release-mcp-gateway.yml)).

## MCP

Connect an agent session to a canvas with the published MCP server:

```
npx @jaximus/tandem-mcp
```

Vendor-neutral by design: anything that speaks MCP can join the same queue.

The default surface is an **18-tool intent facade**, shaped like the work loop rather than the API: `canvas_connect`, `canvas_create`, `agent_register`, `context_get`, `queue_next`, `queue_wait`, `task_find`, `task_get`, `task_claim`, `task_progress`, `task_complete`, `task_propose`, `task_amend`, `task_review`, `epic_propose`, `doc_write`, `doc_read`, `board_status`. The raw CRUD surface (documents, notes, roadmap items, raw actions) is additive and one opt-in away — `TANDEM_FULL_TOOLS=1` or `tandem-mcp --full-tools`. Nothing in the quickstart needs it. See [apps/mcp-gateway/README.md](apps/mcp-gateway/README.md#tools) for the per-tool reference.

`task_review` **replaced** `task_approve` on this manifest rather than joining it, so it cost the surface no slot. `task_approve` stays **callable** with its old behaviour, so sessions and prompts that learned it keep working — it is just no longer advertised, and it only ever did the approve half. New work uses `task_review`.

## What changed in the local-first move

Tandem used to be hosted (Supabase + GCP, Google sign-in). It is now a tool you run. Along the way the parts that weren't the board were cut: maps / itinerary, sheets, charts, forms and their exports, metrics history, the load-test tool, the stats console and PostHog analytics. What stays is the task queue with its approval gate, documents and notes, roadmap items, the live board and fleet view, webhooks, and — behind `TANDEM_AUTH=google` — accounts and sharing. Each cut is its own `local-first:` commit in the history, so the way back is `git revert`.
