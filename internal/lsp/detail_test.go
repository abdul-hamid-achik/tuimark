package lsp

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// --- positions ---------------------------------------------------------------

func TestUTF16Conversion(t *testing.T) {
	// "a日😀b": columns 1..4 in code points; UTF-16 offsets 0, 1, 2, 4, 5.
	lines := splitLines("a日😀b\nsecond")
	for _, tc := range []struct {
		line, col int
		want      Position
	}{
		{0, 0, Position{0, 0}}, // no position: the start of the file
		{0, 7, Position{0, 0}},
		{1, 1, Position{0, 0}},
		{1, 2, Position{0, 1}},
		{1, 3, Position{0, 2}},
		{1, 4, Position{0, 4}}, // after the emoji: two units
		{1, 5, Position{0, 5}}, // past the last character
		{1, 99, Position{0, 5}},
		{2, 3, Position{1, 2}},
		{9, 1, Position{1, 6}}, // past the last line: its end
	} {
		if got := fromDiag(lines, tc.line, tc.col); got != tc.want {
			t.Errorf("fromDiag(%d, %d) = %+v, want %+v", tc.line, tc.col, got, tc.want)
		}
	}
	// The standard library agrees on how long a rune is.
	for _, r := range "a é 日 😀 \U0010FFFF" {
		if want := len(utf16.Encode([]rune{r})); runeUnits(r) != want {
			t.Errorf("runeUnits(%q) = %d, want %d", r, runeUnits(r), want)
		}
	}
}

func TestByteOffsetAndPositionAt(t *testing.T) {
	text := "a日😀b\nsecond\n"
	for _, tc := range []struct {
		pos Position
		off int
	}{
		{Position{0, 0}, 0},
		{Position{0, 1}, 1},
		{Position{0, 2}, 4},
		{Position{0, 3}, 4}, // inside the surrogate pair: rounds down to its start
		{Position{0, 4}, 8},
		{Position{0, 5}, 9},
		{Position{0, 99}, 9}, // clamped to the line
		{Position{1, 2}, 12},
		{Position{2, 0}, 17},
		{Position{9, 0}, len(text)},
	} {
		if got := byteOffset(text, tc.pos); got != tc.off {
			t.Errorf("byteOffset(%+v) = %d, want %d", tc.pos, got, tc.off)
		}
	}
	for _, tc := range []struct {
		off  int
		want Position
	}{{0, Position{0, 0}}, {8, Position{0, 4}}, {9, Position{0, 5}}, {10, Position{1, 0}}, {len(text), Position{2, 0}}, {999, Position{2, 0}}} {
		if got := positionAt(text, tc.off); got != tc.want {
			t.Errorf("positionAt(%d) = %+v, want %+v", tc.off, got, tc.want)
		}
	}
	if got := endPosition("ab\n😀"); got != (Position{1, 2}) {
		t.Errorf("endPosition = %+v", got)
	}
}

func TestTokenEnd(t *testing.T) {
	for _, tc := range []struct {
		line  string
		start int
		want  int
	}{
		{`  <bogus/>`, 2, 8},                 // <bogus: the tag name
		{`<col  title="x">`, 0, 4},           // <col
		{`<col title="x" foo="y">`, 15, 18},  // foo
		{`screen { bogus-prop: 1; }`, 9, 19}, // the property, not its ':'
		{`#a:focus { }`, 0, 8},               // a selector keeps its pseudo-class
		{`日本😀<x/>`, 4, 6},                    // UTF-16 units: the emoji is two
		{`  `, 1, 2},                         // nothing there: the end of the line
		{`<a>`, 3, 3},
	} {
		if got := tokenEnd(tc.line, tc.start); got != tc.want {
			t.Errorf("tokenEnd(%q, %d) = %d, want %d", tc.line, tc.start, got, tc.want)
		}
	}
}

func TestConvertDiagSeverityAndNoLine(t *testing.T) {
	lines := splitLines("ab\ncd")
	w := convertDiag(lines, ir.Diagnostic{Severity: ir.Warning, Code: "B003", Msg: "m", Line: 2, Col: 2})
	if w.Severity != 2 || w.Range.Start != (Position{1, 1}) || w.Range.End != (Position{1, 2}) {
		t.Errorf("warning = %+v", w)
	}
	e := convertDiag(lines, ir.Diagnostic{Severity: ir.Error, Code: "V006", Msg: "m"})
	if e.Severity != 1 || e.Range != (Range{}) {
		t.Errorf("no line: %+v, want the start of the file", e)
	}
}

func TestDedupe(t *testing.T) {
	at := func(l, c int) Range { return Range{Start: Position{l, c}, End: Position{l, c + 3}} }
	in := []lspDiagnostic{
		{Code: "V003", Message: "a", Range: at(3, 1)},
		{Code: "V003", Message: "a", Range: at(3, 1)},                                          // the same
		{Code: "V003", Message: "a", Range: Range{Start: Position{3, 1}, End: Position{3, 9}}}, // same start: the same
		{Code: "V003", Message: "b", Range: at(3, 1)},                                          // another message
		{Code: "V004", Message: "a", Range: at(3, 1)},                                          // another code
		{Code: "V003", Message: "a", Range: at(1, 1)},                                          // another place
	}
	got := dedupe(in)
	var codes []string
	for _, d := range got {
		codes = append(codes, d.Code+d.Message+string(rune('0'+d.Range.Start.Line)))
	}
	want := []string{"V003a1", "V003a3", "V003b3", "V004a3"}
	if !reflect.DeepEqual(codes, want) {
		t.Errorf("dedupe = %v, want %v", codes, want)
	}
}

// --- lexers --------------------------------------------------------------------

func TestLexTUI(t *testing.T) {
	type want struct {
		kind         tuiKind
		tag, attr    string
		prefix       string
		stackNames   string
		presentNames string
	}
	for _, tc := range []struct {
		name string
		src  string // the cursor is at |
		want want
	}{
		{"tag name", `<tui><li|`, want{kind: tuiTagName, prefix: "li", stackNames: "tui"}},
		{"bare <", `<tui>|`, want{kind: tuiText, stackNames: "tui"}},
		{"after <", `<tui><|`, want{kind: tuiTagName, stackNames: "tui"}},
		{"attribute name", `<box bo|`, want{kind: tuiAttrName, tag: "box", prefix: "bo"}},
		{"after a space", `<box id="a" |`, want{kind: tuiAttrName, tag: "box", presentNames: "id"}},
		{"value", `<box border="si|`, want{kind: tuiAttrValue, tag: "box", attr: "border", prefix: "si", presentNames: "border"}},
		{"single quotes", `<box border='|`, want{kind: tuiAttrValue, tag: "box", attr: "border", presentNames: "border"}},
		{"closed value, next attribute", `<box border="single" pa|`, want{kind: tuiAttrName, tag: "box", prefix: "pa", presentNames: "border"}},
		{"text", `<tui><text>hello |`, want{kind: tuiText, prefix: "hello ", stackNames: "tui text"}},
		{"after a closing tag", `<tui><text>a</text>|`, want{kind: tuiText, stackNames: "tui"}},
		{"self-closing", `<tui><spacer/>|`, want{kind: tuiText, stackNames: "tui"}},
		{"inside a comment", `<tui><!-- <bo|`, want{kind: tuiNone, stackNames: "tui"}},
		{"after a comment", `<tui><!-- <x> -->|`, want{kind: tuiText, stackNames: "tui"}},
		{"CDATA", `<tui><![CDATA[ <b|`, want{kind: tuiNone, stackNames: "tui"}},
		{"closing tag", `<tui></tu|`, want{kind: tuiNone, stackNames: "tui"}},
		{"xml declaration", `<?xml version="1.0"?><tui>|`, want{kind: tuiText, stackNames: "tui"}},
		{"unfinished tag then another", "<tui><li\n  <box bo|", want{kind: tuiAttrName, tag: "box", prefix: "bo", stackNames: "tui"}},
		{"> inside a value", `<text title="a>b" |`, want{kind: tuiAttrName, tag: "text", presentNames: "title"}},
	} {
		off := strings.IndexByte(tc.src, '|')
		ctx := lexTUI(strings.Replace(tc.src, "|", "", 1), off)
		var stack []string
		for _, o := range ctx.stack {
			stack = append(stack, o.name)
		}
		var present []string
		for k := range ctx.present {
			present = append(present, k)
		}
		sort.Strings(present)
		got := want{ctx.kind, ctx.tag, ctx.attr, ctx.prefix, strings.Join(stack, " "), strings.Join(present, " ")}
		if got != tc.want {
			t.Errorf("%s: %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestLexTCSS(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want cssCtx
	}{
		{"selector", `scr|`, cssCtx{kind: cssSelector, prefix: "scr"}},
		{"after a rule", "a { color: red; }\nli|", cssCtx{kind: cssSelector, prefix: "\nli"}},
		{"property", "a {\n  col|", cssCtx{kind: cssProperty, prefix: "col"}},
		{"after a declaration", "a { color: red; bo|", cssCtx{kind: cssProperty, prefix: "bo"}},
		{"value", "a { border: si|", cssCtx{kind: cssValue, prop: "border", prefix: " si"}},
		{"token value", "a { color: $a|", cssCtx{kind: cssValue, prop: "color", prefix: " $a"}},
		{"root", ":root { --a: re|", cssCtx{kind: cssValue, prop: "--a", prefix: " re", root: true}},
		{"inside a media block", "@media (max-cols: 80) { li|", cssCtx{kind: cssSelector, prefix: " li"}},
		{"media rule property", "@media (max-cols: 80) { a { wr|", cssCtx{kind: cssProperty, prefix: "wr"}},
		{"media feature", "@media (ma|", cssCtx{kind: cssMedia, prefix: "ma"}},
		{"media value", "@media (theme: da|", cssCtx{kind: cssMedia, prop: "theme", prefix: "da"}},
		{"comment", "a { /* col|", cssCtx{kind: cssNone}},
		{"after a comment", "a { /* x */ col|", cssCtx{kind: cssProperty, prefix: "/* x */ col"}},
		{"string", `a { content: "x|`, cssCtx{kind: cssNone}},
	} {
		off := strings.IndexByte(tc.src, '|')
		got := lexTCSS(strings.Replace(tc.src, "|", "", 1), off)
		if got != tc.want {
			t.Errorf("%s: %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

// --- debounce -------------------------------------------------------------------

func TestDebounceTwoChangesPublishOnce(t *testing.T) {
	fc := &fakeClock{}
	srv := newServer(Options{})
	srv.afterFunc = fc.afterFunc
	c := startWith(t, srv)
	c.initialize()
	dir := writeTree(t, nil)
	good := `<tui version="1"><screen id="s"><text>x</text></screen></tui>` + "\n"
	bad := `<tui version="1"><screen id="s"><text>x</text><bogus/></screen></tui>` + "\n"
	uri := c.open(filepath.Join(dir, "a.tui"), "tuimark", good)
	sync := func() {
		c.call("textDocument/hover", map[string]any{"textDocument": map[string]any{"uri": uri}, "position": Position{}})
	}
	sync()
	if fc.pending() != 1 || fc.delays[0] != DefaultDebounce {
		t.Fatalf("after open: %d timers, delays %v; want one of %v", fc.pending(), fc.delays, DefaultDebounce)
	}
	if n := fc.fire(); n != 1 {
		t.Fatalf("fired %d", n)
	}
	if p := c.nextPublish(uri); len(p.Diagnostics) != 0 {
		t.Fatalf("open: %+v", p.Diagnostics)
	}

	// Two changes inside the window: one timer, one publish, the last text.
	c.change(uri, 2, "<tui version=\"1\"><screen id=\"s\"/></tui>\n")
	c.change(uri, 3, bad)
	sync()
	if fc.pending() != 1 {
		t.Fatalf("two changes queued %d timers, want 1", fc.pending())
	}
	fc.fire()
	sync()
	count := 0
	var last published
	for _, m := range c.notes {
		if m.Method == "textDocument/publishDiagnostics" && decodePublished(t, m).URI == uri {
			count++
			last = decodePublished(t, m)
		}
	}
	if count != 1 || len(last.Diagnostics) != 1 || last.Diagnostics[0].Code != "V001" || *last.Version != 3 {
		t.Errorf("%d publishes, last %+v; want one, with the third text's V001", count, last)
	}

	// After it fired, the next change starts a new window.
	c.change(uri, 4, good)
	sync()
	if fc.pending() != 1 {
		t.Errorf("a change after the window queued %d timers, want 1", fc.pending())
	}
	// A stale version is dropped.
	c.change(uri, 1, bad)
	sync()
	if fc.pending() != 1 {
		t.Errorf("a stale change queued another timer")
	}
	// Shutdown cancels what is pending: firing it later sends nothing.
	if code := c.shutdown(); code != 0 {
		t.Errorf("exit %d", code)
	}
	fc.fire()
}

func TestStylesheetChangeSchedulesTheDocumentsThatLoadIt(t *testing.T) {
	fc := &fakeClock{}
	srv := newServer(Options{})
	srv.afterFunc = fc.afterFunc
	c := startWith(t, srv)
	c.initialize()
	dir := writeTree(t, map[string]string{"t.tcss": "screen { color: red; }\n"})
	docURI := c.open(filepath.Join(dir, "a.tui"), "tuimark", `<tui version="1"><style src="t.tcss"/><screen id="s"><text>x</text></screen></tui>`+"\n")
	c.call("textDocument/hover", map[string]any{"textDocument": map[string]any{"uri": docURI}, "position": Position{}})
	fc.fire() // publishes the document; the stylesheet it loads is scheduled
	fc.fire() // and published in its own window
	sheetURI := pathToURI(filepath.Join(dir, "t.tcss"))
	c.nextPublish(docURI)
	c.nextPublish(sheetURI)

	// A change in the open stylesheet moves its own diagnostics and the
	// document's (a removed :root token, say).
	c.open(filepath.Join(dir, "t.tcss"), "tcss", "screen { color: red; }\n")
	c.call("textDocument/hover", map[string]any{"textDocument": map[string]any{"uri": docURI}, "position": Position{}})
	if n := fc.pending(); n != 2 {
		t.Fatalf("%d timers after the stylesheet opened, want 2 (itself and the document)", n)
	}
}

// --- completion in more places --------------------------------------------------

type completionCase struct {
	name   string
	path   string
	lang   string
	text   string // the cursor is at |
	want   []string
	absent []string
}

func TestCompletionContexts(t *testing.T) {
	const sample = `{"title": "x", "tickets": [{"id": 1, "subject": "s"}], "user": {"name": "n"}}`
	dir := writeTree(t, map[string]string{"sample.json": sample})
	app := filepath.Join(dir, "app.tui")
	c := startServer(t, Options{})
	c.initialize()
	for _, tc := range []completionCase{
		{name: "attribute names", path: app, lang: "tuimark", text: `<tui version="3"><screen id="s"><box |`,
			want: []string{"border", "pad", "on:click", "each"}, absent: []string{"stick", "label"}},
		{name: "attributes are filtered by version", path: app, lang: "tuimark", text: `<tui version="1"><screen id="s"><box |`,
			want: []string{"border", "pad"}, absent: []string{"each", "key"}},
		{name: "a written attribute is not offered again", path: app, lang: "tuimark", text: `<tui version="1"><screen id="s"><box border="single" bo|`,
			absent: []string{"border"}},
		{name: "tags are filtered by version", path: app, lang: "tuimark", text: `<tui version="1"><screen id="s"><t|`,
			want: []string{"text"}, absent: []string{"table", "tabs", "tab"}},
		{name: "tags of version 2", path: app, lang: "tuimark", text: `<tui version="2"><screen id="s"><t|`,
			want: []string{"text", "table", "tabs", "tab"}},
		{name: "to targets", path: app, lang: "tuimark", text: `<tui version="1"><keymap><bind keys="x" action="focus" to="|`,
			want: []string{}},
		{name: "ids", path: app, lang: "tuimark", text: `<tui version="1"><screen id="main"><list id="rows"/><keymap><bind keys="x" action="focus" to="|`,
			want: []string{"#main", "#rows"}},
		{name: "when after the #", path: app, lang: "tuimark", text: `<tui version="1"><screen id="main"><list id="rows"/><keymap><bind keys="x" action="quit" when="#ro|`,
			want: []string{"#rows"}, absent: []string{"#main"}},
		{name: "actions", path: app, lang: "tuimark", text: `<tui version="3"><screen id="s"><list id="l" on:select="open"/><keymap><bind keys="x" action="|`,
			want: []string{"focus", "quit", "move-next", "check-toggle", "open"}},
		{name: "on: actions leave out the hyphenated built-ins", path: app, lang: "tuimark", text: `<tui version="3"><screen id="s"><list id="l" on:select="|`,
			want: []string{"focus", "quit"}, absent: []string{"move-next", "switch-to"}},
		{name: "key tokens", path: app, lang: "tuimark", text: `<tui version="1"><keymap><bind keys="q,ent|`,
			want: []string{"enter"}, absent: []string{"esc"}},
		{name: "key tokens after a comma", path: app, lang: "tuimark", text: `<tui version="1"><keymap><bind keys="q, ctrl+|`,
			want: []string{"ctrl+a", "ctrl+z"}, absent: []string{"ctrl+i", "ctrl+j", "ctrl+m", "enter"}},
		{name: "{path} in text", path: app, lang: "tuimark", text: `<tui version="1"><screen id="s"><text>hi {us|`,
			want: []string{"user", "user.name"}, absent: []string{"title"}},
		{name: "no path outside braces", path: app, lang: "tuimark", text: `<tui version="1"><screen id="s"><text>hi us|`,
			want: []string{}},
		{name: "alias fields", path: app, lang: "tuimark", text: `<tui version="1"><screen id="s"><list id="l" each="tickets as item" key="item.id"><item><text>{item.|`,
			want: []string{"item.id", "item.subject"}},
		{name: "each offers arrays", path: app, lang: "tuimark", text: `<tui version="1"><screen id="s"><list id="l" each="|`,
			want: []string{"tickets"}, absent: []string{"title", "user"}},
		{name: "bind and if paths", path: app, lang: "tuimark", text: `<tui version="1"><screen id="s"><text if="!us|`,
			want: []string{"user", "user.name"}},
		{name: "title paths", path: app, lang: "tuimark", text: `<tui version="1"><screen id="s" title="{t|`,
			want: []string{"title", "tickets"}},
		{name: "tui theme and version", path: app, lang: "tuimark", text: `<tui theme="|`,
			want: []string{"dark", "light"}},
	} {
		text := strings.Replace(tc.text, "|", "", 1)
		uri := c.open(tc.path, tc.lang, text)
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
		if len(tc.want) == 0 && len(tc.absent) == 0 && len(got) != 0 {
			t.Errorf("%s: %v, want nothing", tc.name, keys(got))
		}
	}
}

func TestCompletionStylesheets(t *testing.T) {
	const app = `<tui version="3">
  <style src="t.tcss"/>
  <screen id="main">
    <list id="rows" class="muted" each="tickets as item" key="item.id"><item><text>{item.subject}</text></item></list>
  </screen>
</tui>
`
	dir := writeTree(t, map[string]string{"app.tui": app, "sample.json": `{"tickets": []}`})
	sheet := filepath.Join(dir, "t.tcss")
	c := startServer(t, Options{})
	c.initialize()
	for _, tc := range []completionCase{
		{name: "css property", path: sheet, lang: "tcss", text: "screen {\n  wr|",
			want: []string{"wrap"}, absent: []string{"border"}},
		{name: "css property of version 3 needs a document", path: sheet, lang: "tcss", text: "screen {\n  row-g|",
			want: []string{"row-gap"}},
		{name: "css keyword", path: sheet, lang: "tcss", text: "screen { layout: |",
			want: []string{"row", "column", "grid"}},
		{name: "css keyword prefix", path: sheet, lang: "tcss", text: "screen { border: ro|",
			want: []string{"rounded"}, absent: []string{"single"}},
		{name: "css token", path: sheet, lang: "tcss", text: ":root { --mine: red; }\nscreen { color: $|",
			want: []string{"mine", "fg", "bg"}},
		{name: "css pseudo-class", path: sheet, lang: "tcss", text: "list:|",
			want: []string{"focus", "selected", "root"}},
		{name: "ids of the documents that load it", path: sheet, lang: "tcss", text: "#|",
			want: []string{"main", "rows"}},
		{name: "css classes", path: sheet, lang: "tcss", text: ".|",
			want: []string{"muted"}},
		{name: "css tags", path: sheet, lang: "tcss", text: "li|",
			want: []string{"list"}, absent: []string{"screen"}},
		{name: "css media feature", path: sheet, lang: "tcss", text: "@media (m|",
			want: []string{"max-cols", "min-rows"}},
		{name: "css media theme values", path: sheet, lang: "tcss", text: "@media (theme: |",
			want: []string{"dark", "light"}},
	} {
		text := strings.Replace(tc.text, "|", "", 1)
		uri := c.open(tc.path, tc.lang, text)
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
}

func TestCompletionStylesheetVersionIsTheNewestOfItsDocuments(t *testing.T) {
	// A stylesheet no document loads is offered everything; one loaded only
	// by a version="1" document, only version 1; a version="2" one adds its own.
	v := func(n string) string {
		return `<tui version="` + n + `"><style src="t.tcss"/><screen id="s"><text>x</text></screen></tui>` + "\n"
	}
	for _, tc := range []struct {
		docs       map[string]string
		wantGrid   bool
		wantRowGap bool
	}{
		{map[string]string{}, true, true},
		{map[string]string{"a.tui": v("1")}, false, false},
		{map[string]string{"a.tui": v("1"), "b.tui": v("2")}, true, false},
		{map[string]string{"a.tui": v("2"), "b.tui": v("3")}, true, true},
	} {
		dir := writeTree(t, tc.docs)
		c := startServer(t, Options{})
		c.initialize()
		uri := c.open(filepath.Join(dir, "t.tcss"), "tcss", "screen { row-gap: 1; layout: ")
		got := completionLabels(t, c, uri, Position{0, 29})
		if got["grid"] != tc.wantGrid {
			t.Errorf("%d documents: grid offered = %v", len(tc.docs), got["grid"])
		}
		uri = c.open(filepath.Join(dir, "t.tcss"), "tcss", "screen { row-")
		got = completionLabels(t, c, uri, Position{0, 13})
		if got["row-gap"] != tc.wantRowGap {
			t.Errorf("%d documents: row-gap offered = %v", len(tc.docs), got["row-gap"])
		}
	}
}

func TestCompletionOfClassesFromDocuments(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"app.tui": `<tui version="2"><style src="t.tcss"/><screen id="s"><text class="muted big" class:warn="w">x</text></screen></tui>` + "\n",
	})
	c := startServer(t, Options{})
	c.initialize()
	sheet := filepath.Join(dir, "t.tcss")
	uri := c.open(sheet, "tcss", ".")
	got := completionLabels(t, c, uri, Position{0, 1})
	for _, w := range []string{"muted", "big", "warn"} {
		if !got[w] {
			t.Errorf("%v, missing %q", keys(got), w)
		}
	}
	// And the other way: a .tui's class="" offers what the stylesheet selects.
	app := filepath.Join(dir, "app2.tui")
	c.open(sheet, "tcss", ".fancy { bold: true; }\n")
	const text = `<tui version="2"><style src="t.tcss"/><screen id="s"><text class="|`
	u2 := c.open(app, "tuimark", strings.Replace(text, "|", "", 1))
	got = completionLabels(t, c, u2, positionAt(strings.Replace(text, "|", "", 1), strings.IndexByte(text, '|')))
	if !got["fancy"] {
		t.Errorf("class values: %v, missing fancy", keys(got))
	}
}

func TestCompletionTextEditReplacesTheTypedPrefix(t *testing.T) {
	dir := writeTree(t, nil)
	c := startServer(t, Options{})
	c.initialize()
	text := `<tui version="1"><screen id="main"><list id="rows"/><keymap><bind keys="x" action="quit" when="#ro`
	uri := c.open(filepath.Join(dir, "a.tui"), "tuimark", text)
	r := c.call("textDocument/completion", map[string]any{"textDocument": map[string]any{"uri": uri}, "position": positionAt(text, len(text))})
	var list completionList
	if err := json.Unmarshal(r.Result, &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 1 || list.Items[0].TextEdit == nil {
		t.Fatalf("%+v", list.Items)
	}
	e := list.Items[0].TextEdit
	// The edit starts at the '#', so accepting "#rows" does not give "##rows".
	if e.NewText != "#rows" || e.Range.End != positionAt(text, len(text)) || e.Range.Start != positionAt(text, len(text)-3) {
		t.Errorf("edit = %+v", e)
	}
}

// --- hover, definition ------------------------------------------------------------

func TestHoverTUI(t *testing.T) {
	dir := writeTree(t, nil)
	c := startServer(t, Options{})
	c.initialize()
	text := "<tui version=\"3\">\n  <screen id=\"s\">\n    <box border=\"rounded\" pad=\"1\"/>\n    <bogus/>\n  </screen>\n</tui>\n"
	uri := c.open(filepath.Join(dir, "a.tui"), "tuimark", text)
	hover := func(l, ch int) string {
		r := c.call("textDocument/hover", map[string]any{"textDocument": map[string]any{"uri": uri}, "position": Position{l, ch}})
		if string(r.Result) == "null" {
			return ""
		}
		var h hoverResult
		if err := json.Unmarshal(r.Result, &h); err != nil {
			t.Fatal(err)
		}
		return h.Contents.Value
	}
	if h := hover(2, 6); !strings.Contains(h, "`<box>`") || !strings.Contains(h, "`border`") {
		t.Errorf("tag: %q", h)
	}
	if h := hover(2, 12); !strings.Contains(h, "`border` — attribute of `<box>`") || !strings.Contains(h, "`rounded`") {
		t.Errorf("attribute: %q", h)
	}
	if h := hover(2, 20); !strings.Contains(h, "`rounded` — value of `border`") {
		t.Errorf("value: %q", h)
	}
	if h := hover(3, 4); h != "" {
		t.Errorf("an unknown tag: %q", h)
	}
	// A code in the text, and a published diagnostic under the cursor.
	if h := hover(0, 0); h != "" {
		t.Errorf("the '<' alone: %q", h)
	}
	c.nextPublish(uri)
	if h := hover(3, 5); !strings.Contains(h, "V001") {
		t.Errorf("a diagnostic's code on the cursor: %q", h)
	}
}

func TestDefinitions(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"app.tui": `<tui version="2">
  <style src="t.tcss"/>
  <screen id="main">
    <list id="rows" class="a" each="xs as x" key="x.id"><item><text class="a">{x.n}</text></item></list>
    <box class="b" class:a="on"/>
    <keymap><bind keys="x" action="focus" to="#rows"/><bind keys="y" action="quit" when="#rows:focus"/></keymap>
  </screen>
</tui>
`,
		"t.tcss": ":root { --brand: red; }\nscreen { color: $brand; background: var(--brand); }\n.a { bold: true; }\n",
	})
	c := startServer(t, Options{})
	c.initialize()
	app := c.open(filepath.Join(dir, "app.tui"), "tuimark", mustRead(t, filepath.Join(dir, "app.tui")))
	sheet := c.open(filepath.Join(dir, "t.tcss"), "tcss", mustRead(t, filepath.Join(dir, "t.tcss")))
	def := func(uri string, l, ch int) []location {
		r := c.call("textDocument/definition", map[string]any{"textDocument": map[string]any{"uri": uri}, "position": Position{l, ch}})
		var locs []location
		if string(r.Result) != "null" {
			if err := json.Unmarshal(r.Result, &locs); err != nil {
				t.Fatalf("%v: %s", err, r.Result)
			}
		}
		return locs
	}
	appText, sheetText := mustRead(t, filepath.Join(dir, "app.tui")), mustRead(t, filepath.Join(dir, "t.tcss"))
	// at is the position delta characters into the first occurrence of needle.
	at := func(text, needle string, delta int) Position {
		return positionAt(text, strings.Index(text, needle)+delta)
	}
	rows := Range{Start: Position{3, 4}, End: Position{3, 9}}
	// `to`, and `when` with a pseudo-class, go to the element.
	for _, p := range []Position{at(appText, `to="#rows"`, 6), at(appText, `when="#rows:focus"`, 9), at(appText, `when="#rows:focus"`, 13)} {
		got := def(app, p.Line, p.Character)
		if len(got) != 1 || got[0].URI != app || got[0].Range != rows {
			t.Errorf("definition at %+v = %+v, want %+v", p, got, rows)
		}
	}
	// A class goes to every element that has it (written or guarded).
	got := def(sheet, 2, 2)
	var starts []Position
	for _, l := range got {
		if l.URI != app {
			t.Errorf("class location %+v", l)
		}
		starts = append(starts, l.Range.Start)
	}
	if want := []Position{{3, 4}, at(appText, `<text class="a">`, 0), {4, 4}}; !reflect.DeepEqual(starts, want) {
		t.Errorf(".a = %v, want %v", starts, want)
	}
	// <style src> goes to the file.
	if p := at(appText, `t.tcss"`, 2); true {
		if got := def(app, p.Line, p.Character); len(got) != 1 || got[0].URI != sheet || got[0].Range != (Range{}) {
			t.Errorf("style src = %+v", got)
		}
	}
	// $token and var(--token) go to the declaration.
	want := location{URI: sheet, Range: Range{Start: Position{0, 8}, End: Position{0, 15}}}
	for _, p := range []Position{at(sheetText, "$brand", 3), at(sheetText, "var(--brand)", 7)} {
		if got := def(sheet, p.Line, p.Character); len(got) != 1 || got[0] != want {
			t.Errorf("token at %+v = %+v, want %+v", p, got, want)
		}
	}
	// Nothing on a property name or a brace.
	if got := def(sheet, 1, 10); got != nil {
		t.Errorf("a property: %+v", got)
	}
	if got := def(app, 0, 2); got != nil {
		t.Errorf("a tag: %+v", got)
	}
}

func TestStylesheetDiagnosticsAreDedupedAcrossDocuments(t *testing.T) {
	const doc = `<tui version="1"><style src="t.tcss"/><screen id="s"><text>x</text></screen></tui>` + "\n"
	dir := writeTree(t, map[string]string{
		"a.tui": doc, "b.tui": doc, "t.tcss": "screen { bogus-prop: 1; }\n",
	})
	c := startServer(t, Options{})
	c.initialize()
	// a.tui is open, b.tui is only beside the stylesheet: both load it.
	c.open(filepath.Join(dir, "a.tui"), "tuimark", doc)
	p := c.nextPublish(pathToURI(filepath.Join(dir, "t.tcss")))
	if len(p.Diagnostics) != 1 {
		t.Errorf("%+v, want the one V003 once", p.Diagnostics)
	}
}

func TestStylesheetDiagnosticsFromTheSameNameInAnotherDirectory(t *testing.T) {
	// Two stylesheets called theme.tcss stay apart: routing is by full path.
	const doc = `<tui version="1"><style src="theme.tcss"/><screen id="s"><text>x</text></screen></tui>` + "\n"
	dir := writeTree(t, map[string]string{
		"x/a.tui": doc, "x/theme.tcss": "screen { bogus-prop: 1; }\n",
		"y/b.tui": doc, "y/theme.tcss": "screen { color: red; }\n",
	})
	c := startServer(t, Options{})
	c.initialize()
	c.open(filepath.Join(dir, "x", "a.tui"), "tuimark", doc)
	c.open(filepath.Join(dir, "y", "b.tui"), "tuimark", doc)
	if p := c.nextPublish(pathToURI(filepath.Join(dir, "x", "theme.tcss"))); len(p.Diagnostics) != 1 {
		t.Errorf("x/theme.tcss: %+v", p.Diagnostics)
	}
	if p := c.nextPublish(pathToURI(filepath.Join(dir, "y", "theme.tcss"))); len(p.Diagnostics) != 0 {
		t.Errorf("y/theme.tcss: %+v", p.Diagnostics)
	}
}

func TestAutoThemeIsCheckedUnderBoth(t *testing.T) {
	// theme="auto" (version 2) is validated under both themes, as validate does:
	// a token only the light palette lacks is still reported once.
	const doc = `<tui version="2" theme="auto"><style src="t.tcss"/><screen id="s"><text>x</text></screen></tui>` + "\n"
	dir := writeTree(t, map[string]string{"a.tui": doc, "t.tcss": "@media (theme: dark) { :root { --only-dark: red; } }\nscreen { color: $only-dark; }\n"})
	c := startServer(t, Options{})
	c.initialize()
	c.open(filepath.Join(dir, "a.tui"), "tuimark", doc)
	p := c.nextPublish(pathToURI(filepath.Join(dir, "t.tcss")))
	if len(p.Diagnostics) != 1 || p.Diagnostics[0].Code != "V003" || !strings.Contains(p.Diagnostics[0].Message, "only-dark") {
		t.Errorf("%+v, want one V003 about $only-dark (the light theme lacks it)", p.Diagnostics)
	}
}

func TestUntitledDocument(t *testing.T) {
	c := startServer(t, Options{})
	c.initialize()
	uri := "untitled:Untitled-1"
	c.notify("textDocument/didOpen", map[string]any{"textDocument": map[string]any{
		"uri": uri, "languageId": "tuimark", "version": 1, "text": `<tui version="1"><screen><bogus/></screen></tui>`,
	}})
	p := c.nextPublish(uri)
	if len(p.Diagnostics) == 0 {
		t.Errorf("%+v, want the document's diagnostics", p.Diagnostics)
	}
	// Completion and formatting work without a path.
	if got := completionLabels(t, c, uri, Position{0, 20}); len(got) == 0 {
		t.Errorf("no completion in an untitled buffer")
	}
	c.notify("textDocument/didClose", map[string]any{"textDocument": map[string]any{"uri": uri}})
	if p := c.nextPublish(uri); len(p.Diagnostics) != 0 {
		t.Errorf("after close: %+v, want an empty list", p.Diagnostics)
	}
	_ = c.shutdown()
}

func TestRequestsOnUnknownDocuments(t *testing.T) {
	c := startServer(t, Options{})
	c.initialize()
	for _, m := range []string{"textDocument/hover", "textDocument/definition", "textDocument/completion"} {
		r := c.call(m, map[string]any{"textDocument": map[string]any{"uri": "file:///nope.tui"}, "position": Position{}})
		if r.Error != nil || string(r.Result) != "null" {
			t.Errorf("%s: %+v %s", m, r.Error, r.Result)
		}
	}
	r := c.call("textDocument/formatting", map[string]any{"textDocument": map[string]any{"uri": "file:///nope.tui"}})
	if string(r.Result) != "[]" {
		t.Errorf("formatting: %s", r.Result)
	}
	if r := c.call("textDocument/hover", "not an object"); r.Error == nil || r.Error.Code != -32602 {
		t.Errorf("bad params: %+v", r)
	}
	_ = c.shutdown()
}

func TestURIPaths(t *testing.T) {
	for _, p := range []string{"/tmp/a b/é.tui", "/Users/x/ñ#.tcss"} {
		u := pathToURI(p)
		back, ok := uriToPath(u)
		if !ok || back != p {
			t.Errorf("%q -> %q -> %q (%v)", p, u, back, ok)
		}
	}
	if _, ok := uriToPath("untitled:Untitled-1"); ok {
		t.Error("untitled: has a path")
	}
	if got := pathToURI("/tmp/x.tui"); got != "file:///tmp/x.tui" {
		t.Errorf("pathToURI = %q", got)
	}
}
