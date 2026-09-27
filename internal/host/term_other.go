//go:build !unix

package host

import (
	"io"
	"os"
	"os/signal"
)

// stopSignals catches the interrupt that would otherwise kill the process
// with the terminal still raw. release restores its default action.
func stopSignals() (<-chan os.Signal, func()) {
	sig := make(chan os.Signal, 4)
	signal.Notify(sig, os.Interrupt)
	return sig, func() { signal.Stop(sig) }
}

// reraise ends the process after a stop signal Run could not honor in time
// (the terminal is already restored). There is no default action to
// re-raise here, so it exits with a failure status.
func reraise(os.Signal) { os.Exit(1) }

// inputReady has no polling here: every input is read with plain blocking
// reads.
func inputReady(io.Reader) func() bool { return nil }

// openNoCTTY has no meaning here (see term_unix.go).
const openNoCTTY = 0
