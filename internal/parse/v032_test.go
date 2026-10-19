package parse_test

import (
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// msgs returns "CODE: msg" for every diagnostic of doc.
func msgs(doc *parse.Document) []string {
	var out []string
	for _, d := range doc.Diags {
		out = append(out, d.Code+": "+d.Msg)
	}
	return out
}

func docWith(version, body string) string {
	return "<tui version=\"" + version + "\">\n<screen>\n" + body + "\n</screen>\n</tui>"
}

// v0.3.2: a bad version names every valid one. (V014 named them too, until
// v0.4.1 made version optional.)
func TestBadVersionMentionsEveryVersion(t *testing.T) {
	doc := parse.Parse([]byte(`<tui version="4"><screen/></tui>`), "")
	d, ok := coreFind(doc, "V003")
	if !ok || !strings.Contains(d.Msg, `"1", "2", or "3"`) {
		t.Errorf("V003 = %v (%v)", d, msgs(doc))
	}
}

// v0.3.2: text between '&' and the next ';' that is not an entity name is
// a bare '&'; real entities and numeric references keep their messages.
func TestBareAmpersandBeforeSemicolon(t *testing.T) {
	for _, c := range []struct {
		src, want string
	}{
		{"<text>a & b; c</text>", "bare '&' (write &amp;)"},
		{"<text>a &; c</text>", "bare '&' (write &amp;)"},
		{"<text>a &1x; c</text>", "bare '&' (write &amp;)"},
		{"<text>a &#12 x; c</text>", "bare '&' (write &amp;)"},
		{"<text>a & b</text>", "bare '&' (write &amp;)"},
		{"<text>&nbsp;</text>", "entity &nbsp; is not allowed"},
		{"<text>&#65;</text>", "entity &#65; is not allowed"},
		{"<text>&#x41;</text>", "entity &#x41; is not allowed"},
	} {
		_, _, _, err := parse.ParseXML([]byte(docWith("1", c.src)))
		if err == nil || !strings.Contains(err.Msg, c.want) {
			t.Errorf("%s: err = %v, want %q", c.src, err, c.want)
		}
	}
	if _, _, _, err := parse.ParseXML([]byte(docWith("1", "<text>a &amp; b &lt;c&gt; &quot;&apos;</text>"))); err != nil {
		t.Errorf("predefined entities: %v", err)
	}
}

// v0.3.2: a DOCTYPE, processing instruction, or declaration after the root
// gets the message it gets elsewhere, not the single-root one.
func TestMarkupAfterRoot(t *testing.T) {
	for _, c := range []struct {
		tail, want string
	}{
		{"<!DOCTYPE x>", "DTD/DOCTYPE is not allowed"},
		{"<?pi x?>", "processing instructions are not allowed"},
		{"<![CDATA[x]]>", "declarations are not allowed"},
		{"<tui/>", "a document must have a single root element"},
	} {
		_, _, _, err := parse.ParseXML([]byte("<tui version=\"1\"><screen/></tui>\n" + c.tail))
		if err == nil || !strings.Contains(err.Msg, c.want) {
			t.Errorf("%s: err = %v, want %q", c.tail, err, c.want)
		}
	}
}

// v0.3.2: an unknown tag with text reports V001 only; the V013 "wrap it in
// <text>" would be noise next to it. A known tag still gets V013.
func TestUnknownTagWithTextIsOnlyV001(t *testing.T) {
	doc := parse.Parse([]byte(docWith("1", "<tex>a</tex>")), "")
	if got := coreCodes(doc); got != "V001" {
		t.Errorf("codes %q: %v", got, msgs(doc))
	}
	doc = parse.Parse([]byte(docWith("1", "<box>a</box>")), "")
	if got := coreCodes(doc); got != "V013" {
		t.Errorf("known tag: codes %q: %v", got, msgs(doc))
	}
	// A version="2" tag in a version="1" document is unknown there too.
	doc = parse.Parse([]byte(docWith("1", `<tabs id="t">x</tabs>`)), "")
	if got := coreCodes(doc); got != "V001" {
		t.Errorf("v2 tag in v1: codes %q: %v", got, msgs(doc))
	}
}

// v0.3.2: V001 / V002 / V003 "did you mean" hints and the HTML-name table.
func TestDidYouMeanHints(t *testing.T) {
	for _, c := range []struct {
		name, version, body, code, want string
		not                             bool // want must be absent
	}{
		{"tag typo", "1", `<tex>a</tex>`, "V001", "; did you mean <text>?", false},
		{"tag typo 2", "1", `<bx/>`, "V001", "; did you mean <box>?", false},
		{"tag too far", "1", `<qqqqq/>`, "V001", "did you mean", true},
		{"tag too short", "1", `<q/>`, "V001", "did you mean", true},
		{"v2 tag typo in v1", "1", `<tabel/>`, "V001", `did you mean <table> (requires version="2")?`, false},
		{"v2 tag typo in v2", "2", `<tabel/>`, "V001", "; did you mean <table>?", false},
		{"html div", "1", `<div/>`, "V001", "; use <box>, <col>, or <row>", false},
		{"html span", "1", `<span>x</span>`, "V001", "; use <text>", false},
		{"html h3", "1", `<h3/>`, "V001", "; use <text>", false},
		{"html ul", "1", `<ul/>`, "V001", "; use <list>", false},
		{"html li", "1", `<li/>`, "V001", "; use <item> inside a <list>", false},
		{"html dialog", "1", `<dialog/>`, "V001", "; use <modal>", false},
		{"html checkbox", "1", `<checkbox/>`, "V001", `tag <checkbox> (requires a <tui> without version)`, false},
		{"html select", "1", `<select/>`, "V001", "on:select picks one row", false},
		{"html textarea", "1", `<textarea/>`, "V001", "use <input>", false},
		{"html template", "1", `<template/>`, "V001", "only <style src> loads another file", false},
		{"html img", "1", `<img/>`, "V001", "images are not supported", false},
		{"attr typo", "1", `<box padd="1"/>`, "V002", `; did you mean "pad"?`, false},
		{"attr typo gap", "1", `<box gaps="1"/>`, "V002", `; did you mean "gap"?`, false},
		{"event typo", "1", `<list id="l" each="xs as x" key="x" on:selct="go"><item/></list>`, "V002", `; did you mean "on:select"?`, false},
		{"attr no match", "1", `<box zzzzzz="1"/>`, "V002", "did you mean", true},
		{"v2 attr typo in v2", "2", `<list id="l" each="xs as x" key="x" chekced="m"><item/></list>`, "V002", `; did you mean "checked"?`, false},
		{"v2 attr is not suggested in v1", "1", `<list id="l" each="xs as x" key="x" chekced="m"><item/></list>`, "V002", "did you mean", true},
		// A real attribute on the wrong tag is misplaced, not misspelled.
		{"misplaced bind", "1", `<box bind="x"/>`, "V002", "did you mean", true},
		{"misplaced wrap", "1", `<box wrap="truncate"/>`, "V002", "did you mean", true},
		{"prop typo", "1", `<box style="colr: red"/>`, "V003", `; did you mean "color"?`, false},
		{"prop typo 2", "1", `<box style="paddng: 1"/>`, "V003", `; did you mean "padding"?`, false},
		{"prop no match", "1", `<box style="zzzzzz: 1"/>`, "V003", "did you mean", true},
		{"v2 prop in v1 says what it needs", "1", `<box style="grid-colums: 2"/>`, "V003", `; did you mean "grid-columns" (requires version="2")?`, false},
		{"v2 prop typo in v2", "2", `<box style="grid-colums: 2"/>`, "V003", `; did you mean "grid-columns"?`, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			doc := parse.Parse([]byte(docWith(c.version, c.body)), "")
			d, ok := coreFind(doc, c.code)
			if !ok {
				t.Fatalf("no %s: %v", c.code, msgs(doc))
			}
			if has := strings.Contains(d.Msg, c.want); has == c.not {
				t.Errorf("%s message %q: contains %q = %v", c.code, d.Msg, c.want, has)
			}
		})
	}
	for _, c := range []struct{ val, want string }{
		{"darkk", `did you mean "dark"?`}, {"ligth", `did you mean "light"?`}, {"qqqqqq", ""},
	} {
		doc := parse.Parse([]byte(`<tui version="1" theme="`+c.val+`"><screen/></tui>`), "")
		d, ok := coreFind(doc, "V003")
		if !ok || (c.want != "" && !strings.Contains(d.Msg, c.want)) || (c.want == "" && strings.Contains(d.Msg, "did you mean")) {
			t.Errorf("theme %q: %v (%v)", c.val, d, msgs(doc))
		}
	}
	// "auto" is a version="2" theme: suggested there, not in version="1".
	doc := parse.Parse([]byte(`<tui version="2" theme="aut"><screen/></tui>`), "")
	if d, _ := coreFind(doc, "V003"); !strings.Contains(d.Msg, `did you mean "auto"?`) {
		t.Errorf("theme aut in v2: %v", d)
	}
}

// kmDoc is a document whose screen holds #b2 and whose <tui> holds km.
func kmDoc(version, km string) string {
	return "<tui version=\"" + version + "\">\n<screen>\n<box id=\"b2\"/>\n</screen>\n" + km + "\n</tui>"
}

// v0.3.2: the keymap when= selector goes through the same selector parser,
// so an impossible id or class there is V003 as well.
func TestKeymapWhenRejectsImpossibleSelectors(t *testing.T) {
	for _, when := range []string{"#1", ".9x", "box.-x", "#-a"} {
		doc := parse.Parse([]byte(kmDoc("1", `<keymap><bind keys="x" action="go" when="`+when+`"/></keymap>`)), "")
		d, ok := coreFind(doc, "V003")
		if !ok || !strings.Contains(d.Msg, "must start with a letter or '_'") {
			t.Errorf("when=%q: %v", when, msgs(doc))
		}
	}
	// version="3": the <keymap when> form.
	doc := parse.Parse([]byte(kmDoc("3", `<keymap when="#1"><bind keys="x" action="go"/></keymap>`)), "")
	if d, ok := coreFind(doc, "V003"); !ok || !strings.Contains(d.Msg, "must start with a letter or '_'") {
		t.Errorf("keymap when: %v", msgs(doc))
	}
	doc = parse.Parse([]byte(kmDoc("1", `<keymap><bind keys="x" action="go" when="#b2"/></keymap>`)), "")
	if len(doc.Diags) != 0 {
		t.Errorf("valid when: %v", msgs(doc))
	}
}

// v0.3.2: a leading '+' is rejected in the gap= and pad= attributes, which
// share the declaration checks.
func TestLeadingPlusInGapAndPadAttributes(t *testing.T) {
	for _, body := range []string{`<box gap="+3"/>`, `<box pad="+1"/>`, `<box pad="1 +2"/>`} {
		doc := parse.Parse([]byte(docWith("1", body)), "")
		if got := coreCodes(doc); got != "V003" {
			t.Errorf("%s: codes %q: %v", body, got, msgs(doc))
		}
	}
	for _, body := range []string{`<box gap="3"/>`, `<box pad="1 2"/>`} {
		if doc := parse.Parse([]byte(docWith("1", body)), ""); len(doc.Diags) != 0 {
			t.Errorf("%s: %v", body, msgs(doc))
		}
	}
	if doc := parse.Parse([]byte(docWith("1", `<box style="gap: +3"/>`)), ""); coreCodes(doc) != "V003" {
		t.Errorf("style gap: %v", msgs(doc))
	}
}

// v0.3.2: fmt keeps the XML declaration as written, on the first line, and
// stays idempotent.
func TestFormatKeepsXMLDeclaration(t *testing.T) {
	for _, src := range []string{
		"<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<tui version=\"1\"><screen/></tui>\n",
		"<?xml version='1.0'?><!-- c --><tui version=\"1\"><screen/></tui>",
		"\ufeff<?xml version=\"1.0\" standalone=\"yes\"?>\r\n\r\n<tui version=\"1\"><screen/></tui>\r\n",
	} {
		out, err := parse.Format([]byte(src))
		if err != nil {
			t.Fatalf("%q: %v", src, err)
		}
		first, _, _ := strings.Cut(out, "\n")
		wantFirst, _, _ := strings.Cut(strings.TrimPrefix(strings.ReplaceAll(src, "\r\n", "\n"), "\ufeff"), "?>")
		if !strings.HasPrefix(out, wantFirst+"?>\n") {
			t.Errorf("%q: declaration not kept on the first line: %q (first %q)", src, out, first)
		}
		again, err := parse.Format([]byte(out))
		if err != nil || again != out {
			t.Errorf("%q: not idempotent:\n%q\n%q (%v)", src, out, again, err)
		}
	}
	// No declaration, none added.
	if out, _ := parse.Format([]byte(`<tui version="1"><screen/></tui>`)); strings.Contains(out, "<?xml") {
		t.Errorf("declaration invented: %q", out)
	}
}
