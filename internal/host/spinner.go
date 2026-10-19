package host

import "time"

// SpinnerPeriod is the length of one spinner phase (SPEC v0.4b §12.6):
// the phase of a clock reading t is floor(t / SpinnerPeriod).
const SpinnerPeriod = 100 * time.Millisecond

// PhaseOf returns the spinner phase of the clock reading t: floor(t /
// 100 ms), 0 for a negative t (SPEC v0.4b §12.6). Every tool that turns
// spinners reads its own clock through it: play's virtual clock (V), and
// Run's session clock (§26.13). The glyph itself is paint.Glyph.
func PhaseOf(t time.Duration) int {
	if t <= 0 {
		return 0
	}
	return int(t / SpinnerPeriod)
}

// VPhase is the spinner phase of the app's virtual clock (SPEC v0.4b
// §15.4.1): what a Play draws its frames at.
func (a *App) VPhase() int { return PhaseOf(a.VClock()) }
