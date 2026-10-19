package tuimark_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

// SPEC v0.4a §21 conformance tests of the Go API additions (§18.2) and the
// rounds carry-over (§15.4 point 3).

// roundsDoc keeps queuing lifecycle events: every frame resolves focus in
// its three passes (SPEC §18 step 4) and each pass moves it, because a
// focused button is display: none. So each render queues roundsPerFrame
// on:focus events, the 8-round cap of every settle is reached, and the
// carry-over decides what happens to the events still queued. VERSION is
// replaced by the document version under test.
const roundsDoc = `<tui version="VERSION">
<style>button:focus { display: none; }</style>
<keymap><bind keys="k" action="kick"/></keymap>
<screen id="s">
  <button id="a" on:focus="fa">A</button>
  <button id="b" on:focus="fb">B</button>
</screen>
</tui>`

// roundsPerFrame is how many on:focus events one render of roundsDoc
// queues: one per focus pass (three, SPEC §18 step 4).
const roundsPerFrame = 3

func roundsApp(t *testing.T, version string) (*tuimark.App, *[]string) {
	t.Helper()
	app, err := tuimark.Parse(strings.NewReader(strings.Replace(roundsDoc, "VERSION", version, 1)))
	if err != nil {
		t.Fatal(err)
	}
	if d := app.Validate(); len(d) != 0 {
		t.Fatalf("fixture diagnostics: %v", d)
	}
	var called []string
	for _, act := range []string{"fa", "fb", "kick"} {
		app.On(act, func(ev tuimark.Event) error {
			called = append(called, ev.Action)
			return nil
		})
	}
	return app, &called
}

func stepEvents(evs []tuimark.PlayEvent, step int) []string {
	var out []string
	for _, e := range evs {
		if e.Step == step {
			out = append(out, e.Action)
		}
	}
	return out
}

// 115. Lifecycle events still queued when a settle's 8th round ends are
// kept, ahead of anything queued later, and dispatched first at the next
// settle (the next step, or the next Play call's step 0), before that
// step's own events; that settle's 8 rounds count from there (SPEC v0.4a
// §15.4 point 3). Play is called with handlers, on a version="1" and a
// version="3" document. Under Run, the TUIMARK_LOG rounds record is
// internal/host's TestRunLogsRoundsCap.
func TestRoundsCarryOver(t *testing.T) {
	for _, v := range []string{"1", "3"} {
		t.Run("version="+v, func(t *testing.T) {
			app, called := roundsApp(t, v)
			res, err := app.Play(tuimark.PlayOptions{}, "k")
			if err != nil {
				t.Fatal(err)
			}
			if len(*called) != len(res.Events) {
				t.Errorf("handlers called %d times for %d events", len(*called), len(res.Events))
			}
			// Step 0: the settle's first round holds the events of its
			// input, none for step 0; each of rounds 2 to 8 dispatches what
			// the render before it queued.
			if got := stepEvents(res.Events, 0); len(got) != 7*roundsPerFrame {
				t.Errorf("step 0 dispatched %d events, want %d: %v", len(got), 7*roundsPerFrame, got)
			}
			// Step 1: the events kept at step 0's cap (the batch taken for
			// a 9th round, then what the render after that take queued)
			// come first, then the key's own event.
			carry := 2 * roundsPerFrame
			got := stepEvents(res.Events, 1)
			if len(got) <= carry || got[carry] != "kick" {
				t.Fatalf("step 1 = %v, want %d carried on:focus events and then kick", got, carry)
			}
			for i, a := range got[:carry] {
				if a != "fa" && a != "fb" {
					t.Errorf("step 1 event %d = %q, want a carried on:focus event", i, a)
				}
			}
			// The next Play call's step 0 takes the events step 1's settle
			// kept: they are the first of its 8 rounds, so it dispatches
			// them plus 7 rounds of one render each.
			res2, err := app.Play(tuimark.PlayOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if want := carry + 7*roundsPerFrame; len(res2.Events) != want {
				t.Errorf("the next call's step 0 dispatched %d events, want %d (%d carried + 7 rounds)", len(res2.Events), want, carry)
			}
		})
	}
}

// The public 0.4a surface (SPEC v0.4a §18.2), through package tuimark;
// the behavior itself is covered by internal/host (tests 110-114) and
// internal/play.

const counterDoc = `<tui version="1"><screen id="s"><text>N={n} C={count}</text></screen></tui>`

func counterApp(t *testing.T) *tuimark.App {
	t.Helper()
	app, err := tuimark.Parse(strings.NewReader(counterDoc))
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Bind("", map[string]any{"n": 0, "items": []any{"a", "b"}}); err != nil {
		t.Fatal(err)
	}
	return app
}

// 110: a context already done returns its error, wrapped, before the
// terminal is touched.
func TestPublicRunWithCancelledContext(t *testing.T) {
	app := counterApp(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out strings.Builder
	err := app.RunWith(ctx, &out, tuimark.RunOptions{OnError: func(*tuimark.Event, error) error { return nil }})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RunWith = %v, want context.Canceled", err)
	}
	if err := app.RunContext(ctx, &out); !errors.Is(err, context.Canceled) {
		t.Fatalf("RunContext = %v, want context.Canceled", err)
	}
	if out.Len() != 0 {
		t.Fatalf("wrote %q", out.String())
	}
}

// 111, 113: Quit and Every outside Run change nothing, and Play without a
// tick step never calls an Every function (v0.4b: a tick step does, test
// 206).
func TestPublicQuitAndEveryOutsideRun(t *testing.T) {
	app := counterApp(t)
	var calls atomic.Int32
	stop := app.Every(time.Millisecond, func(b *tuimark.Batch) error {
		calls.Add(1)
		return b.Set("n", 99)
	})
	defer stop()
	app.Quit()
	res, err := app.Play(tuimark.PlayOptions{Cols: 20, Rows: 1}, "set:n=1")
	if err != nil || res.Quit {
		t.Fatalf("Play = %v, quit %v; an earlier Quit must not reach it", err, res.Quit)
	}
	time.Sleep(10 * time.Millisecond)
	if _, err := app.Dump(20, 1); err != nil {
		t.Fatal(err)
	}
	if n := calls.Load(); n != 0 {
		t.Fatalf("the Every function ran %d times outside Run", n)
	}
	if v, _ := app.Get("n"); v != float64(1) {
		t.Fatalf("n = %v", v)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("Every(0, …) did not panic")
		}
	}()
	app.Every(0, func(*tuimark.Batch) error { return nil })
}

// 114: Derive through the public API.
func TestPublicDerive(t *testing.T) {
	app := counterApp(t)
	if err := app.Derive("count", func(get func(string) (any, bool)) (any, error) {
		v, _ := get("items")
		arr, _ := v.([]any)
		return len(arr), nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := app.Set("items", []any{"a", "b", "c"}); err != nil {
		t.Fatal(err)
	}
	if v, _ := app.Get("count"); v != float64(3) {
		t.Fatalf("count = %v", v)
	}
	if err := app.Set("count", 1); err == nil {
		t.Fatal("Set of a derived path: want an error")
	}
	if err := app.Derive("@focus", func(func(string) (any, bool)) (any, error) { return nil, nil }); err == nil {
		t.Fatal("Derive of a reserved path: want an error")
	}
	res, err := app.Play(tuimark.PlayOptions{Cols: 20, Rows: 1}, `set:items=["x"]`)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(res.Dump.Grid[0]); got != "N=0 C=1" {
		t.Fatalf("grid = %q", got)
	}
	boom := errors.New("boom")
	if err := app.Derive("bad", func(get func(string) (any, bool)) (any, error) {
		if v, _ := get("n"); v == float64(2) {
			return nil, boom
		}
		return nil, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Play(tuimark.PlayOptions{Cols: 20, Rows: 1}, "set:n=2", "set:n=3"); !errors.Is(err, boom) {
		t.Fatalf("Play = %v, want the Derive error", err)
	}
	if v, _ := app.Get("n"); v != float64(2) {
		t.Fatalf("n = %v; the step after the error was applied", v)
	}
}
