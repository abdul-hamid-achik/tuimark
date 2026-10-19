package main

import (
	"strings"
	"testing"
)

// On the "terminal too small" screen the board is not drawn, so the board
// keys are off: nothing is revealed, flagged, or moved that cannot be seen.
// The help, quit, and ctrl+c keys still work there, and the board answers
// again as soon as the terminal is big enough.
func TestTooSmallTurnsTheBoardKeysOff(t *testing.T) {
	for _, small := range [][2]int{{35, 13}, {40, 12}, {20, 8}} { // narrow, short, tiny
		r := newRig(t)
		res := r.playAt(small[0], small[1])
		if !strings.Contains(strings.Join(strings.Fields(gridOf(res)), " "), "terminal too small") {
			t.Fatalf("%v: no notice:\n%s", small, gridOf(res))
		}
		cur := r.a.cur
		for _, key := range []string{"enter", "space", "g", "f", "n", "1", "2", "3", "p", "m", "up", "left", "k", "h"} {
			res = r.playAt(small[0], small[1], key)
			if len(res.Events) != 0 {
				t.Errorf("%v: %s fired %v on the hidden board", small, key, actions(res))
			}
		}
		if r.a.g.placed || r.a.cur != cur || r.boolean("flag_mode") || !r.boolean("settings.mouse") || r.a.g.flags != 0 {
			t.Errorf("%v: the hidden board changed (placed %v, cursor %d->%d)", small, r.a.g.placed, cur, r.a.cur)
		}
		// Help and quit work.
		res = r.playAt(small[0], small[1], "?")
		wantActions(t, res, "help_toggle")
		res = r.playAt(small[0], small[1], "?")
		wantActions(t, res, "help_toggle")
		if r.boolean("help_open") {
			t.Errorf("%v: ? did not toggle the help back", small)
		}
		for _, key := range []string{"q", "ctrl+c"} {
			r2 := newRig(t)
			if res := r2.playAt(small[0], small[1], key); !res.Quit {
				t.Errorf("%v: %s did not quit", small, key)
			}
		}
		// Big enough again: the keys play the board.
		res = r.playAt(90, 24, "enter")
		wantActions(t, res, "reveal")
		if !r.a.g.placed {
			t.Errorf("%v: enter did not reveal once the terminal grew", small)
		}
	}
}

// The same when the terminal shrinks in the middle of a game, and grows
// back: the game is where it was and answers again.
func TestShrinkingAndGrowingKeepsTheGame(t *testing.T) {
	r := newRig(t)
	r.play(r.click(4, 4))
	opened, cur := r.a.g.opened, r.a.cur
	res := r.playAt(30, 10, "right", "enter", "g")
	if len(res.Events) != 0 {
		t.Errorf("keys fired %v on the hidden board", actions(res))
	}
	if r.a.g.opened != opened || r.a.cur != cur {
		t.Error("the hidden board changed")
	}
	res = r.playAt(90, 24, "right")
	wantActions(t, res, "cur_right")
	if focusOf(res) != "board" {
		t.Errorf("focus %q after growing back", focusOf(res))
	}
}

// Nothing is reported on tiny terminals either: the notice, the hints, and
// the status line fit or give way without a cut that goes unsaid.
func TestTinyTerminalsAreClean(t *testing.T) {
	for _, b := range []board{layoutBoards[0], layoutBoards[2]} {
		r := newRigOn(t, b, 5)
		r.arrange("started")
		for cols := 10; cols <= 40; cols++ {
			theme := []string{"dark", "light"}[cols%2] // each width in one theme, alternating
			if err := r.a.ui.Set("@theme", theme); err != nil {
				t.Fatal(err)
			}
			for _, rows := range []int{6, 12, 14} {
				res := r.playAt(cols, rows)
				if !res.Dump.OK || len(res.Dump.Errors) != 0 {
					t.Fatalf("%s %s %dx%d: %v\n%s", b.level, theme, cols, rows, res.Dump.Errors, gridOf(res))
				}
			}
		}
	}
}
