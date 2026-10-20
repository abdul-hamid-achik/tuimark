package lsp

import (
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// SPEC v0.4b §15.12: the version the parsed document's vocabulary is
// filtered by is the one the text-level reading gives: 4 for the current
// language, the written 1, 2, or 3, and 1 for any other value and for
// the spike root. A .tcss no document loads is offered the current
// language's vocabulary.
func TestDocVersionIsTheTextVersion(t *testing.T) {
	for _, c := range []struct {
		text string
		want int
	}{
		{`<tui><screen/></tui>`, 4},
		{`<tui theme="auto"><screen/></tui>`, 4},
		{`<tui version="1"><screen/></tui>`, 1},
		{`<tui version="2"><screen/></tui>`, 2},
		{`<tui version="3"><screen/></tui>`, 3},
		{`<tui version="4"><screen/></tui>`, 1},
		{`<tui version="abc"><screen/></tui>`, 1},
		// The parser takes double quotes only: a single-quoted version is
		// V005 and the document reads as 1, which the text reading agrees
		// with.
		{`<tui version='3'><screen/></tui>`, 1},
		{`<app><text>x</text></app>`, 1},
	} {
		if got := docVersion(parse.Parse([]byte(c.text), "t.tui")); got != c.want {
			t.Errorf("docVersion(%q) = %d, want %d", c.text, got, c.want)
		}
		if got := docVersionOf(c.text); got != c.want {
			t.Errorf("docVersionOf(%q) = %d, want %d", c.text, got, c.want)
		}
	}
	if got := loadersVersion(nil); got != 4 {
		t.Errorf("loadersVersion(nil) = %d, want 4", got)
	}
}
