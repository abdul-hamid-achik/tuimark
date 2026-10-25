//go:build unix

package host

import (
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// A data-bound src naming a FIFO must not block the frame: the <img> takes
// the missing-image path (nothing painted, no diagnostic) and the dump
// returns.
func TestImgDataBoundSrcFIFOIsAMissingImage(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "pipe.png")
	if err := syscall.Mkfifo(fifo, 0o644); err != nil {
		t.Skipf("no mkfifo: %v", err)
	}
	a := doc(t, `<tui><screen><img id="p" src="{p}" width="4" height="2"/><text>after</text></screen></tui>`)
	if err := a.Bind("", map[string]any{"p": fifo}); err != nil {
		t.Fatal(err)
	}
	// The dump runs in a goroutine the test does not touch t from, so a
	// timeout (the bug: the frame blocked on the FIFO) fails the test
	// without a late t call from a goroutine still stuck in the read.
	type result struct {
		errs  int
		grid  string
		halfs int
	}
	ch := make(chan result, 1)
	go func() {
		d := a.Dump(20, 6, true)
		r := result{errs: len(d.Errors), grid: strings.Join(d.Grid, "\n")}
		for _, c := range d.Cells {
			if c.Ch == "▀" {
				r.halfs++
			}
		}
		ch <- r
	}()
	select {
	case r := <-ch:
		if r.errs != 0 {
			t.Errorf("%d diagnostics, want none", r.errs)
		}
		if !strings.Contains(r.grid, "after") {
			t.Errorf("the frame after the img was not drawn:\n%s", r.grid)
		}
		if r.halfs != 0 {
			t.Errorf("a FIFO painted %d halfblock cells", r.halfs)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Dump with a FIFO src did not return within 5s")
	}
}
