package main

// tuimark mcp (SPEC v0.3b §15.8) is a Model Context Protocol server on
// stdin/stdout, stdlib only, that gives coding agents the §22 authoring
// loop: validate, dump (at 80 and 120 columns), and play (with --styles and
// --theme), plus inspect and the agent briefing. It is modeled on the
// owner's stdlib MCP server in glyphrun (internal/mcp/protocol.go,
// server.go), not imported from it. Every tool call runs the same code
// path as the matching CLI command, on the command line the SPEC's fixed
// mapping builds (cmdValidate, cmdDump, cmdInspect, cmdAgents, and
// runPlay, cmdPlay's body; tuimark_play hands runPlay its steps as a list
// instead of --input, so a step string with an embedded space needs no
// --script workaround: play.ParseStep never splits its token on spaces,
// so each `steps` element, verbatim, becomes one Step whose Raw a
// PlayFrame reports as `input` — see the SPEC's "steps with spaces" rule),
// so results are byte for byte what the CLI itself would print (MUST 13,
// §15.2). mcp reads no file it would not otherwise read and no
// environment variable, and it writes no file (§15.8 "Files").

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/abdul-hamid-achik/tuimark/internal/play"
)

// cmdMCP is `tuimark mcp`: it takes no arguments and no flags (SPEC
// §15.1), and serves the protocol on stdin/stdout until stdin reaches EOF
// (exit 0) or a read/write fails (exit 1).
func (c *cli) cmdMCP(args []string) int {
	fs := c.newFlags("mcp")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return 1
	}
	if len(pos) != 0 {
		return c.fail(fmt.Errorf("tuimark mcp: takes no arguments"))
	}
	// A client that closes its end of stdout must end the server with exit
	// 1 and a "tuimark: " line (§15.8 "End"), not with the SIGPIPE death
	// the Go runtime gives a broken pipe on fd 1 by default: with SIGPIPE
	// ignored, the write returns EPIPE instead.
	signal.Ignore(syscall.SIGPIPE)
	return runMCPServer(os.Stdin, c.stdout, c.stderr)
}

// --- JSON-RPC framing ---------------------------------------------------

// mcpProtocolVersions are the protocolVersion values `initialize` accepts
// verbatim, in the order tried against the client's requested version;
// anything else gets the first entry (SPEC §15.8: "the version the client
// asked for when it is one of ..., else 2025-11-25").
var mcpProtocolVersions = []string{"2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"}

type rpcErrInfo struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResp struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcErrInfo     `json:"error,omitempty"`
}

// runMCPServer reads one JSON-RPC message per line from in, dispatches
// it, and writes one JSON-RPC response per line to out, until in reaches
// EOF (return 0) or a read or write fails (return 1, with a "tuimark: "
// line on errOut, SPEC §15.8 "End").
func runMCPServer(in io.Reader, out io.Writer, errOut io.Writer) int {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 64*1024), 64*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		resp, answer := mcpHandleLine(line)
		if !answer {
			continue // a notification (no "id"): no reply, whatever its method
		}
		data, err := json.Marshal(resp)
		if err != nil {
			fmt.Fprintln(errOut, "tuimark:", err)
			return 1
		}
		data = append(data, '\n')
		if _, err := out.Write(data); err != nil {
			fmt.Fprintln(errOut, "tuimark:", err)
			return 1
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintln(errOut, "tuimark:", err)
		return 1
	}
	return 0
}

// mcpHandleLine decodes and answers one line (SPEC §15.8 "Protocol"):
//   - not JSON at all: -32700, id null.
//   - JSON but not a request object (a batch array, a string, an object
//     without "jsonrpc": "2.0" or without a string "method"): -32600,
//     with its id when it has one, else null.
//   - a well-formed request object with no "id" member: a notification,
//     answer=false, whatever its method.
//   - otherwise: dispatched by method, answer=true.
func mcpHandleLine(line []byte) (resp rpcResp, answer bool) {
	var probe any
	if err := json.Unmarshal(line, &probe); err != nil {
		return rpcResp{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcErrInfo{Code: -32700, Message: "parse error"}}, true
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(line, &members); err != nil {
		return rpcResp{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcErrInfo{Code: -32600, Message: "invalid request"}}, true
	}
	idRaw, hasID := members["id"]
	errID := json.RawMessage("null")
	if hasID {
		errID = idRaw
	}
	jsonrpcRaw, hasJSONRPC := members["jsonrpc"]
	var jsonrpcVal string
	validJSONRPC := hasJSONRPC && json.Unmarshal(jsonrpcRaw, &jsonrpcVal) == nil && jsonrpcVal == "2.0"
	methodRaw, hasMethod := members["method"]
	var methodVal string
	validMethod := hasMethod && json.Unmarshal(methodRaw, &methodVal) == nil
	if !validJSONRPC || !validMethod {
		return rpcResp{JSONRPC: "2.0", ID: errID, Error: &rpcErrInfo{Code: -32600, Message: "invalid request"}}, true
	}
	if !hasID {
		return rpcResp{}, false
	}
	result, rpcErr := mcpDispatch(methodVal, members["params"])
	if rpcErr != nil {
		return rpcResp{JSONRPC: "2.0", ID: idRaw, Error: rpcErr}, true
	}
	return rpcResp{JSONRPC: "2.0", ID: idRaw, Result: result}, true
}

// mcpDispatch answers the four methods the server knows (SPEC §15.8):
// initialize never fails on the version, ping is {}, tools/list has no
// pagination, tools/call is handled below. Every other method — including
// server/discover, so a client that tries it first falls back to
// initialize — is -32601.
func mcpDispatch(method string, params json.RawMessage) (any, *rpcErrInfo) {
	switch method {
	case "initialize":
		return mcpInitialize(params), nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": mcpToolDefs()}, nil
	case "tools/call":
		return mcpToolsCall(params)
	default:
		return nil, &rpcErrInfo{Code: -32601, Message: "method not found: " + method}
	}
}

func mcpInitialize(params json.RawMessage) map[string]any {
	var p struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if len(params) > 0 {
		_ = json.Unmarshal(params, &p)
	}
	chosen := mcpProtocolVersions[0]
	for _, v := range mcpProtocolVersions {
		if p.ProtocolVersion == v {
			chosen = v
			break
		}
	}
	return map[string]any{
		"protocolVersion": chosen,
		"capabilities": map[string]any{
			"tools": map[string]any{"listChanged": false},
		},
		"serverInfo": map[string]any{
			"name":    "tuimark",
			"version": version, // the bare runtime version, not versionLine()
		},
	}
}

// --- tools ---------------------------------------------------------------

// mcpProp is one JSON Schema property of a tool's inputSchema: its JSON
// type, one of "string", "integer", "boolean", "array" (of strings).
type mcpProp struct {
	Name string
	Type string
}

// mcpTool is one entry of the closed tool list (SPEC §15.8 "Tools"): the
// §22 Loop (validate, dump, play) plus inspect and the briefing. fmt and
// test are left out (they write files); ir, preview, host, and version
// are left out for the reasons the SPEC gives.
type mcpTool struct {
	Name        string
	Description string
	Required    []string
	Props       []mcpProp
}

var mcpTools = []mcpTool{
	{
		Name:        "tuimark_validate",
		Description: "Runs `tuimark validate` on a Tuimark document and returns its diagnostics and catalog as JSON.",
		Required:    []string{"file"},
		Props: []mcpProp{
			{"file", "string"}, {"data", "string"}, {"theme", "string"},
			{"strict", "boolean"}, {"catalog", "string"},
		},
	},
	{
		Name:        "tuimark_dump",
		Description: "Runs `tuimark dump` on a Tuimark document and returns the rendered frame as JSON.",
		Required:    []string{"file"},
		Props: []mcpProp{
			{"file", "string"}, {"data", "string"}, {"cols", "integer"}, {"rows", "integer"},
			{"theme", "string"}, {"styles", "boolean"}, {"cells", "boolean"}, {"strict", "boolean"},
		},
	},
	{
		Name:        "tuimark_play",
		Description: "Runs `tuimark play` to replay steps against a Tuimark document and returns the resulting frame and events as JSON.",
		Required:    []string{"file"},
		Props: []mcpProp{
			{"file", "string"}, {"steps", "array"}, {"data", "string"}, {"cols", "integer"}, {"rows", "integer"},
			{"theme", "string"}, {"styles", "boolean"}, {"cells", "boolean"}, {"frames", "boolean"}, {"strict", "boolean"},
		},
	},
	{
		Name:        "tuimark_inspect",
		Description: "Runs `tuimark inspect` on a Tuimark document and explains one node as JSON.",
		Required:    []string{"file"},
		Props: []mcpProp{
			{"file", "string"}, {"at", "string"}, {"id", "string"}, {"data", "string"},
			{"cols", "integer"}, {"rows", "integer"}, {"theme", "string"}, {"strict", "boolean"},
		},
	},
	{
		Name:        "tuimark_agents",
		Description: "Runs `tuimark agents` and returns the AGENTS.md generated from the catalog.",
	},
}

func toolByName(name string) *mcpTool {
	for i := range mcpTools {
		if mcpTools[i].Name == name {
			return &mcpTools[i]
		}
	}
	return nil
}

func mcpToolDefs() []map[string]any {
	defs := make([]map[string]any, 0, len(mcpTools))
	for _, t := range mcpTools {
		defs = append(defs, map[string]any{
			"name":        t.Name,
			"description": t.Description,
			"inputSchema": mcpSchema(t),
			"annotations": map[string]any{"readOnlyHint": true},
		})
	}
	return defs
}

// mcpSchema builds one tool's inputSchema (SPEC §15.8 "inputSchema"):
// {"type":"object","properties":{...},"required":[...],"additionalProperties":false},
// with the property schemas the SPEC lists; tuimark_agents (no
// properties) has no "required" member, matching the SPEC's literal
// schema for it.
func mcpSchema(t mcpTool) map[string]any {
	if len(t.Props) == 0 {
		return map[string]any{
			"type":                 "object",
			"properties":           map[string]any{},
			"additionalProperties": false,
		}
	}
	props := map[string]any{}
	for _, p := range t.Props {
		switch p.Type {
		case "string":
			s := map[string]any{"type": "string"}
			if p.Name == "theme" {
				s["enum"] = []string{"dark", "light"}
			}
			props[p.Name] = s
		case "integer":
			props[p.Name] = map[string]any{"type": "integer", "minimum": 1, "maximum": 1000}
		case "boolean":
			props[p.Name] = map[string]any{"type": "boolean"}
		case "array":
			props[p.Name] = map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
		}
	}
	req := t.Required
	if req == nil {
		req = []string{}
	}
	return map[string]any{
		"type":                 "object",
		"properties":           props,
		"required":             req,
		"additionalProperties": false,
	}
}

// toolCallParamsOuter is the shape of a tools/call request's params (SPEC
// §15.8): {"name": "...", "arguments": {...}}.
func mcpToolsCall(paramsRaw json.RawMessage) (any, *rpcErrInfo) {
	if len(paramsRaw) == 0 {
		return nil, &rpcErrInfo{Code: -32602, Message: "params must be an object"}
	}
	var outer map[string]json.RawMessage
	if err := json.Unmarshal(paramsRaw, &outer); err != nil {
		return nil, &rpcErrInfo{Code: -32602, Message: "params must be an object"}
	}
	nameRaw, hasName := outer["name"]
	if !hasName {
		return nil, &rpcErrInfo{Code: -32602, Message: `missing "name"`}
	}
	var name string
	if err := json.Unmarshal(nameRaw, &name); err != nil {
		return nil, &rpcErrInfo{Code: -32602, Message: `"name" must be a string`}
	}
	tool := toolByName(name)
	if tool == nil {
		return nil, &rpcErrInfo{Code: -32602, Message: "unknown tool: " + name}
	}
	var order []string
	args := map[string]json.RawMessage{}
	if argRaw, ok := outer["arguments"]; ok {
		o, m, err := decodeOrderedArgs(argRaw)
		if err != nil {
			return nil, &rpcErrInfo{Code: -32602, Message: "arguments must be an object"}
		}
		order, args = o, m
	}
	return callMCPTool(tool, args, order), nil
}

// decodeOrderedArgs decodes a JSON object, keeping its members' document
// order (so an argument error names the first offending key in the order
// the caller wrote it, deterministically — never a Go map's iteration
// order). A duplicate key keeps the JSON rule (the last value wins, as
// plain json.Unmarshal into the map already gives).
func decodeOrderedArgs(raw json.RawMessage) ([]string, map[string]json.RawMessage, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, nil, err
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '{' {
		return nil, nil, fmt.Errorf("want a JSON object")
	}
	var order []string
	m := map[string]json.RawMessage{}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, nil, err
		}
		key := keyTok.(string)
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return nil, nil, err
		}
		order = append(order, key)
		m[key] = v
	}
	if _, err := dec.Token(); err != nil { // consume '}'
		return nil, nil, err
	}
	return order, m, nil
}

// callMCPTool checks arg, then invokes the CLI code path (SPEC §15.8
// "Argument errors ... checked before the command runs").
func callMCPTool(tool *mcpTool, args map[string]json.RawMessage, order []string) map[string]any {
	if errText, bad := checkMCPArgs(tool, order, args); bad {
		return mcpToolText(failText(errText), true)
	}
	switch tool.Name {
	case "tuimark_validate":
		file, _ := mcpStrArg(args, "file")
		return invokeValidateTool(file, args)
	case "tuimark_dump":
		file, _ := mcpStrArg(args, "file")
		return invokeDumpTool(file, args)
	case "tuimark_play":
		file, _ := mcpStrArg(args, "file")
		return invokePlayTool(file, args)
	case "tuimark_inspect":
		file, _ := mcpStrArg(args, "file")
		return invokeInspectTool(file, args)
	case "tuimark_agents":
		return invokeAgentsTool()
	}
	return mcpToolText(failText(fmt.Sprintf("unimplemented tool %q", tool.Name)), true) // unreachable: tool came from toolByName
}

// checkMCPArgs is SPEC §15.8's three argument checks, in this order: an
// argument the tool does not take, a required argument that is missing,
// and a value of the wrong JSON type — each over order, the JSON object's
// own member order, so the result never depends on Go map iteration.
func checkMCPArgs(tool *mcpTool, order []string, args map[string]json.RawMessage) (string, bool) {
	propType := map[string]string{}
	for _, p := range tool.Props {
		propType[p.Name] = p.Type
	}
	for _, k := range order {
		if _, known := propType[k]; !known {
			return fmt.Sprintf("unknown argument %q", k), true
		}
	}
	for _, r := range tool.Required {
		if _, ok := args[r]; !ok {
			return fmt.Sprintf("missing argument %q", r), true
		}
	}
	for _, k := range order {
		if !mcpTypeMatches(args[k], propType[k]) {
			return fmt.Sprintf("%s: want %s", k, mcpTypeLabel(propType[k])), true
		}
	}
	return "", false
}

// mcpTypeMatches reports whether raw's JSON type matches kind ("string",
// "integer", "boolean", or "array" of strings): a JSON null never
// matches, since none of the schema's types accept it, and a JSON number
// with a fraction is not an "integer" (SPEC §15.8), judged exactly on its
// digits (jsonIntText), so 1e3 and 1.0 are integers and
// 1.0000000000000001 is not.
func mcpTypeMatches(raw json.RawMessage, kind string) bool {
	if string(bytes.TrimSpace(raw)) == "null" {
		return false
	}
	switch kind {
	case "string":
		var s string
		return json.Unmarshal(raw, &s) == nil
	case "boolean":
		var b bool
		return json.Unmarshal(raw, &b) == nil
	case "integer":
		_, ok := jsonIntText(raw)
		return ok
	case "array":
		var arr []any
		if err := json.Unmarshal(raw, &arr); err != nil {
			return false
		}
		for _, e := range arr {
			if _, ok := e.(string); !ok {
				return false
			}
		}
		return true
	}
	return false
}

func mcpTypeLabel(kind string) string {
	if kind == "array" {
		return "array of strings"
	}
	return kind
}

func mcpStrArg(args map[string]json.RawMessage, name string) (string, bool) {
	raw, ok := args[name]
	if !ok {
		return "", false
	}
	var s string
	_ = json.Unmarshal(raw, &s)
	return s, true
}

func mcpBoolArg(args map[string]json.RawMessage, name string) (bool, bool) {
	raw, ok := args[name]
	if !ok {
		return false, false
	}
	var b bool
	_ = json.Unmarshal(raw, &b)
	return b, true
}

// mcpIntArg returns an integer argument as the flag value the command
// line gets: the integer's exact decimal text (jsonIntText), never a
// float64 conversion, so the flag parser sees the same digits a shell
// would pass and an out-of-range value gets the CLI's own message
// (`invalid value "99999999999999999999" for flag -cols: value out of
// range`). checkMCPArgs has already rejected a value with a fraction.
func mcpIntArg(args map[string]json.RawMessage, name string) (string, bool) {
	raw, ok := args[name]
	if !ok {
		return "", false
	}
	text, _ := jsonIntText(raw)
	return text, true
}

func mcpStepsArg(args map[string]json.RawMessage, name string) ([]string, bool) {
	raw, ok := args[name]
	if !ok {
		return nil, false
	}
	var arr []string
	_ = json.Unmarshal(raw, &arr)
	return arr, true
}

// failText is a tool result's isError text for a usage/I/O failure: the
// same "tuimark: MESSAGE\n" line cli.fail writes to stderr (SPEC §15.8:
// "its text is the command's stderr line"), built here directly since the
// caller has a message, not a *cli to fail() through.
func failText(msg string) string {
	return "tuimark: " + msg + "\n"
}

// mcpToolText is one tool result: a single text content item, isError set
// only when true (SPEC §15.8 "Results").
func mcpToolText(text string, isErr bool) map[string]any {
	res := map[string]any{"content": []map[string]any{{"type": "text", "text": text}}}
	if isErr {
		res["isError"] = true
	}
	return res
}

// runCapture runs one CLI command with a fresh *cli whose stdout/stderr
// are captured instead of the server's own (which is busy with JSON-RPC),
// exactly as it would run from a shell (SPEC §15.8: "calling the same
// code paths as the CLI commands").
func runCapture(fn func(*cli) int) (stdout, stderr string, code int) {
	var out, errw bytes.Buffer
	sub := &cli{stdout: &out, stderr: &errw}
	code = fn(sub)
	return out.String(), errw.String(), code
}

// mcpResultFromRun turns one command's captured run into a tool result:
// exit 0 or 2 is data (SPEC §15.8: "An exit 2 is data, not a failure"),
// exit 1 is isError with the stderr line.
func mcpResultFromRun(stdout, stderr string, code int) map[string]any {
	if code == 1 {
		return mcpToolText(stderr, true)
	}
	return mcpToolText(stdout, false)
}

func invokeValidateTool(file string, args map[string]json.RawMessage) map[string]any {
	argv := []string{file}
	if v, ok := mcpStrArg(args, "data"); ok {
		argv = append(argv, "--data", v)
	}
	if v, ok := mcpStrArg(args, "theme"); ok {
		argv = append(argv, "--theme", v)
	}
	if v, ok := mcpBoolArg(args, "strict"); ok && v {
		argv = append(argv, "--strict")
	}
	if v, ok := mcpStrArg(args, "catalog"); ok {
		argv = append(argv, "--catalog", v)
	}
	argv = append(argv, "--json")
	out, errw, code := runCapture(func(s *cli) int { return s.cmdValidate(argv) })
	return mcpResultFromRun(out, errw, code)
}

func invokeDumpTool(file string, args map[string]json.RawMessage) map[string]any {
	argv := []string{file}
	if v, ok := mcpStrArg(args, "data"); ok {
		argv = append(argv, "--data", v)
	}
	if v, ok := mcpIntArg(args, "cols"); ok {
		argv = append(argv, "--cols", v)
	}
	if v, ok := mcpIntArg(args, "rows"); ok {
		argv = append(argv, "--rows", v)
	}
	if v, ok := mcpStrArg(args, "theme"); ok {
		argv = append(argv, "--theme", v)
	}
	if v, ok := mcpBoolArg(args, "styles"); ok && v {
		argv = append(argv, "--styles")
	}
	if v, ok := mcpBoolArg(args, "cells"); ok && v {
		argv = append(argv, "--cells")
	}
	if v, ok := mcpBoolArg(args, "strict"); ok && v {
		argv = append(argv, "--strict")
	}
	argv = append(argv, "--format", "json")
	out, errw, code := runCapture(func(s *cli) int { return s.cmdDump(argv) })
	return mcpResultFromRun(out, errw, code)
}

func invokeInspectTool(file string, args map[string]json.RawMessage) map[string]any {
	argv := []string{file}
	if v, ok := mcpStrArg(args, "data"); ok {
		argv = append(argv, "--data", v)
	}
	if v, ok := mcpIntArg(args, "cols"); ok {
		argv = append(argv, "--cols", v)
	}
	if v, ok := mcpIntArg(args, "rows"); ok {
		argv = append(argv, "--rows", v)
	}
	if v, ok := mcpStrArg(args, "theme"); ok {
		argv = append(argv, "--theme", v)
	}
	if v, ok := mcpBoolArg(args, "strict"); ok && v {
		argv = append(argv, "--strict")
	}
	if v, ok := mcpStrArg(args, "at"); ok {
		argv = append(argv, "--at", v)
	}
	if v, ok := mcpStrArg(args, "id"); ok {
		argv = append(argv, "--id", v)
	}
	argv = append(argv, "--json")
	out, errw, code := runCapture(func(s *cli) int { return s.cmdInspect(argv) })
	return mcpResultFromRun(out, errw, code)
}

func invokeAgentsTool() map[string]any {
	out, errw, code := runCapture(func(s *cli) int { return s.cmdAgents(nil) })
	return mcpResultFromRun(out, errw, code)
}

// invokePlayTool runs tuimark play's own code (runPlay) on the command
// line the other tools build too (SPEC §15.8 "Arguments to flags"), so
// the file and every flag go through the CLI's flag parsing and checks,
// in the CLI's order, and a file that looks like a flag (-x.tui, --, -h)
// gives exactly what the command line gives. Only steps bypass argv:
// --input cannot carry a step that holds a space or is empty ("Steps with
// spaces"), so runPlay gets them as a list, one Step per element, parsed
// whole by play.ParseStep. Each Step's Raw is then the element verbatim,
// which is what a PlayFrame's "input" reports.
func invokePlayTool(file string, args map[string]json.RawMessage) map[string]any {
	argv := []string{file}
	if v, ok := mcpStrArg(args, "data"); ok {
		argv = append(argv, "--data", v)
	}
	if v, ok := mcpIntArg(args, "cols"); ok {
		argv = append(argv, "--cols", v)
	}
	if v, ok := mcpIntArg(args, "rows"); ok {
		argv = append(argv, "--rows", v)
	}
	if v, ok := mcpStrArg(args, "theme"); ok {
		argv = append(argv, "--theme", v)
	}
	for _, name := range []string{"strict", "styles", "cells", "frames"} {
		if v, ok := mcpBoolArg(args, name); ok && v {
			argv = append(argv, "--"+name)
		}
	}
	argv = append(argv, "--format", "json")
	steps, _ := mcpStepsArg(args, "steps")
	out, errw, code := runCapture(func(s *cli) int { return s.runPlay(argv, steps) })
	return mcpResultFromRun(out, errw, code)
}

// parsePlaySteps parses each of raw with play.ParseStep — never splitting
// on spaces, unlike --input's grammar — numbering and quoting a parse
// failure exactly as tuimark play's own --input parsing does ("step N
// (RAW): reason"), so the wrapped message is byte for byte the CLI's.
func parsePlaySteps(raw []string) ([]play.Step, error) {
	steps := make([]play.Step, 0, len(raw))
	for i, tok := range raw {
		st, err := play.ParseStep(tok)
		if err != nil {
			return nil, fmt.Errorf("step %d (%s): %v", i+1, tok, err)
		}
		steps = append(steps, st)
	}
	return steps, nil
}
