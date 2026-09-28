package play

import (
	"fmt"

	"github.com/abdul-hamid-achik/tuimark/internal/dump"
	"github.com/abdul-hamid-achik/tuimark/internal/host"
)

// Options configures Play; it mirrors tuimark.PlayOptions (0 for Cols/
// Rows defaults to 80/24, as for tuimark play). Frames is v0.3b: one
// settled frame per applied step, as play --frames (SPEC v0.3b §18.1).
type Options struct {
	Cols, Rows int
	NoHandlers bool
	Styles     bool
	Cells      bool
	Frames     bool
}

// Result is what one Play call did; it mirrors tuimark.PlayResult. Frames
// is set only when Options.Frames was (v0.3b, SPEC §18.1).
type Result struct {
	Events []dump.Event
	Dump   *dump.Dump
	Quit   bool
	Frames []dump.PlayFrame
}

// Play parses every step of the §15.4 --input grammar, then drives app
// through them exactly as Run's loop would from the same input,
// calling its registered handlers unless opts.NoHandlers is set (SPEC
// v0.3 §18.1). Every step is parsed before any is applied, so an invalid
// one changes nothing, not even step 0; a step that cannot be applied
// when its turn comes ends the call and is reported as
// "step N (STEP): reason", with the result so far. Play and Run exclude
// each other: a call made while Run, Loop, or another Play is active on
// app returns an error, a nil result, and does nothing.
func Play(app *host.App, opts Options, steps []string) (*Result, error) {
	if err := app.EnterPlay(); err != nil {
		return nil, err
	}
	defer app.ExitPlay()

	cols, rows := opts.Cols, opts.Rows
	if cols == 0 {
		cols = 80
	}
	if rows == 0 {
		rows = 24
	}
	if err := checkSize(cols, rows); err != nil {
		return nil, fmt.Errorf("tuimark: Play %v", err)
	}

	parsed := make([]Step, len(steps))
	for i, raw := range steps {
		st, err := ParseStep(raw)
		if err != nil {
			return nil, fmt.Errorf("step %d (%s): %v", i+1, raw, err)
		}
		parsed[i] = st
	}

	sess := NewSession(app, cols, rows, opts.Cells, opts.Styles, opts.Frames, opts.NoHandlers)
	sess.Run(parsed)
	res := &Result{Events: sess.Events, Dump: sess.BuildDump(), Quit: sess.Quit, Frames: sess.StepFrames}
	switch {
	case sess.UsageErr != nil:
		return res, fmt.Errorf("step %d (%s): %v", sess.UsageStep, sess.UsageRaw, sess.UsageErr)
	case sess.Err != nil:
		return res, sess.Err
	}
	return res, nil
}
