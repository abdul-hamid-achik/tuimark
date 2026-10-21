package host

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// SPEC v0.4d test 283 (§26.2, §26.14.1, decision 216): the keyboard modes
// of Run, in the scripted-terminal rig of caps_test.go. A <tui> without
// version asks for the Kitty keyboard protocol in the probe, pushes flag 1
// when the terminal answers (modifyOtherKeys level 2 when it does not),
// and pops it in the leave sequence, right after the mouse modes, on every
// path. A document that writes version gets none of it.

// kbDoc is a document of the current language whose mouse is on, so the
// leave sequence has mouse modes to turn off before the keyboard's.
const kbDoc = `<tui mouse="true">
<keymap><bind keys="z" action="quit"/><bind keys="p" action="boom"/></keymap>
<screen id="s"><text>KBAPP</text></screen>
</tui>`

// kbProbe is the probe of a document of the current language, with the
// Kitty query right before DA1.
const kbProbe = "\x1b[?2026$p\x1b[?2027$p\x1b[?u\x1b[c"

// kbReplies answers the probe: synchronized output and grapheme mode
// reported off, DA1; kitty adds the reply to the Kitty query.
func kbReplies(kitty bool) string {
	r := "\x1b[?2026;2$y\x1b[?2027;2$y"
	if kitty {
		r += "\x1b[?1u"
	}
	return r + "\x1b[?64;1;2;22c"
}

func kbSession(extra func(*termSession)) *termSession {
	s := &termSession{probe: true, kbQuery: true, wait: 10 * time.Second}
	if extra != nil {
		extra(s)
	}
	return s
}

func TestKeyboardModes283(t *testing.T) {
	t.Run("a terminal that answers", func(t *testing.T) {
		logPath := filepath.Join(t.TempDir(), "run.log")
		lg, err := openRunLog(logPath, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		r := startSession(t, kbDoc, kbSession(func(s *termSession) { s.log = lg }))
		r.waitOut(t, kbProbe)
		time.Sleep(20 * time.Millisecond)
		if got, want := r.out.String(), enterScreen+kbProbe; got != want {
			t.Fatalf("before the replies the session wrote %q, want %q", got, want)
		}
		r.send(t, kbReplies(true))
		r.waitOut(t, "KBAPP")
		out := r.out.String()
		afterProbe := strings.TrimPrefix(out, enterScreen+kbProbe)
		// CSI > 1 u comes after the probe (and mode 2027, which the replies
		// turn on) and before the first frame.
		if !strings.HasPrefix(afterProbe, graphemeOn+kittyPush) {
			t.Errorf("after the probe: %q, want %q first", afterProbe[:min(40, len(afterProbe))], graphemeOn+kittyPush)
		}
		if strings.Contains(out, mokPush) {
			t.Error("modifyOtherKeys was set although the terminal answered the query")
		}
		r.send(t, "z")
		if err := r.wait(t); err != nil {
			t.Fatal(err)
		}
		lg.end("quit")
		out = r.out.String()
		if !strings.HasSuffix(out, graphemeOff+mouseModesOff+kittyPop+leaveScreen) {
			t.Errorf("leave = %q", out[max(0, len(out)-60):])
		}
		recs := readLog(t, logPath)
		if len(recs) < 2 || recs[1]["ev"] != "caps" || recs[1]["keyboard"] != "kitty" {
			t.Errorf("caps record %v", recs)
		}
		if r.app.keyboard != "" {
			t.Errorf("the session's keyboard outlived it: %q", r.app.keyboard)
		}
	})

	t.Run("a terminal that does not answer", func(t *testing.T) {
		logPath := filepath.Join(t.TempDir(), "run.log")
		lg, err := openRunLog(logPath, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		r := startSession(t, kbDoc, kbSession(func(s *termSession) { s.log = lg }))
		r.waitOut(t, kbProbe)
		r.send(t, kbReplies(false))
		r.waitOut(t, "KBAPP")
		out := r.out.String()
		afterProbe := strings.TrimPrefix(out, enterScreen+kbProbe)
		if !strings.HasPrefix(afterProbe, graphemeOn+mokPush) {
			t.Errorf("after the probe: %q, want %q first", afterProbe[:min(40, len(afterProbe))], graphemeOn+mokPush)
		}
		if strings.Contains(out, kittyPush) {
			t.Error("Kitty was pushed although the terminal did not answer the query")
		}
		r.send(t, "z")
		if err := r.wait(t); err != nil {
			t.Fatal(err)
		}
		lg.end("quit")
		out = r.out.String()
		if !strings.HasSuffix(out, graphemeOff+mouseModesOff+mokPop+leaveScreen) {
			t.Errorf("leave = %q", out[max(0, len(out)-60):])
		}
		recs := readLog(t, logPath)
		if len(recs) < 2 || recs[1]["keyboard"] != "modify-other-keys" {
			t.Errorf("caps record %v", recs)
		}
	})

	t.Run("a stop signal pops the keyboard", func(t *testing.T) {
		r := startSession(t, kbDoc, kbSession(nil))
		r.waitOut(t, kbProbe)
		r.send(t, kbReplies(true))
		r.waitOut(t, "KBAPP")
		r.sigs <- os.Interrupt
		if err := r.wait(t); err == nil {
			t.Fatal("a signal ended the session cleanly")
		}
		if out := r.out.String(); !strings.HasSuffix(out, graphemeOff+mouseModesOff+kittyPop+leaveScreen) {
			t.Errorf("leave = %q", out[max(0, len(out)-60):])
		}
	})

	t.Run("the signal restore pops it too", func(t *testing.T) {
		// The restore path of a loop busy in a handler writes the same
		// sequence from the signal watcher (termSession.leave).
		s := kbSession(nil)
		s.kbPop = kittyPop
		if got, want := s.leave(), kittyPop+leaveScreen; got != want {
			t.Errorf("leave = %q, want %q", got, want)
		}
		s.mouse = true
		if got, want := s.leave(), mouseModesOff+kittyPop+leaveScreen; got != want {
			t.Errorf("leave = %q, want %q", got, want)
		}
	})

	t.Run("a panic in a handler pops it", func(t *testing.T) {
		a := doc(t, kbDoc)
		a.On("boom", func(Event) error { panic("boom") })
		out := &syncBuf{}
		pr, pw := io.Pipe()
		defer pw.Close()
		var panicked any
		done := make(chan struct{})
		go func() {
			defer close(done)
			defer func() { panicked = recover() }()
			_ = a.session(pr, out, func() (int, int) { return 20, 6 }, nil, nil, kbSession(nil), HostHooks{})
		}()
		deadline := time.Now().Add(5 * time.Second)
		for !strings.Contains(out.String(), kbProbe) && time.Now().Before(deadline) {
			time.Sleep(2 * time.Millisecond)
		}
		_, _ = io.WriteString(pw, kbReplies(true))
		for !strings.Contains(out.String(), "KBAPP") && time.Now().Before(deadline) {
			time.Sleep(2 * time.Millisecond)
		}
		_, _ = io.WriteString(pw, "p")
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("the session did not return")
		}
		if panicked == nil {
			t.Fatal("the handler's panic did not unwind through the session")
		}
		if s := out.String(); !strings.HasSuffix(s, graphemeOff+mouseModesOff+kittyPop+leaveScreen) {
			t.Errorf("leave after the panic = %q", s[max(0, len(s)-60):])
		}
	})

	t.Run("no probe", func(t *testing.T) {
		// A session with no probe (the output is not a terminal, or TERM is
		// dumb) writes neither mode and is legacy; its frames show the keys
		// a legacy terminal can send.
		logPath := filepath.Join(t.TempDir(), "run.log")
		lg, err := openRunLog(logPath, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		r := startSession(t, kbDoc, &termSession{log: lg})
		r.waitOut(t, "KBAPP")
		r.send(t, "z")
		if err := r.wait(t); err != nil {
			t.Fatal(err)
		}
		lg.end("quit")
		out := r.out.String()
		for _, s := range []string{queryKitty, kittyPush, mokPush, kittyPop, mokPop} {
			if strings.Contains(out, s) {
				t.Errorf("a session with no probe wrote %q", s)
			}
		}
		recs := readLog(t, logPath)
		if len(recs) < 2 || recs[1]["keyboard"] != "legacy" {
			t.Errorf("caps record %v", recs)
		}
	})

	t.Run("a document that writes version", func(t *testing.T) {
		// version="3": no Kitty query, neither mode, no keyboard member.
		logPath := filepath.Join(t.TempDir(), "run.log")
		lg, err := openRunLog(logPath, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		src := strings.Replace(kbDoc, "<tui mouse", `<tui version="3" mouse`, 1)
		r := startSession(t, src, &termSession{probe: true, wait: 10 * time.Second, log: lg})
		r.waitOut(t, "\x1b[?2026$p\x1b[?2027$p\x1b[c")
		r.send(t, kbReplies(true))
		r.waitOut(t, "KBAPP")
		r.send(t, "z")
		if err := r.wait(t); err != nil {
			t.Fatal(err)
		}
		lg.end("quit")
		out := r.out.String()
		for _, s := range []string{queryKitty, kittyPush, mokPush, kittyPop, mokPop} {
			if strings.Contains(out, s) {
				t.Errorf("a pinned document wrote %q", s)
			}
		}
		recs := readLog(t, logPath)
		if len(recs) < 2 {
			t.Fatalf("records %v", recs)
		}
		if _, has := recs[1]["keyboard"]; has {
			t.Errorf("a pinned document's caps record has keyboard: %v", recs[1])
		}
	})
}

// SPEC v0.4d §26.2: the Kitty query is outside the skip rule and counts as
// a query other than DA1, so the probe of a document in the current
// language runs whenever the output is a terminal and TERM is not dumb.
func TestKeyboardProbePlan(t *testing.T) {
	apple := map[string]string{"TERM": "xterm-256color", "TERM_PROGRAM": "Apple_Terminal"}
	ssh := map[string]string{"TERM": "xterm-256color", "SSH_TTY": "/dev/pts/1"}
	dumb := map[string]string{"TERM": "dumb"}
	for _, c := range []struct {
		name       string
		env        map[string]string
		tty, kb    bool
		probe, ski bool
	}{
		{"current, terminal", map[string]string{"TERM": "xterm"}, true, true, true, false},
		{"current, Apple Terminal", apple, true, true, true, true},
		{"current, SSH", ssh, true, true, true, true},
		{"current, dumb", dumb, true, true, false, false},
		{"current, not a terminal", map[string]string{"TERM": "xterm"}, false, true, false, false},
		{"pinned, Apple Terminal", apple, true, false, false, false},
		{"pinned, terminal", map[string]string{"TERM": "xterm"}, true, false, true, false},
	} {
		probe, skip := probePlanKB(envMap(c.env), c.tty, syncAuto, false, c.kb)
		if probe != c.probe || (probe && skip != c.ski) {
			t.Errorf("%s: probe %v skip %v, want %v %v", c.name, probe, skip, c.probe, c.ski)
		}
	}
	s := &termSession{probe: true, skipDECRQM: true, kbQuery: true}
	if got, want := s.queries(), "\x1b[?u\x1b[c"; got != want {
		t.Errorf("a skipped terminal's queries = %q, want %q", got, want)
	}
	s = &termSession{probe: true, themeQuery: true, sync: syncOn, kbQuery: true}
	if got, want := s.queries(), queryBackground+"\x1b[?2027$p\x1b[?u\x1b[c"; got != want {
		t.Errorf("queries with OSC 11 and TUIMARK_SYNC = %q, want %q", got, want)
	}
}
