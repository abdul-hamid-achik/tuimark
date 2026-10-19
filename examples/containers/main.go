// Command containers runs examples/containers/app.tui: a container-engine
// dashboard in the spirit of Docker Desktop or k9s, built on the Tuimark kit,
// over a simulated engine (no Docker is touched).
//
//	go run ./examples/containers                   # from the repo root
//	go run ./examples/containers -theme light      # Set("@theme", "light") before Run
//	go run ./examples/containers -dump 120x40      # print Dump() as JSON and exit
//	go run ./examples/containers -seed 3           # other CPU, memory and log noise
//	go run ./examples/containers -demo 100x24      # scripted frames as JSON lines, for the docs
//
// The view lives in app.tui, app.tcss and the kit's two stylesheets. This host
// holds no layout, color, or truncation code. It simulates the engine
// (sim.go: a state machine whose clock and random numbers are injected) and
// publishes plain data:
//
//   - Every: a one-second tick advances the engine and writes the containers,
//     their logs, the events, the images and the volumes in one Batch;
//   - Derive: the visible rows (filter and sort), the counts, the container
//     under the cursor with its log, and the palette's matches are recomputed
//     inside every store write, so typing in the filter or moving the cursor
//     updates every pane, and no handler recomputes the views (one only keeps
//     the cursor on a visible row);
//   - handlers: stop, restart, remove and the prunes open the kit's confirm
//     dialog; the dialog's answer does the work and raises a toast;
//   - RunWith: a handler that fails lands in a danger toast and the session
//     goes on.
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
	defaultSeed = 7
	toastTTL    = 4 // ticks a toast stays
)

func main() {
	var o options
	flag.StringVar(&o.dir, "dir", "examples/containers", "directory with app.tui, app.tcss, sample.json")
	flag.StringVar(&o.dump, "dump", "", "print Dump() as JSON at COLSxROWS and exit")
	flag.StringVar(&o.demo, "demo", "", "print the scripted demo at COLSxROWS as JSON lines and exit")
	flag.StringVar(&o.theme, "theme", "", "dark or light: Set(\"@theme\", …) before Run")
	flag.Int64Var(&o.seed, "seed", defaultSeed, "seed of the simulated CPU, memory and log noise")
	flag.Parse()
	if err := run(o); err != nil {
		fmt.Fprintln(os.Stderr, "containers:", err)
		os.Exit(1)
	}
}

type options struct {
	dir, dump, demo, theme string
	seed                   int64
}

// app is the host's state. Handlers and Every ticks run on the runtime's
// loop, one at a time, so none of this needs a lock; a Derive function never
// touches it (it runs under the app's lock and reads the store through get).
type app struct {
	ui        *tuimark.App
	sim       *sim
	toastLeft int      // ticks left before the toast clears
	pending   *pending // what the open confirm dialog will do
}

// pending is the operation a confirm dialog waits on.
type pending struct {
	kind  string // stop, restart, remove, prune-images, prune-volumes
	ids   []string
	names []string
}

func run(o options) error {
	if o.dir == "" {
		o.dir = "examples/containers"
	}
	if o.theme != "" && o.theme != "dark" && o.theme != "light" {
		return fmt.Errorf("-theme wants dark or light, got %q", o.theme)
	}
	if o.demo != "" {
		cols, rows, err := parseSize(o.demo, "-demo")
		if err != nil {
			return err
		}
		return demo(os.Stdout, o.dir, cols, rows, o.theme)
	}
	a, err := load(o.dir, o.seed, time.Now())
	if err != nil {
		return err
	}
	if err := a.register(); err != nil {
		return err
	}
	if o.theme != "" {
		if err := a.setTheme(o.theme); err != nil {
			return err
		}
	}
	if o.dump != "" {
		cols, rows, err := parseSize(o.dump, "-dump")
		if err != nil {
			return err
		}
		d, err := a.ui.Dump(cols, rows)
		if err != nil {
			return err
		}
		b, _ := json.MarshalIndent(d, "", "  ")
		fmt.Println(string(b))
		return nil
	}
	stop := a.ui.Every(time.Second, func(b *tuimark.Batch) error { return a.tick(b, time.Now()) })
	defer stop()
	return a.ui.RunWith(context.Background(), os.Stdout, tuimark.RunOptions{OnError: a.onError})
}

func parseSize(s, flagName string) (cols, rows int, err error) {
	if _, err := fmt.Sscanf(s, "%dx%d", &cols, &rows); err != nil || cols < 1 || rows < 1 {
		return 0, 0, fmt.Errorf("%s wants COLSxROWS, got %q", flagName, s)
	}
	return cols, rows, nil
}

// load loads app.tui from dir and starts the engine at start. Nothing is
// read from sample.json: the host publishes the whole store, and sample.json
// is a snapshot of it for the CLI (tuimark validate, dump, play, run).
func load(dir string, seed int64, start time.Time) (*app, error) {
	ui, err := tuimark.Load(filepath.Join(dir, "app.tui"))
	if err != nil {
		return nil, err
	}
	a := &app{ui: ui, sim: newSim(seed, start)}
	if err := a.publish(); err != nil {
		return nil, err
	}
	return a, nil
}

// commands are the palette's entries: id, label, and the key that does the
// same without the palette.
var commands = []any{
	cmd("stop", "Stop the selected containers…", "s"),
	cmd("start", "Start the selected containers", "u"),
	cmd("restart", "Restart the selected containers…", "r"),
	cmd("remove", "Remove the selected containers…", "d"),
	cmd("detail", "Show the container's detail", "enter"),
	cmd("filter", "Filter containers", "/"),
	cmd("sort", "Cycle the sort order", "o"),
	cmd("containers", "Go to containers", "1"),
	cmd("images", "Go to images", "3"),
	cmd("volumes", "Go to volumes", "4"),
	cmd("events", "Go to events", "5"),
	cmd("prune-images", "Prune unused images…", ""),
	cmd("prune-volumes", "Prune unused volumes…", ""),
	cmd("theme", "Switch the theme", "t"),
	cmd("help", "Show the keys", "?"),
}

func cmd(id, label, hint string) map[string]any {
	return map[string]any{"id": id, "label": label, "hint": hint}
}

// publish writes the whole store: the view's own state first, then what the
// engine shows.
func (a *app) publish() error {
	first, firstName := "", ""
	for _, c := range a.sim.cs { // the first row of the default sort, by name
		if first == "" || c.name < firstName {
			first, firstName = c.id, c.name
		}
	}
	return a.ui.Batch(func(b *tuimark.Batch) error {
		return errors.Join(
			b.Set("view", "containers"),
			b.Set("detail_tab", "logs"),
			b.Set("cur", first),
			b.Set("sel", []any{}),
			b.Set("filter", map[string]any{"q": ""}),
			b.Set("sort", map[string]any{"key": sortKeys[0].key}),
			b.Set("confirm", map[string]any{"open": false, "title": "", "message": "", "danger": false}),
			b.Set("palette", map[string]any{"open": false, "query": "", "cur": "stop", "commands": commands}),
			b.Set("help", map[string]any{"open": false}),
			b.Set("toast", map[string]any{"show": false, "text": "", "is_ok": false, "is_warn": false, "is_danger": false}),
			b.Set("status", map[string]any{"text": "no actions yet"}),
			b.Set("theme", map[string]any{"name": "dark"}),
			a.write(b),
		)
	})
}

// write publishes what the engine shows right now.
func (a *app) write(b *tuimark.Batch) error {
	s := a.sim
	return errors.Join(
		b.Set("all", s.allView()),
		b.Set("logs", s.logsView()),
		b.Set("events", s.eventsView()),
		b.Set("images", s.imagesView()),
		b.Set("volumes", s.volumesView()),
		b.Set("engine", s.engineView()),
		b.Set("clock", s.now.Format("15:04:05")),
	)
}

// tick is the Every function: it advances the engine one second, publishes
// it, and counts the toast down. Tests and the demo call it directly.
func (a *app) tick(b *tuimark.Batch, now time.Time) error {
	a.sim.advance(now)
	errs := []error{a.write(b)}
	if a.toastLeft > 0 {
		a.toastLeft--
		if a.toastLeft == 0 {
			errs = append(errs, b.Set("toast.show", false))
		}
	}
	return errors.Join(errs...)
}

// onError shows a failed handler as a danger toast and keeps the session.
func (a *app) onError(_ *tuimark.Event, err error) error {
	return a.ui.Batch(func(b *tuimark.Batch) error { return a.toast(b, "danger", "error: "+err.Error()) })
}

// setter writes the store: *tuimark.App or *tuimark.Batch.
type setter interface {
	Set(path string, v any) error
}

// toast shows a toast of a level (info, ok, warn, danger) and starts its
// clock; tick clears it.
func (a *app) toast(s setter, level, msg string) error {
	a.toastLeft = toastTTL
	return errors.Join(
		s.Set("toast.text", msg),
		s.Set("toast.is_ok", level == "ok"),
		s.Set("toast.is_warn", level == "warn"),
		s.Set("toast.is_danger", level == "danger"),
		s.Set("toast.show", true),
	)
}

// say records the last action in the status bar.
func (a *app) say(s setter, msg string) error { return s.Set("status.text", msg) }

func (a *app) setTheme(name string) error {
	return errors.Join(a.ui.Set("@theme", name), a.ui.Set("theme.name", name))
}

// instanceOf is the instance a kit event belongs to: the id of its source
// before the last "__" (confirm__yes: confirm).
func instanceOf(ev tuimark.Event) string {
	if i := strings.LastIndex(ev.Source, "__"); i >= 0 {
		return ev.Source[:i]
	}
	return ev.Source
}

// names lists container names for a message: "a", "a and b", "a, b and 2 more".
func names(ns []string) string {
	switch len(ns) {
	case 0:
		return "nothing"
	case 1:
		return ns[0]
	case 2:
		return ns[0] + " and " + ns[1]
	case 3:
		return ns[0] + ", " + ns[1] + " and " + ns[2]
	}
	return fmt.Sprintf("%s, %s and %d more", ns[0], ns[1], len(ns)-2)
}
