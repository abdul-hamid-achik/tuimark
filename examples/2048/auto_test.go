package main

import (
	"math/rand"
	"reflect"
	"testing"
)

// The search simulates the same rules as the game: for random boards, every
// slide gives the same board in both, exponents for values.
func TestSearchBoardMatchesTheGame(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	for n := minSize; n <= maxSize; n++ {
		for trial := 0; trial < 150; trial++ {
			g := newGame(n, int64(trial), defaultP4)
			for i := range g.cells {
				if rng.Intn(3) != 0 {
					g.cells[i] = 1 << uint(1+rng.Intn(6))
				} else {
					g.cells[i] = 0
				}
			}
			b := newEBoard(n, g.cells)
			for _, d := range []dir{dirUp, dirRight, dirDown, dirLeft} {
				cells, _, _, changed := g.slid(d)
				nb, moved := b.slide(d)
				if moved != changed {
					t.Fatalf("n=%d %v on %v: search moved=%v, game changed=%v", n, d, g.cells, moved, changed)
				}
				want := newEBoard(n, cells)
				if nb != want {
					t.Fatalf("n=%d %v on %v: search gave %v, game %v", n, d, g.cells, nb.c[:n*n], want.c[:n*n])
				}
			}
		}
	}
}

func TestAutopilotIsDeterministic(t *testing.T) {
	g := newGame(4, 3, defaultP4)
	for i := 0; i < 60; i++ {
		d1, ok1 := bestMove(4, g.cells, 2)
		d2, ok2 := bestMove(4, g.cells, 2)
		if d1 != d2 || ok1 != ok2 {
			t.Fatalf("move %d: %v %v then %v %v", i, d1, ok1, d2, ok2)
		}
		if !ok1 || !g.move(d1) {
			break
		}
	}
}

// Every move the autopilot picks changes the board, and it says so when no
// slide would.
func TestAutopilotOnlyPicksMovingSlides(t *testing.T) {
	for n := minSize; n <= maxSize; n++ {
		g := newGame(n, 2, defaultP4)
		for i := 0; i < 200 && !g.over; i++ {
			d, ok := bestMove(n, g.cells, 1)
			if !ok {
				t.Fatalf("n=%d move %d: no move on a board that can move", n, i)
			}
			if !g.move(d) {
				t.Fatalf("n=%d move %d: %v changes nothing", n, i, d)
			}
		}
	}
	stuck := board(4, stuckRows()...)
	stuck.move(dirRight)
	if _, ok := bestMove(4, stuck.cells, 2); ok {
		t.Error("the autopilot found a move on a board with none")
	}
}

func TestAutopilotReaches256(t *testing.T) {
	for seed := int64(1); seed <= 3; seed++ {
		g := newGame(4, seed, defaultP4)
		for i := 0; i < 600 && !g.over; i++ {
			d, ok := bestMove(4, g.cells, 2)
			if !ok {
				break
			}
			g.move(d)
		}
		if g.largest() < 256 {
			t.Errorf("seed %d: largest tile %d after %d moves, want 256 or more", seed, g.largest(), g.moves)
		}
	}
}

// A policy with no look-ahead (left, down, right, up, round and round) is
// the yardstick: the autopilot scores at least twice as much with the same seeds.
func TestAutopilotBeatsAFixedCycle(t *testing.T) {
	var auto, cycle int
	for seed := int64(1); seed <= 4; seed++ {
		g := newGame(4, seed, defaultP4)
		for i := 0; i < 400 && !g.over; i++ {
			if d, ok := bestMove(4, g.cells, 2); ok {
				g.move(d)
			}
		}
		auto += g.score
		h := newGame(4, seed, defaultP4)
		order := []dir{dirLeft, dirDown, dirRight, dirUp}
		for i := 0; i < 400 && !h.over; i++ {
			for k := 0; k < 4; k++ {
				if h.move(order[(i+k)%4]) {
					break
				}
			}
		}
		cycle += h.score
	}
	if auto < 2*cycle {
		t.Errorf("autopilot scored %d, the fixed cycle %d: want at least twice as much", auto, cycle)
	}
}

func TestAutopilotPlaysSmallBoards(t *testing.T) {
	for n := minSize; n <= maxSize; n++ {
		g := newGame(n, 1, defaultP4)
		for i := 0; i < 80 && !g.over; i++ {
			d, ok := bestMove(n, g.cells, defaultDepth)
			if !ok || !g.move(d) {
				t.Fatalf("n=%d: stalled at move %d", n, i)
			}
		}
		if g.score == 0 {
			t.Errorf("n=%d: no merge in 80 moves", n)
		}
	}
}

func TestSearchDeepensWhenCrowded(t *testing.T) {
	crowded := board(4, []int{2, 4, 8, 16}, []int{16, 8, 4, 2}, []int{2, 4, 8, 16}, []int{0, 0, 0, 2})
	if got := searchDepth(newEBoard(4, crowded.cells), 2); got != 3 {
		t.Errorf("3 empties: depth %d, want 3", got)
	}
	roomy := board(4, []int{2, 4, 8, 16}, []int{16, 8, 4, 2}, []int{2, 4, 8, 16}, []int{0, 0, 0, 0})
	if got := searchDepth(newEBoard(4, roomy.cells), 2); got != 2 {
		t.Errorf("4 empties: depth %d, want 2", got)
	}
}

// The heuristic prefers a board with room and a tidy corner to one without.
func TestEvalPrefersRoomAndOrder(t *testing.T) {
	tidy := newEBoard(4, board(4, []int{64, 32, 16, 8}, []int{0, 0, 0, 4}).cells)
	messy := newEBoard(4, board(4, []int{8, 64, 4, 32}, []int{16, 0, 0, 4}).cells)
	if tidy.eval() <= messy.eval() {
		t.Errorf("eval(tidy)=%d <= eval(messy)=%d", tidy.eval(), messy.eval())
	}
	roomy := newEBoard(4, board(4, []int{4, 2}).cells)
	full := newEBoard(4, board(4, []int{4, 2, 4, 2}, []int{2, 4, 2, 4}, []int{4, 2, 4, 2}, []int{2, 4, 2, 4}).cells)
	if roomy.eval() <= full.eval() {
		t.Errorf("eval(roomy)=%d <= eval(full)=%d", roomy.eval(), full.eval())
	}
}

func TestSearchCopiesTheBoard(t *testing.T) {
	g := newGame(4, 9, defaultP4)
	before := append([]int(nil), g.cells...)
	bestMove(4, g.cells, 3)
	if !reflect.DeepEqual(before, g.cells) {
		t.Error("bestMove changed the cells it was given")
	}
}
