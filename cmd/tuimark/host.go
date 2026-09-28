package main

// tuimark host FILE: the host protocol of SPEC v0.3b §19.1. This file is
// the bridge: it owns fd 3 (parent -> child) and fd 4 (child -> parent),
// forwards host actions as "event" messages and waits for their "reply",
// serves "set"/"bind"/"batch"/"get" concurrently, and translates
// internal/host's session-start hook, stop request, and stop-signal
// notice (RunHost) into the wire protocol. internal/host does all the
// laying out, painting, and deciding; this bridge only speaks JSON.

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/abdul-hamid-achik/tuimark/internal/host"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// hostProtocol is the version of §19.1 this bridge speaks ("ready"'s
// protocol member): "a parent must check it and stop on a value it does
// not know."
const hostProtocol = 1

// hostQueueSize bounds the fd 4 send queue (§19.1 "Backpressure").
const hostQueueSize = 1024

// hostExitWait bounds the final "exit" write (§19.1 step 4).
const hostExitWait = time.Second

func (c *cli) cmdHost(args []string) int {
	fs := c.newFlags("host")
	data := fs.String("data", "", "JSON file bound as the data store")
	theme := fs.String("theme", "", "dark or light; applied as Set(\"@theme\", …)")
	replyTimeoutFlag := fs.String("reply-timeout", "5s", "how long a host action waits for the parent's reply (100ms to 1m)")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return 1
	}
	file, err := oneFile("host", pos)
	if err != nil {
		return c.hostFail(err)
	}
	replyTimeout, err := parseReplyTimeout(*replyTimeoutFlag)
	if err != nil {
		return c.hostFail(err)
	}
	setTheme := themeFlagSet(fs)
	if setTheme && *theme != "dark" && *theme != "light" {
		return c.hostFail(fmt.Errorf("--theme must be dark or light (got %q)", *theme))
	}
	// The fd 3/4 checks run before the document loads or the terminal is
	// touched (§19.1 "Checking fd 3 and fd 4"); on a platform without
	// them (Windows) this is the usage error §19.1 names.
	fd3, fd4, err := host.OpenHostFDs()
	if err != nil {
		return c.hostFail(err)
	}
	app, err := load(file, *data)
	if err != nil {
		return c.hostFail(err)
	}
	// --theme is applied as Set("@theme", …); TUIMARK_THEME still beats it
	// in RunHost, exactly as in Run (§15.1, §26.4).
	if setTheme {
		if err := app.Set(host.ThemePath, *theme); err != nil {
			return c.hostFail(err)
		}
	}
	return newHostBridge(app, fd3, fd4, replyTimeout).run(c.stdout, c.stderr)
}

// hostFail prints a usage or pre-session error as one "tuimark: host: …"
// line (§19.1 step 1) and returns exit code 1. The runtime's own errors
// already start with "tuimark: ", which is dropped so the line carries the
// prefix once.
func (c *cli) hostFail(err error) int {
	fmt.Fprintf(c.stderr, "tuimark: host: %s\n", hostErrText(err))
	return 1
}

func hostErrText(err error) string {
	s := err.Error()
	for _, p := range []string{"tuimark: ", "tuimark host: "} {
		s = strings.TrimPrefix(s, p)
	}
	return s
}

func parseReplyTimeout(s string) (time.Duration, error) {
	d, err := time.ParseDuration(s)
	if err != nil || d < 100*time.Millisecond || d > time.Minute {
		return 0, fmt.Errorf("--reply-timeout must be a duration from 100ms to 1m (got %q)", s)
	}
	return d, nil
}

// ── Wire messages (child -> parent) ─────────────────────────────────────
//
// Structs, not maps, so every message is written with "type" first and
// its members in the order §19.1 lists them.

type catalogEntry struct {
	Name    string   `json:"name"`
	Builtin bool     `json:"builtin"`
	Sources []string `json:"sources"`
}

type readyMsg struct {
	Type        string          `json:"type"`
	Protocol    int             `json:"protocol"`
	Version     string          `json:"version"`
	Cols        int             `json:"cols"`
	Rows        int             `json:"rows"`
	Theme       string          `json:"theme"`
	Catalog     []catalogEntry  `json:"catalog"`
	Diagnostics []ir.Diagnostic `json:"diagnostics"`
}

type eventMsg struct {
	Type   string         `json:"type"`
	Seq    int64          `json:"seq"`
	Action string         `json:"action"`
	Source string         `json:"source"`
	Keys   map[string]any `json:"keys"`
	Value  any            `json:"value"`
}

type ackMsg struct {
	Type  string          `json:"type"`
	ID    json.RawMessage `json:"id"`
	OK    bool            `json:"ok"`
	Error *string         `json:"error,omitempty"`
}

type getAckMsg struct {
	Type  string          `json:"type"`
	ID    json.RawMessage `json:"id"`
	OK    bool            `json:"ok"`
	Found bool            `json:"found"`
	Value json.RawMessage `json:"value,omitempty"`
}

// errorMsg's Seq is raw JSON, so a reply's seq is echoed exactly as the
// parent wrote it (1.0 stays 1.0, and no digit of a large one is lost).
type errorMsg struct {
	Type  string          `json:"type"`
	ID    json.RawMessage `json:"id,omitempty"`
	Seq   json.RawMessage `json:"seq,omitempty"`
	Error string          `json:"error"`
}

type exitMsg struct {
	Type   string  `json:"type"`
	Reason string  `json:"reason"`
	Signal string  `json:"signal,omitempty"`
	Error  *string `json:"error,omitempty"`
}

// ── The bridge ──────────────────────────────────────────────────────────

// hostBridge is one `tuimark host` session's protocol state: the fd 4
// send queue, the one outstanding event/reply wait (the loop runs one
// handler at a time, so there is never more than one), and the channels
// RunHost's hooks read.
type hostBridge struct {
	app          *host.App
	fd3          *os.File
	out          *fd4Queue
	replyTimeout time.Duration

	mu       sync.Mutex
	seq      int64
	pending  *hostWait
	timedOut map[int64]bool

	// stop is RunHost's stop request (hooks.Stop), sent to at most once;
	// ending is closed at the same moment, so a pending wait ends and no
	// further event is forwarded. signaled is RunHost's stop-signal notice
	// (hooks.Signaled).
	stop       chan host.HostStop
	stopOnce   sync.Once
	ending     chan struct{}
	signaled   chan struct{}
	readySent  atomic.Bool
	readerOnce sync.Once
}

// hostWait is the pending event's wait for its reply.
type hostWait struct {
	seq   int64
	reply chan hostReply
}

type hostReply struct {
	quit   bool
	err    bool
	errMsg string
}

func newHostBridge(app *host.App, fd3, fd4 *os.File, replyTimeout time.Duration) *hostBridge {
	b := &hostBridge{
		app: app, fd3: fd3, replyTimeout: replyTimeout,
		timedOut: map[int64]bool{},
		stop:     make(chan host.HostStop, 1), ending: make(chan struct{}), signaled: make(chan struct{}),
	}
	b.out = newFD4Queue(fd4, hostQueueSize, func(err error) { b.requestStop(host.HostStop{Err: err}) })
	return b
}

// run drives the session end to end and returns the process exit code
// (§19.1 step 4: 0 for quit and eof, 1 for error, 128 + the signal number
// for signal).
func (b *hostBridge) run(stdout, stderr io.Writer) int {
	specs := b.app.Catalog()
	catalog := make([]catalogEntry, len(specs))
	for i, s := range specs {
		sources := s.Sources
		if sources == nil {
			sources = []string{}
		}
		catalog[i] = catalogEntry{Name: s.Name, Builtin: s.Builtin, Sources: sources}
		if !s.Builtin {
			// Every non-builtin action of the catalog gets a handler that
			// forwards its event (§19.1 step 3); built-ins (quit, focus,
			// the §8.4 ones) behave as in Run.
			b.app.On(s.Name, b.forward)
		}
	}
	hooks := host.HostHooks{
		Start: func(cols, rows int, theme string) {
			b.send(readyMsg{
				Type: "ready", Protocol: hostProtocol, Version: version,
				Cols: cols, Rows: rows, Theme: theme,
				Catalog: catalog, Diagnostics: diagsOrEmpty(b.app.Static()),
			})
			b.readySent.Store(true)
			// fd 3 is read only from here on, so "ready" is always the
			// first message on fd 4: nothing a parent sends earlier is
			// answered before it.
			b.readerOnce.Do(func() { go b.readLoop() })
		},
		Stop:     b.stop,
		Signaled: b.signaled,
	}
	res := b.app.RunHost(stdout, hooks)
	if !res.Started {
		// A failure before the session started (§19.1 step 1): a stderr
		// line and exit 1, with no "ready" and no "exit".
		b.out.finish(nil, 0)
		fmt.Fprintf(stderr, "tuimark: host: %s\n", hostErrText(res.Err))
		return 1
	}
	msg, code := hostExit(res)
	var final []byte
	if b.readySent.Load() {
		// "exit" closes what "ready" opened: a session cut before "ready"
		// (a stop signal during the probe) writes neither.
		final = marshalLine(msg)
	}
	b.out.finish(final, hostExitWait)
	_ = b.out.f.Close()
	_ = b.fd3.Close()
	return code
}

// hostExit builds the "exit" message and the exit code for a RunHost
// result.
func hostExit(res host.HostResult) (exitMsg, int) {
	switch res.Reason {
	case "signal":
		return exitMsg{Type: "exit", Reason: "signal", Signal: signalName(res.Signal)}, 128 + signalNumber(res.Signal)
	case "error":
		text := ""
		if res.Err != nil {
			text = res.Err.Error()
		}
		return exitMsg{Type: "exit", Reason: "error", Error: &text}, 1
	case "eof":
		return exitMsg{Type: "exit", Reason: "eof"}, 0
	}
	return exitMsg{Type: "exit", Reason: "quit"}, 0
}

// requestStop sends RunHost's stop request once (fd 3 at end of file, or
// the fd 4 queue failing) and closes ending, which ends a pending wait at
// once and stops every later handler from forwarding.
func (b *hostBridge) requestStop(s host.HostStop) {
	b.stopOnce.Do(func() {
		close(b.ending)
		b.stop <- s
	})
}

// forward is the handler of every non-builtin action (§19.1 "The wait"):
// it writes one "event" with a fresh seq and waits for the "reply" with
// that seq, the reply timeout, a stop signal, or a stop request, whichever
// comes first.
func (b *hostBridge) forward(ev host.Event) error {
	select {
	case <-b.signaled:
		// After a stop signal every handler returns nil at once, without
		// forwarding.
		return nil
	case <-b.ending:
		// After fd 3's end of file (or a failing fd 4) no further event is
		// forwarded.
		return nil
	default:
	}
	w := &hostWait{reply: make(chan hostReply, 1)}
	b.mu.Lock()
	b.seq++
	w.seq = b.seq
	b.pending = w
	b.mu.Unlock()
	defer b.clearWait(w)

	keys := ev.Keys
	if keys == nil {
		keys = map[string]any{}
	}
	if !b.send(eventMsg{Type: "event", Seq: w.seq, Action: ev.Action, Source: ev.Source, Keys: keys, Value: ev.Value}) {
		return nil // fd 4 is failing: the stop request is on its way
	}
	timer := time.NewTimer(b.replyTimeout)
	defer timer.Stop()
	select {
	case r := <-w.reply:
		return r.result()
	case <-b.signaled:
		return nil
	case <-b.ending:
		return nil
	case <-timer.C:
	}
	// Timed out. A reply that raced the timer and was already matched
	// still counts; anything later is a late reply.
	b.mu.Lock()
	if b.pending == w {
		b.pending = nil
		b.timedOut[w.seq] = true
	}
	b.mu.Unlock()
	select {
	case r := <-w.reply:
		return r.result()
	default:
	}
	b.send(errorMsg{Type: "error", Seq: json.RawMessage(strconv.FormatInt(w.seq, 10)), Error: "reply timeout"})
	return nil
}

func (b *hostBridge) clearWait(w *hostWait) {
	b.mu.Lock()
	if b.pending == w {
		b.pending = nil
	}
	b.mu.Unlock()
}

// result is what the waiting handler returns for this reply (§19.1 "The
// reply"): the error when there is one (it wins over quit), else ErrQuit
// for quit, else nil.
func (r hostReply) result() error {
	switch {
	case r.err:
		return errors.New(r.errMsg)
	case r.quit:
		return host.ErrQuit
	}
	return nil
}

func marshalLine(v any) []byte {
	line, err := json.Marshal(v)
	if err != nil {
		// Only a value the store could not hold reaches here; report it
		// instead of writing a broken line.
		line, _ = json.Marshal(errorMsg{Type: "error", Error: "cannot encode message: " + err.Error()})
	}
	return append(line, '\n')
}

// send queues one message on fd 4; false when the queue is full or
// failing (the stop request is then on its way).
func (b *hostBridge) send(v any) bool {
	return b.out.send(marshalLine(v))
}

// ── Parent -> child (fd 3) ──────────────────────────────────────────────

// readLoop reads fd 3 one line at a time, in order, from one goroutine
// (§19.1 "Order"), until end of file or a read error: the parent is gone,
// so the session ends with reason eof (§19.1 "Parent gone").
func (b *hostBridge) readLoop() {
	r := bufio.NewReaderSize(b.fd3, 64*1024)
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			b.handleLine(line)
		}
		if err != nil {
			break
		}
	}
	b.requestStop(host.HostStop{EOF: true})
}

// wireMsg is one parsed input line, kept as raw members so each is
// checked against its type (§19.1 "Malformed input").
type wireMsg map[string]json.RawMessage

// member kinds, from the raw JSON's first byte.
func jsonKind(raw json.RawMessage) byte {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return 0
	}
	switch c := raw[0]; {
	case c == '"':
		return 's'
	case c == '-' || (c >= '0' && c <= '9'):
		return 'n'
	case c == 't' || c == 'f':
		return 'b'
	case c == '{':
		return 'o'
	case c == '[':
		return 'a'
	}
	return 0 // null
}

func (m wireMsg) str(k string) (string, bool) {
	raw, ok := m[k]
	if !ok || jsonKind(raw) != 's' {
		return "", false
	}
	var s string
	return s, json.Unmarshal(raw, &s) == nil
}

func (b *hostBridge) handleLine(line []byte) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return
	}
	var msg wireMsg
	if err := json.Unmarshal(line, &msg); err != nil || msg == nil {
		b.sendError(nil, nil, "not a JSON object")
		return
	}
	// id is optional everywhere; when present it must be a number or a
	// string, and only a valid id is echoed.
	var id json.RawMessage
	if raw, ok := msg["id"]; ok {
		if k := jsonKind(raw); k != 'n' && k != 's' {
			b.sendError(nil, nil, `"id" must be a number or a string`)
			return
		}
		id = raw
	}
	typ, ok := msg.str("type")
	if !ok {
		b.sendError(id, nil, `missing or non-string "type"`)
		return
	}
	switch typ {
	case "set", "bind":
		b.handleSet(typ, id, msg)
	case "batch":
		b.handleBatch(id, msg)
	case "get":
		b.handleGet(id, msg)
	case "reply":
		b.handleReply(id, msg)
	default:
		b.sendError(id, nil, fmt.Sprintf("unknown type %q", typ))
	}
}

// sendError writes {"type":"error",…}, echoing a valid id or seq.
func (b *hostBridge) sendError(id, seq json.RawMessage, text string) {
	b.send(errorMsg{Type: "error", ID: id, Seq: seq, Error: text})
}

// ack answers a write (§19.1 "Acks"); a write without an id gets nothing
// on success and an "error" on failure ("Failures without id").
func (b *hostBridge) ack(id json.RawMessage, err error) {
	if id == nil {
		if err != nil {
			b.sendError(nil, nil, err.Error())
		}
		return
	}
	var text *string
	if err != nil {
		s := err.Error()
		text = &s
	}
	b.send(ackMsg{Type: "ack", ID: id, OK: err == nil, Error: text})
}

func (b *hostBridge) handleSet(typ string, id json.RawMessage, msg wireMsg) {
	path, ok := msg.str("path")
	if !ok {
		b.sendError(id, nil, fmt.Sprintf(`%q needs a string "path"`, typ))
		return
	}
	raw, ok := msg["value"]
	if !ok {
		b.sendError(id, nil, fmt.Sprintf(`%q needs "value"`, typ))
		return
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		b.sendError(id, nil, fmt.Sprintf(`bad "value": %v`, err))
		return
	}
	// bind is an alias of set; Set schedules the redraw.
	b.ack(id, b.app.Set(path, v))
}

func (b *hostBridge) handleGet(id json.RawMessage, msg wireMsg) {
	path, ok := msg.str("path")
	if id == nil || !ok {
		b.sendError(id, nil, `"get" needs "id" and a string "path"`)
		return
	}
	v, found := b.app.Get(path)
	out := getAckMsg{Type: "ack", ID: id, OK: true, Found: found}
	if found {
		raw, err := json.Marshal(v)
		if err != nil {
			text := err.Error()
			b.send(ackMsg{Type: "ack", ID: id, OK: false, Error: &text})
			return
		}
		out.Value = raw
	}
	b.send(out)
}

func (b *hostBridge) handleBatch(id json.RawMessage, msg wireMsg) {
	raw, ok := msg["writes"]
	if !ok || jsonKind(raw) != 'a' {
		b.sendError(id, nil, `"batch" needs a "writes" array`)
		return
	}
	var items []wireMsg
	if err := json.Unmarshal(raw, &items); err != nil {
		b.sendError(id, nil, `every "writes" entry must be an object with a string "path" and a "value"`)
		return
	}
	type write struct {
		path string
		v    any
	}
	writes := make([]write, len(items))
	for i, it := range items {
		path, ok := it.str("path")
		vraw, hasValue := it["value"]
		if it == nil || !ok || !hasValue {
			b.sendError(id, nil, fmt.Sprintf(`"writes"[%d] needs a string "path" and a "value"`, i))
			return
		}
		if err := json.Unmarshal(vraw, &writes[i].v); err != nil {
			b.sendError(id, nil, fmt.Sprintf(`bad "writes"[%d].value: %v`, i, err))
			return
		}
		writes[i].path = path
	}
	// All or nothing (§18.1): Batch applies nothing when any write fails,
	// and its error is the ack's text.
	err := b.app.Batch(func(bt *host.Batch) error {
		for _, w := range writes {
			_ = bt.Set(w.path, w.v)
		}
		return nil
	})
	b.ack(id, err)
}

// handleReply answers the waiting event (§19.1 "The reply"). seq is an
// integer judged exactly on its digits (jsonIntText, no float64): any
// integral form counts (1, 1.0, 1e0), and a fraction or a value outside
// int64 is the malformed-reply error. Every error about a reply echoes
// its id when it has one and its seq exactly as the parent wrote it, and
// a reply with an id that answers the waiting event is acked (§19.1
// "Acks": every message that carries an id is answered).
func (b *hostBridge) handleReply(id json.RawMessage, msg wireMsg) {
	raw, ok := msg["seq"]
	var seq int64
	if ok {
		text, isInt := jsonIntText(raw) // false for anything but a number
		n, err := strconv.ParseInt(text, 10, 64)
		ok, seq = isInt && err == nil, n
	}
	if !ok {
		b.sendError(id, nil, `"reply" needs an integer "seq"`)
		return
	}
	seqRaw := json.RawMessage(bytes.TrimSpace(raw))
	var rep hostReply
	if q, ok := msg["quit"]; ok {
		if jsonKind(q) != 'b' {
			b.sendError(id, seqRaw, `"quit" must be true or false`)
			return
		}
		_ = json.Unmarshal(q, &rep.quit)
	}
	if _, ok := msg["error"]; ok {
		s, ok := msg.str("error")
		if !ok {
			b.sendError(id, seqRaw, `"error" must be a string`)
			return
		}
		rep.err, rep.errMsg = true, s
	}
	// The reply is handed over under the lock, so a wait whose timer fires
	// at the same moment either finds it or has already made it late.
	b.mu.Lock()
	w := b.pending
	matched := w != nil && w.seq == seq
	if matched {
		b.pending = nil
		if id != nil {
			// Queued before the handler can resume, so the ack precedes
			// whatever the reply leads to (a quit reply's exit included).
			// send never blocks and never takes b.mu.
			b.send(ackMsg{Type: "ack", ID: id, OK: true})
		}
		w.reply <- rep // buffered: never blocks
	}
	late := b.timedOut[seq]
	answered := seq >= 1 && seq <= b.seq
	b.mu.Unlock()
	if !matched {
		why := "no event with this seq"
		switch {
		case late:
			why = "reply after the reply timeout"
		case answered:
			why = "event already answered"
		}
		b.sendError(id, seqRaw, why)
	}
}

// ── fd 4 ────────────────────────────────────────────────────────────────

// fd4Queue is the bounded send queue of §19.1 "Backpressure": one
// goroutine writes it to fd 4 in order. A send that finds it full, or a
// write that fails, calls onFail once. After a failed write nothing more
// is written; after an overflow the queued messages still go out, in
// order, if the parent starts reading again.
type fd4Queue struct {
	f        *os.File
	ch       chan []byte
	fin      chan []byte
	done     chan struct{}
	closed   atomic.Bool
	broken   atomic.Bool
	onFail   func(error)
	failOnce sync.Once
}

func newFD4Queue(f *os.File, size int, onFail func(error)) *fd4Queue {
	q := &fd4Queue{f: f, ch: make(chan []byte, size), fin: make(chan []byte, 1), done: make(chan struct{}), onFail: onFail}
	go q.drain()
	return q
}

func (q *fd4Queue) fail(err error) {
	q.failOnce.Do(func() { q.onFail(err) })
}

// send queues msg (one newline-terminated line). It never blocks: a full
// queue means the parent stopped reading.
func (q *fd4Queue) send(msg []byte) bool {
	if q.closed.Load() || q.broken.Load() {
		return false
	}
	select {
	case q.ch <- msg:
		return true
	default:
		q.fail(errors.New("parent is not reading fd 4"))
		return false
	}
}

func (q *fd4Queue) drain() {
	defer close(q.done)
	for {
		select {
		case msg := <-q.ch:
			q.write(msg)
		case final := <-q.fin:
			// Everything queued before the end goes first, so the final
			// message ("exit") is the last one.
		rest:
			for {
				select {
				case msg := <-q.ch:
					q.write(msg)
				default:
					break rest
				}
			}
			if final != nil {
				q.write(final)
			}
			return
		}
	}
}

func (q *fd4Queue) write(msg []byte) {
	if q.broken.Load() {
		return
	}
	if _, err := q.f.Write(msg); err != nil {
		q.broken.Store(true)
		q.fail(err)
	}
}

// finish stops accepting messages, writes what is queued and then final
// (when not nil), and gives up after d (§19.1 step 4): fd 4 is
// non-blocking (host.OpenHostFDs), so a write deadline ends a write the
// parent does not read.
func (q *fd4Queue) finish(final []byte, d time.Duration) {
	q.closed.Store(true)
	_ = q.f.SetWriteDeadline(time.Now().Add(d))
	q.fin <- final
	select {
	case <-q.done:
	case <-time.After(d + 100*time.Millisecond):
		// A descriptor the poller does not hold ignores the deadline; the
		// process exits anyway.
	}
}

// signalName and signalNumber give the "exit" message's signal name and
// the 128 + n exit code; SIGTERM, SIGHUP, and SIGINT are the only stop
// signals Run catches (§26.1).
func signalName(s os.Signal) string {
	if ss, ok := s.(syscall.Signal); ok {
		switch ss {
		case syscall.SIGTERM:
			return "SIGTERM"
		case syscall.SIGHUP:
			return "SIGHUP"
		case syscall.SIGINT:
			return "SIGINT"
		}
	}
	return fmt.Sprint(s)
}

func signalNumber(s os.Signal) int {
	if ss, ok := s.(syscall.Signal); ok {
		return int(ss)
	}
	return 0
}
