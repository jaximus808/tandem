"use client";

import { useEffect } from "react";
import { createPortal } from "react-dom";
import { X } from "lucide-react";
import { FULL_DEMO_VIDEO_URL } from "@/lib/site";

// Click-to-play for the full walkthrough recording: portal to body, scrim
// click, Escape, close button. Never autoplays and never preloads.
export default function FullDemoModal({ onClose }: { onClose: () => void }) {
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [onClose]);

  return createPortal(
    <div
      className="fixed inset-0 z-[2000] flex items-center justify-center bg-black/60 p-4"
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="full-demo-title"
        className="w-full max-w-3xl overflow-hidden rounded-[10px] border border-ink/10 bg-surface shadow-lg lg:max-w-4xl"
      >
        <div className="flex items-center justify-between gap-3 border-b border-ink/10 px-4 py-2.5 dark:border-white/10">
          <h2 id="full-demo-title" className="truncate text-sm font-medium text-ink">
            Tandem — the full demo
          </h2>
          <button
            onClick={onClose}
            aria-label="Close"
            className="-mr-1 flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-ink/55 transition-colors hover:bg-ink/5 hover:text-ink/70 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/40"
          >
            <X size={15} />
          </button>
        </div>
        <div className="aspect-video w-full bg-black">
          <video src={FULL_DEMO_VIDEO_URL} controls preload="none" playsInline className="h-full w-full" />
        </div>
      </div>
    </div>,
    document.body,
  );
}
