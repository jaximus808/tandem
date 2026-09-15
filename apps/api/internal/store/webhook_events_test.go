package store

import "testing"

// The event filter is normalized app-side (0038 leaves dedupe/sort to the app)
// and typo'd names are rejected rather than silently dropped — a webhook that
// never fires is the worst failure mode here.
func TestNormalizeWebhookEvents(t *testing.T) {
	got, err := NormalizeWebhookEvents([]string{"task.completed", "task.approved", "task.completed", " "})
	if err != nil {
		t.Fatalf("NormalizeWebhookEvents: %v", err)
	}
	want := []string{"task.approved", "task.completed"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v (deduped and sorted)", got, want)
		}
	}

	if _, err := NormalizeWebhookEvents([]string{"task.aproved"}); err == nil {
		t.Error("a typo'd event name was accepted")
	}
	if _, err := NormalizeWebhookEvents(nil); err == nil {
		t.Error("an empty event filter was accepted — such a webhook can never fire")
	}
}
