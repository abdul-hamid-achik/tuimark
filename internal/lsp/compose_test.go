package lsp

import (
	"path/filepath"
	"strings"
	"testing"
)

// SPEC v0.4b §6.18.2, §15.12 (phase F1): an open file whose root is
// <fragment> is checked on its own, with no V005; a document that includes
// a fragment reads the editor's text of it. Routing a fragment's
// diagnostics through the documents that include it is F4's.
func TestAnalyzeChecksAFragmentOnItsOwn(t *testing.T) {
	dir := writeTree(t, nil)
	sn := &snapshot{open: map[string]string{}, byURI: map[string]*document{}}
	p := filepath.Join(dir, "orphan.tui")
	a := analyze(sn, p, "<fragment>\n  <zzz/>\n  <text bogus=\"1\">x</text>\n</fragment>\n")
	var got []string
	for _, d := range a.diags {
		got = append(got, d.Code+" "+d.Msg)
	}
	if len(got) != 2 || got[0] != "V001 unknown tag <zzz>" || !strings.HasPrefix(got[1], `V002 attribute "bogus"`) {
		t.Errorf("diagnostics: %v", got)
	}
	if a.version != 4 {
		t.Errorf("version %d, want 4 (a fragment is the current language)", a.version)
	}
}

// A document that includes a fragment: its diagnostics in the fragment
// carry the fragment's absolute path, so they are not mixed with the
// document's own.
func TestAnalyzeNamesFragmentsByAbsolutePath(t *testing.T) {
	dir := writeTree(t, map[string]string{"x.tui": "<fragment><text bogus=\"1\">x</text></fragment>"})
	sn := &snapshot{open: map[string]string{}, byURI: map[string]*document{}}
	app := filepath.Join(dir, "app.tui")
	a := analyze(sn, app, "<tui><screen><include src=\"x.tui\"/></screen></tui>")
	if len(a.diags) != 1 || a.diags[0].File != filepath.Join(dir, "x.tui") {
		t.Fatalf("diagnostics: %v", a.diags)
	}
	if got := a.route("", true); len(got) != 0 {
		t.Errorf("the document's own diagnostics: %v", got)
	}
}
