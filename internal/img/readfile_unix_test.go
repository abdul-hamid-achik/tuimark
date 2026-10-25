//go:build unix

package img

import (
	"errors"
	"image"
	"net"
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

// A unix socket cannot be opened for reading (ENXIO, EOPNOTSUPP on macOS); ReadRegular still
// calls it what it is, ErrNotRegular, as when the path was checked first.
func TestReadRegularCallsASocketNotRegular(t *testing.T) {
	dir, err := os.MkdirTemp("", "s")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sock := filepath.Join(dir, "s.sock")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Skipf("no unix sockets: %v", err)
	}
	defer l.Close()
	err = within(t, 5*time.Second, "ReadRegular(socket)", func() error {
		_, err := ReadRegular(sock, 0)
		return err
	})
	if !errors.Is(err, ErrNotRegular) {
		t.Errorf("socket: %v, want ErrNotRegular", err)
	}
}

// ReadRegular decides by the opened descriptor, not by a look at the path
// beforehand: while a path flips between a link to a regular file and a
// link to a FIFO, every call either reads the file's exact bytes, is
// ErrNotRegular, or fails to open the path mid-swap, and none blocks. (A race cannot fail the old
// check-then-open code deterministically, since it also verified the
// descriptor; this pins the contract of the descriptor-first opener.)
func TestReadRegularSurvivesAPathSwap(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "real.png")
	if err := os.WriteFile(file, []byte("PAYLOAD"), 0o644); err != nil {
		t.Fatal(err)
	}
	fifo := filepath.Join(dir, "pipe")
	if err := syscall.Mkfifo(fifo, 0o644); err != nil {
		t.Skipf("no mkfifo: %v", err)
	}
	link := filepath.Join(dir, "swap")
	stop := make(chan struct{})
	swapped := make(chan struct{})
	go func() {
		defer close(swapped)
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			target, tmp := file, filepath.Join(dir, "tmp")
			if i%2 == 1 {
				target = fifo
			}
			os.Remove(tmp)
			if os.Symlink(target, tmp) == nil {
				os.Rename(tmp, link) // atomic: the path is never absent
			}
		}
	}()
	defer func() { close(stop); <-swapped }()
	for i := 0; i < 2000; i++ {
		if _, err := os.Lstat(link); err != nil {
			time.Sleep(time.Millisecond) // the swapper has not made it yet
			continue
		}
		type result struct {
			data []byte
			err  error
		}
		got := within(t, 5*time.Second, "ReadRegular(swapping path)", func() result {
			d, err := ReadRegular(link, 0)
			return result{d, err}
		})
		switch {
		case got.err == nil && string(got.data) == "PAYLOAD":
		case errors.Is(got.err, ErrNotRegular):
		case errors.Is(got.err, os.ErrNotExist):
		case got.data == nil && errors.Is(got.err, syscall.EINVAL):
			// macos-latest CI has failed this open with EINVAL mid-swap
			// (not reproduced on a local macOS; the likeliest cause is the
			// path walk losing the link being replaced, as ENOENT does
			// elsewhere). The open failed, so nothing was read and nothing
			// could block, which is the contract this test pins.
		default:
			t.Fatalf("call %d: %q, %v; want the file's bytes or ErrNotRegular", i, got.data, got.err)
		}
	}
}

// A FIFO, a link to a device and a socket already in place are refused by
// the Stat before the open: none of them is opened (v0.5.2 opened each
// one and refused it by fstat, and opening some devices has side
// effects). A regular file is opened once.
func TestReadRegularOpensOnlyARegularFile(t *testing.T) {
	dir, err := os.MkdirTemp("", "o")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	opens := 0
	openFile = func(name string, flag int, perm os.FileMode) (*os.File, error) {
		opens++
		return os.OpenFile(name, flag, perm)
	}
	t.Cleanup(func() { openFile = os.OpenFile })

	if err := syscall.Mkfifo(filepath.Join(dir, "fifo"), 0o644); err != nil {
		t.Skipf("no mkfifo: %v", err)
	}
	for name, target := range map[string]string{"null": "/dev/null", "zero": "/dev/zero"} {
		if err := os.Symlink(target, filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	special := []string{"fifo", "null", "zero"}
	if l, err := net.Listen("unix", filepath.Join(dir, "s.sock")); err == nil {
		defer l.Close()
		special = append(special, "s.sock")
	}
	for _, name := range special {
		opens = 0
		err := within(t, 5*time.Second, "ReadRegular("+name+")", func() error {
			_, err := ReadRegular(filepath.Join(dir, name), 0)
			return err
		})
		if !errors.Is(err, ErrNotRegular) || opens != 0 {
			t.Errorf("%s: err %v after %d opens; want ErrNotRegular and no open", name, err, opens)
		}
	}

	file := filepath.Join(dir, "real.png")
	if err := os.WriteFile(file, []byte("PAYLOAD"), 0o644); err != nil {
		t.Fatal(err)
	}
	opens = 0
	if data, err := ReadRegular(file, 0); err != nil || string(data) != "PAYLOAD" || opens != 1 {
		t.Errorf("regular file: %q, %v after %d opens; want PAYLOAD after one", data, err, opens)
	}
}
