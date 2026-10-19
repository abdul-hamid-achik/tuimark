package host

import (
	"runtime"
	"testing"
)

// skipOnJS skips a test that needs what js/wasm cannot give a process: a
// pipe, a real signal, or a terminal's read speed. The core packages are
// tested under GOOS=js GOARCH=wasm (cmd/tuimark-wasm, ADR 0033); the Run
// loop and the CLI are not part of that target.
func skipOnJS(t *testing.T, why string) {
	t.Helper()
	if runtime.GOOS == "js" {
		t.Skip("js/wasm: " + why)
	}
}
