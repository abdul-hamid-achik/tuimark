package host

import (
	"strings"
	"testing"
)

// 93. inspect and the gap shorthand (SPEC v0.3b §10.2, §15.7): for a
// version="3" document, row-gap and column-gap come right after gap, each
// with its own winner, which may be a gap declaration (its rule is then
// that declaration's); a gap that lost to a longhand is in the longhand's
// overridden, and a longhand that lost to a gap is in the gap-won
// longhand's. A version="2" document lists neither.
func TestInspectGapShorthand(t *testing.T) {
	type entry struct {
		value, origin, selector string
		line, col               int
		lost                    []string // "value origin selector"
	}
	styleOf := func(t *testing.T, src string) (order []string, by map[string]entry) {
		t.Helper()
		a := doc(t, src)
		f := a.Frame(20, 6)
		in, err := a.Inspect(f, InspectTarget{ID: "g"})
		if err != nil {
			t.Fatal(err)
		}
		by = map[string]entry{}
		for _, s := range in.Style {
			order = append(order, s.Prop)
			e := entry{origin: s.Origin}
			if s.Value != nil {
				e.value = *s.Value
			}
			if s.Rule != nil {
				e.selector, e.line, e.col = s.Rule.Selector, s.Rule.Line, s.Rule.Col
			}
			for _, l := range s.Overridden {
				e.lost = append(e.lost, l.Value+" "+l.Origin+" "+l.Rule.Selector)
			}
			by[s.Prop] = e
		}
		return order, by
	}
	const grid = `<screen id="main"><box id="g" class="x" style="layout: grid; grid-columns: 2"><text>a</text><text>b</text><text>c</text></box></screen></tui>`

	order, by := styleOf(t, `<tui version="3"><style>
.x { row-gap: 0; }
#g { gap: 2; }
</style>`+grid)
	if i := indexOf(order, "gap"); i < 0 || i+2 >= len(order) || order[i+1] != "row-gap" || order[i+2] != "column-gap" {
		t.Fatalf("style order %v: want row-gap and column-gap right after gap", order)
	}
	gap, rg, cg := by["gap"], by["row-gap"], by["column-gap"]
	if gap.selector != "#g" || gap.value != "2" {
		t.Errorf("gap: %+v", gap)
	}
	// The #g gap declaration wins row-gap: same rule, same position.
	if rg.value != "2" || rg.origin != "author" || rg.selector != "#g" || rg.line != gap.line || rg.col != gap.col {
		t.Errorf("row-gap: %+v, want the #g gap declaration (%d:%d)", rg, gap.line, gap.col)
	}
	if strings.Join(rg.lost, "|") != "0 author .x" {
		t.Errorf("row-gap overridden %q, want the .x row-gap", rg.lost)
	}
	if cg.value != "2" || cg.selector != "#g" || len(cg.lost) != 0 {
		t.Errorf("column-gap: %+v", cg)
	}

	// A later longhand in the same rule wins, and the gap it beat is in
	// its overridden; column-gap still comes from gap.
	_, by = styleOf(t, `<tui version="3"><style>#g { gap: 2; row-gap: 0; }</style>`+grid)
	rg, cg = by["row-gap"], by["column-gap"]
	if rg.value != "0" || rg.selector != "#g" || rg.col == by["gap"].col || strings.Join(rg.lost, "|") != "2 author #g" {
		t.Errorf("row-gap: %+v, want the row-gap declaration over the gap one", rg)
	}
	if cg.value != "2" || cg.col != by["gap"].col {
		t.Errorf("column-gap: %+v", cg)
	}

	// The gap attribute is a gap declaration too (origin attribute).
	_, by = styleOf(t, `<tui version="3"><screen id="main"><col id="g" gap="3"><text>a</text></col></screen></tui>`)
	if rg := by["row-gap"]; rg.value != "3" || rg.origin != "attribute" || rg.selector != `gap="3"` {
		t.Errorf("row-gap from the attribute: %+v", rg)
	}

	// Initial values when nothing sets them.
	_, by = styleOf(t, `<tui version="3"><screen id="main"><box id="g"><text>a</text></box></screen></tui>`)
	if rg := by["row-gap"]; rg.value != "0" || rg.origin != "initial" {
		t.Errorf("row-gap unset: %+v", rg)
	}

	// version="2": neither longhand is listed.
	order, _ = styleOf(t, `<tui version="2"><style>#g { gap: 2; }</style>`+grid)
	if indexOf(order, "row-gap") >= 0 || indexOf(order, "column-gap") >= 0 || indexOf(order, "gap") < 0 {
		t.Errorf("version=2 style order %v", order)
	}
}

func indexOf(list []string, s string) int {
	for i, v := range list {
		if v == s {
			return i
		}
	}
	return -1
}
