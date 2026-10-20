package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// SPEC v0.4b §24 tests of composition in the tools (phase F4), the parts
// that go through the CLI: 147 (inspect source), 148's amplification
// bound, 150 (fmt on fragments), 151's preview --watch bullet, 226 (a
// fragment checked on its own), and 138's inspect source bullet. The
// language server's parts (152) are internal/lsp's; 151's run bullets are
// the Glyphrun spec run_reload_fragments.

// 150. fmt formats a fragment exactly as it formats a document: the same
// text with the root renamed <tui>, renamed back; the output is canonical;
// and an <include> is never read.
func TestV04bFmtOnFragments(t *testing.T) {
	file := composeFixtures + "t133/parts/templates.tui"
	code, out, errw := runCLI("fmt", file)
	if code != 0 {
		t.Fatalf("fmt: exit %d %s", code, errw)
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	asDoc := strings.Replace(strings.Replace(string(raw), "<fragment>", "<tui>", 1), "</fragment>", "</tui>", 1)
	code, docOut, errw := runCLI("fmt", writeTemp(t, "doc.tui", asDoc))
	if code != 0 {
		t.Fatalf("fmt of the document form: exit %d %s", code, errw)
	}
	back := strings.Replace(strings.Replace(docOut, "<tui>", "<fragment>", 1), "</tui>", "</fragment>", 1)
	if out != back {
		t.Errorf("fmt of the fragment:\n%s\nwant (the document's, renamed back):\n%s", out, back)
	}
	formatted := writeTemp(t, "templates.tui", out)
	if code, out, errw := runCLI("fmt", formatted, "--check"); code != 0 {
		t.Errorf("fmt --check of fmt's output: exit %d %s%s", code, out, errw)
	}

	// An include is formatted, never read: its file does not exist.
	inc := writeTemp(t, "inc.tui", `<fragment><include src="missing.tui"/></fragment>`)
	code, out, errw = runCLI("fmt", inc)
	if code != 0 || out != "<fragment>\n  <include src=\"missing.tui\"/>\n</fragment>\n" || errw != "" {
		t.Errorf("fmt of an include: exit %d %q %q", code, out, errw)
	}
	// --write rewrites FILE only.
	if code, _, errw := runCLI("fmt", inc, "--write"); code != 0 {
		t.Fatalf("fmt --write: exit %d %s", code, errw)
	}
	entries, err := os.ReadDir(filepath.Dir(inc))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("fmt --write left %d files, want inc.tui alone", len(entries))
	}
}

// 226. validate checks a fragment on its own: test 223's kit/kit.tui
// reports nothing, with or without --data; next to it, bad.tui reports
// only the checks its elements give themselves; every other command gives
// a fragment its V005.
func TestV04bFragmentCheckedOnItsOwn(t *testing.T) {
	dir := composeFixtures + "t226/"
	for _, args := range [][]string{{}, {"--data", dir + "d.json"}} {
		code, out, errw := runCLI(append([]string{"validate", dir + "kit/kit.tui"}, args...)...)
		if code != 0 || out != "ok (fragment checked on its own)\n" {
			t.Errorf("validate kit.tui %v: exit %d %q %s", args, code, out, errw)
		}
	}
	want := "error V003 bad.tui:2:3 /include <include> needs src=\"FILE\"\n" +
		"error V001 bad.tui:10:7 /template/modal/zzz unknown tag <zzz>\n" +
		"error V002 bad.tui:11:13 /template/modal/text attribute \"bogus\" is not allowed on <text>\n"
	for _, args := range [][]string{{}, {"--data", dir + "d.json"}} {
		code, out, errw := runCLI(append([]string{"validate", dir + "kit/bad.tui"}, args...)...)
		if code != 2 || out != want {
			t.Errorf("validate bad.tui %v: exit %d\n%s%s\nwant\n%s", args, code, out, errw, want)
		}
	}
	got := vdiags(t, dir+"kit/bad.tui", dir+"d.json")
	if len(got) != 3 {
		t.Errorf("validate --json: %v", got)
	}
	code, out, _ := runCLI("dump", dir+"kit/bad.tui", "--format", "json")
	errs, _ := jsonMap(t, out)["errors"].([]any)
	if code != 2 || len(errs) != 1 || errs[0].(map[string]any)["msg"] != "a <fragment> is not a document: check it through the document that includes it" {
		t.Errorf("dump bad.tui: exit %d %s", code, out)
	}
}

// A fragment checked on its own has a document's usage errors: a --theme
// other than dark or light, and a --catalog that cannot be read, exit 1
// before anything is checked, though neither adds a check there.
func TestV04bFragmentValidateFlags(t *testing.T) {
	kit := composeFixtures + "t226/kit/kit.tui"
	for _, args := range [][]string{
		{"--theme", "auto"},
		{"--theme", ""},
		{"--catalog", filepath.Join(t.TempDir(), "nope.json")},
		{"--catalog", writeTemp(t, "cat.json", `{"actions": 3}`)},
	} {
		code, out, errw := runCLI(append([]string{"validate", kit}, args...)...)
		if code != 1 || out != "" || !strings.HasPrefix(errw, "tuimark: ") {
			t.Errorf("validate kit.tui %q: exit %d %q %q", args, code, out, errw)
		}
	}
	for _, args := range [][]string{
		{"--theme", "light"},
		{"--catalog", writeTemp(t, "cat.json", `["other"]`)},
		{"--strict"},
	} {
		code, out, errw := runCLI(append([]string{"validate", kit}, args...)...)
		if code != 0 || out != "ok (fragment checked on its own)\n" {
			t.Errorf("validate kit.tui %q: exit %d %q %q", args, code, out, errw)
		}
	}
}

// inspectSource runs inspect --json with args and returns its source
// member as compact JSON ("" when it has none) and the exit code.
func inspectSource(t *testing.T, args ...string) (string, int) {
	t.Helper()
	code, out, errw := runCLI(append(append([]string{"inspect"}, args...), "--json")...)
	if code != 0 && code != 2 {
		t.Fatalf("inspect %v: exit %d %s", args, code, errw)
	}
	var m map[string]json.RawMessage
	mustUnmarshal(t, out, &m)
	src, ok := m["source"]
	if !ok {
		return "", code
	}
	return compactJSON(t, string(src)), code
}

// compactJSON is s without insignificant whitespace.
func compactJSON(t *testing.T, s string) string {
	t.Helper()
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(s)); err != nil {
		t.Fatalf("%v: %s", err, s)
	}
	return buf.String()
}

// 147. inspect's source names the element a node was built from and the
// uses and includes that placed it, innermost first; via is [] for an
// element the document writes itself; and a document that writes version
// has no source at all.
func TestV04bInspectSource(t *testing.T) {
	t133 := composeFixtures + "t133/"
	got, _ := inspectSource(t, t133+"app.tui", "--data", t133+"d.json", "--id", "billing__line__in")
	want := compactJSON(t, `{"file": "parts/templates.tui", "line": 9, "col": 5,
		"via": [{"use": "billing__line", "template": "field", "file": "parts/templates.tui", "line": 5, "col": 7},
		        {"use": "billing", "template": "address", "file": "app.tui", "line": 5, "col": 5}]}`)
	if got != want {
		t.Errorf("billing__line__in:\n got %s\nwant %s", got, want)
	}
	// Test 134's document has errors (exit 2); inspect still reports.
	app134 := composeFixtures + "t134/ui/app.tui"
	got, code := inspectSource(t, app134, "--id", "a")
	if want := `{"file":"parts/body.tui","line":2,"col":3,"via":[{"include":"parts/body.tui","file":"app.tui","line":4,"col":5}]}`; got != want || code != 2 {
		t.Errorf("a: exit %d\n got %s\nwant %s", code, got, want)
	}
	got, _ = inspectSource(t, app134, "--id", "main")
	if want := `{"file":"app.tui","line":2,"col":3,"via":[]}`; got != want {
		t.Errorf("main: got %s want %s", got, want)
	}
	got, _ = inspectSource(t, t133+"flat.tui", "--data", t133+"d.json", "--id", "billing__line__in")
	if want := `{"file":"flat.tui","line":6,"col":7,"via":[]}`; got != want {
		t.Errorf("flat.tui: got %s want %s", got, want)
	}

	// The text form: a source line, then one line per via entry.
	_, out, _ := runCLI("inspect", t133+"app.tui", "--data", t133+"d.json", "--id", "billing__line__in")
	wantText := "source parts/templates.tui:9:5\n" +
		"via <use id=\"billing__line\" template=\"field\"> parts/templates.tui:5:7\n" +
		"via <use id=\"billing\" template=\"address\"> app.tui:5:5\n"
	if !strings.HasSuffix(out, wantText) {
		t.Errorf("text output:\n%s\nwant it to end with\n%s", out, wantText)
	}
	_, out, _ = runCLI("inspect", app134, "--id", "a")
	if !strings.HasSuffix(out, "source parts/body.tui:2:3\nvia <include src=\"parts/body.tui\"> app.tui:4:5\n") {
		t.Errorf("text output of a:\n%s", out)
	}
	_, out, _ = runCLI("inspect", t133+"flat.tui", "--data", t133+"d.json", "--id", "billing__line__in")
	if !strings.HasSuffix(out, "\nsource flat.tui:6:7\n") || strings.Contains(out, "\nvia ") {
		t.Errorf("text output of flat.tui:\n%s", out)
	}

	// A document that writes version: no source, in either form.
	for _, v := range []string{"1", "2", "3"} {
		doc := writeTemp(t, "app.tui", `<tui version="`+v+`"><screen id="main"><text id="a">x</text></screen></tui>`)
		if got, _ := inspectSource(t, doc, "--id", "a"); got != "" {
			t.Errorf("version=%s: source %s", v, got)
		}
		if _, out, _ := runCLI("inspect", doc, "--id", "a"); strings.Contains(out, "source ") {
			t.Errorf("version=%s: text output has a source line:\n%s", v, out)
		}
	}
}

// 138, the inspect bullet: the copy's input is the template's input,
// placed by the use.
func TestV04bInspectSource138(t *testing.T) {
	dir := composeFixtures + "t138/"
	got, _ := inspectSource(t, dir+"app.tui", "--data", dir+"d.json", "--id", "bill__in")
	want := compactJSON(t, `{"file": "app.tui", "line": 5, "col": 7, "via": [{"use": "bill", "template": "field", "file": "app.tui", "line": 13, "col": 5}]}`)
	if got != want {
		t.Errorf("bill__in:\n got %s\nwant %s", got, want)
	}
}

// A use without id is listed with "use": "" and named <use template="T">
// in the text form.
func TestV04bInspectSourceUseWithoutID(t *testing.T) {
	doc := writeTemp(t, "app.tui", "<tui>\n  <template id=\"t\"><text>x</text></template>\n  <screen id=\"main\">\n    <use template=\"t\"/>\n  </screen>\n</tui>\n")
	got, _ := inspectSource(t, doc, "--at", "0,0")
	if want := `{"file":"app.tui","line":2,"col":20,"via":[{"use":"","template":"t","file":"app.tui","line":4,"col":5}]}`; got != want {
		t.Errorf("source %s, want %s", got, want)
	}
	_, out, _ := runCLI("inspect", doc, "--at", "0,0")
	if !strings.HasSuffix(out, "source app.tui:2:20\nvia <use template=\"t\"> app.tui:4:5\n") {
		t.Errorf("text output:\n%s", out)
	}
}

// A generated node is named by the element it is generated from (SPEC
// v0.4b §15.7): an each row's node by its template element, a table's
// header and body cells by their column, a table row by its item or, with
// none, by the table, a tab label by its tab, and a hint item by its
// <bind> row; each with the include that placed it.
func TestV04bInspectSourceOfGeneratedNodes(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"app.tui":        "<tui>\n  <include src=\"parts/keys.tui\"/>\n  <screen id=\"main\">\n    <include src=\"parts/body.tui\"/>\n  </screen>\n</tui>\n",
		"parts/keys.tui": "<fragment>\n  <keymap>\n    <bind keys=\"q\" action=\"quit\" label=\"quit\"/>\n  </keymap>\n</fragment>\n",
		"parts/body.tui": `<fragment>
  <hints id="h" scope="all" style="height: 1"/>
  <list id="l" each="rows as r" key="r.id" style="height: 2">
    <item><text>{r.name}</text></item>
  </list>
  <table id="t" each="rows as r" key="r.id" style="height: 3">
    <column title="Name">{r.name}</column>
  </table>
  <table id="u" each="rows as r" key="r.id" style="height: 3">
    <column title="Name">{r.name}</column>
    <item class="row"/>
  </table>
  <tabs id="nav" style="height: 2">
    <tab id="one" label="one"><text>first</text></tab>
  </tabs>
</fragment>
`,
		"d.json": `{"rows": [{"id": "a", "name": "alpha"}, {"id": "b", "name": "beta"}]}`,
	})
	keys := `"via":[{"include":"parts/keys.tui","file":"app.tui","line":2,"col":3}]}`
	body := `"via":[{"include":"parts/body.tui","file":"app.tui","line":4,"col":5}]}`
	for _, c := range []struct{ at, path, src string }{
		{"0,0", "/screen#main/hints#h/row/text[0]", `{"file":"parts/keys.tui","line":3,"col":5,` + keys},
		{"1,0", "/screen#main/hints#h/row", `{"file":"parts/keys.tui","line":3,"col":5,` + keys},
		{"0,1", "/screen#main/list#l/item[0]/text", `{"file":"parts/body.tui","line":4,"col":11,` + body},
		{"0,3", "/screen#main/table#t/column", `{"file":"parts/body.tui","line":7,"col":5,` + body},
		{"0,4", "/screen#main/table#t/item[0]/text", `{"file":"parts/body.tui","line":7,"col":5,` + body},
		{"10,4", "/screen#main/table#t/item[0]", `{"file":"parts/body.tui","line":6,"col":3,` + body},
		{"10,7", "/screen#main/table#u/item[0]", `{"file":"parts/body.tui","line":11,"col":5,` + body},
		{"0,9", "/screen#main/tabs#nav/text", `{"file":"parts/body.tui","line":14,"col":5,` + body},
		{"0,10", "/screen#main/tabs#nav/tab#one/text", `{"file":"parts/body.tui","line":14,"col":31,` + body},
	} {
		code, out, errw := runCLI("inspect", filepath.Join(dir, "app.tui"), "--data", filepath.Join(dir, "d.json"),
			"--cols", "30", "--rows", "12", "--at", c.at, "--json")
		if code != 0 {
			t.Fatalf("--at %s: exit %d %s", c.at, code, errw)
		}
		var m struct {
			Path   string          `json:"path"`
			Source json.RawMessage `json:"source"`
		}
		mustUnmarshal(t, out, &m)
		if got := compactJSON(t, string(m.Source)); m.Path != c.path || got != c.src {
			t.Errorf("--at %s: path %s source %s\nwant path %s source %s", c.at, m.Path, got, c.path, c.src)
		}
	}
}

// The via list of a node at the bottom of nested uses stays linear in the
// document. Each use's id there is its scoped id, which holds the ids of
// every use around it, so printed whole the list would grow with the
// square of the nesting; cut as ir.ShownID cuts an id, it prints less than
// the document.
func TestV04bInspectViaIsLinear(t *testing.T) {
	const depth = 200
	doc, scoped := nestedUses(depth, 300)
	for _, form := range [][]string{{"--json"}, {}} {
		r := runProbe(t, map[string]string{"app.tui": doc}, append([]string{"inspect", "FILE", "--id", scoped}, form...)...)
		r.bounded(t, fmt.Sprint("inspect ", form))
		// The node's own id is printed whole wherever inspect names the
		// node; the via list is what must not grow with the nesting.
		if n := len(r.out) + len(r.errw) - strings.Count(r.out, scoped)*len(scoped); n > r.inputBytes {
			t.Errorf("inspect %v printed %d bytes besides the node's id for %d of input", form, n, r.inputBytes)
		}
		if c := strings.Count(r.out, "x…"); c < depth {
			t.Errorf("inspect %v: %d cut ids, want at least %d", form, c, depth)
		}
	}
}

// nestedUses is a document of depth templates, each holding a use of the
// next one with an id of idLen bytes, the last holding <text id="z">,
// and a screen that uses the first; and the scoped id of that text.
func nestedUses(depth, idLen int) (string, string) {
	var doc strings.Builder
	doc.WriteString("<tui>\n")
	ids := make([]string, depth)
	for k := range depth {
		ids[k] = fmt.Sprintf("u%d%s", k, strings.Repeat("x", idLen))
		fmt.Fprintf(&doc, "<template id=\"t%d\"><use id=\"%s\" template=\"t%d\"/></template>\n", k, ids[k], k+1)
	}
	fmt.Fprintf(&doc, "<template id=\"t%d\"><text id=\"z\">x</text></template>\n", depth)
	doc.WriteString("<screen id=\"main\"><use id=\"top\" template=\"t0\"/></screen>\n</tui>\n")
	return doc.String(), "top__" + strings.Join(ids, "__") + "__z"
}

// The IR's from is FILE:LINE of the element a node was built from, and
// nothing of the uses around it, so it does not grow with the nesting.
// What the IR prints besides the nodes' ids stays linear in the document;
// the scoped ids themselves are what the expansion limit charges (ADR
// 0034).
func TestV04bIRFromIsConstant(t *testing.T) {
	doc, _ := nestedUses(200, 300)
	r := runProbe(t, map[string]string{"app.tui": doc}, "ir", "FILE")
	r.bounded(t, "ir")
	froms, ids := 0, 0
	var walk func(n map[string]any)
	walk = func(n map[string]any) {
		if id, ok := n["id"].(string); ok {
			ids += len(id)
		}
		if f, ok := n["from"]; ok {
			froms++
			if s, _ := f.(string); !strings.HasPrefix(s, "app.tui:") || len(s) > len("app.tui:999") {
				t.Errorf("from %q", s)
			}
		}
		for _, c := range kids(t, n) {
			walk(c)
		}
	}
	m := jsonMap(t, r.out)
	walk(m["root"].(map[string]any))
	if froms != 201 {
		t.Errorf("%d nodes with from, want 201 (the uses and the text the screen's use places)", froms)
	}
	// Measured without the indentation, which grows with the depth of any
	// tree, written or placed.
	if n := len(compactJSON(t, r.out)) - ids; n > 10*r.inputBytes {
		t.Errorf("ir printed %d bytes besides the ids and the indentation for %d of input", n, r.inputBytes)
	}
}
