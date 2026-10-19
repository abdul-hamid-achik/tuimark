// Command tetris runs examples/tetris/app.tui: falling blocks in the
// terminal, a demonstration of App.Every as a game loop and of a board drawn
// from data.
//
//	go build -o bin/tetris ./examples/tetris && ./bin/tetris
//	go run ./examples/tetris -autopilot       # let the autopilot play
//	go run ./examples/tetris -seed 7          # another piece sequence
//	go run ./examples/tetris -level 5         # start at level 5
//	go run ./examples/tetris -theme light     # Set("@theme", "light") before Run
//	go run ./examples/tetris -dump 100x24     # print Dump() as JSON and exit
//	go run ./examples/tetris -demo 100x24     # the scripted autopilot run, one JSON line per frame
//
// The view lives entirely in app.tui and theme.tcss: the board is a nested
// container each over runs of same-kind cells, coloured by class: guards, and
// the hold and next panels are the same kind of rows. This host holds the
// rules (game.go, pieces.go, auto.go), publishes plain data, and implements
// the named actions. It has no layout, width, or colour code.
//
// The game loop is one App.Every at a fixed 50 ms. Gravity, the lock delay,
// and the line clear flash all count milliseconds on the game's own clock,
// which step advances by the time since the previous tick, so a level change
// takes effect on the very next tick and nothing re-arms the ticker.
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
	"strconv"
	"strings"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

const (
	defaultSeed  = 56
	tickMs       = 50  // the Every interval
	autoBaseMs   = 100 // time between two acts of the autopilot at -pace 1
	maxStall     = time.Second
	maxActsTick  = 20 // acts of the autopilot in one tick, however fast the pace
	minPace      = 0.1
	maxPace      = 10.0
	maxSizeCells = 1000
)

func main() {
	dir := flag.String("dir", "examples/tetris", "directory with app.tui and theme.tcss")
	theme := flag.String("theme", "", "dark or light: Set(\"@theme\", …) before Run")
	seed := flag.Int64("seed", defaultSeed, "seed of the piece sequence")
	level := flag.Int("level", 1, fmt.Sprintf("starting level, 1 to %d", maxLevel))
	pace := flag.Float64("pace", 1, fmt.Sprintf("autopilot speed, %g to %g times the normal one", minPace, maxPace))
	auto := flag.Bool("autopilot", false, "start with the autopilot playing")
	dumpSize := flag.String("dump", "", "print Dump() as JSON at COLSxROWS and exit")
	demoSize := flag.String("demo", "", "print the scripted autopilot demo at COLSxROWS, one {\"ms\",\"dump\"} JSON line per frame, and exit")
	flag.Parse()
	o := options{dir: *dir, theme: *theme, seed: *seed, level: *level, pace: *pace, auto: *auto, dump: *dumpSize, demo: *demoSize}
	if err := run(o); err != nil {
		fmt.Fprintln(os.Stderr, "tetris:", err)
		if errors.As(err, new(usageError)) {
			flag.Usage()
			os.Exit(2)
		}
		os.Exit(1)
	}
}

// usageError is a bad flag value: the command prints its usage and exits 2,
// as the flag package does for a value it cannot parse.
type usageError string

func (e usageError) Error() string { return string(e) }

type options struct {
	dir, theme, dump, demo string
	seed                   int64
	level                  int
	pace                   float64
	auto                   bool
}

// validate rejects what the game cannot run with (all usage errors, exit 2): a level outside the
// gravity table, a pace that is not a finite number in range (NaN would
// compare false with everything, and an infinite pace is no pace), a theme
// that is not one, and sizes that are not COLSxROWS.
func (o options) validate() error {
	if o.theme != "" && o.theme != "dark" && o.theme != "light" {
		return usageError(fmt.Sprintf("-theme wants dark or light, got %q", o.theme))
	}
	if o.level < 1 || o.level > maxLevel {
		return usageError(fmt.Sprintf("-level wants a number from 1 to %d, got %d", maxLevel, o.level))
	}
	if math.IsNaN(o.pace) || math.IsInf(o.pace, 0) || o.pace < minPace || o.pace > maxPace {
		return usageError(fmt.Sprintf("-pace wants a number from %g to %g, got %v", minPace, maxPace, o.pace))
	}
	for _, s := range []struct{ name, v string }{{"-dump", o.dump}, {"-demo", o.demo}} {
		if s.v == "" {
			continue
		}
		if _, _, err := parseSize(s.name, s.v); err != nil {
			return err
		}
	}
	if o.dump != "" && o.demo != "" {
		return usageError("-dump and -demo cannot be used together")
	}
	return nil
}

// autoMs is the time between two acts of the autopilot at a pace.
func autoMs(pace float64) int64 {
	ms := int64(math.Round(autoBaseMs / pace))
	if ms < 1 {
		ms = 1
	}
	return ms
}

// app is the host's state. Handlers and Every functions run one at a time
// on the runtime's loop, so there is no lock.
type app struct {
	ui *tuimark.App
	g  *game

	auto       bool
	paused     bool
	overHidden bool // the player looked at the board behind the game over modal
	help       bool // the help modal is open
	helpPause  bool // the help modal paused the game, so closing it resumes
	notice     string

	pl      plan  // where the autopilot is taking the piece
	autoMs  int64 // time between two acts of the autopilot
	autoAcc int64 // time since its last act

	last time.Time // the clock of the previous step, to measure play time
}

// load loads app.tui from dir and builds a new game. The store starts from
// the game, not from a file; sample.json is the same store for the tuimark
// CLI, kept in sync by a test.
func load(dir string, seed int64, level int, pace float64) (*app, error) {
	ui, err := tuimark.Load(filepath.Join(dir, "app.tui"))
	if err != nil {
		return nil, err
	}
	a := &app{ui: ui, g: newGame(seed, level), autoMs: autoMs(pace)}
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
	if err := o.validate(); err != nil {
		return err
	}
	if o.demo != "" {
		cols, rows, _ := parseSize("-demo", o.demo)
		return demo(os.Stdout, o.dir, o.theme, cols, rows)
	}
	a, err := load(o.dir, o.seed, o.level, o.pace)
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
		if err := a.sync(); err != nil {
			return err
		}
	}
	if o.dump != "" {
		cols, rows, _ := parseSize("-dump", o.dump)
		d, err := a.ui.Dump(cols, rows)
		if err != nil {
			return err
		}
		b, _ := json.MarshalIndent(d, "", "  ")
		fmt.Println(string(b))
		return nil
	}

	stop := a.ui.Every(tickMs*time.Millisecond, func(b *tuimark.Batch) error { return a.tick(b, time.Now()) })
	defer stop()

	// Run restores the terminal on SIGINT, SIGTERM, or SIGHUP and returns an
	// error naming the signal.
	return a.ui.RunWith(context.Background(), os.Stdout, tuimark.RunOptions{OnError: a.onError})
}

// parseSize reads COLSxROWS: two positive whole numbers, at most
// maxSizeCells each, and nothing else.
func parseSize(flagName, s string) (cols, rows int, err error) {
	bad := usageError(fmt.Sprintf("%s wants COLSxROWS (two numbers from 1 to %d), got %q", flagName, maxSizeCells, s))
	c, r, ok := strings.Cut(s, "x")
	if !ok || !digits(c) || !digits(r) {
		return 0, 0, bad
	}
	cols, e1 := strconv.Atoi(c)
	rows, e2 := strconv.Atoi(r)
	if e1 != nil || e2 != nil || cols < 1 || rows < 1 || cols > maxSizeCells || rows > maxSizeCells {
		return 0, 0, bad
	}
	return cols, rows, nil
}

func digits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
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

// tick is the Every function: step on the clock it is given, and clear a
// pending error once that worked.
func (a *app) tick(b *tuimark.Batch, now time.Time) error {
	if err := a.step(b, now); err != nil {
		return err
	}
	return a.clearNotice(b)
}

// step is the game loop's body, with the clock handed in. It advances the
// game's own clock by the time since the previous step (gravity, the lock
// delay, and the flash all count on it), lets the autopilot act if it is on,
// and publishes the result in the batch. A paused or finished game only keeps
// the wall clock, so a pause is not played time.
func (a *app) step(b setter, now time.Time) error {
	dt := time.Duration(0)
	if !a.last.IsZero() {
		dt = now.Sub(a.last)
	}
	a.last = now
	if a.paused || a.g.over {
		return nil
	}
	if dt > maxStall {
		dt = maxStall // a stall (a suspended terminal) is not play time
	}
	ms := dt.Milliseconds()
	a.g.advance(ms)
	if a.auto {
		a.autoAcc += ms
		acts := 0
		for a.autoAcc >= a.autoMs && acts < maxActsTick {
			a.autoAcc -= a.autoMs
			a.g.autoAct(&a.pl)
			acts++
		}
		if acts == maxActsTick {
			a.autoAcc = 0
		}
	}
	return a.publish(b)
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
	// A move is for the player: it takes the wheel from the autopilot, and
	// does nothing while the game is paused, over, or behind the help.
	move := func(f func() bool) tuimark.Handler {
		return func(tuimark.Event) error {
			if a.paused || a.help || a.g.over {
				return nil
			}
			a.takeWheel()
			f()
			return a.sync()
		}
	}
	on("quit", func(tuimark.Event) error { return tuimark.ErrQuit })
	on("left", move(func() bool { return a.g.shift(-1) }))
	on("right", move(func() bool { return a.g.shift(1) }))
	on("soft_drop", move(a.g.softDrop))
	on("rotate", move(func() bool { return a.g.rotate(1) }))
	on("rotate_back", move(func() bool { return a.g.rotate(-1) }))
	on("hard_drop", move(a.g.hardDrop))
	on("hold", move(a.g.holdPiece))
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
		a.paused, a.helpPause, a.overHidden = false, false, false
		a.pl, a.autoAcc, a.last = plan{}, 0, time.Time{}
		return a.sync()
	})
	// The game over modal's first button: put the modal away to see the final
	// board. A stray key cannot restart the game from here; r does.
	on("over_hide", func(tuimark.Event) error {
		if !a.g.over {
			return nil
		}
		a.overHidden = true
		return a.sync()
	})
	on("autopilot", func(tuimark.Event) error {
		if a.g.over || a.help {
			return nil
		}
		a.auto = !a.auto
		a.pl, a.autoAcc = plan{}, 0
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

// takeWheel switches the autopilot off when the player moves.
func (a *app) takeWheel() {
	if a.auto {
		a.auto = false
		a.pl, a.autoAcc = plan{}, 0
	}
}

// closeHelp closes the help modal and resumes a game that opening it paused.
func (a *app) closeHelp() error {
	a.help = false
	if a.helpPause {
		a.paused, a.helpPause = false, false
	}
	return a.sync()
}
