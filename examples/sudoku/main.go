// Command sudoku runs examples/sudoku/app.tui: Sudoku with a generator,
// pencil marks, hints, undo, and a clock.
//
//	go build -o bin/sudoku ./examples/sudoku && ./bin/sudoku
//	go run ./examples/sudoku -difficulty hard      # easy, medium (default), hard, or expert
//	go run ./examples/sudoku -seed 7               # another puzzle, the same every time
//	go run ./examples/sudoku -puzzle 53007...      # 81 characters, 0 or . for a blank
//	go run ./examples/sudoku -theme light          # Set("@theme", "light") before Run
//	go run ./examples/sudoku -dump 80x24           # print Dump() as JSON and exit
//	go run ./examples/sudoku -demo 90x24           # the logic solver plays a puzzle, one JSON line per frame
//
// The view lives entirely in app.tui and theme.tcss: 81 clickable cells with
// ids by screen position, coloured by class: guards. This host holds the
// rules (game.go), the generator and the solvers (generate.go, solver.go,
// logic.go), publishes plain data (store.go), and implements the named
// actions. It has no layout, width, or colour code.
//
// The clock is one App.Every. Its function is step, a pure function of the
// app and the time it is handed, so tests and the demo drive it with a fake
// clock. All randomness comes from rand.New(rand.NewSource(seed)).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

const (
	defaultSeed = 1
	tickEvery   = 250 * time.Millisecond
	winDelay    = 700 * time.Millisecond // how long the finished board shows before the win modal
	maxSize     = 1000                   // the largest -dump and -demo side
	minCols     = 40                     // the smallest screen the board is drawn on
	minRows     = 14
)

func main() {
	dir := flag.String("dir", "examples/sudoku", "directory with app.tui and theme.tcss")
	theme := flag.String("theme", "", "dark or light: Set(\"@theme\", …) before Run")
	seed := flag.Int64("seed", defaultSeed, "seed of the generator; each new game in a session uses the next seed")
	difficulty := flag.String("difficulty", "medium", "easy, medium, hard, or expert")
	givens := flag.String("puzzle", "", "81 characters, 0 or . for a blank; refused unless it has exactly one solution")
	dumpSize := flag.String("dump", "", "print Dump() as JSON at COLSxROWS and exit")
	demoSize := flag.String("demo", "", "print the scripted demo at COLSxROWS, one {\"ms\",\"dump\"} JSON line per frame, and exit")
	flag.Parse()
	set := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { set[f.Name] = true })
	o := options{dir: *dir, theme: *theme, seed: *seed, difficulty: *difficulty, puzzle: *givens,
		dump: *dumpSize, demo: *demoSize, difficultySet: set["difficulty"], seedSet: set["seed"]}
	if err := run(o); err != nil {
		fmt.Fprintln(os.Stderr, "sudoku:", err)
		os.Exit(1)
	}
}

type options struct {
	dir, theme, difficulty, puzzle, dump, demo string
	seed                                       int64
	difficultySet, seedSet                     bool // the flag was given, whatever its value
}

// app is the host's state. Handlers and Every functions run one at a time
// on the runtime's loop, so there is no lock; a Derive function never
// touches it.
type app struct {
	ui *tuimark.App
	g  *game

	seed  int64 // the seed of the first game; game n uses seed+n
	games int   // games started so far

	elapsed  time.Duration // play time: runs only while the board is in play
	last     time.Time     // the clock of the previous step
	paused   bool
	helpOpen bool
	newOpen  bool
	winOpen  bool
	winShown bool          // the win modal was shown for this game
	winWait  time.Duration // how long the finished board has been showing
	mouse    bool

	info   string // the last thing a hint or a check said, until the next action
	notice string // the last error a tick or a handler returned, until something works
}

// load loads app.tui from dir and starts p. The store starts from the
// game; sample.json is the same store for the tuimark CLI, kept in sync by
// a test.
func load(dir string, p puzzle, seed int64) (*app, error) {
	ui, err := tuimark.Load(filepath.Join(dir, "app.tui"))
	if err != nil {
		return nil, err
	}
	a := &app{ui: ui, g: newGame(p), seed: seed, games: 1, mouse: true}
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

// parseSize reads COLSxROWS for -dump and -demo: two plain decimal numbers,
// each from 1 to maxSize. A sign, a fraction, NaN, Inf, or anything else is
// a usage error.
func parseSize(flagName, s string) (cols, rows int, err error) {
	bad := fmt.Errorf("-%s wants COLSxROWS with each side from 1 to %d, got %q", flagName, maxSize, s)
	c, r, ok := strings.Cut(s, "x")
	if !ok || !plainNumber(c) || !plainNumber(r) {
		return 0, 0, bad
	}
	cols, err1 := strconv.Atoi(c)
	rows, err2 := strconv.Atoi(r)
	if err1 != nil || err2 != nil || cols < 1 || rows < 1 || cols > maxSize || rows > maxSize {
		return 0, 0, bad
	}
	return cols, rows, nil
}

// plainNumber reports whether s is one or more ASCII digits.
func plainNumber(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// startPuzzle is the puzzle the options ask for.
func (o options) startPuzzle() (puzzle, error) {
	if o.puzzle != "" {
		if o.difficultySet {
			return puzzle{}, errors.New("-puzzle and -difficulty exclude each other: a typed puzzle is graded, not generated")
		}
		p, err := fromGivens(strings.TrimSpace(o.puzzle))
		if err != nil {
			return puzzle{}, fmt.Errorf("-puzzle: %w", err)
		}
		return p, nil
	}
	level := levelOf(o.difficulty)
	if level == 0 {
		return puzzle{}, fmt.Errorf("-difficulty wants easy, medium, hard, or expert, got %q", o.difficulty)
	}
	return generateCached(o.seed, level), nil
}

func run(o options) error {
	if o.theme != "" && o.theme != "dark" && o.theme != "light" {
		return fmt.Errorf("-theme wants dark or light, got %q", o.theme)
	}
	if o.demo != "" {
		// The demo plays its own puzzle at a size the board fits: say so
		// before any output, not after some frames.
		if o.puzzle != "" || o.difficultySet || o.seedSet || o.dump != "" {
			return errors.New("-demo plays its own puzzle: it takes no -seed, -difficulty, -puzzle, or -dump")
		}
		cols, rows, err := parseSize("demo", o.demo)
		if err != nil {
			return err
		}
		if cols < minCols || rows < minRows {
			return fmt.Errorf("-demo wants at least %dx%d, the smallest screen the board is drawn on, got %q", minCols, minRows, o.demo)
		}
		return demo(os.Stdout, o.dir, o.theme, cols, rows)
	}
	p, err := o.startPuzzle()
	if err != nil {
		return err
	}
	var cols, rows int
	if o.dump != "" {
		if cols, rows, err = parseSize("dump", o.dump); err != nil {
			return err
		}
	}
	a, err := load(o.dir, p, o.seed)
	if err != nil {
		return err
	}
	if o.theme != "" {
		if err := a.ui.Set("@theme", o.theme); err != nil {
			return err
		}
	}
	if o.dump != "" {
		d, err := a.ui.Dump(cols, rows)
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

// onError keeps the game going when a tick or a handler fails: one bad
// write is not a reason to lose a game. The message goes to the top line
// until the next tick or action works. A quit still quits.
func (a *app) onError(_ *tuimark.Event, err error) error {
	if errors.Is(err, tuimark.ErrQuit) {
		return err
	}
	a.notice = err.Error()
	_ = a.ui.Set("notice", a.notice)
	return nil
}

// blocked reports whether the board is out of play: paused, or under a modal.
func (a *app) blocked() bool { return a.paused || a.helpOpen || a.newOpen || a.winOpen }

// running reports whether the clock counts: the board is in play and the
// game is not finished.
func (a *app) running() bool { return !a.blocked() && !a.g.won }

// tick is the Every function: step on the clock, then clear a notice the
// successful tick shows is stale.
func (a *app) tick(b *tuimark.Batch, now time.Time) error {
	if err := a.step(b, now); err != nil {
		return err
	}
	if a.notice != "" {
		a.notice = ""
		return b.Set("notice", "")
	}
	return nil
}

// step is the clock's body, with the time handed in. It adds the play time
// since the last step while the board is in play, and opens the win modal
// once the finished board has shown for winDelay. It writes only what
// changed.
func (a *app) step(s setter, now time.Time) error {
	dt := time.Duration(0)
	if !a.last.IsZero() {
		dt = now.Sub(a.last)
	}
	a.last = now
	if dt > time.Second {
		dt = time.Second // a stall (a suspended terminal) is not play time
	}
	if a.running() {
		before := clock(a.elapsed.Milliseconds())
		a.elapsed += dt
		if after := clock(a.elapsed.Milliseconds()); after != before {
			if err := s.Set("time", after); err != nil {
				return err
			}
		}
	}
	if a.g.won && !a.winShown && !a.helpOpen && !a.newOpen {
		a.winWait += dt
		if a.winWait >= winDelay {
			a.winShown, a.winOpen = true, true
			return s.Set("win_open", true)
		}
	}
	return nil
}

// restart starts p as a new game: the clock, the modals, and the pause reset.
func (a *app) restart(p puzzle) {
	a.g = newGame(p)
	a.games++
	a.elapsed, a.paused = 0, false
	a.newOpen, a.winOpen, a.winShown, a.winWait = false, false, false, 0
}

// handlers are the named actions of app.tui. Each mutates the host's state;
// the wrapper then publishes the whole raw store in one batch, so a handler
// that does nothing changes nothing, and a successful action clears the
// last error and the last message.
func (a *app) handlers() map[string]tuimark.Handler {
	hs := map[string]tuimark.Handler{}
	on := func(name string, h func(ev tuimark.Event) error) {
		hs[name] = func(ev tuimark.Event) error {
			a.info = ""
			if err := h(ev); err != nil {
				return err
			}
			a.notice = ""
			return a.sync()
		}
	}
	hs["quit"] = func(tuimark.Event) error { return tuimark.ErrQuit }

	for name, d := range map[string][2]int{"up": {-1, 0}, "down": {1, 0}, "left": {0, -1}, "right": {0, 1}} {
		d := d
		on(name, func(tuimark.Event) error {
			if !a.blocked() {
				a.g.moveCursor(d[0], d[1])
			}
			return nil
		})
	}
	on("cell", func(ev tuimark.Event) error {
		if a.blocked() || len(ev.Source) != 3 || ev.Source[0] != 'c' {
			return nil
		}
		r, c := int(ev.Source[1]-'0'), int(ev.Source[2]-'0')
		if r > 8 || c > 8 {
			return fmt.Errorf("no cell %q", ev.Source)
		}
		a.g.cur = r*9 + c
		return nil
	})
	for d := 1; d <= 9; d++ {
		d := d
		on(fmt.Sprintf("put%d", d), func(tuimark.Event) error {
			if !a.blocked() {
				a.g.put(d)
			}
			return nil
		})
	}
	on("erase", func(tuimark.Event) error {
		if !a.blocked() {
			a.g.erase()
		}
		return nil
	})
	on("notes", func(tuimark.Event) error {
		if !a.blocked() && !a.g.won {
			a.g.pencil = !a.g.pencil
		}
		return nil
	})
	on("undo", func(tuimark.Event) error {
		if !a.blocked() {
			a.g.undoMove()
		}
		return nil
	})
	on("redo", func(tuimark.Event) error {
		if !a.blocked() {
			a.g.redoMove()
		}
		return nil
	})
	on("hint", func(tuimark.Event) error {
		if a.blocked() {
			return nil
		}
		if msg, ok := a.g.hint(); ok {
			a.info = msg
		}
		return nil
	})
	on("check", func(tuimark.Event) error {
		if a.blocked() || a.g.won {
			return nil
		}
		switch n := a.g.check(); n {
		case 0:
			a.info = "check: no wrong entries"
		case 1:
			a.info = "check: 1 wrong entry marked"
		default:
			a.info = fmt.Sprintf("check: %d wrong entries marked", n)
		}
		return nil
	})
	on("pause", func(tuimark.Event) error {
		if !a.helpOpen && !a.newOpen && !a.winOpen && !a.g.won {
			a.paused = !a.paused
		}
		return nil
	})
	on("mouse", func(tuimark.Event) error {
		a.mouse = !a.mouse
		return nil
	})
	on("help_open", func(tuimark.Event) error {
		if !a.newOpen && !a.winOpen {
			a.helpOpen = true
		}
		return nil
	})
	on("help_close", func(tuimark.Event) error {
		a.helpOpen = false
		return nil
	})
	on("newgame_open", func(tuimark.Event) error {
		if !a.helpOpen {
			a.newOpen, a.winOpen = true, false
		}
		return nil
	})
	on("newgame_close", func(tuimark.Event) error {
		a.newOpen = false
		return nil
	})
	on("new_game", func(ev tuimark.Event) error {
		level := levelOf(strings.TrimPrefix(ev.Source, "nd-"))
		if level == 0 {
			return fmt.Errorf("new game: no difficulty %q", ev.Source)
		}
		a.restart(generateCached(a.seed+int64(a.games), level))
		return nil
	})
	on("win_close", func(tuimark.Event) error {
		a.winOpen = false
		return nil
	})
	return hs
}
