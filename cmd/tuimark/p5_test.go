package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// SPEC v0.2b §21 tests of the mouse and acceptance phase through the CLI:
// play's mouse and theme steps (47) and the IR of examples/monitor (40).

const mouseFixture = `<tui version="2" mouse="m">
<screen id="s" focus="#b">
  <list id="l" each="rows as r" key="r" on:select="pick"><item><text>{r}</text></item></list>
  <button id="b" label="go" on:click="go"/>
</screen>
</tui>
`

func writeMouseFixture(t *testing.T, mouse bool) (tui, data string) {
	t.Helper()
	dir := t.TempDir()
	tui = filepath.Join(dir, "m.tui")
	data = filepath.Join(dir, "m.json")
	writeFile(t, tui, mouseFixture)
	m := "false"
	if mouse {
		m = "true"
	}
	writeFile(t, data, `{"m": `+m+`, "rows": ["a", "b", "c"]}`)
	return tui, data
}

func playEvents(t *testing.T, out string) []string {
	t.Helper()
	var p struct {
		Events []struct {
			Step   int    `json:"step"`
			Action string `json:"action"`
			Value  any    `json:"value"`
			Keys   map[string]any
		} `json:"events"`
	}
	mustUnmarshal(t, out, &p)
	var evs []string
	for _, e := range p.Events {
		b, _ := json.Marshal(e.Keys)
		evs = append(evs, strings.Join([]string{string(rune('0' + e.Step)), e.Action, string(b)}, " "))
	}
	return evs
}

// 47. click:, wheel-up:, wheel-down:, {"click":…}, {"wheel":…,"at":…},
// and {"theme":…} run as §15.4 says.
func TestPlayMouseSteps(t *testing.T) {
	tui, data := writeMouseFixture(t, true)
	code, out, errw := runCLI("play", tui, "--data", data, "--cols", "20", "--rows", "5", "--format", "json",
		"--input", "click:0,1 wheel-down:0,0 wheel-down:0,0 wheel-up:1,2 click:1,3")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errw)
	}
	want := `1 pick {"r":"b"}|2 pick {"r":"c"}|4 pick {"r":"b"}|5 go {}`
	if got := strings.Join(playEvents(t, out), "|"); got != want {
		t.Errorf("events %s, want %s", got, want)
	}
	script := filepath.Join(t.TempDir(), "s.ndjson")
	writeFile(t, script, `{"click":[0,2]}`+"\n"+`{"wheel":"up","at":[0,0]}`+"\n"+`{"theme":"light"}`+"\n")
	code, out, errw = runCLI("play", tui, "--data", data, "--cols", "20", "--rows", "5", "--format", "json", "--styles", "--script", script)
	if code != 0 {
		t.Fatalf("script: exit %d: %s", code, errw)
	}
	if got := strings.Join(playEvents(t, out), "|"); got != `1 pick {"r":"c"}|2 pick {"r":"b"}` {
		t.Errorf("script events %s", got)
	}
	if !strings.Contains(out, `"theme": "light"`) {
		t.Error("the theme step did not apply")
	}
}

// 47. A cell outside the grid (also after a resize), a malformed cell, a
// wheel direction other than up and down, and a member other than wheel
// and at are usage errors: exit 1, nothing on stdout.
func TestPlayMouseStepErrors(t *testing.T) {
	tui, data := writeMouseFixture(t, true)
	for _, in := range []string{"click:20,0", "click:0,5", "wheel-up:-1,0", "resize:10x5 click:10,0", "click:1", "click:a,1", "wheel-down:1,2,3", "click:+1,2"} {
		code, out, errw := runCLI("play", tui, "--data", data, "--cols", "20", "--rows", "5", "--input", in)
		if code != 1 || out != "" || !strings.Contains(errw, "play: step") {
			t.Errorf("--input %q: exit %d, stdout %q, stderr %q", in, code, out, errw)
		}
	}
	for _, line := range []string{
		`{"wheel":"left","at":[0,0]}`, `{"wheel":"up","at":[0,0],"x":1}`, `{"wheel":"up"}`, `{"at":[0,0]}`,
		`{"click":[1.5,2]}`, `{"click":[1]}`, `{"click":[1,2,3]}`, `{"click":"1,2"}`, `{"click":[0,9]}`,
		`{"theme":"blue"}`,
	} {
		script := filepath.Join(t.TempDir(), "s.ndjson")
		writeFile(t, script, line+"\n")
		code, out, errw := runCLI("play", tui, "--data", data, "--cols", "20", "--rows", "5", "--script", script)
		if code != 1 || out != "" {
			t.Errorf("%s: exit %d, stdout %q, stderr %q", line, code, out, errw)
		}
	}
}

// 47. With mouse false a click records no event and changes no frame.
func TestPlayMouseOff(t *testing.T) {
	tui, data := writeMouseFixture(t, false)
	_, base, _ := runCLI("play", tui, "--data", data, "--cols", "20", "--rows", "5", "--format", "json")
	code, out, errw := runCLI("play", tui, "--data", data, "--cols", "20", "--rows", "5", "--format", "json", "--input", "click:0,1 wheel-down:0,0")
	if code != 0 || out != base {
		t.Errorf("exit %d (%s); output with mouse off differs:\n%s\nwant\n%s", code, errw, out, base)
	}
}

// 40. `tuimark ir` of examples/monitor/studio.tui is IR "0.2", valid
// against schema/ir.v0.2.json, with mouse in app, label/keycap in keymap
// rows, class:NAME in attrs, and the new kinds.
func TestMonitorIR(t *testing.T) {
	code, out, errw := runCLI("ir", "../../examples/monitor/studio.tui")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errw)
	}
	v := decodeJSON(t, []byte(out))
	if errs := validateSchema(t, irV02Schema, v); len(errs) > 0 {
		t.Errorf("monitor IR vs ir.v0.2.json: %v", errs)
	}
	for _, want := range []string{`"version": "0.2"`, `"mouse": "mouse_enabled"`, `"keycap": "1-9"`, `"label": "tabs"`, `"class:live": "live"`, `"class:hot": "p.cpu_hot"`,
		`"kind": "table"`, `"kind": "column"`, `"kind": "tabs"`, `"kind": "tab"`, `"kind": "sparkline"`, `"kind": "hints"`} {
		if !strings.Contains(out, want) {
			t.Errorf("monitor IR lacks %s", want)
		}
	}
}
