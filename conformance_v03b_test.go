package tuimark_test

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

// SPEC v0.3 §21 conformance tests of 0.3b's layout and CSS vocabulary
// (this worktree's area only): row-gap/column-gap (§10.2, §11.7),
// wrap: truncate-start/truncate-middle (§10.3, §11.5.2, §6.9.4), column
// priority (§6.9.3), and sparkline scale groups (§6.11). stick, a
// modal's focus, keymap when, and tab-label counters are other areas'
// items and are not covered here (test 91 is split across areas).

// versionHint3 ends every v0.3b V002/V003 message that needs
// version="3" (SPEC v0.3 §5.1).
const versionHint3 = ` (requires version="3")`

// 91 (this area's items). Each of row-gap, column-gap, and the two wrap
// values is valid in a version="3" document and gives V003 with the
// version="3" hint in version="1" and version="2" documents alike (they
// sit on existing tags, so the tag itself is never the problem). priority
// and scale sit on the 0.2b tags column and sparkline: in a version="2"
// document they give V002 with the version="3" hint, but in a version="1"
// document the tag itself is unknown, so they give only that tag's V001
// with the version="2" hint, not a second diagnostic for the attribute
// (SPEC §5.1 "a tag the version does not know has its attributes checked
// no further").
func TestVersionGate03bLayoutAndCSS(t *testing.T) {
	fill := func(tpl, version string) string { return strings.Replace(tpl, "VERSION", version, 1) }
	hasCode := func(diags []tuimark.Diagnostic, code, suffix string) bool {
		for _, d := range diags {
			if d.Code == code && strings.HasSuffix(d.Msg, suffix) {
				return true
			}
		}
		return false
	}
	hasError := func(diags []tuimark.Diagnostic) bool {
		for _, d := range diags {
			if d.Severity == "error" {
				return true
			}
		}
		return false
	}

	t.Run("row-gap property", func(t *testing.T) {
		tpl := `<tui version="VERSION"><style>box { row-gap: 1; }</style><screen id="main"><box><text>x</text></box></screen></tui>`
		for _, v := range []string{"1", "2"} {
			d := validateSrc(t, fill(tpl, v))
			if !hasCode(d, "V003", versionHint3) {
				t.Errorf("version=%q: want V003 ending %q, got %v", v, versionHint3, d)
			}
		}
		if d := validateSrc(t, fill(tpl, "3")); hasError(d) {
			t.Errorf("version=\"3\": unexpected errors %v", d)
		}
	})

	t.Run("column-gap property", func(t *testing.T) {
		tpl := `<tui version="VERSION"><style>box { column-gap: 1; }</style><screen id="main"><box><text>x</text></box></screen></tui>`
		for _, v := range []string{"1", "2"} {
			d := validateSrc(t, fill(tpl, v))
			if !hasCode(d, "V003", versionHint3) {
				t.Errorf("version=%q: want V003 ending %q, got %v", v, versionHint3, d)
			}
		}
		if d := validateSrc(t, fill(tpl, "3")); hasError(d) {
			t.Errorf("version=\"3\": unexpected errors %v", d)
		}
	})

	for _, val := range []string{"truncate-start", "truncate-middle"} {
		val := val
		t.Run("wrap "+val, func(t *testing.T) {
			tpl := `<tui version="VERSION"><screen id="main"><text wrap="` + val + `">x</text></screen></tui>`
			for _, v := range []string{"1", "2"} {
				d := validateSrc(t, fill(tpl, v))
				if !hasCode(d, "V003", versionHint3) {
					t.Errorf("version=%q: want V003 ending %q, got %v", v, versionHint3, d)
				}
			}
			if d := validateSrc(t, fill(tpl, "3")); hasError(d) {
				t.Errorf("version=\"3\": unexpected errors %v", d)
			}
		})
	}

	t.Run("column priority", func(t *testing.T) {
		tpl := `<tui version="VERSION"><screen id="main"><table id="t" each="xs as x" key="x"><column id="c" priority="1">{x}</column></table></screen></tui>`
		if d := validateSrc(t, fill(tpl, "1")); !hasCode(d, "V001", versionHint) || hasCode(d, "V002", "") {
			t.Errorf("version=\"1\": want only the tag's V001 ending %q, got %v", versionHint, d)
		}
		if d := validateSrc(t, fill(tpl, "2")); !hasCode(d, "V002", versionHint3) {
			t.Errorf("version=\"2\": want V002 ending %q, got %v", versionHint3, d)
		}
		for _, d := range validateSrc(t, fill(tpl, "3")) {
			if strings.HasPrefix(d.Code, "B") {
				continue
			}
			if d.Severity == "error" {
				t.Errorf("version=\"3\": unexpected %s", d)
			}
		}
	})

	t.Run("sparkline scale", func(t *testing.T) {
		tpl := `<tui version="VERSION"><screen id="main"><sparkline id="s" bind="xs" scale="g"/></screen></tui>`
		if d := validateSrc(t, fill(tpl, "1")); !hasCode(d, "V001", versionHint) || hasCode(d, "V002", "") {
			t.Errorf("version=\"1\": want only the tag's V001 ending %q, got %v", versionHint, d)
		}
		if d := validateSrc(t, fill(tpl, "2")); !hasCode(d, "V002", versionHint3) {
			t.Errorf("version=\"2\": want V002 ending %q, got %v", versionHint3, d)
		}
		for _, d := range validateSrc(t, fill(tpl, "3")) {
			if strings.HasPrefix(d.Code, "B") {
				continue
			}
			if d.Severity == "error" {
				t.Errorf("version=\"3\": unexpected %s", d)
			}
		}
	})

	t.Run("scale bad name", func(t *testing.T) {
		src := `<tui version="3"><screen id="main"><sparkline id="s" bind="xs" scale="bad name"/></screen></tui>`
		if d := validateSrc(t, src); !hasCode(d, "V003", "") {
			t.Errorf("scale=\"bad name\": want V003, got %v", d)
		}
	})

	t.Run("priority bad value", func(t *testing.T) {
		for _, val := range []string{"-1", "x"} {
			src := `<tui version="3"><screen id="main"><table id="t" each="xs as x" key="x"><column id="c" priority="` + val + `">{x}</column></table></screen></tui>`
			if d := validateSrc(t, src); !hasCode(d, "V003", "") {
				t.Errorf("priority=%q: want V003, got %v", val, d)
			}
		}
	})

	t.Run("stylesheet loaded by two versions", func(t *testing.T) {
		// A style src="" has no version of its own (SPEC §5.1): the same
		// row-gap rule is V003 with the hint under version="2" and clean
		// under version="3".
		tpl := `<tui version="VERSION"><style>#g { row-gap: 1; }</style><screen id="main"><box id="g"><text>x</text></box></screen></tui>`
		if d := validateSrc(t, fill(tpl, "2")); !hasCode(d, "V003", versionHint3) {
			t.Errorf("version=\"2\": want V003 ending %q, got %v", versionHint3, d)
		}
		if d := validateSrc(t, fill(tpl, "3")); hasError(d) {
			t.Errorf("version=\"3\": unexpected errors %v", d)
		}
	})

	t.Run("invalid wrap value lists the new values only in version 3", func(t *testing.T) {
		v2 := validateSrc(t, `<tui version="2"><screen id="main"><text wrap="bogus">x</text></screen></tui>`)
		v3 := validateSrc(t, `<tui version="3"><screen id="main"><text wrap="bogus">x</text></screen></tui>`)
		var m2, m3 string
		for _, d := range v2 {
			if d.Code == "V003" {
				m2 = d.Msg
			}
		}
		for _, d := range v3 {
			if d.Code == "V003" {
				m3 = d.Msg
			}
		}
		if strings.Contains(m2, "truncate-start") {
			t.Errorf("version=\"2\" message lists truncate-start: %q", m2)
		}
		if !strings.Contains(m3, "truncate-start") || !strings.Contains(m3, "truncate-middle") {
			t.Errorf("version=\"3\" message does not list the new values: %q", m3)
		}
	})
}

// 95. priority (SPEC v0.3b §6.9.3, the fixture and vectors of §21 test
// 95 verbatim).
const priorityFixture = `<tui version="3">
  <screen id="main">
    <table id="t" each="rows as r" key="r.id" gap="1">
      <column id="a" title="A" width="18">{r.a}</column>
      <column id="b" title="B" width="18" priority="3">{r.b}</column>
      <column id="c" title="C" width="18" priority="1">{r.c}</column>
      <column id="d" title="D" width="18" priority="2">{r.d}</column>
      <column id="e" title="E" width="18" priority="1">{r.e}</column>
    </table>
  </screen>
</tui>`

func priorityApp(t *testing.T) *tuimark.App {
	t.Helper()
	app := parseV3(t, priorityFixture)
	bindJSON(t, app, "", `{"rows": [{"id": 1, "a": "a1", "b": "b1", "c": "c1", "d": "d1", "e": "e1"}]}`)
	return app
}

func visibleColumns(d *tuimark.Dump) []string {
	var out []string
	for _, n := range d.Nodes {
		if n.Tag == "column" {
			out = append(out, n.ID)
		}
	}
	return out
}

func TestColumnPriority(t *testing.T) {
	for _, c := range []struct {
		cols int
		want string
	}{
		{120, "a b c d e"},
		{80, "a b c d"}, // e hides first: the smallest N, the last of the two priority="1"
		{60, "a b d"},
		{40, "a b"},
		{20, "a"},
	} {
		app := priorityApp(t)
		d, err := app.Dump(c.cols, 3)
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(visibleColumns(d), " "); got != c.want {
			t.Errorf("cols=%d: columns %q, want %q", c.cols, got, c.want)
		}
	}
	// At 10, only "a" is left, cut, with L003. The table has no border and
	// no mark, so A is the terminal width.
	app := priorityApp(t)
	d, err := app.Dump(10, 3)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(visibleColumns(d), " "); got != "a" {
		t.Errorf("cols=10: columns %q, want \"a\"", got)
	}
	if len(diagsWithCode(d, "L003")) == 0 {
		t.Errorf("cols=10: want L003, got %v", d.Errors)
	}

	// At 60 the columns are at x 0, 19, and 38 (gap 1 between 18-wide
	// columns). A hidden column has no header cell and no body cell in the
	// dump.
	app2 := priorityApp(t)
	d2, err := app2.Dump(60, 3)
	if err != nil {
		t.Fatal(err)
	}
	var xs []int
	for _, n := range d2.Nodes {
		if n.Tag == "column" {
			xs = append(xs, n.X)
		}
	}
	if len(xs) != 3 || xs[0] != 0 || xs[1] != 19 || xs[2] != 38 {
		t.Errorf("cols=60: column x %v, want [0 19 38]", xs)
	}
	for _, n := range d2.Nodes {
		if n.ID == "c" || n.ID == "e" {
			t.Errorf("hidden column %s is in the dump: %+v", n.ID, n)
		}
	}
	// The body row holds one cell per kept column, at the same x.
	var cells []string
	for _, n := range d2.Nodes {
		if n.Tag == "text" && n.Y == 1 {
			cells = append(cells, n.Text+"@"+strconv.Itoa(n.X))
		}
	}
	if got := strings.Join(cells, " "); got != "a1@0 b1@19 d1@38" {
		t.Errorf("cols=60: body cells %q, want a1@0 b1@19 d1@38", got)
	}
	for y, want := range []string{"A                  B                  D", "a1                 b1                 d1"} {
		if got := strings.TrimRight(d2.Grid[y], " "); got != want {
			t.Errorf("cols=60: grid[%d] = %q, want %q", y, got, want)
		}
	}

	// The table's intrinsic width itself (not its laid-out width here,
	// which stretches to fill the screen like any unsized block) does not
	// change when a column hides: internal/layout's
	// TestTablePriorityIntrinsicWidth covers that directly.

	// play --cols 60 --input "resize:120x3" shows all five in its final
	// frame.
	app4 := priorityApp(t)
	res, err := app4.Play(tuimark.PlayOptions{Cols: 60, Rows: 3}, "resize:120x3")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(visibleColumns(res.Dump), " "); got != "a b c d e" {
		t.Errorf("after resize: columns %q, want \"a b c d e\"", got)
	}
}

// dumpSrc parses src, binds data (a JSON object, or "" for none), and
// dumps it at cols×rows.
func dumpSrc(t *testing.T, src, data string, cols, rows int) *tuimark.Dump {
	t.Helper()
	app := parseV3(t, src)
	if data != "" {
		bindJSON(t, app, "", data)
	}
	d, err := app.Dump(cols, rows)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// nodeByKey returns the first dump node with the given tag and key.
func nodeByKey(d *tuimark.Dump, tag, key string) *tuimark.DumpNode {
	for i := range d.Nodes {
		if d.Nodes[i].Tag == tag && d.Nodes[i].Key == key {
			return &d.Nodes[i]
		}
	}
	return nil
}

// mustNode is dumpNode that fails the test when id is not in d.
func mustNode(t *testing.T, d *tuimark.Dump, id string) *tuimark.DumpNode {
	t.Helper()
	n := dumpNode(d, id)
	if n == nil {
		t.Fatalf("no node %q in the dump:\n%s", id, strings.Join(d.Grid, "\n"))
	}
	return n
}

// errorsOf lists d's error-severity diagnostics.
func errorsOf(d *tuimark.Dump) []tuimark.Diagnostic {
	var out []tuimark.Diagnostic
	for _, e := range d.Errors {
		if e.Severity == "error" {
			out = append(out, e)
		}
	}
	return out
}

// 91 (this area's items), continued. A .tcss file loaded with <style
// src> has no version of its own (SPEC §5.1): its row-gap, column-gap,
// and wrap: truncate-middle declarations are V003 with the version="3"
// hint when a version="1" or version="2" document loads it, and valid
// when a version="3" document does. An invalid wrap value keeps its
// (want wrap | nowrap | truncate) in the older documents and lists the
// two new values in a version="3" one, as a property and as the text
// attribute.
func TestVersionGate03bStylesheetFile(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "gaps.tcss"), "#g { row-gap: 1; column-gap: 2; }\ntext { wrap: truncate-middle; }\n")
	doc := `<tui version="%s"><style src="gaps.tcss"/><screen id="main"><box id="g"><text>x</text></box></screen></tui>`
	for _, v := range []string{"1", "2", "3"} {
		p := filepath.Join(dir, "v"+v+".tui")
		mustWrite(t, p, strings.Replace(doc, "%s", v, 1))
		app, err := tuimark.Load(p)
		if err != nil {
			t.Fatal(err)
		}
		var hinted []string
		for _, d := range app.Validate() {
			if d.Code == "V003" && d.File == "gaps.tcss" && strings.HasSuffix(d.Msg, versionHint3) {
				hinted = append(hinted, d.Msg)
			} else if d.Severity == "error" {
				t.Errorf("version=%s: unexpected %s", v, d)
			}
		}
		want := map[string]int{"1": 3, "2": 3, "3": 0}[v]
		if len(hinted) != want {
			t.Errorf("version=%s: hinted V003s %q, want %d", v, hinted, want)
		}
	}

	const old = `(want wrap | nowrap | truncate)`
	const v3 = `(want wrap | nowrap | truncate | truncate-start | truncate-middle)`
	for _, v := range []string{"1", "2", "3"} {
		src := `<tui version="` + v + `"><style>text { wrap: bogus; }</style><screen id="main"><text wrap="zz">x</text></screen></tui>`
		var msgs []string
		for _, d := range validateSrc(t, src) {
			if d.Code == "V003" {
				msgs = append(msgs, d.Msg)
			}
		}
		want := old
		if v == "3" {
			want = v3
		}
		if len(msgs) != 2 {
			t.Fatalf("version=%s: V003s %q, want 2", v, msgs)
		}
		for _, m := range msgs {
			if !strings.HasSuffix(m, want) {
				t.Errorf("version=%s: %q does not end with %q", v, m, want)
			}
		}
	}
}

// 92. scale="NAME" (SPEC v0.3b §6.11), end to end: the vectors of test
// 92 in the dump's grid.
func TestSparklineScale(t *testing.T) {
	const data = `{"a": [0, 5, 10], "b": [20, 40], "c": [1000], "h": [0, 80]}`
	spark := func(id, bind, attrs string) string {
		return `<sparkline id="` + id + `" bind="` + bind + `" width="6"` + attrs + `/>`
	}
	screen := func(body string) string {
		return `<tui version="3"><screen id="main">` + body + `</screen></tui>`
	}
	rows := func(t *testing.T, src string, n int) []string {
		t.Helper()
		d := dumpSrc(t, src, data, 6, n)
		if errs := errorsOf(d); len(errs) > 0 {
			t.Fatalf("errors %v", errs)
		}
		return d.Grid
	}
	check := func(t *testing.T, name string, got []string, want ...string) {
		t.Helper()
		for i, w := range want {
			if i >= len(got) || got[i] != w {
				t.Errorf("%s: rows %q, want %q", name, got, want)
				return
			}
		}
	}

	// One group: lo = 0 and hi = 40 for both.
	check(t, "group", rows(t, screen(spark("a", "a", ` scale="s"`)+spark("b", "b", ` scale="s"`)), 2),
		"    ▁▂", "    ▄█")
	// Without scale, each has its own range.
	check(t, "no scale", rows(t, screen(spark("a", "a", "")+spark("b", "b", "")), 2),
		"    ▄█", "     █")
	// A min/max literal on one member overrides only that member: a uses
	// its own 0..10, b still uses the group's 0..40 (not its own 20..40).
	check(t, "literal on one member", rows(t, screen(spark("a", "a", ` scale="s" min="0" max="10"`)+spark("b", "b", ` scale="s"`)), 2),
		"    ▄█", "    ▄█")
	// A group of one equals no scale.
	check(t, "group of one", rows(t, screen(spark("a", "a", ` scale="solo"`)+spark("b", "b", ` scale="other"`)), 2),
		"    ▄█", "     █")
	// An inactive tab's sparkline is not in the group (its 1000 would
	// flatten both); one with visibility: hidden is, and paints nothing:
	// with its 80, hi = 80.
	keys := `<keymap><bind keys="2" action="switch-to" to="#t2"/></keymap>`
	tabs := `<tabs id="nav"><tab id="t1" label="x"><text>-</text></tab><tab id="t2" label="y">` + spark("c", "c", ` scale="s"`) + `</tab></tabs>`
	check(t, "inactive tab", rows(t, screen(spark("a", "a", ` scale="s"`)+spark("b", "b", ` scale="s"`)+tabs), 4),
		"    ▁▂", "    ▄█")
	hidden := spark("h", "h", ` scale="s" style="visibility: hidden"`)
	check(t, "visibility hidden", rows(t, screen(spark("a", "a", ` scale="s"`)+spark("b", "b", ` scale="s"`)+hidden), 3),
		"    ▁▁", "    ▂▄", "      ")
	// The same inactive sparkline joins the group once its tab is active.
	app := parseV3(t, strings.Replace(screen(spark("a", "a", ` scale="s"`)+spark("b", "b", ` scale="s"`)+tabs), "<screen", keys+"<screen", 1))
	bindJSON(t, app, "", data)
	res, err := app.Play(tuimark.PlayOptions{Cols: 6, Rows: 4}, "2")
	if err != nil {
		t.Fatal(err)
	}
	check(t, "tab made active", res.Dump.Grid, "      ", "      ", "xy    ", "     █")
}

// gapCase is one container of test 93, written with the declarations
// decls, and a function that measures, from the dump, the space between
// its siblings side by side (x) and stacked (y); -1 where the container
// has no such pair.
type gapCase struct {
	name, data string
	src        func(decls string) string
	measure    func(t *testing.T, d *tuimark.Dump) (x, y int)
}

var gapCases = []gapCase{
	{
		name: "col",
		src: func(decls string) string {
			return `<style>#k { ` + decls + ` }</style><screen id="main"><col id="k"><text id="a">a</text><text id="b">b</text></col></screen>`
		},
		measure: func(t *testing.T, d *tuimark.Dump) (int, int) {
			a, b := mustNode(t, d, "a"), mustNode(t, d, "b")
			return -1, b.Y - (a.Y + a.H)
		},
	},
	{
		name: "row",
		src: func(decls string) string {
			return `<style>#k { ` + decls + ` }</style><screen id="main"><row id="k"><text id="a">a</text><text id="b">b</text></row></screen>`
		},
		measure: func(t *testing.T, d *tuimark.Dump) (int, int) {
			a, b := mustNode(t, d, "a"), mustNode(t, d, "b")
			return b.X - (a.X + a.W), -1
		},
	},
	{
		name: "grid",
		src: func(decls string) string {
			return `<style>#k { layout: grid; grid-columns: 2; ` + decls + ` }</style><screen id="main"><box id="k"><text id="a">a</text><text id="b">b</text><text id="c">c</text></box></screen>`
		},
		measure: func(t *testing.T, d *tuimark.Dump) (int, int) {
			a, b, c := mustNode(t, d, "a"), mustNode(t, d, "b"), mustNode(t, d, "c")
			if c.X != a.X {
				t.Errorf("grid: c at x %d, want a's %d", c.X, a.X)
			}
			return b.X - (a.X + a.W), c.Y - (a.Y + a.H)
		},
	},
	{
		name: "table",
		data: `{"rows": [{"id": 1, "a": "a1", "b": "b1"}, {"id": 2, "a": "a2", "b": "b2"}]}`,
		src: func(decls string) string {
			return `<style>#k { ` + decls + ` }</style><screen id="main"><table id="k" each="rows as r" key="r.id"><column id="a" title="A" width="3">{r.a}</column><column id="b" title="B" width="3">{r.b}</column></table></screen>`
		},
		measure: func(t *testing.T, d *tuimark.Dump) (int, int) {
			a, b := mustNode(t, d, "a"), mustNode(t, d, "b")
			// The rows are one below the other whatever the gaps: a table
			// has no row gap (SPEC §6.9.3).
			r1, r2 := nodeByKey(d, "item", "1"), nodeByKey(d, "item", "2")
			if r1 == nil || r2 == nil || r1.Y != 1 || r2.Y != 2 {
				t.Errorf("table rows %+v %+v, want y 1 and 2", r1, r2)
			}
			return b.X - (a.X + a.W), -1
		},
	},
	{
		name: "hints",
		src: func(decls string) string {
			return `<style>#k { ` + decls + ` }</style><keymap><bind keys="q" action="quit" label="quit"/><bind keys="x" action="ex" label="ex"/></keymap><screen id="main"><hints id="k"/></screen>`
		},
		measure: func(t *testing.T, d *tuimark.Dump) (int, int) {
			var items []tuimark.DumpNode
			for _, n := range d.Nodes {
				if n.Tag == "row" {
					items = append(items, n)
				}
			}
			if len(items) != 2 {
				t.Fatalf("hints: items %+v, want 2", items)
			}
			return items[1].X - (items[0].X + items[0].W), -1
		},
	},
	{
		name: "hints column",
		src: func(decls string) string {
			return `<style>#k { layout: column; ` + decls + ` }</style><keymap><bind keys="q" action="quit" label="quit"/><bind keys="x" action="ex" label="ex"/></keymap><screen id="main"><hints id="k"/></screen>`
		},
		measure: func(t *testing.T, d *tuimark.Dump) (int, int) {
			var items []tuimark.DumpNode
			for _, n := range d.Nodes {
				if n.Tag == "row" {
					items = append(items, n)
				}
			}
			if len(items) != 2 {
				t.Fatalf("hints: items %+v, want 2", items)
			}
			return -1, items[1].Y - (items[0].Y + items[0].H)
		},
	},
	{
		name: "tabs",
		src: func(decls string) string {
			return `<style>#k { ` + decls + ` }</style><screen id="main"><tabs id="k"><tab id="t1" label="one"><text id="p">panel</text></tab><tab id="t2" label="two"><text>x</text></tab></tabs></screen>`
		},
		measure: func(t *testing.T, d *tuimark.Dump) (int, int) {
			l1, l2 := nodeByKey(d, "text", "t1"), nodeByKey(d, "text", "t2")
			if l1 == nil || l2 == nil {
				t.Fatalf("tabs: labels %+v %+v", l1, l2)
			}
			// Never a gap between the strip and the panel (§6.10.4).
			if p := mustNode(t, d, "p"); p.Y != 1 {
				t.Errorf("tabs: panel text at y %d, want 1", p.Y)
			}
			return l2.X - (l1.X + l1.W), -1
		},
	},
}

// 93. row-gap and column-gap (SPEC v0.3b §10.2, §11.7): for a col, a row,
// a grid, a table, a hints (along a row and, with layout: column, down),
// and a tabs, `gap: 2; row-gap: 0` and `gap: 1; column-gap: 3` place the
// siblings side by side column-gap apart and the stacked ones row-gap
// apart. A version="2" document with only gap, and the same document as
// version="3", lay out alike.
func TestRowColumnGapPositions(t *testing.T) {
	for _, c := range gapCases {
		t.Run(c.name, func(t *testing.T) {
			for _, v := range []struct {
				decls  string
				wx, wy int
			}{
				{"gap: 2; row-gap: 0;", 2, 0},
				{"gap: 1; column-gap: 3;", 3, 1},
			} {
				d := dumpSrc(t, `<tui version="3">`+c.src(v.decls)+`</tui>`, c.data, 30, 6)
				if errs := errorsOf(d); len(errs) > 0 {
					t.Fatalf("%s: errors %v", v.decls, errs)
				}
				x, y := c.measure(t, d)
				if x != -1 && x != v.wx {
					t.Errorf("%s: side by side %d apart, want %d\n%s", v.decls, x, v.wx, strings.Join(d.Grid, "\n"))
				}
				if y != -1 && y != v.wy {
					t.Errorf("%s: stacked %d apart, want %d\n%s", v.decls, y, v.wy, strings.Join(d.Grid, "\n"))
				}
			}
			// Only gap: the same layout in version="2" and version="3",
			// both axes at the gap.
			d2 := dumpSrc(t, `<tui version="2">`+c.src("gap: 2;")+`</tui>`, c.data, 30, 6)
			d3 := dumpSrc(t, `<tui version="3">`+c.src("gap: 2;")+`</tui>`, c.data, 30, 6)
			if g2, g3 := strings.Join(d2.Grid, "\n"), strings.Join(d3.Grid, "\n"); g2 != g3 {
				t.Errorf("gap only: version=2 grid\n%s\nversion=3 grid\n%s", g2, g3)
			}
			if len(d2.Nodes) != len(d3.Nodes) {
				t.Fatalf("gap only: %d nodes in version=2, %d in version=3", len(d2.Nodes), len(d3.Nodes))
			}
			for i := range d2.Nodes {
				a, b := d2.Nodes[i], d3.Nodes[i]
				if a.Tag != b.Tag || a.ID != b.ID || a.X != b.X || a.Y != b.Y || a.W != b.W || a.H != b.H {
					t.Errorf("gap only: node %d is %+v in version=2, %+v in version=3", i, a, b)
				}
			}
			x, y := c.measure(t, d2)
			if (x != -1 && x != 2) || (y != -1 && y != 2) {
				t.Errorf("gap only in version=2: %d and %d apart, want 2", x, y)
			}
		})
	}
}

// 93. The hint item's own keycap-to-label space is its column-gap: the
// built-in `hints > row { gap: 1 }` gives 1, and a column-gap rule on the
// item row changes it.
func TestHintItemColumnGap(t *testing.T) {
	for _, c := range []struct {
		css  string
		want int
	}{
		{"", 1},
		{"hints > row { column-gap: 3; }", 3},
		{"hints > row { row-gap: 3; }", 1},
	} {
		d := dumpSrc(t, `<tui version="3"><style>`+c.css+`</style><keymap><bind keys="q" action="quit" label="quit"/></keymap><screen id="main"><hints id="k"/></screen></tui>`, "", 20, 1)
		var key, label *tuimark.DumpNode
		for i, n := range d.Nodes {
			switch {
			case n.Tag == "text" && n.Text == "q":
				key = &d.Nodes[i]
			case n.Tag == "text" && n.Text == "quit":
				label = &d.Nodes[i]
			}
		}
		if key == nil || label == nil {
			t.Fatalf("%q: no hint item texts: %+v", c.css, d.Nodes)
		}
		if got := label.X - (key.X + key.W); got != c.want {
			t.Errorf("%q: keycap and label %d apart, want %d", c.css, got, c.want)
		}
	}
}

// 93. The cascade model (SPEC v0.3b §10.2): with .x { row-gap: 0 } and
// #g { gap: 2 } on the same grid, #g's gap wins row-gap too (higher
// specificity), so its rows are 2 apart; with #g { gap: 2; row-gap: 0 }
// the later longhand wins and they touch. The columns stay 2 apart in
// both. (internal/host TestInspectGapShorthand checks inspect's winner.)
func TestGapShorthandCascadeGrid(t *testing.T) {
	grid := func(css string) (colGap, rowGap int) {
		d := dumpSrc(t, `<tui version="3"><style>`+css+`</style><screen id="main"><box id="g" class="x" style="layout: grid; grid-columns: 2"><text id="a">a</text><text id="b">b</text><text id="c">c</text></box></screen></tui>`, "", 20, 6)
		if errs := errorsOf(d); len(errs) > 0 {
			t.Fatalf("%s: errors %v", css, errs)
		}
		a, b, c := mustNode(t, d, "a"), mustNode(t, d, "b"), mustNode(t, d, "c")
		return b.X - (a.X + a.W), c.Y - (a.Y + a.H)
	}
	if cg, rg := grid(".x { row-gap: 0; } #g { gap: 2; }"); cg != 2 || rg != 2 {
		t.Errorf(".x { row-gap: 0 } #g { gap: 2 }: columns %d rows %d apart, want 2 and 2", cg, rg)
	}
	if cg, rg := grid("#g { gap: 2; row-gap: 0; }"); cg != 2 || rg != 0 {
		t.Errorf("#g { gap: 2; row-gap: 0 }: columns %d rows %d apart, want 2 and 0", cg, rg)
	}
	// A row-gap written before gap in the same rule loses to it.
	if cg, rg := grid("#g { row-gap: 0; gap: 2; }"); cg != 2 || rg != 2 {
		t.Errorf("#g { row-gap: 0; gap: 2 }: columns %d rows %d apart, want 2 and 2", cg, rg)
	}
	// The gap attribute counts as a gap declaration too (origin
	// attribute), below any stylesheet rule.
	d := dumpSrc(t, `<tui version="3"><style>.x { column-gap: 0; }</style><screen id="main"><row id="r" class="x" gap="3"><text id="a">a</text><text id="b">b</text></row><col id="c" gap="3"><text id="e">e</text><text id="f">f</text></col></screen></tui>`, "", 20, 6)
	if a, b := mustNode(t, d, "a"), mustNode(t, d, "b"); b.X-(a.X+a.W) != 0 {
		t.Errorf("gap=\"3\" under .x { column-gap: 0 }: %d apart, want 0", b.X-(a.X+a.W))
	}
	if e, f := mustNode(t, d, "e"), mustNode(t, d, "f"); f.Y-(e.Y+e.H) != 3 {
		t.Errorf("gap=\"3\" on a col: %d apart, want 3", f.Y-(e.Y+e.H))
	}
}

// 94. Truncation in table cells (SPEC v0.3b §6.9.4, §11.5.2): a header
// cell and a body cell follow their computed wrap among the three
// truncations; wrap: wrap is painted as truncate and nowrap is clipped
// with no ellipsis, in every version that has tables.
func TestTableCellTruncation(t *testing.T) {
	const path = "/System/Volumes/Data"
	data := `{"rows": [{"id": 1, "p": "` + path + `"}]}`
	column := func(id, class string) string {
		return `<column id="` + id + `" class="` + class + `" title="` + path + `" width="12">{r.p}</column>`
	}
	doc := func(version string, cols ...string) string {
		css := ".w { wrap: wrap; }\n.nw { wrap: nowrap; }\n"
		if version == "3" {
			css += ".mid { wrap: truncate-middle; }\n.start { wrap: truncate-start; }\n"
		}
		return `<tui version="` + version + `"><style>` + css + `</style><screen id="main"><table id="t" each="rows as r" key="r.id" gap="1">` + strings.Join(cols, "") + `</table></screen></tui>`
	}
	const (
		mid   = "/Syste…/Data" // TruncateMiddle(path, 12)
		start = "…olumes/Data" // TruncateStart(path, 12)
		end   = "/System/Vol…" // Truncate(path, 12)
		clip  = "/System/Volu" // nowrap: clipped, no ellipsis
	)
	check := func(t *testing.T, d *tuimark.Dump, want ...string) {
		t.Helper()
		line := strings.Join(want, " ")
		for y := 0; y < 2; y++ { // the header row, then the body row
			if got := strings.TrimRight(d.Grid[y], " "); got != line {
				t.Errorf("row %d = %q, want %q", y, got, line)
			}
		}
		if n := len(diagsWithCode(d, "L008")); n != 0 {
			t.Errorf("L008 on a table cell: %v", d.Errors)
		}
	}
	d := dumpSrc(t, doc("3", column("a", "mid"), column("b", "start"), column("c", "w"), column("d", "nw"), column("e", "")), data, 64, 3)
	if errs := errorsOf(d); len(errs) > 0 {
		t.Fatalf("errors %v", errs)
	}
	check(t, d, mid, start, end, clip, end)
	for _, v := range []string{"2", "3"} {
		d := dumpSrc(t, doc(v, column("c", "w"), column("d", "nw"), column("e", "")), data, 38, 3)
		if errs := errorsOf(d); len(errs) > 0 {
			t.Fatalf("version=%s: errors %v", v, errs)
		}
		check(t, d, end, clip, end)
	}
}

// 94. A text cut by truncate-start or truncate-middle says so with its
// ellipsis, so neither mode reports L008 in a version="3" document; the
// same text left nowrap by default does.
func TestTruncateModesNoL008(t *testing.T) {
	for _, c := range []struct {
		wrap, want string
		l008       bool
	}{
		{"truncate-start", "…efgh", false},
		{"truncate-middle", "ab…gh", false},
		{"", "abcde", true},
	} {
		attr := ""
		if c.wrap != "" {
			attr = ` wrap="` + c.wrap + `"`
		}
		d := dumpSrc(t, `<tui version="3"><screen id="main"><text id="x" width="5"`+attr+`>abcdefgh</text></screen></tui>`, "", 10, 1)
		if !strings.HasPrefix(d.Grid[0], c.want) {
			t.Errorf("wrap=%q: grid %q, want it to start with %q", c.wrap, d.Grid[0], c.want)
		}
		if got := len(diagsWithCode(d, "L008")) > 0; got != c.l008 {
			t.Errorf("wrap=%q: L008 %v, want %v (%v)", c.wrap, got, c.l008, d.Errors)
		}
	}
	// The same through a stylesheet, on several lines each cut on its own.
	d := dumpSrc(t, `<tui version="3"><style>#x { wrap: truncate-middle; width: 5; height: 2; }</style><screen id="main"><text id="x">abcdefgh
12345678</text></screen></tui>`, "", 10, 2)
	if !strings.HasPrefix(d.Grid[0], "ab…gh ") || !strings.HasPrefix(d.Grid[1], "12…78 ") || len(diagsWithCode(d, "L008")) != 0 {
		t.Errorf("two lines: %q, %v", d.Grid, d.Errors)
	}
}
