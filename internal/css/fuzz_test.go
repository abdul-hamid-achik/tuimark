package css

import (
	"os"
	"path/filepath"
	"testing"
)

// FuzzParseSheet checks that no stylesheet text makes the TCSS parser
// panic, for a document of each version, and that every rule it keeps has
// at least one selector. Seeds are the real stylesheets under examples/.
func FuzzParseSheet(f *testing.F) {
	paths, _ := filepath.Glob("../../examples/*/*.tcss")
	more, _ := filepath.Glob("../../examples/*/testdata/*/*.tcss")
	for _, p := range append(paths, more...) {
		if b, err := os.ReadFile(p); err == nil {
			f.Add(string(b))
		}
	}
	f.Add("@media (max-cols: 80) { row }\nbox { color: red; }")
	f.Add("box { padding: +1 }  #1 { } .x:")
	f.Fuzz(func(t *testing.T, src string) {
		for _, v := range []struct{ v2, v3 bool }{{false, false}, {true, false}, {true, true}} {
			sh, _ := ParseSheetIn(src, "f.tcss", v.v2, v.v3)
			for _, r := range sh.Rules {
				if len(r.Selectors) == 0 {
					t.Fatalf("rule without selectors: %+v", r)
				}
			}
			ParseInlineSheetIn(src, "f.tui", 3, 7, v.v2, v.v3)
			ParseDeclsAllIn(src, v.v2, v.v3)
		}
	})
}
