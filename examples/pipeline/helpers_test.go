package main

import (
	"strings"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

// testStart is the fake clock's first reading in the tests.
var testStart = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// rig is an app with a fake clock: advance moves it forward the way Every
// would, one tick per 250ms (one simulated second at the default speed).
type rig struct {
	t     *testing.T
	a     *app
	clock time.Time
}

func newRig(t *testing.T, so simOptions) *rig {
	t.Helper()
	a, err := load(".", so, testStart)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.register(); err != nil {
		t.Fatal(err)
	}
	return &rig{t: t, a: a, clock: testStart}
}

func (r *rig) tick(d time.Duration) {
	r.t.Helper()
	r.clock = r.clock.Add(d)
	if err := r.a.ui.Batch(func(b *tuimark.Batch) error { return r.a.tick(b, r.clock) }); err != nil {
		r.t.Fatal(err)
	}
}

// advance runs the simulation for sec simulated seconds (at speed 4).
func (r *rig) advance(sec int) {
	r.t.Helper()
	for i := 0; i < sec; i++ {
		r.tick(250 * time.Millisecond)
	}
}

// advanceUntil ticks until cond holds, at most max simulated seconds.
func (r *rig) advanceUntil(max int, cond func() bool) {
	r.t.Helper()
	for i := 0; i < max; i++ {
		if cond() {
			return
		}
		r.tick(250 * time.Millisecond)
	}
	if !cond() {
		r.t.Fatalf("condition still false after %d simulated seconds", max)
	}
}

func (r *rig) play(cols, rows int, keys ...string) *tuimark.PlayResult {
	r.t.Helper()
	res, err := r.a.ui.Play(tuimark.PlayOptions{Cols: cols, Rows: rows}, keys...)
	if err != nil {
		r.t.Fatal(err)
	}
	if res.Dump == nil {
		r.t.Fatalf("play %v at %dx%d: no dump", keys, cols, rows)
	}
	if d := res.Dump; !d.OK || len(d.Errors) != 0 {
		r.t.Fatalf("play %v at %dx%d: dump not clean: ok=%v errors=%v", keys, cols, rows, d.OK, d.Errors)
	}
	return res
}

func (r *rig) get(path string) any {
	r.t.Helper()
	v, _ := r.a.ui.Get(path)
	return v
}

func (r *rig) str(path string) string {
	s, _ := r.get(path).(string)
	return s
}

func (r *rig) flag(path string) bool {
	b, _ := r.get(path).(bool)
	return b
}

// status is a job's status in the simulation.
func (r *rig) status(id string) string { return r.a.sim.find(id).status }

func (r *rig) grid(cols, rows int) string {
	r.t.Helper()
	d, err := r.a.ui.Dump(cols, rows)
	if err != nil {
		r.t.Fatal(err)
	}
	if !d.OK || len(d.Errors) != 0 {
		r.t.Fatalf("dump at %dx%d: ok=%v errors=%v", cols, rows, d.OK, d.Errors)
	}
	return strings.Join(d.Grid, "\n")
}

// logLen is the number of lines the log pane shows.
func (r *rig) logLen() int {
	l, _ := r.get("log.lines").([]any)
	return len(l)
}
