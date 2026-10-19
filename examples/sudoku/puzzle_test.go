package main

import (
	"math/rand"
	"strings"
	"testing"
	"time"
)

const (
	wikiPuzzle   = "530070000600195000098000060800060003400803001700020006060000280000419005000080079"
	wikiSolution = "534678912672195348198342567859761423426853791713924856961537284287419635345286179"
)

func mustGrid(t testing.TB, s string) grid {
	t.Helper()
	g, err := parseGrid(s)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

// valid reports whether g is a complete grid with no repeated digit.
func valid(g grid) bool {
	for _, d := range g {
		if d < 1 || d > 9 {
			return false
		}
	}
	return !g.conflicts()
}

func TestGeometry(t *testing.T) {
	for i := 0; i < size; i++ {
		seen := map[int]bool{}
		for _, p := range peers[i] {
			if p == i || seen[p] {
				t.Fatalf("cell %d: peer %d repeated or itself", i, p)
			}
			seen[p] = true
			if rowOf[p] != rowOf[i] && colOf[p] != colOf[i] && boxOf[p] != boxOf[i] {
				t.Fatalf("cell %d: peer %d shares no unit", i, p)
			}
		}
		if len(seen) != 20 {
			t.Fatalf("cell %d has %d peers, want 20", i, len(seen))
		}
	}
	for u, cells := range unitCells {
		seen := map[int]bool{}
		for _, i := range cells {
			seen[i] = true
			switch {
			case u < 9 && rowOf[i] != u, u >= 9 && u < 18 && colOf[i] != u-9, u >= 18 && boxOf[i] != u-18:
				t.Fatalf("unit %d holds cell %d", u, i)
			}
		}
		if len(seen) != 9 {
			t.Fatalf("unit %d has %d distinct cells", u, len(seen))
		}
	}
}

func TestParseGrid(t *testing.T) {
	g := mustGrid(t, strings.Replace(wikiPuzzle, "0", ".", -1))
	if g.String() != wikiPuzzle {
		t.Error("dots and zeros do not read the same")
	}
	for _, bad := range []string{"", wikiPuzzle[:80], wikiPuzzle + "0", strings.Replace(wikiPuzzle, "5", "x", 1), strings.Replace(wikiPuzzle, "5", " ", 1)} {
		if _, err := parseGrid(bad); err == nil {
			t.Errorf("parseGrid(%q) accepted", bad)
		}
	}
}

func TestCountSolutions(t *testing.T) {
	n, sol, _ := countSolutions(mustGrid(t, wikiPuzzle), 2)
	if n != 1 || sol.String() != wikiSolution {
		t.Errorf("the classic puzzle: %d solutions, first %s", n, sol)
	}
	// An empty grid has many solutions: the count stops at the limit.
	if n, _, _ := countSolutions(grid{}, 2); n != 2 {
		t.Errorf("empty grid: %d, want 2 (the limit)", n)
	}
	// Removing the corners of a deadly rectangle leaves exactly two solutions.
	if two := twoSolutions(t); two == "" {
		t.Error("no deadly rectangle found in the classic solution")
	} else if n, _, _ := countSolutions(mustGrid(t, two), 3); n != 2 {
		t.Errorf("twoSolutions gave %d solutions", n)
	}
	// Clashing givens have no solution.
	g := mustGrid(t, wikiPuzzle)
	g[1] = 5
	if n, _, _ := countSolutions(g, 2); n != 0 {
		t.Errorf("clashing givens: %d solutions", n)
	}
	// Valid givens that cannot be completed.
	dead := grid{}
	for c := 0; c < 8; c++ {
		dead[c] = byte(c + 1)
	}
	dead[9+8] = 9 // the ninth digit of row 0 is blocked by column 8's 9
	if n, _, _ := countSolutions(dead, 2); n != 0 {
		t.Errorf("unsolvable grid: %d solutions", n)
	}
}

// twoSolutions is a puzzle with exactly two solutions: the classic solution
// with the four corners of a "deadly rectangle" removed (two rows and two
// columns whose crossings hold a b / b a), found by search.
func twoSolutions(t testing.TB) string {
	t.Helper()
	full := mustGrid(t, wikiSolution)
	for r1 := 0; r1 < 9; r1++ {
		for r2 := r1 + 1; r2 < 9; r2++ {
			for c1 := 0; c1 < 9; c1++ {
				for c2 := c1 + 1; c2 < 9; c2++ {
					a, b, c, d := r1*9+c1, r1*9+c2, r2*9+c1, r2*9+c2
					if full[a] != full[d] || full[b] != full[c] {
						continue
					}
					g := full
					g[a], g[b], g[c], g[d] = 0, 0, 0, 0
					if n, _, _ := countSolutions(g, 3); n == 2 {
						return g.String()
					}
				}
			}
		}
	}
	return ""
}

func TestRandomSolution(t *testing.T) {
	a := randomSolution(rand.New(rand.NewSource(5)))
	b := randomSolution(rand.New(rand.NewSource(5)))
	c := randomSolution(rand.New(rand.NewSource(6)))
	if !valid(a) || !valid(c) {
		t.Fatalf("not valid grids:\n%s\n%s", a, c)
	}
	if a != b {
		t.Error("the same seed gave two grids")
	}
	if a == c {
		t.Error("two seeds gave one grid")
	}
}

// Every difficulty yields, from a spread of seeds, a puzzle with exactly one
// solution whose logic grade is the difficulty asked, with at least the
// clues of its floor, and the same puzzle again from the same seed.
func TestGenerate(t *testing.T) {
	for level := levelEasy; level <= levelExpert; level++ {
		for _, seed := range []int64{1, 2, 3} {
			p := generateCached(seed, level)
			if n, sol, done := countSolutions(p.puz, 2); !done || n != 1 || sol != p.sol {
				t.Fatalf("%s seed %d: %d solutions", levelName(level), seed, n)
			}
			if !valid(p.sol) {
				t.Fatalf("%s seed %d: the solution is not a valid grid", levelName(level), seed)
			}
			for i, d := range p.puz {
				if d != 0 && d != p.sol[i] {
					t.Fatalf("%s seed %d: given %d differs from the solution", levelName(level), seed, i)
				}
			}
			if p.level != level || grade(p.puz) != level {
				t.Errorf("%s seed %d: graded %d", levelName(level), seed, p.level)
			}
			if p.puz.clues() < floors[level] {
				t.Errorf("%s seed %d: %d clues, floor %d", levelName(level), seed, p.puz.clues(), floors[level])
			}
			if again := generate(seed, level); again != p {
				t.Errorf("%s seed %d: a second run gave another puzzle", levelName(level), seed)
			}
		}
	}
	if generateCached(1, levelEasy) == generateCached(2, levelEasy) {
		t.Error("two seeds gave one puzzle")
	}
}

// The harder the level, the fewer clues it keeps, on average.
func TestDifficultiesAreOrderedByClues(t *testing.T) {
	avg := func(level int) float64 {
		sum := 0
		for seed := int64(1); seed <= 3; seed++ {
			sum += generateCached(seed, level).puz.clues()
		}
		return float64(sum) / 3
	}
	easy, medium, hard := avg(levelEasy), avg(levelMedium), avg(levelHard)
	if !(easy > medium && medium > hard) {
		t.Errorf("average clues easy %.1f, medium %.1f, hard %.1f: not decreasing", easy, medium, hard)
	}
}

// fromGivens accepts only a puzzle with exactly one solution.
func TestFromGivens(t *testing.T) {
	p, err := fromGivens(wikiPuzzle)
	if err != nil || p.sol.String() != wikiSolution || p.level != levelEasy {
		t.Fatalf("the classic puzzle: %v, level %d", err, p.level)
	}
	if _, err := fromGivens(strings.Replace(wikiPuzzle, "0", ".", -1)); err != nil {
		t.Errorf("dots: %v", err)
	}
	cases := map[string]string{
		"too short":       wikiPuzzle[:70],
		"bad character":   strings.Replace(wikiPuzzle, "5", "x", 1),
		"repeated digit":  strings.Replace(wikiPuzzle, "0", "5", 1),
		"empty grid":      strings.Repeat("0", 81),
		"several answers": twoSolutions(t),
	}
	for name, s := range cases {
		if _, err := fromGivens(s); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// And each message says which.
	_, err = fromGivens(strings.Repeat("0", 81))
	if err == nil || !strings.Contains(err.Error(), "more than one") {
		t.Errorf("empty grid: %v", err)
	}
	_, err = fromGivens(strings.Replace(wikiPuzzle, "0", "5", 1))
	if err == nil || !strings.Contains(err.Error(), "repeat") {
		t.Errorf("repeated digit: %v", err)
	}
	dead := make([]byte, 81)
	for i := range dead {
		dead[i] = '0'
	}
	for c := 0; c < 8; c++ {
		dead[c] = byte('1' + c)
	}
	dead[17] = '9'
	_, err = fromGivens(string(dead))
	if err == nil || !strings.Contains(err.Error(), "no solution") {
		t.Errorf("unsolvable: %v", err)
	}
}

// Norvig's "impossible" puzzle (it has no solution, and a plain search takes
// seconds to say so) is answered at once: either "no solution" or, if the
// search budget is spent first, a message that says it gave up.
func TestFromGivensIsBounded(t *testing.T) {
	const impossible = ".....5.8....6.1.43..........1.5........1.6...3.......553.....61........4........."
	start := time.Now()
	_, err := fromGivens(impossible)
	if err == nil {
		t.Fatal("the impossible puzzle was accepted")
	}
	if !strings.Contains(err.Error(), "no solution") && !strings.Contains(err.Error(), "gave up") {
		t.Errorf("message %q", err)
	}
	// A plain search takes about ten seconds; the budget makes it a
	// fraction of one (several times that under the race detector).
	if d := time.Since(start); d > 6*time.Second {
		t.Errorf("took %v to refuse the impossible puzzle", d)
	}
	// An empty grid is refused fast too, as having several solutions.
	start = time.Now()
	if _, err := fromGivens(strings.Repeat("0", 81)); err == nil || time.Since(start) > 3*time.Second {
		t.Errorf("empty grid: %v in %v", err, time.Since(start))
	}
}

// A complete grid is refused: there is nothing to play, and a game that can
// never be won would run its clock for ever.
func TestCompleteGridIsRefused(t *testing.T) {
	_, err := fromGivens(wikiSolution)
	if err == nil || !strings.Contains(err.Error(), "no blanks") {
		t.Errorf("a complete grid: %v", err)
	}
	if _, err := (options{puzzle: wikiSolution}).startPuzzle(); err == nil {
		t.Error("-puzzle accepted a complete grid")
	}
}

// The search budget is a node count, not a time: a grid that needs more
// than the budget reports it spent, whatever the machine.
func TestSearchBudget(t *testing.T) {
	g := mustGrid(t, ".....5.8....6.1.43..........1.5........1.6...3.......553.....61........4.........")
	if n, _, done := countSolutions(g, 2); done && n != 0 {
		t.Errorf("impossible puzzle: %d solutions, complete %v", n, done)
	}
	if n, _, done := countSolutions(mustGrid(t, wikiPuzzle), 2); !done || n != 1 {
		t.Errorf("the classic puzzle: %d solutions, complete %v", n, done)
	}
}
