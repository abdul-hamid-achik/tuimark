// Package playground is the engine behind the website's browser
// playground (ADR 0033): sessions of the Tuimark runtime over an
// in-memory file system, driven through JSON calls. cmd/tuimark-wasm is
// its only caller, a thin syscall/js shim; the package itself has no
// build tag, so the CLI-parity tests run natively.
//
// It is private to the website: the method names, the JSON shapes, and
// this package have no compatibility promise. It uses the public
// tuimark package only, so a frame it returns is the one the CLI would
// print for the same document, data, size, and theme (the parity test
// compares them byte for byte).
//
// # Methods
//
// Call runs one method. Arguments are JSON texts; the result is a JSON
// text, an object that holds an "error" member when the call failed and
// never otherwise. A panic inside a call is recovered into the same
// error shape (and drops the session it was on).
//
//	open(files, opts?)             -> {session, dump, events, quit, mockErrors?}
//	render(session, opts?)         -> {dump, events, quit, mockErrors?}
//	step(session, steps, opts?)    -> {dump, events, quit, mockErrors?, stepError?}
//	validate(session)              -> {ok, file, data, diagnostics, actions}
//	reload(session, files, opts?)  -> like render
//	close(session)                 -> {}
//
// files is an object of slash-separated path to file content (the
// .tui and .tcss files, sample.json, an optional mock.json), so a
// <style src="../theme.tcss"/> resolves as on disk. opts holds entry (the
// document's path; default app.tui, or the only .tui file), data (the
// JSON file bound at the root; default sample.json or NAME.sample.json
// beside the entry), mock (a tuimark run --mock file; default mock.json
// beside the entry), theme (dark or light), and cols and rows (default
// 80x24, then the size in effect). steps is one string or an array of
// strings, each exactly one step of tuimark play's --input grammar.
//
// dump is tuimark dump --format json --styles --cells for the document
// at the session's size. render and step are Play calls, so a call also
// settles the lifecycle events a previous call left queued (that is what
// "live" means: dump alone is a side-effect-free snapshot, but the public
// API gives no snapshot with styles and cells).
package playground

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/abdul-hamid-achik/tuimark"
)

const (
	// MaxPaste is the largest paste: step payload in bytes; pasting is
	// slow under wasm, so a longer one is refused.
	MaxPaste = 16 << 10

	maxSessions   = 32
	maxFiles      = 512
	maxTotalBytes = 8 << 20

	defaultCols, defaultRows = 80, 24
)

// Options are the opts object of the methods; each method reads the
// members that apply to it.
type Options struct {
	Entry string `json:"entry,omitempty"`
	Data  string `json:"data,omitempty"`
	Mock  string `json:"mock,omitempty"`
	Theme string `json:"theme,omitempty"`
	Cols  int    `json:"cols,omitempty"`
	Rows  int    `json:"rows,omitempty"`
}

// Frame is what open, render, step, and reload return besides the
// session id.
type Frame struct {
	// Dump is the frame as tuimark dump --format json --styles --cells
	// prints it.
	Dump *tuimark.Dump `json:"dump"`
	// Events are the host actions dispatched during the call, as play's
	// events are.
	Events []tuimark.PlayEvent `json:"events"`
	Quit   bool                `json:"quit"`
	// MockErrors lists the mock batches that failed during the call
	// ("ACTION: reason"), as tuimark run's "! ACTION: reason" strip entries.
	MockErrors []string `json:"mockErrors,omitempty"`
	// StepError is set when a step could not be applied; the frame is the
	// state reached so far and the earlier steps stay applied.
	StepError string `json:"stepError,omitempty"`
}

// Report is validate's result: tuimark validate --json.
type Report struct {
	OK          bool                 `json:"ok"`
	File        string               `json:"file"`
	Data        *string              `json:"data"`
	Diagnostics []tuimark.Diagnostic `json:"diagnostics"`
	Actions     []tuimark.ActionSpec `json:"actions"`
}

type mockWrite struct {
	path  string
	value any
}

type session struct {
	files      memFS
	entry      string
	dataName   string // "" when no data is bound
	mockName   string // "" when there is no mock
	theme      string
	cols, rows int
	app        *tuimark.App
	mockErrs   []string
}

// Manager holds the open sessions. A Manager is safe for concurrent use,
// but calls are serialized.
type Manager struct {
	mu       sync.Mutex
	next     int
	sessions map[int]*session
}

// NewManager returns an empty Manager.
func NewManager() *Manager {
	return &Manager{next: 1, sessions: map[int]*session{}}
}

// Call runs one method (see the package comment) and returns its JSON
// result. It never panics.
func (m *Manager) Call(method string, args ...string) (out string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	arg := func(i int) string {
		if i < len(args) {
			return args[i]
		}
		return ""
	}
	id := 0
	defer func() {
		if r := recover(); r != nil {
			if id != 0 {
				delete(m.sessions, id)
			}
			out = errorJSON(fmt.Errorf("internal error in %s: %v", method, r))
		}
	}()
	var res any
	var err error
	switch method {
	case "open":
		res, err = m.open(arg(0), arg(1))
	case "render", "step", "validate", "reload", "close":
		if err = decodeArg(arg(0), &id); err != nil {
			err = fmt.Errorf("session: %w", err)
			break
		}
		switch method {
		case "render":
			res, err = m.render(id, arg(1))
		case "step":
			res, err = m.step(id, arg(1), arg(2))
		case "validate":
			res, err = m.validate(id)
		case "reload":
			res, err = m.reload(id, arg(1), arg(2))
		case "close":
			delete(m.sessions, id)
			res = struct{}{}
		}
	default:
		err = fmt.Errorf("unknown method %q", method)
	}
	if err != nil {
		return errorJSON(err)
	}
	b, merr := json.Marshal(res)
	if merr != nil {
		return errorJSON(merr)
	}
	return string(b)
}

func errorJSON(err error) string {
	b, _ := json.Marshal(map[string]string{"error": err.Error()})
	return string(b)
}

// decodeArg decodes one JSON argument; absent, "null", and "undefined"
// leave v alone.
func decodeArg(raw string, v any) error {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" || raw == "undefined" {
		return nil
	}
	return json.Unmarshal([]byte(raw), v)
}

func (m *Manager) get(id int) (*session, error) {
	s := m.sessions[id]
	if s == nil {
		return nil, errors.New("no such session")
	}
	return s, nil
}

// buildFiles validates a files object and turns it into a memFS.
func buildFiles(raw string) (memFS, error) {
	var in map[string]string
	if err := decodeArg(raw, &in); err != nil {
		return nil, fmt.Errorf("files: want an object of path to content: %w", err)
	}
	if len(in) == 0 {
		return nil, errors.New("files: no files")
	}
	if len(in) > maxFiles {
		return nil, fmt.Errorf("files: %d files, at most %d", len(in), maxFiles)
	}
	fsys := make(memFS, len(in))
	total := 0
	for p, c := range in {
		k, ok := cleanKey(p)
		if !ok {
			return nil, fmt.Errorf("files: %q is not a valid relative path", p)
		}
		if _, dup := fsys[k]; dup {
			return nil, fmt.Errorf("files: %q appears twice", k)
		}
		total += len(c)
		fsys[k] = c
	}
	if total > maxTotalBytes {
		return nil, fmt.Errorf("files: %d bytes, at most %d", total, maxTotalBytes)
	}
	return fsys, nil
}

func (f memFS) has(p string) bool { _, ok := f[p]; return ok }

// resolve picks the entry, data, and mock names of a session from opts
// and the conventions in the package comment. entryDefault, when set,
// stands in for an absent opts.Entry (a reload keeps the document).
func resolve(files memFS, o Options, entryDefault string) (entry, data, mock string, err error) {
	name := func(what, given string) (string, error) {
		k, ok := cleanKey(given)
		if !ok {
			return "", fmt.Errorf("%s %q is not a valid relative path", what, given)
		}
		if !files.has(k) {
			return "", fmt.Errorf("%s %q is not in files", what, k)
		}
		return k, nil
	}
	switch {
	case o.Entry != "":
		if entry, err = name("entry", o.Entry); err != nil {
			return
		}
	case entryDefault != "" && files.has(entryDefault):
		entry = entryDefault
	case files.has("app.tui"):
		entry = "app.tui"
	default:
		var tuis []string
		for p := range files {
			if strings.HasSuffix(p, ".tui") {
				tuis = append(tuis, p)
			}
		}
		sort.Strings(tuis)
		if len(tuis) != 1 {
			err = fmt.Errorf("entry: %d .tui files and no app.tui; set opts.entry", len(tuis))
			return
		}
		entry = tuis[0]
	}
	dir := path.Dir(entry)
	beside := func(n string) string {
		if dir == "." {
			return n
		}
		return dir + "/" + n
	}
	if o.Data != "" {
		if data, err = name("data", o.Data); err != nil {
			return
		}
	} else if p := strings.TrimSuffix(entry, ".tui") + ".sample.json"; files.has(p) {
		data = p
	} else if p := beside("sample.json"); files.has(p) {
		data = p
	}
	if o.Mock != "" {
		if mock, err = name("mock", o.Mock); err != nil {
			return
		}
	} else if p := beside("mock.json"); files.has(p) {
		mock = p
	}
	return
}

// loadApp loads the entry from files and binds the data file at the root,
// as tuimark dump --data does.
func loadApp(files memFS, entry, data string) (*tuimark.App, error) {
	app, err := tuimark.LoadFS(files, entry)
	if err != nil {
		return nil, err
	}
	if data != "" {
		var v any
		if err := json.Unmarshal([]byte(files[data]), &v); err != nil {
			return nil, fmt.Errorf("%s: %w", data, err)
		}
		if err := app.Bind("", v); err != nil {
			return nil, fmt.Errorf("%s: %w", data, err)
		}
	}
	return app, nil
}

func setTheme(app *tuimark.App, theme string) error {
	switch theme {
	case "":
		return nil
	case "dark", "light":
		return app.Set("@theme", theme)
	}
	return fmt.Errorf("theme must be dark or light (got %q)", theme)
}

// readMock parses and checks a tuimark run --mock file against app's
// catalog, as run does (SPEC v0.4a §15.9): an object whose members name
// host actions, each an array of {"path", "value"} writes.
func readMock(name, raw string, app *tuimark.App) (map[string][]mockWrite, error) {
	bad := fmt.Errorf(`%s: want one JSON object mapping action names to arrays of {"path", "value"} writes`, name)
	var members map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &members); err != nil || members == nil {
		return nil, bad
	}
	actions := map[string]bool{}
	for _, s := range app.Catalog() {
		actions[s.Name] = s.Builtin
	}
	names := make([]string, 0, len(members))
	for n := range members {
		names = append(names, n)
	}
	sort.Strings(names)
	mk := make(map[string][]mockWrite, len(members))
	for _, n := range names {
		builtin, ok := actions[n]
		switch {
		case !ok:
			return nil, fmt.Errorf("%s: %q is not an action of the document", name, n)
		case builtin:
			return nil, fmt.Errorf("%s: %q is a built-in action; a mock names host actions only", name, n)
		}
		var items []map[string]json.RawMessage
		if err := json.Unmarshal(members[n], &items); err != nil || items == nil {
			return nil, fmt.Errorf(`%s: %q: want an array of {"path", "value"} writes`, name, n)
		}
		writes := make([]mockWrite, len(items))
		for i, it := range items {
			var p string
			rp, hasPath := it["path"]
			rv, hasValue := it["value"]
			if it == nil || !hasPath || !hasValue || json.Unmarshal(rp, &p) != nil {
				return nil, fmt.Errorf(`%s: %q[%d] needs a string "path" and a "value"`, name, n, i)
			}
			if err := json.Unmarshal(rv, &writes[i].value); err != nil {
				return nil, fmt.Errorf(`%s: %q[%d].value: %v`, name, n, i, err)
			}
			writes[i].path = p
		}
		mk[n] = writes
	}
	return mk, nil
}

// register answers every host action of app as tuimark run --mock does:
// the action's writes applied as one Batch. A failed batch is recorded in
// s.mockErrs; the handler always returns nil, so the call goes on.
func (s *session) register(app *tuimark.App, mk map[string][]mockWrite) {
	for _, spec := range app.Catalog() {
		if spec.Builtin {
			continue
		}
		writes := mk[spec.Name]
		app.On(spec.Name, func(ev tuimark.Event) error {
			if len(writes) == 0 {
				return nil
			}
			err := app.Batch(func(b *tuimark.Batch) error {
				for _, w := range writes {
					_ = b.Set(w.path, w.value)
				}
				return nil
			})
			if err != nil {
				s.mockErrs = append(s.mockErrs, ev.Action+": "+joinedErrText(err))
			}
			return nil
		})
	}
}

func joinedErrText(err error) string {
	if j, ok := err.(interface{ Unwrap() []error }); ok {
		parts := j.Unwrap()
		texts := make([]string, len(parts))
		for i, p := range parts {
			texts[i] = p.Error()
		}
		return strings.Join(texts, "; ")
	}
	return err.Error()
}

// setup loads a session's app, applies the theme, and wires its mock.
func (s *session) setup() error {
	app, err := loadApp(s.files, s.entry, s.dataName)
	if err != nil {
		return err
	}
	if err := setTheme(app, s.theme); err != nil {
		return err
	}
	if s.mockName != "" {
		mk, err := readMock(s.mockName, s.files[s.mockName], app)
		if err != nil {
			return err
		}
		s.register(app, mk)
	}
	s.app = app
	return nil
}

// size applies opts' cols and rows over the session's.
func (s *session) size(o Options) (cols, rows int) {
	cols, rows = s.cols, s.rows
	if o.Cols != 0 {
		cols = o.Cols
	}
	if o.Rows != 0 {
		rows = o.Rows
	}
	return
}

// play runs steps at the session's size (opts over it) and returns the
// frame. A partial failure (a step that cannot be applied) is the
// frame's StepError; an unusable call (a bad step, a bad size) is the
// error.
func (s *session) play(o Options, steps []string) (*Frame, error) {
	if err := setTheme(s.app, o.Theme); err != nil {
		return nil, err
	}
	if o.Theme != "" {
		s.theme = o.Theme
	}
	cols, rows := s.size(o)
	s.mockErrs = nil
	res, err := s.app.Play(tuimark.PlayOptions{Cols: cols, Rows: rows, Styles: true, Cells: true}, steps...)
	if res == nil {
		return nil, err
	}
	// A resize: step changes the size for the calls after it.
	s.cols, s.rows = res.Dump.Cols, res.Dump.Rows
	f := &Frame{Dump: res.Dump, Events: res.Events, Quit: res.Quit, MockErrors: s.mockErrs}
	if f.Events == nil {
		f.Events = []tuimark.PlayEvent{}
	}
	if err != nil {
		f.StepError = err.Error()
	}
	return f, nil
}

type opened struct {
	Session int `json:"session"`
	*Frame
}

func (m *Manager) open(rawFiles, rawOpts string) (any, error) {
	if len(m.sessions) >= maxSessions {
		return nil, fmt.Errorf("too many open sessions (%d)", maxSessions)
	}
	files, err := buildFiles(rawFiles)
	if err != nil {
		return nil, err
	}
	var o Options
	if err := decodeArg(rawOpts, &o); err != nil {
		return nil, fmt.Errorf("opts: %w", err)
	}
	s := &session{files: files, theme: o.Theme, cols: defaultCols, rows: defaultRows}
	if s.entry, s.dataName, s.mockName, err = resolve(files, o, ""); err != nil {
		return nil, err
	}
	if err := s.setup(); err != nil {
		return nil, err
	}
	f, err := s.play(Options{Cols: o.Cols, Rows: o.Rows}, nil)
	if err != nil {
		return nil, err
	}
	id := m.next
	m.next++
	m.sessions[id] = s
	return opened{Session: id, Frame: f}, nil
}

func (m *Manager) render(id int, rawOpts string) (any, error) {
	s, err := m.get(id)
	if err != nil {
		return nil, err
	}
	var o Options
	if err := decodeArg(rawOpts, &o); err != nil {
		return nil, fmt.Errorf("opts: %w", err)
	}
	return s.play(o, nil)
}

func (m *Manager) step(id int, rawSteps, rawOpts string) (any, error) {
	s, err := m.get(id)
	if err != nil {
		return nil, err
	}
	var steps []string
	var one string
	switch {
	case json.Unmarshal([]byte(rawSteps), &steps) == nil:
	case json.Unmarshal([]byte(rawSteps), &one) == nil:
		steps = []string{one}
	default:
		return nil, errors.New("steps: want a string or an array of strings")
	}
	for i, st := range steps {
		if strings.HasPrefix(st, "paste:") && len(st)-len("paste:") > MaxPaste {
			return nil, fmt.Errorf("step %d: paste is %d bytes, at most %d", i+1, len(st)-len("paste:"), MaxPaste)
		}
	}
	var o Options
	if err := decodeArg(rawOpts, &o); err != nil {
		return nil, fmt.Errorf("opts: %w", err)
	}
	return s.play(o, steps)
}

// validate mirrors tuimark validate --json --data DATA on the files as
// opened, not on the live store a session's steps have changed.
func (m *Manager) validate(id int) (any, error) {
	s, err := m.get(id)
	if err != nil {
		return nil, err
	}
	app, err := loadApp(s.files, s.entry, s.dataName)
	if err != nil {
		return nil, err
	}
	if err := setTheme(app, s.theme); err != nil {
		return nil, err
	}
	diags := app.Validate()
	if s.dataName == "" {
		// Without data every bind path is missing: report only the
		// data-free checks, as tuimark validate does.
		var kept []tuimark.Diagnostic
		for _, d := range diags {
			switch d.Code {
			case "B001", "B002", "B003", "B008", "B009", "B010", "B011":
				continue
			}
			kept = append(kept, d)
		}
		diags = kept
	}
	if diags == nil {
		diags = []tuimark.Diagnostic{}
	}
	r := &Report{OK: true, File: s.entry, Diagnostics: diags, Actions: app.Catalog()}
	for _, d := range diags {
		if d.Severity == "error" {
			r.OK = false
		}
	}
	if s.dataName != "" {
		name := s.dataName
		r.Data = &name
	}
	return r, nil
}

// reload swaps in edited files and keeps what the public API can carry
// across documents: the whole store (bound cursors, typed text, checked
// keys), the focus, and the screen. Scroll offsets and cursors without a
// bind are lost. A failed reload leaves the session as it was.
func (m *Manager) reload(id int, rawFiles, rawOpts string) (any, error) {
	s, err := m.get(id)
	if err != nil {
		return nil, err
	}
	files, err := buildFiles(rawFiles)
	if err != nil {
		return nil, err
	}
	var o Options
	if err := decodeArg(rawOpts, &o); err != nil {
		return nil, fmt.Errorf("opts: %w", err)
	}
	if o.Mock == "" && s.mockName != "" && files.has(s.mockName) {
		o.Mock = s.mockName
	}
	if o.Data == "" && s.dataName != "" && files.has(s.dataName) {
		o.Data = s.dataName
	}
	n := &session{files: files, theme: s.theme, cols: s.cols, rows: s.rows}
	if o.Theme != "" {
		n.theme = o.Theme
	}
	if n.entry, n.dataName, n.mockName, err = resolve(files, o, s.entry); err != nil {
		return nil, err
	}
	if err := n.setup(); err != nil {
		return nil, err
	}
	if store, ok := s.app.Get(""); ok {
		if err := n.app.Bind("", store); err != nil {
			return nil, err
		}
	}
	if sc, ok := s.app.Get("@screen"); ok && sc != nil {
		_ = n.app.Set("@screen", sc)
	}
	if fo, ok := s.app.Get("@focus"); ok && fo != nil {
		_ = n.app.Set("@focus", fo)
	}
	f, err := n.play(Options{Cols: o.Cols, Rows: o.Rows}, nil)
	if err != nil {
		return nil, err
	}
	m.sessions[id] = n
	return f, nil
}
