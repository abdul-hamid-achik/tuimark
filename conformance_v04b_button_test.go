package tuimark_test

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
	"github.com/abdul-hamid-achik/tuimark/internal/catalog"
	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// SPEC v0.4b §24 conformance tests 211-214 and 216 (phase F7): a button's
// label, written as label or as its body, is a template in the current
// language (§6.4.2). Test 215 (a button in a <use> copy) closes with
// composition, in phase F2/F3. The fixture is testdata/v04b/t211/btn.tui
// with btn.json; the cases below derive the variants from its text.

const btnFixtureDir = "testdata/v04b/t211/"

func btnSource(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(btnFixtureDir + "btn.tui")
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// btnApp parses src and binds the JSON data (nil binds nothing).
func btnApp(t *testing.T, src string, data string) *tuimark.App {
	t.Helper()
	app, err := tuimark.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if data != "" {
		var v any
		if err := json.Unmarshal([]byte(data), &v); err != nil {
			t.Fatal(err)
		}
		if err := app.Bind("", v); err != nil {
			t.Fatal(err)
		}
	}
	return app
}

const btnData = `{"n": 3, "no": "Cancel"}`

func diagLines(ds []tuimark.Diagnostic) []string {
	out := []string{}
	for _, d := range ds {
		out = append(out, fmt.Sprintf("%s %s %d:%d %s id=%s %s", d.Severity, d.Code, d.Line, d.Col, d.Path, d.ID, d.Msg))
	}
	return out
}

func btnNode(t *testing.T, d *tuimark.Dump, id string) tuimark.DumpNode {
	t.Helper()
	for _, n := range d.Nodes {
		if n.ID == id {
			return n
		}
	}
	t.Fatalf("no node %q in the dump", id)
	return tuimark.DumpNode{}
}

// wantBox checks a node's geometry and text.
func wantBox(t *testing.T, d *tuimark.Dump, id string, x, y, w, h int, text string) {
	t.Helper()
	n := btnNode(t, d, id)
	if n.X != x || n.Y != y || n.W != w || n.H != h || n.Text != text {
		t.Errorf("%s: @(%d,%d) %dx%d text %q, want @(%d,%d) %dx%d text %q", id, n.X, n.Y, n.W, n.H, n.Text, x, y, w, h, text)
	}
}

func wantGrid0(t *testing.T, d *tuimark.Dump, row0 string) {
	t.Helper()
	if d.Grid[0] != row0 {
		t.Errorf("grid[0] = %q, want %q", d.Grid[0], row0)
	}
}

// 211. A button's label from the data, written as label and as the body;
// a written label wins and the body is never resolved.
func TestButtonLabelFromData(t *testing.T) {
	app := btnApp(t, btnSource(t), btnData)
	if d := app.Validate(); len(d) != 0 {
		t.Fatalf("Validate: %v", diagLines(d))
	}
	d, err := app.Dump(30, 2)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"[ Delete 3 tasks ][ Cancel ]  ", "                              "}; !reflect.DeepEqual(d.Grid, want) {
		t.Errorf("grid %q, want %q", d.Grid, want)
	}
	wantBox(t, d, "del", 0, 0, 18, 1, "Delete 3 tasks")
	wantBox(t, d, "cancel", 18, 0, 10, 1, "Cancel")
	if !btnNode(t, d, "del").Focused {
		t.Error("del is not focused")
	}

	// Both written: the label is the text, and the body is not resolved
	// (no data for it, no B003).
	both := strings.Replace(btnSource(t), `<button id="cancel" on:click="cancel">`, `<button id="cancel" label="Keep" on:click="cancel">`, 1)
	app = btnApp(t, both, `{"n": 3}`)
	if d := app.Validate(); len(d) != 0 {
		t.Errorf("both written: Validate: %v", diagLines(d))
	}
	dd, err := app.Dump(30, 2)
	if err != nil {
		t.Fatal(err)
	}
	wantBox(t, dd, "cancel", 18, 0, 8, 1, "Keep")
}

// 212. The label is resolved again every frame, an auto button follows
// its data and the nodes after it move; a written width stays; nothing
// fires.
func TestButtonLabelFollowsData(t *testing.T) {
	steps := []string{`set:n=12`, `set:no="Back"`}
	opts := tuimark.PlayOptions{Cols: 30, Rows: 2, NoHandlers: true, Frames: true}
	res, err := btnApp(t, btnSource(t), btnData).Play(opts, steps...)
	if err != nil {
		t.Fatal(err)
	}
	f1, f2 := res.Frames[1].Dump, res.Frames[2].Dump
	wantBox(t, f1, "del", 0, 0, 19, 1, "Delete 12 tasks")
	wantBox(t, f1, "cancel", 19, 0, 10, 1, "Cancel")
	wantGrid0(t, f1, "[ Delete 12 tasks ][ Cancel ] ")
	wantBox(t, f2, "cancel", 19, 0, 8, 1, "Back")
	wantGrid0(t, f2, "[ Delete 12 tasks ][ Back ]   ")
	if len(res.Events) != 0 {
		t.Errorf("events %v, want none", res.Events)
	}

	wide := strings.Replace(btnSource(t), `<button id="del" `, `<button id="del" style="width: 20" `, 1)
	res, err = btnApp(t, wide, btnData).Play(opts, steps...)
	if err != nil {
		t.Fatal(err)
	}
	for i, f := range res.Frames[1:] {
		wantBox(t, f.Dump, "del", 0, 0, 20, 1, "Delete 12 tasks")
		if x := btnNode(t, f.Dump, "cancel").X; x != 20 {
			t.Errorf("frame %d: cancel at x %d, want 20", i+1, x)
		}
	}
}

// 213. Missing paths are B003 at the button and resolve to nothing; LFs a
// value brings become spaces; no data, nothing to report.
func TestButtonLabelMissingPathsAndLineFeeds(t *testing.T) {
	src := btnSource(t)
	app := btnApp(t, src, `{"no": "Can\ncel"}`)
	want := []string{`warning B003 4:7 /screen/row/button id=del bind path "n" is missing`}
	if got := diagLines(app.Validate()); !reflect.DeepEqual(got, want) {
		t.Errorf("Validate: %q, want %q", got, want)
	}
	d, err := app.Dump(30, 2)
	if err != nil {
		t.Fatal(err)
	}
	wantBox(t, d, "del", 0, 0, 17, 1, "Delete  tasks")
	wantBox(t, d, "cancel", 17, 0, 11, 1, "Can cel")
	wantGrid0(t, d, "[ Delete  tasks ][ Can cel ]  ")

	app = btnApp(t, src, `{"n": 3}`)
	want = []string{`warning B003 5:7 /screen/row/button id=cancel bind path "no" is missing`}
	if got := diagLines(app.Validate()); !reflect.DeepEqual(got, want) {
		t.Errorf("Validate: %q, want %q", got, want)
	}
	d, err = app.Dump(30, 2)
	if err != nil {
		t.Fatal(err)
	}
	wantBox(t, d, "cancel", 18, 0, 4, 1, "")
	wantGrid0(t, d, "[ Delete 3 tasks ][  ]        ")

}

// 214. A pinned document keeps its 0.4.1 meaning: {path} is V003 with the
// hint, once per attribute or body, no B003, and the label is painted as
// written and never moves.
func TestButtonLabelPinnedDocuments(t *testing.T) {
	for _, v := range []string{"1", "2", "3"} {
		t.Run("version="+v, func(t *testing.T) {
			src := strings.Replace(btnSource(t), "<tui>", `<tui version="`+v+`">`, 1)
			app := btnApp(t, src, btnData)
			const msg = "{path} in a <button> label (requires a <tui> without version)"
			want := []string{
				"error V003 4:24 /screen/row/button id=del " + msg,
				"error V003 5:7 /screen/row/button id=cancel " + msg,
			}
			if got := diagLines(app.Validate()); !reflect.DeepEqual(got, want) {
				t.Errorf("Validate:\n%q\nwant\n%q", got, want)
			}
			d, err := app.Dump(30, 2)
			if err != nil {
				t.Fatal(err)
			}
			wantBox(t, d, "del", 0, 0, 20, 1, "Delete {n} tasks")
			wantBox(t, d, "cancel", 20, 0, 8, 1, "{no}")
			wantGrid0(t, d, "[ Delete {n} tasks ][ {no} ]  ")
			res, err := app.Play(tuimark.PlayOptions{Cols: 30, Rows: 2, NoHandlers: true}, "set:n=12")
			if err != nil {
				t.Fatal(err)
			}
			wantGrid0(t, res.Dump, "[ Delete {n} tasks ][ {no} ]  ")
		})
	}
}

// 216. A button's events do not carry the label, the same document with
// literal labels fires the same events, the IR keeps the label and the
// body as written, and the catalog does not change.
func TestButtonLabelEventsAndTools(t *testing.T) {
	src := btnSource(t)
	opts := tuimark.PlayOptions{Cols: 30, Rows: 2, NoHandlers: true}
	events := func(t *testing.T, src string, steps ...string) string {
		t.Helper()
		res, err := btnApp(t, src, btnData).Play(opts, steps...)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(res.Events)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	const enterTabSpace = `[{"step":1,"action":"delete","source":"del","keys":{},"value":null},{"step":3,"action":"cancel","source":"cancel","keys":{},"value":null}]`
	if got := events(t, src, "enter", "tab", "space"); got != enterTabSpace {
		t.Errorf("enter tab space: %s, want %s", got, enterTabSpace)
	}
	literal := strings.NewReplacer("Delete {n} tasks", "Delete 3 tasks", "{no}", "Cancel").Replace(src)
	if got := events(t, literal, "enter", "tab", "space"); got != enterTabSpace {
		t.Errorf("literal labels: %s, want %s", got, enterTabSpace)
	}

	mouse := strings.Replace(src, "<tui>", `<tui mouse="true">`, 1)
	const click = `[{"step":1,"action":"delete","source":"del","keys":{},"value":null}]`
	if got := events(t, mouse, "click:2,0"); got != click {
		t.Errorf("click:2,0: %s, want %s", got, click)
	}
	keymap := strings.Replace(src, "</tui>", `  <keymap><bind keys="x" action="go"/></keymap>`+"\n</tui>", 1)
	const goEv = `[{"step":1,"action":"go","source":"del","keys":{},"value":null}]`
	if got := events(t, keymap, "x"); got != goEv {
		t.Errorf("x: %s, want %s", got, goEv)
	}

	t.Run("ir", func(t *testing.T) {
		doc := parse.Parse([]byte(src), "btn.tui")
		tree := parse.BuildIR(doc, nil)
		if tree.Version != "0.4" {
			t.Errorf("IR version %q, want 0.4", tree.Version)
		}
		var find func(n *parse.IRNode, id string) *parse.IRNode
		find = func(n *parse.IRNode, id string) *parse.IRNode {
			if n.ID == id {
				return n
			}
			for _, c := range n.Children {
				if f := find(c, id); f != nil {
					return f
				}
			}
			return nil
		}
		if n := find(tree.Root, "del"); n == nil || n.Attrs["label"] != "Delete {n} tasks" {
			t.Errorf("del node: %+v", n)
		}
		if n := find(tree.Root, "cancel"); n == nil || n.Text != "{no}" {
			t.Errorf("cancel node: %+v", n)
		}
	})
	t.Run("catalog", func(t *testing.T) {
		for _, tag := range catalog.Build("x").Tags {
			if tag.Name != "button" {
				continue
			}
			for _, a := range tag.Attrs {
				if a.Name == "label" {
					if a.Since != 1 || a.Keywords == nil || len(a.Keywords) != 0 {
						t.Errorf("button label: since %d keywords %#v, want 1 and []", a.Since, a.Keywords)
					}
					return
				}
			}
		}
		t.Error("button label not in the catalog")
	})
}

// A {…} pair that does not hold exactly a path is the template V003 of §7,
// in the label and in the body of the current language; pinned documents
// report the hint message instead. Control characters stay V007.
func TestButtonLabelMalformedTemplate(t *testing.T) {
	const doc = `<tui>
  <screen id="main">
    <button id="a" label="{a + b}"/>
    <button id="b">{ c }</button>
  </screen>
</tui>`
	got := diagLines(btnApp(t, doc, "").Validate())
	want := []string{
		"error V003 3:20 /screen/button id=a label: {a + b} is not a path; markup has no expressions",
		"error V003 4:20 /screen/button id=b { c } is not a path: write {c}, with no spaces inside the braces",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("current language:\n%q\nwant\n%q", got, want)
	}
	pinned := strings.Replace(doc, "<tui>", `<tui version="3">`, 1)
	for _, l := range diagLines(btnApp(t, pinned, "").Validate()) {
		if !strings.HasSuffix(l, "{path} in a <button> label (requires a <tui> without version)") {
			t.Errorf("pinned: %q", l)
		}
	}
}
