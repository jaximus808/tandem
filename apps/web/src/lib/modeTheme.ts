import type { CanvasMode } from "../types";

/* ─────────────────────────────────────────────────────────────────────────────
   Per-mode CONTENT identity (Docs=violet, Roadmap=rose).

   SCOPE (Design v2 decision, /DESIGN.md): these hues are content-level
   semantics ONLY — e.g. the docTypes icon tints. They never colour chrome or
   agent activity: agent presence, the live cursor/halo, and the "editing …"
   chip all use the single terracotta `agent` token, and interactive chrome
   uses the one indigo `accent`. Don't wire new chrome to this map.
   ───────────────────────────────────────────────────────────────────────────── */

export interface ModeTheme {
  /** The full-strength accent — solid fills, active dots, primary buttons. */
  solid: string;
  /** A faint wash for active-tab backgrounds and soft chips. */
  soft: string;
  /** A hairline tint for borders that should read as "this mode". */
  line: string;
  /** A hover-strength solid, slightly darker than `solid`. */
  hover: string;
}

export const MODE_THEME: Record<CanvasMode, ModeTheme> = {
  welcome: {
    solid: "#64748B",
    soft: "rgba(100,116,139,0.10)",
    line: "rgba(100,116,139,0.22)",
    hover: "#475569",
  },
  docs: {
    solid: "#7C3AED",
    soft: "rgba(124,58,237,0.10)",
    line: "rgba(124,58,237,0.22)",
    hover: "#6D28D9",
  },
  roadmap: {
    solid: "#F43F5E",
    soft: "rgba(244,63,94,0.10)",
    line: "rgba(244,63,94,0.24)",
    hover: "#E11D48",
  },
};

export function modeTheme(mode: CanvasMode | undefined): ModeTheme {
  return MODE_THEME[mode ?? "welcome"] ?? MODE_THEME.welcome;
}
