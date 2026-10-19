package main

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

// The scripted demo behind -demo COLSxROWS: a fixed game played by the
// single-point solver of solver.go, through the same clicks and keys a
// person would use. It switches to flag mode (f) to flag the cells it has
// proven to be mines, clicks open numbers to chord and covered cells to
// reveal, and leaves flag mode (f) again before a reveal. Each line printed
// is {"ms": N, "dump": D}: a frame and how long it shows. The clock is a
// fake that moves one second for every move, the only randomness is the
// seeded mines, and the solver reads no clock, so the output is the same on
// every run and every OS.
//
// demoSeed is a seed on which the solver wins; TestDemoSeedWins checks it
// and DEMO_SEARCH=1 go test -run TestDemoSeedSearch lists others.
var (
	demoBoard = board{"intermediate", 16, 16, 40}
	demoSeed  = int64(1)
)

const (
	demoLeadMs   = 700  // the opening frame, before any move
	demoHoldMs   = 1400 // the last frame, held so a loop restarts cleanly
	demoBodyMs   = 9000 // the moves share this much time
	demoMinStep  = 90
	demoMaxStep  = 450
	demoMaxMoves = 118 // frames, including the opening one
)

type frame struct {
	Ms   int `json:"ms"`
	Dump any `json:"dump"`
}

// demoSteps plays the whole game on a, a fresh app, and calls show with the
// result of every Play: the opening frame, then one per step. advance moves
// the fake clock. opts is what each Play is made with. It stops with an error if the game is lost or runs too
// long.
func demoSteps(a *app, advance func(time.Duration), opts tuimark.PlayOptions, show func(res *tuimark.PlayResult) error) error {
	res, err := a.ui.Play(opts)
	if err != nil {
		return err
	}
	if err := show(res); err != nil {
		return err
	}
	frames := 0
	step := func(input string) error {
		advance(time.Second)
		var err error
		if res, err = a.ui.Play(opts, input); err != nil {
			return err
		}
		frames++
		if frames > demoMaxMoves {
			return fmt.Errorf("demo: the solver needs more than %d moves", demoMaxMoves)
		}
		return show(res)
	}
	for {
		m, ok := nextMove(a.g)
		if !ok {
			break
		}
		if m.kind == mvFlag && !a.flagMode || m.kind == mvReveal && a.flagMode {
			if err := step("f"); err != nil {
				return err
			}
		}
		// Where to click comes from the dump of the frame on the screen,
		// never from a table.
		r, c := a.g.pos(m.cell)
		id := cellID(r, c)
		click := ""
		for _, n := range res.Dump.Nodes {
			if n.ID == id {
				click = fmt.Sprintf("click:%d,%d", n.X, n.Y)
				break
			}
		}
		if click == "" {
			return fmt.Errorf("demo: cell %s is not on the screen at %dx%d", id, opts.Cols, opts.Rows)
		}
		if err := step(click); err != nil {
			return err
		}
	}
	if !a.g.won {
		return fmt.Errorf("demo: the solver lost the game on seed %d", demoSeed)
	}
	return nil
}

func demo(w io.Writer, dir, theme string, cols, rows int) error {
	clock := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	a, err := load(dir, demoBoard, demoSeed, func() time.Time { return clock })
	if err != nil {
		return err
	}
	if theme != "" {
		if err := a.ui.Set("@theme", theme); err != nil {
			return err
		}
	}
	// The moves run through Play with the keys and clicks a person would use;
	// each frame is then a Play with Styles and Cells and no steps, as the
	// contract of the docs site spells out.
	plain := tuimark.PlayOptions{Cols: cols, Rows: rows}
	full := tuimark.PlayOptions{Cols: cols, Rows: rows, Styles: true, Cells: true}
	var dumps []any
	err = demoSteps(a, func(d time.Duration) { clock = clock.Add(d) }, plain, func(*tuimark.PlayResult) error {
		res, err := a.ui.Play(full)
		if err != nil {
			return err
		}
		dumps = append(dumps, res.Dump)
		return nil
	})
	if err != nil {
		return err
	}
	// The moves share demoBodyMs, within sane bounds per frame.
	n := len(dumps) - 1
	each := demoBodyMs / max(n, 1)
	each = clamp(each, demoMinStep, demoMaxStep)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	for i, d := range dumps {
		ms := each
		switch i {
		case 0:
			ms = demoLeadMs
		case len(dumps) - 1:
			ms = demoHoldMs
		}
		if err := enc.Encode(frame{Ms: ms, Dump: d}); err != nil {
			return err
		}
	}
	return nil
}
