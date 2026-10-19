package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

// The demo is the input of the docs site's animated SVG: one JSON object per
// line, {"ms": N, "dump": D}, where D is the Dump (with styles and cells) of
// one frame and N is how long that frame shows. Everything is deterministic:
// a fixed seed, a fake clock that moves one second per tick, and keys sent
// through Play, so the handlers run exactly as in Run.
const (
	demoMaxFrames = 140
	demoKeyMS     = 140 // a key press
	demoTickMS    = 260 // a second of engine time
)

// demoStart is the fake clock's first reading.
var demoStart = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

type demoFrame struct {
	MS   int           `json:"ms"`
	Dump *tuimark.Dump `json:"dump"`
}

// demo plays a session: filter the table and clear it, mark two rows, stop
// and start them through the dialog, open a container's detail and flip
// through its tabs, restart it from the command palette, wait for the
// scheduled crash of worker-email, read it in the events, and open the help.
// The script reads the engine's state, not frame numbers, so it keeps working
// if a duration changes.
func demo(w io.Writer, dir string, cols, rows int, theme string) error {
	// The demo is pinned to the default seed, so the flags that change a live
	// run do not change the artifact.
	a, err := load(dir, defaultSeed, demoStart)
	if err != nil {
		return err
	}
	if err := a.register(); err != nil {
		return err
	}
	if theme != "" {
		if err := a.setTheme(theme); err != nil {
			return err
		}
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	frames := 0
	emit := func(ms int) error {
		frames++
		if frames > demoMaxFrames {
			return fmt.Errorf("demo: more than %d frames", demoMaxFrames)
		}
		res, err := a.ui.Play(tuimark.PlayOptions{Cols: cols, Rows: rows, Styles: true, Cells: true})
		if err != nil {
			return err
		}
		return enc.Encode(demoFrame{MS: ms, Dump: res.Dump})
	}
	clock := demoStart
	tick := func(n int) error {
		for i := 0; i < n; i++ {
			clock = clock.Add(time.Second)
			if err := a.ui.Batch(func(b *tuimark.Batch) error { return a.tick(b, clock) }); err != nil {
				return err
			}
			if err := emit(demoTickMS); err != nil {
				return err
			}
		}
		return nil
	}
	press := func(keys ...string) error {
		for _, k := range keys {
			if _, err := a.ui.Play(tuimark.PlayOptions{Cols: cols, Rows: rows}, k); err != nil {
				return err
			}
			if err := emit(demoKeyMS); err != nil {
				return err
			}
		}
		return nil
	}
	typ := func(s string) error {
		for _, r := range s {
			if err := press("text:" + string(r)); err != nil {
				return err
			}
		}
		return nil
	}

	if err := emit(900); err != nil {
		return err
	}
	// Filter, then clear.
	steps := []func() error{
		func() error { return press("/") },
		func() error { return typ("work") },
		func() error { return tick(1) },
		func() error { return press("esc", "home") },
		// Mark the first two rows and stop them.
		func() error { return press("space", "down", "space") },
		func() error { return tick(1) },
		func() error { return press("s") },
		func() error { return tick(1) },
		func() error { return press("y") },
		func() error { return tick(2) },
		func() error { return press("u") },
		func() error { return tick(2) },
		func() error { return press("c", "home") },
		// Sort by CPU, then back to the name.
		func() error { return press("o") },
		func() error { return tick(2) },
		func() error { return press("o", "o", "o", "o") },
		// The detail of the cursor row and its nested tabs.
		func() error { return press("enter") },
		func() error { return tick(2) },
		func() error { return press("e") },
		func() error { return tick(1) },
		func() error { return press("p") },
		func() error { return tick(1) },
		func() error { return press("l") },
		func() error { return tick(2) },
		func() error { return press("esc") },
		// Restart it from the command palette.
		func() error { return press("ctrl+k") },
		func() error { return typ("rest") },
		func() error { return press("enter") },
		func() error { return tick(1) },
		func() error { return press("y") },
		func() error { return tick(2) },
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return err
		}
	}
	// Wait for worker-email's scheduled crash, then read it in the events.
	w0 := a.sim.byID(idByName(a.sim, "worker-email"))
	for a.sim.step < w0.crashAt+1 {
		if err := tick(1); err != nil {
			return err
		}
	}
	if err := press("5"); err != nil {
		return err
	}
	if err := tick(3); err != nil {
		return err
	}
	// The help overlay, then back to the table, where the status bar shows the
	// engine degraded until the container is back.
	if err := press("?"); err != nil {
		return err
	}
	if err := emit(1200); err != nil {
		return err
	}
	if err := press("esc", "1"); err != nil {
		return err
	}
	if err := tick(2); err != nil {
		return err
	}
	return emit(1400)
}

func idByName(s *sim, name string) string {
	for _, c := range s.cs {
		if strings.EqualFold(c.name, name) {
			return c.id
		}
	}
	return ""
}
