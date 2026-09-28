package host

import "fmt"

// modeRun and modePlay are the values App.mode takes while Run/Loop or
// Play is driving the app (SPEC v0.3 §18.1 Play rule 8: "Play and Run
// exclude each other"). "" is idle.
const (
	modeRun  = "run"
	modePlay = "play"
)

// enterMode claims mode for the caller, or reports which mode is already
// active. The caller must not hold a.mu.
func (a *App) enterMode(mode string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.mode != "" {
		return fmt.Errorf("tuimark: %s cannot start while %s is active", verbFor(mode), verbFor(a.mode))
	}
	a.mode = mode
	return nil
}

// exitMode releases the mode a matching enterMode claimed.
func (a *App) exitMode() {
	a.mu.Lock()
	a.mode = ""
	a.mu.Unlock()
}

func verbFor(mode string) string {
	if mode == modePlay {
		return "Play"
	}
	return "Run"
}

// EnterPlay marks the app as driven by Play (SPEC v0.3 §18.1 rule 8): an
// error, doing nothing, when Run, Loop, or another Play is already
// active. It is not part of the public tuimark API; internal/play calls
// it around one Play call.
func (a *App) EnterPlay() error { return a.enterMode(modePlay) }

// ExitPlay clears the mode a matching EnterPlay claimed.
func (a *App) ExitPlay() { a.exitMode() }
