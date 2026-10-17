package layout

import (
	"strings"
	"testing"
)

func l008(e *Engine) []string {
	var out []string
	for _, d := range e.Diags {
		if d.Code == "L008" {
			out = append(out, d.Msg)
		}
	}
	return out
}

// The checks run as each box is placed (placed), in the order a walk of
// the children after layout gives (check), also where a container places
// its children in another order: docked ones first. L008 is reported
// once per source element, with the message of the first of its boxes
// checked, so the order shows in which message is kept.
func TestChecksKeepTheOrderOfAWalkAfterLayout(t *testing.T) {
	for _, kind := range []string{"col", "scroll"} {
		// Two cut texts that clipFirst counts as one element (no Src, no
		// id): the first child is in flow, 15 columns in 10; the second
		// is docked, 20 columns in 10, and placed first.
		a := txt("aaaaaaaaaaaaaaa", "")
		b := txt("bbbbbbbbbbbbbbbbbbbb", "dock: top")
		box := n(kind, "width: 10", a, b)
		root := n("screen", "", box)
		e := runV3(t, root, 80, 24)
		got := l008(e)
		if len(got) != 1 || !strings.Contains(got[0], "text is 15 columns wide") {
			t.Errorf("%s: L008 %q, want the in-flow text's (15 columns)", kind, got)
		}
		if a.Clipped != "text" || b.Clipped != "text" {
			t.Errorf("%s: Clipped %q and %q, want text and text", kind, a.Clipped, b.Clipped)
		}
	}
	// Without a docked child, every box is checked as it is placed: the
	// first row's message is kept.
	l := n("list", "width: 10", txt("aaaaaaaaaaaaaaa", ""), txt("bbbbbbbbbbbbbbbbbbbb", ""))
	e := runV3(t, n("screen", "", l), 80, 24)
	if got := l008(e); len(got) != 1 || !strings.Contains(got[0], "text is 15 columns wide") {
		t.Errorf("list: L008 %q, want the first text's (15 columns)", got)
	}
}
