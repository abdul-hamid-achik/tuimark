//go:build !tuimark_verify

package layout

// verifying is set by the tuimark_verify build tag (verify_on.go).
const verifying = false

type checkSnap struct{}

func (e *Engine) snapChecks() checkSnap { return checkSnap{} }

func (e *Engine) verifyChecks(*Box, []*Box, int, checkSnap) {}
