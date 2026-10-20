// Package lsp is `tuimark lsp` (SPEC v0.4a §15.12): a Language Server
// Protocol 3.17 server over stdin/stdout for .tui and .tcss files. Like
// `tuimark mcp` it uses only the standard library and reads no
// environment variable. It is stateful where mcp is not: it keeps the
// editor's open buffers, checks them with the same code `validate` runs
// (host.LoadBuffers is Load over those buffers), and answers completion,
// hover, definition, and formatting from the catalog (internal/catalog)
// and the documents.
package lsp

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/abdul-hamid-achik/tuimark/internal/catalog"
	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// DefaultDebounce is the interval at most one publishDiagnostics is sent
// per document in (SPEC §15.12 "Diagnostics").
const DefaultDebounce = 150 * time.Millisecond

// Options configure Serve.
type Options struct {
	// Version is the tuimark version, reported in serverInfo and the
	// catalog's tuimark member.
	Version string
	// Debounce overrides DefaultDebounce (a test's, never the CLI's).
	Debounce time.Duration
}

// timer is the part of *time.Timer the server uses, so a test can drive
// the debounce with a fake clock.
type timer interface{ Stop() bool }

// Server is one language server session.
type Server struct {
	version   string
	debounce  time.Duration
	afterFunc func(time.Duration, func()) timer
	cat       catalog.Catalog
	// analyze is the check a publish runs (analyze); a test replaces it
	// with a slow one.
	analyze func(sn *snapshot, path, text string) *analysis

	// outMu serializes writes: a response from the read loop and a
	// diagnostics notification from a debounce timer.
	outMu    sync.Mutex
	out      io.Writer
	writeErr error
	closed   bool

	// mu guards the state below, which the read loop and the timers share.
	mu        sync.Mutex
	docs      map[string]*document
	gen       int // bumped by every change of what an analysis can read
	pending   map[string]timer
	published map[string]bool
	lastDiags map[string][]lspDiagnostic
	cache     map[string]cachedAnalysis
	// expanded caches parseLoader by path for the generation of the
	// snapshot, as cache does analyze: whether a .tui reads a fragment is
	// found by expanding its includes, which is much cheaper than checking
	// it.
	expanded map[string]cachedParse
	// uriLocks serialize the publishes of one URI: a publish takes its
	// snapshot inside its URI's lock, so they run in the order of the
	// state they read. lastGen is the generation of the snapshot a URI was
	// last published from; reqGen is the generation current at the latest
	// request to publish it (schedule), so a publish that finds a newer
	// one asked for knows its result is stale and leaves it to that one.
	uriLocks map[string]*sync.Mutex
	lastGen  map[string]int
	reqGen   map[string]int

	// Protocol state, touched only by the read loop.
	initialized bool
	shutdown    bool
}

type cachedAnalysis struct {
	gen  int
	text string
	a    *analysis
}

type cachedParse struct {
	gen  int
	text string
	pd   parsedDoc
}

// Serve reads LSP messages from in and writes them to out until `exit`
// (0 after a `shutdown`, else 1), the end of in (0 after a `shutdown`, else
// 1), a malformed header, or a failed write (1, with a "tuimark: " line on
// errOut).
func Serve(in io.Reader, out, errOut io.Writer, opts Options) int {
	return newServer(opts).run(in, out, errOut)
}

func newServer(opts Options) *Server {
	d := opts.Debounce
	if d <= 0 {
		d = DefaultDebounce
	}
	return &Server{
		version:  opts.Version,
		debounce: d,
		afterFunc: func(d time.Duration, f func()) timer {
			return time.AfterFunc(d, f)
		},
		cat:       catalog.Build(opts.Version),
		docs:      map[string]*document{},
		pending:   map[string]timer{},
		published: map[string]bool{},
		lastDiags: map[string][]lspDiagnostic{},
		cache:     map[string]cachedAnalysis{},
		expanded:  map[string]cachedParse{},
		analyze:   analyze,
		uriLocks:  map[string]*sync.Mutex{},
		lastGen:   map[string]int{},
		reqGen:    map[string]int{},
	}
}

func (s *Server) run(in io.Reader, out, errOut io.Writer) int {
	s.out = out
	br := bufio.NewReaderSize(in, 64*1024)
	defer s.stop()
	for {
		body, err := readMessage(br)
		switch {
		case errors.Is(err, io.EOF):
			return s.endCode()
		case err != nil:
			fmt.Fprintln(errOut, "tuimark:", err)
			return 1
		}
		code, done := s.handle(body)
		if err := s.failure(); err != nil {
			fmt.Fprintln(errOut, "tuimark:", err)
			return 1
		}
		if done {
			return code
		}
	}
}

// endCode is the exit code of `exit` and of the end of input: 0 after a
// `shutdown`, else 1.
func (s *Server) endCode() int {
	if s.shutdown {
		return 0
	}
	return 1
}

// stop cancels the debounce timers and closes the output to them.
func (s *Server) stop() {
	s.outMu.Lock()
	s.closed = true
	s.outMu.Unlock()
	s.mu.Lock()
	for uri, t := range s.pending {
		t.Stop()
		delete(s.pending, uri)
	}
	s.mu.Unlock()
}

func (s *Server) failure() error {
	s.outMu.Lock()
	defer s.outMu.Unlock()
	return s.writeErr
}

// send writes one framed message. After the first failed write, and after
// stop, it writes nothing.
func (s *Server) send(body []byte) {
	s.outMu.Lock()
	defer s.outMu.Unlock()
	if s.closed || s.writeErr != nil {
		return
	}
	if _, err := s.out.Write(frame(body)); err != nil {
		s.writeErr = err
	}
}

func (s *Server) reply(id json.RawMessage, result any, e *rpcError) {
	body, err := marshalResponse(id, result, e)
	if err != nil {
		body, _ = marshalResponse(id, nil, &rpcError{Code: codeInternal, Message: err.Error()})
	}
	s.send(body)
}

func (s *Server) notify(method string, params any) {
	body, err := json.Marshal(struct {
		JSONRPC string `json:"jsonrpc"`
		Method  string `json:"method"`
		Params  any    `json:"params"`
	}{"2.0", method, params})
	if err == nil {
		s.send(body)
	}
}

// handle decodes and answers one message. done is true once the server is
// to exit, with the exit code.
//
//   - not JSON at all: -32700, id null, and the server goes on.
//   - JSON that is not a request or notification (no "jsonrpc": "2.0", no
//     string "method"): -32600 with its id when it has one, else null; a
//     response from the client (an "id" with "result" or "error" and no
//     "method") is ignored.
//   - a request with no "id" is a notification: no reply.
func (s *Server) handle(body []byte) (code int, done bool) {
	null := json.RawMessage("null")
	// As in mcp, every number stays raw text, so an "id" is echoed exactly
	// as written, whatever float64 would make of it.
	if !json.Valid(body) {
		s.reply(null, nil, &rpcError{Code: codeParse, Message: "parse error"})
		return 0, false
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(body, &members); err != nil {
		s.reply(null, nil, &rpcError{Code: codeInvalidRequest, Message: "invalid request"})
		return 0, false
	}
	id, hasID := members["id"]
	if !hasID {
		id = null
	}
	var jsonrpc, method string
	okRPC := json.Unmarshal(members["jsonrpc"], &jsonrpc) == nil && jsonrpc == "2.0"
	okMethod := json.Unmarshal(members["method"], &method) == nil
	if !okRPC || !okMethod {
		_, isResult := members["result"]
		_, isError := members["error"]
		if hasID && okRPC && (isResult || isError) && members["method"] == nil {
			return 0, false
		}
		s.reply(id, nil, &rpcError{Code: codeInvalidRequest, Message: "invalid request"})
		return 0, false
	}
	params := members["params"]
	if !hasID {
		return s.notification(method, params)
	}
	switch {
	case method == "initialize":
		if s.initialized {
			s.reply(id, nil, &rpcError{Code: codeInvalidRequest, Message: "already initialized"})
			return 0, false
		}
		s.initialized = true
		s.reply(id, s.initializeResult(), nil)
	case !s.initialized:
		s.reply(id, nil, &rpcError{Code: codeNotInitialized, Message: "server not initialized"})
	case s.shutdown:
		s.reply(id, nil, &rpcError{Code: codeInvalidRequest, Message: "server is shut down"})
	case method == "shutdown":
		s.shutdown = true
		s.reply(id, nil, nil)
	default:
		result, e := s.safeRequest(method, params)
		s.reply(id, result, e)
	}
	return 0, false
}

// notification handles a message with no id. Only `exit` ends the server;
// before `initialize` every other one is dropped.
func (s *Server) notification(method string, params json.RawMessage) (int, bool) {
	if method == "exit" {
		return s.endCode(), true
	}
	if !s.initialized || s.shutdown {
		return 0, false
	}
	switch method {
	case "textDocument/didOpen":
		var p struct {
			TextDocument struct {
				URI        string `json:"uri"`
				LanguageID string `json:"languageId"`
				Version    int    `json:"version"`
				Text       string `json:"text"`
			} `json:"textDocument"`
		}
		if json.Unmarshal(params, &p) == nil && p.TextDocument.URI != "" {
			s.didOpen(p.TextDocument.URI, p.TextDocument.LanguageID, p.TextDocument.Text, p.TextDocument.Version)
		}
	case "textDocument/didChange":
		var p struct {
			TextDocument struct {
				URI     string `json:"uri"`
				Version int    `json:"version"`
			} `json:"textDocument"`
			ContentChanges []struct {
				Text string `json:"text"`
			} `json:"contentChanges"`
		}
		if json.Unmarshal(params, &p) == nil && len(p.ContentChanges) > 0 {
			s.didChange(p.TextDocument.URI, p.ContentChanges[len(p.ContentChanges)-1].Text, p.TextDocument.Version)
		}
	case "textDocument/didClose":
		var p struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
		}
		if json.Unmarshal(params, &p) == nil {
			s.didClose(p.TextDocument.URI)
		}
	case "textDocument/didSave":
		// A file on disk may have changed (a stylesheet nobody has open, a
		// sample.json), which no buffer notification says.
		s.touchAll(nil)
	case "workspace/didChangeWatchedFiles":
		var p struct {
			Changes []struct {
				URI string `json:"uri"`
			} `json:"changes"`
		}
		var named []string
		if json.Unmarshal(params, &p) == nil {
			for _, c := range p.Changes {
				named = append(named, c.URI)
			}
		}
		s.touchAll(named)
	}
	return 0, false
}

// safeRequest is request, with a panic in a feature answered as an internal
// error: a bug in one completion must not end the editor's session.
func (s *Server) safeRequest(method string, params json.RawMessage) (result any, e *rpcError) {
	defer func() {
		if r := recover(); r != nil {
			result, e = nil, &rpcError{Code: codeInternal, Message: fmt.Sprint("internal error: ", r)}
		}
	}()
	return s.request(method, params)
}

type textDocumentPosition struct {
	TextDocument struct {
		URI string `json:"uri"`
	} `json:"textDocument"`
	Position Position `json:"position"`
}

// request answers a request after `initialize`.
func (s *Server) request(method string, params json.RawMessage) (any, *rpcError) {
	var p textDocumentPosition
	switch method {
	case "textDocument/completion", "textDocument/hover", "textDocument/definition", "textDocument/formatting":
		if err := json.Unmarshal(params, &p); err != nil || p.TextDocument.URI == "" {
			return nil, &rpcError{Code: codeInvalidParams, Message: "invalid params"}
		}
	default:
		return nil, &rpcError{Code: codeMethodNotFound, Message: "method not found: " + method}
	}
	sn := s.snapshot()
	doc := sn.byURI[p.TextDocument.URI]
	if doc == nil {
		if method == "textDocument/formatting" {
			return []textEdit{}, nil
		}
		return nil, nil
	}
	switch method {
	case "textDocument/completion":
		return s.complete(sn, doc, p.Position), nil
	case "textDocument/hover":
		return s.hover(sn, doc, p.Position), nil
	case "textDocument/definition":
		return s.definition(sn, doc, p.Position), nil
	}
	return formatDocument(doc), nil
}

// completionTriggers are the characters that open a completion: the start
// of a tag, an attribute, or a value, and the sigils of #id, .class,
// :pseudo, $token, {path}, and a key list.
var completionTriggers = []string{"<", " ", "\"", "=", "#", ".", ":", "$", "{", ",", "@"}

func (s *Server) initializeResult() map[string]any {
	return map[string]any{
		"capabilities": map[string]any{
			"textDocumentSync": map[string]any{
				"openClose": true,
				"change":    1, // TextDocumentSyncKind.Full
				"save":      map[string]any{"includeText": false},
			},
			"completionProvider":         map[string]any{"triggerCharacters": completionTriggers},
			"hoverProvider":              true,
			"definitionProvider":         true,
			"documentFormattingProvider": true,
		},
		"serverInfo": map[string]any{"name": "tuimark", "version": s.version},
	}
}

// --- documents and diagnostics --------------------------------------------

func (s *Server) didOpen(uri, languageID, text string, version int) {
	path, _ := uriToPath(uri)
	d := &document{uri: uri, path: path, kind: kindOf(languageID, uriName(uri, path)), text: text, version: version}
	s.mu.Lock()
	s.docs[uri] = d
	s.gen++
	s.mu.Unlock()
	s.touch(d)
}

func (s *Server) didChange(uri, text string, version int) {
	s.mu.Lock()
	d := s.docs[uri]
	if d == nil || version < d.version {
		s.mu.Unlock()
		return
	}
	d.text, d.version = text, version
	s.gen++
	s.mu.Unlock()
	s.touch(d)
}

func (s *Server) didClose(uri string) {
	s.mu.Lock()
	d := s.docs[uri]
	if d == nil {
		s.mu.Unlock()
		return
	}
	delete(s.docs, uri)
	delete(s.cache, d.path) // a closed .tui's analysis, with its decoded sample.json, is not needed again
	delete(s.expanded, d.path)
	s.gen++
	s.mu.Unlock()
	s.touch(d)
}

// uriName is what kindOf looks at for a document: the path, or the URI of
// a buffer that is not a file (an untitled: one has none).
func uriName(uri, path string) string {
	if path != "" {
		return path
	}
	return uri
}

// touch schedules a publish for every document a change to d moves: d
// itself; for a .tui, the files its last publish noted (noteLoads: the
// stylesheets and fragments it loads, or, for a fragment, the other files
// the documents that read it read) and the open .tui files whose noted
// files name it (the documents that read a fragment, and the fragments
// read beside it); for a stylesheet, the open .tui files that load it;
// for a sample.json, the open .tui files beside it.
func (s *Server) touch(d *document) {
	s.mu.Lock()
	uris := []string{d.uri}
	switch d.kind {
	case kindTUI:
		for _, p := range d.loads {
			uris = append(uris, s.uriFor(p))
		}
		if d.path != "" {
			for _, o := range s.docs {
				if o.kind == kindTUI && o.uri != d.uri && contains(o.loads, d.path) {
					uris = append(uris, o.uri)
				}
			}
		}
	case kindTCSS:
		for _, o := range s.docs {
			if o.kind == kindTUI && contains(o.loads, d.path) {
				uris = append(uris, o.uri)
			}
		}
	default:
		if d.path != "" && filepath.Base(d.path) == sampleName {
			for _, o := range s.docs {
				if o.kind == kindTUI && filepath.Dir(o.path) == filepath.Dir(d.path) {
					uris = append(uris, o.uri)
				}
			}
		}
	}
	s.mu.Unlock()
	for _, u := range uris {
		s.schedule(u)
	}
}

// touchAll is touch for every open .tui and .tcss, after a change on disk
// no buffer reports, and for the stylesheets the open .tui files load (one
// nobody has open is only on disk: a save or a checkout changes it without
// a notification of its own). named are the URIs a
// workspace/didChangeWatchedFiles reports: each one that is open or was
// published before is scheduled too.
func (s *Server) touchAll(named []string) {
	s.mu.Lock()
	s.gen++
	var uris []string
	for _, d := range s.docs {
		if d.kind == kindOther {
			continue
		}
		uris = append(uris, d.uri)
		if d.kind == kindTUI {
			for _, p := range d.loads {
				uris = append(uris, s.uriFor(p))
			}
		}
	}
	for _, u := range named {
		if s.docs[u] != nil || s.published[u] {
			uris = append(uris, u)
		}
	}
	s.mu.Unlock()
	for _, u := range uris {
		s.schedule(u)
	}
}

// uriFor is the URI to publish a file's diagnostics on: the open buffer's
// own when it is open, else a file: URI. The caller holds s.mu.
func (s *Server) uriFor(path string) string {
	for _, d := range s.docs {
		if d.path == path {
			return d.uri
		}
	}
	return pathToURI(path)
}

// schedule arranges one publish of uri, debounce from now. A request while
// one is pending is covered by it: the publish reads the buffers when it
// fires, so a burst of changes sends one, and a document is never sent
// diagnostics more than once per interval.
func (s *Server) schedule(uri string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reqGen[uri] = s.gen
	if _, ok := s.pending[uri]; ok {
		return
	}
	s.pending[uri] = s.afterFunc(s.debounce, func() { s.fire(uri) })
}

func (s *Server) fire(uri string) {
	s.mu.Lock()
	delete(s.pending, uri)
	s.mu.Unlock()
	defer func() { _ = recover() }() // a failed check leaves the last diagnostics as they were
	s.publish(uri)
}

// snapshot copies the open buffers.
func (s *Server) snapshot() *snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	sn := &snapshot{gen: s.gen, byURI: map[string]*document{}, open: map[string]string{}}
	for uri, d := range s.docs {
		c := *d
		sn.byURI[uri] = &c
		if d.path != "" {
			sn.open[d.path] = d.text
		}
	}
	return sn
}

// analysisOf is analyze, cached for the generation of the snapshot: a
// burst of publishes after one change analyzes each .tui once.
func (s *Server) analysisOf(sn *snapshot, path, text string) *analysis {
	s.mu.Lock()
	c, ok := s.cache[path]
	s.mu.Unlock()
	if ok && c.gen == sn.gen && c.text == text {
		return c.a
	}
	a := s.analyze(sn, path, text)
	s.mu.Lock()
	// An entry of an older generation can never be hit again, so it goes;
	// and a slow analysis does not overwrite a newer one.
	for p, e := range s.cache {
		if e.gen < sn.gen {
			delete(s.cache, p)
		}
	}
	if e, ok := s.cache[path]; !ok || e.gen <= sn.gen {
		s.cache[path] = cachedAnalysis{gen: sn.gen, text: text, a: a}
	}
	s.mu.Unlock()
	return a
}

// expandedOf is parseLoader, cached for the generation of the snapshot
// like analysisOf: the includes and uses expanded, nothing checked.
func (s *Server) expandedOf(sn *snapshot, path, text string) parsedDoc {
	s.mu.Lock()
	c, ok := s.expanded[path]
	s.mu.Unlock()
	if ok && c.gen == sn.gen && c.text == text {
		return c.pd
	}
	pd := parseLoader(sn, path, text)
	s.mu.Lock()
	for p, e := range s.expanded {
		if e.gen < sn.gen {
			delete(s.expanded, p)
		}
	}
	if e, ok := s.expanded[path]; !ok || e.gen <= sn.gen {
		s.expanded[path] = cachedParse{gen: sn.gen, text: text, pd: pd}
	}
	s.mu.Unlock()
	return pd
}

// uriLock is the lock that serializes the publishes of uri.
func (s *Server) uriLock(uri string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.uriLocks[uri]
	if m == nil {
		m = &sync.Mutex{}
		s.uriLocks[uri] = m
	}
	return m
}

// publish sends the diagnostics of one URI (SPEC §15.12 "Diagnostics"): a
// .tui's own, or a stylesheet's from every .tui that loads it. A closed
// document, and a stylesheet no .tui loads any more, gets an empty list.
//
// The publishes of one URI run one at a time, each from a snapshot taken
// once it holds the URI's lock, so a slow analysis cannot finish after a
// newer one and replace its diagnostics with older ones. One that finds
// a newer request for its URI (a change after its snapshot) sends nothing:
// that request's publish is coming and says it all. One whose snapshot is
// no newer than the URI's last publish has nothing to add and sends
// nothing either, which is what keeps a .tui and its stylesheet opened
// together from publishing the stylesheet twice.
func (s *Server) publish(uri string) {
	lock := s.uriLock(uri)
	lock.Lock()
	defer lock.Unlock()
	sn := s.snapshot()
	s.mu.Lock()
	last, was := s.lastGen[uri]
	s.mu.Unlock()
	if was && sn.gen <= last {
		return // nothing an analysis reads has changed since this URI was published
	}
	var diags []lspDiagnostic
	// loads are the files an open .tui's diagnostics depend on, noted
	// after its publish (noteLoads).
	var loads []string
	noted := false
	path, isFile := uriToPath(uri)
	doc := sn.byURI[uri]
	kind := kindOf("", uriName(uri, path))
	if doc != nil {
		kind = doc.kind
	}
	switch kind {
	case kindTUI:
		if doc != nil {
			a := s.analysisOf(sn, doc.path, doc.text)
			if a.fragment && doc.path != "" {
				diags, loads = s.fragmentDiags(sn, doc.path, doc.text, a)
			} else {
				diags, loads = a.route("", true), a.reads()
			}
			noted = true
		} else if isFile {
			// A closed fragment gets what the documents that read it say
			// about it, as a closed stylesheet does; any other closed .tui
			// gets an empty list.
			if text, ok := readText(sn, path); ok && parse.Parse([]byte(text), path).FragmentRoot {
				diags, _ = s.fragmentDiags(sn, path, text, nil)
			}
		}
	case kindTCSS:
		if isFile {
			for _, a := range s.loaders(sn, path) {
				diags = append(diags, a.route(path, false)...)
			}
		}
	default:
		// A closed buffer has no extension to say what it was: whatever it
		// was sent is cleared with an empty list.
		s.mu.Lock()
		was := s.published[uri]
		s.mu.Unlock()
		if !was {
			return
		}
	}
	diags = dedupe(diags)
	if diags == nil {
		diags = []lspDiagnostic{}
	}
	p := map[string]any{"uri": uri, "diagnostics": diags}
	if doc != nil {
		p["version"] = doc.version
	}
	s.mu.Lock()
	if sn.gen < s.reqGen[uri] {
		s.mu.Unlock()
		return
	}
	s.published[uri] = true
	s.lastDiags[uri] = diags
	s.lastGen[uri] = sn.gen
	s.mu.Unlock()
	s.notify("textDocument/publishDiagnostics", p)
	if noted {
		s.noteLoads(doc, loads, sn.gen)
	}
}

// noteLoads records loads, the files whose diagnostics the open .tui doc
// moves, as its publish from the state of generation gen found them: the
// stylesheets and fragments a document reads; for a fragment, the other
// files the documents that read it read (its siblings, the fragments it
// includes, and their stylesheets). Each of them that has not been
// published since may have diagnostics this document now contributes (a
// first publish, or a <style src> or <include src> gained), and so may
// each file the previous loads named and these do not (one dropped, here
// or in a file it reads) that was published before: those are scheduled
// like any change, inside the debounce window, and a window already open
// for one (a .tui and its stylesheet opened together) covers it. One
// published from a generation at least as new has already read this
// state, and is left alone.
func (s *Server) noteLoads(doc *document, loads []string, gen int) {
	s.mu.Lock()
	var old []string
	if d := s.docs[doc.uri]; d != nil {
		old, d.loads = d.loads, loads
	}
	var due []string
	seen := map[string]bool{}
	add := func(p string, dropped bool) {
		u := s.uriFor(p)
		if seen[u] {
			return
		}
		seen[u] = true
		if s.published[u] && s.lastGen[u] >= gen || dropped && !s.published[u] {
			return
		}
		due = append(due, u)
	}
	for _, p := range loads {
		add(p, false)
	}
	for _, p := range old {
		add(p, true)
	}
	s.mu.Unlock()
	for _, u := range due {
		s.schedule(u)
	}
}

// loaders are the analyses of every .tui that loads the stylesheet at
// sheet: each open one, and each other .tui file in its directory; and,
// for a fragment among them that names it, the documents that read the
// fragment, wherever they are (a <style src> written in a fragment is the
// including document's, SPEC v0.4b §6.18.2).
func (s *Server) loaders(sn *snapshot, sheet string) []*analysis {
	var out []*analysis
	seen := map[string]bool{}
	for _, pd := range parsedLoaders(sn, sheet) {
		seen[pd.path] = true
		out = append(out, s.analysisOf(sn, pd.path, pd.text))
	}
	for _, p := range tuiCandidates(sn, filepath.Dir(sheet)) {
		text, ok := readText(sn, p)
		if !ok || !contains(styleSrcsOf(p, text), sheet) || !parse.Parse([]byte(text), p).FragmentRoot {
			continue
		}
		for _, a := range s.includers(sn, p) {
			if !seen[a.path] && contains(a.loadPaths, sheet) {
				seen[a.path] = true
				out = append(out, a)
			}
		}
	}
	return out
}

// tuiCandidates are the paths of the open .tui files, sorted, then of the
// other .tui files in each of dirs, each once.
func tuiCandidates(sn *snapshot, dirs ...string) []string {
	var out []string
	seen := map[string]bool{}
	for _, d := range sn.byURI {
		if d.kind == kindTUI && d.path != "" && !seen[d.path] {
			seen[d.path] = true
			out = append(out, d.path)
		}
	}
	sort.Strings(out)
	for _, dd := range dirs {
		entries, _ := os.ReadDir(dd)
		for _, e := range entries {
			if e.IsDir() || kindOf("", e.Name()) != kindTUI {
				continue
			}
			if p := filepath.Join(dd, e.Name()); !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	return out
}

// includers are the analyses of the documents that read the fragment at
// frag, at any depth (SPEC v0.4b §15.12): each open .tui, then each .tui
// in the fragment's directory and in its parent directory, that is not a
// fragment itself and whose includes, expanded, name frag. Only those are
// checked (an analysis lays a document out at three widths): a text whose
// <include src> values never lead to frag (mayRead) costs a scan of the
// texts, and one whose do, an expansion of its includes (expandedOf), so
// an edit of a fragment beside many documents that read other fragments
// stays cheap.
func (s *Server) includers(sn *snapshot, frag string) []*analysis {
	var out []*analysis
	dir := filepath.Dir(frag)
	for _, p := range tuiCandidates(sn, dir, filepath.Dir(dir)) {
		if p == frag {
			continue
		}
		text, ok := readText(sn, p)
		if !ok || !mayRead(sn, p, text, frag, map[string]bool{p: true}) {
			continue
		}
		if pd := s.expandedOf(sn, p, text); !pd.expanded || !contains(fragmentPaths(pd.doc), frag) {
			continue
		}
		if a := s.analysisOf(sn, p, text); !a.fragment && contains(a.fragPaths, frag) {
			out = append(out, a)
		}
	}
	return out
}

// fragmentDiags are the diagnostics of the fragment at frag, whose text
// is text (SPEC v0.4b §15.12): those whose file is the fragment, from
// every document that reads it; or, when none does, its own check (own,
// which a closed fragment does not have). loads are the other files those
// documents read (noteLoads): an edit of the fragment can move their
// diagnostics, and an edit of its includes changes which they are.
func (s *Server) fragmentDiags(sn *snapshot, frag, text string, own *analysis) (diags []lspDiagnostic, loads []string) {
	inc := s.includers(sn, frag)
	if len(inc) == 0 {
		if own == nil {
			return nil, nil
		}
		return own.route("", true), nil
	}
	lines := splitLines(text)
	for _, a := range inc {
		diags = append(diags, a.routeFile(frag, lines)...)
		for _, p := range a.reads() {
			if p != frag {
				loads = append(loads, p)
			}
		}
	}
	return diags, uniqueSorted(loads)
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
