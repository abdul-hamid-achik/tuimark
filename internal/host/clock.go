package host

import (
	"fmt"
	"time"
)

// MaxTickFirings is the most Every calls one tick step may make (SPEC
// v0.4b §15.4.1, decision 79).
const MaxTickFirings = 10000

// VClock returns the app's virtual clock V (SPEC v0.4b §15.4.1): app
// state like the store, 0 when the app is created, advanced only by
// VirtualTick. Run neither reads nor changes it, and Dump, Validate, and
// every command but play, test, and tuimark_play never do.
func (a *App) VClock() time.Duration {
	a.everyMu.Lock()
	defer a.everyMu.Unlock()
	return a.vnow
}

// vDue counts the firings that VirtualTick(v1) would make: for each live
// registration whose virtual deadline u0 is at most v1 and period d,
// floor((v1 - u0) / d) + 1. The caller holds everyMu.
func (a *App) vDue(v1 time.Duration) int64 {
	var n int64
	for _, r := range a.every {
		if r.vnext > v1 {
			continue
		}
		n += int64((v1-r.vnext)/r.d) + 1
		if n > 1<<50 { // far past the limit; keeps the sum from overflowing
			return n
		}
	}
	return n
}

// vInstant returns the smallest virtual deadline that is at most v1 and
// the live registrations due then, in registration order. The caller
// holds everyMu.
func (a *App) vInstant(v1 time.Duration) (u time.Duration, due []*everyReg, ok bool) {
	for _, r := range a.every {
		if r.vnext <= v1 && (!ok || r.vnext < u) {
			u, ok = r.vnext, true
		}
	}
	if !ok {
		return 0, nil, false
	}
	for _, r := range a.every {
		if r.vnext == u {
			due = append(due, r)
		}
	}
	return u, due, true
}

// VirtualTick applies a tick:DUR step of dur to the virtual clock (SPEC
// v0.4b §15.4.1). Let V0 be the clock and V1 = V0 + dur.
//
//  1. Unless noHandlers, it counts the firings due; over MaxTickFirings
//     it returns an apply error and changes nothing.
//  2. While some live registration has a virtual deadline u <= V1, it
//     takes the smallest u, sets V = u, and calls, in registration order,
//     each live registration due at u as its own Batch (as Run's tick
//     does), moving its deadline to u + d. Then it calls settle once, the
//     caller's draw-and-dispatch at the phase of u. A registration an fn
//     makes gets the deadline u + d, and one it stops never fires again,
//     at u included. A step that makes more than MaxTickFirings calls
//     ends there with an apply error, V at that instant and no further
//     settle.
//  3. It sets V = V1 and calls settle once more.
//
// With noHandlers no function is called: V1 is still reached, each
// deadline at or before it moves to its first deadline after it, and
// settle runs once.
//
// settle reports whether the caller has ended the session (a quit event):
// the step then ends where it is. The first error an Every function
// returns is fnErr (ErrQuit included, see errors.Is): the remaining
// firings are not made, V stays at that instant, and settle is not called
// again. applyErr is a limit error. The caller reads V through VClock.
func (a *App) VirtualTick(dur time.Duration, noHandlers bool, settle func() (stop bool)) (fnErr, applyErr error) {
	a.everyMu.Lock()
	v1 := a.vnow + dur
	if noHandlers {
		for _, r := range a.every {
			if r.vnext <= v1 {
				r.vnext += r.d * ((v1-r.vnext)/r.d + 1)
			}
		}
		a.vnow = v1
		a.everyMu.Unlock()
		settle()
		return nil, nil
	}
	if n := a.vDue(v1); n > MaxTickFirings {
		a.everyMu.Unlock()
		return nil, fmt.Errorf("tick: %d Every calls fall due; the limit is %d per step", n, MaxTickFirings)
	}
	a.everyMu.Unlock()

	fired := 0
	for {
		a.everyMu.Lock()
		u, due, ok := a.vInstant(v1)
		if !ok {
			a.vnow = v1
			a.everyMu.Unlock()
			break
		}
		a.vnow = u
		a.everyMu.Unlock()
		for _, r := range due {
			if fired >= MaxTickFirings && a.startTick(r) {
				return nil, fmt.Errorf("tick: more than %d Every calls in one step", MaxTickFirings)
			}
			// The check is the call's start (startTick), right before fn,
			// as in Run's ticks: a registration an earlier function of
			// this instant stopped is skipped.
			started := false
			err := a.Batch(func(b *Batch) error {
				if started = a.startTick(r); !started {
					return nil
				}
				return r.fn(b)
			})
			if !started {
				continue
			}
			fired++
			a.everyMu.Lock()
			if !r.stopped {
				r.vnext = u + r.d
			}
			a.everyMu.Unlock()
			if err != nil {
				return err, nil
			}
		}
		if settle() {
			return nil, nil
		}
	}
	settle()
	return nil, nil
}
