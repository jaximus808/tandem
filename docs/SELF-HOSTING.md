# Self-hosting Tandem

The default Tandem is local: one binary, one SQLite file in `~/.tandem`, no
accounts. This page covers the two things beyond that — running one shared
instance with sign-in for a team, and moving boards off the old hosted
instance — plus a short history of the local-first move.

Back to the [README](../README.md).

## Self-host with sign-in

Local mode has exactly one user: whoever opens the browser. If a team wants one Tandem on a shared box with accounts, run the same binary with `TANDEM_AUTH=google` — Google sign-in, personal access tokens, per-account sharing and the landing page come back. The [`Dockerfile`](../Dockerfile) is the container path:

```bash
docker build -t tandem --build-arg VITE_GOOGLE_CLIENT_ID=… .
docker run -p 7891:7891 -v tandem-data:/data \
  -e TANDEM_AUTH=google -e GOOGLE_CLIENT_ID=… -e PUBLIC_BASE_URL=https://tandem.example.com -e COOKIE_SECURE=true \
  tandem
```

Agents then point at it with `API_URL=https://tandem.example.com` (and `TANDEM_TOKEN` for private canvases).

Every server setting, including the auth ones, is documented in [`apps/api/.env.example`](../apps/api/.env.example).

## Move your boards over

Canvases on the old hosted instance copy straight out of its Supabase project into your local database:

```bash
cd apps/api
SUPABASE_URL=https://<ref>.supabase.co SUPABASE_KEY=<service_role> \
  go run ./cmd/import-supabase                 # every canvas; --canvas <CODE> for one
                                               # --db <path> if it isn't ~/.tandem/tandem.db
```

Tasks keep their `TDM-n` tickets, documents and notes come across, and every canvas is re-owned by the local user.

## What changed in the local-first move

Tandem used to be hosted (Supabase + GCP, Google sign-in). It is now a tool you run. The parts that weren't the board were cut: maps / itinerary, sheets, charts, forms and their exports, metrics history, the load-test tool, the stats console and PostHog analytics. What stays is the task queue with its approval gate, documents and notes, roadmap items, the live board and fleet view, webhooks, and — behind `TANDEM_AUTH=google` — accounts and sharing. Each cut is its own `local-first:` commit, so the way back is `git revert`.
