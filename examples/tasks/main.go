// Command tasks runs examples/tasks/app.tui, a kanban board: three lanes
// (Todo, Doing, Done) of cards you move between lanes, reorder, mark in
// bulk, add, edit, delete, and filter.
//
//	go run ./examples/tasks                    # from the repo root
//	go run ./examples/tasks -file board.json   # load and save the board
//	go run ./examples/tasks -dump 120x30       # print Dump() as JSON and exit
//	go run ./examples/tasks -theme light       # Set("@theme", "light") before Run
//	go run ./examples/tasks -today now         # follow the real date (default: the sample's)
//
// The view lives entirely in app.tui and theme.tcss. This host holds no
// layout, width, padding, column, or truncation code. It keeps one array of
// cards in the store and uses the v0.4 API to turn it into what the markup
// reads:
//
//   - Derive: the three lanes (cols), their counts, and the summary line
//     are recomputed inside every store write, so the filter input, a moved
//     card, and a checked row all update them with no handler;
//   - Every: a once-a-second tick follows the clock for the due dates
//     (-today now) and clears the status line after a few seconds;
//   - RunWith: a handler that fails (say, a board file that cannot be
//     written) lands in the status line and the session goes on.
//
// By default the board comes from sample.json and lives in memory. With
// -file PATH it loads PATH (if it exists) and saves after every change.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

func main() {
	dir := flag.String("dir", "examples/tasks", "directory with app.tui, theme.tcss, sample.json")
	dumpSize := flag.String("dump", "", "print Dump() as JSON at COLSxROWS and exit")
	theme := flag.String("theme", "", "dark or light: Set(\"@theme\", …) before Run")
	file := flag.String("file", "", "load and save the board as JSON at this path (default: in memory, from sample.json)")
	today := flag.String("today", "", "the date due dates are judged against: YYYY-MM-DD, or now (default: the sample's)")
	flag.Parse()
	if err := run(*dir, *dumpSize, *theme, *file, *today); err != nil {
		fmt.Fprintln(os.Stderr, "tasks:", err)
		os.Exit(1)
	}
}

// statusTTL is how many ticks a status message stays on screen.
const statusTTL = 5

// idle is the status line when nothing has happened lately.
const idle = "ready"

// app is the host's state. Handlers, Every ticks and OnError all run on
// the runtime's loop goroutine, one at a time, so none of this needs a
// lock; a Derive function never touches it (it runs under the app's lock
// and reads the store through get).
type app struct {
	ui *tuimark.App

	file string // -file path; "" keeps the board in memory
	live bool   // -today now: the clock sets "today"
	next int    // the number of the next card id

	lastCol  string          // the lane that had focus last, for the filter to hand back
	addCol   string          // the lane a new card goes to
	editID   string          // the card the form edits; "" for a new one
	formPrio int             // the form's priority, 1 to 3
	pending  map[string]bool // the cards the delete confirmation asks about
	ttl      int             // ticks left before the status clears
}

func run(dir, dumpSize, theme, file, today string) error {
	a, err := load(dir)
	if err != nil {
		return err
	}
	a.register()
	if file != "" {
		if err := a.useFile(file); err != nil {
			return err
		}
	}
	if err := a.setToday(today); err != nil {
		return err
	}
	if theme != "" {
		if theme != "dark" && theme != "light" {
			return fmt.Errorf("-theme wants dark or light, got %q", theme)
		}
		if err := a.ui.Set("@theme", theme); err != nil {
			return err
		}
	}
	if dumpSize != "" {
		var cols, rows int
		if _, err := fmt.Sscanf(dumpSize, "%dx%d", &cols, &rows); err != nil {
			return fmt.Errorf("-dump wants COLSxROWS, got %q", dumpSize)
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

// load loads app.tui from dir and binds sample.json. The host's state
// starts from the same sample; handlers and derived values come with
// register.
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
	a := &app{ui: ui, lastCol: "todo", formPrio: 2, pending: map[string]bool{}}
	a.next = nextID(a.cards())
	return a, nil
}

// useFile makes path the board's home: an existing file replaces the
// sample's cards, a missing one starts from the sample and is created by
// the first change.
func (a *app) useFile(path string) error {
	cards, err := readBoard(path)
	msg := fmt.Sprintf("loaded %s from %s", plural(len(cards), "card", "cards"), path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		cards, msg = a.cards(), "new board: "+path+" is created on the first change"
	case err != nil:
		return err
	}
	a.file = path
	a.next = nextID(cards)
	a.ttl = statusTTL
	return a.ui.Batch(func(b *tuimark.Batch) error {
		return errors.Join(b.Set("cards", cards), b.Set("file", path), b.Set("status", msg), b.Set("status_err", false))
	})
}

// setToday applies -today: "" keeps the sample's date, "now" follows the
// clock, anything else is a fixed YYYY-MM-DD.
func (a *app) setToday(v string) error {
	switch v {
	case "":
		return nil
	case "now":
		a.live = true
		return a.ui.Set("today", time.Now().Format(dateLayout))
	}
	if _, ok := parseDay(v); !ok {
		return fmt.Errorf("-today wants YYYY-MM-DD or now, got %q", v)
	}
	return a.ui.Set("today", v)
}

// register installs the derived values and every handler.
func (a *app) register() {
	if err := a.derive(); err != nil {
		panic(err) // a derived path that collides with the store: a programming error
	}
	for name, h := range a.handlers() {
		a.ui.On(name, h)
	}
}

// onError is RunOptions.OnError: show the failure in the status line and
// keep the session going.
func (a *app) onError(_ *tuimark.Event, err error) error {
	a.ttl = statusTTL
	return a.ui.Batch(func(b *tuimark.Batch) error {
		return errors.Join(b.Set("status", "error: "+err.Error()), b.Set("status_err", true))
	})
}

// tick is the Every function: it follows the clock for -today now and
// expires the status message. Tests call it directly.
func (a *app) tick(b *tuimark.Batch, now time.Time) error {
	var errs []error
	if a.live {
		day := now.Format(dateLayout)
		if cur, _ := a.ui.Get("today"); cur != day {
			errs = append(errs, b.Set("today", day))
		}
	}
	if a.ttl > 0 {
		a.ttl--
		if a.ttl == 0 {
			errs = append(errs, b.Set("status", idle), b.Set("status_err", false))
		}
	}
	return errors.Join(errs...)
}

// getter reads the store: App.Get, or the get a Derive function is given.
type getter func(path string) (any, bool)

// setter writes the store: *tuimark.App or *tuimark.Batch.
type setter interface {
	Set(path string, v any) error
}

// decodeCards turns the store's cards (JSON-shaped values) into Go values.
func decodeCards(v any, ok bool) []card {
	if !ok {
		return nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var cards []card
	_ = json.Unmarshal(raw, &cards)
	return cards
}

func (a *app) cards() []card { return decodeCards(a.ui.Get("cards")) }

func text(get getter, path string) string {
	v, _ := get(path)
	s, _ := v.(string)
	return s
}

func dayOf(get getter, path string) time.Time {
	t, _ := parseDay(text(get, path))
	return t
}

// laneIDs are the ids of the cards a lane shows, from the derived cols.
func laneIDs(get getter, col string) map[string]bool {
	out := map[string]bool{}
	v, _ := get("cols." + col)
	list, _ := v.([]any)
	for _, r := range list {
		if m, ok := r.(map[string]any); ok {
			if id, ok := m["id"].(string); ok {
				out[id] = true
			}
		}
	}
	return out
}

// rawMarks are the checked ids of every lane, the hidden ones included.
func rawMarks(get getter) map[string]bool {
	out := map[string]bool{}
	for _, col := range columns {
		v, _ := get("sel." + col)
		list, _ := v.([]any)
		for _, k := range list {
			if id, ok := k.(string); ok {
				out[id] = true
			}
		}
	}
	return out
}

// marked are the checked cards the board shows right now, in board order.
func marked(get getter) []string {
	raw := rawMarks(get)
	var out []string
	for _, col := range columns {
		shown := laneIDs(get, col)
		for _, c := range decodeCards(get("cards")) {
			if c.Col == col && raw[c.ID] && shown[c.ID] {
				out = append(out, c.ID)
			}
		}
	}
	return out
}
