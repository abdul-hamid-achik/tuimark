//go:build unix

package host

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// boundedBy fails the test when f takes longer than d: a read that blocks on a
// FIFO or never ends on /dev/zero is the bug.
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

func TestReadRegularRefusesDevicesAndFIFOs(t *testing.T) {
	dir := t.TempDir()
	zero := filepath.Join(dir, "sample.json")
	if err := os.Symlink("/dev/zero", zero); err != nil {
		t.Fatal(err)
	}
	stdin := filepath.Join(dir, "stdin.json")
	if err := os.Symlink("/dev/stdin", stdin); err != nil {
		t.Fatal(err)
	}
	fifo := filepath.Join(dir, "x.tui")
	if err := syscall.Mkfifo(fifo, 0o644); err != nil {
		t.Skipf("no mkfifo: %v", err)
	}
	for _, p := range []string{zero, stdin, fifo, "/dev/null"} {
		boundedBy(t, 5*time.Second, "ReadRegular("+filepath.Base(p)+")", func() {
			if _, err := ReadRegular(p, 0); !errors.Is(err, ErrNotRegular) {
				t.Errorf("%s: %v, want ErrNotRegular", filepath.Base(p), err)
			}
		})
	}
}

// LoadBuffers reads a <style src> through the same check: a stylesheet
// that is a FIFO or a link to /dev/zero is a stylesheet that cannot be read
// (V006), not a hang.
func TestLoadBuffersRefusesUnsafeStylesheets(t *testing.T) {
	dir := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(dir, "fifo.tcss"), 0o644); err != nil {
		t.Skipf("no mkfifo: %v", err)
	}
	if err := os.Symlink("/dev/zero", filepath.Join(dir, "zero.tcss")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"fifo.tcss", "zero.tcss"} {
		doc := `<tui version="1"><style src="` + name + `"/><screen id="s"><text>x</text></screen></tui>` + "\n"
		boundedBy(t, 5*time.Second, "LoadBuffers("+name+")", func() {
			app, read := LoadBuffers(filepath.Join(dir, "app.tui"), []byte(doc), func(string) ([]byte, bool) { return nil, false })
			d := app.Validate()
			if len(read) != 0 || len(d) != 1 || d[0].Code != "V006" {
				t.Errorf("%s: read %+v, diagnostics %v; want V006 and no read", name, read, d)
			}
		})
	}
}
