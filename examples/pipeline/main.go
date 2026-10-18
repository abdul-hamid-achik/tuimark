// Command pipeline runs examples/pipeline/app.tui: a CI pipeline runner in
// the spirit of GitHub Actions or Buildkite. Five stages (build, test, lint,
// package, deploy) run one after the other, with the jobs of a stage running
// in parallel. Each job shows a spinner while it runs, a progress bar, and
// its elapsed time; a log pane streams the selected job's output; a header
// shows the overall status and progress and a sparkline of running jobs.
//
//	go build -o bin/pipeline ./examples/pipeline && ./bin/pipeline
//	go run ./examples/pipeline -dump 120x30      # print Dump() as JSON and exit
//	go run ./examples/pipeline -theme light      # Set("@theme", "light") before Run
//	go run ./examples/pipeline -seed 3           # other durations and log lines
//	go run ./examples/pipeline -flaky=false      # no failing test: a green run
//	go run ./examples/pipeline -speed 8          # simulated seconds per real second
//	go run ./examples/pipeline -demo 100x24      # scripted frames as JSON lines, for the docs (ignores -seed, -flaky, -speed)
//
// The view lives entirely in app.tui and theme.tcss. This host holds no
// layout, width, color, or truncation code. It simulates the run (sim.go,
// plan.go: a state machine whose clock and random numbers are injected) and
// publishes plain data:
//
//   - Every: a 100ms tick calls tick(batch, time.Now()), which steps the
//     simulation and writes the jobs, the clock, and the sparkline history in
//     one Batch;
//   - Derive: the job rows (spinner frame, icon, labels), the overall status
//     and progress, and the selected job's log are recomputed inside every
//     store write, so moving the cursor changes the log pane with no handler;
//   - RunWith: a handler that fails lands in the status line and the session
//     goes on.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

const (
	defaultSeed  = 7
	defaultSpeed = 4.0
	tickStep     = 100 * time.Millisecond
)

func main() {
	var o options
	flag.StringVar(&o.dir, "dir", "examples/pipeline", "directory with app.tui, theme.tcss, sample.json")
	flag.StringVar(&o.dump, "dump", "", "print Dump() as JSON at COLSxROWS and exit")
	flag.StringVar(&o.demo, "demo", "", "print the scripted demo at COLSxROWS as JSON lines and exit")
	flag.StringVar(&o.theme, "theme", "", "dark or light: Set(\"@theme\", …) before Run")
	flag.Int64Var(&o.sim.seed, "seed", defaultSeed, "seed of the simulated durations and log lines")
	flag.BoolVar(&o.sim.flaky, "flaky", true, "the integration test fails on the first run (-flaky=false for a green run)")
	flag.Float64Var(&o.sim.speed, "speed", defaultSpeed, "simulated seconds per real second")
	flag.DurationVar(&o.interval, "interval", tickStep, "time between simulation ticks")
	flag.Parse()
	if err := run(o); err != nil {
		fmt.Fprintln(os.Stderr, "pipeline:", err)
		os.Exit(1)
	}
}

type options struct {
	dir, dump, demo, theme string
	sim                    simOptions
	interval               time.Duration
}

// simOptions are the knobs of the simulation.
type simOptions struct {
	seed  int64
	flaky bool
	speed float64
}

func defaultSim() simOptions { return simOptions{seed: defaultSeed, flaky: true, speed: defaultSpeed} }

// app is the host's state. Handlers and Every ticks run on the runtime's
// loop, one at a time, so none of this needs a lock; a Derive function
// never touches it (it runs under the app's lock and reads the store
// through get).
type app struct {
	ui  *tuimark.App
	sim *sim
}

// load loads app.tui from dir and starts the simulation at start. Nothing
// is read from sample.json: the host publishes the whole store (the
// pipeline's name, branch, and commit, the jobs, the clock, the history,
// and the selection), and sample.json is a snapshot of it for the CLI.
func load(dir string, so simOptions, start time.Time) (*app, error) {
	ui, err := tuimark.Load(filepath.Join(dir, "app.tui"))
	if err != nil {
		return nil, err
	}
	a := &app{ui: ui}
	if err := a.restart(so, start); err != nil {
		return nil, err
	}
	return a, nil
}

// pipelineMeta is what the header says about the run's source.
func pipelineMeta() map[string]any {
	return map[string]any{
		"name": "tuimark", "branch": "main", "commit": "a3f9c21",
		"message": "layout: clamp table fr columns", "run": float64(1),
	}
}

// restart replaces the simulation with a fresh one and publishes its first
// state, with the selection back on the first job.
func (a *app) restart(so simOptions, start time.Time) error {
	if so.speed <= 0 {
		so.speed = defaultSpeed
	}
	a.sim = newSim(so.seed, so.flaky, so.speed, start)
	return a.ui.Batch(func(b *tuimark.Batch) error {
		return errors.Join(
			b.Set("pipeline", pipelineMeta()),
			a.write(b),
			b.Set("cur", a.sim.jobs[0].ID),
			b.Set("follow", true),
			b.Set("freeze", 0),
			b.Set("confirm_open", false),
			b.Set("help_open", false),
			a.say(b, "Waiting for runners.", false),
		)
	})
}

func run(o options) error {
	if o.dir == "" {
		o.dir = "examples/pipeline"
	}
	if o.theme != "" && o.theme != "dark" && o.theme != "light" {
		return fmt.Errorf("-theme wants dark or light, got %q", o.theme)
	}
	if err := checkSpeed(o.sim.speed); err != nil {
		return err
	}
	if o.demo != "" {
		cols, rows, err := parseSize(o.demo, "-demo")
		if err != nil {
			return err
		}
		return demo(os.Stdout, o.dir, cols, rows, o.theme)
	}
	a, err := load(o.dir, o.sim, time.Now())
	if err != nil {
		return err
	}
	if err := a.register(); err != nil {
		return err
	}
	if o.theme != "" {
		if err := a.ui.Set("@theme", o.theme); err != nil {
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
	if o.interval <= 0 {
		o.interval = tickStep
	}
	a.sim.last = time.Now()
	stop := a.ui.Every(o.interval, func(b *tuimark.Batch) error { return a.tick(b, time.Now()) })
	defer stop()

	// Run restores the terminal on SIGINT, SIGTERM, or SIGHUP and returns
	// an error naming the signal, so the command exits with a failure.
	return a.ui.RunWith(context.Background(), os.Stdout, tuimark.RunOptions{OnError: a.onError})
}

// maxSpeed bounds -speed: a step simulates dt*speed seconds in slices of
// subStep, so a huge or infinite speed would never finish a tick.
const maxSpeed = 1000

// checkSpeed rejects a -speed the simulation cannot step: NaN, infinite,
// zero, negative, or above maxSpeed.
func checkSpeed(v float64) error {
	if math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 || v > maxSpeed {
		return fmt.Errorf("-speed wants a number above 0 and at most %d, got %v", maxSpeed, v)
	}
	return nil
}

func parseSize(s, flagName string) (cols, rows int, err error) {
	if _, err := fmt.Sscanf(s, "%dx%d", &cols, &rows); err != nil || cols < 1 || rows < 1 {
		return 0, 0, fmt.Errorf("%s wants COLSxROWS, got %q", flagName, s)
	}
	return cols, rows, nil
}

// onError keeps the session alive when a tick or a handler fails: the
// message goes to the status line and the run goes on.
func (a *app) onError(ev *tuimark.Event, err error) error {
	_ = a.ui.Set("status", map[string]any{"text": err.Error(), "err": true})
	return nil
}

// tick is the Every function: it moves the simulation to now and publishes
// the result in the tick's batch. It is the only place time enters the
// program, which is why tests and -demo can drive it with a fake clock.
func (a *app) tick(b *tuimark.Batch, now time.Time) error {
	if !a.sim.step(now) {
		return nil
	}
	return a.write(b)
}

// write publishes the simulation's state and any status message it queued.
func (a *app) write(b *tuimark.Batch) error {
	s := a.sim
	errs := []error{
		b.Set("jobs", s.jobsStore()),
		b.Set("clock", s.clockStore()),
		b.Set("spark", s.histStore()),
		b.Set("pipeline.run", float64(s.run)),
	}
	if evs := s.drain(); len(evs) > 0 {
		e := evs[len(evs)-1]
		errs = append(errs, a.say(b, e.Text, e.Err))
	}
	return errors.Join(errs...)
}

// say writes the status line.
func (a *app) say(b *tuimark.Batch, text string, isErr bool) error {
	return b.Set("status", map[string]any{"text": text, "err": isErr})
}

// register installs the handlers and the derived values.
func (a *app) register() error {
	for name, h := range a.handlers() {
		a.ui.On(name, h)
	}
	return a.derive()
}

// curID is the selected job's id; the first job before the list has put a
// key in the store.
func (a *app) curID() string {
	if v, _ := a.ui.Get("cur"); v != nil {
		if s, ok := v.(string); ok && a.sim.find(s) != nil {
			return s
		}
	}
	return a.sim.jobs[0].ID
}

// handlers are the named actions of app.tui.
func (a *app) handlers() map[string]tuimark.Handler {
	ui := a.ui
	hs := map[string]tuimark.Handler{}
	on := func(name string, h tuimark.Handler) { hs[name] = h }
	on("quit", func(tuimark.Event) error { return tuimark.ErrQuit })
	// A new selection shows its log live again.
	on("job_select", func(tuimark.Event) error { return ui.Set("follow", true) })
	on("job_retry", func(tuimark.Event) error {
		err := a.sim.retry(a.curID())
		return ui.Batch(func(b *tuimark.Batch) error {
			if err != nil {
				return a.say(b, err.Error(), true)
			}
			return a.write(b)
		})
	})
	on("run_rerun", func(tuimark.Event) error {
		a.sim.rerun()
		return ui.Batch(func(b *tuimark.Batch) error {
			return errors.Join(a.write(b), b.Set("follow", true), b.Set("freeze", 0))
		})
	})
	on("run_cancel", func(tuimark.Event) error {
		if a.sim.idle() {
			return ui.Set("status", map[string]any{"text": "Nothing to cancel, the run is over.", "err": false})
		}
		return ui.Set("confirm_open", true)
	})
	on("cancel_confirm", func(tuimark.Event) error {
		err := a.sim.cancel()
		return ui.Batch(func(b *tuimark.Batch) error {
			if err != nil {
				return errors.Join(a.say(b, err.Error(), true), b.Set("confirm_open", false))
			}
			return errors.Join(a.write(b), b.Set("confirm_open", false))
		})
	})
	on("cancel_abort", func(tuimark.Event) error { return ui.Set("confirm_open", false) })
	on("follow_toggle", func(tuimark.Event) error {
		v, _ := ui.Get("follow")
		if on, _ := v.(bool); on {
			// Freeze the pane at the lines it shows now.
			n := 0
			if j := a.sim.find(a.curID()); j != nil {
				n = len(j.log)
			}
			return ui.Batch(func(b *tuimark.Batch) error {
				return errors.Join(b.Set("freeze", n), b.Set("follow", false))
			})
		}
		return ui.Set("follow", true)
	})
	on("help_open", func(tuimark.Event) error { return ui.Set("help_open", true) })
	on("help_close", func(tuimark.Event) error { return ui.Set("help_open", false) })
	return hs
}
