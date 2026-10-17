package layout

import (
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
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
	// Docked to the left before docked to the top: dock places the top
	// one first, so the container checks its subtree once it is placed,
	// and the left one's message is kept.
	for _, kind := range []string{"col", "scroll"} {
		a := txt("aaaaaaaaaaaaaaa", "dock: left")
		b := txt("bbbbbbbbbbbbbbbbbbbb", "dock: top")
		e := runV3(t, n("screen", "", n(kind, "width: 10", a, b)), 80, 24)
		if got := l008(e); len(got) != 1 || !strings.Contains(got[0], "text is 15 columns wide") {
			t.Errorf("%s, left then top: L008 %q, want the left text's (15 columns)", kind, got)
		}
		if a.Clipped != "text" || b.Clipped != "text" {
			t.Errorf("%s, left then top: Clipped %q and %q, want text and text", kind, a.Clipped, b.Clipped)
		}
	}
	// Docked first, in flow after: placed in their order, so each box is
	// checked as it is placed, and the docked one's message is kept.
	for _, kind := range []string{"col", "scroll"} {
		b := txt("bbbbbbbbbbbbbbbbbbbb", "dock: top")
		a := txt("aaaaaaaaaaaaaaa", "")
		e := runV3(t, n("screen", "", n(kind, "width: 10", b, a)), 80, 24)
		if got := l008(e); len(got) != 1 || !strings.Contains(got[0], "text is 20 columns wide") {
			t.Errorf("%s, docked first: L008 %q, want the docked text's (20 columns)", kind, got)
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

// A container places its children in their order when they are docked
// to the top or the bottom, then to the left or the right, then in flow
// (each group may be empty); only then are they checked as placed.
func TestPlacesInOrder(t *testing.T) {
	kid := func(dock string) *Box {
		return &Box{Style: &css.Style{Dock: dock}}
	}
	for _, tc := range []struct {
		docks []string
		want  bool
	}{
		{nil, true},
		{[]string{""}, true},
		{[]string{"", ""}, true},
		{[]string{"top", ""}, true},
		{[]string{"bottom", "top", "left", "right", "", ""}, true},
		{[]string{"left", "right"}, true},
		{[]string{"top", "bottom"}, true},
		{[]string{"", "top"}, false},
		{[]string{"left", "top"}, false},
		{[]string{"top", "", "bottom"}, false},
		{[]string{"right", "", "left"}, false},
		{[]string{"", "left"}, false},
	} {
		kids := make([]*Box, len(tc.docks))
		for i, d := range tc.docks {
			kids[i] = kid(d)
		}
		if got := placesInOrder(kids); got != tc.want {
			t.Errorf("placesInOrder(%q) = %v, want %v", tc.docks, got, tc.want)
		}
	}
}
