package layout

import (
	"fmt"
	"strings"
	"testing"
)

// SPEC v0.2 §11.4 auto-fit (ADR 0007) and L006, at the engine level.

// rows builds a list (or scroll, by kind) holding count one-line items.
func rowsOf(kind, decls string, count int) *Box {
	items := make([]*Box, count)
	for i := range items {
		items[i] = n("item", "", txt(fmt.Sprintf("row %d", i), ""))
		items[i].Index = i
	}
	return n(kind, decls, items...)
}

func l006(e *Engine) []string {
	var out []string
	for _, d := range e.Diags {
		if d.Code == "L006" {
			out = append(out, d.ID)
		}
	}
	return out
}

// l006Msg returns the message of the first L006, or "".
func l006Msg(e *Engine) string {
	for _, d := range e.Diags {
		if d.Code == "L006" {
			return d.Msg
		}
	}
	return ""
}

// §21 test 22: a col holding an unsized 2000-row list and a one-line text.
// v0.1 gave the list 80x2000 and pushed the text off-screen; auto-fit gives
// the list the rows left above the text, and the list follows its selection.
func TestAutoFitLongListFollowsSelection(t *testing.T) {
	for _, follow := range []int{0, 1000, 1999} {
		l := rowsOf("list#l", "", 2000)
		l.Follow = follow
		root := n("screen", "", n("col", "", l, txt("status", "")))
		root.Children[0].Children[1].ID = "status"
		e := run(t, root, 80, 24)
		if len(e.Diags) != 0 {
			t.Fatalf("follow %d: %v", follow, e.Diags)
		}
		expectGeo(t, root, map[string]string{"l": "80x23@(0,0)", "status": "80x1@(0,23)"})
		if l.ContentH != 2000 || l.Content.H != 23 {
			t.Errorf("follow %d: content extent %d, viewport %d", follow, l.ContentH, l.Content.H)
		}
		want := max(0, follow-22)
		if l.ScrollY != want {
			t.Errorf("follow %d: offset %d, want %d", follow, l.ScrollY, want)
		}
		it := l.Children[follow]
		if it.Y < l.Content.Y || it.Y >= l.Content.Y+l.Content.H || it.Clip.H != 1 {
			t.Errorf("follow %d: selected item at y=%d clip %+v is not visible", follow, it.Y, it.Clip)
		}
		// Every item is still laid out, the hidden ones outside the clip.
		if first := l.Children[0]; !first.Laid || (follow > 22 && first.Clip.H != 0) {
			t.Errorf("follow %d: first item laid=%v clip %+v", follow, first.Laid, first.Clip)
		}
	}
	// The same with gaps: the extent includes them.
	l := rowsOf("list#l", "gap: 1", 2000)
	l.Follow = 1999
	root := n("screen", "", l)
	run(t, root, 80, 24)
	if l.ContentH != 2000+1999 || l.ScrollY != 3999-24 || l.Children[1999].Y != 23 {
		t.Errorf("gaps: extent %d offset %d last item y=%d", l.ContentH, l.ScrollY, l.Children[1999].Y)
	}
}

// §21 test 23 and rule 8: nothing changes when the content fits.
func TestAutoFitIsANoOpWhenContentFits(t *testing.T) {
	l := rowsOf("list#l", "", 3)
	root := n("screen", "", n("col", "", l, txt("after", "")))
	root.Children[0].Children[1].ID = "after"
	e := run(t, root, 20, 10)
	if len(e.Diags) != 0 {
		t.Fatal(e.Diags)
	}
	expectGeo(t, root, map[string]string{"l": "20x3@(0,0)", "after": "20x1@(0,3)"})
	if l.ContentH != 3 || l.ScrollY != 0 {
		t.Errorf("extent %d offset %d", l.ContentH, l.ScrollY)
	}
	// Exactly full: still the content size.
	full := rowsOf("list#l", "", 9)
	root = n("screen", "", n("col", "", full, txt("after", "")))
	run(t, root, 20, 10)
	if geo(full) != "20x9@(0,0)" {
		t.Errorf("exact fit: %s", geo(full))
	}
}

// §21 test 23 and rule 5: a list inside a <scroll> is an unbounded
// allocation: it keeps its full height, the outer viewport scrolls, and no
// L006 is reported.
func TestAutoFitUnboundedInsideScroll(t *testing.T) {
	l := rowsOf("list#l", "", 50)
	root := n("screen", "", n("scroll#s", "height: 10", txt("head", ""), l))
	e := run(t, root, 20, 24)
	if len(e.Diags) != 0 {
		t.Fatalf("diags: %v", e.Diags)
	}
	s := byID(root, "s")
	if geo(l) != "20x50@(0,1)" || s.ContentH != 51 {
		t.Errorf("list %s, scroll extent %d", geo(l), s.ContentH)
	}
	// A box inside the scroll holding the list is unbounded too.
	l = rowsOf("list#l", "", 50)
	root = n("screen", "", n("scroll", "height: 10", n("box#b", "", l)))
	if e := run(t, root, 20, 24); len(e.Diags) != 0 {
		t.Fatalf("diags: %v", e.Diags)
	}
	expectGeo(t, root, map[string]string{"b": "20x50@(0,0)", "l": "20x50@(0,0)"})
}

// Rule 8: the auto siblings after the shrinkable child move back into
// view; siblings before it and fixed ones keep their v0.1 size.
func TestAutoFitSiblings(t *testing.T) {
	root := n("screen", "",
		n("col", "",
			n("text#top", ""),
			rowsOf("list#l", "", 100),
			n("box#fixed", "height: 3"),
			n("text#bottom", ""),
		),
	)
	byID(root, "top").Text = "top"
	byID(root, "bottom").Text = "bottom"
	run(t, root, 20, 12)
	expectGeo(t, root, map[string]string{
		"top":    "20x1@(0,0)",
		"l":      "20x7@(0,1)",
		"fixed":  "20x3@(0,8)",
		"bottom": "20x1@(0,11)",
	})
	// A container holding a viewport is shrinkable too: the status line
	// under an auto box with a long list stays on the last row.
	root = n("screen", "",
		n("col", "",
			n("box#wrap", "border: single", txt("title", ""), rowsOf("list#l", "", 100)),
			n("text#status", ""),
		),
	)
	byID(root, "status").Text = "ok"
	e := run(t, root, 20, 12)
	if len(e.Diags) != 0 {
		t.Fatal(e.Diags)
	}
	expectGeo(t, root, map[string]string{"wrap": "20x11@(0,0)", "l": "18x8@(1,2)", "status": "20x1@(0,11)"})
	// Shrinkability propagates through docked children as well.
	root = n("screen", "",
		n("col", "",
			n("box#wrap", "", rowsOf("list#l", "dock: top", 100)),
			n("text#status", ""),
		),
	)
	byID(root, "status").Text = "ok"
	run(t, root, 20, 12)
	expectGeo(t, root, map[string]string{"wrap": "20x11@(0,0)", "l": "20x11@(0,0)", "status": "20x1@(0,11)"})
}

// Rule 6: an unsized viewport with an fr sibling is 1fr, and explicit
// cells, percents, and fr are never changed by auto-fit.
func TestAutoFitLeavesSizedAndFrAlone(t *testing.T) {
	root := n("screen", "",
		n("col", "", rowsOf("list#l", "", 100), n("box#f", "height: 1fr")),
	)
	run(t, root, 20, 10)
	expectGeo(t, root, map[string]string{"l": "20x5@(0,0)", "f": "20x5@(0,5)"})

	for _, c := range []struct{ decls, want string }{
		{"height: 4", "20x4@(0,0)"},
		{"height: 50%", "20x5@(0,0)"},
		{"flex: 1", "20x9@(0,0)"},
		{"height: 1fr", "20x9@(0,0)"},
	} {
		root := n("screen", "", n("col", "", rowsOf("list#l", c.decls, 100), txt("x", "")))
		run(t, root, 20, 10)
		if got := geo(byID(root, "l")); got != c.want {
			t.Errorf("%s: %s, want %s", c.decls, got, c.want)
		}
	}
	// A sized list larger than its parent keeps its size (and gets L006).
	root = n("screen", "", n("col", "", rowsOf("list#l", "height: 30", 100)))
	if e := run(t, root, 20, 10); geo(byID(root, "l")) != "20x30@(0,0)" || codesOf(e) != "L003 L006" {
		t.Errorf("sized list: %s, codes %q", geo(byID(root, "l")), codesOf(e))
	}
}

// Rule 2: min/max clamp after the cap; a min larger than what is left
// overflows as in v0.1, and L003 counts only the min.
func TestAutoFitMinMax(t *testing.T) {
	for _, c := range []struct {
		decls  string
		count  int
		want   string
		codes  string
		detail string
	}{
		{"max-height: 4", 100, "20x4@(0,0)", "", "max below the space left"},
		{"min-height: 3", 2, "20x3@(0,0)", "", "min above the content"},
		{"min-height: 6", 100, "20x9@(0,0)", "", "min below the space left"},
		{"min-height: 12", 100, "20x12@(0,0)", "L003 L006", "min above the space left overflows"},
		{"max-height: 50%", 100, "20x5@(0,0)", "", "percent max"},
	} {
		root := n("screen", "", n("col", "", rowsOf("list#l", c.decls, c.count), txt("x", "")))
		e := run(t, root, 20, 10)
		if got := geo(byID(root, "l")); got != c.want {
			t.Errorf("%s: %s, want %s", c.detail, got, c.want)
		}
		if got := codesOf(e); got != c.codes {
			t.Errorf("%s: codes %q, want %q", c.detail, got, c.codes)
		}
	}
	// L003 does not count a deferred child's content: a long list with a
	// small min fits by scrolling.
	root := n("screen", "", n("col", "", rowsOf("list#l", "min-height: 2", 2000), n("box", "height: 3")))
	if e := run(t, root, 20, 10); len(e.Diags) != 0 {
		t.Errorf("small min: %v", e.Diags)
	}
}

// Rule 3 and §21 test 24 (a): shrinkable siblings are served in document
// order; the first leaves 0 to the second, which reports L006.
func TestAutoFitSiblingsInDocumentOrder(t *testing.T) {
	root := n("screen", "",
		n("col", "", rowsOf("list#a", "", 30), rowsOf("list#b", "", 30), txt("status", "")),
	)
	e := run(t, root, 20, 10)
	expectGeo(t, root, map[string]string{"a": "20x9@(0,0)", "b": "20x0@(0,9)"})
	if got := strings.Join(l006(e), " "); got != "b" {
		t.Fatalf("L006 on %q, diags %v", got, e.Diags)
	}
	if d := e.Diags[len(e.Diags)-1]; d.Code != "L006" || d.Severity != "warning" || !strings.Contains(d.Msg, "0 rows") {
		t.Errorf("diag = %+v", d)
	}
	// A small first list leaves the rest to the second.
	root = n("screen", "",
		n("col", "", rowsOf("list#a", "", 3), rowsOf("list#b", "", 30)),
	)
	if e := run(t, root, 20, 10); len(e.Diags) != 0 {
		t.Fatal(e.Diags)
	}
	expectGeo(t, root, map[string]string{"a": "20x3@(0,0)", "b": "20x7@(0,3)"})
	// fr on both shares the space.
	root = n("screen", "",
		n("col", "", rowsOf("list#a", "flex: 1", 30), rowsOf("list#b", "flex: 1", 30)),
	)
	if e := run(t, root, 20, 10); len(e.Diags) != 0 {
		t.Fatal(e.Diags)
	}
	expectGeo(t, root, map[string]string{"a": "20x5@(0,0)", "b": "20x5@(0,5)"})
}

// Margins are part of the slot: the cap leaves room for them.
func TestAutoFitMargins(t *testing.T) {
	root := n("screen", "",
		n("col", "", rowsOf("list#l", "margin: 1 0 2 0", 100), txt("x", "")),
	)
	run(t, root, 20, 10)
	expectGeo(t, root, map[string]string{"l": "20x6@(0,1)"})
	if x := byID(root, "l").Parent.Children[1]; x.Y != 9 {
		t.Errorf("text after the margins at y=%d", x.Y)
	}
}

// The x axis: an unsized scroll with axis="x" in a row fits the columns
// left; a y-only list is not shrinkable on x and keeps its v0.1 width.
func TestAutoFitHorizontal(t *testing.T) {
	sx := n("scroll#sx", "", txt(strings.Repeat("x", 50), ""))
	sx.Axis = "x"
	root := n("screen", "", n("row", "", sx, n("text#end", "")))
	byID(root, "end").Text = "end"
	e := run(t, root, 20, 5)
	if len(e.Diags) != 0 {
		t.Fatal(e.Diags)
	}
	expectGeo(t, root, map[string]string{"sx": "17x5@(0,0)", "end": "3x5@(17,0)"})
	if sx.ContentW != 50 {
		t.Errorf("x extent %d", sx.ContentW)
	}
	l := n("list#l", "", n("item", "", txt(strings.Repeat("y", 50), "")))
	root = n("screen", "", n("row", "", l, n("text#end", "")))
	byID(root, "end").Text = "end"
	if e := run(t, root, 20, 5); len(e.Diags) != 0 {
		t.Fatal(e.Diags)
	}
	expectGeo(t, root, map[string]string{"l": "50x5@(0,0)", "end": "3x5@(50,0)"})
}

// Rule 4: across the parent's main axis, an auto viewport under a
// non-stretch align is capped to the cross size; under stretch it already
// fills it; in a parent that scrolls on that axis it is not capped.
func TestAutoFitCrossAxis(t *testing.T) {
	for _, c := range []struct{ align, want string }{
		{"start", "4x5@(0,0)"},
		{"center", "4x5@(0,0)"},
		{"end", "4x5@(0,0)"},
		{"stretch", "4x5@(0,0)"},
	} {
		root := n("screen", "",
			n("row", "height: 5; align: "+c.align, rowsOf("list#l", "height: auto; width: 4", 20)),
		)
		if e := run(t, root, 20, 10); len(e.Diags) != 0 {
			t.Fatalf("%s: %v", c.align, e.Diags)
		}
		if got := geo(byID(root, "l")); got != c.want {
			t.Errorf("align %s: %s, want %s", c.align, got, c.want)
		}
	}
	// Short content under a non-stretch align keeps its own size.
	root := n("screen", "", n("row", "height: 5; align: end", rowsOf("list#l", "height: auto; width: 4", 2)))
	run(t, root, 20, 10)
	expectGeo(t, root, map[string]string{"l": "4x2@(0,3)"})
	// A row-direction scroll on y: the cross axis is unbounded.
	outer := n("scroll#s", "height: 5; layout: row; align: start", rowsOf("list#l", "height: auto; width: 4", 20))
	root = n("screen", "", outer)
	run(t, root, 20, 10)
	expectGeo(t, root, map[string]string{"l": "4x20@(0,0)"})
}

// Rule 7: docks, the root, and modals are capped by their area as in v0.1.
func TestAutoFitDocksAndModals(t *testing.T) {
	root := n("screen", "", rowsOf("list#l", "dock: top", 100), txt("x", ""))
	run(t, root, 20, 10)
	expectGeo(t, root, map[string]string{"l": "20x10@(0,0)"})
	m := n("modal#m", "width: 10; height: auto", rowsOf("list#ml", "", 100))
	root = n("screen", "", txt("x", ""))
	e := run(t, root, 20, 10, m)
	if len(e.Diags) != 0 {
		t.Fatal(e.Diags)
	}
	expectGeo(t, root, map[string]string{})
	if geo(m) != "10x10@(5,0)" || geo(byID(m, "ml")) != "8x8@(6,1)" {
		t.Errorf("modal %s list %s", geo(m), geo(byID(m, "ml")))
	}
}

// L002 still sees a shrinkable auto parent as auto.
func TestAutoFitKeepsL002(t *testing.T) {
	root := n("screen", "",
		n("col", "", n("box#wrap", "", rowsOf("list#l", "height: 50%", 100))),
	)
	e := run(t, root, 20, 10)
	if got := codesOf(e); !strings.Contains(got, "L002") {
		t.Errorf("codes %q", got)
	}
}

// Empty viewports clamp their offset to 0; their extent is the content box.
func TestEmptyViewportScrollState(t *testing.T) {
	l := n("list#l", "height: 4; border: single")
	l.ScrollY = 7
	root := n("screen", "", l)
	if e := run(t, root, 20, 10); len(e.Diags) != 0 {
		t.Fatal(e.Diags)
	}
	if l.ScrollY != 0 || l.ContentH != 2 || l.ContentW != 18 {
		t.Errorf("empty list: offset %d extent %dx%d", l.ScrollY, l.ContentW, l.ContentH)
	}
}

// §21 test 24 (b) and the exclusion: a sized list clipped by a box that
// does not scroll reports L006; the same list in a <scroll> does not.
func TestL006Clipped(t *testing.T) {
	root := n("screen", "", n("box#b", "height: 5", rowsOf("list#l", "height: 10", 20)))
	e := run(t, root, 20, 24)
	if got := strings.Join(l006(e), " "); got != "l" {
		t.Fatalf("L006 on %q: %v", got, e.Diags)
	}
	if msg := l006Msg(e); !strings.Contains(msg, "clipped on y by box#b") {
		t.Errorf("msg = %q", msg)
	}
	root = n("screen", "", n("scroll#s", "height: 5", rowsOf("list#l", "height: 10", 20)))
	if e := run(t, root, 20, 24); len(e.Diags) != 0 {
		t.Errorf("inside a scroll: %v", e.Diags)
	}
	// Clipping by a box inside the scroll is still reported: the walk stops
	// at the nearest ancestor scrolling on the axis, not before.
	root = n("screen", "", n("scroll", "height: 5", n("box#b", "height: 3", rowsOf("list#l", "height: 10", 20))))
	if got := strings.Join(l006(run(t, root, 20, 24)), " "); got != "l" {
		t.Errorf("box inside a scroll: L006 on %q", got)
	}
	// An x scroll clips on y: that is not the list's scroll axis, so the
	// walk goes on to the root, which holds it.
	sx := n("scroll", "width: 10; height: 5", rowsOf("list#l", "height: 3; width: 30", 20))
	sx.Axis = "x"
	root = n("screen", "", sx)
	if e := run(t, root, 20, 24); len(l006(e)) != 0 {
		t.Errorf("list in an x scroll: %v", e.Diags)
	}
	// The root clips too.
	root = n("screen", "", rowsOf("list#l", "height: 30", 40))
	if got := strings.Join(l006(run(t, root, 20, 10)), " "); got != "l" {
		t.Errorf("root: L006 on %q", got)
	}
	// Horizontal: an x scroll wider than its parent.
	sx = n("scroll#sx", "width: 30; height: 1", txt(strings.Repeat("x", 50), ""))
	sx.Axis = "x"
	root = n("screen", "", n("box", "width: 10", sx))
	e = run(t, root, 20, 5)
	if got := strings.Join(l006(e), " "); got != "sx" || !strings.Contains(l006Msg(e), "clipped on x") {
		t.Errorf("x: L006 on %q: %v", got, e.Diags)
	}
}

// L006 (a) needs content: an empty list with no room is fine, and one
// warning is reported per node even when both conditions hold.
func TestL006NeedsContentAndIsOncePerNode(t *testing.T) {
	root := n("screen", "", n("col", "", n("box", "height: 10"), n("list#l", "")))
	if e := run(t, root, 20, 10); len(e.Diags) != 0 {
		t.Errorf("empty list: %v", e.Diags)
	}
	// Border only: 0 content rows and clipped by the parent.
	root = n("screen", "", n("box", "height: 3", rowsOf("list#l", "height: 5; border: single; padding: 1 0", 4)))
	e := run(t, root, 20, 10)
	if got := l006(e); len(got) != 1 {
		t.Errorf("once per node: %v", e.Diags)
	}
	// overflow: scroll containers are viewports too.
	root = n("screen", "", n("col", "", n("box", "height: 10"), n("box#v", "overflow: scroll", txt("a", ""))))
	if got := strings.Join(l006(run(t, root, 20, 10)), " "); got != "v" {
		t.Errorf("overflow: scroll: L006 on %q", got)
	}
}

// Items scrolled out of a list are clipped by the list itself, which
// scrolls on y: a viewport inside them never reports L006 for y.
func TestL006InsideListItems(t *testing.T) {
	items := make([]*Box, 20)
	for i := range items {
		items[i] = n("item", "", n("scroll", "height: 2", txt("a", ""), txt("b", ""), txt("c", "")))
	}
	l := n("list#l", "height: 5", items...)
	l.Follow = 19
	root := n("screen", "", l)
	if e := run(t, root, 20, 10); len(e.Diags) != 0 {
		t.Errorf("viewports in scrolled-out items: %v", e.Diags)
	}
}
