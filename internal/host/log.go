package host

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// runLog is the NDJSON session log of TUIMARK_LOG (SPEC v0.2b §26.12):
// Run writes it so that the terminal, which Run owns, is never written
// with diagnostics. Every method is safe on a nil *runLog (no log) and
// from any goroutine. A write error stops the logging and never stops
// Run.
type runLog struct {
	mu    sync.Mutex
	f     *os.File
	start time.Time
	dead  bool // a write failed, or the log was closed
}

// openRunLog opens path for the log (SPEC v0.2b §26.12): created with mode
// 0600 or truncated, then narrowed to 0600 whatever mode it had. An empty
// path is no log. An error is returned before Run touches the terminal.
func openRunLog(path string, start time.Time) (*runLog, error) {
	if path == "" {
		return nil, nil
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, fmt.Errorf("tuimark: %s: %v", envLog, err)
	}
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		return nil, fmt.Errorf("tuimark: %s: %v", envLog, err)
	}
	return &runLog{f: f, start: start}, nil
}

// member is one "name": value pair of a log record, in record order.
type member struct {
	name  string
	value any
}

// write appends one record: {"ev": ev, "t": T, members...}, serialized as
// the dump is (SPEC v0.2b §13.2: Go's encoding/json escaping), with t the
// whole milliseconds since Run started on the monotonic clock.
func (l *runLog) write(ev string, members ...member) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.dead {
		return
	}
	var b bytes.Buffer
	b.WriteString(`{"ev":`)
	writeJSON(&b, ev)
	b.WriteString(`,"t":`)
	writeJSON(&b, time.Since(l.start).Milliseconds())
	for _, m := range members {
		b.WriteByte(',')
		writeJSON(&b, m.name)
		b.WriteByte(':')
		writeJSON(&b, m.value)
	}
	b.WriteString("}\n")
	if _, err := l.f.Write(b.Bytes()); err != nil {
		l.dead = true
	}
}

// writeJSON writes v as compact JSON; a value encoding/json cannot write
// becomes null.
func writeJSON(b *bytes.Buffer, v any) {
	out, err := json.Marshal(v)
	if err != nil {
		out = []byte("null")
	}
	b.Write(out)
}

// end writes the end record with its reason (quit, eof, signal, or error)
// and closes the file. It is called once, on every exit path of Run.
func (l *runLog) end(reason string) {
	if l == nil {
		return
	}
	l.write("end", member{"reason", reason})
	l.mu.Lock()
	defer l.mu.Unlock()
	l.dead = true
	_ = l.f.Close()
}

// key logs one key handled. While the focused node is an enabled secret
// input, the record carries "redacted": true and no token (§26.12).
func (l *runLog) key(k Key, secret bool) {
	if secret {
		l.write("key", member{"redacted", true})
		return
	}
	l.write("key", member{"key", k.Name})
}

// paste logs one paste handled: the length of its normalized text, or,
// while a secret input has focus, "redacted": true only.
func (l *runLog) paste(normalized string, secret bool) {
	if secret {
		l.write("paste", member{"redacted", true})
		return
	}
	l.write("paste", member{"bytes", len(normalized)})
}

// action logs one dispatched event with its §8.2 payload. An event whose
// source is a secret input has no value and ends with "redacted": true.
func (l *runLog) action(ev Event, secretSource bool) {
	keys := ev.Keys
	if keys == nil {
		keys = map[string]any{}
	}
	if secretSource {
		l.write("action", member{"action", ev.Action}, member{"source", ev.Source}, member{"keys", keys}, member{"redacted", true})
		return
	}
	l.write("action", member{"action", ev.Action}, member{"source", ev.Source}, member{"keys", keys}, member{"value", ev.Value})
}

// secretFocused reports whether the focused node of the last frame is an
// enabled input with secret="true": while it is, the log redacts keys and
// pastes (SPEC v0.2b §26.12).
func (a *App) secretFocused() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	b := a.focusedBox()
	return b != nil && b.Kind == "input" && b.Secret && !b.Disabled
}

// secretInput reports whether id names an input with secret="true": the
// log never holds the value of its events.
func (a *App) secretInput(id string) bool {
	if id == "" {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	n := a.doc.IDs[id]
	return n != nil && n.Kind == "input" && n.Attrs["secret"] == "true"
}
