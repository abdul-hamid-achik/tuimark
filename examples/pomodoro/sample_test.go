package main

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"regexp"
	"sort"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

// sample.json and mock.json are generated from the real timer, so they
// cannot drift from it: TestSampleInSync fails when they differ, and
//
//	UPDATE_SAMPLE=1 go test ./examples/pomodoro -run TestSampleInSync
//
// rewrites them.
//
// sample.json is the whole store, derived paths included, of this
// documented state: the timer was started at 14:00:00 UTC on 2026-10-17
// with the task "write the spec" and left running with the default phases
// (25m, 5m, 15m, auto-start, a 1.5s flash). The clock now reads 14:34:07 UTC.
// So a work phase (task "write the spec") and a short break are done, and
// the second work phase has been running since 14:30:01.5, with 20:55 on
// the clock. `tuimark validate/dump/play` run no Go host and no Derive, so
// they read this file as it is.

var (
	sampleStart = time.Date(2026, 10, 17, 14, 0, 0, 0, time.UTC)
	sampleNow   = time.Date(2026, 10, 17, 14, 34, 7, 0, time.UTC)
	defaultPlan = plan{defaultWork, defaultShort, defaultLong}
)

// newTestApp loads this directory's app.tui on the view-owned base data,
// with a fake clock at start. The timer has not been started.
func newTestApp(t *testing.T, start time.Time) (*app, *fakeClock) {
	t.Helper()
	clock := &fakeClock{t: start}
	ui, err := tuimark.Load("app.tui")
	if err != nil {
		t.Fatal(err)
	}
	a, err := newApp(ui, baseData(), defaultPlan, true, defaultFlash, time.UTC, clock.now)
	if err != nil {
		t.Fatal(err)
	}
	return a, clock
}

// advance moves the fake clock to now and ticks, as Every would.
func advance(t *testing.T, a *app, clock *fakeClock, now time.Time) {
	t.Helper()
	clock.t = now
	if err := a.ui.Batch(func(b *tuimark.Batch) error { return a.tick(b, now) }); err != nil {
		t.Fatal(err)
	}
}

// sampleApp builds the app in the documented sample state.
func sampleApp(t *testing.T) (*app, *fakeClock) {
	t.Helper()
	a, clock := newTestApp(t, sampleStart)
	if err := a.ui.Set("task", "write the spec"); err != nil {
		t.Fatal(err)
	}
	if err := a.start(true); err != nil {
		t.Fatal(err)
	}
	advance(t, a, clock, sampleNow)
	return a, clock
}

func store(t *testing.T, a *app) map[string]any {
	t.Helper()
	v, ok := a.ui.Get("")
	m, isMap := v.(map[string]any)
	if !ok || !isMap {
		t.Fatalf("Get(\"\") = %v, %v", v, ok)
	}
	return m
}

func sampleData(t *testing.T) map[string]any {
	a, _ := sampleApp(t)
	return store(t, a)
}

// mockData applies literal values only (tuimark run has no Go host and no
// Derive), so each action carries what the real handler visibly does to
// the store, measured against the state just before the action: every
// top-level key whose value differs, and, for an action whose handler moves
// focus itself (task_done), a write to @focus (a Batch write to @focus
// moves focus). The focus the runtime moves by itself, as it does into a
// modal that opens and back out of it, needs no write. An action that
// undoes another (help_close) is measured after the one it undoes ran
// first.
func mockData(t *testing.T) map[string]any {
	t.Helper()
	// Focus is decided when a frame is built, so settle one before reading it.
	focus := func(a *app) any {
		play(t, a, 90, 24)
		f, _ := a.ui.Get("@focus")
		return f
	}
	mock := map[string]any{}
	for _, c := range []struct {
		action string
		focus  bool         // the handler moves focus itself
		setup  func(a *app) // brings the app to the state the action runs in
	}{
		{"toggle", false, nil},
		{"reset", false, nil},
		{"skip", false, nil},
		{"minute_add", false, nil},
		{"minute_sub", false, nil},
		{"help_open", false, nil},
		{"help_close", false, func(a *app) { call(t, a, "help_open") }},
		{"task_done", true, func(a *app) {
			if err := a.ui.Set("@focus", "#task"); err != nil {
				t.Fatal(err)
			}
			if f := focus(a); f != "#task" {
				t.Fatalf("focus = %v, want #task", f)
			}
		}},
	} {
		a, _ := sampleApp(t)
		if c.setup != nil {
			c.setup(a)
		}
		before, focusBefore := store(t, a), focus(a)
		call(t, a, c.action)
		after, focusAfter := store(t, a), focus(a)
		var ops []any
		for _, k := range sortedKeys(after) {
			if !jsonEqual(before[k], after[k]) {
				ops = append(ops, map[string]any{"path": k, "value": after[k]})
			}
		}
		if c.focus && !jsonEqual(focusBefore, focusAfter) {
			ops = append(ops, map[string]any{"path": "@focus", "value": focusAfter})
		}
		if len(ops) == 0 {
			t.Fatalf("%s changed nothing: it has no visible effect to mock", c.action)
		}
		mock[c.action] = ops
	}
	return mock
}

// call runs the handler of an action, as the runtime does for a key.
func call(t *testing.T, a *app, action string) {
	t.Helper()
	h := a.handlers()[action]
	if h == nil {
		t.Fatalf("no handler for %s", action)
	}
	if err := h(tuimark.Event{Action: action}); err != nil {
		t.Fatalf("%s: %v", action, err)
	}
}

func jsonEqual(a, b any) bool {
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return bytes.Equal(ab, bb)
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// compact writes a JSON object with one member per line and every array
// element of objects on its own line, so the files diff and read well.
func compact(t *testing.T, v map[string]any) []byte {
	t.Helper()
	var b bytes.Buffer
	b.WriteString("{\n")
	keys := sortedKeys(v)
	for i, k := range keys {
		kb, _ := json.Marshal(k)
		b.Write([]byte("  " + string(kb) + ": "))
		switch x := v[k].(type) {
		case []any:
			if len(x) > 1 && isObjects(x) {
				b.WriteString("[\n")
				for j, e := range x {
					eb, _ := json.Marshal(e)
					b.WriteString("    " + string(eb))
					if j < len(x)-1 {
						b.WriteString(",")
					}
					b.WriteString("\n")
				}
				b.WriteString("  ]")
				break
			}
			vb, _ := json.Marshal(x)
			b.Write(vb)
		default:
			vb, _ := json.Marshal(x)
			b.Write(vb)
		}
		if i < len(keys)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("}\n")
	return b.Bytes()
}

func isObjects(x []any) bool {
	_, ok := x[0].(map[string]any)
	return ok
}

func checkFile(t *testing.T, name string, want map[string]any) {
	t.Helper()
	if os.Getenv("UPDATE_SAMPLE") != "" {
		if err := os.WriteFile(name, compact(t, want), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	raw, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	var got, norm map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	// Round-trip want through JSON so numbers compare as float64.
	wb, _ := json.Marshal(want)
	_ = json.Unmarshal(wb, &norm)
	if !reflect.DeepEqual(got, norm) {
		t.Errorf("%s differs from what the app builds; run UPDATE_SAMPLE=1 go test ./examples/pomodoro -run TestSampleInSync", name)
	}
}

func TestSampleInSync(t *testing.T) {
	checkFile(t, "sample.json", sampleData(t))
	checkFile(t, "mock.json", mockData(t))
}

// Every action that closes a modal writes its flag false in the mock, so
// `tuimark run --mock` can close what it opened; opening writes it true.
func TestMockClosesTheModal(t *testing.T) {
	mock := mockData(t)
	writes := func(action, path string) any {
		ops, _ := mock[action].([]any)
		for _, op := range ops {
			m := op.(map[string]any)
			if m["path"] == path {
				return m["value"]
			}
		}
		return nil
	}
	if got := writes("help_open", "help_open"); got != true {
		t.Errorf("help_open writes help_open = %v, want true", got)
	}
	if got := writes("help_close", "help_open"); got != false {
		t.Errorf("help_close writes help_open = %v, want false", got)
	}
	// Every modal of the document: whatever it closes with (on:escape or
	// on:close) writes the flag its open= reads as false.
	src, err := os.ReadFile("app.tui")
	if err != nil {
		t.Fatal(err)
	}
	modals := regexp.MustCompile(`<modal\b[^>]*>`).FindAllString(string(src), -1)
	if len(modals) == 0 {
		t.Fatal("no modal found in app.tui")
	}
	for _, m := range modals {
		flag := regexp.MustCompile(`\bopen="([^"]+)"`).FindStringSubmatch(m)
		if flag == nil {
			t.Fatalf("%s has no open= flag", m)
		}
		closers := regexp.MustCompile(`\bon:(?:escape|close)="([^"]+)"`).FindAllStringSubmatch(m, -1)
		if len(closers) == 0 {
			t.Fatalf("%s has no way to close", m)
		}
		for _, c := range closers {
			if got := writes(c[1], flag[1]); got != false {
				t.Errorf("%s writes %s = %v, want false", c[1], flag[1], got)
			}
		}
	}
	if got := writes("task_done", "@focus"); got != "#nav" {
		t.Errorf("task_done writes @focus = %v, want #nav", got)
	}
}

// Every action mock.json answers is an action of the document that the
// host (not the runtime) handles, as `tuimark run --mock` insists.
func TestMockActionsAreHostActions(t *testing.T) {
	a, _ := loadApp(t)
	host := map[string]bool{}
	for _, spec := range a.ui.Catalog() {
		if !spec.Builtin {
			host[spec.Name] = true
		}
	}
	for action := range mockData(t) {
		if !host[action] {
			t.Errorf("mock.json answers %q, which is not a host action of app.tui", action)
		}
	}
}
