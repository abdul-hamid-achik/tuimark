package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/host"
	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// SPEC v0.4b §24 tests 134-137 (composition, phase F1), the parts that go
// through the CLI. The Go API parts are the root package's
// conformance_v04b_compose_test.go; the fixtures are testdata/v04b/t134,
// t134ir, t136, and t137 at the repository root.

const composeFixtures = "../../testdata/v04b/"

// jsonMap decodes one JSON object.
func jsonMap(t *testing.T, raw string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, raw)
	}
	return m
}

// kids is the children member of an IR node.
func kids(t *testing.T, n map[string]any) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, c := range n["children"].([]any) {
		out = append(out, c.(map[string]any))
	}
	return out
}

// 134, the ir bullet: the screen's second child is the include, holding
// the nodes it inserted with `from`; the texts around it have none. The
// fixture of the test has two errors (bogus, sideways), and `ir` exits 2
// on an error-severity diagnostic (SPEC §15.1) without printing the IR, so
// the IR is read from the same files with the two errors removed
// (t134ir); the fixture itself shows the exit code. Reported as an erratum.
func TestV04bIncludeInIR(t *testing.T) {
	code, out, errw := runCLI("ir", composeFixtures+"t134/ui/app.tui")
	if code != 2 || out != "" || !strings.Contains(errw, "V002 parts/body.tui:3:16") {
		t.Errorf("ir of the fixture with errors: exit %d, stdout %q, stderr %s", code, out, errw)
	}
	code, out, errw = runCLI("ir", composeFixtures+"t134ir/ui/app.tui")
	if code != 0 {
		t.Fatalf("ir: exit %d %s%s", code, out, errw)
	}
	if errs := validateSchema(t, irV04Schema, decodeJSON(t, []byte(out))); len(errs) != 0 {
		t.Errorf("IR vs ir.v0.4.json: %v", errs)
	}
	root := jsonMap(t, out)["root"].(map[string]any)
	screen := kids(t, root)[0]
	cs := kids(t, screen)
	if len(cs) != 4 {
		t.Fatalf("the screen has %d children, want top, include, bottom, include", len(cs))
	}
	inc := cs[1]
	if inc["kind"] != "include" || !reflect.DeepEqual(inc["attrs"], map[string]any{"src": "parts/body.tui"}) {
		t.Errorf("second child: %v", inc)
	}
	ins := kids(t, inc)
	if len(ins) != 2 || ins[0]["from"] != "parts/body.tui:2" || ins[1]["from"] != "parts/body.tui:3" {
		t.Errorf("inserted nodes: %v", ins)
	}
	for _, i := range []int{0, 2} {
		if _, has := cs[i]["from"]; has {
			t.Errorf("child %d has from: %v", i, cs[i])
		}
	}
	foot := kids(t, cs[3])
	if cs[3]["kind"] != "include" || len(foot) != 1 || foot[0]["from"] != "../shared/foot.tui:2" {
		t.Errorf("fourth child: %v", cs[3])
	}
	// from is a node's last member.
	if i, j := strings.Index(out, `"from": "parts/body.tui:2"`), strings.Index(out[strings.Index(out, `"from": "parts/body.tui:2"`):], "}"); i < 0 || strings.TrimSpace(out[i+len(`"from": "parts/body.tui:2"`):i+j]) != "" {
		t.Errorf("from is not the last member")
	}
}

// 135, the last bullet: validate checks a fragment on its own (nothing is
// reported for an include, whose file is not read); every other command
// gives the V005 of a fragment given as a document, and exits 2.
func TestV04bValidateFragmentOnItsOwn(t *testing.T) {
	dir := t.TempDir()
	write := func(name, text string) string {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	a := write("parts/a.tui", "<fragment>\n  <include src=\"b.tui\"/>\n</fragment>")
	write("parts/b.tui", "<fragment>\n  <include src=\"a.tui\"/>\n</fragment>")

	code, out, errw := runCLI("validate", a)
	if code != 0 || out != "ok (fragment checked on its own)\n" {
		t.Errorf("validate: exit %d %q %s", code, out, errw)
	}
	code, out, errw = runCLI("validate", a, "--json")
	m := jsonMap(t, out)
	if code != 0 || m["ok"] != true || len(m["diagnostics"].([]any)) != 0 || m["file"] != a {
		t.Errorf("validate --json: exit %d %s %s", code, out, errw)
	}
	code, out, errw = runCLI("dump", a, "--format", "json")
	d := jsonMap(t, out)
	errs, _ := d["errors"].([]any)
	if code != 2 || len(errs) != 1 || errs[0].(map[string]any)["code"] != "V005" ||
		errs[0].(map[string]any)["msg"] != "a <fragment> is not a document: check it through the document that includes it" {
		t.Errorf("dump: exit %d %s %s", code, out, errw)
	}
	// On its own, a fragment gets the checks its elements give themselves.
	o := write("orphan.tui", "<fragment>\n  <zzz/>\n  <text bogus=\"1\">x</text>\n  <include/>\n</fragment>")
	code, out, _ = runCLI("validate", o)
	want := []string{
		"error V001 orphan.tui:2:3 /zzz unknown tag <zzz>",
		`error V002 orphan.tui:3:9 /text attribute "bogus" is not allowed on <text>`,
		`error V003 orphan.tui:4:3 /include <include> needs src="FILE"`,
	}
	if code != 2 || strings.TrimRight(out, "\n") != strings.Join(want, "\n") {
		t.Errorf("validate orphan: exit %d\n%s", code, out)
	}
	// A fragment that is not well-formed gets its V005.
	b := write("broken.tui", "<fragment><text>x</fragment>")
	if code, out, _ := runCLI("validate", b); code != 2 || !strings.Contains(out, "V005") {
		t.Errorf("validate broken: exit %d %s", code, out)
	}
}

// 136. A fragment holds the document's parts, through the commands.
func TestV04bFragmentHoldsTheDocumentsPartsCLI(t *testing.T) {
	app := composeFixtures + "t136/app.tui"
	if code, out, errw := runCLI("validate", app); code != 0 || out != "ok (bindings not checked: no --data)\n" {
		t.Errorf("validate: exit %d %q %s", code, out, errw)
	}
	code, out, errw := runCLI("ir", app)
	if code != 0 {
		t.Fatalf("ir: exit %d %s", code, errw)
	}
	wantKeymap := []any{
		map[string]any{"keys": "q", "action": "quit"},
		map[string]any{"keys": "x", "action": "go"},
	}
	if got := jsonMap(t, out)["keymap"]; !reflect.DeepEqual(got, wantKeymap) {
		t.Errorf("keymap: %v, want %v", got, wantKeymap)
	}
	code, out, errw = runCLI("play", app, "--input", "x", "--format", "json")
	p := decodePlay(t, out)
	if code != 0 || len(p.Events) != 1 || p.Events[0].Step != 1 || p.Events[0].Action != "go" || p.Events[0].Source != "" || len(p.Events[0].Keys) != 0 || p.Events[0].Value != nil {
		t.Errorf("play: exit %d, events %+v: %s", code, p.Events, errw)
	}
	code, out, errw = runCLI("dump", app, "--cols", "10", "--rows", "2", "--styles", "--format", "json")
	if code != 0 {
		t.Fatalf("dump: exit %d %s", code, errw)
	}
	var d struct {
		Styles [][]struct {
			X, W int
			A    []string
		} `json:"styles"`
	}
	if err := json.Unmarshal([]byte(out), &d); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range d.Styles[0] {
		if s.X == 0 && s.W >= 2 && len(s.A) == 1 && s.A[0] == "bold" {
			found = true
		}
	}
	if !found {
		t.Errorf("row 0 spans: %+v, want one covering columns 0 and 1 with bold", d.Styles[0])
	}
}

// 137, the ir bullet: the same stylesheet loaded by a fragment included
// twice is listed once, as joined to the fragment's directory. The
// document has the V004 of the test, so ir exits 2 and the IR is built
// from the loaded document (reported with test 134's).
func TestV04bSameFragmentTwiceIR(t *testing.T) {
	app, err := host.Load(composeFixtures + "t137/app.tui")
	if err != nil {
		t.Fatal(err)
	}
	doc := app.Document()
	b := parse.BuildIR(doc, nil)
	if !reflect.DeepEqual(b.Stylesheets, []string{"look.tcss"}) {
		t.Errorf("stylesheets: %v, want [look.tcss]", b.Stylesheets)
	}
	if got := len(app.Sheets()); got != 1 {
		t.Errorf("%d stylesheets loaded, want 1", got)
	}
	if code, _, errw := runCLI("ir", composeFixtures+"t137/app.tui"); code != 2 || !strings.Contains(errw, "V004 app.tui:6:5") {
		t.Errorf("ir: exit %d %s", code, errw)
	}
}

// B004 for a keymap row written in a fragment is in that file and names the
// include, as B004 for an on:* attribute does (SPEC v0.4b §6.18.6; review of
// phase F1, round 3).
func TestV04bCatalogWarningInAFragmentNamesTheInclude(t *testing.T) {
	dir := t.TempDir()
	app := filepath.Join(dir, "app.tui")
	if err := os.WriteFile(app, []byte("<tui>\n  <include src=\"keys.tui\"/>\n  <screen id=\"s\"/>\n</tui>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "keys.tui"), []byte("<fragment>\n  <keymap>\n    <bind keys=\"a\" action=\"nope\"/>\n  </keymap>\n</fragment>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cat := writeTemp(t, "C.json", `["known"]`)
	code, out, errw := runCLI("validate", app, "--catalog", cat)
	want := `warning B004 keys.tui:3:5 /keymap/bind action "nope" (keys a) is not in the catalog, placed by <include> at app.tui:2:3`
	if code != 0 || !strings.Contains(out, want) {
		t.Errorf("validate: exit %d\n%s%s\nwant a line %s", code, out, errw, want)
	}
}
