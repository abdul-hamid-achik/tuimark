package main

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strconv"
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
	a, err := load(".", defaultSeed)
	if err != nil {
		t.Fatal(err)
	}
	return &rig{t: t, a: a, now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}

// step runs n ticks, each a game interval after the one before.
func (r *rig) step(n int) {
	r.t.Helper()
	for i := 0; i < n; i++ {
		r.now = r.now.Add(r.a.interval())
		if err := r.a.ui.Batch(func(b *tuimark.Batch) error { return r.a.tick(b, r.now) }); err != nil {
			r.t.Fatal(err)
		}
	}
}

// play sends keys through Play, so the handlers run as in Run.
func (r *rig) play(keys ...string) *tuimark.PlayResult {
	r.t.Helper()
	res, err := r.a.ui.Play(tuimark.PlayOptions{Cols: 90, Rows: 24}, keys...)
	if err != nil {
		r.t.Fatal(err)
	}
	return res
}

func (r *rig) head() point { return r.a.g.snake[0] }

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
	d, err := r.a.ui.Dump(90, 24)
	if err != nil {
		r.t.Fatal(err)
	}
	return strings.Join(d.Grid, "\n")
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

// The view is clean at the sizes that matter, in both themes, and in the
// middle of a game as well as at the start.
func TestDumpsAreCleanAtEverySize(t *testing.T) {
	r := newRig(t)
	r.play("A")
	r.step(30)
	for _, theme := range []string{"dark", "light"} {
		if err := r.a.ui.Set("@theme", theme); err != nil {
			t.Fatal(err)
		}
		for _, sz := range [][2]int{{80, 24}, {90, 24}, {120, 30}, {60, 20}, {52, 20}, {40, 14}, {80, 18}} {
			d, err := r.a.ui.Dump(sz[0], sz[1])
			if err != nil {
				t.Fatal(err)
			}
			if !d.OK || len(d.Errors) != 0 {
				t.Errorf("%s %dx%d: ok=%v diagnostics=%v", theme, sz[0], sz[1], d.OK, d.Errors)
			}
		}
	}
}

func TestDumpShowsTheGame(t *testing.T) {
	r := newRig(t)
	g := r.grid()
	for _, want := range []string{"snake", "PLAYING", "score", "best", "length", "level", "1 / 11", "140 ms", "steer", "pause", "quit"} {
		if !strings.Contains(g, want) {
			t.Errorf("grid has no %q:\n%s", want, g)
		}
	}
}

// Narrow terminals drop the side panel and keep the score; tiny ones say so.
func TestResponsiveLayouts(t *testing.T) {
	r := newRig(t)
	d, err := r.a.ui.Dump(60, 20)
	if err != nil {
		t.Fatal(err)
	}
	g := strings.Join(d.Grid, "\n")
	if strings.Contains(g, "stats") || !strings.Contains(g, "score 0") {
		t.Errorf("60x20 should drop the panel and show the score:\n%s", g)
	}
	d, err = r.a.ui.Dump(40, 14)
	if err != nil {
		t.Fatal(err)
	}
	if g := strings.Join(d.Grid, "\n"); !strings.Contains(g, "too small") {
		t.Errorf("40x14 should say the terminal is too small:\n%s", g)
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

// Every steering key turns the snake and fires its action. The cases start
// from a heading where the turn is legal, so a missing keymap row leaves the
// head going the old way and the case fails: up and down from heading
// right, left and right from heading up.
func TestSteeringKeys(t *testing.T) {
	cases := []struct {
		key    string
		action string
		want   point
	}{
		{"up", "up", dirUp}, {"k", "up", dirUp}, {"w", "up", dirUp},
		{"down", "down", dirDown}, {"j", "down", dirDown}, {"s", "down", dirDown},
		{"left", "left", dirLeft}, {"h", "left", dirLeft}, {"a", "left", dirLeft},
		{"right", "right", dirRight}, {"l", "right", dirRight}, {"d", "right", dirRight},
	}
	for _, c := range cases {
		r := newRig(t)
		if c.want == dirLeft || c.want == dirRight {
			r.play("up") // heading up: left and right are both legal now
			r.step(1)
		}
		before := r.head()
		res := r.play(c.key)
		if len(res.Events) != 1 || res.Events[0].Action != c.action {
			t.Errorf("%s: events = %+v, want one %s", c.key, res.Events, c.action)
		}
		r.step(1)
		if got := r.head(); got != before.add(c.want) {
			t.Errorf("%s: head %v, want %v", c.key, got, before.add(c.want))
		}
	}
}

func TestReversalIsIgnored(t *testing.T) {
	r := newRig(t)
	start := r.head()
	r.play("left") // heading right: a 180 into the body
	r.step(2)
	if got := r.head(); got != (point{start.x + 2, start.y}) {
		t.Errorf("head %v, want to go on right to %v", got, point{start.x + 2, start.y})
	}
	if r.a.g.over {
		t.Error("a reversal killed the snake")
	}
}

// Two quick taps between two moves both count: the second is judged
// against the first, not against the old heading.
func TestDoubleTapQueuesTwoTurns(t *testing.T) {
	r := newRig(t)
	start := r.head()
	r.play("up", "left")
	if n := len(r.a.g.turns); n != 2 {
		t.Fatalf("queued %d turns, want 2", n)
	}
	r.step(2)
	want := start.add(dirUp).add(dirLeft)
	if got := r.head(); got != want {
		t.Errorf("head %v, want %v (up then left)", got, want)
	}
}

func TestAtMostTwoTurnsAreQueued(t *testing.T) {
	r := newRig(t)
	r.play("up", "left", "down")
	if n := len(r.a.g.turns); n != 2 {
		t.Errorf("queued %d turns, want 2", n)
	}
	// A repeat of the heading is not queued either.
	r = newRig(t)
	r.play("right", "right")
	if n := len(r.a.g.turns); n != 0 {
		t.Errorf("queued %d turns for the current heading", n)
	}
}

func TestPauseAndResume(t *testing.T) {
	r := newRig(t)
	start := r.head()
	res := r.play("space")
	if len(res.Events) != 1 || res.Events[0].Action != "pause" {
		t.Fatalf("events = %+v, want one pause", res.Events)
	}
	if r.get("paused") != true {
		t.Error("paused not set")
	}
	if g := strings.Join(res.Dump.Grid, "\n"); !strings.Contains(g, "PAUSED") {
		t.Errorf("the status does not say PAUSED:\n%s", g)
	}
	r.step(3)
	if r.head() != start {
		t.Errorf("the snake moved while paused: %v", r.head())
	}
	// Steering is ignored while paused.
	r.play("up")
	if len(r.a.g.turns) != 0 {
		t.Error("a turn was queued while paused")
	}
	r.play("p")
	if r.get("paused") != false {
		t.Error("p did not resume")
	}
	r.step(1)
	if r.head() == start {
		t.Error("the snake did not move after resuming")
	}
	state := r.get("state").(map[string]any)
	if state["label"] != "● PLAYING" {
		t.Errorf("state = %v", state)
	}
}

func TestPlayTimeStopsWhilePaused(t *testing.T) {
	r := newRig(t)
	r.step(10) // 10 x 140ms
	r.play("space")
	r.step(20)
	r.play("space")
	r.step(1)
	if got := r.get("elapsed"); got != "0:01" { // 11 x 140ms = 1.54s
		t.Errorf("elapsed = %v, want 0:01", got)
	}
}

func TestEatingGrowsAndScores(t *testing.T) {
	r := newRig(t)
	g := r.a.g
	g.food = r.head().add(dirRight)
	if err := r.a.sync(); err != nil {
		t.Fatal(err)
	}
	r.step(1)
	if len(g.snake) != 4 || g.score != 10 || g.foods != 1 {
		t.Fatalf("length=%d score=%d foods=%d, want 4/10/1", len(g.snake), g.score, g.foods)
	}
	if r.get("length") != float64(4) || r.get("score") != float64(10) || r.get("best") != float64(10) {
		t.Errorf("store: length=%v score=%v best=%v", r.get("length"), r.get("score"), r.get("best"))
	}
	if r.get("level") != float64(2) || r.get("speed_label") != "133 ms" || r.get("level_label") != "2 / 11" {
		t.Errorf("store: level=%v speed=%v label=%v", r.get("level"), r.get("speed_label"), r.get("level_label"))
	}
	// The new food is on a free cell.
	for _, p := range g.snake {
		if p == g.food {
			t.Errorf("food %v landed on the snake", g.food)
		}
	}
}

// The ticker is re-armed to the new interval when the level changes.
func TestTickerFollowsTheLevel(t *testing.T) {
	r := newRig(t)
	var armed []time.Duration
	r.a.arm = func(d time.Duration) { armed = append(armed, d); r.a.armed = d }
	r.a.rearm()
	r.step(2)
	g := r.a.g
	g.food = r.head().add(dirRight)
	if err := r.a.sync(); err != nil {
		t.Fatal(err)
	}
	r.step(1)
	r.step(1)
	want := []time.Duration{140 * time.Millisecond, 133 * time.Millisecond}
	if len(armed) != len(want) || armed[0] != want[0] || armed[1] != want[1] {
		t.Errorf("armed %v, want %v (once per level change)", armed, want)
	}
}

func TestSpeedRisesWithTheScoreAndStops(t *testing.T) {
	g := newGame(1)
	if g.intervalMs() != 140 {
		t.Errorf("start interval %d, want 140", g.intervalMs())
	}
	prev := g.intervalMs()
	for i := 0; i < 20; i++ {
		g.foods++
		if g.intervalMs() > prev {
			t.Fatalf("interval grew at %d foods", g.foods)
		}
		prev = g.intervalMs()
	}
	if prev != 70 {
		t.Errorf("fastest interval %d, want 70", prev)
	}
}

func TestSparklineGrowsWithMoves(t *testing.T) {
	r := newRig(t)
	r.step(3)
	hist := r.get("speed_hist").([]any)
	if len(hist) != histLen {
		t.Fatalf("history has %d samples, want %d", len(hist), histLen)
	}
	if hist[histLen-1] != 7.1 || hist[histLen-3] != 7.1 || hist[histLen-4] != nil {
		t.Errorf("history = %v", hist)
	}
}

func TestGameOverByHittingAWall(t *testing.T) {
	r := newRig(t)
	// 12 columns to the right wall: 11 safe moves, the 12th hits it.
	r.step(11)
	if r.a.g.over {
		t.Fatal("game over before the wall")
	}
	r.step(1)
	if !r.a.g.over {
		t.Fatal("the wall did not end the game")
	}
	if r.get("over") != true {
		t.Error("over not published")
	}
	frozen := r.head()
	r.step(3)
	if r.head() != frozen {
		t.Error("a dead snake moved")
	}
	res := r.play()
	g := strings.Join(res.Dump.Grid, "\n")
	for _, want := range []string{"GAME OVER", "score 0", "r restart", "q quit"} {
		if !strings.Contains(g, want) {
			t.Errorf("game over modal lacks %q:\n%s", want, g)
		}
	}
	state := r.get("state").(map[string]any)
	if state["label"] != "✖ GAME OVER" || state["over"] != true {
		t.Errorf("state = %v", state)
	}
	// The dead snake looks dead: its cells carry the dead class.
	dead := 0
	for _, n := range res.Dump.Nodes {
		for _, c := range n.Classes {
			if c == "dead" {
				dead++
			}
		}
	}
	if dead == 0 {
		t.Error("no cell has the dead class")
	}
}

func TestGameOverBySelfCollision(t *testing.T) {
	g := newGame(1)
	g.snake = []point{{5, 5}, {5, 6}, {4, 6}, {4, 5}, {4, 4}, {5, 4}} // a hook
	g.dir = dirUp
	g.steer(dirLeft)
	g.move() // left onto (4,5): the body
	if !g.over {
		t.Error("running into the body did not end the game")
	}
}

// The tail's cell is free on the move that follows it.
func TestChasingTheTailIsSafe(t *testing.T) {
	g := newGame(1)
	g.snake = []point{{5, 5}, {6, 5}, {6, 6}, {5, 6}} // head above its tail
	g.dir = dirLeft
	g.food = point{0, 0}
	g.steer(dirDown)
	g.move()
	if g.over {
		t.Error("stepping onto the cell the tail leaves ended the game")
	}
}

func TestRestartAfterGameOverKeepsBest(t *testing.T) {
	r := newRig(t)
	g := r.a.g
	g.food = r.head().add(dirRight)
	if err := r.a.sync(); err != nil {
		t.Fatal(err)
	}
	r.step(1) // eat: score 10
	r.step(12)
	if !g.over {
		t.Fatal("the snake did not hit the wall")
	}
	res := r.play("r")
	if len(res.Events) != 1 || res.Events[0].Action != "restart" {
		t.Fatalf("events = %+v", res.Events)
	}
	if g.over || g.score != 0 || len(g.snake) != 3 || g.best != 10 {
		t.Errorf("over=%v score=%d length=%d best=%d", g.over, g.score, len(g.snake), g.best)
	}
	if r.get("over") != false || r.get("best") != float64(10) || r.get("elapsed") != "0:00" {
		t.Errorf("store: over=%v best=%v elapsed=%v", r.get("over"), r.get("best"), r.get("elapsed"))
	}
	r.step(1)
	if r.head() != (point{boardW/2 + 1, boardH / 2}) {
		t.Errorf("head %v after the restart", r.head())
	}
}

func TestRestartWhilePlaying(t *testing.T) {
	r := newRig(t)
	r.step(5)
	r.play("r")
	if r.head() != (point{boardW / 2, boardH / 2}) || len(r.a.g.snake) != 3 {
		t.Errorf("restart left head %v length %d", r.head(), len(r.a.g.snake))
	}
}

func TestHelpPausesAndResumes(t *testing.T) {
	r := newRig(t)
	start := r.head()
	res := r.play("?")
	if r.get("help_open") != true || r.get("paused") != true {
		t.Error("? should open help and pause")
	}
	if g := strings.Join(res.Dump.Grid, "\n"); !strings.Contains(g, "closes this") {
		t.Errorf("the help modal is not shown:\n%s", g)
	}
	r.step(2)
	if r.head() != start {
		t.Error("the snake moved behind the help")
	}
	r.play("esc")
	if r.get("help_open") != false || r.get("paused") != false {
		t.Errorf("esc should close help and resume: help=%v paused=%v", r.get("help_open"), r.get("paused"))
	}
	// A game paused by hand stays paused after help.
	r.play("p", "?", "?")
	if r.get("help_open") != false || r.get("paused") != true {
		t.Errorf("? twice should leave a hand-paused game paused: help=%v paused=%v", r.get("help_open"), r.get("paused"))
	}
}

func TestQuitKeys(t *testing.T) {
	for _, key := range []string{"q", "ctrl+c"} {
		r := newRig(t)
		if res := r.play(key); !res.Quit {
			t.Errorf("%s did not quit", key)
		}
	}
	// q also quits from the game over modal.
	r := newRig(t)
	r.step(12)
	if res := r.play("q"); !res.Quit {
		t.Error("q did not quit from game over")
	}
}

// esc leaves the game over modal open: its open= is state.over, a derived
// path the runtime cannot write, so a <tui> without version does not close
// it on esc (SPEC v0.4b §8.6 step 1, §18.2). r and q, which it names,
// still work under it.
func TestEscKeepsGameOver(t *testing.T) {
	r := newRig(t)
	r.step(12)
	if !r.a.g.over {
		t.Fatal("the snake did not hit the wall")
	}
	res := r.play("esc")
	if len(res.Events) != 0 || r.get("over") != true {
		t.Errorf("esc: events %+v, over %v", res.Events, r.get("over"))
	}
	if g := strings.Join(res.Dump.Grid, "\n"); !strings.Contains(g, "r restart") {
		t.Errorf("esc closed the game over modal:\n%s", g)
	}
	if res := r.play("r"); len(res.Events) != 1 || res.Events[0].Action != "restart" || r.a.g.over {
		t.Errorf("r after esc: events %+v, over %v", res.Events, r.a.g.over)
	}
}

func TestAutopilotKey(t *testing.T) {
	r := newRig(t)
	r.play("A")
	if r.get("auto") != true || r.get("mode_label") != "autopilot" {
		t.Errorf("auto=%v mode=%v", r.get("auto"), r.get("mode_label"))
	}
	r.step(15)
	if r.a.g.over {
		t.Error("the autopilot died in 15 moves")
	}
	// Steering by hand takes the wheel back.
	r.play("up")
	if r.get("auto") != false || r.get("mode_label") != "manual" {
		t.Errorf("auto=%v mode=%v after steering", r.get("auto"), r.get("mode_label"))
	}
	r.play("t")
	if r.get("auto") != true {
		t.Error("t should toggle the autopilot too")
	}
}

func TestAutopilotFlagStartsPlaying(t *testing.T) {
	r := newRig(t)
	r.a.auto = true
	if err := r.a.sync(); err != nil {
		t.Fatal(err)
	}
	r.step(40)
	if r.a.g.score == 0 {
		t.Error("the autopilot ate nothing in 40 moves")
	}
}

// Some seeds, a few hundred moves each: the autopilot keeps eating and
// does not trap itself in them. The run is deterministic, so this pins it.
func TestAutopilotEats(t *testing.T) {
	for seed := int64(1); seed <= 5; seed++ {
		g := newGame(seed)
		for i := 0; i < 400 && !g.over; i++ {
			g.turns = nil
			g.steer(g.autoDir())
			g.move()
		}
		if g.over || g.foods < 15 {
			t.Errorf("seed %d: over=%v foods=%d after 400 moves", seed, g.over, g.foods)
		}
	}
}

func TestSeedDecidesTheFood(t *testing.T) {
	a, b, c := newGame(7), newGame(7), newGame(8)
	if a.food != b.food {
		t.Errorf("same seed, different food: %v vs %v", a.food, b.food)
	}
	var seq1, seq2 []point
	for i := 0; i < 20; i++ {
		a.placeFood()
		c.placeFood()
		seq1, seq2 = append(seq1, a.food), append(seq2, c.food)
	}
	same := true
	for i := range seq1 {
		if seq1[i] != seq2[i] {
			same = false
		}
	}
	if same {
		t.Error("two seeds gave the same food sequence")
	}
}

func TestFoodNeverLandsOnTheSnake(t *testing.T) {
	g := newGame(3)
	for i := 0; i < 200; i++ {
		g.placeFood()
		if !g.inside(g.food) {
			t.Fatalf("food %v outside", g.food)
		}
		for _, p := range g.snake {
			if p == g.food {
				t.Fatalf("food %v on the snake", g.food)
			}
		}
	}
}

func TestFullBoardIsAWin(t *testing.T) {
	g := newGame(1)
	g.w, g.h = 3, 1
	g.snake = []point{{2, 0}, {1, 0}, {0, 0}}
	g.placeFood()
	if !g.over || !g.won {
		t.Errorf("over=%v won=%v on a full board", g.over, g.won)
	}
}

// The board is rows of runs: every row covers the board's 48 columns, and
// same-kind cells are merged.
func TestBoardRuns(t *testing.T) {
	snake := []point{{3, 2}, {2, 2}, {1, 2}, {0, 2}}
	board := buildBoard(boardW, boardH, snake, point{10, 2}, false)
	if len(board) != boardH {
		t.Fatalf("%d rows", len(board))
	}
	for y, row := range board {
		width := 0
		for _, r := range row.(map[string]any)["runs"].([]any) {
			width += len([]rune(r.(map[string]any)["t"].(string)))
		}
		if width != boardW*2 {
			t.Errorf("row %d is %d columns, want %d", y, width, boardW*2)
		}
	}
	runs := board[2].(map[string]any)["runs"].([]any)
	// body x3 merged, head, empty, food, empty = 5 runs
	if len(runs) != 5 {
		t.Fatalf("row 2 has %d runs, want 5: %v", len(runs), runs)
	}
	body := runs[0].(map[string]any)
	if body["snake"] != true || body["t"] != strings.Repeat("  ", 3) {
		t.Errorf("first run = %v, want three body cells", body)
	}
	if runs[1].(map[string]any)["head"] != true || runs[3].(map[string]any)["food"] != true {
		t.Errorf("runs = %v", runs)
	}
}

func TestHandlersAreDeterministic(t *testing.T) {
	run := func() string {
		r := newRig(t)
		r.play("A")
		r.step(60)
		return r.grid()
	}
	if run() != run() {
		t.Error("two runs of the same seed differ")
	}
}

// bgs is the set of background colours on the dump's styled rows.
func bgs(t *testing.T, r *rig, theme string) map[string]bool {
	t.Helper()
	if err := r.a.ui.Set("@theme", theme); err != nil {
		t.Fatal(err)
	}
	res, err := r.a.ui.Play(tuimark.PlayOptions{Cols: 90, Rows: 24, Styles: true})
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, row := range res.Dump.Styles {
		for _, sp := range row {
			out[sp.BG] = true
		}
	}
	return out
}

// Each kind of cell has its own colour in each theme, and a dead snake
// swaps the live colours for the dead ones.
func TestColoursFollowTheCellKinds(t *testing.T) {
	live := map[string][]string{
		"dark":  {"#2ea043", "#7ee787", "#1a1f27"}, // body, head, board
		"light": {"#2da44e", "#116329", "#e4e9ee"},
	}
	dead := map[string][]string{"dark": {"#7d3b3b", "#f85149"}, "light": {"#d8a3a3", "#a40e26"}}
	for theme, want := range live {
		r := newRig(t)
		r.step(3)
		got := bgs(t, r, theme)
		for _, c := range want {
			if !got[c] {
				t.Errorf("%s: no cell has background %s", theme, c)
			}
		}
		// Up into the top wall, in a column the game over modal does not cover.
		r.play("up")
		r.step(9)
		if !r.a.g.over {
			t.Fatal("the snake is alive")
		}
		got = bgs(t, r, theme)
		for _, c := range dead[theme] {
			if !got[c] {
				t.Errorf("%s: the dead snake lacks background %s", theme, c)
			}
		}
		for _, c := range want[:2] {
			if got[c] {
				t.Errorf("%s: the dead snake still has the live colour %s", theme, c)
			}
		}
	}
}

// bigScore puts a long game's numbers in the store: 5-digit score and best,
// a snake as long as the board allows.
func (r *rig) bigScore(score int) {
	r.t.Helper()
	g := r.a.g
	g.score, g.best = score, score
	for len(g.snake) < 200 {
		g.snake = append(g.snake, g.snake[len(g.snake)-1])
	}
	if err := r.a.sync(); err != nil {
		r.t.Fatal(err)
	}
}

// Every size the view supports (52x20 and up) is clean in both themes with
// the help open, with the game over modal, with 4- and 5-digit scores, and
// with an error in the top line. Below 52x20 the view says it is too small,
// and that is clean too.
func TestCleanAtEverySizeInEveryState(t *testing.T) {
	long := "tick: store write failed: a long message that cannot fit in the top line of a small terminal"
	states := []struct {
		name  string
		setup func(r *rig)
	}{
		{"playing", func(r *rig) {}},
		{"auto", func(r *rig) { r.a.auto = true; _ = r.a.sync() }},
		{"1000", func(r *rig) { r.bigScore(1000) }},
		{"99999 auto", func(r *rig) { r.a.auto = true; r.bigScore(99990) }},
		{"help", func(r *rig) { r.play("?") }},
		{"help 1000", func(r *rig) { r.bigScore(1000); r.play("?") }},
		{"over", func(r *rig) { r.step(12) }},
		{"over 1000", func(r *rig) { r.bigScore(1000); r.a.g.over = true; _ = r.a.sync() }},
		{"over 99999", func(r *rig) { r.bigScore(99999); r.a.g.over = true; _ = r.a.sync() }},
		{"won", func(r *rig) { r.bigScore(99999); r.a.g.over, r.a.g.won = true, true; _ = r.a.sync() }},
		{"notice", func(r *rig) { _ = r.a.onError(nil, errors.New(long)) }},
		{"notice 1000 auto", func(r *rig) { r.a.auto = true; r.bigScore(1000); _ = r.a.onError(nil, errors.New(long)) }},
	}
	sizes := [][2]int{{30, 10}, {40, 14}, {44, 20}, {51, 20}, {52, 19}}
	for c := 52; c <= 100; c++ {
		sizes = append(sizes, [2]int{c, 20})
	}
	sizes = append(sizes, [2]int{80, 24}, [2]int{90, 24}, [2]int{120, 30}, [2]int{200, 50})
	for _, st := range states {
		for _, theme := range []string{"dark", "light"} {
			r := newRig(t)
			if err := r.a.ui.Set("@theme", theme); err != nil {
				t.Fatal(err)
			}
			st.setup(r)
			for _, sz := range sizes {
				d, err := r.a.ui.Dump(sz[0], sz[1])
				if err != nil {
					t.Fatal(err)
				}
				if !d.OK || len(d.Errors) != 0 {
					t.Errorf("%s %s %dx%d: %v\n%s", st.name, theme, sz[0], sz[1], d.Errors, strings.Join(d.Grid, "\n"))
				}
			}
		}
	}
}

// The game over modal shows the whole score line whatever the digits: five
// digits of score and best, the length, at the smallest and a large size.
func TestGameOverModalShowsBigNumbers(t *testing.T) {
	for _, theme := range []string{"dark", "light"} {
		r := newRig(t)
		if err := r.a.ui.Set("@theme", theme); err != nil {
			t.Fatal(err)
		}
		r.bigScore(12340)
		r.a.g.best = 23450
		r.a.g.over = true
		if err := r.a.sync(); err != nil {
			t.Fatal(err)
		}
		for _, sz := range [][2]int{{52, 20}, {80, 24}, {120, 30}} {
			d, err := r.a.ui.Dump(sz[0], sz[1])
			if err != nil {
				t.Fatal(err)
			}
			g := strings.Join(d.Grid, "\n")
			for _, want := range []string{"score 12340", "length 200", "best 23450", "GAME OVER"} {
				if !strings.Contains(g, want) {
					t.Errorf("%s %dx%d: the modal lacks %q:\n%s", theme, sz[0], sz[1], want, g)
				}
			}
			if len(d.Errors) != 0 {
				t.Errorf("%s %dx%d: %v", theme, sz[0], sz[1], d.Errors)
			}
		}
	}
}

// The status line keeps its state label whole beside a 4-digit score at the
// narrowest width that shows the board.
func TestStatusLineKeepsTheStateWithBigScores(t *testing.T) {
	for _, want := range []string{"PLAYING", "GAME OVER"} {
		r := newRig(t)
		r.a.auto = true
		r.bigScore(9990)
		r.a.g.over = want == "GAME OVER"
		if err := r.a.sync(); err != nil {
			t.Fatal(err)
		}
		for c := 52; c <= 79; c++ {
			d, err := r.a.ui.Dump(c, 20)
			if err != nil {
				t.Fatal(err)
			}
			top := d.Grid[0]
			if !strings.Contains(top, want) || !strings.Contains(top, "score 9990 best 9990") {
				t.Errorf("%d cols: top line %q lacks %q or the score", c, top, want)
			}
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
	for _, cols := range []int{52, 80, 120} {
		if err := r.a.onError(nil, errors.New("tick: boom")); err != nil {
			t.Fatalf("onError must keep the session alive, got %v", err)
		}
		d, err := r.a.ui.Dump(cols, 24)
		if err != nil {
			t.Fatal(err)
		}
		if top := d.Grid[0]; !strings.Contains(top, "tick: boom") || !strings.Contains(top, "PLAYING") {
			t.Errorf("%d cols: top line %q should show the error and the state", cols, top)
		}
		// A tick that works clears it, even a paused one.
		r.step(1)
		if v := r.get("notice"); v != "" {
			t.Errorf("%d cols: notice = %v after a good tick", cols, v)
		}
	}
	// A handler that works clears it too.
	if err := r.a.onError(nil, errors.New("tick: boom")); err != nil {
		t.Fatal(err)
	}
	r.play("space")
	if v := r.get("notice"); v != "" {
		t.Errorf("notice = %v after an action", v)
	}
	r.play("space")
	if err := r.a.onError(nil, errors.New("tick: boom")); err != nil {
		t.Fatal(err)
	}
	r.play("space") // paused now: the tick only keeps the clock
	r.step(1)
	if v := r.get("notice"); v != "" {
		t.Errorf("notice = %v after a paused tick", v)
	}
}

// Every action mock.json names is one the document fires and the host
// handles, and a close action writes its flag false.
func TestMockActionsAreDocumentActionsWithHandlers(t *testing.T) {
	raw, err := os.ReadFile("mock.json")
	if err != nil {
		t.Fatal(err)
	}
	var mock map[string][]struct {
		Path  string `json:"path"`
		Value any    `json:"value"`
	}
	if err := json.Unmarshal(raw, &mock); err != nil {
		t.Fatal(err)
	}
	r := newRig(t)
	doc := map[string]tuimark.ActionSpec{}
	for _, spec := range r.a.ui.Catalog() {
		doc[spec.Name] = spec
	}
	for name := range mock {
		spec, ok := doc[name]
		switch {
		case !ok:
			t.Errorf("mock.json mocks %q, which the document does not name", name)
		case spec.Builtin:
			t.Errorf("mock.json mocks %q, a built-in", name)
		case !spec.Registered:
			t.Errorf("mock.json mocks %q, which has no Go handler", name)
		}
	}
	// The other way round: every host action of the document is mocked.
	for _, spec := range doc {
		if !spec.Builtin && spec.Name != "quit" {
			if _, ok := mock[spec.Name]; !ok {
				t.Errorf("the document names %q, which mock.json does not mock", spec.Name)
			}
		}
	}
	closes := false
	for _, w := range mock["help_close"] {
		if w.Path == "help_open" && w.Value == false {
			closes = true
		}
	}
	if !closes {
		t.Errorf("mock help_close = %v, want it to write help_open false", mock["help_close"])
	}
}

// fakeEvery stands in for App.Every and records the order of events.
type fakeEvery struct {
	log []string
	fn  func(*tuimark.Batch) error // the function of the registration that is live
}

func (f *fakeEvery) every(d time.Duration, fn func(*tuimark.Batch) error) func() {
	n := len(f.log)
	f.log = append(f.log, "every "+d.String())
	f.fn = fn
	return func() { f.log = append(f.log, "stop #"+strconv.Itoa(n)) }
}

// startTicker is the closure run installs: each level change stops the live
// registration before arming one of the new interval, so one is live at a
// time, and closing stops the last.
func TestStartTickerStopsThenRearms(t *testing.T) {
	r := newRig(t)
	f := &fakeEvery{}
	closeTicker := r.a.startTicker(f.every)
	if want := []string{"every 140ms"}; !reflect.DeepEqual(f.log, want) {
		t.Fatalf("log = %v, want %v", f.log, want)
	}
	// Ticks inside a level arm nothing.
	for i := 0; i < 2; i++ {
		if err := r.a.ui.Batch(f.fn); err != nil {
			t.Fatal(err)
		}
	}
	if len(f.log) != 1 {
		t.Fatalf("a tick without a level change armed: %v", f.log)
	}
	// Eating moves to level 2: the callback stops #0 and arms 133ms.
	r.a.g.food = r.head().add(dirRight)
	if err := r.a.sync(); err != nil {
		t.Fatal(err)
	}
	if err := r.a.ui.Batch(f.fn); err != nil {
		t.Fatal(err)
	}
	want := []string{"every 140ms", "stop #0", "every 133ms"}
	if !reflect.DeepEqual(f.log, want) {
		t.Fatalf("log = %v, want %v", f.log, want)
	}
	if r.a.armed != 133*time.Millisecond {
		t.Errorf("armed = %v", r.a.armed)
	}
	// A restart goes back to level 1: the same stop-then-arm.
	r.play("r")
	want = append(want, "stop #2", "every 140ms")
	if !reflect.DeepEqual(f.log, want) {
		t.Fatalf("log after restart = %v, want %v", f.log, want)
	}
	closeTicker()
	closeTicker() // closing twice stops once
	want = append(want, "stop #4")
	if !reflect.DeepEqual(f.log, want) {
		t.Errorf("log after close = %v, want %v", f.log, want)
	}
}
