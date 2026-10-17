package layout

import "testing"

// A viewport's content height (scrollLayout) comes from the measure its
// parent already made of its height (measuredContentH) when measuring it
// again would give the same value, and is measured again when it would
// not: a percent-width child reads the viewport's auto width mark, which
// its parent sets between the two measures.
func TestScrollContentHeightReusesTheParentsMeasure(t *testing.T) {
	l := rowsOf("list#l", "", 500)
	root := n("screen", "", l, txt("status", "dock: bottom"))
	e := run(t, root, 80, 24)
	if l.ContentH != 500 || l.Content.H != 23 {
		t.Fatalf("content extent %d, viewport %d", l.ContentH, l.Content.H)
	}
	if v, ok := e.measuredContentH(l); !ok || v != 500 {
		t.Errorf("measuredContentH = %d, %v; want 500, true", v, ok)
	}
	if fresh := e.contentIntrinsic(l, false, l.Content.W); fresh != 500 {
		t.Errorf("contentIntrinsic = %d, want 500", fresh)
	}
}

func TestScrollContentHeightMeasuresAgainAfterAutoMarks(t *testing.T) {
	// The screen measures the col, and so the list's height, before the
	// col's layout marks the list's width auto (align: start, width:
	// auto): row b is then 50% of the list's 20 columns, so its text
	// wraps to two lines and the list measures 3 rows. Laid out, the
	// list's width is auto, so b's 50% is L002 and b takes its own width,
	// 19 columns, on one line: 2 rows, which the 2-row screen shows whole.
	a := n("item", "", txt("xxxxxxxxxxxxxxxxxxxx", ""))
	b := n("item", "width: 50%", txt("aaaa bbbb cccc dddd", "wrap: wrap"))
	l := n("list#l", "width: auto", a, b)
	col := n("col", "height: auto; align: start", l)
	root := n("screen", "", col)
	e := run(t, root, 80, 2)
	if v, ok := e.memoGet(l, false, l.W); !ok || v != 3 {
		t.Fatalf("the col's measure of the list = %d, %v; want 3, true", v, ok)
	}
	if _, ok := e.measuredContentH(l); ok {
		t.Errorf("measuredContentH used a measure that read the auto marks")
	}
	if l.ContentH != 2 || l.Content.H != 2 {
		t.Errorf("content extent %d, viewport %d; want 2, 2 (measured again)", l.ContentH, l.Content.H)
	}
	if codes := codesOf(e); codes != "L002" {
		t.Errorf("diagnostics %q, want L002", codes)
	}
}
