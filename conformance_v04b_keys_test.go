package tuimark_test

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

// SPEC v0.4b §24 conformance tests of phase F6: the key and focus
// defaults of the current language (§8.1, §8.6, §6.10, §6.12). esc closes
// the top modal (160, 161), an open modal traps the keymap (162, 219),
// the hints follow the trap (163), and a tabs takes focus by default
// (164). The 0.4.1 twins (specs/fixtures/v04b/twins) are the frozen
// manifest entries t160-esc-041 … t164-tabs-focusable-041; their events
// are checked here, since a golden holds only the final frame.

const (
	escTUI      = "testdata/v04b/160/esc.tui"
	escJSON     = "testdata/v04b/160/esc.json"
	escTwinTUI  = "specs/fixtures/v04b/twins/esc.tui"
	tabsTUI     = "testdata/v04b/164/tabs.tui"
	tabsJSON    = "testdata/v04b/164/tabs.json"
	tabsTwinTUI = "specs/fixtures/v04b/twins/tabs.tui"
	tabsTwinFoc = "specs/fixtures/v04b/twins/tabs-focusable.tui"
	trapTUI     = "testdata/v04b/219/trap.tui"
	trapJSON    = "testdata/v04b/219/trap.json"
)

// keysFile reads a fixture as text, so a case can edit it before parsing.
func keysFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// keysEdit replaces old, which must occur exactly once in src, with new.
func keysEdit(t *testing.T, src, old, new string) string {
	t.Helper()
	if n := strings.Count(src, old); n != 1 {
		t.Fatalf("%q occurs %d times in the fixture, want 1", old, n)
	}
	return strings.Replace(src, old, new, 1)
}

// keysApp parses src and binds the JSON file data at the root.
func keysApp(t *testing.T, src, data string) *tuimark.App {
	t.Helper()
	app := parseDoc(t, src)
	if data != "" {
		bindJSON(t, app, "", keysFile(t, data))
	}
	return app
}

// keysPlay plays steps at cols×rows with no handlers.
func keysPlay(t *testing.T, app *tuimark.App, cols, rows int, steps ...string) *tuimark.PlayResult {
	t.Helper()
	res, err := app.Play(tuimark.PlayOptions{Cols: cols, Rows: rows, NoHandlers: true, Frames: true}, steps...)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// keysEvents lists events as "step action source value", the value as JSON.
func keysEvents(evs []tuimark.PlayEvent) []string {
	out := []string{}
	for _, e := range evs {
		v, _ := json.Marshal(e.Value)
		out = append(out, fmt.Sprintf("%d %s %s %s", e.Step, e.Action, e.Source, v))
	}
	return out
}

func keysFocus(d *tuimark.Dump) string {
	if d.Focus == nil {
		return ""
	}
	return *d.Focus
}

// keysHints lists the items of the hints node id as "KEY LABEL": the
// generated rows after it in the dump, each with a .hint-key and a
// .hint-label text.
func keysHints(t *testing.T, d *tuimark.Dump, id string) []string {
	t.Helper()
	at := -1
	for i, n := range d.Nodes {
		if n.ID == id {
			at = i
		}
	}
	if at < 0 {
		t.Fatalf("no node %q in the dump:\n%s", id, strings.Join(d.Grid, "\n"))
	}
	h := d.Nodes[at]
	inside := func(n tuimark.DumpNode) bool {
		return n.ID == "" && n.X >= h.X && n.Y >= h.Y && n.X+n.W <= h.X+h.W && n.Y+n.H <= h.Y+h.H
	}
	items := []string{}
	for i := at + 1; i < len(d.Nodes) && inside(d.Nodes[i]); i++ {
		n := d.Nodes[i]
		switch {
		case n.Tag == "row":
			items = append(items, "")
		case n.Tag == "text" && len(items) > 0 && reflect.DeepEqual(n.Classes, []string{"hint-key"}):
			items[len(items)-1] = n.Text
		case n.Tag == "text" && len(items) > 0 && reflect.DeepEqual(n.Classes, []string{"hint-label"}):
			items[len(items)-1] += " " + n.Text
		default:
			return items
		}
	}
	return items
}

// 160. esc closes the top modal: the runtime writes false to the path of
// open (or of bind), the next frame fires on:close and gives focus back.
func TestEscClosesTopModal(t *testing.T) {
	src := keysFile(t, escTUI)

	t.Run("close", func(t *testing.T) {
		res := keysPlay(t, keysApp(t, src, escJSON), 30, 10, "esc")
		got, err := json.Marshal(res.Events)
		if err != nil {
			t.Fatal(err)
		}
		want := `[{"step":0,"action":"opened","source":"dlg","keys":{},"value":null},{"step":1,"action":"closed","source":"dlg","keys":{},"value":null}]`
		if string(got) != want {
			t.Errorf("events = %s, want %s", got, want)
		}
		if dumpNode(res.Dump, "dlg") != nil {
			t.Error("the dump still has #dlg")
		}
		if f := keysFocus(res.Dump); f != "list" {
			t.Errorf("focus = %q, want list", f)
		}
	})

	t.Run("store", func(t *testing.T) {
		app := keysApp(t, src, escJSON)
		keysPlay(t, app, 30, 10, "esc")
		for path, want := range map[string]any{"ui.dlg": false, "ui.closed": false, "ui.other": true} {
			if v, ok := app.Get(path); !ok || v != want {
				t.Errorf("Get(%q) = %v, %v; want %v", path, v, ok, want)
			}
		}
	})

	t.Run("next key", func(t *testing.T) {
		for _, c := range []struct {
			steps []string
			want  []string
		}{
			{[]string{"esc", "esc"}, []string{"0 opened dlg null", "1 closed dlg null", "2 esc_row list null"}},
			{[]string{"esc", "q"}, []string{"0 opened dlg null", "1 closed dlg null", "2 quit list null"}},
		} {
			res := keysPlay(t, keysApp(t, src, escJSON), 30, 10, c.steps...)
			if got := keysEvents(res.Events); !reflect.DeepEqual(got, c.want) {
				t.Errorf("%q: events = %q, want %q", c.steps, got, c.want)
			}
		}
	})

	t.Run("from an input", func(t *testing.T) {
		in := keysEdit(t, src, `<button id="ok" label="ok" on:click="ok"/>`, `<input id="name" bind="name"/>`)
		res := keysPlay(t, keysApp(t, in, escJSON), 30, 10, "text:ab", "esc")
		want := []string{"0 opened dlg null", "2 closed dlg null"}
		if got := keysEvents(res.Events); !reflect.DeepEqual(got, want) {
			t.Errorf("events = %q, want %q", got, want)
		}
		if f := keysFocus(res.Dump); f != "list" {
			t.Errorf("focus = %q, want list", f)
		}
	})

	t.Run("variants that do not close", func(t *testing.T) {
		for _, c := range []struct {
			name, old, new string
			want           []string
		}{
			{"negated open", `open="ui.dlg"`, `open="!ui.closed"`, []string{"0 opened dlg null"}},
			{"literal open", `open="ui.dlg"`, `open="true"`, []string{"0 opened dlg null"}},
			{"on:escape", `on:close="closed"`, `on:close="closed" on:escape="dismiss"`, []string{"0 opened dlg null", "1 dismiss dlg null"}},
		} {
			res := keysPlay(t, keysApp(t, keysEdit(t, src, c.old, c.new), escJSON), 30, 10, "esc")
			if got := keysEvents(res.Events); !reflect.DeepEqual(got, c.want) {
				t.Errorf("%s: events = %q, want %q", c.name, got, c.want)
			}
			if dumpNode(res.Dump, "dlg") == nil {
				t.Errorf("%s: the modal closed", c.name)
			}
		}
	})

	t.Run("variants that close", func(t *testing.T) {
		for _, c := range []struct{ name, new string }{
			{"bind", `bind="ui.dlg"`},
			{"open and bind", `open="ui.dlg" bind="ui.other"`},
		} {
			app := keysApp(t, keysEdit(t, src, `open="ui.dlg"`, c.new), escJSON)
			res := keysPlay(t, app, 30, 10, "esc")
			want := []string{"0 opened dlg null", "1 closed dlg null"}
			if got := keysEvents(res.Events); !reflect.DeepEqual(got, want) {
				t.Errorf("%s: events = %q, want %q", c.name, got, want)
			}
			if v, _ := app.Get("ui.dlg"); v != false {
				t.Errorf("%s: ui.dlg = %v, want false", c.name, v)
			}
			if v, _ := app.Get("ui.other"); v != true {
				t.Errorf("%s: ui.other = %v, want true (only the open path is written)", c.name, v)
			}
		}
	})

	t.Run("twin", func(t *testing.T) {
		res := keysPlay(t, loadWithData(t, escTwinTUI, escJSON), 30, 10, "esc")
		want := []string{"0 opened dlg null", "1 esc_row ok null"}
		if got := keysEvents(res.Events); !reflect.DeepEqual(got, want) {
			t.Errorf("events = %q, want %q", got, want)
		}
		if g := geom(*mustNode(t, res.Dump, "dlg")); g != "24x8@(3,1)" {
			t.Errorf("#dlg = %s, want 24x8@(3,1)", g)
		}
	})
}

// 161. esc never writes a derived path: the modal stays.
func TestEscKeepsDerivedModal(t *testing.T) {
	for _, c := range []struct {
		path string
		val  any
	}{
		{"ui.dlg", true},
		{"ui", map[string]any{"dlg": true}},
	} {
		t.Run(c.path, func(t *testing.T) {
			app := loadWithData(t, escTUI, escJSON)
			if err := app.Derive(c.path, func(func(string) (any, bool)) (any, error) { return c.val, nil }); err != nil {
				t.Fatal(err)
			}
			res := keysPlay(t, app, 30, 10, "esc")
			want := []string{"0 opened dlg null"}
			if got := keysEvents(res.Events); !reflect.DeepEqual(got, want) {
				t.Errorf("events = %q, want %q", got, want)
			}
			if dumpNode(res.Dump, "dlg") == nil {
				t.Error("the modal closed")
			}
			if v, _ := app.Get("ui.dlg"); v != true {
				t.Errorf("ui.dlg = %v, want true", v)
			}
		})
	}
}

// 162. An open modal traps the keymap: a row matches only with a when
// that matches the chain from the focused node up to the top modal, and
// the built-in tab, shift+tab, and ctrl+c stay.
func TestModalTrapsKeymap(t *testing.T) {
	src := keysFile(t, escTUI)
	for _, c := range []struct {
		key   string
		want  []string
		focus string
	}{
		{"q", []string{"0 opened dlg null"}, "ok"},
		{"x", []string{"0 opened dlg null"}, "ok"},
		{"o", []string{"0 opened dlg null"}, "ok"},
		{"y", []string{"0 opened dlg null", "1 confirm ok null"}, "ok"},
		{"enter", []string{"0 opened dlg null", "1 ok ok null"}, "ok"},
		{"ctrl+c", []string{"0 opened dlg null", "1 quit ok null"}, "ok"},
		{"tab", []string{"0 opened dlg null"}, "ok"},
		{"shift+tab", []string{"0 opened dlg null"}, "ok"},
	} {
		res := keysPlay(t, keysApp(t, src, escJSON), 30, 10, c.key)
		if got := keysEvents(res.Events); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: events = %q, want %q", c.key, got, c.want)
		}
		if f := keysFocus(res.Dump); f != c.focus {
			t.Errorf("%s: focus = %q, want %q", c.key, f, c.focus)
		}
		if dumpNode(res.Dump, "dlg") == nil {
			t.Errorf("%s: the modal closed", c.key)
		}
	}

	t.Run("trapgroups", func(t *testing.T) {
		groups := keysFile(t, "testdata/v04b/162/trapgroups.tui")
		for _, c := range []struct {
			key  string
			want []string
		}{
			{"a", []string{"1 in_group ok null"}},
			{"b", []string{}},
			{"c", []string{"1 own_when ok null"}},
			{"d", []string{}},
		} {
			res := keysPlay(t, keysApp(t, groups, ""), 30, 10, c.key)
			if got := keysEvents(res.Events); !reflect.DeepEqual(got, c.want) {
				t.Errorf("%s: events = %q, want %q", c.key, got, c.want)
			}
		}
	})

	t.Run("twin", func(t *testing.T) {
		for _, c := range []struct{ key, action string }{
			{"q", "quit"}, {"x", "screen_row"}, {"o", "open_dialog"}, {"ctrl+c", "copy"}, {"esc", "esc_row"},
		} {
			res := keysPlay(t, loadWithData(t, escTwinTUI, escJSON), 30, 10, c.key)
			want := []string{"0 opened dlg null", "1 " + c.action + " ok null"}
			if got := keysEvents(res.Events); !reflect.DeepEqual(got, want) {
				t.Errorf("%s: events = %q, want %q", c.key, got, want)
			}
		}
	})
}

// 163. The hints follow the trap: under the modal only the rows that
// would fire show, and esc, which closes the modal, shows no row.
func TestHintsUnderTrap(t *testing.T) {
	t.Run("current", func(t *testing.T) {
		d, err := loadWithData(t, escTUI, escJSON).Dump(30, 10)
		if err != nil {
			t.Fatal(err)
		}
		for _, id := range []string{"mkeys", "keys"} {
			if got, want := keysHints(t, d, id), []string{"y yes"}; !reflect.DeepEqual(got, want) {
				t.Errorf("#%s = %q, want %q", id, got, want)
			}
		}
	})

	t.Run("geometry", func(t *testing.T) {
		d, err := loadWithData(t, escTUI, escJSON).Dump(30, 10)
		if err != nil {
			t.Fatal(err)
		}
		if g := geom(*mustNode(t, d, "dlg")); g != "8x5@(11,2)" {
			t.Errorf("#dlg = %s, want 8x5@(11,2)", g)
		}
		if got, want := d.Grid[2], "y yes      ┌──────┐           "; got != want {
			t.Errorf("grid row 2 = %q, want %q", got, want)
		}
	})

	t.Run("twin", func(t *testing.T) {
		d, err := loadWithData(t, escTwinTUI, escJSON).Dump(30, 10)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := keysHints(t, d, "mkeys"), []string{"q quit", "o open"}; !reflect.DeepEqual(got, want) {
			t.Errorf("#mkeys = %q, want %q", got, want)
		}
		if got, want := keysHints(t, d, "keys"), []string{"q quit", "o open", "esc back"}; !reflect.DeepEqual(got, want) {
			t.Errorf("#keys = %q, want %q", got, want)
		}
	})
}

// 164. A tabs takes focus by default in the current language, and the
// focus stays on its strip when left or right changes the active tab.
func TestTabsFocusableByDefault(t *testing.T) {
	src := keysFile(t, tabsTUI)
	type play struct {
		steps []string
		want  []string
		focus string
	}
	run := func(t *testing.T, src string, cases []play) {
		t.Helper()
		for _, c := range cases {
			res := keysPlay(t, keysApp(t, src, tabsJSON), 30, 6, c.steps...)
			if got := keysEvents(res.Events); !reflect.DeepEqual(got, c.want) {
				t.Errorf("%q: events = %q, want %q", c.steps, got, c.want)
			}
			if f := keysFocus(res.Dump); f != c.focus {
				t.Errorf("%q: focus = %q, want %q", c.steps, f, c.focus)
			}
		}
	}

	t.Run("current", func(t *testing.T) {
		run(t, src, []play{
			{nil, []string{}, "nav"},
			{[]string{"right"}, []string{`1 changed nav "two"`}, "nav"},
			{[]string{"right", "right"}, []string{`1 changed nav "two"`, `2 changed nav "one"`}, "nav"},
			{[]string{"left"}, []string{`1 changed nav "two"`}, "nav"},
			{[]string{"right", "tab"}, []string{`1 changed nav "two"`}, "b2"},
			{[]string{"tab"}, []string{}, "l1"},
			{[]string{"shift+tab"}, []string{}, "l1"},
		})
	})

	t.Run("tab focus", func(t *testing.T) {
		run(t, keysEdit(t, src, `<tab id="one" label="one">`, `<tab id="one" label="one" focus="#l1">`), []play{
			{nil, []string{}, "l1"},
			{[]string{"shift+tab", "right"}, []string{`2 changed nav "two"`}, "nav"},
		})
	})

	t.Run("focusable false", func(t *testing.T) {
		run(t, keysEdit(t, src, `on:select="changed">`, `on:select="changed" focusable="false">`), []play{
			{nil, []string{}, "l1"},
			{[]string{"right"}, []string{}, "l1"},
		})
	})

	t.Run("twin", func(t *testing.T) {
		run(t, keysFile(t, tabsTwinTUI), []play{
			{nil, []string{}, "l1"},
			{[]string{"right"}, []string{}, "l1"},
		})
	})

	t.Run("twin focusable", func(t *testing.T) {
		run(t, keysFile(t, tabsTwinFoc), []play{
			{[]string{"right"}, []string{`1 changed nav "two"`}, "b2"},
			{[]string{"right", "right"}, []string{`1 changed nav "two"`}, "b2"},
		})
	})
}

// 219. The keys that work under a modal too are repeated in a
// <keymap when="modal"> (or on the rows): one help per press, with and
// without the modal, and the hints show each key once.
func TestKeysUnderModalIdiom(t *testing.T) {
	src := keysFile(t, trapTUI)
	steps := []string{"?", "ctrl+c", "set:dlg=true", "?", "ctrl+c"}
	const idiom = `[{"step":1,"action":"help","source":"","keys":{},"value":null},{"step":2,"action":"copy","source":"","keys":{},"value":null},{"step":4,"action":"help","source":"ok","keys":{},"value":null},{"step":5,"action":"copy","source":"ok","keys":{},"value":null}]`
	second := "  <keymap when=\"modal\">\n    <bind keys=\"?\" action=\"help\" label=\"help\"/>\n    <bind keys=\"ctrl+c\" action=\"copy\"/>\n  </keymap>\n"

	// check plays steps over doc and compares the events with want and,
	// frame by frame, #h's items with hints(step).
	check := func(t *testing.T, doc, want string, hints func(step int) []string) {
		t.Helper()
		res := keysPlay(t, keysApp(t, doc, trapJSON), 30, 8, steps...)
		got, err := json.Marshal(res.Events)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Errorf("events = %s\nwant     %s", got, want)
		}
		if f := keysFocus(res.Dump); f != "ok" {
			t.Errorf("focus = %q, want ok", f)
		}
		if len(res.Frames) != len(steps)+1 {
			t.Fatalf("%d frames, want %d", len(res.Frames), len(steps)+1)
		}
		for _, fr := range res.Frames {
			if got, want := keysHints(t, fr.Dump, "h"), hints(fr.Step); !reflect.DeepEqual(got, want) {
				t.Errorf("step %d: #h = %q, want %q", fr.Step, got, want)
			}
		}
	}
	one := func(int) []string { return []string{"? help"} }

	t.Run("validate", func(t *testing.T) {
		if diags := keysApp(t, src, trapJSON).Validate(); len(diags) != 0 {
			t.Errorf("diagnostics = %v, want none", diags)
		}
	})

	t.Run("idiom", func(t *testing.T) {
		check(t, src, idiom, one)
	})

	t.Run("scope all", func(t *testing.T) {
		check(t, keysEdit(t, src, `<hints id="h"/>`, `<hints id="h" scope="all"/>`), idiom, func(int) []string {
			return []string{"? help", "? help"}
		})
	})

	t.Run("without the idiom", func(t *testing.T) {
		want := `[{"step":1,"action":"help","source":"","keys":{},"value":null},{"step":2,"action":"copy","source":"","keys":{},"value":null},{"step":5,"action":"quit","source":"ok","keys":{},"value":null}]`
		check(t, keysEdit(t, src, second, ""), want, func(step int) []string {
			if step >= 3 {
				return []string{}
			}
			return []string{"? help"}
		})
	})

	t.Run("on the rows", func(t *testing.T) {
		doc := keysEdit(t, src, second, "")
		doc = keysEdit(t, doc, "    <bind keys=\"ctrl+c\" action=\"copy\"/>\n  </keymap>\n",
			"    <bind keys=\"ctrl+c\" action=\"copy\"/>\n    <bind keys=\"?\" action=\"help\" label=\"help\" when=\"modal\"/>\n    <bind keys=\"ctrl+c\" action=\"copy\" when=\"modal\"/>\n  </keymap>\n")
		check(t, doc, idiom, one)
	})

	t.Run("pinned", func(t *testing.T) {
		check(t, keysEdit(t, src, "<tui>", `<tui version="3">`), idiom, one)
	})

	t.Run("one selector", func(t *testing.T) {
		for _, attrs := range []string{"", ` version="1"`, ` version="2"`, ` version="3"`} {
			doc := "<tui" + attrs + ">\n  <keymap><bind keys=\"?\" action=\"help\" when=\"screen, modal\"/></keymap>\n  <screen id=\"main\"><text>x</text></screen>\n</tui>\n"
			var got []string
			for _, d := range parseDoc(t, doc).Validate() {
				got = append(got, fmt.Sprintf("%s %s %d:%d %s", d.Severity, d.Code, d.Line, d.Col, d.Msg))
			}
			want := []string{`error V003 2:11 when="screen, modal": selector "screen, modal": unexpected ","`}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("<tui%s>: diagnostics = %q, want %q", attrs, got, want)
			}
		}
	})
}
