package parse_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/agentsdoc"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// seedDocs returns every real .tui document in the repository (examples,
// the kit, test data), for the fuzz corpora.
func seedDocs() []string {
	var out []string
	for _, pat := range []string{
		"../../examples/*/*.tui", "../../examples/*/*/*.tui", "../../examples/*/*/*/*.tui",
		"../../kit/*.tui", "../../kit/*/*.tui",
		"../../testdata/*/*.tui", "../../testdata/*/*/*.tui", "../../testdata/*/*/*/*.tui",
	} {
		paths, _ := filepath.Glob(pat)
		for _, p := range paths {
			if b, err := os.ReadFile(p); err == nil && len(b) <= 8<<10 {
				out = append(out, string(b))
			}
		}
	}
	return out
}

// edgeDocs are hand-written inputs for the shapes fuzzing rarely reaches
// from the real documents alone.
func edgeDocs() []string {
	deep := strings.Repeat("<box>", 300) + strings.Repeat("</box>", 300)
	return []string{
		"",
		string(rune(0xFEFF)) + "<tui><screen/></tui>",
		"\xef\xbb\xbf<?xml version=\"1.0\"?><tui><screen/></tui>",
		"<tui><screen>" + deep + "</screen></tui>",
		"<tui><screen><text>" + strings.Repeat("a", 4000) + "</text></screen></tui>",
		"<tui><screen><text id=\"" + strings.Repeat("x", 4000) + "\">{" + strings.Repeat("a.", 500) + "b}</text></screen></tui>",
		"<tui><screen><text>a\x00b\x1b[31mc\x7fd\u0085e" + string(rune(0x202e)) + "f</text></screen></tui>",
		"<tui><screen><text>\xff\xfe\xc3</text></screen></tui>",
		"<tui><screen><text><![CDATA[<b>&amp;]]></text></screen></tui>",
		"<tui><screen><text>&amp;&lt;&gt;&quot;&apos;&#65;&#x41;&bogus;&#0;&#x110000;</text></screen></tui>",
		"<!DOCTYPE tui [<!ENTITY x \"y\">]><tui><screen><text>&x;</text></screen></tui>",
		"<tui><!-- c --><?pi x?><screen/><!-- tail --></tui><!-- after -->",
		"<tui version=\"9\" theme=\"auto\" mouse=\"!m\"><screen/></tui>",
		"<app><col><text>x</text></col></app>",
		"<tui><screen><list id=\"l\" each=\"a  as  b\" key=\"b.id\"><item><text>{b}</text></item></list></screen></tui>",
		"<tui><keymap when=\"#a\"><bind keys=\",,ctrl+i,ctrl+,\" action=\"x\"/></keymap><keymap/><screen/></tui>",
		"<tui><screen><table id=\"t\" each=\"r as r\"><column title=\"{r}\">{r}</column></table></screen></tui>",
		"<tui>\n\n\t<screen>\r\n<text\n  id=\"a\"\n  id=\"a\"\n>x</text></screen></tui>",
		"<<<>>>&&&\"'",
		"<tui><screen><text style=\"color: red; wrap: truncate-middle; width: 1fr\"/></screen></tui>",
	}
}

// FuzzParse checks the invariants of the parser entry the CLI uses, for any
// byte string: no panic; every diagnostic has a code from the diagnostic
// table and a position inside the input; the result (diagnostics and IR)
// is the same when the same bytes are parsed again.
func FuzzParse(f *testing.F) {
	for _, s := range seedDocs() {
		f.Add(s)
	}
	for _, s := range edgeDocs() {
		f.Add(s)
	}
	known := map[string]bool{}
	for _, d := range agentsdoc.DiagCodes() {
		known[d.Code] = true
	}
	f.Fuzz(func(t *testing.T, src string) {
		if len(src) > 16<<10 {
			t.Skip()
		}
		doc := parse.Parse([]byte(src), "")
		lines := strings.Split(src, "\n")
		for _, d := range doc.Diags {
			if !known[d.Code] {
				t.Fatalf("unknown diagnostic code %q: %+v", d.Code, d)
			}
			if d.Severity != ir.Error && d.Severity != ir.Warning {
				t.Fatalf("bad severity %q: %+v", d.Severity, d)
			}
			if d.Line < 1 || d.Line > len(lines) || d.Col < 1 {
				t.Fatalf("diagnostic position out of the input (%d lines): %+v", len(lines), d)
			}
		}
		again := parse.Parse([]byte(src), "")
		if !reflect.DeepEqual(doc.Diags, again.Diags) {
			t.Fatalf("diagnostics differ between two parses:\n%+v\n%+v", doc.Diags, again.Diags)
		}
		a, err1 := json.Marshal(parse.BuildIR(doc, nil))
		b, err2 := json.Marshal(parse.BuildIR(again, nil))
		if err1 != nil || err2 != nil {
			t.Fatalf("IR does not marshal: %v %v", err1, err2)
		}
		if string(a) != string(b) {
			t.Fatalf("IR differs between two parses:\n%s\n%s", a, b)
		}
	})
}
