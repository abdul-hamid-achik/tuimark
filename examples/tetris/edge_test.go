package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// Edge cases of the rules and of the command line, each found by a review of
// the first version.

// A T in its reversed state can be kicked two rows up, to a box row of -1.
// Its top corners are then above the field: open space, never filled, and
// never an index out of range.
func TestTSpinCornersAboveTheField(t *testing.T) {
	g := emptyGame(kT)
	g.cur = piece{k: kT, rot: 2, x: 3, y: -1}
	if n := g.corners(); n != 0 {
		t.Errorf("an empty field gave %d corners", n)
	}
	g.grid[1][3] = kJ // the lower left corner
	g.grid[1][5] = kJ // the lower right corner
	if n := g.corners(); n != 2 {
		t.Errorf("two filled lower corners gave %d, want 2: the top ones are above the field", n)
	}
	// Locking it there is a lock out, not a T-spin and not a panic.
	g.rotated = true
	g.lock()
	if !g.over || g.last == "T-SPIN" || g.score != 0 {
		t.Errorf("over=%v last=%q score=%d", g.over, g.last, g.score)
	}
}

// The state found by the review, reached with the keys a player would press:
// turn back, step right, wait one gravity row, turn back again (the kick that
// lifts the T to row -1), and drop.
func TestKickedToTheCeilingThenDropDoesNotPanic(t *testing.T) {
	r := newRig(t)
	g := r.a.g
	g.grid = [boardH][boardW]kind{}
	g.grid[2][4], g.grid[1][7], g.grid[2][7], g.grid[3][5] = kJ, kJ, kJ, kJ
	g.hold, g.holdUsed = kNone, false
	g.spawn(kT)
	r.sync()
	r.step(1)
	r.play("z", "l")
	r.step(21) // one gravity row at level 1
	r.play("z")
	if want := (piece{k: kT, rot: 2, x: 5, y: -1}); g.cur != want || !g.rotated {
		t.Fatalf("the T is %+v (rotated %v), want %+v: the setup no longer reaches the ceiling kick", g.cur, g.rotated, want)
	}
	r.play("space") // would index the grid at -1
	if !g.over || g.last == "T-SPIN" {
		t.Errorf("over=%v last=%q after the drop", g.over, g.last)
	}
	// And the lock delay path, through the loop's tick.
	r = newRig(t)
	g = r.a.g
	g.grid = [boardH][boardW]kind{}
	g.grid[2][4], g.grid[1][7], g.grid[2][7], g.grid[3][5] = kJ, kJ, kJ, kJ
	g.hold, g.holdUsed = kNone, false
	g.spawn(kT)
	r.sync()
	r.step(1)
	r.play("z", "l")
	r.step(21)
	r.play("z")
	r.step(10) // the lock delay runs out
	if !g.over {
		t.Error("the T at the ceiling never locked")
	}
}

// The lock delay can be restarted 15 times: lifting the piece off the floor
// and letting it land again counts, and once the restarts are used the piece
// locks as it lands. A T turned up and back every 300 ms is locked within a
// few seconds, not kept alive for ever.
func TestAirborneTurnsCannotKeepAPieceAliveForEver(t *testing.T) {
	r := newRig(t)
	r.step(1)
	r.ground()
	g := r.a.g
	start := g.t
	for i := 0; i < 400 && g.pieces == 0; i++ {
		if i%6 == 0 {
			r.play("up", "z")
		}
		r.step(1)
	}
	if g.pieces == 0 {
		t.Fatalf("the piece was still alive after %d ms of turning (resets %d)", g.t-start, g.resets)
	}
	// 15 restarts of 500 ms at most, then the last delay: well under 10 s.
	if g.t-start > 10000 {
		t.Errorf("it took %d ms to lock", g.t-start)
	}
}

// The same at the rules level, for a piece that rests, is lifted, and rests
// on the very row it was on: with the 15 restarts used it locks as it lands,
// and with restarts left it gets a delay again.
func TestLandingAgainOnTheSameRowDoesNotRefillTheDelay(t *testing.T) {
	g := emptyGame(kT)
	g.cur.y += g.dropDistance()
	g.lowest = g.cur.y
	g.settle(false)
	// Lift it one row, as a turn or a kick can, with the restarts used up.
	g.resets = maxResets
	g.cur.y--
	g.settle(true) // airborne now
	if g.lockAt != -1 {
		t.Fatalf("a lifted piece has a lock deadline %d", g.lockAt)
	}
	g.advance(1000) // gravity brings it back to the row it was on
	if g.cur.y != g.lowest {
		t.Fatalf("the piece is on row %d, want its old row %d", g.cur.y, g.lowest)
	}
	if g.pieces != 1 {
		t.Errorf("the piece got a fresh lock delay (pieces %d, lockAt %d): the used restarts were forgotten", g.pieces, g.lockAt)
	}
	// With restarts left it does get the delay.
	g = emptyGame(kT)
	g.cur.y += g.dropDistance()
	g.lowest = g.cur.y
	g.settle(false)
	g.resets = 3
	g.cur.y--
	g.settle(true)
	if g.resets != 4 {
		t.Errorf("lifting the piece used %d restarts, want 4", g.resets)
	}
	g.advance(1000)
	if g.pieces != 0 || g.lockAt < 0 {
		t.Errorf("pieces %d lockAt %d: the piece should rest with a delay", g.pieces, g.lockAt)
	}
}

// A piece that locks wholly above the skyline and away from the spawn ends
// the game on its own: it is a lock out, not a block out.
func TestLockOutIsNotBlockOut(t *testing.T) {
	g := emptyGame(kO)
	g.cur = piece{k: kO, x: -1, y: 0} // columns 0 and 1, rows 0 and 1
	id := g.id
	g.lock()
	if !g.over || g.has {
		t.Fatalf("over=%v has=%v after a piece locked in the hidden rows", g.over, g.has)
	}
	if g.id != id {
		t.Error("another piece was spawned: this was a block out, not a lock out")
	}
	// One row lower it reaches the visible field: the game goes on.
	g = emptyGame(kO)
	g.cur = piece{k: kO, x: -1, y: 1}
	id = g.id
	g.lock()
	if g.over || g.id != id+1 {
		t.Errorf("over=%v id %d -> %d: a piece with a visible cell should not end the game", g.over, id, g.id)
	}
}

// Scoring uses the level before the lines of the clear are added, and the
// level rises with the 10th line.
func TestLevelUpBoundary(t *testing.T) {
	g := newGame(1, 1)
	g.lines = 8
	fillRows(g, 2, 9)
	dropI(g, 9)
	g.hardDrop() // a double that takes it from 8 to 10 lines
	if g.score != 300 {
		t.Errorf("the double that crossed into level 2 scored %d, want 300 (level 1)", g.score)
	}
	if g.level() != 2 || g.lines != 10 {
		t.Errorf("level %d lines %d, want level 2 at 10 lines", g.level(), g.lines)
	}
	g.advance(flashMs)
	score := g.score
	fillRows(g, 1, 9)
	dropI(g, 9)
	g.hardDrop()
	// A single at level 2, and the combo bonus at level 2.
	if want := score + 100*2 + 50*1*2; g.score != want {
		t.Errorf("the next single scored %d, want %d (level 2)", g.score-score, want-score)
	}
	// 9 lines then a single reaches 10 only after scoring at level 1.
	g = newGame(1, 1)
	g.lines = 9
	fillRows(g, 1, 9)
	dropI(g, 9)
	g.hardDrop()
	if g.score != 100 || g.level() != 2 {
		t.Errorf("score %d level %d, want 100 and level 2", g.score, g.level())
	}
}

// The SRS kick tables, pinned row for row from the guideline's wiki page
// (x right, y up), so no transition can drift or be swapped. The T-like
// pieces are also derived from the page's offset formulation: a kick is the
// offset of the old state less the offset of the new.
func TestKickTablesPinned(t *testing.T) {
	type tr = turn
	wikiJLSTZ := map[tr][5][2]int{
		{0, 1}: {{0, 0}, {-1, 0}, {-1, 1}, {0, -2}, {-1, -2}},
		{1, 0}: {{0, 0}, {1, 0}, {1, -1}, {0, 2}, {1, 2}},
		{1, 2}: {{0, 0}, {1, 0}, {1, -1}, {0, 2}, {1, 2}},
		{2, 1}: {{0, 0}, {-1, 0}, {-1, 1}, {0, -2}, {-1, -2}},
		{2, 3}: {{0, 0}, {1, 0}, {1, 1}, {0, -2}, {1, -2}},
		{3, 2}: {{0, 0}, {-1, 0}, {-1, -1}, {0, 2}, {-1, 2}},
		{3, 0}: {{0, 0}, {-1, 0}, {-1, -1}, {0, 2}, {-1, 2}},
		{0, 3}: {{0, 0}, {1, 0}, {1, 1}, {0, -2}, {1, -2}},
	}
	wikiI := map[tr][5][2]int{
		{0, 1}: {{0, 0}, {-2, 0}, {1, 0}, {-2, -1}, {1, 2}},
		{1, 0}: {{0, 0}, {2, 0}, {-1, 0}, {2, 1}, {-1, -2}},
		{1, 2}: {{0, 0}, {-1, 0}, {2, 0}, {-1, 2}, {2, -1}},
		{2, 1}: {{0, 0}, {1, 0}, {-2, 0}, {1, -2}, {-2, 1}},
		{2, 3}: {{0, 0}, {2, 0}, {-1, 0}, {2, 1}, {-1, -2}},
		{3, 2}: {{0, 0}, {-2, 0}, {1, 0}, {-2, -1}, {1, 2}},
		{3, 0}: {{0, 0}, {1, 0}, {-2, 0}, {1, -2}, {-2, 1}},
		{0, 3}: {{0, 0}, {-1, 0}, {2, 0}, {-1, 2}, {2, -1}},
	}
	for _, c := range []struct {
		k    kind
		wiki map[tr][5][2]int
	}{{kT, wikiJLSTZ}, {kJ, wikiJLSTZ}, {kL, wikiJLSTZ}, {kS, wikiJLSTZ}, {kZ, wikiJLSTZ}, {kI, wikiI}} {
		for turn, want := range c.wiki {
			got := kickOffsets(c.k, turn.from, turn.to)
			for i := range want {
				// kickOffsets has y down: the wiki's y is its negation.
				if got[i] != (point{want[i][0], -want[i][1]}) {
					t.Errorf("%s %d->%d test %d = %v, the wiki has %v", c.k.letter(), turn.from, turn.to, i+1, got[i], want[i])
				}
			}
		}
	}
	// The offset formulation of the JLSTZ table.
	off := [4][5][2]int{
		{},
		{{0, 0}, {1, 0}, {1, -1}, {0, 2}, {1, 2}},
		{},
		{{0, 0}, {-1, 0}, {-1, -1}, {0, 2}, {-1, 2}},
	}
	for turn, want := range wikiJLSTZ {
		for i := range want {
			d := [2]int{off[turn.from][i][0] - off[turn.to][i][0], off[turn.from][i][1] - off[turn.to][i][1]}
			if d != want[i] {
				t.Errorf("JLSTZ %d->%d test %d: offsets give %v, the table has %v", turn.from, turn.to, i+1, d, want[i])
			}
		}
	}
	// Every rotation state a piece can turn to has a table: eight turns each.
	if len(kicksJLSTZ) != 8 || len(kicksI) != 8 {
		t.Errorf("%d and %d transitions, want 8 each", len(kicksJLSTZ), len(kicksI))
	}
}

// The game over modal's lines fit in full down to 24 columns with the
// longest numbers a long game has.
func TestGameOverTextIsWhole(t *testing.T) {
	r := newRig(t)
	r.gameOver()
	g := r.a.g
	g.score, g.best, g.lines, g.startLevel = 123456, 123456, 123, 1
	r.sync()
	for cols := 24; cols <= 60; cols++ {
		d, err := r.a.ui.Dump(cols, 24)
		if err != nil {
			t.Fatal(err)
		}
		grid := strings.Join(d.Grid, "\n")
		for _, want := range []string{"score 123456", "best 123456", "lines 123", "level 13"} {
			if !strings.Contains(grid, want) {
				t.Fatalf("%d columns: the game over modal lacks %q:\n%s", cols, want, grid)
			}
		}
		if len(d.Errors) != 0 {
			t.Errorf("%d columns: %v", cols, d.Errors)
		}
	}
}

// While a modal is open the keys it would ignore are not advertised. A
// modal traps the keymap, and the rows repeated for it in
// <keymap when="modal"> (quit and help) carry no label, so the help lists
// each key once: the bar under a modal is empty, while q and ? still work
// there. The move keys come back with the board.
func TestHintsHideDeadKeysBehindModals(t *testing.T) {
	last := func(r *rig) string {
		d, err := r.a.ui.Dump(100, 24)
		if err != nil {
			t.Fatal(err)
		}
		return d.Grid[len(d.Grid)-1]
	}
	dead := []string{"move", "soft", "rotate", "back", "drop", "hold", "pause", "restart", "auto"}
	r := newRig(t)
	if h := last(r); !strings.Contains(h, "q quit") || !strings.Contains(h, "move") || !strings.Contains(h, "auto") {
		t.Errorf("playing: hints %q", h)
	}
	for name, open := range map[string]func(*rig){
		"help":                func(r *rig) { r.play("?") },
		"game over":           func(r *rig) { r.gameOver() },
		"help over game over": func(r *rig) { r.gameOver(); r.play("?") },
	} {
		r := newRig(t)
		open(r)
		h := last(r)
		if strings.TrimSpace(h) != "" {
			t.Errorf("%s: hints %q, want none under a modal", name, h)
		}
		for _, w := range dead {
			if strings.Contains(h, w) {
				t.Errorf("%s: hints %q advertise %q, which does nothing now", name, h, w)
			}
		}
	}
	// Back on the board after the help closes.
	r = newRig(t)
	r.play("?", "esc")
	if h := last(r); !strings.Contains(h, "move") {
		t.Errorf("after the help: hints %q", h)
	}
	// The keys really are dead there: the keymap does not even fire them.
	r = newRig(t)
	r.play("?")
	if res := r.play("left", "p", "a", "c"); len(res.Events) != 0 {
		t.Errorf("keys behind the help fired %+v", res.Events)
	}
}

// The real command line: a value the flag package cannot parse or the game
// cannot run with is a usage error (message naming the flag, usage, exit 2)
// and starts nothing; good ones run.
func TestCommandLine(t *testing.T) {
	if os.Getenv("TETRIS_RUN_MAIN") != "" {
		args := []string{os.Args[0]}
		for i, a := range os.Args {
			if a == "--" {
				args = append(args, os.Args[i+1:]...)
				break
			}
		}
		os.Args = args
		main()
		os.Exit(0)
	}
	run := func(args ...string) (string, int) {
		cmd := exec.Command(os.Args[0], append([]string{"-test.run=^TestCommandLine$", "--"}, args...)...)
		cmd.Env = append(os.Environ(), "TETRIS_RUN_MAIN=1")
		out, err := cmd.CombinedOutput()
		code := 0
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else if err != nil {
			t.Fatal(err)
		}
		return string(out), code
	}
	for _, c := range []struct {
		args []string
		flag string
	}{
		{[]string{"-level", "abc"}, "-level"},
		{[]string{"-level", "0"}, "-level"},
		{[]string{"-level", "16"}, "-level"},
		{[]string{"-level", "99999999999999999999"}, "-level"},
		{[]string{"-pace", "abc"}, "-pace"},
		{[]string{"-pace", "NaN"}, "-pace"},
		{[]string{"-pace", "Inf"}, "-pace"},
		{[]string{"-pace", "-Inf"}, "-pace"},
		{[]string{"-pace", "1e309"}, "-pace"},
		{[]string{"-pace", "0"}, "-pace"},
		{[]string{"-pace", "11"}, "-pace"},
		{[]string{"-seed", "abc"}, "-seed"},
		{[]string{"-seed", "99999999999999999999"}, "-seed"},
		{[]string{"-theme", "mauve"}, "-theme"},
		{[]string{"-dump", "banana"}, "-dump"},
		{[]string{"-dump", "80x24junk"}, "-dump"},
		{[]string{"-demo", "0x0"}, "-demo"},
		{[]string{"-dump", "80x24", "-demo", "80x24"}, "-dump"},
	} {
		out, code := run(append([]string{"-dir", "."}, c.args...)...)
		if code != 2 || !strings.Contains(out, c.flag) || !strings.Contains(out, "Usage") {
			t.Errorf("%v: exit %d, output %.200q; want exit 2, usage, naming %q", c.args, code, out, c.flag)
		}
	}
	if out, code := run("-dir", ".", "-dump", "40x24", "-level", "3", "-pace", "2.5", "-seed", "-4"); code != 0 || !strings.Contains(out, `"ok": true`) {
		t.Errorf("good flags: exit %d\n%.200s", code, out)
	}
	if out, code := run("-dir", "nowhere", "-dump", "40x24"); code != 1 || !strings.Contains(out, "tetris:") {
		t.Errorf("a missing directory: exit %d output %.200q, want exit 1 (not a usage error)", code, out)
	}
}
