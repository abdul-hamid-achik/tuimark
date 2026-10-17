//go:build tuimark_verify

package host

import "fmt"

// verifyFrames checks every frame Frame and Dump give against a cold build
// from the same state (coldRender) and panics on the first difference:
// `go test -tags tuimark_verify ./...` runs the whole suite that way, and
// a tuimark binary built with the tag checks every golden and play step.
const verifyFrames = true

// verifiedFrame is Frame's build (liveFrame) checked against a cold
// render from the state it started from: the same frame, the same events
// queued, and the same state left behind. A skipped build is checked the
// same way, which is the frame skip's own premise.
func (a *App) verifiedFrame(cols, rows int, theme string) *Frame {
	if a.noCache {
		return a.liveFrame(cols, rows, theme)
	}
	in := a.snapAll()
	nPending := len(a.pending)
	f := a.liveFrame(cols, rows, theme)
	warmPending := append([]Event(nil), a.pending[nPending:]...)
	warmAfter := a.frameInputs(cols, rows, theme)
	g, coldPending, coldAfter := a.coldRender(in, cols, rows, theme)
	a.mustMatch("Frame", f, g, warmPending, coldPending[nPending:], warmAfter, coldAfter)
	return f
}

// verifiedRender is Dump's render checked the same way.
func (a *App) verifiedRender(cols, rows int, theme string) *Frame {
	if a.noCache {
		return a.render(cols, rows, theme)
	}
	in := a.snapAll()
	nPending := len(a.pending)
	f := a.render(cols, rows, theme)
	warmPending := append([]Event(nil), a.pending[nPending:]...)
	warmAfter := a.frameInputs(cols, rows, theme)
	g, coldPending, coldAfter := a.coldRender(in, cols, rows, theme)
	a.mustMatch("Dump", f, g, warmPending, coldPending[nPending:], warmAfter, coldAfter)
	return f
}

func (a *App) mustMatch(what string, f, g *Frame, wp, cp []Event, wa, ca string) {
	if d := FrameDiff(f, g); d != "" {
		panic(fmt.Sprintf("tuimark_verify: %s: the cached frame differs from a cold build: %s", what, d))
	}
	if d := eventsDiff(wp, cp); d != "" {
		panic(fmt.Sprintf("tuimark_verify: %s: %s", what, d))
	}
	if wa != ca {
		panic(fmt.Sprintf("tuimark_verify: %s: the state left behind differs:\n  warm: %q\n  cold: %q", what, wa, ca))
	}
}
