package main

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

// loadApp is the wiring main.go's run does before Run: load, then register.
func loadApp(t *testing.T) *app {
	t.Helper()
	a, err := load(".")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.register(); err != nil {
		t.Fatal(err)
	}
	return a
}

func grid(t *testing.T, a *app, cols, rows int) string {
	t.Helper()
	d, err := a.ui.Dump(cols, rows)
	if err != nil {
		t.Fatal(err)
	}
	if !d.OK {
		t.Fatalf("dump errors: %v", d.Errors)
	}
	return strings.Join(d.Grid, "\n")
}

func TestValidateIsClean(t *testing.T) {
	a := loadApp(t)
	for _, theme := range []string{"dark", "light"} {
		if err := a.ui.Set("@theme", theme); err != nil {
			t.Fatal(err)
		}
		if diags := a.ui.Validate(); len(diags) != 0 {
			t.Errorf("%s: Validate gave %v", theme, diags)
		}
	}
}

func TestDumpShowsTheLog(t *testing.T) {
	a := loadApp(t)
	for _, sz := range [][2]int{{80, 24}, {120, 30}, {100, 30}, {40, 14}} {
		g := grid(t, a, sz[0], sz[1])
		for _, want := range []string{"logs", "LIVE", "error", "lines/s", "of 48 lines", "q quit"} {
			if sz[0] < 60 && (want == "lines/s" || want == "q quit") {
				continue // the narrow layout drops the rate and most hints
			}
			if !strings.Contains(g, want) {
				t.Errorf("%dx%d: grid has no %q:\n%s", sz[0], sz[1], want, g)
			}
		}
	}
}

// Every action the document names has a handler, except the built-ins.
func TestHandlersCoverCatalog(t *testing.T) {
	a := loadApp(t)
	for _, spec := range a.ui.Catalog() {
		if !spec.Builtin && !spec.Registered {
			t.Errorf("no handler for %s (%v)", spec.Name, spec.Sources)
		}
	}
}

func TestPlayPauseAndQuit(t *testing.T) {
	a := loadApp(t)
	opts := tuimark.PlayOptions{Cols: 80, Rows: 24}
	res, err := a.ui.Play(opts, "p")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Events) != 1 || res.Events[0].Action != "pause" {
		t.Fatalf("events = %+v, want one pause", res.Events)
	}
	if !a.paused {
		t.Error("the host did not pause")
	}
	if v, _ := a.ui.Get("state_label"); v != "\u2016 PAUSED" {
		t.Errorf("state_label = %v, want PAUSED", v)
	}
	if g := strings.Join(res.Dump.Grid, "\n"); !strings.Contains(g, "PAUSED") {
		t.Errorf("the status line does not say PAUSED:\n%s", g)
	}
	// While paused a tick adds nothing.
	before := len(a.buf)
	if err := a.ui.Batch(a.tick); err != nil {
		t.Fatal(err)
	}
	if len(a.buf) != before {
		t.Errorf("a paused tick added %d lines", len(a.buf)-before)
	}
	// p again resumes; ctrl+c quits through the quit handler.
	res, err = a.ui.Play(opts, "p", "ctrl+c", "p")
	if err != nil {
		t.Fatal(err)
	}
	if a.paused || !res.Quit {
		t.Errorf("paused=%v quit=%v, want resumed and quit", a.paused, res.Quit)
	}
}

func TestPlayLevelTogglesFilterTheLog(t *testing.T) {
	a := loadApp(t)
	opts := tuimark.PlayOptions{Cols: 100, Rows: 30}
	res, err := a.ui.Play(opts, "1", "4")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Events) != 2 || res.Events[0].Action != "toggle_error" || res.Events[1].Action != "toggle_debug" {
		t.Fatalf("events = %+v", res.Events)
	}
	vis, _ := a.ui.Get("visible")
	for _, it := range vis.([]any) {
		l := it.(map[string]any)
		if l["level"] == lvlError || l["level"] == lvlDebug {
			t.Fatalf("a %v line is still visible: %v", l["level"], l)
		}
	}
	g := strings.Join(res.Dump.Grid, "\n")
	if strings.Contains(g, "ERROR") || strings.Contains(g, "DEBUG") {
		t.Errorf("the grid still shows hidden levels:\n%s", g)
	}
	if !strings.Contains(g, "○ error") || !strings.Contains(g, "● warn") {
		t.Errorf("the level chips do not show the toggles:\n%s", g)
	}
	// The counts do not follow the toggles; the shown count does.
	st, _ := a.ui.Get("stats")
	m := st.(map[string]any)
	if m["error"] != float64(6) || m["shown"] == m["total"] {
		t.Errorf("stats = %v", m)
	}
}

func TestPlayFilterBoxNarrowsTheLog(t *testing.T) {
	a := loadApp(t)
	opts := tuimark.PlayOptions{Cols: 100, Rows: 30}
	// "/" focuses the filter; typing sets the query; enter hands focus back.
	res, err := a.ui.Play(opts, "/", "text:billing -retry", "enter")
	if err != nil {
		t.Fatal(err)
	}
	var actions []string
	for _, e := range res.Events {
		actions = append(actions, e.Action)
	}
	if strings.Join(actions, ",") != "filter_apply" {
		t.Errorf("actions = %v, want filter_apply", actions)
	}
	if res.Dump.Focus == nil || *res.Dump.Focus != "log" {
		t.Errorf("focus = %v, want log", res.Dump.Focus)
	}
	g := strings.Join(res.Dump.Grid, "\n")
	if !strings.Contains(g, "card_declined") || strings.Contains(g, "upstream timeout") {
		t.Errorf("filter did not narrow the log:\n%s", g)
	}
	if !strings.Contains(g, " of 48 lines") {
		t.Errorf("the title has no counts:\n%s", g)
	}
	// esc inside the box clears it and returns to the log.
	res, err = a.ui.Play(opts, "/", "esc")
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := a.ui.Get("filter"); v != "" {
		t.Errorf("filter = %q after esc", v)
	}
	if !strings.Contains(strings.Join(res.Dump.Grid, "\n"), "48 of 48 lines") {
		t.Errorf("clearing the filter did not restore the log")
	}
}

func TestPlayClearAndHelp(t *testing.T) {
	a := loadApp(t)
	opts := tuimark.PlayOptions{Cols: 80, Rows: 24}
	res, err := a.ui.Play(opts, "c")
	if err != nil {
		t.Fatal(err)
	}
	if len(a.buf) != 0 {
		t.Errorf("clear left %d lines", len(a.buf))
	}
	if g := strings.Join(res.Dump.Grid, "\n"); !strings.Contains(g, "no lines to show") || !strings.Contains(g, "0 of 0 lines") {
		t.Errorf("empty state missing:\n%s", g)
	}
	res, err = a.ui.Play(opts, "?")
	if err != nil {
		t.Fatal(err)
	}
	if f := deref(res.Dump.Focus); f != "help-ok" {
		t.Fatalf("focus = %v, want the help modal's button", f)
	}
	if g := strings.Join(res.Dump.Grid, "\n"); !strings.Contains(g, "keys") {
		t.Errorf("no help modal:\n%s", g)
	}
	res, err = a.ui.Play(opts, "esc")
	if err != nil {
		t.Fatal(err)
	}
	if deref(res.Dump.Focus) == "help-ok" {
		t.Error("esc did not close the help")
	}
}

func TestTickAppendsAndTrims(t *testing.T) {
	a := loadApp(t)
	a.cap = 60
	before := a.nextID
	for i := 0; i < 20; i++ {
		if err := a.ui.Batch(a.tick); err != nil {
			t.Fatal(err)
		}
	}
	if a.nextID <= before {
		t.Fatal("ticks added no lines")
	}
	if len(a.buf) > 60 {
		t.Errorf("buffer holds %d lines, cap is 60", len(a.buf))
	}
	if a.buf[0].m["cont"] == true {
		t.Error("the trimmed buffer starts on a stack frame")
	}
	lines, _ := a.ui.Get("lines")
	if n := len(lines.([]any)); n != len(a.buf) {
		t.Errorf("store has %d lines, host has %d", n, len(a.buf))
	}
	st, _ := a.ui.Get("stats")
	if st.(map[string]any)["total"] != float64(len(a.buf)) {
		t.Errorf("stats total = %v, want %d (Derive must follow a tick)", st.(map[string]any)["total"], len(a.buf))
	}
	// The ring buffer keeps the newest lines.
	if last := a.buf[len(a.buf)-1].id; last != a.nextID {
		t.Errorf("last id = %d, want %d", last, a.nextID)
	}
}

func TestRateTickFeedsTheSparkline(t *testing.T) {
	a := loadApp(t)
	if err := a.ui.Batch(a.tick); err != nil {
		t.Fatal(err)
	}
	n := a.ingested
	if err := a.ui.Batch(a.rateTick); err != nil {
		t.Fatal(err)
	}
	hist, _ := a.ui.Get("rate_hist")
	hs := hist.([]any)
	if len(hs) != histLen || hs[len(hs)-1] != float64(n) {
		t.Errorf("rate_hist = %v, want %d entries ending in %d", hs, histLen, n)
	}
	if a.ingested != 0 {
		t.Error("the rate tick did not reset the counter")
	}
	if v, _ := a.ui.Get("rate_label"); v != rateLabel(float64(n)) {
		t.Errorf("rate_label = %v", v)
	}
}

func TestOnErrorShowsTheErrorInTheStatusLine(t *testing.T) {
	a := loadApp(t)
	if err := a.onError(nil, errors.New("follow: boom")); err != nil {
		t.Fatalf("onError must keep the session alive, got %v", err)
	}
	if g := grid(t, a, 100, 30); !strings.Contains(strings.SplitN(g, "\n", 2)[0], "follow: boom") {
		t.Errorf("the error is not shown:\n%s", g)
	}
	// A tick that works again clears it.
	if err := a.ui.Batch(a.tick); err != nil {
		t.Fatal(err)
	}
	if v, _ := a.ui.Get("notice"); v != "" {
		t.Errorf("notice = %v after a good tick", v)
	}
}

func deref(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}

// logRows are the rows inside the log's border, top to bottom.
func logRows(res *tuimark.PlayResult) []string {
	g := res.Dump.Grid
	var rows []string
	for _, r := range g[4 : len(g)-2] {
		rows = append(rows, strings.TrimSpace(strings.Trim(r, "│")))
	}
	return rows
}

// The log follows its end while you are at the bottom and holds still
// once you scroll up (stick="bottom"), until you jump back down with G.
func TestLogFollowsUntilScrolledUp(t *testing.T) {
	a := loadApp(t)
	opts := tuimark.PlayOptions{Cols: 100, Rows: 24}
	res, err := a.ui.Play(opts)
	if err != nil {
		t.Fatal(err)
	}
	bottom := logRows(res)
	tickN := func(n int) {
		for i := 0; i < n; i++ {
			if err := a.ui.Batch(a.tick); err != nil {
				t.Fatal(err)
			}
		}
	}
	// At the bottom: new lines push the view along.
	tickN(3)
	res, err = a.ui.Play(opts)
	if err != nil {
		t.Fatal(err)
	}
	followed := logRows(res)
	if followed[0] == bottom[0] {
		t.Fatal("the log did not follow new lines")
	}
	// Scroll up a page, then let lines arrive: the view must not move.
	res, err = a.ui.Play(opts, "pgup")
	if err != nil {
		t.Fatal(err)
	}
	held := logRows(res)
	if held[0] == followed[0] {
		t.Fatal("pgup did not scroll")
	}
	tickN(6)
	res, err = a.ui.Play(opts)
	if err != nil {
		t.Fatal(err)
	}
	if got := logRows(res); strings.Join(got, "\n") != strings.Join(held, "\n") {
		t.Errorf("the log jumped while scrolled up:\nwas:\n%s\nnow:\n%s", strings.Join(held, "\n"), strings.Join(got, "\n"))
	}
	// G jumps to the end and follows again.
	res, err = a.ui.Play(opts, "G")
	if err != nil {
		t.Fatal(err)
	}
	end := logRows(res)
	tickN(3)
	res, err = a.ui.Play(opts)
	if err != nil {
		t.Fatal(err)
	}
	if got := logRows(res); strings.Join(got, "\n") == strings.Join(end, "\n") {
		t.Error("the log did not follow again after G")
	}
}

func TestPlayMouseToggle(t *testing.T) {
	a := loadApp(t)
	if _, err := a.ui.Play(tuimark.PlayOptions{Cols: 80, Rows: 24}, "m"); err != nil {
		t.Fatal(err)
	}
	if v, _ := a.ui.Get("mouse"); v != true {
		t.Errorf("mouse = %v, want true", v)
	}
	if _, err := a.ui.Play(tuimark.PlayOptions{Cols: 80, Rows: 24}, "m"); err != nil {
		t.Fatal(err)
	}
	if v, _ := a.ui.Get("mouse"); v != false {
		t.Errorf("mouse = %v, want false", v)
	}
}

// Every key of mock.json is an action app.tui fires, so `tuimark run
// --mock mock.json` accepts the file.
func TestMockNamesOnlyDocumentActions(t *testing.T) {
	a := loadApp(t)
	known := map[string]bool{}
	for _, spec := range a.ui.Catalog() {
		if !spec.Builtin {
			known[spec.Name] = true
		}
	}
	raw, err := os.ReadFile("mock.json")
	if err != nil {
		t.Fatal(err)
	}
	var mock map[string][]struct {
		Path  string `json:"path"`
		Value any    `json:"value"`
	}
	if err := json.Unmarshal(raw, &mock); err != nil {
		t.Fatal(err)
	}
	for action, writes := range mock {
		if !known[action] {
			t.Errorf("mock.json mocks %q, which app.tui does not fire", action)
		}
		if len(writes) == 0 {
			t.Errorf("%s has no writes", action)
		}
	}
	for _, want := range []string{"pause", "clear", "toggle_error", "toggle_warn", "toggle_info", "toggle_debug"} {
		if _, ok := mock[want]; !ok {
			t.Errorf("mock.json does not mock %s", want)
		}
	}
}

// A long error must neither break the layout nor raise a diagnostic, at
// the widths Validate checks, in both themes.
func TestLongErrorStaysInTheStatusLine(t *testing.T) {
	a := loadApp(t)
	long := "follow: open /var/log/some/deeply/nested/service/directory/with/a/very/long/path/app.log: permission denied"
	if err := a.onError(nil, errors.New(long)); err != nil {
		t.Fatal(err)
	}
	for _, theme := range []string{"dark", "light"} {
		if err := a.ui.Set("@theme", theme); err != nil {
			t.Fatal(err)
		}
		if diags := a.ui.Validate(); len(diags) != 0 {
			t.Errorf("%s: Validate gave %v", theme, diags)
		}
	}
	for _, cols := range []int{40, 80, 120} {
		g := grid(t, a, cols, 24)
		first := strings.SplitN(g, "\n", 2)[0]
		if !strings.Contains(first, "follow: ") {
			t.Errorf("%d cols: the error is not in the status line: %q", cols, first)
		}
		if strings.Contains(first, "api-gateway") {
			t.Errorf("%d cols: the source name is still shown beside the error", cols)
		}
	}
}
