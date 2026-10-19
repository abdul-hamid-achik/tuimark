package main

import (
	"errors"
	"fmt"
	"math"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

// rig is an app with a fake clock: step plays the Every function.
type rig struct {
	t   *testing.T
	a   *app
	now time.Time
}

func newRig(t *testing.T) *rig {
	t.Helper()
	a, err := load(".", defaultSeed, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	return &rig{t: t, a: a, now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}

// step runs n ticks, each one Every interval after the one before.
func (r *rig) step(n int) {
	r.t.Helper()
	for i := 0; i < n; i++ {
		r.now = r.now.Add(tickMs * time.Millisecond)
		if err := r.a.ui.Batch(func(b *tuimark.Batch) error { return r.a.tick(b, r.now) }); err != nil {
			r.t.Fatal(err)
		}
	}
}

// play sends keys through Play, so the handlers run as in Run.
func (r *rig) play(keys ...string) *tuimark.PlayResult {
	r.t.Helper()
	res, err := r.a.ui.Play(tuimark.PlayOptions{Cols: 100, Rows: 24}, keys...)
	if err != nil {
		r.t.Fatal(err)
	}
	return res
}

func (r *rig) get(path string) any {
	r.t.Helper()
	v, ok := r.a.ui.Get(path)
	if !ok {
		r.t.Fatalf("Get(%q) found nothing", path)
	}
	return v
}

func (r *rig) grid() string {
	r.t.Helper()
	d, err := r.a.ui.Dump(100, 24)
	if err != nil {
		r.t.Fatal(err)
	}
	return strings.Join(d.Grid, "\n")
}

func (r *rig) sync() {
	r.t.Helper()
	if err := r.a.sync(); err != nil {
		r.t.Fatal(err)
	}
}

// setPiece puts a piece of the given kind at the top and publishes it.
func (r *rig) setPiece(k kind) {
	r.t.Helper()
	g := r.a.g
	g.cur = piece{k: k, x: 3, y: 1}
	g.has = true
	g.lockAt, g.resets, g.rotated, g.gravAcc = -1, 0, false, 0
	g.id++
	r.sync()
}

// ground drops the piece to its resting row and publishes it.
func (r *rig) ground() {
	r.t.Helper()
	for r.a.g.softDrop() {
	}
	r.sync()
}

// gameOver stacks pieces in one column until the game ends.
func (r *rig) gameOver() {
	r.t.Helper()
	for i := 0; !r.a.g.over && i < 100; i++ {
		call(r.t, r.a, "hard_drop")
	}
	if !r.a.g.over {
		r.t.Fatal("dropping pieces in one column never ended the game")
	}
}

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

func TestDumpShowsTheGame(t *testing.T) {
	r := newRig(t)
	g := r.grid()
	for _, want := range []string{
		"tetris", "PLAYING", "manual", "stats", "score", "best", "level", "lines", "pieces", "combo", "gravity",
		"1000 ms/row", "last clear", "stack height", "hold", "next",
		"q quit", "? help", "←→ move", "↓ soft", "↑ rotate", "z back", "space drop", "c hold", "p pause", "r restart", "a auto",
	} {
		if !strings.Contains(g, want) {
			t.Errorf("grid has no %q:\n%s", want, g)
		}
	}
	// The field is 10 cells of two columns inside a wall, 20 rows, and the
	// ghost shows where the piece will land.
	if !strings.Contains(g, "░░") {
		t.Errorf("no ghost in the field:\n%s", g)
	}
}

// Every key of the keymap fires its action, and only that one. A missing
// keymap row leaves its key doing nothing, which fails here.
func TestEveryKeyFiresItsAction(t *testing.T) {
	cases := []struct{ key, action string }{
		{"left", "left"}, {"h", "left"},
		{"right", "right"}, {"l", "right"},
		{"down", "soft_drop"}, {"j", "soft_drop"},
		{"up", "rotate"}, {"x", "rotate"}, {"k", "rotate"},
		{"z", "rotate_back"},
		{"space", "hard_drop"},
		{"c", "hold"},
		{"p", "pause"},
		{"r", "restart"},
		{"a", "autopilot"},
		{"?", "help_open"},
	}
	for _, c := range cases {
		r := newRig(t)
		res := r.play(c.key)
		if len(res.Events) != 1 || res.Events[0].Action != c.action {
			t.Errorf("%s: events = %+v, want one %s", c.key, res.Events, c.action)
		}
	}
	for _, key := range []string{"q", "ctrl+c"} {
		if res := newRig(t).play(key); !res.Quit {
			t.Errorf("%s did not quit", key)
		}
	}
}

// Each action does what its name says to the game and to the store.
func TestKeysMoveThePiece(t *testing.T) {
	r := newRig(t)
	r.setPiece(kT)
	g := r.a.g

	r.play("left")
	if g.cur.x != 2 {
		t.Errorf("left: x=%d, want 2", g.cur.x)
	}
	r.play("l")
	r.play("right")
	if g.cur.x != 4 {
		t.Errorf("l and right: x=%d, want 4", g.cur.x)
	}
	r.play("up")
	if g.cur.rot != 1 {
		t.Errorf("up: rotation %d, want 1", g.cur.rot)
	}
	r.play("z")
	r.play("z")
	if g.cur.rot != 3 {
		t.Errorf("up then z z: rotation %d, want 3", g.cur.rot)
	}
	y := g.cur.y
	r.play("down")
	if g.cur.y != y+1 || g.score != 1 {
		t.Errorf("down: y=%d score=%d, want y=%d score=1", g.cur.y, g.score, y+1)
	}
	cells := r.get("cur").(map[string]any)["cells"].([]any)
	if len(cells) != 4 || r.get("cur").(map[string]any)["kind"] != "T" {
		t.Errorf("the store's piece is %v", r.get("cur"))
	}

	n := g.dropDistance()
	score := g.score
	r.play("space")
	if g.pieces != 1 || g.score != score+2*n {
		t.Errorf("space: pieces=%d score=%d, want 1 and %d", g.pieces, g.score, score+2*n)
	}
	if r.get("pieces") != 1.0 || r.get("score") != float64(g.score) {
		t.Errorf("store pieces=%v score=%v", r.get("pieces"), r.get("score"))
	}

	held := g.cur.k
	r.play("c")
	if g.hold != held || r.get("hold") != held.letter() {
		t.Errorf("c: hold=%v store=%v, want %s", g.hold, r.get("hold"), held.letter())
	}
}

func TestManualMovesAreIgnoredWhenTheyCannotCount(t *testing.T) {
	// Paused, behind the help, and over: no key moves anything.
	for _, setup := range []struct {
		name string
		do   func(r *rig)
	}{
		{"paused", func(r *rig) { r.play("p") }},
		{"help", func(r *rig) { r.play("?") }},
		{"over", func(r *rig) { r.gameOver() }},
	} {
		r := newRig(t)
		r.setPiece(kT)
		setup.do(r)
		before := *r.a.g
		// Behind a modal, space belongs to the focused button (it closes the
		// help, restarts the game), so it is not a hard drop to try.
		keys := []string{"left", "right", "down", "up", "z", "c"}
		if setup.name == "paused" {
			keys = append(keys, "space")
		}
		for _, k := range keys {
			r.play(k)
		}
		after := *r.a.g
		if before.cur != after.cur || before.score != after.score || before.pieces != after.pieces || before.hold != after.hold || before.grid != after.grid {
			t.Errorf("%s: a key changed the game: cur %+v -> %+v, score %d -> %d, pieces %d -> %d, hold %v -> %v", setup.name,
				before.cur, after.cur, before.score, after.score, before.pieces, after.pieces, before.hold, after.hold)
		}
	}
}

func TestPauseHidesTheBoardAndStopsTheClock(t *testing.T) {
	r := newRig(t)
	r.step(1)
	r.setPiece(kT)
	call(t, r.a, "hard_drop") // something on the floor to hide
	res := r.play("p")
	g := strings.Join(res.Dump.Grid, "\n")
	if !strings.Contains(g, "PAUSED") || strings.Contains(g, "░") {
		t.Errorf("the paused view:\n%s", g)
	}
	// Not a single piece, ghost, or flash run in the store's board.
	for y, row := range r.get("board").([]any) {
		for _, run := range row.(map[string]any)["runs"].([]any) {
			m := run.(map[string]any)
			for _, k := range []string{"ki", "ko", "kt", "ks", "kz", "kj", "kl", "ghost", "flash"} {
				if m[k] == true {
					t.Errorf("row %d of the paused board still has %s: %v", y, k, m)
				}
			}
		}
	}
	// A note in the middle of the board says why.
	notes := 0
	for _, row := range r.get("board").([]any) {
		for _, run := range row.(map[string]any)["runs"].([]any) {
			if run.(map[string]any)["note"] == true {
				notes++
			}
		}
	}
	if notes != 1 {
		t.Errorf("%d note runs on the paused board, want 1", notes)
	}
	// The clock stands still, however long the pause.
	t0, y := r.a.g.t, r.a.g.cur.y
	r.step(100)
	if r.a.g.t != t0 || r.a.g.cur.y != y {
		t.Errorf("game clock %d -> %d and y %d -> %d during a pause", t0, r.a.g.t, y, r.a.g.cur.y)
	}
	// Resume: the board is back, and the first tick after a long pause is
	// not play time.
	r.play("p")
	if r.get("paused") != false {
		t.Fatal("p did not resume")
	}
	r.step(1)
	if r.a.g.t-t0 > tickMs {
		t.Errorf("the pause counted as play time: %d ms", r.a.g.t-t0)
	}
	if g := r.grid(); !strings.Contains(g, "░") || strings.Contains(g, "PAUSED ‖") {
		t.Errorf("the resumed view:\n%s", g)
	}
	state := r.get("state").(map[string]any)
	if state["label"] != "● PLAYING" {
		t.Errorf("state = %v", state)
	}
}

func TestHelpPausesAndResumes(t *testing.T) {
	r := newRig(t)
	r.step(1)
	res := r.play("?")
	if r.get("help_open") != true || r.get("paused") != true {
		t.Error("? should open help and pause")
	}
	g := strings.Join(res.Dump.Grid, "\n")
	for _, want := range []string{"tetris keys", "quit", "help", "move", "soft", "rotate", "back", "drop", "hold", "pause", "restart", "auto", "close"} {
		if !strings.Contains(g, want) {
			t.Errorf("the help lacks %q:\n%s", want, g)
		}
	}
	y := r.a.g.cur.y
	r.step(30)
	if r.a.g.cur.y != y {
		t.Error("the piece fell behind the help")
	}
	r.play("esc")
	if r.get("help_open") != false || r.get("paused") != false {
		t.Errorf("esc should close help and resume: help=%v paused=%v", r.get("help_open"), r.get("paused"))
	}
	// ? again closes it too.
	r.play("?", "?")
	if r.get("help_open") != false || r.get("paused") != false {
		t.Errorf("? twice: help=%v paused=%v", r.get("help_open"), r.get("paused"))
	}
	// A game paused by hand stays paused after help.
	r.play("p", "?", "?")
	if r.get("help_open") != false || r.get("paused") != true {
		t.Errorf("? twice should leave a hand-paused game paused: help=%v paused=%v", r.get("help_open"), r.get("paused"))
	}
	// The close button closes it.
	r = newRig(t)
	r.play("?")
	res = r.play("enter")
	if len(res.Events) != 1 || res.Events[0].Action != "help_close" {
		t.Errorf("enter on the focused close button: events %+v", res.Events)
	}
	if r.get("help_open") != false {
		t.Error("the close button left the help open")
	}
	// Opening help also stops the keys that would pause or restart under it.
	r = newRig(t)
	r.play("?")
	r.play("p", "r", "a")
	if r.get("paused") != true || r.get("auto") != false || r.a.g.pieces != 0 {
		t.Error("p, r, or a acted behind the help")
	}
}

func TestGameOverModal(t *testing.T) {
	r := newRig(t)
	r.gameOver()
	res := r.play()
	g := strings.Join(res.Dump.Grid, "\n")
	for _, want := range []string{"GAME OVER", "score", "lines", "level", "best", "restart", "quit"} {
		if !strings.Contains(g, want) {
			t.Errorf("game over view lacks %q:\n%s", want, g)
		}
	}
	state := r.get("state").(map[string]any)
	if state["label"] != "✖ GAME OVER" || state["over"] != true {
		t.Errorf("state = %v", state)
	}
	// Frozen: the clock does not run and the autopilot cannot be switched on.
	t0 := r.a.g.t
	r.step(20)
	r.play("a", "p")
	if r.a.g.t != t0 || r.get("auto") != false || r.get("paused") != false {
		t.Error("the game moved after it was over")
	}
	// The first button is "board", not restart: tab then enter restarts,
	// with the best kept.
	best := r.a.g.best
	res = r.play("tab", "enter")
	if len(res.Events) != 1 || res.Events[0].Action != "restart" {
		t.Fatalf("tab enter in the game over modal: events %+v", res.Events)
	}
	if r.get("over") != false || r.get("over_modal") != false || r.a.g.pieces != 0 || r.a.g.best != best || best == 0 {
		t.Errorf("after restart: over=%v pieces=%d best=%d (was %d)", r.get("over"), r.a.g.pieces, r.a.g.best, best)
	}
	// r restarts from the modal as well, and q and the quit button quit.
	r = newRig(t)
	r.gameOver()
	r.play("r")
	if r.a.g.over {
		t.Error("r did not restart a finished game")
	}
	r = newRig(t)
	r.gameOver()
	if res := r.play("q"); !res.Quit {
		t.Error("q did not quit from game over")
	}
	r = newRig(t)
	r.gameOver()
	if res := r.play("tab", "tab", "enter"); !res.Quit {
		t.Errorf("the quit button did not quit: %+v", res.Events)
	}
}

// A player still hammering hard drop when the stack tops out must see the
// final score: space and enter fall on the first button, which only puts the
// modal away, and r restarts from there.
func TestStrayKeysDoNotRestartAFinishedGame(t *testing.T) {
	for _, key := range []string{"space", "enter"} {
		r := newRig(t)
		r.gameOver()
		score := r.a.g.score
		res := r.play(key, key, "space", "enter")
		if !r.a.g.over || r.get("over") != true || r.a.g.score != score {
			t.Fatalf("%s: a stray key restarted the game (over=%v)", key, r.get("over"))
		}
		if r.get("over_modal") != false {
			t.Errorf("%s: the modal is still up", key)
		}
		g := strings.Join(res.Dump.Grid, "\n")
		for _, want := range []string{"GAME OVER", fmt.Sprintf("%d │", score)} {
			if !strings.Contains(g, want) {
				t.Errorf("%s: with the modal away the view lacks %q:\n%s", key, want, g)
			}
		}
		if len(res.Events) == 0 || res.Events[0].Action != "over_hide" {
			t.Errorf("%s: first event %+v, want over_hide", key, res.Events)
		}
		// With the modal away, r still restarts, and q quits.
		r.play("r")
		if r.a.g.over {
			t.Errorf("%s: r did not restart after the modal was put away", key)
		}
	}
	r := newRig(t)
	r.gameOver()
	r.play("space")
	if res := r.play("q"); !res.Quit {
		t.Error("q did not quit with the game over modal away")
	}
	// A new game over brings the modal back.
	r = newRig(t)
	r.gameOver()
	r.play("space", "r")
	r.gameOver()
	if r.get("over_modal") != true {
		t.Error("the game over modal did not come back after a restart")
	}
}

// ? in the game over modal stacks the help on top of it; esc takes the help
// away, and the game over modal is still there to restart from.
func TestHelpOverGameOver(t *testing.T) {
	r := newRig(t)
	r.gameOver()
	res := r.play("?")
	if r.get("help_open") != true || r.get("over") != true {
		t.Fatalf("help=%v over=%v", r.get("help_open"), r.get("over"))
	}
	if g := strings.Join(res.Dump.Grid, "\n"); !strings.Contains(g, "tetris keys") {
		t.Errorf("no help on top:\n%s", g)
	}
	if r.get("paused") != false {
		t.Error("opening the help on a finished game paused it")
	}
	res = r.play("esc")
	if r.get("help_open") != false || r.get("over") != true {
		t.Errorf("after esc: help=%v over=%v", r.get("help_open"), r.get("over"))
	}
	if g := strings.Join(res.Dump.Grid, "\n"); !strings.Contains(g, "GAME OVER") || strings.Contains(g, "tetris keys") {
		t.Errorf("esc should leave the game over modal alone:\n%s", g)
	}
	res = r.play("tab", "enter")
	if len(res.Events) != 1 || res.Events[0].Action != "restart" || r.get("over") != false {
		t.Errorf("tab enter after the help: events %+v over %v", res.Events, r.get("over"))
	}
}

func TestRestartKeepsTheBestScore(t *testing.T) {
	r := newRig(t)
	r.play("space", "space")
	best := r.a.g.score
	if best == 0 || r.get("best") != float64(best) {
		t.Fatalf("score %d best %v", best, r.get("best"))
	}
	r.play("r")
	if r.get("score") != 0.0 || r.get("best") != float64(best) || r.get("pieces") != 0.0 {
		t.Errorf("after restart score=%v best=%v pieces=%v", r.get("score"), r.get("best"), r.get("pieces"))
	}
	// Restart also ends a pause.
	r.play("p", "r")
	if r.get("paused") != false {
		t.Error("restart left the game paused")
	}
}

// The loop counts milliseconds on the game's clock, so the tick never needs
// to change: a level change takes effect on the very next tick.
func TestGravityFollowsTheLevelWithoutRearming(t *testing.T) {
	ticksToFall := func(r *rig) int {
		r.t.Helper()
		y := r.a.g.cur.y
		for n := 1; n <= 40; n++ {
			r.step(1)
			if r.a.g.cur.y != y {
				return n
			}
		}
		return -1
	}
	r := newRig(t)
	r.step(1) // the first tick only starts the clock
	if n := ticksToFall(r); n != 1000/tickMs {
		t.Errorf("level 1: the piece fell after %d ticks, want %d", n, 1000/tickMs)
	}
	// Ten lines later it is level 2 (793 ms) and the very next row comes
	// sooner, with the same ticker.
	r = newRig(t)
	r.step(1)
	r.a.g.lines = 10
	r.sync()
	if n := ticksToFall(r); n != 16 {
		t.Errorf("level 2: the piece fell after %d ticks, want 16 (793 ms)", n)
	}
	// At the last level several rows fall in one tick.
	r = newRig(t)
	r.a.g.startLevel = maxLevel
	r.step(1)
	y := r.a.g.cur.y
	r.step(1)
	if got := r.a.g.cur.y - y; got != 7 {
		t.Errorf("level %d: %d rows in a 50 ms tick, want 7 (7 ms a row)", maxLevel, got)
	}
}

func TestAStalledTickIsNotPlayTime(t *testing.T) {
	r := newRig(t)
	r.step(1)
	t0 := r.a.g.t
	r.now = r.now.Add(time.Hour)
	if err := r.a.ui.Batch(func(b *tuimark.Batch) error { return r.a.tick(b, r.now) }); err != nil {
		t.Fatal(err)
	}
	if got := r.a.g.t - t0; got != maxStall.Milliseconds() {
		t.Errorf("an hour's stall counted as %d ms, want %d", got, maxStall.Milliseconds())
	}
}

func TestLockDelayThroughTheLoop(t *testing.T) {
	r := newRig(t)
	r.step(1)
	r.ground()
	pieces := r.a.g.pieces
	r.step(9) // 450 ms
	if r.a.g.pieces != pieces {
		t.Fatal("locked before the delay was out")
	}
	r.step(1)
	if r.a.g.pieces != pieces+1 {
		t.Error("did not lock after 500 ms")
	}
}

func TestLineClearFlashesOnTheBoardThenCollapses(t *testing.T) {
	r := newRig(t)
	r.step(1)
	g := r.a.g
	fillRows(g, 2, 9)
	g.grid[boardH-3][0] = kS
	dropI(g, 9)
	r.sync()
	call(t, r.a, "hard_drop")
	if !g.clearing {
		t.Fatal("no clear")
	}
	flashed := 0
	board := r.get("board").([]any)
	for y, row := range board {
		runs := row.(map[string]any)["runs"].([]any)
		isFlash := len(runs) == 1 && runs[0].(map[string]any)["flash"] == true
		want := y >= visibleH-2
		if isFlash != want {
			t.Errorf("board row %d flash=%v, want %v", y, isFlash, want)
		}
		if isFlash {
			flashed++
			if txt := runs[0].(map[string]any)["t"]; txt != strings.Repeat("▓▓", boardW) {
				t.Errorf("flash row text %q", txt)
			}
		}
	}
	if flashed != 2 {
		t.Errorf("%d rows flash, want 2", flashed)
	}
	if grid := r.grid(); !strings.Contains(grid, "▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓") {
		t.Errorf("the dump shows no flashing rows:\n%s", grid)
	}
	// No falling piece or ghost during the flash.
	if cur := r.get("cur").(map[string]any); cur["kind"] != "" || len(cur["cells"].([]any)) != 0 {
		t.Errorf("a piece during the flash: %v", cur)
	}
	r.step(3) // 150 ms
	if !g.clearing {
		t.Fatal("collapsed early")
	}
	r.step(1)
	if g.clearing || r.get("flash").([]any) == nil || len(r.get("flash").([]any)) != 0 {
		t.Errorf("clearing=%v flash=%v after the flash time", g.clearing, r.get("flash"))
	}
	if r.get("lines") != 2.0 || r.get("last") != "DOUBLE" || r.get("score") != 300.0 {
		t.Errorf("lines=%v last=%v score=%v, want 2, DOUBLE, 300", r.get("lines"), r.get("last"), r.get("score"))
	}
	if grid := r.grid(); !strings.Contains(grid, "last clear") || !strings.Contains(grid, "DOUBLE") || !strings.Contains(grid, "lines              2") {
		t.Errorf("the stats do not show the clear:\n%s", grid)
	}
	// The collapsed S fell to the floor in the published field.
	field := r.get("field").([]any)
	if row := field[boardH-1].(string); row[0:1] != "S" {
		t.Errorf("floor row %q, want the S fallen into column 0", row)
	}
}

// The sparkline's history is the stack height after each lock, newest last,
// padded with nulls on the left.
func TestHeightHistory(t *testing.T) {
	r := newRig(t)
	hist := r.get("height_hist").([]any)
	if len(hist) != histLen {
		t.Fatalf("history has %d samples, want %d", len(hist), histLen)
	}
	for i, v := range hist {
		if v != nil {
			t.Fatalf("sample %d = %v at the start, want null", i, v)
		}
	}
	r.play("space", "space", "space")
	hist = r.get("height_hist").([]any)
	if hist[histLen-4] != nil || hist[histLen-3] == nil || hist[histLen-1] == nil {
		t.Errorf("history after three locks = %v", hist)
	}
	for i := histLen - 3; i < histLen; i++ {
		if h := hist[i].(float64); h < 1 || h > 20 {
			t.Errorf("height %v out of range in %v", h, hist[histLen-3:])
		}
	}
	if !strings.Contains(r.grid(), "last 20 pieces") {
		t.Error("the sparkline has no caption")
	}
	r.play("r")
	for _, v := range r.get("height_hist").([]any) {
		if v != nil {
			t.Fatalf("restart left a history: %v", r.get("height_hist"))
		}
	}
}

func TestAutopilotKey(t *testing.T) {
	r := newRig(t)
	r.play("a")
	if r.get("auto") != true || r.get("mode_label") != "autopilot" {
		t.Errorf("auto=%v mode=%v", r.get("auto"), r.get("mode_label"))
	}
	if !strings.Contains(r.grid(), "autopilot") {
		t.Error("the top line does not say autopilot")
	}
	r.play("a")
	if r.get("auto") != false || r.get("mode_label") != "manual" {
		t.Errorf("auto=%v mode=%v after the second a", r.get("auto"), r.get("mode_label"))
	}
	// Playing by hand takes the wheel back.
	r.play("a")
	r.play("left")
	if r.get("auto") != false || r.get("mode_label") != "manual" {
		t.Errorf("auto=%v mode=%v after a move", r.get("auto"), r.get("mode_label"))
	}
}

func TestAutopilotActsOnItsOwnPace(t *testing.T) {
	sig := func(r *rig) string { return fmt.Sprint(r.a.g.cur, r.a.g.pieces) }
	r := newRig(t)
	r.step(1)
	r.play("a")
	// 100 ms between acts at pace 1: every second 50 ms tick.
	changed := []bool{}
	last := sig(r)
	for i := 0; i < 8; i++ {
		r.step(1)
		now := sig(r)
		changed = append(changed, now != last)
		last = now
	}
	for i, c := range changed {
		if want := i%2 == 1; c != want {
			t.Errorf("tick %d changed=%v, want %v (changes: %v)", i+1, c, want, changed)
		}
	}
	// Twice the pace: an act every tick.
	a, err := load(".", defaultSeed, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if a.autoMs != 50 {
		t.Errorf("autoMs at pace 2 = %d, want 50", a.autoMs)
	}
	if got := autoMs(1); got != 100 {
		t.Errorf("autoMs(1) = %d", got)
	}
}

func TestAutopilotPlaysTheGameAndLooksAlive(t *testing.T) {
	r := newRig(t)
	r.step(1)
	r.play("a")
	moved := map[string]bool{}
	for i := 0; i < 400; i++ {
		r.step(1)
		moved[fmt.Sprint(r.a.g.cur)] = true
	}
	if r.a.g.over {
		t.Fatal("the autopilot lost in 400 ticks")
	}
	if r.a.g.pieces < 10 || r.a.g.score == 0 {
		t.Errorf("pieces=%d score=%d after 20 s of autopilot", r.a.g.pieces, r.a.g.score)
	}
	if len(moved) < 50 {
		t.Errorf("the piece was in only %d distinct places", len(moved))
	}
	if r.get("pieces") != float64(r.a.g.pieces) {
		t.Error("the store lags the game")
	}
}

func TestAutopilotFlagStartsPlaying(t *testing.T) {
	r := newRig(t)
	r.a.auto = true
	r.sync()
	r.step(60)
	if r.a.g.pieces == 0 {
		t.Error("the autopilot placed nothing in 3 s")
	}
}

func TestHandlersAreDeterministic(t *testing.T) {
	run := func() string {
		r := newRig(t)
		r.play("a")
		r.step(300)
		return r.grid() + fmt.Sprint(r.a.g.score)
	}
	if run() != run() {
		t.Error("two runs of the same seed differ")
	}
}

func TestHoldPanel(t *testing.T) {
	r := newRig(t)
	rows := func(path string) []any { return r.get(path).([]any) }
	filled := func(path string) int {
		n := 0
		for _, row := range rows(path) {
			for _, run := range row.(map[string]any)["runs"].([]any) {
				if !run.(map[string]any)["empty"].(bool) {
					n += len([]rune(run.(map[string]any)["t"].(string)))
				}
			}
		}
		return n
	}
	if filled("hold_rows") != 0 {
		t.Error("the hold panel is not empty at the start")
	}
	first := r.a.g.cur.k
	r.play("c")
	if filled("hold_rows") != 8 {
		t.Errorf("hold panel has %d filled columns, want 8 (a piece)", filled("hold_rows"))
	}
	dim := func() bool {
		for _, row := range rows("hold_rows") {
			for _, run := range row.(map[string]any)["runs"].([]any) {
				if run.(map[string]any)["dim"] == true {
					return true
				}
			}
		}
		return false
	}
	if !dim() || r.get("hold_used") != true {
		t.Error("a used hold is not dimmed")
	}
	// The second hold of the same piece changes nothing.
	cur := r.a.g.cur
	r.play("c")
	if r.a.g.cur != cur || r.a.g.hold != first {
		t.Error("a second hold swapped again")
	}
	r.play("space")
	if dim() || r.get("hold_used") != false {
		t.Error("the hold stays dimmed after a lock")
	}
}

func TestNextPanelShowsTheQueue(t *testing.T) {
	r := newRig(t)
	next := r.get("next_rows").([]any)
	if len(next) != queueLen*3-1 {
		t.Fatalf("next panel has %d rows, want %d", len(next), queueLen*3-1)
	}
	queue := r.get("queue").([]any)
	if len(queue) != queueLen {
		t.Fatalf("queue = %v", queue)
	}
	flag := map[string]string{"I": "ki", "O": "ko", "T": "kt", "S": "ks", "Z": "kz", "J": "kj", "L": "kl"}
	for i, q := range queue {
		filled := 0
		for _, row := range next[i*3 : i*3+2] {
			for _, run := range row.(map[string]any)["runs"].([]any) {
				m := run.(map[string]any)
				if m["empty"] == true {
					continue
				}
				if m[flag[q.(string)]] != true {
					t.Errorf("piece %d (%s) has a run of the wrong colour: %v", i, q, m)
				}
				filled += len([]rune(m["t"].(string)))
			}
		}
		if filled != 8 {
			t.Errorf("piece %d (%s) is %d columns of blocks, want 8", i, q, filled)
		}
	}
	// Taking a piece moves the queue along.
	second := queue[0].(string)
	r.play("space")
	if got := r.a.g.cur.k.letter(); got != second {
		t.Errorf("the piece after a drop is %s, the queue promised %s", got, second)
	}
	if r.get("queue").([]any)[0] == queue[0] && r.get("queue").([]any)[1] != queue[1] {
		t.Error("queue not shifted")
	}
}

// decodeBoard reads the board's runs back into one kind per cell.
func decodeBoard(t *testing.T, board []any) [visibleH][boardW]ck {
	t.Helper()
	var out [visibleH][boardW]ck
	if len(board) != visibleH {
		t.Fatalf("board has %d rows, want %d", len(board), visibleH)
	}
	for y, row := range board {
		x := 0
		for _, run := range row.(map[string]any)["runs"].([]any) {
			m := run.(map[string]any)
			n := len([]rune(m["t"].(string))) / 2
			c := ckEmpty
			for i, f := range []string{"ki", "ko", "kt", "ks", "kz", "kj", "kl"} {
				if m[f] == true {
					c = ck(i + 1)
				}
			}
			if m["ghost"] == true {
				c = ckGhost
			}
			if m["flash"] == true {
				c = ckFlash
			}
			for i := 0; i < n; i++ {
				out[y][x+i] = c
			}
			x += n
		}
		if x != boardW {
			t.Fatalf("row %d is %d cells wide", y, x)
		}
	}
	return out
}

// The board in the store is the game, row for row, in every state a game of
// autopilot goes through.
func TestBoardIsTheGame(t *testing.T) {
	r := newRig(t)
	r.step(1)
	r.play("a")
	flashes := 0
	for i := 0; i < 500; i++ {
		r.step(1)
		g := r.a.g
		got := decodeBoard(t, r.get("board").([]any))
		var want [visibleH][boardW]ck
		for y := hidden; y < boardH; y++ {
			for x := 0; x < boardW; x++ {
				want[y-hidden][x] = ck(g.grid[y][x])
			}
		}
		for _, y := range g.rows {
			for x := 0; x < boardW; x++ {
				want[y-hidden][x] = ckFlash
			}
		}
		for _, c := range g.ghost() {
			if c.y >= hidden {
				want[c.y-hidden][c.x] = ckGhost
			}
		}
		if g.canAct() {
			for _, c := range g.cur.cells() {
				if c.y >= hidden {
					want[c.y-hidden][c.x] = ck(g.cur.k)
				}
			}
		}
		if got != want {
			t.Fatalf("tick %d: the board differs from the game\n got %v\nwant %v", i, got, want)
		}
		if g.clearing {
			flashes++
		}
	}
	if flashes == 0 {
		t.Error("500 ticks of autopilot never cleared a line: the flash was not exercised")
	}
}

// bgAt is the background colour of the cell at (x, y) in the dump.
func bgAt(res *tuimark.PlayResult, x, y int) string {
	for _, sp := range res.Dump.Styles[y] {
		if x >= sp.X && x < sp.X+sp.W {
			return sp.BG
		}
	}
	return ""
}

func fgAt(res *tuimark.PlayResult, x, y int) string {
	for _, sp := range res.Dump.Styles[y] {
		if x >= sp.X && x < sp.X+sp.W {
			return sp.FG
		}
	}
	return ""
}

// Every kind of cell has its own colour in each theme.
func TestColoursFollowTheCellKinds(t *testing.T) {
	palette := map[string]map[string]string{
		"dark": {
			"I": "#3fc8e0", "O": "#f2cc4a", "T": "#b370ee", "S": "#5bd16f", "Z": "#f0626a", "J": "#5b8cf0", "L": "#f09a4b",
			"flash": "#f4f7fb", "board": "#171b22", "ghost": "#6a7380",
		},
		"light": {
			"I": "#0e8ea3", "O": "#c49a06", "T": "#8a46cf", "S": "#2a9a43", "Z": "#cf3540", "J": "#2f5fd0", "L": "#d0701a",
			"flash": "#1f2328", "board": "#e4e9ee", "ghost": "#7a8592",
		},
	}
	for theme, want := range palette {
		r := newRig(t)
		if err := r.a.ui.Set("@theme", theme); err != nil {
			t.Fatal(err)
		}
		g := r.a.g
		// One of each kind on the floor, a ghost from the falling piece.
		for i, k := range kinds {
			g.grid[boardH-1][i] = k
		}
		g.grid[boardH-2][9] = kNone
		g.rows = []int{boardH - 4}
		g.clearing = false
		r.setPiece(kT)
		g.cur.x = 6
		r.sync()
		res, err := r.a.ui.Play(tuimark.PlayOptions{Cols: 100, Rows: 24, Styles: true})
		if err != nil {
			t.Fatal(err)
		}
		var fieldX, fieldY int
		for _, n := range res.Dump.Nodes {
			if n.ID == "field" {
				fieldX, fieldY = n.X, n.Y
			}
		}
		cell := func(x, y int) (string, string) {
			return fgAt(res, fieldX+1+2*x, fieldY+1+y), bgAt(res, fieldX+1+2*x, fieldY+1+y)
		}
		for i, k := range kinds {
			if _, bg := cell(i, visibleH-1); bg != want[k.letter()] {
				t.Errorf("%s: the %s block has background %s, want %s", theme, k.letter(), bg, want[k.letter()])
			}
		}
		// The falling T is a T; the ghost has the ghost colour on the board.
		fg, bg := cell(7, 1)
		_ = fg
		if bg == "" {
			t.Fatal("no style for the piece cell")
		}
		ghostY := visibleH - 2 - 0
		if fg, bg := cell(7, ghostY); fg != want["ghost"] || bg != want["board"] {
			t.Errorf("%s: ghost cell fg=%s bg=%s, want fg=%s on the board colour %s", theme, fg, bg, want["ghost"], want["board"])
		}
		// Flashing rows: put them in the field and look.
		g.clearing = true
		for x := 0; x < boardW; x++ {
			g.grid[boardH-4][x] = kJ
		}
		r.sync()
		res, err = r.a.ui.Play(tuimark.PlayOptions{Cols: 100, Rows: 24, Styles: true})
		if err != nil {
			t.Fatal(err)
		}
		if _, bg := cell(3, visibleH-4); bg != want["flash"] {
			t.Errorf("%s: a flashing cell has background %s, want %s", theme, bg, want["flash"])
		}
	}
}

func TestResponsiveLayouts(t *testing.T) {
	r := newRig(t)
	dump := func(c, rows int) string {
		d, err := r.a.ui.Dump(c, rows)
		if err != nil {
			t.Fatal(err)
		}
		return strings.Join(d.Grid, "\n")
	}
	g := dump(100, 24)
	for _, want := range []string{"stats", "hold", "next", "stack height"} {
		if !strings.Contains(g, want) {
			t.Errorf("100x24 lacks %q", want)
		}
	}
	// 73 columns: no room for the stats; the top line carries the score.
	g = dump(73, 24)
	if strings.Contains(g, "stats") || !strings.Contains(g, "score 0  lines 0  level 1") || !strings.Contains(g, "next") {
		t.Errorf("73x24 should drop the stats, keep the previews, and show the score on top:\n%s", g)
	}
	// 47 columns: the field alone, with the score.
	g = dump(47, 24)
	if strings.Contains(g, "next") || strings.Contains(g, "hold") || !strings.Contains(g, "┏━") || !strings.Contains(g, "PLAYING") {
		t.Errorf("47x24 should be the field and the state:\n%s", g)
	}
	// The top line carries all three numbers from 70 columns, the score alone
	// from 32, and nothing of the sort below: the state is always there.
	top := func(c int) string { g := dump(c, 24); return g[:strings.Index(g, "\n")] }
	for _, c := range []struct {
		cols                       int
		score, lines, level, state bool
	}{
		{73, true, true, true, true}, {70, true, true, true, true},
		{69, true, false, false, true}, {48, true, false, false, true}, {32, true, false, false, true},
		{31, false, false, false, true}, {24, false, false, false, true},
	} {
		line := top(c.cols)
		for _, w := range []struct {
			text string
			want bool
		}{{"score 0", c.score}, {"lines 0", c.lines}, {"level 1", c.level}, {"PLAYING", c.state}} {
			if strings.Contains(line, w.text) != w.want {
				t.Errorf("%d columns: top line %q: %q present = %v, want %v", c.cols, line, w.text, !w.want, w.want)
			}
		}
	}
	// Too small says so.
	for _, sz := range [][2]int{{100, 23}, {60, 20}, {40, 14}, {23, 24}} {
		g = dump(sz[0], sz[1])
		if !strings.Contains(g, "too small") || strings.Contains(g, "┏━") {
			t.Errorf("%dx%d should say the terminal is too small:\n%s", sz[0], sz[1], g)
		}
	}
	g = dump(24, 24)
	if strings.Contains(g, "too small") || !strings.Contains(g, "┏━") {
		t.Errorf("24x24 is the smallest that shows the field:\n%s", g)
	}
}

// <hints> drops the last rows when it runs out of room, so quit comes first
// in the keymap and help second.
func TestKeymapOrder(t *testing.T) {
	src, err := os.ReadFile("app.tui")
	if err != nil {
		t.Fatal(err)
	}
	binds := regexp.MustCompile(`<bind keys="([^"]+)" action="([^"]+)"`).FindAllStringSubmatch(string(src), -1)
	if len(binds) < 3 {
		t.Fatalf("found %d keymap rows", len(binds))
	}
	if binds[0][2] != "quit" || binds[1][2] != "help_open" {
		t.Errorf("the keymap starts with %s then %s, want quit then help_open", binds[0][2], binds[1][2])
	}
	// And the hints of a very narrow terminal keep exactly those two.
	r := newRig(t)
	d, err := r.a.ui.Dump(24, 24)
	if err != nil {
		t.Fatal(err)
	}
	last := d.Grid[len(d.Grid)-1]
	if !strings.HasPrefix(strings.TrimSpace(last), "q quit  ? help") {
		t.Errorf("24-column hints = %q, want them to start with q quit and ? help", last)
	}
}

func TestHintsKeepQuitAndHelp(t *testing.T) {
	r := newRig(t)
	for c := 24; c <= 60; c += 2 {
		d, err := r.a.ui.Dump(c, 24)
		if err != nil {
			t.Fatal(err)
		}
		last := d.Grid[len(d.Grid)-1]
		if !strings.Contains(last, "q quit") || !strings.Contains(last, "? help") {
			t.Errorf("%d columns: hints %q lose quit or help", c, last)
		}
	}
}

func TestHandlersCoverCatalog(t *testing.T) {
	r := newRig(t)
	names := map[string]bool{}
	for _, spec := range r.a.ui.Catalog() {
		names[spec.Name] = true
		if !spec.Builtin && !spec.Registered {
			t.Errorf("no handler for %s (%v)", spec.Name, spec.Sources)
		}
	}
	// And every handler is used by the document: no dead code.
	for name := range r.a.handlers() {
		if !names[name] {
			t.Errorf("handler %s is never named by the document", name)
		}
	}
}

// An error from a tick or a handler shows in the top line without ending
// the game, and the next tick or action that works takes it away. A quit
// still quits.
func TestErrorsShowInTheTopLine(t *testing.T) {
	r := newRig(t)
	if err := r.a.onError(nil, tuimark.ErrQuit); !errors.Is(err, tuimark.ErrQuit) {
		t.Fatalf("onError(ErrQuit) = %v, want ErrQuit", err)
	}
	if r.a.notice != "" {
		t.Errorf("a quit left the notice %q", r.a.notice)
	}
	for _, cols := range []int{24, 60, 100, 140} {
		if err := r.a.onError(nil, errors.New("tick: boom")); err != nil {
			t.Fatalf("onError must keep the session alive, got %v", err)
		}
		d, err := r.a.ui.Dump(cols, 24)
		if err != nil {
			t.Fatal(err)
		}
		top := d.Grid[0]
		if !strings.Contains(top, "tick: boom") || !strings.Contains(top, "PLAYING") {
			t.Errorf("%d cols: top line %q should show the error and the state", cols, top)
		}
		r.step(1)
		if v := r.get("notice"); v != "" {
			t.Errorf("%d cols: notice = %v after a good tick", cols, v)
		}
	}
	// Every handler that works clears it too.
	for _, key := range []string{"left", "p", "c", "a", "?", "r"} {
		r := newRig(t)
		if err := r.a.onError(nil, errors.New("boom")); err != nil {
			t.Fatal(err)
		}
		r.play(key)
		if v := r.get("notice"); v != "" {
			t.Errorf("notice = %v after %s", v, key)
		}
	}
	// A paused tick clears it as well.
	r = newRig(t)
	r.play("p")
	_ = r.a.onError(nil, errors.New("boom"))
	r.step(1)
	if v := r.get("notice"); v != "" {
		t.Errorf("notice = %v after a paused tick", v)
	}
}

func TestOptionsValidate(t *testing.T) {
	ok := options{level: 1, pace: 1}
	if err := ok.validate(); err != nil {
		t.Fatal(err)
	}
	if err := (options{level: maxLevel, pace: maxPace, theme: "light", dump: "100x24"}).validate(); err != nil {
		t.Fatal(err)
	}
	for name, bad := range map[string]func(o *options){
		"level 0":       func(o *options) { o.level = 0 },
		"level -3":      func(o *options) { o.level = -3 },
		"level 16":      func(o *options) { o.level = maxLevel + 1 },
		"pace 0":        func(o *options) { o.pace = 0 },
		"pace negative": func(o *options) { o.pace = -1 },
		"pace NaN":      func(o *options) { o.pace = math.NaN() },
		"pace +Inf":     func(o *options) { o.pace = math.Inf(1) },
		"pace -Inf":     func(o *options) { o.pace = math.Inf(-1) },
		"pace tiny":     func(o *options) { o.pace = 0.01 },
		"pace huge":     func(o *options) { o.pace = 11 },
		"theme":         func(o *options) { o.theme = "blue" },
		"dump size":     func(o *options) { o.dump = "100" },
		"demo size":     func(o *options) { o.demo = "0x24" },
		"both":          func(o *options) { o.dump, o.demo = "100x24", "100x24" },
	} {
		o := ok
		bad(&o)
		if o.validate() == nil {
			t.Errorf("%s should not validate", name)
		}
	}
	// The error says which flag.
	o := ok
	o.pace = math.NaN()
	if err := o.validate(); err == nil || !strings.Contains(err.Error(), "-pace") {
		t.Errorf("error = %v, want it to name -pace", err)
	}
}

func TestParseSize(t *testing.T) {
	if c, r, err := parseSize("-dump", "90x24"); err != nil || c != 90 || r != 24 {
		t.Errorf("parseSize = %d %d %v", c, r, err)
	}
	for _, bad := range []string{"", "90", "x24", "0x24", "90x0", "ax b", "90x24junk", "-5x24", "+90x24", "1001x24", "90x1001", "90 x24", "90x24x1"} {
		if _, _, err := parseSize("-dump", bad); err == nil {
			t.Errorf("parseSize(%q) should fail", bad)
		}
	}
}

func TestRunRejectsBadFlags(t *testing.T) {
	for _, o := range []options{
		{dir: ".", level: 0, pace: 1},
		{dir: ".", level: 1, pace: math.NaN()},
		{dir: ".", level: 1, pace: 1, theme: "x"},
	} {
		if err := run(o); err == nil {
			t.Errorf("run(%+v) should fail before anything starts", o)
		}
	}
}
