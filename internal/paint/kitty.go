package paint

import (
	"hash/fnv"
	"strconv"
	"strings"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/layout"
)

// The kitty graphics protocol's Unicode placeholders (v0.4d phase 3): an
// image transmitted once as a virtual placement (a=t,…,U=1,c,r) is then
// shown by ordinary text — a U+10EEEE placeholder cell whose foreground
// color carries the image id and whose combining diacritics name the row
// and column inside the image. The terminal scales the image to the
// rectangle the placeholder cells cover, so diffing, scrolling, and
// reflowing a photo is moving text. See
// sw.kovidgoyal.net/kitty/graphics-protocol §Unicode placeholders.

// Placeholder is the code point of a kitty image placeholder cell,
// U+10EEEE of the Supplementary Private Use Area-B: one column wide,
// like every cell of the grid.
const Placeholder = '\U0010EEEE'

// rowDiacritics are the combining marks of the digits 0-9, the first ten
// entries of kitty's gen/rowcolumn-diacritics.txt (Unicode 6.0 combining
// chars of class 230 that never decompose), in table order: 0 → U+0305
// COMBINING OVERLINE, 1 → U+030D, 2 → U+030E, … 9 → U+034A. A value of
// one digit is that one mark; a larger value composes its decimal digits,
// most significant first (kitty's table itself carries the same marks for
// its values 0-9, which is every row and column a cell names here
// digit-wise).
var rowDiacritics = [10]rune{
	0x0305, 0x030D, 0x030E, 0x0310, 0x0312,
	0x033D, 0x033E, 0x033F, 0x0346, 0x034A,
}

// Diacritics encodes n (a row or column number of a placeholder image)
// as kitty's combining marks: each decimal digit, most significant first,
// becomes its mark of rowDiacritics (0 → U+0305, 1 → U+030D, 2 → U+030E,
// so 23 → U+030E U+034A). Negative n never occurs; it encodes as 0.
func Diacritics(n int) string {
	if n < 0 {
		n = 0
	}
	if n < 10 {
		return string(rowDiacritics[n])
	}
	s := strconv.Itoa(n)
	var b strings.Builder
	for i := range len(s) {
		b.WriteRune(rowDiacritics[s[i]-'0'])
	}
	return b.String()
}

// KittyID is the 24-bit image id of a photo-mode <img>: a stable hash of
// its source and cell size, so every frame of a session names the same
// (src, W, H) the same id and the host transmits it once; a new size
// class is a new id. The id is carried as the placeholder cells'
// truecolor foreground (38;2;r;g;b of the id's three bytes, kitty's
// 24-bit id encoding), so it must fit 24 bits and never be 0.
func KittyID(src string, w, h int) int32 {
	k := fnv.New32a()
	k.Write([]byte(src))
	k.Write([]byte{0})
	k.Write([]byte(strconv.Itoa(w)))
	k.Write([]byte{0})
	k.Write([]byte(strconv.Itoa(h)))
	return int32(k.Sum32()&0xFFFFFF) | 1
}

// IDColor is the foreground color that carries id on a placeholder cell.
func IDColor(id int32) css.Color {
	return css.Color{Kind: css.ColorRGB, R: uint8(id >> 16), G: uint8(id >> 8), B: uint8(id)}
}

// imgPlaceholders paints a photo-mode <img> as kitty placeholder cells
// (v0.4d phase 3): one U+10EEEE per cell, the image id in every cell's
// truecolor foreground, and the row and column in combining diacritics.
// Only the first column of each row carries diacritics — the row's own
// number and column 0's — because a placeholder cell inherits row,
// column, and id from the cell to its left when it carries none (kitty's
// left-inheritance rule); a bare placeholder cell one column on is the
// same row, the next column. The frame's Imgs side table records the
// image once for Run's one-time transmission.
func (p *painter) imgPlaceholders(b *layout.Box, clip layout.Rect, st cellStyle) {
	c := b.Content
	id := KittyID(b.ImgSrc, c.W, c.H)
	fg := IDColor(id)
	known := false
	for _, k := range p.g.Imgs {
		if k.ID == id {
			known = true
			break
		}
	}
	if !known {
		p.g.Imgs = append(p.g.Imgs, GridImage{ID: id, Src: b.ImgSrc, W: c.W, H: c.H})
	}
	vis := p.visible(c, clip)
	for y := vis.Y; y < vis.Y+vis.H; y++ {
		row := y - c.Y
		for x := vis.X; x < vis.X+vis.W; x++ {
			s := st
			s.fg = fg
			ext := ""
			if x == c.X {
				ext = Diacritics(row) + Diacritics(0)
			}
			p.put(clip, x, y, Placeholder, ext, 1, s)
		}
	}
}
