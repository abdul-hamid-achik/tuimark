package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// SPEC v0.4d §21 test 303's play parts and §15.4/§15.5/§15.8's append
// step, through the CLI: --input, --script, the manifest, and tuimark_play.

type d5Play struct {
	Dump struct {
		Grid []string `json:"grid"`
	}
	Grid   []string `json:"grid"`
	Events []struct {
		Step   int    `json:"step"`
		Action string `json:"action"`
	} `json:"events"`
}

func d5PlayJSON(t *testing.T, args ...string) (d5Play, int, string) {
	t.Helper()
	code, out, errw := runCLI(append([]string{"play", d5Msgs, "--data", d5Data, "--format", "json"}, args...)...)
	var p d5Play
	if code == 0 {
		if err := json.Unmarshal([]byte(out), &p); err != nil {
			t.Fatalf("play output: %v\n%s", err, out)
		}
	}
	return p, code, errw
}

// Test 303: play --input 'append:messages={"text":"hi"}' and the script
// step {"append": {"path": "messages", "value": {"text": "hi"}}} append.
func TestD5PlayAppendStep303(t *testing.T) {
	p, code, errw := d5PlayJSON(t, "--input", `append:messages={"text":"hi"} append:live.text="!"`)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errw)
	}
	g := strings.Join(p.Grid, "\n")
	if !strings.Contains(g, "msg-hi") || !strings.Contains(g, "live=hello!") {
		t.Errorf("--input: the grid does not show the appends:\n%s", g)
	}

	script := filepath.Join(t.TempDir(), "s.ndjson")
	writeFile(t, script, `{"append": {"path": "messages", "value": {"text": "hi"}}}`+"\n"+
		`{"append": {"value": "!", "path": "live.text"}}`+"\n")
	p, code, errw = d5PlayJSON(t, "--script", script)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errw)
	}
	g = strings.Join(p.Grid, "\n")
	if !strings.Contains(g, "msg-hi") || !strings.Contains(g, "live=hello!") {
		t.Errorf("--script: the grid does not show the appends:\n%s", g)
	}
	// A number keeps its JSON type through the script step, as for set.
	script = filepath.Join(t.TempDir(), "s.ndjson")
	writeFile(t, script, `{"append": {"path": "messages", "value": 7}}`+"\n")
	if _, code, errw := d5PlayJSON(t, "--script", script); code != 0 {
		t.Errorf("a number element: exit %d: %s", code, errw)
	}
}

// Test 303: a failing append step is a usage error with the reason of
// §18.3: step N (INPUT): REASON on stderr, nothing on stdout, exit 1.
func TestD5PlayAppendErrors303(t *testing.T) {
	cases := []struct{ input, want string }{
		{`append:nope="x"`, `tuimark: play: step 1 (append:nope="x"): append "nope": "nope" is missing`},
		{`append:n=1`, `tuimark: play: step 1 (append:n=1): append "n": "n" holds a number, not a string or an array`},
		{`append:busy=1`, `tuimark: play: step 1 (append:busy=1): append "busy": "busy" holds a boolean, not a string or an array`},
		{`append:live.text=5`, `tuimark: play: step 1 (append:live.text=5): append "live.text": "live.text" holds a string, and the value is a number, not a string`},
		{`append:=1`, `tuimark: play: step 1 (append:=1): append "": the root is not a string or an array`},
		{`append:@focus="x"`, `tuimark: play: step 1 (append:@focus="x"): append "@focus": "@focus" is a reserved path`},
		{`append:a..b="x"`, `tuimark: play: step 1 (append:a..b="x"): tuimark: bad path "a..b"`},
		{`append:messages`, `tuimark: play: step 1 (append:messages): append: wants PATH=JSON`},
		{`append:messages={`, `tuimark: play: step 1 (append:messages={): bad JSON value:`},
	}
	for _, c := range cases {
		code, out, errw := runCLI("play", d5Msgs, "--data", d5Data, "--input", c.input)
		if code != 1 || out != "" || !strings.HasPrefix(errw, c.want) || strings.Count(errw, "\n") != 1 {
			t.Errorf("%s: exit %d stdout %q stderr %q, want %q", c.input, code, out, errw, c.want)
		}
	}
	// The usage error stops the call after the steps before it.
	code, out, errw := runCLI("play", d5Msgs, "--data", d5Data, "--input", `append:messages={"text":"a"} append:n=1`)
	if code != 1 || out != "" || !strings.HasPrefix(errw, "tuimark: play: step 2 (append:n=1): ") {
		t.Errorf("second step: exit %d stdout %q stderr %q", code, out, errw)
	}

	// The script step: exactly path and value.
	script := func(line string) string {
		p := filepath.Join(t.TempDir(), "s.ndjson")
		writeFile(t, p, line+"\n")
		return p
	}
	scripts := []struct{ line, want string }{
		{`{"append": {"path": "messages"}}`, `append: wants {"path": ..., "value": ...} (both required)`},
		{`{"append": {"value": 1}}`, `append: wants {"path": ..., "value": ...} (both required)`},
		{`{"append": {"path": "messages", "value": 1, "extra": 2}}`, `append: unknown member "extra" (want exactly "path" and "value")`},
		{`{"append": {"path": 5, "value": 1}}`, `append.path`},
		{`{"append": "messages"}`, `append: want a JSON object`},
		{`{"append": {"path": "n", "value": 1}}`, `append "n": "n" holds a number, not a string or an array`},
	}
	for _, c := range scripts {
		code, out, errw := runCLI("play", d5Msgs, "--data", d5Data, "--script", script(c.line))
		if code != 1 || out != "" || !strings.Contains(errw, "tuimark: play: step 1 ("+c.line+"): ") || !strings.Contains(errw, c.want) {
			t.Errorf("%s: exit %d stdout %q stderr %q, want it to hold %q", c.line, code, out, errw, c.want)
		}
	}
	// A line with a second member is not an append step.
	code, _, errw = runCLI("play", d5Msgs, "--data", d5Data, "--script", script(`{"append": {"path": "messages", "value": 1}, "text": "x"}`))
	if code != 1 || !strings.Contains(errw, "more than one step member on one line") {
		t.Errorf("two members: exit %d %q", code, errw)
	}
	// The members named in the two messages gain append.
	for _, line := range []string{`{}`, `{"nope": 1}`} {
		_, _, errw := runCLI("play", d5Msgs, "--script", script(line))
		if !strings.Contains(errw, "append)") {
			t.Errorf("%s: %q does not name append", line, errw)
		}
	}
}

// §15.5: a manifest entry's input holds append: steps, goldens included.
func TestD5ManifestAppend(t *testing.T) {
	dir := t.TempDir()
	tui, err := os.ReadFile(d5Msgs)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "app.tui")
	writeFile(t, file, string(tui))
	data := filepath.Join(dir, "sample.json")
	writeFile(t, data, `{"messages": [], "live": {"text": "hello"}, "busy": true}`)
	writeManifest(t, dir, `[{"name":"e","file":"`+file+`","data":"`+data+`","sizes":["40x4"],"input":"append:live.text=\"!\""}]`)
	if code, out, errw := runCLI("test", dir, "--update"); code != 0 {
		t.Fatalf("update: exit %d %s%s", code, out, errw)
	}
	text, err := os.ReadFile(filepath.Join(dir, "e", "40x4.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(text), "live=hello!") {
		t.Errorf("golden:\n%s", text)
	}
	if code, out, _ := runCLI("test", dir); code != 0 || !strings.Contains(out, "PASS e 40x4") {
		t.Errorf("golden does not pass: exit %d %s", code, out)
	}
}

// §15.8: tuimark_play's steps accept append: steps, one element each.
func TestD5MCPPlayAppend(t *testing.T) {
	step := `append:messages={"text":"a b"}`
	params := fmt.Sprintf(`{"name":"tuimark_play","arguments":{"file":%q,"data":%q,"steps":[%s]}}`, d5Msgs, d5Data, mustJSON(t, step))
	got, isErr := mcpCall(t, params)
	// --input cannot carry the space; the same step as a script line does.
	script := filepath.Join(t.TempDir(), "s.ndjson")
	writeFile(t, script, `{"append": {"path": "messages", "value": {"text": "a b"}}}`+"\n")
	want, wantErr := cliResult("play", d5Msgs, "--data", d5Data, "--script", script, "--format", "json")
	if got != want || isErr != wantErr || isErr || !strings.Contains(got, "msg-a b") {
		t.Errorf("MCP %q (%v), CLI %q (%v)", got, isErr, want, wantErr)
	}
	params = fmt.Sprintf(`{"name":"tuimark_play","arguments":{"file":%q,"data":%q,"steps":["append:n=1"]}}`, d5Msgs, d5Data)
	got, isErr = mcpCall(t, params)
	if !isErr || !strings.HasPrefix(got, `tuimark: play: step 1 (append:n=1): append "n": "n" holds a number`) {
		t.Errorf("failing step: %q %v", got, isErr)
	}
}
