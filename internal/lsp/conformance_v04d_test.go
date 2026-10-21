package lsp

import (
	"path/filepath"
	"strings"
	"testing"
)

// SPEC v0.4d §6.20.6 (decision 212): the language server completes runs
// on text, span in selectors, and pre-wrap, each only for a document in
// the current language.
func TestV04dLSPRunsAndPreWrap(t *testing.T) {
	cur := "<tui>\n  <style src=\"t.tcss\"/>\n  <screen id=\"main\"><text id=\"r\" runs=\"msg.runs\"/></screen>\n</tui>\n"
	pinned := strings.Replace(cur, "<tui>", `<tui version="3">`, 1)
	dir := writeTree(t, map[string]string{"app.tui": cur, "t.tcss": "text { }\n"})
	sheet := filepath.Join(dir, "t.tcss")
	app := filepath.Join(dir, "app.tui")
	c := startServer(t, Options{})
	c.initialize()
	c.open(app, "tuimark", cur)
	for _, tc := range []completionCase{
		{name: "runs on a text", path: filepath.Join(dir, "x.tui"), lang: "tuimark", text: `<tui><screen id="s"><text |`,
			want: []string{"runs", "wrap"}},
		{name: "runs is a current-language attribute", path: filepath.Join(dir, "y.tui"), lang: "tuimark", text: `<tui version="3"><screen id="s"><text |`,
			want: []string{"wrap"}, absent: []string{"runs"}},
		{name: "runs on a box is no attribute", path: filepath.Join(dir, "z.tui"), lang: "tuimark", text: `<tui><screen id="s"><box |`,
			absent: []string{"runs"}},
		{name: "pre-wrap as a text attribute", path: filepath.Join(dir, "w.tui"), lang: "tuimark", text: `<tui><screen id="s"><text wrap="|`,
			want: []string{"pre-wrap", "wrap", "nowrap", "truncate"}},
		{name: "pre-wrap is not offered to version 3", path: filepath.Join(dir, "v.tui"), lang: "tuimark", text: `<tui version="3"><screen id="s"><text wrap="|`,
			want: []string{"truncate-middle"}, absent: []string{"pre-wrap"}},
		{name: "runs offers paths", path: filepath.Join(dir, "p.tui"), lang: "tuimark", text: `<tui><screen id="s"><text runs="|`,
			want: []string{}},
		{name: "pre-wrap in a stylesheet", path: sheet, lang: "tcss", text: "text { wrap: |",
			want: []string{"pre-wrap", "wrap", "nowrap"}},
		{name: "span in a selector", path: sheet, lang: "tcss", text: "text > sp|",
			want: []string{"span"}},
	} {
		text := strings.Replace(tc.text, "|", "", 1)
		uri := c.open(tc.path, tc.lang, text)
		c.change(uri, 2, text)
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

	// A stylesheet loaded by a pinned document offers neither pre-wrap nor span.
	pdir := writeTree(t, map[string]string{"app.tui": pinned, "t.tcss": "text { }\n"})
	psheet := filepath.Join(pdir, "t.tcss")
	c.open(filepath.Join(pdir, "app.tui"), "tuimark", pinned)
	for _, tc := range []completionCase{
		{name: "pinned: no pre-wrap", path: psheet, lang: "tcss", text: "text { wrap: |",
			want: []string{"wrap"}, absent: []string{"pre-wrap"}},
		{name: "pinned: no span", path: psheet, lang: "tcss", text: "text > sp|",
			absent: []string{"span"}},
	} {
		text := strings.Replace(tc.text, "|", "", 1)
		uri := c.open(tc.path, tc.lang, text)
		c.change(uri, 2, text)
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
