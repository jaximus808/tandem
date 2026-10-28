// Shared task/epic state visuals — Design System v3 ("Tool").
//
// The closed six-state set (see /DESIGN.md §Tokens), drawn from the
// `state-*` tokens, which flip between light and dark on their own, so no
// `dark:` variants are needed here:
//   proposed ochre · ready slate · working teal · done green · failed brick ·
//   rejected zinc.
// This is the ONE definition; TaskBoard (and any future task surface) imports
// it rather than carrying its own hex tables. No new hues.
//
// A state chip is a DOT + WORD, not a filled pill (DESIGN.md §Components,
// "Chips"): a 6px state-token dot (drawn by a ::before, so callers keep their
// single <span>) followed by the label in ink/70. Callers that want a tinted
// ground for a one-off chip pair CHIP_BASE with `ground`.

export interface StateChipDef {
  label: string;
  /** Dot + word: a ::before dot in the state token, label in ink/70. Pair with CHIP_BASE. */
  chip: string;
  /** Tinted-ground variant (token /10 ground + full-strength token text), for one-off emphasis. */
  ground: string;
  /** Solid dot / progress-segment fill in the state token. */
  dot: string;
  /** Text-only treatment (claimant chips, inline annotations). */
  text: string;
}

/** Base chip shell shared by every state chip: 11px, 3px radius, no ground of its own. */
export const CHIP_BASE =
  "inline-flex shrink-0 items-center gap-1.5 rounded-sm px-1.5 py-px text-[11px] font-medium leading-4";

/** The ::before dot every `chip` draws; the per-state class adds its colour. */
const DOT_BEFORE =
  "text-ink/70 before:h-1.5 before:w-1.5 before:shrink-0 before:rounded-full before:content-['']";

// Keyed by ActionState, but typed Record<string, …> so callers can keep the
// defensive `STATE_CHIP[state] ?? STATE_CHIP.proposed` lookup on raw strings.
export const STATE_CHIP: Record<string, StateChipDef> = {
  proposed: {
    label: "Proposed",
    chip: `${DOT_BEFORE} before:bg-state-proposed`,
    ground: "bg-state-proposed/10 text-state-proposed",
    dot: "bg-state-proposed",
    text: "text-state-proposed",
  },
  approved: {
    label: "Ready",
    chip: `${DOT_BEFORE} before:bg-state-approved`,
    ground: "bg-state-approved/10 text-state-approved",
    dot: "bg-state-approved",
    text: "text-state-approved",
  },
  executing: {
    label: "Working",
    chip: `${DOT_BEFORE} before:bg-state-executing`,
    ground: "bg-state-executing/10 text-state-executing",
    dot: "bg-state-executing",
    text: "text-state-executing",
  },
  done: {
    label: "Done",
    chip: `${DOT_BEFORE} before:bg-state-done`,
    ground: "bg-state-done/10 text-state-done",
    dot: "bg-state-done",
    text: "text-state-done",
  },
  failed: {
    label: "Failed",
    chip: `${DOT_BEFORE} before:bg-state-failed`,
    ground: "bg-state-failed/10 text-state-failed",
    dot: "bg-state-failed",
    text: "text-state-failed",
  },
  rejected: {
    label: "Rejected",
    chip: `${DOT_BEFORE} before:bg-state-rejected`,
    ground: "bg-state-rejected/10 text-state-rejected",
    dot: "bg-state-rejected",
    text: "text-state-rejected",
  },
};
