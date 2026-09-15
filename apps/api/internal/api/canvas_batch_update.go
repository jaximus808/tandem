package api

import (
	"net/http"

	"github.com/agentcanvas/api/internal/store"
	"github.com/google/uuid"
)

// Batch-UPDATE handlers.
//
// These mirror the CreateXBatch pattern (canvas_handler.go): they patch MANY
// existing elements in ONE request and fire exactly ONE state broadcast at the
// very end (NOT per item). The single-broadcast-per-batch is the whole point —
// it's the fix for agents blowing the per-turn tool-call limit when they had to
// call canvas_event_update once per element (see the applyBatch note at
// ws_handler.go and the create-batch note at canvas_handler.go:684).
//
// Each request body is {items:[{id, <partial fields>}]}. Each item embeds the
// existing per-type Patch struct (so the partial fields decode with the exact
// same json tags and semantics as the single PATCH handlers) plus the target
// id. Items with a missing/zero id are skipped; the response returns the ids
// that were actually applied.
//
// "Applied" means ROWS-AFFECTED, not "patched without error" (TDM-138). Each
// store.UpdateX now returns the true rows-affected, so an id that is absent or
// belongs to another canvas — a silent 0-row no-op — is NOT echoed back as
// updated. See runBatchAffected / finishBatch in batch_common.go, which also
// converge the canvas even when a batch partially fails.
//
// Execution: each handler validates its items synchronously (400s stay on the
// request goroutine), then runs the per-item store calls concurrently via
// runBatch — every element is its own DB row keyed by id, so the writes are
// independent. The one exception is sheet columns, which live inside their
// sheet's JSON array and so stay serial (see UpdateSheetColumnsBatch). The
// trailing broadcast is async (broadcastBatchUpdate), so the caller unblocks as
// soon as the writes are durable.

// broadcastBatchUpdate fires the single trailing state broadcast and writes the
// list of applied ids. Callers do all the per-item work first, then hand the
// applied ids here. The broadcast is async: the writes are already durable, so
// the caller (an agent over the MCP path) unblocks immediately instead of waiting
// on a full-canvas read + marshal + WS fan-out that only exists to converge
// viewers — the same reasoning as broadcastStateAsync elsewhere.
func (h *Handler) broadcastBatchUpdate(w http.ResponseWriter, r *http.Request, updated []string) {
	// Thin wrapper over finishBatch (no batch error) for the serial handlers that
	// build their applied list directly (sheet columns). The parallel handlers call
	// finishBatch themselves so they can surface a partial-batch error.
	h.finishBatch(w, r, updated, nil, "updated")
}

// POST /api/canvas/notes/batch-update
func (h *Handler) UpdateNotesBatch(w http.ResponseWriter, r *http.Request) {
	canvasID := CanvasIDFromCtx(r.Context())
	type noteItem struct {
		ID uuid.UUID `json:"id"`
		store.NotePatch
	}
	var body struct {
		Items []noteItem `json:"items"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(body.Items) == 0 {
		writeError(w, http.StatusBadRequest, "items: at least one entry is required")
		return
	}
	if tooManyBatchItems(w, len(body.Items)) {
		return
	}
	items := make([]noteItem, 0, len(body.Items))
	for _, item := range body.Items {
		if item.ID == uuid.Nil {
			continue
		}
		items = append(items, item)
	}
	applied, batchErr := runBatchAffected(len(items), func(i int) (int, error) {
		return h.store.UpdateNote(store.WithoutVersionBump(r.Context()), canvasID, items[i].ID, items[i].NotePatch)
	})
	h.finishBatch(w, r, appliedIDs(applied, func(i int) string { return items[i].ID.String() }), batchErr, "updated")
}

// POST /api/canvas/roadmap-items/batch-update
func (h *Handler) UpdateRoadmapItemsBatch(w http.ResponseWriter, r *http.Request) {
	canvasID := CanvasIDFromCtx(r.Context())
	type roadmapItem struct {
		ID uuid.UUID `json:"id"`
		store.RoadmapItemPatch
	}
	var body struct {
		Items []roadmapItem `json:"items"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(body.Items) == 0 {
		writeError(w, http.StatusBadRequest, "items: at least one entry is required")
		return
	}
	if tooManyBatchItems(w, len(body.Items)) {
		return
	}
	items := make([]roadmapItem, 0, len(body.Items))
	for _, item := range body.Items {
		if item.ID == uuid.Nil {
			continue
		}
		items = append(items, item)
	}
	applied, batchErr := runBatchAffected(len(items), func(i int) (int, error) {
		return h.store.UpdateRoadmapItem(store.WithoutVersionBump(r.Context()), canvasID, items[i].ID, items[i].RoadmapItemPatch)
	})
	h.finishBatch(w, r, appliedIDs(applied, func(i int) string { return items[i].ID.String() }), batchErr, "updated")
}
