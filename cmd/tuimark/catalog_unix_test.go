//go:build unix

package main

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// SPEC v0.4a §15.10: `tuimark catalog` exits 1 when stdout cannot be
// written, as `tuimark catalog | head -c 1` makes it, not with the SIGPIPE
// death the Go runtime gives a broken pipe on fd 1 by default. Like
// TestMCPStdoutClosedExitsOne, the child is a re-run of this test binary,
// since only a real OS pipe raises SIGPIPE.
func TestCatalogStdoutClosedExitsOne(t *testing.T) {
	if os.Getenv(hostPtyChildEnv) != "" {
		t.Skip("child process")
	}
	cmd := hostChildCmd([]string{"catalog"}, hostEnv())
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stderr := &syncBuf{}
	cmd.Stdout, cmd.Stderr = outW, stderr
	// The reader is gone before the first write.
	_ = outR.Close()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	_ = outW.Close()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var werr error
	select {
	case werr = <-done:
	case <-time.After(10 * slowdown * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("catalog did not exit")
	}
	var ee *exec.ExitError
	if !errors.As(werr, &ee) || ee.ExitCode() != 1 {
		t.Fatalf("exit: %v, want exit status 1, not a signal (stderr %q)", werr, stderr.String())
	}
	if got := stderr.String(); !strings.HasPrefix(got, "tuimark: ") || !strings.Contains(got, "broken pipe") {
		t.Errorf("stderr %q, want a \"tuimark: …broken pipe\" line", got)
	}
}
