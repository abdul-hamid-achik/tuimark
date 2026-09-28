package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

// SPEC v0.3b §21 test 107: examples/monitor on 0.3b. §17.4 "In 0.3b"
// changes the 0.3a fixture in exactly three places: seven <keymap when>
// groups that keep the row order, row-gap: 0 on #cores, and priority on
// the Processes columns in place of the @media rules. The 0.3a files are
// frozen in testdata/v0.3.0 (they are also what §21 test 84 compares in
// its version="2" form, since the live file now needs version="3"). The
// IR keymap array is compared in cmd/tuimark
// (TestMonitorKeymapGroupsKeepTheIRKeymap); the goldens in the root
// package (TestMonitorAcceptance).

const frozenTUI = "testdata/v0.3.0/studio.tui"

// loadDoc loads a monitor document with sample.json and no handlers.
func loadDoc(t *testing.T, tui string) *tuimark.App {
	t.Helper()
	ui, err := tuimark.Load(tui)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("sample.json")
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	if err := ui.Bind("", data); err != nil {
		t.Fatal(err)
	}
	return ui
}

// The live files are the frozen 0.3a files with the three changes of
// §17.4 "In 0.3b" and nothing else: outside the keymap, studio.tui only
// gains the four priority attributes; studio.tcss only gains row-gap: 0
// and loses the three @media rules. The keymap itself is checked by what
// it does (below) and by the IR.
func TestOnlyTheThreeChanges(t *testing.T) {
	read := func(path string) string {
		t.Helper()
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	replaceOnce := func(s, old, new string) string {
		t.Helper()
		if strings.Count(s, old) != 1 {
			t.Fatalf("the 0.3a file has %d copies of %q", strings.Count(s, old), old)
		}
		return strings.Replace(s, old, new, 1)
	}

	liveTUI, oldTUI := read("studio.tui"), read(frozenTUI)
	const screen = "  <screen id=\"studio\">\n"
	liveHead, liveBody, ok1 := strings.Cut(liveTUI, screen)
	oldHead, oldBody, ok2 := strings.Cut(oldTUI, screen)
	if !ok1 || !ok2 {
		t.Fatal("no <screen id=\"studio\"> line")
	}
	want := oldBody
	for _, r := range [][2]string{
		{`width="8" on:click="sort_mem"`, `width="8" priority="3" on:click="sort_mem"`},
		{`title="THR" width="4">`, `title="THR" width="4" priority="2">`},
		{`title="I/O" width="9">`, `title="I/O" width="9" priority="1">`},
		{`title="USER" width="10">`, `title="USER" width="10" priority="1">`},
	} {
		want = replaceOnce(want, r[0], r[1])
	}
	if liveBody != want {
		t.Error("studio.tui changed outside the keymap beyond the four priority attributes")
	}
	liveLines, oldLines := strings.Split(liveHead, "\n"), strings.Split(oldHead, "\n")
	if liveLines[0] != oldLines[0] || liveLines[1] != oldLines[1] {
		t.Errorf("the <tui> or <style> line changed: %q", liveLines[:2])
	}
	if n := strings.Count(liveHead, "<keymap"); n != 7 {
		t.Errorf("%d <keymap> elements, want 7", n)
	}
	if strings.Count(liveHead, "<bind ") != strings.Count(oldHead, "<bind ") {
		t.Error("the number of keymap rows changed")
	}

	wantCSS := replaceOnce(read("testdata/v0.3.0/studio.tcss"),
		"#cores { layout: grid; grid-columns: 4; grid-min-width: 22; gap: 1; }",
		"#cores { layout: grid; grid-columns: 4; grid-min-width: 22; gap: 1; row-gap: 0; }")
	for _, rule := range []string{
		"@media (max-cols: 99) { #c-io, #c-user { display: none; } }\n",
		"@media (max-cols: 77) { #c-thr { display: none; } }\n",
		"@media (max-cols: 57) { #c-mem { display: none; } }\n",
	} {
		wantCSS = replaceOnce(wantCSS, rule, "")
	}
	if got := read("studio.tcss"); got != wantCSS {
		t.Error("studio.tcss differs from the 0.3a file beyond row-gap and the @media rules")
	}
	// Neither scale nor modal focus= is used (§17.4 "What does not change").
	if strings.Contains(liveTUI, "scale=") {
		t.Error("a sparkline uses scale")
	}
	for _, l := range strings.Split(liveTUI, "\n") {
		if strings.Contains(l, "<modal ") && strings.Contains(l, " focus=") {
			t.Errorf("a modal uses focus=: %s", strings.TrimSpace(l))
		}
	}
}

// hintsOf lists each <hints>' items, "key label" in order, by the hints'
// id, as the frame shows them.
func hintsOf(d *tuimark.Dump) map[string][]string {
	out := map[string][]string{}
	cur := ""
	for _, n := range d.Nodes {
		if n.Tag == "hints" {
			cur = n.ID
			out[cur] = []string{}
			continue
		}
		for _, c := range n.Classes {
			if c == "hint-key" || c == "hint-label" {
				out[cur] = append(out[cur], c+"="+n.Text)
			}
		}
	}
	return out
}

// stepResult is what a step does that the keymap decides.
type stepResult struct {
	Err    string
	Quit   bool
	Events string
	Focus  string
	Hints  map[string][]string
}

func playStep(t *testing.T, ui *tuimark.App, opts tuimark.PlayOptions, step string) stepResult {
	t.Helper()
	res, err := ui.Play(opts, step)
	var r stepResult
	if err != nil {
		r.Err = err.Error()
	}
	if res == nil {
		return r
	}
	evs, jerr := json.Marshal(res.Events)
	if jerr != nil {
		t.Fatal(jerr)
	}
	r.Quit, r.Events = res.Quit, string(evs)
	if res.Dump != nil {
		if res.Dump.Focus != nil {
			r.Focus = *res.Dump.Focus
		}
		r.Hints = hintsOf(res.Dump)
	}
	return r
}

// The seven groups keep 0.3a's dispatch and hints (§8.1): the same
// catalog, and, step by step over a scripted tour of every focus context
// (the table, the filter, the kill and detail modals, the help modal's
// scope="all" list, the tab strip, the settings list) and a seeded random
// walk, the same events, quits, focus, and <hints> items as the frozen
// 0.3a document. Host actions are recorded, not handled; set: and focus:
// steps stand in for what the handlers would do.
func TestKeymapGroupsKeepDispatchAndHints(t *testing.T) {
	if a, b := loadDoc(t, "studio.tui").Catalog(), loadDoc(t, frozenTUI).Catalog(); !reflect.DeepEqual(a, b) {
		t.Errorf("Catalog differs from 0.3a's:\n0.3b %+v\n0.3a %+v", a, b)
	}

	tour := []string{
		"j", "space", "ctrl+a", "ctrl+d", "c", "m", "enter", "K", "X", "down", "pgdn", "home",
		"/", "set:filter_open=true", "focus:#filter", "text:ch", "q", "c", "tab", "shift+tab", "esc", "ctrl+u", "enter",
		"set:filter_open=false", "focus:#procs",
		"set:kill_open=true", "y", "n", "j", "space", "tab", "enter", "K", "1", "esc", "set:kill_open=false",
		"set:detail_open=true", "r", "K", "space", "j", "tab", "esc", "enter", "q", "set:detail_open=false",
		"set:help_open=true", "tab", "enter", "?", "esc", "q", "set:help_open=false",
		"8", "j", "k", "down", "enter", "space", "-", "s", "tab",
		"1", "tab", "shift+tab", "right", "left", "l", "h", "p", "r", "?",
		"2", "down", "pgdn", "j", "3", "9", "tab", "7", "q", "ctrl+c",
	}
	pool := []string{
		"q", "ctrl+c", "?", "tab", "shift+tab", "esc", "1", "2", "3", "4", "5", "6", "7", "8", "9",
		"right", "left", "l", "h", "p", "r", "j", "k", "space", "ctrl+a", "ctrl+d", "c", "m", "/",
		"enter", "K", "X", "y", "n", "-", "s", "up", "down", "home", "end", "pgup", "pgdn",
		"backspace", "ctrl+u", "text:ab",
		"set:kill_open=true", "set:kill_open=false", "set:detail_open=true", "set:detail_open=false",
		"set:help_open=true", "set:help_open=false", "set:filter_open=true", "set:filter_open=false",
		"focus:#filter", "focus:#procs", "focus:#settings-list", "focus:#nav",
	}
	rng := rand.New(rand.NewSource(107))
	walk := make([]string, 200)
	for i := range walk {
		walk[i] = pool[rng.Intn(len(pool))]
	}

	for _, run := range []struct {
		name  string
		opts  tuimark.PlayOptions
		steps []string
	}{
		{"tour 120x30", tuimark.PlayOptions{Cols: 120, Rows: 30, NoHandlers: true}, tour},
		{"walk 120x30", tuimark.PlayOptions{Cols: 120, Rows: 30, NoHandlers: true}, walk},
		{"tour 57x24", tuimark.PlayOptions{Cols: 57, Rows: 24, NoHandlers: true}, tour},
	} {
		t.Run(run.name, func(t *testing.T) {
			live, old := loadDoc(t, "studio.tui"), loadDoc(t, frozenTUI)
			quits, events := 0, 0
			for i, step := range run.steps {
				got, want := playStep(t, live, run.opts, step), playStep(t, old, run.opts, step)
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("step %d (%s) after %q:\n0.3b %+v\n0.3a %+v", i, step, run.steps[:i], got, want)
				}
				if got.Quit {
					quits++
				}
				if got.Events != "null" && got.Events != "[]" {
					events++
				}
			}
			if quits == 0 || events == 0 {
				t.Errorf("the steps fired %d events and %d quits: not a useful comparison", events, quits)
			}
		})
	}
}

// Tab in the filter still fires filter_apply and leaves the tab alone:
// the #filter group comes before the #app group that holds tab,right,l
// (§17.4 item 1.2; the monitor-play-filter golden).
func TestTabInTheFilterAppliesIt(t *testing.T) {
	ui := loadDoc(t, "studio.tui")
	opts := tuimark.PlayOptions{Cols: 120, Rows: 24, NoHandlers: true}
	res, err := ui.Play(opts, "/", "set:filter_open=true", "focus:#filter", "tab")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range res.Events {
		got = append(got, e.Action+" "+e.Source)
	}
	if strings.Join(got, ", ") != "filter_open procs, filter_apply filter" {
		t.Errorf("events %q", got)
	}
	if v, _ := ui.Get("view"); v != "processes" {
		t.Errorf("view %v after tab in the filter", v)
	}
}

// columnIDs are the Processes columns the frame lays out.
func columnIDs(t *testing.T, ui *tuimark.App, cols, rows int) (ids []string, errs []tuimark.Diagnostic) {
	t.Helper()
	d, err := ui.Dump(cols, rows)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range d.Nodes {
		if n.Tag == "column" {
			ids = append(ids, n.ID)
		}
	}
	return ids, d.Errors
}

// The Processes columns hide by priority (§6.9.3), at the widths §17.4
// "In 0.3b" gives: all seven from 65 terminal columns, six (no c-user)
// from 54 to 64, five (no c-io) from 44 to 53, four (no c-thr) from 39 to
// 43, three below. Test 107 lists 7, 7, 7, 6, 6, and 4 at 100, 80, 78, 58,
// 57, and 40; monitor_resize.yml sees 7, 7, 7, 6, and 4 at 120, 99, 77,
// 57, and 40.
func TestProcessesColumnsByPriority(t *testing.T) {
	ui := loadDoc(t, "studio.tui")
	all := []string{"c-pid", "c-name", "c-cpu", "c-mem", "c-thr", "c-io", "c-user"}
	for _, c := range []struct{ cols, n int }{
		{120, 7}, {100, 7}, {99, 7}, {80, 7}, {78, 7}, {77, 7}, {65, 7},
		{64, 6}, {58, 6}, {57, 6}, {54, 6},
		{53, 5}, {44, 5},
		{43, 4}, {40, 4}, {39, 4},
		{38, 3},
	} {
		ids, errs := columnIDs(t, ui, c.cols, 24)
		if got, want := strings.Join(ids, " "), strings.Join(all[:c.n], " "); got != want {
			t.Errorf("%d columns: %q, want %q", c.cols, got, want)
		}
		if len(errs) > 0 {
			t.Errorf("%d columns: %v", c.cols, errs)
		}
	}
}

// With row-gap: 0 the twelve cores in one column are 12 rows tall: at
// 40x12 #cores-view (9 rows) scrolls with scroll {"y": 0, "h": 12}; at
// 40x24 it fits, 14 rows tall.
func TestCoresViewAtForty(t *testing.T) {
	ui := loadDoc(t, "studio.tui")
	if err := ui.Set("view", "cpu"); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ rows, h int }{{12, 9}, {24, 14}} {
		d, err := ui.Dump(40, c.rows)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, n := range d.Nodes {
			if n.ID != "cores-view" {
				continue
			}
			found = true
			s, _ := json.Marshal(n.Scroll)
			if n.H != c.h || string(s) != `{"y":0,"h":12}` {
				t.Errorf("40x%d: cores-view %dx%d scroll %s", c.rows, n.W, n.H, s)
			}
		}
		if !found {
			t.Errorf("40x%d: no cores-view", c.rows)
		}
	}
}

// Validate() (40, 80, and 120 columns by 24 rows) reports nothing, in
// every tab and both themes.
func TestValidateReportsNothing(t *testing.T) {
	ui := loadDoc(t, "studio.tui")
	for _, theme := range []string{"dark", "light"} {
		if err := ui.Set("@theme", theme); err != nil {
			t.Fatal(err)
		}
		for _, tab := range []string{"overview", "cpu", "memory", "thermal", "disk", "network", "processes", "settings", "trends"} {
			if err := ui.Set("view", tab); err != nil {
				t.Fatal(err)
			}
			for _, d := range ui.Validate() {
				t.Errorf("%s, tab %s: %s", theme, tab, fmt.Sprint(d))
			}
		}
	}
}
