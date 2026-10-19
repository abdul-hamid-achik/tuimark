package main

import "math/rand"

// The rules of the game, with no Tuimark in sight: a field, a falling piece,
// a seeded 7-bag, and a game clock the caller advances. Everything here is a
// function of the seed and the sequence of calls, so tests and the -demo
// script replay it exactly.

const (
	boardW   = 10
	boardH   = 22 // the whole field: hidden rows first
	hidden   = 2  // rows above the skyline; pieces spawn in them
	visibleH = boardH - hidden

	queueLen  = 5   // pieces the next panel shows
	lockMs    = 500 // lock delay
	maxResets = 15  // moves or turns that may restart the lock delay
	flashMs   = 200 // how long full rows flash before they collapse
	histLen   = 20  // samples of the stack height sparkline
	maxLevel  = 15  // gravity stops getting faster here
)

// gravity is the time one row takes to fall, in ms, at levels 1 to 15. The
// guideline's curve (0.8 - (L-1) x 0.007)^(L-1) seconds, rounded to whole
// milliseconds.
var gravity = [maxLevel]int64{1000, 793, 618, 473, 355, 262, 190, 135, 94, 64, 43, 28, 18, 11, 7}

var (
	lineScore  = [5]int{0, 100, 300, 500, 800}
	tspinScore = [5]int{400, 800, 1200, 1600, 1600}
	lineName   = [5]string{"", "SINGLE", "DOUBLE", "TRIPLE", "TETRIS"}
)

type game struct {
	rng        *rand.Rand
	startLevel int

	grid [boardH][boardW]kind
	cur  piece
	has  bool // a piece is falling

	hold     kind
	holdUsed bool
	queue    []kind

	score, lines, pieces int
	combo                int // -1 until a lock clears rows; then clears in a row, less one
	b2b                  bool
	best                 int // the best score since the program started
	last                 string
	hist                 []int // stack height after each lock, newest last

	t       int64 // the game clock, ms of play
	gravAcc int64 // time since the piece last fell a row
	lockAt  int64 // when a grounded piece locks; -1 while it is not grounded
	resets  int   // lock delay restarts used since the piece last got lower
	lowest  int   // the lowest row the piece has reached
	rotated bool  // the last thing the piece did was a successful turn
	id      int   // pieces spawned: names the current one

	clearing   bool
	rows       []int // full rows, while they flash
	clearUntil int64
	over       bool
}

func newGame(seed int64, startLevel int) *game {
	g := &game{rng: rand.New(rand.NewSource(seed)), startLevel: startLevel}
	g.restart()
	return g
}

// restart starts a new game; the RNG goes on from where it was and the best
// score is kept.
func (g *game) restart() {
	g.grid = [boardH][boardW]kind{}
	g.has, g.hold, g.holdUsed = false, kNone, false
	g.queue = nil
	g.score, g.lines, g.pieces, g.combo = 0, 0, 0, -1
	g.b2b, g.last, g.hist = false, "", nil
	g.t, g.gravAcc, g.lockAt, g.resets, g.lowest = 0, 0, -1, 0, 0
	g.rotated, g.clearing, g.rows, g.over = false, false, nil, false
	g.spawn(g.next())
}

func (g *game) level() int { return g.startLevel + g.lines/10 }

// gravityMs is the time one row takes to fall at the current level.
func (g *game) gravityMs() int64 {
	l := g.level()
	if l > maxLevel {
		l = maxLevel
	}
	return gravity[l-1]
}

// next takes the next piece of the 7-bag. The queue always holds at least
// queueLen + 1 pieces, laid down a whole shuffled bag at a time, so every
// run of seven starting at a bag's edge holds each piece once.
func (g *game) next() kind {
	for len(g.queue) <= queueLen {
		bag := kinds
		for i := len(bag) - 1; i > 0; i-- {
			j := g.rng.Intn(i + 1)
			bag[i], bag[j] = bag[j], bag[i]
		}
		g.queue = append(g.queue, bag[:]...)
	}
	k := g.queue[0]
	g.queue = g.queue[1:]
	return k
}

// preview is the pieces after the current one. next always leaves at least
// queueLen of them in the queue.
func (g *game) preview() []kind { return append([]kind(nil), g.queue[:queueLen]...) }

func (g *game) valid(p piece) bool {
	for _, c := range p.cells() {
		if c.x < 0 || c.x >= boardW || c.y < 0 || c.y >= boardH || g.grid[c.y][c.x] != kNone {
			return false
		}
	}
	return true
}

func (g *game) grounded() bool {
	p := g.cur
	p.y++
	return !g.valid(p)
}

// spawn puts a piece of kind k at the top, centred, and lets it fall a row
// at once if it can, so it shows in the field. A piece that does not fit is
// the end of the game (block out).
func (g *game) spawn(k kind) {
	p := piece{k: k, x: 3, y: 0}
	g.id++
	g.gravAcc, g.lockAt, g.resets, g.rotated = 0, -1, 0, false
	if !g.valid(p) {
		g.has, g.over = false, true
		return
	}
	g.cur, g.has = p, true
	g.lowest = p.y
	p.y++
	if g.valid(p) {
		g.cur = p
		g.lowest = p.y
	}
	g.settle(false)
}

// settle is called after the piece moves: it starts, restarts, or cancels
// the lock delay. A move or turn made while the delay is running pays one of
// the maxResets restarts, whether or not it leaves the piece resting, so
// lifting the piece off the floor and letting it land again cannot be
// repeated for ever. A piece that lands once the restarts are used up locks
// at once, unless it has reached a row lower than any before (tryDown
// refills the count then).
func (g *game) settle(reset bool) {
	timed := g.lockAt >= 0
	granted := false
	if reset && timed && g.resets < maxResets {
		g.resets++
		granted = true
	}
	if !g.grounded() {
		g.lockAt = -1
		return
	}
	switch {
	case !timed:
		g.lockAt = g.t + lockMs
		if g.resets >= maxResets {
			g.lockAt = g.t
		}
	case granted:
		g.lockAt = g.t + lockMs
	}
}

func (g *game) canAct() bool { return g.has && !g.clearing && !g.over }

// advance moves the game clock by dt ms: rows fall, a grounded piece locks
// when its delay is out, and flashing rows collapse.
func (g *game) advance(dt int64) {
	if g.over || dt <= 0 {
		return
	}
	g.t += dt
	if g.clearing {
		if g.t >= g.clearUntil {
			g.finishClear()
		}
		return
	}
	if !g.has {
		return
	}
	gi := g.gravityMs()
	g.gravAcc += dt
	for g.gravAcc >= gi {
		g.gravAcc -= gi
		if !g.tryDown() {
			g.gravAcc = 0
			break
		}
	}
	if g.has && g.lockAt >= 0 && g.t >= g.lockAt {
		g.lock()
	}
}

// tryDown falls one row if there is room.
func (g *game) tryDown() bool {
	p := g.cur
	p.y++
	if !g.valid(p) {
		return false
	}
	g.cur = p
	g.rotated = false
	if p.y > g.lowest {
		g.lowest = p.y
		g.resets = 0
	}
	g.settle(false)
	return true
}

func (g *game) shift(dx int) bool {
	if !g.canAct() {
		return false
	}
	p := g.cur
	p.x += dx
	if !g.valid(p) {
		return false
	}
	g.cur = p
	g.rotated = false
	g.settle(true)
	return true
}

// rotate turns the piece a quarter clockwise (dir 1) or back (dir -1), trying
// the five SRS kicks in order. It reports whether the piece turned; O does not.
func (g *game) rotate(dir int) bool {
	if !g.canAct() || g.cur.k == kO {
		return false
	}
	to := (g.cur.rot + dir + 4) % 4
	for _, off := range kickOffsets(g.cur.k, g.cur.rot, to) {
		p := g.cur
		p.rot, p.x, p.y = to, p.x+off.x, p.y+off.y
		if g.valid(p) {
			g.cur = p
			g.rotated = true
			if p.y > g.lowest {
				g.lowest = p.y
				g.resets = 0
			}
			g.settle(true)
			return true
		}
	}
	return false
}

func (g *game) addScore(n int) {
	g.score += n
	if g.score > g.best {
		g.best = g.score
	}
}

// softDrop falls one row for one point; a grounded piece does not move.
func (g *game) softDrop() bool {
	if !g.canAct() || !g.tryDown() {
		return false
	}
	g.gravAcc = 0
	g.addScore(1)
	return true
}

// dropDistance is how many rows the piece can fall.
func (g *game) dropDistance() int {
	p, n := g.cur, 0
	for {
		p.y++
		if !g.valid(p) {
			return n
		}
		n++
	}
}

// hardDrop falls as far as it can for two points a row and locks at once.
func (g *game) hardDrop() bool {
	if !g.canAct() {
		return false
	}
	n := g.dropDistance()
	if n > 0 {
		g.cur.y += n
		g.rotated = false
		g.addScore(2 * n)
	}
	g.lock()
	return true
}

// holdPiece swaps the falling piece with the held one (or with the next, the
// first time). It works once per piece.
func (g *game) holdPiece() bool {
	if !g.canAct() || g.holdUsed {
		return false
	}
	k := g.hold
	if k == kNone {
		k = g.next()
	}
	g.hold, g.holdUsed = g.cur.k, true
	g.spawn(k)
	return true
}

// ghost is the cells the piece would fill if it dropped now.
func (g *game) ghost() []point {
	if !g.canAct() {
		return nil
	}
	p := g.cur
	p.y += g.dropDistance()
	cells := p.cells()
	return cells[:]
}

// corners counts the corners of the T's box that are filled, wall, or floor.
func (g *game) corners() int {
	n := 0
	for _, d := range [4]point{{0, 0}, {2, 0}, {0, 2}, {2, 2}} {
		x, y := g.cur.x+d.x, g.cur.y+d.y
		switch {
		case x < 0 || x >= boardW || y >= boardH:
			n++ // the walls and the floor count as filled
		case y < 0:
			// Above the field is open space: a T kicked up to the ceiling
			// has nothing to be stuck against there.
		case g.grid[y][x] != kNone:
			n++
		}
	}
	return n
}

// lock writes the piece into the field and scores it. Full rows start to
// flash and the next piece waits for them; otherwise the next piece comes
// at once. A piece that locks wholly above the skyline ends the game.
func (g *game) lock() {
	tspin := g.cur.k == kT && g.rotated && g.corners() >= 3
	out := true
	for _, c := range g.cur.cells() {
		g.grid[c.y][c.x] = g.cur.k
		if c.y >= hidden {
			out = false
		}
	}
	g.has, g.holdUsed, g.lockAt = false, false, -1
	g.pieces++
	if out {
		g.over = true
		return
	}
	rows := g.fullRows()
	n := len(rows)
	lvl := g.level()
	if n == 0 {
		g.combo = -1
		if tspin {
			g.addScore(tspinScore[0] * lvl)
			g.last = "T-SPIN"
		}
		g.sample()
		g.spawn(g.next())
		return
	}
	pts := lineScore[n]
	name := lineName[n]
	if tspin {
		pts = tspinScore[n]
		name = "T-SPIN " + name
	}
	if n == 4 || tspin {
		if g.b2b {
			pts = pts * 3 / 2
			name = "B2B " + name
		}
		g.b2b = true
	} else {
		g.b2b = false
	}
	g.addScore(pts * lvl)
	g.combo++
	if g.combo > 0 {
		g.addScore(50 * g.combo * lvl)
	}
	g.last = name
	g.lines += n
	g.clearing, g.rows, g.clearUntil = true, rows, g.t+flashMs
}

func (g *game) fullRows() []int {
	var rows []int
	for y := 0; y < boardH; y++ {
		full := true
		for x := 0; x < boardW; x++ {
			if g.grid[y][x] == kNone {
				full = false
				break
			}
		}
		if full {
			rows = append(rows, y)
		}
	}
	return rows
}

// finishClear removes the flashing rows, lets what is above fall, and
// brings the next piece.
func (g *game) finishClear() {
	gone := map[int]bool{}
	for _, y := range g.rows {
		gone[y] = true
	}
	var grid [boardH][boardW]kind
	dst := boardH - 1
	for y := boardH - 1; y >= 0; y-- {
		if gone[y] {
			continue
		}
		grid[dst] = g.grid[y]
		dst--
	}
	g.grid = grid
	g.clearing, g.rows = false, nil
	g.sample()
	g.spawn(g.next())
}

// stackHeight is the height of the highest filled cell, capped at the
// sparkline's top.
func (g *game) stackHeight() int {
	for y := 0; y < boardH; y++ {
		for x := 0; x < boardW; x++ {
			if g.grid[y][x] != kNone {
				h := boardH - y
				if h > visibleH {
					h = visibleH
				}
				return h
			}
		}
	}
	return 0
}

// sample appends the stack height to the sparkline's history.
func (g *game) sample() {
	g.hist = append(g.hist, g.stackHeight())
	if len(g.hist) > histLen {
		g.hist = g.hist[len(g.hist)-histLen:]
	}
}
