//go:build windows

package host

// resizeSignal has no signal source on Windows; Loop polls the size.
func resizeSignal(stop <-chan struct{}) <-chan struct{} { return nil }
