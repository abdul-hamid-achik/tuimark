package main

import (
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

// rig is an app with a fake clock: tick plays the Every function.
type rig struct {
	t   *testing.T
	a   *app
	now time.Time
}

func newRigCfg(t *testing.T, cfg config) *rig {
	t.Helper()
	a, err := load(".", cfg)
	if err != nil {
		t.Fatal(err)
	}
	r := &rig{t: t, a: a, now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	a.clock = func() time.Time { return r.now }
	return r
}

func newRig(t *testing.T) *rig { t.Helper(); return newRigCfg(t, defaultConfig()) }

// noFours is the default config with only 2s spawning, for tests that set a
// board up by hand and read its tiles back.
func noFours() config {
	c := defaultConfig()
	c.p4 = 0
	return c
}

// tick advances the clock by d and runs the Every function once.
func (r *rig) tick(d time.Duration) {
	r.t.Helper()
	r.now = r.now.Add(d)
	if err := r.a.ui.Batch(func(b *tuimark.Batch) error { return r.a.tick(b, r.now) }); err != nil {
		r.t.Fatal(err)
	}
}

// play sends keys through Play, so the handlers run as in Run.
func (r *rig) play(keys ...string) *tuimark.PlayResult { return r.playAt(90, 24, keys...) }

func (r *rig) playAt(cols, rows int, keys ...string) *tuimark.PlayResult {
	r.t.Helper()
	res, err := r.a.ui.Play(tuimark.PlayOptions{Cols: cols, Rows: rows}, keys...)
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
	return normalize(r.t, v)
}

func (r *rig) dump(cols, rows int) *tuimark.Dump {
	r.t.Helper()
	d, err := r.a.ui.Dump(cols, rows)
	if err != nil {
		r.t.Fatal(err)
	}
	return d
}

func (r *rig) grid(cols, rows int) string {
	r.t.Helper()
	return strings.Join(r.dump(cols, rows).Grid, "\n")
}

// setBoard puts rows on the board (short rows are padded with zeros),
// clears every highlight and the undo step, and publishes.
func setBoard(a *app, rows [][]int) {
	n := a.g.n
	cells := make([]int, n*n)
	for y, r := range rows {
		for x, v := range r {
			cells[y*n+x] = v
		}
	}
	a.g.cells = cells
	a.g.clearFlash()
	a.g.prev = nil
	a.g.over = !a.g.canMove()
	a.flashUntil = time.Time{}
	if err := a.sync(); err != nil {
		panic(err)
	}
}

func (r *rig) setBoard(rows ...[]int) { setBoard(r.a, rows) }

// stuckRows is a 4x4 board on which sliding right ends the game: the row
// shifts, the spawn (a 2 or a 4) fills the last gap, and no two neighbours
// are equal.
func stuckRows() [][]int {
	return [][]int{
		{8, 16, 8, 16},
		{16, 8, 16, 8},
		{8, 16, 8, 16},
		{32, 64, 128, 0},
	}
}

func tilesOf(a *app) [][]int {
	n := a.g.n
	out := make([][]int, n)
	for y := range out {
		out[y] = a.g.cells[y*n : (y+1)*n]
	}
	return out
}

func count(cells []int) int {
	c := 0
	for _, v := range cells {
		if v != 0 {
			c++
		}
	}
	return c
}

// nodeText is the text of the grid inside the rectangle of the node with id,
// so a check on a modal cannot be satisfied by the hints bar or the board
// behind it.
func nodeText(d *tuimark.Dump, id string) string {
	for _, n := range d.Nodes {
		if n.ID != id {
			continue
		}
		var lines []string
		for y := n.Y; y < n.Y+n.H && y < len(d.Grid); y++ {
			rs := []rune(d.Grid[y])
			lo, hi := n.X, n.X+n.W
			if lo > len(rs) {
				lo = len(rs)
			}
			if hi > len(rs) {
				hi = len(rs)
			}
			lines = append(lines, string(rs[lo:hi]))
		}
		return strings.Join(lines, "\n")
	}
	return ""
}

func hasClass(n tuimark.DumpNode, class string) bool {
	for _, c := range n.Classes {
		if c == class {
			return true
		}
	}
	return false
}

// withClass returns the nodes of the dump that carry class.
func withClass(d *tuimark.Dump, class string) []tuimark.DumpNode {
	var out []tuimark.DumpNode
	for _, n := range d.Nodes {
		if hasClass(n, class) {
			out = append(out, n)
		}
	}
	return out
}

// litTiles returns the board tiles (not the panel's legend swatches) that
// carry class.
func litTiles(d *tuimark.Dump, class string) []tuimark.DumpNode {
	var out []tuimark.DumpNode
	for _, n := range d.Nodes {
		if hasClass(n, "tile") && hasClass(n, class) {
			out = append(out, n)
		}
	}
	return out
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
	for _, want := range []string{"2048", "PLAYING", "manual", "score", "last gain", "best", "moves", "largest tile", "4×4", "score over moves", "merged", "new tile", "slide", "undo", "new", "auto", "help", "quit"} {
		if !strings.Contains(g, want) {
			t.Errorf("grid has no %q:\n%s", want, g)
		}
	}
}

// The first two rows of the keymap are help and quit, so <hints> keeps them
// when it runs out of room, and they do.
func TestKeymapStartsWithHelpAndQuit(t *testing.T) {
	src, err := os.ReadFile("app.tui")
	if err != nil {
		t.Fatal(err)
	}
	rows := regexp.MustCompile(`<bind keys="([^"]+)" action="([^"]+)"`).FindAllStringSubmatch(string(src), -1)
	if len(rows) < 3 || rows[0][1] != "?" || rows[0][2] != "help_open" || !strings.HasPrefix(rows[1][1], "q") || rows[1][2] != "quit" {
		t.Fatalf("keymap starts %v, want ? help_open then q quit", rows)
	}
	r := newRig(t)
	for _, width := range []int{40, 30, 24} {
		bottom := r.dump(width, 14).Grid[13]
		if !strings.Contains(bottom, "help") || !strings.Contains(bottom, "quit") {
			t.Errorf("%d cols: the hints line %q lost help or quit", width, bottom)
		}
	}
}

// Every slide key moves a lone tile to its wall and fires its action. A
// missing keymap row leaves the tile where it was and fails the case.
func TestSlideKeys(t *testing.T) {
	cases := []struct {
		keys []string
		x, y int // the wall cell the tile from (1,1) ends in
	}{
		{[]string{"left", "h", "a"}, 0, 1},
		{[]string{"right", "l", "d"}, 3, 1},
		{[]string{"up", "k", "w"}, 1, 0},
		{[]string{"down", "j", "s"}, 1, 3},
	}
	for _, c := range cases {
		for _, key := range c.keys {
			r := newRigCfg(t, noFours())
			r.setBoard(nil, []int{0, 2})
			r.play(key)
			if v := r.a.g.cells[c.y*4+c.x]; v != 2 || r.a.g.moves != 1 {
				t.Errorf("%s: wall cell (%d,%d) = %d, moves %d; cells %v", key, c.x, c.y, v, r.a.g.moves, r.a.g.cells)
			}
			if count(r.a.g.cells) != 2 {
				t.Errorf("%s: %d tiles after a move of one, want 2", key, count(r.a.g.cells))
			}
		}
	}
}

func TestSlidingFollowsTheStore(t *testing.T) {
	r := newRigCfg(t, noFours())
	r.setBoard([]int{2, 2, 4, 0})
	r.play("left")
	tiles := r.get("tiles").([]any)
	row0 := tiles[0].([]any)
	if row0[0] != float64(4) || row0[1] != float64(4) {
		t.Errorf("store row 0 = %v, want 4 4 …", row0)
	}
	if r.get("score") != float64(4) || r.get("gain") != float64(4) || r.get("moves") != float64(1) {
		t.Errorf("score %v gain %v moves %v", r.get("score"), r.get("gain"), r.get("moves"))
	}
	if r.get("gain_label") != "+4" || r.get("largest") != float64(4) {
		t.Errorf("gain_label %v largest %v", r.get("gain_label"), r.get("largest"))
	}
	g := r.grid(90, 24)
	if !strings.Contains(g, "+4") {
		t.Errorf("the last gain +4 is not on screen:\n%s", g)
	}
}

func TestNoOpKeyChangesNothing(t *testing.T) {
	r := newRig(t)
	r.setBoard([]int{2, 4, 8, 16})
	before := storeOf(t, r.a)
	r.play("left", "h", "a")
	if after := storeOf(t, r.a); !reflect.DeepEqual(before, after) {
		t.Error("a slide that moves nothing changed the store")
	}
	if r.a.g.moves != 0 || r.a.flashUntil != (time.Time{}) {
		t.Errorf("moves %d flashUntil %v after no-ops", r.a.g.moves, r.a.flashUntil)
	}
}

func TestBestScoreIsKeptInMemory(t *testing.T) {
	r := newRigCfg(t, noFours())
	r.setBoard([]int{2, 2, 0, 0})
	r.play("left")
	if r.get("best") != float64(4) {
		t.Fatalf("best = %v", r.get("best"))
	}
	r.play("r")
	if r.get("best") != float64(4) || r.get("score") != float64(0) {
		t.Errorf("after restart best %v score %v", r.get("best"), r.get("score"))
	}
}

func TestUndoKey(t *testing.T) {
	r := newRigCfg(t, noFours())
	r.setBoard([]int{2, 2, 0, 0})
	before := append([]int(nil), r.a.g.cells...)
	r.play("left")
	r.play("u")
	if !reflect.DeepEqual(r.a.g.cells, before) {
		t.Errorf("undo left %v, want %v", r.a.g.cells, before)
	}
	if r.get("score") != float64(0) || r.get("moves") != float64(0) || r.get("gain_label") != "" {
		t.Errorf("score %v moves %v gain %q", r.get("score"), r.get("moves"), r.get("gain_label"))
	}
	if r.get("best") != float64(4) {
		t.Errorf("undo lowered the best to %v", r.get("best"))
	}
	hist := r.get("hist").([]any)
	if hist[len(hist)-1] != float64(0) || hist[len(hist)-2] != nil {
		t.Errorf("hist = %v, want only the opening sample", hist)
	}
	// Only the last move can be taken back.
	r.play("u")
	if !reflect.DeepEqual(r.a.g.cells, before) {
		t.Error("a second u changed the board")
	}
	// Undo is one step, whatever came before.
	r.play("left", "up")
	moves := r.a.g.moves
	r.play("u", "u")
	if r.a.g.moves != moves-1 {
		t.Errorf("two u took back %d moves, want 1", moves-r.a.g.moves)
	}
}

func TestRestartKey(t *testing.T) {
	r := newRig(t)
	r.setBoard([]int{2, 2, 0, 0}, []int{4, 4})
	r.play("left")
	if r.a.g.score == 0 {
		t.Fatal("no score to reset")
	}
	r.play("r")
	g := r.a.g
	if g.score != 0 || g.moves != 0 || g.over || count(g.cells) != 2 || g.hasFlash() || g.canUndo() {
		t.Errorf("restart: score %d moves %d over %v tiles %d flash %v undo %v", g.score, g.moves, g.over, count(g.cells), g.hasFlash(), g.canUndo())
	}
	if hist := r.get("hist").([]any); hist[len(hist)-1] != float64(0) {
		t.Errorf("hist = %v", hist)
	}
}

// The highlights of a move are class guards that the tick clears once the
// flash time is up, and not before.
func TestHighlightsSetByTheMoveAndClearedByTheTick(t *testing.T) {
	r := newRigCfg(t, noFours())
	r.setBoard([]int{2, 2, 0, 0})
	r.play("left")

	pop := litTiles(r.dump(90, 24), "pop")
	fresh := litTiles(r.dump(90, 24), "fresh")
	if len(pop) != 1 || len(fresh) != 1 {
		t.Fatalf("%d pop and %d fresh tiles after a merge and a spawn, want 1 and 1", len(pop), len(fresh))
	}
	if r.get("pop").([]any)[0].([]any)[0] != true {
		t.Errorf("pop = %v, want the merged cell (0,0)", r.get("pop"))
	}
	flat := func(v any) (n int) {
		for _, row := range v.([]any) {
			for _, c := range row.([]any) {
				if c == true {
					n++
				}
			}
		}
		return
	}
	if flat(r.get("fresh")) != 1 {
		t.Errorf("fresh = %v", r.get("fresh"))
	}

	flash := r.a.cfg.flash
	r.tick(flash - time.Millisecond)
	if len(litTiles(r.dump(90, 24), "pop")) != 1 {
		t.Error("the highlights cleared before the flash time was up")
	}
	r.tick(time.Millisecond)
	d := r.dump(90, 24)
	if len(litTiles(d, "pop")) != 0 || len(litTiles(d, "fresh")) != 0 {
		t.Error("the tick did not clear the highlights")
	}
	if flat(r.get("pop")) != 0 || flat(r.get("fresh")) != 0 {
		t.Errorf("the store still has pop %v fresh %v", r.get("pop"), r.get("fresh"))
	}
	// The tiles themselves stay.
	if r.a.g.cells[0] != 4 || count(r.a.g.cells) != 2 {
		t.Errorf("clearing the highlights changed the board: %v", r.a.g.cells)
	}
	if r.a.cfg.flash < 150*time.Millisecond || r.a.cfg.flash > 250*time.Millisecond {
		t.Errorf("the default flash is %v, want 150 to 250 ms", r.a.cfg.flash)
	}
}

func TestEachMoveRestartsTheFlash(t *testing.T) {
	r := newRigCfg(t, noFours())
	r.setBoard([]int{2, 2, 0, 0})
	r.play("left")
	r.tick(150 * time.Millisecond)
	r.play("down")
	r.tick(150 * time.Millisecond) // 300 ms after the first move, 150 after the second
	if !r.a.g.hasFlash() {
		t.Error("the first move's timer cleared the second move's highlights")
	}
	r.tick(100 * time.Millisecond)
	if r.a.g.hasFlash() {
		t.Error("the highlights outlived their time")
	}
}

func TestZeroFlashClearsOnTheNextTick(t *testing.T) {
	c := noFours()
	c.flash = 0
	r := newRigCfg(t, c)
	r.setBoard([]int{2, 2, 0, 0})
	r.play("left")
	r.tick(tickEvery)
	if r.a.g.hasFlash() {
		t.Error("-flash 0 left highlights after a tick")
	}
}

// A tick with nothing to do publishes nothing.
func TestIdleTickWritesNothing(t *testing.T) {
	r := newRig(t)
	before := storeOf(t, r.a)
	for i := 0; i < 5; i++ {
		r.tick(tickEvery)
	}
	if after := storeOf(t, r.a); !reflect.DeepEqual(before, after) {
		t.Error("idle ticks changed the store")
	}
	if r.a.g.moves != 0 {
		t.Error("the game moved by itself")
	}
}

func TestAutopilotKeys(t *testing.T) {
	for _, key := range []string{"A", "t"} {
		r := newRig(t)
		r.play(key)
		if r.get("auto") != true || r.get("mode_label") != "autopilot" {
			t.Errorf("%s: auto=%v mode=%v", key, r.get("auto"), r.get("mode_label"))
		}
		if g := r.grid(90, 24); !strings.Contains(g, "autopilot") {
			t.Errorf("%s: the top line does not say autopilot:\n%s", key, g)
		}
		r.play(key)
		if r.get("auto") != false || r.get("mode_label") != "manual" {
			t.Errorf("%s again: auto=%v mode=%v", key, r.get("auto"), r.get("mode_label"))
		}
	}
	// a is a slide, not the autopilot.
	r := newRig(t)
	r.play("a")
	if r.get("auto") != false {
		t.Error("a toggled the autopilot; it is left")
	}
}

// The autopilot moves on the tick it is switched on, then every -speed, and
// each move is the one bestMove picks.
func TestAutopilotPaceAndChoice(t *testing.T) {
	r := newRig(t)
	r.play("A")
	var moves []int
	for i := 0; i < 7; i++ {
		want, _ := bestMove(r.a.g.n, r.a.g.cells, r.a.cfg.depth)
		cells := append([]int(nil), r.a.g.cells...)
		before := r.a.g.moves
		r.tick(tickEvery)
		moves = append(moves, r.a.g.moves)
		if r.a.g.moves != before {
			// The new board is the slide bestMove chose plus one spawn.
			slid, _, _, _ := (&game{n: 4, cells: cells}).slid(want)
			for j, v := range slid {
				if v != 0 && r.a.g.cells[j] != v {
					t.Fatalf("tick %d: tile %d is %d, bestMove's %v gives %d", i, j, r.a.g.cells[j], want, v)
				}
			}
		}
	}
	if want := []int{1, 1, 1, 2, 2, 2, 3}; !reflect.DeepEqual(moves, want) {
		t.Errorf("moves after each 50 ms tick = %v, want %v", moves, want)
	}
}

// The autopilot moves on a tick, so -speed rounds up to the next multiple of
// the 50 ms tick: 70 plays like 100, 120 like 150, and 50 moves on every tick.
func TestSpeedFlagSetsThePace(t *testing.T) {
	for _, c := range []struct {
		ms   int
		want []int // moves after each 50 ms tick
	}{
		{50, []int{1, 2, 3, 4, 5, 6}},
		{70, []int{1, 1, 2, 2, 3, 3}},
		{100, []int{1, 1, 2, 2, 3, 3}},
		{120, []int{1, 1, 1, 2, 2, 2}},
		{150, []int{1, 1, 1, 2, 2, 2}},
	} {
		cfg := defaultConfig()
		cfg.speed = time.Duration(c.ms) * time.Millisecond
		r := newRigCfg(t, cfg)
		r.play("A")
		var moves []int
		for i := 0; i < 6; i++ {
			r.tick(tickEvery)
			moves = append(moves, r.a.g.moves)
		}
		if !reflect.DeepEqual(moves, c.want) {
			t.Errorf("-speed %d: moves after each tick = %v, want %v", c.ms, moves, c.want)
		}
	}
}

func TestSlidingByHandStopsTheAutopilot(t *testing.T) {
	r := newRig(t)
	r.play("A")
	r.tick(tickEvery)
	r.play("left", "up", "right", "down") // at least one of them slides something
	if r.get("auto") != false {
		t.Error("a hand slide left the autopilot on")
	}
	moves := r.a.g.moves
	for i := 0; i < 10; i++ {
		r.tick(tickEvery)
	}
	if r.a.g.moves != moves {
		t.Error("the autopilot kept playing after the hand slide")
	}
}

// The autopilot plays a small board to the end, opens the game over modal,
// and then leaves the board alone.
func TestAutopilotPlaysToGameOver(t *testing.T) {
	c := defaultConfig()
	c.size = 3
	c.speed = 20 * time.Millisecond
	r := newRigCfg(t, c)
	r.play("A")
	for i := 0; i < 4000 && !r.a.g.over; i++ {
		r.tick(tickEvery)
		if r.a.winOpen { // 2048 on a 3x3 board is out of reach, but be safe
			break
		}
	}
	if !r.a.g.over || !r.a.overOpen || r.get("over_open") != true {
		t.Fatalf("the 3x3 game did not end: over=%v moves=%d", r.a.g.over, r.a.g.moves)
	}
	moves, cells := r.a.g.moves, append([]int(nil), r.a.g.cells...)
	for i := 0; i < 10; i++ {
		r.tick(tickEvery)
	}
	if r.a.g.moves != moves || !reflect.DeepEqual(cells, r.a.g.cells) {
		t.Error("the autopilot moved on a finished game")
	}
	if r.a.auto || r.get("mode_label") != "manual" {
		t.Error("the autopilot stayed on after the game ended")
	}
}

// help does not open over the win or game over modal, which own the keyboard.
func TestHelpDoesNotOpenOverAnotherModal(t *testing.T) {
	r := newRigCfg(t, noFours())
	r.setBoard(stuckRows()...)
	r.play("right", "?")
	if r.get("help_open") != false || r.get("over_open") != true {
		t.Errorf("? over the game over modal: help_open %v over_open %v", r.get("help_open"), r.get("over_open"))
	}
	r = newRigCfg(t, noFours())
	r.setBoard([]int{1024, 1024})
	r.play("left", "?")
	if r.get("help_open") != false || r.get("win_open") != true {
		t.Errorf("? over the win modal: help_open %v win_open %v", r.get("help_open"), r.get("win_open"))
	}
}

// The default seed opens a board every slide can move, so the mock and the
// first keys all do something.
func TestDefaultSeedOpensWithEverySlideAvailable(t *testing.T) {
	g := newGame(defaultSize, defaultSeed, defaultP4)
	for _, d := range []dir{dirUp, dirRight, dirDown, dirLeft} {
		if _, _, _, changed := g.slid(d); !changed {
			t.Errorf("%v does nothing on the opening board %v", d, g.cells)
		}
	}
}

func TestHelpModal(t *testing.T) {
	r := newRigCfg(t, noFours())
	r.setBoard(nil, []int{0, 2})
	res := r.play("?")
	if r.get("help_open") != true {
		t.Fatal("? did not open help")
	}
	g := nodeText(res.Dump, "help")
	for _, want := range []string{"closes this", "slide", "undo", "auto", "quit"} {
		if !strings.Contains(g, want) {
			t.Errorf("the help modal lacks %q:\n%s", want, g)
		}
	}
	// Nothing plays behind the help.
	r.play("left", "u", "A")
	if r.a.g.moves != 0 || r.a.auto {
		t.Errorf("keys reached the game behind help: moves %d auto %v", r.a.g.moves, r.a.auto)
	}
	r.play("?")
	if r.get("help_open") != false {
		t.Error("? did not close help")
	}
	r.play("?", "esc")
	if r.get("help_open") != false {
		t.Error("esc did not close help")
	}
	r.play("left")
	if r.a.g.moves != 1 {
		t.Error("the game did not resume after help")
	}
	// The autopilot waits behind help.
	r.play("A", "?")
	moves := r.a.g.moves
	for i := 0; i < 10; i++ {
		r.tick(tickEvery)
	}
	if r.a.g.moves != moves {
		t.Error("the autopilot moved behind help")
	}
	r.play("esc")
	r.tick(tickEvery)
	if r.a.g.moves == moves {
		t.Error("the autopilot did not resume after help")
	}
}

func TestQuitKeys(t *testing.T) {
	for _, key := range []string{"q", "ctrl+c"} {
		r := newRig(t)
		if res := r.play(key); !res.Quit {
			t.Errorf("%s did not quit", key)
		}
	}
	// q also quits from every modal.
	for name, open := range map[string]func(r *rig){
		"help": func(r *rig) { r.play("?") },
		"win": func(r *rig) {
			r.setBoard([]int{1024, 1024})
			r.play("left")
		},
		"over": func(r *rig) {
			r.setBoard(stuckRows()...)
			r.play("right")
		},
	} {
		r := newRig(t)
		open(r)
		if res := r.play("q"); !res.Quit {
			t.Errorf("q did not quit from the %s modal", name)
		}
	}
}

func TestWinModal(t *testing.T) {
	r := newRigCfg(t, noFours())
	r.setBoard([]int{1024, 1024, 0, 0}, []int{2})
	res := r.play("left")
	if r.get("win_open") != true {
		t.Fatalf("two 1024s did not open the win modal; cells %v", r.a.g.cells)
	}
	g := nodeText(res.Dump, "win")
	for _, want := range []string{"2048 in 1 moves", "keep going", "new game", "score 2048"} {
		if !strings.Contains(g, want) {
			t.Errorf("win modal lacks %q:\n%s", want, g)
		}
	}
	if r.get("state").(map[string]any)["won"] != true {
		t.Errorf("state = %v, want won", r.get("state"))
	}
	// Focus lands in the modal, on its first button.
	if res.Dump.Focus == nil || *res.Dump.Focus != "keep" {
		t.Errorf("focus = %v, want keep", res.Dump.Focus)
	}
	// Nothing slides behind the modal, and the autopilot does not start.
	moves := r.a.g.moves
	r.play("left", "right", "up", "down", "A", "u")
	if r.a.g.moves != moves || r.a.auto {
		t.Errorf("keys reached the game behind the win modal: moves %d auto %v", r.a.g.moves, r.a.auto)
	}
	// Enter on "keep going" closes it and the game goes on.
	r.play("enter")
	if r.get("win_open") != false || !r.a.g.keep {
		t.Fatalf("enter on keep going: win_open %v keep %v", r.get("win_open"), r.a.g.keep)
	}
	r.play("right")
	if r.a.g.moves != moves+1 {
		t.Error("the game did not go on after keep going")
	}
	// A later 4096 does not open the modal again.
	r.setBoard([]int{2048, 2048})
	r.play("left")
	if r.get("win_open") != false {
		t.Error("the win modal opened a second time")
	}
}

func TestWinModalEscapeKeepsGoing(t *testing.T) {
	r := newRigCfg(t, noFours())
	r.setBoard([]int{1024, 1024})
	r.play("left", "esc")
	if r.get("win_open") != false || !r.a.g.keep {
		t.Errorf("esc: win_open %v keep %v", r.get("win_open"), r.a.g.keep)
	}
}

func TestWinModalNewGameButton(t *testing.T) {
	r := newRigCfg(t, noFours())
	r.setBoard([]int{1024, 1024})
	r.play("left", "tab", "enter")
	if r.get("win_open") != false || r.a.g.score != 0 || r.a.g.moves != 0 || count(r.a.g.cells) != 2 {
		t.Errorf("new game button: win_open %v score %d moves %d tiles %d", r.get("win_open"), r.a.g.score, r.a.g.moves, count(r.a.g.cells))
	}
	if r.get("best") != float64(2048) {
		t.Errorf("best = %v, want 2048", r.get("best"))
	}
}

func TestGameOverModal(t *testing.T) {
	r := newRigCfg(t, noFours())
	r.setBoard(stuckRows()...)
	res := r.play("right")
	if r.get("over") != true || r.get("over_open") != true {
		t.Fatalf("the stuck board did not end the game: over %v over_open %v\n%v", r.get("over"), r.get("over_open"), r.a.g.cells)
	}
	g := nodeText(res.Dump, "over")
	for _, want := range []string{"GAME OVER", "largest tile 128", "undo", "new game", "esc looks at the board"} {
		if !strings.Contains(g, want) {
			t.Errorf("game over modal lacks %q:\n%s", want, g)
		}
	}
	g = strings.Join(res.Dump.Grid, "\n")
	if !strings.Contains(g, "✖ GAME OVER") {
		t.Errorf("the top line does not say GAME OVER:\n%s", g)
	}
	if len(withClass(res.Dump, "dead")) == 0 {
		t.Error("the board does not carry the dead class")
	}
	// Nothing plays on.
	cells := append([]int(nil), r.a.g.cells...)
	r.play("left", "up", "A")
	if !reflect.DeepEqual(cells, r.a.g.cells) || r.a.auto {
		t.Error("keys reached a finished game")
	}
	// esc closes the modal and leaves the board to look at; u still works.
	r.play("esc")
	if r.get("over_open") != false || r.get("over") != true {
		t.Errorf("esc: over_open %v over %v, want the modal closed and the game over", r.get("over_open"), r.get("over"))
	}
	if g := r.grid(90, 24); !strings.Contains(g, "GAME OVER") {
		t.Errorf("the top line lost GAME OVER after esc:\n%s", g)
	}
	r.play("u")
	if r.get("over") != false || count(r.a.g.cells) != 15 {
		t.Errorf("u after esc: over %v tiles %d", r.get("over"), count(r.a.g.cells))
	}
}

func TestGameOverUndoKeyAndButtons(t *testing.T) {
	// u from the open modal takes the last move back and closes it.
	r := newRigCfg(t, noFours())
	r.setBoard(stuckRows()...)
	r.play("right", "u")
	if r.get("over") != false || r.get("over_open") != false {
		t.Errorf("u: over %v over_open %v", r.get("over"), r.get("over_open"))
	}
	if !reflect.DeepEqual(tilesOf(r.a), stuckRows()) {
		t.Errorf("u did not restore the board: %v", tilesOf(r.a))
	}
	// Enter on the focused undo button does the same.
	r = newRigCfg(t, noFours())
	r.setBoard(stuckRows()...)
	res := r.play("right")
	if res.Dump.Focus == nil || *res.Dump.Focus != "takeback" {
		t.Fatalf("focus = %v, want takeback", res.Dump.Focus)
	}
	r.play("enter")
	if r.get("over") != false || !reflect.DeepEqual(tilesOf(r.a), stuckRows()) {
		t.Errorf("undo button: over %v tiles %v", r.get("over"), tilesOf(r.a))
	}
	// The new game button starts over and keeps the best score.
	r = newRigCfg(t, noFours())
	r.setBoard(stuckRows()...)
	r.play("right")
	r.a.g.best = 777
	r.play("tab", "enter")
	if r.get("over") != false || r.get("over_open") != false || r.a.g.moves != 0 || r.a.g.best != 777 {
		t.Errorf("new game button: over %v over_open %v moves %d best %d", r.get("over"), r.get("over_open"), r.a.g.moves, r.a.g.best)
	}
	// r from the open modal does it too.
	r = newRigCfg(t, noFours())
	r.setBoard(stuckRows()...)
	r.play("right", "r")
	if r.get("over") != false || r.get("over_open") != false {
		t.Errorf("r from game over: over %v over_open %v", r.get("over"), r.get("over_open"))
	}
}

// Every error from a tick or a handler shows in the top line without ending
// the game; the next action that works takes it away, and so does a tick
// once the message is old.
func TestErrorsShowInTheTopLine(t *testing.T) {
	r := newRig(t)
	if err := r.a.onError(nil, tuimark.ErrQuit); !errors.Is(err, tuimark.ErrQuit) {
		t.Fatalf("onError(ErrQuit) = %v, want ErrQuit", err)
	}
	if r.a.notice != "" {
		t.Errorf("a quit left the notice %q", r.a.notice)
	}
	for _, cols := range []int{40, 52, 80, 120} {
		if err := r.a.onError(nil, errors.New("tick: boom")); err != nil {
			t.Fatalf("onError must keep the session alive, got %v", err)
		}
		top := r.dump(cols, 24).Grid[0]
		if !strings.Contains(top, "tick: boom") || !strings.Contains(top, "PLAYING") {
			t.Errorf("%d cols: top line %q should show the error and the state", cols, top)
		}
		r.tick(time.Second) // too young to clear
		if v := r.get("notice"); v != "tick: boom" {
			t.Errorf("%d cols: a young notice was cleared by a tick: %v", cols, v)
		}
		r.tick(noticeTTL)
		if v := r.get("notice"); v != "" {
			t.Errorf("%d cols: notice = %v after the TTL", cols, v)
		}
	}
	// A handler that works clears it at once.
	if err := r.a.onError(nil, errors.New("tick: boom")); err != nil {
		t.Fatal(err)
	}
	r.play("left")
	if v := r.get("notice"); v != "" {
		t.Errorf("notice = %v after an action", v)
	}
	// And the game was never stopped.
	if r.a.g.over {
		t.Error("the game ended")
	}
}

// Determinism: the same keys on the same seed give the same store.
func TestHandlersAreDeterministic(t *testing.T) {
	keys := []string{"left", "down", "right", "up", "u", "left", "A", "r", "left", "down"}
	run := func() map[string]any {
		r := newRig(t)
		r.play(keys...)
		for i := 0; i < 10; i++ {
			r.tick(tickEvery)
		}
		return storeOf(t, r.a)
	}
	if !reflect.DeepEqual(run(), run()) {
		t.Error("two runs of the same keys differ")
	}
}

func TestSparklineFollowsTheScore(t *testing.T) {
	r := newRigCfg(t, noFours())
	r.setBoard([]int{2, 2, 0, 0}, []int{4, 4})
	r.play("left")
	hist := r.get("hist").([]any)
	if len(hist) != histLen {
		t.Fatalf("hist has %d samples, want %d", len(hist), histLen)
	}
	if hist[histLen-1] != float64(r.a.g.score) || hist[histLen-2] != float64(0) || hist[0] != nil {
		t.Errorf("hist = %v, want …nil 0 %d", hist, r.a.g.score)
	}
	// A long game squeezes into the same width and ends at the score.
	r.a.auto = true
	for i := 0; i < 300; i++ {
		r.tick(tickEvery)
		if r.a.g.over {
			break
		}
	}
	hist = r.get("hist").([]any)
	if len(hist) != histLen || hist[histLen-1] != float64(r.a.g.score) || hist[0] != float64(0) {
		t.Errorf("long hist = %v, score %d", hist, r.a.g.score)
	}
	var prev float64
	for _, v := range hist {
		if f, ok := v.(float64); ok {
			if f < prev {
				t.Errorf("the score history falls: %v", hist)
			}
			prev = f
		}
	}
}

func TestTileText(t *testing.T) {
	for v, want := range map[int]string{0: "", 2: "2", 128: "128", 2048: "2048", 65536: "65536", 131072: "131072", 1048576: "2^20", 1 << 30: "2^30"} {
		if got := tileText(v); got != want {
			t.Errorf("tileText(%d) = %q, want %q", v, got, want)
		}
	}
}

// Each tile carries exactly one value class, and the board shows its value.
func TestTilesCarryOneValueClass(t *testing.T) {
	r := newRig(t)
	r.setBoard(
		[]int{2, 4, 8, 16},
		[]int{32, 64, 128, 256},
		[]int{512, 1024, 2048, 4096},
		[]int{0, 8192, 131072, 2},
	)
	d := r.dump(120, 30)
	classOf := map[string]int{}
	for _, n := range tileNodes(d) {
		var vc []string
		for _, c := range n.Classes {
			if c == "empty" || c == "vbig" || (strings.HasPrefix(c, "v") && len(c) > 1 && c[1] >= '0' && c[1] <= '9') {
				vc = append(vc, c)
			}
		}
		if len(vc) != 1 {
			t.Errorf("tile at (%d,%d) has value classes %v", n.X, n.Y, vc)
		}
		classOf[vc[0]]++
	}
	want := map[string]int{"empty": 1, "v2": 2, "v4": 1, "v8": 1, "v16": 1, "v32": 1, "v64": 1, "v128": 1, "v256": 1, "v512": 1, "v1024": 1, "v2048": 1, "vbig": 3}
	if !reflect.DeepEqual(classOf, want) {
		t.Errorf("value classes = %v, want %v", classOf, want)
	}
	g := strings.Join(d.Grid, "\n")
	for _, s := range []string{"131072", "8192", "4096", "2048"} {
		if !strings.Contains(g, s) {
			t.Errorf("the board does not show %s:\n%s", s, g)
		}
	}
}

// bg is the background colour of the first cell of every node of class, by
// value class, from a styled dump.
func tileColours(t *testing.T, r *rig, cols, rows int) map[string]string {
	t.Helper()
	res, err := r.a.ui.Play(tuimark.PlayOptions{Cols: cols, Rows: rows, Styles: true})
	if err != nil {
		t.Fatal(err)
	}
	d := res.Dump
	out := map[string]string{}
	for _, n := range d.Nodes {
		if n.Tag != "col" {
			continue
		}
		isTile := false
		for _, c := range n.Classes {
			if c == "tile" {
				isTile = true
			}
		}
		if !isTile {
			continue
		}
		var key string
		for _, c := range n.Classes {
			if c == "pop" || c == "fresh" || c == "tile" {
				continue
			}
			key += c
		}
		for _, sp := range d.Styles[n.Y] {
			if sp.X <= n.X && n.X < sp.X+sp.W {
				v := sp.BG
				for _, a := range sp.A {
					if a == "reverse" {
						v += " reverse"
					}
				}
				out[key+"|"+classes(n.Classes)] = v
			}
		}
	}
	return out
}

func classes(cs []string) string {
	var out []string
	for _, c := range cs {
		if c == "pop" || c == "fresh" {
			out = append(out, c)
		}
	}
	return strings.Join(out, ",")
}

// Every value has its own colour in both themes, the highlights differ from
// every value, and the two themes differ.
func TestColoursFollowTheTileValues(t *testing.T) {
	rows := [][]int{
		{2, 4, 8, 16, 32, 64},
		{128, 256, 512, 1024, 2048, 4096},
		{0, 0, 0, 0, 0, 0},
	}
	byTheme := map[string]map[string]string{}
	for _, theme := range []string{"dark", "light"} {
		c := defaultConfig()
		c.size = 6
		r := newRigCfg(t, c)
		if err := r.a.ui.Set("@theme", theme); err != nil {
			t.Fatal(err)
		}
		r.setBoard(rows...)
		colours := tileColours(t, r, 120, 40)
		byTheme[theme] = colours
		seen := map[string]string{}
		for k, bg := range colours {
			cls := strings.SplitN(k, "|", 2)[0]
			if prev, dup := seen[bg]; dup && prev != cls {
				t.Errorf("%s: %s and %s share the colour %s", theme, prev, cls, bg)
			}
			seen[bg] = cls
		}
		if len(colours) < 13 {
			t.Errorf("%s: only %d tile colours found: %v", theme, len(colours), colours)
		}
		// The board colour is none of them: empty tiles read as holes.
		r.a.g.pop[0], r.a.g.fresh[7] = true, true
		_ = r.a.sync()
		hl := tileColours(t, r, 120, 40)
		var popBG, freshBG string
		for k, bg := range hl {
			switch {
			case strings.HasSuffix(k, "|pop"):
				popBG = bg
			case strings.HasSuffix(k, "|fresh"):
				freshBG = bg
			}
		}
		if !strings.Contains(popBG, "reverse") {
			t.Errorf("%s: a merged tile is %q, want it reversed", theme, popBG)
		}
		if freshBG == "" {
			t.Errorf("%s: a new tile is %q, want a colour of its own", theme, freshBG)
		}
		for k, bg := range colours {
			if bg == freshBG {
				t.Errorf("%s: the new-tile colour %s is also %s", theme, bg, k)
			}
		}
	}
	for k, dark := range byTheme["dark"] {
		if byTheme["light"][k] == dark {
			t.Errorf("%s has the same colour %s in both themes", k, dark)
		}
	}
}

// ---- layout ----

// tileNodes are the tiles of a dump: the cols with class tile.
func tileNodes(d *tuimark.Dump) []tuimark.DumpNode { return withClass(d, "tile") }

// needsFull is whether the full 7x3 tiles fit a board of side n in a
// terminal of cols x rows: the breakpoints app.tui's stylesheet states.
func needsFull(n, cols, rows int) bool {
	minCols := 9 * n
	if n == 6 {
		minCols = 84
	}
	return rows >= 4*n+3 && cols >= minCols
}

// A board is shown whole at every size where it fits (full tiles when there
// is room for them, one-row tiles after that) and every smaller size says so
// instead: never a board with tiles cut off. The sweep covers every width at
// five heights and every height at six widths, for each board size.
func TestBoardFitsOrSaysSoAtEverySize(t *testing.T) {
	type size struct{ cols, rows int }
	var sizes []size
	for _, rows := range []int{10, 14, 20, 24, 30} {
		for cols := 15; cols <= 130; cols++ {
			sizes = append(sizes, size{cols, rows})
		}
	}
	for _, cols := range []int{30, 40, 60, 80, 100, 130} {
		for rows := 6; rows <= 40; rows++ {
			sizes = append(sizes, size{cols, rows})
		}
	}
	for n := minSize; n <= maxSize; n++ {
		c := defaultConfig()
		c.size = n
		r := newRigCfg(t, c)
		for _, sz := range sizes {
			d := r.dump(sz.cols, sz.rows)
			g := strings.Join(d.Grid, "\n")
			where := fmt.Sprintf("n=%d %dx%d", n, sz.cols, sz.rows)
			if !d.OK || len(d.Errors) != 0 {
				t.Fatalf("%s: %v\n%s", where, d.Errors, g)
			}
			tiles := tileNodes(d)
			small := strings.Contains(g, "too small")
			fitsSmall := sz.cols >= 7*n+1 && sz.rows >= n+4
			if small == fitsSmall {
				t.Fatalf("%s: too-small message %v but the one-row board fits %v\n%s", where, small, fitsSmall, g)
			}
			if small {
				if len(tiles) != 0 {
					t.Fatalf("%s: tiles shown beside the too-small message", where)
				}
				continue
			}
			if len(tiles) != n*n {
				t.Fatalf("%s: %d tiles, want %d\n%s", where, len(tiles), n*n, g)
			}
			wantW, wantH := 6, 1
			if needsFull(n, sz.cols, sz.rows) {
				wantW, wantH = 7, 3
			}
			for _, tl := range tiles {
				if tl.W != wantW || tl.H != wantH {
					t.Fatalf("%s: tile %dx%d, want %dx%d", where, tl.W, tl.H, wantW, wantH)
				}
				if tl.X+tl.W > sz.cols || tl.Y+tl.H > sz.rows {
					t.Fatalf("%s: tile at (%d,%d) %dx%d leaves the screen", where, tl.X, tl.Y, tl.W, tl.H)
				}
			}
		}
	}
}

// The side panel shows from 80 columns and never beside a board it would
// cut; below 80 the top line carries the score.
func TestPanelAndCompactScore(t *testing.T) {
	r := newRig(t)
	if g := r.grid(80, 24); !strings.Contains(g, "stats") || strings.Contains(g, "score 0 best 0") {
		t.Errorf("80x24 should show the panel and no compact score:\n%s", g)
	}
	for _, cols := range []int{79, 60, 40} {
		g := r.grid(cols, 24)
		if strings.Contains(g, "stats") || !strings.Contains(g, "score 0 best 0") {
			t.Errorf("%d cols should drop the panel and show the score:\n%s", cols, g)
		}
	}
	// Short terminals drop it too: its 13 rows need 15 of the screen.
	if g := r.grid(90, 15); !strings.Contains(g, "stats") {
		t.Errorf("90x15 should still show the panel:\n%s", g)
	}
	if g := r.grid(90, 14); strings.Contains(g, "stats") || !strings.Contains(g, "score 0 best 0") {
		t.Errorf("90x14 should drop the panel and show the score:\n%s", g)
	}
}

func TestStatusLineKeepsTheStateBesideBigScores(t *testing.T) {
	for _, score := range []int{99990, 123456, 1234567} {
		for _, over := range []bool{false, true} {
			r := newRig(t)
			r.a.g.score, r.a.g.best = score, score
			r.a.g.over = over
			r.a.overOpen = false
			_ = r.a.sync()
			want := "PLAYING"
			if over {
				want = "GAME OVER"
			}
			full := fmt.Sprintf("score %d best %d", score, score)
			check := func(cols, rows int) {
				d := r.dump(cols, rows)
				top := d.Grid[0]
				if !d.OK || len(d.Errors) != 0 {
					t.Errorf("score %d %dx%d: %v\n%s", score, cols, rows, d.Errors, top)
				}
				if !strings.Contains(top, want) || !strings.Contains(top, "score ") {
					t.Errorf("score %d %dx%d: top line %q lacks %q or the score", score, cols, rows, top, want)
				}
				// Five digits fit whole from 39 columns; longer scores are
				// shortened, never the state.
				if score == 99990 && !strings.Contains(top, full) {
					t.Errorf("score %d %dx%d: top line %q lacks %q", score, cols, rows, top, full)
				}
			}
			for cols := 39; cols <= 79; cols++ {
				check(cols, 20)
			}
			// Short and wide: the panel gives way to the same status line.
			check(100, 14)
			check(120, 14)
		}
	}
}

// ---- diagnostics at every size ----

// bigGame is a late game: every value class on the board, five-digit
// totals, a long history.
func (r *rig) bigGame() {
	r.t.Helper()
	g := r.a.g
	vals := []int{2, 4, 8, 16, 32, 64, 128, 256, 512, 1024, 2048, 4096, 8192, 16384, 32768, 65536, 131072}
	for i := range g.cells {
		g.cells[i] = vals[i%len(vals)]
	}
	g.score, g.best, g.moves, g.gain = 99999, 99999, 12345, 2048
	g.hist = nil
	for i := 0; i < 300; i++ {
		g.hist = append(g.hist, i*333)
	}
	g.pop[0], g.fresh[1] = true, true
	g.won = true
	if err := r.a.sync(); err != nil {
		r.t.Fatal(err)
	}
}

// Every size the view supports is clean in both themes, for every board
// size, in the middle of a game and in the states around it, and with every
// modal open in turn with its buttons on screen.
func TestCleanAtEverySizeInEveryState(t *testing.T) {
	long := "tick: store write failed: a long message that cannot fit in the top line of a small terminal"
	states := []struct {
		name  string
		setup func(r *rig)
		want  []string // text that must be on screen at every size from 40x14
	}{
		{"start", func(r *rig) {}, nil},
		{"mid-game", func(r *rig) {
			for i := 0; i < 60; i++ {
				d, _ := bestMove(r.a.g.n, r.a.g.cells, 1)
				r.a.doMove(d, r.now)
			}
			_ = r.a.sync()
		}, nil},
		{"autopilot", func(r *rig) { r.a.auto = true; _ = r.a.sync() }, nil},
		{"big", func(r *rig) { r.bigGame() }, nil},
		{"big auto", func(r *rig) { r.a.auto = true; r.bigGame() }, nil},
		{"notice", func(r *rig) { _ = r.a.onError(nil, errors.New(long)) }, nil},
		{"notice big", func(r *rig) { r.bigGame(); _ = r.a.onError(nil, errors.New(long)) }, nil},
		{"help", func(r *rig) { r.play("?") }, []string{"quit", "closes this"}},
		{"help big", func(r *rig) { r.bigGame(); r.play("?") }, []string{"quit", "closes this"}},
		{"win", func(r *rig) { r.bigGame(); r.a.winOpen = true; _ = r.a.sync() }, []string{"keep going", "new game", "score 99999"}},
		{"over", func(r *rig) { r.bigGame(); r.a.g.over, r.a.overOpen = true, true; _ = r.a.sync() }, []string{"undo", "new game", "GAME OVER", "score 99999"}},
		{"over closed", func(r *rig) { r.bigGame(); r.a.g.over = true; _ = r.a.sync() }, []string{"GAME OVER"}},
	}
	// The modal each state opens, if any: its words must be inside it.
	modalOf := map[string]string{"help": "help", "help big": "help", "win": "win", "over": "over"}
	sizes := [][2]int{{30, 10}, {40, 14}, {60, 20}, {80, 24}, {90, 24}, {100, 30}, {120, 30}, {200, 50}}
	for n := minSize; n <= maxSize; n++ {
		for _, st := range states {
			for _, theme := range []string{"dark", "light"} {
				c := defaultConfig()
				c.size = n
				r := newRigCfg(t, c)
				if err := r.a.ui.Set("@theme", theme); err != nil {
					t.Fatal(err)
				}
				st.setup(r)
				for _, sz := range sizes {
					d := r.dump(sz[0], sz[1])
					g := strings.Join(d.Grid, "\n")
					if !d.OK || len(d.Errors) != 0 {
						t.Errorf("n=%d %s %s %dx%d: %v\n%s", n, st.name, theme, sz[0], sz[1], d.Errors, g)
					}
					if sz[0] < 40 || sz[1] < 14 {
						continue
					}
					text := g
					if m := modalOf[st.name]; m != "" {
						if text = nodeText(d, m); text == "" {
							t.Errorf("n=%d %s %s %dx%d: no modal %q on screen\n%s", n, st.name, theme, sz[0], sz[1], m, g)
						}
					}
					for _, want := range st.want {
						if !strings.Contains(text, want) {
							t.Errorf("n=%d %s %s %dx%d: no %q on screen\n%s", n, st.name, theme, sz[0], sz[1], want, g)
						}
					}
				}
			}
		}
	}
}

// The modals really open: the keys that open them, at the sizes the README
// names, with the focused button on screen and nothing cut.
func TestModalsOpenAndAreCleanAtEverySize(t *testing.T) {
	open := map[string]func(r *rig){
		"help": func(r *rig) { r.play("?") },
		"win": func(r *rig) {
			r.setBoard([]int{1024, 1024})
			r.play("left")
		},
		"over": func(r *rig) {
			r.setBoard(stuckRows()...)
			r.play("right")
		},
	}
	flag := map[string]string{"help": "help_open", "win": "win_open", "over": "over_open"}
	buttons := map[string][]string{"help": {"quit"}, "win": {"keep going", "new game"}, "over": {"undo", "new game"}}
	for name, f := range open {
		for _, theme := range []string{"dark", "light"} {
			for _, sz := range [][2]int{{40, 14}, {60, 20}, {80, 24}, {100, 30}, {120, 30}} {
				r := newRigCfg(t, noFours())
				if err := r.a.ui.Set("@theme", theme); err != nil {
					t.Fatal(err)
				}
				f(r)
				if r.get(flag[name]) != true {
					t.Fatalf("%s: the modal did not open", name)
				}
				res := r.playAt(sz[0], sz[1])
				g := strings.Join(res.Dump.Grid, "\n")
				if !res.Dump.OK || len(res.Dump.Errors) != 0 {
					t.Errorf("%s %s %dx%d: %v\n%s", name, theme, sz[0], sz[1], res.Dump.Errors, g)
				}
				inside := nodeText(res.Dump, name)
				for _, b := range buttons[name] {
					if !strings.Contains(inside, b) {
						t.Errorf("%s %s %dx%d: %q is not in the modal\n%s", name, theme, sz[0], sz[1], b, g)
					}
				}
				if name != "help" && (res.Dump.Focus == nil || !strings.Contains("keep takeback", *res.Dump.Focus)) {
					t.Errorf("%s %s %dx%d: focus = %v, want a button of the modal", name, theme, sz[0], sz[1], res.Dump.Focus)
				}
			}
		}
	}
}

// ---- flags ----

func TestFlagValidation(t *testing.T) {
	ok := defaultOptions()
	if err := ok.validate(); err != nil {
		t.Fatalf("the defaults are invalid: %v", err)
	}
	for _, size := range []int{minSize, 4, maxSize} {
		o := ok
		o.cfg.size = size
		if err := o.validate(); err != nil {
			t.Errorf("-size %d: %v", size, err)
		}
	}
	nan := math.NaN()
	inf := math.Inf(1)
	bad := map[string]func(o *options){
		"-size 2":     func(o *options) { o.cfg.size = 2 },
		"-size 7":     func(o *options) { o.cfg.size = 7 },
		"-size -4":    func(o *options) { o.cfg.size = -4 },
		"-speed 49":   func(o *options) { o.speedMs = 49 },
		"-speed 20":   func(o *options) { o.speedMs = 20 },
		"-speed 0":    func(o *options) { o.speedMs = 0 },
		"-speed 5001": func(o *options) { o.speedMs = 5001 },
		// A value that would wrap to a valid duration if multiplied first.
		"-speed 288230376151711844":  func(o *options) { o.speedMs = 288230376151711844 },
		"-speed 9223372036854775807": func(o *options) { o.speedMs = math.MaxInt64 },
		"-flash -1":                  func(o *options) { o.flashMs = -1 },
		"-flash 2001":                func(o *options) { o.flashMs = 2001 },
		"-flash 9223372036854775807": func(o *options) { o.flashMs = math.MaxInt64 },
		"-demo with -size":           func(o *options) { o.demo = "90x24"; o.gameFlags = []string{"-size"} },
		"-depth 0":                   func(o *options) { o.cfg.depth = 0 },
		"-depth 4":                   func(o *options) { o.cfg.depth = 4 },
		"-chance4 NaN":               func(o *options) { o.cfg.p4 = nan },
		"-chance4 +Inf":              func(o *options) { o.cfg.p4 = inf },
		"-chance4 -Inf":              func(o *options) { o.cfg.p4 = -inf },
		"-chance4 -0.01":             func(o *options) { o.cfg.p4 = -0.01 },
		"-chance4 1.01":              func(o *options) { o.cfg.p4 = 1.01 },
		"-theme blue":                func(o *options) { o.theme = "blue" },
	}
	for name, mutate := range bad {
		o := ok
		mutate(&o)
		err := o.validate()
		if err == nil {
			t.Errorf("%s was accepted", name)
			continue
		}
		var u usageError
		if !errors.As(err, &u) {
			t.Errorf("%s: %v is not a usage error", name, err)
		}
	}
	for _, ms := range []int{50, 70, 150, 5000} {
		o := ok
		o.speedMs = ms
		if err := o.validate(); err != nil {
			t.Errorf("-speed %d: %v", ms, err)
		}
	}
	o := ok
	o.demo = "90x24"
	o.theme = "light"
	if err := o.validate(); err != nil {
		t.Errorf("-demo with -theme: %v", err)
	}
	for _, p := range []float64{0, 0.1, 1} {
		o := ok
		o.cfg.p4 = p
		if err := o.validate(); err != nil {
			t.Errorf("-chance4 %v: %v", p, err)
		}
	}
}

// The real command line: a bad flag is a usage error (exit 2) that names
// the flag and starts nothing; good ones run.
func TestCommandLine(t *testing.T) {
	if os.Getenv("G2048_RUN_MAIN") != "" {
		args := []string{os.Args[0]}
		for i, a := range os.Args {
			if a == "--" {
				args = append(args, os.Args[i+1:]...)
				break
			}
		}
		os.Args = args
		main()
		os.Exit(0)
	}
	run := func(args ...string) (string, int) {
		cmd := exec.Command(os.Args[0], append([]string{"-test.run=TestCommandLine", "--"}, args...)...)
		cmd.Env = append(os.Environ(), "G2048_RUN_MAIN=1")
		out, err := cmd.CombinedOutput()
		code := 0
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else if err != nil {
			t.Fatal(err)
		}
		return string(out), code
	}
	for _, c := range []struct {
		args []string
		flag string
	}{
		{[]string{"-dir", ".", "-size", "2", "-dump", "80x24"}, "-size"},
		{[]string{"-dir", ".", "-size", "7", "-dump", "80x24"}, "-size"},
		{[]string{"-dir", ".", "-speed", "NaN", "-dump", "80x24"}, ""},
		{[]string{"-dir", ".", "-speed", "5", "-dump", "80x24"}, "-speed"},
		{[]string{"-dir", ".", "-flash", "99999", "-dump", "80x24"}, "-flash"},
		{[]string{"-dir", ".", "-depth", "9", "-dump", "80x24"}, "-depth"},
		{[]string{"-dir", ".", "-chance4", "NaN", "-dump", "80x24"}, "-chance4"},
		{[]string{"-dir", ".", "-chance4", "Inf", "-dump", "80x24"}, "-chance4"},
		{[]string{"-dir", ".", "-chance4", "-Inf", "-dump", "80x24"}, "-chance4"},
		{[]string{"-dir", ".", "-chance4", "2", "-dump", "80x24"}, "-chance4"},
		{[]string{"-dir", ".", "-theme", "mauve", "-dump", "80x24"}, "-theme"},
		{[]string{"-dir", ".", "-dump", "banana"}, "-dump"},
		{[]string{"-dir", ".", "-demo", "0x0"}, "-demo"},
		{[]string{"-dir", ".", "-dump", "80x24junk"}, "-dump"},
		{[]string{"-dir", ".", "-dump", "80x24x7"}, "-dump"},
		{[]string{"-dir", ".", "-dump", "+80x+24"}, "-dump"},
		{[]string{"-dir", ".", "-dump", "2000x2000"}, "-dump"},
		{[]string{"-dir", ".", "-dump", "80x1001"}, "-dump"},
		{[]string{"-dir", ".", "-speed", "288230376151711844", "-dump", "80x24"}, "-speed"},
		{[]string{"-dir", ".", "-speed", "9223372036854775807", "-dump", "80x24"}, "-speed"},
		{[]string{"-dir", ".", "-speed", "20", "-dump", "80x24"}, "-speed"},
		{[]string{"-dir", ".", "-demo", "90x24", "-size", "5"}, "-size"},
		{[]string{"-dir", ".", "-demo", "90x24", "-seed", "3"}, "-seed"},
		{[]string{"-dir", ".", "-demo", "90x24", "-autopilot"}, "-autopilot"},
	} {
		out, code := run(c.args...)
		if code != 2 || (c.flag != "" && !strings.Contains(out, c.flag)) {
			t.Errorf("%v: exit %d, output %q; want exit 2 naming %q", c.args, code, out, c.flag)
		}
	}
	// -demo takes -theme and -dir; the help text states the real ranges.
	if out, code := run("-dir", ".", "-demo", "30x10", "-theme", "light"); code != 0 || !strings.Contains(out, `"ms"`) {
		t.Errorf("-demo -theme light: exit %d\n%.200s", code, out)
	}
	if out, _ := run("-h"); !strings.Contains(out, "50 to 5000") || !strings.Contains(out, "multiple of 50") {
		t.Errorf("-h does not state the -speed range and its rounding:\n%s", out)
	}
	out, code := run("-dir", ".", "-size", "5", "-seed", "3", "-dump", "100x30")
	if code != 0 || !strings.Contains(out, `"ok": true`) || !strings.Contains(out, "5×5") {
		t.Errorf("a good command line: exit %d\n%.300s", code, out)
	}
	out, code = run("-dir", ".", "-autopilot", "-theme", "light", "-dump", "80x24")
	if code != 0 || !strings.Contains(out, "autopilot") {
		t.Errorf("-autopilot -theme light: exit %d\n%.300s", code, out)
	}
}

// -size, -seed, and -chance4 reach the game.
func TestFlagsReachTheGame(t *testing.T) {
	for n := minSize; n <= maxSize; n++ {
		c := defaultConfig()
		c.size = n
		a, err := load(".", c)
		if err != nil {
			t.Fatal(err)
		}
		if a.g.n != n || len(a.g.cells) != n*n {
			t.Errorf("-size %d built a %d board", n, a.g.n)
		}
		v, _ := a.ui.Get("size_label")
		if v != fmt.Sprintf("%d×%d", n, n) {
			t.Errorf("size_label = %v", v)
		}
	}
	c1, c2 := defaultConfig(), defaultConfig()
	c2.seed = 3
	a1, _ := load(".", c1)
	a2, _ := load(".", c2)
	if reflect.DeepEqual(a1.g.cells, a2.g.cells) {
		t.Error("two seeds opened the same board")
	}
	c := defaultConfig()
	c.p4 = 1
	a, _ := load(".", c)
	for _, v := range a.g.cells {
		if v != 0 && v != 4 {
			t.Errorf("-chance4 1 opened with a %d", v)
		}
	}
}

// A move that makes the first 2048 and ends the game opens the game over
// modal only: undo has to stay reachable from it (the win is still recorded).
func TestWinThatEndsTheGameOpensOnlyGameOver(t *testing.T) {
	rows := [][]int{
		{1024, 1024, 8, 16},
		{4, 2, 4, 8},
		{2, 4, 8, 4},
		{4, 8, 2, 16},
	}
	setup := func() (*rig, *tuimark.PlayResult) {
		r := newRigCfg(t, noFours())
		r.setBoard(rows...)
		res := r.play("left")
		if !r.a.g.over || !r.a.g.won {
			t.Fatalf("the move did not both win and end the game: over %v won %v\n%v", r.a.g.over, r.a.g.won, r.a.g.cells)
		}
		if r.get("over_open") != true || r.get("win_open") != false {
			t.Fatalf("over_open %v win_open %v, want only the game over modal", r.get("over_open"), r.get("win_open"))
		}
		return r, res
	}
	// u takes the move back and closes the modal.
	r, res := setup()
	if res.Dump.Focus == nil || *res.Dump.Focus != "takeback" {
		t.Errorf("focus = %v, want takeback", res.Dump.Focus)
	}
	r.play("u")
	if r.get("over") != false || r.get("over_open") != false || r.get("win_open") != false || !reflect.DeepEqual(tilesOf(r.a), rows) {
		t.Errorf("u: over %v over_open %v win_open %v tiles %v", r.get("over"), r.get("over_open"), r.get("win_open"), tilesOf(r.a))
	}
	// So does enter on the focused undo button.
	r, _ = setup()
	r.play("enter")
	if r.get("over") != false || r.get("over_open") != false || !reflect.DeepEqual(tilesOf(r.a), rows) {
		t.Errorf("enter on undo: over %v over_open %v tiles %v", r.get("over"), r.get("over_open"), tilesOf(r.a))
	}
}

// The win modal opens once per game: taking the winning move back and
// making 2048 again does not open it a second time; a new game re-arms it.
func TestWinModalOpensOncePerGame(t *testing.T) {
	r := newRigCfg(t, noFours())
	r.setBoard([]int{1024, 1024})
	r.play("left")
	if r.get("win_open") != true {
		t.Fatal("the first 2048 did not open the win modal")
	}
	r.play("esc", "u", "left")
	if r.get("win_open") != false {
		t.Error("the win modal opened a second time after undo and the same merge")
	}
	if r.a.g.largest() != 2048 {
		t.Fatalf("the merge did not happen again: %v", r.a.g.cells)
	}
	r.play("r")
	r.setBoard([]int{1024, 1024})
	r.play("left")
	if r.get("win_open") != true {
		t.Error("a new game did not re-arm the win modal")
	}
}

// Undoing while the autopilot plays hands the wheel back, like a slide by
// hand, instead of replaying the move at once.
func TestUndoHandsTheWheelBack(t *testing.T) {
	r := newRig(t)
	r.play("A")
	for i := 0; i < 7; i++ {
		r.tick(tickEvery)
	}
	moves := r.a.g.moves
	if moves < 2 {
		t.Fatalf("the autopilot made %d moves", moves)
	}
	r.play("u")
	if r.get("auto") != false || r.get("mode_label") != "manual" || r.a.g.moves != moves-1 {
		t.Errorf("after u: auto %v mode %v moves %d (was %d)", r.get("auto"), r.get("mode_label"), r.a.g.moves, moves)
	}
	for i := 0; i < 6; i++ {
		r.tick(tickEvery)
	}
	if r.a.g.moves != moves-1 {
		t.Errorf("the autopilot replayed after undo: moves %d", r.a.g.moves)
	}
}

func TestParseSizeIsStrict(t *testing.T) {
	for in, want := range map[string][2]int{"80x24": {80, 24}, "1x1": {1, 1}, "1000x1000": {1000, 1000}} {
		c, r, err := parseSize("-dump", in)
		if err != nil || c != want[0] || r != want[1] {
			t.Errorf("parseSize(%q) = %d, %d, %v", in, c, r, err)
		}
	}
	for _, in := range []string{"", "80", "80x", "x24", "80x24junk", "80x24x7", "+80x+24", "-80x24", " 80x24", "80x24 ", "0x24", "80x0", "1001x24", "80x1001", "2000x2000", "8e1x24", "80X24"} {
		if _, _, err := parseSize("-dump", in); err == nil {
			t.Errorf("parseSize(%q) was accepted", in)
		}
	}
}

// The line saying the board does not fit is clean and whole down to 15
// columns, for every board size.
func TestTooSmallMessageFitsNarrowTerminals(t *testing.T) {
	for n := minSize; n <= maxSize; n++ {
		c := defaultConfig()
		c.size = n
		for _, over := range []bool{false, true} { // the longest state label is GAME OVER
			r := newRigCfg(t, c)
			r.a.g.over = over
			_ = r.a.sync()
			for cols := 15; cols <= 40; cols++ {
				for _, rows := range []int{5, 6, 30} {
					d := r.dump(cols, rows)
					g := strings.Join(d.Grid, "\n")
					if !d.OK || len(d.Errors) != 0 {
						t.Fatalf("n=%d over=%v %dx%d: %v\n%s", n, over, cols, rows, d.Errors, g)
					}
					if (cols <= 7*n || rows <= n+3) && !strings.Contains(g, "too small") {
						t.Fatalf("n=%d over=%v %dx%d: the message is cut or missing\n%s", n, over, cols, rows, g)
					}
				}
			}
		}
	}
}
