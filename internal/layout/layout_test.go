package layout

import (
	"fmt"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// n builds a box. kind may carry an id ("box#inbox"); decls is a style=""
// body applied on top of the built-in sheet.
func n(kind, decls string, kids ...*Box) *Box {
	id := ""
	if i := strings.IndexByte(kind, '#'); i >= 0 {
		kind, id = kind[:i], kind[i+1:]
	}
	b := &Box{Tag: kind, Kind: kind, ID: id, Follow: -1, Index: -1}
	if decls != "" {
		ds, err := css.ParseDecls(decls)
		if err != nil {
			panic(err)
		}
		b.Inline = ds
	}
	for _, k := range kids {
		k.Parent = b
		b.Children = append(b.Children, k)
	}
	return b
}

// txt builds a text box.
func txt(s, decls string) *Box {
	b := n("text", decls)
	b.Text = s
	return b
}

func styleTree(c *css.Cascade, b *Box, parent *css.Style) {
	b.Style = c.Compute(b, parent, b.Hints, b.Inline)
	for _, k := range b.Children {
		styleTree(c, k, &b.Style)
	}
}

// run styles (built-in sheet + inline decls) and lays out root and modals.
func run(t *testing.T, root *Box, cols, rows int, modals ...*Box) *Engine {
	t.Helper()
	c := css.NewCascade(nil, css.Env{Cols: cols, Rows: rows})
	styleTree(c, root, nil)
	for _, m := range modals {
		styleTree(c, m, nil)
	}
	if len(c.Diags) != 0 {
		t.Fatalf("cascade: %v", c.Diags)
	}
	e := &Engine{File: "t.tui"}
	e.Layout(root, modals, cols, rows)
	return e
}

func geo(b *Box) string { return fmt.Sprintf("%dx%d@(%d,%d)", b.W, b.H, b.X, b.Y) }

func byID(root *Box, id string) *Box {
	var out *Box
	root.Walk(func(b *Box) {
		if b.ID == id && out == nil {
			out = b
		}
	})
	return out
}

func expectGeo(t *testing.T, root *Box, want map[string]string) {
	t.Helper()
	for id, w := range want {
		b := byID(root, id)
		if b == nil {
			t.Errorf("no box #%s", id)
			continue
		}
		if got := geo(b); got != w {
			t.Errorf("#%s = %s, want %s", id, got, w)
		}
	}
}

func codesOf(e *Engine) string {
	var out []string
	for _, d := range e.Diags {
		out = append(out, d.Code)
	}
	return strings.Join(out, " ")
}

// SPEC §16.2: the spike geometry through the layout engine alone.
func TestSpikeGeometry(t *testing.T) {
	for _, c := range []struct {
		cols                  int
		inbox, detail, status string
	}{
		{80, "24x23@(0,0)", "56x23@(24,0)", "80x1@(0,23)"},
		{120, "36x23@(0,0)", "84x23@(36,0)", "120x1@(0,23)"},
		{40, "12x23@(0,0)", "28x23@(12,0)", "40x1@(0,23)"},
	} {
		root := n("col#app", "width: 100%; height: 100%",
			n("col#root", "width: 1fr; height: 1fr",
				n("row#body", "width: 1fr; height: 1fr",
					n("box#inbox", "width: 30%; height: 1fr; border: single", txt("Inbox", "")),
					n("box#detail", "width: 1fr; height: 1fr; border: single", txt("Detail", "")),
				),
				n("box#status", "width: 1fr; height: 1", txt("ready", "")),
			),
		)
		e := run(t, root, c.cols, 24)
		if len(e.Diags) != 0 {
			t.Errorf("%d cols: %v", c.cols, e.Diags)
		}
		expectGeo(t, root, map[string]string{"app": fmt.Sprintf("%dx24@(0,0)", c.cols), "inbox": c.inbox, "detail": c.detail, "status": c.status})
		inboxText := byID(root, "inbox").Children[0]
		if inboxText.X != 1 || inboxText.Y != 1 {
			t.Errorf("%d cols: text sits at the content origin, got (%d,%d)", c.cols, inboxText.X, inboxText.Y)
		}
	}
}

// SPEC §11.4 allocation arithmetic on a 100-cell row.
func TestAllocateMainAxis(t *testing.T) {
	cases := []struct {
		name  string
		row   string
		kids  []string // decls per child box
		want  []string // "w@x" per child
		codes string
	}{
		{"fixed + fr", "", []string{"width: 10", "width: 1fr"}, []string{"10@0", "90@10"}, ""},
		{"percent is of the parent, not of remain", "", []string{"width: 20", "width: 50%", "width: 1fr"}, []string{"20@0", "50@20", "30@70"}, ""},
		{"percent floors", "", []string{"width: 33%", "width: 1fr"}, []string{"33@0", "67@33"}, ""},
		{"fr floor split, remainder to the last fr", "width: 10", []string{"width: 1fr", "width: 1fr", "width: 1fr"}, []string{"3@0", "3@3", "4@6"}, ""},
		{"weighted fr", "width: 10", []string{"width: 1fr", "width: 2fr", "width: 1fr"}, []string{"2@0", "5@2", "3@7"}, ""},
		{"fractional fr weights", "width: 10", []string{"width: 0.5fr", "width: 1.5fr"}, []string{"2@0", "8@2"}, ""},
		{"flex is an fr alias on the main axis", "", []string{"width: 20", "flex: 1", "flex: 3"}, []string{"20@0", "20@20", "60@40"}, ""},
		{"flex 0 keeps the size", "", []string{"width: 20; flex: 0", "width: 1fr"}, []string{"20@0", "80@20"}, ""},
		{"flex 0 does not join the fr pool", "", []string{"flex: 1", "flex: 0"}, []string{"100@0", "0@100"}, ""},
		{"flex 0 without a size is auto next to fr", "", []string{"width: 1fr", "flex: 0"}, []string{"100@0", "0@100"}, ""},
		{"decimal fr weights split exactly", "width: 30", []string{"width: 0.1fr", "width: 0.2fr"}, []string{"10@0", "20@10"}, ""},
		{"equal decimal weights split evenly", "width: 86", []string{"width: 0.1fr", "width: 0.1fr"}, []string{"43@0", "43@43"}, ""},
		{"decimal flex weights split exactly", "width: 30", []string{"flex: 0.1", "flex: 0.2"}, []string{"10@0", "20@10"}, ""},
		{"decimal percent is exact", "width: 375", []string{"width: 18.4%", "width: 1fr"}, []string{"69@0", "306@69"}, ""},
		{"sibling-fr rule: unspecified becomes 1fr", "", []string{"", "width: 1fr"}, []string{"50@0", "50@50"}, ""},
		{"no fr: unspecified is auto (empty box = 0)", "", []string{"", "width: 10"}, []string{"0@0", "10@0"}, ""},
		{"gap between in-flow children", "gap: 2", []string{"width: 10", "width: 50%", "width: 1fr"}, []string{"10@0", "50@12", "36@64"}, ""},
		{"max clamps fixed", "", []string{"width: 50; max-width: 20", "width: 1fr"}, []string{"20@0", "80@20"}, ""},
		{"min clamps percent before the fr split", "", []string{"width: 10%; min-width: 30", "width: 1fr"}, []string{"30@0", "70@30"}, ""},
		{"max percent bound", "", []string{"width: 80; max-width: 25%", "width: 1fr"}, []string{"25@0", "75@25"}, ""},
		{"fr clamps after the split without redistribution", "", []string{"width: 1fr; max-width: 10", "width: 1fr"}, []string{"10@0", "50@10"}, ""},
		{"min on fr may overflow", "", []string{"width: 1fr; min-width: 70", "width: 1fr"}, []string{"70@0", "50@70"}, ""},
		{"margins are part of the slot", "", []string{"width: 10; margin: 1 2", "width: 1fr"}, []string{"10@2", "86@14"}, ""},
		{"fixed overflow is clipped, not reflowed (L003)", "width: 20", []string{"width: 15", "width: 15"}, []string{"15@0", "15@15"}, "L003"},
		{"fixed overflow leaves fr at 0", "width: 20", []string{"width: 25", "width: 1fr"}, []string{"25@0", "0@25"}, "L003"},
		{"percent over 100 does not warn", "width: 20", []string{"width: 150%"}, []string{"30@0"}, ""},
		{"min over the parent warns (L003)", "", []string{"width: 10; min-width: 120"}, []string{"120@0"}, "L003"},
		{"fr mins over the parent warn", "", []string{"width: 1fr; min-width: 60", "width: 1fr; min-width: 60"}, []string{"60@0", "60@60"}, "L003"},
		{"percent mins over the parent warn", "", []string{"width: 50%; min-width: 70", "width: 50%; min-width: 70"}, []string{"70@0", "70@70"}, "L003"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var kids []*Box
			for i, d := range c.kids {
				kids = append(kids, n(fmt.Sprintf("box#k%d", i), d))
			}
			rowDecl := "width: 100; height: 5"
			if c.row != "" {
				rowDecl += "; " + c.row
			}
			root := n("screen", "", n("row#r", rowDecl, kids...))
			e := run(t, root, 200, 10)
			for i, w := range c.want {
				k := kids[i]
				if got := fmt.Sprintf("%d@%d", k.W, k.X); got != w {
					t.Errorf("child %d (%s) = %s, want %s", i, c.kids[i], got, w)
				}
			}
			if got := codesOf(e); got != c.codes {
				t.Errorf("diagnostics = %q, want %q (%v)", got, c.codes, e.Diags)
			}
		})
	}
}

func TestAllocateColumn(t *testing.T) {
	// A col with a fixed header, an fr body, and a gap.
	root := n("screen", "",
		n("col#c", "gap: 1",
			n("box#a", "height: 2"),
			n("box#b", "height: 2"),
			n("box#c3", "height: 1fr"),
			n("box#d", "height: 25%"),
		),
	)
	run(t, root, 30, 24)
	// remain = 24 - 3 gaps = 21; a=2, b=2, d=floor(24*25/100)=6, c3=21-10=11.
	expectGeo(t, root, map[string]string{
		"a": "30x2@(0,0)", "b": "30x2@(0,3)", "c3": "30x11@(0,6)", "d": "30x6@(0,18)",
	})
}

func TestAutoSizes(t *testing.T) {
	root := n("screen", "",
		n("row#r", "height: 3",
			txt("hello", ""),
			n("box#b", "border: single", txt("abc", "")),
			n("box#p", "padding: 0 2", txt("xy", "")),
			n("box#e", ""),
		),
		n("col#c", "height: auto",
			txt("one", ""), txt("two", ""),
		),
		n("box#w", "width: 10", txt("aaa bbb ccc ddd", "wrap: wrap")),
	)
	run(t, root, 80, 24)
	r := byID(root, "r")
	if got := geo(r.Children[0]); got != "5x3@(0,0)" {
		t.Errorf("auto text width = %s (height stretches)", got)
	}
	expectGeo(t, root, map[string]string{
		"b": "5x3@(5,0)",  // "abc" + 2 border
		"p": "6x3@(10,0)", // "xy" + 2*2 padding
		"e": "0x3@(16,0)", // empty auto box
		"c": "80x2@(0,3)", // two lines of text
		"w": "10x2@(0,5)", // wrapped: "aaa bbb" / "ccc ddd"
	})
}

// §11.2: box in a row is auto wide, box in a col is auto tall; cross axis
// fills (align: stretch).
func TestDefaultSizesAndCrossAxis(t *testing.T) {
	root := n("screen", "",
		n("row#r", "height: 5",
			n("box#inrow", "", txt("abcd", "")),
		),
		n("col#c",
			"",
			n("box#incol", "", txt("x", "")),
		),
	)
	run(t, root, 20, 24)
	expectGeo(t, root, map[string]string{
		"r":     "20x5@(0,0)",
		"inrow": "4x5@(0,0)",
		"c":     "20x19@(0,5)",
		"incol": "20x1@(0,5)",
	})
}

func TestAlignAndJustify(t *testing.T) {
	cases := []struct {
		name, row string
		kids      []string
		want      []string // "WxH@(x,y)" relative to the row origin (0,0)
	}{
		{"stretch by default", "", []string{"width: 4"}, []string{"4x5@(0,0)"}},
		{"align start keeps auto height", "align: start", []string{"width: 4; height: auto"}, []string{"4x0@(0,0)"}},
		{"align center", "align: center", []string{"width: 4; height: 1"}, []string{"4x1@(0,2)"}},
		{"align end", "align: end", []string{"width: 4; height: 2"}, []string{"4x2@(0,3)"}},
		{"justify start", "justify: start", []string{"width: 4"}, []string{"4x5@(0,0)"}},
		{"justify center", "justify: center", []string{"width: 4"}, []string{"4x5@(8,0)"}},
		{"justify end", "justify: end", []string{"width: 4"}, []string{"4x5@(16,0)"}},
		{"space-between", "justify: space-between", []string{"width: 2", "width: 2", "width: 2"}, []string{"2x5@(0,0)", "2x5@(9,0)", "2x5@(18,0)"}},
		{"space-between remainder goes last", "justify: space-between; width: 21", []string{"width: 2", "width: 2", "width: 2"}, []string{"2x5@(0,0)", "2x5@(9,0)", "2x5@(19,0)"}},
		{"justify is ignored with fr children", "justify: end", []string{"width: 4", "width: 1fr"}, []string{"4x5@(0,0)", "16x5@(4,0)"}},
		{"justify with gap", "justify: end; gap: 1", []string{"width: 4", "width: 4"}, []string{"4x5@(11,0)", "4x5@(16,0)"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var kids []*Box
			for i, d := range c.kids {
				kids = append(kids, n(fmt.Sprintf("box#k%d", i), d))
			}
			decl := "width: 20; height: 5"
			if c.row != "" {
				decl += "; " + c.row
			}
			root := n("screen", "", n("row", decl, kids...))
			run(t, root, 40, 10)
			for i, w := range c.want {
				if got := geo(kids[i]); got != w {
					t.Errorf("child %d = %s, want %s", i, got, w)
				}
			}
		})
	}
	// align: center on a column centers auto-width text horizontally.
	col := n("screen", "", n("col", "width: 20; height: 3; align: center", txt("abcd", "")))
	run(t, col, 40, 10)
	if got := geo(col.Children[0].Children[0]); got != "4x1@(8,0)" {
		t.Errorf("centered text = %s", got)
	}
}

// SPEC §11.3 pass 1: docks carve strips, top/bottom first (full width),
// then left/right, in document order.
func TestDocks(t *testing.T) {
	root := n("screen", "",
		n("box#left", "dock: left; width: 10"),
		n("box#top", "dock: top; height: 2"),
		n("box#main", "height: 1fr"),
		n("box#bottom", "dock: bottom; height: 1"),
		n("box#right", "dock: right; width: 25%"),
	)
	e := run(t, root, 80, 24)
	if len(e.Diags) != 0 {
		t.Fatal(e.Diags)
	}
	expectGeo(t, root, map[string]string{
		"top":    "80x2@(0,0)",
		"bottom": "80x1@(0,23)",
		"left":   "10x21@(0,2)",
		"right":  "20x21@(60,2)", // 25% of the container's 80 cells, not of what the left dock left
		"main":   "50x21@(10,2)",
	})
}

func TestDockAutoMarginAndClamp(t *testing.T) {
	root := n("screen", "",
		n("box#top", "dock: top; margin: 1 2", txt("header", "")),
		n("box#bar", "dock: left; min-width: 6", txt("ab", "")),
		n("box#fill", "height: 1fr"),
	)
	run(t, root, 40, 12)
	expectGeo(t, root, map[string]string{
		"top":  "36x1@(2,1)", // auto height 1, margins around it
		"bar":  "6x9@(0,3)",  // intrinsic 2, clamped up to min-width 6
		"fill": "34x9@(6,3)",
	})
}

func TestDockDiagnostics(t *testing.T) {
	root := n("screen", "",
		n("box#a", "dock: top; height: 1fr", txt("x", "")),
		n("box#b", "dock: bottom; height: 1"),
		n("box#c", "dock: bottom; height: 1"),
		n("box#d", "dock: left; flex: 2", txt("yy", "")),
	)
	e := run(t, root, 40, 12)
	// #d: flex is a grow weight on the column screen's vertical main axis,
	// not on the left dock's horizontal axis, so it is not V010 (SPEC §9).
	if got := codesOf(e); got != "V010 L005" {
		t.Fatalf("codes = %q: %v", got, e.Diags)
	}
	if e.Diags[0].Severity != ir.Error || e.Diags[1].Severity != ir.Warning || e.Diags[0].ID != "a" || e.Diags[0].File != "t.tui" {
		t.Errorf("diags = %+v", e.Diags)
	}
	// V010 treats the size as auto.
	expectGeo(t, root, map[string]string{"a": "40x1@(0,0)", "d": "2x9@(0,1)"})
	// Layout twice with the same engine: diagnostics are not duplicated.
	e.Layout(root, nil, 40, 12)
	if got := codesOf(e); got != "V010 L005" {
		t.Errorf("after a second pass: %q", got)
	}
}

// Finding 21: flex only conflicts with a dock when the dock's axis is the
// parent's main axis; otherwise the dock keeps its explicit size.
func TestDockFlexAxis(t *testing.T) {
	col := n("screen", "",
		n("box#l", "dock: left; width: 5; flex: 1; border: single", txt("L", "")),
		txt("flow", ""),
	)
	e := run(t, col, 20, 6)
	if len(e.Diags) != 0 {
		t.Errorf("left dock + flex in a column: %v", e.Diags)
	}
	expectGeo(t, col, map[string]string{"l": "5x6@(0,0)"})
	row := n("screen", "", n("row#r", "",
		n("box#l", "dock: left; width: 5; flex: 1", txt("L", "")),
		n("box#t", "dock: top; height: 2; flex: 1"),
		txt("flow", ""),
	))
	e = run(t, row, 20, 6)
	if got := codesOf(e); got != "V010" || e.Diags[0].ID != "l" {
		t.Errorf("left dock + flex in a row: %q %v", got, e.Diags)
	}
	expectGeo(t, row, map[string]string{"l": "1x4@(0,2)", "t": "20x2@(0,0)"})
	if DockConflict(byID(row, "t").Style, true) || !DockConflict(byID(row, "t").Style, false) {
		t.Error("DockConflict: top dock + flex conflicts only in a column parent")
	}
}

// Finding 18: a docked child's % is of the container's content box, not of
// the area left by earlier docks.
func TestDockPercentOfContainer(t *testing.T) {
	root := n("screen", "",
		n("box#t", "dock: top; height: 50%; border: single", txt("top", "")),
		n("box#b", "dock: bottom; height: 50%; border: single", txt("bottom", "")),
	)
	run(t, root, 20, 20)
	expectGeo(t, root, map[string]string{"t": "20x10@(0,0)", "b": "20x10@(0,10)"})
	lr := n("screen", "",
		n("box#l", "dock: left; width: 50%"),
		n("box#r", "dock: right; width: 50%; max-width: 40%"),
	)
	run(t, lr, 20, 6)
	expectGeo(t, lr, map[string]string{"l": "10x6@(0,0)", "r": "8x6@(12,0)"})
}

// L001: an fr size on the scroll axis of a scroll/list has no leftover.
func TestL001FrInsideScroll(t *testing.T) {
	root := n("screen", "",
		n("scroll#s", "height: 5",
			n("box#fr", "height: 1fr", txt("a", "")),
		),
	)
	e := run(t, root, 20, 10)
	if got := codesOf(e); got != "L001" {
		t.Fatalf("codes = %q", got)
	}
	expectGeo(t, root, map[string]string{"fr": "20x1@(0,0)"}) // treated as auto
	// fr across the scroll axis is fine.
	ok := n("screen", "", n("scroll", "height: 5", n("box", "width: 1fr", txt("a", ""))))
	if e := run(t, ok, 20, 10); len(e.Diags) != 0 {
		t.Errorf("fr on the cross axis of a scroll: %v", e.Diags)
	}
}

// L002: % of a parent sized by its content on that axis.
func TestL002PercentOfAutoParent(t *testing.T) {
	main := n("screen", "",
		n("col", "",
			n("box#auto", "", // auto height in a col
				n("box#pct", "height: 50%", txt("x", "")),
			),
		),
	)
	e := run(t, main, 20, 10)
	if got := codesOf(e); got != "L002" {
		t.Fatalf("main axis: codes = %q", got)
	}
	if e.Diags[0].ID != "pct" || !strings.Contains(e.Diags[0].Msg, "height: 50%") {
		t.Errorf("diag = %+v", e.Diags[0])
	}
	expectGeo(t, main, map[string]string{"pct": "20x1@(0,0)"}) // treated as auto

	cross := n("screen", "",
		n("row", "",
			n("box#auto", "", // auto width in a row
				n("box#pct", "width: 50%", txt("abc", "")),
			),
		),
	)
	e = run(t, cross, 20, 10)
	if got := codesOf(e); got != "L002" {
		t.Fatalf("cross axis: codes = %q", got)
	}
	// A percent of a sized parent is fine.
	fine := n("screen", "", n("col", "", n("box", "height: 4", n("box#p", "height: 50%"))))
	if e := run(t, fine, 20, 10); len(e.Diags) != 0 {
		t.Errorf("sized parent: %v", e.Diags)
	}
	expectGeo(t, fine, map[string]string{"p": "20x2@(0,0)"})
}

// §11.1: width/height describe the outer box; the border is inside it and
// negative content clamps to 0.
func TestBoxModel(t *testing.T) {
	root := n("screen", "",
		n("box#b", "width: 10; height: 4; border: double; padding: 1 2"),
		n("box#tiny", "width: 1; height: 1; border: single; padding: 1"),
	)
	run(t, root, 20, 10)
	b := byID(root, "b")
	if geo(b) != "10x4@(0,0)" || b.Content != (Rect{3, 2, 4, 0}) {
		t.Errorf("box = %s content %+v", geo(b), b.Content)
	}
	tiny := byID(root, "tiny")
	if geo(tiny) != "1x1@(0,4)" || tiny.Content.W != 0 || tiny.Content.H != 0 {
		t.Errorf("tiny = %s content %+v", geo(tiny), tiny.Content)
	}
	// Every border style counts as one cell.
	for _, st := range []string{"single", "double", "rounded", "thick"} {
		r := n("screen", "", n("box#x", "height: 3; border: "+st, txt("a", "")))
		run(t, r, 10, 5)
		if c := byID(r, "x").Content; c != (Rect{1, 1, 8, 1}) {
			t.Errorf("border %s content = %+v", st, c)
		}
	}
}

func TestClipToParentContent(t *testing.T) {
	root := n("screen", "",
		n("box#outer", "width: 10; height: 4; border: single",
			n("box#inner", "width: 20; height: 1"),
		),
	)
	run(t, root, 40, 10)
	inner := byID(root, "inner")
	if geo(inner) != "20x1@(1,1)" {
		t.Errorf("inner keeps its size: %s", geo(inner))
	}
	if inner.Clip != (Rect{1, 1, 8, 1}) {
		t.Errorf("inner clip = %+v, want the parent's content box", inner.Clip)
	}
}

func TestScrollOffsetClamping(t *testing.T) {
	lines := make([]*Box, 30)
	for i := range lines {
		lines[i] = txt(fmt.Sprintf("line %d", i), "")
	}
	for _, c := range []struct {
		in, want int
	}{
		{0, 0}, {5, 5}, {20, 20}, {100, 20}, {-5, 0},
	} {
		root := n("screen", "", n("scroll#s", "height: 10", lines...))
		s := byID(root, "s")
		s.ScrollY = c.in
		run(t, root, 20, 24)
		if s.ScrollY != c.want || s.ContentH != 30 {
			t.Errorf("offset %d -> %d (content %d), want %d", c.in, s.ScrollY, s.ContentH, c.want)
		}
		if first := s.Children[0]; first.Y != -c.want {
			t.Errorf("offset %d: first line at y=%d", c.in, first.Y)
		}
	}
	// Content shorter than the viewport never scrolls.
	short := n("screen", "", n("scroll#s", "height: 10", txt("a", "")))
	byID(short, "s").ScrollY = 3
	run(t, short, 20, 24)
	if s := byID(short, "s"); s.ScrollY != 0 || s.ContentH != 10 {
		t.Errorf("short content: offset %d content %d", s.ScrollY, s.ContentH)
	}
	// Horizontal scrolling.
	wide := n("screen", "", n("scroll#s", "width: 5; height: 1; layout: row", txt("abcdefghij", "")))
	byID(wide, "s").Axis = "x"
	byID(wide, "s").ScrollX = 99
	run(t, wide, 20, 24)
	if s := byID(wide, "s"); s.ScrollX != 5 || s.ContentW != 10 {
		t.Errorf("x scroll: offset %d content %d", s.ScrollX, s.ContentW)
	}
}

func TestListFollow(t *testing.T) {
	mk := func() (*Box, *Box) {
		items := make([]*Box, 10)
		for i := range items {
			items[i] = n("item", "", txt(fmt.Sprintf("row %d", i), ""))
			items[i].Index = i
		}
		root := n("screen", "", n("list#l", "height: 3", items...))
		return root, byID(root, "l")
	}
	for _, c := range []struct {
		follow, offset, want int
	}{
		{0, 0, 0},
		{2, 0, 0},
		{3, 0, 1},  // just below: align the bottom edge
		{7, 0, 5},  // far below
		{9, 0, 7},  // last item
		{2, 5, 2},  // above the viewport: align the top edge
		{6, 5, 5},  // already visible: keep the offset
		{-1, 4, 4}, // no follow
	} {
		root, l := mk()
		l.Follow, l.ScrollY = c.follow, c.offset
		run(t, root, 20, 10)
		if l.ScrollY != c.want {
			t.Errorf("follow %d from offset %d -> %d, want %d", c.follow, c.offset, l.ScrollY, c.want)
		}
		if c.follow >= 0 {
			it := l.Children[c.follow]
			if it.Y < l.Content.Y || it.Y >= l.Content.Y+l.Content.H {
				t.Errorf("follow %d: item at y=%d is outside the viewport", c.follow, it.Y)
			}
		}
	}
	// Scroll/list children with no layout default to a column.
	root, l := mk()
	run(t, root, 20, 10)
	if l.ContentH != 10 || l.Children[1].Y != 1 {
		t.Errorf("list content %d, item 1 at %d", l.ContentH, l.Children[1].Y)
	}
}

func TestModalLayout(t *testing.T) {
	cases := []struct {
		decls string
		want  string
	}{
		{"", "64x19@(8,2)"}, // default 80% x 80%, centered
		{"width: 40; height: 10", "40x10@(20,7)"},
		{"width: 50%; height: 50%", "40x12@(20,6)"},
		{"width: 200; height: 100", "80x24@(0,0)"}, // never larger than the screen
		{"width: auto; height: auto", "7x3@(36,10)"},
		{"width: 1fr; height: 1fr", "64x19@(8,2)"},
		{"width: 80%; max-width: 30; height: 10", "30x10@(25,7)"},
		{"width: 20; height: 5; min-height: 12", "20x12@(30,6)"},
		{"min-width: 200; max-height: 4", "80x4@(0,10)"},
	}
	for _, c := range cases {
		root := n("screen", "", n("box", "height: 1fr", txt("under", "")))
		m := n("modal#m", c.decls, txt("hello", ""))
		run(t, root, 80, 24, m)
		if got := geo(m); got != c.want {
			t.Errorf("modal %q = %s, want %s", c.decls, got, c.want)
		}
		if !m.Modal || !m.Laid {
			t.Error("modal flags")
		}
		if txt := m.Children[0]; txt.X != m.X+1 || txt.Y != m.Y+1 {
			t.Errorf("modal content at (%d,%d)", txt.X, txt.Y)
		}
	}
}

func TestRootSizes(t *testing.T) {
	for _, c := range []struct {
		decls string
		want  string
	}{
		{"", "30x10@(0,0)"},
		{"width: 50%; height: 50%", "15x5@(0,0)"},
		{"width: 12; height: 3", "12x3@(0,0)"},
		{"width: 99; height: 99", "30x10@(0,0)"},
		{"width: auto; height: auto", "30x10@(0,0)"},
		{"max-width: 20; min-height: 99", "20x10@(0,0)"},
	} {
		root := n("col", c.decls)
		run(t, root, 30, 10)
		if got := geo(root); got != c.want {
			t.Errorf("root %q = %s, want %s", c.decls, got, c.want)
		}
	}
}

// Auto-sized containers measure docks, gaps, margins, and wrapped text.
func TestIntrinsicContainers(t *testing.T) {
	root := n("screen", "",
		n("box#docked", "", // auto height in the screen column
			n("box", "dock: top; height: 2"),
			n("box", "dock: left; width: 3", txt("l", "")),
			txt("flow", ""),
		),
		n("box#rowbox", "layout: row; gap: 1",
			txt("aaa bbb ccc", "width: 5; wrap: wrap"),
			txt("x", "margin: 0 0 1 0"),
			n("box", "width: 50%"),
		),
		n("row#autorow", "width: auto; height: auto; gap: 2; padding: 1",
			txt("ab", ""), txt("cde", ""),
		),
	)
	run(t, root, 20, 24)
	expectGeo(t, root, map[string]string{
		"docked":  "20x3@(0,0)", // top dock 2 + max(flow 1, left dock 1)
		"rowbox":  "20x3@(0,3)", // the wrapped text is 3 lines at width 5
		"autorow": "20x3@(0,6)", // cross axis stretches; height = 1 + 2*1 padding
	})
	auto := byID(root, "autorow")
	if w := (&Engine{memo: map[memoKey]int{}}).intrinsic(auto, true, 0); w != 9 {
		t.Errorf("row intrinsic width = %d, want 2+2+3 + 2 padding", w)
	}
}

func TestIntrinsicWidgets(t *testing.T) {
	in := n("input#i", "")
	in.Text, in.Placeholder = "abc", "search here"
	btn := n("button#b", "")
	btn.Text = "OK"
	bbtn := n("button#bb", "border: single")
	bbtn.Text = "OK"
	root := n("screen", "",
		n("row#r", "height: 1",
			in, btn, bbtn,
			n("progress#p", ""),
			n("rule#ry", ""),
			n("spacer#sp", ""),
		),
		n("rule#rx", ""),
	)
	byID(root, "ry").Axis = "y"
	run(t, root, 80, 10)
	expectGeo(t, root, map[string]string{
		"i":  "12x1@(0,0)", // max(value, placeholder) + 1 for the cursor
		"b":  "6x1@(12,0)", // "[ OK ]"
		"bb": "4x1@(18,0)", // bordered: bare label + 2
		"p":  "10x1@(22,0)",
		"ry": "1x1@(32,0)",
		"sp": "47x1@(33,0)", // spacer { flex: 1 } takes the rest
		"rx": "80x1@(0,1)",
	})
	if ButtonLabel(btn) != "[ OK ]" || ButtonLabel(bbtn) != "OK" {
		t.Error("ButtonLabel")
	}
}

func TestPseudoState(t *testing.T) {
	b := n("item", "")
	if !b.HasPseudo("empty") {
		t.Error("an item with no children and no text is :empty")
	}
	b.Focused, b.Selected, b.Disabled = true, true, true
	for _, p := range []string{"focus", "selected", "disabled"} {
		if !b.HasPseudo(p) {
			t.Errorf(":%s", p)
		}
	}
	if b.HasPseudo("hover") {
		t.Error(":hover is not a state")
	}
	t2 := txt("x", "")
	if t2.HasPseudo("empty") {
		t.Error("text with content is not :empty")
	}
	var nilParent *Box = n("box", "")
	if nilParent.ParentElement() != nil {
		t.Error("root has no parent element")
	}
	if b.Direction() != "column" || n("row", "").Direction() != "row" {
		t.Error("Direction defaults")
	}
	withLayout := n("box", "")
	withLayout.Style.Layout = "row"
	if withLayout.Direction() != "row" {
		t.Error("layout: row on a box")
	}
	if n("text", "").IsContainer() || !n("modal", "").IsContainer() {
		t.Error("IsContainer")
	}
	if b.MatchTag() != "item" || b.MatchID() != "" || n("box#x", "").MatchID() != "x" {
		t.Error("MatchTag / MatchID")
	}
	child := n("text", "")
	n("box", "", child)
	if child.ParentElement() == nil || child.ParentElement().MatchTag() != "box" {
		t.Error("ParentElement")
	}
	cls := n("box", "")
	cls.Classes = []string{"a"}
	if !cls.HasClass("a") || cls.HasClass("b") {
		t.Error("HasClass")
	}
}

func TestRect(t *testing.T) {
	a := Rect{0, 0, 10, 10}
	if got := a.Intersect(Rect{5, 5, 10, 10}); got != (Rect{5, 5, 5, 5}) {
		t.Errorf("overlap = %+v", got)
	}
	if got := a.Intersect(Rect{20, 20, 5, 5}); got.W != 0 || got.H != 0 {
		t.Errorf("disjoint = %+v", got)
	}
	if !a.Contains(0, 0) || !a.Contains(9, 9) || a.Contains(10, 0) || a.Contains(-1, 0) {
		t.Error("Contains")
	}
}

func TestTextHelpers(t *testing.T) {
	if Width("ñandú─") != 6 || Width("") != 0 {
		t.Error("Width counts runes")
	}
	for _, c := range []struct {
		s    string
		n    int
		cut  string
		trnc string
	}{
		{"hello", 10, "hello", "hello"},
		{"hello", 5, "hello", "hello"},
		{"hello", 4, "hell", "hel…"},
		{"hello", 1, "h", "…"},
		{"hello", 0, "", ""},
		{"ñandú", 3, "ñan", "ña…"},
	} {
		if got := Cut(c.s, c.n); got != c.cut {
			t.Errorf("Cut(%q, %d) = %q", c.s, c.n, got)
		}
		if got := Truncate(c.s, c.n); got != c.trnc {
			t.Errorf("Truncate(%q, %d) = %q", c.s, c.n, got)
		}
	}
	for _, c := range []struct {
		text  string
		width int
		mode  string
		want  string
	}{
		{"", 10, "wrap", ""},
		{"a b c", 10, "", "a b c"},
		{"one\ntwo", 10, "nowrap", "one|two"},
		{"aaa bbb ccc", 7, "wrap", "aaa bbb|ccc"},
		{"aaa bbb ccc", 3, "wrap", "aaa|bbb|ccc"},
		{"abcdefgh", 3, "wrap", "abc|def|gh"},
		{"xx abcdefgh", 3, "wrap", "xx|abc|def|gh"},
		{"a\n\nb", 5, "wrap", "a||b"},
		{"a   b", 10, "wrap", "a b"},
		{"abc def", 0, "wrap", "abc def"},
		{"abcdef\nxy", 4, "truncate", "abc…|xy"},
		{"abcdef", 0, "truncate", "abcdef"},
	} {
		got := strings.Join(Lines(c.text, c.width, c.mode), "|")
		if got != c.want {
			t.Errorf("Lines(%q, %d, %q) = %q, want %q", c.text, c.width, c.mode, got, c.want)
		}
	}
	if MaxWidth("ab\nabcd\nc") != 4 || MaxWidth("") != 0 {
		t.Error("MaxWidth")
	}
}

func TestOverflowScrollIsAViewport(t *testing.T) {
	root := n("screen", "",
		n("box#v", "height: 2; overflow: scroll", txt("a", ""), txt("b", ""), txt("c", "")),
		n("box#h", "height: 2; overflow: hidden", txt("a", ""), txt("b", ""), txt("c", "")),
		n("text#t", "overflow: scroll"),
	)
	byID(root, "v").ScrollY = 5
	run(t, root, 10, 10)
	v, h := byID(root, "v"), byID(root, "h")
	if !v.Scrolls() || h.Scrolls() || byID(root, "t").Scrolls() {
		t.Fatal("only containers with overflow: scroll are viewports")
	}
	if v.ScrollY != 1 || v.ContentH != 3 || v.Children[0].Y != v.Y-1 {
		t.Errorf("viewport offset %d content %d first child y %d", v.ScrollY, v.ContentH, v.Children[0].Y)
	}
	// overflow: hidden keeps the flex layout: auto children overflow the
	// box and are clipped in paint.
	if h.Children[2].Y != h.Y+2 || h.Children[2].Clip.H != 0 {
		t.Errorf("hidden overflow: third child at %d clip %+v", h.Children[2].Y, h.Children[2].Clip)
	}
}

// mustScalar parses a size for the arithmetic tests.
func mustScalar(t *testing.T, s string) ir.Scalar {
	t.Helper()
	sc, err := ir.ParseScalar(s)
	if err != nil {
		t.Fatalf("ParseScalar(%q): %v", s, err)
	}
	return sc
}

// Finding 19: §11.4 splits are exact over the decimal literals. Brute force
// one-decimal weight pairs against integer arithmetic on tenths.
func TestFrSplitIsExact(t *testing.T) {
	for a := 1; a <= 30; a++ {
		for b := 1; b <= 30; b++ {
			wa := mustScalar(t, fmt.Sprintf("%d.%dfr", a/10, a%10))
			wb := mustScalar(t, fmt.Sprintf("%d.%dfr", b/10, b%10))
			for remain := 0; remain <= 150; remain++ {
				got := frSplit(remain, []ir.Scalar{wa, wb})
				first := remain * a / (a + b)
				if got[0] != first || got[1] != remain-first {
					t.Fatalf("frSplit(%d, %v, %v) = %v, want [%d %d]", remain, wa, wb, got, first, remain-first)
				}
			}
		}
	}
	for base := 0; base <= 400; base++ {
		for tenths := 0; tenths <= 1000; tenths += 7 {
			n := mustScalar(t, fmt.Sprintf("%d.%d%%", tenths/10, tenths%10))
			if got, want := percent(base, n), base*tenths/1000; got != want {
				t.Fatalf("percent(%d, %v) = %d, want %d", base, n, got, want)
			}
		}
	}
	// Absurd sizes clamp instead of overflowing.
	if got := cells(9223372036854775807); got != maxCells {
		t.Errorf("cells(max) = %d", got)
	}
	if got := percent(maxCells, ir.Scalar{Kind: ir.Pct, N: 1e15}); got != maxCells {
		t.Errorf("percent(huge) = %d", got)
	}
	if got := percent(maxCells, mustScalar(t, "99999999999999999999999999999999%")); got != maxCells {
		t.Errorf("percent(huge literal) = %d", got)
	}
}

// Round-2 finding 8: the exact arithmetic is on the decimal literal itself,
// not on its float64, even past float64 precision (~17 significant digits).
func TestExactArithmeticUsesTheLiteral(t *testing.T) {
	// floor(300 * 33.333333333333333333 / 100) = floor(99.999999999999999999) = 99;
	// the float64 of the literal is 33.333333333333336, which gives 100.
	if got := percent(300, mustScalar(t, "33.333333333333333333%")); got != 99 {
		t.Errorf("percent(300, 33.333333333333333333%%) = %d, want 99", got)
	}
	// floor(3 * 1 / 3.0000000000000001) = 0; the float64 of the second
	// weight is exactly 2, which would take the integer path and give 1.
	if got := frSplit(3, []ir.Scalar{mustScalar(t, "1fr"), mustScalar(t, "2.0000000000000001fr")}); got[0] != 0 || got[1] != 3 {
		t.Errorf("frSplit(3, 1fr 2.0000000000000001fr) = %v, want [0 3]", got)
	}
	// A weight too small for a float64 is still a positive weight.
	tiny := mustScalar(t, "0."+strings.Repeat("0", 400)+"1fr")
	if got := frSplit(10, []ir.Scalar{tiny, mustScalar(t, "1fr")}); got[0] != 0 || got[1] != 10 {
		t.Errorf("frSplit(10, tiny 1fr) = %v, want [0 10]", got)
	}
	if got := frSplit(10, []ir.Scalar{mustScalar(t, "1fr"), tiny}); got[0] != 9 || got[1] != 1 {
		t.Errorf("frSplit(10, 1fr tiny) = %v, want [9 1]", got)
	}
	// Whole literals keep the integer fast path, however they are written.
	if got := frSplit(9, []ir.Scalar{mustScalar(t, "1.0fr"), mustScalar(t, "002fr")}); got[0] != 3 || got[1] != 6 {
		t.Errorf("frSplit(9, 1.0fr 002fr) = %v, want [3 6]", got)
	}
	// End to end: the dump geometry follows the literal.
	pct := n("screen", "", n("row#r", "width: 300; height: 3",
		n("box#a", "width: 33.333333333333333333%; height: 1"),
		n("box#b", "width: 1fr; height: 1"),
	))
	run(t, pct, 300, 5)
	expectGeo(t, pct, map[string]string{"a": "99x1@(0,0)", "b": "201x1@(99,0)"})
	fr := n("screen", "", n("row#r", "width: 3; height: 1",
		n("box#a", "width: 1fr"),
		n("box#b", "width: 2.0000000000000001fr"),
	))
	run(t, fr, 10, 2)
	expectGeo(t, fr, map[string]string{"a": "0x1@(0,0)", "b": "3x1@(0,0)"})
	// flex: N is Nfr, exactly too.
	fl := n("screen", "", n("row#r", "width: 3; height: 1",
		n("box#a", "flex: 1"),
		n("box#b", "flex: 2.0000000000000001"),
	))
	run(t, fl, 10, 2)
	expectGeo(t, fl, map[string]string{"a": "0x1@(0,0)", "b": "3x1@(0,0)"})
}

// Round-2 finding 6: the built-in sheet's col/row 1fr and spacer flex: 1
// are defaults the author never wrote; a docked <row>, <col>, or <spacer>
// without a size is not V010 (and is sized as auto). The same fr or flex
// written in the document still is.
func TestDockIgnoresBuiltInFrAndFlex(t *testing.T) {
	status := n("row#status", "dock: bottom", txt("status", ""))
	side := n("col#side", "dock: left", txt("side", ""))
	sp := n("spacer#sp", "dock: top")
	root := n("screen", "", status, side, sp, txt("body", ""))
	e := run(t, root, 40, 6)
	if len(e.Diags) != 0 {
		t.Errorf("docked row/col/spacer with built-in sizes: %v", e.Diags)
	}
	expectGeo(t, root, map[string]string{"status": "40x1@(0,5)", "side": "4x5@(0,0)", "sp": "40x0@(0,0)"})
	for _, st := range []css.Style{status.Style, side.Style, sp.Style} {
		if DockConflict(st, false) {
			t.Errorf("DockConflict(%+v) with only built-in fr/flex", st)
		}
	}
	for _, c := range []struct {
		name string
		box  *Box
	}{
		{"row height: 1fr", n("row#x", "dock: bottom; height: 1fr", txt("s", ""))},
		{"col width: 2fr", n("col#x", "dock: left; width: 2fr", txt("s", ""))},
		{"spacer flex: 1", n("spacer#x", "dock: top; flex: 1")},
		{"box flex: 3", n("box#x", "dock: bottom; flex: 3", txt("s", ""))},
	} {
		e := run(t, n("screen", "", c.box, txt("body", "")), 40, 6)
		if got := codesOf(e); got != "V010" {
			t.Errorf("%s written by the author: codes = %q, want V010", c.name, got)
		}
	}
	// A presentational attribute counts as written too.
	hinted := n("row#x", "dock: bottom", txt("s", ""))
	hinted.Hints = []css.Decl{{Prop: "height", Value: "1fr"}}
	if got := codesOf(run(t, n("screen", "", hinted, txt("body", "")), 40, 6)); got != "V010" {
		t.Errorf("height=\"1fr\" hint on a docked row: codes = %q, want V010", got)
	}
}

// Finding 17: a % child of a dock or modal sized by its content is L002,
// like a % child of any other auto parent.
func TestL002InAutoDockAndModal(t *testing.T) {
	dock := n("screen", "",
		n("box#d", "dock: top", n("box#q", "height: 50%", txt("in dock", ""))),
	)
	e := run(t, dock, 40, 3)
	if got := codesOf(e); got != "L002" || e.Diags[0].ID != "q" {
		t.Errorf("auto dock: codes = %q %v", got, e.Diags)
	}
	left := n("screen", "",
		n("box#d", "dock: left", n("box#q", "width: 50%", txt("in dock", ""))),
	)
	e = run(t, left, 40, 3)
	if got := codesOf(e); got != "L002" || e.Diags[0].ID != "q" {
		t.Errorf("auto left dock: codes = %q %v", got, e.Diags)
	}
	m := n("modal#m", "width: auto; height: auto",
		txt("a modal with some text", ""),
		n("box#p", "width: 50%", txt("half", "")),
	)
	e = run(t, n("screen", ""), 40, 8, m)
	if got := codesOf(e); got != "L002" || e.Diags[0].ID != "p" {
		t.Errorf("auto modal: codes = %q %v", got, e.Diags)
	}
	// A sized dock is not an auto parent.
	sized := n("screen", "",
		n("box#d", "dock: top; height: 4", n("box#q", "height: 50%", txt("x", ""))),
	)
	if e := run(t, sized, 40, 10); len(e.Diags) != 0 {
		t.Errorf("sized dock: %v", e.Diags)
	}
	expectGeo(t, sized, map[string]string{"q": "40x2@(0,0)"})
}

// Finding 20: left/right docks share the row under full-width top/bottom
// strips, so the intrinsic width is max(tb, lr + flow).
func TestIntrinsicWidthWithDocks(t *testing.T) {
	top := n("text#top", "dock: top")
	top.Text = "twenty-chars-wide-xx"
	left := n("text#left", "dock: left; width: 5")
	left.Text = "L"
	flow := n("text#flow", "")
	flow.Text = "abc"
	root := n("screen", "", n("row#r", "height: 6",
		n("box#c", "width: auto; border: single", top, left, flow),
		n("box#after", "border: single", txt("next", "")),
	))
	run(t, root, 40, 7)
	expectGeo(t, root, map[string]string{
		"c":     "22x6@(0,0)",
		"top":   "20x1@(1,1)",
		"left":  "5x3@(1,2)",
		"after": "6x6@(22,0)",
	})
}
