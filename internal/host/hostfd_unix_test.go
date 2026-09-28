//go:build unix

package host

import (
	"os"
	"testing"
)

// SPEC v0.3b §19.1: fd 3/4 are checked as a pipe or a socket, not a
// poller instance, fd 3 open for reading and fd 4 for writing, before
// anything else. This test drives validateHostFD directly (fd 3 and fd 4
// are fixed numbers ValidateHostFDs itself cannot fake in a unit test).
func TestValidateHostFD(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()

	if err := validateHostFD(int(r.Fd()), true); err != nil {
		t.Errorf("a real pipe read end, wantRead: %v", err)
	}
	if err := validateHostFD(int(w.Fd()), false); err != nil {
		t.Errorf("a real pipe write end, wantWrite: %v", err)
	}
	if err := validateHostFD(int(r.Fd()), false); err == nil {
		t.Error("a read-only pipe end passed wantWrite")
	}
	if err := validateHostFD(int(w.Fd()), true); err == nil {
		t.Error("a write-only pipe end passed wantRead")
	}

	// A regular file is neither a pipe nor a socket.
	f, err := os.CreateTemp(t.TempDir(), "hostfd")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := validateHostFD(int(f.Fd()), true); err == nil {
		t.Error("a regular file passed the pipe/socket check")
	}

	// A closed fd is "not open".
	cr, cw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	fd := int(cr.Fd())
	_ = cr.Close()
	_ = cw.Close()
	if err := validateHostFD(fd, true); err == nil {
		t.Error("a closed fd passed the check")
	}
}
