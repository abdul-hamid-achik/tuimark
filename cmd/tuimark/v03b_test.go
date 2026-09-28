package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// SPEC v0.3 §21 tests 93 and 95 through the CLI (v0.3b, the layout and
// CSS items): the committed goldens of the version="2" fixtures that use
// gap, inspect's winner for row-gap, and play's resize over a priority
// table.

// 93. A version="2" document with only gap dumps byte-identically before
// and after row-gap/column-gap: the goldens of the version="2" grid,
// table, tabs, and hints fixtures (a gap in a stylesheet, gap="…"
// attributes, and the built-in hints gaps), written before 0.3b, still
// match dump's output byte for byte, text and JSON.
func TestGapOnlyVersionTwoGoldensUnchanged(t *testing.T) {
	const dir = "../../testdata/golden"
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var entries []goldenEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatal(err)
	}
	fixtures := map[string]bool{"grid": true, "table": true, "tabs": true, "hints": true}
	n := 0
	for _, e := range entries {
		if !fixtures[e.Name] {
			continue
		}
		src, err := os.ReadFile(filepath.Join("../..", e.File))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(src), `<tui version="2"`) {
			t.Fatalf("%s: %s is not a version=\"2\" document", e.Name, e.File)
		}
		args := func(size string) []string {
			cols, rows, _ := strings.Cut(size, "x")
			a := []string{"dump", filepath.Join("../..", e.File), "--cols", cols, "--rows", rows}
			if e.Data != "" {
				a = append(a, "--data", filepath.Join("../..", e.Data))
			}
			return a
		}
		for _, size := range e.Sizes {
			_, out, _ := runCLI(args(size)...)
			want, err := os.ReadFile(filepath.Join(dir, e.Name, size+".txt"))
			if err != nil {
				t.Fatal(err)
			}
			if out != string(want) {
				t.Errorf("%s %s: text dump differs from the golden\n%s", e.Name, size, out)
			}
			n++
		}
		for _, size := range e.JSON {
			_, out, _ := runCLI(append(args(size), "--format", "json")...)
			want, err := os.ReadFile(filepath.Join(dir, e.Name, size+".json"))
			if err != nil {
				t.Fatal(err)
			}
			if out != string(want) {
				t.Errorf("%s %s: JSON dump differs from the golden\n%s", e.Name, size, out)
			}
			n++
		}
	}
	if n < 8 {
		t.Errorf("compared %d goldens, want the grid, table, tabs, and hints ones", n)
	}
}

// 93. inspect names the #g gap declaration as the winner for row-gap
// when .x { row-gap: 0 } is less specific, with the .x declaration in its
// overridden; row-gap and column-gap follow gap in the style list.
func TestInspectRowGapWinner(t *testing.T) {
	tui := writeTemp(t, "g.tui", `<tui version="3"><style>
.x { row-gap: 0; }
#g { gap: 2; }
</style><screen id="main"><box id="g" class="x" style="layout: grid; grid-columns: 2"><text>a</text><text>b</text><text>c</text></box></screen></tui>`)
	code, out, errw := runCLI("inspect", tui, "--id", "g", "--cols", "20", "--rows", "6", "--json")
	if code != 0 {
		t.Fatalf("exit %d: %s\n%s", code, errw, out)
	}
	var in struct {
		Style []struct {
			Prop   string  `json:"prop"`
			Value  *string `json:"value"`
			Origin string  `json:"origin"`
			Rule   *struct {
				Selector  string `json:"selector"`
				Line, Col int
			} `json:"rule"`
			Overridden []struct {
				Value string `json:"value"`
				Rule  struct {
					Selector string `json:"selector"`
				} `json:"rule"`
			} `json:"overridden"`
		} `json:"style"`
	}
	mustUnmarshal(t, out, &in)
	var props []string
	idx := map[string]int{}
	for i, s := range in.Style {
		props = append(props, s.Prop)
		idx[s.Prop] = i
	}
	if !strings.Contains(strings.Join(props, " "), "flex gap row-gap column-gap padding") {
		t.Errorf("style order %v", props)
	}
	gap, rg := in.Style[idx["gap"]], in.Style[idx["row-gap"]]
	if rg.Value == nil || *rg.Value != "2" || rg.Origin != "author" || rg.Rule == nil || rg.Rule.Selector != "#g" {
		t.Fatalf("row-gap %+v, want 2 from #g", rg)
	}
	if gap.Rule == nil || rg.Rule.Line != gap.Rule.Line || rg.Rule.Col != gap.Rule.Col {
		t.Errorf("row-gap's rule %+v is not the gap declaration %+v", rg.Rule, gap.Rule)
	}
	if len(rg.Overridden) != 1 || rg.Overridden[0].Value != "0" || rg.Overridden[0].Rule.Selector != ".x" {
		t.Errorf("row-gap overridden %+v, want the .x declaration", rg.Overridden)
	}
}

// 95. play --cols 60 --input "resize:120x3" over the priority fixture
// shows all five columns in its final frame (priority is re-evaluated
// every frame); the initial frame at 60 had three.
func TestPriorityPlayResize(t *testing.T) {
	tui := writeTemp(t, "priority.tui", `<tui version="3">
  <screen id="main">
    <table id="t" each="rows as r" key="r.id" gap="1">
      <column id="a" title="A" width="18">{r.a}</column>
      <column id="b" title="B" width="18" priority="3">{r.b}</column>
      <column id="c" title="C" width="18" priority="1">{r.c}</column>
      <column id="d" title="D" width="18" priority="2">{r.d}</column>
      <column id="e" title="E" width="18" priority="1">{r.e}</column>
    </table>
  </screen>
</tui>
`)
	data := filepath.Join(filepath.Dir(tui), "priority.json")
	if err := os.WriteFile(data, []byte(`{"rows": [{"id": 1, "a": "a1", "b": "b1", "c": "c1", "d": "d1", "e": "e1"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	columns := func(out string) string {
		var d struct {
			Cols  int `json:"cols"`
			Nodes []struct {
				ID  string `json:"id"`
				Tag string `json:"tag"`
			} `json:"nodes"`
		}
		mustUnmarshal(t, out, &d)
		var ids []string
		for _, n := range d.Nodes {
			if n.Tag == "column" {
				ids = append(ids, n.ID)
			}
		}
		return strings.Join(ids, " ")
	}
	code, out, errw := runCLI("dump", tui, "--data", data, "--cols", "60", "--rows", "3", "--format", "json")
	if code != 0 {
		t.Fatalf("dump: exit %d: %s", code, errw)
	}
	if got := columns(out); got != "a b d" {
		t.Errorf("dump at 60: columns %q, want \"a b d\"", got)
	}
	code, out, errw = runCLI("play", tui, "--data", data, "--cols", "60", "--rows", "3", "--input", "resize:120x3", "--format", "json")
	if code != 0 {
		t.Fatalf("play: exit %d: %s\n%s", code, errw, out)
	}
	if got := columns(out); got != "a b c d e" {
		t.Errorf("play after resize:120x3: columns %q, want \"a b c d e\"", got)
	}
}

// 107. examples/monitor on 0.3b (SPEC v0.3b §17.4 "In 0.3b"): its one
// <keymap> became seven <keymap when> groups whose rows drop the when
// their group gives them, and `tuimark ir`'s top-level keymap array (each
// row's effective when, SPEC v0.3b §8.1) is byte-identical to the 0.3a
// file's, frozen in examples/monitor/testdata/v0.3.0. The #filter
// tab,shift+tab row still comes before the #app tab,right,l and
// shift+tab,left,h rows, so tab in the filter fires filter_apply
// (monitor-play-filter). The root tree keeps each <keymap> node with its
// when as written, the ctrl+c row's when="" included.
func TestMonitorKeymapGroupsKeepTheIRKeymap(t *testing.T) {
	type irDoc struct {
		Keymap json.RawMessage `json:"keymap"`
		Root   struct {
			Children []struct {
				Kind     string            `json:"kind"`
				Attrs    map[string]string `json:"attrs"`
				Children []struct {
					Attrs map[string]string `json:"attrs"`
				} `json:"children"`
			} `json:"children"`
		} `json:"root"`
	}
	ir := func(path string) irDoc {
		t.Helper()
		code, out, errw := runCLI("ir", path)
		if code != 0 {
			t.Fatalf("ir %s: exit %d: %s", path, code, errw)
		}
		var d irDoc
		mustUnmarshal(t, out, &d)
		return d
	}
	live := ir("../../examples/monitor/studio.tui")
	frozen := ir("../../examples/monitor/testdata/v0.3.0/studio.tui")
	if len(live.Keymap) == 0 || string(live.Keymap) != string(frozen.Keymap) {
		t.Errorf("IR keymap differs from 0.3a's:\n0.3b %s\n0.3a %s", live.Keymap, frozen.Keymap)
	}

	var rows []struct {
		Keys, Action, When string
	}
	if err := json.Unmarshal(live.Keymap, &rows); err != nil {
		t.Fatal(err)
	}
	index := func(keys, when string) int {
		t.Helper()
		for i, r := range rows {
			if r.Keys == keys && r.When == when {
				return i
			}
		}
		t.Fatalf("no keymap row %s with when=%q", keys, when)
		return -1
	}
	filter := index("tab,shift+tab", "#filter")
	if next, prev := index("tab,right,l", "#app"), index("shift+tab,left,h", "#app"); filter > next || filter > prev {
		t.Errorf("the #filter tab row is row %d, after the #app rows %d and %d", filter, next, prev)
	}
	if i := index("ctrl+c", ""); rows[i].Action != "quit" {
		t.Errorf("ctrl+c row %+v", rows[i])
	}

	var groups []string
	ctrlC, hasCtrlC := "", false
	for _, c := range live.Root.Children {
		if c.Kind != "keymap" {
			continue
		}
		groups = append(groups, c.Attrs["when"])
		for _, b := range c.Children {
			if b.Attrs["keys"] == "ctrl+c" {
				ctrlC, hasCtrlC = b.Attrs["when"]
			}
		}
	}
	want := "#app #filter #app #procs:focus #kill #detail #settings-list:focus"
	if got := strings.Join(groups, " "); got != want {
		t.Errorf("keymap groups %q, want %q", got, want)
	}
	if !hasCtrlC || ctrlC != "" {
		t.Errorf("the ctrl+c bind node's when is %q (present: %v), want \"\" as written", ctrlC, hasCtrlC)
	}
}
