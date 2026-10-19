package main

import (
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// deferJobs makes searches wait: the test runs them when it likes, so it can
// act while the computer is "thinking".
func (a *testApp) deferJobs() *[]func() {
	var pending []func()
	a.dispatch = func(f func()) { pending = append(pending, f) }
	return &pending
}

func TestComputerReplies(t *testing.T) {
	a := newTestApp(t)
	res := a.play(t, ":", "text:e4", "enter")
	if len(a.g.SAN) != 2 || a.g.SAN[0] != "e4" {
		t.Fatalf("moves = %v, want e4 and a reply", a.g.SAN)
	}
	if a.flag(t, "thinking") || a.str(t, "turn") != "white" {
		t.Errorf("thinking=%v turn=%s", a.flag(t, "thinking"), a.str(t, "turn"))
	}
	// The engine panel reports the search: depth 3 at level 3.
	ev := a.get(t, "engine_view").(map[string]any)
	if ev["has"] != true || !strings.HasPrefix(ev["line"].(string), "depth 3 · ") || !strings.HasSuffix(ev["line"].(string), " nodes") {
		t.Errorf("engine_view = %v", ev)
	}
	if !strings.HasPrefix(ev["pv"].(string), "pv 1...") {
		t.Errorf("pv = %q, want a variation starting with black's move", ev["pv"])
	}
	g := grid(a.play(t))
	if !strings.Contains(g, "depth 3") || !strings.Contains(g, "White to move") {
		t.Errorf("the screen does not show the search:\n%s", g)
	}
	_ = res
}

// At a fixed depth the computer plays the same game every time.
func TestComputerIsDeterministic(t *testing.T) {
	game := func() string {
		a := newTestApp(t)
		a.moves(t, "e4", "Nf3", "Bc4", "d3")
		return strings.Join(a.g.SAN, " ")
	}
	first := game()
	if second := game(); first != second {
		t.Errorf("two games differ:\n%s\n%s", first, second)
	}
	if len(strings.Fields(first)) != 8 {
		t.Errorf("game = %q, want 8 plies", first)
	}
}

func TestStaleResultIsDropped(t *testing.T) {
	cases := map[string]func(a *testApp){
		"undo":     func(a *testApp) { a.play(t, "u") },
		"new game": func(a *testApp) { a.play(t, "n", "enter") },
		"resign":   func(a *testApp) { a.play(t, "r", "y") },
		"review":   func(a *testApp) { a.play(t, "v") },
	}
	// (A flag fall behind a running search is in TestFlagFallLosesOnTime.)
	for name, act := range cases {
		a := newTestApp(t)
		pending := a.deferJobs()
		a.play(t, ":", "text:e4", "enter")
		if !a.flag(t, "thinking") || len(*pending) != 1 {
			t.Fatalf("%s: thinking=%v, %d jobs", name, a.flag(t, "thinking"), len(*pending))
		}
		act(a)
		if a.flag(t, "thinking") {
			t.Errorf("%s: still thinking", name)
		}
		before := a.fenNow()
		beforePly := len(a.g.SAN)
		for _, f := range *pending {
			f()
		}
		if a.fenNow() != before || len(a.g.SAN) != beforePly {
			t.Errorf("%s: a late result changed the game: %s -> %s", name, before, a.fenNow())
		}
	}
	// And a result that is still current is applied.
	a := newTestApp(t)
	pending := a.deferJobs()
	a.play(t, ":", "text:e4", "enter")
	for _, f := range *pending {
		f()
	}
	if len(a.g.SAN) != 2 {
		t.Errorf("a current result was dropped: %v", a.g.SAN)
	}
}

// Only the side to move may claim a draw: not on the computer's turn.
func TestClaimIsForTheSideToMove(t *testing.T) {
	a := newTestApp(t, func(o *options) { o.fen = "4k3/8/8/8/8/8/4P3/R3K3 b - - 100 80" })
	pending := a.deferJobs()
	a.tickAfter(t, tickStep) // black is the computer: it starts thinking
	if !a.flag(t, "thinking") || len(*pending) != 1 {
		t.Fatalf("thinking=%v jobs=%d", a.flag(t, "thinking"), len(*pending))
	}
	a.play(t, "d")
	if a.g.Over() || !a.flag(t, "thinking") || !strings.Contains(a.str(t, "notice"), "computer's turn") {
		t.Errorf("a claim on the computer's turn: over=%v notice=%q", a.g.Over(), a.str(t, "notice"))
	}
	// The computer, no better than equal, claims it itself when its answer
	// comes in.
	for _, f := range *pending {
		f()
	}
	if a.str(t, "result") != "1/2-1/2" {
		t.Errorf("result %s", a.str(t, "result"))
	}
	// On your own move the claim is yours.
	b := newTestApp(t, func(o *options) { o.fen = "4k3/8/8/8/8/8/4P3/R3K3 w - - 100 80" })
	b.play(t, "d")
	if b.str(t, "result") != "1/2-1/2" {
		t.Errorf("result %s", b.str(t, "result"))
	}
}

func TestStopFlagEndsASearchThatIsNoLongerWanted(t *testing.T) {
	a := newTestApp(t)
	pending := a.deferJobs()
	a.play(t, ":", "text:e4", "enter")
	a.mu.Lock()
	stop := a.stop
	a.mu.Unlock()
	if stop == nil || stop.Load() {
		t.Fatal("the search has no live stop flag")
	}
	a.play(t, "u")
	if !stop.Load() {
		t.Error("undo did not stop the search")
	}
	_ = pending
}

func TestThinkingIndicatorAnimates(t *testing.T) {
	a := newTestApp(t)
	pending := a.deferJobs()
	res := a.play(t, ":", "text:e4", "enter")
	if len(*pending) != 1 {
		t.Fatal("no search started")
	}
	g := grid(res)
	if !strings.Contains(g, "thinking…") || !strings.Contains(g, "Black is thinking…") {
		t.Errorf("no thinking indicator:\n%s", g)
	}
	first := a.str(t, "think_label")
	a.tickAfter(t, tickStep)
	second := a.str(t, "think_label")
	a.tickAfter(t, tickStep)
	third := a.str(t, "think_label")
	if first == second || second == third || first == third {
		t.Errorf("the indicator does not animate: %q %q %q", first, second, third)
	}
	// The computer's clock is what runs; the human cannot move meanwhile.
	a.click(t, "e7")
	if a.num(t, "sel") != -1 || !strings.Contains(a.str(t, "notice"), "computer's turn") {
		t.Errorf("a person moved for the computer: sel=%d notice=%q", a.num(t, "sel"), a.str(t, "notice"))
	}
	a.play(t, ":", "text:e5", "enter")
	if len(a.g.SAN) != 1 {
		t.Errorf("moves = %v", a.g.SAN)
	}
	for _, f := range *pending {
		f()
	}
	if a.flag(t, "thinking") || a.str(t, "think_label") != "" {
		t.Error("the indicator stays after the answer")
	}
}

func TestHumanPlaysBlack(t *testing.T) {
	a := newTestApp(t, func(o *options) { o.side = "black" })
	if !a.flag(t, "flip") || a.str(t, "human") != "black" {
		t.Fatalf("flip=%v human=%s", a.flag(t, "flip"), a.str(t, "human"))
	}
	if a.num(t, "cur") != sq(t, "e7") {
		t.Errorf("the cursor starts on %s", sqName(a.num(t, "cur")))
	}
	// White is the computer: it opens on the first tick.
	a.tickAfter(t, tickStep)
	if len(a.g.SAN) != 1 {
		t.Fatalf("the computer did not open: %v", a.g.SAN)
	}
	if top := a.get(t, "top").(map[string]any); top["name"] != "computer L3" {
		t.Errorf("top = %v", top)
	}
	a.moves(t, "e5")
	if len(a.g.SAN) != 3 {
		t.Errorf("moves = %v", a.g.SAN)
	}
}

func TestComputerVersusComputer(t *testing.T) {
	game := func() string {
		a := newTestApp(t, func(o *options) { o.mode = modeCvC; o.level = 1 })
		if got := grid(a.play(t)); !strings.Contains(got, "computer vs computer") {
			t.Errorf("no mode label:\n%s", got)
		}
		a.tickAfter(t, tickStep)
		if len(a.g.SAN) != 1 {
			t.Fatalf("the first move did not come at once: %v", a.g.SAN)
		}
		// It waits cvcDelay between moves.
		a.tickAfter(t, tickStep)
		if len(a.g.SAN) != 1 {
			t.Errorf("the second move came after %v", tickStep)
		}
		a.tickAfter(t, cvcDelay)
		if len(a.g.SAN) != 2 {
			t.Errorf("no second move after the pause: %v", a.g.SAN)
		}
		for i := 0; i < 20; i++ {
			a.tickAfter(t, cvcDelay)
		}
		if len(a.g.SAN) != 22 && !a.g.Over() {
			t.Errorf("%d plies after 22 steps", len(a.g.SAN))
		}
		return strings.Join(a.g.SAN, " ")
	}
	if first, second := game(), game(); first != second {
		t.Errorf("the computer plays two different games:\n%s\n%s", first, second)
	}
	// Watching: a modal pauses it.
	a := newTestApp(t, func(o *options) { o.mode = modeCvC; o.level = 1 })
	a.play(t, "?")
	a.tickAfter(t, tickStep)
	a.tickAfter(t, cvcDelay)
	if len(a.g.SAN) != 0 {
		t.Errorf("the computer moved under the help dialog: %v", a.g.SAN)
	}
	a.play(t, "esc")
	a.tickAfter(t, tickStep)
	if len(a.g.SAN) != 1 {
		t.Errorf("the computer did not resume: %v", a.g.SAN)
	}
	// Nothing to resign, no hint, in a game the computers play.
	a.play(t, "r")
	if a.flag(t, "resign_open") || !strings.Contains(a.str(t, "notice"), "will not resign") {
		t.Errorf("notice = %q", a.str(t, "notice"))
	}
	a.play(t, "H")
	if !strings.Contains(a.str(t, "notice"), "no hint needed") {
		t.Errorf("notice = %q", a.str(t, "notice"))
	}
}

func TestUndoAndRedo(t *testing.T) {
	// Against the computer, undo takes back your move and its reply.
	a := newTestApp(t)
	a.moves(t, "e4", "d4")
	if len(a.g.SAN) != 4 {
		t.Fatalf("moves = %v", a.g.SAN)
	}
	res := a.play(t, "u")
	wantActions(t, res, "undo")
	if len(a.g.SAN) != 2 || a.str(t, "turn") != "white" {
		t.Errorf("after undo: %v, %s to move", a.g.SAN, a.str(t, "turn"))
	}
	if !a.g.Redoable() {
		t.Fatal("nothing to redo")
	}
	second := a.g.SAN[1]
	res = a.play(t, "U")
	wantActions(t, res, "redo")
	if len(a.g.SAN) != 4 || a.g.SAN[3] != "" && len(a.g.SAN[3]) == 0 {
		t.Errorf("after redo: %v", a.g.SAN)
	}
	_ = second
	// Undo all the way, then once more.
	a.play(t, "u", "u")
	if len(a.g.SAN) != 0 || a.fenNow() != startFEN {
		t.Errorf("not at the start: %v %s", a.g.SAN, a.fenNow())
	}
	a.play(t, "u")
	if a.str(t, "notice") != "nothing to undo" {
		t.Errorf("notice = %q", a.str(t, "notice"))
	}
	// A new move clears the redo stack.
	a.moves(t, "c4")
	a.play(t, "U")
	if a.str(t, "notice") != "nothing to redo" {
		t.Errorf("notice = %q", a.str(t, "notice"))
	}

	// With two players, one move at a time.
	b := newTestApp(t, twoPlayers)
	b.moves(t, "e4", "e5", "Nf3")
	b.play(t, "u")
	if len(b.g.SAN) != 2 {
		t.Errorf("moves = %v", b.g.SAN)
	}
	b.play(t, "U")
	if len(b.g.SAN) != 3 || b.g.SAN[2] != "Nf3" {
		t.Errorf("moves = %v", b.g.SAN)
	}
	// The status follows the position.
	b.play(t, "u", "u")
	if !strings.Contains(grid(b.play(t)), "Black to move") {
		t.Error("the status does not follow the undo")
	}
	// Undoing a checkmate reopens the game.
	c := newTestApp(t, twoPlayers)
	c.moves(t, "f3", "e5", "g4", "Qh4")
	if !c.g.Over() {
		t.Fatal("fool's mate did not end the game")
	}
	c.play(t, "u")
	if c.g.Over() || c.str(t, "result") != "*" {
		t.Errorf("the result stays after an undo: %s", c.str(t, "result"))
	}
}

func TestHintShowsTheBestMove(t *testing.T) {
	a := newTestApp(t)
	res := a.play(t, "H")
	wantActions(t, res, "hint")
	h := asInts(a.get(t, "hint").([]any))
	if h[0] < 0 || !a.g.Cur().IsLegal(Move{From: uint8(h[0]), To: uint8(h[1])}) {
		t.Fatalf("hint = %v", h)
	}
	if n := a.str(t, "notice"); !strings.HasPrefix(n, "hint: ") {
		t.Errorf("notice = %q", n)
	}
	cells := a.get(t, "cells").(map[string]any)
	marked := 0
	for _, c := range cells {
		if c.(map[string]any)["hint"] == true {
			marked++
		}
	}
	if marked != 2 {
		t.Errorf("%d cells are marked as the hint, want 2", marked)
	}
	// The hint is the move the computer would play with the same budget.
	sp := StartPosition()
	want := Search(sp, []uint64{sp.Key()}, hintLimits, nil)
	if int(want.Move.From) != h[0] || int(want.Move.To) != h[1] {
		t.Errorf("hint %v, search %v", h, want.Move)
	}
	// A move clears it.
	a.moves(t, "e4")
	if got := asInts(a.get(t, "hint").([]any)); got[0] != -1 {
		t.Errorf("hint = %v after a move", got)
	}
	// Not in a review, not out of turn, not after the game.
	a.play(t, "v", "H")
	if !strings.Contains(a.str(t, "notice"), "leave the review") {
		t.Errorf("notice = %q", a.str(t, "notice"))
	}
	over := newTestApp(t, twoPlayers)
	over.moves(t, "f3", "e5", "g4", "Qh4")
	over.play(t, "H")
	if !strings.Contains(over.str(t, "notice"), "game is over") {
		t.Errorf("notice = %q", over.str(t, "notice"))
	}
	// A hint that arrives after an undo is dropped.
	b := newTestApp(t, twoPlayers)
	pending := b.deferJobs()
	b.play(t, "H")
	b.moves(t, "e4")
	for _, f := range *pending {
		f()
	}
	if got := asInts(b.get(t, "hint").([]any)); got[0] != -1 {
		t.Errorf("a late hint was shown: %v", got)
	}
}

func TestResign(t *testing.T) {
	a := newTestApp(t)
	a.moves(t, "e4")
	res := a.play(t, "r")
	wantActions(t, res, "resign_open")
	g := grid(res)
	if !strings.Contains(g, "resign?") || !strings.Contains(g, "keep playing") || focused(res) != "resign-no" {
		t.Errorf("focus %q:\n%s", focused(res), g)
	}
	// Keep playing: esc and n both.
	for _, key := range []string{"esc", "n"} {
		a.play(t, key)
		if a.flag(t, "resign_open") || a.g.Over() {
			t.Errorf("%s: open=%v over=%v", key, a.flag(t, "resign_open"), a.g.Over())
		}
		a.play(t, "r")
	}
	res = a.play(t, "y")
	wantActions(t, res, "resign_yes")
	if a.str(t, "result") != "0-1" || a.str(t, "reason") != "resignation" {
		t.Errorf("result %s %s", a.str(t, "result"), a.str(t, "reason"))
	}
	g = grid(a.play(t))
	if !strings.Contains(g, "Resignation · 0–1") || !strings.Contains(g, "White resigned") {
		t.Errorf("status:\n%s", g)
	}
	// Undo takes the resignation back, with the move before it.
	a.play(t, "u")
	if a.g.Over() || a.str(t, "result") != "*" {
		t.Error("undo kept the resignation")
	}
	// Two players: the side to move resigns.
	b := newTestApp(t, twoPlayers)
	b.moves(t, "e4")
	b.play(t, "r", "y")
	if b.str(t, "result") != "1-0" || !strings.Contains(grid(b.play(t)), "Black resigned") {
		t.Errorf("result %s", b.str(t, "result"))
	}
	b.play(t, "r")
	if b.flag(t, "resign_open") {
		t.Error("the dialog opens after the game")
	}
}

func TestClaimDraw(t *testing.T) {
	a := newTestApp(t, twoPlayers)
	a.play(t, "d")
	if !strings.Contains(a.str(t, "notice"), "no draw to claim") {
		t.Errorf("notice = %q", a.str(t, "notice"))
	}
	a.moves(t, "Nf3", "Nf6", "Ng1", "Ng8")
	if got := a.get(t, "claim"); got != "" {
		t.Errorf("claim = %v after one repetition", got)
	}
	a.moves(t, "Nf3", "Nf6", "Ng1", "Ng8")
	if a.str(t, "claim") != "threefold repetition" {
		t.Fatalf("claim = %q", a.str(t, "claim"))
	}
	g := grid(a.play(t))
	if !strings.Contains(g, "d claims a draw: threefold repetition") {
		t.Errorf("the status does not offer the claim:\n%s", g)
	}
	res := a.play(t, "d")
	wantActions(t, res, "claim")
	if a.str(t, "result") != "1/2-1/2" || a.str(t, "reason") != "threefold repetition" {
		t.Errorf("result %s %s", a.str(t, "result"), a.str(t, "reason"))
	}
	if g := grid(a.play(t)); !strings.Contains(g, "Threefold repetition · ½–½") {
		t.Errorf("status:\n%s", g)
	}
	// The fifty-move rule.
	b := newTestApp(t, twoPlayers, func(o *options) { o.fen = "4k3/8/8/8/8/8/4P3/R3K3 w - - 100 80" })
	if b.str(t, "claim") != "fifty-move rule" {
		t.Fatalf("claim = %q", b.str(t, "claim"))
	}
	b.play(t, "d")
	if b.str(t, "reason") != "fifty-move rule" {
		t.Errorf("reason = %q", b.str(t, "reason"))
	}
}

func TestComputerClaimsADrawWhenNoBetter(t *testing.T) {
	// A bare-kings-and-rook shuffle where black, the computer, is worse and
	// the fifty-move rule applies.
	a := newTestApp(t, func(o *options) {
		o.side = "white"
		o.fen = "4k3/8/8/8/8/8/4P3/R3K3 b - - 100 80"
	})
	_ = a
	// Black to move is the computer: the first tick starts it, and with a
	// lost position it claims the draw.
	a.tickAfter(t, tickStep)
	if a.str(t, "result") != "1/2-1/2" || a.str(t, "reason") != "fifty-move rule" {
		t.Errorf("result %s %s, moves %v", a.str(t, "result"), a.str(t, "reason"), a.g.SAN)
	}
}

func TestReviewMode(t *testing.T) {
	a := newTestApp(t, twoPlayers)
	a.moves(t, "e4", "e5", "Nf3", "Nc6")
	res := a.play(t, "v")
	wantActions(t, res, "review")
	g := grid(res)
	if !strings.Contains(g, "Review · move 4 of 4") {
		t.Errorf("no review status:\n%s", g)
	}
	steps := []struct {
		key  string
		ply  int
		fen  string
		text string
	}{
		{"left", 3, "rnbqkbnr/pppp1ppp/8/4p3/4P3/5N2/PPPP1PPP/RNBQKB1R b KQkq - 1 2", "move 3 of 4"},
		{"left", 2, "rnbqkbnr/pppp1ppp/8/4p3/4P3/8/PPPP1PPP/RNBQKBNR w KQkq e6 0 2", "move 2 of 4"},
		{"home", 0, startFEN, "move 0 of 4"},
		{"left", 0, startFEN, "move 0 of 4"},
		{"right", 1, "rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq e3 0 1", "move 1 of 4"},
		{"end", 4, "", "move 4 of 4"},
		{"right", 4, "", "move 4 of 4"},
	}
	for _, s := range steps {
		res := a.play(t, s.key)
		if a.num(t, "ply") != s.ply {
			t.Errorf("%s: ply = %d, want %d", s.key, a.num(t, "ply"), s.ply)
		}
		if s.fen != "" {
			got := a.viewPos().FEN()
			// The en passant square is bookkeeping here; compare the board.
			if strings.Fields(got)[0] != strings.Fields(s.fen)[0] {
				t.Errorf("%s: board %s, want %s", s.key, got, s.fen)
			}
		}
		if !strings.Contains(grid(res), "Review · "+s.text) {
			t.Errorf("%s: status lacks %q:\n%s", s.key, s.text, grid(res))
		}
	}
	// The move list marks the move on the board, and the cursor row follows.
	a.play(t, "home", "right", "right", "right")
	rows := a.get(t, "mrows").([]any)
	if rows[1].(map[string]any)["cw"] != true || a.num(t, "mcur") != 2 {
		t.Errorf("row 2 = %v, mcur = %d", rows[1], a.num(t, "mcur"))
	}
	// Clicks and undo do nothing in review, and say why.
	a.click(t, "e2")
	if a.num(t, "sel") != -1 || !strings.Contains(a.str(t, "notice"), "reviewing") {
		t.Errorf("sel=%d notice=%q", a.num(t, "sel"), a.str(t, "notice"))
	}
	a.play(t, "u")
	if !strings.Contains(a.str(t, "notice"), "leave the review") || len(a.g.SAN) != 4 {
		t.Errorf("notice = %q", a.str(t, "notice"))
	}
	// enter plays on from here, dropping the rest.
	res = a.play(t, "enter")
	wantActions(t, res, "select")
	if a.flag(t, "review") || len(a.g.SAN) != 3 || a.str(t, "turn") != "black" {
		t.Errorf("review=%v moves=%v turn=%s", a.flag(t, "review"), a.g.SAN, a.str(t, "turn"))
	}
	a.moves(t, "Nc6")
	// v goes in and out.
	a.play(t, "v")
	if !a.flag(t, "review") {
		t.Fatal("v did not start the review")
	}
	a.play(t, "v")
	if a.flag(t, "review") || a.num(t, "ply") != 4 {
		t.Errorf("v did not leave the review: ply %d", a.num(t, "ply"))
	}
}

func TestPGNExportInADialogAndToAFile(t *testing.T) {
	a := newTestApp(t, twoPlayers)
	a.moves(t, "e4", "e5", "Nf3")
	res := a.play(t, "p")
	wantActions(t, res, "pgn")
	if !a.flag(t, "pgn_open") || focused(res) != "pgn-ok" {
		t.Fatalf("open=%v focus=%q", a.flag(t, "pgn_open"), focused(res))
	}
	text := a.str(t, "pgn_text")
	for _, want := range []string{`[Event "Casual game"]`, `[Date "2026.10.19"]`, `[White "White"]`, `[Result "*"]`, "1. e4 e5 2. Nf3 *"} {
		if !strings.Contains(text, want) {
			t.Errorf("PGN lacks %q:\n%s", want, text)
		}
	}
	g := grid(res)
	if !strings.Contains(g, `[Event "Casual game"]`) || !strings.Contains(g, "close") {
		t.Errorf("the dialog does not show the PGN:\n%s", g)
	}
	res = a.play(t, "enter")
	wantActions(t, res, "pgn_close")
	if a.flag(t, "pgn_open") || focused(res) != "board" {
		t.Errorf("open=%v focus=%q", a.flag(t, "pgn_open"), focused(res))
	}
	a.play(t, "p", "esc")
	if a.flag(t, "pgn_open") {
		t.Error("esc did not close the dialog")
	}

	// With -save the game is written to the file instead.
	path := filepath.Join(t.TempDir(), "game.pgn")
	b := newTestApp(t, twoPlayers, func(o *options) { o.save = path })
	b.moves(t, "d4", "d5")
	b.play(t, "p")
	if b.flag(t, "pgn_open") {
		t.Error("the dialog opened with -save")
	}
	if !strings.Contains(b.str(t, "notice"), "saved "+path) {
		t.Errorf("notice = %q", b.str(t, "notice"))
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	back, err := ParsePGN(string(raw))
	if err != nil || strings.Join(back.SAN, " ") != "d4 d5" {
		t.Errorf("saved file reads back as %v, %v:\n%s", back, err, raw)
	}
	// A path that cannot be written is a notice, not a crash.
	c := newTestApp(t, twoPlayers, func(o *options) { o.save = filepath.Join(t.TempDir(), "no", "such", "dir.pgn") })
	c.play(t, "p")
	if !strings.Contains(c.str(t, "notice"), "cannot save") {
		t.Errorf("notice = %q", c.str(t, "notice"))
	}
}

func TestLoadPGNAndReviewIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "opera.pgn")
	text := "[Event \"Opera House\"]\n[White \"Paul Morphy\"]\n[Black \"Duke Karl\"]\n[Result \"1-0\"]\n\n" + operaGame + " 1-0\n"
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	a := newTestApp(t, func(o *options) { o.pgn = path })
	if !a.flag(t, "review") || a.num(t, "ply") != 0 || a.num(t, "total") != 33 || a.str(t, "mode") != mode2P {
		t.Fatalf("review=%v ply=%d total=%d mode=%s", a.flag(t, "review"), a.num(t, "ply"), a.num(t, "total"), a.str(t, "mode"))
	}
	g := grid(a.play(t))
	for _, want := range []string{"Paul Morphy", "Duke Karl", "Review · move 0 of 33"} {
		if !strings.Contains(g, want) {
			t.Errorf("grid lacks %q:\n%s", want, g)
		}
	}
	a.play(t, "end")
	g = grid(a.play(t))
	if !strings.Contains(g, "Checkmate · 1–0") || !strings.Contains(g, "Rd8#") {
		t.Errorf("the end of the game:\n%s", g)
	}
	// One step back and the game is not over there.
	a.play(t, "left")
	if g := grid(a.play(t)); strings.Contains(g, "Checkmate") || !strings.Contains(g, "Review · move 32 of 33") {
		t.Errorf("move 32:\n%s", g)
	}
	// Bad files are errors.
	bad := filepath.Join(t.TempDir(), "bad.pgn")
	_ = os.WriteFile(bad, []byte("1. e4 e5 2. Ke3"), 0o644)
	o := defaults()
	o.pgn = bad
	s, _ := o.validate()
	if _, err := newApp(o, s); err == nil || !strings.Contains(err.Error(), "-pgn") {
		t.Errorf("a bad PGN gave %v", err)
	}
	o.pgn = filepath.Join(t.TempDir(), "missing.pgn")
	if _, err := newApp(o, s); err == nil {
		t.Error("a missing PGN gave no error")
	}
}

func TestFENFlag(t *testing.T) {
	a := newTestApp(t, twoPlayers, func(o *options) { o.fen = "4k3/8/8/8/8/8/4P3/4K3 b - - 0 7" })
	if a.str(t, "turn") != "black" || a.fenNow() != "4k3/8/8/8/8/8/4P3/4K3 b - - 0 7" {
		t.Errorf("FEN %s", a.fenNow())
	}
	a.moves(t, "Kd7")
	rows := a.get(t, "mrows").([]any)
	if first := rows[0].(map[string]any); first["n"] != float64(7) || first["w"] != "…" || first["b"] != "Kd7" {
		t.Errorf("row = %v, want 7. … Kd7", first)
	}
	o := defaults()
	o.fen = "nonsense"
	s, _ := o.validate()
	if _, err := newApp(o, s); err == nil || !strings.Contains(err.Error(), "-fen") {
		t.Errorf("a bad FEN gave %v", err)
	}
}

func TestNewGameDialog(t *testing.T) {
	a := newTestApp(t)
	res := a.play(t, "n")
	wantActions(t, res, "new_open")
	g := grid(res)
	for _, want := range []string{"new game", "human vs computer", "you play", "White", "level", "3 (up to depth 3)", "clock", "no clock", "start", "cancel"} {
		if !strings.Contains(g, want) {
			t.Errorf("dialog lacks %q:\n%s", want, g)
		}
	}
	if focused(res) != "ng-start" {
		t.Errorf("focus = %q", focused(res))
	}
	for _, c := range []struct {
		key, path string
		want      any
	}{
		{"m", "ng.mode", "2p"}, {"m", "ng.mode", "cvc"}, {"m", "ng.mode", "hvc"},
		{"s", "ng.side", "black"}, {"s", "ng.side", "random"}, {"s", "ng.side", "white"},
		{"l", "ng.level", float64(4)}, {"c", "ng.clock", "5+0"}, {"c", "ng.clock", "3+2"}, {"c", "ng.clock", "10+5"}, {"c", "ng.clock", "none"},
	} {
		a.play(t, c.key)
		if got := a.get(t, c.path); got != c.want {
			t.Errorf("%s: %s = %v, want %v", c.key, c.path, got, c.want)
		}
	}
	for i := 0; i < 2; i++ {
		a.play(t, "l")
	}
	if a.num(t, "ng.level") != 6 {
		t.Errorf("level = %d", a.num(t, "ng.level"))
	}
	a.play(t, "l")
	if a.num(t, "ng.level") != 1 {
		t.Errorf("level did not wrap: %d", a.num(t, "ng.level"))
	}
	// The side row is only for a game against the computer; the level row
	// only when there is a computer.
	a.play(t, "m")
	g = grid(a.play(t))
	if strings.Contains(g, "you play") || !strings.Contains(g, "two players") || strings.Contains(g, "(depth") {
		t.Errorf("two players still asks for a side or a level:\n%s", g)
	}
	// Start: two players, 3+2, black's reply to nothing.
	a.play(t, "c", "c")
	res = a.play(t, "enter")
	wantActions(t, res, "ng_start")
	if a.flag(t, "new_open") || focused(res) != "board" || a.str(t, "mode") != mode2P || a.g.Ply() != 0 {
		t.Errorf("open=%v focus=%q mode=%s", a.flag(t, "new_open"), focused(res), a.str(t, "mode"))
	}
	clk := a.get(t, "clock").(map[string]any)
	if clk["on"] != true || clk["w"] != float64(180000) {
		t.Errorf("clock = %v, want 3 minutes", clk)
	}
	if a.num(t, "level") != 1 {
		t.Errorf("level = %d", a.num(t, "level"))
	}
	// The dialog opens on the current settings.
	a.play(t, "n")
	if a.str(t, "ng.mode") != "2p" || a.str(t, "ng.clock") != "3+2" {
		t.Errorf("ng = %v", a.get(t, "ng"))
	}
	// esc cancels and changes nothing.
	a.play(t, "m", "esc")
	if a.flag(t, "new_open") || a.str(t, "mode") != mode2P {
		t.Errorf("open=%v mode=%s", a.flag(t, "new_open"), a.str(t, "mode"))
	}
	// A game in progress is replaced.
	b := newTestApp(t, twoPlayers)
	b.moves(t, "e4", "e5")
	b.play(t, "n", "enter")
	if b.g.Ply() != 0 || b.fenNow() != startFEN {
		t.Errorf("new game kept the old one: %v", b.g.SAN)
	}
	// Starting as black flips the board and the computer opens.
	c := newTestApp(t)
	c.play(t, "n", "s", "enter")
	if !c.flag(t, "flip") || c.str(t, "human") != "black" {
		t.Errorf("flip=%v human=%s", c.flag(t, "flip"), c.str(t, "human"))
	}
	c.tickAfter(t, tickStep)
	if len(c.g.SAN) != 1 {
		t.Errorf("the computer did not open: %v", c.g.SAN)
	}
}

func TestRandomSideFollowsTheSeed(t *testing.T) {
	for _, seed := range []int64{1, 2, 3, 7, 11} {
		want := "white"
		if rand.New(rand.NewSource(seed)).Intn(2) == 1 {
			want = "black"
		}
		a := newTestApp(t, func(o *options) { o.seed = seed })
		a.play(t, "n", "s", "s", "enter")
		if got := a.str(t, "human"); got != want {
			t.Errorf("seed %d: you play %s, want %s", seed, got, want)
		}
		o := defaults()
		o.side, o.seed = "random", seed
		s, err := o.validate()
		if err != nil {
			t.Fatal(err)
		}
		if s.human.String() != want {
			t.Errorf("seed %d: -side random gives %s, want %s", seed, s.human, want)
		}
	}
}
