package main

import (
	"encoding/json"
	"io"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

// The scripted demo behind -demo COLSxROWS: a fresh game with the default
// seed, the autopilot switched on with the real key, then one frame per two
// ticks. Each line printed is {"ms": N, "dump": D}: the frame and how long it
// shows. The clock is a fake that advances by the game's own tick, and the
// only randomness is the seeded piece sequence, so the output is the same on
// every run and every OS.
const (
	demoFrames   = 92  // frames after the opening one
	demoFrameMs  = 100 // every frame shows for the time two ticks take
	demoLeadMs   = 600 // the opening frame, before any key
	demoHoldMs   = 1200
	ticksPerShot = demoFrameMs / tickMs

	// demoPace is the autopilot's speed in the demo: twice the normal one,
	// an act every tick, so ten seconds of film hold about twenty-five
	// pieces and the seed's first tetris.
	demoPace = 2.0
)

type frame struct {
	Ms   int `json:"ms"`
	Dump any `json:"dump"`
}

func demo(w io.Writer, dir, theme string, cols, rows int) error {
	a, err := load(dir, defaultSeed, 1, demoPace)
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

	// The opening frame, then the `a` key through Play, so the handler runs
	// exactly as in Run.
	if err := snap(demoLeadMs); err != nil {
		return err
	}
	if _, err := a.ui.Play(tuimark.PlayOptions{Cols: cols, Rows: rows}, "a"); err != nil {
		return err
	}

	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := a.ui.Batch(func(b *tuimark.Batch) error { return a.tick(b, clock) }); err != nil {
		return err
	}
	for i := 1; i <= demoFrames; i++ {
		for t := 0; t < ticksPerShot; t++ {
			clock = clock.Add(tickMs * time.Millisecond)
			if err := a.ui.Batch(func(b *tuimark.Batch) error { return a.tick(b, clock) }); err != nil {
				return err
			}
		}
		ms := demoFrameMs
		if i == demoFrames {
			ms = demoHoldMs
		}
		if err := snap(ms); err != nil {
			return err
		}
	}
	return nil
}
