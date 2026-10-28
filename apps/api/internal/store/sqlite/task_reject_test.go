package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agentcanvas/api/internal/store"
)

// TDM-14: the conditional reject, against the real schema.
func TestTDM14RejectTaskFromApproved(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	c := newCanvas(t, s)
	task := newTask(t, s, c.ID, "approved", `{"title":"ready work"}`)
	approvedBy := "human"
	if _, err := s.UpdateActionState(ctx, c.ID, task.ID, store.ActionStatePatch{State: "approved", ApprovedBy: &approvedBy}); err != nil {
		t.Fatal(err)
	}

	reason := "not this sprint"
	entry := store.NewStateAudit("human", "approved", "rejected", reason, time.Now())
	got, v, err := s.RejectTask(ctx, c.ID, task.ID, "approved", &reason, entry)
	if err != nil || got.State != "rejected" || v == 0 {
		t.Fatalf("reject approved: %+v v=%d %v", got, v, err)
	}
	if got.Error == nil || *got.Error != reason {
		t.Fatalf("reason not stored in error: %v", got.Error)
	}
	if got.ApprovedBy == nil || *got.ApprovedBy != "human" {
		t.Fatalf("approved_by must survive the reject so undo can restore it: %v", got.ApprovedBy)
	}
	if from := store.RejectedFrom(got.Payload); from != "approved" {
		t.Fatalf("RejectedFrom = %q, want approved (audit entry must ride the write)", from)
	}

	// Idempotent retry: version 0, no error.
	if _, v, err := s.RejectTask(ctx, c.ID, task.ID, "approved", &reason, entry); err != nil || v != 0 {
		t.Fatalf("retry must be idempotent: v=%d %v", v, err)
	}

	// The undo: rejected → approved via the existing rewind keeps approved_by.
	back, _, err := s.ReopenAction(ctx, c.ID, task.ID, "rejected", "approved")
	if err != nil || back.State != "approved" || back.ApprovedBy == nil || back.Error != nil {
		t.Fatalf("restore: %+v %v", back, err)
	}
}

func TestTDM14RejectTaskRefusesClaimed(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	c := newCanvas(t, s)
	task := newTask(t, s, c.ID, "approved", `{"title":"ready work"}`)
	if _, _, err := s.ClaimAction(ctx, c.ID, task.ID, "worker-1"); err != nil {
		t.Fatal(err)
	}
	entry := store.NewStateAudit("human", "approved", "rejected", "", time.Now())
	// The caller believed it was approved; the claim won the race.
	if _, _, err := s.RejectTask(ctx, c.ID, task.ID, "approved", nil, entry); !errors.Is(err, store.ErrIllegalActionState) {
		t.Fatalf("rejecting a claimed task: %v, want ErrIllegalActionState", err)
	}
	now, _ := s.GetAction(ctx, c.ID, task.ID)
	if now.State != "executing" || now.ClaimedBy == nil || *now.ClaimedBy != "worker-1" {
		t.Fatalf("claimed task was disturbed: %+v", now)
	}
}
