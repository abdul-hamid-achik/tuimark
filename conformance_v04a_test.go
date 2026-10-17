package tuimark_test

import (
	"strings"
	"testing"

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
