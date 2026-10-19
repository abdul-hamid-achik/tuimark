package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/dump"
)

// SPEC v0.4b §24, phase F10: the CLI parts of the spinner and tick:DUR
// tests (190, 191, 193, 203, 204, 208, 210). The Go API parts are the
// root package's conformance_v04b_clock_test.go; the Run side of the
// spinner (194-196) is F11's: internal/host's runclock_*_test.go, and
// conformance_v04b_clock_run_test.go here for the CLI side of 196.

const clockS = "../../testdata/v04b/spinner/S.tui"

var clockSGrid0 = []string{"⠋ loading           ", "  ⠋                 ", "    ⠋               ", "⠋                   ", "                    "}

// 190 (CLI). dump S.tui at phase 0: --format text gives the same grid, a
// second run is byte-identical, and --cells names the owners.
func TestV04bClock190DumpCLI(t *testing.T) {
	code, out, errw := runCLI("dump", clockS, "--cols", "20", "--rows", "5", "--format", "json", "--cells")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errw)
	}
	var d dump.Dump
	if err := json.Unmarshal([]byte(out), &d); err != nil {
		t.Fatal(err)
	}
	if strings.Join(d.Grid, "|") != strings.Join(clockSGrid0, "|") {
		t.Errorf("grid = %q", d.Grid)
	}
	for _, n := range d.Nodes {
		if n.Tag == "progress" && n.Text != "" {
			t.Errorf("%s has text %q", n.ID, n.Text)
		}
	}
	owner := func(x, y int) string {
		for _, c := range d.Cells {
			if c.X == x && c.Y == y {
				return c.ID
			}
		}
		return "?"
	}
	if owner(2, 1) != "mid" || owner(0, 3) != "start" {
		t.Errorf("owners: (2,1) = %q, (0,3) = %q", owner(2, 1), owner(0, 3))
	}
	_, txt1, _ := runCLI("dump", clockS, "--cols", "20", "--rows", "5")
	_, txt2, _ := runCLI("dump", clockS, "--cols", "20", "--rows", "5")
	if txt1 != txt2 {
		t.Error("two runs differ")
	}
	if !strings.Contains(txt1, strings.Join(clockSGrid0, "\n")) {
		t.Errorf("--format text grid:\n%s", txt1)
	}
	_, out2, _ := runCLI("dump", clockS, "--cols", "20", "--rows", "5", "--format", "json", "--cells")
	if out != out2 {
		t.Error("two JSON runs differ")
	}
}

// 191 (CLI). play with tick steps, --frames, --script, and the dump after.
func TestV04bClock191PlayFramesAndScript(t *testing.T) {
	const input = "tick:99ms tick:1ms tick:900ms tick:250ms"
	code, out, errw := runCLI("play", clockS, "--cols", "20", "--rows", "5", "--input", input, "--frames", "--format", "json")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errw)
	}
	p := decodePlay(t, out)
	steps := strings.Fields(input)
	if len(p.Frames) != 5 {
		t.Fatalf("%d frames", len(p.Frames))
	}
	first := []string{"⠋", "⠋", "⠙", "⠋", "⠹"}
	for i, f := range p.Frames {
		wantInput := ""
		if i > 0 {
			wantInput = steps[i-1]
		}
		if f.Step != i || f.Input != wantInput || len(f.Events) != 0 {
			t.Errorf("frame %d: step %d input %q events %v", i, f.Step, f.Input, f.Events)
		}
		if !strings.HasPrefix(f.Dump.Grid[0], first[i]) {
			t.Errorf("frame %d grid[0] = %q", i, f.Dump.Grid[0])
		}
	}
	want := []string{"⠹ loading           ", "  ⠹                 ", "    ⠹               ", "⠹                   "}
	if strings.Join(p.Grid[:4], "|") != strings.Join(want, "|") {
		t.Errorf("step 4 rows = %q", p.Grid[:4])
	}
	if strings.Join(p.Grid, "|") != strings.Join(p.Frames[4].Dump.Grid, "|") || len(p.Events) != 0 {
		t.Error("top-level grid or events differ from the last frame")
	}

	// --script: the same output except each frame's input, the script line.
	lines := []string{`{"tick":"99ms"}`, `{"tick":"1ms"}`, `{"tick":"900ms"}`, `{"tick":"250ms"}`}
	script := filepath.Join(t.TempDir(), "s.ndjson")
	writeFile(t, script, strings.Join(lines, "\n")+"\n")
	code, out2, errw := runCLI("play", clockS, "--cols", "20", "--rows", "5", "--script", script, "--frames", "--format", "json")
	if code != 0 {
		t.Fatalf("script: exit %d: %s", code, errw)
	}
	q := decodePlay(t, out2)
	for i, f := range q.Frames {
		if i > 0 && f.Input != lines[i-1] {
			t.Errorf("script frame %d input = %q", i, f.Input)
		}
		q.Frames[i].Input = p.Frames[i].Input
	}
	a, _ := json.Marshal(p)
	b, _ := json.Marshal(q)
	if string(a) != string(b) {
		t.Error("--script output differs from --input beyond the frames' input")
	}

	_, after, _ := runCLI("dump", clockS, "--cols", "20", "--rows", "5", "--format", "json")
	var d dump.Dump
	if err := json.Unmarshal([]byte(after), &d); err != nil || !strings.HasPrefix(d.Grid[0], "⠋") {
		t.Errorf("dump after: %v %q", err, d.Grid)
	}
}

func clockDoc(t *testing.T, line string) string {
	t.Helper()
	return writeTemp(t, "F.tui", `<tui><screen id="main">`+line+`</screen></tui>`)
}

func clockData(t *testing.T) string {
	return writeTemp(t, "d.json", `{}`)
}

type clockDiag struct {
	Code string `json:"code"`
	Msg  string `json:"msg"`
	Line int    `json:"line"`
	Col  int    `json:"col"`
}

// 193. The diagnostics of indeterminate.
func TestV04bClock193Diagnostics(t *testing.T) {
	data := clockData(t)
	cases := []struct {
		line  string
		codes []string // code and message, in order
		grid0 string
		exit  int
	}{
		{`<progress id="p" indeterminate="true" value="40"/>`, []string{`V003 indeterminate="true" cannot be combined with bind or value`}, "████████░░░░░░░░░░░░", 2},
		{`<progress id="p" indeterminate="yes"/>`, []string{`V003 indeterminate="yes": want true or false`}, strings.Repeat("░", 20), 2},
		{`<progress id="p" indeterminate="p"/>`, []string{`V003 indeterminate="p": want true or false`}, strings.Repeat("░", 20), 2},
		{`<progress id="p" indeterminate="false" value="40"/>`, nil, "████████░░░░░░░░░░░░", 0},
		// (B003 is reported at the node, before the attribute's V003.)
		{`<progress id="p" indeterminate="true" bind="p"/>`, []string{`B003 bind path "p" is missing`, `V003 indeterminate="true" cannot be combined with bind or value`}, "", 2},
		{`<progress id="p" indeterminate="true"/>`, nil, "⠋" + strings.Repeat(" ", 19), 0},
		{`<progress id="p" value="40" indeterminate="true"/>`, []string{`V003 indeterminate="true" cannot be combined with bind or value`}, "████████░░░░░░░░░░░░", 2},
	}
	for _, c := range cases {
		t.Run(c.line, func(t *testing.T) {
			f := clockDoc(t, c.line)
			code, out, errw := runCLI("validate", f, "--data", data, "--json")
			var v struct {
				Diagnostics []clockDiag `json:"diagnostics"`
			}
			if err := json.Unmarshal([]byte(out), &v); err != nil {
				t.Fatalf("%v\n%s\n%s", err, out, errw)
			}
			var got []string
			for _, d := range v.Diagnostics {
				got = append(got, d.Code+" "+d.Msg)
			}
			if len(got) != len(c.codes) {
				t.Fatalf("diagnostics = %q, want %q", got, c.codes)
			}
			for i := range got {
				if !strings.HasPrefix(got[i], c.codes[i]) {
					t.Errorf("diagnostic %d = %q, want it to start with %q", i, got[i], c.codes[i])
				}
			}
			if code != c.exit && !(c.exit == 2 && strings.HasPrefix(c.codes[0], "B")) {
				t.Errorf("exit %d, want %d", code, c.exit)
			}
			if c.grid0 == "" {
				return
			}
			_, dout, _ := runCLI("dump", f, "--data", data, "--cols", "20", "--rows", "2", "--format", "json")
			var d dump.Dump
			if err := json.Unmarshal([]byte(dout), &d); err != nil {
				t.Fatal(err)
			}
			if d.Grid[0] != c.grid0 {
				t.Errorf("grid[0] = %q, want %q", d.Grid[0], c.grid0)
			}
		})
	}

	t.Run("position", func(t *testing.T) {
		f := clockDoc(t, `<progress id="p" indeterminate="true" value="40"/>`)
		_, out, _ := runCLI("validate", f, "--data", data, "--json")
		var v struct {
			Diagnostics []clockDiag `json:"diagnostics"`
		}
		if err := json.Unmarshal([]byte(out), &v); err != nil || len(v.Diagnostics) != 1 {
			t.Fatalf("%v %s", err, out)
		}
		// <tui><screen id="main"> is 23 columns, so <progress starts at
		// column 24 and its indeterminate attribute 17 columns later.
		if d := v.Diagnostics[0]; d.Line != 1 || d.Col != 24+len(`<progress id="p" `) {
			t.Errorf("at %d:%d, want the attribute indeterminate at 1:41", d.Line, d.Col)
		}
	})

	t.Run("box", func(t *testing.T) {
		f := clockDoc(t, `<box id="p" indeterminate="true"/>`)
		_, out, _ := runCLI("validate", f, "--data", data, "--json")
		if !strings.Contains(out, `"code": "V002"`) {
			t.Errorf("want V002:\n%s", out)
		}
	})

	for _, ver := range []string{"1", "2", "3"} {
		t.Run("version "+ver, func(t *testing.T) {
			f := writeTemp(t, "F.tui", `<tui version="`+ver+`"><screen id="main"><progress id="p" indeterminate="true"/></screen></tui>`)
			code, out, _ := runCLI("validate", f, "--data", data, "--json")
			var v struct {
				Diagnostics []clockDiag `json:"diagnostics"`
			}
			if err := json.Unmarshal([]byte(out), &v); err != nil || len(v.Diagnostics) != 1 {
				t.Fatalf("%v %s", err, out)
			}
			d := v.Diagnostics[0]
			if d.Code != "V002" || !strings.HasSuffix(d.Msg, " (requires a <tui> without version)") || code != 2 {
				t.Errorf("exit %d diagnostic %+v", code, d)
			}
			_, dout, _ := runCLI("dump", f, "--data", data, "--cols", "20", "--rows", "2", "--format", "json")
			var dd dump.Dump
			if err := json.Unmarshal([]byte(dout), &dd); err != nil {
				t.Fatal(err)
			}
			if dd.Grid[0] != strings.Repeat("░", 20) {
				t.Errorf("grid[0] = %q, want a bar at 0", dd.Grid[0])
			}
		})
	}
}

// 203 (CLI). tick of any document exits 0 and only moves spinners, and
// tuimark_play with the same step gives the CLI's output byte for byte.
func TestV04bClock203CLIAndMCP(t *testing.T) {
	code, out, errw := runCLI("play", clockS, "--cols", "20", "--rows", "5", "--input", "tick:1s", "--format", "json")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errw)
	}
	if p := decodePlay(t, out); len(p.Events) != 0 || !strings.HasPrefix(p.Grid[0], "⠋") {
		t.Errorf("events %v grid[0] %q (1 s is phase 10, glyph 0)", p.Events, p.Grid[0])
	}
	// Any document: exit 0, and only the spinners move, so the final grid
	// is the one of the same play without the tick step.
	for _, c := range []struct{ file, data string }{
		{inboxFixture, inboxData},
		{"../../examples/monitor/studio.tui", "../../examples/monitor/sample.json"},
		{clockS, ""},
	} {
		args := []string{"play", c.file, "--format", "json"}
		if c.data != "" {
			args = append(args, "--data", c.data)
		}
		code, with, errw := runCLI(append(args, "--input", "tick:1s")...)
		if code != 0 {
			t.Errorf("%s: exit %d: %s", c.file, code, errw)
			continue
		}
		_, without, _ := runCLI(args...) // step 0 only
		a, b := decodePlay(t, with), decodePlay(t, without)
		if strings.Join(a.Grid, "\n") != strings.Join(b.Grid, "\n") && c.file != clockS {
			t.Errorf("%s: tick:1s changed the grid beyond spinners", c.file)
		}
		if len(a.Events) != 0 {
			t.Errorf("%s: tick:1s fired %v", c.file, a.Events)
		}
	}
	srv := newMCPTestServer(t)
	defer srv.closeAndWait()
	result := srv.toolCallResult(1, "tuimark_play", map[string]any{"file": clockS, "cols": 20, "rows": 5, "steps": []any{"tick:1s"}})
	got := toolText(t, result)
	_, want, _ := runCLI("play", clockS, "--cols", "20", "--rows", "5", "--input", "tick:1s", "--format", "json")
	if got != want {
		t.Errorf("tuimark_play differs from the CLI:\n%s\n%s", got, want)
	}
}

// 204 (CLI). The grammar of DUR and its reasons.
func TestV04bClock204Grammar(t *testing.T) {
	const bad = `tick: %q is not a duration (want a number and a unit ms, s, m, or h, such as 250ms)`
	form := func(text string) string { return strings.Replace(bad, "%q", `"`+text+`"`, 1) }
	cases := []struct{ step, reason string }{
		{"tick:0ms", "tick: duration must be at least 1ms"},
		{"tick:0s", "tick: duration must be at least 1ms"},
		{"tick:1.5ms", "tick: duration must be a whole number of milliseconds"},
		{"tick:2h", "tick: duration must be at most 1h"},
		{"tick:1h1ms", "tick: duration must be at most 1h"},
		{"tick:0.5ms", "tick: duration must be a whole number of milliseconds"},
		{"tick:99999999999999999999h", "tick: duration must be at most 1h"},
		{"tick:-5s", form("-5s")},
		{"tick:+5s", form("+5s")},
		{"tick:100", form("100")},
		{"tick:.5s", form(".5s")},
		{"tick:1S", form("1S")},
		{"tick:", form("")},
		{"tick:250us", form("250us")},
		{"tick:1 s", ""}, // split by --input: a second step that is no key
	}
	for _, c := range cases {
		if c.reason == "" {
			continue
		}
		code, out, errw := runCLI("play", clockS, "--input", c.step)
		want := "tuimark: play: step 1 (" + c.step + "): " + c.reason
		if code != 1 || out != "" || strings.TrimSpace(errw) != want {
			t.Errorf("%s: exit %d stdout %q stderr %q, want %q", c.step, code, out, errw, want)
		}
	}
	for _, ok := range []string{"tick:1ms", "tick:250ms", "tick:1.5s", "tick:1m30s", "tick:60m", "tick:1h"} {
		if code, _, errw := runCLI("play", clockS, "--input", ok); code != 0 {
			t.Errorf("%s: exit %d: %s", ok, code, errw)
		}
	}
	scripts := []struct{ line, want string }{
		{`{"tick":250}`, "tick: want a JSON string"},
		{`{"tick":"250ms","key":"a"}`, "more than one step member on one line"},
		{`{"tick":"0ms"}`, "tick: duration must be at least 1ms"},
	}
	for _, s := range scripts {
		path := filepath.Join(t.TempDir(), "s.ndjson")
		writeFile(t, path, s.line+"\n")
		code, out, errw := runCLI("play", clockS, "--script", path)
		if code != 1 || out != "" || !strings.Contains(errw, "tuimark: play: step 1 ("+s.line+"): "+s.want) {
			t.Errorf("%s: exit %d stdout %q stderr %q, want it to hold %q", s.line, code, out, errw, s.want)
		}
	}
	// The members named in the two messages gain tick.
	path := filepath.Join(t.TempDir(), "s.ndjson")
	writeFile(t, path, "{}\n")
	if _, _, errw := runCLI("play", clockS, "--script", path); !strings.Contains(errw, "theme, tick)") {
		t.Errorf("no known step member: %q", errw)
	}
	writeFile(t, path, `{"nope":1}`+"\n")
	if _, _, errw := runCLI("play", clockS, "--script", path); !strings.Contains(errw, "theme, tick)") {
		t.Errorf("unknown step member: %q", errw)
	}
}

// 208 (all but the committed manifest entry, which waits for F5's
// sheet). A manifest entry with input "tick:100ms" has goldens that start
// with the phase-1 glyph; a second --update leaves them byte-identical,
// without --allow-breaking; an entry without input stays at phase 0.
func TestV04bClock208Goldens(t *testing.T) {
	dir := t.TempDir()
	tui := filepath.Join(dir, "S.tui")
	src, err := os.ReadFile(clockS)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, tui, string(src))
	writeManifest(t, dir, `[`+
		`{"name":"spinner","file":"`+tui+`","sizes":["20x5"],"json":["20x5"],"input":"tick:100ms","frozen":false},`+
		`{"name":"rest","file":"`+tui+`","sizes":["20x5"],"json":["20x5"],"frozen":false}]`)
	if code, out, errw := runCLI("test", dir, "--update"); code != 0 {
		t.Fatalf("seed: exit %d: %s%s", code, out, errw)
	}
	read := func(name, size string) string {
		b, err := os.ReadFile(filepath.Join(dir, name, size))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	files := []string{"spinner/20x5.txt", "spinner/20x5.json", "rest/20x5.txt", "rest/20x5.json"}
	before := map[string]string{}
	for _, f := range files {
		name, size, _ := strings.Cut(f, "/")
		before[f] = read(name, size)
	}
	var p dump.Dump
	if err := json.Unmarshal([]byte(before["spinner/20x5.json"]), &p); err != nil {
		t.Fatal(err)
	}
	for i, row := range p.Grid[:4] {
		if !strings.HasPrefix(row, "⠙") && !strings.HasPrefix(strings.TrimLeft(row, " "), "⠙") {
			t.Errorf("spinner row %d = %q has no phase-1 glyph", i, row)
		}
	}
	if !strings.HasPrefix(p.Grid[0], "⠙") {
		t.Errorf("spinner grid[0] = %q", p.Grid[0])
	}
	var r dump.Dump
	if err := json.Unmarshal([]byte(before["rest/20x5.json"]), &r); err != nil || !strings.HasPrefix(r.Grid[0], "⠋") {
		t.Errorf("an entry without input: %v %q", err, r.Grid)
	}
	if code, out, errw := runCLI("test", dir, "--update"); code != 0 {
		t.Fatalf("second update: exit %d: %s%s", code, out, errw)
	}
	for _, f := range files {
		name, size, _ := strings.Cut(f, "/")
		if got := read(name, size); got != before[f] {
			t.Errorf("%s changed on a second --update", f)
		}
	}
	if code, out, errw := runCLI("test", dir); code != 0 {
		t.Errorf("test without --update: exit %d: %s%s", code, out, errw)
	}
}

// 210 (CLI). The spinner in a list item, through play.
func TestV04bClock210PlayList(t *testing.T) {
	f := writeTemp(t, "L.tui", `<tui><screen id="main"><list id="l" each="rows as r" key="r.id"><item><progress indeterminate="true"/><text>{r.name}</text></item></list></screen></tui>`)
	data := writeTemp(t, "rows.json", `{"rows":[{"id":1,"name":"A"},{"id":2,"name":"B"}]}`)
	code, out, errw := runCLI("play", f, "--data", data, "--cols", "20", "--rows", "4", "--input", "tick:100ms", "--frames", "--format", "json")
	if code != 0 {
		t.Fatalf("exit %d: %s%s", code, out, errw)
	}
	p := decodePlay(t, out)
	pad := func(s string) string { return s + strings.Repeat(" ", 20-len([]rune(s))) }
	for step, glyph := range []string{"⠋", "⠙"} {
		want := strings.Join([]string{pad(glyph), pad("A"), pad(glyph), pad("B")}, "|")
		if got := strings.Join(p.Frames[step].Dump.Grid, "|"); got != want {
			t.Errorf("step %d grid = %q, want %q", step, got, want)
		}
	}
}

// IR 0.4 keeps indeterminate in attrs, as written (SPEC §23.6), and
// validates against schema/ir.v0.4.json.
func TestV04bClockIRKeepsIndeterminate(t *testing.T) {
	code, out, errw := runCLI("ir", clockS)
	if code != 0 || !strings.Contains(out, `"version": "0.4"`) || !strings.Contains(out, `"indeterminate": "true"`) {
		t.Fatalf("ir: exit %d %s%s", code, out, errw)
	}
	if errs := validateSchema(t, irV04Schema, decodeJSON(t, []byte(out))); len(errs) != 0 {
		t.Errorf("IR vs ir.v0.4.json: %v", errs)
	}
}
