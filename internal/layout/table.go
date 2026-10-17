package layout

import (
	"errors"
	"math"
	"strconv"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// The geometry of a version="2" <table> (SPEC §6.9.3). A table is a
// viewport scrolling on y whose children are its visible column boxes
// (the ones the cascade did not drop for display: none) while it is laid
// out; its rows are generated after layout (§18 step 7) and placed with
// PlaceTableRow.

// TableColumns returns the column boxes of table t, in document order
// (its children of kind column; rows come after them).
func TableColumns(t *Box) []*Box {
	var out []*Box
	for _, c := range t.Children {
		if c.Kind == "column" {
			out = append(out, c)
		}
	}
	return out
}

// visibleColumns returns the visible columns of table t before priority
// hiding (SPEC v0.3b §6.9.3 "What comes before it"): the set its first
// layout recorded in AllColumns, else its column children.
func visibleColumns(t *Box) []*Box {
	if t.AllColumns != nil {
		return t.AllColumns
	}
	return TableColumns(t)
}

// tableHeader is 1 when the header row is shown: some visible column has
// a title attribute, even one that resolves to the empty string (SPEC
// §6.9.3), so the layout does not jump when the data changes.
func tableHeader(cols []*Box) int {
	for _, c := range cols {
		if _, ok := c.Src.Attr("title"); ok {
			return 1
		}
	}
	return 0
}

// tablePlaceholderRow is 1 when table t has no rows and has a placeholder
// attribute (even one that resolves to the empty string), else 0: an
// empty table asks for one body row to paint its placeholder on (SPEC
// §6.9.3, §6.9.4), so an auto-fit table does not collapse to its header
// and hide it.
func tablePlaceholderRow(t *Box) int {
	if clampCells(t.Rows) > 0 {
		return 0
	}
	if _, ok := t.Src.Attr("placeholder"); ok {
		return 1
	}
	return 0
}

// tableIntrinsic is the content-driven size of a table without its border
// and padding (SPEC §6.9.3): across, the mark channel, the visible
// columns' intrinsic widths (a Cell column's cell size, another column's
// measure clamped by its min-width and max-width), and the gaps between
// them, and at least the placeholder's width when the placeholder row is
// shown; down, the header row and max(n, p) body rows, where p is the
// placeholder row (tablePlaceholderRow). The intrinsic width is measured
// over every visible column, priority-hidden ones included (SPEC v0.3b
// §6.9.3 "What comes before it"): visibleColumns is the set before any
// hiding, whether or not the table has been laid out yet. gaps are the
// table's column-gap in a version="3" document (v3).
func tableIntrinsic(t *Box, horizontal, v3 bool) int {
	cols := visibleColumns(t)
	p := tablePlaceholderRow(t)
	if !horizontal {
		return tableHeader(cols) + max(clampCells(t.Rows), p)
	}
	v := clampCells(t.MarkChan)
	if k := len(cols); k > 0 {
		v += columnGap(t.Style, v3) * (k - 1)
	}
	for _, c := range cols {
		if s := c.Style.Width; s.Kind == ir.Cell {
			v += cells(s.N)
		} else {
			v += clampAxis(c, true, clampCells(c.Measure), -1)
		}
	}
	if p == 1 {
		v = max(v, Width(t.Placeholder))
	}
	return clampCells(v)
}

// tableLayout lays out table t inside its content box (SPEC §6.9.3): the
// header row, the column widths and positions, the body viewport V, and
// the scroll offset o, which follows the cursor row (t.Follow) as a list
// follows its selection and is then clamped to [0, max(0, n − V)]. The
// header cells (the visible column boxes) are laid out only while the
// header is shown; their x and width are set either way, for the rows.
// clip is t's clip inside its content box.
//
// In a version="3" document, once A (the content width less the mark
// channel) is known, columns hide by priority (SPEC v0.3b §6.9.3): t's
// intrinsic width and t.Header (above) use the full column set, recorded
// in t.AllColumns, so a hidden column still counts in both, and
// t.Children is narrowed to the kept columns, which is what host.placeRows
// reads afterward (layout.TableColumns), so a hidden column gets no header
// cell, no body cells, and no dump node.
func (e *Engine) tableLayout(t *Box, clip Rect) {
	cols := visibleColumns(t)
	c := t.Content
	t.Header = tableHeader(cols)
	ch := min(clampCells(t.MarkChan), c.W)
	a := max(0, c.W-ch)
	gap := columnGap(t.Style, e.V3)
	if e.V3 {
		t.AllColumns = cols
		cols = hidePriority(cols, a, gap)
		t.Children = cols
	}
	t.View = max(0, c.H-t.Header)
	widths := e.tableColumns(t, cols, a)
	x := c.X + ch
	for j, col := range cols {
		col.X, col.Y, col.W, col.H = x, c.Y, widths[j], 1
		col.Content = col.Outer()
		col.Clip = clip.Intersect(col.Outer())
		col.Laid = t.Header == 1
		x += widths[j] + gap
	}
	n, v := clampCells(t.Rows), t.View
	o := t.ScrollY
	if v > 0 && t.Follow >= 0 && t.Follow < n {
		if t.Follow < o {
			o = t.Follow
		}
		if t.Follow >= o+v {
			o = t.Follow - v + 1
		}
	}
	o = max(0, min(o, max(0, n-v)))
	t.ScrollX, t.ScrollY = 0, o
	t.ContentW, t.ContentH = c.W, max(v, n)
}

// tableColumns allocates the visible columns of t along x (SPEC §6.9.3):
// the §11.4 algorithm over a row whose content width is a (the table's
// content width without the mark channel), with the table's gap between
// adjacent columns. A column's size is its computed width (auto when
// unset: the sibling-fr rule does not apply to columns), clamped by its
// min-width and max-width, percentages of a; an auto column takes its
// measure and is never shrinkable; fr columns share what is left, the
// last taking the remainder, and are then clamped. L003 (a warning on the
// table) reports the cell sizes, the percentages, and the min-widths of
// the auto and fr columns that, with the gaps, exceed a: the widths the
// author asked for, never the data-dependent measures. In a version="3"
// document cols is already the columns left after priority hiding
// (tableLayout), so L003 is evaluated on them (SPEC v0.3b §6.9.3, §14).
func (e *Engine) tableColumns(t *Box, cols []*Box, a int) []int {
	k := len(cols)
	sizes := make([]int, k)
	if k == 0 {
		return sizes
	}
	gap := columnGap(t.Style, e.V3)
	remain := max(0, a-gap*(k-1))
	need := gap * (k - 1)
	var frs []int
	var weights []ir.Scalar
	for j, col := range cols {
		s := col.Style.Width
		var take int
		switch s.Kind {
		case ir.Cell:
			take = cells(s.N)
			need += take
		case ir.Pct:
			take = percent(a, s)
			need += take
		case ir.Fr:
			frs = append(frs, j)
			weights = append(weights, s)
			need += minWidth(col, a)
			continue
		default: // auto, or unset
			take = clampCells(col.Measure)
			need += minWidth(col, a)
		}
		take = clampAxis(col, true, take, a)
		sizes[j] = take
		remain -= take
	}
	remain = max(0, remain)
	for i, take := range frSplit(remain, weights) {
		sizes[frs[i]] = clampAxis(cols[frs[i]], true, take, a)
	}
	if need > a {
		e.report(t, ir.Warning, "L003", "the widths and min-widths of the visible columns need %d cells but the table has %d; columns are clipped", need, a)
	}
	return sizes
}

// minWidth is a column's computed min-width in cells (a percent of a), or
// 0 when it has none.
func minWidth(col *Box, a int) int {
	switch s := col.Style.MinW; s.Kind {
	case ir.Cell:
		return cells(s.N)
	case ir.Pct:
		return percent(a, s)
	}
	return 0
}

// columnPriority returns a column's priority="N" (SPEC v0.3b §6.9.3) and
// whether it has one. priority is a table column attribute, not a CSS
// property (SPEC §6.15 ‡), validated as a non-negative integer at parse
// time (internal/parse); a column without it never hides by priority.
func columnPriority(col *Box) (n int, ok bool) {
	if col.Src == nil {
		return 0, false
	}
	v, has := col.Src.Attr("priority")
	if !has {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	if errors.Is(err, strconv.ErrRange) {
		// Digits only (the parser checked), just too many of them: the
		// largest priority there is, so it hides last.
		return math.MaxInt, true
	}
	return n, err == nil
}

// naturalWidth is a visible column's natural width for priority hiding
// (SPEC v0.3b §6.9.3): a Cell column's cell size; floor(A·p/100) for a
// p% column; an auto column's measure, clamped by its min-width and
// max-width; an fr column's min-width (0 when unset).
func naturalWidth(col *Box, a int) int {
	switch s := col.Style.Width; s.Kind {
	case ir.Cell:
		return cells(s.N)
	case ir.Pct:
		return percent(a, s)
	case ir.Fr:
		return minWidth(col, a)
	default: // auto, or unset
		return clampAxis(col, true, clampCells(col.Measure), a)
	}
}

// hidePriority applies SPEC v0.3b §6.9.3 "Columns that hide by priority":
// while the natural widths of the visible columns, plus gap·(k − 1),
// exceed a, and at least one visible column has a priority, the visible
// column with the smallest priority (the last in document order among
// equal priorities) is hidden; the sum is then recomputed. Columns
// without a priority never hide this way, so the rule is opt-in per
// column. Only called for a version="3" document (tableLayout); gap is
// the table's column-gap there.
func hidePriority(cols []*Box, a, gap int) []*Box {
	kept := append([]*Box(nil), cols...)
	for {
		sum := gap * max(0, len(kept)-1)
		for _, c := range kept {
			sum += naturalWidth(c, a)
		}
		if sum <= a {
			return kept
		}
		idx, lowest := -1, 0
		for i, c := range kept {
			n, ok := columnPriority(c)
			if !ok {
				continue
			}
			if idx == -1 || n <= lowest {
				idx, lowest = i, n
			}
		}
		if idx == -1 {
			return kept
		}
		kept = append(kept[:idx], kept[idx+1:]...)
	}
}

// PlaceTableRow places row i of table t after layout (SPEC §6.9.3, §18
// step 7): a box W × 1 at (content x, body y + i − o), whatever its own
// style says, with its mark channel (t.MarkChan columns at its left, the
// header's too) before its content, and one body cell per visible column
// in row.Children, in the order of cols: at the column's x and width, on
// the row's line. Everything is clipped at the table's content box.
func PlaceTableRow(t, row *Box, cols []*Box, i int) {
	c := t.Content
	clip := t.Clip.Intersect(c)
	ch := min(clampCells(t.MarkChan), c.W)
	row.X, row.Y, row.W, row.H = c.X, c.Y+t.Header+i-t.ScrollY, c.W, 1
	row.Laid = true
	row.Clip = clip.Intersect(row.Outer())
	row.Chan = ch
	row.Content = Rect{X: row.X + ch, Y: row.Y, W: max(0, row.W-ch), H: 1}
	for j, cell := range row.Children {
		if j >= len(cols) {
			break
		}
		col := cols[j]
		cell.X, cell.Y, cell.W, cell.H = col.X, row.Y, col.W, 1
		cell.Laid = true
		cell.Content = cell.Outer()
		cell.Clip = row.Clip.Intersect(cell.Outer())
	}
}
