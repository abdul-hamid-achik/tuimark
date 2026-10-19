package main

import (
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

// newApp loads the sample and installs the handlers and derived values, as
// run does.
func newApp(t *testing.T) *app {
	t.Helper()
	a, err := load(".")
	if err != nil {
		t.Fatal(err)
	}
	a.register()
	return a
}

// play replays steps against the app at 100x30.
func play(t *testing.T, a *app, steps ...string) *tuimark.PlayResult {
	t.Helper()
	return playAt(t, a, tuimark.PlayOptions{Cols: 100, Rows: 30}, steps...)
}

func playAt(t *testing.T, a *app, opts tuimark.PlayOptions, steps ...string) *tuimark.PlayResult {
	t.Helper()
	res, err := a.ui.Play(opts, steps...)
	if err != nil {
		t.Fatalf("play %v: %v", steps, err)
	}
	return res
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

func get(t *testing.T, a *app, path string) any {
	t.Helper()
	v, ok := a.ui.Get(path)
	if !ok {
		t.Fatalf("%s is missing from the store", path)
	}
	return v
}

func getStr(t *testing.T, a *app, path string) string {
	t.Helper()
	s, _ := get(t, a, path).(string)
	return s
}

func getBool(t *testing.T, a *app, path string) bool {
	t.Helper()
	b, _ := get(t, a, path).(bool)
	return b
}
