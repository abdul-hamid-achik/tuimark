package main

import (
	"math/rand"
	"strings"
	"testing"
)

// fromGrid is a game with its mines laid by hand: '*' is a mine, anything
// else is safe. The game counts as started.
func fromGrid(rows ...string) *game {
	g := newGame(len(rows[0]), len(rows), 0)
	for r, line := range rows {
		for c, ch := range line {
			if ch == '*' {
				g.mine[g.idx(r, c)] = true
				g.mines++
			}
		}
	}
	for i := range g.adj {
		for _, n := range g.neighbors(i) {
			if g.mine[n] {
				g.adj[i]++
			}
		}
	}
	g.placed = true
	return g
}

func (g *game) at(r, c int) int { return g.idx(r, c) }

// shown draws the board as the player sees it.
func (g *game) shown() string {
	var b strings.Builder
	for r := 0; r < g.rows; r++ {
		for c := 0; c < g.cols; c++ {
			i := g.idx(r, c)
			switch g.st[i] {
			case stHidden:
				b.WriteByte('#')
			case stFlag:
				b.WriteByte('F')
			case stQuestion:
				b.WriteByte('?')
			default:
				if g.adj[i] == 0 {
					b.WriteByte('.')
				} else {
					b.WriteByte('0' + g.adj[i])
				}
			}
		}
		if r < g.rows-1 {
			b.WriteByte('/')
		}
	}
	return b.String()
}

func TestPlaceLeavesTheFirstCellAndItsNeighborsClear(t *testing.T) {
	for _, b := range presets {
		for _, first := range []int{0, b.cols - 1, (b.rows - 1) * b.cols, b.rows*b.cols - 1, b.rows/2*b.cols + b.cols/2, 1, b.cols} {
			for seed := int64(0); seed < 40; seed++ {
				g := newGame(b.cols, b.rows, b.mines)
				g.reveal(first, rand.New(rand.NewSource(seed)))
				n := 0
				for _, m := range g.mine {
					if m {
						n++
					}
				}
				if n != b.mines {
					t.Fatalf("%s seed %d: %d mines, want %d", b.name, seed, n, b.mines)
				}
				if g.mine[first] || g.adj[first] != 0 {
					t.Fatalf("%s seed %d: first cell %d is a mine or has a number", b.name, seed, first)
				}
				for _, nb := range g.neighbors(first) {
					if g.mine[nb] {
						t.Fatalf("%s seed %d: a mine next to the first cell", b.name, seed)
					}
				}
				if g.opened < 1+len(g.neighbors(first)) {
					t.Fatalf("%s seed %d: the first reveal opened %d cells, want at least the cell and its neighbors", b.name, seed, g.opened)
				}
				if g.over {
					t.Fatalf("%s seed %d: the first reveal ended the game", b.name, seed)
				}
			}
		}
	}
}

// The densest board the bounds allow still has room for the clear zone.
func TestPlaceOnTheDensestBoard(t *testing.T) {
	lo, hi := minesRange(minCols, minRows)
	if lo != 1 || hi != minCols*minRows-10 {
		t.Fatalf("minesRange = %d, %d", lo, hi)
	}
	for first := 0; first < minCols*minRows; first++ {
		g := newGame(minCols, minRows, hi)
		g.reveal(first, rand.New(rand.NewSource(3)))
		if g.over || g.mine[first] {
			t.Fatalf("first %d: over=%v mine=%v", first, g.over, g.mine[first])
		}
	}
}

func TestPlaceIsDeterministicInTheSeed(t *testing.T) {
	lay := func(seed int64) string {
		g := newGame(16, 16, 40)
		g.reveal(g.idx(8, 8), rand.New(rand.NewSource(seed)))
		var b strings.Builder
		for _, m := range g.mine {
			if m {
				b.WriteByte('*')
			} else {
				b.WriteByte('.')
			}
		}
		return b.String()
	}
	if lay(5) != lay(5) {
		t.Error("the same seed laid different mines")
	}
	if lay(5) == lay(6) {
		t.Error("two seeds laid the same mines")
	}
}

func TestFloodFillOpensZerosAndTheirBorder(t *testing.T) {
	g := fromGrid(
		"....*",
		".....",
		".....",
		"*....",
	)
	g.reveal(g.at(2, 2), nil)
	// One zero opens the whole board but the mines, which the win flags.
	if got, want := g.shown(), "...1F/...11/11.../F1..."; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
	if !g.won {
		t.Error("opening the only region should win")
	}
}

func TestFloodFillStopsAtNumbersAndFlags(t *testing.T) {
	g := fromGrid(
		"*....",
		".....",
		".....",
		".....",
	)
	g.st[g.at(2, 2)] = stFlag // a flag in the way is not opened
	g.flags = 1
	g.reveal(g.at(3, 4), nil)
	if g.st[g.at(2, 2)] != stFlag {
		t.Error("the flood opened a flagged cell")
	}
	if g.st[g.at(0, 0)] != stHidden {
		t.Error("the flood opened the mine")
	}
	if g.st[g.at(1, 1)] != stOpen || g.adj[g.at(1, 1)] != 1 {
		t.Errorf("the border number was not opened:\n%s", strings.ReplaceAll(g.shown(), "/", "\n"))
	}
	if g.won {
		t.Error("won with the flagged cell still covered")
	}
}

func TestFloodFillOpensQuestionMarks(t *testing.T) {
	g := fromGrid("....*", ".....", ".....")
	g.st[g.at(2, 0)] = stQuestion
	g.reveal(g.at(2, 2), nil)
	if g.st[g.at(2, 0)] != stOpen {
		t.Error("a question mark was left covered by the flood")
	}
}

func TestRevealIgnoresFlagsAndOpenCells(t *testing.T) {
	g := fromGrid("*..", "...", "...")
	g.cycle(g.at(0, 0))
	if g.reveal(g.at(0, 0), nil) || g.over {
		t.Error("revealing a flagged mine did something")
	}
	g.reveal(g.at(2, 2), nil)
	if g.reveal(g.at(2, 2), nil) {
		t.Error("revealing an open cell reported a change")
	}
}

func TestRevealAMineLosesAndNamesIt(t *testing.T) {
	g := fromGrid("*..", "...", "...")
	if !g.reveal(g.at(0, 0), nil) {
		t.Fatal("no change")
	}
	if !g.over || g.won || g.boom != g.at(0, 0) {
		t.Errorf("over=%v won=%v boom=%d", g.over, g.won, g.boom)
	}
	if g.reveal(g.at(2, 2), nil) {
		t.Error("a move after the game ended changed something")
	}
}

func TestFlagCycle(t *testing.T) {
	g := fromGrid("*..", "...", "...")
	i := g.at(1, 1)
	want := []struct {
		st    cellState
		flags int
	}{{stFlag, 1}, {stQuestion, 0}, {stHidden, 0}, {stFlag, 1}}
	for k, w := range want {
		if !g.cycle(i) || g.st[i] != w.st || g.flags != w.flags {
			t.Fatalf("step %d: state %d flags %d, want %d, %d", k, g.st[i], g.flags, w.st, w.flags)
		}
	}
	g.st[i] = stOpen
	if g.cycle(i) {
		t.Error("an open cell took a flag")
	}
}

func TestMinesLeftGoesNegative(t *testing.T) {
	g := fromGrid("*..", "...", "...")
	g.cycle(g.at(1, 1))
	g.cycle(g.at(1, 2))
	if g.minesLeft() != -1 {
		t.Errorf("minesLeft = %d, want -1", g.minesLeft())
	}
	g.cycle(g.at(1, 1)) // to a question mark: not counted
	if g.minesLeft() != 0 {
		t.Errorf("minesLeft = %d, want 0", g.minesLeft())
	}
}

func TestChordOpensTheRestWhenFlagsMatch(t *testing.T) {
	g := fromGrid(
		".*...",
		"..*..",
		".....",
		".....",
	)
	one := g.at(2, 2)
	g.reveal(one, nil) // a lone 1: the mine is at (1,2)
	if g.st[one] != stOpen || g.adj[one] != 1 {
		t.Fatalf("setup: (2,2) should be an open 1:\n%s", strings.ReplaceAll(g.shown(), "/", "\n"))
	}
	if g.chord(one) {
		t.Fatal("chorded with no flag")
	}
	// Flag the mine next to it and chord: the covered neighbors open.
	g.cycle(g.at(1, 2))
	before := g.opened
	if !g.chord(one) {
		t.Fatal("chord did nothing with the flag in place")
	}
	if g.opened <= before {
		t.Errorf("chord opened %d cells", g.opened-before)
	}
	for _, n := range g.neighbors(one) {
		if n != g.at(1, 2) && g.st[n] != stOpen {
			t.Errorf("neighbor %d is still covered", n)
		}
	}
}

func TestChordNeedsExactlyMatchingFlags(t *testing.T) {
	g := fromGrid(
		"*.*..",
		".....",
		".....",
	)
	two := g.at(1, 1)
	g.reveal(two, nil)
	if g.adj[two] != 2 || g.st[two] != stOpen {
		t.Fatalf("setup: (1,1) should be an open 2:\n%s", strings.ReplaceAll(g.shown(), "/", "\n"))
	}
	g.cycle(g.at(0, 0))
	if g.chord(two) {
		t.Error("chorded a 2 with one flag")
	}
	g.cycle(g.at(0, 2))
	g.cycle(g.at(0, 1)) // a third flag, on a safe cell: too many
	if g.chord(two) {
		t.Error("chorded a 2 with three flags")
	}
}

func TestChordWithAWrongFlagLoses(t *testing.T) {
	g := fromGrid(
		".*...",
		".....",
		".....",
	)
	one := g.at(1, 0) // a 1 next to the mine at (0,1)
	g.reveal(one, nil)
	if g.st[one] != stOpen || g.adj[one] != 1 {
		t.Fatalf("setup:\n%s", strings.ReplaceAll(g.shown(), "/", "\n"))
	}
	g.cycle(g.at(0, 0)) // flag the wrong cell
	if !g.chord(one) || !g.over || g.won {
		t.Fatalf("over=%v won=%v: chording past a wrong flag should lose", g.over, g.won)
	}
	if g.boom != g.at(0, 1) {
		t.Errorf("boom = %d, want the mine at (0,1)", g.boom)
	}
}

func TestChordOnlyOnOpenNumbers(t *testing.T) {
	g := fromGrid("*...", "....", "....")
	if g.chord(g.at(1, 1)) {
		t.Error("chorded a covered cell")
	}
	g.reveal(g.at(2, 3), nil)
	if z := g.at(2, 3); g.adj[z] != 0 || g.chord(z) {
		t.Error("chorded a zero")
	}
}

func TestWinFlagsTheMines(t *testing.T) {
	g := fromGrid("*..", "...", "...")
	g.reveal(g.at(2, 2), nil)
	if !g.over || !g.won {
		t.Fatalf("over=%v won=%v", g.over, g.won)
	}
	if g.st[g.at(0, 0)] != stFlag || g.flags != 1 || g.minesLeft() != 0 {
		t.Errorf("mine state %d, flags %d, left %d", g.st[g.at(0, 0)], g.flags, g.minesLeft())
	}
}

// The solver reads only what the player sees, and never flags a safe cell
// or chords into a mine: on every seed the only way it loses is a guess.
func TestSolverLosesOnlyOnAGuess(t *testing.T) {
	for _, b := range presets[:2] {
		for seed := int64(1); seed <= 25; seed++ {
			g := newGame(b.cols, b.rows, b.mines)
			rng := rand.New(rand.NewSource(seed))
			guesses := 0
			for moves := 0; moves < 2000; moves++ {
				m, ok := nextMove(g)
				if !ok {
					break
				}
				switch m.kind {
				case mvFlag:
					if !g.mine[m.cell] {
						t.Fatalf("%s seed %d: flagged safe cell %d", b.name, seed, m.cell)
					}
					g.cycle(m.cell)
				case mvChord:
					g.chord(m.cell)
					if g.over && !g.won {
						t.Fatalf("%s seed %d: a chord lost", b.name, seed)
					}
				case mvReveal:
					if g.placed {
						guesses++
					}
					lost := g.mine[m.cell]
					g.reveal(m.cell, rng)
					if g.over && !g.won && !lost {
						t.Fatalf("%s seed %d: lost on a safe cell", b.name, seed)
					}
				}
			}
			if !g.over {
				t.Fatalf("%s seed %d: the solver stopped in the middle of a game", b.name, seed)
			}
			if !g.won && guesses == 0 {
				t.Fatalf("%s seed %d: lost without guessing", b.name, seed)
			}
		}
	}
}
