package main

// tuimark run FILE: a document live in the terminal, with no Go and no
// parent program (SPEC v0.4a §15.9). The session is Run()'s, through
// RunHost and the internal hooks `run` adds to it: every host action is
// answered here (its --mock writes applied as one Batch, then the event
// recorded in the strip and in --events), F12 shows or hides the event
// strip, the panel lists the document's static diagnostics, two ctrl+c
// within a second always end the session, and a watcher reloads the
// document, its stylesheets, --data, and --mock when they change,
// handing the new app to the loop. internal/host lays out, paints, and
// decides everything else.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/abdul-hamid-achik/tuimark/internal/host"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// runPoll is how often the watcher looks at the watched files, as
// `preview --watch` does (SPEC v0.4a §15.9 "Reload").
const runPoll = 150 * time.Millisecond

// stripKeep is the most entries the strip ever shows (SPEC v0.4a §15.9:
// min(8, max(1, floor(rows / 3)))), so no more are kept.
const stripKeep = 8

func (c *cli) cmdRun(args []string) int {
	fs := c.newFlags("run")
	data := fs.String("data", "", "JSON file bound as the data store")
	theme := fs.String("theme", "", "dark or light; applied as Set(\"@theme\", …)")
	events := fs.String("events", "", "NDJSON file that gets one line per host action")
	mock := fs.String("mock", "", "JSON file mapping actions to the store writes they make")
	strip := fs.Bool("strip", false, "show the event strip from the start (F12 shows or hides it)")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return 1
	}
	file, err := oneFile("run", pos)
	if err != nil {
		return c.runFail(err)
	}
	setTheme := themeFlagSet(fs)
	if setTheme && *theme != "dark" && *theme != "light" {
		return c.runFail(fmt.Errorf("--theme must be dark or light (got %q)", *theme))
	}
	r := &runner{file: file, data: *data, mockPath: *mock, theme: *theme, setTheme: setTheme, stripOn: *strip}
	// Everything that can fail before the session does so here, before the
	// terminal is touched (SPEC v0.4a §15.9 "Errors before the session").
	// A document with error-severity diagnostics still runs, as under
	// Run(): its static diagnostics are the panel's from the first frame.
	r.dataStamp = fingerprint([]string{r.data})
	app, mk, err := r.load(true, true)
	if err != nil {
		return c.runFail(err)
	}
	if *events != "" {
		f, err := host.OpenRecordFile(*events, "the event log")
		if err != nil {
			return c.runFail(fmt.Errorf("--events: %v", err))
		}
		defer f.Close()
		r.events = &eventLog{w: f}
	}
	r.sheets = sheetsOf(app)
	r.panel = diagLines(app.Static())
	r.register(app, mk)
	return r.run(app, c.stdout, c.stderr)
}

// runFail prints an error before (or instead of) the session as one
// "tuimark: run: …" line and returns exit code 1 (SPEC v0.4a §15.9).
func (c *cli) runFail(err error) int {
	fmt.Fprintf(c.stderr, "tuimark: run: %s\n", hostErrText(err))
	return 1
}

// runner is one `tuimark run` session: what the command line named, the
// strip and the panel, and the --events log. The strip and the panel are
// read and written on the loop's goroutine (handlers, the F12 notice, the
// overlay, a swap's Applied); the mutex keeps them safe from any other.
type runner struct {
	file, data, mockPath, theme string
	setTheme                    bool

	// The watcher's own state: the stylesheets of the last load that read
	// the document far enough to resolve its <style src>, and the
	// fingerprint of --data when it was last bound.
	sheets    []string
	dataStamp string

	mu      sync.Mutex
	entries []string // the strip's entries, newest last, at most stripKeep
	stripOn bool
	panel   []string // the panel's lines: diagnostics, or a reload's error
	events  *eventLog
}

// mockWrite is one write of a --mock action: the writes of a §19.1
// batch.
type mockWrite struct {
	path  string
	value any
}

// load reads FILE, --data, and --mock as the session needs them, and
// applies --theme: at the start (initial), when the document only has to
// load, and at a reload, when it must also have no error-severity static
// diagnostic (SPEC v0.4a §15.9 "Success"). It binds --data at the root
// when bind is set: at the start, and at a reload only when the file
// changed since it was last bound. The error is the usage error at the
// start and the panel's text at a reload (a diagError for a document with
// errors).
func (r *runner) load(initial, bind bool) (app *host.App, mk map[string][]mockWrite, err error) {
	app, err = host.Load(r.file)
	if err != nil {
		return nil, nil, err
	}
	if r.setTheme {
		if err := app.Set(host.ThemePath, r.theme); err != nil {
			return nil, nil, err
		}
	}
	if !initial {
		if app.Document().Root != nil {
			r.sheets = sheetsOf(app)
		}
		if st := app.Static(); st.HasErrors() {
			return nil, nil, diagError(st)
		}
	}
	if r.data != "" && bind {
		v, err := readJSONFile(r.data)
		if err != nil {
			return nil, nil, err
		}
		if err := app.Bind("", v); err != nil {
			return nil, nil, fmt.Errorf("%s: %w", r.data, err)
		}
	}
	if r.mockPath != "" {
		mk, err = readMock(r.mockPath, app)
		if err != nil {
			return nil, nil, err
		}
	}
	return app, mk, nil
}

// diagError is a reload whose document has an error-severity static
// diagnostic: the panel lists them all.
type diagError ir.Diags

func (d diagError) Error() string { return strings.Join(diagLines(ir.Diags(d)), "\n") }

// readJSONFile reads and parses one JSON file.
func readJSONFile(path string) (any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return v, nil
}

// readMock reads a --mock file and checks it against app's catalog (SPEC
// v0.4a §15.9 "--mock"): one JSON object whose members name actions of the
// catalog that are not built in, each an array of writes {"path": P,
// "value": V}, as a §19.1 batch's (a string path and a value; other
// members are ignored there too). Members are checked in name order, so
// the error a file gives does not depend on map order.
func readMock(path string, app *host.App) (map[string][]mockWrite, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(raw, &members); err != nil || members == nil {
		if err == nil || jsonKind(raw) != 'o' {
			return nil, fmt.Errorf(`%s: want one JSON object mapping action names to arrays of {"path", "value"} writes`, path)
		}
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	actions := map[string]bool{}
	for _, s := range app.Catalog() {
		actions[s.Name] = s.Builtin
	}
	names := make([]string, 0, len(members))
	for name := range members {
		names = append(names, name)
	}
	sort.Strings(names)
	mk := make(map[string][]mockWrite, len(members))
	for _, name := range names {
		builtin, ok := actions[name]
		switch {
		case !ok:
			return nil, fmt.Errorf("%s: %q is not an action of the document", path, name)
		case builtin:
			return nil, fmt.Errorf("%s: %q is a built-in action; a mock names host actions only", path, name)
		}
		var items []wireMsg
		if jsonKind(members[name]) != 'a' || json.Unmarshal(members[name], &items) != nil {
			return nil, fmt.Errorf(`%s: %q: want an array of {"path", "value"} writes`, path, name)
		}
		writes := make([]mockWrite, len(items))
		for i, it := range items {
			p, ok := it.str("path")
			vraw, hasValue := it["value"]
			if it == nil || !ok || !hasValue {
				return nil, fmt.Errorf(`%s: %q[%d] needs a string "path" and a "value"`, path, name, i)
			}
			if err := json.Unmarshal(vraw, &writes[i].value); err != nil {
				return nil, fmt.Errorf(`%s: %q[%d].value: %v`, path, name, i, err)
			}
			writes[i].path = p
		}
		mk[name] = writes
	}
	return mk, nil
}

// register gives every non-builtin action of app's catalog its handler
// (SPEC v0.4a §15.9 "Host actions"): the action's mock writes, applied as
// one Batch (literal values: Batch copies each one in), then the event
// recorded. A failed batch changes nothing and adds a "! ACTION: …" entry
// before the event's. It always returns nil; built-ins behave as in
// Run(), and quit ends the session, since no quit handler is registered.
func (r *runner) register(app *host.App, mk map[string][]mockWrite) {
	for _, s := range app.Catalog() {
		if s.Builtin {
			continue
		}
		writes := mk[s.Name]
		app.On(s.Name, func(ev host.Event) error {
			if len(writes) > 0 {
				err := app.Batch(func(b *host.Batch) error {
					for _, w := range writes {
						_ = b.Set(w.path, w.value)
					}
					return nil
				})
				if err != nil {
					r.add("! " + ev.Action + ": " + joinedErrText(err))
				}
			}
			r.record(ev, secretSource(app, ev.Source))
			return nil
		})
	}
}

// secretSource reports whether id names an input with secret="true" in
// app's document: its events show [redacted] in the strip and have no
// value in --events (SPEC v0.4a §15.9, v0.2b §26.12).
func secretSource(app *host.App, id string) bool {
	if id == "" {
		return false
	}
	n := app.Document().IDs[id]
	return n != nil && n.Kind == "input" && n.Attrs["secret"] == "true"
}

// joinedErrText is a Batch error's parts (SPEC v0.3b §18.1, errors.Join)
// separated by "; ".
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

// record adds the event's strip entry and its --events line. A failed
// --events write stops the log, never the session, and adds one
// "! events: …" entry.
func (r *runner) record(ev host.Event, secret bool) {
	r.add(stripEventLine(ev, secret))
	if r.events == nil {
		return
	}
	if err := r.events.write(ev, secret); err != nil {
		r.add("! events: " + err.Error())
	}
}

// stripEventLine is an event's strip entry: "ACTION SOURCE KEYS VALUE", as
// `play`'s text output writes an event without its step (SPEC §15.4),
// with VALUE "[redacted]" when the source is a secret input.
func stripEventLine(ev host.Event, secret bool) string {
	src := ev.Source
	if src == "" {
		src = "-"
	}
	keys := ev.Keys
	if keys == nil {
		keys = map[string]any{}
	}
	keysJSON, _ := json.Marshal(keys)
	val := "[redacted]"
	if !secret {
		raw, _ := json.Marshal(ev.Value)
		val = string(raw)
	}
	return fmt.Sprintf("%s %s %s %s", ev.Action, src, keysJSON, val)
}

// add appends one strip entry, keeping the last stripKeep.
func (r *runner) add(entry string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, entry)
	if n := len(r.entries); n > stripKeep {
		r.entries = append([]string(nil), r.entries[n-stripKeep:]...)
	}
}

// overlay is RunHost's Overlay hook: the panel's lines for the top rows
// and the strip's for the bottom rows of a rows-high terminal.
func (r *runner) overlay(cols, rows int) (top, bottom []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	top = panelLines(r.panel, rows)
	if r.stripOn {
		bottom = stripLines(r.entries, rows)
	}
	return top, bottom
}

// stripLines are the strip's rows (SPEC v0.4a §15.9 "Size"): the last
// min(8, max(1, floor(rows / 3))) entries, newest last. With no entry the
// strip paints nothing.
func stripLines(entries []string, rows int) []string {
	n := min(stripKeep, max(1, rows/3))
	if len(entries) > n {
		entries = entries[len(entries)-n:]
	}
	return append([]string(nil), entries...)
}

// panelLines are the panel's rows (SPEC v0.4a §15.9 "Panel"): at most
// max(1, floor(rows / 3)), the last one reading "… N more" when lines
// has more than fit, N the lines it leaves out.
func panelLines(lines []string, rows int) []string {
	n := max(1, rows/3)
	if len(lines) <= n {
		return append([]string(nil), lines...)
	}
	out := append([]string(nil), lines[:n-1]...)
	return append(out, fmt.Sprintf("… %d more", len(lines)-(n-1)))
}

// diagLines are diagnostics in validate's text form, one per line.
func diagLines(ds ir.Diags) []string {
	out := make([]string, len(ds))
	for i, d := range ds {
		out[i] = d.String()
	}
	return out
}

// errLines is a failed reload's error in validate's text form: the
// document's diagnostics, or one "tuimark: …" line.
func errLines(err error) []string {
	var d diagError
	if errors.As(err, &d) {
		return diagLines(ir.Diags(d))
	}
	return []string{"tuimark: " + hostErrText(err)}
}

// sheetsOf is the stylesheets app's load named: Files lists the document
// first (Load always has its path), then every <style src>.
func sheetsOf(app *host.App) []string {
	files := app.Files()
	if len(files) > 0 {
		files = files[1:]
	}
	return files
}

// watched is every file a reload looks at: FILE, the stylesheets of the
// last load that resolved them, --data, and --mock.
func (r *runner) watched() []string {
	files := append([]string{r.file}, r.sheets...)
	return append(files, r.data, r.mockPath)
}

// watch polls the watched files every runPoll until done is closed, and
// hands each change it sees to the loop as one reload (SPEC v0.4a §15.9
// "Reload"): changes seen within one poll are one reload. It waits for
// the loop to take one before it looks again.
func (r *runner) watch(swaps chan<- host.HostSwap, done <-chan struct{}) {
	stamp := fingerprint(r.watched())
	t := time.NewTicker(runPoll)
	defer t.Stop()
	for {
		select {
		case <-done:
			return
		case <-t.C:
		}
		now := fingerprint(r.watched())
		if now == stamp {
			continue
		}
		dataNow := fingerprint([]string{r.data})
		sw, ok := r.reload(dataNow != r.dataStamp)
		select {
		case swaps <- sw:
		case <-done:
			return
		}
		if ok {
			// The loop applies a swap as it takes it: --data, bound again
			// or not, is now the store's source.
			r.dataStamp = dataNow
		}
		// A reload may name other stylesheets: they are watched from now
		// on, as they are now.
		stamp = fingerprint(r.watched())
	}
}

// reload builds what the loop applies for one change: a new app (the
// files loaded again, handlers registered for its catalog), the strip's
// "reload" line and its warnings for the panel; or, when it fails, the
// panel's new lines while the old app runs on (SPEC v0.4a §15.9
// "Success", "Failure"). dataChanged reports that --data changed since it
// was last bound: the new data is then the root, else the store moves.
func (r *runner) reload(dataChanged bool) (host.HostSwap, bool) {
	keep := r.data == "" || !dataChanged
	app, mk, err := r.load(false, !keep)
	if err != nil {
		lines := errLines(err)
		return host.HostSwap{Applied: func() {
			r.mu.Lock()
			r.panel = lines
			r.mu.Unlock()
		}}, false
	}
	r.register(app, mk)
	lines := diagLines(app.Static())
	return host.HostSwap{App: app, KeepStore: keep, Applied: func() {
		r.add("reload")
		r.mu.Lock()
		r.panel = lines
		r.mu.Unlock()
	}}, true
}

// run drives the session and returns the exit code (SPEC v0.4a §15.9
// "Exit codes"): 0 for quit and the end of the input, 1 for an error, 128
// + the signal number for a stop signal.
func (r *runner) run(app *host.App, stdout, stderr io.Writer) int {
	swaps := make(chan host.HostSwap)
	done := make(chan struct{})
	var once sync.Once
	hooks := host.HostHooks{
		// The watcher starts with the session, once the terminal's size is
		// known, so no reload reaches a session that never started.
		Start: func(cols, rows int, theme string) {
			once.Do(func() { go r.watch(swaps, done) })
		},
		Overlay: r.overlay,
		F12: func() {
			r.mu.Lock()
			r.stripOn = !r.stripOn
			r.mu.Unlock()
		},
		CtrlCTwice: true,
		Swap:       swaps,
	}
	res := app.RunHost(stdout, hooks)
	close(done)
	switch res.Reason {
	case "signal":
		return 128 + signalNumber(res.Signal)
	case "error":
		fmt.Fprintf(stderr, "tuimark: run: %s\n", hostErrText(res.Err))
		return 1
	}
	return 0
}

// eventLog is the --events file (SPEC v0.4a §15.9): one line per
// dispatched host action, the members of a §19.1 "event" without "type",
// seq counting from 1. A write error stops it.
type eventLog struct {
	w    io.Writer
	seq  int64
	dead bool
}

type eventRecord struct {
	Seq    int64          `json:"seq"`
	Action string         `json:"action"`
	Source string         `json:"source"`
	Keys   map[string]any `json:"keys"`
	Value  any            `json:"value"`
}

// redactedRecord is the record of an event whose source is a secret input:
// no value, and "redacted": true at the end, as in TUIMARK_LOG.
type redactedRecord struct {
	Seq      int64          `json:"seq"`
	Action   string         `json:"action"`
	Source   string         `json:"source"`
	Keys     map[string]any `json:"keys"`
	Redacted bool           `json:"redacted"`
}

// write appends ev's record. It returns the error of the write that
// stopped the log, once; afterwards it writes nothing and returns nil.
func (l *eventLog) write(ev host.Event, secret bool) error {
	if l.dead {
		return nil
	}
	l.seq++
	keys := ev.Keys
	if keys == nil {
		keys = map[string]any{}
	}
	var rec any = eventRecord{Seq: l.seq, Action: ev.Action, Source: ev.Source, Keys: keys, Value: ev.Value}
	if secret {
		rec = redactedRecord{Seq: l.seq, Action: ev.Action, Source: ev.Source, Keys: keys, Redacted: true}
	}
	line, err := json.Marshal(rec)
	if err == nil {
		_, err = l.w.Write(append(line, '\n'))
	}
	if err != nil {
		l.dead = true
		return err
	}
	return nil
}
