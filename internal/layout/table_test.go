package layout

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// SPEC v0.2b §6.9.3: the geometry of a table, on hand-built boxes. The
// host (internal/host) builds the same boxes from a document and adds
// the rows after layout; see its table tests for the whole frame.

// tbl builds a table box with n rows and the given columns.
func tbl(decls string, rows int, cols ...*Box) *Box {
	t := n("table#t", decls, cols...)
	t.Rows = rows
	return t
}

// col builds a column box with a measure; title adds a title attribute.
func col(decls string, measure int, title bool) *Box {
	c := n("column", decls)
	c.Measure, c.Fixed = measure, true
	c.Src = &ir.Node{Tag: "column", Kind: "column", Attrs: map[string]string{}}
	if title {
		c.Src.Attrs["title"] = ""
	}
	return c
}

func widths(t *Box) string {
	var out []string
	for _, c := range TableColumns(t) {
		out = append(out, fmt.Sprintf("%d@%d", c.W, c.X))
	}
	return strings.Join(out, " ")
}

// 50. Column widths: the §6.9.5 case (cells 5, fr, auto 4 over A = 30 with
// gap 1); percentages of A (the width without the mark channel); min and
// max clamp an fr column after its share, without redistribution; an auto
// column takes its measure, whatever the space; the columns start after
// the mark channel.
func TestTableColumnWidths(t *testing.T) {
	for _, c := range []struct {
		name, decls string
		mark        int
		cols        []*Box
		want        string
	}{
		{"spec 6.9.5", "gap: 1", 0, []*Box{col("width: 5", 3, true), col("width: 1fr", 24, true), col("", 4, true)}, "5@0 19@6 4@26"},
		{"percent of A", "", 4, []*Box{col("width: 50%", 1, true), col("width: 1fr", 1, false)}, "13@4 13@17"},
		{"fr max-width", "", 0, []*Box{col("width: 1fr; max-width: 4", 1, false), col("width: 1fr", 1, false)}, "4@0 15@4"},
		{"fr min-width", "", 0, []*Box{col("width: 1fr; min-width: 25", 1, false), col("width: 5", 1, false)}, "25@0 5@25"},
		{"auto measure overflows", "gap: 2", 0, []*Box{col("", 25, false), col("", 10, false)}, "25@0 10@27"},
		{"auto clamped", "", 0, []*Box{col("max-width: 6", 25, false), col("min-width: 3", 1, false)}, "6@0 3@6"},
	} {
		t.Run(c.name, func(t *testing.T) {
			tb := tbl(c.decls, 0, c.cols...)
			tb.MarkChan = c.mark
			e := run(t, n("col", "", tb), 30, 5)
			if got := widths(tb); got != c.want {
				t.Errorf("widths %s, want %s", got, c.want)
			}
			for _, d := range e.Diags {
				if d.Code == "L003" {
					t.Errorf("unexpected %s", d)
				}
			}
		})
	}
}

// 50. L003 counts the widths the author asked for: cell sizes,
// percentages of A, the min-widths of auto and fr columns, and the gaps;
// never the measures.
func TestTableL003(t *testing.T) {
	tb := tbl("gap: 1", 0, col("width: 15", 1, false), col("width: 1fr; min-width: 4", 1, false), col("width: 30%", 1, false))
	e := run(t, n("col", "", tb), 30, 5)
	var got []string
	for _, d := range e.Diags {
		got = append(got, d.Code+" "+d.ID+" "+d.Msg)
	}
	// 15 + 4 + floor(30·30/100) + 2 gaps = 30: fits exactly.
	if len(got) != 0 {
		t.Errorf("diags %v", got)
	}
	tb = tbl("gap: 1", 0, col("width: 16", 1, false), col("width: 1fr; min-width: 4", 1, false), col("width: 30%", 1, false))
	e = run(t, n("col", "", tb), 30, 5)
	if len(e.Diags) != 1 || e.Diags[0].Code != "L003" || e.Diags[0].ID != "t" || !strings.Contains(e.Diags[0].Msg, "need 31 cells but the table has 30") {
		t.Errorf("diags %v", e.Diags)
	}
}

// §6.9.3: the header is shown when a visible column has a title attribute;
// the body viewport is the rest of the content box; the intrinsic size is
// the channel, the columns' intrinsic widths, and the gaps across, and the
// header plus one row per row down, plus border and padding; a table is
// shrinkable on y only.
func TestTableHeaderViewAndIntrinsic(t *testing.T) {
	tb := tbl("border: single; gap: 1", 40, col("width: 6", 30, true), col("width: 1fr; min-width: 3; max-width: 9", 30, false), col("", 4, false))
	tb.MarkChan = 2
	e := run(t, n("col", "", tb, txt("status", "")), 30, 10)
	if tb.Header != 1 || tb.View != 6 || tb.H != 9 {
		t.Errorf("header %d view %d h %d", tb.Header, tb.View, tb.H)
	}
	// 2 + 6 + clamp(30, 3, 9) + 4 + 2 gaps + 2 border columns.
	if got := e.intrinsic(tb, true, 0); got != 2+6+9+4+2+2 {
		t.Errorf("intrinsic width %d", got)
	}
	if got := e.intrinsic(tb, false, 0); got != 1+40+2 {
		t.Errorf("intrinsic height %d", got)
	}
	if !e.shrinkable(tb, false) || e.shrinkable(tb, true) {
		t.Error("a table is shrinkable on y only")
	}
	noTitle := tbl("", 3, col("", 1, false))
	run(t, n("col", "", noTitle), 10, 5)
	if noTitle.Header != 0 || noTitle.H != 3 || TableColumns(noTitle)[0].Laid {
		t.Errorf("without titles: header %d h %d, column laid %v", noTitle.Header, noTitle.H, TableColumns(noTitle)[0].Laid)
	}
}

// §6.9.3: the offset follows the cursor row (above: o = cursor; below:
// o = cursor − V + 1), then is clamped to [0, max(0, n − V)]; with V = 0
// it is only clamped. scroll counts body rows: the extent is max(V, n).
func TestTableOffsetFollowsCursor(t *testing.T) {
	for _, c := range []struct {
		rows, height, cursor, start, want, extent int
	}{
		{100, 6, 0, 0, 0, 100},    // V = 5
		{100, 6, 10, 0, 6, 100},   // below: 10 − 5 + 1
		{100, 6, 3, 8, 3, 100},    // above
		{100, 6, 50, 48, 48, 100}, // visible: unchanged
		{100, 6, -1, 500, 95, 100},
		{3, 6, 2, 9, 0, 5},
		{100, 1, 50, 7, 7, 100}, // V = 0: only clamped
	} {
		tb := tbl(fmt.Sprintf("height: %d", c.height), c.rows, col("", 1, true))
		tb.Follow, tb.ScrollY = c.cursor, c.start
		run(t, n("col", "", tb), 10, 10)
		if tb.ScrollY != c.want || tb.ContentH != c.extent {
			t.Errorf("%+v: offset %d extent %d", c, tb.ScrollY, tb.ContentH)
		}
	}
}

// §6.9.3: a row is W × 1 at (content x, body y + i − o), its mark channel
// inside it; each body cell is at its column's x and width; all clipped
// at the table's content box.
func TestPlaceTableRow(t *testing.T) {
	tb := tbl("border: single; gap: 1", 10, col("width: 3", 3, true), col("width: 20", 2, true))
	tb.MarkChan, tb.Follow = 2, 0
	run(t, n("col", "", tb), 12, 6)
	cols := TableColumns(tb)
	row := n("item", "", n("text", ""), n("text", ""))
	PlaceTableRow(tb, row, cols, 1)
	if got := fmt.Sprint(row.Outer(), row.Content, row.Chan, row.Clip); got != "{1 3 10 1} {3 3 8 1} 2 {1 3 10 1}" {
		t.Errorf("row %s", got)
	}
	a, b := row.Children[0], row.Children[1]
	if got := fmt.Sprint(a.Outer(), a.Clip, b.Outer(), b.Clip, a.Laid && b.Laid); got != "{3 3 3 1} {3 3 3 1} {7 3 20 1} {7 3 4 1} true" {
		t.Errorf("cells %s", got)
	}
}

// colP is col with a priority attribute (SPEC v0.3b §6.9.3).
func colP(decls string, measure int, title bool, priority int) *Box {
	c := col(decls, measure, title)
	c.Src.Attrs["priority"] = fmt.Sprint(priority)
	return c
}

// 95. Column priority: the table's intrinsic width is a standalone
// measure over every visible column (tableIntrinsic itself never hides
// anything; only tableLayout does, once A is known), so it does not
// depend on whether the table is ever laid out narrow enough to hide one.
func TestTablePriorityIntrinsicWidth(t *testing.T) {
	tb := tbl("gap: 1", 1,
		col("width: 18", 18, true), colP("width: 18", 18, true, 3),
		colP("width: 18", 18, true, 1), colP("width: 18", 18, true, 2),
		colP("width: 18", 18, true, 1))
	c := css.NewCascade(nil, css.Env{Cols: 200, Rows: 5})
	styleTree(c, tb, nil)
	e := &Engine{File: "t.tui", V3: true, memo: map[memoKey]int{}, seen: map[string]bool{}}
	// 5*18 + 4 gaps = 94, measured before tb is ever placed (so before
	// any hiding could apply).
	if got := e.intrinsic(tb, true, 0); got != 94 {
		t.Errorf("intrinsic width = %d, want 94", got)
	}
	// After a layout at 40 columns has hidden c, d, and e, a fresh measure
	// of the same box still counts them ("the intrinsic width does not
	// shrink when a column hides"), and so does a second layout, which
	// brings them back when the table is wide enough.
	runV3(t, n("col", "", tb), 40, 5)
	if got := widths(tb); got != "18@0 18@19" {
		t.Fatalf("widths at 40 = %s, want a b", got)
	}
	e2 := &Engine{File: "t.tui", V3: true, memo: map[memoKey]int{}, seen: map[string]bool{}}
	if got := e2.intrinsic(tb, true, 0); got != 94 {
		t.Errorf("intrinsic width after hiding = %d, want 94", got)
	}
	runV3(t, n("col", "", tb), 120, 5)
	if got := widths(tb); got != "18@0 18@19 18@38 18@57 18@76" {
		t.Errorf("widths when laid out again at 120 = %s, want all five", got)
	}
}

// 95. The header row is decided before priority hides anything: a header
// shown only because a hidden column has a title stays, blank.
func TestTablePriorityKeepsHeader(t *testing.T) {
	tb := tbl("", 1, col("width: 18", 18, false), colP("width: 18", 18, true, 1))
	runV3(t, n("col", "", tb), 20, 5)
	if got := widths(tb); got != "18@0" {
		t.Fatalf("widths at 20 = %s, want the untitled column alone", got)
	}
	if tb.Header != 1 || tb.View != 1 {
		t.Errorf("header %d view %d, want 1 and 1 (the table auto-fits: header plus one row)", tb.Header, tb.View)
	}
	if !TableColumns(tb)[0].Laid {
		t.Errorf("the kept column's header cell is not laid out")
	}
}

// A priority too large for an int is still digits only (the parser's
// check): it is the largest priority, so it hides last.
func TestColumnPriorityOverflow(t *testing.T) {
	c := col("width: 18", 18, false)
	c.Src.Attrs["priority"] = "99999999999999999999999"
	if n, ok := columnPriority(c); !ok || n != math.MaxInt {
		t.Errorf("priority = %d, %v; want MaxInt, true", n, ok)
	}
	tb := tbl("", 1, c, colP("width: 18", 18, false, 5))
	runV3(t, n("col", "", tb), 20, 5)
	if got := widths(tb); got != "18@0" {
		t.Errorf("widths at 20 = %s, want the huge-priority column kept", got)
	}
}

// runV3 is run (layout_test.go) with Engine.V3 set, for the version="3"
// features of a table (priority, v0.3b).
func runV3(t *testing.T, root *Box, cols, rows int) *Engine {
	t.Helper()
	c := css.NewCascade(nil, css.Env{Cols: cols, Rows: rows})
	styleTree(c, root, nil)
	if len(c.Diags) != 0 {
		t.Fatalf("cascade: %v", c.Diags)
	}
	e := &Engine{File: "t.tui", V3: true}
	e.Layout(root, nil, cols, rows)
	return e
}

// 95. Column priority (SPEC v0.3b §6.9.3): while the natural widths of
// the visible columns plus the gaps exceed A, the visible column with the
// smallest priority (the last in document order among equal priorities)
// hides; a column without priority never hides; a hidden column is
// removed from TableColumns (so it gets no header cell, no body cell, and
// no dump node); L003 is then evaluated on the columns left, so it does
// not also fire once enough of them have hidden.
func TestTablePriorityHiding(t *testing.T) {
	cA := col("width: 18", 18, true)     // no priority: never hides
	cB := colP("width: 18", 18, true, 3) // priority 3
	cC := colP("width: 18", 18, true, 1) // priority 1 (hides 2nd: earlier than e)
	cD := colP("width: 18", 18, true, 2) // priority 2
	cE := colP("width: 18", 18, true, 1) // priority 1 (hides 1st: the last of the tie)
	tb := tbl("gap: 1", 1, cA, cB, cC, cD, cE)
	e := runV3(t, n("col", "", tb), 80, 5)
	// 5*18 + 4 = 94 > 80: e hides (the last of the two priority=1
	// columns), leaving 4*18 + 3 = 75 <= 80.
	if got := widths(tb); got != "18@0 18@19 18@38 18@57" {
		t.Errorf("widths at 80 = %s, want a b c d (e hidden)", got)
	}
	if len(TableColumns(tb)) != 4 {
		t.Errorf("TableColumns after hiding = %d, want 4", len(TableColumns(tb)))
	}
	for _, d := range e.Diags {
		if d.Code == "L003" {
			t.Errorf("unexpected L003 at 80 once e hides: %s", d.Msg)
		}
	}
	// At 40: only a and b are left (75 shrinks as c, then d hide too).
	tb2 := tbl("gap: 1", 1, col("width: 18", 18, true), colP("width: 18", 18, true, 3),
		colP("width: 18", 18, true, 1), colP("width: 18", 18, true, 2), colP("width: 18", 18, true, 1))
	runV3(t, n("col", "", tb2), 40, 5)
	if got := widths(tb2); got != "18@0 18@19" {
		t.Errorf("widths at 40 = %s, want a b", got)
	}
	// At 10: only a is left, cut, with L003.
	tb3 := tbl("gap: 1", 1, col("width: 18", 18, true), colP("width: 18", 18, true, 3),
		colP("width: 18", 18, true, 1), colP("width: 18", 18, true, 2), colP("width: 18", 18, true, 1))
	e3 := runV3(t, n("col", "", tb3), 10, 5)
	// a's own cell width (18) is unchanged; it overflows the 10-column
	// table and is clipped at paint time, which is what L003 warns about.
	if got := widths(tb3); got != "18@0" {
		t.Errorf("widths at 10 = %s, want a alone at its own width, clipped", got)
	}
	found := false
	for _, d := range e3.Diags {
		if d.Code == "L003" {
			found = true
		}
	}
	if !found {
		t.Errorf("want L003 at 10, got %v", e3.Diags)
	}
}
