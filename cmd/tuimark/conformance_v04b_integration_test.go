package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/catalog"
)

// SPEC v0.4b §24 conformance tests closed by phase F12 (integration), the
// parts that go through the CLI: 130 (the number 4 in the catalog; its ir
// bullet is TestV04bIRVersion), 132 with 172's last bullet (tuimark new
// under the current language, test 120 rerun), and the CLI bullets of 215,
// 223, and 225 (validate --json and its exit code, inspect --json). The Go
// API parts of 215 and 223-225 are the root package's
// conformance_v04b_integration_test.go. The fixtures are
// testdata/v04b/t215, t223, and t225.

// 130. The catalog numbers the current language 4: versions stay [1, 2,
// 3], current is 4, every 0.4b tag of §5.1's table has since 4, as do
// single on list and indeterminate on progress, and every since is one of
// versions or current.
func TestV04bIntegration130TheNumber4(t *testing.T) {
	code, out, errw := runCLI("catalog")
	if code != 0 {
		t.Fatalf("catalog: exit %d %s", code, errw)
	}
	var c catalog.Catalog
	if err := json.Unmarshal([]byte(out), &c); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c.Versions, []int{1, 2, 3}) || c.Current != 4 {
		t.Errorf("versions %v, current %d; want [1 2 3] and 4", c.Versions, c.Current)
	}
	tags := map[string]catalog.Tag{}
	for _, tg := range c.Tags {
		tags[tg.Name] = tg
	}
	for _, name := range []string{"include", "fragment", "template", "use", "checkbox", "select"} {
		if tg, ok := tags[name]; !ok || tg.Since != 4 {
			t.Errorf("tag %s: since %d (listed %v), want 4", name, tg.Since, ok)
		}
	}
	attrSince := func(tag, attr string) int {
		for _, a := range tags[tag].Attrs {
			if a.Name == attr {
				return a.Since
			}
		}
		return -1
	}
	for _, ta := range [][2]string{{"list", "single"}, {"progress", "indeterminate"}} {
		if s := attrSince(ta[0], ta[1]); s != 4 {
			t.Errorf("%s on %s: since %d, want 4", ta[1], ta[0], s)
		}
	}
	ok := func(s int) bool { return s == c.Current || s >= 1 && s <= 3 }
	for _, tg := range c.Tags {
		if !ok(tg.Since) {
			t.Errorf("tag %s: since %d", tg.Name, tg.Since)
		}
		for _, a := range tg.Attrs {
			if !ok(a.Since) {
				t.Errorf("%s on %s: since %d", a.Name, tg.Name, a.Since)
			}
		}
	}
}

// 132, and 172's last bullet (test 120 rerun under the current language's
// defaults). For no flag, --go, and --ts, tuimark new writes a document
// with no version that is clean at 40, 80, and 120 columns under both
// themes, and whose hint bar shows `q quit` at 80 columns: the bar shrinks
// to its items instead of sharing the row with the spacer.
func TestV04bIntegration132NewUnderTheCurrentLanguage(t *testing.T) {
	for _, flags := range [][]string{nil, {"--go"}, {"--ts"}} {
		name := "proj" + strings.Join(flags, "")
		dir, _ := newProject(t, name, flags...)
		doc, data := filepath.Join(dir, "app.tui"), filepath.Join(dir, "sample.json")
		src := readFileT(t, doc)
		if !strings.HasPrefix(src, "<tui") || strings.Contains(strings.SplitN(src, "\n", 2)[0], "version") {
			t.Errorf("%v: the document writes a version: %q", flags, strings.SplitN(src, "\n", 2)[0])
		}
		for _, theme := range []string{"dark", "light"} {
			if code, out, errw := runCLI("validate", doc, "--data", data, "--theme", theme); code != 0 || out != "ok\n" || errw != "" {
				t.Errorf("%v validate --theme %s: exit %d %q %q", flags, theme, code, out, errw)
			}
			for _, cols := range []string{"40", "80", "120"} {
				code, out, _ := runCLI("dump", doc, "--data", data, "--cols", cols, "--rows", "24", "--theme", theme, "--format", "json")
				var d struct {
					OK     bool              `json:"ok"`
					Errors []json.RawMessage `json:"errors"`
					Grid   []string          `json:"grid"`
				}
				if err := json.Unmarshal([]byte(out), &d); err != nil {
					t.Fatal(err)
				}
				if code != 0 || !d.OK || len(d.Errors) != 0 {
					t.Errorf("%v dump %s columns, %s: exit %d, diagnostics %s", flags, cols, theme, code, d.Errors)
				}
				last := ""
				if len(d.Grid) > 0 {
					last = d.Grid[len(d.Grid)-1]
				}
				if cols == "80" && !strings.Contains(last, "q quit") {
					t.Errorf("%v at 80 columns, %s: the hint bar has no `q quit`: %q", flags, theme, last)
				}
			}
		}
	}
}

// 215, through the CLI: validate --data reports nothing; without the
// archive's count, exactly one warning, in the via form; and the events
// of "tab enter" hold no with alias in keys.
func TestV04bIntegration215CLI(t *testing.T) {
	dir := composeFixtures + "t215/"
	kit := dir + "kit.tui"
	sameLines(t, "validate", vdiags(t, kit, dir+"kit.json"), []string{})
	noCount := writeTemp(t, "kit.json", `{"dialogs": {"delete": {"verb": "Delete", "count": 3, "cancel": "Cancel"}, "archive": {"verb": "Archive", "cancel": "Keep"}}}`)
	sameLines(t, "without count", vdiags(t, kit, noCount), []string{
		`warning B003 kit.tui:4:7 /screen/row/button id=arc__ok bind path "c.count" is missing (dialogs.archive.count via <use id="arc">)`,
	})
	code, out, errw := runCLI("play", kit, "--data", dir+"kit.json", "--cols", "30", "--rows", "3", "--input", "tab enter", "--format", "json")
	if code != 0 {
		t.Fatalf("play: exit %d %s", code, errw)
	}
	var p struct {
		Events any `json:"events"`
	}
	if err := json.Unmarshal([]byte(out), &p); err != nil {
		t.Fatal(err)
	}
	var want any
	_ = json.Unmarshal([]byte(`[{"step": 2, "action": "cancel", "source": "del__no", "keys": {}, "value": null}]`), &want)
	if !reflect.DeepEqual(p.Events, want) {
		t.Errorf("events %v, want %v", p.Events, want)
	}
}

// 223, through the CLI: validate --data d.json prints ok and exits 0, and
// inspect names the fragment's stylesheet as the rule that won border,
// over the built-in modal rule.
func TestV04bIntegration223CLI(t *testing.T) {
	dir := composeFixtures + "t223/"
	app, data := dir+"app.tui", dir+"d.json"
	if code, out, errw := runCLI("validate", app, "--data", data); code != 0 || out != "ok\n" || errw != "" {
		t.Errorf("validate: exit %d %q %q", code, out, errw)
	}
	code, out, errw := runCLI("inspect", app, "--data", data, "--id", "del__dialog", "--json")
	if code != 0 {
		t.Fatalf("inspect: exit %d %s", code, errw)
	}
	var in struct {
		Style []map[string]any `json:"style"`
	}
	if err := json.Unmarshal([]byte(out), &in); err != nil {
		t.Fatal(err)
	}
	var border map[string]any
	for _, s := range in.Style {
		if s["prop"] == "border" {
			border = s
		}
	}
	if border == nil {
		t.Fatalf("inspect has no border entry:\n%s", out)
	}
	over, _ := border["overridden"].([]any)
	if len(over) != 1 {
		t.Fatalf("border overridden %v", border["overridden"])
	}
	ua := over[0].(map[string]any)
	uaRule := ua["rule"].(map[string]any)
	if uaRule["file"] != "<ua>" || uaRule["selector"] != "modal" {
		t.Errorf("the overridden rule is %v, want <ua> modal", uaRule)
	}
	// The overridden rule's line and column are the built-in sheet's, which
	// the SPEC writes as {…}: compare the rest of the entry exactly.
	ua["rule"] = "…"
	var want map[string]any
	_ = json.Unmarshal([]byte(`{"prop": "border", "value": "rounded", "origin": "author", "rule": {"file": "kit/kit.tcss", "line": 1, "col": 13, "selector": ".k-dialog", "specificity": 10}, "overridden": [{"value": "single", "origin": "ua", "rule": "…"}]}`), &want)
	if !reflect.DeepEqual(border, want) {
		got, _ := json.Marshal(border)
		t.Errorf("border entry %s", got)
	}
}

// 225, through the CLI: the misplaced copy is exactly one L004 error at
// the template's <modal>, with the placement suffix (decision 102), and
// validate exits 2; moving the <use> after </col> removes it; the form in
// a modal validates clean.
func TestV04bIntegration225CLI(t *testing.T) {
	dir := composeFixtures + "t225/"
	app, data := dir+"app.tui", dir+"d.json"
	sameLines(t, "misplaced", vdiags(t, app, data), []string{
		`error L004 kit/kit.tui:7:5 /screen/col/modal id=del__dialog <modal> must be a direct child of <screen> (and come last), placed by <use id="del"> at app.tui:12:7`,
	})
	if code, _, _ := runCLI("validate", app, "--data", data); code != 2 {
		t.Errorf("validate exits %d, want 2", code)
	}
	raw, err := os.ReadFile(app)
	if err != nil {
		t.Fatal(err)
	}
	use := "      <use id=\"del\" template=\"k-confirm\" with=\"ui.dialogs.del as d\"/>\n    </col>\n"
	if !strings.Contains(string(raw), use) {
		t.Fatal("app.tui changed")
	}
	moved := filepath.Join(t.TempDir(), "app.tui")
	if err := os.WriteFile(moved, []byte(strings.Replace(string(raw), use, "    </col>\n    <use id=\"del\" template=\"k-confirm\" with=\"ui.dialogs.del as d\"/>\n", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(filepath.Dir(moved), "kit"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"kit.tui", "kit.tcss"} {
		if err := os.WriteFile(filepath.Join(filepath.Dir(moved), "kit", f), mustRead(t, dir+"kit/"+f), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if code, out, errw := runCLI("validate", moved, "--data", data); code != 0 || out != "ok\n" {
		t.Errorf("moved: exit %d %q %q", code, out, errw)
	}
	sameLines(t, "form", vdiags(t, dir+"form.tui", dir+"form.json"), []string{})
}
