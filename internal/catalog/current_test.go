package catalog

import (
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// SPEC v0.4b §15.10: current is the language of a <tui> without version,
// a number no version value accepts; keys.rejected_since is the language
// whose keymaps reject the never-matching tokens.
func TestCurrentAndRejectedSince(t *testing.T) {
	c := Build("x")
	if c.Current != 4 || c.Keys.RejectedSince != 4 {
		t.Errorf("current %d, rejected_since %d; want 4 and 4", c.Current, c.Keys.RejectedSince)
	}
	if d := parse.Parse([]byte(`<tui><screen id="s"/></tui>`), "t.tui"); int(d.Lang) != c.Current {
		t.Errorf("a <tui> without version is Lang %d, current %d", d.Lang, c.Current)
	}
	for _, v := range c.Versions {
		if v == c.Current {
			t.Errorf("current %d is listed in versions %v", c.Current, c.Versions)
		}
	}
}

// Items only the current language accepts have since current. F0 has none;
// the test adds a tag and an attribute for its duration.
func TestCurrentItemsHaveSinceCurrent(t *testing.T) {
	kinds, attrs := ir.KindsCurrent, parse.TagAttrsCurrent
	t.Cleanup(func() { ir.KindsCurrent, parse.TagAttrsCurrent = kinds, attrs })
	ir.KindsCurrent = []string{"checkbox"}
	parse.TagAttrsCurrent = map[string]map[string]bool{"checkbox": {"id": true}, "list": {"single": true}}
	c := Build("x")
	got := map[string]int{}
	for _, tg := range c.Tags {
		got[tg.Name] = tg.Since
		for _, a := range tg.Attrs {
			got[tg.Name+"."+a.Name] = a.Since
		}
	}
	for name, want := range map[string]int{"checkbox": 4, "checkbox.id": 4, "list.single": 4, "list.checked": 2, "list": 1} {
		if got[name] != want {
			t.Errorf("%s: since %d, want %d", name, got[name], want)
		}
	}
}
