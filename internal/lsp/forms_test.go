package lsp

import (
	"path/filepath"
	"strings"
	"testing"
)

// SPEC v0.4b §6.14.1, §6.16, §6.17, §15.12 (tests 131 and 188): the
// checkbox, the select, and single are current-language vocabulary, so
// the language server offers them, and single's values true and false, in
// a <tui> without version and in no pinned document; with an invalid
// version, and under the spike root <app>, only the version 1 tags.
func TestCompletionOffersTheFormControlsInTheCurrentLanguage(t *testing.T) {
	dir := writeTree(t, nil)
	app := filepath.Join(dir, "app.tui")
	c := startServer(t, Options{})
	c.initialize()
	for _, tc := range []completionCase{
		{name: "tag", path: app, lang: "tuimark", text: `<tui><screen id="s"><che|`,
			want: []string{"checkbox"}},
		{name: "tag after <", path: app, lang: "tuimark", text: `<tui><screen id="s"><|`,
			want: []string{"checkbox", "select", "button", "table"}},
		{name: "tag in version 3", path: app, lang: "tuimark", text: `<tui version="3"><screen id="s"><che|`,
			absent: []string{"checkbox"}},
		{name: "tags in version 3", path: app, lang: "tuimark", text: `<tui version="3"><screen id="s"><|`,
			want: []string{"button", "table"}, absent: []string{"checkbox", "select"}},
		{name: "tag in version 1", path: app, lang: "tuimark", text: `<tui version="1"><screen id="s"><|`,
			want: []string{"button"}, absent: []string{"checkbox"}},
		{name: "tags in version abc", path: app, lang: "tuimark", text: `<tui version="abc"><screen id="s"><|`,
			want: []string{"button", "list"}, absent: []string{"checkbox", "select", "table", "tabs"}},
		{name: "tags under app", path: app, lang: "tuimark", text: `<app><|`,
			want: []string{"button", "list"}, absent: []string{"checkbox", "select", "table", "tabs"}},
		{name: "select attributes", path: app, lang: "tuimark", text: `<tui><screen id="s"><select |`,
			want: []string{"bind", "each", "key", "label", "placeholder", "on:change", "id"}, absent: []string{"on:click", "checked"}},
		{name: "checkbox attributes", path: app, lang: "tuimark", text: `<tui><screen id="s"><checkbox |`,
			want: []string{"bind", "label", "on:change", "id"}, absent: []string{"on:click", "each"}},
		{name: "single on list", path: app, lang: "tuimark", text: `<tui><screen id="s"><list |`,
			want: []string{"single", "checked"}},
		{name: "single on table", path: app, lang: "tuimark", text: `<tui><screen id="s"><table |`,
			want: []string{"single"}},
		{name: "single values", path: app, lang: "tuimark", text: `<tui><screen id="s"><list single="|`,
			want: []string{"true", "false"}},
		{name: "single in version 3", path: app, lang: "tuimark", text: `<tui version="3"><screen id="s"><list |`,
			want: []string{"checked"}, absent: []string{"single"}},
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
