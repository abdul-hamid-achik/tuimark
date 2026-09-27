package layout

import (
	"fmt"
	"strings"
	"testing"

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
