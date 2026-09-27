package tuimark_test

import (
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

// SPEC v0.2b §21 conformance tests of the ergonomics phase through the
// public API: each on containers (49), multi-select (64), and the
// built-in actions in Catalog and Validate (62). Key handling itself is
// covered in internal/host (p2_test.go) and through play in cmd/tuimark.

const p2Doc = `<tui version="2">
<keymap>
  <bind keys="space" action="check-toggle" when="#l:focus"/>
  <bind keys="j" action="move-next"/>
  <bind keys="enter" action="open" when="#pane"/>
</keymap>
<screen id="main" focus="#l">
  <col id="pane">
    <list id="l" each="rows as r" key="r.id" checked="marked" mark="*" on:change="marks">
      <item><text>{r.name}</text></item>
    </list>
  </col>
  <row id="tags" each="rows as r" key="r.id"><text>{r.name}</text></row>
</screen>
</tui>`

func p2App(t *testing.T) *tuimark.App {
	t.Helper()
	app, err := tuimark.Parse(strings.NewReader(p2Doc))
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Bind("rows", []any{map[string]any{"id": "a", "name": "alpha"}, map[string]any{"id": 2, "name": "two"}}); err != nil {
		t.Fatal(err)
	}
	if err := app.Bind("marked", []any{2}); err != nil {
		t.Fatal(err)
	}
	return app
}

// 49, 64: Dump carries the container template's keys, the checked row, and
// the mark channel; classes stay a version="2" member.
func TestDumpEachAndChecked(t *testing.T) {
	d, err := p2App(t).Dump(20, 4)
	if err != nil {
		t.Fatal(err)
	}
	if !d.OK || len(d.Errors) != 0 {
		t.Fatalf("errors %v", d.Errors)
	}
	var checked, tags []string
	for _, n := range d.Nodes {
		if n.Checked {
			checked = append(checked, n.Tag+":"+n.Key)
		}
		if n.Tag == "text" && n.Key != "" {
			tags = append(tags, n.Key+"="+n.Text)
		}
	}
	if strings.Join(checked, " ") != "item:2" || strings.Join(tags, " ") != "a=alpha 2=two" {
		t.Errorf("checked %v, template keys %v", checked, tags)
	}
	if d.Grid[0] != "  alpha             " || d.Grid[1] != "* two               " {
		t.Errorf("grid %q", d.Grid)
	}
}

// 62: referenced built-ins are listed with Builtin: true next to the host
// actions, and a static incompatible target is B007.
func TestCatalogAndValidateBuiltins(t *testing.T) {
	app := p2App(t)
	got := map[string]bool{}
	for _, s := range app.Catalog() {
		got[s.Name] = s.Builtin
	}
	if !got["check-toggle"] || !got["move-next"] || got["open"] || got["marks"] {
		t.Errorf("catalog %v", got)
	}
	for _, d := range app.Validate() {
		if d.Severity == "error" {
			t.Errorf("valid document: %s", d)
		}
	}
	bad, err := tuimark.Parse(strings.NewReader(strings.Replace(p2Doc, `when="#l:focus"`, `when="#pane:focus"`, 1)))
	if err != nil {
		t.Fatal(err)
	}
	_ = bad.Bind("rows", []any{})
	_ = bad.Bind("marked", []any{})
	var codes []string
	for _, d := range bad.Validate() {
		codes = append(codes, d.Code)
	}
	// #pane is a col: check-toggle can never act on it (and #pane is not
	// focusable, which is not a diagnostic).
	if strings.Join(codes, " ") != "B007" {
		t.Errorf("codes %v", codes)
	}
}
