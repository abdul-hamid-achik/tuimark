//go:build unix

package host

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// Test 194 through run() itself (SPEC v0.4b §26.13): Run in this process,
// on a pty, with TUIMARK_COLOR, TUIMARK_SYNC, and TUIMARK_LOG set, and the
// fake clock in through testRunClock. The environment-to-profile mapping,
// run()'s own start, the log's clock, and the spinner's clock are all
// the ones a real Run uses. Nothing sleeps to wait for state: every wait
// blocks on the pty's output.

// waitBuf is what the terminal received, with a wait for more of it.
type waitBuf struct {
	mu   sync.Mutex
	cond *sync.Cond
	b    bytes.Buffer
}

func newWaitBuf() *waitBuf {
	w := &waitBuf{}
	w.cond = sync.NewCond(&w.mu)
	return w
}

func (w *waitBuf) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.b.Write(p)
	w.cond.Broadcast()
	return len(p), nil
}

// waitFor blocks until the output past from holds want, and returns the
// position after it.
func (w *waitBuf) waitFor(t *testing.T, from int, want string) int {
	t.Helper()
	w.mu.Lock()
	defer w.mu.Unlock()
	timedOut := false
	stop := time.AfterFunc(10*time.Second, func() {
		w.mu.Lock()
		timedOut = true
		w.cond.Broadcast()
		w.mu.Unlock()
	})
	defer stop.Stop()
	for {
		if i := strings.Index(w.b.String()[from:], want); i >= 0 {
			return from + i + len(want)
		}
		if timedOut {
			t.Fatalf("timed out waiting for %q; got %q", want, w.b.String()[from:])
		}
		w.cond.Wait()
	}
}

func TestRunClockThroughRun(t *testing.T) {
	ascii := []string{"|", "/", "-", "\\"}
	for _, c := range []struct {
		color string
		set   []string
	}{{"truecolor", braille}, {"16", ascii}} {
		t.Run(c.color, func(t *testing.T) {
			throughRun(t, c.color, c.set)
		})
	}
}

func throughRun(t *testing.T, color string, set []string) {
	master, name, err := openPTY()
	if err != nil {
		t.Skipf("no pty: %v", err)
	}
	slave, err := os.OpenFile(name, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		master.Close()
		t.Skipf("cannot open the pty slave %s: %v", name, err)
	}
	if err := unix.IoctlSetWinsize(int(slave.Fd()), unix.TIOCSWINSZ, &unix.Winsize{Row: 5, Col: 20}); err != nil {
		t.Fatal(err)
	}
	screen := newWaitBuf()
	go func() { _, _ = io.Copy(screen, master) }()
	t.Cleanup(func() { slave.Close(); master.Close() })

	// The environment Run reads (§26.10), pinned.
	for _, k := range terminalEnv {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
	logPath := filepath.Join(t.TempDir(), "run.ndjson")
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("TUIMARK_COLOR", color)
	t.Setenv("TUIMARK_SYNC", "0")
	t.Setenv("TUIMARK_LOG", logPath)

	clk := newFakeClock()
	testRunClock = clk
	t.Cleanup(func() { testRunClock = nil })
	oldStdin := os.Stdin
	os.Stdin = slave
	t.Cleanup(func() { os.Stdin = oldStdin })

	a := doc(t, spinDoc("", ""))
	done := make(chan error, 1)
	go func() { done <- a.Run(slave) }()

	// The probe: answer it, so the first frame does not wait for its cap.
	pos := screen.waitFor(t, 0, "\x1b[c")
	if _, err := master.WriteString(ptyV04aProbe); err != nil {
		t.Fatal(err)
	}
	pos = screen.waitFor(t, pos, set[0])
	for i := 1; i <= 10; i++ {
		// A phase boundary of the fake clock: the spinner turns once. The
		// loop arms its timer after it writes a frame, so wait for the
		// timer to be armed for this boundary before moving the clock.
		clk.waitArmed(t, time.Duration(i)*100*time.Millisecond)
		clk.Advance(t, 100*time.Millisecond)
		pos = screen.waitFor(t, pos, set[i%len(set)])
	}
	// An unbound key and the built-in quit; the key's log record carries
	// the fake clock's t.
	if _, err := master.WriteString("x\x03"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return")
	}

	recs := readLog(t, logPath)
	var evs []string
	for _, r := range recs {
		evs = append(evs, r["ev"].(string))
	}
	if got, want := strings.Join(evs, " "), "start caps frame key key action end"; got != want {
		t.Fatalf("log records = %s, want %s", got, want)
	}
	if recs[1]["color"] != color {
		t.Errorf("caps color = %v, want %s", recs[1]["color"], color)
	}
	if recs[3]["key"] != "x" || recs[3]["t"] != float64(1000) {
		t.Errorf("key record %v, want x at t = 1000 (ten advances of the fake clock)", recs[3])
	}
}
