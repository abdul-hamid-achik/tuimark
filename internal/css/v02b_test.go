package css

import (
	"fmt"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// SPEC v0.2b §5.1, §10: the version="2" properties, pseudo-classes, type
// selectors, and media feature, checked per document version; the token
// order under @media (theme); and the declaration trace of `inspect`.

func TestCheckDeclVersionGate(t *testing.T) {
	for _, c := range []struct {
		prop, val string
		v1Hint    bool // version="1": V003 with the version hint
		v2OK      bool
	}{
		{"layout", "grid", true, true},
		{"grid-columns", "3", true, true},
		{"grid-min-width", "22", true, true},
		{"scrollbar", "auto", true, true},
		{"bar", "eighths", true, true},
		{"layout", "row", false, true},
	} {
		err := CheckDeclIn(c.prop, c.val, false, false)
		if hinted := err != nil && strings.HasSuffix(err.Error(), ir.VersionHint); hinted != c.v1Hint {
			t.Errorf("v1 %s: %s: %v", c.prop, c.val, err)
		}
		if err := CheckDeclIn(c.prop, c.val, true, false); (err == nil) != c.v2OK {
			t.Errorf("v2 %s: %s: %v", c.prop, c.val, err)
		}
	}
	// The v1 check is CheckDecl, unchanged for version="1" values.
	if err := CheckDecl("layout", "grid"); err == nil || !strings.Contains(err.Error(), "want column | row") {
		t.Errorf("CheckDecl layout: grid = %v", err)
	}
	for _, c := range []struct{ prop, val string }{
		{"grid-columns", "0"}, {"grid-columns", "13"}, {"grid-columns", "1.5"}, {"grid-columns", "-1"},
		{"grid-columns", "+3"}, {"grid-columns", ""}, {"grid-min-width", "0"}, {"grid-min-width", "2.5"},
		{"scrollbar", "always"}, {"bar", "half"}, {"layout", "flex"},
	} {
		err := CheckDeclIn(c.prop, c.val, true, false)
		if err == nil || strings.HasSuffix(err.Error(), ir.VersionHint) {
			t.Errorf("v2 %s: %q = %v, want a plain V003", c.prop, c.val, err)
		}
	}
	for _, v := range []string{"1", "12"} {
		if err := CheckDeclIn("grid-columns", v, true, false); err != nil {
			t.Errorf("grid-columns: %s = %v", v, err)
		}
	}
	if err := CheckDeclIn("grid-min-width", "99999999999999999999", true, false); err != nil {
		t.Errorf("a huge grid-min-width is still an integer >= 1: %v", err)
	}
}

func TestVersionTwoPropertiesCompute(t *testing.T) {
	sh, diags := ParseSheetIn(`box { layout: grid; grid-columns: 4; grid-min-width: 22; scrollbar: auto; } progress { bar: eighths; }`, "a.tcss", true, false)
	if len(diags) > 0 {
		t.Fatal(diags)
	}
	c := NewCascade([]*Sheet{sh}, Env{Cols: 80, Rows: 24})
	st := c.Compute(&el{tag: "box"}, nil, nil, nil)
	if st.Layout != "grid" || st.GridColumns != 4 || st.GridMinWidth != 22 || st.Scrollbar != "auto" || st.Bar != "block" {
		t.Errorf("box %+v", st)
	}
	child := c.Compute(&el{tag: "text"}, &st, nil, nil)
	if child.GridColumns != 1 || child.GridMinWidth != 0 || child.Scrollbar != "none" {
		t.Errorf("the new properties are not inherited: %+v", child)
	}
	if p := c.Compute(&el{tag: "progress"}, nil, nil, nil); p.Bar != "eighths" {
		t.Errorf("progress bar %q", p.Bar)
	}
	for _, p := range []string{"grid-columns", "grid-min-width", "scrollbar", "bar"} {
		if !PropertyV2(p) {
			t.Errorf("%s is not marked version=\"2\"", p)
		}
	}
	if PropertyV2("layout") || len(PropertyOrder) != 35 || PropertyOrder[0] != "layout" || PropertyOrder[34] != "bar" {
		t.Errorf("property order %v", PropertyOrder)
	}
	for _, p := range PropertyOrder {
		if _, ok := props[p]; !ok {
			t.Errorf("PropertyOrder names %q, no such property", p)
		}
	}
	if len(props) != len(PropertyOrder) {
		t.Errorf("%d properties, %d in PropertyOrder", len(props), len(PropertyOrder))
	}
}

func TestSelectorVersionGate(t *testing.T) {
	for _, sel := range []string{"item:checked", "col:focus-within", "table", "column", "tabs", "tab", "sparkline", "hints > row", "#x > tab:focus"} {
		_, err := ParseSelectorIn(sel, false)
		if err == nil || !strings.HasSuffix(err.Error(), ir.VersionHint) {
			t.Errorf("v1 %q: %v", sel, err)
		}
		if _, err := ParseSelectorIn(sel, true); err != nil {
			t.Errorf("v2 %q: %v", sel, err)
		}
	}
	if _, err := ParseSelectorIn("box:hover", true); err == nil || !strings.Contains(err.Error(), ":checked :focus-within") {
		t.Errorf("v2 :hover = %v", err)
	}
	if _, err := ParseSelectorIn("widget", true); err == nil || strings.HasSuffix(err.Error(), ir.VersionHint) {
		t.Errorf("v2 unknown type = %v", err)
	}
	s, _ := ParseSelectorIn("col:focus-within > text:checked", true)
	if s.Specificity() != 1+10+1+10 {
		t.Errorf(":checked and :focus-within count +10: %d", s.Specificity())
	}
}

func TestThemeMedia(t *testing.T) {
	src := "@media (theme: light) { text { bold: true; } }\n@media (theme: dark) { text { italic: true; } }"
	sh, diags := ParseSheetIn(src, "a.tcss", true, false)
	if len(diags) > 0 {
		t.Fatal(diags)
	}
	if sh.Rules[0].Media.Text != "(theme: light)" || sh.Rules[0].Media.String() != "(theme: light)" {
		t.Errorf("media %+v", sh.Rules[0].Media)
	}
	for theme, bold := range map[string]bool{"light": true, "dark": false, "": false, "auto": false} {
		st := NewCascade([]*Sheet{sh}, Env{Cols: 80, Rows: 24, Theme: theme}).Compute(&el{tag: "text"}, nil, nil, nil)
		if st.Bold != bold || st.Italic == bold {
			t.Errorf("theme %q: bold %v italic %v", theme, st.Bold, st.Italic)
		}
	}
	_, diags = ParseSheetIn(src, "a.tcss", false, false)
	if len(diags) != 2 || !strings.HasSuffix(diags[0].Msg, ir.VersionHint) {
		t.Errorf("v1 theme media: %v", diags)
	}
	for _, bad := range []string{"@media (theme: auto) { text { bold: true; } }", "@media (theme: sepia) {}", "@media (colour: dark) {}"} {
		_, diags := ParseSheetIn(bad, "a.tcss", true, false)
		if len(diags) != 1 || diags[0].Code != "V003" || strings.HasSuffix(diags[0].Msg, ir.VersionHint) {
			t.Errorf("%q: %v", bad, diags)
		}
	}
	// A size feature still matches whatever the theme.
	m := &Media{Feature: "max-cols", N: 80}
	if !m.MatchesEnv(Env{Cols: 80, Theme: "light"}) || m.MatchesEnv(Env{Cols: 81}) {
		t.Error("size media")
	}
}

// §10.4: tokens apply in document order under the effective theme; the
// static token check and IR tokens count a theme block only for its theme.
func TestTokensPerTheme(t *testing.T) {
	base := ":root { --brand: #e0443e; }\n"
	light := "@media (theme: light) { :root { --brand: #c8102e; --only: red; } }\n"
	use := "#t { color: $only; }\n"
	sh, diags := ParseSheetIn(base+light+use, "a.tcss", true, false)
	if len(diags) > 0 {
		t.Fatal(diags)
	}
	for theme, want := range map[string]string{"dark": "#e0443e", "light": "#c8102e"} {
		c := NewCascade([]*Sheet{sh}, Env{Cols: 80, Rows: 24, Theme: theme})
		if c.Tokens["brand"] != want {
			t.Errorf("%s: brand %q", theme, c.Tokens["brand"])
		}
		if IRTokens([]*Sheet{sh}, theme)["brand"] != want {
			t.Errorf("IR %s: brand %q", theme, IRTokens([]*Sheet{sh}, theme)["brand"])
		}
	}
	if IRTokens([]*Sheet{sh}, "auto")["brand"] != "#e0443e" {
		t.Error("IR tokens: auto is dark")
	}
	if d := CheckTokens([]*Sheet{sh}, "dark"); len(d) != 1 || !strings.Contains(d[0].Msg, "$only") {
		t.Errorf("dark: %v", d)
	}
	if d := CheckTokens([]*Sheet{sh}, "light"); len(d) != 0 {
		t.Errorf("light: %v", d)
	}
	// The light block before the base :root loses.
	sh2, _ := ParseSheetIn(light+base, "a.tcss", true, false)
	if c := NewCascade([]*Sheet{sh2}, Env{Theme: "light"}); c.Tokens["brand"] != "#e0443e" {
		t.Errorf("a light block before the base: brand %q", c.Tokens["brand"])
	}
}

// The built-in sheet holds the 0.2b rules of §10.6 and parses as a
// version="2" sheet.
func TestUASheet02bRules(t *testing.T) {
	for _, want := range []string{"table  { layout: column; wrap: truncate; }", "table > item:selected { reverse: true; }", "tabs   { layout: column; }", "tab    { layout: column; }", "sparkline { width: auto; height: auto; }", "hints  { layout: row; gap: 2; }", "hints > row { gap: 1; }"} {
		if !strings.Contains(UASheet, want) {
			t.Errorf("UASheet lacks %q", want)
		}
	}
	c := NewCascade(nil, Env{Cols: 80, Rows: 24})
	if st := c.Compute(&el{tag: "table"}, nil, nil, nil); st.Wrap != "truncate" || st.Layout != "column" {
		t.Errorf("table %+v", st)
	}
	if st := c.Compute(&el{tag: "row", parent: &el{tag: "hints"}}, nil, nil, nil); st.Gap != 1 {
		t.Errorf("hints > row gap %d", st.Gap)
	}
}

// `inspect`'s trace: the winner, the losers highest priority first, the
// origins, and inherited properties.
func TestExplain(t *testing.T) {
	sh, diags := ParseSheetIn("text { color: red; }\n.hot { color: blue; }\n@media (min-cols: 10) { #t { color: green; } }\n", "a.tcss", true, false)
	if len(diags) > 0 {
		t.Fatal(diags)
	}
	c := NewCascade([]*Sheet{sh}, Env{Cols: 80, Rows: 24})
	parent := Style{Italic: true}
	e := &el{tag: "text", id: "t", classes: []string{"hot"}}
	hints := []Decl{{Prop: "width", Value: "5", Line: 3, Col: 9, File: "d.tui", Attr: `width="5"`}}
	inline := []Decl{{Prop: "width", Value: "7", Line: 3, Col: 20, File: "d.tui"}}
	st, tr := c.Explain(e, &parent, hints, inline)
	if st.Color.String() != "green" || st.Width.String() != "7" || !st.Italic {
		t.Errorf("style %+v", st)
	}
	col := tr["color"]
	if col == nil || col.Winner.Selector != "#t" || col.Winner.Media != "(min-cols: 10)" || col.Winner.Specificity != 100 || col.Winner.Line != 3 {
		t.Fatalf("color winner %+v", col)
	}
	var lost []string
	for _, l := range col.Losers {
		lost = append(lost, l.Selector+"="+l.Value)
	}
	if strings.Join(lost, " ") != ".hot=blue text=red" {
		t.Errorf("losers %v", lost)
	}
	w := tr["width"]
	if w.Winner.Origin != OriginInline || w.Winner.Selector != `style=""` || w.Winner.Specificity != 1000 {
		t.Errorf("width winner %+v", w.Winner)
	}
	if len(w.Losers) != 2 || w.Losers[0].Selector != `width="5"` || w.Losers[0].Origin != OriginHint || w.Losers[1].File != UAFile {
		t.Errorf("width losers %+v", w.Losers)
	}
	if it := tr["italic"]; it == nil || !it.Inherited || it.Winner != nil {
		t.Errorf("italic %+v", it)
	}
	if _, ok := tr["dock"]; ok {
		t.Error("an unset, non-inherited property has a trace")
	}
	// Compute and Explain agree.
	if got := c.Compute(e, &parent, hints, inline); got != st {
		t.Errorf("Compute %+v != Explain %+v", got, st)
	}
}

// Regression (review of 0.2b, SPEC §15.7): a declaration of a selector
// list is one declaration in the trace, however many selectors of the list
// match: it never overrides itself, its selector is the list as written,
// and its specificity is that of the highest-specificity matching
// selector. Computed values are what they were.
func TestExplainSelectorList(t *testing.T) {
	sh, diags := ParseSheetIn("text, .a { bold: true; }\n.b,   .a { dim: true; }\ntext , .a { color: red; }\n.z, text { color: blue; }\n", "l.tcss", true, false)
	if len(diags) > 0 {
		t.Fatal(diags)
	}
	c := NewCascade([]*Sheet{sh}, Env{Cols: 80, Rows: 24})
	e := &el{tag: "text", id: "t", classes: []string{"a", "b"}}
	st, tr := c.Explain(e, nil, nil, nil)
	if !st.Bold || !st.Dim || st.Color.String() != "red" {
		t.Errorf("style %+v", st)
	}
	for _, want := range []struct {
		prop, sel string
		spec      int
		losers    string
	}{
		{"bold", "text, .a", 10, ""},
		{"dim", ".b, .a", 10, ""},
		// .z does not match: `.z, text` counts with text (1) and loses to
		// `text , .a` (10); each declaration appears once.
		{"color", "text, .a", 10, ".z, text/1=blue"},
	} {
		pt := tr[want.prop]
		if pt == nil || pt.Winner == nil {
			t.Fatalf("%s: no trace", want.prop)
		}
		if pt.Winner.Selector != want.sel || pt.Winner.Specificity != want.spec {
			t.Errorf("%s winner %q (%d), want %q (%d)", want.prop, pt.Winner.Selector, pt.Winner.Specificity, want.sel, want.spec)
		}
		var lost []string
		for _, l := range pt.Losers {
			lost = append(lost, fmt.Sprintf("%s/%d=%s", l.Selector, l.Specificity, l.Value))
		}
		if strings.Join(lost, " ") != want.losers {
			t.Errorf("%s losers %v, want %q", want.prop, lost, want.losers)
		}
	}
	if got := c.Compute(e, nil, nil, nil); got != st {
		t.Errorf("Compute %+v != Explain %+v", got, st)
	}
	// The built-in sheet's list rule is reported whole.
	ua := NewCascade(nil, Env{Cols: 80, Rows: 24})
	_, tr = ua.Explain(&el{tag: "input"}, nil, nil, nil)
	if w := tr["width"]; w == nil || w.Winner.Selector != "input, button, progress, rule" || w.Winner.File != UAFile || len(w.Losers) != 0 {
		t.Errorf("ua width %+v", w)
	}
}

func TestFormatProp(t *testing.T) {
	st := Initial()
	st.Width, _ = ir.ParseScalar("30%")
	st.Pad = [4]int{1, 2, 3, 4}
	st.Color, _ = ParseLiteralColor("#ABC")
	st.FlexSet, st.FlexLit = true, "1.5"
	for prop, want := range map[string]string{
		"width": "30%", "padding": "1 2 3 4", "color": "#aabbcc", "flex": "1.5", "bold": "false",
		"grid-columns": "1", "scrollbar": "none", "bar": "block", "display": "flex", "gap": "0",
		"border": "none", "overflow": "hidden",
	} {
		if got, ok := FormatProp(st, prop); !ok || got != want {
			t.Errorf("%s = %q %v, want %q", prop, got, ok, want)
		}
	}
	for _, prop := range []string{"height", "background", "dock", "align", "content-align", "wrap", "grid-min-width", "layout", "min-width", "nope"} {
		if got, ok := FormatProp(st, prop); ok {
			t.Errorf("%s = %q, want no computed value", prop, got)
		}
	}
}
