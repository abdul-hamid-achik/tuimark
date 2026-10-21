//go:build !js && !windows

package host

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// ADR 0034 through LoadBuffers (the LSP's loader): the symlinked include
// cycle is found, by the real file, and quickly; the names are the written
// paths, absolute as LoadBuffers names every file.
func TestSymlinkedIncludeCycleLoadBuffers(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"app.tui":  "<tui>\n  <screen id=\"s\"><include src=\"frag.tui\"/></screen>\n</tui>\n",
		"frag.tui": "<fragment>\n  <include src=\"sub/frag.tui\"/>\n  <include src=\"sub/frag.tui\"/>\n</fragment>\n",
	})
	if err := os.Symlink(".", filepath.Join(dir, "sub")); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}
	done := make(chan []string, 1)
	go func() {
		app, _ := LoadBuffers(filepath.Join(dir, "app.tui"), []byte("<tui>\n  <screen id=\"s\"><include src=\"frag.tui\"/></screen>\n</tui>\n"), func(string) ([]byte, bool) { return nil, false })
		done <- diagLines(app)
	}()
	frag := filepath.Join(dir, "frag.tui")
	chain := "app.tui → " + frag + " → " + filepath.Join(dir, "sub/frag.tui")
	want := []string{
		"V006 " + frag + ":2:3 include src=\"sub/frag.tui\" is an include cycle: " + chain,
		"V006 " + frag + ":3:3 include src=\"sub/frag.tui\" is an include cycle: " + chain,
	}
	select {
	case got := <-done:
		if !reflect.DeepEqual(got, want) {
			t.Errorf("diagnostics:\n%v\nwant\n%v", got, want)
		}
	case <-time.After(30 * slowdown * time.Second):
		t.Fatal("LoadBuffers hangs on the symlinked include cycle")
	}
}
