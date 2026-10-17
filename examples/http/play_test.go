package main

import (
	"strings"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

// The Play flows below drive the real handlers headlessly. Sending is
// injected: a.dispatch runs the request inline (or holds it back), so the
// store can be asserted right after the step that sent.

func playOpts() tuimark.PlayOptions { return tuimark.PlayOptions{Cols: 120, Rows: 30} }

func actions(res *tuimark.PlayResult) []string {
	var out []string
	for _, e := range res.Events {
		out = append(out, e.Action)
	}
	return out
}

func mustGet(t *testing.T, a *app, path string) any {
	t.Helper()
	v, ok := a.ui.Get(path)
	if !ok {
		t.Fatalf("Get(%q): missing", path)
	}
	return v
}

// demoApp is an app whose requests go to an in-process fake API, run inline.
func demoApp(t *testing.T) *app {
	t.Helper()
	a := loadApp(t)
	api := newFakeAPI(30 * time.Millisecond)
	t.Cleanup(api.Close)
	a.api = api
	a.timeout = 5 * time.Second
	if err := a.setBase(api.URL(), "demo"); err != nil {
		t.Fatal(err)
	}
	a.dispatch = func(f func()) { f() }
	return a
}

func TestSelectingARequestLoadsItIntoTheEditor(t *testing.T) {
	a := loadApp(t)
	// reqs has focus and its cursor is on "Create user"; down moves to "Replace user".
	res, err := a.ui.Play(playOpts(), "down")
	if err != nil {
		t.Fatal(err)
	}
	if got := actions(res); len(got) != 1 || got[0] != "req_select" {
		t.Fatalf("events = %v, want [req_select]", got)
	}
	if k := res.Events[0].Keys["r"]; k != "c4" {
		t.Errorf("selected key = %v, want c4", k)
	}
	for path, want := range map[string]any{
		"req.id": "c4", "req.name": "Replace user", "req.method": "PUT",
		"req.url": "{{base}}/users/1", "req.dirty": false,
	} {
		if got := mustGet(t, a, path); got != want {
			t.Errorf("%s = %v, want %v", path, got, want)
		}
	}
	if grid := strings.Join(res.Dump.Grid, "\n"); !strings.Contains(grid, "request · Replace user") {
		t.Errorf("the editor title did not follow the selection:\n%s", grid)
	}
	if s := mustGet(t, a, "mf.put"); s != true {
		t.Errorf("mf.put = %v", s)
	}
}

func TestTypingInTheURLInputDoesNotQuit(t *testing.T) {
	a := loadApp(t)
	res, err := a.ui.Play(playOpts(), "focus:#url", "text:?q=quit", "ctrl+a", "text:x")
	if err != nil {
		t.Fatal(err)
	}
	if res.Quit {
		t.Fatal("typing q quit the app: the keymap saw a key the input should consume")
	}
	url := mustGet(t, a, "req.url").(string)
	// the text lands at the end, then ctrl+a (the input's, not a global key) moves home
	if url != "x{{base}}/users?q=quit" {
		t.Errorf("req.url = %q", url)
	}
	if mustGet(t, a, "req.dirty") != true {
		t.Error("typing did not mark the request dirty")
	}
	for _, act := range actions(res) {
		if act == "quit" || act == "send" || act == "req_save" {
			t.Errorf("typing fired %q", act)
		}
	}
	// The summary is derived from the URL being typed.
	if s := mustGet(t, a, "summary").(string); !strings.Contains(s, "?q=quit") {
		t.Errorf("summary = %q", s)
	}
	// Leave the input and q quits again.
	res, err = a.ui.Play(playOpts(), "focus:#reqs", "q")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Quit {
		t.Error("q from the list should quit")
	}
}

func TestSwitchingTheResponseTab(t *testing.T) {
	a := loadApp(t)
	res, err := a.ui.Play(playOpts(), "h")
	if err != nil {
		t.Fatal(err)
	}
	if got := mustGet(t, a, "resp_tab"); got != "t-headers" {
		t.Errorf("resp_tab = %v, want t-headers", got)
	}
	grid := strings.Join(res.Dump.Grid, "\n")
	for _, w := range []string{"Header", "Value", "X-Request-Id", "req-0003", "fakeapi/1.0"} {
		if !strings.Contains(grid, w) {
			t.Errorf("headers tab lacks %q:\n%s", w, grid)
		}
	}
	if strings.Contains(grid, `"active": true`) {
		t.Error("the body is still painted on the headers tab")
	}
	res, _ = a.ui.Play(playOpts(), "b")
	if got := mustGet(t, a, "resp_tab"); got != "t-body" {
		t.Errorf("resp_tab = %v after b", got)
	}
	if !strings.Contains(strings.Join(res.Dump.Grid, "\n"), `"active": true`) {
		t.Error("b did not bring the body back")
	}
}

func TestSendingFillsTheResponseAndTheHistory(t *testing.T) {
	a := demoApp(t)
	// Pick "Health check" (6 rows down from "Create user" is c9; go by id
	// instead) and send it with ctrl+s.
	if err := a.selectRequest("c7"); err != nil {
		t.Fatal(err)
	}
	res, err := a.ui.Play(playOpts(), "ctrl+s")
	if err != nil {
		t.Fatal(err)
	}
	if got := actions(res); len(got) == 0 || got[0] != "send" {
		t.Fatalf("events = %v", got)
	}
	if got := mustGet(t, a, "resp.status"); got != 200.0 {
		t.Errorf("resp.status = %v", got)
	}
	if body := mustGet(t, a, "resp.body").(string); !strings.Contains(body, "\n  \"status\": \"ok\"") {
		t.Errorf("body not pretty-printed:\n%s", body)
	}
	if mustGet(t, a, "flight.active") != false {
		t.Error("still in flight")
	}
	hist := mustGet(t, a, "history").([]any)
	if len(hist) != 4 {
		t.Fatalf("history has %d entries, want 4", len(hist))
	}
	first := hist[0].(map[string]any)
	if first["method"] != "GET" || first["url"] != "{{base}}/health" || first["status"] != 200.0 {
		t.Errorf("newest history entry = %v", first)
	}
	if c := mustGet(t, a, "counts.hist"); c != "4" {
		t.Errorf("counts.hist = %v", c)
	}
	if rv := mustGet(t, a, "rv.k2"); rv != true {
		t.Errorf("rv.k2 = %v", rv)
	}
	if st := mustGet(t, a, "status").(string); !strings.HasPrefix(st, "GET /health → 200 OK in ") {
		t.Errorf("status = %q", st)
	}
	grid := strings.Join(res.Dump.Grid, "\n")
	if !strings.Contains(grid, "200 OK") || !strings.Contains(grid, `"version": "1.4.2"`) {
		t.Errorf("the response pane did not update:\n%s", grid)
	}

	// Enter on the URL input sends too, and a 500 is an error status.
	_ = a.ui.Set("req.url", "{{base}}/error")
	res, err = a.ui.Play(playOpts(), "focus:#url", "enter")
	if err != nil {
		t.Fatal(err)
	}
	if mustGet(t, a, "resp.status") != 500.0 || mustGet(t, a, "rv.k5") != true || mustGet(t, a, "status_err") != true {
		t.Errorf("500 not shown as an error: %v", mustGet(t, a, "rv"))
	}
	if !strings.Contains(strings.Join(res.Dump.Grid, "\n"), "500 Internal Server Error") {
		t.Error("the 500 is not on screen")
	}
}

func TestTheInFlightStateShowsBeforeAnyTickAndEscCancels(t *testing.T) {
	a := demoApp(t)
	var held func()
	a.dispatch = func(f func()) { held = f } // the request starts, but is not run yet
	if err := a.selectRequest("c8"); err != nil {
		t.Fatal(err)
	}
	res, err := a.ui.Play(playOpts(), "ctrl+s")
	if err != nil {
		t.Fatal(err)
	}
	if held == nil {
		t.Fatal("send did not dispatch")
	}
	if mustGet(t, a, "flight.active") != true {
		t.Fatal("flight.active should be true from the Set that starts the request")
	}
	grid := strings.Join(res.Dump.Grid, "\n")
	if !strings.Contains(grid, "esc cancels") || !strings.Contains(grid, "GET /slow") || !strings.Contains(grid, spinner[0]) {
		t.Errorf("in-flight row missing:\n%s", grid)
	}
	// A second send while one is running is refused.
	if _, err := a.ui.Play(playOpts(), "ctrl+s"); err != nil {
		t.Fatal(err)
	}
	if st := mustGet(t, a, "status").(string); !strings.Contains(st, "already in flight") {
		t.Errorf("status = %q", st)
	}

	// esc cancels; the held request then finishes as "canceled".
	if _, err := a.ui.Play(playOpts(), "esc"); err != nil {
		t.Fatal(err)
	}
	held()
	if got := mustGet(t, a, "resp.error"); got != "canceled" {
		t.Errorf("resp.error = %v, want canceled", got)
	}
	if mustGet(t, a, "flight.active") != false || mustGet(t, a, "rv.kerr") != true {
		t.Error("the pane did not return to idle with an error status")
	}
	if n := len(mustGet(t, a, "history").([]any)); n != 3 {
		t.Errorf("a canceled request was recorded: %d history entries", n)
	}
}

func TestEditingSavingAndFiltering(t *testing.T) {
	a := loadApp(t)
	// m cycles POST -> PUT; header_add through the draft input; s saves in place.
	res, err := a.ui.Play(playOpts(), "m", "focus:#draft", "text:X-Trace: 42", "enter", "focus:#reqs", "s")
	if err != nil {
		t.Fatal(err)
	}
	if mustGet(t, a, "req.method") != "PUT" {
		t.Errorf("req.method = %v", mustGet(t, a, "req.method"))
	}
	hs := mustGet(t, a, "req.headers").([]any)
	if len(hs) != 3 || hs[2].(map[string]any)["k"] != "X-Trace" {
		t.Errorf("headers = %v", hs)
	}
	if mustGet(t, a, "req.draft") != "" {
		t.Errorf("draft not cleared: %v", mustGet(t, a, "req.draft"))
	}
	coll := mustGet(t, a, "collection").([]any)
	c3 := coll[2].(map[string]any)
	if len(coll) != 10 || c3["method"] != "PUT" || len(c3["headers"].([]any)) != 3 || c3["name"] != "Create user" {
		t.Errorf("collection[2] = %v (len %d)", c3, len(coll))
	}
	if mustGet(t, a, "req.dirty") != false {
		t.Error("still dirty after saving")
	}
	_ = res

	// A new request, saved, is appended.
	if _, err := a.ui.Play(playOpts(), "n", "focus:#reqs", "s"); err != nil {
		t.Fatal(err)
	}
	coll = mustGet(t, a, "collection").([]any)
	if len(coll) != 11 {
		t.Fatalf("collection has %d entries after saving a new request", len(coll))
	}
	if last := coll[10].(map[string]any); last["id"] != "c11" || last["name"] != "GET /" {
		t.Errorf("new entry = %v", last)
	}

	// The filter narrows the list; esc clears it and returns to the list.
	res, err = a.ui.Play(playOpts(), "/", "text:err")
	if err != nil {
		t.Fatal(err)
	}
	if got := mustGet(t, a, "counts.coll"); got != "1/11" {
		t.Errorf("counts.coll = %v", got)
	}
	grid := strings.Join(res.Dump.Grid, "\n")
	if !strings.Contains(grid, "Server error") || strings.Contains(grid, "List users") {
		t.Errorf("filtered list wrong:\n%s", grid)
	}
	if _, err := a.ui.Play(playOpts(), "esc"); err != nil {
		t.Fatal(err)
	}
	if mustGet(t, a, "query") != "" {
		t.Errorf("esc did not clear the filter")
	}
	if f := mustGet(t, a, "@focus"); f != "#reqs" {
		t.Errorf("focus = %v, want #reqs", f)
	}
}

func TestDroppingAHeaderAndDeletingARequest(t *testing.T) {
	a := loadApp(t)
	if _, err := a.ui.Play(playOpts(), "focus:#hdrs", "d"); err != nil {
		t.Fatal(err)
	}
	hs := mustGet(t, a, "req.headers").([]any)
	if len(hs) != 1 || hs[0].(map[string]any)["k"] != "Accept" {
		t.Errorf("headers after d = %v", hs)
	}
	if _, err := a.ui.Play(playOpts(), "focus:#reqs", "D"); err != nil {
		t.Fatal(err)
	}
	if n := len(mustGet(t, a, "collection").([]any)); n != 9 {
		t.Errorf("collection has %d entries after D, want 9", n)
	}
}

func TestSelectingAHistoryEntryReplaysItsResponse(t *testing.T) {
	a := loadApp(t)
	res, err := a.ui.Play(playOpts(), "2", "down", "down")
	if err != nil {
		t.Fatal(err)
	}
	if mustGet(t, a, "side_tab") != "t-hist" {
		t.Fatalf("side_tab = %v", mustGet(t, a, "side_tab"))
	}
	var sawHist bool
	for _, e := range res.Events {
		sawHist = sawHist || e.Action == "hist_select"
	}
	if !sawHist {
		t.Fatalf("no hist_select in %v", actions(res))
	}
	if got := mustGet(t, a, "req.url"); got != "{{base}}/health" {
		t.Errorf("req.url = %v", got)
	}
	if got := mustGet(t, a, "req.id"); got != "" {
		t.Errorf("a history request is not a saved one: id = %v", got)
	}
}

func TestQuitCancelsWhatIsInFlight(t *testing.T) {
	a := demoApp(t)
	a.dispatch = func(func()) {}
	if _, err := a.ui.Play(playOpts(), "focus:#reqs", "ctrl+s"); err != nil {
		t.Fatal(err)
	}
	f := a.flight
	if f == nil {
		t.Fatal("not in flight")
	}
	res, err := a.ui.Play(playOpts(), "q")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Quit {
		t.Fatal("q did not quit")
	}
	select {
	case <-f.ctx.Done():
	default:
		t.Error("the request in flight was not canceled by quitting")
	}
}

func TestHelpListsEveryKey(t *testing.T) {
	a := loadApp(t)
	res, err := a.ui.Play(playOpts(), "focus:#reqs", "?")
	if err != nil {
		t.Fatal(err)
	}
	grid := strings.Join(res.Dump.Grid, "\n")
	for _, w := range []string{"keys", "ctrl+s", "send", "1-2", "pane", "b/h", "body/headers", "new", "drop header"} {
		if !strings.Contains(grid, w) {
			t.Errorf("help lacks %q", w)
		}
	}
	// q is the modal's, not the app's: it closes the help instead of quitting.
	res, err = a.ui.Play(playOpts(), "q")
	if err != nil {
		t.Fatal(err)
	}
	if res.Quit || mustGet(t, a, "help_open") != false {
		t.Errorf("q in the help: quit=%v help_open=%v", res.Quit, mustGet(t, a, "help_open"))
	}
}
