package paint

import (
	"reflect"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/layout"
)

// SPEC v0.4b §12.6: the spinner's glyph sets and phase.
func TestSpinnerGlyphSets(t *testing.T) {
	var braille, ascii []rune
	for p := 0; p < 10; p++ {
		braille = append(braille, Glyph(p, false))
	}
	for p := 0; p < 4; p++ {
		ascii = append(ascii, Glyph(p, true))
	}
	if string(braille) != "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏" || string(ascii) != `|/-\` {
		t.Errorf("braille %q, ascii %q", string(braille), string(ascii))
	}
	if Glyph(10, false) != '⠋' || Glyph(12, false) != '⠹' || Glyph(4, true) != '|' || Glyph(-3, false) != '⠋' {
		t.Error("the glyph is S[P mod len(S)], and a negative phase counts as 0")
	}
}

// spinner builds a <progress indeterminate="true"> box.
func spinner(id, decls string) *layout.Box {
	b := bx("progress#"+id, decls)
	b.Src = &ir.Node{Spinner: true}
	return b
}

func renderPhase(t *testing.T, phase, cols, rows int, root *layout.Box, modals ...*layout.Box) *Grid {
	t.Helper()
	c := css.NewCascade(nil, css.Env{Cols: cols, Rows: rows})
	styleAll(c, root, nil)
	for _, m := range modals {
		styleAll(c, m, nil)
	}
	(&layout.Engine{}).Layout(root, modals, cols, rows)
	return PaintPhase(root, modals, cols, rows, phase)
}

// A spinner paints one glyph in the first row of its content box, at the
// column content-align gives, and nothing else; its table lists the cell.
func TestSpinnerPlacement(t *testing.T) {
	root := bx("col", "",
		spinner("a", "width: 5"),
		spinner("b", "width: 5; content-align: center"),
		spinner("c", "width: 5; content-align: end"),
		spinner("d", "width: 5; height: 3; content-align: center"),
	)
	g := renderPhase(t, 3, 5, 6, root)
	want := "⠸    \n  ⠸  \n    ⠸\n  ⠸  \n     \n     "
	if got := lines(g); got != want {
		t.Errorf("grid:\n%s\nwant:\n%s", got, want)
	}
	if !reflect.DeepEqual(g.Spinners, []SpinnerCell{{0, 0}, {2, 1}, {4, 2}, {2, 3}}) {
		t.Errorf("Spinners = %v", g.Spinners)
	}
}

// A zero-width spinner paints nothing; a hidden one neither paints nor
// counts.
func TestSpinnerNothingToPaint(t *testing.T) {
	root := bx("col", "", spinner("a", "width: 0"), spinner("b", "width: 3; visibility: hidden"))
	g := renderPhase(t, 0, 4, 2, root)
	if got := strings.TrimSpace(lines(g)); got != "" || len(g.Spinners) != 0 {
		t.Errorf("grid %q, Spinners %v", lines(g), g.Spinners)
	}
}

// A later painter that writes the glyph's cell drops it from the table: a
// modal over the spinner does; a modal that leaves the cell alone does not. The grid itself needs no new member.
func TestSpinnerTableDropsOverpaintedCells(t *testing.T) {
	over := bx("modal", "width: 100%; height: 100%")
	g := renderPhase(t, 0, 6, 2, bx("col", "", spinner("a", "width: 3")), over)
	if len(g.Spinners) != 0 {
		t.Errorf("a modal over the cell: Spinners = %v", g.Spinners)
	}
	small := bx("modal", "width: 2; height: 1; margin: 1 0 0 3")
	g = renderPhase(t, 0, 6, 3, bx("col", "", spinner("a", "width: 3")), small)
	if !reflect.DeepEqual(g.Spinners, []SpinnerCell{{0, 0}}) {
		t.Errorf("a modal elsewhere: Spinners = %v", g.Spinners)
	}
}

// The phase is the caller's: the same layout paints a new glyph at each.
func TestSpinnerPhaseChangesOnlyTheGlyph(t *testing.T) {
	root := bx("col", "", spinner("a", "width: 2"))
	c := css.NewCascade(nil, css.Env{Cols: 3, Rows: 1})
	styleAll(c, root, nil)
	(&layout.Engine{}).Layout(root, nil, 3, 1)
	for p, want := range map[int]string{0: "⠋  ", 1: "⠙  ", 10: "⠋  ", 12: "⠹  "} {
		if got := lines(PaintPhase(root, nil, 3, 1, p)); got != want {
			t.Errorf("phase %d: %q, want %q", p, got, want)
		}
	}
	if got := lines(Paint(root, nil, 3, 1)); got != "⠋  " {
		t.Errorf("Paint = %q, want phase 0", got)
	}
}
