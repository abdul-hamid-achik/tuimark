package css

import (
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// el is a minimal Element for selector and cascade tests.
type el struct {
	tag, id string
	classes []string
	pseudos []string
	parent  *el
}

func (e *el) MatchTag() string { return e.tag }
func (e *el) MatchID() string  { return e.id }
func (e *el) HasClass(c string) bool {
	for _, x := range e.classes {
		if x == c {
			return true
		}
	}
	return false
}
func (e *el) HasPseudo(p string) bool {
	for _, x := range e.pseudos {
		if x == p {
			return true
		}
	}
	return false
}
func (e *el) ParentElement() Element {
	if e.parent == nil {
		return nil
	}
	return e.parent
}

func mustSheet(t *testing.T, src string) *Sheet {
	t.Helper()
	sh, diags := ParseSheet(src, "t.tcss")
	if len(diags) != 0 {
		t.Fatalf("ParseSheet: %v", diags)
	}
	return sh
}

func TestParseSheetErrors(t *testing.T) {
	cases := []struct {
		name, src string
		line, col int
		msg       string
	}{
		{"unknown property", "box {\n  colour: red;\n}", 2, 3, `unknown property "colour"`},
		{"px unit", "box { width: 10px; }", 1, 7, "bad size"},
		{"em unit", "box { height: 2em; }", 1, 7, "bad size"},
		{"fraction of a cell", "box { width: 2.5; }", 1, 7, "cells are integers"},
		{"negative", "box { width: -3; }", 1, 7, "bad size"},
		{"bad enum", "box { layout: grid; }", 1, 7, "want column | row"},
		{"bad border", "box { border: dashed; }", 1, 7, "want none | single"},
		{"gap range", "box { gap: 5; }", 1, 7, "0-4"},
		{"gap two values", "box { gap: 1 2; }", 1, 7, "0-4"},
		{"padding units", "box { padding: 1px; }", 1, 7, "cells only"},
		{"margin five values", "box { margin: 1 1 1 1 1; }", 1, 7, "1-4 cell values"},
		{"flex negative", "box { flex: -1; }", 1, 7, "number >= 0"},
		{"flex inf", "box { flex: inf; }", 1, 7, "number >= 0"},
		{"flex nan", "box { flex: NaN; }", 1, 7, "number >= 0"},
		{"flex hex float", "box { flex: 0x1p4; }", 1, 7, "number >= 0"},
		{"bad bool", "box { bold: yes; }", 1, 7, "want true or false"},
		{"bad color", "box { color: blurple; }", 1, 7, "bad color"},
		{"bad hex color", "box { color: #12345; }", 1, 7, "bad color"},
		{"bad token ref", "box { color: $1x; }", 1, 7, "bad token reference"},
		{"empty value", "box { color: ; }", 1, 7, "empty value"},
		{"no colon", "box { bold }", 1, 7, "expected 'property: value'"},
		{"important", "box { bold: true !important; }", 1, 7, "!important"},
		{"nested rule", "box {\n  text { bold: true; }\n}", 2, 3, "nested rules are not supported"},
		{"custom property outside root", "box { --x: red; }", 1, 7, "only allowed in :root"},
		{"missing brace", "box bold: true;", 1, 1, "expected '{'"},
		{"stray close", "}\nbox { bold: true; }", 1, 1, "unexpected '}'"},
		{"unterminated block", "box { bold: true;", 1, 18, "unterminated declaration block"},
		{"unterminated comment", "box { bold: true; }\n/* x", 2, 1, "unterminated comment"},
		{"universal", "* { bold: true; }", 1, 1, "universal selector"},
		{"descendant", "row box { bold: true; }", 1, 1, "descendant combinator"},
		{"nesting", "& .a { bold: true; }", 1, 1, "nesting"},
		{"attribute selector", "box[id] { bold: true; }", 1, 1, "attribute selectors"},
		{"nth-child", "item:nth-child(2) { bold: true; }", 1, 1, ":nth-child is not supported"},
		{"nth-of-type", "item:nth-of-type { bold: true; }", 1, 1, "nth-of-type"},
		{"hover", "button:hover { bold: true; }", 1, 1, "pseudo-class :hover is not supported"},
		{"pseudo element", "text::before { bold: true; }", 1, 1, "pseudo-elements"},
		{"adjacent sibling", "box + text { bold: true; }", 1, 1, "sibling combinators"},
		{"general sibling", "box ~ text { bold: true; }", 1, 1, "sibling combinators"},
		{"unknown type", "widget { bold: true; }", 1, 1, `unknown type "widget"`},
		{"leading combinator", "> box { bold: true; }", 1, 1, "needs a selector on both sides"},
		{"trailing combinator", "box > { bold: true; }", 1, 1, "ends with a combinator"},
		{"empty id", "# { bold: true; }", 1, 1, "'#' needs an id"},
		{"empty class", ". { bold: true; }", 1, 1, "'.' needs a class name"},
		{"two ids", "#a#b { bold: true; }", 1, 1, "more than one #id"},
		{"empty selector in list", "box, { bold: true; }", 1, 1, "empty selector"},
		{"root in compound", "box:root { bold: true; }", 1, 1, ":root must stand alone"},
		{"root with others", ":root, box { --x: red; }", 1, 1, ":root must be the only selector"},
		{"root property", ":root { color: red; }", 1, 9, "only --token definitions"},
		{"root token not literal", ":root { --x: $fg; }", 1, 9, "token --x"},
		{"root bad token name", ":root { --1x: red; }", 1, 9, "bad token name"},
		{"media and", "@media (max-cols: 80) and (min-cols: 40) { }", 1, 1, "exactly one feature per block"},
		{"media or", "@media (max-cols: 80) or (min-rows: 3) { }", 1, 1, "exactly one feature per block"},
		{"media comma", "@media (max-cols: 80), (min-rows: 3) { }", 1, 1, "exactly one feature per block"},
		{"media type", "@media screen { }", 1, 1, "exactly one feature per block"},
		{"media width", "@media (max-width: 80) { }", 1, 1, `feature "max-width" is not supported`},
		{"media fraction", "@media (max-cols: 8.5) { }", 1, 1, "whole number"},
		{"media negative", "@media (max-cols: -1) { }", 1, 1, "whole number"},
		{"media no colon", "@media (max-cols) { }", 1, 1, "want (feature: N)"},
		{"media garbage", "@media x(max-cols: 3) { }", 1, 1, "want (feature: N)"},
		{"nested media", "@media (max-cols: 80) {\n  @media (min-cols: 40) { box { bold: true; } }\n}", 2, 3, "nested @media"},
		{"unterminated media", "@media (max-cols: 80) {\n  box { bold: true; }\n", 3, 1, "unterminated @media block"},
		{"import", "@import \"x.tcss\";", 1, 1, "unsupported at-rule"},
		{"font-face", "@font-face { }", 1, 1, "unsupported at-rule @font-face"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, diags := ParseSheet(c.src, "t.tcss")
			if len(diags) == 0 {
				t.Fatal("want a V003")
			}
			d := diags[0]
			if d.Code != "V003" || d.Severity != ir.Error || d.File != "t.tcss" {
				t.Errorf("diag = %+v", d)
			}
			if d.Line != c.line || d.Col != c.col {
				t.Errorf("at %d:%d, want %d:%d (%s)", d.Line, d.Col, c.line, c.col, d.Msg)
			}
			if !strings.Contains(d.Msg, c.msg) {
				t.Errorf("message %q does not contain %q", d.Msg, c.msg)
			}
		})
	}
}

func TestParseSheetRecovers(t *testing.T) {
	src := `
/* leading comment */
box { colour: red; bold: true; }
row box { dim: true; }
@media (max-cols: 80) and (min-cols: 1) { col { italic: true; } }
text { underline: true; }
`
	sh, diags := ParseSheet(src, "t.tcss")
	if len(diags) != 3 {
		t.Fatalf("want 3 diagnostics, got %v", diags)
	}
	if len(sh.Rules) != 2 {
		t.Fatalf("want the 2 valid rules kept, got %+v", sh.Rules)
	}
	if sh.Rules[0].Selectors[0].Text != "box" || len(sh.Rules[0].Decls) != 1 || sh.Rules[0].Decls[0].Prop != "bold" {
		t.Errorf("rule 0 = %+v (the bad declaration is dropped, the good one kept)", sh.Rules[0])
	}
	if sh.Rules[1].Selectors[0].Text != "text" || sh.Rules[1].Line != 6 {
		t.Errorf("rule 1 = %+v", sh.Rules[1])
	}
}

func TestParseSheetStructure(t *testing.T) {
	sh := mustSheet(t, `
:root { --accent: magenta; --brand-2: #abc; }
#header, #status { height: 1; background: $panel; }
list > item:selected { reverse: true; }
@media (max-cols: 80) {
  #body { layout: column; }
  /* comment inside media */
  #sidebar { width: 100%; height: 8 }
}
@media(min-rows:21){ #x { dim: true; } }
`)
	if len(sh.Rules) != 6 {
		t.Fatalf("rules = %d", len(sh.Rules))
	}
	if !sh.Rules[0].Selectors[0].Root || len(sh.Rules[0].Decls) != 2 {
		t.Errorf(":root rule = %+v", sh.Rules[0])
	}
	if len(sh.Rules[1].Selectors) != 2 || sh.Rules[1].Selectors[1].Parts[0].ID != "status" {
		t.Errorf("selector list = %+v", sh.Rules[1].Selectors)
	}
	child := sh.Rules[2].Selectors[0]
	if len(child.Parts) != 2 || child.Parts[0].Tag != "list" || child.Parts[1].Tag != "item" || child.Parts[1].Pseudos[0] != "selected" {
		t.Errorf("child selector = %+v", child)
	}
	for i := 3; i <= 4; i++ {
		if m := sh.Rules[i].Media; m == nil || m.Feature != "max-cols" || m.N != 80 {
			t.Errorf("rule %d media = %v", i, m)
		}
	}
	if d := sh.Rules[4].Decls; len(d) != 2 || d[1].Prop != "height" || d[1].Value != "8" {
		t.Errorf("last declaration without ';' = %+v", d)
	}
	if m := sh.Rules[4].Media.String(); m != "(max-cols: 80)" {
		t.Errorf("Media.String() = %q", m)
	}
	if m := sh.Rules[5].Media; m.Feature != "min-rows" || m.N != 21 {
		t.Errorf("media without spaces = %+v", m)
	}
	// "@media(min-rows:21)" without spaces.
	sh2 := mustSheet(t, "@media(min-rows:21){ #x { dim: true; } }")
	if m := sh2.Rules[0].Media; m.Feature != "min-rows" || m.N != 21 {
		t.Errorf("media = %+v", m)
	}
	// CRLF line endings keep positions.
	_, diags := ParseSheet("box {\r\n  bogus: 1;\r\n}", "t.tcss")
	if len(diags) != 1 || diags[0].Line != 2 || diags[0].Col != 3 {
		t.Errorf("CRLF diag = %v", diags)
	}
	// Inline <style> bodies report positions inside the .tui file.
	_, diags = ParseInlineSheet("\n  box { bogus: 1; }", "app.tui", 4, 10)
	if len(diags) != 1 || diags[0].Line != 5 || diags[0].Col != 9 || diags[0].File != "app.tui" {
		t.Errorf("inline diag = %v", diags)
	}
}

// A leading UTF-8 BOM (common from Windows editors) must not break the
// first rule; internal/parse's XML reader already strips one from .tui
// source, so a standalone .tcss file should be no less forgiving.
func TestParseSheetStripsBOM(t *testing.T) {
	sh, diags := ParseSheet("\uFEFF#a { width: 5; }\n", "bom.tcss")
	if len(diags) != 0 {
		t.Fatalf("diags = %v", diags)
	}
	if len(sh.Rules) != 1 || sh.Rules[0].Selectors[0].Parts[0].ID != "a" || sh.Rules[0].Decls[0].Value != "5" {
		t.Errorf("rules = %+v", sh.Rules)
	}
}

// SPEC §10.5: one feature per block; max-* and min-* are inclusive.
func TestMediaMatches(t *testing.T) {
	cases := []struct {
		feat       string
		n          int
		cols, rows int
		want       bool
	}{
		{"max-cols", 80, 80, 24, true},
		{"max-cols", 80, 81, 24, false},
		{"max-cols", 80, 40, 24, true},
		{"min-cols", 81, 81, 24, true},
		{"min-cols", 81, 80, 24, false},
		{"max-rows", 20, 80, 20, true},
		{"max-rows", 20, 80, 21, false},
		{"min-rows", 21, 80, 21, true},
		{"min-rows", 21, 80, 20, false},
		{"bogus", 1, 80, 24, false},
	}
	for _, c := range cases {
		m := &Media{Feature: c.feat, N: c.n}
		if got := m.Matches(c.cols, c.rows); got != c.want {
			t.Errorf("(%s: %d) at %dx%d = %v, want %v", c.feat, c.n, c.cols, c.rows, got, c.want)
		}
	}
	var none *Media
	if !none.Matches(1, 1) || none.String() != "" {
		t.Error("a rule outside @media always matches")
	}
}

// SPEC §10.1: type=1, class=10, id=100, pseudo +10.
func TestSpecificity(t *testing.T) {
	for _, c := range []struct {
		sel  string
		want int
	}{
		{"box", 1},
		{".a", 10},
		{"#a", 100},
		{":focus", 10},
		{"box.a", 11},
		{"box.a.b", 21},
		{"#a.b", 110},
		{"input:focus", 11},
		{"list > item:selected", 12},
		{"row > box#a.b:focus", 122},
		{"col > row > text", 3},
		{"#x:disabled:empty", 120},
	} {
		s, err := ParseSelector(c.sel)
		if err != nil {
			t.Fatalf("%s: %v", c.sel, err)
		}
		if got := s.Specificity(); got != c.want {
			t.Errorf("Specificity(%s) = %d, want %d", c.sel, got, c.want)
		}
	}
}

func TestSelectorMatching(t *testing.T) {
	screen := &el{tag: "screen", id: "main"}
	row := &el{tag: "row", id: "body", classes: []string{"chrome"}, parent: screen}
	box := &el{tag: "box", id: "inbox", classes: []string{"panel", "muted"}, pseudos: []string{"focus"}, parent: row}
	text := &el{tag: "text", parent: box}
	cases := []struct {
		sel  string
		e    *el
		want bool
	}{
		{"box", box, true},
		{"row", box, false},
		{"#inbox", box, true},
		{"#other", box, false},
		{".panel", box, true},
		{".panel.muted", box, true},
		{".panel.nope", box, false},
		{"box:focus", box, true},
		{"box:selected", box, false},
		{"row > box", box, true},
		{"screen > box", box, false}, // child, not descendant
		{"screen > row > box", box, true},
		{"row.chrome > #inbox:focus", box, true},
		{"box > text", text, true},
		{"row > text", text, false},
		{"screen > row", row, true},
		{"col > screen", screen, false}, // no parent
		{"#main", screen, true},
	}
	for _, c := range cases {
		s, err := ParseSelector(c.sel)
		if err != nil {
			t.Fatalf("%s: %v", c.sel, err)
		}
		if got := s.Matches(c.e); got != c.want {
			t.Errorf("%q matches %s#%s = %v, want %v", c.sel, c.e.tag, c.e.id, got, c.want)
		}
	}
	root, _ := ParseSelector(":root")
	if !root.Root || root.Matches(screen) {
		t.Error(":root never matches a node")
	}
	if (Selector{}).Matches(box) {
		t.Error("an empty selector matches nothing")
	}
	list, err := ParseSelectorList("#header, #status")
	if err != nil || len(list) != 2 || list[1].Text != "#status" {
		t.Errorf("ParseSelectorList = %+v, %v", list, err)
	}
}

// tui, style, keymap, and bind never become a laid-out Box (host.inflate
// builds none for them, and the screen — not <tui> — is the root of the
// matched/inherited tree), so a type selector naming one would otherwise
// parse cleanly and then silently never match anything.
func TestSelectorRejectsNonRenderedTypeTags(t *testing.T) {
	for _, sel := range []string{"tui", "style", "keymap", "bind", "tui > screen", "screen > style"} {
		if _, err := ParseSelector(sel); err == nil {
			t.Errorf("ParseSelector(%q) accepted a non-rendered type selector", sel)
		} else if !strings.Contains(err.Error(), "is not rendered") {
			t.Errorf("ParseSelector(%q) = %v, want a \"is not rendered\" error", sel, err)
		}
	}
	// screen and every widget tag still work.
	if _, err := ParseSelector("tui"); err == nil {
		t.Error("tui accepted")
	}
	for _, sel := range []string{"screen", "screen > box", "col > row > box"} {
		if _, err := ParseSelector(sel); err != nil {
			t.Errorf("ParseSelector(%q): %v", sel, err)
		}
	}
	if _, diags := ParseSheet("tui { color: red; }", "t.tcss"); len(diags) != 1 || diags[0].Code != "V003" {
		t.Errorf("tui{} in a sheet: diags = %v, want one V003", diags)
	}
}

func TestCheckDecl(t *testing.T) {
	good := [][2]string{
		{"layout", "row"}, {"layout", "column"}, {"dock", "top"}, {"dock", "left"},
		{"width", "12"}, {"width", "30%"}, {"width", "1fr"}, {"height", "auto"},
		{"min-width", "4"}, {"max-height", "50%"}, {"flex", "0"}, {"flex", "1"}, {"flex", "2.5"},
		// SPEC §10.2 gives min-*/max-* the Size grammar (N | N% | Nfr |
		// auto), same as width/height; only cells and percent constrain
		// anything at layout time (§11.3), but auto and fr are accepted
		// and simply have no effect, rather than being rejected outright.
		{"min-width", "auto"}, {"min-width", "1fr"}, {"max-height", "auto"}, {"max-width", "1fr"},
		{"min-height", "1fr"}, {"max-width", "auto"},
		{"gap", "0"}, {"gap", "4"}, {"padding", "1"}, {"padding", "1 2"}, {"padding", "1 2 3"},
		{"margin", "0 1 2 3"}, {"align", "stretch"}, {"justify", "space-between"},
		{"overflow", "scroll"}, {"content-align", "center"}, {"display", "none"},
		{"color", "$accent"}, {"color", "var(--accent)"}, {"background", "#0d1117"},
		{"border-color", "#fff"}, {"title-color", "bright-cyan"}, {"color", "default"},
		{"border", "rounded"}, {"bold", "true"}, {"reverse", "false"}, {"wrap", "truncate"},
		{"visibility", "hidden"},
	}
	for _, g := range good {
		if err := CheckDecl(g[0], g[1]); err != nil {
			t.Errorf("CheckDecl(%s: %s): %v", g[0], g[1], err)
		}
	}
	bad := [][2]string{
		{"float", "left"}, {"font-size", "12"}, {"layout", "grid"}, {"width", "10px"},
		{"width", "1vw"}, {"min-width", "10px"}, {"flex", "1e2"}, {"flex", "+1"},
		{"gap", "-1"}, {"gap", "1.5"}, {"padding", "1px"}, {"padding", ""}, {"margin", "-1"},
		{"align", "baseline"}, {"justify", "space-around"}, {"display", "block"},
		{"display", "grid"}, {"color", "rgb(1,2,3)"}, {"color", "#ggg"}, {"border", "1"},
		{"bold", "1"}, {"wrap", "break-word"}, {"visibility", "collapse"}, {"--accent", "red"},
		{"color", "var(--)"},
	}
	for _, b := range bad {
		if err := CheckDecl(b[0], b[1]); err == nil {
			t.Errorf("CheckDecl(%s: %s) accepted", b[0], b[1])
		}
	}
	if len(PropertyNames()) != len(props) {
		t.Error("PropertyNames must list every property")
	}
	for _, p := range PropertyNames() {
		if PropertyValues(p) == "" {
			t.Errorf("PropertyValues(%s) is empty", p)
		}
	}
	if PropertyValues("nope") != "" {
		t.Error("PropertyValues of an unknown property")
	}
}

func TestParseDecls(t *testing.T) {
	d, err := ParseDecls(" color: red ;width:5;; bold: true")
	if err != nil || len(d) != 3 || d[0].Prop != "color" || d[0].Value != "red" || d[1].Value != "5" {
		t.Fatalf("ParseDecls = %+v, %v", d, err)
	}
	for _, src := range []string{"color", "colour: red", "width: 10px", "--x: red"} {
		if _, err := ParseDecls(src); err == nil {
			t.Errorf("ParseDecls(%q) accepted", src)
		}
	}
}

// ParseDeclsAll recovers per declaration (like a stylesheet rule's block):
// one bad declaration is reported and dropped, every valid one — before or
// after it — is kept, unlike ParseDecls (an all-or-nothing wrapper kept
// for callers that only want a single pass/fail result).
func TestParseDeclsAllRecoversPerDeclaration(t *testing.T) {
	decls, errs := ParseDeclsAll("width: 3; bogus: 1; height: 9x; color: red")
	if len(errs) != 2 {
		t.Fatalf("errs = %v, want 2", errs)
	}
	var kept []string
	for _, d := range decls {
		kept = append(kept, d.Prop+"="+d.Value)
	}
	if strings.Join(kept, " ") != "width=3 color=red" {
		t.Errorf("kept decls = %v", kept)
	}
	if d, _ := ParseDeclsAll(""); len(d) != 0 {
		t.Errorf("empty style: %v", d)
	}
}

func TestColors(t *testing.T) {
	for _, c := range []struct {
		in   string
		want Color
		str  string
	}{
		{"default", Color{Kind: ColorDefault}, "default"},
		{"black", Color{Kind: ColorANSI, Index: 0}, "black"},
		{"cyan", Color{Kind: ColorANSI, Index: 6}, "cyan"},
		{"bright-black", Color{Kind: ColorANSI, Index: 8}, "bright-black"},
		{"bright-white", Color{Kind: ColorANSI, Index: 15}, "bright-white"},
		{"#0d1117", Color{Kind: ColorRGB, R: 0x0d, G: 0x11, B: 0x17}, "#0d1117"},
		{"#fff", Color{Kind: ColorRGB, R: 255, G: 255, B: 255}, "#ffffff"},
		{"#AbC", Color{Kind: ColorRGB, R: 0xaa, G: 0xbb, B: 0xcc}, "#aabbcc"},
	} {
		got, err := ParseLiteralColor(c.in)
		if err != nil || got != c.want {
			t.Errorf("ParseLiteralColor(%q) = %+v, %v", c.in, got, err)
		}
		if got.String() != c.str {
			t.Errorf("String() = %q, want %q", got.String(), c.str)
		}
		if !got.IsSet() {
			t.Errorf("%q IsSet() = false", c.in)
		}
	}
	for _, in := range []string{"", "Cyan", "grey", "#12", "#1234", "#12345g", "#+12345", "$fg", "rgb(0,0,0)"} {
		if _, err := ParseLiteralColor(in); err == nil {
			t.Errorf("ParseLiteralColor(%q) accepted", in)
		}
	}
	if (Color{}).IsSet() || (Color{}).String() != "" {
		t.Error("zero Color is unset")
	}
	tokens := map[string]string{"accent": "cyan", "loop": "$accent"}
	for _, c := range []struct {
		in   string
		want Color
	}{
		{"$accent", Color{Kind: ColorANSI, Index: 6}},
		{"var(--accent)", Color{Kind: ColorANSI, Index: 6}},
		{"var( --accent )", Color{}}, // "--" must follow "var(" directly
		{"red", Color{Kind: ColorANSI, Index: 1}},
	} {
		got, err := ResolveColor(c.in, tokens)
		if c.want == (Color{}) {
			if err == nil {
				t.Errorf("ResolveColor(%q) accepted", c.in)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("ResolveColor(%q) = %+v, %v", c.in, got, err)
		}
	}
	if _, err := ResolveColor("$nope", tokens); err == nil || !strings.Contains(err.Error(), "unknown token $nope") {
		t.Errorf("unknown token: %v", err)
	}
	if _, err := ResolveColor("$loop", tokens); err == nil || !strings.Contains(err.Error(), "refers to another token") {
		t.Errorf("token chain: %v", err)
	}
}

func TestThemesHaveRequiredTokens(t *testing.T) {
	// SPEC §10.4 default dark theme.
	want := map[string]string{
		"bg": "#0d1117", "fg": "#e6edf3", "muted": "#8b949e", "accent": "cyan", "ok": "green",
		"warn": "yellow", "danger": "red", "border": "#30363d", "focus": "cyan", "panel": "#161b22",
	}
	for k, v := range want {
		if Themes["dark"][k] != v {
			t.Errorf("dark $%s = %q, want %q", k, Themes["dark"][k], v)
		}
	}
	for name, th := range Themes {
		for _, tok := range RequiredTokens {
			v, ok := th[tok]
			if !ok {
				t.Errorf("theme %s lacks $%s", name, tok)
				continue
			}
			if _, err := ParseLiteralColor(v); err != nil {
				t.Errorf("theme %s $%s: %v", name, tok, err)
			}
		}
	}
	if len(RequiredTokens) != 10 {
		t.Errorf("SPEC lists 10 required tokens, have %d", len(RequiredTokens))
	}
}

func TestCascadeTokens(t *testing.T) {
	sh := mustSheet(t, `
:root { --accent: magenta; --brand: #102030; }
@media (max-cols: 40) { :root { --accent: yellow; } }
box { color: $accent; background: var(--brand); border-color: $fg; }
`)
	e := &el{tag: "box"}
	c := NewCascade([]*Sheet{sh}, Env{Cols: 80, Rows: 24})
	if len(c.Diags) != 0 {
		t.Fatal(c.Diags)
	}
	st := c.Compute(e, nil, nil, nil)
	if st.Color != (Color{Kind: ColorANSI, Index: 5}) {
		t.Errorf(":root token override: color = %v", st.Color)
	}
	if st.Background.String() != "#102030" {
		t.Errorf("var(--brand) = %v", st.Background)
	}
	if st.BorderColor.String() != "#e6edf3" {
		t.Errorf("$fg from the dark theme = %v", st.BorderColor)
	}
	if c.Tokens["fg"] != "#e6edf3" || c.Tokens["accent"] != "magenta" {
		t.Errorf("tokens = %v", c.Tokens)
	}
	narrow := NewCascade([]*Sheet{sh}, Env{Cols: 40, Rows: 24})
	if st := narrow.Compute(e, nil, nil, nil); st.Color != (Color{Kind: ColorANSI, Index: 3}) {
		t.Errorf("token defined inside @media: %v", st.Color)
	}
	light := NewCascade(nil, Env{Cols: 80, Rows: 24, Theme: "light"})
	if light.Tokens["accent"] != "blue" || len(light.Diags) != 0 {
		t.Errorf("light theme tokens = %v, %v", light.Tokens, light.Diags)
	}
	unknown := NewCascade(nil, Env{Cols: 80, Rows: 24, Theme: "neon"})
	if unknown.Tokens["accent"] != "cyan" {
		t.Error("an unknown theme falls back to dark")
	}
	for _, tok := range RequiredTokens {
		if _, ok := c.Tokens[tok]; !ok {
			t.Errorf("token $%s missing after theme apply", tok)
		}
	}
}

func TestCascadeUnknownTokenIsV003Once(t *testing.T) {
	sh := mustSheet(t, "box { color: $nope; bold: true; }")
	c := NewCascade([]*Sheet{sh}, Env{Cols: 80, Rows: 24})
	for i := 0; i < 3; i++ {
		st := c.Compute(&el{tag: "box"}, nil, nil, nil)
		if st.Color.IsSet() || !st.Bold {
			t.Fatalf("a failed declaration is skipped, others apply: %+v", st)
		}
	}
	if len(c.Diags) != 1 || c.Diags[0].Code != "V003" || c.Diags[0].Line != 1 || c.Diags[0].Col != 7 || !strings.Contains(c.Diags[0].Msg, "unknown token $nope") {
		t.Errorf("diags = %v", c.Diags)
	}
	inline := NewCascade(nil, Env{Cols: 80, Rows: 24})
	inline.Compute(&el{tag: "box"}, nil, nil, []Decl{{Prop: "color", Value: "$missing", Line: 3, Col: 5}})
	if len(inline.Diags) != 1 || inline.Diags[0].Line != 3 {
		t.Errorf("inline token diag = %v", inline.Diags)
	}
}

// A hint or style="" declaration has no Sheet of its own, so its diagnostic
// file must come from Decl.File; a sheet rule's file still comes from the
// Sheet (unaffected by Decl.File, which sheetParser-built decls leave "").
func TestCascadeDiagnosticUsesDeclFile(t *testing.T) {
	inline := NewCascade(nil, Env{Cols: 80, Rows: 24})
	inline.Compute(&el{tag: "box"}, nil,
		[]Decl{{Prop: "color", Value: "$nope", Line: 1, Col: 1, File: "app.tui"}},
		nil)
	if len(inline.Diags) != 1 || inline.Diags[0].File != "app.tui" {
		t.Errorf("hint diag = %+v, want File \"app.tui\"", inline.Diags)
	}

	inline2 := NewCascade(nil, Env{Cols: 80, Rows: 24})
	inline2.Compute(&el{tag: "box"}, nil, nil,
		[]Decl{{Prop: "color", Value: "$nope", Line: 3, Col: 28, File: "app.tui"}})
	if len(inline2.Diags) != 1 || inline2.Diags[0].File != "app.tui" || inline2.Diags[0].Line != 3 || inline2.Diags[0].Col != 28 {
		t.Errorf("inline diag = %+v, want File \"app.tui\" at 3:28", inline2.Diags)
	}

	sh := mustSheet(t, "box { color: $nope; }")
	fromSheet := NewCascade([]*Sheet{sh}, Env{Cols: 80, Rows: 24})
	fromSheet.Compute(&el{tag: "box"}, nil, nil, nil)
	if len(fromSheet.Diags) != 1 || fromSheet.Diags[0].File != "t.tcss" {
		t.Errorf("sheet-rule diag = %+v, want File \"t.tcss\" (from the Sheet, not Decl.File)", fromSheet.Diags)
	}
}

// Origins: UA < attribute hints < author (specificity, then order) < style="".
func TestCascadeOrigins(t *testing.T) {
	box := &el{tag: "box", id: "x", classes: []string{"a"}}
	cases := []struct {
		name          string
		author        string
		hints, inline []Decl
		want          string // width
	}{
		{"UA default", "", nil, nil, ""},
		{"hint beats UA", "", []Decl{{Prop: "width", Value: "30%"}}, nil, "30%"},
		{"author type selector beats hint", "box { width: 10; }", []Decl{{Prop: "width", Value: "30%"}}, nil, "10"},
		{"inline beats id selector", "#x { width: 10; }", nil, []Decl{{Prop: "width", Value: "3"}}, "3"},
		{"inline beats everything", "#x.a { width: 10; }", []Decl{{Prop: "width", Value: "7"}}, []Decl{{Prop: "width", Value: "3"}}, "3"},
		{"id beats class regardless of order", "#x { width: 1; } .a { width: 2; }", nil, nil, "1"},
		{"class beats type regardless of order", ".a { width: 2; } box { width: 3; }", nil, nil, "2"},
		{"later rule wins ties", "box { width: 4; } box { width: 5; }", nil, nil, "5"},
		{"later declaration wins in a rule", "box { width: 4; width: 6; }", nil, nil, "6"},
		{"later hint wins", "", []Decl{{Prop: "width", Value: "1"}, {Prop: "width", Value: "2"}}, nil, "2"},
		{"later inline wins", "", nil, []Decl{{Prop: "width", Value: "1"}, {Prop: "width", Value: "2"}}, "2"},
		{"media rule applies at size", "@media (max-cols: 80) { box { width: 9; } }", nil, nil, "9"},
		{"media rule skipped at size", "@media (min-cols: 81) { box { width: 9; } }", nil, nil, ""},
		{"media rule is ordered like any other", "@media (max-cols: 80) { box { width: 9; } } box { width: 8; }", nil, nil, "8"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var sheets []*Sheet
			if c.author != "" {
				sheets = append(sheets, mustSheet(t, c.author))
			}
			casc := NewCascade(sheets, Env{Cols: 80, Rows: 24})
			st := casc.Compute(box, nil, c.hints, c.inline)
			if got := st.Width.String(); got != c.want {
				t.Errorf("width = %q, want %q", got, c.want)
			}
		})
	}
	// Later sheets win ties against earlier ones.
	casc := NewCascade([]*Sheet{mustSheet(t, ".a { width: 1; }"), mustSheet(t, ".a { width: 2; }")}, Env{Cols: 80, Rows: 24})
	if st := casc.Compute(box, nil, nil, nil); st.Width.String() != "2" {
		t.Errorf("later sheet: width = %v", st.Width)
	}
	// Pseudo-classes add specificity: box:focus (11) beats .a (10).
	casc = NewCascade([]*Sheet{mustSheet(t, "box:focus { width: 1; } .a { width: 2; }")}, Env{Cols: 80, Rows: 24})
	focused := &el{tag: "box", classes: []string{"a"}, pseudos: []string{"focus"}}
	if st := casc.Compute(focused, nil, nil, nil); st.Width.String() != "1" {
		t.Errorf("pseudo specificity: width = %v", st.Width)
	}
	if st := casc.Compute(&el{tag: "box", classes: []string{"a"}}, nil, nil, nil); st.Width.String() != "2" {
		t.Errorf("unfocused: width = %v", st.Width)
	}
}

// SPEC §10.6 built-in defaults plus the documented extras.
func TestUADefaults(t *testing.T) {
	c := NewCascade(nil, Env{Cols: 80, Rows: 24})
	for _, x := range []struct {
		tag, layout, w, h string
	}{
		{"screen", "column", "100%", "100%"},
		{"col", "column", "1fr", "1fr"},
		{"row", "row", "1fr", "1fr"},
		{"box", "column", "", ""},
		{"text", "", "auto", "auto"},
		{"scroll", "column", "", ""},
		{"list", "column", "", ""},
		{"item", "column", "", ""},
		{"input", "", "auto", "auto"},
		{"button", "", "auto", "auto"},
		{"progress", "", "auto", "auto"},
		{"rule", "", "auto", "auto"},
	} {
		st := c.Compute(&el{tag: x.tag}, nil, nil, nil)
		if st.Layout != x.layout || st.Width.String() != x.w || st.Height.String() != x.h {
			t.Errorf("%s: layout=%q width=%q height=%q", x.tag, st.Layout, st.Width, st.Height)
		}
	}
	if st := c.Compute(&el{tag: "modal"}, nil, nil, nil); st.Border != "single" || st.Layout != "column" {
		t.Errorf("modal = %+v", st)
	}
	if st := c.Compute(&el{tag: "spacer"}, nil, nil, nil); !st.FlexSet || st.Flex != 1 {
		t.Errorf("spacer flex = %v", st.Flex)
	}
	list := &el{tag: "list"}
	if st := c.Compute(&el{tag: "item", pseudos: []string{"selected"}, parent: list}, nil, nil, nil); !st.Reverse {
		t.Error("list > item:selected is reversed")
	}
	if st := c.Compute(&el{tag: "item", pseudos: []string{"selected"}}, nil, nil, nil); st.Reverse {
		t.Error("a selected item outside a list is not reversed")
	}
	init := Initial()
	if init.Display != "flex" || init.Visibility != "visible" || init.Border != "none" || init.Overflow != "hidden" || init.Justify != "start" {
		t.Errorf("Initial() = %+v", init)
	}
}

func TestInheritance(t *testing.T) {
	c := NewCascade([]*Sheet{mustSheet(t, `
#p { color: red; bold: true; dim: true; italic: true; underline: true; visibility: hidden; wrap: wrap;
     background: blue; border: single; padding: 1; reverse: true; layout: row; gap: 2; }
#c2 { color: green; bold: false; }
`)}, Env{Cols: 80, Rows: 24})
	parent := &el{tag: "box", id: "p"}
	ps := c.Compute(parent, nil, nil, nil)
	child := c.Compute(&el{tag: "box", parent: parent}, &ps, nil, nil)
	if child.Color.String() != "red" || !child.Bold || !child.Dim || !child.Italic || !child.Underline || !child.Reverse || child.Visibility != "hidden" || child.Wrap != "wrap" {
		t.Errorf("inherited properties not inherited: %+v", child)
	}
	if child.Background.IsSet() || child.Border != "none" || child.Pad != [4]int{} || child.Layout != "column" || child.Gap != 0 {
		t.Errorf("non-inherited properties leaked: %+v", child)
	}
	own := c.Compute(&el{tag: "box", id: "c2", parent: parent}, &ps, nil, nil)
	if own.Color.String() != "green" || own.Bold {
		t.Errorf("a child's own value beats inheritance: %+v", own)
	}
	grand := c.Compute(&el{tag: "text"}, &own, nil, nil)
	if grand.Color.String() != "green" || !grand.Dim {
		t.Errorf("inheritance is transitive: %+v", grand)
	}
}

func TestApplyTypedValues(t *testing.T) {
	c := NewCascade([]*Sheet{mustSheet(t, `
box {
  dock: bottom; min-width: 3; min-height: 10%; max-width: 20; max-height: 50%;
  flex: 2.5; gap: 3; padding: 1 2; margin: 1 2 3; align: center; justify: end;
  overflow: scroll; content-align: end; display: none; title-color: $accent;
  border: thick; reverse: true;
}`)}, Env{Cols: 80, Rows: 24})
	st := c.Compute(&el{tag: "box"}, nil, nil, nil)
	if st.Dock != "bottom" || st.MinW.String() != "3" || st.MinH.String() != "10%" || st.MaxW.String() != "20" || st.MaxH.String() != "50%" {
		t.Errorf("sizes: %+v", st)
	}
	if !st.FlexSet || st.Flex != 2.5 || st.Gap != 3 || st.Pad != [4]int{1, 2, 1, 2} || st.Margin != [4]int{1, 2, 3, 2} {
		t.Errorf("box values: %+v", st)
	}
	if st.Align != "center" || st.Justify != "end" || st.Overflow != "scroll" || st.ContentAlign != "end" || st.Display != "none" || st.Border != "thick" || !st.Reverse {
		t.Errorf("enums: %+v", st)
	}
	if st.TitleColor.String() != "cyan" {
		t.Errorf("title-color = %v", st.TitleColor)
	}
	four, _ := parseBox("1 2 3 4")
	if four != [4]int{1, 2, 3, 4} {
		t.Errorf("parseBox 4 = %v", four)
	}
}
