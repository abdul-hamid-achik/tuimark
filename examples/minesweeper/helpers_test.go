package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

var epoch = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

var beginner = board{"beginner", 9, 9, 10}

// rig is an app with a fake clock and the default seed.
type rig struct {
	t   *testing.T
	a   *app
	now time.Time
}

func newRigOn(t testing.TB, b board, seed int64) *rig {
	t.Helper()
	r := &rig{now: epoch}
	if tt, ok := t.(*testing.T); ok {
		r.t = tt
	}
	a, err := load(".", b, seed, func() time.Time { return r.now })
	if err != nil {
		t.Fatal(err)
	}
	r.a = a
	return r
}

func newRig(t *testing.T) *rig { t.Helper(); return newRigOn(t, beginner, defaultSeed) }

// advance moves the clock and runs the Every function.
func (r *rig) advance(d time.Duration) {
	r.t.Helper()
	r.now = r.now.Add(d)
	if err := r.a.ui.Batch(func(b *tuimark.Batch) error { return r.a.tick(b) }); err != nil {
		r.t.Fatal(err)
	}
}

// play sends steps through Play at 90x24, so the handlers run as in Run.
func (r *rig) play(steps ...string) *tuimark.PlayResult { return r.playAt(90, 24, steps...) }

func (r *rig) playAt(cols, rows int, steps ...string) *tuimark.PlayResult {
	r.t.Helper()
	res, err := r.a.ui.Play(tuimark.PlayOptions{Cols: cols, Rows: rows}, steps...)
	if err != nil {
		r.t.Fatal(err)
	}
	return res
}

// click is the Play step that clicks cell (row, col) of the board.
func (r *rig) click(row, col int) string {
	r.t.Helper()
	return clickStep(r.t, r.playAt(90, 24), row, col)
}

// clickStep finds cell (row, col) in a dump's nodes and names the click.
func clickStep(t testing.TB, res *tuimark.PlayResult, row, col int) string {
	t.Helper()
	id := cellID(row, col)
	for _, n := range res.Dump.Nodes {
		if n.ID == id {
			return fmt.Sprintf("click:%d,%d", n.X, n.Y)
		}
	}
	t.Fatalf("no node %s in the dump", id)
	return ""
}

func (r *rig) get(path string) any {
	r.t.Helper()
	v, ok := r.a.ui.Get(path)
	if !ok {
		r.t.Fatalf("Get(%q) found nothing", path)
	}
	return v
}

func (r *rig) str(path string) string { r.t.Helper(); s, _ := r.get(path).(string); return s }

func (r *rig) boolean(path string) bool { r.t.Helper(); b, _ := r.get(path).(bool); return b }

func (r *rig) number(path string) int {
	r.t.Helper()
	switch v := r.get(path).(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	r.t.Fatalf("%s is not a number: %v", path, r.get(path))
	return 0
}

func (r *rig) grid(cols, rows int) string {
	r.t.Helper()
	return strings.Join(r.playAt(cols, rows).Dump.Grid, "\n")
}

func gridOf(res *tuimark.PlayResult) string { return strings.Join(res.Dump.Grid, "\n") }

func actions(res *tuimark.PlayResult) []string {
	var out []string
	for _, e := range res.Events {
		out = append(out, e.Action)
	}
	return out
}

func wantActions(t testing.TB, res *tuimark.PlayResult, want ...string) {
	t.Helper()
	got := actions(res)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("actions = %v, want %v", got, want)
	}
}

func focusOf(res *tuimark.PlayResult) string {
	if res.Dump.Focus == nil {
		return ""
	}
	return *res.Dump.Focus
}

// on is a boolean in the store, false when the path is not there: a cell
// outside the grid is just {"on": false}.
func (r *rig) on(path string) bool {
	r.t.Helper()
	v, _ := r.a.ui.Get(path)
	b, _ := v.(bool)
	return b
}
