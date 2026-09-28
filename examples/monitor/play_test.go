package main

import (
	"errors"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

// SPEC v0.3 §21 test 81: Play with handlers, on examples/monitor with
// its handlers (the same wiring main_test.go's loadMonitor uses).

func newRegisteredMonitor(t *testing.T) *monitor {
	t.Helper()
	m, err := load(".")
	if err != nil {
		t.Fatal(err)
	}
	m.register()
	return m
}

func TestPlayOpensAndClosesTheKillConfirmation(t *testing.T) {
	m := newRegisteredMonitor(t)
	opts := tuimark.PlayOptions{Cols: 80, Rows: 24}

	// "7","K" opens the kill confirmation through kill_ask: "7" switches
	// to the processes tab (focusing #procs), "K" (when="#procs:focus")
	// fires kill_ask.
	res, err := m.ui.Play(opts, "7", "K")
	if err != nil {
		t.Fatalf("7 K: %v", err)
	}
	if res.Dump.Focus == nil || *res.Dump.Focus != "kill" {
		t.Fatalf("kill_ask did not open the confirmation: focus=%v events=%+v", res.Dump.Focus, res.Events)
	}

	// "n" closes it.
	res, err = m.ui.Play(opts, "n")
	if err != nil {
		t.Fatalf("n: %v", err)
	}
	if res.Dump.Focus != nil && *res.Dump.Focus == "kill" {
		t.Fatalf("n did not close the confirmation")
	}
}

func TestPlayEnterAndSpaceFireNothingWithConfirmationOpen(t *testing.T) {
	m := newRegisteredMonitor(t)
	opts := tuimark.PlayOptions{Cols: 80, Rows: 24}

	if _, err := m.ui.Play(opts, "7", "K"); err != nil {
		t.Fatal(err)
	}
	// The confirmation's buttons are focusable="false" (SPEC v0.3 §30.4
	// item 15), so enter/space with it open fire nothing.
	res, err := m.ui.Play(opts, "enter", "space")
	if err != nil {
		t.Fatalf("enter space: %v", err)
	}
	if len(res.Events) != 0 {
		t.Errorf("enter/space with the confirmation open fired %+v, want none", res.Events)
	}
}

func TestPlayHandlerErrQuit(t *testing.T) {
	m := newRegisteredMonitor(t)
	opts := tuimark.PlayOptions{Cols: 80, Rows: 24}

	// ctrl+c fires the built-in quit action, whose handler (registered by
	// m.register(), same as main.go's) returns tuimark.ErrQuit. It ends
	// the call with Quit: true and skips the remaining step ("n").
	res, err := m.ui.Play(opts, "7", "K", "ctrl+c", "n")
	if err != nil {
		t.Fatalf("ctrl+c: %v", err)
	}
	if !res.Quit {
		t.Fatalf("ctrl+c did not quit: %+v", res)
	}
	for _, e := range res.Events {
		if e.Step == 4 {
			t.Fatalf("step 4 (n) ran after Quit: %+v", res.Events)
		}
	}
}

func TestPlayHandlerOtherError(t *testing.T) {
	m := newRegisteredMonitor(t)
	opts := tuimark.PlayOptions{Cols: 80, Rows: 24}

	boom := errors.New("boom")
	m.ui.On("kill_confirm", func(tuimark.Event) error { return boom })

	res, err := m.ui.Play(opts, "7", "K", "y")
	if !errors.Is(err, boom) {
		t.Fatalf("kill_confirm error = %v, want boom", err)
	}
	if res == nil || len(res.Events) == 0 {
		t.Fatalf("want the events so far, got %+v", res)
	}
}
