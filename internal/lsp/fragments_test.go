package lsp

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// SPEC v0.4b §15.12, test 152 (phase F4): a fragment's URI gets the
// diagnostics the documents that read it give its file, found among the
// open documents and the .tui files of its directory and the parent; one
// no document reads is checked on its own; definition follows <include
// src>, template=, a scoped #id, and a use's #id across files; completion
// offers the expanded document's template ids and scoped ids.

// v04b reads a file of the conformance fixtures (testdata/v04b).
func v04b(t *testing.T, rel string) string {
	t.Helper()
	return mustRead(t, filepath.Join("..", "..", "testdata", "v04b", rel))
}

// t134Tree writes test 134's files, with app as ui/app.tui.
func t134Tree(t *testing.T, app string) string {
	t.Helper()
	return writeTree(t, map[string]string{
		"ui/app.tui":        app,
		"ui/parts/body.tui": v04b(t, "t134/ui/parts/body.tui"),
		"shared/foot.tui":   v04b(t, "t134/shared/foot.tui"),
	})
}

// codesAt is each diagnostic as "CODE LINE:CHAR", 0-based as LSP's.
func codesAt(ds []lspDiagnostic) []string {
	var out []string
	for _, d := range ds {
		out = append(out, d.Code+" "+itoa(d.Range.Start.Line)+":"+itoa(d.Range.Start.Character))
	}
	return out
}

// 152, the first bullet: only ui/parts/body.tui is open; its V002 comes
// from ui/app.tui in the parent directory, and the V003 of shared/foot.tui
// is not routed to it.
func TestFragmentDiagnosticsComeFromItsIncluder(t *testing.T) {
	dir := t134Tree(t, v04b(t, "t134/ui/app.tui"))
	c := startServer(t, Options{})
	c.initialize()
	body := filepath.Join(dir, "ui/parts/body.tui")
	p := c.nextPublish(c.open(body, "tuimark", mustRead(t, body)))
	if got := codesAt(p.Diagnostics); len(got) != 1 || got[0] != "V002 2:15" {
		t.Errorf("body.tui: %v, want [V002 2:15]", got)
	}
	_ = c.shutdown()
}

// What only the document that reads a fragment can say reaches the
// fragment's URI: an id the document writes before the include makes the
// fragment's id a V004, which the fragment checked on its own cannot see.
func TestFragmentGetsWhatOnlyItsIncluderSees(t *testing.T) {
	app := strings.Replace(v04b(t, "t134/ui/app.tui"), "<text>top</text>", `<text id="a">top</text>`, 1)
	dir := t134Tree(t, app)
	c := startServer(t, Options{})
	c.initialize()
	body := filepath.Join(dir, "ui/parts/body.tui")
	p := c.nextPublish(c.open(body, "tuimark", mustRead(t, body)))
	if got := codesAt(p.Diagnostics); len(got) != 2 || got[0] != "V004 1:2" || got[1] != "V002 2:15" {
		t.Errorf("body.tui: %v, want [V004 1:2 V002 2:15]", got)
	}
	_ = c.shutdown()
}

// The fragment's URI follows its includer: opened together, a change to
// the document republishes the fragment, and a change to the fragment
// republishes the document; once the document no longer includes it, the
// fragment is checked on its own again.
func TestFragmentAndIncluderRepublishEachOther(t *testing.T) {
	fc := &fakeClock{}
	srv := newServer(Options{})
	srv.afterFunc = fc.afterFunc
	c := startWith(t, srv)
	c.initialize()
	withID := strings.Replace(v04b(t, "t134/ui/app.tui"), "<text>top</text>", `<text id="a">top</text>`, 1)
	dir := t134Tree(t, withID)
	appPath, bodyPath := filepath.Join(dir, "ui/app.tui"), filepath.Join(dir, "ui/parts/body.tui")
	appURI := c.open(appPath, "tuimark", withID)
	bodyURI := c.open(bodyPath, "tuimark", mustRead(t, bodyPath))
	fireAll := func() {
		c.call("textDocument/hover", map[string]any{"textDocument": map[string]any{"uri": appURI}, "position": Position{}})
		for i := 0; i < 4 && fc.fire() > 0; i++ {
			c.call("textDocument/hover", map[string]any{"textDocument": map[string]any{"uri": appURI}, "position": Position{}})
		}
	}
	fireAll()
	c.nextPublish(appURI)
	if got := codesAt(c.nextPublish(bodyURI).Diagnostics); len(got) != 2 {
		t.Fatalf("body.tui: %v, want V004 and V002", got)
	}
	c.notes = nil

	// A change to the fragment: the document is published again.
	c.change(bodyURI, 2, strings.Replace(mustRead(t, bodyPath), ` bogus="1"`, "", 1))
	fireAll()
	c.nextPublish(appURI)
	if got := codesAt(c.nextPublish(bodyURI).Diagnostics); len(got) != 1 || got[0] != "V004 1:2" {
		t.Errorf("body.tui fixed: %v, want [V004 1:2]", got)
	}
	c.notes = nil

	// A change to the document: the fragment is published again, and with
	// no include left it is checked on its own.
	c.change(appURI, 2, strings.Replace(withID, `<include src="parts/body.tui"/>`, "", 1))
	fireAll()
	if got := codesAt(c.nextPublish(bodyURI).Diagnostics); len(got) != 0 {
		t.Errorf("body.tui on its own: %v, want none", got)
	}
	_ = c.shutdown()
}

// A closed fragment, like a closed stylesheet, gets the diagnostics the
// open document gives its file; a stylesheet a fragment loads resolves
// against the fragment's directory and gets its own.
func TestClosedFragmentsAndTheirStylesheets(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"ui/app.tui":        strings.Replace(v04b(t, "t134/ui/app.tui"), "<screen", `<include src="parts/kit.tui"/>`+"\n  <screen", 1),
		"ui/parts/body.tui": v04b(t, "t134/ui/parts/body.tui"),
		"shared/foot.tui":   v04b(t, "t134/shared/foot.tui"),
		"ui/parts/kit.tui":  "<fragment>\n  <style src=\"kit.tcss\"/>\n</fragment>\n",
		"ui/parts/kit.tcss": "screen { bogus-prop: 1; }\n",
	})
	c := startServer(t, Options{})
	c.initialize()
	appPath := filepath.Join(dir, "ui/app.tui")
	c.nextPublish(c.open(appPath, "tuimark", mustRead(t, appPath)))
	for _, w := range []struct{ file, want string }{
		{"ui/parts/body.tui", "V002 2:15"},
		{"shared/foot.tui", "V003 1:15"},
		{"ui/parts/kit.tcss", "V003 0:9"},
	} {
		if got := codesAt(c.nextPublish(pathToURI(filepath.Join(dir, w.file))).Diagnostics); len(got) != 1 || got[0] != w.want {
			t.Errorf("%s: %v, want [%s]", w.file, got, w.want)
		}
	}
	_ = c.shutdown()
}

// 152, the second bullet: an open fragment no document reads is checked
// on its own, with no V005.
func TestOrphanFragment(t *testing.T) {
	dir := writeTree(t, nil)
	c := startServer(t, Options{})
	c.initialize()
	uri := c.open(filepath.Join(dir, "orphan.tui"), "tuimark", "<fragment>\n  <zzz/>\n  <text bogus=\"1\">x</text>\n</fragment>")
	if got := codesAt(c.nextPublish(uri).Diagnostics); len(got) != 2 || got[0] != "V001 1:2" || got[1] != "V002 2:8" {
		t.Errorf("orphan.tui: %v, want [V001 1:2 V002 2:8]", got)
	}
	_ = c.shutdown()
}

// definitionAt asks for the definition at the first occurrence of mark in
// text, plus skip bytes.
func definitionAt(t *testing.T, c *client, uri, text, mark string, skip int) []location {
	t.Helper()
	i := strings.Index(text, mark)
	if i < 0 {
		t.Fatalf("no %q", mark)
	}
	r := c.call("textDocument/definition", map[string]any{"textDocument": map[string]any{"uri": uri}, "position": positionAt(text, i+skip)})
	var locs []location
	if string(r.Result) != "null" {
		if err := json.Unmarshal(r.Result, &locs); err != nil {
			t.Fatalf("%v: %s", err, r.Result)
		}
	}
	return locs
}

// t133Tree writes test 133's files.
func t133Tree(t *testing.T) string {
	t.Helper()
	return writeTree(t, map[string]string{
		"app.tui":             v04b(t, "t133/app.tui"),
		"parts/templates.tui": v04b(t, "t133/parts/templates.tui"),
	})
}

// 152, the third bullet, and the other references composition adds.
func TestDefinitionAcrossFiles(t *testing.T) {
	dir := t133Tree(t)
	c := startServer(t, Options{})
	c.initialize()
	app := v04b(t, "t133/app.tui")
	uri := c.open(filepath.Join(dir, "app.tui"), "tuimark", app)
	tmpl := pathToURI(filepath.Join(dir, "parts/templates.tui"))
	for _, tc := range []struct {
		name, mark string
		skip       int
		want       location
	}{
		{"template=", `"address"`, 2, location{URI: tmpl, Range: Range{Start: Position{1, 2}, End: Position{1, 11}}}},
		{"include src", `parts/templates.tui`, 3, location{URI: tmpl}},
		{"a scoped #id", `billing__line__in`, 3, location{URI: tmpl, Range: Range{Start: Position{8, 4}, End: Position{8, 10}}}},
	} {
		locs := definitionAt(t, c, uri, app, tc.mark, tc.skip)
		if len(locs) != 1 || locs[0] != tc.want {
			t.Errorf("%s: %+v, want %+v", tc.name, locs, tc.want)
		}
	}
	// A use's #id resolves to that <use>, in the document or in a
	// template (a nested use's scoped id).
	focused := strings.Replace(app, "#billing__line__in", "#billing", 1)
	c.change(uri, 2, focused)
	if locs := definitionAt(t, c, uri, focused, `#billing"`, 2); len(locs) != 1 || locs[0].URI != uri || locs[0].Range.Start != (Position{4, 4}) {
		t.Errorf("#billing: %+v, want the <use> at 4:4 of app.tui", locs)
	}
	nested := strings.Replace(app, "#billing__line__in", "#shipping__line", 1)
	c.change(uri, 3, nested)
	if locs := definitionAt(t, c, uri, nested, `#shipping__line`, 2); len(locs) != 1 || locs[0].URI != tmpl || locs[0].Range.Start != (Position{4, 6}) {
		t.Errorf("#shipping__line: %+v, want the <use> at 4:6 of parts/templates.tui", locs)
	}
	// In a fragment: through the document that reads it.
	tpath := filepath.Join(dir, "parts/templates.tui")
	ttext := mustRead(t, tpath)
	turi := c.open(tpath, "tuimark", ttext)
	if locs := definitionAt(t, c, turi, ttext, `"field"`, 2); len(locs) != 1 || locs[0].URI != turi || locs[0].Range.Start != (Position{7, 2}) {
		t.Errorf("template=\"field\" in the fragment: %+v, want its <template> at 7:2", locs)
	}
	// A pinned document keeps 0.4.1's definitions: no template= there.
	pinned := "<tui version=\"3\">\n  <screen id=\"s\" focus=\"#x\">\n    <text id=\"x\">x</text>\n  </screen>\n</tui>\n"
	puri := c.open(filepath.Join(dir, "pinned.tui"), "tuimark", pinned)
	if locs := definitionAt(t, c, puri, pinned, `#x"`, 1); len(locs) != 1 || locs[0].Range.Start != (Position{2, 4}) {
		t.Errorf("pinned #x: %+v", locs)
	}
	_ = c.shutdown()
}

// 152, the fourth bullet: completion in template= offers the expanded
// document's templates, and the #id targets add the copies' scoped ids.
func TestCompletionOfTemplatesAndScopedIDs(t *testing.T) {
	dir := t133Tree(t)
	c := startServer(t, Options{})
	c.initialize()
	app := v04b(t, "t133/app.tui")
	for _, tc := range []struct {
		name, text string
		want       []string
	}{
		{"template=", strings.Replace(app, `template="address" with="order.billing`, `template="|" with="order.billing`, 1), []string{"address", "field"}},
		{"focus=", strings.Replace(app, `focus="#billing__line__in"`, `focus="#|"`, 1), []string{"#billing__line__in", "#billing__box", "#shipping__box", "#shipping__line__in", "#main"}},
	} {
		text := strings.Replace(tc.text, "|", "", 1)
		uri := c.open(filepath.Join(dir, "app.tui"), "tuimark", text)
		got := completionLabels(t, c, uri, positionAt(text, strings.IndexByte(tc.text, '|')))
		for _, w := range tc.want {
			if !got[w] {
				t.Errorf("%s: %v, missing %q", tc.name, keys(got), w)
			}
		}
	}
	_ = c.shutdown()
}

// Completion offers every scoped id of every copy, so its cost is per
// copy: once the document is analyzed, offering them costs less than the
// analysis did (each item's range is the request's, computed once, not
// once per item over the whole text).
func TestCompletionOfManyScopedIDs(t *testing.T) {
	const uses, ids = 3000, 8
	var b strings.Builder
	b.WriteString("<tui>\n  <template id=\"t\">\n    <col>\n")
	for k := range ids {
		b.WriteString("      <text id=\"x" + itoa(k) + "\">x</text>\n")
	}
	b.WriteString("    </col>\n  </template>\n  <screen id=\"main\">\n")
	for k := range uses {
		b.WriteString("    <use id=\"u" + itoa(k) + "\" template=\"t\"/>\n")
	}
	// The cursor at the end of a long text, where a position costs the
	// most.
	b.WriteString("  </screen>\n  <!-- " + strings.Repeat("long text\n", 200000) + " -->\n")
	b.WriteString("  <keymap>\n    <bind keys=\"x\" action=\"focus\" to=\"#|\"/>\n  </keymap>\n</tui>\n")
	marked := b.String()
	text := strings.Replace(marked, "|", "", 1)
	dir := writeTree(t, nil)
	srv := newServer(Options{})
	d := &document{uri: pathToURI(filepath.Join(dir, "app.tui")), path: filepath.Join(dir, "app.tui"), kind: kindTUI, text: text}
	sn := &snapshot{byURI: map[string]*document{d.uri: d}, open: map[string]string{d.path: text}}
	start := time.Now()
	srv.analysisOf(sn, d.path, d.text)
	analyzed := time.Since(start)
	start = time.Now()
	list := srv.complete(sn, d, positionAt(text, strings.IndexByte(marked, '|')))
	completed := time.Since(start)
	if n := len(list.Items); n < uses*ids {
		t.Fatalf("%d items, want at least %d", n, uses*ids)
	}
	if completed > analyzed {
		t.Errorf("completion took %v, the analysis %v", completed, analyzed)
	}
}

// 152, the last bullet: a non-canonical fragment is formatted as fmt
// formats it.
func TestFormattingAFragment(t *testing.T) {
	dir := writeTree(t, nil)
	c := startServer(t, Options{})
	c.initialize()
	messy := "<fragment>\n<text   id=\"a\">x</text>\n</fragment>"
	want, err := parse.Format([]byte(messy))
	if err != nil {
		t.Fatal(err)
	}
	uri := c.open(filepath.Join(dir, "kit.tui"), "tuimark", messy)
	var edits []textEdit
	r := c.call("textDocument/formatting", map[string]any{"textDocument": map[string]any{"uri": uri}, "options": map[string]any{}})
	if err := json.Unmarshal(r.Result, &edits); err != nil {
		t.Fatal(err)
	}
	if len(edits) != 1 || edits[0].NewText != want || !strings.HasPrefix(want, "<fragment>\n  <text id=\"a\">") {
		t.Errorf("edits = %+v, want one edit with fmt's output %q", edits, want)
	}
	_ = c.shutdown()
}
