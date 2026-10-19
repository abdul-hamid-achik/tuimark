package main

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

// ---- states to look at ---------------------------------------------------

// boards the layout tests run on: the presets and a few custom sizes,
// including the smallest and the largest.
var layoutBoards = []board{
	beginner,
	{"intermediate", 16, 16, 40},
	{"expert", 30, 16, 99},
	{"custom", 30, 9, 50},
	{"custom", 12, 10, 20},
}

// arrange puts the rig in a look: "fresh", "started", "flagmode", "paused",
// "lost", "won", "notice", or "cornered" (the cursor in a corner, a marked
// game).
func (r *rig) arrange(kind string) {
	r.t.Helper()
	g := r.a.g
	start := func() { r.play(r.click(g.rows/2, g.cols/2)) }
	switch kind {
	case "fresh":
	case "started":
		start()
		r.advance(1500 * time.Millisecond)
	case "flagmode":
		start()
		r.play("f", "g")
	case "paused":
		start()
		r.play("p")
	case "lost":
		start()
		r.a.g.reveal(firstMine(r.a.g), r.a.rng)
		r.sync()
	case "won":
		start()
		for i := range g.mine {
			if !g.mine[i] {
				g.reveal(i, r.a.rng)
			}
		}
		r.a.after()
		r.sync()
	case "notice":
		_ = r.a.onError(nil, fmt.Errorf("%s", strings.Repeat("something went wrong ", 12)))
	case "cornered":
		start()
		for i := 0; i < 40; i++ {
			r.play("up", "left")
		}
		r.play("f", "enter", "f")
	default:
		r.t.Fatalf("no look %q", kind)
	}
}

func (r *rig) sync() {
	r.t.Helper()
	if err := r.a.sync(); err != nil {
		r.t.Fatal(err)
	}
}

var layoutLooks = []string{"fresh", "started", "flagmode", "paused", "lost", "won", "notice", "cornered"}

// The view is clean (ok, no diagnostics) in every look, on every board, in
// both themes, with the help open or not, at every size that matters
// including the ones too small for the board.
func TestNoDiagnosticsAnywhere(t *testing.T) {
	sizes := [][2]int{{40, 14}, {60, 20}, {80, 24}, {100, 30}, {120, 30}}
	// The help is the same modal over any look, so it is opened over three.
	helpOver := map[string]bool{"fresh": true, "started": true, "lost": true}
	for _, b := range layoutBoards[:4] { // the custom 12x10 is covered by the edge tests
		for _, look := range layoutLooks {
			r := newRigOn(t, b, 5)
			r.arrange(look)
			for _, help := range []bool{false, true} {
				if help && !helpOver[look] {
					continue
				}
				if help {
					r.play("?")
				}
				for _, theme := range []string{"dark", "light"} {
					if err := r.a.ui.Set("@theme", theme); err != nil {
						t.Fatal(err)
					}
					for _, sz := range sizes {
						res := r.playAt(sz[0], sz[1])
						if !res.Dump.OK || len(res.Dump.Errors) != 0 {
							t.Errorf("%s %dx%d %s %s help=%v %dx%d: %v", b.level, b.cols, b.rows, look, theme, help, sz[0], sz[1], res.Dump.Errors)
						}
						if help && focusOf(res) != "help-body" {
							t.Errorf("%s %s help=%v %dx%d: focus %q", b.level, look, help, sz[0], sz[1], focusOf(res))
						}
					}
				}
			}
		}
	}
}

// ---- the help modal ------------------------------------------------------

// The help is clean at every size from 40x14 up and keeps its close button
// on the screen, so a small terminal can always close it.
func TestHelpModalIsCleanAndClosableAtEverySize(t *testing.T) {
	for _, theme := range []string{"dark", "light"} {
		for _, sz := range [][2]int{{40, 14}, {40, 12}, {40, 10}, {30, 9}, {44, 15}, {50, 17}, {60, 18}, {60, 19}, {60, 20}, {80, 24}, {120, 30}, {200, 50}} {
			r := newRig(t)
			if err := r.a.ui.Set("@theme", theme); err != nil {
				t.Fatal(err)
			}
			res := r.playAt(sz[0], sz[1], "?")
			if len(res.Dump.Errors) != 0 {
				t.Errorf("%s %dx%d: %v", theme, sz[0], sz[1], res.Dump.Errors)
			}
			var btn bool
			for _, n := range res.Dump.Nodes {
				if n.ID == "help-ok" && n.Y+n.H <= sz[1] && n.X+n.W <= sz[0] && n.H > 0 {
					btn = true
				}
			}
			if !btn || !strings.Contains(gridOf(res), "close") {
				t.Errorf("%s %dx%d: no close button on the screen:\n%s", theme, sz[0], sz[1], gridOf(res))
			}
		}
	}
}

// The help lists every key of the game.
func TestHelpMentionsEveryKey(t *testing.T) {
	r := newRig(t)
	g := gridOf(r.playAt(80, 30, "?"))
	for _, want := range []string{"click", "enter space", "arrows hjkl", "flag mode", "new game", "1 2 3", "pause", "mouse", "? esc enter", "q ctrl+c", "first reveal is never a mine", "Chord"} {
		if !strings.Contains(g, want) {
			t.Errorf("the help has no %q:\n%s", want, g)
		}
	}
}

// ---- too small, and where the panel goes --------------------------------

func needFor(b board) (cols, rows int) {
	cols = 2*b.cols + 2
	if cols < 36 {
		cols = 36
	}
	return cols, b.rows + 4
}

// onScreen reports whether every cell of the board is a node wholly inside
// a cols x rows screen.
func onScreen(res *tuimark.PlayResult, b board, cols, rows int) bool {
	seen := 0
	for _, n := range res.Dump.Nodes {
		if len(n.ID) == 5 && n.ID[0] == 'c' && n.W == 2 && n.H == 1 {
			seen++
			if n.X < 0 || n.Y < 0 || n.X+n.W > cols || n.Y+n.H > rows {
				return false
			}
		}
	}
	return seen == b.cols*b.rows
}

func noCells(res *tuimark.PlayResult) bool {
	for _, n := range res.Dump.Nodes {
		if len(n.ID) == 5 && n.ID[0] == 'c' && n.ID != "cell" {
			return false
		}
	}
	return true
}

// A terminal too small for the board says so plainly, with the size it
// needs, instead of cutting the board; the board is whole at the size it
// needs and above, and nothing is reported either way.
func TestTooSmallSaysSoAtTheEdges(t *testing.T) {
	for _, b := range layoutBoards {
		nc, nr := needFor(b)
		want := fmt.Sprintf("this board needs %d columns by %d rows", nc, nr)
		for _, theme := range []string{"dark", "light"} {
			for _, look := range []string{"fresh", "started"} {
				r := newRigOn(t, b, 2)
				r.arrange(look)
				if err := r.a.ui.Set("@theme", theme); err != nil {
					t.Fatal(err)
				}
				for _, c := range []struct {
					cols, rows int
					small      bool
				}{
					{nc - 1, nr + 10, true}, {nc, nr + 10, false},
					{nc + 40, nr - 1, true}, {nc + 40, nr, false},
					{nc - 1, nr - 1, true}, {nc, nr, false},
					{nc + 100, nr + 20, false},
				} {
					res := r.playAt(c.cols, c.rows)
					name := fmt.Sprintf("%s %dx%d %s %s at %dx%d", b.level, b.cols, b.rows, look, theme, c.cols, c.rows)
					if !res.Dump.OK || len(res.Dump.Errors) != 0 {
						t.Errorf("%s: %v", name, res.Dump.Errors)
					}
					g := gridOf(res)
					if c.small {
						flat := strings.Join(strings.Fields(g), " ")
						if !strings.Contains(flat, "terminal too small") || !strings.Contains(flat, want) {
							t.Errorf("%s: no notice naming %q:\n%s", name, want, g)
						}
						if !noCells(res) {
							t.Errorf("%s: cells are still laid out", name)
						}
						continue
					}
					if strings.Contains(g, "too small") {
						t.Errorf("%s: says too small though the board fits:\n%s", name, g)
					}
					if !onScreen(res, b, c.cols, c.rows) {
						t.Errorf("%s: the board is not whole on the screen:\n%s", name, g)
					}
					if last := strings.Split(g, "\n")[c.rows-1]; !strings.Contains(last, "help") {
						t.Errorf("%s: the key hints are not on the last row: %q", name, last)
					}
				}
			}
		}
	}
}

// The side panel sits beside the board from need+30 columns; below that
// its counters move into the status line.
func TestPanelBesideTheBoardOrCountersInTheStatusLine(t *testing.T) {
	for _, b := range layoutBoards {
		nc, nr := needFor(b)
		r := newRigOn(t, b, 2)
		r.arrange("started")
		for _, c := range []struct {
			cols int
			side bool
		}{{nc, false}, {nc + sideRoom - 1, false}, {nc + sideRoom, true}, {nc + sideRoom + 25, true}} {
			res := r.playAt(c.cols, nr+2)
			g := gridOf(res)
			top := strings.Split(g, "\n")[0]
			name := fmt.Sprintf("%s %dx%d at %d columns", b.level, b.cols, b.rows, c.cols)
			if c.side != strings.Contains(g, "best times") {
				t.Errorf("%s: side panel shown = %v, want %v:\n%s", name, strings.Contains(g, "best times"), c.side, g)
			}
			if c.side {
				if strings.Contains(top, "mines ") && !strings.Contains(top, "minesweeper") {
					t.Errorf("%s: counters in the status line beside a side panel: %q", name, top)
				}
				if !panelBesideBoard(res) {
					t.Errorf("%s: the panel is not to the right of the board:\n%s", name, g)
				}
				continue
			}
			// The counters are all in the status line: face, state, mines left, time.
			for _, want := range []string{":)", "playing", "mines", "010", "0:01"} {
				if want == "010" {
					want = r.str("mines_label")
				}
				if !strings.Contains(top, want) {
					t.Errorf("%s: the status line %q lacks %q", name, top, want)
				}
			}
			if !onScreen(res, b, c.cols, nr+2) {
				t.Errorf("%s: the board is cut:\n%s", name, g)
			}
		}
	}
}

func panelBesideBoard(res *tuimark.PlayResult) bool {
	var side, board *struct{ x, w int }
	for _, n := range res.Dump.Nodes {
		switch n.ID {
		case "side":
			side = &struct{ x, w int }{n.X, n.W}
		case "board":
			board = &struct{ x, w int }{n.X, n.W}
		}
	}
	return side != nil && board != nil && side.x >= board.x+board.w
}

// The status line keeps the state and the counters on a narrow terminal,
// with the longest notice and with the largest numbers.
func TestStatusLineKeepsTheCountersOnNarrowTerminals(t *testing.T) {
	r := newRig(t)
	r.arrange("started")
	r.a.elapsed = 98*time.Minute + 59*time.Second
	if err := r.a.sync(); err != nil {
		t.Fatal(err)
	}
	r.a.g.flags = 99 // 10 - 99: the widest counter
	r.sync()
	if r.str("mines_label") != "-89" {
		t.Fatalf("mines_label = %s", r.str("mines_label"))
	}
	for cols := 36; cols <= 65; cols++ { // from 66 columns the panel takes the counters
		res := r.playAt(cols, 14)
		top := strings.Split(gridOf(res), "\n")[0]
		for _, want := range []string{"playing", "-89", "98:59"} {
			if !strings.Contains(top, want) {
				t.Fatalf("%d columns: %q lacks %q", cols, top, want)
			}
		}
		if len(res.Dump.Errors) != 0 {
			t.Fatalf("%d columns: %v", cols, res.Dump.Errors)
		}
	}
}

// ---- the board's look ------------------------------------------------------

func hexRGB(s string) (r, g, b float64) {
	var ri, gi, bi int
	fmt.Sscanf(strings.TrimPrefix(s, "#"), "%02x%02x%02x", &ri, &gi, &bi)
	return float64(ri), float64(gi), float64(bi)
}

func luminance(hex string) float64 {
	r, g, b := hexRGB(hex)
	f := func(c float64) float64 {
		c /= 255
		if c <= 0.03928 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	return 0.2126*f(r) + 0.7152*f(g) + 0.0722*f(b)
}

func contrast(a, b string) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// spanAt is the style of the cell at (x, y) of a dump made with styles.
func spanAt(t testing.TB, res *tuimark.PlayResult, x, y int) (fg, bg string, attrs []string) {
	t.Helper()
	for _, s := range res.Dump.Styles[y] {
		if x >= s.X && x < s.X+s.W {
			return s.FG, s.BG, s.A
		}
	}
	t.Fatalf("no style at %d,%d", x, y)
	return
}

func hasAttr(attrs []string, a string) bool {
	for _, x := range attrs {
		if x == a {
			return true
		}
	}
	return false
}

func (r *rig) styled(cols, rows int) *tuimark.PlayResult {
	r.t.Helper()
	res, err := r.a.ui.Play(tuimark.PlayOptions{Cols: cols, Rows: rows, Styles: true})
	if err != nil {
		r.t.Fatal(err)
	}
	return res
}

func (r *rig) cellStyle(res *tuimark.PlayResult, row, col int) (fg, bg string, attrs []string) {
	r.t.Helper()
	id := cellID(row, col)
	for _, n := range res.Dump.Nodes {
		if n.ID == id {
			return spanAt(r.t, res, n.X, n.Y)
		}
	}
	r.t.Fatalf("no node %s", id)
	return
}

// numbersGame is a 16x9 board showing every number from 1 to 8 on an open
// cell, a covered cell, a flag, a question mark, and an empty open cell.
// Block k holds the number k in its middle, with k of its neighbors mines.
func numbersGame() (g *game, centers [9][2]int) {
	rows := make([][]byte, 9)
	for i := range rows {
		rows[i] = []byte(strings.Repeat(".", 16))
	}
	type d struct{ r, c int }
	var ring []d
	for dr := -1; dr <= 1; dr++ {
		for dc := -1; dc <= 1; dc++ {
			if dr != 0 || dc != 0 {
				ring = append(ring, d{dr, dc})
			}
		}
	}
	for k := 1; k <= 8; k++ {
		br, bc := ((k-1)/4)*4+1, ((k-1)%4)*4+1 // block centers: rows 1 and 5, columns 1, 5, 9, 13
		centers[k] = [2]int{br, bc}
		for _, o := range ring[:k] {
			rows[br+o.r][bc+o.c] = '*'
		}
	}
	lines := make([]string, len(rows))
	for i, r := range rows {
		lines[i] = string(r)
	}
	g = fromGrid(lines...)
	for k := 1; k <= 8; k++ {
		i := g.idx(centers[k][0], centers[k][1])
		g.st[i] = stOpen
		g.opened++
	}
	g.st[g.idx(8, 15)] = stOpen // an empty open cell, far from every mine
	g.opened++
	return g, centers
}

func rig16x9(t *testing.T, theme string) (*rig, *game, [9][2]int) {
	t.Helper()
	r := newRigOn(t, beginner, 1)
	g, centers := numbersGame()
	r.a.g, r.a.cur = g, g.idx(8, 0)
	r.a.level = "custom"
	r.sync()
	if err := r.a.ui.Set("@theme", theme); err != nil {
		t.Fatal(err)
	}
	return r, g, centers
}

// The classic number colors, adjusted for both themes: each number is a
// legible color on the open cells, of the family its name says, and
// different from every other.
func TestNumberColorsAreLegibleAndClassic(t *testing.T) {
	families := map[int]func(r, g, b float64) bool{
		1: func(r, g, b float64) bool { return b > r+60 && b > g+20 },              // blue
		2: func(r, g, b float64) bool { return g > r+40 && g > b+40 },              // green
		3: func(r, g, b float64) bool { return r > g+80 && r > b+80 },              // red
		4: func(r, g, b float64) bool { return b > r+20 && b > g+40 },              // navy
		5: func(r, g, b float64) bool { return r > g+30 && r > b+30 },              // maroon
		6: func(r, g, b float64) bool { return g > r+60 && b > r+60 },              // teal
		7: func(r, g, b float64) bool { return max3(r, g, b)-min3(r, g, b) <= 24 }, // black or white
		8: func(r, g, b float64) bool { return max3(r, g, b)-min3(r, g, b) <= 24 }, // gray
	}
	for _, theme := range []string{"dark", "light"} {
		r, _, centers := rig16x9(t, theme)
		res := r.styled(120, 30)
		_, openBG, _ := r.cellStyle(res, 8, 15)
		seen := map[string]int{}
		var c7, c8 float64
		for k := 1; k <= 8; k++ {
			fg, bg, attrs := r.cellStyle(res, centers[k][0], centers[k][1])
			if bg != openBG {
				t.Errorf("%s: %d is on %s, not the open background %s", theme, k, bg, openBG)
			}
			if c := contrast(fg, bg); c < 4.5 {
				t.Errorf("%s: number %d (%s on %s) has contrast %.2f, want 4.5", theme, k, fg, bg, c)
			}
			rr, gg, bb := hexRGB(fg)
			if !families[k](rr, gg, bb) {
				t.Errorf("%s: number %d is %s, not of its classic family", theme, k, fg)
			}
			if !hasAttr(attrs, "bold") {
				t.Errorf("%s: number %d is not bold", theme, k)
			}
			if prev, dup := seen[fg]; dup {
				t.Errorf("%s: numbers %d and %d share %s", theme, prev, k, fg)
			}
			seen[fg] = k
			switch k {
			case 7:
				c7 = contrast(fg, bg)
			case 8:
				c8 = contrast(fg, bg)
			}
		}
		if c7 <= c8 {
			t.Errorf("%s: 7 (%.1f) should stand out more than the gray 8 (%.1f)", theme, c7, c8)
		}
		// 7 is white on dark and black on light, as the classic black adjusted.
		fg7, _, _ := r.cellStyle(res, centers[7][0], centers[7][1])
		if l := luminance(fg7); (theme == "dark") != (l > 0.5) {
			t.Errorf("%s: number 7 is %s", theme, fg7)
		}
	}
}

func max3(a, b, c float64) float64 {
	if b > a {
		a = b
	}
	if c > a {
		a = c
	}
	return a
}

func min3(a, b, c float64) float64 {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}

// Covered cells come in two shades laid out like a checkerboard, so
// neighbors can be told apart; flags and question marks are legible on both;
// and the cursor is a cell of its own color on covered and open cells alike.
func TestCoveredFlaggedAndCursorLook(t *testing.T) {
	for _, theme := range []string{"dark", "light"} {
		r, g, centers := rig16x9(t, theme)
		// (3,11) and (3,13) are on the base shade, (3,12) and (3,14) on the
		// alternate one (row + column odd).
		g.cycle(g.idx(3, 14)) // flag on the alternate shade
		g.cycle(g.idx(3, 11)) // flag on the base shade
		g.cycle(g.idx(3, 13)) // question mark on the base shade
		g.cycle(g.idx(3, 13))
		g.cycle(g.idx(3, 12)) // question mark on the alternate shade
		g.cycle(g.idx(3, 12))
		r.sync()
		res := r.styled(120, 30)
		_, openBG, _ := r.cellStyle(res, 8, 15)
		baseFG, baseBG, _ := r.cellStyle(res, 7, 15) // 7+15 even
		altFG, altBG, _ := r.cellStyle(res, 7, 14)   // 7+14 odd
		if baseBG == altBG || contrast(baseBG, altBG) < 1.2 {
			t.Errorf("%s: the two covered shades %s and %s are too alike", theme, baseBG, altBG)
		}
		if contrast(baseBG, openBG) < 1.3 || contrast(altBG, openBG) < 1.15 {
			t.Errorf("%s: covered %s/%s and open %s are too alike", theme, baseBG, altBG, openBG)
		}
		if baseFG == baseBG || altFG == altBG {
			t.Errorf("%s: the covered texture is invisible", theme)
		}
		flagFG := map[string]string{}
		for name, c := range map[string][2]int{"flag alt": {3, 14}, "flag base": {3, 11}, "q base": {3, 13}, "q alt": {3, 12}} {
			fg, bg, attrs := r.cellStyle(res, c[0], c[1])
			want := baseBG
			if strings.HasSuffix(name, "alt") {
				want = altBG
			}
			if bg != want || contrast(fg, bg) < 4.5 || !hasAttr(attrs, "bold") {
				t.Errorf("%s: %s is %s on %s (%v), contrast %.2f, want %s behind it", theme, name, fg, bg, attrs, contrast(fg, bg), want)
			}
			flagFG[name] = fg
		}
		if flagFG["flag alt"] != flagFG["flag base"] || flagFG["q alt"] != flagFG["q base"] || flagFG["flag base"] == flagFG["q base"] {
			t.Errorf("%s: flags and question marks are not told apart by color: %v", theme, flagFG)
		}
		// The cursor: on a covered cell of the base shade it is not the
		// covered look, it has its own colors, legible, bold; on an open
		// number and on a covered cell of the other shade it is the same.
		type look struct{ fg, bg string }
		var seen []look
		for _, at := range [][2]int{{8, 0}, {3, 12}, {centers[1][0], centers[1][1]}, {8, 15}, {3, 14}} {
			r.a.cur = g.idx(at[0], at[1])
			r.sync()
			res = r.styled(120, 30)
			fg, bg, attrs := r.cellStyle(res, at[0], at[1])
			if !hasAttr(attrs, "bold") || contrast(fg, bg) < 4.5 {
				t.Errorf("%s: the cursor at %v is %s on %s (%v), contrast %.2f", theme, at, fg, bg, attrs, contrast(fg, bg))
			}
			if bg == baseBG || bg == altBG || bg == openBG || contrast(bg, openBG) < 3 || contrast(bg, baseBG) < 2.5 || contrast(bg, altBG) < 2.5 {
				t.Errorf("%s: the cursor background %s is not clearly its own (open %s, covered %s/%s)", theme, bg, openBG, baseBG, altBG)
			}
			seen = append(seen, look{fg, bg})
			// Nothing else is drawn as the cursor.
			for _, other := range [][2]int{{7, 15}, {7, 14}, {centers[2][0], centers[2][1]}} {
				if other == at {
					continue
				}
				if _, obg, _ := r.cellStyle(res, other[0], other[1]); obg == bg {
					t.Errorf("%s: cursor at %v: cell %v has the cursor's background", theme, at, other)
				}
			}
		}
		for _, s := range seen {
			if s != seen[0] {
				t.Errorf("%s: the cursor looks different on different cells: %v", theme, seen)
				break
			}
		}
	}
}

// The cursor is not drawn once the game is over, and is neither the look of
// the mine that went off nor of any number.
func TestCursorGoesWhenTheGameEnds(t *testing.T) {
	for _, theme := range []string{"dark", "light"} {
		for _, end := range []string{"lost", "won"} {
			r := newRigOn(t, beginner, 5)
			r.arrange("started")
			if err := r.a.ui.Set("@theme", theme); err != nil {
				t.Fatal(err)
			}
			cursor := r.a.cur
			before := r.styled(90, 24)
			cr, cc := r.a.g.pos(cursor)
			_, curBG, _ := r.cellStyle(before, cr, cc)
			r.arrange(end)
			if r.on("b." + cellID(cr, cc) + ".cur") {
				t.Errorf("%s %s: the store still has the cursor on", theme, end)
			}
			for rr := 0; rr < maxRows; rr++ {
				for c := 0; c < maxCols; c++ {
					if r.on("b." + cellID(rr, c) + ".cur") {
						t.Errorf("%s %s: cell %s carries the cursor", theme, end, cellID(rr, c))
					}
				}
			}
			after := r.styled(90, 24)
			for rr := 0; rr < r.a.g.rows; rr++ {
				for c := 0; c < r.a.g.cols; c++ {
					if _, bg, _ := r.cellStyle(after, rr, c); bg == curBG {
						t.Errorf("%s %s: cell %d,%d is still painted as the cursor (%s)", theme, end, rr, c, bg)
					}
				}
			}
			// A new game brings it back, in the middle.
			r.play("n")
			if !r.on("b.c0404.cur") {
				t.Errorf("%s %s: n did not bring the cursor back", theme, end)
			}
		}
	}
}

// While a game is on, the cursor on a red 3 is blue, not the red of the
// mine that goes off (both themes).
func TestCursorIsNotTheExplodedMine(t *testing.T) {
	for _, theme := range []string{"dark", "light"} {
		r, g, centers := rig16x9(t, theme)
		r.a.cur = g.idx(centers[3][0], centers[3][1]) // on a red 3
		r.sync()
		res := r.styled(120, 30)
		_, curBG, _ := r.cellStyle(res, centers[3][0], centers[3][1])
		g.over, g.boom = true, g.idx(centers[1][0]-1, centers[1][1]-1)
		r.sync()
		res = r.styled(120, 30)
		_, boomBG, _ := r.cellStyle(res, centers[1][0]-1, centers[1][1]-1)
		cr, cg, cb := hexRGB(curBG)
		br, bg, bb := hexRGB(boomBG)
		if !(cb > cr+40 && cb > cg) || !(br > bb+40 && br > bg+40) {
			t.Errorf("%s: cursor %s should be blue and the exploded mine %s red", theme, curBG, boomBG)
		}
	}
}

// After a loss the mines, the wrong flag, and the mine that went off are
// legible and different from each other and from the open cells.
func TestLostBoardColors(t *testing.T) {
	for _, theme := range []string{"dark", "light"} {
		r, g, centers := rig16x9(t, theme)
		wrong := g.idx(8, 8) // a safe cell flagged
		g.cycle(wrong)
		boom := g.idx(centers[1][0]-1, centers[1][1]-1) // the mine of block 1
		g.over, g.boom = true, boom
		r.sync()
		res := r.styled(120, 30)
		_, openBG, _ := r.cellStyle(res, 8, 15)
		// an unflagged mine: block 2 has two mines
		mr, mc := centers[2][0]-1, centers[2][1]-1
		if !g.mine[g.idx(mr, mc)] {
			t.Fatal("setup: no mine there")
		}
		mfg, mbg, _ := r.cellStyle(res, mr, mc)
		wfg, wbg, _ := r.cellStyle(res, 8, 8)
		bfg, bbg, battrs := r.cellStyle(res, centers[1][0]-1, centers[1][1]-1)
		for name, c := range map[string]float64{"mine": contrast(mfg, mbg), "wrong flag": contrast(wfg, wbg), "boom": contrast(bfg, bbg)} {
			if c < 4.5 {
				t.Errorf("%s: %s has contrast %.2f", theme, name, c)
			}
		}
		if mbg == openBG || wbg == openBG || bbg == openBG {
			t.Errorf("%s: a loss marker has the open background", theme)
		}
		if bbg == mbg {
			t.Errorf("%s: the mine that went off looks like the other mines", theme)
		}
		if !hasAttr(battrs, "bold") {
			t.Errorf("%s: the exploded mine is not bold", theme)
		}
		if got := r.str("b." + cellID(mr, mc) + ".t"); got != glyphMine {
			t.Errorf("mine text %q", got)
		}
	}
}

// The panel's state word is colored for the end of the game, legibly.
func TestStateColors(t *testing.T) {
	for _, theme := range []string{"dark", "light"} {
		colors := map[string]string{}
		for _, look := range []string{"started", "lost", "won", "paused"} {
			r := newRigOn(t, beginner, 5)
			r.arrange(look)
			if err := r.a.ui.Set("@theme", theme); err != nil {
				t.Fatal(err)
			}
			res := r.styled(90, 24)
			word := map[string]string{"started": "playing", "lost": "boom", "won": "cleared", "paused": "paused"}[look]
			found := false
			for y, row := range res.Dump.Grid {
				if x := strings.Index(row, word); x >= 0 && y > 2 && y < 12 {
					fg, bg, _ := spanAt(t, res, len([]rune(row[:x])), y)
					if c := contrast(fg, bg); c < 4.5 {
						t.Errorf("%s %s: %q has contrast %.2f", theme, look, word, c)
					}
					colors[look] = fg
					found = true
				}
			}
			if !found {
				t.Errorf("%s %s: no %q in the panel", theme, look, word)
			}
		}
		if colors["lost"] == colors["won"] || colors["won"] == colors["started"] || colors["paused"] == colors["started"] {
			t.Errorf("%s: the state colors do not tell the states apart: %v", theme, colors)
		}
	}
}

// Pausing paints the board cells hidden: the covered texture and the
// numbers are gone from the grid and the frame is still there.
func TestPausedBoardIsBlank(t *testing.T) {
	r := newRig(t)
	r.arrange("started")
	r.play("p")
	g := r.grid(90, 24)
	if strings.Contains(g, glyphHidden) || strings.Contains(g, glyphFlag) {
		t.Errorf("cells are painted while paused:\n%s", g)
	}
	for _, line := range strings.Split(g, "\n")[6:16] {
		if strings.ContainsAny(line, "12345678") && !strings.Contains(line, "│  │") && strings.Count(line, "│") < 2 {
			t.Errorf("a number shows while paused: %q", line)
		}
	}
	if !strings.Contains(g, "┌") || !strings.Contains(g, "paused") {
		t.Errorf("no frame or no word about the pause:\n%s", g)
	}
}
