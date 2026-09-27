package main

// The types below mirror the shape of the JSON store the view (agent.tui)
// reads through {path} bindings and <list each=...>. Agent keeps its own
// copy in memory (see engine.go) and pushes it to the UI with ui.Set
// whenever it changes; the view never computes anything itself.

// message is one transcript row: a user prompt, an assistant reply (built
// up token by token while streaming), or in --headless --json mode a status
// note is not stored here — those go straight to the event stream.
type message struct {
	ID   string `json:"id"`
	Role string `json:"role"`
	Text string `json:"text"`
}

// activityEntry is one line in the side panel's "tools" list: one call.
type activityEntry struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// fileEntry is one line in the side panel's "files" list: one path a tool
// touched (read, searched, or edited).
type fileEntry struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}

// approvalState backs the approval modal: open/tool/preview.
type approvalState struct {
	Open    bool   `json:"open"`
	Tool    string `json:"tool"`
	Preview string `json:"preview"`
}
