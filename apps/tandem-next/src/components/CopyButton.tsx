"use client";

import { useEffect, useRef, useState } from "react";

function writeClipboard(text: string, done: () => void) {
  // navigator.clipboard exists only in secure contexts (https/localhost); a
  // plain-http preview gets the execCommand fallback so copy never fails silently.
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

/** The quiet button next to the install line. Says "copy", then "copied". */
export default function CopyButton({ text, label = "Copy the setup command" }: { text: string; label?: string }) {
  const [copied, setCopied] = useState(false);
  const timer = useRef(0);
  useEffect(() => () => window.clearTimeout(timer.current), []);
  return (
    <button
      type="button"
      className={copied ? "btn is-done" : "btn"}
      aria-label={label}
      onClick={() =>
        writeClipboard(text, () => {
          setCopied(true);
          window.clearTimeout(timer.current);
          timer.current = window.setTimeout(() => setCopied(false), 1600);
        })
      }
    >
      {copied ? "copied" : "copy"}
    </button>
  );
}
