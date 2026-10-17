package host

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// withDoc opens the modal m when a handler sets o. The frame that opens
// it moves focus to #x, so one round then dispatches fx and opened, in
// that order: a round with an event after the first.
const withDoc = `<tui version="1">
<keymap>
  <bind keys="g" action="go"/>
  <bind keys="n" action="next"/>
</keymap>
<screen id="s">
  <text>[{o}]</text>
  <modal id="m" open="o" on:open="opened"><button id="x" on:focus="fx">X</button></modal>
</screen>
</tui>`

var errBoom = errors.New("boom")

// withRig drives loopWith (RunWith's loop on a pipe) and records every
// dispatched action in order.
type withRig struct {
	app  *App
	in   *io.PipeWriter
	done chan error
	mu   sync.Mutex
	seen []string
	log  string // TUIMARK_LOG path, "" for none
}

func (r *withRig) record(s string) {
	r.mu.Lock()
	r.seen = append(r.seen, s)
	r.mu.Unlock()
}

func (r *withRig) actions() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.seen...)
}

// waitFor waits until the recorded actions are exactly want.
func (r *withRig) waitFor(t *testing.T, want ...string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Join(r.actions(), " ") == strings.Join(want, " ") {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("actions %v, want %v", r.actions(), want)
}

func (r *withRig) send(t *testing.T, s string) {
	t.Helper()
	if _, err := io.WriteString(r.in, s); err != nil {
		t.Fatal(err)
	}
}

func (r *withRig) wait(t *testing.T) error {
	t.Helper()
	select {
	case err := <-r.done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("the loop did not return")
	}
	return nil
}

// startWith runs loopWith on withDoc. handlers maps an action to what its
// handler does after recording it; every action of the document records.
func startWith(t *testing.T, ctx context.Context, opts RunOptions, logged bool, handlers map[string]func(*App) error) *withRig {
	t.Helper()
	a := doc(t, withDoc)
	_ = a.Bind("", map[string]any{"o": false})
	r := &withRig{app: a, done: make(chan error, 1)}
	for _, act := range []string{"go", "next", "fx", "opened"} {
		act := act
		a.On(act, func(Event) error {
			r.record(act)
			if h := handlers[act]; h != nil {
				return h(a)
			}
			return nil
		})
	}
	var lg *runLog
	if logged {
		r.log = filepath.Join(t.TempDir(), "run.log")
		var err error
		if lg, err = openRunLog(r.log, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	pr, pw := io.Pipe()
	r.in = pw
	go func() {
		r.done <- a.loopWith(ctx, pr, &syncBuf{}, func() (int, int) { return 30, 8 }, nil, opts, lg)
	}()
	t.Cleanup(func() { pw.Close() })
	return r
}

// openModal is the go handler: it opens the modal, so the frame after it
// queues fx and opened for one round.
func openModal(a *App) error { return a.Set("o", true) }

// afterAction returns the records that follow the action record named
// act in the log at path.
func afterAction(t *testing.T, path, act string) []map[string]any {
	t.Helper()
	recs := readLog(t, path)
	for i, rec := range recs {
		if rec["ev"] == "action" && rec["action"] == act {
			return recs[i+1:]
		}
	}
	t.Fatalf("no %s action in the log: %v", act, recs)
	return nil
}

// 110 (headless part). A cancellation is taken at the loop's stop checks
// (SPEC v0.4a §18.2): while it waits, it ends the session at once; during
// a handler, when the handler returns, so the round's later events do not
// run and no further frame is drawn. RunWith's error wraps ctx.Err(), and
// TUIMARK_LOG's end record is "error". A context already done returns at
// once, writing nothing; RunContext while Play is active returns the
// §18.1 rule 8 error.
func TestRunWithCancel(t *testing.T) {
	t.Run("while waiting", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		r := startWith(t, ctx, RunOptions{}, true, nil)
		r.send(t, "n")
		r.waitFor(t, "next")
		cancel()
		if err := r.wait(t); !errors.Is(err, context.Canceled) {
			t.Fatalf("RunWith = %v, want an error wrapping context.Canceled", err)
		}
		recs := readLog(t, r.log)
		if last := recs[len(recs)-1]; last["ev"] != "end" || last["reason"] != "error" {
			t.Errorf("last record %v, want end with reason error", last)
		}
	})
	t.Run("during a handler", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		r := startWith(t, ctx, RunOptions{}, true, map[string]func(*App) error{
			"go": openModal,
			"fx": func(*App) error { cancel(); return nil },
		})
		r.send(t, "g")
		if err := r.wait(t); !errors.Is(err, context.Canceled) {
			t.Fatalf("RunWith = %v, want an error wrapping context.Canceled", err)
		}
		if got := strings.Join(r.actions(), " "); got != "go fx" {
			t.Errorf("actions %q, want %q (opened is in fx's round)", got, "go fx")
		}
		for _, rec := range afterAction(t, r.log, "fx") {
			if rec["ev"] != "end" {
				t.Errorf("after the cancelling handler: %v", rec)
			}
		}
	})
	t.Run("before the call", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		a := doc(t, withDoc)
		out := &syncBuf{}
		err := a.loopWith(ctx, strings.NewReader("n"), out, func() (int, int) { return 30, 8 }, nil, RunOptions{}, nil)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("loopWith = %v, want an error wrapping context.Canceled", err)
		}
		if out.String() != "" {
			t.Errorf("wrote %q", out.String())
		}
		// RunWith checks the context before anything else of Run's: no
		// terminal needed (the test's stdin is not one).
		if err := a.RunWith(ctx, out, RunOptions{}); !errors.Is(err, context.Canceled) || out.String() != "" {
			t.Errorf("RunWith = %v, wrote %q", err, out.String())
		}
	})
	t.Run("while Play is active", func(t *testing.T) {
		a := doc(t, withDoc)
		if err := a.EnterPlay(); err != nil {
			t.Fatal(err)
		}
		defer a.ExitPlay()
		err := a.RunContext(context.Background(), io.Discard)
		if err == nil || !strings.Contains(err.Error(), "Run cannot start while Play is active") {
			t.Errorf("RunContext during Play = %v", err)
		}
	})
}

// 111. Quit (SPEC v0.4a §18.2): from another goroutine it ends Run with
// nil; from a handler, when that handler returns, so the round's later
// events do not run and no further frame is drawn; made while no Run is
// active (before Run, or during Play) it changes nothing for a later Run.
func TestQuit(t *testing.T) {
	t.Run("from another goroutine", func(t *testing.T) {
		r := startWith(t, context.Background(), RunOptions{}, true, nil)
		r.send(t, "n")
		r.waitFor(t, "next")
		go r.app.Quit()
		if err := r.wait(t); err != nil {
			t.Fatalf("Run = %v, want nil", err)
		}
		recs := readLog(t, r.log)
		if last := recs[len(recs)-1]; last["ev"] != "end" || last["reason"] != "quit" {
			t.Errorf("last record %v, want end with reason quit", last)
		}
	})
	t.Run("from a handler", func(t *testing.T) {
		r := startWith(t, context.Background(), RunOptions{}, true, map[string]func(*App) error{
			"go": openModal,
			"fx": func(a *App) error { a.Quit(); return nil },
		})
		r.send(t, "g")
		if err := r.wait(t); err != nil {
			t.Fatalf("Run = %v, want nil", err)
		}
		if got := strings.Join(r.actions(), " "); got != "go fx" {
			t.Errorf("actions %q, want %q", got, "go fx")
		}
		for _, rec := range afterAction(t, r.log, "fx") {
			if rec["ev"] != "end" {
				t.Errorf("after the handler that quit: %v", rec)
			}
		}
	})
	t.Run("before Run and during Play", func(t *testing.T) {
		a := doc(t, withDoc)
		a.Quit()
		if err := a.EnterPlay(); err != nil {
			t.Fatal(err)
		}
		a.Quit()
		a.ExitPlay()
		r := &withRig{app: a, done: make(chan error, 1)}
		for _, act := range []string{"go", "next"} {
			act := act
			a.On(act, func(Event) error { r.record(act); return nil })
		}
		pr, pw := io.Pipe()
		r.in = pw
		defer pw.Close()
		go func() {
			r.done <- a.loopWith(context.Background(), pr, &syncBuf{}, func() (int, int) { return 30, 8 }, nil, RunOptions{}, nil)
		}()
		r.send(t, "n")
		r.waitFor(t, "next")
		r.send(t, "n")
		r.waitFor(t, "next", "next")
		select {
		case err := <-r.done:
			t.Fatalf("Run ended (%v): an earlier Quit reached it", err)
		default:
		}
		a.Quit()
		if err := r.wait(t); err != nil {
			t.Fatal(err)
		}
	})
}

// 112. OnError (SPEC v0.4a §18.2). With OnError returning nil, the failed
// handler counts as having returned nil: the round's later events, the
// next input of the same read, and the next key all dispatch. OnError
// returning ErrQuit ends RunWith with nil and TUIMARK_LOG's end "quit";
// returning errBoom ends it with errBoom; without OnError, errBoom ends
// it as Run always has.
func TestRunWithOnError(t *testing.T) {
	boom := map[string]func(*App) error{
		"go": func(a *App) error { _ = openModal(a); return errBoom },
		"fx": func(*App) error { return errBoom },
	}
	t.Run("nil continues", func(t *testing.T) {
		var mu sync.Mutex
		var failed []string
		r := startWith(t, context.Background(), RunOptions{OnError: func(ev *Event, err error) error {
			mu.Lock()
			defer mu.Unlock()
			if ev == nil || !errors.Is(err, errBoom) {
				t.Errorf("OnError(%v, %v)", ev, err)
				return err
			}
			failed = append(failed, ev.Action)
			return nil
		}}, false, boom)
		// One read: go fails, then n is the next input of the same read;
		// its settle dispatches fx (fails) and opened in one round.
		r.send(t, "gn")
		r.waitFor(t, "go", "next", "fx", "opened")
		r.send(t, "n")
		r.waitFor(t, "go", "next", "fx", "opened", "next")
		mu.Lock()
		if strings.Join(failed, " ") != "go fx" {
			t.Errorf("OnError saw %v", failed)
		}
		mu.Unlock()
		r.app.Quit()
		if err := r.wait(t); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("ErrQuit quits", func(t *testing.T) {
		r := startWith(t, context.Background(), RunOptions{OnError: func(*Event, error) error {
			return ErrQuit
		}}, true, boom)
		r.send(t, "gn")
		if err := r.wait(t); err != nil {
			t.Fatalf("RunWith = %v, want nil", err)
		}
		if got := strings.Join(r.actions(), " "); got != "go" {
			t.Errorf("actions %q, want %q", got, "go")
		}
		recs := readLog(t, r.log)
		if last := recs[len(recs)-1]; last["ev"] != "end" || last["reason"] != "quit" {
			t.Errorf("last record %v, want end with reason quit", last)
		}
	})
	t.Run("errBoom ends", func(t *testing.T) {
		r := startWith(t, context.Background(), RunOptions{OnError: func(_ *Event, err error) error {
			return err
		}}, false, boom)
		r.send(t, "g")
		if err := r.wait(t); err != errBoom {
			t.Fatalf("RunWith = %v, want errBoom", err)
		}
	})
	t.Run("without OnError", func(t *testing.T) {
		r := startWith(t, context.Background(), RunOptions{}, false, boom)
		r.send(t, "g")
		if err := r.wait(t); err != errBoom {
			t.Fatalf("RunWith = %v, want errBoom", err)
		}
	})
}
