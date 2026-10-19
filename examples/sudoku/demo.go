package main

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

// The scripted demo behind -demo COLSxROWS: an easy puzzle with few blanks,
// solved by the logic solver of logic.go through the app's own inputs. The
// cursor jumps to a cell with a mouse click, the digit goes in with its key.
// First the player pencils in the candidates of one hard-looking cell with
// the notes key. Each line printed is {"ms": N, "dump": D}: the frame and
// how long it shows.
//
// The clock is a fake that advances by what each frame shows, and the only
// randomness is the seeded generator, so the output is the same on every run
// and every machine.
const (
	demoSeed   = 3
	demoClues  = 52  // an easy puzzle with about 29 blanks, to fit the frame budget
	demoLeadMs = 500 // the opening frame, before any input
	demoMoveMs = 90  // a frame in which the cursor jumped
	demoPutMs  = 150 // a frame in which a digit went in
	demoHoldMs = 1300
)

type frame struct {
	Ms   int `json:"ms"`
	Dump any `json:"dump"`
}

// demoStep is one thing the script does: the click that moves the cursor
// to a cell, then the keys typed there.
type demoStep struct {
	cell int
	keys []string
}

// demoScript is the whole input of the demo for puzzle p. It starts with
// the notes on the cell that has the fewest candidates but more than one,
// then places every digit in the order the logic solver finds them.
func demoScript(p puzzle) ([]demoStep, error) {
	l := newLogic(p.puz)
	var steps []demoStep
	noted, nc := -1, 10
	for i := 0; i < size; i++ {
		if p.puz[i] == 0 {
			if n := count(l.cand[i]); n > 1 && n < nc {
				noted, nc = i, n
			}
		}
	}
	if noted >= 0 {
		keys := []string{"n"}
		for d := 1; d <= 9; d++ {
			if has(l.cand[noted], d) {
				keys = append(keys, fmt.Sprint(d))
			}
		}
		steps = append(steps, demoStep{cell: noted, keys: append(keys, "n")})
	}
	for !l.solved() {
		st, ok := l.next()
		if !ok {
			return nil, fmt.Errorf("the demo puzzle needs more than the logic solver has")
		}
		if st.place {
			steps = append(steps, demoStep{cell: st.idx, keys: []string{fmt.Sprint(st.digit)}})
		}
	}
	return steps, nil
}

func demo(w io.Writer, dir, theme string, cols, rows int) error {
	p := generateFloor(demoSeed, levelEasy, demoClues)
	a, err := load(dir, p, demoSeed)
	if err != nil {
		return err
	}
	if theme != "" {
		if err := a.ui.Set("@theme", theme); err != nil {
			return err
		}
	}
	steps, err := demoScript(p)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	opts := tuimark.PlayOptions{Cols: cols, Rows: rows}
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var tickErr error
	// advance runs the clock for ms of play, in ticks.
	advance := func(ms int) {
		for left := time.Duration(ms) * time.Millisecond; left > 0; left -= tickEvery {
			clock = clock.Add(min(tickEvery, left))
			if err := a.ui.Batch(func(b *tuimark.Batch) error { return a.tick(b, clock) }); err != nil && tickErr == nil {
				tickErr = err
			}
		}
	}
	snap := func(ms int) error {
		res, err := a.ui.Play(tuimark.PlayOptions{Cols: cols, Rows: rows, Styles: true, Cells: true})
		if err != nil {
			return err
		}
		return enc.Encode(frame{Ms: ms, Dump: res.Dump})
	}
	// input plays steps, lets the clock run for the frame, and shows it.
	input := func(ms int, steps ...string) error {
		if _, err := a.ui.Play(opts, steps...); err != nil {
			return err
		}
		advance(ms)
		return snap(ms)
	}

	tickErr = a.ui.Batch(func(b *tuimark.Batch) error { return a.tick(b, clock) }) // the reference time
	if err := snap(demoLeadMs); err != nil {
		return err
	}
	advance(demoLeadMs)
	for _, st := range steps {
		click, err := cellClick(a, cols, rows, st.cell)
		if err != nil {
			return err
		}
		if a.g.cur != st.cell {
			if err := input(demoMoveMs, click); err != nil {
				return err
			}
		}
		for _, k := range st.keys {
			if err := input(demoPutMs, k); err != nil {
				return err
			}
		}
	}
	if tickErr != nil {
		return tickErr
	}
	if !a.g.won {
		return fmt.Errorf("the demo ended with the board unsolved")
	}
	// The finished board shows, every cell highlighted, until the modal opens.
	if err := snap(int(winDelay / time.Millisecond)); err != nil {
		return err
	}
	advance(int(winDelay/time.Millisecond) + int(tickEvery/time.Millisecond))
	if !a.winOpen {
		return fmt.Errorf("the win modal did not open")
	}
	return snap(demoHoldMs)
}

// cellClick is the play step that clicks a cell at the demo's size.
func cellClick(a *app, cols, rows, i int) (string, error) {
	d, err := a.ui.Dump(cols, rows)
	if err != nil {
		return "", err
	}
	id := cellID(i)
	for _, n := range d.Nodes {
		if n.ID == id {
			return fmt.Sprintf("click:%d,%d", n.X+1, n.Y), nil
		}
	}
	return "", fmt.Errorf("no cell %s at %dx%d", id, cols, rows)
}
