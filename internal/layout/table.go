package layout

import "github.com/abdul-hamid-achik/tuimark/internal/ir"

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
// placeholder row (tablePlaceholderRow).
func tableIntrinsic(t *Box, horizontal bool) int {
	cols := TableColumns(t)
	p := tablePlaceholderRow(t)
	if !horizontal {
		return tableHeader(cols) + max(clampCells(t.Rows), p)
	}
	v := clampCells(t.MarkChan)
	if k := len(cols); k > 0 {
		v += t.Style.Gap * (k - 1)
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
func (e *Engine) tableLayout(t *Box, clip Rect) {
	cols := TableColumns(t)
	c := t.Content
	t.Header = tableHeader(cols)
	t.View = max(0, c.H-t.Header)
	ch := min(clampCells(t.MarkChan), c.W)
	widths := e.tableColumns(t, cols, max(0, c.W-ch))
	x := c.X + ch
	for j, col := range cols {
		col.X, col.Y, col.W, col.H = x, c.Y, widths[j], 1
		col.Content = col.Outer()
		col.Clip = clip.Intersect(col.Outer())
		col.Laid = t.Header == 1
		x += widths[j] + t.Style.Gap
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
// author asked for, never the data-dependent measures.
func (e *Engine) tableColumns(t *Box, cols []*Box, a int) []int {
	k := len(cols)
	sizes := make([]int, k)
	if k == 0 {
		return sizes
	}
	gap := t.Style.Gap
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
