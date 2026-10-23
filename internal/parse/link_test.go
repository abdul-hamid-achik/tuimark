package parse_test

import (
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// findText walks the expanded tree for the first <text> element.
func findText(n *ir.Node) *ir.Node {
	if n == nil {
		return nil
	}
	if n.Kind == "text" {
		return n
	}
	for _, c := range n.Children {
		if t := findText(c); t != nil {
			return t
		}
	}
	return nil
}

func TestHrefAttributeParsed(t *testing.T) {
	doc := parse.Parse([]byte(`<tui><screen><text href="https://example.com/a?x=1">hi</text></screen></tui>`), "a.tui")
	if doc.Diags.HasErrors() {
		t.Fatalf("diags: %v", doc.Diags)
	}
	n := findText(doc.Root)
	if n == nil {
		t.Fatal("no <text> in the document")
	}
	if got := n.Attrs["href"]; got != "https://example.com/a?x=1" {
		t.Errorf("href = %q", got)
	}
	// A control character in the URI is V007, stripped like text.
	doc = parse.Parse([]byte("<tui><screen><text href=\"a\x1b[0mb\">hi</text></screen></tui>"), "a.tui")
	n = findText(doc.Root)
	if got := n.Attrs["href"]; got != "ab" {
		t.Errorf("sanitized href = %q, want ab", got)
	}
	if !doc.Diags.HasErrors() || doc.Diags[0].Code != "V007" {
		t.Errorf("diags = %v, want one V007", doc.Diags)
	}
}

func TestHrefNeedsCurrentLanguage(t *testing.T) {
	doc := parse.Parse([]byte(`<tui version="1"><screen><text href="https://x">hi</text></screen></tui>`), "a.tui")
	if !doc.Diags.HasErrors() || doc.Diags[0].Code != "V002" {
		t.Fatalf("diags = %v, want V002", doc.Diags)
	}
	if !strings.Contains(doc.Diags[0].Msg, "href") || !strings.Contains(doc.Diags[0].Msg, "without version") {
		t.Errorf("V002 message = %q", doc.Diags[0].Msg)
	}
	// And it is an attribute of <text> only, even in the current language.
	doc = parse.Parse([]byte(`<tui><screen><box href="https://x"/></screen></tui>`), "a.tui")
	if !doc.Diags.HasErrors() || doc.Diags[0].Code != "V002" {
		t.Fatalf("diags = %v, want V002", doc.Diags)
	}
}
