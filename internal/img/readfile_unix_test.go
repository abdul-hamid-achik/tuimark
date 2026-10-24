//go:build unix

package img

import (
	"errors"
	"image"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// within runs f in a goroutine and returns its result, or fails the test
// when f takes longer than d (a read that blocks on a FIFO or never ends
// on /dev/zero is the bug). f must not touch t: after a timeout it is
// still running, and a t call after the test has ended panics or is lost;
// it hands back what the test body checks. A timed-out f is left to
// finish on its own (its result channel is buffered).
func within[T any](t *testing.T, d time.Duration, what string, f func() T) T {
	t.Helper()
	ch := make(chan T, 1)
	go func() { ch <- f() }()
	select {
	case v := <-ch:
		return v
	case <-time.After(d):
		t.Fatalf("%s did not return within %v", what, d)
	}
	panic("unreachable")
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
	type result struct {
		im  image.Image
		err error
	}
	for _, name := range []string{"fifo.png", "zero.png"} {
		got := within(t, 5*time.Second, "Image("+name+")", func() result {
			im, err := (&Registry{}).Image(name, dir)
			return result{im, err}
		})
		if got.im != nil || !errors.Is(got.err, ErrNotRegular) {
			t.Errorf("%s: image %v, err %v; want nil and ErrNotRegular", name, got.im, got.err)
		}
	}
}
