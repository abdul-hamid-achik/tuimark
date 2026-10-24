//go:build unix

package img

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// boundedBy fails the test when f takes longer than d: a read that blocks
// on a FIFO or never ends on /dev/zero is the bug.
func boundedBy(t *testing.T, d time.Duration, what string, f func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { f(); close(done) }()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("%s did not return within %v", what, d)
	}
}

// A data-bound src (0.5.1) can name any path. A FIFO, a device and a link
// to /dev/zero are errors, never a block of the frame loop or an unbounded
// read (v0.5.2: Image used os.ReadFile).
func TestImageRefusesSpecialFiles(t *testing.T) {
	dir := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(dir, "fifo.png"), 0o644); err != nil {
		t.Skipf("no mkfifo: %v", err)
	}
	if err := os.Symlink("/dev/zero", filepath.Join(dir, "zero.png")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"fifo.png", "zero.png"} {
		boundedBy(t, 5*time.Second, "Image("+name+")", func() {
			im, err := (&Registry{}).Image(name, dir)
			if im != nil || !errors.Is(err, ErrNotRegular) {
				t.Errorf("%s: image %v, err %v; want nil and ErrNotRegular", name, im, err)
			}
		})
	}
}
