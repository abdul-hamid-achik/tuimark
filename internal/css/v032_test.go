package css

import (
	"strings"
	"testing"
)

// v0.3.2: a selector inside an @media block that runs into the block's own
// '}' (`@media (...) { row }`) reports the missing '{' and lets that '}'
// close the block, so the rules after it stay unconditional and no
// "unterminated @media block" follows.
func TestMediaBlockClosedByStraySelectorBrace(t *testing.T) {
	src := "@media (max-cols: 80) { row }\nbox { color: red; }\n"
	sh, diags := ParseSheet(src, "t.tcss")
	if len(diags) != 1 || !strings.Contains(diags[0].Msg, `expected '{' after selector "row"`) {
		t.Fatalf("want only the expected-'{' V003, got %v", diags)
	}
	if len(sh.Rules) != 1 || sh.Rules[0].Selectors[0].Text != "box" {
		t.Fatalf("rules = %+v", sh.Rules)
	}
	if sh.Rules[0].Media != nil {
		t.Errorf("box rule must be unconditional, got media %v", sh.Rules[0].Media)
	}
	// Cascade level: the rule applies at 120 columns too.
	c := NewCascade([]*Sheet{sh}, Env{Cols: 120, Rows: 24})
	if st := c.Compute(&el{tag: "box"}, nil, nil, nil); st.Color == Initial().Color {
		t.Errorf("box color not applied at 120 cols: %+v", st.Color)
	}
	// A ';'-terminated junk selector inside the block keeps the block open.
	sh, diags = ParseSheet("@media (max-cols: 80) { row; col { dim: true; } }\nbox { bold: true; }", "t.tcss")
	if len(diags) != 1 || len(sh.Rules) != 2 || sh.Rules[0].Media == nil || sh.Rules[1].Media != nil {
		t.Errorf("semicolon recovery: diags=%v rules=%+v", diags, sh.Rules)
	}
}

// v0.3.2: an id or class name outside [A-Za-z_][A-Za-z0-9_-]* (SPEC §7,
// the same grammar markup ids and classes follow) can never match, so a
// selector naming one is V003-worthy instead of validating silently.
func TestSelectorRejectsImpossibleIDAndClass(t *testing.T) {
	for _, src := range []string{"#1", "#1a", "#-x", ".9x", ".-x", "box.1x", "box#9", "row > .9", "box:focus.-a"} {
		_, err := ParseSelector(src)
		if err == nil {
			t.Errorf("ParseSelector(%q) accepted", src)
			continue
		}
		if !strings.Contains(err.Error(), "must start with a letter or '_'") {
			t.Errorf("ParseSelector(%q): %v", src, err)
		}
		// The same parser runs in a stylesheet rule.
		if _, diags := ParseSheet(src+" { bold: true; }", "t.tcss"); len(diags) != 1 || diags[0].Code != "V003" {
			t.Errorf("sheet %q: diags %v", src, diags)
		}
	}
	for _, src := range []string{"#_a", "#a-1", "#A9", ".x", ".a-b_c9", "box.b2", "#a1.b2:focus"} {
		if _, err := ParseSelector(src); err != nil {
			t.Errorf("ParseSelector(%q): %v", src, err)
		}
	}
}

// v0.3.2: a leading '+' is not a plain integer cell value: gap, row-gap,
// column-gap, padding, margin, and an @media number reject it, while every
// value accepted before (including "-0") stays accepted.
func TestLeadingPlusIsRejectedInCellValues(t *testing.T) {
	for _, d := range [][2]string{
		{"gap", "+3"}, {"gap", "+0"}, {"padding", "+1"}, {"padding", "1 +2"},
		{"margin", "+1 2"}, {"row-gap", "+1"}, {"column-gap", "+1"},
	} {
		if err := CheckDeclIn(d[0], d[1], true, true); err == nil {
			t.Errorf("CheckDeclIn(%s: %s) accepted", d[0], d[1])
		}
	}
	for _, d := range [][2]string{
		{"gap", "3"}, {"gap", "0"}, {"gap", "-0"}, {"padding", "1 2"}, {"margin", "0 1 2 3"}, {"row-gap", "2"},
	} {
		if err := CheckDeclIn(d[0], d[1], true, true); err != nil {
			t.Errorf("CheckDeclIn(%s: %s): %v", d[0], d[1], err)
		}
	}
	if _, diags := ParseSheet("@media (max-cols: +80) { box { bold: true; } }", "t.tcss"); len(diags) != 1 || diags[0].Code != "V003" {
		t.Errorf("@media +80: diags %v", diags)
	}
}

// v0.3.2: an empty pseudo-class name has its own message.
func TestEmptyPseudoClass(t *testing.T) {
	for _, src := range []string{"box:", "box: > row", "box:.x"} {
		_, err := ParseSelector(src)
		if err == nil || !strings.Contains(err.Error(), "empty pseudo-class after ':'") {
			t.Errorf("ParseSelector(%q) = %v", src, err)
		}
	}
}

// v0.3.2: an unknown property gets a "did you mean" for a close name in the
// document's version only.
func TestUnknownPropertySuggestion(t *testing.T) {
	for _, c := range []struct {
		prop       string
		v2, v3     bool
		want       string
		wantAbsent bool
	}{
		{"colr", false, false, `; did you mean "color"?`, false},
		{"paddin", false, false, `; did you mean "padding"?`, false},
		{"backgrund", false, false, `; did you mean "background"?`, false},
		{"zzzzzz", false, false, "did you mean", true},
		// A close name that needs a newer version says so.
		{"grid-colums", false, false, `; did you mean "grid-columns" (requires version="2")?`, false},
		{"grid-colums", true, false, `; did you mean "grid-columns"?`, false},
		{"row-gp", true, false, `; did you mean "row-gap" (requires version="3")?`, false},
		{"row-gp", true, true, `; did you mean "row-gap"?`, false},
	} {
		err := CheckDeclIn(c.prop, "1", c.v2, c.v3)
		if err == nil || !strings.Contains(err.Error(), "unknown property") {
			t.Fatalf("%s: %v", c.prop, err)
		}
		if has := strings.Contains(err.Error(), c.want); has == c.wantAbsent {
			t.Errorf("%s (v2=%v v3=%v): %v", c.prop, c.v2, c.v3, err)
		}
	}
}
