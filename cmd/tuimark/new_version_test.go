package main

import (
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/catalog"
	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// The generated document writes no version (v0.4.1), so it is the current
// language: the newest version this build accepts. A build that learns a
// newer version fails here until a <tui> without version reads as it.
func TestNewTemplateIsTheNewestVersion(t *testing.T) {
	newest := catalog.Versions[len(catalog.Versions)-1]
	b, err := templates.ReadFile("newtemplate/app.tui")
	if err != nil {
		t.Fatal(err)
	}
	doc := parse.Parse(b, "app.tui")
	if v, ok := doc.Root.Attr("version"); ok {
		t.Errorf("newtemplate/app.tui writes version=%q, want none", v)
	}
	if newest != 3 || !doc.V3 {
		t.Errorf("a <tui> without version reads as V3=%v, the newest version is %d", doc.V3, newest)
	}
}
