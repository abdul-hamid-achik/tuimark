package main

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Scripted is a deterministic, in-process provider: it needs no network and
// no model, so it is the default and the one exercised by fast tests. It
// recognizes a handful of literal prompt shapes and otherwise answers with a
// canned explanation, always streaming its reply token by token.
type Scripted struct {
	// Delay between streamed tokens. Tests set this to 0.
	Delay time.Duration
}

// NewScripted returns a Scripted provider with a small, human-visible delay
// between streamed tokens.
func NewScripted() *Scripted { return &Scripted{Delay: 15 * time.Millisecond} }

func (s *Scripted) Name() string { return "scripted" }

var (
	reRead = regexp.MustCompile(`^read\s+(\S+)$`)
	reEdit = regexp.MustCompile(`^edit\s+(\S+):\s*(.+?)\s*->\s*(.+)$`)
	reRun  = regexp.MustCompile(`^run\s+(\S+)(?:\s+(.*))?$`)
	reSrch = regexp.MustCompile(`^search\s+(.+)$`)
)

// Chat implements Provider. When history's last message is a tool result it
// produces the final, streamed answer; otherwise it parses the latest user
// message into at most one tool call, or streams a canned reply.
func (s *Scripted) Chat(ctx context.Context, history []ChatMessage, onToken func(string)) (ChatResponse, error) {
	if len(history) > 0 && history[len(history)-1].Role == "tool" {
		result := history[len(history)-1].Content
		return s.stream(ctx, onToken, "done. "+summarize(result))
	}
	prompt := lastUser(history)
	trimmed := strings.TrimSpace(prompt)
	lower := strings.ToLower(trimmed)

	switch {
	case lower == "list" || lower == "list files":
		return ChatResponse{ToolCalls: []ToolCall{{ID: "call-1", Name: "list_files", Arguments: map[string]any{"path": "."}}}}, nil
	case reRead.MatchString(trimmed):
		m := reRead.FindStringSubmatch(trimmed)
		return ChatResponse{ToolCalls: []ToolCall{{ID: "call-1", Name: "read_file", Arguments: map[string]any{"path": m[1]}}}}, nil
	case reSrch.MatchString(trimmed):
		m := reSrch.FindStringSubmatch(trimmed)
		return ChatResponse{ToolCalls: []ToolCall{{ID: "call-1", Name: "search", Arguments: map[string]any{"text": m[1]}}}}, nil
	case reEdit.MatchString(trimmed):
		m := reEdit.FindStringSubmatch(trimmed)
		return ChatResponse{ToolCalls: []ToolCall{{ID: "call-1", Name: "edit_file", Arguments: map[string]any{
			"path": m[1], "old_string": m[2], "new_string": m[3],
		}}}}, nil
	case reRun.MatchString(trimmed):
		m := reRun.FindStringSubmatch(trimmed)
		argv := []any{m[1]}
		for _, a := range strings.Fields(m[2]) {
			argv = append(argv, a)
		}
		return ChatResponse{ToolCalls: []ToolCall{{ID: "call-1", Name: "run_command", Arguments: map[string]any{"argv": argv}}}}, nil
	}
	return s.stream(ctx, onToken, canned(trimmed))
}

func canned(prompt string) string {
	if prompt == "" {
		prompt = "that"
	}
	return fmt.Sprintf(
		"I'm the scripted provider: a deterministic stand-in for a model. "+
			"I understood %q as a plain question, so here is a canned explanation. "+
			"Try \"list\", \"read FILE\", \"search TEXT\", \"edit FILE: OLD -> NEW\", or \"run CMD ARGS\" "+
			"to see a tool call.", prompt,
	)
}

func summarize(result string) string {
	if len(result) > 160 {
		return result[:160] + "…"
	}
	if result == "" {
		return "(empty result)"
	}
	return result
}

func lastUser(history []ChatMessage) string {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == "user" {
			return history[i].Content
		}
	}
	return ""
}

// stream splits text into words and delivers them one at a time through
// onToken, honoring ctx cancellation between tokens.
func (s *Scripted) stream(ctx context.Context, onToken func(string), text string) (ChatResponse, error) {
	words := strings.Fields(text)
	var b strings.Builder
	for i, w := range words {
		select {
		case <-ctx.Done():
			return ChatResponse{Content: b.String()}, ctx.Err()
		default:
		}
		if i > 0 {
			b.WriteByte(' ')
			if onToken != nil {
				onToken(" ")
			}
		}
		b.WriteString(w)
		if onToken != nil {
			onToken(w)
		}
		if s.Delay > 0 {
			select {
			case <-ctx.Done():
				return ChatResponse{Content: b.String()}, ctx.Err()
			case <-time.After(s.Delay):
			}
		}
	}
	return ChatResponse{Content: b.String()}, nil
}

// argvFromAny coerces a JSON-decoded []any (as tool arguments carry) into
// []string, used by both providers' tool-call plumbing.
func argvFromAny(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, e := range arr {
		switch x := e.(type) {
		case string:
			out = append(out, x)
		case float64:
			out = append(out, strconv.FormatFloat(x, 'f', -1, 64))
		}
	}
	return out
}
