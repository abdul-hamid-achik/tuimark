package paint

import "github.com/abdul-hamid-achik/tuimark/internal/layout"

// Clone returns a copy of g that shares nothing with it: painting over the
// copy leaves g as it was.
func (g *Grid) Clone() *Grid {
	c := *g
	c.Cells = append([]Cell(nil), g.Cells...)
	return &c
}

// PaintRow replaces row y of g with s, painted from column 0 and cut at
// the grid's width, the rest of the row spaces, every cell in the
// terminal's default colors with attrs and no owner. It is how `tuimark
// run` paints its event strip and diagnostics panel over a copy of a
// frame's grid (SPEC v0.4a §15.9). A whole row is replaced, so no wide
// cluster is split; s is segmented and painted by the rules every text
// follows (SPEC v0.2 §12.1: a control character becomes a space, a wide
// cluster that does not fit is a space), and the row is made to read as
// its cells (unjoin). The input cursor is hidden when it stands on the
// row. A row outside the grid changes nothing.
func (g *Grid) PaintRow(y int, s string, attrs uint8) {
	if y < 0 || y >= g.H {
		return
	}
	row := g.Cells[y*g.W : (y+1)*g.W]
	for x := range row {
		row[x] = Cell{Ch: ' ', Attrs: attrs}
	}
	p := &painter{g: g}
	p.text(layout.Rect{X: 0, Y: y, W: g.W, H: 1}, 0, y, s, cellStyle{attrs: attrs, clearOwner: true})
	p.unjoin()
	if g.CursorOn && g.CursorY == y {
		g.CursorOn = false
	}
}
