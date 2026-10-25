package paint

import (
	"image/color"
	"strconv"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/layout"
)

// Diacritics encodes one row or column number as ONE mark of kitty's
// rowcolumn-diacritics table, indexed by the number: 0 → U+0305, 1 →
// U+030D, … 9 → U+034A, 10 → U+034B, 11 → U+034C, 12 → U+0350 (v0.5.2: the
// 0.5 code spelled a number of 10 or more digit by digit, so row 12 named
// rows 1 and 2 and an image of 10+ rows placed wrong).
func TestDiacritics(t *testing.T) {
	for _, c := range []struct {
		n    int
		want string
	}{
		{0, "\u0305"},
		{1, "\u030d"},
		{2, "\u030e"},
		{3, "\u0310"},
		{9, "\u034a"},
		{10, "\u034b"},
		{11, "\u034c"},
		{12, "\u0350"},
		{296, "\U0001d244"},
		{297, ""}, // past the table: no mark, not a silent clamp
		{1000, ""},
		{-1, ""},
	} {
		if got := Diacritics(c.n); got != c.want {
			t.Errorf("Diacritics(%d) = %+q, want %+q", c.n, got, c.want)
		}
	}
}

// The table is kitty's: 297 entries in file order, every one a
// non-spacing combining mark, strictly increasing (the file is sorted),
// with the entries the protocol documents at both ends.
func TestRowColumnDiacriticsTable(t *testing.T) {
	if len(rowColumnDiacritics) != 297 {
		t.Fatalf("table has %d entries, want 297", len(rowColumnDiacritics))
	}
	for i, r := range rowColumnDiacritics {
		if !unicode.Is(unicode.Mn, r) {
			t.Errorf("entry %d U+%04X is not a non-spacing mark", i, r)
		}
		if i > 0 && r <= rowColumnDiacritics[i-1] {
			t.Errorf("entry %d U+%04X does not follow U+%04X", i, r, rowColumnDiacritics[i-1])
		}
	}
	for i, want := range map[int]rune{0: 0x0305, 9: 0x034A, 10: 0x034B, 11: 0x034C, 12: 0x0350, 13: 0x0351, 14: 0x0352, 296: 0x1D244} {
		if rowColumnDiacritics[i] != want {
			t.Errorf("entry %d = U+%04X, want U+%04X", i, rowColumnDiacritics[i], want)
		}
	}
}

// A photo-mode <img> 12 rows tall names each row with ONE mark: row 10 is
// U+034B, row 11 U+034C, not the digit pairs of rows 1 and 0/1.
func TestImgPlaceholdersTallImage(t *testing.T) {
	b := bx("img", "width: 12; height: 12")
	b.Img, b.ImgW, b.ImgH = halfImg(6, 6, color.RGBA{R: 255, A: 255}, color.RGBA{B: 255, A: 255}), 6, 6
	b.ImgSrc, b.ImgPhoto = "img.png", true
	g := render(t, bx("col", "", b), 12, 12)
	for y := range 12 {
		want := Diacritics(y) + Diacritics(0)
		if got := g.At(0, y).Ext; got != want {
			t.Errorf("row %d first cell ext = %+q, want %+q", y, got, want)
		}
		if n := utf8.RuneCountInString(g.At(0, y).Ext); n != 2 {
			t.Errorf("row %d first cell carries %d marks, want 2 (row, column)", y, n)
		}
	}
	if got := g.At(0, 10).Ext; got != "\u034b\u0305" {
		t.Errorf("row 10 ext = %+q, want U+034B U+0305", got)
	}
	if got := g.At(0, 11).Ext; got != "\u034c\u0305" {
		t.Errorf("row 11 ext = %+q, want U+034C U+0305", got)
	}
}

// A photo-mode <img> 20 columns wide: column 0 names row and column, the
// cells to its right inherit (kitty's left-inheritance rule). Scrolled 12
// columns to the left, the first visible cell has no neighbour to inherit
// from, so it names its own row and column (by table index).
func TestImgPlaceholdersWideAndClipped(t *testing.T) {
	mk := func() *layout.Box {
		b := bx("img", "width: 20; height: 1")
		b.Img, b.ImgW, b.ImgH = halfImg(6, 6, color.RGBA{R: 255, A: 255}, color.RGBA{B: 255, A: 255}), 6, 6
		b.ImgSrc, b.ImgPhoto = "img.png", true
		return b
	}
	g := render(t, bx("col", "", mk()), 20, 1)
	if got := g.At(0, 0).Ext; got != Diacritics(0)+Diacritics(0) {
		t.Errorf("column 0 ext = %+q", got)
	}
	for x := 1; x < 20; x++ {
		if got := g.At(x, 0).Ext; got != "" {
			t.Errorf("column %d ext = %+q, want none (left inheritance)", x, got)
		}
	}
	s := bx("scroll#s", "width: 5; height: 1", mk())
	s.Axis, s.ScrollX = "x", 12
	g = render(t, bx("col", "", s), 5, 1)
	if got, want := g.At(0, 0).Ext, Diacritics(0)+Diacritics(12); got != want {
		t.Errorf("first visible cell (column 12) ext = %+q, want %+q\n%s", got, want, lines(g))
	}
	if got := g.At(1, 0).Ext; got != "" {
		t.Errorf("second visible cell ext = %+q, want none", got)
	}
}

// Kitty's table names rows and columns 0 to 296 (MaxPlaceholderCells).
// A photo <img> of 299 rows paints placeholders in rows 0 to 296, each
// naming its own row, and leaves rows 297 and 298 blank: no mark can name
// them. The same holds for columns, and for an image scrolled so that the
// rows past the table are the visible ones.
func TestImgPlaceholdersStopAtTheTable(t *testing.T) {
	mk := func(w, h int) *layout.Box {
		b := bx("img", "width: "+strconv.Itoa(w)+"; height: "+strconv.Itoa(h))
		b.Img, b.ImgW, b.ImgH = halfImg(6, 6, color.RGBA{R: 255, A: 255}, color.RGBA{B: 255, A: 255}), 6, 6
		b.ImgSrc, b.ImgPhoto = "img.png", true
		return b
	}
	g := render(t, bx("col", "", mk(1, 299)), 1, 299)
	for y := range 299 {
		c := g.At(0, y)
		if y < MaxPlaceholderCells {
			if want := Diacritics(y) + Diacritics(0); c.Ch != Placeholder || c.Ext != want {
				t.Errorf("row %d = %q ext %+q, want a placeholder with %+q", y, c.Ch, c.Ext, want)
			}
		} else if c.Ch != ' ' || c.Ext != "" {
			t.Errorf("row %d = %q ext %+q, want blank (past the table)", y, c.Ch, c.Ext)
		}
	}
	g = render(t, bx("col", "", mk(299, 1)), 299, 1)
	for x := range 299 {
		c := g.At(x, 0)
		if x < MaxPlaceholderCells {
			if c.Ch != Placeholder {
				t.Errorf("column %d = %q, want a placeholder", x, c.Ch)
			}
		} else if c.Ch != ' ' {
			t.Errorf("column %d = %q, want blank (past the table)", x, c.Ch)
		}
	}
	s := bx("scroll#s", "width: 1; height: 10", mk(1, 300))
	s.Axis, s.ScrollY = "y", 290
	g = render(t, bx("col", "", s), 1, 10)
	for y := range 10 {
		c, row := g.At(0, y), 290+y
		if row < MaxPlaceholderCells {
			if want := Diacritics(row) + Diacritics(0); c.Ch != Placeholder || c.Ext != want {
				t.Errorf("scrolled row %d = %q ext %+q, want a placeholder with %+q\n%s", row, c.Ch, c.Ext, want, lines(g))
			}
		} else if c.Ch != ' ' {
			t.Errorf("scrolled row %d = %q, want blank (past the table)", row, c.Ch)
		}
	}
}

// KittyID is stable, never 0, always under 2^24 (it rides a truecolor
// foreground), and a new size class is a new id.
func TestKittyID(t *testing.T) {
	id := KittyID("img.png", 3, 2)
	if id != KittyID("img.png", 3, 2) {
		t.Error("KittyID is not stable")
	}
	if id <= 0 || id >= 1<<24 {
		t.Errorf("KittyID = %d, want 0 < id < 2^24", id)
	}
	if id == KittyID("img.png", 4, 2) || id == KittyID("other.png", 3, 2) {
		t.Errorf("KittyID collisions for a new size class or source: %d", id)
	}
}

// A photo-mode <img> paints one U+10EEEE placeholder per cell (v0.4d
// phase 3): the first column of each row carries the row diacritic and
// column 0's (kitty's left inheritance fills the rest), every cell's
// foreground is the image id's truecolor encoding, and the frame's Imgs
// side table names the image once.
func TestImgPaintsPlaceholders(t *testing.T) {
	b := bx("img", "width: 3; height: 2")
	b.Img, b.ImgW, b.ImgH = halfImg(6, 6, color.RGBA{R: 255, A: 255}, color.RGBA{B: 255, A: 255}), 6, 6
	b.ImgSrc, b.ImgPhoto = "img.png", true
	g := render(t, bx("col", "", b), 3, 2)
	id := KittyID("img.png", 3, 2)
	wantFG := css.Color{Kind: css.ColorRGB, R: uint8(id >> 16), G: uint8(id >> 8), B: uint8(id)}
	// The diacritics of the first cell of each row: the row's number and
	// column 0's.
	firstCol := [2]string{"\u0305\u0305", "\u030d\u0305"}
	for y := range 2 {
		for x := range 3 {
			c := g.At(x, y)
			if c.Ch != Placeholder || c.Wide || c.Cont {
				t.Errorf("cell %d,%d = %q wide %v cont %v, want a plain placeholder", x, y, c.Ch, c.Wide, c.Cont)
			}
			if want := firstCol[y]; (x == 0 && c.Ext != want) || (x > 0 && c.Ext != "") {
				t.Errorf("cell %d,%d ext = %q, want %q", x, y, c.Ext, want)
			}
			if c.FG != wantFG {
				t.Errorf("cell %d,%d fg = %v, want %v", x, y, c.FG, wantFG)
			}
		}
	}
	if len(g.Imgs) != 1 || g.Imgs[0] != (GridImage{ID: id, Src: "img.png", W: 3, H: 2}) {
		t.Errorf("Imgs = %+v, want one {%d img.png 3 2}", g.Imgs, id)
	}
	// The same image painted twice in a frame registers once.
	second := bx("img", "width: 3; height: 2")
	second.Img, second.ImgW, second.ImgH = b.Img, 6, 6
	second.ImgSrc, second.ImgPhoto = "img.png", true
	g = render(t, bx("row", "", b, second), 7, 2)
	if len(g.Imgs) != 1 {
		t.Errorf("Imgs = %+v, want the id registered once", g.Imgs)
	}
}

// The same box without ImgPhoto keeps the halfblocks of phase 2b.
func TestImgPhotoOffPaintsHalfblocks(t *testing.T) {
	b := bx("img", "width: 2; height: 1")
	b.Img, b.ImgW, b.ImgH = halfImg(4, 4, color.RGBA{R: 255, A: 255}, color.RGBA{B: 255, A: 255}), 4, 4
	b.ImgSrc = "img.png"
	g := render(t, bx("col", "", b), 2, 1)
	for x := range 2 {
		if c := g.At(x, 0); c.Ch != '▀' || c.FG.Kind != css.ColorRGB || c.BG.Kind != css.ColorRGB {
			t.Errorf("cell %d = %q fg %v bg %v, want a halfblock", x, c.Ch, c.FG, c.BG)
		}
	}
	if len(g.Imgs) != 0 {
		t.Errorf("Imgs = %+v, want none", g.Imgs)
	}
}

// layout is referenced (the painter signature) without further asserts.
