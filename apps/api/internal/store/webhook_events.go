package store

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// Webhook secrets and the event vocabulary, shared by every Store
// implementation and the API/webhooks packages.

// WebhookSecretPrefix namespaces webhook signing secrets. It exists for the same
// reason PATPrefix does: a secret that turns up in a log or a support ticket is
// immediately identifiable as what it is (and as what it is NOT — a PAT, a claim
// token, a canvas code). Receivers see it verbatim as their HMAC key.
const WebhookSecretPrefix = "whsec_"

// GenerateWebhookSecret mints a webhook signing key: WebhookSecretPrefix + 32
// random bytes (hex) = 256 bits, the same budget as a PAT and far more than
// HMAC-SHA256 needs. crypto/rand, obviously — this key is the only thing
// standing between a receiver and a forged task.completed.
//
// The server ALWAYS generates it. There is no path for a client to supply one:
// a caller-chosen secret is a caller-chosen weak secret, and it would also mean
// accepting key material on a request body that gets logged by proxies.
func GenerateWebhookSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Errorf("crypto/rand: %w", err))
	}
	return WebhookSecretPrefix + hex.EncodeToString(b)
}

// KnownWebhookEvents is the event vocabulary a webhook may subscribe to. The
// task lifecycle events the API emits (see api/task_events.go) must stay in
// step with this list — an emit of an unknown event reaches no subscriber.
var KnownWebhookEvents = []string{
	"task.approved", "task.completed", "task.claim_expired", "task.returned", "task.rejected",
}

// IsKnownWebhookEvent reports whether e is one of KnownWebhookEvents.
func IsKnownWebhookEvent(e string) bool {
	for _, k := range KnownWebhookEvents {
		if k == e {
			return true
		}
	}
	return false
}

// NormalizeWebhookEvents de-duplicates and sorts an event filter — the
// app-enforced half of the events contract (migration 0038 leaves dedupe/sort to
// the app and only CHECKs membership + non-emptiness). Unknown names are
// returned as an error rather than silently dropped: a typo'd event that is
// quietly discarded produces a webhook that never fires, the worst failure mode
// for this feature.
func NormalizeWebhookEvents(events []string) ([]string, error) {
	seen := map[string]bool{}
	out := make([]string, 0, len(events))
	for _, e := range events {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		if !IsKnownWebhookEvent(e) {
			return nil, fmt.Errorf("unknown webhook event %q (known: %s)", e, strings.Join(KnownWebhookEvents, ", "))
		}
		if seen[e] {
			continue
		}
		seen[e] = true
		out = append(out, e)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("webhook must subscribe to at least one event (known: %s)", strings.Join(KnownWebhookEvents, ", "))
	}
	sort.Strings(out)
	return out, nil
}

// ── DB row types ────────────────────────────────────────────────────────────
