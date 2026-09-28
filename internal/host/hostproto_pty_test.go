//go:build unix

package host

import (
	"fmt"
	"os"
	"testing"
	"time"
)

const runHostPtyDoc = `<tui version="1">
<keymap><bind keys="z" action="quit"/></keymap>
<screen id="s"><text>RUNHOST</text></screen>
</tui>`

// runHostPtyChild runs RunHost on the real pty and reports its HostResult
// on stderr. mode "quit" leaves HostHooks zero (ctrl+z/"z" quits as
// usual); mode "stopeof" sends a HostStop{EOF: true} after the first
// frame, as cmd/tuimark's host bridge does on fd 3 EOF (SPEC v0.3b
// §19.1).
func runHostPtyChild(t *testing.T, mode string) {
	signalGrace = time.Second
	a := doc(t, runHostPtyDoc)
	var hooks HostHooks
	if mode == "stopeof" {
		stop := make(chan HostStop, 1)
		hooks.Stop = stop
		go func() {
			time.Sleep(300 * time.Millisecond)
			stop <- HostStop{EOF: true}
		}()
	}
	res := a.RunHost(os.Stdout, hooks)
	fmt.Fprintf(os.Stderr, "RESULT reason=%s started=%v err=%v signal=%v\n", res.Reason, res.Started, res.Err, res.Signal)
	os.Exit(0)
}

// RunHost's result names why the session ended (SPEC v0.3b §19.1): "quit"
// for the built-in quit, "eof" for a HostStop{EOF: true} (the bridge's
// translation of fd 3 reaching end of file), distinct from each other and
// from a plain Run() caller's nil-conflates-both return.
func TestRunHostResult(t *testing.T) {
	if mode := os.Getenv(ptyChildEnv); mode != "" {
		runHostPtyChild(t, mode)
		return
	}
	t.Run("quit", func(t *testing.T) {
		r := startPtyChild(t, "TestRunHostResult", "quit")
		if !r.waitFor(r.screen, "RUNHOST", 10*time.Second) {
			t.Fatalf("the app never rendered; %s", r.describe())
		}
		r.write(t, "z")
		if !r.waitFor(r.stderr, "RESULT reason=quit started=true", 5*time.Second) {
			t.Fatalf("RunHost did not report reason=quit; %s", r.describe())
		}
	})
	t.Run("stopeof", func(t *testing.T) {
		r := startPtyChild(t, "TestRunHostResult", "stopeof")
		if !r.waitFor(r.screen, "RUNHOST", 10*time.Second) {
			t.Fatalf("the app never rendered; %s", r.describe())
		}
		if !r.waitFor(r.stderr, "RESULT reason=eof started=true", 5*time.Second) {
			t.Fatalf("RunHost did not report reason=eof for HostStop{EOF: true}; %s", r.describe())
		}
	})
}
