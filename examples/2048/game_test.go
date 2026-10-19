package main

import (
	"reflect"
	"testing"
)

func TestSlideLine(t *testing.T) {
	cases := []struct {
		in     []int
		out    []int
		merged []bool
		gain   int
	}{
		{[]int{0, 0, 0, 0}, []int{0, 0, 0, 0}, []bool{false, false, false, false}, 0},
		{[]int{2, 0, 0, 0}, []int{2, 0, 0, 0}, []bool{false, false, false, false}, 0},
		{[]int{0, 0, 0, 2}, []int{2, 0, 0, 0}, []bool{false, false, false, false}, 0},
		{[]int{2, 2, 0, 0}, []int{4, 0, 0, 0}, []bool{true, false, false, false}, 4},
		{[]int{2, 0, 2, 0}, []int{4, 0, 0, 0}, []bool{true, false, false, false}, 4},
		// A merged tile does not merge again: 2 2 4 is 4 4, not 8.
		{[]int{2, 2, 4, 0}, []int{4, 4, 0, 0}, []bool{true, false, false, false}, 4},
		// Four equal tiles make two merges, not one 8.
		{[]int{2, 2, 2, 2}, []int{4, 4, 0, 0}, []bool{true, true, false, false}, 8},
		// Three equal tiles merge the pair nearest the wall.
		{[]int{2, 2, 2, 0}, []int{4, 2, 0, 0}, []bool{true, false, false, false}, 4},
		{[]int{4, 2, 2, 0}, []int{4, 4, 0, 0}, []bool{false, true, false, false}, 4},
		{[]int{2, 4, 2, 4}, []int{2, 4, 2, 4}, []bool{false, false, false, false}, 0},
		{[]int{1024, 1024, 0, 0}, []int{2048, 0, 0, 0}, []bool{true, false, false, false}, 2048},
	}
	for _, c := range cases {
		out, merged, gain := slideLine(c.in)
		if !reflect.DeepEqual(out, c.out) || !reflect.DeepEqual(merged, c.merged) || gain != c.gain {
			t.Errorf("slideLine(%v) = %v %v %d, want %v %v %d", c.in, out, merged, gain, c.out, c.merged, c.gain)
		}
	}
}

// board builds a game of side n from rows (short rows are padded with 0).
func board(n int, rows ...[]int) *game {
	g := newGame(n, 1, 0)
	g.cells = make([]int, n*n)
	for y, r := range rows {
		for x, v := range r {
			g.cells[y*n+x] = v
		}
	}
	g.clearFlash()
	g.hist = []int{0}
	g.prev = nil
	g.score, g.moves, g.gain = 0, 0, 0
	return g
}

// Each direction slides towards its own wall.
func TestMoveDirections(t *testing.T) {
	for _, c := range []struct {
		d    dir
		x, y int // where a lone tile at (1,1) ends up
	}{
		{dirLeft, 0, 1}, {dirRight, 3, 1}, {dirUp, 1, 0}, {dirDown, 1, 3},
	} {
		g := board(4, nil, []int{0, 2})
		if !g.move(c.d) {
			t.Fatalf("%v did not move", c.d)
		}
		if g.cells[c.y*4+c.x] != 2 {
			t.Errorf("%v: no tile at (%d,%d): %v", c.d, c.x, c.y, g.cells)
		}
	}
}

func TestMergeScoresTheSumOfMerges(t *testing.T) {
	g := board(4, []int{2, 2, 4, 4}, []int{8, 0, 8, 0})
	g.move(dirLeft)
	// 2+2 -> 4, 4+4 -> 8, 8+8 -> 16: 4 + 8 + 16.
	if g.gain != 28 || g.score != 28 || g.best != 28 {
		t.Errorf("gain %d score %d best %d, want 28", g.gain, g.score, g.best)
	}
	if g.cells[0] != 4 || g.cells[1] != 8 || g.cells[4] != 16 {
		t.Errorf("cells = %v", g.cells)
	}
	if !g.pop[0] || !g.pop[1] || !g.pop[4] {
		t.Errorf("pop = %v, want the three merged tiles", g.pop)
	}
}

// A slide that changes nothing spawns nothing, counts nothing, and leaves
// the generator where it was.
func TestNoOpMoveChangesNothing(t *testing.T) {
	a := board(4, []int{2, 4, 8, 16})
	b := board(4, []int{2, 4, 8, 16})
	if a.move(dirLeft) {
		t.Fatal("a flush row moved left")
	}
	if a.moves != 0 || a.canUndo() || a.hasFlash() || a.score != 0 || len(a.hist) != 1 {
		t.Errorf("a no-op left traces: moves=%d undo=%v flash=%v score=%d hist=%v", a.moves, a.canUndo(), a.hasFlash(), a.score, a.hist)
	}
	if !reflect.DeepEqual(a.cells, []int{2, 4, 8, 16, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}) {
		t.Errorf("cells changed: %v", a.cells)
	}
	// The generator did not move: the next real move spawns the same tile
	// as in a game that never tried the no-op.
	a.move(dirDown)
	b.move(dirDown)
	if !reflect.DeepEqual(a.cells, b.cells) {
		t.Errorf("a no-op consumed randomness:\n%v\n%v", a.cells, b.cells)
	}
}

func TestSpawnValues(t *testing.T) {
	count := func(p4 float64, n int) (twos, fours int) {
		g := newGame(6, 3, p4)
		for i := 0; i < n; i++ {
			g.cells = make([]int, 36)
			g.clearFlash()
			g.spawn()
			for _, v := range g.cells {
				switch v {
				case 2:
					twos++
				case 4:
					fours++
				}
			}
		}
		return
	}
	if tw, fo := count(0, 300); tw != 300 || fo != 0 {
		t.Errorf("p4=0: %d twos %d fours", tw, fo)
	}
	if tw, fo := count(1, 300); tw != 0 || fo != 300 {
		t.Errorf("p4=1: %d twos %d fours", tw, fo)
	}
	tw, fo := count(defaultP4, 5000)
	if tw+fo != 5000 || fo < 400 || fo > 600 {
		t.Errorf("default: %d twos %d fours of 5000, want about 10%% fours", tw, fo)
	}
}

func TestSpawnMarksFreshAndOnlyOne(t *testing.T) {
	g := board(4, []int{2, 0, 0, 0}, []int{0, 2})
	g.move(dirLeft)
	fresh := 0
	for i, f := range g.fresh {
		if f {
			fresh++
			if g.cells[i] == 0 {
				t.Errorf("fresh flag on an empty cell %d", i)
			}
		}
	}
	if fresh != 1 {
		t.Errorf("%d fresh cells, want 1", fresh)
	}
	tiles := 0
	for _, v := range g.cells {
		if v != 0 {
			tiles++
		}
	}
	if tiles != 3 {
		t.Errorf("%d tiles after one move of two, want 3", tiles)
	}
}

func TestNewGameHasTwoTilesAndNoHighlights(t *testing.T) {
	for n := minSize; n <= maxSize; n++ {
		g := newGame(n, 5, defaultP4)
		tiles := 0
		for _, v := range g.cells {
			if v != 0 {
				tiles++
				if v != 2 && v != 4 {
					t.Errorf("n=%d: opening tile %d", n, v)
				}
			}
		}
		if tiles != 2 || g.hasFlash() || g.moves != 0 || g.score != 0 || g.over {
			t.Errorf("n=%d: tiles=%d flash=%v moves=%d score=%d over=%v", n, tiles, g.hasFlash(), g.moves, g.score, g.over)
		}
	}
}

func TestSeedDecidesTheGame(t *testing.T) {
	play := func(seed int64) []int {
		g := newGame(4, seed, defaultP4)
		for i := 0; i < 40; i++ {
			d, ok := bestMove(4, g.cells, 1)
			if !ok {
				break
			}
			g.move(d)
		}
		return append([]int{g.score}, g.cells...)
	}
	if !reflect.DeepEqual(play(7), play(7)) {
		t.Error("one seed played two games")
	}
	if reflect.DeepEqual(play(7), play(8)) {
		t.Error("two seeds played the same game")
	}
}

func TestUndoRestoresTheLastMoveOnly(t *testing.T) {
	g := board(4, []int{2, 2, 0, 0}, []int{0, 0, 4})
	before := append([]int(nil), g.cells...)
	g.move(dirLeft)
	if !g.canUndo() {
		t.Fatal("no undo after a move")
	}
	g.best = 999 // a better game earlier: undo must not lower it
	if !g.undo() {
		t.Fatal("undo failed")
	}
	if !reflect.DeepEqual(g.cells, before) || g.score != 0 || g.moves != 0 || g.gain != 0 || len(g.hist) != 1 {
		t.Errorf("undo left cells=%v score=%d moves=%d hist=%v", g.cells, g.score, g.moves, g.hist)
	}
	if g.best != 999 {
		t.Errorf("undo lowered the best score to %d", g.best)
	}
	if g.hasFlash() {
		t.Error("undo left highlights")
	}
	if g.canUndo() || g.undo() {
		t.Error("a second undo went further back")
	}
}

// Undo does not rewind the generator: a move played again spawns from where
// the generator stands, so on some seeds the board differs from the first try.
func TestUndoDoesNotRewindTheGenerator(t *testing.T) {
	differs := 0
	for seed := int64(1); seed <= 20; seed++ {
		g := newGame(4, seed, defaultP4)
		g.move(dirLeft)
		first := append([]int(nil), g.cells...)
		g.undo()
		g.move(dirLeft)
		if !reflect.DeepEqual(first, g.cells) {
			differs++
		}
	}
	if differs == 0 {
		t.Error("a replayed move always spawned the same tile: undo rewinds the generator")
	}
}

func TestUndoTakesBackGameOver(t *testing.T) {
	g := board(4, stuckRows()...)
	g.p4 = 0
	if !g.move(dirRight) || !g.over {
		t.Fatalf("the stuck board did not end the game: over=%v cells=%v", g.over, g.cells)
	}
	if !g.undo() || g.over {
		t.Errorf("undo did not reopen the game: over=%v", g.over)
	}
}

func TestWinIsReportedOnceOnTheFirst2048(t *testing.T) {
	g := board(4, []int{1024, 1024, 0, 0}, []int{1024, 1024})
	g.move(dirLeft)
	if !g.reachedWin || !g.won {
		t.Fatal("the first 2048 did not set reachedWin")
	}
	g.move(dirRight)
	if g.reachedWin {
		t.Error("reachedWin stayed set on the next move")
	}
	// A second 2048 later is not news.
	g2 := board(4, []int{2048, 0, 0, 0}, []int{1024, 1024})
	g2.won = true
	g2.move(dirLeft)
	if g2.reachedWin {
		t.Error("a 2048 after the win was reported again")
	}
}

func TestCanMove(t *testing.T) {
	if board(4, stuckRows()...).canMove() == false {
		t.Error("the board before the last slide can move")
	}
	g := board(2, []int{2, 4}, []int{4, 2})
	if g.canMove() {
		t.Error("a 2x2 checkerboard cannot move")
	}
	g = board(2, []int{2, 4}, []int{4, 4})
	if !g.canMove() {
		t.Error("two equal neighbours can merge")
	}
	g = board(2, []int{2, 4}, []int{4, 0})
	if !g.canMove() {
		t.Error("an empty cell can be moved into")
	}
}

func TestLargest(t *testing.T) {
	g := board(4, []int{2, 64, 8}, []int{0, 0, 512})
	if g.largest() != 512 || g.empties() != 12 {
		t.Errorf("largest %d empties %d", g.largest(), g.empties())
	}
}

func TestSampleHist(t *testing.T) {
	// Shorter than the width: right-aligned, gaps to the left.
	got := sampleHist([]int{0, 4, 12}, 5)
	want := []any{nil, nil, 0, 4, 12}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("short history = %v, want %v", got, want)
	}
	// Longer: evenly resampled, first and last kept.
	long := make([]int, 101)
	for i := range long {
		long[i] = i * 10
	}
	got = sampleHist(long, 5)
	want = []any{0, 250, 500, 750, 1000}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("long history = %v, want %v", got, want)
	}
	// Exactly the width.
	got = sampleHist([]int{1, 2, 3}, 3)
	if !reflect.DeepEqual(got, []any{1, 2, 3}) {
		t.Errorf("exact history = %v", got)
	}
}

func TestExponent(t *testing.T) {
	for v, want := range map[int]uint8{1: 0, 2: 1, 4: 2, 2048: 11, 131072: 17} {
		if got := exponent(v); got != want {
			t.Errorf("exponent(%d) = %d, want %d", v, got, want)
		}
	}
}
