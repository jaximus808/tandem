import type { CanvasMode, DocumentType } from "../types";

/* Documents (migration 0024) are the canvas's tabs. A document's `type` maps to
   the existing per-type "mode" that renders it (and carries its accent colour via
   modeTheme). This is the one place that mapping lives. */

export const DOC_TYPE_TO_MODE: Record<DocumentType, CanvasMode> = {
  notes: "docs",
  roadmap: "roadmap",
  // A folder holds no content and never renders as a tab; it only nests other
  // documents in the explorer tree. This entry keeps the Record total.
  folder: "welcome",
};

export const DOC_TYPE_LABEL: Record<DocumentType, string> = {
  notes: "Notes",
  roadmap: "Roadmap",
  folder: "Folder",
};

// Types a user can spin up from the "+" tab menu.
export const CREATABLE_DOC_TYPES: DocumentType[] = ["notes", "roadmap"];
