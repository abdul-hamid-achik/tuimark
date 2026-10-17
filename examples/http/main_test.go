package main

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

// loadApp is the wiring main.go's run() does before Run: load, register.
func loadApp(t *testing.T) *app {
	t.Helper()
	a, err := load(".")
	if err != nil {
		t.Fatal(err)
	}
	a.register()
	t.Cleanup(a.stopRoot)
	return a
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

func TestEveryActionHasAHandler(t *testing.T) {
	a := loadApp(t)
	for _, spec := range a.ui.Catalog() {
		if !spec.Builtin && !spec.Registered {
			t.Errorf("action %q has no handler", spec.Name)
		}
	}
}

func TestDumpShowsTheSampleAt80And120(t *testing.T) {
	a := loadApp(t)
	for _, sz := range []struct {
		cols, rows int
		want       []string
	}{
		{80, 24, []string{"List users", "Create user", "201 Created", "143 ms", "application/json", "ctrl+s send", `"active": true`}},
		{120, 30, []string{"collection 10", "history 3", "PATCH", "headers 6", "Content-Type", "demo · timeout 10s", "q quit", `"team": "math"`}},
	} {
		d, err := a.ui.Dump(sz.cols, sz.rows)
		if err != nil {
			t.Fatal(err)
		}
		if !d.OK || len(d.Errors) != 0 {
			t.Fatalf("%dx%d: dump errors %v", sz.cols, sz.rows, d.Errors)
		}
		grid := strings.Join(d.Grid, "\n")
		for _, w := range sz.want {
			if !strings.Contains(grid, w) {
				t.Errorf("%dx%d: grid lacks %q:\n%s", sz.cols, sz.rows, w, grid)
			}
		}
	}
}

// At 40 columns the sidebar is hidden (a @media rule) and nothing clips.
func TestNarrowTerminalHidesTheSidebar(t *testing.T) {
	a := loadApp(t)
	d, err := a.ui.Dump(40, 24)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Errors) != 0 {
		t.Fatalf("diagnostics at 40 columns: %v", d.Errors)
	}
	if grid := strings.Join(d.Grid, "\n"); strings.Contains(grid, "List users") || !strings.Contains(grid, "request · Create user") {
		t.Errorf("unexpected 40-column grid:\n%s", grid)
	}
}

// sample.json carries every path the markup reads, derived ones included,
// because the CLI (and `tuimark run --mock`) has no Derive. This keeps
// that copy honest: after register(), each derived path must equal what
// the sample says.
func TestSampleDerivedValuesAgreeWithDerive(t *testing.T) {
	raw, err := os.ReadFile("sample.json")
	if err != nil {
		t.Fatal(err)
	}
	var sample map[string]any
	if err := json.Unmarshal(raw, &sample); err != nil {
		t.Fatal(err)
	}
	a := loadApp(t)
	for _, path := range []string{"mf", "visible", "history_view", "counts", "rv", "summary", "edit_title"} {
		got, ok := a.ui.Get(path)
		if !ok {
			t.Errorf("%s: not in the store after register()", path)
			continue
		}
		if !reflect.DeepEqual(got, sample[path]) {
			gb, _ := json.Marshal(got)
			wb, _ := json.Marshal(sample[path])
			t.Errorf("%s differs from sample.json:\n derive: %s\n sample: %s", path, gb, wb)
		}
	}
}

// The response in sample.json is what the fake API really answers, so the
// default view is not a made-up picture of it.
func TestSampleResponseMatchesTheFakeAPI(t *testing.T) {
	a := loadApp(t)
	api := newFakeAPI(time.Millisecond)
	defer api.Close()
	req := a.snapshot()
	req.URL = resolve("{{base}}/users", api.URL())
	res := send(a.root, a.hc, req, 5*time.Second)
	if res.Err != "" {
		t.Fatal(res.Err)
	}
	want, _ := a.ui.Get("resp.body")
	if res.Body != want {
		t.Errorf("fake API body differs from sample.json's resp.body:\n got: %q\nwant: %q", res.Body, want)
	}
	if res.Status != 201 || humanSize(res.Size) != "169 B" {
		t.Errorf("status %d size %s, sample says 201 / 169 B", res.Status, humanSize(res.Size))
	}
}

func TestDerivedValuesFollowTheStore(t *testing.T) {
	a := loadApp(t)
	if err := a.ui.Set("query", "slow"); err != nil {
		t.Fatal(err)
	}
	vis, _ := a.ui.Get("visible")
	rows := vis.([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["name"] != "Slow endpoint" {
		t.Fatalf("visible after filtering = %v", rows)
	}
	if c, _ := a.ui.Get("counts.coll"); c != "1/10" {
		t.Errorf("counts.coll = %v, want 1/10", c)
	}
	if hv, _ := a.ui.Get("history_view"); len(hv.([]any)) != 0 {
		t.Errorf("history_view after filtering by slow = %v", hv)
	}
	_ = a.ui.Set("query", "404")
	hv, _ := a.ui.Get("history_view")
	if rows := hv.([]any); len(rows) != 1 || !rows[0].(map[string]any)["k4"].(bool) {
		t.Errorf("history_view for 404 = %v", rows)
	}

	// The editor's method drives the badge flags and the summary.
	_ = a.ui.Set("req.method", "DELETE")
	if v, _ := a.ui.Get("mf.delete"); v != true {
		t.Errorf("mf.delete = %v", v)
	}
	if v, _ := a.ui.Get("mf.post"); v != false {
		t.Errorf("mf.post = %v", v)
	}
	if s, _ := a.ui.Get("summary"); !strings.HasPrefix(s.(string), "DELETE http://127.0.0.1:8099/users") {
		t.Errorf("summary = %v", s)
	}
	_ = a.ui.Set("req.dirty", true)
	if s, _ := a.ui.Get("edit_title"); s != "request · Create user ●" {
		t.Errorf("edit_title = %v", s)
	}

	// A derived path is read-only.
	if err := a.ui.Set("counts", "x"); err == nil {
		t.Error("Set on a derived path should fail")
	}
}

func TestResponseViewFlags(t *testing.T) {
	cases := []struct {
		resp map[string]any
		flag string
		line string
	}{
		{map[string]any{"status": 200.0, "reason": "OK"}, "k2", "200 OK"},
		{map[string]any{"status": 301.0, "reason": "Moved Permanently"}, "k3", "301 Moved Permanently"},
		{map[string]any{"status": 404.0, "reason": "Not Found"}, "k4", "404 Not Found"},
		{map[string]any{"status": 500.0, "reason": "Internal Server Error"}, "k5", "500 Internal Server Error"},
		{map[string]any{"status": 0.0, "error": "timed out after 1 s"}, "kerr", "ERR"},
	}
	for _, c := range cases {
		v := responseView(c.resp)
		for _, k := range []string{"k2", "k3", "k4", "k5", "kerr"} {
			if want := k == c.flag; v[k] != want {
				t.Errorf("%v: %s = %v, want %v", c.resp, k, v[k], want)
			}
		}
		if v["line"] != c.line {
			t.Errorf("line = %v, want %q", v["line"], c.line)
		}
	}
}

func TestTickAdvancesTheSpinner(t *testing.T) {
	a := loadApp(t)
	f := &flight{cancel: func() {}, start: time.Now().Add(-2 * time.Second), label: "GET /slow"}
	a.flight = f
	_ = a.ui.Set("flight", flightView(f.label, 0, 0, a.timeout))
	if err := a.ui.Batch(func(b *tuimark.Batch) error { return a.tick(b) }); err != nil {
		t.Fatal(err)
	}
	v, _ := a.ui.Get("flight")
	m := v.(map[string]any)
	if m["active"] != true || m["spin"] != spinner[1] || m["label"] != "GET /slow" {
		t.Errorf("flight after a tick = %v", m)
	}
	if pct := m["pct"].(float64); pct < 15 || pct > 25 {
		t.Errorf("pct = %v, want about 20 (2s of 10s)", pct)
	}
	// With nothing in flight a tick writes nothing.
	a.flight = nil
	_ = a.ui.Set("flight", idleFlight())
	if err := a.ui.Batch(func(b *tuimark.Batch) error { return a.tick(b) }); err != nil {
		t.Fatal(err)
	}
	if v, _ := a.ui.Get("flight.active"); v != false {
		t.Errorf("flight.active after an idle tick = %v", v)
	}
}

// A tick is computed on the loop and applied later; the request goroutine
// can finish in between. The stale tick must not bring the in-flight row
// (and with it a frozen spinner) back over the finished response.
func TestAStaleTickCannotReviveTheFlight(t *testing.T) {
	a := demoApp(t)
	var held func()
	a.dispatch = func(f func()) { held = f }
	if err := a.selectRequest("c7"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ui.Play(playOpts(), "ctrl+s"); err != nil {
		t.Fatal(err)
	}
	stale := a.tickUpdate() // computed while the request is in flight
	if stale == nil {
		t.Fatal("no tick update while in flight")
	}
	held() // the request finishes first
	if err := a.ui.Batch(func(b *tuimark.Batch) error { return applyTick(b, stale) }); err != nil {
		t.Fatal(err)
	}
	if v, _ := a.ui.Get("flight.active"); v != false {
		t.Fatalf("flight.active = %v after a stale tick", v)
	}
	res, err := a.ui.Play(playOpts())
	if err != nil {
		t.Fatal(err)
	}
	grid := strings.Join(res.Dump.Grid, "\n")
	if strings.Contains(grid, "esc cancels") || !strings.Contains(grid, "200 OK") {
		t.Errorf("the status row did not come back:\n%s", grid)
	}
}

func TestTheCacheFollowsTheHistory(t *testing.T) {
	a := demoApp(t)
	a.dispatch = func(f func()) { f() }
	for range 52 {
		if _, err := a.ui.Play(playOpts(), "ctrl+s"); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(mustGet(t, a, "history").([]any)); n != 50 {
		t.Fatalf("history has %d entries, want 50", n)
	}
	a.mu.Lock()
	n := len(a.cache)
	a.mu.Unlock()
	if n != 50 {
		t.Errorf("cache holds %d responses, want 50", n)
	}
}
