package main

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

// sample.json and mock.json are generated, so they cannot drift from the
// host: TestSampleInSync fails when they differ, and
//
//	UPDATE_SAMPLE=1 go test ./examples/chess -run TestSampleInSync
//
// rewrites them.
//
// sample.json is the whole store the app builds at its start with no flags:
// you (white) against the computer at level 3, the starting position, no
// clock, derived paths included. `tuimark validate`, `dump`, and `run` have
// no Go host and no Derive, so they read these literal values.
//
// mock.json answers each action of `tuimark run --mock` with the literal
// writes the real handler makes from a state that fits it: the store paths
// that differ after the action, found by running it.

func sampleData(t testing.TB) map[string]any {
	t.Helper()
	v, _ := newTestApp(t).ui.Get("")
	return normalize(t, v).(map[string]any)
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

// change is one literal write of a mock answer.
type change = map[string]any

// diffStore appends a write for every path where got differs from want,
// descending into objects and writing arrays whole.
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

// mockSteps are the keys that fire each action. setup puts the app in a
// state where the action does something worth answering with; the keys
// before the last only set the scene, and the last key fires the action.
var mockSteps = []struct {
	action string
	setup  func(*testApp)
	keys   []string
}{
	{action: "up", keys: []string{"up"}},
	{action: "down", keys: []string{"down"}},
	{action: "left", keys: []string{"left"}},
	{action: "right", keys: []string{"right"}},
	{action: "first", keys: []string{"home"}},
	{action: "last", keys: []string{"end"}},
	{action: "select", keys: []string{"down", "up", "enter"}},
	{action: "cell", keys: []string{"click:e2"}},
	{action: "cmd_submit", keys: []string{":", "text:e4", "enter"}},
	{action: "cmd_cancel", keys: []string{":", "text:Nf", "esc"}},
	{action: "undo", keys: []string{":", "text:e4", "enter", "u"}},
	{action: "redo", keys: []string{":", "text:e4", "enter", "u", "U"}},
	{action: "flip", keys: []string{"f"}},
	{action: "hint", keys: []string{"H"}},
	{action: "claim", keys: []string{"d"}},
	{action: "review", keys: []string{":", "text:e4", "enter", "v"}},
	{action: "new_open", keys: []string{"n"}},
	{action: "ng_mode", keys: []string{"n", "m"}},
	{action: "ng_side", keys: []string{"n", "s"}},
	{action: "ng_level", keys: []string{"n", "l"}},
	{action: "ng_clock", keys: []string{"n", "c"}},
	{action: "ng_start", keys: []string{"n", "c", "enter"}},
	{action: "ng_cancel", keys: []string{"n", "esc"}},
	{action: "resign_open", keys: []string{"r"}},
	{action: "resign_yes", keys: []string{"r", "y"}},
	{action: "resign_no", keys: []string{"r", "n"}},
	{action: "pgn", keys: []string{"p"}},
	{action: "pgn_close", keys: []string{"p", "enter"}},
	{action: "promo_q", setup: promoSetup, keys: []string{":", "text:a8", "enter", "q"}},
	{action: "promo_r", setup: promoSetup, keys: []string{":", "text:a8", "enter", "r"}},
	{action: "promo_b", setup: promoSetup, keys: []string{":", "text:a8", "enter", "b"}},
	{action: "promo_n", setup: promoSetup, keys: []string{":", "text:a8", "enter", "n"}},
	{action: "promo_cancel", setup: promoSetup, keys: []string{":", "text:a8", "enter", "esc"}},
	{action: "toggle_mouse", keys: []string{"m"}},
	{action: "toggle_letters", keys: []string{"L"}},
	{action: "help_open", keys: []string{"?"}},
	{action: "help_close", keys: []string{"?", "?"}},
}

// twoPlayerSetup lets one keyboard play both sides, so a move is answered by
// nothing but the move.
func twoPlayerSetup(a *testApp) {
	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.set
	s.mode = mode2P
	a.startGame(s, StartPosition(), nil)
	_ = a.writeLocked()
}

// promoSetup is a white pawn about to promote.
func promoSetup(a *testApp) {
	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.set
	s.mode = mode2P
	start, _ := ParseFEN("4k3/P7/8/8/8/8/8/4K3 w - - 0 1")
	a.startGame(s, start, nil)
	_ = a.writeLocked()
}

// stepsOf turns a mock step's keys into play steps; click:e2 clicks the cell
// of the square.
func (a *testApp) stepsOf(t testing.TB, keys []string) []string {
	out := make([]string, len(keys))
	for i, k := range keys {
		if sqName, ok := strings.CutPrefix(k, "click:"); ok {
			x, y := a.cellCenter(t, 100, 24, sqName)
			k = "click:" + strconv.Itoa(x) + "," + strconv.Itoa(y)
		}
		out[i] = k
	}
	return out
}

// mockData answers each action with the writes its handler makes: the store
// paths that differ across the last key (the state just before the action
// against the state after it), plus a write to "@focus" when the handler
// moved the focus, which a store diff cannot see.
func mockData(t testing.TB) map[string]any {
	t.Helper()
	mock := map[string]any{}
	opts := tuimark.PlayOptions{Cols: 100, Rows: 24}
	for _, s := range mockSteps {
		a := newTestApp(t)
		if s.setup != nil {
			s.setup(a)
		}
		keys := a.stepsOf(t, s.keys)
		scene, last := keys[:len(keys)-1], keys[len(keys)-1:]
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
		changes := append([]any{}, diff...)
		if f := focused(res); f != "" && f != focused(before) {
			changes = append(changes, change{"path": "@focus", "value": "#" + f})
		}
		mock[s.action] = changes
	}
	return mock
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
		t.Errorf("%s differs from what the host builds; run UPDATE_SAMPLE=1 go test ./examples/chess -run TestSampleInSync", name)
	}
}

func TestSampleInSync(t *testing.T) {
	checkFile(t, "sample.json", sampleData(t))
	checkFile(t, "mock.json", mockData(t))
}

// The derived paths are in the sample, and the sample is the start of a
// game against the computer.
func TestSampleShape(t *testing.T) {
	s := sampleData(t)
	for _, k := range []string{"cells", "labels", "mrows", "mcur", "top", "bot", "status", "evalbar", "engine_view", "mode_label", "think_label", "ng_view"} {
		if _, ok := s[k]; !ok {
			t.Errorf("sample has no derived %q", k)
		}
	}
	if len(s["cells"].(map[string]any)) != 64 {
		t.Errorf("the sample has %d cells, want 64", len(s["cells"].(map[string]any)))
	}
	if s["mode"] != modeHvC || s["level"] != float64(3) || s["turn"] != "white" || s["human"] != "white" {
		t.Errorf("sample is not a fresh game against level 3: %v %v %v", s["mode"], s["level"], s["turn"])
	}
	if got := s["status"].(map[string]any)["text"]; got != "White to move" {
		t.Errorf("status = %v", got)
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

// mock.json answers exactly the host actions the document names: no stale
// name, and no action left without an answer.
func TestMockNamesRealActions(t *testing.T) {
	a := newTestApp(t)
	mock := readMock(t)
	named := map[string]bool{}
	for _, spec := range a.ui.Catalog() {
		if spec.Builtin {
			continue
		}
		named[spec.Name] = true
		if _, ok := mock[spec.Name]; !ok && spec.Name != "quit" {
			t.Errorf("mock.json has no answer for %s", spec.Name)
		}
		if !spec.Registered {
			t.Errorf("action %s has no Go handler", spec.Name)
		}
	}
	for name := range mock {
		if !named[name] {
			t.Errorf("mock.json answers %q, which no key or button fires", name)
		}
	}
}

// closeFlags are the dialogs' open flags: every action that closes a dialog
// answers by writing its flag false, so under `run --mock` esc and the
// buttons really close it.
var closeFlags = map[string]string{
	"help_close": "help_open", "ng_cancel": "new_open", "ng_start": "new_open",
	"resign_yes": "resign_open", "resign_no": "resign_open", "promo_cancel": "promo_open",
	"promo_q": "promo_open", "promo_r": "promo_open", "promo_b": "promo_open", "promo_n": "promo_open",
	"pgn_close": "pgn_open",
}

func TestMockClosesTheModals(t *testing.T) {
	a := newTestApp(t)
	mock := readMock(t)
	for action, flag := range closeFlags {
		if v, ok := writes(mock, action, flag); !ok || v != false {
			t.Errorf("%s closes a dialog, but mock.json writes %s = %v (written: %v)", action, flag, v, ok)
		}
	}
	// Every action a dialog fires on escape or close is on the list.
	found := 0
	for _, spec := range a.ui.Catalog() {
		for _, src := range spec.Sources {
			if strings.HasSuffix(src, " on:escape") || strings.HasSuffix(src, " on:close") {
				found++
				if _, ok := closeFlags[spec.Name]; !ok {
					t.Errorf("%s closes a dialog on escape, but is not in closeFlags", spec.Name)
				}
			}
		}
	}
	if found < 5 {
		t.Errorf("found only %d escape actions in the catalog", found)
	}
}

// Every action that moves the focus in the real app has a "@focus" write,
// found by playing the action, and only those do.
func TestMockFocusWritesMatchTheHandlers(t *testing.T) {
	mock := readMock(t)
	for _, name := range []string{"cmd_cancel", "cmd_submit", "help_close", "ng_cancel", "ng_start", "resign_no", "resign_yes", "pgn_close", "promo_q", "promo_cancel"} {
		if v, ok := writes(mock, name, "@focus"); !ok || v != "#board" {
			t.Errorf("%s: @focus = %v (written: %v), want #board", name, v, ok)
		}
	}
	// Opening a dialog moves the focus into it (the runtime does that, and a
	// mock has to say so); nothing else but the closers above writes it.
	opens := map[string]string{"help_open": "#help-ok", "new_open": "#ng-start", "resign_open": "#resign-no", "pgn": "#pgn-ok"}
	for name, want := range opens {
		if v, ok := writes(mock, name, "@focus"); !ok || v != want {
			t.Errorf("%s: @focus = %v (written: %v), want %s", name, v, ok, want)
		}
	}
	closers := map[string]bool{}
	for _, name := range []string{"cmd_cancel", "cmd_submit", "help_close", "ng_cancel", "ng_start", "resign_no", "resign_yes", "pgn_close", "promo_q", "promo_r", "promo_b", "promo_n", "promo_cancel"} {
		closers[name] = true
	}
	for name, ws := range mock {
		for _, w := range ws {
			if w["path"] == "@focus" && !closers[name] && opens[name] == "" {
				t.Errorf("%s writes @focus %v, but its handler does not move the focus", name, w["value"])
			}
		}
	}
}
