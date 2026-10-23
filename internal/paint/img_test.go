package paint

import (
	"image"
	"image/color"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/layout"
)

// halfImg builds a w×h px source whose top half is one color and bottom
// half another: the two rows of pixels a halfblock cell reads.
func halfImg(w, h int, top, bottom color.RGBA) *image.RGBA {
	im := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		c := top
		if y >= h/2 {
			c = bottom
		}
		for x := range w {
			im.SetRGBA(x, y, c)
		}
	}
	return im
}

// An <img> paints one ▀ (U+2580) per cell, its foreground the upper
// pixel's color and its background the lower pixel's, in truecolor RGB
// (v0.4d phase 2b). A 2×1-cell image from a 4×4 px source resized to 2×2
// px gives exactly two cells.
func TestImgPaintsHalfblocks(t *testing.T) {
	red, blue := color.RGBA{R: 255, A: 255}, color.RGBA{B: 255, A: 255}
	im := halfImg(4, 4, red, blue)
	b := bx("img", "width: 2; height: 1")
	b.Img, b.ImgW, b.ImgH = im, 4, 4
	g := render(t, bx("col", "", b), 2, 1)
	for x := range 2 {
		c := g.At(x, 0)
		if c.Ch != '▀' || c.Ext != "" || c.Cont || c.Wide {
			t.Errorf("cell %d = %q ext %q cont %v wide %v, want a plain ▀", x, c.Ch, c.Ext, c.Cont, c.Wide)
		}
		if c.FG != (css.Color{Kind: css.ColorRGB, R: 255}) {
			t.Errorf("cell %d fg = %v, want rgb(255,0,0)", x, c.FG)
		}
		if c.BG != (css.Color{Kind: css.ColorRGB, B: 255}) {
			t.Errorf("cell %d bg = %v, want rgb(0,0,255)", x, c.BG)
		}
	}
}

// A box whose source did not decode paints nothing, and neither does a
// zero-sized one; the owner still claims the area as any box does.
func TestImgUndecodedPaintsNothing(t *testing.T) {
	b := bx("img", "width: 4; height: 1")
	g := render(t, bx("col", "", b), 4, 1)
	for x := range 4 {
		if c := g.At(x, 0); c.Ch != ' ' || c.FG.IsSet() || c.BG.IsSet() {
			t.Errorf("cell %d = %q fg %v bg %v, want a blank cell", x, c.Ch, c.FG, c.BG)
		}
	}
}

// An img is clipped like any other box: only the visible cells of the
// content box are painted.
func TestImgClipped(t *testing.T) {
	red, blue := color.RGBA{R: 255, A: 255}, color.RGBA{B: 255, A: 255}
	im := halfImg(8, 8, red, blue)
	b := bx("img", "width: 4; height: 1")
	b.Img, b.ImgW, b.ImgH = im, 8, 8
	// A 2-column grid shows only the img's first two cells.
	g := render(t, bx("col", "", b), 2, 1)
	for x := range 2 {
		if c := g.At(x, 0); c.Ch != '▀' {
			t.Errorf("cell %d = %q, want ▀", x, c.Ch)
		}
	}
}

var _ = layout.Width // (the helpers above build boxes, not text)
