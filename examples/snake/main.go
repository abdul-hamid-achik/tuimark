// Command snake runs examples/snake/app.tui: Snake in the terminal, a
// demonstration of App.Every as a game loop.
//
//	go build -o bin/snake ./examples/snake && ./bin/snake
//	go run ./examples/snake -autopilot       # let the BFS autopilot play
//	go run ./examples/snake -seed 7          # another food sequence
//	go run ./examples/snake -theme light     # Set("@theme", "light") before Run
//	go run ./examples/snake -dump 90x24      # print Dump() as JSON and exit
//	go run ./examples/snake -demo 90x24      # the scripted autopilot run, one JSON line per frame
//
// The view lives entirely in app.tui and theme.tcss: the board is a nested
// container each over runs of same-kind cells, coloured by class: guards.
// This host holds the rules (game.go, auto.go), publishes plain data, and
// implements the named actions. It has no layout, width, or colour code.
//
// The game loop is one App.Every. Its interval is the snake's speed, so the
// host stops the registration and arms another whenever the level changes
// (140ms at level 1, 7ms faster per food, 70ms at level 11). Every callback
// is step, a pure function of the app and the clock it is handed.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

const (
	defaultSeed = 1
	histLen     = 22 // samples of the speed sparkline: its width in cells
)

func main() {
	dir := flag.String("dir", "examples/snake", "directory with app.tui and theme.tcss")
	theme := flag.String("theme", "", "dark or light: Set(\"@theme\", …) before Run")
	seed := flag.Int64("seed", defaultSeed, "seed of the food placement")
	auto := flag.Bool("autopilot", false, "start with the autopilot playing")
	dumpSize := flag.String("dump", "", "print Dump() as JSON at COLSxROWS and exit")
	demoSize := flag.String("demo", "", "print the scripted autopilot demo at COLSxROWS, one {\"ms\",\"dump\"} JSON line per frame, and exit")
	flag.Parse()
	o := options{dir: *dir, theme: *theme, seed: *seed, auto: *auto, dump: *dumpSize, demo: *demoSize}
	if err := run(o); err != nil {
		fmt.Fprintln(os.Stderr, "snake:", err)
		os.Exit(1)
	}
}

type options struct {
	dir, theme, dump, demo string
	seed                   int64
	auto                   bool
}

// app is the host's state. Handlers and Every functions run one at a time
// on the runtime's loop, so there is no lock.
type app struct {
	ui *tuimark.App
	g  *game

	auto      bool
	paused    bool
	help      bool // the help modal is open
	helpPause bool // the help modal paused the game, so closing it resumes
	hist      []any
	elapsed   time.Duration
	notice    string    // the last error a tick or a handler returned, until something works
	last      time.Time // the clock of the previous step, to measure play time

	// arm replaces the Every registration with one of interval d. It is nil
	// until run installs it: tests and the demo call step themselves.
	arm   func(d time.Duration)
	armed time.Duration
}

// load loads app.tui from dir and builds a new game with seed. The store
// starts from the game, not from a file; sample.json is the same store for
// the tuimark CLI, kept in sync by a test.
func load(dir string, seed int64) (*app, error) {
	ui, err := tuimark.Load(filepath.Join(dir, "app.tui"))
	if err != nil {
		return nil, err
	}
	a := &app{ui: ui, g: newGame(seed), hist: make([]any, histLen)}
	if err := ui.Bind("", a.raw()); err != nil {
		return nil, err
	}
	for name, h := range a.handlers() {
		ui.On(name, h)
	}
	if err := a.derive(); err != nil {
		return nil, err
	}
	return a, nil
}

func run(o options) error {
	if o.theme != "" && o.theme != "dark" && o.theme != "light" {
		return fmt.Errorf("-theme wants dark or light, got %q", o.theme)
	}
	if o.demo != "" {
		cols, rows, err := parseSize("-demo", o.demo)
		if err != nil {
			return err
		}
		return demo(os.Stdout, o.dir, o.theme, cols, rows)
	}
	a, err := load(o.dir, o.seed)
	if err != nil {
		return err
	}
	if o.theme != "" {
		if err := a.ui.Set("@theme", o.theme); err != nil {
			return err
		}
	}
	if o.auto {
		a.auto = true
		if err := a.ui.Batch(a.pub); err != nil {
			return err
		}
	}
	if o.dump != "" {
		cols, rows, err := parseSize("-dump", o.dump)
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

	closeTicker := a.startTicker(a.ui.Every)
	defer closeTicker()

	// Run restores the terminal on SIGINT, SIGTERM, or SIGHUP and returns an
	// error naming the signal.
	return a.ui.RunWith(context.Background(), os.Stdout, tuimark.RunOptions{OnError: a.onError})
}

func parseSize(flagName, s string) (cols, rows int, err error) {
	if _, err := fmt.Sscanf(s, "%dx%d", &cols, &rows); err != nil || cols < 1 || rows < 1 {
		return 0, 0, fmt.Errorf("%s wants COLSxROWS, got %q", flagName, s)
	}
	return cols, rows, nil
}

// onError keeps the game going when a tick or a handler fails: one bad
// write is not a reason to end a game. The message goes to the top line
// until the next tick or action works. A quit still quits.
func (a *app) onError(_ *tuimark.Event, err error) error {
	if errors.Is(err, tuimark.ErrQuit) {
		return err
	}
	a.notice = err.Error()
	_ = a.ui.Set("notice", a.notice)
	return nil
}

// clearNotice takes the error off the top line once something works again.
func (a *app) clearNotice(s setter) error {
	if a.notice == "" {
		return nil
	}
	a.notice = ""
	return s.Set("notice", "")
}

// startTicker installs the Every registration through every (App.Every in
// run, a fake in tests): arm stops the registration it holds before it arms
// one of the new interval, so exactly one runs at a time. It arms the
// opening interval and returns the function that stops the last one.
func (a *app) startTicker(every func(time.Duration, func(*tuimark.Batch) error) func()) (closeTicker func()) {
	var stop func()
	a.arm = func(d time.Duration) {
		if stop != nil {
			stop()
		}
		a.armed = d
		stop = every(d, func(b *tuimark.Batch) error { return a.tick(b, time.Now()) })
	}
	a.rearm()
	return func() {
		if stop != nil {
			stop()
			stop = nil
		}
	}
}

// interval is the time between two moves at the current level.
func (a *app) interval() time.Duration { return time.Duration(a.g.intervalMs()) * time.Millisecond }

// rearm moves the Every registration to the current interval when the level
// changed it. Without run's arm (tests, the demo) it does nothing.
func (a *app) rearm() {
	if a.arm != nil && a.armed != a.interval() {
		a.arm(a.interval())
	}
}

// tick is the Every function: step on the real clock, then keep the ticker
// at the speed of the level.
func (a *app) tick(b *tuimark.Batch, now time.Time) error {
	if err := a.step(b, now); err != nil {
		return err
	}
	if err := a.clearNotice(b); err != nil {
		return err
	}
	a.rearm()
	return nil
}

// step is the game loop's body, with the clock handed in. It measures play
// time, lets the autopilot steer if it is on, moves the snake one cell,
// and publishes the result in the batch. A paused or finished game only
// keeps the clock.
func (a *app) step(b *tuimark.Batch, now time.Time) error {
	dt := time.Duration(0)
	if !a.last.IsZero() {
		dt = now.Sub(a.last)
	}
	a.last = now
	if a.paused || a.g.over {
		return nil
	}
	if dt > time.Second {
		dt = time.Second // a stall (suspended terminal) is not play time
	}
	a.elapsed += dt
	if a.auto {
		a.g.turns = nil
		a.g.steer(a.g.autoDir())
	}
	a.g.move()
	if !a.g.over {
		a.sample()
	}
	return a.publish(b)
}

// sample appends the current pace, in moves per second, to the sparkline's
// history.
func (a *app) sample() {
	pace := float64(int(10000/float64(a.g.intervalMs())+0.5)) / 10
	a.hist = append(a.hist[1:], pace)
}

// sync publishes the raw store in one batch.
func (a *app) sync() error { return a.ui.Batch(a.pub) }

// handlers are the named actions of app.tui.
func (a *app) handlers() map[string]tuimark.Handler {
	hs := map[string]tuimark.Handler{}
	// A handler that works also takes a pending error off the top line.
	on := func(name string, h tuimark.Handler) {
		hs[name] = func(ev tuimark.Event) error {
			if err := h(ev); err != nil {
				return err
			}
			return a.clearNotice(a.ui)
		}
	}
	on("quit", func(tuimark.Event) error { return tuimark.ErrQuit })
	for name, d := range map[string]point{"up": dirUp, "right": dirRight, "down": dirDown, "left": dirLeft} {
		d := d
		on(name, func(tuimark.Event) error { return a.steer(d) })
	}
	on("pause", func(tuimark.Event) error {
		if a.g.over || a.help {
			return nil
		}
		a.paused = !a.paused
		return a.sync()
	})
	on("restart", func(tuimark.Event) error {
		if a.help {
			return nil
		}
		a.g.restart()
		a.paused, a.helpPause = false, false
		a.elapsed, a.last = 0, time.Time{}
		a.hist = make([]any, histLen)
		if err := a.sync(); err != nil {
			return err
		}
		a.rearm()
		return nil
	})
	on("autopilot", func(tuimark.Event) error {
		if a.g.over || a.help {
			return nil
		}
		a.auto = !a.auto
		return a.sync()
	})
	on("help_open", func(tuimark.Event) error {
		if a.help {
			return a.closeHelp()
		}
		a.help = true
		if !a.paused && !a.g.over {
			a.paused, a.helpPause = true, true
		}
		return a.sync()
	})
	on("help_close", func(tuimark.Event) error { return a.closeHelp() })
	return hs
}

// closeHelp closes the help modal and resumes a game that opening it paused.
func (a *app) closeHelp() error {
	a.help = false
	if a.helpPause {
		a.paused, a.helpPause = false, false
	}
	return a.sync()
}

// steer queues a turn. Steering by hand takes the wheel from the autopilot.
func (a *app) steer(d point) error {
	if a.paused || a.g.over || a.help {
		return nil
	}
	if a.auto {
		a.auto = false
		a.g.turns = nil
		if err := a.sync(); err != nil {
			return err
		}
	}
	a.g.steer(d)
	return nil
}
