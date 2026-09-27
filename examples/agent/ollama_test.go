package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestOllamaChatStreamsTokens exercises the real HTTP client against an
// httptest server that streams several NDJSON chunks, verifying incremental
// content is forwarded through onToken and the final content accumulates.
func TestOllamaChatStreamsTokens(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("server: decoding request: %v", err)
		}
		if req.Model != "qwen-test" {
			t.Errorf("request model = %q, want %q", req.Model, "qwen-test")
		}
		if !req.Stream {
			t.Errorf("request Stream = false, want true")
		}
		if len(req.Tools) == 0 {
			t.Errorf("request carried no tool definitions")
		}
		w.Header().Set("content-type", "application/x-ndjson")
		chunks := []chatChunk{
			{Message: wireMessage{Role: "assistant", Content: "hello "}},
			{Message: wireMessage{Role: "assistant", Content: "world"}},
			{Message: wireMessage{Role: "assistant", Content: ""}, Done: true},
		}
		for _, c := range chunks {
			b, _ := json.Marshal(c)
			w.Write(b)
			w.Write([]byte("\n"))
		}
	}))
	defer srv.Close()

	o := NewOllama(srv.URL, "qwen-test", srv.Client())
	var tokens []string
	resp, err := o.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "hi"}}, func(s string) {
		tokens = append(tokens, s)
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "hello world" {
		t.Errorf("Chat content = %q, want %q", resp.Content, "hello world")
	}
	if strings.Join(tokens, "") != "hello world" {
		t.Errorf("streamed tokens = %q, want %q", strings.Join(tokens, ""), "hello world")
	}
	if len(tokens) < 2 {
		t.Errorf("expected multiple streamed chunks, got %v", tokens)
	}
}

// TestOllamaChatToolCall verifies a tool_calls chunk decodes into
// ChatResponse.ToolCalls with its arguments intact.
func TestOllamaChatToolCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/x-ndjson")
		chunks := []chatChunk{
			{Message: wireMessage{Role: "assistant", ToolCalls: []wireToolCall{{
				Function: wireFunction{Name: "read_file", Arguments: map[string]any{"path": "notes.txt"}},
			}}}},
			{Message: wireMessage{Role: "assistant"}, Done: true},
		}
		for _, c := range chunks {
			b, _ := json.Marshal(c)
			fmt.Fprintf(w, "%s\n", b)
		}
	}))
	defer srv.Close()

	o := NewOllama(srv.URL, "qwen-test", srv.Client())
	resp, err := o.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "read notes.txt"}}, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("ToolCalls = %v, want exactly one", resp.ToolCalls)
	}
	tc := resp.ToolCalls[0]
	if tc.Name != "read_file" || tc.Arguments["path"] != "notes.txt" {
		t.Errorf("ToolCalls[0] = %+v, want read_file(path=notes.txt)", tc)
	}
}

// TestOllamaChatHTTPError verifies a non-200 response surfaces as an error
// rather than being silently swallowed.
func TestOllamaChatHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	o := NewOllama(srv.URL, "qwen-test", srv.Client())
	if _, err := o.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "hi"}}, nil); err == nil {
		t.Error("Chat against a failing server should return an error")
	}
}

// TestFixtureServerRoundTrip drives the loopback fixture through the real
// Ollama client: first turn asks for a tool call, second (once a tool
// result is supplied) answers with the final content.
func TestFixtureServerRoundTrip(t *testing.T) {
	fx, err := NewFixtureServer("fixture-model")
	if err != nil {
		t.Fatalf("NewFixtureServer: %v", err)
	}
	defer fx.Close()

	o := NewOllama(fx.Endpoint(), "fixture-model", nil)
	resp, err := o.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "read notes.txt"}}, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "read_file" {
		t.Fatalf("first turn ToolCalls = %v, want one read_file call", resp.ToolCalls)
	}

	history := []ChatMessage{
		{Role: "user", Content: "read notes.txt"},
		{Role: "assistant", ToolCalls: resp.ToolCalls},
		{Role: "tool", Content: "before\n"},
	}
	var tokens []string
	resp2, err := o.Chat(context.Background(), history, func(s string) { tokens = append(tokens, s) })
	if err != nil {
		t.Fatalf("Chat (second turn): %v", err)
	}
	if !strings.Contains(resp2.Content, "before") {
		t.Errorf("final content = %q, want it to mention the tool result", resp2.Content)
	}
	if len(tokens) == 0 {
		t.Error("expected the final answer to be streamed through onToken")
	}
}
