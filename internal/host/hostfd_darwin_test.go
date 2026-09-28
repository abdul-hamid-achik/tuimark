//go:build darwin

package host

import (
	"testing"

	"golang.org/x/sys/unix"
)

// SPEC v0.3b §19.1: on darwin a kqueue instance (what the Go runtime's own
// poller opens, and what would land at fd 3 for a parent that passes
// nothing) reports as a FIFO under fstat like a real pipe, so the poller
// check must reject it by its kevent behavior, not its file type.
func TestValidateHostFDRejectsKqueue(t *testing.T) {
	kq, err := unix.Kqueue()
	if err != nil {
		t.Skipf("no kqueue: %v", err)
	}
	defer unix.Close(kq)
	if !isPollerFD(kq) {
		t.Fatal("isPollerFD did not recognize a real kqueue")
	}
	if err := validateHostFD(kq, true); err == nil {
		t.Error("a kqueue fd passed validateHostFD")
	}
}
