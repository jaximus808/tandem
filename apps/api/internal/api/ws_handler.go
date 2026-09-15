package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/agentcanvas/api/internal/auth"
	"github.com/agentcanvas/api/internal/store"
	"github.com/agentcanvas/api/internal/ws"
	"github.com/google/uuid"
)

// errOpSkipped is a sentinel applyOp returns for a malformed/no-op sub-message
// (bad JSON, missing id, invalid enum, empty update list, ...). It's not a
// mutation failure — the caller does not log it again (any diagnostic
// log.Printf already fired inline in the switch below), and it never
// triggers a broadcast on its own. Distinguishing it from a real store error
// lets a "batch" op skip one bad sub-op without silencing genuine mutation
// errors for the others.
var errOpSkipped = errors.New("ws: op skipped")

// WSHandler handles WebSocket upgrades for browser clients.
type WSHandler struct {
	store   store.Store
	hub     *ws.Hub
	authSvc *auth.Service
}

func NewWSHandler(s store.Store, hub *ws.Hub, authSvc *auth.Service) *WSHandler {
	return &WSHandler{store: s, hub: hub, authSvc: authSvc}
}

// GET /ws?code=CANVAS_CODE
func (wh *WSHandler) ServeWS(w http.ResponseWriter, r *http.Request) {
	code := strings.ToUpper(r.URL.Query().Get("code"))
	if code == "" {
		writeError(w, http.StatusBadRequest, "code query parameter required")
		return
	}

	canvas, err := wh.store.GetCanvasByCode(r.Context(), code)
	if err != nil {
		writeError(w, http.StatusNotFound, "canvas not found")
		return
	}

	// Resolve the connecting human's role from the session cookie (anonymous →
	// nil → public canvases only). This MUST happen before the upgrade so we can
	// still write an HTTP error: once upgraded we can only close the socket.
	var uid *uuid.UUID
	if id, ok := sessionUserID(wh.authSvc, r); ok {
		uid = &id
	}
	role, err := wh.store.ResolveCanvasRole(r.Context(), canvas, uid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not resolve canvas access")
		return
	}
	if role == "none" {
		writeError(w, http.StatusForbidden, "this canvas is private — sign in with an account it's shared with")
		return
	}
	canWrite := role == "write"

	conn, err := ws.Upgrade(w, r)
	if err != nil {
		log.Printf("ws upgrade: %v", err)
		return
	}

	canvasID := canvas.ID
	// Read-only viewers still connect and receive state + live broadcasts; the
	// closure simply mutes their inbound writes (see handleOp). The gate is read
	// from the client each op (not captured) so a live sharing change can flip it
	// without reconnecting — see reevaluateAccess.
	// Provenance for everything this socket writes (TDM-40). The WS is the
	// browser's channel and has no agent path at all, so the connection's
	// resolved user id IS the derivation: signed in ⇒ "human", not ⇒ "anonymous".
	// Fixed for the life of the connection — the cookie was checked at upgrade.
	author := AuthorForSession(uid)
	var client *ws.Client
	client = ws.NewClient(wh.hub, canvasID, conn, func(cid uuid.UUID, raw []byte) {
		wh.handleOp(cid, raw, client.CanWrite(), author)
	})
	client.SetIdentity(uid, canWrite)
	wh.hub.Register(client)

	// Send current state immediately on connect
	go func() {
		c, state, edits, err := wh.store.GetCanvasState(context.Background(), canvasID)
		if err != nil {
			log.Printf("ws initial state: %v", err)
			return
		}
		c.YourRole = role // let the board render read-only when role != write
		data, _ := json.Marshal(stateMsg{Type: "state", Canvas: c, State: state, PendingEdits: edits})
		client.Send(data)
	}()

	go client.WritePump()
	client.ReadPump() // blocks until disconnect
}

func (wh *WSHandler) handleOp(canvasID uuid.UUID, raw []byte, canWrite bool, author string) {
	// Every inbound WS op is a mutation. Read-only viewers are dropped silently —
	// the gate lives here (not just in the UI) so a crafted client can't write.
	if !canWrite {
		return
	}

	// The ops below run on a background context (they outlive the read pump), so
	// the derived author is stamped onto it here — that's what applyOp and the
	// shared ensureDefaultDocument helper read.
	ctx := WithAuthor(context.Background(), author)

	// Peek at the op name (and, for a batch envelope, the sub-op list) before
	// fully decoding — the per-op struct below is filled out by applyOp for
	// each op individually.
	var probe struct {
		Op  string            `json:"op"`
		Ops []json.RawMessage `json:"ops"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return
	}

	if probe.Op == "batch" {
		// Batch envelope: apply every sub-op, then broadcast exactly ONCE, so a
		// multi-op edit costs one full-canvas broadcast instead of one per op.
		wh.applyBatch(ctx, canvasID, probe.Ops)
		broadcastStateBy(ctx, wh.store, wh.hub, canvasID, "user")
		return
	}

	if err := wh.applyOp(ctx, canvasID, raw); err != nil {
		if err != errOpSkipped {
			log.Printf("ws op %s error: %v", probe.Op, err)
		}
		return
	}
	// WS ops come from a browser viewer — attribute to "user" so the agent
	// cursor doesn't fire when a human edits (even an agent-created entity).
	broadcastStateBy(ctx, wh.store, wh.hub, canvasID, "user")
}

// applyBatch executes every sub-op inside a "batch" envelope, one store call
// per sub-op via applyOp. What batching buys here is NOT fewer store round
// trips, it's fewer broadcasts: the caller (handleOp) fires exactly ONE
// broadcastStateBy after this returns, instead of one per sub-op. A
// failing/malformed sub-op is logged and skipped; the rest still apply and we
// still broadcast once for whatever succeeded.
func (wh *WSHandler) applyBatch(ctx context.Context, canvasID uuid.UUID, ops []json.RawMessage) {
	for _, sub := range ops {
		if err := wh.applyOp(ctx, canvasID, sub); err != nil && err != errOpSkipped {
			var subProbe struct {
				Op string `json:"op"`
			}
			_ = json.Unmarshal(sub, &subProbe)
			log.Printf("ws batch sub-op %s error: %v", subProbe.Op, err)
		}
	}
}

// applyOp decodes and executes the mutation for a single op. It never
// broadcasts — the caller (handleOp) does that exactly once, whether this was
// the only op on the message or one of several inside a "batch" envelope.
// Returns errOpSkipped for malformed/no-op input (bad JSON, missing id,
// invalid enum, empty update list — the same cases that used to silently
// abort handleOp before broadcasting), or the underlying store error.
func (wh *WSHandler) applyOp(ctx context.Context, canvasID uuid.UUID, raw []byte) error {
	var msg struct {
		Op          string          `json:"op"`
		ID          *uuid.UUID      `json:"id"`
		EntityID    *uuid.UUID      `json:"entityId"`
		Instruction string          `json:"instruction"`
		Mode        string          `json:"mode"`
		TemplateID  string          `json:"templateId"`
		Partial     json.RawMessage `json:"partial"`
		Data        json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &msg); err != nil {
		return errOpSkipped
	}

	var mutErr error

	switch msg.Op {
	case "mode.set":
		if !isValidMode(msg.Mode) {
			log.Printf("ws op mode.set: invalid mode %q", msg.Mode)
			return errOpSkipped
		}
		_, mutErr = wh.store.SetMode(ctx, canvasID, msg.Mode)

	case "mode.enable":
		if !isEnableableMode(msg.Mode) {
			log.Printf("ws op mode.enable: invalid mode %q", msg.Mode)
			return errOpSkipped
		}
		_, mutErr = wh.store.EnableMode(ctx, canvasID, msg.Mode)

	case "template.apply":
		if !isValidMode(msg.Mode) {
			log.Printf("ws op template.apply: invalid mode %q", msg.Mode)
			return errOpSkipped
		}
		_, mutErr = wh.store.ApplyTemplate(ctx, canvasID, msg.Mode)

	case "note.add":
		var data struct {
			Body       string     `json:"body"`
			ImageRefs  []string   `json:"imageRefs"`
			ParentID   *uuid.UUID `json:"parentId"`
			ParentKind *string    `json:"parentKind"`
		}
		if len(msg.Data) > 0 {
			if err := json.Unmarshal(msg.Data, &data); err != nil {
				log.Printf("ws op note.add: bad data: %v", err)
				return errOpSkipped
			}
		}
		if data.ImageRefs == nil {
			data.ImageRefs = []string{}
		}
		n := &store.Note{
			ID:         uuid.New(),
			Kind:       "note",
			Body:       data.Body,
			ImageRefs:  data.ImageRefs,
			ParentID:   data.ParentID,
			ParentKind: data.ParentKind,
			CreatedBy:  "user",
			AuthoredBy: AuthorFromCtx(ctx), // TDM-40; "human" or "anonymous" here
		}
		// Land the note in the canvas's default notes document (creating one if
		// needed) so it belongs to a tab, matching the agent/REST path.
		docID, derr := ensureDefaultDocument(ctx, wh.store, canvasID, "notes", "user")
		if derr != nil {
			log.Printf("ws op note.add: document: %v", derr)
			return errOpSkipped
		}
		n.DocumentID = &docID
		_, mutErr = wh.store.CreateNote(ctx, canvasID, n)

	case "note.update":
		if msg.ID == nil {
			return errOpSkipped
		}
		var patch store.NotePatch
		if err := json.Unmarshal(msg.Partial, &patch); err != nil {
			return errOpSkipped
		}
		_, mutErr = wh.store.UpdateNote(ctx, canvasID, *msg.ID, patch)

	case "note.delete":
		if msg.ID == nil {
			return errOpSkipped
		}
		_, mutErr = wh.store.DeleteNote(ctx, canvasID, *msg.ID)

	case "roadmap.add":
		var data struct {
			ParentID  *uuid.UUID `json:"parentId"`
			Title     string     `json:"title"`
			Body      string     `json:"body"`
			Status    string     `json:"status"`
			Stage     string     `json:"stage"`
			Assignee  string     `json:"assignee"`
			SortOrder int        `json:"sortOrder"`
		}
		if len(msg.Data) > 0 {
			if err := json.Unmarshal(msg.Data, &data); err != nil {
				log.Printf("ws op roadmap.add: bad data: %v", err)
				return errOpSkipped
			}
		}
		if data.Status == "" {
			data.Status = "todo"
		}
		r := &store.RoadmapItem{
			ID:        uuid.New(),
			Kind:      "roadmap",
			ParentID:  data.ParentID,
			Title:     data.Title,
			Body:      data.Body,
			Status:    data.Status,
			Stage:     data.Stage,
			Assignee:  data.Assignee,
			SortOrder: data.SortOrder,
			CreatedBy: "user",
		}
		docID, derr := ensureDefaultDocument(ctx, wh.store, canvasID, "roadmap", "user")
		if derr != nil {
			log.Printf("ws op roadmap.add: document: %v", derr)
			return errOpSkipped
		}
		r.DocumentID = &docID
		_, mutErr = wh.store.CreateRoadmapItem(ctx, canvasID, r)

	case "roadmap.update":
		if msg.ID == nil {
			return errOpSkipped
		}
		var patch store.RoadmapItemPatch
		if err := json.Unmarshal(msg.Partial, &patch); err != nil {
			return errOpSkipped
		}
		_, mutErr = wh.store.UpdateRoadmapItem(ctx, canvasID, *msg.ID, patch)

	case "roadmap.delete":
		if msg.ID == nil {
			return errOpSkipped
		}
		_, mutErr = wh.store.DeleteRoadmapItem(ctx, canvasID, *msg.ID)

	case "roadmap.reorder":
		var payload struct {
			Updates []store.RoadmapReorder `json:"updates"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			log.Printf("ws op roadmap.reorder: bad payload: %v", err)
			return errOpSkipped
		}
		if len(payload.Updates) == 0 {
			return errOpSkipped
		}
		_, mutErr = wh.store.ReorderRoadmapItems(ctx, canvasID, payload.Updates)

	case "document.add":
		var data struct {
			Type      string         `json:"type"`
			Name      string         `json:"name"`
			Config    map[string]any `json:"config"`
			SortOrder int            `json:"sortOrder"`
			ParentID  *uuid.UUID     `json:"parentId"`
		}
		if len(msg.Data) > 0 {
			if err := json.Unmarshal(msg.Data, &data); err != nil {
				log.Printf("ws op document.add: bad data: %v", err)
				return errOpSkipped
			}
		}
		if !docTypesCreatable[data.Type] {
			log.Printf("ws op document.add: invalid type %q", data.Type)
			return errOpSkipped
		}
		if data.Name == "" {
			data.Name = defaultDocNames[data.Type]
		}
		doc := &store.Document{ID: uuid.New(), Kind: "document", Type: data.Type,
			Name: data.Name, Config: data.Config, ParentID: data.ParentID, SortOrder: data.SortOrder,
			CreatedBy: "user", AuthoredBy: AuthorFromCtx(ctx)} // TDM-40
		_, mutErr = wh.store.CreateDocument(ctx, canvasID, doc)

	case "document.update":
		if msg.ID == nil {
			return errOpSkipped
		}
		var patch store.DocumentPatch
		if err := json.Unmarshal(msg.Partial, &patch); err != nil {
			return errOpSkipped
		}
		_, mutErr = wh.store.UpdateDocument(ctx, canvasID, *msg.ID, patch)

	case "document.delete":
		if msg.ID == nil {
			return errOpSkipped
		}
		_, mutErr = wh.store.DeleteDocument(ctx, canvasID, *msg.ID)

	case "document.reorder":
		var payload struct {
			Updates []store.DocumentReorder `json:"updates"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			log.Printf("ws op document.reorder: bad payload: %v", err)
			return errOpSkipped
		}
		if len(payload.Updates) == 0 {
			return errOpSkipped
		}
		_, mutErr = wh.store.ReorderDocuments(ctx, canvasID, payload.Updates)

	case "scoped_edit_request":
		if msg.EntityID == nil {
			return errOpSkipped
		}
		_, mutErr = wh.store.CreatePendingEdit(ctx, canvasID, *msg.EntityID, msg.Instruction)
	}

	return mutErr
}
