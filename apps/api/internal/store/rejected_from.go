package store

import "encoding/json"

// RejectedFrom reads, off a stored payload's audit log, the state the task was
// in when it was most recently rejected (TDM-14): "proposed" for a proposal the
// gate turned down, "approved" for ready work a human pulled back out of the
// queue. Empty when the trail carries no rejection entry — a row rejected before
// rejections were audited, or one whose entry fell off the capped log.
//
// This is what lets undoing a rejection put the task back WHERE IT WAS rather
// than always into 'proposed'. Callers must treat "" as "proposed": an unknown
// origin resolves into the gate, never past it.
func RejectedFrom(payload json.RawMessage) string {
	log := storedAudit(payload)
	for i := len(log) - 1; i >= 0; i-- {
		e := log[i]
		if len(e.Change) != 1 || e.Change[0] != StateChange {
			continue // a content edit, not a move
		}
		// The NEWEST state move decides, and it must be the rejection itself. If
		// the latest move is anything else, the entry for the rejection that put
		// the task where it is now was lost, and an OLDER rejection's origin says
		// nothing about this one — fall back to "unknown" rather than trusting it.
		if e.ToState == "rejected" {
			return e.FromState
		}
		return ""
	}
	return ""
}
