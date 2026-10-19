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

// sample.json is the store the host builds at start with the default seed
// and flags, derived paths included, and mock.json gives every action
// literal effects for `tuimark run --mock`. Both are generated from the
// code, so they cannot drift: TestSampleInSync fails when they differ, and
//
//	UPDATE_SAMPLE=1 go test ./examples/2048 -run TestSampleInSync
//
// rewrites them. The tuimark CLI runs no Go host and no Derive, so it reads
// the real start state from sample.json.

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

// storeOf is the whole store of a, through JSON.
func storeOf(t *testing.T, a *app) map[string]any {
	t.Helper()
	v, ok := a.ui.Get("")
	if !ok {
		t.Fatal("Get(\"\") found no store")
	}
	return normalize(t, v).(map[string]any)
}

func startApp(t *testing.T) *app {
	t.Helper()
	a, err := load(".", defaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// call runs the handler of an action, as the runtime does for a key or a
// click.
func call(t *testing.T, a *app, action string) {
	t.Helper()
	h := a.handlers()[action]
	if h == nil {
		t.Fatalf("no handler for %s", action)
	}
	if err := h(tuimark.Event{}); err != nil {
		t.Fatalf("%s: %v", action, err)
	}
}

// documentReads is the set of store paths app.tui reads: the first segment
// of every {binding}, if/each/bind/open/checked/key value, and class:NAME
// guard, minus the aliases of its each loops. A mock that writes a path
// outside it is writing something nothing shows.
func documentReads(t *testing.T) map[string]bool {
	t.Helper()
	src, err := os.ReadFile("app.tui")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	aliases := map[string]bool{}
	for _, m := range regexp.MustCompile(`\bas (\w+)"`).FindAllStringSubmatch(text, -1) {
		aliases[m[1]] = true
	}
	var paths []string
	for _, m := range regexp.MustCompile(`\{([^}]+)\}`).FindAllStringSubmatch(text, -1) {
		paths = append(paths, m[1])
	}
	for _, m := range regexp.MustCompile(`(?:\b(?:if|each|bind|open|checked|key)|class:[\w-]+)="!?([^" ]+)`).FindAllStringSubmatch(text, -1) {
		paths = append(paths, m[1])
	}
	reads := map[string]bool{}
	for _, p := range paths {
		root := strings.SplitN(p, ".", 2)[0]
		if !aliases[root] {
			reads[root] = true
		}
	}
	return reads
}

// mockData applies literal values only (tuimark run has no Go host and no
// Derive), so each action carries what the real handler visibly does to the
// store, measured against the state just before the action: every
// top-level key whose value differs. The runtime moves focus into a modal
// and out of it by itself, so no @focus write is needed. An action that
// undoes or closes another is measured after the one it undoes ran first.
func mockData(t *testing.T) map[string]any {
	t.Helper()
	moves := func(a *app) {
		// Any slide that moves something.
		for _, d := range []string{"left", "down", "right", "up"} {
			before := a.g.moves
			call(t, a, d)
			if a.g.moves != before {
				return
			}
		}
		t.Fatal("no slide moves the sample board")
	}
	reads := documentReads(t)
	mock := map[string]any{}
	for _, c := range []struct {
		action string
		setup  func(a *app) // brings the app to the state the action runs in
	}{
		{"up", nil},
		{"right", nil},
		{"down", nil},
		{"left", nil},
		{"undo", moves},
		{"restart", moves},
		{"autopilot", nil},
		{"help_open", nil},
		{"help_close", func(a *app) { call(t, a, "help_open") }},
		{"keep_going", func(a *app) {
			setBoard(a, [][]int{{1024, 1024, 0, 0}, {}, {}, {}})
			call(t, a, "left")
			if !a.winOpen {
				t.Fatal("two 1024s did not open the win modal")
			}
		}},
		{"over_close", func(a *app) {
			setBoard(a, stuckRows())
			call(t, a, "right")
			if !a.overOpen {
				t.Fatal("the stuck board did not open the game over modal")
			}
		}},
	} {
		a := startApp(t)
		if c.setup != nil {
			c.setup(a)
		}
		before := storeOf(t, a)
		call(t, a, c.action)
		after := storeOf(t, a)
		var ops []any
		for _, k := range sortedKeys(after) {
			// The derived board is what the view reads; the raw matrices it is
			// derived from are not, and `run --mock` runs no Derive.
			if !reads[k] {
				continue
			}
			if !reflect.DeepEqual(before[k], after[k]) {
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

// compact writes a JSON object with one member per line and every array of
// objects one element per line, so the files diff and read well.
func compact(t *testing.T, v map[string]any) []byte {
	t.Helper()
	var b bytes.Buffer
	keys := sortedKeys(v)
	b.WriteString("{\n")
	for i, k := range keys {
		kb, _ := json.Marshal(k)
		b.WriteString("  " + string(kb) + ": ")
		b.Write(compactValue(t, v[k], "  "))
		if i < len(keys)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("}\n")
	return b.Bytes()
}

// compactValue writes an array whose elements are objects or arrays, or the
// mock's action lists, one element per line; anything else on one line.
func compactValue(t *testing.T, v any, indent string) []byte {
	t.Helper()
	list, ok := v.([]any)
	if !ok || len(list) == 0 {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	switch list[0].(type) {
	case map[string]any, []any:
	default:
		b, _ := json.Marshal(v)
		return b
	}
	var b bytes.Buffer
	b.WriteString("[\n")
	for j, e := range list {
		eb, _ := json.Marshal(e)
		b.WriteString(indent + "  " + string(eb))
		if j < len(list)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString(indent + "]")
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
		t.Errorf("%s differs from the code's output; run UPDATE_SAMPLE=1 go test ./examples/2048 -run TestSampleInSync", name)
	}
}

func TestSampleInSync(t *testing.T) {
	checkFile(t, "sample.json", storeOf(t, startApp(t)))
	checkFile(t, "mock.json", mockData(t))
}

// sample.json is the opening store: the documented state a reader sees
// first, with every derived path present.
func TestSampleIsTheOpeningBoard(t *testing.T) {
	raw, err := os.ReadFile("sample.json")
	if err != nil {
		t.Fatal(err)
	}
	var s map[string]any
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"board", "dims", "size_label", "gain_label", "state", "mode_label", "tiles", "hist"} {
		if _, ok := s[k]; !ok {
			t.Errorf("sample.json lacks %q", k)
		}
	}
	tiles := 0
	for _, row := range s["tiles"].([]any) {
		for _, v := range row.([]any) {
			if v.(float64) > 0 {
				tiles++
			}
		}
	}
	if tiles != 2 || s["score"] != float64(0) || s["moves"] != float64(0) {
		t.Errorf("opening board has %d tiles, score %v, moves %v; want 2, 0, 0", tiles, s["score"], s["moves"])
	}
}

// mock.json writes only paths app.tui reads, and the document really does
// not read the raw matrices (so leaving them out is not hiding a gap).
func TestMockWritesOnlyWhatTheDocumentReads(t *testing.T) {
	reads := documentReads(t)
	for _, k := range []string{"board", "score", "hist", "win_open", "over_open", "help_open", "auto", "mode_label", "gain_label", "state", "dims", "largest", "moves", "best", "notice", "size_label"} {
		if !reads[k] {
			t.Errorf("the document does not read %q: documentReads misses something", k)
		}
	}
	store := storeOf(t, startApp(t))
	for _, k := range []string{"tiles", "pop", "fresh", "gain", "size"} {
		if _, ok := store[k]; !ok {
			t.Fatalf("the store has no %q", k)
		}
		if reads[k] {
			t.Errorf("the document reads %q", k)
		}
	}
	for action, ops := range mockData(t) {
		for _, op := range ops.([]any) {
			path := op.(map[string]any)["path"].(string)
			if !reads[path] {
				t.Errorf("%s writes %q, which app.tui never reads", action, path)
			}
		}
	}
	// A slide shows through the derived board, which the mock carries.
	found := false
	for _, op := range mockData(t)["left"].([]any) {
		if op.(map[string]any)["path"] == "board" {
			found = true
		}
	}
	if !found {
		t.Error("the left mock does not write the board")
	}
}

// Every action mock.json names is one the document fires and the host
// handles, every host action of the document is mocked, and a close action
// writes its flag false.
func TestMockActionsAreDocumentActionsWithHandlers(t *testing.T) {
	raw, err := os.ReadFile("mock.json")
	if err != nil {
		t.Fatal(err)
	}
	var mock map[string][]struct {
		Path  string `json:"path"`
		Value any    `json:"value"`
	}
	if err := json.Unmarshal(raw, &mock); err != nil {
		t.Fatal(err)
	}
	a := startApp(t)
	doc := map[string]bool{}
	for _, spec := range a.ui.Catalog() {
		if spec.Builtin {
			if _, ok := mock[spec.Name]; ok {
				t.Errorf("mock.json mocks %q, a built-in", spec.Name)
			}
			continue
		}
		doc[spec.Name] = true
		if !spec.Registered {
			t.Errorf("the document names %q, which has no Go handler", spec.Name)
		}
		if _, ok := mock[spec.Name]; !ok && spec.Name != "quit" {
			t.Errorf("the document names %q, which mock.json does not mock", spec.Name)
		}
	}
	for name := range mock {
		if !doc[name] {
			t.Errorf("mock.json mocks %q, which the document does not name", name)
		}
	}
	writes := func(action, path string) any {
		for _, w := range mock[action] {
			if w.Path == path {
				return w.Value
			}
		}
		return "not written"
	}
	// Every modal of the document: whatever it closes with (on:escape or
	// on:close) writes the flag its open= reads as false.
	src, err := os.ReadFile("app.tui")
	if err != nil {
		t.Fatal(err)
	}
	modals := regexp.MustCompile(`<modal\b[^>]*>`).FindAllString(string(src), -1)
	if len(modals) != 3 {
		t.Fatalf("found %d modals in app.tui, want 3", len(modals))
	}
	for _, m := range modals {
		flag := regexp.MustCompile(`\bopen="([^"]+)"`).FindStringSubmatch(m)
		if flag == nil {
			t.Fatalf("%s has no open= flag", m)
		}
		closers := regexp.MustCompile(`\bon:(?:escape|close)="([^"]+)"`).FindAllStringSubmatch(m, -1)
		if len(closers) == 0 {
			t.Fatalf("%s has no way to close", m)
		}
		for _, c := range closers {
			if got := writes(c[1], flag[1]); got != false {
				t.Errorf("%s writes %s = %v, want false", c[1], flag[1], got)
			}
		}
	}
	if got := writes("help_open", "help_open"); got != true {
		t.Errorf("help_open writes help_open = %v, want true", got)
	}
}
