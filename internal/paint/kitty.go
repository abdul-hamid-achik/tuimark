package paint

import (
	"hash/fnv"
	"strconv"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/layout"
)

// The kitty graphics protocol's Unicode placeholders (v0.4d phase 3): an
// image transmitted once with a virtual placement (a=T,…,U=1,c,r) is then
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

// MaxPlaceholderCells is how many rows and columns kitty's diacritic table
// can name (the length of rowColumnDiacritics): the cells of a photo <img>
// past row or column 296 cannot be addressed and are not painted.
const MaxPlaceholderCells = len(rowColumnDiacritics)

// Diacritics encodes n (a row or column number of a placeholder image) as
// kitty's combining mark: ONE mark, entry n of rowColumnDiacritics (0 →
// U+0305, 1 → U+030D, … 9 → U+034A, 10 → U+034B), never the decimal
// digits of n. A number outside the table (negative, or past
// MaxPlaceholderCells-1) has no mark: "". The painter never asks for one.
func Diacritics(n int) string {
	if n < 0 || n >= MaxPlaceholderCells {
		return ""
	}
	return string(rowColumnDiacritics[n])
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
// truecolor foreground, and the row and column in combining diacritics
// (one mark each, kitty's table). Only the first visible cell of each
// row carries diacritics — the row's own number and its own column's
// (column 0 unless the image is clipped on the left) — because a
// placeholder cell inherits row, column, and id from the cell to its left
// when it carries none (kitty's left-inheritance rule); a bare
// placeholder cell one column on is the same row, the next column. Cells
// past kitty's table (MaxPlaceholderCells rows or columns) are not
// painted. The frame's Imgs side table records the image once for Run's
// one-time transmission.
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
	for y := vis.Y; y < vis.Y+vis.H && y-c.Y < MaxPlaceholderCells; y++ {
		row := y - c.Y
		for x := vis.X; x < vis.X+vis.W && x-c.X < MaxPlaceholderCells; x++ {
			s := st
			s.fg = fg
			ext := ""
			if x == vis.X {
				ext = Diacritics(row) + Diacritics(x-c.X)
			}
			p.put(clip, x, y, Placeholder, ext, 1, s)
		}
	}
}
