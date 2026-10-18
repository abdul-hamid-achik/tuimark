package main

import "time"

// The cycle is eight phases long: work, short break, work, short break,
// work, short break, work, long break. pos is the index in it.
const seqLen = 8

const (
	kindWork  = "work"
	kindShort = "short"
	kindLong  = "long"
)

// maxPhase is the longest phase +/- or a flag may make: the big digits
// show minutes in two places.
const maxPhase = 99*time.Minute + 59*time.Second

// kindAt is the kind of phase pos.
func kindAt(pos int) string {
	switch {
	case pos%seqLen == seqLen-1:
		return kindLong
	case pos%2 == 0:
		return kindWork
	default:
		return kindShort
	}
}

// plan holds the length of each kind of phase.
type plan struct {
	work, short, long time.Duration
}

func (p plan) of(kind string) time.Duration {
	switch kind {
	case kindShort:
		return p.short
	case kindLong:
		return p.long
	default:
		return p.work
	}
}

// session is one phase that ran to its end.
type session struct {
	id    int
	kind  string
	start time.Time
	dur   time.Duration
	task  string
}

// timer is the Pomodoro state machine. It never reads the clock: every
// method that depends on time takes it as an argument, so tests and the
// demo drive it with a fake one.
type timer struct {
	plan  plan
	auto  bool          // start the next phase by itself after the flash
	flash time.Duration // how long the done flash lasts

	pos     int
	left    time.Duration // time left in the phase
	total   time.Duration // length of the phase, +/- included
	running bool
	done    time.Duration // > 0: the done flash, this much of it left
	started time.Time     // when the phase first ran; zero until it does
	last    time.Time     // the instant step has accounted for
	task    string        // the task name the next completed work phase gets

	sessions []session // oldest first
	nextID   int
}

func newTimer(p plan, auto bool, flash time.Duration) *timer {
	t := &timer{plan: p, auto: auto, flash: flash}
	t.enter(0)
	return t
}

func (t *timer) kind() string { return kindAt(t.pos) }

// enter makes phase pos the current one, stopped at its full length.
func (t *timer) enter(pos int) {
	t.pos = ((pos % seqLen) + seqLen) % seqLen
	t.total = t.plan.of(t.kind())
	t.left = t.total
	t.done = 0
	t.started = time.Time{}
}

// step accounts for the time since the last call. While the timer runs it
// counts the phase down; at zero it records the session and shows the done
// flash, and when the flash ends it moves on to the next phase, which
// starts at once when auto is set. A gap longer than a phase is walked
// through phase by phase. A clock that moved backwards is ignored: that call
// counts no time and leaves the accounted instant where it was, so time the
// timer already counted is never counted twice when the clock catches up.
func (t *timer) step(now time.Time) {
	if t.last.IsZero() {
		t.last = now
		return
	}
	dt := now.Sub(t.last)
	if dt < 0 {
		return
	}
	t.last = now
	for dt > 0 {
		if t.done > 0 {
			use := minDur(dt, t.done)
			t.done -= use
			dt -= use
			if t.done == 0 {
				t.advance(now.Add(-dt))
			}
			continue
		}
		if !t.running {
			return
		}
		use := minDur(dt, t.left)
		t.left -= use
		dt -= use
		if t.left == 0 {
			t.complete(now.Add(-dt))
		}
	}
}

// complete ends the phase: it records the session and starts the flash,
// or moves on at once when there is no flash.
func (t *timer) complete(at time.Time) {
	start := t.started
	if start.IsZero() {
		start = at.Add(-t.total)
	}
	t.nextID++
	task := ""
	if t.kind() == kindWork {
		task = t.task
	}
	t.sessions = append(t.sessions, session{id: t.nextID, kind: t.kind(), start: start, dur: t.total, task: task})
	if t.flash <= 0 {
		t.advance(at)
		return
	}
	t.done = t.flash
}

// advance moves on to the next phase. It runs when auto is set, and stops
// otherwise.
func (t *timer) advance(at time.Time) {
	t.enter(t.pos + 1)
	t.running = t.auto
	if t.running {
		t.started = at
	}
}

// toggle starts or pauses the timer. It does nothing during the flash.
func (t *timer) toggle(now time.Time) {
	if t.done > 0 {
		return
	}
	t.running = !t.running
	if t.running && t.started.IsZero() {
		t.started = now
	}
}

// finishFlash ends a pending done flash at once: the phase that just ended
// is already recorded, so what is left is the move to the next phase. reset
// and skip call it first, so a key pressed during the flash acts on the
// phase that follows and the cycle never goes backwards.
func (t *timer) finishFlash(now time.Time) {
	if t.done > 0 {
		t.done = 0
		t.advance(now)
	}
}

// reset puts the phase back to its full planned length, stopped. During the
// done flash it first moves on to the next phase and resets that one.
func (t *timer) reset(now time.Time) {
	t.finishFlash(now)
	t.running = false
	t.enter(t.pos)
}

// skip moves on to the next phase without recording a session. A running
// timer keeps running. During the done flash the next phase is the one the
// flash was about to start, so that one is skipped.
func (t *timer) skip(now time.Time) {
	t.finishFlash(now)
	t.enter(t.pos + 1)
	if t.running {
		t.started = now
	}
}

// addMinute lengthens the phase by a minute.
func (t *timer) addMinute() bool {
	if t.done > 0 || t.total+time.Minute > maxPhase {
		return false
	}
	t.total += time.Minute
	t.left += time.Minute
	return true
}

// subMinute shortens the phase by a minute, keeping at least a second.
func (t *timer) subMinute() bool {
	if t.done > 0 || t.left <= time.Minute {
		return false
	}
	t.total -= time.Minute
	t.left -= time.Minute
	return true
}

func minDur(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
