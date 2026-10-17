package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// message is any server-to-client message.
type message struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

// client drives a Server over pipes with Content-Length framing.
type client struct {
	t      *testing.T
	srv    *Server
	in     *io.PipeWriter
	msgs   chan message
	exit   chan int
	stderr *bytes.Buffer
	nextID int
	notes  []message // notifications read while waiting for something else
}

// startServer runs a server with the given options (the default debounce
// when zero) and returns its client.
func startServer(t *testing.T, opts Options) *client {
	return startWith(t, newServer(opts))
}

func startWith(t *testing.T, srv *Server) *client {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	c := &client{t: t, srv: srv, in: inW, msgs: make(chan message, 1024), exit: make(chan int, 1), stderr: &bytes.Buffer{}}
	go func() {
		c.exit <- srv.run(inR, outW, c.stderr)
		outW.Close()
	}()
	go func() {
		br := bufio.NewReader(outR)
		for {
			body, err := readMessage(br)
			if err != nil {
				close(c.msgs)
				return
			}
			var m message
			if err := json.Unmarshal(body, &m); err != nil {
				t.Errorf("server sent invalid JSON %q: %v", body, err)
				continue
			}
			c.msgs <- m
		}
	}()
	t.Cleanup(func() { inW.Close() })
	return c
}

func (c *client) sendRaw(b []byte) {
	c.t.Helper()
	if _, err := c.in.Write(b); err != nil {
		c.t.Fatalf("write to server: %v", err)
	}
}

func (c *client) send(v any) {
	c.t.Helper()
	body, err := json.Marshal(v)
	if err != nil {
		c.t.Fatal(err)
	}
	c.sendRaw(frame(body))
}

func (c *client) notify(method string, params any) {
	c.t.Helper()
	c.send(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

// call sends a request and returns its response, keeping every
// notification that arrives first.
func (c *client) call(method string, params any) message {
	c.t.Helper()
	c.nextID++
	id := c.nextID
	c.send(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	return c.awaitResponse(fmt.Sprint(id))
}

func (c *client) awaitResponse(id string) message {
	c.t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		select {
		case m, ok := <-c.msgs:
			if !ok {
				c.t.Fatalf("server closed the output waiting for response %s", id)
			}
			if m.Method != "" {
				c.notes = append(c.notes, m)
				continue
			}
			if string(m.ID) == id {
				return m
			}
		case <-deadline:
			c.t.Fatalf("no response %s", id)
		}
	}
}

func (c *client) initialize() {
	c.t.Helper()
	r := c.call("initialize", map[string]any{"capabilities": map[string]any{}})
	if r.Error != nil {
		c.t.Fatalf("initialize: %+v", r.Error)
	}
	c.notify("initialized", map[string]any{})
}

func (c *client) open(path, languageID, text string) string {
	c.t.Helper()
	uri := pathToURI(path)
	c.notify("textDocument/didOpen", map[string]any{"textDocument": map[string]any{
		"uri": uri, "languageId": languageID, "version": 1, "text": text,
	}})
	return uri
}

func (c *client) change(uri string, version int, text string) {
	c.t.Helper()
	c.notify("textDocument/didChange", map[string]any{
		"textDocument":   map[string]any{"uri": uri, "version": version},
		"contentChanges": []map[string]any{{"text": text}},
	})
}

// published is a decoded publishDiagnostics.
type published struct {
	URI         string          `json:"uri"`
	Version     *int            `json:"version"`
	Diagnostics []lspDiagnostic `json:"diagnostics"`
}

func decodePublished(t *testing.T, m message) published {
	t.Helper()
	var p published
	if err := json.Unmarshal(m.Params, &p); err != nil {
		t.Fatal(err)
	}
	if p.Diagnostics == nil {
		t.Fatalf("publishDiagnostics without a diagnostics array: %s", m.Params)
	}
	return p
}

// next returns the next publishDiagnostics for uri, from the notifications
// already read or the ones that arrive within a few seconds.
func (c *client) nextPublish(uri string) published {
	c.t.Helper()
	for i, m := range c.notes {
		if m.Method == "textDocument/publishDiagnostics" {
			if p := decodePublished(c.t, m); p.URI == uri {
				c.notes = append(c.notes[:i], c.notes[i+1:]...)
				return p
			}
		}
	}
	deadline := time.After(10 * time.Second)
	for {
		select {
		case m, ok := <-c.msgs:
			if !ok {
				c.t.Fatalf("server closed the output waiting for a publish on %s", uri)
			}
			if m.Method == "textDocument/publishDiagnostics" {
				if p := decodePublished(c.t, m); p.URI == uri {
					return p
				}
			}
			c.notes = append(c.notes, m)
		case <-deadline:
			c.t.Fatalf("no publishDiagnostics for %s", uri)
		}
	}
}

// noPublish fails when a publishDiagnostics for uri is read within d.
func (c *client) noPublish(uri string, d time.Duration) {
	c.t.Helper()
	for _, m := range c.notes {
		if m.Method == "textDocument/publishDiagnostics" && decodePublished(c.t, m).URI == uri {
			c.t.Fatalf("unexpected publishDiagnostics for %s: %s", uri, m.Params)
		}
	}
	deadline := time.After(d)
	for {
		select {
		case m, ok := <-c.msgs:
			if !ok {
				return
			}
			if m.Method == "textDocument/publishDiagnostics" && decodePublished(c.t, m).URI == uri {
				c.t.Fatalf("unexpected publishDiagnostics for %s: %s", uri, m.Params)
			}
			c.notes = append(c.notes, m)
		case <-deadline:
			return
		}
	}
}

// shutdown sends shutdown and exit, and returns the exit code.
func (c *client) shutdown() int {
	c.t.Helper()
	if r := c.call("shutdown", nil); r.Error != nil {
		c.t.Fatalf("shutdown: %+v", r.Error)
	}
	c.notify("exit", nil)
	return c.waitExit()
}

func (c *client) waitExit() int {
	c.t.Helper()
	select {
	case code := <-c.exit:
		return code
	case <-time.After(10 * time.Second):
		c.t.Fatal("server did not exit")
		return -1
	}
}

// writeTree writes files (path relative to dir -> text) and returns dir.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for name, text := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// fakeClock is a manual clock for the debounce: afterFunc queues a
// callback, fire runs everything queued.
type fakeClock struct {
	mu      sync.Mutex
	queued  []*fakeTimer
	delays  []time.Duration
	started int
}

type fakeTimer struct {
	f       func()
	stopped atomic.Bool
}

func (t *fakeTimer) Stop() bool { t.stopped.Store(true); return true }

func (fc *fakeClock) afterFunc(d time.Duration, f func()) timer {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	t := &fakeTimer{f: f}
	fc.queued = append(fc.queued, t)
	fc.delays = append(fc.delays, d)
	fc.started++
	return t
}

// fire runs every queued callback, once, and reports how many ran.
func (fc *fakeClock) fire() int {
	fc.mu.Lock()
	q := fc.queued
	fc.queued = nil
	fc.mu.Unlock()
	n := 0
	for _, t := range q {
		if !t.stopped.Load() {
			t.f()
			n++
		}
	}
	return n
}

func (fc *fakeClock) pending() int {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	return len(fc.queued)
}
