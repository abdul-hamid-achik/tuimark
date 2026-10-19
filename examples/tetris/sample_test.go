package main

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

// sample.json is the store the host builds at start with the default seed,
// derived paths included, and mock.json gives every action literal effects
// for `tuimark run --mock`. Both are generated from the code, so they cannot
// drift: TestSampleInSync fails when they differ, and
//
//	UPDATE_SAMPLE=1 go test ./examples/tetris -run TestSampleInSync
//
// rewrites them. The tuimark CLI runs no Go host and no Derive, so it reads
// the real start state from sample.json. That state is a new game with the
// default seed, level 1, the manual mode, before the first tick.

func newApp(t *testing.T) *app {
	t.Helper()
	a, err := load(".", defaultSeed, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func store(t *testing.T, a *app) map[string]any {
	t.Helper()
	v, ok := a.ui.Get("")
	m, isMap := v.(map[string]any)
	if !ok || !isMap {
		t.Fatalf("Get(\"\") = %v, %v", v, ok)
	}
	return normalize(t, m).(map[string]any)
}

// normalize round-trips v through JSON so numbers compare as float64.
func normalize(t *testing.T, v any) any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func jsonEqual(a, b any) bool {
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return bytes.Equal(ab, bb)
}

// call runs the handler of an action, as the runtime does for a key.
func call(t *testing.T, a *app, action string) {
	t.Helper()
	h := a.handlers()[action]
	if h == nil {
		t.Fatalf("no handler for %s", action)
	}
	if err := h(tuimark.Event{Action: action}); err != nil {
		t.Fatalf("%s: %v", action, err)
	}
}

// viewPaths are the top-level store paths app.tui reads: in {path} bindings,
// if=, class:NAME= guards, each=, bind=, and open=, less the aliases of the
// each templates. `tuimark run --mock` has no Derive, so a mock is only worth
// writing for these; the raw paths the Go host keeps for itself (the field,
// the falling piece, the queue, ...) are read by Derive alone.
func viewPaths(t *testing.T) map[string]bool {
	t.Helper()
	b, err := os.ReadFile("app.tui")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	paths, aliases := map[string]bool{}, map[string]bool{}
	for _, m := range regexp.MustCompile(`\bas ([a-z_][a-z0-9_]*)"`).FindAllStringSubmatch(src, -1) {
		aliases[m[1]] = true
	}
	for _, re := range []string{
		`\{([a-z_][a-z0-9_]*)[.}]`,
		`\bif="!?([a-z_][a-z0-9_]*)`,
		`\bclass:[a-z0-9_-]+="!?([a-z_][a-z0-9_]*)`,
		`\beach="([a-z_][a-z0-9_]*) as`,
		`\bbind="([a-z_][a-z0-9_]*)`,
		`\bopen="!?([a-z_][a-z0-9_]*)`,
	} {
		for _, m := range regexp.MustCompile(re).FindAllStringSubmatch(src, -1) {
			if !aliases[m[1]] {
				paths[m[1]] = true
			}
		}
	}
	if len(paths) < 15 {
		t.Fatalf("found only %d view paths: %v", len(paths), paths)
	}
	return paths
}

// mockData applies literal values only: no Go host runs under
// `tuimark run`, so each action carries what the real handler visibly does to
// the store, measured against the state just before the action: every
// top-level key the view reads whose value differs. An action that undoes
// another (help_close) is measured after the one it undoes ran, restart and
// over_hide are measured from a game that is over (restart reopens the board
// and closes the modal it covered), and hard_drop from a game with the hold
// used, so it undims the held piece. Pause and autopilot are toggles the
// mock can only do one way.
func mockData(t *testing.T) map[string]any {
	t.Helper()
	mock := map[string]any{}
	read := viewPaths(t)
	for _, c := range []struct {
		action string
		setup  func(a *app)
	}{
		{"left", nil},
		{"right", nil},
		{"soft_drop", nil},
		{"rotate", nil},
		{"rotate_back", nil},
		{"hard_drop", func(a *app) { call(t, a, "hold") }},
		{"hold", nil},
		{"pause", nil},
		{"autopilot", nil},
		{"restart", func(a *app) { topOut(t, a) }},
		{"over_hide", func(a *app) { topOut(t, a) }},
		{"help_open", nil},
		{"help_close", func(a *app) { call(t, a, "help_open") }},
	} {
		a := newApp(t)
		if c.setup != nil {
			c.setup(a)
		}
		before := store(t, a)
		call(t, a, c.action)
		after := store(t, a)
		var ops []any
		for _, k := range sortedKeys(after) {
			if read[k] && !jsonEqual(before[k], after[k]) {
				ops = append(ops, map[string]any{"path": k, "value": after[k]})
			}
		}
		if len(ops) == 0 {
			t.Fatalf("%s changed nothing: it has no visible effect to mock", c.action)
		}
		mock[c.action] = ops
	}
	return mock
}

// topOut stacks pieces in one column until the game is over.
func topOut(t *testing.T, a *app) {
	t.Helper()
	for i := 0; !a.g.over && i < 100; i++ {
		call(t, a, "hard_drop")
	}
	if !a.g.over {
		t.Fatal("dropping pieces in one column never ended the game")
	}
}

// compact writes a JSON object with one member per line and every array
// of objects one element per line, so the files diff and read well.
func compact(t *testing.T, v map[string]any) []byte {
	t.Helper()
	var b bytes.Buffer
	b.WriteString("{\n")
	keys := sortedKeys(v)
	for i, k := range keys {
		kb, _ := json.Marshal(k)
		b.WriteString("  " + string(kb) + ": ")
		x, isList := v[k].([]any)
		if isList && len(x) > 1 {
			if _, isObj := x[0].(map[string]any); isObj {
				b.WriteString("[\n")
				for j, e := range x {
					eb, _ := json.Marshal(e)
					b.WriteString("    " + string(eb))
					if j < len(x)-1 {
						b.WriteString(",")
					}
					b.WriteString("\n")
				}
				b.WriteString("  ]")
			} else {
				vb, _ := json.Marshal(x)
				b.Write(vb)
			}
		} else {
			vb, _ := json.Marshal(v[k])
			b.Write(vb)
		}
		if i < len(keys)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("}\n")
	return b.Bytes()
}

func checkFile(t *testing.T, name string, want map[string]any) {
	t.Helper()
	if os.Getenv("UPDATE_SAMPLE") != "" {
		if err := os.WriteFile(name, compact(t, want), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	raw, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if !reflect.DeepEqual(got, normalize(t, want)) {
		t.Errorf("%s differs from what the app builds; run UPDATE_SAMPLE=1 go test ./examples/tetris -run TestSampleInSync", name)
	}
}

func TestSampleInSync(t *testing.T) {
	checkFile(t, "sample.json", store(t, newApp(t)))
	checkFile(t, "mock.json", mockData(t))
}

// The sample is a fresh game: a piece in the top rows, five pieces queued,
// nothing held, nothing scored, and every derived path present, because the
// CLI runs no Derive.
func TestSampleIsAFreshGame(t *testing.T) {
	raw, err := os.ReadFile("sample.json")
	if err != nil {
		t.Fatal(err)
	}
	var s map[string]any
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"board", "hold_rows", "next_rows", "state", "mode_label", "speed_label"} {
		if _, ok := s[k]; !ok {
			t.Errorf("sample.json lacks the derived path %q", k)
		}
	}
	if q, _ := s["queue"].([]any); len(q) != queueLen {
		t.Errorf("queue = %v, want %d pieces", s["queue"], queueLen)
	}
	if s["score"] != 0.0 || s["lines"] != 0.0 || s["hold"] != "" || s["over"] != false || s["paused"] != false {
		t.Errorf("sample is not a fresh game: %v", s)
	}
	cur, _ := s["cur"].(map[string]any)
	if cells, _ := cur["cells"].([]any); len(cells) != 4 || cur["kind"] == "" {
		t.Errorf("cur = %v, want a piece of four cells", cur)
	}
	if rows, _ := s["field"].([]any); len(rows) != boardH {
		t.Errorf("field has %d rows, want %d", len(rows), boardH)
	}
}

// Every action that closes a modal writes its flag false in the mock, so
// `tuimark run --mock` can close what it opened; opening writes it true.
func TestMockClosesTheModals(t *testing.T) {
	mock := mockData(t)
	writes := func(action, path string) any {
		ops, _ := mock[action].([]any)
		for _, op := range ops {
			m := op.(map[string]any)
			if m["path"] == path {
				return m["value"]
			}
		}
		return nil
	}
	if got := writes("help_open", "help_open"); got != true {
		t.Errorf("help_open writes help_open = %v, want true", got)
	}
	if got := writes("help_close", "help_open"); got != false {
		t.Errorf("help_close writes help_open = %v, want false", got)
	}
	for _, action := range []string{"restart", "over_hide"} {
		if got := writes(action, "over_modal"); got != false {
			t.Errorf("%s writes over_modal = %v, want false: the game over modal would stay", action, got)
		}
	}
	if got := writes("restart", "over"); got != false {
		t.Errorf("restart writes over = %v, want false", got)
	}
	// Every modal of the document: whatever closes it writes the flag its
	// open= reads as false. The game over modal closes with its restart
	// button; the help modal with escape and its close button.
	src, err := os.ReadFile("app.tui")
	if err != nil {
		t.Fatal(err)
	}
	modalRe := regexp.MustCompile(`(?s)<modal\b[^>]*>.*?</modal>`)
	modals := modalRe.FindAllString(string(src), -1)
	if len(modals) != 2 {
		t.Fatalf("found %d modals in app.tui, want 2", len(modals))
	}
	for _, m := range modals {
		head := regexp.MustCompile(`<modal\b[^>]*>`).FindString(m)
		flag := regexp.MustCompile(`\bopen="([^"]+)"`).FindStringSubmatch(head)
		if flag == nil {
			t.Fatalf("%s has no open= flag", head)
		}
		var closers []string
		for _, c := range regexp.MustCompile(`\bon:(?:escape|close|click)="([^"]+)"`).FindAllStringSubmatch(m, -1) {
			if c[1] != "quit" {
				closers = append(closers, c[1])
			}
		}
		if len(closers) == 0 {
			t.Fatalf("%s has no way to close", head)
		}
		for _, c := range closers {
			if got := writes(c, flag[1]); got != false {
				t.Errorf("%s writes %s = %v, want false", c, flag[1], got)
			}
		}
	}
}

// The mock writes only paths the view reads (a raw path only Derive reads
// would do nothing under `run --mock`), and every path the view reads is in
// the sample, so nothing binds to a missing path.
func TestMockWritesWhatTheViewReads(t *testing.T) {
	read := viewPaths(t)
	raw, err := os.ReadFile("mock.json")
	if err != nil {
		t.Fatal(err)
	}
	var mock map[string][]struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(raw, &mock); err != nil {
		t.Fatal(err)
	}
	for action, ops := range mock {
		for _, op := range ops {
			if !read[op.Path] {
				t.Errorf("mock %s writes %q, which app.tui never reads", action, op.Path)
			}
		}
	}
	sample := store(t, newApp(t))
	for p := range read {
		if _, ok := sample[p]; !ok {
			t.Errorf("app.tui reads %q, which the store lacks", p)
		}
	}
	// hard_drop undims the hold the mock's hold dimmed.
	var hardDrop []string
	for _, op := range mock["hard_drop"] {
		hardDrop = append(hardDrop, op.Path)
	}
	if !strings.Contains(strings.Join(hardDrop, " "), "hold_rows") {
		t.Errorf("hard_drop mock writes %v: it leaves the dimmed hold dim", hardDrop)
	}
}

// Every action mock.json answers is an action of the document that the host
// (not the runtime) handles, and every host action of the document but quit
// has a mock.
func TestMockActionsAreHostActions(t *testing.T) {
	a := newApp(t)
	doc := map[string]tuimark.ActionSpec{}
	for _, spec := range a.ui.Catalog() {
		doc[spec.Name] = spec
	}
	mock := mockData(t)
	for name := range mock {
		spec, ok := doc[name]
		switch {
		case !ok:
			t.Errorf("mock.json mocks %q, which the document does not name", name)
		case spec.Builtin:
			t.Errorf("mock.json mocks %q, a built-in", name)
		case !spec.Registered:
			t.Errorf("mock.json mocks %q, which has no Go handler", name)
		}
	}
	for _, spec := range doc {
		if !spec.Builtin && spec.Name != "quit" {
			if _, ok := mock[spec.Name]; !ok {
				t.Errorf("the document names %q, which mock.json does not mock", spec.Name)
			}
		}
	}
}
