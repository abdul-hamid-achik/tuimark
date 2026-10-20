package parse_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// The expansion depth limit (the F2 review, approved by the owner): the
// tree `tuimark ir` prints, where an <include> or a <use> is a level
// holding what it inserted, nests at most MaxExpandedDepth levels, the
// root at 1 and the written ancestors counted. Past it, the innermost
// include or use being expanded reports V021 and inserts nothing, and no
// later include or use expands.

// irDepth is the depth of the tree `tuimark ir` prints from n: the
// as-written lists (Source, an include's or a use's children) where an
// element has them.
func irDepth(n *ir.Node) int {
	kids := n.Children
	if n.Source != nil {
		kids = n.Source
	}
	d := 0
	for _, c := range kids {
		d = max(d, irDepth(c))
	}
	return d + 1
}

// treeDepth is the depth of the frame's tree (Children only).
func treeDepth(n *ir.Node) int {
	d := 0
	for _, c := range n.Children {
		d = max(d, treeDepth(c))
	}
	return d + 1
}

// useChain is a chain of n+1 templates, each holding only a use of the
// next (element-less, but each use a level), the last a text, placed by a
// use in the screen, then a use of a one-text template.
func useChain(n int) string {
	var b strings.Builder
	b.WriteString("<tui>\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "<template id=\"t%d\"><use template=\"t%d\"/></template>\n", i, i+1)
	}
	fmt.Fprintf(&b, "<template id=\"t%d\"><text>x</text></template>\n", n)
	b.WriteString("<template id=\"later\"><text>later</text></template>\n")
	b.WriteString("<screen><use template=\"t0\"/><use template=\"later\"/></screen>\n</tui>")
	return b.String()
}

// tui is 1, screen 2, the use of t0 3, the use of tK 3+K: the chain of 252
// puts its text at 256, the limit; 253 puts it at 257, and 254 puts the
// last use itself there. Both report at the use of t253, the innermost
// use whose copy would cross, written in t252's body at line 254.
func TestExpansionDepthUseChain(t *testing.T) {
	if parse.MaxExpandedDepth != 256 {
		t.Fatalf("MaxExpandedDepth = %d", parse.MaxExpandedDepth)
	}
	doc := parse.Parse([]byte(useChain(252)), "app.tui")
	if got := v021(doc); len(got) != 0 {
		t.Fatalf("at the limit: %v", got)
	}
	if d := irDepth(doc.Root); d != 256 {
		t.Errorf("at the limit: the ir tree nests %d levels, want 256", d)
	}
	if got := screenTexts(doc); got != "x later" {
		t.Errorf("at the limit: screen texts %q", got)
	}
	want := `app.tui:254:21 use template="t253" exceeds the expansion depth limit of 256 levels: it inserts nothing, and no later include or use expands`
	for _, n := range []int{253, 254, 2000} {
		doc := parse.Parse([]byte(useChain(n)), "app.tui")
		if got := v021(doc); len(got) != 1 || got[0] != want {
			t.Errorf("chain of %d: V021 %q, want %q", n, got, want)
		}
		if d := irDepth(doc.Root); d > 256 {
			t.Errorf("chain of %d: the ir tree nests %d levels", n, d)
		}
		// The enclosing uses keep what they inserted (nothing but uses),
		// and the later use does not expand.
		if got := screenTexts(doc); got != "" {
			t.Errorf("chain of %d: screen texts %q, want none", n, got)
		}
	}
}

// screenTexts is the text of each text node of the first screen's tree,
// in order.
func screenTexts(doc *parse.Document) string {
	var out []string
	var walk func(n *ir.Node)
	walk = func(n *ir.Node) {
		if n.Kind == "text" {
			out = append(out, n.Text)
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(screenOf(doc))
	return strings.Join(out, " ")
}

// A chain of uses that each nest a col (review probe deepnoid, 33 000
// levels in 19 s and 2.6 GB before the limit): the use of tK stands at
// 3+2K and its col at 4+2K, so the col of t126 is at 256 and the use of
// t127 inside it would be at 257: V021 at the use of t126, in t125's body.
func TestExpansionDepthNestedUses(t *testing.T) {
	var b strings.Builder
	b.WriteString("<tui>\n")
	const n = 400
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "<template id=\"t%d\"><col><use template=\"t%d\"/></col></template>\n", i, i+1)
	}
	fmt.Fprintf(&b, "<template id=\"t%d\"><text>x</text></template>\n", n)
	b.WriteString("<screen><use template=\"t0\"/></screen>\n</tui>")
	doc := parse.Parse([]byte(b.String()), "app.tui")
	want := `app.tui:127:26 use template="t126" exceeds the expansion depth limit of 256 levels: it inserts nothing, and no later include or use expands`
	if got := v021(doc); len(got) != 1 || got[0] != want {
		t.Errorf("V021 %q, want %q", got, want)
	}
	if d := irDepth(doc.Root); d != 255 {
		t.Errorf("the ir tree nests %d levels, want 255 (the use of t126 at 255, empty)", d)
	}
	if d := treeDepth(doc.Root); d != 128 {
		t.Errorf("the frame's tree nests %d levels, want 128 (tui, screen, 126 cols)", d)
	}
}

// Includes add up the same way: five files, each nesting 60 cols and then
// an include of the next, each well under the parser's own 256. The
// include of f1 stands at 3, f1's cols at 4-63, the include of f2 at 64,
// and so on: f5's cols would start at 248 and pass 256, so the include of
// f5, written in f4, reports V021 and inserts nothing.
func TestExpansionDepthIncludeChain(t *testing.T) {
	files := map[string]string{}
	for i := 1; i <= 5; i++ {
		inner := "<text>end</text>"
		if i < 5 {
			inner = fmt.Sprintf("<include src=\"f%d.tui\"/>", i+1)
		}
		files[fmt.Sprintf("f%d.tui", i)] = "<fragment>" + strings.Repeat("<col>", 60) + inner + strings.Repeat("</col>", 60) + "</fragment>"
	}
	const app = "<tui>\n<screen>\n<include src=\"f1.tui\"/>\n<include src=\"f1.tui\"/>\n</screen>\n</tui>"
	doc := parse.ParseWith([]byte(app), "app.tui", memLoader(files, "app.tui", nil))
	want := `f4.tui:1:311 include src="f5.tui" exceeds the expansion depth limit of 256 levels: it inserts nothing, and no later include or use expands`
	if got := v021(doc); len(got) != 1 || got[0] != want {
		t.Errorf("V021 %q, want %q", got, want)
	}
	if d := irDepth(doc.Root); d != 247 {
		t.Errorf("the ir tree nests %d levels, want 247 (the include of f5, empty)", d)
	}
	// The second include of f1 does not expand.
	if src := doc.Root.Children[0].Source; len(src) != 2 || len(src[1].Children) != 0 {
		t.Errorf("the screen as written: %d entries, the second holding %d", len(src), len(src[1].Children))
	}
	// The same chain in the body of an unused template, as written (the
	// template at 2, as the screen): the definitions' own message.
	tdoc := parse.ParseWith([]byte("<tui>\n<template id=\"T\"><include src=\"f1.tui\"/></template>\n<screen id=\"main\"><text>x</text></screen>\n</tui>"), "app.tui", memLoader(files, "app.tui", nil))
	want = `f4.tui:1:311 include src="f5.tui" in <template id="T"> exceeds the expansion depth limit of 256 levels: it inserts nothing into the template's own checks, and no later include in a template definition expands`
	if got := v021(tdoc); len(got) != 1 || got[0] != want {
		t.Errorf("in a template: V021 %q, want %q", got, want)
	}
	// f5 with 8 cols puts its text at 256, which fits.
	files["f5.tui"] = "<fragment>" + strings.Repeat("<col>", 8) + "<text>end</text>" + strings.Repeat("</col>", 8) + "</fragment>"
	doc = parse.ParseWith([]byte(app), "app.tui", memLoader(files, "app.tui", nil))
	if got := v021(doc); len(got) != 0 {
		t.Errorf("at the limit: %v", got)
	}
	if d := irDepth(doc.Root); d != 256 {
		t.Errorf("at the limit: the ir tree nests %d levels, want 256", d)
	}
}
