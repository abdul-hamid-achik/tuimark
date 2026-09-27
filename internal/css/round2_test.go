package css

import (
	"strings"
	"testing"
)

// Round-2 finding 6: the cascade records whether the winning width,
// height, and flex came from the built-in sheet, so a dock can ignore the
// col/row 1fr and spacer flex: 1 defaults (layout.DockConflict).
func TestUAOriginOfSizeAndFlex(t *testing.T) {
	c := NewCascade(nil, Env{Cols: 80, Rows: 24})
	row := c.Compute(&el{tag: "row"}, nil, nil, nil)
	if !row.WidthUA || !row.HeightUA || row.Height.String() != "1fr" {
		t.Errorf("row defaults: WidthUA=%v HeightUA=%v height=%s", row.WidthUA, row.HeightUA, row.Height)
	}
	if sp := c.Compute(&el{tag: "spacer"}, nil, nil, nil); !sp.FlexUA || !sp.FlexSet {
		t.Errorf("spacer flex: FlexUA=%v FlexSet=%v", sp.FlexUA, sp.FlexSet)
	}
	if box := c.Compute(&el{tag: "box"}, nil, nil, nil); box.WidthUA || box.HeightUA || box.FlexUA {
		t.Errorf("nothing set on a box: %+v", box)
	}
	decl := func(prop, val string) []Decl { return []Decl{{Prop: prop, Value: val}} }
	// Each origin the document controls clears the flag.
	if st := c.Compute(&el{tag: "row"}, nil, decl("height", "1fr"), nil); st.HeightUA || !st.WidthUA {
		t.Errorf("hint height: HeightUA=%v WidthUA=%v", st.HeightUA, st.WidthUA)
	}
	if st := c.Compute(&el{tag: "col"}, nil, nil, decl("width", "2fr")); st.WidthUA || !st.HeightUA {
		t.Errorf("inline width: WidthUA=%v HeightUA=%v", st.WidthUA, st.HeightUA)
	}
	if st := c.Compute(&el{tag: "spacer"}, nil, nil, decl("flex", "1")); st.FlexUA {
		t.Error("inline flex: FlexUA")
	}
	author := NewCascade([]*Sheet{mustSheet(t, `row { height: 1fr; } spacer { flex: 2; }`)}, Env{Cols: 80, Rows: 24})
	if st := author.Compute(&el{tag: "row"}, nil, nil, nil); st.HeightUA || !st.WidthUA {
		t.Errorf("author height: HeightUA=%v WidthUA=%v", st.HeightUA, st.WidthUA)
	}
	if st := author.Compute(&el{tag: "spacer"}, nil, nil, nil); st.FlexUA || st.Flex != 2 {
		t.Errorf("author flex: FlexUA=%v Flex=%v", st.FlexUA, st.Flex)
	}
}

// Round-2 finding 8: flex keeps its exact literal (flex: N is Nfr).
func TestFlexKeepsItsLiteral(t *testing.T) {
	c := NewCascade(nil, Env{Cols: 80, Rows: 24})
	st := c.Compute(&el{tag: "box"}, nil, nil, []Decl{{Prop: "flex", Value: "2.0000000000000001"}})
	if st.FlexLit != "2.0000000000000001" || st.Flex != 2 || !st.FlexSet {
		t.Errorf("flex: lit %q float %v set %v", st.FlexLit, st.Flex, st.FlexSet)
	}
	tiny := "0." + strings.Repeat("0", 400) + "1"
	st = c.Compute(&el{tag: "box"}, nil, nil, []Decl{{Prop: "flex", Value: tiny}})
	if st.FlexLit != tiny || !(st.Flex > 0) {
		t.Errorf("tiny flex: lit %.20q float %v (the float must stay > 0 like the literal)", st.FlexLit, st.Flex)
	}
}

// Round-2 finding 9: a huge whole number of cells is a valid size (layout
// clamps it to 2^40), whatever the platform's float-to-int conversion does.
func TestHugeWholeCellSizes(t *testing.T) {
	for _, v := range []string{"99999999999999999999", "9223372036854775807", "9223372036854775808"} {
		for _, prop := range []string{"width", "height", "min-width", "max-height"} {
			if err := CheckDecl(prop, v); err != nil {
				t.Errorf("%s: %s: %v", prop, v, err)
			}
		}
	}
	if err := CheckDecl("width", "1.00000000000000000001"); err == nil || !strings.Contains(err.Error(), "cells are integers") {
		t.Errorf("width: 1.00000000000000000001 = %v, want cells are integers", err)
	}
	if err := CheckDecl("width", "0."+strings.Repeat("0", 400)+"1fr"); err != nil {
		t.Errorf("tiny fr weight: %v", err)
	}
	// Past ir.MaxDecimalLen a literal is rejected with a clear message
	// (exact arithmetic on it would cost quadratic time per frame).
	for _, prop := range []string{"width", "flex"} {
		if err := CheckDecl(prop, strings.Repeat("1", 1001)); err == nil || !strings.Contains(err.Error(), "at most 1000 characters") {
			t.Errorf("%s: 1001-digit literal = %v", prop, err)
		}
	}
}

// Round-2 finding 11: style="" declarations (parse.Document.InlineDecls)
// get the same static token check as sheet rules, with the diagnostic the
// cascade gives the same declaration when it renders, so the two dedupe.
func TestCheckTokensInline(t *testing.T) {
	root := mustSheet(t, `:root { --brand: magenta; }`)
	inline := []Decl{
		{Prop: "color", Value: "$typo2", Line: 7, Col: 28, File: "a.tui"},
		{Prop: "background", Value: "var(--typo3)", Line: 4, Col: 24, File: "a.tui"},
		{Prop: "color", Value: "$brand", Line: 5, Col: 1, File: "a.tui"},
		{Prop: "color", Value: "$accent", Line: 5, Col: 1, File: "a.tui"},
		{Prop: "bold", Value: "true", Line: 5, Col: 1, File: "a.tui"},
		// The same declaration twice (two passes over one node) is one diagnostic.
		{Prop: "color", Value: "$typo2", Line: 7, Col: 28, File: "a.tui"},
	}
	diags := CheckTokens([]*Sheet{root}, "dark", inline...)
	var got []string
	for _, d := range diags {
		got = append(got, d.String())
	}
	want := []string{
		"error V003 a.tui:4:24 background: unknown token $typo3",
		"error V003 a.tui:7:28 color: unknown token $typo2",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("CheckTokens(inline) =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	// Rendering the node reports exactly the same diagnostic.
	c := NewCascade([]*Sheet{root}, Env{Cols: 80, Rows: 24})
	c.Compute(&el{tag: "box"}, nil, nil, inline[:1])
	if len(c.Diags) != 1 || c.Diags[0].String() != want[1] {
		t.Errorf("render-time diagnostic %v, want %q", c.Diags, want[1])
	}
	// Without inline declarations the call is what it always was.
	if d := CheckTokens([]*Sheet{root}, "dark"); len(d) != 0 {
		t.Errorf("no inline: %v", d)
	}
}
