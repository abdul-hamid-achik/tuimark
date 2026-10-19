// Command minesweeper runs examples/minesweeper/app.tui: Minesweeper with
// the mouse and the keyboard.
//
//	go build -o bin/minesweeper ./examples/minesweeper && ./bin/minesweeper
//	go run ./examples/minesweeper -level expert           # 30x16 with 99 mines
//	go run ./examples/minesweeper -cols 20 -rows 12 -mines 40   # a custom board
//	go run ./examples/minesweeper -seed 7                 # another set of mines
//	go run ./examples/minesweeper -theme light            # Set("@theme", "light") before Run
//	go run ./examples/minesweeper -dump 90x24             # print Dump() as JSON and exit
//	go run ./examples/minesweeper -demo 90x24             # the solver's scripted game, one JSON line per frame
//
// The view lives entirely in app.tui and theme.tcss. The board is one
// static, clickable cell for every position of the largest grid (30x16),
// because on:click cannot sit inside an each template; a Go test writes
// that part of app.tui, and sizes.tcss, from the same constants this host
// uses. This host holds the rules (game.go), publishes plain data and the
// cells that changed, and implements the named actions. It has no layout,
// width, or colour code.
//
// The clock is one App.Every. The timer starts at the first reveal and
// stops when the game ends, the pause key is pressed, or the help is open.
// Every function that reads the time takes it from app.now, which tests and
// the demo replace with a fake.
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
	"regexp"
	"strconv"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

const (
	defaultSeed = 1
	tickEvery   = 200 * time.Millisecond
)

func main() {
	dir := flag.String("dir", "examples/minesweeper", "directory with app.tui and theme.tcss")
	theme := flag.String("theme", "", "dark or light: Set(\"@theme\", …) before Run")
	seed := flag.Int64("seed", defaultSeed, "seed of the mine placement")
	level := flag.String("level", "beginner", "beginner (9x9, 10), intermediate (16x16, 40), or expert (30x16, 99)")
	cols := flag.Int("cols", 0, "custom board: columns ("+fmt.Sprint(minCols)+"-"+fmt.Sprint(maxCols)+"); with -rows and -mines")
	rows := flag.Int("rows", 0, "custom board: rows ("+fmt.Sprint(minRows)+"-"+fmt.Sprint(maxRows)+")")
	mines := flag.Int("mines", 0, "custom board: mines (1 to cells-10)")
	dumpSize := flag.String("dump", "", "print Dump() as JSON at COLSxROWS and exit")
	demoSize := flag.String("demo", "", "print the solver's scripted game at COLSxROWS, one {\"ms\",\"dump\"} JSON line per frame, and exit")
	flag.Parse()
	o := options{dir: *dir, theme: *theme, seed: *seed, level: *level, dump: *dumpSize, demo: *demoSize}
	flag.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "cols":
			o.cols, o.hasCols = *cols, true
		case "rows":
			o.rows, o.hasRows = *rows, true
		case "mines":
			o.mines, o.hasMines = *mines, true
		case "level":
			o.hasLevel = true
		case "theme":
			o.hasTheme = true
		case "dump":
			o.hasDump = true
		case "demo":
			o.hasDemo = true
		}
	})
	if err := run(o); err != nil {
		fmt.Fprintln(os.Stderr, "minesweeper:", err)
		if errors.Is(err, errUsage) {
			os.Exit(2)
		}
		os.Exit(1)
	}
}

// errUsage marks a bad flag.
var errUsage = errors.New("usage")

func usagef(format string, args ...any) error {
	return fmt.Errorf("%w: %s", errUsage, fmt.Sprintf(format, args...))
}

type options struct {
	dir, theme, dump, demo, level string
	seed                          int64
	cols, rows, mines             int
	hasCols, hasRows, hasMines    bool
	hasLevel                      bool
	hasTheme, hasDump, hasDemo    bool // the flag was given, even as ""
}

// board is a level and its size.
type board struct {
	level             string
	cols, rows, mines int
}

// resolve checks the flags and gives the board they ask for. A custom
// board needs all three of -cols, -rows, and -mines and no -level.
func (o options) resolve() (board, error) {
	custom := o.hasCols || o.hasRows || o.hasMines
	if !custom {
		for _, p := range presets {
			if p.name == o.level {
				return board{p.name, p.cols, p.rows, p.mines}, nil
			}
		}
		return board{}, usagef("-level wants beginner, intermediate, or expert, got %q", o.level)
	}
	if o.hasLevel {
		return board{}, usagef("-level cannot go with -cols, -rows, or -mines")
	}
	if !(o.hasCols && o.hasRows && o.hasMines) {
		return board{}, usagef("a custom board needs -cols, -rows, and -mines together")
	}
	if o.cols < minCols || o.cols > maxCols {
		return board{}, usagef("-cols wants %d to %d, got %d", minCols, maxCols, o.cols)
	}
	if o.rows < minRows || o.rows > maxRows {
		return board{}, usagef("-rows wants %d to %d, got %d", minRows, maxRows, o.rows)
	}
	lo, hi := minesRange(o.cols, o.rows)
	if o.mines < lo || o.mines > hi {
		return board{}, usagef("-mines wants %d to %d on a %dx%d board, got %d", lo, hi, o.cols, o.rows, o.mines)
	}
	return board{"custom", o.cols, o.rows, o.mines}, nil
}

var sizeRE = regexp.MustCompile(`^(\d+)x(\d+)$`)

// parseSize reads COLSxROWS, exactly digits, an x, and digits, each from 1
// to 1000, for -dump and -demo.
func parseSize(flagName, s string) (cols, rows int, err error) {
	m := sizeRE.FindStringSubmatch(s)
	if m != nil && len(m[1]) <= 4 && len(m[2]) <= 4 {
		cols, _ = strconv.Atoi(m[1])
		rows, _ = strconv.Atoi(m[2])
		if cols >= 1 && cols <= 1000 && rows >= 1 && rows <= 1000 {
			return cols, rows, nil
		}
	}
	return 0, 0, usagef("%s wants COLSxROWS between 1x1 and 1000x1000, got %q", flagName, s)
}

// checkDemoSize wants a terminal the demo's board fits in: its clicks are
// aimed at cells that are on the screen.
func checkDemoSize(cols, rows int) error {
	if nc, nr := needCols(demoBoard.cols), needRows(demoBoard.rows); cols < nc || rows < nr {
		return usagef("-demo needs at least %dx%d for its %s board, got %dx%d", nc, nr, demoBoard.level, cols, rows)
	}
	return nil
}

// app is the host's state. Handlers and Every functions run one at a time
// on the runtime's loop, so there is no lock.
type app struct {
	ui  *tuimark.App
	now func() time.Time
	rng *rand.Rand

	level string
	g     *game
	cur   int // the cursor cell

	flagMode bool // a click flags instead of revealing
	paused   bool
	help     bool
	mouse    bool

	elapsed time.Duration // playing time
	last    time.Time     // the clock when elapsed was last brought up to date
	best    map[string]int
	notice  string

	shown     []cellView // what the store holds, by screen position
	shownRows int
}

// load loads app.tui from dir and starts a game on b with the random
// source seeded by seed. The store starts from the game, not from a file;
// sample.json is the same store for the tuimark CLI, kept in sync by a test.
func load(dir string, b board, seed int64, now func() time.Time) (*app, error) {
	ui, err := tuimark.Load(filepath.Join(dir, "app.tui"))
	if err != nil {
		return nil, err
	}
	a := &app{
		ui: ui, now: now, rng: rand.New(rand.NewSource(seed)), mouse: true,
		best:  map[string]int{},
		shown: make([]cellView, maxRows*maxCols),
	}
	for _, p := range presets {
		a.best[p.name] = -1
	}
	a.setBoard(b)
	if err := ui.Bind("", a.raw()); err != nil {
		return nil, err
	}
	a.remember()
	if err := a.register(); err != nil {
		return nil, err
	}
	return a, nil
}

// setBoard starts a fresh game on b.
func (a *app) setBoard(b board) {
	a.level = b.level
	a.g = newGame(b.cols, b.rows, b.mines)
	a.cur = a.g.idx(b.rows/2, b.cols/2)
	a.paused = false
	a.elapsed = 0
	if a.now != nil {
		a.last = a.now()
	}
}

// remember records what the store now shows of the board.
func (a *app) remember() {
	for r := 0; r < maxRows; r++ {
		for c := 0; c < maxCols; c++ {
			a.shown[r*maxCols+c] = a.view(r, c)
		}
	}
	a.shownRows = a.g.rows
}

// register installs the handlers and the derived values.
func (a *app) register() error {
	for name, h := range a.handlers() {
		a.ui.On(name, h)
	}
	return a.derive()
}

func run(o options) error {
	b, err := o.resolve()
	if err != nil {
		return err
	}
	if (o.hasTheme || o.theme != "") && o.theme != "dark" && o.theme != "light" {
		return usagef("-theme wants dark or light, got %q", o.theme)
	}
	var dc, dr, mc, mr int
	if o.hasDump || o.dump != "" {
		if dc, dr, err = parseSize("-dump", o.dump); err != nil {
			return err
		}
	}
	if o.hasDemo || o.demo != "" {
		if mc, mr, err = parseSize("-demo", o.demo); err != nil {
			return err
		}
		if err := checkDemoSize(mc, mr); err != nil {
			return err
		}
		return demo(os.Stdout, o.dir, o.theme, mc, mr)
	}
	a, err := load(o.dir, b, o.seed, time.Now)
	if err != nil {
		return err
	}
	if o.hasTheme || o.theme != "" {
		if err := a.ui.Set("@theme", o.theme); err != nil {
			return err
		}
	}
	if o.hasDump || o.dump != "" {
		d, err := a.ui.Dump(dc, dr)
		if err != nil {
			return err
		}
		out, _ := json.MarshalIndent(d, "", "  ")
		fmt.Println(string(out))
		return nil
	}
	stop := a.ui.Every(tickEvery, a.tick)
	defer stop()

	// Run restores the terminal on SIGINT, SIGTERM, or SIGHUP and returns an
	// error naming the signal.
	return a.ui.RunWith(context.Background(), os.Stdout, tuimark.RunOptions{OnError: a.onError})
}

// onError keeps the game going when a tick or a handler fails: the message
// goes to the top line until the next action or tick works. A quit still
// quits.
func (a *app) onError(_ *tuimark.Event, err error) error {
	if errors.Is(err, tuimark.ErrQuit) {
		return err
	}
	a.notice = err.Error()
	_ = a.ui.Set("notice", a.notice)
	return nil
}

// running reports whether the clock is counting: the mines are laid, the
// game is not over, and neither the pause nor the help is up.
func (a *app) running() bool { return a.g.placed && !a.g.over && !a.paused && !a.help }

// settle brings the playing time up to now. A gap over a second (a
// suspended terminal) counts as a second. Call it before an action changes
// whether the clock runs.
func (a *app) settle(now time.Time) {
	if a.running() {
		if dt := now.Sub(a.last); dt > 0 {
			if dt > time.Second {
				dt = time.Second
			}
			a.elapsed += dt
		}
	}
	a.last = now
}

// tick is the Every function: it keeps the clock and takes a stale error
// off the top line.
func (a *app) tick(b *tuimark.Batch) error {
	before := a.secs()
	a.settle(a.now())
	if a.notice != "" {
		a.notice = ""
		if err := b.Set("notice", ""); err != nil {
			return err
		}
	}
	if s := a.secs(); s != before {
		return b.Set("secs", s)
	}
	return nil
}

// sync publishes the changes in one batch.
func (a *app) sync() error {
	return a.ui.Batch(func(b *tuimark.Batch) error { return a.publish(b) })
}

// handlers are the named actions of app.tui.
func (a *app) handlers() map[string]tuimark.Handler {
	hs := map[string]tuimark.Handler{}
	// An action settles the clock first, so the time it sees is its own; an
	// action that works takes a pending error off the top line and
	// publishes what changed.
	on := func(name string, h func(tuimark.Event) error) {
		hs[name] = func(ev tuimark.Event) error {
			a.settle(a.now())
			if err := h(ev); err != nil {
				return err
			}
			a.notice = ""
			return a.sync()
		}
	}
	on("quit", func(tuimark.Event) error { return tuimark.ErrQuit })
	for name, d := range map[string][2]int{"cur_up": {-1, 0}, "cur_down": {1, 0}, "cur_left": {0, -1}, "cur_right": {0, 1}} {
		d := d
		on(name, func(tuimark.Event) error { a.moveCursor(d[0], d[1]); return nil })
	}
	on("reveal", func(tuimark.Event) error { a.act(a.cur); return nil })
	on("cell", func(ev tuimark.Event) error {
		r, c, ok := parseCellID(ev.Source)
		if !ok || r >= a.g.rows || c >= a.g.cols {
			return fmt.Errorf("click on %q is not a cell of the board", ev.Source)
		}
		a.cur = a.g.idx(r, c)
		a.act(a.cur)
		return nil
	})
	on("flag", func(tuimark.Event) error {
		if !a.paused && a.g.cycle(a.cur) {
			a.after()
		}
		return nil
	})
	on("flag_mode", func(tuimark.Event) error { a.flagMode = !a.flagMode; return nil })
	on("new", func(tuimark.Event) error {
		a.setBoard(board{a.level, a.g.cols, a.g.rows, a.g.mines})
		return nil
	})
	for _, p := range presets {
		p := p
		on("level_"+p.name, func(tuimark.Event) error {
			a.setBoard(board{p.name, p.cols, p.rows, p.mines})
			return nil
		})
	}
	on("pause", func(tuimark.Event) error {
		if a.g.placed && !a.g.over {
			a.paused = !a.paused
		}
		return nil
	})
	on("toggle_mouse", func(tuimark.Event) error { a.mouse = !a.mouse; return nil })
	on("help_toggle", func(tuimark.Event) error { a.help = !a.help; return nil })
	on("help_close", func(tuimark.Event) error { a.help = false; return nil })
	return hs
}

// moveCursor moves the cursor by (dr, dc) and stops at the edges.
func (a *app) moveCursor(dr, dc int) {
	if a.paused { // the board is hidden: the cursor stays where it was
		return
	}
	r, c := a.g.pos(a.cur)
	r, c = clamp(r+dr, 0, a.g.rows-1), clamp(c+dc, 0, a.g.cols-1)
	a.cur = a.g.idx(r, c)
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// act is a click, or enter or space, on cell i: on an open number it
// chords; in flag mode it marks a covered cell; otherwise it reveals.
func (a *app) act(i int) {
	g := a.g
	if g.over || a.paused {
		return
	}
	var changed bool
	switch {
	case g.st[i] == stOpen:
		changed = g.chord(i)
	case a.flagMode:
		changed = g.cycle(i)
	default:
		changed = g.reveal(i, a.rng)
	}
	if changed {
		a.after()
	}
}

// after records a finished game: a win on a preset board that beats its
// best time sets the new best.
func (a *app) after() {
	if !(a.g.over && a.g.won) {
		return
	}
	if s := a.secs(); a.level != "custom" && (a.best[a.level] < 0 || s < a.best[a.level]) {
		a.best[a.level] = s
	}
}
