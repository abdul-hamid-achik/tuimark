package ir

import (
	"strings"
	"testing"
)

// FuzzBindingGrammar checks the small binding grammars (SPEC §7) on any
// string: no panic, and whatever a parser accepts reassembles to the input
// it was given, so no byte is silently dropped or invented.
func FuzzBindingGrammar(f *testing.F) {
	for _, s := range []string{
		"", "a", "a.b.c", "{a}", "x {a.b} y {c}", "{", "}", "{}", "{ a }", "{a", "a}", "{{a}}", "{a}{b}",
		"tickets as item", "tickets  as item", " tickets as item", "a as b as c", "a. as b", "1a as b",
		"!a", "! a", "!!a", "a.b", "_x9", "é", "{é}", "\x00", "a as \xff", strings.Repeat("{a}", 200),
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if segs, err := ParseInterp(s); err == nil {
			var b strings.Builder
			for _, sg := range segs {
				if sg.Path != "" {
					if !IsPath(sg.Path) {
						t.Fatalf("segment path %q is not a path", sg.Path)
					}
					b.WriteString("{" + sg.Path + "}")
				} else {
					b.WriteString(sg.Lit)
				}
			}
			if b.String() != s {
				t.Fatalf("ParseInterp(%q) does not reassemble: %q", s, b.String())
			}
		}
		_ = HasInterp(s)
		if e, err := ParseEach(s); err == nil {
			if e.Path+" as "+e.Alias != s || !IsPath(e.Path) || !IsIdent(e.Alias) {
				t.Fatalf("ParseEach(%q) = %+v", s, e)
			}
		}
		if g, err := ParseGuard(s); err == nil {
			want := g.Path
			if g.Neg {
				want = "!" + want
			}
			if want != s || !IsPath(g.Path) {
				t.Fatalf("ParseGuard(%q) = %+v", s, g)
			}
		}
		_ = IsGuardName(s)
		_ = IsFlag(s)
	})
}

// FuzzKeyTokens checks key-token validation and splitting: a valid token is
// never empty, never holds a comma or a space, and survives SplitKeys.
func FuzzKeyTokens(f *testing.F) {
	for _, s := range []string{
		"", "enter", "ctrl+c", "ctrl+", "ctrl+C", "ctrl+ab", "shift+tab", "q,ctrl+c", " q , ", ",", ",,", "space", " ", "é", "\x7f", "~", "ctrl+,",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if ValidKey(s) {
			if s == "" || strings.ContainsAny(s, ", \t\n") {
				t.Fatalf("ValidKey(%q) is true", s)
			}
			got := SplitKeys(s)
			if len(got) != 1 || got[0] != s {
				t.Fatalf("SplitKeys(%q) = %q", s, got)
			}
		}
		for _, k := range SplitKeys(s) {
			if k == "" || k != strings.TrimSpace(k) || strings.Contains(k, ",") {
				t.Fatalf("SplitKeys(%q) has token %q", s, k)
			}
		}
	})
}

// FuzzScalar checks the unit grammar (N, N%, Nfr, auto) and the decimal
// literal canonicalisation: no panic, and a canonical literal is a fixed
// point of itself.
func FuzzScalar(f *testing.F) {
	for _, s := range []string{
		"", "auto", "0", "1", "10%", "1fr", "0fr", "0.5fr", "1.5", "007", "-1", "+1", "1e3", "1.", ".5", "NaN", "inf",
		"1px", " 3 ", "99999999999999999999999999", "0.000000000000000000000000000000001fr", strings.Repeat("9", 1200),
		"1%%", "fr", "%",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if sc, err := ParseScalar(s); err == nil && sc.Kind != Auto {
			if sc.Lit == "" {
				t.Fatalf("ParseScalar(%q) has no literal: %+v", s, sc)
			}
			_ = sc.IsWhole()
		}
		lit, _, ok := ParseDecimalLit(s)
		if !ok {
			return
		}
		lit2, _, ok2 := ParseDecimalLit(lit)
		if !ok2 || lit2 != lit {
			t.Fatalf("ParseDecimalLit(%q) = %q, which is not canonical (%q, %v)", s, lit, lit2, ok2)
		}
		for _, n := range []int64{0, 1, 1 << 40} {
			_ = DecimalCmp(lit, n)
		}
	})
}
