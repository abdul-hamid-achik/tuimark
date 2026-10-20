package tuimark_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/abdul-hamid-achik/tuimark"
)

// Regression tests for the independent review of phase F1 (round 3):
// V004 for a nested repeat, the placement suffix on three more
// diagnostics, and F10's spinner attribute on a placed element.

// V004 stands at the include that placed the second occurrence and not the
// first (SPEC v0.4b §6.18.4): with the repeated element two includes deep,
// the innermost include is the same written one every time, so the outer
// one tells the copies apart. The flat case (the fragment included three
// times directly) gives the same two diagnostics, one per extra copy.
func TestDuplicateIDOfANestedRepeat(t *testing.T) {
	got := diagTexts(mustLoad(t, "testdata/v04b/r3nested/app.tui").Validate())
	const first = `(first defined at row.tui:2:3, placed by <include> at outer.tui:2:3)`
	want := []string{
		`V004 app.tui:4:5 /screen/include id=r duplicate id "r" ` + first,
		`V004 app.tui:5:5 /screen/include id=r duplicate id "r" ` + first,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("nested:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	got = diagTexts(mustLoad(t, "testdata/v04b/r3flat/app.tui").Validate())
	const flat = `(first defined at row.tui:2:3, placed by <include> at app.tui:3:5)`
	want = []string{
		`V004 app.tui:4:5 /screen/include id=r duplicate id "r" ` + flat,
		`V004 app.tui:5:5 /screen/include id=r duplicate id "r" ` + flat,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("flat:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// An over-cap file is refused as an include's V006, with the reason, under
// Load and LoadFS alike, and nothing is inserted.
func TestIncludeOverTheReadCap(t *testing.T) {
	dir := t.TempDir()
	write := func(name, text string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("app.tui", appWith(`<include src="big.tui"/>`))
	f, err := os.Create(filepath.Join(dir, "big.tui"))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(64<<20 + 1); err != nil { // a sparse file of one byte over the cap
		t.Fatal(err)
	}
	f.Close()
	const want = `V006 app.tui:3:5 /screen/include id= include src="big.tui" cannot be read: file too large (over 67108864 bytes)`
	byLoad := mustLoad(t, filepath.Join(dir, "app.tui"))
	byFS, err := tuimark.LoadFS(os.DirFS(dir), "app.tui")
	if err != nil {
		t.Fatal(err)
	}
	for name, app := range map[string]*tuimark.App{"Load": byLoad, "LoadFS": byFS} {
		if got := diagTexts(app.Validate()); !reflect.DeepEqual(got, []string{want}) {
			t.Errorf("%s: %v", name, got)
		}
		screenIsEmpty(t, app)
	}
}

// A <style src> written in a fragment: its V006 names the include that
// placed it (SPEC v0.4b §6.18.6).
func TestStyleErrorInAFragmentNamesTheInclude(t *testing.T) {
	mem := fstest.MapFS{
		"app.tui":        {Data: []byte("<tui>\n  <screen id=\"s\"/>\n  <include src=\"parts/look.tui\"/>\n</tui>\n")},
		"parts/look.tui": {Data: []byte("<fragment>\n\n  <style src=\"missing.tcss\"/>\n</fragment>\n")},
	}
	app, err := tuimark.LoadFS(mem, "app.tui")
	if err != nil {
		t.Fatal(err)
	}
	got := diagTexts(app.Validate())
	if len(got) != 1 || !strings.HasPrefix(got[0], `V006 parts/look.tui:3:3 /style id= style src="missing.tcss" cannot be read: `) ||
		!strings.HasSuffix(got[0], `, placed by <include> at app.tui:3:3`) {
		t.Errorf("diagnostics: %v", got)
	}
	// A <style> of the document itself has no suffix.
	own := fstest.MapFS{"app.tui": {Data: []byte("<tui>\n  <screen id=\"s\"/>\n  <style src=\"missing.tcss\"/>\n</tui>\n")}}
	app, err = tuimark.LoadFS(own, "app.tui")
	if err != nil {
		t.Fatal(err)
	}
	if got := diagTexts(app.Validate()); len(got) != 1 || strings.Contains(got[0], "placed by") {
		t.Errorf("own style: %v", got)
	}
}

// V016 about an element's children, raised while its content is read, is a
// check that depends on where the element stands: it carries the suffix.
func TestV016InAFragmentNamesTheInclude(t *testing.T) {
	mem := fstest.MapFS{
		"app.tui": {Data: []byte(appWith(`<include src="s.tui"/>`))},
		"s.tui":   {Data: []byte("<fragment>\n  <sparkline bind=\"h\"><text>a</text></sparkline>\n</fragment>\n")},
	}
	app, err := tuimark.LoadFS(mem, "app.tui")
	if err != nil {
		t.Fatal(err)
	}
	got := diagTexts(app.Validate())
	want := `V016 s.tui:2:23 /screen/sparkline id= <sparkline> takes no children (found <text>), placed by <include> at app.tui:3:5`
	found := false
	for _, g := range got {
		found = found || g == want
	}
	if !found {
		t.Errorf("diagnostics:\n%s\nwant\n%s", strings.Join(got, "\n"), want)
	}
}

// F10: the spinner attribute on an element an include placed. Its checks
// are self-checks (V002, V003): no suffix, whatever the file.
func TestIndeterminateInAFragmentHasNoSuffix(t *testing.T) {
	mem := fstest.MapFS{
		"app.tui": {Data: []byte(appWith(`<include src="p.tui"/>`))},
		"p.tui":   {Data: []byte("<fragment>\n  <progress indeterminate=\"maybe\"/>\n  <text indeterminate=\"true\">x</text>\n</fragment>\n")},
	}
	app, err := tuimark.LoadFS(mem, "app.tui")
	if err != nil {
		t.Fatal(err)
	}
	got := diagTexts(app.Validate())
	if len(got) != 2 || !strings.HasPrefix(got[0], "V003 p.tui:2:") || !strings.HasPrefix(got[1], "V002 p.tui:3:") {
		t.Fatalf("diagnostics: %v", got)
	}
	for _, g := range got {
		if strings.Contains(g, "placed by") {
			t.Errorf("a self-check has the suffix: %s", g)
		}
	}
}
