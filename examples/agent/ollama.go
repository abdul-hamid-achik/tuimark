package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Ollama is a real HTTP client for a local Ollama server's streaming
// /api/chat endpoint (NDJSON, one JSON object per line, the last with
// "done": true). It is used both against a real server and, in tests and
// the --fixture flag, against the in-process loopback fake in fixture.go.
type Ollama struct {
	Endpoint string // e.g. http://127.0.0.1:11434
	Model    string
	Client   *http.Client
}

// NewOllama returns an Ollama provider. A nil client gets a default timeout.
func NewOllama(endpoint, model string, client *http.Client) *Ollama {
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	return &Ollama{Endpoint: endpoint, Model: model, Client: client}
}

func (o *Ollama) Name() string { return "ollama" }

// wireMessage is one entry of the /api/chat "messages" array.
type wireMessage struct {
	Role      string         `json:"role"`
	Content   string         `json:"content"`
	ToolCalls []wireToolCall `json:"tool_calls,omitempty"`
	ToolName  string         `json:"tool_name,omitempty"`
}

type wireToolCall struct {
	Function wireFunction `json:"function"`
}

type wireFunction struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type wireTool struct {
	Type     string       `json:"type"`
	Function wireToolSpec `json:"function"`
}

type wireToolSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  wireToolSchema `json:"parameters"`
}

type wireToolSchema struct {
	Type       string                    `json:"type"`
	Properties map[string]map[string]any `json:"properties"`
}

func ollamaTools() []wireTool {
	out := make([]wireTool, 0, len(toolCatalog))
	for _, t := range toolCatalog {
		props := map[string]map[string]any{}
		for _, p := range t.Params {
			typ := "string"
			if p == "argv" {
				typ = "array"
			}
			props[p] = map[string]any{"type": typ}
		}
		out = append(out, wireTool{
			Type: "function",
			Function: wireToolSpec{
				Name: t.Name, Description: t.Description,
				Parameters: wireToolSchema{Type: "object", Properties: props},
			},
		})
	}
	return out
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []wireMessage `json:"messages"`
	Stream   bool          `json:"stream"`
	Tools    []wireTool    `json:"tools,omitempty"`
}

type chatChunk struct {
	Message wireMessage `json:"message"`
	Done    bool        `json:"done"`
}

// systemPrompt tells tool-calling models the workspace conventions; small
// local models otherwise invent absolute paths like "/workspace".
const systemPrompt = "You are a coding agent working inside one workspace directory. " +
	"Every path you pass to a tool is relative to the workspace root; use \".\" for the root and never an absolute path. " +
	"Use the tools to inspect files before answering questions about them. " +
	"edit_file and run_command need the operator's approval and are refused in plan mode; if a tool returns an error, explain it briefly."

// Chat implements Provider against a real (or fixture) Ollama server: it
// posts the history as an NDJSON streaming request and forwards each
// incremental content delta through onToken as it is decoded.
func (o *Ollama) Chat(ctx context.Context, history []ChatMessage, onToken func(string)) (ChatResponse, error) {
	req := chatRequest{Model: o.Model, Stream: true, Tools: ollamaTools()}
	req.Messages = append(req.Messages, wireMessage{Role: "system", Content: systemPrompt})
	for _, m := range history {
		wm := wireMessage{Role: m.Role, Content: m.Content}
		if m.ToolName != "" {
			wm.ToolName = m.ToolName
		}
		for _, tc := range m.ToolCalls {
			wm.ToolCalls = append(wm.ToolCalls, wireToolCall{Function: wireFunction{Name: tc.Name, Arguments: tc.Arguments}})
		}
		req.Messages = append(req.Messages, wm)
	}
	body, err := json.Marshal(req)
	if err != nil {
		return ChatResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.Endpoint+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return ChatResponse{}, err
	}
	httpReq.Header.Set("content-type", "application/json")
	resp, err := o.Client.Do(httpReq)
	if err != nil {
		return ChatResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ChatResponse{}, fmt.Errorf("ollama: %s: unexpected status %s", o.Endpoint, resp.Status)
	}

	var content string
	var calls []ToolCall
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var chunk chatChunk
		if err := json.Unmarshal(line, &chunk); err != nil {
			return ChatResponse{Content: content, ToolCalls: calls}, fmt.Errorf("ollama: decoding stream: %w", err)
		}
		if chunk.Message.Content != "" {
			content += chunk.Message.Content
			if onToken != nil {
				onToken(chunk.Message.Content)
			}
		}
		for i, tc := range chunk.Message.ToolCalls {
			calls = append(calls, ToolCall{
				ID:        fmt.Sprintf("call-%d", i+1),
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			})
		}
		if chunk.Done {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return ChatResponse{Content: content, ToolCalls: calls}, fmt.Errorf("ollama: reading stream: %w", err)
	}
	return ChatResponse{Content: content, ToolCalls: calls}, nil
}
