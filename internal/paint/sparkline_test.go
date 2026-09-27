package paint

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

// SPEC v0.2b §6.11 (sparkline), §21 test 56: the normative vectors, row by
// row, on hand-built boxes; values after min= and max= are given as
// series (NaN is a gap: null or not a number).

// spark renders a sparkline W columns and H rows wide with series vals,
// and min/max when given (nil otherwise), and returns its rows, top first.
func spark(t *testing.T, vals []float64, w, h int, lo, hi *float64) []string {
	t.Helper()
	s := bx("sparkline", fmt.Sprintf("width: %d; height: %d", w, h))
	s.Series = vals
	if lo != nil {
		s.Lo, s.HasLo = *lo, true
	}
	if hi != nil {
		s.Hi, s.HasHi = *hi, true
	}
	return render(t, bx("col", "", s), w, h).Lines()
}

func num(f float64) *float64 { return &f }

// 56. The §6.11 vectors: min and max, a gap (null), values clamped to
// [lo, hi], the last W values right-aligned, a flat series at half
// height, and H = 2.
func TestSparklineVectors(t *testing.T) {
	nan := math.NaN()
	for _, c := range []struct {
		vals   []float64
		w, h   int
		lo, hi *float64
		want   []string
	}{
		{[]float64{0, 1, 4, 8, nan, 9}, 6, 1, num(0), num(8), []string{" ▁▄█ █"}},
		{[]float64{3, 7, 1, 9, 4}, 8, 1, nil, nil, []string{"   ▂▆ █▃"}},
		{[]float64{5, 5, 5}, 3, 1, nil, nil, []string{"▄▄▄"}},
		{[]float64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100, 0}, 4, 1, num(0), num(100), []string{"▆▇█ "}},
		{[]float64{0, 2, 4, 6, 8, 10, 12, 14, 16}, 9, 2, num(0), num(16), []string{"     ▂▄▆█", " ▂▄▆█████"}},
	} {
		if got := spark(t, c.vals, c.w, c.h, c.lo, c.hi); strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("%v (W %d, H %d): %q, want %q", c.vals, c.w, c.h, got, c.want)
		}
	}
}

// 56. Nothing is painted without a number among the shown values (an
// empty array, only gaps); a negative min is a number like any other;
// painted cells take the node's style and the others keep its background.
func TestSparklineEdges(t *testing.T) {
	nan := math.NaN()
	if got := spark(t, nil, 4, 1, nil, nil); got[0] != "    " {
		t.Errorf("empty array: %q", got[0])
	}
	if got := spark(t, []float64{nan, nan}, 4, 1, nil, nil); got[0] != "    " {
		t.Errorf("only gaps: %q", got[0])
	}
	// Only the shown values count for lo and hi: 100 is out of view.
	if got := spark(t, []float64{100, 0, 5, 10}, 3, 1, nil, nil); got[0] != " ▄█" {
		t.Errorf("shown values only: %q", got[0])
	}
	if got := spark(t, []float64{-5, 0, 5}, 3, 1, num(-5), num(5)); got[0] != " ▄█" {
		t.Errorf("negative min: %q", got[0])
	}
	s := bx("sparkline", "width: 2; height: 1; color: red; background: blue")
	s.Series = []float64{0, 1}
	g := render(t, bx("col", "", s), 2, 1)
	if c := g.At(1, 0); c.Ch != '█' || c.FG.String() != "red" || c.BG.String() != "blue" {
		t.Errorf("painted cell %+v", c)
	}
	if c := g.At(0, 0); c.Ch != ' ' || c.BG.String() != "blue" {
		t.Errorf("unpainted cell %+v", c)
	}
	// The intrinsic width is the array's length, one row high.
	auto := bx("sparkline", "")
	auto.Series = []float64{1, 2, 3, 4, 5}
	render(t, bx("row", "", auto), 20, 3)
	if auto.W != 5 || auto.H != 3 {
		t.Errorf("sparkline in a row: %dx%d", auto.W, auto.H)
	}
}
