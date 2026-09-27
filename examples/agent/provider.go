// Providers turn a conversation into an assistant reply, streamed token by
// token, and may ask the engine to run a tool on their behalf. The engine
// (see engine.go) owns the loop that executes tools and feeds results back.
package main

import "context"

// ChatMessage is one turn in the conversation sent to a provider. Role is
// "user", "assistant", or "tool". ToolCalls is set on an assistant message
// that requested tools; ToolName/ToolCallID identify a tool-result message.
type ChatMessage struct {
	Role       string
	Content    string
	ToolCalls  []ToolCall
	ToolName   string
	ToolCallID string
}

// ToolCall is one tool invocation a provider asked for.
type ToolCall struct {
	ID        string
	Name      string
	Arguments map[string]any
}

// ChatResponse is one provider turn: streamed content (already delivered via
// the onToken callback and repeated here in full) and, instead of a final
// answer, zero or more requested tool calls.
type ChatResponse struct {
	Content   string
	ToolCalls []ToolCall
}

// Provider answers one chat turn, streaming content through onToken as it
// arrives. onToken may be called zero or more times before Chat returns.
type Provider interface {
	Name() string
	Chat(ctx context.Context, history []ChatMessage, onToken func(string)) (ChatResponse, error)
}

// toolDefs describes the fixed tool catalog offered to providers that
// support tool calling (SPEC-external convention; the ollama provider turns
// this into its JSON tool schema).
type toolSpec struct {
	Name        string
	Description string
	Params      []string // ordered argument names, for the JSON schema
}

var toolCatalog = []toolSpec{
	{Name: "list_files", Description: "list files under a directory of the workspace; path is relative to the workspace root (\".\" is the root)", Params: []string{"path"}},
	{Name: "read_file", Description: "read a file; path is relative to the workspace root, e.g. \"notes.txt\"", Params: []string{"path"}},
	{Name: "search", Description: "search for a substring across workspace files", Params: []string{"text"}},
	{Name: "edit_file", Description: "replace one exact, unique occurrence of old_string with new_string in a file (path relative to the workspace root); needs the operator's approval", Params: []string{"path", "old_string", "new_string"}},
	{Name: "run_command", Description: "run a command as an argv array (no shell) with the workspace root as working directory; relative paths only; needs the operator's approval", Params: []string{"argv"}},
}
