package main

// The autopilot: expectimax over the real rules at a fixed depth, with a
// heuristic of the usual kind (empty cells, merges available, tiles kept in
// rows and columns that only rise one way, small tiles preferred). It is
// integer arithmetic from end to end and it never looks at a clock, so a
// board always gets the same answer on every machine; ties go in the order
// of searchOrder.

// eboard is a board of exponents (0 empty, 1 is a 2, 2 a 4, ...), a value
// type so the search copies it freely.
type eboard struct {
	n int
	c [maxSize * maxSize]uint8
}

func exponent(v int) uint8 {
	var e uint8
	for v > 1 {
		v >>= 1
		e++
	}
	return e
}

func newEBoard(n int, cells []int) eboard {
	b := eboard{n: n}
	for i, v := range cells {
		if v > 0 {
			b.c[i] = exponent(v)
		}
	}
	return b
}

// at is the cell index of position k along line i in direction d: k = 0 is
// next to the wall the tiles slide towards (the same order as game.line).
func (b *eboard) at(d dir, i, k int) int {
	n := b.n
	switch d {
	case dirLeft:
		return i*n + k
	case dirRight:
		return i*n + (n - 1 - k)
	case dirUp:
		return k*n + i
	default:
		return (n-1-k)*n + i
	}
}

// slide is the board after sliding d, and whether anything moved.
func (b eboard) slide(d dir) (eboard, bool) {
	out := eboard{n: b.n}
	n := b.n
	moved := false
	for i := 0; i < n; i++ {
		w := 0
		last := -1
		for k := 0; k < n; k++ {
			v := b.c[b.at(d, i, k)]
			if v == 0 {
				continue
			}
			if last >= 0 && out.c[out.at(d, i, last)] == v {
				out.c[out.at(d, i, last)] = v + 1
				last = -1
				continue
			}
			out.c[out.at(d, i, w)] = v
			last = w
			w++
		}
	}
	for i := 0; i < n*n; i++ {
		if out.c[i] != b.c[i] {
			moved = true
			break
		}
	}
	return out, moved
}

// Heuristic weights.
const (
	lostPenalty = 200000
	emptyWeight = 270
	mergeWeight = 700
	monoWeight  = 47
	sumWeight   = 11
)

func pow3(r int64) int64 { return r * r * r }
func pow4(r int64) int64 { return r * r * r * r }

// eval scores a board: higher is better.
func (b *eboard) eval() int64 {
	n := b.n
	var empty, merges int64
	var sum, mono int64
	for pass := 0; pass < 2; pass++ {
		for i := 0; i < n; i++ {
			var left, right int64
			prev := int64(-1)
			for k := 0; k < n; k++ {
				var r int64
				if pass == 0 {
					r = int64(b.c[i*n+k])
				} else {
					r = int64(b.c[k*n+i])
				}
				if pass == 0 && r == 0 {
					empty++
				}
				sum += pow3(r)
				if prev >= 0 {
					if prev == r && r != 0 {
						merges++
					}
					if prev > r {
						left += pow4(prev) - pow4(r)
					} else {
						right += pow4(r) - pow4(prev)
					}
				}
				prev = r
			}
			if left < right {
				mono += left
			} else {
				mono += right
			}
		}
	}
	return lostPenalty + emptyWeight*empty + mergeWeight*merges - monoWeight*mono - sumWeight*sum
}

// searchOrder is the order moves are tried in; the first of equal scores wins.
var searchOrder = [...]dir{dirLeft, dirDown, dirRight, dirUp}

// value is the expected score of a board just after a slide, looking ply
// more moves ahead: the mean over every spawn (a 2 with weight 9, a 4 with
// weight 1) of the best slide that follows.
func value(b eboard, ply int) int64 {
	if ply == 0 {
		return b.eval()
	}
	var total int64
	var k int64
	for i := 0; i < b.n*b.n; i++ {
		if b.c[i] != 0 {
			continue
		}
		k++
		for e, w := range [2]int64{9, 1} {
			nb := b
			nb.c[i] = uint8(e + 1)
			// A board with no move scores 0: the game ends there.
			var best int64
			found := false
			for _, d := range searchOrder {
				s, moved := nb.slide(d)
				if !moved {
					continue
				}
				if v := value(s, ply-1); !found || v > best {
					best, found = v, true
				}
			}
			total += w * best
		}
	}
	if k == 0 {
		return b.eval()
	}
	return total / (10 * k)
}

// searchDepth is the plies to look ahead: the base depth, and one more once
// the board is crowded, when there are few spawns to branch on.
func searchDepth(b eboard, base int) int {
	empty := 0
	for i := 0; i < b.n*b.n; i++ {
		if b.c[i] == 0 {
			empty++
		}
	}
	if empty <= 3 {
		return base + 1
	}
	return base
}

// bestMove is the autopilot's move for cells. ok is false when no slide
// changes the board.
func bestMove(n int, cells []int, depth int) (d dir, ok bool) {
	b := newEBoard(n, cells)
	ply := searchDepth(b, depth)
	var best int64
	for _, c := range searchOrder {
		s, moved := b.slide(c)
		if !moved {
			continue
		}
		if v := value(s, ply-1); !ok || v > best {
			best, d, ok = v, c, true
		}
	}
	return d, ok
}
