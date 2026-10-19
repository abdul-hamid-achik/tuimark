package tuimark_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

// SPEC v0.4b §24 conformance tests of the spinner and the clock step
// tick:DUR (phase F10): the parts the Go API observes. The CLI parts
// (dump and play output, exit codes, the golden manifest) are
// cmd/tuimark's conformance_v04b_clock_test.go; the Run side of the
// spinner (tests 194-196) is F11's.

// clockS is the document S.tui of tests 190-192, 196, 204, and 208.
const clockSFile = "testdata/v04b/spinner/S.tui"

func clockLoad(t *testing.T, file string) *tuimark.App {
	t.Helper()
	app, err := tuimark.Load(file)
	if err != nil {
		t.Fatal(err)
	}
	return app
}

func clockParse(t *testing.T, src string) *tuimark.App {
	t.Helper()
	app, err := tuimark.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	return app
}

func gridRow0(t *testing.T, d *tuimark.Dump) string {
	t.Helper()
	if d == nil || len(d.Grid) == 0 {
		t.Fatal("no dump grid")
	}
	return d.Grid[0]
}

// 190. The spinner at phase 0, through the public Dump.
func TestClock190SpinnerPhase0(t *testing.T) {
	app := clockLoad(t, clockSFile)
	d, err := app.Dump(20, 5)
	if err != nil {
		t.Fatal(err)
	}
	if !d.OK || len(d.Errors) != 0 {
		t.Fatalf("ok=%v errors=%v", d.OK, d.Errors)
	}
	type nodeBox struct {
		id, tag    string
		x, y, w, h int
	}
	var got []nodeBox
	for _, n := range d.Nodes {
		got = append(got, nodeBox{n.ID, n.Tag, n.X, n.Y, n.W, n.H})
		if n.Tag == "progress" && n.Text != "" {
			t.Errorf("progress %q has text %q", n.ID, n.Text)
		}
	}
	want := []nodeBox{
		{"main", "screen", 0, 0, 20, 5}, {"", "row", 0, 0, 20, 1}, {"sp", "progress", 0, 0, 1, 1},
		{"", "text", 2, 0, 7, 1}, {"mid", "progress", 0, 1, 5, 1}, {"end", "progress", 0, 2, 5, 1},
		{"start", "progress", 0, 3, 5, 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("nodes = %v\nwant %v", got, want)
	}
	wantGrid := []string{"⠋ loading           ", "  ⠋                 ", "    ⠋               ", "⠋                   ", "                    "}
	if !reflect.DeepEqual(d.Grid, wantGrid) {
		t.Errorf("grid = %q\nwant %q", d.Grid, wantGrid)
	}
	d2, _ := app.Dump(20, 5)
	if !reflect.DeepEqual(d, d2) {
		t.Error("a second Dump differs (no clock is read)")
	}
}

// 191 (Go side). tick moves the spinner through Play's frames, one entry
// per step, and Dump after all of it still shows phase 0.
func TestClock191PhasesThroughTick(t *testing.T) {
	app := clockLoad(t, clockSFile)
	steps := []string{"tick:99ms", "tick:1ms", "tick:900ms", "tick:250ms"}
	res, err := app.Play(tuimark.PlayOptions{Cols: 20, Rows: 5, Frames: true}, steps...)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Frames) != 5 {
		t.Fatalf("%d frames, want 5", len(res.Frames))
	}
	wantFirst := []string{"⠋", "⠋", "⠙", "⠋", "⠹"}
	wantInput := append([]string{""}, steps...)
	for i, f := range res.Frames {
		if f.Step != i || f.Input != wantInput[i] || len(f.Events) != 0 {
			t.Errorf("frame %d = step %d input %q events %v", i, f.Step, f.Input, f.Events)
		}
		if got := gridRow0(t, f.Dump); !strings.HasPrefix(got, wantFirst[i]) {
			t.Errorf("step %d grid[0] = %q, want it to start with %s", i, got, wantFirst[i])
		}
	}
	wantRows := []string{"⠹ loading           ", "  ⠹                 ", "    ⠹               ", "⠹                   "}
	if !reflect.DeepEqual(res.Dump.Grid[:4], wantRows) {
		t.Errorf("step 4 rows = %q", res.Dump.Grid[:4])
	}
	if !reflect.DeepEqual(res.Dump.Grid, res.Frames[4].Dump.Grid) || len(res.Events) != 0 {
		t.Error("the final dump is not the last frame's, or events is not empty")
	}
	d, _ := app.Dump(20, 5)
	if !strings.HasPrefix(gridRow0(t, d), "⠋") {
		t.Errorf("Dump after the ticks = %q, want phase 0", d.Grid[0])
	}
}

// 192. The virtual clock is app state: it persists across Play calls and
// neither Dump nor a stepless Play resets it.
func TestClock192VirtualClockIsAppState(t *testing.T) {
	app := clockLoad(t, clockSFile)
	opts := tuimark.PlayOptions{Cols: 20, Rows: 5}
	for i, want := range []string{"⠋", "⠙"} {
		res, err := app.Play(opts, "tick:60ms")
		if err != nil {
			t.Fatal(err)
		}
		if got := gridRow0(t, res.Dump); !strings.HasPrefix(got, want) {
			t.Errorf("call %d: %q, want %s", i+1, got, want)
		}
	}
	d, _ := app.Dump(20, 5)
	if !strings.HasPrefix(d.Grid[0], "⠋") {
		t.Errorf("Dump = %q, want phase 0", d.Grid[0])
	}
	res, err := app.Play(opts)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.Dump.Grid[0], "⠙") {
		t.Errorf("a Play with no step = %q, want the clock's phase 1 (step 0 renders it)", res.Dump.Grid[0])
	}
	d, _ = app.Dump(20, 5)
	if !strings.HasPrefix(d.Grid[0], "⠋") {
		t.Errorf("Dump after Play = %q, want phase 0", d.Grid[0])
	}
}

const clockNDoc = `<tui><screen id="main"><text>{n}</text></screen></tui>`

// clockCounter is test 197's app: n bound to 0 and an Every(250ms) that
// adds 1 to n. calls counts the function's calls.
func clockCounter(t *testing.T, calls *int) *tuimark.App {
	t.Helper()
	app := clockParse(t, clockNDoc)
	if err := app.Bind("n", 0); err != nil {
		t.Fatal(err)
	}
	app.Every(250*time.Millisecond, func(b *tuimark.Batch) error {
		if calls != nil {
			*calls++
		}
		v, _ := app.Get("n")
		return b.Set("n", v.(float64)+1)
	})
	return app
}

func nOf(t *testing.T, app *tuimark.App) float64 {
	t.Helper()
	v, ok := app.Get("n")
	if !ok {
		t.Fatal("n is missing")
	}
	return v.(float64)
}

// 197. Every and tick:.
func TestClock197EveryAndTick(t *testing.T) {
	var calls int
	app := clockCounter(t, &calls)
	res, err := app.Play(tuimark.PlayOptions{Cols: 10, Rows: 1, Frames: true}, "tick:1s")
	if err != nil {
		t.Fatal(err)
	}
	if res.Dump.Grid[0] != "4         " || len(res.Events) != 0 || res.Quit {
		t.Errorf("grid %q events %v quit %v", res.Dump.Grid[0], res.Events, res.Quit)
	}
	if len(res.Frames) != 2 || res.Frames[0].Dump.Grid[0] != "0         " || res.Frames[1].Dump.Grid[0] != "4         " {
		t.Errorf("frames = %+v", res.Frames)
	}
	if calls != 4 || nOf(t, app) != 4 {
		t.Errorf("fn ran %d times, n = %v", calls, nOf(t, app))
	}

	t.Run("249ms then 1ms", func(t *testing.T) {
		app := clockCounter(t, nil)
		opts := tuimark.PlayOptions{Cols: 10, Rows: 1}
		if _, err := app.Play(opts, "tick:249ms"); err != nil || nOf(t, app) != 0 {
			t.Fatalf("err %v, n = %v", err, nOf(t, app))
		}
		if _, err := app.Play(opts, "tick:1ms"); err != nil || nOf(t, app) != 1 {
			t.Fatalf("err %v, n = %v", err, nOf(t, app))
		}
	})
	t.Run("limit", func(t *testing.T) {
		app := clockCounter(t, nil)
		res, err := app.Play(tuimark.PlayOptions{Cols: 10, Rows: 1}, "tick:1h")
		want := "step 1 (tick:1h): tick: 14400 Every calls fall due; the limit is 10000 per step"
		if err == nil || err.Error() != want {
			t.Fatalf("err = %v, want %q", err, want)
		}
		if res == nil || res.Dump == nil {
			t.Fatal("no result so far")
		}
		if nOf(t, app) != 0 {
			t.Errorf("n = %v", nOf(t, app))
		}
		// V is unchanged: the first firing is still at 250 ms.
		if _, err := app.Play(tuimark.PlayOptions{}, "tick:249ms"); err != nil || nOf(t, app) != 0 {
			t.Fatalf("err %v, n = %v", err, nOf(t, app))
		}
	})
	t.Run("under the limit", func(t *testing.T) {
		app := clockCounter(t, nil)
		if _, err := app.Play(tuimark.PlayOptions{}, "tick:2500ms"); err != nil || nOf(t, app) != 10 {
			t.Fatalf("err %v, n = %v", err, nOf(t, app))
		}
	})
}

// clockLetters registers an Every(d) that appends letter to s.
func clockLetters(app *tuimark.App, d time.Duration, letter string) func() {
	return app.Every(d, func(b *tuimark.Batch) error {
		v, _ := app.Get("s")
		s, _ := v.(string)
		return b.Set("s", s+letter)
	})
}

func sOf(app *tuimark.App) string {
	v, _ := app.Get("s")
	s, _ := v.(string)
	return s
}

// 198. Order and ties, and one settle per instant.
func TestClock198OrderAndTies(t *testing.T) {
	opts := tuimark.PlayOptions{Cols: 10, Rows: 1}
	app := clockParse(t, clockNDoc)
	clockLetters(app, 100*time.Millisecond, "a")
	clockLetters(app, 250*time.Millisecond, "b")
	if _, err := app.Play(opts, "tick:500ms"); err != nil {
		t.Fatal(err)
	}
	if got := sOf(app); got != "aabaaab" {
		t.Errorf("s = %q, want aabaaab", got)
	}
	if _, err := app.Play(opts, "tick:100ms"); err != nil {
		t.Fatal(err)
	}
	if got := sOf(app); got != "aabaaaba" {
		t.Errorf("s = %q, want aabaaaba", got)
	}
	swapped := clockParse(t, clockNDoc)
	clockLetters(swapped, 250*time.Millisecond, "b")
	clockLetters(swapped, 100*time.Millisecond, "a")
	if _, err := swapped.Play(opts, "tick:500ms"); err != nil {
		t.Fatal(err)
	}
	if got := sOf(swapped); got != "aabaaba" {
		t.Errorf("B first: s = %q, want aabaaba", got)
	}
}

func TestClock198OneSettlePerInstant(t *testing.T) {
	app := clockParse(t, `<tui><screen id="main"><text>x</text><modal id="m" open="ui.open" on:open="load"><text>m</text></modal></screen></tui>`)
	if err := app.Bind("", map[string]any{"ui": map[string]any{"open": false}, "busy": false}); err != nil {
		t.Fatal(err)
	}
	app.On("load", func(tuimark.Event) error { return app.Set("busy", true) })
	var seen []any
	app.Every(250*time.Millisecond, func(b *tuimark.Batch) error { return b.Set("ui.open", true) })
	app.Every(250*time.Millisecond, func(*tuimark.Batch) error {
		v, _ := app.Get("busy")
		seen = append(seen, v)
		return nil
	})
	res, err := app.Play(tuimark.PlayOptions{}, "tick:250ms")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(seen, []any{false}) {
		t.Errorf("seen = %v, want [false]: one settle after both functions", seen)
	}
	want := []tuimark.PlayEvent{{Step: 1, Action: "load", Source: "m", Keys: map[string]any{}, Value: nil}}
	if !reflect.DeepEqual(res.Events, want) {
		t.Errorf("events = %+v, want %+v", res.Events, want)
	}
	if _, err := app.Play(tuimark.PlayOptions{}, "tick:250ms"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(seen, []any{false, true}) {
		t.Errorf("seen = %v, want [false true]", seen)
	}
}

// 199. Deadlines are virtual and persist across Play calls.
func TestClock199VirtualDeadlines(t *testing.T) {
	app := clockParse(t, clockNDoc)
	var c, c2 int
	app.Every(100*time.Millisecond, func(*tuimark.Batch) error { c++; return nil })
	play := func(step string) {
		t.Helper()
		if _, err := app.Play(tuimark.PlayOptions{}, step); err != nil {
			t.Fatal(err)
		}
	}
	play("tick:50ms")
	if c != 0 {
		t.Fatalf("c = %d after 50ms", c)
	}
	play("tick:50ms")
	if c != 1 {
		t.Fatalf("c = %d after 100ms", c)
	}
	play("tick:250ms")
	if c != 3 {
		t.Fatalf("c = %d at V = 350ms", c)
	}
	app.Every(100*time.Millisecond, func(*tuimark.Batch) error { c2++; return nil })
	play("tick:100ms")
	if c != 4 || c2 != 1 {
		t.Fatalf("c = %d, c2 = %d at V = 450ms, want 4 and 1", c, c2)
	}
}

// 200. Registering and stopping during a tick.
func TestClock200RegisterAndStopDuringTick(t *testing.T) {
	// F stops itself on its second call.
	{
		app := clockParse(t, clockNDoc)
		var g, f int
		var stopF func()
		stopF = app.Every(200*time.Millisecond, func(*tuimark.Batch) error {
			f++
			switch f {
			case 1:
				app.Every(100*time.Millisecond, func(*tuimark.Batch) error { g++; return nil })
			case 2:
				stopF()
			}
			return nil
		})
		if _, err := app.Play(tuimark.PlayOptions{}, "tick:1s"); err != nil {
			t.Fatal(err)
		}
		if f != 2 || g != 8 {
			t.Errorf("F ran %d times and G %d, want 2 and 8 (G at 300..1000; F first at 400)", f, g)
		}
	}
	// F's second call stops G's registration instead.
	{
		app := clockParse(t, clockNDoc)
		var g, f int
		var stopG func()
		app.Every(200*time.Millisecond, func(*tuimark.Batch) error {
			f++
			switch f {
			case 1:
				stopG = app.Every(100*time.Millisecond, func(*tuimark.Batch) error { g++; return nil })
			case 2:
				stopG()
			}
			return nil
		})
		if _, err := app.Play(tuimark.PlayOptions{}, "tick:1s"); err != nil {
			t.Fatal(err)
		}
		if f != 5 || g != 1 {
			t.Errorf("F ran %d times and G %d, want 5 (200..1000) and 1 (G does not run at 400)", f, g)
		}
	}
}

// 201. Events from Every carry the tick step.
func TestClock201EventsCarryTheTickStep(t *testing.T) {
	const doc = `<tui><screen id="main"><text>x</text><modal id="m" open="open" on:open="opened"><text>m</text></modal></screen></tui>`
	build := func() *tuimark.App {
		app := clockParse(t, doc)
		if err := app.Bind("open", false); err != nil {
			t.Fatal(err)
		}
		var stop func()
		stop = app.Every(300*time.Millisecond, func(b *tuimark.Batch) error {
			stop()
			return b.Set("open", true)
		})
		return app
	}
	want := func(step int) []tuimark.PlayEvent {
		return []tuimark.PlayEvent{{Step: step, Action: "opened", Source: "m", Keys: map[string]any{}, Value: nil}}
	}
	res, err := build().Play(tuimark.PlayOptions{Frames: true}, "tick:200ms", "tick:200ms")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(res.Events, want(2)) {
		t.Errorf("events = %+v", res.Events)
	}
	hasModal := func(d *tuimark.Dump) bool {
		for _, n := range d.Nodes {
			if n.ID == "m" {
				return true
			}
		}
		return false
	}
	if len(res.Frames) != 3 {
		t.Fatalf("%d frames", len(res.Frames))
	}
	if len(res.Frames[1].Events) != 0 || hasModal(res.Frames[1].Dump) {
		t.Error("step 1 must have no events and no modal")
	}
	if !reflect.DeepEqual(res.Frames[2].Events, want(2)) || !hasModal(res.Frames[2].Dump) {
		t.Error("step 2 must have the event and the modal")
	}
	res, err = build().Play(tuimark.PlayOptions{}, "tick:400ms")
	if err != nil || !reflect.DeepEqual(res.Events, want(1)) {
		t.Errorf("single step: err %v events %+v", err, res.Events)
	}
}

var errClockBoom = errors.New("boom")

// 202. Quit and error from an Every function.
func TestClock202QuitAndError(t *testing.T) {
	const doc = `<tui><screen id="main"><text>x</text><list id="l"><item><text>a</text></item><item><text>b</text></item></list></screen></tui>`
	build := func(ret error) (*tuimark.App, *int) {
		app := clockParse(t, doc)
		k := new(int)
		app.Every(100*time.Millisecond, func(*tuimark.Batch) error {
			*k++
			if *k == 3 {
				return ret
			}
			return nil
		})
		return app, k
	}
	app, k := build(tuimark.ErrQuit)
	res, err := app.Play(tuimark.PlayOptions{Frames: true}, "tick:1s", "down")
	if err != nil || !res.Quit || *k != 3 {
		t.Fatalf("err %v quit %v k %d", err, res != nil && res.Quit, *k)
	}
	if len(res.Events) != 0 {
		t.Errorf("the step down was applied: %+v", res.Events)
	}
	if len(res.Frames) != 2 || res.Frames[1].Input != "tick:1s" {
		t.Errorf("frames = %+v; the tick step must have its entry and down none", res.Frames)
	}
	app, k = build(errClockBoom)
	res, err = app.Play(tuimark.PlayOptions{}, "tick:1s")
	if !errors.Is(err, errClockBoom) || res == nil || res.Dump == nil || *k != 3 {
		t.Fatalf("err %v res %v k %d", err, res, *k)
	}
}

// 203 (Go side). NoHandlers skips the Every functions but moves the clock
// and the deadlines.
func TestClock203NoHandlers(t *testing.T) {
	var calls int
	app := clockCounter(t, &calls)
	if _, err := app.Play(tuimark.PlayOptions{NoHandlers: true}, "tick:1s"); err != nil {
		t.Fatal(err)
	}
	if calls != 0 || nOf(t, app) != 0 {
		t.Fatalf("fn ran %d times, n = %v", calls, nOf(t, app))
	}
	if _, err := app.Play(tuimark.PlayOptions{}, "tick:250ms"); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || nOf(t, app) != 1 {
		t.Errorf("fn ran %d times, n = %v; the deadline moved to 1250 ms, V = 1250 ms", calls, nOf(t, app))
	}
}

// 204 (Go side). A bad DUR is a parse error before any step is applied.
func TestClock204GoGrammar(t *testing.T) {
	app := clockParse(t, clockNDoc)
	res, err := app.Play(tuimark.PlayOptions{}, "set:n=7", "tick:0ms")
	want := "step 2 (tick:0ms): tick: duration must be at least 1ms"
	if err == nil || err.Error() != want || res != nil {
		t.Fatalf("err %v res %v, want %q and a nil result", err, res, want)
	}
	if _, ok := app.Get("n"); ok {
		t.Error("the set: step was applied")
	}
	for _, ok := range []string{"tick:1ms", "tick:250ms", "tick:1.5s", "tick:1m30s", "tick:60m", "tick:1h"} {
		if _, err := clockParse(t, clockNDoc).Play(tuimark.PlayOptions{}, ok); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
	// A time.Duration written with String() is a valid step.
	if _, err := app.Play(tuimark.PlayOptions{}, "tick:"+(1500*time.Millisecond).String()); err != nil {
		t.Errorf("Duration.String(): %v", err)
	}
}

// 205. The limit of 10 000 firings per step.
func TestClock205Limit(t *testing.T) {
	opts := tuimark.PlayOptions{Cols: 10, Rows: 1}
	build := func() (*tuimark.App, *int) {
		app := clockParse(t, clockNDoc)
		n := new(int)
		app.Every(time.Millisecond, func(*tuimark.Batch) error { *n++; return nil })
		return app, n
	}
	app, n := build()
	res, err := app.Play(opts, "set:x=1", "tick:1m")
	want := "step 2 (tick:1m): tick: 60000 Every calls fall due; the limit is 10000 per step"
	if err == nil || err.Error() != want || res == nil {
		t.Fatalf("err %v res %v, want %q", err, res, want)
	}
	if *n != 0 {
		t.Errorf("n = %d", *n)
	}
	if v, _ := app.Get("x"); v != float64(1) {
		t.Errorf("the earlier step was not applied: x = %v", v)
	}
	// V is unchanged: 10 s is exactly 10 000 firings and runs them all.
	if _, err := app.Play(opts, "tick:10s"); err != nil || *n != 10000 {
		t.Fatalf("tick:10s: err %v, n = %d", err, *n)
	}

	// A function that registers another Every(1ms) on each call.
	app = clockParse(t, clockNDoc)
	calls := 0
	var register func()
	register = func() {
		app.Every(time.Millisecond, func(*tuimark.Batch) error {
			calls++
			register()
			return nil
		})
	}
	register()
	_, err = app.Play(opts, "tick:1s")
	want = "step 1 (tick:1s): tick: more than 10000 Every calls in one step"
	if err == nil || err.Error() != want {
		t.Fatalf("err = %v, want %q", err, want)
	}
	if calls != 10000 {
		t.Errorf("%d firings stay applied, want 10000", calls)
	}
}

// 206. Nothing else ticks.
func TestClock206NothingElseTicks(t *testing.T) {
	var calls int
	app := clockCounter(t, &calls)
	if _, err := app.Dump(10, 1); err != nil {
		t.Fatal(err)
	}
	_ = app.Validate()
	if _, err := app.Play(tuimark.PlayOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Play(tuimark.PlayOptions{}, "down", "text:abc", "set:x=1"); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatalf("fn ran %d times with no tick step", calls)
	}
}

// 207. Pinned documents: tick fires Every as in the current language,
// carries no hint, and a spinner stays a bar.
func TestClock207PinnedDocuments(t *testing.T) {
	var current string
	for _, ver := range []string{"", ` version="1"`, ` version="3"`} {
		app := clockParse(t, `<tui`+ver+`><screen id="main"><text>{n}</text></screen></tui>`)
		if err := app.Bind("n", 0); err != nil {
			t.Fatal(err)
		}
		app.Every(250*time.Millisecond, func(b *tuimark.Batch) error {
			v, _ := app.Get("n")
			return b.Set("n", v.(float64)+1)
		})
		res, err := app.Play(tuimark.PlayOptions{Cols: 10, Rows: 1}, "tick:1s")
		if err != nil {
			t.Fatalf("version%q: %v", ver, err)
		}
		if len(res.Dump.Errors) != 0 || !res.Dump.OK {
			t.Errorf("version%q: diagnostics %v", ver, res.Dump.Errors)
		}
		if ver == "" {
			current = res.Dump.Grid[0]
		} else if res.Dump.Grid[0] != current {
			t.Errorf("version%q: grid %q, current language %q", ver, res.Dump.Grid[0], current)
		}
	}
	// A version="3" document with a spinner shows a bar at tick, whatever
	// the phase.
	app := clockParse(t, `<tui version="3"><screen id="main"><progress id="p" indeterminate="true"/></screen></tui>`)
	res, err := app.Play(tuimark.PlayOptions{Cols: 20, Rows: 1}, "tick:100ms", "tick:300ms")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := res.Dump.Grid[0], strings.Repeat("░", 20); got != want {
		t.Errorf("grid = %q, want a bar at 0", got)
	}
	if len(res.Dump.Errors) != 1 || !strings.HasSuffix(res.Dump.Errors[0].Msg, " (requires a <tui> without version)") {
		t.Errorf("errors = %v", res.Dump.Errors)
	}
}

// 210. A spinner in an each template and in a list <item> paints once per
// row, with the same glyph in every row at each phase.
func TestClock210SpinnerInTemplateAndListItem(t *testing.T) {
	app := clockParse(t, `<tui><screen id="main"><list id="l" each="rows as r" key="r.id"><item><progress indeterminate="true"/><text>{r.name}</text></item></list></screen></tui>`)
	if err := app.Bind("rows", []any{map[string]any{"id": 1, "name": "A"}, map[string]any{"id": 2, "name": "B"}}); err != nil {
		t.Fatal(err)
	}
	res, err := app.Play(tuimark.PlayOptions{Cols: 20, Rows: 4, Frames: true}, "tick:100ms")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Dump.Errors) != 0 {
		t.Fatalf("diagnostics: %v", res.Dump.Errors)
	}
	pad := func(s string) string { return s + strings.Repeat(" ", 20-len([]rune(s))) }
	if got, want := res.Frames[0].Dump.Grid, []string{pad("⠋"), pad("A"), pad("⠋"), pad("B")}; !reflect.DeepEqual(got, want) {
		t.Errorf("step 0 grid = %q, want %q", got, want)
	}
	if got, want := res.Frames[1].Dump.Grid, []string{pad("⠙"), pad("A"), pad("⠙"), pad("B")}; !reflect.DeepEqual(got, want) {
		t.Errorf("step 1 grid = %q, want %q", got, want)
	}
	// And in a container each template.
	app = clockParse(t, `<tui><screen id="main"><col each="rows as r"><progress indeterminate="true"/></col></screen></tui>`)
	if err := app.Bind("rows", []any{1, 2}); err != nil {
		t.Fatal(err)
	}
	d, err := app.Dump(20, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Errors) != 0 {
		t.Errorf("an each template: %v", d.Errors)
	}
}
