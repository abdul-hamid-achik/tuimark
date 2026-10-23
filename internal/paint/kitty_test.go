package paint

import (
	"image/color"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
)

// Diacritics encodes 0-9 as one mark each (kitty's rowcolumn-diacritics
// table order: 0 → U+0305, 1 → U+030D, 2 → U+030E, … 9 → U+034A) and a
// larger number as its decimal digits, most significant first (v0.4d
// phase 3).
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
		{10, "\u030d\u0305"},
		{23, "\u030e\u0310"},
		{100, "\u030d\u0305\u0305"},
		{8025, "\u0346\u0305\u030e\u033d"},
	} {
		if got := Diacritics(c.n); got != c.want {
			t.Errorf("Diacritics(%d) = %q, want %q", c.n, got, c.want)
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
