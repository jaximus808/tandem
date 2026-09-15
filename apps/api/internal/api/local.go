package api

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/agentcanvas/api/internal/auth"
	"github.com/agentcanvas/api/internal/store"
)

// Local mode (TANDEM_AUTH=none) — Tandem running on one person's machine.
//
// There is no sign-in, so there is also no session cookie to tell "the human in
// the browser" apart from "an agent on the API". Everything downstream keys on
// that distinction — canvas ownership, the approval gate, provenance (human vs
// agent:<id>), the owner-only routes — and none of it needs to change if the
// browser simply HAS a session. So local mode mints one: when the SPA's
// index.html is served (a navigation only a browser makes; the gateway never
// fetches HTML), a session for the fixed local user is set as the same cookie
// Google sign-in would set. From then on every fetch the page makes carries it
// and the existing cookie path (sessionUserID) resolves the local owner.
//
// Agents are untouched: the MCP gateway talks to /api/mcp/auth and /api/canvas/*
// with bearer tokens and no cookie, so they stay agents. That is the whole
// reason the cookie is minted on the HTML response and not on every request —
// minting it for a cookie-less /api/mcp/auth call would make each agent the
// owner.

// LocalUserID is the one user local mode has. Fixed so the web can compare
// ownerUserId against a constant (apps/web/src/lib/auth.ts LOCAL_USER_ID) and so
// an import from a hosted instance can remap owners onto it.
var LocalUserID = uuid.MustParse("00000000-0000-4000-8000-000000000001")

// LocalUserTemplate is the row EnsureLocalUser upserts at boot. Canvases it
// creates are public + writable so a bare canvas code (what the gateway holds)
// can propose and claim work, exactly like an anonymous canvas on the hosted
// product. GoogleSub is the unique key the users table upserts on.
func LocalUserTemplate() *store.User {
	return &store.User{
		ID:                      LocalUserID,
		GoogleSub:               "local",
		Email:                   "local@tandem",
		DisplayName:             "You",
		DefaultCanvasVisibility: "public",
		DefaultPublicRole:       "write",
		AgentFollowStyle:        "cinematic",
	}
}

// localSessionTTL is deliberately long: the cookie is the local user's
// identity, not a login, and re-minting it on every index.html load keeps it
// alive anyway.
const localSessionTTL = 365 * 24 * time.Hour

// ensureLocalSession sets the local user's session cookie on w unless r already
// carries a valid one. Returns true when a cookie was set.
func ensureLocalSession(authSvc *auth.Service, w http.ResponseWriter, r *http.Request, cookieSecure bool) bool {
	if authSvc == nil {
		return false
	}
	if _, ok := sessionUserID(authSvc, r); ok {
		return false
	}
	token, err := authSvc.IssueSession(LocalUserID, localSessionTTL)
	if err != nil {
		return false
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(localSessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	return true
}
