package main

import (
	"encoding/json"
	"io"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

// The scripted demo behind -demo COLSxROWS: a fresh game with the default
// seed, the autopilot switched on with the real key, then one frame per
// move until the playing time reaches demoPlayMs. Each line printed is
// {"ms": N, "dump": D}: the frame and how long it shows. The clock is a
// fake that advances by the game's own interval, and the only randomness is
// the seeded food, so the output is the same on every run and every OS.
const (
	demoPlayMs = 6600 // time on the clock while the autopilot plays
	demoLeadMs = 500  // the opening frame, before any key
	demoHoldMs = 1300 // the last frame, held so a loop restarts cleanly
)

type frame struct {
	Ms   int `json:"ms"`
	Dump any `json:"dump"`
}

func demo(w io.Writer, dir, theme string, cols, rows int) error {
	a, err := load(dir, defaultSeed)
	if err != nil {
		return err
	}
	if theme != "" {
		if err := a.ui.Set("@theme", theme); err != nil {
			return err
		}
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	snap := func(ms int) error {
		res, err := a.ui.Play(tuimark.PlayOptions{Cols: cols, Rows: rows, Styles: true, Cells: true})
		if err != nil {
			return err
		}
		return enc.Encode(frame{Ms: ms, Dump: res.Dump})
	}

	// The opening frame, then the `A` key through Play, so the handler runs
	// exactly as in Run.
	if err := snap(demoLeadMs); err != nil {
		return err
	}
	if _, err := a.ui.Play(tuimark.PlayOptions{Cols: cols, Rows: rows}, "A"); err != nil {
		return err
	}

	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	played := 0
	for !a.g.over {
		ms := a.g.intervalMs()
		clock = clock.Add(time.Duration(ms) * time.Millisecond)
		if err := a.ui.Batch(func(b *tuimark.Batch) error { return a.tick(b, clock) }); err != nil {
			return err
		}
		played += ms
		next := a.g.intervalMs()
		if played >= demoPlayMs || a.g.over {
			return snap(demoHoldMs)
		}
		if err := snap(next); err != nil {
			return err
		}
	}
	return snap(demoHoldMs)
}
