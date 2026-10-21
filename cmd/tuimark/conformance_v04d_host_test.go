//go:build unix

package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

// SPEC v0.4d §21 tests of phase D5a-1 over the host protocol (§19.1): 303's
// wire parts, 304, 313, and the non-inline line of 301. The Go API parts
// are the root package's conformance_v04d_host_test.go, the TypeScript
// client's are sdk/ts, and 301's inline lines wait for D5a-2.

const (
	d5Msgs = "../../testdata/v04d/t303/msgs.tui"
	d5Data = "../../testdata/v04d/t303/sample.json"
	d5Two  = "../../testdata/v04d/t313/two.tui"
)

// d5Ready is the exact tail of `ready` (SPEC v0.4d §19.1): messages come
// after diagnostics.
const d5Ready = `"diagnostics":[],"messages":["set","bind","batch","get","reply","append","commit"]}`

func d5Quit(t *testing.T, r *hostRig) {
	t.Helper()
	r.write(t, "q")
	if code := r.exit(t, 10*slowdown*time.Second); code != 0 {
		t.Errorf("exit code %d, want 0", code)
	}
	r.assertLast(t, `{"type":"exit","reason":"quit"}`)
	r.assertRestored(t)
}

// d5Get asks for path and returns the ack's JSON value text.
func d5Get(t *testing.T, r *hostRig, from int, id, path string) (int, string) {
	t.Helper()
	r.send(t, map[string]any{"type": "get", "id": id, "path": path})
	i, l := r.waitMsg(t, from, func(l hostLine) bool { return l.m["type"] == "ack" && l.m["id"] == id })
	v, _ := json.Marshal(l.m["value"])
	if l.m["found"] != true {
		return i, "<missing>"
	}
	return i, string(v)
}

// Test 304: ready has messages after diagnostics, and protocol is 1.
func TestD5aReadyMessages304(t *testing.T) {
	r := startHost(t, hostOpts{}, "host", d5Msgs, "--data", d5Data)
	if !r.waitFor(r.screen, "HOSTAPP", 10*slowdown*time.Second) {
		t.Fatalf("no first frame; %s", r.describe())
	}
	_, first := r.waitMsg(t, 0, func(hostLine) bool { return true })
	if first.m["type"] != "ready" || first.m["protocol"] != float64(1) {
		t.Errorf("first message %s", first.raw)
	}
	if !strings.HasSuffix(first.raw, d5Ready) {
		t.Errorf("ready = %s\nwant it to end with %s", first.raw, d5Ready)
	}
	d5Quit(t, r)
}

// Test 303 over fd 3: append, batch entries with op, malformed ops.
func TestD5aWireAppend303(t *testing.T) {
	r := startHost(t, hostOpts{}, "host", d5Msgs, "--data", d5Data)
	if !r.waitFor(r.screen, "HOSTAPP", 10*slowdown*time.Second) {
		t.Fatalf("no first frame; %s", r.describe())
	}
	i, _ := r.waitMsg(t, 0, ofType("ready"))

	r.send(t, map[string]any{"type": "append", "id": 7, "path": "messages", "value": map[string]any{"text": "hi"}})
	i = r.waitRaw(t, i, `{"type":"ack","id":7,"ok":true}`)
	if !r.waitFor(r.screen, "msg-hi", 5*slowdown*time.Second) {
		t.Fatalf("the frame never showed the append; %s", r.describe())
	}
	var got string
	i, got = d5Get(t, r, i, "g1", "messages")
	if got != `[{"text":"hi"}]` {
		t.Errorf("messages = %s", got)
	}

	// A batch entry with "op": "append", the second entry a set.
	r.send(t, map[string]any{"type": "batch", "id": 8, "writes": []any{
		map[string]any{"op": "append", "path": "live.text", "value": " world"},
		map[string]any{"path": "busy", "value": false},
	}})
	i = r.waitRaw(t, i, `{"type":"ack","id":8,"ok":true}`)
	i, got = d5Get(t, r, i, "g1b", "live.text")
	if got != `"hello world"` {
		t.Errorf("live.text = %s", got)
	}
	i, got = d5Get(t, r, i, "g1c", "busy")
	if got != "false" {
		t.Errorf("busy = %s", got)
	}
	// "op": "set" is the default spelled out; appends see earlier entries.
	r.send(t, map[string]any{"type": "batch", "id": "b2", "writes": []any{
		map[string]any{"op": "set", "path": "log", "value": []any{}},
		map[string]any{"op": "append", "path": "log", "value": "x"},
		map[string]any{"op": "append", "path": "log", "value": []any{1}},
	}})
	i = r.waitRaw(t, i, `{"type":"ack","id":"b2","ok":true}`)
	i, got = d5Get(t, r, i, "g2", "log")
	if got != `["x",[1]]` {
		t.Errorf("log = %s", got)
	}

	// An op that is neither set nor append makes the message malformed:
	// an error that echoes the id, and nothing of it is applied (the first
	// entry, valid, included).
	for n, op := range []any{"nope", "APPEND", "", nil, 5, true, []any{}} {
		id := 100 + n
		r.send(t, map[string]any{"type": "batch", "id": id, "writes": []any{
			map[string]any{"path": "n", "value": 99},
			map[string]any{"op": op, "path": "n", "value": 1},
		}})
		var l hostLine
		i, l = r.waitMsg(t, i, func(l hostLine) bool { return l.m["type"] == "error" && l.m["id"] == float64(id) })
		if l.m["error"] != `"writes"[1] "op" must be "set" or "append"` {
			t.Errorf("op %v: %s", op, l.raw)
		}
	}
	i, got = d5Get(t, r, i, "g3", "n")
	if got != "3" {
		t.Errorf("a malformed batch applied a write: n = %s", got)
	}

	// A failing append is acked with the error of §18.3 and changes
	// nothing; without an id it is an error message.
	r.send(t, map[string]any{"type": "append", "id": 20, "path": "n", "value": 1})
	var l hostLine
	i, l = r.waitMsg(t, i, func(l hostLine) bool { return l.m["type"] == "ack" && l.m["id"] == float64(20) })
	if l.raw != `{"type":"ack","id":20,"ok":false,"error":"append \"n\": \"n\" holds a number, not a string or an array"}` {
		t.Errorf("failing append ack = %s", l.raw)
	}
	r.send(t, map[string]any{"type": "append", "path": "missing", "value": "x"})
	i = r.waitRaw(t, i, `{"type":"error","error":"append \"missing\": \"missing\" is missing"}`)
	// ... and a failing append in a batch fails the batch as a whole.
	r.send(t, map[string]any{"type": "batch", "id": 21, "writes": []any{
		map[string]any{"path": "busy", "value": true},
		map[string]any{"op": "append", "path": "n", "value": 1},
	}})
	i, l = r.waitMsg(t, i, func(l hostLine) bool { return l.m["type"] == "ack" && l.m["id"] == float64(21) })
	if l.raw != `{"type":"ack","id":21,"ok":false,"error":"append \"n\": \"n\" holds a number, not a string or an array"}` {
		t.Errorf("failing batch ack = %s", l.raw)
	}
	i, got = d5Get(t, r, i, "g4", "busy")
	if got != "false" {
		t.Errorf("a failed batch applied a write: busy = %s", got)
	}

	// Malformed appends.
	r.send(t, map[string]any{"type": "append", "id": 30, "value": "x"})
	i, l = r.waitMsg(t, i, func(l hostLine) bool { return l.m["type"] == "error" && l.m["id"] == float64(30) })
	if l.m["error"] != `"append" needs a string "path"` {
		t.Errorf("append without path: %s", l.raw)
	}
	r.send(t, map[string]any{"type": "append", "id": 31, "path": "messages"})
	i, l = r.waitMsg(t, i, func(l hostLine) bool { return l.m["type"] == "error" && l.m["id"] == float64(31) })
	if l.m["error"] != `"append" needs "value"` {
		t.Errorf("append without value: %s", l.raw)
	}
	d5Quit(t, r)
}

// Test 301's non-inline line, and the parse of `commit` (§19.1): without
// --inline a well-formed commit is acked with the error of §18.3; a
// malformed one is an error message, and nothing is printed either way.
func TestD5aWireCommit301(t *testing.T) {
	r := startHost(t, hostOpts{}, "host", d5Msgs, "--data", d5Data)
	if !r.waitFor(r.screen, "HOSTAPP", 10*slowdown*time.Second) {
		t.Fatalf("no first frame; %s", r.describe())
	}
	i, _ := r.waitMsg(t, 0, ofType("ready"))
	notInline := `{"type":"ack","id":1,"ok":false,"error":"commit: not in inline mode"}`

	r.send(t, map[string]any{"type": "commit", "id": 1, "lines": []any{"hi"}})
	i = r.waitRaw(t, i, notInline)
	r.send(t, map[string]any{"type": "commit", "id": 1, "template": "t", "with": "a as b", "props": map[string]any{"k": "v"}})
	i = r.waitRaw(t, i, notInline)
	// The rules of §18.3 are judged after "not in inline mode", so a commit
	// that breaks one is the same ack here.
	r.send(t, map[string]any{"type": "commit", "id": 1})
	i = r.waitRaw(t, i, notInline)
	// Without an id the failure is an error message.
	r.send(t, map[string]any{"type": "commit", "lines": []any{"hi"}})
	i = r.waitRaw(t, i, `{"type":"error","error":"commit: not in inline mode"}`)

	malformed := []struct {
		msg  map[string]any
		want string
	}{
		{map[string]any{"lines": "hi"}, `"commit" needs "lines" to be an array`},
		{map[string]any{"lines": nil}, `"commit" needs "lines" to be an array`},
		{map[string]any{"lines": map[string]any{}}, `"commit" needs "lines" to be an array`},
		{map[string]any{"template": 5}, `"commit" needs "template" to be a string`},
		{map[string]any{"template": "t", "with": []any{}}, `"commit" needs "with" to be a string`},
		{map[string]any{"template": "t", "props": "x"}, `"commit" needs "props" to be an object of strings`},
		{map[string]any{"template": "t", "props": []any{"x"}}, `"commit" needs "props" to be an object of strings`},
		{map[string]any{"template": "t", "props": map[string]any{"k": 1}}, `"commit" needs "props" to be an object of strings`},
	}
	for n, c := range malformed {
		id := 50 + n
		msg := map[string]any{"type": "commit", "id": id}
		for k, v := range c.msg {
			msg[k] = v
		}
		r.send(t, msg)
		var l hostLine
		i, l = r.waitMsg(t, i, func(l hostLine) bool { return l.m["type"] == "error" && l.m["id"] == float64(id) })
		if l.m["error"] != c.want {
			t.Errorf("%v: %s, want error %q", c.msg, l.raw, c.want)
		}
	}
	d5Quit(t, r)
}

// Test 313 over `host`: --screen picks the first frame's screen, a get of
// @screen says so, and an unknown id is a pre-session error with nothing
// on fd 4 and the terminal untouched.
func TestD5aHostScreen313(t *testing.T) {
	r := startHost(t, hostOpts{}, "host", d5Two, "--screen", "b")
	if !r.waitFor(r.screen, "SCREEN-B", 10*slowdown*time.Second) {
		t.Fatalf("the first frame is not screen b; %s", r.describe())
	}
	if strings.Contains(r.screen.String(), "SCREEN-A") {
		t.Errorf("the session wrote a row of screen a")
	}
	i, first := r.waitMsg(t, 0, func(hostLine) bool { return true })
	if first.m["type"] != "ready" {
		t.Errorf("first fd 4 message %s, want ready", first.raw)
	}
	var got string
	_, got = d5Get(t, r, i, "g", "@screen")
	if got != `"b"` {
		t.Errorf("@screen = %s", got)
	}
	d5Quit(t, r)

	// Without --screen the first frame shows a.
	r = startHost(t, hostOpts{}, "host", d5Two)
	if !r.waitFor(r.screen, "SCREEN-A", 10*slowdown*time.Second) {
		t.Fatalf("the first frame is not screen a; %s", r.describe())
	}
	i, _ = r.waitMsg(t, 0, ofType("ready"))
	_, got = d5Get(t, r, i, "g", "@screen")
	if got != `"a"` {
		t.Errorf("@screen = %s", got)
	}
	d5Quit(t, r)

	// An unknown id: one line, exit 1, nothing on fd 4, nothing on the
	// terminal.
	r = startHost(t, hostOpts{noProbe: true}, "host", d5Two, "--screen", "nope")
	if code := r.exit(t, 10*slowdown*time.Second); code != 1 {
		t.Errorf("exit code %d, want 1", code)
	}
	if s := r.stderr.String(); s != "tuimark: host: no screen with id \"nope\"\n" {
		t.Errorf("stderr = %q", s)
	}
	if lines := r.snapshot(); len(lines) != 0 {
		t.Errorf("fd 4 got %d messages, want none; %s", len(lines), r.describe())
	}
	if s := r.screen.String(); s != "" {
		t.Errorf("the terminal was touched: %q", s)
	}
	// An explicit empty id names no screen either.
	r = startHost(t, hostOpts{noProbe: true}, "host", d5Two, "--screen", "")
	if code := r.exit(t, 10*slowdown*time.Second); code != 1 {
		t.Errorf("--screen \"\": exit code %d, want 1", code)
	}
	if s := r.stderr.String(); s != "tuimark: host: no screen with id \"\"\n" {
		t.Errorf("--screen \"\": stderr = %q", s)
	}
}

// Test 313 over `run`: --screen b shows b from the first frame, and a
// reload keeps it; an unknown id is a pre-session error.
func TestD5aRunScreen313(t *testing.T) {
	dir := t.TempDir()
	doc, err := os.ReadFile(d5Two)
	if err != nil {
		t.Fatal(err)
	}
	file := writeIn(t, dir, "two.tui", string(doc))
	r := startHost(t, hostOpts{cols: 60, rows: 12}, "run", file, "--screen", "b")
	if !r.waitFor(r.screen, "SCREEN-B", 10*slowdown*time.Second) {
		t.Fatalf("the first frame is not screen b; %s", runDescribe(r))
	}
	if strings.Contains(r.screen.String(), "SCREEN-A") {
		t.Errorf("the session wrote a row of screen a")
	}
	// A reload keeps the screen the session is on.
	writeAtomic(t, file, strings.ReplaceAll(string(doc), "<text>SCREEN-B</text>", "<text>SCREEN-B</text><text>RELOADED</text>"))
	if !r.waitFor(r.screen, "RELOADED", 5*slowdown*time.Second) {
		t.Fatalf("no reload; %s", runDescribe(r))
	}
	if strings.Contains(r.screen.String(), "SCREEN-A") {
		t.Errorf("the reload went back to screen a; %s", runDescribe(r))
	}
	r.write(t, "q")
	if code := r.exit(t, 5*slowdown*time.Second); code != 0 {
		t.Fatalf("quit exited %d; %s", code, runDescribe(r))
	}
	r.assertRestored(t)

	r = startHost(t, hostOpts{noProbe: true}, "run", file, "--screen", "nope")
	if code := r.exit(t, 10*slowdown*time.Second); code != 1 {
		t.Errorf("exit code %d, want 1", code)
	}
	if s := r.stderr.String(); s != "tuimark: run: no screen with id \"nope\"\n" {
		t.Errorf("stderr = %q", s)
	}
	if s := r.screen.String(); s != "" {
		t.Errorf("the terminal was touched: %q", s)
	}
}
