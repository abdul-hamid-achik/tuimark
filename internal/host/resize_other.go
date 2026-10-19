//go:build !unix

package host

// resizeSignal has no signal source where there is no SIGWINCH (Windows,
// js/wasm): Loop polls the size, and js/wasm never calls Run.
func resizeSignal(stop <-chan struct{}) <-chan struct{} { return nil }
