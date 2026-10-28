import { useCallback, useEffect, useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { ArrowDown, ChevronRight, Terminal } from "lucide-react";
import {
  fetchAgentTranscript,
  TranscriptError,
  type TranscriptEvent,
  type TranscriptInfo,
} from "../lib/api";
import { useConfig } from "../lib/config";
import { fullDate } from "../lib/relativeTime";

/* ─────────────────────────────────────────────────────────────────────────────
   AgentWatch — the live Claude Code transcript of ONE agent (TDM-19), rendered
   into FleetView's "Live session" slot.

   THE READ. GET /api/canvas/agents/{id}/transcript (TDM-18) pages the agent's
   newest Claude Code .jsonl by BYTE OFFSET. We open with `tail=100`, then follow
   with `after=<next>&transcript=<id>` every ~1.5s. A `more:true` page drains
   straight away; a `reset:true` page (the agent moved to a newer file) is thrown
   away and the pane re-opens on the new file with a fresh tail rather than
   paging the whole thing from byte 0.

   ONE REQUEST AT A TIME. The loop awaits each answer before it sleeps, so a
   slow read (the first lookup for an agent with no transcript is a cold scan of
   ~/.claude and can take seconds) never stacks a second poll behind it. The
   loop PARKS — no timers, no requests — while the tab is hidden (Page
   Visibility API) or the Agents surface isn't on screen, and wakes the moment
   it is again.

   It never grows forever: the in-memory list keeps the newest MAX_EVENTS.
   ──────────────────────────────────────────────────────────────────────────── */

const TAIL = 100;
const POLL_MS = 1500;
// No linked transcript yet: check again, but less eagerly — every miss may be a scan.
const NOT_FOUND_MS = 4000;
const ERROR_MS = 5000;
const MAX_EVENTS = 2000;
// "At the bottom" tolerance, px — a sub-line scroll offset still counts as following.
const STICK_PX = 32;

type Phase = "looking" | "not_found" | "live";
type Fatal = { code: string; message: string };

// Refusals that no amount of polling will change.
const FATAL_CODES = new Set(["transcripts_local_only", "transcripts_disabled", "agent_not_found", "invalid_agent_id"]);

interface Props {
  code: string;
  agentId: string;
  agentName: string;
  /** The agent's declared model — sharpens the "no transcript" explanation. */
  model?: string;
  /** False while the Agents surface is hidden (kept alive by CSS): the loop parks. */
  active: boolean;
}

function usePageVisible(): boolean {
  const [visible, setVisible] = useState(() => typeof document === "undefined" || document.visibilityState !== "hidden");
  useEffect(() => {
    const onChange = () => setVisible(document.visibilityState !== "hidden");
    document.addEventListener("visibilitychange", onChange);
    return () => document.removeEventListener("visibilitychange", onChange);
  }, []);
  return visible;
}

export default function AgentWatch({ code, agentId, agentName, model, active }: Props) {
  const config = useConfig();
  const enabled = config.transcripts;
  const pageVisible = usePageVisible();
  const runnable = enabled && active && pageVisible;

  const [phase, setPhase] = useState<Phase>("looking");
  const [fatal, setFatal] = useState<Fatal | null>(null);
  const [netError, setNetError] = useState<string | null>(null);
  const [info, setInfo] = useState<TranscriptInfo | null>(null);
  const [events, setEvents] = useState<TranscriptEvent[]>([]);
  const [dropped, setDropped] = useState(false);

  // The loop reads these through refs so visibility flips don't restart it.
  const runnableRef = useRef(runnable);
  const wakeRef = useRef<(() => void) | null>(null);
  runnableRef.current = runnable;
  useEffect(() => {
    if (runnable && wakeRef.current) {
      const wake = wakeRef.current;
      wakeRef.current = null;
      wake();
    }
  }, [runnable]);

  useEffect(() => {
    if (!enabled) return;
    let cancelled = false;
    let timer: number | undefined;
    let sleepDone: (() => void) | null = null;
    // Where the next read starts. null = open (or re-open) with a tail.
    let cursor: { transcript: string; next: number } | null = null;
    // How many events the list holds — the loop is its only writer.
    let held = 0;

    setPhase("looking");
    setFatal(null);
    setNetError(null);
    setInfo(null);
    setEvents([]);
    setDropped(false);

    const sleep = (ms: number) =>
      new Promise<void>((resolve) => {
        sleepDone = resolve;
        timer = window.setTimeout(resolve, ms);
      });
    const untilRunnable = () =>
      runnableRef.current
        ? Promise.resolve()
        : new Promise<void>((resolve) => {
            wakeRef.current = resolve;
          });

    void (async () => {
      while (!cancelled) {
        await untilRunnable();
        if (cancelled) return;
        let delay = POLL_MS;
        try {
          const from = cursor;
          const page = await fetchAgentTranscript(
            code,
            agentId,
            from ? { after: from.next, transcript: from.transcript } : { tail: TAIL },
          );
          if (cancelled) return;
          setNetError(null);
          if (!page.found || !page.transcript) {
            cursor = null;
            setPhase("not_found");
            setInfo(null);
            setEvents([]);
            held = 0;
            delay = NOT_FOUND_MS;
          } else if (from && page.reset) {
            // A different (newer) file, or this one was rewritten: re-open on a tail.
            cursor = null;
            delay = 0;
          } else {
            cursor = { transcript: page.transcript.id, next: page.next };
            setInfo(page.transcript);
            setPhase("live");
            const fresh = page.events ?? [];
            if (!from) {
              held = Math.min(fresh.length, MAX_EVENTS);
              setEvents(fresh.slice(-MAX_EVENTS));
              setDropped(fresh.length > 0 && fresh[0].offset > 0);
            } else if (fresh.length > 0) {
              held += fresh.length;
              if (held > MAX_EVENTS) {
                held = MAX_EVENTS;
                setDropped(true);
              }
              setEvents((prev) => {
                const merged = prev.concat(fresh);
                return merged.length <= MAX_EVENTS ? merged : merged.slice(merged.length - MAX_EVENTS);
              });
            }
            if (page.more) delay = 0;
          }
        } catch (e) {
          if (cancelled) return;
          if (e instanceof TranscriptError && FATAL_CODES.has(e.code)) {
            setFatal({ code: e.code, message: e.message });
            return;
          }
          setNetError(e instanceof Error ? e.message : "Could not reach the server");
          delay = ERROR_MS;
        }
        if (delay > 0) await sleep(delay);
      }
    })();

    return () => {
      cancelled = true;
      window.clearTimeout(timer);
      sleepDone?.();
      const wake = wakeRef.current;
      wakeRef.current = null;
      wake?.();
    };
  }, [code, agentId, enabled]);

  // ── follow the bottom unless the reader scrolled up ──────────────────────
  const scrollerRef = useRef<HTMLDivElement>(null);
  const stickRef = useRef(true);
  const [stuck, setStuck] = useState(true);
  const onScroll = useCallback(() => {
    const el = scrollerRef.current;
    if (!el) return;
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < STICK_PX;
    stickRef.current = atBottom;
    setStuck(atBottom);
  }, []);
  const jumpToLatest = useCallback(() => {
    const el = scrollerRef.current;
    if (!el) return;
    el.scrollTop = el.scrollHeight;
    stickRef.current = true;
    setStuck(true);
  }, []);
  useLayoutEffect(() => {
    const el = scrollerRef.current;
    if (el && stickRef.current) el.scrollTop = el.scrollHeight;
  }, [events]);

  const [expanded, setExpanded] = useState<Set<string>>(() => new Set());
  const toggle = useCallback((id: string) => {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  // ── states that aren't a log ─────────────────────────────────────────────
  if (!enabled) {
    return config.local ? (
      <Notice title="Transcripts are turned off">
        This server runs with <Code>TANDEM_TRANSCRIPTS=off</Code>, so it doesn't read Claude Code session files.
        Restart it without that setting to watch agents live from the board.
      </Notice>
    ) : (
      <Notice title="Transcripts are local-only">
        Live sessions are read from Claude Code's files on the machine running Tandem, so they are only served in
        local mode. This server runs with Google sign-in (<Code>TANDEM_AUTH=google</Code>), where a canvas reader must
        never see the operator's history.
      </Notice>
    );
  }
  if (fatal) {
    if (fatal.code === "transcripts_local_only") {
      return (
        <Notice title="Transcripts are local-only">
          This server only serves Claude Code transcripts in local mode (<Code>TANDEM_AUTH=none</Code>).
        </Notice>
      );
    }
    if (fatal.code === "transcripts_disabled") {
      return (
        <Notice title="Transcripts are turned off">
          This server runs with <Code>TANDEM_TRANSCRIPTS=off</Code>. Restart it without that setting to watch agents
          live.
        </Notice>
      );
    }
    return (
      <Notice title="Agent not found">
        {agentName} isn't registered on this canvas any more, so there is no session to read.
      </Notice>
    );
  }
  if (phase === "looking") {
    return (
      <Notice title="Looking for this agent's session…" live={runnable}>
        Searching Claude Code's session files for the one {agentName} connected from. The first look can take a few
        seconds.
        {netError && <ErrorLine message={netError} />}
      </Notice>
    );
  }
  if (phase === "not_found") {
    const nonClaude = !!model && !/claude/i.test(model);
    return (
      <Notice title="No Claude Code session linked" live={runnable}>
        {nonClaude ? (
          <>
            {agentName} declared <Code>{model}</Code>, so it probably isn't a Claude Code session — only Claude Code
            transcripts can be shown here.
          </>
        ) : (
          <>
            No Claude Code session on this machine names {agentName}. It may run in another client, on another machine,
            or it hasn't connected from a Claude Code session yet. This pane picks the session up as soon as one
            appears.
          </>
        )}
        {netError && <ErrorLine message={netError} />}
      </Notice>
    );
  }

  // ── the log ──────────────────────────────────────────────────────────────
  const sub = info?.subagent;
  const lastModel = sub?.model || [...events].reverse().find((e) => e.model)?.model;
  return (
    <div className="flex min-h-0 flex-col rounded-md border border-ink/10 bg-surface">
      <header className="flex shrink-0 flex-col gap-0.5 border-b border-ink/10 px-2.5 py-2">
        <div className="flex min-w-0 items-center gap-1.5">
          <span
            className={`shrink-0 rounded-sm px-1 py-px font-code text-[10.5px] ${
              info?.kind === "subagent" ? "bg-agent/10 text-agent" : "bg-ink/[0.06] text-ink/60"
            }`}
          >
            {info?.kind ?? "session"}
          </span>
          {info?.project && (
            <span className="min-w-0 flex-1 truncate font-code text-[11px] text-ink/60" title={info.path}>
              {info.project}
            </span>
          )}
          {sub?.finished ? (
            <span className="shrink-0 rounded-sm bg-state-done/10 px-1 py-px font-code text-[10.5px] text-state-done">
              finished
            </span>
          ) : (
            <LiveDot live={runnable} />
          )}
        </div>
        {(sub?.description || sub?.agentType || lastModel) && (
          <p className="flex min-w-0 items-baseline gap-1.5 text-[11.5px] text-ink/55">
            {sub?.agentType && <span className="shrink-0 font-code text-[10.5px] text-ink/45">{sub.agentType}</span>}
            {sub?.description && <span className="min-w-0 flex-1 truncate">{sub.description}</span>}
            {lastModel && <span className="ml-auto shrink-0 font-code text-[10.5px] text-ink/45">{lastModel}</span>}
          </p>
        )}
        {netError && <ErrorLine message={netError} />}
      </header>

      <div className="relative min-h-0">
        <div
          ref={scrollerRef}
          onScroll={onScroll}
          className="h-[min(60vh,560px)] min-h-[220px] overflow-y-auto px-2.5 py-2"
          role="log"
          aria-label={`${agentName} live session`}
        >
          {dropped && (
            <p className="mb-1.5 font-code text-[10.5px] text-ink/40">… earlier activity not shown</p>
          )}
          {events.length === 0 ? (
            <p className="font-code text-[11.5px] text-ink/45">No activity in this session yet.</p>
          ) : (
            <ol className="flex flex-col gap-1">
              {events.map((e) => (
                <EventRow key={e.id || e.offset} e={e} open={expanded.has(e.id)} onToggle={() => toggle(e.id)} />
              ))}
            </ol>
          )}
        </div>
        {!stuck && events.length > 0 && (
          <button
            onClick={jumpToLatest}
            className="absolute bottom-2 right-3 flex items-center gap-1 rounded border border-ink/15 bg-surface px-2 py-1 text-[11px] font-medium text-ink/75 shadow-lg transition-colors hover:bg-paper hover:text-ink focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/40"
          >
            <ArrowDown size={12} />
            Jump to latest
          </button>
        )}
      </div>
    </div>
  );
}

// ── rows ─────────────────────────────────────────────────────────────────────

function EventRow({ e, open, onToggle }: { e: TranscriptEvent; open: boolean; onToggle: () => void }) {
  const when = e.ts ? fullDate(e.ts) : undefined;
  switch (e.kind) {
    case "tool_use":
      return (
        <li className="flex min-w-0 items-baseline gap-1.5 font-code text-[11.5px] leading-snug" title={when}>
          <span className="shrink-0 text-state-executing" aria-hidden>
            ▸
          </span>
          <span className="shrink-0 font-medium text-ink/85">{e.tool || "tool"}</span>
          {e.input && <span className="min-w-0 flex-1 truncate text-ink/60" title={e.input}>{e.input}</span>}
        </li>
      );
    case "tool_result": {
      const text = e.text ?? "";
      const lines = text ? text.split("\n").length : 0;
      const first = text.split("\n").find((l) => l.trim()) ?? "";
      const tone = e.isError ? "text-state-failed" : "text-ink/45";
      return (
        <li className="min-w-0 pl-3" title={when}>
          <button
            onClick={onToggle}
            disabled={!text}
            aria-expanded={open}
            className={`-mx-1 flex w-[calc(100%+0.5rem)] min-w-0 items-baseline gap-1 rounded-sm px-1 text-left font-code text-[11px] leading-snug transition-colors enabled:hover:bg-ink/[0.04] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/40 ${tone}`}
          >
            <ChevronRight
              size={11}
              className={`shrink-0 self-center transition-transform ${open ? "rotate-90" : ""} ${text ? "" : "opacity-0"}`}
            />
            <span className="shrink-0">{e.isError ? "error" : "result"}</span>
            {lines > 0 && <span className="shrink-0 opacity-75">· {lines} {lines === 1 ? "line" : "lines"}</span>}
            {!open && first && <span className="min-w-0 flex-1 truncate opacity-75">{first}</span>}
          </button>
          {open && text && (
            <pre
              className={`mt-0.5 max-h-64 overflow-auto whitespace-pre-wrap break-words rounded-sm border border-ink/10 bg-paper px-2 py-1.5 font-code text-[11px] leading-snug ${
                e.isError ? "text-state-failed" : "text-ink/70"
              }`}
            >
              {text}
              {e.truncated && <Cut length={e.length} />}
            </pre>
          )}
        </li>
      );
    }
    case "user":
      return (
        <li className="min-w-0 border-l border-ink/15 pl-2 text-[11.5px] leading-snug text-ink/45" title={when}>
          <button
            onClick={onToggle}
            aria-expanded={open}
            className="block w-full text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/40"
          >
            <span className="mr-1 font-code text-[10.5px] uppercase tracking-wide text-ink/35">user</span>
            <span className={`whitespace-pre-wrap break-words ${open ? "" : "line-clamp-2"}`}>{e.text}</span>
            {open && e.truncated && <Cut length={e.length} />}
          </button>
        </li>
      );
    case "text":
    default:
      return (
        <li className="min-w-0 whitespace-pre-wrap break-words py-0.5 text-[12.5px] leading-relaxed text-ink/85" title={when}>
          {e.text}
          {e.truncated && <Cut length={e.length} />}
        </li>
      );
  }
}

function Cut({ length }: { length?: number }) {
  return (
    <span className="mt-0.5 block font-code text-[10.5px] text-ink/40">
      … cut{length ? ` (${formatBytes(length)} in full)` : ""}
    </span>
  );
}

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}

function LiveDot({ live }: { live: boolean }) {
  return (
    <span
      className="ml-auto flex shrink-0 items-center gap-1 font-code text-[10.5px] text-ink/45"
      title={live ? "Following the session live" : "Paused while this tab or surface is hidden"}
    >
      <span className={`h-1.5 w-1.5 rounded-full ${live ? "animate-pulse bg-state-executing" : "bg-ink/25"}`} />
      {live ? "live" : "paused"}
    </span>
  );
}

function Notice({ title, live, children }: { title: string; live?: boolean; children: ReactNode }) {
  return (
    <div className="rounded-md border border-dashed border-ink/15 px-3 py-3">
      <p className="flex items-center gap-1.5 text-[12.5px] font-medium text-ink/75">
        <Terminal size={13} className="shrink-0 text-ink/40" />
        <span className="min-w-0 flex-1">{title}</span>
        {live !== undefined && <LiveDot live={live} />}
      </p>
      <div className="mt-1 text-[12px] leading-relaxed text-ink/55">{children}</div>
    </div>
  );
}

function Code({ children }: { children: ReactNode }) {
  return <code className="rounded-sm bg-ink/[0.06] px-1 font-code text-[11px] text-ink/70">{children}</code>;
}

function ErrorLine({ message }: { message: string }) {
  return <p className="mt-1 font-code text-[11px] text-state-failed">Couldn't refresh: {message} — retrying.</p>;
}
