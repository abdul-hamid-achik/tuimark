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

// SPEC v0.3b §15.8 "End": the server exits 1 when writing stdout fails.
// A client that closes its end of stdout makes the next write fail with
// EPIPE; by default the Go runtime turns a broken pipe on fd 1 into a
// SIGPIPE death (no exit code, no stderr line), so the server has to
// ignore SIGPIPE to report it. This needs a real OS pipe on fd 1 (an
// io.Pipe never raises SIGPIPE), so the child is a re-run of this test
// binary (TestHostSubcommandChild) running `tuimark mcp`.
func TestMCPStdoutClosedExitsOne(t *testing.T) {
	if os.Getenv(hostPtyChildEnv) != "" {
		t.Skip("child process")
	}
	cmd := hostChildCmd([]string{"mcp"}, hostEnv())
	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stderr := &syncBuf{}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = inR, outW, stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	_ = inR.Close()
	_ = outW.Close()
	// The client goes away: nobody reads the server's stdout any more.
	_ = outR.Close()
	if _, err := inW.Write([]byte(`{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n")); err != nil {
		t.Fatal(err)
	}
	_ = inW.Close()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var werr error
	select {
	case werr = <-done:
	case <-time.After(10 * slowdown * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("the server did not exit")
	}
	var ee *exec.ExitError
	if !errors.As(werr, &ee) {
		t.Fatalf("exit: %v, want exit status 1 (stderr %q)", werr, stderr.String())
	}
	if ee.ExitCode() != 1 {
		t.Fatalf("exit: %v (code %d), want exit status 1, not a signal (stderr %q)", werr, ee.ExitCode(), stderr.String())
	}
	if got := stderr.String(); !strings.HasPrefix(got, "tuimark: ") || !strings.Contains(got, "broken pipe") || strings.Count(got, "\n") != 1 {
		t.Errorf("stderr %q, want one \"tuimark: …broken pipe\" line", got)
	}
}
