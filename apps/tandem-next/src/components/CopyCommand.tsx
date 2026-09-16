"use client";

import { useState, type ReactNode } from "react";

function CopyIcon({ className = "" }: { className?: string }) {
  return (
    <svg
      className={className}
      width={13}
      height={13}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <rect x="9" y="9" width="12" height="12" rx="2" />
      <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
    </svg>
  );
}

function CheckIcon({ className = "" }: { className?: string }) {
  return (
    <svg
      className={className}
      width={13}
      height={13}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2.4}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M20 6 9 17l-5-5" />
    </svg>
  );
}

function writeClipboard(text: string, done: () => void) {
  // navigator.clipboard exists only in secure contexts (https/localhost) — a
  // plain-http preview gets the execCommand fallback so the page's primary
  // conversion path never fails silently.
  if (typeof navigator !== "undefined" && navigator.clipboard) {
    navigator.clipboard.writeText(text).then(done, () => {});
    return;
  }
  const ta = document.createElement("textarea");
  ta.value = text;
  ta.style.position = "fixed";
  ta.style.opacity = "0";
  document.body.appendChild(ta);
  ta.select();
  try {
    if (document.execCommand("copy")) done();
  } finally {
    document.body.removeChild(ta);
  }
}

export function CopyButton({ text, label, tone = "dark" }: { text: string; label: string; tone?: "dark" | "light" }) {
  const [copied, setCopied] = useState(false);
  const idle =
    tone === "dark"
      ? "border-white/15 text-zinc-500 hover:border-white/30 hover:text-zinc-200"
      : "border-ink/15 text-ink/50 hover:border-ink/30 hover:text-ink";
  return (
    <button
      type="button"
      aria-label={`Copy ${label}`}
      onClick={() =>
        writeClipboard(text, () => {
          setCopied(true);
          window.setTimeout(() => setCopied(false), 1600);
        })
      }
      className={`inline-flex shrink-0 items-center gap-1.5 rounded border px-2 py-1 font-code text-[10px] transition-colors ${
        copied ? "border-emerald-400/40 text-emerald-500" : idle
      }`}
    >
      {copied ? <CheckIcon /> : <CopyIcon />}
      {copied ? "copied" : "copy"}
    </button>
  );
}

/** A dark terminal card: title bar (dots · mono label · copy) + body.
    Explicit dark ground so it reads the same in both themes. */
export function TerminalBlock({
  label,
  copyText,
  children,
}: {
  label: string;
  copyText: string;
  children: ReactNode;
}) {
  return (
    <div className="overflow-hidden rounded-lg border border-white/10 bg-[#101014] text-zinc-200 shadow-sm">
      <div className="flex items-center gap-2 border-b border-white/[0.06] px-4 py-2.5">
        <span className="h-2 w-2 rounded-full bg-white/15" />
        <span className="h-2 w-2 rounded-full bg-white/15" />
        <span className="truncate font-code text-[10px] text-zinc-500">{label}</span>
        <span className="ml-auto">
          <CopyButton text={copyText} label={label} />
        </span>
      </div>
      <div className="overflow-x-auto px-4 py-3.5 font-code text-[11.5px] leading-relaxed">{children}</div>
    </div>
  );
}

/** The install one-liner as a compact terminal line with a copy button — the
    hero's primary action. */
export function CommandLine({ cmd, label = "terminal" }: { cmd: string; label?: string }) {
  return (
    <div className="flex items-center gap-3 overflow-hidden rounded-md border border-white/10 bg-[#101014] py-2 pl-3.5 pr-2 text-zinc-200 shadow-sm">
      <span className="min-w-0 flex-1 truncate font-code text-[12.5px]">
        <span className="text-indigo-400">$</span> <span className="text-zinc-100">{cmd}</span>
      </span>
      <CopyButton text={cmd} label={label} />
    </div>
  );
}
