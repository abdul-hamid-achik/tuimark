package parse_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// SPEC v0.2b §5.1, §6.13, §6.15, §13.1: what the document builder accepts
// in a version="2" document, and IR 0.2.

func coreV2(body string) string {
	return "<tui version=\"2\">\n<screen>\n" + body + "\n</screen>\n</tui>"
}

// The version is read before anything else, whatever the attribute order.
func TestVersionTwoDocument(t *testing.T) {
	doc := parse.Parse([]byte(`<tui theme="auto" mouse="!quiet" version="2"><screen/></tui>`), "")
	if len(doc.Diags) != 0 || !doc.V2 || doc.Theme != "auto" || doc.Mouse != "!quiet" {
		t.Fatalf("v2 %v theme %q mouse %q diags %v", doc.V2, doc.Theme, doc.Mouse, doc.Diags)
	}
	for _, v := range []string{"1", "3", ""} {
		d := parse.Parse([]byte(`<tui version="`+v+`"><screen/></tui>`), "")
		if d.V2 {
			t.Errorf("version=%q read as version=2", v)
		}
	}
	for _, c := range []struct{ src, code string }{
		{`<tui version="2" mouse="yes please"><screen/></tui>`, "V003"},
		{`<tui version="2" theme="sepia"><screen/></tui>`, "V003"},
		{`<tui version="1" theme="auto"><screen/></tui>`, "V003"},
	} {
		d := parse.Parse([]byte(c.src), "")
		if coreCodes(d) != c.code {
			t.Errorf("%s: %s", c.src, coreCodes(d))
		}
	}
}

// §6.15: the new tags take exactly their rows; a table's <item> only
// class and class:NAME; <item> may sit in a table.
func TestVersionTwoAttributeRows(t *testing.T) {
	for _, c := range []struct {
		name, body, codes string
	}{
		{"table row", `<table id="t" each="xs as x" key="x" bind="c" checked="m" mark="▸" placeholder="none" on:select="s" on:change="c" class:hot="h"><item class="r" class:dim="d"/><column id="c" class="n" title="N" width="5" style="bold: true" on:click="k">{x}</column></table>`, ""},
		{"table item extra attr", `<table id="t" each="xs as x"><item id="i"/><column>{x}</column></table>`, "V002"},
		{"column hidden", `<table id="t" each="xs as x"><column hidden="true">{x}</column></table>`, "V002"},
		{"tabs row", `<tabs id="n" bind="v" mark="▸" on:select="s" focusable="true"><tab id="a" label="1 a" short="1" focus="#i" gap="1" pad="1" border="single" if="x" hidden="false" disabled="false"><input id="i"/></tab></tabs>`, ""},
		{"tab width", `<tabs id="n"><tab id="a" label="a" width="5"/></tabs>`, "V002"},
		{"tab label path", `<tabs id="n"><tab id="a" label="{x}"/></tabs>`, "V003"},
		{"tabs mark path", `<tabs id="n" mark="{m}"><tab id="a" label="a"/></tabs>`, "V003"},
		{"sparkline row", `<sparkline bind="h" min="-5" max="0.25"/>`, ""},
		{"sparkline bad min", `<sparkline bind="h" min="x"/>`, "V003"},
		{"sparkline exponent max", `<sparkline bind="h" max="1e3"/>`, "V003"},
		{"hints scope all", `<hints scope="all"/>`, ""},
		{"hints scope bad", `<hints scope="behind"/>`, "V003"},
		{"list checked", `<list id="l" each="xs as x" key="x" checked="marks" mark="✓" on:change="c"><item><text>{x}</text></item></list>`, ""},
		{"list checked not a path", `<list id="l" each="xs as x" key="x" checked="a b"><item><text>{x}</text></item></list>`, "V003"},
		{"container each", `<col each="xs as x" key="x.id"><text>{x.name}</text></col>`, ""},
		{"container each malformed", `<box each="xs"><text>x</text></box>`, "V011"},
		{"table inside a list item", `<list id="l" each="xs as x" key="x"><item><table id="t" each="ys as y"><column>{y}</column></table></item></list>`, "V001"},
		{"item outside list and table", `<box><item/></box>`, "V001"},
		{"class guard everywhere", `<col class:a="x"><row class:b="!y"><box class:c="z"><text class:d="p.q">x</text></box></row></col>`, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			doc := parse.Parse([]byte(coreV2(c.body)), "")
			if got := coreCodes(doc); got != c.codes {
				t.Errorf("codes %q, want %q: %v", got, c.codes, doc.Diags)
			}
			for _, d := range doc.Diags {
				if strings.HasSuffix(d.Msg, ir.VersionHint) {
					t.Errorf("a version=\"2\" document got the version hint: %s", d)
				}
			}
		})
	}
}

// §6.13: class:NAME guards are recorded in attribute order; the node's
// class list stays the class attribute's.
func TestClassGuardsRecorded(t *testing.T) {
	doc := parse.Parse([]byte(coreV2(`<text class="a b" class:live="s.live" class:stale="!s.fresh">x</text>`)), "")
	if len(doc.Diags) != 0 {
		t.Fatal(doc.Diags)
	}
	n := coreNode(doc.Root, "text")
	if strings.Join(n.Classes, " ") != "a b" || len(n.ClassGuards) != 2 ||
		n.ClassGuards[0] != (ir.ClassGuard{Name: "live", Guard: "s.live"}) || n.ClassGuards[1] != (ir.ClassGuard{Name: "stale", Guard: "!s.fresh"}) {
		t.Errorf("classes %v guards %v", n.Classes, n.ClassGuards)
	}
}

// §6.12, §8.4: label and keycap on a version="2" keymap row (literal
// text), and the hyphenated built-in actions.
func TestVersionTwoKeymap(t *testing.T) {
	src := `<tui version="2"><keymap>
<bind keys="q" action="quit" label="quit"/>
<bind keys="1" action="switch-to" to="#s" keycap="1-9" label=""/>
<bind keys="j" action="move-next"/>
<bind keys="x" action="go" when="#s:focus-within"/>
</keymap><screen id="s"/></tui>`
	doc := parse.Parse([]byte(src), "")
	if len(doc.Diags) != 0 {
		t.Fatal(doc.Diags)
	}
	k := doc.Keymap
	if !k[0].HasLabel || k[0].Label != "quit" || k[0].HasKeycap || !k[1].HasLabel || k[1].Label != "" || k[1].Keycap != "1-9" || k[2].HasLabel {
		t.Errorf("keymap %+v", k)
	}
	for _, c := range []struct{ row, code string }{
		{`<bind keys="q" action="quit" label="{x}"/>`, "V003"},
		{`<bind keys="q" action="quit" keycap="{x}"/>`, "V003"},
		{`<bind keys="q" action="quit" keycap="a&#9;b"/>`, "V005"},
		{`<bind keys="q" action="move-sideways"/>`, "V003"},
	} {
		d := parse.Parse([]byte(`<tui version="2"><keymap>`+c.row+`</keymap><screen id="s"/></tui>`), "")
		if got := coreCodes(d); got != c.code {
			t.Errorf("%s: %q", c.row, got)
		}
	}
	// A built-in in an on:* attribute is V003 (an action is a plain name).
	if d := parse.Parse([]byte(coreV2(`<button id="b" on:click="move-next">go</button>`)), ""); coreCodes(d) != "V003" {
		t.Errorf("on:click=move-next: %v", d.Diags)
	}
}

// §13.1: IR 0.2 for version="2" documents, 0.1 otherwise; mouse, label,
// keycap, class:NAME in attrs, the new kinds, and a column's text.
func TestBuildIRVersionTwo(t *testing.T) {
	src := `<tui version="2" mouse="m"><keymap><bind keys="q" action="quit" label="quit" keycap="Q"/><bind keys="x" action="go"/></keymap>
<screen id="s"><table id="t" each="xs as x" key="x"><item class:hot="x.hot"/><column title="N">{x.n}</column></table><hints id="h"/></screen></tui>`
	doc := parse.Parse([]byte(src), "")
	if len(doc.Diags) != 0 {
		t.Fatal(doc.Diags)
	}
	b, err := json.Marshal(parse.BuildIR(doc, map[string]string{}))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{`"version":"0.2"`, `"mouse":"m"`, `{"keys":"q","action":"quit","label":"quit","keycap":"Q"}`, `{"keys":"x","action":"go"}`, `"kind":"table"`, `"attrs":{"class:hot":"x.hot"}`, `"kind":"column","classes":[],"attrs":{"title":"N"}`, `"text":"{x.n}"`, `"kind":"hints"`} {
		if !strings.Contains(s, want) {
			t.Errorf("IR lacks %s:\n%s", want, s)
		}
	}
	v1 := parse.Parse([]byte(`<tui version="1"><screen id="s"/></tui>`), "")
	if b, _ := json.Marshal(parse.BuildIR(v1, nil)); !strings.Contains(string(b), `"version":"0.1"`) || strings.Contains(string(b), "mouse") {
		t.Errorf("v1 IR %s", b)
	}
}
