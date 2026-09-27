package paint

import (
	"strconv"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/layout"
	"github.com/abdul-hamid-achik/tuimark/internal/uniwidth"
)

// cont marks, in fakeTerm, the second column of a glyph the terminal drew
// two columns wide.
const cont = "\x00"

// fakeTerm is a scripted terminal (SPEC v0.2 §21 test 34 style) that may
// measure some clusters differently from §11.5.1: width gives its own
// width for a cluster, 0 meaning "as §11.5.1". It understands what Diff
// and Full write: CUP, CHA, ED 2, ECH, SGR (ignored), DECSET/DECRST 7
// (autowrap) and 25, LF, and text, which it segments afresh after every
// escape sequence. Like xterm, writing over either half of a wide glyph
// blanks the other half, and with autowrap on a glyph that does not fit
// before the right margin wraps to the next line, scrolling at the bottom.
type fakeTerm struct {
	w, h     int
	cells    []string
	x, y     int
	pending  bool // the cursor is past the last column (autowrap pending)
	autowrap bool
	width    map[string]int
}

func newFakeTerm(w, h int, width map[string]int) *fakeTerm {
	t := &fakeTerm{w: w, h: h, cells: make([]string, w*h), autowrap: true, width: width}
	t.clear()
	return t
}

func (t *fakeTerm) clear() {
	for i := range t.cells {
		t.cells[i] = " "
	}
}

func (t *fakeTerm) at(x, y int) string { return t.cells[y*t.w+x] }

// blank clears column x of the cursor row, and the other half of the wide
// glyph it belongs to.
func (t *fakeTerm) blank(x int) {
	row := t.cells[t.y*t.w : (t.y+1)*t.w]
	switch {
	case row[x] == cont:
		row[x-1] = " "
	case x+1 < t.w && row[x+1] == cont:
		row[x+1] = " "
	}
	row[x] = " "
}

func (t *fakeTerm) scroll() {
	copy(t.cells, t.cells[t.w:])
	for i := len(t.cells) - t.w; i < len(t.cells); i++ {
		t.cells[i] = " "
	}
}

func (t *fakeTerm) put(c string) {
	tw := t.width[c]
	if tw == 0 {
		tw = uniwidth.ClusterWidth(c)
	}
	if tw == 0 {
		return
	}
	if t.pending || t.x+tw > t.w {
		if !t.autowrap {
			// DECAWM off: a narrow glyph overwrites the last column; a
			// wide one that does not fit is dropped (as tmux does).
			if tw > 1 {
				return
			}
			t.x, t.pending = t.w-1, false
		} else {
			t.x, t.pending = 0, false
			if t.y++; t.y == t.h {
				t.y = t.h - 1
				t.scroll()
			}
		}
	}
	for i := 0; i < tw; i++ {
		t.blank(t.x + i)
	}
	t.cells[t.y*t.w+t.x] = c
	if tw == 2 {
		t.cells[t.y*t.w+t.x+1] = cont
	}
	if t.x += tw; t.x >= t.w {
		t.x, t.pending = t.w-1, true
	}
}

func (t *fakeTerm) write(tb testing.TB, out string) {
	tb.Helper()
	for out != "" {
		switch {
		case strings.HasPrefix(out, "\x1b["):
			end := strings.IndexFunc(out[2:], func(r rune) bool { return r >= 0x40 && r <= 0x7e })
			if end < 0 {
				tb.Fatalf("unterminated CSI in %q", out)
			}
			params, final := out[2:2+end], out[2+end]
			out = out[3+end:]
			n := func(i, def int) int {
				f := strings.Split(strings.TrimPrefix(params, "?"), ";")
				if i < len(f) && f[i] != "" {
					v, err := strconv.Atoi(f[i])
					if err == nil {
						return v
					}
				}
				return def
			}
			switch final {
			case 'H':
				t.y, t.x, t.pending = n(0, 1)-1, n(1, 1)-1, false
			case 'G':
				t.x, t.pending = n(0, 1)-1, false
			case 'J':
				t.clear()
			case 'X':
				for i := t.x; i < min(t.w, t.x+n(0, 1)); i++ {
					t.blank(i)
				}
			case 'h', 'l':
				if params == "?7" {
					t.autowrap = final == 'h'
				}
			}
		case out[0] == '\n':
			out = out[1:]
			t.x, t.pending = 0, false // ONLCR: the tty writes CR LF
			if t.y++; t.y == t.h {
				t.y = t.h - 1
				t.scroll()
			}
		default:
			end := strings.IndexAny(out, "\x1b\n")
			if end < 0 {
				end = len(out)
			}
			for _, c := range uniwidth.Split(out[:end]) {
				t.put(c)
			}
			out = out[end:]
		}
	}
}

// damage returns the cells of g that the terminal does not show as g has
// them, leaving out the columns of disputed clusters (those the terminal
// measures differently). A disputed cluster's columns may show its own
// glyph, the half of a glyph, or a blank, never a stale glyph.
func (t *fakeTerm) damage(g *Grid) []string {
	var bad []string
	for y := 0; y < g.H; y++ {
		for x := 0; x < g.W; x++ {
			c := g.At(x, y)
			if c.Cont {
				continue
			}
			s, w := c.Grapheme(), c.Width()
			if tw := t.width[s]; tw != 0 && tw != w {
				for i := x; i < x+w; i++ { // its own columns; a spill is checked as the next cells
					if got := t.at(i, y); got != s && got != cont && got != " " {
						bad = append(bad, strconv.Itoa(i)+","+strconv.Itoa(y)+" stale "+strconv.Quote(got))
					}
				}
				continue
			}
			if got := t.at(x, y); got != s {
				bad = append(bad, strconv.Itoa(x)+","+strconv.Itoa(y)+" "+strconv.Quote(got)+" want "+strconv.Quote(s))
			}
			if w == 2 && t.at(x+1, y) != cont {
				bad = append(bad, strconv.Itoa(x+1)+","+strconv.Itoa(y)+" not a continuation")
			}
		}
	}
	return bad
}

func paintRow(w int, s string) *Grid {
	p := &painter{g: NewGrid(w, 1)}
	p.text(all, 0, 0, s, cellStyle{})
	p.settle()
	p.unjoin()
	return p.g
}

// §26.5: a terminal that measures a complex cluster differently damages at
// most that cluster's own cells, in diff frames as in full repaints: the
// unit after a rewritten complex cluster is rewritten too, after the CHA,
// and the cluster's columns are erased first, so a continuation column
// never shows a stale glyph.
func TestDiffKeepsDamageInsideTheCluster(t *testing.T) {
	keycap1, keycap2 := "1\uFE0F\u20E3", "2\uFE0F\u20E3" // W = 1, complex
	cases := []struct {
		name     string
		w        int
		from, to string
		width    map[string]int
	}{
		{"drawn wider: the next cell survives", 6, "k" + keycap1 + "z|", "k" + keycap2 + "z|", map[string]int{keycap1: 2, keycap2: 2}},
		{"drawn wider: the border survives", 4, "ab" + keycap1 + "│", "ab" + keycap2 + "│", map[string]int{keycap1: 2, keycap2: 2}},
		{"drawn wider before a wide cluster", 6, "k" + keycap1 + "微|", "k" + keycap2 + "微|", map[string]int{keycap1: 2, keycap2: 2}},
		{"drawn narrower: no stale continuation", 5, "xyz|", "⸺z|", map[string]int{"⸺": 1}},
		{"drawn narrower after a change elsewhere", 6, "⸺ab|", "⸺xb|", map[string]int{"⸺": 1}},
		{"a chain of disputed clusters", 7, "a" + keycap1 + keycap1 + "b|", "a" + keycap2 + keycap2 + "b|", map[string]int{keycap1: 2, keycap2: 2}},
	}
	for _, c := range cases {
		from, to := paintRow(c.w, c.from), paintRow(c.w, c.to)
		term := newFakeTerm(c.w, 1, c.width)
		term.write(t, Diff(nil, from))
		if bad := term.damage(from); len(bad) != 0 {
			t.Errorf("%s: full frame damaged %v", c.name, bad)
		}
		d := Diff(from, to)
		term.write(t, d)
		if bad := term.damage(to); len(bad) != 0 {
			t.Errorf("%s: diff frame %q damaged %v (screen %q)", c.name, d, bad, term.cells)
		}
		// And back again.
		term.write(t, Diff(to, from))
		if bad := term.damage(from); len(bad) != 0 {
			t.Errorf("%s: second diff damaged %v (screen %q)", c.name, bad, term.cells)
		}
	}
	// With mode 2027 on (NoCHA) the terminal agrees with the layout: a diff
	// rewrites only what changed.
	a, b := paintRow(6, "k"+keycap1+"z|"), paintRow(6, "k"+keycap2+"z|")
	if got := DiffWith(a, b, Options{NoCHA: true}); got != "\x1b[1;2H\x1b[0;39;49m"+keycap2+"\x1b[0m\x1b[?25l" {
		t.Errorf("NoCHA diff = %q", got)
	}
	// With CHA: the cluster's column is erased, the cluster written, the
	// cursor re-positioned, and the unchanged z after it written again.
	if got := Diff(a, b); got != "\x1b[1;2H\x1b[0;39;49m\x1b[1X"+keycap2+"\x1b[3Gz\x1b[0m\x1b[?25l" {
		t.Errorf("CHA diff = %q", got)
	}
}

// §26.1, §26.5: a complex cluster in the last column that the terminal
// draws wider must not wrap the row or scroll the screen. Full (preview)
// writes its grid with autowrap off and turns it back on after; Run turns
// it off for the whole session (AutowrapOff, AutowrapOn).
func TestLastColumnClusterDoesNotScroll(t *testing.T) {
	emoji := "\U0001F600\uFE0E" // W = 1, complex; many terminals draw it 2 wide
	width := map[string]int{emoji: 2}
	root := func(last string) *layout.Box {
		return bx("col", "", tx("top line", ""), bx("row", "height: 1", tx("abc", ""), bx("spacer", ""), tx(last, "")))
	}
	g := render(t, root(emoji), 10, 2)
	checkGrid(t, g)
	if got := g.Lines(); got[0] != "top line  " || got[1] != "abc      "+emoji {
		t.Fatalf("grid = %q", got)
	}
	full := Full(g)
	if !strings.HasPrefix(full, AutowrapOff) || !strings.HasSuffix(full, AutowrapOn) {
		t.Errorf("Full must turn autowrap off around the grid: %q", full)
	}
	term := newFakeTerm(10, 3, width)
	term.write(t, full)
	if bad := term.damage(g); len(bad) != 0 || !term.autowrap {
		t.Errorf("preview grid damaged %v (autowrap back on: %v): %q", bad, term.autowrap, term.cells)
	}
	// Run's frames, with autowrap off as Run's enter sequence sets it: the
	// first frame and a diff that changes the last column keep the screen.
	term = newFakeTerm(10, 2, width)
	term.write(t, AutowrapOff)
	term.write(t, Diff(nil, g))
	if bad := term.damage(g); len(bad) != 0 {
		t.Errorf("first frame damaged %v: %q", bad, term.cells)
	}
	h := render(t, root("x"), 10, 2)
	term.write(t, Diff(g, h))
	term.write(t, Diff(h, g))
	if bad := term.damage(g); len(bad) != 0 {
		t.Errorf("diff frames damaged %v: %q", bad, term.cells)
	}
	// Without it, the same frame scrolls the whole screen: why Run needs it.
	term = newFakeTerm(10, 2, width)
	term.write(t, Diff(nil, g))
	if term.at(0, 0) == "t" {
		t.Errorf("the fake terminal should scroll with autowrap on: %q", term.cells)
	}
}
