package css

import (
	"fmt"
	"testing"
)

// versionFlags maps a document version to CheckDeclIn's v2 and v3 flags.
func versionFlags(v int) (v2, v3 bool) { return v >= 2, v >= 3 }

// TestCatalogAccessorsMatchTheParser checks that what the catalog accessors
// report is what the parser accepts, at the version they report and not
// below it (SPEC v0.4a §15.10, test 119).
func TestCatalogAccessorsMatchTheParser(t *testing.T) {
	for _, name := range PropertyNames() {
		since := PropertySince(name)
		if since < 1 || since > 3 {
			t.Fatalf("PropertySince(%q) = %d", name, since)
		}
		for _, k := range PropertyKeywords(name) {
			for v := 1; v <= 3; v++ {
				v2, v3 := versionFlags(v)
				err := CheckDeclIn(name, k.Name, v2, v3)
				if v >= k.Since && v >= since && err != nil {
					t.Errorf("%s: %s at version %d: %v", name, k.Name, v, err)
				}
				if (v < k.Since || v < since) && err == nil {
					t.Errorf("%s: %s accepted at version %d, before its since %d", name, k.Name, v, k.Since)
				}
			}
		}
	}
	if got := PropertySince("wrap"); got != 1 {
		t.Errorf("wrap since = %d, want 1", got)
	}
	if got := PropertySince("row-gap"); got != 3 {
		t.Errorf("row-gap since = %d, want 3", got)
	}
	if got := PropertySince("grid-columns"); got != 2 {
		t.Errorf("grid-columns since = %d, want 2", got)
	}
	if PropertyKeywords("width") != nil || PropertyKeywords("color") != nil {
		t.Error("a size or color property has keywords")
	}
}

func TestPseudoClassesMatchTheParser(t *testing.T) {
	seen := map[string]int{}
	for _, p := range PseudoClasses() {
		seen[p.Name] = p.Since
		for v := 1; v <= 3; v++ {
			_, err := ParseSelectorIn("list:"+p.Name, v >= 2)
			if v >= p.Since && err != nil {
				t.Errorf(":%s at version %d: %v", p.Name, v, err)
			}
			if v < p.Since && err == nil {
				t.Errorf(":%s accepted at version %d, before its since %d", p.Name, v, p.Since)
			}
		}
	}
	for _, want := range []string{"focus", "selected", "disabled", "empty", "checked", "focus-within"} {
		if _, ok := seen[want]; !ok {
			t.Errorf("pseudo-class :%s missing", want)
		}
	}
	if _, err := ParseSelectorIn("list:hover", true); err == nil {
		t.Error(":hover accepted")
	}
}

func TestMediaFeaturesMatchTheParser(t *testing.T) {
	names := map[string]bool{}
	for _, f := range MediaFeatures() {
		names[f.Name] = true
		check := func(cond string, since int) {
			for v := 1; v <= 3; v++ {
				_, err := parseMedia("@media "+cond, v >= 2)
				if v >= since && err != nil {
					t.Errorf("%s at version %d: %v", cond, v, err)
				}
				if v < since && err == nil {
					t.Errorf("%s accepted at version %d, before its since %d", cond, v, since)
				}
			}
		}
		if len(f.Keywords) == 0 {
			check(fmt.Sprintf("(%s: 40)", f.Name), f.Since)
			continue
		}
		for _, k := range f.Keywords {
			check(fmt.Sprintf("(%s: %s)", f.Name, k.Name), k.Since)
		}
	}
	for _, bad := range []string{"(max-width: 40)", "(hover: dark)"} {
		if _, err := parseMedia("@media "+bad, true); err == nil {
			t.Errorf("%s accepted", bad)
		}
	}
	if len(names) != 5 {
		t.Errorf("media features = %v", names)
	}
}
