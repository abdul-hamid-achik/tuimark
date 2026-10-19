package main

import (
	"strings"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

// testStart is the engine's first clock reading in every test.
var testStart = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

// rig is the app under test with a fake clock: advance moves it by whole
// seconds through the same tick the live loop runs.
type rig struct {
	t     *testing.T
	a     *app
	clock time.Time
}

func newRig(t *testing.T) *rig {
	t.Helper()
	a, err := load(".", defaultSeed, testStart)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.register(); err != nil {
		t.Fatal(err)
	}
	return &rig{t: t, a: a, clock: testStart}
}

func (r *rig) advance(secs int) {
	r.t.Helper()
	for i := 0; i < secs; i++ {
		r.clock = r.clock.Add(time.Second)
		if err := r.a.ui.Batch(func(b *tuimark.Batch) error { return r.a.tick(b, r.clock) }); err != nil {
			r.t.Fatal(err)
		}
	}
}

func (r *rig) play(cols, rows int, steps ...string) *tuimark.PlayResult {
	r.t.Helper()
	return r.playOpts(tuimark.PlayOptions{Cols: cols, Rows: rows}, steps...)
}

func (r *rig) playOpts(opts tuimark.PlayOptions, steps ...string) *tuimark.PlayResult {
	r.t.Helper()
	res, err := r.a.ui.Play(opts, steps...)
	if err != nil {
		r.t.Fatalf("play %v: %v", steps, err)
	}
	return res
}

func playStyles(cols, rows int) tuimark.PlayOptions {
	return tuimark.PlayOptions{Cols: cols, Rows: rows, Styles: true}
}

func (r *rig) get(path string) any {
	r.t.Helper()
	v, ok := r.a.ui.Get(path)
	if !ok {
		r.t.Fatalf("%s is missing from the store", path)
	}
	return v
}

func (r *rig) str(path string) string { s, _ := r.get(path).(string); return s }
func (r *rig) boolean(path string) bool {
	b, _ := r.get(path).(bool)
	return b
}
func (r *rig) num(path string) float64 { n, _ := r.get(path).(float64); return n }

func (r *rig) set(path string, v any) {
	r.t.Helper()
	if err := r.a.ui.Set(path, v); err != nil {
		r.t.Fatal(err)
	}
}

// idOf is the id of the container with a name.
func (r *rig) idOf(name string) string {
	r.t.Helper()
	for _, c := range r.a.sim.cs {
		if c.name == name {
			return c.id
		}
	}
	r.t.Fatalf("no container %q", name)
	return ""
}

func actions(res *tuimark.PlayResult) []string {
	var out []string
	for _, e := range res.Events {
		out = append(out, e.Action)
	}
	return out
}

func has(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func focusOf(res *tuimark.PlayResult) string {
	if res.Dump.Focus == nil {
		return ""
	}
	return *res.Dump.Focus
}

func gridOf(res *tuimark.PlayResult) string { return strings.Join(res.Dump.Grid, "\n") }

// rowNames are the names of the visible rows, in order.
func (r *rig) rowNames() []string {
	var out []string
	for _, row := range listAt(r.a.ui.Get, "rows") {
		out = append(out, field(row, "name"))
	}
	return out
}
