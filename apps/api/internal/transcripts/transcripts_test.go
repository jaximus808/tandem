package transcripts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The fixtures under testdata/projects are synthetic, shaped like Claude Code's
// on-disk layout: one main session (a planner that connects, then reads a board
// that MENTIONS the worker's agentId) and one subagent (the worker that
// connects). They are copied into a temp dir so tests can touch mtimes and
// append to them.
const (
	tdm18Session   = "11111111-1111-4111-8111-111111111111"
	tdm18Planner   = "aaaaaaaa-0000-4000-8000-000000000001"
	tdm18Worker    = "bbbbbbbb-0000-4000-8000-000000000002"
	tdm18BlobOnly  = "cccccccc-0000-4000-8000-000000000003"
	tdm18Project   = "-Users-demo-repo"
	tdm18Subagent  = "a0b1c2d3e4f5"
	tdm18Unmatched = "dddddddd-0000-4000-8000-000000000004"
)

// tdm18Clock is a settable clock for the index.
type tdm18Clock struct{ t time.Time }

func (c *tdm18Clock) now() time.Time { return c.t }

// tdm18CopyFixtures copies testdata/projects into a temp root and returns it.
func tdm18CopyFixtures(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "projects")
	src := filepath.Join("testdata", "projects")
	err := filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		dst := filepath.Join(root, rel)
		if info.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, raw, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func tdm18MainPath(root string) string {
	return filepath.Join(root, tdm18Project, tdm18Session+".jsonl")
}

func tdm18SubPath(root string) string {
	return filepath.Join(root, tdm18Project, tdm18Session, "subagents", "agent-"+tdm18Subagent+".jsonl")
}

func tdm18Append(t *testing.T, path, s string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(s); err != nil {
		t.Fatal(err)
	}
	f.Close()
}

// tdm18Line marshals one synthetic JSONL record, newline included.
func tdm18Line(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw) + "\n"
}

func tdm18ToolResult(id, text string) map[string]any {
	return map[string]any{
		"type": "user", "timestamp": "2026-01-02T11:00:00.000Z",
		"message": map[string]any{"role": "user", "content": []any{
			map[string]any{"type": "tool_result", "tool_use_id": id, "content": []any{map[string]any{"type": "text", "text": text}}},
		}},
	}
}

func tdm18ToolUse(id, name string, input map[string]any) map[string]any {
	return map[string]any{
		"type": "assistant", "timestamp": "2026-01-02T11:00:00.000Z",
		"message": map[string]any{"role": "assistant", "model": "claude-test-2", "content": []any{
			map[string]any{"type": "tool_use", "id": id, "name": name, "input": input},
		}},
	}
}

func TestTDM18LookupMatchesMainSessionAndSubagent(t *testing.T) {
	root := tdm18CopyFixtures(t)
	// Make the PLANNER's file the newest. It mentions the worker's agentId in a
	// board_status result, so a substring matcher walking newest-first would
	// hand the planner's transcript to the worker.
	now := time.Now()
	if err := os.Chtimes(tdm18SubPath(root), now.Add(-time.Hour), now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(tdm18MainPath(root), now, now); err != nil {
		t.Fatal(err)
	}
	x := New(Options{Root: root})

	main, err := x.Lookup(tdm18Planner)
	if err != nil || main == nil {
		t.Fatalf("planner lookup = %v, %v", main, err)
	}
	if main.Kind != "session" || main.ID != tdm18Session || main.SessionID != tdm18Session || main.Project != tdm18Project || main.Subagent != nil {
		t.Fatalf("planner transcript = %+v", main)
	}

	sub, err := x.Lookup(strings.ToUpper(tdm18Worker)) // ids compare case-insensitively
	if err != nil || sub == nil {
		t.Fatalf("worker lookup = %v, %v", sub, err)
	}
	if sub.Kind != "subagent" || sub.ID != tdm18Session+"/agent-"+tdm18Subagent || sub.SubagentID != tdm18Subagent || sub.SessionID != tdm18Session {
		t.Fatalf("worker transcript = %+v", sub)
	}
	if sub.Subagent == nil || sub.Subagent.Description != "Work DEMO-1" || sub.Subagent.AgentType != "general-purpose" || sub.Subagent.Model != "opus" || sub.Subagent.Finished == nil || *sub.Subagent.Finished {
		t.Fatalf("subagent meta = %+v", sub.Subagent)
	}

	// An id that appears only inside an api-request-blob copy is not a match;
	// neither is one that appears nowhere.
	for _, id := range []string{tdm18BlobOnly, tdm18Unmatched} {
		if got, err := x.Lookup(id); err != nil || got != nil {
			t.Fatalf("Lookup(%s) = %+v, %v; want no match", id, got, err)
		}
	}
}

func TestTDM18LookupIgnoresFilesOutsideWindow(t *testing.T) {
	root := tdm18CopyFixtures(t)
	old := time.Now().Add(-30 * 24 * time.Hour)
	if err := os.Chtimes(tdm18SubPath(root), old, old); err != nil {
		t.Fatal(err)
	}
	x := New(Options{Root: root, MaxAge: 7 * 24 * time.Hour})
	if got, _ := x.Lookup(tdm18Worker); got != nil {
		t.Fatalf("a 30-day-old file matched: %+v", got)
	}
	if got, _ := x.Lookup(tdm18Planner); got == nil {
		t.Fatal("a fresh file did not match")
	}
}

func TestTDM18LookupMissingRootIsNoMatch(t *testing.T) {
	x := New(Options{Root: filepath.Join(t.TempDir(), "nope")})
	if got, err := x.Lookup(tdm18Planner); got != nil || err != nil {
		t.Fatalf("missing root = %+v, %v", got, err)
	}
}

// The connect call and its result land in separate writes; an incremental scan
// that stops between them must still pair them on the next scan.
func TestTDM18IncrementalScanPairsAcrossAppends(t *testing.T) {
	root := tdm18CopyFixtures(t)
	clock := &tdm18Clock{t: time.Now()}
	x := New(Options{Root: root, Now: clock.now})
	const lateAgent = "eeeeeeee-0000-4000-8000-000000000005"

	tdm18Append(t, tdm18MainPath(root), tdm18Line(t, tdm18ToolUse("toolu_late", "mcp__tandem__agent_register", map[string]any{"name": "renamed"})))
	if got, _ := x.Lookup(lateAgent); got != nil {
		t.Fatalf("matched before the result was written: %+v", got)
	}
	// Result arrives, half-written first: an incomplete line is not consumed.
	line := tdm18Line(t, tdm18ToolResult("toolu_late", `{"agentId":"`+lateAgent+`"}`))
	tdm18Append(t, tdm18MainPath(root), line[:20])
	clock.t = clock.t.Add(time.Minute)
	if got, _ := x.Lookup(lateAgent); got != nil {
		t.Fatalf("matched a half-written line: %+v", got)
	}
	tdm18Append(t, tdm18MainPath(root), line[20:])
	clock.t = clock.t.Add(time.Minute)
	got, err := x.Lookup(lateAgent)
	if err != nil || got == nil || got.Kind != "session" {
		t.Fatalf("late registration = %+v, %v", got, err)
	}
	// The earlier registration in the same file still resolves.
	if p, _ := x.Lookup(tdm18Planner); p == nil || p.Path != got.Path {
		t.Fatalf("planner = %+v", p)
	}
}

func TestTDM18ReadNormalisesEvents(t *testing.T) {
	root := tdm18CopyFixtures(t)
	page, err := Read(tdm18SubPath(root), ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(tdm18SubPath(root))
	if page.Next != info.Size() || page.Size != info.Size() || page.More || page.Reset {
		t.Fatalf("page meta = next %d size %d more %v reset %v (file %d)", page.Next, page.Size, page.More, page.Reset, info.Size())
	}
	var kinds []string
	for _, e := range page.Events {
		kinds = append(kinds, e.Kind)
	}
	// prompt, connect call, connect result, prose, Bash call, Bash result —
	// the attachment and the empty thinking block are dropped.
	want := "user,tool_use,tool_result,text,tool_use,tool_result"
	if strings.Join(kinds, ",") != want {
		t.Fatalf("kinds = %v, want %s", kinds, want)
	}
	e := page.Events
	if e[0].Text != "Handoff: claim DEMO-1 and do it." || e[0].Timestamp != "2026-01-02T10:01:00.000Z" {
		t.Fatalf("prompt = %+v", e[0])
	}
	if e[1].Tool != "mcp__tandem__canvas_connect" || e[1].Input != "DEMOCODE" || e[1].ToolUseID != "toolu_sub_connect" || e[1].Model != "claude-test-2" {
		t.Fatalf("connect call = %+v", e[1])
	}
	if e[2].ToolUseID != "toolu_sub_connect" || !strings.Contains(e[2].Text, tdm18Worker) {
		t.Fatalf("connect result = %+v", e[2])
	}
	if e[4].Tool != "Bash" || e[4].Input != "Run the Go tests" {
		t.Fatalf("bash call = %+v", e[4])
	}
	if e[5].Text != "ok  \tdemo/pkg\t0.01s" || e[5].IsError {
		t.Fatalf("bash result (string content) = %+v", e[5])
	}
	for i := 1; i < len(e); i++ {
		if e[i].ID == e[i-1].ID {
			t.Fatalf("duplicate event id %s", e[i].ID)
		}
	}

	// The main session skips mode/permission-mode/api-request-blob AND isMeta.
	mp, err := Read(tdm18MainPath(root), ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range mp.Events {
		if strings.Contains(ev.Text, "meta noise") || strings.Contains(ev.Text, tdm18BlobOnly) {
			t.Fatalf("bookkeeping leaked into events: %+v", ev)
		}
	}
	if len(mp.Events) != 7 {
		t.Fatalf("main events = %d, want 7: %+v", len(mp.Events), mp.Events)
	}
}

func TestTDM18ReadTailsByOffset(t *testing.T) {
	root := tdm18CopyFixtures(t)
	path := tdm18SubPath(root)
	first, err := Read(path, ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// Nothing new: an empty page that keeps the offset.
	same, _ := Read(path, ReadOptions{After: first.Next})
	if len(same.Events) != 0 || same.Next != first.Next {
		t.Fatalf("idle tail = %d events, next %d (want 0, %d)", len(same.Events), same.Next, first.Next)
	}

	// A half-written line is not consumed and does not move the offset.
	line := tdm18Line(t, tdm18ToolUse("toolu_new", "Read", map[string]any{"file_path": "/repo/main.go"}))
	tdm18Append(t, path, line[:30])
	half, _ := Read(path, ReadOptions{After: first.Next})
	if len(half.Events) != 0 || half.Next != first.Next {
		t.Fatalf("half line = %d events, next %d", len(half.Events), half.Next)
	}
	tdm18Append(t, path, line[30:])
	tdm18Append(t, path, tdm18Line(t, tdm18ToolResult("toolu_new", "package main")))
	next, _ := Read(path, ReadOptions{After: first.Next})
	if len(next.Events) != 2 || next.Events[0].Input != "/repo/main.go" || next.Events[1].Text != "package main" {
		t.Fatalf("appended events = %+v", next.Events)
	}
	if next.Events[0].Offset != first.Next {
		t.Fatalf("first new event offset = %d, want %d", next.Events[0].Offset, first.Next)
	}

	// Limit pages: one event per call walks the same events in order.
	var walked []string
	after := int64(0)
	for i := 0; i < 20; i++ {
		p, err := Read(path, ReadOptions{After: after, Limit: 1})
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range p.Events {
			walked = append(walked, e.ID)
		}
		after = p.Next
		if !p.More {
			break
		}
	}
	all, _ := Read(path, ReadOptions{})
	if len(walked) != len(all.Events) {
		t.Fatalf("paged %d events, full read %d", len(walked), len(all.Events))
	}

	// tail=n is the last n events of the file.
	tail, _ := Read(path, ReadOptions{Tail: 2})
	if len(tail.Events) != 2 || tail.Events[1].Text != "package main" || tail.Next != all.Next {
		t.Fatalf("tail = %+v next %d", tail.Events, tail.Next)
	}

	// An offset past the end (file rewritten) restarts from 0 and says so.
	reset, _ := Read(path, ReadOptions{After: all.Size + 100})
	if !reset.Reset || len(reset.Events) != len(all.Events) {
		t.Fatalf("reset = %v with %d events", reset.Reset, len(reset.Events))
	}

	// An offset in the middle of a line skips to the next line boundary.
	mid, _ := Read(path, ReadOptions{After: first.Next + 5})
	if len(mid.Events) != 1 || mid.Events[0].Text != "package main" {
		t.Fatalf("mid-line offset = %+v", mid.Events)
	}
}

func TestTDM18ReadTruncatesLongOutput(t *testing.T) {
	root := tdm18CopyFixtures(t)
	path := tdm18SubPath(root)
	start, _ := Read(path, ReadOptions{})

	// 3000 bytes of 3-byte runes: the cut must land on a rune boundary.
	long := strings.Repeat("é", 1000) + strings.Repeat("€", 1000)
	tdm18Append(t, path, tdm18Line(t, tdm18ToolResult("toolu_long", long)))
	huge := strings.Repeat("x", 20000)
	tdm18Append(t, path, tdm18Line(t, tdm18ToolUse("toolu_huge", "Write", map[string]any{"content": huge})))

	p, err := Read(path, ReadOptions{After: start.Next})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Events) != 2 {
		t.Fatalf("events = %d", len(p.Events))
	}
	r := p.Events[0]
	if !r.Truncated || r.Length != len(long) || len(r.Text) > ToolResultLimit || len(r.Text) < ToolResultLimit-3 {
		t.Fatalf("tool_result truncation: truncated=%v length=%d text=%d bytes", r.Truncated, r.Length, len(r.Text))
	}
	if !strings.HasPrefix(long, r.Text) || !utf8Valid(r.Text) {
		t.Fatal("truncated text is not a valid prefix")
	}
	if u := p.Events[1]; len(u.Input) > InputSummaryLimit || !strings.HasPrefix(u.Input, `{"content":"xxx`) {
		t.Fatalf("input summary = %d bytes %q…", len(u.Input), u.Input[:20])
	}
}

func utf8Valid(s string) bool { return strings.ToValidUTF8(s, "�") == s }

func TestTDM18Env(t *testing.T) {
	for _, v := range []string{"off", "OFF", "0", "false", "no"} {
		if !Disabled(v) {
			t.Fatalf("Disabled(%q) = false", v)
		}
	}
	for _, v := range []string{"", "on", "1"} {
		if Disabled(v) {
			t.Fatalf("Disabled(%q) = true", v)
		}
	}
	dir := t.TempDir()
	t.Setenv("TANDEM_CLAUDE_DIR", dir)
	if got := RootFromEnv(); got != filepath.Join(dir, "projects") {
		t.Fatalf("RootFromEnv = %q", got)
	}
	t.Setenv("TANDEM_CLAUDE_DIR", filepath.Join(dir, "projects"))
	if got := RootFromEnv(); got != filepath.Join(dir, "projects") {
		t.Fatalf("RootFromEnv (projects dir) = %q", got)
	}
	t.Setenv("TANDEM_TRANSCRIPTS", "off")
	if FromEnv() != nil {
		t.Fatal("FromEnv built an index with TANDEM_TRANSCRIPTS=off")
	}
	t.Setenv("TANDEM_TRANSCRIPTS", "")
	if x := FromEnv(); x == nil || x.Root() != filepath.Join(dir, "projects") {
		t.Fatalf("FromEnv = %+v", x)
	}
}
