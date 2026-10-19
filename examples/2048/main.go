// Command 2048 runs examples/2048/app.tui: the sliding-tiles game, a
// demonstration of nested container each, class: guards, and App.Every.
//
//	go build -o bin/2048 ./examples/2048 && ./bin/2048
//	go run ./examples/2048 -size 5            # a 5x5 board (3 to 6)
//	go run ./examples/2048 -autopilot         # let the expectimax autopilot play
//	go run ./examples/2048 -seed 7            # another run of tiles
//	go run ./examples/2048 -theme light       # Set("@theme", "light") before Run
//	go run ./examples/2048 -dump 90x24        # print Dump() as JSON and exit
//	go run ./examples/2048 -demo 90x24        # the scripted autopilot run, one JSON line per frame
//
// The view lives entirely in app.tui and theme.tcss: the board is a nested
// container each over rows of tiles, coloured by class: guards. This host
// holds the rules (game.go, auto.go), publishes plain data, and implements
// the named actions. It has no layout, width, or colour code.
//
// One App.Every ticks every 50 ms. Each tick clears the pop and new-tile
// highlights a move set once their time is up, and moves the autopilot when
// it is due. Every callback is tick, a pure function of the app and the clock
// it is handed.
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
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

const (
	defaultSeed  = 2
	defaultSpeed = 150 * time.Millisecond // between two autopilot moves
	defaultFlash = 200 * time.Millisecond // how long the highlights of a move last
	defaultDepth = 2
	tickEvery    = 50 * time.Millisecond
	noticeTTL    = 3 * time.Second
)

// config is everything a game is built from; the zero value is not valid,
// use defaultConfig.
type config struct {
	size  int
	seed  int64
	speed time.Duration
	flash time.Duration
	depth int
	p4    float64
}

func defaultConfig() config {
	return config{size: defaultSize, seed: defaultSeed, speed: defaultSpeed, flash: defaultFlash, depth: defaultDepth, p4: defaultP4}
}

type options struct {
	dir, theme, dump, demo string
	cfg                    config
	// speedMs and flashMs are the flags as typed: they are range-checked as
	// integers before they become durations, so a huge value cannot wrap.
	speedMs, flashMs int
	auto             bool
	gameFlags        []string // the game flags the command line set, for -demo to refuse
}

func defaultOptions() options {
	return options{
		cfg:     defaultConfig(),
		speedMs: int(defaultSpeed / time.Millisecond),
		flashMs: int(defaultFlash / time.Millisecond),
	}
}

// gameFlagNames are the flags that shape a game; -demo plays a fixed one.
var gameFlagNames = map[string]bool{"seed": true, "size": true, "autopilot": true, "speed": true, "flash": true, "depth": true, "chance4": true}

func main() {
	o := defaultOptions()
	flag.StringVar(&o.dir, "dir", "examples/2048", "directory with app.tui and theme.tcss")
	flag.StringVar(&o.theme, "theme", "", "dark or light: Set(\"@theme\", …) before Run")
	flag.Int64Var(&o.cfg.seed, "seed", defaultSeed, "seed of the tile spawns")
	flag.IntVar(&o.cfg.size, "size", defaultSize, "board size, 3 to 6")
	flag.BoolVar(&o.auto, "autopilot", false, "start with the autopilot playing")
	flag.IntVar(&o.speedMs, "speed", o.speedMs, "milliseconds between autopilot moves, 50 to 5000 (it moves on the 50 ms tick, so the time rounds up to a multiple of 50)")
	flag.IntVar(&o.flashMs, "flash", o.flashMs, "milliseconds a move's highlights last, 0 to 2000 (they clear on the 50 ms tick, so the time rounds up to a multiple of 50)")
	flag.IntVar(&o.cfg.depth, "depth", defaultDepth, "autopilot search depth, 1 to 3")
	flag.Float64Var(&o.cfg.p4, "chance4", defaultP4, "chance that a new tile is a 4, 0 to 1")
	flag.StringVar(&o.dump, "dump", "", "print Dump() as JSON at COLSxROWS and exit")
	flag.StringVar(&o.demo, "demo", "", "print the scripted autopilot demo at COLSxROWS, one {\"ms\",\"dump\"} JSON line per frame, and exit")
	flag.Parse()
	flag.Visit(func(f *flag.Flag) {
		if gameFlagNames[f.Name] {
			o.gameFlags = append(o.gameFlags, "-"+f.Name)
		}
	})
	if err := run(o); err != nil {
		fmt.Fprintln(os.Stderr, "2048:", err)
		if errors.As(err, new(usageError)) {
			flag.Usage()
			os.Exit(2)
		}
		os.Exit(1)
	}
}

// usageError is a bad flag value.
type usageError string

func (e usageError) Error() string { return string(e) }

// validate rejects flag values the game cannot run with: out of range, NaN,
// or infinite.
func (o options) validate() error {
	c := o.cfg
	switch {
	case o.theme != "" && o.theme != "dark" && o.theme != "light":
		return usageError(fmt.Sprintf("-theme wants dark or light, got %q", o.theme))
	case c.size < minSize || c.size > maxSize:
		return usageError(fmt.Sprintf("-size wants %d to %d, got %d", minSize, maxSize, c.size))
	case o.speedMs < 50 || o.speedMs > 5000:
		return usageError(fmt.Sprintf("-speed wants 50 to 5000 milliseconds, got %d", o.speedMs))
	case o.flashMs < 0 || o.flashMs > 2000:
		return usageError(fmt.Sprintf("-flash wants 0 to 2000 milliseconds, got %d", o.flashMs))
	case c.depth < 1 || c.depth > 3:
		return usageError(fmt.Sprintf("-depth wants 1 to 3, got %d", c.depth))
	case math.IsNaN(c.p4) || math.IsInf(c.p4, 0) || c.p4 < 0 || c.p4 > 1:
		return usageError(fmt.Sprintf("-chance4 wants a number from 0 to 1, got %v", c.p4))
	case o.demo != "" && len(o.gameFlags) > 0:
		return usageError(fmt.Sprintf("-demo plays a fixed game and does not take %s", strings.Join(o.gameFlags, ", ")))
	}
	return nil
}

// app is the host's state. Handlers and Every functions run one at a time
// on the runtime's loop, so there is no lock.
type app struct {
	ui  *tuimark.App
	g   *game
	cfg config

	// clock is the time handlers stamp a move's highlights with: time.Now
	// in Run, a fake in tests and the demo.
	clock func() time.Time

	auto     bool
	help     bool // the help modal is open
	winOpen  bool // the 2048 modal is open
	overOpen bool // the game over modal is open

	notice   string // the last error a tick or a handler returned, until something works
	noticeAt time.Time

	flashUntil time.Time // when the highlights of the last move clear; zero when none
	nextAuto   time.Time // when the autopilot moves next; zero means at once
}

// load loads app.tui from dir and builds a new game from cfg. The store
// starts from the game, not from a file; sample.json is the same store for
// the tuimark CLI, kept in sync by a test.
func load(dir string, cfg config) (*app, error) {
	ui, err := tuimark.Load(filepath.Join(dir, "app.tui"))
	if err != nil {
		return nil, err
	}
	a := &app{ui: ui, g: newGame(cfg.size, cfg.seed, cfg.p4), cfg: cfg, clock: time.Now}
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
	o.cfg.speed = time.Duration(o.speedMs) * time.Millisecond
	o.cfg.flash = time.Duration(o.flashMs) * time.Millisecond
	if o.demo != "" {
		cols, rows, err := parseSize("-demo", o.demo)
		if err != nil {
			return usageError(err.Error())
		}
		return demo(os.Stdout, o.dir, o.theme, cols, rows)
	}
	var dumpCols, dumpRows int
	if o.dump != "" {
		var err error
		if dumpCols, dumpRows, err = parseSize("-dump", o.dump); err != nil {
			return usageError(err.Error())
		}
	}
	a, err := load(o.dir, o.cfg)
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
		d, err := a.ui.Dump(dumpCols, dumpRows)
		if err != nil {
			return err
		}
		b, _ := json.MarshalIndent(d, "", "  ")
		fmt.Println(string(b))
		return nil
	}

	stop := a.ui.Every(tickEvery, func(b *tuimark.Batch) error { return a.tick(b, time.Now()) })
	defer stop()

	// Run restores the terminal on SIGINT, SIGTERM, or SIGHUP and returns an
	// error naming the signal.
	return a.ui.RunWith(context.Background(), os.Stdout, tuimark.RunOptions{OnError: a.onError})
}

// maxTerminal is the largest dimension Dump accepts.
const maxTerminal = 1000

var sizePattern = regexp.MustCompile(`^([0-9]+)x([0-9]+)$`)

// parseSize reads COLSxROWS, nothing else around it, each from 1 to 1000.
func parseSize(flagName, s string) (cols, rows int, err error) {
	m := sizePattern.FindStringSubmatch(s)
	if m != nil {
		cols, _ = strconv.Atoi(m[1])
		rows, _ = strconv.Atoi(m[2])
	}
	if m == nil || cols < 1 || rows < 1 || cols > maxTerminal || rows > maxTerminal {
		return 0, 0, fmt.Errorf("%s wants COLSxROWS with each from 1 to %d, got %q", flagName, maxTerminal, s)
	}
	return cols, rows, nil
}

// onError keeps the game going when a tick or a handler fails: one bad
// write is not a reason to end a game. The message goes to the top line
// until the next action that works, or until a tick finds it noticeTTL old.
// A quit still quits.
func (a *app) onError(_ *tuimark.Event, err error) error {
	if errors.Is(err, tuimark.ErrQuit) {
		return err
	}
	a.notice = err.Error()
	a.noticeAt = a.clock()
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

// modalOpen reports whether a modal holds the keyboard.
func (a *app) modalOpen() bool { return a.help || a.winOpen || a.overOpen }

// doMove slides d at time now, stamps the highlights, and opens the win or
// game over modal the move led to. It reports whether anything moved.
func (a *app) doMove(d dir, now time.Time) bool {
	if !a.g.move(d) {
		return false
	}
	a.flashUntil = now.Add(a.cfg.flash)
	switch {
	case a.g.over:
		// A move that makes 2048 and ends the game opens the game over
		// modal only (the win is recorded): undo stays reachable.
		a.overOpen = true
		a.auto = false // a finished game hands the board back
	case a.g.reachedWin && !a.g.keep:
		a.winOpen = true
	}
	return true
}

// tick is the Every function, with the clock handed in: it takes an old
// notice off the top line, clears the highlights once their time is up, and
// plays the autopilot's move when it is due. It publishes only when
// something changed.
func (a *app) tick(b *tuimark.Batch, now time.Time) error {
	dirty := false
	if a.notice != "" && now.Sub(a.noticeAt) >= noticeTTL {
		a.notice = ""
		dirty = true
	}
	if !a.flashUntil.IsZero() && !now.Before(a.flashUntil) {
		a.g.clearFlash()
		a.flashUntil = time.Time{}
		dirty = true
	}
	if a.auto && !a.modalOpen() && !a.g.over && !now.Before(a.nextAuto) {
		a.nextAuto = now.Add(a.cfg.speed)
		if d, ok := bestMove(a.g.n, a.g.cells, a.cfg.depth); ok && a.doMove(d, now) {
			dirty = true
		}
	}
	if !dirty {
		return nil
	}
	return a.publish(b)
}

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
	for _, d := range []dir{dirUp, dirRight, dirDown, dirLeft} {
		d := d
		on(d.String(), func(tuimark.Event) error { return a.slide(d) })
	}
	on("undo", func(tuimark.Event) error {
		if a.help || a.winOpen || !a.g.undo() {
			return nil
		}
		a.overOpen = false
		a.flashUntil = time.Time{}
		a.auto = false // undoing hands the wheel back, like a slide by hand
		return a.sync()
	})
	on("restart", func(tuimark.Event) error {
		if a.help {
			return nil
		}
		a.g.reset()
		a.winOpen, a.overOpen = false, false
		a.flashUntil, a.nextAuto = time.Time{}, time.Time{}
		return a.sync()
	})
	on("autopilot", func(tuimark.Event) error {
		if a.modalOpen() || a.g.over {
			return nil
		}
		a.auto = !a.auto
		a.nextAuto = time.Time{}
		return a.sync()
	})
	on("help_open", func(tuimark.Event) error {
		if a.winOpen || a.overOpen { // those modals own the keyboard
			return nil
		}
		a.help = !a.help
		return a.sync()
	})
	on("help_close", func(tuimark.Event) error {
		a.help = false
		return a.sync()
	})
	on("keep_going", func(tuimark.Event) error {
		a.g.keep = true
		a.winOpen = false
		return a.sync()
	})
	on("over_close", func(tuimark.Event) error {
		a.overOpen = false
		return a.sync()
	})
	return hs
}

// slide is a hand-played move. Taking the wheel turns the autopilot off.
func (a *app) slide(d dir) error {
	if a.modalOpen() || a.g.over {
		return nil
	}
	a.auto = false
	a.doMove(d, a.clock())
	return a.sync()
}
