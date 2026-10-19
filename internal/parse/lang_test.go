package parse_test

import (
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// SPEC v0.4b §5.1: the language a document is read in. No version is the
// current language (4); a written 1, 2, or 3 pins it; any other value,
// version="4" included, is V003 and reads as 1, as does the spike <app>
// and a document whose root is not <tui>.
func TestDocumentLang(t *testing.T) {
	for _, c := range []struct {
		src  string
		want ir.Lang
		v003 string
	}{
		{`<tui><screen/></tui>`, ir.LangCurrent, ""},
		{`<tui theme="dark"><screen/></tui>`, ir.LangCurrent, ""},
		{`<tui version="1"><screen/></tui>`, ir.Lang1, ""},
		{`<tui version="2"><screen/></tui>`, ir.Lang2, ""},
		{`<tui version="3"><screen/></tui>`, ir.Lang3, ""},
		{`<tui version="4"><screen/></tui>`, ir.Lang1, `version="4": want "1", "2", or "3" (the document is read as version="1")`},
		{`<tui version="abc"><screen/></tui>`, ir.Lang1, `version="abc": want "1", "2", or "3" (the document is read as version="1")`},
		{`<tui version=""><screen/></tui>`, ir.Lang1, `version="": want "1", "2", or "3" (the document is read as version="1")`},
		{`<app><text>x</text></app>`, ir.Lang1, ""},
		{`<foo/>`, ir.Lang1, ""},
	} {
		doc := parse.Parse([]byte(c.src), "t.tui")
		if doc.Lang != c.want {
			t.Errorf("%s: Lang %d, want %d", c.src, doc.Lang, c.want)
		}
		if doc.V2 != doc.Lang.V2() || doc.V3 != doc.Lang.V3() {
			t.Errorf("%s: V2=%v V3=%v disagree with Lang %d", c.src, doc.V2, doc.V3, doc.Lang)
		}
		var got []string
		for _, d := range doc.Diags {
			if d.Code == "V003" {
				got = append(got, d.Msg)
			}
		}
		if c.v003 == "" && len(got) != 0 || c.v003 != "" && (len(got) != 1 || got[0] != c.v003) {
			t.Errorf("%s: V003 %q, want %q", c.src, got, c.v003)
		}
	}
}

// SPEC v0.4b §13.1, §23.6: IR 0.4 for the current language only.
func TestIRVersionByLang(t *testing.T) {
	for src, want := range map[string]string{
		`<tui><screen/></tui>`:             ir.VersionV4,
		`<tui version="3"><screen/></tui>`: ir.VersionV3,
		`<tui version="2"><screen/></tui>`: ir.VersionV2,
		`<tui version="1"><screen/></tui>`: ir.Version,
		`<tui version="4"><screen/></tui>`: ir.Version,
		`<app><text>x</text></app>`:        ir.Version,
	} {
		if got := parse.BuildIR(parse.Parse([]byte(src), "t.tui"), nil).Version; got != want {
			t.Errorf("%s: IR %q, want %q", src, got, want)
		}
	}
}

// injectCurrent stands in for a later 0.4b phase: it adds a current-only
// tag and a current-only attribute on an existing tag, and restores both
// tables when the test ends. Tests that call it never run in parallel.
func injectCurrent(t *testing.T) {
	t.Helper()
	kinds, attrs := ir.KindsCurrent, parse.TagAttrsCurrent
	t.Cleanup(func() { ir.KindsCurrent, parse.TagAttrsCurrent = kinds, attrs })
	ir.KindsCurrent = []string{"checkbox"}
	parse.TagAttrsCurrent = map[string]map[string]bool{
		"checkbox": {"id": true, "bind": true, "label": true},
		"list":     {"single": true},
	}
}

// SPEC v0.4b §5.1: in a pinned document a current-only tag is V001 and a
// current-only attribute V002, each with a message that names the item
// without calling it unknown and ends with the hint; the current
// language accepts both. The machinery is F0's; the tables stay empty
// until each phase fills them.
func TestCurrentHintOnPinnedDocuments(t *testing.T) {
	if len(ir.KindsCurrent) != 0 || len(parse.TagAttrsCurrent) != 0 {
		t.Fatalf("F0 declares no current-language vocabulary: %v %v", ir.KindsCurrent, parse.TagAttrsCurrent)
	}
	injectCurrent(t)
	const body = `<screen><checkbox id="c" bind="on" label="x"/><list id="l" single="true"><item><text>a</text></item></list></screen></tui>`
	for _, v := range []string{"1", "2", "3"} {
		doc := parse.Parse([]byte(`<tui version="`+v+`">`+body), "t.tui")
		var got []string
		for _, d := range doc.Diags {
			got = append(got, d.Code+" "+d.Msg)
		}
		want := []string{
			"V001 tag <checkbox> (requires a <tui> without version)",
			`V002 attribute "single" on <list> (requires a <tui> without version)`,
		}
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Errorf("version=%s:\n%s\nwant\n%s", v, strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
	}
	doc := parse.Parse([]byte(`<tui>`+body), "t.tui")
	for _, d := range doc.Diags {
		if d.Code == "V001" || d.Code == "V002" {
			t.Errorf("current language: %s %s", d.Code, d.Msg)
		}
	}
}

// inject04b stands in for the filled 0.4b tables (F1, F2, F8, F9): the
// composition tags, checkbox and select, and their attributes, restored
// when the test ends. Tests that call it never run in parallel.
func inject04b(t *testing.T) {
	t.Helper()
	kinds, attrs := ir.KindsCurrent, parse.TagAttrsCurrent
	t.Cleanup(func() { ir.KindsCurrent, parse.TagAttrsCurrent = kinds, attrs })
	ir.KindsCurrent = []string{"include", "fragment", "template", "use", "checkbox", "select"}
	parse.TagAttrsCurrent = map[string]map[string]bool{
		"include":  {"src": true},
		"template": {"name": true},
		"use":      {"template": true, "with": true},
		"checkbox": {"id": true, "bind": true, "label": true},
		"select":   {"id": true, "bind": true, "each": true, "key": true},
		"list":     {"single": true},
	}
}

// SPEC v0.4b §5.1 ("keeps its 0.4.1 meaning, diagnostics, and dumps"): a
// misspelling is not a 0.4b item, so a pinned document gets 0.4.1's V001
// or V002 for it byte for byte, even when the name is close to a 0.4b tag
// or a 0.4b attribute shares it. The texts are tuimark 0.4.1's. The
// current language suggests its own tags, and treats an attribute another
// tag takes (with, on <use>) as misplaced, not misspelled, as it does
// for every attribute.
func TestPinnedSuggestionsAsIn041(t *testing.T) {
	inject04b(t)
	const body = `<screen><selec id="s"/><checkbx id="c"/><usee id="u"/><box with="x"/></screen></tui>`
	diags := func(src string) string {
		var got []string
		for _, d := range parse.Parse([]byte(src), "t.tui").Diags {
			got = append(got, d.Code+" "+d.Msg)
		}
		return strings.Join(got, "\n")
	}
	pinned := strings.Join([]string{
		"V001 unknown tag <selec>",
		"V001 unknown tag <checkbx>",
		"V001 unknown tag <usee>",
		`V002 attribute "with" is not allowed on <box>; did you mean "width"?`,
	}, "\n")
	for _, v := range []string{"1", "2", "3"} {
		if got := diags(`<tui version="` + v + `">` + body); got != pinned {
			t.Errorf("version=%s:\n%s\nwant (0.4.1)\n%s", v, got, pinned)
		}
	}
	current := strings.Join([]string{
		"V001 unknown tag <selec>; did you mean <select>?",
		"V001 unknown tag <checkbx>; did you mean <checkbox>?",
		"V001 unknown tag <usee>; did you mean <use>?",
		`V002 attribute "with" is not allowed on <box>`,
	}, "\n")
	if got := diags(`<tui>` + body); got != current {
		t.Errorf("current language:\n%s\nwant\n%s", got, current)
	}
}

// keymapK is SPEC v0.4b test 209's document K; VERSION is replaced by the
// <tui> attributes of each case.
const keymapK = `<tui VERSION>
  <screen id="main">
    <input id="q" bind="q"/>
    <list id="l"><item><text>a</text></item></list>
  </screen>
  <keymap>
    <bind keys="ctrl+i" action="a"/>
    <bind keys="ctrl+j,x" action="b"/>
    <bind keys="ctrl+m,ctrl+i" action="c"/>
  </keymap>
</tui>`

// SPEC v0.4b §8.1 (decisions 82, 83): in the current language each
// ctrl+i, ctrl+j, ctrl+m keymap token is V003 at its <bind>, once per
// token, with no hint; the row and its other tokens stay. A pinned
// document reports nothing.
func TestNeverMatchKeysInKeymap(t *testing.T) {
	doc := parse.Parse([]byte(strings.Replace(keymapK, "VERSION", "", 1)), "K.tui")
	want := []string{
		`7:5 key "ctrl+i" never reaches a keymap: it arrives as tab (write tab)`,
		`8:5 key "ctrl+j" never reaches a keymap: it arrives as enter (write enter)`,
		`9:5 key "ctrl+m" never reaches a keymap: it arrives as enter (write enter)`,
		`9:5 key "ctrl+i" never reaches a keymap: it arrives as tab (write tab)`,
	}
	var got []string
	for _, d := range doc.Diags {
		if d.Code != "V003" || d.Severity != ir.Error {
			t.Errorf("unexpected %s %s %s", d.Severity, d.Code, d.Msg)
			continue
		}
		got = append(got, itoa(d.Line)+":"+itoa(d.Col)+" "+d.Msg)
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("current language:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if len(doc.Keymap) != 3 || strings.Join(doc.Keymap[1].Keys, ",") != "ctrl+j,x" {
		t.Errorf("the rows and their tokens are kept: %+v", doc.Keymap)
	}
	for _, v := range []string{"1", "2", "3"} {
		doc := parse.Parse([]byte(strings.Replace(keymapK, "VERSION", `version="`+v+`"`, 1)), "K.tui")
		if len(doc.Diags) != 0 {
			t.Errorf("version=%s: %v", v, doc.Diags)
		}
	}
	// Implementer decision: a token written twice in one keys list is
	// reported twice, like any other bad token of the list.
	doc = parse.Parse([]byte(`<tui><screen/><keymap><bind keys="ctrl+i,ctrl+i" action="a"/></keymap></tui>`), "t.tui")
	if len(doc.Diags) != 2 {
		t.Errorf("ctrl+i twice: %v", doc.Diags)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}
