//go:build !tuimark_verify

package host

// verifyFrames is set by the tuimark_verify build tag (verify_on.go): it
// checks every frame Frame and Dump give against a cold build. Off, the
// two hooks below are the plain builds.
const verifyFrames = false

func (a *App) verifiedFrame(cols, rows int, theme string) *Frame {
	return a.liveFrame(cols, rows, theme)
}

func (a *App) verifiedRender(cols, rows int, theme string) *Frame {
	return a.render(cols, rows, theme)
}
