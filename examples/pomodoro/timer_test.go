package main

import (
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)

func testTimer(auto bool) *timer {
	return newTimer(plan{work: 25 * time.Minute, short: 5 * time.Minute, long: 15 * time.Minute}, auto, 1500*time.Millisecond)
}

// runFor starts the timer at t0 and steps it second by second for d.
func runFor(t *timer, d time.Duration) time.Time {
	t.step(t0)
	t.toggle(t0)
	now := t0
	for end := t0.Add(d); now.Before(end); {
		now = now.Add(time.Second)
		t.step(now)
	}
	return now
}

func TestPhaseSequence(t *testing.T) {
	want := []string{
		kindWork, kindShort, kindWork, kindShort, kindWork, kindShort, kindWork, kindLong,
		kindWork, // the cycle starts over
	}
	for i, k := range want {
		if got := kindAt(i); got != k {
			t.Errorf("kindAt(%d) = %s, want %s", i, got, k)
		}
	}
	tm := testTimer(true)
	// A whole cycle: 4 x 25m + 3 x 5m + 15m, with a 1.5s flash after each.
	cycle := 4*25*time.Minute + 3*5*time.Minute + 15*time.Minute + 8*1500*time.Millisecond
	end := runFor(tm, cycle)
	if len(tm.sessions) != 8 {
		t.Fatalf("%d sessions after a cycle, want 8", len(tm.sessions))
	}
	for i, s := range tm.sessions {
		if s.kind != want[i] {
			t.Errorf("session %d is %s, want %s", i, s.kind, want[i])
		}
	}
	if tm.pos != 0 || tm.kind() != kindWork || !tm.running {
		t.Errorf("after the cycle: pos %d running %v, want the first work phase running", tm.pos, tm.running)
	}
	_ = end
}

func TestPauseStopsTime(t *testing.T) {
	tm := testTimer(true)
	now := runFor(tm, 60*time.Second)
	if tm.left != 24*time.Minute {
		t.Fatalf("left = %s after a minute, want 24m", tm.left)
	}
	tm.toggle(now) // pause
	tm.step(now.Add(time.Hour))
	if tm.left != 24*time.Minute || tm.running {
		t.Errorf("a paused hour moved the clock: left %s running %v", tm.left, tm.running)
	}
	later := now.Add(time.Hour)
	tm.toggle(later) // resume
	tm.step(later.Add(30 * time.Second))
	if tm.left != 24*time.Minute-30*time.Second {
		t.Errorf("left = %s after 30s more, want 23m30s", tm.left)
	}
}

func TestFlashThenNextPhase(t *testing.T) {
	tm := testTimer(true)
	runFor(tm, 25*time.Minute)
	if tm.done != 1500*time.Millisecond || tm.pos != 0 || tm.left != 0 {
		t.Fatalf("at the end: done %s pos %d left %s, want the flash on the work phase", tm.done, tm.pos, tm.left)
	}
	if len(tm.sessions) != 1 || tm.sessions[0].kind != kindWork || tm.sessions[0].dur != 25*time.Minute {
		t.Fatalf("sessions = %+v", tm.sessions)
	}
	now := t0.Add(25 * time.Minute)
	tm.step(now.Add(time.Second))
	if tm.done != 500*time.Millisecond || tm.pos != 0 {
		t.Errorf("1s into the flash: done %s pos %d", tm.done, tm.pos)
	}
	tm.step(now.Add(2 * time.Second)) // the flash ended 0.5s ago
	if tm.pos != 1 || tm.kind() != kindShort || !tm.running {
		t.Fatalf("after the flash: pos %d running %v, want the short break running", tm.pos, tm.running)
	}
	if want := 5*time.Minute - 500*time.Millisecond; tm.left != want {
		t.Errorf("left = %s, want %s (the overshoot counts)", tm.left, want)
	}
}

func TestNoAutoStopsAfterTheFlash(t *testing.T) {
	tm := testTimer(false)
	runFor(tm, 25*time.Minute+3*time.Second)
	if tm.pos != 1 || tm.running || tm.left != 5*time.Minute {
		t.Errorf("pos %d running %v left %s, want the short break waiting", tm.pos, tm.running, tm.left)
	}
	tm.step(t0.Add(time.Hour))
	if tm.left != 5*time.Minute {
		t.Errorf("a stopped timer moved: left %s", tm.left)
	}
}

func TestLongGapWalksThePhases(t *testing.T) {
	tm := testTimer(true)
	tm.step(t0)
	tm.toggle(t0)
	tm.step(t0.Add(3*time.Hour + 17*time.Second)) // a laptop that slept
	if len(tm.sessions) < 6 {
		t.Errorf("only %d sessions after 3h", len(tm.sessions))
	}
	if tm.left <= 0 || tm.left > tm.total {
		t.Errorf("left %s of %s", tm.left, tm.total)
	}
	left, pos, n := tm.left, tm.pos, len(tm.sessions)
	tm.step(t0) // the clock went back: nothing happens
	if tm.left != left || tm.pos != pos || len(tm.sessions) != n {
		t.Errorf("a backwards clock changed the timer: left %s pos %d sessions %d", tm.left, tm.pos, len(tm.sessions))
	}
}

// A clock that goes back counts no time, and when it catches up the time
// before the jump is not counted a second time.
func TestBackwardsClockCountsNothingTwice(t *testing.T) {
	tm := testTimer(true)
	tm.step(t0)
	tm.toggle(t0)
	tm.step(t0.Add(10 * time.Minute))
	if tm.left != 15*time.Minute {
		t.Fatalf("left = %s after 10m, want 15m", tm.left)
	}
	tm.step(t0.Add(5 * time.Minute)) // the clock jumped back 5m
	if tm.left != 15*time.Minute {
		t.Errorf("left = %s after a backwards step, want 15m (no time passed)", tm.left)
	}
	tm.step(t0.Add(10*time.Minute + time.Second))
	if tm.left != 15*time.Minute-time.Second {
		t.Errorf("left = %s one second past the old instant, want 14m59s (nothing counted twice)", tm.left)
	}
}

// A key pressed during the done flash acts on the phase that follows it:
// the flash is finished first, so the cycle never goes backwards.
func TestResetDuringTheFlash(t *testing.T) {
	tm := testTimer(true)
	now := runFor(tm, 25*time.Minute)
	if tm.done <= 0 || tm.pos != 0 || len(tm.sessions) != 1 {
		t.Fatalf("not in the work flash: done %s pos %d sessions %d", tm.done, tm.pos, len(tm.sessions))
	}
	tm.reset(now)
	if tm.pos != 1 || tm.kind() != kindShort || tm.done != 0 || tm.running || tm.left != 5*time.Minute {
		t.Errorf("reset in the flash: pos %d kind %s done %s running %v left %s, want the short break stopped at 5m", tm.pos, tm.kind(), tm.done, tm.running, tm.left)
	}
	if len(tm.sessions) != 1 {
		t.Errorf("%d sessions, want the finished work phase kept once", len(tm.sessions))
	}
}

func TestSkipDuringTheFlash(t *testing.T) {
	for _, auto := range []bool{true, false} {
		tm := testTimer(auto)
		now := runFor(tm, 25*time.Minute)
		if tm.done <= 0 {
			t.Fatal("not in the flash")
		}
		tm.skip(now)
		// The break the flash was about to start is the one skipped.
		if tm.pos != 2 || tm.kind() != kindWork || tm.done != 0 || tm.left != 25*time.Minute {
			t.Errorf("auto=%v: skip in the flash: pos %d done %s left %s, want the next work phase", auto, tm.pos, tm.done, tm.left)
		}
		if tm.running != auto {
			t.Errorf("auto=%v: running = %v after the skip", auto, tm.running)
		}
		if len(tm.sessions) != 1 {
			t.Errorf("auto=%v: %d sessions, want 1", auto, len(tm.sessions))
		}
	}
}

// Whatever s or r does during a flash, the number of finished work phases
// the cycle dots show never drops. The flashes of the last work phase and of
// the long break are left out: a skip there reaches the end of the cycle,
// which legitimately starts over, just as a skip in the long break does.
func TestKeysInTheFlashNeverMoveTheDotsBack(t *testing.T) {
	doneDots := func(tm *timer) int {
		n := (tm.pos + 1) / 2
		if tm.done > 0 && tm.kind() == kindWork {
			n++
		}
		return n
	}
	for pos := 0; pos < seqLen-2; pos++ {
		for _, key := range []string{"s", "r"} {
			for _, auto := range []bool{true, false} {
				tm := testTimer(auto)
				tm.enter(pos)
				tm.step(t0)
				tm.toggle(t0)
				now := t0
				for tm.done == 0 {
					now = now.Add(time.Second)
					tm.step(now)
				}
				before := doneDots(tm)
				if key == "s" {
					tm.skip(now)
				} else {
					tm.reset(now)
				}
				if after := doneDots(tm); after < before {
					t.Errorf("pos %d key %s auto=%v: dots went from %d to %d", pos, key, auto, before, after)
				}
			}
		}
	}
}

func TestResetSkipAndMinutes(t *testing.T) {
	tm := testTimer(true)
	now := runFor(tm, 90*time.Second)
	tm.reset(now)
	if tm.running || tm.left != 25*time.Minute || tm.total != 25*time.Minute {
		t.Errorf("reset: running %v left %s total %s", tm.running, tm.left, tm.total)
	}
	if !tm.addMinute() || tm.left != 26*time.Minute || tm.total != 26*time.Minute {
		t.Errorf("addMinute: left %s total %s", tm.left, tm.total)
	}
	if !tm.subMinute() || !tm.subMinute() || tm.left != 24*time.Minute {
		t.Errorf("subMinute: left %s", tm.left)
	}
	tm.left = 30 * time.Second
	if tm.subMinute() {
		t.Error("subMinute with under a minute left should do nothing")
	}
	tm.total = maxPhase
	if tm.addMinute() {
		t.Error("addMinute at the cap should do nothing")
	}
	tm.reset(now)
	tm.skip(now)
	if tm.pos != 1 || tm.left != 5*time.Minute || len(tm.sessions) != 0 {
		t.Errorf("skip: pos %d left %s sessions %d (a skip records nothing)", tm.pos, tm.left, len(tm.sessions))
	}
	// Skipping keeps running state: stopped here, so it stays stopped.
	if tm.running {
		t.Error("skip started a stopped timer")
	}
}

func TestSessionsCarryTheTask(t *testing.T) {
	tm := testTimer(true)
	tm.task = "review the PR"
	runFor(tm, 25*time.Minute)
	tm.task = "ignored"
	runFor2 := t0.Add(25*time.Minute + 2*time.Second + 5*time.Minute)
	tm.step(runFor2)
	if len(tm.sessions) != 2 || tm.sessions[0].task != "review the PR" || tm.sessions[1].task != "" {
		t.Errorf("sessions = %+v", tm.sessions)
	}
}

// Every digit renders at the font's width and height, and a line of glyphs
// is the same width however many digits are blank or inked.
func TestFontWidths(t *testing.T) {
	for d := 0; d < 10; d++ {
		for r, row := range digitGlyphs[d] {
			if w := len([]rune(row)); w != fontWidth {
				t.Errorf("digit %d row %d is %d columns wide, want %d: %q", d, r, w, fontWidth, row)
			}
		}
		out := bigText(string(rune('0'+d)), true)
		lines := strings.Split(out, "\n")
		if len(lines) != fontRows {
			t.Errorf("digit %d has %d rows, want %d", d, len(lines), fontRows)
		}
		for _, l := range lines {
			if w := len([]rune(l)); w != fontWidth {
				t.Errorf("digit %d line %q is %d wide", d, l, w)
			}
		}
	}
	for r := 0; r < fontRows; r++ {
		if len([]rune(colonGlyph[r])) != colonWidth || len([]rune(blankColon[r])) != colonWidth {
			t.Errorf("colon row %d has the wrong width", r)
		}
	}
	for _, s := range []string{"00:00", "12:34", "59:59", "99:99", "00:00:00", "23:59:59", "8"} {
		for _, colon := range []bool{true, false} {
			lines := strings.Split(bigText(s, colon), "\n")
			if len(lines) != fontRows {
				t.Fatalf("%s: %d rows", s, len(lines))
			}
			for _, l := range lines {
				if w := len([]rune(l)); w != bigWidth(s) {
					t.Errorf("%s colon=%v: line %q is %d wide, want %d", s, colon, l, w, bigWidth(s))
				}
			}
		}
	}
	if w := bigWidth("12:34"); w != 4*fontWidth+colonWidth+4 {
		t.Errorf("bigWidth(12:34) = %d", w)
	}
	// The colon blinks off to blanks, not to a narrower string.
	on, off := bigText("12:34", true), bigText("12:34", false)
	if on == off || len([]rune(on)) != len([]rune(off)) {
		t.Error("the blinking colon changes the width, or does not change at all")
	}
}

// Digits are told apart: no two glyphs are the same.
func TestFontGlyphsAreDistinct(t *testing.T) {
	seen := map[string]int{}
	for d := 0; d < 10; d++ {
		k := strings.Join(digitGlyphs[d][:], "\n")
		if prev, dup := seen[k]; dup {
			t.Errorf("digits %d and %d look the same", prev, d)
		}
		seen[k] = d
	}
}
