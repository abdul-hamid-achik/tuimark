package host

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/layout"
)

// SPEC v0.2b §6.9 (table and column) through the whole frame: rows
// generated after layout (§18 step 7), the cursor and the offset (§6.9.2,
// §6.9.3), cells and styling (§6.9.4), diagnostics (§14), the key
// dispatch and the built-in actions on a table (§8.4, §8.6), and the
// measure cache. §21 tests 50–53, 60 (table thumb), 62–64 for tables.

// rowsJSON is n rows {"id": i, "n": "row i"}.
func rowsJSON(n int) string {
	var b strings.Builder
	b.WriteString("[")
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"id":%d,"n":"row %d"}`, i, i)
	}
	b.WriteString("]")
	return b.String()
}

const bigTable = `<tui version="2"><screen id="s" focus="#t">
<table id="t" each="rows as r" key="r.id" bind="cur" on:select="sel">
  <column title="ID" width="6">{r.id}</column>
  <column title="Name">{r.n}</column>
</table>
<text id="status">status</text>
</screen></tui>`

// tableRowsOf returns the generated rows of table id in f, as "key" or
// "key*" for the cursor row, and the number of body cells.
func tableRowsOf(f *Frame, id string) (rows []string, cells int) {
	for _, b := range f.ByID[id].Children {
		if b.Kind != "item" {
			continue
		}
		k := b.Key
		if b.Selected {
			k += "*"
		}
		rows = append(rows, k)
		cells += len(b.Children)
	}
	return rows, cells
}

func tableScroll(t *testing.T, a *App, cols, rows int) (o, h int) {
	t.Helper()
	for _, n := range a.Dump(cols, rows, false).Nodes {
		if n.ID == "t" {
			return *n.Scroll.Y, *n.Scroll.H
		}
	}
	t.Fatal("no table node")
	return 0, 0
}

// 51. 2000 rows at 80×24: only the visible rows and their cells are laid
// out and dumped; scroll.h is 2000; down, pgdn, end, and home move the
// cursor with P = the body viewport and the offset follows; the offset
// also follows a Set of the bound path, a re-sort of the array, and a
// resize; on:select fires once per change with {alias: key}; the bound
// key round-trips as a number.
func TestTable2000Rows(t *testing.T) {
	a := doc(t, bigTable)
	bindJSON(t, a, `{"cur":null,"rows":`+rowsJSON(2000)+`}`)
	f := a.Frame(80, 24)
	tb := f.ByID["t"]
	if tb.H != 23 || tb.View != 22 || f.ByID["status"].Y != 23 {
		t.Fatalf("table h %d view %d, status at %d", tb.H, tb.View, f.ByID["status"].Y)
	}
	rows, cells := tableRowsOf(f, "t")
	if len(rows) != 22 || cells != 44 || rows[0] != "0*" || rows[21] != "21" {
		t.Fatalf("rows %v (%d cells)", rows, cells)
	}
	d := a.Dump(80, 24, false)
	items := 0
	for _, n := range d.Nodes {
		if n.Tag == "item" {
			items++
		}
	}
	if o, h := tableScroll(t, a, 80, 24); items != 22 || o != 0 || h != 2000 {
		t.Errorf("dump: %d items, scroll %d/%d", items, o, h)
	}
	step := func(k Key, wantCursor string, wantOffset int, wantEvents string) {
		t.Helper()
		got := evText(t, keysAt(a, 80, 24, k))
		if got != wantEvents {
			t.Errorf("%s: events %s, want %s", k.Name, got, wantEvents)
		}
		f := a.Frame(80, 24)
		rows, _ := tableRowsOf(f, "t")
		sel := ""
		for _, r := range rows {
			if strings.HasSuffix(r, "*") {
				sel = r
			}
		}
		if sel != wantCursor || f.ByID["t"].ScrollY != wantOffset {
			t.Errorf("%s: cursor %q offset %d, want %q %d", k.Name, sel, f.ByID["t"].ScrollY, wantCursor, wantOffset)
		}
	}
	sel := func(k int) string {
		return fmt.Sprintf(`{"action":"sel","source":"t","keys":{"r":%d},"value":null}`, k)
	}
	step(named("down"), "1*", 0, sel(1))
	step(named("pgdn"), "23*", 2, sel(23))
	step(named("end"), "1999*", 1978, sel(1999))
	step(named("down"), "1999*", 1978, "")
	step(named("pgup"), "1977*", 1977, sel(1977))
	step(named("home"), "0*", 0, sel(0))
	step(named("up"), "0*", 0, "")
	if s := storeJSON(t, a, "cur"); s != "0" {
		t.Errorf("bind round-trips the numeric key: cur = %s", s)
	}
	// A Set of the bound path moves the cursor without on:select; the
	// offset brings the cursor row into view.
	if err := a.Set("cur", 500); err != nil {
		t.Fatal(err)
	}
	if evs := keysAt(a, 80, 24); len(evs) != 0 || len(a.TakePending()) != 0 {
		t.Errorf("a Set fired %v", evs)
	}
	if o, _ := tableScroll(t, a, 80, 24); o != 479 {
		t.Errorf("after Set: offset %d, want 479", o)
	}
	// A re-sort: the bound key is now at index 1499.
	var rev []any
	_ = json.Unmarshal([]byte(rowsJSON(2000)), &rev)
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	_ = a.Set("rows", rev)
	f = a.Frame(80, 24)
	if f.ByID["t"].ScrollY != 1478 {
		t.Errorf("after a re-sort: offset %d, want 1478", f.ByID["t"].ScrollY)
	}
	// A resize: V = 10.
	f = a.Frame(80, 12)
	if rows, _ := tableRowsOf(f, "t"); f.ByID["t"].ScrollY != 1490 || rows[9] != "500*" {
		t.Errorf("after a resize: offset %d rows %v", f.ByID["t"].ScrollY, rows)
	}
}

// 51. The cursor by key: "7" does not select the row keyed 7 (the cursor
// keeps its index); with two rows keyed 7, 7 selects the first; null and a
// missing path (B003) keep the index; B006 without key= (parse).
func TestTableCursorByKey(t *testing.T) {
	a := doc(t, bigTable)
	bindJSON(t, a, `{"cur":2,"rows":[{"id":1},{"id":7},{"id":2},{"id":7}]}`)
	cursor := func() string {
		rows, _ := tableRowsOf(a.Frame(20, 8), "t")
		return strings.Join(rows, " ")
	}
	if got := cursor(); got != "1 7 2* 7" {
		t.Errorf("bound 2: %s", got)
	}
	_ = a.Set("cur", "7")
	if got := cursor(); got != "1 7 2* 7" {
		t.Errorf(`bound "7" moved the cursor: %s`, got)
	}
	_ = a.Set("cur", 7)
	if got := cursor(); got != "1 7* 2 7" {
		t.Errorf("bound 7: %s", got)
	}
	_ = a.Set("cur", nil)
	if got := cursor(); got != "1 7* 2 7" {
		t.Errorf("bound null: %s", got)
	}
	bindJSON(t, a, `{"rows":[{"id":1},{"id":7},{"id":2},{"id":7}]}`)
	f := a.Frame(20, 8)
	if !hasDiag(f, "B003", `"cur"`) {
		t.Errorf("no B003 for a missing bind path: %v", f.Diags)
	}
	if rows, _ := tableRowsOf(f, "t"); strings.Join(rows, " ") != "1 7* 2 7" {
		t.Errorf("missing bind path: %v", rows)
	}
	noKey := doc(t, `<tui version="2"><screen id="s"><table id="t" each="rows as r"><column>{r}</column></table></screen></tui>`)
	found := false
	for _, d := range noKey.Validate() {
		found = found || d.Code == "B006"
	}
	if !found {
		t.Error("no B006 for a table without key=")
	}
	bindJSON(t, noKey, `{"rows":["a","b"]}`)
	if rows, _ := tableRowsOf(noKey.Frame(10, 3), "t"); strings.Join(rows, " ") != "0* 1" {
		t.Errorf("keys without key= are the indices: %v", rows)
	}
}

// 51. A table inside a <scroll> takes its intrinsic height: every row is
// laid out and dumped, o = 0, and the ancestor does not scroll to follow
// the cursor; no L006.
func TestTableInsideScroll(t *testing.T) {
	a := doc(t, `<tui version="2"><screen id="s" focus="#t"><scroll id="sc">
<table id="t" each="rows as r" key="r.id" bind="cur"><column title="N">{r.n}</column></table>
</scroll></screen></tui>`)
	bindJSON(t, a, `{"cur":40,"rows":`+rowsJSON(50)+`}`)
	f := a.Frame(20, 10)
	rows, _ := tableRowsOf(f, "t")
	tb := f.ByID["t"]
	if len(rows) != 50 || tb.H != 51 || tb.ScrollY != 0 || f.ByID["sc"].ScrollY != 0 {
		t.Errorf("%d rows, h %d, offset %d, scroll %d", len(rows), tb.H, tb.ScrollY, f.ByID["sc"].ScrollY)
	}
	if hasDiag(f, "L006", "") {
		t.Errorf("L006 inside a scroll: %v", f.Diags)
	}
}

// 52. An empty array paints the placeholder dim and centered on the first
// body row, resolved in the table's scope, with no node for it; nothing
// when the body viewport has no row.
func TestTablePlaceholder(t *testing.T) {
	src := `<tui version="2"><screen id="s"><table id="t" each="rows as r" key="r" placeholder="no {what}" style="%s"><column title="Name">{r}</column></table></screen></tui>`
	a := doc(t, fmt.Sprintf(src, "height: 3"))
	bindJSON(t, a, `{"what":"results","rows":[]}`)
	f := a.Frame(20, 3)
	lines := f.Grid.Lines()
	if lines[0] != "Name                " || lines[1] != "     no results     " || lines[2] != strings.Repeat(" ", 20) {
		t.Errorf("grid %q", lines)
	}
	for x := 5; x < 15; x++ {
		if f.Grid.At(x, 1).Attrs&0x2 == 0 || f.Grid.At(x, 1).Owner != "t" {
			t.Errorf("placeholder cell %d: attrs %b owner %q", x, f.Grid.At(x, 1).Attrs, f.Grid.At(x, 1).Owner)
		}
	}
	if n := len(f.ByID["t"].Children); n != 1 {
		t.Errorf("the placeholder made a node: %d children", n)
	}
	// Cut to the width with Truncate.
	if f := a.Frame(8, 3); f.Grid.Lines()[1] != "no resu…" {
		t.Errorf("narrow: %q", f.Grid.Lines()[1])
	}
	// No body row: nothing.
	b := doc(t, fmt.Sprintf(src, "height: 1"))
	bindJSON(t, b, `{"what":"x","rows":[]}`)
	if f := b.Frame(20, 3); strings.Contains(strings.Join(f.Grid.Lines(), ""), "no") {
		t.Errorf("placeholder without a body row: %q", f.Grid.Lines())
	}
	// Rows: no placeholder.
	bindJSON(t, a, `{"what":"x","rows":["a"]}`)
	if f := a.Frame(20, 3); strings.Contains(strings.Join(f.Grid.Lines(), ""), "no") {
		t.Errorf("placeholder with rows: %q", f.Grid.Lines())
	}
}

// 53. Diagnostics that read every row are reported once per template or
// guard and path, and the same whatever the scroll offset; L006 for a
// table whose body viewport is 0 rows while it has rows; B001 for a
// missing or non-array each; a key missing on a row is B003 per row.
func TestTableRowDiagnostics(t *testing.T) {
	a := doc(t, `<tui version="2"><screen id="s" focus="#t">
<table id="t" each="rows as r" key="r.id" bind="cur" style="height: 5"><item class:odd="r.odd"/>
  <column title="N" class:hot="r.hot">{r.n}{r.extra}</column>
</table></screen></tui>`)
	var rows []string
	for i := 0; i < 40; i++ {
		extra := `,"extra":"","odd":false,"hot":false`
		if i == 30 {
			extra = ""
		}
		rows = append(rows, fmt.Sprintf(`{"id":%d,"n":"r%d"%s}`, i, i, extra))
	}
	bindJSON(t, a, `{"cur":0,"rows":[`+strings.Join(rows, ",")+`]}`)
	diags := func() string {
		var out []string
		for _, d := range a.Frame(20, 6).Diags {
			out = append(out, d.Code+" "+d.Msg)
		}
		return strings.Join(out, "\n")
	}
	top := diags()
	want := "B002 class:odd=\"r.odd\": path \"r.odd\" is missing\nB002 class:hot=\"r.hot\": path \"r.hot\" is missing\nB003 bind path \"r.extra\" is missing"
	if top != want {
		t.Errorf("at offset 0:\n%s\nwant\n%s", top, want)
	}
	_ = a.Set("cur", 35)
	if got := diags(); got != top {
		t.Errorf("the diagnostics depend on the offset:\n%s", got)
	}
	// Keys: B003 per row.
	bindJSON(t, a, `{"rows":[{"n":"a","extra":"","odd":1,"hot":1},{"n":"b","extra":"","odd":1,"hot":1}]}`)
	got := diags()
	if !strings.Contains(got, `key="r.id" is missing on row 0`) || !strings.Contains(got, `key="r.id" is missing on row 1`) {
		t.Errorf("per-row key B003:\n%s", got)
	}
	// L006: no body row while it has rows.
	l6 := doc(t, `<tui version="2"><screen id="s"><table id="t" each="rows as r" key="r" style="height: 1"><column title="N">{r}</column></table></screen></tui>`)
	bindJSON(t, l6, `{"rows":["a"]}`)
	if f := l6.Frame(10, 4); !hasDiag(f, "L006", "table gets 0 rows") {
		t.Errorf("no L006: %v", f.Diags)
	}
	bindJSON(t, l6, `{"rows":[]}`)
	if f := l6.Frame(10, 4); hasDiag(f, "L006", "") {
		t.Errorf("L006 without rows: %v", f.Diags)
	}
	for data, msg := range map[string]string{`{}`: `path "rows" is missing`, `{"rows":{"a":1}}`: `"rows" is an object, not an array`} {
		bindJSON(t, l6, data)
		f := l6.Frame(10, 4)
		if !hasDiag(f, "B001", msg) || !f.Diags.HasErrors() {
			t.Errorf("%s: %v", data, f.Diags)
		}
		if n := f.ByID["t"].Rows; n != 0 {
			t.Errorf("%s: %d rows", data, n)
		}
	}
}

const styledTable = `<tui version="2"><style>
  .num { content-align: end; }
  #c-cpu { content-align: end; bold: true; border: single; }
  #t > item > text.hot { color: red; }
  #t > item.blocked { color: blue; }
  table > item { border: single; padding: 1; height: 3; margin: 1; background: green; }
  #t > column { color: yellow; }
</style>
<screen id="s" focus="#t">
<table id="t" each="rows as r" key="r.id" gap="1">
  <item class="row" class:blocked="r.blocked" class:row="r.blocked"/>
  <column id="c-name" title="Name">{r.name}</column>
  <column id="c-cpu" title="CPU%" class="num hot" class:hot="r.hot" class:warm="r.hot">{r.cpu}</column>
</table>
</screen></tui>`

// 53, 63. Rules on a column style its header cell only: body cells
// inherit from their row and the table, never from their column, and are
// styled through the column's classes, which they carry, followed by the
// column's guards truthy on their row (without repeats); the header gets
// the static classes only. Rows carry the row template's classes and
// guards. Rows and cells ignore their own borders, padding, margins, and
// sizes.
func TestTableCellStyling(t *testing.T) {
	a := doc(t, styledTable)
	bindJSON(t, a, `{"rows":[{"id":1,"name":"init","cpu":"0.5","hot":false,"blocked":true},{"id":2,"name":"make","cpu":"99","hot":true,"blocked":false}]}`)
	f := a.Frame(20, 4)
	if got := f.Grid.Lines(); got[0] != fmt.Sprintf("%-20s", "Name CPU%") || got[1] != fmt.Sprintf("%-20s", "init  0.5") || got[2] != fmt.Sprintf("%-20s", "make   99") {
		t.Errorf("grid %q", got)
	}
	tb := f.ByID["t"]
	hdr := f.ByID["c-cpu"]
	if strings.Join(hdr.Classes, " ") != "num hot" || !hdr.Style.Bold || hdr.Style.Color.String() != "yellow" {
		t.Errorf("header: classes %v bold %v color %s", hdr.Classes, hdr.Style.Bold, hdr.Style.Color)
	}
	var rows []*layout.Box
	for _, b := range tb.Children {
		if b.Kind == "item" {
			rows = append(rows, b)
		}
	}
	if len(rows) != 2 {
		t.Fatalf("%d rows", len(rows))
	}
	for i, r := range rows {
		if r.X != 0 || r.Y != 1+i || r.W != 20 || r.H != 1 || r.Style.Border != "single" {
			t.Errorf("row %d: %v (border %s)", i, r.Outer(), r.Style.Border)
		}
		cpu := r.Children[1]
		if cpu.Style.Bold || cpu.Style.Color.String() == "yellow" {
			t.Errorf("row %d: a body cell took its column's style", i)
		}
	}
	if got := strings.Join(rows[0].Classes, " "); got != "row blocked" || rows[0].Style.Color.String() != "blue" {
		t.Errorf("row 0 classes %q color %s", got, rows[0].Style.Color)
	}
	if got := strings.Join(rows[1].Classes, " "); got != "row" {
		t.Errorf("row 1 classes %q", got)
	}
	if got := strings.Join(rows[0].Children[1].Classes, " "); got != "num hot" {
		t.Errorf("row 0 cpu classes %q", got)
	}
	if got := strings.Join(rows[1].Children[1].Classes, " "); got != "num hot warm" {
		t.Errorf("row 1 cpu classes %q", got)
	}
	// .hot is a static class of the column: both body cells are red.
	for i, r := range rows {
		if c := r.Children[1].Style.Color.String(); c != "red" {
			t.Errorf("row %d cpu color %s", i, c)
		}
	}
	// No border painted by a row or a header cell; the row background
	// fills its whole width.
	if strings.ContainsAny(strings.Join(f.Grid.Lines(), ""), "┌│└") {
		t.Errorf("a fixed box painted a border: %q", f.Grid.Lines())
	}
	if f.Grid.At(19, 1).BG.String() != "green" {
		t.Errorf("row background at the right edge: %s", f.Grid.At(19, 1).BG)
	}
}

// 50. @media hides a column: its header cell and its body cells go
// together; the other columns take the space.
func TestTableMediaHidesColumn(t *testing.T) {
	a := doc(t, `<tui version="2"><style>@media (max-cols: 15) { #c-b { display: none; } }</style>
<screen id="s"><table id="t" each="rows as r" key="r.a" gap="1">
<column id="c-a" title="A" width="1fr">{r.a}</column><column id="c-b" title="B">{r.b}</column>
</table></screen></tui>`)
	bindJSON(t, a, `{"rows":[{"a":"x","b":"bee"}]}`)
	if got := a.Frame(20, 2).Grid.Lines(); got[0] != "A                B  " || got[1] != "x                bee" {
		t.Errorf("wide %q", got)
	}
	f := a.Frame(15, 2)
	if got := f.Grid.Lines(); got[0] != "A              " || got[1] != "x              " {
		t.Errorf("narrow %q", got)
	}
	if f.ByID["c-b"] != nil && f.ByID["c-b"].Laid {
		t.Error("the hidden column is laid out")
	}
	for _, r := range f.ByID["t"].Children {
		if r.Kind == "item" && len(r.Children) != 1 {
			t.Errorf("row with %d cells", len(r.Children))
		}
	}
}

// 50. An auto column's measure reads every row, visible or not; a table
// without titles has no header; the mark channel shifts the header and
// the cells and paints the mark of checked rows.
func TestTableMeasureHeaderAndMark(t *testing.T) {
	a := doc(t, `<tui version="2"><keymap><bind keys="space" action="check-toggle"/></keymap>
<screen id="s" focus="#t"><table id="t" each="rows as r" key="r.id" checked="m" mark="✓" style="height: 3">
<column>{r.n}</column><column>|</column></table></screen></tui>`)
	bindJSON(t, a, `{"m":[2],"rows":[{"id":1,"n":"a"},{"id":2,"n":"b"},{"id":3,"n":"c"},{"id":4,"n":"a-very-long-name"}]}`)
	f := a.Frame(24, 3)
	if got := f.Grid.Lines(); got[0] != "  a               |     " || got[1] != "✓ b               |     " || got[2] != "  c               |     " {
		t.Errorf("grid %q", got)
	}
	if f.ByID["t"].Header != 0 {
		t.Error("a header without titles")
	}
	keysAt(a, 24, 3, Key{Name: "space", Rune: ' '})
	if s := storeJSON(t, a, "m"); s != "[2,1]" {
		t.Errorf("check-toggle on a table: %s", s)
	}
}

// 62, 64. The built-in actions on a table target: move-* move its cursor
// as a user move (bind, on:select, the offset follows), check-* change its
// checked array (on:change); a table without rows matches no move-* row,
// so the key reaches the next row; a focused table consumes the arrow and
// paging keys only with rows, and never fires on:click from enter.
func TestTableBuiltinActions(t *testing.T) {
	a := doc(t, `<tui version="2"><keymap>
  <bind keys="j" action="move-next" to="#t"/>
  <bind keys="k" action="move-prev" to="#t"/>
  <bind keys="g" action="move-last" to="#t"/>
  <bind keys="d" action="move-page-down" to="#t"/>
  <bind keys="x" action="check-toggle" to="#t"/>
  <bind keys="ctrl+a" action="check-all" to="#t"/>
  <bind keys="ctrl+d" action="check-none" to="#t"/>
  <bind keys="j" action="fallback"/>
  <bind keys="down" action="down_fell"/>
  <bind keys="enter" action="entered"/>
</keymap>
<screen id="s" focus="#b"><button id="b" label="b"/>
<table id="t" each="rows as r" key="r.id" bind="cur" checked="m" on:select="sel" on:change="ch" on:click="clicked" style="height: 4">
<column title="N">{r.id}</column></table></screen></tui>`)
	bindJSON(t, a, `{"cur":null,"m":[],"rows":`+rowsJSON(10)+`}`)
	got := evText(t, keysAt(a, 20, 6, r('j'), r('j'), r('k'), r('d'), r('g'), r('x'), Key{Name: "ctrl+a"}, Key{Name: "ctrl+d"}))
	want := strings.Join([]string{
		`{"action":"sel","source":"t","keys":{"r":1},"value":null}`,
		`{"action":"sel","source":"t","keys":{"r":2},"value":null}`,
		`{"action":"sel","source":"t","keys":{"r":1},"value":null}`,
		`{"action":"sel","source":"t","keys":{"r":4},"value":null}`,
		`{"action":"sel","source":"t","keys":{"r":9},"value":null}`,
		`{"action":"ch","source":"t","keys":{"r":9},"value":[9]}`,
		`{"action":"ch","source":"t","keys":{"r":9},"value":[9,0,1,2,3,4,5,6,7,8]}`,
		`{"action":"ch","source":"t","keys":{"r":9},"value":[]}`,
	}, "\n")
	if got != want {
		t.Errorf("events:\n%s\nwant\n%s", got, want)
	}
	if o := a.Frame(20, 6).ByID["t"].ScrollY; o != 7 {
		t.Errorf("offset %d after move-last (V = 3)", o)
	}
	// move-next on the last row consumes j silently.
	if evs := keysAt(a, 20, 6, r('j')); len(evs) != 0 {
		t.Errorf("move-next on the last row: %s", evText(t, evs))
	}
	// Without rows, the move-* row does not match: j reaches "fallback".
	bindJSON(t, a, `{"cur":null,"m":[],"rows":[]}`)
	if got := evText(t, keysAt(a, 20, 6, r('j'))); !strings.Contains(got, `"fallback"`) {
		t.Errorf("an empty table matched move-next: %s", got)
	}
	// Focused: down is consumed only with rows; enter never fires the
	// table's on:click.
	_ = a.Set("@focus", "#t")
	if got := evText(t, keysAt(a, 20, 6, named("down"), named("enter"))); !strings.Contains(got, "down_fell") || !strings.Contains(got, "entered") || strings.Contains(got, "clicked") {
		t.Errorf("empty focused table: %s", got)
	}
	bindJSON(t, a, `{"cur":null,"m":[],"rows":`+rowsJSON(3)+`}`)
	if got := evText(t, keysAt(a, 20, 6, named("down"), named("enter"))); got != `{"action":"sel","source":"t","keys":{"r":1},"value":null}`+"\n"+`{"action":"entered","source":"t","keys":{"r":1},"value":null}` {
		t.Errorf("focused table: %s", got)
	}
}

// §8.3: a table is focusable by default, in the tab cycle.
func TestTableFocusableByDefault(t *testing.T) {
	a := doc(t, `<tui version="2"><screen id="s"><input id="i"/><table id="t" each="rows as r" key="r"><column>{r}</column></table></screen></tui>`)
	bindJSON(t, a, `{"rows":["a"]}`)
	a.Frame(10, 4)
	a.HandleKey(named("tab"))
	if f := a.Frame(10, 4); f.Focus != "t" || !f.ByID["t"].Focused {
		t.Errorf("focus %q", f.Focus)
	}
}

// 60. scrollbar: auto on a table: view = V (the body viewport) and
// content = n (its rows), on the right border.
func TestTableScrollbar(t *testing.T) {
	a := doc(t, `<tui version="2"><style>#t { border: single; scrollbar: auto; height: 10; }</style>
<screen id="s" focus="#t"><table id="t" each="rows as r" key="r.id" bind="cur"><column title="N">{r.id}</column></table></screen></tui>`)
	// Outer 10: track 8; header 1, V = 7; n = 21: length floor(56/21) = 2;
	// cursor 20: o = 14, pos floor(6·14/14) = 6: rows 7-8.
	bindJSON(t, a, `{"cur":20,"rows":`+rowsJSON(21)+`}`)
	f := a.Frame(10, 10)
	var thumb []int
	for y := 0; y < 10; y++ {
		if f.Grid.At(9, y).Ch == '┃' {
			thumb = append(thumb, y)
		}
	}
	if fmt.Sprint(thumb) != "[7 8]" || f.ByID["t"].ScrollY != 14 {
		t.Errorf("thumb rows %v, offset %d\n%s", thumb, f.ByID["t"].ScrollY, strings.Join(f.Grid.Lines(), "\n"))
	}
	bindJSON(t, a, `{"cur":0,"rows":`+rowsJSON(7)+`}`)
	if f := a.Frame(10, 10); strings.ContainsRune(strings.Join(f.Grid.Lines(), ""), '┃') {
		t.Error("a thumb without overflow")
	}
}

// §6.9.4 "Cost": the rows are resolved once and kept while the store paths
// they read are unchanged: moving the cursor (a write of the bind path) or
// setting an unrelated path keeps them; setting the array, a path under
// it, a path a cell template reads outside the alias, or the whole store,
// or turning on --strict, resolves them again.
func TestTableRowCache(t *testing.T) {
	a := doc(t, `<tui version="2"><screen id="s" focus="#t">
<table id="t" each="rows as r" key="r.id" bind="cur"><column title="N">{r.n}{unit}</column></table>
<text>{other}</text></screen></tui>`)
	bindJSON(t, a, `{"cur":0,"unit":"%","other":"o","rows":`+rowsJSON(5)+`}`)
	n := a.doc.IDs["t"]
	data := func() *tableData {
		a.Frame(20, 8)
		return a.tables[n].data
	}
	first := data()
	if first.texts[0][3] != "row 3%" {
		t.Fatalf("cell %q", first.texts[0][3])
	}
	keysAt(a, 20, 8, named("down"))
	_ = a.Set("other", "p")
	if data() != first {
		t.Error("a cursor move or an unrelated Set dropped the cache")
	}
	for _, c := range []struct {
		path string
		v    any
		cell string
	}{
		{"rows.3.n", "three", "three%"},
		{"unit", "!", "three!"},
		{"rows", []any{map[string]any{"id": 0, "n": "x"}, map[string]any{"id": 1, "n": "y"}, map[string]any{"id": 2, "n": "z"}, map[string]any{"id": 3, "n": "w"}}, "w!"},
	} {
		before := data()
		_ = a.Set(c.path, c.v)
		after := data()
		if after == before || after.texts[0][3] != c.cell {
			t.Errorf("Set(%s): cache kept (%v), cell %q", c.path, after == before, after.texts[0][3])
		}
	}
	before := data()
	bindJSON(t, a, `{"cur":0,"rows":[{"id":0}]}`)
	if after := data(); after == before || after.rows != 1 {
		t.Error("a whole-store write kept the cache")
	}
	before = data()
	a.SetStrict(true)
	after := data()
	if after == before || !after.diags.HasErrors() {
		t.Errorf("--strict kept the cache: %v", after.diags)
	}
}

// 53. #c-cpu { content-align: end; } aligns the header cell only: a rule
// on a column styles its header; the body cells take the class route.
func TestTableColumnRuleAlignsHeaderOnly(t *testing.T) {
	a := doc(t, `<tui version="2"><style>#c-cpu { content-align: end; } .num { content-align: center; }</style>
<screen id="s"><table id="t" each="rows as r" key="r.id">
<column id="c-cpu" title="CPU" width="7">{r.cpu}</column><column id="c-mem" class="num" title="M" width="5">{r.mem}</column>
</table></screen></tui>`)
	bindJSON(t, a, `{"rows":[{"id":1,"cpu":"1.5","mem":"x"}]}`)
	if got := a.Frame(12, 2).Grid.Lines(); got[0] != "    CPU  M  " || got[1] != "1.5      x  " {
		t.Errorf("grid %q", got)
	}
}
