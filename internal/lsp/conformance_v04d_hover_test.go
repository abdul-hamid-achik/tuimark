package lsp

import (
	"path/filepath"
	"strings"
	"testing"
)

// Hover names runs, pre-wrap, and span (SPEC v0.4d §6.20.6).
func TestV04dLSPHover(t *testing.T) {
	cur := "<tui>\n  <style src=\"t.tcss\"/>\n  <screen id=\"main\"><text runs=\"r\" wrap=\"pre-wrap\"/></screen>\n</tui>\n"
	sheet := "text > span { bold: true; }\ntext { wrap: pre-wrap; }\n"
	dir := writeTree(t, map[string]string{"app.tui": cur, "t.tcss": sheet})
	c := startServer(t, Options{})
	c.initialize()
	uri := c.open(filepath.Join(dir, "app.tui"), "tuimark", cur)
	suri := c.open(filepath.Join(dir, "t.tcss"), "tcss", sheet)
	at := func(text, mark string, skip int) Position {
		i := strings.Index(text, mark)
		if i < 0 {
			t.Fatalf("no %q", mark)
		}
		return positionAt(text, i+skip)
	}
	for _, tc := range []struct {
		name string
		uri  string
		pos  Position
		want string
	}{
		{"the runs attribute", uri, at(cur, `runs="r"`, 1), "`runs` — attribute of `<text>`"},
		{"span in a selector", suri, at(sheet, "> span", 3), "`span` — type generated from"},
		{"the pre-wrap value", suri, at(sheet, `wrap: pre-wrap`, 8), "`pre-wrap` — value of `wrap`"},
	} {
		if got := hoverAt(t, c, tc.uri, tc.pos); !strings.Contains(got, tc.want) {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
	_ = c.shutdown()
}
