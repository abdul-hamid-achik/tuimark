// Package paint turns a laid-out Box tree into a cols×rows cell grid:
// backgrounds, borders, titles, and widget content. Parents paint first,
// children overwrite, modals paint last (SPEC §12).
package paint

import (
	"strings"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/layout"
)

// Attribute bits.
const (
	Bold uint8 = 1 << iota
	Dim
	Italic
	Underline
	Reverse
)

// Cell is one terminal cell.
type Cell struct {
	Ch    rune
	FG    css.Color
	BG    css.Color
	Attrs uint8
	Owner string // id of the nearest id-bearing box that painted this cell
}

// Grid is the painted frame.
type Grid struct {
	W, H  int
	Cells []Cell
	// Cursor is the input cursor position, when an input is focused.
	CursorX, CursorY int
	CursorOn         bool
}

// NewGrid returns a grid filled with spaces.
func NewGrid(w, h int) *Grid {
	g := &Grid{W: max(w, 0), H: max(h, 0)}
	g.Cells = make([]Cell, g.W*g.H)
	for i := range g.Cells {
		g.Cells[i].Ch = ' '
	}
	return g
}

// At returns the cell at (x, y).
func (g *Grid) At(x, y int) *Cell { return &g.Cells[y*g.W+x] }

// Lines returns each row as a string of exactly W runes.
func (g *Grid) Lines() []string {
	out := make([]string, g.H)
	var b strings.Builder
	for y := 0; y < g.H; y++ {
		b.Reset()
		for x := 0; x < g.W; x++ {
			b.WriteRune(g.Cells[y*g.W+x].Ch)
		}
		out[y] = b.String()
	}
	return out
}

type painter struct {
	g *Grid
}

func (p *painter) set(clip layout.Rect, x, y int, ch rune, st cellStyle) {
	if !clip.Contains(x, y) || x < 0 || y < 0 || x >= p.g.W || y >= p.g.H {
		return
	}
	if ch < 0x20 || (ch >= 0x7f && ch <= 0x9f) {
		ch = ' ' // the grid never carries control characters (SPEC §13.2)
	}
	c := p.g.At(x, y)
	c.Ch = ch
	if st.fg.IsSet() {
		c.FG = st.fg
	}
	if st.bg.IsSet() {
		c.BG = st.bg
	}
	c.Attrs = st.attrs
	if st.owner != "" || st.clearOwner {
		c.Owner = st.owner
	}
}

// visible is the part of r inside clip and the grid: the only cells paint
// ever touches. Loops walk it instead of a box's logical rect, so the cost
// of a frame is bounded by cols×rows whatever size layout produced.
func (p *painter) visible(r, clip layout.Rect) layout.Rect {
	return r.Intersect(clip).Intersect(layout.Rect{W: p.g.W, H: p.g.H})
}

func (p *painter) text(clip layout.Rect, x, y int, s string, st cellStyle) {
	for _, r := range s {
		p.set(clip, x, y, r, st)
		x++
	}
}

type cellStyle struct {
	fg, bg     css.Color
	attrs      uint8
	owner      string
	clearOwner bool
}

func attrsOf(s css.Style) uint8 {
	var a uint8
	if s.Bold {
		a |= Bold
	}
	if s.Dim {
		a |= Dim
	}
	if s.Italic {
		a |= Italic
	}
	if s.Underline {
		a |= Underline
	}
	if s.Reverse {
		a |= Reverse
	}
	return a
}

// Paint paints root and then the modal layer into a fresh grid.
func Paint(root *layout.Box, modals []*layout.Box, cols, rows int) *Grid {
	p := &painter{g: NewGrid(cols, rows)}
	p.box(root, "")
	for _, m := range modals {
		p.box(m, "")
	}
	return p.g
}

func ownerOf(b *layout.Box, inherited string) string {
	if b.ID != "" {
		return b.ID
	}
	return inherited
}

var borders = map[string][6]rune{
	// top-left, top-right, bottom-left, bottom-right, horizontal, vertical
	"single":  {'┌', '┐', '└', '┘', '─', '│'},
	"double":  {'╔', '╗', '╚', '╝', '═', '║'},
	"rounded": {'╭', '╮', '╰', '╯', '─', '│'},
	"thick":   {'┏', '┓', '┗', '┛', '━', '┃'},
}

func (p *painter) box(b *layout.Box, owner string) {
	if !b.Laid {
		return
	}
	owner = ownerOf(b, owner)
	st := b.Style
	visible := st.Visibility != "hidden"
	base := cellStyle{fg: st.Color, bg: st.Background, attrs: attrsOf(st), owner: owner, clearOwner: true}
	clip := b.Clip
	if visible {
		// Background (modals always clear what is underneath). A reversed
		// box fills its whole rect too, so a selected list item reads as a
		// bar rather than as reversed glyphs only.
		if st.Background.IsSet() || b.Modal || st.Reverse {
			fill := cellStyle{fg: st.Color, bg: st.Background, owner: owner, clearOwner: true}
			if st.Reverse {
				fill.attrs = Reverse
			}
			v := p.visible(b.Outer(), clip)
			for y := v.Y; y < v.Y+v.H; y++ {
				for x := v.X; x < v.X+v.W; x++ {
					p.set(clip, x, y, ' ', fill)
				}
			}
		} else if b.ID != "" {
			// Claim ownership of the box area without touching glyphs.
			v := p.visible(b.Outer(), clip)
			for y := v.Y; y < v.Y+v.H; y++ {
				for x := v.X; x < v.X+v.W; x++ {
					p.g.At(x, y).Owner = owner
				}
			}
		}
		p.border(b, owner)
		p.content(b, base)
	}
	for _, c := range b.Children {
		p.box(c, owner)
	}
}

func (p *painter) border(b *layout.Box, owner string) {
	set, ok := borders[b.Style.Border]
	if !ok || b.W < 2 || b.H < 2 {
		return
	}
	fg := b.Style.BorderColor
	if !fg.IsSet() {
		fg = b.Style.Color
	}
	st := cellStyle{fg: fg, bg: b.Style.Background, owner: owner, clearOwner: true}
	x0, y0, x1, y1 := b.X, b.Y, b.X+b.W-1, b.Y+b.H-1
	clip := b.Clip
	p.set(clip, x0, y0, set[0], st)
	p.set(clip, x1, y0, set[1], st)
	p.set(clip, x0, y1, set[2], st)
	p.set(clip, x1, y1, set[3], st)
	v := p.visible(b.Outer(), clip)
	for x := max(x0+1, v.X); x < min(x1, v.X+v.W); x++ {
		p.set(clip, x, y0, set[4], st)
		p.set(clip, x, y1, set[4], st)
	}
	for y := max(y0+1, v.Y); y < min(y1, v.Y+v.H); y++ {
		p.set(clip, x0, y, set[5], st)
		p.set(clip, x1, y, set[5], st)
	}
	if b.Title != "" && b.W >= 5 {
		room := b.W - 4 // corner, lead rule, ... , corner
		t := " " + b.Title + " "
		if layout.Width(t) > room {
			t = layout.Truncate(t, room)
		}
		tst := st
		if b.Style.TitleColor.IsSet() {
			tst.fg = b.Style.TitleColor
		}
		p.text(clip, x0+2, y0, t, tst)
	}
}

func alignOffset(mode string, room, w int) int {
	switch mode {
	case "center":
		return max(0, (room-w)/2)
	case "end":
		return max(0, room-w)
	}
	return 0
}

func (p *painter) content(b *layout.Box, st cellStyle) {
	c := b.Content
	clip := b.Clip.Intersect(c)
	switch b.Kind {
	case "text":
		lines := layout.Lines(b.Text, c.W, b.Style.Wrap)
		for i, line := range lines {
			if i >= c.H {
				break
			}
			line = layout.Cut(line, c.W)
			x := c.X + alignOffset(b.Style.ContentAlign, c.W, layout.Width(line))
			p.text(clip, x, c.Y+i, line, st)
		}
	case "button":
		label := layout.Cut(layout.ButtonLabel(b), c.W)
		x := c.X + alignOffset(b.Style.ContentAlign, c.W, layout.Width(label))
		p.text(clip, x, c.Y, label, st)
	case "input":
		p.input(b, clip, st)
	case "progress":
		if c.W <= 0 || c.H <= 0 {
			return
		}
		v := b.Value
		if v < 0 {
			v = 0
		}
		if v > 100 {
			v = 100
		}
		filled := int(float64(c.W)*v/100 + 0.5)
		vis := p.visible(layout.Rect{X: c.X, Y: c.Y, W: c.W, H: 1}, clip)
		for x := vis.X - c.X; x < vis.X-c.X+vis.W; x++ {
			if x < filled {
				p.set(clip, c.X+x, c.Y, '█', st)
			} else {
				dim := st
				dim.attrs |= Dim
				p.set(clip, c.X+x, c.Y, '░', dim)
			}
		}
	case "rule":
		o := b.Outer()
		v := p.visible(o, b.Clip)
		if b.Axis == "y" {
			for y := v.Y; y < v.Y+v.H; y++ {
				p.set(b.Clip, o.X, y, '│', st)
			}
		} else {
			for x := v.X; x < v.X+v.W; x++ {
				p.set(b.Clip, x, o.Y, '─', st)
			}
		}
	}
}

func (p *painter) input(b *layout.Box, clip layout.Rect, st cellStyle) {
	c := b.Content
	if c.W <= 0 || c.H <= 0 {
		return
	}
	runes := []rune(b.Text)
	if b.Secret {
		for i := range runes {
			runes[i] = '•'
		}
	}
	cursor := min(max(b.Cursor, 0), len(runes))
	if len(runes) == 0 && b.Placeholder != "" {
		ph := st
		ph.attrs |= Dim
		p.text(clip, c.X, c.Y, layout.Cut(b.Placeholder, c.W), ph)
	} else {
		// Keep the cursor visible: show the tail when the value is long.
		start := 0
		if cursor >= c.W {
			start = cursor - c.W + 1
		}
		end := min(len(runes), start+c.W)
		p.text(clip, c.X, c.Y, string(runes[start:end]), st)
		cursor -= start
	}
	if b.Focused {
		cx := c.X + min(cursor, c.W-1)
		if len(runes) == 0 {
			cx = c.X
		}
		if clip.Contains(cx, c.Y) {
			p.g.CursorX, p.g.CursorY, p.g.CursorOn = cx, c.Y, true
		}
	}
}
