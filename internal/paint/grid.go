// Package paint turns a laid-out Box tree into a cols×rows cell grid:
// backgrounds, borders, titles, and widget content. Parents paint first,
// children overwrite, modals paint last (SPEC §12).
package paint

import (
	"math"
	"math/bits"
	"strings"
	"unicode/utf8"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/layout"
	"github.com/abdul-hamid-achik/tuimark/internal/uniwidth"
)

// Attribute bits.
const (
	Bold uint8 = 1 << iota
	Dim
	Italic
	Underline
	Reverse
)

// Cell is one terminal column (SPEC v0.2 §12.1). A lead cell holds one
// painted grapheme cluster, 1 or 2 columns wide: Ch is its first code point
// and Ext the code points after it ("" for the usual one-code-point
// cluster), so Grapheme() is Ch followed by Ext. Wide marks a width-2
// cluster, whose second column is the next cell: a continuation (Cont),
// which holds no glyph of its own (Ch 0, Ext "") and carries its lead's
// style and owner. No cell is ever half of a wide cluster.
type Cell struct {
	Ch    rune
	Ext   string
	Wide  bool
	Cont  bool
	FG    css.Color
	BG    css.Color
	Attrs uint8
	Owner string // id of the nearest id-bearing box that painted this cell
}

// Grapheme returns the cluster a lead cell holds, or "" for a continuation.
func (c Cell) Grapheme() string {
	if c.Cont {
		return ""
	}
	if c.Ext == "" {
		return string(c.Ch)
	}
	return string(c.Ch) + c.Ext
}

// Width is the number of columns the cell's cluster takes: 2 for a wide
// lead, 0 for a continuation, 1 otherwise.
func (c Cell) Width() int {
	switch {
	case c.Cont:
		return 0
	case c.Wide:
		return 2
	}
	return 1
}

// Complex reports whether the cell leads a complex cluster (SPEC §11.5.1):
// width 2 or more than one code point. Complex clusters set the dump's
// "wide" flag and get CHA re-positioning in Diff and Full.
func (c Cell) Complex() bool { return !c.Cont && (c.Wide || c.Ext != "") }

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

// Lines returns each row as a string exactly W columns wide: the lead
// cells' clusters concatenated, a wide cluster written once, continuation
// cells contributing nothing (SPEC v0.2 MUST 6). In a painted grid, a row
// read as one string segments into exactly its lead cells' clusters (see
// unjoin), so its width is W. For a grid of one-code-point width-1
// clusters, which is every ASCII and box-drawing frame, each row is
// exactly W runes, as in v0.1.
func (g *Grid) Lines() []string {
	out := make([]string, g.H)
	var b strings.Builder
	for y := 0; y < g.H; y++ {
		b.Reset()
		for _, c := range g.Cells[y*g.W : (y+1)*g.W] {
			if c.Cont {
				continue
			}
			b.WriteRune(c.Ch)
			b.WriteString(c.Ext)
		}
		out[y] = b.String()
	}
	return out
}

// HasComplex reports whether some lead cell holds a complex cluster (SPEC
// §13.2: the dump's "wide" flag).
func (g *Grid) HasComplex() bool {
	for i := range g.Cells {
		if g.Cells[i].Complex() {
			return true
		}
	}
	return false
}

type painter struct {
	g *Grid
}

// inside reports whether (x, y) is inside both clip and the grid.
func (p *painter) inside(clip layout.Rect, x, y int) bool {
	return clip.Contains(x, y) && x >= 0 && y >= 0 && x < p.g.W && y < p.g.H
}

func (p *painter) set(clip layout.Rect, x, y int, ch rune, st cellStyle) {
	p.put(clip, x, y, ch, "", 1, st)
}

// put paints one grapheme cluster (first code point r, the rest ext) of
// width w at (x, y), by the rules of SPEC v0.2 §12.1: a width-0 cluster is
// never painted; a cluster is painted only when all its columns are inside
// clip and the grid; a width-2 cluster whose second column falls outside
// paints a space, with the same style, in its first column.
func (p *painter) put(clip layout.Rect, x, y int, r rune, ext string, w int, st cellStyle) {
	if w <= 0 || !p.inside(clip, x, y) {
		return
	}
	if w == 2 && !p.inside(clip, x+1, y) {
		r, ext, w = ' ', "", 1
	}
	if r < 0x20 || (r >= 0x7f && r <= 0x9f) {
		r, ext = ' ', "" // the grid never carries control characters (SPEC §13.2)
	}
	if ext != "" && !utf8.ValidString(ext) {
		ext = strings.ToValidUTF8(ext, string(utf8.RuneError))
	}
	p.unlink(x, y)
	if w == 2 {
		p.unlink(x+1, y)
	}
	c := p.g.At(x, y)
	c.Ch, c.Ext, c.Wide, c.Cont = r, ext, w == 2, false
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
	if w == 2 {
		*p.g.At(x+1, y) = Cell{Cont: true, FG: c.FG, BG: c.BG, Attrs: c.Attrs, Owner: c.Owner}
	}
}

// unlink breaks the wide cluster that (x, y) is half of, if any, before
// (x, y) is written: the other half becomes a space and keeps its style,
// so no cell is ever left as half of a wide cluster (SPEC v0.2 §12.1).
func (p *painter) unlink(x, y int) {
	c := p.g.At(x, y)
	switch {
	case c.Cont:
		if x > 0 {
			l := p.g.At(x-1, y)
			l.Ch, l.Ext, l.Wide = ' ', "", false
		}
		c.Ch, c.Cont = ' ', false
	case c.Wide:
		if x+1 < p.g.W {
			n := p.g.At(x+1, y)
			n.Ch, n.Ext, n.Cont = ' ', "", false
		}
		c.Wide = false
	}
}

// settle gives every continuation cell its lead's style and owner again:
// claiming ownership of a box area (which writes no glyph) may have split
// the two halves of a wide cluster between owners.
func (p *painter) settle() {
	for y := 0; y < p.g.H; y++ {
		row := p.g.Cells[y*p.g.W : (y+1)*p.g.W]
		for x := 1; x < len(row); x++ {
			if row[x].Cont {
				l := row[x-1]
				row[x].FG, row[x].BG, row[x].Attrs, row[x].Owner = l.FG, l.BG, l.Attrs, l.Owner
			}
		}
	}
}

// unjoin makes every row, read as one string, segment into exactly its
// lead cells' clusters (SPEC v0.2 §12.1, MUST 6). Each string is
// segmented on its own (§11.5.1), so clusters painted side by side from
// different strings could join when the row is read whole, in a dump or
// by the terminal: a regional indicator after a lone one, a Hangul vowel
// after a leading jamo, a pictograph after a trailing ZWJ, a mark after
// anything, anything after a Prepend. Scanning a row from the left, the
// first lead whose cluster joins the one before it becomes a space, with
// its style and owner (a width-2 lead becomes two spaces); when the lead
// before it ends with a Prepend code point, which joins whatever follows,
// that lead becomes a space instead. This repeats until the row reads as
// its cells. Widths do not change, so layout is unaffected.
func (p *painter) unjoin() {
	for y := 0; y < p.g.H; y++ {
		row := p.g.Cells[y*p.g.W : (y+1)*p.g.W]
		if !mayJoin(row) {
			continue
		}
		// Each pass turns a lead that is not a space into a space, so
		// there are at most len(row) passes.
		for n := 0; n <= len(row); n++ {
			x := firstJoin(row)
			if x < 0 {
				break
			}
			c := &row[x]
			if c.Wide && x+1 < len(row) {
				r := &row[x+1]
				r.Ch, r.Ext, r.Cont = ' ', "", false
			}
			c.Ch, c.Ext, c.Wide = ' ', "", false
		}
	}
}

// mayJoin reports whether a lead cell of row holds a joiner code point
// (uniwidth.Joiner), without which the row reads as its cells.
func mayJoin(row []Cell) bool {
	for i := range row {
		if c := &row[i]; !c.Cont && (c.Ext != "" || uniwidth.Joiner(c.Ch)) {
			return true
		}
	}
	return false
}

// firstJoin reads row as one string and returns the column of the lead
// cell unjoin replaces at the first place where it does not segment into
// the row's lead cells' clusters, or -1 when it does.
func firstJoin(row []Cell) int {
	var b strings.Builder
	var cols, ends []int // each lead's column and the byte offset after it
	for x := range row {
		if c := &row[x]; !c.Cont {
			b.WriteRune(c.Ch)
			b.WriteString(c.Ext)
			cols, ends = append(cols, x), append(ends, b.Len())
		}
	}
	s := b.String()
	i, off, at := 0, 0, -1
	uniwidth.Each(s, func(c string, _ int) bool {
		start := off
		off += len(c)
		switch {
		case off == ends[i]:
			i++
			return true
		case off < ends[i]:
			at = cols[i] // the lead is not one cluster in its row
		case uniwidth.Count(s[start:ends[i]]+" ") == 1:
			at = cols[i] // it ends with a Prepend (UAX #29 GB9b)
		default:
			at = cols[i+1] // the next lead joins it
		}
		return false
	})
	return at
}

// visible is the part of r inside clip and the grid: the only cells paint
// ever touches. Loops walk it instead of a box's logical rect, so the cost
// of a frame is bounded by cols×rows whatever size layout produced.
func (p *painter) visible(r, clip layout.Rect) layout.Rect {
	return r.Intersect(clip).Intersect(layout.Rect{W: p.g.W, H: p.g.H})
}

// text paints s cluster by cluster from (x, y). Width-0 clusters occupy no
// column; clusters past the right edge of clip are not visited.
func (p *painter) text(clip layout.Rect, x, y int, s string, st cellStyle) {
	if y < clip.Y || y >= clip.Y+clip.H || y < 0 || y >= p.g.H {
		return
	}
	right := min(clip.X+clip.W, p.g.W)
	uniwidth.Each(s, func(c string, w int) bool {
		if x >= right {
			return false
		}
		if w > 0 {
			r, n := utf8.DecodeRuneInString(c)
			p.put(clip, x, y, r, c[n:], w, st)
			x += w
		}
		return true
	})
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
	p.settle()
	p.unjoin()
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
	if b == nil || !b.Laid {
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
		p.scrollbar(b, owner)
		p.content(b, base)
		p.mark(b, base)
	}
	for _, c := range b.Children {
		p.box(c, owner)
	}
	if visible && b.Kind == "table" {
		p.placeholder(b, base)
	}
}

// borderStyle is the style of b's border cells: its border-color, else
// its color, on its background.
func borderStyle(b *layout.Box, owner string) cellStyle {
	fg := b.Style.BorderColor
	if !fg.IsSet() {
		fg = b.Style.Color
	}
	return cellStyle{fg: fg, bg: b.Style.Background, owner: owner, clearOwner: true}
}

func (p *painter) border(b *layout.Box, owner string) {
	set, ok := borders[b.Style.Border]
	if !ok || b.W < 2 || b.H < 2 || b.Fixed {
		// A table's header cells, rows, and body cells ignore their own
		// border (SPEC §6.9.3).
		return
	}
	st := borderStyle(b, owner)
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

// thumbs are the scrollbar glyphs per border style (SPEC §12.3): a ┃ would
// not show on a thick border, so double and thick borders get █.
var thumbs = map[string]rune{"single": '┃', "rounded": '┃', "double": '█', "thick": '█'}

// scrollbar paints the thumb of a viewport with scrollbar: auto over its
// right border (SPEC §12.3), when it scrolls on y, its content is taller
// than its viewport, it has a border, and its outer height h is at least
// 3. With track = h − 2, view the viewport's content-box height, content
// its content extent on y (for a table, its body viewport V and its n
// rows), and offset its offset:
//
//	length = max(1, floor(track * view / content))
//	pos    = floor((track − length) * offset / (content − view))
//
// the thumb covers rows y+1+pos to y+pos+length of the right border, in
// the border's style, clipped like the border, owned like it. It never
// takes layout space.
func (p *painter) scrollbar(b *layout.Box, owner string) {
	glyph, ok := thumbs[b.Style.Border]
	if !ok || b.Style.Scrollbar != "auto" || b.H < 3 || b.W < 1 {
		return
	}
	if _, sy := b.ScrollAxes(); !sy {
		return
	}
	view, content := b.Content.H, b.ContentH
	if b.Kind == "table" {
		view, content = b.View, b.Rows
	}
	if content <= view || view < 0 {
		return
	}
	track := b.H - 2
	length := max(1, mulDiv(track, view, content))
	pos := mulDiv(track-length, min(max(b.ScrollY, 0), content-view), content-view)
	st := borderStyle(b, owner)
	x := b.X + b.W - 1
	for y := b.Y + 1 + pos; y <= b.Y+pos+length; y++ {
		p.set(b.Clip, x, y, glyph, st)
	}
}

// mulDiv returns floor(a * b / c) for a, b ≥ 0 and c > 0 whose quotient
// is at most a (b ≤ c), without overflowing the product.
func mulDiv(a, b, c int) int {
	if a <= 0 || b <= 0 || c <= 0 {
		return 0
	}
	hi, lo := bits.Mul64(uint64(a), uint64(b))
	q, _ := bits.Div64(hi, lo, uint64(c))
	return int(q)
}

// placeholder paints a table's placeholder when it has no rows (SPEC
// §6.9.4): the text, resolved in the table's scope and cut to the content
// width W with Truncate, dim, on the first body row, at content x +
// floor((W − width(text)) / 2). Nothing is painted when the body viewport
// has no row. It creates no node.
func (p *painter) placeholder(t *layout.Box, st cellStyle) {
	c := t.Content
	if t.Rows > 0 || t.Placeholder == "" || t.View <= 0 || c.W <= 0 {
		return
	}
	text := layout.Truncate(t.Placeholder, c.W)
	st.attrs |= Dim
	x := c.X + max(0, (c.W-layout.Width(text))/2)
	p.text(t.Clip.Intersect(c), x, c.Y+t.Header, text, st)
}

// eighths are the partial blocks of bar: eighths, 1/8 to 7/8 of a cell
// (SPEC §12.4).
var eighths = [8]rune{' ', '▏', '▎', '▍', '▌', '▋', '▊', '▉'}

// mark paints the mark of a checked row in its mark channel (SPEC §6.14):
// the first width(mark) columns of the channel, on the first content row,
// in the row's style (so a reverse selection covers it). The channel's
// last column stays blank.
func (p *painter) mark(b *layout.Box, st cellStyle) {
	if !b.Checked || b.Chan <= 0 || b.Mark == "" || b.Content.H <= 0 {
		return
	}
	ch := layout.Rect{X: b.Content.X - b.Chan, Y: b.Content.Y, W: b.Chan, H: 1}
	p.text(b.Clip.Intersect(ch), ch.X, ch.Y, b.Mark, st)
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
	case "text", "column":
		// Each line is painted whole from its aligned origin and clipped at
		// the content box, cluster by cluster (SPEC v0.2 §11.5.2, §12.1): a
		// wide cluster straddling the right edge paints a space. A table's
		// header cell (its column, whose text is its resolved title) and
		// body cells never wrap: wrap: wrap paints as truncate (§6.9.4).
		mode := b.Style.Wrap
		if b.Fixed && mode == "wrap" {
			mode = "truncate"
		}
		lines := layout.Lines(b.Text, c.W, mode)
		for i, line := range lines {
			if i >= c.H {
				break
			}
			x := c.X + alignOffset(b.Style.ContentAlign, c.W, layout.Width(line))
			p.text(clip, x, c.Y+i, line, st)
		}
	case "button":
		label := layout.ButtonLabel(b)
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
		// bar: block (the default, v1): int(W·v/100 + 0.5) full cells.
		// bar: eighths (SPEC §12.4): e = floor(W·8·v/100 + 0.5) eighths,
		// in double precision and in this order: e div 8 full cells, then
		// the partial glyph e mod 8 when it is not 0, in the node's style.
		// The rest is ░ with dim either way; only the first row is painted.
		filled, part := int(float64(c.W)*v/100+0.5), 0
		if b.Style.Bar == "eighths" {
			e := int(math.Floor(float64(c.W)*8*v/100 + 0.5))
			filled, part = e/8, e%8
		}
		vis := p.visible(layout.Rect{X: c.X, Y: c.Y, W: c.W, H: 1}, clip)
		for x := vis.X - c.X; x < vis.X-c.X+vis.W; x++ {
			if x < filled {
				p.set(clip, c.X+x, c.Y, '█', st)
			} else if x == filled && part > 0 {
				p.set(clip, c.X+x, c.Y, eighths[part], st)
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

// input paints an input's value, or its dim placeholder, and places the
// cursor (SPEC v0.2 §12.1). Box.Cursor counts code points; a cursor inside
// a cluster stands after that cluster. With W the content width and p the
// width of the clusters before the cursor, the painted window starts at the
// first cluster when p < W, else at the first cluster s such that the
// clusters from s up to the cursor are at most W-1 columns; clusters are
// painted from there while they fit. A secret input shows one • per
// cluster. For ASCII values this is the v0.1 rule, one rune per cell.
func (p *painter) input(b *layout.Box, clip layout.Rect, st cellStyle) {
	c := b.Content
	if c.W <= 0 || c.H <= 0 {
		return
	}
	type cluster struct {
		s    string
		w, n int // width, code points
	}
	var cl []cluster
	uniwidth.Each(b.Text, func(s string, w int) bool {
		if b.Secret {
			cl = append(cl, cluster{"•", 1, utf8.RuneCountInString(s)})
		} else {
			cl = append(cl, cluster{s, w, utf8.RuneCountInString(s)})
		}
		return true
	})
	// k is the number of clusters before the cursor, pw their width.
	k, pw := 0, 0
	for runes := max(b.Cursor, 0); k < len(cl) && runes > 0; k++ {
		runes -= cl[k].n
		pw += cl[k].w
	}
	start, before := 0, pw // before: width of the clusters from start to the cursor
	if pw >= c.W {
		for start < k && before > c.W-1 {
			before -= cl[start].w
			start++
		}
	}
	if len(cl) == 0 && b.Placeholder != "" {
		ph := st
		ph.attrs |= Dim
		p.text(clip, c.X, c.Y, layout.Cut(b.Placeholder, c.W), ph)
	} else {
		x, used := c.X, 0
		for _, cc := range cl[start:] {
			if used+cc.w > c.W {
				break
			}
			if cc.w > 0 {
				r, n := utf8.DecodeRuneInString(cc.s)
				p.put(clip, x, c.Y, r, cc.s[n:], cc.w, st)
			}
			x += cc.w
			used += cc.w
		}
	}
	if b.Focused {
		cx := c.X + min(before, c.W-1)
		if len(cl) == 0 {
			cx = c.X
		}
		if clip.Contains(cx, c.Y) {
			p.g.CursorX, p.g.CursorY, p.g.CursorOn = cx, c.Y, true
		}
	}
}
