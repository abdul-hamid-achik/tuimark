package main

import (
	"fmt"
	"math/rand"
	"sync"
)

// A puzzle with its solution and the grade the logic solver gives it.
type puzzle struct {
	puz   grid // the givens
	sol   grid // the one solution
	level int  // 1 easy .. 4 expert, as graded by grade
}

// Generation is seeded randomized backtracking, then removal:
//
//  1. A complete grid comes from the solver with a seeded random digit order.
//  2. Cells are visited in a seeded random order. A cell's clue is removed
//     when the puzzle still has exactly one solution (counted up to 2) and
//     the logic solver still grades it no harder than the difficulty asked.
//  3. The result is kept when its grade is exactly that difficulty. Else the
//     removal is tried again with the next order, up to attempts times on a
//     grid and on up to grids grids; if none gives the grade, the closest
//     result wins (the highest grade, then the fewest clues).
//
// Removal stops at the clue floor of the difficulty (floors), or when no
// clue can be removed. Every random
// choice is drawn from rand.New(rand.NewSource(seed)): the same seed and
// difficulty give the same puzzle on every machine.
const (
	attempts = 40 // removal orders tried on one complete grid
	grids    = 4  // complete grids tried
)

// floors is the fewest clues each level keeps (by level, index 0 unused):
// without a floor, removal leaves puzzles far sparser than the name says.
var floors = [...]int{0, 38, 30, 26, 0}

// generate makes a puzzle of the given level from seed. It tries up to
// grids complete grids; when none of them gives the level it returns the
// closest puzzle found, whose level says what it really is.
func generate(seed int64, level int) puzzle { return generateFloor(seed, level, floors[level]) }

// generateFloor is generate with its own clue floor: the demo wants an easy
// puzzle with few blanks.
func generateFloor(seed int64, level, floor int) puzzle {
	rng := rand.New(rand.NewSource(seed))
	var best puzzle
	for g := 0; g < grids; g++ {
		sol := randomSolution(rng)
		for a := 0; a < attempts; a++ {
			p := carve(rng, sol, level, floor)
			if p.level == level {
				return p
			}
			if best.level == 0 || p.level > best.level || (p.level == best.level && p.puz.clues() < best.puz.clues()) {
				best = p
			}
		}
	}
	return best
}

// carve removes clues from a copy of sol in a random order.
func carve(rng *rand.Rand, sol grid, level, floor int) puzzle {
	g := sol
	clues := size
	for _, i := range rng.Perm(size) {
		if clues <= floor {
			break
		}
		d := g[i]
		g[i] = 0
		if n, _, done := countSolutions(g, 2); !done || n != 1 || grade(g) > level {
			g[i] = d
			continue
		}
		clues--
	}
	return puzzle{puz: g, sol: sol, level: grade(g)}
}

var (
	cacheMu sync.Mutex
	cache   = map[string]puzzle{}
)

// generateCached is generate with a memo: the result is a pure function of
// its arguments, and tests and handlers ask for the same few puzzles often.
func generateCached(seed int64, level int) puzzle {
	key := fmt.Sprintf("%d/%d", seed, level)
	cacheMu.Lock()
	p, ok := cache[key]
	cacheMu.Unlock()
	if ok {
		return p
	}
	p = generate(seed, level)
	cacheMu.Lock()
	cache[key] = p
	cacheMu.Unlock()
	return p
}

// fromGivens checks a puzzle typed in by a person: 81 characters, at least one
// blank, exactly one solution. It returns the puzzle with its grade. The
// check has a budget (searchBudget nodes): a puzzle that spends it is refused
// with a message that says so, not left to run.
func fromGivens(s string) (puzzle, error) {
	g, err := parseGrid(s)
	if err != nil {
		return puzzle{}, err
	}
	if g.clues() == size {
		return puzzle{}, fmt.Errorf("the puzzle has no blanks: nothing to solve")
	}
	if g.conflicts() {
		return puzzle{}, fmt.Errorf("the givens repeat a digit in a row, column, or box")
	}
	n, sol, done := countSolutions(g, 2)
	switch {
	case n >= 2:
		return puzzle{}, fmt.Errorf("the puzzle has more than one solution")
	case !done:
		return puzzle{}, fmt.Errorf("the puzzle could not be checked: the search gave up after %d steps (too few givens, or no solution)", searchBudget)
	case n == 0:
		return puzzle{}, fmt.Errorf("the puzzle has no solution")
	}
	return puzzle{puz: g, sol: sol, level: grade(g)}, nil
}
