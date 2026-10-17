package host

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// everyDoc shows the counter n and has a key that blocks its handler.
const everyDoc = `<tui version="1">
<keymap><bind keys="b" action="block"/></keymap>
<screen id="s"><text>N={n}</text></screen>
</tui>`

// everyRig runs loopWith on everyDoc.
type everyRig struct {
	app  *App
	in   *io.PipeWriter
	done chan error
}

func newEveryApp(t *testing.T) *App {
	t.Helper()
	a := doc(t, everyDoc)
	_ = a.Bind("", map[string]any{"n": 0})
	return a
}

func runEvery(t *testing.T, a *App, opts RunOptions) *everyRig {
	t.Helper()
	pr, pw := io.Pipe()
	r := &everyRig{app: a, in: pw, done: make(chan error, 1)}
	go func() {
		r.done <- a.loopWith(context.Background(), pr, &syncBuf{}, func() (int, int) { return 20, 3 }, nil, opts, nil)
	}()
	t.Cleanup(func() { pw.Close() })
	return r
}

func (r *everyRig) quit(t *testing.T) error {
	t.Helper()
	r.app.Quit()
	return r.wait(t)
}

func (r *everyRig) wait(t *testing.T) error {
	t.Helper()
	select {
	case err := <-r.done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("the loop did not return")
	}
	return nil
}

// screenOf is the live frame's text.
func screenOf(a *App) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.last == nil || a.last.Grid == nil {
		return ""
	}
	return strings.Join(a.last.Grid.Lines(), "\n")
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// incr is an Every function that increments n through its batch.
func incr(a *App) func(b *Batch) error {
	return func(b *Batch) error {
		v, _ := a.Get("n")
		n, _ := v.(float64)
		return b.Set("n", n+1)
	}
}

// 113 (headless part; the pty part is TestEveryPty). Every (SPEC v0.4a
// §18.2) runs fn on the loop between inputs, as one Batch per tick, and a
// tick is a settle (the frame shows n increasing).
func TestEvery(t *testing.T) {
	t.Run("ticks update the frame", func(t *testing.T) {
		a := newEveryApp(t)
		stop := a.Every(5*time.Millisecond, incr(a))
		defer stop()
		r := runEvery(t, a, RunOptions{})
		for _, want := range []string{"N=1", "N=2", "N=3"} {
			waitUntil(t, want, func() bool { return strings.Contains(screenOf(a), want) })
		}
		if err := r.quit(t); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("missed ticks coalesce", func(t *testing.T) {
		a := newEveryApp(t)
		var mu sync.Mutex
		var calls []time.Time
		var blockEnd time.Time
		stop := a.Every(20*time.Millisecond, func(*Batch) error {
			mu.Lock()
			calls = append(calls, time.Now())
			mu.Unlock()
			return nil
		})
		defer stop()
		a.On("block", func(Event) error {
			time.Sleep(100 * time.Millisecond)
			mu.Lock()
			blockEnd = time.Now()
			mu.Unlock()
			return nil
		})
		r := runEvery(t, a, RunOptions{})
		waitUntil(t, "a first tick", func() bool { mu.Lock(); defer mu.Unlock(); return len(calls) > 0 })
		if _, err := io.WriteString(r.in, "b"); err != nil {
			t.Fatal(err)
		}
		waitUntil(t, "the blocking handler", func() bool { mu.Lock(); defer mu.Unlock(); return !blockEnd.IsZero() })
		time.Sleep(5 * time.Millisecond)
		mu.Lock()
		blockStart := blockEnd.Add(-100 * time.Millisecond)
		late := 0
		for _, c := range calls {
			if c.After(blockStart) && !c.After(blockEnd.Add(5*time.Millisecond)) {
				late++
			}
		}
		mu.Unlock()
		// One late tick when the loop is free (two at most, when the
		// next tick on the interval's phase falls right after it), never
		// the five the 100 ms held back.
		if late < 1 || late > 2 {
			t.Errorf("%d ticks ran for the 100 ms block, want 1", late)
		}
		if err := r.quit(t); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("simultaneous ticks run in registration order", func(t *testing.T) {
		a := newEveryApp(t)
		var mu sync.Mutex
		var order []string
		reg := func(name string) func() {
			return a.Every(10*time.Millisecond, func(*Batch) error {
				mu.Lock()
				order = append(order, name)
				mu.Unlock()
				return nil
			})
		}
		s1, s2 := reg("first"), reg("second")
		defer s1()
		defer s2()
		r := runEvery(t, a, RunOptions{})
		waitUntil(t, "six ticks", func() bool { mu.Lock(); defer mu.Unlock(); return len(order) >= 6 })
		if err := r.quit(t); err != nil {
			t.Fatal(err)
		}
		mu.Lock()
		defer mu.Unlock()
		for i := 0; i+1 < len(order); i += 2 {
			if order[i] != "first" || order[i+1] != "second" {
				t.Fatalf("order %v, want first and second alternating", order)
			}
		}
	})
	t.Run("stop from fn", func(t *testing.T) {
		a := newEveryApp(t)
		var stop func()
		var calls atomic.Int32
		stop = a.Every(5*time.Millisecond, func(b *Batch) error {
			if calls.Add(1) == 3 {
				stop()
				stop() // idempotent
			}
			return incr(a)(b)
		})
		r := runEvery(t, a, RunOptions{})
		waitUntil(t, "N=3", func() bool { return strings.Contains(screenOf(a), "N=3") })
		time.Sleep(50 * time.Millisecond)
		if n := calls.Load(); n != 3 {
			t.Errorf("fn ran %d times, want 3 (stopped from its third call)", n)
		}
		if v, _ := a.Get("n"); v != float64(3) {
			t.Errorf("n = %v, want 3: the stopping call's batch applies", v)
		}
		if err := r.quit(t); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("a registration survives into a second Run", func(t *testing.T) {
		a := newEveryApp(t)
		var calls atomic.Int32
		stop := a.Every(5*time.Millisecond, func(*Batch) error { calls.Add(1); return nil })
		defer stop()
		r := runEvery(t, a, RunOptions{})
		waitUntil(t, "a tick", func() bool { return calls.Load() > 0 })
		if err := r.quit(t); err != nil {
			t.Fatal(err)
		}
		after := calls.Load()
		time.Sleep(20 * time.Millisecond)
		if calls.Load() != after {
			t.Fatal("fn ran while no Run was active")
		}
		r = runEvery(t, a, RunOptions{})
		waitUntil(t, "a tick in the second Run", func() bool { return calls.Load() > after })
		if err := r.quit(t); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("a registration made during Run", func(t *testing.T) {
		a := newEveryApp(t)
		r := runEvery(t, a, RunOptions{})
		waitUntil(t, "the first frame", func() bool { return strings.Contains(screenOf(a), "N=0") })
		stop := a.Every(5*time.Millisecond, incr(a))
		defer stop()
		waitUntil(t, "N=2", func() bool { return strings.Contains(screenOf(a), "N=2") })
		if err := r.quit(t); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("an error reaches OnError with a nil event", func(t *testing.T) {
		a := newEveryApp(t)
		var seen atomic.Int32
		stop := a.Every(5*time.Millisecond, func(*Batch) error { return errBoom })
		defer stop()
		r := runEvery(t, a, RunOptions{OnError: func(ev *Event, err error) error {
			if ev != nil || !errors.Is(err, errBoom) {
				t.Errorf("OnError(%v, %v), want a nil event and errBoom", ev, err)
			}
			seen.Add(1)
			return nil
		}})
		waitUntil(t, "two errors", func() bool { return seen.Load() >= 2 })
		if err := r.quit(t); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("an error ends Run without OnError", func(t *testing.T) {
		a := newEveryApp(t)
		stop := a.Every(5*time.Millisecond, func(*Batch) error { return errBoom })
		defer stop()
		r := runEvery(t, a, RunOptions{})
		if err := r.wait(t); !errors.Is(err, errBoom) {
			t.Fatalf("Run = %v, want errBoom", err)
		}
	})
	// 112's last item: ErrQuit from an Every function, which Batch's
	// joined error wraps, is a quit.
	t.Run("ErrQuit is a quit", func(t *testing.T) {
		a := newEveryApp(t)
		stop := a.Every(5*time.Millisecond, func(*Batch) error { return ErrQuit })
		defer stop()
		r := runEvery(t, a, RunOptions{OnError: func(*Event, error) error {
			t.Error("OnError called for a quit")
			return nil
		}})
		if err := r.wait(t); err != nil {
			t.Fatalf("Run = %v, want nil", err)
		}
	})
	t.Run("Dump and Validate never call fn", func(t *testing.T) {
		a := newEveryApp(t)
		var calls atomic.Int32
		stop := a.Every(time.Millisecond, func(*Batch) error { calls.Add(1); return nil })
		defer stop()
		for i := 0; i < 5; i++ {
			a.Dump(20, 3, false)
			a.Validate()
			time.Sleep(2 * time.Millisecond)
		}
		if n := calls.Load(); n != 0 {
			t.Errorf("fn ran %d times outside Run", n)
		}
	})
	t.Run("a non-positive interval panics", func(t *testing.T) {
		for _, d := range []time.Duration{0, -time.Second} {
			func() {
				defer func() {
					if recover() == nil {
						t.Errorf("Every(%v) did not panic", d)
					}
				}()
				newEveryApp(t).Every(d, func(*Batch) error { return nil })
			}()
		}
	})
}
