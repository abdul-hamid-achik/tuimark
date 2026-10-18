package main

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

// The demo is the input of the docs site's animated SVG: one JSON object per
// line, {"ms": N, "dump": D}, where D is the Dump (with styles and cells) of
// one frame and N is how long that frame shows. Everything is deterministic:
// a fixed seed, a fake clock that moves a fixed step per frame, and keys
// sent through Play, so the handlers run exactly as in Run.
const (
	demoStep      = 250 * time.Millisecond // fake time per frame; 1s of run at -speed 4
	demoFrameMS   = 100                    // how long an ordinary frame shows
	demoMaxFrames = 120
)

// demoStart is the fake clock's first reading.
var demoStart = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

type demoFrame struct {
	MS   int           `json:"ms"`
	Dump *tuimark.Dump `json:"dump"`
}

// demo plays the run: it watches the build, moves the cursor to the
// integration job, sees it fail, presses r, and watches the retry pass and
// the deploy finish. The script reads the simulation's state, not frame
// numbers, so it keeps working if a duration changes.
func demo(w io.Writer, dir string, cols, rows int, theme string) error {
	// The demo is pinned to the default run (seed, flaky test, speed), so the
	// flags that change a live run do not change the artifact.
	a, err := load(dir, defaultSim(), demoStart)
	if err != nil {
		return err
	}
	if err := a.register(); err != nil {
		return err
	}
	if theme != "" {
		if err := a.ui.Set("@theme", theme); err != nil {
			return err
		}
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	emit := func(ms int) error {
		res, err := a.ui.Play(tuimark.PlayOptions{Cols: cols, Rows: rows, Styles: true, Cells: true})
		if err != nil {
			return err
		}
		return enc.Encode(demoFrame{MS: ms, Dump: res.Dump})
	}
	clock := demoStart
	tick := func() error {
		clock = clock.Add(demoStep)
		return a.ui.Batch(func(b *tuimark.Batch) error { return a.tick(b, clock) })
	}
	press := func(keys ...string) error {
		_, err := a.ui.Play(tuimark.PlayOptions{Cols: cols, Rows: rows}, keys...)
		return err
	}

	if err := emit(600); err != nil {
		return err
	}
	phase, left := "watch", 0
	for frame := 1; ; frame++ {
		if frame > demoMaxFrames {
			return fmt.Errorf("demo: still running after %d frames", demoMaxFrames)
		}
		ms := demoFrameMS
		integ := a.sim.find("integration")
		switch phase {
		case "watch": // the build passes, the tests start; go to integration
			if integ.status == stRunning && integ.elapsed >= 1.5 {
				phase, left = "move", 3
			}
		case "move":
			if err := press("down"); err != nil {
				return err
			}
			ms = 240
			if left--; left == 0 {
				phase, left = "failed", 4
			}
		case "failed": // wait for the failure, look at it, then retry
			if integ.status == stFailed {
				if left--; left == 0 {
					phase = "retry"
				}
				ms = 220
			}
		case "retry":
			if err := press("r"); err != nil {
				return err
			}
			phase, ms = "finish", 320
		case "finish":
			if a.sim.idle() {
				return emit(1400)
			}
		}
		if err := tick(); err != nil {
			return err
		}
		if err := emit(ms); err != nil {
			return err
		}
	}
}
