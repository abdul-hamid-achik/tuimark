// Command http runs examples/http/app.tui, a small API client in the
// spirit of Postman or Insomnia: a collection of saved requests and a
// history of sent ones, an editor (method, URL, headers, body), and a
// response pane with the status, timing, the pretty-printed body, and a
// table of headers.
//
//	go build -o bin/http ./examples/http && ./bin/http   # demo, no network
//	./bin/http -base https://api.example.com              # {{base}} points there
//	go run ./examples/http -dump 120x30                   # print Dump() as JSON and exit
//	go run ./examples/http -theme light                   # Set("@theme", "light") before Run
//
// Without -base the host starts an in-process net/http/httptest server with
// a small fake API (see fakeapi.go) and the collection's {{base}}
// placeholder points at it. Any other URL you type is sent for real, with
// a timeout (-timeout, default 10s).
//
// The view lives entirely in app.tui and theme.tcss. This host holds no
// layout, width, padding, column, or truncation code: it loads sample.json,
// sends requests in a goroutine and applies the result with Batch,
// animates the in-flight state with Every, computes the filtered lists and
// summaries with Derive, and shows errors through RunWith's OnError, all
// through the public API (Load, Bind, Set, Get, Batch, On, Every, Derive,
// Dump, RunWith).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

func main() {
	dir := flag.String("dir", "examples/http", "directory with app.tui, theme.tcss, sample.json")
	dumpSize := flag.String("dump", "", "print Dump() as JSON at COLSxROWS and exit")
	theme := flag.String("theme", "", "dark or light: Set(\"@theme\", …) before Run")
	base := flag.String("base", "", "URL the {{base}} placeholder points at (default: the built-in fake API)")
	timeout := flag.Duration("timeout", 10*time.Second, "per-request timeout")
	flag.Parse()
	if err := run(*dir, *dumpSize, *theme, *base, *timeout); err != nil {
		fmt.Fprintln(os.Stderr, "http:", err)
		os.Exit(1)
	}
}

// app is the host's state. The store (ui) is the source of truth for what
// the view shows; the host keeps only what the view must not see: the
// running request, the cancel function, and the cached responses.
type app struct {
	ui      *tuimark.App
	hc      *http.Client
	timeout time.Duration
	api     *fakeAPI // the demo server, nil with -base

	root     context.Context // ends with the session: cancels what is in flight
	stopRoot context.CancelFunc

	// dispatch runs the request. The default is a goroutine; tests run it
	// inline to assert the store after a Play step.
	dispatch func(func())

	mu     sync.Mutex
	flight *flight
	cache  map[string]map[string]any // history id -> response, as stored
	hseq   int
	cseq   int
}

// flight is the request in progress.
type flight struct {
	ctx    context.Context
	cancel context.CancelFunc
	start  time.Time
	label  string
	frame  int
}

func run(dir, dumpSize, theme, base string, timeout time.Duration) error {
	a, err := load(dir)
	if err != nil {
		return err
	}
	a.timeout = timeout
	a.register()
	if theme != "" {
		if theme != "dark" && theme != "light" {
			return fmt.Errorf("-theme wants dark or light, got %q", theme)
		}
		if err := a.ui.Set("@theme", theme); err != nil {
			return err
		}
	}
	if dumpSize != "" {
		var cols, rows int
		if _, err := fmt.Sscanf(dumpSize, "%dx%d", &cols, &rows); err != nil {
			return fmt.Errorf("-dump wants COLSxROWS, got %q", dumpSize)
		}
		d, err := a.ui.Dump(cols, rows)
		if err != nil {
			return err
		}
		b, _ := json.MarshalIndent(d, "", "  ")
		fmt.Println(string(b))
		return nil
	}
	defer a.stopRoot()
	if base == "" {
		a.api = newFakeAPI(1500 * time.Millisecond)
		defer a.api.Close()
		if err := a.setBase(a.api.URL(), "demo"); err != nil {
			return err
		}
	} else {
		if err := a.setBase(strings.TrimRight(base, "/"), "live"); err != nil {
			return err
		}
	}
	stop := a.ui.Every(100*time.Millisecond, a.tick)
	defer stop()
	return a.ui.RunWith(a.root, os.Stdout, tuimark.RunOptions{
		OnError: func(ev *tuimark.Event, err error) error {
			// A failed handler or timer must not end the session: say so in
			// the status line and go on.
			_ = a.say("error: "+err.Error(), true)
			return nil
		},
	})
}

// load loads app.tui from dir, binds sample.json, and reads the host's own
// bookkeeping (id counters, the cached response of the first history entry)
// from the same sample.
func load(dir string) (*app, error) {
	ui, err := tuimark.Load(filepath.Join(dir, "app.tui"))
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(filepath.Join(dir, "sample.json"))
	if err != nil {
		return nil, err
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}
	if err := ui.Bind("", data); err != nil {
		return nil, err
	}
	root, stop := context.WithCancel(context.Background())
	a := &app{
		ui: ui, hc: &http.Client{}, timeout: 10 * time.Second,
		root: root, stopRoot: stop,
		cache: map[string]map[string]any{},
	}
	a.dispatch = func(f func()) { go f() }
	hist := asList(data["history"])
	a.hseq = len(hist)
	for _, c := range asList(data["collection"]) {
		var n int
		fmt.Sscanf(str(asMap(c)["id"]), "c%d", &n)
		a.cseq = max(a.cseq, n)
	}
	if len(hist) > 0 {
		if resp := asMap(data["resp"]); resp != nil {
			a.cache[str(asMap(hist[0])["id"])] = resp
		}
	}
	return a, nil
}

// setBase points the {{base}} placeholder at base; kind is "demo" or
// "live", shown with the timeout in the title bar.
func (a *app) setBase(base, kind string) error {
	return a.ui.Batch(func(b *tuimark.Batch) error {
		_ = b.Set("base", base)
		return b.Set("mode", kind+" · timeout "+a.timeout.String())
	})
}

// say writes the status line.
func (a *app) say(msg string, isErr bool) error {
	return a.ui.Batch(func(b *tuimark.Batch) error {
		_ = b.Set("status", msg)
		return b.Set("status_err", isErr)
	})
}

// tick is the Every function: while a request is in flight it advances the
// spinner and the elapsed time. It never ticks in Dump or Play.
func (a *app) tick(b *tuimark.Batch) error {
	u := a.tickUpdate()
	if u == nil {
		return nil
	}
	return applyTick(b, u)
}

// tickUpdate computes the next spinner frame, elapsed time, and timeout
// share for the request in flight, or nil when none is.
func (a *app) tickUpdate() map[string]any {
	a.mu.Lock()
	defer a.mu.Unlock()
	f := a.flight
	if f == nil {
		return nil
	}
	f.frame++
	return flightView(f.label, f.frame, time.Since(f.start), a.timeout)
}

// applyTick writes only the animated members. startSend and finish own
// flight.active and flight.label: a tick that lands after finish (it was
// computed before, applied after) then only touches a hidden row and can
// never bring the in-flight row back.
func applyTick(b *tuimark.Batch, u map[string]any) error {
	for _, k := range []string{"spin", "elapsed", "pct"} {
		if err := b.Set("flight."+k, u[k]); err != nil {
			return err
		}
	}
	return nil
}

var spinner = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// flightView is the store value of the in-flight row: the spinner frame,
// the elapsed time, and the share of the timeout used so far.
func flightView(label string, frame int, elapsed, timeout time.Duration) map[string]any {
	pct := 0.0
	if timeout > 0 {
		pct = min(100, float64(elapsed)/float64(timeout)*100)
	}
	return map[string]any{
		"active": true, "label": label, "spin": spinner[frame%len(spinner)],
		"elapsed": fmt.Sprintf("%.1fs", elapsed.Seconds()), "pct": float64(int(pct)),
	}
}

func idleFlight() map[string]any {
	return map[string]any{"active": false, "label": "", "spin": spinner[0], "elapsed": "0.0s", "pct": 0.0}
}

// ---- small store helpers ----

func asMap(v any) map[string]any { m, _ := v.(map[string]any); return m }
func asList(v any) []any         { l, _ := v.([]any); return l }
func str(v any) string           { s, _ := v.(string); return s }
func flt(v any) float64          { f, _ := v.(float64); return f }
func truthy(v any) bool          { b, _ := v.(bool); return b }

func (a *app) get(path string) any { v, _ := a.ui.Get(path); return v }
