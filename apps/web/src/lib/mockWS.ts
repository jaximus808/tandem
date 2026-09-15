// In-memory replacement for the WS + REST layers, used when VITE_MOCK=1.
// Lets you iterate on the UI with hot reload and no backend.

import type {
  CanvasMeta,
  CanvasState,
  FreshnessFields,
  FreshnessPatchFields,
  PendingEdit,
  WSClientMessage,
  Note,
  RoadmapItem,
} from "../types";
import { mockCanvas, mockState, mockNewId } from "./mockFixture";

/* Merge a patch that may carry the freshness pair (migration 0037). Clearing
   rides explicit flags, because an absent field means "leave it alone" — so a
   plain spread would leave a cleared verifiedAt in place and strand the flags
   on the entity. The API applies exactly this rule server-side; the mock has to
   agree or "Clear verification" would appear broken in mock mode only. */
function mergeWithFreshness<T extends FreshnessFields>(
  entity: T,
  partial: Partial<T> & FreshnessPatchFields,
): T {
  const merged = { ...entity, ...partial } as T & FreshnessPatchFields;
  if (partial.clearVerifiedAt) delete merged.verifiedAt;
  if (partial.clearStaleAfterSeconds) delete merged.staleAfterSeconds;
  delete merged.clearVerifiedAt;
  delete merged.clearStaleAfterSeconds;
  return merged;
}

type StateHandler = (canvas: CanvasMeta, canvases: CanvasMeta[], state: CanvasState, pendingEdits: PendingEdit[]) => void;

let canvas: CanvasMeta = { ...mockCanvas };
let state: CanvasState = JSON.parse(JSON.stringify(mockState));
const handlers: StateHandler[] = [];

function broadcast() {
  // Async tick so handlers run after the current render flush, mimicking
  // network round-trip ordering.
  queueMicrotask(() => {
    handlers.forEach((h) => h(canvas, [], state, []));
  });
}

export function mockConnect(_code: string) {
  // Schedule an initial state push so App.tsx can render.
  broadcast();
}

export function mockOnStateUpdate(fn: StateHandler): () => void {
  handlers.push(fn);
  // Replay current state on subscribe.
  queueMicrotask(() => fn(canvas, [], state, []));
  return () => {
    const i = handlers.indexOf(fn);
    if (i >= 0) handlers.splice(i, 1);
  };
}

export function mockSendOp(op: WSClientMessage) {
  state = applyOp(state, op);
  canvas = applyToCanvas(canvas, op);
  canvas.version = state.version;
  broadcast();
}

function applyToCanvas(c: CanvasMeta, op: WSClientMessage): CanvasMeta {
  switch (op.op) {
    case "mode.set":
      return { ...c, mode: op.mode };
    case "template.apply":
      return { ...c, mode: op.mode };
    default:
      return c;
  }
}

function applyOp(s: CanvasState, op: WSClientMessage): CanvasState {
  const next: CanvasState = {
    ...s,
    version: s.version + 1,
    notes: { ...s.notes },
    roadmapItems: { ...s.roadmapItems },
  };

  switch (op.op) {
    case "mode.set":
      next.mode = op.mode;
      break;
    case "template.apply":
      next.mode = op.mode;
      break;

    case "note.add": {
      const id = mockNewId();
      const data = (op.data ?? { body: "", imageRefs: [] }) as Omit<Note, "id" | "kind" | "createdBy" | "updatedAt">;
      next.notes[id] = {
        id,
        kind: "note",
        createdBy: "user",
        updatedAt: Date.now(),
        body: data.body ?? "",
        imageRefs: data.imageRefs ?? [],
        parentId: data.parentId,
        ...(data as object),
      } as Note;
      break;
    }
    case "note.update":
      if (next.notes[op.id]) {
        next.notes[op.id] = {
          ...mergeWithFreshness(next.notes[op.id], op.partial),
          updatedAt: Date.now(),
        };
      }
      break;
    case "note.delete":
      delete next.notes[op.id];
      break;

    case "roadmap.add": {
      const id = mockNewId();
      const data = op.data as Omit<RoadmapItem, "id" | "kind" | "createdBy" | "updatedAt">;
      next.roadmapItems[id] = {
        id,
        kind: "roadmap",
        createdBy: "user",
        updatedAt: Date.now(),
        title: data.title ?? "",
        body: data.body ?? "",
        status: data.status ?? "todo",
        sortOrder: data.sortOrder ?? 0,
        parentId: data.parentId,
      };
      break;
    }
    case "roadmap.update":
      if (next.roadmapItems[op.id]) {
        next.roadmapItems[op.id] = {
          ...mergeWithFreshness(next.roadmapItems[op.id], op.partial),
          updatedAt: Date.now(),
        };
      }
      break;
    case "roadmap.delete": {
      // Mirror the DB CASCADE on parent_id: deleting an item drops its descendants too.
      const toDelete = new Set<string>([op.id]);
      let grew = true;
      while (grew) {
        grew = false;
        for (const it of Object.values(next.roadmapItems)) {
          if (it.parentId && toDelete.has(it.parentId) && !toDelete.has(it.id)) {
            toDelete.add(it.id);
            grew = true;
          }
        }
      }
      for (const id of toDelete) delete next.roadmapItems[id];
      break;
    }
    case "roadmap.reorder": {
      for (const u of op.updates) {
        const existing = next.roadmapItems[u.id];
        if (!existing) continue;
        next.roadmapItems[u.id] = {
          ...existing,
          parentId: u.parentId ?? undefined,
          sortOrder: u.sortOrder,
          updatedAt: Date.now(),
        };
      }
      break;
    }

  }

  // Auto-leave welcome on any entity write (matches server behavior).
  if (s.mode === "welcome") {
    if (op.op.startsWith("note.")) next.mode = "docs";
    else if (op.op.startsWith("roadmap.")) next.mode = "roadmap";
  }

  return next;
}
