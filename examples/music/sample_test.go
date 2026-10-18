package main

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

// sample.json and mock.json are generated, so they cannot drift from the
// host: TestSampleInSync fails when they differ, and
//
//	UPDATE_SAMPLE=1 go test ./examples/music -run TestSampleInSync
//
// rewrites them.
//
// sample.json is the whole store the app builds at its documented point:
// the default seed (7), "Tessellate" playing at 1:12 with "Window Seat
// Aurora" and "Soft Alloy" queued, then 24 ticks of 100 ms of a fake clock
// from 2026-10-19 21:00:00 UTC, derived paths included. `tuimark validate`,
// `dump`, and `run` have no Go host and no Derive, so they read these
// literal values.
//
// mock.json answers each action of `tuimark run --mock` with the literal
// writes the real handler makes from that same state: the store paths that
// differ after the action, found by running it.

// newApp is the wiring run does before Run: load, then register.
func newApp(t testing.TB) *app {
	t.Helper()
	a, err := load(".", defaultSeed)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.register(); err != nil {
		t.Fatal(err)
	}
	return a
}

// normalize round-trips v through JSON so every number is a float64 and
// every container a []any or map[string]any.
func normalize(t testing.TB, v any) any {
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

func sampleData(t testing.TB) map[string]any {
	t.Helper()
	v, _ := newApp(t).ui.Get("")
	return normalize(t, v).(map[string]any)
}

// change is one literal write of a mock answer.
type change = map[string]any

// diffStore appends a write for every path where got differs from want,
// descending into objects and writing arrays whole. The tab and the
// cursors are the document's own business (built-in actions), so they are
// left out.
func diffStore(prefix string, want, got any, out *[]any) {
	wm, wok := want.(map[string]any)
	gm, gok := got.(map[string]any)
	if wok && gok {
		keys := map[string]bool{}
		for k := range wm {
			keys[k] = true
		}
		for k := range gm {
			keys[k] = true
		}
		var sorted []string
		for k := range keys {
			sorted = append(sorted, k)
		}
		sort.Strings(sorted)
		for _, k := range sorted {
			if prefix == "" && (k == "tab" || k == "cur") {
				continue
			}
			p := k
			if prefix != "" {
				p = prefix + "." + k
			}
			diffStore(p, wm[k], gm[k], out)
		}
		return
	}
	if !reflect.DeepEqual(want, got) {
		*out = append(*out, change{"path": prefix, "value": got})
	}
}

// mockSteps are the keys that fire each action from the sample state. The
// last key fires the action; the ones before it only set the scene. skip
// names top-level paths left out of the answer: the host derives them, so
// `run` (which has no Derive) never changed them while the scene was set.
var mockSteps = []struct {
	action string
	keys   []string
	skip   []string
}{
	{action: "toggle_play", keys: []string{"space"}},
	{action: "next", keys: []string{"n"}},
	{action: "prev", keys: []string{"p"}},
	{action: "toggle_shuffle", keys: []string{"s"}},
	{action: "cycle_repeat", keys: []string{"r"}},
	{action: "vol_up", keys: []string{"+"}},
	{action: "vol_down", keys: []string{"-"}},
	{action: "seek_fwd", keys: []string{"right"}},
	{action: "seek_back", keys: []string{"left"}},
	{action: "lib_play", keys: []string{"enter"}},
	{action: "lib_add", keys: []string{"a"}},
	{action: "q_play", keys: []string{"2", "enter"}},
	{action: "q_remove", keys: []string{"2", "x"}},
	{action: "q_clear", keys: []string{"2", "c"}},
	{action: "album_play", keys: []string{"3", "enter"}},
	{action: "album_add", keys: []string{"3", "a"}},
	{action: "filter_apply", keys: []string{"/", "enter"}},
	// The filter holds text when esc clears it. Under `run` it narrowed
	// nothing, so the rows and counts the scene changed are not written.
	{action: "filter_clear", keys: []string{"/", "text:ona", "esc"}, skip: []string{"lib_rows", "counts"}},
}

// focused is the id of the focused node, or "".
func focused(res *tuimark.PlayResult) string {
	if res.Dump.Focus == nil {
		return ""
	}
	return *res.Dump.Focus
}

// mockData answers each action with the writes its handler makes: the
// store paths that differ across the last key (the state just before the
// action against the state after it), plus a write to "@focus" when the
// handler moved the focus, which a store diff cannot see. A modal's
// open/close actions are two plain writes of its flag.
func mockData(t testing.TB) map[string]any {
	t.Helper()
	mock := map[string]any{}
	for _, s := range mockSteps {
		a := newApp(t)
		opts := tuimark.PlayOptions{Cols: 100, Rows: 24}
		scene, last := s.keys[:len(s.keys)-1], s.keys[len(s.keys)-1:]
		before, err := a.ui.Play(opts, scene...)
		if err != nil {
			t.Fatalf("%s: %v", s.action, err)
		}
		v, _ := a.ui.Get("")
		prev := normalize(t, v)
		res, err := a.ui.Play(opts, last...)
		if err != nil {
			t.Fatalf("%s: %v", s.action, err)
		}
		fired := false
		for _, e := range res.Events {
			fired = fired || e.Action == s.action
		}
		if !fired {
			t.Fatalf("%v did not fire %s: %+v", s.keys, s.action, res.Events)
		}
		v, _ = a.ui.Get("")
		var diff []any
		diffStore("", prev, normalize(t, v), &diff)
		changes := []any{}
		for _, c := range diff {
			path := c.(change)["path"].(string)
			if !skipped(path, s.skip) {
				changes = append(changes, c)
			}
		}
		if f := focused(res); f != "" && f != focused(before) {
			changes = append(changes, change{"path": "@focus", "value": "#" + f})
		}
		mock[s.action] = changes
	}
	mock["help_open"] = []any{change{"path": "help_open", "value": true}}
	mock["help_close"] = []any{change{"path": "help_open", "value": false}}
	return mock
}

// skipped reports whether path is one of the skipped top-level paths, or
// below one.
func skipped(path string, skip []string) bool {
	for _, s := range skip {
		if path == s || strings.HasPrefix(path, s+".") {
			return true
		}
	}
	return false
}

// compact writes a JSON object with one member per line and every array of
// objects with one element per line, so the files diff and read well.
func compact(t testing.TB, v map[string]any) []byte {
	t.Helper()
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b bytes.Buffer
	enc := func(x any) []byte {
		var eb bytes.Buffer
		e := json.NewEncoder(&eb)
		e.SetEscapeHTML(false)
		if err := e.Encode(x); err != nil {
			t.Fatal(err)
		}
		return bytes.TrimRight(eb.Bytes(), "\n")
	}
	b.WriteString("{\n")
	for i, k := range keys {
		b.WriteString("  ")
		b.Write(enc(k))
		b.WriteString(": ")
		if list, ok := v[k].([]any); ok && len(list) > 0 {
			if _, objs := list[0].(map[string]any); objs {
				b.WriteString("[\n")
				for j, e := range list {
					b.WriteString("    ")
					b.Write(enc(e))
					if j < len(list)-1 {
						b.WriteString(",")
					}
					b.WriteString("\n")
				}
				b.WriteString("  ]")
				goto next
			}
		}
		b.Write(enc(v[k]))
	next:
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
		t.Errorf("%s differs from what the host builds; run UPDATE_SAMPLE=1 go test ./examples/music -run TestSampleInSync", name)
	}
}

func TestSampleInSync(t *testing.T) {
	checkFile(t, "sample.json", sampleData(t))
	checkFile(t, "mock.json", mockData(t))
}

// The derived paths are in the sample, and the documented point is the
// state the sample shows.
func TestSampleShape(t *testing.T) {
	s := sampleData(t)
	for _, k := range []string{"lib_rows", "queue_rows", "album_rows", "counts", "now", "vol", "player", "queue", "spectrum", "loud"} {
		if _, ok := s[k]; !ok {
			t.Errorf("sample has no %q", k)
		}
	}
	player := s["player"].(map[string]any)
	if player["current"] != "t02" || player["playing"] != true {
		t.Errorf("player = %v, want Tessellate playing", player)
	}
	if got := s["now"].(map[string]any)["time"]; got != "1:12 / 4:12" {
		t.Errorf("now.time = %v, want 1:12 / 4:12", got)
	}
}

// mock.json answers exactly the host actions the document names: no stale
// name, and no action left without an answer.
func TestMockNamesRealActions(t *testing.T) {
	a := newApp(t)
	raw, err := os.ReadFile("mock.json")
	if err != nil {
		t.Fatal(err)
	}
	var mock map[string]any
	if err := json.Unmarshal(raw, &mock); err != nil {
		t.Fatal(err)
	}
	named := map[string]bool{}
	for _, spec := range a.ui.Catalog() {
		if spec.Builtin {
			continue
		}
		named[spec.Name] = true
		if _, ok := mock[spec.Name]; !ok && spec.Name != "quit" {
			t.Errorf("mock.json has no answer for %s", spec.Name)
		}
	}
	for name := range mock {
		if !named[name] {
			t.Errorf("mock.json answers %q, which no key or button fires", name)
		}
	}
}

func readMock(t *testing.T) map[string][]map[string]any {
	t.Helper()
	raw, err := os.ReadFile("mock.json")
	if err != nil {
		t.Fatal(err)
	}
	var mock map[string][]map[string]any
	if err := json.Unmarshal(raw, &mock); err != nil {
		t.Fatal(err)
	}
	return mock
}

func writes(mock map[string][]map[string]any, action, path string) (any, bool) {
	for _, w := range mock[action] {
		if w["path"] == path {
			return w["value"], true
		}
	}
	return nil, false
}

// An action that a modal fires on escape or close answers by writing the modal's open flag false, so
// under `run --mock` esc and the button really close it.
func TestMockClosesTheModal(t *testing.T) {
	a := newApp(t)
	mock := readMock(t)
	found := 0
	for _, spec := range a.ui.Catalog() {
		closes := false
		for _, src := range spec.Sources {
			closes = closes || strings.HasSuffix(src, " on:escape") || strings.HasSuffix(src, " on:close")
		}
		if !closes {
			continue
		}
		found++
		if v, ok := writes(mock, spec.Name, "help_open"); !ok || v != false {
			t.Errorf("%s closes the modal, but mock.json writes help_open = %v (written: %v)", spec.Name, v, ok)
		}
	}
	if found == 0 {
		t.Error("no modal-close action found in the catalog")
	}
}

// The filter's actions move the focus to the table, as the handlers do;
// a mock write to "@focus" does the same under `run`.
func TestMockMovesTheFocus(t *testing.T) {
	mock := readMock(t)
	for _, action := range []string{"filter_apply", "filter_clear"} {
		if v, ok := writes(mock, action, "@focus"); !ok || v != "#lib" {
			t.Errorf("%s: @focus = %v (written: %v), want #lib", action, v, ok)
		}
	}
	if v, ok := writes(mock, "filter_clear", "filter"); !ok || v != "" {
		t.Errorf("filter_clear: filter = %v (written: %v), want the empty string", v, ok)
	}
	// Derived rows the scene changed stay out of the answer.
	for _, path := range []string{"lib_rows", "counts"} {
		if _, ok := writes(mock, "filter_clear", path); ok {
			t.Errorf("filter_clear writes the derived %s", path)
		}
	}
}

// Every action that moves the focus in the real app has a "@focus" write,
// and the generator finds it by playing the action, not from a list.
func TestMockFocusWritesMatchTheHandlers(t *testing.T) {
	mock := readMock(t)
	for name, ws := range mock {
		for _, w := range ws {
			if w["path"] == "@focus" && name != "filter_apply" && name != "filter_clear" {
				t.Errorf("%s writes @focus %v, but its handler does not move the focus", name, w["value"])
			}
		}
	}
}
