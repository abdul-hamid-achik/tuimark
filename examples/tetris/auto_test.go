package main

import (
	"fmt"
	"testing"
)

// playAuto lets the autopilot play on a game clock that moves actMs per act,
// until pieces have locked or the game ends. It returns the game.
func playAuto(seed int64, level, pieces int, actMs int64) *game {
	g := newGame(seed, level)
	var p plan
	for acts := 0; g.pieces < pieces && !g.over && acts < 200*pieces; acts++ {
		g.autoAct(&p)
		g.advance(actMs)
	}
	return g
}

func TestChoosePutsTheIOnAFourWideGap(t *testing.T) {
	g := emptyGame(kI)
	for x := 4; x < boardW; x++ {
		g.grid[boardH-1][x] = kJ
	}
	g.id++
	p := g.choose()
	if !p.ok || p.rot%2 != 0 || p.x != 0 {
		t.Errorf("plan = %+v, want the I flat at box column 0 over the gap", p)
	}
}

func TestChooseStandsTheIInANarrowWell(t *testing.T) {
	g := emptyGame(kI)
	for y := boardH - 3; y < boardH; y++ {
		for x := 0; x < boardW; x++ {
			if x != 9 {
				g.grid[y][x] = kJ
			}
		}
	}
	p := g.choose()
	if !p.ok || p.rot != 1 || p.x != 7 {
		t.Errorf("plan = %+v, want the I standing in column 9 (rotation 1, box column 7)", p)
	}
}

func TestChooseIsDeterministic(t *testing.T) {
	g := newGame(9, 1)
	first := g.choose()
	for i := 0; i < 50; i++ {
		if got := g.choose(); got != first {
			t.Fatalf("choose gave %+v then %+v", first, got)
		}
	}
}

// When every landing would lock out, there is no plan, and the autopilot
// just drops the piece where it is instead of dithering.
func TestNoPlanWhenEveryLandingLocksOut(t *testing.T) {
	g := emptyGame(kO)
	for y := hidden; y < boardH; y++ {
		for x := 0; x < 9; x++ {
			g.grid[y][x] = kJ
		}
	}
	g.cur = piece{k: kO, x: 3, y: 1}
	var p plan
	g.id++
	if g.autoAct(&p); p.ok {
		t.Errorf("plan = %+v, want none", p)
	}
	if g.pieces != 1 {
		t.Errorf("%d pieces locked by the first act, want the piece dropped where it was", g.pieces)
	}
}

// A blocked shift makes the plan give up: the piece is dropped where it is.
func TestABlockedActDropsThePieceWhereItIs(t *testing.T) {
	g := emptyGame(kT)
	g.id++
	for y := 0; y < boardH-3; y++ {
		g.grid[y][6] = kJ // a wall the T cannot cross
	}
	p := plan{id: g.id, rot: 0, x: 7, ok: true}
	g.autoAct(&p)
	if !p.fail || g.cur.x != 3 {
		t.Fatalf("plan %+v, piece at x=%d after the blocked shift", p, g.cur.x)
	}
	for i := 0; i < 20 && g.pieces == 0; i++ {
		g.autoAct(&p)
		g.advance(50)
	}
	if g.pieces != 1 {
		t.Fatal("the piece was never dropped")
	}
	for x := 0; x < boardW; x++ {
		if (x < 3 || x > 5) && g.grid[boardH-1][x] != kNone && x != 6 {
			t.Errorf("a block landed in column %d, not where the piece stood", x)
		}
	}
	if g.grid[boardH-1][4] != kT {
		t.Errorf("the T did not land in its own columns: %v", g.grid[boardH-1])
	}
}

// An act turns the piece at most one step and shifts it at most one column,
// so the moves can be watched.
func TestAnActMovesThePieceOneStep(t *testing.T) {
	g := newGame(56, 1)
	var p plan
	for g.pieces < 6 {
		before := g.cur
		id := g.id
		g.autoAct(&p)
		if g.id != id || g.over {
			continue // it locked: a new piece
		}
		if dx := g.cur.x - before.x; dx < -2 || dx > 2 {
			// A kick may add a column to a turn.
			t.Fatalf("one act moved the piece %d columns: %+v to %+v", dx, before, g.cur)
		}
		if d := (g.cur.rot - before.rot + 4) % 4; d > 1 && d != 3 {
			t.Fatalf("one act turned the piece %d quarters", d)
		}
		g.advance(50)
	}
}

// The autopilot is a good player: over hundreds of pieces it clears lines
// and survives, and what it does is the same every time.
func TestAutopilotPlaysWell(t *testing.T) {
	// At the default pace (an act every 100 ms) and at the demo's (50 ms).
	for _, actMs := range []int64{autoBaseMs, autoBaseMs / 2} {
		for _, seed := range []int64{1, 2, 3, 56} {
			g := playAuto(seed, 1, 250, actMs)
			if g.over {
				t.Errorf("%d ms an act, seed %d: the autopilot lost after %d pieces", actMs, seed, g.pieces)
			}
			if g.hold != kNone {
				t.Errorf("%d ms an act, seed %d: the autopilot used hold", actMs, seed)
			}
			t.Logf("%d ms an act, seed %d: %d lines, %d pieces, score %d", actMs, seed, g.lines, g.pieces, g.score)
			if g.lines < 80 {
				t.Errorf("%d ms an act, seed %d: %d lines in %d pieces, want at least 80", actMs, seed, g.lines, g.pieces)
			}
		}
	}
}

func TestAutopilotIsDeterministic(t *testing.T) {
	sig := func() string {
		g := playAuto(56, 1, 120, 100)
		return fmt.Sprint(g.grid, g.score, g.lines, g.pieces, g.cur)
	}
	if sig() != sig() {
		t.Error("two runs of one seed differ")
	}
}

// The weights are integers: scoring a field gives the same number whatever
// the CPU, and more lines, fewer holes, and a flatter top all score higher.
func TestScoreOrdersFields(t *testing.T) {
	var empty, withHole, withBump, withLine [boardH][boardW]kind
	for x := 0; x < boardW; x++ {
		withHole[boardH-1][x] = kJ
		withLine[boardH-1][x] = kJ
	}
	withHole[boardH-1][4] = kNone
	withHole[boardH-2][4] = kJ // a block over the gap: one hole
	withBump[boardH-1][0], withBump[boardH-2][0], withBump[boardH-3][0] = kJ, kJ, kJ
	if score(&empty) != 0 {
		t.Errorf("empty field scores %d", score(&empty))
	}
	if score(&withLine) != wLines {
		t.Errorf("a cleared line scores %d, want %d", score(&withLine), wLines)
	}
	if !(score(&withHole) < score(&withBump)) {
		t.Errorf("hole %d should score below a bump %d", score(&withHole), score(&withBump))
	}
	if !(score(&withBump) < score(&empty)) {
		t.Errorf("a tower %d should score below an empty field %d", score(&withBump), score(&empty))
	}
	// Exact value: height 3, no holes, bumpiness 3 (the tower beside a flat).
	if want := 3*wHeight + 3*wBump; score(&withBump) != want {
		t.Errorf("tower scores %d, want %d", score(&withBump), want)
	}
}

// The autopilot soft drops its pieces like a player, and the points count.
func TestAutopilotDropsScore(t *testing.T) {
	g := newGame(56, 1)
	var p plan
	for g.pieces == 0 {
		g.autoAct(&p)
	}
	if g.score < 15 {
		t.Errorf("score %d after one piece dropped from the top, want the rows it fell", g.score)
	}
}
