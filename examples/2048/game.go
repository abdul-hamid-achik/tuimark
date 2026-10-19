package main

import (
	"math/rand"
)

// The rules of 2048, as pure state: a square board of values (0 is empty),
// four slides, one spawn per move that changed something, a single level of
// undo. Nothing here reads a clock or the terminal; the only randomness is
// the seeded generator the game owns.

const (
	minSize     = 3
	maxSize     = 6
	defaultSize = 4
	winValue    = 2048
	histLen     = 22 // samples of the score sparkline: its width in cells
	defaultP4   = 0.1
)

// dir is a slide direction.
type dir int

const (
	dirUp dir = iota
	dirRight
	dirDown
	dirLeft
)

var dirNames = [...]string{"up", "right", "down", "left"}

func (d dir) String() string { return dirNames[d] }

// snapshot is what one level of undo restores. The generator is not part of
// it: a replayed move spawns from where the generator stands, not from
// where it stood.
type snapshot struct {
	cells   []int
	score   int
	moves   int
	gain    int
	histLen int
}

// game is one board and its running totals.
type game struct {
	n          int
	cells      []int  // row-major values, 0 for an empty cell
	pop        []bool // cells a merge landed on in the last move
	fresh      []bool // the cell the last move spawned a tile on
	score      int
	best       int
	moves      int
	gain       int // score of the last move
	won        bool
	over       bool
	keep       bool // the player chose to keep going past 2048
	hist       []int
	prev       *snapshot
	rng        *rand.Rand
	p4         float64 // chance that a spawned tile is a 4
	reachedWin bool    // the last move made the first 2048
}

func newGame(n int, seed int64, p4 float64) *game {
	g := &game{n: n, rng: rand.New(rand.NewSource(seed)), p4: p4}
	g.reset()
	return g
}

// reset starts a new board, keeping the best score and the generator.
func (g *game) reset() {
	g.cells = make([]int, g.n*g.n)
	g.clearFlash()
	g.score, g.moves, g.gain = 0, 0, 0
	g.won, g.over, g.keep, g.reachedWin = false, false, false, false
	g.prev = nil
	g.hist = []int{0}
	g.spawn()
	g.spawn()
	g.clearFlash() // the opening tiles are not news
}

func (g *game) clearFlash() {
	g.pop = make([]bool, g.n*g.n)
	g.fresh = make([]bool, g.n*g.n)
}

func (g *game) hasFlash() bool {
	for i := range g.pop {
		if g.pop[i] || g.fresh[i] {
			return true
		}
	}
	return false
}

// spawn puts a 2 (a 4 with chance p4) on a random empty cell. It reports
// whether there was room.
func (g *game) spawn() bool {
	var empty []int
	for i, v := range g.cells {
		if v == 0 {
			empty = append(empty, i)
		}
	}
	if len(empty) == 0 {
		return false
	}
	at := empty[g.rng.Intn(len(empty))]
	v := 2
	if g.rng.Float64() < g.p4 {
		v = 4
	}
	g.cells[at] = v
	g.fresh[at] = true
	return true
}

// line returns the cell indices of line i read in the direction of the
// slide: index 0 is the cell next to the wall the tiles slide towards.
func (g *game) line(d dir, i int) []int {
	n := g.n
	idx := make([]int, n)
	for k := 0; k < n; k++ {
		switch d {
		case dirLeft:
			idx[k] = i*n + k
		case dirRight:
			idx[k] = i*n + (n - 1 - k)
		case dirUp:
			idx[k] = k*n + i
		default: // down
			idx[k] = (n-1-k)*n + i
		}
	}
	return idx
}

// slideLine packs the non-zero values of in towards index 0 and merges equal
// neighbours once each, left to right. merged marks the slots a merge landed
// on; gain is the sum of the merged values.
func slideLine(in []int) (out []int, merged []bool, gain int) {
	out = make([]int, len(in))
	merged = make([]bool, len(in))
	w := 0
	last := -1 // slot of the last tile written that may still merge
	for _, v := range in {
		if v == 0 {
			continue
		}
		if last >= 0 && out[last] == v {
			out[last] = v * 2
			gain += v * 2
			merged[last] = true
			last = -1
			continue
		}
		out[w] = v
		last = w
		w++
	}
	return out, merged, gain
}

// slid is the board after sliding d, without touching g.
func (g *game) slid(d dir) (cells []int, pop []bool, gain int, changed bool) {
	cells = make([]int, len(g.cells))
	pop = make([]bool, len(g.cells))
	for i := 0; i < g.n; i++ {
		idx := g.line(d, i)
		in := make([]int, g.n)
		for k, at := range idx {
			in[k] = g.cells[at]
		}
		out, merged, gn := slideLine(in)
		gain += gn
		for k, at := range idx {
			cells[at] = out[k]
			pop[at] = merged[k]
		}
	}
	for i := range cells {
		if cells[i] != g.cells[i] {
			changed = true
			break
		}
	}
	return cells, pop, gain, changed
}

// move slides in direction d. A move that changes nothing returns false and
// leaves the game untouched: no spawn, no count, no undo step.
func (g *game) move(d dir) bool {
	cells, pop, gain, changed := g.slid(d)
	if !changed {
		return false
	}
	g.prev = &snapshot{
		cells:   append([]int(nil), g.cells...),
		score:   g.score,
		moves:   g.moves,
		gain:    g.gain,
		histLen: len(g.hist),
	}
	g.cells = cells
	g.pop = pop
	g.fresh = make([]bool, len(cells))
	g.score += gain
	g.gain = gain
	g.moves++
	if g.score > g.best {
		g.best = g.score
	}
	g.reachedWin = false
	if !g.won {
		for i, v := range g.cells {
			if v >= winValue && pop[i] {
				g.won, g.reachedWin = true, true
				break
			}
		}
	}
	g.spawn()
	g.hist = append(g.hist, g.score)
	g.over = !g.canMove()
	return true
}

// canMove reports whether any slide would change the board.
func (g *game) canMove() bool {
	n := g.n
	for i, v := range g.cells {
		if v == 0 {
			return true
		}
		if i%n+1 < n && g.cells[i+1] == v {
			return true
		}
		if i+n < len(g.cells) && g.cells[i+n] == v {
			return true
		}
	}
	return false
}

// canUndo reports whether there is a move to take back.
func (g *game) canUndo() bool { return g.prev != nil }

// undo takes back the last move. The best score stays.
func (g *game) undo() bool {
	if g.prev == nil {
		return false
	}
	p := g.prev
	g.cells = append([]int(nil), p.cells...)
	// won and keep stay: the win is once per game, whatever is undone
	g.score, g.moves, g.gain = p.score, p.moves, p.gain
	g.hist = g.hist[:p.histLen]
	g.over, g.reachedWin = false, false
	g.clearFlash()
	g.prev = nil
	return true
}

// largest is the biggest tile on the board.
func (g *game) largest() int {
	m := 0
	for _, v := range g.cells {
		if v > m {
			m = v
		}
	}
	return m
}

func (g *game) empties() int {
	c := 0
	for _, v := range g.cells {
		if v == 0 {
			c++
		}
	}
	return c
}

// sampleHist is the score history squeezed into width samples for the
// sparkline: right-aligned when the game is shorter than the width, evenly
// resampled (integer arithmetic only) when it is longer, always ending at
// the current score. Missing samples are nil.
func sampleHist(hist []int, width int) []any {
	out := make([]any, width)
	l := len(hist)
	if l <= width {
		for i, v := range hist {
			out[width-l+i] = v
		}
		return out
	}
	for i := 0; i < width; i++ {
		out[i] = hist[i*(l-1)/(width-1)]
	}
	return out
}
