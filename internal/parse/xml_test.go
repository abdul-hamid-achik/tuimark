package parse_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// SPEC §5: every well-formedness failure is V005 with an exact line:col.
func TestParseXMLErrors(t *testing.T) {
	cases := []struct {
		name      string
		src       string
		line, col int
		msg       string
	}{
		{"mismatched close", `<app><box></app>`, 1, 11, "closing tag </app> does not match <box> opened at 1:6"},
		{"mismatched close multiline", "<app>\n\n  <box>\n  </col>\n</app>", 4, 3, "does not match <box> opened at 3:3"},
		{"crlf lines", "<app>\r\n<box>\r\n</app>", 3, 1, "does not match <box> opened at 2:1"},
		// XML 1.0 §2.11: a lone CR is a line break too (round-2 finding 10).
		{"cr lines", "<app>\r<box>\r</app>", 3, 1, "does not match <box> opened at 2:1"},
		{"mixed line ends", "<app>\r\n\r<box>\n\r\r</app>", 6, 1, "does not match <box> opened at 3:1"},
		{"unclosed root", `<app>`, 1, 1, "unclosed <app>"},
		{"unclosed child", "<app>\n  <box>\n", 2, 3, "unclosed <box>"},
		{"eof in start tag", `<app`, 1, 1, "end of input inside the start tag"},
		{"single quotes", `<app id='a'/>`, 1, 9, "must use double quotes"},
		{"unquoted", `<app id=a/>`, 1, 9, "must be double-quoted"},
		{"no value", `<app id/>`, 1, 6, "has no value"},
		{"no space between attrs", `<app a="1"b="2"/>`, 1, 11, "expected whitespace before attribute"},
		{"duplicate attr", `<app id="a" id="b"/>`, 1, 13, `duplicate attribute "id"`},
		{"lt in attr", "<app>\n  <box title=\"a<b\"/>\n</app>", 2, 8, "'<' is not allowed in attribute values"},
		{"unterminated value", `<app id="a/>`, 1, 6, "unterminated value"},
		{"named entity", `<app><text>ab &copy;</text></app>`, 1, 15, "entity &copy; is not allowed"},
		{"numeric entity", `<app><text>&#65;</text></app>`, 1, 12, "entity &#65; is not allowed"},
		{"entity in attr", `<app id="&nbsp;"/>`, 1, 10, "entity &nbsp; is not allowed"},
		{"bare ampersand", `<app><text>a & b</text></app>`, 1, 14, "bare '&'"},
		{"entity after multibyte", `<app><text>ñé&x;</text></app>`, 1, 14, "entity &x; is not allowed"},
		{"entity on second line of text", "<app><text>a\n  &x;</text></app>", 2, 3, "entity &x;"},
		{"doctype", `<!DOCTYPE app><app/>`, 1, 1, "DTD/DOCTYPE is not allowed"},
		{"doctype lowercase", "\n<!doctype app><app/>", 2, 1, "DTD/DOCTYPE is not allowed"},
		{"processing instruction", `<app><?pi x?></app>`, 1, 6, "processing instructions are not allowed"},
		{"stylesheet PI is not the xml decl", `<?xml-stylesheet href="a"?><app/>`, 1, 1, "processing instructions are not allowed"},
		{"declaration", `<app><!ELEMENT x></app>`, 1, 6, "declarations are not allowed"},
		{"double dash in comment", `<app><!-- a -- b --></app>`, 1, 6, `"--" is not allowed inside a comment`},
		{"unterminated comment", `<app><!-- x </app>`, 1, 6, "unterminated comment"},
		{"unterminated cdata", `<app><![CDATA[x</app>`, 1, 6, "unterminated CDATA section"},
		{"text before root", `hello<app/>`, 1, 1, "text before the root element"},
		{"text after root", `<app/>x`, 1, 7, "text after the root element"},
		{"two roots", "<app/>\n<app/>", 2, 1, "a document must have a single root element"},
		{"empty document", ``, 1, 1, "document has no root element"},
		{"only a comment", `<!-- c -->`, 1, 11, "document has no root element"},
		{"invalid utf8", "\xff<app/>", 1, 1, "not valid UTF-8"},
		{"empty tag name", `<>`, 1, 1, "expected a tag name"},
		{"space before tag name", `< app/>`, 1, 1, "expected a tag name"},
		{"digit tag name", `<1app/>`, 1, 1, "expected a tag name"},
		{"malformed close", `<app></app x>`, 1, 6, "malformed closing tag </app"},
		{"unterminated xml decl", `<?xml version="1.0"`, 1, 1, "unterminated XML declaration"},
		{"bad attribute name", `<app 1a="x"/>`, 1, 6, "bad attribute"},

		// XML declaration: real well-formedness, not the old "read up to
		// the first '?>'" shortcut (SPEC §5 well-formed XML).
		{"xml decl with no version", `<?xml?>`, 1, 1, "expected whitespace and version"},
		{"xml decl garbage attr", `<?xml foo bar?>`, 1, 11, `expected '=' after "foo"`},
		{"xml decl wrong attr name", `<?xml foo="1.0"?>`, 1, 1, "expected version"},
		{"xml decl bad version", `<?xml version="2.0"?>`, 1, 1, "unsupported version"},
		// VersionNum ::= '1.' [0-9]+ (XML 1.0 §2.8; round-2 finding 12).
		{"xml decl version 1.x", `<?xml version="1.x"?><app/>`, 1, 1, "unsupported version"},
		{"xml decl version 1.0a", `<?xml version="1.0a"?><app/>`, 1, 1, "unsupported version"},
		{"xml decl version 1. space", `<?xml version="1. "?><app/>`, 1, 1, "unsupported version"},
		{"xml decl version 1.-", `<?xml version="1.-"?><app/>`, 1, 1, "unsupported version"},
		{"xml decl version 1.", `<?xml version="1."?><app/>`, 1, 1, "unsupported version"},
		{"xml decl version 1.٣", `<?xml version="1.٣"?><app/>`, 1, 1, "unsupported version"},
		{"xml decl non-utf8 encoding", `<?xml version="1.0" encoding="Shift_JIS"?>`, 1, 1, "encoding must be UTF-8"},

		// ']]>' is never allowed in character data (XML 1.0 §2.4), even
		// inside <text>.
		{"cdata end marker in text", `<app><text>a]]>b</text></app>`, 1, 13, "']]>' is not allowed"},

		// A comment body may not contain "--" anywhere, including right
		// before the closing "-->" (which would make the sequence "--->").
		{"comment ends in --->", `<app><!-- a ---></app>`, 1, 6, "may not end with '--->'"},

		// A comment is still XML: a control character inside one is not
		// well-formed, even though "-- is not allowed" already rejects two
		// hyphens in a row.
		{"esc inside comment", "<app><!--\x1b--></app>", 1, 10, "U+001B"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root, _, _, err := parse.ParseXML([]byte(c.src))
			if err == nil {
				t.Fatalf("want a syntax error, got root %+v", root)
			}
			if err.Line != c.line || err.Col != c.col {
				t.Errorf("position %d:%d, want %d:%d (%s)", err.Line, err.Col, c.line, c.col, err.Msg)
			}
			if !strings.Contains(err.Msg, c.msg) {
				t.Errorf("message %q does not contain %q", err.Msg, c.msg)
			}
			if want := fmt.Sprintf("%d:%d: %s", err.Line, err.Col, err.Msg); err.Error() != want {
				t.Errorf("Error() = %q, want %q", err.Error(), want)
			}
			// The IR builder reports every syntax error as V005 at the same place.
			doc := parse.Parse([]byte(c.src), "x.tui")
			if len(doc.Diags) != 1 || doc.Diags[0].Code != "V005" || doc.Diags[0].Line != c.line || doc.Diags[0].Col != c.col {
				t.Errorf("Parse diags = %v, want one V005 at %d:%d", doc.Diags, c.line, c.col)
			}
			if doc.Root != nil {
				t.Errorf("a malformed document has no root")
			}
		})
	}
}

func TestParseXMLTree(t *testing.T) {
	src := "\ufeff<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n" +
		"<!-- lead -->\n" +
		"<app a=\"1\" on:click=\"go\" q=\"&quot;&amp;&apos;&lt;&gt;\">\n" +
		"  <text>x &lt; y</text><![CDATA[<raw & stuff>]]><!-- in -->\n" +
		"  <box/>\n" +
		"  <box></box>\n" +
		"</app>\n" +
		"<!-- tail -->\n"
	root, prolog, epilog, err := parse.ParseXML([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(prolog) != 1 || prolog[0].Type != parse.RawComment || prolog[0].Text != " lead " || prolog[0].Line != 2 {
		t.Errorf("prolog = %+v", prolog)
	}
	if len(epilog) != 1 || epilog[0].Text != " tail " || epilog[0].Line != 8 {
		t.Errorf("epilog = %+v", epilog)
	}
	if root.Name != "app" || root.Line != 3 || root.Col != 1 || root.SelfClosing {
		t.Fatalf("root = %+v", root)
	}
	wantAttrs := []parse.RawAttr{
		{Name: "a", Value: "1", Line: 3, Col: 6},
		{Name: "on:click", Value: "go", Line: 3, Col: 12},
		{Name: "q", Value: `"&'<>`, Line: 3, Col: 26},
	}
	if len(root.Attrs) != len(wantAttrs) {
		t.Fatalf("attrs = %+v", root.Attrs)
	}
	for i, a := range wantAttrs {
		if root.Attrs[i] != a {
			t.Errorf("attr %d = %+v, want %+v", i, root.Attrs[i], a)
		}
	}
	var kinds []string
	for _, c := range root.Children {
		switch c.Type {
		case parse.RawElement:
			kinds = append(kinds, "<"+c.Name+">")
		case parse.RawText:
			kinds = append(kinds, "text")
		case parse.RawComment:
			kinds = append(kinds, "comment")
		case parse.RawCData:
			kinds = append(kinds, "cdata")
		}
	}
	if got := strings.Join(kinds, " "); got != "text <text> cdata comment text <box> text <box> text" {
		t.Errorf("children = %s", got)
	}
	text := root.Children[1]
	if text.Line != 4 || text.Col != 3 || len(text.Children) != 1 || text.Children[0].Text != "x < y" {
		t.Errorf("text element = %+v / %+v", text, text.Children)
	}
	if cd := root.Children[2]; cd.Text != "<raw & stuff>" || cd.Line != 4 || cd.Col != 24 {
		t.Errorf("cdata = %+v", cd)
	}
	if !root.Children[5].SelfClosing || root.Children[7].SelfClosing {
		t.Error("SelfClosing must distinguish <box/> from <box></box>")
	}
}

func TestParseXMLAcceptsSubset(t *testing.T) {
	for _, src := range []string{
		`<app/>`,
		`<app />`,
		"<app\n  id=\"a\"\n  class=\"b c\"\n/>",
		`<app id = "a"></app >`,
		`<?xml version="1.0"?><app/>`,
		"<?xml version='1.0'?>\n<app/>",
		"\n\n  <app/>\n\n",
		`<app><![CDATA[]]></app>`,
		`<app><!----></app>`,
		`<app><text>&amp;&lt;&gt;&quot;&apos;</text></app>`,
		`<a-b.c_d/>`,
		// Any XML S (not just a literal space) may follow '<?xml', and a
		// declared UTF-8 encoding (any case) plus standalone are optional.
		"<?xml\tversion=\"1.0\" encoding=\"UTF-8\"?>\n<app/>",
		"<?xml\nversion=\"1.0\"?><app/>",
		`<?xml version="1.0" encoding="utf-8" standalone="yes"?><app/>`,
		`<?xml version="1.1"?><app/>`,
		`<?xml version="1.10"?><app/>`,
		`<?xml version="1.0123456789"?><app/>`,
		// Line ends: CRLF and a lone CR both count as LF (XML 1.0 §2.11).
		"<app>\r<text>x</text>\r</app>\r",
	} {
		if _, _, _, err := parse.ParseXML([]byte(src)); err != nil {
			t.Errorf("ParseXML(%q): %v", src, err)
		}
	}
}

// SPEC §5 asks for lowercase tag/attribute names, but that is an IR-builder
// rule (V001/V002 in build.go), not a tokenizer one: the tokenizer accepts
// any XML NameStartChar/NameChar run, so a non-ASCII name is a distinct
// tag/attribute rather than getting cut short mid-name and misreported.
func TestParseXMLAcceptsNonASCIINames(t *testing.T) {
	root, _, _, err := parse.ParseXML([]byte(`<tëxt tïtle="x">hi</tëxt>`))
	if err != nil {
		t.Fatalf("ParseXML: %v", err)
	}
	if root.Name != "tëxt" {
		t.Errorf("root.Name = %q, want %q", root.Name, "tëxt")
	}
	if len(root.Attrs) != 1 || root.Attrs[0].Name != "tïtle" {
		t.Errorf("attrs = %+v", root.Attrs)
	}
}

// A pathological depth must fail with a diagnostic (V005), not exhaust
// memory: Node.Path and RawNode both cost O(depth) per node, so an
// unbounded document is O(depth²) overall.
func TestParseXMLRejectsExcessiveNesting(t *testing.T) {
	open := strings.Repeat("<col>", 300)
	closeTags := strings.Repeat("</col>", 300)
	_, _, _, err := parse.ParseXML([]byte("<app>" + open + closeTags + "</app>"))
	if err == nil {
		t.Fatal("300 levels of nesting: want a SyntaxError")
	}
	if !strings.Contains(err.Msg, "nested deeper than 256 levels") {
		t.Errorf("err = %v", err)
	}
	// A reasonable depth still parses fine.
	shallow := strings.Repeat("<col>", 50) + strings.Repeat("</col>", 50)
	if _, _, _, err := parse.ParseXML([]byte("<app>" + shallow + "</app>")); err != nil {
		t.Errorf("50 levels of nesting: %v", err)
	}
}
