//go:build !tuimark_verify

package host

import "github.com/abdul-hamid-achik/tuimark/internal/layout"

// verifyFrames is set by the tuimark_verify build tag (verify_on.go): it
// checks every frame Frame and Dump give against a cold build. Off, the
// two hooks below are the plain builds, and checkReuse checks nothing.
const verifyFrames = false

func (a *App) verifiedFrame(cols, rows int, theme string) *Frame {
	return a.liveFrame(cols, rows, theme)
}

func (a *App) verifiedRender(cols, rows int, theme string) *Frame {
	return a.render(cols, rows, theme)
}

func checkReuse(*listRows, *layout.Box, []*layout.Box) {}
