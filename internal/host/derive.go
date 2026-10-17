package host

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
)

// derivation is one Derive registration (SPEC v0.4a §18.2).
type derivation struct {
	path string
	fn   func(get func(path string) (any, bool)) (any, error)
}

// Derive declares path a derived value: the result of fn over the store,
// kept in the store at path and recomputed by the runtime (SPEC v0.4a
// §18.2). path must be a non-empty store path in Set's grammar; a
// reserved path, a path inside or above a derived path, and a path Set
// could not write in the current store are errors, and register nothing.
//
// fn runs once when registered, then after every change to the store, as
// part of that change and under the app's lock: a Set, Bind, or Batch
// (once per Batch, after all its writes), and every write the runtime
// makes (an input's bind, a list or table cursor's bind, a checked array,
// a tab's bind, the tab activations of a focus request and their undo).
// Derived values are recomputed in registration order, so a later one
// reads the earlier ones' new values. get has Get's results over the
// store being written (inside a Batch, with every write of the batch),
// except that a reserved path ("@focus", "@screen", "@theme") gives
// (nil, false). fn's value is coerced to JSON as Bind coerces it. fn must
// not call any method of the App (the lock is held), and a panic in it is
// not recovered: it propagates out of the call whose write ran fn, with
// the lock released. A Batch's writes are then undone, and a Derive call
// registers nothing; a Set's or a runtime write's write stays applied,
// and the derived values after the one that panicked keep their old
// values.
//
// When fn returns an error, or a value that cannot be coerced, null is
// written at path and the error is reported once: while Run is active, on
// the loop at its next settle, as a handler error with no event; while
// Play is active, as an error of the step being applied; otherwise at the
// start of the next Run or Play (only the first such error is kept until
// it is reported). The write that caused it returns nil.
//
// A Set, Bind, or b.Set whose path is a derived path, or lies inside one,
// returns an error and changes nothing; a write to an ancestor is allowed
// and followed by the recomputation, but fails as a whole, changing
// nothing, when the recomputation could not write the derived path. For
// the runtime a derived path is unwritable: an input, list, table,
// checked array, or tabs bound to one does not change, fires no on:change
// or on:select, and a built-in keymap row that targets it does not match.
// Derive is safe from any goroutine.
func (a *App) Derive(path string, fn func(get func(path string) (any, bool)) (any, error)) error {
	switch {
	case fn == nil:
		return fmt.Errorf("tuimark: Derive %q: nil function", path)
	case isReservedPath(path):
		return fmt.Errorf("tuimark: Derive %q: a reserved path cannot be derived", path)
	case path == "" || !validStorePath(path):
		return fmt.Errorf("tuimark: Derive: bad path %q", path)
	}
	if err := a.register(path, fn); err != nil {
		return err
	}
	a.Wake()
	return nil
}

// register is Derive's registration under a.mu, released by a defer even
// when fn panics in the first computation (see applyUnderLock); the
// registration is then dropped.
func (a *App) register(path string, fn func(get func(path string) (any, bool)) (any, error)) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, d := range a.derived {
		if covers(d.path, path) || covers(path, d.path) {
			return fmt.Errorf("tuimark: Derive %q: the path is inside or above the derived path %q", path, d.path)
		}
	}
	if !canAssign(a.store, path) {
		return fmt.Errorf("tuimark: Derive %q: the path cannot be written in the current store", path)
	}
	a.derived = append(a.derived, &derivation{path: path, fn: fn})
	computed := false
	defer func() {
		if !computed { // fn panicked: register nothing, as for an error
			a.derived = a.derived[:len(a.derived)-1]
		}
	}()
	// A registration changes what the runtime may write even when the
	// recomputed value equals the stored one (and recompute wrote
	// nothing): never let the frame skip (liveFrame) hand back a frame
	// built before it. Counted before the computation, so a panic in it
	// still invalidates the frame.
	a.writes++
	a.recompute()
	computed = true
	return nil
}

// isReservedPath reports whether path is one of the reserved Set paths
// (SPEC §18): they do not live in the store.
func isReservedPath(path string) bool {
	return path == "@focus" || path == "@screen" || path == ThemePath
}

// covers reports whether p is the path d or lies inside it, segment by
// segment ("view" covers "view.rows" but not "viewport").
func covers(d, p string) bool {
	return p == d || strings.HasPrefix(p, d+".")
}

// errDerived is wrapped by every error a write gets because of a derived
// path (derivedConflict).
var errDerived = errors.New("derived path")

// derivedConflict reports why writing v at path would break a derived
// value (SPEC v0.4a §18.2), or nil: path is a derived path or lies inside
// one, or path is an ancestor of a derived path (the root included) and
// the derived path could not be written under v. With insideOnly, only
// the first is checked: a Batch's own writes are checked as ancestors
// once, against the store the whole batch leaves (derivedUnwritable). The
// caller holds a.mu.
func (a *App) derivedConflict(path string, v any, insideOnly bool) error {
	for _, d := range a.derived {
		if path != "" && covers(d.path, path) {
			if path == d.path {
				return fmt.Errorf("tuimark: %q is a %w (Derive): only its function writes it", path, errDerived)
			}
			return fmt.Errorf("tuimark: %q is inside the %w %q (Derive): only its function writes it", path, errDerived, d.path)
		}
		if insideOnly {
			continue
		}
		if path == "" || covers(path, d.path) {
			rel := d.path
			if path != "" {
				rel = d.path[len(path)+1:]
			}
			if !canAssign(v, rel) {
				return fmt.Errorf("tuimark: set %q: the %w %q could not be written under the new value", path, errDerived, d.path)
			}
		}
	}
	return nil
}

// derivedTarget returns derivedConflict's error when path is a derived
// path or lies inside one, whatever the value: Batch.Set refuses such a
// write when it is queued. The caller must not hold a.mu.
func (a *App) derivedTarget(path string) error {
	if path == "" {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.derivedConflict(path, nil, true)
}

// derivedUnwritable returns an error naming the first derived path the
// store cannot hold: a Batch's end check, before its one recomputation
// (SPEC v0.4a §18.2: a write to an ancestor fails as a whole when the
// recomputation cannot write the derived path). The caller holds a.mu.
func (a *App) derivedUnwritable() error {
	for _, d := range a.derived {
		if !canAssign(a.store, d.path) {
			return fmt.Errorf("tuimark: Batch: the %w %q could not be written under the batch's writes", errDerived, d.path)
		}
	}
	return nil
}

// derivedBlocked reports whether a runtime write of v at path would break
// a derived value: for the runtime the path is unwritable, and the widget
// that would write it does not change (SPEC v0.4a §18.2). The caller
// holds a.mu.
func (a *App) derivedBlocked(path string, v any) bool {
	return len(a.derived) > 0 && path != "" && a.derivedConflict(path, v, false) != nil
}

// writeStore writes v (JSON-shaped) at path, "" being the whole store: the
// primitive every store write goes through, Set's and the runtime's
// alike. It refuses a write that would break a derived value
// (derivedConflict) and one assign cannot make, changing nothing, and
// otherwise records the write for the caches (wrote) and recomputes the
// derived values (recompute). The caller holds a.mu.
func (a *App) writeStore(path string, v any) error {
	return a.storeWrite(path, v, false)
}

// storeWrite is writeStore; with batch, for a Batch's own queued write,
// it leaves the ancestor check to the batch's end (derivedUnwritable), so
// a later write of the batch can make the derived path writable again.
// The runtime's writes made while a Batch applies (a "@focus" request's
// tab activations) keep the full check. The caller holds a.mu.
func (a *App) storeWrite(path string, v any, batch bool) error {
	if len(a.derived) > 0 {
		if err := a.derivedConflict(path, v, batch); err != nil {
			return err
		}
	}
	root, err := assign(a.store, path, v)
	if err != nil {
		return err
	}
	a.store = root
	a.wrote(path)
	a.recompute()
	return nil
}

// unwriteStore removes the last member of path from its parent object
// (unassign), for the undo of a tab activation, unless path is or lies
// inside a derived path; the derived values are then recomputed. The
// caller holds a.mu.
func (a *App) unwriteStore(path string) {
	for _, d := range a.derived {
		if covers(d.path, path) {
			return
		}
	}
	unassign(a.store, path)
	a.wrote(path)
	a.recompute()
}

// recompute computes every derived value again, in registration order,
// and writes each one that changed (SPEC v0.4a §18.2). While a Batch
// applies its writes (deriveHold), it only notes that a recomputation is
// due, which the Batch runs once at its end. The caller holds a.mu.
func (a *App) recompute() {
	if len(a.derived) == 0 {
		return
	}
	if a.deriveHold > 0 {
		a.deriveDue = true
		return
	}
	get := func(path string) (any, bool) {
		if isReservedPath(path) {
			return nil, false
		}
		v, ok := lookup(a.store, path)
		if !ok {
			return nil, false
		}
		return deepCopyJSON(v), true
	}
	for _, d := range a.derived {
		v, err := d.fn(get)
		if err == nil {
			v, err = coerceJSON(v)
		}
		if err != nil {
			a.noteDeriveErr(fmt.Errorf("tuimark: Derive %q: %w", d.path, err))
			v = nil
		}
		if cur, ok := lookup(a.store, d.path); ok && reflect.DeepEqual(cur, v) {
			continue // unchanged: no write, so the caches stay valid
		}
		root, err := assign(a.store, d.path, v)
		if err != nil {
			// Writes that could break a derived path are refused before
			// they apply (derivedConflict), so this does not happen.
			a.noteDeriveErr(fmt.Errorf("tuimark: Derive %q: %w", d.path, err))
			continue
		}
		a.store = root
		a.wrote(d.path)
	}
}

// coerceJSON coerces fn's value as Bind does (ToJSON), copying a value
// that is already JSON-shaped instead of marshalling it, so the store
// never shares memory with what fn returned.
func coerceJSON(v any) (any, error) {
	if jsonShaped(v) {
		return deepCopyJSON(v), nil
	}
	return ToJSON(v)
}

// jsonShaped reports whether v holds only nil, bool, finite float64,
// string, []any, and map[string]any.
func jsonShaped(v any) bool {
	switch x := v.(type) {
	case nil, bool, string:
		return true
	case float64:
		return !math.IsNaN(x) && !math.IsInf(x, 0)
	case []any:
		for _, e := range x {
			if !jsonShaped(e) {
				return false
			}
		}
		return true
	case map[string]any:
		for _, e := range x {
			if !jsonShaped(e) {
				return false
			}
		}
		return true
	}
	return false
}

// noteDeriveErr keeps err to report it, unless an earlier error is still
// waiting to be reported (SPEC v0.4a §18.2: only the first is kept). The
// caller holds a.mu.
func (a *App) noteDeriveErr(err error) {
	if a.deriveErr == nil {
		a.deriveErr = err
	}
}

// TakeDeriveErr returns and clears the Derive error waiting to be
// reported (SPEC v0.4a §18.2): Run reports it at a settle as a handler
// error with no event, and Play as an error of the step being applied.
func (a *App) TakeDeriveErr() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	err := a.deriveErr
	a.deriveErr = nil
	return err
}
