package main

import "math/rand"

// A backtracking solver over candidate bitmasks with constraint propagation,
// in the manner of Peter Norvig's essay: placing a digit removes it from the
// candidates of the peers; a cell left with one candidate places it; a digit
// with one place left in a row, column, or box goes there; any cell or unit
// left with nothing is a dead end. The search then branches on the empty cell
// with the fewest candidates.
//
// It has no clock, but it does have a budget: at most searchBudget nodes. A
// legitimate puzzle needs a few dozen; a pathological one (an unsolvable grid
// with few givens can take billions of nodes to refute) ends with the budget
// spent, and the caller treats "unknown" as "not proved unique".
//
// With a nil rng the digits are tried in ascending order and ties broken by
// cell number, so a count is the same on every run. With an rng the digits and
// the ties are shuffled, which is how a random complete grid is made.

// searchBudget is the most search nodes a count may use.
const searchBudget = 200000

type solver struct {
	rng          *rand.Rand
	limit, found int
	nodes        int
	spent        bool // the budget ran out before the search finished
	first        grid
}

// candGrid is the candidates of every cell. A cell with one bit set is placed.
type candGrid [size]uint16

// unitsOfCell are the three units (row, column, box) of a cell.
func unitsOfCell(i int) [3]int { return [3]int{rowOf[i], 9 + colOf[i], 18 + boxOf[i]} }

// assign places digit d in cell i: every other candidate of the cell goes.
// It reports false on a dead end.
func (c *candGrid) assign(i, d int) bool {
	others := c[i] &^ bit(d)
	for e := 1; e <= 9; e++ {
		if has(others, e) && !c.eliminate(i, e) {
			return false
		}
	}
	return has(c[i], d)
}

// eliminate removes digit d from the candidates of cell i, and follows what
// that forces. It reports false on a dead end.
func (c *candGrid) eliminate(i, d int) bool {
	if !has(c[i], d) {
		return true
	}
	c[i] &^= bit(d)
	switch count(c[i]) {
	case 0:
		return false
	case 1:
		last := lowest(c[i])
		for _, p := range peers[i] {
			if !c.eliminate(p, last) {
				return false
			}
		}
	}
	// A unit with one place left for d puts it there.
	for _, u := range unitsOfCell(i) {
		place, n := -1, 0
		for _, j := range unitCells[u] {
			if has(c[j], d) {
				place = j
				n++
			}
		}
		switch {
		case n == 0:
			return false
		case n == 1 && count(c[place]) > 1:
			if !c.assign(place, d) {
				return false
			}
		}
	}
	return true
}

// newCandGrid loads g. It reports false when the givens contradict each other.
func newCandGrid(g grid) (candGrid, bool) {
	var c candGrid
	for i := range c {
		c[i] = allDigits
	}
	for i, d := range g {
		if d != 0 && !c.assign(i, int(d)) {
			return c, false
		}
	}
	return c, true
}

// countSolutions counts the solutions of g up to limit and returns the
// first one found. It returns 0 for a grid whose givens clash. When the
// search budget runs out first it returns what it found so far, with
// complete false: a count of 0 or 1 is then not a proof.
func countSolutions(g grid, limit int) (n int, first grid, complete bool) {
	c, ok := newCandGrid(g)
	if !ok {
		return 0, grid{}, true
	}
	s := &solver{limit: limit}
	s.search(c)
	return s.found, s.first, !s.spent
}

// randomSolution is a complete valid grid, drawn from rng.
func randomSolution(rng *rand.Rand) grid {
	c, _ := newCandGrid(grid{})
	s := &solver{rng: rng, limit: 1}
	s.search(c)
	return s.first
}

// search fills the grid by depth-first search until limit solutions are
// found or the budget is spent. It returns true when it should stop.
func (s *solver) search(c candGrid) bool {
	if s.nodes++; s.nodes > searchBudget {
		s.spent = true
		return true
	}
	best, bestN, ties := -1, 10, 0
	for i := 0; i < size; i++ {
		n := count(c[i])
		if n < 2 {
			continue
		}
		switch {
		case n < bestN:
			best, bestN, ties = i, n, 1
		case n == bestN && s.rng != nil:
			// Reservoir choice among the tied cells keeps the draw uniform.
			ties++
			if s.rng.Intn(ties) == 0 {
				best = i
			}
		}
	}
	if best < 0 { // every cell is placed
		s.found++
		if s.found == 1 {
			for i := range c {
				s.first[i] = byte(lowest(c[i]))
			}
		}
		return s.found >= s.limit
	}
	digits := make([]int, 0, 9)
	for d := 1; d <= 9; d++ {
		if has(c[best], d) {
			digits = append(digits, d)
		}
	}
	if s.rng != nil {
		s.rng.Shuffle(len(digits), func(a, b int) { digits[a], digits[b] = digits[b], digits[a] })
	}
	for _, d := range digits {
		next := c
		if next.assign(best, d) && s.search(next) {
			return true
		}
	}
	return false
}
