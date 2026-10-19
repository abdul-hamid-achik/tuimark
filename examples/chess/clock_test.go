package main

import (
	"strings"
	"testing"
	"time"
)

func TestFmtClock(t *testing.T) {
	for ms, want := range map[int64]string{
		300000: "5:00", 299001: "5:00", 299000: "4:59", 59000: "0:59", 20000: "0:20", 19999: "19.9",
		9500: "9.5", 100: "0.1", 0: "0.0", -5: "0.0", 3600000: "1:00:00", 3661000: "1:01:01",
	} {
		if got := fmtClock(ms); got != want {
			t.Errorf("fmtClock(%d) = %q, want %q", ms, got, want)
		}
	}
}

func TestParseClock(t *testing.T) {
	good := map[string]clockSpec{
		"none": {}, "": {}, "off": {}, "5+0": {300000, 0}, "3+2": {180000, 2000}, "10+5": {600000, 5000},
		"0.5+0": {30000, 0}, "15": {900000, 0}, " 1+1 ": {60000, 1000}, "0.1+0": {6000, 0}, "180+60": {10800000, 60000},
		"2.5+3": {150000, 3000},
	}
	for in, want := range good {
		got, err := parseClock(in)
		if err != nil || got != want {
			t.Errorf("parseClock(%q) = %+v, %v, want %+v", in, got, err, want)
		}
	}
	for _, in := range []string{
		"NaN", "NaN+1", "5+NaN", "Inf", "5+Inf", "+Inf+0", "-Inf+0", "1e999+0", "5+1e999", "abc", "5+x", "x+5",
		"0+0", "0+5", "-1+0", "5+-1", "0.05+0", "181+0", "1000+0", "5+61", "+5",
		// Nothing is silently rounded or read in another base.
		"5+1.5", "5+0.0001", "0x1p-2+0", "0x10+0", "1.25+0", "5.+0", ".5+0", "5+", "1e1+0", "1_0+0", "00000+0", "5+0x1", "५+0",
	} {
		if got, err := parseClock(in); err == nil {
			t.Errorf("parseClock(%q) = %+v, want an error", in, got)
		}
	}
	// What a spec prints reads back.
	for _, s := range []string{"5+0", "3+2", "10+5", "0.5+0", "none", "2.5+3", "0.1+0"} {
		c, _ := parseClock(s)
		if back, err := parseClock(c.String()); err != nil || back != c || c.String() != s {
			t.Errorf("%q -> %q -> %+v, %v", s, c.String(), back, err)
		}
	}
	for _, choice := range clockChoices {
		if _, err := parseClock(choice); err != nil {
			t.Errorf("the dialog offers %q, which does not parse: %v", choice, err)
		}
	}
}

func clockMs(t *testing.T, a *testApp) (w, b int64) {
	t.Helper()
	c := a.get(t, "clock").(map[string]any)
	return int64(c["w"].(float64)), int64(c["b"].(float64))
}

// clocked is a two-player game with a time control and the two opening
// moves played: each side's clock starts after its own first move, so now
// white's is running.
func clocked(t *testing.T, spec string) *testApp {
	a := newTestApp(t, twoPlayers, func(o *options) { o.clock = spec })
	a.moves(t, "e4", "e5")
	return a
}

func TestEachClockStartsAfterItsOwnFirstMove(t *testing.T) {
	a := newTestApp(t, twoPlayers, func(o *options) { o.clock = "1+0" })
	a.tickFor(t, 5*time.Second)
	if w, b := clockMs(t, a); w != 60000 || b != 60000 {
		t.Fatalf("a clock ran before white's first move: %d %d", w, b)
	}
	if run := a.get(t, "clock").(map[string]any)["run"]; run != "" {
		t.Errorf("run = %v before any move", run)
	}
	a.moves(t, "e4")
	a.tickFor(t, 5*time.Second)
	if w, b := clockMs(t, a); w != 60000 || b != 60000 {
		t.Fatalf("a clock ran before black's first move: %d %d", w, b)
	}
	a.moves(t, "e5")
	a.tickFor(t, 2*time.Second)
	if w, b := clockMs(t, a); w != 58000 || b != 60000 {
		t.Errorf("after both first moves: %d %d, want white's clock running", w, b)
	}
	if run := a.get(t, "clock").(map[string]any)["run"]; run != "white" {
		t.Errorf("run = %v", run)
	}
}

func TestClockTicksForTheSideToMove(t *testing.T) {
	a := clocked(t, "1+0")
	if w, b := clockMs(t, a); w != 60000 || b != 60000 {
		t.Fatalf("clocks = %d %d", w, b)
	}
	g := grid(a.play(t))
	if !strings.Contains(g, "1:00") {
		t.Errorf("no clock on screen:\n%s", g)
	}
	a.tickAfter(t, time.Second)
	a.tickAfter(t, 500*time.Millisecond)
	if w, b := clockMs(t, a); w != 58500 || b != 60000 {
		t.Errorf("after 1.5 s: %d %d", w, b)
	}
	if got := a.get(t, "bot").(map[string]any)["clock"]; got != "0:59" {
		t.Errorf("bot clock = %v", got)
	}
	a.moves(t, "Nf3")
	a.tickFor(t, 2*time.Second)
	if w, b := clockMs(t, a); w != 58500 || b != 58000 {
		t.Errorf("after black's 2 s: %d %d", w, b)
	}
	if run := a.get(t, "clock").(map[string]any)["run"]; run != "black" {
		t.Errorf("run = %v", run)
	}
	// A long stall is not thinking time.
	a.tickAfter(t, time.Hour)
	if _, b := clockMs(t, a); b != 57000 {
		t.Errorf("a stall cost %d ms", 58000-b)
	}
}

// The time since the last tick is charged to the side that moves, at the
// move, not to the side that moves next.
func TestTheClockIsSettledAtTheMove(t *testing.T) {
	a := clocked(t, "5+0")
	a.tickAfter(t, 50*time.Millisecond)
	a.clock.Advance(90 * time.Millisecond)
	a.moves(t, "Nf3")
	a.tickAfter(t, 10*time.Millisecond)
	if w, b := clockMs(t, a); w != 300000-140 || b != 300000-10 {
		t.Errorf("white has %d and black %d, want white charged 140 ms and black 10", 300000-w, 300000-b)
	}
}

// A flag that fell between the last tick and the move ends the game; the
// move is not played.
func TestAFlagThatFellBeforeTheMoveEndsTheGame(t *testing.T) {
	a := clocked(t, "0.1+0")
	a.tickFor(t, 5900*time.Millisecond)
	if a.g.Over() {
		t.Fatal("the game ended early")
	}
	a.clock.Advance(500 * time.Millisecond)
	a.moves(t, "Nf3")
	if a.str(t, "result") != "0-1" || a.str(t, "reason") != "time forfeit" {
		t.Errorf("result %s %s", a.str(t, "result"), a.str(t, "reason"))
	}
	if len(a.g.SAN) != 2 {
		t.Errorf("the move was played after the flag fell: %v", a.g.SAN)
	}
	if w, _ := clockMs(t, a); w != 0 {
		t.Errorf("white has %d ms", w)
	}
}

func TestClockIncrement(t *testing.T) {
	a := clocked(t, "3+2")
	// The first move of each side earned nothing.
	if w, b := clockMs(t, a); w != 180000 || b != 180000 {
		t.Fatalf("clocks after the first moves: %d %d", w, b)
	}
	a.tickFor(t, 5*time.Second)
	a.moves(t, "Nf3")
	if w, _ := clockMs(t, a); w != 180000-5000+2000 {
		t.Errorf("white has %d ms after a move with an increment", w)
	}
	a.tickFor(t, 3*time.Second)
	a.moves(t, "Nc6")
	if _, b := clockMs(t, a); b != 180000-3000+2000 {
		t.Errorf("black has %d ms", b)
	}
	// Undo gives the time back as it stood.
	a.play(t, "u")
	if w, b := clockMs(t, a); w != 177000 || b != 180000 {
		t.Errorf("after undo: %d %d", w, b)
	}
	a.play(t, "U")
	if _, b := clockMs(t, a); b != 179000 {
		t.Errorf("after redo: %d", b)
	}
}

func TestFlagFallLosesOnTime(t *testing.T) {
	a := clocked(t, "0.1+0")
	for i := 0; i < 5; i++ {
		a.tickAfter(t, time.Second)
	}
	if a.g.Over() {
		t.Fatal("the game ended with a second left")
	}
	a.tickAfter(t, time.Second)
	if a.str(t, "result") != "0-1" || a.str(t, "reason") != "time forfeit" {
		t.Fatalf("result %s %s", a.str(t, "result"), a.str(t, "reason"))
	}
	if w, _ := clockMs(t, a); w != 0 {
		t.Errorf("white has %d ms", w)
	}
	g := grid(a.play(t))
	if !strings.Contains(g, "Time forfeit · 0–1") || !strings.Contains(g, "White ran out of time") {
		t.Errorf("status:\n%s", g)
	}
	// The clock stops with the game.
	a.tickAfter(t, time.Second)
	if _, b := clockMs(t, a); b != 6000 {
		t.Errorf("black's clock moved after the game: %d", b)
	}
	// Black flags too, against a side that can still mate.
	b := clocked(t, "0.1+0")
	b.moves(t, "Nf3")
	for i := 0; i < 7; i++ {
		b.tickAfter(t, time.Second)
	}
	if b.str(t, "result") != "1-0" || !strings.Contains(grid(b.play(t)), "Black ran out of time") {
		t.Errorf("result %s", b.str(t, "result"))
	}
	// The computer loses on time too.
	d := newTestApp(t, func(o *options) { o.clock = "0.1+0" })
	pending := d.deferJobs()
	d.moves(t, "e4")
	for _, f := range *pending {
		f()
	}
	*pending = nil
	d.moves(t, "Nf3")
	if len(d.g.SAN) != 3 || !d.flag(t, "thinking") {
		t.Fatalf("setup: moves %v thinking %v", d.g.SAN, d.flag(t, "thinking"))
	}
	for i := 0; i < 7; i++ {
		d.tickAfter(t, time.Second)
	}
	if d.str(t, "result") != "1-0" || d.str(t, "reason") != "time forfeit" {
		t.Errorf("the computer did not flag: %s %s", d.str(t, "result"), d.str(t, "reason"))
	}
	for _, f := range *pending {
		f()
	}
	if len(d.g.SAN) != 3 {
		t.Errorf("the flagged computer still moved: %v", d.g.SAN)
	}
}

// FIDE 6.9: the other side wins on time unless it could not possibly mate,
// which is decided conservatively: it has only a king, or a lone knight or
// bishop against a bare king.
func TestFlagFallAgainstTooLittleMaterial(t *testing.T) {
	cases := []struct {
		name, fen string
		loser     Color
		want      string
	}{
		{"bare king can't win", "4k3/8/8/8/8/8/P7/4KN2 w - - 0 1", White, "1/2-1/2"},
		{"lone knight against a bare king", "4k3/8/8/8/8/8/P7/4KN2 w - - 0 1", Black, "1-0"}, // white has a pawn too
		{"knight v pawn: the knight side wins", "4k3/p7/8/8/8/8/8/4KN2 w - - 0 1", Black, "1-0"},
		{"knight v pawn: the pawn side wins", "4k3/p7/8/8/8/8/8/4KN2 w - - 0 1", White, "0-1"},
		{"opposite bishops win on time", "4kb2/p7/8/8/8/8/8/3BK3 w - - 0 1", Black, "1-0"},
		{"opposite bishops win on time (other side)", "4kb2/p7/8/8/8/8/8/3BK3 w - - 0 1", White, "0-1"},
		{"rook against a bare king", "4k3/8/8/8/8/8/8/R3K3 w - - 0 1", Black, "1-0"},
		{"bare king against a rook", "4k3/8/8/8/8/8/8/R3K3 w - - 0 1", White, "1/2-1/2"},
		{"two knights can mate", "4k3/8/8/8/8/8/8/2NNK3 w - - 0 1", Black, "1-0"},
	}
	for _, c := range cases {
		g := NewGame(mustFEN(t, c.fen))
		if g.Over() {
			t.Fatalf("%s: the position is over before the flag falls: %s", c.name, g.Reason)
		}
		g.Flag(c.loser)
		if g.Result != c.want {
			t.Errorf("%s: %s flags, result %s, want %s", c.name, c.loser, g.Result, c.want)
		}
		if c.want == "1/2-1/2" && g.Reason != "timeout vs insufficient material" {
			t.Errorf("%s: reason %q", c.name, g.Reason)
		}
	}
}

// The same through the clock: white, with a pawn and a knight, flags against
// a bare king.
func TestAFlagAgainstABareKingIsADraw(t *testing.T) {
	a := newTestApp(t, twoPlayers, func(o *options) {
		o.clock = "0.1+0"
		o.fen = "4k3/8/8/8/8/8/P7/4KN2 w - - 0 1"
	})
	a.moves(t, "Nd2", "Kd7") // both first moves, free
	for i := 0; i < 7; i++ {
		a.tickAfter(t, time.Second)
	}
	if a.str(t, "result") != "1/2-1/2" || !strings.Contains(a.str(t, "reason"), "insufficient") {
		t.Errorf("result %s %s", a.str(t, "result"), a.str(t, "reason"))
	}
	if !strings.Contains(grid(a.play(t)), "Draw") {
		t.Error("the status does not say draw")
	}
}

func TestClockPausesInReviewUnderDialogsAndWithoutAClock(t *testing.T) {
	a := clocked(t, "5+0")
	a.moves(t, "Nf3", "Nc6")
	a.play(t, "v")
	a.tickFor(t, 10*time.Second)
	if w, b := clockMs(t, a); w != 300000 || b != 300000 {
		t.Errorf("the clock ran in review: %d %d", w, b)
	}
	if run := a.get(t, "clock").(map[string]any)["run"]; run != "" {
		t.Errorf("run = %v in review", run)
	}
	a.play(t, "v")
	for _, key := range []string{"?", "n", "r", "p"} {
		a.play(t, key)
		a.tickFor(t, 10*time.Second)
		if w, _ := clockMs(t, a); w != 300000 {
			t.Errorf("the clock ran under the %s dialog: %d", key, w)
		}
		a.play(t, "esc")
	}
	a.tickAfter(t, time.Second)
	if w, _ := clockMs(t, a); w != 299000 {
		t.Errorf("the clock did not resume: %d", w)
	}
	// No clock: nothing moves, and the strip shows a dash.
	b := newTestApp(t, twoPlayers)
	b.moves(t, "e4", "e5")
	b.tickFor(t, time.Minute)
	if w, _ := clockMs(t, b); w != 0 {
		t.Errorf("a game without a clock has %d ms", w)
	}
	if got := b.get(t, "bot").(map[string]any)["clock"]; got != "–" {
		t.Errorf("clock label = %v", got)
	}
	// Thinking time is the computer's: its clock runs while it thinks.
	c := newTestApp(t, func(o *options) { o.clock = "5+0" })
	pending := c.deferJobs()
	c.moves(t, "e4")
	for _, f := range *pending {
		f()
	}
	*pending = nil
	c.moves(t, "Nf3") // white's second move: black, the computer, is on the clock
	c.tickFor(t, 4*time.Second)
	if w, b := clockMs(t, c); w != 300000 || b != 296000 {
		t.Errorf("clocks while the computer thinks: %d %d", w, b)
	}
}

// The answer is delayed so a quick level is not instant; that delay is not
// thinking time, and the clock gets it back. Neither is the pause the
// viewer gets between two moves of a game the computer plays.
func TestWaitingIsNotChargedToTheComputer(t *testing.T) {
	a := newTestApp(t, func(o *options) { o.clock = "1+0" })
	pending := a.deferJobs()
	a.moves(t, "e4")
	for _, f := range *pending {
		f()
	}
	*pending = nil
	a.moves(t, "Nf3")
	a.mu.Lock()
	j := &job{gen: a.gen, pos: *a.g.Cur(), keys: append([]uint64(nil), a.g.Keys...), lim: levelLimits[1], stop: a.stop}
	a.mu.Unlock()
	res := Search(j.pos, j.keys, j.lim, nil)
	a.tickFor(t, 400*time.Millisecond) // the search slept this long
	_, before := clockMs(t, a)
	a.finish(j, res, 400*time.Millisecond)
	if _, after := clockMs(t, a); after != before+400 {
		t.Errorf("black had %d ms before the answer and %d after, want 400 ms back", before, after)
	}

	c := newTestApp(t, func(o *options) { o.mode = modeCvC; o.clock = "1+0"; o.level = 1 })
	cp := c.deferJobs()
	c.tickAfter(t, tickStep)
	(*cp)[0]() // white moves
	*cp = nil
	c.tickAfter(t, cvcDelay)
	(*cp)[0]() // black moves, and the pause begins
	*cp = nil
	if len(c.g.SAN) != 2 {
		t.Fatalf("moves %v", c.g.SAN)
	}
	w0, b0 := clockMs(t, c)
	c.tickAfter(t, tickStep)
	c.tickAfter(t, tickStep)
	c.tickAfter(t, tickStep)
	if w, b := clockMs(t, c); w != w0 || b != b0 {
		t.Errorf("the viewer's pause was charged: %d %d -> %d %d", w0, b0, w, b)
	}
}

func TestLowClockIsFlagged(t *testing.T) {
	a := clocked(t, "0.5+0")
	if bot := a.get(t, "bot").(map[string]any); bot["low"] != false {
		t.Errorf("0:30 is low: %v", bot)
	}
	a.tickAfter(t, time.Second)
	for i := 0; i < 10; i++ {
		a.tickAfter(t, time.Second)
	}
	if bot := a.get(t, "bot").(map[string]any); bot["low"] != true || bot["clock"] != "19.0" {
		t.Errorf("bot = %v, want a low clock at 19.0", bot)
	}
}

// With no room for the side panel the clocks move to the top line.
func TestCompactTopLineShowsTheClocks(t *testing.T) {
	a := clocked(t, "3+2")
	a.moves(t, "Nf3", "Nc6")
	for _, sz := range [][2]int{{40, 14}, {50, 16}, {59, 20}} {
		res := a.playAt(t, sz[0], sz[1])
		top := res.Dump.Grid[0]
		if !strings.Contains(top, "W 3:02") || !strings.Contains(top, "B 3:02") {
			t.Errorf("%dx%d: no clocks in the top line: %q", sz[0], sz[1], top)
		}
		if !strings.Contains(top, "White to move") {
			t.Errorf("%dx%d: the status is cut: %q", sz[0], sz[1], top)
		}
	}
	// A check does not cut the status either.
	c := newTestApp(t, twoPlayers, func(o *options) { o.clock = "3+2" })
	c.moves(t, "e4", "f5", "Qh5")
	top := c.playAt(t, 40, 14).Dump.Grid[0]
	if !strings.Contains(top, "Check · Black to move") || !strings.Contains(top, "W 3:02") || !strings.Contains(top, "B 3:00") {
		t.Errorf("40x14 top line: %q", top)
	}
	// Without a time control there is nothing to show.
	d := newTestApp(t, twoPlayers)
	if top := d.playAt(t, 40, 14).Dump.Grid[0]; strings.Contains(top, "W ") {
		t.Errorf("clocks without a clock: %q", top)
	}
}

// Playing on from a review restores the clocks of that position, not the
// ones the game ended with.
func TestPlayingOnFromAReviewRestoresTheClocks(t *testing.T) {
	a := clocked(t, "0.1+0")
	for i := 0; i < 7; i++ {
		a.tickAfter(t, time.Second)
	}
	if a.str(t, "result") != "0-1" {
		t.Fatalf("white did not flag: %s", a.str(t, "result"))
	}
	a.play(t, "v", "left", "left", "enter")
	if a.g.Over() || a.flag(t, "review") {
		t.Fatalf("over=%v review=%v after playing on", a.g.Over(), a.flag(t, "review"))
	}
	if w, b := clockMs(t, a); w != 6000 || b != 6000 {
		t.Errorf("the clocks after playing on from the start: %d %d, want 6000 each", w, b)
	}
	a.moves(t, "d4")
	a.tickAfter(t, time.Second)
	if a.g.Over() {
		t.Error("the game flagged again at once")
	}
}
