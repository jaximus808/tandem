package store

import (
	"crypto/rand"
	"time"

	"github.com/google/uuid"
)

// Distinct agent names (TDM-22).
//
// Agent names are unique per canvas (migration 0036) and registering an
// existing name refreshes THAT agent. On its own that rule merges agents that
// merely happen to share a name: every subagent that connects without a `name`
// (the default used to be the bare role, "executor"), or several workers
// spawned under one planner with the same name, collapse into one row on the
// board, which then shows one agent holding everyone's tickets.
//
// The register path therefore distinguishes "this is the agent that already
// holds the name" (refresh, as before) from "this is a different agent that
// wants the same name" (take a distinct name instead). AgentRegisterIntent
// carries what the caller knows; ResolveAgentName is the whole policy, shared by
// the SQLite store and the handler tests' fake so the two cannot drift.

// AgentRegisterIntent is what a registration knows beyond the row itself.
type AgentRegisterIntent struct {
	// NameGiven is false when the caller sent no name. A nameless registration
	// is given a minted "<role>-<4 chars>" name instead of the bare role, unless
	// the caller's session is already a registered agent (then it keeps that
	// agent and its name).
	NameGiven bool
	// SessionAgentID is the agent id the caller's session already holds (the
	// gateway sends it once a session has registered). Self-asserted, like every
	// agent identity on the canvas; it only ever selects the row to refresh.
	SessionAgentID *uuid.UUID
	// SessionIdentity is the caller's asserted agent identity (the
	// X-Tandem-Agent header): a registered name or agent id once the session has
	// registered, an anonymous "session-…" claimant id before. Matching an
	// existing row's name or id means the same session is re-registering.
	SessionIdentity string
	// LiveAfter: an existing agent last seen after this instant is live. A live
	// agent under the same parent holding the requested name is a sibling, not
	// this caller, so the caller gets a suffixed name. Zero = never live. Callers
	// normally leave it zero and the store fills it from its claim TTL: an agent
	// quiet for longer than that has also lost any claim it held, so handing its
	// name back to a newcomer under the same parent cannot strand work.
	LiveAfter time.Time
}

// isSessionAgent reports whether existing is the agent the caller's session
// already is.
func (in AgentRegisterIntent) isSessionAgent(existing *Agent) bool {
	if existing == nil {
		return false
	}
	if in.SessionAgentID != nil && *in.SessionAgentID == existing.ID {
		return true
	}
	return in.SessionIdentity != "" &&
		(in.SessionIdentity == existing.Name || in.SessionIdentity == existing.ID.String())
}

// AgentNameDecision is ResolveAgentName's answer.
type AgentNameDecision int

const (
	// RefreshAgent: upsert on the requested name, refreshing the row that holds
	// it (or inserting it if nobody does). The pre-TDM-22 behaviour.
	RefreshAgent AgentNameDecision = iota
	// DistinctAgent: the name belongs to a different agent (or none was given);
	// register under a fresh, unused name derived from the requested one.
	DistinctAgent
)

// ResolveAgentName decides how a registration of a (a.Name possibly empty)
// treats existing, the row currently holding the requested name (nil if none).
// sessionAgent is the row the caller's session already is, when known (nil
// otherwise). It returns the name to register under and whether to refresh or
// take a distinct name.
//
// Rules, in order:
//  1. No name given: the session's own agent if it has one (keep its name and
//     refresh it); otherwise a distinct name minted from the role.
//  2. The holder of the name IS this session: refresh (a re-register keeps its
//     agent; this is what stops a reconnect spawning a duplicate).
//  3. No parentAgentId: refresh. Plain named registration is unchanged.
//  4. Parent given and the holder has a different parent (or none): distinct.
//  5. Same parent and the holder is live: a sibling, so distinct.
//  6. Same parent and the holder is stale: refresh (it is this worker coming
//     back, or its name is free to reuse).
func ResolveAgentName(a *Agent, in AgentRegisterIntent, existing, sessionAgent *Agent) (string, AgentNameDecision) {
	if !in.NameGiven {
		if sessionAgent != nil {
			return sessionAgent.Name, RefreshAgent
		}
		return a.Role, DistinctAgent
	}
	if existing == nil || in.isSessionAgent(existing) || a.ParentAgentID == nil {
		return a.Name, RefreshAgent
	}
	if existing.ParentAgentID == nil || *existing.ParentAgentID != *a.ParentAgentID {
		return a.Name, DistinctAgent
	}
	if !in.LiveAfter.IsZero() && existing.LastSeenAt.After(in.LiveAfter) {
		return a.Name, DistinctAgent
	}
	return a.Name, RefreshAgent
}

// agentSuffixAlphabet is lowercase alphanumerics minus the easily-confused
// 0/o and 1/l, so a suffix reads back unambiguously off the board.
const agentSuffixAlphabet = "abcdefghijkmnpqrstuvwxyz23456789"

// SuffixedAgentName returns base + "-" + 4 random characters ("executor-k7xq").
func SuffixedAgentName(base string) string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	out := make([]byte, len(b))
	for i, v := range b {
		out[i] = agentSuffixAlphabet[int(v)%len(agentSuffixAlphabet)]
	}
	return base + "-" + string(out)
}
