package main

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

// The demo is a scripted run for the docs site, printed as one JSON object
// per line: {"ms": N, "dump": D}, where D is the settled frame (with styles
// and cells) and N is how long it shows. It starts seven seconds before the
// end of the second work phase and plays it out in half-second steps, so the
// colon blinks and the last seconds pulse; then the done flash, and the
// short break starting to count. The clock is fake and in UTC, so the
// output is byte-identical on every run and every machine.

var demoStart = time.Date(2026, 10, 17, 14, 30, 0, 0, time.UTC)

const (
	demoStep      = 500 * time.Millisecond
	demoLeft      = 7 * time.Second // time left in the work phase when the demo opens
	demoCountdown = 14              // frames from 7s left down to 0.5s left
	demoFlash     = 3               // frames of the done flash
	demoBreak     = 5               // frames of the break, the last one held
	demoHold      = 1000            // ms the last frame shows
)

// fakeClock is the demo's and the tests' clock.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

// demoApp builds the app at the demo's opening state: the second work
// phase running with demoLeft to go, after a finished work phase and a
// short break.
func demoApp(dir, theme string) (*app, *fakeClock, error) {
	clock := &fakeClock{t: demoStart}
	ui, err := tuimark.Load(filepath.Join(dir, "app.tui"))
	if err != nil {
		return nil, nil, err
	}
	base := baseData()
	base["task"] = "write the spec"
	a, err := newApp(ui, base, plan{defaultWork, defaultShort, defaultLong}, true, defaultFlash, time.UTC, clock.now)
	if err != nil {
		return nil, nil, err
	}
	if theme != "" {
		if err := ui.Set("@theme", theme); err != nil {
			return nil, nil, err
		}
	}
	t := a.tm
	t.sessions = []session{
		{id: 1, kind: kindWork, start: time.Date(2026, 10, 17, 13, 34, 0, 0, time.UTC), dur: 25 * time.Minute, task: "outline the talk"},
		{id: 2, kind: kindShort, start: time.Date(2026, 10, 17, 14, 0, 1, 0, time.UTC), dur: 5 * time.Minute},
	}
	t.nextID = 2
	t.enter(2)
	t.left = demoLeft
	t.running = true
	t.started = demoStart.Add(-(t.total - t.left))
	t.task = "write the spec"
	t.last = demoStart
	if err := a.ui.Batch(func(b *tuimark.Batch) error { return a.publish(b, demoStart) }); err != nil {
		return nil, nil, err
	}
	return a, clock, nil
}

type demoLine struct {
	Ms   int           `json:"ms"`
	Dump *tuimark.Dump `json:"dump"`
}

// runDemo writes the demo frames to w.
func runDemo(o options, w io.Writer) error {
	cols, rows, err := parseSize("-demo", o.demo)
	if err != nil {
		return err
	}
	a, clock, err := demoApp(o.dir, o.theme)
	if err != nil {
		return err
	}
	frames := demoCountdown + demoFlash + demoBreak
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	for i := 0; i < frames; i++ {
		if i > 0 {
			clock.t = clock.t.Add(demoStep)
			now := clock.now()
			if err := a.ui.Batch(func(b *tuimark.Batch) error { return a.tick(b, now) }); err != nil {
				return err
			}
		}
		res, err := a.ui.Play(tuimark.PlayOptions{Cols: cols, Rows: rows, Styles: true, Cells: true})
		if err != nil {
			return err
		}
		if !res.Dump.OK || len(res.Dump.Errors) > 0 {
			return fmt.Errorf("demo frame %d: %v", i, res.Dump.Errors)
		}
		hold := int(demoStep / time.Millisecond)
		if i == frames-1 {
			hold = demoHold
		}
		if err := enc.Encode(demoLine{Ms: hold, Dump: res.Dump}); err != nil {
			return err
		}
	}
	return nil
}
