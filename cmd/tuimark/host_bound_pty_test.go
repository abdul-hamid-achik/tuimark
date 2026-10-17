//go:build unix

package main

// SPEC v0.4a §21 test 116 (host part), end to end: a line of 64 MiB or more
// on fd 3 ends `tuimark host` with exit reason error, and the terminal is
// restored (§19.1 "Line bound").

import (
	"bytes"
	"os"
	"testing"
	"time"
)

// writeBigLine writes n bytes of x and a newline to the child's fd 3 from a
// goroutine, and returns a channel that gets the write's error (the child
// closing its end after the bound is the expected one).
func writeBigLine(r *hostRig, n int) <-chan error {
	done := make(chan error, 1)
	go func() {
		chunk := bytes.Repeat([]byte{'x'}, 1<<20)
		for n > 0 {
			k := min(n, len(chunk))
			if _, err := r.toFd3.Write(chunk[:k]); err != nil {
				done <- err
				return
			}
			n -= k
		}
		_, err := r.toFd3.Write([]byte("\n"))
		done <- err
	}()
	return done
}

func TestHostFD3LineBoundEndsTheSession(t *testing.T) {
	if os.Getenv(hostPtyChildEnv) != "" {
		t.Skip("child process")
	}
	file, data := writeHostFixture(t, hostDoc)
	r := startHostApp(t, hostOpts{}, file, data)
	i, _ := r.waitMsg(t, 0, ofType("ready"))

	// 64 MiB − 1 bytes: processed (not a JSON object), and the session goes on.
	if err := <-writeBigLine(r, maxFD3Line-1); err != nil {
		t.Fatalf("writing a line of 64 MiB - 1 bytes: %v", err)
	}
	i = r.waitRaw(t, i, `{"type":"error","error":"not a JSON object"}`)
	r.send(t, map[string]any{"type": "get", "id": "after", "path": "note"})
	i = r.waitRaw(t, i, `{"type":"ack","id":"after","ok":true,"found":true,"value":"n0"}`)
	if !r.alive() {
		t.Fatal("a line under the bound ended the session")
	}

	// 64 MiB: the child writes the error, ends the session with reason
	// error, and restores the terminal.
	_ = writeBigLine(r, maxFD3Line)
	r.waitRaw(t, i, `{"type":"error","error":"fd 3 line of 64 MiB or more"}`)
	if code := r.exit(t, 20*slowdown*time.Second); code != 1 {
		t.Errorf("exit code %d, want 1", code)
	}
	r.assertLast(t, `{"type":"exit","reason":"error","error":"fd 3 line of 64 MiB or more"}`)
	r.assertRestored(t)
}
