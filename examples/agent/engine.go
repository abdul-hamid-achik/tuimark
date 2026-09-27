// Engine wires a Provider and a Tools sandbox to a loaded tuimark.App
// entirely through the public package: tuimark.Load, Bind, Set, On, Dump,
// Run. It never imports tuimark's internal packages. The view (agent.tui +
// agent.tcss) owns every pixel; this file owns data and named actions.
package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

// contextBudget is the rough character budget context_pct is measured
// against; it is a demo heuristic, not a real token count.
const contextBudget = 6000

// EngineEvent is one structured step of a turn, used by --headless --json
// and by tests. Interactive mode instead reflects state through ui.Set and
// does not read these.
type EngineEvent struct {
	Type     string         `json:"type"`
	Role     string         `json:"role,omitempty"`
	Text     string         `json:"text,omitempty"`
	Tool     string         `json:"tool,omitempty"`
	Args     map[string]any `json:"args,omitempty"`
	Result   string         `json:"result,omitempty"`
	Approved *bool          `json:"approved,omitempty"`
	Mode     string         `json:"mode,omitempty"`
}

// Agent drives the conversation: it owns the in-memory transcript/tool
// state, runs provider turns (with tool calls and approval) and mirrors
// everything into the bound UI via tuimark.App.Set.
type Agent struct {
	ui        *tuimark.App
	provider  Provider
	provName  string
	model     string
	tools     *Tools
	workspace string

	mu         sync.Mutex
	mode       string // "plan" or "build"
	helpOpen   bool
	messages   []message
	activity   []activityEntry
	files      []fileEntry
	filesSeen  map[string]bool
	seq        int
	cancel     context.CancelFunc
	approvalCh chan bool

	// Headless/testing knobs; zero values are the interactive defaults.
	Headless        bool
	ApproveEdits    bool
	ApproveCommands bool
	OnEvent         func(EngineEvent)
}

// NewAgent builds an Agent over an already-loaded, already-wired-to-nothing
// tuimark.App. Call Wire to register its action handlers.
func NewAgent(ui *tuimark.App, provider Provider, provName, model string, tools *Tools, workspace, mode string) *Agent {
	return &Agent{
		ui: ui, provider: provider, provName: provName, model: model,
		tools: tools, workspace: workspace, mode: mode,
		filesSeen: map[string]bool{},
	}
}

// Init pushes the agent's starting state (empty transcript, chosen mode and
// provider/model) into the UI. Call it once after Wire, before Run.
func (ag *Agent) Init() error {
	ag.mu.Lock()
	defer ag.mu.Unlock()
	data := map[string]any{
		"app_name":         "tuimark-agent",
		"provider":         ag.provName,
		"model":            ag.model,
		"mode":             ag.mode,
		"workspace":        ag.workspace,
		"status":           "ready",
		"draft":            "",
		"context_pct":      0,
		"selected_message": "",
		"help_open":        false,
		"approval":         approvalState{},
		"messages":         []message{},
		"tool_activity":    []activityEntry{},
		"files_touched":    []fileEntry{},
	}
	return ag.ui.Bind("", data)
}

// Wire registers every named action agent.tui references.
func (ag *Agent) Wire() {
	ag.ui.On("send", ag.onSend)
	ag.ui.On("approve", func(tuimark.Event) error { ag.resolveApproval(true); return nil })
	ag.ui.On("deny", func(tuimark.Event) error { ag.resolveApproval(false); return nil })
	ag.ui.On("toggle_mode", ag.onToggleMode)
	ag.ui.On("clear_transcript", ag.onClearTranscript)
	ag.ui.On("cancel", ag.onCancel)
	ag.ui.On("toggle_help", ag.onToggleHelp)
	ag.ui.On("quit", func(tuimark.Event) error { return tuimark.ErrQuit })
}

func (ag *Agent) emit(ev EngineEvent) {
	if ag.OnEvent != nil {
		ag.OnEvent(ev)
	}
}

func (ag *Agent) nextID(prefix string) string {
	ag.seq++
	return fmt.Sprintf("%s-%d", prefix, ag.seq)
}

// --- action handlers -------------------------------------------------------

func (ag *Agent) onSend(ev tuimark.Event) error {
	text, _ := ev.Value.(string)
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	_ = ag.ui.Set("draft", "")
	ag.StartTurn(text)
	return nil
}

func (ag *Agent) onToggleMode(tuimark.Event) error {
	ag.mu.Lock()
	if ag.mode == "plan" {
		ag.mode = "build"
	} else {
		ag.mode = "plan"
	}
	mode := ag.mode
	ag.mu.Unlock()
	ag.emit(EngineEvent{Type: "mode", Mode: mode})
	return ag.ui.Set("mode", mode)
}

func (ag *Agent) onClearTranscript(tuimark.Event) error {
	ag.mu.Lock()
	ag.messages = nil
	ag.mu.Unlock()
	if err := ag.ui.Set("messages", []message{}); err != nil {
		return err
	}
	return ag.ui.Set("selected_message", "")
}

func (ag *Agent) onCancel(tuimark.Event) error {
	ag.mu.Lock()
	c := ag.cancel
	ag.mu.Unlock()
	if c != nil {
		c()
	}
	return nil
}

func (ag *Agent) onToggleHelp(tuimark.Event) error {
	// The action fires both from the keymap ("?"/"ctrl+h") and from the
	// help modal's own on:escape/close button; either way it just flips.
	open := ag.toggleHelpState()
	return ag.ui.Set("help_open", open)
}

func (ag *Agent) toggleHelpState() bool {
	ag.mu.Lock()
	defer ag.mu.Unlock()
	ag.helpOpen = !ag.helpOpen
	return ag.helpOpen
}

func (ag *Agent) resolveApproval(approved bool) {
	ag.mu.Lock()
	ch := ag.approvalCh
	ag.approvalCh = nil
	ag.mu.Unlock()
	if ch == nil {
		return
	}
	_ = ag.ui.Set("approval", approvalState{})
	ch <- approved
}

func (ag *Agent) Mode() string {
	ag.mu.Lock()
	defer ag.mu.Unlock()
	return ag.mode
}

// --- turns -------------------------------------------------------------

// StartTurn appends the user's message and runs the provider turn in a
// goroutine, so the caller (a key handler in interactive mode) never
// blocks. Headless mode instead calls RunTurn directly and waits.
func (ag *Agent) StartTurn(prompt string) {
	ctx, cancel := context.WithCancel(context.Background())
	ag.mu.Lock()
	ag.cancel = cancel
	ag.mu.Unlock()
	go func() {
		defer func() {
			ag.mu.Lock()
			ag.cancel = nil
			ag.mu.Unlock()
		}()
		ag.RunTurn(ctx, prompt)
	}()
}

// RunTurn appends prompt as a user message, then drives the provider loop
// (streaming tokens, running tool calls with approval) until it produces a
// final answer, is cancelled, or hits maxToolRounds.
func (ag *Agent) RunTurn(ctx context.Context, prompt string) {
	ag.appendMessage("user", prompt)
	_ = ag.ui.Set("status", "thinking")
	ag.emit(EngineEvent{Type: "user_message", Role: "user", Text: prompt})

	history := ag.chatHistory()
	const maxToolRounds = 6
	for round := 0; round < maxToolRounds; round++ {
		if ctx.Err() != nil {
			ag.finishCancelled()
			return
		}
		id := ag.startAssistantMessage()
		resp, err := ag.provider.Chat(ctx, history, func(tok string) { ag.appendToMessage(id, tok) })
		if err != nil {
			if ctx.Err() != nil {
				ag.finishCancelled()
				return
			}
			ag.setMessageText(id, fmt.Sprintf("(provider error: %v)", err))
			_ = ag.ui.Set("status", "error")
			ag.emit(EngineEvent{Type: "error", Text: err.Error()})
			return
		}
		if len(resp.ToolCalls) == 0 {
			ag.setMessageText(id, resp.Content)
			ag.emit(EngineEvent{Type: "assistant_message", Role: "assistant", Text: resp.Content})
			_ = ag.ui.Set("status", "ready")
			ag.updateContextPct()
			return
		}
		// A tool-calling turn has no visible content of its own; drop the
		// placeholder message rather than show an empty transcript row.
		ag.removeMessage(id)
		history = append(history, ChatMessage{Role: "assistant", ToolCalls: resp.ToolCalls})
		for _, tc := range resp.ToolCalls {
			if ctx.Err() != nil {
				ag.finishCancelled()
				return
			}
			result := ag.runTool(ctx, tc)
			history = append(history, ChatMessage{Role: "tool", ToolName: tc.Name, Content: result})
		}
	}
	ag.appendMessage("assistant", "(stopped after too many tool calls in a row)")
	_ = ag.ui.Set("status", "ready")
}

func (ag *Agent) finishCancelled() {
	ag.appendMessage("assistant", "(cancelled)")
	_ = ag.ui.Set("status", "cancelled")
	ag.emit(EngineEvent{Type: "cancelled"})
}

// chatHistory converts the transcript into provider-facing messages.
func (ag *Agent) chatHistory() []ChatMessage {
	ag.mu.Lock()
	defer ag.mu.Unlock()
	out := make([]ChatMessage, 0, len(ag.messages))
	for _, m := range ag.messages {
		out = append(out, ChatMessage{Role: m.Role, Content: m.Text})
	}
	return out
}

// --- tool execution ------------------------------------------------------

// planRefused tools are read/write actions plan mode never allows.
var planRefused = map[string]bool{"edit_file": true, "run_command": true}

// approvalRequired tools need a human (or --approve-* flag) yes before
// running, even in build mode.
var approvalRequired = map[string]bool{"edit_file": true, "run_command": true}

func (ag *Agent) runTool(ctx context.Context, tc ToolCall) string {
	label := toolLabel(tc)
	ag.pushActivity(label)
	ag.emit(EngineEvent{Type: "tool_call", Tool: tc.Name, Args: tc.Arguments})

	mode := ag.Mode()
	if mode == "plan" && planRefused[tc.Name] {
		result := "refused: " + tc.Name + " is not allowed in plan mode"
		ag.emit(EngineEvent{Type: "tool_result", Tool: tc.Name, Result: result})
		return result
	}
	if approvalRequired[tc.Name] {
		approved := ag.approve(ctx, tc)
		ag.emit(EngineEvent{Type: "approval", Tool: tc.Name, Approved: &approved})
		if !approved {
			result := "denied: " + tc.Name
			ag.emit(EngineEvent{Type: "tool_result", Tool: tc.Name, Result: result})
			return result
		}
	}
	result := ag.execTool(ctx, tc)
	ag.emit(EngineEvent{Type: "tool_result", Tool: tc.Name, Result: result})
	return result
}

func toolLabel(tc ToolCall) string {
	switch tc.Name {
	case "read_file":
		return "read_file " + str(tc.Arguments["path"])
	case "list_files":
		return "list_files " + str(tc.Arguments["path"])
	case "search":
		return "search " + str(tc.Arguments["text"])
	case "edit_file":
		return "edit_file " + str(tc.Arguments["path"])
	case "run_command":
		return "run_command " + strings.Join(argvFromAny(tc.Arguments["argv"]), " ")
	default:
		return tc.Name
	}
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func toolPreview(tc ToolCall) string {
	switch tc.Name {
	case "edit_file":
		return fmt.Sprintf("%s\n\n- %s\n+ %s", str(tc.Arguments["path"]), str(tc.Arguments["old_string"]), str(tc.Arguments["new_string"]))
	case "run_command":
		return strings.Join(argvFromAny(tc.Arguments["argv"]), " ")
	default:
		return toolLabel(tc)
	}
}

// approve requests a yes/no for tc. Headless mode answers immediately from
// its --approve-* flags; interactive mode opens the approval modal and
// blocks until "approve", "deny", or ctx is cancelled.
func (ag *Agent) approve(ctx context.Context, tc ToolCall) bool {
	if ag.Headless {
		switch tc.Name {
		case "edit_file":
			return ag.ApproveEdits
		case "run_command":
			return ag.ApproveCommands
		default:
			return false
		}
	}
	ch := make(chan bool, 1)
	ag.mu.Lock()
	ag.approvalCh = ch
	ag.mu.Unlock()
	_ = ag.ui.Set("approval", approvalState{Open: true, Tool: tc.Name, Preview: toolPreview(tc)})
	_ = ag.ui.Set("status", "awaiting approval: "+tc.Name)
	select {
	case ans := <-ch:
		return ans
	case <-ctx.Done():
		return false
	}
}

func (ag *Agent) execTool(ctx context.Context, tc ToolCall) string {
	switch tc.Name {
	case "list_files":
		files, err := ag.tools.ListFiles(str(tc.Arguments["path"]))
		if err != nil {
			return "error: " + err.Error()
		}
		return strings.Join(files, "\n")
	case "read_file":
		path := str(tc.Arguments["path"])
		content, err := ag.tools.ReadFile(path)
		if err != nil {
			return "error: " + err.Error()
		}
		ag.pushFile(path)
		return content
	case "search":
		hits, err := ag.tools.Search(str(tc.Arguments["text"]))
		if err != nil {
			return "error: " + err.Error()
		}
		if len(hits) == 0 {
			return "no matches"
		}
		var b strings.Builder
		for _, h := range hits {
			fmt.Fprintf(&b, "%s:%d: %s\n", h.Path, h.Line, h.Text)
		}
		return strings.TrimRight(b.String(), "\n")
	case "edit_file":
		path := str(tc.Arguments["path"])
		if err := ag.tools.EditFile(path, str(tc.Arguments["old_string"]), str(tc.Arguments["new_string"])); err != nil {
			return "error: " + err.Error()
		}
		ag.pushFile(path)
		return "edited " + path
	case "run_command":
		argv := argvFromAny(tc.Arguments["argv"])
		res, err := ag.tools.RunCommand(ctx, argv, 5*time.Second)
		if err != nil {
			return "error: " + err.Error()
		}
		if res.TimedOut {
			return "timed out"
		}
		return fmt.Sprintf("exit %d\n%s", res.ExitCode, res.Output)
	default:
		return "error: unknown tool " + tc.Name
	}
}

// --- state -> UI mirroring -----------------------------------------------

func (ag *Agent) appendMessage(role, text string) string {
	ag.mu.Lock()
	id := ag.nextID("m")
	ag.messages = append(ag.messages, message{ID: id, Role: role, Text: text})
	snapshot := append([]message(nil), ag.messages...)
	ag.mu.Unlock()
	_ = ag.ui.Set("messages", snapshot)
	_ = ag.ui.Set("selected_message", id)
	return id
}

func (ag *Agent) startAssistantMessage() string { return ag.appendMessage("assistant", "") }

func (ag *Agent) appendToMessage(id, tok string) {
	ag.mu.Lock()
	for i := range ag.messages {
		if ag.messages[i].ID == id {
			ag.messages[i].Text += tok
			break
		}
	}
	snapshot := append([]message(nil), ag.messages...)
	ag.mu.Unlock()
	_ = ag.ui.Set("messages", snapshot)
}

func (ag *Agent) setMessageText(id, text string) {
	ag.mu.Lock()
	for i := range ag.messages {
		if ag.messages[i].ID == id {
			ag.messages[i].Text = text
			break
		}
	}
	snapshot := append([]message(nil), ag.messages...)
	ag.mu.Unlock()
	_ = ag.ui.Set("messages", snapshot)
}

func (ag *Agent) removeMessage(id string) {
	ag.mu.Lock()
	out := ag.messages[:0:0]
	for _, m := range ag.messages {
		if m.ID != id {
			out = append(out, m)
		}
	}
	ag.messages = out
	snapshot := append([]message(nil), ag.messages...)
	var lastID string
	if len(snapshot) > 0 {
		lastID = snapshot[len(snapshot)-1].ID
	}
	ag.mu.Unlock()
	_ = ag.ui.Set("messages", snapshot)
	_ = ag.ui.Set("selected_message", lastID)
}

func (ag *Agent) pushActivity(label string) {
	ag.mu.Lock()
	ag.activity = append(ag.activity, activityEntry{ID: ag.nextID("a"), Label: label})
	snapshot := append([]activityEntry(nil), ag.activity...)
	ag.mu.Unlock()
	_ = ag.ui.Set("tool_activity", snapshot)
}

func (ag *Agent) pushFile(path string) {
	ag.mu.Lock()
	if ag.filesSeen[path] {
		ag.mu.Unlock()
		return
	}
	ag.filesSeen[path] = true
	ag.files = append(ag.files, fileEntry{ID: ag.nextID("f"), Path: path})
	snapshot := append([]fileEntry(nil), ag.files...)
	ag.mu.Unlock()
	_ = ag.ui.Set("files_touched", snapshot)
}

func (ag *Agent) updateContextPct() {
	ag.mu.Lock()
	total := 0
	for _, m := range ag.messages {
		total += len(m.Text)
	}
	ag.mu.Unlock()
	pct := total * 100 / contextBudget
	if pct > 100 {
		pct = 100
	}
	_ = ag.ui.Set("context_pct", pct)
}
