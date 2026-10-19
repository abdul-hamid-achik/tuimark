package main

import (
	"testing"
	"time"
)

// Tab moves between the board and the move box and nowhere else: the move
// list and the cells never take the focus, so the keymap (which belongs to
// the board's focus) is always reachable.
func TestTabCyclesTheBoardAndTheMoveBox(t *testing.T) {
	a := newTestApp(t, twoPlayers)
	a.moves(t, "e4", "e5", "Nf3")
	seen := []string{focused(a.play(t))}
	for i := 0; i < 4; i++ {
		seen = append(seen, focused(a.play(t, "tab")))
	}
	want := []string{"board", "cmd", "board", "cmd", "board"}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("focus order %v, want %v", seen, want)
		}
	}
	// shift+tab goes the other way round; either way the keys of the board
	// work once it has the focus again.
	a.play(t, "shift+tab")
	a.play(t, "tab")
	res := a.play(t, "?")
	wantActions(t, res, "help_open")
}

// The focus comes back to the board after everything that takes it away.
func TestFocusReturnsToTheBoard(t *testing.T) {
	steps := map[string][]string{
		"help":     {"?", "esc"},
		"new":      {"n", "esc"},
		"start":    {"n", "enter"},
		"resign":   {"r", "n"},
		"resign y": {"r", "y"},
		"pgn":      {"p", "esc"},
		"cmd esc":  {":", "esc"},
		"cmd mv":   {":", "text:d4", "enter"},
	}
	for name, keys := range steps {
		a := newTestApp(t, twoPlayers)
		res := a.play(t, keys...)
		if focused(res) != "board" {
			t.Errorf("%s: focus = %q", name, focused(res))
		}
		if got := actions(a.play(t, "f")); len(got) != 1 || got[0] != "flip" {
			t.Errorf("%s: the board's keys are dead afterwards: %v", name, got)
		}
	}
}

// A search that takes its time still answers no faster than thinkDelay in
// live play (a quick level would otherwise reply before the eye sees the
// move), and a hint never waits.
func TestLiveSearchesTakeTheirMinimumTime(t *testing.T) {
	a := newTestApp(t)
	a.thinkDelay = 120 * time.Millisecond
	start := time.Now()
	a.moves(t, "e4")
	if d := time.Since(start); d < a.thinkDelay {
		t.Errorf("the computer answered in %v, less than %v", d, a.thinkDelay)
	}
	start = time.Now()
	a.play(t, "H")
	if d := time.Since(start); d > 100*time.Millisecond {
		t.Errorf("the hint waited %v", d)
	}
	if minThink < 300*time.Millisecond {
		t.Errorf("minThink = %v", minThink)
	}
}
