import type { ReactNode } from "react";

/** Canon section eyebrow — 12px Inter 500 uppercase tracking-wide ink/50. */
export function Eyebrow({ children, className = "" }: { children: ReactNode; className?: string }) {
  return (
    <span className={`text-xs font-medium uppercase tracking-wide text-ink/50 ${className}`}>
      {children}
    </span>
  );
}

/** Monospace chip for a real MCP tool name (machine text — mono is earned). */
export function ToolChip({ name }: { name: string }) {
  return (
    <span className="rounded border border-ink/10 bg-surface px-1.5 py-0.5 font-code text-[10.5px] font-medium text-ink/70">
      {name}
    </span>
  );
}

/** Machine text inside prose — tool names, endpoints, states, env vars. */
export function M({ children }: { children: ReactNode }) {
  return (
    <code className="whitespace-nowrap rounded border border-ink/10 bg-ink/[0.05] px-1.5 py-px font-code text-[0.82em] text-ink/85">
      {children}
    </code>
  );
}

/** Bolded lead-in inside a line. */
export function B({ children }: { children: ReactNode }) {
  return <strong className="font-medium text-ink">{children}</strong>;
}

/** The three freshness states a briefing can report, in the semantic palette. */
export function FreshnessChips() {
  const states: { label: string; cls: string }[] = [
    { label: "fresh", cls: "bg-emerald-500/10 text-emerald-600 dark:text-emerald-400" },
    { label: "aging", cls: "bg-amber-500/10 text-amber-600 dark:text-amber-400" },
    { label: "stale", cls: "bg-rose-500/10 text-rose-600 dark:text-rose-400" },
  ];
  return (
    <span className="inline-flex gap-1 align-middle">
      {states.map((s) => (
        <span
          key={s.label}
          className={`rounded px-1.5 py-px text-[10px] font-semibold uppercase tracking-wide ${s.cls}`}
        >
          {s.label}
        </span>
      ))}
    </span>
  );
}

export function ArrowIcon({ className = "" }: { className?: string }) {
  return (
    <svg
      className={className}
      width={16}
      height={16}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.8}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M5 12h14M13 6l6 6-6 6" />
    </svg>
  );
}

export function CommitIcon({ className = "" }: { className?: string }) {
  return (
    <svg
      className={className}
      width={12}
      height={12}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      aria-hidden="true"
    >
      <circle cx="12" cy="12" r="3.2" />
      <path d="M1.5 12h7.3M15.2 12h7.3" />
    </svg>
  );
}

/** Primary / secondary button classes (canon: 36px landing height). */
export const BTN_PRIMARY =
  "inline-flex h-9 items-center justify-center gap-2 rounded-md bg-accent px-5 text-[13px] font-medium text-white transition-[filter] hover:brightness-95 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/40 focus-visible:ring-offset-2 focus-visible:ring-offset-paper";
export const BTN_SECONDARY =
  "inline-flex h-9 items-center justify-center gap-2 rounded-md border border-ink/15 bg-surface px-5 text-[13px] font-medium text-ink transition-colors hover:border-ink/25 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/40";
