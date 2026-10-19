package tuimark_test

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

// SPEC v0.4b §24 conformance tests for phase F8, forms I: single="true" on
// a list or a table (§6.14.1) and the <checkbox> (§6.16). Tests 128 and
// 173-178 close here, with test 186's and test 222's checkbox parts; the
// select halves of 173, 186, and 222 are phase F9's, at the end of this
// file. The fixtures are testdata/v04b/174/checkbox.tui (test 174's text),
// 177/radio.tui, and 178/radio-table.tui; variants are derived from their
// text.

const formsDir = "testdata/v04b/"

func formsSource(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(formsDir + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// eventsJSON is a play's events as tuimark play prints them, compact.
func eventsJSON(t *testing.T, evs []tuimark.PlayEvent) string {
	t.Helper()
	if evs == nil {
		evs = []tuimark.PlayEvent{}
	}
	b, err := json.Marshal(evs)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// formsPlay plays steps on a fresh app of src with data at cols×rows.
func formsPlay(t *testing.T, src, data string, cols, rows int, steps ...string) *tuimark.PlayResult {
	t.Helper()
	res, err := btnApp(t, src, data).Play(tuimark.PlayOptions{Cols: cols, Rows: rows, NoHandlers: true}, steps...)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// formsStyled is formsPlay with --styles.
func formsStyled(t *testing.T, src, data string, cols, rows int, steps ...string) *tuimark.PlayResult {
	t.Helper()
	res, err := btnApp(t, src, data).Play(tuimark.PlayOptions{Cols: cols, Rows: rows, NoHandlers: true, Styles: true}, steps...)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// spansJSON is one row of a dump's styles, compact.
func spansJSON(t *testing.T, d *tuimark.Dump, row int) string {
	t.Helper()
	b, err := json.Marshal(d.Styles[row])
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// hasDiag reports whether ds holds a diagnostic with code whose message
// contains msg.
func hasDiag(ds []tuimark.Diagnostic, code, msg string) bool {
	for _, d := range ds {
		if d.Code == code && strings.Contains(d.Msg, msg) {
			return true
		}
	}
	return false
}

func codesOf(ds []tuimark.Diagnostic) []string {
	out := []string{}
	for _, d := range ds {
		out = append(out, d.Code)
	}
	return out
}

// focusOf is a dump's focus, "" when nothing is focused.
func focusOf(d *tuimark.Dump) string {
	if d.Focus == nil {
		return ""
	}
	return *d.Focus
}

func hasNode(d *tuimark.Dump, tag string) bool {
	for _, n := range d.Nodes {
		if n.Tag == tag {
			return true
		}
	}
	return false
}

const checkboxData = `{"settings": {"dark": true}}`

// 128. The current language and pinned documents.
func TestFormsCurrentLanguageAndPinned(t *testing.T) {
	const data = `{"on": false, "zones": []}`
	const cur = `<tui><screen><checkbox id="c" bind="on" label="x"/></screen></tui>`
	if d := btnApp(t, cur, data).Validate(); len(d) != 0 {
		t.Errorf("current: %v", diagLines(d))
	}
	for _, v := range []string{"1", "2", "3"} {
		src := strings.Replace(cur, "<tui>", `<tui version="`+v+`">`, 1)
		app := btnApp(t, src, data)
		ds := app.Validate()
		if len(ds) != 1 || ds[0].Code != "V001" || ds[0].Col != 26 || !strings.HasSuffix(ds[0].Msg, " (requires a <tui> without version)") {
			t.Errorf("version=%s: %v", v, diagLines(ds))
		}
		d, err := app.Dump(20, 2)
		if err != nil {
			t.Fatal(err)
		}
		if hasNode(d, "checkbox") {
			t.Errorf("version=%s: the dump has a checkbox node", v)
		}
	}

	// single in a version="3" document: V002 with the hint, and the list
	// stays a multi-select with checked.
	const pinned = `<tui version="3"><keymap><bind keys="t" action="check-toggle"/></keymap><screen focus="#l">` +
		`<list id="l" each="zones as z" key="z" checked="pick" single="true"><item><text>{z}</text></item></list></screen></tui>`
	ds := btnApp(t, pinned, `{"zones": [], "pick": []}`).Validate()
	if len(ds) != 1 || ds[0].Code != "V002" || ds[0].Msg != `attribute "single" on <list> (requires a <tui> without version)` {
		t.Errorf("pinned single: %v", diagLines(ds))
	}
	res := formsPlay(t, pinned, `{"zones": ["a", "b"], "pick": []}`, 20, 3, "space", "t")
	if focusOf(res.Dump) != "l" || eventsJSON(t, res.Events) != "[]" {
		t.Errorf("pinned single: focus %q, events %s", focusOf(res.Dump), eventsJSON(t, res.Events))
	}
	app := btnApp(t, pinned, `{"zones": ["a", "b"], "pick": []}`)
	if _, err := app.Play(tuimark.PlayOptions{Cols: 20, Rows: 3, NoHandlers: true}, "space", "t"); err != nil {
		t.Fatal(err)
	}
	if got, _ := app.Get("pick"); !reflect.DeepEqual(got, []any{"a"}) {
		t.Errorf("pinned single: check-toggle wrote %v, want the array [a]", got)
	}

	// version="4": V003 naming the three versions; read as version="1",
	// where the checkbox is V001 with the 0.4b hint.
	ds = btnApp(t, strings.Replace(cur, "<tui>", `<tui version="4">`, 1), data).Validate()
	if len(ds) != 2 || ds[0].Code != "V003" || !strings.Contains(ds[0].Msg, `"1", "2", or "3"`) ||
		ds[1].Code != "V001" || !strings.HasSuffix(ds[1].Msg, " (requires a <tui> without version)") {
		t.Errorf("version=4: %v", diagLines(ds))
	}
}

// 173. The version gate, for the checkbox and single (the select parts
// are TestSelectVersionGate).
func TestFormsVersionGate(t *testing.T) {
	doc := func(version, body string) string {
		v := ""
		if version != "" {
			v = ` version="` + version + `"`
		}
		return `<tui` + v + `><screen>` + body + `</screen></tui>`
	}
	const hint = " (requires a <tui> without version)"
	ds := btnApp(t, doc("3", `<checkbox id="a"/>`), "").Validate()
	if len(ds) != 1 || ds[0].Code != "V001" || ds[0].Msg != "tag <checkbox>"+hint {
		t.Errorf("checkbox in version=3: %v", diagLines(ds))
	}
	// Not checked further: no V012 without id, no V016 with children.
	ds = btnApp(t, doc("3", `<checkbox><text>x</text></checkbox>`), "").Validate()
	if !reflect.DeepEqual(codesOf(ds), []string{"V001"}) {
		t.Errorf("checkbox without id, with a child, in version=3: %v", diagLines(ds))
	}
	const single = `<list id="l" each="xs as x" key="x" checked="c" single="true"><item/></list>`
	ds = btnApp(t, doc("3", single), `{"xs": [], "c": []}`).Validate()
	if len(ds) != 1 || ds[0].Code != "V002" || !strings.HasSuffix(ds[0].Msg, hint) {
		t.Errorf("single in version=3: %v", diagLines(ds))
	}
	const table = `<table id="t" each="xs as x" key="x" checked="c" single="true"><column>{x}</column></table>`
	// version="1": only the table's own V001, no V002 for single.
	ds = btnApp(t, doc("1", `<table id="t" each="xs as x" key="x" checked="c" single="true"/>`), "").Validate()
	if !reflect.DeepEqual(codesOf(ds), []string{"V001"}) || ds[0].Msg != `tag <table> (requires version="2")` {
		t.Errorf("single on a table in version=1: %v", diagLines(ds))
	}
	const sheet = `<style>checkbox { color: red; }</style>`
	ds = btnApp(t, `<tui version="3">`+sheet+`<screen/></tui>`, "").Validate()
	if len(ds) != 1 || ds[0].Code != "V003" || !strings.HasSuffix(ds[0].Msg, hint) {
		t.Errorf("checkbox selector in version=3: %v", diagLines(ds))
	}
	for _, body := range []string{`<checkbox id="a"/>`, single, table} {
		if ds := btnApp(t, doc("", body), `{"xs": [], "c": null}`).Validate(); len(ds) != 0 {
			t.Errorf("current %s: %v", body, diagLines(ds))
		}
	}
	if ds := btnApp(t, `<tui>`+sheet+`<screen/></tui>`, "").Validate(); len(ds) != 0 {
		t.Errorf("checkbox selector in the current language: %v", diagLines(ds))
	}
	// The same type selector in a keymap's when.
	const when = `<keymap><bind keys="x" action="go" when="checkbox:focus"/></keymap>`
	ds = btnApp(t, `<tui version="3">`+when+`<screen/></tui>`, "").Validate()
	if len(ds) != 1 || ds[0].Code != "V003" || !strings.HasSuffix(ds[0].Msg, hint) {
		t.Errorf("checkbox selector in a version=3 when: %v", diagLines(ds))
	}
	if ds := btnApp(t, `<tui>`+when+`<screen/></tui>`, "").Validate(); len(ds) != 0 {
		t.Errorf("checkbox selector in a current when: %v", diagLines(ds))
	}
	// The HTML-name hints (decision 67).
	ds = btnApp(t, doc("", `<radio/>`), "").Validate()
	if len(ds) != 1 || ds[0].Code != "V001" || !strings.Contains(ds[0].Msg, `a radio group is a <list> with checked= and single="true"`) {
		t.Errorf("radio in the current language: %v", diagLines(ds))
	}
	ds = btnApp(t, doc("3", `<radio/>`), "").Validate()
	if len(ds) != 1 || ds[0].Code != "V001" || !strings.Contains(ds[0].Msg, "a <list> with on:select picks one row") || strings.Contains(ds[0].Msg, "single") {
		t.Errorf("radio in version=3: %v", diagLines(ds))
	}
}

// 174. Checkbox.
func TestCheckbox(t *testing.T) {
	src := formsSource(t, "174/checkbox.tui")
	app := btnApp(t, src, checkboxData)
	if ds := app.Validate(); len(ds) != 0 {
		t.Fatalf("Validate: %v", diagLines(ds))
	}
	d, err := app.Dump(20, 3)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"[x] Dark mode       ", "[ ] Beta            ", "                    "}; !reflect.DeepEqual(d.Grid, want) {
		t.Errorf("grid %q, want %q", d.Grid, want)
	}
	nodes, _ := json.Marshal(d.Nodes)
	const wantNodes = `[{"id":"main","tag":"screen","x":0,"y":0,"w":20,"h":3},` +
		`{"id":"dark","tag":"checkbox","x":0,"y":0,"w":20,"h":1,"text":"Dark mode","focused":true,"checked":true},` +
		`{"id":"beta","tag":"checkbox","x":0,"y":1,"w":20,"h":1,"text":"Beta"}]`
	if string(nodes) != wantNodes {
		t.Errorf("nodes %s\nwant  %s", nodes, wantNodes)
	}

	res := formsPlay(t, src, checkboxData, 20, 3, "space")
	if res.Dump.Grid[0] != "[ ] Dark mode       " || btnNode(t, res.Dump, "dark").Checked ||
		eventsJSON(t, res.Events) != `[{"step":1,"action":"dark_changed","source":"dark","keys":{},"value":false}]` {
		t.Errorf("space: %q %s", res.Dump.Grid[0], eventsJSON(t, res.Events))
	}
	res = formsPlay(t, src, checkboxData, 20, 3, "space", "space")
	if got := eventsJSON(t, res.Events); got != `[{"step":1,"action":"dark_changed","source":"dark","keys":{},"value":false},{"step":2,"action":"dark_changed","source":"dark","keys":{},"value":true}]` {
		t.Errorf("space space: %s", got)
	}
	// enter is not consumed: it goes on to the keymap (decision 97).
	res = formsPlay(t, src, checkboxData, 20, 3, "enter")
	if eventsJSON(t, res.Events) != "[]" || !btnNode(t, res.Dump, "dark").Checked {
		t.Errorf("enter: %s", eventsJSON(t, res.Events))
	}
	submit := strings.Replace(src, "<tui>", `<tui><keymap><bind keys="enter" action="submit"/></keymap>`, 1)
	res = formsPlay(t, submit, checkboxData, 20, 3, "enter")
	if got := eventsJSON(t, res.Events); got != `[{"step":1,"action":"submit","source":"dark","keys":{},"value":true}]` {
		t.Errorf("enter with a submit row: %s", got)
	}
	res = formsPlay(t, src, checkboxData, 20, 3, "tab", "space")
	if focusOf(res.Dump) != "beta" || res.Dump.Grid[1] != "[x] Beta            " || eventsJSON(t, res.Events) != "[]" {
		t.Errorf("tab space: focus %q, %q, %s", focusOf(res.Dump), res.Dump.Grid[1], eventsJSON(t, res.Events))
	}
	res = formsPlay(t, src, checkboxData, 20, 3, "tab", "space", "tab", "tab")
	if !btnNode(t, res.Dump, "beta").Checked {
		t.Error("tab space tab tab: beta lost its value")
	}

	// Styles: the focus look reverses the whole rect (§10.6).
	green := strings.Replace(src, "<tui>", `<tui><style>checkbox:checked { color: green; }</style>`, 1)
	res = formsStyled(t, green, checkboxData, 20, 3)
	if got := spansJSON(t, res.Dump, 0); got != `[{"x":0,"w":20,"fg":"green","bg":"default","a":["reverse"]}]` {
		t.Errorf("styles row 0: %s", got)
	}
	if got := spansJSON(t, res.Dump, 1); got != `[{"x":0,"w":20,"fg":"default","bg":"default"}]` {
		t.Errorf("styles row 1: %s", got)
	}
	res = formsStyled(t, green, checkboxData, 20, 3, "space")
	if got := spansJSON(t, res.Dump, 0); got != `[{"x":0,"w":20,"fg":"default","bg":"default","a":["reverse"]}]` {
		t.Errorf("space styles row 0: %s", got)
	}
	if got := spansJSON(t, res.Dump, 1); got != `[{"x":0,"w":20,"fg":"default","bg":"default"}]` {
		t.Errorf("space styles row 1: %s", got)
	}
	noLook := strings.Replace(green, "</style>", ` checkbox:focus { reverse: false; }</style>`, 1)
	res = formsStyled(t, noLook, checkboxData, 20, 3)
	if got := spansJSON(t, res.Dump, 0); got != `[{"x":0,"w":13,"fg":"green","bg":"default"},{"x":13,"w":7,"fg":"default","bg":"default"}]` {
		t.Errorf("no focus look, row 0: %s", got)
	}
	res = formsStyled(t, noLook, checkboxData, 20, 3, "space")
	if got := spansJSON(t, res.Dump, 0); got != `[{"x":0,"w":20,"fg":"default","bg":"default"}]` {
		t.Errorf("no focus look, space, row 0: %s", got)
	}

	// Intrinsic width: 4 + width(label).
	row := strings.Replace(strings.Replace(src, `<checkbox id="dark"`, `<row style="height: 1"><checkbox id="dark"`, 1), `label="Beta"/>`, `label="Beta"/></row>`, 1)
	d, err = btnApp(t, row, checkboxData).Dump(30, 3)
	if err != nil {
		t.Fatal(err)
	}
	wantBox(t, d, "dark", 0, 0, 13, 1, "Dark mode")
	wantBox(t, d, "beta", 13, 0, 8, 1, "Beta")
}

// §6.16.3: a checkbox is a leaf that never shrinks. In a row too narrow
// for both, the second keeps its width and the row cuts it (L009).
func TestCheckboxNeverShrinks(t *testing.T) {
	src := formsSource(t, "174/checkbox.tui")
	row := strings.Replace(strings.Replace(src, `<checkbox id="dark"`, `<row id="r" style="height: 1"><checkbox id="dark"`, 1), `label="Beta"/>`, `label="Beta"/></row>`, 1)
	app := btnApp(t, row, checkboxData)
	d, err := app.Dump(15, 3)
	if err != nil {
		t.Fatal(err)
	}
	wantBox(t, d, "dark", 0, 0, 13, 1, "Dark mode")
	wantBox(t, d, "beta", 13, 0, 8, 1, "Beta")
	if d.Grid[0] != "[x] Dark mode[ " {
		t.Errorf("grid[0] = %q", d.Grid[0])
	}
	if !hasDiag(d.Errors, "L009", "") {
		t.Errorf("no L009 at 15 columns: %v", diagLines(d.Errors))
	}
}

// 175. Checkbox values.
func TestCheckboxValues(t *testing.T) {
	src := formsSource(t, "174/checkbox.tui")
	app := btnApp(t, src, `{"settings": {"dark": "yes"}}`)
	ds := app.Validate()
	if len(ds) != 1 || ds[0].Code != "B011" || ds[0].ID != "dark" || ds[0].Severity != "warning" {
		t.Errorf(`"yes": %v`, diagLines(ds))
	}
	d, _ := app.Dump(20, 3)
	wantGrid0(t, d, "[ ] Dark mode       ")
	res := formsPlay(t, src, `{"settings": {"dark": "yes"}}`, 20, 3, "space")
	if got := eventsJSON(t, res.Events); got != `[{"step":1,"action":"dark_changed","source":"dark","keys":{},"value":true}]` || len(res.Dump.Errors) != 0 {
		t.Errorf(`"yes" space: %s %v`, got, res.Dump.Errors)
	}

	app = btnApp(t, src, `{"settings": {"dark": null}}`)
	if ds := app.Validate(); len(ds) != 0 {
		t.Errorf("null: %v", diagLines(ds))
	}
	d, _ = app.Dump(20, 3)
	wantGrid0(t, d, "[ ] Dark mode       ")

	ds = btnApp(t, src, `{}`).Validate()
	if !reflect.DeepEqual(codesOf(ds), []string{"B003"}) || ds[0].Severity != "warning" {
		t.Errorf("{}: %v", diagLines(ds))
	}
	res = formsPlay(t, src, `{}`, 20, 3, "space")
	if got := eventsJSON(t, res.Events); got != `[{"step":1,"action":"dark_changed","source":"dark","keys":{},"value":true}]` ||
		!btnNode(t, res.Dump, "dark").Checked || len(res.Dump.Errors) != 0 {
		t.Errorf("{} space: %s %v", got, res.Dump.Errors)
	}

	ds = btnApp(t, src, `{"settings": 5}`).Validate()
	if !reflect.DeepEqual(codesOf(ds), []string{"B003"}) {
		t.Errorf("settings = 5: %v", diagLines(ds))
	}
	res = formsPlay(t, src, `{"settings": 5}`, 20, 3, "space")
	if eventsJSON(t, res.Events) != "[]" || btnNode(t, res.Dump, "dark").Checked {
		t.Errorf("settings = 5, space: %s", eventsJSON(t, res.Events))
	}

	// A derived path: space changes nothing and fires nothing.
	app = btnApp(t, src, `{}`)
	if err := app.Derive("settings.dark", func(func(string) (any, bool)) (any, error) { return false, nil }); err != nil {
		t.Fatal(err)
	}
	res, err := app.Play(tuimark.PlayOptions{Cols: 20, Rows: 3, NoHandlers: true}, "space")
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := app.Get("settings.dark"); eventsJSON(t, res.Events) != "[]" || v != false {
		t.Errorf("derived: %s, settings.dark = %v", eventsJSON(t, res.Events), v)
	}
}

// 176. Checkbox structure, labels, narrow widths, disabled, and the mouse.
func TestCheckboxStructureAndMouse(t *testing.T) {
	doc := func(body string) string { return `<tui><screen>` + body + `</screen></tui>` }
	for _, c := range []struct{ body, code string }{
		{`<checkbox label="x"/>`, "V012"},
		{`<checkbox id="a"><text>x</text></checkbox>`, "V016"},
		{`<checkbox id="a">x</checkbox>`, "V016"},
		{`<checkbox id="a" on:click="go"/>`, "V002"},
		{`<list id="l" each="xs as x" key="x"><item><checkbox id="a"/></item></list>`, "V001"},
		{`<list id="l" each="xs as x" key="x"><item><checkbox/></item></list>`, "V001"},
		{`<box each="xs as x"><checkbox/></box>`, "V001"},
	} {
		if ds := btnApp(t, doc(c.body), `{"xs": []}`).Validate(); !reflect.DeepEqual(codesOf(ds), []string{c.code}) {
			t.Errorf("%s: %v, want %s alone", c.body, diagLines(ds), c.code)
		}
	}

	// Labels are templates.
	const label = `<checkbox id="n" label="Notify {who}"/>`
	for _, c := range []struct{ data, row0 string }{
		{`{"who": "ana"}`, "[ ] Notify ana      "},
		{`{}`, "[ ] Notify          "},
		{`{"who": "a\nb"}`, "[ ] Notify a b      "},
	} {
		app := btnApp(t, doc(label), c.data)
		d, err := app.Dump(20, 1)
		if err != nil {
			t.Fatal(err)
		}
		wantGrid0(t, d, c.row0)
		if c.data == `{}` {
			if ds := app.Validate(); !reflect.DeepEqual(codesOf(ds), []string{"B003"}) {
				t.Errorf("missing who: %v", diagLines(ds))
			}
			if n := btnNode(t, d, "n"); n.Text != "Notify " {
				t.Errorf("missing who: text %q", n.Text)
			}
		}
	}

	src := formsSource(t, "174/checkbox.tui")
	for _, c := range []struct{ width, want string }{{"5", "[x] …"}, {"3", "[x]"}} {
		narrow := strings.Replace(src, `<checkbox id="dark"`, `<checkbox id="dark" width="`+c.width+`"`, 1)
		d, err := btnApp(t, narrow, checkboxData).Dump(20, 3)
		if err != nil {
			t.Fatal(err)
		}
		if got := d.Grid[0][:len(c.want)]; got != c.want || strings.TrimRight(d.Grid[0], " ") != c.want {
			t.Errorf("width=%s: %q, want %q", c.width, d.Grid[0], c.want)
		}
	}

	mouse := strings.Replace(strings.Replace(src, "<tui>", `<tui mouse="true">`, 1), `focus="#dark"`, `focus="#beta"`, 1)
	disabled := strings.Replace(mouse, `<checkbox id="dark"`, `<checkbox id="dark" disabled="true"`, 1)
	res := formsPlay(t, disabled, checkboxData, 20, 3, "tab")
	if focusOf(res.Dump) != "beta" {
		t.Errorf("disabled: tab moved focus to %q", focusOf(res.Dump))
	}
	res = formsPlay(t, disabled, checkboxData, 20, 3, "click:1,0")
	if eventsJSON(t, res.Events) != "[]" || focusOf(res.Dump) != "beta" || !btnNode(t, res.Dump, "dark").Checked {
		t.Errorf("disabled click: focus %q, %s", focusOf(res.Dump), eventsJSON(t, res.Events))
	}
	res = formsPlay(t, mouse, checkboxData, 20, 3, "click:1,0")
	if focusOf(res.Dump) != "dark" || btnNode(t, res.Dump, "dark").Checked ||
		eventsJSON(t, res.Events) != `[{"step":1,"action":"dark_changed","source":"dark","keys":{},"value":false}]` {
		t.Errorf("click: focus %q, %s", focusOf(res.Dump), eventsJSON(t, res.Events))
	}
	res = formsPlay(t, mouse, checkboxData, 20, 3, "wheel-down:1,0")
	if eventsJSON(t, res.Events) != "[]" || focusOf(res.Dump) != "beta" || !btnNode(t, res.Dump, "dark").Checked {
		t.Errorf("wheel: focus %q, %s", focusOf(res.Dump), eventsJSON(t, res.Events))
	}
}

const radioData = `{"size": "m", "sizes": [{"id": "s", "label": "Small"}, {"id": "m", "label": "Medium"}, {"id": "l", "label": "Large"}]}`

// checkedKeysOf lists the keys of the checked rows of a dump.
func checkedKeysOf(d *tuimark.Dump) []string {
	out := []string{}
	for _, n := range d.Nodes {
		if n.Tag == "item" && n.Checked {
			out = append(out, n.Key)
		}
	}
	return out
}

// singleCases runs test 177's plays on src (the list of 177, or the table
// of 178), which give the same events and grids.
func singleCases(t *testing.T, src string) {
	t.Helper()
	d, err := btnApp(t, src, radioData).Dump(20, 3)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"  Small             ", "● Medium            ", "  Large             "}; !reflect.DeepEqual(d.Grid, want) {
		t.Errorf("grid %q, want %q", d.Grid, want)
	}
	if got := checkedKeysOf(d); !reflect.DeepEqual(got, []string{"m"}) {
		t.Errorf("checked rows %v", got)
	}
	for _, c := range []struct {
		steps        []string
		row0, row1   string
		events       string
		checkedAfter []string
	}{
		{[]string{"space"}, "● Small             ", "  Medium            ", `[{"step":1,"action":"size_changed","source":"size","keys":{"it":"s"},"value":"s"}]`, []string{"s"}},
		{[]string{"down", "space"}, "  Small             ", "● Medium            ", `[]`, []string{"m"}},
		{[]string{"x", "x"}, "  Small             ", "  Medium            ", `[{"step":1,"action":"size_changed","source":"size","keys":{"it":"s"},"value":null}]`, []string{}},
		{[]string{"a"}, "  Small             ", "● Medium            ", `[{"step":1,"action":"all_fallback","source":"size","keys":{"it":"s"},"value":null}]`, []string{"m"}},
	} {
		res := formsPlay(t, src, radioData, 20, 3, c.steps...)
		if got := eventsJSON(t, res.Events); got != c.events {
			t.Errorf("%v: events %s, want %s", c.steps, got, c.events)
		}
		if res.Dump.Grid[0] != c.row0 || res.Dump.Grid[1] != c.row1 {
			t.Errorf("%v: grid %q", c.steps, res.Dump.Grid)
		}
		if got := checkedKeysOf(res.Dump); !reflect.DeepEqual(got, c.checkedAfter) {
			t.Errorf("%v: checked rows %v, want %v", c.steps, got, c.checkedAfter)
		}
	}

	// Other values.
	const sizes = `"sizes": [{"id": "s", "label": "Small"}, {"id": "m", "label": "Medium"}, {"id": "l", "label": "Large"}]`
	app := btnApp(t, src, `{"size": ["m"], `+sizes+`}`)
	if ds := app.Validate(); !reflect.DeepEqual(codesOf(ds), []string{"B008"}) || ds[0].Severity != "error" {
		t.Errorf("an array: %v", diagLines(ds))
	}
	res, err := app.Play(tuimark.PlayOptions{Cols: 20, Rows: 3, NoHandlers: true}, "space")
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := app.Get("size"); eventsJSON(t, res.Events) != "[]" || len(checkedKeysOf(res.Dump)) != 0 || !reflect.DeepEqual(v, []any{"m"}) {
		t.Errorf("an array, space: %s, size %v", eventsJSON(t, res.Events), v)
	}
	app = btnApp(t, src, `{"size": null, `+sizes+`}`)
	if ds := app.Validate(); len(ds) != 0 {
		t.Errorf("null: %v", diagLines(ds))
	}
	res = formsPlay(t, src, `{"size": null, `+sizes+`}`, 20, 3, "space")
	if got := eventsJSON(t, res.Events); got != `[{"step":1,"action":"size_changed","source":"size","keys":{"it":"s"},"value":"s"}]` {
		t.Errorf("null, space: %s", got)
	}
	app = btnApp(t, src, `{`+sizes+`}`)
	if ds := app.Validate(); !reflect.DeepEqual(codesOf(ds), []string{"B003"}) {
		t.Errorf("absent: %v", diagLines(ds))
	}
	if _, err := app.Play(tuimark.PlayOptions{Cols: 20, Rows: 3, NoHandlers: true}, "space"); err != nil {
		t.Fatal(err)
	}
	if v, _ := app.Get("size"); v != "s" {
		t.Errorf("absent, space: size = %v, want \"s\"", v)
	}
}

// 177. single on a list.
func TestSingleList(t *testing.T) {
	src := formsSource(t, "177/radio.tui")
	if ds := btnApp(t, src, radioData).Validate(); len(ds) != 0 {
		t.Fatalf("Validate: %v", diagLines(ds))
	}
	singleCases(t, src)

	// space needs no keymap row (ADR 0031), and a space row never fires
	// while the single list has focus.
	pick := strings.Replace(src, "<keymap>", `<keymap><bind keys="space" action="pick"/>`, 1)
	draft := strings.Replace(src, "<keymap>", `<keymap><bind keys="space" action="check-toggle" when="#size:focus"/>`, 1)
	res := formsPlay(t, pick, radioData, 20, 3, "space")
	if got := eventsJSON(t, res.Events); got != `[{"step":1,"action":"size_changed","source":"size","keys":{"it":"s"},"value":"s"}]` {
		t.Errorf("pick row: %s", got)
	}
	// With the draft's space row added back, every result is unchanged.
	singleCases(t, draft)

	// Static checks.
	b007 := strings.Replace(src, "<keymap>", `<keymap><bind keys="b" action="check-all" when="#size:focus"/>`, 1)
	if ds := btnApp(t, b007, radioData).Validate(); !reflect.DeepEqual(codesOf(ds), []string{"B007"}) || ds[0].Severity != "error" {
		t.Errorf("check-all on a single list: %v", diagLines(ds))
	}
	// An invalid single is ignored, so the list is a multi-select and the
	// data's string is also B008 there.
	if ds := btnApp(t, strings.Replace(src, `single="true"`, `single="yes"`, 1), radioData).Validate(); !hasDiag(ds, "V003", `single="yes": want true or false`) {
		t.Errorf(`single="yes": %v`, diagLines(ds))
	}
	// Without mark and on:change too, which need checked themselves.
	noMark := strings.Replace(strings.Replace(src, `checked="size" `, ``, 1), ` mark="●"`, ``, 1)
	noMark = strings.Replace(noMark, ` on:change="size_changed"`, ``, 1)
	// The x row's check-none then names a list without checked: B007.
	if ds := btnApp(t, noMark, radioData).Validate(); !reflect.DeepEqual(codesOf(ds), []string{"B007", "V018"}) ||
		!hasDiag(ds, "V018", "single on <list> needs checked=") {
		t.Errorf("single without checked: %v", diagLines(ds))
	}

	// Numeric keys: "2" is not 2, and space writes the number 1.
	const numeric = `{"size": "2", "sizes": [{"id": 1, "label": "Small"}, {"id": 2, "label": "Medium"}, {"id": 3, "label": "Large"}]}`
	d, err := btnApp(t, src, numeric).Dump(20, 3)
	if err != nil {
		t.Fatal(err)
	}
	if got := checkedKeysOf(d); len(got) != 0 {
		t.Errorf("numeric keys: checked %v", got)
	}
	res = formsPlay(t, src, numeric, 20, 3, "space")
	if got := eventsJSON(t, res.Events); got != `[{"step":1,"action":"size_changed","source":"size","keys":{"it":1},"value":1}]` {
		t.Errorf("numeric keys, space: %s", got)
	}
}

// 178. single on a table.
func TestSingleTable(t *testing.T) {
	src := formsSource(t, "178/radio-table.tui")
	if ds := btnApp(t, src, radioData).Validate(); len(ds) != 0 {
		t.Fatalf("Validate: %v", diagLines(ds))
	}
	singleCases(t, src)
}

// A single widget that cannot act still consumes space (decision 98), and
// a check-toggle on a row whose key is an object changes nothing
// (decision 43).
func TestSingleConsumesSpace(t *testing.T) {
	src := strings.Replace(formsSource(t, "177/radio.tui"), "<keymap>", `<keymap><bind keys="space" action="pick"/>`, 1)
	const sizes = `"sizes": [{"id": "s", "label": "Small"}]`
	for _, data := range []string{`{"size": ["m"], ` + sizes + `}`, `{"size": {"a": 1}, ` + sizes + `}`, `{"size": 5, ` + sizes + `}`} {
		res := formsPlay(t, src, data, 20, 3, "space")
		if data == `{"size": 5, `+sizes+`}` {
			// 5 is a key no row has: space checks s.
			if got := eventsJSON(t, res.Events); got != `[{"step":1,"action":"size_changed","source":"size","keys":{"it":"s"},"value":"s"}]` {
				t.Errorf("%s: %s", data, got)
			}
			continue
		}
		if eventsJSON(t, res.Events) != "[]" {
			t.Errorf("%s: %s", data, eventsJSON(t, res.Events))
		}
	}
	// Unwritable: a value along the path is not an object.
	under := strings.Replace(src, `checked="size"`, `checked="sel.size"`, 1)
	res := formsPlay(t, under, `{"sel": 3, `+sizes+`}`, 20, 3, "space")
	if eventsJSON(t, res.Events) != "[]" {
		t.Errorf("unwritable: %s", eventsJSON(t, res.Events))
	}
	// Derived: unwritable too.
	app := btnApp(t, src, `{`+sizes+`}`)
	if err := app.Derive("size", func(func(string) (any, bool)) (any, error) { return nil, nil }); err != nil {
		t.Fatal(err)
	}
	res, err := app.Play(tuimark.PlayOptions{Cols: 20, Rows: 3, NoHandlers: true}, "space")
	if err != nil {
		t.Fatal(err)
	}
	if eventsJSON(t, res.Events) != "[]" {
		t.Errorf("derived: %s", eventsJSON(t, res.Events))
	}
	// An object key cannot be chosen.
	obj := strings.Replace(src, `key="it.id"`, `key="it"`, 1)
	res = formsPlay(t, obj, `{"size": null, `+sizes+`}`, 20, 3, "space")
	if eventsJSON(t, res.Events) != "[]" {
		t.Errorf("object key: %s", eventsJSON(t, res.Events))
	}
	// A multi-select keeps space not implicit.
	multi := strings.Replace(src, ` single="true"`, ``, 1)
	res = formsPlay(t, multi, `{"size": [], `+sizes+`}`, 20, 3, "space")
	if got := eventsJSON(t, res.Events); got != `[{"step":1,"action":"pick","source":"size","keys":{"it":"s"},"value":null}]` {
		t.Errorf("multi-select space: %s", got)
	}
}

// 186 (the checkbox part). B007 for a built-in naming a checkbox, and a
// keymap row's payload carries its value.
func TestCheckboxKeymap(t *testing.T) {
	src := formsSource(t, "174/checkbox.tui")
	for _, row := range []string{`<bind keys="n" action="move-next" to="#dark"/>`, `<bind keys="t" action="check-toggle" to="#dark"/>`} {
		s := strings.Replace(src, "<tui>", "<tui><keymap>"+row+"</keymap>", 1)
		if ds := btnApp(t, s, checkboxData).Validate(); !reflect.DeepEqual(codesOf(ds), []string{"B007"}) || ds[0].Severity != "error" {
			t.Errorf("%s: %v", row, diagLines(ds))
		}
	}
	peek := strings.Replace(src, "<tui>", `<tui><keymap><bind keys="p" action="peek"/></keymap>`, 1)
	res := formsPlay(t, peek, checkboxData, 20, 3, "p", "space", "p")
	want := `[{"step":1,"action":"peek","source":"dark","keys":{},"value":true},` +
		`{"step":2,"action":"dark_changed","source":"dark","keys":{},"value":false},` +
		`{"step":3,"action":"peek","source":"dark","keys":{},"value":false}]`
	if got := eventsJSON(t, res.Events); got != want {
		t.Errorf("peek: %s", got)
	}
	// A B011 value is shown unchecked, and so it is sent.
	res = formsPlay(t, peek, `{"settings": {"dark": "yes"}}`, 20, 3, "p")
	if got := eventsJSON(t, res.Events); got != `[{"step":1,"action":"peek","source":"dark","keys":{},"value":false}]` {
		t.Errorf("peek on B011: %s", got)
	}
}

// 222 (the checkbox part). One read sees the checkbox's value.
func TestFormsOneRead(t *testing.T) {
	const src = `<tui>
  <keymap>
    <bind keys="x" action="go" when="#c:checked"/>
  </keymap>
  <screen id="main" focus="#c">
    <checkbox id="c" label="C"/>
  </screen>
</tui>`
	res := formsPlay(t, src, "", 20, 2, "space", "x")
	if got := eventsJSON(t, res.Events); got != `[{"step":2,"action":"go","source":"c","keys":{},"value":true}]` {
		t.Errorf("two steps: %s", got)
	}
	res = formsPlay(t, src, "", 20, 2, "text: x")
	if got := eventsJSON(t, res.Events); got != `[{"step":1,"action":"go","source":"c","keys":{},"value":true}]` {
		t.Errorf("one read: %s", got)
	}
	res = formsPlay(t, src, "", 20, 2, "x")
	if got := eventsJSON(t, res.Events); got != "[]" {
		t.Errorf("no toggle: %s", got)
	}
}

// A checkbox without bind keeps its value across Play calls and screens,
// and Dump and Validate leave it alone (§6.16.2, §18.1).
func TestCheckboxStateKeptByID(t *testing.T) {
	const src = `<tui>
  <keymap><bind keys="s" action="switch-to" to="#two"/><bind keys="o" action="switch-to" to="#one"/></keymap>
  <screen id="one" focus="#c"><checkbox id="c" label="C"/></screen>
  <screen id="two"><text>two</text></screen>
</tui>`
	app := btnApp(t, src, "")
	opts := tuimark.PlayOptions{Cols: 10, Rows: 1, NoHandlers: true}
	if _, err := app.Play(opts, "space"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		d, err := app.Dump(10, 1)
		if err != nil {
			t.Fatal(err)
		}
		if d.Grid[0] != "[x] C     " {
			t.Errorf("dump %d: %q", i, d.Grid[0])
		}
	}
	res, err := app.Play(opts, "s", "o")
	if err != nil {
		t.Fatal(err)
	}
	if res.Dump.Grid[0] != "[x] C     " {
		t.Errorf("after a screen switch: %q", res.Dump.Grid[0])
	}
	// A host Set of a bound path changes the checkbox without on:change.
	bound := `<tui><screen focus="#c"><checkbox id="c" bind="on" on:change="ch"/></screen></tui>`
	app = btnApp(t, bound, `{"on": false}`)
	if err := app.Set("on", true); err != nil {
		t.Fatal(err)
	}
	res, err = app.Play(opts)
	if err != nil {
		t.Fatal(err)
	}
	if res.Dump.Grid[0] != "[x]       " || eventsJSON(t, res.Events) != "[]" {
		t.Errorf("Set: %q %s", res.Dump.Grid[0], eventsJSON(t, res.Events))
	}
}

// SPEC v0.4b §24 conformance tests for phase F9, forms II: the <select>
// and its option list (§6.17). Tests 179-187 and 220 close here, with
// the select halves of 173, 186, and 222. The fixtures are
// testdata/v04b/179/select.tui and select.json (§6.17.7's text),
// 181/select-up.tui, 182/modal.tui, and 220/focus.tui and focus.json;
// variants are derived from their text.

func selectFixture(t *testing.T) (src, data string) {
	t.Helper()
	return formsSource(t, "179/select.tui"), formsSource(t, "179/select.json")
}

// nodesJSON is a dump's nodes of the given tags, compact, in order.
func nodesJSON(t *testing.T, d *tuimark.Dump, tags ...string) string {
	t.Helper()
	keep := []tuimark.DumpNode{}
	for _, n := range d.Nodes {
		for _, tag := range tags {
			if n.Tag == tag {
				keep = append(keep, n)
				break
			}
		}
	}
	b, err := json.Marshal(keep)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// zonesWith is select.json's zones with Los Angeles added, and tz.
func zonesWith(tz string) string {
	return `{"tz": ` + tz + `, "zones": [{"id": "utc", "name": "UTC"}, {"id": "lis", "name": "Lisbon"}, {"id": "tok", "name": "Tokyo"}, {"id": "la", "name": "Los Angeles"}]}`
}

// The open list of select.tui at 20×6, as §6.17.7 gives it.
var selectOpenGrid = []string{
	"Lisbon             ▾",
	"┌──────────────────┐",
	"│UTC               │",
	"│Lisbon            │",
	"│Tokyo             │",
	"└──────────────────┘",
}

const selectOpenNodes = `[{"id":"main","tag":"screen","x":0,"y":0,"w":20,"h":6},` +
	`{"id":"tz","tag":"select","x":0,"y":0,"w":20,"h":1,"text":"Lisbon","focused":true},` +
	`{"id":"c","tag":"checkbox","x":0,"y":1,"w":20,"h":1,"text":"C"},` +
	`{"tag":"list","x":0,"y":1,"w":20,"h":5,"scroll":{"y":0,"h":3}},` +
	`{"tag":"item","x":1,"y":2,"w":18,"h":1,"key":"utc"},{"tag":"text","x":1,"y":2,"w":18,"h":1,"text":"UTC"},` +
	`{"tag":"item","x":1,"y":3,"w":18,"h":1,"key":"lis","selected":true,"checked":true},{"tag":"text","x":1,"y":3,"w":18,"h":1,"text":"Lisbon"},` +
	`{"tag":"item","x":1,"y":4,"w":18,"h":1,"key":"tok"},{"tag":"text","x":1,"y":4,"w":18,"h":1,"text":"Tokyo"}]`

// 173 (the select part). The version gate for <select>, <option>, and a
// select > list rule.
func TestSelectVersionGate(t *testing.T) {
	const hint = " (requires a <tui> without version)"
	doc := func(version, body string) string {
		v := ""
		if version != "" {
			v = ` version="` + version + `"`
		}
		return `<tui` + v + `>` + body + `</tui>`
	}
	const sel = `<screen><select id="b" each="xs as x" key="x.id"/></screen>`
	ds := btnApp(t, doc("3", sel), `{"xs": []}`).Validate()
	if len(ds) != 1 || ds[0].Code != "V001" || ds[0].Msg != "tag <select>"+hint {
		t.Errorf("select in version=3: %v", diagLines(ds))
	}
	// Not checked further: no V012, V016, or V017.
	ds = btnApp(t, doc("3", `<screen><select><text>x</text></select></screen>`), "").Validate()
	if !reflect.DeepEqual(codesOf(ds), []string{"V001"}) {
		t.Errorf("select without id, each, and key, with a child, in version=3: %v", diagLines(ds))
	}
	const sheet = `<style>select > list { max-height: 4; }</style>`
	ds = btnApp(t, doc("3", sheet+`<screen/>`), "").Validate()
	if len(ds) != 1 || ds[0].Code != "V003" || !strings.HasSuffix(ds[0].Msg, hint) {
		t.Errorf("select > list in version=3: %v", diagLines(ds))
	}
	for _, body := range []string{sel, sheet + `<screen/>`} {
		if ds := btnApp(t, doc("", body), `{"xs": []}`).Validate(); len(ds) != 0 {
			t.Errorf("current %s: %v", body, diagLines(ds))
		}
	}
	ds = btnApp(t, doc("", `<screen><option/></screen>`), "").Validate()
	if len(ds) != 1 || ds[0].Code != "V001" || !strings.Contains(ds[0].Msg, "use <select each=… key=… label=…>") {
		t.Errorf("option in the current language: %v", diagLines(ds))
	}
	ds = btnApp(t, doc("3", `<screen><option/></screen>`), "").Validate()
	if len(ds) != 1 || ds[0].Code != "V001" || !strings.Contains(ds[0].Msg, "a <list> with on:select picks one row") || strings.Contains(ds[0].Msg, "<select") {
		t.Errorf("option in version=3: %v", diagLines(ds))
	}
}

// 179. Select closed.
func TestSelectClosed(t *testing.T) {
	src, data := selectFixture(t)
	app := btnApp(t, src, data)
	if ds := app.Validate(); len(ds) != 0 {
		t.Fatalf("Validate: %v", diagLines(ds))
	}
	d, err := app.Dump(20, 6)
	if err != nil {
		t.Fatal(err)
	}
	blank := strings.Repeat(" ", 20)
	if want := []string{"Lisbon             ▾", "[ ] C               ", blank, blank, blank, blank}; !reflect.DeepEqual(d.Grid, want) {
		t.Errorf("grid %q", d.Grid)
	}
	if got := nodesJSON(t, d, "select", "list"); got != `[{"id":"tz","tag":"select","x":0,"y":0,"w":20,"h":1,"text":"Lisbon","focused":true}]` {
		t.Errorf("nodes %s", got)
	}

	// "tz": null: the placeholder, dim, under the focus look.
	null := strings.Replace(data, `"tz": "lis"`, `"tz": null`, 1)
	res := formsStyled(t, src, null, 20, 6)
	if res.Dump.Grid[0] != "zone               ▾" {
		t.Errorf("null: %q", res.Dump.Grid[0])
	}
	if got := spansJSON(t, res.Dump, 0); got != `[{"x":0,"w":4,"fg":"default","bg":"default","a":["dim","reverse"]},{"x":4,"w":16,"fg":"default","bg":"default","a":["reverse"]}]` {
		t.Errorf("null styles row 0: %s", got)
	}
	if n := btnNode(t, res.Dump, "tz"); n.Text != "" {
		t.Errorf("null: the tz node has text %q", n.Text)
	}
	noLook := strings.Replace(src, "<tui>", `<tui><style>select:focus { reverse: false; }</style>`, 1)
	res = formsStyled(t, noLook, null, 20, 6)
	if got := spansJSON(t, res.Dump, 0); got != `[{"x":0,"w":4,"fg":"default","bg":"default","a":["dim"]},{"x":4,"w":16,"fg":"default","bg":"default"}]` {
		t.Errorf("null, no focus look, row 0: %s", got)
	}

	// "tz": "nope": the same grid, and one B012.
	nope := strings.Replace(data, `"tz": "lis"`, `"tz": "nope"`, 1)
	app = btnApp(t, src, nope)
	ds := app.Validate()
	if len(ds) != 1 || ds[0].Code != "B012" || ds[0].Severity != "warning" || ds[0].ID != "tz" {
		t.Errorf("nope: %v", diagLines(ds))
	}
	if d, _ := app.Dump(20, 6); d.Grid[0] != "zone               ▾" {
		t.Errorf("nope: %q", d.Grid[0])
	}

	// No option: the placeholder, and enter opens nothing.
	const none = `{"tz": null, "zones": []}`
	res = formsPlay(t, src, none, 20, 6, "enter")
	if res.Dump.Grid[0] != "zone               ▾" || hasNode(res.Dump, "list") || eventsJSON(t, res.Events) != "[]" {
		t.Errorf("no option: %q list %v %s", res.Dump.Grid[0], hasNode(res.Dump, "list"), eventsJSON(t, res.Events))
	}
	// A value with no option is B012 too.
	if ds := btnApp(t, src, `{"tz": "lis", "zones": []}`).Validate(); !reflect.DeepEqual(codesOf(ds), []string{"B012"}) {
		t.Errorf("no option, a value: %v", diagLines(ds))
	}
	if ds := btnApp(t, src, `{"tz": null, "zones": 5}`).Validate(); !reflect.DeepEqual(codesOf(ds), []string{"B001"}) || ds[0].Severity != "error" {
		t.Errorf("zones 5: %v", diagLines(ds))
	}

	// Numeric keys: the number 1 chooses, the string "1" does not.
	const nums = `"zones": [{"id": 1, "name": "One"}, {"id": 2, "name": "Two"}]}`
	app = btnApp(t, src, `{"tz": 1, `+nums)
	if ds := app.Validate(); len(ds) != 0 {
		t.Errorf("tz 1: %v", diagLines(ds))
	}
	if d, _ := app.Dump(20, 6); d.Grid[0] != "One                ▾" {
		t.Errorf("tz 1: %q", d.Grid[0])
	}
	app = btnApp(t, src, `{"tz": "1", `+nums)
	if ds := app.Validate(); !reflect.DeepEqual(codesOf(ds), []string{"B012"}) {
		t.Errorf(`tz "1": %v`, diagLines(ds))
	}
	if d, _ := app.Dump(20, 6); d.Grid[0] != "zone               ▾" {
		t.Errorf(`tz "1": %q`, d.Grid[0])
	}

	// Truncation, and the intrinsic width.
	w8 := strings.Replace(src, `<select id="tz"`, `<select id="tz" width="8"`, 1)
	if d, _ := btnApp(t, w8, zonesWith(`"la"`)).Dump(20, 6); d.Grid[0] != "Los A… ▾            " {
		t.Errorf("width 8: %q", d.Grid[0])
	}
	row := strings.Replace(strings.Replace(src, `<select id="tz"`, `<row style="height: 1"><select id="tz"`, 1), `on:change="tz_changed"/>`, `on:change="tz_changed"/></row>`, 1)
	d, err = btnApp(t, row, zonesWith(`"lis"`)).Dump(20, 6)
	if err != nil {
		t.Fatal(err)
	}
	wantBox(t, d, "tz", 0, 0, 13, 1, "Lisbon")
}

// 180. Select open and pick.
func TestSelectOpenAndPick(t *testing.T) {
	src, data := selectFixture(t)
	res := formsPlay(t, src, data, 20, 6, "enter")
	nodes, _ := json.Marshal(res.Dump.Nodes)
	if !reflect.DeepEqual(res.Dump.Grid, selectOpenGrid) || string(nodes) != selectOpenNodes || focusOf(res.Dump) != "tz" || eventsJSON(t, res.Events) != "[]" {
		t.Errorf("enter: %q\n%s\nfocus %q events %s", res.Dump.Grid, nodes, focusOf(res.Dump), eventsJSON(t, res.Events))
	}
	const picked = `[{"step":3,"action":"tz_changed","source":"tz","keys":{"z":"tok"},"value":"tok"}]`
	for _, steps := range [][]string{{"enter", "down", "enter"}, {"space", "down", "space"}} {
		res = formsPlay(t, src, data, 20, 6, steps...)
		if got := eventsJSON(t, res.Events); got != picked || res.Dump.Grid[0] != "Tokyo              ▾" || res.Dump.Grid[1] != "[ ] C               " || hasNode(res.Dump, "list") {
			t.Errorf("%v: %s %q", steps, got, res.Dump.Grid)
		}
	}
	for _, steps := range [][]string{{"enter", "enter"}, {"enter", "up", "esc"}} {
		res = formsPlay(t, src, data, 20, 6, steps...)
		if eventsJSON(t, res.Events) != "[]" || res.Dump.Grid[0] != "Lisbon             ▾" || hasNode(res.Dump, "list") {
			t.Errorf("%v: %s %q", steps, eventsJSON(t, res.Events), res.Dump.Grid[0])
		}
	}
	res = formsPlay(t, src, data, 20, 6, "enter", "up")
	if got := nodesJSON(t, res.Dump, "item"); got != `[{"tag":"item","x":1,"y":2,"w":18,"h":1,"key":"utc","selected":true},`+
		`{"tag":"item","x":1,"y":3,"w":18,"h":1,"key":"lis","checked":true},{"tag":"item","x":1,"y":4,"w":18,"h":1,"key":"tok"}]` ||
		res.Dump.Grid[0] != "Lisbon             ▾" || eventsJSON(t, res.Events) != "[]" {
		t.Errorf("enter up: %s %q %s", got, res.Dump.Grid[0], eventsJSON(t, res.Events))
	}
	res = formsPlay(t, src, data, 20, 6, "enter", "q")
	if res.Quit || eventsJSON(t, res.Events) != "[]" || !hasNode(res.Dump, "list") {
		t.Errorf("enter q: quit %v %s", res.Quit, eventsJSON(t, res.Events))
	}
	res = formsPlay(t, src, data, 20, 6, "enter", "tab")
	if hasNode(res.Dump, "list") || focusOf(res.Dump) != "c" || eventsJSON(t, res.Events) != "[]" {
		t.Errorf("enter tab: focus %q %s", focusOf(res.Dump), eventsJSON(t, res.Events))
	}
	res = formsPlay(t, src, data, 20, 6, "enter", "ctrl+c")
	if got := eventsJSON(t, res.Events); got != `[{"step":2,"action":"quit","source":"tz","keys":{},"value":null}]` {
		t.Errorf("enter ctrl+c: %s", got)
	}

	// Without bind: the remembered key.
	nobind := strings.Replace(src, ` bind="tz"`, "", 1)
	d, err := btnApp(t, nobind, data).Dump(20, 6)
	if err != nil {
		t.Fatal(err)
	}
	if d.Grid[0] != "zone               ▾" {
		t.Errorf("no bind: %q", d.Grid[0])
	}
	res = formsPlay(t, nobind, data, 20, 6, "enter", "down", "enter", "enter")
	if got := eventsJSON(t, res.Events); got != `[{"step":3,"action":"tz_changed","source":"tz","keys":{"z":"lis"},"value":"lis"}]` ||
		res.Dump.Grid[0] != "Lisbon             ▾" {
		t.Errorf("no bind: %s %q", got, res.Dump.Grid[0])
	}
	if got := nodesJSON(t, res.Dump, "item"); !strings.Contains(got, `"key":"lis","selected":true,"checked":true`) {
		t.Errorf("no bind, opened again: %s", got)
	}
}

// 180, beyond its bullets: a pick into an unwritable path writes nothing
// and fires nothing, and the list still closes; a pick creates a missing
// path; the payload of a pick is a copy of the key with its JSON type.
func TestSelectPickWrites(t *testing.T) {
	src, _ := selectFixture(t)
	const zones = `"zones": [{"id": "utc", "name": "UTC"}, {"id": 7, "name": "Seven"}]`
	res := formsPlay(t, src, `{`+zones+`}`, 20, 6, "enter", "down", "enter")
	if got := eventsJSON(t, res.Events); got != `[{"step":3,"action":"tz_changed","source":"tz","keys":{"z":7},"value":7}]` || res.Dump.Grid[0] != "Seven              ▾" {
		t.Errorf("missing path: %s %q", got, res.Dump.Grid[0])
	}
	deep := strings.Replace(src, `bind="tz"`, `bind="s.tz"`, 1)
	res = formsPlay(t, deep, `{"s": 3, `+zones+`}`, 20, 6, "enter", "down", "enter")
	if eventsJSON(t, res.Events) != "[]" || hasNode(res.Dump, "list") {
		t.Errorf("unwritable: %s list %v", eventsJSON(t, res.Events), hasNode(res.Dump, "list"))
	}
	// A B012 value is written over.
	res = formsPlay(t, src, `{"tz": "nope", `+zones+`}`, 20, 6, "enter", "enter")
	if got := eventsJSON(t, res.Events); got != `[{"step":2,"action":"tz_changed","source":"tz","keys":{"z":"utc"},"value":"utc"}]` {
		t.Errorf("B012 overwritten: %s", got)
	}
}

// 181. Above and scrolling.
func TestSelectAboveAndScrolling(t *testing.T) {
	_, data := selectFixture(t)
	up := formsSource(t, "181/select-up.tui")
	res := formsPlay(t, up, data, 20, 6, "enter")
	if want := []string{"┌──────────────────┐", "│UTC               │", "│Lisbon            │", "│Tokyo             │", "└──────────────────┘", "Lisbon             ▾"}; !reflect.DeepEqual(res.Dump.Grid, want) {
		t.Errorf("above: %q", res.Dump.Grid)
	}
	if got := nodesJSON(t, res.Dump, "list"); got != `[{"tag":"list","x":0,"y":0,"w":20,"h":5,"scroll":{"y":0,"h":3}}]` {
		t.Errorf("above: %s", got)
	}
	res = formsPlay(t, up, data, 20, 4, "enter")
	if got := nodesJSON(t, res.Dump, "list", "item"); got != `[{"tag":"list","x":0,"y":0,"w":20,"h":3,"scroll":{"y":1,"h":3}},{"tag":"item","x":1,"y":1,"w":18,"h":1,"key":"lis","selected":true,"checked":true}]` ||
		res.Dump.Grid[1] != "│Lisbon            │" {
		t.Errorf("rows 4: %s %q", got, res.Dump.Grid)
	}
	res = formsPlay(t, up, data, 20, 4, "enter", "up")
	if got := nodesJSON(t, res.Dump, "list"); got != `[{"tag":"list","x":0,"y":0,"w":20,"h":3,"scroll":{"y":0,"h":3}}]` || res.Dump.Grid[1] != "│UTC               │" {
		t.Errorf("rows 4, up: %s %q", got, res.Dump.Grid)
	}

	src, _ := selectFixture(t)
	maxh := strings.Replace(src, "<tui>", `<tui><style>select > list { max-height: 4; }</style>`, 1)
	res = formsPlay(t, maxh, data, 20, 6, "enter")
	if got := nodesJSON(t, res.Dump, "item", "list"); got != `[{"tag":"list","x":0,"y":1,"w":20,"h":4,"scroll":{"y":0,"h":3}},`+
		`{"tag":"item","x":1,"y":2,"w":18,"h":1,"key":"utc"},{"tag":"item","x":1,"y":3,"w":18,"h":1,"key":"lis","selected":true,"checked":true}]` {
		t.Errorf("max-height: %s", got)
	}
	res = formsPlay(t, maxh, data, 20, 6, "enter", "end")
	if got := nodesJSON(t, res.Dump, "item", "list"); got != `[{"tag":"list","x":0,"y":1,"w":20,"h":4,"scroll":{"y":1,"h":3}},`+
		`{"tag":"item","x":1,"y":2,"w":18,"h":1,"key":"lis","checked":true},{"tag":"item","x":1,"y":3,"w":18,"h":1,"key":"tok","selected":true}]` {
		t.Errorf("max-height, end: %s", got)
	}

	wx := strings.Replace(src, `<select id="tz"`, `<select id="tz" width="5" style="margin: 0 0 0 15"`, 1)
	res = formsPlay(t, wx, zonesWith(`"lis"`), 20, 6, "enter")
	if n := btnNode(t, res.Dump, "tz"); n.X != 15 || n.Y != 0 || n.W != 5 || n.H != 1 {
		t.Errorf("width and x: tz %+v", n)
	}
	if got := nodesJSON(t, res.Dump, "list"); !strings.HasPrefix(got, `[{"tag":"list","x":7,"y":1,"w":13,"h":5,`) {
		t.Errorf("width and x: %s", got)
	}
}

// 182. Select in a modal: the list lies over the modal's border, after
// the modal in the dump; the first esc only closes the list.
func TestSelectInModal(t *testing.T) {
	_, data := selectFixture(t)
	src := formsSource(t, "182/modal.tui")
	res := formsPlay(t, src, data, 30, 10, "enter")
	if res.Dump.Grid[7] != "     └│Tokyo           │┘     " || res.Dump.Grid[8] != "      └────────────────┘      " {
		t.Errorf("grid %q", res.Dump.Grid)
	}
	tags := []string{}
	for _, n := range res.Dump.Nodes {
		tags = append(tags, n.Tag)
	}
	if got := strings.Join(tags, " "); got != "screen text modal select list item text item text item text" {
		t.Errorf("node order: %s", got)
	}
	if got := nodesJSON(t, res.Dump, "list"); !strings.HasPrefix(got, `[{"tag":"list","x":6,"y":4,"w":18,"h":5,`) {
		t.Errorf("list: %s", got)
	}
	res = formsPlay(t, src, data, 30, 10, "enter", "esc", "esc")
	if got := eventsJSON(t, res.Events); got != `[{"step":3,"action":"m_escape","source":"m","keys":{},"value":null}]` {
		t.Errorf("enter esc esc: %s", got)
	}
}

// 183. Select and the mouse.
func TestSelectMouse(t *testing.T) {
	src, data := selectFixture(t)
	mouse := strings.Replace(src, "<tui>", `<tui mouse="true">`, 1)
	res := formsPlay(t, mouse, data, 20, 6, "click:3,0")
	if !reflect.DeepEqual(res.Dump.Grid, selectOpenGrid) {
		t.Errorf("click:3,0: %q", res.Dump.Grid)
	}
	res = formsPlay(t, mouse, data, 20, 6, "click:3,0", "click:2,4")
	if got := eventsJSON(t, res.Events); got != `[{"step":2,"action":"tz_changed","source":"tz","keys":{"z":"tok"},"value":"tok"}]` || hasNode(res.Dump, "list") {
		t.Errorf("click:2,4: %s", got)
	}
	res = formsPlay(t, mouse, data, 20, 6, "click:3,0", "click:5,0")
	if hasNode(res.Dump, "list") || eventsJSON(t, res.Events) != "[]" {
		t.Errorf("click:5,0: list %v %s", hasNode(res.Dump, "list"), eventsJSON(t, res.Events))
	}
	res = formsPlay(t, mouse, data, 20, 8, "click:3,0", "click:2,7")
	if hasNode(res.Dump, "list") || eventsJSON(t, res.Events) != "[]" || focusOf(res.Dump) != "tz" {
		t.Errorf("click:2,7: list %v %s", hasNode(res.Dump, "list"), eventsJSON(t, res.Events))
	}
	res = formsPlay(t, mouse, data, 20, 6, "click:3,0", "wheel-down:3,3", "enter")
	if got := eventsJSON(t, res.Events); got != `[{"step":3,"action":"tz_changed","source":"tz","keys":{"z":"tok"},"value":"tok"}]` {
		t.Errorf("wheel: %s", got)
	}
	button := strings.Replace(mouse, `<checkbox id="c" label="C"/>`, `<checkbox id="c" label="C"/><button id="b" label="B" on:click="b_clicked"/>`, 1)
	res = formsPlay(t, button, data, 20, 6, "click:3,0", "click:2,2")
	if got := eventsJSON(t, res.Events); got != `[{"step":2,"action":"tz_changed","source":"tz","keys":{"z":"utc"},"value":"utc"}]` {
		t.Errorf("no click-through: %s", got)
	}
	res = formsPlay(t, button, data, 20, 6, "click:3,0", "click:2,2", "click:2,2")
	if got := eventsJSON(t, res.Events); got != `[{"step":2,"action":"tz_changed","source":"tz","keys":{"z":"utc"},"value":"utc"},{"step":3,"action":"b_clicked","source":"b","keys":{},"value":null}]` {
		t.Errorf("then the button: %s", got)
	}
	// --cells: the open list's cells belong to the select.
	res, err := btnApp(t, mouse, data).Play(tuimark.PlayOptions{Cols: 20, Rows: 6, NoHandlers: true, Cells: true}, "click:3,0")
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, c := range res.Dump.Cells {
		if c.Y >= 1 && c.Y <= 5 {
			n++
			if c.ID != "tz" {
				t.Errorf("--cells: (%d,%d) belongs to %q", c.X, c.Y, c.ID)
				break
			}
		}
	}
	if n != 100 {
		t.Errorf("--cells: %d cells under the list", n)
	}
}

// 184. Closing by itself.
func TestSelectClosesByItself(t *testing.T) {
	src, data := selectFixture(t)
	res := formsPlay(t, src, data, 20, 6, "enter", "focus:#c")
	if hasNode(res.Dump, "list") || focusOf(res.Dump) != "c" {
		t.Errorf("focus:#c: list %v focus %q", hasNode(res.Dump, "list"), focusOf(res.Dump))
	}
	res = formsPlay(t, src, data, 20, 6, "enter", "set:zones=[]", "enter")
	if hasNode(res.Dump, "list") || eventsJSON(t, res.Events) != "[]" {
		t.Errorf("set:zones=[]: list %v %s", hasNode(res.Dump, "list"), eventsJSON(t, res.Events))
	}
	show := strings.Replace(src, `<select id="tz"`, `<select id="tz" if="show"`, 1)
	showData := strings.Replace(data, "{", `{"show": true, `, 1)
	res, err := btnApp(t, show, showData).Play(tuimark.PlayOptions{Cols: 20, Rows: 6, NoHandlers: true, Frames: true}, "enter", "set:show=false", "set:show=true")
	if err != nil {
		t.Fatal(err)
	}
	// Frames[i] is step i's frame, step 0 first.
	if len(res.Frames) != 4 || res.Frames[2].Step != 2 {
		t.Fatalf("%d frames", len(res.Frames))
	}
	if f := res.Frames[1].Dump; !hasNode(f, "list") {
		t.Error("step 1: no list")
	}
	if f := res.Frames[2].Dump; hasNode(f, "list") || hasNode(f, "select") {
		t.Errorf("step 2: list %v select %v", hasNode(f, "list"), hasNode(f, "select"))
	}
	if f := res.Frames[3].Dump; hasNode(f, "list") || !hasNode(f, "select") || eventsJSON(t, res.Events) != "[]" {
		t.Errorf("step 3: list %v select %v %s", hasNode(f, "list"), hasNode(f, "select"), eventsJSON(t, res.Events))
	}
	res = formsPlay(t, src, data, 20, 6, "enter", "resize:30x8")
	if got := nodesJSON(t, res.Dump, "list"); !strings.HasPrefix(got, `[{"tag":"list","x":0,"y":1,"w":30,"h":5,`) {
		t.Errorf("resize: %s", got)
	}
	res = formsPlay(t, src, data, 20, 6, "enter", `set:tz="tok"`)
	if got := nodesJSON(t, res.Dump, "item"); res.Dump.Grid[0] != "Tokyo              ▾" ||
		got != `[{"tag":"item","x":1,"y":2,"w":18,"h":1,"key":"utc"},{"tag":"item","x":1,"y":3,"w":18,"h":1,"key":"lis","selected":true},{"tag":"item","x":1,"y":4,"w":18,"h":1,"key":"tok","checked":true}]` {
		t.Errorf("set:tz: %q %s", res.Dump.Grid[0], got)
	}
	// A disabled select closes its list.
	dis := strings.Replace(src, `<select id="tz"`, `<select id="tz" disabled="off"`, 1)
	res = formsPlay(t, dis, strings.Replace(data, "{", `{"off": false, `, 1), 20, 6, "enter", "set:off=true")
	if hasNode(res.Dump, "list") {
		t.Error("disabled: the list is still open")
	}
	// The visible rect becomes empty (decision 66): a host write moves the
	// select out of its scroll's view.
	scroll := `<tui><screen id="main" focus="#tz"><scroll id="sc" height="2"><text>{top}</text>` +
		`<select id="tz" bind="tz" each="zones as z" key="z.id" label="{z.name}"/></scroll></screen></tui>`
	scrollData := strings.Replace(data, "{", `{"top": "a", `, 1)
	if res = formsPlay(t, scroll, scrollData, 20, 6, "enter"); !hasNode(res.Dump, "list") {
		t.Errorf("in a scroll: the list did not open: %q", res.Dump.Grid)
	}
	res = formsPlay(t, scroll, scrollData, 20, 6, "enter", `set:top="a\nb\nc"`)
	if hasNode(res.Dump, "list") || eventsJSON(t, res.Events) != "[]" {
		t.Errorf("empty rect: the list is still open: %q %s", res.Dump.Grid, eventsJSON(t, res.Events))
	}
}

// 185. Select structure and diagnostics.
func TestSelectStructure(t *testing.T) {
	doc := func(body string) string { return `<tui><screen>` + body + `</screen></tui>` }
	for _, c := range []struct {
		body  string
		codes []string
	}{
		{`<select each="xs as x" key="x.id"/>`, []string{"V012"}},
		{`<select id="s" each="xs as x"/>`, []string{"V017"}},
		{`<select id="s" key="x.id"/>`, []string{"V017"}},
		{`<select id="s" each="xs as x" key="x.id"><text/></select>`, []string{"V016"}},
		{`<list id="l" each="xs as x" key="x"><item><select id="s" each="xs as y" key="y"/></item></list>`, []string{"V001"}},
		{`<select id="s" each="xs" key="x.id"/>`, []string{"V011"}},
		{`<select id="s" each="xs as x" key="x.id" on:click="go"/>`, []string{"V002"}},
	} {
		ds := btnApp(t, doc(c.body), `{"xs": []}`).Validate()
		if !reflect.DeepEqual(codesOf(ds), c.codes) {
			t.Errorf("%s: %v", c.body, diagLines(ds))
		}
	}
	src, data := selectFixture(t)
	labels := strings.Replace(src, `label="{z.name}"`, `label="{z.name} {z.nope}"`, 1)
	ds := btnApp(t, labels, data).Validate()
	if !reflect.DeepEqual(codesOf(ds), []string{"B003"}) || !strings.Contains(ds[0].Msg, `"z.nope"`) {
		t.Errorf("labels: %v", diagLines(ds))
	}
	keyless := strings.Replace(data, `{"id": "tok", "name": "Tokyo"}`, `{"name": "Tokyo"}`, 1)
	app := btnApp(t, src, strings.Replace(keyless, `"tz": "lis"`, `"tz": 2`, 1))
	ds = app.Validate()
	if !reflect.DeepEqual(codesOf(ds), []string{"B003"}) || !strings.Contains(ds[0].Msg, "option 2") {
		t.Errorf("a missing key: %v", diagLines(ds))
	}
	if d, _ := app.Dump(20, 6); d.Grid[0] != "Tokyo              ▾" {
		t.Errorf("a missing key is the index: %q", d.Grid[0])
	}
	// Without label an option shows its key.
	bare := strings.Replace(src, ` label="{z.name}"`, "", 1)
	if d, _ := btnApp(t, bare, data).Dump(20, 6); d.Grid[0] != "lis                ▾" {
		t.Errorf("no label: %q", d.Grid[0])
	}
}

// 186 (the select part). B007 for a built-in naming a select, and a
// keymap row's payload carries its chosen key.
func TestSelectKeymap(t *testing.T) {
	src, data := selectFixture(t)
	for _, row := range []string{`<bind keys="n" action="move-next" to="#tz"/>`, `<bind keys="t" action="check-toggle" to="#tz"/>`} {
		s := strings.Replace(src, "<keymap>", "<keymap>"+row, 1)
		if ds := btnApp(t, s, data).Validate(); !reflect.DeepEqual(codesOf(ds), []string{"B007"}) || ds[0].Severity != "error" {
			t.Errorf("%s: %v", row, diagLines(ds))
		}
	}
	peek := strings.Replace(src, "<keymap>", `<keymap><bind keys="p" action="peek"/>`, 1)
	res := formsPlay(t, peek, data, 20, 6, "p")
	if got := eventsJSON(t, res.Events); got != `[{"step":1,"action":"peek","source":"tz","keys":{},"value":"lis"}]` {
		t.Errorf("peek: %s", got)
	}
	for _, d := range []string{strings.Replace(data, `"tz": "lis"`, `"tz": null`, 1), strings.Replace(data, `"tz": "lis"`, `"tz": "nope"`, 1)} {
		res = formsPlay(t, peek, d, 20, 6, "p")
		if got := eventsJSON(t, res.Events); got != `[{"step":1,"action":"peek","source":"tz","keys":{},"value":null}]` {
			t.Errorf("peek with nothing chosen: %s", got)
		}
	}
	// A closed select leaves up and down to the keymap (decision 62).
	arrows := strings.Replace(src, "<keymap>", `<keymap><bind keys="down" action="dn"/>`, 1)
	res = formsPlay(t, arrows, data, 20, 6, "down")
	if got := eventsJSON(t, res.Events); got != `[{"step":1,"action":"dn","source":"tz","keys":{},"value":"lis"}]` {
		t.Errorf("down while closed: %s", got)
	}
}

// 187. hints while open.
func TestSelectHints(t *testing.T) {
	src, data := selectFixture(t)
	h := strings.Replace(src, `<bind keys="q" action="quit"/>`, `<bind keys="q" action="quit" label="quit"/><bind keys="tab" action="next" label="next"/>`, 1)
	h = strings.Replace(h, `<checkbox id="c" label="C"/>`, `<checkbox id="c" label="C"/><hints id="h"/>`, 1)
	hintTexts := func(d *tuimark.Dump) string {
		var out []string
		for _, n := range d.Nodes {
			if n.Tag == "text" && len(n.Classes) > 0 && (n.Classes[0] == "hint-key" || n.Classes[0] == "hint-label") {
				out = append(out, n.Text)
			}
		}
		return strings.Join(out, " ")
	}
	res := formsPlay(t, h, data, 20, 6)
	if got := hintTexts(res.Dump); got != "q quit tab next" {
		t.Errorf("before enter: %q", got)
	}
	res = formsPlay(t, h, data, 20, 6, "enter")
	if got := hintTexts(res.Dump); got != "tab next" {
		t.Errorf("after enter: %q", got)
	}
	res = formsPlay(t, h, data, 20, 6, "enter", "tab")
	if got := eventsJSON(t, res.Events); got != `[{"step":2,"action":"next","source":"tz","keys":{},"value":"lis"}]` || hasNode(res.Dump, "list") {
		t.Errorf("enter tab: %s list %v", got, hasNode(res.Dump, "list"))
	}
}

// 220. The focus look.
func TestFocusLook(t *testing.T) {
	src, data := formsSource(t, "220/focus.tui"), formsSource(t, "220/focus.json")
	if ds := btnApp(t, src, data).Validate(); len(ds) != 0 {
		t.Errorf("validate: %v", diagLines(ds))
	}
	const (
		plain = `{"x":0,"w":20,"fg":"default","bg":"default"}`
		rev   = `{"x":0,"w":20,"fg":"default","bg":"default","a":["reverse"]}`
		ph    = `{"x":0,"w":4,"fg":"default","bg":"default","a":["dim"]},{"x":4,"w":16,"fg":"default","bg":"default"}`
	)
	rows := func(d *tuimark.Dump) []string {
		out := []string{}
		for i := range d.Styles {
			out = append(out, strings.TrimSuffix(strings.TrimPrefix(spansJSON(t, d, i), "["), "]"))
		}
		return out
	}
	res := formsStyled(t, src, data, 20, 4)
	if want := []string{"[ Save ]            ", "[ ] News            ", "size               ▾", "name                "}; !reflect.DeepEqual(res.Dump.Grid, want) {
		t.Errorf("grid %q", res.Dump.Grid)
	}
	if got := rows(res.Dump); !reflect.DeepEqual(got, []string{rev, plain, ph, ph}) {
		t.Errorf("dump styles %q", got)
	}
	res = formsStyled(t, src, data, 20, 4, "tab")
	if got := rows(res.Dump); focusOf(res.Dump) != "c" || got[0] != plain || got[1] != rev {
		t.Errorf("tab: focus %q %q", focusOf(res.Dump), got)
	}
	res = formsStyled(t, src, data, 20, 4, "tab", "tab")
	if got := rows(res.Dump); focusOf(res.Dump) != "s" || got[0] != plain || got[1] != plain ||
		got[2] != `{"x":0,"w":4,"fg":"default","bg":"default","a":["dim","reverse"]},{"x":4,"w":16,"fg":"default","bg":"default","a":["reverse"]}` {
		t.Errorf("tab tab: focus %q %q", focusOf(res.Dump), got)
	}
	res = formsStyled(t, src, data, 20, 4, "tab", "tab", "tab")
	if got := rows(res.Dump); focusOf(res.Dump) != "i" || !reflect.DeepEqual(got, []string{plain, plain, ph, ph}) {
		t.Errorf("tab tab tab: focus %q %q", focusOf(res.Dump), got)
	}
	author := strings.Replace(strings.Replace(src, "<tui>", `<tui><style>.k-btn:focus { reverse: false; bold: true; }</style>`, 1), `<button id="b"`, `<button id="b" class="k-btn"`, 1)
	res = formsStyled(t, author, data, 20, 4)
	if got := rows(res.Dump)[0]; got != `{"x":0,"w":8,"fg":"default","bg":"default","a":["bold"]},{"x":8,"w":12,"fg":"default","bg":"default"}` {
		t.Errorf("author rule: %s", got)
	}
	redundant := strings.Replace(src, "<tui>", `<tui><style>button:focus { reverse: true; }</style>`, 1)
	res = formsStyled(t, redundant, data, 20, 4)
	if got := rows(res.Dump); !reflect.DeepEqual(got, []string{rev, plain, ph, ph}) {
		t.Errorf("redundant rule: %q", got)
	}
	pinned := strings.Replace(src, "<tui>", `<tui version="3">`, 1)
	pinned = strings.Replace(pinned, "    <checkbox id=\"c\" label=\"News\"/>\n", "", 1)
	pinned = strings.Replace(pinned, "    <select id=\"s\" bind=\"size\" each=\"sizes as o\" key=\"o.id\" label=\"{o.label}\" placeholder=\"size\"/>\n", "", 1)
	res = formsStyled(t, pinned, data, 20, 4)
	if got := rows(res.Dump); got[0] != plain || got[1] != ph {
		t.Errorf("pinned: %q", got)
	}
}

// 222 (the select part). One read covers the select: enter, down, enter
// in one text step pick tok at step 1.
func TestSelectOneRead(t *testing.T) {
	src, data := selectFixture(t)
	res := formsPlay(t, src, data, 20, 6, "text:\r\x1b[B\r")
	if got := eventsJSON(t, res.Events); got != `[{"step":1,"action":"tz_changed","source":"tz","keys":{"z":"tok"},"value":"tok"}]` || res.Dump.Grid[0] != "Tokyo              ▾" {
		t.Errorf("one read: %s %q", got, res.Dump.Grid[0])
	}
}

// §6.17.3: Dump on a live app shows an open list and changes nothing; a
// select without bind keeps its key across Play calls; Validate never
// opens the list.
func TestSelectLiveDump(t *testing.T) {
	src, data := selectFixture(t)
	app := btnApp(t, src, data)
	opts := tuimark.PlayOptions{Cols: 20, Rows: 6, NoHandlers: true}
	if _, err := app.Play(opts, "enter"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		d, err := app.Dump(20, 6)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(d.Grid, selectOpenGrid) {
			t.Errorf("dump %d: %q", i, d.Grid)
		}
	}
	if ds := app.Validate(); len(ds) != 0 {
		t.Errorf("validate: %v", diagLines(ds))
	}
	res, err := app.Play(opts, "down", "enter")
	if err != nil {
		t.Fatal(err)
	}
	if got := eventsJSON(t, res.Events); got != `[{"step":2,"action":"tz_changed","source":"tz","keys":{"z":"tok"},"value":"tok"}]` {
		t.Errorf("after the dumps: %s", got)
	}

	// A Dump whose frame would close the list (the select is below the
	// grid there) closes it in that dump only: the live list stays open,
	// with its cursor.
	low := strings.Replace(src, `<select id="tz"`, `<text height="5">top</text><select id="tz"`, 1)
	app = btnApp(t, low, data)
	opts = tuimark.PlayOptions{Cols: 20, Rows: 8, NoHandlers: true}
	if res, err = app.Play(opts, "enter", "up"); err != nil {
		t.Fatal(err)
	}
	if !hasNode(res.Dump, "list") {
		t.Fatalf("low: the list did not open: %q", res.Dump.Grid)
	}
	d, err := app.Dump(20, 3)
	if err != nil {
		t.Fatal(err)
	}
	if hasNode(d, "list") {
		t.Errorf("low, at 20x3: a list: %q", d.Grid)
	}
	if res, err = app.Play(opts); err != nil {
		t.Fatal(err)
	}
	if got := nodesJSON(t, res.Dump, "item"); !strings.Contains(got, `"key":"utc","selected":true`) {
		t.Errorf("low, after the dump: %s %q", got, res.Dump.Grid)
	}
}
