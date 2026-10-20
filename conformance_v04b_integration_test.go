package tuimark_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

// SPEC v0.4b §24 conformance tests closed by phase F12 (integration),
// through the Go API: 215 (a button in a copy), 223 (a kit fragment end to
// end), 224 (a template root's size by its place), and 225 (a misplaced
// copy, and a form in a modal). They hold together what F1-F11 built
// apart: composition, button labels, the trap, esc, and the forms.
//
// Decision 102 (a diagnostic about a placed element names its placement)
// is checked by 225's misplaced bullet, and decision 108 (a template's
// root writes its own size) by 224's convention bullet.
//
// The fixtures are testdata/v04b/t215, t223, t224, and t225, the SPEC's
// text verbatim. The CLI parts (validate --json and its exit code, and
// inspect --json) are cmd/tuimark's conformance_v04b_integration_test.go,
// with tests 130 and 132.

const f12Dir = "testdata/v04b/"

// f12Play plays steps on app at cols×rows as tuimark play does: host
// actions are recorded, not handled.
func f12Play(t *testing.T, app *tuimark.App, cols, rows int, frames bool, steps ...string) *tuimark.PlayResult {
	t.Helper()
	res, err := app.Play(tuimark.PlayOptions{Cols: cols, Rows: rows, NoHandlers: true, Frames: frames}, steps...)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// f12Copy copies the fixture directory dir into a temporary directory,
// rewrites file there with old replaced by new (each once), and returns
// the rewritten file's path. A document that includes a fragment by a
// relative path needs its directory, which a lone temporary file lacks.
func f12Copy(t *testing.T, dir, file string, edits ...string) string {
	t.Helper()
	tmp := t.TempDir()
	err := filepath.WalkDir(dir, func(path string, e os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		if e.IsDir() {
			return os.MkdirAll(filepath.Join(tmp, rel), 0o755)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(tmp, rel), b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(tmp, file)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)
	for i := 0; i+1 < len(edits); i += 2 {
		if !strings.Contains(src, edits[i]) {
			t.Fatalf("%s holds no %q", file, edits[i])
		}
		src = strings.Replace(src, edits[i], edits[i+1], 1)
	}
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// f12Node is the node with id in d, or the zero node and false.
func f12Node(d *tuimark.Dump, id string) (tuimark.DumpNode, bool) {
	for _, n := range d.Nodes {
		if n.ID == id {
			return n, true
		}
	}
	return tuimark.DumpNode{}, false
}

// f12Hints is the key and label texts of the hint items in d, in order,
// as "KEY LABEL".
func f12Hints(d *tuimark.Dump) []string {
	out := []string{}
	key := ""
	for _, n := range d.Nodes {
		for _, c := range n.Classes {
			switch c {
			case "hint-key":
				key = n.Text
			case "hint-label":
				out = append(out, key+" "+n.Text)
			}
		}
	}
	return out
}

func f12Diags(app *tuimark.App) []string {
	out := []string{}
	for _, d := range app.Validate() {
		out = append(out, d.Severity+" "+diagText(d))
	}
	return out
}

// 215. A button in a copy: each copy's labels resolve through its own
// with alias, the widths follow the data, a missing path is one B003 in
// the via form, and an event's keys hold no with alias.
func TestV04bIntegration215ButtonInACopy(t *testing.T) {
	dir := f12Dir + "t215/"
	app := f5Load(t, dir+"kit.tui", dir+"kit.json")
	if got := f12Diags(app); len(got) != 0 {
		t.Errorf("validate: %q", got)
	}
	d := f5Dump(t, app, 30, 3)
	if want := []string{"[ Delete 3 tasks ][ Cancel ]  ", "[ Archive 12 tasks ][ Keep ]  ", "                              "}; !reflect.DeepEqual(d.Grid, want) {
		t.Errorf("grid %q", d.Grid)
	}
	if d.Focus == nil || *d.Focus != "del__ok" {
		t.Errorf("focus %v, want del__ok", d.Focus)
	}
	for id, want := range map[string]string{
		"del__ok": "18x1 at (0,0) Delete 3 tasks",
		"del__no": "10x1 at (18,0) Cancel",
		"arc__ok": "20x1 at (0,1) Archive 12 tasks",
		"arc__no": "8x1 at (20,1) Keep",
	} {
		n, ok := f12Node(d, id)
		if got := f5Geo(n) + " " + n.Text; !ok || got != want || n.Tag != "button" {
			t.Errorf("%s: %s %q, want %q", id, n.Tag, got, want)
		}
	}

	noCount := f5Load(t, dir+"kit.tui", "")
	if err := noCount.Bind("", map[string]any{"dialogs": map[string]any{
		"delete":  map[string]any{"verb": "Delete", "count": 3.0, "cancel": "Cancel"},
		"archive": map[string]any{"verb": "Archive", "cancel": "Keep"},
	}}); err != nil {
		t.Fatal(err)
	}
	want := []string{`warning B003 kit.tui:4:7 /screen/row/button id=arc__ok bind path "c.count" is missing (dialogs.archive.count via <use id="arc">)`}
	if got := f12Diags(noCount); !reflect.DeepEqual(got, want) {
		t.Errorf("without count:\n got %q\nwant %q", got, want)
	}
	if n, _ := f12Node(f5Dump(t, noCount, 30, 3), "arc__ok"); n.W != 18 || n.H != 1 || n.Text != "Archive  tasks" {
		t.Errorf("arc__ok without count: %s %q", f5Geo(n), n.Text)
	}

	res := f12Play(t, f5Load(t, dir+"kit.tui", dir+"kit.json"), 30, 3, false, "tab", "enter")
	if got, want := eventsJSON(t, res.Events), `[{"step":2,"action":"cancel","source":"del__no","keys":{},"value":null}]`; got != want {
		t.Errorf("events %s, want %s", got, want)
	}
}

// 223. A kit fragment, end to end: the fragment's keymap row comes first
// in the hints, the top copy's focus= holds the focus and is the event's
// source, the trap holds q, and esc writes false through the with alias,
// closing only the top modal. (The stylesheet bullet, an inspect entry, is
// cmd/tuimark's.)
func TestV04bIntegration223KitFragment(t *testing.T) {
	dir := f12Dir + "t223/"
	app, d1, both := dir+"app.tui", dir+"d.json", dir+"both.json"
	if got := f12Diags(f5Load(t, app, d1)); len(got) != 0 {
		t.Errorf("validate: %q", got)
	}

	// Hints: scope="all" lists the fragment's row first, then the
	// document's; scope="active" only what fires now.
	if got := f12Hints(f5Dump(t, f5Load(t, app, d1), 80, 24)); !reflect.DeepEqual(got, []string{"y yes", "q quit"}) {
		t.Errorf("hints: %q", got)
	}
	active := f12Copy(t, dir, "app.tui", `scope="all"`, `scope="active"`)
	if got := f12Hints(f5Dump(t, f5Load(t, active, d1), 80, 24)); !reflect.DeepEqual(got, []string{"y yes"}) {
		t.Errorf("active hints with a dialog open: %q", got)
	}
	res := f12Play(t, f5Load(t, active, d1), 80, 24, false, "esc")
	if got := f12Hints(res.Dump); !reflect.DeepEqual(got, []string{"q quit"}) {
		t.Errorf("active hints after esc: %q", got)
	}

	// The top copy.
	for _, c := range []struct{ data, focus string }{{both, "arc__no"}, {d1, "del__no"}} {
		a := f5Load(t, app, c.data)
		if d := f5Dump(t, a, 80, 24); d.Focus == nil || *d.Focus != c.focus {
			t.Errorf("%s: focus %v, want %s", c.data, d.Focus, c.focus)
		}
		res := f12Play(t, a, 80, 24, false, "y")
		if got, want := eventsJSON(t, res.Events), `[{"step":1,"action":"dialog_confirm","source":"`+c.focus+`","keys":{},"value":null}]`; got != want {
			t.Errorf("%s: y fires %s, want %s", c.data, got, want)
		}
	}

	// The trap.
	if res := f12Play(t, f5Load(t, app, d1), 80, 24, false, "q"); len(res.Events) != 0 {
		t.Errorf("q under the dialog fired %s", eventsJSON(t, res.Events))
	}

	// esc writes through the alias.
	res = f12Play(t, f5Load(t, app, d1), 80, 24, true, "esc", "q")
	if got, want := eventsJSON(t, res.Events), `[{"step":1,"action":"dialog_closed","source":"del__dialog","keys":{},"value":null},{"step":2,"action":"quit","source":"","keys":{},"value":null}]`; got != want {
		t.Errorf("esc q: %s, want %s", got, want)
	}
	if len(res.Frames) < 2 {
		t.Fatalf("%d frames", len(res.Frames))
	}
	if _, ok := f12Node(res.Frames[1].Dump, "del__dialog"); ok {
		t.Error("the frame of step 1 still has del__dialog")
	}
	a := f5Load(t, app, d1)
	f12Play(t, a, 80, 24, false, "esc")
	if v, ok := a.Get("ui.dialogs.del.open"); !ok || v != false {
		t.Errorf("ui.dialogs.del.open = %v (%v), want false", v, ok)
	}
	if v, ok := a.Get("ui.dialogs.arc.open"); !ok || v != false {
		t.Errorf("ui.dialogs.arc.open = %v (%v), want false", v, ok)
	}
	if v, ok := a.Get("d"); ok {
		t.Errorf("the store has a member d: %v", v)
	}

	// Only the top modal closes.
	a = f5Load(t, app, both)
	res = f12Play(t, a, 80, 24, false, "esc")
	if got, want := eventsJSON(t, res.Events), `[{"step":1,"action":"dialog_closed","source":"arc__dialog","keys":{},"value":null}]`; got != want {
		t.Errorf("esc with both open: %s, want %s", got, want)
	}
	if _, ok := f12Node(res.Dump, "del__dialog"); !ok {
		t.Error("del__dialog closed too")
	}
	if v, _ := a.Get("ui.dialogs.del.open"); v != true {
		t.Errorf("ui.dialogs.del.open = %v, want true", v)
	}
}

// 224. A template root's size by its place: the same row is 1fr directly
// in a screen or a tab and content-sized in a col, exactly as the hand
// expansion is; and a root that writes its height (decision 108's
// convention) is that size wherever it is placed.
func TestV04bIntegration224TemplateRootSize(t *testing.T) {
	raw, err := os.ReadFile(f12Dir + "t224/size.tui")
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)
	const lines = "    <use template=\"bar\" with=\"a as b\"/>\n    <text id=\"after\">after</text>\n"
	const hand = "    <row class=\"bar\"><text>top</text></row>\n    <text id=\"after\">after</text>\n"
	if !strings.Contains(src, lines) {
		t.Fatal("size.tui changed")
	}
	places := []struct {
		name string
		wrap func(string) string
		want map[string]string // bar, after; then with height: 1
		conv map[string]string
	}{
		{"screen", func(s string) string { return s },
			map[string]string{"bar": "20x5 at (0,0)", "after": "20x1 at (0,5)"},
			map[string]string{"bar": "20x1 at (0,0)", "after": "20x1 at (0,1)"}},
		{"tab", func(s string) string {
			return "    <tabs id=\"t\" style=\"height: 6\"><tab id=\"one\" label=\"one\">\n" + s + "    </tab></tabs>\n"
		},
			map[string]string{"bar": "20x4 at (0,1)", "after": "20x1 at (0,5)"},
			map[string]string{"bar": "20x1 at (0,1)", "after": "20x1 at (0,2)"}},
		{"col", func(s string) string { return "    <col id=\"c\">\n" + s + "    </col>\n" },
			map[string]string{"c": "20x6 at (0,0)", "bar": "20x1 at (0,0)", "after": "20x1 at (0,1)"},
			map[string]string{"c": "20x6 at (0,0)", "bar": "20x1 at (0,0)", "after": "20x1 at (0,1)"}},
	}
	data := f12Dir + "t224/size.json"
	geo := func(d *tuimark.Dump) map[string]string {
		out := map[string]string{}
		for _, n := range d.Nodes {
			key := n.ID
			if reflect.DeepEqual(n.Classes, []string{"bar"}) {
				key = "bar"
			}
			if key != "" {
				if _, seen := out[key]; !seen {
					out[key] = f5Geo(n)
				}
			}
		}
		return out
	}
	for _, p := range places {
		for _, conv := range []bool{false, true} {
			base, written, want := src, hand, p.want
			if conv {
				// The template's root and the hand-written row both write it.
				const row, sized = `<row class="bar">`, `<row class="bar" style="height: 1">`
				base = strings.Replace(src, row, sized, 1)
				written = strings.Replace(hand, row, sized, 1)
				want = p.conv
			}
			doc := strings.Replace(base, lines, p.wrap(lines), 1)
			flat := strings.Replace(base, lines, p.wrap(written), 1)
			label := fmt.Sprintf("%s (height: 1 %v)", p.name, conv)
			app := f5Parse(t, doc, data)
			if got := f12Diags(app); len(got) != 0 {
				t.Errorf("%s: %q", label, got)
			}
			d := f5Dump(t, app, 20, 6)
			got := geo(d)
			for k, w := range want {
				if got[k] != w {
					t.Errorf("%s: %s is %s, want %s", label, k, got[k], w)
				}
			}
			h := f5Dump(t, f5Parse(t, flat, data), 20, 6)
			a, _ := json.Marshal(d)
			b, _ := json.Marshal(h)
			if string(a) != string(b) {
				t.Errorf("%s: the dump differs from the hand expansion:\n%s\n%s", label, a, b)
			}
		}
	}
}

// 225. A misplaced copy is reported at the template's element with the
// placement suffix (decision 102), and moving the use removes it; a form
// in a modal keeps the modal's focus cycle, and its <keymap when> rows
// fire from every control while the trap holds the rest.
func TestV04bIntegration225MisplacedAndForm(t *testing.T) {
	dir := f12Dir + "t225/"
	want := []string{`error L004 kit/kit.tui:7:5 /screen/col/modal id=del__dialog <modal> must be a direct child of <screen> (and come last), placed by <use id="del"> at app.tui:12:7`}
	if got := f12Diags(f5Load(t, dir+"app.tui", dir+"d.json")); !reflect.DeepEqual(got, want) {
		t.Errorf("misplaced:\n got %q\nwant %q", got, want)
	}
	moved := f12Copy(t, dir, "app.tui",
		"      <use id=\"del\" template=\"k-confirm\" with=\"ui.dialogs.del as d\"/>\n    </col>\n",
		"    </col>\n    <use id=\"del\" template=\"k-confirm\" with=\"ui.dialogs.del as d\"/>\n")
	if got := f12Diags(f5Load(t, moved, dir+"d.json")); len(got) != 0 {
		t.Errorf("moved: %q", got)
	}

	form, data := dir+"form.tui", dir+"form.json"
	load := func() *tuimark.App { return f5Load(t, form, data) }
	if got := f12Diags(load()); len(got) != 0 {
		t.Errorf("form: %q", got)
	}
	if d := f5Dump(t, load(), 80, 24); d.Focus == nil || *d.Focus != "name" {
		t.Errorf("focus %v, want name", d.Focus)
	}
	res := f12Play(t, load(), 80, 24, true, "tab", "tab", "tab", "tab")
	var foci []string
	for _, f := range res.Frames[1:] {
		if f.Dump.Focus == nil {
			foci = append(foci, "")
		} else {
			foci = append(foci, *f.Dump.Focus)
		}
	}
	if want := []string{"news", "tz", "size", "name"}; !reflect.DeepEqual(foci, want) {
		t.Errorf("tab order %q, want %q", foci, want)
	}

	saves := `[{"step":1,"action":"save","source":"name","keys":{},"value":""},{"step":3,"action":"save","source":"news","keys":{},"value":false},{"step":5,"action":"save","source":"tz","keys":{},"value":null},{"step":7,"action":"save","source":"size","keys":{"it":"s"},"value":null}]`
	cases := []struct {
		steps  string
		events string
	}{
		{"ctrl+s tab ctrl+s tab ctrl+s tab ctrl+s", saves},
		{"tab enter", `[{"step":2,"action":"save","source":"news","keys":{},"value":false}]`},
		{"tab space", `[]`},
		{"tab tab enter", `[]`},
		{"tab tab tab space", `[]`},
	}
	for _, c := range cases {
		res := f12Play(t, load(), 80, 24, false, strings.Fields(c.steps)...)
		if got := eventsJSON(t, res.Events); got != c.events {
			t.Errorf("%s: %s, want %s", c.steps, got, c.events)
		}
		news, _ := f12Node(res.Dump, "news")
		if checked := c.steps == "tab space"; news.Checked != checked {
			t.Errorf("%s: news checked %v", c.steps, news.Checked)
		}
		if c.steps == "tab tab enter" {
			// The select takes enter and opens its option list: a list
			// without an id, after every other node (SPEC v0.4b §6.17.6).
			var opts []string
			open := false
			for _, n := range res.Dump.Nodes {
				if n.Tag == "list" && n.ID == "" {
					open = true
				} else if open && n.Tag == "item" {
					opts = append(opts, n.Key)
				}
			}
			if want := []string{"utc", "lis"}; !reflect.DeepEqual(opts, want) {
				t.Errorf("tab tab enter: the select's open options %q, want %q", opts, want)
			}
		}
		if c.steps == "tab tab tab space" {
			var rows []string
			for _, n := range res.Dump.Nodes {
				if n.Tag == "item" {
					rows = append(rows, fmt.Sprintf("%s %v", n.Key, n.Checked))
				}
			}
			if want := []string{"s true", "m false"}; !reflect.DeepEqual(rows, want) {
				t.Errorf("the radio's rows %q, want %q", rows, want)
			}
		}
	}
	res = f12Play(t, load(), 80, 24, false, "tab", "tab", "tab", "enter")
	if got, want := eventsJSON(t, res.Events), `[{"step":4,"action":"save","source":"size","keys":{"it":"s"},"value":null}]`; got != want {
		t.Errorf("enter on the radio: %s, want %s", got, want)
	}

	// Without when=".form" the trap holds every row.
	raw, err := os.ReadFile(form)
	if err != nil {
		t.Fatal(err)
	}
	bare := strings.Replace(string(raw), `<keymap when=".form">`, `<keymap>`, 1)
	for _, steps := range []string{"ctrl+s tab ctrl+s tab ctrl+s tab ctrl+s", "tab enter", "tab space", "tab tab enter", "tab tab tab space", "tab tab tab enter"} {
		res := f12Play(t, f5Parse(t, bare, data), 80, 24, false, strings.Fields(steps)...)
		for _, e := range res.Events {
			if e.Action == "save" {
				t.Errorf("without when, %s fired save: %s", steps, eventsJSON(t, res.Events))
			}
		}
	}
}
