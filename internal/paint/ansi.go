package paint

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
)

func colorCode(c css.Color, bg bool) string {
	switch c.Kind {
	case css.ColorANSI:
		base := 30
		if bg {
			base = 40
		}
		if c.Index >= 8 {
			return strconv.Itoa(base + 60 + int(c.Index) - 8)
		}
		return strconv.Itoa(base + int(c.Index))
	case css.ColorRGB:
		if bg {
			return fmt.Sprintf("48;2;%d;%d;%d", c.R, c.G, c.B)
		}
		return fmt.Sprintf("38;2;%d;%d;%d", c.R, c.G, c.B)
	}
	if bg {
		return "49"
	}
	return "39"
}

// SGR returns the escape sequence that selects c's style from a reset state.
func SGR(c Cell) string {
	parts := []string{"0"}
	if c.Attrs&Bold != 0 {
		parts = append(parts, "1")
	}
	if c.Attrs&Dim != 0 {
		parts = append(parts, "2")
	}
	if c.Attrs&Italic != 0 {
		parts = append(parts, "3")
	}
	if c.Attrs&Underline != 0 {
		parts = append(parts, "4")
	}
	if c.Attrs&Reverse != 0 {
		parts = append(parts, "7")
	}
	parts = append(parts, colorCode(c.FG, false), colorCode(c.BG, true))
	return "\x1b[" + strings.Join(parts, ";") + "m"
}

func sameStyle(a, b Cell) bool {
	return a.FG == b.FG && a.BG == b.BG && a.Attrs == b.Attrs
}

// Full renders the whole grid as ANSI lines (for `preview` on a TTY).
func Full(g *Grid) string {
	var b strings.Builder
	for y := 0; y < g.H; y++ {
		var last *Cell
		for x := 0; x < g.W; x++ {
			c := g.At(x, y)
			if last == nil || !sameStyle(*last, *c) {
				b.WriteString(SGR(*c))
			}
			b.WriteRune(c.Ch)
			last = c
		}
		b.WriteString("\x1b[0m\n")
	}
	return b.String()
}

// Diff renders the changes from prev to next with absolute cursor moves.
// A nil or differently sized prev repaints everything.
func Diff(prev, next *Grid) string {
	var b strings.Builder
	full := prev == nil || prev.W != next.W || prev.H != next.H
	if full {
		b.WriteString("\x1b[0m\x1b[2J")
	}
	var style *Cell
	for y := 0; y < next.H; y++ {
		x := 0
		for x < next.W {
			if !full && *prev.At(x, y) == *next.At(x, y) {
				x++
				continue
			}
			fmt.Fprintf(&b, "\x1b[%d;%dH", y+1, x+1)
			for x < next.W && (full || *prev.At(x, y) != *next.At(x, y)) {
				c := next.At(x, y)
				if style == nil || !sameStyle(*style, *c) {
					b.WriteString(SGR(*c))
					cc := *c
					style = &cc
				}
				b.WriteRune(c.Ch)
				x++
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
