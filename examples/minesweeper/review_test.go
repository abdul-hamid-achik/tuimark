package main

import (
	"errors"
	"os"
	"regexp"
	"strings"
	"testing"
)

// ---- chords ---------------------------------------------------------------

// A question mark is covered, so a chord opens it (and counts it as a
// neighbor left to open).
func TestChordOpensQuestionMarks(t *testing.T) {
	g := fromGrid("*..", "...", "...")
	one := g.at(1, 1)
	g.reveal(one, nil)
	g.cycle(g.at(0, 0)) // the mine, flagged
	g.cycle(g.at(2, 2))
	g.cycle(g.at(2, 2)) // a question mark
	if g.st[g.at(2, 2)] != stQuestion {
		t.Fatal("setup: no question mark")
	}
	if !g.chord(one) {
		t.Fatal("chord did nothing")
	}
	if g.st[g.at(2, 2)] != stOpen {
		t.Error("the question mark next to the chorded number stayed covered")
	}
}

// With a question mark as the only covered neighbor, the number can still be
// chorded: the chord is something to do.
func TestChordWithOnlyAQuestionMarkLeft(t *testing.T) {
	g := fromGrid("*..", "...", "...")
	for i := range g.st {
		g.st[i] = stOpen
	}
	g.opened = g.cells() - 1
	g.st[g.at(0, 0)] = stFlag
	g.flags = 1
	g.st[g.at(2, 2)] = stQuestion
	g.opened--
	one := g.at(1, 1)
	if !g.canChord(one) {
		t.Fatal("a number with a flag and a question mark left cannot be chorded")
	}
	g.chord(one)
	if g.st[g.at(2, 2)] != stOpen || !g.won {
		t.Errorf("the question mark was not opened (state %d, won %v)", g.st[g.at(2, 2)], g.won)
	}
}

// A chord that opens two mines names the first one in reading order and
// stops there.
func TestChordStopsAtTheFirstMine(t *testing.T) {
	g := fromGrid("*.*", "...", "...")
	two := g.at(1, 1)
	g.reveal(two, nil)
	g.cycle(g.at(1, 0)) // two wrong flags, as many as the number says
	g.cycle(g.at(1, 2))
	if !g.chord(two) {
		t.Fatal("chord did nothing")
	}
	if !g.over || g.won {
		t.Fatalf("over=%v won=%v", g.over, g.won)
	}
	if g.boom != g.at(0, 0) {
		t.Errorf("boom = %d, want the first mine in reading order, %d", g.boom, g.at(0, 0))
	}
	if g.st[g.at(0, 2)] == stOpen {
		t.Error("the chord went on to open the second mine")
	}
}

// ---- flags -----------------------------------------------------------------

func TestSizeFlagsAreStrict(t *testing.T) {
	for _, s := range []string{"", "90", "90x", "x24", "90x 24", " 90x24", "90x24 ", "+90x24", "-9x24", "9x-4", "90X24", "90x24x3", "0x24", "90x0", "1001x24", "90x1001", "99999999999999999999x24", "9 0x24", "90×24"} {
		if c, r, err := parseSize("-dump", s); err == nil || !errors.Is(err, errUsage) {
			t.Errorf("%q gave %d, %d, %v; want a usage error", s, c, r, err)
		}
	}
	for s, want := range map[string][2]int{"1x1": {1, 1}, "90x24": {90, 24}, "1000x1000": {1000, 1000}, "036x020": {36, 20}} {
		if c, r, err := parseSize("-dump", s); err != nil || [2]int{c, r} != want {
			t.Errorf("%q gave %d, %d, %v; want %v", s, c, r, err, want)
		}
	}
}

// A flag given empty is an error, not a missing flag: -theme "" must not
// be taken for no theme, and -dump "" must not start the interactive game.
func TestEmptyFlagsAreUsageErrors(t *testing.T) {
	for _, o := range []options{
		{dir: ".", level: "beginner", hasTheme: true, theme: ""},
		{dir: ".", level: "beginner", hasDump: true, dump: ""},
		{dir: ".", level: "beginner", hasDemo: true, demo: ""},
		{dir: ".", level: "beginner", dump: "90x 24"},
		{dir: ".", level: "beginner", dump: "+90x24"},
		{dir: ".", level: "beginner", hasDump: true, dump: " "},
	} {
		if err := run(o); err == nil || !errors.Is(err, errUsage) {
			t.Errorf("%+v: err = %v, want a usage error", o, err)
		}
	}
}

// The demo needs its board on the screen: a terminal smaller than that is a
// usage error, up front, not a failure halfway through the game.
func TestDemoSizeIsCheckedUpFront(t *testing.T) {
	nc, nr := needCols(demoBoard.cols), needRows(demoBoard.rows)
	for _, sz := range [][2]int{{40, 14}, {nc - 1, nr}, {nc, nr - 1}, {1, 1}, {nc - 1, 1000}} {
		if err := checkDemoSize(sz[0], sz[1]); err == nil || !errors.Is(err, errUsage) {
			t.Errorf("%dx%d: %v, want a usage error", sz[0], sz[1], err)
		}
	}
	for _, sz := range [][2]int{{nc, nr}, {90, 24}, {1000, 1000}} {
		if err := checkDemoSize(sz[0], sz[1]); err != nil {
			t.Errorf("%dx%d: %v", sz[0], sz[1], err)
		}
	}
	for _, s := range []string{"40x14", "35x20", "36x19"} {
		if err := run(options{dir: ".", level: "beginner", demo: s}); err == nil || !errors.Is(err, errUsage) {
			t.Errorf("-demo %s: %v, want a usage error", s, err)
		}
	}
}

// ---- the board's look -----------------------------------------------------

// Everything on a cell is right-aligned: a number, a flag, a question mark,
// a mine, a wrong flag, so a mark next to a number does not look cramped.
func TestCellTextsShareOneAlignment(t *testing.T) {
	kinds := map[string]bool{}
	for _, look := range []string{"started", "flagmode", "lost", "won"} {
		r := newRigOn(t, beginner, 5)
		r.arrange(look)
		g := r.a.g
		g.cycle(firstCovered(g)) // a flag, a question mark
		g.cycle(firstCovered(g))
		g.cycle(firstCovered(g))
		r.sync()
		for rr := 0; rr < g.rows; rr++ {
			for cc := 0; cc < g.cols; cc++ {
				tx := r.str("b." + cellID(rr, cc) + ".t")
				if len([]rune(tx)) != 2 {
					t.Fatalf("cell text %q is not two characters", tx)
				}
				kinds[tx] = true
				switch rs := []rune(tx); {
				case tx == glyphHidden:
				case rs[0] != ' ':
					t.Errorf("%s: cell %s has %q, not right-aligned", look, cellID(rr, cc), tx)
				}
			}
		}
	}
	for _, want := range []string{glyphHidden, glyphEmpty, glyphFlag, glyphMine, " 1", " 2"} {
		if !kinds[want] {
			t.Errorf("the states never showed %q", want)
		}
	}
}

func firstCovered(g *game) int {
	for i := range g.st {
		if g.st[i] == stHidden {
			return i
		}
	}
	return 0
}

// A mark beside a number reads as two separate cells.
func TestMarksBesideNumbersAreNotCramped(t *testing.T) {
	for _, g := range []string{glyphFlag, glyphQuestion, glyphMine, glyphWrong} {
		if !strings.HasPrefix(g, " ") || len([]rune(g)) != 2 {
			t.Errorf("%q is not a space and a mark", g)
		}
	}
}

// The generated cells alternate a second cover shade by class: static, by
// position, so a covered board has a checkerboard grid.
func TestCellsAlternateByPosition(t *testing.T) {
	src, err := os.ReadFile("app.tui")
	if err != nil {
		t.Fatal(err)
	}
	cells := regexp.MustCompile(`<text id="c(\d\d)(\d\d)" class="(cell(?: alt)?)"`).FindAllStringSubmatch(string(src), -1)
	if len(cells) != maxRows*maxCols {
		t.Fatalf("%d cells in app.tui", len(cells))
	}
	alt := 0
	for _, m := range cells {
		var r, c int
		r = int(m[1][0]-'0')*10 + int(m[1][1]-'0')
		c = int(m[2][0]-'0')*10 + int(m[2][1]-'0')
		want := "cell"
		if (r+c)%2 == 1 {
			want = "cell alt"
			alt++
		}
		if m[3] != want {
			t.Errorf("cell c%02d%02d has class %q, want %q", r, c, m[3], want)
		}
	}
	if alt != maxRows*maxCols/2 {
		t.Errorf("%d alternate cells", alt)
	}
	// The dump agrees: neighbors differ.
	r := newRig(t)
	res := r.playAt(90, 24)
	classes := map[string][]string{}
	for _, n := range res.Dump.Nodes {
		classes[n.ID] = n.Classes
	}
	has := func(id string) bool {
		for _, c := range classes[id] {
			if c == "alt" {
				return true
			}
		}
		return false
	}
	if has("c0000") || !has("c0001") || !has("c0100") || has("c0101") {
		t.Errorf("classes: c0000 %v, c0001 %v, c0100 %v, c0101 %v", classes["c0000"], classes["c0001"], classes["c0100"], classes["c0101"])
	}
}

// ---- the sample -----------------------------------------------------------

// Cells outside the grid are {"on": false} and nothing else, and the sample
// stays small.
func TestSampleIsLean(t *testing.T) {
	raw, err := os.ReadFile("sample.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) > 40000 {
		t.Errorf("sample.json is %d bytes: off-grid cells should be {\"on\": false}", len(raw))
	}
	// The host publishes the same shape when a cell leaves the grid.
	r := newRigOn(t, board{"intermediate", 16, 16, 40}, 1)
	r.play("1")
	v, _ := r.a.ui.Get("b.c1010")
	if m, _ := v.(map[string]any); len(m) != 1 || m["on"] != false {
		t.Errorf("a cell that left the grid is %v", v)
	}
	r.play("3")
	v, _ = r.a.ui.Get("b.c1010")
	if m, _ := v.(map[string]any); len(m) != 17 || m["on"] != true {
		t.Errorf("a cell that came onto the grid is %v", v)
	}
}

// ---- the help -------------------------------------------------------------

// The help takes the focus on its body, so on a short terminal the arrow
// keys scroll it at once, without a tab; enter still closes it.
func TestHelpScrollsAtOnceOnAShortTerminal(t *testing.T) {
	r := newRig(t)
	res := r.playAt(40, 14, "?")
	if focusOf(res) != "help-body" {
		t.Fatalf("focus %q, want help-body", focusOf(res))
	}
	if strings.Contains(gridOf(res), "Chord") {
		t.Fatalf("the last line is already on the screen; the test needs a terminal where it is cut:\n%s", gridOf(res))
	}
	var y0 int
	res = r.playAt(40, 14, "down", "down", "down")
	for _, n := range res.Dump.Nodes {
		if n.ID == "help-body" && n.Scroll != nil && n.Scroll.Y != nil {
			y0 = *n.Scroll.Y
		}
	}
	if y0 != 3 {
		t.Errorf("three downs scrolled the help to %d, want 3", y0)
	}
	res = r.playAt(40, 14, "end")
	if g := strings.Join(strings.Fields(gridOf(res)), " "); !strings.Contains(g, "cells around it") {
		t.Errorf("end did not bring the last line of the help up:\n%s", gridOf(res))
	}
	res = r.playAt(40, 14, "enter")
	wantActions(t, res, "help_close")
	if r.boolean("help_open") {
		t.Error("enter did not close the help")
	}
}
