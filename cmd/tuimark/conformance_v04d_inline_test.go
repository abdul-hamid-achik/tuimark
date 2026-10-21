package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// SPEC v0.4d §21 tests of phase D4 (inline mode, §26.15) through the CLI:
// 294 to 298, 300, 306 (the manifest and MCP parts), and 315. The fixtures
// are testdata/v04d/t<NNN>; the bytes a session writes are the pty and
// terminal-emulator tests of internal/host. Tests 294, 296, 299, and 315
// use an <input> in place of the textarea (D3 brings it): the geometry is
// the same, and the nodes differ only in the tag. Test 275 is the
// byte-identical comparison with tuimark 0.4.5 that the cmp3 and cmp5
// scripts run over every example and golden document.

const (
	inT294    = "../../testdata/v04d/t294/in.tui"
	inT294Dat = "../../testdata/v04d/t294/in.json"
	inPinned  = "../../testdata/v04d/t294/pinned.tui"
	inCard    = "../../testdata/v04d/t296/card.tui"
	inModal   = "../../testdata/v04d/t300/modal.tui"
	inClick   = "../../testdata/v04d/t300/clickable.tui"
	inOvl     = "../../testdata/v04d/t315/ovl.tui"
	inOvlCol  = "../../testdata/v04d/t315/ovl-col.tui"
	inOvlBox  = "../../testdata/v04d/t315/ovl-box.tui"
	inOvlDat  = "../../testdata/v04d/t315/ovl.json"
	inStr     = "../../testdata/v04d/t315/str.tui"
	inStrDat  = "../../testdata/v04d/t315/str.json"
)

func strList(v any) []string {
	var out []string
	for _, e := range v.([]any) {
		out = append(out, e.(string))
	}
	return out
}

// inScript writes one JSON step per line and returns the --script path.
func inScript(t *testing.T, steps ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "steps.ndjson")
	if err := os.WriteFile(path, []byte(strings.Join(steps, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// SPEC v0.4d test 294: the region is the screen sized by its content, at
// the terminal's width; rows is its height and terminal_rows the
// terminal's; without --inline the dump is the terminal's height.
func TestD4Region294(t *testing.T) {
	m := dumpMap(t, inT294, "--data", inT294Dat, "--cols", "30", "--rows", "24", "--inline")
	if m["rows"] != 3.0 || m["terminal_rows"] != 24.0 || m["cols"] != 30.0 {
		t.Errorf("rows %v terminal_rows %v cols %v", m["rows"], m["terminal_rows"], m["cols"])
	}
	want := []string{"──────────────────────────────", "Message                       ", "ready                         "}
	if got := gridOfMap(m); !reflect.DeepEqual(got, want) {
		t.Errorf("grid %q", got)
	}
	type geo struct {
		tag        string
		x, y, w, h float64
	}
	wantNodes := map[string]geo{"main": {"screen", 0, 0, 30, 3}, "composer": {"input", 0, 1, 30, 1}, "status": {"text", 0, 2, 30, 1}}
	for id, g := range wantNodes {
		n := nodeOfMap(t, m, id)
		if n["tag"] != g.tag || n["x"] != g.x || n["y"] != g.y || n["w"] != g.w || n["h"] != g.h {
			t.Errorf("node %s = %v, want %v", id, n, g)
		}
	}
	// The member comes right after rows (SPEC v0.4d §13.2).
	_, out, _ := runCLI("dump", inT294, "--data", inT294Dat, "--cols", "30", "--rows", "24", "--inline", "--format", "json")
	if i, j := strings.Index(out, `"rows"`), strings.Index(out, `"terminal_rows"`); i < 0 || j < i || strings.Index(out, `"ok"`) < j {
		t.Errorf("terminal_rows is not right after rows:\n%s", out[:120])
	}
	// Without --inline: 24 rows, no terminal_rows.
	m = dumpMap(t, inT294, "--data", inT294Dat, "--cols", "30", "--rows", "24")
	if _, has := m["terminal_rows"]; has || m["rows"] != 24.0 {
		t.Errorf("full screen: rows %v, terminal_rows %v", m["rows"], m["terminal_rows"])
	}
	// --rows 2: the status is cut, L009 on the screen.
	m = dumpMap(t, inT294, "--data", inT294Dat, "--cols", "30", "--rows", "2", "--inline")
	if m["rows"] != 2.0 || m["terminal_rows"] != 2.0 {
		t.Errorf("--rows 2: rows %v terminal_rows %v", m["rows"], m["terminal_rows"])
	}
	errs := m["errors"].([]any)
	if len(errs) != 1 || errs[0].(map[string]any)["code"] != "L009" || errs[0].(map[string]any)["path"] != "/screen" {
		t.Errorf("--rows 2 errors %v", errs)
	}
	// inspect lays the screen out the same way.
	code, out, errw := runCLI("inspect", inT294, "--data", inT294Dat, "--cols", "30", "--rows", "24", "--inline", "--at", "3,2", "--json")
	if code != 0 {
		t.Fatalf("inspect: exit %d %s", code, errw)
	}
	if im := jsonMap(t, out); im["rows"] != 3.0 || nodeIDOf(im) != "status" {
		t.Errorf("inspect --inline: %v", im)
	}
	// @media reads the terminal's rows, never the region's (decision 242).
	src := "<tui><style>@media (min-rows: 20) { #t { color: red; } }</style><screen id=\"s\"><text id=\"t\">x</text></screen></tui>"
	path := writeTemp(t, "m.tui", src)
	m = playMap(t, path, "--cols", "10", "--rows", "24", "--inline", "--styles")
	if got := m["styles"].([]any)[0].([]any)[0].(map[string]any)["fg"]; got != "red" {
		t.Errorf("@media (min-height: 20) with a region of 1 row in 24: fg %v", got)
	}
}

func nodeIDOf(im map[string]any) string {
	n, _ := im["node"].(map[string]any)
	id, _ := n["id"].(string)
	return id
}

// SPEC v0.4d test 295: committing lines.
func TestD4CommitLines295(t *testing.T) {
	script := inScript(t, `{"commit": {"lines": ["You: hello", [{"text": "ok", "class": "good"}]]}}`)
	m := playMap(t, inT294, "--data", inT294Dat, "--cols", "30", "--rows", "24", "--inline", "--styles", "--script", script)
	sb := m["scrollback"].(map[string]any)
	wantGrid := []string{"You: hello                    ", "ok                            "}
	if got := strList(sb["grid"]); !reflect.DeepEqual(got, wantGrid) {
		t.Errorf("scrollback grid %q", got)
	}
	var wantStyles any
	_ = json.Unmarshal([]byte(`[[{"x":0,"w":30,"fg":"default","bg":"default"}],[{"x":0,"w":2,"fg":"green","bg":"default"},{"x":2,"w":28,"fg":"default","bg":"default"}]]`), &wantStyles)
	if !reflect.DeepEqual(sb["styles"], wantStyles) {
		t.Errorf("scrollback styles %v", sb["styles"])
	}
	// scrollback comes after events and before frames.
	_, out, _ := runCLI("play", inT294, "--data", inT294Dat, "--cols", "30", "--rows", "24", "--inline", "--styles", "--frames", "--format", "json", "--script", script)
	if e, s, f := strings.Index(out, `"events"`), strings.Index(out, `"scrollback"`), strings.LastIndex(out, `"frames"`); !(e < s && s < f) {
		t.Errorf("member order: events %d, scrollback %d, frames %d", e, s, f)
	}
	m = playMap(t, inT294, "--data", inT294Dat, "--cols", "30", "--rows", "24", "--inline", "--frames", "--script", script)
	var counts []float64
	for _, f := range m["frames"].([]any) {
		counts = append(counts, f.(map[string]any)["scrollback_rows"].(float64))
	}
	if !reflect.DeepEqual(counts, []float64{0, 2}) {
		t.Errorf("scrollback_rows %v", counts)
	}
	// Without --styles there is no styles member.
	if _, has := m["scrollback"].(map[string]any)["styles"]; has {
		t.Errorf("styles without --styles")
	}
	// Wrapping, TAB, the controls of decision 265, and LF.
	long := strings.Repeat("0123456789", 4)
	m = playMap(t, inT294, "--data", inT294Dat, "--cols", "30", "--rows", "24", "--inline",
		"--script", inScript(t, `{"commit": {"lines": ["`+long+`"]}}`, `{"commit": {"lines": ["a\tb", "a\tb\u001bc", "a\nb", ""]}}`))
	want := []string{
		"012345678901234567890123456789",
		"0123456789                    ",
		"a    b                        ",
		"a    b                        ",
		"a                             ",
		"b                             ",
		"                              ",
	}
	if got := strList(m["scrollback"].(map[string]any)["grid"]); !reflect.DeepEqual(got, want) {
		t.Errorf("rows:\n%q\nwant\n%q", got, want)
	}
	// Text format: an === scrollback === section after the events, or none.
	code, out, _ := runCLI("play", inT294, "--data", inT294Dat, "--cols", "30", "--rows", "24", "--inline", "--script", script)
	if code != 0 || !strings.Contains(out, "=== events ===\nnone\n=== scrollback ===\nYou: hello                    \nok                            \n") {
		t.Errorf("text form:\n%s", out)
	}
	_, out, _ = runCLI("play", inT294, "--data", inT294Dat, "--inline")
	if !strings.HasSuffix(out, "=== scrollback ===\nnone\n") {
		t.Errorf("text form without commits:\n%s", out)
	}
	// The --input form: a commit: step takes the rest of the token.
	m = playMap(t, inT294, "--data", inT294Dat, "--cols", "30", "--inline", "--input", `commit:{"lines":["x"]}`)
	if got := strList(m["scrollback"].(map[string]any)["grid"]); len(got) != 1 || !strings.HasPrefix(got[0], "x ") {
		t.Errorf("--input commit: %q", got)
	}
}

// SPEC v0.4d test 296: committing a template.
func TestD4CommitTemplate296(t *testing.T) {
	step := `{"commit": {"template": "card", "with": "last as m", "props": {"who": "Claude"}}}`
	run := func(extra ...string) []string {
		args := []string{inCard, "--data", inT294Dat, "--cols", "30", "--rows", "24", "--inline", "--script", inScript(t, append([]string{step}, extra...)...)}
		m := playMap(t, args...)
		return strList(m["scrollback"].(map[string]any)["grid"])
	}
	if got := run(); !reflect.DeepEqual(got, []string{"Claude: done                  "}) {
		t.Errorf("rows %q", got)
	}
	// A set: after it does not change the row.
	if got := run(`{"set": {"path": "last.text", "value": "x"}}`); !reflect.DeepEqual(got, []string{"Claude: done                  "}) {
		t.Errorf("after a set: %q", got)
	}
	// {path} removes a TAB.
	dir := t.TempDir()
	data := filepath.Join(dir, "tab.json")
	writeFile(t, data, `{"draft": "", "status": "ready", "last": {"text": "a\tb"}}`)
	m := playMap(t, inCard, "--data", data, "--cols", "30", "--inline", "--script", inScript(t, step))
	if got := strList(m["scrollback"].(map[string]any)["grid"]); !strings.HasPrefix(got[0], "Claude: ab ") {
		t.Errorf("TAB in {path}: %q", got)
	}
	// An input in the copy: no V012, painted unfocused.
	field := `{"commit": {"template": "field", "with": "last as m"}}`
	code, out, errw := runCLI("play", inCard, "--data", inT294Dat, "--cols", "30", "--rows", "24", "--inline", "--format", "json", "--script", inScript(t, field))
	if code != 0 {
		t.Fatalf("a template with an input: exit %d %s%s", code, out, errw)
	}
	pm := jsonMap(t, out)
	if got := strList(pm["scrollback"].(map[string]any)["grid"]); len(got) != 1 || !strings.HasPrefix(got[0], "done") {
		t.Errorf("the input's row: %q", got)
	}
	if pm["focus"] != "composer" {
		t.Errorf("focus = %v, the commit moved it", pm["focus"])
	}
}

// SPEC v0.4d test 297: commit errors are the usage error of the step, and
// nothing is printed.
func TestD4CommitErrors297(t *testing.T) {
	for _, c := range []struct{ step, reason string }{
		{`{"commit": {}}`, `commit: give either lines or template`},
		{`{"commit": {"lines": ["a"], "template": "card"}}`, `commit: give either lines or template`},
		{`{"commit": {"lines": ["a"], "with": "x as y"}}`, `commit: with and props need a template`},
		{`{"commit": {"lines": ["a"], "props": {"p": "v"}}}`, `commit: with and props need a template`},
		{`{"commit": {"lines": ["a", 5]}}`, `commit: line 1 is a number, not a string or an array of runs`},
		{`{"commit": {"lines": [["a", true]]}}`, `commit: line 0: run 1 is a boolean, not a string or an object`},
		{`{"commit": {"lines": [[{"class": "b"}]]}}`, `commit: line 0: run 0 has no string "text"`},
		{`{"commit": {"lines": [[{"text": "x", "class": 3}]]}}`, `commit: line 0: run 0 has a "class" that is not a list of class names`},
		{`{"commit": {"lines": [[{"text": "x", "class": "9bad"}]]}}`, `commit: line 0: run 0 has a "class" that is not a list of class names`},
		{`{"commit": {"template": "nope"}}`, `commit: no <template id="nope">`},
		{`{"commit": {"template": "card", "with": "nothing as m", "props": {"who": "C"}}}`, `commit: with path "nothing" (alias m) is missing`},
		{`{"commit": {"template": "card", "with": "last as m", "props": {"whom": "C"}}}`, `commit: prop "whom" is not declared by <template id="card">`},
		{`{"commit": {"template": "card", "with": "last as m", "props": {"who": "{last.text}"}}}`, `commit: prop "who" makes the pair "{last.text}": a prop is text; pass a path with with="…"`},
		{`{"commit": {"template": "card", "with": "last as m,last as n"}}`, `commit: with="last as m,last as n" is malformed (want "path as alias", clauses separated by ", ")`},
		{`{"commit": {"template": "card", "with": "last as m, other as m"}}`, `commit: with="last as m, other as m" is malformed (want "path as alias", clauses separated by ", ")`},
		{`{"commit": {"template": "card", "with": "last as m", "props": {"id": "x"}}}`, `commit: prop "id" is not declared by <template id="card">`},
	} {
		script := inScript(t, c.step)
		code, out, errw := runCLI("play", inCard, "--data", inT294Dat, "--cols", "30", "--rows", "24", "--inline", "--format", "json", "--script", script)
		want := fmt.Sprintf("tuimark: play: step 1 (%s): %s\n", c.step, c.reason)
		if code != 1 || out != "" || errw != want {
			t.Errorf("%s:\nexit %d stdout %q stderr %q\nwant stderr %q", c.step, code, out, errw, want)
		}
	}
	// Without --inline.
	step := `{"commit": {"lines": ["a"]}}`
	code, out, errw := runCLI("play", inCard, "--data", inT294Dat, "--format", "json", "--script", inScript(t, step))
	if want := "tuimark: play: step 1 (" + step + "): commit: not in inline mode\n"; code != 1 || out != "" || errw != want {
		t.Errorf("without --inline: exit %d %q %q", code, out, errw)
	}
	// Also through --input, and an unknown member.
	code, _, errw = runCLI("play", inCard, "--data", inT294Dat, "--inline", "--input", `commit:{"lines":["a"],"x":1}`)
	if code != 1 || !strings.Contains(errw, `commit: unknown member "x"`) {
		t.Errorf("unknown member: %d %q", code, errw)
	}
	// The members the script errors name include commit.
	code, _, errw = runCLI("play", inCard, "--script", inScript(t, `{}`))
	if code != 1 || !strings.Contains(errw, "wheel with at, theme, tick, commit, append)") {
		t.Errorf("no known step member: %d %q", code, errw)
	}
	// A commit that fails prints nothing even after earlier commits made
	// no change: the earlier ones are not in the output either, since the
	// usage error prints no dump.
	script := inScript(t, `{"commit": {"lines": ["ok"]}}`, `{"commit": {"lines": [1]}}`)
	code, out, errw = runCLI("play", inCard, "--data", inT294Dat, "--inline", "--format", "json", "--script", script)
	if code != 1 || out != "" || !strings.Contains(errw, "step 2 ") {
		t.Errorf("second step: %d %q %q", code, out, errw)
	}
}

// SPEC v0.4d test 298: inline and pinned documents (the host line is D5a's).
func TestD4Pinned298(t *testing.T) {
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"dump", inPinned, "--inline"}, "tuimark: dump: inline mode needs a <tui> without version\n"},
		{[]string{"play", inPinned, "--inline"}, "tuimark: play: inline mode needs a <tui> without version\n"},
		{[]string{"inspect", inPinned, "--inline", "--at", "0,0"}, "tuimark: inspect: inline mode needs a <tui> without version\n"},
		{[]string{"run", inPinned, "--inline"}, "tuimark: run: inline mode needs a <tui> without version\n"},
	} {
		code, out, errw := runCLI(c.args...)
		if code != 1 || out != "" || errw != c.want {
			t.Errorf("%v: exit %d stdout %q stderr %q", c.args, code, out, errw)
		}
	}
	// A current-language document with a pinned twin: the same flag works.
	if code, _, errw := runCLI("dump", inT294, "--data", inT294Dat, "--inline"); code != 0 {
		t.Errorf("dump --inline of the current language: %d %s", code, errw)
	}
}

// SPEC v0.4d test 300: the layers are laid out against the region, and the
// mouse is off.
func TestD4Layers300(t *testing.T) {
	m := dumpMap(t, inModal, "--data", inT294Dat, "--cols", "30", "--rows", "24", "--inline")
	if m["rows"] != 3.0 {
		t.Errorf("rows %v", m["rows"])
	}
	n := nodeOfMap(t, m, "m")
	if n["tag"] != "modal" || n["x"] != 13.0 || n["y"] != 0.0 || n["w"] != 3.0 || n["h"] != 3.0 || n["focused"] != true || n["clipped"] != "children" {
		t.Errorf("modal %v", n)
	}
	wantGrid := []string{"─────────────┌─┐──────────────", "Message      │a│              ", "ready        └─┘              "}
	if got := gridOfMap(m); !reflect.DeepEqual(got, wantGrid) {
		t.Errorf("grid %q", got)
	}
	errs := m["errors"].([]any)
	if len(errs) != 1 {
		t.Fatalf("errors %v", errs)
	}
	e := errs[0].(map[string]any)
	if e["severity"] != "warning" || e["code"] != "L009" || e["msg"] != "cuts text[1] on y: 0 of 1 cells shown; give it room, or write overflow: hidden to keep the cut" || e["id"] != "m" {
		t.Errorf("diagnostic %v", e)
	}
	// Without its style the modal is sized by its content and capped at
	// 80 % of the region's 3 rows.
	src, err := os.ReadFile(inModal)
	if err != nil {
		t.Fatal(err)
	}
	path := writeTemp(t, "modal2.tui", strings.Replace(string(src), ` style="height: 10"`, "", 1))
	m = dumpMap(t, path, "--data", filepath.Join("..", "..", "testdata", "v04d", "t294", "in.json"), "--cols", "30", "--rows", "24", "--inline")
	n = nodeOfMap(t, m, "m")
	if n["h"] != 2.0 || n["x"] != 13.0 || n["y"] != 0.0 {
		t.Errorf("content-sized modal %v", n)
	}
	if e := m["errors"].([]any)[0].(map[string]any); !strings.HasPrefix(e["msg"].(string), "cuts text[0] on y:") {
		t.Errorf("diagnostic %v", e)
	}
	// A click: step under play --inline changes nothing: the mouse is off
	// whatever mouse says.
	m = playMap(t, inClick, "--cols", "20", "--rows", "10", "--inline", "--input", "click:2,0")
	if evs := m["events"].([]any); len(evs) != 0 {
		t.Errorf("click under --inline fired %v", evs)
	}
	m = playMap(t, inClick, "--cols", "20", "--rows", "10", "--input", "click:2,0")
	if evs := m["events"].([]any); len(evs) != 1 {
		t.Errorf("click in full screen fired %v", evs)
	}
}

// SPEC v0.4d test 315: a region taller than the terminal shares the
// terminal's rows as a content-sized modal's children do, and streaming.
func TestD4Tall315(t *testing.T) {
	m := dumpMap(t, inOvl, "--data", inOvlDat, "--cols", "30", "--rows", "12", "--inline")
	if m["rows"] != 12.0 || m["terminal_rows"] != 12.0 || len(m["errors"].([]any)) != 0 {
		t.Errorf("rows %v terminal_rows %v errors %v", m["rows"], m["terminal_rows"], m["errors"])
	}
	live := nodeOfMap(t, m, "live")
	if live["y"] != 0.0 || live["w"] != 30.0 || live["h"] != 10.0 {
		t.Errorf("scroll %v", live)
	}
	if sc := live["scroll"].(map[string]any); sc["y"] != 20.0 || sc["h"] != 30.0 {
		t.Errorf("scroll %v", sc)
	}
	if c := nodeOfMap(t, m, "composer"); c["y"] != 10.0 || c["h"] != 1.0 {
		t.Errorf("composer %v", c)
	}
	if s := nodeOfMap(t, m, "status"); s["y"] != 11.0 || s["h"] != 1.0 {
		t.Errorf("status %v", s)
	}
	grid := gridOfMap(m)
	for i := 0; i < 10; i++ {
		if want := fmt.Sprintf("%-30s", fmt.Sprintf("line %d", 21+i)); grid[i] != want {
			t.Errorf("grid[%d] = %q, want %q", i, grid[i], want)
		}
	}
	if grid[10] != fmt.Sprintf("%-30s", "Message") || grid[11] != fmt.Sprintf("%-30s", "ready") {
		t.Errorf("grid tail %q", grid[10:])
	}
	// A col shows the start, cut, with L009 on the col.
	m = dumpMap(t, inOvlCol, "--data", inOvlDat, "--cols", "30", "--rows", "12", "--inline")
	col := nodeOfMap(t, m, "live")
	if col["y"] != 0.0 || col["w"] != 30.0 || col["h"] != 10.0 {
		t.Errorf("col %v", col)
	}
	errs := m["errors"].([]any)
	if len(errs) != 1 {
		t.Fatalf("col errors %v", errs)
	}
	if e := errs[0].(map[string]any); e["code"] != "L009" || e["path"] != "/screen/col" || e["msg"] != "cuts text on y: 10 of 30 cells shown; give it room, or write overflow: hidden to keep the cut" {
		t.Errorf("col diagnostic %v", e)
	}
	if g := gridOfMap(m); g[0] != fmt.Sprintf("%-30s", "line 1") || g[9] != fmt.Sprintf("%-30s", "line 10") {
		t.Errorf("col grid %q", g)
	}
	// A box never shrinks: the composer and the status fall below.
	m = dumpMap(t, inOvlBox, "--data", inOvlDat, "--cols", "30", "--rows", "12", "--inline")
	if box := nodeOfMap(t, m, "live"); box["h"] != 30.0 {
		t.Errorf("box %v", box)
	}
	if c := nodeOfMap(t, m, "composer"); c["y"] != 30.0 {
		t.Errorf("composer %v", c)
	}
	errs = m["errors"].([]any)
	if len(errs) != 1 {
		t.Fatalf("box errors %v", errs)
	}
	if e := errs[0].(map[string]any); e["code"] != "L009" || e["path"] != "/screen" || e["msg"] != "cuts #live on y: 12 of 30 cells shown; #live is sized by its content and does not shrink: give it height: 1fr (or flex: 1), or write overflow: hidden to keep the cut" {
		t.Errorf("box diagnostic %v", e)
	}
	// Streaming: a finished block is committed, then the write that removes
	// it from the region; the block is printed once and the region leaves no
	// gap.
	m = playMap(t, inStr, "--data", inStrDat, "--cols", "30", "--rows", "24", "--inline", "--frames", "--script", "../../testdata/v04d/t315/stream.ndjson")
	if got := strList(m["scrollback"].(map[string]any)["grid"]); !reflect.DeepEqual(got, []string{"first block                   "}) {
		t.Errorf("scrollback %q", got)
	}
	frames := m["frames"].([]any)
	f1 := frames[1].(map[string]any)
	d1 := f1["dump"].(map[string]any)
	if f1["scrollback_rows"] != 1.0 || d1["rows"] != 3.0 || strList(d1["grid"])[0] != "first block                   " {
		t.Errorf("frame 1: scrollback_rows %v rows %v grid %q", f1["scrollback_rows"], d1["rows"], d1["grid"])
	}
	f2 := frames[2].(map[string]any)
	if f2["scrollback_rows"] != 1.0 || !reflect.DeepEqual(strList(f2["dump"].(map[string]any)["grid"]), []string{"tail                          ", "streaming                     "}) {
		t.Errorf("frame 2: %v", f2)
	}
}

// SPEC v0.4d §15.5 and §15.8, decision 250: the manifest's inline member
// and the MCP tools' inline argument give what the commands print.
func TestD4ManifestAndMCP(t *testing.T) {
	dir := t.TempDir()
	abs := func(p string) string { a, _ := filepath.Abs(p); return a }
	writeManifest(t, dir, `[{"name":"d","file":"`+abs(inT294)+`","data":"`+abs(inT294Dat)+`","sizes":["30x24"],"json":["30x24"],"inline":true},
{"name":"p","file":"`+abs(inT294)+`","data":"`+abs(inT294Dat)+`","sizes":["30x24"],"json":["30x24"],"inline":true,"styles":true,"script":"`+inScript(t, `{"commit": {"lines": ["hi"]}}`)+`"}]`)
	if code, out, errw := runCLI("test", dir, "--update"); code != 0 {
		t.Fatalf("update: exit %d %s%s", code, out, errw)
	}
	b, err := os.ReadFile(filepath.Join(dir, "d", "30x24.json"))
	if err != nil {
		t.Fatal(err)
	}
	if m := jsonMap(t, string(b)); m["rows"] != 3.0 || m["terminal_rows"] != 24.0 {
		t.Errorf("dump golden rows %v terminal_rows %v", m["rows"], m["terminal_rows"])
	}
	b, err = os.ReadFile(filepath.Join(dir, "p", "30x24.json"))
	if err != nil {
		t.Fatal(err)
	}
	if m := jsonMap(t, string(b)); m["scrollback"].(map[string]any)["grid"].([]any)[0] != "hi                            " {
		t.Errorf("play golden scrollback %v", m["scrollback"])
	}
	if code, out, _ := runCLI("test", dir); code != 0 || !strings.Contains(out, "PASS d 30x24") || !strings.Contains(out, "PASS p 30x24") {
		t.Errorf("goldens do not pass: %d %s", code, out)
	}
	// An entry whose document writes version fails with the error of --inline.
	writeManifest(t, dir, `[{"name":"x","file":"`+abs(inPinned)+`","sizes":["30x5"],"inline":true}]`)
	if code, out, _ := runCLI("test", dir); code != 1 || !strings.Contains(out, "FAIL x 30x5: dump: inline mode needs a <tui> without version") {
		t.Errorf("pinned entry: %d %s", code, out)
	}

	// MCP: dump, play, and inspect take inline and pass --inline.
	for name, want := range map[string][]string{
		"tuimark_dump":    {"dump", inT294, "--data", inT294Dat, "--cols", "30", "--rows", "24", "--inline", "--format", "json"},
		"tuimark_inspect": {"inspect", inT294, "--data", inT294Dat, "--cols", "30", "--rows", "24", "--at", "3,2", "--inline", "--json"},
	} {
		args := map[string]any{"file": inT294, "data": inT294Dat, "cols": 30, "rows": 24, "inline": true}
		if name == "tuimark_inspect" {
			args["at"] = "3,2"
		}
		params, _ := json.Marshal(map[string]any{"name": name, "arguments": args})
		got, isErr := mcpCall(t, string(params))
		wantOut, wantErr := cliResult(want...)
		if got != wantOut || isErr != wantErr {
			t.Errorf("%s: MCP %q (%v), CLI %q (%v)", name, got, isErr, wantOut, wantErr)
		}
	}
	commit := `commit:{"lines": ["You: hello", [{"text": "ok", "class": "good"}]]}`
	params, _ := json.Marshal(map[string]any{"name": "tuimark_play", "arguments": map[string]any{
		"file": inT294, "data": inT294Dat, "cols": 30, "rows": 24, "inline": true, "styles": true, "steps": []string{commit},
	}})
	got, isErr := mcpCall(t, string(params))
	script := inScript(t, `{"commit": {"lines": ["You: hello", [{"text": "ok", "class": "good"}]]}}`)
	want, wantErr := cliResult("play", inT294, "--data", inT294Dat, "--cols", "30", "--rows", "24", "--styles", "--inline", "--format", "json", "--script", script)
	// The step's input differs (it is the raw step) only in frames, which
	// this call does not ask for.
	if isErr || wantErr || got != want {
		t.Errorf("tuimark_play: MCP %q (%v), CLI %q (%v)", got, isErr, want, wantErr)
	}
	// The input schema lists inline.
	for _, tool := range mcpTools {
		has := false
		for _, p := range tool.Props {
			has = has || p.Name == "inline"
		}
		if want := tool.Name == "tuimark_dump" || tool.Name == "tuimark_play" || tool.Name == "tuimark_inspect"; has != want {
			t.Errorf("%s has inline = %v", tool.Name, has)
		}
	}
}
