package host

import "fmt"

// Batch queues Set calls for one atomic application (SPEC v0.3 §18.1);
// see App.Batch.
type Batch struct {
	a        *App
	done     bool
	writes   []batchWrite
	firstErr error
}

// batchWrite is one queued, already-validated write: v is JSON-coerced.
type batchWrite struct {
	path string
	v    any
}

// Batch calls fn once, with a fresh *Batch. fn runs without the app's
// lock, so it may call Get, Set, or Batch; a Set or an inner Batch called
// from fn applies on its own, before this one, since neither goes through
// b (SPEC v0.3 §18.1). When fn returns nil and every b.Set succeeded, the
// queued writes are applied in order, as one operation, all or nothing,
// with at most one redraw request. When fn returns an error, or any
// b.Set failed (even if fn ignored it and returned nil), nothing is
// applied and Batch returns the first such error. Batch is safe to call
// from any goroutine, including a handler.
func (a *App) Batch(fn func(b *Batch) error) error {
	b := &Batch{a: a}
	ferr := fn(b)
	b.done = true
	if b.firstErr != nil {
		return b.firstErr
	}
	if ferr != nil {
		return ferr
	}
	return a.applyBatch(b.writes)
}

// Set validates path and v as Set does (JSON coercion, path grammar, the
// reserved paths' values) and queues the write; it applies only once
// every b.Set and fn itself have succeeded (SPEC v0.3 §18.1). Called on
// a *Batch kept after Batch's fn has returned, it returns an error and
// queues nothing.
func (b *Batch) Set(path string, v any) error {
	if b.done {
		return fmt.Errorf("tuimark: Batch.Set called after Batch's fn returned")
	}
	jv, err := ToJSON(v)
	if err != nil {
		return b.fail(err)
	}
	switch path {
	case "@focus":
		// SPEC §18: any JSON value is accepted here, as for Set — a
		// target that turns out not to be focusable is a silent no-op
		// (requestFocus), not an error.
	case "@screen":
		s, _ := jv.(string)
		if !b.a.screenExists(s) {
			return b.fail(fmt.Errorf("tuimark: no screen with id %q", s))
		}
	case ThemePath:
		if _, ok := validThemeValue(jv); !ok {
			return b.fail(fmt.Errorf("tuimark: %s must be \"dark\", \"light\", or \"auto\" (got %s)", ThemePath, jsonText(jv)))
		}
	default:
		if path != "" && !validStorePath(path) {
			return b.fail(fmt.Errorf("tuimark: bad path %q", path))
		}
		if path == "" {
			if _, ok := jv.(map[string]any); !ok {
				return b.fail(fmt.Errorf("tuimark: the root value must be a JSON object, got %T", jv))
			}
		}
	}
	b.writes = append(b.writes, batchWrite{path: path, v: jv})
	return nil
}

// fail records err as the batch's first failure, if none is recorded yet,
// and returns it.
func (b *Batch) fail(err error) error {
	if b.firstErr == nil {
		b.firstErr = err
	}
	return err
}

// batchSnap is the runtime state a queued write can change, besides the
// store, for Batch's all-or-nothing apply: the reserved-path fields
// "@focus"/"@screen" (requestFocus/switchScreen) and "@theme" touch, and
// the remembered tabs of unbound tabs, which a "@focus" into an inactive
// tab writes in place (openTabsFor), so the map is copied. The other
// fields are reassigned wholesale, never mutated in place, so a plain
// save of their values is enough to undo them (unlike the store: see
// applyBatch). The caller holds a.mu.
type batchSnap struct {
	hostTheme string
	focus     string
	focusInit bool
	screen    int
	focusReq  *focusRequest
	tabMem    map[string]string
}

func (a *App) snapForBatch() batchSnap {
	return batchSnap{a.hostTheme, a.focus, a.focusInit, a.screen, a.focusReq, copyStrings(a.tabMem)}
}

func (a *App) restoreBatchSnap(s batchSnap) {
	a.hostTheme, a.focus, a.focusInit, a.screen, a.focusReq = s.hostTheme, s.focus, s.focusInit, s.screen, s.focusReq
	a.tabMem = s.tabMem
}

// applyBatch applies writes in order, atomically, under one lock
// acquisition (SPEC v0.3 §18.1 Batch "Applying the lot"): no frame, dump,
// Get, handler, or Play step observes a state in which some of the
// writes are applied and others are not. assign mutates an existing map
// or array in place, so the trial runs on a deep copy of the store,
// discarded on failure; the store is only kept once every write has
// succeeded.
func (a *App) applyBatch(writes []batchWrite) error {
	if len(writes) == 0 {
		return nil
	}
	a.mu.Lock()
	origStore := a.store
	snap := a.snapForBatch()
	a.store = deepCopyJSON(a.store)
	var failErr error
	for _, w := range writes {
		if err := a.applyLocked(w.path, w.v); err != nil {
			failErr = err
			break
		}
	}
	if failErr != nil {
		a.store = origStore
		a.restoreBatchSnap(snap)
		a.mu.Unlock()
		return failErr
	}
	a.mu.Unlock()
	a.Wake() // at most one redraw for the whole batch
	return nil
}
