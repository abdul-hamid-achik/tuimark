package parse_test

import (
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// 55. SPEC v0.2b §6.10.1, §6.11, §6.12, and §14: the structure of tabs,
// tab, sparkline, and hints. V016 for a tab outside a tabs, a tabs child
// other than tab, text in a tabs, and children or text in a sparkline or
// a hints; V017 for a tabs without a tab; V018 for a mark of 3 columns;
// V012 for a tabs or a tab without id; V003 for a tab without a non-empty
// label, for {path} in label, short, or mark, and for a sparkline
// without bind; B005 for a tab focus= that names no node or a node
// outside the tab. Inside a list item or a container template, a tabs
// and its tabs are V001 as a whole, without V012.
func TestTabsSparklineHintsStructure(t *testing.T) {
	for _, c := range []struct {
		name, body, codes string
	}{
		{"complete", `<tabs id="n" bind="v" mark="▸" gap="2" on:select="s"><tab id="a" label="1 a" short="1" focus="#i"><input id="i"/></tab><tab id="b" label="b"><text>b</text></tab></tabs>`, ""},
		{"whitespace in a tabs", "<tabs id=\"n\">\n  <tab id=\"a\" label=\"a\"/>\n</tabs>", ""},
		{"tab outside a tabs", `<tab id="a" label="a"/>`, "V016"},
		{"tab in a box", `<box><tab id="a" label="a"/></box>`, "V016"},
		{"text child of a tabs", `<tabs id="n"><tab id="a" label="a"/><text>x</text></tabs>`, "V016"},
		{"item child of a tabs", `<tabs id="n"><tab id="a" label="a"/><item/></tabs>`, "V016"},
		{"character data in a tabs", `<tabs id="n">hello<tab id="a" label="a"/></tabs>`, "V016"},
		{"no tab", `<tabs id="n"></tabs>`, "V017"},
		{"only a text", `<tabs id="n"><text>x</text></tabs>`, "V017 V016"},
		{"mark of 3 columns", `<tabs id="n" mark="abc"><tab id="a" label="a"/></tabs>`, "V018"},
		{"wide mark", `<tabs id="n" mark="微"><tab id="a" label="a"/></tabs>`, ""},
		{"tabs without id", `<tabs><tab id="a" label="a"/></tabs>`, "V012"},
		{"focusable tabs without id", `<tabs focusable="true"><tab id="a" label="a"/></tabs>`, "V012"},
		{"tab without id", `<tabs id="n"><tab label="a"/></tabs>`, "V012"},
		{"tab without label", `<tabs id="n"><tab id="a"/></tabs>`, "V003"},
		{"tab with an empty label", `<tabs id="n"><tab id="a" label=""/></tabs>`, "V003"},
		{"path in label", `<tabs id="n"><tab id="a" label="{x}"/></tabs>`, "V003"},
		{"path in short", `<tabs id="n"><tab id="a" label="a" short="{x}"/></tabs>`, "V003"},
		{"control character in a label", "<tabs id=\"n\"><tab id=\"a\" label=\"a\x7f\"/></tabs>", "V007"},
		{"tab focus to a missing id", `<tabs id="n"><tab id="a" label="a" focus="#nope"/></tabs>`, "B005"},
		{"tab focus outside the tab", `<tabs id="n"><tab id="a" label="a" focus="#i"/><tab id="b" label="b"><input id="i"/></tab></tabs>`, "B005"},
		{"tab focus into a list item", `<tabs id="n"><tab id="a" label="a" focus="#r"><list id="l"><item><text id="r">r</text></item></list></tab></tabs>`, "B005"},
		{"tabs in a list item", `<list id="l" each="ys as y" key="y"><item><tabs><tab label="a"/></tabs></item></list>`, "V001 V001"},
		{"sparkline", `<sparkline bind="h" min="-5" max="0.25"/>`, ""},
		{"sparkline without bind", `<sparkline/>`, "V003"},
		{"sparkline with a child", `<sparkline bind="h"><text>x</text></sparkline>`, "V016"},
		{"sparkline with text", `<sparkline bind="h">x</sparkline>`, "V016"},
		{"hints", `<hints id="h" scope="all"/>`, ""},
		{"hints with a child", `<hints><text>x</text></hints>`, "V016"},
		{"hints with text", `<hints>x</hints>`, "V016"},
	} {
		t.Run(c.name, func(t *testing.T) {
			doc := parse.Parse([]byte(coreV2(c.body)), "")
			var codes []string
			for _, d := range doc.Diags {
				codes = append(codes, d.Code)
			}
			if got := strings.Join(codes, " "); got != c.codes {
				t.Errorf("codes %q, want %q (%v)", got, c.codes, doc.Diags)
			}
		})
	}
}

// 57. label and keycap on <bind> are literal text: a {path} is V003; they
// are carried on the keymap row.
func TestBindLabelAndKeycap(t *testing.T) {
	doc := parse.Parse([]byte(`<tui version="2"><keymap><bind keys="ctrl+a" action="all" label="all" keycap="^A"/><bind keys="q" action="quit"/></keymap><screen/></tui>`), "")
	if len(doc.Diags) != 0 {
		t.Fatal(doc.Diags)
	}
	k := doc.Keymap[0]
	if k.Label != "all" || !k.HasLabel || k.Keycap != "^A" || !k.HasKeycap || doc.Keymap[1].HasLabel {
		t.Errorf("rows %+v", doc.Keymap)
	}
	for _, bad := range []string{`label="{x}"`, `keycap="{k}"`} {
		d := parse.Parse([]byte(`<tui version="2"><keymap><bind keys="q" action="quit" `+bad+`/></keymap><screen/></tui>`), "")
		if len(d.Diags) != 1 || d.Diags[0].Code != "V003" {
			t.Errorf("%s: %v", bad, d.Diags)
		}
	}
}
