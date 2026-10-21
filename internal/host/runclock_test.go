package host

import (
	"context"
	"io"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/tuimark/internal/paint"
)

// The clock seam of Run (SPEC v0.4b §26.13, tests 194-196, decisions 75,
// 85, and 105): the runtime's own tests put a fake clock in the session
// and advance it, so every count of redraws and ticks is exact. Nothing
// here sleeps to wait for state: Advance returns once the loop has
// handled what the advance made fall due and is about to block again.

// fakeClock is a runClock that moves only when the test advances it.
type fakeClock struct {
	mu     sync.Mutex
	cond   *sync.Cond
	epoch  time.Time
	now    time.Time
	timers []*fakeTimer
	// quiet counts the times the loop reached the point where it blocks
	// with no fired timer left untaken: a turn of the loop that finished
	// everything the test had made due. fed counts the reads the loop
	// took, and quietAtFeed is quiet when it took the last one.
	quiet       int
	fed         int
	quietAtFeed int
	hooked      bool // the loop calls markIdle (not so when a test goes through run())
	timedOut    bool // a wait gave up
}

type fakeTimer struct {
	c      *fakeClock
	ch     chan time.Time
	due    time.Time
	active bool
}

func newFakeClock() *fakeClock {
	c := &fakeClock{epoch: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)}
	c.now = c.epoch
	c.cond = sync.NewCond(&c.mu)
	return c
}

// elapsed is how far the clock has been advanced since it was made, which
// is the session's t when the session started at its creation.
func (c *fakeClock) elapsed() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now.Sub(c.epoch)
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) NewTimer(d time.Duration) runTimer {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &fakeTimer{c: c, ch: make(chan time.Time, 1)}
	c.timers = append(c.timers, t)
	t.arm(d)
	return t
}

// arm starts the timer for d from now; d <= 0 fires at once, as a Go
// timer's does. The caller holds c.mu.
func (t *fakeTimer) arm(d time.Duration) {
	t.due = t.c.now.Add(d)
	t.active = true
	if d <= 0 {
		t.fire()
	}
	t.c.cond.Broadcast()
}

// fire delivers the clock's reading, once, and the timer is spent. The
// caller holds c.mu.
func (t *fakeTimer) fire() {
	t.active = false
	select {
	case t.ch <- t.c.now:
	default:
	}
}

func (t *fakeTimer) C() <-chan time.Time { return t.ch }

func (t *fakeTimer) Stop() bool {
	t.c.mu.Lock()
	defer t.c.mu.Unlock()
	was := t.active
	t.active = false
	return was
}

func (t *fakeTimer) Reset(d time.Duration) bool {
	t.c.mu.Lock()
	defer t.c.mu.Unlock()
	was := t.active
	t.arm(d)
	return was
}

// markIdle is the loop's idle hook (termSession.idle): it runs at the top
// of a turn of the loop, right before the select blocks.
func (c *fakeClock) markIdle() {
	c.mu.Lock()
	c.hooked = true
	if !c.pending() {
		c.quiet++
	}
	c.cond.Broadcast()
	c.mu.Unlock()
}

// markFed is the loop's hook for a read it took (termSession.fed).
func (c *fakeClock) markFed() {
	c.mu.Lock()
	c.fed++
	c.quietAtFeed = c.quiet
	c.cond.Broadcast()
	c.mu.Unlock()
}

// pending reports whether a timer fired and the loop has not taken it.
// The caller holds c.mu.
func (c *fakeClock) pending() bool {
	for _, t := range c.timers {
		if len(t.ch) > 0 {
			return true
		}
	}
	return false
}

// quietSeq is the number of quiet turns of the loop so far.
func (c *fakeClock) quietSeq() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.quiet
}

// wait blocks until cond holds, which the caller evaluates under c.mu.
func (c *fakeClock) wait(t *testing.T, what string, cond func() bool) {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.timedOut = false
	stop := time.AfterFunc(10*time.Second, func() {
		c.mu.Lock()
		c.timedOut = true
		c.cond.Broadcast()
		c.mu.Unlock()
	})
	defer stop.Stop()
	for !cond() {
		if c.timedOut {
			t.Fatalf("timed out waiting for %s", what)
		}
		c.cond.Wait()
	}
}

// waitQuiet waits until the loop has had a quiet turn after seq (from
// quietSeq, read before whatever the test did to wake it): everything
// that was due has been handled and the loop is about to block.
func (c *fakeClock) waitQuiet(t *testing.T, seq int) {
	t.Helper()
	c.wait(t, "the loop to go quiet", func() bool { return c.quiet > seq })
}

// Advance moves the clock by d, fires the timers that fall due, and
// returns how many fired once the loop has handled them and gone idle
// again. With none due it returns at once: nothing wakes the loop.
func (c *fakeClock) Advance(t *testing.T, d time.Duration) int {
	t.Helper()
	c.mu.Lock()
	seq := c.quiet
	c.now = c.now.Add(d)
	n := 0
	for _, tm := range c.timers {
		if tm.active && !tm.due.After(c.now) {
			tm.fire()
			n++
		}
	}
	hooked := c.hooked
	c.mu.Unlock()
	if n > 0 && hooked {
		c.waitQuiet(t, seq)
	}
	return n
}

// waitArmed blocks until a timer is armed to fire at elapsed after the
// clock's creation. A test that drives Run through run() has no idle hook
// to tell it the loop has armed its timers: the loop arms a timer from a
// relative duration it computed from an earlier reading of the clock, so
// an Advance that lands between the reading and the arming moves the
// clock under a timer that is not armed yet, and the arming then counts
// its duration from the new reading, one period late. Waiting for the
// armed deadline before the Advance leaves no such window.
func (c *fakeClock) waitArmed(t *testing.T, elapsed time.Duration) {
	t.Helper()
	want := c.epoch.Add(elapsed)
	c.wait(t, "a timer armed for "+elapsed.String(), func() bool {
		for _, tm := range c.timers {
			if tm.active && tm.due.Equal(want) {
				return true
			}
		}
		return false
	})
}

// active is the number of armed timers.
func (c *fakeClock) active() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, t := range c.timers {
		if t.active {
			n++
		}
	}
	return n
}

// clockRig runs the session of Run (App.session) on the fake clock, with
// the terminal's input and output in memory: the loop is the one Run
// drives, without the raw mode and the probe.
type clockRig struct {
	t    *testing.T
	app  *App
	clk  *fakeClock
	in   *io.PipeWriter
	out  *syncBuf
	done chan error
	sess *termSession
	pos  int // out up to here has been looked at
	log  string
}

type rigOpts struct {
	profile paint.Profile
	cols    int
	rows    int
	log     bool
	hooks   HostHooks
	setup   func(a *App)
}

// startClockRig loads src, runs its session, and waits for the first
// frame. TUIMARK_SYNC=0 is sync off.
func startClockRig(t *testing.T, src string, o rigOpts) *clockRig {
	t.Helper()
	a := doc(t, src)
	return startClockRigApp(t, a, o)
}

func startClockRigApp(t *testing.T, a *App, o rigOpts) *clockRig {
	t.Helper()
	if o.cols == 0 {
		o.cols, o.rows = 20, 5
	}
	if o.setup != nil {
		o.setup(a)
	}
	clk := newFakeClock()
	r := &clockRig{t: t, app: a, clk: clk, out: &syncBuf{}, done: make(chan error, 1)}
	var pr *io.PipeReader
	pr, r.in = io.Pipe()
	var lg *runLog
	if o.log {
		r.log = filepath.Join(t.TempDir(), "run.ndjson")
		var err error
		if lg, err = openRunLog(r.log, clk.Now()); err != nil {
			t.Fatal(err)
		}
		lg.useClock(clk)
	}
	r.sess = &termSession{profile: o.profile, sync: syncOff, log: lg, clk: clk, idle: clk.markIdle, fed: clk.markFed, noPoll: true}
	go func() {
		r.done <- a.loopSess(context.Background(), pr, r.out, func() (int, int) { return o.cols, o.rows }, nil, r.sess, o.hooks)
	}()
	t.Cleanup(func() {
		r.in.Close()
		a.Quit()
	})
	clk.waitQuiet(t, 0)
	return r
}

// loopSess is loopWith with the session given: the headless entry point
// of the clock tests.
func (a *App) loopSess(ctx context.Context, in io.Reader, out io.Writer, size func() (int, int), resize <-chan struct{}, sess *termSession, hooks HostHooks) (err error) {
	if err := a.enterMode(modeRun); err != nil {
		return err
	}
	defer a.exitMode()
	sess.ctx = ctx
	defer func() { sess.log.end(endReason(err, sess)) }()
	return a.session(in, out, size, resize, nil, sess, hooks)
}

// since returns what the loop wrote since the last call.
func (r *clockRig) since() string {
	all := r.out.String()
	s := all[r.pos:]
	r.pos = len(all)
	return s
}

// key sends input and waits for the loop to handle it.
func (r *clockRig) key(s string) {
	r.t.Helper()
	r.clk.mu.Lock()
	fed := r.clk.fed
	r.clk.mu.Unlock()
	if _, err := r.in.Write([]byte(s)); err != nil {
		r.t.Fatal(err)
	}
	// The loop took the read, and then had a quiet turn: it handled it.
	r.clk.wait(r.t, "the loop to take the key", func() bool { return r.clk.fed > fed })
	r.clk.mu.Lock()
	seq := r.clk.quietAtFeed
	r.clk.mu.Unlock()
	r.clk.waitQuiet(r.t, seq)
}

// quit ends the session and returns its error.
func (r *clockRig) quit() error {
	r.t.Helper()
	r.app.Quit()
	select {
	case err := <-r.done:
		return err
	case <-time.After(10 * time.Second):
		r.t.Fatal("the loop did not return")
	}
	return nil
}
