package parse

import (
	"math/rand"
	"slices"
	"testing"
)

// The cycle of every use that closes one (V019, the F2 review), checked
// against the graph itself on random graphs of templates: a simple cycle
// through that use, from the use's own template, whose first templates and
// count are what the message names; and, while the search budget lasts,
// the shortest one (the uses in document order) whenever it holds at most
// cycleShown templates.

// randomGraph is one of the shapes the fuzz draws: dense or sparse
// random, rings with repeated uses and chords, rings of rings, and
// complete layers.
func randomGraph(rng *rand.Rand) [][]int {
	n := 1 + rng.Intn(40)
	adj := make([][]int, n)
	add := func(a, b int) { adj[a] = append(adj[a], b) }
	switch rng.Intn(5) {
	case 0:
		p := []float64{0.02, 0.05, 0.1, 0.3}[rng.Intn(4)]
		for i := range n {
			for j := range n {
				if rng.Float64() < p {
					add(i, j)
				}
			}
		}
	case 1:
		for i := range n {
			add(i, (i+1)%n)
			if rng.Intn(5) == 0 {
				add(i, (i+1)%n)
			}
		}
	case 2:
		for i := range n {
			add(i, (i+1)%n)
		}
		for range 1 + rng.Intn(4) {
			a := rng.Intn(n)
			adj[a] = slices.Insert(adj[a], rng.Intn(len(adj[a])+1), rng.Intn(n))
		}
	case 3:
		k := 1 + rng.Intn(5)
		for i := range n {
			if rng.Intn(2) == 0 {
				add(i, (i+k)%n)
			} else {
				add(i, (i+1)%n)
			}
		}
	default:
		layers, width := 2+rng.Intn(10), 1+rng.Intn(4)
		n = layers * width
		adj = make([][]int, n)
		for l := range layers {
			for a := range width {
				for b := range width {
					if rng.Intn(4) > 0 {
						adj[l*width+a] = append(adj[l*width+a], (l+1)%layers*width+b)
					}
				}
			}
		}
	}
	for i := range adj {
		if rng.Intn(8) == 0 {
			adj[i] = slices.Insert(adj[i], rng.Intn(len(adj[i])+1), -1) // a use naming no template
		}
	}
	// The templates in a random document order.
	perm := rng.Perm(len(adj))
	out := make([][]int, len(adj))
	for i, row := range adj {
		r := make([]int, len(row))
		for j, s := range row {
			r[j] = -1
			if s >= 0 {
				r[j] = perm[s]
			}
		}
		out[perm[i]] = r
	}
	return out
}

// refShortest is the old pathTo over the whole graph: the shortest path
// from s to t, both included, the uses in document order; nil if none.
func refShortest(adj [][]int, s, t int) []int {
	if s == t {
		return []int{t}
	}
	from := map[int]int{s: -1}
	queue := []int{s}
	for len(queue) > 0 {
		x := queue[0]
		queue = queue[1:]
		for _, y := range adj[x] {
			if y < 0 {
				continue
			}
			if _, seen := from[y]; seen {
				continue
			}
			from[y] = x
			if y == t {
				var path []int
				for p := t; p >= 0; p = from[p] {
					path = append(path, p)
				}
				slices.Reverse(path)
				return path
			}
			queue = append(queue, y)
		}
	}
	return nil
}

func TestCycleOfEachUseFuzz(t *testing.T) {
	rng := rand.New(rand.NewSource(20261019))
	var uses, exact, long, cut int
	for round := range 3000 {
		adj := randomGraph(rng)
		for _, budget := range []int{1 << 60, 0, 25, 300} {
			g := newCycleGraph(adj, budget)
			g.full = true
			res := g.cycles()
			for a := range adj {
				for j, b := range adj[a] {
					ref := []int(nil)
					if b >= 0 {
						ref = refShortest(adj, b, a)
					}
					r := res[a][j]
					if (r != nil) != (ref != nil) {
						t.Fatalf("round %d budget %d: use %d of %d (to %d): cycle %v, want one: %v", round, budget, j, a, b, r, ref != nil)
					}
					if r == nil {
						continue
					}
					uses++
					all := r.all
					if all[0] != a || (len(all) > 1 && all[1] != b) || (len(all) == 1 && b != a) {
						t.Fatalf("round %d: use %d→%d: cycle %v does not start with the use", round, a, b, all)
					}
					seen := map[int]bool{}
					for k, x := range all {
						if seen[x] {
							t.Fatalf("round %d: use %d→%d: cycle %v repeats %d", round, a, b, all, x)
						}
						seen[x] = true
						if next := all[(k+1)%len(all)]; !slices.Contains(adj[x], next) {
							t.Fatalf("round %d: use %d→%d: cycle %v: %d does not use %d", round, a, b, all, x, next)
						}
					}
					if !slices.Equal(r.ids, all[:min(len(all), cycleShown)]) || r.more != max(0, len(all)-cycleShown) {
						t.Fatalf("round %d: use %d→%d: named %v + %d more, cycle %v", round, a, b, r.ids, r.more, all)
					}
					if len(all) > cycleShown {
						long++
					}
					if budget == 1<<60 && len(ref) <= cycleShown {
						exact++
						if want := append([]int{a}, ref[:len(ref)-1]...); !slices.Equal(all, want) {
							t.Fatalf("round %d: use %d→%d: cycle %v, want the shortest %v", round, a, b, all, want)
						}
					}
					if budget < 1<<60 && len(ref) <= cycleShown && len(all) > len(ref) {
						cut++
					}
				}
			}
		}
	}
	t.Logf("%d uses on cycles checked: %d shortest within %d, %d longer than %d, %d named by the fallback past a spent budget", uses, exact, cycleShown, long, cycleShown, cut)
	if exact == 0 || long == 0 || cut == 0 {
		t.Errorf("the fuzz misses a case")
	}
}
