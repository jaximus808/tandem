import type { CanvasMode } from "../types";
import type { AgentEdit } from "../lib/useAgentActivity";
import type { FleetAgent } from "../lib/api";
import { vendorOf } from "./FleetView";

interface Props {
  /** The ACTIVE fleet — the same rows the Agents surface lists (isActiveFleetAgent),
   *  so the header count and the surface can never disagree. */
  agents: FleetAgent[];
  /** How many of them hold a claim right now. */
  working: number;
  edit: AgentEdit | null;
  reading: boolean;
  onJump: (mode: CanvasMode) => void;
  /** Open the Agents surface (components/FleetView.tsx). */
  onOpenAgents: () => void;
  /** Whether the Agents surface is the one showing — lights the chip. */
  current: boolean;
}

const MODE_LABEL: Record<CanvasMode, string> = {
  welcome: "Templates",
  docs: "Docs",
  roadmap: "Roadmap",
};

// Avatars drawn before the stack folds the rest into "+n".
const MAX_AVATARS = 4;

/**
 * Header presence (TDM-17): a compact avatar stack + count that opens the
 * Agents surface. It answers "is anyone here, and is anything running" at a
 * glance; WHO is doing WHAT lives on the Agents surface, one row per agent with
 * every ticket it holds. The old single-ticket chip is gone — it could only
 * ever show one claimant, which on a fleet was the wrong one more often than not.
 *
 * When an agent is mid-edit on a document, a live "editing {Mode}" chip still
 * rides beside the stack and jumps to where it's writing.
 */
export default function AgentPresence({ agents, working, edit, reading, onJump, onOpenAgents, current }: Props) {
  if (agents.length === 0) return null;
  const showReading = !edit && reading;
  const shown = agents.slice(0, MAX_AVATARS);
  const extra = agents.length - shown.length;
  const total = agents.length;
  const label = `${total} agent${total === 1 ? "" : "s"}${working > 0 ? ` · ${working} working` : ""}`;

  // ZONE 2 · AMBIENT of the canvas header's collapse contract — the block
  // comment above `HEADER_DROP` in App.tsx is the whole rule; this is the one
  // element in the header's right-hand zone that is allowed to give up space.
  // `min-w-0 overflow-hidden` is what makes that safe: the block collapses and
  // CLIPS inside its own box instead of spilling out over a neighbour (the
  // failure TDM-152 fixed for the identity group). It holds no popover of its
  // own — the Agents surface it opens is a page, not a panel — so nothing that
  // must escape the box lives in here; don't add one.
  // Because it clips from the RIGHT, the DOM order below IS the collapse order:
  // the live edit/read chip goes first, and the stack + count — the "agents are
  // here" signal — survives longest. Keep that order.
  // `px-1 -mx-1` re-opens the 4px focus ring so containment doesn't shave it.
  // `hidden ... sm:flex` is ladder rank 5; below sm the Agents surface is in
  // the mobile nav drawer.
  return (
    <div className="relative hidden min-w-0 items-center gap-2 overflow-hidden px-1 -mx-1 sm:flex">
      <button
        onClick={onOpenAgents}
        aria-current={current ? "page" : undefined}
        aria-label={`Open Agents — ${label}`}
        title={agents
          .map((a) => `${a.name}${a.tasks.length > 0 ? ` — ${a.tasks.map((t) => t.ticketId ?? "task").join(", ")}` : ""}`)
          .join("\n")}
        className={[
          "inline-flex h-8 shrink-0 items-center gap-2 rounded px-1.5 text-[12.5px] font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/40",
          current ? "bg-ink/[0.06] text-ink" : "text-ink/60 hover:bg-ink/[0.05] hover:text-ink",
        ].join(" ")}
      >
        <span className="flex shrink-0 -space-x-1" aria-hidden="true">
          {shown.map((a) => {
            const isClaude = vendorOf(a).kind === "claude";
            const busy = a.tasks.length > 0;
            return (
              <span
                key={a.id ?? a.name}
                className={`relative grid h-5 w-5 place-items-center overflow-hidden rounded-[4px] ring-2 ring-paper ${
                  isClaude ? "bg-agent" : "bg-zinc-700"
                }`}
              >
                <Sparkle />
                {showReading && <span className="tandem-scan absolute inset-0" />}
                {/* Working agents carry a state-executing tick in the corner. */}
                {busy && (
                  <span className="absolute bottom-0 right-0 h-1.5 w-1.5 rounded-tl-[2px] bg-state-executing" />
                )}
              </span>
            );
          })}
          {extra > 0 && (
            <span className="relative grid h-5 min-w-5 place-items-center rounded-[4px] bg-ink/10 px-1 font-code text-[10px] font-medium text-ink/70 ring-2 ring-paper">
              +{extra}
            </span>
          )}
        </span>
        <span className="shrink-0 tabular-nums">{label}</span>
      </button>

      {/* Live edit/read chip. Priority: editing (where it's writing) → reading
          (it's looking at the canvas). Agent activity is terracotta (the
          `agent` token); the chip names the mode in words. */}
      {edit ? (
        <button
          onClick={() => onJump(edit.mode)}
          className="inline-flex shrink-0 items-center gap-1.5 rounded-sm bg-agent/10 px-1.5 py-px text-[11px] font-medium text-agent transition-opacity hover:opacity-80 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/40"
          title={`Jump to ${MODE_LABEL[edit.mode]} — where it's writing`}
        >
          <span className="relative flex h-1.5 w-1.5">
            <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-agent opacity-70" />
            <span className="relative inline-flex h-1.5 w-1.5 rounded-full bg-agent" />
          </span>
          editing {MODE_LABEL[edit.mode].toLowerCase()}
        </button>
      ) : showReading ? (
        <span
          className="inline-flex shrink-0 items-center gap-1.5 rounded-sm bg-agent/10 px-1.5 py-px text-[11px] font-medium text-agent"
          title="An agent is reading the canvas"
        >
          <span className="flex items-end gap-[2px]" aria-hidden="true">
            <span className="tandem-read-bar h-2 w-[2px] rounded-full bg-agent" style={{ animationDelay: "0ms" }} />
            <span className="tandem-read-bar h-2 w-[2px] rounded-full bg-agent" style={{ animationDelay: "140ms" }} />
            <span className="tandem-read-bar h-2 w-[2px] rounded-full bg-agent" style={{ animationDelay: "280ms" }} />
          </span>
          reading
        </span>
      ) : null}
    </div>
  );
}

function Sparkle() {
  return (
    <svg width="10" height="10" viewBox="0 0 12 12" fill="none" aria-hidden="true">
      <path
        d="M6 0.5 C6.4 3 7.2 3.8 9.5 4.2 C7.2 4.6 6.4 5.4 6 7.8 C5.6 5.4 4.8 4.6 2.5 4.2 C4.8 3.8 5.6 3 6 0.5 Z"
        fill="white"
      />
      <circle cx="10" cy="9" r="1" fill="white" opacity="0.85" />
    </svg>
  );
}
