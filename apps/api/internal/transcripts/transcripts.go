// Package transcripts links a Tandem agent to the Claude Code transcript it is
// running in, and reads that transcript back as a tail of normalised events
// (TDM-18). LOCAL MODE ONLY — see the gate in internal/api/transcript_handler.go.
//
// # Where the files are
//
// Claude Code writes one JSONL file per session and one per subagent:
//
//	<root>/<cwd-slug>/<sessionId>.jsonl
//	<root>/<cwd-slug>/<sessionId>/subagents/agent-<id>.jsonl   (+ agent-<id>.meta.json)
//
// where <root> is ~/.claude/projects (TANDEM_CLAUDE_DIR, else CLAUDE_CONFIG_DIR,
// else ~/.claude, then /projects) and the slug is the session's cwd with "/"
// replaced by "-". Each line is one JSON record; only `user` and `assistant`
// records carry conversation, everything else (mode, permission-mode,
// api-request-blob, attachment, …) is bookkeeping and is skipped.
//
// # How an agent is matched to a file
//
// A Tandem agent learns its agentId from the `canvas_connect` (or
// `agent_register`) tool call, whose tool_result is the gateway's JSON answer
// with a top-level "agentId". So a file belongs to agentId X when it contains an
// assistant tool_use named mcp__<server>__canvas_connect / __agent_register
// followed by a user tool_result for THAT tool_use id whose payload names X.
// Matching on the pairing rather than on the bare uuid matters: an orchestrator's
// transcript mentions its workers' agentIds too (board_status, handoffs), and a
// substring match would hand the orchestrator's file to every worker.
//
// # Staying cheap
//
// ~/.claude/projects is routinely gigabytes. The index only considers files
// modified within MaxAge (7 days), remembers per file how far it has scanned
// (byte offset at a line boundary) plus any connect calls still awaiting their
// result, and rescans a file only from that offset when its (mtime, size)
// changes. A lookup walks candidates newest first and stops at the first match,
// so the agent being watched live — whose file is by definition the freshest —
// is found without reading the archive. Only lines that contain a needle are
// JSON-decoded at all.
package transcripts

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Limits. Exported so the API layer and the tests speak the same numbers.
const (
	// DefaultMaxAge bounds which files are scanned for a match.
	DefaultMaxAge = 7 * 24 * time.Hour
	// DefaultRefreshInterval is the minimum gap between directory walks.
	DefaultRefreshInterval = 2 * time.Second
	// ToolResultLimit caps a tool_result's text, in bytes.
	ToolResultLimit = 2048
	// TextLimit caps assistant / user prose, in bytes.
	TextLimit = 8192
	// InputSummaryLimit caps a tool_use's one-line input summary, in bytes.
	InputSummaryLimit = 240
	// DefaultLimit / MaxLimit bound the events one Read returns.
	DefaultLimit = 200
	MaxLimit     = 1000
)

// Options configures an Index. Zero values take the defaults above.
type Options struct {
	// Root is the projects directory (…/.claude/projects).
	Root            string
	MaxAge          time.Duration
	RefreshInterval time.Duration
	// Now is the clock (tests).
	Now func() time.Time
}

// Disabled reports whether a TANDEM_TRANSCRIPTS value turns the feature off.
func Disabled(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "off", "0", "false", "no", "disabled":
		return true
	}
	return false
}

// RootFromEnv resolves the projects directory from the environment:
// TANDEM_CLAUDE_DIR, else CLAUDE_CONFIG_DIR (Claude Code's own override), else
// ~/.claude — each with /projects appended. A value that already ends in
// "projects" is taken as the projects directory itself.
func RootFromEnv() string {
	dir := strings.TrimSpace(os.Getenv("TANDEM_CLAUDE_DIR"))
	if dir == "" {
		dir = strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR"))
	}
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		dir = filepath.Join(home, ".claude")
	}
	if strings.HasPrefix(dir, "~"+string(filepath.Separator)) {
		if home, err := os.UserHomeDir(); err == nil {
			dir = filepath.Join(home, dir[2:])
		}
	}
	if filepath.Base(filepath.Clean(dir)) == "projects" {
		return filepath.Clean(dir)
	}
	return filepath.Join(dir, "projects")
}

// FromEnv builds the Index the server uses, or returns nil when
// TANDEM_TRANSCRIPTS turns it off. The auth-mode gate is NOT here: the caller
// decides local-vs-hosted (internal/api owns that fact).
func FromEnv() *Index {
	if Disabled(os.Getenv("TANDEM_TRANSCRIPTS")) {
		return nil
	}
	root := RootFromEnv()
	if root == "" {
		return nil
	}
	return New(Options{Root: root})
}

// Index maps agentIds to transcript files. Safe for concurrent use.
type Index struct {
	opts Options

	mu         sync.Mutex
	files      map[string]*fileState
	candidates []candidate // newest first, from the last walk
	lastWalk   time.Time
}

type candidate struct {
	path    string
	modTime time.Time
	size    int64
}

// fileState is the incremental match state for one file.
type fileState struct {
	modTime time.Time
	size    int64
	// scanned is the byte offset (always at a line boundary) scanned so far.
	scanned int64
	// pending holds connect/register tool_use ids whose result has not been
	// seen yet — the pairing survives a scan that stops between the two lines.
	pending map[string]struct{}
	agents  []string
}

// New builds an Index.
func New(opts Options) *Index {
	if opts.MaxAge <= 0 {
		opts.MaxAge = DefaultMaxAge
	}
	if opts.RefreshInterval <= 0 {
		opts.RefreshInterval = DefaultRefreshInterval
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &Index{opts: opts, files: map[string]*fileState{}}
}

// Root is the projects directory this index reads.
func (x *Index) Root() string { return x.opts.Root }

// Transcript identifies one matched file.
type Transcript struct {
	// ID is stable for the file: "<sessionId>" for a main session,
	// "<sessionId>/agent-<id>" for a subagent. Clients echo it back so a
	// byte offset is never applied to a different file.
	ID         string        `json:"id"`
	Kind       string        `json:"kind"` // "session" | "subagent"
	Path       string        `json:"path"`
	Project    string        `json:"project"` // the cwd slug directory
	SessionID  string        `json:"sessionId"`
	SubagentID string        `json:"subagentId,omitempty"`
	ModifiedAt time.Time     `json:"modifiedAt"`
	Size       int64         `json:"size"`
	Subagent   *SubagentMeta `json:"subagent,omitempty"`
}

// SubagentMeta is the useful slice of agent-<id>.meta.json.
type SubagentMeta struct {
	AgentType   string `json:"agentType,omitempty"`
	Description string `json:"description,omitempty"`
	Model       string `json:"model,omitempty"`
	Finished    *bool  `json:"finished,omitempty"`
}

// Lookup finds the transcript a Tandem agentId is running in. When the same
// agentId registered from several files (re-registering a name returns the same
// id), the most recently modified one wins. Returns (nil, nil) for no match.
func (x *Index) Lookup(agentID string) (*Transcript, error) {
	agentID = strings.ToLower(strings.TrimSpace(agentID))
	if agentID == "" {
		return nil, nil
	}
	x.mu.Lock()
	defer x.mu.Unlock()

	if err := x.refreshLocked(); err != nil {
		return nil, err
	}
	for _, c := range x.candidates {
		st := x.scanLocked(c)
		if st == nil {
			continue
		}
		for _, a := range st.agents {
			if a == agentID {
				return x.describe(c), nil
			}
		}
	}
	return nil, nil
}

// refreshLocked re-walks the root when the last walk is older than
// RefreshInterval. The walk is fixed-depth (projects → sessions → subagents)
// rather than a recursive WalkDir, so tool-result spill directories and other
// siblings are never descended into.
func (x *Index) refreshLocked() error {
	now := x.opts.Now()
	if !x.lastWalk.IsZero() && now.Sub(x.lastWalk) < x.opts.RefreshInterval {
		return nil
	}
	projects, err := os.ReadDir(x.opts.Root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			x.candidates = nil
			x.lastWalk = now
			return nil
		}
		return err
	}
	cutoff := now.Add(-x.opts.MaxAge)
	var out []candidate
	add := func(path string, info os.FileInfo) {
		if info == nil || !info.Mode().IsRegular() || info.ModTime().Before(cutoff) {
			return
		}
		out = append(out, candidate{path: path, modTime: info.ModTime(), size: info.Size()})
	}
	for _, p := range projects {
		if !p.IsDir() {
			continue
		}
		pdir := filepath.Join(x.opts.Root, p.Name())
		entries, err := os.ReadDir(pdir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if !e.IsDir() {
				if strings.HasSuffix(name, ".jsonl") {
					info, _ := e.Info()
					add(filepath.Join(pdir, name), info)
				}
				continue
			}
			sdir := filepath.Join(pdir, name, "subagents")
			subs, err := os.ReadDir(sdir)
			if err != nil {
				continue
			}
			for _, s := range subs {
				sn := s.Name()
				if s.IsDir() || !strings.HasPrefix(sn, "agent-") || !strings.HasSuffix(sn, ".jsonl") {
					continue
				}
				info, _ := s.Info()
				add(filepath.Join(sdir, sn), info)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].modTime.Equal(out[j].modTime) {
			return out[i].modTime.After(out[j].modTime)
		}
		return out[i].path < out[j].path
	})
	x.candidates = out
	x.lastWalk = now
	return nil
}

// scanLocked brings one file's match state up to date and returns it (nil when
// the file can't be read).
func (x *Index) scanLocked(c candidate) *fileState {
	st := x.files[c.path]
	if st != nil && st.modTime.Equal(c.modTime) && st.size == c.size {
		return st
	}
	f, err := os.Open(c.path)
	if err != nil {
		delete(x.files, c.path)
		return nil
	}
	defer f.Close()
	// The walk's (mtime, size) may be a couple of seconds stale; the file's own
	// stat is what the reset check and the cache key must use.
	if info, err := f.Stat(); err == nil {
		c.modTime, c.size = info.ModTime(), info.Size()
	}
	if st == nil || c.size < st.scanned {
		// New, or truncated/rewritten: start over.
		st = &fileState{pending: map[string]struct{}{}}
		x.files[c.path] = st
	}
	if _, err := f.Seek(st.scanned, io.SeekStart); err != nil {
		return st
	}
	lr := newLineReader(f)
	for {
		line, err := lr.next()
		if err != nil {
			break // EOF or an incomplete trailing line: resume from st.scanned next time
		}
		st.scanned += int64(len(line))
		matchLine(st, line)
	}
	st.modTime, st.size = c.modTime, c.size
	return st
}

var (
	needleConnect  = []byte(`canvas_connect"`)
	needleRegister = []byte(`agent_register"`)
	needleResult   = []byte(`"tool_result"`)
	needleAgentID  = []byte(`agentId`)
	needleUser     = []byte(`"type":"user"`)
	needleAsst     = []byte(`"type":"assistant"`)
)

// matchLine advances one file's match state by one line.
func matchLine(st *fileState, line []byte) {
	if bytes.Contains(line, needleConnect) || bytes.Contains(line, needleRegister) {
		if bytes.Contains(line, needleAsst) {
			if rec, ok := parseRecord(line); ok && rec.Type == "assistant" {
				for _, b := range rec.blocks() {
					if b.Type == "tool_use" && isRegisterTool(b.Name) && b.ID != "" {
						st.pending[b.ID] = struct{}{}
					}
				}
			}
		}
	}
	if len(st.pending) == 0 || !bytes.Contains(line, needleResult) || !bytes.Contains(line, needleAgentID) || !bytes.Contains(line, needleUser) {
		return
	}
	rec, ok := parseRecord(line)
	if !ok || rec.Type != "user" {
		return
	}
	for _, b := range rec.blocks() {
		if b.Type != "tool_result" {
			continue
		}
		if _, waiting := st.pending[b.ToolUseID]; !waiting {
			continue
		}
		delete(st.pending, b.ToolUseID)
		if b.IsError {
			continue
		}
		if id := extractAgentID(resultText(b.Content)); id != "" && !contains(st.agents, id) {
			st.agents = append(st.agents, id)
		}
	}
}

// isRegisterTool: the MCP server's local name is the user's choice ("tandem" by
// default), so match mcp__<anything>__canvas_connect / __agent_register.
func isRegisterTool(name string) bool {
	if !strings.HasPrefix(name, "mcp__") {
		return false
	}
	i := strings.LastIndex(name, "__")
	if i < 0 {
		return false
	}
	switch name[i+2:] {
	case "canvas_connect", "agent_register":
		return true
	}
	return false
}

var agentIDRe = regexp.MustCompile(`"agentId"\s*:\s*"([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})"`)

// extractAgentID reads the agentId out of a connect/register answer: the
// top-level field, else agent.agentId, else the first "agentId" in the text
// (an answer wrapped in prose).
func extractAgentID(text string) string {
	var body struct {
		AgentID string `json:"agentId"`
		Agent   *struct {
			AgentID    string `json:"agentId"`
			Registered *bool  `json:"registered"`
		} `json:"agent"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(text)), &body); err == nil {
		if body.AgentID != "" {
			return strings.ToLower(body.AgentID)
		}
		if body.Agent != nil && body.Agent.AgentID != "" {
			return strings.ToLower(body.Agent.AgentID)
		}
		return ""
	}
	if m := agentIDRe.FindStringSubmatch(text); m != nil {
		return strings.ToLower(m[1])
	}
	return ""
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// describe turns a candidate path into a Transcript (meta included).
func (x *Index) describe(c candidate) *Transcript {
	t := &Transcript{Path: c.path, ModifiedAt: c.modTime.UTC(), Size: c.size}
	base := strings.TrimSuffix(filepath.Base(c.path), ".jsonl")
	dir := filepath.Dir(c.path)
	if filepath.Base(dir) == "subagents" {
		sessionDir := filepath.Dir(dir)
		t.Kind = "subagent"
		t.SessionID = filepath.Base(sessionDir)
		t.Project = filepath.Base(filepath.Dir(sessionDir))
		t.SubagentID = strings.TrimPrefix(base, "agent-")
		t.ID = t.SessionID + "/" + base
		t.Subagent = readMeta(filepath.Join(dir, base+".meta.json"))
	} else {
		t.Kind = "session"
		t.SessionID = base
		t.Project = filepath.Base(dir)
		t.ID = base
	}
	return t
}

func readMeta(path string) *SubagentMeta {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var m SubagentMeta
	if json.Unmarshal(raw, &m) != nil {
		return nil
	}
	return &m
}

// ── Reading events ───────────────────────────────────────────────────────────

// Event is one normalised transcript entry. A JSONL record can yield several
// (an assistant turn with prose and two tool calls is three events); ID is
// "<line offset>.<block index>", unique within the file.
type Event struct {
	ID     string `json:"id"`
	Offset int64  `json:"offset"`
	// Kind: "user" (a prompt), "text" (assistant prose), "tool_use",
	// "tool_result".
	Kind      string `json:"kind"`
	Timestamp string `json:"ts,omitempty"`
	Text      string `json:"text,omitempty"`
	Tool      string `json:"tool,omitempty"`  // tool_use: tool name
	Input     string `json:"input,omitempty"` // tool_use: one-line input summary
	ToolUseID string `json:"toolUseId,omitempty"`
	IsError   bool   `json:"isError,omitempty"`
	// Truncated is set when Text was cut; Length is then the original size in
	// bytes.
	Truncated bool   `json:"truncated,omitempty"`
	Length    int    `json:"length,omitempty"`
	Model     string `json:"model,omitempty"`
}

// ReadOptions selects a window of a transcript.
type ReadOptions struct {
	// After is the byte offset to resume from (the previous page's Next).
	After int64
	// Limit caps the events returned (DefaultLimit when 0, at most MaxLimit).
	Limit int
	// Tail, when > 0, ignores After and returns the LAST Tail events of the
	// file — the "open the monitor on a long-running agent" read.
	Tail int
}

// Page is one read.
type Page struct {
	Events []Event `json:"events"`
	// Next is the offset to pass as After on the next read: just past the last
	// complete line consumed. A partially written last line is never consumed.
	Next int64 `json:"next"`
	Size int64 `json:"size"`
	// More is true when Limit stopped the read before the end of the file.
	More bool `json:"more"`
	// Reset is true when After could not apply to this file (past its end — it
	// was rewritten) and the read restarted from 0.
	Reset bool `json:"reset"`
}

// Read returns events from path per opts.
func Read(path string, opts ReadOptions) (*Page, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := info.Size()
	limit := opts.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}
	page := &Page{Events: []Event{}, Size: size}

	tail := opts.Tail
	if tail > MaxLimit {
		tail = MaxLimit
	}
	start := opts.After
	if tail > 0 {
		start = 0
	}
	if start < 0 {
		start = 0
	}
	if start > size {
		start = 0
		page.Reset = true
	}
	// An offset not at a line boundary (a hand-built URL): skip to the next one.
	if start > 0 {
		var prev [1]byte
		if _, err := f.ReadAt(prev[:], start-1); err == nil && prev[0] != '\n' {
			if _, err := f.Seek(start, io.SeekStart); err != nil {
				return nil, err
			}
			lr := newLineReader(f)
			line, err := lr.next()
			if err != nil {
				page.Next = start
				return page, nil
			}
			start += int64(len(line))
		}
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return nil, err
	}
	lr := newLineReader(f)
	off := start
	for {
		line, err := lr.next()
		if err != nil {
			break
		}
		lineOff := off
		off += int64(len(line))
		evs := lineEvents(line, lineOff)
		if len(evs) == 0 {
			continue
		}
		page.Events = append(page.Events, evs...)
		if tail > 0 {
			if len(page.Events) > 2*tail {
				page.Events = append([]Event(nil), page.Events[len(page.Events)-tail:]...)
			}
			continue
		}
		if len(page.Events) >= limit {
			page.Next = off
			page.More = off < size
			return page, nil
		}
	}
	if tail > 0 && len(page.Events) > tail {
		page.Events = page.Events[len(page.Events)-tail:]
	}
	page.Next = off
	return page, nil
}

// record is the slice of a JSONL line the reader needs.
type record struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	IsMeta    bool   `json:"isMeta"`
	Message   struct {
		Model   string          `json:"model"`
		Content json.RawMessage `json:"content"`
	} `json:"message"`
	parsed []block
	str    string
}

type block struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
}

func parseRecord(line []byte) (*record, bool) {
	var r record
	if err := json.Unmarshal(line, &r); err != nil {
		return nil, false
	}
	c := bytes.TrimSpace(r.Message.Content)
	if len(c) > 0 {
		switch c[0] {
		case '"':
			_ = json.Unmarshal(c, &r.str)
		case '[':
			_ = json.Unmarshal(c, &r.parsed)
		}
	}
	return &r, true
}

func (r *record) blocks() []block { return r.parsed }

// lineEvents normalises one JSONL line (none for bookkeeping records).
func lineEvents(line []byte, off int64) []Event {
	if !bytes.Contains(line, needleUser) && !bytes.Contains(line, needleAsst) {
		return nil
	}
	rec, ok := parseRecord(line)
	if !ok || rec.IsMeta || (rec.Type != "user" && rec.Type != "assistant") {
		return nil
	}
	var out []Event
	mk := func(i int, kind string) Event {
		return Event{ID: itoa(off) + "." + itoa(int64(i)), Offset: off, Kind: kind, Timestamp: rec.Timestamp}
	}
	if rec.Type == "user" && rec.str != "" {
		e := mk(0, "user")
		setText(&e, rec.str, TextLimit)
		return []Event{e}
	}
	for i, b := range rec.parsed {
		switch {
		case rec.Type == "user" && b.Type == "text":
			if strings.TrimSpace(b.Text) == "" {
				continue
			}
			e := mk(i, "user")
			setText(&e, b.Text, TextLimit)
			out = append(out, e)
		case rec.Type == "user" && b.Type == "tool_result":
			e := mk(i, "tool_result")
			e.ToolUseID = b.ToolUseID
			e.IsError = b.IsError
			setText(&e, resultText(b.Content), ToolResultLimit)
			out = append(out, e)
		case rec.Type == "assistant" && b.Type == "text":
			if strings.TrimSpace(b.Text) == "" {
				continue
			}
			e := mk(i, "text")
			e.Model = rec.Message.Model
			setText(&e, b.Text, TextLimit)
			out = append(out, e)
		case rec.Type == "assistant" && b.Type == "tool_use":
			e := mk(i, "tool_use")
			e.Model = rec.Message.Model
			e.Tool = b.Name
			e.ToolUseID = b.ID
			e.Input = summarizeInput(b.Input)
			out = append(out, e)
		}
	}
	return out
}

func setText(e *Event, s string, limit int) {
	if len(s) > limit {
		e.Truncated = true
		e.Length = len(s)
		s = truncate(s, limit)
	}
	e.Text = s
}

// resultText flattens a tool_result's content: a plain string, or an array of
// blocks whose text parts are joined (an image becomes "[image]").
func resultText(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return ""
	}
	if raw[0] == '"' {
		var s string
		_ = json.Unmarshal(raw, &s)
		return s
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) != nil {
		return ""
	}
	var sb strings.Builder
	for i, p := range parts {
		if i > 0 {
			sb.WriteByte('\n')
		}
		switch p.Type {
		case "text":
			sb.WriteString(p.Text)
		case "image":
			sb.WriteString("[image]")
		default:
			sb.WriteString("[" + p.Type + "]")
		}
	}
	return sb.String()
}

// summaryKeys are the input fields that say what a call is about, in order of
// preference (Agent's description, Bash's command, Read/Edit's file_path, a
// Tandem tool's ticket id, …).
var summaryKeys = []string{"description", "command", "file_path", "notebook_path", "path", "pattern", "query", "url", "id", "code", "title", "name", "prompt"}

// summarizeInput renders a tool_use input as one short line.
func summarizeInput(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) == nil {
		for _, k := range summaryKeys {
			if s, ok := m[k].(string); ok && strings.TrimSpace(s) != "" {
				return truncate(oneLine(s), InputSummaryLimit)
			}
		}
	}
	var buf bytes.Buffer
	if json.Compact(&buf, raw) == nil {
		return truncate(oneLine(buf.String()), InputSummaryLimit)
	}
	return truncate(oneLine(string(raw)), InputSummaryLimit)
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// truncate cuts s to at most n bytes without splitting a UTF-8 sequence.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// ── Line reading ─────────────────────────────────────────────────────────────

// lineReader yields complete lines (including the trailing '\n') from r,
// reusing one buffer. A final line without '\n' is reported as io.EOF — the
// writer is mid-append, and consuming it would move an offset past bytes that
// are not there yet.
type lineReader struct {
	br  *bufio.Reader
	buf []byte
}

func newLineReader(r io.Reader) *lineReader {
	return &lineReader{br: bufio.NewReaderSize(r, 256<<10)}
}

func (l *lineReader) next() ([]byte, error) {
	l.buf = l.buf[:0]
	for {
		frag, err := l.br.ReadSlice('\n')
		l.buf = append(l.buf, frag...)
		if err == nil {
			return l.buf, nil
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		return nil, io.EOF
	}
}
