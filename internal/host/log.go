package host

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"golang.org/x/term"
)

// runLog is the NDJSON session log of TUIMARK_LOG (SPEC v0.2b §26.12):
// Run writes it so that the terminal, which Run owns, is never written
// with diagnostics. Every method is safe on a nil *runLog (no log) and
// from any goroutine. A write error stops the logging and never stops
// Run.
type runLog struct {
	mu      sync.Mutex
	f       *os.File
	start   time.Time
	started bool // the start record was written: a session began
	dead    bool // a write failed, or the log was closed
}

// openRunLog opens path for the log (SPEC v0.2b §26.12), creating it with
// mode 0600 when it does not exist. What it opened decides the rest: a
// regular file (a symlink to one included) is truncated and narrowed to
// 0600 whatever mode it had; a terminal is refused, since the log exists
// to keep diagnostics off terminals; anything else (/dev/null, a FIFO, a
// pipe such as a shell's /dev/fd/N) is written as it is, never truncated
// and never chmod-ed. An empty path is no log. An error is returned before
// Run touches the terminal.
func openRunLog(path string, start time.Time) (*runLog, error) {
	if path == "" {
		return nil, nil
	}
	fail := func(f *os.File, err error) (*runLog, error) {
		if f != nil {
			f.Close()
		}
		return nil, fmt.Errorf("tuimark: %s: %v", envLog, err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|openNoCTTY, 0o600)
	if err != nil {
		return fail(nil, err)
	}
	st, err := f.Stat()
	if err != nil {
		return fail(f, err)
	}
	if st.Mode().IsRegular() {
		if err := f.Truncate(0); err != nil {
			return fail(f, err)
		}
		if err := f.Chmod(0o600); err != nil {
			return fail(f, err)
		}
		return &runLog{f: f, start: start}, nil
	}
	if isTerminalFile(f) {
		return fail(f, fmt.Errorf("%s is a terminal; the log never goes to a terminal (use a file or a pipe)", path))
	}
	return &runLog{f: f, start: start}, nil
}

// isTerminalFile reports whether f is a terminal, without changing its
// blocking mode (f.Fd would).
func isTerminalFile(f *os.File) bool {
	rc, err := f.SyscallConn()
	if err != nil {
		return false
	}
	tty := false
	if err := rc.Control(func(fd uintptr) { tty = term.IsTerminal(int(fd)) }); err != nil {
		return false
	}
	return tty
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

// begin writes the start record, the first of a session: from here on,
// end writes the end record.
func (l *runLog) begin(cols, rows int) {
	if l == nil {
		return
	}
	l.write("start", member{"version", Version}, member{"cols", cols}, member{"rows", rows})
	l.mu.Lock()
	l.started = true
	l.mu.Unlock()
}

// end writes the end record with its reason (quit, eof, signal, or error)
// and closes the file. It is called once, on every exit path of Run. When
// Run returns before its session began (stdin is not a terminal, or raw
// mode failed), there is no start record, so no end record is written
// either: the file is closed with no records (SPEC v0.2b §26.12).
func (l *runLog) end(reason string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	started := l.started
	l.mu.Unlock()
	if started {
		l.write("end", member{"reason", reason})
	}
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
