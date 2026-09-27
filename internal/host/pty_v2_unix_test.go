//go:build unix

package host

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// runV2PtyChild runs Run on the pty with a version="2" document whose
// theme is TUIMARK_PTY_THEME (auto by default). Events are reported on
// stderr.
func runV2PtyChild(t *testing.T) {
	src := autoDoc
	if th := os.Getenv("TUIMARK_PTY_THEME"); th != "" {
		src = strings.Replace(src, `theme="auto"`, `theme="`+th+`"`, 1)
	}
	a := doc(t, src)
	_ = a.Bind("", map[string]any{"text": "", "pw": ""})
	for _, act := range []string{"chg", "hello"} {
		a.On(act, func(ev Event) error {
			fmt.Fprintf(os.Stderr, "EVENT %s %q\n", ev.Action, fmt.Sprint(ev.Value))
			return nil
		})
	}
	err := a.Run(os.Stdout)
	fmt.Fprintf(os.Stderr, "RUN-RETURNED %v\n", err)
	os.Exit(0)
}

// SPEC v0.2b §21 tests 41 and 67 through a real pty: theme="auto" puts
// OSC 11 first in the probe and the reply decides the first frame's
// theme; TUIMARK_THEME or a dark document write no OSC 11; under the
// Apple Terminal/SSH skip rule an auto document sends OSC 11 and DA1 and
// nothing else; TUIMARK_LOG writes the session's NDJSON log with mode
// 0600 and redacts a secret input's keys, pastes, and values.
func TestRunPtyV2(t *testing.T) {
	if os.Getenv(ptyChildEnv) == "run-v2" {
		runV2PtyChild(t)
		return
	}
	start := func(t *testing.T, env []string) *ptyRig {
		t.Helper()
		return startPtyChildEnv(t, "TestRunPtyV2", "run-v2", env)
	}
	finish := func(t *testing.T, r *ptyRig) string {
		t.Helper()
		if !r.waitFor(r.screen, "AUTO", 10*time.Second) || !r.waitFor(r.stderr, "EVENT hello", 5*time.Second) {
			t.Fatalf("the app never rendered; %s", r.describe())
		}
		r.write(t, "\x03")
		if !r.wait(10 * time.Second) {
			t.Fatalf("ctrl+c did not quit; %s", r.describe())
		}
		r.slave.Close()
		time.Sleep(50 * time.Millisecond)
		return r.screen.String()
	}
	base := []string{"TERM=xterm-256color", "TUIMARK_COLOR=truecolor"}
	t.Run("osc11 light", func(t *testing.T) {
		r := start(t, base)
		if !r.waitFor(r.screen, "\x1b[c", 10*time.Second) {
			t.Fatalf("no probe; %s", r.describe())
		}
		r.write(t, "\x1b]11;rgb:ffff/ffff/ffff\x1b\\\x1b[?2026;2$y\x1b[?2027;2$y\x1b[?64;1;22c")
		s := finish(t, r)
		probe := enterScreen + queryBackground + "\x1b[?2026$p\x1b[?2027$p\x1b[c"
		if !strings.HasPrefix(s, probe) {
			t.Errorf("start = %q, want %q", s[:min(len(s), len(probe)+8)], probe)
		}
		if !strings.Contains(s, lightBG) || strings.Contains(s, darkBG) {
			t.Error("the first frame is not light")
		}
	})
	for _, c := range []struct {
		name string
		env  []string
		want string
	}{
		{"TUIMARK_THEME=light", []string{"TUIMARK_THEME=light"}, lightBG},
		{"document dark", []string{"TUIMARK_PTY_THEME=dark", "COLORFGBG=0;15"}, darkBG},
	} {
		t.Run(c.name+" writes no OSC 11", func(t *testing.T) {
			r := start(t, append(append([]string(nil), base...), c.env...))
			if !r.waitFor(r.screen, "\x1b[c", 10*time.Second) {
				t.Fatalf("no probe; %s", r.describe())
			}
			r.write(t, "\x1b[?64c")
			s := finish(t, r)
			if strings.Contains(s, "\x1b]11") {
				t.Errorf("OSC 11 written: %q", s[:min(len(s), 80)])
			}
			if !strings.Contains(s, c.want) {
				t.Errorf("no %q in the frames", c.want)
			}
		})
	}
	t.Run("skip rule sends OSC 11 and DA1 only", func(t *testing.T) {
		r := start(t, append(append([]string(nil), base...), "TERM_PROGRAM=Apple_Terminal", "COLORFGBG=15;0"))
		if !r.waitFor(r.screen, "\x1b[c", 10*time.Second) {
			t.Fatalf("no probe; %s", r.describe())
		}
		s := finish(t, r) // no reply: the cap, then COLORFGBG (dark)
		if !strings.HasPrefix(s, enterScreen+queryBackground+"\x1b[c") || strings.Contains(s, "$p") {
			t.Errorf("start = %q", s[:min(len(s), 60)])
		}
		if !strings.Contains(s, darkBG) {
			t.Error("COLORFGBG 15;0 did not give dark")
		}
	})
	t.Run("TUIMARK_LOG", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "session.log")
		if err := os.WriteFile(p, []byte("stale\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(p, 0o644); err != nil {
			t.Fatal(err)
		}
		r := start(t, append(append([]string(nil), base...), "TUIMARK_LOG="+p, "COLORFGBG=0;15"))
		if !r.waitFor(r.screen, "\x1b[c", 10*time.Second) {
			t.Fatalf("no probe; %s", r.describe())
		}
		r.write(t, "\x1b[?64c") // no OSC 11 reply: COLORFGBG says light
		if !r.waitFor(r.stderr, "EVENT hello", 5*time.Second) {
			t.Fatalf("no first frame; %s", r.describe())
		}
		r.write(t, "hi")
		if !r.waitFor(r.stderr, `EVENT chg "hi"`, 5*time.Second) {
			t.Fatalf("no change; %s", r.describe())
		}
		r.write(t, "\t")
		time.Sleep(100 * time.Millisecond)
		r.write(t, "zz")
		if !r.waitFor(r.stderr, `EVENT chg "zz"`, 5*time.Second) {
			t.Fatalf("no secret change; %s", r.describe())
		}
		r.write(t, "\x1b[200~qq\x1b[201~")
		if !r.waitFor(r.stderr, `EVENT chg "zzqq"`, 5*time.Second) {
			t.Fatalf("no paste; %s", r.describe())
		}
		s := finish(t, r)
		if !strings.Contains(s, lightBG) {
			t.Error("COLORFGBG 0;15 did not give light")
		}
		st, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if st.Mode().Perm() != 0o600 {
			t.Errorf("log mode %v, want 0600", st.Mode().Perm())
		}
		recs := readLog(t, p)
		if len(recs) < 3 || recs[0]["ev"] != "start" || recs[1]["ev"] != "caps" || recs[len(recs)-1]["ev"] != "end" || recs[len(recs)-1]["reason"] != "quit" {
			t.Fatalf("records %v", recs)
		}
		caps := recs[1]
		if caps["probe"] != true || caps["theme"] != "light" || caps["theme_from"] != "COLORFGBG" || caps["color"] != "truecolor" {
			t.Errorf("caps %v", caps)
		}
		var keys []string
		for _, rec := range recs {
			if rec["ev"] == "key" && rec["redacted"] != true {
				keys = append(keys, rec["key"].(string))
			}
		}
		if strings.Join(keys, " ") != "h i tab" {
			t.Errorf("keys %v", keys)
		}
		raw, _ := os.ReadFile(p)
		if strings.Contains(string(raw), "zz") || strings.Contains(string(raw), "qq") || strings.Contains(string(raw), "stale") {
			t.Errorf("the log holds secret or stale text:\n%s", raw)
		}
	})
}

// runMousePtyChild runs Run on the pty with modeDoc and the mouse on. A
// pick turns the mouse off when TUIMARK_PTY_MOUSE_OFF is set.
func runMousePtyChild(t *testing.T) {
	a := doc(t, modeDoc)
	_ = a.Bind("", map[string]any{"m": true, "rows": []any{"a", "b"}})
	a.On("pick", func(ev Event) error {
		fmt.Fprintf(os.Stderr, "EVENT pick %v\n", ev.Keys["r"])
		if os.Getenv("TUIMARK_PTY_MOUSE_OFF") != "" {
			return a.Set("m", false)
		}
		return nil
	})
	err := a.Run(os.Stdout)
	fmt.Fprintf(os.Stderr, "RUN-RETURNED %v\n", err)
	os.Exit(0)
}

// SPEC v0.2b §21 test 44 through Run on a pty: mouse="m" true writes
// CSI ?1000h CSI ?1006h before the first frame's bytes; an SGR click acts;
// Set("m", false) makes the next frame write CSI ?1000l CSI ?1006l; the
// leave sequence turns the modes off only while they are on; ?1003h is
// never written.
func TestRunPtyMouse(t *testing.T) {
	if os.Getenv(ptyChildEnv) == "run-mouse" {
		runMousePtyChild(t)
		return
	}
	for _, off := range []bool{false, true} {
		t.Run(fmt.Sprintf("off=%v", off), func(t *testing.T) {
			env := []string{"TERM=xterm-256color", "TUIMARK_COLOR=truecolor"}
			if off {
				env = append(env, "TUIMARK_PTY_MOUSE_OFF=1")
			}
			r := startPtyChildEnv(t, "TestRunPtyMouse", "run-mouse", env)
			if !r.waitFor(r.screen, "\x1b[c", 10*time.Second) {
				t.Fatalf("no probe; %s", r.describe())
			}
			r.write(t, "\x1b[?64c")
			if !r.waitFor(r.screen, mouseModesOn, 5*time.Second) {
				t.Fatalf("no mouse modes; %s", r.describe())
			}
			time.Sleep(100 * time.Millisecond)
			r.write(t, "\x1b[<0;1;2M\x1b[<0;1;2m")
			if !r.waitFor(r.stderr, "EVENT pick b", 5*time.Second) {
				t.Fatalf("the click did not act; %s", r.describe())
			}
			if off && !r.waitFor(r.screen, mouseModesOff, 5*time.Second) {
				t.Fatalf("the modes did not turn off; %s", r.describe())
			}
			r.write(t, "\x03")
			if !r.wait(10 * time.Second) {
				t.Fatalf("ctrl+c did not quit; %s", r.describe())
			}
			r.slave.Close()
			time.Sleep(50 * time.Millisecond)
			s := r.screen.String()
			probeEnd := strings.Index(s, "\x1b[c") + len("\x1b[c")
			on, first := strings.Index(s, mouseModesOn), strings.Index(s[probeEnd:], "a")
			if on < probeEnd || first < 0 || on > probeEnd+first {
				t.Errorf("modes on at %d, probe end %d, first frame text at %d", on, probeEnd, probeEnd+first)
			}
			if strings.Contains(s, "\x1b[?1003h") || strings.Count(s, mouseModesOff) != 1 {
				t.Errorf("mode writes: %q", s)
			}
			leave := s[strings.LastIndex(s, "\x1b[?2004l"):]
			before := s[:strings.LastIndex(s, "\x1b[?2004l")]
			if off == strings.HasSuffix(before, mouseModesOff) || strings.Contains(leave, mouseModesOff) {
				t.Errorf("leave (off=%v): %q", off, s[max(0, len(s)-80):])
			}
		})
	}
}
