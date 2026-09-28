package tuimark_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
	"github.com/abdul-hamid-achik/tuimark/internal/host"
	"github.com/abdul-hamid-achik/tuimark/internal/play"
)

// SPEC v0.3b §21 test 102: Play frames, against the draft's frames.tui
// fixture (specs/fixtures/frames.tui, specs/fixtures/frames.json).

const (
	framesFixtureTui  = "specs/fixtures/frames.tui"
	framesFixtureJSON = "specs/fixtures/frames.json"
)

func framesFixtureData(t *testing.T) any {
	t.Helper()
	raw, err := os.ReadFile(framesFixtureJSON)
	if err != nil {
		t.Fatal(err)
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func loadFramesApp(t *testing.T) *tuimark.App {
	t.Helper()
	app, err := tuimark.Load(framesFixtureTui)
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Bind("", framesFixtureData(t)); err != nil {
		t.Fatal(err)
	}
	return app
}

// selectedKey returns the key of the <item> row a dump marks selected.
func selectedKey(t *testing.T, d *tuimark.Dump) string {
	t.Helper()
	for _, n := range d.Nodes {
		if n.Tag == "item" && n.Selected {
			return n.Key
		}
	}
	t.Fatalf("no selected item in dump nodes: %+v", d.Nodes)
	return ""
}

// Play(PlayOptions{Frames: true, NoHandlers: true}, "down", "down") on a
// fresh app returns 3 frames, for steps 0, 1, and 2, with inputs "",
// "down", and "down"; their dumps select items a, b, and c in turn, and
// their events are [], [picked b], [picked c].
func TestPlayFramesBasic(t *testing.T) {
	app := loadFramesApp(t)
	res, err := app.Play(tuimark.PlayOptions{Cols: 20, Rows: 6, Frames: true, NoHandlers: true}, "down", "down")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Frames) != 3 {
		t.Fatalf("len(Frames) = %d, want 3", len(res.Frames))
	}

	wantSteps := []int{0, 1, 2}
	wantInputs := []string{"", "down", "down"}
	wantSelected := []string{"a", "b", "c"}
	for i, pf := range res.Frames {
		if pf.Step != wantSteps[i] {
			t.Errorf("Frames[%d].Step = %d, want %d", i, pf.Step, wantSteps[i])
		}
		if pf.Input != wantInputs[i] {
			t.Errorf("Frames[%d].Input = %q, want %q", i, pf.Input, wantInputs[i])
		}
		if got := selectedKey(t, pf.Dump); got != wantSelected[i] {
			t.Errorf("Frames[%d] selected = %q, want %q", i, got, wantSelected[i])
		}
	}

	if len(res.Frames[0].Events) != 0 {
		t.Errorf("Frames[0].Events = %+v, want none", res.Frames[0].Events)
	}
	for i, wantKey := range []string{"b", "c"} {
		evs := res.Frames[i+1].Events
		if len(evs) != 1 {
			t.Fatalf("Frames[%d].Events = %+v, want exactly one", i+1, evs)
		}
		e := evs[0]
		if e.Step != i+1 || e.Action != "picked" || e.Source != "rows" || e.Value != nil {
			t.Errorf("Frames[%d].Events[0] = %+v, want step %d picked/rows/nil value", i+1, e, i+1)
		}
		if e.Keys["it"] != wantKey {
			t.Errorf("Frames[%d].Events[0].Keys = %+v, want it=%q", i+1, e.Keys, wantKey)
		}
	}

	// Every Frame's Events is non-nil (marshals as [] rather than null).
	for i, pf := range res.Frames {
		if pf.Events == nil {
			t.Errorf("Frames[%d].Events is nil, want non-nil (even if empty)", i)
		}
	}
}

// Without opts.Frames, PlayResult.Frames is nil.
func TestPlayFramesNilWithoutOption(t *testing.T) {
	app := loadFramesApp(t)
	res, err := app.Play(tuimark.PlayOptions{Cols: 20, Rows: 6, NoHandlers: true}, "down", "down")
	if err != nil {
		t.Fatal(err)
	}
	if res.Frames != nil {
		t.Errorf("Frames = %+v, want nil", res.Frames)
	}
}

// With the steps "down", "q", "down": 3 frames (steps 0, 1, 2). The last
// has input "q", b still selected, and the quit event (keys: {it: b}).
// Quit is true, and the second "down" (step 3) is never applied.
func TestPlayFramesEndsEarlyOnQuit(t *testing.T) {
	app := loadFramesApp(t)
	res, err := app.Play(tuimark.PlayOptions{Cols: 20, Rows: 6, Frames: true, NoHandlers: true}, "down", "q", "down")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Quit {
		t.Fatal("Quit = false, want true")
	}
	if len(res.Frames) != 3 {
		t.Fatalf("len(Frames) = %d, want 3 (the un-applied third step has no entry)", len(res.Frames))
	}
	last := res.Frames[2]
	if last.Step != 2 || last.Input != "q" {
		t.Errorf("last frame = step %d input %q, want step 2 input \"q\"", last.Step, last.Input)
	}
	if got := selectedKey(t, last.Dump); got != "b" {
		t.Errorf("last frame selected = %q, want b (still selected)", got)
	}
	if len(last.Events) != 1 {
		t.Fatalf("last frame events = %+v, want exactly one (the quit event)", last.Events)
	}
	qe := last.Events[0]
	if qe.Action != "quit" || qe.Source != "rows" || qe.Keys["it"] != "b" {
		t.Errorf("quit event = %+v, want action=quit source=rows keys={it:b}", qe)
	}
}

// tuimark play frames.tui --data frames.json --cols 20 --rows 6 --input
// "down down" --frames --format json and Play(opts.Frames: true, ...)
// give the same frames, member for member (SPEC v0.3b §18.1 rule 9): one
// engine, internal/play.Session, serves both tuimark.Play (through
// internal/play.Play) and cmd/tuimark's play command. This drives that
// engine exactly as cmd/tuimark's cmdPlay does (cells=false, styles=
// false, frames=true, noHandlers=true, over internal/host.App directly)
// and compares the two paths' JSON, so a bug in either wiring — Play
// forgetting to pass Frames through, say — would show up as a mismatch.
func TestPlayFramesParityWithCLIEngine(t *testing.T) {
	app := loadFramesApp(t)
	res, err := app.Play(tuimark.PlayOptions{Cols: 20, Rows: 6, Frames: true, NoHandlers: true}, "down", "down")
	if err != nil {
		t.Fatal(err)
	}
	gotViaPlay, err := json.Marshal(res.Frames)
	if err != nil {
		t.Fatal(err)
	}

	hostApp, err := host.Load(framesFixtureTui)
	if err != nil {
		t.Fatal(err)
	}
	if err := hostApp.Bind("", framesFixtureData(t)); err != nil {
		t.Fatal(err)
	}
	steps, err := play.ParseSteps("down down")
	if err != nil {
		t.Fatal(err)
	}
	sess := play.NewSession(hostApp, 20, 6, false, false, true, true)
	sess.Run(steps)
	gotViaCLIEngine, err := json.Marshal(sess.StepFrames)
	if err != nil {
		t.Fatal(err)
	}

	if string(gotViaPlay) != string(gotViaCLIEngine) {
		t.Errorf("Play's frames and tuimark play's engine's frames differ:\nPlay:   %s\nengine: %s", gotViaPlay, gotViaCLIEngine)
	}
}
