package main

import (
	"strings"
	"testing"
)

// wikiGame is the classic puzzle, whose solution is known.
func wikiGame(t testing.TB) *game {
	t.Helper()
	p, err := fromGivens(wikiPuzzle)
	if err != nil {
		t.Fatal(err)
	}
	return newGame(p)
}

// at moves the cursor to row r, column c (from 0).
func (g *game) at(r, c int) *game { g.cur = r*9 + c; return g }

// right and wrong are the solution digit of the cursor cell and another.
func (g *game) right() int { return int(g.sol[g.cur]) }
func (g *game) wrong() int { return int(g.sol[g.cur])%9 + 1 }

func TestNewGameStartsOnTheFirstEmptyCell(t *testing.T) {
	g := wikiGame(t)
	if g.cur != 2 { // "53.." : the third cell is the first blank
		t.Errorf("cursor at %d, want 2", g.cur)
	}
	if g.filled() != 30 || g.won || g.mistakes != 0 || g.hints != 0 {
		t.Errorf("filled %d won %v mistakes %d hints %d", g.filled(), g.won, g.mistakes, g.hints)
	}
}

func TestMoveCursorStopsAtTheEdges(t *testing.T) {
	g := wikiGame(t).at(0, 0)
	g.moveCursor(-1, 0)
	g.moveCursor(0, -1)
	if g.cur != 0 {
		t.Errorf("moved off the top left: %d", g.cur)
	}
	g.at(8, 8).moveCursor(1, 0)
	g.moveCursor(0, 1)
	if g.cur != 80 {
		t.Errorf("moved off the bottom right: %d", g.cur)
	}
	g.at(4, 4).moveCursor(-1, 1)
	if g.cur != 3*9+5 {
		t.Errorf("diagonal move landed on %d", g.cur)
	}
}

func TestPutPlacesADigitAndCountsAMistake(t *testing.T) {
	g := wikiGame(t).at(0, 2)
	if !g.put(g.right()) || g.vals[2] != 4 || g.mistakes != 0 {
		t.Fatalf("right digit: vals %d mistakes %d", g.vals[2], g.mistakes)
	}
	g.at(0, 3)
	w := g.wrong()
	if !g.put(w) || int(g.vals[3]) != w || g.mistakes != 1 {
		t.Fatalf("wrong digit: vals %d mistakes %d", g.vals[3], g.mistakes)
	}
	// The same digit again does nothing; replacing it with the other wrong
	// digit counts again.
	if g.put(w) {
		t.Error("the same digit again was a move")
	}
	if g.mistakes != 1 {
		t.Errorf("mistakes = %d after a no-op", g.mistakes)
	}
	other := w%9 + 1
	if other == g.right() {
		other = other%9 + 1
	}
	if !g.put(other) || g.mistakes != 2 {
		t.Errorf("a second wrong digit: mistakes = %d, want 2", g.mistakes)
	}
	g.mistakes = 1
	if !g.put(g.right()) || g.mistakes != 1 {
		t.Errorf("fixing the cell: mistakes %d", g.mistakes)
	}
}

func TestGivensAreFixed(t *testing.T) {
	g := wikiGame(t).at(0, 0)
	if g.put(1) || g.erase() || g.vals[0] != 5 {
		t.Error("a given changed")
	}
	g.pencil = true
	if g.put(1) || g.notes[0] != 0 {
		t.Error("a given took a note")
	}
}

func TestPlacingClearsTheNotesOfPeers(t *testing.T) {
	g := wikiGame(t).at(0, 2)
	g.pencil = true
	// Notes of 4 in a peer in the same row, the same column, the same box,
	// and one that is no peer.
	for _, rc := range [][2]int{{0, 3}, {1, 2}, {1, 1}, {3, 3}} {
		g.at(rc[0], rc[1])
		g.put(4)
	}
	g.at(0, 2)
	g.put(1) // a note of its own, which the digit also clears
	g.pencil = false
	g.put(4)
	for _, rc := range [][2]int{{0, 3}, {1, 2}, {1, 1}} {
		if n := g.notes[rc[0]*9+rc[1]]; has(n, 4) {
			t.Errorf("peer r%dc%d kept its note of 4", rc[0]+1, rc[1]+1)
		}
	}
	if !has(g.notes[3*9+3], 4) {
		t.Error("a cell that is no peer lost its note")
	}
	if g.notes[2] != 0 {
		t.Errorf("the cell kept its own notes: %b", g.notes[2])
	}
}

func TestNotesModeTogglesPencilMarks(t *testing.T) {
	g := wikiGame(t).at(0, 2)
	g.pencil = true
	g.put(3)
	g.put(7)
	if g.notes[2] != set(3, 7) || g.vals[2] != 0 {
		t.Fatalf("notes %b vals %d", g.notes[2], g.vals[2])
	}
	g.put(3)
	if g.notes[2] != set(7) {
		t.Errorf("a second press did not clear the note: %b", g.notes[2])
	}
	if g.mistakes != 0 {
		t.Error("a note counted as a mistake")
	}
	// A filled cell takes no notes.
	g.pencil = false
	g.put(g.right())
	g.pencil = true
	if g.put(5) || has(g.notes[2], 5) {
		t.Error("a filled cell took a note")
	}
}

func TestEraseClearsDigitAndNotes(t *testing.T) {
	g := wikiGame(t).at(0, 2)
	if g.erase() {
		t.Error("erasing an empty cell was a move")
	}
	g.pencil = true
	g.put(1)
	if !g.erase() || g.notes[2] != 0 {
		t.Error("erase kept the notes")
	}
	g.pencil = false
	g.put(g.wrong())
	if !g.erase() || g.vals[2] != 0 {
		t.Error("erase kept the digit")
	}
}

// One undo takes back a placement together with the notes it removed.
func TestUndoRestoresTheNotesAPlacementCleared(t *testing.T) {
	g := wikiGame(t).at(0, 3)
	g.pencil = true
	g.put(4)
	g.at(0, 2)
	g.pencil = false
	before := g.notes
	g.put(4)
	if has(g.notes[3], 4) {
		t.Fatal("the placement kept the peer's note")
	}
	if len(g.undo) != 2 {
		t.Fatalf("%d moves, want 2 (the note and the digit)", len(g.undo))
	}
	if !g.undoMove() {
		t.Fatal("nothing to undo")
	}
	if g.vals[2] != 0 || g.notes != before {
		t.Errorf("undo left vals %d, notes changed: %v", g.vals[2], g.notes != before)
	}
	if g.cur != 2 {
		t.Errorf("undo left the cursor on %d", g.cur)
	}
	if !g.redoMove() || g.vals[2] != 4 || has(g.notes[3], 4) {
		t.Error("redo did not do the placement and the clearing again")
	}
}

func TestUndoRedoStacks(t *testing.T) {
	g := wikiGame(t)
	if g.undoMove() || g.redoMove() {
		t.Error("undo or redo on a new game did something")
	}
	g.at(0, 2).put(g.right())
	g.at(0, 3).put(g.wrong())
	if g.mistakes != 1 {
		t.Fatal("setup")
	}
	g.undoMove()
	g.undoMove()
	if g.filled() != 30 || len(g.redo) != 2 {
		t.Fatalf("filled %d redo %d", g.filled(), len(g.redo))
	}
	if g.mistakes != 1 {
		t.Error("undo took a mistake back")
	}
	g.redoMove()
	g.redoMove()
	if g.filled() != 32 || g.mistakes != 1 {
		t.Errorf("filled %d mistakes %d after redo", g.filled(), g.mistakes)
	}
	// A new move empties the redo stack.
	g.undoMove()
	g.at(0, 3).put(g.right())
	if g.canRedo() {
		t.Error("redo survived a new move")
	}
}

func TestCheckMarksWrongEntriesUntilTheyChange(t *testing.T) {
	g := wikiGame(t)
	g.at(0, 2).put(g.right())
	g.at(0, 3).put(g.wrong())
	g.at(0, 5).put(g.wrong())
	if n := g.check(); n != 2 {
		t.Fatalf("check found %d wrong entries, want 2", n)
	}
	if !g.marks[3] || !g.marks[5] || g.marks[2] || g.marks[0] {
		t.Errorf("marks: %v", g.marks[:8])
	}
	g.at(0, 3).put(g.right())
	if g.marks[3] || !g.marks[5] {
		t.Error("the fixed cell kept its mark, or the other lost it")
	}
	if n := g.check(); n != 1 {
		t.Errorf("second check found %d, want 1", n)
	}
	g.at(0, 5).erase()
	if g.marks[5] {
		t.Error("an erased cell kept its mark")
	}
	if n := g.check(); n != 0 {
		t.Errorf("third check found %d, want 0", n)
	}
}

func TestHintFillsTheCursorCellFirst(t *testing.T) {
	g := wikiGame(t).at(4, 4)
	msg, ok := g.hint()
	if !ok || g.vals[40] != g.sol[40] || g.hints != 1 || g.mistakes != 0 {
		t.Fatalf("hint: %q %v, vals %d hints %d", msg, ok, g.vals[40], g.hints)
	}
	if !strings.Contains(msg, "r5c5") || !strings.Contains(msg, "your cell") {
		t.Errorf("message %q", msg)
	}
	// Over a wrong entry it replaces it.
	g.at(0, 2).put(g.wrong())
	if _, ok := g.hint(); !ok || g.vals[2] != g.sol[2] || g.hints != 2 {
		t.Errorf("hint over a wrong entry: vals %d hints %d", g.vals[2], g.hints)
	}
}

// With the cursor on a right cell, the hint goes where a naked or hidden
// single is, says which, and moves the cursor there.
func TestHintFindsASingleElsewhere(t *testing.T) {
	g := wikiGame(t).at(0, 0)
	msg, ok := g.hint()
	if !ok {
		t.Fatal("no hint")
	}
	if g.cur == 0 {
		t.Error("the cursor stayed on a given")
	}
	if !strings.Contains(msg, "single") {
		t.Errorf("message %q does not name the technique", msg)
	}
	if g.vals[g.cur] != g.sol[g.cur] || g.puz[g.cur] != 0 {
		t.Error("the hint did not fill an empty cell correctly")
	}
	// A hint can be undone; the count stays.
	if !g.undoMove() || g.vals[g.cur] != 0 || g.hints != 1 {
		t.Errorf("undo of a hint: vals %d hints %d", g.vals[g.cur], g.hints)
	}
}

// A wrong entry elsewhere does not mislead the hint: it plans from the
// entries that are right.
func TestHintIgnoresWrongEntries(t *testing.T) {
	// Where the hint goes from the puzzle alone, with no entries at all.
	clean := wikiGame(t).at(0, 0)
	cleanMsg, ok := clean.hint()
	if !ok {
		t.Fatal("no hint on a clean game")
	}
	want := clean.cur
	// Find a wrong entry that would mislead a planner trusting every entry:
	// with it on the board, the first single is somewhere else.
	found := 0
	for c := 0; c < size; c++ {
		if wikiGame(t).puz[c] != 0 {
			continue
		}
		g := wikiGame(t).at(c/9, c%9)
		g.put(g.wrong())
		if st, ok := newLogic(g.vals).single(); !ok || st.idx == want {
			continue // trusting the wrong entry would not change the answer
		}
		found++
		g.at(0, 0) // a given: the hint must plan
		msg, ok := g.hint()
		if !ok || g.cur != want {
			t.Fatalf("wrong entry at r%dc%d: the hint went to %d (%q), want %d (%q)", c/9+1, c%9+1, g.cur, msg, want, cleanMsg)
		}
		if g.vals[want] != g.sol[want] {
			t.Errorf("hint filled %d, solution %d", g.vals[want], g.sol[want])
		}
	}
	if found == 0 {
		t.Fatal("no wrong entry misleads a planner that trusts every entry: the test proves nothing")
	}
}

func TestWinningStopsInput(t *testing.T) {
	g := wikiGame(t)
	for !g.won {
		if _, ok := g.hint(); !ok {
			t.Fatal("hints ran out before the board was full")
		}
	}
	if g.vals != g.sol || g.hints == 0 {
		t.Error("won, but the board is not the solution")
	}
	g.at(0, 2)
	if g.put(1) || g.erase() || g.undoMove() || g.redoMove() {
		t.Error("input accepted after the win")
	}
	if _, ok := g.hint(); ok {
		t.Error("a hint after the win")
	}
}

// A full board that is not the solution is not a win.
func TestFullButWrongIsNotAWin(t *testing.T) {
	g := wikiGame(t)
	for i := range g.vals {
		if g.vals[i] == 0 {
			g.at(i/9, i%9).put(g.right())
		}
	}
	if !g.won {
		t.Fatal("setup: the right digits everywhere should win")
	}
	g2 := wikiGame(t)
	for i := range g2.vals {
		if g2.vals[i] == 0 {
			g2.at(i/9, i%9).put(g2.wrong())
		}
	}
	if g2.won || g2.filled() != 81 {
		t.Errorf("won %v filled %d", g2.won, g2.filled())
	}
}
