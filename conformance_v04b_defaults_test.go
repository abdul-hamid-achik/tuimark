package tuimark_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

// SPEC v0.4b §24 conformance tests of the layout defaults (phase F5):
// tests 129 (its current-language bullet), 156-159, 165-171, and 221.
// Each runs the current-language document as the SPEC writes it, from
// testdata/v04b/<test>/, and its pinned twin, the same text with
// version="3" on <tui> (specs/fixtures/v04b/twins/), whose results are
// tuimark 0.4.1's. The twins are also frozen entries of the golden
// manifest (t156-…-041 and the others), written by tuimark 0.4.1 itself,
// which tuimark test checks byte for byte; here the test checks the
// values the SPEC states for them. The CLI parts (inspect, validate's
// exit code) are cmd/tuimark's conformance_v04b_defaults_test.go.

const (
	v04bDir      = "testdata/v04b"
	twinsDir     = "specs/fixtures/v04b/twins"
	labelCurrent = "current"
	labelTwin    = "twin"
)

// f5Load loads a document from a file and binds the JSON file data at
// the root ("" for none).
func f5Load(t *testing.T, file, data string) *tuimark.App {
	t.Helper()
	app, err := tuimark.Load(file)
	if err != nil {
		t.Fatal(err)
	}
	if data != "" {
		raw, err := os.ReadFile(data)
		if err != nil {
			t.Fatal(err)
		}
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			t.Fatal(err)
		}
		if err := app.Bind("", v); err != nil {
			t.Fatal(err)
		}
	}
	return app
}

// f5Parse is f5Load for a document given as text, written to a
// temporary file so a <style src> or a relative path never matters.
func f5Parse(t *testing.T, src, data string) *tuimark.App {
	t.Helper()
	path := filepath.Join(t.TempDir(), "doc.tui")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return f5Load(t, path, data)
}

// f5Dump dumps app at cols×rows.
func f5Dump(t *testing.T, app *tuimark.App, cols, rows int) *tuimark.Dump {
	t.Helper()
	d, err := app.Dump(cols, rows)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// f5Twin is the twin of a current-language document: the same text with
// version="3" on <tui>.
func f5Twin(src string) string {
	if !strings.HasPrefix(src, "<tui>") {
		panic("not a <tui> without version: " + src)
	}
	return `<tui version="3">` + strings.TrimPrefix(src, "<tui>")
}

// f5Geo is node's geometry as the SPEC writes it, "WxH at (X,Y)".
func f5Geo(n tuimark.DumpNode) string {
	return fmt.Sprintf("%dx%d at (%d,%d)", n.W, n.H, n.X, n.Y)
}

// f5Want checks the geometry of nodes by id; "#tag[k]" names the k-th
// node of that tag (0-based, in dump order) for a node with no id.
func f5Want(t *testing.T, label string, d *tuimark.Dump, want map[string]string) {
	t.Helper()
	for key, w := range want {
		var got []string
		for _, n := range d.Nodes {
			if n.ID == key {
				got = append(got, f5Geo(n))
			}
		}
		if strings.HasPrefix(key, "#") {
			tag, k, _ := strings.Cut(strings.TrimPrefix(key, "#"), "[")
			idx := 0
			fmt.Sscanf(k, "%d]", &idx)
			i := 0
			for _, n := range d.Nodes {
				if n.Tag == tag && n.ID == "" {
					if i == idx {
						got = append(got, f5Geo(n))
					}
					i++
				}
			}
		}
		if len(got) != 1 || got[0] != w {
			t.Errorf("%s: %s is %v, want %s", label, key, got, w)
		}
	}
}

// f5Diags lists diagnostics as "SEVERITY CODE #ID MSG" ("SEVERITY CODE MSG"
// without an id).
func f5Diags(ds []tuimark.Diagnostic) []string {
	out := []string{}
	for _, d := range ds {
		id := ""
		if d.ID != "" {
			id = " #" + d.ID
		}
		out = append(out, fmt.Sprintf("%s %s%s %s", d.Severity, d.Code, id, d.Msg))
	}
	return out
}

// f5WantDiags checks the diagnostics, in any order (the SPEC lists them
// without one).
func f5WantDiags(t *testing.T, label string, got []tuimark.Diagnostic, want ...string) {
	t.Helper()
	want = append([]string{}, want...)
	sort.Strings(want)
	g := f5Diags(got)
	sort.Strings(g)
	if !reflect.DeepEqual(g, want) {
		t.Errorf("%s: diagnostics\n%q\nwant\n%q", label, g, want)
	}
}

// f5Sub replaces old, which must occur exactly once in src, by new, so a
// variant never silently tests the unchanged document.
func f5Sub(t *testing.T, src, old, new string) string {
	t.Helper()
	if n := strings.Count(src, old); n != 1 {
		t.Fatalf("%q occurs %d times", old, n)
	}
	return strings.Replace(src, old, new, 1)
}

// f5Read reads a fixture file.
func f5Read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Every fixture of testdata/v04b this phase reads is the SPEC's document
// as written, and its twin in specs/fixtures/v04b/twins is the same text
// with version="3" (test 221's twin adds the style its pinned bullet
// names).
func TestV04bDefaultsFixturesAndTwins(t *testing.T) {
	for _, c := range []struct{ test, file string }{
		{"156", "hints-col.tui"}, {"157", "hints-row.tui"}, {"158", "confirm.tui"}, {"159", "help.tui"},
		{"165", "panes.tui"}, {"166", "stack.tui"}, {"167", "pct.tui"}, {"168", "starve.tui"},
		{"168", "starve-app.tui"}, {"169", "twopane.tui"}, {"170", "freeze.tui"},
	} {
		cur := f5Read(t, filepath.Join(v04bDir, c.test, c.file))
		if twin := f5Read(t, filepath.Join(twinsDir, c.file)); twin != f5Twin(cur) {
			t.Errorf("%s/%s: the twin is not the document with version=\"3\"", c.test, c.file)
		}
	}
	cur := f5Read(t, filepath.Join(v04bDir, "221", "field.tui"))
	want := f5Sub(t, f5Twin(cur), `title="new">`, `title="new" style="width: auto; height: auto">`)
	if twin := f5Read(t, filepath.Join(twinsDir, "field.tui")); twin != want {
		t.Error("221: the twin is not the document with version=\"3\" and the modal's style")
	}
}

// 129, last bullet. The col/row document without version lays out by
// §11.2's current-language rule: the row is a child of screen (1fr), the
// cols are not (content-sized across, filling down). Its version="3"
// form, the frozen twin colrow-041, gives both cols 1fr.
func TestV04bColRowCurrentLanguage(t *testing.T) {
	const doc = `<tui><screen><row><col><text>a</text></col><col><text>b</text></col></row></screen></tui>`
	for _, c := range []struct {
		src  string
		want map[string]string
	}{
		{doc, map[string]string{"#row[0]": "20x3 at (0,0)", "#col[0]": "1x3 at (0,0)", "#col[1]": "1x3 at (1,0)"}},
		{f5Twin(doc), map[string]string{"#row[0]": "20x3 at (0,0)", "#col[0]": "10x3 at (0,0)", "#col[1]": "10x3 at (10,0)"}},
	} {
		d := f5Dump(t, f5Parse(t, c.src, ""), 20, 3)
		f5Want(t, c.src, d, c.want)
		f5WantDiags(t, c.src, d.Errors)
	}
}

// 156. A <hints> in a column is auto: one row, and the col (1fr) takes
// the rest. The twin splits the screen by the sibling-fr rule.
func TestV04bHintsInAColumn(t *testing.T) {
	file := filepath.Join(v04bDir, "156", "hints-col.tui")
	d := f5Dump(t, f5Load(t, file, ""), 20, 6)
	f5Want(t, labelCurrent, d, map[string]string{"body": "20x5 at (0,0)", "keys": "20x1 at (0,5)"})
	f5WantDiags(t, labelCurrent, d.Errors)
	d = f5Dump(t, f5Load(t, filepath.Join(twinsDir, "hints-col.tui"), ""), 20, 6)
	f5Want(t, labelTwin, d, map[string]string{"body": "20x3 at (0,0)", "keys": "20x3 at (0,3)"})
}

// f5HintItems counts the hint items laid out (generated rows of the
// hints).
func f5HintItems(d *tuimark.Dump) int {
	n := 0
	for _, x := range d.Nodes {
		if x.Tag == "row" && x.ID == "" {
			n++
		}
	}
	return n
}

// 157. A <hints> in a row is auto and shrinks along its items' axis: it
// takes up to its full width before the spacer gets anything, and drops
// the items that do not fit, with no diagnostic.
func TestV04bHintsInARow(t *testing.T) {
	app := f5Load(t, filepath.Join(v04bDir, "157", "hints-row.tui"), "")
	d := f5Dump(t, app, 30, 1)
	f5Want(t, "30x1", d, map[string]string{"keys": "24x1 at (6,0)", "#spacer[0]": "1x1 at (5,0)"})
	if d.Grid[0] != "ready q quit  / find  r reload" || f5HintItems(d) != 3 {
		t.Errorf("30x1: row %q with %d items", d.Grid[0], f5HintItems(d))
	}
	f5WantDiags(t, "30x1", d.Errors)
	d = f5Dump(t, app, 20, 1)
	f5Want(t, "20x1", d, map[string]string{"keys": "15x1 at (5,0)", "#spacer[0]": "0x1 at (5,0)"})
	if d.Grid[0] != "readyq quit  / find " || f5HintItems(d) != 2 {
		t.Errorf("20x1: row %q with %d items", d.Grid[0], f5HintItems(d))
	}
	f5WantDiags(t, "20x1", d.Errors)
	d = f5Dump(t, f5Load(t, filepath.Join(twinsDir, "hints-row.tui"), ""), 30, 1)
	f5Want(t, labelTwin, d, map[string]string{"keys": "13x1 at (17,0)"})
	if f5HintItems(d) != 1 || !strings.Contains(d.Grid[0], "q quit") {
		t.Errorf("twin: row %q with %d items", d.Grid[0], f5HintItems(d))
	}
}

// 158. A modal with no size takes its content size, its title counting
// as width(title) + 6. The twin takes 80% of the root.
func TestV04bModalSizedByItsContent(t *testing.T) {
	file := filepath.Join(v04bDir, "158", "confirm.tui")
	d := f5Dump(t, f5Load(t, file, ""), 40, 12)
	f5Want(t, labelCurrent, d, map[string]string{
		"confirm": "20x4 at (10,4)", "#text[1]": "18x1 at (11,5)", "act": "18x1 at (11,6)",
		"yes": "7x1 at (11,6)", "no": "6x1 at (18,6)",
	})
	if d.Focus == nil || *d.Focus != "yes" {
		t.Errorf("focus = %v, want yes", d.Focus)
	}
	wantRows := []string{
		"          ┌─ delete? ────────┐          ",
		"          │Delete report.pdf?│          ",
		"          │[ yes ][ no ]     │          ",
		"          └──────────────────┘          ",
	}
	if got := d.Grid[4:8]; !reflect.DeepEqual(got, wantRows) {
		t.Errorf("rows 4-7:\n%q\nwant\n%q", got, wantRows)
	}
	f5WantDiags(t, labelCurrent, d.Errors)

	long := f5Sub(t, f5Read(t, file), `title="delete?"`, `title="delete this file now?"`)
	d = f5Dump(t, f5Parse(t, long, ""), 40, 12)
	f5Want(t, "long title", d, map[string]string{"confirm": "27x4 at (6,4)", "act": "25x1 at (7,6)"})
	if d.Grid[4] != "      ┌─ delete this file now? ─┐       " {
		t.Errorf("long title: row 4 = %q", d.Grid[4])
	}

	d = f5Dump(t, f5Load(t, filepath.Join(twinsDir, "confirm.tui"), ""), 40, 12)
	f5Want(t, labelTwin, d, map[string]string{"confirm": "32x9 at (4,1)", "act": "30x6 at (5,3)", "yes": "7x6 at (5,3)"})
}

// 159. A modal over its cap: 80% of the root unless a max is written,
// clipped and reported by L009 unless overflow: hidden is written; an
// explicit auto is capped by the root only.
func TestV04bModalOverItsCap(t *testing.T) {
	file := filepath.Join(v04bDir, "159", "help.tui")
	src := f5Read(t, file)
	d := f5Dump(t, f5Load(t, file, ""), 40, 12)
	f5Want(t, "help", d, map[string]string{"help": "28x9 at (6,1)"})
	if d.Grid[1] != "      ┌─ a long help title here ─┐      " {
		t.Errorf("help: row 1 = %q", d.Grid[1])
	}
	f5WantDiags(t, "help", d.Errors, "warning L009 #help cuts text[7] on y: 0 of 1 cells shown; give it room, or write overflow: hidden to keep the cut")

	hidden := f5Sub(t, src, `title="a long help title here">`, `title="a long help title here" style="overflow: hidden">`)
	d = f5Dump(t, f5Parse(t, hidden, ""), 40, 12)
	f5Want(t, "overflow: hidden", d, map[string]string{"help": "28x9 at (6,1)"})
	f5WantDiags(t, "overflow: hidden", d.Errors)

	var lines strings.Builder
	for i := 1; i <= 12; i++ {
		fmt.Fprintf(&lines, "<text>line %d</text>", i)
	}
	scroll := `<tui><screen id="main"><modal id="help" open="true" title="keys"><scroll id="body">` + lines.String() + `</scroll><text>esc closes</text></modal></screen></tui>`
	d = f5Dump(t, f5Parse(t, scroll, ""), 40, 12)
	f5Want(t, "scroll", d, map[string]string{"help": "12x9 at (14,1)", "body": "10x6 at (15,2)"})
	f5WantDiags(t, "scroll", d.Errors)

	untitled := f5Sub(t, src, ` title="a long help title here"`, "")
	for _, rule := range []string{"#help { height: auto; }", "#help { max-height: 100%; }"} {
		doc := f5Sub(t, untitled, "<tui>", "<tui><style>"+rule+"</style>")
		d = f5Dump(t, f5Parse(t, doc, ""), 40, 12)
		f5Want(t, rule, d, map[string]string{"help": "9x12 at (15,0)"})
		if rule == "#help { height: auto; }" {
			f5WantDiags(t, rule, d.Errors, "warning L009 #help cuts text[10] on y: 0 of 1 cells shown; give it room, or write overflow: hidden to keep the cut")
		}
	}

	d = f5Dump(t, f5Load(t, filepath.Join(twinsDir, "help.tui"), ""), 40, 12)
	f5Want(t, labelTwin, d, map[string]string{"help": "32x9 at (4,1)"})
}

// 165. col and row take their content size outside screen, modal, and
// tab; the twin splits the row.
func TestV04bColRowContentSize(t *testing.T) {
	file := filepath.Join(v04bDir, "165", "panes.tui")
	d := f5Dump(t, f5Load(t, file, ""), 30, 8)
	f5Want(t, labelCurrent, d, map[string]string{
		"top": "30x4 at (0,0)", "status": "30x4 at (0,4)", "nav": "5x4 at (0,0)", "main-pane": "12x4 at (5,0)",
	})
	f5WantDiags(t, labelCurrent, d.Errors)
	d = f5Dump(t, f5Load(t, filepath.Join(twinsDir, "panes.tui"), ""), 30, 8)
	f5Want(t, labelTwin, d, map[string]string{"nav": "15x4 at (0,0)", "main-pane": "15x4 at (15,0)"})
}

// 166. No sibling-fr rule outside viewports: an unsized row or box next
// to an fr sibling keeps its content size, and the fr row takes the rest.
func TestV04bNoSiblingFrOutsideViewports(t *testing.T) {
	d := f5Dump(t, f5Load(t, filepath.Join(v04bDir, "166", "stack.tui"), ""), 30, 8)
	f5Want(t, labelCurrent, d, map[string]string{
		"r1": "30x1 at (0,0)", "b": "30x1 at (0,1)", "r3": "30x5 at (0,2)", "r4": "30x1 at (0,7)",
		"x": "1x1 at (0,7)", "#spacer[0]": "28x1 at (1,7)", "y": "1x1 at (29,7)",
	})
	f5WantDiags(t, labelCurrent, d.Errors)
	d = f5Dump(t, f5Load(t, filepath.Join(twinsDir, "stack.tui"), ""), 30, 8)
	f5Want(t, labelTwin, d, map[string]string{
		"r1": "30x2 at (0,0)", "b": "30x2 at (0,2)", "r3": "30x2 at (0,4)", "r4": "30x2 at (0,6)",
		"x": "10x2 at (0,6)", "#spacer[0]": "10x2 at (10,6)", "y": "10x2 at (20,6)",
	})
}

// 167. L002 under a row that is now auto names the fix (decision 109);
// the L009 that cuts #half keeps the generic text, since #half writes its
// height. The twin has no diagnostic.
func TestV04bL002UnderAnAutoContainer(t *testing.T) {
	d := f5Dump(t, f5Load(t, filepath.Join(v04bDir, "167", "pct.tui"), ""), 30, 8)
	f5Want(t, labelCurrent, d, map[string]string{"r": "30x0 at (0,0)"})
	f5WantDiags(t, labelCurrent, d.Errors,
		"error L002 #half height: 50% is a percent of a parent sized by its content (auto); give the parent a size (height: 1fr fills it)",
		"warning L009 #r cuts #half on y: 0 of 1 cells shown; give it room, or write overflow: hidden to keep the cut",
	)
	d = f5Dump(t, f5Load(t, filepath.Join(twinsDir, "pct.tui"), ""), 30, 8)
	f5Want(t, labelTwin, d, map[string]string{"r": "30x8 at (0,0)", "half": "1x4 at (0,0)"})
	f5WantDiags(t, labelTwin, d.Errors)
}

// 168. A viewport next to an fr sibling is 1fr whatever its data (ADR
// 0031); with no fr sibling it stays auto and is served after the
// non-shrinkable auto siblings.
func TestV04bViewportNextToAnFrSibling(t *testing.T) {
	data := func(n string) string { return filepath.Join(v04bDir, "168", n+".json") }
	file := filepath.Join(v04bDir, "168", "starve.tui")
	for _, n := range []string{"d30", "d3"} {
		for _, c := range []struct {
			label string
			app   *tuimark.App
		}{
			{"as written", f5Load(t, file, data(n))},
			{"height: 1fr", f5Parse(t, f5Sub(t, f5Read(t, file), `key="x">`, `key="x" style="height: 1fr">`), data(n))},
			{labelTwin, f5Load(t, filepath.Join(twinsDir, "starve.tui"), filepath.Join(twinsDir, n+".json"))},
		} {
			d := f5Dump(t, c.app, 20, 10)
			f5Want(t, n+" "+c.label, d, map[string]string{"l": "20x5 at (0,0)", "detail": "20x5 at (0,5)"})
			f5WantDiags(t, n+" "+c.label, d.Errors)
		}
	}
	app := filepath.Join(v04bDir, "168", "starve-app.tui")
	for n, want := range map[string]map[string]string{
		"d30": {"l": "20x9 at (0,0)", "detail": "20x1 at (0,9)"},
		"d3":  {"l": "20x3 at (0,0)", "detail": "20x1 at (0,3)"},
	} {
		d := f5Dump(t, f5Load(t, app, data(n)), 20, 10)
		f5Want(t, "no fr sibling "+n, d, want)
		f5WantDiags(t, "no fr sibling "+n, d.Errors)
		d = f5Dump(t, f5Load(t, filepath.Join(twinsDir, "starve-app.tui"), data(n)), 20, 10)
		f5Want(t, "twin, no fr sibling "+n, d, map[string]string{"l": "20x5 at (0,0)", "detail": "20x5 at (0,5)"})
	}
}

// 169. Content-sized panes do not shrink: the paragraph is measured
// unwrapped, and L009 names the cause and the fix (decision 109); width:
// 1fr makes the pane fit and wrap.
func TestV04bContentSizedPanesDoNotShrink(t *testing.T) {
	file := filepath.Join(v04bDir, "169", "twopane.tui")
	d := f5Dump(t, f5Load(t, file, ""), 40, 6)
	f5Want(t, labelCurrent, d, map[string]string{"nav": "5x6 at (0,0)", "body": "70x6 at (5,0)", "para": "70x1 at (5,0)"})
	f5WantDiags(t, labelCurrent, d.Errors, "warning L009 #top cuts #body on x: 35 of 70 cells shown; #body is sized by its content and does not shrink: give it width: 1fr (or flex: 1), or write overflow: hidden to keep the cut")
	if d.Grid[0] != "inboxThe quick brown fox jumps over the " {
		t.Errorf("row 0 = %q", d.Grid[0])
	}
	fr := f5Sub(t, f5Read(t, file), `<col id="body">`, `<col id="body" style="width: 1fr">`)
	d = f5Dump(t, f5Parse(t, fr, ""), 40, 6)
	f5Want(t, "width: 1fr", d, map[string]string{"body": "35x6 at (5,0)", "para": "35x2 at (5,0)"})
	f5WantDiags(t, "width: 1fr", d.Errors)
	d = f5Dump(t, f5Load(t, filepath.Join(twinsDir, "twopane.tui"), ""), 40, 6)
	f5Want(t, labelTwin, d, map[string]string{"nav": "20x6 at (0,0)", "body": "20x6 at (20,0)", "para": "20x4 at (20,0)"})

	d = f5Dump(t, f5Load(t, filepath.Join(v04bDir, "165", "panes.tui"), ""), 12, 8)
	f5Want(t, "panes 12x8", d, map[string]string{"nav": "5x4 at (0,0)", "main-pane": "12x4 at (5,0)"})
	f5WantDiags(t, "panes 12x8", d.Errors, "warning L009 #top cuts #main-pane on x: 7 of 12 cells shown; #main-pane is sized by its content and does not shrink: give it width: 1fr (or flex: 1), or write overflow: hidden to keep the cut")
	d = f5Dump(t, f5Load(t, filepath.Join(twinsDir, "panes.tui"), ""), 12, 8)
	f5Want(t, "twin panes 12x8", d, map[string]string{"nav": "6x4 at (0,0)", "main-pane": "6x4 at (6,0)"})
	if codes := f5Diags(d.Errors); len(codes) != 1 || !strings.HasPrefix(codes[0], "warning L008 ") {
		t.Errorf("twin panes 12x8: diagnostics %q, want one L008", codes)
	}
}

// f5Widths lists the widths (or, for the col, the heights) of the boxes
// of one row of test 170's table.
func f5Widths(d *tuimark.Dump, height bool, ids ...string) string {
	var out []string
	for _, id := range ids {
		for _, n := range d.Nodes {
			if n.ID == id {
				if height {
					out = append(out, fmt.Sprint(n.H))
				} else {
					out = append(out, fmt.Sprint(n.W))
				}
			}
		}
	}
	return strings.Join(out, " ")
}

// 170. fr with min/max: in the current language a clamped fr child is
// frozen and what it frees is shared again, as CSS resolves flexible
// lengths; the twin clamps after one split. A table's columns keep §29
// item 23 in both.
func TestV04bFrWithMinMax(t *testing.T) {
	type row struct{ r1, r3, r4, c1 string }
	for _, c := range []struct {
		file  string
		cols  int
		want  row
		diags []string
	}{
		{filepath.Join(v04bDir, "170", "freeze.tui"), 30, row{"5 12 13", "40 0 0", "14 14 2", "4 12 4"}, []string{"L003 #r3"}},
		{filepath.Join(v04bDir, "170", "freeze.tui"), 90, row{"5 42 43", "40 20 30", "44 44 2", "4 12 4"}, nil},
		{filepath.Join(v04bDir, "170", "freeze.tui"), 10, row{"3 3 4", "40 0 0", "4 4 2", "4 12 4"}, []string{"L003 #r3"}},
		{filepath.Join(twinsDir, "freeze.tui"), 30, row{"5 10 10", "40 10 10", "10 10 2", "6 12 8"}, []string{"L003 #r3", "L009 #c1"}},
		{filepath.Join(twinsDir, "freeze.tui"), 90, row{"5 30 30", "40 20 30", "30 30 2", "6 12 8"}, []string{"L009 #c1"}},
		{filepath.Join(twinsDir, "freeze.tui"), 10, row{"3 3 4", "40 3 4", "3 3 2", "6 12 8"}, []string{"L003 #r3", "L009 #c1"}},
	} {
		label := fmt.Sprintf("%s at %d", c.file, c.cols)
		d := f5Dump(t, f5Load(t, c.file, ""), c.cols, 24)
		got := row{
			f5Widths(d, false, "a1", "b1", "c1x"), f5Widths(d, false, "a3", "b3", "c3"),
			f5Widths(d, false, "a4", "b4", "c4"), f5Widths(d, true, "a2", "b2", "c2"),
		}
		if got != c.want {
			t.Errorf("%s: %+v, want %+v", label, got, c.want)
		}
		var codes []string
		for _, e := range d.Errors {
			codes = append(codes, e.Code+" #"+e.ID)
		}
		if !reflect.DeepEqual(codes, c.diags) {
			t.Errorf("%s: diagnostics %q, want %q", label, codes, c.diags)
		}
	}
	const table = `<tui>
  <style>#c1 { max-width: 5; }</style>
  <screen id="main">
    <table id="t" each="rows as r" key="r.id">
      <column id="c1" title="a" width="1fr">{r.a}</column>
      <column id="c2" title="b" width="1fr">{r.b}</column>
      <column id="c3" title="c" width="1fr">{r.c}</column>
    </table>
  </screen>
</tui>`
	data := filepath.Join(t.TempDir(), "rows.json")
	if err := os.WriteFile(data, []byte(`{"rows": [{"id": 1, "a": "x", "b": "y", "c": "z"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, src := range []string{table, f5Twin(table)} {
		d := f5Dump(t, f5Parse(t, src, data), 30, 5)
		f5Want(t, "table", d, map[string]string{"c1": "5x1 at (0,0)", "c2": "10x1 at (5,0)", "c3": "10x1 at (15,0)"})
	}
}

// 171. What does not change: an each container's generated rows take
// their content size (their twin shares the col); docks, scrolls, and
// grids lay out as in 0.4.1; a modal's built-in col 1fr is auto along its
// scroll axis, with no L001. (Pinned and spike documents: the golden
// manifest, which tuimark test runs, and TestPinnedDocumentsDumpAsIn041.)
func TestV04bWhatDoesNotChange(t *testing.T) {
	const each = `<tui><screen id="main"><col id="app"><col id="feed" each="xs as x"><row><text>{x}</text></row></col><text id="end">end</text></col></screen></tui>`
	data := filepath.Join(t.TempDir(), "each.json")
	if err := os.WriteFile(data, []byte(`{"xs": ["a", "b"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	d := f5Dump(t, f5Parse(t, each, data), 30, 8)
	f5Want(t, "each", d, map[string]string{"feed": "30x2 at (0,0)", "#row[0]": "30x1 at (0,0)", "#row[1]": "30x1 at (0,1)", "end": "30x1 at (0,2)"})
	d = f5Dump(t, f5Parse(t, f5Twin(each), data), 30, 8)
	f5Want(t, "each twin", d, map[string]string{"feed": "30x7 at (0,0)", "#row[0]": "30x3 at (0,0)", "#row[1]": "30x4 at (0,3)", "end": "30x1 at (0,7)"})

	const docks = `<tui><screen id="main"><row id="bar" style="dock: bottom"><text>status</text></row><scroll id="s"><col id="c"><text>a</text></col></scroll><box id="g" style="layout: grid; grid-columns: 2"><col id="g1"><text>x</text></col><row id="g2"><text>y</text></row></box></screen></tui>`
	for _, src := range []string{docks, f5Twin(docks)} {
		d := f5Dump(t, f5Parse(t, src, ""), 30, 8)
		f5Want(t, src, d, map[string]string{
			"bar": "30x1 at (0,7)", "s": "30x1 at (0,0)", "c": "30x1 at (0,0)",
			"g": "30x1 at (0,1)", "g1": "15x1 at (0,1)", "g2": "15x1 at (15,1)",
		})
		f5WantDiags(t, src, d.Errors)
	}

	const l001 = `<tui><screen id="main"><modal id="m" open="true" style="overflow: scroll"><col id="c"><text>a</text></col></modal></screen></tui>`
	d = f5Dump(t, f5Parse(t, l001, ""), 30, 10)
	f5Want(t, "L001", d, map[string]string{"m": "3x3 at (13,3)"})
	f5WantDiags(t, "L001", d.Errors)
}

// 221. An input in a modal sized by its content: the input is max(0 + 1,
// 20) columns whatever its value, so typing never moves it or the modal;
// a placeholder can widen it; a secret follows the same rule; a long
// value scrolls inside. The pinned twin grows with the value, as 0.4.1.
func TestV04bInputInAModalSizedByItsContent(t *testing.T) {
	file := filepath.Join(v04bDir, "221", "field.tui")
	data := filepath.Join(v04bDir, "221", "t.json")
	play := func(t *testing.T, app *tuimark.App, steps ...string) *tuimark.PlayResult {
		t.Helper()
		res, err := app.Play(tuimark.PlayOptions{Cols: 40, Rows: 12, NoHandlers: true, Frames: true}, steps...)
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Events) != 0 || len(res.Frames) != 1+len(steps) {
			t.Fatalf("events %v, %d frames", res.Events, len(res.Frames))
		}
		return res
	}
	res := play(t, f5Load(t, file, data), "text:abcdefgh")
	for i, f := range res.Frames {
		f5Want(t, fmt.Sprintf("frame %d", i), f.Dump, map[string]string{"m": "22x3 at (9,4)", "t": "20x1 at (10,5)"})
	}
	wantRows := []string{
		"         ┌─ new ──────────────┐         ",
		"         │abcdefgh            │         ",
		"         └────────────────────┘         ",
	}
	if got := res.Frames[1].Dump.Grid[4:7]; !reflect.DeepEqual(got, wantRows) {
		t.Errorf("step 1, rows 4-6:\n%q\nwant\n%q", got, wantRows)
	}
	if got := res.Frames[0].Dump.Grid[5]; got != "         │                    │         " {
		t.Errorf("step 0, row 5 = %q", got)
	}

	src := f5Read(t, file)
	for _, c := range []struct{ label, attr, m, t string }{
		{"placeholder", ` placeholder="type a new name here"`, "23x3 at (8,4)", "21x1 at (9,5)"},
		{"secret", ` secret="true"`, "22x3 at (9,4)", "20x1 at (10,5)"},
	} {
		doc := f5Sub(t, src, `bind="t"/>`, `bind="t"`+c.attr+`/>`)
		res := play(t, f5Parse(t, doc, data), "text:abcdefgh")
		for i, f := range res.Frames {
			f5Want(t, fmt.Sprintf("%s, frame %d", c.label, i), f.Dump, map[string]string{"m": c.m, "t": c.t})
		}
	}
	res = play(t, f5Load(t, file, data), "text:abcdefghijklmnopqrstuvwxyz")
	for i, f := range res.Frames {
		f5Want(t, fmt.Sprintf("long value, frame %d", i), f.Dump, map[string]string{"m": "22x3 at (9,4)"})
	}

	res = play(t, f5Load(t, filepath.Join(twinsDir, "field.tui"), filepath.Join(twinsDir, "t.json")), "text:abcdefgh")
	f5Want(t, "pinned, step 0", res.Frames[0].Dump, map[string]string{"m": "3x3 at (18,4)"})
	f5Want(t, "pinned, step 1", res.Frames[1].Dump, map[string]string{"m": "11x3 at (14,4)"})
}
