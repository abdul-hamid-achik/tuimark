package layout

import (
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// imgBox builds an img box of pw×ph pixels (the decoded source's size).
func imgBox(decls string, pw, ph int) *Box {
	b := n("img", decls)
	b.ImgW, b.ImgH = pw, ph
	return b
}

// runCurrent styles with the current language's built-in sheet (an <img>
// exists only there, so its width/height: auto rule does) and lays out
// root.
func runCurrent(t *testing.T, root *Box, cols, rows int) *Engine {
	t.Helper()
	c := css.NewCascadeLang(nil, css.Env{Cols: cols, Rows: rows}, ir.LangCurrent)
	styleTree(c, root, nil)
	if len(c.Diags) != 0 {
		t.Fatalf("cascade: %v", c.Diags)
	}
	e := &Engine{File: "t.tui"}
	e.Layout(root, nil, cols, rows)
	return e
}

// The natural size of an <img> (v0.4d phase 2b): without width=/height=,
// 40 cells wide (clamped to the container's width), as tall as the
// image's pixel aspect is at two pixels per row, h = round(w·imgH/imgW/2)
// — the engine assumes the usual terminal cell aspect, a cell twice as
// tall as wide, because it cannot know the cell's pixel geometry.
func TestImgNaturalSize(t *testing.T) {
	cases := []struct {
		name         string
		decls        string
		pw, ph, cols int
		wantW, wantH int
	}{
		{"square 80x80 in 80 cols", "", 80, 80, 80, 40, 20},
		{"wide 100x50 in 80 cols", "", 100, 50, 80, 40, 10},
		{"tall 50x100 in 80 cols", "", 50, 100, 80, 40, 40},
		{"clamped to a 30-col container", "", 80, 80, 30, 30, 15},
		{"width= given, height derived", "width: 10", 80, 80, 80, 10, 5},
		{"height= given, width natural", "height: 3", 80, 80, 80, 40, 3},
		{"both given", "width: 6; height: 2", 80, 80, 80, 6, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := n("screen", "", n("col", "align: start", imgBox(c.decls, c.pw, c.ph)))
			runCurrent(t, root, c.cols, 50)
			b := root.Children[0].Children[0]
			if b.W != c.wantW || b.H != c.wantH {
				t.Fatalf("img %dx%d, want %dx%d", b.W, b.H, c.wantW, c.wantH)
			}
		})
	}
}

// An img whose source did not decode (ImgW 0) has no natural size: the
// box collapses to nothing instead of guessing.
func TestImgUndecodedCollapses(t *testing.T) {
	root := n("screen", "", n("col", "align: start", imgBox("", 0, 0)))
	runCurrent(t, root, 80, 24)
	if b := root.Children[0].Children[0]; b.W != 0 || b.H != 0 {
		t.Fatalf("undecoded img %dx%d, want 0x0", b.W, b.H)
	}
}

// An img shrinks to its container's width (its natural width clamps
// there) but never shrinks down: its height comes from its width.
func TestImgShrinkableOnlyAcross(t *testing.T) {
	e := &Engine{}
	b := imgBox("", 80, 80)
	if !e.shrinkable(b, true) {
		t.Error("img not shrinkable horizontally")
	}
	if e.shrinkable(b, false) {
		t.Error("img shrinkable vertically")
	}
}
