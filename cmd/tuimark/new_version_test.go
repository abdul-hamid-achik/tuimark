package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/catalog"
)

// The generated document is of the newest version this build accepts
// (SPEC v0.4a §15.11: "version=\"3\" in 0.4a; 0.4b makes it \"4\""): a build
// that learns version 4 fails here until the template follows.
func TestNewTemplateIsTheNewestVersion(t *testing.T) {
	newest := catalog.Versions[len(catalog.Versions)-1]
	b, err := templates.ReadFile("newtemplate/app.tui")
	if err != nil {
		t.Fatal(err)
	}
	if want := fmt.Sprintf(`<tui version="%d"`, newest); !strings.HasPrefix(string(b), want) {
		t.Errorf("newtemplate/app.tui starts %.40q, want %s", b, want)
	}
}
