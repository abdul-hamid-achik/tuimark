package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

// SPEC v0.3b §21 test 109 ("tuimark mcp"): a Go test in cmd/tuimark runs
// the server over pipes. F and D are examples/monitor/studio.tui and
// examples/monitor/sample.json, relative to this package directory,
// exactly as the SPEC names them.
const (
	mcpFixtureFile = "../../examples/monitor/studio.tui"
	mcpDataFile    = "../../examples/monitor/sample.json"
)

// mcpTestServer drives runMCPServer over a real pair of io.Pipes (an
// in-process pipe, same synchronization contract as an OS pipe: a write
// blocks until read), so requests and replies are exchanged one line at a
// time as they would be over the server's real stdin/stdout.
type mcpTestServer struct {
	t       *testing.T
	inW     *io.PipeWriter
	scanner *bufio.Scanner
	errBuf  *bytes.Buffer
	done    chan int
}

func newMCPTestServer(t *testing.T) *mcpTestServer {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	errBuf := &bytes.Buffer{}
	done := make(chan int, 1)
	go func() {
		code := runMCPServer(inR, outW, errBuf)
		outW.Close()
		done <- code
	}()
	sc := bufio.NewScanner(outR)
	sc.Buffer(make([]byte, 64*1024), 64*1024*1024)
	return &mcpTestServer{t: t, inW: inW, scanner: sc, errBuf: errBuf, done: done}
}

// send writes one JSON-RPC request line, marshaled from v.
func (s *mcpTestServer) send(v any) {
	s.t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		s.t.Fatalf("marshal request: %v", err)
	}
	if _, err := s.inW.Write(append(b, '\n')); err != nil {
		s.t.Fatalf("write request: %v", err)
	}
}

// sendRaw writes one raw line (used for the not-JSON vector).
func (s *mcpTestServer) sendRaw(line string) {
	s.t.Helper()
	if _, err := s.inW.Write([]byte(line + "\n")); err != nil {
		s.t.Fatalf("write raw line: %v", err)
	}
}

// recv reads and decodes one JSON-RPC response line.
func (s *mcpTestServer) recv() map[string]any {
	s.t.Helper()
	if !s.scanner.Scan() {
		s.t.Fatalf("server closed before answering: %v", s.scanner.Err())
	}
	var m map[string]any
	if err := json.Unmarshal(s.scanner.Bytes(), &m); err != nil {
		s.t.Fatalf("response is not JSON: %v\n%s", err, s.scanner.Text())
	}
	return m
}

// closeAndWait closes stdin (EOF) and returns the server's exit code.
func (s *mcpTestServer) closeAndWait() int {
	s.t.Helper()
	s.inW.Close()
	return <-s.done
}

func mustMap(t *testing.T, v any, what string) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("%s: want an object, got %T (%v)", what, v, v)
	}
	return m
}

// toolCallResult sends one tools/call request and returns its (decoded)
// result object.
func (s *mcpTestServer) toolCallResult(id int, name string, args map[string]any) map[string]any {
	s.t.Helper()
	s.send(map[string]any{
		"jsonrpc": "2.0", "id": id, "method": "tools/call",
		"params": map[string]any{"name": name, "arguments": args},
	})
	resp := s.recv()
	if resp["error"] != nil {
		s.t.Fatalf("tools/call %s: unexpected JSON-RPC error: %v", name, resp["error"])
	}
	return mustMap(s.t, resp["result"], "result")
}

// toolText returns a successful tool result's one text content item, and
// fails the test if the result is an error result.
func toolText(t *testing.T, result map[string]any) string {
	t.Helper()
	if result["isError"] == true {
		content, _ := result["content"].([]any)
		var text string
		if len(content) > 0 {
			item := mustMap(t, content[0], "content[0]")
			text, _ = item["text"].(string)
		}
		t.Fatalf("unexpected isError result: %s", text)
	}
	content, ok := result["content"].([]any)
	if !ok || len(content) != 1 {
		t.Fatalf("want exactly one content item, got %#v", result["content"])
	}
	item := mustMap(t, content[0], "content[0]")
	if item["type"] != "text" {
		t.Fatalf(`content[0].type = %v, want "text"`, item["type"])
	}
	text, ok := item["text"].(string)
	if !ok {
		t.Fatalf("content[0].text is not a string: %#v", item["text"])
	}
	return text
}

// errorText returns an isError result's text, failing the test if the
// result is not an error result.
func errorText(t *testing.T, result map[string]any) string {
	t.Helper()
	if result["isError"] != true {
		t.Fatalf("want isError result, got %#v", result)
	}
	content, ok := result["content"].([]any)
	if !ok || len(content) != 1 {
		t.Fatalf("want exactly one content item, got %#v", result["content"])
	}
	item := mustMap(t, content[0], "content[0]")
	text, ok := item["text"].(string)
	if !ok {
		t.Fatalf("content[0].text is not a string: %#v", item["text"])
	}
	return text
}

func rpcErrorCode(t *testing.T, resp map[string]any) int {
	t.Helper()
	errObj := mustMap(t, resp["error"], "error")
	code, ok := errObj["code"].(float64)
	if !ok {
		t.Fatalf("error.code is not a number: %#v", errObj["code"])
	}
	return int(code)
}

// TestMCP is SPEC v0.3b §21 test 109 ("tuimark mcp"): every vector the
// SPEC lists, run over a real pair of pipes.
func TestMCP(t *testing.T) {
	srv := newMCPTestServer(t)

	// initialize: the requested protocolVersion is echoed back when it is
	// one of the four known versions, else the server answers
	// "2025-11-25". serverInfo.version is the bare runtime version (never
	// the "tuimark VERSION ..." line versionLine() prints).
	t.Run("initialize known version", func(t *testing.T) {
		srv.send(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{"protocolVersion": "2025-06-18"}})
		resp := srv.recv()
		result := mustMap(t, resp["result"], "result")
		if result["protocolVersion"] != "2025-06-18" {
			t.Errorf("protocolVersion = %v, want 2025-06-18", result["protocolVersion"])
		}
		si := mustMap(t, result["serverInfo"], "serverInfo")
		if si["name"] != "tuimark" {
			t.Errorf("serverInfo.name = %v, want tuimark", si["name"])
		}
		v, _ := si["version"].(string)
		if v == "" || strings.HasPrefix(v, "tuimark ") {
			t.Errorf("serverInfo.version = %q, want the bare version", v)
		}
		if v != version {
			t.Errorf("serverInfo.version = %q, want %q (the package's bare version)", v, version)
		}
	})

	t.Run("initialize unknown version falls back", func(t *testing.T) {
		srv.send(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "initialize", "params": map[string]any{"protocolVersion": "2099-01-01"}})
		resp := srv.recv()
		result := mustMap(t, resp["result"], "result")
		if result["protocolVersion"] != "2025-11-25" {
			t.Errorf("protocolVersion = %v, want 2025-11-25", result["protocolVersion"])
		}
	})

	t.Run("server/discover is method not found", func(t *testing.T) {
		srv.send(map[string]any{"jsonrpc": "2.0", "id": 3, "method": "server/discover"})
		resp := srv.recv()
		if code := rpcErrorCode(t, resp); code != -32601 {
			t.Errorf("server/discover error code = %d, want -32601", code)
		}
	})

	// tools/list: the five tools of §15.8, each with a description, its
	// inputSchema, and annotations.readOnlyHint: true.
	t.Run("tools/list", func(t *testing.T) {
		srv.send(map[string]any{"jsonrpc": "2.0", "id": 4, "method": "tools/list"})
		resp := srv.recv()
		result := mustMap(t, resp["result"], "result")
		toolsAny, ok := result["tools"].([]any)
		if !ok {
			t.Fatalf("tools/list result.tools is not an array: %#v", result["tools"])
		}
		wantNames := map[string]bool{
			"tuimark_validate": false, "tuimark_dump": false, "tuimark_play": false,
			"tuimark_inspect": false, "tuimark_agents": false,
		}
		if len(toolsAny) != len(wantNames) {
			t.Fatalf("tools/list returned %d tools, want %d", len(toolsAny), len(wantNames))
		}
		for _, ta := range toolsAny {
			tool := mustMap(t, ta, "tool")
			name, _ := tool["name"].(string)
			if _, known := wantNames[name]; !known {
				t.Errorf("unexpected tool name %q", name)
				continue
			}
			wantNames[name] = true
			if desc, _ := tool["description"].(string); desc == "" {
				t.Errorf("%s: empty description", name)
			}
			schema := mustMap(t, tool["inputSchema"], name+".inputSchema")
			if schema["type"] != "object" {
				t.Errorf("%s: inputSchema.type = %v, want object", name, schema["type"])
			}
			ann := mustMap(t, tool["annotations"], name+".annotations")
			if ann["readOnlyHint"] != true {
				t.Errorf("%s: annotations.readOnlyHint = %v, want true", name, ann["readOnlyHint"])
			}
		}
		for name, seen := range wantNames {
			if !seen {
				t.Errorf("tools/list is missing %q", name)
			}
		}
	})

	// tuimark_validate {file: F, data: D} equals `tuimark validate F
	// --data D --json` byte for byte: ok true, no diagnostics, text ends
	// in "}\n".
	t.Run("tuimark_validate", func(t *testing.T) {
		result := srv.toolCallResult(5, "tuimark_validate", map[string]any{"file": mcpFixtureFile, "data": mcpDataFile})
		text := toolText(t, result)
		_, wantOut, wantErr := runCLI("validate", mcpFixtureFile, "--data", mcpDataFile, "--json")
		if text != wantOut {
			t.Fatalf("tuimark_validate text differs from `tuimark validate --json`\nmcp:\n%s\ncli:\n%s", text, wantOut)
		}
		if !strings.HasSuffix(text, "}\n") {
			t.Errorf("tuimark_validate text does not end in }\\n: %q", tail(text, 20))
		}
		var v struct {
			OK          bool  `json:"ok"`
			Diagnostics []any `json:"diagnostics"`
		}
		if err := json.Unmarshal([]byte(text), &v); err != nil {
			t.Fatalf("tuimark_validate text is not the validate JSON shape: %v", err)
		}
		if !v.OK {
			t.Errorf("tuimark_validate: ok = false, want true (cli stderr: %s)", wantErr)
		}
		if len(v.Diagnostics) != 0 {
			t.Errorf("tuimark_validate: diagnostics = %v, want none", v.Diagnostics)
		}
	})

	// tuimark_dump {file: F, data: D} equals `tuimark dump F --data D
	// --format json`.
	t.Run("tuimark_dump with data", func(t *testing.T) {
		result := srv.toolCallResult(6, "tuimark_dump", map[string]any{"file": mcpFixtureFile, "data": mcpDataFile})
		text := toolText(t, result)
		_, wantOut, _ := runCLI("dump", mcpFixtureFile, "--data", mcpDataFile, "--format", "json")
		if text != wantOut {
			t.Fatalf("tuimark_dump text differs from `tuimark dump --format json`")
		}
	})

	// tuimark_dump {file: F} without data (a B001 error; the CLI exits
	// 2) is a normal result, not isError, whose JSON has "ok": false.
	t.Run("tuimark_dump without data is data, not isError", func(t *testing.T) {
		result := srv.toolCallResult(7, "tuimark_dump", map[string]any{"file": mcpFixtureFile})
		text := toolText(t, result)
		var d struct {
			OK bool `json:"ok"`
		}
		if err := json.Unmarshal([]byte(text), &d); err != nil {
			t.Fatalf("tuimark_dump (no data) text is not JSON: %v", err)
		}
		if d.OK {
			t.Errorf("tuimark_dump (no data): ok = true, want false (B001)")
		}
	})

	// tuimark_play {file: F, data: D, steps: ["tab", "down"]} equals
	// play F --data D --input "tab down" --format json: focus is
	// "settings-list", and events is the one view_changed event.
	t.Run("tuimark_play steps", func(t *testing.T) {
		result := srv.toolCallResult(8, "tuimark_play", map[string]any{
			"file": mcpFixtureFile, "data": mcpDataFile, "steps": []any{"tab", "down"},
		})
		text := toolText(t, result)
		_, wantOut, _ := runCLI("play", mcpFixtureFile, "--data", mcpDataFile, "--input", "tab down", "--format", "json")
		if text != wantOut {
			t.Fatalf("tuimark_play text differs from `tuimark play --input \"tab down\" --format json`")
		}
		var p struct {
			Focus  string `json:"focus"`
			Events []struct {
				Step   int            `json:"step"`
				Action string         `json:"action"`
				Source string         `json:"source"`
				Keys   map[string]any `json:"keys"`
				Value  any            `json:"value"`
			} `json:"events"`
		}
		if err := json.Unmarshal([]byte(text), &p); err != nil {
			t.Fatalf("tuimark_play text is not JSON: %v", err)
		}
		if p.Focus != "settings-list" {
			t.Errorf("tuimark_play: focus = %q, want settings-list", p.Focus)
		}
		if len(p.Events) != 1 || p.Events[0].Step != 1 || p.Events[0].Action != "view_changed" ||
			p.Events[0].Source != "nav" || p.Events[0].Value != "settings" {
			t.Errorf("tuimark_play: events = %+v, want one view_changed event on nav with value settings", p.Events)
		}
	})

	// With "frames": true and steps that need embedded spaces
	// ("text:foo bar"), the frames' "input" members are the step
	// strings as given ("", "focus:#filter", "text:foo bar"), never a
	// script line — play.ParseStep never splits its token on spaces, so
	// --input could not carry this call at all.
	t.Run("tuimark_play frames with a step containing a space", func(t *testing.T) {
		result := srv.toolCallResult(9, "tuimark_play", map[string]any{
			"file": mcpFixtureFile, "data": mcpDataFile,
			"steps": []any{"focus:#filter", "text:foo bar"}, "frames": true,
		})
		text := toolText(t, result)
		var p struct {
			Frames []struct {
				Step  int    `json:"step"`
				Input string `json:"input"`
			} `json:"frames"`
		}
		if err := json.Unmarshal([]byte(text), &p); err != nil {
			t.Fatalf("tuimark_play (frames) text is not JSON: %v", err)
		}
		wantInputs := []string{"", "focus:#filter", "text:foo bar"}
		if len(p.Frames) != len(wantInputs) {
			t.Fatalf("tuimark_play (frames): %d frames, want %d", len(p.Frames), len(wantInputs))
		}
		for i, want := range wantInputs {
			if p.Frames[i].Input != want {
				t.Errorf("tuimark_play (frames): frame %d input = %q, want %q", i, p.Frames[i].Input, want)
			}
		}
	})

	// tuimark_inspect {file: F, data: D, at: "0,0"} equals inspect F
	// --data D --at 0,0 --json.
	t.Run("tuimark_inspect", func(t *testing.T) {
		result := srv.toolCallResult(10, "tuimark_inspect", map[string]any{"file": mcpFixtureFile, "data": mcpDataFile, "at": "0,0"})
		text := toolText(t, result)
		_, wantOut, _ := runCLI("inspect", mcpFixtureFile, "--data", mcpDataFile, "--at", "0,0", "--json")
		if text != wantOut {
			t.Fatalf("tuimark_inspect text differs from `tuimark inspect --at 0,0 --json`")
		}
	})

	// tuimark_agents {} equals tuimark agents, whose text ends in ".\n".
	t.Run("tuimark_agents", func(t *testing.T) {
		result := srv.toolCallResult(11, "tuimark_agents", map[string]any{})
		text := toolText(t, result)
		_, wantOut, _ := runCLI("agents")
		if text != wantOut {
			t.Fatalf("tuimark_agents text differs from `tuimark agents`")
		}
		if !strings.HasSuffix(text, ".\n") {
			t.Errorf("tuimark_agents text does not end in .\\n: %q", tail(text, 20))
		}
	})

	// isError results: a missing file, a wrong-typed argument, and an
	// argument the tool does not take.
	t.Run("isError: missing file", func(t *testing.T) {
		result := srv.toolCallResult(12, "tuimark_dump", map[string]any{"file": "nope.tui"})
		text := errorText(t, result)
		if text != "tuimark: open nope.tui: no such file or directory\n" {
			t.Errorf("text = %q, want the open error", text)
		}
	})

	t.Run("isError: wrong argument type", func(t *testing.T) {
		result := srv.toolCallResult(13, "tuimark_dump", map[string]any{"file": mcpFixtureFile, "cols": "abc"})
		text := errorText(t, result)
		if text != "tuimark: cols: want integer\n" {
			t.Errorf("text = %q, want the want-integer error", text)
		}
	})

	t.Run("isError: unknown argument", func(t *testing.T) {
		result := srv.toolCallResult(14, "tuimark_play", map[string]any{"file": mcpFixtureFile, "input": "tab"})
		text := errorText(t, result)
		if text != `tuimark: unknown argument "input"`+"\n" {
			t.Errorf("text = %q, want the unknown-argument error", text)
		}
	})

	// An unknown tool name gets -32602, an unknown method -32601, and a
	// line that is not JSON -32700 with "id": null.
	t.Run("unknown tool is -32602", func(t *testing.T) {
		srv.send(map[string]any{"jsonrpc": "2.0", "id": 15, "method": "tools/call", "params": map[string]any{"name": "nope", "arguments": map[string]any{}}})
		resp := srv.recv()
		if code := rpcErrorCode(t, resp); code != -32602 {
			t.Errorf("unknown tool error code = %d, want -32602", code)
		}
	})

	t.Run("unknown method is -32601", func(t *testing.T) {
		srv.send(map[string]any{"jsonrpc": "2.0", "id": 16, "method": "not_a_real_method"})
		resp := srv.recv()
		if code := rpcErrorCode(t, resp); code != -32601 {
			t.Errorf("unknown method error code = %d, want -32601", code)
		}
	})

	t.Run("not JSON is -32700 with id null", func(t *testing.T) {
		srv.sendRaw("this is not json")
		resp := srv.recv()
		if code := rpcErrorCode(t, resp); code != -32700 {
			t.Errorf("parse error code = %d, want -32700", code)
		}
		id, hasID := resp["id"]
		if !hasID || id != nil {
			t.Errorf(`id = %#v, want null`, resp["id"])
		}
	})

	// Closing stdin ends the server with exit status 0.
	t.Run("closing stdin exits 0", func(t *testing.T) {
		if code := srv.closeAndWait(); code != 0 {
			t.Errorf("exit code = %d, want 0 (stderr: %s)", code, srv.errBuf.String())
		}
	})
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
