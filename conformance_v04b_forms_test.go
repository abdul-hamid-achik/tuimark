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
// select halves of 173, 186, and 222 close with F9. The fixtures are
// testdata/v04b/174/checkbox.tui (test 174's text), 177/radio.tui, and
// 178/radio-table.tui; variants are derived from their text.

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
// close with F9).
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
	// version="1": the table's own V001 (and its column's), and no V002
	// for single.
	ds = btnApp(t, doc("1", table), "").Validate()
	if !reflect.DeepEqual(codesOf(ds), []string{"V001", "V001"}) || ds[0].Msg != `tag <table> (requires version="2")` {
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

// 174's text form: the dark line ends with " focus checked".
func TestCheckboxTextDump(t *testing.T) {
	d, err := btnApp(t, formsSource(t, "174/checkbox.tui"), checkboxData).Dump(20, 3)
	if err != nil {
		t.Fatal(err)
	}
	if n := btnNode(t, d, "dark"); !n.Focused || !n.Checked {
		t.Errorf("dark: %+v", n)
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
	for name, s := range map[string]string{"pick": pick, "draft": draft} {
		res := formsPlay(t, s, radioData, 20, 3, "space")
		if got := eventsJSON(t, res.Events); got != `[{"step":1,"action":"size_changed","source":"size","keys":{"it":"s"},"value":"s"}]` {
			t.Errorf("%s row: %s", name, got)
		}
		singleCases(t, draft)
	}

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
	res := formsPlay(t, src, numeric, 20, 3, "space")
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
	multi := strings.Replace(strings.Replace(src, ` single="true"`, ``, 1), `"size"`, `"size"`, 1)
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
