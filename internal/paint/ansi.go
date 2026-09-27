package paint

import (
	"fmt"
	"strconv"
	"strings"
)

// SGR returns the escape sequence that selects c's style from a reset
// state, in truecolor (the v0.1 output).
func SGR(c Cell) string { return SGRWith(c, TrueColor) }

// sgrAttrs are the attribute codes in the order SGR writes them.
var sgrAttrs = [...]struct {
	bit  uint8
	code string
}{{Bold, ";1"}, {Dim, ";2"}, {Italic, ";3"}, {Underline, ";4"}, {Reverse, ";7"}}

// SGRWith returns the escape sequence that selects c's style from a reset
// state in profile p (SPEC v0.2 §26.3): CSI 0, the attributes that are on
// (bold 1, dim 2, italic 3, underline 4, reverse 7, in that order), then
// the foreground and background codes. The none profile omits the color
// codes and keeps the attributes, so a reverse selection stays visible.
func SGRWith(c Cell, p Profile) string {
	var b strings.Builder
	b.WriteString("\x1b[0")
	for _, a := range sgrAttrs {
		if c.Attrs&a.bit != 0 {
			b.WriteString(a.code)
		}
	}
	for _, code := range [2]string{colorCode(c.FG, false, p), colorCode(c.BG, true, p)} {
		if code != "" {
			b.WriteByte(';')
			b.WriteString(code)
		}
	}
	b.WriteByte('m')
	return b.String()
}

func sameStyle(a, b Cell) bool {
	return a.FG == b.FG && a.BG == b.BG && a.Attrs == b.Attrs
}

// Options tunes how Diff and Full encode a frame for a terminal. The zero
// value is the default: truecolor, CHA re-positioning on.
type Options struct {
	// Profile is the color depth of the SGR sequences (SPEC v0.2 §26.3).
	// Run and preview set it from the environment; the zero value is
	// truecolor.
	Profile Profile
	// NoCHA turns off CHA re-positioning. By default every complex cluster
	// (SPEC v0.2 §11.5.1: width 2 or more than one code point) whose last
	// column is not the last column of its row is followed by CSI n G, n
	// being the 1-based column after the cluster, so that a terminal that
	// measures the cluster differently damages at most that cluster's own
	// cells (§26.5); in a diff frame the cluster's columns are also erased
	// first and the unit after it is written too (see DiffWith). Run sets
	// NoCHA when the terminal has grapheme cluster mode 2027 on.
	NoCHA bool
}

// cluster writes the lead cell c at 0-based column x of a w-column row and
// returns the column after it and whether it wrote CHA. Unless o.NoCHA, a
// complex cluster is followed by CHA to that column when it does not end
// the row, and, when erase is set (a diff frame, where the screen still
// shows the previous frame), preceded by ECH (CSI n X) over its n columns,
// so that a terminal that draws it narrower leaves blanks, not stale
// glyphs, in the rest of its columns.
func (o Options) cluster(b *strings.Builder, c *Cell, x, w int, erase bool) (int, bool) {
	next := x + 1
	if c.Wide {
		next = x + 2
	}
	cha := !o.NoCHA && c.Complex()
	if cha && erase {
		b.WriteString("\x1b[")
		b.WriteString(strconv.Itoa(next - x))
		b.WriteByte('X')
	}
	b.WriteRune(c.Ch)
	b.WriteString(c.Ext)
	if cha && next < w {
		b.WriteString("\x1b[")
		b.WriteString(strconv.Itoa(next + 1))
		b.WriteByte('G')
		return next, true
	}
	return next, false
}

// AutowrapOff and AutowrapOn turn autowrap (DECAWM) off and back on (SPEC
// v0.2 §26.1, §26.5). With autowrap off, a cluster in the last column of a
// row that the terminal draws wider than the layout measured is clipped
// to that column instead of wrapping the row, which on the last row would
// scroll the whole screen. Run writes AutowrapOff in its enter sequence
// and AutowrapOn in its leave sequence; FullWith writes both around the
// grid.
const (
	AutowrapOff = "\x1b[?7l"
	AutowrapOn  = "\x1b[?7h"
)

// Full renders the whole grid as ANSI lines (for `preview` on a TTY), with
// the default Options.
func Full(g *Grid) string { return FullWith(g, Options{}) }

// FullWith renders the whole grid as ANSI lines, between AutowrapOff and
// AutowrapOn. Each wide cluster is written once; continuation cells write
// nothing.
func FullWith(g *Grid, o Options) string {
	var b strings.Builder
	b.WriteString(AutowrapOff)
	for y := 0; y < g.H; y++ {
		var last *Cell
		for x := 0; x < g.W; {
			c := g.At(x, y)
			if c.Cont {
				x++ // unreachable in a painted grid: a continuation follows its lead
				continue
			}
			if last == nil || !sameStyle(*last, *c) {
				b.WriteString(SGRWith(*c, o.Profile))
			}
			x, _ = o.cluster(&b, c, x, g.W, false)
			last = c
		}
		b.WriteString("\x1b[0m\n")
	}
	b.WriteString(AutowrapOn)
	return b.String()
}

// Diff renders the changes from prev to next with absolute cursor moves and
// the default Options. A nil or differently sized prev repaints everything.
func Diff(prev, next *Grid) string { return DiffWith(prev, next, Options{}) }

// DiffWith renders the changes from prev to next. A wide cluster is one
// unit (SPEC v0.2 §26.5): when either of its columns changed, both are
// rewritten by writing the lead once; a continuation column is never
// written on its own. Unless o.NoCHA, a complex cluster that is rewritten
// is erased first (ECH over its columns), and when CHA follows it, the
// unit after it on its row is rewritten too, even when unchanged: a
// terminal that draws the cluster wider has spilled into that unit, and
// rewriting it after the CHA confines the damage to the cluster's own
// cells, as a full repaint does.
func DiffWith(prev, next *Grid, o Options) string {
	var b strings.Builder
	full := prev == nil || prev.W != next.W || prev.H != next.H
	if full {
		b.WriteString("\x1b[0m\x1b[2J")
	}
	// changed reports whether the unit led by next's cell (x, y) differs
	// from prev: its lead or, for a wide cluster, its continuation.
	changed := func(x, y int) bool {
		if full || *prev.At(x, y) != *next.At(x, y) {
			return true
		}
		return next.At(x, y).Wide && x+1 < next.W && *prev.At(x+1, y) != *next.At(x+1, y)
	}
	unit := func(x, y int) int {
		if next.At(x, y).Wide {
			return 2
		}
		return 1
	}
	var style *Cell
	for y := 0; y < next.H; y++ {
		x := 0
		for x < next.W {
			if next.At(x, y).Cont {
				x++ // unreachable in a painted grid: a continuation follows its lead
				continue
			}
			if !changed(x, y) {
				x += unit(x, y)
				continue
			}
			fmt.Fprintf(&b, "\x1b[%d;%dH", y+1, x+1)
			for force := false; x < next.W && !next.At(x, y).Cont && (force || changed(x, y)); {
				c := next.At(x, y)
				if style == nil || !sameStyle(*style, *c) {
					b.WriteString(SGRWith(*c, o.Profile))
					cc := *c
					style = &cc
				}
				x, force = o.cluster(&b, c, x, next.W, !full)
			}
		}
	}
	b.WriteString("\x1b[0m")
	if next.CursorOn {
		fmt.Fprintf(&b, "\x1b[%d;%dH\x1b[?25h", next.CursorY+1, next.CursorX+1)
	} else {
		b.WriteString("\x1b[?25l")
	}
	return b.String()
}
