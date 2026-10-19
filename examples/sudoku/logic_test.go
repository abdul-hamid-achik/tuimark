package main

import (
	"testing"
)

// cands builds a solver whose only candidates are the ones given: cell
// index to digit set. Every other cell is empty with no candidates, which
// the techniques skip.
func cands(m map[int][]int) *logicSolver {
	l := &logicSolver{}
	for i, ds := range m {
		for _, d := range ds {
			l.cand[i] |= bit(d)
		}
	}
	return l
}

func set(ds ...int) uint16 {
	var m uint16
	for _, d := range ds {
		m |= bit(d)
	}
	return m
}

func TestNakedSingle(t *testing.T) {
	l := newLogic(mustGrid(t, wikiPuzzle))
	st, ok := l.nakedSingle()
	if !ok || !st.place || st.tech != "naked single" || st.level != levelEasy {
		t.Fatalf("step = %+v, %v", st, ok)
	}
	if l.g[st.idx] != byte(st.digit) || st.digit != int(mustGrid(t, wikiSolution)[st.idx]) {
		t.Errorf("placed %d at %d, which is not the solution", st.digit, st.idx)
	}
	// The digit left the candidates of the cell's peers.
	for _, p := range peers[st.idx] {
		if has(l.cand[p], st.digit) {
			t.Errorf("peer %d still has %d", p, st.digit)
		}
	}
}

func TestHiddenSingle(t *testing.T) {
	// In row 0 only cell 4 can take a 7.
	l := cands(map[int][]int{0: {1, 2}, 1: {1, 2, 3}, 2: {2, 3}, 4: {2, 7}, 5: {1, 3}})
	st, ok := l.hiddenSingle()
	if !ok || st.idx != 4 || st.digit != 7 || st.tech != "hidden single" {
		t.Fatalf("step = %+v, %v", st, ok)
	}
}

func TestLockedCandidatesPointing(t *testing.T) {
	// In box 0 a 5 fits only in cells 0 and 1, both in row 0: row 0's other
	// cells (3 and 8, outside the box) lose their 5.
	l := cands(map[int][]int{0: {5, 6}, 1: {5, 7}, 3: {5, 8}, 8: {5, 9}, 13: {5, 1}})
	if !l.lockedCandidates() {
		t.Fatal("no progress")
	}
	if has(l.cand[3], 5) || has(l.cand[8], 5) {
		t.Error("the 5 is still a candidate in row 0 outside the box")
	}
	if !has(l.cand[0], 5) || !has(l.cand[1], 5) {
		t.Error("the 5 left the box's own cells")
	}
	if !has(l.cand[13], 5) {
		t.Error("the 5 left a row that is not the locked one")
	}
}

func TestLockedCandidatesClaiming(t *testing.T) {
	// In row 0 a 5 fits only in cells 0 and 1, both in box 0: the rest of
	// box 0 (cells 9 and 20) loses its 5.
	l := cands(map[int][]int{0: {5, 6}, 1: {5, 7}, 9: {5, 8}, 20: {5, 9}, 30: {5, 1}})
	if !l.lockedCandidates() {
		t.Fatal("no progress")
	}
	if has(l.cand[9], 5) || has(l.cand[20], 5) {
		t.Error("the 5 is still a candidate in the box outside the row")
	}
	if !has(l.cand[30], 5) {
		t.Error("the 5 left a cell outside the box")
	}
}

func TestNakedPair(t *testing.T) {
	l := cands(map[int][]int{0: {1, 2}, 1: {1, 2}, 2: {1, 2, 3}, 5: {2, 4}, 12: {1, 4}})
	if !l.nakedPair() {
		t.Fatal("no progress")
	}
	if l.cand[2] != set(3) || l.cand[5] != set(4) {
		t.Errorf("row mates: %b %b, want 3 and 4 left", l.cand[2], l.cand[5])
	}
	if l.cand[0] != set(1, 2) || l.cand[1] != set(1, 2) {
		t.Error("the pair itself changed")
	}
	// Cell 12 is in row 1 and box 1: it shares no unit with the pair.
	if l.cand[12] != set(1, 4) {
		t.Errorf("a cell outside the pair's units changed: %b", l.cand[12])
	}
	if l.nakedPair() {
		t.Error("the technique reports progress twice for one elimination")
	}
}

func TestHiddenPair(t *testing.T) {
	// In row 0 the digits 1 and 2 fit only in cells 0 and 1.
	l := cands(map[int][]int{0: {1, 2, 3, 4}, 1: {1, 2, 5}, 2: {3, 4, 5, 6}, 3: {3, 4, 5, 6}, 4: {3, 4, 6}})
	if !l.hiddenPair() {
		t.Fatal("no progress")
	}
	if l.cand[0] != set(1, 2) || l.cand[1] != set(1, 2) {
		t.Errorf("pair cells: %b %b, want only 1 and 2", l.cand[0], l.cand[1])
	}
	if l.cand[2] != set(3, 4, 5, 6) {
		t.Error("another cell changed")
	}
	if l.hiddenPair() {
		t.Error("the technique reports progress twice for one elimination")
	}
}

func TestXWingRows(t *testing.T) {
	// A 7 fits only in columns 2 and 6 of rows 1 and 5. It cannot go
	// anywhere else in those two columns.
	l := cands(map[int][]int{
		1*9 + 2: {7, 1}, 1*9 + 6: {7, 2}, 5*9 + 2: {7, 3}, 5*9 + 6: {7, 4},
		3*9 + 2: {7, 8}, 8*9 + 6: {7, 9}, 3*9 + 4: {7, 5},
	})
	if !l.xwing() {
		t.Fatal("no progress")
	}
	if has(l.cand[3*9+2], 7) || has(l.cand[8*9+6], 7) {
		t.Error("the 7 is still a candidate in the wing's columns")
	}
	if !has(l.cand[3*9+4], 7) {
		t.Error("the 7 left a column that is not a wing's")
	}
	if !has(l.cand[1*9+2], 7) || !has(l.cand[5*9+6], 7) {
		t.Error("the wing's corners lost their 7")
	}
}

func TestXWingColumns(t *testing.T) {
	// The same shape on its side: a 7 fits only in rows 2 and 6 of columns
	// 1 and 5, so it cannot go elsewhere in rows 2 and 6.
	l := cands(map[int][]int{
		2*9 + 1: {7, 1}, 6*9 + 1: {7, 2}, 2*9 + 5: {7, 3}, 6*9 + 5: {7, 4},
		2*9 + 3: {7, 8}, 6*9 + 8: {7, 9}, 4*9 + 3: {7, 5},
	})
	if !l.xwing() {
		t.Fatal("no progress")
	}
	if has(l.cand[2*9+3], 7) || has(l.cand[6*9+8], 7) {
		t.Error("the 7 is still a candidate in the wing's rows")
	}
	if !has(l.cand[4*9+3], 7) {
		t.Error("the 7 left a row that is not a wing's")
	}
}

// The grade of a puzzle is the hardest technique its solution path needs,
// and a generated puzzle of each level really needs it.
func TestGradeFollowsTheTechniquesUsed(t *testing.T) {
	if got := grade(mustGrid(t, wikiPuzzle)); got != levelEasy {
		t.Errorf("the classic puzzle grades %d, want easy", got)
	}
	for level := levelEasy; level <= levelHard; level++ {
		p := generateCached(1, level)
		l := newLogic(p.puz)
		used := map[string]bool{}
		hardest := 0
		for !l.solved() {
			st, ok := l.next()
			if !ok {
				t.Fatalf("%s: the solver is stuck", levelName(level))
			}
			used[st.tech] = true
			if st.level > hardest {
				hardest = st.level
			}
		}
		if hardest != level {
			t.Errorf("%s: hardest technique level %d (%v)", levelName(level), hardest, used)
		}
		if l.g != p.sol {
			t.Errorf("%s: the logic solver's answer is not the solution", levelName(level))
		}
	}
	// An expert puzzle stops the solver before the end.
	p := generateCached(1, levelExpert)
	l := newLogic(p.puz)
	for {
		if _, ok := l.next(); !ok {
			break
		}
	}
	if l.solved() {
		t.Error("the expert puzzle was solved by logic alone")
	}
}

// single finds a placement without touching other candidates, and every
// placement it suggests is the solution's.
func TestSingleIsAlwaysRight(t *testing.T) {
	p := generateCached(2, levelMedium)
	l := newLogic(p.puz)
	for i := 0; i < 10; i++ {
		st, ok := l.single()
		if !ok {
			break
		}
		if p.sol[st.idx] != byte(st.digit) {
			t.Fatalf("single put %d at %d, the solution has %d", st.digit, st.idx, p.sol[st.idx])
		}
	}
}
