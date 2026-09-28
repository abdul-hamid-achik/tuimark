package paint

import (
	"fmt"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/layout"
)

// SPEC v0.2b §12.3 (scrollbar) and §12.4 (bar: eighths), §21 tests 60
// and 61, on hand-built boxes styled with version="2" declarations.

// bx2 is bx with version="2" declarations (scrollbar, bar).
func bx2(kind, decls string, kids ...*layout.Box) *layout.Box {
	b := bx(kind, "", kids...)
	if decls != "" {
		ds, errs := css.ParseDeclsAllIn(decls, true, false)
		if len(errs) > 0 {
			panic(errs[0])
		}
		b.Inline = ds
	}
	return b
}

// scrolled renders a scroll of outer height h, width 6, holding one box
// content rows tall, at offset off, and returns the grid and the scroll.
func scrolled(t *testing.T, decls string, h, content, off int) (*Grid, *layout.Box) {
	t.Helper()
	s := bx2("scroll#s", decls+"; height: "+fmt.Sprint(h), bx("box", fmt.Sprintf("height: %d", content)))
	s.ScrollY = off
	root := bx("col", "", s)
	return render(t, root, 6, h), s
}

// thumbRows lists the rows whose column x holds glyph.
func thumbRows(g *Grid, x int, glyph rune) []int {
	var out []int
	for y := 0; y < g.H; y++ {
		if c := g.At(x, y); c.Ch == glyph {
			out = append(out, y)
		}
	}
	return out
}

// 60. The §12.3 vectors, track = 8 and view = 8 (outer height 10 with a
// border): content 100 gives length 1 and offsets 0, 46, 92 give pos 0,
// 3, 7; content 16 gives length 4 and offsets 0, 4, 8 give pos 0, 2, 4.
// The thumb is ┃ on single and rounded borders and █ on double and thick
// ones, in the border's color, owned by the viewport.
func TestScrollbarVectors(t *testing.T) {
	for _, c := range []struct {
		content, off, pos, length int
	}{
		{100, 0, 0, 1}, {100, 46, 3, 1}, {100, 92, 7, 1},
		{16, 0, 0, 4}, {16, 4, 2, 4}, {16, 8, 4, 4},
	} {
		for border, glyph := range map[string]rune{"single": '┃', "rounded": '┃', "double": '█', "thick": '█'} {
			g, s := scrolled(t, "border: "+border+"; border-color: red; scrollbar: auto", 10, c.content, c.off)
			if s.ScrollY != c.off || s.ContentH != c.content || s.Content.H != 8 {
				t.Fatalf("setup: offset %d extent %d view %d", s.ScrollY, s.ContentH, s.Content.H)
			}
			var want []int
			for y := 1 + c.pos; y <= c.pos+c.length; y++ {
				want = append(want, y)
			}
			if got := thumbRows(g, 5, glyph); fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("%s content %d offset %d: thumb rows %v, want %v\n%s", border, c.content, c.off, got, want, lines(g))
			}
			if cell := g.At(5, 1+c.pos); cell.FG != mustColor(t, "red") || cell.Owner != "s" {
				t.Errorf("%s: thumb fg %v owner %q", border, cell.FG, cell.Owner)
			}
		}
	}
}

// 60. No thumb by default (scrollbar: none), without a border, without
// overflow, with an outer height below 3, or on a viewport that scrolls
// only on x; and the thumb takes no layout space: every width and
// position is the same with and without it.
func TestScrollbarWhenNot(t *testing.T) {
	for _, c := range []struct {
		name, decls string
		h, content  int
	}{
		{"default none", "border: single", 10, 100},
		{"no border", "scrollbar: auto", 10, 100},
		{"no overflow", "border: single; scrollbar: auto", 10, 8},
		{"height 2", "border: single; scrollbar: auto", 2, 100},
	} {
		g, _ := scrolled(t, c.decls, c.h, c.content, 0)
		for y := 0; y < g.H; y++ {
			if ch := g.At(5, y).Ch; ch == '┃' || ch == '█' {
				t.Errorf("%s: a thumb at row %d\n%s", c.name, y, lines(g))
			}
		}
	}
	x := bx2("scroll#s", "border: single; scrollbar: auto; height: 5", bx("box", "width: 50; height: 1"))
	x.Axis = "x"
	g := render(t, bx("col", "", x), 6, 5)
	if strings.ContainsAny(lines(g), "┃█") {
		t.Errorf("a viewport scrolling only on x got a thumb:\n%s", lines(g))
	}
	geom := func(decls string) string {
		inner := bx("text", "")
		inner.Text = strings.Repeat("x\n", 30)
		s := bx2("scroll#s", decls, inner)
		render(t, bx("col", "", s), 8, 6)
		return fmt.Sprintf("%v %v %v %v", s.Outer(), s.Content, inner.Outer(), s.ContentH)
	}
	if a, b := geom("border: single"), geom("border: single; scrollbar: auto"); a != b {
		t.Errorf("the thumb changed the layout: %s vs %s", a, b)
	}
}

// 60. mulDiv never overflows: a huge extent still gives a thumb inside
// the track.
func TestMulDiv(t *testing.T) {
	if got := mulDiv(1<<40, 1<<40, 1<<41); got != 1<<39 {
		t.Errorf("mulDiv = %d", got)
	}
	if mulDiv(8, 0, 100) != 0 || mulDiv(0, 5, 7) != 0 || mulDiv(7, 46, 92) != 3 {
		t.Error("small vectors")
	}
}

// progressRow paints a 10-column progress at value v with decls and
// returns its row and the grid.
func progressRow(t *testing.T, decls string, v float64) (string, *Grid) {
	t.Helper()
	p := bx2("progress", decls+"; width: 10")
	p.Value = v
	g := render(t, bx("col", "", p), 10, 1)
	return g.Lines()[0], g
}

// 61. The §12.4 vectors (W = 10): e = floor(W·8·v/100 + 0.5), e div 8
// full cells, the partial glyph e mod 8 in the node's style, the rest ░
// with dim; and bar: block unchanged from v1.
func TestBarEighths(t *testing.T) {
	for _, c := range []struct {
		v    float64
		want string
	}{
		{0, "░░░░░░░░░░"}, {1, "▏░░░░░░░░░"}, {12.5, "█▎░░░░░░░░"}, {33, "███▎░░░░░░"},
		{50, "█████░░░░░"}, {99, "█████████▉"}, {100, "██████████"},
		{-5, "░░░░░░░░░░"}, {250, "██████████"},
	} {
		row, g := progressRow(t, "bar: eighths; color: green", c.v)
		if row != c.want {
			t.Errorf("v=%v: %q, want %q", c.v, row, c.want)
		}
		for x := 0; x < 10; x++ {
			cell := g.At(x, 0)
			dim := cell.Attrs&Dim != 0
			if (cell.Ch == '░') != dim {
				t.Errorf("v=%v col %d %q: dim %v", c.v, x, string(cell.Ch), dim)
			}
			if cell.Ch != '░' && cell.FG != mustColor(t, "green") {
				t.Errorf("v=%v col %d: fg %v, want the node's color", c.v, x, cell.FG)
			}
		}
	}
	for _, c := range []struct {
		v    float64
		want string
	}{{0, "░░░░░░░░░░"}, {1, "░░░░░░░░░░"}, {12.5, "█░░░░░░░░░"}, {33, "███░░░░░░░"}, {99, "██████████"}} {
		for _, decls := range []string{"bar: block", "color: green"} {
			if row, _ := progressRow(t, decls, c.v); row != c.want {
				t.Errorf("%s v=%v: %q, want %q", decls, c.v, row, c.want)
			}
		}
	}
}

// §6.9.3: a table's header cells, rows, and body cells ignore their own
// border; a cell never wraps (wrap: wrap paints as truncate).
func TestFixedBoxesIgnoreBorderAndWrap(t *testing.T) {
	cell := tx("a long cell text", "border: single; wrap: wrap")
	cell.Fixed = true
	root := bx("col", "", cell)
	c := css.NewCascade(nil, css.Env{Cols: 8, Rows: 3})
	styleAll(c, root, nil)
	(&layout.Engine{}).Layout(root, nil, 8, 3)
	// The table places a cell itself: one row, its content box its rect.
	cell.X, cell.Y, cell.W, cell.H = 0, 1, 8, 1
	cell.Content, cell.Clip = cell.Outer(), cell.Outer()
	g := Paint(root, nil, 8, 3)
	if got := g.Lines(); got[0] != "        " || got[1] != "a long …" || got[2] != "        " {
		t.Errorf("grid %q", got)
	}
}

func mustColor(t *testing.T, s string) css.Color {
	t.Helper()
	c, err := css.ParseLiteralColor(s)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
