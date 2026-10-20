package main

import (
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/catalog"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// The generated document writes no version (v0.4.1), so it is the current
// language (SPEC v0.4b §5.1, §15.11): the catalog's current, which is no
// writable version, rather than the newest of versions. A build whose
// current language moves fails here until a <tui> without version reads
// as it.
func TestNewTemplateIsTheNewestVersion(t *testing.T) {
	b, err := templates.ReadFile("newtemplate/app.tui")
	if err != nil {
		t.Fatal(err)
	}
	doc := parse.Parse(b, "app.tui")
	if v, ok := doc.Root.Attr("version"); ok {
		t.Errorf("newtemplate/app.tui writes version=%q, want none", v)
	}
	if doc.Lang != ir.LangCurrent || int(doc.Lang) != catalog.Current {
		t.Errorf("a <tui> without version reads as language %d, the current language is %d (catalog %d)", doc.Lang, ir.LangCurrent, catalog.Current)
	}
	for _, v := range catalog.Versions {
		if v >= catalog.Current {
			t.Errorf("versions %v holds %d, not below the current language %d", catalog.Versions, v, catalog.Current)
		}
	}
}
