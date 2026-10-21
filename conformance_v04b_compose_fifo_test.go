//go:build !js && !windows

package tuimark_test

import (
	"os"
	"path/filepath"
	"reflect"
	"syscall"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

// An include of a FIFO is refused, not waited on: V006 "not a regular
// file", under Load and under LoadFS (review of phase F1, round 3).
func TestIncludeOfAFIFOIsRefused(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.tui"), []byte(appWith(`<include src="pipe.tui"/>`)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(dir, "pipe.tui"), 0o644); err != nil {
		t.Skipf("no FIFOs here: %v", err)
	}
	const want = `V006 app.tui:3:5 /screen/include id= include src="pipe.tui" cannot be read: not a regular file`
	done := make(chan map[string][]string, 1)
	go func() {
		out := map[string][]string{}
		byLoad, err := tuimark.Load(filepath.Join(dir, "app.tui"))
		if err == nil {
			out["Load"] = diagTexts(byLoad.Validate())
		}
		byFS, err := tuimark.LoadFS(os.DirFS(dir), "app.tui")
		if err == nil {
			out["LoadFS"] = diagTexts(byFS.Validate())
		}
		done <- out
	}()
	select {
	case out := <-done:
		for name, got := range out {
			if !reflect.DeepEqual(got, []string{want}) {
				t.Errorf("%s: %v", name, got)
			}
		}
		if len(out) != 2 {
			t.Errorf("only %d loaders answered", len(out))
		}
	case <-time.After(20 * slowdown * time.Second):
		t.Fatal("an include of a FIFO hangs")
	}
}
