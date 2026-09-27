package main

import (
	"context"
	"strings"
	"testing"
)

func chatNow(t *testing.T, p Provider, history []ChatMessage) (ChatResponse, []string) {
	t.Helper()
	var tokens []string
	resp, err := p.Chat(context.Background(), history, func(s string) { tokens = append(tokens, s) })
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	return resp, tokens
}

func TestScriptedRecognizesList(t *testing.T) {
	s := &Scripted{}
	resp, _ := chatNow(t, s, []ChatMessage{{Role: "user", Content: "list"}})
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "list_files" {
		t.Fatalf("ToolCalls = %v, want one list_files call", resp.ToolCalls)
	}
}

func TestScriptedRecognizesRead(t *testing.T) {
	s := &Scripted{}
	resp, _ := chatNow(t, s, []ChatMessage{{Role: "user", Content: "read main.go"}})
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "read_file" || resp.ToolCalls[0].Arguments["path"] != "main.go" {
		t.Fatalf("ToolCalls = %v, want read_file(path=main.go)", resp.ToolCalls)
	}
}

func TestScriptedRecognizesSearch(t *testing.T) {
	s := &Scripted{}
	resp, _ := chatNow(t, s, []ChatMessage{{Role: "user", Content: "search TODO"}})
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "search" || resp.ToolCalls[0].Arguments["text"] != "TODO" {
		t.Fatalf("ToolCalls = %v, want search(text=TODO)", resp.ToolCalls)
	}
}

func TestScriptedRecognizesEdit(t *testing.T) {
	s := &Scripted{}
	resp, _ := chatNow(t, s, []ChatMessage{{Role: "user", Content: "edit notes.txt: before -> after"}})
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("ToolCalls = %v, want one edit_file call", resp.ToolCalls)
	}
	tc := resp.ToolCalls[0]
	if tc.Name != "edit_file" || tc.Arguments["path"] != "notes.txt" || tc.Arguments["old_string"] != "before" || tc.Arguments["new_string"] != "after" {
		t.Fatalf("edit_file call = %+v, want path=notes.txt old=before new=after", tc)
	}
}

func TestScriptedRecognizesRun(t *testing.T) {
	s := &Scripted{}
	resp, _ := chatNow(t, s, []ChatMessage{{Role: "user", Content: "run echo hello world"}})
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "run_command" {
		t.Fatalf("ToolCalls = %v, want one run_command call", resp.ToolCalls)
	}
	argv := argvFromAny(resp.ToolCalls[0].Arguments["argv"])
	if strings.Join(argv, " ") != "echo hello world" {
		t.Errorf("argv = %v, want [echo hello world]", argv)
	}
}

func TestScriptedCannedReply(t *testing.T) {
	s := &Scripted{}
	resp, tokens := chatNow(t, s, []ChatMessage{{Role: "user", Content: "what can you do?"}})
	if len(resp.ToolCalls) != 0 {
		t.Fatalf("ToolCalls = %v, want none for a plain question", resp.ToolCalls)
	}
	if resp.Content == "" {
		t.Fatal("canned reply content is empty")
	}
	if strings.Join(tokens, "") != resp.Content {
		t.Errorf("streamed tokens %q do not reconstruct the final content %q", strings.Join(tokens, ""), resp.Content)
	}
	if len(tokens) < 2 {
		t.Errorf("expected the canned reply to stream more than one token, got %v", tokens)
	}
}

func TestScriptedFollowsUpAfterToolResult(t *testing.T) {
	s := &Scripted{}
	history := []ChatMessage{
		{Role: "user", Content: "list"},
		{Role: "assistant", ToolCalls: []ToolCall{{Name: "list_files"}}},
		{Role: "tool", Content: "main.go\nnotes.txt"},
	}
	resp, _ := chatNow(t, s, history)
	if len(resp.ToolCalls) != 0 {
		t.Fatalf("ToolCalls after a tool result = %v, want none", resp.ToolCalls)
	}
	if !strings.Contains(resp.Content, "main.go") {
		t.Errorf("final content = %q, want it to reference the tool result", resp.Content)
	}
}

func TestScriptedStreamHonorsCancellation(t *testing.T) {
	s := &Scripted{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := s.Chat(ctx, []ChatMessage{{Role: "user", Content: "what can you do?"}}, nil)
	if err == nil {
		t.Error("Chat with an already-cancelled context should return an error")
	}
}
