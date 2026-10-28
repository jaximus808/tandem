import { useEffect, useRef, useState } from "react";
import { X } from "lucide-react";
import { createPortal } from "react-dom";
import TandemLogo from "./TandemLogo";

interface Props {
  initialMode: "create" | "join";
  onJoin: (code: string) => void;
  onClose: () => void;
  onOpenMCP: () => void;
}

/* The hero used to carry both the create-canvas and join-by-code forms inline,
   which crowded it. They now live here, in one focused modal opened by the
   hero's CTA buttons. `initialMode` decides which field is autofocused. */
export default function CanvasLauncher({ initialMode, onJoin, onClose, onOpenMCP }: Props) {
  const [name, setName] = useState("");
  const [code, setCode] = useState("");
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState("");
  const nameRef = useRef<HTMLInputElement>(null);
  const codeRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [onClose]);

  useEffect(() => {
    const t = setTimeout(() => {
      (initialMode === "join" ? codeRef : nameRef).current?.focus();
    }, 60);
    return () => clearTimeout(t);
  }, [initialMode]);

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault();
    const trimmed = name.trim();
    if (!trimmed) {
      setError("Give your canvas a name.");
      nameRef.current?.focus();
      return;
    }
    setCreating(true);
    setError("");
    try {
      const res = await fetch("/api/canvases", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        // Send the session cookie so a logged-in create is owned by the account.
        credentials: "same-origin",
        body: JSON.stringify({ name: trimmed }),
      });
      if (!res.ok) {
        const detail = await res.text();
        throw new Error(res.status >= 500 ? "Server error — try again." : detail);
      }
      const canvas = (await res.json()) as { code: string };
      onJoin(canvas.code);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setCreating(false);
    }
  }

  function handleJoin(e: React.FormEvent) {
    e.preventDefault();
    const clean = code.trim().toUpperCase().replace(/[^A-Z0-9]/g, "");
    if (clean.length !== 8) {
      setError("Canvas codes are 8 characters.");
      return;
    }
    onJoin(clean);
  }

  return createPortal(
    <div
      className="fixed inset-0 z-[2000] flex items-center justify-center bg-black/40 p-4"
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="launcher-title"
        className="w-full max-w-md overflow-hidden rounded-md border border-ink/10 bg-surface shadow-lg"
      >
        {/* header */}
        <div className="flex items-center gap-2 border-b border-ink/10 bg-paper px-4 py-3">
          <TandemLogo size={24} animate={false} />
          <span id="launcher-title" className="text-base font-semibold tracking-tight text-ink">
            Open a canvas
          </span>
          <button
            onClick={onClose}
            aria-label="Close"
            className="ml-auto flex h-7 w-7 items-center justify-center rounded text-ink/50 transition-colors hover:bg-ink/[0.05] hover:text-ink focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/40"
          >
            <X size={15} />
          </button>
        </div>

        <div className="space-y-4 p-4">
          {/* Create */}
          <form onSubmit={handleCreate} className="space-y-1.5">
            <label className="block text-xs font-medium text-ink/60">
              Start something new
            </label>
            <input
              ref={nameRef}
              value={name}
              onChange={(e) => {
                setName(e.target.value);
                setError("");
              }}
              placeholder="Name a canvas — anything at all"
              className="h-8 w-full rounded border border-ink/15 bg-surface px-2.5 text-[13px] placeholder:text-ink/40 focus:border-accent/60 focus:outline-none focus:ring-2 focus:ring-accent/20"
            />
            <button
              type="submit"
              disabled={creating}
              className="inline-flex h-8 w-full items-center justify-center rounded bg-primary px-3.5 text-[13px] font-medium text-primary-fg transition-colors hover:bg-primary/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/40 focus-visible:ring-offset-2 focus-visible:ring-offset-surface disabled:opacity-50"
            >
              {creating ? "Creating…" : "Create canvas →"}
            </button>
          </form>

          <div className="flex items-center gap-3 text-[11px] font-medium text-ink/50">
            <span className="h-px flex-1 bg-ink/10" />
            or
            <span className="h-px flex-1 bg-ink/10" />
          </div>

          {/* Join */}
          <form onSubmit={handleJoin} className="space-y-1.5">
            <label className="block text-xs font-medium text-ink/60">
              Join with a code
            </label>
            <div className="flex gap-2">
              <input
                ref={codeRef}
                value={code}
                onChange={(e) => {
                  setCode(e.target.value.toUpperCase());
                  setError("");
                }}
                placeholder="TOKYO7X3K"
                maxLength={8}
                className="h-8 min-w-0 flex-1 rounded border border-ink/15 bg-surface px-2.5 font-code text-[13px] uppercase tracking-widest placeholder:text-ink/40 focus:border-accent/60 focus:outline-none focus:ring-2 focus:ring-accent/20"
              />
              <button
                type="submit"
                className="inline-flex h-8 shrink-0 items-center rounded border border-ink/15 bg-surface px-3.5 text-[13px] font-medium text-ink transition-colors hover:bg-ink/[0.04] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/40"
              >
                Open
              </button>
            </div>
          </form>

          {error && <p className="text-[13px] text-state-failed">{error}</p>}

          <button
            onClick={onOpenMCP}
            className="flex w-full items-center justify-center gap-1.5 border-t border-ink/10 pt-3 text-[12px] font-medium text-accent hover:underline underline-offset-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/40"
          >
            Connecting an AI agent instead? See the guide →
          </button>
        </div>
      </div>
    </div>,
    document.body,
  );
}
