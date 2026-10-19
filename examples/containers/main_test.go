package main

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

// ---- the view: no diagnostics, at every size, in both themes ----

// The kit promises 40, 80 and 120 columns; this app also holds a short
// terminal (14 rows), which the detail pane needs @media (max-rows) for.
var sizes = [][2]int{{40, 24}, {80, 24}, {120, 40}, {60, 14}}

// cleanAt asserts that the app, as it stands, has no diagnostic (warnings
// included) at every size under both themes.
func cleanAt(t *testing.T, ui *tuimark.App, label string) {
	t.Helper()
	for _, d := range ui.Validate() {
		t.Errorf("%s: validate: %s", label, d)
	}
	for _, theme := range []string{"dark", "light"} {
		if err := ui.Set("@theme", theme); err != nil {
			t.Fatal(err)
		}
		for _, s := range sizes {
			d, err := ui.Dump(s[0], s[1])
			if err != nil {
				t.Fatalf("%s %s %dx%d: %v", label, theme, s[0], s[1], err)
			}
			if !d.OK || len(d.Errors) > 0 {
				t.Errorf("%s %s %dx%d: ok=%v errors=%v\n%s", label, theme, s[0], s[1], d.OK, d.Errors, strings.Join(d.Grid, "\n"))
			}
		}
	}
}

func TestValidateIsClean(t *testing.T) {
	for _, d := range newRig(t).a.ui.Validate() {
		t.Errorf("diagnostic: %s", d)
	}
}

// Inactive tabs are not laid out and closed modals paint nothing, so a clean
// first screen proves little: visit every tab (and every nested tab) and open
// every modal, with data that fills a pane and with none.
func TestEveryStateIsCleanAtEverySize(t *testing.T) {
	type state struct {
		set  [][2]any
		want string // what must be on screen at 120x40 in dark
	}
	states := map[string]state{
		"containers":      {[][2]any{{"view", "containers"}}, "api-gateway"},
		"containers-fill": {[][2]any{{"view", "containers"}, {"filter.q", "a"}}, "Containers · "},
		"no-match":        {[][2]any{{"view", "containers"}, {"filter.q", "zzz"}}, "no container matches the filter"},
		"sort-cpu":        {[][2]any{{"sort.key", "cpu"}}, "sort cpu"},
		"detail-logs":     {[][2]any{{"view", "detail"}}, "api-gateway · log"},
		"detail-env":      {[][2]any{{"view", "detail"}, {"detail_tab", "env"}}, "NODE_ENV"},
		"detail-ports":    {[][2]any{{"view", "detail"}, {"detail_tab", "ports"}}, "Published ports"},
		"images":          {[][2]any{{"view", "images"}}, "REPOSITORY"},
		"volumes":         {[][2]any{{"view", "volumes"}}, "pgdata"},
		"events":          {[][2]any{{"view", "events"}}, "Events · "},
		"confirm":         {[][2]any{{"confirm.open", true}, {"confirm.title", "Stop api-gateway?"}, {"confirm.message", "api-gateway will get SIGTERM and stop. Nothing is removed; start brings it back."}}, "Stop api-gateway?"},
		"confirm-danger": {[][2]any{{"confirm.open", true}, {"confirm.danger", true}, {"confirm.title", "Remove 3 containers?"},
			{"confirm.message", "api-gateway, billing-api and edge-proxy and its writable layer will be deleted. Volumes are kept. This cannot be undone."}}, "Remove 3 containers?"},
		"palette":          {[][2]any{{"palette.open", true}}, "Command palette"},
		"palette-filtered": {[][2]any{{"palette.open", true}, {"palette.query", "prune"}}, "Prune unused images"},
		"palette-empty":    {[][2]any{{"palette.open", true}, {"palette.query", "zzz"}}, "No command matches"},
		"help":             {[][2]any{{"help.open", true}}, "close help"},
		"toast-info":       {[][2]any{{"toast.show", true}, {"toast.text", "hello"}}, "hello"},
		"toast-danger":     {[][2]any{{"toast.show", true}, {"toast.text", "hello"}, {"toast.is_danger", true}}, "hello"},
		"toast-ok":         {[][2]any{{"toast.show", true}, {"toast.text", "hello"}, {"toast.is_ok", true}}, "hello"},
		"toast-warn":       {[][2]any{{"toast.show", true}, {"toast.text", "hello"}, {"toast.is_warn", true}}, "hello"},
		"empty-fleet":      {[][2]any{{"view", "detail"}, {"all", []any{}}, {"cur", ""}}, "no container"},
	}
	names := make([]string, 0, len(states))
	for n := range states {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			r := newRig(t)
			r.advance(sampleSecs)
			for _, kv := range states[name].set {
				r.set(kv[0].(string), kv[1])
			}
			cleanAt(t, r.a.ui, name)
			if want := states[name].want; want != "" {
				r.set("@theme", "dark")
				d, err := r.a.ui.Dump(120, 40)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(strings.Join(d.Grid, "\n"), want) {
					t.Errorf("%s: %q is not on screen:\n%s", name, want, strings.Join(d.Grid, "\n"))
				}
			}
		})
	}
}

// A detail pane for each kind of container: one with no ports, one that
// exited, one that is restarting.
func TestDetailOfEveryKindOfContainerIsClean(t *testing.T) {
	for _, name := range []string{"worker-email", "worker-reports", "db-migrate", "postgres"} {
		for _, tab := range []string{"logs", "env", "ports"} {
			t.Run(name+"-"+tab, func(t *testing.T) {
				r := newRig(t)
				r.advance(sampleSecs)
				r.set("view", "detail")
				r.set("detail_tab", tab)
				r.set("cur", r.idOf(name))
				cleanAt(t, r.a.ui, name+" "+tab)
			})
		}
	}
	r := newRig(t)
	r.advance(sampleSecs)
	c := r.a.sim.byID(r.idOf("api-gateway"))
	r.a.sim.restartC(c)
	r.advance(1)
	r.set("view", "detail")
	r.set("cur", c.id)
	cleanAt(t, r.a.ui, "restarting")
}

func TestEveryTabShowsItsContent(t *testing.T) {
	want := map[string][]string{
		"containers": {"Containers · 12 of 12", "api-gateway", "edge-proxy", "STATUS", "running"},
		"detail":     {"status", "acme/gateway:2.14.0", "CPU", "Memory", "logs (", "env (", "ports ("},
		"images":     {"Images · 13", "REPOSITORY", "nginx", "postgres"},
		"volumes":    {"Volumes · 6", "pgdata", "old-backup"},
		"events":     {"Events · ", "engine"},
	}
	for view, subs := range want {
		r := newRig(t)
		r.advance(sampleSecs)
		res := r.playOpts(tuimark.PlayOptions{Cols: 120, Rows: 40}, `set:view="`+view+`"`)
		grid := gridOf(res)
		for _, s := range subs {
			if !strings.Contains(grid, s) {
				t.Errorf("%s: missing %q in\n%s", view, s, grid)
			}
		}
	}
}

// ---- the table: filter, marks, sort ----

func TestFilterNarrowsTheTableAsYouType(t *testing.T) {
	r := newRig(t)
	r.advance(sampleSecs)
	res := r.play(120, 40, "/")
	if focusOf(res) != "containers__q" {
		t.Fatalf("/ should focus the filter, focus = %q", focusOf(res))
	}
	res = r.play(120, 40, "text:work")
	if got := strings.Join(r.rowNames(), ","); got != "worker-email,worker-reports" {
		t.Fatalf("rows = %s", got)
	}
	grid := gridOf(res)
	if !strings.Contains(grid, "Containers · 2 of 12") || strings.Contains(grid, "postgres") {
		t.Errorf("the table did not narrow:\n%s", grid)
	}
	// The cursor was on api-gateway, which the filter hides: the host moves
	// it to the first visible row.
	if got := r.str("cur"); got != r.idOf("worker-email") {
		t.Errorf("cur = %s, want worker-email", r.a.sim.byID(got).name)
	}
	// The filter matches the image and the state too.
	r.play(120, 40, "ctrl+u", "text:nginx")
	if got := strings.Join(r.rowNames(), ","); got != "edge-proxy" {
		t.Errorf("image match: rows = %s", got)
	}
	r.play(120, 40, "ctrl+u", "text:exit")
	if got := strings.Join(r.rowNames(), ","); got != "db-migrate,worker-reports" {
		t.Errorf("state match: rows = %s", got)
	}
	// enter hands the keys back to the table; esc clears the filter.
	res = r.play(120, 40, "enter")
	if focusOf(res) != "ctrs" {
		t.Errorf("enter should focus the table, focus = %q", focusOf(res))
	}
	res = r.play(120, 40, "/", "esc")
	if r.str("filter.q") != "" || focusOf(res) != "ctrs" || len(r.rowNames()) != 12 {
		t.Errorf("esc should clear the filter and go back to the table: q=%q focus=%q rows=%d", r.str("filter.q"), focusOf(res), len(r.rowNames()))
	}
}

func TestSpaceMarksRowsAndTheTitleCountsThem(t *testing.T) {
	r := newRig(t)
	r.advance(sampleSecs)
	res := r.play(120, 40, "space", "down", "space")
	marks := listAt(r.a.ui.Get, "sel")
	if len(marks) != 2 {
		t.Fatalf("sel = %v, want 2 marks", marks)
	}
	if !strings.Contains(gridOf(res), "2 selected") || strings.Count(gridOf(res), "✓") != 2 {
		t.Errorf("two marked rows and the count:\n%s", gridOf(res))
	}
	r.play(120, 40, "c")
	if n := len(listAt(r.a.ui.Get, "sel")); n != 0 {
		t.Errorf("c should clear the marks, %d left", n)
	}
	r.play(120, 40, "a")
	if n := len(listAt(r.a.ui.Get, "sel")); n != 12 {
		t.Errorf("a should mark every row, got %d", n)
	}
}

func TestSortCyclesAndOrdersTheRows(t *testing.T) {
	r := newRig(t)
	r.advance(sampleSecs)
	res := r.play(120, 40, "o")
	if r.str("sort.key") != "cpu" || !strings.Contains(gridOf(res), "sort cpu") {
		t.Fatalf("o should sort by cpu: key=%q", r.str("sort.key"))
	}
	rows := listAt(r.a.ui.Get, "rows")
	prev := 1e9
	for _, row := range rows {
		n := numField(row, "cpu_n")
		if n > prev {
			t.Errorf("cpu order broken at %s: %v after %v", field(row, "name"), n, prev)
		}
		prev = n
	}
	r.play(120, 40, "o", "o", "o")
	if r.str("sort.key") != "status" {
		t.Fatalf("four more presses should reach status, at %q", r.str("sort.key"))
	}
	names := r.rowNames()
	if last := names[len(names)-1]; last != "worker-reports" && last != "db-migrate" {
		t.Errorf("exited containers come last when sorted by status, got %v", names)
	}
	r.play(120, 40, "o")
	if r.str("sort.key") != "name" {
		t.Errorf("the cycle should come back to name, at %q", r.str("sort.key"))
	}
}

// ---- the detail pane ----

func TestEnterOpensTheDetailOfTheCursorRow(t *testing.T) {
	r := newRig(t)
	r.advance(sampleSecs)
	res := r.play(120, 40, "down", "down", "down", "enter")
	if r.str("view") != "detail" {
		t.Fatalf("enter should switch to the detail tab, view = %q", r.str("view"))
	}
	if r.str("detail.name") != "edge-proxy" {
		t.Errorf("detail.name = %q, want edge-proxy (the fourth row)", r.str("detail.name"))
	}
	grid := gridOf(res)
	for _, want := range []string{"edge-proxy", "nginx:1.27-alpine", "edge-proxy · log", "GET /"} {
		if !strings.Contains(grid, want) {
			t.Errorf("missing %q:\n%s", want, grid)
		}
	}
	if focusOf(res) != "logs-view" {
		t.Errorf("focus should be on the log, got %q", focusOf(res))
	}
	// l/e/p switch the nested tabs.
	res = r.play(120, 40, "e")
	if r.str("detail_tab") != "env" || !strings.Contains(gridOf(res), "NGINX_HOST") {
		t.Errorf("e should show the environment: tab=%q\n%s", r.str("detail_tab"), gridOf(res))
	}
	res = r.play(120, 40, "p")
	if r.str("detail_tab") != "ports" || !strings.Contains(gridOf(res), "8443") {
		t.Errorf("p should show the ports: tab=%q\n%s", r.str("detail_tab"), gridOf(res))
	}
	res = r.play(120, 40, "l")
	if r.str("detail_tab") != "logs" {
		t.Errorf("l should show the log, tab=%q", r.str("detail_tab"))
	}
	// esc goes back to the table, on the same row.
	res = r.play(120, 40, "esc")
	if r.str("view") != "containers" || focusOf(res) != "ctrs" {
		t.Errorf("esc should go back: view=%q focus=%q", r.str("view"), focusOf(res))
	}
	if r.str("cur") != r.idOf("edge-proxy") {
		t.Errorf("the cursor should stay on edge-proxy")
	}
}

func TestTheLogGrowsAndFollowsItsEnd(t *testing.T) {
	r := newRig(t)
	r.advance(sampleSecs)
	r.play(100, 24, "2")
	before := int(r.num("detail.nlogs"))
	r.advance(10)
	after := int(r.num("detail.nlogs"))
	if after <= before {
		t.Fatalf("the log should grow: %d then %d lines", before, after)
	}
	res := r.play(100, 24)
	last := r.a.sim.byID(r.str("cur")).logs
	lastLine := last[len(last)-1].text
	if !strings.Contains(gridOf(res), lastLine) {
		t.Errorf("the newest line %q should be in view:\n%s", lastLine, gridOf(res))
	}
}

func TestTheStatsFollowTheContainerOnEverySecond(t *testing.T) {
	r := newRig(t)
	r.advance(sampleSecs)
	before := listAt(r.a.ui.Get, "detail.cpu_hist")
	r.advance(3)
	after := listAt(r.a.ui.Get, "detail.cpu_hist")
	if len(after) != len(before)+3 && len(after) != histLen {
		t.Errorf("cpu history %d then %d samples", len(before), len(after))
	}
}

// ---- actions: confirm, perform, report ----

func TestStopAsksThenStops(t *testing.T) {
	r := newRig(t)
	r.advance(sampleSecs)
	res := r.play(120, 40, "s")
	if !r.boolean("confirm.open") || r.boolean("confirm.danger") {
		t.Fatalf("s should open a plain confirm dialog; events %v", actions(res))
	}
	if focusOf(res) != "confirm__no" {
		t.Errorf("focus should start on the safe button, got %q", focusOf(res))
	}
	if !strings.Contains(gridOf(res), "Stop api-gateway?") {
		t.Errorf("the dialog does not name the container:\n%s", gridOf(res))
	}
	res = r.play(120, 40, "y")
	c := r.a.sim.byID(r.idOf("api-gateway"))
	if c.state != stExited || r.boolean("confirm.open") {
		t.Fatalf("y should stop it: state=%s open=%v", c.state, r.boolean("confirm.open"))
	}
	if r.str("toast.text") != "Stopped api-gateway" || !r.boolean("toast.is_ok") {
		t.Errorf("toast = %q", r.str("toast.text"))
	}
	if !strings.Contains(r.str("status.text"), "Stopped api-gateway") {
		t.Errorf("status = %q", r.str("status.text"))
	}
	if !strings.Contains(gridOf(res), "Stopped api-gateway") {
		t.Errorf("the toast is not on screen:\n%s", gridOf(res))
	}
	if r.str("detail.state") != "exit 0" {
		t.Errorf("detail.state = %q", r.str("detail.state"))
	}
	// A cancelled dialog changes nothing.
	r.play(120, 40, "down", "r")
	r.play(120, 40, "n")
	if r.boolean("confirm.open") || r.a.sim.byID(r.idOf("billing-api")).state != stRunning {
		t.Error("n should cancel")
	}
	r.play(120, 40, "r", "esc")
	if r.boolean("confirm.open") {
		t.Error("esc should cancel")
	}
}

func TestStartBringsAStoppedContainerBack(t *testing.T) {
	r := newRig(t)
	r.advance(sampleSecs)
	r.play(120, 40, "down", "down", "u") // db-migrate: exit 0
	if got := r.a.sim.byID(r.idOf("db-migrate")).state; got != stRunning {
		t.Fatalf("db-migrate should be running, is %s", got)
	}
	if r.str("toast.text") != "Started db-migrate" {
		t.Errorf("toast = %q", r.str("toast.text"))
	}
	r.play(120, 40, "u")
	if r.str("toast.text") != "Already running" {
		t.Errorf("starting a running container: toast = %q", r.str("toast.text"))
	}
}

func TestMarkedRowsAreTheTargets(t *testing.T) {
	r := newRig(t)
	r.advance(sampleSecs)
	res := r.play(120, 40, "space", "down", "space", "down", "space", "s")
	grid := gridOf(res)
	if !strings.Contains(grid, "Stop 3 containers?") {
		t.Errorf("three marked rows:\n%s", grid)
	}
	r.play(120, 40, "y")
	for _, n := range []string{"api-gateway", "billing-api", "db-migrate"} {
		if st := r.a.sim.byID(r.idOf(n)).state; st != stExited {
			t.Errorf("%s is %s, want exited", n, st)
		}
	}
	if r.str("toast.text") != "Stopped api-gateway and billing-api" { // db-migrate was already exited
		t.Errorf("toast = %q", r.str("toast.text"))
	}
	if r.a.sim.byID(r.idOf("edge-proxy")).state != stRunning {
		t.Error("an unmarked container must not be touched")
	}
}

// Marks on rows the filter hides are kept but never acted on.
func TestHiddenMarksAreNotTargets(t *testing.T) {
	r := newRig(t)
	r.advance(sampleSecs)
	r.play(120, 40, "space", "down", "space") // api-gateway, billing-api
	r.play(120, 40, "/", "text:edge", "enter")
	if n := len(listAt(r.a.ui.Get, "sel")); n != 2 {
		t.Fatalf("the filter should keep the marks, %d left", n)
	}
	res := r.play(120, 40, "s")
	if !strings.Contains(gridOf(res), "Stop edge-proxy?") {
		t.Errorf("only the visible row is a target:\n%s", gridOf(res))
	}
	r.play(120, 40, "y")
	if r.a.sim.byID(r.idOf("api-gateway")).state != stRunning || r.a.sim.byID(r.idOf("billing-api")).state != stRunning {
		t.Error("hidden marked containers must not be stopped")
	}
	if n := len(listAt(r.a.ui.Get, "sel")); n != 2 {
		t.Errorf("the hidden marks should stay, %d left", n)
	}
}

func TestRemoveIsDangerousAndMovesTheCursor(t *testing.T) {
	r := newRig(t)
	r.advance(sampleSecs)
	id := r.idOf("billing-api")
	res := r.play(120, 40, "down", "d")
	if !r.boolean("confirm.danger") || !strings.Contains(gridOf(res), "Remove billing-api?") {
		t.Fatalf("d should ask to remove billing-api:\n%s", gridOf(res))
	}
	res = r.play(120, 40, "y")
	if r.a.sim.byID(id) != nil {
		t.Fatal("billing-api should be gone")
	}
	if got := r.a.sim.byID(r.str("cur")); got == nil || got.name != "db-migrate" {
		t.Errorf("the cursor should move to the next row, db-migrate; it is %v", got)
	}
	if !strings.Contains(gridOf(res), "Removed billing-api") || strings.Contains(gridOf(res), "║   billing-api") {
		t.Errorf("the row should be gone and the toast shown:\n%s", gridOf(res))
	}
	if !strings.Contains(r.str("counts.line"), "10 up") && r.num("counts.total") != 11 {
		t.Errorf("counts.total = %v", r.num("counts.total"))
	}
	// Removing a marked row drops its mark.
	r.play(120, 40, "space", "d", "y")
	if n := len(listAt(r.a.ui.Get, "sel")); n != 0 {
		t.Errorf("a removed container's mark should go with it, %d left", n)
	}
}

func TestRemovingTheLastContainerLeavesAnEmptyDetail(t *testing.T) {
	r := newRig(t)
	r.advance(sampleSecs)
	r.play(120, 40, "a", "d", "y")
	if r.num("counts.total") != 0 || r.str("cur") != "" {
		t.Fatalf("total=%v cur=%q", r.num("counts.total"), r.str("cur"))
	}
	res := r.play(100, 24, "2")
	if !strings.Contains(gridOf(res), "no container") {
		t.Errorf("an empty detail pane should say so:\n%s", gridOf(res))
	}
	cleanAt(t, r.a.ui, "empty")
	res = r.play(100, 24, "1")
	if !strings.Contains(gridOf(res), "no container matches the filter") {
		t.Errorf("an empty table shows its placeholder:\n%s", gridOf(res))
	}
	if res = r.play(100, 24, "s"); r.boolean("confirm.open") || r.str("toast.text") != "No container selected" {
		t.Errorf("stop with nothing to stop: toast = %q", r.str("toast.text"))
	}
}

func TestRestartGoesDownAndComesBack(t *testing.T) {
	r := newRig(t)
	r.advance(sampleSecs)
	r.play(120, 40, "r", "y")
	c := r.a.sim.byID(r.idOf("api-gateway"))
	if c.state != stRestarting || !r.boolean("toast.is_warn") {
		t.Fatalf("state=%s warn=%v", c.state, r.boolean("toast.is_warn"))
	}
	if r.str("detail.state") != "restarting" || !r.boolean("detail.warn") {
		t.Errorf("detail.state = %q", r.str("detail.state"))
	}
	r.advance(stopGrace + 1)
	if c.state != stRunning {
		t.Errorf("it should be back after %d ticks, is %s", stopGrace+1, c.state)
	}
	if r.str("detail.state") != "running" {
		t.Errorf("detail.state = %q", r.str("detail.state"))
	}
}

func TestACrashShowsInEventsAndTheStatusBar(t *testing.T) {
	r := newRig(t)
	r.advance(sampleSecs)
	w := r.a.sim.byID(r.idOf("worker-email"))
	if w.state != stRunning {
		t.Fatal("worker-email should be running before its crash")
	}
	r.advance(w.crashAt - r.a.sim.step)
	if w.state != stRestarting {
		t.Fatalf("worker-email should crash at step %d, is %s", w.crashAt, w.state)
	}
	found := false
	for _, e := range listAt(r.a.ui.Get, "events.lines") {
		if strings.Contains(field(e, "text"), "die worker-email (exit 137") {
			found = true
		}
	}
	if !found {
		t.Error("the crash is not among the events")
	}
	res := r.play(120, 40, "5")
	if !strings.Contains(gridOf(res), "die worker-email") {
		t.Errorf("the events tab does not show the crash:\n%s", gridOf(res))
	}
	r.advance(stopGrace + 1)
	if w.state != stRunning {
		t.Errorf("on-failure policy: it should be running again, is %s", w.state)
	}
}

func TestPrunesAskAndFreeSpace(t *testing.T) {
	r := newRig(t)
	r.advance(sampleSecs)
	res := r.play(120, 40, "3", "p")
	if !r.boolean("confirm.open") || !strings.Contains(gridOf(res), "Remove 2 unused images?") {
		t.Fatalf("p on images should ask:\n%s", gridOf(res))
	}
	r.play(120, 40, "y")
	if n := len(listAt(r.a.ui.Get, "images")); n != 11 {
		t.Errorf("images left = %d, want 11", n)
	}
	if !strings.Contains(r.str("toast.text"), "Removed 2 images") {
		t.Errorf("toast = %q", r.str("toast.text"))
	}
	r.play(120, 40, "p")
	if r.boolean("confirm.open") || r.str("toast.text") != "No unused images" {
		t.Errorf("a second prune has nothing to do: toast = %q", r.str("toast.text"))
	}
	res = r.play(120, 40, "4", "p")
	if !strings.Contains(gridOf(res), "Remove 1 unused volume?") {
		t.Fatalf("p on volumes should ask:\n%s", gridOf(res))
	}
	r.play(120, 40, "y")
	if n := len(listAt(r.a.ui.Get, "volumes")); n != 5 {
		t.Errorf("volumes left = %d, want 5", n)
	}
}

// ---- the palette, the help, the theme ----

func TestPaletteFiltersAndRuns(t *testing.T) {
	r := newRig(t)
	r.advance(sampleSecs)
	res := r.play(120, 40, "ctrl+k")
	if !r.boolean("palette.open") || focusOf(res) != "palette__q" {
		t.Fatalf("ctrl+k should open the palette on its input; open=%v focus=%q", r.boolean("palette.open"), focusOf(res))
	}
	if !strings.Contains(gridOf(res), "Stop the selected containers") || !strings.Contains(gridOf(res), "Go to containers") {
		t.Errorf("all commands should list first:\n%s", gridOf(res))
	}
	res = r.play(120, 40, "text:volumes")
	grid := gridOf(res)
	if !strings.Contains(grid, "Go to volumes") || strings.Contains(grid, "Go to images") {
		t.Errorf("typing should filter the list:\n%s", grid)
	}
	if n := len(listAt(r.a.ui.Get, "found.items")); n != 2 { // go to volumes, prune unused volumes
		t.Errorf("found has %d items, want 2", n)
	}
	res = r.play(120, 40, "enter")
	if r.boolean("palette.open") || !has(actions(res), "palette_run") || r.str("view") != "volumes" {
		t.Errorf("enter should run the first match: events %v view=%q", actions(res), r.str("view"))
	}
	if !strings.Contains(gridOf(res), "Volumes · 6") {
		t.Errorf("the volumes tab is not showing:\n%s", gridOf(res))
	}
}

func TestPaletteCommandsReachTheConfirmDialog(t *testing.T) {
	r := newRig(t)
	r.advance(sampleSecs)
	res := r.play(120, 40, "ctrl+k", "enter") // the first command: stop
	if r.boolean("palette.open") || !r.boolean("confirm.open") || !strings.Contains(gridOf(res), "Stop api-gateway?") {
		t.Fatalf("stop from the palette should open the dialog; palette=%v confirm=%v\n%s", r.boolean("palette.open"), r.boolean("confirm.open"), gridOf(res))
	}
	if focusOf(res) != "confirm__no" {
		t.Errorf("focus = %q, want the safe button", focusOf(res))
	}
	r.play(120, 40, "n")
	r.play(120, 40, "ctrl+k", "text:remove", "enter")
	if !r.boolean("confirm.danger") {
		t.Error("remove from the palette should be a danger dialog")
	}
	r.play(120, 40, "n")
	// Arrow keys move the palette cursor although focus stays in the input.
	r.play(120, 40, "ctrl+k", "down", "down")
	if got := r.str("palette.cur"); got != "start" && got != "restart" {
		t.Errorf("two downs: cur = %q", got)
	}
	r.play(120, 40, "esc")
	if r.boolean("palette.open") {
		t.Error("esc should close the palette")
	}
	res = r.play(120, 40, "ctrl+k", "text:zzz")
	if !strings.Contains(gridOf(res), "No command matches “zzz”") {
		t.Errorf("an empty result should say so:\n%s", gridOf(res))
	}
	r.play(120, 40, "enter")
	if !r.boolean("palette.open") {
		t.Error("enter with no match should leave the palette open")
	}
}

func TestHelpListsTheKeysAndCloses(t *testing.T) {
	r := newRig(t)
	r.advance(sampleSecs)
	res := r.playOpts(tuimark.PlayOptions{Cols: 120, Rows: 40}, "?")
	if !r.boolean("help.open") {
		t.Fatal("? should open the help")
	}
	grid := gridOf(res)
	for _, want := range []string{"help", "commands", "quit", "filter", "stop", "remove", "close help"} {
		if !strings.Contains(grid, want) {
			t.Errorf("help is missing %q:\n%s", want, grid)
		}
	}
	r.play(120, 40, "q")
	if r.boolean("help.open") {
		t.Error("q should close the help")
	}
	r.play(120, 40, "?", "esc")
	if r.boolean("help.open") {
		t.Error("esc should close the help")
	}
	// q quits when no overlay is open.
	res = r.play(120, 40, "q")
	if !has(actions(res), "quit") {
		t.Errorf("q should quit; events %v", actions(res))
	}
}

func TestThemeToggleKeyAndTabsByNumber(t *testing.T) {
	r := newRig(t)
	r.play(100, 30, "t")
	if r.str("theme.name") != "light" {
		t.Fatalf("theme.name = %q", r.str("theme.name"))
	}
	res := r.playOpts(tuimark.PlayOptions{Cols: 100, Rows: 30, Styles: true}, "t")
	if r.str("theme.name") != "dark" || res.Dump.Theme != "dark" {
		t.Errorf("theme.name = %q, dump theme = %q", r.str("theme.name"), res.Dump.Theme)
	}
	for _, kv := range [][2]string{{"3", "images"}, {"4", "volumes"}, {"5", "events"}, {"2", "detail"}, {"1", "containers"}} {
		r.play(100, 30, kv[0])
		if r.str("view") != kv[1] {
			t.Errorf("%s: view = %q, want %q", kv[0], r.str("view"), kv[1])
		}
	}
	r.play(100, 30, "]")
	if r.str("view") != "detail" {
		t.Errorf("]: view = %q", r.str("view"))
	}
	r.play(100, 30, "[")
	if r.str("view") != "containers" {
		t.Errorf("[: view = %q", r.str("view"))
	}
}

// ---- the clock ----

func TestTickClearsTheToast(t *testing.T) {
	r := newRig(t)
	r.advance(sampleSecs)
	r.play(100, 24, "s", "y")
	if !r.boolean("toast.show") {
		t.Fatal("the toast should show")
	}
	r.advance(toastTTL - 1)
	if !r.boolean("toast.show") {
		t.Fatalf("the toast cleared early")
	}
	r.advance(1)
	if r.boolean("toast.show") {
		t.Errorf("the toast should be gone after %d ticks", toastTTL)
	}
	// esc dismisses it at once.
	r.play(100, 24, "s", "y")
	r.play(100, 24, "esc")
	if r.boolean("toast.show") {
		t.Error("esc should dismiss the toast")
	}
}

// ---- focus ----

// Closing an overlay gives the keys back to the table, whichever way it ended.
func TestFocusReturnsToTheTableAfterOverlays(t *testing.T) {
	r := newRig(t)
	r.advance(sampleSecs)
	for _, steps := range [][]string{
		{"s", "y"}, {"s", "n"}, {"s", "esc"}, {"ctrl+k", "esc"}, {"?", "q"}, {"?", "esc"},
	} {
		res := r.play(100, 24, steps...)
		if focusOf(res) != "ctrs" {
			t.Errorf("after %v focus = %q, want ctrs", steps, focusOf(res))
		}
	}
}

// Typing in the filter is typing: q does not quit, digits do not switch tabs,
// t does not switch the theme.
func TestTypingInTheFilterIsNotACommand(t *testing.T) {
	r := newRig(t)
	r.advance(sampleSecs)
	res := r.play(100, 24, "/", "text:q1t?s")
	if has(actions(res), "quit") || r.str("view") != "containers" || r.str("theme.name") != "dark" || r.boolean("help.open") || r.boolean("confirm.open") {
		t.Errorf("the keys should have been typed: view=%q events %v", r.str("view"), actions(res))
	}
	if r.str("filter.q") != "q1t?s" {
		t.Errorf("filter.q = %q", r.str("filter.q"))
	}
}

func TestTheSimulationIsDeterministic(t *testing.T) {
	a, b := newRig(t), newRig(t)
	a.advance(40)
	b.advance(40)
	if fmt.Sprint(a.snapshot()) != fmt.Sprint(b.snapshot()) {
		t.Error("two runs with the same seed differ")
	}
}
