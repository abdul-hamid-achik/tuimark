package main

// The autopilot: for the falling piece it tries every rotation in every
// column, drops it straight down, and scores the field that results with the
// El-Tetris weights (aggregate height, completed lines, holes, bumpiness).
// The weights are the published ones scaled by a million and used as
// integers, so the choice never depends on how a CPU rounds floating point.
// Rotations and columns are tried in a fixed order and only a strictly better
// score replaces the best, so a seed always plays out the same way.

const (
	wHeight int64 = -510066
	wLines  int64 = 760666
	wHoles  int64 = -356630
	wBump   int64 = -184483

	// autoRows is how many rows one act of the autopilot drops a piece that
	// is already in its column.
	autoRows = 8
)

// plan is where the autopilot wants the piece with the given id to end up.
type plan struct {
	id   int  // the piece it is for (game.id)
	rot  int  // the rotation to land in
	x    int  // the box column to land in
	ok   bool // a placement was found
	fail bool // an act was blocked: stop being clever, drop it
}

// rotations is how many different orientations a piece has.
func rotations(k kind) int {
	switch k {
	case kO:
		return 1
	case kI, kS, kZ:
		return 2
	}
	return 4
}

// choose picks the best landing for the falling piece.
func (g *game) choose() plan {
	p := plan{id: g.id}
	if !g.has {
		return p
	}
	var best int64
	for rot := 0; rot < rotations(g.cur.k); rot++ {
		for x := -2; x < boardW; x++ {
			start := piece{k: g.cur.k, rot: rot, x: x, y: 0}
			if !g.valid(start) {
				continue
			}
			land := start
			for {
				next := land
				next.y++
				if !g.valid(next) {
					break
				}
				land = next
			}
			grid := g.grid
			above := true
			for _, c := range land.cells() {
				grid[c.y][c.x] = land.k
				if c.y >= hidden {
					above = false
				}
			}
			if above {
				continue // would lock out
			}
			s := score(&grid)
			if !p.ok || s > best {
				best, p.ok, p.rot, p.x = s, true, rot, x
			}
		}
	}
	return p
}

// score clears the full rows of grid and weighs what is left.
func score(grid *[boardH][boardW]kind) int64 {
	var lines int64
	var rows [boardH][boardW]kind
	dst := boardH - 1
	for y := boardH - 1; y >= 0; y-- {
		full := true
		for x := 0; x < boardW; x++ {
			if grid[y][x] == kNone {
				full = false
				break
			}
		}
		if full {
			lines++
			continue
		}
		rows[dst] = grid[y]
		dst--
	}
	var height, holes, bump int64
	prev := int64(-1)
	for x := 0; x < boardW; x++ {
		h := int64(0)
		for y := 0; y < boardH; y++ {
			if rows[y][x] != kNone {
				if h == 0 {
					h = int64(boardH - y)
				}
			} else if h > 0 {
				holes++
			}
		}
		height += h
		if prev >= 0 {
			if h > prev {
				bump += h - prev
			} else {
				bump += prev - h
			}
		}
		prev = h
	}
	return wHeight*height + wLines*lines + wHoles*holes + wBump*bump
}

// autoAct is one act of the autopilot: it turns the piece one step and shifts
// it one column toward the plan, and once the piece is in place it drops it,
// autoRows rows at a time, and locks it when it rests. It reports whether it
// did anything. A blocked act makes the plan give up and the piece drop where
// it is.
func (g *game) autoAct(p *plan) bool {
	if !g.canAct() {
		return false
	}
	if p.id != g.id {
		*p = g.choose()
	}
	if p.ok && !p.fail && (g.cur.rot != p.rot || g.cur.x != p.x) {
		if g.cur.rot != p.rot {
			dir := 1
			if (p.rot-g.cur.rot+4)%4 == 3 {
				dir = -1
			}
			if !g.rotate(dir) {
				p.fail = true
				return true
			}
		}
		if g.cur.x != p.x {
			dx := 1
			if p.x < g.cur.x {
				dx = -1
			}
			if !g.shift(dx) {
				p.fail = true
			}
		}
		return true
	}
	if g.dropDistance() > 0 {
		for i := 0; i < autoRows && g.softDrop(); i++ {
		}
	} else {
		g.hardDrop()
	}
	return true
}
