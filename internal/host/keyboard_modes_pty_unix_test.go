//go:build unix

package host

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// SPEC v0.4d test 283, through Run: on a pty, with the environment Run
// reads. TERM=dumb writes neither mode; a document that writes version
// under the skip rule writes no probe at all; a document in the current
// language under the skip rule asks only for the Kitty keyboard.
func TestKeyboardModesRunPty(t *testing.T) {
	if mode := os.Getenv(ptyChildEnv); strings.HasPrefix(mode, "kb-") {
		src := kbDoc
		if mode == "kb-v3" {
			src = strings.Replace(kbDoc, "<tui mouse", `<tui version="3" mouse`, 1)
		}
		a := doc(t, src)
		err := a.Run(os.Stdout)
		os.Stderr.WriteString("RUN-RETURNED ")
		if err != nil {
			os.Stderr.WriteString(err.Error())
		} else {
			os.Stderr.WriteString("<nil>")
		}
		os.Stderr.WriteString("\n")
		os.Exit(0)
	}
	start := func(t *testing.T, mode string, env ...string) (*ptyRig, string) {
		t.Helper()
		logPath := filepath.Join(t.TempDir(), "run.log")
		env = append([]string{"TUIMARK_COLOR=truecolor", "TUIMARK_LOG=" + logPath}, env...)
		return startPtyChildEnv(t, "TestKeyboardModesRunPty", mode, env), logPath
	}
	finish := func(t *testing.T, r *ptyRig) {
		t.Helper()
		r.write(t, "z")
		if !r.wait(10 * time.Second) {
			t.Fatalf("Run did not quit; %s", r.describe())
		}
	}
	t.Run("TERM=dumb", func(t *testing.T) {
		r, logPath := start(t, "kb-dumb", "TERM=dumb")
		if !r.waitFor(r.screen, "KBAPP", 10*time.Second) {
			t.Fatalf("no first frame; %s", r.describe())
		}
		finish(t, r)
		for _, s := range []string{queryKitty, kittyPush, mokPush, kittyPop, mokPop, "\x1b[c"} {
			if strings.Contains(r.screen.String(), s) {
				t.Errorf("TERM=dumb wrote %q", s)
			}
		}
		recs := readLog(t, logPath)
		if len(recs) < 2 || recs[1]["keyboard"] != "legacy" || recs[1]["probe"] != false {
			t.Errorf("caps record %v", recs)
		}
	})
	t.Run("version written under the skip rule", func(t *testing.T) {
		r, logPath := start(t, "kb-v3", "TERM=xterm-256color", "TERM_PROGRAM=Apple_Terminal")
		if !r.waitFor(r.screen, "KBAPP", 10*time.Second) {
			t.Fatalf("no first frame; %s", r.describe())
		}
		finish(t, r)
		if strings.Contains(r.screen.String(), "\x1b[c") || strings.Contains(r.screen.String(), queryKitty) {
			t.Errorf("a pinned document under the skip rule wrote a probe: %q", r.screen.String())
		}
		recs := readLog(t, logPath)
		if len(recs) < 2 || recs[1]["probe"] != false {
			t.Errorf("caps record %v", recs)
		}
		if _, has := recs[1]["keyboard"]; has {
			t.Errorf("a pinned document's caps record has keyboard: %v", recs[1])
		}
	})
	t.Run("the current language under the skip rule", func(t *testing.T) {
		r, logPath := start(t, "kb-apple", "TERM=xterm-256color", "TERM_PROGRAM=Apple_Terminal")
		if !r.waitFor(r.screen, enterScreen+"\x1b[?u\x1b[c", 10*time.Second) {
			t.Fatalf("no probe with only the Kitty query; %s", r.describe())
		}
		if strings.Contains(r.screen.String(), "$p") {
			t.Errorf("the skip rule left the DECRQM queries in: %q", r.screen.String())
		}
		r.write(t, "\x1b[?1u\x1b[?64;1;2;22c")
		if !r.waitFor(r.screen, "KBAPP", 10*time.Second) {
			t.Fatalf("no first frame; %s", r.describe())
		}
		if !strings.Contains(r.screen.String(), kittyPush) {
			t.Errorf("no CSI > 1 u after the reply: %q", r.screen.String())
		}
		finish(t, r)
		if s := r.screen.String(); !strings.Contains(s, mouseModesOff+kittyPop+leaveScreen) {
			t.Errorf("leave = %q", s[max(0, len(s)-60):])
		}
		recs := readLog(t, logPath)
		if len(recs) < 2 || recs[1]["keyboard"] != "kitty" || recs[1]["probe"] != true {
			t.Errorf("caps record %v", recs)
		}
	})
}
