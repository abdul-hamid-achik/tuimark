package main

// A solver that thinks like a person: it keeps the candidates of every
// empty cell and applies named techniques, the cheapest first. How hard a
// puzzle is follows from the hardest technique it cannot do without:
//
//	level  difficulty  techniques
//	1      easy        naked single, hidden single
//	2      medium      + locked candidates (pointing, claiming), naked pair
//	3      hard        + hidden pair, X-wing
//	4      expert      anything beyond these: the solver gets stuck
//
// The grade is a property of the puzzle alone, found by running this solver
// to the end, so it is the same on every run and every machine.

const (
	levelEasy   = 1
	levelMedium = 2
	levelHard   = 3
	levelExpert = 4
)

var difficulties = []string{"easy", "medium", "hard", "expert"}

// levelName is the difficulty word of a level.
func levelName(level int) string { return difficulties[level-1] }

// levelOf is the level of a difficulty word, or 0.
func levelOf(name string) int {
	for i, d := range difficulties {
		if d == name {
			return i + 1
		}
	}
	return 0
}

// step is one thing the solver did.
type step struct {
	tech  string
	level int
	place bool // a digit was placed; otherwise candidates were removed
	idx   int  // the cell of a placement
	digit int  // its digit
}

type logicSolver struct {
	g    grid
	cand [size]uint16
}

// newLogic starts from g, with every empty cell's candidates worked out
// from the digits placed in its row, column, and box.
func newLogic(g grid) *logicSolver {
	l := &logicSolver{g: g}
	for i := 0; i < size; i++ {
		if g[i] != 0 {
			continue
		}
		m := allDigits
		for _, p := range peers[i] {
			if g[p] != 0 {
				m &^= bit(int(g[p]))
			}
		}
		l.cand[i] = m
	}
	return l
}

// place puts d in cell i and takes it from the candidates of its peers.
func (l *logicSolver) place(i, d int) {
	l.g[i] = byte(d)
	l.cand[i] = 0
	for _, p := range peers[i] {
		l.cand[p] &^= bit(d)
	}
}

// solved reports whether every cell holds a digit.
func (l *logicSolver) solved() bool {
	for _, d := range l.g {
		if d == 0 {
			return false
		}
	}
	return true
}

// contradiction reports a dead end: an empty cell with no candidate left.
func (l *logicSolver) contradiction() bool {
	for i := 0; i < size; i++ {
		if l.g[i] == 0 && l.cand[i] == 0 {
			return true
		}
	}
	return false
}

// next applies the cheapest technique that makes progress and describes it.
// It returns false when none does: the puzzle is solved, or needs more.
func (l *logicSolver) next() (step, bool) {
	if l.contradiction() {
		return step{}, false
	}
	if st, ok := l.nakedSingle(); ok {
		return st, true
	}
	if st, ok := l.hiddenSingle(); ok {
		return st, true
	}
	if l.lockedCandidates() {
		return step{tech: "locked candidates", level: levelMedium}, true
	}
	if l.nakedPair() {
		return step{tech: "naked pair", level: levelMedium}, true
	}
	if l.hiddenPair() {
		return step{tech: "hidden pair", level: levelHard}, true
	}
	if l.xwing() {
		return step{tech: "x-wing", level: levelHard}, true
	}
	return step{}, false
}

// single finds a placement by a naked or hidden single without touching
// the candidates of other cells: what a hint needs.
func (l *logicSolver) single() (step, bool) {
	if l.contradiction() {
		return step{}, false
	}
	if st, ok := l.nakedSingle(); ok {
		return st, true
	}
	return l.hiddenSingle()
}

// nakedSingle: a cell with one candidate left.
func (l *logicSolver) nakedSingle() (step, bool) {
	for i := 0; i < size; i++ {
		if l.g[i] == 0 && count(l.cand[i]) == 1 {
			d := lowest(l.cand[i])
			l.place(i, d)
			return step{tech: "naked single", level: levelEasy, place: true, idx: i, digit: d}, true
		}
	}
	return step{}, false
}

// hiddenSingle: a digit with one cell left to go in a row, column, or box.
func (l *logicSolver) hiddenSingle() (step, bool) {
	for u := 0; u < units; u++ {
		for d := 1; d <= 9; d++ {
			at, n := -1, 0
			for _, i := range unitCells[u] {
				if l.g[i] == 0 && has(l.cand[i], d) {
					at = i
					n++
				}
			}
			if n == 1 {
				l.place(at, d)
				return step{tech: "hidden single", level: levelEasy, place: true, idx: at, digit: d}, true
			}
		}
	}
	return step{}, false
}

// lockedCandidates: when a digit's places in a box all lie on one line, it
// cannot go elsewhere on that line (pointing); when its places on a line
// all lie in one box, it cannot go elsewhere in that box (claiming).
func (l *logicSolver) lockedCandidates() bool {
	progress := false
	for b := 0; b < 9; b++ {
		for d := 1; d <= 9; d++ {
			var rows, cols uint16
			n := 0
			for _, i := range unitCells[18+b] {
				if has(l.cand[i], d) {
					rows |= 1 << rowOf[i]
					cols |= 1 << colOf[i]
					n++
				}
			}
			if n < 2 {
				continue
			}
			if count(rows) == 1 {
				progress = l.remove(unitCells[lowest(rows)-1], d, func(i int) bool { return boxOf[i] != b }) || progress
			}
			if count(cols) == 1 {
				progress = l.remove(unitCells[9+lowest(cols)-1], d, func(i int) bool { return boxOf[i] != b }) || progress
			}
		}
	}
	for u := 0; u < 18; u++ {
		for d := 1; d <= 9; d++ {
			var boxes uint16
			n := 0
			for _, i := range unitCells[u] {
				if has(l.cand[i], d) {
					boxes |= 1 << boxOf[i]
					n++
				}
			}
			if n < 2 || count(boxes) != 1 {
				continue
			}
			inUnit := func(i int) bool {
				if u < 9 {
					return rowOf[i] != u
				}
				return colOf[i] != u-9
			}
			progress = l.remove(unitCells[18+lowest(boxes)-1], d, inUnit) || progress
		}
	}
	return progress
}

// remove takes digit d from the cells of a unit that pass keep.
func (l *logicSolver) remove(cells [9]int, d int, pick func(int) bool) bool {
	progress := false
	for _, i := range cells {
		if pick(i) && has(l.cand[i], d) {
			l.cand[i] &^= bit(d)
			progress = true
		}
	}
	return progress
}

// nakedPair: two cells of a unit with the same two candidates own those
// digits, so no other cell of the unit can have them.
func (l *logicSolver) nakedPair() bool {
	progress := false
	for u := 0; u < units; u++ {
		cells := unitCells[u]
		for a := 0; a < 9; a++ {
			i := cells[a]
			if l.g[i] != 0 || count(l.cand[i]) != 2 {
				continue
			}
			for b := a + 1; b < 9; b++ {
				j := cells[b]
				if l.cand[j] != l.cand[i] {
					continue
				}
				m := l.cand[i]
				for _, k := range cells {
					if k != i && k != j && l.cand[k]&m != 0 {
						l.cand[k] &^= m
						progress = true
					}
				}
			}
		}
	}
	return progress
}

// hiddenPair: two digits that fit in only the same two cells of a unit
// belong to those cells, so every other candidate there goes.
func (l *logicSolver) hiddenPair() bool {
	progress := false
	for u := 0; u < units; u++ {
		cells := unitCells[u]
		var where [10]uint16
		for d := 1; d <= 9; d++ {
			for k, i := range cells {
				if has(l.cand[i], d) {
					where[d] |= 1 << k
				}
			}
		}
		for d1 := 1; d1 <= 9; d1++ {
			if count(where[d1]) != 2 {
				continue
			}
			for d2 := d1 + 1; d2 <= 9; d2++ {
				if where[d2] != where[d1] {
					continue
				}
				pair := bit(d1) | bit(d2)
				for k, i := range cells {
					if where[d1]&(1<<k) != 0 && l.cand[i] != pair {
						l.cand[i] &= pair
						progress = true
					}
				}
			}
		}
	}
	return progress
}

// xwing: a digit that fits in the same two columns in two rows (or two
// rows in two columns) takes those columns' other cells out of play.
func (l *logicSolver) xwing() bool {
	progress := false
	for d := 1; d <= 9; d++ {
		for _, byRow := range []bool{true, false} {
			var lines [9]uint16 // for each line, which crossing positions hold d
			for line := 0; line < 9; line++ {
				for k := 0; k < 9; k++ {
					i := unitCells[line][k]
					if !byRow {
						i = unitCells[9+line][k]
					}
					if has(l.cand[i], d) {
						lines[line] |= 1 << k
					}
				}
			}
			for a := 0; a < 9; a++ {
				if count(lines[a]) != 2 {
					continue
				}
				for b := a + 1; b < 9; b++ {
					if lines[b] != lines[a] {
						continue
					}
					for k := 0; k < 9; k++ {
						if lines[a]&(1<<k) == 0 {
							continue
						}
						for line := 0; line < 9; line++ {
							if line == a || line == b {
								continue
							}
							i := k*9 + line // byRow: crossing k is the column, line the row
							if byRow {
								i = line*9 + k
							}
							if has(l.cand[i], d) {
								l.cand[i] &^= bit(d)
								progress = true
							}
						}
					}
				}
			}
		}
	}
	return progress
}

// grade is the level of g: the hardest technique it needs, or expert when
// the techniques above run out. A grid that is not uniquely solvable has
// no meaningful grade; callers check that first.
func grade(g grid) int {
	l := newLogic(g)
	level := levelEasy
	for !l.solved() {
		st, ok := l.next()
		if !ok {
			return levelExpert
		}
		if st.level > level {
			level = st.level
		}
	}
	return level
}
