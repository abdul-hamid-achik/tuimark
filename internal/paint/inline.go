package paint

import (
	"strconv"
	"strings"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
)

// The relative-move writer of an inline region (SPEC v0.4d §26.15.3,
// decision 243). The region is the active screen drawn below the
// terminal's cursor instead of in the alternate screen, so its row on the
// screen is unknown (it would take a cursor report to learn it): rows are
// reached with CR, CSI N A, and CSI N B only, columns with CSI n G (CHA),
// and new rows are made by CR LF on the region's last row. The sequences
// never include CSI H, CSI 2 J, an absolute row, or CSI ? 1049: Diff and
// Full, which the full-screen session uses, are not touched.

// Region keeps what the writer knows of the terminal between frames: the
// grid it last drew (nil when the region was erased), how many rows the
// region has on the terminal, and the row of the region the cursor was
// left on. The zero Region is a region not drawn yet, with the cursor at
// its (empty) first row.
type Region struct {
	prev *Grid
	// exist is the number of rows of the region on the terminal. Row 0 is
	// the line the cursor was on when the region began, so a region of no
	// rows still has it.
	exist int
	// cur is the row of the region the cursor is on, 0-based.
	cur int
	// started is false until the first frame or erase, so the first frame
	// starts with CR CSI J (the region's start line may hold what a shell
	// left of it).
	started bool
	// col0 is whether the cursor is known to be on the first column: after
	// CR, which is how the rows are reached and made, so a write at the
	// column 0 needs no CHA.
	col0 bool
}

// Drawn reports whether the region has a grid on the terminal (a frame was
// drawn and no erase since).
func (r *Region) Drawn() bool { return r.prev != nil }

// Last returns the grid the region last drew, nil when it was erased.
func (r *Region) Last() *Grid { return r.prev }

// Row returns the row of the region the cursor is on.
func (r *Region) Row() int { return r.cur }

// Forget drops the grid so that the next frame is a full repaint, without
// writing anything: the terminal's screen is no longer what the region
// last drew (a commit was printed, the terminal was resized).
func (r *Region) Forget() { r.prev = nil }

// Erase returns the sequence that moves to the region's first row and
// erases from there to the end of the screen: CSI N A when the cursor is
// not on it, CR, CSI J. The next frame is a full repaint.
func (r *Region) Erase() string {
	var b strings.Builder
	r.moveUp(&b)
	b.WriteString("\r\x1b[J")
	r.prev, r.exist, r.cur, r.started, r.col0 = nil, 1, 0, true, true
	return b.String()
}

// moveUp moves the cursor to the region's first row.
func (r *Region) moveUp(b *strings.Builder) {
	if r.cur > 0 {
		b.WriteString("\x1b[")
		b.WriteString(strconv.Itoa(r.cur))
		b.WriteByte('A')
	}
	r.cur = 0
}

// goRow moves the cursor to row y of the region: relatively, within the
// rows the region has, and by CR LF from its last row to make the rows it
// does not have yet.
func (r *Region) goRow(b *strings.Builder, y int) {
	if r.exist < 1 {
		r.exist = 1
	}
	for r.cur != y {
		switch {
		case r.cur < y && r.cur+1 >= r.exist:
			b.WriteString("\r\n")
			r.cur++
			r.exist = r.cur + 1
			r.col0 = true
		case r.cur < y:
			n := min(y, r.exist-1) - r.cur
			b.WriteString("\x1b[")
			b.WriteString(strconv.Itoa(n))
			b.WriteByte('B')
			r.cur += n
		default:
			b.WriteString("\x1b[")
			b.WriteString(strconv.Itoa(r.cur - y))
			b.WriteByte('A')
			r.cur = y
		}
	}
}

// Frame returns the bytes that bring the region from what it last drew to
// g, and remembers g. A region that was erased (or never drawn) is drawn
// whole: g is diffed against blank cells, after CR CSI J on the first
// frame; a region of another width is erased first. Rows are made with
// CR LF, rows the region no longer uses are erased with CSI J, and the
// cursor ends on the caret of a focused input, shown, or on the region's
// last row, hidden.
func (r *Region) Frame(g *Grid, o Options) string {
	var b strings.Builder
	full := r.prev == nil || r.prev.W != g.W
	prev := r.prev
	if full {
		// Never drawn, or drawn at another width: erased from its first
		// row. A region erased already (a commit, a resize) has the
		// cursor on its first row, and is drawn whole.
		if r.prev != nil || !r.started {
			b.WriteString(r.Erase())
		}
		prev = nil
	}
	r.started = true
	blank := Cell{Ch: ' '}
	at := func(x, y int) Cell {
		if prev == nil || y >= prev.H {
			return blank
		}
		return *prev.At(x, y)
	}
	// changed reports whether the unit led by next's cell (x, y) differs
	// from what the terminal shows there: its lead or, for a wide cluster,
	// its continuation.
	changed := func(x, y int) bool {
		c := g.At(x, y)
		if !sameLook(at(x, y), *c) {
			return true
		}
		return c.Wide && x+1 < g.W && !sameLook(at(x+1, y), *g.At(x+1, y))
	}
	unit := func(x, y int) int {
		if g.At(x, y).Wide {
			return 2
		}
		return 1
	}
	var style Cell // the terminal's style: reset after every frame
	for y := 0; y < g.H; y++ {
		if y >= r.exist {
			// A row the terminal does not have yet: made even when blank,
			// so the region has all its rows.
			r.goRow(&b, y)
		}
		x := 0
		for x < g.W {
			if g.At(x, y).Cont {
				x++
				continue
			}
			if !changed(x, y) {
				x += unit(x, y)
				continue
			}
			r.goRow(&b, y)
			r.column(&b, x)
			for force := false; x < g.W && !g.At(x, y).Cont && (force || changed(x, y)); {
				c := g.At(x, y)
				if !sameStyle(style, *c) {
					b.WriteString(SGRWith(*c, o.Profile))
					style = *c
				}
				x, force = o.cluster(&b, c, x, g.W, !full)
				r.col0 = false
			}
		}
	}
	b.WriteString("\x1b[0m")
	// The rows the region no longer uses.
	if used := max(g.H, 1); r.exist > used {
		r.goRow(&b, used)
		b.WriteString("\r\x1b[J")
		r.exist, r.col0 = used, true
	}
	if g.CursorOn {
		r.goRow(&b, min(g.CursorY, max(g.H-1, 0)))
		r.column(&b, g.CursorX)
		b.WriteString("\x1b[?25h")
	} else {
		r.goRow(&b, max(g.H-1, 0))
		b.WriteString("\x1b[?25l")
	}
	r.prev = g
	return b.String()
}

// column moves the cursor to column x with CHA, unless it is known to be
// there.
func (r *Region) column(b *strings.Builder, x int) {
	if x == 0 && r.col0 {
		return
	}
	b.WriteString("\x1b[")
	b.WriteString(strconv.Itoa(x + 1))
	b.WriteByte('G')
	r.col0 = x == 0
}

// sameLook reports whether two cells paint the same on a terminal: the
// cluster and its look, not the id of the node that owns it.
func sameLook(a, b Cell) bool {
	return a.Ch == b.Ch && a.Ext == b.Ext && a.Wide == b.Wide && a.Cont == b.Cont && sameStyle(a, b)
}

// blankCell reports whether c is a cell a committed row does not need to
// write: a space with the default background and no attribute (SPEC v0.4d
// §26.15.4).
func blankCell(c *Cell) bool {
	return c.Ch == ' ' && c.Ext == "" && !c.Wide && !c.Cont && c.Attrs == 0 &&
		(c.BG.Kind == css.ColorNone || c.BG.Kind == css.ColorDefault)
}

// CommitRows returns the bytes that print the rows of g into the
// scrollback (SPEC v0.4d §26.15.4, decision 244): for each row its cells,
// with their SGR, up to its last cell that is not blank, then CSI 0 m and
// CR LF. The terminal's style is the default at the start of each row, so
// a default-style row is its text alone.
func (o Options) CommitRows(g *Grid) string {
	var b strings.Builder
	for y := 0; y < g.H; y++ {
		last := -1
		for x := g.W - 1; x >= 0; x-- {
			if c := g.At(x, y); !c.Cont && !blankCell(c) {
				last = x
				break
			}
		}
		var style Cell
		for x := 0; x <= last; {
			c := g.At(x, y)
			if c.Cont {
				x++ // unreachable in a painted grid: a continuation follows its lead
				continue
			}
			if !sameStyle(style, *c) {
				b.WriteString(SGRWith(*c, o.Profile))
				style = *c
			}
			x, _ = o.cluster(&b, c, x, g.W, false)
		}
		b.WriteString("\x1b[0m\r\n")
	}
	return b.String()
}
