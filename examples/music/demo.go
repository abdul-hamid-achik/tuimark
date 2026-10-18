package main

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

// The scripted demo behind -demo: a fixed run of frames, one per tick of
// the fake clock, that the docs site turns into an animated SVG.
const (
	demoFrames = 80
	demoSpeed  = 6 // the playback clock runs 6x, so the position bar visibly moves
	demoHoldMs = 1300
)

// demoKeys is the input applied before the tick of a frame.
var demoKeys = map[int][]string{
	8:  {"a"},     // queue the track under the cursor: the tab label counts up
	30: {"n"},     // skip: the next track is the queued one
	52: {"space"}, // pause: the bars fall
	66: {"space"}, // resume: the bars come back
}

// demoFrame is one line of -demo output.
type demoFrame struct {
	Ms   int           `json:"ms"`
	Dump *tuimark.Dump `json:"dump"`
}

// demo plays the script on a, which must be freshly loaded and registered,
// and writes one {"ms":N,"dump":D} object per line to w. Each frame applies
// that frame's keys through Play (so the handlers run as in Run), calls the
// same tick function Every uses with the fake clock advanced, and dumps the
// settled frame with styles and cells. Nothing in it reads the real clock
// or a global random source, so the bytes are the same on every run.
func (a *app) demo(w io.Writer, cols, rows int) error {
	a.p.speed = demoSpeed
	// The opening state ran sampleTicks ticks from the epoch; the script
	// continues on that clock.
	start := epoch.Add((sampleTicks + 1) * tickStep)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	for i := 0; i < demoFrames; i++ {
		if keys := demoKeys[i]; len(keys) > 0 {
			if _, err := a.ui.Play(tuimark.PlayOptions{Cols: cols, Rows: rows}, keys...); err != nil {
				return fmt.Errorf("frame %d: %w", i, err)
			}
		}
		now := start.Add(time.Duration(i) * tickStep)
		if err := a.ui.Batch(func(b *tuimark.Batch) error { return a.tick(b, now) }); err != nil {
			return fmt.Errorf("frame %d: %w", i, err)
		}
		res, err := a.ui.Play(tuimark.PlayOptions{Cols: cols, Rows: rows, Styles: true, Cells: true})
		if err != nil {
			return fmt.Errorf("frame %d: %w", i, err)
		}
		ms := int(tickStep / time.Millisecond)
		if i == demoFrames-1 {
			ms = demoHoldMs
		}
		if err := enc.Encode(demoFrame{Ms: ms, Dump: res.Dump}); err != nil {
			return err
		}
	}
	return nil
}
