package lsp

import (
	"path/filepath"
	"strings"
	"testing"
)

// SPEC v0.4b §6.18.3, §15.12 (phase F2): <template> and <use> are
// current-language vocabulary, so the language server offers them, with
// their attributes, in a <tui> without version and in no pinned document.
func TestCompletionOffersTemplateAndUseInTheCurrentLanguage(t *testing.T) {
	dir := writeTree(t, nil)
	app := filepath.Join(dir, "app.tui")
	c := startServer(t, Options{})
	c.initialize()
	for _, tc := range []completionCase{
		{name: "tags under tui", path: app, lang: "tuimark", text: `<tui><|`,
			want: []string{"template", "include", "screen"}},
		{name: "tags under screen", path: app, lang: "tuimark", text: `<tui><screen id="s"><|`,
			want: []string{"use", "template", "button"}},
		{name: "tags in version 3", path: app, lang: "tuimark", text: `<tui version="3"><screen id="s"><|`,
			want: []string{"button"}, absent: []string{"template", "use"}},
		{name: "use attributes", path: app, lang: "tuimark", text: `<tui><screen id="s"><use |`,
			want: []string{"id", "template", "with", "if", "hidden", "class"}, absent: []string{"style", "disabled", "on:click", "width"}},
		{name: "template attributes", path: app, lang: "tuimark", text: `<tui><template |`,
			want: []string{"id"}, absent: []string{"class", "style", "if"}},
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
