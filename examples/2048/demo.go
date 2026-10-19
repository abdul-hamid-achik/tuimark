package main

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

// The scripted demo behind -demo COLSxROWS: a fresh game with the default
// seed and the autopilot switched on with the real key. The first
// demoSlowMoves moves get a frame each, at the autopilot's own pace, so the
// pop and new-tile highlights read; after that each frame is demoGroup moves
// later and shows for less, so the score climbs far enough to show big tiles
// inside the time the docs site allows. Then the autopilot is switched off,
// the highlights clear, and the last frame holds. Each line printed is
// {"ms": N, "dump": D}: the frame and how long it shows. The clock is a
// fake that advances tick by tick, and the only randomness is the seeded
// spawns, so the output is the same on every run and every OS.
const (
	demoLeadMs     = 500  // the opening frame, before any key
	demoHoldMs     = 1300 // the last frame, held so a loop restarts cleanly
	demoSlowMoves  = 26   // moves shown one frame each
	demoSlowMs     = 150  // how long each of those frames shows
	demoFastFrames = 60   // frames after that
	demoGroup      = 4    // moves between two of those frames
	demoFastMs     = 85   // how long each of those frames shows
)

type frame struct {
	Ms   int `json:"ms"`
	Dump any `json:"dump"`
}

func demo(w io.Writer, dir, theme string, cols, rows int) error {
	a, err := load(dir, defaultConfig())
	if err != nil {
		return err
	}
	if theme != "" {
		if err := a.ui.Set("@theme", theme); err != nil {
			return err
		}
	}
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	a.clock = func() time.Time { return clock }

	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	snap := func(ms int) error {
		res, err := a.ui.Play(tuimark.PlayOptions{Cols: cols, Rows: rows, Styles: true, Cells: true})
		if err != nil {
			return err
		}
		return enc.Encode(frame{Ms: ms, Dump: res.Dump})
	}
	// tick advances the fake clock by one tick and runs the Every function.
	tick := func() error {
		clock = clock.Add(tickEvery)
		return a.ui.Batch(func(b *tuimark.Batch) error { return a.tick(b, clock) })
	}
	// nextMove ticks until the autopilot has moved once; false when it cannot
	// (the game ended, or a modal holds it).
	nextMove := func() (bool, error) {
		before := a.g.moves
		for i := 0; i < 100; i++ {
			if a.g.over || a.modalOpen() {
				return false, nil
			}
			if err := tick(); err != nil {
				return false, err
			}
			if a.g.moves != before {
				return true, nil
			}
		}
		return false, fmt.Errorf("the autopilot did not move in 100 ticks")
	}

	if err := snap(demoLeadMs); err != nil {
		return err
	}
	// The `A` key through Play, so the handler runs exactly as in Run.
	if _, err := a.ui.Play(tuimark.PlayOptions{Cols: cols, Rows: rows}, "A"); err != nil {
		return err
	}

	alive := true
	for i := 0; i < demoSlowMoves && alive; i++ {
		ok, err := nextMove()
		if err != nil {
			return err
		}
		if alive = ok; ok {
			if err := snap(demoSlowMs); err != nil {
				return err
			}
		}
	}
	for f := 0; f < demoFastFrames && alive; f++ {
		for m := 0; m < demoGroup && alive; m++ {
			ok, err := nextMove()
			if err != nil {
				return err
			}
			alive = ok
		}
		if err := snap(demoFastMs); err != nil {
			return err
		}
	}

	// Hand the board back: the autopilot off, the highlights cleared.
	if a.auto {
		if _, err := a.ui.Play(tuimark.PlayOptions{Cols: cols, Rows: rows}, "A"); err != nil {
			return err
		}
	}
	for i := 0; i < int(a.cfg.flash/tickEvery)+1; i++ {
		if err := tick(); err != nil {
			return err
		}
	}
	return snap(demoHoldMs)
}
