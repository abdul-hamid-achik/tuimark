package parse

import (
	"fmt"
	"strings"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// cycleShown is how many templates a V019 names before it shortens a cycle
// (the owner's decision in the F2 review): a cycle of at most cycleShown
// templates is named whole, as before, and a longer one by its first
// cycleShown templates and the count of the others.
const cycleShown = 8

// shownID is id as a composition diagnostic shows it (ir.ShownID): a
// V019 shows every template id it quotes so.
func shownID(id string) string { return ir.ShownID(id) }

// cycleSearchBudget is how many uses the searches for a shortest cycle may
// follow in one document, all of them together, in the document order of
// the uses that close the cycles (the F2 review). A use whose search finds
// a cycle of at most cycleShown templates within it is named by that
// cycle, the shortest; a use whose search finds none, or is cut off, is
// named by its component's simple cycle (cycleGraph.fallback). A million
// steps costs a few milliseconds.
const cycleSearchBudget = 1000000

// checkCycles reports each <use> of a template's body that closes a cycle
// of templates (SPEC v0.4b §6.18.3): it is written in a template A and
// names a template that is A, or holds a use of A through the uses of its
// body at any depth. Each is one V019, used or not, and never expands.
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
	res := newCycleGraph(adj, cycleSearchBudget).cycles()
	for i, t := range cp.order {
		for j, u := range t.uses {
			c := res[i][j]
			if c == nil {
				continue
			}
			names := make([]string, len(c.ids))
			for k, x := range c.ids {
				names[k] = cp.order[x].node.ID
			}
			b.errUse(u, "V019", "<use template=%q> in <template id=%q> makes a cycle of templates: %s", shownID(u.Attrs["template"]), shownID(t.node.ID), cycleText(names, c.more))
			if cp.cyclic == nil {
				cp.cyclic = map[*RawNode]bool{}
			}
			cp.cyclic[cp.useOf[u].raw] = true
		}
	}
}

// cycleText is how a V019 names a cycle (each id as shownID shows it): its
// templates from the one that
// holds the use (names, at most cycleShown), "… (N more)" when more
// remain, and the first again.
func cycleText(names []string, more int) string {
	out := make([]string, 0, len(names)+2)
	for _, n := range names {
		out = append(out, shownID(n))
	}
	if more > 0 {
		out = append(out, fmt.Sprintf("… (%d more)", more))
	}
	return strings.Join(append(out, out[0]), " → ")
}

// cycleResult is the cycle a V019 names: its first templates, from the one
// that holds the use (ids, at most cycleShown), and how many more it holds.
// all is the whole cycle, kept only for the tests (cycleGraph.full).
type cycleResult struct {
	ids  []int
	more int
	all  []int
}

// cycleGraph is the graph of templates (adj, by index) with its strongly
// connected components (comp): a use from A to B closes a cycle exactly
// when A and B are in one component.
type cycleGraph struct {
	adj  [][]int
	comp []int
	// budget is what is left of cycleSearchBudget.
	budget int
	// full keeps each cycle whole in cycleResult.all (tests only).
	full bool
	// The shortest search's state, reused: seen[x] == epoch marks x
	// reached, from is the template it was reached from.
	seen  []int
	from  []int
	epoch int
}

func newCycleGraph(adj [][]int, budget int) *cycleGraph {
	n := len(adj)
	return &cycleGraph{adj: adj, comp: components(adj), budget: budget, seen: make([]int, n), from: make([]int, n)}
}

// cycles finds the cycle of each use that closes one: res[i][j] for the
// use j of template i, nil for a use that closes none.
func (g *cycleGraph) cycles() [][]*cycleResult {
	n := len(g.adj)
	res := make([][]*cycleResult, n)
	// later[i] holds the uses of template i the fallback names; byComp
	// the templates of each component that has one, in document order.
	later := make([][]int, n)
	byComp := map[int][]int{}
	var comps []int
	for i := range g.adj {
		res[i] = make([]*cycleResult, len(g.adj[i]))
		for j, s := range g.adj[i] {
			if s < 0 || g.comp[s] != g.comp[i] {
				continue
			}
			if p := g.shortest(s, i); p != nil {
				ids := append([]int{i}, p[:len(p)-1]...)
				r := &cycleResult{ids: ids}
				if g.full {
					r.all = ids
				}
				res[i][j] = r
				continue
			}
			c := g.comp[i]
			if _, ok := byComp[c]; !ok {
				byComp[c] = nil
				comps = append(comps, c)
			}
			later[i] = append(later[i], j)
		}
	}
	if len(comps) == 0 {
		return res
	}
	for i := range g.adj {
		if _, ok := byComp[g.comp[i]]; ok {
			byComp[g.comp[i]] = append(byComp[g.comp[i]], i)
		}
	}
	f := newFallback(n)
	for _, c := range comps {
		f.run(g, byComp[c], later, res)
	}
	return res
}

// shortest is the shortest path of templates from s to t, both included,
// the uses taken in document order, when it holds at most cycleShown-1
// uses and the search for it stays within the budget; else nil. It
// searches t's component only, which holds every path from s to t.
func (g *cycleGraph) shortest(s, t int) []int {
	if s == t {
		return []int{t}
	}
	if g.budget <= 0 {
		return nil
	}
	c := g.comp[t]
	g.epoch++
	g.seen[s], g.from[s] = g.epoch, -1
	level := []int{s}
	for depth := 0; len(level) > 0 && depth < cycleShown-1; depth++ {
		var next []int
		for _, x := range level {
			for _, y := range g.adj[x] {
				if g.budget--; g.budget < 0 {
					return nil // cut off
				}
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
				next = append(next, y)
			}
		}
		level = next
	}
	return nil
}

// fallback names, in time linear in a component (up to a logarithm), one
// simple cycle through each of its uses (the F2 review). With r the
// component's first template, an out-tree of shortest paths from r and an
// in-tree of shortest paths to r, the cycle of the use from A to B is
// A → B, up the in-tree from B to x, then down the out-tree from x to A,
// where x is the deepest template on the out-tree path from r to A that is
// an in-tree ancestor of B (r always is). It is simple: the in-tree part
// holds in-tree ancestors of B only, and the out-tree part after x none.
// x is found offline: a walk of the out-tree keeps its path in a segment
// tree over the in-tree's Euler intervals, each node holding the path's
// templates whose interval covers it, deepest on top.
type fallback struct {
	parentIn, depthIn, tin, tout []int
	depthOut                     []int
	kidsIn, kidsOut, radj        [][]int
	mark                         []int
	epoch                        int
}

func newFallback(n int) *fallback {
	f := &fallback{
		parentIn: make([]int, n), depthIn: make([]int, n), tin: make([]int, n), tout: make([]int, n),
		depthOut: make([]int, n), kidsIn: make([][]int, n), kidsOut: make([][]int, n), radj: make([][]int, n),
		mark: make([]int, n),
	}
	return f
}

// run names the uses later[A] of the templates of one component, members
// in document order.
func (f *fallback) run(g *cycleGraph, members []int, later [][]int, res [][]*cycleResult) {
	c := g.comp[members[0]]
	r := members[0]
	for _, v := range members {
		f.kidsIn[v], f.kidsOut[v], f.radj[v] = f.kidsIn[v][:0], f.kidsOut[v][:0], f.radj[v][:0]
	}
	for _, u := range members {
		for _, y := range g.adj[u] {
			if y >= 0 && g.comp[y] == c {
				f.radj[y] = append(f.radj[y], u)
			}
		}
	}
	// The out-tree: shortest paths from r, the uses in document order.
	f.epoch++
	f.mark[r], f.depthOut[r] = f.epoch, 0
	queue := []int{r}
	for k := 0; k < len(queue); k++ {
		x := queue[k]
		for _, y := range g.adj[x] {
			if y < 0 || g.comp[y] != c || f.mark[y] == f.epoch {
				continue
			}
			f.mark[y], f.depthOut[y] = f.epoch, f.depthOut[x]+1
			f.kidsOut[x] = append(f.kidsOut[x], y)
			queue = append(queue, y)
		}
	}
	// The in-tree: shortest paths to r, and its Euler intervals.
	f.epoch++
	f.mark[r], f.depthIn[r], f.parentIn[r] = f.epoch, 0, -1
	queue = append(queue[:0], r)
	for k := 0; k < len(queue); k++ {
		x := queue[k]
		for _, u := range f.radj[x] {
			if f.mark[u] == f.epoch {
				continue
			}
			f.mark[u], f.depthIn[u], f.parentIn[u] = f.epoch, f.depthIn[x]+1, x
			f.kidsIn[x] = append(f.kidsIn[x], u)
			queue = append(queue, u)
		}
	}
	type frame struct{ v, next int }
	counter := 0
	walk := []frame{{v: r}}
	f.tin[r] = counter
	counter++
	for len(walk) > 0 {
		top := &walk[len(walk)-1]
		if top.next < len(f.kidsIn[top.v]) {
			u := f.kidsIn[top.v][top.next]
			top.next++
			f.tin[u] = counter
			counter++
			walk = append(walk, frame{v: u})
			continue
		}
		f.tout[top.v] = counter
		walk = walk[:len(walk)-1]
	}
	// The walk of the out-tree, its path in the segment tree.
	m := len(members)
	seg := make([][]int, 2*m)
	cover := func(v int, push bool) {
		for l, h := f.tin[v]+m, f.tout[v]+m; l < h; l, h = l>>1, h>>1 {
			if l&1 == 1 {
				if push {
					seg[l] = append(seg[l], v)
				} else {
					seg[l] = seg[l][:len(seg[l])-1]
				}
				l++
			}
			if h&1 == 1 {
				h--
				if push {
					seg[h] = append(seg[h], v)
				} else {
					seg[h] = seg[h][:len(seg[h])-1]
				}
			}
		}
	}
	var path []int
	walk = append(walk[:0], frame{v: r})
	path = append(path, r)
	cover(r, true)
	f.name(g, r, path, seg, later, res)
	for len(walk) > 0 {
		top := &walk[len(walk)-1]
		if top.next < len(f.kidsOut[top.v]) {
			u := f.kidsOut[top.v][top.next]
			top.next++
			walk = append(walk, frame{v: u})
			path = append(path, u)
			cover(u, true)
			f.name(g, u, path, seg, later, res)
			continue
		}
		cover(top.v, false)
		path = path[:len(path)-1]
		walk = walk[:len(walk)-1]
	}
}

// name names the uses later[a] of a, the walk of the out-tree at a (path
// is r … a).
func (f *fallback) name(g *cycleGraph, a int, path []int, seg [][]int, later [][]int, res [][]*cycleResult) {
	m := len(seg) / 2
	for _, j := range later[a] {
		b := g.adj[a][j]
		x := -1
		for i := f.tin[b] + m; i >= 1; i >>= 1 {
			if s := seg[i]; len(s) > 0 {
				if top := s[len(s)-1]; x < 0 || f.depthOut[top] > f.depthOut[x] {
					x = top
				}
			}
		}
		length := 1 + (f.depthIn[b] - f.depthIn[x]) + (f.depthOut[a] - f.depthOut[x])
		limit := cycleShown
		if g.full {
			limit = length
		}
		ids := []int{a}
		for y := b; len(ids) < limit && y != a; y = f.parentIn[y] {
			ids = append(ids, y)
			if y == x {
				break
			}
		}
		if x != a {
			for d := f.depthOut[x] + 1; d < f.depthOut[a] && len(ids) < limit; d++ {
				ids = append(ids, path[d])
			}
		}
		r := &cycleResult{ids: ids[:min(len(ids), cycleShown)], more: max(0, length-cycleShown)}
		if g.full {
			r.all = ids
		}
		res[a][j] = r
	}
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
