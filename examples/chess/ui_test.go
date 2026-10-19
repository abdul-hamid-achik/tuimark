package main

import (
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// A keymap context and the (key, action) pairs it must have. The document's
// keymap is parsed and compared with these, so a row added or removed
// without its test fails, and every pair is played.
type keyCase struct{ key, action string }

var keymapCases = map[string][]keyCase{
	"#board": {
		{"?", "help_open"}, {"q", "quit"}, {"ctrl+c", "quit"},
		{"up", "up"}, {"k", "up"}, {"down", "down"}, {"j", "down"},
		{"left", "left"}, {"h", "left"}, {"right", "right"}, {"l", "right"},
		{"home", "first"}, {"end", "last"}, {"enter", "select"}, {"space", "select"},
		{":", "focus"}, {"/", "focus"},
		{"u", "undo"}, {"U", "redo"}, {"n", "new_open"}, {"f", "flip"}, {"H", "hint"},
		{"r", "resign_open"}, {"d", "claim"}, {"v", "review"}, {"p", "pgn"},
		{"m", "toggle_mouse"}, {"L", "toggle_letters"},
	},
	"#cmd":     {{"esc", "cmd_cancel"}},
	"#help":    {{"?", "help_close"}, {"q", "help_close"}, {"enter", "help_close"}},
	"#newgame": {{"m", "ng_mode"}, {"s", "ng_side"}, {"l", "ng_level"}, {"c", "ng_clock"}},
	"#resign":  {{"y", "resign_yes"}, {"n", "resign_no"}},
	"#promo":   {{"q", "promo_q"}, {"r", "promo_r"}, {"b", "promo_b"}, {"n", "promo_n"}},
	"#pgn":     {{"q", "pgn_close"}, {"p", "pgn_close"}, {"enter", "pgn_close"}},
}

// escapeCases are the actions esc fires, by dialog.
var escapeCases = map[string]string{
	"?": "help_close", "n": "ng_cancel", "r": "resign_no", "p": "pgn_close",
}

var bindRe = regexp.MustCompile(`<bind keys="([^"]*)" action="([^"]*)"`)
var keymapRe = regexp.MustCompile(`(?s)<keymap when="(#[a-z]+)">(.*?)</keymap>`)

func TestKeymapMatchesTheTable(t *testing.T) {
	raw, err := os.ReadFile("app.tui")
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]map[string]string{}
	for _, m := range keymapRe.FindAllStringSubmatch(string(raw), -1) {
		ctx := m[1]
		found[ctx] = map[string]string{}
		for _, b := range bindRe.FindAllStringSubmatch(m[2], -1) {
			for _, k := range strings.Split(b[1], ",") {
				// keys="," cannot be written; the list separator is the comma.
				found[ctx][k] = b[2]
			}
		}
	}
	for ctx, cases := range keymapCases {
		want := map[string]string{}
		for _, c := range cases {
			want[c.key] = c.action
		}
		got := found[ctx]
		for k, a := range want {
			if got[k] != a {
				t.Errorf("%s: key %q is bound to %q in app.tui, the table wants %q", ctx, k, got[k], a)
			}
		}
		for k, a := range got {
			if want[k] != a {
				t.Errorf("%s: app.tui binds %q to %q, which the table does not cover", ctx, k, a)
			}
		}
	}
	for ctx := range found {
		if _, ok := keymapCases[ctx]; !ok {
			t.Errorf("app.tui has a keymap for %s that the table does not cover", ctx)
		}
	}
}

// opens puts the app in the context.
func (a *testApp) opens(t *testing.T, ctx string) {
	t.Helper()
	switch ctx {
	case "#board":
	case "#cmd":
		a.play(t, ":")
	case "#help":
		a.play(t, "?")
	case "#newgame":
		a.play(t, "n")
	case "#resign":
		a.play(t, "r")
	case "#pgn":
		a.play(t, "p")
	case "#promo":
		a.moves(t, "a8")
	}
}

func TestEveryKeyFiresItsAction(t *testing.T) {
	for ctx, cases := range keymapCases {
		for _, c := range cases {
			a := newTestApp(t, twoPlayers, func(o *options) {
				if ctx == "#promo" {
					o.fen = "4k3/P7/8/8/8/8/8/4K3 w - - 0 1"
				}
			})
			a.opens(t, ctx)
			res := a.play(t, c.key)
			switch c.action {
			case "focus": // a built-in: the move box takes the focus
				if focused(res) != "cmd" || len(actions(res)) != 0 {
					t.Errorf("%s %q: focus %q, actions %v", ctx, c.key, focused(res), actions(res))
				}
			case "quit":
				if !res.Quit {
					t.Errorf("%s %q: did not quit (%v)", ctx, c.key, actions(res))
				}
			case "promo_q", "promo_r", "promo_b", "promo_n":
				if got := actions(res); len(got) != 1 || got[0] != c.action {
					t.Errorf("%s %q: actions %v, want %s", ctx, c.key, got, c.action)
				}
			default:
				got := actions(res)
				if len(got) == 0 || got[0] != c.action {
					t.Errorf("%s %q: actions %v, want %s first", ctx, c.key, got, c.action)
				}
			}
		}
	}
}

func TestEscapeClosesEveryDialog(t *testing.T) {
	for key, action := range escapeCases {
		a := newTestApp(t, twoPlayers)
		a.moves(t, "e4")
		a.play(t, key)
		res := a.play(t, "esc")
		if got := actions(res); len(got) != 1 || got[0] != action {
			t.Errorf("%s then esc: %v, want %s", key, got, action)
		}
		if focused(res) != "board" {
			t.Errorf("%s then esc: focus %q", key, focused(res))
		}
	}
	a := newTestApp(t, twoPlayers, func(o *options) { o.fen = "4k3/P7/8/8/8/8/8/4K3 w - - 0 1" })
	a.moves(t, "a8")
	if got := actions(a.play(t, "esc")); len(got) != 1 || got[0] != "promo_cancel" {
		t.Errorf("promotion then esc: %v", got)
	}
}

// The board's keys do nothing behind a dialog: the arrows do not move the
// cursor, u does not undo.
func TestBoardKeysAreDeadBehindADialog(t *testing.T) {
	a := newTestApp(t, twoPlayers)
	a.moves(t, "e4")
	cur := a.num(t, "cur")
	for _, ctx := range []string{"#help", "#resign", "#pgn"} {
		a.opens(t, ctx)
		res := a.play(t, "up", "u", "f", "k", "j", "x")
		for _, act := range actions(res) {
			if act != "up" && act != "down" && act != "left" && act != "right" {
				t.Errorf("%s: %q fired behind the dialog", ctx, act)
			}
		}
		if len(a.g.SAN) != 1 || a.flag(t, "flip") || a.num(t, "cur") != cur {
			t.Errorf("%s: the board reacted behind the dialog", ctx)
		}
		a.play(t, "esc")
	}
}

func TestKeymapOrderKeepsHelpAndQuitFirst(t *testing.T) {
	raw, err := os.ReadFile("app.tui")
	if err != nil {
		t.Fatal(err)
	}
	binds := bindRe.FindAllStringSubmatch(string(raw), -1)
	if len(binds) < 2 || binds[0][1] != "?" || binds[1][1] != "q" {
		t.Errorf("the first two keymap rows are %v, want ? then q", binds[:2])
	}
}

// The hints drop from the end, so help and quit survive the narrow bars.
func TestHintsKeepHelpAndQuit(t *testing.T) {
	a := newTestApp(t)
	for _, cols := range []int{40, 50, 60, 80, 100, 120} {
		d, err := a.ui.Dump(cols, 24)
		if err != nil {
			t.Fatal(err)
		}
		g := d.Grid[len(d.Grid)-1]
		for _, want := range []string{"? help", "q quit"} {
			if !strings.Contains(g, want) {
				t.Errorf("%d columns: the hint bar has no %q: %q", cols, want, g)
			}
		}
	}
}

// ---- dialogs ----

type dialog struct {
	name    string
	setup   func(t *testing.T) *testApp
	buttons []string // texts that must be visible
}

func promoApp(t *testing.T) *testApp {
	a := newTestApp(t, twoPlayers, func(o *options) { o.fen = "4k3/P7/8/8/8/8/8/4K3 w - - 0 1" })
	a.moves(t, "a8")
	return a
}

func dialogs() []dialog {
	open := func(keys ...string) func(t *testing.T) *testApp {
		return func(t *testing.T) *testApp {
			a := newTestApp(t)
			a.moves(t, "e4")
			a.play(t, keys...)
			return a
		}
	}
	return []dialog{
		{"help", open("?"), []string{"close"}},
		{"new game", open("n"), []string{"start", "cancel"}},
		{"new game 2p", open("n", "m"), []string{"start", "cancel"}},
		{"new game cvc", open("n", "m", "m"), []string{"start", "cancel"}},
		{"resign", open("r"), []string{"resign", "keep playing"}},
		{"promotion", promoApp, []string{"queen", "rook", "bishop", "knight"}},
		{"pgn", open("p"), []string{"close"}},
	}
}

var sizes = [][2]int{{40, 14}, {50, 16}, {60, 20}, {80, 24}, {100, 30}, {120, 30}}

func TestDialogsAreCleanAtEverySizeWithTheirButtonsVisible(t *testing.T) {
	for _, d := range dialogs() {
		for _, theme := range []string{"dark", "light"} {
			a := d.setup(t)
			if err := a.ui.Set("@theme", theme); err != nil {
				t.Fatal(err)
			}
			for _, sz := range sizes {
				dump, err := a.ui.Dump(sz[0], sz[1])
				if err != nil {
					t.Fatal(err)
				}
				if !dump.OK || len(dump.Errors) != 0 {
					t.Errorf("%s %s %dx%d: %v", d.name, theme, sz[0], sz[1], dump.Errors)
				}
				if dump.Wide {
					t.Errorf("%s %s %dx%d: the dump is wide", d.name, theme, sz[0], sz[1])
				}
				g := strings.Join(dump.Grid, "\n")
				for _, b := range d.buttons {
					if !strings.Contains(g, b) {
						t.Errorf("%s %s %dx%d: no %q:\n%s", d.name, theme, sz[0], sz[1], b, g)
					}
				}
			}
		}
	}
}

// Really open: the dialog's own flag is set at every size (a dialog that is
// not open at a size would pass the test above by being absent).
func TestDialogsReallyOpen(t *testing.T) {
	flags := map[string]string{"help": "help_open", "new game": "new_open", "new game 2p": "new_open", "new game cvc": "new_open", "resign": "resign_open", "promotion": "promo_open", "pgn": "pgn_open"}
	for _, d := range dialogs() {
		a := d.setup(t)
		if !a.flag(t, flags[d.name]) {
			t.Errorf("%s: %s is not set", d.name, flags[d.name])
		}
		for _, sz := range sizes {
			res := a.playAt(t, sz[0], sz[1])
			if f := focused(res); f == "board" || f == "cmd" || f == "" {
				t.Errorf("%s %dx%d: focus %q is not in the dialog", d.name, sz[0], sz[1], f)
			}
		}
	}
}

// Every dialog closes by its buttons and keys at the smallest sizes too.
func TestDialogsStayClosableAtSmallSizes(t *testing.T) {
	for _, sz := range [][2]int{{40, 12}, {40, 13}, {40, 14}, {50, 13}, {35, 12}} {
		for _, d := range dialogs() {
			a := d.setup(t)
			res := a.playAt(t, sz[0], sz[1], "esc")
			if len(res.Dump.Errors) != 0 {
				t.Errorf("%s %dx%d: %v", d.name, sz[0], sz[1], res.Dump.Errors)
			}
			if focused(res) != "board" && d.name != "promotion" {
				t.Errorf("%s %dx%d: esc left the focus on %q", d.name, sz[0], sz[1], focused(res))
			}
			if a.modalOpen() {
				t.Errorf("%s %dx%d: still open after esc", d.name, sz[0], sz[1])
			}
		}
	}
}

func TestDialogButtonsClickToo(t *testing.T) {
	click := func(a *testApp, id string, cols, rows int) []string {
		d, err := a.ui.Dump(cols, rows)
		if err != nil {
			t.Fatal(err)
		}
		for _, n := range d.Nodes {
			if n.ID == id {
				res := a.playAt(t, cols, rows, "click:"+strconv.Itoa(n.X+n.W/2)+","+strconv.Itoa(n.Y))
				return actions(res)
			}
		}
		t.Fatalf("no node %s", id)
		return nil
	}
	for _, c := range []struct {
		open, id, action string
	}{
		{"?", "help-ok", "help_close"}, {"n", "ng-start", "ng_start"}, {"n", "ng-cancel", "ng_cancel"},
		{"n", "ng-mode", "ng_mode"}, {"n", "ng-side", "ng_side"}, {"n", "ng-level", "ng_level"}, {"n", "ng-clock", "ng_clock"},
		{"r", "resign-yes", "resign_yes"}, {"r", "resign-no", "resign_no"}, {"p", "pgn-ok", "pgn_close"},
	} {
		a := newTestApp(t)
		a.moves(t, "e4")
		a.play(t, c.open)
		if got := click(a, c.id, 100, 30); len(got) != 1 || got[0] != c.action {
			t.Errorf("click on %s: %v, want %s", c.id, got, c.action)
		}
	}
	for id, action := range map[string]string{"promo-q": "promo_q", "promo-r": "promo_r", "promo-b": "promo_b", "promo-n": "promo_n"} {
		a := promoApp(t)
		if got := click(a, id, 100, 30); len(got) != 1 || got[0] != action {
			t.Errorf("click on %s: %v, want %s", id, got, action)
		}
	}
}

// ---- states x sizes x themes ----

type scene struct {
	name  string
	build func(t *testing.T) *testApp
}

func scenes() []scene {
	typed := func(opt func(*options), sans ...string) func(t *testing.T) *testApp {
		return func(t *testing.T) *testApp {
			a := newTestApp(t, twoPlayers, opt)
			a.moves(t, sans...)
			return a
		}
	}
	none := func(*options) {}
	long := strings.Fields("e4 e5 Nf3 Nc6 Bb5 a6 Ba4 Nf6 O-O Be7 Re1 b5 Bb3 d6 c3 O-O h3 Nb8 d4 Nbd7 c4 c6 cxb5 axb5 Nc3 Bb7 Bg5 b4 Nb1 h6 Bh4 c5 dxe5 Nxe4 Bxe7 Qxe7 exd6 Qf6 Nbd2 Nxd6 Nc4 Nxc4 Bxc4 Nb6 Ne5 Rae8 Bxf7+ Rxf7 Nxf7 Rxe1+ Qxe1 Kxf7 Qe3 Qg5 Qxg5 hxg5 b3 Ke6 a3 Kd6 axb4 cxb4 Ra5 Nd5 f3 Bc8 Kf2 Bf5 Ra7 g6 Ra6+ Kc5 Ke1 Nf4 g3 Nxh3 Kd2 Kb5 Rd6 Kc5 Ra6 Nf2 g4 Bd3 Re6")
	return []scene{
		{"start", func(t *testing.T) *testApp { return newTestApp(t) }},
		{"start as black", func(t *testing.T) *testApp {
			return newTestApp(t, func(o *options) { o.side = "black" })
		}},
		{"opening", typed(none, "e4", "e5", "Nf3", "Nc6", "Bb5")},
		{"a long game", typed(none, long...)},
		{"selected piece", func(t *testing.T) *testApp {
			a := newTestApp(t, twoPlayers)
			a.moves(t, "e4", "d5")
			a.click(t, "e4")
			return a
		}},
		{"check", typed(none, "e4", "f6", "Qh5")},
		{"checkmate", typed(none, "f3", "e5", "g4", "Qh4")},
		{"stalemate", typed(func(o *options) { o.fen = "7k/8/6K1/8/8/8/8/5Q2 w - - 0 1" }, "Qf7")},
		{"time forfeit", func(t *testing.T) *testApp {
			a := newTestApp(t, twoPlayers, func(o *options) { o.clock = "0.1+0" })
			for i := 0; i < 7; i++ {
				a.tickAfter(t, time.Second)
			}
			return a
		}},
		{"review", func(t *testing.T) *testApp {
			a := newTestApp(t, twoPlayers)
			a.moves(t, "e4", "e5", "Nf3", "Nc6")
			a.play(t, "v", "left", "left")
			return a
		}},
		{"flipped", func(t *testing.T) *testApp {
			a := newTestApp(t, twoPlayers)
			a.moves(t, "e4", "e5")
			a.play(t, "f")
			a.click(t, "g8")
			return a
		}},
		{"letters", func(t *testing.T) *testApp {
			return newTestApp(t, twoPlayers, func(o *options) { o.letters = true })
		}},
		{"thinking", func(t *testing.T) *testApp {
			a := newTestApp(t)
			a.deferJobs()
			a.moves(t, "e4")
			return a
		}},
		{"a hint", func(t *testing.T) *testApp {
			a := newTestApp(t)
			a.play(t, "H")
			return a
		}},
		{"clocks", func(t *testing.T) *testApp {
			a := newTestApp(t, twoPlayers, func(o *options) { o.clock = "3+2" })
			a.moves(t, "e4", "e5")
			a.tickFor(t, 3*time.Second)
			return a
		}},
		{"a long notice", func(t *testing.T) *testApp {
			a := newTestApp(t, twoPlayers)
			a.play(t, ":", "text:Qxz9", "enter")
			return a
		}},
		{"a long move box", func(t *testing.T) *testApp {
			a := newTestApp(t, twoPlayers)
			a.play(t, ":", "text:this is a very long text that does not fit in the move box at all, really")
			return a
		}},
		{"a claimable draw", func(t *testing.T) *testApp {
			return typed(none, "Nf3", "Nf6", "Ng1", "Ng8", "Nf3", "Nf6", "Ng1", "Ng8")(t)
		}},
		{"cvc", func(t *testing.T) *testApp {
			a := newTestApp(t, func(o *options) { o.mode = modeCvC; o.level = 1 })
			a.tickAfter(t, tickStep)
			return a
		}},
		{"engine info", func(t *testing.T) *testApp {
			a := newTestApp(t, func(o *options) { o.level = 4 })
			a.moves(t, "e4")
			return a
		}},
		{"captured pieces", typed(none, "e4", "d5", "exd5", "Qxd5", "Nc3", "Qxg2", "Bxg2", "Nc6", "Bxc6+", "bxc6")},
	}
}

func TestNoDiagnosticsOverStatesSizesAndThemes(t *testing.T) {
	all := append([][2]int(nil), sizes...)
	all = append(all, [2]int{70, 22}, [2]int{100, 14}, [2]int{120, 40}, [2]int{80, 19}, [2]int{79, 20}, [2]int{59, 20}, [2]int{35, 12})
	for _, s := range scenes() {
		a := s.build(t)
		for _, theme := range []string{"dark", "light"} {
			if err := a.ui.Set("@theme", theme); err != nil {
				t.Fatal(err)
			}
			for _, sz := range all {
				d, err := a.ui.Dump(sz[0], sz[1])
				if err != nil {
					t.Fatal(err)
				}
				if !d.OK || len(d.Errors) != 0 {
					t.Errorf("%s %s %dx%d: %v", s.name, theme, sz[0], sz[1], d.Errors)
				}
				if d.Wide {
					t.Errorf("%s %s %dx%d: the dump is wide", s.name, theme, sz[0], sz[1])
				}
			}
			if diags := a.ui.Validate(); len(diags) != 0 {
				t.Errorf("%s %s: Validate gave %v", s.name, theme, diags)
			}
		}
	}
}

// The board is always whole: eight ranks and eight files of figurines fit at
// every size from 40x14 up, and the status is somewhere on screen.
func TestBoardFitsAtEverySize(t *testing.T) {
	a := newTestApp(t, twoPlayers)
	a.moves(t, "e4", "e5")
	for _, sz := range append([][2]int{{80, 20}, {79, 24}, {41, 15}}, sizes...) {
		res := a.playAt(t, sz[0], sz[1])
		g := grid(res)
		for _, want := range []string{"a  ", "h", "White to move"} {
			if !strings.Contains(g, want) {
				t.Errorf("%dx%d: no %q:\n%s", sz[0], sz[1], want, g)
			}
		}
		count := 0
		for _, r := range g {
			if strings.ContainsRune("♜♞♝♛♚♟", r) {
				count++
			}
		}
		if count < 32 {
			t.Errorf("%dx%d: %d figurines on screen, want at least 32 (the side panel adds more):\n%s", sz[0], sz[1], count, g)
		}
	}
}

// ---- flags ----

func TestFlagsAreValidated(t *testing.T) {
	bad := map[string]func(o *options){
		"-theme wants":  func(o *options) { o.theme = "blue" },
		"-mode wants":   func(o *options) { o.mode = "solo" },
		"-level wants":  func(o *options) { o.level = 0 },
		"-level wants7": func(o *options) { o.level = 7 },
		"-level wants-": func(o *options) { o.level = -3 },
		"-side wants":   func(o *options) { o.side = "green" },
		"-clock":        func(o *options) { o.clock = "NaN+0" },
		"-clock Inf":    func(o *options) { o.clock = "5+Inf" },
		"-clock range":  func(o *options) { o.clock = "0+0" },
		"-clock inc":    func(o *options) { o.clock = "5+120" },
		"together":      func(o *options) { o.fen = "4k3/8/8/8/8/8/8/4K3 w - - 0 1"; o.pgn = "x.pgn" },
	}
	for name, f := range bad {
		o := defaults()
		f(&o)
		if _, err := o.validate(); err == nil {
			t.Errorf("%s: no error", name)
		} else if want := strings.Fields(name)[0]; strings.HasPrefix(want, "-") && !strings.Contains(err.Error(), want) {
			t.Errorf("%s: error %q does not name the flag", name, err)
		}
	}
	for _, level := range []int{1, 2, 3, 4, 5, 6} {
		o := defaults()
		o.level = level
		if _, err := o.validate(); err != nil {
			t.Errorf("level %d: %v", level, err)
		}
	}
	if _, err := defaults().validate(); err != nil {
		t.Errorf("the defaults are invalid: %v", err)
	}
}

func TestSizeFlagsAreValidated(t *testing.T) {
	for in, want := range map[string][2]int{"100x24": {100, 24}, "1x1": {1, 1}, "1000x1000": {1000, 1000}} {
		c, r, err := parseSize("-dump", in)
		if err != nil || c != want[0] || r != want[1] {
			t.Errorf("%q = %d,%d,%v", in, c, r, err)
		}
	}
	for _, in := range []string{"", "100", "100x", "x24", "0x24", "100x0", "-5x24", "100x-1", "1001x24", "NaNxNaN", "InfxInf", "100x24x5", "100x24 ", "1e2x24", "abc", fmtFloat(math.NaN()), "99999999999999999999x1"} {
		if c, r, err := parseSize("-dump", in); err == nil {
			t.Errorf("%q = %d,%d, want an error", in, c, r)
		} else if !strings.Contains(err.Error(), "-dump") {
			t.Errorf("%q: error %q does not name the flag", in, err)
		}
	}
	// run reports a bad size before it starts anything.
	for _, o := range []options{{dump: "nope"}, {demo: "0x0"}} {
		base := defaults()
		base.dump, base.demo = o.dump, o.demo
		if err := run(base); err == nil {
			t.Errorf("run(%+v) accepted a bad size", o)
		}
	}
	bad := defaults()
	bad.level = 9
	bad.demo = "100x24"
	if err := run(bad); err == nil || !strings.Contains(err.Error(), "-level") {
		t.Errorf("run with a bad level: %v", err)
	}
}

func fmtFloat(f float64) string { return strconv.FormatFloat(f, 'g', -1, 64) }

// ---- concurrency ----

// With real goroutines the search and the loop share the app: this is what
// the race detector watches.
func TestSearchOnItsOwnGoroutine(t *testing.T) {
	a := newTestApp(t, func(o *options) { o.level = 2 })
	a.dispatch = func(f func()) { go f() }
	a.play(t, ":", "text:e4", "enter")
	deadline := time.Now().Add(10 * time.Second)
	for {
		a.mu.Lock()
		n := len(a.g.SAN)
		a.mu.Unlock()
		if n == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the computer never answered")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if a.flag(t, "thinking") {
		t.Error("still thinking after the answer")
	}
	// Undo while it thinks, many times over: nothing races and no stale
	// answer sneaks in.
	for i := 0; i < 20; i++ {
		b := newTestApp(t, func(o *options) { o.level = 3 })
		b.dispatch = func(f func()) { go f() }
		b.play(t, ":", "text:d4", "enter")
		b.play(t, "u")
		b.play(t, ":", "text:c4", "enter")
		b.play(t, "n", "enter")
		time.Sleep(time.Duration(i%5) * time.Millisecond)
		b.mu.Lock()
		n := len(b.g.SAN)
		b.mu.Unlock()
		if n != 0 && n != 2 {
			t.Fatalf("round %d: %d plies, a new game starts at 0", i, n)
		}
		b.shutdown()
	}
}
