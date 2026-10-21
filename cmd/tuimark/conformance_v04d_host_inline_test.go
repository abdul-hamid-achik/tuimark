//go:build unix

package main

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// SPEC v0.4d §21 tests of phase D5a-2 over `tuimark host --inline` (§19.1):
// 301 (the commit over fd 3 in an inline session), 298's `host` line, and
// the rules of §19.1 "0.4d additions" that need a pty. The Go API parts of
// the commit are internal/host's, the TypeScript client's are sdk/ts and
// examples/host-ts, and the non-inline line of 301 is
// TestD5aWireCommit301.

const (
	d5aIn      = "../../testdata/v04d/t301/in.tui"
	d5aData    = "../../testdata/v04d/t301/in.json"
	d5aPinned  = "../../testdata/v04d/t301/pinned.tui"
	d5aEraseCR = "\r\x1b[J"
	// the commit of a line: its cells up to the last one that is not blank,
	// CSI 0 m, CR LF (§26.15.4).
	d5aHi = "hi\x1b[0m\r\n"
	// the leave sequence of an inline session (§26.15.3), after the CR CSI J.
	d5aLeave = "\x1b[?2004l\x1b[0m\x1b[?25h\x1b[?7h"
)

// startInlineHost starts `tuimark host in.tui --inline` and waits for its
// first frame and its ready message; it returns the rig and the index
// after ready.
func startInlineHost(t *testing.T, args ...string) (*hostRig, int) {
	t.Helper()
	argv := append([]string{"host", d5aIn, "--data", d5aData, "--inline"}, args...)
	r := startHost(t, hostOpts{cols: 40, rows: 12}, argv...)
	if !r.waitFor(r.screen, "INLINEAPP", 10*slowdown*time.Second) {
		t.Fatalf("no first frame; %s", r.describe())
	}
	i, _ := r.waitMsg(t, 0, ofType("ready"))
	return r, i
}

// d5aQuitInline quits with q and checks the inline restore: the screen ends
// with the leave sequence (no alternate screen), and the pty is cooked.
func d5aQuitInline(t *testing.T, r *hostRig) {
	t.Helper()
	r.write(t, "q")
	if code := r.exit(t, 10*slowdown*time.Second); code != 0 {
		t.Errorf("exit code %d, want 0", code)
	}
	r.assertLast(t, `{"type":"exit","reason":"quit"}`)
	d5aAssertLeft(t, r)
}

func d5aAssertLeft(t *testing.T, r *hostRig) {
	t.Helper()
	deadline := time.Now().Add(3 * slowdown * time.Second)
	for !strings.HasSuffix(r.screen.String(), d5aLeave) && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	s := r.screen.String()
	if !strings.HasSuffix(s, d5aLeave) {
		t.Errorf("the screen does not end with the inline leave sequence: ...%q", s[max(0, len(s)-80):])
	}
	if strings.Contains(s, "\x1b[?1049") {
		t.Errorf("an inline session wrote the alternate screen: %q", s)
	}
}

// SPEC 0.4d test 301: the commit over fd 3 in an inline session. `ready`
// lists the messages; a line commit is acked ok after its rows are written
// and prints above the region; a template commit prints the copy.
func TestD5a2HostInlineCommit301(t *testing.T) {
	r, i := startInlineHost(t)
	_, ready := r.waitMsg(t, 0, ofType("ready"))
	if !strings.HasSuffix(ready.raw, d5Ready) {
		t.Errorf("ready = %s\nwant it to end with %s", ready.raw, d5Ready)
	}
	if strings.Contains(r.screen.String(), "\x1b[?1049h") {
		t.Errorf("--inline entered the alternate screen")
	}

	// A line: acked ok, and its row is in the capture, written after a
	// CR CSI J that erased the region, with the region drawn again after it.
	before := len(r.screen.String())
	r.send(t, map[string]any{"type": "commit", "id": 1, "lines": []any{"hi"}})
	i = r.waitRaw(t, i, `{"type":"ack","id":1,"ok":true}`)
	if !r.waitFor(r.screen, d5aHi, 5*slowdown*time.Second) {
		t.Fatalf("the committed row never reached the terminal; %s", r.describe())
	}
	after := r.screen.String()[before:]
	if !strings.HasPrefix(after, d5aEraseCR) && !strings.Contains(after, d5aEraseCR+d5aHi) {
		t.Errorf("a commit does not start with CR CSI J: %q", after)
	}
	if !strings.Contains(after[strings.Index(after, d5aHi)+len(d5aHi):], "INLINEAPP") {
		t.Errorf("the region was not drawn again below the committed row: %q", after)
	}

	// A template's copy, with its props and data.
	r.send(t, map[string]any{"type": "commit", "id": 2, "template": "card", "with": "m as m", "props": map[string]any{"who": "Claude"}})
	i = r.waitRaw(t, i, `{"type":"ack","id":2,"ok":true}`)
	if !r.waitFor(r.screen, "Claude: hello\x1b[0m\r\n", 5*slowdown*time.Second) {
		t.Fatalf("the template's copy never reached the terminal; %s", r.describe())
	}

	// A run's class is a string of class names, as in §6.20.2: an array of
	// names, which the TypeScript client's types allowed by mistake, is the
	// rule's error.
	r.send(t, map[string]any{"type": "commit", "id": 5, "lines": []any{[]any{map[string]any{"text": "bold", "class": "k-strong k-em"}, " plain"}}})
	i = r.waitRaw(t, i, `{"type":"ack","id":5,"ok":true}`)
	r.send(t, map[string]any{"type": "commit", "id": 6, "lines": []any{[]any{map[string]any{"text": "x", "class": []any{"k-strong"}}}}})
	var l hostLine
	i, l = r.waitMsg(t, i, ackOf(6))
	d5aWantAckError(t, l, `commit: line 0: run 0 has a "class" that is not a list of class names`)

	// A commit that breaks a rule of §18.3 is acked with its error.
	r.send(t, map[string]any{"type": "commit", "id": 3, "template": "nope"})
	i, l = r.waitMsg(t, i, ackOf(3))
	d5aWantAckError(t, l, `commit: no <template id="nope">`)
	r.send(t, map[string]any{"type": "commit", "id": 4, "lines": []any{"a"}, "template": "card"})
	i, l = r.waitMsg(t, i, ackOf(4))
	d5aWantAckError(t, l, "commit: give either lines or template")
	r.send(t, map[string]any{"type": "commit", "lines": []any{5}})
	_, l = r.waitMsg(t, i, ofType("error"))
	if l.m["error"] != "commit: line 0 is a number, not a string or an array of runs" {
		t.Errorf("error = %s", l.raw)
	}
	d5aQuitInline(t, r)
}

// SPEC 0.4d test 301: a commit sent while an event waits for its reply is
// printed before the reply is sent (§19.1 "During a wait").
func TestD5a2HostInlineCommitDuringWait301(t *testing.T) {
	r, i := startInlineHost(t)
	r.write(t, "a")
	var ev hostLine
	i, ev = r.waitMsg(t, i, ofType("event"))
	if ev.m["action"] != "ask" {
		t.Fatalf("event %s", ev.raw)
	}
	// The loop is inside the handler, waiting. The commit is acked and its
	// row is on the terminal before any reply was sent.
	r.send(t, map[string]any{"type": "commit", "id": 10, "lines": []any{"hi"}})
	i = r.waitRaw(t, i, `{"type":"ack","id":10,"ok":true}`)
	if !r.waitFor(r.screen, d5aHi, 5*slowdown*time.Second) {
		t.Fatalf("the commit was not printed during the wait; %s", r.describe())
	}
	// Writes sent before the reply are drawn after the commit, in one frame.
	r.send(t, map[string]any{"type": "set", "id": 11, "path": "note", "value": "CHANGED"})
	i = r.waitRaw(t, i, `{"type":"ack","id":11,"ok":true}`)
	r.send(t, map[string]any{"type": "reply", "seq": ev.m["seq"], "id": 12})
	r.waitRaw(t, i, `{"type":"ack","id":12,"ok":true}`)
	if !r.waitFor(r.screen, "CHANGED", 5*slowdown*time.Second) {
		t.Fatalf("the frame after the reply never showed the set; %s", r.describe())
	}
	s := r.screen.String()
	if strings.Index(s, d5aHi) > strings.LastIndex(s, "CHANGED") {
		t.Errorf("the commit was printed after the frame that shows the set")
	}
	d5aQuitInline(t, r)
}

// SPEC 0.4d test 301: a commit and a reply with quit, written to fd 3 in
// one write, leave the committed rows above the cursor: the commit is
// printed before the leave sequence.
func TestD5a2HostInlineCommitThenQuit301(t *testing.T) {
	r, i := startInlineHost(t)
	r.write(t, "a")
	var ev hostLine
	i, ev = r.waitMsg(t, i, ofType("event"))
	seq := ev.m["seq"]
	commit := `{"type":"commit","id":20,"lines":["last words"]}`
	reply := `{"type":"reply","seq":` + jsonNum(seq) + `,"quit":true}`
	// One write: the reader sees both lines at once.
	if _, err := r.toFd3.WriteString(commit + "\n" + reply + "\n"); err != nil {
		t.Fatal(err)
	}
	_ = i
	if code := r.exit(t, 10*slowdown*time.Second); code != 0 {
		t.Errorf("exit code %d, want 0", code)
	}
	r.assertLast(t, `{"type":"exit","reason":"quit"}`)
	lines := r.snapshot()
	found := false
	for _, l := range lines {
		if l.raw == `{"type":"ack","id":20,"ok":true}` {
			found = true
		}
	}
	if !found {
		t.Errorf("the commit was not acked; %s", r.describe())
	}
	d5aAssertLeft(t, r)
	s := r.screen.String()
	row := "last words\x1b[0m\r\n"
	at := strings.LastIndex(s, row)
	if at < 0 {
		t.Fatalf("the committed row is not in the capture: %q", s)
	}
	d5aWantRowThenLeave(t, s, "last words")
}

// d5aWantRowThenLeave checks that row (a committed line) is in the capture
// s above the cursor: the last CR CSI J, which the leave sequence writes,
// comes after it, and only escape sequences follow that.
func d5aWantRowThenLeave(t *testing.T, s, row string) {
	t.Helper()
	text := row + "\x1b[0m\r\n"
	at := strings.LastIndex(s, text)
	if at < 0 {
		t.Errorf("the committed row %q is not in the capture: ...%q", row, s[max(0, len(s)-120):])
		return
	}
	erase := strings.LastIndex(s, d5aEraseCR)
	if erase < at+len(text) {
		t.Errorf("the committed row %q was printed after the leave sequence began", row)
		return
	}
	tail := s[erase+len(d5aEraseCR):]
	if !d5aOnlyEscapes.MatchString(tail) || !strings.HasSuffix(tail, d5aLeave) {
		t.Errorf("after the last CR CSI J the capture is not the leave sequence: %q", tail)
	}
}

var d5aOnlyEscapes = regexp.MustCompile(`^(\x1b\[[0-9;?<>]*[A-Za-z])*$`)

func ackOf(id int) func(hostLine) bool {
	return func(l hostLine) bool { return l.m["type"] == "ack" && l.m["id"] == float64(id) }
}

func d5aWantAckError(t *testing.T, l hostLine, want string) {
	t.Helper()
	if l.m["ok"] != false || l.m["error"] != want {
		t.Errorf("ack = %s, want ok false with error %q", l.raw, want)
	}
}

// SPEC 0.4d test 301: a commit accepted before fd 3 ends is printed before
// the leave sequence.
func TestD5a2HostInlineCommitThenEOF301(t *testing.T) {
	r, i := startInlineHost(t)
	r.send(t, map[string]any{"type": "commit", "id": 30, "lines": []any{"bye"}})
	r.waitRaw(t, i, `{"type":"ack","id":30,"ok":true}`)
	_ = r.toFd3.Close()
	if code := r.exit(t, 10*slowdown*time.Second); code != 0 {
		t.Errorf("exit code %d, want 0", code)
	}
	r.assertLast(t, `{"type":"exit","reason":"eof"}`)
	d5aAssertLeft(t, r)
	s := r.screen.String()
	d5aWantRowThenLeave(t, s, "bye")
}

// SPEC 0.4d test 298's `host` line: a document that writes version makes
// `host --inline` print the message and exit 1, with no ready and the
// terminal untouched.
func TestD5a2HostInlinePinned298(t *testing.T) {
	r := startHost(t, hostOpts{noProbe: true}, "host", d5aPinned, "--inline")
	if code := r.exit(t, 10*slowdown*time.Second); code != 1 {
		t.Errorf("exit code %d, want 1", code)
	}
	if s := r.stderr.String(); s != "tuimark: host: inline mode needs a <tui> without version\n" {
		t.Errorf("stderr = %q", s)
	}
	if lines := r.snapshot(); len(lines) != 0 {
		t.Errorf("fd 4 got %d messages, want none; %s", len(lines), r.describe())
	}
	if s := r.screen.String(); s != "" {
		t.Errorf("the terminal was touched: %q", s)
	}
}

// jsonNum prints a JSON number decoded as a float64 without an exponent.
func jsonNum(v any) string {
	f, _ := v.(float64)
	return strconv.FormatFloat(f, 'f', -1, 64)
}
