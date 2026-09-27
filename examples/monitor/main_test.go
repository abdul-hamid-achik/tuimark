package main

import (
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

// The host of SPEC v0.2b §17.4.4, headless: each handler is called as
// Run would call it, and the effect is read from Dump (the public API
// only; the host holds no layout code, so the document lays out what the
// handlers derive).

func loadMonitor(t *testing.T) (*monitor, map[string]tuimark.Handler) {
	t.Helper()
	m, err := load(".")
	if err != nil {
		t.Fatal(err)
	}
	return m, m.handlers()
}

func screen(t *testing.T, m *monitor, cols, rows int) string {
	t.Helper()
	d, err := m.ui.Dump(cols, rows)
	if err != nil {
		t.Fatal(err)
	}
	if !d.OK {
		t.Fatalf("dump errors: %v", d.Errors)
	}
	return strings.Join(d.Grid, "\n")
}

func fire(t *testing.T, hs map[string]tuimark.Handler, action string, ev tuimark.Event) {
	t.Helper()
	h, ok := hs[action]
	if !ok {
		t.Fatalf("no handler for %s", action)
	}
	ev.Action = action
	if ev.Keys == nil {
		ev.Keys = map[string]any{}
	}
	if err := h(ev); err != nil {
		t.Fatalf("%s: %v", action, err)
	}
}

// Every action the document names has a handler, except the built-ins.
func TestHandlersCoverCatalog(t *testing.T) {
	m, hs := loadMonitor(t)
	for _, a := range m.ui.Catalog() {
		if _, ok := hs[a.Name]; !ok && !a.Builtin {
			t.Errorf("no handler for %s (%v)", a.Name, a.Sources)
		}
	}
	if err := hs["quit"](tuimark.Event{Action: "quit"}); err != tuimark.ErrQuit {
		t.Errorf("quit returned %v", err)
	}
}

// sort_cpu twice reverses the order and moves the arrow; sort_mem sorts
// by memory.
func TestSort(t *testing.T) {
	m, hs := loadMonitor(t)
	fire(t, hs, "sort_cpu", tuimark.Event{})
	if s := screen(t, m, 100, 30); !strings.Contains(s, "CPU%▼") || !strings.Contains(strings.Split(s, "\n")[4], "WindowServer") {
		t.Errorf("first c:\n%s", s)
	}
	fire(t, hs, "sort_cpu", tuimark.Event{})
	s := screen(t, m, 100, 30)
	// Ascending; the table keeps its cursor row (WindowServer) in view.
	if !strings.Contains(s, "CPU%▲") || strings.Index(s, "Finder") > strings.Index(s, "WindowServer") || strings.Index(s, "zsh") > strings.Index(s, "Finder") {
		t.Errorf("second c:\n%s", s)
	}
	fire(t, hs, "sort_mem", tuimark.Event{})
	s = screen(t, m, 100, 30)
	if !strings.Contains(s, "MEM▼") || strings.Contains(s, "CPU%▲") || !strings.Contains(strings.Split(s, "\n")[4], "kernel_task") {
		t.Errorf("m:\n%s", s)
	}
}

// The filter narrows the list by name; cancel restores the query and the
// list and closes the row.
func TestFilter(t *testing.T) {
	m, hs := loadMonitor(t)
	fire(t, hs, "filter_open", tuimark.Event{})
	fire(t, hs, "filter", tuimark.Event{Source: "filter", Value: "FIRE"})
	s := screen(t, m, 100, 30)
	if !strings.Contains(s, "🦊 firefox") || strings.Contains(s, "WindowServer") {
		t.Errorf("filtered:\n%s", s)
	}
	fire(t, hs, "filter_cancel", tuimark.Event{})
	s = screen(t, m, 100, 30)
	if !strings.Contains(s, "WindowServer") || strings.Contains(s, "/ ") {
		t.Errorf("cancelled:\n%s", s)
	}
}

// kill_ask fills the confirmation from the marked rows (blocked ones in
// place), kill_force_ask adds the warning, and kill_confirm drops only the
// eligible processes.
func TestKill(t *testing.T) {
	m, hs := loadMonitor(t)
	fire(t, hs, "marks_changed", tuimark.Event{Value: []any{1.0, 4242.0}})
	fire(t, hs, "kill_force_ask", tuimark.Event{})
	s := screen(t, m, 80, 24)
	for _, want := range []string{"[BLOCKED ]       1 launchd protected", "[ELIGIBLE]    4242 微信", "SIGKILL cannot be caught"} {
		if !strings.Contains(s, want) {
			t.Errorf("confirmation lacks %q:\n%s", want, s)
		}
	}
	fire(t, hs, "kill_confirm", tuimark.Event{})
	s = screen(t, m, 80, 24)
	if strings.Contains(s, "kill processes") || strings.Contains(s, "微信") || !strings.Contains(s, "launchd") {
		t.Errorf("after confirm:\n%s", s)
	}
	// Without marks the cursor row is asked about; cancel closes.
	fire(t, hs, "marks_changed", tuimark.Event{Value: []any{}})
	fire(t, hs, "cursor_moved", tuimark.Event{Keys: map[string]any{"p": 9921.0}})
	fire(t, hs, "kill_ask", tuimark.Event{})
	if s := screen(t, m, 80, 24); !strings.Contains(s, "[ELIGIBLE]    9921 node") || strings.Contains(s, "SIGKILL") {
		t.Errorf("cursor row:\n%s", s)
	}
	fire(t, hs, "kill_cancel", tuimark.Event{})
	if s := screen(t, m, 80, 24); strings.Contains(s, "kill processes") {
		t.Errorf("after cancel:\n%s", s)
	}
}

// diagnose pins the detail to the cursor row; once that pid leaves the
// snapshot the detail says so.
func TestDetailVanishes(t *testing.T) {
	m, hs := loadMonitor(t)
	fire(t, hs, "cursor_moved", tuimark.Event{Keys: map[string]any{"p": 11020.0}})
	fire(t, hs, "diagnose", tuimark.Event{})
	if s := screen(t, m, 100, 30); !strings.Contains(s, "process detail · sleep · pid 11020") {
		t.Errorf("detail:\n%s", s)
	}
	for i := 0; i < 3; i++ {
		m.sample()
	}
	fire(t, hs, "detail_refresh", tuimark.Event{})
	if s := screen(t, m, 100, 30); !strings.Contains(s, "no longer present") {
		t.Errorf("vanished:\n%s", s)
	}
	fire(t, hs, "detail_close", tuimark.Event{})
	if s := screen(t, m, 100, 30); strings.Contains(s, "no longer present") {
		t.Errorf("closed:\n%s", s)
	}
}

// Settings cycle, mark dirty, switch the mouse, and save.
func TestSettings(t *testing.T) {
	m, hs := loadMonitor(t)
	if err := m.ui.Set("view", "settings"); err != nil {
		t.Fatal(err)
	}
	fire(t, hs, "setting_next", tuimark.Event{Keys: map[string]any{"s": "mouse"}})
	s := screen(t, m, 80, 24)
	if !strings.Contains(s, "mouse                   off") {
		t.Errorf("mouse off:\n%s", s)
	}
	fire(t, hs, "setting_prev", tuimark.Event{Keys: map[string]any{"s": "refresh"}})
	fire(t, hs, "settings_save", tuimark.Event{})
	s = screen(t, m, 80, 24)
	if !strings.Contains(s, "refresh interval        5s") || !strings.Contains(s, "saved: 2 changed") {
		t.Errorf("saved:\n%s", s)
	}
	if m.interval.String() != "5s" {
		t.Errorf("interval %v", m.interval)
	}
}

// pause switches the status; a sample while paused is not taken by the
// collector, and refresh takes one at once.
func TestPauseAndSample(t *testing.T) {
	m, hs := loadMonitor(t)
	fire(t, hs, "pause", tuimark.Event{})
	if s := screen(t, m, 100, 30); !strings.Contains(s, "‖ PAUSED") {
		t.Errorf("paused:\n%s", s)
	}
	fire(t, hs, "refresh", tuimark.Event{})
	fire(t, hs, "pause", tuimark.Event{})
	if s := screen(t, m, 100, 30); !strings.Contains(s, "● LIVE") || strings.Contains(s, "12:00:00") {
		t.Errorf("live after a refresh:\n%s", s)
	}
}
