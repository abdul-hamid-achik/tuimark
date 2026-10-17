package layout

import "github.com/abdul-hamid-achik/tuimark/internal/ir"

// Scratch is the memory one Layout call cuts its work lists from: the
// size lists, the size specs, and the child lists a container's
// allocation and measure build, one element per child. None of them
// outlives the call (nothing the engine leaves on a box or returns comes
// from it), so a Scratch is reset at the start of each Layout and its
// memory serves the next one: laying out a container of many children
// (a list of 20 000 rows) then allocates nothing per frame. A Scratch
// serves one Layout call at a time; the host keeps one per app, whose
// renders hold its lock. The zero value is ready to use.
type Scratch struct {
	ints    arena[int]
	scalars arena[ir.Scalar]
	boxes   arena[*Box]
}

// reset makes all of s's memory free again. The lists holding pointers
// are cleared, so that s keeps nothing the last call left alive.
func (s *Scratch) reset() {
	s.ints.reset(false)
	s.scalars.reset(true)
	s.boxes.reset(true)
}

// arenaChunk is the size, in elements, of an arena's smallest chunk.
const arenaChunk = 4096

// arena hands out zeroed slices cut from a few chunks, in order, until
// reset. A slice it hands out stays valid until then: chunks are never
// moved or reused before a reset. ci and off are the chunk being cut and
// the first free element in it.
type arena[T any] struct {
	chunks [][]T
	ci     int
	off    int
}

// take returns a zeroed slice of length and capacity n.
func (a *arena[T]) take(n int) []T {
	for a.ci < len(a.chunks) {
		if c := a.chunks[a.ci]; len(c)-a.off >= n {
			out := c[a.off : a.off+n : a.off+n]
			a.off += n
			clear(out)
			return out
		}
		a.ci++
		a.off = 0
	}
	c := make([]T, max(n, arenaChunk))
	a.chunks = append(a.chunks, c)
	a.ci, a.off = len(a.chunks)-1, n
	return c[:n:n]
}

// reset frees every slice a handed out. When the last round needed more
// than one chunk, they are merged into one as large as all of them, so
// that the same round fits in it next time. clearUsed clears what the
// round used (for element types holding pointers).
func (a *arena[T]) reset(clearUsed bool) {
	if clearUsed {
		for i := 0; i < len(a.chunks) && i <= a.ci; i++ {
			c := a.chunks[i]
			if i == a.ci {
				c = c[:a.off]
			}
			clear(c)
		}
	}
	if len(a.chunks) > 1 {
		total := 0
		for _, c := range a.chunks {
			total += len(c)
		}
		a.chunks = [][]T{make([]T, total)}
	}
	a.ci, a.off = 0, 0
}
