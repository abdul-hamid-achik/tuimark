package lsp

import (
	"path/filepath"
	"testing"
)

// v0.4.1: the text-level version completion filters by is the parser's: 3
// without a version attribute, 1 for any value other than 1, 2, or 3 and
// for the spike root.
func TestDocVersionOfMatchesTheParser(t *testing.T) {
	for _, c := range []struct {
		text string
		want int
	}{
		{"", 3},
		{"<tui>\n<screen/>\n</tui>", 3},
		{`<tui theme="auto"><screen/></tui>`, 3},
		{`<tui version="1"><screen/></tui>`, 1},
		{`<tui theme="dark" version="2">`, 2},
		{`<tui version="3">`, 3},
		{`<tui version="4">`, 1},
		{`<tui version="03">`, 1},
		{`<tui version="abc">`, 1},
		{`<tui version="">`, 1},
		{`<tui version=`, 1},
		{`<app width="100%">`, 1},
		{"<tui>\n<screen><text>version=\"2\"</text></screen>", 3},
	} {
		if got := docVersionOf(c.text); got != c.want {
			t.Errorf("docVersionOf(%q) = %d, want %d", c.text, got, c.want)
		}
	}
}

// v0.4.1: a document without version is offered the current vocabulary.
func TestCompletionWithoutVersionOffersCurrentTags(t *testing.T) {
	dir := writeTree(t, nil)
	c := startServer(t, Options{})
	c.initialize()
	const text = "<tui>\n<screen>\n<\n</screen>\n</tui>\n"
	uri := c.open(filepath.Join(dir, "app.tui"), "tui", text)
	items := completionLabels(t, c, uri, Position{2, 1})
	for _, tag := range []string{"table", "tabs", "sparkline", "hints", "box"} {
		if !items[tag] {
			t.Errorf("completion %v, missing %s", keys(items), tag)
		}
	}
	_ = c.shutdown()
}
