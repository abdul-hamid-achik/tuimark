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

	"github.com/abdul-hamid-achik/tuimark"
)

// sample.json and mock.json are generated, so they cannot drift from the
// host: TestSampleInSync fails when they differ, and
//
//	UPDATE_SAMPLE=1 go test ./examples/minesweeper -run TestSampleInSync
//
// rewrites them.
//
// sample.json is the whole store the app builds at start: a beginner game
// with the default seed (1), nothing revealed yet, the cursor in the middle,
// derived paths included. `tuimark validate`, `dump`, and `run` have no Go
// host and no Derive, so they read these literal values.
//
// mock.json answers each action of `tuimark run --mock` with the literal
// writes the real handler makes from that same state: the store paths that
// differ after the action, found by running it.

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

func startRig(t testing.TB) *rig { return newRigOn(t, beginner, defaultSeed) }

func storeOf(t testing.TB, r *rig) any {
	t.Helper()
	v, ok := r.a.ui.Get("")
	if !ok {
		t.Fatal("Get(\"\") found no store")
	}
	return normalize(t, v)
}

func sampleData(t testing.TB) map[string]any { return storeOf(t, startRig(t)).(map[string]any) }

type change = map[string]any

// mockSkip are the raw top-level paths the document never reads, only
// Derive does; `run --mock` has no Derive, so writing them would be noise.
var mockSkip = map[string]bool{"state": true, "mines_left": true, "dims": true, "level": true, "paused": true, "secs": true, "best": true}

// diffStore appends a write for every path where got differs from want,
// descending into objects and writing arrays at once. A board cell is
// written field by field, except one that has just come onto the grid
// (it was {"on": false}), which is written whole, and one that has gone
// off, which only needs its "on" turned off.
func diffStore(prefix string, want, got any, out *[]any) {
	wm, wok := want.(map[string]any)
	gm, gok := got.(map[string]any)
	if wok && gok {
		if isCell(prefix) {
			switch {
			case gm["on"] != true:
				if wm["on"] == true {
					*out = append(*out, change{"path": prefix + ".on", "value": false})
				}
				return
			case wm["on"] != true:
				*out = append(*out, change{"path": prefix, "value": got})
				return
			}
		}
		keys := map[string]bool{}
		for k := range wm {
			keys[k] = true
		}
		for k := range gm {
			keys[k] = true
		}
		var sorted []string
		for k := range keys {
			if prefix == "" && mockSkip[k] {
				continue
			}
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

// isCell reports whether path is one board cell, "b.cRRCC".
func isCell(path string) bool {
	_, id, ok := strings.Cut(path, ".")
	return strings.HasPrefix(path, "b.") && ok && !strings.Contains(id, ".")
}

// mockSteps are the steps that fire each action from the sample state. The
// last step fires the action; the ones before it only set the scene. A step
// "cell:R,C" clicks that cell of the board. The level keys are answered
// by levelAnswer instead. `new` is answered from a paused game, so the
// answer also takes the pause off.
var mockSteps = []struct {
	action string
	steps  []string
}{
	{"cur_up", []string{"up"}},
	{"cur_down", []string{"down"}},
	{"cur_left", []string{"left"}},
	{"cur_right", []string{"right"}},
	{"reveal", []string{"enter"}},
	{"cell", []string{"cell:2,6"}},
	{"flag", []string{"g"}},
	{"flag_mode", []string{"f"}},
	{"new", []string{"enter", "p", "n"}},
	{"pause", []string{"enter", "p"}},
	{"toggle_mouse", []string{"m"}},
	{"help_toggle", []string{"?"}},
	{"help_close", []string{"?", "esc"}},
}

// levelKeys fire the three level actions.
var levelKeys = map[string]string{"level_beginner": "1", "level_intermediate": "2", "level_expert": "3"}

// resolveStep turns "cell:R,C" into the click that lands on the cell.
func resolveStep(t testing.TB, r *rig, s string) string {
	t.Helper()
	spec, ok := strings.CutPrefix(s, "cell:")
	if !ok {
		return s
	}
	var row, col int
	if _, err := fmt.Sscanf(spec, "%d,%d", &row, &col); err != nil {
		t.Fatalf("bad step %q", s)
	}
	res, err := r.a.ui.Play(tuimark.PlayOptions{Cols: 90, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	return clickStep(t, res, row, col)
}

// resetGroups are the paths a level answer writes in full, because they
// all depend on the board and an answer must not rely on which board was
// showing: the rows, the size classes, and the labels.
var resetGroups = []string{"rows", "fit", "is", "lvl", "best_label", "best_now", "face", "state_label",
	"mines_label", "time_label", "mode_label", "level_label", "need_label"}

// leaves flattens v under prefix into writes, in path order.
func leaves(prefix string, v any, out *[]any) {
	if m, ok := v.(map[string]any); ok {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			leaves(prefix+"."+k, m[k], out)
		}
		return
	}
	*out = append(*out, change{"path": prefix, "value": v})
}

// levelAnswer is what a level key writes: the fresh board of that level
// from the start state, wherever the replay stood before. It is the diff
// from the start state, plus the board-dependent paths in full, plus
// "on": false for every position of a larger board that this one lacks,
// plus the cursor on the middle of the new board.
func levelAnswer(t testing.TB, key string) []any {
	t.Helper()
	r := startRig(t)
	start := storeOf(t, r)
	r.a.ui.Play(tuimark.PlayOptions{Cols: 90, Rows: 24}, key)
	target := storeOf(t, r).(map[string]any)
	var diff []any
	diffStore("", start, target, &diff)
	written := map[string]bool{}
	for _, w := range diff {
		written[w.(change)["path"].(string)] = true
	}
	var extra []any
	for _, g := range resetGroups {
		leaves(g, target[g], &extra)
	}
	cells := target["b"].(map[string]any)
	for _, p := range presets { // every position some level has
		for rr := 0; rr < p.rows; rr++ {
			for cc := 0; cc < p.cols; cc++ {
				id := cellID(rr, cc)
				if cells[id].(map[string]any)["on"] == false {
					extra = append(extra, change{"path": "b." + id + ".on", "value": false})
				}
			}
		}
	}
	// The cursor: off the middle of every other level's board, on this one's.
	cr, cc := r.a.g.pos(r.a.cur)
	for _, p := range presets {
		id := cellID(p.rows/2, p.cols/2)
		if cells[id].(map[string]any)["on"] == true && id != cellID(cr, cc) {
			extra = append(extra, change{"path": "b." + id + ".cur", "value": false})
		}
	}
	extra = append(extra, change{"path": "b." + cellID(cr, cc) + ".cur", "value": true})
	seen := map[string]bool{}
	var out []any
	for _, w := range append(diff, extra...) {
		p := w.(change)["path"].(string)
		if !seen[p] {
			seen[p] = true
			out = append(out, w)
		}
	}
	return out
}

// mockData answers each action with the writes its handler makes: the
// store paths that differ across the last step, for what the document
// reads. No handler moves the focus (the runtime puts it back on the board
// when a modal closes), so there is no "@focus" write.
func mockData(t testing.TB) map[string]any {
	t.Helper()
	mock := map[string]any{}
	for _, s := range mockSteps {
		r := startRig(t)
		scene, last := s.steps[:len(s.steps)-1], s.steps[len(s.steps)-1]
		for _, step := range scene {
			r.a.ui.Play(tuimark.PlayOptions{Cols: 90, Rows: 24}, resolveStep(t, r, step))
		}
		prev := storeOf(t, r)
		res, err := r.a.ui.Play(tuimark.PlayOptions{Cols: 90, Rows: 24}, resolveStep(t, r, last))
		if err != nil {
			t.Fatalf("%s: %v", s.action, err)
		}
		fired := false
		for _, e := range res.Events {
			fired = fired || e.Action == s.action
		}
		if !fired {
			t.Fatalf("%v did not fire %s: %+v", s.steps, s.action, res.Events)
		}
		var diff []any
		diffStore("", prev, storeOf(t, r), &diff)
		if diff == nil {
			diff = []any{}
		}
		mock[s.action] = diff
	}
	for action, key := range levelKeys {
		mock[action] = levelAnswer(t, key)
	}
	return mock
}

// compact writes a JSON object with one member per line; an object member
// gets one entry per line, and an array of objects one element per line, so
// the files diff and read well.
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
		case map[string]any:
			if len(x) > 0 {
				b.WriteString("{\n")
				ek := make([]string, 0, len(x))
				for k2 := range x {
					ek = append(ek, k2)
				}
				sort.Strings(ek)
				for j, k2 := range ek {
					b.WriteString("    ")
					b.Write(enc(k2))
					b.WriteString(": ")
					b.Write(enc(x[k2]))
					if j < len(ek)-1 {
						b.WriteString(",")
					}
					b.WriteString("\n")
				}
				b.WriteString("  }")
				break
			}
			b.Write(enc(x))
		case []any:
			if len(x) > 0 {
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
		t.Errorf("%s differs from what the host builds; run UPDATE_SAMPLE=1 go test ./examples/minesweeper -run TestSampleInSync", name)
	}
}

func TestSampleInSync(t *testing.T) {
	checkFile(t, "sample.json", sampleData(t))
	checkFile(t, "mock.json", mockData(t))
}

// The sample is the whole start state: raw and derived paths, a cell for
// every position of the largest grid with every class guard in it.
func TestSampleShape(t *testing.T) {
	s := sampleData(t)
	for _, k := range []string{"b", "rows", "dims", "level", "state", "secs", "mines_left", "flag_mode",
		"paused", "help_open", "notice", "best", "settings",
		"mines_label", "time_label", "face", "state_label", "mode_label", "level_label", "need_label",
		"is", "lvl", "best_label", "fit"} {
		if _, ok := s[k]; !ok {
			t.Errorf("sample has no %q", k)
		}
	}
	b := s["b"].(map[string]any)
	if len(b) != maxRows*maxCols {
		t.Errorf("sample has %d cells, want %d", len(b), maxRows*maxCols)
	}
	on := 0
	for id, cv := range b {
		cell := cv.(map[string]any)
		if cell["on"] == false {
			// Off the grid: its `if` is false, so nothing else is read.
			if len(cell) != 1 {
				t.Errorf("cell %s is off but holds %v", id, cell)
			}
			continue
		}
		on++
		for _, g := range append([]string{"on", "t"}, guardNames...) {
			if _, ok := cell[g]; !ok {
				t.Errorf("cell %s has no %q", id, g)
			}
		}
	}
	if on != 81 {
		t.Errorf("%d cells are on, want the 81 of a beginner board", on)
	}
	if got := s["level_label"]; got != "beginner 9x9, 10 mines" {
		t.Errorf("level_label = %v", got)
	}
	if got := s["mines_label"]; got != "010" {
		t.Errorf("mines_label = %v", got)
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
// name, and no action left without an answer. Quit has nothing to write.
func TestMockNamesRealActions(t *testing.T) {
	r := newRig(t)
	mock := readMock(t)
	named := map[string]bool{}
	for _, spec := range r.a.ui.Catalog() {
		if spec.Builtin {
			continue
		}
		named[spec.Name] = true
		if !spec.Registered {
			t.Errorf("%s has no handler", spec.Name)
		}
		if _, ok := mock[spec.Name]; !ok && spec.Name != "quit" {
			t.Errorf("mock.json has no answer for %s", spec.Name)
		}
	}
	for name := range mock {
		if !named[name] {
			t.Errorf("mock.json answers %q, which no key, click, or button fires", name)
		}
	}
}

// Every action that closes the modal answers by writing its flag false, so
// under `run --mock` esc and the button really close it.
func TestMockClosesTheModal(t *testing.T) {
	r := newRig(t)
	mock := readMock(t)
	found := 0
	for _, spec := range r.a.ui.Catalog() {
		closes := false
		for _, src := range spec.Sources {
			closes = closes || strings.HasSuffix(src, " on:escape") || strings.HasSuffix(src, " on:close") ||
				strings.HasPrefix(src, "help-ok") || strings.Contains(src, "#help")
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

// No handler moves the focus, so no answer may write "@focus".
func TestMockMovesNoFocus(t *testing.T) {
	for name, ws := range readMock(t) {
		for _, w := range ws {
			if w["path"] == "@focus" {
				t.Errorf("%s writes @focus, but its handler does not move the focus", name)
			}
		}
	}
}

// The answer to the first reveal opens the cells the real handler opened.
func TestMockRevealMatchesTheHandler(t *testing.T) {
	mock := readMock(t)
	r := startRig(t)
	r.play("enter")
	opened := 0
	for _, w := range mock["reveal"] {
		path, _ := w["path"].(string)
		if strings.HasPrefix(path, "b.") && strings.HasSuffix(path, ".hid") && w["value"] == false {
			opened++
		}
	}
	if opened != r.a.g.opened {
		t.Errorf("mock reveal opens %d cells, the handler opened %d", opened, r.a.g.opened)
	}
	if opened < 2 {
		t.Errorf("the first reveal opened %d cells: the sample seed should open a region", opened)
	}
}

// The answers carry the derived values their actions change, because `run`
// has no Derive: pausing hides the board through is.paused, and the
// levels change the dims and the labels.
func TestMockAnswersCarryTheDerivedValues(t *testing.T) {
	mock := readMock(t)
	for action, want := range map[string]map[string]any{
		"pause":              {"is.paused": true, "face": ":|", "state_label": "paused"},
		"flag_mode":          {"flag_mode": true, "mode_label": "flag"},
		"level_intermediate": {"level_label": "intermediate 16x16, 40 mines", "mines_label": "040"},
		"help_toggle":        {"help_open": true},
	} {
		for path, v := range want {
			got, ok := writes(mock, action, path)
			if !ok || !reflect.DeepEqual(got, v) {
				t.Errorf("%s writes %s = %v (written: %v), want %v", action, path, got, ok, v)
			}
		}
	}
}

// A cell that stays on the grid is written field by field (so a cursor
// that moves is one true and one false, never two cursors); only a cell
// that comes onto the grid is written whole, and one that leaves it only
// turns "on" off.
func TestMockWritesCellsFieldByField(t *testing.T) {
	mock := readMock(t)
	for action, ws := range mock {
		for _, w := range ws {
			path, _ := w["path"].(string)
			if !isCell(path) {
				continue
			}
			cell, _ := w["value"].(map[string]any)
			if cell["on"] != true || len(cell) != 17 {
				t.Errorf("%s writes the whole cell %s as %v", action, path, w["value"])
			}
			if action != "level_intermediate" && action != "level_expert" {
				t.Errorf("%s writes the whole cell %s, which only a level change brings onto the grid", action, path)
			}
		}
	}
	// The cursor moves with one true and one false.
	for _, action := range []string{"cur_up", "cur_down", "cur_left", "cur_right"} {
		var trues, falses int
		for _, w := range mock[action] {
			if p, _ := w["path"].(string); strings.HasSuffix(p, ".cur") {
				if w["value"] == true {
					trues++
				} else {
					falses++
				}
			}
		}
		if trues != 1 || falses != 1 {
			t.Errorf("%s writes %d cursors on and %d off, want one of each", action, trues, falses)
		}
	}
}

// Everything a mock answer writes is something the document reads: `run`
// has no Derive, so a raw path only Derive reads is noise.
func TestMockWritesOnlyWhatTheDocumentReads(t *testing.T) {
	src, err := os.ReadFile("app.tui")
	if err != nil {
		t.Fatal(err)
	}
	read := map[string]bool{}
	for _, re := range []string{`\{([a-z_][a-zA-Z0-9_.]*)\}`, `(?:if|open|mouse|class:[a-z0-9_-]+)="!?([a-z_][a-zA-Z0-9_.]*)"`, `\{([a-z_][a-zA-Z0-9_]*)\.`} {
		for _, m := range regexp.MustCompile(re).FindAllStringSubmatch(string(src), -1) {
			read[strings.SplitN(m[1], ".", 2)[0]] = true
		}
	}
	for _, k := range []string{"b", "rows", "fit", "settings", "help_open", "flag_mode", "notice", "is", "lvl", "face", "mines_label"} {
		if !read[k] {
			t.Errorf("the document does not read %q, which this test expects it to", k)
		}
	}
	for action, ws := range readMock(t) {
		for _, w := range ws {
			path, _ := w["path"].(string)
			if top := strings.SplitN(path, ".", 2)[0]; !read[top] {
				t.Errorf("%s writes %s, which the document does not read", action, path)
			}
		}
	}
}

// Replayed in order, as `run --mock` does, the answers keep the board
// coherent: after any pair of level keys the board, the rows, the labels,
// and the size classes all describe that one level.
func TestMockLevelsReplayInAnyOrder(t *testing.T) {
	mock := readMock(t)
	keys := []string{"level_beginner", "level_intermediate", "level_expert"}
	dims := map[string][2]int{"level_beginner": {9, 9}, "level_intermediate": {16, 16}, "level_expert": {30, 16}}
	labels := map[string]string{"level_beginner": "beginner 9x9, 10 mines", "level_intermediate": "intermediate 16x16, 40 mines", "level_expert": "expert 30x16, 99 mines"}
	fits := map[string]string{"level_beginner": "nc36", "level_intermediate": "nc36", "level_expert": "nc62"}
	for _, first := range keys {
		for _, second := range keys {
			store := sampleData(t)
			for _, action := range []string{first, "reveal", second} {
				for _, w := range mock[action] {
					setPath(store, w["path"].(string), w["value"])
				}
			}
			d := dims[second]
			on := 0
			for _, c := range store["b"].(map[string]any) {
				if c.(map[string]any)["on"] == true {
					on++
				}
			}
			if on != d[0]*d[1] {
				t.Errorf("%s then %s: %d cells on, want %d", first, second, on, d[0]*d[1])
			}
			rows := 0
			for _, v := range store["rows"].(map[string]any) {
				if v == true {
					rows++
				}
			}
			if rows != d[1] {
				t.Errorf("%s then %s: %d rows on, want %d", first, second, rows, d[1])
			}
			if store["level_label"] != labels[second] {
				t.Errorf("%s then %s: level_label %v", first, second, store["level_label"])
			}
			for k, v := range store["fit"].(map[string]any) {
				if v == true && !strings.HasSuffix(k, fits[second][2:]) && k[:2] == "nc" && k != fits[second] {
					t.Errorf("%s then %s: fit.%s is still true", first, second, k)
				}
			}
			if store["fit"].(map[string]any)[fits[second]] != true {
				t.Errorf("%s then %s: fit.%s is not true", first, second, fits[second])
			}
			// Exactly one cursor, on the middle of the board.
			cur := 0
			for id, c := range store["b"].(map[string]any) {
				if c.(map[string]any)["on"] == true && c.(map[string]any)["cur"] == true { // an off cell is never drawn
					cur++
					if id != cellID(d[1]/2, d[0]/2) {
						t.Errorf("%s then %s: the cursor is on %s", first, second, id)
					}
				}
			}
			if cur != 1 {
				t.Errorf("%s then %s: %d cursors", first, second, cur)
			}
		}
	}
}

// One p hides the board for the rest of a session unless new and the level
// keys take the pause off again.
func TestMockPauseDoesNotStick(t *testing.T) {
	mock := readMock(t)
	for _, action := range []string{"new", "level_beginner", "level_intermediate", "level_expert"} {
		store := sampleData(t)
		for _, a := range []string{"pause", action} {
			for _, w := range mock[a] {
				setPath(store, w["path"].(string), w["value"])
			}
		}
		if store["is"].(map[string]any)["paused"] != false || store["face"] != ":)" || store["state_label"] != "ready" {
			t.Errorf("pause then %s leaves is.paused=%v face=%v state_label=%v", action, store["is"], store["face"], store["state_label"])
		}
	}
}

// setPath applies one mock write, as run --mock does.
func setPath(store map[string]any, path string, v any) {
	parts := strings.Split(path, ".")
	cur := store
	for _, p := range parts[:len(parts)-1] {
		next, ok := cur[p].(map[string]any)
		if !ok {
			next = map[string]any{}
			cur[p] = next
		}
		cur = next
	}
	// A copy, as run makes: later writes into a cell must not reach the answer.
	b, _ := json.Marshal(v)
	var cp any
	_ = json.Unmarshal(b, &cp)
	cur[parts[len(parts)-1]] = cp
}
