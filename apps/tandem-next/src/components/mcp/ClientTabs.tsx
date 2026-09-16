"use client";

import { useState, type ReactNode } from "react";
import { CopyButton } from "@/components/CopyCommand";

export interface Tab {
  id: string;
  label: string;
  badge?: string;
  blurb: ReactNode;
  /** Code blocks, each with a short lead line. */
  blocks: { text?: ReactNode; code: string }[];
}

/** A dark code slab with a copy button — the page's one code idiom. */
export function CodeBlock({ code }: { code: string }) {
  return (
    <div className="overflow-hidden rounded-lg border border-white/10 bg-[#101014] text-zinc-200 shadow-sm">
      <div className="flex items-center justify-end border-b border-white/[0.06] px-3 py-1.5">
        <CopyButton text={code} label="snippet" />
      </div>
      <pre className="overflow-x-auto whitespace-pre px-4 py-3.5 font-code text-[11.5px] leading-[1.7] text-zinc-300">{code}</pre>
    </div>
  );
}

/** Pill tabs over a card — used for install methods and client configs. */
export default function ClientTabs({ tabs }: { tabs: Tab[] }) {
  const [active, setActive] = useState(tabs[0].id);
  const tab = tabs.find((t) => t.id === active) ?? tabs[0];
  return (
    <div>
      <div className="flex flex-wrap gap-1.5" role="tablist">
        {tabs.map((t) => {
          const on = t.id === tab.id;
          return (
            <button
              key={t.id}
              role="tab"
              aria-selected={on}
              onClick={() => setActive(t.id)}
              className={[
                "inline-flex items-center gap-1.5 whitespace-nowrap rounded-md border px-3 py-1.5 text-[13px] font-medium transition-colors",
                on
                  ? "border-accent/30 bg-accent/10 text-accent"
                  : "border-ink/15 bg-surface text-ink/60 hover:border-ink/25 hover:text-ink",
              ].join(" ")}
            >
              {t.label}
              {t.badge && (
                <span
                  className={`rounded px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide ${
                    on ? "bg-accent/15 text-accent" : "bg-ink/10 text-ink/50"
                  }`}
                >
                  {t.badge}
                </span>
              )}
            </button>
          );
        })}
      </div>

      <div className="mt-3 space-y-4 rounded-lg border border-ink/10 bg-surface p-4">
        <p className="text-sm leading-relaxed text-ink/60">{tab.blurb}</p>
        {tab.blocks.map((b, i) => (
          <div key={i} className="space-y-2">
            {b.text && <p className="text-sm text-ink/70">{b.text}</p>}
            <CodeBlock code={b.code} />
          </div>
        ))}
      </div>
    </div>
  );
}
