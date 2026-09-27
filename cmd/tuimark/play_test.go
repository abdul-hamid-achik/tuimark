package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// playFixture is a small self-contained document exercising an input, a
// bound list, and a keymap quit binding, isolated from examples/** (owned
// by other agents) so these tests never depend on it.
const playFixtureTUI = `<tui version="1">
  <style>
    input:focus { color: $accent; }
  </style>
  <keymap>
    <bind keys="q" action="quit"/>
  </keymap>
  <screen id="main" focus="#query">
    <col width="100%" height="100%">
      <input id="query" placeholder="search" on:change="filter"/>
      <list id="rows" each="items as item" key="item" on:select="open">
        <item><text>{item}</text></item>
      </list>
    </col>
  </screen>
</tui>`

const playFixtureData = `{"items": ["a", "b", "c"]}`

func writePlayFixture(t *testing.T) (tui, data string) {
	t.Helper()
	dir := t.TempDir()
	tui = filepath.Join(dir, "app.tui")
	data = filepath.Join(dir, "data.json")
	writeFile(t, tui, playFixtureTUI)
	writeFile(t, data, playFixtureData)
	return tui, data
}

// writeFile is writeTemp without a fresh tempdir per call, so several
// related files (a document plus its data or a script) can share one.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// --- D1: tuimark play ----------------------------------------------------

func TestPlayTextCoalescesIntoOneChangeEvent(t *testing.T) {
	tui, data := writePlayFixture(t)
	code, out, errw := runCLI("play", tui, "--data", data, "--input", "text:hi")
	if code != 0 {
		t.Fatalf("exit %d, want 0; stderr=%s", code, errw)
	}
	if strings.Count(out, "filter") != 1 {
		t.Fatalf("want exactly one filter event, got:\n%s", out)
	}
	if !strings.Contains(out, `1 filter query {} "hi"`) {
		t.Errorf("want the on:change event with value \"hi\" at step 1:\n%s", out)
	}
}

// finding 23 / SPEC v0.2 §15.4 (`text:STR` "decoded by the §26.8 decoder"),
// §26.8 ("Only keys ... and pastes ... come out of it"), §28.21 ("text:
// goes through it in one read"; play events are "exactly what Run would
// dispatch"). A `text:` step whose bytes contain a bracketed-paste marker
// must decode the paste as a paste (one edit, one on:change), not drop it:
// before the fix, runStep decoded text: with host.DecodeKeys, which
// discards paste inputs by design, so the "XY" here was silently lost.
func TestPlayTextStepDeliversBracketedPastes(t *testing.T) {
	tui, data := writePlayFixture(t)
	scriptPath := filepath.Join(t.TempDir(), "s.ndjson")
	writeFile(t, scriptPath, `{"text":"ab\u001b[200~XY\u001b[201~cd"}`)

	code, out, errw := runCLI("play", tui, "--data", data, "--script", scriptPath, "--format", "json")
	if code != 0 {
		t.Fatalf("exit %d, want 0; stderr=%s", code, errw)
	}
	var d struct {
		Events []struct {
			Action string `json:"action"`
			Value  any    `json:"value"`
		} `json:"events"`
	}
	mustUnmarshal(t, out, &d)
	// Run would dispatch three on:change events for this one read: "ab"
	// (coalesced keys), "abXY" (the paste, one edit), "abXYcd" (coalesced
	// keys again) — never a value that lost the pasted "XY".
	if len(d.Events) != 3 {
		t.Fatalf("want 3 filter events (ab / abXY / abXYcd), got %d:\n%s", len(d.Events), out)
	}
	wantValues := []string{"ab", "abXY", "abXYcd"}
	for i, want := range wantValues {
		if d.Events[i].Action != "filter" || d.Events[i].Value != want {
			t.Errorf("event %d = %+v, want filter %q", i, d.Events[i], want)
		}
	}

	// A text: step that is only a bracketed paste (no surrounding keys)
	// must still deliver it as a paste: one edit, one on:change.
	scriptPath2 := filepath.Join(t.TempDir(), "s2.ndjson")
	writeFile(t, scriptPath2, `{"text":"\u001b[200~pasted q\u001b[201~"}`)
	code, out, errw = runCLI("play", tui, "--data", data, "--script", scriptPath2, "--format", "json")
	if code != 0 {
		t.Fatalf("exit %d, want 0; stderr=%s", code, errw)
	}
	var d2 struct {
		Events []struct {
			Action string `json:"action"`
			Value  any    `json:"value"`
		} `json:"events"`
	}
	mustUnmarshal(t, out, &d2)
	if len(d2.Events) != 1 || d2.Events[0].Action != "filter" || d2.Events[0].Value != "pasted q" {
		t.Fatalf("want exactly one filter event with value \"pasted q\", got %+v:\n%s", d2.Events, out)
	}
}

func TestPlayTabMovesFocusWithNoEvent(t *testing.T) {
	tui, data := writePlayFixture(t)
	code, out, errw := runCLI("play", tui, "--data", data, "--input", "tab", "--format", "json")
	if code != 0 {
		t.Fatalf("exit %d, want 0; stderr=%s", code, errw)
	}
	var d struct {
		Focus  string `json:"focus"`
		Events []any  `json:"events"`
	}
	mustUnmarshal(t, out, &d)
	if d.Focus != "rows" {
		t.Errorf("focus = %q, want rows", d.Focus)
	}
	if len(d.Events) != 0 {
		t.Errorf("want no events for a tab with no on:focus, got %v", d.Events)
	}
}

func TestPlayDownFiresOnSelect(t *testing.T) {
	tui, data := writePlayFixture(t)
	code, out, errw := runCLI("play", tui, "--data", data, "--input", "tab down", "--format", "json")
	if code != 0 {
		t.Fatalf("exit %d, want 0; stderr=%s", code, errw)
	}
	if !strings.Contains(out, `"action": "open"`) || !strings.Contains(out, `"item": "b"`) {
		t.Errorf("want an open event with keys.item=\"b\":\n%s", out)
	}
}

func TestPlayPasteIntoInputIsOneEditAndNeverQuits(t *testing.T) {
	tui, data := writePlayFixture(t)
	code, out, errw := runCLI("play", tui, "--data", data, "--input", "paste:q,and,x", "--format", "json")
	if code != 0 {
		t.Fatalf("exit %d, want 0; stderr=%s", code, errw)
	}
	if !strings.Contains(out, `"value": "q,and,x"`) {
		t.Errorf("want the paste delivered verbatim (normalized) into the input:\n%s", out)
	}
	if strings.Contains(out, `"action": "quit"`) {
		t.Fatal("a paste containing q must never fire the built-in quit")
	}
}

func TestPlayPasteOffAnInputIsDiscarded(t *testing.T) {
	tui, data := writePlayFixture(t)
	// tab moves focus onto the list; a paste there must do nothing at all.
	code, out, errw := runCLI("play", tui, "--data", data, "--input", "tab paste:zzz", "--format", "json")
	if code != 0 {
		t.Fatalf("exit %d, want 0; stderr=%s", code, errw)
	}
	var d struct {
		Events []any `json:"events"`
	}
	mustUnmarshal(t, out, &d)
	if len(d.Events) != 0 {
		t.Errorf("a paste off an input must fire nothing, got %v", d.Events)
	}
}

func TestPlayQuitEndsSessionAndSkipsRemainingSteps(t *testing.T) {
	tui, data := writePlayFixture(t)
	code, out, errw := runCLI("play", tui, "--data", data, "--input", "tab ctrl+c down", "--format", "json")
	if code != 0 {
		t.Fatalf("exit %d, want 0; stderr=%s", code, errw)
	}
	if !strings.Contains(out, `"action": "quit"`) {
		t.Fatalf("want a quit event:\n%s", out)
	}
	if strings.Contains(out, `"action": "open"`) {
		t.Fatal("the down step after ctrl+c must not have been applied")
	}
}

func TestPlayKeymapQuitAlsoEndsSession(t *testing.T) {
	tui, data := writePlayFixture(t)
	code, out, errw := runCLI("play", tui, "--data", data, "--input", "tab q down", "--format", "json")
	if code != 0 {
		t.Fatalf("exit %d, want 0; stderr=%s", code, errw)
	}
	if !strings.Contains(out, `"action": "quit"`) || strings.Contains(out, `"action": "open"`) {
		t.Errorf("want the keymap's q to quit and stop before down:\n%s", out)
	}
}

// finding 31 / SPEC §8.1 ("ctrl+i, ctrl+j, and ctrl+m arrive as tab/enter
// and never match"), §15.4 KEY step ("delivered as one read holding that
// key"). ctrl+i (0x09), ctrl+j (0x0a), and ctrl+m (0x0d) are decoded by
// the terminal decoder as tab/enter/enter before the generic ctrl+<letter>
// case ever runs, so a keymap bound to ctrl+i/ctrl+j/ctrl+m can never fire
// under Run. Before the fix, keyFromToken mapped the play step's literal
// token instead, so `play --input ctrl+m` fired a binding Run could never
// reach for the same key, and pressing ctrl+m for real fired a different
// binding (enter's) that `play ctrl+m` never exercised.
func TestPlayCtrlIJMArriveAsTabAndEnter(t *testing.T) {
	doc := `<tui version="1">
  <keymap>
    <bind keys="ctrl+m" action="cm"/>
    <bind keys="ctrl+i" action="ci"/>
    <bind keys="enter" action="ent"/>
    <bind keys="tab" action="tb"/>
  </keymap>
  <screen id="main"><text>x</text></screen>
</tui>`
	file := writeTemp(t, "ctrl.tui", doc)

	code, out, errw := runCLI("play", file, "--input", "ctrl+m ctrl+i", "--format", "json")
	if code != 0 {
		t.Fatalf("exit %d, want 0; stderr=%s", code, errw)
	}
	var d struct {
		Events []struct {
			Action string `json:"action"`
		} `json:"events"`
	}
	mustUnmarshal(t, out, &d)
	if len(d.Events) != 2 {
		t.Fatalf("want 2 events, got %d:\n%s", len(d.Events), out)
	}
	// ctrl+m decodes as enter, and ctrl+i as tab: the events must be
	// "ent"/"tb" (what Run would actually dispatch), never "cm"/"ci"
	// (bindings Run can never reach through these bytes).
	if d.Events[0].Action != "ent" {
		t.Errorf("ctrl+m step: action = %q, want ent (ctrl+m decodes as enter)", d.Events[0].Action)
	}
	if d.Events[1].Action != "tb" {
		t.Errorf("ctrl+i step: action = %q, want tb (ctrl+i decodes as tab)", d.Events[1].Action)
	}

	// The equivalent text: step (through the §26.8 byte decoder) must
	// dispatch the same "ent" event for the raw CR byte, so a key step
	// never disagrees with what text:/Run would do for the same input.
	scriptPath := filepath.Join(t.TempDir(), "s.ndjson")
	writeFile(t, scriptPath, `{"text":"\r"}`)
	code2, out2, errw2 := runCLI("play", file, "--script", scriptPath, "--format", "json")
	if code2 != 0 {
		t.Fatalf("exit %d, want 0; stderr=%s", code2, errw2)
	}
	var d2 struct {
		Events []struct {
			Action string `json:"action"`
		} `json:"events"`
	}
	mustUnmarshal(t, out2, &d2)
	if len(d2.Events) != 1 || d2.Events[0].Action != "ent" {
		t.Errorf("text:\\r step: events = %+v, want one \"ent\" event", d2.Events)
	}
}

func TestPlaySetFocusAndResizeSteps(t *testing.T) {
	tui, data := writePlayFixture(t)
	code, out, errw := runCLI("play", tui, "--data", data, "--input", `set:items=["x","y"] focus:query resize:40x10`, "--format", "json")
	if code != 0 {
		t.Fatalf("exit %d, want 0; stderr=%s", code, errw)
	}
	var d struct {
		Cols  int      `json:"cols"`
		Rows  int      `json:"rows"`
		Focus string   `json:"focus"`
		Grid  []string `json:"grid"`
	}
	mustUnmarshal(t, out, &d)
	if d.Cols != 40 || d.Rows != 10 {
		t.Errorf("cols/rows = %d/%d, want 40/10 after resize:40x10", d.Cols, d.Rows)
	}
	if d.Focus != "query" {
		t.Errorf("focus = %q, want query after focus:query", d.Focus)
	}
	if !strings.Contains(strings.Join(d.Grid, "\n"), "x") {
		t.Errorf("want the new items reflected after set:, grid:\n%s", strings.Join(d.Grid, "\n"))
	}
}

func TestPlaySetBadPathIsUsageError(t *testing.T) {
	tui, data := writePlayFixture(t)
	code, out, errw := runCLI("play", tui, "--data", data, "--input", "set:@screen=\"nope\"")
	if code != 1 {
		t.Fatalf("exit %d, want 1; stdout=%s stderr=%s", code, out, errw)
	}
	if out != "" {
		t.Errorf("a usage error must print nothing to stdout, got:\n%s", out)
	}
	if !strings.Contains(errw, "play: step 1") {
		t.Errorf("want the step-number-and-input usage error format, got:\n%s", errw)
	}
}

func TestPlayInvalidKeyTokenIsUsageError(t *testing.T) {
	tui, data := writePlayFixture(t)
	code, _, errw := runCLI("play", tui, "--data", data, "--input", "slash")
	if code != 1 {
		t.Fatalf("exit %d, want 1; stderr=%s", code, errw)
	}
	if !strings.Contains(errw, "step 1 (slash)") {
		t.Errorf("want the offending step named, got:\n%s", errw)
	}
}

// SPEC v0.2b §21 test 28 as amended (mouse steps are valid, test 47): on a
// document without the mouse they change nothing and fire nothing.
func TestPlayMouseStepsWithoutMouseDoNothing(t *testing.T) {
	tui, data := writePlayFixture(t)
	_, base, _ := runCLI("play", tui, "--data", data)
	for _, step := range []string{"click:1,2", "wheel-up:1,2", "wheel-down:1,2"} {
		code, out, errw := runCLI("play", tui, "--data", data, "--input", step)
		if code != 0 || out != base {
			t.Errorf("--input %s: exit %d, stderr %q; the output differs from no steps:\n%s", step, code, errw, out)
		}
	}
}

func TestPlayInputAndScriptAreMutuallyExclusive(t *testing.T) {
	tui, data := writePlayFixture(t)
	scriptPath := filepath.Join(t.TempDir(), "s.ndjson")
	writeFile(t, scriptPath, `{"key":"down"}`+"\n")
	code, _, errw := runCLI("play", tui, "--data", data, "--input", "down", "--script", scriptPath)
	if code != 1 || !strings.Contains(errw, "mutually exclusive") {
		t.Fatalf("exit %d, stderr %q, want exit 1 mentioning mutually exclusive", code, errw)
	}
}

func TestPlayScriptSteps(t *testing.T) {
	tui, data := writePlayFixture(t)
	scriptPath := filepath.Join(t.TempDir(), "s.ndjson")
	writeFile(t, scriptPath, strings.Join([]string{
		`{"text":"hola mundo"}`,
		``, // blank lines are ignored
		`{"key":"down"}`,
		`{"paste":"a\nb"}`,
		`{"set":{"path":"items","value":["p","q"]}}`,
		`{"focus":"#query"}`,
		`{"resize":[60,24]}`,
	}, "\n"))
	code, out, errw := runCLI("play", tui, "--data", data, "--script", scriptPath, "--format", "json")
	if code != 0 {
		t.Fatalf("exit %d, want 0; stderr=%s\n%s", code, errw, out)
	}
	var d struct {
		Cols int `json:"cols"`
	}
	mustUnmarshal(t, out, &d)
	if d.Cols != 60 {
		t.Errorf("cols = %d, want 60 after {\"resize\":[60,24]}", d.Cols)
	}
}

// SPEC v0.2b §15.4 (amended by Phase 6, test 47): {"theme": …} is a step
// (TestPlayThemeSteps), and so are the mouse members (TestPlayMouseSteps
// in p5_test.go).
func TestPlayScriptMouseMembersAreSteps(t *testing.T) {
	tui, data := writePlayFixture(t)
	for _, line := range []string{`{"click":[1,2]}`, `{"wheel":"up","at":[1,2]}`, `{"at":[1,2],"wheel":"down"}`} {
		scriptPath := filepath.Join(t.TempDir(), "s.ndjson")
		writeFile(t, scriptPath, line)
		code, _, errw := runCLI("play", tui, "--data", data, "--script", scriptPath)
		if code != 0 {
			t.Errorf("script line %s: exit %d, stderr %q, want 0", line, code, errw)
		}
	}
}

// finding 24 / SPEC v0.2 §15.4 ("Each object holds exactly one of these
// members"), Errors ("a step that cannot be parsed or applied ... is a
// usage error"). Before the fix, parseScriptLine's plain json.Unmarshal
// into a struct with pointer fields matched member names
// case-insensitively, silently dropped unknown members and extra array
// elements, and defaulted a missing `set` path/value to "" (the store
// root) or null — so a typo could silently rewrite the whole store or
// resize to the wrong size instead of failing.
func TestPlayScriptStrictlyValidatesStepObjects(t *testing.T) {
	tui, data := writePlayFixture(t)
	cases := []struct {
		name, line string
	}{
		{"set path typo defaults to root", `{"set":{"pth":"items","value":["zz"]}}`},
		{"set missing value", `{"set":{"path":"items"}}`},
		{"set missing path", `{"set":{"value":["zz"]}}`},
		{"uppercase KEY", `{"KEY":"tab"}`},
		{"mixed-case Key", `{"Key":"tab"}`},
		{"unknown extra member", `{"key":"tab","extra":1}`},
		{"duplicate member", `{"key":"tab","key":"down"}`},
		{"resize with extra element", `{"resize":[30,8,5]}`},
		{"resize with too few elements", `{"resize":[30]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scriptPath := filepath.Join(t.TempDir(), "s.ndjson")
			writeFile(t, scriptPath, tc.line)
			code, out, errw := runCLI("play", tui, "--data", data, "--script", scriptPath, "--format", "json")
			if code != 1 {
				t.Fatalf("%s: exit %d, want 1; stdout=%s stderr=%s", tc.line, code, out, errw)
			}
			if out != "" {
				t.Errorf("%s: a usage error must print nothing to stdout, got:\n%s", tc.line, out)
			}
		})
	}
}

func TestPlayFramesIncludesOneEntryPerAppliedStep(t *testing.T) {
	tui, data := writePlayFixture(t)
	code, out, errw := runCLI("play", tui, "--data", data, "--input", "tab down", "--frames", "--format", "json")
	if code != 0 {
		t.Fatalf("exit %d, want 0; stderr=%s", code, errw)
	}
	var d struct {
		Frames []struct {
			Step  int    `json:"step"`
			Input string `json:"input"`
		} `json:"frames"`
	}
	mustUnmarshal(t, out, &d)
	if len(d.Frames) != 3 {
		t.Fatalf("want 3 frames (step 0 + 2 applied steps), got %d: %+v", len(d.Frames), d.Frames)
	}
	if d.Frames[0].Step != 0 || d.Frames[0].Input != "" {
		t.Errorf("frame 0 = %+v, want step 0 with empty input", d.Frames[0])
	}
	if d.Frames[1].Input != "tab" || d.Frames[2].Input != "down" {
		t.Errorf("frame inputs = %q, %q, want tab, down", d.Frames[1].Input, d.Frames[2].Input)
	}
}

func TestPlayTextFormatEventsSection(t *testing.T) {
	tui, data := writePlayFixture(t)
	code, out, errw := runCLI("play", tui, "--data", data, "--input", "tab down")
	if code != 0 {
		t.Fatalf("exit %d, want 0; stderr=%s", code, errw)
	}
	if !strings.Contains(out, "=== events ===\n2 open rows") {
		t.Errorf("want the documented text event line, got:\n%s", out)
	}
}

func TestPlayTextFormatNoEventsPrintsNone(t *testing.T) {
	tui, data := writePlayFixture(t)
	code, out, errw := runCLI("play", tui, "--data", data, "--input", "tab")
	if code != 0 {
		t.Fatalf("exit %d, want 0; stderr=%s", code, errw)
	}
	if !strings.Contains(out, "=== events ===\nnone\n") {
		t.Errorf("want \"none\" when no event fired, got:\n%s", out)
	}
}

func TestPlayExitsTwoOnErrorSeverityDump(t *testing.T) {
	tui := writeTemp(t, "bad.tui", `<tui version="1"><screen id="main"><widget/></screen></tui>`)
	code, out, errw := runCLI("play", tui)
	if code != 2 {
		t.Fatalf("exit %d, want 2; stdout=%s stderr=%s", code, out, errw)
	}
}

func TestPlayThemeOverride(t *testing.T) {
	tui, data := writePlayFixture(t)
	code, out, errw := runCLI("play", tui, "--data", data, "--theme", "light", "--styles", "--format", "json")
	if code != 0 {
		t.Fatalf("exit %d, want 0; stderr=%s", code, errw)
	}
	if !strings.Contains(out, `"theme": "light"`) {
		t.Errorf("want theme:light in the dump, got:\n%s", out)
	}
}

func TestPlayBadThemeIsUsageError(t *testing.T) {
	tui, data := writePlayFixture(t)
	for _, bad := range []string{"auto", "sepia"} {
		code, _, errw := runCLI("play", tui, "--data", data, "--theme", bad)
		if code != 1 || !strings.Contains(errw, "dark or light") {
			t.Errorf("--theme %s: exit %d, stderr %q, want exit 1 mentioning dark or light", bad, code, errw)
		}
	}
}

// finding 27 / SPEC v0.2 §15.1: "--theme takes dark or light ... Any other
// value, auto included, is a usage error." An explicitly empty value is
// still a given value (not the flag being left out), so it must fail the
// same way "auto" does; before the fix, applyTheme treated "" as "the
// flag not given" because the flag's own default is also "", so
// `--theme ""` silently kept the document's theme instead of erroring.
func TestPlayEmptyThemeIsUsageError(t *testing.T) {
	tui, data := writePlayFixture(t)
	code, out, errw := runCLI("play", tui, "--data", data, "--theme", "")
	if code != 1 {
		t.Fatalf("--theme \"\": exit %d, want 1; stdout=%s stderr=%s", code, out, errw)
	}
	if !strings.Contains(errw, "dark or light") {
		t.Errorf("--theme \"\": want the dark-or-light usage error, got:\n%s", errw)
	}
}

func TestPlayCellsFlag(t *testing.T) {
	tui, data := writePlayFixture(t)
	code, out, errw := runCLI("play", tui, "--data", data, "--cells", "--cols", "6", "--rows", "1", "--format", "json")
	if code != 0 {
		t.Fatalf("exit %d, want 0; stderr=%s", code, errw)
	}
	var d struct {
		Cells []any `json:"cells"`
	}
	mustUnmarshal(t, out, &d)
	if len(d.Cells) != 6 {
		t.Errorf("want cols*rows = 6 cells, got %d", len(d.Cells))
	}
}

func mustUnmarshal(t *testing.T, s string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(s), v); err != nil {
		t.Fatalf("%v\n%s", err, s)
	}
}

func TestPlayNoStepsIsJustStepZero(t *testing.T) {
	tui, data := writePlayFixture(t)
	code, out, errw := runCLI("play", tui, "--data", data, "--frames", "--format", "json")
	if code != 0 {
		t.Fatalf("exit %d, want 0; stderr=%s", code, errw)
	}
	var d struct {
		Frames []any `json:"frames"`
	}
	mustUnmarshal(t, out, &d)
	if len(d.Frames) != 1 {
		t.Errorf("want exactly one frame (step 0) with no --input/--script, got %d", len(d.Frames))
	}
}

func TestPlayVersionBumpedTo02a(t *testing.T) {
	code, out, errw := runCLI("version")
	if code != 0 {
		t.Fatalf("exit %d, want 0; stderr=%s", code, errw)
	}
	if !strings.Contains(out, "0.2.0-a") {
		t.Errorf("want the version string to mention 0.2.0-a, got %q", out)
	}
}

// TestParseInputStepsSplitsOnSpaceRuns is a sanity check on
// parseInputSteps's ASCII-space splitting (runs of spaces are one
// separator; leading/trailing ignored).
func TestParseInputStepsSplitsOnSpaceRuns(t *testing.T) {
	steps, err := parseInputSteps("  down   down  ")
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 2 {
		t.Fatalf("got %d steps, want 2: %+v", len(steps), steps)
	}
}
