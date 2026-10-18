package main

// The autopilot: breadth-first search to the food, with the fallback of
// the move that keeps the most room. It is a pure function of the game,
// with a fixed direction order for every tie, so a seed always plays out
// the same way.

// autoDir picks the direction for the next move.
func (g *game) autoDir() point {
	heading := g.dir
	if n := len(g.turns); n > 0 {
		heading = g.turns[n-1]
	}
	var legal []point
	for _, d := range dirs {
		if d == heading.neg() {
			continue
		}
		if g.safeStep(g.snake[0].add(d)) {
			legal = append(legal, d)
		}
	}
	if len(legal) == 0 {
		return heading // doomed: any move ends the game
	}
	want, ok := g.pathStep()
	bestDir, bestRoom := legal[0], -1
	for _, d := range legal {
		room := g.roomAfter(d)
		if ok && d == want && room >= len(g.snake) {
			return d // the shortest way to the food, and not a trap
		}
		if room > bestRoom {
			bestDir, bestRoom = d, room
		}
	}
	return bestDir
}

// safeStep reports whether the head may enter p on the next move: inside
// the board and not on a body cell that stays put (the tail moves on).
func (g *game) safeStep(p point) bool {
	if !g.inside(p) {
		return false
	}
	for _, b := range g.snake[:len(g.snake)-1] {
		if b == p {
			return false
		}
	}
	return true
}

// pathStep is the first step of the shortest path from the head to the
// food. A body cell counts as blocked until the tail has moved past it, so
// the search may route through cells the snake is about to leave.
func (g *game) pathStep() (point, bool) {
	if !g.inside(g.food) {
		return point{}, false
	}
	n := len(g.snake)
	busy := make([]int, g.w*g.h) // steps until a cell is free; 0 = free now
	for i, p := range g.snake {
		busy[p.y*g.w+p.x] = n - i
	}
	first := make([]int, g.w*g.h) // direction index + 1 of the first step
	seen := make([]bool, g.w*g.h)
	start := g.snake[0]
	seen[start.y*g.w+start.x] = true
	type item struct {
		p point
		d int
	}
	queue := []item{{start, 0}}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for i, d := range dirs {
			np := cur.p.add(d)
			if !g.inside(np) {
				continue
			}
			idx := np.y*g.w + np.x
			if seen[idx] || cur.d+1 < busy[idx] {
				continue
			}
			seen[idx] = true
			if cur.d == 0 {
				first[idx] = i + 1
			} else {
				first[idx] = first[cur.p.y*g.w+cur.p.x]
			}
			if np == g.food {
				return dirs[first[idx]-1], true
			}
			queue = append(queue, item{np, cur.d + 1})
		}
	}
	return point{}, false
}

// roomAfter counts the cells the head could still reach after moving in d
// (the snake not growing), a flood fill over the free cells.
func (g *game) roomAfter(d point) int {
	head := g.snake[0].add(d)
	blocked := make([]bool, g.w*g.h)
	for _, p := range g.snake[:len(g.snake)-1] {
		blocked[p.y*g.w+p.x] = true
	}
	blocked[head.y*g.w+head.x] = true
	count := 0
	stack := []point{head}
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		count++
		for _, dd := range dirs {
			np := p.add(dd)
			if !g.inside(np) || blocked[np.y*g.w+np.x] {
				continue
			}
			blocked[np.y*g.w+np.x] = true
			stack = append(stack, np)
		}
	}
	return count
}
