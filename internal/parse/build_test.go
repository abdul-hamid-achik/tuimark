package parse_test

import (
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// coreV1 wraps body so that it starts at line 3, column 1 of a v1 document.
func coreV1(body string) string {
	return "<tui version=\"1\">\n<screen>\n" + body + "\n</screen>\n</tui>"
}

func coreFind(doc *parse.Document, code string) (ir.Diagnostic, bool) {
	for _, d := range doc.Diags {
		if d.Code == code {
			return d, true
		}
	}
	return ir.Diagnostic{}, false
}

func coreCodes(doc *parse.Document) string {
	var out []string
	for _, d := range doc.Diags {
		out = append(out, d.Code)
	}
	return strings.Join(out, " ")
}

// coreNode finds the first node with tag in document order.
func coreNode(n *ir.Node, tag string) *ir.Node {
	if n == nil {
		return nil
	}
	if n.Tag == tag {
		return n
	}
	for _, c := range n.Children {
		if f := coreNode(c, tag); f != nil {
			return f
		}
	}
	return nil
}

func TestFixturesParseClean(t *testing.T) {
	for _, path := range []string{"../../examples/spike/inbox.tui", "../../examples/inbox/app.tui"} {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		doc := parse.Parse(src, path)
		if len(doc.Diags) != 0 {
			t.Errorf("%s: %v", path, doc.Diags)
		}
	}
}

func TestSpikeDocument(t *testing.T) {
	src, _ := os.ReadFile("../../examples/spike/inbox.tui")
	doc := parse.Parse(src, "examples/spike/inbox.tui")
	if !doc.Spike || doc.Root.Tag != "app" || doc.Root.Kind != "col" {
		t.Fatalf("spike root: spike=%v tag=%s kind=%s", doc.Spike, doc.Root.Tag, doc.Root.Kind)
	}
	if len(doc.Screens) != 0 || len(doc.Keymap) != 0 || len(doc.Styles) != 0 {
		t.Error("a spike document has no screens, keymap, or styles")
	}
	for _, id := range []string{"root", "body", "inbox", "detail", "status"} {
		if doc.IDs[id] == nil {
			t.Errorf("IDs lacks %q", id)
		}
	}
	inbox := doc.IDs["inbox"]
	if inbox.Line != 4 || inbox.Col != 7 || inbox.Path != "/col/row/box" {
		t.Errorf("inbox position %d:%d path %s", inbox.Line, inbox.Col, inbox.Path)
	}
	if strings.Join(inbox.Order, " ") != "id width height border" {
		t.Errorf("attribute order = %v", inbox.Order)
	}
	// Presentational attributes become CSS hints; border="1" means single.
	var hints []string
	for _, h := range inbox.Hints {
		hints = append(hints, h.Name+"="+h.Value)
	}
	if strings.Join(hints, " ") != "width=30% height=1fr border=single" {
		t.Errorf("hints = %v", hints)
	}
	if txt := inbox.Children[0]; txt.Tag != "text" || txt.Text != "Inbox" {
		t.Errorf("inbox text = %+v", txt)
	}
	if doc.File != "examples/spike/inbox.tui" {
		t.Errorf("File = %q", doc.File)
	}
}

func TestV1Document(t *testing.T) {
	src, _ := os.ReadFile("../../examples/inbox/app.tui")
	doc := parse.Parse(src, "examples/inbox/app.tui")
	if doc.Spike || doc.Version != "1" || doc.Theme != "dark" {
		t.Fatalf("spike=%v version=%q theme=%q", doc.Spike, doc.Version, doc.Theme)
	}
	if len(doc.Styles) != 1 || doc.Styles[0].Src != "theme.tcss" || doc.Styles[0].Line != 2 {
		t.Errorf("styles = %+v", doc.Styles)
	}
	if len(doc.Screens) != 1 || doc.Screens[0].ID != "main" {
		t.Fatalf("screens = %v", doc.Screens)
	}
	if len(doc.Keymap) != 3 {
		t.Fatalf("keymap = %+v", doc.Keymap)
	}
	k0, k1, k2 := doc.Keymap[0], doc.Keymap[1], doc.Keymap[2]
	if strings.Join(k0.Keys, ",") != "q" || k0.Action != "quit" || k0.Line != 4 {
		t.Errorf("keymap[0] = %+v", k0)
	}
	if k1.Action != "open" || k1.When != "#inbox:focus" || k1.WhenSel == nil || k1.WhenSel.Parts[0].ID != "inbox" {
		t.Errorf("keymap[1] = %+v", k1)
	}
	if k2.Action != "focus" || k2.To != "query" {
		t.Errorf("keymap[2] = %+v (to is stored without '#')", k2)
	}
	list := doc.IDs["inbox"]
	if list.Each != "tickets as item" || list.Bind != "selected" || list.On["select"] != "open" || list.Attrs["key"] != "item.id" {
		t.Errorf("list = %+v", list)
	}
	if q := doc.IDs["query"]; q.Bind != "query" || q.On["submit"] != "search" || q.Attrs["placeholder"] != "search" {
		t.Errorf("input = %+v", q)
	}
	if h := coreNode(doc.IDs["header"], "text"); h.Text != "mail  {folder}" {
		t.Errorf("header text = %q (inner spaces are kept)", h.Text)
	}
	guards := []string{}
	for _, c := range doc.IDs["detail"].Children {
		guards = append(guards, c.If)
	}
	if strings.Join(guards, "|") != "selected_ticket|!selected_ticket" {
		t.Errorf("detail guards = %v", guards)
	}
	if doc.IDs["body"].Path != "/screen/col/row" {
		t.Errorf("path = %q", doc.IDs["body"].Path)
	}
}

// Every parse-pass code of SPEC §14 that the parser emits, with its exact
// position. v1 bodies start at 3:1 (see coreV1).
func TestParseDiagnostics(t *testing.T) {
	cases := []struct {
		name, src, code string
		line, col       int
		msg             string
	}{
		// V001 unknown tag / tag out of context.
		{"unknown tag", coreV1(`<widget/>`), "V001", 3, 1, "unknown tag <widget>"},
		{"uppercase tag", coreV1(`<Box/>`), "V001", 3, 1, "lowercase"},
		{"namespaced tag", coreV1(`<x:box/>`), "V001", 3, 1, "namespaces"},
		{"app inside tui", coreV1(`<app/>`), "V001", 3, 1, "phase-0 root alias"},
		{"item outside list", coreV1(`<item/>`), "V001", 3, 1, "direct child of <list>"},
		{"bind outside keymap", coreV1(`<bind keys="q" action="quit"/>`), "V001", 3, 1, "inside <keymap>"},
		{"nested screen", coreV1(`<screen id="x"/>`), "V001", 3, 1, "direct child of <tui>"},
		{"style in screen", coreV1(`<style/>`), "V001", 3, 1, "direct child of <tui>"},
		{"keymap in screen", coreV1(`<keymap/>`), "V001", 3, 1, "direct child of <tui>"},
		{"nested tui", coreV1(`<tui version="1"/>`), "V001", 3, 1, "only valid as the document root"},
		{"list child not item", coreV1(`<list id="l"><box/></list>`), "V001", 3, 14, "<list> children must be <item>"},
		{"layout tag directly in tui", "<tui version=\"1\">\n<col/>\n<screen/>\n</tui>", "V001", 2, 1, "must be inside a <screen>"},
		{"keymap child not bind", "<tui version=\"1\">\n<keymap>\n<col/>\n</keymap>\n<screen/>\n</tui>", "V001", 3, 1, "<keymap> children must be <bind>"},
		{"spike: v1 tag", "<app>\n<list/>\n</app>", "V001", 2, 1, "not in the phase-0 vocabulary"},
		{"spike: unknown tag", "<app>\n<widget/>\n</app>", "V001", 2, 1, "unknown tag <widget>"},
		{"spike: nested app", "<app>\n<app/>\n</app>", "V001", 2, 1, "only valid as the document root"},

		// V002 unknown attribute.
		{"spike: title", `<app><box title="x"/></app>`, "V002", 1, 11, "phase-0 attributes"},
		{"spike: style", `<app style="bold: true"/>`, "V002", 1, 6, "phase-0 attributes"},
		{"spike: on:click", `<app><box on:click="x"/></app>`, "V002", 1, 11, "phase-0 attributes"},
		{"bind on box", coreV1(`<box bind="x"/>`), "V002", 3, 6, `"bind" is not allowed on <box>`},
		{"each on text", coreV1(`<text each="a as b"/>`), "V002", 3, 7, `"each" is not allowed on <text>`},
		{"value on input", coreV1(`<input id="q" value="3"/>`), "V002", 3, 15, `"value" is not allowed on <input>`},
		{"unknown event", coreV1(`<box on:hover="x"/>`), "V002", 3, 6, `unknown event "on:hover"`},
		{"event on wrong tag", coreV1(`<button id="b" on:select="x"/>`), "V002", 3, 16, `"on:select" is not allowed on <button>`},
		{"uppercase attribute", coreV1(`<box ID="x"/>`), "V002", 3, 6, `"ID"`},
		{"title on tui", "<tui version=\"1\" title=\"x\">\n<screen/>\n</tui>", "V002", 1, 18, `"title" is not allowed on <tui>`},
		{"width on screen", "<tui version=\"1\">\n<screen width=\"10\"/>\n</tui>", "V002", 2, 9, `"width" is not allowed on <screen>`},
		{"attribute on keymap", "<tui version=\"1\">\n<keymap id=\"k\"/>\n<screen/>\n</tui>", "V002", 2, 9, `"id" is not allowed on <keymap>`},

		// V003 bad unit / value / token / property.
		{"width foo", coreV1(`<box width="foo"/>`), "V003", 3, 6, "bad size"},
		{"width px", coreV1(`<box width="10px"/>`), "V003", 3, 6, "bad size"},
		{"height negative", coreV1(`<box height="-1"/>`), "V003", 3, 6, "bad size"},
		{"fraction of a cell", coreV1(`<box width="1.5"/>`), "V003", 3, 6, "cells are integers"},
		{"zero fr", coreV1(`<box width="0fr"/>`), "V003", 3, 6, "must be > 0"},
		{"gap too big", coreV1(`<box gap="5"/>`), "V003", 3, 6, "0-4"},
		{"pad unit", coreV1(`<box pad="1px"/>`), "V003", 3, 6, "cells only"},
		{"pad five values", coreV1(`<box pad="1 2 3 4 5"/>`), "V003", 3, 6, "1-4 cell values"},
		{"border 2", coreV1(`<box border="2"/>`), "V003", 3, 6, "want 1, 0, none, single"},
		{"wrap yes", coreV1(`<text wrap="yes">x</text>`), "V003", 3, 7, "wrap"},
		{"style unknown property", coreV1(`<box style="colour: red"/>`), "V003", 3, 6, `unknown property "colour"`},
		{"style bad unit", coreV1(`<box style="width: 10px"/>`), "V003", 3, 6, "bad size"},
		{"style no colon", coreV1(`<box style="width"/>`), "V003", 3, 6, "expected 'property: value'"},
		{"style custom property", coreV1(`<box style="--accent: red"/>`), "V003", 3, 6, "only allowed in :root"},
		{"action with space", coreV1(`<button id="b" on:click="do it"/>`), "V003", 3, 16, "plain name"},
		{"action call", coreV1(`<button id="b" on:click="open()"/>`), "V003", 3, 16, "plain name"},
		{"id starts with digit", coreV1(`<box id="1abc"/>`), "V003", 3, 6, "bad id"},
		{"class with dot", coreV1(`<box class="a.b"/>`), "V003", 3, 6, "bad class name"},
		{"hidden expression", coreV1(`<box hidden="a b"/>`), "V003", 3, 6, "want true, false, a path, or !path"},
		{"focusable yes", coreV1(`<box focusable="yes"/>`), "V003", 3, 6, "want true or false"},
		{"secret 1", coreV1(`<input id="q" secret="1"/>`), "V003", 3, 15, "want true or false"},
		{"bind not a path", coreV1(`<input id="q" bind="a b"/>`), "V003", 3, 15, "is not a path"},
		{"bind index", coreV1(`<input id="q" bind="tickets.0"/>`), "V003", 3, 15, "is not a path"},
		{"open number", coreV1(`<modal id="m" open="1"/>`), "V003", 3, 15, "want true, false, a path, or !path"},
		{"key not a path", coreV1(`<list id="l" each="t as i" key="a-b"><item/></list>`), "V003", 3, 28, "key="},
		{"title expression", coreV1(`<box title="{a+b}"/>`), "V003", 3, 6, "no expressions"},
		{"placeholder expression", coreV1(`<input id="q" placeholder="{1}"/>`), "V003", 3, 15, "no expressions"},
		{"label interpolation", coreV1(`<button id="b" label="{x}"/>`), "V003", 3, 16, "only allowed in <text>, title, and placeholder"},
		{"button body interpolation", coreV1(`<button id="b">{x}</button>`), "V003", 3, 1, "only allowed in <text>, title, and placeholder"},
		{"text expression", coreV1(`<text>n = {count + 1}</text>`), "V003", 3, 7, "no expressions"},
		{"text filter", coreV1(`<text>{name|upper}</text>`), "V003", 3, 7, "no expressions"},
		{"scroll axis", coreV1(`<scroll axis="z"/>`), "V003", 3, 9, "x, y, or both"},
		{"rule axis both", coreV1(`<rule axis="both"/>`), "V003", 3, 7, "want x or y"},
		{"progress over 100", coreV1(`<progress value="101"/>`), "V003", 3, 11, "0-100"},
		{"progress negative", coreV1(`<progress value="-1"/>`), "V003", 3, 11, "0-100"},
		{"progress exponent", coreV1(`<progress value="1e2"/>`), "V003", 3, 11, "0-100"},
		{"progress nan", coreV1(`<progress value="NaN"/>`), "V003", 3, 11, "0-100"},
		{"version 3", "<tui version=\"3\">\n<screen/>\n</tui>", "V003", 1, 6, `version="3"`},
		{"unknown theme", "<tui version=\"1\" theme=\"neon\">\n<screen/>\n</tui>", "V003", 1, 18, `unknown theme "neon"`},
		{"screen focus without #", "<tui version=\"1\">\n<screen focus=\"query\"/>\n</tui>", "V003", 2, 9, "want #id"},
		{"empty keys", "<tui version=\"1\">\n<keymap>\n<bind keys=\"\" action=\"quit\"/>\n</keymap>\n<screen/>\n</tui>", "V003", 3, 1, "needs keys="},
		{"bad key token", "<tui version=\"1\">\n<keymap>\n<bind keys=\"q,f1\" action=\"quit\"/>\n</keymap>\n<screen/>\n</tui>", "V003", 3, 1, `unknown key "f1"`},
		{"no action", "<tui version=\"1\">\n<keymap>\n<bind keys=\"q\"/>\n</keymap>\n<screen/>\n</tui>", "V003", 3, 1, "needs action="},
		{"action expression", "<tui version=\"1\">\n<keymap>\n<bind keys=\"q\" action=\"a.b\"/>\n</keymap>\n<screen/>\n</tui>", "V003", 3, 1, "plain name"},
		{"when universal", "<tui version=\"1\">\n<keymap>\n<bind keys=\"q\" action=\"x\" when=\"*\"/>\n</keymap>\n<screen/>\n</tui>", "V003", 3, 1, "universal selector"},
		{"when root", "<tui version=\"1\">\n<keymap>\n<bind keys=\"q\" action=\"x\" when=\":root\"/>\n</keymap>\n<screen/>\n</tui>", "V003", 3, 1, ":root is not a node selector"},
		{"to without #", "<tui version=\"1\">\n<keymap>\n<bind keys=\"/\" action=\"focus\" to=\"query\"/>\n</keymap>\n<screen/>\n</tui>", "V003", 3, 1, "want #id"},
		{"focus without to", "<tui version=\"1\">\n<keymap>\n<bind keys=\"/\" action=\"focus\"/>\n</keymap>\n<screen/>\n</tui>", "V003", 3, 1, `needs to="#id"`},

		// V004 duplicate id.
		{"duplicate id", coreV1("<box id=\"a\"/>\n<box id=\"a\"/>"), "V004", 4, 1, `duplicate id "a" (first defined at 3:1)`},
		{"duplicate id spike", "<app>\n<box id=\"a\"/>\n<col>\n<box id=\"a\"/>\n</col>\n</app>", "V004", 4, 1, "first defined at 2:1"},

		// V005 structure.
		{"foreign root", "<html/>", "V005", 1, 1, "document root must be <tui>"},
		{"tui without screen", `<tui version="1"/>`, "V005", 1, 1, "no <screen>"},

		// V007 control characters and ANSI.
		{"ansi in text", coreV1("<text>a\x1b[31mred\x1b[0m</text>"), "V007", 3, 7, "control character or ANSI"},
		{"bell in title", coreV1("<box title=\"a\x07b\"/>"), "V007", 3, 6, "control character"},
		{"osc in placeholder", coreV1("<input id=\"q\" placeholder=\"\x1b]0;x\x07ph\"/>"), "V007", 3, 15, "control character"},
		{"c1 in label", coreV1("<button id=\"b\" label=\"a\u009bb\"/>"), "V007", 3, 16, "control character"},
		{"tab inside text", coreV1("<text>a\tb</text>"), "V007", 3, 7, "control character"},
		{"spike ansi", "<app><text>\x1b[1mx</text></app>", "V007", 1, 12, "control character"},

		// V011 each / if missing path.
		{"if empty", coreV1(`<box if=""/>`), "V011", 3, 6, "not a path"},
		{"if expression", coreV1(`<box if="a == b"/>`), "V011", 3, 6, "not a path"},
		{"if bang only", coreV1(`<box if="!"/>`), "V011", 3, 6, "not a path"},
		{"each without alias", coreV1(`<list id="l" each="tickets"><item/></list>`), "V011", 3, 14, `"path as alias"`},
		{"each empty", coreV1(`<list id="l" each=""><item/></list>`), "V011", 3, 14, `"path as alias"`},

		// V012 focusable without id.
		{"input without id", coreV1(`<input/>`), "V012", 3, 1, "<input> is focusable and needs an id"},
		{"list without id", coreV1(`<list/>`), "V012", 3, 1, "needs an id"},
		{"button without id", coreV1(`<button>ok</button>`), "V012", 3, 1, "needs an id"},
		{"modal without id", coreV1(`<modal open="true"/>`), "V012", 3, 1, "needs an id"},
		{"two screens without id", "<tui version=\"1\">\n<screen id=\"a\"/>\n<screen/>\n</tui>", "V012", 3, 1, "each <screen> needs an id"},
		// SPEC §8.3: "Default focusable: input, list, button, modal.
		// Others only if focusable=\"true\"." then "Focusable widgets
		// without id -> V012" — the general rule, not just the four
		// default kinds.
		{"focusable box without id", coreV1(`<box focusable="true"/>`), "V012", 3, 1, "<box> is focusable and needs an id"},
		{"focusable col without id", coreV1(`<col focusable="true"><text>panel</text></col>`), "V012", 3, 1, "needs an id"},
		{"focusable text without id", coreV1(`<text focusable="true">x</text>`), "V012", 3, 1, "needs an id"},
		{"focusable scroll without id", coreV1(`<scroll focusable="true"/>`), "V012", 3, 1, "needs an id"},

		// V013 content model.
		{"element in text", coreV1(`<text><box/></text>`), "V013", 3, 7, "<text> cannot contain elements (found <box>)"},
		{"element in input", coreV1(`<input id="q"><box/></input>`), "V013", 3, 15, "<input> cannot contain elements"},
		{"text in input", coreV1(`<input id="q">x</input>`), "V013", 3, 15, "wrap it in <text>"},
		{"text in col", coreV1(`<col>hello</col>`), "V013", 3, 6, "wrap it in <text>"},
		{"text in rule", coreV1(`<rule>x</rule>`), "V013", 3, 7, "wrap it in <text>"},
		{"text in spacer", coreV1(`<spacer>x</spacer>`), "V013", 3, 9, "wrap it in <text>"},
		{"element in progress", coreV1(`<progress><text/></progress>`), "V013", 3, 11, "<progress> cannot contain elements"},
		{"style src and body", "<tui version=\"1\">\n<style src=\"a.tcss\">box { bold: true; }</style>\n<screen/>\n</tui>", "V013", 2, 1, "either src or an inline body"},
		{"each list without item", coreV1(`<list id="l" each="t as i" key="i.id"></list>`), "V013", 3, 1, "exactly one <item> template (found 0)"},
		{"each list with two items", coreV1(`<list id="l" each="t as i" key="i.id"><item/><item/></list>`), "V013", 3, 1, "(found 2)"},
		{"text in keymap", "<tui version=\"1\">\n<keymap>q</keymap>\n<screen/>\n</tui>", "V013", 2, 9, "wrap it in <text>"},

		// V014 missing version.
		{"missing version", "<tui>\n<screen/>\n</tui>", "V014", 1, 1, `needs version="1"`},

		// L004 modal placement.
		{"modal not last", coreV1("<modal id=\"m\" open=\"true\"/>\n<box/>"), "L004", 4, 1, "found <box> after it"},
		{"modal not in screen", coreV1(`<box><modal id="m"/></box>`), "L004", 3, 6, "direct child of <screen>"},

		// B005 keymap/screen references.
		{"when missing id", "<tui version=\"1\">\n<keymap>\n<bind keys=\"enter\" action=\"open\" when=\"#nope:focus\"/>\n</keymap>\n<screen/>\n</tui>", "B005", 3, 1, "refers to #nope"},
		{"to missing id", "<tui version=\"1\">\n<keymap>\n<bind keys=\"/\" action=\"focus\" to=\"#nope\"/>\n</keymap>\n<screen/>\n</tui>", "B005", 3, 1, `to="#nope"`},
		{"screen focus missing id", "<tui version=\"1\">\n<screen focus=\"#nope\"/>\n</tui>", "B005", 2, 1, `focus="#nope"`},

		// B006 each without key (warning).
		{"each without key", coreV1(`<list id="l" each="t as i"><item/></list>`), "B006", 3, 1, "has no key="},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			doc := parse.Parse([]byte(c.src), "dir/app.tui")
			d, ok := coreFind(doc, c.code)
			if !ok {
				t.Fatalf("want %s, got [%s] %v", c.code, coreCodes(doc), doc.Diags)
			}
			if d.Line != c.line || d.Col != c.col {
				t.Errorf("%s at %d:%d, want %d:%d (%s)", c.code, d.Line, d.Col, c.line, c.col, d.Msg)
			}
			if !strings.Contains(d.Msg, c.msg) {
				t.Errorf("message %q does not contain %q", d.Msg, c.msg)
			}
			if d.File != "app.tui" {
				t.Errorf("File = %q, want the base name", d.File)
			}
			wantSev := ir.Error
			if c.code == "B006" {
				wantSev = ir.Warning
			}
			if d.Severity != wantSev {
				t.Errorf("severity %s, want %s", d.Severity, wantSev)
			}
		})
	}
}

func TestDiagnosticsAreSortedByPosition(t *testing.T) {
	doc := parse.Parse([]byte(coreV1("<widget/>\n<box width=\"x\" gap=\"9\"/>\n<input/>")), "")
	prevLine, prevCol := 0, 0
	for _, d := range doc.Diags {
		if d.Line < prevLine || (d.Line == prevLine && d.Col < prevCol) {
			t.Fatalf("diagnostics out of order: %v", doc.Diags)
		}
		prevLine, prevCol = d.Line, d.Col
		if d.File != "" {
			t.Errorf("Parse(src, \"\") must not invent a file name: %q", d.File)
		}
	}
	if got := coreCodes(doc); got != "V001 V003 V003 V012" {
		t.Errorf("codes = %s", got)
	}
}

func TestCleanDocumentsHaveNoDiagnostics(t *testing.T) {
	for _, src := range []string{
		coreV1(`<box if="!selected_ticket" hidden="false" disabled="!ready" class="a b-c _d"/>`),
		coreV1(`<modal id="m" open="dialog.open" title="Hi {user.name}" on:escape="close" on:open="o" on:close="c"/>`),
		coreV1(`<modal id="m" bind="show"/>`),
		coreV1(`<modal id="m" open="!closed"/>`),
		coreV1(`<input id="q" bind="query" placeholder="search {folder}" secret="true" on:change="c" on:submit="s" on:focus="f"/>`),
		coreV1(`<button id="b" label="Save" on:click="save"/>`),
		coreV1(`<button id="b" focusable="false">Save</button>`),
		coreV1(`<box id="b" focusable="true"/>`),
		// A list's own items are navigated by the list, never individually
		// focused (host.collectFocusables skips them), and an each=
		// template item cannot have a unique id of its own: no V012 for an
		// item or anything inside one (focus attributes there are V002,
		// see TestListItemsNeverTakeFocus).
		coreV1(`<list id="l" focusable="true"><item><text>a</text></item></list>`),
		coreV1(`<list id="l" each="rows as r" key="r.id"><item><row class="x"><text>{r.id}</text><spacer/></row></item></list>`),
		coreV1(`<progress bind="pct"/><progress value="0"/><progress value="100"/><progress value="37.5"/>`),
		coreV1(`<scroll axis="both"><text wrap="truncate">x</text></scroll><rule axis="y"/><spacer/>`),
		coreV1(`<row width="50%" height="auto" gap="4" pad="1 2" border="rounded" style="color: $accent; bold: true"/>`),
		coreV1(`<box border="0"/><box border="none"/><box border="double"/><box border="thick"/>`),
		coreV1(`<list id="l"><item id="a"><text>A</text></item><item><text>B</text></item></list>`),
		coreV1(`<text>a { b</text><text>}</text><text>{a}{b.c}</text>`),
		coreV1(`<box title="{ unclosed"/>`),
		"<tui version=\"1\" theme=\"light\">\n<style>\nbox { bold: true; }\n</style>\n<keymap>\n<bind keys=\"q,ctrl+c\" action=\"quit\"/>\n<bind keys=\"/\" action=\"focus\" to=\"#q\"/>\n<bind keys=\"enter\" action=\"go\" when=\"input#q:focus\"/>\n</keymap>\n<screen id=\"a\" focus=\"#q\" title=\"A\">\n<input id=\"q\"/>\n</screen>\n<screen id=\"b\"/>\n</tui>",
		"<tui version=\"1\">\n<screen>\n<box/>\n<modal id=\"a\"/>\n<modal id=\"b\"/>\n</screen>\n</tui>",
		"<?xml version=\"1.0\"?>\n<!-- c -->\n<app><!-- in --><text>x</text></app>\n<!-- after -->",
		"<app>\n\t<box pad=\"1\">\n\t\t<text>\n\t\t\tInbox\n\t\t</text>\n\t</box>\n</app>",
	} {
		doc := parse.Parse([]byte(src), "t.tui")
		if len(doc.Diags) != 0 {
			t.Errorf("%s\n=> %v", src, doc.Diags)
		}
	}
}

// Each tag accepts exactly its catalog (parse.TagAttrs); anything else is V002.
func TestAttributeCatalog(t *testing.T) {
	own := map[string][]string{ // SPEC §6.1, §6.4, §6.5 own attributes
		"tui":      {"version", "theme"},
		"style":    {"src"},
		"bind":     {"keys", "action", "when", "to"},
		"screen":   {"id", "focus", "title"},
		"list":     {"each", "key", "bind", "on:select"},
		"input":    {"bind", "placeholder", "on:submit", "on:change", "secret"},
		"button":   {"on:click", "label"},
		"progress": {"bind", "value"},
		"modal":    {"open", "title", "on:escape"},
		"scroll":   {"axis"},
		"rule":     {"axis"},
	}
	for tag, attrs := range own {
		for _, a := range attrs {
			if !parse.TagAttrs[tag][a] {
				t.Errorf("<%s> must accept %s", tag, a)
			}
		}
	}
	for _, tag := range ir.Kinds {
		if _, ok := parse.TagAttrs[tag]; !ok {
			t.Errorf("TagAttrs has no entry for <%s>", tag)
		}
	}
	// bind is allowed only on input, list, progress, modal (SPEC §7).
	for tag, allowed := range parse.TagAttrs {
		if allowed["bind"] != (tag == "input" || tag == "list" || tag == "progress" || tag == "modal") {
			t.Errorf("bind on <%s>: allowed=%v", tag, allowed["bind"])
		}
		if allowed["each"] != (tag == "list") {
			t.Errorf("each on <%s>: allowed=%v", tag, allowed["each"])
		}
	}
	tags := make([]string, 0, len(parse.TagAttrs))
	for tag := range parse.TagAttrs {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	for _, tag := range tags {
		if tag == "tui" || tag == "keymap" {
			continue
		}
		for attr := range parse.TagAttrs[tag] {
			src := coreV1("<" + tag + " " + attr + `="x"/>`)
			if doc := parse.Parse([]byte(src), ""); strings.Contains(coreCodes(doc), "V002") {
				t.Errorf("<%s %s>: unexpected V002 %v", tag, attr, doc.Diags)
			}
		}
		src := coreV1("<" + tag + ` bogus="x"/>`)
		if _, ok := coreFind(parse.Parse([]byte(src), ""), "V002"); !ok {
			t.Errorf("<%s bogus>: want V002", tag)
		}
	}
}

// SPEC §6.7: a spike (<app>) document accepts only id class width height gap
// pad border.
func TestSpikeAttributeWhitelist(t *testing.T) {
	for _, a := range ir.SpikeAttrs {
		src := `<app><box ` + a + `="1"/></app>`
		if _, ok := coreFind(parse.Parse([]byte(src), ""), "V002"); ok {
			t.Errorf("spike attribute %s rejected", a)
		}
	}
	for _, a := range []string{"title", "style", "hidden", "disabled", "focusable", "if", "bind", "each", "on:click", "wrap", "placeholder", "value", "axis"} {
		src := `<app><box ` + a + `="1"/></app>`
		if _, ok := coreFind(parse.Parse([]byte(src), ""), "V002"); !ok {
			t.Errorf("spike attribute %s accepted", a)
		}
	}
	// The spike vocabulary is app col row box text; {path} is literal text.
	doc := parse.Parse([]byte(`<app><col><row><box><text>{folder}</text></box></row></col></app>`), "")
	if len(doc.Diags) != 0 {
		t.Fatalf("spike vocabulary: %v", doc.Diags)
	}
	if txt := coreNode(doc.Root, "text"); txt.Text != "{folder}" {
		t.Errorf("text = %q", txt.Text)
	}
	for _, tag := range []string{"screen", "scroll", "spacer", "rule", "list", "input", "button", "progress", "modal", "keymap", "style"} {
		doc := parse.Parse([]byte(`<app><`+tag+`/></app>`), "")
		if d, ok := coreFind(doc, "V001"); !ok || !strings.Contains(d.Msg, "phase-0 vocabulary") {
			t.Errorf("<%s> in a spike document: %v", tag, doc.Diags)
		}
	}
}

func TestTextNormalization(t *testing.T) {
	cases := []struct{ body, want string }{
		{"Inbox", "Inbox"},
		{"\n    mail  {folder}\n  ", "mail  {folder}"},
		{"\n  a\n\n  b\n", "a\n\nb"},
		{"\n\t\tInbox\n\t", "Inbox"},
		{"   ", ""},
		{"x &amp; y", "x & y"},
		{"<![CDATA[a < b]]>", "a < b"},
		{"a<!-- c -->b", "ab"},
		// NBSP (U+00A0) is not XML whitespace (the S production is only
		// space/tab/CR/LF), and rejecting numeric character references
		// (&#160;) makes it the only way to indent <text>: it must
		// survive normalization, unlike a plain space or tab.
		{"  x", "  x"},
	}
	for _, c := range cases {
		doc := parse.Parse([]byte(coreV1("<text>"+c.body+"</text>")), "")
		if len(doc.Diags) != 0 {
			t.Errorf("%q: %v", c.body, doc.Diags)
			continue
		}
		if got := coreNode(doc.Root, "text").Text; got != c.want {
			t.Errorf("text %q => %q, want %q", c.body, got, c.want)
		}
	}
	// Button bodies are normalized the same way; label= wins over the body
	// at runtime but both are stored.
	doc := parse.Parse([]byte(coreV1("<button id=\"b\">\n   Save  it \n</button>")), "")
	if got := doc.IDs["b"].Text; got != "Save  it" {
		t.Errorf("button text = %q", got)
	}
	for _, c := range []struct{ in, want string }{
		{"", ""},
		{"a", "a"},
		{"  a  ", "a"},
		{"\n\na\n\n", "a"},
		{" a \n b ", "a\nb"},
		{"a\n\n\nb", "a\n\n\nb"},
	} {
		if got := parse.NormalizeText(c.in); got != c.want {
			t.Errorf("NormalizeText(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// V007: control characters and ANSI escapes are stripped and reported.
func TestControlCharactersAreStripped(t *testing.T) {
	doc := parse.Parse([]byte(coreV1("<text>a\x1b[31mred\x1b[0m!</text>\n<box id=\"b\" title=\"t\x07i\x1b]0;x\x07tle\"/>\n<input id=\"q\" placeholder=\"p\x1b]0;y\x1b\\h\"/>")), "")
	if got := coreNode(doc.Root, "text").Text; got != "ared!" {
		t.Errorf("text = %q", got)
	}
	if got := doc.IDs["b"].Attrs["title"]; got != "title" {
		t.Errorf("title = %q", got)
	}
	if got := doc.IDs["q"].Attrs["placeholder"]; got != "ph" {
		t.Errorf("placeholder = %q", got)
	}
	n := 0
	for _, d := range doc.Diags {
		if d.Code == "V007" {
			n++
		}
	}
	if n != 3 || len(doc.Diags) != 3 {
		t.Errorf("want three V007, got %v", doc.Diags)
	}
	// Tabs and line breaks in attribute values are plain spaces (XML
	// attribute-value normalization), not V007.
	doc = parse.Parse([]byte(coreV1("<box title=\"a\tb\nc\"/>")), "")
	if len(doc.Diags) != 0 || coreNode(doc.Root, "box").Attrs["title"] != "a b c" {
		t.Errorf("title = %q, diags %v", coreNode(doc.Root, "box").Attrs["title"], doc.Diags)
	}

	for _, c := range []struct {
		in     string
		keepNL bool
		want   string
		bad    bool
	}{
		{"plain", true, "plain", false},
		{"a\nb", true, "a\nb", false},
		{"a\nb", false, "ab", true},
		{"a\tb", true, "ab", true},
		{"a\rb", true, "ab", true},
		{"\x1b[1;31mred\x1b[0m", true, "red", true},
		{"\x1b[?25l", true, "", true},
		{"x\x1b]8;;http://e\x07link\x1b]8;;\x07", true, "xlink", true},
		{"x\x1b]0;t\x1b\\y", true, "xy", true},
		{"a\x1b", true, "a", true},
		{"a\x1bcb", true, "ab", true},
		{"a\x1b[31", true, "a", true},
		{"del\x7f", true, "del", true},
		{"c1\u0085\u009b", true, "c1", true},
		{"ñandú ─│", true, "ñandú ─│", false},
		{"nul\x00", true, "nul", true},
	} {
		got, bad := parse.StripControl(c.in, c.keepNL)
		if got != c.want || bad != c.bad {
			t.Errorf("StripControl(%q, %v) = %q, %v; want %q, %v", c.in, c.keepNL, got, bad, c.want, c.bad)
		}
	}
}

// SPEC §5 defines "whitespace-only text nodes" through well-formed XML's
// own S production (space, tab, CR, LF), not Unicode's broader notion of
// whitespace: a control character such as VT/NEL is not whitespace at all
// (so it must not be silently trimmed away before the control-character
// check runs), and neither is a non-ASCII space such as NBSP or
// U+3000 (the only way to indent <text>, since numeric character
// references are rejected).
func TestXMLWhitespaceIsNotUnicodeWhitespace(t *testing.T) {
	// VT (U+000B) and a C1 NEL (U+0085) at the edges of <text> must reach
	// StripControl instead of being trimmed away first: stripped + V007,
	// same as any other control character in <text>.
	doc := parse.Parse([]byte(coreV1("<text id=\"t\">\u000bhello\u0085</text>")), "t.tui")
	if got := doc.IDs["t"].Text; got != "hello" {
		t.Errorf("text = %q, want %q", got, "hello")
	}
	if _, ok := coreFind(doc, "V007"); !ok {
		t.Errorf("want V007, got %v", doc.Diags)
	}

	// VT/FF directly inside a non-text container (would-be ignorable
	// whitespace) are not whitespace either, so the text is reported
	// instead of silently ignored.
	doc = parse.Parse([]byte(coreV1("<col>\u000b\u000c<text>x</text></col>")), "t.tui")
	if _, ok := coreFind(doc, "V013"); !ok {
		t.Errorf("VT/FF ignorable whitespace: want V013, got %v", doc.Diags)
	}

	// NBSP and IDEOGRAPHIC SPACE are not XML whitespace either: a <col>
	// holding only them is non-whitespace content (V013), the same rule
	// the parser already applies to any other stray text.
	doc = parse.Parse([]byte(coreV1("<col> 　</col>")), "t.tui")
	if _, ok := coreFind(doc, "V013"); !ok {
		t.Errorf("NBSP/U+3000 content: want V013, got %v", doc.Diags)
	}

	// Plain XML S whitespace between elements is still ignored.
	doc = parse.Parse([]byte(coreV1("<col>\n  \t\n  <text>x</text>\n</col>")), "t.tui")
	if len(doc.Diags) != 0 {
		t.Errorf("plain whitespace: %v", doc.Diags)
	}
}

// SPEC §5 restricts tag and attribute names to lowercase, but that is an
// IR-builder rule: the tokenizer accepts a non-ASCII name in full (finding
// 5), so build.go reports the right vocabulary error (V001/V002) instead
// of a misleading, truncated V005 from the tokenizer choking mid-name.
func TestNonASCIINameGetsVocabularyError(t *testing.T) {
	doc := parse.Parse([]byte(coreV1(`<tëxt>hi</tëxt>`)), "t.tui")
	if d, ok := coreFind(doc, "V001"); !ok || !strings.Contains(d.Msg, "tëxt") {
		t.Errorf("non-ASCII tag: want V001 naming <tëxt>, got %v", doc.Diags)
	}
	if _, ok := coreFind(doc, "V005"); ok {
		t.Errorf("non-ASCII tag: want no V005, got %v", doc.Diags)
	}

	doc = parse.Parse([]byte(coreV1(`<text tïtle="x">hi</text>`)), "t.tui")
	if d, ok := coreFind(doc, "V002"); !ok || !strings.Contains(d.Msg, "tïtle") {
		t.Errorf("non-ASCII attribute: want V002 naming %q, got %v", "tïtle", doc.Diags)
	}
	if _, ok := coreFind(doc, "V005"); ok {
		t.Errorf("non-ASCII attribute: want no V005, got %v", doc.Diags)
	}
}

// SPEC §1/§13.1: an unknown kind is an error, not a widget, and must fail
// before layout. Its own subtree is still built (so its diagnostics, and
// any diagnostics from valid ids/attributes nested inside it, are still
// reported), but the node itself must not reach the IR tree: it must never
// show up in dump nodes, `tuimark ir` output, or layout/paint.
func TestUnknownKindExcludedFromTree(t *testing.T) {
	doc := parse.Parse([]byte(coreV1(`<widget id="w" width="20"><box id="inner"/></widget><box id="after"/>`)), "t.tui")
	if _, ok := coreFind(doc, "V001"); !ok {
		t.Fatalf("want V001: %v", doc.Diags)
	}
	if coreNode(doc.Root, "widget") != nil {
		t.Error("the unknown <widget> must not appear in the IR tree")
	}
	if doc.IDs["w"] != nil || doc.IDs["inner"] != nil {
		t.Errorf("ids inside the excluded subtree must not leak into doc.IDs: %v", doc.IDs)
	}
	screen := coreNode(doc.Root, "screen")
	if screen == nil || len(screen.Children) != 1 || screen.Children[0].ID != "after" {
		t.Errorf("screen.Children = %+v, want only the sibling <box id=\"after\">", screen.Children)
	}
	// A known kind in the wrong place (SPEC still calls it an error) is
	// not affected: it keeps its place in the tree.
	doc = parse.Parse([]byte("<tui version=\"1\">\n<col id=\"c\"/>\n<screen/>\n</tui>"), "t.tui")
	if _, ok := coreFind(doc, "V001"); !ok {
		t.Fatalf("want V001 for a misplaced but known tag: %v", doc.Diags)
	}
	if doc.IDs["c"] == nil {
		t.Error("a known (if misplaced) tag must still be built")
	}
}

func TestInlineStyleAndHints(t *testing.T) {
	doc := parse.Parse([]byte(coreV1(`<box id="b" width="10" pad="1 2" gap="2" border="1" style="color: red; width: 5;"/><text id="t" wrap="wrap">x</text>`)), "")
	if len(doc.Diags) != 0 {
		t.Fatal(doc.Diags)
	}
	b := doc.IDs["b"]
	var hints, inline []string
	for _, h := range b.Hints {
		hints = append(hints, h.Name+"="+h.Value)
	}
	for _, p := range b.Inline {
		inline = append(inline, p.Name+"="+p.Value)
	}
	if strings.Join(hints, " ") != "width=10 padding=1 2 gap=2 border=single" {
		t.Errorf("hints = %v", hints)
	}
	if strings.Join(inline, " ") != "color=red width=5" {
		t.Errorf("inline = %v", inline)
	}
	if h := doc.IDs["t"].Hints; len(h) != 1 || h[0].Name != "wrap" || h[0].Value != "wrap" {
		t.Errorf("wrap hint = %v", h)
	}
}

// A bad declaration in style="" must not drop every other declaration in
// the same attribute, matching how a <style> rule's block already
// recovers (only the bad one is dropped, everything else — before or
// after it — is kept).
func TestStyleAttributeRecoversPerDeclaration(t *testing.T) {
	doc := parse.Parse([]byte(coreV1(`<box id="a" style="width: 3; bogus: 1; height: 9x"/>`)), "t.tui")
	var v003 []string
	for _, d := range doc.Diags {
		if d.Code == "V003" {
			v003 = append(v003, d.Msg)
		}
	}
	if len(v003) != 2 {
		t.Fatalf("want two V003 (bogus + height: 9x), got %v", doc.Diags)
	}
	var inline []string
	for _, p := range doc.IDs["a"].Inline {
		inline = append(inline, p.Name+"="+p.Value)
	}
	if strings.Join(inline, " ") != "width=3" {
		t.Errorf("inline = %v, want only the valid width:3 kept", inline)
	}
}

func TestInlineStyleBody(t *testing.T) {
	src := "<tui version=\"1\">\n  <style>\n    box { bold: true; }\n  </style>\n<screen/>\n</tui>"
	doc := parse.Parse([]byte(src), "")
	if len(doc.Styles) != 1 {
		t.Fatalf("styles = %+v", doc.Styles)
	}
	s := doc.Styles[0]
	if s.Src != "" || !strings.Contains(s.Body, "box { bold: true; }") || s.Line != 2 || s.Col != 3 || s.BodyLine != 2 || s.BodyCol != 10 {
		t.Errorf("style ref = %+v", s)
	}
}
