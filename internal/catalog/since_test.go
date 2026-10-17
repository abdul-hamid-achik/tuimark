package catalog

import "testing"

// A version is never listed lower than what it belongs to: a keyword is
// not older than its attribute, an attribute not older than its tag, and a
// property keyword not older than its property (SPEC v0.4a §15.10, "since
// is the lowest document version that accepts it").
func TestSinceNeverBelowItsOwner(t *testing.T) {
	c := Build("x")
	for _, tg := range c.Tags {
		for _, a := range tg.Attrs {
			if a.Since < tg.Since {
				t.Errorf("%s.%s: since %d < tag since %d", tg.Name, a.Name, a.Since, tg.Since)
			}
			for _, k := range a.Keywords {
				if k.Since < a.Since {
					t.Errorf("%s.%s=%s: since %d < attribute since %d", tg.Name, a.Name, k.Name, k.Since, a.Since)
				}
			}
		}
	}
	for _, p := range c.Properties {
		for _, k := range p.Keywords {
			if k.Since < p.Since {
				t.Errorf("property %s: %s since %d < property since %d", p.Name, k.Name, k.Since, p.Since)
			}
		}
	}
	for _, m := range c.Media {
		for _, k := range m.Keywords {
			if k.Since < m.Since {
				t.Errorf("media %s: %s since %d < feature since %d", m.Name, k.Name, k.Since, m.Since)
			}
		}
	}
}

// The case that motivated the rule: border on a version="2" tag has the
// attribute's since, and on a version="1" tag its own.
func TestBorderKeywordsFollowTheirTag(t *testing.T) {
	c := Build("x")
	want := map[string]int{"tab": 2, "table": 2, "box": 1}
	seen := map[string]bool{}
	for _, tg := range c.Tags {
		w, ok := want[tg.Name]
		if !ok {
			continue
		}
		for _, a := range tg.Attrs {
			if a.Name != "border" {
				continue
			}
			seen[tg.Name] = true
			if a.Since != w || len(a.Keywords) == 0 {
				t.Errorf("%s.border: since %d with %d keywords, want since %d", tg.Name, a.Since, len(a.Keywords), w)
			}
			for _, k := range a.Keywords {
				if k.Since != w {
					t.Errorf("%s.border=%s since %d, want %d", tg.Name, k.Name, k.Since, w)
				}
			}
		}
	}
	if len(seen) != len(want) {
		t.Errorf("border seen on %v, want %v", seen, want)
	}
}
