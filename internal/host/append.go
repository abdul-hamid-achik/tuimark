package host

import (
	"fmt"
	"unsafe"
)

// Append adds v to the value at path (SPEC v0.4d §18.3, decisions 252 and
// 253): a string value is concatenated with v, which must be a string; an
// array value gets v, any JSON value after Set's coercion, as one element
// at its end (an array v is one element, never spread). Every other case
// is an error that changes nothing. Otherwise it is Set of the new value:
// it schedules a redraw and recomputes the derived values.
func (a *App) Append(path string, v any) error {
	jv, err := ToJSON(v)
	if err != nil {
		return err
	}
	if err := checkAppendPath(path); err != nil {
		return err
	}
	if err := a.appendUnderLock(path, jv); err != nil {
		return err
	}
	a.Wake()
	return nil
}

// appendUnderLock is appendLocked under a.mu, released by a defer for the
// reason applyUnderLock gives: a Derive function runs under the lock.
func (a *App) appendUnderLock(path string, jv any) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.appendLocked(path, jv, false)
}

// Append queues an append as b.Set queues a set, after checking its
// arguments as Set does (the coercion, the path's grammar, a reserved
// path, a derived path). The value at the path is checked when the batch
// is applied, against the store as the earlier writes of the batch left
// it (SPEC v0.4d §18.3), and a failure there applies nothing.
func (b *Batch) Append(path string, v any) error {
	if b.done {
		return fmt.Errorf("tuimark: Batch.Append called after Batch's fn returned")
	}
	jv, err := ToJSON(v)
	if err != nil {
		return b.fail(err)
	}
	if err := checkAppendPath(path); err != nil {
		return b.fail(err)
	}
	if err := b.a.derivedTarget(path); err != nil {
		return b.fail(err)
	}
	b.writes = append(b.writes, batchWrite{path: path, v: jv, appendOp: true})
	return nil
}

// checkAppendPath is the part of Append's checks that depends on the path
// alone (SPEC v0.4d §18.3): a reserved path, the root, and a malformed
// path, in that order. A derived path is the store's to refuse
// (derivedConflict), with the text Set gives.
func checkAppendPath(path string) error {
	if isReservedPath(path) {
		return fmt.Errorf("append %q: %q is a reserved path", path, path)
	}
	if path == "" {
		return fmt.Errorf(`append "": the root is not a string or an array`)
	}
	if !validStorePath(path) {
		return fmt.Errorf("tuimark: bad path %q", path)
	}
	return nil
}

// appendLocked appends jv (already JSON-coerced) at path under a.mu:
// Append's store half and the apply of a Batch's queued append. batch is
// true inside a Batch's trial (storeWrite then leaves the ancestors of
// derived paths to the batch's end). A write that fails changes nothing.
func (a *App) appendLocked(path string, jv any, batch bool) error {
	// A derived path, or a path inside one, gives Set's error, before the
	// value is looked at.
	if len(a.derived) > 0 {
		if err := a.derivedConflict(path, nil, true); err != nil {
			return err
		}
	}
	cur, ok := lookup(a.store, path)
	if !ok {
		return fmt.Errorf("append %q: %q is missing", path, path)
	}
	switch c := cur.(type) {
	case string:
		s, isStr := jv.(string)
		if !isStr {
			return fmt.Errorf("append %q: %q holds a string, and the value is %s, not a string", path, path, runTypeName(jv))
		}
		return a.storeWrite(path, c+s, batch)
	case []any:
		return a.storeWrite(path, a.cow.appendTo(c, jv), batch)
	}
	return fmt.Errorf("append %q: %q holds %s, not a string or an array", path, path, runTypeName(cur))
}

// appendTo returns c with v added at its end. Out of a trial (a nil cowSet,
// which owns everything) the store appends in place, amortized O(1): a
// store holds no two arrays over the same memory. Inside a trial the array
// may be shared with the store the batch started from, which a rollback
// restores, so the first append to an array the trial does not own goes to
// a copy, which the trial then owns; later appends of the trial go in place.
func (s cowSet) appendTo(c []any, v any) []any {
	if s == nil {
		return append(c, v)
	}
	if _, owned := s[unsafe.Pointer(unsafe.SliceData(c))]; !owned || len(c) == 0 {
		c = append(make([]any, 0, len(c)+1), c...)
	}
	out := append(c, v)
	s[unsafe.Pointer(unsafe.SliceData(out))] = struct{}{}
	return out
}
