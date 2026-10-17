//go:build unix

package lsp

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// boundedBy fails the test when f takes longer than d.
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

// A sample.json that is a link to /dev/zero, or to /dev/stdin (which a
// cloned repository can carry), is absent: the server neither grows
// without bound nor reads its own protocol stream.
func TestUnsafeSampleJSONIsAbsent(t *testing.T) {
	dir := t.TempDir()
	sn := &snapshot{open: map[string]string{}, byURI: map[string]*document{}}
	for _, target := range []string{"/dev/zero", "/dev/stdin", "/dev/random"} {
		link := filepath.Join(dir, sampleName)
		os.Remove(link)
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		boundedBy(t, 5*time.Second, "readSample("+target+")", func() {
			if _, ok := readSample(sn, filepath.Join(dir, "a.tui")); ok {
				t.Errorf("a sample.json linked to %s was read", target)
			}
		})
	}
	// A FIFO named sample.json blocks no one either.
	os.Remove(filepath.Join(dir, sampleName))
	if err := syscall.Mkfifo(filepath.Join(dir, sampleName), 0o644); err != nil {
		t.Skipf("no mkfifo: %v", err)
	}
	boundedBy(t, 5*time.Second, "readSample(fifo)", func() {
		if _, ok := readSample(sn, filepath.Join(dir, "a.tui")); ok {
			t.Error("a FIFO sample.json was read")
		}
	})
}

// A whole session over a workspace that carries a sample.json linked to
// /dev/zero and a FIFO named x.tui in the stylesheet's directory: the
// diagnostics arrive, quickly, and the shutdown is clean.
func TestSessionOverUnsafeWorkspaceFiles(t *testing.T) {
	doc := `<tui version="1"><style src="theme.tcss"/><screen id="s"><text id="t">{name}</text></screen></tui>` + "\n"
	dir := writeTree(t, map[string]string{"theme.tcss": "#t { bogus-prop: 1; }\n"})
	if err := os.Symlink("/dev/zero", filepath.Join(dir, sampleName)); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(dir, "x.tui"), 0o644); err != nil {
		t.Skipf("no mkfifo: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "d.tui"), 0o755); err != nil {
		t.Fatal(err)
	}
	c := startServer(t, Options{Debounce: 10 * time.Millisecond})
	c.initialize()
	start := time.Now()
	docURI := c.open(filepath.Join(dir, "a.tui"), "tuimark", doc)
	sheetURI := c.open(filepath.Join(dir, "theme.tcss"), "tcss", "#t { bogus-prop: 1; }\n")
	if p := c.nextPublish(docURI); len(p.Diagnostics) != 0 {
		t.Errorf("document: %+v, want none (no data, so no B003)", p.Diagnostics)
	}
	if p := c.nextPublish(sheetURI); len(p.Diagnostics) != 1 || p.Diagnostics[0].Code != "V003" {
		t.Errorf("stylesheet: %+v, want its V003", p.Diagnostics)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("the diagnostics took %v", d)
	}
	if code := c.shutdown(); code != 0 {
		t.Errorf("exit %d, want 0", code)
	}
}
