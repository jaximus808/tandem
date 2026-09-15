package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/agentcanvas/api/internal/store"
)

func openTest(t *testing.T, opts ...Option) *Store {
	t.Helper()
	s, err := Open(":memory:", opts...)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(s.Close)
	return s
}

func newCanvas(t *testing.T, s *Store) *store.Canvas {
	t.Helper()
	c, err := s.CreateCanvas(context.Background(), "test", nil, "", "")
	if err != nil {
		t.Fatalf("CreateCanvas: %v", err)
	}
	return c
}

func newTask(t *testing.T, s *Store, canvasID uuid.UUID, state string, payload string) *store.Action {
	t.Helper()
	ctx := context.Background()
	first, err := s.ReserveTaskTickets(ctx, canvasID, 1)
	if err != nil {
		t.Fatalf("ReserveTaskTickets: %v", err)
	}
	a := &store.Action{Type: "task", State: state, Payload: json.RawMessage(payload), ProposedBy: "tester", Ticket: &first}
	if _, err := s.CreateAction(ctx, canvasID, a); err != nil {
		t.Fatalf("CreateAction: %v", err)
	}
	return a
}

func TestMigrateIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tandem.db")
	s1, err := Open(path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	c := newCanvas(t, s1)
	s1.Close()

	s2, err := Open(path)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	defer s2.Close()
	got, err := s2.GetCanvasByCode(context.Background(), c.Code)
	if err != nil {
		t.Fatalf("canvas did not survive reopen: %v", err)
	}
	if got.ID != c.ID {
		t.Fatalf("id mismatch after reopen: %s vs %s", got.ID, c.ID)
	}
	var v int
	if err := s2.DB().QueryRow(`SELECT COUNT(*) FROM schema_version`).Scan(&v); err != nil || v != 1 {
		t.Fatalf("schema_version rows = %d (err %v), want 1", v, err)
	}
}

func TestCanvasCreateGetStateBump(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	c := newCanvas(t, s)
	if len(c.Code) != 8 || c.ClaimToken == "" || c.Version != 0 || c.Mode != "welcome" {
		t.Fatalf("unexpected fresh canvas: %+v", c)
	}
	byCode, err := s.GetCanvasByCode(ctx, c.Code)
	if err != nil {
		t.Fatal(err)
	}
	if byCode.ClaimToken != "" {
		t.Fatal("claim token leaked on a read path")
	}
	if byCode.ApprovalPolicy != "epic" || byCode.Visibility != "public" || byCode.PublicRole != "write" {
		t.Fatalf("defaults wrong: %+v", byCode)
	}
	v, err := s.BumpCanvasVersion(ctx, c.ID)
	if err != nil || v != 1 {
		t.Fatalf("bump = %d, %v", v, err)
	}
	if _, err := s.BumpCanvasVersion(ctx, uuid.New()); err == nil {
		t.Fatal("bumping a missing canvas must fail")
	}
	canvas, state, edits, err := s.GetCanvasState(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if canvas.Version != 1 || state.Version != 1 || len(edits) != 0 {
		t.Fatalf("state = v%d edits=%d", state.Version, len(edits))
	}
	for name, m := range map[string]int{"documents": len(state.Documents), "notes": len(state.Notes), "roadmapItems": len(state.RoadmapItems), "actions": len(state.Actions), "agents": len(state.Agents)} {
		if m != 0 {
			t.Fatalf("%s not empty", name)
		}
	}
	if state.Documents == nil || state.Actions == nil {
		t.Fatal("requested kinds must be empty maps, not nil")
	}
	// Kinds projection: unrequested kinds are nil.
	_, partial, _, err := s.GetCanvasKinds(ctx, c.ID, []string{"actions"})
	if err != nil {
		t.Fatal(err)
	}
	if partial.Documents != nil || partial.Actions == nil {
		t.Fatal("GetCanvasKinds projection wrong")
	}
	if _, err := s.EnableMode(ctx, c.ID, "docs"); err != nil {
		t.Fatal(err)
	}
	again, err := s.EnableMode(ctx, c.ID, "docs")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetCanvasByID(ctx, c.ID)
	if len(got.EnabledModes) != 1 || got.Version != 2 || again != 2 {
		t.Fatalf("EnableMode not idempotent: modes=%v v=%d again=%d", got.EnabledModes, got.Version, again)
	}
	if err := s.DeleteCanvas(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetCanvasByID(ctx, c.ID); err == nil {
		t.Fatal("canvas survived delete")
	}
}

func TestClaimCanvasAndOwnership(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	u, err := s.UpsertUserByGoogleSub(ctx, &store.User{GoogleSub: "g1", Email: "A@Example.com", DisplayName: "A"})
	if err != nil {
		t.Fatal(err)
	}
	c := newCanvas(t, s)
	if _, err := s.ClaimCanvas(ctx, c.Code, "clm_wrong", u.ID); !errors.Is(err, store.ErrInvalidClaimToken) {
		t.Fatalf("wrong token: %v", err)
	}
	claimed, err := s.ClaimCanvas(ctx, c.Code, c.ClaimToken, u.ID)
	if err != nil || claimed.OwnerUserID == nil || *claimed.OwnerUserID != u.ID {
		t.Fatalf("claim: %v %+v", err, claimed)
	}
	if _, err := s.ClaimCanvas(ctx, c.Code, c.ClaimToken, u.ID); !errors.Is(err, store.ErrAlreadyClaimed) {
		t.Fatalf("second claim: %v", err)
	}
	if _, err := s.ClaimCanvas(ctx, "NOPE1234", "x", u.ID); !errors.Is(err, store.ErrCanvasNotFound) {
		t.Fatalf("missing: %v", err)
	}
	owned, err := s.ListCanvasesByOwner(ctx, u.ID)
	if err != nil || len(owned) != 1 {
		t.Fatalf("owned = %d, %v", len(owned), err)
	}
	role, _ := s.ResolveCanvasRole(ctx, claimed, &u.ID)
	if role != "write" {
		t.Fatalf("owner role = %q", role)
	}
	anon, _ := s.ResolveCanvasRole(ctx, claimed, nil)
	if anon != "write" {
		t.Fatalf("public role = %q", anon)
	}
	if _, err := s.SetCanvasVisibility(ctx, c.ID, "private", "read"); err != nil {
		t.Fatal(err)
	}
	priv, _ := s.GetCanvasByID(ctx, c.ID)
	if r, _ := s.ResolveCanvasRole(ctx, priv, nil); r != "none" {
		t.Fatalf("private anon role = %q", r)
	}
	other, _ := s.UpsertUserByGoogleSub(ctx, &store.User{GoogleSub: "g2", Email: "b@example.com"})
	if err := s.UpsertCanvasAccess(ctx, c.ID, other.ID, "read"); err != nil {
		t.Fatal(err)
	}
	if r, _ := s.ResolveCanvasRole(ctx, priv, &other.ID); r != "read" {
		t.Fatalf("shared role = %q", r)
	}
	shared, _ := s.ListCanvasesSharedWithUser(ctx, other.ID)
	if len(shared) != 1 || shared[0].YourRole != "read" {
		t.Fatalf("shared list: %+v", shared)
	}
	if got, err := s.GetUserByEmail(ctx, "a@example.com"); err != nil || got.ID != u.ID {
		t.Fatalf("email lookup case-insensitive: %v", err)
	}
	n := store.NewNotification(other.ID, "canvas_shared", &c.ID, &u.ID, "read")
	if err := s.CreateNotification(ctx, n); err != nil {
		t.Fatal(err)
	}
	list, _ := s.ListNotifications(ctx, other.ID, 10)
	if len(list) != 1 || list[0].CanvasCode != c.Code || list[0].ActorName != "A" || list[0].Read {
		t.Fatalf("notifications: %+v", list)
	}
	if cnt, _ := s.CountUnreadNotifications(ctx, other.ID); cnt != 1 {
		t.Fatalf("unread = %d", cnt)
	}
	_ = s.MarkNotificationsRead(ctx, other.ID)
	if cnt, _ := s.CountUnreadNotifications(ctx, other.ID); cnt != 0 {
		t.Fatalf("unread after mark = %d", cnt)
	}
	if err := s.DeleteUserAccount(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetCanvasByID(ctx, c.ID); err == nil {
		t.Fatal("owned canvas must go with the account")
	}
}

func TestDocumentsAndNotesRoundTrip(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	c := newCanvas(t, s)
	human := "human"
	doc := &store.Document{Type: "notes", Name: "Thesis", AuthoredBy: &human, Config: map[string]any{"k": "v"}}
	if _, err := s.CreateDocument(ctx, c.ID, doc); err != nil {
		t.Fatal(err)
	}
	n1 := &store.Note{DocumentID: &doc.ID, Body: "first", CreatedBy: "user"}
	n2 := &store.Note{DocumentID: &doc.ID, Body: "second"}
	if _, err := s.CreateNotes(ctx, c.ID, []*store.Note{n1, n2}); err != nil {
		t.Fatal(err)
	}
	n3 := &store.Note{DocumentID: &doc.ID, Body: "third"}
	if _, err := s.CreateNote(ctx, c.ID, n3); err != nil {
		t.Fatal(err)
	}
	notes, err := s.ListNotesByDocument(ctx, c.ID, doc.ID)
	if err != nil || len(notes) != 3 {
		t.Fatalf("notes = %d, %v", len(notes), err)
	}
	for i, n := range notes {
		if n.SortOrder != i {
			t.Fatalf("sort order not appended: %+v", n)
		}
	}
	if notes[0].ImageRefs == nil {
		t.Fatal("ImageRefs must be non-nil")
	}
	canvas, _ := s.GetCanvasByID(ctx, c.ID)
	if canvas.Mode != "docs" {
		t.Fatalf("first note must leave welcome: mode=%s", canvas.Mode)
	}
	body := "edited"
	verified := time.Now().UTC().Truncate(time.Second)
	stale := 3600
	affected, err := s.UpdateNote(ctx, c.ID, n1.ID, store.NotePatch{Body: &body, FreshnessPatch: store.FreshnessPatch{VerifiedAt: &verified, StaleAfterSeconds: &stale}})
	if err != nil || affected != 1 {
		t.Fatalf("update note: %d %v", affected, err)
	}
	if affected, _ := s.UpdateNote(ctx, uuid.New(), n1.ID, store.NotePatch{Body: &body}); affected != 0 {
		t.Fatal("cross-canvas update must affect 0 rows")
	}
	gotDoc, err := s.GetDocument(ctx, c.ID, doc.ID)
	if err != nil || gotDoc.Config["k"] != "v" || gotDoc.AuthoredBy == nil {
		t.Fatalf("doc round trip: %+v %v", gotDoc, err)
	}
	if _, err := s.SetCanvasBriefingDoc(ctx, c.ID, &doc.ID); err != nil {
		t.Fatal(err)
	}
	links, err := s.GetLinkedEntities(ctx, c.ID, []uuid.UUID{n1.ID, uuid.New()})
	if err != nil || len(links) != 1 || links[0].Body != "edited" || links[0].VerifiedAt == nil || *links[0].StaleAfterSeconds != 3600 {
		t.Fatalf("linked: %+v %v", links, err)
	}
	item := &store.RoadmapItem{Title: "Goal", Assignee: "agent", Stage: "Now"}
	if _, err := s.CreateRoadmapItem(ctx, c.ID, item); err != nil {
		t.Fatal(err)
	}
	child := &store.RoadmapItem{Title: "Sub", ParentID: &item.ID}
	if _, err := s.CreateRoadmapItem(ctx, c.ID, child); err != nil {
		t.Fatal(err)
	}
	agentItems, _ := s.ListRoadmapItems(ctx, c.ID, "agent")
	if len(agentItems) != 1 || agentItems[0].Stage != "Now" {
		t.Fatalf("agent items: %+v", agentItems)
	}
	_, sum, _, err := s.GetCanvasSummary(ctx, c.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Counts["notes"] != 3 || sum.Counts["roadmapItems"] != 2 || sum.Counts["documents"] != 1 || len(sum.Sample.Notes) != 1 {
		t.Fatalf("summary: %+v sample notes=%d", sum.Counts, len(sum.Sample.Notes))
	}
	// Copy keeps tabs, notes, hierarchy and the briefing designation under new ids.
	u, _ := s.UpsertUserByGoogleSub(ctx, &store.User{GoogleSub: "g", Email: "u@x"})
	cp, err := s.CopyCanvas(ctx, c.ID, u.ID, "copy")
	if err != nil {
		t.Fatal(err)
	}
	if cp.ID == c.ID || cp.Code == c.Code || cp.OwnerUserID == nil || cp.BriefingDocID == nil || *cp.BriefingDocID == doc.ID {
		t.Fatalf("copy: %+v", cp)
	}
	_, cst, _, _ := s.GetCanvasState(ctx, cp.ID)
	if len(cst.Documents) != 1 || len(cst.Notes) != 3 || len(cst.RoadmapItems) != 2 || len(cst.Actions) != 0 {
		t.Fatalf("copy state: docs=%d notes=%d items=%d", len(cst.Documents), len(cst.Notes), len(cst.RoadmapItems))
	}
	for _, r := range cst.RoadmapItems {
		if r.ParentID != nil {
			if _, ok := cst.RoadmapItems[r.ParentID.String()]; !ok {
				t.Fatal("copied roadmap parent not remapped")
			}
		}
	}
	// Deleting the document cascades its notes.
	if n, err := s.DeleteDocument(ctx, c.ID, doc.ID); err != nil || n != 1 {
		t.Fatalf("delete doc: %d %v", n, err)
	}
	if notes, _ := s.ListNotesByDocument(ctx, c.ID, doc.ID); len(notes) != 0 {
		t.Fatal("notes survived document delete")
	}
	if cv, _ := s.GetCanvasByID(ctx, c.ID); cv.BriefingDocID != nil {
		t.Fatal("briefing_doc_id must clear when the document goes")
	}
}

func TestTicketsAreSequential(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	c := newCanvas(t, s)
	a := newTask(t, s, c.ID, "proposed", `{"title":"one"}`)
	b := newTask(t, s, c.ID, "proposed", `{"title":"two"}`)
	if a.TicketID() != "TDM-1" || b.TicketID() != "TDM-2" {
		t.Fatalf("tickets: %s %s", a.TicketID(), b.TicketID())
	}
	first, err := s.ReserveTaskTickets(ctx, c.ID, 5)
	if err != nil || first != 3 {
		t.Fatalf("reserve 5: %d %v", first, err)
	}
	if _, err := s.ReserveTaskTickets(ctx, c.ID, 0); err == nil {
		t.Fatal("n=0 must be refused")
	}
	if _, err := s.ReserveTaskTickets(ctx, uuid.New(), 1); err == nil {
		t.Fatal("missing canvas must be refused")
	}
	got, err := s.GetActionByTicket(ctx, c.ID, 2)
	if err != nil || got.ID != b.ID {
		t.Fatalf("by ticket: %v", err)
	}
	if _, err := s.GetActionByTicket(ctx, c.ID, 99); !errors.Is(err, store.ErrActionNotFound) {
		t.Fatalf("missing ticket: %v", err)
	}
	all, _ := s.ListActions(ctx, c.ID, "", "task", "agent")
	if len(all) != 2 || all[0].ID != a.ID {
		t.Fatalf("list order/filter: %d", len(all))
	}
	humanOnly, _ := s.ListActions(ctx, c.ID, "", "", "human")
	if len(humanOnly) != 0 {
		t.Fatal("assignee filter leaked")
	}
}

func TestClaimRaceHasOneWinner(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	c := newCanvas(t, s)
	task := newTask(t, s, c.ID, "approved", `{"title":"race"}`)
	const racers = 8
	var wg sync.WaitGroup
	var mu sync.Mutex
	wins, losses := 0, 0
	for i := range racers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, out, err := s.ClaimAction(ctx, c.ID, task.ID, "worker-"+string(rune('a'+i)))
			mu.Lock()
			defer mu.Unlock()
			var already *store.AlreadyClaimedError
			switch {
			case err == nil:
				wins++
				if out.ClaimGeneration != 1 || out.ExpiredClaimBy != "" {
					t.Errorf("winner outcome: %+v", out)
				}
			case errors.As(err, &already):
				losses++
				if already.ClaimedBy == "" {
					t.Errorf("loser not told the holder")
				}
			default:
				t.Errorf("unexpected: %v", err)
			}
		}(i)
	}
	wg.Wait()
	if wins != 1 || losses != racers-1 {
		t.Fatalf("wins=%d losses=%d", wins, losses)
	}
	got, _ := s.GetAction(ctx, c.ID, task.ID)
	if got.State != "executing" || store.ReadClaimRecord(got.Payload).Generation != 1 {
		t.Fatalf("post-race row: %s gen=%d", got.State, store.ReadClaimRecord(got.Payload).Generation)
	}
	// Idempotent self-retry: same generation, no error.
	_, out, err := s.ClaimAction(ctx, c.ID, task.ID, *got.ClaimedBy)
	if err != nil || out.ClaimGeneration != 1 {
		t.Fatalf("self retry: %+v %v", out, err)
	}
	// Non-task refuses by type; wrong state refuses by state.
	epic := &store.Action{Type: "epic", State: "approved", Payload: json.RawMessage(`{}`)}
	_, _ = s.CreateAction(ctx, c.ID, epic)
	if _, _, err := s.ClaimAction(ctx, c.ID, epic.ID, "x"); !errors.Is(err, store.ErrIllegalActionState) {
		t.Fatalf("epic claim: %v", err)
	}
	proposed := newTask(t, s, c.ID, "proposed", `{}`)
	if _, _, err := s.ClaimAction(ctx, c.ID, proposed.ID, "x"); !errors.Is(err, store.ErrIllegalActionState) {
		t.Fatalf("proposed claim: %v", err)
	}
	if _, _, err := s.ClaimAction(ctx, c.ID, uuid.New(), "x"); !errors.Is(err, store.ErrActionNotFound) {
		t.Fatalf("missing claim: %v", err)
	}
}

func TestExpiredClaimTakeover(t *testing.T) {
	s := openTest(t, WithClaimTTL(50*time.Millisecond))
	ctx := context.Background()
	c := newCanvas(t, s)
	task := newTask(t, s, c.ID, "approved", `{"title":"slow"}`)
	if _, _, err := s.ClaimAction(ctx, c.ID, task.ID, "dead-worker"); err != nil {
		t.Fatal(err)
	}
	// Live claim: a rival loses.
	if _, _, err := s.ClaimAction(ctx, c.ID, task.ID, "rival"); err == nil {
		t.Fatal("rival must lose a live claim")
	}
	// Heartbeat extends the lease; a non-holder cannot.
	if _, ok, _ := s.TouchActionClaim(ctx, c.ID, task.ID, "rival"); ok {
		t.Fatal("non-holder extended the lease")
	}
	if _, ok, _ := s.TouchActionClaim(ctx, c.ID, task.ID, "dead-worker"); !ok {
		t.Fatal("holder could not heartbeat")
	}
	time.Sleep(80 * time.Millisecond)
	taken, out, err := s.ClaimAction(ctx, c.ID, task.ID, "rival")
	if err != nil {
		t.Fatalf("takeover: %v", err)
	}
	if out.ExpiredClaimBy != "dead-worker" || out.ExpiredClaimAt.IsZero() || out.ClaimGeneration != 2 || *taken.ClaimedBy != "rival" {
		t.Fatalf("takeover outcome: %+v claimedBy=%v", out, *taken.ClaimedBy)
	}
	// The old holder's lapsed lease is now a lost race, naming the new holder.
	time.Sleep(80 * time.Millisecond)
	_, out, err = s.ClaimAction(ctx, c.ID, task.ID, "rival")
	if err != nil || out.ClaimGeneration != 3 || out.ExpiredClaimBy != "" {
		t.Fatalf("self-takeover must mint a new generation with no handoff: %+v %v", out, err)
	}
	// Release → back on the queue, then requeue/reopen paths.
	released, v, err := s.ReleaseAction(ctx, c.ID, task.ID)
	if err != nil || released.State != "approved" || released.ClaimedBy != nil || v == 0 {
		t.Fatalf("release: %+v %d %v", released, v, err)
	}
	if _, v, err := s.ReleaseAction(ctx, c.ID, task.ID); err != nil || v != 0 {
		t.Fatalf("release must be idempotent: %d %v", v, err)
	}
	// The next claim keeps counting generations up (a lease instance, not an agent).
	_, out, _ = s.ClaimAction(ctx, c.ID, task.ID, "third")
	if out.ClaimGeneration != 4 {
		t.Fatalf("generation after release = %d", out.ClaimGeneration)
	}
}

func TestContentGateAndAudit(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	c := newCanvas(t, s)
	task := newTask(t, s, c.ID, "approved", `{"title":"original","body":"b"}`)
	// Bookkeeping write on an approved task: no gate, no revert.
	out, err := s.UpdateActionPayload(ctx, c.ID, task.ID, json.RawMessage(`{"title":"original","body":"b","progress":["step 1"]}`), "agent:x")
	if err != nil || out.Reverted || out.Version == 0 {
		t.Fatalf("bookkeeping: %+v %v", out, err)
	}
	// Content edit on an approved task costs the approval.
	out, err = s.UpdateActionPayload(ctx, c.ID, task.ID, json.RawMessage(`{"title":"changed","body":"b"}`), "agent:x")
	if err != nil || !out.Reverted || out.Action == nil || out.Action.State != "proposed" {
		t.Fatalf("revert: %+v %v", out, err)
	}
	var p struct {
		Audit []store.ContentAudit `json:"audit"`
	}
	_ = json.Unmarshal(out.Action.Payload, &p)
	if len(p.Audit) != 1 || !p.Audit[0].Reverted || p.Audit[0].Actor != "agent:x" {
		t.Fatalf("audit: %+v", p.Audit)
	}
	// Done tasks are history.
	done := "done"
	if _, err := s.UpdateActionState(ctx, c.ID, task.ID, store.ActionStatePatch{State: done}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateActionPayload(ctx, c.ID, task.ID, json.RawMessage(`{"title":"rewrite history"}`), "agent:x"); !errors.Is(err, store.ErrContentLocked) {
		t.Fatalf("done edit: %v", err)
	}
	// Reopen done → approved clears result/error; reject → resubmit round trip.
	reopened, _, err := s.ReopenAction(ctx, c.ID, task.ID, "done", "approved")
	if err != nil || reopened.State != "approved" {
		t.Fatalf("reopen: %v", err)
	}
	reason := "not like this"
	if _, err := s.UpdateActionState(ctx, c.ID, task.ID, store.ActionStatePatch{State: "rejected", Error: &reason}); err != nil {
		t.Fatal(err)
	}
	entry := store.NewStateAudit("agent:x", "rejected", "proposed", "fixed it", time.Now())
	re, _, err := s.ResubmitAction(ctx, c.ID, task.ID, json.RawMessage(`{"title":"take two","body":"b"}`), entry)
	if err != nil || re.State != "proposed" || re.Error != nil || re.ApprovedBy != nil {
		t.Fatalf("resubmit: %+v %v", re, err)
	}
	_ = json.Unmarshal(re.Payload, &p)
	if len(p.Audit) != 2 || p.Audit[1].Note != "fixed it" {
		t.Fatalf("resubmit audit: %+v", p.Audit)
	}
	if _, _, err := s.ResubmitAction(ctx, c.ID, task.ID, nil, entry); !errors.Is(err, store.ErrIllegalActionState) {
		t.Fatalf("double resubmit: %v", err)
	}
	// AppendActionAudit + contention trail ride the payload without a bump.
	before, _ := s.GetCanvasByID(ctx, c.ID)
	if _, err := s.AppendActionAudit(ctx, c.ID, task.ID, store.NewStateAudit("human", "proposed", "approved", "", time.Now())); err != nil {
		t.Fatal(err)
	}
	fresh, err := s.AppendContentionEvent(ctx, c.ID, task.ID, store.NewLostClaim("loser", "winner", 0, time.Now()))
	if err != nil || len(store.ReadContention(fresh.Payload)) != 1 {
		t.Fatalf("contention: %v", err)
	}
	after, _ := s.GetCanvasByID(ctx, c.ID)
	if before.Version != after.Version {
		t.Fatal("audit/contention must not bump the version")
	}
	if n, err := s.DeleteAction(ctx, c.ID, task.ID); err != nil || n != 1 {
		t.Fatalf("delete: %d %v", n, err)
	}
}

func TestEpicApprovalCascade(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	c := newCanvas(t, s)
	epic := &store.Action{Type: "epic", State: "proposed", Payload: json.RawMessage(`{"title":"E1"}`)}
	if _, err := s.CreateAction(ctx, c.ID, epic); err != nil {
		t.Fatal(err)
	}
	eid := epic.ID.String()
	t1 := newTask(t, s, c.ID, "proposed", `{"title":"a","epicId":"`+eid+`"}`)
	t2 := newTask(t, s, c.ID, "proposed", `{"title":"b","epicId":"`+eid+`","requiresApproval":true}`)
	t3 := newTask(t, s, c.ID, "proposed", `{"title":"c","epicId":"`+eid+`","requiresApproval":"true"}`)
	other := newTask(t, s, c.ID, "proposed", `{"title":"d"}`)
	flipped, err := s.ApproveEpicTasks(ctx, c.ID, epic.ID, "policy:epic")
	if err != nil || len(flipped) != 1 || flipped[0].ID != t1.ID || *flipped[0].ApprovedBy != "policy:epic" {
		t.Fatalf("cascade: %d %v", len(flipped), err)
	}
	for _, id := range []uuid.UUID{t2.ID, t3.ID, other.ID} {
		if a, _ := s.GetAction(ctx, c.ID, id); a.State != "proposed" {
			t.Fatalf("%s should stay proposed", a.TicketID())
		}
	}
	// Second cascade: nothing to flip, no bump.
	before, _ := s.GetCanvasByID(ctx, c.ID)
	if again, _ := s.ApproveEpicTasks(ctx, c.ID, epic.ID, "policy:epic"); len(again) != 0 {
		t.Fatal("second cascade flipped rows")
	}
	if after, _ := s.GetCanvasByID(ctx, c.ID); after.Version != before.Version {
		t.Fatal("empty cascade bumped the version")
	}
	// Unapprove takes back only policy-approved, unclaimed tickets.
	if _, _, err := s.ClaimAction(ctx, c.ID, t1.ID, "w"); err != nil {
		t.Fatal(err)
	}
	if back, _ := s.UnapproveEpicTasks(ctx, c.ID, epic.ID, "policy:epic"); len(back) != 0 {
		t.Fatal("unapprove took a claimed ticket")
	}
	_, _, _ = s.ReleaseAction(ctx, c.ID, t1.ID)
	back, err := s.UnapproveEpicTasks(ctx, c.ID, epic.ID, "policy:epic")
	if err != nil || len(back) != 1 || back[0].State != "proposed" || back[0].ApprovedBy != nil {
		t.Fatalf("unapprove: %+v %v", back, err)
	}
	// Batch approve is retry-safe.
	batch, err := s.ApproveActionsBatch(ctx, c.ID, []uuid.UUID{t1.ID, t2.ID, uuid.New()}, "jaxon")
	if err != nil || len(batch) != 2 {
		t.Fatalf("batch: %d %v", len(batch), err)
	}
	if again, _ := s.ApproveActionsBatch(ctx, c.ID, []uuid.UUID{t1.ID}, "jaxon"); len(again) != 0 {
		t.Fatal("batch re-approved a moved ticket")
	}
	// MoveEpic: approve then rewind with a reason.
	if _, _, err := s.MoveEpic(ctx, c.ID, epic.ID, "proposed", "rejected", "nope"); err != nil {
		t.Fatal(err)
	}
	e, _ := s.GetAction(ctx, c.ID, epic.ID)
	if e.State != "rejected" || e.Error == nil || *e.Error != "nope" {
		t.Fatalf("move epic: %+v", e)
	}
	if _, _, err := s.MoveEpic(ctx, c.ID, t1.ID, "approved", "proposed", ""); !errors.Is(err, store.ErrIllegalActionState) {
		t.Fatalf("task via MoveEpic: %v", err)
	}
}

func TestAgentsUpsertAndPresence(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	c := newCanvas(t, s)
	model := "claude"
	planner := &store.Agent{Name: "orchestrator", Role: "planner", Model: &model}
	if _, err := s.RegisterAgent(ctx, c.ID, planner); err != nil {
		t.Fatal(err)
	}
	worker := &store.Agent{Name: "worker-1", Role: "executor", ParentAgentID: &planner.ID}
	if _, err := s.RegisterAgent(ctx, c.ID, worker); err != nil {
		t.Fatal(err)
	}
	// Re-registering without the parent keeps it.
	again := &store.Agent{Name: "worker-1", Role: "executor"}
	if _, err := s.RegisterAgent(ctx, c.ID, again); err != nil {
		t.Fatal(err)
	}
	if again.ID != worker.ID || again.ParentAgentID == nil || *again.ParentAgentID != planner.ID {
		t.Fatalf("re-register re-keyed or unparented: %+v", again)
	}
	if err := s.TouchOrCreateAgent(ctx, c.ID, "ci-bot"); err != nil {
		t.Fatal(err)
	}
	agents, _ := s.ListAgents(ctx, c.ID)
	if len(agents) != 3 || agents[2].Name != "ci-bot" || agents[2].Role != "executor" {
		t.Fatalf("agents: %+v", agents)
	}
	if _, err := s.GetAgent(ctx, uuid.New(), planner.ID); err == nil {
		t.Fatal("agent lookup must be canvas-scoped")
	}
}

func TestWebhookDeliveries(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	c := newCanvas(t, s)
	secret := store.GenerateWebhookSecret()
	w, err := s.CreateWebhook(ctx, c.ID, &store.Webhook{URL: "http://localhost:8787/webhook", Secret: secret, Events: []string{"task.approved", "task.approved"}, Enabled: true, Name: "listen"})
	if err != nil {
		t.Fatal(err)
	}
	if w.Secret != "" || w.SecretLastFour != secret[len(secret)-4:] || len(w.Events) != 1 {
		t.Fatalf("create scrub/normalize: %+v", w)
	}
	if _, err := s.CreateWebhook(ctx, c.ID, &store.Webhook{URL: "http://x", Secret: "s", Events: []string{"task.nope"}}); err == nil {
		t.Fatal("unknown event accepted")
	}
	forEvent, _ := s.ListWebhooksForEvent(ctx, c.ID, "task.approved")
	if len(forEvent) != 1 || forEvent[0].Secret != secret {
		t.Fatalf("for event: %+v", forEvent)
	}
	if none, _ := s.ListWebhooksForEvent(ctx, c.ID, "task.completed"); len(none) != 0 {
		t.Fatal("event filter leaked")
	}
	withSecret, _ := s.GetWebhookWithSecret(ctx, w.ID)
	if withSecret.Secret != secret {
		t.Fatal("worker lookup lost the secret")
	}
	eventID := uuid.New()
	d := &store.WebhookDelivery{WebhookID: w.ID, CanvasID: c.ID, EventID: eventID, EventType: "task.approved", Payload: json.RawMessage(`{"x":1}`)}
	if n, err := s.CreateWebhookDeliveries(ctx, []*store.WebhookDelivery{d}); err != nil || n != 1 {
		t.Fatalf("fan-out: %d %v", n, err)
	}
	if n, _ := s.CreateWebhookDeliveries(ctx, []*store.WebhookDelivery{d}); n != 0 {
		t.Fatal("re-emit must be a no-op")
	}
	leased, err := s.LeaseWebhookDeliveries(ctx, 10)
	if err != nil || len(leased) != 1 || leased[0].Status != "delivering" || leased[0].AttemptCount != 1 || leased[0].LastAttemptAt == nil {
		t.Fatalf("lease: %+v %v", leased, err)
	}
	if again, _ := s.LeaseWebhookDeliveries(ctx, 10); len(again) != 0 {
		t.Fatal("double lease")
	}
	status := 502
	if err := s.MarkWebhookDeliveryFailed(ctx, leased[0].ID, time.Now().Add(-time.Second), store.WebhookDeliveryResult{ResponseStatus: &status, ResponseBody: "bad gateway"}); err != nil {
		t.Fatal(err)
	}
	retry, _ := s.LeaseWebhookDeliveries(ctx, 10)
	if len(retry) != 1 || retry[0].AttemptCount != 2 || *retry[0].ResponseStatus != 502 {
		t.Fatalf("retry lease: %+v", retry)
	}
	if err := s.MarkWebhookDeliveryDead(ctx, retry[0].ID, store.WebhookDeliveryResult{Error: "gave up"}); err != nil {
		t.Fatal(err)
	}
	dead, _ := s.ListWebhookDeliveries(ctx, c.ID, &w.ID, "dead", 0)
	if len(dead) != 1 || dead[0].Error != "gave up" {
		t.Fatalf("dead list: %+v", dead)
	}
	revived, err := s.RetryWebhookDelivery(ctx, c.ID, dead[0].ID)
	if err != nil || revived.Status != "pending" || revived.AttemptCount != 0 || revived.Error != "" {
		t.Fatalf("retry button: %+v %v", revived, err)
	}
	if _, err := s.RetryWebhookDelivery(ctx, c.ID, dead[0].ID); !errors.Is(err, store.ErrWebhookDeliveryNotRetryable) {
		t.Fatalf("retrying a pending row: %v", err)
	}
	ok, _ := s.LeaseWebhookDeliveries(ctx, 1)
	if err := s.MarkWebhookDeliveryOK(ctx, ok[0].ID, store.WebhookDeliveryResult{ResponseStatus: &status}); err != nil {
		t.Fatal(err)
	}
	// Reap: a stuck delivering row older than the lease goes back to failed.
	d2 := &store.WebhookDelivery{WebhookID: w.ID, CanvasID: c.ID, EventID: uuid.New(), EventType: "task.approved"}
	_, _ = s.CreateWebhookDeliveries(ctx, []*store.WebhookDelivery{d2})
	stuck, _ := s.LeaseWebhookDeliveries(ctx, 1)
	if len(stuck) != 1 {
		t.Fatal("lease for reap")
	}
	if n, _ := s.ReapStuckWebhookDeliveries(ctx, time.Hour); n != 0 {
		t.Fatal("fresh lease reaped")
	}
	time.Sleep(20 * time.Millisecond)
	if n, _ := s.ReapStuckWebhookDeliveries(ctx, 10*time.Millisecond); n != 1 {
		t.Fatal("stale lease not reaped")
	}
	enabled := false
	if upd, err := s.UpdateWebhook(ctx, c.ID, w.ID, store.WebhookPatch{Enabled: &enabled}); err != nil || upd.Enabled {
		t.Fatalf("update: %+v %v", upd, err)
	}
	if cnt, _ := s.CountWebhooks(ctx, c.ID); cnt != 1 {
		t.Fatalf("count = %d", cnt)
	}
	if err := s.DeleteWebhook(ctx, c.ID, w.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteWebhook(ctx, c.ID, w.ID); !errors.Is(err, store.ErrWebhookNotFound) {
		t.Fatalf("double delete: %v", err)
	}
	if left, _ := s.ListWebhookDeliveries(ctx, c.ID, nil, "", 0); len(left) != 0 {
		t.Fatal("deliveries survived webhook delete")
	}
}

func TestPATAndOAuth(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	u, _ := s.UpsertUserByGoogleSub(ctx, &store.User{GoogleSub: "g", Email: "u@x"})
	plain := store.GeneratePAT()
	tok, err := s.CreatePersonalAccessToken(ctx, u.ID, "cli", store.HashToken(plain), store.LastFour(plain))
	if err != nil {
		t.Fatal(err)
	}
	uid, err := s.UserIDByTokenHash(ctx, store.HashToken(plain))
	if err != nil || uid != u.ID {
		t.Fatalf("lookup: %v", err)
	}
	if _, err := s.UserIDByTokenHash(ctx, store.HashToken("nope")); !errors.Is(err, store.ErrInvalidToken) {
		t.Fatalf("bad hash: %v", err)
	}
	list, _ := s.ListPersonalAccessTokens(ctx, u.ID)
	if len(list) != 1 || list[0].LastUsedAt == nil {
		t.Fatalf("list: %+v", list)
	}
	if err := s.DeletePersonalAccessToken(ctx, uuid.New(), tok.ID); !errors.Is(err, store.ErrInvalidToken) {
		t.Fatalf("wrong owner delete: %v", err)
	}
	if err := s.DeletePersonalAccessToken(ctx, u.ID, tok.ID); err != nil {
		t.Fatal(err)
	}
	// OAuth: client → code (single use) → grant → refresh rotation → revoke.
	client := &store.OAuthClient{ID: store.GenerateOAuthClientID(), ClientName: "claude.ai", RedirectURIs: []string{"https://claude.ai/cb"}, GrantTypes: []string{"authorization_code"}, TokenEndpointAuthMethod: "none"}
	if err := s.CreateOAuthClient(ctx, client); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetOAuthClient(ctx, client.ID)
	if err != nil || !got.AllowsRedirect("https://claude.ai/cb") {
		t.Fatalf("client: %+v %v", got, err)
	}
	code := store.GenerateAuthCode()
	if err := s.CreateAuthCode(ctx, store.HashToken(code), &store.OAuthCode{ClientID: client.ID, UserID: u.ID, RedirectURI: "https://claude.ai/cb", CodeChallenge: "c", CodeChallengeMethod: "S256", ExpiresAt: time.Now().Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	consumed, err := s.ConsumeAuthCode(ctx, store.HashToken(code))
	if err != nil || consumed.UserID != u.ID {
		t.Fatalf("consume: %v", err)
	}
	if _, err := s.ConsumeAuthCode(ctx, store.HashToken(code)); !errors.Is(err, store.ErrInvalidGrant) {
		t.Fatalf("replay: %v", err)
	}
	access, refresh := store.GenerateAccessToken(), store.GenerateRefreshToken()
	exp := time.Now().Add(time.Hour)
	if err := s.CreateOAuthGrant(ctx, store.HashToken(access), store.HashToken(refresh), &store.OAuthGrant{ClientID: client.ID, UserID: u.ID, Scope: "canvas", AccessExpiresAt: exp, RefreshExpiresAt: &exp}); err != nil {
		t.Fatal(err)
	}
	uid, cid, err := s.OAuthUserByAccessHash(ctx, store.HashToken(access))
	if err != nil || uid != u.ID || cid != client.ID {
		t.Fatalf("access lookup: %v", err)
	}
	if live, _ := s.HasLiveOAuthGrant(ctx, u.ID, client.ID); !live {
		t.Fatal("grant should be live")
	}
	conns, _ := s.ListOAuthConnections(ctx, u.ID)
	if len(conns) != 1 || conns[0].ClientName != "claude.ai" || conns[0].LastUsedAt == nil {
		t.Fatalf("connections: %+v", conns)
	}
	g, err := s.ConsumeRefreshGrant(ctx, store.HashToken(refresh))
	if err != nil || g.UserID != u.ID {
		t.Fatalf("refresh: %v", err)
	}
	if _, err := s.ConsumeRefreshGrant(ctx, store.HashToken(refresh)); !errors.Is(err, store.ErrInvalidGrant) {
		t.Fatalf("refresh replay: %v", err)
	}
	if _, _, err := s.OAuthUserByAccessHash(ctx, store.HashToken(access)); !errors.Is(err, store.ErrInvalidToken) {
		t.Fatalf("revoked access still resolves: %v", err)
	}
	if err := s.RevokeOAuthConnection(ctx, u.ID, client.ID); !errors.Is(err, store.ErrInvalidGrant) {
		t.Fatalf("nothing left to revoke: %v", err)
	}
	if got, err := s.UpdateUserAgentFollowStyle(ctx, u.ID, "minimal"); err != nil || got.AgentFollowStyle != "minimal" {
		t.Fatalf("pref: %v", err)
	}
	if _, err := s.UpdateUserDefaultPublicRole(ctx, uuid.New(), "read"); !errors.Is(err, store.ErrUserNotFound) {
		t.Fatalf("missing user pref: %v", err)
	}
}

func TestTimestampsCompareLexically(t *testing.T) {
	a := fmtTime(time.Date(2026, 1, 1, 0, 0, 0, 500000000, time.UTC))
	b := fmtTime(time.Date(2026, 1, 1, 0, 0, 0, 500000001, time.UTC))
	if !(a < b) {
		t.Fatalf("fixed-width timestamps must order lexically: %s vs %s", a, b)
	}
	if parseTime(a).Nanosecond() != 500000000 {
		t.Fatal("round trip lost precision")
	}
}

// Provenance (authored_by) is stamped on INSERT and comes back on every read
// path — the store half of what api/provenance.go derives from the request.
func TestAuthoredByRoundTrips(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	c := newCanvas(t, s)
	author := "agent:planner-1"

	a := &store.Action{Type: "task", State: "proposed", Payload: json.RawMessage(`{"title":"t"}`), ProposedBy: "p", AuthoredBy: &author}
	if _, err := s.CreateAction(ctx, c.ID, a); err != nil {
		t.Fatalf("CreateAction: %v", err)
	}
	got, err := s.GetAction(ctx, c.ID, a.ID)
	if err != nil {
		t.Fatalf("GetAction: %v", err)
	}
	if got.AuthoredBy == nil || *got.AuthoredBy != author {
		t.Errorf("action AuthoredBy = %v, want %q", got.AuthoredBy, author)
	}

	unset := &store.Action{Type: "task", State: "proposed", Payload: json.RawMessage(`{"title":"u"}`), ProposedBy: "p"}
	if _, err := s.CreateAction(ctx, c.ID, unset); err != nil {
		t.Fatalf("CreateAction: %v", err)
	}
	if got, _ := s.GetAction(ctx, c.ID, unset.ID); got.AuthoredBy != nil {
		t.Errorf("unset AuthoredBy came back as %q", *got.AuthoredBy)
	}

	d := &store.Document{Type: "notes", Name: "Notes", AuthoredBy: &author}
	if _, err := s.CreateDocument(ctx, c.ID, d); err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}
	n := &store.Note{Body: "hello", DocumentID: &d.ID, AuthoredBy: &author}
	if _, err := s.CreateNote(ctx, c.ID, n); err != nil {
		t.Fatalf("CreateNote: %v", err)
	}
	_, state, _, err := s.GetCanvasState(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetCanvasState: %v", err)
	}
	if doc := state.Documents[d.ID.String()]; doc == nil || doc.AuthoredBy == nil || *doc.AuthoredBy != author {
		t.Errorf("document AuthoredBy not surfaced: %+v", doc)
	}
	if note := state.Notes[n.ID.String()]; note == nil || note.AuthoredBy == nil || *note.AuthoredBy != author {
		t.Errorf("note AuthoredBy not surfaced: %+v", note)
	}
}
