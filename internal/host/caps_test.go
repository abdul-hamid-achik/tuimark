package host

import (
	"bytes"
	"errors"
	"io"
	"os"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/paint"
)

func envMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// SPEC v0.2 §26.10 and §21 test 35: TUIMARK_* values are exact and
// case-sensitive, empty means unset, and an invalid value is an error.
func TestReadRunConfig(t *testing.T) {
	cfg, err := readRunConfig(envMap(map[string]string{"TERM": "xterm-256color"}))
	if err != nil || cfg.profile != paint.ANSI256 || cfg.theme != "" || cfg.sync != syncAuto {
		t.Errorf("defaults = %+v, %v", cfg, err)
	}
	cfg, err = readRunConfig(envMap(map[string]string{"TERM": "xterm", "TUIMARK_COLOR": "none", "TUIMARK_THEME": "light", "TUIMARK_SYNC": "1"}))
	if err != nil || cfg.profile != paint.NoColor || cfg.theme != "light" || cfg.sync != syncOn {
		t.Errorf("set = %+v, %v", cfg, err)
	}
	cfg, err = readRunConfig(envMap(map[string]string{"TUIMARK_COLOR": "", "TUIMARK_THEME": "", "TUIMARK_SYNC": "0", "COLORTERM": "truecolor", "TERM": "xterm"}))
	if err != nil || cfg.profile != paint.TrueColor || cfg.theme != "" || cfg.sync != syncOff {
		t.Errorf("empty values = %+v, %v", cfg, err)
	}
	for _, bad := range []map[string]string{
		{"TUIMARK_COLOR": "TrueColor"},
		{"TUIMARK_COLOR": "8"},
		{"TUIMARK_THEME": "Light"},
		{"TUIMARK_THEME": "auto"},
		{"TUIMARK_SYNC": "2"},
		{"TUIMARK_SYNC": "true"},
	} {
		if _, err := readRunConfig(envMap(bad)); err == nil {
			t.Errorf("%v accepted", bad)
		} else {
			for k := range bad {
				if !strings.Contains(err.Error(), k) {
					t.Errorf("%v: error %q does not name %s", bad, err, k)
				}
			}
		}
	}
}

// §21 test 35: an invalid TUIMARK_* value makes Run return an error before
// it touches the terminal: nothing is written (and no raw mode, which the
// early return rules out).
func TestRunRejectsBadEnvironment(t *testing.T) {
	for _, kv := range [][2]string{{"TUIMARK_COLOR", "bogus"}, {"TUIMARK_THEME", "auto"}, {"TUIMARK_SYNC", "yes"}} {
		t.Run(kv[0], func(t *testing.T) {
			t.Setenv(kv[0], kv[1])
			var out bytes.Buffer
			err := doc(t, loopDoc).Run(&out)
			if err == nil || !strings.Contains(err.Error(), kv[0]) {
				t.Errorf("Run with %s=%s: %v", kv[0], kv[1], err)
			}
			if out.Len() != 0 {
				t.Errorf("Run wrote %q", out.String())
			}
		})
	}
}

// §26.3 and §21 test 33/35: DetectColorProfile is Run's detection, shared
// with preview: --color comes first (and then the environment is not
// read), else the §26.3 order; invalid values are errors naming the source.
func TestDetectColorProfile(t *testing.T) {
	read := false
	spy := func(m map[string]string) func(string) string {
		return func(k string) string { read = true; return m[k] }
	}
	for _, c := range []struct {
		flag string
		env  map[string]string
		want paint.Profile
	}{
		{"", map[string]string{"TERM": "xterm-256color"}, paint.ANSI256},
		{"", map[string]string{"TERM": "xterm", "TUIMARK_COLOR": "16"}, paint.ANSI16},
		{"", map[string]string{"TERM": "xterm-kitty", "NO_COLOR": "1"}, paint.NoColor},
		{"", map[string]string{"WT_SESSION": "x"}, paint.TrueColor},
		{"", map[string]string{}, paint.NoColor},
		{"256", map[string]string{"TERM": "dumb", "TUIMARK_COLOR": "none"}, paint.ANSI256},
		{"none", map[string]string{"COLORTERM": "truecolor", "TERM": "xterm"}, paint.NoColor},
		{"truecolor", map[string]string{"TUIMARK_COLOR": "bogus"}, paint.TrueColor},
	} {
		read = false
		got, err := DetectColorProfile(c.flag, spy(c.env))
		if err != nil || got != c.want {
			t.Errorf("flag %q env %v: %v, %v; want %v", c.flag, c.env, got, err, c.want)
		}
		if c.flag != "" && read {
			t.Errorf("flag %q: the environment was read", c.flag)
		}
		// Run's config agrees whenever no flag is involved.
		if c.flag == "" {
			if cfg, err := readRunConfig(envMap(c.env)); err != nil || cfg.profile != got {
				t.Errorf("env %v: Run's profile %v, %v; DetectColorProfile %v", c.env, cfg.profile, err, got)
			}
		}
	}
	if _, err := DetectColorProfile("TrueColor", envMap(nil)); err == nil || !strings.Contains(err.Error(), "--color") {
		t.Errorf("invalid flag: %v", err)
	}
	if _, err := DetectColorProfile("", envMap(map[string]string{"TUIMARK_COLOR": "8"})); err == nil || !strings.Contains(err.Error(), "TUIMARK_COLOR") {
		t.Errorf("invalid TUIMARK_COLOR: %v", err)
	}
}

// §26.2 and §21 test 34: the probe runs only for a terminal output with a
// TERM other than dumb, and the skip rule holds for Apple Terminal and SSH
// unless TERM names a known terminal or WT_SESSION is set.
func TestShouldProbe(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		tty  bool
		want bool
	}{
		{"terminal", map[string]string{"TERM": "xterm-256color"}, true, true},
		{"empty TERM", map[string]string{}, true, true},
		{"not a terminal", map[string]string{"TERM": "xterm-256color"}, false, false},
		{"dumb", map[string]string{"TERM": "dumb"}, true, false},
		{"Apple Terminal", map[string]string{"TERM": "xterm-256color", "TERM_PROGRAM": "Apple_Terminal"}, true, false},
		{"SSH", map[string]string{"TERM": "xterm-256color", "SSH_TTY": "/dev/ttys003"}, true, false},
		{"empty SSH_TTY is unset", map[string]string{"TERM": "xterm-256color", "SSH_TTY": ""}, true, true},
		{"iTerm", map[string]string{"TERM": "xterm-256color", "TERM_PROGRAM": "iTerm.app"}, true, true},
		{"SSH into ghostty", map[string]string{"TERM": "xterm-ghostty", "SSH_TTY": "/dev/pts/1"}, true, true},
		{"SSH into wezterm", map[string]string{"TERM": "wezterm", "SSH_TTY": "/dev/pts/1"}, true, true},
		{"SSH into alacritty", map[string]string{"TERM": "alacritty", "SSH_TTY": "/dev/pts/1"}, true, true},
		{"SSH into kitty", map[string]string{"TERM": "xterm-kitty", "SSH_TTY": "/dev/pts/1"}, true, true},
		{"SSH into rio", map[string]string{"TERM": "rio", "SSH_TTY": "/dev/pts/1"}, true, true},
		{"Apple Terminal with WT_SESSION", map[string]string{"TERM_PROGRAM": "Apple_Terminal", "WT_SESSION": "x"}, true, true},
		{"SSH with WT_SESSION", map[string]string{"SSH_TTY": "/dev/pts/1", "WT_SESSION": "x"}, true, true},
		{"dumb beats the exceptions", map[string]string{"TERM": "dumb", "WT_SESSION": "x"}, true, false},
	}
	for _, c := range cases {
		for _, s := range []syncMode{syncAuto, syncOn, syncOff} {
			if got := shouldProbe(envMap(c.env), c.tty, s); got != c.want {
				t.Errorf("%s (sync %d): %v, want %v", c.name, s, got, c.want)
			}
		}
	}
	// TUIMARK_SYNC set leaves the 2026 query out; DA1 always ends it.
	if q := probeQueries(syncAuto); q != "\x1b[?2026$p\x1b[?2027$p\x1b[c" {
		t.Errorf("queries = %q", q)
	}
	for _, s := range []syncMode{syncOn, syncOff} {
		if q := probeQueries(s); q != "\x1b[?2027$p\x1b[c" {
			t.Errorf("queries with TUIMARK_SYNC = %q", q)
		}
	}
}

const probeDoc = `<tui version="1">
<style>screen { background: $bg; color: $fg; }</style>
<keymap><bind keys="z" action="quit"/></keymap>
<screen id="s" focus="#q">
  <input id="q" bind="text" on:change="chg" on:focus="hello"/>
  <list id="l" each="rows as r" key="r" on:select="pick"><item><text>{r}</text></item></list>
  <text>微信ok</text>
</screen>
</tui>`

// sessRig runs a terminal session on pipes: a scripted fake terminal. The
// test reads what the session wrote and types (or replies) through in.
type sessRig struct {
	app  *App
	in   *io.PipeWriter
	out  *syncBuf
	sigs chan os.Signal
	done chan error
	evs  chan Event
}

func startSession(t *testing.T, src string, sess *termSession) *sessRig {
	t.Helper()
	a := doc(t, src)
	_ = a.Bind("", map[string]any{"text": "", "rows": []any{"a", "b"}})
	r := &sessRig{app: a, out: &syncBuf{}, sigs: make(chan os.Signal, 1), done: make(chan error, 1), evs: make(chan Event, 64)}
	for _, act := range []string{"chg", "hello", "pick"} {
		a.On(act, func(ev Event) error { r.evs <- ev; return nil })
	}
	pr, pw := io.Pipe()
	r.in = pw
	go func() {
		r.done <- a.session(pr, r.out, func() (int, int) { return 20, 6 }, nil, r.sigs, sess, HostHooks{})
	}()
	t.Cleanup(func() { pw.Close() })
	return r
}

func (r *sessRig) send(t *testing.T, s string) {
	t.Helper()
	if _, err := io.WriteString(r.in, s); err != nil {
		t.Fatal(err)
	}
}

func (r *sessRig) waitOut(t *testing.T, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(r.out.String(), want) {
		if time.Now().After(deadline) {
			t.Fatalf("output never had %q: %q", want, r.out.String())
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func (r *sessRig) next(t *testing.T) Event {
	t.Helper()
	select {
	case ev := <-r.evs:
		return ev
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for an event")
	}
	return Event{}
}

func (r *sessRig) wait(t *testing.T) error {
	t.Helper()
	select {
	case err := <-r.done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("the session did not return")
	}
	return nil
}

var chaRe = regexp.MustCompile(`微\x1b\[\d+G`)

// §21 test 34, with replies: 2026 and 2027 reported off (Ps = 2). Frames
// are wrapped in synchronized output, mode 2027 is turned on before the
// first frame and off at exit, no CHA is written, and the keys typed
// during the probe are handled after the first frame (after its
// lifecycle events).
func TestProbeWithReplies(t *testing.T) {
	r := startSession(t, probeDoc, &termSession{probe: true, wait: 10 * time.Second})
	r.waitOut(t, "\x1b[?2026$p\x1b[?2027$p\x1b[c")
	r.send(t, "ab") // typed while the probe waits
	if got, want := r.out.String(), enterScreen+"\x1b[?2026$p\x1b[?2027$p\x1b[c"; got != want {
		t.Fatalf("before the replies the session wrote %q, want %q", got, want)
	}
	r.send(t, "\x1b[?2026;2$y\x1b[?2027;2$y\x1b[?64;1;2;22c")
	if ev := r.next(t); ev.Action != "hello" {
		t.Fatalf("first event %+v, want the first frame's on:focus", ev)
	}
	if ev := r.next(t); ev.Action != "chg" || ev.Value != "ab" {
		t.Fatalf("keys typed during the probe: %+v", ev)
	}
	r.send(t, "\x03") // ctrl+c: the built-in quit
	if err := r.wait(t); err != nil {
		t.Fatal(err)
	}
	out := r.out.String()
	afterProbe := strings.TrimPrefix(out, enterScreen+"\x1b[?2026$p\x1b[?2027$p\x1b[c")
	if !strings.HasPrefix(afterProbe, graphemeOn+syncBegin+"\x1b[0m\x1b[2J") {
		t.Errorf("mode 2027 and a synchronized first frame should follow the probe: %q", afterProbe[:min(60, len(afterProbe))])
	}
	if !strings.HasSuffix(out, graphemeOff+leaveScreen) {
		t.Fatalf("leave = %q", out[max(0, len(out)-40):])
	}
	frames := strings.TrimSuffix(strings.TrimPrefix(afterProbe, graphemeOn), graphemeOff+leaveScreen)
	if n := strings.Count(frames, syncBegin); n < 2 || n != strings.Count(frames, syncEnd) {
		t.Errorf("frames not wrapped one by one: %d begins, %d ends", n, strings.Count(frames, syncEnd))
	}
	for _, f := range strings.SplitAfter(frames, syncEnd) {
		if f != "" && (!strings.HasPrefix(f, syncBegin) || !strings.HasSuffix(f, syncEnd)) {
			t.Errorf("frame outside synchronized output: %q", f)
		}
	}
	if chaRe.MatchString(out) {
		t.Errorf("CHA written although mode 2027 is on: %q", out)
	}
	if !strings.Contains(out, "微信ok") {
		t.Errorf("the wide text was not written: %q", out)
	}
}

// §21 test 34, without replies: the first frame comes after the cap, with
// no synchronized output and CHA after complex clusters; 2027 is never
// turned on or off.
func TestProbeWithoutReplies(t *testing.T) {
	start := time.Now()
	r := startSession(t, probeDoc, &termSession{probe: true, wait: 150 * time.Millisecond})
	r.waitOut(t, "\x1b[2J\x1b[1;1H")
	if d := time.Since(start); d < 150*time.Millisecond {
		t.Errorf("first frame after %v, before the probe's cap", d)
	}
	if ev := r.next(t); ev.Action != "hello" {
		t.Fatalf("event %+v", ev)
	}
	// Late replies are swallowed without effect: no key, no mode change.
	r.send(t, "\x1b[?2026;2$y\x1b[?2027;2$y\x1b[?62c")
	r.send(t, "x")
	if ev := r.next(t); ev.Action != "chg" || ev.Value != "x" {
		t.Fatalf("after late replies: %+v", ev)
	}
	r.send(t, "\x03")
	if err := r.wait(t); err != nil {
		t.Fatal(err)
	}
	out := r.out.String()
	if strings.Contains(out, syncBegin) || strings.Contains(out, "2027h") || strings.Contains(out, "2027l") {
		t.Errorf("no replies, yet synchronized output or mode 2027: %q", out)
	}
	if !chaRe.MatchString(out) {
		t.Errorf("no CHA after the wide cluster: %q", out)
	}
	if !strings.HasSuffix(out, leaveScreen) {
		t.Errorf("leave = %q", out[max(0, len(out)-40):])
	}
}

// §26.5: Ps 1 or 3 means mode 2027 is already on: Run neither turns it on
// nor off, and writes no CHA. Ps 0 or 4 is "not supported": CHA.
func TestProbeGraphemeAlreadyOn(t *testing.T) {
	for _, c := range []struct {
		ps  string
		cha bool
	}{{"1", false}, {"3", false}, {"0", true}, {"4", true}} {
		r := startSession(t, probeDoc, &termSession{probe: true, wait: 10 * time.Second})
		r.waitOut(t, "\x1b[c")
		r.send(t, "\x1b[?2027;"+c.ps+"$y\x1b[?62c")
		r.next(t) // on:focus
		r.send(t, "\x03")
		if err := r.wait(t); err != nil {
			t.Fatal(err)
		}
		out := r.out.String()
		if strings.Contains(out, "2027h") || strings.Contains(out, "2027l") || strings.Contains(out, syncBegin) {
			t.Errorf("Ps=%s: %q", c.ps, out)
		}
		if chaRe.MatchString(out) != c.cha {
			t.Errorf("Ps=%s: CHA %v, want %v", c.ps, !c.cha, c.cha)
		}
	}
}

// §21 test 35 (session level): TUIMARK_SYNC=1 wraps frames without asking;
// TUIMARK_SYNC=0 never wraps, even when the terminal would support it.
// Either way the 2026 query is not sent.
func TestSyncForced(t *testing.T) {
	for _, c := range []struct {
		mode syncMode
		wrap bool
	}{{syncOn, true}, {syncOff, false}} {
		r := startSession(t, probeDoc, &termSession{probe: true, sync: c.mode, wait: 10 * time.Second})
		r.waitOut(t, "\x1b[c")
		r.send(t, "\x1b[?2026;1$y\x1b[?62c")
		r.next(t)
		r.send(t, "\x03")
		if err := r.wait(t); err != nil {
			t.Fatal(err)
		}
		out := r.out.String()
		if strings.Contains(out, querySync) {
			t.Errorf("sync %d: the 2026 query was sent", c.mode)
		}
		if strings.Contains(out, syncBegin) != c.wrap {
			t.Errorf("sync %d: wrapped %v, want %v", c.mode, !c.wrap, c.wrap)
		}
	}
	// Without the probe (not a terminal, dumb, skip rule) TUIMARK_SYNC=1
	// still forces it; unset, nothing is wrapped.
	r := startSession(t, probeDoc, &termSession{sync: syncOn})
	r.next(t)
	r.send(t, "\x03")
	r.wait(t)
	if out := r.out.String(); !strings.Contains(out, syncBegin) || strings.Contains(out, "$p") {
		t.Errorf("forced without a probe: %q", out)
	}
}

// §26.1: a stop signal during the probe stops the session exactly as
// during the loop; the leave sequence is written.
func TestProbeSignal(t *testing.T) {
	r := startSession(t, probeDoc, &termSession{probe: true, wait: time.Hour})
	r.waitOut(t, "\x1b[c")
	r.sigs <- syscall.SIGTERM
	var se *SignalError
	if err := r.wait(t); !errors.As(err, &se) {
		t.Fatalf("err = %v", err)
	}
	if out := r.out.String(); !strings.HasSuffix(out, leaveScreen) || strings.Contains(out, "\x1b[1;1H") {
		t.Errorf("out = %q", out)
	}
}

// §21 test 36 (EOF): input that ends during the probe still gets its keys
// handled after the first frame, and the leave sequence turns off
// bracketed paste and, when the session turned it on, mode 2027.
func TestSessionEOFLeave(t *testing.T) {
	// EOF during the probe: the queued keys are handled, then the session
	// ends without mode 2027.
	r := startSession(t, probeDoc, &termSession{probe: true, wait: time.Hour})
	r.waitOut(t, "\x1b[c")
	r.send(t, "xy")
	r.in.Close()
	if err := r.wait(t); err != nil {
		t.Fatal(err)
	}
	if ev := r.next(t); ev.Action != "hello" {
		t.Errorf("event %+v", ev)
	}
	if ev := r.next(t); ev.Action != "chg" || ev.Value != "xy" {
		t.Errorf("event %+v", ev)
	}
	if out := r.out.String(); !strings.HasSuffix(out, leaveScreen) || strings.Contains(out, "2027l") {
		t.Errorf("leave = %q", out[max(0, len(out)-40):])
	}
	// EOF after mode 2027 was turned on.
	r = startSession(t, probeDoc, &termSession{probe: true, wait: time.Hour})
	r.waitOut(t, "\x1b[c")
	r.send(t, "\x1b[?2027;2$y\x1b[?62c")
	r.next(t)
	r.in.Close()
	if err := r.wait(t); err != nil {
		t.Fatal(err)
	}
	if out := r.out.String(); !strings.HasSuffix(out, "\x1b[?2027l\x1b[?2004l\x1b[0m\x1b[?25h\x1b[?7h\x1b[?1049l") {
		t.Errorf("leave = %q", out[max(0, len(out)-40):])
	}
	// Loop (no probe) writes the same enter and leave sequences.
	rig := startLoop(t, nil)
	rig.next(t)
	rig.in.Close()
	rig.wait(t)
	if out := rig.out.String(); !strings.HasPrefix(out, "\x1b[?1049h\x1b[?25l\x1b[?7l\x1b[H\x1b[2J\x1b[?2004h") || !strings.HasSuffix(out, "\x1b[?2004l\x1b[0m\x1b[?25h\x1b[?7h\x1b[?1049l") {
		t.Errorf("Loop enter/leave: %q … %q", out[:min(30, len(out))], out[max(0, len(out)-30):])
	}
}

// §26.3: the session writes SGR in its profile; Loop stays truecolor.
func TestSessionProfile(t *testing.T) {
	for _, c := range []struct {
		p    paint.Profile
		want string
	}{
		{paint.TrueColor, "48;2;13;17;23"},
		{paint.ANSI256, "48;5;233"},
		{paint.ANSI16, ";40m"},
	} {
		r := startSession(t, probeDoc, &termSession{profile: c.p})
		r.next(t)
		r.send(t, "\x03")
		r.wait(t)
		if out := r.out.String(); !strings.Contains(out, c.want) {
			t.Errorf("%v: no %q in %q", c.p, c.want, out)
		}
	}
	r := startSession(t, probeDoc, &termSession{profile: paint.NoColor})
	r.next(t)
	r.send(t, "\x03")
	r.wait(t)
	if out := r.out.String(); strings.Contains(out, "38;") || strings.Contains(out, "48;") || strings.Contains(out, ";39") || !strings.Contains(out, "\x1b[0;7m") {
		t.Errorf("none: colors written or attributes lost: %q", out)
	}
}

// §26.7 and §21 test 28/32 (loop level): a bracketed paste into the
// focused input is one edit with one on:change carrying the normalized
// value, even split across reads; the same paste with a list focused, or
// into a disabled input, does nothing (its q does not quit).
func TestLoopPaste(t *testing.T) {
	rig := startLoop(t, nil)
	rig.next(t) // on:focus
	rig.send(t, "\x1b[200~q\r\n")
	rig.send(t, "x\tz\x1b[2")
	rig.send(t, "01~")
	if ev := rig.next(t); ev.Action != "chg" || ev.Value != "q x z" {
		t.Fatalf("paste = %+v", ev)
	}
	rig.waitScreen(t, "[q x z]")
	// An empty paste changes nothing and fires nothing; typing still works.
	rig.send(t, "\x1b[200~\x1b[31m\x1b[201~k")
	if ev := rig.next(t); ev.Action != "chg" || ev.Value != "q x zk" {
		t.Fatalf("after an empty paste: %+v", ev)
	}
	// Focus the list: a paste is discarded, never reaches the keymap.
	rig.send(t, "\t")
	rig.send(t, "\x1b[200~q\x1b[B\r\x1b[201~")
	rig.send(t, "\x1b[B")
	if ev := rig.next(t); ev.Action != "pick" || ev.Keys["r"] != "b" {
		t.Fatalf("after a paste on the list: %+v (the pasted down key must not move it)", ev)
	}
	select {
	case err := <-rig.done:
		t.Fatalf("a paste on the list quit the loop: %v", err)
	default:
	}
	rig.send(t, "q")
	if err := rig.wait(t); err != nil {
		t.Fatal(err)
	}
	// A disabled input takes no paste (focus is put on it by hand: the
	// runtime never focuses a disabled node).
	a := doc(t, v1(`<input id="d" bind="v" disabled="true" on:change="c"/>`))
	_ = a.Bind("v", "")
	a.Frame(20, 3)
	a.mu.Lock()
	a.focus = "d"
	b := a.focusedBox()
	a.mu.Unlock()
	if b == nil || !b.Disabled || b.Kind != "input" {
		t.Fatalf("focused box = %+v", b)
	}
	if evs := a.HandlePaste("text"); len(evs) != 0 {
		t.Errorf("disabled input: %+v", evs)
	}
	if v, _ := a.Get("v"); v != "" {
		t.Errorf("disabled input value = %v", v)
	}
}

// §26.7: HandlePaste inserts at the cursor as one edit (an input without
// on:change changes without an event), and nothing is focused → nothing.
func TestHandlePaste(t *testing.T) {
	n := doc(t, v1(`<text>nothing to focus</text>`))
	n.Frame(30, 4)
	if evs := n.HandlePaste("x"); len(evs) != 0 || n.Focus() != "" {
		t.Errorf("nothing focused: %+v", evs)
	}
	a := doc(t, v1(`<input id="q" bind="v"/><input id="p" bind="w" on:change="c"/>`))
	_ = a.Bind("", map[string]any{"v": "ab", "w": ""})
	a.Frame(30, 4)
	if a.Focus() != "q" {
		t.Fatalf("focus = %q", a.Focus())
	}
	a.HandleKey(Key{Name: "left"})
	if evs := a.HandlePaste("1\n2"); len(evs) != 0 {
		t.Errorf("no on:change: %+v", evs)
	}
	if v, _ := a.Get("v"); v != "a1 2b" {
		t.Errorf("value = %q, want the paste at the cursor", v)
	}
	a.HandleKey(Key{Name: "x", Rune: 'x'})
	if v, _ := a.Get("v"); v != "a1 2xb" {
		t.Errorf("cursor after the paste: value %q", v)
	}
	a.HandleKey(Key{Name: "tab"})
	a.Frame(30, 4)
	evs := a.HandlePaste("hello\tworld")
	if len(evs) != 1 || evs[0].Action != "c" || evs[0].Value != "hello world" {
		t.Errorf("on:change = %+v", evs)
	}
}

// §26.4 and §21 test 37 (Go side): SetTheme switches the token set for
// frames and keeps the static diagnostics exact; the environment never
// reaches Frame or Dump.
func TestSetTheme(t *testing.T) {
	src := `<tui version="1"><style>screen { background: $bg; color: $nope; }</style><screen id="s"><text>hi</text></screen></tui>`
	a := doc(t, src)
	bg := func() css.Color { return a.Frame(10, 2).Grid.At(5, 1).BG }
	dark, _ := css.ParseLiteralColor(css.Themes["dark"]["bg"])
	light, _ := css.ParseLiteralColor(css.Themes["light"]["bg"])
	t.Setenv("TUIMARK_THEME", "light")
	if got := bg(); got != dark {
		t.Errorf("Frame read TUIMARK_THEME: %v", got)
	}
	if a.Theme() != "dark" {
		t.Errorf("Theme() = %q", a.Theme())
	}
	before := diagText(a.Static())
	if !strings.Contains(before, "$nope") {
		t.Fatalf("static = %s", before)
	}
	if err := a.SetTheme("light"); err != nil {
		t.Fatal(err)
	}
	if got := bg(); got != light || a.Theme() != "light" {
		t.Errorf("light bg = %v, theme %q", got, a.Theme())
	}
	if got := diagText(a.Static()); got != before {
		t.Errorf("static diagnostics changed:\n%s\nwant\n%s", got, before)
	}
	if n := strings.Count(diagText(a.Dump(10, 2, false).Errors), "$nope"); n != 1 {
		t.Errorf("$nope reported %d times", n)
	}
	for _, bad := range []string{"auto", "Light", ""} {
		if err := a.SetTheme(bad); err == nil {
			t.Errorf("SetTheme(%q) accepted", bad)
		}
	}
	undo := a.overrideTheme("dark")
	if bg() != dark {
		t.Error("override to dark")
	}
	undo()
	if bg() != light {
		t.Error("undo did not restore light")
	}
	// A document theme stays the default until overridden.
	b := doc(t, strings.Replace(src, `version="1"`, `version="1" theme="light"`, 1))
	if b.Theme() != "light" {
		t.Errorf("document theme = %q", b.Theme())
	}
	undo = b.overrideTheme("")
	undo()
	if b.Theme() != "light" {
		t.Error("an empty override changed the theme")
	}
}

// diagText is one diagnostic per line.
func diagText(ds []ir.Diagnostic) string {
	var b strings.Builder
	for _, d := range ds {
		b.WriteString(d.String())
		b.WriteByte('\n')
	}
	return b.String()
}

// TestSessionTogglesAutowrap: a Run session turns autowrap (DECAWM) off on
// entry and back on when it leaves, so a last-column cluster that a
// terminal draws wider than Tuimark measured cannot wrap or scroll the
// alternate screen (SPEC v0.2 §26.1, §26.9).
func TestSessionTogglesAutowrap(t *testing.T) {
	if !strings.Contains(enterScreen, "\x1b[?7l") {
		t.Fatalf("enterScreen %q does not turn autowrap off", enterScreen)
	}
	if !strings.Contains(leaveScreen, "\x1b[?7h") || strings.Index(leaveScreen, "\x1b[?7h") > strings.Index(leaveScreen, "\x1b[?1049l") {
		t.Fatalf("leaveScreen %q must turn autowrap on before leaving the alternate screen", leaveScreen)
	}
}
