// Command chess runs examples/chess/app.tui: a full chess game for the
// terminal, a demonstration of a clickable 64-cell board, App.Every as a
// clock, and a search that runs off the loop and applies its result with
// App.Batch.
//
//	go build -o bin/chess ./examples/chess && ./bin/chess
//	go run ./examples/chess -mode 2p                 # two players at one keyboard
//	go run ./examples/chess -mode cvc -level 2       # watch the computer play itself
//	go run ./examples/chess -side black -level 5     # play black against level 5
//	go run ./examples/chess -clock 3+2               # a 3 minute clock, 2 seconds a move
//	go run ./examples/chess -fen "8/8/8/4k3/8/4K3/4P3/8 w - - 0 1"
//	go run ./examples/chess -pgn game.pgn            # step through a game with the arrows
//	go run ./examples/chess -save game.pgn           # p writes the game there
//	go run ./examples/chess -letters                 # K Q R B N P instead of figurines
//	go run ./examples/chess -theme light             # Set("@theme", "light") before Run
//	go run ./examples/chess -dump 100x24             # print Dump() as JSON and exit
//	go run ./examples/chess -demo 100x24             # the Opera Game, one JSON line per frame
//
// The view lives entirely in app.tui and theme.tcss: the board is 64 static
// cells with their own ids (an each template cannot take on:click), coloured
// by class: guards. This host holds the rules and the engine (board.go,
// movegen.go, san.go, search.go, …), publishes plain data, and implements
// the named actions. It has no layout, width, or colour code.
//
// The computer thinks off the loop: a search runs in a goroutine on a copy of
// the position, tagged with a generation number, and applies its move with
// App.Batch; an undo, a new game, or a mode change bumps the generation, so a
// result that arrives late is dropped. One App.Every ticks the clocks, animates
// the thinking indicator, and paces the computer when it plays itself.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

const (
	defaultSeed  = 1
	defaultLevel = 3
)

func main() {
	var o options
	fs := newFlagSet(&o)
	_ = fs.Parse(os.Args[1:]) // ExitOnError
	fs.Visit(func(f *flag.Flag) { o.modeSet = o.modeSet || f.Name == "mode" })
	if err := run(o); err != nil {
		fmt.Fprintln(os.Stderr, "chess:", err)
		os.Exit(1)
	}
}

// newFlagSet declares the flags into o.
func newFlagSet(o *options) *flag.FlagSet {
	fs := flag.NewFlagSet("chess", flag.ExitOnError)
	fs.StringVar(&o.dir, "dir", "examples/chess", "directory with app.tui and theme.tcss")
	fs.StringVar(&o.theme, "theme", "", "dark or light: Set(\"@theme\", …) before Run")
	fs.Int64Var(&o.seed, "seed", defaultSeed, "seed of the random choice of side")
	fs.StringVar(&o.dump, "dump", "", "print Dump() as JSON at COLSxROWS and exit")
	fs.StringVar(&o.demo, "demo", "", "print the scripted Opera Game at COLSxROWS, one {\"ms\",\"dump\"} JSON line per frame, and exit")
	fs.StringVar(&o.mode, "mode", modeHvC, "hvc (you against the computer), 2p (two players), or cvc (watch)")
	fs.StringVar(&o.side, "side", "white", "your side against the computer: white, black, or random")
	fs.IntVar(&o.level, "level", defaultLevel, "the computer's level, 1 to 6")
	fs.StringVar(&o.clock, "clock", "none", "time control MINUTES+SECONDS (5+0, 3+2, 10+5) or none")
	fs.StringVar(&o.fen, "fen", "", "start from this position")
	fs.StringVar(&o.pgn, "pgn", "", "load this PGN file and review it")
	fs.StringVar(&o.save, "save", "", "where p writes the game as PGN (without it p shows the PGN)")
	fs.BoolVar(&o.letters, "letters", false, "show pieces as letters (K Q R B N P) for fonts without chess glyphs")
	return fs
}

type options struct {
	dir, theme, dump, demo, mode, side, clock, fen, pgn, save string
	seed                                                      int64
	level                                                     int
	letters                                                   bool
	modeSet                                                   bool // -mode was given
}

// defaults is the options of a plain start, for tests and the demo.
func defaults() options {
	return options{dir: ".", mode: modeHvC, side: "white", level: defaultLevel, clock: "none", seed: defaultSeed}
}

// validate checks every option and returns a usage error for a bad one.
func (o options) validate() (settings, error) {
	var s settings
	if o.theme != "" && o.theme != "dark" && o.theme != "light" {
		return s, fmt.Errorf("-theme wants dark or light, got %q", o.theme)
	}
	switch o.mode {
	case modeHvC, mode2P, modeCvC:
	default:
		return s, fmt.Errorf("-mode wants hvc, 2p, or cvc, got %q", o.mode)
	}
	if o.level < 1 || o.level > 6 {
		return s, fmt.Errorf("-level wants 1 to 6, got %d", o.level)
	}
	human := White
	switch o.side {
	case "white":
	case "black":
		human = Black
	case "random":
		if rand.New(rand.NewSource(o.seed)).Intn(2) == 1 {
			human = Black
		}
	default:
		return s, fmt.Errorf("-side wants white, black, or random, got %q", o.side)
	}
	clock, err := parseClock(o.clock)
	if err != nil {
		return s, fmt.Errorf("-clock: %w", err)
	}
	if o.fen != "" && o.pgn != "" {
		return s, errors.New("-fen and -pgn cannot be used together")
	}
	return settings{mode: o.mode, human: human, level: o.level, clock: clock}, nil
}

func run(o options) error {
	s, err := o.validate()
	if err != nil {
		return err
	}
	if o.demo != "" {
		cols, rows, err := parseSize("-demo", o.demo)
		if err != nil {
			return err
		}
		return demo(os.Stdout, o.dir, o.theme, cols, rows)
	}
	var dumpCols, dumpRows int
	if o.dump != "" {
		if dumpCols, dumpRows, err = parseSize("-dump", o.dump); err != nil {
			return err
		}
	}
	a, err := newApp(o, s)
	if err != nil {
		return err
	}
	if o.theme != "" {
		if err := a.ui.Set("@theme", o.theme); err != nil {
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

	// A search runs in its own goroutine, and a quick level still takes
	// minThink to answer.
	a.dispatch = func(f func()) { go f() }
	a.thinkDelay = minThink
	stop := a.ui.Every(tickStep, func(*tuimark.Batch) error { return a.tick() })
	defer stop()
	defer a.shutdown()

	// Run restores the terminal on SIGINT, SIGTERM, or SIGHUP and returns an
	// error naming the signal.
	return a.ui.RunWith(context.Background(), os.Stdout, tuimark.RunOptions{OnError: a.onError})
}

// parseSize reads COLSxROWS, both plain decimal numbers from 1 to 1000.
func parseSize(flagName, s string) (cols, rows int, err error) {
	bad := fmt.Errorf("%s wants COLSxROWS (each 1 to 1000), got %q", flagName, s)
	w, h, ok := strings.Cut(s, "x")
	if !ok || !plainNumber(w) || !plainNumber(h) {
		return 0, 0, bad
	}
	cols, _ = strconv.Atoi(w)
	rows, _ = strconv.Atoi(h)
	if cols < 1 || rows < 1 || cols > 1000 || rows > 1000 {
		return 0, 0, bad
	}
	return cols, rows, nil
}

// plainNumber reports whether s is one to four ASCII digits.
func plainNumber(s string) bool {
	if s == "" || len(s) > 4 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// newApp loads app.tui from o.dir and starts the game the options describe.
func newApp(o options, s settings) (*app, error) {
	ui, err := tuimark.Load(filepath.Join(o.dir, "app.tui"))
	if err != nil {
		return nil, err
	}
	a := &app{
		ui: ui, now: time.Now, rng: rand.New(rand.NewSource(o.seed)),
		mouse: true, letters: o.letters, save: o.save,
		evals: map[uint64]int{},
	}
	a.dispatch = func(f func()) { f() }
	start := StartPosition()
	var g *Game
	switch {
	case o.fen != "":
		if start, err = ParseFEN(o.fen); err != nil {
			return nil, fmt.Errorf("-fen: %w", err)
		}
	case o.pgn != "":
		text, err := os.ReadFile(o.pgn)
		if err != nil {
			return nil, err
		}
		if g, err = ParsePGN(string(text)); err != nil {
			return nil, fmt.Errorf("-pgn: %w", err)
		}
		if o.mode == modeHvC && !o.modeSet {
			s.mode = mode2P
		}
	}
	a.startGame(s, start, g)
	a.ng = ngDraft{mode: s.mode, side: s.human.String(), level: s.level, clock: s.clock.String()}
	if g != nil {
		a.enterReview(0)
	}
	if err := ui.Bind("", a.raw()); err != nil {
		return nil, err
	}
	if err := a.register(); err != nil {
		return nil, err
	}
	return a, nil
}

// onError keeps the game going when a handler or a tick fails: the message
// goes to the notice until the next action or a few seconds pass. A quit
// still quits.
func (a *app) onError(_ *tuimark.Event, err error) error {
	if errors.Is(err, tuimark.ErrQuit) {
		return err
	}
	a.mu.Lock()
	a.say(err.Error())
	_ = a.writeLocked()
	a.mu.Unlock()
	return nil
}

// tick is the Every function: it charges the clock, animates the thinking
// indicator, ages the notice, and, when the computer plays itself, starts
// the next search once the pause is over.
func (a *app) tick() error {
	a.mu.Lock()
	changed := a.tickClock(a.now())
	if a.thinking {
		a.spin++
		changed = true
	}
	if a.noticeTicks > 0 {
		a.noticeTicks--
		if a.noticeTicks == 0 {
			a.notice = ""
		}
		changed = true
	}
	j := a.takeJob()
	if j != nil {
		changed = true
	}
	var err error
	if changed {
		err = a.writeLocked()
	}
	a.mu.Unlock()
	if j != nil {
		a.dispatch(func() { a.runJob(j) })
	}
	return err
}

// shutdown stops a search still running.
func (a *app) shutdown() {
	a.mu.Lock()
	a.invalidate()
	a.mu.Unlock()
}
