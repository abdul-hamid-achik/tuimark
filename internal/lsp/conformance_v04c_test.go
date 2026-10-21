package lsp

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// SPEC v0.4c §15.12 "Props and the slot" (decision 181), test 249's
// language-server part (phase C3): completion of a <use>'s props and of
// {$name} in a template's body, `slot` as a tag name only in a template's
// body, hover on a {$name}, a prop attribute, and <slot/>, definition from
// either to the name in the template's props, and the diagnostics, which
// stay validate's.

// t249App is test 249's app.tui.
const t249App = "<tui>\n" +
	"  <template id=\"t\" props=\"title max\"><text>{$title}</text></template>\n" +
	"  <screen id=\"main\"><use template=\"t\" /></screen>\n" +
	"</tui>"

// 249, the second bullet, with its positions as written.
func TestV04cLSP249(t *testing.T) {
	dir := writeTree(t, nil)
	c := startServer(t, Options{})
	c.initialize()
	uri := c.open(filepath.Join(dir, "app.tui"), "tuimark", t249App)
	for _, pos := range []Position{{2, 38}, {1, 45}} {
		got := completionLabels(t, c, uri, pos)
		if !got["title"] || !got["max"] {
			t.Errorf("completion at %+v: %v, want title and max among them", pos, keys(got))
		}
	}
	r := c.call("textDocument/definition", map[string]any{"textDocument": map[string]any{"uri": uri}, "position": Position{1, 47}})
	var locs []location
	if err := json.Unmarshal(r.Result, &locs); err != nil {
		t.Fatalf("%v: %s", err, r.Result)
	}
	want := location{URI: uri, Range: Range{Start: Position{1, 26}, End: Position{1, 31}}}
	if len(locs) != 1 || locs[0] != want {
		t.Errorf("definition = %+v, want %+v", locs, want)
	}
	_ = c.shutdown()
}

// The completion items around 249's: a prop the use writes, before or
// after the cursor, is not offered again; the props come after the use's
// own attributes; {$ in an attribute value of a template's body offers the
// props too; nothing outside a template, in a pinned document, or on a use
// whose template is unknown; `slot` only in a template's body.
func TestV04cLSPCompletion(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"parts.tui": "<fragment>\n  <template id=\"far\" props=\"label width\"><text>{$label}</text></template>\n</fragment>\n",
	})
	app := filepath.Join(dir, "app.tui")
	c := startServer(t, Options{})
	c.initialize()
	const tmpl = "<tui>\n  <template id=\"t\" props=\"title max\"><box><text>{$title}</text></box></template>\n"
	for _, tc := range []completionCase{
		{name: "props on a use", text: tmpl + `  <screen id="main"><use template="t" |/></screen></tui>`,
			want: []string{"title", "max", "with", "id"}},
		{name: "a written prop before the cursor", text: tmpl + `  <screen id="main"><use template="t" title="x" |/></screen></tui>`,
			want: []string{"max"}, absent: []string{"title", "template"}},
		{name: "a written prop after the cursor", text: tmpl + `  <screen id="main"><use template="t" | max="1"/></screen></tui>`,
			want: []string{"title"}, absent: []string{"max"}},
		{name: "the template after the cursor", text: tmpl + `  <screen id="main"><use | template="t"/></screen></tui>`,
			want: []string{"title", "max"}},
		{name: "a prefix", text: tmpl + `  <screen id="main"><use template="t" ti|/></screen></tui>`,
			want: []string{"title"}, absent: []string{"max"}},
		{name: "a template in an included fragment", text: "<tui>\n  <include src=\"parts.tui\"/>\n  <screen id=\"main\"><use template=\"far\" |/></screen></tui>",
			want: []string{"label", "width"}},
		{name: "an unknown template", text: tmpl + `  <screen id="main"><use template="zz" |/></screen></tui>`,
			want: []string{"with"}, absent: []string{"title", "max"}},
		{name: "a buffer that does not parse", text: tmpl + `  <screen id="main"><use template="t" |`,
			want: []string{"title", "max"}},
		{name: "{$ in text", text: "<tui>\n  <template id=\"t\" props=\"title max\"><box><text>a {$|</text></box></template>\n  <screen id=\"main\"/></tui>",
			want: []string{"title", "max"}},
		{name: "{$ with a prefix", text: "<tui>\n  <template id=\"t\" props=\"title max\"><box><text>{$ma|}</text></box></template>\n  <screen id=\"main\"/></tui>",
			want: []string{"max"}, absent: []string{"title"}},
		{name: "{$ in an attribute value", text: "<tui>\n  <template id=\"t\" props=\"title max\"><box title=\"{$|\"/></template>\n  <screen id=\"main\"/></tui>",
			want: []string{"title", "max"}},
		{name: "{$ in a use's content in a template's body", text: "<tui>\n  <template id=\"t\" props=\"title max\"><use template=\"w\"><text>{$|</text></use></template>\n  <template id=\"w\"><box><slot/></box></template>\n  <screen id=\"main\"/></tui>",
			want: []string{"title", "max"}},
		{name: "{$ outside a template", text: "<tui>\n  <template id=\"t\" props=\"title max\"><text>x</text></template>\n  <screen id=\"main\"><text>{$|</text></screen></tui>",
			absent: []string{"title", "max"}},
		{name: "{$ in the template's own attributes", text: "<tui>\n  <template id=\"t\" props=\"title max\" x=\"{$|\"><text>x</text></template>\n  <screen id=\"main\"/></tui>",
			absent: []string{"title", "max"}},
		{name: "{$ in a pinned document", text: "<tui version=\"3\">\n  <template id=\"t\" props=\"title max\"><text>{$|</text></template>\n  <screen id=\"main\"/></tui>",
			absent: []string{"title", "max"}},
		{name: "slot in a template's body", text: "<tui>\n  <template id=\"t\"><box><sl|",
			want: []string{"slot"}},
		{name: "no slot in a screen", text: "<tui>\n  <screen id=\"main\"><sl|",
			absent: []string{"slot"}},
		{name: "no slot at the top of a fragment", text: "<fragment>\n  <sl|",
			absent: []string{"slot"}},
		{name: "slot in a fragment's template", text: "<fragment>\n  <template id=\"t\"><sl|",
			want: []string{"slot"}},
		{name: "no slot after a template closes", text: "<tui>\n  <template id=\"t\"><box/></template>\n  <sl|",
			absent: []string{"slot"}},
	} {
		text := strings.Replace(tc.text, "|", "", 1)
		uri := c.open(app, "tuimark", text)
		c.change(uri, 2, text) // the same document again, for a repeated path
		got := completionLabels(t, c, uri, positionAt(text, strings.IndexByte(tc.text, '|')))
		for _, w := range tc.want {
			if !got[w] {
				t.Errorf("%s: %v, missing %q", tc.name, keys(got), w)
			}
		}
		for _, a := range tc.absent {
			if got[a] {
				t.Errorf("%s: %q offered", tc.name, a)
			}
		}
	}
	_ = c.shutdown()
}

// The props come after the use's own attributes, with their template in
// the detail.
func TestV04cLSPCompletionOrder(t *testing.T) {
	dir := writeTree(t, nil)
	c := startServer(t, Options{})
	c.initialize()
	uri := c.open(filepath.Join(dir, "app.tui"), "tuimark", t249App)
	r := c.call("textDocument/completion", map[string]any{"textDocument": map[string]any{"uri": uri}, "position": Position{2, 38}})
	var list completionList
	if err := json.Unmarshal(r.Result, &list); err != nil {
		t.Fatal(err)
	}
	var labels []string
	for _, it := range list.Items {
		labels = append(labels, it.Label)
	}
	n := len(labels)
	if n < 3 || labels[n-2] != "title" || labels[n-1] != "max" {
		t.Fatalf("items = %v, want the props last, in their order", labels)
	}
	if it := list.Items[n-1]; it.Detail != `prop of <template id="t">` || it.TextEdit.NewText != `max="$1"` || it.SortText <= list.Items[0].SortText {
		t.Errorf("max = %+v", it)
	}
	_ = c.shutdown()
}

// hoverAt is the hover text at pos, or "".
func hoverAt(t *testing.T, c *client, uri string, pos Position) string {
	t.Helper()
	r := c.call("textDocument/hover", map[string]any{"textDocument": map[string]any{"uri": uri}, "position": pos})
	if string(r.Result) == "null" {
		return ""
	}
	var h hoverResult
	if err := json.Unmarshal(r.Result, &h); err != nil {
		t.Fatal(err)
	}
	return h.Contents.Value
}

func TestV04cLSPHover(t *testing.T) {
	dir := writeTree(t, nil)
	c := startServer(t, Options{})
	c.initialize()
	text := "<tui>\n" +
		"  <template id=\"t\" props=\"title max\"><box title=\"{$title}\"><text>{$max}</text><slot/></box></template>\n" +
		"  <screen id=\"main\"><use template=\"t\" title=\"a\" max=\"b\"/></screen>\n" +
		"</tui>"
	uri := c.open(filepath.Join(dir, "app.tui"), "tuimark", text)
	at := func(mark string, skip int) Position {
		i := strings.Index(text, mark)
		if i < 0 {
			t.Fatalf("no %q", mark)
		}
		return positionAt(text, i+skip)
	}
	for _, tc := range []struct {
		name string
		pos  Position
		want string
	}{
		{"{$name} in text", at("{$max}", 3), "`{$max}` — prop of <template id=\"t\">\n\nProps: `title`, `max`"},
		{"{$name} in a value", at("{$title}", 2), "`{$title}` — prop of <template id=\"t\">\n\nProps: `title`, `max`"},
		{"a prop of a use", at(`title="a"`, 1), "`title` — prop of <template id=\"t\">\n\nProps: `title`, `max`"},
		{"<slot/>", at("<slot/>", 2), "`<slot>` — tag (a <tui> without version)\n\nAttributes: none"},
	} {
		if got := hoverAt(t, c, uri, tc.pos); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
	// A {$name} the template does not declare names no template.
	other := strings.Replace(text, "{$max}", "{$zzz}", 1)
	c.change(uri, 2, other)
	if got := hoverAt(t, c, uri, positionAt(other, strings.Index(other, "{$zzz}")+3)); strings.Contains(got, "prop of") {
		t.Errorf("undeclared: %q", got)
	}
	c.change(uri, 3, text)
	// The use's own attribute keeps the catalog's text.
	if got := hoverAt(t, c, uri, at(`template="t" title`, 1)); !strings.Contains(got, "`template` — attribute of `<use>`") {
		t.Errorf("template: %q", got)
	}
	_ = c.shutdown()
}

func TestV04cLSPDefinition(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"parts.tui": "<fragment>\n  <template id=\"far\" props=\"label width\"><text>{$label}</text></template>\n</fragment>\n",
	})
	c := startServer(t, Options{})
	c.initialize()
	text := "<tui>\n" +
		"  <include src=\"parts.tui\"/>\n" +
		"  <template id=\"t\" props=\"title max\"><box title=\"{$max}\"><use template=\"far\" label=\"{$title}\"/></box></template>\n" +
		"  <screen id=\"main\"><use id=\"u\" max=\"1\" template=\"t\" title=\"a\"/></screen>\n" +
		"</tui>"
	uri := c.open(filepath.Join(dir, "app.tui"), "tuimark", text)
	parts := pathToURI(filepath.Join(dir, "parts.tui"))
	rng := func(line, char, n int) Range {
		return Range{Start: Position{line, char}, End: Position{line, char + n}}
	}
	for _, tc := range []struct {
		name, mark string
		skip       int
		want       []location
	}{
		{"{$name} in a value", `{$max}`, 3, []location{{URI: uri, Range: rng(2, 32, 3)}}},
		{"{$name} in a use's prop", `{$title}`, 2, []location{{URI: uri, Range: rng(2, 26, 5)}}},
		{"a prop attribute", `title="a"`, 2, []location{{URI: uri, Range: rng(2, 26, 5)}}},
		{"a prop before template=", `max="1"`, 0, []location{{URI: uri, Range: rng(2, 32, 3)}}},
		{"a prop of a template in a fragment", `label="{$title}"`, 3, []location{{URI: parts, Range: rng(1, 28, 5)}}},
		{"template= still names the template", `template="t"`, 10, []location{{URI: uri, Range: rng(2, 2, 9)}}},
		{"a use's own attribute", `id="u"`, 1, nil},
	} {
		got := definitionAt(t, c, uri, text, tc.mark, tc.skip)
		if len(got) != len(tc.want) || len(got) == 1 && got[0] != tc.want[0] {
			t.Errorf("%s: %+v, want %+v", tc.name, got, tc.want)
		}
	}
	_ = c.shutdown()
}

// A pinned document has no props: {$name} and an attribute of a <use>
// resolve to nothing, and hover is the catalog's.
func TestV04cLSPPinned(t *testing.T) {
	dir := writeTree(t, nil)
	c := startServer(t, Options{})
	c.initialize()
	text := strings.Replace(t249App, "<tui>", `<tui version="3">`, 1)
	uri := c.open(filepath.Join(dir, "app.tui"), "tuimark", text)
	if got := definitionAt(t, c, uri, text, "{$title}", 3); got != nil {
		t.Errorf("definition: %+v", got)
	}
	if got := hoverAt(t, c, uri, positionAt(text, strings.Index(text, "{$title}")+3)); strings.Contains(got, "prop of") {
		t.Errorf("hover: %q", got)
	}
	_ = c.shutdown()
}

// The diagnostics are validate's: a document with props and a slot, and a
// fragment checked on its own whose template's slot comes from an include
// (test 263), through the same LoadBuffers.
func TestV04cLSPDiagnostics(t *testing.T) {
	for _, f := range []string{"235/app.tui", "235/fill.tui", "243/app.tui", "246/app.tui", "256/app.tui"} {
		p, err := filepath.Abs(filepath.Join("..", "..", "testdata", "v04c", f))
		if err != nil {
			t.Fatal(err)
		}
		sn := &snapshot{open: map[string]string{}, byURI: map[string]*document{}}
		got := diagKeys(analyze(sn, p, mustRead(t, p)).diags)
		if want := validateReference(t, p); strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Errorf("%s:\nanalyze  %q\nvalidate %q", f, got, want)
		}
	}
	v04c := func(rel string) string {
		return mustRead(t, filepath.Join("..", "..", "testdata", "v04c", rel))
	}
	dir := writeTree(t, map[string]string{
		"card-slot.tui": v04c("263/card-slot.tui"),
		"kit2.tui":      v04c("263/kit2.tui"),
	})
	c := startServer(t, Options{})
	c.initialize()
	for _, tc := range []struct {
		file string
		want []string
	}{
		{"kit.tui", []string{"V002 7:10", "V001 8:4"}},
		{"kit2.tui", []string{"V013 7:4", "V002 7:10", "V001 8:4"}},
	} {
		uri := c.open(filepath.Join(dir, tc.file), "tuimark", v04c("263/"+tc.file))
		if got := codesAt(c.nextPublish(uri).Diagnostics); strings.Join(got, " ") != strings.Join(tc.want, " ") {
			t.Errorf("%s: %v, want %v", tc.file, got, tc.want)
		}
	}
	_ = c.shutdown()
}
