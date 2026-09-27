package parse_test

import (
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// 53. SPEC v0.2b §6.9.1 and §14: the structure of a table. V016 for each
// trigger (a column outside a table; a child of a table other than
// column and one item, or a second item; character data in a table; a
// table item with children or text; an element inside a column); V017
// without each or without a column; V012 without id; B006 without key.
// A table inside a list item or a container template is V001 as a whole,
// without V012 or B006.
func TestTableStructure(t *testing.T) {
	for _, c := range []struct {
		name, body, codes string
	}{
		{"complete", `<table id="t" each="xs as x" key="x.id" bind="c" placeholder="none"><item class="r"/><column title="A" width="5">{x.a}</column><column>{x.b}</column></table>`, ""},
		{"column outside a table", `<column>x</column>`, "V016"},
		{"column in a box", `<box><column>x</column></box>`, "V016"},
		{"text in a table", `<table id="t" each="xs as x" key="x"><column>{x}</column><text>t</text></table>`, "V016"},
		{"second item", `<table id="t" each="xs as x" key="x"><item/><item/><column>{x}</column></table>`, "V016"},
		{"character data in a table", `<table id="t" each="xs as x" key="x">hello<column>{x}</column></table>`, "V016"},
		{"whitespace in a table", "<table id=\"t\" each=\"xs as x\" key=\"x\">\n  <column>{x}</column>\n</table>", ""},
		{"item with children", `<table id="t" each="xs as x" key="x"><item><text>a</text></item><column>{x}</column></table>`, "V016"},
		{"item with text", `<table id="t" each="xs as x" key="x"><item>a</item><column>{x}</column></table>`, "V016"},
		{"element in a column", `<table id="t" each="xs as x" key="x"><column><text>{x}</text></column></table>`, "V016"},
		{"no each", `<table id="t"><column>a</column></table>`, "V017"},
		{"no column", `<table id="t" each="xs as x" key="x"><item/></table>`, "V017"},
		{"malformed each", `<table id="t" each="xs" key="x"><column>a</column></table>`, "V011"},
		{"no id", `<table each="xs as x" key="x"><column>{x}</column></table>`, "V012"},
		{"no id, focusable", `<table each="xs as x" key="x" focusable="true"><column>{x}</column></table>`, "V012"},
		{"no key", `<table id="t" each="xs as x"><column>{x}</column></table>`, "B006"},
		{"in a list item", `<list id="l" each="ys as y" key="y"><item><table each="xs as x"><column>{x}</column></table></item></list>`, "V001"},
		{"in an each template", `<col each="ys as y"><table each="xs as x"><column>{x}</column></table></col>`, "V001"},
	} {
		t.Run(c.name, func(t *testing.T) {
			doc := parse.Parse([]byte(coreV2(c.body)), "")
			if got := coreCodes(doc); got != c.codes {
				t.Errorf("codes %q, want %q: %v", got, c.codes, doc.Diags)
			}
			for _, d := range doc.Diags {
				if d.Code == "B006" && d.Severity != ir.Warning {
					t.Errorf("B006 is a warning: %s", d)
				}
			}
		})
	}
}

// In a version="1" document a table and its children are V001 with the
// version hint, and nothing inside them gets a 0.2b structure code.
func TestTableStructureVersionOne(t *testing.T) {
	doc := parse.Parse([]byte(`<tui version="1"><screen id="s"><table id="t"><item><text>a</text></item><column><text>b</text></column>x</table></screen></tui>`), "")
	for _, d := range doc.Diags {
		switch d.Code {
		case "V016", "V017", "B006", "V012":
			t.Errorf("a version=\"1\" document got %s", d)
		}
	}
	if !strings.Contains(coreCodes(doc), "V001") {
		t.Errorf("codes %s", coreCodes(doc))
	}
}
