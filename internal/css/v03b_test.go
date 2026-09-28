package css

import "testing"

// SPEC v0.3 §10.2, §21 tests 91 and 93 (v0.3b): row-gap and column-gap.

// 91. row-gap and column-gap are V003 with the version="3" hint outside a
// version="3" document (v2 false regardless of v3, and v2 true, v3
// false); valid, and resolved on their own, inside one. wrap's two new
// values (truncate-start, truncate-middle) follow the same rule; an
// invalid wrap value lists them among the expected values only when v3.
func TestGapPropertiesVersionGate(t *testing.T) {
	for _, prop := range []string{"row-gap", "column-gap"} {
		if err := CheckDeclIn(prop, "2", false, false); err == nil {
			t.Errorf("%s: version=\"1\" accepted it", prop)
		}
		if err := CheckDeclIn(prop, "2", true, false); err == nil {
			t.Errorf("%s: version=\"2\" accepted it", prop)
		}
		if err := CheckDeclIn(prop, "2", true, true); err != nil {
			t.Errorf("%s: version=\"3\": %v", prop, err)
		}
		if err := CheckDeclIn(prop, "5", true, true); err == nil {
			t.Errorf("%s=5: accepted (want 0-4)", prop)
		}
	}
	for _, val := range []string{"truncate-start", "truncate-middle"} {
		if err := CheckDeclIn("wrap", val, true, false); err == nil {
			t.Errorf("wrap: %s accepted in version=\"2\"", val)
		} else if !hasVersionHint3(err) {
			t.Errorf("wrap: %s: %v (want the version=\"3\" hint)", val, err)
		}
		if err := CheckDeclIn("wrap", val, true, true); err != nil {
			t.Errorf("wrap: %s in version=\"3\": %v", val, err)
		}
	}
	// An invalid wrap value lists truncate-start/truncate-middle among the
	// expected values only for a version="3" document.
	err2 := CheckDeclIn("wrap", "bogus", true, false).Error()
	if contains(err2, "truncate-start") {
		t.Errorf("version=\"2\" wrap error lists truncate-start: %s", err2)
	}
	err3 := CheckDeclIn("wrap", "bogus", true, true).Error()
	if !contains(err3, "truncate-start") || !contains(err3, "truncate-middle") {
		t.Errorf("version=\"3\" wrap error does not list the new values: %s", err3)
	}
}

func hasVersionHint3(err error) bool {
	return err != nil && contains(err.Error(), `(requires version="3")`)
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// 93. The cascade shorthand: a gap declaration counts as row-gap and
// column-gap too, each with gap's origin, specificity, and order; a
// row-gap/column-gap declaration is then resolved against it like any
// other property (higher specificity wins, a later declaration at equal
// specificity wins). A version="1"/"2" document never sees this (the
// properties cannot appear there), so Style.Gap alone still drives it.
func TestGapCascadeShorthand(t *testing.T) {
	sh, diags := ParseSheetIn(`.x { row-gap: 0; } #g { gap: 2; }`, "a.tcss", true, true)
	if len(diags) > 0 {
		t.Fatal(diags)
	}
	c := NewCascade([]*Sheet{sh}, Env{Cols: 80, Rows: 24})
	st := c.Compute(&el{tag: "box", id: "g", classes: []string{"x"}}, nil, nil, nil)
	// #g (specificity 100) beats .x (specificity 10) for row-gap, even
	// though .x sets row-gap directly and #g only implies it through gap.
	if st.Gap != 2 || st.RowGap != 2 || st.ColumnGap != 2 {
		t.Errorf("gap %d row-gap %d column-gap %d, want 2 2 2", st.Gap, st.RowGap, st.ColumnGap)
	}
	// A later, equally specific row-gap declaration on the same element
	// beats an earlier gap.
	sh2, diags2 := ParseSheetIn(`#g { gap: 2; row-gap: 0; }`, "b.tcss", true, true)
	if len(diags2) > 0 {
		t.Fatal(diags2)
	}
	c2 := NewCascade([]*Sheet{sh2}, Env{Cols: 80, Rows: 24})
	st2 := c2.Compute(&el{tag: "box", id: "g"}, nil, nil, nil)
	if st2.Gap != 2 || st2.RowGap != 0 || st2.ColumnGap != 2 {
		t.Errorf("gap %d row-gap %d column-gap %d, want 2 0 2", st2.Gap, st2.RowGap, st2.ColumnGap)
	}
}

// An out-of-range row-gap/column-gap names the property the document
// wrote, not gap (the V003 message is the author's only pointer to the
// declaration).
func TestGapPropertiesBadValueNamesProperty(t *testing.T) {
	for _, prop := range []string{"gap", "row-gap", "column-gap"} {
		err := CheckDeclIn(prop, "7", true, true)
		want := prop + `: bad value "7" (want one integer 0-4)`
		if err == nil || err.Error() != want {
			t.Errorf("%s: 7: %v, want %s", prop, err, want)
		}
	}
}
