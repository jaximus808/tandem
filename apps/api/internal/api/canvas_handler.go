package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/agentcanvas/api/internal/auth"
	"github.com/agentcanvas/api/internal/metrics"
	"github.com/agentcanvas/api/internal/store"
	"github.com/agentcanvas/api/internal/ws"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type Handler struct {
	store store.Store
	hub   *ws.Hub
	// localOwner, when set, owns every canvas created anonymously (local mode).
	localOwner *uuid.UUID
	// events is the outbound task-webhook seam (TDM-37). OPTIONAL: nil means
	// "no webhooks wired" and every emit is a no-op, which is what handler tests
	// and a WEBHOOKS_ENABLED=false deployment get. See task_events.go.
	events TaskEventEmitter
	// metrics is the /api/metrics registry (TDM-42). OPTIONAL: a nil registry
	// makes every counter increment a no-op (the methods are nil-receiver safe),
	// which is what handler tests and a METRICS_ENABLED=false deployment get.
	metrics *metrics.Registry
	// github is the READ-ONLY GitHub status proxy (TDM-45). Lazily defaulted by
	// githubClient(), so a Handler built without it still answers; tests replace
	// it with one pointed at an httptest server (nothing calls real GitHub).
	github *githubClient
	// authSvc validates the session cookie on the PUBLIC-by-code endpoints that
	// carry no JWT middleware — the exports (TDM-130). OPTIONAL: nil means no
	// session can be read, so those endpoints see every caller as anonymous (public
	// canvases only), which is the safe default for a Handler built without it.
	authSvc *auth.Service
	// waiters holds the long-poll waiters parked on GET /api/canvas/queue/wait
	// (TDM-148). A VALUE, not a pointer, and usable at its zero value, so a
	// Handler built as a struct literal still has a working registry — and so the
	// approve paths can signal it without a nil check. See queue_wait.go.
	waiters queueWaiters
}

// HandlerOption customizes NewHandler. Optional dependencies go here rather than
// in the positional signature so wiring one doesn't churn every test that
// constructs a Handler without caring about it.
type HandlerOption func(*Handler)

// WithTaskEvents attaches the outbound task-lifecycle event emitter.
func WithTaskEvents(e TaskEventEmitter) HandlerOption {
	return func(h *Handler) { h.events = e }
}

// WithMetrics attaches the operational metrics registry, so the handlers can
// count the task-queue signals (claims, claim conflicts, TTL takeovers) that no
// per-route latency number can show. Threaded as the concrete *metrics.Registry
// rather than an interface so a nil stays a usable nil.
func WithMetrics(reg *metrics.Registry) HandlerOption {
	return func(h *Handler) { h.metrics = reg }
}

// WithLocalOwner makes every canvas created WITHOUT a signed-in user owned by
// the given user — local mode, where the one user on the machine owns whatever
// an agent creates too (otherwise agent-created boards would not appear on the
// dashboard and would have no owner to approve on them).
func WithLocalOwner(id uuid.UUID) HandlerOption {
	return func(h *Handler) { h.localOwner = &id }
}

// WithAuthService attaches the session validator so the public-by-code endpoints
// (the exports) can resolve a caller's role from the cookie and enforce a private
// canvas's visibility, the same gate ServeWS applies (TDM-130).
func WithAuthService(a *auth.Service) HandlerOption {
	return func(h *Handler) { h.authSvc = a }
}

// resolveCanvasReadRole enforces a canvas's visibility on a public-by-code
// endpoint (the exports), exactly as ServeWS does before upgrading a socket
// (TDM-130): resolve the caller's role from the session cookie (anonymous → nil →
// public canvases only) and refuse a private canvas the caller holds no grant on.
// Returns (role, true) to proceed; on refusal it writes the HTTP response and
// returns false, so the caller just `return`s. The 8-char code alone must never
// be enough to read a PRIVATE canvas's contents.
func (h *Handler) resolveCanvasReadRole(w http.ResponseWriter, r *http.Request, canvas *store.Canvas) (string, bool) {
	var uid *uuid.UUID
	if h.authSvc != nil {
		if id, ok := sessionUserID(h.authSvc, r); ok {
			uid = &id
		}
	}
	role, err := h.store.ResolveCanvasRole(r.Context(), canvas, uid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not resolve canvas access")
		return "", false
	}
	if role == "none" {
		writeError(w, http.StatusForbidden, "this canvas is private — sign in with an account it's shared with")
		return "", false
	}
	return role, true
}

func NewHandler(s store.Store, hub *ws.Hub, opts ...HandlerOption) *Handler {
	h := &Handler{store: s, hub: hub}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// POST /api/canvases
func (h *Handler) CreateCanvas(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := decode(r, &body); err != nil || body.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	// Owned only when a logged-in human creates it (OptionalUser middleware).
	// MCP/gateway creates have no session cookie → anonymous, as intended.
	var owner *uuid.UUID
	var visibility, publicRole string
	uid, ok := UserIDFromCtx(r.Context())
	if !ok && h.localOwner != nil {
		// Local mode: an agent's canvas_create is still the local user's board.
		uid, ok = *h.localOwner, true
	}
	if ok {
		owner = &uid
		// Apply the owner's default posture (visibility + public role) to the new
		// canvas. If the lookup fails we leave them "" and fall back to the column
		// defaults ('public'/'write') rather than block the create. Anonymous
		// creates skip this and keep the fully-open column defaults.
		if u, err := h.store.GetUserByID(r.Context(), uid); err == nil {
			visibility = u.DefaultCanvasVisibility
			publicRole = u.DefaultPublicRole
		}
	}
	canvas, err := h.store.CreateCanvas(r.Context(), body.Name, owner, visibility, publicRole)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, canvas)
}

// GET /api/canvases/{code}
func (h *Handler) GetCanvasByCode(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	canvas, err := h.store.GetCanvasByCode(r.Context(), code)
	if err != nil {
		writeError(w, http.StatusNotFound, "canvas not found")
		return
	}
	// A PRIVATE canvas must not leak its name/owner — or even its existence — to a
	// caller with no grant (TDM-139). Resolve the session role and answer an
	// existence-agnostic 404 (NOT 403) when it's "none", so guessing an 8-char code
	// cannot distinguish a private canvas from one that doesn't exist. Public
	// canvases are unaffected: the code is their access token.
	if canvas.Visibility == "private" {
		var uid *uuid.UUID
		if h.authSvc != nil {
			if id, ok := sessionUserID(h.authSvc, r); ok {
				uid = &id
			}
		}
		role, rerr := h.store.ResolveCanvasRole(r.Context(), canvas, uid)
		if rerr != nil {
			writeError(w, http.StatusInternalServerError, "could not resolve canvas access")
			return
		}
		if role == "none" {
			writeError(w, http.StatusNotFound, "canvas not found")
			return
		}
	}
	writeJSON(w, http.StatusOK, canvas)
}

// GET /api/me/canvases  (RequireUser) — the signed-in user's owned canvases.
func (h *Handler) MeCanvases(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFromCtx(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not signed in")
		return
	}
	canvases, err := h.store.ListCanvasesByOwner(r.Context(), uid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, canvases)
}

// POST /api/canvases/{code}/copy  (RequireUser) — deep-copy a canvas into one
// owned by the signed-in user. This is how you "own" an anonymous canvas.
func (h *Handler) CopyCanvas(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFromCtx(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not signed in")
		return
	}
	src, err := h.store.GetCanvasByCode(r.Context(), chi.URLParam(r, "code"))
	if err != nil {
		writeError(w, http.StatusNotFound, "canvas not found")
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	_ = decode(r, &body)
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = src.Name
	}
	canvas, err := h.store.CopyCanvas(r.Context(), src.ID, uid, name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, canvas)
}

// POST /api/canvases/{code}/claim  (RequireUser) — take ownership of an
// anonymous canvas in place (no copy), so an agent that created it keeps writing
// to the same canvas the user now owns. Requires the private claim token; the
// canvas code alone (view capability) is not enough. First valid claim wins.
func (h *Handler) ClaimCanvas(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFromCtx(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not signed in")
		return
	}
	var body struct {
		ClaimToken string `json:"claimToken"`
	}
	if err := decode(r, &body); err != nil || strings.TrimSpace(body.ClaimToken) == "" {
		writeError(w, http.StatusBadRequest, "claimToken is required")
		return
	}
	canvas, err := h.store.ClaimCanvas(r.Context(), chi.URLParam(r, "code"), strings.TrimSpace(body.ClaimToken), uid)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrCanvasNotFound):
			writeError(w, http.StatusNotFound, "canvas not found")
		case errors.Is(err, store.ErrAlreadyClaimed):
			// Distinct from a bad token so the web can offer "Copy instead".
			writeError(w, http.StatusConflict, "canvas already claimed")
		case errors.Is(err, store.ErrInvalidClaimToken):
			writeError(w, http.StatusForbidden, "invalid claim token")
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, canvas)
}

// GET /api/canvas/state  (JWT required)
//
// Read shapes (query params), to keep a big canvas from barfing the agent's
// token budget — see state_read.go:
//   - ?full=true             → the entire canvas (back-compat).
//   - ?fields=roadmapItems,… → only those kinds, in full.
//   - (neither)              → lightweight summary: per-kind counts + names.
func (h *Handler) GetState(w http.ResponseWriter, r *http.Request) {
	fields, err := validateStateFields(r.URL.Query().Get("fields"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	full := r.URL.Query().Get("full") == "true"

	canvasID := CanvasIDFromCtx(r.Context())

	// This endpoint is the agent/gateway read path (the web client never calls
	// it — it lives off the WS state stream). So a hit here means an agent just
	// read the canvas; pulse that to viewers as live "reading" presence.
	broadcastActivity(h.hub, canvasID, "read")

	// Default (no fields, not full) → cheap summary. The store does the trimming
	// (exact counts + a capped name-only sample per kind), so a summary read never
	// loads the whole board.
	if !full && len(fields) == 0 {
		canvas, sum, edits, err := h.store.GetCanvasSummary(r.Context(), canvasID, maxSummaryNames)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, summarizeFromStore(canvas, sum, edits))
		return
	}

	// Field-filtered read → fetch only the requested kinds via per-table SELECTs,
	// again avoiding a full-canvas load. full=true keeps the whole-canvas escape
	// hatch (GetCanvasState) for back-compat.
	if len(fields) > 0 {
		canvas, state, edits, err := h.store.GetCanvasKinds(r.Context(), canvasID, fields)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, stateMsg{
			Type:         "state",
			Canvas:       canvas,
			State:        state,
			PendingEdits: edits,
		})
		return
	}

	canvas, state, edits, err := h.store.GetCanvasState(r.Context(), canvasID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, stateMsg{
		Type:         "state",
		Canvas:       canvas,
		State:        state,
		PendingEdits: edits,
	})
}

// POST /api/canvas/mode  (JWT required)
func (h *Handler) SetMode(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Mode string `json:"mode"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if !isValidMode(body.Mode) {
		writeError(w, http.StatusBadRequest, "invalid mode")
		return
	}
	canvasID := CanvasIDFromCtx(r.Context())
	if _, err := h.store.SetMode(r.Context(), canvasID, body.Mode); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	broadcastStateAsync(r.Context(), h.store, h.hub, canvasID)
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

// EnableMode turns on a tab the user added via "+", even before it has content.
// POST /api/canvas/mode/enable  (JWT required)
func (h *Handler) EnableMode(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Mode string `json:"mode"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if !isEnableableMode(body.Mode) {
		writeError(w, http.StatusBadRequest, "invalid mode")
		return
	}
	canvasID := CanvasIDFromCtx(r.Context())
	if _, err := h.store.EnableMode(r.Context(), canvasID, body.Mode); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	broadcastStateAsync(r.Context(), h.store, h.hub, canvasID)
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

// POST /api/canvas/template  (JWT required)
func (h *Handler) ApplyTemplate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TemplateID string `json:"templateId"`
		Mode       string `json:"mode"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if !isValidMode(body.Mode) {
		writeError(w, http.StatusBadRequest, "invalid mode")
		return
	}
	canvasID := CanvasIDFromCtx(r.Context())
	if _, err := h.store.ApplyTemplate(r.Context(), canvasID, body.Mode); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	broadcastStateAsync(r.Context(), h.store, h.hub, canvasID)
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func isValidMode(mode string) bool {
	switch mode {
	case "welcome", "docs", "roadmap":
		return true
	}
	return false
}

// isEnableableMode is isValidMode minus "welcome" — welcome is the template
// picker, not a content tab a user can add.
func isEnableableMode(mode string) bool {
	return mode != "welcome" && isValidMode(mode)
}

// POST /api/canvas/notes
func (h *Handler) CreateNote(w http.ResponseWriter, r *http.Request) {
	canvasID := CanvasIDFromCtx(r.Context())
	var body struct {
		Body       string     `json:"body"`
		ImageRefs  []string   `json:"imageRefs"`
		ParentID   *uuid.UUID `json:"parentId"`
		ParentKind *string    `json:"parentKind"`
		CreatedBy  string     `json:"createdBy"`
		Document   string     `json:"document"` // target notes doc (id or name)
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.CreatedBy == "" {
		body.CreatedBy = "agent"
	}
	docID, ok := h.attachDocument(w, r, canvasID, "notes", body.Document)
	if !ok {
		return
	}
	n := &store.Note{
		ID: uuid.New(), Kind: "note", DocumentID: docID,
		Body: body.Body, ImageRefs: body.ImageRefs,
		ParentID: body.ParentID, ParentKind: body.ParentKind,
		CreatedBy: body.CreatedBy,
		// Provenance (TDM-40): server-derived, never from the body — CreatedBy
		// above is the caller's freeform label, AuthoredBy is the trustworthy one.
		AuthoredBy: AuthorFromCtx(r.Context()),
	}
	if _, err := h.store.CreateNote(r.Context(), canvasID, n); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	broadcastStateAsync(r.Context(), h.store, h.hub, canvasID)
	writeJSON(w, http.StatusCreated, n)
}

// POST /api/canvas/notes/batch — add many notes in one shot. The whole array
// persists via a single bulk INSERT and fires ONE state broadcast, so an
// N-note batch costs one full-canvas reload instead of N. `document` sets a
// shared target notes doc for the batch; each note may still override it with
// its own `document`. sortOrder is assigned the same way as the single path
// (appended after the last note in its target document) — see
// store.CreateNotes.
func (h *Handler) CreateNotesBatch(w http.ResponseWriter, r *http.Request) {
	canvasID := CanvasIDFromCtx(r.Context())
	type noteItem struct {
		Body       string     `json:"body"`
		ImageRefs  []string   `json:"imageRefs"`
		ParentID   *uuid.UUID `json:"parentId"`
		ParentKind *string    `json:"parentKind"`
		CreatedBy  string     `json:"createdBy"`
		Document   string     `json:"document"` // per-note override
	}
	var body struct {
		Document string     `json:"document"` // shared target for the batch
		Notes    []noteItem `json:"notes"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(body.Notes) == 0 {
		writeError(w, http.StatusBadRequest, "notes: at least one note is required")
		return
	}
	// Resolve each note's target doc through a cache so a shared/default doc is
	// resolved (and created on demand) exactly once for the whole batch.
	docCache := map[string]*uuid.UUID{}
	resolveDoc := func(ref string) (*uuid.UUID, error) {
		if cached, ok := docCache[ref]; ok {
			return cached, nil
		}
		id, err := h.documentForContent(r.Context(), canvasID, "notes", ref)
		if err != nil {
			return nil, err
		}
		docCache[ref] = &id
		return &id, nil
	}
	// Provenance (TDM-40) is per-request, so one derivation covers the batch.
	author := AuthorFromCtx(r.Context())
	notes := make([]*store.Note, 0, len(body.Notes))
	for i := range body.Notes {
		item := body.Notes[i]
		createdBy := item.CreatedBy
		if createdBy == "" {
			createdBy = "agent"
		}
		ref := item.Document
		if ref == "" {
			ref = body.Document
		}
		docID, err := resolveDoc(ref)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		notes = append(notes, &store.Note{
			ID: uuid.New(), Kind: "note", DocumentID: docID,
			Body: item.Body, ImageRefs: item.ImageRefs,
			ParentID: item.ParentID, ParentKind: item.ParentKind,
			CreatedBy: createdBy, AuthoredBy: author,
		})
	}
	if _, err := h.store.CreateNotes(r.Context(), canvasID, notes); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	broadcastStateAsync(r.Context(), h.store, h.hub, canvasID)
	writeJSON(w, http.StatusCreated, map[string]any{"notes": notes})
}

// PATCH /api/canvas/notes/{id}
func (h *Handler) UpdateNote(w http.ResponseWriter, r *http.Request) {
	canvasID := CanvasIDFromCtx(r.Context())
	id, _ := uuid.Parse(chi.URLParam(r, "id"))
	var patch store.NotePatch
	if err := decode(r, &patch); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := h.store.UpdateNote(r.Context(), canvasID, id, patch); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	broadcastStateAsync(r.Context(), h.store, h.hub, canvasID)
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

// DELETE /api/canvas/notes/{id}
func (h *Handler) DeleteNote(w http.ResponseWriter, r *http.Request) {
	canvasID := CanvasIDFromCtx(r.Context())
	id, _ := uuid.Parse(chi.URLParam(r, "id"))
	if _, err := h.store.DeleteNote(r.Context(), canvasID, id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	broadcastStateAsync(r.Context(), h.store, h.hub, canvasID)
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

// GET /api/canvas/roadmap-items?assignee=  — list roadmap items, optionally
// filtered by assignee. assignee=agent returns the agent-task queue: goals a
// human marked for an agent session to pull and execute (the MCP hits this).
func (h *Handler) ListRoadmapItems(w http.ResponseWriter, r *http.Request) {
	canvasID := CanvasIDFromCtx(r.Context())
	assignee := r.URL.Query().Get("assignee")
	if assignee != "" && assignee != "agent" && assignee != "human" {
		writeError(w, http.StatusBadRequest, "assignee must be 'agent' or 'human'")
		return
	}
	items, err := h.store.ListRoadmapItems(r.Context(), canvasID, assignee)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// POST /api/canvas/roadmap-items
func (h *Handler) CreateRoadmapItem(w http.ResponseWriter, r *http.Request) {
	canvasID := CanvasIDFromCtx(r.Context())
	var body struct {
		ParentID  *uuid.UUID `json:"parentId"`
		Title     string     `json:"title"`
		Body      string     `json:"body"`
		Status    string     `json:"status"`
		Stage     string     `json:"stage"`
		Assignee  string     `json:"assignee"`
		SortOrder int        `json:"sortOrder"`
		CreatedBy string     `json:"createdBy"`
		Document  string     `json:"document"` // target roadmap doc (id or name)
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.CreatedBy == "" {
		body.CreatedBy = "agent"
	}
	if body.Status == "" {
		body.Status = "todo"
	}
	if body.Assignee != "" && body.Assignee != "agent" && body.Assignee != "human" {
		writeError(w, http.StatusBadRequest, "assignee must be 'agent' or 'human'")
		return
	}
	// "human" is the implicit default — store it as unmarked so the column stays
	// NULL for goals and only agent tasks carry a value.
	if body.Assignee == "human" {
		body.Assignee = ""
	}
	docID, ok := h.attachDocument(w, r, canvasID, "roadmap", body.Document)
	if !ok {
		return
	}
	item := &store.RoadmapItem{
		ID: uuid.New(), Kind: "roadmap", DocumentID: docID,
		ParentID: body.ParentID, Title: body.Title, Body: body.Body,
		Status: body.Status, Stage: body.Stage, Assignee: body.Assignee,
		SortOrder: body.SortOrder, CreatedBy: body.CreatedBy,
	}
	if _, err := h.store.CreateRoadmapItem(r.Context(), canvasID, item); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	broadcastStateAsync(r.Context(), h.store, h.hub, canvasID)
	writeJSON(w, http.StatusCreated, item)
}

// POST /api/canvas/roadmap-items/batch — add many roadmap items in one shot.
// The whole array persists via a single bulk INSERT and fires ONE state
// broadcast, so an N-item batch costs one full-canvas reload instead of N.
// `document` sets a shared target roadmap doc for the batch; each item may
// still override it with its own `document`.
func (h *Handler) CreateRoadmapItemsBatch(w http.ResponseWriter, r *http.Request) {
	canvasID := CanvasIDFromCtx(r.Context())
	type roadmapItem struct {
		ParentID  *uuid.UUID `json:"parentId"`
		Title     string     `json:"title"`
		Body      string     `json:"body"`
		Status    string     `json:"status"`
		Stage     string     `json:"stage"`
		Assignee  string     `json:"assignee"`
		SortOrder int        `json:"sortOrder"`
		CreatedBy string     `json:"createdBy"`
		Document  string     `json:"document"` // per-item override
	}
	var body struct {
		Document string        `json:"document"` // shared target for the batch
		Items    []roadmapItem `json:"items"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(body.Items) == 0 {
		writeError(w, http.StatusBadRequest, "items: at least one roadmap item is required")
		return
	}
	// Resolve each item's target doc through a cache so a shared/default doc is
	// resolved (and created on demand) exactly once for the whole batch.
	docCache := map[string]*uuid.UUID{}
	resolveDoc := func(ref string) (*uuid.UUID, error) {
		if cached, ok := docCache[ref]; ok {
			return cached, nil
		}
		id, err := h.documentForContent(r.Context(), canvasID, "roadmap", ref)
		if err != nil {
			return nil, err
		}
		docCache[ref] = &id
		return &id, nil
	}
	items := make([]*store.RoadmapItem, 0, len(body.Items))
	for i := range body.Items {
		item := body.Items[i]
		createdBy := item.CreatedBy
		if createdBy == "" {
			createdBy = "agent"
		}
		if item.Status == "" {
			item.Status = "todo"
		}
		if item.Assignee != "" && item.Assignee != "agent" && item.Assignee != "human" {
			writeError(w, http.StatusBadRequest, "assignee must be 'agent' or 'human'")
			return
		}
		// "human" is the implicit default — store it as unmarked so the column
		// stays NULL for goals and only agent tasks carry a value.
		if item.Assignee == "human" {
			item.Assignee = ""
		}
		ref := item.Document
		if ref == "" {
			ref = body.Document
		}
		docID, err := resolveDoc(ref)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		items = append(items, &store.RoadmapItem{
			ID: uuid.New(), Kind: "roadmap", DocumentID: docID,
			ParentID: item.ParentID, Title: item.Title, Body: item.Body,
			Status: item.Status, Stage: item.Stage, Assignee: item.Assignee,
			SortOrder: item.SortOrder, CreatedBy: createdBy,
		})
	}
	if _, err := h.store.CreateRoadmapItems(r.Context(), canvasID, items); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	broadcastStateAsync(r.Context(), h.store, h.hub, canvasID)
	writeJSON(w, http.StatusCreated, map[string]any{"items": items})
}

// PATCH /api/canvas/roadmap-items/{id}
func (h *Handler) UpdateRoadmapItem(w http.ResponseWriter, r *http.Request) {
	canvasID := CanvasIDFromCtx(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var patch store.RoadmapItemPatch
	if err := decode(r, &patch); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := h.store.UpdateRoadmapItem(r.Context(), canvasID, id, patch); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	broadcastStateAsync(r.Context(), h.store, h.hub, canvasID)
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

// DELETE /api/canvas/roadmap-items/{id}
func (h *Handler) DeleteRoadmapItem(w http.ResponseWriter, r *http.Request) {
	canvasID := CanvasIDFromCtx(r.Context())
	id, _ := uuid.Parse(chi.URLParam(r, "id"))
	if _, err := h.store.DeleteRoadmapItem(r.Context(), canvasID, id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	broadcastStateAsync(r.Context(), h.store, h.hub, canvasID)
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

// POST /api/canvas/pending-edits
func (h *Handler) CreatePendingEdit(w http.ResponseWriter, r *http.Request) {
	canvasID := CanvasIDFromCtx(r.Context())
	var body struct {
		EntityID    uuid.UUID `json:"entityId"`
		Instruction string    `json:"instruction"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	edit, err := h.store.CreatePendingEdit(r.Context(), canvasID, body.EntityID, body.Instruction)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Synchronous on purpose (unlike the other single-item writes): pending-edit
	// create/delete do NOT bump the canvas version, so the frontend's drop-older
	// guard can't order two of their snapshots. Broadcasting synchronously keeps
	// the gateway's serialized create→delete ordered (the handler doesn't return —
	// and so the next tool call can't start — until this snapshot is out), which an
	// async detach would break, letting a stale "edit still pending" snapshot land
	// after the delete. These ops are rare and off the hot path, so the sync reload
	// cost is fine.
	broadcastState(r.Context(), h.store, h.hub, canvasID)
	writeJSON(w, http.StatusCreated, edit)
}

// GET /api/canvas/pending-edits  (JWT required)
//
// Cheap, targeted read of just the pending edit queue. canvas_pending_edits_read
// used to hit GET /api/canvas/state, which now builds the whole summary (counts +
// per-kind names for the entire board) only to throw all of it away except
// pendingEdits — wasteful server work on every poll. This queries the one table
// and returns nothing else.
func (h *Handler) ListPendingEdits(w http.ResponseWriter, r *http.Request) {
	canvasID := CanvasIDFromCtx(r.Context())
	edits, err := h.store.ListPendingEdits(r.Context(), canvasID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pendingEdits": edits})
}

// DELETE /api/canvas/pending-edits/{id}
func (h *Handler) DeletePendingEdit(w http.ResponseWriter, r *http.Request) {
	canvasID := CanvasIDFromCtx(r.Context())
	id, _ := uuid.Parse(chi.URLParam(r, "id"))
	if err := h.store.DeletePendingEdit(r.Context(), canvasID, id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Synchronous on purpose — see CreatePendingEdit. Pending-edit changes don't
	// bump the version, so keep the create→delete broadcast ordering that gateway
	// serialization + a synchronous broadcast provide.
	broadcastState(r.Context(), h.store, h.hub, canvasID)
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}
