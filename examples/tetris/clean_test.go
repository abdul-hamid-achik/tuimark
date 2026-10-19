package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

// bigNumbers puts a very long game's numbers in the store: eight-digit
// score and best, five-digit lines and pieces, a three-digit level.
func (r *rig) bigNumbers() {
	r.t.Helper()
	g := r.a.g
	g.score, g.best = 99999999, 99999999
	g.lines, g.pieces, g.startLevel = 99999, 99999, 900
	g.last = "B2B T-SPIN TRIPLE"
	g.combo = 99
	r.sync()
}

type look struct {
	name  string
	setup func(r *rig)
}

// looks are real states of the game, each reached through the game's own
// calls, not by writing the store.
func looks() []look {
	long := "tick: store write failed: a long message that cannot fit in the top line of a small terminal"
	return []look{
		{"fresh", func(r *rig) {}},
		{"auto", func(r *rig) { r.play("a"); r.step(80) }},
		{"mid-game", func(r *rig) { r.play("a"); r.step(300); r.play("a") }},
		{"big numbers", func(r *rig) { r.bigNumbers() }},
		{"big numbers auto", func(r *rig) { r.play("a"); r.bigNumbers() }},
		{"hold used", func(r *rig) { r.play("c") }},
		{"flash", func(r *rig) {
			g := r.a.g
			fillRows(g, 4, 9)
			dropI(g, 9)
			r.sync()
			call(r.t, r.a, "hard_drop")
			if !g.clearing {
				r.t.Fatal("no flash")
			}
		}},
		{"paused", func(r *rig) { r.play("p") }},
		{"notice", func(r *rig) { _ = r.a.onError(nil, errors.New(long)) }},
		{"notice big", func(r *rig) { r.bigNumbers(); _ = r.a.onError(nil, errors.New(long)) }},
		{"help", func(r *rig) { r.play("?") }},
		{"help big", func(r *rig) { r.bigNumbers(); r.play("?") }},
		{"help over game over", func(r *rig) { r.gameOver(); r.play("?") }},
		{"over", func(r *rig) { r.gameOver() }},
		{"over big", func(r *rig) { r.bigNumbers(); r.gameOver() }},
	}
}

// No state of the game has a diagnostic at any of the sizes that matter, in
// either theme, warnings included: every modal open in turn, the board
// hidden, the rows flashing, and numbers as long as they get.
func TestNoDiagnosticsInEveryStateAtEverySize(t *testing.T) {
	sizes := [][2]int{{40, 14}, {60, 20}, {80, 24}, {100, 30}, {120, 30}, {100, 24}, {74, 24}, {73, 24}, {48, 24}, {47, 24}, {24, 24}, {23, 24}, {200, 50}}
	for _, lk := range looks() {
		for _, theme := range []string{"dark", "light"} {
			r := newRig(t)
			if err := r.a.ui.Set("@theme", theme); err != nil {
				t.Fatal(err)
			}
			r.step(1)
			lk.setup(r)
			for _, sz := range sizes {
				d, err := r.a.ui.Dump(sz[0], sz[1])
				if err != nil {
					t.Fatal(err)
				}
				if !d.OK || len(d.Errors) != 0 {
					t.Errorf("%s %s %dx%d: %v\n%s", lk.name, theme, sz[0], sz[1], d.Errors, strings.Join(d.Grid, "\n"))
				}
			}
		}
	}
}

// The same, for every width from 24 to 90 columns and a sample beyond, at
// a height where the board shows and one where it gives way to a notice.
func TestNoDiagnosticsAtEveryWidth(t *testing.T) {
	// The looks that only repeat another with different numbers or in a
	// later position are covered at the sizes above.
	repeats := map[string]bool{"mid-game": true, "big numbers": true, "notice big": true}
	for _, lk := range looks() {
		if repeats[lk.name] {
			continue
		}
		r := newRig(t)
		r.step(1)
		lk.setup(r)
		for _, theme := range []string{"dark", "light"} {
			if err := r.a.ui.Set("@theme", theme); err != nil {
				t.Fatal(err)
			}
			for c := 24; c <= 140; c++ {
				if c > 90 && c%10 != 0 {
					continue // every width up to 90, where the layouts change; then a sample
				}
				for _, rows := range []int{14, 24} {
					d, err := r.a.ui.Dump(c, rows)
					if err != nil {
						t.Fatal(err)
					}
					if !d.OK || len(d.Errors) != 0 {
						t.Errorf("%s %s %dx%d: %v\n%s", lk.name, theme, c, rows, d.Errors, strings.Join(d.Grid, "\n"))
					}
				}
			}
		}
	}
}

// Every modal has its buttons on screen and inside the modal from 40x14 up:
// the help's close, and the game over modal's restart and quit.
func TestModalButtonsFit(t *testing.T) {
	sizes := [][2]int{{40, 14}, {44, 14}, {60, 14}, {120, 14}, {40, 15}, {40, 24}, {60, 20}, {80, 24}, {100, 30}, {120, 30}}
	modals := []struct {
		modal   string
		buttons []string
		open    func(r *rig)
		action  map[string]string // button -> the action it fires
	}{
		{"help", []string{"help-ok"}, func(r *rig) { r.play("?") }, map[string]string{"help-ok": "help_close"}},
		{"over", []string{"over-board", "over-restart", "over-quit"}, func(r *rig) { r.gameOver() }, map[string]string{"over-board": "over_hide", "over-restart": "restart", "over-quit": "quit"}},
	}
	for _, m := range modals {
		for _, theme := range []string{"dark", "light"} {
			for _, sz := range sizes {
				r := newRig(t)
				if err := r.a.ui.Set("@theme", theme); err != nil {
					t.Fatal(err)
				}
				m.open(r)
				res, err := r.a.ui.Play(tuimark.PlayOptions{Cols: sz[0], Rows: sz[1]})
				if err != nil {
					t.Fatal(err)
				}
				byID := map[string]tuimark.DumpNode{}
				for _, n := range res.Dump.Nodes {
					byID[n.ID] = n
				}
				box, ok := byID[m.modal]
				if !ok {
					t.Fatalf("%s %s %dx%d: no %s modal in the dump", m.modal, theme, sz[0], sz[1], m.modal)
				}
				if len(res.Dump.Errors) > 0 {
					t.Errorf("%s %s %dx%d: %v", m.modal, theme, sz[0], sz[1], res.Dump.Errors)
				}
				for _, id := range m.buttons {
					b, ok := byID[id]
					if !ok {
						t.Errorf("%s %s %dx%d: no #%s", m.modal, theme, sz[0], sz[1], id)
						continue
					}
					if b.H < 1 || b.W < 3 || b.Y < box.Y || b.Y+b.H > box.Y+box.H || b.X < box.X || b.X+b.W > box.X+box.W || b.Y+b.H > sz[1] || b.X+b.W > sz[0] {
						t.Errorf("%s %s %dx%d: #%s at x=%d y=%d %dx%d is outside the modal (x=%d y=%d %dx%d) or the screen", m.modal, theme, sz[0], sz[1], id, b.X, b.Y, b.W, b.H, box.X, box.Y, box.W, box.H)
					}
					// The label is really on screen.
					row := res.Dump.Grid[b.Y]
					if !strings.Contains(row, strings.TrimSpace(b.Text)) {
						t.Errorf("%s %s %dx%d: #%s label %q is not on its row %q", m.modal, theme, sz[0], sz[1], id, b.Text, row)
					}
				}
			}
		}
	}
	// Each button, activated, fires its action.
	for _, m := range modals {
		for button, action := range m.action {
			r := newRig(t)
			m.open(r)
			var steps []string
			for i, id := range m.buttons {
				if id == button {
					for j := 0; j < i; j++ {
						steps = append(steps, "tab")
					}
				}
			}
			steps = append(steps, "enter")
			res := r.play(steps...)
			if action == "quit" {
				if !res.Quit {
					t.Errorf("#%s did not quit", button)
				}
				continue
			}
			if len(res.Events) == 0 || res.Events[len(res.Events)-1].Action != action {
				t.Errorf("#%s fired %+v, want %s", button, res.Events, action)
			}
		}
	}
}

// Both modals are clean, with their buttons on screen, at the heights where
// the help and the note change (14 to 20), 24 and 30, and every width from 24 to 60, over every look that can sit under
// them; the help also over the game over modal.
func TestModalsAcrossHeightsAndWidths(t *testing.T) {
	over := func(r *rig) { r.gameOver() }
	help := func(r *rig) { r.play("?") }
	both := func(r *rig) { r.gameOver(); r.play("?") }
	for name, open := range map[string]func(*rig){"over": over, "help": help, "help over over": both} {
		for _, theme := range []string{"dark", "light"} {
			r := newRig(t)
			if err := r.a.ui.Set("@theme", theme); err != nil {
				t.Fatal(err)
			}
			open(r)
			for _, rows := range []int{14, 15, 16, 17, 18, 19, 20, 24, 30} {
				for cols := 24; cols <= 60; cols++ {
					res, err := r.a.ui.Play(tuimark.PlayOptions{Cols: cols, Rows: rows})
					if err != nil {
						t.Fatal(err)
					}
					if !res.Dump.OK || len(res.Dump.Errors) != 0 {
						t.Errorf("%s %s %dx%d: %v", name, theme, cols, rows, res.Dump.Errors)
					}
					if cols < 40 {
						continue // the modals must fit from 40x14; below that they only have to be clean
					}
					byID := map[string]tuimark.DumpNode{}
					for _, n := range res.Dump.Nodes {
						byID[n.ID] = n
					}
					for _, id := range map[string][]string{"over": {"over-board", "over-restart", "over-quit"}, "help": {"help-ok"}, "help over over": {"help-ok"}}[name] {
						b, ok := byID[id]
						if !ok || b.H < 1 || b.Y+b.H > rows || b.X+b.W > cols {
							t.Errorf("%s %s %dx%d: #%s is not on screen: %+v", name, theme, cols, rows, id, b)
						}
					}
				}
			}
		}
	}
}

// The help lists every labelled key at the smallest size that shows it, so
// nothing a player needs is cut.
func TestHelpListsTheKeysAtTheSmallestSize(t *testing.T) {
	r := newRig(t)
	r.play("?")
	d, err := r.a.ui.Dump(40, 14)
	if err != nil {
		t.Fatal(err)
	}
	g := strings.Join(d.Grid, "\n")
	for _, want := range []string{"quit", "help", "move", "soft", "rotate", "back", "drop", "hold", "pause", "restart", "auto", "close"} {
		if !strings.Contains(g, want) {
			t.Errorf("the help at 40x14 lacks %q:\n%s", want, g)
		}
	}
}

// The status line keeps its state label whole beside a long score, from the
// widest to the narrowest width that shows it.
func TestStatusLineKeepsTheStateWithBigNumbers(t *testing.T) {
	for _, want := range []string{"PLAYING", "GAME OVER"} {
		r := newRig(t)
		r.bigNumbers()
		if want == "GAME OVER" {
			r.a.g.over = true
			r.sync()
		}
		for c := 26; c <= 140; c++ {
			d, err := r.a.ui.Dump(c, 24)
			if err != nil {
				t.Fatal(err)
			}
			if top := d.Grid[0]; !strings.Contains(top, want) {
				t.Errorf("%d cols: top line %q lacks %q", c, top, want)
			}
		}
	}
}

func TestNoticeNeverHidesTheStateOrOverflows(t *testing.T) {
	r := newRig(t)
	_ = r.a.onError(nil, fmt.Errorf("%s", strings.Repeat("x", 300)))
	for c := 26; c <= 140; c += 7 {
		d, err := r.a.ui.Dump(c, 24)
		if err != nil {
			t.Fatal(err)
		}
		if !d.OK || len(d.Errors) != 0 {
			t.Errorf("%d cols: %v", c, d.Errors)
		}
	}
}
