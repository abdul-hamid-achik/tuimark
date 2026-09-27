package parse_test

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// codeSet returns the distinct codes of doc's diagnostics, sorted.
func codeSet(doc *parse.Document) string {
	seen := map[string]bool{}
	var out []string
	for _, d := range doc.Diags {
		if !seen[d.Code] {
			seen[d.Code] = true
			out = append(out, d.Code)
		}
	}
	sort.Strings(out)
	return strings.Join(out, " ")
}

// findMsg returns the first diagnostic with code whose message contains sub.
func findMsg(doc *parse.Document, code, sub string) (ir.Diagnostic, bool) {
	for _, d := range doc.Diags {
		if d.Code == code && strings.Contains(d.Msg, sub) {
			return d, true
		}
	}
	return ir.Diagnostic{}, false
}

// Round-2 finding 7: an unknown tag is kept out of the tree, but the
// tree-level checks (V004, V012, V013, B006) still visit its subtree.
func TestUnknownTagSubtreeGetsTreeLevelChecks(t *testing.T) {
	src := "<tui version=\"1\">\n<screen id=\"main\">\n" +
		"<widget>\n" +
		"<button id=\"a\" label=\"x\"/>\n" +
		"<button label=\"noid\"/>\n" +
		"<list id=\"l\" each=\"xs as x\"><item/><item/></list>\n" +
		"<gadget><box id=\"deep\"/><box id=\"deep\"/></gadget>\n" +
		"</widget>\n" +
		"<button id=\"a\" label=\"y\"/>\n" +
		"</screen>\n</tui>"
	doc := parse.Parse([]byte(src), "u1.tui")
	for _, want := range []struct {
		code string
		line int
		sub  string
	}{
		{"V001", 3, "unknown tag <widget>"},
		{"V012", 5, "needs an id"},
		{"V013", 6, "exactly one <item>"},
		{"B006", 6, "no key"},
		{"V001", 7, "unknown tag <gadget>"},
		{"V004", 7, `duplicate id "deep"`},
		// Document order: the dropped button comes first, so the tree one
		// is the duplicate.
		{"V004", 9, `duplicate id "a" (first defined at 4:1)`},
	} {
		found := false
		for _, d := range doc.Diags {
			if d.Code == want.code && d.Line == want.line && strings.Contains(d.Msg, want.sub) {
				found = true
			}
		}
		if !found {
			t.Errorf("missing %s at line %d (%q); got %v", want.code, want.line, want.sub, doc.Diags)
		}
	}
	// The dropped subtree still never becomes addressable.
	if n := doc.IDs["a"]; n == nil || n.Line != 9 {
		t.Errorf(`doc.IDs["a"] = %+v, want the <button> in the tree (line 9)`, n)
	}
	for _, id := range []string{"l", "deep"} {
		if doc.IDs[id] != nil {
			t.Errorf("doc.IDs[%q] leaks from a dropped subtree", id)
		}
	}
	// The unknown tag itself gets V001 only: its attributes (id,
	// focusable) are not validated, so they add no V004/V012.
	doc = parse.Parse([]byte(coreV1(`<widget id="w" focusable="true"/><box id="w"/>`)), "t.tui")
	if got := codeSet(doc); got != "V001" {
		t.Errorf("unknown tag attributes: codes = %s (%v)", got, doc.Diags)
	}
}

// Round-2 finding 10: XML 1.0 §2.11 end-of-line handling turns a lone CR
// into LF, like CRLF: no V007, lines kept apart, positions per line.
func TestLoneCRIsALineBreak(t *testing.T) {
	src := "<tui version=\"1\">\r  <screen id=\"main\">\r    <text id=\"t\">\r      hello\r      world\r    </text>\r  </screen>\r</tui>\r"
	doc := parse.Parse([]byte(src), "cr.tui")
	if len(doc.Diags) != 0 {
		t.Fatalf("CR-only line ends: %v", doc.Diags)
	}
	txt := doc.IDs["t"]
	if txt.Text != "hello\nworld" || txt.Line != 3 || txt.Col != 5 {
		t.Errorf("text = %q at %d:%d, want \"hello\\nworld\" at 3:5", txt.Text, txt.Line, txt.Col)
	}
	doc = parse.Parse([]byte("<tui version=\"1\">\r<screen>\r  <box>\r</screen>\r</tui>"), "crerr.tui")
	if d, ok := coreFind(doc, "V005"); !ok || d.Line != 4 || d.Col != 1 || !strings.Contains(d.Msg, "opened at 3:3") {
		t.Errorf("V005 position with CR line ends = %v", doc.Diags)
	}
}

// Round-2 finding 12: the XML declaration's version is '1.' [0-9]+.
func TestXMLDeclVersionIsNumeric(t *testing.T) {
	for _, v := range []string{"1.x", "1.0a", "1. ", "1.-"} {
		doc := parse.Parse([]byte(`<?xml version="`+v+`"?>`+"\n"+coreV1(`<text>hi</text>`)), "t.tui")
		if d, ok := coreFind(doc, "V005"); !ok || !strings.Contains(d.Msg, "unsupported version") {
			t.Errorf("version=%q: want V005 unsupported version, got %v", v, doc.Diags)
		}
	}
	for _, v := range []string{"1.0", "1.10"} {
		doc := parse.Parse([]byte(`<?xml version="`+v+`"?>`+"\n"+coreV1(`<text>hi</text>`)), "t.tui")
		if len(doc.Diags) != 0 {
			t.Errorf("version=%q: %v", v, doc.Diags)
		}
	}
}

// Round-2 finding 15: nothing inside a list row ever takes focus (the list
// does, and navigates the rows), so a focus target there is rejected
// statically instead of rendering a widget that never responds.
func TestListItemsNeverTakeFocus(t *testing.T) {
	// The per-row button: V001 (with the keymap advice), and screen
	// focus= naming it is B005. No V012: an id would not help.
	rowbtn := "<tui version=\"1\">\n<screen id=\"s\" focus=\"#del\">\n" +
		"<list id=\"rows\" each=\"rows as r\" key=\"r.id\"><item><row><text>{r.id}</text><button id=\"del\" label=\"x\" on:click=\"delete\"/><button label=\"noid\"/></row></item></list>\n" +
		"<button id=\"after\" label=\"after\"/>\n</screen>\n</tui>"
	doc := parse.Parse([]byte(rowbtn), "rowbtn.tui")
	if got := codeSet(doc); got != "B005 V001" {
		t.Errorf("row button: codes = %s (%v)", got, doc.Diags)
	}
	if d, ok := findMsg(doc, "V001", "<button> inside a list <item> can never take focus"); !ok ||
		!strings.Contains(d.Msg, "list rows are navigated by their list") || !strings.Contains(d.Msg, `when="#rows:focus"`) {
		t.Errorf("V001 message = %v", doc.Diags)
	}
	if _, ok := findMsg(doc, "B005", `focus="#del" is inside a list <item>`); !ok {
		t.Errorf("screen focus=#del: want B005, got %v", doc.Diags)
	}
	// Static list: focus attributes on an item are V002, a button inside
	// one is V001, and keymap to=/when= naming either id is B005.
	static := "<tui version=\"1\">\n<keymap>\n" +
		"<bind keys=\"x\" action=\"focus\" to=\"#b1\"/>\n" +
		"<bind keys=\"y\" action=\"go\" when=\"#it:focus\"/>\n" +
		"<bind keys=\"z\" action=\"go\" when=\"#l:focus\"/>\n" +
		"</keymap>\n<screen>\n" +
		"<list id=\"l\"><item id=\"it\" focusable=\"true\" on:click=\"go\" on:focus=\"f\"><text>a</text></item><item><button id=\"b1\" on:click=\"one\"/></item></list>\n" +
		"</screen>\n</tui>"
	doc = parse.Parse([]byte(static), "static.tui")
	for _, attr := range []string{"focusable", "on:click", "on:focus"} {
		if _, ok := findMsg(doc, "V002", fmt.Sprintf("attribute %q is not allowed on a list <item>: list rows are navigated by their list", attr)); !ok {
			t.Errorf("item %s: want V002, got %v", attr, doc.Diags)
		}
	}
	if _, ok := findMsg(doc, "V001", "<button> inside a list <item>"); !ok {
		t.Errorf("button in a static item: want V001, got %v", doc.Diags)
	}
	if _, ok := findMsg(doc, "V002", "(on <button>)"); ok {
		t.Errorf("a button inside an item is V001 as a whole, not also V002 per attribute: %v", doc.Diags)
	}
	if _, ok := findMsg(doc, "B005", `to="#b1" is inside a list <item>`); !ok {
		t.Errorf("keymap to=#b1: want B005, got %v", doc.Diags)
	}
	if _, ok := findMsg(doc, "B005", `when="#it:focus" refers to #it, which is inside a list <item>`); !ok {
		t.Errorf("keymap when=#it: want B005, got %v", doc.Diags)
	}
	for _, d := range doc.Diags {
		if d.Line == 5 {
			t.Errorf("when=#l:focus names the list itself, which is fine: %v", d)
		}
	}
	// Anything focusable, or with a click/focus handler, anywhere in a row.
	for _, c := range []struct{ body, code, sub string }{
		{`<list id="l"><item><box on:click="x"/></item></list>`, "V002", `"on:click" is not allowed inside a list <item> (on <box>)`},
		{`<list id="l"><item><col><text focusable="false">a</text></col></item></list>`, "V002", `"focusable" is not allowed inside a list <item> (on <text>)`},
		{`<list id="l"><item><box id="b" focusable="true"/></item></list>`, "V002", `"focusable" is not allowed inside a list <item>`},
		{`<list id="l" each="a as x" key="x"><item><list id="inner"><item/></list></item></list>`, "V001", "<list> inside a list <item>"},
		{`<list id="l"><item><input id="q"/></item></list>`, "V001", "<input> inside a list <item>"},
	} {
		doc := parse.Parse([]byte(coreV1(c.body)), "t.tui")
		if _, ok := findMsg(doc, c.code, c.sub); !ok {
			t.Errorf("%s: want %s %q, got %v", c.body, c.code, c.sub, doc.Diags)
		}
		if _, ok := coreFind(doc, "V012"); ok {
			t.Errorf("%s: no V012 inside a list item: %v", c.body, doc.Diags)
		}
	}
	// The catalog (and so AGENTS.md) no longer offers them on <item>.
	for _, a := range []string{"focusable", "on:click", "on:focus"} {
		if parse.TagAttrs["item"][a] {
			t.Errorf("TagAttrs[item] still lists %s", a)
		}
	}
	// Outside a list row the same markup is fine.
	doc = parse.Parse([]byte(coreV1(`<box id="b" focusable="true" on:click="x" on:focus="y"/><button id="c" label="c"/>`)), "t.tui")
	if len(doc.Diags) != 0 {
		t.Errorf("outside a list: %v", doc.Diags)
	}
}

// Round-2 finding 20: the binding grammar spells out its whitespace (SPEC
// §7): interp := "{" path "}", each := path " as " ident,
// if := path | "!" path.
func TestBindingWhitespaceIsRejected(t *testing.T) {
	for _, c := range []struct{ body, code string }{
		{`<text>{ folder }</text>`, "V003"},
		{`<text>{folder }</text>`, "V003"},
		{`<box title="{ folder }"/>`, "V003"},
		{`<box if="! a"/>`, "V011"},
		{`<box if=" a"/>`, "V011"},
		{`<box hidden="! a"/>`, "V003"},
		{`<list id="l" each="tickets  as item" key="item"><item/></list>`, "V011"},
		{`<list id="l" each=" tickets as item" key="item"><item/></list>`, "V011"},
		{`<list id="l" each="tickets as item " key="item"><item/></list>`, "V011"},
	} {
		doc := parse.Parse([]byte(coreV1(c.body)), "t.tui")
		if got := codeSet(doc); got != c.code {
			t.Errorf("%s: codes = %s, want %s (%v)", c.body, got, c.code, doc.Diags)
		}
	}
	if d, ok := coreFind(parse.Parse([]byte(coreV1(`<text>{ folder }</text>`)), "t.tui"), "V003"); !ok || !strings.Contains(d.Msg, "write {folder}") {
		t.Errorf("the V003 for { folder } should say how to write it: %v", d)
	}
	// The tight forms are fine.
	doc := parse.Parse([]byte(coreV1(`<text>{folder}</text><box if="!a" hidden="b.c"/><list id="l" each="tickets as item" key="item.id"><item/></list>`)), "t.tui")
	if len(doc.Diags) != 0 {
		t.Errorf("tight bindings: %v", doc.Diags)
	}
}

// Round-2 finding 9: sizes are checked on the literal: a huge whole number
// of cells is a whole number (layout clamps it), a fraction hidden by
// float64 rounding is still a fraction, and a tiny positive fr weight is
// not zero. Finding 8: value="..." is compared with 100 exactly.
func TestSizeLiteralsInDocuments(t *testing.T) {
	tiny := "0." + strings.Repeat("0", 400) + "1fr"
	for _, body := range []string{
		`<col style="height: 99999999999999999999"/>`,
		`<box width="99999999999999999999"/>`,
		`<box width="9223372036854775807" height="9223372036854775808"/>`,
		`<box style="min-width: 99999999999999999999999"/>`,
		`<row><box width="` + tiny + `"/><box width="1fr"/></row>`,
		`<box style="flex: ` + strings.TrimSuffix(tiny, "fr") + `"/>`,
		`<progress value="100.000000000000000000"/>`,
	} {
		if doc := parse.Parse([]byte(coreV1(body)), "t.tui"); len(doc.Diags) != 0 {
			t.Errorf("%.80s: %v", body, doc.Diags)
		}
	}
	for _, c := range []struct{ body, sub string }{
		{`<box width="1.00000000000000000001"/>`, "cells are integers"},
		{`<box style="height: 1.00000000000000000001"/>`, "cells are integers"},
		{`<box width="0fr"/>`, "fr weight must be > 0"},
		{`<progress value="100.00000000000000001"/>`, "want a number 0-100"},
		{`<box width="` + strings.Repeat("1", ir.MaxDecimalLen+1) + `"/>`, "at most 1000 characters"},
	} {
		doc := parse.Parse([]byte(coreV1(c.body)), "t.tui")
		if d, ok := coreFind(doc, "V003"); !ok || !strings.Contains(d.Msg, c.sub) {
			t.Errorf("%.80s: want V003 %q, got %v", c.body, c.sub, doc.Diags)
		}
	}
}

// Round-2 finding 11: InlineDecls lists the style="" declarations of every
// node, rendered or not, with the file and position the cascade reports,
// so css.CheckTokens can check their tokens statically.
func TestInlineDeclsCoverEveryNode(t *testing.T) {
	src := "<tui version=\"1\">\n" +
		"<screen id=\"main\">\n" +
		"<text id=\"t\" style=\"color: $typo1\">hi</text>\n" +
		"<text id=\"v\" if=\"nope\" style=\"background: var(--typo3)\">y</text>\n" +
		"<list id=\"l\" each=\"xs as x\" key=\"x\"><item style=\"color: $typo5\"><text>{x}</text></item></list>\n" +
		"<widget><box style=\"color: $typo6\"/></widget>\n" +
		"<modal id=\"m\" open=\"false\" style=\"color: $typo2\"><text>x</text></modal>\n" +
		"</screen>\n" +
		"<screen id=\"other\"><text style=\"color: $typo4; bold: true\">o</text></screen>\n" +
		"</tui>"
	doc := parse.Parse([]byte(src), "dir/a.tui")
	var got []string
	for _, d := range doc.InlineDecls() {
		got = append(got, fmt.Sprintf("%s %d:%d %s=%s", d.File, d.Line, d.Col, d.Prop, d.Value))
	}
	want := []string{
		"a.tui 3:14 color=$typo1",
		"a.tui 4:24 background=var(--typo3)",
		"a.tui 5:43 color=$typo5",
		"a.tui 7:28 color=$typo2",
		"a.tui 9:26 color=$typo4",
		"a.tui 9:26 bold=true",
		"a.tui 6:14 color=$typo6",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("InlineDecls =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	var msgs []string
	for _, d := range css.CheckTokens(nil, doc.Theme, doc.InlineDecls()...) {
		msgs = append(msgs, fmt.Sprintf("%s %s:%d:%d %s", d.Code, d.File, d.Line, d.Col, d.Msg))
	}
	wantMsgs := []string{
		"V003 a.tui:3:14 color: unknown token $typo1",
		"V003 a.tui:4:24 background: unknown token $typo3",
		"V003 a.tui:5:43 color: unknown token $typo5",
		"V003 a.tui:6:14 color: unknown token $typo6",
		"V003 a.tui:7:28 color: unknown token $typo2",
		"V003 a.tui:9:26 color: unknown token $typo4",
	}
	if strings.Join(msgs, "\n") != strings.Join(wantMsgs, "\n") {
		t.Errorf("CheckTokens(inline) =\n%s\nwant\n%s", strings.Join(msgs, "\n"), strings.Join(wantMsgs, "\n"))
	}
	// A document read from a reader has no file name, like the cascade's.
	if ds := parse.Parse([]byte(src), "").InlineDecls(); len(ds) == 0 || ds[0].File != "" {
		t.Errorf("InlineDecls without a file: %+v", ds)
	}
}
