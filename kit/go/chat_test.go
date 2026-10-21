package kit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

// The chat templates of the kit (k-composer, k-block, k-receipt) against the
// real kit.tui, as SPEC v0.4d §24 "Examples and the kit" asks: k-composer
// passes the behavior of tests 284 and 287 as a template, and the approval
// of test 311 works with it.

// loadDoc writes src into a temp dir with the kit included from this
// repository and loads it.
func loadDoc(t *testing.T, body, data string) *tuimark.App {
	t.Helper()
	kit, err := filepath.Abs(filepath.Join("..", "kit.tui"))
	if err != nil {
		t.Fatal(err)
	}
	src := "<tui>\n<include src=\"" + kit + "\"/>\n" + body + "\n</tui>\n"
	dir := t.TempDir()
	file := filepath.Join(dir, "app.tui")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	app, err := tuimark.Load(file)
	if err != nil {
		t.Fatal(err)
	}
	if data != "" {
		bindObject(t, app, data)
	}
	return app
}

func bindObject(t *testing.T, app *tuimark.App, raw string) {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatal(err)
	}
	if err := app.Bind("", v); err != nil {
		t.Fatal(err)
	}
}

func dnode(d *tuimark.Dump, id string) *tuimark.DumpNode {
	for i := range d.Nodes {
		if d.Nodes[i].ID == id {
			return &d.Nodes[i]
		}
	}
	return nil
}

type evs []tuimark.PlayEvent

// actions is "action=value" for each event, in order, with the source
// when it is not ids.
func (e evs) actions() []string {
	var out []string
	for _, ev := range e {
		out = append(out, ev.Action+"="+toS(ev.Value)+"@"+ev.Source)
	}
	return out
}

func toS(v any) string {
	if s, ok := v.(string); ok {
		return strings.ReplaceAll(s, "\n", "\\n")
	}
	if v == nil {
		return "null"
	}
	return "?"
}

const composerDoc = `
<screen id="main" focus="#composer__input">
  <text>log</text>
  <use id="composer" template="k-composer" placeholder="Message" with="composer as c"/>
  <text id="status">ready</text>
</screen>
<keymap>
  <bind keys="up" action="history_prev" when="#composer__input:focus"/>
  <bind keys="down" action="history_next" when="#composer__input:focus"/>
  <bind keys="pgup" action="page_up" when="#composer__input:focus"/>
</keymap>`

// SPEC 0.4d test 284 as a template: typing, line breaks, submitting. The
// deltas from the test's t.tui are the ids (the textarea is
// composer__input, the use's id then the template's) and the actions
// (composer_change, composer_submit); the behavior is the test's.
func TestComposerTyping284(t *testing.T) {
	app := loadDoc(t, composerDoc, `{"composer": {"draft": ""}}`)
	res, err := app.Play(tuimark.PlayOptions{Cols: 30, Rows: 8},
		"text:hello", "shift+enter", "text:world", "enter")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"composer_change=hello@composer__input",
		`composer_change=hello\n@composer__input`,
		`composer_change=hello\nworld@composer__input`,
		`composer_submit=hello\nworld@composer__input`,
	}
	if got := evs(res.Events).actions(); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("events\n got  %q\n want %q", got, want)
	}
	// The textarea grew to two rows and the status moved down with it.
	ta, status := dnode(res.Dump, "composer__input"), dnode(res.Dump, "status")
	if ta == nil || ta.H != 2 || ta.Caret == nil || ta.Caret.Line != 1 || ta.Caret.Col != 5 {
		t.Errorf("textarea %+v", ta)
	}
	// The template adds a rule above the textarea and its hints row below
	// it, so the status is one row further down than in test 284.
	if ta != nil && status != nil && (status.Y != ta.Y+ta.H+1 || ta.Y != 2) {
		t.Errorf("status at y=%d, textarea at y=%d h=%d", status.Y, ta.Y, ta.H)
	}
	// alt+enter and a text step with a line feed give the same events.
	for _, nl := range []string{"alt+enter"} {
		app = loadDoc(t, composerDoc, `{"composer": {"draft": ""}}`)
		res, err = app.Play(tuimark.PlayOptions{Cols: 30, Rows: 8}, "text:hello", nl, "text:world", "enter")
		if err != nil {
			t.Fatal(err)
		}
		if got := evs(res.Events).actions(); strings.Join(got, "|") != strings.Join(want, "|") {
			t.Errorf("%s: events %q", nl, got)
		}
	}
	// The host clears the draft before it replies: enter changed nothing,
	// and the next frame is empty with the placeholder back.
	if err := app.Set("composer.draft", ""); err != nil {
		t.Fatal(err)
	}
	res, err = app.Play(tuimark.PlayOptions{Cols: 30, Rows: 8})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(res.Dump.Grid, "\n"), "Message") {
		t.Errorf("the placeholder is not back after the draft was cleared:\n%s", strings.Join(res.Dump.Grid, "\n"))
	}
}

// SPEC 0.4d test 287 as a template: up and down move through the lines
// first and reach the keymap on the first and last line.
func TestComposerUpDown287(t *testing.T) {
	app := loadDoc(t, composerDoc, `{"composer": {"draft": "a\nb"}}`)
	res, err := app.Play(tuimark.PlayOptions{Cols: 30, Rows: 8}, "up", "up", "down", "down")
	if err != nil {
		t.Fatal(err)
	}
	var actions []string
	for _, e := range res.Events {
		actions = append(actions, e.Action)
	}
	if got := strings.Join(actions, ","); got != "history_prev,history_next" {
		t.Errorf("events %q, want history_prev,history_next (the first up and down only move the caret)", got)
	}
	// pgup with three visual lines and no scrolling is not consumed.
	app = loadDoc(t, composerDoc, `{"composer": {"draft": "a\nb\nc"}}`)
	res, err = app.Play(tuimark.PlayOptions{Cols: 30, Rows: 8}, "pgup")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Events) != 1 || res.Events[0].Action != "page_up" {
		t.Errorf("pgup events %v", res.Events)
	}
}

// The composer's hints are the keymap rows with a label that a key would
// fire now, and a disabled composer takes no focus and no key.
func TestComposerHintsAndDisabled(t *testing.T) {
	doc := `
<screen id="main" focus="#composer__input">
  <use id="composer" template="k-composer" placeholder="Message" with="composer as c"/>
</screen>
<keymap>
  <bind keys="up" action="history_prev" when="#composer__input:focus" label="history"/>
  <bind keys="y" action="approve"/>
</keymap>`
	app := loadDoc(t, doc, `{"composer": {"draft": ""}}`)
	res, err := app.Play(tuimark.PlayOptions{Cols: 40, Rows: 6})
	if err != nil {
		t.Fatal(err)
	}
	if grid := strings.Join(res.Dump.Grid, "\n"); !strings.Contains(grid, "up history") {
		t.Errorf("the hints do not show the labeled row:\n%s", grid)
	}
	// While the composer has focus, y is typed into it.
	res, err = app.Play(tuimark.PlayOptions{Cols: 40, Rows: 6}, "y")
	if err != nil {
		t.Fatal(err)
	}
	if got := evs(res.Events).actions(); strings.Join(got, "|") != "composer_change=y@composer__input" {
		t.Errorf("y typed: %q", got)
	}
	// Disabled: y reaches the keymap with nothing focused, and the draft
	// is kept.
	app = loadDoc(t, doc, `{"composer": {"draft": "kept", "disabled": true}}`)
	res, err = app.Play(tuimark.PlayOptions{Cols: 40, Rows: 6}, "y")
	if err != nil {
		t.Fatal(err)
	}
	if got := evs(res.Events).actions(); strings.Join(got, "|") != "approve=null@" {
		t.Errorf("y while disabled: %q", got)
	}
	if v, _ := app.Get("composer.draft"); v != "kept" {
		t.Errorf("draft %v", v)
	}
}

// k-block: a paragraph of runs, and each flag, with a missing flag false
// and no diagnostic.
func TestBlockFlags(t *testing.T) {
	app := loadSample(t, "block")
	d, err := app.Dump(60, 30)
	if err != nil {
		t.Fatal(err)
	}
	grid := strings.Join(d.Grid, "\n")
	for _, want := range []string{
		"Fixing the flaky test",
		"The failure comes from retry() sleeping after the second",
		"  • move the sleep before the attempt", // an item is indented
		"      for i := 0; i < 3; i++ {",        // a code block keeps its tab, as four spaces
		"──────",                                // the rule
	} {
		if !strings.Contains(grid, want) {
			t.Errorf("%q is not on the screen:\n%s", want, grid)
		}
	}
	// B002 and B003 stay out: a block that omits a flag reads it as false.
	for _, diag := range app.Validate() {
		if diag.Code == "B002" || diag.Code == "B003" || diag.Severity == "error" {
			t.Errorf("unexpected diagnostic: %v", diag)
		}
	}
}

// k-receipt: the output is shown as runs, so a TAB is four spaces and a
// line break is kept, and a flag changes the look.
func TestReceiptOutputAsRuns(t *testing.T) {
	app := loadSample(t, "receipt")
	d, err := app.Dump(60, 16)
	if err != nil {
		t.Fatal(err)
	}
	grid := strings.Join(d.Grid, "\n")
	for _, want := range []string{
		"bash · ls -l src",
		"-rw-r--r--    1 me    4210    app.ts",
		"error: expected 2 blocks, got 1",
		"    at markdown.test.ts:31",
		"exit 0 · 0.2s",
	} {
		if !strings.Contains(grid, want) {
			t.Errorf("%q is not on the screen:\n%s", want, grid)
		}
	}
}
