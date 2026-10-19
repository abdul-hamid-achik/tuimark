package main

import (
	"strconv"
	"strings"
	"testing"
)

func TestValidateIsClean(t *testing.T) {
	a := newTestApp(t)
	for _, theme := range []string{"dark", "light"} {
		if err := a.ui.Set("@theme", theme); err != nil {
			t.Fatal(err)
		}
		if diags := a.ui.Validate(); len(diags) != 0 {
			t.Errorf("%s: Validate gave %v", theme, diags)
		}
	}
}

// Every action the document names has a handler, except the built-ins.
func TestHandlersCoverCatalog(t *testing.T) {
	a := newTestApp(t)
	n := 0
	for _, spec := range a.ui.Catalog() {
		if spec.Builtin {
			continue
		}
		n++
		if !spec.Registered {
			t.Errorf("no handler for %s (%v)", spec.Name, spec.Sources)
		}
	}
	if n < 30 {
		t.Errorf("the catalog has only %d host actions", n)
	}
}

func TestStartScreen(t *testing.T) {
	a := newTestApp(t)
	res := a.play(t)
	g := grid(res)
	for _, want := range []string{"chess", "vs computer", "White to move", "you", "computer L3", "moves", "engine", "eval", "? help", "q quit", "a    b    c"} {
		if !strings.Contains(g, want) {
			t.Errorf("start screen lacks %q:\n%s", want, g)
		}
	}
	if res.Dump.Wide {
		t.Error("the dump is wide: a figurine is not one column")
	}
	if focused(res) != "board" {
		t.Errorf("focus = %q, want the board", focused(res))
	}
	// One filled set for both sides; the colour tells them apart.
	for _, want := range []string{"♜", "♞", "♝", "♛", "♚", "♟"} {
		if !strings.Contains(g, want) {
			t.Errorf("the board has no %s", want)
		}
	}
}

func TestClickSelectsAndMoves(t *testing.T) {
	a := newTestApp(t, twoPlayers)
	res := a.click(t, "e2")
	wantActions(t, res, "cell")
	if a.num(t, "sel") != sq(t, "e2") {
		t.Fatalf("sel = %d, want e2", a.num(t, "sel"))
	}
	targets := asInts(a.get(t, "targets").([]any))
	if len(targets) != 2 || !contains(targets, sq(t, "e3")) || !contains(targets, sq(t, "e4")) {
		t.Errorf("targets = %v, want e3 and e4", targets)
	}
	// The cells show it: a marker on both targets, the selection on e2.
	cells := a.get(t, "cells").(map[string]any)
	e4 := cells[cellID(4, 4)].(map[string]any)
	e2 := cells[cellID(6, 4)].(map[string]any)
	if e4["tgt"] != true || e4["t"] != "•" || e2["sel"] != true {
		t.Errorf("e4 = %v, e2 = %v", e4, e2)
	}
	if !strings.Contains(grid(res), "•") {
		t.Errorf("no target marker on the board:\n%s", grid(res))
	}
	// Click again to deselect.
	a.click(t, "e2")
	if a.num(t, "sel") != -1 {
		t.Error("a second click on the piece did not deselect it")
	}
	// Another own piece switches the selection.
	a.click(t, "e2")
	a.click(t, "g1")
	if a.num(t, "sel") != sq(t, "g1") {
		t.Errorf("sel = %d, want g1", a.num(t, "sel"))
	}
	// A click on a square that is not a target clears the selection.
	a.click(t, "a5")
	if a.num(t, "sel") != -1 {
		t.Error("a click on a non-target square kept the selection")
	}
	// Select, then move.
	a.click(t, "e2")
	a.click(t, "e4")
	if got := a.fenNow(); got != "rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq e3 0 1" {
		t.Errorf("FEN = %s", got)
	}
	if a.num(t, "sel") != -1 || a.str(t, "turn") != "black" {
		t.Errorf("sel=%d turn=%s after the move", a.num(t, "sel"), a.str(t, "turn"))
	}
	// An enemy piece cannot be picked up.
	a.click(t, "e7")
	if a.num(t, "sel") != sq(t, "e7") {
		t.Errorf("black to move could not pick up e7: sel = %d", a.num(t, "sel"))
	}
	a.click(t, "e2") // white's pawn: not black's to move
	if a.num(t, "sel") != -1 {
		t.Errorf("black selected a white piece: sel = %d", a.num(t, "sel"))
	}
}

func TestCapturesAreMarked(t *testing.T) {
	a := newTestApp(t, twoPlayers)
	a.moves(t, "e4", "d5")
	a.click(t, "e4")
	caps := asInts(a.get(t, "caps").([]any))
	if len(caps) != 1 || caps[0] != sq(t, "d5") {
		t.Errorf("caps = %v, want d5", caps)
	}
	cells := a.get(t, "cells").(map[string]any)
	d5 := cells[cellID(3, 3)].(map[string]any)
	if d5["cap"] != true || d5["tgt"] != false || d5["t"] != "♟" {
		t.Errorf("d5 = %v, want a capture on a pawn", d5)
	}
	a.click(t, "d5")
	if a.g.SAN[len(a.g.SAN)-1] != "exd5" {
		t.Errorf("last move %s", a.g.SAN[len(a.g.SAN)-1])
	}
	if got := a.get(t, "captured").(map[string]any); got["w"] != "♟" {
		t.Errorf("captured = %v", got)
	}
}

func TestKeyboardCursor(t *testing.T) {
	a := newTestApp(t, twoPlayers)
	if a.num(t, "cur") != sq(t, "e2") {
		t.Fatalf("the cursor starts on %d, want e2", a.num(t, "cur"))
	}
	// The cursor is hidden until a key asks for it; the first enter only
	// shows it, a click hides it again.
	cellsNow := func() map[string]any { return a.get(t, "cells").(map[string]any) }
	if cellsNow()[cellID(6, 4)].(map[string]any)["cur"] != false {
		t.Error("the cursor is drawn before any key")
	}
	a.play(t, "enter")
	if a.num(t, "sel") != -1 || cellsNow()[cellID(6, 4)].(map[string]any)["cur"] != true {
		t.Errorf("the first enter selected (sel=%d) instead of showing the cursor", a.num(t, "sel"))
	}
	a.click(t, "a1")
	if cellsNow()[cellID(7, 0)].(map[string]any)["cur"] != false || a.num(t, "cur") != sq(t, "a1") {
		t.Error("a click did not hide the cursor")
	}
	a.play(t, "l", "l", "l", "l", "k") // a1 -> e1 -> e2, and the cursor is back
	if a.num(t, "cur") != sq(t, "e2") || cellsNow()[cellID(6, 4)].(map[string]any)["cur"] != true {
		t.Fatalf("the cursor is on %s", sqName(a.num(t, "cur")))
	}
	for _, c := range []struct {
		key  string
		want string
	}{
		{"up", "e3"}, {"k", "e4"}, {"down", "e3"}, {"j", "e2"},
		{"left", "d2"}, {"h", "c2"}, {"right", "d2"}, {"l", "e2"},
		{"home", "a2"}, {"end", "h2"},
	} {
		res := a.play(t, c.key)
		if got := a.num(t, "cur"); got != sq(t, c.want) {
			t.Errorf("%s: cursor on %s, want %s", c.key, sqName(got), c.want)
		}
		if len(actions(res)) != 1 {
			t.Errorf("%s fired %v", c.key, actions(res))
		}
	}
	// The edges hold.
	a.play(t, "end", "right", "right")
	if a.num(t, "cur") != sq(t, "h2") {
		t.Errorf("the cursor left the board: %s", sqName(a.num(t, "cur")))
	}
	for i := 0; i < 9; i++ {
		a.play(t, "up")
	}
	if a.num(t, "cur") != sq(t, "h8") {
		t.Errorf("the cursor is on %s, want h8", sqName(a.num(t, "cur")))
	}
	// Enter and space select, then move.
	a.play(t, "home", "down", "down", "down", "down", "down", "down") // a2
	if a.num(t, "cur") != sq(t, "a2") {
		t.Fatalf("cursor on %s", sqName(a.num(t, "cur")))
	}
	a.play(t, "enter")
	if a.num(t, "sel") != sq(t, "a2") {
		t.Errorf("enter did not select a2")
	}
	a.play(t, "up", "up", "space")
	if a.fenNow() != "rnbqkbnr/pppppppp/8/8/P7/8/1PPPPPPP/RNBQKBNR b KQkq a3 0 1" {
		t.Errorf("space did not play a4: %s", a.fenNow())
	}
	// The cursor is drawn.
	cells := a.get(t, "cells").(map[string]any)
	if cells[cellID(4, 0)].(map[string]any)["cur"] != true {
		t.Error("the cursor cell is not marked")
	}
}

func TestFlipKeepsTheCellsAndMovesTheData(t *testing.T) {
	a := newTestApp(t, twoPlayers)
	before := a.get(t, "cells").(map[string]any)
	if tl, br := before[cellID(0, 0)].(map[string]any), before[cellID(7, 4)].(map[string]any); tl["t"] != "♜" || tl["b"] != true || br["t"] != "♚" || br["w"] != true {
		t.Fatal("the board does not start with black's rook at the top left")
	}
	res := a.play(t, "f")
	wantActions(t, res, "flip")
	after := a.get(t, "cells").(map[string]any)
	if tl, br := after[cellID(0, 0)].(map[string]any), after[cellID(7, 3)].(map[string]any); tl["t"] != "♜" || tl["w"] != true || br["t"] != "♚" || br["b"] != true {
		t.Errorf("flipped: top left %v, bottom %v", after[cellID(0, 0)], after[cellID(7, 3)])
	}
	if a.get(t, "labels").(map[string]any)["f0"] != "h" || a.get(t, "labels").(map[string]any)["r0"] != "1" {
		t.Errorf("labels = %v", a.get(t, "labels"))
	}
	g := grid(res)
	if !strings.Contains(g, "h    g    f") {
		t.Errorf("the file labels are not flipped:\n%s", g)
	}
	// A click on the same screen cell is another square now.
	a.click(t, "e2")
	if a.num(t, "sel") != sq(t, "e2") {
		t.Errorf("the click on the flipped board selected %s", sqName(a.num(t, "sel")))
	}
	// Up goes toward rank 1 on the flipped board.
	a.play(t, "up")
	if a.num(t, "cur") != sq(t, "e1") {
		t.Errorf("up moved to %s on the flipped board", sqName(a.num(t, "cur")))
	}
	// The player strips swap.
	if top := a.get(t, "top").(map[string]any); top["name"] != "White" {
		t.Errorf("top = %v", top)
	}
}

func TestMoveBox(t *testing.T) {
	a := newTestApp(t, twoPlayers)
	res := a.play(t, ":")
	wantActions(t, res)
	if focused(res) != "cmd" {
		t.Fatalf("focus = %q after :, want cmd", focused(res))
	}
	if !strings.Contains(grid(res), "esc leave") {
		t.Errorf("the hint bar does not offer esc in the move box:\n%s", grid(res))
	}
	res = a.play(t, "text:Nf3", "enter")
	wantActions(t, res, "cmd_submit")
	if a.fenNow() != "rnbqkbnr/pppppppp/8/8/8/5N2/PPPPPPPP/RNBQKB1R b KQkq - 1 1" {
		t.Errorf("FEN = %s", a.fenNow())
	}
	if focused(res) != "board" || a.str(t, "cmd") != "" {
		t.Errorf("after a move: focus %q, cmd %q", focused(res), a.str(t, "cmd"))
	}
	// "/" opens it too; UCI works; lenient spellings work.
	a.play(t, "/", "text:e7e5", "enter")
	a.play(t, ":", "text:nc3", "enter")
	a.play(t, ":", "text:G8F6", "enter")
	if a.g.SAN[len(a.g.SAN)-1] != "Nf6" || len(a.g.SAN) != 4 {
		t.Errorf("moves = %v", a.g.SAN)
	}
	// An illegal move: a notice, the text stays, the focus stays.
	res = a.play(t, ":", "text:Qh5", "enter")
	if focused(res) != "cmd" {
		t.Errorf("focus = %q after an illegal move, want the move box", focused(res))
	}
	if n := a.str(t, "notice"); !strings.Contains(n, "illegal move") {
		t.Errorf("notice = %q", n)
	}
	if !strings.Contains(grid(res), "illegal move: Qh5") {
		t.Errorf("the notice is not on screen:\n%s", grid(res))
	}
	if a.str(t, "cmd") != "Qh5" {
		t.Errorf("the text was cleared: %q", a.str(t, "cmd"))
	}
	// A legal correction goes through and clears the notice.
	a.play(t, "ctrl+u", "text:d4", "enter")
	if a.str(t, "notice") != "" || len(a.g.SAN) != 5 {
		t.Errorf("notice %q, moves %v", a.str(t, "notice"), a.g.SAN)
	}
	// esc leaves the box and clears it.
	res = a.play(t, ":", "text:xyz", "esc")
	wantActions(t, res, "cmd_cancel")
	if focused(res) != "board" || a.str(t, "cmd") != "" {
		t.Errorf("after esc: focus %q, cmd %q", focused(res), a.str(t, "cmd"))
	}
	// Ambiguity is explained.
	b := newTestApp(t, twoPlayers, func(o *options) { o.fen = "4k3/8/8/8/8/8/8/1N2KN2 w - - 0 1" })
	b.play(t, ":", "text:Nd2", "enter")
	if n := b.str(t, "notice"); !strings.Contains(n, "ambiguous") {
		t.Errorf("notice = %q, want an ambiguity", n)
	}
}

func TestTypingDoesNotTriggerKeys(t *testing.T) {
	// q in the move box types a q; it must not quit.
	a := newTestApp(t, twoPlayers)
	res := a.play(t, ":", "text:qrnuhfd")
	if res.Quit || len(actions(res)) != 0 {
		t.Errorf("typing fired %v, quit=%v", actions(res), res.Quit)
	}
	if a.str(t, "cmd") != "qrnuhfd" {
		t.Errorf("cmd = %q", a.str(t, "cmd"))
	}
}

func TestPromotion(t *testing.T) {
	const fen = "4k3/P7/8/8/8/8/8/4K3 w - - 0 1"
	start := func(t *testing.T) *testApp {
		return newTestApp(t, twoPlayers, func(o *options) { o.fen = fen })
	}
	// By clicks: the dialog asks.
	a := start(t)
	a.click(t, "a7")
	res := a.click(t, "a8")
	if !a.flag(t, "promo_open") {
		t.Fatalf("no promotion dialog:\n%s", grid(res))
	}
	g := grid(res)
	for _, want := range []string{"promote to", "queen", "rook", "bishop", "knight"} {
		if !strings.Contains(g, want) {
			t.Errorf("dialog lacks %q:\n%s", want, g)
		}
	}
	if focused(res) != "promo-q" {
		t.Errorf("focus = %q, want the queen button", focused(res))
	}
	// enter takes the default: a queen, with check.
	a.play(t, "enter")
	if a.g.SAN[0] != "a8=Q+" || a.flag(t, "promo_open") {
		t.Errorf("moves %v, dialog open %v", a.g.SAN, a.flag(t, "promo_open"))
	}
	if focused(a.play(t)) != "board" {
		t.Error("the focus did not return to the board")
	}
	// The keys q r b n each choose.
	for key, want := range map[string]string{"q": "a8=Q+", "r": "a8=R+", "b": "a8=B", "n": "a8=N"} {
		a := start(t)
		a.moves(t, "a8")
		if !a.flag(t, "promo_open") {
			t.Fatalf("typing a8 did not ask for the piece")
		}
		res := a.play(t, key)
		if len(a.g.SAN) != 1 || a.g.SAN[0] != want {
			t.Errorf("%s: moves %v, want %s", key, a.g.SAN, want)
		}
		wantActions(t, res, "promo_"+key)
	}
	// esc cancels: no move, the pawn stays.
	a = start(t)
	a.click(t, "a7")
	a.click(t, "a8")
	res = a.play(t, "esc")
	wantActions(t, res, "promo_cancel")
	if len(a.g.SAN) != 0 || a.flag(t, "promo_open") || a.fenNow() != "4k3/P7/8/8/8/8/8/4K3 w - - 0 1" {
		t.Errorf("cancel: %v %s", a.g.SAN, a.fenNow())
	}
	// A typed promotion with the piece does not ask.
	a = start(t)
	a.moves(t, "a8=N")
	if a.flag(t, "promo_open") || a.g.SAN[0] != "a8=N" {
		t.Errorf("a8=N: moves %v, dialog %v", a.g.SAN, a.flag(t, "promo_open"))
	}
	// A capture-promotion by click asks too, and the pawn's target is marked.
	b := newTestApp(t, twoPlayers, func(o *options) { o.fen = "1n2k3/P7/8/8/8/8/8/4K3 w - - 0 1" })
	b.click(t, "a7")
	if caps := asInts(b.get(t, "caps").([]any)); len(caps) != 1 || caps[0] != sq(t, "b8") {
		t.Errorf("caps = %v", caps)
	}
	b.click(t, "b8")
	b.play(t, "n")
	if b.g.SAN[0] != "axb8=N" {
		t.Errorf("moves %v", b.g.SAN)
	}
}

func TestCastlingAndEnPassantByTyping(t *testing.T) {
	a := newTestApp(t, twoPlayers)
	a.moves(t, "e4", "a6", "Nf3", "a5", "Be2", "b6", "O-O")
	if a.fenNow() != "rnbqkbnr/2pppppp/1p6/p7/4P3/5N2/PPPPBPPP/RNBQ1RK1 b kq - 1 4" {
		t.Errorf("FEN = %s", a.fenNow())
	}
	b := newTestApp(t, twoPlayers)
	b.moves(t, "e4", "a6", "e5", "d5", "exd6")
	if b.fenNow() != "rnbqkbnr/1pp1pppp/p2P4/8/8/8/PPPP1PPP/RNBQKBNR b KQkq - 0 3" {
		t.Errorf("en passant: FEN = %s", b.fenNow())
	}
	// The same by clicks.
	c := newTestApp(t, twoPlayers)
	c.moves(t, "e4", "a6", "e5", "d5")
	c.click(t, "e5")
	if caps := asInts(c.get(t, "caps").([]any)); len(caps) != 1 || caps[0] != sq(t, "d6") {
		t.Errorf("caps = %v, want the en passant square d6", caps)
	}
	c.click(t, "d6")
	if c.g.SAN[len(c.g.SAN)-1] != "exd6" {
		t.Errorf("moves %v", c.g.SAN)
	}
}

func TestMoveListAndStatus(t *testing.T) {
	a := newTestApp(t, twoPlayers)
	a.moves(t, "e4", "e5", "Qh5", "Nc6", "Bc4", "Nf6", "Qxf7")
	rows := a.get(t, "mrows").([]any)
	if len(rows) != 4 {
		t.Fatalf("%d rows", len(rows))
	}
	last := rows[3].(map[string]any)
	if last["n"] != float64(4) || last["w"] != "Qxf7#" || last["b"] != "" || last["cw"] != true {
		t.Errorf("last row = %v", last)
	}
	res := a.play(t)
	g := grid(res)
	for _, want := range []string{"1.   e4", "e5", "Qxf7#", "Checkmate · 1–0", "White wins"} {
		if !strings.Contains(g, want) {
			t.Errorf("grid lacks %q:\n%s", want, g)
		}
	}
	if st := a.get(t, "status").(map[string]any); st["over"] != true {
		t.Errorf("status = %v", st)
	}
	// Nothing moves after the game: no cursor selection either.
	a.click(t, "e4")
	if a.num(t, "sel") != -1 {
		t.Error("selected a piece after checkmate")
	}
	if !strings.Contains(a.str(t, "notice"), "game is over") {
		t.Errorf("notice = %q", a.str(t, "notice"))
	}
}

func TestCheckIsShown(t *testing.T) {
	a := newTestApp(t, twoPlayers)
	a.moves(t, "e4", "f6", "Qh5")
	res := a.play(t)
	g := grid(res)
	if !strings.Contains(g, "Check · Black to move") {
		t.Errorf("no check in the status:\n%s", g)
	}
	cells := a.get(t, "cells").(map[string]any)
	if cells[cellID(0, 4)].(map[string]any)["chk"] != true {
		t.Error("the king in check is not marked")
	}
	if !strings.Contains(g, "Qh5+") && !strings.Contains(g, "Qh5") {
		t.Error("the move list lacks Qh5")
	}
}

func TestLastMoveAndCapturedAndMaterial(t *testing.T) {
	a := newTestApp(t, twoPlayers)
	a.moves(t, "e4", "d5", "exd5", "Qxd5", "Nc3", "Qxg2")
	cells := a.get(t, "cells").(map[string]any)
	// Qxg2: from d5 (screen row 3, column 3) to g2 (row 6, column 6).
	from, to := cells[cellID(3, 3)].(map[string]any), cells[cellID(6, 6)].(map[string]any)
	if from["last"] != true || to["last"] != true {
		t.Errorf("the last move's squares are not marked: %v %v", from, to)
	}
	capd := a.get(t, "captured").(map[string]any)
	if capd["w"] != "♟" || capd["b"] != "♟♟" {
		t.Errorf("captured = %v", capd)
	}
	// White has lost two pawns, black one: black is up one.
	if a.num(t, "matdiff") != -1 {
		t.Errorf("matdiff = %d, want -1", a.num(t, "matdiff"))
	}
	bot := a.get(t, "bot").(map[string]any)
	top := a.get(t, "top").(map[string]any)
	if top["cap"] != "♟♟" || top["lead"] != "+1" || bot["cap"] != "♟" || bot["lead"] != "" {
		t.Errorf("top = %v, bot = %v", top, bot)
	}
}

func TestMouseToggleStopsClicks(t *testing.T) {
	a := newTestApp(t, twoPlayers)
	x, y := a.cellCenter(t, 100, 24, "e2")
	res := a.play(t, "m")
	wantActions(t, res, "toggle_mouse")
	if a.flag(t, "mouse") {
		t.Fatal("the mouse is still on")
	}
	res = a.play(t, "click:"+strconv.Itoa(x)+","+strconv.Itoa(y))
	if len(actions(res)) != 0 {
		t.Errorf("a click fired %v with the mouse off", actions(res))
	}
	a.play(t, "m")
	res = a.play(t, "click:"+strconv.Itoa(x)+","+strconv.Itoa(y))
	wantActions(t, res, "cell")
}

func TestLettersMode(t *testing.T) {
	a := newTestApp(t, func(o *options) { o.letters = true })
	res := a.play(t)
	g := grid(res)
	for _, want := range []string{" r ", " k ", " K ", " Q ", " P "} {
		if !strings.Contains(g, want) {
			t.Errorf("letters mode lacks %q:\n%s", want, g)
		}
	}
	for _, line := range res.Dump.Grid[1:17] {
		if board := string([]rune(line)[:45]); strings.ContainsAny(board, "♜♞♝♛♚♟") {
			t.Errorf("figurines on a letters board: %q", board)
		}
	}
	res = a.play(t, "L")
	wantActions(t, res, "toggle_letters")
	if !strings.Contains(grid(res), "♜") {
		t.Error("L did not bring the figurines back")
	}
}

func TestNoticeClearsOnTheNextActionOrAfterAWhile(t *testing.T) {
	a := newTestApp(t, twoPlayers)
	a.play(t, "u") // nothing to undo
	if a.str(t, "notice") != "nothing to undo" {
		t.Fatalf("notice = %q", a.str(t, "notice"))
	}
	if !strings.Contains(grid(a.play(t)), "nothing to undo") {
		t.Error("the notice is not on screen")
	}
	// A key that fires no action does not clear it.
	a.play(t, "z")
	if a.str(t, "notice") != "nothing to undo" {
		t.Errorf("notice = %q after a key that did nothing", a.str(t, "notice"))
	}
	// The next action does.
	a.play(t, "down")
	if a.str(t, "notice") != "" {
		t.Errorf("notice = %q after an action", a.str(t, "notice"))
	}
	// Or the ticks, after noticeTicks of them.
	a.play(t, "u")
	for i := 0; i < noticeTicks-1; i++ {
		a.tickAfter(t, tickStep)
	}
	if a.str(t, "notice") == "" {
		t.Fatalf("notice gone after %d ticks", noticeTicks-1)
	}
	a.tickAfter(t, tickStep)
	if a.str(t, "notice") != "" {
		t.Errorf("notice = %q after %d ticks", a.str(t, "notice"), noticeTicks)
	}
}

func TestOnErrorShowsANoticeAndQuitStillQuits(t *testing.T) {
	a := newTestApp(t)
	if err := a.onError(nil, errTest("boom")); err != nil {
		t.Fatalf("onError returned %v", err)
	}
	if a.str(t, "notice") != "boom" || !strings.Contains(grid(a.play(t)), "boom") {
		t.Errorf("notice = %q", a.str(t, "notice"))
	}
	if err := a.onError(nil, tuimarkErrQuit()); err == nil {
		t.Error("ErrQuit was swallowed")
	}
	a.play(t, "down")
	if a.str(t, "notice") != "" {
		t.Error("the next action did not clear the notice")
	}
}
