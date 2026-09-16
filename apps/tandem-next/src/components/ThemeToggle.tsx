"use client";

import { useEffect, useState } from "react";
import { Moon, Sun } from "lucide-react";

const KEY = "tandem.theme";

/** Reads the current class the boot script (app/layout.tsx) set on <html>. */
function isDarkNow(): boolean {
  return typeof document !== "undefined" && document.documentElement.classList.contains("dark");
}

// Light / dark toggle, same storage key as the app so a preference set here
// carries into the board and back. Renders nothing until mounted so the server
// HTML never disagrees with the boot script's decision.
export default function ThemeToggle() {
  const [dark, setDark] = useState<boolean | null>(null);
  useEffect(() => setDark(isDarkNow()), []);

  if (dark === null) return <span className="h-8 w-8" aria-hidden="true" />;

  return (
    <button
      type="button"
      onClick={() => {
        const next = !dark;
        document.documentElement.classList.toggle("dark", next);
        try {
          localStorage.setItem(KEY, next ? "dark" : "light");
        } catch {
          /* private mode — the class still applied for this page */
        }
        setDark(next);
      }}
      aria-label={dark ? "Switch to light theme" : "Switch to dark theme"}
      title={dark ? "Light theme" : "Dark theme"}
      className="grid h-8 w-8 place-items-center rounded-md text-ink/55 transition-colors hover:bg-ink/5 hover:text-ink focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/40"
    >
      {dark ? <Sun size={15} /> : <Moon size={15} />}
    </button>
  );
}
