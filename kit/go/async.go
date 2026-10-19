// async: run work off the loop and write its state to the store.
//
// Copy this file into your project and change the package name. It depends
// only on github.com/abdul-hamid-achik/tuimark.
//
// A handler must return quickly: a slow one delays every key. Load runs the
// slow part (a request, a scan) in a goroutine and keeps three paths under
// the one you name, so the markup can show a "loading" line, an error and the
// data with ordinary `if` and {path}:
//
//	<path>.loading  true while the work runs
//	<path>.error    "" or the error's message
//	<path>.data     the result (kept, not cleared, while a reload runs)
package kit

import (
	"context"
	"errors"
	"sync"

	"github.com/abdul-hamid-achik/tuimark"
)

// Loader runs loads for one app. All methods are safe from any goroutine.
type Loader struct {
	app *tuimark.App

	mu      sync.Mutex
	current map[string]*Task // the latest run per path
}

// NewLoader creates a Loader for app.
func NewLoader(app *tuimark.App) *Loader {
	return &Loader{app: app, current: map[string]*Task{}}
}

// Task is one run of Load.
type Task struct {
	done   chan struct{}
	cancel context.CancelFunc
}

// Wait blocks until the run has finished: it wrote its result, or it was
// canceled or superseded and dropped it.
func (t *Task) Wait() { <-t.done }

// Cancel cancels the context given to the work. The work decides whether to
// stop early, but its result is dropped either way; <path>.loading goes back to
// false, and the old data and error stay. A run that a later Load superseded
// writes nothing: the later load owns <path>.loading.
func (t *Task) Cancel() { t.cancel() }

// Load sets <path>.loading to true and <path>.error to "", runs fn in a
// goroutine, then writes <path>.data (or <path>.error) and <path>.loading
// false in one Batch. If Load is called again for the same path while fn
// runs, the first run's context is canceled and its result is dropped, so a
// slow old request never overwrites a newer one. fn's value must be
// JSON-shaped.
func (l *Loader) Load(path string, fn func(ctx context.Context) (any, error)) (*Task, error) {
	_, hasData := l.app.Get(path + ".data")
	ctx, cancel := context.WithCancel(context.Background())
	t := &Task{done: make(chan struct{}), cancel: cancel}

	l.mu.Lock()
	if old := l.current[path]; old != nil {
		old.cancel()
	}
	l.current[path] = t
	l.mu.Unlock()

	err := l.app.Batch(func(b *tuimark.Batch) error {
		if err := b.Set(path+".loading", true); err != nil {
			return err
		}
		if err := b.Set(path+".error", ""); err != nil {
			return err
		}
		if !hasData {
			return b.Set(path+".data", nil)
		}
		return nil
	})
	if err != nil {
		l.mu.Lock()
		if l.current[path] == t {
			delete(l.current, path)
		}
		l.mu.Unlock()
		cancel()
		close(t.done)
		return nil, err
	}
	go func() {
		defer close(t.done)
		defer cancel()
		v, ferr := fn(ctx)
		// Check and write under the lock, so a Load that supersedes this run
		// either waits for the write or has already made it drop the result.
		l.mu.Lock()
		defer l.mu.Unlock()
		wasCurrent := l.current[path] == t
		if wasCurrent {
			delete(l.current, path)
		}
		if !wasCurrent {
			return // superseded by a later Load, which owns the paths now
		}
		if ctx.Err() != nil { // canceled while current: stop the spinner, keep the rest
			_ = l.app.Set(path+".loading", false)
			return
		}
		_ = l.app.Batch(func(b *tuimark.Batch) error {
			msg := ""
			if ferr != nil {
				msg = ferr.Error()
			} else if err := b.Set(path+".data", v); err != nil {
				msg = err.Error()
			}
			return errors.Join(b.Set(path+".error", msg), b.Set(path+".loading", false))
		})
	}()
	return t, nil
}
