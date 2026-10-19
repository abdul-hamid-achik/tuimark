package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/abdul-hamid-achik/tuimark"
)

// after is what a handler asks the next store write to do besides writing
// the raw half: move the focus, or clear the move box.
type after struct {
	focus    string
	clearCmd bool
}

// wrap runs a handler under mu, then writes the store and starts any search
// it asked for, outside the lock. A user error (an illegal move) is a notice,
// not an error: only a programming error comes back from a handler.
func (a *app) wrap(f func(ev tuimark.Event) error) tuimark.Handler {
	return func(ev tuimark.Event) error {
		a.mu.Lock()
		a.notice, a.noticeTicks = "", 0
		a.after = after{}
		err := f(ev)
		if errors.Is(err, tuimark.ErrQuit) {
			a.invalidate()
			a.mu.Unlock()
			return err
		}
		var jobs []*job
		if err == nil {
			if j := a.takeJob(); j != nil {
				jobs = append(jobs, j)
			}
			if a.extra != nil {
				jobs = append(jobs, a.extra)
				a.extra = nil
			}
		}
		werr := a.writeLocked()
		a.mu.Unlock()
		for _, j := range jobs {
			j := j
			a.dispatch(func() { a.runJob(j) })
		}
		if err != nil {
			return err
		}
		return werr
	}
}

// register installs the handlers and the derived values.
func (a *app) register() error {
	for name, h := range a.handlers() {
		a.ui.On(name, a.wrap(h))
	}
	return a.derive()
}

// handlers are the named actions of app.tui.
func (a *app) handlers() map[string]func(tuimark.Event) error {
	hs := map[string]func(tuimark.Event) error{}
	on := func(name string, h func(tuimark.Event) error) { hs[name] = h }
	on("quit", func(tuimark.Event) error { return tuimark.ErrQuit })
	on("help_open", func(tuimark.Event) error { a.help = true; return nil })
	on("help_close", func(tuimark.Event) error { a.help = false; a.after.focus = "#board"; return nil })

	on("up", func(tuimark.Event) error { a.arrow(0, -1); return nil })
	on("down", func(tuimark.Event) error { a.arrow(0, 1); return nil })
	on("left", func(tuimark.Event) error { a.arrow(-1, 0); return nil })
	on("right", func(tuimark.Event) error { a.arrow(1, 0); return nil })
	on("first", func(tuimark.Event) error {
		a.curOn = true
		if a.review {
			a.setPly(0)
		} else {
			r, _ := a.screenPos(a.cur)
			a.cur = screenSq(r, 0, a.flip)
		}
		return nil
	})
	on("last", func(tuimark.Event) error {
		a.curOn = true
		if a.review {
			a.setPly(len(a.g.Moves))
		} else {
			r, _ := a.screenPos(a.cur)
			a.cur = screenSq(r, 7, a.flip)
		}
		return nil
	})
	on("select", func(tuimark.Event) error {
		if a.review {
			return a.reviewPlayOn()
		}
		if !a.curOn {
			a.curOn = true // the first press only shows where the cursor is
			return nil
		}
		return a.selectSquare(a.cur)
	})
	on("cell", func(ev tuimark.Event) error {
		sq, ok := a.squareOfCell(ev.Source)
		if !ok {
			return fmt.Errorf("click from %q, which is no board cell", ev.Source)
		}
		a.curOn = false          // a mouse user has no use for the cursor
		a.after.focus = "#board" // and a click leaves the move box
		return a.selectSquare(sq)
	})

	on("cmd_submit", func(ev tuimark.Event) error { return a.submitMove(asStr(ev.Value)) })
	on("cmd_cancel", func(tuimark.Event) error {
		a.after.focus, a.after.clearCmd = "#board", true
		return nil
	})

	on("undo", func(tuimark.Event) error { a.undo(); return nil })
	on("redo", func(tuimark.Event) error { a.redo(); return nil })
	on("flip", func(tuimark.Event) error { a.flip = !a.flip; return nil })
	on("hint", func(tuimark.Event) error {
		switch {
		case a.review:
			a.say("leave the review first: v")
		case a.g.Over():
			a.say("the game is over")
		case a.set.mode == modeCvC:
			a.say("the computers are playing: no hint needed")
		case !a.humanTurn():
			a.say("it is the computer's turn")
		default:
			a.extra = a.hintJob()
			a.say("looking for a hint…")
		}
		return nil
	})
	on("claim", func(tuimark.Event) error {
		if a.review {
			a.say("leave the review first: v")
			return nil
		}
		if !a.humanTurn() && !a.g.Over() {
			a.say("it is the computer's turn: claim a draw on your move")
			return nil
		}
		if err := a.g.ClaimDraw(); err != nil {
			a.say(err.Error())
			return nil
		}
		a.invalidate()
		a.sel, a.targets, a.caps = -1, nil, nil
		a.say("draw claimed: " + a.g.Reason)
		return nil
	})

	on("resign_open", func(tuimark.Event) error {
		switch {
		case a.review:
			a.say("leave the review first: v")
		case a.g.Over():
			a.say("the game is over")
		case a.set.mode == modeCvC:
			a.say("the computers will not resign")
		default:
			a.resignOpen = true
		}
		return nil
	})
	on("resign_yes", func(tuimark.Event) error {
		a.resignOpen = false
		a.after.focus = "#board"
		if a.g.Over() {
			// The game ended behind the dialog (a claim, a mate, the clock):
			// keep that result.
			a.say("the game ended meanwhile: " + a.g.Reason)
			return nil
		}
		loser := a.g.Cur().Turn
		if a.set.mode == modeHvC {
			loser = a.set.human
		}
		a.g.Resign(loser)
		a.invalidate()
		a.sel, a.targets, a.caps = -1, nil, nil
		return nil
	})
	on("resign_no", func(tuimark.Event) error { a.resignOpen = false; a.after.focus = "#board"; return nil })

	on("new_open", func(tuimark.Event) error {
		a.ng = ngDraft{mode: a.set.mode, side: "white", level: a.set.level, clock: a.set.clock.String()}
		if a.set.mode == modeHvC {
			a.ng.side = a.set.human.String()
		}
		a.newOpen = true
		return nil
	})
	on("ng_mode", func(tuimark.Event) error {
		order := []string{modeHvC, mode2P, modeCvC}
		a.ng.mode = order[(indexOf(order, a.ng.mode)+1)%len(order)]
		return nil
	})
	on("ng_side", func(tuimark.Event) error {
		order := []string{"white", "black", "random"}
		a.ng.side = order[(indexOf(order, a.ng.side)+1)%len(order)]
		return nil
	})
	on("ng_level", func(tuimark.Event) error { a.ng.level = a.ng.level%6 + 1; return nil })
	on("ng_clock", func(tuimark.Event) error {
		i := indexOf(clockChoices, a.ng.clock)
		a.ng.clock = clockChoices[(i+1)%len(clockChoices)]
		return nil
	})
	on("ng_start", func(tuimark.Event) error { return a.startFromDraft() })
	on("ng_cancel", func(tuimark.Event) error { a.newOpen = false; a.after.focus = "#board"; return nil })

	on("pgn", func(tuimark.Event) error { return a.exportPGN() })
	on("pgn_close", func(tuimark.Event) error { a.pgnOpen = false; a.after.focus = "#board"; return nil })

	on("review", func(tuimark.Event) error {
		if a.review {
			a.leaveReview()
		} else {
			a.enterReview(len(a.g.Moves))
		}
		return nil
	})

	for name, t := range map[string]PieceType{"promo_q": Queen, "promo_r": Rook, "promo_b": Bishop, "promo_n": Knight} {
		t := t
		on(name, func(tuimark.Event) error { return a.promote(t) })
	}
	on("promo_cancel", func(tuimark.Event) error { a.promoOpen = false; a.after.focus = "#board"; return nil })

	on("toggle_mouse", func(tuimark.Event) error { a.mouse = !a.mouse; return nil })
	on("toggle_letters", func(tuimark.Event) error { a.letters = !a.letters; return nil })
	return hs
}

func indexOf(s []string, v string) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}

// squareOfCell reads the square off a click's source id (vRC, the cell at
// screen row R and column C).
func (a *app) squareOfCell(id string) (int, bool) {
	if len(id) != 3 || id[0] != 'v' || id[1] < '0' || id[1] > '7' || id[2] < '0' || id[2] > '7' {
		return 0, false
	}
	return screenSq(int(id[1]-'0'), int(id[2]-'0'), a.flip), true
}

func (a *app) screenPos(sq int) (r, c int) {
	if a.flip {
		return rank(sq), 7 - file(sq)
	}
	return 7 - rank(sq), file(sq)
}

// arrow moves the cursor on the screen, or steps through the game in review.
func (a *app) arrow(dx, dy int) {
	if a.review {
		a.setPly(a.ply + dx)
		return
	}
	a.curOn = true
	r, c := a.screenPos(a.cur)
	a.cur = screenSq(clamp(r+dy, 0, 7), clamp(c+dx, 0, 7), a.flip)
}

// reviewPlayOn leaves the review at the position on the board and goes on
// playing from there, dropping the moves after it.
func (a *app) reviewPlayOn() error {
	dropped := len(a.g.Moves) - a.ply
	if dropped > 0 {
		// The rest of the game goes to the redo stack, not away, and the
		// clocks are those of the position played on from.
		a.g.Redo = nil
		for a.g.Ply() > a.ply {
			a.g.Undo()
		}
		if a.clk.spec.On() {
			a.clk.ms = a.g.Stamp[a.ply]
		}
	}
	a.leaveReview()
	switch {
	case dropped == 0:
		a.say("back to the game")
	case dropped == 1:
		a.say("playing on after " + a.moveLabel(a.ply) + " (U brings back the move after it)")
	default:
		a.say(fmt.Sprintf("playing on after %s (U brings back the %d moves after it)", a.moveLabel(a.ply), dropped))
	}
	return nil
}

// promote completes the pending pawn move with the chosen piece.
func (a *app) promote(t PieceType) error {
	a.promoOpen = false
	a.after.focus = "#board"
	m := Move{From: uint8(a.promo.from), To: uint8(a.promo.to), Promo: t}
	if !a.g.Cur().IsLegal(m) {
		a.say("that promotion is not legal now")
		return nil
	}
	return a.playMove(m)
}

// submitMove plays what was typed in the move box.
func (a *app) submitMove(text string) error {
	switch {
	case a.review:
		a.say("reviewing: v goes back to the game")
		return nil
	case a.g.Over():
		a.say("the game is over: n starts another")
		return nil
	case !a.humanTurn():
		a.say("it is the computer's turn")
		return nil
	}
	m, err := ParseMove(a.g.Cur(), text)
	switch {
	case errors.Is(err, ErrNeedPromotion):
		a.promo.from, a.promo.to = int(m.From), int(m.To)
		a.promoOpen = true
		a.after.clearCmd = true
		return nil
	case err != nil:
		a.say(err.Error())
		return nil // the text stays so it can be fixed
	}
	a.after.focus, a.after.clearCmd = "#board", true
	a.curOn = false
	return a.playMove(m)
}

// startFromDraft starts the game the new game dialog describes.
func (a *app) startFromDraft() error {
	clock, err := parseClock(a.ng.clock)
	if err != nil {
		a.say(err.Error())
		return nil
	}
	human := White
	switch a.ng.side {
	case "black":
		human = Black
	case "random":
		if a.rng.Intn(2) == 1 {
			human = Black
		}
	}
	s := settings{mode: a.ng.mode, human: human, level: a.ng.level, clock: clock}
	a.startGame(s, StartPosition(), nil)
	a.after.focus = "#board"
	return nil
}

// dialogPGN is the PGN for the dialog: the movetext on one line, which the
// dialog wraps to its own width (a file gets the 80-column wrap).
func (a *app) dialogPGN() string { return strings.TrimRight(a.g.pgnText(0), "\n") }

// exportPGN writes the game to the -save path, or shows it in a dialog.
func (a *app) exportPGN() error {
	a.g.Tags["Date"] = a.now().UTC().Format("2006.01.02")
	if a.save == "" {
		a.pgnText = a.dialogPGN()
		a.pgnOpen = true
		return nil
	}
	text := a.g.PGN()
	if err := os.WriteFile(a.save, []byte(text), 0o644); err != nil {
		a.say("cannot save: " + err.Error())
		return nil
	}
	a.say("saved " + a.save)
	return nil
}
