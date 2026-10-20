package tuimark_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

// SPEC v0.4b §24 conformance tests of composition, phase F2: <template>
// and <use> (tests 133 and 138), the parts the Go API observes. The CLI
// parts are cmd/tuimark's conformance_v04b_templates_test.go; the fixtures
// are testdata/v04b/t133 and t138.

// loadWith loads a fixture and binds its JSON data at the root.
func loadWith(t *testing.T, doc, data string) *tuimark.App {
	t.Helper()
	app := mustLoad(t, doc)
	raw, err := os.ReadFile(data)
	if err != nil {
		t.Fatal(err)
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	if err := app.Bind("", v); err != nil {
		t.Fatal(err)
	}
	return app
}

// 133. Each copy's input writes the path its alias stands for, through
// two levels of with; LoadFS gives the same dump and diagnostics as Load.
func TestTemplateExpansionWritesThroughAliases(t *testing.T) {
	dir := filepath.Join("testdata", "v04b", "t133")
	app := loadWith(t, filepath.Join(dir, "app.tui"), filepath.Join(dir, "d.json"))
	res, err := app.Play(tuimark.PlayOptions{}, "text:9", "tab", "text:8")
	if err != nil {
		t.Fatal(err)
	}
	if f := res.Dump.Focus; f == nil || *f != "shipping__line__in" || len(res.Events) != 0 {
		t.Errorf("focus %v, events %v", f, res.Events)
	}
	for path, want := range map[string]any{"order.billing.street": "9", "order.shipping.street": "8"} {
		if got, _ := app.Get(path); got != want {
			t.Errorf("Get(%q) = %v, want %q", path, got, want)
		}
	}

	viaFS, err := tuimark.LoadFS(os.DirFS(dir), "app.tui")
	if err != nil {
		t.Fatal(err)
	}
	direct := mustLoad(t, filepath.Join(dir, "app.tui"))
	a, err := direct.Dump(80, 24)
	if err != nil {
		t.Fatal(err)
	}
	b, err := viaFS.Dump(80, 24)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Errorf("LoadFS dump differs from Load's")
	}
	if va, vb := diagTexts(direct.Validate()), diagTexts(viaFS.Validate()); !reflect.DeepEqual(va, vb) {
		t.Errorf("Validate: Load %v, LoadFS %v", va, vb)
	}
}

// 138. Two copies of one template write their own paths, and their events
// name the scoped ids.
func TestTemplateCopiesKeepTheirOwnState(t *testing.T) {
	dir := filepath.Join("testdata", "v04b", "t138")
	app := loadWith(t, filepath.Join(dir, "app.tui"), filepath.Join(dir, "d.json"))
	res, err := app.Play(tuimark.PlayOptions{}, "text:ab", "enter", "tab", "text:c", "enter", "ctrl+s")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range res.Events {
		got = append(got, e.Action+"@"+e.Source)
	}
	if want := []string{"save@bill__in", "save@ship__in", "save_all@ship__in"}; !reflect.DeepEqual(got, want) {
		t.Errorf("events %v, want %v", got, want)
	}
	for path, want := range map[string]any{"form.bill.value": "ab", "form.ship.value": "c"} {
		if got, _ := app.Get(path); got != want {
			t.Errorf("Get(%q) = %v, want %q", path, got, want)
		}
	}
}

// parseApp parses src and binds data at the root.
func parseApp(t *testing.T, src string, data map[string]any) *tuimark.App {
	t.Helper()
	app, err := tuimark.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Bind("", data); err != nil {
		t.Fatal(err)
	}
	return app
}

// 141, the use-site aliases bullet (decision 12): a copy reads an alias in
// scope where its use stands, a list row's included, which it was not
// given.
func TestTemplateCopyReadsUseSiteAliases(t *testing.T) {
	app := parseApp(t, `<tui><template id="t"><text>{r.name}</text></template><screen id="main"><list id="l" each="rows as r" key="r.id"><item><use template="t"/></item></list></screen></tui>`,
		map[string]any{"rows": []any{map[string]any{"id": "a", "name": "A"}, map[string]any{"id": "b", "name": "B"}}})
	if ds := app.Validate(); len(ds) != 0 {
		t.Errorf("Validate: %v", diagTexts(ds))
	}
	d, err := app.Dump(20, 4)
	if err != nil {
		t.Fatal(err)
	}
	var texts []string
	for _, n := range d.Nodes {
		if n.Tag == "text" {
			texts = append(texts, n.Text)
		}
	}
	if !reflect.DeepEqual(texts, []string{"A", "B"}) {
		t.Errorf("texts %v, want A B", texts)
	}
}

// 144. A use's if and hidden decide whether its copy is in the frame,
// evaluated where the use stands (its own with is not in scope); a
// missing path is one B002 at the use.
func TestTemplateUseIf(t *testing.T) {
	const doc = "<tui>\n  <template id=\"banner\">\n    <text id=\"msg\">{b.text}</text>\n  </template>\n  <screen id=\"main\">\n    <use id=\"top\" template=\"banner\" with=\"note as b\" IF/>\n    <text>after</text>\n  </screen>\n</tui>"
	data := map[string]any{"show": false, "note": map[string]any{"text": "hi"}}
	app := parseApp(t, strings.Replace(doc, "IF", `if="show"`, 1), data)
	d, err := app.Dump(10, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Nodes) != 2 || d.Nodes[1].Text != "after" || d.Nodes[1].Y != 0 {
		t.Errorf("hidden: %+v", d.Nodes)
	}
	res, err := app.Play(tuimark.PlayOptions{Cols: 10, Rows: 3}, "set:show=true")
	if err != nil {
		t.Fatal(err)
	}
	if n := res.Dump.Nodes; len(n) != 3 || n[1].ID != "top__msg" || n[1].Text != "hi" || n[1].Y != 0 || n[2].Text != "after" || n[2].Y != 1 {
		t.Errorf("shown: %+v", n)
	}
	app = parseApp(t, strings.Replace(doc, "IF", `if="b.text"`, 1), data)
	ds := app.Validate()
	if len(ds) != 1 || ds[0].Code != "B002" || ds[0].Line != 6 || ds[0].Col != 5 {
		t.Errorf("if=b.text: %v", diagTexts(ds))
	}
	if d, _ := app.Dump(10, 3); len(d.Nodes) != 2 {
		t.Errorf("if=b.text: the copy is in the frame: %+v", d.Nodes)
	}
	app = parseApp(t, strings.Replace(doc, "IF", `hidden="!show"`, 1), data)
	if d, _ := app.Dump(10, 3); len(d.Nodes) != 2 {
		t.Errorf("hidden=!show: the copy is in the frame: %+v", d.Nodes)
	}
}
