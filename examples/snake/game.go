package main

import "math/rand"

// The rules of the game, with no Tuimark in sight: a grid, a snake, a food
// cell, and a seeded RNG. Everything here is deterministic given the seed
// and the sequence of calls, so tests and the -demo script replay it
// exactly.

const (
	boardW = 24 // cells across
	boardH = 16 // cells down

	foodPoints = 10 // score per food
	maxLevel   = 11 // level 1 is the slowest, maxLevel the fastest
	baseMs     = 140
	stepMs     = 7 // faster by this much per food: 140ms at level 1, 70ms at 11
	maxTurns   = 2 // turns queued between two moves
)

type point struct{ x, y int }

func (p point) add(d point) point { return point{p.x + d.x, p.y + d.y} }

// neg is the opposite direction.
func (p point) neg() point { return point{-p.x, -p.y} }

var (
	dirUp    = point{0, -1}
	dirRight = point{1, 0}
	dirDown  = point{0, 1}
	dirLeft  = point{-1, 0}
	// dirs is the fixed order every tie-break uses.
	dirs = [4]point{dirUp, dirRight, dirDown, dirLeft}
)

type game struct {
	w, h  int
	rng   *rand.Rand
	snake []point // head first
	dir   point   // the direction of the last move
	turns []point // queued turns, at most maxTurns
	food  point
	score int
	foods int // food eaten this game: the level follows
	best  int // the best score since the program started
	over  bool
	won   bool // the snake fills the board
}

func newGame(seed int64) *game {
	g := &game{w: boardW, h: boardH, rng: rand.New(rand.NewSource(seed))}
	g.restart()
	return g
}

// restart starts a new game; the RNG goes on from where it was and the best
// score is kept.
func (g *game) restart() {
	cx, cy := g.w/2, g.h/2
	g.snake = []point{{cx, cy}, {cx - 1, cy}, {cx - 2, cy}}
	g.dir = dirRight
	g.turns = nil
	g.score, g.foods = 0, 0
	g.over, g.won = false, false
	g.placeFood()
}

// level is 1 at the start and grows by one per food, up to maxLevel.
func (g *game) level() int {
	if l := g.foods + 1; l < maxLevel {
		return l
	}
	return maxLevel
}

// intervalMs is the time between two moves at the current level.
func (g *game) intervalMs() int { return baseMs - stepMs*(g.level()-1) }

func (g *game) inside(p point) bool { return p.x >= 0 && p.x < g.w && p.y >= 0 && p.y < g.h }

// occupied marks the snake's cells.
func (g *game) occupied() []bool {
	occ := make([]bool, g.w*g.h)
	for _, p := range g.snake {
		occ[p.y*g.w+p.x] = true
	}
	return occ
}

// placeFood puts the food on a free cell chosen by the RNG, taking the
// free cells in row-major order so a seed names one sequence of foods. A
// board with no free cell is a win.
func (g *game) placeFood() {
	occ := g.occupied()
	free := make([]point, 0, len(occ))
	for y := 0; y < g.h; y++ {
		for x := 0; x < g.w; x++ {
			if !occ[y*g.w+x] {
				free = append(free, point{x, y})
			}
		}
	}
	if len(free) == 0 {
		g.food = point{-1, -1}
		g.over, g.won = true, true
		return
	}
	g.food = free[g.rng.Intn(len(free))]
}

// steer queues a turn. A turn that repeats the heading, or reverses it into
// the body, is ignored, judged against the last queued turn so that two
// quick taps (up, then left) both count. It reports whether the turn was
// queued.
func (g *game) steer(d point) bool {
	if g.over {
		return false
	}
	last := g.dir
	if n := len(g.turns); n > 0 {
		last = g.turns[n-1]
	}
	if d == last || d == last.neg() || len(g.turns) >= maxTurns {
		return false
	}
	g.turns = append(g.turns, d)
	return true
}

// move advances the snake one cell, taking the oldest queued turn first,
// and reports whether it ate. Hitting a wall or the body ends the game; the
// snake then stays where it is.
func (g *game) move() (ate bool) {
	if g.over {
		return false
	}
	if len(g.turns) > 0 {
		g.dir, g.turns = g.turns[0], g.turns[1:]
	}
	head := g.snake[0].add(g.dir)
	ate = head == g.food
	if !g.inside(head) {
		g.over = true
		return false
	}
	// The tail leaves its cell on this move unless the snake grows.
	body := g.snake
	if !ate {
		body = g.snake[:len(g.snake)-1]
	}
	for _, p := range body {
		if p == head {
			g.over = true
			return false
		}
	}
	next := make([]point, 0, len(g.snake)+1)
	next = append(next, head)
	next = append(next, body...)
	g.snake = next
	if ate {
		g.score += foodPoints
		g.foods++
		if g.score > g.best {
			g.best = g.score
		}
		g.placeFood()
	}
	return ate
}
