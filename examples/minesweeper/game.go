package main

import "math/rand"

// The rules of Minesweeper, with no UI in them. A game is a grid of cells
// numbered row by row (index = row*cols + col). The mines are laid at the
// first reveal, from the random source the caller hands in, so the first
// cell and its neighbors are never mines and the first reveal opens a
// region.

// cellState is what the player sees of a cell that is not open.
type cellState uint8

const (
	stHidden   cellState = iota // covered
	stOpen                      // revealed
	stFlag                      // covered, marked as a mine
	stQuestion                  // covered, marked with a question mark
)

// Bounds of a board. The document has one clickable cell for every
// position of the largest board, so these are also the limits of -cols and
// -rows. Customs are no smaller than the beginner board, which keeps the
// side panel (nine rows) no taller than any board.
const (
	minCols, maxCols = 9, 30
	minRows, maxRows = 9, 16
)

// A preset difficulty.
type preset struct {
	name              string
	cols, rows, mines int
}

var presets = []preset{
	{"beginner", 9, 9, 10},
	{"intermediate", 16, 16, 40},
	{"expert", 30, 16, 99},
}

// minesRange is the number of mines a cols x rows board may hold: at least
// one, and fewer than cells - 9, so the first cell and its neighbors can
// always be left clear.
func minesRange(cols, rows int) (lo, hi int) { return 1, cols*rows - 10 }

type game struct {
	cols, rows, mines int
	mine              []bool
	adj               []uint8 // mines around each cell
	st                []cellState
	placed            bool // the mines are laid (the first reveal happened)
	over, won         bool
	boom              int // the mine that was opened, or -1
	opened            int // open cells
	flags             int // cells marked with a flag (question marks do not count)
}

func newGame(cols, rows, mines int) *game {
	n := cols * rows
	return &game{
		cols: cols, rows: rows, mines: mines,
		mine: make([]bool, n), adj: make([]uint8, n), st: make([]cellState, n),
		boom: -1,
	}
}

func (g *game) cells() int { return g.cols * g.rows }

// idx and pos convert between a cell number and its row and column.
func (g *game) idx(r, c int) int     { return r*g.cols + c }
func (g *game) pos(i int) (r, c int) { return i / g.cols, i % g.cols }

// neighbors lists the cells around i, in reading order.
func (g *game) neighbors(i int) []int {
	r, c := g.pos(i)
	out := make([]int, 0, 8)
	for dr := -1; dr <= 1; dr++ {
		for dc := -1; dc <= 1; dc++ {
			if dr == 0 && dc == 0 {
				continue
			}
			rr, cc := r+dr, c+dc
			if rr < 0 || rr >= g.rows || cc < 0 || cc >= g.cols {
				continue
			}
			out = append(out, g.idx(rr, cc))
		}
	}
	return out
}

// place lays the mines, none on first or around it, choosing among the
// other cells with rng (a partial shuffle, so the draw count depends only
// on the board).
func (g *game) place(first int, rng *rand.Rand) {
	safe := map[int]bool{first: true}
	for _, n := range g.neighbors(first) {
		safe[n] = true
	}
	var cand []int
	for i := 0; i < g.cells(); i++ {
		if !safe[i] {
			cand = append(cand, i)
		}
	}
	for k := 0; k < g.mines; k++ {
		j := k + rng.Intn(len(cand)-k)
		cand[k], cand[j] = cand[j], cand[k]
		g.mine[cand[k]] = true
	}
	for i := range g.adj {
		for _, n := range g.neighbors(i) {
			if g.mine[n] {
				g.adj[i]++
			}
		}
	}
	g.placed = true
}

// reveal opens a covered cell (flagged cells are left alone). The first
// reveal lays the mines. It reports whether anything changed.
func (g *game) reveal(i int, rng *rand.Rand) bool {
	if g.over || g.st[i] == stOpen || g.st[i] == stFlag {
		return false
	}
	if !g.placed {
		g.place(i, rng)
	}
	g.open(i)
	g.settle()
	return true
}

// open opens i and, for a zero, floods outward over the covered cells that
// are not flagged. It stops the game on a mine.
func (g *game) open(i int) {
	if g.st[i] == stOpen || g.st[i] == stFlag {
		return
	}
	if g.mine[i] {
		g.boom = i
		g.over = true
		return
	}
	stack := []int{i}
	g.st[i] = stOpen
	g.opened++
	for len(stack) > 0 {
		j := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if g.adj[j] != 0 {
			continue
		}
		for _, n := range g.neighbors(j) {
			if g.st[n] == stHidden || g.st[n] == stQuestion {
				g.st[n] = stOpen
				g.opened++
				stack = append(stack, n)
			}
		}
	}
}

// settle ends the game when every safe cell is open: the mines are then
// flagged, as the classic game does.
func (g *game) settle() {
	if g.over || g.opened != g.cells()-g.mines {
		return
	}
	g.over, g.won = true, true
	for i, m := range g.mine {
		if m {
			g.st[i] = stFlag
		}
	}
	g.flags = g.mines
}

// flagsAround counts the flags next to i.
func (g *game) flagsAround(i int) int {
	n := 0
	for _, j := range g.neighbors(i) {
		if g.st[j] == stFlag {
			n++
		}
	}
	return n
}

// canChord reports whether chording i would open anything: it is an open
// number whose adjacent flags match it and that has covered neighbors left.
func (g *game) canChord(i int) bool {
	if g.over || g.st[i] != stOpen || g.adj[i] == 0 || g.flagsAround(i) != int(g.adj[i]) {
		return false
	}
	for _, j := range g.neighbors(i) {
		if g.st[j] == stHidden || g.st[j] == stQuestion {
			return true
		}
	}
	return false
}

// chord opens the covered neighbors of an open number whose flags match
// it. A wrong flag makes one of them a mine: the game is lost.
func (g *game) chord(i int) bool {
	if !g.canChord(i) {
		return false
	}
	for _, j := range g.neighbors(i) {
		if g.st[j] == stHidden || g.st[j] == stQuestion {
			g.open(j)
			if g.over {
				return true
			}
		}
	}
	g.settle()
	return true
}

// cycle steps a covered cell through no mark, flag, question mark.
func (g *game) cycle(i int) bool {
	if g.over {
		return false
	}
	switch g.st[i] {
	case stHidden:
		g.st[i] = stFlag
		g.flags++
	case stFlag:
		g.st[i] = stQuestion
		g.flags--
	case stQuestion:
		g.st[i] = stHidden
	default:
		return false
	}
	return true
}

// minesLeft is the counter on the panel: mines minus flags. It goes
// negative when more cells are flagged than there are mines.
func (g *game) minesLeft() int { return g.mines - g.flags }
