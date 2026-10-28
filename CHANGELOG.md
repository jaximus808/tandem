# Changelog — Tandem server

Releases of the `tandem` binary (version: `apps/api/VERSION`, tag `v<version>`).
The MCP gateway (`@jaximus/tandem-mcp`) has its own log in
[`apps/mcp-gateway/CHANGELOG.md`](apps/mcp-gateway/CHANGELOG.md).

## v0.2.0 — 2026-10-28

Paired gateway release: `@jaximus/tandem-mcp` 3.2.0.

### Board redesign: DESIGN.md v3 "Tool"

The web UI now looks like a working tool rather than generic AI-generated UI.
(TDM-10 `cef8b2b`, TDM-11 `6ef6e5c`, TDM-12 `85ea607`, TDM-13 `31e59cc`,
TDM-23 `c7164b9`)

- Primary buttons are ink, not indigo. The accent is a desaturated blue.
- Radii are capped at 6px. No more pills or `rounded-full` chips.
- Muted `state-*` tokens, so `executing` is teal instead of violet. A state
  reads as a dot plus a word on cards and chips.
- Borders instead of shadows. Lane headers are quieter, and the bulk bar is
  restyled.
- The dashboard defaults to a dense canvas list/table.
- Polish: one-off chips use the state-chip ground, the local-mode empty state
  drops account copy, and no label or badge is smaller than 10px.

### Bulk reject of approved work

- **API: reject approved, unclaimed tasks.** `RejectAction` used to accept only
  `proposed` tasks. It now also takes `approved` tasks that nobody has claimed.
  Claimed or finished tasks are refused with coded 409s (`reject_claimed`,
  `reject_finished`), and rejecting is still human-only. (TDM-14 `a6e3361`)
- **New `POST /api/canvas/actions/reject-batch`** with `{ ids, reason }`. It
  returns an outcome for each id.
- **Undo restores the prior state.** Undoing a rejection reads the origin from
  the audit trail, so a task that was rejected from `approved` goes back to
  `approved`, not `proposed`. `task.rejected` events and `queue_wait` carry
  `from: "approved"` when that applies.
- **Board: multi-select + bulk reject in the Ready lane**, with the same
  triage the Proposed lane already had. Selection is scoped to its lane. The
  board shows the reason for each refusal and offers a batch Undo.
  (TDM-15 `a0c52f5`)

### Agent monitor

- **Attribution keyed by agent id.** Claims used to be keyed by the claimant's
  name, so subagents that shared a name, or one agent holding several claims,
  overwrote each other. Tasks now record `actions.claimed_by_agent_id`, and
  every claim an agent holds is shown. **Schema change (schemaVersion 2): the
  database migrates at boot**, so restarting the server is enough.
  (TDM-16 `308dfa1`)
- **Distinct default agent names.** A registration with no name gets
  `<role>-<4 chars>`. A name already held by a different agent (another
  planner's worker, or a live sibling under the same parent) gets a suffix, so
  same-role subagents no longer merge into one agent. The same session
  re-registering its own name still refreshes that agent. (TDM-22 `d6c81be`)
- **Agents surface in the rail.** The fleet roster moved from a header popover
  to its own surface (Board / Agents / Documents / Summary). Each live agent is
  a row nested under its orchestrator, showing every ticket it holds, its
  newest progress line, time held, and lease state (with a stale-lease warning). The header
  shrinks to an avatar stack + count. (TDM-17 `e5d636b`)
- **Live "watch agent".** In local mode the server links each Tandem agent to
  its Claude Code transcript under `~/.claude/projects`, matched by the
  `agentId` its `canvas_connect` / `agent_register` result returned. It is served
  from `GET /api/canvas/agents/{agentId}/transcript` and streamed into the
  Agents surface's detail pane. The pane auto-scrolls, has a jump-to-latest
  button, collapses tool results, and pauses while hidden.
  (TDM-18 `4225f0c`, TDM-19 `0a0d907`)

#### Privacy: agent transcripts

The watch pane reads your Claude Code session files from disk. Limits:

- **Local mode only.** Under `TANDEM_AUTH=google` the endpoint always refuses
  (`403 transcripts_local_only`), because a self-hosted server must never expose
  its operator's files.
- **Opt out with `TANDEM_TRANSCRIPTS=off`** (or `0` / `false`).
- **`TANDEM_CLAUDE_DIR`** overrides the root it reads from. The default is
  `CLAUDE_CONFIG_DIR` if that is set, otherwise `~/.claude`.
- `/api/config` reports whether transcripts are available.

## v0.1.1 and earlier

The first local-first releases (2026-09-15) predate this file. See the git
history and the GitHub Releases.
