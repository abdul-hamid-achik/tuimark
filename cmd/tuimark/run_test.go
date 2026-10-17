package main

// SPEC v0.4a §15.9, test 121: the parts of `tuimark run` that need no
// terminal: the errors before the session, --mock, --events, and the
// strip's and the panel's lines. run_pty_test.go runs whole sessions, and
// specs/glyphrun/run_*.yml the rest of test 121.

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/tuimark/internal/host"
)

// runDoc has two host actions from a keymap and an input whose events are
// redacted.
const runDoc = `<tui version="1">
<keymap><bind keys="s" action="save"/><bind keys="q" action="quit"/></keymap>
<screen id="s" focus="#pw">
  <text>RUNAPP {saved}</text>
  <input id="pw" secret="true" on:submit="login"/>
</screen>
</tui>`

// writeIn writes one file into dir and returns its path.
func writeIn(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// Test 121, "Errors before the session": each is one "tuimark: run: …"
// line on stderr and exit 1, and nothing reaches stdout, where the session
// would have written its first byte.
func TestRunErrorsBeforeTheSession(t *testing.T) {
	dir := t.TempDir()
	file := writeIn(t, dir, "app.tui", runDoc)
	data := writeIn(t, dir, "data.json", `{"saved": "no"}`)
	mock := func(name, body string) string { return writeIn(t, dir, name, body) }
	cases := []struct {
		name string
		args []string
		want string // a part of the one stderr line
	}{
		{"no FILE", []string{"run"}, "want exactly one FILE, got 0"},
		{"two FILEs", []string{"run", file, file}, "want exactly one FILE, got 2"},
		{"missing FILE", []string{"run", filepath.Join(dir, "nope.tui")}, "nope.tui"},
		{"bad theme", []string{"run", file, "--theme", "blue"}, `--theme must be dark or light (got "blue")`},
		{"missing data", []string{"run", file, "--data", filepath.Join(dir, "nope.json")}, "nope.json"},
		{"bad data", []string{"run", file, "--data", mock("bad.json", `{"a":`)}, "bad.json"},
		{"missing mock", []string{"run", file, "--mock", filepath.Join(dir, "nomock.json")}, "nomock.json"},
		{"bad mock JSON", []string{"run", file, "--mock", mock("m1.json", `{"save": [`)}, "m1.json"},
		{"mock not an object", []string{"run", file, "--mock", mock("m2.json", `[]`)}, "want one JSON object"},
		{"mock null", []string{"run", file, "--mock", mock("m3.json", `null`)}, "want one JSON object"},
		{"unknown action", []string{"run", file, "--mock", mock("m4.json", `{"sav": []}`)}, `"sav" is not an action of the document`},
		{"built-in action", []string{"run", file, "--mock", mock("m5.json", `{"quit": []}`)}, `"quit" is a built-in action`},
		{"value not an array", []string{"run", file, "--mock", mock("m6.json", `{"save": {"path": "a", "value": 1}}`)}, `"save": want an array`},
		{"write without path", []string{"run", file, "--mock", mock("m7.json", `{"save": [{"value": 1}]}`)}, `"save"[0] needs a string "path" and a "value"`},
		{"write without value", []string{"run", file, "--mock", mock("m8.json", `{"save": [{"path": "a"}]}`)}, `"save"[0] needs`},
		{"write not an object", []string{"run", file, "--mock", mock("m9.json", `{"save": [1]}`)}, `"save"`},
		{"path not a string", []string{"run", file, "--mock", mock("m10.json", `{"save": [{"path": 1, "value": 1}]}`)}, `"save"[0] needs`},
		{"events cannot be opened", []string{"run", file, "--data", data, "--events", filepath.Join(dir, "no", "dir", "ev.ndjson")}, "--events:"},
	}
	for _, tc := range cases {
		code, stdout, stderr := runCLI(tc.args...)
		lines := strings.Split(strings.TrimSuffix(stderr, "\n"), "\n")
		switch {
		case code != 1:
			t.Errorf("%s: exit %d, want 1 (stderr %q)", tc.name, code, stderr)
		case stdout != "":
			t.Errorf("%s: stdout %q, want nothing before the session", tc.name, stdout)
		case len(lines) != 1 || !strings.HasPrefix(lines[0], "tuimark: run: ") || !strings.HasSuffix(stderr, "\n"):
			t.Errorf("%s: stderr %q, want one \"tuimark: run: …\" line", tc.name, stderr)
		case !strings.Contains(lines[0], tc.want):
			t.Errorf("%s: stderr %q, want it to say %q", tc.name, lines[0], tc.want)
		}
	}
}

// A mock is checked before --events is opened: a usage error leaves no
// event file behind.
func TestRunMockErrorCreatesNoEventFile(t *testing.T) {
	dir := t.TempDir()
	file := writeIn(t, dir, "app.tui", runDoc)
	mock := writeIn(t, dir, "mock.json", `{"nope": []}`)
	ev := filepath.Join(dir, "ev.ndjson")
	if code, _, _ := runCLI("run", file, "--mock", mock, "--events", ev); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if _, err := os.Stat(ev); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the event file exists after a usage error: %v", err)
	}
}

// startRunner loads a runner as cmdRun does, without a session: handlers
// registered, the strip and the panel empty, --events in buf.
func startRunner(t *testing.T, doc, mock string) (*runner, *host.App, *bytes.Buffer) {
	t.Helper()
	dir := t.TempDir()
	r := &runner{file: writeIn(t, dir, "app.tui", doc)}
	if mock != "" {
		r.mockPath = writeIn(t, dir, "mock.json", mock)
	}
	app, mk, err := r.load(true, true)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	r.events = &eventLog{w: &buf}
	r.register(app, mk)
	return r, app, &buf
}

func (r *runner) strip() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.entries...)
}

// Test 121, "--mock": an action's writes are applied as one Batch before
// the event is recorded, with literal values (each application writes the
// file's value again, never one the store changed since); a failed batch
// changes nothing, adds "! ACTION: …" with the joined error's parts, and
// the event is still recorded.
func TestRunMockAppliesAndRecords(t *testing.T) {
	r, app, buf := startRunner(t, runDoc, `{
  "save": [{"path": "saved", "value": {"n": [1, 2]}}, {"path": "count", "value": 1}],
  "login": [{"path": "ok", "value": true}, {"path": "a..b", "value": 1}, {"path": "@screen", "value": "nowhere"}]
}`)
	if _, err := app.Dispatch(host.Event{Action: "save"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := app.Get("saved"); !reflect.DeepEqual(got, map[string]any{"n": []any{1.0, 2.0}}) {
		t.Errorf("saved = %#v after the mock", got)
	}
	// The store's copy changes; the mock's does not.
	if err := app.Set("saved.extra", "x"); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Dispatch(host.Event{Action: "save"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := app.Get("saved"); !reflect.DeepEqual(got, map[string]any{"n": []any{1.0, 2.0}}) {
		t.Errorf("saved = %#v: the mock's value was shared with the store", got)
	}

	// Two of login's writes fail, so the batch changes nothing (not even
	// ok), and the strip names both parts.
	if _, err := app.Dispatch(host.Event{Action: "login", Source: "pw", Value: "hunter2"}); err != nil {
		t.Fatal(err)
	}
	if got, ok := app.Get("ok"); ok {
		t.Errorf("a failed batch wrote ok = %v", got)
	}
	strip := r.strip()
	if len(strip) != 4 || strip[0] != "save - {} null" || strip[1] != "save - {} null" || strip[3] != "login pw {} [redacted]" {
		t.Fatalf("strip %q", strip)
	}
	if fail := strip[2]; !strings.HasPrefix(fail, "! login: ") || strings.Count(fail, "; ") != 1 || !strings.Contains(fail, `"a..b"`) || !strings.Contains(fail, `"nowhere"`) {
		t.Errorf("failed batch entry %q, want both parts joined by \"; \"", fail)
	}
	want := `{"seq":1,"action":"save","source":"","keys":{},"value":null}
{"seq":2,"action":"save","source":"","keys":{},"value":null}
{"seq":3,"action":"login","source":"pw","keys":{},"redacted":true}
`
	if buf.String() != want {
		t.Errorf("--events:\n%s\nwant:\n%s", buf.String(), want)
	}
	if strings.Contains(buf.String()+strings.Join(strip, "\n"), "hunter2") {
		t.Error("a secret input's value reached the strip or --events")
	}
}

// An action without a mock entry only records its event; keys and values
// are written as play's text output writes them.
func TestRunEventLines(t *testing.T) {
	ev := host.Event{Action: "pick", Source: "l", Keys: map[string]any{"r": "b"}, Value: "b"}
	if got := stripEventLine(ev, false); got != `pick l {"r":"b"} "b"` {
		t.Errorf("strip line %q", got)
	}
	if got := stripEventLine(ev, true); got != `pick l {"r":"b"} [redacted]` {
		t.Errorf("secret strip line %q", got)
	}
	var buf bytes.Buffer
	l := &eventLog{w: &buf}
	if err := l.write(ev, false); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != `{"seq":1,"action":"pick","source":"l","keys":{"r":"b"},"value":"b"}`+"\n" {
		t.Errorf("record %q", got)
	}
}

// failWriter fails every write after the first n.
type failWriter struct {
	n   int
	buf bytes.Buffer
}

func (w *failWriter) Write(p []byte) (int, error) {
	if w.n == 0 {
		return 0, errors.New("disk full")
	}
	w.n--
	return w.buf.Write(p)
}

// Test 121, "Write errors": a failed --events write stops the log, adds
// "! events: ERROR" once, and the session (the handler) goes on.
func TestRunEventsWriteErrorStopsOnlyTheLog(t *testing.T) {
	r, app, _ := startRunner(t, runDoc, `{"save": [{"path": "saved", "value": "yes"}]}`)
	fw := &failWriter{n: 1}
	r.events = &eventLog{w: fw}
	for i := 0; i < 3; i++ {
		if quit, err := app.Dispatch(host.Event{Action: "save"}); quit || err != nil {
			t.Fatalf("dispatch %d: quit %v err %v", i, quit, err)
		}
	}
	want := []string{"save - {} null", "save - {} null", "! events: disk full", "save - {} null"}
	if got := r.strip(); !reflect.DeepEqual(got, want) {
		t.Errorf("strip %q, want %q", got, want)
	}
	if got := fw.buf.String(); got != `{"seq":1,"action":"save","source":"","keys":{},"value":null}`+"\n" {
		t.Errorf("log %q, want the first record only", got)
	}
	if got, _ := app.Get("saved"); got != "yes" {
		t.Errorf("saved = %v: the mock stopped with the log", got)
	}
}

// The strip keeps the last 8 entries at most.
func TestRunStripKeepsEight(t *testing.T) {
	r := &runner{}
	for i := 0; i < 12; i++ {
		r.add(string(rune('a' + i)))
	}
	if got := strings.Join(r.strip(), ""); got != "efghijkl" {
		t.Errorf("entries %q", got)
	}
}

// Test 121, "Size": the strip shows the last min(8, max(1, floor(rows /
// 3))) entries, and nothing while it has none; the panel at most max(1,
// floor(rows / 3)) lines, the last reading "… N more" when lines are left
// out.
func TestRunStripAndPanelSizes(t *testing.T) {
	entries := []string{"1", "2", "3", "4", "5", "6", "7", "8"}
	for _, tc := range []struct {
		rows int
		want string
	}{{30, "12345678"}, {24, "12345678"}, {23, "2345678"}, {12, "5678"}, {5, "8"}, {2, "8"}, {1, "8"}} {
		if got := strings.Join(stripLines(entries, tc.rows), ""); got != tc.want {
			t.Errorf("strip at %d rows: %q, want %q", tc.rows, got, tc.want)
		}
	}
	if got := stripLines(nil, 30); len(got) != 0 {
		t.Errorf("an empty strip paints %q", got)
	}

	lines := []string{"a", "b", "c", "d", "e"}
	for _, tc := range []struct {
		rows int
		want []string
	}{
		{30, lines},
		{15, lines},
		{12, []string{"a", "b", "c", "… 2 more"}},
		{9, []string{"a", "b", "… 3 more"}},
		{3, []string{"… 5 more"}},
		{1, []string{"… 5 more"}},
	} {
		if got := panelLines(lines, tc.rows); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("panel at %d rows: %q, want %q", tc.rows, got, tc.want)
		}
	}
	if got := panelLines(nil, 30); len(got) != 0 {
		t.Errorf("an empty panel paints %q", got)
	}
}

// The overlay hook gives the panel always and the strip only while it is
// shown (F12, --strip).
func TestRunOverlayHook(t *testing.T) {
	r := &runner{panel: []string{"warning B003 x"}}
	r.add("save - {} null")
	top, bottom := r.overlay(80, 24)
	if !reflect.DeepEqual(top, []string{"warning B003 x"}) || bottom != nil {
		t.Errorf("hidden strip: top %q bottom %q", top, bottom)
	}
	r.stripOn = true
	if _, bottom := r.overlay(80, 24); !reflect.DeepEqual(bottom, []string{"save - {} null"}) {
		t.Errorf("shown strip: %q", bottom)
	}
}

// A failed reload's panel: a document with errors lists its diagnostics in
// validate's text form; a load or mock error is one "tuimark: …" line.
func TestRunReloadPanel(t *testing.T) {
	dir := t.TempDir()
	file := writeIn(t, dir, "app.tui", runDoc)
	r := &runner{file: file}
	if _, _, err := r.load(true, true); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(`<tui version="1"><screen id="s"><text bogus="1">x</text></screen></tui>`), 0o644); err != nil {
		t.Fatal(err)
	}
	sw, ok := r.reload(false)
	if ok || sw.App != nil {
		t.Fatal("a document with V002 reloaded")
	}
	sw.Applied()
	if len(r.panel) != 1 || !strings.Contains(r.panel[0], "V002") {
		t.Errorf("panel %q, want the V002 line", r.panel)
	}
	if err := os.WriteFile(file, []byte(runDoc), 0o644); err != nil {
		t.Fatal(err)
	}
	r.mockPath = writeIn(t, dir, "mock.json", `{"typo": []}`)
	sw, ok = r.reload(false)
	if ok {
		t.Fatal("a reload with a bad mock succeeded")
	}
	sw.Applied()
	if len(r.panel) != 1 || !strings.HasPrefix(r.panel[0], "tuimark: ") || !strings.Contains(r.panel[0], `"typo" is not an action`) {
		t.Errorf("panel %q, want the mock error", r.panel)
	}
	if err := os.WriteFile(r.mockPath, []byte(`{"save": []}`), 0o644); err != nil {
		t.Fatal(err)
	}
	sw, ok = r.reload(false)
	if !ok || sw.App == nil || !sw.KeepStore {
		t.Fatalf("reload: ok %v app %v keep %v", ok, sw.App != nil, sw.KeepStore)
	}
	sw.Applied()
	if len(r.panel) != 0 || !reflect.DeepEqual(r.strip(), []string{"reload"}) {
		t.Errorf("after a good reload: panel %q strip %q", r.panel, r.strip())
	}
}

// A reload binds --data again only when it changed: the store moves
// otherwise.
func TestRunReloadData(t *testing.T) {
	dir := t.TempDir()
	r := &runner{file: writeIn(t, dir, "app.tui", runDoc), data: writeIn(t, dir, "data.json", `{"saved": "one"}`)}
	if _, _, err := r.load(true, true); err != nil {
		t.Fatal(err)
	}
	sw, ok := r.reload(false)
	if !ok || !sw.KeepStore {
		t.Fatalf("unchanged --data: ok %v keep %v", ok, sw.KeepStore)
	}
	if err := os.WriteFile(r.data, []byte(`{"saved": "two"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	sw, ok = r.reload(true)
	if !ok || sw.KeepStore {
		t.Fatalf("changed --data: ok %v keep %v", ok, sw.KeepStore)
	}
	if got, _ := sw.App.Get("saved"); got != "two" {
		t.Errorf("saved = %v, want the new data", got)
	}
	// Bad data after a change is a failed reload.
	if err := os.WriteFile(r.data, []byte(`{`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.reload(true); ok {
		t.Error("a reload with bad --data succeeded")
	}
}

// writeAtomic replaces path's content in one rename, so the watcher never
// reads half a file.
func writeAtomic(t *testing.T, path, content string) {
	t.Helper()
	tmp := path + ".new"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, path); err != nil {
		t.Fatal(err)
	}
}

// A stylesheet that a failed reload names is watched from then on, even
// when that load failed because the stylesheet does not exist yet: the
// reload that creating it brings succeeds.
func TestRunReloadWatchesAMissingStylesheet(t *testing.T) {
	dir := t.TempDir()
	r := &runner{file: writeIn(t, dir, "app.tui", runDoc)}
	if _, _, err := r.load(true, true); err != nil {
		t.Fatal(err)
	}
	r.sheets = nil
	writeIn(t, dir, "app.tui", strings.Replace(runDoc, "<keymap>", `<style src="new.tcss"/><keymap>`, 1))
	sw, ok := r.reload(false)
	if ok {
		t.Fatal("a reload with a missing stylesheet succeeded")
	}
	sw.Applied()
	sheet := filepath.Join(dir, "new.tcss")
	if !reflect.DeepEqual(r.sheets, []string{sheet}) || !strings.Contains(strings.Join(r.panel, "\n"), "V006") {
		t.Fatalf("sheets %q panel %q, want new.tcss watched and V006 shown", r.sheets, r.panel)
	}
	if w := r.watched(); !reflect.DeepEqual(w[:2], []string{r.file, sheet}) {
		t.Errorf("watched %q", w)
	}
	writeIn(t, dir, "new.tcss", "text { bold: true; }\n")
	if _, ok := r.reload(false); !ok {
		t.Errorf("the reload after creating the stylesheet failed: %q", r.panel)
	}
}

// The watcher hands one reload per change it sees, and waits for the
// loop to take it.
func TestRunWatchHandsReloads(t *testing.T) {
	dir := t.TempDir()
	r := &runner{file: writeIn(t, dir, "app.tui", runDoc)}
	if _, _, err := r.load(true, true); err != nil {
		t.Fatal(err)
	}
	r.seen = stamps(r.watched())
	swaps := make(chan host.HostSwap)
	done := make(chan struct{})
	defer close(done)
	go r.watch(swaps, done)
	next := func() host.HostSwap {
		t.Helper()
		select {
		case sw := <-swaps:
			return sw
		case <-time.After(5 * time.Second):
			t.Fatal("no reload")
		}
		return host.HostSwap{}
	}
	// An edit that breaks the document is a failed reload; the fix is a
	// change of its own, and a successful one.
	writeAtomic(t, r.file, "<tui version=\"1\"><screen id=\"s\"><text>x</screen></tui>")
	if sw := next(); sw.App != nil {
		t.Fatal("a V005 document reloaded")
	}
	writeAtomic(t, r.file, runDoc)
	if sw := next(); sw.App == nil {
		t.Fatal("the fix did not reload")
	}
}

// A regular --events file is created 0600, and an existing one is
// truncated and narrowed to 0600.
func TestRunEventsFileMode(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "ev.ndjson")
	f, err := host.OpenRecordFile(p, "the event log")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if st, err := os.Stat(p); err != nil || st.Mode().Perm() != 0o600 {
		t.Errorf("created with %v (%v), want 0600", st.Mode().Perm(), err)
	}
	if err := os.WriteFile(p, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, 0o644); err != nil {
		t.Fatal(err)
	}
	f, err = host.OpenRecordFile(p, "the event log")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	st, _ := os.Stat(p)
	if st.Mode().Perm() != 0o600 || st.Size() != 0 {
		t.Errorf("existing file: mode %v size %d, want 0600 and empty", st.Mode().Perm(), st.Size())
	}
}
