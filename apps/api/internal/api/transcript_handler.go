package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/agentcanvas/api/internal/store"
	"github.com/agentcanvas/api/internal/transcripts"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Agent transcripts (TDM-18): watch a Tandem agent's Claude Code session live
// from the board.
//
//	GET /api/canvas/agents/{agentId}/transcript?after=<offset>&limit=<n>&tail=<n>&transcript=<id>
//
// A canvas-JWT read (any role), like GET /api/canvas/agents: the canvas comes
// from the token, and {agentId} must be an agent REGISTERED ON THAT CANVAS — a
// token for canvas A cannot read the transcript of an agent that only exists on
// canvas B, even though both files sit in the same ~/.claude.
//
// LOCAL MODE ONLY, BY CONSTRUCTION. The endpoint reads files off the host's
// disk. In local mode the host and the viewer are the same person; on a
// self-hosted TANDEM_AUTH=google server they are not, and a canvas reader must
// never be able to read the operator's Claude Code history. So under
// TANDEM_AUTH=google no index is even built and the route answers 403
// `transcripts_local_only`; TANDEM_TRANSCRIPTS=off answers 404
// `transcripts_disabled`. GET /api/config reports `transcripts` so the web can
// hide the surface rather than probe it.
//
// The answer (200):
//
//	{ "type": "agent.transcript", "agentId": "<uuid>", "found": true,
//	  "transcript": { id, kind, path, project, sessionId, subagentId?, modifiedAt, size, subagent? },
//	  "events": [ { id, offset, kind, ts, text?, tool?, input?, toolUseId?, isError?, truncated?, length?, model? } ],
//	  "next": <offset>, "size": <bytes>, "more": false, "reset": false }
//
// `found:false` (with empty events) means the agent exists but no transcript
// names it — it runs in another client, or its file is older than the scan
// window. That is a normal answer, not a 404, so a polling monitor stays quiet.
type transcriptHandler struct {
	store store.Store
	index *transcripts.Index // nil = disabled or not local
	local bool
}

func (th *transcriptHandler) GetAgentTranscript(w http.ResponseWriter, r *http.Request) {
	if !th.local {
		writeCodedError(w, http.StatusForbidden, "transcripts_local_only",
			"agent transcripts are served only in local mode (TANDEM_AUTH=none): they are files on the server's disk", nil)
		return
	}
	if th.index == nil {
		writeCodedError(w, http.StatusNotFound, "transcripts_disabled",
			"agent transcripts are turned off on this server (TANDEM_TRANSCRIPTS=off)", nil)
		return
	}

	rawID := chi.URLParam(r, "agentId")
	agentID, err := uuid.Parse(rawID)
	if err != nil {
		writeCodedError(w, http.StatusBadRequest, "invalid_agent_id", "agentId must be a registered agent's uuid", map[string]string{"agentId": rawID})
		return
	}
	q := r.URL.Query()
	opts, bad := parseTranscriptQuery(q.Get("after"), q.Get("limit"), q.Get("tail"))
	if bad != "" {
		writeCodedError(w, http.StatusBadRequest, "invalid_query", bad, nil)
		return
	}

	ctx := r.Context()
	canvasID := CanvasIDFromCtx(ctx)
	if _, err := th.store.GetAgent(ctx, canvasID, agentID); err != nil {
		writeCodedError(w, http.StatusNotFound, "agent_not_found", "no agent with that id is registered on this canvas", map[string]string{"agentId": agentID.String()})
		return
	}

	t, err := th.index.Lookup(agentID.String())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not read transcripts: "+err.Error())
		return
	}
	resp := map[string]any{
		"type":    "agent.transcript",
		"agentId": agentID.String(),
		"found":   t != nil,
	}
	if t == nil {
		resp["events"] = []transcripts.Event{}
		resp["next"] = 0
		resp["size"] = 0
		resp["more"] = false
		resp["reset"] = false
		writeJSON(w, http.StatusOK, resp)
		return
	}

	// An offset is only meaningful for the file it came from: when the agent's
	// newest transcript is no longer the one the client was tailing, start over.
	reset := false
	if prev := strings.TrimSpace(q.Get("transcript")); prev != "" && prev != t.ID && opts.Tail == 0 {
		opts.After = 0
		reset = true
	}
	page, err := transcripts.Read(t.Path, opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not read transcript: "+err.Error())
		return
	}
	t.Size = page.Size
	resp["transcript"] = t
	resp["events"] = page.Events
	resp["next"] = page.Next
	resp["size"] = page.Size
	resp["more"] = page.More
	resp["reset"] = page.Reset || reset
	writeJSON(w, http.StatusOK, resp)
}

// parseTranscriptQuery validates after / limit / tail. Returns a message when
// one is malformed.
func parseTranscriptQuery(after, limit, tail string) (transcripts.ReadOptions, string) {
	var o transcripts.ReadOptions
	if after != "" {
		n, err := strconv.ParseInt(after, 10, 64)
		if err != nil || n < 0 {
			return o, "after must be a non-negative byte offset (the previous answer's `next`)"
		}
		o.After = n
	}
	if limit != "" {
		n, err := strconv.Atoi(limit)
		if err != nil || n < 1 || n > transcripts.MaxLimit {
			return o, "limit must be between 1 and " + strconv.Itoa(transcripts.MaxLimit)
		}
		o.Limit = n
	}
	if tail != "" {
		n, err := strconv.Atoi(tail)
		if err != nil || n < 1 || n > transcripts.MaxLimit {
			return o, "tail must be between 1 and " + strconv.Itoa(transcripts.MaxLimit)
		}
		if after != "" {
			return o, "pass either after (resume) or tail (open on the last n events), not both"
		}
		o.Tail = n
	}
	return o, ""
}
