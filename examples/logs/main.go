// Command logs runs examples/logs/app.tui: a live log viewer in the spirit
// of tail -f and a small lnav. A seeded generator streams realistic API
// service logs (requests, queries, cache misses, retries, warnings, errors
// with a stack) into a ring buffer; a scroll pane follows the end until
// you scroll up; a filter box and four level toggles narrow what is shown;
// a status line counts lines per level and graphs lines per second.
//
//	go build -o bin/logs ./examples/logs && ./bin/logs
//	go run ./examples/logs --dump 120x30     # print Dump() as JSON and exit
//	go run ./examples/logs --theme light     # Set("@theme", "light") before Run
//	go run ./examples/logs --seed 42         # another deterministic stream
//	go run ./examples/logs --follow app.log  # tail a real file instead
//
// The view lives entirely in app.tui and theme.tcss. This host holds no
// layout, width, padding, color, or truncation code. It loads sample.json,
// keeps the ring buffer, and publishes plain data: a Batch from App.Every
// appends the new lines, a second Every feeds the lines-per-second
// sparkline, and App.Derive keeps the filtered "visible" list, the counts,
// and the status labels, so the markup stays free of expressions. Errors
// from a tick (a file that vanished, say) reach RunWith's OnError, which
// shows them in the status line instead of ending the session.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

const (
	defaultSeed   = 44
	defaultBuffer = 2000 // the ring buffer: lines kept
	histLen       = 24   // seconds of lines-per-second history
	backlogTicks  = 4 * histLen
	backlogLines  = 48 // lines the opening view starts with, at least
)

func main() {
	dir := flag.String("dir", "examples/logs", "directory with app.tui, theme.tcss, sample.json")
	dumpSize := flag.String("dump", "", "print Dump() as JSON at COLSxROWS and exit")
	theme := flag.String("theme", "", "dark or light: Set(\"@theme\", …) before Run")
	seed := flag.Int64("seed", defaultSeed, "seed of the fake log generator")
	follow := flag.String("follow", "", "tail this file instead of the fake generator")
	buffer := flag.Int("buffer", defaultBuffer, "lines kept in the ring buffer")
	interval := flag.Duration("interval", tickStep, "time between generator ticks")
	flag.Parse()
	o := options{dir: *dir, dump: *dumpSize, theme: *theme, seed: *seed, follow: *follow, buffer: *buffer, interval: *interval}
	if err := run(o); err != nil {
		fmt.Fprintln(os.Stderr, "logs:", err)
		os.Exit(1)
	}
}

type options struct {
	dir, dump, theme, follow string
	seed                     int64
	buffer                   int
	interval                 time.Duration
}

// line is one buffered log line. m is its store form, built once.
type line struct {
	id    int
	level string
	m     map[string]any
}

// app is the host's state. Everything below runs on the runtime's loop:
// handlers and Every functions are called one at a time, so no lock.
type app struct {
	ui  *tuimark.App
	src source
	now func() time.Time

	buf      []line
	cap      int
	nextID   int
	lastHay  string // search text of the last parent line, for its stack
	paused   bool
	ingested int // lines added since the last rate tick
	hist     []float64
	notice   string
}

// load loads app.tui from dir, binds sample.json, and starts the host on
// the default seed: the generator is replayed up to the point the sample
// shows, so the first tick continues exactly where the sample stops.
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
	a := &app{ui: ui, cap: defaultBuffer, now: time.Now}
	if err := a.useFake(defaultSeed); err != nil {
		return nil, err
	}
	return a, nil
}

// backlog runs the generator for backlogTicks ticks (histLen seconds of
// stream) and returns the last lines, never starting on a stack frame,
// plus the lines-per-second history of those seconds.
func backlog(g *fake) ([]entry, []float64) {
	var all []entry
	var hist []float64
	sec := 0
	for i := 1; i <= backlogTicks; i++ {
		es, _ := g.Next()
		all = append(all, es...)
		sec += len(es)
		if i%4 == 0 {
			hist = append(hist, float64(sec))
			sec = 0
		}
	}
	start := len(all) - backlogLines
	if start < 0 {
		start = 0
	}
	for start > 0 && all[start].Cont {
		start--
	}
	return all[start:], hist
}

// useFake switches the stream to a fake generator with this seed, with its
// backlog already in the buffer.
func (a *app) useFake(seed int64) error {
	g := newFake(seed)
	es, hist := backlog(g)
	return a.reset(g, es, hist)
}

// useFile switches the stream to a real file, positioned at its end, with
// its last lines as the backlog.
func (a *app) useFile(path string) error {
	src, es, err := newFileSource(path, a.now)
	if err != nil {
		return err
	}
	return a.reset(src, es, make([]float64, histLen))
}

// reset replaces the source, the buffer, and the history, and publishes
// them in one batch.
func (a *app) reset(src source, es []entry, hist []float64) error {
	a.src, a.buf, a.nextID, a.ingested = src, nil, 0, 0
	a.hist = hist
	a.notice = ""
	a.append(es)
	return a.ui.Batch(func(b *tuimark.Batch) error {
		return errors.Join(
			b.Set("source", src.Name()),
			b.Set("lines", a.items()),
			b.Set("rate_hist", a.hist),
			b.Set("notice", ""),
		)
	})
}

// append numbers the entries, builds their store form, and trims the ring
// buffer to its capacity, never leaving a stack frame first.
func (a *app) append(es []entry) {
	for i, e := range es {
		a.nextID++
		hay := groupHay(es, i)
		lvl := strings.ToUpper(e.Level)
		if e.Cont {
			lvl = ""
		}
		a.buf = append(a.buf, line{id: a.nextID, level: e.Level, m: map[string]any{
			"id": a.nextID, "ts": e.TS, "lvl": lvl, "level": e.Level, "svc": e.Svc, "msg": e.Msg,
			"cont": e.Cont, "hay": hay,
			"error": e.Level == lvlError, "warn": e.Level == lvlWarn,
			"info": e.Level == lvlInfo, "debug": e.Level == lvlDebug,
		}})
	}
	if over := len(a.buf) - a.cap; over > 0 {
		for over < len(a.buf) && a.buf[over].m["cont"] == true {
			over++
		}
		a.buf = append([]line(nil), a.buf[over:]...)
	}
}

// groupHay is the search text of es[i]: the lowercase service, level, and
// message of its event, with the messages of its stack frames. A stack
// frame has its parent's text, so a filter keeps or drops an event whole.
func groupHay(es []entry, i int) string {
	start := i
	for start > 0 && es[start].Cont {
		start--
	}
	p := es[start]
	var b strings.Builder
	b.WriteString(p.Svc + " " + p.Level + " " + p.Msg)
	for j := start + 1; j < len(es) && es[j].Cont; j++ {
		b.WriteString(" " + es[j].Msg)
	}
	return strings.ToLower(b.String())
}

// items is the buffer as the store holds it.
func (a *app) items() []any {
	out := make([]any, len(a.buf))
	for i, l := range a.buf {
		out[i] = l.m
	}
	return out
}

func run(o options) error {
	a, err := load(o.dir)
	if err != nil {
		return err
	}
	if o.buffer > 0 {
		a.cap = o.buffer
	}
	if err := a.register(); err != nil {
		return err
	}
	if o.theme != "" {
		if o.theme != "dark" && o.theme != "light" {
			return fmt.Errorf("--theme wants dark or light, got %q", o.theme)
		}
		if err := a.ui.Set("@theme", o.theme); err != nil {
			return err
		}
	}
	if o.follow != "" {
		if err := a.useFile(o.follow); err != nil {
			return err
		}
	} else if o.seed != defaultSeed {
		if err := a.useFake(o.seed); err != nil {
			return err
		}
	}
	if o.dump != "" {
		var cols, rows int
		if _, err := fmt.Sscanf(o.dump, "%dx%d", &cols, &rows); err != nil {
			return fmt.Errorf("--dump wants COLSxROWS, got %q", o.dump)
		}
		d, err := a.ui.Dump(cols, rows)
		if err != nil {
			return err
		}
		b, _ := json.MarshalIndent(d, "", "  ")
		fmt.Println(string(b))
		return nil
	}
	if o.interval <= 0 {
		o.interval = tickStep
	}
	stopTick := a.ui.Every(o.interval, a.tick)
	defer stopTick()
	stopRate := a.ui.Every(time.Second, a.rateTick)
	defer stopRate()

	// Run restores the terminal on SIGINT, SIGTERM, or SIGHUP and returns
	// an error naming the signal, so the command exits with a failure.
	return a.ui.RunWith(context.Background(), os.Stdout, tuimark.RunOptions{OnError: a.onError})
}

// onError keeps the session alive when a tick or a handler fails: the
// message goes to the status line and the stream goes on.
func (a *app) onError(ev *tuimark.Event, err error) error {
	a.notice = err.Error()
	_ = a.ui.Set("notice", a.notice)
	return nil
}

// tick is the Every function of the stream: it asks the source for new
// lines and publishes the buffer in the tick's batch. A paused stream
// reads nothing; a file source then catches up on resume.
func (a *app) tick(b *tuimark.Batch) error {
	if a.paused {
		return nil
	}
	es, err := a.src.Next()
	if err != nil {
		return err
	}
	if a.notice != "" { // the source works again
		a.notice = ""
		if err := b.Set("notice", ""); err != nil {
			return err
		}
	}
	if len(es) == 0 {
		return nil
	}
	a.append(es)
	a.ingested += len(es)
	return b.Set("lines", a.items())
}

// rateTick closes one second: how many lines arrived goes to the end of
// the history the sparkline draws.
func (a *app) rateTick(b *tuimark.Batch) error {
	a.hist = append(a.hist, float64(a.ingested))
	if len(a.hist) > histLen {
		a.hist = a.hist[len(a.hist)-histLen:]
	}
	a.ingested = 0
	return b.Set("rate_hist", a.hist)
}

// register installs the handlers and the derived values.
func (a *app) register() error {
	for name, h := range a.handlers() {
		a.ui.On(name, h)
	}
	return a.derive()
}

// handlers are the named actions of app.tui.
func (a *app) handlers() map[string]tuimark.Handler {
	ui := a.ui
	hs := map[string]tuimark.Handler{}
	on := func(name string, h tuimark.Handler) { hs[name] = h }
	on("quit", func(tuimark.Event) error { return tuimark.ErrQuit })
	on("pause", func(tuimark.Event) error {
		a.paused = !a.paused
		return ui.Set("paused", a.paused)
	})
	on("clear", func(tuimark.Event) error {
		a.buf = nil
		return ui.Set("lines", []any{})
	})
	for _, name := range levelNames {
		name := name
		on("toggle_"+name, func(tuimark.Event) error { return a.toggleLevel(name) })
	}
	on("filter_apply", func(tuimark.Event) error { return ui.Set("@focus", "#log") })
	on("filter_clear", func(tuimark.Event) error {
		return errors.Join(ui.Set("filter", ""), ui.Set("@focus", "#log"))
	})
	on("toggle_mouse", func(tuimark.Event) error {
		v, _ := ui.Get("mouse")
		enabled, _ := v.(bool)
		return ui.Set("mouse", !enabled)
	})
	on("help_open", func(tuimark.Event) error { return ui.Set("help_open", true) })
	on("help_close", func(tuimark.Event) error { return ui.Set("help_open", false) })
	return hs
}

// toggleLevel flips one level's switch in the store; "visible" and the
// counts follow by themselves.
func (a *app) toggleLevel(name string) error {
	v, _ := a.ui.Get("levels." + name)
	on, _ := v.(bool)
	return a.ui.Set("levels."+name, !on)
}
