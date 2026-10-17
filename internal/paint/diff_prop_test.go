package paint

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/layout"
	"github.com/abdul-hamid-achik/tuimark/internal/uniwidth"
)

// termModel is an ideal terminal for TestDiffConvergesRandom: a grid of
// clusters and a cursor, understanding exactly the sequences Diff emits
// (CUP, CHA, ECH, ED 2; SGR and mode changes are ignored). A wide cluster
// covers its lead cell and the next one, as uniwidth measures it.
type termModel struct {
	w, h   int
	screen [][]string
	x, y   int
}

func (m *termModel) put(s string) {
	uniwidth.Each(s, func(c string, cw int) bool {
		if cw == 0 || m.x >= m.w {
			return true
		}
		m.screen[m.y][m.x] = c
		if cw == 2 && m.x+1 < m.w {
			m.screen[m.y][m.x+1] = ""
		}
		m.x += cw
		return true
	})
}

func replayDiff(m *termModel, out string) error {
	i := 0
	for i < len(out) {
		if out[i] != 0x1b {
			j := i
			for j < len(out) && out[j] != 0x1b {
				j++
			}
			m.put(out[i:j])
			i = j
			continue
		}
		if i+1 >= len(out) || out[i+1] != '[' {
			return fmt.Errorf("unexpected escape at %d", i)
		}
		j := i + 2
		for j < len(out) && !(out[j] >= 0x40 && out[j] <= 0x7e) {
			j++
		}
		params, final := out[i+2:j], out[j]
		num := func(def int) int {
			n, err := strconv.Atoi(strings.TrimPrefix(params, "?"))
			if err != nil {
				return def
			}
			return n
		}
		switch final {
		case 'H':
			p := strings.Split(params, ";")
			r, _ := strconv.Atoi(p[0])
			c, _ := strconv.Atoi(p[1])
			m.y, m.x = r-1, c-1
		case 'G':
			m.x = num(1) - 1
		case 'X':
			for k := 0; k < num(1) && m.x+k < m.w; k++ {
				m.screen[m.y][m.x+k] = " "
			}
		case 'J':
			for y := range m.screen {
				for x := range m.screen[y] {
					m.screen[y][x] = " "
				}
			}
		case 'm', 'h', 'l':
		default:
			return fmt.Errorf("unknown CSI %q", out[i:j+1])
		}
		i = j + 1
	}
	return nil
}

func gridScreen(g *Grid) [][]string {
	s := make([][]string, g.H)
	for y := 0; y < g.H; y++ {
		s[y] = make([]string, g.W)
		for x := 0; x < g.W; x++ {
			if c := g.At(x, y); c.Cont {
				s[y][x] = ""
			} else {
				s[y][x] = c.Grapheme()
			}
		}
	}
	return s
}

var propClusters = []struct {
	ch  rune
	ext string
}{{' ', ""}, {'a', ""}, {'中', ""}, {'e', "́"}, {0x1F1FA, "\U0001F1F8"}, {0x1F44D, "\U0001F3FD"}, {0x1F468, "‍\U0001F469‍\U0001F467"}, {'b', ""}}

func randClusterGrid(rng *rand.Rand, w, h int) *Grid {
	g := NewGrid(w, h)
	p := &painter{g: g}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			k := propClusters[rng.Intn(len(propClusters))]
			wd := uniwidth.ClusterWidth(string(k.ch) + k.ext)
			if wd == 2 && x+1 >= w {
				continue
			}
			p.put(layout.Rect{W: w, H: h}, x, y, k.ch, k.ext, wd, cellStyle{})
		}
	}
	p.settle()
	p.unjoin()
	return g
}

// TestDiffConvergesRandom replays DiffWith(prev, next) on a terminal that
// shows prev and checks that it then shows next, for random frames of
// ASCII, CJK, combining, regional-indicator, skin-tone, and ZWJ clusters,
// with and without CHA (SPEC §26.5). It pins the property every frame
// diff relies on: only changed units are written, and writing them is
// enough.
func TestDiffConvergesRandom(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	fails := 0
	for iter := 0; iter < 20000 && fails < 3; iter++ {
		w, h := 1+rng.Intn(10), 1+rng.Intn(3)
		prev, next := randClusterGrid(rng, w, h), randClusterGrid(rng, w, h)
		for _, o := range []Options{{}, {NoCHA: true}} {
			d := DiffWith(prev, next, o)
			m := &termModel{w: w, h: h, screen: gridScreen(prev)}
			if err := replayDiff(m, d); err != nil {
				t.Fatalf("parse: %v", err)
			}
			want := gridScreen(next)
			bad := false
			for y := 0; y < h && !bad; y++ {
				for x := 0; x < w && !bad; x++ {
					if m.screen[y][x] != want[y][x] {
						bad = true
						fails++
						t.Errorf("iter %d opts %+v cell (%d,%d): got %q want %q\nprev=%q\nnext=%q\ndiff=%q", iter, o, x, y, m.screen[y][x], want[y][x], gridScreen(prev), gridScreen(next), d)
					}
				}
			}
		}
	}
}
