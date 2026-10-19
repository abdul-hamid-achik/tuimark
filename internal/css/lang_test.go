package css

import (
	"fmt"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// errText is an error's message, or "" for nil.
func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// SPEC v0.4b §5.1: the ir.Lang variants check a pinned document exactly as
// the bool functions do for its version, and, while no 0.4b phase has
// added vocabulary, the current language as version="3".
func TestLangVariantsMatchTheBoolFunctions(t *testing.T) {
	decls := [][2]string{
		{"row-gap", "2"}, {"layout", "grid"}, {"wrap", "truncate-middle"}, {"bar", "eighths"},
		{"color", "$accent"}, {"bogus", "1"}, {"width", "1fr"},
	}
	sels := []string{"table", "tabs > tab", "list:checked", "box:focus-within", "tui", "#a.b", "x y"}
	sheet := "@media (theme: light) { :root { --a: #fff; } }\nhints { row-gap: 1; wrap: truncate-start; }\nlist:checked { bold: true; }\n"
	for _, l := range []ir.Lang{ir.Lang1, ir.Lang2, ir.Lang3, ir.Lang4} {
		v2, v3 := l.V2(), l.V3()
		for _, d := range decls {
			if a, b := errText(CheckDeclLang(d[0], d[1], l)), errText(CheckDeclIn(d[0], d[1], v2, v3)); a != b {
				t.Errorf("Lang(%d) %s: %q, bool function %q", l, d[0], a, b)
			}
		}
		for _, s := range sels {
			_, e1 := ParseSelectorLang(s, l)
			_, e2 := ParseSelectorIn(s, v2)
			_, e3 := ParseSelectorListLang(s, l)
			_, e4 := ParseSelectorListIn(s, v2)
			if errText(e1) != errText(e2) || errText(e3) != errText(e4) {
				t.Errorf("Lang(%d) selector %q: %q / %q, bool function %q / %q", l, s, e1, e3, e2, e4)
			}
		}
		_, d1 := ParseSheetLang(sheet, "t.tcss", l)
		_, d2 := ParseSheetIn(sheet, "t.tcss", v2, v3)
		_, d3 := ParseInlineSheetLang(sheet, "t.tui", 3, 5, l)
		_, d4 := ParseInlineSheetIn(sheet, "t.tui", 3, 5, v2, v3)
		if fmt.Sprint(d1) != fmt.Sprint(d2) || fmt.Sprint(d3) != fmt.Sprint(d4) {
			t.Errorf("Lang(%d) sheet: %v / %v, bool function %v / %v", l, d1, d3, d2, d4)
		}
		_, e1 := ParseDeclsAllLang("row-gap: 1; bar: eighths; nope: 1", l)
		_, e2 := ParseDeclsAllIn("row-gap: 1; bar: eighths; nope: 1", v2, v3)
		if fmt.Sprint(e1) != fmt.Sprint(e2) {
			t.Errorf("Lang(%d) style: %v, bool function %v", l, e1, e2)
		}
	}
}

// SPEC v0.4b §10.1, decision 94. A type selector naming a current-only tag
// carries the hint in a pinned document; one naming a composition tag
// (never a node) is the unknown type of 0.4.1 there, with no hint, and in
// the current language is "not a node". F1 adds the composition tags
// include and fragment; the test adds a current-only node tag for its
// duration.
func TestCurrentTagSelectors(t *testing.T) {
	for _, tag := range []string{"include", "fragment", "template", "use"} { // F1, F2
		for l, want := range map[ir.Lang]string{
			ir.Lang3: fmt.Sprintf(`selector %q: unknown type %q (not in the tag catalog)`, tag, tag),
			ir.Lang4: fmt.Sprintf(`selector %q: <%s> is not a node and never matches`, tag, tag),
		} {
			if _, err := ParseSelectorLang(tag, l); errText(err) != want {
				t.Errorf("Lang(%d): %s gives %v, want %s", l, tag, err, want)
			}
		}
	}
	kinds, nodes := ir.KindsCurrent, notANodeTags
	t.Cleanup(func() { ir.KindsCurrent, notANodeTags = kinds, nodes })
	ir.KindsCurrent = []string{"checkbox", "include"}
	notANodeTags = map[string]bool{"include": true}

	for _, c := range []struct {
		sel  string
		l    ir.Lang
		want string
	}{
		{"checkbox", ir.Lang1, `selector "checkbox": type "checkbox" (requires a <tui> without version)`},
		{"checkbox", ir.Lang3, `selector "checkbox": type "checkbox" (requires a <tui> without version)`},
		{"box > checkbox:focus", ir.Lang3, `selector "box > checkbox:focus": type "checkbox" (requires a <tui> without version)`},
		{"checkbox", ir.Lang4, ""},
		{"include", ir.Lang1, `selector "include": unknown type "include" (not in the tag catalog)`},
		{"include", ir.Lang3, `selector "include": unknown type "include" (not in the tag catalog)`},
		{"include", ir.Lang4, `selector "include": <include> is not a node and never matches`},
	} {
		if _, err := ParseSelectorLang(c.sel, c.l); errText(err) != c.want {
			t.Errorf("Lang(%d) %q: %q, want %q", c.l, c.sel, errText(err), c.want)
		}
	}
	// The bool functions have no current language: they read as pinned.
	if _, err := ParseSelectorIn("checkbox", true); errText(err) != `selector "checkbox": type "checkbox" (requires a <tui> without version)` {
		t.Errorf("ParseSelectorIn: %v", err)
	}
	// A stylesheet and a keymap when= go through the same check.
	if _, diags := ParseSheetLang("checkbox { bold: true; }", "t.tcss", ir.Lang3); len(diags) != 1 {
		t.Errorf("pinned sheet: %v", diags)
	}
	if _, diags := ParseSheetLang("checkbox { bold: true; }", "t.tcss", ir.Lang4); len(diags) != 0 {
		t.Errorf("current sheet: %v", diags)
	}
}
