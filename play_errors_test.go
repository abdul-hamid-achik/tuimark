package tuimark_test

import (
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

// SPEC v0.3 §21 test 83: Play errors.
//
// (The "Play while Run's loop is active" half needs test 71's harness,
// which lives in internal/host as an internal test file; it is tested
// there, directly against the EnterPlay/ExitPlay primitive Play itself
// calls: see internal/host/play_batch_test.go.)

const playErrDoc = `<tui version="1">
  <screen id="s1" focus="#q"><input id="q" bind="query" on:focus="focused" on:change="changed"/></screen>
  <screen id="s2"><text>two</text></screen>
</tui>`

func newPlayErrApp(t *testing.T) *tuimark.App {
	t.Helper()
	app, err := tuimark.Parse(strings.NewReader(playErrDoc))
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Bind("", map[string]any{"query": ""}); err != nil {
		t.Fatal(err)
	}
	return app
}

// An invalid step (an unknown key, a bad click:, a prefix with nothing
// after the colon) returns an error naming its index and a nil result,
// and nothing is applied, step 0 included.
func TestPlayParseErrors(t *testing.T) {
	cases := []struct {
		name string
		step string
	}{
		{"bad click coordinate", "click:x"},
		{"unknown key token", "F13"},
		{"text without a colon", "text"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			app := newPlayErrApp(t)
			res, err := app.Play(tuimark.PlayOptions{}, "set:query=\"before\"", c.step)
			if err == nil {
				t.Fatal("want an error")
			}
			if !strings.Contains(err.Error(), "step 2") {
				t.Errorf("error = %q, want it to name step 2", err.Error())
			}
			if res != nil {
				t.Errorf("result = %+v, want nil (nothing applied, step 0 included)", res)
			}
			// Not even the first step applied: "before" never reached
			// the store, since a parse error stops before anything runs.
			if v, _ := app.Get("query"); v != "" {
				t.Errorf("query = %v, want unchanged (nothing applied)", v)
			}
			// @focus is still nil: not even step 0 (the live frame) ran.
			if v, ok := app.Get("@focus"); v != nil || !ok {
				t.Errorf("@focus = (%v, %v), want (nil, true): step 0 must not have run either", v, ok)
			}
		})
	}
}

// A size of -1 or 1001 is an error; Cols: 0 alone gives 80 columns and
// keeps Rows.
func TestPlaySizeErrors(t *testing.T) {
	app := newPlayErrApp(t)
	for _, sz := range []tuimark.PlayOptions{{Cols: -1}, {Cols: 1001}, {Rows: -1}, {Rows: 1001}} {
		if res, err := app.Play(sz); err == nil {
			t.Errorf("Play(%+v) succeeded, want a size error", sz)
		} else if res != nil {
			t.Errorf("Play(%+v) result = %+v, want nil", sz, res)
		}
	}

	app2 := newPlayErrApp(t)
	res, err := app2.Play(tuimark.PlayOptions{Cols: 0, Rows: 10})
	if err != nil {
		t.Fatalf("Cols: 0 alone: %v", err)
	}
	if res.Dump.Cols != 80 || res.Dump.Rows != 10 {
		t.Errorf("dump is %dx%d, want 80x10 (Cols: 0 defaults to 80, Rows kept)", res.Dump.Cols, res.Dump.Rows)
	}
}

// A set:@screen="nope" in step 2 returns "step 2 (…): …" with the events
// of steps 0 and 1, and step 1's effect stays.
func TestPlayApplyErrorMidCall(t *testing.T) {
	app := newPlayErrApp(t)
	// Step 0 fires "focused" (screen focus="#q", on:focus="focused");
	// step 1 (text:hi) fires one "changed" on:change.
	res, err := app.Play(tuimark.PlayOptions{}, "text:hi", `set:@screen="nope"`, `set:query="two"`)
	if err == nil {
		t.Fatal("want an error")
	}
	if !strings.Contains(err.Error(), "step 2") {
		t.Errorf("error = %q, want it to name step 2", err.Error())
	}
	if res == nil {
		t.Fatal("want the result so far, not nil")
	}
	wantSteps := map[int]string{0: "focused", 1: "changed"}
	if len(res.Events) != len(wantSteps) {
		t.Fatalf("events = %+v, want one per step in %v", res.Events, wantSteps)
	}
	for _, e := range res.Events {
		if wantSteps[e.Step] != e.Action {
			t.Errorf("event %+v, want action %q for step %d", e, wantSteps[e.Step], e.Step)
		}
	}
	// Step 1's effect (query="hi") stays; step 3 never ran.
	if v, _ := app.Get("query"); v != "hi" {
		t.Errorf("query = %v, want hi (step 1 stays applied, step 3 never ran)", v)
	}
}

// Run while Play is active (a handler that calls Run) returns an error
// before Run looks at the terminal. (Play while Run's loop is active is
// tested in internal/host, which has test 71's harness: see
// internal/host/play_batch_test.go.)
func TestRunDuringPlay(t *testing.T) {
	doc := `<tui version="1"><keymap><bind keys="g" action="go"/></keymap><screen><text>hi</text></screen></tui>`
	app, err := tuimark.Parse(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	called := false
	app.On("go", func(tuimark.Event) error {
		called = true
		// Run must return an error before it even looks at the terminal
		// (stdin is not one in a test binary either, but the mode guard
		// is checked first, so nil here still exercises the guard).
		if err := app.Run(nil); err == nil {
			t.Error("Run succeeded while Play is active, want an error")
		} else if !strings.Contains(err.Error(), "Play is active") {
			t.Errorf("Run error = %q, want it to name Play as active", err.Error())
		}
		return nil
	})
	if _, err := app.Play(tuimark.PlayOptions{}, "g"); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("the go handler never ran")
	}
}
