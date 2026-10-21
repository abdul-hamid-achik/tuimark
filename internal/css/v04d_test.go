package css

import (
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// SPEC 0.4d test 274 (the wrap part) and decision 202: the value pre-wrap
// is the current language's; a pinned document gets the section 5.1
// message with the list of values that document accepts.
func TestPreWrapIsCurrentLanguageOnly(t *testing.T) {
	if err := CheckDeclLang("wrap", "pre-wrap", ir.Lang4); err != nil {
		t.Fatalf("current language rejects pre-wrap: %v", err)
	}
	for _, c := range []struct {
		l    ir.Lang
		want string
	}{
		{ir.Lang1, `wrap: value "pre-wrap" (want wrap | nowrap | truncate) (requires a <tui> without version)`},
		{ir.Lang2, `wrap: value "pre-wrap" (want wrap | nowrap | truncate) (requires a <tui> without version)`},
		{ir.Lang3, `wrap: value "pre-wrap" (want wrap | nowrap | truncate | truncate-start | truncate-middle) (requires a <tui> without version)`},
	} {
		err := CheckDeclLang("wrap", "pre-wrap", c.l)
		if err == nil || err.Error() != c.want {
			t.Errorf("Lang(%d): %v, want %s", c.l, err, c.want)
		}
	}
	// The bool functions are pinned documents.
	if err := CheckDeclIn("wrap", "pre-wrap", true, true); err == nil || !strings.Contains(err.Error(), "requires a <tui> without version") {
		t.Errorf("CheckDeclIn(true, true): %v", err)
	}
	// A bad value keeps the list of tuimark 0.4.5 in the current language
	// (test 275: every output for a document, an invalid one included,
	// stays byte-identical), pre-wrap left out.
	if err := CheckDeclLang("wrap", "bogus", ir.Lang4); err == nil || err.Error() != `wrap: bad value "bogus" (want wrap | nowrap | truncate | truncate-start | truncate-middle)` {
		t.Errorf("current language, bad wrap: %v", err)
	}
	if err := CheckDeclLang("wrap", "bogus", ir.Lang3); err == nil || strings.Contains(err.Error(), "pre-wrap") {
		t.Errorf("version 3, bad wrap: %v", err)
	}
}

func TestPreWrapInSheetsAndInlineStyles(t *testing.T) {
	src := "text { wrap: pre-wrap; }"
	if _, diags := ParseSheetLang(src, "t.tcss", ir.Lang4); len(diags) != 0 {
		t.Errorf("current language sheet: %v", diags)
	}
	if _, diags := ParseSheetLang(src, "t.tcss", ir.Lang3); len(diags) != 1 || !strings.Contains(diags[0].Msg, `value "pre-wrap"`) || !strings.HasSuffix(diags[0].Msg, "(requires a <tui> without version)") {
		t.Errorf("version 3 sheet: %v", diags)
	}
	if ds, errs := ParseDeclsAllLang("wrap: pre-wrap; bold: true", ir.Lang4); len(errs) != 0 || len(ds) != 2 {
		t.Errorf("current language style=\"\": %v %v", ds, errs)
	}
	if ds, errs := ParseDeclsAllLang("wrap: pre-wrap; bold: true", ir.Lang3); len(errs) != 1 || len(ds) != 1 {
		t.Errorf("version 3 style=\"\": %v %v", ds, errs)
	}
}

func TestPreWrapKeywordSince4(t *testing.T) {
	var found bool
	for _, k := range PropertyKeywords("wrap") {
		if k.Name == "pre-wrap" {
			found = true
			if k.Since != 4 {
				t.Errorf("pre-wrap since = %d, want 4", k.Since)
			}
		}
	}
	if !found {
		t.Error("pre-wrap is not a keyword of wrap")
	}
	if got := PropertyValues("wrap"); !strings.Contains(got, "pre-wrap") {
		t.Errorf("PropertyValues(wrap) = %q", got)
	}
}
