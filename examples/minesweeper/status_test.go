package main

import (
	"strings"
	"testing"
)

// The sizes the README names: the terminal each preset needs.
func TestPresetsNeedTheSizesTheREADMEGives(t *testing.T) {
	want := map[string][2]int{"beginner": {36, 13}, "intermediate": {36, 20}, "expert": {62, 20}}
	for _, p := range presets {
		nc, nr := needFor(board{p.name, p.cols, p.rows, p.mines})
		if got := want[p.name]; got != [2]int{nc, nr} {
			t.Errorf("%s needs %dx%d, want %v", p.name, nc, nr, got)
		}
		// The host and the test agree.
		if needCols(p.cols) != nc || needRows(p.rows) != nr {
			t.Errorf("%s: needCols/needRows = %d, %d", p.name, needCols(p.cols), needRows(p.rows))
		}
	}
}

// The status line holds the brand and the level beside the side panel. When
// the counters move into it, the brand goes; the level label goes up to 75
// columns, and the best time of the level goes below 44.
func TestStatusLineDropsTheBrandThenTheLevelThenTheBest(t *testing.T) {
	for _, b := range layoutBoards[:3] {
		r := newRigOn(t, b, 2)
		r.arrange("started")
		nc, _ := needFor(b)
		for _, cols := range []int{nc, 40, 43, 44, 50, 62, 65, 66, 67, 75, 76, 79, 80, 91, 92, 100, 120} {
			if cols < nc {
				continue
			}
			top := strings.Split(r.grid(cols, 24), "\n")[0]
			side := cols >= nc+sideRoom
			compact := !side
			checks := []struct {
				name string
				got  bool
				want bool
			}{
				{"brand", strings.Contains(top, "◆ minesweeper"), side},
				{"level", strings.Contains(top, b.level+" "), side || cols >= 76},
				{"best", strings.Contains(top, "best --"), compact && cols >= 44},
			}
			for _, c := range checks {
				if c.got != c.want {
					t.Errorf("%s at %d columns: %s shown = %v, want %v in %q", b.level, cols, c.name, c.got, c.want, top)
				}
			}
		}
	}
}

// The best time in the status line is the one of the board being played.
func TestCompactBestTimeIsTheCurrentLevels(t *testing.T) {
	r := winning(t, beginner, 42)
	r.play("n")
	if got := strings.Split(r.grid(60, 24), "\n")[0]; !strings.Contains(got, "best 0:42") {
		t.Errorf("the beginner best is not in %q", got)
	}
	r.play("2")
	if got := strings.Split(r.grid(60, 24), "\n")[0]; !strings.Contains(got, "best --") {
		t.Errorf("intermediate shows the beginner's best: %q", got)
	}
	r.play("1")
	if got := r.str("best_now"); got != "0:42" {
		t.Errorf("best_now = %s", got)
	}
}
