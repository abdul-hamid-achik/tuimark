package main

import (
	"math"
	"sort"
	"testing"
)

// The rules, with no Tuimark in sight.

func cellSet(cells []point) map[point]bool {
	m := map[point]bool{}
	for _, c := range cells {
		m[c] = true
	}
	return m
}

func sameCells(a, b []point) bool {
	sa, sb := cellSet(a), cellSet(b)
	if len(sa) != len(sb) {
		return false
	}
	for c := range sa {
		if !sb[c] {
			return false
		}
	}
	return true
}

// emptyGame is a game with a piece of kind k at the top and an empty field.
func emptyGame(k kind) *game {
	g := newGame(1, 1)
	g.hold, g.holdUsed = kNone, false
	g.cur = piece{k: k, x: 3, y: 1}
	g.has = true
	g.lockAt, g.resets, g.rotated = -1, 0, false
	return g
}

// fillRows fills the bottom n rows of the field but for column gap.
func fillRows(g *game, n, gap int) {
	for y := boardH - n; y < boardH; y++ {
		for x := 0; x < boardW; x++ {
			if x != gap {
				g.grid[y][x] = kJ
			}
		}
	}
}

// dropI is a vertical I in column col resting on the floor, ready to lock.
func dropI(g *game, col int) {
	g.cur = piece{k: kI, rot: 1, x: col - 2, y: boardH - 4}
	g.has = true
}

func pts(cells [4]point) []point { return cells[:] }

func TestShapesMatchSRS(t *testing.T) {
	want := map[kind][4][]point{
		kI: {
			{{0, 1}, {1, 1}, {2, 1}, {3, 1}},
			{{2, 0}, {2, 1}, {2, 2}, {2, 3}},
			{{0, 2}, {1, 2}, {2, 2}, {3, 2}},
			{{1, 0}, {1, 1}, {1, 2}, {1, 3}},
		},
		kO: {
			{{1, 0}, {2, 0}, {1, 1}, {2, 1}}, {{1, 0}, {2, 0}, {1, 1}, {2, 1}},
			{{1, 0}, {2, 0}, {1, 1}, {2, 1}}, {{1, 0}, {2, 0}, {1, 1}, {2, 1}},
		},
		kT: {
			{{1, 0}, {0, 1}, {1, 1}, {2, 1}},
			{{1, 0}, {1, 1}, {2, 1}, {1, 2}},
			{{0, 1}, {1, 1}, {2, 1}, {1, 2}},
			{{1, 0}, {0, 1}, {1, 1}, {1, 2}},
		},
		kS: {
			{{1, 0}, {2, 0}, {0, 1}, {1, 1}},
			{{1, 0}, {1, 1}, {2, 1}, {2, 2}},
			{{1, 1}, {2, 1}, {0, 2}, {1, 2}},
			{{0, 0}, {0, 1}, {1, 1}, {1, 2}},
		},
		kZ: {
			{{0, 0}, {1, 0}, {1, 1}, {2, 1}},
			{{2, 0}, {1, 1}, {2, 1}, {1, 2}},
			{{0, 1}, {1, 1}, {1, 2}, {2, 2}},
			{{1, 0}, {0, 1}, {1, 1}, {0, 2}},
		},
		kJ: {
			{{0, 0}, {0, 1}, {1, 1}, {2, 1}},
			{{1, 0}, {2, 0}, {1, 1}, {1, 2}},
			{{0, 1}, {1, 1}, {2, 1}, {2, 2}},
			{{1, 0}, {1, 1}, {0, 2}, {1, 2}},
		},
		kL: {
			{{2, 0}, {0, 1}, {1, 1}, {2, 1}},
			{{1, 0}, {1, 1}, {1, 2}, {2, 2}},
			{{0, 1}, {1, 1}, {2, 1}, {0, 2}},
			{{0, 0}, {1, 0}, {1, 1}, {1, 2}},
		},
	}
	for k, rots := range want {
		for r := 0; r < 4; r++ {
			if !sameCells(shapes[k][r][:], rots[r]) {
				t.Errorf("%s rotation %d = %v, want %v", k.letter(), r, shapes[k][r], rots[r])
			}
		}
	}
}

// The kick tables were typed in both directions; a turn and its reverse
// must undo each other, offset for offset. A typo in either half breaks it.
func TestKickTablesAreSymmetric(t *testing.T) {
	for _, k := range []kind{kT, kI} {
		for a := 0; a < 4; a++ {
			for _, d := range []int{1, 3} {
				b := (a + d) % 4
				fwd, back := kickOffsets(k, a, b), kickOffsets(k, b, a)
				if fwd[0] != (point{}) {
					t.Errorf("%s %d->%d: first offset %v, want (0,0)", k.letter(), a, b, fwd[0])
				}
				distinct := false
				for i := range fwd {
					if fwd[i] != (point{-back[i].x, -back[i].y}) {
						t.Errorf("%s %d->%d offset %d = %v but %d->%d has %v", k.letter(), a, b, i, fwd[i], b, a, back[i])
					}
					if i > 0 && fwd[i] != (point{}) {
						distinct = true
					}
				}
				if !distinct {
					t.Errorf("%s %d->%d has no real kicks", k.letter(), a, b)
				}
			}
		}
	}
	// A few entries straight from the guideline, y flipped to point down.
	if got := kickOffsets(kT, 0, 1)[2]; got != (point{-1, -1}) {
		t.Errorf("T 0->R test 3 = %v, want (-1,-1)", got)
	}
	if got := kickOffsets(kI, 0, 1)[3]; got != (point{-2, 1}) {
		t.Errorf("I 0->R test 4 = %v, want (-2,1)", got)
	}
	if got := kickOffsets(kI, 3, 0)[4]; got != (point{-2, -1}) {
		t.Errorf("I L->0 test 5 = %v, want (-2,-1)", got)
	}
}

func TestIPieceWallKick(t *testing.T) {
	// Vertical, flat against the left wall: turning back to flat needs the
	// second kick, two columns right.
	g := emptyGame(kI)
	g.cur = piece{k: kI, rot: 1, x: -2, y: 5} // its column is column 0
	if !g.rotate(-1) {
		t.Fatal("the I did not turn")
	}
	if want := (piece{k: kI, rot: 0, x: 0, y: 5}); g.cur != want {
		t.Errorf("after the wall kick the I is %+v, want %+v", g.cur, want)
	}
	// Flat on the floor, turning up: the first kicks fail, the fifth lifts it
	// two rows and moves it one column.
	g = emptyGame(kI)
	g.cur = piece{k: kI, rot: 0, x: 3, y: boardH - 2}
	if !g.rotate(1) {
		t.Fatal("the I did not turn on the floor")
	}
	if want := (piece{k: kI, rot: 1, x: 4, y: boardH - 4}); g.cur != want {
		t.Errorf("after the floor kick the I is %+v, want %+v", g.cur, want)
	}
}

func TestRotationDirectionsAndLimits(t *testing.T) {
	g := emptyGame(kT)
	if !g.rotate(1) || g.cur.rot != 1 {
		t.Errorf("clockwise gave rotation %d", g.cur.rot)
	}
	g = emptyGame(kT)
	if !g.rotate(-1) || g.cur.rot != 3 {
		t.Errorf("counter-clockwise gave rotation %d", g.cur.rot)
	}
	g = emptyGame(kO)
	before := g.cur
	if g.rotate(1) || g.cur != before {
		t.Error("the O turned")
	}
	// A turn with no room does nothing at all.
	g = emptyGame(kT)
	g.cur = piece{k: kT, x: 3, y: 5}
	for _, c := range []point{{3, 4}, {4, 4}, {5, 4}, {3, 6}, {4, 6}, {5, 6}, {2, 5}, {6, 5}, {3, 3}, {4, 3}, {5, 3}, {2, 4}, {6, 4}, {2, 6}, {6, 6}, {2, 7}, {3, 7}, {4, 7}, {5, 7}, {6, 7}} {
		g.grid[c.y][c.x] = kJ
	}
	before = g.cur
	if g.rotate(1) || g.cur != before {
		t.Errorf("a boxed-in T turned: %+v", g.cur)
	}
}

func TestShiftStopsAtWalls(t *testing.T) {
	g := emptyGame(kT)
	for i := 0; i < 20; i++ {
		g.shift(-1)
	}
	if g.cur.x != 0 {
		t.Errorf("T at the left wall has x=%d, want 0", g.cur.x)
	}
	if g.shift(-1) {
		t.Error("shifted through the wall")
	}
	for i := 0; i < 20; i++ {
		g.shift(1)
	}
	if g.cur.x != boardW-3 {
		t.Errorf("T at the right wall has x=%d, want %d", g.cur.x, boardW-3)
	}
}

func TestSevenBagGivesEachPieceOncePerSeven(t *testing.T) {
	for seed := int64(1); seed <= 20; seed++ {
		g := newGame(seed, 1)
		seq := []kind{g.cur.k}
		for i := 0; i < 69; i++ {
			seq = append(seq, g.next())
		}
		for b := 0; b < 10; b++ {
			chunk := append([]kind(nil), seq[b*7:b*7+7]...)
			sort.Slice(chunk, func(i, j int) bool { return chunk[i] < chunk[j] })
			for i, k := range chunk {
				if k != kinds[i] {
					t.Fatalf("seed %d: pieces %d to %d are %v, not one of each", seed, b*7, b*7+6, seq[b*7:b*7+7])
				}
			}
		}
	}
}

func TestSeedDecidesThePieces(t *testing.T) {
	seqOf := func(seed int64) string {
		g := newGame(seed, 1)
		s := g.cur.k.letter()
		for i := 0; i < 30; i++ {
			s += g.next().letter()
		}
		return s
	}
	if seqOf(5) != seqOf(5) {
		t.Error("one seed gave two sequences")
	}
	if seqOf(5) == seqOf(6) {
		t.Error("two seeds gave the same sequence")
	}
}

func TestPreviewHoldsTheNextFive(t *testing.T) {
	g := newGame(3, 1)
	p := g.preview()
	if len(p) != queueLen {
		t.Fatalf("preview has %d pieces", len(p))
	}
	// Taking pieces moves the preview along, and it stays full.
	for i := 0; i < 30; i++ {
		first := g.preview()[0]
		if got := g.next(); got != first {
			t.Fatalf("next gave %s, the preview promised %s", got.letter(), first.letter())
		}
		if len(g.preview()) != queueLen {
			t.Fatal("the preview ran short")
		}
	}
}

func TestSpawnShowsThePieceAndFallsAtOnce(t *testing.T) {
	g := newGame(1, 1)
	visible := false
	for _, c := range g.cur.cells() {
		if c.y >= hidden {
			visible = true
		}
	}
	if !visible {
		t.Errorf("a new piece is wholly hidden: %v", g.cur.cells())
	}
	if g.cur.x != 3 || g.cur.y != 1 {
		t.Errorf("spawned at %d,%d, want 3,1 (row 0, one row fallen)", g.cur.x, g.cur.y)
	}
}

func TestBlockOutEndsTheGame(t *testing.T) {
	g := newGame(1, 1)
	g.grid[1][4] = kJ // in the way of every spawn
	g.spawn(kT)
	if !g.over || g.has {
		t.Errorf("over=%v has=%v after a blocked spawn", g.over, g.has)
	}
	// And it is reached by stacking: drop pieces in one column.
	g = newGame(1, 1)
	for i := 0; i < 100 && !g.over; i++ {
		g.hardDrop()
	}
	if !g.over {
		t.Error("a stack up to the spawn never ended the game")
	}
}

func TestLockOutEndsTheGame(t *testing.T) {
	g := emptyGame(kO)
	g.cur = piece{k: kO, x: 3, y: 0} // its cells are in rows 0 and 1, above the skyline
	g.lock()
	if !g.over {
		t.Error("a piece locked wholly above the skyline did not end the game")
	}
	// A vertical I in the left column reaches into the visible rows: no lock out.
	g = emptyGame(kI)
	g.cur = piece{k: kI, rot: 1, x: -2, y: 0}
	g.lock()
	if g.over {
		t.Error("a piece with a cell in the visible rows ended the game")
	}
}

func TestGravityByLevel(t *testing.T) {
	cases := []struct {
		level int
		ms    int64
		rows  int
	}{
		{1, 999, 0},
		{1, 1000, 1},
		{1, 2000, 2},
		{5, 355 * 3, 3},
		{10, 640, 10},
		{15, 50, 7},
	}
	for _, c := range cases {
		g := newGame(1, c.level)
		y := g.cur.y
		g.advance(c.ms)
		if got := g.cur.y - y; got != c.rows {
			t.Errorf("level %d: %d ms moved the piece %d rows, want %d", c.level, c.ms, got, c.rows)
		}
	}
	// The table only gets faster.
	for i := 1; i < maxLevel; i++ {
		if gravity[i] >= gravity[i-1] {
			t.Errorf("gravity[%d]=%d is not faster than %d", i, gravity[i], gravity[i-1])
		}
	}
	// Past the table, the last speed holds.
	g := newGame(1, maxLevel)
	g.lines = 100
	if g.gravityMs() != gravity[maxLevel-1] || g.level() != maxLevel+10 {
		t.Errorf("level %d gravity %d", g.level(), g.gravityMs())
	}
}

func TestLevelRisesEveryTenLines(t *testing.T) {
	g := newGame(1, 1)
	g.lines = 9
	if g.level() != 1 {
		t.Errorf("9 lines is level %d", g.level())
	}
	fillRows(g, 1, 9)
	dropI(g, 9)
	g.hardDrop()
	if g.lines != 10 || g.level() != 2 {
		t.Errorf("lines=%d level=%d, want 10 and 2", g.lines, g.level())
	}
	g.advance(flashMs)
	if g.gravityMs() != gravity[1] {
		t.Errorf("gravity %d at level 2, want %d", g.gravityMs(), gravity[1])
	}
	if g2 := newGame(1, 4); g2.level() != 4 {
		t.Errorf("start level 4 gave %d", g2.level())
	}
}

func TestSoftAndHardDropScore(t *testing.T) {
	g := newGame(1, 1)
	if !g.softDrop() || g.score != 1 {
		t.Errorf("soft drop scored %d", g.score)
	}
	n := g.dropDistance()
	g.hardDrop()
	if g.score != 1+2*n {
		t.Errorf("hard drop of %d rows left score %d, want %d", n, g.score, 1+2*n)
	}
	// A grounded piece does not soft drop, and scores nothing for trying.
	g = newGame(1, 1)
	g.cur.y += g.dropDistance()
	g.settle(false)
	before := g.score
	if g.softDrop() || g.score != before {
		t.Error("a grounded piece soft dropped")
	}
	if g.best != g.score {
		t.Errorf("best %d, score %d", g.best, g.score)
	}
}

func TestLineScores(t *testing.T) {
	want := map[int]int{1: 100, 2: 300, 3: 500, 4: 800}
	for n, pts := range want {
		for _, level := range []int{1, 3} {
			g := newGame(1, level)
			fillRows(g, n, 9)
			dropI(g, 9)
			g.hardDrop()
			if g.lines != n {
				t.Fatalf("%d rows: lines=%d", n, g.lines)
			}
			if g.score != pts*level {
				t.Errorf("%d rows at level %d scored %d, want %d", n, level, g.score, pts*level)
			}
			if g.last != lineName[n] {
				t.Errorf("last = %q, want %q", g.last, lineName[n])
			}
		}
	}
}

func TestComboAndItsReset(t *testing.T) {
	for _, level := range []int{1, 3} {
		g := newGame(1, level)
		// Single clears in a row: 100, then 100 + 50 x 1, then 100 + 50 x 2,
		// each times the level.
		total := 0
		for i := 0; i < 3; i++ {
			total += (100 + 50*i) * level
			fillRows(g, 1, 9)
			dropI(g, 9)
			g.hardDrop()
			g.advance(flashMs)
			if g.score != total {
				t.Fatalf("level %d clear %d: score %d, want %d (combo %d)", level, i+1, g.score, total, g.combo)
			}
		}
		// A lock that clears nothing ends the combo.
		g.hardDrop()
		if g.combo != -1 {
			t.Errorf("combo %d after a lock with no clear, want -1", g.combo)
		}
	}
}

// The gravity table is the guideline's curve, (0.8 - (L-1) x 0.007)^(L-1)
// seconds, rounded to milliseconds.
func TestGravityIsTheGuidelineCurve(t *testing.T) {
	for l := 1; l <= maxLevel; l++ {
		s := math.Pow(0.8-float64(l-1)*0.007, float64(l-1))
		if want := int64(math.Round(s * 1000)); gravity[l-1] != want {
			t.Errorf("level %d: the guideline gives %.3f ms, which rounds to %d; the table has %d", l, s*1000, want, gravity[l-1])
		}
	}
}

func TestScoreTablesAreTheDocumentedOnes(t *testing.T) {
	if lineScore != [5]int{0, 100, 300, 500, 800} {
		t.Errorf("line scores %v", lineScore)
	}
	if tspinScore != [5]int{400, 800, 1200, 1600, 1600} {
		t.Errorf("T-spin scores %v", tspinScore)
	}
	if boardW != 10 || visibleH != 20 || hidden != 2 || queueLen != 5 || lockMs != 500 || maxResets != 15 || flashMs != 200 || histLen != 20 {
		t.Error("a constant the README documents has changed")
	}
}

func TestBackToBack(t *testing.T) {
	g := newGame(1, 1)
	for i, want := range []struct {
		score int
		name  string
	}{{800, "TETRIS"}, {800 + 1200 + 50, "B2B TETRIS"}} {
		fillRows(g, 4, 9)
		dropI(g, 9)
		g.hardDrop()
		g.advance(flashMs)
		if g.score != want.score || g.last != want.name {
			t.Fatalf("tetris %d: score %d %q, want %d %q", i+1, g.score, g.last, want.score, want.name)
		}
	}
	// A single between two tetrises breaks the chain.
	before := g.score
	fillRows(g, 1, 9)
	dropI(g, 9)
	g.hardDrop()
	g.advance(flashMs)
	fillRows(g, 4, 9)
	dropI(g, 9)
	g.hardDrop()
	g.advance(flashMs)
	if g.last != "TETRIS" {
		t.Errorf("last = %q after a single between tetrises, want a plain TETRIS", g.last)
	}
	if g.score <= before {
		t.Error("nothing scored")
	}
}

// A T that turns into a slot with three corners filled is a T-spin; the same
// lock without the turn is not.
func tspinSetup() *game {
	g := emptyGame(kT)
	for x := 0; x < boardW; x++ {
		g.grid[boardH-1][x] = kJ
		if x < 3 || x > 5 {
			g.grid[boardH-2][x] = kJ
		}
	}
	g.grid[boardH-1][4] = kNone
	g.grid[boardH-3][3] = kJ // the overhang: a third filled corner
	g.cur = piece{k: kT, rot: 1, x: 3, y: boardH - 3}
	return g
}

func TestTSpinDouble(t *testing.T) {
	g := tspinSetup()
	if !g.valid(g.cur) {
		t.Fatal("the setup puts the T in the wall")
	}
	if !g.rotate(1) || g.cur.rot != 2 {
		t.Fatalf("the T did not turn into the slot: %+v", g.cur)
	}
	g.hardDrop()
	if g.lines != 2 || g.last != "T-SPIN DOUBLE" || g.score != 1200 {
		t.Errorf("lines=%d last=%q score=%d, want 2 T-SPIN DOUBLE 1200", g.lines, g.last, g.score)
	}
}

// Only two corners filled: the turn is not a T-spin.
func TestTurnWithTwoCornersIsNotATSpin(t *testing.T) {
	g := tspinSetup()
	g.grid[boardH-3][3] = kNone // take the overhang away: two corners left
	if !g.rotate(1) {
		t.Fatal("the T did not turn")
	}
	g.hardDrop()
	if g.lines != 2 || g.last != "DOUBLE" || g.score != 300 {
		t.Errorf("lines=%d last=%q score=%d, want 2 DOUBLE 300", g.lines, g.last, g.score)
	}
}

// A T-spin with no line is worth 400, and with one line 800: the two lines
// of the double need both rows full.
func TestTSpinWithFewerLines(t *testing.T) {
	g := tspinSetup()
	g.grid[boardH-1][0], g.grid[boardH-2][0] = kNone, kNone // neither row can clear
	g.rotate(1)
	g.hardDrop()
	if g.lines != 0 || g.last != "T-SPIN" || g.score != 400 {
		t.Errorf("no lines: lines=%d last=%q score=%d, want 0 T-SPIN 400", g.lines, g.last, g.score)
	}
	g = tspinSetup()
	g.grid[boardH-1][0] = kNone // the lower row cannot clear
	g.rotate(1)
	g.hardDrop()
	if g.lines != 1 || g.last != "T-SPIN SINGLE" || g.score != 800 {
		t.Errorf("one line: lines=%d last=%q score=%d, want 1 T-SPIN SINGLE 800", g.lines, g.last, g.score)
	}
}

func TestTheSameLockWithoutATurnIsAPlainDouble(t *testing.T) {
	g := tspinSetup()
	g.cur = piece{k: kT, rot: 2, x: 3, y: boardH - 3}
	g.lock()
	if g.lines != 2 || g.last != "DOUBLE" || g.score != 300 {
		t.Errorf("lines=%d last=%q score=%d, want 2 DOUBLE 300", g.lines, g.last, g.score)
	}
}

func TestTSpinThenTetrisIsBackToBack(t *testing.T) {
	g := tspinSetup()
	g.rotate(1)
	g.hardDrop()
	g.advance(flashMs)
	if !g.b2b {
		t.Fatal("a T-spin double should start a back-to-back chain")
	}
	fillRows(g, 4, 9)
	dropI(g, 9)
	g.hardDrop()
	if g.last != "B2B TETRIS" {
		t.Errorf("last = %q, want B2B TETRIS", g.last)
	}
}

func TestLineClearFlashesThenCollapses(t *testing.T) {
	g := newGame(1, 1)
	fillRows(g, 2, 9)
	g.grid[boardH-3][0] = kS // rests on the rows that will go
	dropI(g, 9)
	g.hardDrop()
	if !g.clearing || len(g.rows) != 2 || g.has {
		t.Fatalf("clearing=%v rows=%v has=%v right after the lock", g.clearing, g.rows, g.has)
	}
	if g.grid[boardH-1][0] == kNone || g.grid[boardH-2][0] == kNone {
		t.Error("the full rows left the field before the flash")
	}
	// Nothing moves while the rows flash.
	if g.shift(1) || g.rotate(1) || g.holdPiece() || g.hardDrop() || g.softDrop() {
		t.Error("the player acted during the flash")
	}
	g.advance(199)
	if !g.clearing {
		t.Fatal("the flash ended early")
	}
	g.advance(1)
	if g.clearing || !g.has {
		t.Fatalf("clearing=%v has=%v after the flash", g.clearing, g.has)
	}
	if g.grid[boardH-1][0] != kS {
		t.Errorf("the S above the cleared rows is %v, want it fallen to the floor", g.grid[boardH-1][0])
	}
	for x := 1; x < boardW; x++ {
		if x == 9 {
			continue
		}
		if g.grid[boardH-1][x] != kNone {
			t.Errorf("column %d of the floor row is filled after the clear", x)
		}
	}
}

func TestGhostLandsWhereTheHardDropDoes(t *testing.T) {
	g := newGame(2, 1)
	g.shift(2)
	ghost := g.ghost()
	n := g.dropDistance()
	g.hardDrop()
	var landed []point
	for y := 0; y < boardH; y++ {
		for x := 0; x < boardW; x++ {
			if g.grid[y][x] != kNone {
				landed = append(landed, point{x, y})
			}
		}
	}
	if !sameCells(ghost, landed) {
		t.Errorf("ghost %v, landed %v (dropped %d rows)", ghost, landed, n)
	}
	// No ghost while the rows flash or when the game is over.
	g.over = true
	if g.ghost() != nil {
		t.Error("a ghost in a finished game")
	}
}

func TestLockDelay(t *testing.T) {
	g := newGame(1, 1)
	for g.softDrop() {
	}
	pieces := g.pieces
	g.advance(499)
	if g.pieces != pieces {
		t.Fatal("locked before the delay was out")
	}
	g.advance(1)
	if g.pieces != pieces+1 {
		t.Error("did not lock when the delay was out")
	}
}

func TestMovesRestartTheLockDelayFifteenTimes(t *testing.T) {
	g := newGame(1, 1)
	for g.softDrop() {
	}
	deadline := g.lockAt
	dir := 1
	for i := 1; i <= 15; i++ {
		g.advance(100)
		if !g.shift(dir) {
			dir = -dir
			g.shift(dir)
		}
		if g.lockAt <= deadline {
			t.Fatalf("move %d did not restart the lock delay (%d, was %d)", i, g.lockAt, deadline)
		}
		deadline = g.lockAt
	}
	g.advance(100)
	if !g.shift(dir) && !g.shift(-dir) {
		t.Fatal("no room to move")
	}
	if g.lockAt != deadline {
		t.Errorf("move 16 moved the deadline from %d to %d", deadline, g.lockAt)
	}
	// The piece still locks at the old deadline.
	pieces := g.pieces
	g.advance(deadline - g.t)
	if g.pieces != pieces+1 {
		t.Error("the piece outlived its last lock delay")
	}
}

func TestATurnRestartsTheLockDelayToo(t *testing.T) {
	g := emptyGame(kT)
	g.cur.y += g.dropDistance()
	g.settle(false)
	deadline := g.lockAt
	g.advance(200)
	// On the floor the T turns by its first kick, one row up and one left.
	if !g.rotate(1) || !g.grounded() {
		t.Fatalf("the T did not turn and come to rest: %+v", g.cur)
	}
	if g.lockAt != deadline+200 {
		t.Errorf("a turn put the deadline at %d, want %d", g.lockAt, deadline+200)
	}
}

func TestFallingToANewRowRefillsTheResets(t *testing.T) {
	g := emptyGame(kO)
	for x := 0; x <= 4; x++ {
		g.grid[boardH-1][x] = kJ // a ledge
	}
	g.cur = piece{k: kO, x: 0, y: 5}
	g.cur.y += g.dropDistance()
	g.lowest, g.t = g.cur.y, 0
	g.settle(false)
	g.resets = 15
	for i := 0; i < 4; i++ { // O cells are columns x+1 and x+2: off the ledge at x=4
		g.shift(1)
	}
	if g.grounded() {
		t.Fatal("the piece is still on the ledge")
	}
	g.advance(gravity[0])
	if g.resets != 0 {
		t.Errorf("resets = %d after reaching a new lowest row, want 0", g.resets)
	}
	if g.cur.y != boardH-2 {
		t.Errorf("piece fell to row %d, want %d", g.cur.y, boardH-2)
	}
}

func TestHoldSwapsOncePerPiece(t *testing.T) {
	g := newGame(4, 1)
	first, queue0 := g.cur.k, g.preview()[0]
	if !g.holdPiece() {
		t.Fatal("the first hold did not work")
	}
	if g.hold != first || g.cur.k != queue0 {
		t.Errorf("hold=%s cur=%s, want %s held and %s falling", g.hold.letter(), g.cur.k.letter(), first.letter(), queue0.letter())
	}
	cur, hold, id := g.cur, g.hold, g.id
	if g.holdPiece() {
		t.Error("a second hold of the same piece worked")
	}
	if g.cur != cur || g.hold != hold || g.id != id {
		t.Error("the refused hold changed the game")
	}
	// After the piece locks, hold works again and gives back the held piece.
	g.hardDrop()
	falling := g.cur.k
	if !g.holdPiece() {
		t.Fatal("hold did not come back after a lock")
	}
	if g.cur.k != first || g.hold != falling {
		t.Errorf("cur=%s hold=%s, want %s and %s", g.cur.k.letter(), g.hold.letter(), first.letter(), falling.letter())
	}
	// A held piece comes back in its spawn rotation at the top.
	if g.cur.rot != 0 || g.cur.x != 3 || g.cur.y != 1 {
		t.Errorf("held piece returned as %+v", g.cur)
	}
}

func TestRestartKeepsBestAndSequence(t *testing.T) {
	g := newGame(1, 1)
	g.hardDrop()
	g.hardDrop()
	best := g.best
	if best == 0 {
		t.Fatal("hard drops scored nothing")
	}
	g.restart()
	if g.score != 0 || g.lines != 0 || g.pieces != 0 || g.over || g.best != best || g.hold != kNone {
		t.Errorf("restart left score=%d lines=%d pieces=%d over=%v best=%d hold=%v", g.score, g.lines, g.pieces, g.over, g.best, g.hold)
	}
	for y := 0; y < boardH; y++ {
		for x := 0; x < boardW; x++ {
			if g.grid[y][x] != kNone {
				t.Fatal("restart left blocks in the field")
			}
		}
	}
	if !g.has {
		t.Error("no piece after a restart")
	}
}

func TestStackHeightSampling(t *testing.T) {
	g := newGame(1, 1)
	if g.stackHeight() != 0 {
		t.Errorf("empty field has height %d", g.stackHeight())
	}
	g.hardDrop()
	if len(g.hist) != 1 || g.hist[0] < 1 {
		t.Errorf("hist = %v after one lock", g.hist)
	}
	for i := 0; i < 40; i++ {
		g.sample()
	}
	if len(g.hist) != histLen {
		t.Errorf("hist has %d samples, want at most %d", len(g.hist), histLen)
	}
	for i := 0; i < boardH; i++ {
		g.grid[i][0] = kJ
	}
	if g.stackHeight() != visibleH {
		t.Errorf("a full column has height %d, want it capped at %d", g.stackHeight(), visibleH)
	}
}
