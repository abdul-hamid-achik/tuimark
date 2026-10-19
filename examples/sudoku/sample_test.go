package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

// sample.json and mock.json are generated, so they cannot drift from the
// host: TestSampleInSync fails when they differ, and
//
//	UPDATE_SAMPLE=1 go test ./examples/sudoku -run TestSampleInSync
//
// rewrites them.
//
// sample.json is the whole store of a new game at its documented point: the
// default seed (1), medium, the cursor on the first empty cell, time 0:00,
// derived paths included. `tuimark validate`, `dump`, and `run` have no Go
// host and no Derive, so they read these literal values.
//
// mock.json answers each action of `tuimark run --mock` with the literal
// writes the real handler makes: the store paths that differ across the
// action, found by running it from the sample state, derived paths included.
// Only paths the document reads are kept (`run` has no Derive, and a path
// nothing reads changes nothing on screen). Three kinds of answer differ:
//
//   - the arrows are recorded with the cursor on r5c5, so a move shows;
//   - the close actions are recorded with their modal open;
//   - pause and mouse answer with nothing: a toggle that can only be turned
//     one way would trap the session (a PAUSED board that later digits
//     paint into, a mouse that stays off).

// sampleLevel is the difficulty of the sample's puzzle.
const sampleLevel = levelMedium

// samplePuzzle is the puzzle of the sample and of most tests.
func samplePuzzle() puzzle { return generateCached(defaultSeed, sampleLevel) }

// newApp is the wiring run does before Run: load, with the sample puzzle.
func newApp(t testing.TB) *app {
	t.Helper()
	a, err := load(".", samplePuzzle(), defaultSeed)
	if err != nil {
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
type write = map[string]any

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
		*out = append(*out, write{"path": prefix, "value": got})
	}
}

// focused is the id of the focused node, or "".
func focused(res *tuimark.PlayResult) string {
	if res.Dump.Focus == nil {
		return ""
	}
	return *res.Dump.Focus
}

// cellStep is the play step that clicks the cell with this id in a dump
// at cols x rows.
func cellStep(t testing.TB, a *app, cols, rows int, id string) string {
	t.Helper()
	d, err := a.ui.Dump(cols, rows)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range d.Nodes {
		if n.ID == id {
			return fmt.Sprintf("click:%d,%d", n.X+1, n.Y)
		}
	}
	t.Fatalf("no node %q at %dx%d", id, cols, rows)
	return ""
}

// wrongDigit is a digit that is not the solution of the cursor cell.
func wrongDigit(a *app) string {
	return fmt.Sprint(int(a.g.sol[a.g.cur])%9 + 1)
}

// finish solves the game with hints and lets the clock open the win modal.
func finish(t testing.TB, a *app) {
	t.Helper()
	for !a.g.won {
		if _, ok := a.g.hint(); !ok {
			t.Fatal("hint found nothing to fill")
		}
	}
	if err := a.sync(); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 4; i++ {
		now = now.Add(tickEvery)
		tickAt(t, a, now)
	}
}

// readPaths are the store paths app.tui reads: bound text, if, open, mouse,
// and class: guards.
var readPaths = func() []string {
	raw, err := os.ReadFile("app.tui")
	if err != nil {
		panic(err)
	}
	re := regexp.MustCompile(`\{([a-z_][a-zA-Z0-9_.]*)\}|(?:if|open|mouse|class:[a-z_-]+)="!?([a-z_][a-zA-Z0-9_.]*)"`)
	seen := map[string]bool{}
	var out []string
	for _, m := range re.FindAllStringSubmatch(string(raw), -1) {
		p := m[1] + m[2]
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}()

// readByDocument reports whether the document reads path, or something
// under it, or a parent of it.
func readByDocument(path string) bool {
	for _, r := range readPaths {
		if r == path || strings.HasPrefix(r, path+".") || strings.HasPrefix(path, r+".") {
			return true
		}
	}
	return false
}

// mockSteps are the keys that fire each action from the sample state. The
// last key fires the action; the scene runs before it and only sets the
// scene.
var mockSteps = []struct {
	inert  bool // the answer is empty by design
	action string
	scene  func(t testing.TB, a *app) []string
	keys   []string // the key steps after the scene; the last fires the action
	last   func(t testing.TB, a *app) string
}{
	{action: "cell", last: func(t testing.TB, a *app) string { return cellStep(t, a, 100, 24, "c44") }},
	{action: "up", scene: center, keys: []string{"up"}},
	{action: "down", scene: center, keys: []string{"down"}},
	{action: "left", scene: center, keys: []string{"left"}},
	{action: "right", scene: center, keys: []string{"right"}},
	{action: "put1", keys: []string{"1"}},
	{action: "put2", keys: []string{"2"}},
	{action: "put3", keys: []string{"3"}},
	{action: "put4", keys: []string{"4"}},
	{action: "put5", keys: []string{"5"}},
	{action: "put6", keys: []string{"6"}},
	{action: "put7", keys: []string{"7"}},
	{action: "put8", keys: []string{"8"}},
	{action: "put9", keys: []string{"9"}},
	{action: "erase", keys: []string{"0"}},
	{action: "notes", keys: []string{"n"}},
	{action: "undo", keys: []string{"u"}},
	{action: "redo", keys: []string{"U"}},
	{action: "hint", keys: []string{"H"}},
	{action: "check", keys: []string{"c"}},
	{action: "pause", inert: true, keys: []string{"p"}},
	{action: "mouse", inert: true, keys: []string{"m"}},
	{action: "help_open", keys: []string{"?"}},
	{action: "help_close", scene: keysOf("?"), keys: []string{"esc"}},
	{action: "newgame_open", keys: []string{"N"}},
	{action: "newgame_close", scene: keysOf("N"), keys: []string{"esc"}},
	{action: "new_game", scene: keysOf("N"), keys: []string{"enter"}},
	{action: "win_close", scene: func(t testing.TB, a *app) []string { finish(t, a); return nil }, keys: []string{"esc"}},
}

func keysOf(keys ...string) func(testing.TB, *app) []string {
	return func(testing.TB, *app) []string { return keys }
}

// center is the scene that puts the cursor on the middle cell.
func center(t testing.TB, a *app) []string {
	return []string{cellStep(t, a, 100, 24, "c44")}
}

// mockData answers each action with the writes its handler makes: the
// store paths that differ across the last step (the state just before the
// action against the state after it), plus a write to "@focus" when the
// handler moved the focus, which a store diff cannot see.
func mockData(t testing.TB) map[string]any {
	t.Helper()
	mock := map[string]any{}
	opts := tuimark.PlayOptions{Cols: 100, Rows: 24}
	for _, s := range mockSteps {
		a := newApp(t)
		var scene []string
		if s.scene != nil {
			scene = s.scene(t, a)
		}
		keys := s.keys
		if s.last != nil {
			keys = []string{s.last(t, a)}
		}
		scene = append(scene, keys[:len(keys)-1]...)
		before, err := a.ui.Play(opts, scene...)
		if err != nil {
			t.Fatalf("%s: %v", s.action, err)
		}
		v, _ := a.ui.Get("")
		prev := normalize(t, v)
		res, err := a.ui.Play(opts, keys[len(keys)-1])
		if err != nil {
			t.Fatalf("%s: %v", s.action, err)
		}
		fired := false
		for _, e := range res.Events {
			fired = fired || e.Action == s.action
		}
		if !fired {
			t.Fatalf("%v did not fire %s: %+v", keys, s.action, res.Events)
		}
		v, _ = a.ui.Get("")
		var diff []any
		diffStore("", prev, normalize(t, v), &diff)
		changes := []any{}
		for _, w := range diff {
			if !s.inert && readByDocument(w.(write)["path"].(string)) {
				changes = append(changes, w)
			}
		}
		if f := focused(res); !s.inert && f != "" && f != focused(before) {
			changes = append(changes, write{"path": "@focus", "value": "#" + f})
		}
		mock[s.action] = changes
	}
	return mock
}

// compact writes a JSON object with one member per line, a list of objects
// or an object of objects with one element per line, so the files diff and
// read well.
func compact(t testing.TB, v map[string]any) []byte {
	t.Helper()
	enc := func(x any) []byte {
		var eb bytes.Buffer
		e := json.NewEncoder(&eb)
		e.SetEscapeHTML(false)
		if err := e.Encode(x); err != nil {
			t.Fatal(err)
		}
		return bytes.TrimRight(eb.Bytes(), "\n")
	}
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b bytes.Buffer
	b.WriteString("{\n")
	for i, k := range keys {
		b.WriteString("  ")
		b.Write(enc(k))
		b.WriteString(": ")
		switch x := v[k].(type) {
		case []any:
			if len(x) > 0 {
				if _, objs := x[0].(map[string]any); objs {
					b.WriteString("[\n")
					for j, e := range x {
						b.WriteString("    ")
						b.Write(enc(e))
						if j < len(x)-1 {
							b.WriteString(",")
						}
						b.WriteString("\n")
					}
					b.WriteString("  ]")
					break
				}
			}
			b.Write(enc(x))
		case map[string]any:
			if allObjects(x) {
				names := make([]string, 0, len(x))
				for n := range x {
					names = append(names, n)
				}
				sort.Strings(names)
				b.WriteString("{\n")
				for j, n := range names {
					b.WriteString("    ")
					b.Write(enc(n))
					b.WriteString(": ")
					b.Write(enc(x[n]))
					if j < len(names)-1 {
						b.WriteString(",")
					}
					b.WriteString("\n")
				}
				b.WriteString("  }")
				break
			}
			b.Write(enc(x))
		default:
			b.Write(enc(x))
		}
		if i < len(keys)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("}\n")
	return b.Bytes()
}

// allObjects reports whether m has at least two members, all objects.
func allObjects(m map[string]any) bool {
	if len(m) < 2 {
		return false
	}
	for _, e := range m {
		if _, ok := e.(map[string]any); !ok {
			return false
		}
	}
	return true
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
		t.Errorf("%s differs from what the host builds; run UPDATE_SAMPLE=1 go test ./examples/sudoku -run TestSampleInSync", name)
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
	for _, k := range []string{"cells", "boxes", "sel", "mini", "pad", "filled", "state", "diff_label", "mode_label",
		"puzzle", "vals", "marks", "notes", "cur", "time", "mistakes", "hints", "paused", "won", "mouse"} {
		if _, ok := s[k]; !ok {
			t.Errorf("sample has no %q", k)
		}
	}
	if s["time"] != "0:00" || s["diff"] != "medium" || s["mouse"] != true || s["won"] != false {
		t.Errorf("sample is not a new medium game: time=%v diff=%v mouse=%v won=%v", s["time"], s["diff"], s["mouse"], s["won"])
	}
	if got := len(s["cells"].(map[string]any)); got != size {
		t.Errorf("sample has %d cells, want %d", got, size)
	}
	p := samplePuzzle()
	if s["puzzle"] != p.puz.String() || s["vals"] != p.puz.String() {
		t.Error("the sample's grid is not the sample puzzle")
	}
	if want := fmt.Sprintf("medium · %d clues", p.puz.clues()); s["diff_label"] != want {
		t.Errorf("diff_label = %v, want %q", s["diff_label"], want)
	}
	if first := strings.IndexByte(p.puz.String(), '0'); s["cur"] != float64(first) {
		t.Errorf("cur = %v, want the first empty cell %d", s["cur"], first)
	}
}

// mock.json answers exactly the host actions the document names: no stale
// name, and no action left without an answer.
func TestMockNamesRealActions(t *testing.T) {
	a := newApp(t)
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
			t.Errorf("no handler for %s (%v)", spec.Name, spec.Sources)
		}
	}
	for name := range mock {
		if !named[name] {
			t.Errorf("mock.json answers %q, which no key or button fires", name)
		}
	}
}

func readMock(t testing.TB) map[string][]map[string]any {
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

// The modal flag each close action must write false, so under `run --mock`
// esc and the buttons really close the modal.
var closeFlags = map[string]string{"help_close": "help_open", "newgame_close": "new_open", "win_close": "win_open"}

// A modal's close action answers by writing its open flag false; found
// from the catalog (every action a modal fires on escape or close), not
// from the list above, so a new modal without a close answer is caught.
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
		flag, ok := closeFlags[spec.Name]
		if !ok {
			t.Errorf("%s closes a modal, but the test does not know its flag", spec.Name)
			continue
		}
		if v, ok := writes(mock, spec.Name, flag); !ok || v != false {
			t.Errorf("%s: mock.json writes %s = %v (written: %v), want false", spec.Name, flag, v, ok)
		}
	}
	if found != len(closeFlags) {
		t.Errorf("found %d modal-close actions in the catalog, want %d", found, len(closeFlags))
	}
}

// An action that opens a modal writes its flag true, and starting a game
// closes the new-game modal.
func TestMockOpensAndStartsGames(t *testing.T) {
	mock := readMock(t)
	if v, ok := writes(mock, "help_open", "help_open"); !ok || v != true {
		t.Errorf("help_open writes help_open = %v (written: %v)", v, ok)
	}
	if v, ok := writes(mock, "newgame_open", "new_open"); !ok || v != true {
		t.Errorf("newgame_open writes new_open = %v (written: %v)", v, ok)
	}
	if v, ok := writes(mock, "new_game", "new_open"); !ok || v != false {
		t.Errorf("new_game writes new_open = %v (written: %v), want false", v, ok)
	}
	cells := 0
	for _, w := range mock["new_game"] {
		if strings.HasPrefix(w["path"].(string), "cells.") {
			cells++
		}
	}
	if cells < 10 {
		t.Errorf("new_game writes %d cell paths: no new puzzle on screen", cells)
	}
}

// The mock reflects what the handlers do to the focus: whatever focus move
// the generator saw is in the file, and only the actions that open or
// close a modal move it.
func TestMockFocusWrites(t *testing.T) {
	mock := readMock(t)
	for name, ws := range mock {
		for _, w := range ws {
			if w["path"] != "@focus" {
				continue
			}
			if name != "help_open" && name != "help_close" && name != "newgame_open" && name != "newgame_close" &&
				name != "new_game" && name != "win_close" {
				t.Errorf("%s writes @focus %v, but it opens or closes no modal", name, w["value"])
			}
		}
	}
	if v, ok := writes(mock, "help_open", "@focus"); !ok || v != "#help-ok" {
		t.Errorf("help_open: @focus = %v (written: %v), want #help-ok", v, ok)
	}
}

// run --mock has no Derive and reads only what the document shows: a mock
// write of any other path is dead weight, and the generator drops them.
func TestMockWritesOnlyWhatTheDocumentReads(t *testing.T) {
	if len(readPaths) < 100 {
		t.Fatalf("only %d paths found in app.tui", len(readPaths))
	}
	n := 0
	for name, ws := range readMock(t) {
		for _, w := range ws {
			p := w["path"].(string)
			if p == "@focus" {
				continue
			}
			n++
			if !readByDocument(p) {
				t.Errorf("%s writes %s, which app.tui never reads", name, p)
			}
		}
	}
	if n < 100 {
		t.Errorf("only %d writes checked", n)
	}
	for _, dead := range []string{"cur", "marks", "paused", "puzzle", "vals", "notes", "won", "clues"} {
		if readByDocument(dead) {
			t.Errorf("%s is read by the document: update the test", dead)
		}
	}
}

// The toggles with no way back answer with nothing, so a `run --mock`
// session cannot be trapped on a PAUSED board or with the mouse off.
func TestMockToggleAnswersAreHarmless(t *testing.T) {
	mock := readMock(t)
	for _, action := range []string{"pause", "mouse"} {
		if ws, ok := mock[action]; !ok || len(ws) != 0 {
			t.Errorf("%s answers %v, want an empty list", action, ws)
		}
	}
	// And no answer sets paused or turns the mouse off.
	for name, ws := range mock {
		for _, w := range ws {
			if w["path"] == "paused" || (w["path"] == "mouse" && w["value"] == false) {
				t.Errorf("%s writes %v", name, w)
			}
		}
	}
}

// The answers recorded from the sample state itself do what the handler
// does there: erase and undo have nothing to act on, check finds no wrong
// entry, and none of them marks the blank cursor cell as wrong.
func TestMockAnswersFromTheSampleAreHonest(t *testing.T) {
	mock := readMock(t)
	for _, action := range []string{"erase", "undo", "redo"} {
		if ws := mock[action]; len(ws) != 0 {
			t.Errorf("%s answers %v from a new game, where it does nothing", action, ws)
		}
	}
	if v, ok := writes(mock, "check", "info"); !ok || v != "check: no wrong entries" {
		t.Errorf("check info = %v (written: %v)", v, ok)
	}
	for _, w := range mock["check"] {
		if p, _ := w["path"].(string); strings.HasSuffix(p, ".wrong") && w["value"] == true {
			t.Errorf("check marks %s wrong in a new game", p)
		}
	}
}
