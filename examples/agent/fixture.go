package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
)

// FixtureServer is an in-process, loopback fake Ollama server. It answers
// /api/chat with deterministic streamed NDJSON: the first turn requests a
// read_file tool call, the second (once a tool-result message is present in
// the history) answers with a final message describing what it read. It
// exists so the ollama provider's real HTTP streaming client can be
// exercised end to end, in tests and in glyph specs, without a model.
type FixtureServer struct {
	srv   *http.Server
	ln    net.Listener
	Model string
}

// NewFixtureServer starts the fixture on 127.0.0.1:0 (an OS-assigned free
// port) and returns it already serving. Call Close to stop it.
func NewFixtureServer(model string) (*FixtureServer, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	if model == "" {
		model = "fixture-model"
	}
	f := &FixtureServer{ln: ln, Model: model}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/chat", f.handleChat)
	mux.HandleFunc("/api/tags", f.handleTags)
	f.srv = &http.Server{Handler: mux}
	go f.srv.Serve(ln)
	return f, nil
}

// Endpoint is the base URL to pass as --endpoint.
func (f *FixtureServer) Endpoint() string { return "http://" + f.ln.Addr().String() }

// Close stops the fixture server.
func (f *FixtureServer) Close() error { return f.srv.Close() }

func (f *FixtureServer) handleTags(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"models": []map[string]any{{"name": f.Model, "size": 1}},
	})
}

func (f *FixtureServer) handleChat(w http.ResponseWriter, r *http.Request) {
	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	hasToolResult := false
	var toolResult string
	for _, m := range req.Messages {
		if m.Role == "tool" {
			hasToolResult = true
			toolResult = m.Content
		}
	}

	var first wireMessage
	if hasToolResult {
		first = wireMessage{Role: "assistant", Content: fmt.Sprintf("fixture read the workspace over loopback HTTP: %s", toolResult)}
	} else {
		first = wireMessage{Role: "assistant", Content: "", ToolCalls: []wireToolCall{{
			Function: wireFunction{Name: "read_file", Arguments: map[string]any{"path": "notes.txt"}},
		}}}
	}

	w.Header().Set("content-type", "application/x-ndjson")
	enc := json.NewEncoder(w)
	_ = enc.Encode(chatChunk{Message: first, Done: false})
	_ = enc.Encode(chatChunk{Message: wireMessage{Role: "assistant"}, Done: true})
}
