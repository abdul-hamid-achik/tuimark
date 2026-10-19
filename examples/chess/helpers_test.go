package main

import (
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

// fakeClock is the clock the tests hand the app: it moves only when told.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newClock() *fakeClock { return &fakeClock{t: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// testApp is an app with a fake clock; searches run inline.
type testApp struct {
	*app
	clock *fakeClock
}

// newTestApp wires an app the way run does, with the options changed by opt.
func newTestApp(t testing.TB, opt ...func(*options)) *testApp {
	t.Helper()
	o := defaults()
	for _, f := range opt {
		f(&o)
	}
	s, err := o.validate()
	if err != nil {
		t.Fatal(err)
	}
	a, err := newApp(o, s)
	if err != nil {
		t.Fatal(err)
	}
	c := newClock()
	a.now = c.Now
	a.lastTick = c.Now()
	a.engineAt = time.Time{}
	return &testApp{app: a, clock: c}
}

func twoPlayers(o *options) { o.mode = mode2P }

// tickAfter moves the fake clock and runs the Every function once.
func (a *testApp) tickAfter(t testing.TB, d time.Duration) {
	t.Helper()
	a.clock.Advance(d)
	if err := a.tick(); err != nil {
		t.Fatal(err)
	}
}

// tickFor runs the Every function every tickStep until d has passed, as the
// loop would.
func (a *testApp) tickFor(t testing.TB, d time.Duration) {
	t.Helper()
	for elapsed := time.Duration(0); elapsed < d; elapsed += tickStep {
		a.tickAfter(t, tickStep)
	}
}

func (a *testApp) play(t testing.TB, steps ...string) *tuimark.PlayResult {
	t.Helper()
	return a.playAt(t, 100, 24, steps...)
}

func (a *testApp) playAt(t testing.TB, cols, rows int, steps ...string) *tuimark.PlayResult {
	t.Helper()
	res, err := a.ui.Play(tuimark.PlayOptions{Cols: cols, Rows: rows}, steps...)
	if err != nil {
		t.Fatalf("play %v: %v", steps, err)
	}
	if !res.Dump.OK {
		t.Fatalf("play %v: dump errors %v", steps, res.Dump.Errors)
	}
	return res
}

func actions(res *tuimark.PlayResult) []string {
	var out []string
	for _, e := range res.Events {
		out = append(out, e.Action)
	}
	return out
}

func grid(res *tuimark.PlayResult) string { return strings.Join(res.Dump.Grid, "\n") }

func focused(res *tuimark.PlayResult) string {
	if res.Dump.Focus == nil {
		return ""
	}
	return *res.Dump.Focus
}

// get reads a store path.
func (a *testApp) get(t testing.TB, path string) any {
	t.Helper()
	v, ok := a.ui.Get(path)
	if !ok {
		t.Fatalf("%s is not in the store", path)
	}
	return v
}

func (a *testApp) str(t testing.TB, path string) string {
	t.Helper()
	s, _ := a.get(t, path).(string)
	return s
}

func (a *testApp) flag(t testing.TB, path string) bool {
	t.Helper()
	b, _ := a.get(t, path).(bool)
	return b
}

func (a *testApp) num(t testing.TB, path string) int {
	t.Helper()
	f, _ := a.get(t, path).(float64)
	return int(f)
}

func wantActions(t testing.TB, res *tuimark.PlayResult, want ...string) {
	t.Helper()
	if got := strings.Join(actions(res), ","); got != strings.Join(want, ",") {
		t.Fatalf("actions = %q, want %q", got, strings.Join(want, ","))
	}
}

// sq is the square index of a name like "e4".
func sq(t testing.TB, name string) int {
	t.Helper()
	s, ok := parseSquare(name)
	if !ok {
		t.Fatalf("bad square %q", name)
	}
	return s
}

// cellCenter is the screen cell (x, y) of the middle of the board cell that
// shows the square, found in a dump: the click that hits it.
func (a *testApp) cellCenter(t testing.TB, cols, rows int, square string) (int, int) {
	t.Helper()
	d, err := a.ui.Dump(cols, rows)
	if err != nil {
		t.Fatal(err)
	}
	flip := a.flag(t, "flip")
	s := sq(t, square)
	for r := 0; r < 8; r++ {
		for c := 0; c < 8; c++ {
			if screenSq(r, c, flip) != s {
				continue
			}
			id := cellID(r, c)
			for _, n := range d.Nodes {
				if n.ID == id {
					return n.X + n.W/2, n.Y
				}
			}
		}
	}
	t.Fatalf("no cell for %s", square)
	return 0, 0
}

func (a *testApp) click(t testing.TB, square string) *tuimark.PlayResult {
	t.Helper()
	x, y := a.cellCenter(t, 100, 24, square)
	return a.play(t, "click:"+strconv.Itoa(x)+","+strconv.Itoa(y))
}

// moves plays SAN moves through the move box, as a person would type them.
func (a *testApp) moves(t testing.TB, sans ...string) {
	t.Helper()
	for _, s := range sans {
		a.play(t, ":", "text:"+s, "enter")
	}
}

// fenNow is the FEN of the position the game is in.
func (a *testApp) fenNow() string { return a.g.Cur().FEN() }

type errTest string

func (e errTest) Error() string { return string(e) }

func tuimarkErrQuit() error { return tuimark.ErrQuit }
