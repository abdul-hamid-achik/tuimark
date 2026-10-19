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

// HasSpinner reports whether the document holds a spinner anywhere (every
// screen, modal, and tab, a list item or an each template included). Run
// reads the phase of its clock only for such a document: the phase is an
// input of the frame (SPEC v0.4b §26.13), so a document without a
// spinner builds every frame at phase 0 and keeps the frame skip.
func (a *App) HasSpinner() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.kinds().spinner
}
