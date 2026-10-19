package lsp

import (
	"path/filepath"
	"strings"
	"testing"
)

// SPEC v0.4b §6.4.1, §15.12: indeterminate is current-language
// vocabulary, so the language server offers it, and its values true and
// false, in a <tui> without version and in no pinned document.
func TestCompletionOffersIndeterminateInTheCurrentLanguage(t *testing.T) {
	dir := writeTree(t, nil)
	app := filepath.Join(dir, "app.tui")
	c := startServer(t, Options{})
	c.initialize()
	for _, tc := range []completionCase{
		{name: "attribute", path: app, lang: "tuimark", text: `<tui><screen id="s"><progress |`,
			want: []string{"indeterminate", "value", "bind"}},
		{name: "attribute while typing", path: app, lang: "tuimark", text: `<tui><screen id="s"><progress inde|`,
			want: []string{"indeterminate"}},
		{name: "values", path: app, lang: "tuimark", text: `<tui><screen id="s"><progress indeterminate="|`,
			want: []string{"true", "false"}},
		{name: "not on other tags", path: app, lang: "tuimark", text: `<tui><screen id="s"><box |`,
			absent: []string{"indeterminate"}},
		{name: "not in version 3", path: app, lang: "tuimark", text: `<tui version="3"><screen id="s"><progress |`,
			want: []string{"value"}, absent: []string{"indeterminate"}},
		{name: "not in version 1", path: app, lang: "tuimark", text: `<tui version="1"><screen id="s"><progress |`,
			want: []string{"value"}, absent: []string{"indeterminate"}},
	} {
		text := strings.Replace(tc.text, "|", "", 1)
		uri := c.open(tc.path, tc.lang, text)
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
