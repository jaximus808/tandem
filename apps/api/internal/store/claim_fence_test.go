package store

import (
	"encoding/json"
	"testing"
	"time"
)

// TDM-98 — the fencing token, at the store layer: minted by the claim itself,
// with the record-carrying semantics the SQLite store exercises end to end in
// store/sqlite (TestClaimRaceHasOneWinner, TestExpiredClaimTakeover).

// ── The record is server-owned ────────────────────────────────────────────────

// The payload-only write path (the content gate) replaces the caller's claim key
// with the stored one, exactly as it does for the audit log: a caller can neither
// forge a generation nor blank the one it will be fenced by.
func TestContentGateCarriesTheClaimRecord(t *testing.T) {
	stored := json.RawMessage(`{"title":"t","body":"b","claim":{"generation":4,"holder":"worker-a"}}`)
	current := &Action{Type: "task", State: "executing", Payload: stored}

	// A bookkeeping write that omits the record entirely (the web editor's shape).
	next, out, err := DecideContentUpdate(current, json.RawMessage(`{"title":"t","body":"b","links":["x"]}`), "human", time.Now().UTC())
	if err != nil {
		t.Fatalf("bookkeeping write: %v", err)
	}
	if out.Reverted {
		t.Fatal("a links-only write must not trip the approval gate")
	}
	if got := ReadClaimRecord(next).Generation; got != 4 {
		t.Fatalf("generation after an omitting write = %d, want 4 (carried, not dropped)", got)
	}

	// A write that tries to LOWER the generation, which is the interesting attack:
	// present a small number and every stale token becomes valid again.
	next, _, err = DecideContentUpdate(current,
		json.RawMessage(`{"title":"t","body":"b","claim":{"generation":1,"holder":"worker-b"}}`), "agent:worker-b", time.Now().UTC())
	if err != nil {
		t.Fatalf("forging write: %v", err)
	}
	rec := ReadClaimRecord(next)
	if rec.Generation != 4 || rec.Holder != "worker-a" {
		t.Fatalf("record after a forging write = %+v, want the stored generation 4 / worker-a", rec)
	}

	// A task that has never been claimed must not grow an empty record.
	next, _, err = DecideContentUpdate(&Action{Type: "task", State: "proposed", Payload: json.RawMessage(`{"title":"t"}`)},
		json.RawMessage(`{"title":"t","claim":{"generation":9}}`), "agent:x", time.Now().UTC())
	if err != nil {
		t.Fatalf("unclaimed write: %v", err)
	}
	var p map[string]any
	if err := json.Unmarshal(next, &p); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if _, present := p[ClaimRecordKey]; present {
		t.Fatalf("an unclaimed task grew a claim record: %s", next)
	}
}

// CarryClaimRecord is the same rule for the OTHER payload door — the one a state
// transition writes raw.
func TestCarryClaimRecord(t *testing.T) {
	stored := json.RawMessage(`{"title":"t","claim":{"generation":7,"holder":"worker-a"}}`)

	got := CarryClaimRecord(json.RawMessage(`{"title":"t","claim":{"generation":1}}`), stored)
	if rec := ReadClaimRecord(got); rec.Generation != 7 || rec.Holder != "worker-a" {
		t.Fatalf("record = %+v, want generation 7 / worker-a", rec)
	}
	got = CarryClaimRecord(json.RawMessage(`{"title":"t"}`), stored)
	if rec := ReadClaimRecord(got); rec.Generation != 7 {
		t.Fatalf("omitted record was not carried: %s", got)
	}
	// No stored record → the caller's is dropped, not kept.
	got = CarryClaimRecord(json.RawMessage(`{"title":"t","claim":{"generation":5}}`), json.RawMessage(`{"title":"t"}`))
	if rec := ReadClaimRecord(got); rec.Generation != 0 {
		t.Fatalf("a caller's record survived onto an unclaimed task: %s", got)
	}
	// Nothing to do → the payload is returned untouched (byte-identical), so this
	// never churns a payload it has no reason to rewrite.
	in := json.RawMessage(`{"title":"t"}`)
	if got := CarryClaimRecord(in, json.RawMessage(`{"title":"t"}`)); string(got) != string(in) {
		t.Fatalf("payload was rewritten with nothing to change: %s", got)
	}
	// Unparsable input is passed through rather than mangled.
	bad := json.RawMessage(`not json`)
	if got := CarryClaimRecord(bad, stored); string(got) != string(bad) {
		t.Fatalf("unparsable payload = %s, want it untouched", got)
	}
}

// ReadClaimRecord must never fail a write: everything odd reads as "no token",
// which degrades to the holder-identity check.
func TestReadClaimRecordTolerance(t *testing.T) {
	for name, raw := range map[string]string{
		"empty":            ``,
		"not an object":    `[1,2]`,
		"no claim key":     `{"title":"t"}`,
		"claim not object": `{"claim":"nope"}`,
		"negative":         `{"claim":{"generation":-2}}`,
	} {
		t.Run(name, func(t *testing.T) {
			if got := ReadClaimRecord(json.RawMessage(raw)).Generation; got != 0 {
				t.Fatalf("generation = %d, want 0", got)
			}
		})
	}
	if got := ReadClaimRecord(json.RawMessage(`{"claim":{"generation":12,"holder":"w"}}`)); got.Generation != 12 || got.Holder != "w" {
		t.Fatalf("record = %+v, want generation 12 / w", got)
	}
}
