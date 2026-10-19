package main

import "fmt"

// The rules of play, with no store and no clock: a game is a puzzle plus
// what the player did to it. Every method is a pure function of the game
// and its arguments, so the tests drive it directly.

// change is one cell's before and after, the unit of undo and redo.
type change struct {
	idx        int
	oldV, newV byte
	oldN, newN uint16
}

type game struct {
	puzzle
	vals     grid         // givens and entries
	notes    [size]uint16 // pencil marks, a digit set per cell
	marks    [size]bool   // entries that check found wrong, until they change
	cur      int          // the cursor
	pencil   bool         // notes mode: digits toggle pencil marks
	undo     [][]change   // one element per move, a placement and the notes it cleared together
	redo     [][]change
	mistakes int // entries that differ from the solution, counted when placed
	hints    int // hints used
	won      bool
}

// newGame starts p with the cursor on the first empty cell.
func newGame(p puzzle) *game {
	g := &game{puzzle: p, vals: p.puz}
	for i, d := range p.puz {
		if d == 0 {
			g.cur = i
			break
		}
	}
	return g
}

func (g *game) given(i int) bool { return g.puz[i] != 0 }

// filled is how many cells hold a digit.
func (g *game) filled() int { return g.vals.clues() }

// placed is how many times digit d is on the board.
func (g *game) placed(d int) int {
	n := 0
	for _, v := range g.vals {
		if int(v) == d {
			n++
		}
	}
	return n
}

// moveCursor moves by rows and columns, stopping at the edges.
func (g *game) moveCursor(dr, dc int) {
	r, c := rowOf[g.cur]+dr, colOf[g.cur]+dc
	if r < 0 || r > 8 || c < 0 || c > 8 {
		return
	}
	g.cur = r*9 + c
}

// commit applies a move and files it for undo. It clears redo, marks the
// game won when the board equals the solution, and reports whether there
// was anything to do.
func (g *game) commit(cs []change) bool {
	if len(cs) == 0 {
		return false
	}
	g.apply(cs, true)
	g.undo = append(g.undo, cs)
	g.redo = nil
	g.won = g.vals == g.sol
	return true
}

// apply sets the cells of a move to their new state (forward) or their old
// one. A cell that changes loses its wrong mark.
func (g *game) apply(cs []change, forward bool) {
	for _, c := range cs {
		v, n := c.newV, c.newN
		if !forward {
			v, n = c.oldV, c.oldN
		}
		g.vals[c.idx], g.notes[c.idx] = v, n
		g.marks[c.idx] = false
	}
}

// clearing returns the changes that take digit d out of the pencil marks of
// the peers of cell i.
func (g *game) clearing(i, d int) []change {
	var cs []change
	for _, p := range peers[i] {
		if g.vals[p] == 0 && has(g.notes[p], d) {
			cs = append(cs, change{idx: p, oldN: g.notes[p], newN: g.notes[p] &^ bit(d)})
		}
	}
	return cs
}

// fill is the move that puts d in cell i: the cell's own pencil marks go,
// and so does d from every peer's.
func (g *game) fill(i, d int) []change {
	cs := []change{{idx: i, oldV: g.vals[i], newV: byte(d), oldN: g.notes[i]}}
	return append(cs, g.clearing(i, d)...)
}

// put handles a digit key. In notes mode it toggles a pencil mark on an
// empty cell; otherwise it places the digit, removing it from the pencil
// marks of the cell's peers. A given cell, a finished game, and a digit that
// is already there do nothing. Placing a digit that differs from the
// solution counts one mistake.
func (g *game) put(d int) bool {
	i := g.cur
	if g.won || g.given(i) {
		return false
	}
	if g.pencil {
		if g.vals[i] != 0 {
			return false
		}
		return g.commit([]change{{idx: i, oldN: g.notes[i], newN: g.notes[i] ^ bit(d)}})
	}
	if int(g.vals[i]) == d {
		return false
	}
	if byte(d) != g.sol[i] {
		g.mistakes++
	}
	return g.commit(g.fill(i, d))
}

// erase clears the digit and the pencil marks of the cursor cell.
func (g *game) erase() bool {
	i := g.cur
	if g.won || g.given(i) || (g.vals[i] == 0 && g.notes[i] == 0) {
		return false
	}
	return g.commit([]change{{idx: i, oldV: g.vals[i], oldN: g.notes[i]}})
}

func (g *game) canUndo() bool { return !g.won && len(g.undo) > 0 }
func (g *game) canRedo() bool { return !g.won && len(g.redo) > 0 }

// undoMove takes back the last move, with the notes it cleared.
func (g *game) undoMove() bool {
	if !g.canUndo() {
		return false
	}
	cs := g.undo[len(g.undo)-1]
	g.undo = g.undo[:len(g.undo)-1]
	g.apply(cs, false)
	g.redo = append(g.redo, cs)
	g.cur = cs[0].idx
	return true
}

// redoMove does a move again. It does not count a mistake twice.
func (g *game) redoMove() bool {
	if !g.canRedo() {
		return false
	}
	cs := g.redo[len(g.redo)-1]
	g.redo = g.redo[:len(g.redo)-1]
	g.apply(cs, true)
	g.undo = append(g.undo, cs)
	g.cur = cs[0].idx
	g.won = g.vals == g.sol
	return true
}

// check marks every entry that differs from the solution and returns how
// many there are. Marks go away when the cell changes.
func (g *game) check() int {
	n := 0
	for i := range g.vals {
		g.marks[i] = g.vals[i] != 0 && !g.given(i) && g.vals[i] != g.sol[i]
		if g.marks[i] {
			n++
		}
	}
	return n
}

// hint fills one correct cell and counts it. The target is the cursor cell
// when it is not right yet; otherwise the first cell a naked or hidden single
// solves from the entries that are right; otherwise the first cell that is
// not right. The cursor goes there. The returned text says what was done.
func (g *game) hint() (string, bool) {
	if g.won {
		return "", false
	}
	right := func(i int) bool { return g.vals[i] == g.sol[i] }
	target, tech := -1, ""
	if !g.given(g.cur) && !right(g.cur) {
		target, tech = g.cur, "your cell"
	}
	if target < 0 {
		trusted := g.puz
		for i := range trusted {
			if right(i) {
				trusted[i] = g.vals[i]
			}
		}
		if st, ok := newLogic(trusted).single(); ok {
			target, tech = st.idx, st.tech
		}
	}
	if target < 0 {
		for i := range g.vals {
			if !right(i) {
				target, tech = i, "next empty cell"
				break
			}
		}
	}
	if target < 0 {
		return "", false
	}
	d := int(g.sol[target])
	g.hints++
	g.cur = target
	g.commit(g.fill(target, d))
	return fmt.Sprintf("hint: r%dc%d = %d, %s", rowOf[target]+1, colOf[target]+1, d, tech), true
}
