//go:build unix

package host

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// The smoke test of test 194: Run on a pty with the real clock. It checks
// ceilings and order only, and asserts no rate: a wall-clock floor fails
// under -race on a loaded runner. The exact counts are the fake clock's
// (runclock_run_test.go).

const spinPtyDoc = `<tui>
<keymap><bind keys="q" action="quit"/></keymap>
<screen id="main">%s<text>READY</text></screen>
</tui>`

// spinPtyExtra is test 195's first bullet: an input with a handler, and a
// counter an Every function increments every 300 ms.
const spinPtyExtra = `<input id="q" on:change="ping"/><text>n={n}</text>`

func runSpinPtyChild(t *testing.T, s1 string, extra bool) {
	src := fmt.Sprintf(spinPtyDoc, s1)
	if extra {
		src = strings.Replace(src, "<text>READY</text>", "<text>READY</text>"+spinPtyExtra, 1)
	}
	a := doc(t, src)
	if extra {
		_ = a.Bind("", map[string]any{"n": 0})
		a.On("ping", func(Event) error { fmt.Fprintln(os.Stderr, "PING"); return nil })
		a.Every(300*time.Millisecond, incr(a))
	}
	err := a.Run(os.Stdout)
	fmt.Fprintf(os.Stderr, "RUN-RETURNED %v\n", err)
	os.Exit(0)
}

func startSpinPty(t *testing.T, mode string) (*ptyRig, int) {
	t.Helper()
	r := startPtyChildEnv(t, "TestRunClockPtySmoke", mode, []string{"TERM=xterm-256color", "TUIMARK_COLOR=truecolor"})
	if !r.waitFor(r.screen, "\x1b[?2027$p\x1b[c", 10*time.Second) {
		t.Fatalf("never saw the probe; %s", r.describe())
	}
	r.write(t, ptyV04aProbe)
	if !r.waitFor(r.screen, "READY", 10*time.Second) {
		t.Fatalf("never saw the first frame; %s", r.describe())
	}
	return r, len(r.screen.String())
}

func TestRunClockPtySmoke(t *testing.T) {
	switch os.Getenv(ptyChildEnv) {
	case "spin-visible":
		runSpinPtyChild(t, `<progress id="s1" indeterminate="true"/>`, true)
		return
	case "spin-hidden":
		runSpinPtyChild(t, `<progress id="s1" indeterminate="true" style="visibility: hidden"/>`, false)
		return
	}
	t.Run("visible spinner turns", func(t *testing.T) {
		r, pos := startSpinPty(t, "spin-visible")
		// At least two redraws, and the Every counter at 1 or more, within
		// the ceiling; no rate is asserted.
		ceiling := time.Now().Add(2 * slowdown * time.Second)
		var glyphs []string
		counted := false
		for time.Now().Before(ceiling) && (len(glyphs) < 2 || !counted) {
			glyphs, counted = glyphs[:0], false
			for _, w := range writes(r.screen.String()[pos:]) {
				switch {
				case strings.ContainsAny(w.text, strings.Join(braille, "")):
					glyphs = append(glyphs, w.text)
				case w.text >= "1" && w.text <= "9":
					counted = true // a digit after the first frame: n moved
				}
			}
			time.Sleep(10 * time.Millisecond)
		}
		if len(glyphs) < 2 || !counted {
			t.Fatalf("%d redraws, n moved = %v within the ceiling, want at least 2 and true; %s", len(glyphs), counted, r.describe())
		}
		if strings.Contains(r.stderr.String(), "PING") {
			t.Errorf("ping was called by a redraw; %s", r.describe())
		}
		// Order: the first redraw is not phase 0 again, and no redraw
		// repeats the glyph before it.
		if glyphs[0] == "⠋" {
			t.Errorf("the first redraw is phase 0 again: %q", glyphs)
		}
		for i := 1; i < len(glyphs); i++ {
			if glyphs[i] == glyphs[i-1] {
				t.Errorf("redraw %d repeats %q", i, glyphs[i])
			}
		}
		r.write(t, "\x03") // ctrl+c: the built-in quit, which an input does not take
		if !r.wait(10 * time.Second) {
			t.Fatalf("Run did not quit; %s", r.describe())
		}
		if !strings.Contains(r.stderr.String(), "RUN-RETURNED <nil>") {
			t.Errorf("Run did not return nil; %s", r.describe())
		}
	})
	t.Run("hidden spinner does not", func(t *testing.T) {
		r, pos := startSpinPty(t, "spin-hidden")
		time.Sleep(time.Second) // a negative: nothing may be written in a second
		if got := r.screen.String()[pos:]; strings.TrimSpace(strings.NewReplacer("\x1b[0m", "", "\x1b[?25l", "", "\x1b[?2026h", "", "\x1b[?2026l", "").Replace(got)) != "" {
			t.Errorf("a hidden spinner wrote %q", got)
		}
		r.write(t, "q")
		if !r.wait(10 * time.Second) {
			t.Fatalf("Run did not quit; %s", r.describe())
		}
	})
}
