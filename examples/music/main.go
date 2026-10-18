// Command music runs examples/music/app.tui: a terminal music player in
// the spirit of cmus, with no audio. Playback is a clock: a position that
// advances while "playing", a track list that ends and moves on by the
// repeat and shuffle modes, and a spectrum visualizer that bounces while
// a track plays and falls to rest when it is paused.
//
//	go run ./examples/music                      # from the repo root
//	go run ./examples/music -theme light         # Set("@theme", "light") before Run
//	go run ./examples/music -seed 9              # another spectrum, another shuffle
//	go run ./examples/music -speed 8             # the playback clock at 8x (to see tracks end)
//	go run ./examples/music -paused              # start paused
//	go run ./examples/music -dump 100x24         # print Dump() as JSON and exit
//	go run ./examples/music -demo 100x24         # print the scripted demo, one JSON frame per line
//
// The view lives entirely in app.tui and theme.tcss. This host holds no
// layout, width, padding, color, or truncation code. It keeps a player
// model (player.go) and publishes plain data:
//
//   - Every: one tick, every 100 ms, calls player.step with the time, then
//     writes the position, the spectrum, and the loudness history in one
//     Batch (one redraw);
//   - Derive: the filtered library, the queue and album rows, the counts
//     of the tab labels, the now-playing fields, and the volume label are
//     computed inside every store write, so the markup has no expressions;
//   - RunWith: an error from a tick or a handler lands in the now-playing
//     panel and playback goes on.
//
// The step function takes the time as an argument and all randomness comes
// from a seeded source, so tests and -demo drive it with a fake clock and
// get the same bytes on every run.
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
	defaultSeed = 7
	tickStep    = 100 * time.Millisecond
)

func main() {
	dir := flag.String("dir", "examples/music", "directory with app.tui and theme.tcss")
	theme := flag.String("theme", "", "dark or light: Set(\"@theme\", …) before Run")
	seed := flag.Int64("seed", defaultSeed, "seed of the spectrum noise and of shuffle")
	speed := flag.Int64("speed", 1, "playback clock multiplier (1 is real time)")
	paused := flag.Bool("paused", false, "start paused")
	dump := flag.String("dump", "", "print Dump() as JSON at COLSxROWS and exit")
	demo := flag.String("demo", "", "print the scripted demo at COLSxROWS, one JSON frame per line, and exit")
	flag.Parse()
	o := options{dir: *dir, theme: *theme, seed: *seed, speed: *speed, paused: *paused, dump: *dump, demo: *demo}
	if err := run(o); err != nil {
		fmt.Fprintln(os.Stderr, "music:", err)
		os.Exit(1)
	}
}

type options struct {
	dir, theme, dump, demo string
	seed, speed            int64
	paused                 bool
}

// app is the host's state. Handlers, Every ticks, and OnError run on the
// runtime's loop one at a time, so none of this needs a lock; a Derive
// function never touches it (it runs under the app's lock and reads the
// store through get).
type app struct {
	ui *tuimark.App
	p  *player

	// notice is the message of the last failure, as the store holds it.
	// A handler that succeeds clears it; so does the tick noticeTicks
	// ticks after the failure, so a failing tick's message is readable.
	notice    string
	noticeAge int
}

// noticeTicks is how many ticks (3 s) a failure stays in the panel when
// nothing else clears it.
const noticeTicks = 30

// epoch is the fake clock's zero: the sample and the demo count from it.
var epoch = time.Date(2026, 10, 19, 21, 0, 0, 0, time.UTC)

// sampleTicks is how many ticks of tickStep the opening state has run: the
// documented point sample.json is a snapshot of.
const sampleTicks = 80

// load loads app.tui from dir and binds the opening store. The opening
// state is the player after setup: track "Tessellate" playing at 1:04,
// two tracks queued, then sampleTicks ticks of a fake clock, so the
// spectrum is already moving.
func load(dir string, seed int64) (*app, error) {
	ui, err := tuimark.Load(filepath.Join(dir, "app.tui"))
	if err != nil {
		return nil, err
	}
	a := &app{ui: ui, p: newPlayer(seed)}
	a.setup()
	if err := ui.Bind("", a.initialStore()); err != nil {
		return nil, err
	}
	return a, nil
}

// setup brings the player to the documented opening point.
func (a *app) setup() {
	p := a.p
	p.playID("t02")
	p.posMs = 64 * 1000
	p.enqueue("t09")
	p.enqueue("t14")
	for i := 0; i <= sampleTicks; i++ {
		p.step(epoch.Add(time.Duration(i) * tickStep))
	}
}

// initialStore is every host-written path. The derived paths (lib_rows,
// queue_rows, album_rows, counts, now, vol) come from register.
func (a *app) initialStore() map[string]any {
	s := map[string]any{
		"filter":    "",
		"tab":       "t-library",
		"cur":       map[string]any{"lib": "t04", "q": "q1", "albums": "Night Bus Hymns"},
		"help_open": false,
		"notice":    "",
	}
	for k, v := range a.state() {
		s[k] = v
	}
	return s
}

// state is the part of the store the player owns.
func (a *app) state() map[string]any {
	p := a.p
	q := make([]any, len(p.queue))
	for i, it := range p.queue {
		q[i] = map[string]any{"qid": it.QID, "id": it.ID}
	}
	return map[string]any{
		"player": map[string]any{
			"current": catalog[p.cur].ID,
			"playing": p.playing,
			"shuffle": p.shuffle,
			"repeat":  p.repeat,
			"volume":  p.volume,
			"pos":     float64(p.posMs) / 1000,
		},
		"queue":    q,
		"spectrum": append([]int(nil), p.bands...),
		"loud":     append([]int(nil), p.loud...),
	}
}

// write adds the player's state to a batch.
func (a *app) write(b *tuimark.Batch) error {
	st := a.state()
	var errs []error
	for _, k := range []string{"player", "queue", "spectrum", "loud"} {
		errs = append(errs, b.Set(k, st[k]))
	}
	return errors.Join(errs...)
}

// publish writes the player's state in one batch: what a handler does
// after it changed the player.
func (a *app) publish() error {
	return a.ui.Batch(a.write)
}

// tick is the Every function, with the time passed in: it advances the
// player and publishes it.
func (a *app) tick(b *tuimark.Batch, now time.Time) error {
	a.p.step(now)
	if err := a.write(b); err != nil {
		return err
	}
	if a.notice != "" {
		if a.noticeAge++; a.noticeAge >= noticeTicks {
			a.notice = ""
			return b.Set("notice", "")
		}
	}
	return nil
}

func parseSize(flagName, s string) (cols, rows int, err error) {
	if _, err := fmt.Sscanf(s, "%dx%d", &cols, &rows); err != nil || cols < 1 || rows < 1 {
		return 0, 0, fmt.Errorf("-%s wants COLSxROWS, got %q", flagName, s)
	}
	return cols, rows, nil
}

func run(o options) error {
	if o.speed < 1 {
		return fmt.Errorf("-speed wants 1 or more, got %d", o.speed)
	}
	if o.theme != "" && o.theme != "dark" && o.theme != "light" {
		return fmt.Errorf("-theme wants dark or light, got %q", o.theme)
	}
	a, err := load(o.dir, o.seed)
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
	if o.demo != "" {
		cols, rows, err := parseSize("demo", o.demo)
		if err != nil {
			return err
		}
		return a.demo(os.Stdout, cols, rows)
	}
	a.p.speed = o.speed
	if o.paused {
		a.p.playing = false
		if err := a.publish(); err != nil {
			return err
		}
	}
	if o.dump != "" {
		cols, rows, err := parseSize("dump", o.dump)
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
	// setup ran on the fake clock; the first real tick only sets the
	// reference time, so the position does not jump.
	a.p.last = time.Time{}
	stop := a.ui.Every(tickStep, func(b *tuimark.Batch) error { return a.tick(b, time.Now()) })
	defer stop()

	// RunWith restores the terminal on SIGINT, SIGTERM, or SIGHUP and
	// returns an error naming the signal, so the command exits with a
	// failure.
	return a.ui.RunWith(context.Background(), os.Stdout, tuimark.RunOptions{OnError: a.onError})
}

// onError keeps the session alive when a tick or a handler fails: the
// message goes to the now-playing panel and the music goes on.
func (a *app) onError(ev *tuimark.Event, err error) error {
	a.notice, a.noticeAge = err.Error(), 0
	_ = a.ui.Set("notice", a.notice)
	return nil
}

// register installs the handlers and the derived values.
func (a *app) register() error {
	for name, h := range a.handlers() {
		a.ui.On(name, h)
	}
	return a.derive()
}
