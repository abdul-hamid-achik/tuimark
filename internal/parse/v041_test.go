package parse_test

import (
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// v0.4.1: a <tui> without version holds every version="3" rule, whatever
// else it writes; since v0.4b it is the current language, IR 0.4.
func TestMissingVersionReadsAsCurrent(t *testing.T) {
	for _, src := range []string{
		`<tui><screen/></tui>`,
		`<tui theme="auto" mouse="true"><screen/></tui>`,
	} {
		doc := parse.Parse([]byte(src), "")
		if len(doc.Diags) != 0 {
			t.Errorf("%s: %v, want no diagnostics", src, msgs(doc))
		}
		if !doc.V2 || !doc.V3 {
			t.Errorf("%s: V2=%v V3=%v, want both", src, doc.V2, doc.V3)
		}
		if v := parse.BuildIR(doc, nil).Version; v != ir.VersionV4 {
			t.Errorf("%s: ir version %q, want %q", src, v, ir.VersionV4)
		}
	}
	for version, v2v3 := range map[string][2]bool{"1": {false, false}, "2": {true, false}, "3": {true, true}, "x": {false, false}} {
		doc := parse.Parse([]byte(`<tui version="`+version+`"><screen/></tui>`), "")
		if doc.V2 != v2v3[0] || doc.V3 != v2v3[1] {
			t.Errorf("version=%q: V2=%v V3=%v, want %v", version, doc.V2, doc.V3, v2v3)
		}
	}
}
