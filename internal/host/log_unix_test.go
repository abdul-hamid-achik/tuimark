//go:build unix

package host

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Regression (review of 0.2b, SPEC v0.2b §26.12 File): only a regular file
// is truncated and narrowed to 0600. /dev/null and a FIFO are written as
// they are, with their modes untouched, so TUIMARK_LOG=/dev/null or a
// shell's process substitution works; a terminal is refused before Run
// touches anything, and keeps its mode; a symlink to a regular file is
// followed and its target narrowed.
func TestRunLogFileTypes(t *testing.T) {
	t.Run("dev null", func(t *testing.T) {
		before, err := os.Stat(os.DevNull)
		if err != nil {
			t.Skip(err)
		}
		lg, err := openRunLog(os.DevNull, time.Now())
		if err != nil {
			t.Fatalf("openRunLog(%s) = %v", os.DevNull, err)
		}
		lg.begin(80, 24)
		lg.key(Key{Name: "a"}, false)
		lg.end("quit")
		if after, _ := os.Stat(os.DevNull); after.Mode() != before.Mode() {
			t.Errorf("%s mode %v, was %v", os.DevNull, after.Mode(), before.Mode())
		}
	})
	t.Run("fifo", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "log.fifo")
		if err := syscall.Mkfifo(p, 0o644); err != nil {
			t.Skipf("mkfifo: %v", err)
		}
		if err := os.Chmod(p, 0o644); err != nil {
			t.Fatal(err)
		}
		got := make(chan string, 1)
		go func() {
			f, err := os.Open(p)
			if err != nil {
				got <- "open: " + err.Error()
				return
			}
			defer f.Close()
			b, _ := io.ReadAll(f)
			got <- string(b)
		}()
		lg, err := openRunLog(p, time.Now())
		if err != nil {
			t.Fatalf("openRunLog(fifo) = %v", err)
		}
		lg.begin(80, 24)
		lg.end("quit")
		select {
		case s := <-got:
			if !strings.HasPrefix(s, `{"ev":"start"`) || !strings.Contains(s, `{"ev":"end"`) || strings.Count(s, "\n") != 2 {
				t.Errorf("the FIFO received %q", s)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("the FIFO reader got nothing")
		}
		if st, _ := os.Stat(p); st.Mode().Perm() != 0o644 || st.Mode()&os.ModeNamedPipe == 0 {
			t.Errorf("FIFO mode %v, want 0644 kept", st.Mode())
		}
	})
	t.Run("terminal", func(t *testing.T) {
		master, name, err := openPTY()
		if err != nil {
			t.Skipf("no pty: %v", err)
		}
		defer master.Close()
		before, err := os.Stat(name)
		if err != nil {
			t.Fatal(err)
		}
		lg, err := openRunLog(name, time.Now())
		if err == nil {
			lg.end("quit")
			t.Fatalf("openRunLog(%s) accepted a terminal", name)
		}
		if !strings.Contains(err.Error(), envLog) || !strings.Contains(err.Error(), "terminal") {
			t.Errorf("error %v", err)
		}
		if after, _ := os.Stat(name); after.Mode() != before.Mode() {
			t.Errorf("terminal mode %v, was %v", after.Mode(), before.Mode())
		}
	})
	t.Run("symlink to a regular file", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "real.log")
		if err := os.WriteFile(target, []byte("stale\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(target, 0o644); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(dir, "link.log")
		if err := os.Symlink(target, link); err != nil {
			t.Skip(err)
		}
		lg, err := openRunLog(link, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		lg.begin(80, 24)
		lg.end("quit")
		st, _ := os.Stat(target)
		if st.Mode().Perm() != 0o600 {
			t.Errorf("target mode %v, want 0600", st.Mode().Perm())
		}
		recs := readLog(t, target)
		if len(recs) != 2 || recs[0]["ev"] != "start" || recs[1]["ev"] != "end" {
			t.Fatalf("records %v", recs)
		}
		// The start record carries the runtime version of the cut (SPEC
		// v0.2b §26.12 example, §24).
		if recs[0]["version"] != "0.2.0-b" {
			t.Errorf("start version %v, want 0.2.0-b", recs[0]["version"])
		}
	})
}
