package tuimark_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/abdul-hamid-achik/tuimark"
	"github.com/abdul-hamid-achik/tuimark/internal/dump"
)

// SPEC §21 conformance tests. Spike goldens are frozen: never regenerate
// them to make a later phase pass — add a new fixture instead.

const spike = "examples/spike/inbox.tui"

func load(t *testing.T, path string) *tuimark.App {
	t.Helper()
	app, err := tuimark.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return app
}

func dumpOf(t *testing.T, app *tuimark.App, cols, rows int) *tuimark.Dump {
	t.Helper()
	d, err := app.Dump(cols, rows)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func node(t *testing.T, d *tuimark.Dump, id string) tuimark.DumpNode {
	t.Helper()
	for _, n := range d.Nodes {
		if n.ID == id {
			return n
		}
	}
	t.Fatalf("node %q not in dump", id)
	return tuimark.DumpNode{}
}

func geom(n tuimark.DumpNode) string {
	return fmt.Sprintf("%dx%d@(%d,%d)", n.W, n.H, n.X, n.Y)
}

// 1. Parse examples/spike/inbox.tui → no diagnostics.
func TestSpikeParsesClean(t *testing.T) {
	app := load(t, spike)
	if diags := app.Validate(); len(diags) != 0 {
		t.Fatalf("want no diagnostics, got %v", diags)
	}
}

// 2 + 3. Required geometry at 80, 120, and 40 columns (SPEC §16.2).
func TestSpikeGeometry(t *testing.T) {
	cases := []struct {
		cols                  int
		inbox, detail, status string
	}{
		{80, "24x23@(0,0)", "56x23@(24,0)", "80x1@(0,23)"},
		{120, "36x23@(0,0)", "84x23@(36,0)", "120x1@(0,23)"},
		{40, "12x23@(0,0)", "28x23@(12,0)", "40x1@(0,23)"},
	}
	app := load(t, spike)
	for _, c := range cases {
		t.Run(fmt.Sprint(c.cols), func(t *testing.T) {
			d := dumpOf(t, app, c.cols, 24)
			if !d.OK {
				t.Fatalf("dump not ok: %v", d.Errors)
			}
			for id, want := range map[string]string{"inbox": c.inbox, "detail": c.detail, "status": c.status} {
				if got := geom(node(t, d, id)); got != want {
					t.Errorf("%s at %d cols: got %s, want %s", id, c.cols, got, want)
				}
			}
		})
	}
}

// §16.2: go test MUST assert the three node lines of the 80×24 text dump.
func TestSpikeTextNodeLines(t *testing.T) {
	d := dumpOf(t, load(t, spike), 80, 24)
	text := dump.Text(d)
	for _, want := range [][]string{
		{"inbox", "box", "24x23", "@(0,0)"},
		{"detail", "box", "56x23", "@(24,0)"},
		{"status", "box", "80x1", "@(0,23)"},
	} {
		found := false
		for _, line := range strings.Split(text, "\n") {
			if strings.Join(strings.Fields(line), " ") == strings.Join(want, " ") {
				found = true
			}
		}
		if !found {
			t.Errorf("text dump lacks node line %q:\n%s", strings.Join(want, " "), text)
		}
	}
}

// 4. grid[y] length equals cols for every row; no ANSI.
func TestGridShape(t *testing.T) {
	app := load(t, spike)
	for _, size := range [][2]int{{40, 24}, {80, 24}, {120, 24}, {7, 3}, {1, 1}, {200, 60}} {
		d := dumpOf(t, app, size[0], size[1])
		if len(d.Grid) != size[1] {
			t.Fatalf("%v: %d grid rows", size, len(d.Grid))
		}
		for y, row := range d.Grid {
			if n := utf8.RuneCountInString(row); n != size[0] {
				t.Errorf("%v: row %d has %d runes", size, y, n)
			}
			if strings.ContainsRune(row, 0x1b) {
				t.Errorf("%v: row %d contains ESC", size, y)
			}
		}
	}
}

// Frozen goldens (SPEC §16): text at 40/80/120 and JSON at 80.
func TestSpikeGoldens(t *testing.T) {
	app := load(t, spike)
	for _, cols := range []int{40, 80, 120} {
		want, err := os.ReadFile(filepath.Join("testdata/golden/spike", fmt.Sprintf("%dx24.txt", cols)))
		if err != nil {
			t.Fatal(err)
		}
		if got := dump.Text(dumpOf(t, app, cols, 24)); got != string(want) {
			t.Errorf("%dx24 text golden mismatch:\n--- got\n%s\n--- want\n%s", cols, got, want)
		}
	}
	want, err := os.ReadFile("testdata/golden/spike/80x24.json")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := dump.JSON(dumpOf(t, app, 80, 24))
	if !bytes.Equal(got, want) {
		t.Errorf("80x24 JSON golden mismatch:\n%s", got)
	}
	// §16.3: the first two rows.
	d := dumpOf(t, app, 80, 24)
	if d.Grid[0] != "┌"+strings.Repeat("─", 22)+"┐┌"+strings.Repeat("─", 54)+"┐" {
		t.Errorf("row 0: %q", d.Grid[0])
	}
	if !strings.HasPrefix(d.Grid[1], "│Inbox") || !strings.Contains(d.Grid[1], "││Detail") {
		t.Errorf("row 1: %q", d.Grid[1])
	}
}

func codes(diags []tuimark.Diagnostic) []string {
	var out []string
	for _, d := range diags {
		out = append(out, d.Code)
	}
	return out
}

func hasCode(diags []tuimark.Diagnostic, code string) bool {
	for _, d := range diags {
		if d.Code == code {
			return true
		}
	}
	return false
}

func parseDoc(t *testing.T, src string) *tuimark.App {
	t.Helper()
	app, err := tuimark.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	return app
}

// 5–9. Error codes (exit code 2 is covered in cmd/tuimark tests).
func TestDiagnosticCodes(t *testing.T) {
	cases := []struct {
		name, code, src string
	}{
		{"unknown tag", "V001", `<app><widget/></app>`},
		{"duplicate id", "V004", `<app><box id="a"/><box id="a"/></app>`},
		{"bad unit", "V003", `<app><box width="foo"/></app>`},
		{"unclosed", "V005", `<app><box></app>`},
		{"text with element", "V013", `<app><text><box/></text></app>`},
		{"unknown attr", "V002", `<app><box title="x"/></app>`},
		{"px unit", "V003", `<app><box width="10px"/></app>`},
		{"single quotes", "V005", `<app><box id='a'/></app>`},
		{"doctype", "V005", `<!DOCTYPE app><app/>`},
		{"entity", "V005", `<app><text>&nbsp;</text></app>`},
		{"uppercase", "V001", `<app><Box/></app>`},
		{"missing version", "V014", `<tui><screen/></tui>`},
		{"ansi", "V007", "<app><text>\x1b[31mred</text></app>"},
		{"no id on input", "V012", `<tui version="1"><screen><input/></screen></tui>`},
		{"each bad", "V011", `<tui version="1"><screen><list id="l" each="tickets"><item/></list></screen></tui>`},
		{"modal not last", "L004", `<tui version="1"><screen><modal id="m" open="true"/><box/></screen></tui>`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			diags := parseDoc(t, c.src).Validate()
			if !hasCode(diags, c.code) {
				t.Fatalf("want %s, got %v", c.code, diags)
			}
			d, _ := parseDoc(t, c.src).Dump(80, 24)
			if d.OK {
				t.Fatalf("dump should not be ok")
			}
		})
	}
}

// The JSON dump shape matches SPEC §13.2.
func TestDumpJSONShape(t *testing.T) {
	d := dumpOf(t, load(t, spike), 80, 24)
	b, _ := json.Marshal(d)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"cols", "rows", "ok", "focus", "errors", "nodes", "grid"} {
		if _, ok := m[k]; !ok {
			t.Errorf("dump JSON lacks %q", k)
		}
	}
	if m["focus"] != nil {
		t.Errorf("spike focus should be null")
	}
}

// Finding 27: the public Dump rejects negative sizes instead of returning a
// dump whose cols/rows contradict its grid; 0 is an empty, consistent grid.
func TestDumpSizeContract(t *testing.T) {
	app := parseDoc(t, `<tui version="1"><screen id="s"><text>hi</text></screen></tui>`)
	for _, sz := range [][2]int{{-5, 3}, {5, -3}, {-1, -1}} {
		if d, err := app.Dump(sz[0], sz[1]); err == nil {
			t.Errorf("Dump(%d, %d) = cols %d rows %d grid %d, want an error", sz[0], sz[1], d.Cols, d.Rows, len(d.Grid))
		}
	}
	for _, sz := range [][2]int{{0, 2}, {3, 0}, {0, 0}, {5, 3}} {
		d := dumpOf(t, app, sz[0], sz[1])
		if d.Cols != sz[0] || d.Rows != sz[1] || len(d.Grid) != d.Rows {
			t.Errorf("Dump(%d, %d): cols %d rows %d grid %d", sz[0], sz[1], d.Cols, d.Rows, len(d.Grid))
		}
		for _, line := range d.Grid {
			if utf8.RuneCountInString(line) != d.Cols {
				t.Errorf("Dump(%d, %d): %q", sz[0], sz[1], line)
			}
		}
	}
}

// Findings 16/31: a huge size is clipped in paint (SPEC §11.4), so dump and
// validate cost what the visible grid costs and still finish promptly.
func TestHugeSizesDumpPromptly(t *testing.T) {
	docs := []string{
		`<app><box width="99999999999" height="3" border="1"/></app>`,
		`<app><box id="a" width="10" height="99999999999"/></app>`,
		`<app><row><box id="a" width="1000000000"/></row></app>`,
		`<tui version="1"><style>#a { min-width: 999999999999999; background: red; }</style><screen><row><box id="a"/><box/></row></screen></tui>`,
	}
	done := make(chan string, 1)
	go func() {
		for _, src := range docs {
			app, err := tuimark.Parse(strings.NewReader(src))
			if err != nil {
				done <- err.Error()
				return
			}
			d, err := app.Dump(20, 3)
			if err != nil || len(d.Grid) != 3 || utf8.RuneCountInString(d.Grid[0]) != 20 {
				done <- fmt.Sprintf("%s: %v %+v", src, err, d)
				return
			}
			app.Validate()
		}
		done <- ""
	}()
	select {
	case msg := <-done:
		if msg != "" {
			t.Error(msg)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("dump/validate of a huge box did not finish: paint walks the logical rect")
	}
}

// SPEC v0.2 §21 tests 22–25: auto-fit of unsized viewports (§11.4, ADR
// 0007) applies to version="1" documents, and L006 reports viewports that
// can never show part of their content.

func l006IDs(diags []tuimark.Diagnostic) []string {
	var out []string
	for _, d := range diags {
		if d.Code == "L006" {
			out = append(out, d.ID)
		}
	}
	return out
}

// 22. A col holding an unsized 2000-row list and a one-line text at 80×24:
// the list gets the rows left above the text, the text stays on the last
// row, and the list follows its selection. v0.1 gave the list 80x2000 and
// pushed the status line off-screen.
func TestAutoFitLongList(t *testing.T) {
	app := parseDoc(t, `<tui version="1"><screen id="s"><col>
  <list id="l" each="rows as r" key="r" bind="sel"><item><text>{r}</text></item></list>
  <text id="status">status line</text>
</col></screen></tui>`)
	rows := make([]string, 2000)
	for i := range rows {
		rows[i] = fmt.Sprintf("r%04d", i)
	}
	if err := app.Bind("rows", rows); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		sel, first, last string // selection, top visible row, row above the status
	}{
		{"r0000", "r0000", "r0022"},
		{"r1000", "r0978", "r1000"},
		{"r1999", "r1977", "r1999"},
	} {
		if err := app.Bind("sel", c.sel); err != nil {
			t.Fatal(err)
		}
		d := dumpOf(t, app, 80, 24)
		if !d.OK || len(d.Errors) != 0 {
			t.Fatalf("%s: %v", c.sel, d.Errors)
		}
		if got := geom(node(t, d, "l")); got != "80x23@(0,0)" {
			t.Errorf("%s: list %s", c.sel, got)
		}
		if got := geom(node(t, d, "status")); got != "80x1@(0,23)" {
			t.Errorf("%s: status %s", c.sel, got)
		}
		if !strings.HasPrefix(d.Grid[0], c.first) || !strings.HasPrefix(d.Grid[22], c.last) || !strings.HasPrefix(d.Grid[23], "status line") {
			t.Errorf("%s: grid rows 0, 22, 23 = %q %q %q", c.sel, d.Grid[0], d.Grid[22], d.Grid[23])
		}
		items := 0
		for _, n := range d.Nodes {
			if n.Tag == "item" {
				items++
				if n.Selected && n.Key != c.sel {
					t.Errorf("%s: selected item %q", c.sel, n.Key)
				}
			}
		}
		if items != 2000 {
			t.Errorf("%s: %d item nodes, want every item laid out", c.sel, items)
		}
	}
	if diags := app.Validate(); len(diags) != 0 {
		t.Errorf("validate: %v", diags)
	}
}

// 23. Auto-fit is a no-op when the content fits and in unbounded
// allocations.
func TestAutoFitNoOp(t *testing.T) {
	app := parseDoc(t, `<tui version="1"><screen id="s"><col>
  <list id="l" each="rows as r" key="r"><item><text>{r}</text></item></list>
  <text id="after">after</text>
</col></screen></tui>`)
	_ = app.Bind("rows", []string{"a", "b", "c"})
	d := dumpOf(t, app, 80, 24)
	if geom(node(t, d, "l")) != "80x3@(0,0)" || geom(node(t, d, "after")) != "80x1@(0,3)" || len(d.Errors) != 0 {
		t.Errorf("fits: list %s after %s %v", geom(node(t, d, "l")), geom(node(t, d, "after")), d.Errors)
	}
	app = parseDoc(t, `<tui version="1"><screen id="s"><scroll id="outer" style="height: 5">
  <list id="l" each="rows as r" key="r"><item><text>{r}</text></item></list>
</scroll></screen></tui>`)
	rows := make([]string, 40)
	for i := range rows {
		rows[i] = fmt.Sprint(i)
	}
	_ = app.Bind("rows", rows)
	d = dumpOf(t, app, 80, 24)
	if geom(node(t, d, "l")) != "80x40@(0,0)" || len(d.Errors) != 0 {
		t.Errorf("inside a scroll: list %s %v", geom(node(t, d, "l")), d.Errors)
	}
	if diags := app.Validate(); len(diags) != 0 {
		t.Errorf("validate: %v", diags)
	}
}

// 24. L006: two stacked unsized lists that overflow (the second gets 0
// rows), a sized list clipped by a box that does not scroll, and not the
// same list inside a <scroll>.
func TestL006(t *testing.T) {
	rows := make([]string, 40)
	for i := range rows {
		rows[i] = fmt.Sprint(i)
	}
	cases := []struct {
		name, body, want string
	}{
		{"stacked lists", `<col>
  <list id="a" each="rows as r" key="r"><item><text>{r}</text></item></list>
  <list id="b" each="rows as r" key="r"><item><text>{r}</text></item></list>
</col>`, "b"},
		{"clipped by a box", `<box style="height: 5">
  <list id="l" style="height: 10" each="rows as r" key="r"><item><text>{r}</text></item></list>
</box>`, "l"},
		{"inside a scroll", `<scroll style="height: 5">
  <list id="l" style="height: 10" each="rows as r" key="r"><item><text>{r}</text></item></list>
</scroll>`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			app := parseDoc(t, `<tui version="1"><screen id="s">`+c.body+`</screen></tui>`)
			_ = app.Bind("rows", rows)
			d := dumpOf(t, app, 80, 24)
			if got := strings.Join(l006IDs(d.Errors), " "); got != c.want {
				t.Errorf("dump L006 on %q, want %q: %v", got, c.want, d.Errors)
			}
			if !d.OK {
				t.Errorf("L006 is a warning: %v", d.Errors)
			}
			for _, e := range d.Errors {
				if e.Code == "L006" && e.Severity != "warning" {
					t.Errorf("severity %q", e.Severity)
				}
			}
			if got := strings.Join(l006IDs(app.Validate()), " "); got != c.want {
				t.Errorf("validate L006 on %q, want %q", got, c.want)
			}
		})
	}
	app := parseDoc(t, `<tui version="1"><screen id="s">`+cases[0].body+`</screen></tui>`)
	_ = app.Bind("rows", rows)
	d := dumpOf(t, app, 80, 24)
	if geom(node(t, d, "a")) != "80x24@(0,0)" || geom(node(t, d, "b")) != "80x0@(0,24)" {
		t.Errorf("document order: a %s b %s", geom(node(t, d, "a")), geom(node(t, d, "b")))
	}
}

// 25. The examples with sample data: the viewport geometry at 40, 80, and
// 120 columns is the v0.1 one (every scrollable there is fr or fits), and
// no L006 is reported. The before/after comparison (dashboard and agent
// identical to v0.1 in nodes and grid) is recorded in ADR 0007, section
// "Comparación (resultado)".
func TestAutoFitExamplesUnchanged(t *testing.T) {
	bind := func(t *testing.T, app *tuimark.App, path string) {
		t.Helper()
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var data map[string]any
		if err := json.Unmarshal(raw, &data); err != nil {
			t.Fatal(err)
		}
		if err := app.Bind("", data); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct {
		file, data string
		cols, rows int
		want       map[string]string
	}{
		{"examples/dashboard/app.tui", "examples/dashboard/sample.json", 40, 24, map[string]string{"steps": "20x16@(1,2)", "logScroll": "15x18@(24,4)"}},
		{"examples/dashboard/app.tui", "examples/dashboard/sample.json", 80, 24, map[string]string{"steps": "20x16@(1,2)", "logScroll": "55x18@(24,4)"}},
		{"examples/dashboard/app.tui", "examples/dashboard/sample.json", 120, 30, map[string]string{"steps": "30x22@(1,2)", "logScroll": "85x24@(34,4)"}},
		{"examples/agent/agent.tui", "examples/agent/sample.json", 40, 24, map[string]string{"transcript": "40x20@(0,0)"}},
		{"examples/agent/agent.tui", "examples/agent/sample.json", 80, 24, map[string]string{"transcript": "80x19@(0,1)"}},
		{"examples/agent/agent.tui", "examples/agent/sample.json", 120, 30, map[string]string{"transcript": "89x25@(0,1)", "activity": "28x2@(91,2)", "files": "28x1@(91,16)"}},
	} {
		app := load(t, c.file)
		bind(t, app, c.data)
		d := dumpOf(t, app, c.cols, c.rows)
		if !d.OK || len(d.Errors) != 0 {
			t.Errorf("%s %dx%d: %v", c.file, c.cols, c.rows, d.Errors)
		}
		for id, want := range c.want {
			if got := geom(node(t, d, id)); got != want {
				t.Errorf("%s %dx%d: #%s = %s, want %s", c.file, c.cols, c.rows, id, got, want)
			}
		}
	}
}

// scrollOf renders a node's §13.2 scroll member as JSON ("" when absent),
// so the axis pairs and their order are part of what is compared.
func scrollOf(t *testing.T, n tuimark.DumpNode) string {
	t.Helper()
	if n.Scroll == nil {
		return ""
	}
	b, err := json.Marshal(n.Scroll)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// 22 (continued). The list dumps `scroll` with the offset it used and
// h = 2000 plus any gaps (SPEC v0.2 §13.2, §21 test 22).
func TestAutoFitLongListDumpsScroll(t *testing.T) {
	rows := make([]string, 2000)
	for i := range rows {
		rows[i] = fmt.Sprintf("r%04d", i)
	}
	for _, c := range []struct {
		gap  string
		sel  string
		want string
	}{
		{"", "r0000", `{"y":0,"h":2000}`},
		{"", "r1000", `{"y":978,"h":2000}`},
		{"", "r1999", `{"y":1977,"h":2000}`},
		{"gap: 1", "r0000", `{"y":0,"h":3999}`},
		{"gap: 1", "r1999", `{"y":3976,"h":3999}`},
	} {
		app := parseDoc(t, `<tui version="1"><screen id="s"><col>
  <list id="l" style="`+c.gap+`" each="rows as r" key="r" bind="sel"><item><text>{r}</text></item></list>
  <text id="status">status line</text>
</col></screen></tui>`)
		if err := app.Bind("rows", rows); err != nil {
			t.Fatal(err)
		}
		if err := app.Bind("sel", c.sel); err != nil {
			t.Fatal(err)
		}
		d := dumpOf(t, app, 80, 24)
		if got := scrollOf(t, node(t, d, "l")); got != c.want {
			t.Errorf("%q %s: scroll %s, want %s", c.gap, c.sel, got, c.want)
		}
		if got := scrollOf(t, node(t, d, "status")); got != "" {
			t.Errorf("a text has no scroll: %s", got)
		}
	}
}

// SPEC v0.2 §11.5.2/§12.1: a secret input is sized by what it paints (one
// • per cluster), so a CJK secret and an ASCII secret with the same number
// of clusters give the same geometry and the same grid.
func TestSecretInputGeometryHidesWidth(t *testing.T) {
	const doc = `<tui version="1"><screen id="m"><row><input id="s" secret="true" bind="s"/><text id="bar">|</text></row></screen></tui>`
	var ref *tuimark.Dump
	for _, v := range []string{"abcd", "微信微信", "👨‍👩‍👧xyz", "​​​​"} {
		app := parseDoc(t, doc)
		if err := app.Bind("s", v); err != nil {
			t.Fatal(err)
		}
		d := dumpOf(t, app, 12, 1)
		if !d.OK {
			t.Fatalf("%q: %v", v, d.Errors)
		}
		in := node(t, d, "s")
		if in.W != 5 || in.Text != "••••" || d.Grid[0] != "•••• |      " {
			t.Errorf("%q: input %s text %q grid %q, want 5 wide, 4 bullets painted", v, geom(in), in.Text, d.Grid[0])
		}
		if ref == nil {
			ref = d
			continue
		}
		if geom(node(t, d, "bar")) != geom(node(t, ref, "bar")) || strings.Join(d.Grid, "\n") != strings.Join(ref.Grid, "\n") {
			t.Errorf("%q: geometry or grid differs from an ASCII secret of the same length", v)
		}
	}
}

// SPEC v0.2 §11.3/§14 L001: an fr weight the document wrote along one of a
// viewport's own scroll axes; built-in sizes (col/row 1fr, spacer flex: 1)
// are auto there without a diagnostic, and every other container is a
// flex parent.
func TestL001ViewportRule(t *testing.T) {
	for _, c := range []struct {
		name, body string
		l001       bool
	}{
		{"scroll > col", `<col><scroll id="sc"><col id="in"><text>a</text><text>b</text></col></scroll><text>status</text></col>`, false},
		{"scroll > row", `<col><scroll id="sc"><row id="in"><text>a</text></row></scroll><text>status</text></col>`, false},
		{"scroll > spacer", `<scroll style="height: 3"><text>a</text><spacer/><text>b</text></scroll>`, false},
		{"x scroll > col", `<col><scroll axis="x"><col id="in"><text>a</text></col></scroll></col>`, false},
		{"list > item > row", `<list id="l" each="rows as r" key="r"><item><row><text>{r}</text></row></item></list>`, false},
		{"list > item > col", `<list id="l" each="rows as r" key="r"><item><col><text>{r}</text></col></item></list>`, false},
		{"overflow: scroll > col", `<box style="overflow: scroll; height: 5"><col id="in"><text>a</text></col></box>`, false},
		{"overflow: scroll > col with height: 1fr", `<box style="overflow: scroll; height: 5"><col id="in" style="height: 1fr"><text>a</text></col></box>`, true},
		{"scroll > box with flex: 1", `<scroll style="height: 5"><box id="in" style="flex: 1"><text>a</text></box></scroll>`, true},
		{"box > col with height: 1fr", `<box style="height: 5"><col id="in" style="height: 1fr"><text>a</text></col></box>`, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			app := parseDoc(t, `<tui version="1"><screen id="s">`+c.body+`</screen></tui>`)
			_ = app.Bind("rows", []string{"a", "b"})
			if got := hasCode(app.Validate(), "L001"); got != c.l001 {
				t.Errorf("L001 = %v, want %v: %v", got, c.l001, app.Validate())
			}
			d := dumpOf(t, app, 20, 6)
			if d.OK == c.l001 {
				t.Errorf("dump ok = %v: %v", d.OK, d.Errors)
			}
		})
	}
	// <scroll><col> lays the col out at its content height (auto on the
	// scroll axis), and the unsized scroll auto-fits above the status.
	app := parseDoc(t, `<tui version="1"><screen id="s"><col><scroll id="sc"><col id="in"><text>a</text><text>b</text></col></scroll><text id="st">status</text></col></screen></tui>`)
	d := dumpOf(t, app, 20, 6)
	if geom(node(t, d, "in")) != "20x2@(0,0)" || geom(node(t, d, "sc")) != "20x2@(0,0)" || geom(node(t, d, "st")) != "20x1@(0,2)" {
		t.Errorf("in %s sc %s st %s", geom(node(t, d, "in")), geom(node(t, d, "sc")), geom(node(t, d, "st")))
	}
}

// SPEC v0.2 §11.4 shrinkable: a viewport is shrinkable on an axis it does
// not scroll when a child is, so a long list inside a <scroll axis="x">
// fits the rows left and the status line stays on the last row.
func TestAutoFitThroughCrossAxisViewport(t *testing.T) {
	app := parseDoc(t, `<tui version="1"><screen id="s"><col id="c"><scroll id="sc" axis="x"><list id="l" each="rows as r" key="r"><item><text>{r}</text></item></list></scroll><text id="t2">status</text></col></screen></tui>`)
	rows := make([]string, 2000)
	for i := range rows {
		rows[i] = fmt.Sprint(i)
	}
	_ = app.Bind("rows", rows)
	d := dumpOf(t, app, 80, 24)
	if len(d.Errors) != 0 {
		t.Fatalf("%v", d.Errors)
	}
	for id, want := range map[string]string{"sc": "80x23@(0,0)", "l": "80x23@(0,0)", "t2": "80x1@(0,23)"} {
		if got := geom(node(t, d, id)); got != want {
			t.Errorf("#%s = %s, want %s", id, got, want)
		}
	}
	if got := scrollOf(t, node(t, d, "sc")); got != `{"x":0,"w":80}` {
		t.Errorf("sc scroll %s", got)
	}
	if got := scrollOf(t, node(t, d, "l")); got != `{"y":0,"h":2000}` {
		t.Errorf("l scroll %s", got)
	}
}

// SPEC v0.2 §11.4: axis= is allowed only on scroll and rule; a container
// with overflow: scroll scrolls on y, also under layout: row.
func TestOverflowScrollAxisIsY(t *testing.T) {
	x := strings.Repeat("x", 200)
	app := parseDoc(t, `<tui version="1"><screen id="s"><col><box id="ov" axis="x" style="overflow: scroll"><text>`+x+`</text></box></col></screen></tui>`)
	if !hasCode(app.Validate(), "V002") {
		t.Errorf("axis on a box: %v", app.Validate())
	}
	app = parseDoc(t, `<tui version="1"><screen id="s"><row><box id="ov" style="overflow: scroll; layout: row; height: 3"><text>`+x+`</text></box><text id="right">right</text></row></screen></tui>`)
	d := dumpOf(t, app, 80, 24)
	if got := scrollOf(t, node(t, d, "ov")); got != `{"y":0,"h":3}` {
		t.Errorf("overflow: scroll box scroll %s, want y only", got)
	}
}
