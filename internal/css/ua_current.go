package css

import (
	"fmt"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// UASheetCurrent is the built-in stylesheet of the current language, a
// <tui> without version (SPEC v0.4b §10.6): UASheet with its col and row
// rules replaced by three rules, so an unsized col or row is 1fr only as
// a direct child of screen, modal, or tab (§11.2), and the focus look
// added at its end (ADR 0031), so a focused button is reversed with no
// author rule. A document that writes version, and the spike root <app>,
// keep UASheet unchanged (Lang1-Lang3).
//
// The focus rule names only button: a type selector this sheet's
// language does not know would make init panic, so the form-control
// phases add checkbox and select to it with their tags (plan §2).
//
// The rest of the text is UASheet's, rule for rule and in order
// (TestUASheetCurrentIsUASheetWithTheCurrentRules), so tuimark inspect
// reports the same rules for both apart from these.
const UASheetCurrent = `
screen { layout: column; width: 100%; height: 100%; }
col    { layout: column; }
row    { layout: row; }
screen > col, screen > row, modal > col, modal > row, tab > col, tab > row { width: 1fr; height: 1fr; }
box    { layout: column; }
text   { width: auto; height: auto; }
scroll { layout: column; }
list   { layout: column; }
item   { layout: column; }
modal  { layout: column; border: single; }
spacer { flex: 1; }
input, button, progress, rule { width: auto; height: auto; }
list > item:selected { reverse: true; }
table  { layout: column; wrap: truncate; }
table > item:selected { reverse: true; }
tabs   { layout: column; }
tab    { layout: column; }
sparkline { width: auto; height: auto; }
hints  { layout: row; gap: 2; }
hints > row { gap: 1; }
button:focus { reverse: true; }
`

var uaSheetCurrent *Sheet

func init() {
	s, diags := ParseSheetLang(UASheetCurrent, UAFile, ir.LangCurrent)
	if len(diags) > 0 {
		panic(fmt.Sprintf("built-in stylesheet of the current language: %v", diags))
	}
	uaSheetCurrent = s
}

// UALang returns the parsed built-in sheet of a document of language l
// (read-only): UASheetCurrent's for the current language, UASheet's for
// a pinned document and the spike (Lang 0 is read as Lang1).
func UALang(l ir.Lang) *Sheet {
	if l.V4() {
		return uaSheetCurrent
	}
	return uaSheet
}
