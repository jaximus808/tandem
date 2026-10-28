"use client";

/* SchedulerHero — the Tandem scheduler, alive, as separate terminal windows.

   Left: the orchestrator session (planner). It takes the prompt, proposes the
   epic, parks on queue_wait until the human approves on the board, then
   dispatches one subagent per ready task with `Task(sonnet-14) TDM-x` (or `spawn(codex-22)`) and only
   reads board_status after that. Right: worker windows that open as they are
   dispatched (a pill carrying the Tandem mark and the ticket rides a line from
   the Task line to the new window), self-serve from the shared queue with
   atomic claims, and close when queue_next answers `0 ready`.

   Engine: `script.ts` generates a cycle deterministically and `reduce` applies
   it. One rAF driver advances a clock only while the panel is on screen and
   the tab is visible; state changes happen per event, never per frame. Layout
   is read with refs only when a worker is dispatched (for the flight path) and
   when the tiling changes (FLIP), never per frame. */

import { useEffect, useLayoutEffect, useRef, useState } from "react";
import styles from "./SchedulerHero.module.css";
import {
  BOOT_CYCLE,
  BOOT_STATE,
  BOOT_T,
  buildScript,
  cycleEnd,
  DISPATCH,
  modelLabel,
  reduce,
  type BoardState,
  type Line,
  type Tone,
  type WorkerPane,
} from "./script";

export interface SchedulerHeroProps {
  /** Extra class on the outer wrapper (the parent controls max-width). */
  className?: string;
  /** Start the loop paused (the static boot frame stays up). */
  paused?: boolean;
}

const useIsoLayoutEffect = typeof window === "undefined" ? useEffect : useLayoutEffect;

const BOOT_SEQ = BOOT_STATE.seq;
const BOOT_WORKERS = new Set(BOOT_STATE.workers.map((w) => w.name));
const MAX_PANES = 4;
const WORKER_LINES = 4;
const FLIGHT_MS = 1_400;

interface Flight {
  id: number;
  name: string;
  tone: Tone;
  ticket: string;
  d: string;
  len: number;
}

export default function SchedulerHero({ className, paused = false }: SchedulerHeroProps) {
  const [state, setState] = useState<BoardState>(BOOT_STATE);
  const stateRef = useRef<BoardState>(BOOT_STATE);
  const rootRef = useRef<HTMLDivElement>(null);
  const stageRef = useRef<HTMLDivElement>(null);
  const orchRef = useRef<HTMLElement>(null);
  const tilesRef = useRef<HTMLDivElement>(null);
  const reducedRef = useRef(false);

  /* ---- driver ------------------------------------------------------ */
  useEffect(() => {
    if (paused) return;
    const mq = window.matchMedia("(prefers-reduced-motion: reduce)");
    reducedRef.current = mq.matches;
    if (mq.matches) return;

    let cycle = BOOT_CYCLE;
    let script = buildScript(cycle);
    let end = cycleEnd(script);
    let idx = script.findIndex((e) => e.t > BOOT_T);
    if (idx < 0) idx = script.length;
    let elapsed = BOOT_T;
    let last: number | null = null;
    let raf = 0;
    let onScreen = true;
    let tabVisible = !document.hidden;
    let running = false;

    const tick = (now: number) => {
      if (last == null) last = now;
      elapsed += Math.min(now - last, 120); // never "catch up" after a stall
      last = now;

      let s = stateRef.current;
      let changed = false;
      while (idx < script.length && script[idx].t <= elapsed) {
        s = reduce(s, script[idx]);
        idx++;
        changed = true;
      }
      if (elapsed >= end) {
        elapsed -= end;
        cycle++;
        script = buildScript(cycle);
        end = cycleEnd(script);
        idx = 0;
      }
      if (changed) {
        stateRef.current = s;
        setState(s);
      }
      raf = requestAnimationFrame(tick);
    };

    const sync = () => {
      const should = onScreen && tabVisible;
      if (should && !running) {
        running = true;
        last = null;
        raf = requestAnimationFrame(tick);
      } else if (!should && running) {
        running = false;
        cancelAnimationFrame(raf);
      }
    };

    const io = new IntersectionObserver(
      ([entry]) => {
        onScreen = entry.isIntersecting;
        sync();
      },
      { threshold: 0.05 },
    );
    if (rootRef.current) io.observe(rootRef.current);
    const onVis = () => {
      tabVisible = !document.hidden;
      sync();
    };
    document.addEventListener("visibilitychange", onVis);
    sync();

    return () => {
      io.disconnect();
      document.removeEventListener("visibilitychange", onVis);
      cancelAnimationFrame(raf);
    };
  }, [paused]);

  /* ---- FLIP: worker windows glide when the tiling changes ----------- */
  const tilePos = useRef<Map<string, [number, number]>>(new Map());
  const visible = state.workers.slice(0, MAX_PANES);
  const hidden = state.workers.length - visible.length;
  const tileKey = visible.map((w) => w.name).join("|");
  useIsoLayoutEffect(() => {
    const grid = tilesRef.current;
    if (!grid) return;
    const base = grid.getBoundingClientRect();
    const next = new Map<string, [number, number]>();
    const moves: Array<[HTMLElement, number, number]> = [];
    grid.querySelectorAll<HTMLElement>("[data-pane]").forEach((el) => {
      const key = el.dataset.pane as string;
      const r = el.getBoundingClientRect();
      const pos: [number, number] = [r.left - base.left, r.top - base.top];
      next.set(key, pos);
      const prev = tilePos.current.get(key);
      if (prev && !reducedRef.current) {
        const dx = prev[0] - pos[0];
        const dy = prev[1] - pos[1];
        if (Math.abs(dx) > 0.5 || Math.abs(dy) > 0.5) moves.push([el, dx, dy]);
      }
    });
    tilePos.current = next;
    if (!moves.length) return;
    for (const [el, dx, dy] of moves) {
      el.style.transition = "none";
      el.style.transform = `translate(${dx}px, ${dy}px)`;
    }
    void grid.offsetHeight; // commit the inverted position
    for (const [el] of moves) {
      el.style.transition = "";
      el.style.transform = "";
    }
  }, [tileKey]);

  /* ---- dispatch flights: Task line → new window ---------------------- */
  const [flights, setFlights] = useState<Flight[]>([]);
  const seen = useRef<Set<string>>(new Set(BOOT_WORKERS));
  const flightSeq = useRef(0);
  useIsoLayoutEffect(() => {
    const stage = stageRef.current;
    const orch = orchRef.current;
    if (!stage || !orch) return;
    const fresh = visible.filter((w) => !seen.current.has(w.name));
    for (const w of state.workers) seen.current.add(w.name);
    if (!fresh.length || reducedRef.current || stage.clientWidth < 700) return;

    const base = stage.getBoundingClientRect();
    const orchR = orch.getBoundingClientRect();
    const added: Flight[] = [];
    for (const w of fresh) {
      const line = orch.querySelector<HTMLElement>(`[data-dispatch="${w.name}"]`);
      const pane = stage.querySelector<HTMLElement>(`[data-pane="${w.name}"]`);
      if (!line || !pane) continue;
      const lr = line.getBoundingClientRect();
      const pr = pane.getBoundingClientRect();
      if (lr.bottom > orchR.bottom || lr.top < orchR.top + 40) continue; // line not visible
      const ticket = (state.orch.find((l) => l.kind === "call" && DISPATCH.exec(l.tool)?.[1] === w.name) as Extract<Line, { kind: "call" }> | undefined)?.arg ?? "";
      const x1 = orchR.right - base.left;
      const y1 = lr.top + lr.height / 2 - base.top;
      const x2 = pr.left - base.left;
      const y2 = pr.top + 16 - base.top;
      const gap = x2 - x1;
      let d: string;
      if (gap > 6 && gap < 80) {
        /* first column: straight across the gap, up or down inside it */
        const gx = x1 + gap / 2;
        d = `M ${x1} ${y1} H ${gx} V ${y2} H ${x2 - 2}`;
      } else {
        /* second column: along the orchestrator gap, then the row corridor */
        const tiles = tilesRef.current?.getBoundingClientRect();
        const g = tiles ? pr.left - (tiles.left + (pr.left - tiles.left) / 2) : 8; // ≈ half the column gap
        const corridorY = (pr.top - base.top) - Math.max(6, Math.min(g, 10));
        const gx = (tiles ? tiles.left - base.left : x1) - 7;
        d = `M ${x1} ${y1} H ${gx} V ${corridorY} H ${x2 - 7} V ${y2} H ${x2 - 2}`;
      }
      const len = pathLength(d);
      added.push({ id: ++flightSeq.current, name: w.name, tone: w.tone, ticket, d, len });
    }
    if (!added.length) return;
    setFlights((f) => [...f, ...added].slice(-2));
    const timer = window.setTimeout(() => {
      setFlights((f) => f.filter((x) => !added.some((a) => a.id === x.id)));
    }, FLIGHT_MS);
    return () => window.clearTimeout(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tileKey]);

  const done = state.tickets.filter((t) => t.state === "done").length;
  const live = state.workers.filter((w) => w.active).length;

  return (
    <div
      ref={rootRef}
      className={[styles.root, className].filter(Boolean).join(" ")}
      role="img"
      aria-label="Animated example of a Tandem session, alternating between a Claude orchestrator with Claude subagents (Opus, Sonnet, Haiku) and a Codex orchestrator with Codex subagents. Left: the orchestrator proposes an epic of tickets, waits until a human approves it on the board, then dispatches one subagent per task and never claims anything itself. Right: worker agents appear as they are dispatched, call queue_next and task_claim on a shared queue, report progress, complete with a commit hash, and leave when the queue is empty. When two workers claim the same ticket at the same moment, one is told claimed: false and who holds it, and takes the next ticket. Ticket names and hashes are illustrative."
    >
      <div className={styles.strip} aria-hidden="true">
        <span className={styles.stripLeft}>
          <Mark className={styles.stripMark} />
          <span className={styles.stripName}>tandem</span>
          {state.epic ? (
            <>
              <span className={styles.stripSep}>/</span>
              <span className={styles.stripEpic}>{state.epic}</span>
            </>
          ) : null}
        </span>
        <span className={styles.stripRight}>
          <span className={live ? styles.liveOn : styles.liveOff} />
          <span className={styles.stat}>
            {live} agent{live === 1 ? "" : "s"}
          </span>
          {hidden > 0 ? <span className={styles.chip}>+{hidden}</span> : null}
          <span className={`${styles.stripSep} ${styles.hideSm}`}>·</span>
          <span className={`${styles.stat} ${styles.hideSm}`}>{state.tickets.length ? `${done} of ${state.tickets.length} done` : "idle"}</span>
          <span className={styles.stripSep}>·</span>
          <span className={state.refused ? styles.statRefused : styles.stat}>{state.refused} refused</span>
        </span>
      </div>

      <div className={styles.stage} ref={stageRef} aria-hidden="true">
        <section className={`${styles.win} ${styles.orch}`} ref={orchRef}>
          <div className={styles.winHead}>
            <span className={styles.winName}>
              <span className={styles.orchDot} />
              orchestrator · {state.agent}
            </span>
            <span className={styles.winCur}>planner</span>
          </div>
          <div className={styles.log}>
            {state.orch.map((l) => (
              <TranscriptLine key={l.id} l={l} fresh={l.id > BOOT_SEQ} />
            ))}
          </div>
        </section>

        <div className={styles.tiles} ref={tilesRef} data-n={visible.length}>
          {visible.map((w) => (
            <WorkerWindow key={w.name} w={w} fresh={!BOOT_WORKERS.has(w.name)} />
          ))}
          {visible.length === 0 ? <div className={styles.empty}>no agents running · waiting on the next approval</div> : null}
        </div>

        {flights.length ? (
          <svg className={styles.wires}>
            {flights.map((f) => (
              <path
                key={f.id}
                className={`${styles.wire} ${styles[`tone_${f.tone}`]}`}
                d={f.d}
                style={{ strokeDasharray: f.len, strokeDashoffset: f.len }}
              />
            ))}
          </svg>
        ) : null}
        {flights.map((f) => (
          <div key={f.id} className={`${styles.pill} ${styles[`tone_${f.tone}`]}`} style={{ offsetPath: `path("${f.d}")` }}>
            <Mark className={styles.pillMark} />
            {f.ticket}
          </div>
        ))}
      </div>
    </div>
  );
}

/** The Tandem mark (assets/svg/tandem-mark-white.svg), inlined in currentColor. */
function Mark({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 120 120" aria-hidden="true">
      <g transform="translate(60,60) scale(0.84) translate(-60,-60)" fill="none" stroke="currentColor" strokeLinecap="round">
        <path d="M85.3 37.3 A34 34 0 0 1 52.9 93.3" strokeWidth="9" />
        <path d="M34.7 82.7 A34 34 0 0 1 67.1 26.7" strokeWidth="9" />
        <line x1="43" y1="89" x2="77" y2="31" strokeWidth="5" />
        <circle cx="77" cy="31" r="11" fill="currentColor" stroke="none" />
        <circle cx="43" cy="89" r="11" strokeWidth="6" />
      </g>
    </svg>
  );
}

/** Length of an axis-aligned M/H/V path (the only shapes the flights use). */
function pathLength(d: string): number {
  const tok = d.trim().split(/\s+/);
  let x = 0;
  let y = 0;
  let len = 0;
  for (let i = 0; i < tok.length; i++) {
    const c = tok[i];
    if (c === "M") {
      x = +tok[++i];
      y = +tok[++i];
    } else if (c === "H") {
      const nx = +tok[++i];
      len += Math.abs(nx - x);
      x = nx;
    } else if (c === "V") {
      const ny = +tok[++i];
      len += Math.abs(ny - y);
      y = ny;
    }
  }
  return len;
}

function WorkerWindow({ w, fresh }: { w: WorkerPane; fresh: boolean }) {
  return (
    <section
      className={[styles.win, styles.tile, styles[`tone_${w.tone}`], fresh ? styles.tileIn : "", w.active ? "" : styles.tileOut].join(" ")}
      data-pane={w.name}
    >
      <div className={styles.winHead}>
        <span className={styles.winName}>
          <span className={styles.dot} />
          {modelLabel(w.name)}
          <span className={styles.winId}>#{w.name.slice(w.name.lastIndexOf("-") + 1)}</span>
        </span>
        <span className={styles.winCur}>{w.active ? (w.current ?? "queue") : "leaving"}</span>
      </div>
      <div className={styles.lines}>
        {w.lines.slice(-WORKER_LINES).map((l) => (
          <TranscriptLine key={l.id} l={l} fresh={fresh || l.id > BOOT_SEQ} />
        ))}
      </div>
    </section>
  );
}

/* agent names and ticket refs never break at their hyphen */
const REF = /((?:opus|sonnet|haiku|codex)-\d+|TDM-\d+)/g;
function Text({ s }: { s: string }) {
  const parts = s.split(REF);
  if (parts.length === 1) return <>{s}</>;
  return (
    <>
      {parts.map((p, i) =>
        i % 2 ? (
          <span key={i} className={styles.ref}>
            {p}
          </span>
        ) : (
          p
        ),
      )}
    </>
  );
}

function TranscriptLine({ l, fresh }: { l: Line; fresh: boolean }) {
  const f = fresh ? styles.fresh : "";
  switch (l.kind) {
    case "user":
      return (
        <div className={`${styles.line} ${styles.user} ${f}`}>
          <span className={styles.prompt}>&gt;</span>
          <span className={styles.text}>{l.text}</span>
        </div>
      );
    case "call": {
      const dispatch = DISPATCH.exec(l.tool)?.[1] ?? null;
      return (
        <div
          className={[styles.line, styles.call, l.tone ? styles[`tone_${l.tone}`] : "", dispatch ? styles.dispatch : "", f].join(" ")}
          data-dispatch={dispatch ?? undefined}
        >
          <span className={styles.bullet}>●</span>
          <span className={styles.text}>
            <span className={styles.tool}>{l.tool}</span>
            {l.arg ? (
              <span className={styles.arg}>
                {" "}
                <Text s={l.arg} />
              </span>
            ) : null}
          </span>
        </div>
      );
    }
    case "result":
      return (
        <div className={[styles.line, styles.result, styles[`res_${l.tone}`], l.race ? styles.raceLose : "", f].join(" ")}>
          <span className={styles.elbow}>⎿</span>
          <span className={styles.text}>
            <Text s={l.text} />
          </span>
        </div>
      );
    case "say":
      return (
        <div className={`${styles.line} ${styles.say} ${f}`}>
          <span className={styles.sayDot}>●</span>
          <span className={styles.text}>
            <Text s={l.text} />
          </span>
        </div>
      );
    case "human":
      return (
        <div className={`${styles.line} ${styles.human} ${f}`}>
          <span className={styles.check}>✓</span>
          <span className={styles.text}>{l.text}</span>
        </div>
      );
  }
}
