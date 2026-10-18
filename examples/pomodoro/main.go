// Command pomodoro runs examples/pomodoro/app.tui: a big-digit clock and
// Pomodoro timer. Two tabs: a work/short break/long break timer with cycle
// dots, a phase progress bar, a task name, and the list of completed
// sessions; and a clock with a small world-clock table.
//
//	go build -o bin/pomodoro ./examples/pomodoro && ./bin/pomodoro
//	go run ./examples/pomodoro -work 50m -short 10m -long 30m
//	go run ./examples/pomodoro -dump 90x24      # print Dump() as JSON and exit
//	go run ./examples/pomodoro -theme light     # Set("@theme", "light") before Run
//	go run ./examples/pomodoro -now 2026-10-17T14:34:07Z   # a fixed starting clock
//	go run ./examples/pomodoro -demo 90x24      # scripted frames, one JSON object per line
//
// The view lives entirely in app.tui and theme.tcss. This host holds no
// layout, width, padding, color, or truncation code. It keeps the timer
// (timer.go) and publishes plain data: an App.Every tick steps the timer
// with the clock and writes the raw state (the phase, the time left, the
// instant) in one Batch, and App.Derive turns that into everything the
// document shows: the big digits (font.go), the labels, the progress, the
// cycle dots, the class: flags, the world clock rows (derive.go). The
// colon blink and the last-seconds pulse are functions of the published
// instant, so they animate with no extra state.
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
	defaultWork  = 25 * time.Minute
	defaultShort = 5 * time.Minute
	defaultLong  = 15 * time.Minute
	defaultFlash = 1500 * time.Millisecond
	defaultTick  = 100 * time.Millisecond
)

func main() {
	var o options
	flag.StringVar(&o.dir, "dir", "examples/pomodoro", "directory with app.tui, theme.tcss, sample.json")
	flag.StringVar(&o.theme, "theme", "", "dark or light: Set(\"@theme\", …) before Run")
	flag.StringVar(&o.dump, "dump", "", "print Dump() as JSON at COLSxROWS and exit")
	flag.StringVar(&o.demo, "demo", "", "print the scripted demo at COLSxROWS: one {\"ms\",\"dump\"} JSON object per line")
	flag.StringVar(&o.now, "now", "", "start the clock at this RFC 3339 time (UTC zone; it still advances); for tests and screenshots")
	flag.DurationVar(&o.plan.work, "work", defaultWork, "length of a work phase")
	flag.DurationVar(&o.plan.short, "short", defaultShort, "length of a short break")
	flag.DurationVar(&o.plan.long, "long", defaultLong, "length of the long break (after four work phases)")
	flag.BoolVar(&o.auto, "auto", true, "start the next phase by itself; -auto=false waits for space")
	flag.DurationVar(&o.flash, "flash", defaultFlash, "how long the done flash lasts at the end of a phase")
	flag.BoolVar(&o.start, "start", false, "start the timer at once")
	flag.DurationVar(&o.tick, "tick", defaultTick, "time between clock ticks")
	flag.Parse()
	if err := run(o); err != nil {
		fmt.Fprintln(os.Stderr, "pomodoro:", err)
		os.Exit(1)
	}
}

type options struct {
	dir, theme, dump, demo, now string
	plan                        plan
	auto, start                 bool
	flash, tick                 time.Duration
}

// validate rejects values the timer cannot run with: a phase shorter than a
// second would let a catching-up tick loop for ever, and one of 100 minutes
// or more would not fit the two-digit minutes of the big font.
func (o options) validate() error {
	for _, p := range []struct {
		name string
		d    time.Duration
	}{{"-work", o.plan.work}, {"-short", o.plan.short}, {"-long", o.plan.long}} {
		if p.d < time.Second || p.d > maxPhase {
			return fmt.Errorf("%s wants a duration between 1s and %s, got %s", p.name, maxPhase, p.d)
		}
	}
	if o.flash < 0 {
		return fmt.Errorf("-flash wants a duration of 0 or more, got %s", o.flash)
	}
	if o.tick <= 0 {
		return fmt.Errorf("-tick wants a positive duration, got %s", o.tick)
	}
	if o.theme != "" && o.theme != "dark" && o.theme != "light" {
		return fmt.Errorf("-theme wants dark or light, got %q", o.theme)
	}
	return nil
}

// app is the host's state. Everything below runs on the runtime's loop:
// handlers and Every functions are called one at a time, so no lock.
type app struct {
	ui    *tuimark.App
	tm    *timer
	loc   *time.Location // the zone of the Local clock row and the session times
	nowFn func() time.Time

	published int    // sessions already written to the store
	notice    string // the error message shown in the top row; "" when none
}

// baseData is the part of the store the view owns: the host never writes
// it except through the handlers below. sample.json starts from it.
func baseData() map[string]any {
	return map[string]any{
		"tab":       "pomo",
		"task":      "",
		"help_open": false,
		"notice":    "",
	}
}

// newApp wires a document to a timer. base is the store's view-owned part
// (baseData, or sample.json's); the timer's keys are published by start.
func newApp(ui *tuimark.App, base map[string]any, p plan, auto bool, flash time.Duration, loc *time.Location, nowFn func() time.Time) (*app, error) {
	if err := ui.Bind("", base); err != nil {
		return nil, err
	}
	a := &app{ui: ui, tm: newTimer(p, auto, flash), loc: loc, nowFn: nowFn, published: -1}
	for name, h := range a.handlers() {
		ui.On(name, h)
	}
	if err := a.derive(); err != nil {
		return nil, err
	}
	return a, nil
}

// load reads dir/app.tui and dir/sample.json and builds the app on them.
func load(dir string, p plan, auto bool, flash time.Duration, loc *time.Location, nowFn func() time.Time) (*app, error) {
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
	// Only the view-owned keys of the sample are kept: the timer's own keys
	// and the derived ones are recomputed from the flags.
	base := baseData()
	for k := range base {
		if v, ok := data[k]; ok {
			base[k] = v
		}
	}
	base["task"] = ""
	return newApp(ui, base, p, auto, flash, loc, nowFn)
}

// start steps the timer to now for the first time and publishes it all.
func (a *app) start(running bool) error {
	now := a.nowFn()
	a.tm.step(now)
	if running {
		a.tm.toggle(now)
	}
	return a.ui.Batch(func(b *tuimark.Batch) error { return a.publish(b, now) })
}

// syncTask copies the task name the input holds into the timer, which
// stamps it on the work phase that ends next.
func (a *app) syncTask() {
	v, _ := a.ui.Get("task")
	a.tm.task, _ = v.(string)
}

// tick steps the timer to now and publishes it. It is the Every function
// and, with a fake clock, the one tests and the demo call.
func (a *app) tick(b *tuimark.Batch, now time.Time) error {
	a.syncTask()
	a.tm.step(now)
	return a.publish(b, now)
}

// publish writes the timer's raw state into the batch. The derived values
// follow by themselves.
func (a *app) publish(b *tuimark.Batch, now time.Time) error {
	t := a.tm
	name, off := now.In(a.loc).Zone()
	errs := []error{
		b.Set("pos", t.pos),
		b.Set("left_ms", t.left.Milliseconds()),
		b.Set("total_ms", t.total.Milliseconds()),
		b.Set("running", t.running),
		b.Set("flash_ms", t.done.Milliseconds()),
		b.Set("now_ms", now.UnixMilli()),
		b.Set("zone", map[string]any{"name": name, "off": off}),
		b.Set("auto", t.auto),
		b.Set("plan", map[string]any{
			"work_ms": t.plan.work.Milliseconds(), "short_ms": t.plan.short.Milliseconds(), "long_ms": t.plan.long.Milliseconds(),
		}),
	}
	if len(t.sessions) != a.published {
		errs = append(errs, b.Set("sessions", a.sessionRows()))
		a.published = len(t.sessions)
	}
	// A tick or an action that worked means whatever failed before is over.
	if err := errors.Join(errs...); err != nil || a.notice == "" {
		return err
	}
	a.notice = ""
	return b.Set("notice", "")
}

var sessionLabels = map[string]string{kindWork: "Work", kindShort: "Short break", kindLong: "Long break"}

// sessionRows is the completed sessions, newest first, as the store holds
// them.
func (a *app) sessionRows() []any {
	rows := make([]any, 0, len(a.tm.sessions))
	for i := len(a.tm.sessions) - 1; i >= 0; i-- {
		s := a.tm.sessions[i]
		task := s.task
		if s.kind == kindWork && task == "" {
			task = "(no task)"
		}
		rows = append(rows, map[string]any{
			"id": s.id, "kind": s.kind, "label": sessionLabels[s.kind],
			"start": s.start.In(a.loc).Format("15:04"),
			"dur":   mmss(int(s.dur / time.Second)), "secs": int(s.dur / time.Second),
			"task": task,
			"work": s.kind == kindWork, "short": s.kind == kindShort, "long": s.kind == kindLong,
		})
	}
	return rows
}

// handlers are the named actions of app.tui.
func (a *app) handlers() map[string]tuimark.Handler {
	hs := map[string]tuimark.Handler{}
	on := func(name string, h tuimark.Handler) { hs[name] = h }
	on("quit", func(tuimark.Event) error { return tuimark.ErrQuit })
	on("toggle", func(tuimark.Event) error { return a.act(func(now time.Time) { a.tm.toggle(now) }) })
	on("reset", func(tuimark.Event) error { return a.act(func(now time.Time) { a.tm.reset(now) }) })
	on("skip", func(tuimark.Event) error { return a.act(func(now time.Time) { a.tm.skip(now) }) })
	on("minute_add", func(tuimark.Event) error { return a.act(func(time.Time) { a.tm.addMinute() }) })
	on("minute_sub", func(tuimark.Event) error { return a.act(func(time.Time) { a.tm.subMinute() }) })
	on("task_done", func(tuimark.Event) error { return a.set("@focus", "#nav") })
	on("help_open", func(tuimark.Event) error { return a.set("help_open", true) })
	on("help_close", func(tuimark.Event) error { return a.set("help_open", false) })
	return hs
}

// set writes one path and, when that worked, clears a notice an earlier
// failure left.
func (a *app) set(path string, v any) error {
	if err := a.ui.Set(path, v); err != nil {
		return err
	}
	if a.notice == "" {
		return nil
	}
	a.notice = ""
	return a.ui.Set("notice", "")
}

// act settles the timer to now, applies f, and publishes the result in one
// batch, so a key never acts on stale time.
func (a *app) act(f func(now time.Time)) error {
	now := a.nowFn()
	return a.ui.Batch(func(b *tuimark.Batch) error {
		a.syncTask()
		a.tm.step(now)
		f(now)
		return a.publish(b, now)
	})
}

// onError keeps the session alive when a tick or a handler fails: the
// message goes to the top row and the clock goes on. The next tick or action
// that works clears it.
func (a *app) onError(ev *tuimark.Event, err error) error {
	a.notice = err.Error()
	_ = a.ui.Set("notice", a.notice)
	return nil
}

// parseSize reads COLSxROWS.
func parseSize(flagName, s string) (cols, rows int, err error) {
	if _, err := fmt.Sscanf(s, "%dx%d", &cols, &rows); err != nil || cols < 1 || rows < 1 {
		return 0, 0, fmt.Errorf("%s wants COLSxROWS, got %q", flagName, s)
	}
	return cols, rows, nil
}

func run(o options) error {
	if err := o.validate(); err != nil {
		return err
	}
	if o.demo != "" {
		return runDemo(o, os.Stdout)
	}
	loc, nowFn, err := clockFor(o.now)
	if err != nil {
		return err
	}
	a, err := load(o.dir, o.plan, o.auto, o.flash, loc, nowFn)
	if err != nil {
		return err
	}
	if o.theme != "" {
		if err := a.ui.Set("@theme", o.theme); err != nil {
			return err
		}
	}
	if err := a.start(o.start); err != nil {
		return err
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
	stop := a.ui.Every(o.tick, func(b *tuimark.Batch) error { return a.tick(b, a.nowFn()) })
	defer stop()
	// Run restores the terminal on SIGINT, SIGTERM, or SIGHUP and returns an
	// error naming the signal, so the command exits with a failure.
	return a.ui.RunWith(context.Background(), os.Stdout, tuimark.RunOptions{OnError: a.onError})
}

// clockFor returns the zone and the clock of a run: the machine's, or, with
// -now, a UTC clock that starts at that instant and advances in real time.
func clockFor(now string) (*time.Location, func() time.Time, error) {
	if now == "" {
		return time.Local, time.Now, nil
	}
	base, err := time.Parse(time.RFC3339, now)
	if err != nil {
		return nil, nil, fmt.Errorf("-now wants an RFC 3339 time such as 2026-10-17T14:34:07Z, got %q", now)
	}
	wall0 := time.Now()
	return time.UTC, func() time.Time { return base.Add(time.Since(wall0)) }, nil
}
