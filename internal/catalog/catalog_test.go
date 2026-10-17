package catalog

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// versions lists exactly the document versions the parser accepts: each
// one parses without a V003 on `version`, and the next one is rejected.
func TestVersionsAreTheParsers(t *testing.T) {
	doc := func(v int) *parse.Document {
		return parse.Parse([]byte(fmt.Sprintf(`<tui version="%d"><screen id="s"/></tui>`, v)), "t.tui")
	}
	for _, v := range Versions {
		if d := doc(v); d.Diags.HasErrors() {
			t.Errorf("version %d: %v", v, d.Diags)
		}
	}
	if d := doc(Versions[len(Versions)-1] + 1); !d.Diags.HasErrors() {
		t.Errorf("version %d is accepted, but the catalog does not list it", Versions[len(Versions)-1]+1)
	}
}

func TestBuildIsDeterministicAndIndependent(t *testing.T) {
	a, b := Build("9.9.9"), Build("9.9.9")
	if !reflect.DeepEqual(a, b) {
		t.Fatal("Build is not deterministic")
	}
	if a.Catalog != Format || a.Tuimark != "9.9.9" {
		t.Errorf("catalog %d, tuimark %q", a.Catalog, a.Tuimark)
	}
	// A caller may edit what it got without touching the next build.
	a.Versions[0] = 99
	a.Keys.Named[0] = "x"
	a.Tags[0].Name = "x"
	if c := Build("9.9.9"); !reflect.DeepEqual(c, b) {
		t.Error("Build shares state with its result")
	}
}

// Every tag, attribute, and property keeps a non-nil keywords list, so the
// JSON never says null.
func TestKeywordListsAreNeverNil(t *testing.T) {
	c := Build("x")
	for _, tg := range c.Tags {
		for _, a := range tg.Attrs {
			if a.Keywords == nil {
				t.Errorf("%s.%s: keywords is nil", tg.Name, a.Name)
			}
		}
	}
	for _, p := range c.Properties {
		if p.Keywords == nil {
			t.Errorf("property %s: keywords is nil", p.Name)
		}
	}
	for _, m := range c.Media {
		if m.Keywords == nil {
			t.Errorf("media %s: keywords is nil", m.Name)
		}
	}
}
