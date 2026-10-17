//go:build !linux

package host

import "github.com/abdul-hamid-achik/tuimark/internal/layout"

// adviseHuge does nothing outside Linux (hugeslab_linux.go).
func adviseHuge([]layout.Box) {}
