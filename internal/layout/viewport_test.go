package layout

import (
	"strings"
	"testing"
)

// SPEC v0.2 §11.3/§14 L001 for viewports: an fr weight the document wrote
// along a viewport's own scroll axis is L001 (and auto); the built-in
// sheet's col/row 1fr and spacer flex: 1 are auto there without a
// diagnostic, as they are for V010. Every other container is a flex parent.
func TestL001ViewportScrollAxisOnly(t *testing.T) {
	cases := []struct {
		name string
		root *Box
		want string            // diagnostic codes
		geo  map[string]string // treated as auto on the scroll axis
	}{
		{"scroll > col (built-in 1fr)", n("screen", "", n("col", "",
			n("scroll#sc", "", n("col#in", "", txt("a", ""), txt("b", ""))),
			txt("status", ""))), "", map[string]string{"sc": "20x2@(0,0)", "in": "20x2@(0,0)"}},
		{"scroll > row (built-in 1fr)", n("screen", "", n("col", "",
			n("scroll#sc", "", n("row#in", "", txt("a", ""), txt("b", ""))))), "", map[string]string{"in": "20x1@(0,0)"}},
		{"scroll > spacer (built-in flex: 1)", n("screen", "",
			n("scroll#sc", "height: 5", txt("a", ""), n("spacer#sp", ""), txt("b", ""))), "", map[string]string{"sp": "20x0@(0,1)"}},
		{"overflow: scroll box > col", n("screen", "",
			n("box#ov", "overflow: scroll; height: 5", n("col#in", "", txt("a", "")))), "", map[string]string{"in": "20x1@(0,0)"}},
		{"x scroll > col: y is not a scroll axis", n("screen", "",
			n("scroll#sc", "height: 4", n("col#in", "", txt("a", "")))), "", nil},
		{"list > item > row", n("screen", "",
			n("list#l", "", n("item", "", n("row#in", "", txt("a", ""))))), "", map[string]string{"in": "20x1@(0,0)"}},
		{"author height: 1fr in overflow: scroll", n("screen", "",
			n("box#ov", "overflow: scroll; height: 5", n("col#in", "height: 1fr", txt("a", "")))), "L001", map[string]string{"in": "20x1@(0,0)"}},
		{"author flex: 2 in scroll", n("screen", "",
			n("scroll#sc", "height: 5", n("box#in", "flex: 2", txt("a", "")))), "L001", map[string]string{"in": "20x1@(0,0)"}},
		{"author width: 1fr in a row that scrolls on x", n("screen", "",
			n("scroll#sc", "layout: row; height: 3", n("box#in", "width: 1fr", txt("abc", "")))), "L001", map[string]string{"in": "3x3@(0,0)"}},
		{"author flex: 0 on a col: the built-in 1fr is left", n("screen", "",
			n("scroll#sc", "height: 5", n("col#in", "flex: 0", txt("a", "")))), "", map[string]string{"in": "20x1@(0,0)"}},
		{"author height: 1fr on a spacer", n("screen", "",
			n("scroll#sc", "height: 5", txt("a", ""), n("spacer#sp", "height: 1fr"))), "L001", map[string]string{"sp": "20x0@(0,1)"}},
	}
	cases[4].root.Children[0].Axis = "x"
	cases[8].root.Children[0].Axis = "x"
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := run(t, c.root, 20, 10)
			if got := codesOf(e); got != c.want {
				t.Fatalf("codes = %q, want %q: %v", got, c.want, e.Diags)
			}
			expectGeo(t, c.root, c.geo)
		})
	}
	// The message names the declaration the author wrote.
	e := run(t, cases[7].root, 20, 10)
	if len(e.Diags) != 1 || !strings.HasPrefix(e.Diags[0].Msg, "flex: 2 inside a scrolling scroll") {
		t.Errorf("flex message: %v", e.Diags)
	}
}

// SPEC v0.2 §11.4: a scroll's axes come from axis= (y by default); a list
// and an overflow: scroll container scroll on y only, since axis= is
// scroll-only. A value the parser rejected (V003) scrolls on y, so a
// viewport always has a scroll axis.
func TestScrollAxes(t *testing.T) {
	cases := []struct {
		kind, decls, axis string
		x, y              bool
	}{
		{"scroll", "", "", false, true},
		{"scroll", "", "x", true, false},
		{"scroll", "", "both", true, true},
		{"scroll", "", "diagonal", false, true},
		{"list", "", "x", false, true},
		{"box", "overflow: scroll; layout: row", "x", false, true},
		{"col", "overflow: scroll", "both", false, true},
		{"box", "", "", false, false},
	}
	for _, c := range cases {
		b := n(c.kind, c.decls)
		b.Axis = c.axis
		run(t, n("screen", "", b), 10, 5)
		if x, y := b.ScrollAxes(); x != c.x || y != c.y {
			t.Errorf("%s %q axis=%q: scroll axes x=%v y=%v, want x=%v y=%v", c.kind, c.decls, c.axis, x, y, c.x, c.y)
		}
	}
	// With the default, a scroll whose axis= was rejected is shrinkable on
	// y and auto-fits like any other: the status line stays on screen.
	sc := n("scroll#sc", "", txt("a", ""), txt("b", ""), txt("c", ""))
	sc.Axis = "diagonal"
	root := n("screen", "", n("col", "", sc, txt("status", "")))
	root.Children[0].Children[1].ID = "status"
	run(t, root, 10, 3)
	expectGeo(t, root, map[string]string{"sc": "10x2@(0,0)", "status": "10x1@(0,2)"})
}

// SPEC v0.2 §11.4 shrinkable: on an axis it does not scroll, a viewport is
// shrinkable like any other container, when a child is. A <scroll
// axis="x"> holding a long list fits the rows left (the status line stays
// on screen, and nothing is L006).
func TestShrinkableThroughViewportOtherAxis(t *testing.T) {
	sc := n("scroll#sc", "", rowsOf("list#l", "", 2000))
	sc.Axis = "x"
	root := n("screen", "", n("col", "", sc, txt("status", "")))
	root.Children[0].Children[1].ID = "status"
	e := run(t, root, 80, 24)
	if len(e.Diags) != 0 {
		t.Fatalf("diags: %v", e.Diags)
	}
	expectGeo(t, root, map[string]string{"sc": "80x23@(0,0)", "l": "80x23@(0,0)", "status": "80x1@(0,23)"})
	if !e.shrinkable(sc, false) || e.shrinkable(sc.Children[0], true) {
		t.Errorf("shrinkable: sc on y %v, list on x %v", e.shrinkable(sc, false), e.shrinkable(sc.Children[0], true))
	}
}

// SPEC v0.2 §11.5.2: a secret input measures what it paints, one • per
// cluster, so its width does not reveal the display width of its value.
func TestSecretInputIntrinsicWidth(t *testing.T) {
	for _, v := range []string{"abcd", "微信微信", "a👍🏽é微", "​​​​"} {
		in := n("input#s", "")
		in.Secret, in.Text = true, v
		e := &Engine{}
		e.memo, e.seen = map[memoKey]int{}, map[string]bool{}
		if got := e.intrinsic(in, true, 0); got != 5 {
			t.Errorf("secret %q: intrinsic width %d, want 5 (4 bullets + cursor)", v, got)
		}
	}
	// A visible value is measured by its display width, and the
	// placeholder still counts.
	in := n("input#s", "")
	in.Text = "微信"
	e := &Engine{memo: map[memoKey]int{}, seen: map[string]bool{}}
	if got := e.intrinsic(in, true, 0); got != 5 {
		t.Errorf("visible CJK: %d, want 5", got)
	}
	in = n("input#s", "")
	in.Secret, in.Text, in.Placeholder = true, "微", "password"
	e = &Engine{memo: map[memoKey]int{}, seen: map[string]bool{}}
	if got := e.intrinsic(in, true, 0); got != 9 {
		t.Errorf("secret with placeholder: %d, want 9", got)
	}
}
