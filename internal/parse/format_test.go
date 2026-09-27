package parse

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// messyInputs are hand-written, well-formed-but-awkward documents: attribute
// order, ragged multi-line text, comments at every position, entity
// escaping, and an inline <style> body. fmt only requires well-formed XML,
// so these need not be IR-valid.
var messyInputs = map[string]string{
	"attr_order": `<app><box width="10" class="c" border="1" id="a"/></app>`,

	"multiline_text": "<app>\n" +
		"  <col id=\"root\">\n" +
		"        <text>\n" +
		"      line one\n" +
		"\n" +
		"          line two\n" +
		"    </text>\n" +
		"  </col>\n" +
		"</app>\n",

	"comments": "<!-- top -->\n" +
		"<app>\n" +
		"  <!-- inside -->\n" +
		"  <box id=\"a\"/>\n" +
		"  <!-- trailing -->\n" +
		"</app>\n" +
		"<!-- bottom -->\n",

	"escaping": `<app><box id="a" title="a &lt;b&gt; &amp; &quot;c&quot;"/></app>`,

	"already_selfclosing": "<app>\n  <box id=\"a\"/>\n  <box id=\"b\"></box>\n</app>\n",

	"inline_style": "<tui version=\"1\">\n" +
		"  <style>\n" +
		"    screen { color: $fg; }\n" +
		"\n" +
		"    .title { bold: true; }\n" +
		"  </style>\n" +
		"  <screen id=\"main\"><box/></screen>\n" +
		"</tui>\n",

	"button_label": `<tui version="1"><screen><button id="b">click me</button></screen></tui>`,

	"style_with_entities": "<tui version=\"1\">\n" +
		"  <style>\n" +
		"    /* header &amp; footer */\n" +
		"    #a { bold: true; }\n" +
		"  </style>\n" +
		"  <screen><box id=\"a\"/></screen>\n" +
		"</tui>\n",

	"empty_text": "<app>\n  <text>\n\n  </text>\n</app>\n",

	"nested_rows": "<app>\n" +
		"  <row id=\"r\" width=\"1fr\" gap=\"1\">\n" +
		"    <box id=\"a\" border=\"1\"><text>A</text></box>\n" +
		"    <box id=\"b\" border=\"1\"><text>B</text></box>\n" +
		"  </row>\n" +
		"</app>\n",
}

func allTuiSources(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, f := range allExampleFiles(t) {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		out[f] = string(src)
	}
	for name, src := range messyInputs {
		out["messy:"+name] = src
	}
	return out
}

func TestFormatIdempotent(t *testing.T) {
	for name, src := range allTuiSources(t) {
		name, src := name, src
		t.Run(name, func(t *testing.T) {
			once, err := Format([]byte(src))
			if err != nil {
				t.Fatalf("Format: %v", err)
			}
			twice, err := Format([]byte(once))
			if err != nil {
				t.Fatalf("Format(Format(x)): %v", err)
			}
			if once != twice {
				t.Errorf("not idempotent:\n--- once ---\n%s\n--- twice ---\n%s", once, twice)
			}
			if !strings.HasSuffix(once, "\n") || strings.HasSuffix(once, "\n\n") {
				t.Errorf("output must end with exactly one newline, got %q at end", once[max(0, len(once)-5):])
			}
		})
	}
}

func TestFormatPreservesSemantics(t *testing.T) {
	for name, src := range allTuiSources(t) {
		name, src := name, src
		t.Run(name, func(t *testing.T) {
			formatted, err := Format([]byte(src))
			if err != nil {
				t.Fatalf("Format: %v", err)
			}
			origDoc := Parse([]byte(src), "")
			fmtDoc := Parse([]byte(formatted), "")
			origIR := BuildIR(origDoc, map[string]string{})
			fmtIR := BuildIR(fmtDoc, map[string]string{})
			ob, err := json.Marshal(origIR)
			if err != nil {
				t.Fatal(err)
			}
			fb, err := json.Marshal(fmtIR)
			if err != nil {
				t.Fatal(err)
			}
			if string(ob) != string(fb) {
				t.Errorf("IR changed by formatting:\n--- original ---\n%s\n--- formatted ---\n%s\n--- original IR ---\n%s\n--- formatted IR ---\n%s", src, formatted, ob, fb)
			}
		})
	}
}

func TestFormatKeepsComments(t *testing.T) {
	out, err := Format([]byte(messyInputs["comments"]))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<!-- top -->", "<!-- inside -->", "<!-- trailing -->", "<!-- bottom -->"} {
		if !strings.Contains(out, want) {
			t.Errorf("formatted output missing %q:\n%s", want, out)
		}
	}
}

func TestFormatAttributeOrder(t *testing.T) {
	out, err := Format([]byte(messyInputs["attr_order"]))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("want 3 lines, got %d:\n%s", len(lines), out)
	}
	want := `<box id="a" class="c" width="10" border="1"/>`
	if got := strings.TrimSpace(lines[1]); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatSelfClosesEmptyElements(t *testing.T) {
	out, err := Format([]byte(messyInputs["already_selfclosing"]))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "</box>") {
		t.Errorf("empty <box></box> should self-close:\n%s", out)
	}
	if got := strings.Count(out, "/>"); got != 2 {
		t.Errorf("want 2 self-closing tags, got %d:\n%s", got, out)
	}
}

func TestFormatInlineTextStaysOnOneLine(t *testing.T) {
	out, err := Format([]byte(`<app><text>hi</text></app>`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "<text>hi</text>") {
		t.Errorf("single-line text body should stay inline:\n%s", out)
	}
}

func TestFormatMultilineTextIsIndentedOneLevelPerLine(t *testing.T) {
	out, err := Format([]byte(messyInputs["multiline_text"]))
	if err != nil {
		t.Fatal(err)
	}
	want := "    <text>\n      line one\n\n      line two\n    </text>\n"
	if !strings.Contains(out, want) {
		t.Errorf("want multi-line text block:\n%s\n--- got ---\n%s", want, out)
	}
}

// A <style> body decoded from an entity (&amp;) or read raw from a CDATA
// section can contain '&' or '<'; fmt must re-escape them (SPEC §5: MUST
// be well-formed XML), not write them back raw the way writeTextBody
// already escapes (via textEscaper) but writeStyleBody used not to.
//
// The CDATA case is a local fixture rather than another messyInputs entry:
// writeStyleBody always reformats a style body onto its own indented
// lines, so a single-line CDATA source and its multi-line fmt output
// differ in surrounding whitespace even once escaping is correct — a
// separate, pre-existing gap in how well TestFormatPreservesSemantics'
// exact-IR comparison tolerates single-line style bodies, not something
// this fix changes either way.
func TestFormatEscapesStyleBody(t *testing.T) {
	cases := map[string]string{
		"style_with_entities": messyInputs["style_with_entities"],
		"style_with_cdata": "<tui version=\"1\">\n" +
			"  <style><![CDATA[ /* a < b & c */ #a { bold: true; } ]]></style>\n" +
			"  <screen><box id=\"a\"/></screen>\n" +
			"</tui>\n",
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			out, err := Format([]byte(src))
			if err != nil {
				t.Fatalf("Format: %v", err)
			}
			if _, _, _, perr := ParseXML([]byte(out)); perr != nil {
				t.Fatalf("fmt produced ill-formed XML: %v\n--- output ---\n%s", perr, out)
			}
			origDoc := Parse([]byte(src), "")
			fmtDoc := Parse([]byte(out), "")
			if len(origDoc.Diags) != 0 || len(fmtDoc.Diags) != 0 {
				t.Fatalf("diags: orig=%v fmt=%v", origDoc.Diags, fmtDoc.Diags)
			}
			origBody := origDoc.Styles[0].Body
			fmtBody := fmtDoc.Styles[0].Body
			if !strings.Contains(origBody, "a < b & c") && !strings.Contains(origBody, "header & footer") {
				t.Fatalf("test setup: original body = %q", origBody)
			}
			// writeStyleBody always reformats whitespace/indentation (that
			// part is unrelated to this fix and not meant to be
			// byte-identical); what must survive round-tripping through a
			// well-formed document is the actual characters.
			squeeze := func(s string) string { return strings.Join(strings.Fields(s), " ") }
			if squeeze(fmtBody) != squeeze(origBody) {
				t.Errorf("fmt changed the decoded style body:\n--- original ---\n%q\n--- formatted ---\n%q", origBody, fmtBody)
			}
		})
	}
}

func TestFormatRejectsIllFormedXML(t *testing.T) {
	_, err := Format([]byte(`<app><box></app>`))
	if err == nil {
		t.Fatal("want an error for unclosed XML (V005)")
	}
	if _, ok := err.(*SyntaxError); !ok {
		t.Fatalf("want *SyntaxError, got %T", err)
	}
}
