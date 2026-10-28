import { useCallback, useEffect, useMemo, useState, type KeyboardEvent as ReactKeyboardEvent, type ReactNode } from "react";
import { AlertTriangle, ArrowLeft, Bot, Shield, Users, X } from "lucide-react";
import type { Action, TaskPayload, TaskProgressEntry } from "../types";
import type { FleetAgent, FleetTask, FleetWait } from "../lib/api";
import type { useFleetRoster } from "../lib/useFleetRoster";
import { useActivityFeed } from "../lib/useActivityFeed";
import { contentionByAgent, type ContentionTally } from "../lib/contention";
import { ageOf, fullDate } from "../lib/relativeTime";
import { deriveLease, leaseAge, leaseSentence, type Lease } from "../lib/lease";
import { LeaseChip } from "./TaskBoard";
import { useFreshnessNow } from "./Freshness";
import ActivityFeed from "./ActivityFeed";

/* ─────────────────────────────────────────────────────────────────────────────
   FleetView — the AGENTS surface (TDM-17). It used to be a header popover
   (TDM-47); a team of agents is a first-class thing to look at, so it is now a
   surface in the rail beside Board / Documents / Summary, and the header keeps
   only a compact avatar stack + count that opens it (AgentPresence.tsx).

   THE ROSTER. One row per live agent, nested under the orchestrator that
   spawned it (parentAgentId), each row carrying:

     ●  opus-agentspanel-tdm17   executor   claude-opus-5-5
        TDM-17  Agents panel: replace the header…     ● 2m      held 14m
          ↳ Wrote the surface; wiring App.tsx                      1m
        ⚠ claim lease ran out 3 minutes ago — reclaimable

   Name, role and model on the identity line; then EVERY claim the agent holds
   (ticket ref + title, click → the task on the Board), its newest task_progress
   line, how long it has been held, and the lease health — with a spelled-out
   warning once the lease has lapsed, because that is the one state that asks a
   human for something.

   LIVE. The roster REST read (fleet_handler.go) is refetched on WS lifecycle
   pings (claim / complete / approve) by useFleetRoster. A task_progress report
   is NOT a lifecycle ping — it is a state push — so the per-task details (the
   progress line, the lease, the title) are read from the canvas actions already
   in hand, which every state push keeps current. The roster says WHO holds
   WHAT; canvas state says how that work is going right now. Claims that land in
   state before the roster refetch are folded in the same way, and claims that
   have finished drop out immediately.

   WATCHING ONE AGENT (the hook TDM-19 fills). Clicking an agent's identity line
   selects it — `watchedAgentId` / `onWatchAgent` are lifted to App so any other
   surface can open the same agent — and a detail pane opens beside the roster:
   identity, every claim with its newest progress line (earlier ones behind a
   "Show N earlier" disclosure), and a slot for the live Claude Code transcript
   that takes the rest of the pane's height (TDM-25). That slot renders `renderAgentWatch(agent)` when App
   provides one, and a placeholder until then. Only registered agents (those
   with an id) can be watched; an unregistered claimant has no session to read.
   ──────────────────────────────────────────────────────────────────────────── */

// An idle registered agent counts as "around" for this long after its last
// provable activity. Agents holding a claim are always active regardless (a
// long task is quiet by nature).
const ACTIVE_MS = 30 * 60_000;
// How recently a completion still reads as "just finished" on an idle row.
const RECENT_DONE_MS = 45 * 60_000;
// Ages re-render on this cadence so "12m" doesn't freeze while the surface is up.
const TICK_MS = 30_000;
const MAX_DEPTH = 4;

export type FleetRosterState = ReturnType<typeof useFleetRoster>;

interface Props {
  /** Canvas code — the feed read is a canvas-JWT call keyed on it. */
  code: string;
  /** The live roster read, owned by App so the header count reads the same rows. */
  fleet: FleetRosterState;
  /** Canvas actions, already in hand: live progress/lease, "just finished", contention. */
  actions: Record<string, Action>;
  /** Tasks sitting in `proposed`: the fleet's blocker is the human. */
  proposedCount: number;
  /** Whether the surface is on screen — gates the age tick and the feed fetch. */
  visible: boolean;
  /** Focus a task on the Board (scope → its epic, detail slide-over open). */
  onOpenTask: (taskId: string) => void;
  /** Show the Board — used by the "awaiting approval" foot. */
  onOpenBoard: () => void;
  /** Open the Connect dialog — the empty state's one action. */
  onConnect: () => void;
  /** The agent whose detail pane is open (a registered agent id), or null. */
  watchedAgentId: string | null;
  /** Select an agent to watch (null closes the pane). TDM-19's entry point. */
  onWatchAgent: (agentId: string | null) => void;
  /** TDM-19: the live transcript pane for the watched agent. Placeholder when absent. */
  renderAgentWatch?: (agent: FleetAgent) => ReactNode;
}

type TabId = "roster" | "feed";
const TABS: { id: TabId; label: string }[] = [
  { id: "roster", label: "Roster" },
  { id: "feed", label: "Feed" },
];

// ── identity ─────────────────────────────────────────────────────────────────

type VendorKind = "claude" | "vendor" | "model" | "external";
type Vendor = { label: string; kind: VendorKind; title: string };

// Vendor is READ OFF the declared model, never guessed from behaviour. An
// unregistered claimant has no model to read (it never called agent_register),
// so it gets its own treatment rather than a wrong badge. The chip shows the
// declared model string itself when there is one — "claude-opus-5-5" says more
// than "claude".
export function vendorOf(a: FleetAgent): Vendor {
  if (!a.registered) {
    return {
      label: "external",
      kind: "external",
      title: `${a.name} holds work on this canvas but never registered (no agent_register), so its model is unknown`,
    };
  }
  const model = (a.model ?? "").trim();
  const named = (kind: VendorKind, fallback: string): Vendor => ({
    label: model || fallback,
    kind,
    title: model ? `Model: ${model}` : `${fallback} agent — no model declared`,
  });
  if (/claude/i.test(model) || (!model && /claude/i.test(a.name))) return named("claude", "claude");
  if (/codex|gpt|openai|^o[134]\b|gemini/i.test(model)) return named("vendor", "agent");
  if (model) return named("model", "agent");
  return { label: "agent", kind: "model", title: `${a.name} registered without declaring a model` };
}

const VENDOR_CLS: Record<VendorKind, string> = {
  // Terracotta is the agent-presence token — the one place it survives.
  claude: "bg-agent/10 text-agent",
  vendor: "bg-ink/[0.06] text-ink/60",
  model: "bg-ink/[0.06] text-ink/55",
  external: "border border-dashed border-ink/25 text-ink/55",
};

/** The stable key a row is selected by. Only registered agents have one. */
export function agentKey(a: FleetAgent): string {
  return a.id ?? `claimant:${a.name}`;
}

// ── activity ─────────────────────────────────────────────────────────────────

function within(iso: string | undefined, ms: number, now = Date.now()): boolean {
  if (!iso) return false;
  const t = Date.parse(iso);
  return Number.isFinite(t) && now - t < ms;
}

/** Working, waiting, or online and recently active — "around" right now.
 *  Shared with the header's avatar stack so the two can never disagree. */
export function isActiveFleetAgent(a: FleetAgent): boolean {
  return (
    a.tasks.length > 0 ||
    !!a.waiting ||
    (a.registered ? a.status === "online" && within(a.lastActivityAt, ACTIVE_MS) : within(a.lastActivityAt, ACTIVE_MS))
  );
}

/* One claim as the surface renders it: the roster's row, overlaid with the live
   action from canvas state when we have it. */
export type LiveClaim = {
  id: string;
  ticketId?: string;
  title: string;
  /** Lease derived from the freshest data we hold (state push > roster read). */
  action: Action;
  /** Stable start of this lease generation — the honest "held for". */
  startedAt?: string;
  progress: TaskProgressEntry[];
};

type RosterTaskExtra = FleetTask & {
  claimedBy?: string;
  firstClaimedAt?: string;
  progress?: { at: string; note?: string }[];
};

function claimRecordAt(a: Action): string | undefined {
  const rec = (a.payload as { claim?: { at?: string } } | undefined)?.claim;
  return rec?.at;
}

function progressOf(a: Action | undefined, fallback?: RosterTaskExtra): TaskProgressEntry[] {
  const live = (a?.payload as TaskPayload | undefined)?.progress;
  if (live && live.length > 0) return live;
  return (fallback?.progress ?? []).map((p) => ({ at: p.at, note: p.note ?? "" }));
}

/** Newest progress entry by timestamp (the log is append-only, but be safe). */
function newestProgress(list: TaskProgressEntry[]): TaskProgressEntry | undefined {
  let best: TaskProgressEntry | undefined;
  let bestT = -Infinity;
  for (const e of list) {
    const t = Date.parse(e.at);
    if (Number.isFinite(t) && t >= bestT) {
      best = e;
      bestT = t;
    }
  }
  return best;
}

// Adapt a roster task to the minimal Action shape deriveLease reads, for a claim
// canvas state doesn't carry (it should, but the roster is the fallback).
function actionOfRosterTask(task: RosterTaskExtra): Action {
  return {
    id: task.id,
    type: task.type,
    state: task.state,
    claimedBy: task.claimedBy,
    claimedAt: task.claimedAt,
    ticketId: task.ticketId,
    payload: { title: task.title ?? "", progress: task.progress ?? [] },
  } as unknown as Action;
}

/**
 * Every claim this agent holds RIGHT NOW: the roster's tasks, minus any the live
 * state says have finished or changed hands, plus any executing claim the state
 * attributes to it that the roster hasn't refetched yet. Attribution matches the
 * roster's own join (fleet_handler.go buildRoster): the claim's stored agent id
 * first, then the free-text claimedBy name.
 */
function liveClaimsOf(agent: FleetAgent, actions: Record<string, Action>): LiveClaim[] {
  const holds = (a: Action) =>
    a.state === "executing" &&
    (a.claimedByAgentId
      ? a.claimedByAgentId === agent.id
      : a.claimedBy === agent.name || (!!agent.id && a.claimedBy === agent.id));

  const out = new Map<string, LiveClaim>();
  for (const t of agent.tasks as RosterTaskExtra[]) {
    const live = actions[t.id];
    if (live && !holds(live)) continue; // finished, released, or reclaimed since the read
    const action = live ?? actionOfRosterTask(t);
    out.set(t.id, {
      id: t.id,
      ticketId: live?.ticketId ?? t.ticketId,
      title: (live?.payload as TaskPayload | undefined)?.title || t.title || "Untitled task",
      action,
      startedAt: (live && claimRecordAt(live)) ?? t.firstClaimedAt ?? t.claimedAt,
      progress: progressOf(live, t),
    });
  }
  // Claims the roster hasn't refetched yet. Only those carrying the stored agent
  // id (TDM-16): a legacy name-only claim is left to the roster's own join, so
  // two agents sharing a name never both show it.
  for (const a of Object.values(actions)) {
    if (!agent.id || !a.claimedByAgentId || out.has(a.id) || !holds(a)) continue;
    out.set(a.id, {
      id: a.id,
      ticketId: a.ticketId,
      title: (a.payload as TaskPayload | undefined)?.title || "Untitled task",
      action: a,
      startedAt: claimRecordAt(a) ?? a.claimedAt,
      progress: progressOf(a),
    });
  }
  // Newest claim first, as the roster sorts them.
  return [...out.values()].sort(
    (x, y) => (Date.parse(y.startedAt ?? "") || 0) - (Date.parse(x.startedAt ?? "") || 0),
  );
}

/** What an agent holding claims is doing with them (TDM-24).
 *    working  at least one claim's lease is still live (lib/lease: not "stale")
 *    stalled  every claim it holds has lapsed — nobody has heard from it in a
 *             full TTL, and the next agent to ask takes the work over
 *  An agent holding nothing has no hold at all. */
export type FleetHold = "working" | "stalled";

function holdOf(claims: LiveClaim[], now: number): FleetHold | null {
  if (claims.length === 0) return null;
  return claims.some((c) => deriveLease(c.action, now).health !== "stale") ? "working" : "stalled";
}

export interface FleetSummary {
  /** Everyone "around": holding a claim (live or lapsed), waiting, or recently active. */
  active: FleetAgent[];
  /** Agents with at least one live lease. */
  working: number;
  /** Agents whose every claim has lapsed. */
  stalled: number;
  /** Per-agent hold, keyed by agentKey; absent when the agent holds nothing. */
  holds: Map<string, FleetHold>;
  /** Live claims per agent, keyed by agentKey (every agent passed in). */
  claims: Map<string, LiveClaim[]>;
}

/** The fleet's read of itself — the header's avatar stack + count AND the
 *  Agents surface both come from this, so the two can never disagree. Lease
 *  state is derived from the live `actions` at `now`, so a heartbeat landing on
 *  a state push flips a stalled agent back to working on the next render. */
export function activeFleetSummary(
  agents: FleetAgent[],
  actions: Record<string, Action>,
  now: number,
): FleetSummary {
  const active: FleetAgent[] = [];
  const holds = new Map<string, FleetHold>();
  const claims = new Map<string, LiveClaim[]>();
  let working = 0;
  let stalled = 0;
  for (const a of agents) {
    const key = agentKey(a);
    const held = liveClaimsOf(a, actions);
    claims.set(key, held);
    const hold = holdOf(held, now);
    if (hold) holds.set(key, hold);
    if (hold || isActiveFleetAgent(a)) active.push(a);
    if (hold === "working") working++;
    else if (hold === "stalled") stalled++;
  }
  return { active, working, stalled, holds, claims };
}

// ── component ────────────────────────────────────────────────────────────────

export default function FleetView({
  code,
  fleet,
  actions,
  proposedCount,
  visible,
  onOpenTask,
  onOpenBoard,
  onConnect,
  watchedAgentId,
  onWatchAgent,
  renderAgentWatch,
}: Props) {
  const { roster, error, loading, reload } = fleet;
  const [showDormant, setShowDormant] = useState(false);
  const [tab, setTab] = useState<TabId>("roster");
  const [actorFilter, setActorFilter] = useState<string | null>(null);
  // The feed fetches only while it's the visible tab — a board where nobody
  // opens it costs one WS subscription and nothing else.
  const feed = useActivityFeed(code, visible && tab === "feed");
  // One clock for every age and lease on the surface. It ticks while hidden too
  // (cheap), so switching to the surface never shows a frozen read.
  const now = useFreshnessNow(TICK_MS);

  // Another board's agent names mean nothing here.
  useEffect(() => setActorFilter(null), [code]);

  const all = useMemo(() => roster?.agents ?? [], [roster]);

  // The one shared read (the header's too): live claims, holds and counts,
  // recomputed on every state push and every tick of the lease clock.
  const summary = useMemo(() => activeFleetSummary(all, actions, now), [all, actions, now]);
  const claimsOf = useCallback((a: FleetAgent) => summary.claims.get(agentKey(a)) ?? [], [summary]);
  const holdOfAgent = useCallback((a: FleetAgent) => summary.holds.get(agentKey(a)) ?? null, [summary]);

  const active = summary.active;
  const dormant = useMemo(() => {
    const on = new Set(active);
    return all.filter((a) => !on.has(a));
  }, [all, active]);
  // Stalled agents (every claim lapsed) list in their own section below
  // everyone else, so a dead claim never reads as running work.
  const live = useMemo(() => active.filter((a) => holdOfAgent(a) !== "stalled"), [active, holdOfAgent]);
  const stalledList = useMemo(() => active.filter((a) => holdOfAgent(a) === "stalled"), [active, holdOfAgent]);
  const workingCount = summary.working;
  const stalledCount = summary.stalled;
  // Parked on the queue and holding nothing: present, blocked, and the human is
  // the blocker. Zero unless the server is holding a live wait (see FleetWait).
  const waitingCount = useMemo(
    () => active.filter((a) => claimsOf(a).length === 0 && a.waiting).length,
    [active, claimsOf],
  );
  const unnamedWaiting = roster?.counts?.waitingUnattributed ?? 0;
  const idleCount = active.length - workingCount - stalledCount - waitingCount;

  // "Just finished": the newest done/failed task each identity completed, taken
  // straight from canvas state we already hold — no second fetch.
  const lastDone = useMemo(() => {
    const out = new Map<string, Action>();
    for (const a of Object.values(actions)) {
      if (a.type !== "task") continue;
      if (a.state !== "done" && a.state !== "failed") continue;
      if (!a.claimedBy || !within(a.updatedAt, RECENT_DONE_MS, now)) continue;
      const prev = out.get(a.claimedBy);
      if (!prev || Date.parse(a.updatedAt) > Date.parse(prev.updatedAt)) out.set(a.claimedBy, a);
    }
    return out;
  }, [actions, now]);

  // Collisions per agent (TDM-100), derived from the canvas actions in hand.
  const contention = useMemo(() => contentionByAgent(actions), [actions]);

  // Nest executors under the parent that spawned them. An agent whose parent
  // isn't on the active roster renders at the top level rather than vanishing.
  const tree = useMemo(() => buildTree(live), [live]);
  const stalledTree = useMemo(() => buildTree(stalledList), [stalledList]);
  const dormantTree = useMemo(() => buildTree(dormant), [dormant]);

  const watched = useMemo(
    () => (watchedAgentId ? all.find((a) => a.id === watchedAgentId) ?? null : null),
    [all, watchedAgentId],
  );
  // Esc closes the detail pane while the surface is up.
  useEffect(() => {
    if (!visible || !watchedAgentId) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape" && !e.defaultPrevented) onWatchAgent(null);
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [visible, watchedAgentId, onWatchAgent]);

  function onTabKey(e: ReactKeyboardEvent) {
    if (e.key !== "ArrowLeft" && e.key !== "ArrowRight") return;
    e.preventDefault();
    const i = TABS.findIndex((t) => t.id === tab);
    const next = TABS[(i + (e.key === "ArrowRight" ? 1 : TABS.length - 1)) % TABS.length];
    setTab(next.id);
    (e.currentTarget as HTMLElement).querySelector<HTMLElement>(`#agents-tab-${next.id}`)?.focus();
  }

  const rowCtx: RowCtx = {
    childrenById: tree.childrenById,
    claimsOf,
    holdOf: holdOfAgent,
    lastDone,
    contention,
    now,
    watchedAgentId,
    onWatch: (a) => a.id && onWatchAgent(a.id === watchedAgentId ? null : a.id),
    onOpenTask,
  };

  return (
    <div className="flex min-h-0 min-w-0 flex-1 bg-paper text-ink">
      {/* Roster column. Hidden below sm while an agent is watched — on a phone
          the detail pane takes the whole surface and has a back control. */}
      <section
        aria-label="Agents"
        className={`min-h-0 min-w-0 flex-1 flex-col ${watched ? "hidden sm:flex" : "flex"}`}
      >
        <header className="flex shrink-0 items-center gap-3 border-b border-ink/10 px-4 py-2.5 sm:px-5">
          <Bot size={17} strokeWidth={1.75} className="shrink-0 text-ink/45" aria-hidden="true" />
          <h2 className="shrink-0 text-[14px] font-semibold tracking-tight text-ink">Agents</h2>
          <p className="min-w-0 truncate text-[12px] text-ink/50">
            {active.length === 0
              ? "Nobody connected"
              : [
                  workingCount > 0 && `${workingCount} working`,
                  stalledCount > 0 && `${stalledCount} stalled`,
                  waitingCount > 0 && `${waitingCount} waiting`,
                  idleCount > 0 && `${idleCount} idle`,
                  roster?.counts && roster.counts.unregistered > 0 && `${roster.counts.unregistered} external`,
                  unnamedWaiting > 0 && `${unnamedWaiting} unnamed waiting`,
                ]
                  .filter(Boolean)
                  .join(" · ")}
          </p>
          <div
            role="tablist"
            aria-label="Agent views"
            onKeyDown={onTabKey}
            className="ml-auto flex shrink-0 items-center gap-0.5"
          >
            {TABS.map((t) => (
              <button
                key={t.id}
                role="tab"
                id={`agents-tab-${t.id}`}
                aria-selected={tab === t.id}
                aria-controls={`agents-panel-${t.id}`}
                onClick={() => setTab(t.id)}
                className={[
                  "tandem-tap flex h-7 items-center gap-1.5 rounded px-2.5 text-[12px] font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/40",
                  tab === t.id ? "bg-ink/[0.06] text-ink" : "text-ink/50 hover:bg-ink/[0.05] hover:text-ink",
                ].join(" ")}
              >
                {t.label}
                {t.id === "feed" && tab !== "feed" && feed.unseen > 0 && (
                  <span
                    aria-label={`${feed.unseen} new`}
                    className="flex h-4 min-w-4 items-center justify-center rounded-sm bg-agent px-1 text-[10px] font-semibold leading-none text-paper tabular-nums"
                  >
                    {feed.unseen > 9 ? "9+" : feed.unseen}
                  </span>
                )}
              </button>
            ))}
          </div>
        </header>

        {tab === "feed" && (
          <div
            role="tabpanel"
            id="agents-panel-feed"
            aria-labelledby="agents-tab-feed"
            className="mx-auto flex min-h-0 w-full max-w-3xl flex-1 flex-col"
          >
            <ActivityFeed
              events={feed.events}
              loading={feed.loading}
              error={feed.error}
              onReload={() => void feed.reload()}
              onOpenTask={onOpenTask}
              actorFilter={actorFilter}
              onFilterChange={setActorFilter}
            />
          </div>
        )}

        <div
          role="tabpanel"
          id="agents-panel-roster"
          aria-labelledby="agents-tab-roster"
          hidden={tab !== "roster"}
          className={tab === "roster" ? "tandem-fab-reserve min-h-0 flex-1 overflow-y-auto" : "hidden"}
        >
          {/* What the fleet is waiting on. The blocker is you — so it leads. */}
          {proposedCount > 0 && (
            <button
              onClick={onOpenBoard}
              className="flex w-full items-center gap-2 border-b border-ink/10 bg-state-proposed/[0.06] px-4 py-2 text-left transition-colors hover:bg-state-proposed/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-accent/40 sm:px-5"
            >
              <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-state-proposed" aria-hidden="true" />
              <span className="min-w-0 flex-1 truncate text-[12.5px] text-ink/80">
                <span className="font-medium">
                  {proposedCount} task{proposedCount === 1 ? "" : "s"} awaiting your approval
                </span>
                <span className="text-ink/55">
                  {waitingCount + unnamedWaiting > 0
                    ? ` — ${waitingCount + unnamedWaiting} agent${waitingCount + unnamedWaiting === 1 ? " is" : "s are"} parked on the queue; approving wakes ${waitingCount + unnamedWaiting === 1 ? "it" : "them"}.`
                    : workingCount === 0
                      ? " — nothing is running; the fleet is waiting on you."
                      : ""}
                </span>
              </span>
              <span className="shrink-0 text-[12px] font-medium text-ink underline-offset-2 hover:underline">
                Review
              </span>
            </button>
          )}

          {/* A failed refresh keeps the last roster on screen — stale rows beat
              a blank surface while the next ping or poll retries. */}
          {error && roster && (
            <div className="flex items-center gap-2 border-b border-ink/10 px-4 py-1.5 sm:px-5">
              <span className="min-w-0 flex-1 truncate text-[12px] text-ink/55">
                Couldn't refresh — showing the last roster.
              </span>
              <button
                onClick={() => void reload()}
                className="shrink-0 text-[12px] font-medium text-accent hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/40"
              >
                Retry
              </button>
            </div>
          )}

          {error && !roster ? (
            <div className="px-4 py-6 sm:px-5">
              <p className="text-[13px] leading-relaxed text-ink/70">{error}</p>
              <button
                onClick={() => void reload()}
                className="mt-2 h-7 rounded border border-ink/15 px-2.5 text-[12px] font-medium text-ink/80 transition-colors hover:border-ink/25 hover:bg-ink/[0.04] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/40"
              >
                Try again
              </button>
            </div>
          ) : !roster && loading ? (
            <p className="px-4 py-6 text-[13px] text-ink/50 sm:px-5">Reading the roster…</p>
          ) : active.length === 0 ? (
            <div className="max-w-lg px-4 py-6 sm:px-5">
              <p className="text-[13px] font-medium text-ink/80">
                {dormant.length > 0 ? "Nobody is around right now." : "No agents here yet."}
              </p>
              <p className="mt-1 text-[12.5px] leading-relaxed text-ink/55">
                {dormant.length > 0
                  ? `${dormant.length} agent${dormant.length === 1 ? " has" : "s have"} worked this canvas before. Start a session and it rejoins the list here.`
                  : "Point a session at this canvas over MCP. It appears the moment it registers, and every task it claims shows up here."}
              </p>
              <button
                onClick={onConnect}
                className="mt-3 h-8 rounded bg-primary px-3 text-[13px] font-medium text-primary-fg transition-colors hover:bg-primary/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/40 focus-visible:ring-offset-2 focus-visible:ring-offset-paper"
              >
                Connect an agent
              </button>
            </div>
          ) : (
            <>
              {tree.roots.length > 0 && (
                <ul className="divide-y divide-ink/[0.07]">
                  {tree.roots.map((a) => (
                    <AgentRow key={agentKey(a)} agent={a} depth={0} ctx={rowCtx} />
                  ))}
                </ul>
              )}
              {/* Holding claims nobody is working: every lease lapsed (TDM-24).
                  Below the live fleet, each claim still carrying its warning. */}
              {stalledTree.roots.length > 0 && (
                <div className={tree.roots.length > 0 ? "border-t border-ink/10" : undefined}>
                  <p className="px-4 pb-1 pt-2 text-[11px] font-medium text-ink/55 sm:px-5">
                    Stalled
                    <span className="font-normal text-ink/45"> — holding claims whose lease lapsed</span>
                  </p>
                  <ul className="divide-y divide-ink/[0.07]">
                    {stalledTree.roots.map((a) => (
                      <AgentRow
                        key={agentKey(a)}
                        agent={a}
                        depth={0}
                        ctx={{ ...rowCtx, childrenById: stalledTree.childrenById }}
                      />
                    ))}
                  </ul>
                </div>
              )}
            </>
          )}

          {/* Everyone who has registered here but isn't around right now. */}
          {dormant.length > 0 && (
            <div className="border-t border-ink/10">
              <button
                onClick={() => setShowDormant((s) => !s)}
                aria-expanded={showDormant}
                className="flex w-full items-center gap-1.5 px-4 py-2 text-left text-[12px] text-ink/50 transition-colors hover:bg-ink/[0.03] hover:text-ink/75 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-accent/40 sm:px-5"
              >
                {showDormant ? "Hide" : "Show"} {dormant.length} dormant
                {!showDormant && <span className="text-ink/45">— registered here, not around now</span>}
              </button>
              {showDormant && (
                <ul className="divide-y divide-ink/[0.07] pb-1">
                  {dormantTree.roots.map((a) => (
                    <AgentRow
                      key={agentKey(a)}
                      agent={a}
                      depth={0}
                      ctx={{ ...rowCtx, childrenById: dormantTree.childrenById }}
                      dormant
                    />
                  ))}
                </ul>
              )}
            </div>
          )}
        </div>
      </section>

      {watched && (
        <AgentDetail
          agent={watched}
          claims={claimsOf(watched)}
          now={now}
          onClose={() => onWatchAgent(null)}
          onOpenTask={onOpenTask}
          renderAgentWatch={renderAgentWatch}
        />
      )}
    </div>
  );
}

function buildTree(list: FleetAgent[]) {
  const byId = new Map<string, FleetAgent>();
  for (const a of list) if (a.id) byId.set(a.id, a);
  const childrenById = new Map<string, FleetAgent[]>();
  const roots: FleetAgent[] = [];
  for (const a of list) {
    const pid = a.parentAgentId;
    if (!pid || pid === a.id || !byId.has(pid)) {
      roots.push(a);
      continue;
    }
    const kids = childrenById.get(pid) ?? [];
    kids.push(a);
    childrenById.set(pid, kids);
  }
  return { roots, childrenById };
}

// ── rows ─────────────────────────────────────────────────────────────────────

interface RowCtx {
  childrenById: Map<string, FleetAgent[]>;
  claimsOf: (a: FleetAgent) => LiveClaim[];
  holdOf: (a: FleetAgent) => FleetHold | null;
  lastDone: Map<string, Action>;
  contention: Map<string, ContentionTally>;
  now: number;
  watchedAgentId: string | null;
  onWatch: (a: FleetAgent) => void;
  onOpenTask: (taskId: string) => void;
}

/* One fleet member, plus any executors it spawned (nested under a hairline
   rail — the same hierarchy idiom the board uses for epic membership). */
function AgentRow({
  agent,
  depth,
  ctx,
  dormant = false,
}: {
  agent: FleetAgent;
  depth: number;
  ctx: RowCtx;
  dormant?: boolean;
}) {
  const vendor = vendorOf(agent);
  const kids = depth < MAX_DEPTH && agent.id ? (ctx.childrenById.get(agent.id) ?? []) : [];
  const claims = ctx.claimsOf(agent);
  const working = claims.length > 0;
  const stalled = ctx.holdOf(agent) === "stalled";
  const done = ctx.lastDone.get(agent.name) ?? (agent.id ? ctx.lastDone.get(agent.id) : undefined);
  // Parked on the queue long poll RIGHT NOW (TDM-151) — ranks below a claim.
  const waiting = !working ? agent.waiting : undefined;
  const raced = ctx.contention.get(agent.name) ?? (agent.id ? ctx.contention.get(agent.id) : undefined);
  const selected = !!agent.id && agent.id === ctx.watchedAgentId;
  const watchable = !!agent.id;

  const identity = (
    <>
      <StatusDot working={working && !stalled} stalled={stalled} waiting={!!waiting} done={!working && !waiting ? done : undefined} dormant={dormant} />
      <span className="min-w-0 truncate font-code text-[12.5px] font-medium text-ink" title={agent.name}>
        {agent.name}
      </span>
      {agent.role && agent.role !== "agent" && (
        <span className="shrink-0 text-[11px] text-ink/50">{agent.role}</span>
      )}
      {raced && <ContentionCount tally={raced} name={agent.name} />}
      <span
        title={vendor.title}
        className={`ml-auto max-w-[45%] shrink-0 truncate rounded-sm px-1 py-px font-code text-[10.5px] ${VENDOR_CLS[vendor.kind]}`}
      >
        {vendor.label}
      </span>
    </>
  );

  return (
    <li className={depth > 0 ? "border-t border-ink/[0.07] first:border-t-0" : undefined}>
      <div className={`${dormant ? "py-1.5" : "py-2"} pl-4 pr-4 sm:pr-5 ${selected ? "bg-ink/[0.05]" : ""}`}>
        {/* Identity line: state · name · role · model. Clicking it watches the
            agent (opens the detail pane). Unregistered claimants have no id and
            so no session to watch — their line is static. */}
        {watchable ? (
          <button
            onClick={() => ctx.onWatch(agent)}
            aria-pressed={selected}
            title={selected ? `Close ${agent.name}` : `Watch ${agent.name}`}
            className="-mx-1 flex w-[calc(100%+0.5rem)] items-center gap-2 rounded px-1 py-0.5 text-left transition-colors hover:bg-ink/[0.04] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/40"
          >
            {identity}
          </button>
        ) : (
          <div className="flex items-center gap-2 py-0.5">{identity}</div>
        )}

        {/* Status: one block per claim, or a single waiting / done / idle line. */}
        <div className="mt-0.5 flex flex-col gap-1 pl-[14px]">
          {working ? (
            claims.map((c) => (
              <ClaimBlock key={c.id} claim={c} agentName={agent.name} now={ctx.now} onOpen={ctx.onOpenTask} />
            ))
          ) : waiting ? (
            <WaitingLine wait={waiting} agentName={agent.name} />
          ) : done ? (
            <DoneLine action={done} agentName={agent.name} onOpen={ctx.onOpenTask} />
          ) : (
            <div className="flex items-center gap-2 py-px">
              <span className="min-w-0 flex-1 text-[12px] text-ink/50">idle</span>
              <span
                className="shrink-0 font-code text-[11px] text-ink/50"
                title={agent.lastActivityAt ? `Last active ${fullDate(agent.lastActivityAt)}` : "No recorded activity"}
              >
                <span className="sr-only">last active </span>
                {ageOf(agent.lastActivityAt)}
                <span className="sr-only"> ago</span>
              </span>
            </div>
          )}
        </div>
      </div>

      {kids.length > 0 && (
        <ul className="ml-[22px] border-l border-ink/10">
          {kids.map((k) => (
            <AgentRow key={agentKey(k)} agent={k} depth={depth + 1} ctx={ctx} dormant={dormant} />
          ))}
        </ul>
      )}
    </li>
  );
}

/* A live claim: ticket + title (click → the task on the Board), lease health,
   held-for; then the newest task_progress line; then — only once the lease has
   lapsed — the spelled-out warning. */
function ClaimBlock({
  claim,
  agentName,
  now,
  onOpen,
}: {
  claim: LiveClaim;
  agentName: string;
  now: number;
  onOpen: (taskId: string) => void;
}) {
  const lease = deriveLease(claim.action, now);
  const last = newestProgress(claim.progress);
  return (
    <div className="min-w-0">
      <button
        onClick={() => onOpen(claim.id)}
        title={`${claim.ticketId ? `${claim.ticketId} — ` : ""}${claim.title}\nOpen on the Board`}
        aria-label={`${agentName} is working ${claim.ticketId ?? "a task"}, ${claim.title}, held for ${ageOf(claim.startedAt)}${
          lease.health === "stale" ? " — claim lapsed, reclaimable" : lease.health === "slipping" ? " — no recent heartbeat" : ""
        }. Open it on the board.`}
        className="-mx-1 flex w-[calc(100%+0.5rem)] items-center gap-2 rounded px-1 py-px text-left transition-colors hover:bg-ink/[0.04] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/40"
      >
        {claim.ticketId && (
          <span className="shrink-0 font-code text-[11px] font-medium text-state-executing">{claim.ticketId}</span>
        )}
        <span className="min-w-0 flex-1 truncate text-[12.5px] text-ink/80">{claim.title}</span>
        <LeaseChip lease={lease} />
        <span
          className="w-[52px] shrink-0 text-right font-code text-[11px] text-ink/50"
          title={claim.startedAt ? `Held since ${fullDate(claim.startedAt)}` : undefined}
        >
          <span className="sr-only">held </span>
          {ageOf(claim.startedAt)}
        </span>
      </button>
      {last?.note && (
        <div className="flex items-baseline gap-2 pl-1" title={last.note}>
          <span aria-hidden="true" className="shrink-0 font-code text-[11px] text-ink/30">
            ↳
          </span>
          <span className="min-w-0 flex-1 truncate text-[12px] text-ink/60">{last.note}</span>
          <span className="w-[52px] shrink-0 text-right font-code text-[11px] text-ink/45" title={fullDate(last.at)}>
            <span className="sr-only">reported </span>
            {ageOf(last.at)}
          </span>
        </div>
      )}
      {lease.health === "stale" && <StaleWarning lease={lease} />}
    </div>
  );
}

function StaleWarning({ lease }: { lease: Lease }) {
  return (
    <p className="mt-0.5 flex items-start gap-1.5 pl-1 text-[11.5px] leading-snug text-state-proposed" title={leaseSentence(lease)}>
      <AlertTriangle size={12} className="mt-px shrink-0" aria-hidden="true" />
      <span className="min-w-0">
        Claim lease lapsed{lease.overdueMs >= 60_000 ? ` ${leaseAge(lease.overdueMs)} ago` : ""} — the next agent to ask takes it over.
      </span>
    </p>
  );
}

/* ContentionCount — how often this member has been refused (TDM-100).
     ⛨ n  FENCED  writes refused because it no longer held the claim — the near
                  miss worth a human's attention.
     ⌾ n  RACED   claims it lost at the door; routine on a busy queue.
   Both shown, fenced first, each hidden at zero. No hue — weight carries it. */
function ContentionCount({ tally, name }: { tally: ContentionTally; name: string }) {
  if (tally.total === 0) return null;
  const title = [
    tally.fenced > 0
      ? `${tally.fenced} write${tally.fenced === 1 ? "" : "s"} by ${name} refused because it no longer held the claim — its lease had lapsed and the task had moved on.`
      : "",
    tally.raced > 0
      ? `${tally.raced} time${tally.raced === 1 ? "" : "s"} ${name} asked for a task another agent already held, and yielded.`
      : "",
  ]
    .filter(Boolean)
    .join(" ");
  return (
    <span className="flex shrink-0 items-center gap-1.5" title={title}>
      {tally.fenced > 0 && (
        <span className="inline-flex items-center gap-0.5 font-code text-[10.5px] font-medium text-ink/70">
          <Shield size={10} className="shrink-0" aria-hidden="true" />
          {tally.fenced}
        </span>
      )}
      {tally.raced > 0 && (
        <span className="inline-flex items-center gap-0.5 font-code text-[10.5px] text-ink/45">
          <Users size={10} className="shrink-0" aria-hidden="true" />
          {tally.raced}
        </span>
      )}
      <span className="sr-only">{title}</span>
    </span>
  );
}

function StatusDot({
  working,
  stalled = false,
  waiting,
  done,
  dormant,
}: {
  working: boolean;
  stalled?: boolean;
  waiting: boolean;
  done?: Action;
  dormant: boolean;
}) {
  // Working and waiting are the two live states this surface lets breathe.
  if (working) return <span aria-hidden="true" className="tandem-breathe h-1.5 w-1.5 shrink-0 rounded-full bg-state-executing" />;
  // Stalled holds claims but nothing is moving — so it does NOT breathe.
  if (stalled) return <span aria-hidden="true" className="h-1.5 w-1.5 shrink-0 rounded-full border border-state-proposed" />;
  if (waiting) return <span aria-hidden="true" className="tandem-breathe h-1.5 w-1.5 shrink-0 rounded-full bg-state-proposed" />;
  if (done) {
    return (
      <span
        aria-hidden="true"
        className={`h-1.5 w-1.5 shrink-0 rounded-full ${done.state === "failed" ? "bg-state-failed" : "bg-state-done"}`}
      />
    );
  }
  return (
    <span
      aria-hidden="true"
      className={`h-1.5 w-1.5 shrink-0 rounded-full border ${dormant ? "border-ink/20" : "border-ink/35"}`}
    />
  );
}

/* Parked on the queue, blocked on a human (TDM-151). Not a button: a wait points
   at no one task — the approval bar at the top of the roster is the action. */
function WaitingLine({ wait, agentName }: { wait: FleetWait; agentName: string }) {
  const what = wait.scope === "epic" ? "waiting for its batch to be approved" : "waiting for approved work";
  return (
    <div
      className="flex items-center gap-2 py-px"
      title={`${agentName} is holding the queue open, ${what} — since ${fullDate(wait.since)}.\nIt is blocked on you: approve something on the Board and it wakes.`}
    >
      <span className="min-w-0 flex-1 truncate text-[12px] text-state-proposed">{what}</span>
      <span className="shrink-0 font-code text-[11px] text-ink/50">
        <span className="sr-only">waiting for </span>
        {ageOf(wait.since)}
      </span>
    </div>
  );
}

// The last thing this agent finished, while it's still recent enough to matter.
function DoneLine({
  action,
  agentName,
  onOpen,
}: {
  action: Action;
  agentName: string;
  onOpen: (taskId: string) => void;
}) {
  const failed = action.state === "failed";
  const title = (action.payload as { title?: string }).title || "Untitled task";
  return (
    <button
      onClick={() => onOpen(action.id)}
      title={`${action.ticketId ? `${action.ticketId} — ` : ""}${title}\nOpen on the Board`}
      aria-label={`${agentName} ${failed ? "failed" : "finished"} ${action.ticketId ?? "a task"}, ${title}, ${ageOf(action.updatedAt)} ago. Open it on the board.`}
      className="-mx-1 flex w-[calc(100%+0.5rem)] items-center gap-2 rounded px-1 py-px text-left transition-colors hover:bg-ink/[0.04] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/40"
    >
      <span className="shrink-0 text-[12px] text-ink/50">{failed ? "failed" : "finished"}</span>
      {action.ticketId && (
        <span className={`shrink-0 font-code text-[11px] font-medium ${failed ? "text-state-failed" : "text-state-done"}`}>
          {action.ticketId}
        </span>
      )}
      <span className="min-w-0 flex-1 truncate text-[12.5px] text-ink/60">{title}</span>
      <span className="w-[52px] shrink-0 text-right font-code text-[11px] text-ink/50" title={fullDate(action.updatedAt)}>
        {ageOf(action.updatedAt)}
      </span>
    </button>
  );
}

// ── detail pane (the TDM-19 hook) ────────────────────────────────────────────

/* The watched agent: identity, every claim with its newest progress line, and the
   live-session slot. TDM-19 renders the Claude Code transcript into that slot by
   passing `renderAgentWatch` from App; until then it shows a placeholder.

   Layout (TDM-25): the transcript is the pane's main content. The body below the
   header is a non-scrolling flex column; the Holding section is capped at 40% of
   it and scrolls on its own, and the Live session section flex-fills the rest
   (so it never gets less than ~60%), handing AgentWatch a bounded height so its
   own log scroller, auto-scroll and Jump-to-latest keep working. */
function AgentDetail({
  agent,
  claims,
  now,
  onClose,
  onOpenTask,
  renderAgentWatch,
}: {
  agent: FleetAgent;
  claims: LiveClaim[];
  now: number;
  onClose: () => void;
  onOpenTask: (taskId: string) => void;
  renderAgentWatch?: (agent: FleetAgent) => ReactNode;
}) {
  const vendor = vendorOf(agent);
  // Claims whose earlier progress lines are disclosed. Keyed by task id, so a
  // stale entry from a previously watched agent is harmless.
  const [openLogs, setOpenLogs] = useState<Set<string>>(() => new Set());
  const toggleLog = (id: string) =>
    setOpenLogs((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  return (
    <aside
      aria-label={`${agent.name} detail`}
      className="flex min-h-0 w-full min-w-0 flex-col border-ink/10 bg-paper sm:w-[min(460px,45%)] sm:border-l"
    >
      <header className="flex shrink-0 items-center gap-2 border-b border-ink/10 px-3 py-2.5 sm:px-4">
        <button
          onClick={onClose}
          aria-label="Back to the roster"
          className="flex h-8 w-8 shrink-0 items-center justify-center rounded text-ink/50 transition-colors hover:bg-ink/[0.05] hover:text-ink focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/40 sm:hidden"
        >
          <ArrowLeft size={16} />
        </button>
        <div className="min-w-0 flex-1">
          <p className="truncate font-code text-[13px] font-medium text-ink" title={agent.name}>
            {agent.name}
          </p>
          <p className="mt-0.5 flex min-w-0 items-center gap-2 text-[11.5px] text-ink/50">
            {agent.role && <span className="shrink-0">{agent.role}</span>}
            <span title={vendor.title} className={`min-w-0 truncate rounded-sm px-1 py-px font-code text-[10.5px] ${VENDOR_CLS[vendor.kind]}`}>
              {vendor.label}
            </span>
            <span className="shrink-0 font-code" title={agent.lastActivityAt ? fullDate(agent.lastActivityAt) : undefined}>
              active {ageOf(agent.lastActivityAt)} ago
            </span>
          </p>
        </div>
        <button
          onClick={onClose}
          aria-label="Close agent detail"
          className="hidden h-7 w-7 shrink-0 items-center justify-center rounded text-ink/45 transition-colors hover:bg-ink/[0.05] hover:text-ink focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/40 sm:flex"
        >
          <X size={15} />
        </button>
      </header>

      <div className="flex min-h-0 flex-1 flex-col">
        <section className="max-h-[40%] min-h-0 shrink-0 overflow-y-auto border-b border-ink/10 px-3 py-3 sm:px-4">
          <h3 className="text-[11px] font-semibold uppercase tracking-wide text-ink/45">
            Holding {claims.length > 0 ? claims.length : ""}
          </h3>
          {claims.length === 0 ? (
            <p className="mt-1.5 text-[12.5px] text-ink/55">
              {agent.waiting ? "Nothing — parked on the queue, waiting for approved work." : "No claims right now."}
            </p>
          ) : (
            <ul className="mt-1.5 flex flex-col gap-3">
              {claims.map((c) => {
                const lease = deriveLease(c.action, now);
                const log = [...c.progress].sort((x, y) => (Date.parse(y.at) || 0) - (Date.parse(x.at) || 0));
                const logOpen = openLogs.has(c.id);
                const earlier = log.length - 1;
                const shown = logOpen ? log : log.slice(0, 1);
                return (
                  <li key={c.id} className="min-w-0">
                    <button
                      onClick={() => onOpenTask(c.id)}
                      className="-mx-1 flex w-[calc(100%+0.5rem)] items-center gap-2 rounded px-1 py-px text-left transition-colors hover:bg-ink/[0.04] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/40"
                      title="Open on the Board"
                    >
                      {c.ticketId && (
                        <span className="shrink-0 font-code text-[11px] font-medium text-state-executing">{c.ticketId}</span>
                      )}
                      <span className="min-w-0 flex-1 truncate text-[12.5px] font-medium text-ink/85">{c.title}</span>
                      <LeaseChip lease={lease} />
                    </button>
                    <p className="mt-0.5 font-code text-[11px] text-ink/45">
                      held {ageOf(c.startedAt)}
                      {c.startedAt ? ` · since ${fullDate(c.startedAt)}` : ""}
                    </p>
                    {lease.health !== "none" && (
                      <p className={`mt-0.5 text-[11.5px] leading-snug ${lease.health === "stale" ? "text-state-proposed" : "text-ink/50"}`}>
                        {leaseSentence(lease)}
                      </p>
                    )}
                    {log.length > 0 && (
                      <ol className="mt-1.5 flex flex-col gap-1 border-l border-ink/10 pl-2.5">
                        {shown.map((e, i) => (
                          <li key={`${e.at}-${i}`} className="flex items-baseline gap-2">
                            <span className="min-w-0 flex-1 text-[12px] leading-snug text-ink/70">{e.note}</span>
                            <span className="shrink-0 font-code text-[10.5px] text-ink/40" title={fullDate(e.at)}>
                              {ageOf(e.at)}
                            </span>
                          </li>
                        ))}
                      </ol>
                    )}
                    {earlier > 0 && (
                      <button
                        onClick={() => toggleLog(c.id)}
                        aria-expanded={logOpen}
                        className="-mx-1 mt-1 rounded px-1 py-px text-[11.5px] text-ink/50 transition-colors hover:bg-ink/[0.04] hover:text-ink/75 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/40"
                      >
                        {logOpen ? "Hide earlier" : `Show ${earlier} earlier`}
                      </button>
                    )}
                  </li>
                );
              })}
            </ul>
          )}
        </section>

        {/* TDM-19 slot: the live Claude Code transcript for this agent. */}
        <section className="flex min-h-0 flex-1 flex-col px-3 py-3 sm:px-4" data-agent-watch-slot={agent.id}>
          <h3 className="shrink-0 text-[11px] font-semibold uppercase tracking-wide text-ink/45">Live session</h3>
          {renderAgentWatch ? (
            <div className="mt-1.5 flex min-h-0 flex-1 flex-col">{renderAgentWatch(agent)}</div>
          ) : (
            <div className="mt-1.5 rounded border border-dashed border-ink/15 px-2.5 py-2 text-[12px] leading-snug text-ink/50">
              The agent's live session transcript will appear here.
            </div>
          )}
        </section>
      </div>
    </aside>
  );
}
