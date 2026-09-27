package parse_test

import (
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// SPEC v0.2b §6.8, §6.14, §8.4, §14: the static checks of the ergonomics
// phase — container each templates, multi-select, and the built-in
// actions.

// 49. What a container each template may hold (§6.8 item 8), the alias
// rule (item 7), and that key without each is ignored (item 9).
func TestContainerEachTemplateChecks(t *testing.T) {
	for _, c := range []struct {
		name, body, codes string
	}{
		{"plain template", `<col each="xs as x" key="x.id"><row class:hot="x.hot"><text>{x.name}</text><progress bind="x.pct"/></row></col>`, ""},
		{"key without each is ignored", `<box key="x.id"><text>x</text></box>`, ""},
		{"input", `<col each="xs as x"><input id="i"/></col>`, "V001 V004"},
		{"input without id", `<col each="xs as x"><input/></col>`, "V001"},
		{"button deep", `<row each="xs as x"><box><box><button label="b"/></box></box></row>`, "V001"},
		{"list", `<box each="xs as x"><list id="l"><item><text>a</text></item></list></box>`, "V001 V004"},
		{"table", `<box each="xs as x"><table each="ys as y"><column>{y}</column></table></box>`, "V001"},
		{"tabs and its tab", `<box each="xs as x"><tabs><tab label="a"/></tabs></box>`, "V001 V001"},
		{"focusable", `<col each="xs as x"><text focusable="true">{x}</text></col>`, "V002"},
		{"on:click deep", `<col each="xs as x"><row><text on:click="go">{x}</text></row></col>`, "V002"},
		{"on:focus", `<col each="xs as x"><box on:focus="f"/></col>`, "V002"},
		{"id", `<col each="xs as x"><text id="name">{x}</text></col>`, "V004"},
		{"the container itself keeps its id and handlers", `<col id="c" each="xs as x" focusable="true" on:click="go"><text>{x}</text></col>`, ""},
		{"nested each resolves both aliases", `<col each="xs as x"><row each="x.ys as y"><text>{x.a}{y}</text></row></col>`, ""},
		{"nested alias repeats", `<col each="xs as x"><row each="x.ys as x"><text>{x}</text></row></col>`, "V011"},
		{"alias repeats a list alias", `<list id="l" each="xs as x" key="x"><item><box each="x.ys as x"><text>{x}</text></box></item></list>`, "V011"},
		{"container in a list item", `<list id="l" each="xs as x" key="x"><item><box each="x.ys as y"><text>{y}</text></box></item></list>`, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			doc := parse.Parse([]byte(coreV2(c.body)), "")
			if got := coreCodes(doc); got != c.codes {
				t.Errorf("codes %q, want %q: %v", got, c.codes, doc.Diags)
			}
		})
	}
}

// The template rules are version="2" rules: in a version="1" document
// each= on a container is V002 with the version hint and nothing else.
func TestContainerEachInVersionOne(t *testing.T) {
	src := "<tui version=\"1\">\n<screen>\n" + `<col each="xs as x"><input id="i"/><text id="t" on:click="go">{x}</text></col>` + "\n</screen>\n</tui>"
	doc := parse.Parse([]byte(src), "")
	if got := coreCodes(doc); got != "V002" {
		t.Fatalf("codes %q: %v", got, doc.Diags)
	}
	if !strings.HasSuffix(doc.Diags[0].Msg, ir.VersionHint) {
		t.Errorf("no version hint: %v", doc.Diags[0])
	}
}

// 64. V018 (§6.14): checked needs each and key; a static list cannot have
// it; mark is 1 or 2 columns and needs checked; on:change on a list needs
// checked.
func TestMultiSelectV018(t *testing.T) {
	for _, c := range []struct {
		name, body, codes string
	}{
		{"complete", `<list id="l" each="xs as x" key="x" checked="m" mark="✓" on:change="c"><item><text>{x}</text></item></list>`, ""},
		{"wide mark", `<list id="l" each="xs as x" key="x" checked="m" mark="微"><item><text>{x}</text></item></list>`, ""},
		{"checked without key", `<list id="l" each="xs as x" checked="m"><item><text>{x}</text></item></list>`, "V018 B006"},
		{"checked on static items", `<list id="l" checked="m"><item><text>a</text></item></list>`, "V018"},
		{"mark of 3 columns", `<list id="l" each="xs as x" key="x" checked="m" mark="-->"><item><text>{x}</text></item></list>`, "V018"},
		{"empty mark", `<list id="l" each="xs as x" key="x" checked="m" mark=""><item><text>{x}</text></item></list>`, "V018"},
		{"mark without checked", `<list id="l" each="xs as x" key="x" mark="✓"><item><text>{x}</text></item></list>`, "V018"},
		{"on:change without checked", `<list id="l" each="xs as x" key="x" on:change="c"><item><text>{x}</text></item></list>`, "V018"},
		{"table mark without checked", `<table id="t" each="xs as x" key="x" mark="▸"><column>{x}</column></table>`, "V018"},
		{"tabs mark of 3 columns", `<tabs id="n" mark="abc"><tab id="a" label="a"/></tabs>`, "V018"},
	} {
		t.Run(c.name, func(t *testing.T) {
			doc := parse.Parse([]byte(coreV2(c.body)), "")
			if got := coreCodes(doc); got != c.codes {
				t.Errorf("codes %q, want %q: %v", got, c.codes, doc.Diags)
			}
		})
	}
}

func keymapV2(rows string) string {
	return `<tui version="2"><keymap>` + rows + `</keymap><screen id="main">
<text id="t">x</text><button id="b" label="b"/><input id="i"/><progress id="p" value="5"/>
<list id="l" each="xs as x" key="x"><item><text>{x}</text></item></list>
<list id="m" each="xs as x" key="x" checked="marks"><item><text>{x}</text></item></list>
<scroll id="s"><text>y</text></scroll><box id="bx"/>
<tabs id="n"><tab id="ta" label="a"/></tabs>
</screen><screen id="other"/></tui>`
}

// 62. The static checks of §8.4: switch-to without to is V003; B007 for a
// target known without data to be incompatible, from to= or from a when
// that is exactly #id:focus; a hyphenated built-in in on:* is V003.
func TestBuiltinStaticChecks(t *testing.T) {
	for _, c := range []struct {
		name, rows, codes string
	}{
		{"move-next text", `<bind keys="j" action="move-next" to="#t"/>`, "B007"},
		{"move-prev button by when", `<bind keys="k" action="move-prev" when="#b:focus"/>`, "B007"},
		{"move-first input", `<bind keys="g" action="move-first" to="#i"/>`, "B007"},
		{"move-last progress", `<bind keys="e" action="move-last" to="#p"/>`, "B007"},
		{"move-next tab", `<bind keys="j" action="move-next" to="#ta"/>`, "B007"},
		{"move-page-down tabs", `<bind keys="d" action="move-page-down" to="#n"/>`, "B007"},
		{"move-next tabs", `<bind keys="j" action="move-next" to="#n"/>`, ""},
		{"move-next list", `<bind keys="j" action="move-next" to="#l"/>`, ""},
		{"move-next scroll", `<bind keys="j" action="move-next" to="#s"/>`, ""},
		{"move-next box (may scroll by CSS)", `<bind keys="j" action="move-next" to="#bx"/>`, ""},
		{"check-all list without checked", `<bind keys="ctrl+a" action="check-all" to="#l"/>`, "B007"},
		{"check-toggle list by when", `<bind keys="space" action="check-toggle" when="#l:focus"/>`, "B007"},
		{"check-none text", `<bind keys="ctrl+d" action="check-none" to="#t"/>`, "B007"},
		{"check-all list with checked", `<bind keys="ctrl+a" action="check-all" to="#m"/>`, ""},
		{"switch-to without to", `<bind keys="2" action="switch-to"/>`, "V003"},
		{"switch-to list", `<bind keys="2" action="switch-to" to="#l"/>`, "B007"},
		{"switch-to tab", `<bind keys="2" action="switch-to" to="#ta"/>`, ""},
		{"switch-to screen", `<bind keys="2" action="switch-to" to="#other"/>`, ""},
		{"switch-to missing", `<bind keys="2" action="switch-to" to="#nope"/>`, "B005"},
		{"when is not exactly #id:focus", `<bind keys="j" action="move-next" when="#t"/>`, ""},
		{"when with a type", `<bind keys="j" action="move-next" when="text#t:focus"/>`, ""},
		{"without target", `<bind keys="j" action="move-next"/>`, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			doc := parse.Parse([]byte(keymapV2(c.rows)), "")
			if got := coreCodes(doc); got != c.codes {
				t.Errorf("codes %q, want %q: %v", got, c.codes, doc.Diags)
			}
			for _, d := range doc.Diags {
				if d.Code == "B007" && d.Severity != ir.Error {
					t.Errorf("B007 is an error: %v", d)
				}
			}
		})
	}
	doc := parse.Parse([]byte(coreV2(`<button id="b" label="b" on:click="move-next"/>`)), "")
	if got := coreCodes(doc); got != "V003" {
		t.Errorf("on:click=\"move-next\": %q %v", got, doc.Diags)
	}
}
