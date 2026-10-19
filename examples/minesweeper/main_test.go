package main

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

func TestValidateIsClean(t *testing.T) {
	r := newRig(t)
	for _, theme := range []string{"dark", "light"} {
		if err := r.a.ui.Set("@theme", theme); err != nil {
			t.Fatal(err)
		}
		if diags := r.a.ui.Validate(); len(diags) != 0 {
			t.Errorf("%s: Validate gave %v", theme, diags)
		}
	}
}

// Every action the document names has a handler, except the built-ins.
func TestHandlersCoverCatalog(t *testing.T) {
	r := newRig(t)
	for _, spec := range r.a.ui.Catalog() {
		if !spec.Builtin && !spec.Registered {
			t.Errorf("no handler for %s (%v)", spec.Name, spec.Sources)
		}
	}
}

func TestDumpShowsTheGame(t *testing.T) {
	r := newRig(t)
	g := r.grid(90, 24)
	for _, want := range []string{"minesweeper", "beginner 9x9, 10 mines", "ready", "mines left", "010", "0:00",
		"best times", "intermediate", "expert", "help", "quit", "reveal", "flag mode"} {
		if !strings.Contains(g, want) {
			t.Errorf("grid has no %q:\n%s", want, g)
		}
	}
	// The covered board is 9 rows of 18 covered columns.
	if n := strings.Count(g, strings.Repeat(glyphHidden, 9)); n != 9 {
		t.Errorf("%d covered rows, want 9:\n%s", n, g)
	}
}

// ---- keys ---------------------------------------------------------------

// keyActions is every key the keymap binds and the action it fires from a
// fresh game. TestKeysTableIsComplete keeps it in step with app.tui, so
// deleting a row from the keymap fails TestKeysFireTheirActions.
var keyActions = map[string]string{
	"?": "help_toggle", "q": "quit", "ctrl+c": "quit",
	"up": "cur_up", "k": "cur_up", "down": "cur_down", "j": "cur_down",
	"left": "cur_left", "h": "cur_left", "right": "cur_right", "l": "cur_right",
	"enter": "reveal", "space": "reveal",
	"f": "flag_mode", "g": "flag", "n": "new",
	"1": "level_beginner", "2": "level_intermediate", "3": "level_expert",
	"p": "pause", "m": "toggle_mouse",
}

func TestKeysFireTheirActions(t *testing.T) {
	for key, action := range keyActions {
		r := newRig(t)
		res := r.play(key)
		if len(res.Events) != 1 || res.Events[0].Action != action {
			t.Errorf("%s: events = %v, want one %s", key, actions(res), action)
		}
		if action == "quit" && !res.Quit {
			t.Errorf("%s did not quit", key)
		}
	}
}

func TestKeysTableIsComplete(t *testing.T) {
	src, err := os.ReadFile("app.tui")
	if err != nil {
		t.Fatal(err)
	}
	// keys="..." on the rows of the three keymaps (the help one has its own table below)
	seen := map[string]bool{}
	for _, m := range regexp.MustCompile(`keys="([^"]+)"`).FindAllStringSubmatch(string(src), -1) {
		for _, k := range strings.Split(m[1], ",") {
			seen[k] = true
		}
	}
	for k := range seen {
		if _, ok := keyActions[k]; !ok && k != "enter" {
			t.Errorf("app.tui binds %q, which TestKeysFireTheirActions does not try", k)
		}
	}
	for k := range keyActions {
		if !seen[k] {
			t.Errorf("the table has %q, which app.tui does not bind", k)
		}
	}
}

func TestKeymapPutsHelpAndQuitFirst(t *testing.T) {
	src, err := os.ReadFile("app.tui")
	if err != nil {
		t.Fatal(err)
	}
	rows := regexp.MustCompile(`<bind keys="([^"]+)" action="([^"]+)"`).FindAllStringSubmatch(string(src), -1)
	if len(rows) < 2 || rows[0][2] != "help_toggle" || rows[1][2] != "quit" {
		t.Errorf("the first keymap rows are %v, want help_toggle then quit", rows[:2])
	}
}

// The hints keep the help and quit keys when the terminal is too narrow for
// all of them.
func TestHintsKeepHelpAndQuit(t *testing.T) {
	for _, cols := range []int{40, 44, 50, 60, 90} {
		r := newRig(t)
		g := r.grid(cols, 24)
		last := strings.Split(g, "\n")[23]
		for _, want := range []string{"? help", "q quit"} {
			if !strings.Contains(last, want) {
				t.Errorf("%d columns: hints %q lack %q", cols, last, want)
			}
		}
	}
}

// ---- the board ----------------------------------------------------------

func TestClickRevealsAndMovesTheCursor(t *testing.T) {
	r := newRig(t)
	res := r.play(r.click(2, 6))
	wantActions(t, res, "cell")
	if res.Events[0].Source != cellID(2, 6) {
		t.Errorf("source = %q, want %s", res.Events[0].Source, cellID(2, 6))
	}
	if !r.a.g.placed || r.a.g.st[r.a.g.idx(2, 6)] != stOpen {
		t.Error("the click did not reveal the cell")
	}
	if r.a.cur != r.a.g.idx(2, 6) || !r.boolean("b.c0206.cur") {
		t.Error("the click did not move the cursor to the cell")
	}
	if r.str("state") != "playing" {
		t.Errorf("state = %s", r.str("state"))
	}
	// The first reveal is never a mine and opens a region.
	if r.a.g.opened < 9 {
		t.Errorf("the first reveal opened %d cells", r.a.g.opened)
	}
}

// A click on a cell outside the grid cannot happen (it is not drawn), and a
// forged event for one is an error shown in the status line.
func TestClickOnAnUnknownCellIsAnError(t *testing.T) {
	r := newRig(t)
	h := r.a.handlers()["cell"]
	for _, src := range []string{"", "c9999", "c0912", "x0101", "c0a01", "c0000 "} {
		if err := h(tuimark.Event{Action: "cell", Source: src}); err == nil {
			t.Errorf("source %q gave no error", src)
		}
	}
	// c0909 is outside a 9x9 board's rows 0-8.
	if err := h(tuimark.Event{Action: "cell", Source: "c0900"}); err == nil {
		t.Error("a cell below the board gave no error")
	}
}

func TestEnterAndSpaceRevealAtTheCursor(t *testing.T) {
	for _, key := range []string{"enter", "space"} {
		r := newRig(t)
		r.play("right", "down", key)
		g := r.a.g
		want := g.idx(5, 5) // the cursor starts in the middle (4,4)
		if g.st[want] != stOpen {
			t.Errorf("%s: the cell under the cursor is not open", key)
		}
	}
}

func TestCursorMovesAndStopsAtTheEdges(t *testing.T) {
	r := newRig(t)
	g := r.a.g
	if r.a.cur != g.idx(4, 4) {
		t.Fatalf("the cursor starts at %d, want the middle", r.a.cur)
	}
	for _, c := range []struct {
		keys     []string
		row, col int
	}{
		{[]string{"up"}, 3, 4}, {[]string{"k"}, 2, 4}, {[]string{"left", "h"}, 2, 2},
		{[]string{"down", "j"}, 4, 2}, {[]string{"right", "l"}, 4, 4},
	} {
		r.play(c.keys...)
		if row, col := g.pos(r.a.cur); row != c.row || col != c.col {
			t.Errorf("after %v the cursor is at %d,%d, want %d,%d", c.keys, row, col, c.row, c.col)
		}
	}
	for i := 0; i < 12; i++ {
		r.play("up", "left")
	}
	if r.a.cur != 0 {
		t.Errorf("the cursor is at %d, want the top-left corner", r.a.cur)
	}
	for i := 0; i < 12; i++ {
		r.play("down", "right")
	}
	if r.a.cur != g.cells()-1 {
		t.Errorf("the cursor is at %d, want the bottom-right corner", r.a.cur)
	}
	// The store has the cursor on exactly one cell.
	n := 0
	for rr := 0; rr < maxRows; rr++ {
		for cc := 0; cc < maxCols; cc++ {
			if r.on("b." + cellID(rr, cc) + ".cur") {
				n++
			}
		}
	}
	if n != 1 || !r.boolean("b.c0808.cur") {
		t.Errorf("%d cells carry the cursor, c0808 = %v", n, r.boolean("b.c0808.cur"))
	}
}

func TestFlagKeyCyclesTheCursorCell(t *testing.T) {
	r := newRig(t)
	cell := cellID(4, 4)
	steps := []struct {
		flag, q, hid bool
		t            string
		left         string
	}{
		{true, false, true, glyphFlag, "009"},
		{false, true, true, glyphQuestion, "010"},
		{false, false, true, glyphHidden, "010"},
	}
	for i, s := range steps {
		res := r.play("g")
		wantActions(t, res, "flag")
		if r.boolean("b."+cell+".flag") != s.flag || r.boolean("b."+cell+".q") != s.q || r.boolean("b."+cell+".hid") != s.hid {
			t.Errorf("step %d: flag=%v q=%v hid=%v", i, r.boolean("b."+cell+".flag"), r.boolean("b."+cell+".q"), r.boolean("b."+cell+".hid"))
		}
		if got := r.str("b." + cell + ".t"); got != s.t {
			t.Errorf("step %d: text %q, want %q", i, got, s.t)
		}
		if got := r.str("mines_label"); got != s.left {
			t.Errorf("step %d: mines_label %q, want %q", i, got, s.left)
		}
	}
	// Flagging before the first reveal does not start the clock or lay mines.
	if r.a.g.placed || r.str("state") != "ready" {
		t.Error("flagging started the game")
	}
}

func TestFlagModeMakesAClickFlag(t *testing.T) {
	r := newRig(t)
	r.play("f")
	if !r.boolean("flag_mode") || r.str("mode_label") != "flag" {
		t.Fatal("f did not turn flag mode on")
	}
	click := r.click(1, 1)
	r.play(click)
	if !r.boolean("b.c0101.flag") || r.a.g.placed {
		t.Error("a click in flag mode did not flag the cell, or revealed it")
	}
	r.play(click)
	if !r.boolean("b.c0101.q") {
		t.Error("a second click did not make a question mark")
	}
	r.play(click)
	if r.boolean("b.c0101.q") || r.boolean("b.c0101.flag") || !r.boolean("b.c0101.hid") {
		t.Error("a third click did not clear the mark")
	}
	r.play("f")
	if r.boolean("flag_mode") || r.str("mode_label") != "reveal" {
		t.Error("f did not turn flag mode off")
	}
	r.play(click)
	if !r.a.g.placed {
		t.Error("a click outside flag mode did not reveal")
	}
}

// Enter in flag mode flags too, and a flagged cell is not revealed by a
// click outside flag mode.
func TestEnterFollowsTheMode(t *testing.T) {
	r := newRig(t)
	r.play("f", "enter")
	if !r.boolean("b.c0404.flag") {
		t.Error("enter in flag mode did not flag")
	}
	r.play("f", "enter")
	if !r.boolean("b.c0404.flag") || r.a.g.placed {
		t.Error("enter on a flagged cell revealed it")
	}
}

// Open numbers chord with a click, with enter, and with space, in either
// mode.
func TestChordByClickAndKey(t *testing.T) {
	for _, how := range []string{"click", "enter", "space"} {
		for _, flagMode := range []bool{false, true} {
			r := newRigOn(t, beginner, 1)
			a := r.a
			// A lone 1 with its mine flagged, then chord.
			a.g = fromGrid(
				"*........",
				".........",
				".........",
				".........",
				".........",
				".........",
				".........",
				".........",
				".........",
			)
			a.g.mines = 1
			a.cur = a.g.idx(1, 1)
			a.g.reveal(a.g.idx(1, 1), nil)
			a.g.cycle(a.g.idx(0, 0))
			if err := a.sync(); err != nil {
				t.Fatal(err)
			}
			if flagMode {
				r.play("f")
			}
			var res *tuimark.PlayResult
			if how == "click" {
				res = r.play(r.click(1, 1))
			} else {
				res = r.play(how)
			}
			if len(res.Events) == 0 {
				t.Fatalf("%s: no action fired", how)
			}
			// The flood from the chord opens the empty board: a win.
			if !a.g.won {
				t.Errorf("%s (flag mode %v): chord did not open the neighbors:\n%s", how, flagMode, strings.ReplaceAll(a.g.shown(), "/", "\n"))
			}
		}
	}
}

func TestNewGameKeepsTheLevelAndResetsTheClock(t *testing.T) {
	r := newRig(t)
	r.play(r.click(4, 4))
	for i := 0; i < 15; i++ {
		r.advance(200 * time.Millisecond)
	}
	if r.number("secs") != 3 {
		t.Fatalf("secs = %d, want 3", r.number("secs"))
	}
	r.play("g", "f")
	res := r.play("n")
	wantActions(t, res, "new")
	if r.a.g.placed || r.number("secs") != 0 || r.str("state") != "ready" || r.a.g.opened != 0 {
		t.Errorf("new game: placed=%v secs=%d state=%s", r.a.g.placed, r.number("secs"), r.str("state"))
	}
	if r.str("level") != "beginner" || r.str("mines_label") != "010" {
		t.Errorf("level=%s mines=%s", r.str("level"), r.str("mines_label"))
	}
	if !strings.Contains(r.grid(90, 24), strings.Repeat(glyphHidden, 9)) {
		t.Error("the board is not covered again")
	}
}

// ---- levels -------------------------------------------------------------

func TestLevelKeysChangeTheBoard(t *testing.T) {
	cases := []struct {
		key        string
		level      string
		cols, rows int
		mines      string
	}{
		{"2", "intermediate", 16, 16, "040"}, {"3", "expert", 30, 16, "099"}, {"1", "beginner", 9, 9, "010"},
	}
	r := newRig(t)
	for _, c := range cases {
		r.play(c.key)
		if r.str("level") != c.level || r.number("dims.cols") != c.cols || r.number("dims.rows") != c.rows {
			t.Errorf("%s: level %s %dx%d", c.key, r.str("level"), r.number("dims.cols"), r.number("dims.rows"))
		}
		if r.str("mines_label") != c.mines {
			t.Errorf("%s: mines_label %s, want %s", c.key, r.str("mines_label"), c.mines)
		}
		// Exactly the cells of the grid are on, and the rows.
		on, rowsOn := 0, 0
		for rr := 0; rr < maxRows; rr++ {
			if r.boolean("rows." + rowID(rr)) {
				rowsOn++
			}
			for cc := 0; cc < maxCols; cc++ {
				if r.boolean("b." + cellID(rr, cc) + ".on") {
					on++
					if rr >= c.rows || cc >= c.cols {
						t.Errorf("%s: cell %s is on outside the grid", c.key, cellID(rr, cc))
					}
				}
			}
		}
		if on != c.cols*c.rows || rowsOn != c.rows {
			t.Errorf("%s: %d cells and %d rows on, want %d and %d", c.key, on, rowsOn, c.cols*c.rows, c.rows)
		}
		// The cursor is in the middle of the new board.
		if row, col := r.a.g.pos(r.a.cur); row != c.rows/2 || col != c.cols/2 {
			t.Errorf("%s: cursor at %d,%d", c.key, row, col)
		}
	}
}

// Every cell the grid shows can be clicked, corners included, at every
// level; the dump has a node for each.
func TestEveryCellOfEveryLevelIsClickable(t *testing.T) {
	for _, p := range presets {
		r := newRigOn(t, board{p.name, p.cols, p.rows, p.mines}, 1)
		res := r.playAt(120, 30)
		nodes := map[string]bool{}
		for _, n := range res.Dump.Nodes {
			nodes[n.ID] = true
		}
		for rr := 0; rr < maxRows; rr++ {
			for cc := 0; cc < maxCols; cc++ {
				inGrid := rr < p.rows && cc < p.cols
				if nodes[cellID(rr, cc)] != inGrid {
					t.Fatalf("%s: cell %s has a node = %v, want %v", p.name, cellID(rr, cc), nodes[cellID(rr, cc)], inGrid)
				}
			}
		}
		// Clicking the last cell works.
		step := clickStep(t, res, p.rows-1, p.cols-1)
		ev := r.playAt(120, 30, step)
		wantActions(t, ev, "cell")
		if ev.Events[0].Source != cellID(p.rows-1, p.cols-1) {
			t.Errorf("%s: the corner click reported %s", p.name, ev.Events[0].Source)
		}
	}
}

func TestCustomBoards(t *testing.T) {
	for _, b := range []board{{"custom", 12, 10, 20}, {"custom", 30, 9, 50}, {"custom", 9, 16, 71}} {
		r := newRigOn(t, b, 3)
		if r.str("level") != "custom" || r.number("dims.mines") != b.mines {
			t.Errorf("%v: level %s mines %d", b, r.str("level"), r.number("dims.mines"))
		}
		r.play(r.click(b.rows/2, b.cols/2))
		if r.a.g.over {
			t.Errorf("%v: the first click ended the game", b)
		}
		if !strings.Contains(r.grid(120, 30), "custom") {
			t.Errorf("%v: the level is not on the screen", b)
		}
		// n keeps the custom board.
		r.play("n")
		if r.a.g.cols != b.cols || r.a.g.rows != b.rows || r.a.g.mines != b.mines || r.str("level") != "custom" {
			t.Errorf("%v: n changed the board", b)
		}
	}
}

// ---- pause, help, mouse -------------------------------------------------

func TestPauseHidesTheBoardAndStopsTheClock(t *testing.T) {
	r := newRig(t)
	r.play(r.click(4, 4))
	for i := 0; i < 10; i++ {
		r.advance(200 * time.Millisecond)
	}
	// The cursor on a safe covered cell: enter would reveal it and g would flag
	// it if the paused board answered.
	cov := -1
	for i := range r.a.g.st {
		if r.a.g.st[i] == stHidden && !r.a.g.mine[i] {
			cov = i
			break
		}
	}
	r.a.cur = cov
	if err := r.a.sync(); err != nil {
		t.Fatal(err)
	}
	res := r.play("p")
	wantActions(t, res, "pause")
	if r.str("state") != "paused" || r.str("face") != ":|" {
		t.Errorf("state %s face %s", r.str("state"), r.str("face"))
	}
	g := gridOf(res)
	if strings.Contains(g, glyphHidden) || strings.Contains(g, " 1") && strings.Count(g, " 1") > 3 {
		t.Errorf("the board is still on the screen:\n%s", g)
	}
	if !strings.Contains(g, "paused") {
		t.Errorf("no word about the pause:\n%s", g)
	}
	before := r.number("secs")
	for i := 0; i < 25; i++ {
		r.advance(200 * time.Millisecond)
	}
	if r.number("secs") != before {
		t.Errorf("the clock ran while paused: %d -> %d", before, r.number("secs"))
	}
	// Nothing on the board answers: not a click on a covered cell, not
	// enter or space on the cursor's covered cell, not g, not an arrow.
	opened := r.a.g.opened
	covRow, covCol := r.a.g.pos(cov)
	for _, steps := range [][]string{{r.click(covRow, covCol)}, {"enter"}, {"space"}, {"g"}, {"right"}} {
		res = r.play(steps...)
		for _, e := range res.Events {
			if e.Action == "cell" {
				t.Errorf("%v: a click reached the hidden board", steps)
			}
		}
		if r.a.g.opened != opened || r.a.g.flags != 0 || r.a.g.st[cov] != stHidden {
			t.Fatalf("the paused board answered to %v", steps)
		}
	}
	if r.a.cur != cov {
		t.Errorf("the cursor moved while paused: %d -> %d", cov, r.a.cur)
	}
	// p resumes, the board is back, the same key now works, and the clock goes on.
	r.play("p")
	if r.str("state") != "playing" || !strings.Contains(r.grid(90, 24), glyphHidden) {
		t.Error("p did not resume")
	}
	r.play("g")
	if r.a.g.st[cov] != stFlag {
		t.Error("g did not flag the cursor cell once resumed")
	}
	r.play("g", "g") // back to covered
	for i := 0; i < 10; i++ {
		r.advance(200 * time.Millisecond)
	}
	if r.number("secs") != before+2 {
		t.Errorf("secs = %d after resuming, want %d", r.number("secs"), before+2)
	}
}

func TestPauseNeedsAStartedGame(t *testing.T) {
	r := newRig(t)
	r.play("p")
	if r.boolean("paused") {
		t.Error("paused a game that has not started")
	}
	r.play(r.click(4, 4))
	r.a.g.over = true // finished
	r.play("p")
	if r.boolean("paused") {
		t.Error("paused a finished game")
	}
}

func TestNewGameClearsThePause(t *testing.T) {
	r := newRig(t)
	r.play(r.click(4, 4), "p", "n")
	if r.boolean("paused") || r.str("state") != "ready" {
		t.Error("n left the pause on")
	}
}

func TestHelpOpensStopsTheClockAndCloses(t *testing.T) {
	r := newRig(t)
	r.play(r.click(4, 4))
	for i := 0; i < 10; i++ {
		r.advance(200 * time.Millisecond)
	}
	res := r.play("?")
	wantActions(t, res, "help_toggle")
	if focusOf(res) != "help-body" || !strings.Contains(gridOf(res), "reveal; on a number, chord") {
		t.Errorf("focus %q:\n%s", focusOf(res), gridOf(res))
	}
	before := r.number("secs")
	for i := 0; i < 30; i++ {
		r.advance(200 * time.Millisecond)
	}
	if r.number("secs") != before {
		t.Errorf("the clock ran under the help: %d -> %d", before, r.number("secs"))
	}
	// The board keys are quiet while the help is open.
	snapshot := r.a.g.shown()
	cur := r.a.cur
	res = r.play("up", "k", "g", "f", "n", "1", "2", "3", "p", "m")
	for _, e := range res.Events {
		t.Errorf("%s fired under the help", e.Action)
	}
	if r.a.g.shown() != snapshot || r.a.cur != cur || r.boolean("flag_mode") || r.boolean("paused") || !r.boolean("settings.mouse") {
		t.Error("a key reached the board under the help")
	}
	res = r.play("esc")
	wantActions(t, res, "help_close")
	if r.boolean("help_open") || focusOf(res) != "board" {
		t.Errorf("help_open=%v focus=%q after esc", r.boolean("help_open"), focusOf(res))
	}
	for i := 0; i < 10; i++ {
		r.advance(200 * time.Millisecond)
	}
	if r.number("secs") != before+2 {
		t.Errorf("secs = %d after closing, want %d", r.number("secs"), before+2)
	}
}

func TestHelpClosesWithEveryKeyAndTheButton(t *testing.T) {
	for _, key := range []string{"esc", "?", "enter"} {
		r := newRig(t)
		r.play("?")
		res := r.play(key)
		if r.boolean("help_open") {
			t.Errorf("%s did not close the help (events %v)", key, actions(res))
		}
		if res.Quit {
			t.Errorf("%s quit from the help", key)
		}
	}
	// q and ctrl+c quit from the help as anywhere: a modal traps the
	// keymap, so they are repeated in <keymap when="modal">.
	for _, key := range []string{"q", "ctrl+c"} {
		r := newRig(t)
		r.play("?")
		if res := r.play(key); !res.Quit {
			t.Errorf("%s did not quit from the help", key)
		}
	}
	r := newRig(t)
	res := r.play("?")
	// The close button is on the screen and a click closes it.
	var x, y int
	for _, n := range res.Dump.Nodes {
		if n.ID == "help-ok" {
			x, y = n.X+1, n.Y
		}
	}
	r.play(fmtClick(x, y))
	if r.boolean("help_open") {
		t.Error("clicking the close button did not close the help")
	}
	// Ctrl+C quits from the help as anywhere.
	r = newRig(t)
	r.play("?")
	if res := r.play("ctrl+c"); !res.Quit {
		t.Error("ctrl+c did not quit from the help")
	}
}

func fmtClick(x, y int) string { return fmt.Sprintf("click:%d,%d", x, y) }

func TestMouseKeyTurnsTheMouseOffAndOn(t *testing.T) {
	r := newRig(t)
	click := r.click(4, 4)
	r.play("m")
	if r.boolean("settings.mouse") {
		t.Fatal("m did not turn the mouse off")
	}
	res := r.play(click)
	if len(res.Events) != 0 || r.a.g.placed {
		t.Errorf("a click reached the board with the mouse off: %v", actions(res))
	}
	r.play("m")
	if !r.boolean("settings.mouse") {
		t.Fatal("m did not turn the mouse on")
	}
	wantActions(t, r.play(click), "cell")
}

// ---- the clock ----------------------------------------------------------

func TestClockStartsAtTheFirstRevealAndStopsAtTheEnd(t *testing.T) {
	r := newRig(t)
	for i := 0; i < 20; i++ { // flags and a quiet board do not start it
		r.advance(200 * time.Millisecond)
	}
	r.play("g")
	for i := 0; i < 20; i++ {
		r.advance(200 * time.Millisecond)
	}
	if r.number("secs") != 0 || r.str("time_label") != "0:00" {
		t.Fatalf("the clock ran before the first reveal: %d", r.number("secs"))
	}
	r.play(r.click(2, 6))
	for i := 0; i < 5*7; i++ {
		r.advance(200 * time.Millisecond)
	}
	if r.number("secs") != 7 || r.str("time_label") != "0:07" {
		t.Fatalf("secs = %d (%s), want 7", r.number("secs"), r.str("time_label"))
	}
	// Lose: reveal a mine. The clock stops.
	mine := firstMine(r.a.g)
	r.clickIndex(mine)
	if r.str("state") != "lost" {
		t.Fatalf("state = %s", r.str("state"))
	}
	at := r.number("secs")
	for i := 0; i < 50; i++ {
		r.advance(200 * time.Millisecond)
	}
	if r.number("secs") != at {
		t.Errorf("the clock ran after the game ended: %d -> %d", at, r.number("secs"))
	}
}

func firstMine(g *game) int {
	for i, m := range g.mine {
		if m {
			return i
		}
	}
	return -1
}

// clickIndex clicks board cell i through the UI.
func (r *rig) clickIndex(i int) *tuimark.PlayResult {
	r.t.Helper()
	row, col := r.a.g.pos(i)
	return r.play(r.click(row, col))
}

func TestClockMinutesAndTheCap(t *testing.T) {
	r := newRig(t)
	r.play(r.click(4, 4))
	for i := 0; i < 5*83; i++ {
		r.advance(200 * time.Millisecond)
	}
	if r.str("time_label") != "1:23" {
		t.Errorf("time_label = %s, want 1:23", r.str("time_label"))
	}
	for _, c := range []struct {
		secs int
		want string
	}{{0, "0:00"}, {59, "0:59"}, {60, "1:00"}, {3599, "59:59"}, {maxSecs, "99:59"}} {
		if got := clock(c.secs); got != c.want {
			t.Errorf("clock(%d) = %s, want %s", c.secs, got, c.want)
		}
	}
	r.a.elapsed = 200 * time.Hour
	if r.a.secs() != maxSecs {
		t.Errorf("secs() = %d, want the cap %d", r.a.secs(), maxSecs)
	}
}

// A gap longer than a second (a suspended terminal) counts as a second.
func TestAStallIsNotPlayTime(t *testing.T) {
	r := newRig(t)
	r.play(r.click(4, 4))
	r.advance(time.Hour)
	if r.number("secs") != 1 {
		t.Errorf("secs = %d after an hour's stall, want 1", r.number("secs"))
	}
}

// A key press settles the clock first, so a win at the end of a long gap
// is timed by the keys' own clock.
func TestActionsSettleTheClock(t *testing.T) {
	r := newRig(t)
	r.play(r.click(4, 4))
	r.now = r.now.Add(900 * time.Millisecond)
	r.play("right") // settles 900 ms
	r.now = r.now.Add(900 * time.Millisecond)
	r.play("right") // and 900 more
	if r.a.secs() != 1 {
		t.Errorf("secs = %d, want 1 (1.8 s)", r.a.secs())
	}
	r.advance(200 * time.Millisecond)
	if r.number("secs") != 2 {
		t.Errorf("the store shows %d, want 2 after the tick", r.number("secs"))
	}
}

// ---- winning, losing, best times ---------------------------------------

// solve plays the game to its end with the solver, through Go calls on the
// app's own game, and publishes the result.
func (r *rig) solve() {
	r.t.Helper()
	for {
		m, ok := nextMove(r.a.g)
		if !ok {
			break
		}
		switch m.kind {
		case mvFlag:
			r.a.g.cycle(m.cell)
		case mvChord:
			r.a.g.chord(m.cell)
		default:
			r.a.g.reveal(m.cell, r.a.rng)
		}
		r.a.after()
	}
	if err := r.a.sync(); err != nil {
		r.t.Fatal(err)
	}
}

// winning loads seeds until the solver wins one and returns that rig after
// the win; the clock has run for secs seconds.
func winning(t *testing.T, b board, secs int) *rig {
	t.Helper()
	for seed := int64(1); seed < 60; seed++ {
		r := newRigOn(t, b, seed)
		r.play(r.click(b.rows/2, b.cols/2))
		for i := 0; i < secs*5; i++ {
			r.advance(200 * time.Millisecond)
		}
		r.solve()
		if r.a.g.won {
			return r
		}
	}
	t.Fatalf("the solver never won on %v", b)
	return nil
}

func TestWinningStopsTheClockAndShowsIt(t *testing.T) {
	r := winning(t, beginner, 42)
	if r.str("state") != "won" || r.str("face") != "B)" || r.str("state_label") != "cleared" {
		t.Errorf("state %s face %s label %s", r.str("state"), r.str("face"), r.str("state_label"))
	}
	if r.str("mines_label") != "000" {
		t.Errorf("mines_label = %s, want 000 (the mines are flagged)", r.str("mines_label"))
	}
	g := r.grid(90, 24)
	for _, want := range []string{"B) cleared", "0:42"} {
		if !strings.Contains(g, want) {
			t.Errorf("no %q on the screen:\n%s", want, g)
		}
	}
	if r.number("secs") != 42 {
		t.Errorf("secs = %d, want 42", r.number("secs"))
	}
	// Every safe cell is open and every mine flagged.
	g2 := r.a.g
	for i := range g2.mine {
		if g2.mine[i] != (g2.st[i] == stFlag) || (!g2.mine[i] && g2.st[i] != stOpen) {
			t.Fatalf("cell %d: mine=%v state=%d", i, g2.mine[i], g2.st[i])
		}
	}
	// Further clicks and keys change nothing.
	shown := g2.shown()
	r.play(r.click(0, 0), "enter", "g")
	if g2.shown() != shown {
		t.Error("the finished board answered")
	}
	for i := 0; i < 20; i++ {
		r.advance(200 * time.Millisecond)
	}
	if r.number("secs") != 42 {
		t.Errorf("the clock ran after the win: %d", r.number("secs"))
	}
}

func TestBestTimesKeepTheFastestWinPerLevel(t *testing.T) {
	r := winning(t, beginner, 42)
	if got := r.str("best_label.beginner"); got != "0:42" {
		t.Fatalf("best beginner = %s, want 0:42", got)
	}
	if r.str("best_label.expert") != "--" || r.str("best_label.intermediate") != "--" {
		t.Error("another level has a best time")
	}
	if !regexp.MustCompile(`beginner\s+0:42`).MatchString(r.grid(90, 24)) {
		t.Errorf("the best time is not in the panel:\n%s", r.grid(90, 24))
	}
	// A slower win does not replace it; a faster one does.
	for _, c := range []struct{ secs, want int }{{60, 42}, {42, 42}, {30, 30}} {
		r.forceWin(c.secs)
		if r.str("state") != "won" || r.number("secs") != c.secs {
			t.Fatalf("a %d s win: state %s secs %d", c.secs, r.str("state"), r.number("secs"))
		}
		if got := r.a.best["beginner"]; got != c.want {
			t.Errorf("after a %d s win best = %d, want %d", c.secs, got, c.want)
		}
	}
	if got := r.str("best_label.beginner"); got != "0:30" {
		t.Errorf("the label shows %s, want 0:30", got)
	}
}

// forceWin starts a hand-made beginner game (nine mines along the top row
// and one under the corner), lets secs seconds pass, and wins it with one
// click far from the mines.
func (r *rig) forceWin(secs int) {
	r.t.Helper()
	r.play("n")
	rows := []string{"*********", "*........"}
	for len(rows) < 9 {
		rows = append(rows, ".........")
	}
	r.a.g = fromGrid(rows...)
	r.a.g.mines = 10
	r.a.last, r.a.elapsed = r.now, 0
	if err := r.a.sync(); err != nil {
		r.t.Fatal(err)
	}
	for i := 0; i < secs*5; i++ {
		r.advance(200 * time.Millisecond)
	}
	r.play(r.click(8, 8))
}

func TestCustomBoardsKeepNoBestTime(t *testing.T) {
	r := winning(t, board{"custom", 12, 10, 12}, 5)
	for _, p := range presets {
		if r.a.best[p.name] != -1 {
			t.Errorf("a custom win set the %s best time", p.name)
		}
	}
}

func TestLosingShowsTheMinesTheWrongFlagsAndTheCulprit(t *testing.T) {
	r := newRig(t)
	r.play(r.click(4, 4))
	g := r.a.g
	// A wrong flag: the first safe covered cell; a right flag: a mine.
	var safe, flagged, boom = -1, -1, -1
	for i := range g.mine {
		switch {
		case g.mine[i] && flagged < 0:
			flagged = i
		case g.mine[i] && boom < 0:
			boom = i
		case !g.mine[i] && g.st[i] == stHidden && safe < 0:
			safe = i
		}
	}
	r.play("f")
	r.clickIndex(safe)
	r.clickIndex(flagged)
	r.play("f")
	res := r.clickIndex(boom)
	if r.str("state") != "lost" || r.str("face") != ":(" || r.str("state_label") != "boom" {
		t.Fatalf("state %s face %s", r.str("state"), r.str("face"))
	}
	row := func(i int) string { rr, cc := g.pos(i); return cellID(rr, cc) }
	if !r.boolean("b."+row(boom)+".boom") || r.str("b."+row(boom)+".t") != glyphMine {
		t.Error("the mine that went off is not marked")
	}
	if !r.boolean("b."+row(safe)+".wrong") || r.str("b."+row(safe)+".t") != glyphWrong {
		t.Error("the wrong flag is not marked")
	}
	if !r.boolean("b." + row(flagged) + ".flag") {
		t.Error("a right flag was taken away")
	}
	shownMines := 0
	for i := range g.mine {
		if g.mine[i] && g.st[i] != stFlag && r.boolean("b."+row(i)+".mine") {
			shownMines++
		}
	}
	want := g.mines - 1 // all but the one flagged
	if shownMines != want {
		t.Errorf("%d mines shown, want %d", shownMines, want)
	}
	if !strings.Contains(gridOf(res), ":( boom") {
		t.Errorf("no face in the panel:\n%s", gridOf(res))
	}
	// Nothing else answers after the loss.
	shown := g.shown()
	r.play("enter", "g", r.click(0, 0))
	if g.shown() != shown {
		t.Error("the lost board answered")
	}
}

// ---- errors and notices -------------------------------------------------

func TestOnErrorShowsANoticeThatClears(t *testing.T) {
	r := newRig(t)
	err := r.a.onError(nil, errors.New("disk on fire"))
	if err != nil {
		t.Fatalf("onError = %v", err)
	}
	g := r.grid(90, 24)
	if !strings.Contains(g, "disk on fire") || strings.Contains(g, "beginner 9x9") {
		t.Errorf("the notice is not in the status line:\n%s", g)
	}
	// The next action that works clears it.
	r.play("right")
	if r.str("notice") != "" || strings.Contains(r.grid(90, 24), "disk on fire") {
		t.Error("an action did not clear the notice")
	}
	// So does the next tick.
	_ = r.a.onError(nil, errors.New("again"))
	r.advance(200 * time.Millisecond)
	if r.str("notice") != "" {
		t.Error("a tick did not clear the notice")
	}
	// A quit still quits.
	if err := r.a.onError(nil, tuimark.ErrQuit); !errors.Is(err, tuimark.ErrQuit) {
		t.Errorf("onError(ErrQuit) = %v", err)
	}
}

func TestBadClickBecomesANotice(t *testing.T) {
	r := newRig(t)
	ev := tuimark.Event{Action: "cell", Source: "c9999"}
	err := r.a.handlers()["cell"](ev)
	if err == nil {
		t.Fatal("no error")
	}
	_ = r.a.onError(&ev, err)
	if !strings.Contains(r.grid(90, 24), "not a cell") {
		t.Error("the error is not on the screen")
	}
}

// ---- determinism --------------------------------------------------------

func TestSeedDecidesTheMines(t *testing.T) {
	lay := func(seed int64) string {
		r := newRigOn(t, beginner, seed)
		r.play(r.click(4, 4))
		return r.a.g.shown()
	}
	if lay(1) != lay(1) {
		t.Error("one seed gave two boards")
	}
	if lay(1) == lay(2) {
		t.Error("two seeds gave one board")
	}
}

func TestPlayIsDeterministic(t *testing.T) {
	run := func() string {
		r := newRig(t)
		r.play(r.click(4, 4), "g", "f", "right", "enter", "n", "2")
		return strings.Join(r.playAt(90, 24).Dump.Grid, "\n")
	}
	if run() != run() {
		t.Error("two runs differ")
	}
}

// ---- the options --------------------------------------------------------

func TestOptionsResolve(t *testing.T) {
	ok := []struct {
		o    options
		want board
	}{
		{options{level: "beginner"}, board{"beginner", 9, 9, 10}},
		{options{level: "intermediate"}, board{"intermediate", 16, 16, 40}},
		{options{level: "expert"}, board{"expert", 30, 16, 99}},
		{options{cols: 9, rows: 9, mines: 71, hasCols: true, hasRows: true, hasMines: true}, board{"custom", 9, 9, 71}},
		{options{cols: 30, rows: 16, mines: 470, hasCols: true, hasRows: true, hasMines: true}, board{"custom", 30, 16, 470}},
		{options{cols: 12, rows: 10, mines: 1, hasCols: true, hasRows: true, hasMines: true}, board{"custom", 12, 10, 1}},
	}
	for _, c := range ok {
		got, err := c.o.resolve()
		if err != nil || got != c.want {
			t.Errorf("%+v: %+v, %v; want %+v", c.o, got, err, c.want)
		}
	}
	bad := []options{
		{level: "hard"}, {level: ""},
		{cols: 8, rows: 9, mines: 5, hasCols: true, hasRows: true, hasMines: true},
		{cols: 31, rows: 9, mines: 5, hasCols: true, hasRows: true, hasMines: true},
		{cols: 9, rows: 8, mines: 5, hasCols: true, hasRows: true, hasMines: true},
		{cols: 9, rows: 17, mines: 5, hasCols: true, hasRows: true, hasMines: true},
		{cols: 9, rows: 9, mines: 0, hasCols: true, hasRows: true, hasMines: true},
		{cols: 9, rows: 9, mines: 72, hasCols: true, hasRows: true, hasMines: true}, // cells - 9
		{cols: 30, rows: 16, mines: 471, hasCols: true, hasRows: true, hasMines: true},
		{cols: 9, rows: 9, mines: -3, hasCols: true, hasRows: true, hasMines: true},
		{cols: 12, hasCols: true},
		{cols: 12, rows: 10, hasCols: true, hasRows: true},
		{mines: 10, hasMines: true, level: "beginner"},
		{cols: 12, rows: 10, mines: 5, hasCols: true, hasRows: true, hasMines: true, hasLevel: true, level: "expert"},
	}
	for _, o := range bad {
		if b, err := o.resolve(); err == nil || !errors.Is(err, errUsage) {
			t.Errorf("%+v resolved to %+v, %v; want a usage error", o, b, err)
		}
	}
}

func TestRunRejectsBadFlags(t *testing.T) {
	for _, o := range []options{
		{dir: ".", level: "beginner", theme: "blue"},
		{dir: ".", level: "beginner", dump: "90"},
		{dir: ".", level: "beginner", dump: "0x24"},
		{dir: ".", level: "beginner", dump: "90x24x3"},
		{dir: ".", level: "beginner", dump: "1001x24"},
		{dir: ".", level: "beginner", demo: "-3x9"},
		{dir: ".", level: "beginner", demo: "ninety"},
		{dir: ".", level: "nope"},
	} {
		err := run(o)
		if err == nil || !errors.Is(err, errUsage) {
			t.Errorf("%+v: err = %v, want a usage error", o, err)
		}
	}
	if c, r, err := parseSize("-dump", "90x24"); err != nil || c != 90 || r != 24 {
		t.Errorf("parseSize = %d, %d, %v", c, r, err)
	}
}

// ---- the document -------------------------------------------------------

// The fit classes on #app are the ones the host publishes and sizes.tcss
// has rules for.
func TestFitClassesMatchTheGenerator(t *testing.T) {
	src, err := os.ReadFile("app.tui")
	if err != nil {
		t.Fatal(err)
	}
	var want []string
	for _, v := range fitCols() {
		want = append(want, fmt.Sprintf("nc%d", v))
	}
	for _, v := range fitRows() {
		want = append(want, fmt.Sprintf("nh%d", v))
	}
	var have []string
	for _, m := range regexp.MustCompile(`class:((?:nc|nh)\d+)="fit\.(?:nc|nh)\d+"`).FindAllStringSubmatch(string(src), -1) {
		have = append(have, m[1])
	}
	sort.Strings(want)
	sort.Strings(have)
	if strings.Join(want, " ") != strings.Join(have, " ") {
		t.Errorf("#app has %v, the host publishes %v", have, want)
	}
	r := newRig(t)
	fit, _ := r.a.ui.Get("fit")
	if m, _ := fit.(map[string]any); len(m) != len(want) {
		t.Errorf("fit has %d entries, want %d", len(m), len(want))
	}
}
