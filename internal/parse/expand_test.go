package parse_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// <template> and <use> (SPEC v0.4b §6.18.3-§6.18.5, phase F2): what the
// parser decides on its own. The conformance tests that go through the
// CLI or the Go API are cmd/tuimark's and the root package's
// conformance_v04b_templates_test.go.

func parseDoc(src string) *parse.Document {
	return parse.Parse([]byte(src), "app.tui")
}

func useDiags(doc *parse.Document) []string {
	var out []string
	for _, d := range doc.Diags {
		out = append(out, fmt.Sprintf("%s %d:%d %s", d.Code, d.Line, d.Col, d.Msg))
	}
	return out
}

// screenOf is the first screen of a document.
func screenOf(doc *parse.Document) *ir.Node {
	for _, c := range doc.Root.Children {
		if c.Kind == "screen" {
			return c
		}
	}
	return nil
}

// ADR 0034 for uses: a use counts one for itself and one for every element
// its copy builds, at every depth. A template of ExpansionLimit-1 elements
// copied once is exactly the limit; one more element is V021 at the use,
// which inserts nothing and stays in the source IR, empty.
func TestUseExpansionLimitBoundary(t *testing.T) {
	doc := func(n int) *parse.Document {
		return parseDoc("<tui>\n<template id=\"big\">" + strings.Repeat("<text>a</text>", n) + "</template>\n<screen>\n<use template=\"big\"/>\n</screen>\n</tui>")
	}
	d := doc(parse.ExpansionLimit - 1)
	if got := v021(d); len(got) != 0 || len(screenOf(d).Children) != parse.ExpansionLimit-1 {
		t.Errorf("at the limit: V021 %v, %d children", got, len(screenOf(d).Children))
	}
	d = doc(parse.ExpansionLimit)
	want := `app.tui:4:1 use template="big" exceeds the expansion limit (100000 elements or 16 MiB of placed source): it inserts nothing, and no later include or use expands`
	if got := v021(d); len(got) != 1 || got[0] != want {
		t.Errorf("over the limit: V021 %v, want %s", got, want)
	}
	s := screenOf(d)
	if len(s.Children) != 0 || len(s.Source) != 1 || s.Source[0].Kind != "use" || len(s.Source[0].Children) != 0 {
		t.Errorf("the use inserted %d elements; source %+v", len(s.Children), s.Source)
	}
}

// The V021 is at the innermost use that was expanding; the copy around it
// keeps what it built, and no later use expands.
func TestUseExpansionLimitKeepsTheEnclosingAndStopsTheRest(t *testing.T) {
	src := "<tui>\n" +
		"<template id=\"a\"><text>1</text><text>2</text><use template=\"big\"/><text>3</text></template>\n" +
		"<template id=\"big\">" + strings.Repeat("<text>a</text>", parse.ExpansionLimit) + "</template>\n" +
		"<template id=\"c\"><text>late</text></template>\n" +
		"<screen>\n<use template=\"a\"/>\n<use template=\"c\"/>\n<use template=\"big\"/>\n</screen>\n</tui>"
	d := parseDoc(src)
	got := v021(d)
	if len(got) != 1 || !strings.HasPrefix(got[0], `app.tui:2:46 use template="big" exceeds`) {
		t.Fatalf("V021 %v", got)
	}
	var txt []string
	for _, c := range screenOf(d).Children {
		txt = append(txt, c.Text)
	}
	if strings.Join(txt, "") != "123" {
		t.Errorf("texts %v, want 1 2 3", txt)
	}
	if len(d.Diags) != 1 {
		t.Errorf("diagnostics: %s", diagsOf(d))
	}
}

// A use overflow also stops the includes of later copies; an include
// overflow (step 1) stops every use (step 3).
func TestExpansionLimitAcrossIncludesAndUses(t *testing.T) {
	big := "<template id=\"big\">" + strings.Repeat("<text>a</text>", parse.ExpansionLimit) + "</template>"
	files := map[string]string{"f.tui": "<fragment><text>inc</text></fragment>", "big.tui": texts(parse.ExpansionLimit)}
	d := parse.ParseWith([]byte("<tui>"+big+"<template id=\"t\"><include src=\"f.tui\"/></template><screen><use template=\"big\"/><use template=\"t\"/></screen></tui>"), "app.tui", memLoader(files, "app.tui", nil))
	if got := v021(d); len(got) != 1 || !strings.Contains(got[0], `use template="big"`) || len(screenOf(d).Children) != 0 {
		t.Errorf("use first: V021 %v, %d children", got, len(screenOf(d).Children))
	}
	d = parse.ParseWith([]byte("<tui><template id=\"t\"><text>t</text></template><screen><use template=\"t\"/><include src=\"big.tui\"/></screen></tui>"), "app.tui", memLoader(files, "app.tui", nil))
	if got := v021(d); len(got) != 1 || !strings.Contains(got[0], `include src="big.tui"`) || len(screenOf(d).Children) != 0 {
		t.Errorf("include first: V021 %v, %d children", got, len(screenOf(d).Children))
	}
}

// Uses count too: thirty levels of templates, each copying the next twice,
// return at once with exactly one V021.
func TestUseExpansionLimitFanOut(t *testing.T) {
	var b strings.Builder
	b.WriteString("<tui>")
	for i := 0; i < 30; i++ {
		fmt.Fprintf(&b, "<template id=\"t%d\"><use template=\"t%d\"/><use template=\"t%d\"/></template>", i, i+1, i+1)
	}
	b.WriteString("<template id=\"t30\"><text>x</text></template><screen><use template=\"t0\"/></screen></tui>")
	start := time.Now()
	d := parseDoc(b.String())
	if el := time.Since(start); el > 5*time.Second {
		t.Errorf("took %v", el)
	}
	if got := v021(d); len(got) != 1 {
		t.Errorf("V021 %v, want exactly one", got)
	}
	if n := len(screenOf(d).Children); n <= 0 || n >= parse.ExpansionLimit {
		t.Errorf("%d elements inserted", n)
	}
}

// Each copy charges its template's source span, from <template to the end
// of </template>, and each element it places its placement suffix (the F2
// re-review; a use without id has no prefix): a template whose span and
// one element's suffix make exactly 1 MiB, copied 16 times, is the budget;
// the 17th copy is V021.
func TestUseByteBudgetBoundary(t *testing.T) {
	const head, tail = `<template id="t"><text>a</text><!--`, "--></template>"
	// The suffix ", placed by <use template="t"> at app.tui:1:COL", with
	// a 7-digit column for every use.
	suffix := len(`, placed by <use template="t"> at app.tui:1:`) + 7
	tmpl := head + strings.Repeat("x", 1<<20-suffix-len(head)-len(tail)) + tail
	if first, last := len("<tui>"+tmpl+"<screen>")+1, len("<tui>"+tmpl+"<screen>")+1+16*len(`<use template="t"/>`); len(fmt.Sprint(first)) != 7 || len(fmt.Sprint(last)) != 7 {
		t.Fatalf("columns %d..%d", first, last)
	}
	place := func(n int) *parse.Document {
		return parseDoc("<tui>" + tmpl + "<screen>" + strings.Repeat(`<use template="t"/>`, n) + "</screen></tui>")
	}
	d := place(16)
	if got := v021(d); len(got) != 0 || len(screenOf(d).Children) != 16 {
		t.Errorf("at the budget: V021 %v, %d children", got, len(screenOf(d).Children))
	}
	d = place(17)
	if got := v021(d); len(got) != 1 || !strings.Contains(got[0], `use template="t" exceeds the expansion limit`) || len(screenOf(d).Children) != 16 {
		t.Errorf("over the budget: V021 %v, %d children", got, len(screenOf(d).Children))
	}
}

// A self-check of an element a template holds is reported once, however
// many copies there are, and with no placement suffix (SPEC v0.4b
// §6.18.6).
func TestTemplateSelfChecksOncePerWrittenElement(t *testing.T) {
	d := parseDoc("<tui>\n<template id=\"t\">\n<text wrap=\"bogus\" zap=\"1\">{x.name}</text>\n</template>\n<screen>\n<use template=\"t\" with=\"a as x\"/>\n<use template=\"t\" with=\"b as x\"/>\n</screen>\n</tui>")
	got := useDiags(d)
	if len(got) != 2 || !strings.HasPrefix(got[0], "V003 3:7 wrap: bad value") || !strings.HasPrefix(got[1], `V002 3:20 attribute "zap" is not allowed on <text>`) || strings.Contains(strings.Join(got, ""), "placed by") {
		t.Errorf("diagnostics: %v", got)
	}
}

// An unused template's elements get only the checks they give themselves:
// no V012 for an input without id, no V004, no B-code. A malformed each or
// if is such a check, V011, although §6.18.3's list names only V001,
// V002, V003, V007, V013, V015, and V018; a fragment checked on its own
// (F1) reports it the same way. Reported as an erratum of the list.
func TestUnusedTemplateReportsItsOwnV011(t *testing.T) {
	d := parseDoc("<tui>\n<template id=\"t\"><list id=\"l\" each=\"bad\"><item/></list><input bind=\"v\"/><text id=\"a\">a</text><text id=\"a\">b</text></template>\n<screen id=\"m\"/>\n</tui>")
	if got := useDiags(d); len(got) != 1 || got[0] != `V011 2:31 each="bad" must be "path as alias"` {
		t.Errorf("diagnostics: %v", got)
	}
}

// with= grammar and collisions (SPEC v0.4b §6.18.5): each is one V011 at
// the use, which inserts nothing; an each alias inside the copy that
// repeats a with alias is V011 at that element, and the copy is inserted.
func TestWithGrammarAndCollisions(t *testing.T) {
	const field = `<template id="field"><text>{f.label}</text></template>`
	doc := func(use string) *parse.Document {
		return parseDoc("<tui>\n" + field + "\n<template id=\"t\"><text>{r.city}</text></template>\n<screen id=\"main\">\n" + use + "\n</screen>\n</tui>")
	}
	for _, w := range []string{"x as f,y as g", "x as f,  y as g", "x as f, ", "x as f,, y as g", ""} {
		d := doc(`<use id="u" template="field" with="` + w + `"/>`)
		want := fmt.Sprintf(`V011 5:1 with=%q: want "path as alias", separated by ", "`, w)
		if got := useDiags(d); len(got) != 1 || got[0] != want || len(screenOf(d).Children) != 0 {
			t.Errorf("with=%q: %v, %d children", w, got, len(screenOf(d).Children))
		}
	}
	d := doc(`<use id="u" template="field" with="x as f, y as f"/>`)
	if got := useDiags(d); len(got) != 1 || got[0] != `V011 5:1 with alias "f" appears twice` || len(screenOf(d).Children) != 0 {
		t.Errorf("twice: %v", got)
	}
	d = doc(`<list id="l" each="rows as r" key="r.id"><item><use template="t" with="r.addr as r"/></item></list>`)
	if got := useDiags(d); len(got) != 1 || got[0] != `V011 5:48 with alias "r" repeats an alias in scope` {
		t.Errorf("in scope: %v", got)
	}
	if item := screenOf(d).Children[0].Children[0]; len(item.Children) != 0 {
		t.Errorf("the use inserted %d elements", len(item.Children))
	}
	// An enclosing use's with alias is in scope at a use of its copy.
	d = parseDoc("<tui>\n" + field + "\n<template id=\"outer\"><use template=\"field\" with=\"f.x as f\"/></template>\n<screen id=\"main\">\n<use template=\"outer\" with=\"a as f\"/>\n</screen>\n</tui>")
	if got := useDiags(d); len(got) != 1 || !strings.HasPrefix(got[0], `V011 3:22 with alias "f" repeats an alias in scope`) {
		t.Errorf("nested: %v", got)
	}
	d = parseDoc("<tui>\n<template id=\"t2\"><box each=\"a.items as a\"><text>{a}</text></box></template>\n<screen id=\"main\">\n    <use id=\"u\" template=\"t2\" with=\"data as a\"/>\n</screen>\n</tui>")
	got := useDiags(d)
	if len(got) != 1 || got[0] != `V011 2:19 each alias "a" repeats the with alias of <use id="u"> at app.tui:4:5` {
		t.Errorf("each alias: %v", got)
	}
	box := screenOf(d).Children
	if len(box) != 1 || box[0].Each != "data.items as a" || box[0].Children[0].Text != "{a}" {
		t.Errorf("the copy: %+v", box)
	}
}

// Paths go through the with aliases where they are read (SPEC v0.4b
// §6.18.5), each alias of the copy shadowing them, and what the element
// wrote is kept for `tuimark ir`.
func TestWithAliasSubstitution(t *testing.T) {
	d := parseDoc(`<tui>
<template id="t">
  <col if="!f.off" class:hot="f.hot" title="{f.title}">
    <text>{f.a} and {g}</text>
    <list id="l" each="f.rows as f" key="f.id" checked="f.marked"><item><text>{f.name}</text></item></list>
    <input id="i" bind="f.v" hidden="f.h" disabled="true"/>
  </col>
</template>
<screen id="main"><use id="u" template="t" with="form.x as f"/></screen>
</tui>`)
	col := screenOf(d).Children[0]
	if col.If != "!form.x.off" || col.Attrs["title"] != "{form.x.title}" || col.ClassGuards[0].Guard != "form.x.hot" {
		t.Errorf("col: if %q title %q guard %q", col.If, col.Attrs["title"], col.ClassGuards[0].Guard)
	}
	if col.Written == nil || col.Written.If != "!f.off" || col.Written.Attrs["title"] != "{f.title}" {
		t.Errorf("col as written: %+v", col.Written)
	}
	if txt := col.Children[0]; txt.Text != "{form.x.a} and {g}" || txt.Written.Text != "{f.a} and {g}" {
		t.Errorf("text %q, written %q", txt.Text, txt.Written.Text)
	}
	l := col.Children[1]
	// The list's own each alias f shadows the with alias inside it (V011
	// at the list), and its key and rows read the row.
	if l.Each != "form.x.rows as f" || l.Attrs["key"] != "f.id" || l.Attrs["checked"] != "form.x.marked" || l.Children[0].Children[0].Text != "{f.name}" {
		t.Errorf("list: each %q key %q checked %q row %q", l.Each, l.Attrs["key"], l.Attrs["checked"], l.Children[0].Children[0].Text)
	}
	in := col.Children[2]
	if in.Bind != "form.x.v" || in.Attrs["hidden"] != "form.x.h" || in.Attrs["disabled"] != "true" || in.ID != "u__i" {
		t.Errorf("input: bind %q hidden %q disabled %q id %q", in.Bind, in.Attrs["hidden"], in.Attrs["disabled"], in.ID)
	}
	if got := useDiags(d); len(got) != 1 || !strings.HasPrefix(got[0], `V011 5:5 each alias "f" repeats the with alias of <use id="u">`) {
		t.Errorf("diagnostics: %v", got)
	}
}

// Scoped ids nest, and a #x in a template names the element of the copy
// whose local id is x, else resolves where the use stands (SPEC v0.4b
// §6.18.4).
func TestScopedIDsAndLocalReferences(t *testing.T) {
	d := parseDoc(`<tui>
<template id="inner"><input id="c" bind="v"/></template>
<template id="mid">
  <tabs id="tabs">
    <tab id="one" label="One" focus="#b__c"><use id="b" template="inner"/></tab>
    <tab id="two" label="Two" focus="#q"><text>t</text></tab>
  </tabs>
</template>
<screen id="main"><input id="q" bind="q"/><use id="a" template="mid"/></screen>
</tui>`)
	// #q is no local id of the copy: it names the screen's q, which is
	// outside the tab (B005), not a missing id.
	if got := useDiags(d); len(got) != 1 || got[0] != `B005 6:5 focus="#q" names a node outside this tab; a tab's focus= names a node inside it, placed by <use id="a"> at app.tui:9:43` {
		t.Fatalf("diagnostics: %v", got)
	}
	for _, id := range []string{"a__tabs", "a__one", "a__b__c", "a__two", "q"} {
		if d.IDs[id] == nil {
			t.Errorf("no node %s", id)
		}
	}
	if f := d.IDs["a__one"].Attrs["focus"]; f != "#a__b__c" {
		t.Errorf("one's focus %q, want #a__b__c", f)
	}
	if f := d.IDs["a__two"].Attrs["focus"]; f != "#q" {
		t.Errorf("two's focus %q, want #q", f)
	}
	if w := d.IDs["a__one"].Written.Attrs["focus"]; w != "#b__c" {
		t.Errorf("one's focus as written %q", w)
	}
}

// The ids of uses and templates are in the id space; a use without id in a
// container each template needs none (its ids are V004 there anyway).
func TestUseIDsInTheIDSpace(t *testing.T) {
	d := parseDoc("<tui>\n<template id=\"t\"><text id=\"x\">x</text></template>\n<screen id=\"main\">\n<use id=\"t\" template=\"t\"/>\n<text id=\"t__x\">y</text>\n</screen>\n</tui>")
	got := useDiags(d)
	if len(got) != 2 || got[0] != `V004 4:1 duplicate id "t" (first defined at 2:1)` || !strings.HasPrefix(got[1], `V004 5:1 duplicate id "t__x" (first defined at 2:18, placed by <use id="t"> at app.tui:4:1)`) {
		t.Errorf("diagnostics: %v", got)
	}
}

// A use in a template holding a use of itself never expands at run time
// either: the static V019 is the only report, and the other uses of the
// templates on the cycle still expand.
func TestCycleReportedOnceAndOthersExpand(t *testing.T) {
	d := parseDoc("<tui>\n<template id=\"a\"><text>a</text><use template=\"b\"/></template>\n<template id=\"b\"><text>b</text><use template=\"a\"/></template>\n<screen id=\"main\"><use template=\"a\"/><use template=\"b\"/></screen>\n</tui>")
	got := useDiags(d)
	if len(got) != 2 || !strings.HasPrefix(got[0], "V019 2:32 ") || !strings.HasPrefix(got[1], "V019 3:32 ") {
		t.Errorf("diagnostics: %v", got)
	}
	var txt []string
	for _, c := range screenOf(d).Children {
		txt = append(txt, c.Text)
	}
	if strings.Join(txt, " ") != "a b" {
		t.Errorf("texts %v, want a b", txt)
	}
}

// 3 levels of templates, each copying the next twice: 8 copies of the
// leaf per document.
func BenchmarkUseExpansion(b *testing.B) {
	src := []byte(`<tui>
<template id="leaf"><row id="r"><text>{f.label}</text><input id="in" bind="f.value"/></row></template>
<template id="mid"><col id="c"><use id="x" template="leaf" with="g.x as f"/><use id="y" template="leaf" with="g.y as f"/></col></template>
<template id="top"><box id="b"><use id="p" template="mid" with="h.p as g"/><use id="q" template="mid" with="h.q as g"/></box></template>
<screen id="main"><use id="one" template="top" with="d.one as h"/><use id="two" template="top" with="d.two as h"/></screen>
</tui>`)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		d := parse.Parse(src, "app.tui")
		if len(d.Diags) != 0 || d.IDs["two__q__y__in"] == nil {
			b.Fatalf("diagnostics %v", d.Diags)
		}
	}
}
