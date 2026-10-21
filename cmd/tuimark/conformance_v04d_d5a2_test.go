package main

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

// SPEC v0.4d §21 test 311 (the proposal's examples), through the CLI. The
// fixtures are testdata/v04d/t311. The 0.4.5 lines are frozen: they were
// recorded by tuimark 0.4.5 on legacy.tui, unscoped.tui, and
// legacy-card.tui, which CI cannot run.

const (
	t311Dir  = "../../testdata/v04d/t311/"
	t311Data = t311Dir + "data.json"
)

type t311Play struct {
	Events []struct {
		Step   int    `json:"step"`
		Action string `json:"action"`
		Source string `json:"source"`
		Value  any    `json:"value"`
	} `json:"events"`
	Scrollback *struct {
		Grid []string `json:"grid"`
	} `json:"scrollback"`
}

func t311Run(t *testing.T, args ...string) t311Play {
	t.Helper()
	code, out, errw := runCLI(append([]string{"play"}, append(args, "--format", "json")...)...)
	if code != 0 {
		t.Fatalf("play %v: exit %d\n%s%s", args, code, out, errw)
	}
	var p t311Play
	if err := json.Unmarshal([]byte(out), &p); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	return p
}

func t311Events(p t311Play) []string {
	var out []string
	for _, e := range p.Events {
		v, _ := json.Marshal(e.Value)
		out = append(out, strings.Join([]string{strconv.Itoa(e.Step), e.Action, e.Source, string(v)}, "|"))
	}
	return out
}

const t311Input = "y n enter esc set:approval.open=false esc text:h"

// SPEC 0.4d test 311: the document validates with no error against the
// data, and its approval works: the composer is disabled while it shows,
// the rows are scoped by the class the screen has meanwhile.
func TestD5a2Approval311(t *testing.T) {
	code, out, errw := runCLI("validate", t311Dir+"app.tui", "--data", t311Data)
	if code != 0 || strings.Contains(out, "error") {
		t.Fatalf("validate: %d %s%s", code, out, errw)
	}
	p := t311Run(t, t311Dir+"app.tui", "--data", t311Data, "--inline", "--cols", "40", "--rows", "12", "--input", t311Input)
	want := []string{
		`1|approve||null`, `2|deny||null`, `3|deny||null`, `4|deny||null`,
		`6|cancel|composer|""`, `7|typed|composer|"h"`,
	}
	if got := t311Events(p); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("events\n got  %q\n want %q", got, want)
	}
}

// SPEC 0.4d test 311: tuimark 0.4.5 records the same events with an input
// and a body text, without --inline; with the rows unscoped and the
// composer never disabled it records the typed text and no approve.
func TestD5a2Approval311Legacy(t *testing.T) {
	p := t311Run(t, t311Dir+"legacy.tui", "--data", t311Data, "--cols", "40", "--rows", "12", "--input", t311Input)
	want := []string{
		`1|approve||null`, `2|deny||null`, `3|deny||null`, `4|deny||null`,
		`6|cancel|composer|""`, `7|typed|composer|"h"`,
	}
	if got := t311Events(p); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("legacy events\n got  %q\n want %q", got, want)
	}
	// Frozen from tuimark 0.4.5.
	p = t311Run(t, t311Dir+"unscoped.tui", "--data", t311Data, "--cols", "40", "--rows", "12", "--input", t311Input)
	want = []string{
		`1|typed|composer|"y"`, `2|typed|composer|"yn"`, `3|send|composer|"yn"`,
		`4|deny|composer|"yn"`, `6|deny|composer|"yn"`, `7|typed|composer|"ynh"`,
	}
	if got := t311Events(p); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("unscoped events\n got  %q\n want %q", got, want)
	}
}

// SPEC 0.4d test 311: committing a card. The slot of the copy stays empty,
// and a wrapper without a slot prints the card with its content, the item
// written to a scratch path before and cleared after; the region stays as
// it was.
func TestD5a2CommitCard311(t *testing.T) {
	steps := t311Dir + "card-steps.ndjson"
	p := t311Run(t, t311Dir+"card.tui", "--data", t311Data, "--inline", "--cols", "40", "--rows", "12", "--script", steps)
	if p.Scrollback == nil {
		t.Fatal("no scrollback")
	}
	var rows []string
	for _, r := range p.Scrollback.Grid {
		rows = append(rows, strings.TrimRight(r, " "))
	}
	want := []string{"assistant", "assistant", "first", "second", "- old line", "+ new    line"}
	if strings.Join(rows, "\n") != strings.Join(want, "\n") {
		t.Errorf("scrollback\n got  %q\n want %q", rows, want)
	}
	// The wrapper renders the same rows with a body text in place of runs
	// under tuimark 0.4.5 (frozen): assistant, first, second.
	code, out, errw := runCLI("dump", t311Dir+"legacy-card.tui", "--data", t311Dir+"legacy-card.json", "--cols", "20", "--rows", "4")
	if code != 0 {
		t.Fatalf("dump: %d %s", code, errw)
	}
	for _, w := range []string{"assistant", "first", "second"} {
		if !strings.Contains(out, w) {
			t.Errorf("%q missing from\n%s", w, out)
		}
	}
}
