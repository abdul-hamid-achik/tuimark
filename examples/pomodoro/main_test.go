package main

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

// loadApp is the wiring main.go's run does before Run, with a fake clock at
// the sample's start: the app is loaded, registered, and published, with
// the timer stopped at 25:00.
func loadApp(t *testing.T) (*app, *fakeClock) {
	t.Helper()
	a, clock := newTestApp(t, sampleStart)
	if err := a.start(false); err != nil {
		t.Fatal(err)
	}
	return a, clock
}

func play(t *testing.T, a *app, cols, rows int, steps ...string) *tuimark.PlayResult {
	t.Helper()
	res, err := a.ui.Play(tuimark.PlayOptions{Cols: cols, Rows: rows}, steps...)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func gridOf(res *tuimark.PlayResult) string { return strings.Join(res.Dump.Grid, "\n") }

func getStr(t *testing.T, a *app, path string) string {
	t.Helper()
	v, ok := a.ui.Get(path)
	if !ok {
		t.Fatalf("no %s in the store", path)
	}
	s, _ := v.(string)
	return s
}

func getNum(t *testing.T, a *app, path string) float64 {
	t.Helper()
	v, ok := a.ui.Get(path)
	if !ok {
		t.Fatalf("no %s in the store", path)
	}
	f, _ := v.(float64)
	return f
}

func getBool(t *testing.T, a *app, path string) bool {
	t.Helper()
	v, ok := a.ui.Get(path)
	if !ok {
		t.Fatalf("no %s in the store", path)
	}
	b, _ := v.(bool)
	return b
}

func TestValidateIsClean(t *testing.T) {
	a, _ := loadApp(t)
	for _, theme := range []string{"dark", "light"} {
		if err := a.ui.Set("@theme", theme); err != nil {
			t.Fatal(err)
		}
		if diags := a.ui.Validate(); len(diags) != 0 {
			t.Errorf("%s: Validate gave %v", theme, diags)
		}
	}
}

// Both tabs, from the smallest supported screen up, in both themes, and in
// every look of the timer (running, paused, the last ten seconds with the
// pulse on and off, the done flash on and off, the long break, and the help
// modal on top of each), have no diagnostics at all: warnings count. An
// inactive tab is not laid out, so each tab is visited.
func TestNoDiagnosticsAnywhere(t *testing.T) {
	looks := []struct {
		name string
		do   func(t *testing.T, a *app, clock *fakeClock)
	}{
		{"running", func(*testing.T, *app, *fakeClock) {}},
		{"paused", func(t *testing.T, a *app, clock *fakeClock) { call(t, a, "toggle") }},
		{"last ten", func(t *testing.T, a *app, clock *fakeClock) {
			a.tm.left = 7 * time.Second
			advance(t, a, clock, clock.t)
		}},
		{"last ten, other half second", func(t *testing.T, a *app, clock *fakeClock) {
			a.tm.left = 7 * time.Second
			advance(t, a, clock, clock.t.Add(500*time.Millisecond))
		}},
		{"flash", func(t *testing.T, a *app, clock *fakeClock) {
			a.tm.left = time.Second
			advance(t, a, clock, clock.t.Add(time.Second))
			if !getBool(t, a, "is.flash") {
				t.Fatal("not in the flash")
			}
		}},
		{"flash, other half second", func(t *testing.T, a *app, clock *fakeClock) {
			a.tm.left = time.Second
			advance(t, a, clock, clock.t.Add(time.Second+500*time.Millisecond))
			if !getBool(t, a, "is.flash") {
				t.Fatal("not in the flash")
			}
		}},
		{"short break", func(t *testing.T, a *app, clock *fakeClock) { call(t, a, "skip") }},
		{"long break", func(t *testing.T, a *app, clock *fakeClock) {
			for i := a.tm.pos; i < seqLen-1; i++ {
				call(t, a, "skip")
			}
			if !getBool(t, a, "is.long") {
				t.Fatal("not in the long break")
			}
		}},
		{"no task, no history", func(t *testing.T, a *app, clock *fakeClock) {
			if err := a.ui.Set("task", ""); err != nil {
				t.Fatal(err)
			}
			a.tm.sessions, a.published = nil, -1
			advance(t, a, clock, clock.t)
		}},
	}
	sizes := [][2]int{{40, 14}, {44, 14}, {48, 15}, {50, 16}, {60, 20}, {80, 24}, {90, 24}, {120, 30}, {100, 40}}
	for _, theme := range []string{"dark", "light"} {
		for _, look := range looks {
			for _, tab := range []string{"pomo", "clock"} {
				for _, help := range []bool{false, true} {
					for _, sz := range sizes {
						a, clock := sampleApp(t)
						if err := a.ui.Set("@theme", theme); err != nil {
							t.Fatal(err)
						}
						look.do(t, a, clock)
						if err := a.ui.Set("tab", tab); err != nil {
							t.Fatal(err)
						}
						if err := a.ui.Set("help_open", help); err != nil {
							t.Fatal(err)
						}
						res, err := a.ui.Play(tuimark.PlayOptions{Cols: sz[0], Rows: sz[1]})
						if err != nil {
							t.Fatal(err)
						}
						if !res.Dump.OK || len(res.Dump.Errors) > 0 {
							t.Errorf("%s %s %s help=%v %dx%d: %v", theme, look.name, tab, help, sz[0], sz[1], res.Dump.Errors)
						}
					}
				}
			}
		}
	}
}

func TestDumpShowsTheTimer(t *testing.T) {
	a, _ := sampleApp(t)
	for _, sz := range [][2]int{{80, 24}, {120, 30}, {60, 20}} {
		res := play(t, a, sz[0], sz[1])
		g := gridOf(res)
		for _, want := range []string{"pomodoro", "RUNNING", "FOCUS", "write the spec", "completed", "space start/pause"} {
			if sz[0] < 80 && want == "space start/pause" {
				continue
			}
			if !strings.Contains(g, want) {
				t.Errorf("%dx%d: grid has no %q:\n%s", sz[0], sz[1], want, g)
			}
		}
	}
}

// Every action the document names has a handler, except the built-ins.
func TestHandlersCoverCatalog(t *testing.T) {
	a, _ := loadApp(t)
	for _, spec := range a.ui.Catalog() {
		if !spec.Builtin && !spec.Registered {
			t.Errorf("no handler for %s (%v)", spec.Name, spec.Sources)
		}
	}
}

func TestPlayStartAndPause(t *testing.T) {
	a, clock := loadApp(t)
	if getBool(t, a, "running") || getStr(t, a, "timer.label") != "25:00" {
		t.Fatal("the app should open stopped at 25:00")
	}
	if g := gridOf(play(t, a, 80, 24)); !strings.Contains(g, "READY") {
		t.Errorf("a fresh timer is not READY:\n%s", g)
	}
	res := play(t, a, 80, 24, "space")
	if len(res.Events) != 1 || res.Events[0].Action != "toggle" {
		t.Fatalf("events = %+v, want one toggle", res.Events)
	}
	if !getBool(t, a, "running") || !strings.Contains(gridOf(res), "RUNNING") {
		t.Errorf("space did not start the timer:\n%s", gridOf(res))
	}
	advance(t, a, clock, clock.t.Add(90*time.Second))
	if got := getStr(t, a, "timer.label"); got != "23:30" {
		t.Errorf("label = %s after 90s, want 23:30", got)
	}
	res = play(t, a, 80, 24, "space")
	if getBool(t, a, "running") || !strings.Contains(gridOf(res), "PAUSED") {
		t.Errorf("space did not pause:\n%s", gridOf(res))
	}
	if !getBool(t, a, "is.paused") {
		t.Error("is.paused is false while paused")
	}
	advance(t, a, clock, clock.t.Add(time.Hour))
	if got := getStr(t, a, "timer.label"); got != "23:30" {
		t.Errorf("a paused hour moved the label to %s", got)
	}
}

// A key settles the timer to now first: pausing 90s in pauses at 23:30
// even when no tick ran in between.
func TestKeysSettleTheClockFirst(t *testing.T) {
	a, clock := loadApp(t)
	play(t, a, 80, 24, "space")
	clock.t = clock.t.Add(90 * time.Second) // no tick
	play(t, a, 80, 24, "space")
	if got := getStr(t, a, "timer.label"); got != "23:30" {
		t.Errorf("label = %s, want 23:30", got)
	}
}

func TestPlayReset(t *testing.T) {
	a, clock := loadApp(t)
	play(t, a, 80, 24, "space")
	advance(t, a, clock, clock.t.Add(10*time.Minute))
	res := play(t, a, 80, 24, "r")
	if res.Events[0].Action != "reset" {
		t.Fatalf("events = %+v", res.Events)
	}
	if getBool(t, a, "running") || getStr(t, a, "timer.label") != "25:00" || getNum(t, a, "timer.progress") != 0 {
		t.Errorf("reset: running %v label %s", getBool(t, a, "running"), getStr(t, a, "timer.label"))
	}
}

func TestPlaySkipWalksTheCycle(t *testing.T) {
	a, _ := loadApp(t)
	wantTitle := []string{"SHORT BREAK", "FOCUS", "SHORT BREAK", "FOCUS", "SHORT BREAK", "FOCUS", "LONG BREAK", "FOCUS"}
	wantLabel := []string{"05:00", "25:00", "05:00", "25:00", "05:00", "25:00", "15:00", "25:00"}
	for i := range wantTitle {
		res := play(t, a, 80, 24, "s")
		if res.Events[0].Action != "skip" {
			t.Fatalf("events = %+v", res.Events)
		}
		if got := getStr(t, a, "timer.title"); got != wantTitle[i] {
			t.Errorf("skip %d: title %q, want %q", i+1, got, wantTitle[i])
		}
		if got := getStr(t, a, "timer.label"); got != wantLabel[i] {
			t.Errorf("skip %d: label %q, want %q", i+1, got, wantLabel[i])
		}
	}
	if n := len(a.tm.sessions); n != 0 {
		t.Errorf("skipping recorded %d sessions", n)
	}
}

func TestPlayAddAndRemoveAMinute(t *testing.T) {
	a, _ := loadApp(t)
	play(t, a, 80, 24, "+", "+")
	if got := getStr(t, a, "timer.label"); got != "27:00" {
		t.Errorf("label = %s after ++, want 27:00", got)
	}
	res := play(t, a, 80, 24, "-")
	if got := getStr(t, a, "timer.label"); got != "26:00" {
		t.Errorf("label = %s after -, want 26:00", got)
	}
	if !strings.Contains(gridOf(res), "██") {
		t.Error("no digits in the grid")
	}
	if got := getNum(t, a, "total_ms"); got != 26*60*1000 {
		t.Errorf("total_ms = %v, want the phase length to follow", got)
	}
	// r puts the planned length back.
	play(t, a, 80, 24, "r")
	if got := getStr(t, a, "timer.label"); got != "25:00" {
		t.Errorf("label = %s after reset, want 25:00", got)
	}
}

func TestPlayTabs(t *testing.T) {
	a, _ := sampleApp(t)
	steps := []struct {
		keys []string
		tab  string
	}{
		{[]string{"2"}, "clock"},
		{[]string{"1"}, "pomo"},
		{[]string{"]"}, "clock"},
		{[]string{"["}, "pomo"},
		{[]string{"]", "]"}, "pomo"}, // wraps around
	}
	for _, s := range steps {
		res := play(t, a, 90, 24, s.keys...)
		if got := getStr(t, a, "tab"); got != s.tab {
			t.Errorf("after %v: tab = %s, want %s", s.keys, got, s.tab)
		}
		g := gridOf(res)
		if (s.tab == "clock") != strings.Contains(g, "world clocks") {
			t.Errorf("after %v the grid does not show the %s tab:\n%s", s.keys, s.tab, g)
		}
	}
	res := play(t, a, 90, 24, "2")
	g := gridOf(res)
	for _, want := range []string{"Saturday, 17 October 2026", "Tokyo", "UTC+5:30", "00:34"} {
		if !strings.Contains(g, want) {
			t.Errorf("clock tab has no %q:\n%s", want, g)
		}
	}
}

func TestPlayTaskInput(t *testing.T) {
	a, clock := sampleApp(t)
	if err := a.ui.Set("task", ""); err != nil {
		t.Fatal(err)
	}
	// From the Clock tab, t brings the task input into view and focuses it.
	play(t, a, 90, 24, "2")
	play(t, a, 90, 24, "t")
	if got := getStr(t, a, "tab"); got != "pomo" {
		t.Errorf("t left the tab at %s, want pomo", got)
	}
	if f, _ := a.ui.Get("@focus"); f != "#task" {
		t.Fatalf("focus = %v, want #task", f)
	}
	// While typing, the keys are text: q, r, s, +, 1, space do not act.
	res := play(t, a, 90, 24, "text:qrs+1 a b")
	if len(res.Events) != 0 {
		t.Errorf("typing fired %+v", res.Events)
	}
	if got := getStr(t, a, "task"); got != "qrs+1 a b" {
		t.Errorf("task = %q", got)
	}
	// enter keeps the name and leaves the input.
	res = play(t, a, 90, 24, "backspace", "backspace", "backspace", "backspace", "backspace", "backspace", "backspace", "backspace", "backspace", "text:write docs", "enter")
	if got := getStr(t, a, "task"); got != "write docs" {
		t.Errorf("task = %q after enter, want write docs", got)
	}
	if f, _ := a.ui.Get("@focus"); f != "#nav" {
		t.Errorf("focus = %v after enter, want #nav", f)
	}
	if res.Events[len(res.Events)-1].Action != "task_done" {
		t.Errorf("events = %+v, want a task_done last", res.Events)
	}
	// esc leaves it too, and keeps what was typed.
	play(t, a, 90, 24, "t", "text:!", "esc")
	if f, _ := a.ui.Get("@focus"); f != "#nav" {
		t.Errorf("focus = %v after esc, want #nav", f)
	}
	if got := getStr(t, a, "task"); got != "write docs!" {
		t.Errorf("task = %q after esc", got)
	}
	// And space works again: the task name goes on the session that ends.
	play(t, a, 90, 24, "space")
	if getBool(t, a, "running") {
		t.Fatal("the timer was running in the sample; space should have paused it")
	}
	play(t, a, 90, 24, "space")
	advance(t, a, clock, clock.t.Add(21*time.Minute))
	rows, _ := a.ui.Get("sessions")
	list := rows.([]any)
	top := list[0].(map[string]any)
	if top["kind"] != "work" || top["task"] != "write docs!" {
		t.Errorf("newest session = %v, want the work phase named write docs!", top)
	}
}

func TestPlayHelpAndQuit(t *testing.T) {
	a, _ := loadApp(t)
	res := play(t, a, 90, 24, "?")
	if !getBool(t, a, "help_open") || !strings.Contains(gridOf(res), "pomodoro keys") {
		t.Errorf("? did not open the help:\n%s", gridOf(res))
	}
	// Under the modal the timer's keys do not fire.
	res = play(t, a, 90, 24, "r")
	for _, e := range res.Events {
		if e.Action == "reset" {
			t.Error("r fired a reset under the help modal")
		}
	}
	res = play(t, a, 90, 24, "esc")
	if getBool(t, a, "help_open") || strings.Contains(gridOf(res), "pomodoro keys") {
		t.Error("esc did not close the help")
	}
	if res = play(t, a, 90, 24, "?", "q"); getBool(t, a, "help_open") || res.Quit {
		t.Errorf("q in the help should close it, not quit (open=%v quit=%v)", getBool(t, a, "help_open"), res.Quit)
	}
	if res = play(t, a, 90, 24, "q"); !res.Quit {
		t.Error("q did not quit")
	}
	if res = play(t, a, 90, 24, "ctrl+c"); !res.Quit {
		t.Error("ctrl+c did not quit")
	}
}

// The colon blinks each second and the last ten seconds pulse on alternate
// half-seconds; both come from the published instant.
func TestBlinkAndPulse(t *testing.T) {
	a, clock := loadApp(t)
	play(t, a, 80, 24, "space")
	// Far from the end: the colon blinks, nothing pulses.
	start := clock.t
	for i := 0; i < 8; i++ {
		advance(t, a, clock, start.Add(time.Duration(i)*500*time.Millisecond))
		if getBool(t, a, "is.pulse") || getBool(t, a, "is.last10") {
			t.Fatal("pulse in the middle of a phase")
		}
	}
	// Seven seconds to go.
	a.tm.left = 7 * time.Second
	base := clock.t
	var pulses, colonOn int
	prev := false
	for i := 0; i < 8; i++ {
		advance(t, a, clock, base.Add(time.Duration(i)*500*time.Millisecond))
		if !getBool(t, a, "is.last10") {
			t.Fatalf("step %d: is.last10 is false at %s", i, getStr(t, a, "timer.label"))
		}
		p := getBool(t, a, "is.pulse")
		if i > 0 && p == prev {
			t.Errorf("step %d: pulse did not alternate", i)
		}
		prev = p
		if p {
			pulses++
		}
		if colonOnFor(getStr(t, a, "timer.big")) {
			colonOn++
		}
	}
	if pulses != 4 {
		t.Errorf("%d pulses in 8 half-seconds, want 4", pulses)
	}
	if colonOn != 4 {
		t.Errorf("the colon was on %d times in 8 half-seconds, want 4", colonOn)
	}
	// Paused, the colon stays on and nothing pulses.
	play(t, a, 80, 24, "space")
	for i := 0; i < 4; i++ {
		advance(t, a, clock, clock.t.Add(500*time.Millisecond))
		if getBool(t, a, "is.pulse") || !colonOnFor(getStr(t, a, "timer.big")) {
			t.Fatal("a paused timer blinked or pulsed")
		}
	}
}

// colonOnFor reports whether row 1 of a "MM:SS" string has ink in the colon
// columns: after two glyphs and their two gaps.
func colonOnFor(big string) bool {
	row := []rune(strings.Split(big, "\n")[1])
	at := 2 * (fontWidth + len(glyphGap))
	return string(row[at:at+colonWidth]) == "██"
}

func TestPhaseClasses(t *testing.T) {
	a, clock := sampleApp(t)
	classesOf := func() []string {
		res := play(t, a, 90, 24)
		for _, n := range res.Dump.Nodes {
			if n.ID == "tone" {
				return n.Classes
			}
		}
		t.Fatal("no node tone")
		return nil
	}
	has := func(cs []string, name string) bool {
		for _, c := range cs {
			if c == name {
				return true
			}
		}
		return false
	}
	if cs := classesOf(); !has(cs, "work") || has(cs, "paused") || has(cs, "short") {
		t.Errorf("a running work phase has classes %v", cs)
	}
	play(t, a, 90, 24, "space")
	if cs := classesOf(); !has(cs, "work") || !has(cs, "paused") {
		t.Errorf("a paused work phase has classes %v", cs)
	}
	play(t, a, 90, 24, "space", "s")
	if cs := classesOf(); !has(cs, "short") || has(cs, "work") || has(cs, "paused") {
		t.Errorf("a running break has classes %v", cs)
	}
	a.tm.left = 4 * time.Second
	advance(t, a, clock, clock.t.Add(500*time.Millisecond))
	advance(t, a, clock, clock.t.Add(500*time.Millisecond))
	cs := classesOf()
	if !has(cs, "short") || (!has(cs, "pulse")) == getBool(t, a, "is.pulse") {
		t.Errorf("the last seconds have classes %v (is.pulse %v)", cs, getBool(t, a, "is.pulse"))
	}
}

// The colors of the digits are the palette's, in both themes.
func TestColorsPerPhase(t *testing.T) {
	type look struct{ theme, phase, fg string }
	want := []look{
		{"dark", kindWork, "#ff6b5b"}, {"dark", kindShort, "#5ec27a"}, {"dark", kindLong, "#4fc3c7"},
		{"light", kindWork, "#c4301f"}, {"light", kindShort, "#1f7a3a"}, {"light", kindLong, "#0f7c80"},
	}
	for _, w := range want {
		a, _ := loadApp(t)
		if err := a.ui.Set("@theme", w.theme); err != nil {
			t.Fatal(err)
		}
		switch w.phase {
		case kindShort:
			play(t, a, 90, 24, "s")
		case kindLong:
			play(t, a, 90, 24, "s", "s", "s", "s", "s", "s", "s")
		}
		res, err := a.ui.Play(tuimark.PlayOptions{Cols: 90, Rows: 24, Styles: true})
		if err != nil {
			t.Fatal(err)
		}
		// The digits are on rows 3-7, centred; find an inked cell.
		var got string
		for y := 3; y < 8 && got == ""; y++ {
			for _, sp := range res.Dump.Styles[y] {
				if strings.ContainsRune(string([]rune(res.Dump.Grid[y])[sp.X:sp.X+sp.W]), '█') {
					got = sp.FG
					break
				}
			}
		}
		if got != w.fg {
			t.Errorf("%s %s: digits are %q, want %s", w.theme, w.phase, got, w.fg)
		}
	}
}

// A paused timer is muted.
func TestPausedIsMuted(t *testing.T) {
	a, clock := loadApp(t)
	play(t, a, 90, 24, "space")
	advance(t, a, clock, clock.t.Add(time.Minute))
	play(t, a, 90, 24, "space")
	res, err := a.ui.Play(tuimark.PlayOptions{Cols: 90, Rows: 24, Styles: true})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for y := 3; y < 8; y++ {
		for _, sp := range res.Dump.Styles[y] {
			if sp.FG == "#7f8794" {
				found = true
			}
		}
	}
	if !found {
		t.Error("the paused digits are not muted")
	}
}

// The done flash at the end of a phase: the digits reverse on alternate
// half-seconds, the title says DONE, and then the next phase starts.
func TestDoneFlash(t *testing.T) {
	a, clock := sampleApp(t)
	a.tm.left = 1500 * time.Millisecond
	advance(t, a, clock, clock.t) // publish it
	var flashOn []bool
	for i := 0; i < 3; i++ {
		advance(t, a, clock, clock.t.Add(500*time.Millisecond))
	}
	// left reached zero at the third half-second: the flash starts now.
	if !getBool(t, a, "is.flash") || getStr(t, a, "timer.label") != "00:00" {
		t.Fatalf("flash=%v label=%s", getBool(t, a, "is.flash"), getStr(t, a, "timer.label"))
	}
	if g := gridOf(play(t, a, 90, 24)); !strings.Contains(g, "DONE") {
		t.Errorf("no DONE in the flash:\n%s", g)
	}
	for i := 0; i < 3; i++ {
		flashOn = append(flashOn, getBool(t, a, "is.flash_on"))
		advance(t, a, clock, clock.t.Add(500*time.Millisecond))
	}
	if flashOn[0] == flashOn[1] || flashOn[1] == flashOn[2] {
		t.Errorf("the flash does not alternate: %v", flashOn)
	}
	if getBool(t, a, "is.flash") || getStr(t, a, "timer.title") != "SHORT BREAK" || getStr(t, a, "timer.label") != "05:00" {
		t.Errorf("after the flash: flash=%v title=%s label=%s", getBool(t, a, "is.flash"), getStr(t, a, "timer.title"), getStr(t, a, "timer.label"))
	}
	if !getBool(t, a, "running") {
		t.Error("the break did not start by itself")
	}
}

func TestOptionsValidate(t *testing.T) {
	ok := options{plan: defaultPlan, flash: defaultFlash, tick: defaultTick}
	if err := ok.validate(); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []func(o *options){
		func(o *options) { o.plan.work = 0 },
		func(o *options) { o.plan.short = 500 * time.Millisecond },
		func(o *options) { o.plan.long = 100 * time.Minute },
		func(o *options) { o.flash = -1 },
		func(o *options) { o.tick = 0 },
		func(o *options) { o.theme = "blue" },
	} {
		o := ok
		bad(&o)
		if o.validate() == nil {
			t.Errorf("%+v should not validate", o)
		}
	}
}

func TestClockForNow(t *testing.T) {
	loc, now, err := clockFor("2026-10-17T14:34:07Z")
	if err != nil || loc != time.UTC {
		t.Fatal(loc, err)
	}
	if n := now(); n.Before(sampleNow) || n.After(sampleNow.Add(5*time.Second)) {
		t.Errorf("now = %v", n)
	}
	if _, _, err := clockFor("yesterday"); err == nil {
		t.Error("a bad -now should be an error")
	}
}

func TestParseSize(t *testing.T) {
	if c, r, err := parseSize("-dump", "90x24"); err != nil || c != 90 || r != 24 {
		t.Errorf("parseSize = %d %d %v", c, r, err)
	}
	for _, bad := range []string{"", "90", "x24", "0x24", "ax b"} {
		if _, _, err := parseSize("-dump", bad); err == nil {
			t.Errorf("parseSize(%q) should fail", bad)
		}
	}
}

// worldColumns is the header text and width of each visible column of the
// world-clock table at cols x 24 on the Clock tab.
func worldColumns(t *testing.T, a *app, cols int) map[string]int {
	t.Helper()
	if err := a.ui.Set("tab", "clock"); err != nil {
		t.Fatal(err)
	}
	res := play(t, a, cols, 24)
	if len(res.Dump.Errors) > 0 {
		t.Fatalf("%d cols: %v", cols, res.Dump.Errors)
	}
	got := map[string]int{}
	for _, n := range res.Dump.Nodes {
		if n.Tag == "column" {
			got[n.Text] = n.W
		}
	}
	return got
}

// The City column keeps a floor (.city min-width), so the table's
// priority columns drop when the terminal is narrow instead of City being
// squeezed: Offset (priority 1) first, then Day (priority 2). Each comes
// back when the table widens.
func TestWorldColumnsDropOnNarrowScreens(t *testing.T) {
	a, _ := sampleApp(t)
	has := func(cols map[string]int, names ...string) bool {
		for _, n := range names {
			if _, ok := cols[n]; !ok {
				return false
			}
		}
		return true
	}
	for _, c := range []struct {
		cols       int
		shown, not []string
	}{
		{37, []string{"City", "Time"}, []string{"Offset", "Day"}},
		{38, []string{"City", "Time", "Day"}, []string{"Offset"}},
		{40, []string{"City", "Time", "Day"}, []string{"Offset"}},
		{47, []string{"City", "Time", "Day"}, []string{"Offset"}},
		{48, []string{"City", "Offset", "Time", "Day"}, nil},
		{50, []string{"City", "Offset", "Time", "Day"}, nil},
		{80, []string{"City", "Offset", "Time", "Day"}, nil},
		{120, []string{"City", "Offset", "Time", "Day"}, nil},
	} {
		got := worldColumns(t, a, c.cols)
		if !has(got, c.shown...) {
			t.Errorf("%d cols: columns %v, want %v shown", c.cols, got, c.shown)
		}
		for _, n := range c.not {
			if _, ok := got[n]; ok {
				t.Errorf("%d cols: column %s still shown (%v)", c.cols, n, got)
			}
		}
		if got["City"] < 24 {
			t.Errorf("%d cols: City is %d wide, want the 24-column floor", c.cols, got["City"])
		}
	}
}

// The help modal's close button is on screen from 40x14 up.
func TestHelpCloseButtonFits(t *testing.T) {
	for _, sz := range [][2]int{{40, 14}, {44, 14}, {60, 14}, {120, 14}, {40, 15}, {40, 24}} {
		a, _ := loadApp(t)
		res := play(t, a, sz[0], sz[1], "?")
		var ok, modal *tuimark.DumpNode
		for i := range res.Dump.Nodes {
			switch res.Dump.Nodes[i].ID {
			case "help-ok":
				ok = &res.Dump.Nodes[i]
			case "help":
				modal = &res.Dump.Nodes[i]
			}
		}
		if ok == nil || modal == nil {
			t.Fatalf("%dx%d: no help modal in the dump", sz[0], sz[1])
		}
		if ok.H < 1 || ok.Y < modal.Y || ok.Y+ok.H > modal.Y+modal.H || ok.Y+ok.H > sz[1] {
			t.Errorf("%dx%d: #help-ok at y=%d h=%d is outside the modal (y=%d h=%d)", sz[0], sz[1], ok.Y, ok.H, modal.Y, modal.H)
		}
		if len(res.Dump.Errors) > 0 {
			t.Errorf("%dx%d: %v", sz[0], sz[1], res.Dump.Errors)
		}
	}
}

// s and r pressed during the done flash first finish it, so they act on the
// phase that follows and the cycle dots never go backwards.
func TestKeysDuringTheFlashKeepTheDots(t *testing.T) {
	for _, key := range []string{"r", "s"} {
		a, clock := sampleApp(t)
		a.tm.left = time.Second
		advance(t, a, clock, clock.t.Add(time.Second))
		if !getBool(t, a, "is.flash") {
			t.Fatal("not in the flash")
		}
		dots := getStr(t, a, "timer.dots") // work 2 just ended: ● ● ○ ○
		if dots != "● ● ○ ○" {
			t.Fatalf("dots in the flash = %q", dots)
		}
		play(t, a, 90, 24, key)
		if getBool(t, a, "is.flash") {
			t.Errorf("%s left the flash running", key)
		}
		after := getStr(t, a, "timer.dots")
		if strings.Count(after, "●") < strings.Count(dots, "●") {
			t.Errorf("%s moved the dots back: %q -> %q", key, dots, after)
		}
		if len(a.tm.sessions) != 3 {
			t.Errorf("%s: %d sessions, want the 3 finished ones", key, len(a.tm.sessions))
		}
	}
}

// An error shown in the top row goes away with the next tick or action that
// works.
func TestNoticeClearsOnTheNextSuccess(t *testing.T) {
	for _, how := range []string{"tick", "toggle", "help_open", "task_done"} {
		a, clock := loadApp(t)
		_ = a.onError(nil, errors.New("boom"))
		if got := getStr(t, a, "notice"); got != "boom" {
			t.Fatalf("notice = %q, want boom", got)
		}
		switch how {
		case "tick":
			advance(t, a, clock, clock.t.Add(100*time.Millisecond))
		default:
			call(t, a, how)
		}
		if got := getStr(t, a, "notice"); got != "" {
			t.Errorf("notice = %q after %s, want it cleared", got, how)
		}
		if g := gridOf(play(t, a, 90, 24)); strings.Contains(g, "boom") {
			t.Errorf("%s: the grid still shows the notice:\n%s", how, g)
		}
	}
}
