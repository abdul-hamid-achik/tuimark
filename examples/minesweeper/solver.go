package main

// A single-point solver, for the scripted demo. It plays like a person who
// only looks at one number at a time:
//
//   - a number whose covered neighbors are all mines gets them flagged;
//   - a number whose flags are all there gets chorded (its other neighbors
//     are safe);
//   - when neither applies it guesses the covered cell with the lowest
//     chance of a mine, counted per number and compared as exact fractions.
//
// It reads only what the player sees (cell states and the numbers on open
// cells), never the mines. Every choice is made in cell order, so the same
// game gives the same moves, with no floating point in them.

type moveKind uint8

const (
	mvReveal moveKind = iota // open a covered cell
	mvFlag                   // flag a covered cell
	mvChord                  // chord an open number
)

type move struct {
	kind moveKind
	cell int
}

func (m moveKind) String() string {
	return [...]string{"reveal", "flag", "chord"}[m]
}

// covered reports whether the player could still open or flag i.
func (g *game) covered(i int) bool { return g.st[i] == stHidden || g.st[i] == stQuestion }

// nextMove is the solver's next move, or false when the game is over. On a
// fresh board it opens the middle.
func nextMove(g *game) (move, bool) {
	if g.over {
		return move{}, false
	}
	if !g.placed {
		return move{mvReveal, g.idx(g.rows/2, g.cols/2)}, true
	}
	for i := 0; i < g.cells(); i++ {
		if g.st[i] != stOpen || g.adj[i] == 0 {
			continue
		}
		cov, fl := 0, 0
		first := -1
		for _, j := range g.neighbors(i) {
			switch {
			case g.covered(j):
				cov++
				if first < 0 {
					first = j
				}
			case g.st[j] == stFlag:
				fl++
			}
		}
		if cov > 0 && int(g.adj[i]) == fl+cov {
			return move{mvFlag, first}, true
		}
	}
	for i := 0; i < g.cells(); i++ {
		if g.canChord(i) {
			return move{mvChord, i}, true
		}
	}
	return move{mvReveal, guess(g)}, true
}

// guess is the covered cell least likely to be a mine. A cell next to
// numbers takes the worst odds among them (the mines still missing around
// the number, over its covered neighbors); any other cell takes the odds
// for the whole board. Fractions compare by cross-multiplication and the
// first cell in reading order wins a tie.
func guess(g *game) int {
	coveredCells := g.cells() - g.opened - g.flags
	gn, gd := g.mines-g.flags, coveredCells
	best, bn, bd := -1, 0, 1
	for i := 0; i < g.cells(); i++ {
		if !g.covered(i) {
			continue
		}
		n, d := gn, gd
		have := false
		for _, j := range g.neighbors(i) {
			if g.st[j] != stOpen || g.adj[j] == 0 {
				continue
			}
			cov, fl := 0, 0
			for _, k := range g.neighbors(j) {
				if g.covered(k) {
					cov++
				} else if g.st[k] == stFlag {
					fl++
				}
			}
			jn, jd := int(g.adj[j])-fl, cov
			if !have || jn*d > n*jd { // the worse odds of the two
				n, d, have = jn, jd, true
			}
		}
		if best < 0 || n*bd < bn*d {
			best, bn, bd = i, n, d
		}
	}
	return best
}
