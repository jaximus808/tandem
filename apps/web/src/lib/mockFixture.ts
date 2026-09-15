// Hardcoded canvas fixture for offline UI iteration.
// Activated by setting VITE_MOCK=1 (see package.json `dev:mock`).
// When on, the WS layer and API layer use in-memory state instead of the server.

import type {
  CanvasMeta,
  CanvasState,
  Note,
  RoadmapItem,
  Document,
  EntityId,
} from "../types";

export const MOCK_ENABLED = import.meta.env.VITE_MOCK === "1";

let counter = 0;
const id = (): EntityId => `mock-${++counter}`;

const note1Id = id();
const note2Id = id();

const notes: Note[] = [
  {
    id: note1Id,
    kind: "note",
    body: `# Trip plan: McLean → Snowmass

**Destination:** Anderson Ranch Arts Center, 5263 Owl Creek Rd

## Strategy comparison

| Option | Cost (RT) | Door-to-door | Pros | Cons |
|---|---|---|---|---|
| DCA → DEN + rental | $400–650 | ~8h | Cheapest, flexible | Long drive after flight |
| Fly to ASE | $430–580 | ~7h | Less driving | Weather cancellations |
| Fly to EGE | $550–700 | ~6h | Shorter drive | Limited flights |

## Recommended play-by-play

1. Log off Thursday 5pm ET
2. DCA → DEN nonstop (6:30pm)
3. Pick up rental, drive I-70 W → CO-82 W
4. Arrive Snowmass ~1am MT

> Alternative: hotel near DEN, drive Friday morning (you WFH Fridays).
`,
    imageRefs: [],
    sortOrder: 0,
    createdBy: "agent",
    updatedAt: Date.now(),
  },
  {
    id: note2Id,
    kind: "note",
    body: "Quick reminder: pack the wide-angle lens for Shibuya. Also need a foldable tripod.",
    imageRefs: [],
    sortOrder: 1,
    createdBy: "user",
    updatedAt: Date.now(),
  },
];

const launchId = id();
const authId = id();
const authJwtId = id();
const authSsoId = id();
const billingId = id();
const billingStripeId = id();
const billingInvoiceId = id();
const onboardingId = id();
const q3Id = id();
const dashId = id();
const bugBashId = id();
const techDebtId = id();
const loggerId = id();

const roadmapItems: RoadmapItem[] = [
  // Top-level: Launch v2 (in progress, 3 children)
  { id: launchId, kind: "roadmap", title: "🚀 Launch v2",
    body: "", status: "in_progress", sortOrder: 0,
    createdBy: "agent", updatedAt: Date.now() },
  { id: authId, kind: "roadmap", parentId: launchId, title: "Auth rewrite",
    body: "", status: "done", sortOrder: 0,
    createdBy: "agent", updatedAt: Date.now() },
  { id: authJwtId, kind: "roadmap", parentId: authId, title: "Migrate JWT signing key",
    body: "", status: "done", sortOrder: 0,
    createdBy: "agent", updatedAt: Date.now() },
  { id: authSsoId, kind: "roadmap", parentId: authId, title: "SSO support (Okta + Google)",
    body: "", status: "in_progress", sortOrder: 1,
    createdBy: "agent", updatedAt: Date.now() },
  { id: billingId, kind: "roadmap", parentId: launchId, title: "Billing pipeline",
    body: "", status: "todo", sortOrder: 1,
    createdBy: "agent", updatedAt: Date.now() },
  { id: billingStripeId, kind: "roadmap", parentId: billingId, title: "Stripe webhooks",
    body: "", status: "todo", sortOrder: 0,
    createdBy: "agent", updatedAt: Date.now() },
  { id: billingInvoiceId, kind: "roadmap", parentId: billingId, title: "Invoice email templates",
    body: "", status: "blocked", sortOrder: 1,
    createdBy: "user", updatedAt: Date.now() },
  { id: onboardingId, kind: "roadmap", parentId: launchId, title: "Onboarding redesign",
    body: "", status: "in_progress", sortOrder: 2,
    createdBy: "agent", updatedAt: Date.now() },

  // Top-level: Q3 polish (todo, 2 children)
  { id: q3Id, kind: "roadmap", title: "📊 Q3 polish",
    body: "", status: "todo", sortOrder: 1,
    createdBy: "agent", updatedAt: Date.now() },
  { id: dashId, kind: "roadmap", parentId: q3Id, title: "Dashboard perf audit",
    body: "", status: "todo", sortOrder: 0,
    createdBy: "agent", updatedAt: Date.now() },
  { id: bugBashId, kind: "roadmap", parentId: q3Id, title: "Bug bash week",
    body: "", status: "todo", sortOrder: 1,
    createdBy: "user", updatedAt: Date.now() },

  // Top-level: Tech debt (blocked, 1 child)
  { id: techDebtId, kind: "roadmap", title: "🐛 Tech debt",
    body: "", status: "blocked", sortOrder: 2,
    createdBy: "agent", updatedAt: Date.now() },
  { id: loggerId, kind: "roadmap", parentId: techDebtId, title: "Migrate off legacy logger",
    body: "", status: "blocked", sortOrder: 0,
    createdBy: "agent", updatedAt: Date.now() },
];

const initialCanvas: CanvasMeta = {
  id: "mock-canvas",
  code: "MOCKCNV1",
  name: "Mock Canvas (offline)",
  mode: "docs",
  version: 1,
  createdAt: new Date().toISOString(),
  updatedAt: new Date().toISOString(),
};

function toRecord<T extends { id: EntityId }>(arr: T[]): Record<EntityId, T> {
  const out: Record<EntityId, T> = {};
  for (const item of arr) out[item.id] = item;
  return out;
}

// Documents (migration 0024): the mock's content predates the model, so wrap
// each kind in a named document and stamp its rows with that documentId — the
// canvas tab strip renders one tab per document.
const notesDocId = id();
const roadmapDocId = id();

const documents: Document[] = [
  { id: notesDocId, kind: "document", type: "notes", name: "Notes", sortOrder: 2, config: {}, createdBy: "user", updatedAt: Date.now() },
  { id: roadmapDocId, kind: "document", type: "roadmap", name: "Roadmap", sortOrder: 3, config: {}, createdBy: "user", updatedAt: Date.now() },
];

const stamp = <T,>(arr: T[], documentId: string): T[] => arr.map((e) => ({ ...e, documentId }));

const initialState: CanvasState = {
  version: 1,
  mode: "docs",
  enabledModes: [],
  documents: toRecord(documents),
  notes: toRecord(stamp(notes, notesDocId)),
  roadmapItems: toRecord(stamp(roadmapItems, roadmapDocId)),
  actions: {},
  agents: {},
};

export const mockCanvas: CanvasMeta = initialCanvas;
export const mockState: CanvasState = initialState;
export const mockNewId = id;
