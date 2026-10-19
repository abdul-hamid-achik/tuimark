package main

import (
	"math"
	"math/bits"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

// ---- colours ----

var tokenRe = regexp.MustCompile(`--([a-z-]+):\s*(#[0-9a-fA-F]{6});`)

// themeTokens are the :root colours of theme.tcss (the dark palette; the
// board's colours are the same in the light one).
func themeTokens(t testing.TB) map[string]string {
	t.Helper()
	raw, err := os.ReadFile("theme.tcss")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	text = text[:strings.Index(text, "@media")]
	out := map[string]string{}
	for _, m := range tokenRe.FindAllStringSubmatch(text, -1) {
		out[m[1]] = strings.ToLower(m[2])
	}
	return out
}

// themeTokensFor is themeTokens with the `@media (theme: light)` palette
// applied on top for the light theme.
func themeTokensFor(t testing.TB, theme string) map[string]string {
	t.Helper()
	out := themeTokens(t)
	if theme != "light" {
		return out
	}
	raw, err := os.ReadFile("theme.tcss")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	i := strings.Index(text, "@media (theme: light)")
	if i < 0 {
		t.Fatal("theme.tcss has no light palette")
	}
	text = text[i:]
	text = text[:strings.Index(text, "\n}\n")]
	for _, m := range tokenRe.FindAllStringSubmatch(text, -1) {
		out[m[1]] = strings.ToLower(m[2])
	}
	return out
}

func luminance(hex string) float64 {
	var c [3]float64
	for i := range c {
		v, _ := strconv.ParseUint(hex[1+2*i:3+2*i], 16, 8)
		f := float64(v) / 255
		if f <= 0.03928 {
			c[i] = f / 12.92
		} else {
			c[i] = math.Pow((f+0.055)/1.055, 2.4)
		}
	}
	return 0.2126*c[0] + 0.7152*c[1] + 0.0722*c[2]
}

func contrast(a, b string) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// Both piece colours read on every square state: at least 3:1.
func TestPiecesReadOnEverySquareState(t *testing.T) {
	tok := themeTokens(t)
	for _, bg := range []string{"sq-light", "sq-dark", "last-light", "last-dark", "hint-light", "hint-dark", "sel", "cursor", "cap", "chk"} {
		for _, fg := range []string{"pw", "pb"} {
			if tok[bg] == "" || tok[fg] == "" {
				t.Fatalf("theme.tcss lacks --%s or --%s", bg, fg)
			}
			if c := contrast(tok[fg], tok[bg]); c < 3 {
				t.Errorf("--%s on --%s has contrast %.2f, want at least 3", fg, bg, c)
			}
		}
	}
	// The target dot too, on the squares it appears on.
	if c := contrast(tok["tgt-light"], tok["sq-light"]); c < 3 {
		t.Errorf("the target dot on a light square: %.2f", c)
	}
	if c := contrast(tok["tgt-dark"], tok["sq-dark"]); c < 3 {
		t.Errorf("the target dot on a dark square: %.2f", c)
	}
}

// Both sides use the filled figurines, told apart by colour.
func TestOneFilledFigurineSetTwoColours(t *testing.T) {
	for typ := Pawn; typ <= King; typ++ {
		w, b := mk(White, typ).Glyph(), mk(Black, typ).Glyph()
		if w != b || w == "" {
			t.Errorf("type %d: glyphs %q and %q", typ, w, b)
		}
	}
	if got := mk(White, King).Glyph() + mk(White, Queen).Glyph() + mk(White, Rook).Glyph() + mk(White, Bishop).Glyph() + mk(White, Knight).Glyph() + mk(White, Pawn).Glyph(); got != "♚♛♜♝♞♟" {
		t.Errorf("glyphs = %s", got)
	}
	a := newTestApp(t, twoPlayers)
	res := a.play(t)
	if strings.ContainsAny(grid(res), "♔♕♖♗♘♙") {
		t.Errorf("outline figurines on screen:\n%s", grid(res))
	}
	// White's pieces are told apart by class, and the strips follow suit.
	cells := a.get(t, "cells").(map[string]any)
	if w := cells[cellID(7, 0)].(map[string]any); w["w"] != true || w["t"] != "♜" {
		t.Errorf("a1 = %v", w)
	}
	if top := a.get(t, "top").(map[string]any); top["black"] != true || top["capwhite"] != true || top["glyph"] != "♚" {
		t.Errorf("top = %v", top)
	}
	if bot := a.get(t, "bot").(map[string]any); bot["white"] != true || bot["capblack"] != true {
		t.Errorf("bot = %v", bot)
	}
}

// bgAt is the painted background of the board cell showing the square.
func (a *testApp) bgAt(t testing.TB, theme, square string) (bg, fg string) {
	t.Helper()
	if err := a.ui.Set("@theme", theme); err != nil {
		t.Fatal(err)
	}
	res, err := a.ui.Play(tuimark.PlayOptions{Cols: 100, Rows: 24, Styles: true})
	if err != nil {
		t.Fatal(err)
	}
	flip := a.flag(t, "flip")
	s := sq(t, square)
	for r := 0; r < 8; r++ {
		for c := 0; c < 8; c++ {
			if screenSq(r, c, flip) != s {
				continue
			}
			for _, n := range res.Dump.Nodes {
				if n.ID != cellID(r, c) {
					continue
				}
				x, y := n.X+n.W/2, n.Y
				for _, sp := range res.Dump.Styles[y] {
					if x >= sp.X && x < sp.X+sp.W {
						return sp.BG, sp.FG
					}
				}
			}
		}
	}
	t.Fatalf("no cell for %s", square)
	return
}

// The states paint over the square in a fixed order: cursor, check,
// selection, capture, hint, last move. Every combination, on a light and a
// dark square, in both themes.
func TestSquareStatesPaintInOrder(t *testing.T) {
	tok := themeTokens(t)
	order := []string{"cur", "chk", "sel", "cap", "hint", "last"}
	for _, theme := range []string{"dark", "light"} {
		for _, square := range []string{"e4", "d4"} { // e4 is light, d4 is dark
			shade := "light"
			if (file(sq(t, square))+rank(sq(t, square)))%2 == 0 {
				shade = "dark"
			}
			a := newTestApp(t, twoPlayers)
			s := sq(t, square)
			other := sq(t, "a1")
			for mask := 0; mask < 1<<len(order); mask++ {
				if theme == "light" && bits.OnesCount(uint(mask)) > 2 {
					continue // the board's colours are the same in both themes: pairs are enough
				}
				on := map[string]bool{}
				for i, name := range order {
					on[name] = mask&(1<<i) != 0
				}
				set := func(path string, v any) {
					if err := a.ui.Set(path, v); err != nil {
						t.Fatal(err)
					}
				}
				set("curon", on["cur"])
				set("cur", s)
				set("sel", map[bool]int{true: s, false: -1}[on["sel"]])
				set("caps", map[bool][]any{true: {s}, false: {}}[on["cap"]])
				set("checksq", map[bool]int{true: s, false: -1}[on["chk"]])
				set("hint", map[bool][]any{true: {other, s}, false: {-1, -1}}[on["hint"]])
				set("last", map[bool][]any{true: {other, s}, false: {-1, -1}}[on["last"]])
				want := tok["sq-"+shade]
				for _, name := range order {
					if on[name] {
						switch name {
						case "cur":
							want = tok["cursor"]
						case "chk":
							want = tok["chk"]
						case "sel":
							want = tok["sel"]
						case "cap":
							want = tok["cap"]
						case "hint":
							want = tok["hint-"+shade]
						case "last":
							want = tok["last-"+shade]
						}
						break
					}
				}
				if got, _ := a.bgAt(t, theme, square); got != want {
					t.Fatalf("%s %s square with %v: painted %s, want %s", theme, shade, on, got, want)
				}
			}
		}
	}
}

// The repros of the order: a recapture keeps its red, a cursor on the last
// move shows, a selected piece on the hint shows, a checked king on the hint
// shows.
func TestSquareStatesInRealGames(t *testing.T) {
	tok := themeTokens(t)
	for _, theme := range []string{"dark", "light"} {
		a := newTestApp(t, twoPlayers)
		a.moves(t, "e4", "d5")
		a.click(t, "e4")
		if bg, _ := a.bgAt(t, theme, "d5"); bg != tok["cap"] {
			t.Errorf("%s: the recapture square is %s, want the capture red %s", theme, bg, tok["cap"])
		}
		a.click(t, "e4") // deselect
		for path, v := range map[string]any{"curon": true, "cur": sq(t, "d5")} {
			if err := a.ui.Set(path, v); err != nil {
				t.Fatal(err)
			}
		}
		if bg, _ := a.bgAt(t, theme, "d5"); bg != tok["cursor"] {
			t.Errorf("%s: the cursor on the last move is %s, want %s", theme, bg, tok["cursor"])
		}

		b := newTestApp(t, twoPlayers)
		b.play(t, "H")
		h := asInts(b.get(t, "hint").([]any))
		from := sqName(h[0])
		b.click(t, from)
		if bg, _ := b.bgAt(t, theme, from); bg != tok["sel"] {
			t.Errorf("%s: the selected piece on the hint is %s, want %s", theme, bg, tok["sel"])
		}

		c := newTestApp(t, twoPlayers, func(o *options) { o.fen = "4k3/8/8/8/8/8/4q3/4K3 w - - 0 1" })
		c.play(t, "H")
		if bg, _ := c.bgAt(t, theme, "e1"); bg != tok["chk"] {
			t.Errorf("%s: the checked king on the hint is %s, want %s", theme, bg, tok["chk"])
		}
	}
}

// White's share of the evaluation bar is light and the track dark, in both
// themes: the bar does not follow the theme's text colour.
func TestEvaluationBarIsWhiteOnDark(t *testing.T) {
	a := newTestApp(t, twoPlayers)
	a.moves(t, "f3", "e5", "g4", "Qh4") // black wins: the bar is empty of white
	b := newTestApp(t, twoPlayers)
	b.moves(t, "e4", "f6", "d4", "g5", "Qh5") // not over
	c := newTestApp(t, twoPlayers, func(o *options) { o.fen = "4k3/8/8/8/8/8/8/Q3K3 w - - 0 1" })
	for _, theme := range []string{"dark", "light"} {
		tok := themeTokensFor(t, theme)
		if err := c.ui.Set("@theme", theme); err != nil {
			t.Fatal(err)
		}
		res, _ := c.ui.Play(tuimark.PlayOptions{Cols: 100, Rows: 24, Styles: true})
		found := false
		for _, n := range res.Dump.Nodes {
			if n.ID != "evalbar" {
				continue
			}
			found = true
			row := []rune(res.Dump.Grid[n.Y])
			fill, track := -1, -1
			for x := n.X; x < n.X+n.W; x++ {
				if row[x] == '█' && fill < 0 {
					fill = x
				}
				if row[x] == '░' && track < 0 {
					track = x
				}
			}
			if fill < 0 {
				t.Fatalf("%s: white is a queen up and has no fill:\n%s", theme, grid(res))
			}
			for _, sp := range res.Dump.Styles[n.Y] {
				if fill >= sp.X && fill < sp.X+sp.W {
					if sp.FG != tok["bar-white"] || sp.BG != tok["bar-track"] {
						t.Errorf("%s: the fill is %s on %s, want %s on %s", theme, sp.FG, sp.BG, tok["bar-white"], tok["bar-track"])
					}
					if luminance(sp.FG) < luminance(sp.BG) {
						t.Errorf("%s: white's share is darker than the track", theme)
					}
					// A full bar must not vanish into the screen around it.
					if c := contrast(sp.FG, tok["bg"]); c < 1.2 {
						t.Errorf("%s: white's share %s is %.2f:1 against the background %s", theme, sp.FG, c, tok["bg"])
					}
				}
			}
		}
		if !found {
			t.Fatal("no evaluation bar")
		}
	}
	_, _ = a, b
}

// ---- results and notices ----

func TestResignDoesNothingOnceTheGameIsOver(t *testing.T) {
	// A draw claimed by the computer behind the dialog.
	a := newTestApp(t, func(o *options) { o.fen = "4k3/8/8/8/8/8/4P3/R3K3 b - - 100 80" })
	pending := a.deferJobs()
	a.tickAfter(t, tickStep)
	a.play(t, "r")
	if !a.flag(t, "resign_open") {
		t.Fatal("no resign dialog on the computer's turn")
	}
	for _, f := range *pending {
		f()
	}
	if a.str(t, "result") != "1/2-1/2" {
		t.Fatalf("the computer did not claim: %s", a.str(t, "result"))
	}
	res := a.play(t, "y")
	if a.str(t, "result") != "1/2-1/2" || a.str(t, "reason") != "fifty-move rule" {
		t.Errorf("resigning changed %s %s", a.str(t, "result"), a.str(t, "reason"))
	}
	if a.flag(t, "resign_open") || focused(res) != "board" {
		t.Errorf("the dialog stays: open=%v focus=%q", a.flag(t, "resign_open"), focused(res))
	}
	if !strings.Contains(a.str(t, "notice"), "ended meanwhile") {
		t.Errorf("notice = %q", a.str(t, "notice"))
	}

	// A mate behind the dialog.
	c := newTestApp(t, func(o *options) { o.fen = "r6k/8/8/4N3/8/8/5PPP/6K1 w - - 0 1" })
	pc := c.deferJobs()
	c.moves(t, "Nc4")
	c.play(t, "r")
	for _, f := range *pc {
		f()
	}
	if c.str(t, "result") != "0-1" || c.str(t, "reason") != "checkmate" {
		t.Fatalf("the computer did not mate: %v %s %s", c.g.SAN, c.str(t, "result"), c.str(t, "reason"))
	}
	c.play(t, "y")
	if c.str(t, "reason") != "checkmate" {
		t.Errorf("resigning turned the mate into %s", c.str(t, "reason"))
	}
}

// The reply to 1.e4 is computed, then you take back and play 1.d4: the late
// reply must not land on the new game, even though the search itself
// finished with a result.
func TestALateFinishedReplyIsDropped(t *testing.T) {
	a := newTestApp(t)
	a.dispatch = func(func()) {}
	a.moves(t, "e4")
	a.mu.Lock()
	j := &job{gen: a.gen, pos: *a.g.Cur(), keys: append([]uint64(nil), a.g.Keys...), lim: levelLimits[3], stop: a.stop}
	a.mu.Unlock()
	res := Search(j.pos, j.keys, j.lim, nil) // done before the undo
	a.play(t, "u")
	a.moves(t, "d4")
	// The reply is a legal move in the new position too, so only the
	// generation can stop it.
	if !a.g.Cur().IsLegal(res.Move) {
		t.Fatalf("test setup: %s is not legal after 1.d4", res.Move)
	}
	a.finish(j, res, 0)
	if got := strings.Join(a.g.SAN, " "); got != "d4" {
		t.Errorf("game = %q after the late answer, want d4 alone", got)
	}
}

func TestUndoAsBlackWhenTheComputerOpened(t *testing.T) {
	a := newTestApp(t, func(o *options) { o.side = "black" })
	a.tickAfter(t, tickStep) // the computer opens
	if len(a.g.SAN) != 1 {
		t.Fatalf("moves %v", a.g.SAN)
	}
	first := a.g.SAN[0]
	a.play(t, "u")
	if !strings.Contains(a.str(t, "notice"), "nothing to take back") || len(a.g.SAN) != 1 || a.g.SAN[0] != first {
		t.Errorf("notice %q, moves %v", a.str(t, "notice"), a.g.SAN)
	}
	if a.flag(t, "thinking") {
		t.Error("the computer started thinking again")
	}
	// After your own move it takes back both.
	a.moves(t, "e5")
	a.play(t, "u")
	if len(a.g.SAN) != 1 {
		t.Errorf("moves %v after undoing your move and the reply", a.g.SAN)
	}
}

func TestClickingLeavesTheMoveBox(t *testing.T) {
	a := newTestApp(t, twoPlayers)
	if focused(a.play(t, ":")) != "cmd" {
		t.Fatal("no move box")
	}
	res := a.click(t, "e2")
	if focused(res) != "board" {
		t.Errorf("focus = %q after a click", focused(res))
	}
	res = a.play(t, "up", "enter")
	wantActions(t, res, "up", "select")
}

func TestUpAndDownDoNothingInAReview(t *testing.T) {
	a := newTestApp(t, twoPlayers)
	a.moves(t, "e4", "e5", "Nf3")
	a.play(t, "v", "left")
	ply := a.num(t, "ply")
	for _, key := range []string{"up", "down", "k", "j"} {
		a.play(t, key)
		if a.num(t, "ply") != ply {
			t.Errorf("%s moved the review to ply %d", key, a.num(t, "ply"))
		}
	}
	a.play(t, "h")
	if a.num(t, "ply") != ply-1 {
		t.Errorf("h did not step back: %d", a.num(t, "ply"))
	}
}

// ---- review: play on ----

func loadOpera(t *testing.T) *testApp {
	path := filepath.Join(t.TempDir(), "opera.pgn")
	if err := os.WriteFile(path, []byte("[White \"Morphy\"]\n[Black \"Duke\"]\n\n"+operaGame+" 1-0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return newTestApp(t, func(o *options) { o.pgn = path })
}

func TestPlayingOnKeepsTheRestAsRedo(t *testing.T) {
	a := loadOpera(t)
	a.play(t, "right", "right", "right", "space")
	if a.flag(t, "review") || len(a.g.SAN) != 3 {
		t.Fatalf("review=%v moves=%v", a.flag(t, "review"), a.g.SAN)
	}
	n := a.str(t, "notice")
	if !strings.Contains(n, "after 2.Nf3") || !strings.Contains(n, "30 moves") || !strings.Contains(n, "U brings") {
		t.Errorf("notice = %q", n)
	}
	// U brings the rest back, one move at a time.
	a.play(t, "U")
	if len(a.g.SAN) != 4 || a.g.SAN[3] != "d6" {
		t.Fatalf("after U: %v", a.g.SAN)
	}
	for i := 0; i < 29; i++ {
		a.play(t, "U")
	}
	if len(a.g.SAN) != 33 || a.g.Result != "1-0" {
		t.Errorf("the whole game is not back: %d plies, %s", len(a.g.SAN), a.g.Result)
	}
	// A black move is named with dots.
	b := loadOpera(t)
	b.play(t, "right", "right", "right", "right", "enter")
	if n := b.str(t, "notice"); !strings.Contains(n, "after 2...d6") {
		t.Errorf("notice = %q", n)
	}
	// A new move after playing on drops the rest for good.
	c := loadOpera(t)
	c.play(t, "right", "right", "enter")
	c.moves(t, "Nf3")
	c.play(t, "U")
	if c.str(t, "notice") != "nothing to redo" {
		t.Errorf("notice = %q", c.str(t, "notice"))
	}
	// At the end of the game there is nothing to keep.
	d := loadOpera(t)
	d.play(t, "end", "enter")
	if !strings.Contains(d.str(t, "notice"), "back to the game") || len(d.g.SAN) != 33 {
		t.Errorf("notice %q, %d plies", d.str(t, "notice"), len(d.g.SAN))
	}
}

// ---- dialogs ----

func TestHelpScrollsByKeyboardAtSmallSizes(t *testing.T) {
	a := newTestApp(t)
	res := a.playAt(t, 40, 14, "?")
	if g := grid(res); strings.Contains(g, "quits") || !strings.Contains(g, "Click a piece") {
		t.Fatalf("the help starts scrolled:\n%s", g)
	}
	res = a.playAt(t, 40, 14, "tab")
	if focused(res) != "help-scroll" {
		t.Fatalf("tab focused %q, want the help text", focused(res))
	}
	res = a.playAt(t, 40, 14, "end")
	g := grid(res)
	if !strings.Contains(g, "quits") || strings.Contains(g, "Click a piece") {
		t.Errorf("end did not scroll the help to its last lines:\n%s", g)
	}
	// enter and esc still close it.
	res = a.playAt(t, 40, 14, "enter")
	wantActions(t, res, "help_close")
	a.playAt(t, 40, 14, "?", "tab", "esc")
	if a.flag(t, "help_open") {
		t.Error("esc did not close the help from the text")
	}
}

func TestHelpMentionsEveryKeyGroup(t *testing.T) {
	raw, err := os.ReadFile("app.tui")
	if err != nil {
		t.Fatal(err)
	}
	doc := string(raw)
	help := doc[strings.Index(doc, `<scroll id="help-scroll"`):]
	help = help[:strings.Index(help, "</scroll>")]
	flat := strings.Join(strings.Fields(help), " ")
	for _, want := range []string{"n starts a new game", "q quits", "tab moves between the board and the move box", "esc leaves the box or a dialog", "H shows the best move", "Arrows (and h j k l)", "enter plays on from there"} {
		if !strings.Contains(flat, want) {
			t.Errorf("the help lacks %q", want)
		}
	}
	// And they are on screen, wrapped, once the text is tall enough to show.
	a := newTestApp(t)
	g := grid(a.playAt(t, 120, 40, "?"))
	for _, want := range []string{"starts a new game", "tab moves between the board and the", "dialog, q quits"} {
		if !strings.Contains(g, want) {
			t.Errorf("the help on screen lacks %q:\n%s", want, g)
		}
	}
}

func TestPGNDialogIsOneParagraphAndTheFileIsWrapped(t *testing.T) {
	path := filepath.Join(t.TempDir(), "g.pgn")
	a := newTestApp(t, twoPlayers, func(o *options) { o.save = path })
	for _, san := range operaMoves() {
		a.moves(t, san)
	}
	// The dialog text has its movetext on one line (the dialog wraps it).
	a.save = ""
	a.play(t, "p")
	text := a.str(t, "pgn_text")
	_, move, ok := strings.Cut(text, "\n\n")
	if !ok || strings.Contains(move, "\n") || !strings.HasPrefix(move, "1. e4 e5") || !strings.HasSuffix(move, "17. Rd8# 1-0") {
		t.Errorf("movetext in the dialog:\n%q", move)
	}
	a.play(t, "esc")
	// The file keeps the 80-column wrap.
	a.save = path
	a.play(t, "p")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	long := 0
	for _, l := range lines {
		if len(l) > 80 {
			t.Errorf("a line of %d columns in the file", len(l))
		}
		if len(l) > 70 {
			long++
		}
	}
	if long < 2 {
		t.Errorf("the file is not wrapped: %d long lines", long)
	}
}

func TestPGNDialogFollowsTheGameBehindIt(t *testing.T) {
	a := newTestApp(t)
	pending := a.deferJobs()
	a.moves(t, "e4")
	a.play(t, "p") // open while the computer thinks
	if !strings.Contains(a.str(t, "pgn_text"), "1. e4 *") {
		t.Fatalf("pgn_text = %q", a.str(t, "pgn_text"))
	}
	for _, f := range *pending {
		f()
	}
	if len(a.g.SAN) != 2 {
		t.Fatalf("moves %v", a.g.SAN)
	}
	if want := "1. e4 " + a.g.SAN[1] + " *"; !strings.Contains(a.str(t, "pgn_text"), want) {
		t.Errorf("the dialog is stale: %q, want %q in it", a.str(t, "pgn_text"), want)
	}
}

func TestPGNTimeControlIsWhole(t *testing.T) {
	a := newTestApp(t, twoPlayers, func(o *options) { o.clock = "3+2" })
	a.play(t, "p")
	if !strings.Contains(a.str(t, "pgn_text"), `[TimeControl "180+2"]`) {
		t.Errorf("pgn_text = %q", a.str(t, "pgn_text"))
	}
	b := newTestApp(t, twoPlayers, func(o *options) { o.clock = "0.5+0" })
	b.play(t, "p")
	if !strings.Contains(b.str(t, "pgn_text"), `[TimeControl "30+0"]`) {
		t.Errorf("pgn_text = %q", b.str(t, "pgn_text"))
	}
}

func TestEnginePanelAfterTheGame(t *testing.T) {
	a := newTestApp(t, twoPlayers)
	if v := a.get(t, "engine_view").(map[string]any); v["line"] != "idle: H asks for a hint" {
		t.Errorf("engine_view = %v", v)
	}
	a.moves(t, "f3", "e5", "g4", "Qh4")
	v := a.get(t, "engine_view").(map[string]any)
	if v["line"] != "game over · 0–1" {
		t.Errorf("engine_view = %v", v)
	}
	if g := grid(a.play(t)); strings.Contains(g, "asks for a hint") || !strings.Contains(g, "game over · 0–1") {
		t.Errorf("the panel after the game:\n%s", g)
	}
	// Against the computer too.
	b := newTestApp(t)
	b.play(t, "r", "y")
	if v := b.get(t, "engine_view").(map[string]any); !strings.HasPrefix(v["line"].(string), "game over · ") {
		t.Errorf("engine_view = %v", v)
	}
}

func TestLevelsAreDescribedHonestly(t *testing.T) {
	a := newTestApp(t)
	a.play(t, "n")
	for i := 0; i < 6; i++ {
		level := a.num(t, "ng.level")
		want := strconv.Itoa(level) + " (up to depth " + strconv.Itoa(levelLimits[level].Depth) + ")"
		if got := a.get(t, "ng_view").(map[string]any)["level"]; got != want {
			t.Errorf("level %d: %v, want %q", level, got, want)
		}
		a.play(t, "l")
	}
	if got := a.get(t, "ng_view").(map[string]any)["level"]; got != "3 (up to depth 3)" {
		t.Errorf("after a full turn: %v", got)
	}
}

// ---- PGN import ----

func TestPGNImportOddities(t *testing.T) {
	for _, c := range []struct{ name, text, want string }{
		{"e.p. suffix", "1. e4 a6 2. e5 d5 3. exd6 e.p. Qxd6 4. Nf3 *", "e4 a6 e5 d5 exd6 Qxd6 Nf3"},
		{"e.p. standalone and glued", "1. e4 a6 2. e5 d5 3. exd6e.p. Qxd6 *", "e4 a6 e5 d5 exd6 Qxd6"},
		{"standalone annotation marks", "1. e4 ! e5 ?! 2. Nf3 !! Nc6 ?? 3. Bb5 !? *", "e4 e5 Nf3 Nc6 Bb5"},
		{"a ; comment with ) in a variation", "1. e4 e5 2. Nf3 (2. f4 ; not (this) one)\nexf4) 2... Nc6 *", "e4 e5 Nf3 Nc6"},
		{"a ; comment in a variation with a {", "1. e4 e5 (1... c5 ; { never closed\n2. Nf3) 2. Nf3 *", "e4 e5 Nf3"},
	} {
		g, err := ParsePGN(c.text)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if got := strings.Join(g.SAN, " "); got != c.want {
			t.Errorf("%s: moves %q, want %q", c.name, got, c.want)
		}
	}
}

// ---- unbounded work ----

func TestCraftedPositionsDoNotHangTheLoop(t *testing.T) {
	const fen = "3k4/8/1n1n1N1N/nNnNnNnN/NnNnNnNn/8/8/3K4 w - - 0 1"
	p := mustFEN(t, fen)
	run := func(name string, f func()) {
		done := make(chan struct{})
		go func() { f(); close(done) }()
		select {
		case <-done:
		case <-time.After(20 * time.Second):
			t.Fatalf("%s is still running after 20 s", name)
		}
	}
	run("the evaluation bar", func() { _ = EvalWhite(&p) })
	for level := 1; level <= 2; level++ {
		level := level
		run("level "+strconv.Itoa(level), func() {
			res := Search(p, []uint64{p.Key()}, levelLimits[level], nil)
			if !res.Found || !p.IsLegal(res.Move) {
				t.Errorf("level %d found %+v", level, res)
			}
		})
	}
	// Black to move as well.
	q := mustFEN(t, strings.Replace(fen, " w ", " b ", 1))
	run("the evaluation bar (black)", func() { _ = EvalWhite(&q) })
	run("level 1 (black)", func() {
		if res := Search(q, []uint64{q.Key()}, levelLimits[1], nil); !res.Found || !q.IsLegal(res.Move) {
			t.Errorf("found %+v", res)
		}
	})
	// And in the app: loading such a position and writing the store is quick.
	run("the app", func() {
		a := newTestApp(t, twoPlayers, func(o *options) { o.fen = fen })
		a.moves(t, "Nd5")
	})
}

// Normal positions are not touched by the budgets: they search to their depth.
func TestNodeBudgetsLeaveNormalPositionsAlone(t *testing.T) {
	p := StartPosition()
	for level := 1; level <= 4; level++ {
		res := Search(p, []uint64{p.Key()}, levelLimits[level], nil)
		if res.Depth != levelLimits[level].Depth {
			t.Errorf("level %d reached depth %d, want %d", level, res.Depth, levelLimits[level].Depth)
		}
	}
}

// ---- mocks ----

func TestMockNotesInTheReadmeHold(t *testing.T) {
	mock := readMock(t)
	// Toggles are one-way in a mock: they write the new value only.
	for action, path := range map[string]string{"toggle_mouse": "mouse", "toggle_letters": "letters", "flip": "flip"} {
		if v, ok := writes(mock, action, path); !ok || v == nil {
			t.Errorf("%s does not write %s", action, path)
		}
	}
	// The move recorded from the sample state includes the computer's reply.
	if v, ok := writes(mock, "cmd_submit", "san"); !ok || len(v.([]any)) != 2 {
		t.Errorf("cmd_submit san = %v", v)
	}
	text := strings.Join(strings.Fields(readme(t)), " ")
	for _, want := range []string{"toggles are one-way", "the promotion mocks start from a position with a pawn about to promote"} {
		if !strings.Contains(text, want) {
			t.Errorf("README lacks %q", want)
		}
	}
	if strings.Contains(strings.ToLower(text), "the brief") || strings.Contains(strings.ToLower(text), "coordinator") {
		t.Error("the README mentions its own briefing")
	}
}

func TestSourcesDoNotMentionTheirBriefing(t *testing.T) {
	files, _ := filepath.Glob("*")
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") && f != "fixes_test.go" {
			continue
		}
		if f == "fixes_test.go" {
			continue
		}
		raw, err := os.ReadFile(f)
		if err != nil || strings.Contains(f, ".") && strings.HasSuffix(f, ".json") {
			continue
		}
		low := strings.ToLower(string(raw))
		if strings.Contains(low, "the brief") || strings.Contains(low, "coordinator") {
			t.Errorf("%s mentions its briefing", f)
		}
	}
}
