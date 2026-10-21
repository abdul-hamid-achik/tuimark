package tuimark_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// SPEC v0.4c §24 conformance tests of the slot, phase C2 (tests 241, 248,
// and 260), the parts the Go API observes: a document whose uses hold
// fills gives the frame of the same markup written by hand, and a fill is
// charged to the expansion that builds it. The CLI parts (validate's
// lists, the dumps' classes, ids, and texts) are cmd/tuimark's
// conformance_v04c_slot_test.go. The fixtures are testdata/v04c/<test>.

// 241. The fills and the classes of the uses give the hand-written frame.
func TestSlotFillsEqualTheHandWrittenMarkup(t *testing.T) {
	dir := filepath.Join("testdata", "v04c", "241")
	data := filepath.Join(dir, "d.json")
	sameFrame(t, "241", loadWith(t, filepath.Join(dir, "app.tui"), data), loadWith(t, filepath.Join(dir, "hand.tui"), data), 20, 6)
}

// limitMsg is the message of a V021 at a use of template id.
func limitMsg(id string) string {
	return `use template="` + id + `" exceeds the expansion limit (100000 elements or 16 MiB of placed source): it inserts nothing, and no later include or use expands`
}

// writeDoc writes doc as name in a temporary directory.
func writeDoc(t *testing.T, name, doc string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// noErrors checks that validate reports no error (warnings are allowed).
func noErrors(t *testing.T, what, path string) {
	t.Helper()
	for _, d := range mustLoad(t, path).Validate() {
		if d.Severity == "error" {
			t.Errorf("%s: %s", what, diagText(d))
		}
	}
}

// 248, the fills: a fill's elements count, a slot does not, and the
// checks of a later use go on after a V021; a forwarded fill is charged
// to the inner use, whose content holds the forwarding slot.
func TestSlotFillsAreCharged(t *testing.T) {
	texts := func(n int) string { return strings.Repeat("      <text>a</text>\n", n) }
	fill := func(n int, after string) string {
		return "<tui>\n  <template id=\"w\"><slot/></template>\n  <screen id=\"main\">\n    <use template=\"w\">\n" + texts(n) + "    </use>\n" + after + "  </screen>\n</tui>\n"
	}
	noErrors(t, "99 999 texts", writeDoc(t, "fill.tui", fill(99999, "")))
	app := mustLoad(t, writeDoc(t, "fill.tui", fill(100000, "")))
	if got, want := diagTexts(app.Validate()), []string{`V021 fill.tui:4:5 /screen/use id= ` + limitMsg("w")}; !reflect.DeepEqual(got, want) {
		t.Errorf("100 000 texts: %v", got)
	}
	screenIsEmpty(t, app)
	// The use after the V021 does not expand, but its attributes are
	// checked against its template.
	after := mustLoad(t, writeDoc(t, "fill.tui", fill(100000, "    <use template=\"w\" style=\"x\"/>\n")))
	if got, want := diagTexts(after.Validate()), []string{
		`V021 fill.tui:4:5 /screen/use id= ` + limitMsg("w"),
		`V002 fill.tui:100006:23 /screen/use id= attribute "style" is not allowed on <use>: a <use> is not a node; give this copy a static class="…" and style that class, put the attribute on the template's element, or declare it in the template's props`,
	}; !reflect.DeepEqual(got, want) {
		t.Errorf("a use after the V021: %v", got)
	}
	fw := func(n int) string {
		return "<tui>\n  <template id=\"inner\"><slot/></template>\n  <template id=\"outer\"><use template=\"inner\"><slot/></use></template>\n  <screen id=\"main\">\n    <use template=\"outer\">\n" + texts(n) + "    </use>\n  </screen>\n</tui>\n"
	}
	noErrors(t, "forwarded, 99 998 texts", writeDoc(t, "fw.tui", fw(99998)))
	app = mustLoad(t, writeDoc(t, "fw.tui", fw(99999)))
	if got, want := diagTexts(app.Validate()), []string{`V021 fw.tui:3:24 /screen/use id= ` + limitMsg("inner")}; !reflect.DeepEqual(got, want) {
		t.Errorf("forwarded, 99 999 texts: %v", got)
	}
	screenIsEmpty(t, app)
}

// §6.18.8: a slot is a depth level though not an element. 126 nested uses
// of a template that is one slot put a box's text at level 256 (tui,
// screen, 126 uses, 126 slots, box, text); one more box is a V021 at the
// innermost use, whose fill is being built when it goes past.
func TestSlotIsADepthLevel(t *testing.T) {
	doc := func(inner string) string {
		return "<tui>\n<template id=\"w\"><slot/></template>\n<screen id=\"main\">\n" + strings.Repeat(`<use template="w">`, 126) + inner + strings.Repeat("</use>", 126) + "\n</screen>\n</tui>\n"
	}
	noErrors(t, "level 256", writeDoc(t, "d.tui", doc("<box><text>x</text></box>")))
	app := mustLoad(t, writeDoc(t, "d.tui", doc("<box><box><text>x</text></box></box>")))
	if got, want := diagTexts(app.Validate()), []string{`V021 d.tui:4:2251 /screen/use id= use template="w" exceeds the expansion depth limit of 256 levels: it inserts nothing, and no later include or use expands`}; !reflect.DeepEqual(got, want) {
		t.Errorf("level 257: %v", got)
	}
}

// 260. A forwarded fill past 16 MiB is charged when it is built, to the
// inner use's expansion.
func TestSlotForwardedFillPast16MiB(t *testing.T) {
	doc := "<tui>\n  <template id=\"inner\"><slot/></template>\n  <template id=\"outer\"><use template=\"inner\"><slot/></use></template>\n  <screen id=\"main\">\n    <use template=\"outer\">\n      <text>" + strings.Repeat("a", 16777216) + "</text>\n    </use>\n  </screen>\n</tui>\n"
	app := mustLoad(t, writeDoc(t, "fw16.tui", doc))
	if got, want := diagTexts(app.Validate()), []string{`V021 fw16.tui:3:24 /screen/use id= ` + limitMsg("inner")}; !reflect.DeepEqual(got, want) {
		t.Errorf("validate: %v", got)
	}
	screenIsEmpty(t, app)
}
