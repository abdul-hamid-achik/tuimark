package tuimark_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
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
