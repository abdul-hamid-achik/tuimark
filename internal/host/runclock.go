package host

import "time"

// The clock seam of Run (SPEC v0.4b §26.13, decision 105): a session
// reads its start, the spinner timer, the Every deadlines, and the
// TUIMARK_LOG timestamps through one clock. The real clock is the
// default; the runtime's own tests put a fake one in through
// testRunClock, so a count of redraws or ticks is exact and never
// depends on the machine. Nothing here is public.
//
// What stays on the real clock, on purpose: the escape timeout, the
// capability probe's cap, the size poll, the signal timers, the
// ctrl+c window, and the `us` of a frame record (a duration of work, not
// a point of the session).

// runClock is the clock of one Run session.
type runClock interface {
	// Now is the clock's reading. Differences of two readings are the
	// session's elapsed time (the real clock's carry the monotonic reading).
	Now() time.Time
	// NewTimer returns a timer that fires d from now, as time.NewTimer.
	NewTimer(d time.Duration) runTimer
}

// runTimer is a timer of a runClock, with time.Timer's contract: Stop
// reports whether it stopped an active timer, and does not drain C; Reset
// re-arms a stopped or expired timer, after its channel was drained when
// Stop returned false.
type runTimer interface {
	C() <-chan time.Time
	Stop() bool
	Reset(d time.Duration) bool
}

type realClock struct{}

func (realClock) Now() time.Time                    { return time.Now() }
func (realClock) NewTimer(d time.Duration) runTimer { return realTimer{time.NewTimer(d)} }

type realTimer struct{ t *time.Timer }

func (r realTimer) C() <-chan time.Time        { return r.t.C }
func (r realTimer) Stop() bool                 { return r.t.Stop() }
func (r realTimer) Reset(d time.Duration) bool { return r.t.Reset(d) }

// testRunClock, when non-nil, is the clock every Run, RunHost, and Loop
// session of the process reads. Only the runtime's tests set it, before
// the session starts and with no session running.
var testRunClock runClock

// clock returns the clock of the session: the session's own (tests that
// build a termSession), else the test hook, else the real one.
func (s *termSession) clock() runClock {
	if s.clk != nil {
		return s.clk
	}
	return defaultRunClock()
}

// defaultRunClock is the test hook, else the real clock.
func defaultRunClock() runClock {
	if testRunClock != nil {
		return testRunClock
	}
	return realClock{}
}

// setClock gives the app the clock of the session that drives it, or nil
// when it stops driving one: Every reads it under everyMu, so a
// registration made during a session counts its first tick from the
// session's clock.
func (a *App) setClock(c runClock) {
	a.everyMu.Lock()
	a.clk = c
	a.everyMu.Unlock()
}

// nowLocked is the clock's reading for a registration: the session's, or
// the real one outside a session. The caller holds everyMu.
func (a *App) nowLocked() time.Time {
	if a.clk != nil {
		return a.clk.Now()
	}
	return time.Now()
}
