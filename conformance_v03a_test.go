package tuimark_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

// SPEC v0.3 §21 conformance tests of 0.3a's clipping warnings: version="3"
// acceptance and IR (test 84), L008 (test 85), L009 (test 86), the dump
// schema (test 87, covered in cmd/tuimark/schema_conformance_test.go), and
// compatibility (test 88, covered by `tuimark test` in task verify).

// parseV3 parses src (a version="3" document) and fails the test on any
// diagnostic whose severity is "error" outside the cases the caller
// already expects (checked separately by the caller).
func parseV3(t *testing.T, src string) *tuimark.App {
	t.Helper()
	app, err := tuimark.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	return app
}

func bindJSON(t *testing.T, app *tuimark.App, path, raw string) {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatal(err)
	}
	if err := app.Bind(path, v); err != nil {
		t.Fatal(err)
	}
}

func dumpNode(d *tuimark.Dump, id string) *tuimark.DumpNode {
	for i := range d.Nodes {
		if d.Nodes[i].ID == id {
			return &d.Nodes[i]
		}
	}
	return nil
}

func diagsWithCode(d *tuimark.Dump, code string) []tuimark.Diagnostic {
	var out []tuimark.Diagnostic
	for _, e := range d.Errors {
		if e.Code == code {
			out = append(out, e)
		}
	}
	return out
}

func requireNoParseErrors(t *testing.T, app *tuimark.App) {
	t.Helper()
	for _, d := range app.Validate() {
		if d.Severity == "error" {
			t.Fatalf("unexpected error diagnostic: %s", d)
		}
	}
}

// 84. version="3" is accepted with no diagnostic. version="4" being V003
// with a message listing "1", "2", or "3" is covered in
// conformance_v02b_test.go with the rest of the version gate, and IR
// "0.3" by cmd/tuimark TestVersionThreeFixturesIR.
func TestVersionThreeIsAccepted(t *testing.T) {
	app := parseV3(t, `<tui version="3"><screen id="main"><text>x</text></screen></tui>`)
	requireNoParseErrors(t, app)
}

// loadVersionCopy copies path's directory's .tui and .tcss files into a
// fresh t.TempDir() (so a relative <style src> keeps resolving, and
// nothing leaks into the repo tree, which other packages' tests glob),
// with path's own copy's <tui version="from" changed to version="to",
// and returns that copy's path.
func loadVersionCopy(t *testing.T, path, from, to string) string {
	t.Helper()
	dir := filepath.Dir(path)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	replaced := false
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := filepath.Ext(e.Name())
		if ext != ".tui" && ext != ".tcss" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if e.Name() == filepath.Base(path) {
			swapped := bytes.Replace(raw, []byte(`<tui version="`+from+`"`), []byte(`<tui version="`+to+`"`), 1)
			if bytes.Equal(swapped, raw) {
				t.Fatalf(`%s: no <tui version="%s" to replace`, path, from)
			}
			raw, replaced = swapped, true
		}
		if err := os.WriteFile(filepath.Join(tmp, e.Name()), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if !replaced {
		t.Fatalf("%s: not found while copying %s", path, dir)
	}
	return filepath.Join(tmp, filepath.Base(path))
}

func loadWithData(t *testing.T, tuiPath, dataPath string) *tuimark.App {
	t.Helper()
	app, err := tuimark.Load(tuiPath)
	if err != nil {
		t.Fatal(err)
	}
	if dataPath != "" {
		if _, err := os.Stat(dataPath); err == nil {
			raw, err := os.ReadFile(dataPath)
			if err != nil {
				t.Fatal(err)
			}
			var data any
			if err := json.Unmarshal(raw, &data); err != nil {
				t.Fatal(err)
			}
			if err := app.Bind("", data); err != nil {
				t.Fatal(err)
			}
		}
	}
	return app
}

// 84. specs/fixtures/{grid,hints,table,tabs}.tui and
// examples/monitor/studio.tui, with only their version changed between
// "2" and "3", give byte-identical dumps and diagnostics in both forms at
// every size where neither L008 nor L009 fires. The fixtures are
// version="2" and get a version="3" copy; examples/monitor is
// version="3" since 0.3a (test 89) and gets a version="2" copy.
func TestVersionThreeMatchesVersionTwoWithoutClipping(t *testing.T) {
	fixtures := []struct{ tui, data string }{
		{"specs/fixtures/grid.tui", "specs/fixtures/grid.json"},
		{"specs/fixtures/hints.tui", "specs/fixtures/hints.json"},
		{"specs/fixtures/table.tui", "specs/fixtures/table.json"},
		{"specs/fixtures/tabs.tui", "specs/fixtures/tabs.json"},
		{"examples/monitor/studio.tui", "examples/monitor/sample.json"},
	}
	for _, fx := range fixtures {
		t.Run(fx.tui, func(t *testing.T) {
			raw, err := os.ReadFile(fx.tui)
			if err != nil {
				t.Fatal(err)
			}
			v2path, v3path := fx.tui, ""
			if bytes.Contains(raw, []byte(`<tui version="3"`)) {
				v2path, v3path = loadVersionCopy(t, fx.tui, "3", "2"), fx.tui
			} else {
				v3path = loadVersionCopy(t, fx.tui, "2", "3")
			}
			v2app := loadWithData(t, v2path, fx.data)
			v3app := loadWithData(t, v3path, fx.data)
			for _, cols := range []int{40, 80, 120} {
				d2, err := v2app.Dump(cols, 24)
				if err != nil {
					t.Fatal(err)
				}
				d3, err := v3app.Dump(cols, 24)
				if err != nil {
					t.Fatal(err)
				}
				clipped := false
				for _, e := range d3.Errors {
					if e.Code == "L008" || e.Code == "L009" {
						clipped = true
					}
				}
				if clipped {
					continue // this size legitimately differs; not asserted here
				}
				b2, err := json.Marshal(d2)
				if err != nil {
					t.Fatal(err)
				}
				b3, err := json.Marshal(d3)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(b2, b3) {
					t.Errorf("%s at %dx24: v3 dump differs from v2 with no L008/L009 reported:\nv2=%s\nv3=%s", fx.tui, cols, b2, b3)
				}
			}
		})
	}
}

// 85. L008 vectors: the two reporting cases (a width:10 text with a
// 14-column line; a wrap:wrap text with height:2 whose body wraps to 3
// lines), the six non-reporting cases, and one L008/clipped-per-row for
// the rows of one each template.
func TestL008Vectors(t *testing.T) {
	const src = `<tui version="3">
<keymap><bind keys="q" action="quit" label="quit"/></keymap>
<screen id="main">
<text id="wide" width="10">ABCDEFGHIJKLMN</text>
<text id="wraps" wrap="wrap" width="6" height="2">alpha beta gamma</text>
<text id="nowrapDoc" width="5" wrap="nowrap">ABCDEFG</text>
<box style="wrap: nowrap"><text id="nowrapAncestor" width="5">ABCDEFG</text></box>
<text id="truncated" width="5" wrap="truncate">ABCDEFG</text>
<text id="hiddenText" width="5" style="visibility: hidden">ABCDEFG</text>
<text id="overflowHidden" wrap="wrap" width="6" height="2" style="overflow: hidden">alpha beta gamma</text>
<list id="rows" width="10" height="3" each="items as item" key="item.k"><item><text width="10">{item.v}</text></item></list>
<table id="tbl" width="10" height="3" each="rows2 as r" key="r.k"><column id="c1" width="3">{r.v}</column></table>
<tabs id="tb" width="4" height="3"><tab id="ta" label="Averylonglabelindeed"><text>x</text></tab></tabs>
<hints id="hh" width="3" height="1"/>
</screen>
</tui>`
	app := parseV3(t, src)
	bindJSON(t, app, "items", `[{"k":0,"v":"ABCDEFGHIJKLMN"},{"k":1,"v":"ABCDEFGHIJKLMN"},{"k":2,"v":"ABCDEFGHIJKLMN"}]`)
	bindJSON(t, app, "rows2", `[{"k":0,"v":"ABCDEFGHIJKLMNOP"}]`)
	requireNoParseErrors(t, app)

	d, err := app.Dump(80, 100)
	if err != nil {
		t.Fatal(err)
	}

	wantClipped := []string{"wide", "wraps"}
	for _, id := range wantClipped {
		n := dumpNode(d, id)
		if n == nil {
			t.Fatalf("no node %q in dump", id)
		}
		if n.Clipped != "text" {
			t.Errorf("%s: clipped = %q, want %q", id, n.Clipped, "text")
		}
	}
	noClip := []string{"nowrapDoc", "nowrapAncestor", "truncated", "hiddenText", "overflowHidden"}
	for _, id := range noClip {
		n := dumpNode(d, id)
		if n == nil {
			t.Fatalf("no node %q in dump", id)
		}
		if n.Clipped != "" {
			t.Errorf("%s: unexpectedly clipped %q", id, n.Clipped)
		}
	}
	// The rows of the list's <item> template all cut their text: one
	// L008 diagnostic (deduped on the template's source position), and
	// clipped "text" on every row's text node.
	rowClips := 0
	for _, n := range d.Nodes {
		if n.Tag == "text" && n.ID == "" && n.Clipped == "text" {
			rowClips++
		}
	}
	if rowClips != 3 {
		t.Errorf("clipped list rows = %d, want 3", rowClips)
	}
	l008 := diagsWithCode(d, "L008")
	if len(l008) != 3 { // wide, wraps, and the one list-template diagnostic
		t.Errorf("L008 diagnostics = %d, want 3: %v", len(l008), l008)
	}
	// A version="2" document never reports L008 or carries clipped.
	v2src := strings.Replace(src, `version="3"`, `version="2"`, 1)
	app2 := parseV3(t, v2src)
	bindJSON(t, app2, "items", `[{"k":0,"v":"ABCDEFGHIJKLMN"},{"k":1,"v":"ABCDEFGHIJKLMN"},{"k":2,"v":"ABCDEFGHIJKLMN"}]`)
	bindJSON(t, app2, "rows2", `[{"k":0,"v":"ABCDEFGHIJKLMNOP"}]`)
	d2, err := app2.Dump(80, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagsWithCode(d2, "L008")) != 0 {
		t.Errorf("version=\"2\": unexpected L008: %v", diagsWithCode(d2, "L008"))
	}
	for _, n := range d2.Nodes {
		if n.Clipped != "" {
			t.Errorf("version=\"2\": node %q unexpectedly clipped %q", n.ID, n.Clipped)
		}
	}
}

// panelsGridSrc is the §30.2/§21 test-86 grid vector: four 4-row panels in
// a layout: grid #panels of height 7. At 80 columns grid-min-width: 28
// fits 2 columns (2 rows of panels, 8 rows of content into 7): the first
// panel of the second row is cut. At 120 columns it fits 4 columns (one
// row, 4 into 7): nothing is cut.
func panelsGridSrc(version, extra string) string {
	return `<tui version="` + version + `"><screen id="main">
<box id="panels" style="layout: grid; grid-columns: 4; grid-min-width: 28; height: 7` + extra + `">
<box height="4"><text>a</text></box>
<box height="4"><text>b</text></box>
<box height="4"><text>c</text></box>
<box height="4"><text>d</text></box>
</box>
</screen></tui>`
}

// 86. The #panels grid vector at 80 vs 120 columns, the message naming
// the first panel of the second row, y, and "3 of 4 cells shown".
func TestL009PanelsGrid(t *testing.T) {
	app := parseV3(t, panelsGridSrc("3", ""))
	requireNoParseErrors(t, app)

	d80, err := app.Dump(80, 24)
	if err != nil {
		t.Fatal(err)
	}
	l009 := diagsWithCode(d80, "L009")
	if len(l009) != 1 {
		t.Fatalf("80 cols: L009 diagnostics = %d, want 1: %v", len(l009), l009)
	}
	if l009[0].ID != "panels" {
		t.Errorf("80 cols: L009 id = %q, want %q", l009[0].ID, "panels")
	}
	const want = "cuts box[2] on y: 3 of 4 cells shown"
	if !strings.Contains(l009[0].Msg, want) {
		t.Errorf("80 cols: L009 msg = %q, want it to contain %q", l009[0].Msg, want)
	}
	if n := dumpNode(d80, "panels"); n == nil || n.Clipped != "children" {
		t.Errorf("80 cols: #panels clipped = %+v, want \"children\"", n)
	}

	d120, err := app.Dump(120, 24)
	if err != nil {
		t.Fatal(err)
	}
	if l009 := diagsWithCode(d120, "L009"); len(l009) != 0 {
		t.Errorf("120 cols: unexpected L009: %v", l009)
	}
	if n := dumpNode(d120, "panels"); n != nil && n.Clipped != "" {
		t.Errorf("120 cols: #panels unexpectedly clipped %q", n.Clipped)
	}

	// overflow: hidden written on #panels silences it.
	appHidden := parseV3(t, panelsGridSrc("3", "; overflow: hidden"))
	dHidden, err := appHidden.Dump(80, 24)
	if err != nil {
		t.Fatal(err)
	}
	if l009 := diagsWithCode(dHidden, "L009"); len(l009) != 0 {
		t.Errorf("overflow: hidden: unexpected L009: %v", l009)
	}

	// A version="2" document never reports L009.
	appV2 := parseV3(t, panelsGridSrc("2", ""))
	dV2, err := appV2.Dump(80, 24)
	if err != nil {
		t.Fatal(err)
	}
	if l009 := diagsWithCode(dV2, "L009"); len(l009) != 0 {
		t.Errorf(`version="2": unexpected L009: %v`, l009)
	}
	if n := dumpNode(dV2, "panels"); n != nil && n.Clipped != "" {
		t.Errorf(`version="2": #panels unexpectedly clipped %q`, n.Clipped)
	}
}

// 86. The rest of the L009 vectors: a 3-row box holding a 4-line text
// (L009 on the box, no L008 on the text); list/table/hints never, even
// with a genuine geometry mismatch; a child a scrolling ancestor
// legitimately clips is not double-reported; a cut child L006 already
// reports, and a node L003 already reports, are not reported again;
// visibility: hidden children are ignored.
func TestL009RestOfVectors(t *testing.T) {
	const src = `<tui version="3">
<keymap><bind keys="q" action="quit" label="quit"/></keymap>
<screen id="main">
<box id="b3" height="3"><text id="t8">
a
b
c
d
</text></box>
<list id="lst2" width="10" height="3" each="wideitems as w" key="w.k"><item width="20"><text>{w.v}</text></item></list>
<table id="tbl2" width="10" height="3" each="rows3 as r" key="r.k"><column id="cc" width="3">{r.v}</column></table>
<hints id="hh2" width="20" height="0"/>
<box id="outerL006" width="5"><scroll id="innerScroll" axis="x" width="20"><text>abcdefghijklmnopqrst</text></scroll></box>
<box id="outerL003" height="5">
<box height="3"><text>x</text></box>
<box height="4"><text>y</text></box>
</box>
<box id="outerHidden" width="3"><box width="10" style="visibility: hidden"/></box>
</screen>
</tui>`
	app := parseV3(t, src)
	bindJSON(t, app, "wideitems", `[{"k":0,"v":"x"}]`)
	bindJSON(t, app, "rows3", `[{"k":0,"v":"ABCDEFGHIJ"}]`)
	requireNoParseErrors(t, app)

	d, err := app.Dump(80, 200)
	if err != nil {
		t.Fatal(err)
	}

	byID := func(id string) []tuimark.Diagnostic {
		var out []tuimark.Diagnostic
		for _, e := range d.Errors {
			if e.ID == id {
				out = append(out, e)
			}
		}
		return out
	}
	codesOf := func(ds []tuimark.Diagnostic) []string {
		var out []string
		for _, d := range ds {
			out = append(out, d.Code)
		}
		return out
	}

	// b3/t8: L009 on the box, no L008 on the text.
	if n := dumpNode(d, "b3"); n == nil || n.Clipped != "children" {
		t.Errorf("#b3 clipped = %+v, want \"children\"", n)
	}
	if n := dumpNode(d, "t8"); n == nil || n.Clipped != "" {
		t.Errorf("#t8 clipped = %+v, want \"\"", n)
	}
	if l009 := byID("b3"); len(l009) == 0 || l009[0].Code != "L009" {
		t.Errorf("#b3: want an L009, got %v", codesOf(l009))
	}

	// list/table/hints never report L009 as the cutting node, even where
	// their content genuinely does not fit.
	for _, id := range []string{"lst2", "tbl2", "hh2"} {
		if l009 := byID(id); len(l009) != 0 {
			t.Errorf("#%s: unexpected L009: %v", id, l009)
		}
		if n := dumpNode(d, id); n != nil && n.Clipped != "" {
			t.Errorf("#%s: unexpectedly clipped %q", id, n.Clipped)
		}
	}

	// outerL006: the scroll's own L006 explains the cut; the box gets no
	// L009 for that child.
	if l006 := byID("innerScroll"); len(l006) == 0 || l006[0].Code != "L006" {
		t.Errorf("#innerScroll: want an L006, got %v", codesOf(l006))
	}
	if l009 := byID("outerL006"); len(l009) != 0 {
		t.Errorf("#outerL006: unexpected L009: %v", l009)
	}
	if n := dumpNode(d, "outerL006"); n != nil && n.Clipped != "" {
		t.Errorf("#outerL006: unexpectedly clipped %q", n.Clipped)
	}

	// outerL003: L003 already explains the cut; no L009 for the same box.
	all := byID("outerL003")
	if len(all) == 0 {
		t.Fatalf("#outerL003: no diagnostics, want L003")
	}
	var hasL003 bool
	for _, e := range all {
		switch e.Code {
		case "L003":
			hasL003 = true
		case "L009":
			t.Errorf("#outerL003: unexpected L009: %s", e)
		}
	}
	if !hasL003 {
		t.Errorf("#outerL003: want an L003, got %v", codesOf(all))
	}
	if n := dumpNode(d, "outerL003"); n != nil && n.Clipped != "" {
		t.Errorf("#outerL003: unexpectedly clipped %q", n.Clipped)
	}

	// outerHidden: its only child is visibility: hidden, so it does not
	// count even though it sticks out.
	if l009 := byID("outerHidden"); len(l009) != 0 {
		t.Errorf("#outerHidden: unexpected L009: %v", l009)
	}
}

// Review fix (SPEC v0.3 §14, §30.4 item 13c): the rows of one template
// that cut by different amounts still give one L008 and one L009, while
// every row that is cut gets clipped. The dedup once keyed on the
// message, which differs from row to row.
func TestClipOneDiagnosticPerTemplate(t *testing.T) {
	const src = `<tui version="3">
<screen id="main">
<list id="rows" width="10" height="4" each="items as item" key="item.k"><item><text width="10">{item.v}</text></item></list>
<col each="items as it" key="it.k"><box width="10" height="1"><text style="wrap: wrap">{it.v}</text></box></col>
</screen>
</tui>`
	app := parseV3(t, src)
	bindJSON(t, app, "items", `[{"k":0,"v":"ABCDEFGHIJKLMN"},{"k":1,"v":"ABCDEFGHIJKLMNOPQRSTUVWXYZ"},{"k":2,"v":"ABCDEFGHIJ"}]`)
	requireNoParseErrors(t, app)
	d, err := app.Dump(40, 24)
	if err != nil {
		t.Fatal(err)
	}
	if got := diagsWithCode(d, "L008"); len(got) != 1 {
		t.Errorf("L008: %d diagnostics for one template, want 1: %v", len(got), got)
	}
	if got := diagsWithCode(d, "L009"); len(got) != 1 {
		t.Errorf("L009: %d diagnostics for one template, want 1: %v", len(got), got)
	}
	texts, boxes := 0, 0
	for _, n := range d.Nodes {
		switch {
		case n.Tag == "text" && n.Clipped == "text":
			texts++
		case n.Tag == "box" && n.Clipped == "children":
			boxes++
		}
	}
	if texts != 2 || boxes != 2 {
		t.Errorf("clipped rows: %d texts and %d boxes, want 2 and 2 (the third row fits)", texts, boxes)
	}
}
