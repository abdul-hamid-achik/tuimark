package ir

import (
	"math/big"
	"strings"
	"testing"
)

func TestParseScalar(t *testing.T) {
	ok := []struct {
		in   string
		kind ScalarKind
		n    float64
	}{
		{"0", Cell, 0},
		{"12", Cell, 12},
		{" 7 ", Cell, 7},
		{"30%", Pct, 30},
		{"0%", Pct, 0},
		{"100%", Pct, 100},
		{"150%", Pct, 150},
		{"12.5%", Pct, 12.5},
		{"1fr", Fr, 1},
		{"2fr", Fr, 2},
		{"1.5fr", Fr, 1.5},
		{"auto", Auto, 0},
	}
	for _, c := range ok {
		t.Run(c.in, func(t *testing.T) {
			s, err := ParseScalar(c.in)
			if err != nil {
				t.Fatalf("ParseScalar(%q): %v", c.in, err)
			}
			if s.Kind != c.kind || s.N != c.n {
				t.Fatalf("ParseScalar(%q) = %+v, want kind %d n %v", c.in, s, c.kind, c.n)
			}
		})
	}
	// SPEC §9: px, em, ch, vw, vh, w, h, min-content are forbidden; sizes are
	// non-negative and a cell count is an integer.
	bad := []string{
		"", " ", "px", "10px", "2em", "3ch", "50vw", "50vh", "5w", "5h", "min-content",
		"-1", "-5%", "-1fr", "+3", "1.5", "0.5", ".5", "1e2", "1E2", "1e2%", "0fr", "0.0fr",
		"%", "fr", "AUTO", "Auto", "1 fr", "10 %", "1,5", "0x10", "inf", "Inf%", "nan",
		"NaNfr", "infinityfr", "1_0", "fr1", "auto%",
	}
	for _, in := range bad {
		t.Run("bad/"+in, func(t *testing.T) {
			if s, err := ParseScalar(in); err == nil {
				t.Fatalf("ParseScalar(%q) = %+v, want an error", in, s)
			}
		})
	}
}

func TestScalarString(t *testing.T) {
	for _, c := range []struct {
		s    Scalar
		want string
	}{
		{Scalar{Kind: Cell, N: 12}, "12"},
		{Scalar{Kind: Pct, N: 30}, "30%"},
		{Scalar{Kind: Pct, N: 12.5}, "12.5%"},
		{Scalar{Kind: Fr, N: 1}, "1fr"},
		{Scalar{Kind: Fr, N: 1.5}, "1.5fr"},
		{Scalar{Kind: Auto}, "auto"},
		{Scalar{}, ""},
	} {
		if got := c.s.String(); got != c.want {
			t.Errorf("%+v.String() = %q, want %q", c.s, got, c.want)
		}
	}
	// String round-trips through ParseScalar.
	for _, in := range []string{"3", "40%", "2fr", "auto"} {
		s, _ := ParseScalar(in)
		if s.String() != in {
			t.Errorf("round trip %q -> %q", in, s.String())
		}
	}
}

func TestParseDecimal(t *testing.T) {
	for _, c := range []struct {
		in   string
		want float64
		ok   bool
	}{
		{"0", 0, true},
		{"42", 42, true},
		{"1.25", 1.25, true},
		{"100", 100, true},
		{"", 0, false},
		{".", 0, false},
		{".5", 0, false},
		{"5.", 0, false},
		{"1.2.3", 0, false},
		{"-1", 0, false},
		{"+1", 0, false},
		{"1e3", 0, false},
		{"inf", 0, false},
		{"NaN", 0, false},
		{"0x1p4", 0, false},
		{" 1", 0, false},
	} {
		got, ok := ParseDecimal(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("ParseDecimal(%q) = %v, %v; want %v, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestIsIdentAndPath(t *testing.T) {
	for _, c := range []struct {
		in           string
		ident, apath bool
	}{
		{"a", true, true},
		{"_x", true, true},
		{"Folder", true, true},
		{"item2", true, true},
		{"selected_ticket", true, true},
		{"2item", false, false},
		{"", false, false},
		{"a-b", false, false},
		{"a b", false, false},
		{"a.b", false, true},
		{"selected_ticket.title", false, true},
		{"a.b.c", false, true},
		{"a.", false, false},
		{".a", false, false},
		{"a..b", false, false},
		{"tickets.0.title", false, false}, // markup has no indexes (SPEC §7)
		{"a.2b", false, false},
		{"a[0]", false, false},
		{"f(x)", false, false},
		{"a|upper", false, false},
		{"ñ", false, false},
	} {
		if got := IsIdent(c.in); got != c.ident {
			t.Errorf("IsIdent(%q) = %v, want %v", c.in, got, c.ident)
		}
		if got := IsPath(c.in); got != c.apath {
			t.Errorf("IsPath(%q) = %v, want %v", c.in, got, c.apath)
		}
	}
}

func TestParseEach(t *testing.T) {
	good := []struct {
		in          string
		path, alias string
	}{
		{"tickets as item", "tickets", "item"},
		{"inbox.tickets as t", "inbox.tickets", "t"},
	}
	for _, c := range good {
		e, err := ParseEach(c.in)
		if err != nil {
			t.Errorf("ParseEach(%q): %v", c.in, err)
			continue
		}
		if e.Path != c.path || e.Alias != c.alias {
			t.Errorf("ParseEach(%q) = %+v", c.in, e)
		}
	}
	for _, in := range []string{
		"", "tickets", "as item", "tickets as", "tickets as item as x", "tickets in item",
		"tickets as item.id", "tickets.0 as item", "tickets as 1x", "tickets[0] as item", "a-b as c",
		// SPEC §7 spells the whitespace out: each := path " as " ident
		// (round-2 finding 20).
		"  rows   as   r  ", "tickets  as item", "tickets as  item", " tickets as item",
		"tickets as item ", "tickets\tas item", "tickets as\titem",
	} {
		if e, err := ParseEach(in); err == nil {
			t.Errorf("ParseEach(%q) = %+v, want an error", in, e)
		}
	}
}

func TestParseGuard(t *testing.T) {
	for _, c := range []struct {
		in   string
		path string
		neg  bool
	}{
		{"selected_ticket", "selected_ticket", false},
		{"!selected_ticket", "selected_ticket", true},
		{"a.b", "a.b", false},
	} {
		g, err := ParseGuard(c.in)
		if err != nil {
			t.Errorf("ParseGuard(%q): %v", c.in, err)
			continue
		}
		if g.Path != c.path || g.Neg != c.neg {
			t.Errorf("ParseGuard(%q) = %+v", c.in, g)
		}
	}
	// No whitespace anywhere in if := path | "!" path (round-2 finding 20).
	for _, in := range []string{"", "!", "!!a", "a == b", "a && b", "a || b", "count > 0", "not a", "a.", "f(x)", "! a.b", " x ", "x ", "! x", " !x", "!x "} {
		if g, err := ParseGuard(in); err == nil {
			t.Errorf("ParseGuard(%q) = %+v, want an error", in, g)
		}
	}
}

func TestParseInterp(t *testing.T) {
	good := []struct {
		in   string
		want []Segment
	}{
		{"", nil},
		{"plain", []Segment{{Lit: "plain"}}},
		{"{folder}", []Segment{{Path: "folder"}}},
		{"mail  {folder}", []Segment{{Lit: "mail  "}, {Path: "folder"}}},
		{"{count} tickets", []Segment{{Path: "count"}, {Lit: " tickets"}}},
		{"{a}{b}", []Segment{{Path: "a"}, {Path: "b"}}},
		{"x {selected_ticket.title} y", []Segment{{Lit: "x "}, {Path: "selected_ticket.title"}, {Lit: " y"}}},
		// An unclosed brace is literal text.
		{"{unclosed", []Segment{{Lit: "{unclosed"}}},
		{"a { b", []Segment{{Lit: "a { b"}}},
		{"closing } only", []Segment{{Lit: "closing } only"}}},
		{"{a} {", []Segment{{Path: "a"}, {Lit: " {"}}},
	}
	for _, c := range good {
		got, err := ParseInterp(c.in)
		if err != nil {
			t.Errorf("ParseInterp(%q): %v", c.in, err)
			continue
		}
		if len(got) != len(c.want) {
			t.Errorf("ParseInterp(%q) = %+v, want %+v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("ParseInterp(%q)[%d] = %+v, want %+v", c.in, i, got[i], c.want[i])
			}
		}
	}
	// No expressions, filters, calls, indexes, or JS/Py blocks (SPEC §1, §7).
	for _, in := range []string{
		"{}", "{ }", "{a + b}", "{count > 0 ? 'x' : 'y'}", "{name|upper}", "{len(tickets)}",
		"{tickets.0.title}", "{tickets[0]}", "{js}x{/js}x", "{!a}", "{a.}", "{{a}}",
		// interp := "{" path "}", with no spaces (round-2 finding 20).
		"{ item.id }", "{ folder }", "{folder }", "{ folder}", "x {\tfolder} y",
	} {
		if segs, err := ParseInterp(in); err == nil {
			t.Errorf("ParseInterp(%q) = %+v, want an error", in, segs)
		}
	}
}

func TestHasInterp(t *testing.T) {
	for _, c := range []struct {
		in   string
		want bool
	}{
		{"", false},
		{"plain", false},
		{"{a", false},
		{"{a}", true},
		{"x {a.b} y", true},
		{"{1+1}", true}, // malformed interpolation still counts as "uses {…}"
	} {
		if got := HasInterp(c.in); got != c.want {
			t.Errorf("HasInterp(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// SPEC §7 truthiness table.
func TestTruthy(t *testing.T) {
	for _, c := range []struct {
		name string
		v    any
		want bool
	}{
		{"null", nil, false},
		{"false", false, false},
		{"true", true, true},
		{"0", float64(0), false},
		{"-0", -float64(0), false},
		{"1", float64(1), true},
		{"-1", float64(-1), true},
		{"0.5", 0.5, true},
		{"int 0", 0, false},
		{"int 3", 3, true},
		{`""`, "", false},
		{`"0"`, "0", true},
		{`"false"`, "false", true},
		{`" "`, " ", true},
		{"[]", []any{}, false},
		{"[null]", []any{nil}, true},
		{"{}", map[string]any{}, true},
		{"{a:1}", map[string]any{"a": 1.0}, true},
	} {
		if got := Truthy(c.v); got != c.want {
			t.Errorf("Truthy(%s) = %v, want %v", c.name, got, c.want)
		}
	}
}

// SPEC §8.1 key tokens.
func TestValidKey(t *testing.T) {
	valid := []string{
		"a", "z", "q", "0", "9", "/", "?", "enter", "esc", "tab", "backspace", "space",
		"up", "down", "left", "right", "home", "end", "pgup", "pgdn", "ctrl+c", "ctrl+a",
		"ctrl+z", "shift+tab",
	}
	for _, k := range valid {
		if !ValidKey(k) {
			t.Errorf("ValidKey(%q) = false, want true", k)
		}
	}
	invalid := []string{
		"", " ", ",", "ctrl+", "ctrl+C", "ctrl+1", "ctrl+enter", "alt+x", "shift+a", "f1",
		"return", "escape", "pageup", "ab", "ñ", "\t", "\x7f",
	}
	for _, k := range invalid {
		if ValidKey(k) {
			t.Errorf("ValidKey(%q) = true, want false", k)
		}
	}
}

func TestSplitKeys(t *testing.T) {
	for _, c := range []struct {
		in   string
		want []string
	}{
		{"q", []string{"q"}},
		{"q,ctrl+c", []string{"q", "ctrl+c"}},
		{" q , ctrl+c ,", []string{"q", "ctrl+c"}},
		{"", nil},
		{",,", nil},
	} {
		got := SplitKeys(c.in)
		if strings.Join(got, "|") != strings.Join(c.want, "|") || len(got) != len(c.want) {
			t.Errorf("SplitKeys(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIsKind(t *testing.T) {
	if len(Kinds) != 18 {
		t.Fatalf("catalog has %d kinds; SPEC §13.1 lists 18 (17 tags + the <tui> root)", len(Kinds))
	}
	for _, k := range Kinds {
		if !IsKind(k) {
			t.Errorf("IsKind(%q) = false", k)
		}
	}
	for _, k := range []string{"app", "widget", "table", "tabs", "grid", "markdown", "textarea", "Box", ""} {
		if IsKind(k) {
			t.Errorf("IsKind(%q) = true", k)
		}
	}
}

func TestNodeAttr(t *testing.T) {
	var nilNode *Node
	if _, ok := nilNode.Attr("id"); ok {
		t.Error("nil node has no attributes")
	}
	n := &Node{Attrs: map[string]string{"id": "x", "empty": ""}}
	if v, ok := n.Attr("id"); !ok || v != "x" {
		t.Errorf("Attr(id) = %q, %v", v, ok)
	}
	if v, ok := n.Attr("empty"); !ok || v != "" {
		t.Errorf("Attr(empty) = %q, %v", v, ok)
	}
	if _, ok := n.Attr("missing"); ok {
		t.Error("Attr(missing) reported present")
	}
}

func TestDiagnostics(t *testing.T) {
	d := Diagnostic{Severity: Error, Code: "V001", Msg: "unknown tag <foo>", File: "app.tui", Line: 12, Col: 3, Path: "/screen/col/foo"}
	if got, want := d.String(), "error V001 app.tui:12:3 /screen/col/foo unknown tag <foo>"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
	noFile := Diagnostic{Severity: Warning, Code: "B006", Msg: "m", Line: 2, Col: 1}
	if got, want := noFile.String(), "warning B006 <input>:2:1 m"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
	bare := Diagnostic{Severity: Error, Code: "V003", Msg: "required theme token $bg is missing"}
	if got, want := bare.String(), "error V003 required theme token $bg is missing"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}

	ds := Diags{
		{Severity: Warning, Code: "B006", File: "b.tui", Line: 1, Col: 1},
		{Severity: Warning, Code: "L005", File: "a.tui", Line: 9, Col: 1},
		{Severity: Warning, Code: "L003", File: "a.tui", Line: 9, Col: 1},
		{Severity: Warning, Code: "V013", File: "a.tui", Line: 2, Col: 5},
		{Severity: Warning, Code: "V012", File: "a.tui", Line: 2, Col: 1},
	}
	if ds.HasErrors() {
		t.Error("warnings only: HasErrors() = true")
	}
	sorted := ds.Sorted()
	var got []string
	for _, d := range sorted {
		got = append(got, d.Code)
	}
	if strings.Join(got, " ") != "V012 V013 L003 L005 B006" {
		t.Errorf("Sorted() order = %v", got)
	}
	if ds[0].Code != "B006" {
		t.Error("Sorted() must not reorder the receiver")
	}
	ds = append(ds, Diagnostic{Severity: Error, Code: "V001"})
	if !ds.HasErrors() {
		t.Error("HasErrors() = false with an error")
	}

	n := &Node{ID: "inbox", Line: 4, Col: 7, Path: "/screen/list"}
	at := At(n, "app.tui", Warning, "B006", "list %s has no key", "inbox")
	if at.Line != 4 || at.Col != 7 || at.Path != "/screen/list" || at.ID != "inbox" || at.File != "app.tui" || at.Msg != "list inbox has no key" {
		t.Errorf("At() = %+v", at)
	}
	if at := At(nil, "", Error, "V003", "x"); at.Line != 0 || at.ID != "" {
		t.Errorf("At(nil) = %+v", at)
	}
}

// Round-2 findings 8 and 9: a size keeps its canonical decimal literal (its
// exact value), and whether a cell count is whole or an fr weight is zero
// is decided on that literal, not on the float64, whatever the platform.
func TestParseScalarLiteral(t *testing.T) {
	tiny := "0." + strings.Repeat("0", 400) + "1"
	for _, c := range []struct {
		in, lit, str string
		kind         ScalarKind
	}{
		{"12", "12", "12", Cell},
		{"007", "7", "7", Cell},
		{"2.0", "2", "2", Cell},
		{"12.50%", "12.5", "12.5%", Pct},
		{"0.10fr", "0.1", "0.1fr", Fr},
		// Whole numbers beyond the int64 range are whole numbers of cells
		// (layout clamps every size to 2^40 cells).
		{"99999999999999999999", "99999999999999999999", "99999999999999999999", Cell},
		{"9223372036854775807", "9223372036854775807", "9223372036854775807", Cell},
		{"9223372036854775808", "9223372036854775808", "9223372036854775808", Cell},
		{"99999999999999999999999999999999%", "99999999999999999999999999999999", "99999999999999999999999999999999%", Pct},
		{"1" + strings.Repeat("0", 400), "1" + strings.Repeat("0", 400), "1" + strings.Repeat("0", 400), Cell},
		// Precision beyond a float64 is kept.
		{"33.333333333333333333%", "33.333333333333333333", "33.333333333333333333%", Pct},
		{"2.0000000000000001fr", "2.0000000000000001", "2.0000000000000001fr", Fr},
		// A positive weight too small for a float64 is still positive.
		{tiny + "fr", tiny, tiny + "fr", Fr},
	} {
		s, err := ParseScalar(c.in)
		if err != nil {
			t.Errorf("ParseScalar(%.60q): %v", c.in, err)
			continue
		}
		if s.Kind != c.kind || s.Lit != c.lit || s.String() != c.str {
			t.Errorf("ParseScalar(%.60q) = kind %d lit %.60q string %.60q", c.in, s.Kind, s.Lit, s.String())
		}
		if !(s.N > 0) && c.lit != "0" {
			t.Errorf("ParseScalar(%.60q).N = %v, want > 0 like the literal", c.in, s.N)
		}
		if want, _ := new(big.Rat).SetString(c.lit); s.Rat().Cmp(want) != 0 {
			t.Errorf("ParseScalar(%.60q).Rat() = %v, want %v", c.in, s.Rat(), want)
		}
	}
	for _, c := range []struct{ in, msg string }{
		// A fraction float64 rounding would hide is still a fraction.
		{"1.00000000000000000001", "cells are integers"},
		{"9007199254740993.5", "cells are integers"},
		{"0fr", "fr weight must be > 0"},
		{"000.000fr", "fr weight must be > 0"},
		{strings.Repeat("9", MaxDecimalLen+1), "at most"},
		{"1." + strings.Repeat("5", MaxDecimalLen) + "%", "at most"},
	} {
		if s, err := ParseScalar(c.in); err == nil || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("ParseScalar(%.60q) = %+v, %v; want an error containing %q", c.in, s, err, c.msg)
		}
	}
	// A Scalar built in code has no literal and is exactly N.
	if s := (Scalar{Kind: Cell, N: 1e20}); s.String() != "100000000000000000000" {
		t.Errorf("Scalar{Cell, 1e20}.String() = %q", s.String())
	}
	if r := (Scalar{Kind: Fr, N: 0.1}).Rat(); r.Cmp(big.NewRat(1, 10)) != 0 {
		t.Errorf("Scalar{Fr, 0.1}.Rat() = %v, want 1/10", r)
	}
}

func TestDecimalCmp(t *testing.T) {
	for _, c := range []struct {
		lit  string
		n    int64
		want int
	}{
		{"100", 100, 0},
		{"99.99999999999999999999", 100, -1},
		{"100.00000000000000001", 100, 1},
		{"0", 0, 0},
	} {
		if got := DecimalCmp(c.lit, c.n); got != c.want {
			t.Errorf("DecimalCmp(%q, %d) = %d, want %d", c.lit, c.n, got, c.want)
		}
	}
}
