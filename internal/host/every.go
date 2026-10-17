package host

import (
	"time"
)

// everyReg is one Every registration (SPEC v0.4a §18.2).
type everyReg struct {
	d  time.Duration
	fn func(b *Batch) error
	// next is when its next tick falls due: d after the call that made it
	// during a session, and d after each session's start for every live
	// registration (everyBegin). Guarded by App.everyMu.
	next    time.Time
	stopped bool // stop was called; guarded by App.everyMu
}

// Every arranges for fn to be called every d while Run is active (SPEC
// v0.4a §18.2). Each call gets a fresh *Batch whose writes apply as one
// Batch. fn runs on the loop's goroutine, between inputs, as a handler
// does, and each tick is a settle: the frame is drawn and the queued
// lifecycle events run. A tick that falls due while the loop is busy runs
// once when it is free (missed ticks are dropped, as with time.Ticker),
// and registrations due at the same moment run in the order they were
// registered. An error fn returns, or a failed write of its batch, is a
// handler error with no event: a quit (errors.Is(err, ErrQuit)) ends the
// session, OnError gets any other error with ev == nil, and without
// OnError it ends Run.
//
// Every may be called before Run or during it: a registration made during
// a session counts its first tick d after the call, and each session
// counts the first tick of every other live registration d after it
// starts. Registrations persist across Run calls until stopped. Dump,
// Validate, and Play never call fn. A d of zero or less panics, as
// time.NewTicker does.
//
// stop cancels the registration: it is idempotent, safe from any
// goroutine (fn included), and never waits for a running fn. Once it
// returns, no new call of fn starts; a call already running finishes and
// its batch applies.
func (a *App) Every(d time.Duration, fn func(b *Batch) error) (stop func()) {
	if d <= 0 {
		panic("tuimark: non-positive interval for Every")
	}
	r := &everyReg{d: d, fn: fn}
	a.everyMu.Lock()
	r.next = time.Now().Add(d)
	a.every = append(a.every, r)
	a.everyMu.Unlock()
	a.kickEvery()
	return func() {
		a.everyMu.Lock()
		if !r.stopped {
			r.stopped = true
			for i, x := range a.every {
				if x == r {
					a.every = append(a.every[:i:i], a.every[i+1:]...)
					break
				}
			}
		}
		a.everyMu.Unlock()
		a.kickEvery()
	}
}

// kickEvery asks a running loop to arm its tick timer again: a
// registration was added or stopped.
func (a *App) kickEvery() {
	select {
	case a.everyKick <- struct{}{}:
	default:
	}
}

// everyBegin starts a session's clock: every live registration's first
// tick falls due d after now.
func (a *App) everyBegin(now time.Time) {
	a.everyMu.Lock()
	defer a.everyMu.Unlock()
	for _, r := range a.every {
		r.next = now.Add(r.d)
	}
}

// everyDue returns when the earliest live registration falls due, and
// false when there is none.
func (a *App) everyDue() (time.Time, bool) {
	a.everyMu.Lock()
	defer a.everyMu.Unlock()
	var due time.Time
	for i, r := range a.every {
		if i == 0 || r.next.Before(due) {
			due = r.next
		}
	}
	return due, len(a.every) > 0
}

// takeDue returns the registrations due at now, in registration order,
// and moves each one's next tick to the first multiple of its interval,
// on its own phase, after now: the ticks it missed while the loop was
// busy are dropped, as time.Ticker drops them.
func (a *App) takeDue(now time.Time) []*everyReg {
	a.everyMu.Lock()
	defer a.everyMu.Unlock()
	var out []*everyReg
	for _, r := range a.every {
		if r.next.After(now) {
			continue
		}
		out = append(out, r)
		r.next = r.next.Add(r.d * (now.Sub(r.next)/r.d + 1))
	}
	return out
}

// startTick reports whether fn of r may start now: stop has not been
// called. Checked under everyMu, so once stop returns no new call starts.
func (a *App) startTick(r *everyReg) bool {
	a.everyMu.Lock()
	defer a.everyMu.Unlock()
	return !r.stopped
}
