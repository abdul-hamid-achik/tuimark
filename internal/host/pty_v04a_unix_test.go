//go:build unix

package host

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// ptyV04aDoc is withDoc with a marker the parent waits for.
var ptyV04aDoc = strings.Replace(withDoc, `<text>[{o}]</text>`, `<text>CTXAPP [{o}]</text>`, 1)

// ptyV04aProbe answers the capability probe: synchronized output and
// grapheme mode supported, mode 2027 off (Run turns it on).
const ptyV04aProbe = "\x1b[?2026;2$y\x1b[?2027;2$y\x1b[?64;1;22c"

// reportCooked writes whether the terminal is back in cooked mode: the
// parent cannot look at it once this session leader exits (darwin
// revokes it).
func reportCooked() {
	tio, err := unix.IoctlGetTermios(0, ioctlGetTermios)
	fmt.Fprintf(os.Stderr, "COOKED %v\n", err == nil && tio.Lflag&unix.ICANON != 0 && tio.Lflag&unix.ECHO != 0)
}

// runCtxPtyChild runs RunContext on the pty: in mode "ctx-pre" with a
// context already cancelled; in mode "ctx-run" with a context the fx
// handler cancels (fx and opened fire in one round, after g opens the
// modal).
func runCtxPtyChild(t *testing.T, mode string) {
	a := doc(t, ptyV04aDoc)
	_ = a.Bind("", map[string]any{"o": false})
	if mode == "ctx-pre" {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := a.RunContext(ctx, os.Stdout)
		fmt.Fprintf(os.Stderr, "PRE %v %v\n", errors.Is(err, context.Canceled), err)
		reportCooked()
		os.Exit(0)
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.On("go", func(Event) error { return a.Set("o", true) })
	a.On("fx", func(Event) error {
		fmt.Fprintln(os.Stderr, "EVENT fx")
		cancel()
		return nil
	})
	a.On("opened", func(Event) error {
		fmt.Fprintln(os.Stderr, "EVENT opened")
		return nil
	})
	err := a.RunContext(ctx, os.Stdout)
	fmt.Fprintf(os.Stderr, "RUN-RETURNED %v CANCELED %v\n", err, errors.Is(err, context.Canceled))
	reportCooked()
	os.Exit(0)
}

// everyPtyDoc shows n three times, so every tick changes three adjacent
// cells and the frame diff writes them as one run ("222").
const everyPtyDoc = `<tui version="1">
<keymap><bind keys="q" action="quit"/></keymap>
<screen id="s"><text>EVERYAPP {n}{n}{n}</text></screen>
</tui>`

// runEveryPtyChild registers Every(20ms) incrementing n, then runs Run
// twice: the registration survives into the second Run.
func runEveryPtyChild(t *testing.T) {
	a := doc(t, everyPtyDoc)
	_ = a.Bind("", map[string]any{"n": 0})
	a.Every(20*time.Millisecond, func(b *Batch) error {
		v, _ := a.Get("n")
		return b.Set("n", v.(float64)+1)
	})
	for i := 1; i <= 2; i++ {
		err := a.Run(os.Stdout)
		v, _ := a.Get("n")
		fmt.Fprintf(os.Stderr, "RUN%d-RETURNED %v N %v\n", i, err, v)
	}
	os.Exit(0)
}

// 113 under a pty (SPEC v0.4a §18.2): with Every(20ms) incrementing n,
// the frames Run writes show n increasing, and the registration survives
// into a second Run.
func TestEveryPty(t *testing.T) {
	if os.Getenv(ptyChildEnv) == "every" {
		runEveryPtyChild(t)
		return
	}
	r := startPtyChildEnv(t, "TestEveryPty", "every", []string{"TERM=xterm-256color", "TUIMARK_COLOR=truecolor"})
	// after waits until the screen output past pos holds want, and
	// returns the position after it.
	after := func(pos int, want, what string) int {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for {
			if i := strings.Index(r.screen.String()[pos:], want); i >= 0 {
				return pos + i + len(want)
			}
			if time.Now().After(deadline) {
				t.Fatalf("never saw %s; %s", what, r.describe())
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	run := func(pos, from int) int {
		t.Helper()
		pos = after(pos, "\x1b[?2027$p\x1b[c", "the probe")
		r.write(t, ptyV04aProbe)
		// The frames show n increasing, in order.
		for n := from + 1; n <= from+3; n++ {
			pos = after(pos, strings.Repeat(fmt.Sprint(n), 3), fmt.Sprintf("n=%d in a frame", n))
		}
		return pos
	}
	pos := run(0, 0)
	r.write(t, "q")
	if !r.waitFor(r.stderr, "RUN1-RETURNED <nil>", 5*time.Second) {
		t.Fatalf("the first Run did not quit; %s", r.describe())
	}
	var n int
	e := r.stderr.String()
	if _, err := fmt.Sscanf(e[strings.Index(e, "RUN1-RETURNED"):], "RUN1-RETURNED <nil> N %d", &n); err != nil {
		t.Fatalf("parse %q: %v", e, err)
	}
	run(pos, n)
	r.write(t, "q")
	if !r.wait(10 * time.Second) {
		t.Fatalf("the second Run did not quit; %s", r.describe())
	}
	if !strings.Contains(r.stderr.String(), "RUN2-RETURNED <nil>") {
		t.Errorf("second Run; %s", r.describe())
	}
}

// 110 under a pty (SPEC v0.4a §18.2). Cancelling the context during a
// handler ends the session when the handler returns: the round's later
// event (opened) does not run, the terminal is restored (the leave
// sequence, cooked mode), RunContext's error wraps context.Canceled, and
// TUIMARK_LOG's end record is "error". A context cancelled before the
// call returns at once, wrapping context.Canceled, and writes nothing to
// the terminal.
func TestRunContextPty(t *testing.T) {
	if mode := os.Getenv(ptyChildEnv); strings.HasPrefix(mode, "ctx-") {
		runCtxPtyChild(t, mode)
		return
	}
	base := []string{"TERM=xterm-256color", "TUIMARK_COLOR=truecolor"}
	t.Run("cancelled during a handler", func(t *testing.T) {
		logPath := filepath.Join(t.TempDir(), "run.log")
		r := startPtyChildEnv(t, "TestRunContextPty", "ctx-run", append(base, "TUIMARK_LOG="+logPath))
		if !r.waitFor(r.screen, "\x1b[?2027$p\x1b[c", 10*time.Second) {
			t.Fatalf("no probe; %s", r.describe())
		}
		r.write(t, ptyV04aProbe)
		if !r.waitFor(r.screen, "CTXAPP", 10*time.Second) {
			t.Fatalf("the app never rendered; %s", r.describe())
		}
		r.write(t, "g")
		if !r.wait(10 * time.Second) {
			t.Fatalf("cancelling did not end Run; %s", r.describe())
		}
		e := r.stderr.String()
		if !strings.Contains(e, "EVENT fx") || strings.Contains(e, "EVENT opened") {
			t.Errorf("events: want fx and not opened; %s", r.describe())
		}
		if !strings.Contains(e, "RUN-RETURNED tuimark: Run cancelled: context canceled CANCELED true") {
			t.Errorf("RunContext's error; %s", r.describe())
		}
		if !strings.Contains(e, "COOKED true") {
			t.Errorf("the terminal is not cooked; %s", r.describe())
		}
		r.slave.Close()
		time.Sleep(50 * time.Millisecond)
		if s := r.screen.String(); !strings.HasSuffix(s, leaveWith2027) {
			t.Errorf("leave = %q", s[max(0, len(s)-50):])
		}
		recs := readLog(t, logPath)
		if last := recs[len(recs)-1]; last["ev"] != "end" || last["reason"] != "error" {
			t.Errorf("last record %v, want end with reason error", last)
		}
	})
	t.Run("cancelled before the call", func(t *testing.T) {
		r := startPtyChildEnv(t, "TestRunContextPty", "ctx-pre", base)
		if !r.wait(10 * time.Second) {
			t.Fatalf("RunContext did not return; %s", r.describe())
		}
		e := r.stderr.String()
		if !strings.Contains(e, "PRE true tuimark: Run cancelled: context canceled") || !strings.Contains(e, "COOKED true") {
			t.Errorf("child; %s", r.describe())
		}
		r.slave.Close()
		time.Sleep(50 * time.Millisecond)
		if s := r.screen.String(); s != "" {
			t.Errorf("wrote %q to the terminal", s)
		}
	})
}
