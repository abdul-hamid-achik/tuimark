package css

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// fuzzChain is a small fixed document path for the cascade fuzzers:
// screen > col#main.a > list#l > item.b:selected > text#t.a.b
func fuzzChain() []*el {
	screen := &el{tag: "screen"}
	col := &el{tag: "col", id: "main", classes: []string{"a"}, parent: screen}
	list := &el{tag: "list", id: "l", parent: col, pseudos: []string{"focus"}}
	item := &el{tag: "item", classes: []string{"b"}, pseudos: []string{"selected"}, parent: list}
	text := &el{tag: "text", id: "t", classes: []string{"a", "b"}, pseudos: []string{"checked", "disabled"}, parent: item}
	return []*el{screen, col, list, item, text}
}

// FuzzSheetCascade parses any stylesheet for each document version, then
// runs the cascade over a fixed element chain at several sizes and both
// themes. Invariants: no panic; parsing and the cascade are deterministic
// (the same input gives equal sheets, diagnostics, tokens, and styles).
func FuzzSheetCascade(f *testing.F) {
	for _, pat := range []string{"../../examples/*/*.tcss", "../../examples/*/*/*.tcss", "../../kit/*.tcss", "../../testdata/*/*.tcss", "../../testdata/*/*/*.tcss"} {
		paths, _ := filepath.Glob(pat)
		for _, p := range paths {
			if b, err := os.ReadFile(p); err == nil && len(b) <= 8<<10 {
				f.Add(string(b))
			}
		}
	}
	for _, s := range []string{
		"",
		"" + string(rune(0xFEFF)) + ":root { --x: #fff }",
		"@media (theme: light) { :root { bg: red } } @media (max-cols: 40) { text { color: $x } }",
		"* { }", "a{b{c{d{e{", "}}}}", "text { color: ; }", "text { width: 99999999999999999999999fr; flex: 1e9 }",
		"text { padding: 1 2 3 4 5 } text:focus:selected:checked, list > item .a #t { reverse: true }",
		"/* unterminated", "text { color: \xff\xfe }", "@media", "@media (", "@media (x) {", "text { gap: 4; row-gap: 1; column-gap: 9 }",
		"col { layout: grid; grid-columns: 12; grid-min-width: 1 } text { wrap: truncate-middle; content-align: center }",
		strings.Repeat("text { color: red }\n", 300),
		"text { --a: $b; color: $a; background: var(--a); border-color: var(--zzz) }",
	} {
		f.Add(s)
	}
	chain := fuzzChain()
	f.Fuzz(func(t *testing.T, src string) {
		if len(src) > 16<<10 {
			t.Skip()
		}
		for _, v := range []struct{ v2, v3 bool }{{false, false}, {true, false}, {true, true}} {
			sh, diags := ParseSheetIn(src, "f.tcss", v.v2, v.v3)
			sh2, diags2 := ParseSheetIn(src, "f.tcss", v.v2, v.v3)
			if !reflect.DeepEqual(sh, sh2) || !reflect.DeepEqual(diags, diags2) {
				t.Fatalf("v2=%v v3=%v: two parses differ", v.v2, v.v3)
			}
			for _, d := range diags {
				if d.Line < 1 || d.Col < 1 {
					t.Fatalf("diagnostic without a position: %+v", d)
				}
			}
			_ = CheckTokens([]*Sheet{sh}, "dark")
			_ = KnownTokens([]*Sheet{sh}, "light")
			_ = IRTokens([]*Sheet{sh}, "auto")
			_ = NamesClass([]*Sheet{sh}, "a")
			for _, env := range []Env{{80, 24, "dark"}, {1, 1, "light"}, {200, 60, ""}} {
				c := NewCascade([]*Sheet{sh}, env)
				c2 := NewCascade([]*Sheet{sh}, env)
				if !reflect.DeepEqual(c.Tokens, c2.Tokens) {
					t.Fatalf("tokens differ between two cascades")
				}
				var parent *Style
				for _, e := range chain {
					st := c.Compute(e, parent, nil, nil)
					st2 := c2.Compute(e, parent, nil, nil)
					if !reflect.DeepEqual(st, st2) {
						t.Fatalf("styles of <%s> differ between two cascades", e.tag)
					}
					parent = &st
				}
			}
		}
	})
}

// FuzzSelectorAndValues checks the selector parser, the value checkers, and
// color resolution on any string: no panic, and a selector that parses
// matches without panicking against a fixed element.
func FuzzSelectorAndValues(f *testing.F) {
	for _, s := range []string{
		"", "text", "#id", ".c", "a b", "a > b", "a, b", ":root", "text:focus", "text:nope", "a >", "> a", "a > > b", "#", ".", ":",
		"a.b.c#d:focus:selected", "col > row .x", "*", "\x00", "é", "list > item:selected", "a,,b",
		"#fff", "#ffff", "#12345g", "red", "default", "$accent", "var(--x)", "var(--)", "var(--x", "$", "1fr", "50%", "auto", "10px",
		"true", "wrap", "1 2 3 4", "-1", "space-between", "none", "rounded", "99999999999999999999",
	} {
		f.Add(s)
	}
	leaf := fuzzChain()[4]
	tokens := map[string]string{"x": "#abc", "y": "$x", "z": "red"}
	f.Fuzz(func(t *testing.T, s string) {
		for _, v2 := range []bool{false, true} {
			if sels, err := ParseSelectorListIn(s, v2); err == nil {
				for _, sel := range sels {
					_ = sel.Matches(leaf)
					_ = sel.Matches(leaf.parent)
				}
			}
		}
		for _, name := range PropertyNames() {
			_ = CheckDeclIn(name, s, false, false)
			_ = CheckDeclIn(name, s, true, true)
		}
		_, _ = ParseLiteralColor(s)
		_, _ = ResolveColor(s, tokens)
		_, _ = TokenRef(s)
	})
}
