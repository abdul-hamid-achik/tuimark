package layout

import (
	"fmt"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
)

// SPEC v0.2b §11.7 (layout: grid), §6.10.4 (the tab strip), and §6.12
// (hint items), §21 tests 54, 57, 58: the integer arithmetic on
// hand-built boxes.

// n2 is n with version="2" declarations (layout: grid, grid-columns, …).
func n2(kind, decls string, kids ...*Box) *Box {
	b := n(kind, "", kids...)
	if decls != "" {
		ds, errs := css.ParseDeclsAllIn(decls, true, false)
		if len(errs) > 0 {
			panic(errs[0])
		}
		b.Inline = ds
	}
	return b
}

// 58. The column count and widths of items 1-2: n = K without
// grid-min-width, else clamp(floor((Wc + g) / (M + g)), 1, K); the last
// column takes the remainder.
func TestGridColumnsArithmetic(t *testing.T) {
	for _, c := range []struct {
		k, m, g, wc int
		want        string
	}{
		{4, 22, 1, 38, "[38]"},           // the cores at 40 columns
		{4, 22, 1, 78, "[25 25 26]"},     // at 80
		{4, 22, 1, 118, "[28 28 28 31]"}, // at 120, clamped to K
		{3, 8, 1, 20, "[9 10]"},          // §11.7 fixture at 20
		{3, 8, 1, 30, "[9 9 10]"},        // and at 30
		{3, 0, 2, 20, "[5 5 6]"},         // no grid-min-width: n = K
		{2, 50, 0, 10, "[10]"},           // at least one column
		{2, 0, 4, 2, "[0 0]"},            // gaps larger than the width
	} {
		b := &Box{Style: css.Initial()}
		b.Style.GridColumns, b.Style.GridMinWidth, b.Style.Gap = c.k, c.m, c.g
		_, w := gridColumns(b, c.wc, false)
		if got := fmt.Sprint(w); got != c.want {
			t.Errorf("K %d M %d g %d Wc %d: %s, want %s", c.k, c.m, c.g, c.wc, got, c.want)
		}
	}
}

// 58. Placement: rows as tall as their tallest child, the gap between
// rows and columns, align start/center/end/stretch in the row, margins
// inside the cell; the intrinsic size of item 6.
func TestGridPlacementAndIntrinsic(t *testing.T) {
	cell := func(id string, lines int) *Box {
		return txt(strings.TrimSuffix(strings.Repeat(id+"\n", lines), "\n"), "")
	}
	for _, c := range []struct {
		align string
		want  string // geometry of the short child b
	}{
		{"stretch", "9x3@(10,0)"},
		{"start", "9x1@(10,0)"},
		{"center", "9x1@(10,1)"},
		{"end", "9x1@(10,2)"},
	} {
		a, b, cc := cell("a", 3), cell("b", 1), cell("c", 2)
		b.ID = "b"
		g := n2("box#g", "layout: grid; grid-columns: 2; gap: 1; align: "+c.align, a, b, cc)
		root := n("col", "", g)
		run(t, root, 19, 10)
		if geo(b) != c.want {
			t.Errorf("align %s: b %s, want %s", c.align, geo(b), c.want)
		}
		// Rows 3 and 2 tall with a gap of 1: 6; columns 9 and 9.
		if geo(g) != "19x6@(0,0)" || geo(cc) != "9x2@(0,4)" {
			t.Errorf("align %s: g %s, c %s", c.align, geo(g), geo(cc))
		}
	}
	// Intrinsic width: n' = min(K, children) columns of M; without M,
	// n' = K columns (item 1 lays out K) of the widest child.
	for _, c := range []struct {
		decls string
		want  int
	}{
		{"layout: grid; grid-columns: 4; grid-min-width: 6; gap: 1", 6*2 + 1},
		{"layout: grid; grid-columns: 4; gap: 2", 5*4 + 2*3},
		{"layout: grid; grid-columns: 1; gap: 2", 5},
	} {
		g := n2("box#g", c.decls, txt("abc", ""), txt("abcde", ""))
		root := n("row", "", g)
		run(t, root, 40, 3)
		if g.W != c.want {
			t.Errorf("%s: intrinsic width %d, want %d", c.decls, g.W, c.want)
		}
	}
	// Margins sit inside the cell; a cell height comes from the child's
	// height in cells when it has one.
	m := txt("m", "margin: 1")
	h := n("box", "height: 4")
	g := n2("box#g", "layout: grid; grid-columns: 2", m, h)
	run(t, n("col", "", g), 20, 10)
	if geo(m) != "8x2@(1,1)" || geo(h) != "10x4@(10,0)" || g.H != 4 {
		t.Errorf("margins: m %s, h %s, g h %d", geo(m), geo(h), g.H)
	}
}

// 58. Docked children are pulled out first; the grid lays out the rest.
func TestGridWithDock(t *testing.T) {
	top := txt("top", "dock: top")
	a, b := txt("a", ""), txt("b", "")
	g := n2("box#g", "layout: grid; grid-columns: 2", top, a, b)
	run(t, n("col", "", g), 10, 5)
	if geo(top) != "10x1@(0,0)" || geo(a) != "5x1@(0,1)" || geo(b) != "5x1@(5,1)" || g.H != 2 {
		t.Errorf("top %s a %s b %s g h %d", geo(top), geo(a), geo(b), g.H)
	}
}

// 58. Without grid-min-width, an auto-width grid is as wide as the K
// columns item 1 lays out, also with fewer children than K, so no child
// is cut: K = 3 columns of the widest child (5) and two gaps of 1.
func TestGridIntrinsicWidthWithoutMinWidth(t *testing.T) {
	a, b := txt("alpha", ""), txt("beta", "")
	g := n2("box#g", "layout: grid; grid-columns: 3; gap: 1", a, b)
	rest := txt("|rest", "")
	e := run(t, n("row", "height: 1", g, rest), 30, 1)
	if len(e.Diags) != 0 {
		t.Fatal(e.Diags)
	}
	if geo(g) != "17x1@(0,0)" || geo(a) != "5x1@(0,0)" || geo(b) != "5x1@(6,0)" || rest.X != 17 {
		t.Errorf("g %s alpha %s beta %s rest x %d", geo(g), geo(a), geo(b), rest.X)
	}
}

// 58. The grid lays out its in-flow children in what the docks leave, and
// measures its intrinsic height at that same width: with a 20-cell left
// dock in 42 columns, Wc = 22 gives 2 columns (M = 10, g = 1), so six
// children take 3 rows, the box is 5 rows tall, and nothing is cut.
func TestGridLeftDockMeasuredAtWhatIsLeft(t *testing.T) {
	side := txt("SIDE", "dock: left; width: 20")
	var cs []*Box
	for i := 0; i < 6; i++ {
		cs = append(cs, txt(fmt.Sprintf("c%d", i), ""))
	}
	g := n2("box#g", "layout: grid; grid-columns: 4; grid-min-width: 10; gap: 1", append([]*Box{side}, cs...)...)
	status := txt("status", "")
	e := run(t, n("col", "", g, status), 42, 8)
	if len(e.Diags) != 0 {
		t.Fatal(e.Diags)
	}
	if geo(g) != "42x5@(0,0)" || geo(side) != "20x5@(0,0)" || geo(cs[4]) != "10x1@(20,4)" || geo(cs[5]) != "11x1@(31,4)" || status.Y != 5 {
		t.Errorf("g %s side %s c4 %s c5 %s status y %d", geo(g), geo(side), geo(cs[4]), geo(cs[5]), status.Y)
	}
}

// 58 (and v1 §11.3). A container's height is measured with its in-flow
// children at the width the left and right docks leave, so wrapped text
// next to a dock gets all its lines.
func TestWrappedTextNextToADock(t *testing.T) {
	side := txt("SIDE", "dock: left; width: 20")
	w := txt("aaaa bbbb cccc dddd eeee ffff gggg", "wrap: wrap")
	b := n("box#b", "", side, w)
	status := txt("status", "")
	run(t, n("col", "", b, status), 42, 6)
	if geo(b) != "42x2@(0,0)" || geo(w) != "22x2@(20,0)" || status.Y != 2 {
		t.Errorf("box %s text %s status y %d", geo(b), geo(w), status.Y)
	}
}

// 58 (and v1 §11.3). In a viewport, docked children are pulled out of
// the content extent, as its measure counts them, and scroll with it: they
// are never laid out in flow (no L007 in a grid), and the extent reaches
// the last row. In a grid scroll with a 20-cell left dock, the grid gets
// 22 columns (2 grid columns, 3 rows, extent 5); in a plain scroll the
// extent is the number of in-flow rows. Follow counts the in-flow
// children only, below a top dock.
func TestViewportDocksAreCarved(t *testing.T) {
	mk := func() (*Box, *Box, []*Box) {
		side := txt("SIDE", "dock: left; width: 20")
		var cs []*Box
		for i := 0; i < 6; i++ {
			cs = append(cs, txt(fmt.Sprintf("c%d", i), ""))
		}
		sv := n2("scroll#sv", "layout: grid; grid-columns: 4; grid-min-width: 10; gap: 1", append([]*Box{side}, cs...)...)
		return sv, side, cs
	}
	sv, side, cs := mk()
	e := run(t, n("col", "", sv, txt("status", "")), 42, 8)
	if len(e.Diags) != 0 {
		t.Fatal(e.Diags)
	}
	if sv.ContentH != 5 || geo(side) != "20x5@(0,0)" || geo(cs[0]) != "10x1@(20,0)" || geo(cs[5]) != "11x1@(31,4)" {
		t.Errorf("grid scroll: extent %d side %s c0 %s c5 %s", sv.ContentH, geo(side), geo(cs[0]), geo(cs[5]))
	}
	// 3 rows of viewport: the last row can be scrolled to.
	sv, side, cs = mk()
	sv.ScrollY = 99
	run(t, n("col", "", sv, txt("status", "")), 42, 4)
	if sv.H != 3 || sv.ScrollY != 2 || cs[5].Y != 2 || side.Y != -2 {
		t.Errorf("scrolled: h %d offset %d c5 y %d side y %d", sv.H, sv.ScrollY, cs[5].Y, side.Y)
	}

	// A grid that scrolls on x: Wc is the content-box width (42) minus the
	// dock (20), so 2 columns of 10 and 11 at x 20 and 31.
	sv, _, cs = mk()
	sv.Axis = "x"
	run(t, n("col", "", sv), 42, 8)
	if geo(cs[1]) != "11x1@(31,0)" || geo(cs[2]) != "10x1@(20,2)" {
		t.Errorf("x scroll: c1 %s c2 %s", geo(cs[1]), geo(cs[2]))
	}

	// A plain scroll: 8 rows beside a 6-cell dock.
	side = txt("SIDE", "dock: left; width: 6")
	var rows []*Box
	for i := 0; i < 8; i++ {
		rows = append(rows, txt(fmt.Sprintf("r%d", i), ""))
	}
	plain := n("scroll#p", "height: 4", append([]*Box{side}, rows...)...)
	e = run(t, n("col", "", plain), 20, 6)
	if len(e.Diags) != 0 {
		t.Fatal(e.Diags)
	}
	if plain.ContentH != 8 || geo(side) != "6x8@(0,0)" || geo(rows[0]) != "14x1@(6,0)" || geo(rows[7]) != "14x1@(6,7)" {
		t.Errorf("plain scroll: extent %d side %s r0 %s r7 %s", plain.ContentH, geo(side), geo(rows[0]), geo(rows[7]))
	}

	// Follow: in-flow child 5 (children index 6, after the top dock) is
	// at content y 1 + 5 = 6; a 3-row viewport scrolls to 4.
	top := txt("TOP", "dock: top")
	var its []*Box
	for i := 0; i < 6; i++ {
		its = append(its, txt(fmt.Sprintf("i%d", i), ""))
	}
	fl := n("scroll#f", "height: 3", append([]*Box{top}, its...)...)
	fl.Follow = 6
	run(t, n("col", "", fl), 20, 6)
	if fl.ContentH != 7 || fl.ScrollY != 4 || its[5].Y != 2 || top.Y != -4 {
		t.Errorf("follow: extent %d offset %d i5 y %d top y %d", fl.ContentH, fl.ScrollY, its[5].Y, top.Y)
	}
}

// 54. The tab strip tiers and label positions (§6.10.4): W1 and W2 with
// the mark slot and the gap; tier 3 is the active label alone.
func TestTabStripTiers(t *testing.T) {
	strip := func(cols int, active int) string {
		tabs := n2("tabs#t", "gap: 2")
		tabs.Mark = "▸"
		texts := [][2]string{{"1 overview", "1 ovr"}, {"7 processes", "7 proc"}, {"8 settings", "8 cfg"}}
		var tab *Box
		for i, tx := range texts {
			l := n("text", "")
			l.Role, l.Full, l.Short, l.Fixed, l.Selected = RoleTabLabel, tx[0], tx[1], true, i == active
			l.Parent = tabs
			tabs.Children = append(tabs.Children, l)
		}
		tab = n("tab", "", txt("panel", ""))
		tab.Parent = tabs
		tabs.Children = append(tabs.Children, tab)
		run(t, n("col", "", tabs), cols, 3)
		var out []string
		for _, l := range tabs.Children[:3] {
			if l.Laid {
				out = append(out, fmt.Sprintf("%s@%d+%d", l.Text, l.X, l.W))
			}
		}
		if geo(tab) != fmt.Sprintf("%dx1@(0,1)", cols) {
			out = append(out, "panel "+geo(tab))
		}
		return strings.Join(out, " ")
	}
	for _, c := range []struct {
		cols, active int
		want         string
	}{
		{40, 1, "1 overview@0+11 7 processes@13+12 8 settings@27+11"},
		{40, 2, "1 overview@0+11 7 processes@13+12 8 settings@27+11"},
		{38, 0, "1 overview@0+11 7 processes@13+12 8 settings@27+11"},
		{37, 0, "1 ovr@0+6 7 proc@8+7 8 cfg@17+6"},
		{23, 2, "1 ovr@0+6 7 proc@8+7 8 cfg@17+6"},
		{22, 1, "‹ 7 processes ›@0+16"},
		{12, 1, "‹ 7 proc ›@0+11"},
		{8, 1, "‹ 7 pr…@0+8"},
		{1, 1, "@0+1"},
	} {
		if got := strip(c.cols, c.active); got != c.want {
			t.Errorf("%d cols, active %d: %s, want %s", c.cols, c.active, got, c.want)
		}
	}
}
