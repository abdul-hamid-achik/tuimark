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
	boundedBy(t, 5*time.Second, "Dump with a FIFO src", func() {
		d := a.Dump(20, 6, true)
		if len(d.Errors) != 0 {
			t.Errorf("errors: %v", d.Errors)
		}
		if got := strings.Join(d.Grid, "\n"); !strings.Contains(got, "after") {
			t.Errorf("the frame after the img was not drawn:\n%s", got)
		}
		for _, c := range d.Cells {
			if c.Ch == "▀" {
				t.Errorf("a FIFO painted halfblocks")
			}
		}
	})
}
