package parse

import (
	"fmt"
	"strings"
)

// cycleShown is how many templates a V019 names before it shortens a cycle
// (the owner's decision in the F2 review): a cycle of at most cycleShown
// templates is named whole, as before, and a longer one by its first
// cycleShown templates and the count of the others.
const cycleShown = 8

// checkCycles reports each <use> of a template's body that closes a cycle
// of templates (SPEC v0.4b §6.18.3): it is written in a template A and
// names a template that is A, or holds a use of A through the uses of its
// body at any depth. Each is one V019, used or not, and never expands.
//
// The cycle a V019 names is the shortest one through its use, the uses
// taken in document order (pathFrom). Finding it for every use of a long
// cycle is quadratic (ring10000 took 12 s), so the search for each use
// stops at cycleShown templates, which is all a message names whole; past
// that, a component that is a single ring has one cycle, read off the
// ring, and any other component pays a full search for that use.
func (b *builder) checkCycles() {
	cp := b.comp
	n := len(cp.order)
	idx := make(map[*tmpl]int, n)
	for i, t := range cp.order {
		idx[t] = i
		t.uses = usesIn(nil, written(t.node))
	}
	// adj[i] holds the template each use of template i names, in document
	// order, -1 for a use that names none.
	adj := make([][]int, n)
	for i, t := range cp.order {
		adj[i] = make([]int, len(t.uses))
		for j, u := range t.uses {
			adj[i][j] = -1
			if s := cp.tmpls[u.Attrs["template"]]; s != nil {
				adj[i][j] = idx[s]
			}
		}
	}
	g := newCycleGraph(adj)
	for i, t := range cp.order {
		for j, u := range t.uses {
			s := adj[i][j]
			if s < 0 || g.comp[s] != g.comp[i] {
				continue
			}
			ids, more := g.cycle(i, s)
			names := make([]string, len(ids), len(ids)+2)
			for k, x := range ids {
				names[k] = cp.order[x].node.ID
			}
			if more > 0 {
				names = append(names, fmt.Sprintf("… (%d more)", more))
			}
			names = append(names, t.node.ID)
			b.errUse(u, "V019", "<use template=%q> in <template id=%q> makes a cycle of templates: %s", u.Attrs["template"], t.node.ID, strings.Join(names, " → "))
			if cp.cyclic == nil {
				cp.cyclic = map[*RawNode]bool{}
			}
			cp.cyclic[cp.useOf[u].raw] = true
		}
	}
}

// cycleGraph is the graph of templates (adj, by index) with its strongly
// connected components (comp): a use from A to B closes a cycle exactly
// when A and B are in one component.
type cycleGraph struct {
	adj  [][]int
	comp []int
	// ring[c] is set when component c is a single ring: each of its
	// templates names exactly one template of c (succ), through one use or
	// several.
	ring []bool
	succ []int
	size []int
	// The search's state, reused: seen[x] == epoch marks x reached, from
	// is the template it was reached from.
	seen  []int
	from  []int
	epoch int
	queue []int
}

func newCycleGraph(adj [][]int) *cycleGraph {
	n := len(adj)
	g := &cycleGraph{adj: adj, comp: components(adj), succ: make([]int, n), seen: make([]int, n), from: make([]int, n)}
	nc := 0
	for _, c := range g.comp {
		nc = max(nc, c+1)
	}
	g.ring, g.size = make([]bool, nc), make([]int, nc)
	for c := range g.ring {
		g.ring[c] = true
	}
	for i := range adj {
		c := g.comp[i]
		g.size[c]++
		g.succ[i] = -1
		for _, s := range adj[i] {
			if s < 0 || g.comp[s] != c || s == g.succ[i] {
				continue
			}
			if g.succ[i] >= 0 {
				g.ring[c] = false // a second template of c
			}
			g.succ[i] = s
		}
		if g.succ[i] < 0 {
			g.ring[c] = false // no use inside c: a template alone
		}
	}
	return g
}

// cycle is the cycle the use from t to s closes, from t: the templates it
// names, at most cycleShown of them, and how many more it holds (0 when it
// is named whole). The message closes it with t again.
func (g *cycleGraph) cycle(t, s int) (ids []int, more int) {
	if p := g.pathFrom(s, t, cycleShown-1); p != nil {
		return append([]int{t}, p[:len(p)-1]...), 0
	}
	c := g.comp[t]
	if g.ring[c] {
		ids = []int{t}
		for x := s; len(ids) < cycleShown; x = g.succ[x] {
			ids = append(ids, x)
		}
		return ids, g.size[c] - cycleShown
	}
	p := g.pathFrom(s, t, -1)
	return append([]int{t}, p[:cycleShown-1]...), len(p) - cycleShown
}

// pathFrom is the shortest path of templates from s to t, both included,
// the uses taken in document order, or nil when it is longer than limit
// uses (limit < 0: no limit). It searches t's component only, which holds
// every path from s to t.
func (g *cycleGraph) pathFrom(s, t, limit int) []int {
	if s == t {
		return []int{t}
	}
	c := g.comp[t]
	g.epoch++
	g.seen[s], g.from[s] = g.epoch, -1
	g.queue = append(g.queue[:0], s)
	for depth := 0; len(g.queue) > 0 && (limit < 0 || depth < limit); depth++ {
		level := g.queue
		g.queue = nil
		for _, x := range level {
			for _, y := range g.adj[x] {
				if y < 0 || g.comp[y] != c || g.seen[y] == g.epoch {
					continue
				}
				g.seen[y], g.from[y] = g.epoch, x
				if y == t {
					var path []int
					for p := t; p >= 0; p = g.from[p] {
						path = append(path, p)
					}
					for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
						path[i], path[j] = path[j], path[i]
					}
					return path
				}
				g.queue = append(g.queue, y)
			}
		}
	}
	return nil
}

// components numbers the strongly connected components of adj (Tarjan's
// algorithm, iterative, so a long chain of templates does not nest calls).
func components(adj [][]int) []int {
	n := len(adj)
	index, low, comp := make([]int, n), make([]int, n), make([]int, n)
	for i := range index {
		index[i] = -1
	}
	on := make([]bool, n)
	var stack []int
	type frame struct{ v, next int }
	var calls []frame
	counter, ncomp := 0, 0
	for root := range adj {
		if index[root] >= 0 {
			continue
		}
		calls = append(calls[:0], frame{v: root})
		index[root], low[root] = counter, counter
		counter++
		stack = append(stack, root)
		on[root] = true
		for len(calls) > 0 {
			f := &calls[len(calls)-1]
			v := f.v
			if f.next < len(adj[v]) {
				w := adj[v][f.next]
				f.next++
				switch {
				case w < 0:
				case index[w] < 0:
					index[w], low[w] = counter, counter
					counter++
					stack = append(stack, w)
					on[w] = true
					calls = append(calls, frame{v: w})
				case on[w]:
					low[v] = min(low[v], index[w])
				}
				continue
			}
			calls = calls[:len(calls)-1]
			if len(calls) > 0 {
				p := calls[len(calls)-1].v
				low[p] = min(low[p], low[v])
			}
			if low[v] == index[v] {
				for {
					w := stack[len(stack)-1]
					stack = stack[:len(stack)-1]
					on[w] = false
					comp[w] = ncomp
					if w == v {
						break
					}
				}
				ncomp++
			}
		}
	}
	return comp
}
