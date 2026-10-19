//go:build !js && !windows

package tuimark_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

// ADR 0034: a symlinked directory gives a fragment a second path to
// itself, and the include cycle is still found, by the real file. The
// messages keep the written paths; the symlink is never resolved in them.
// (LoadBuffers, the LSP's loader, is internal/host's TestSymlinkedIncludeCycleLoadBuffers.)
func TestSymlinkedIncludeCycle(t *testing.T) {
	dir := t.TempDir()
	write := func(name, text string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("app.tui", appWith(`<include src="frag.tui"/>`))
	write("frag.tui", "<fragment>\n  <include src=\"sub/frag.tui\"/>\n  <include src=\"sub/frag.tui\"/>\n</fragment>\n")
	if err := os.Symlink(".", filepath.Join(dir, "sub")); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}
	want := []string{
		`V006 frag.tui:2:3 /screen/include id= include src="sub/frag.tui" is an include cycle: app.tui → frag.tui → sub/frag.tui`,
		`V006 frag.tui:3:3 /screen/include id= include src="sub/frag.tui" is an include cycle: app.tui → frag.tui → sub/frag.tui`,
	}
	done := make(chan map[string][]string, 1)
	go func() {
		out := map[string][]string{}
		byLoad, err := tuimark.Load(filepath.Join(dir, "app.tui"))
		if err != nil {
			t.Error(err)
		} else {
			out["Load"] = diagTexts(byLoad.Validate())
		}
		byFS, err := tuimark.LoadFS(os.DirFS(dir), "app.tui")
		if err != nil {
			t.Error(err)
		} else {
			out["LoadFS"] = diagTexts(byFS.Validate())
		}
		done <- out
	}()
	select {
	case out := <-done:
		for name, got := range out {
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%s:\n%v\nwant\n%v", name, got, want)
			}
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the symlinked include cycle is not found: the expansion hangs")
	}
}
