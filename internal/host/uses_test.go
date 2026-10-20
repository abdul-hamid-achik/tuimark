package host

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// SPEC v0.4b §6.18.3-§6.18.6 (phase F3): the binding semantics of
// template copies in the host.

// useApp loads src as app.tui and binds data, JSON text, at the root.
func useApp(t *testing.T, src, data string) *App {
	t.Helper()
	dir := writeTree(t, map[string]string{"app.tui": src})
	a, err := Load(filepath.Join(dir, "app.tui"))
	if err != nil {
		t.Fatal(err)
	}
	bindJSON(t, a, data)
	return a
}

// useDiags is a's diagnostics as "SEV CODE L:C PATH MSG".
func useDiags(a *App) []string {
	out := []string{}
	for _, d := range a.Validate() {
		out = append(out, fmt.Sprintf("%s %s %d:%d %s %s", d.Severity, d.Code, d.Line, d.Col, d.Path, d.Msg))
	}
	return out
}

func sameDiags(t *testing.T, what string, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s:\n got %s\nwant %s", what, strings.Join(got, "\n     "), strings.Join(want, "\n     "))
	}
}

// texts is the texts of a's text nodes at 20x6.
func texts(a *App) []string {
	out := []string{}
	for _, n := range a.Dump(20, 6, false).Nodes {
		if n.Tag == "text" {
			out = append(out, n.Text)
		}
	}
	return out
}

// Decision 14: a with path that does not resolve is one B003 at the use
// (an error under --strict), and nothing the copy reads through the alias
// reports anything, a nested use's with, its copy's {path}, bind, and
// flags included. The alias reads as a missing path.
func TestMissingWithPathIsOneB003AtTheUse(t *testing.T) {
	const src = `<tui>
  <template id="field">
    <row>
      <text if="!f.on" class:x="f.x">[{f.label}]</text>
      <input id="in" bind="f.value"/>
    </row>
  </template>
  <template id="address">
    <col>
      <text>{a.city}</text>
      <use id="zip" template="field" with="a.zip as f" if="!a.off"/>
    </col>
  </template>
  <screen id="main">
    <use id="ship" template="address" with="order.ship as a"/>
  </screen>
</tui>`
	a := useApp(t, src, `{"order": {}}`)
	sameDiags(t, "missing", useDiags(a), []string{`warning B003 15:5 /screen/use with path "order.ship" (alias a) is missing`})
	if got := texts(a); !reflect.DeepEqual(got, []string{"", "[]"}) {
		t.Errorf("texts %q", got)
	}
	a.SetStrict(true)
	sameDiags(t, "--strict", useDiags(a), []string{`error B003 15:5 /screen/use with path "order.ship" (alias a) is missing`})
	// The nested use's own with path, through ship's alias, names it as
	// written and says where it went.
	a.SetStrict(false)
	bindJSON(t, a, `{"order": {"ship": {"city": "Nice"}}}`)
	sameDiags(t, "nested", useDiags(a), []string{`warning B003 11:7 /screen/col/use with path "a.zip" (alias f) is missing (order.ship.zip via <use id="ship">)`})
}

// §6.18.5 "Messages": a B001, B002, or B003 about a path of a copy that
// goes through a with alias quotes the path (and the attribute) as
// written, then ` (P via <use id="U">)`; a path through a row alias keeps
// the placement suffix. An each's B001 reports even when the alias's own
// path is missing (§6.18.5 exempts B002 and B003 only), and a flag
// through an alias that stands for something other than an object is
// B002.
func TestPathsThroughAWithAliasNameTheirUse(t *testing.T) {
	const src = `<tui>
  <template id="t">
    <col>
      <box each="f.items as it"><text>{it}</text></box>
      <input id="in" bind="f.value"/>
      <progress bind="f.pct"/>
      <text class:on="f.on" if="f.shown" hidden="f.gone">{f.name}</text>
      <list id="l" each="f.rows as r" key="r.id" bind="f.cur"><item><text>{r.id}</text></item></list>
    </col>
  </template>
  <screen id="main">
    <use id="u" template="t" with="form.a as f"/>
  </screen>
</tui>`
	a := useApp(t, src, `{"form": {"a": {"pct": "half", "shown": true, "rows": [{"id": 1}, {"x": 2}]}}}`)
	sameDiags(t, "an object", useDiags(a), []string{
		`error B001 4:7 /screen/col/box each="f.items as it": path "f.items" is missing (want an array) (form.a.items via <use id="u">)`,
		`warning B003 5:7 /screen/col/input bind path "f.value" is missing (form.a.value via <use id="u">)`,
		`warning B003 6:7 /screen/col/progress progress bind "f.pct" is a string, want a number 0-100 (form.a.pct via <use id="u">)`,
		`warning B003 7:7 /screen/col/text bind path "f.name" is missing (form.a.name via <use id="u">)`,
		`warning B003 8:7 /screen/col/list key="r.id" is missing on item 1, placed by <use id="u"> at app.tui:12:5`,
		`warning B003 8:7 /screen/col/list bind path "f.cur" is missing (form.a.cur via <use id="u">)`,
		`warning B003 8:69 /screen/col/list/item/text bind path "r.id" is missing, placed by <use id="u"> at app.tui:12:5`,
	})
	bindJSON(t, a, `{"form": {"a": {"items": 3, "shown": true, "rows": 5}}}`)
	sameDiags(t, "not arrays", useDiags(a), []string{
		`error B001 4:7 /screen/col/box each="f.items as it": "f.items" is a number, not an array (form.a.items via <use id="u">)`,
		`warning B003 5:7 /screen/col/input bind path "f.value" is missing (form.a.value via <use id="u">)`,
		`warning B003 6:7 /screen/col/progress bind path "f.pct" is missing (form.a.pct via <use id="u">)`,
		`warning B003 7:7 /screen/col/text bind path "f.name" is missing (form.a.name via <use id="u">)`,
		`error B001 8:7 /screen/col/list each="f.rows as r": "f.rows" is a number, not an array (form.a.rows via <use id="u">)`,
		`warning B003 8:7 /screen/col/list bind path "f.cur" is missing (form.a.cur via <use id="u">)`,
	})
	bindJSON(t, a, `{"form": {"a": 7}}`)
	sameDiags(t, "a number", useDiags(a), []string{
		`error B001 4:7 /screen/col/box each="f.items as it": path "f.items" is missing (want an array) (form.a.items via <use id="u">)`,
		`warning B003 5:7 /screen/col/input bind path "f.value" is missing (form.a.value via <use id="u">)`,
		`warning B003 6:7 /screen/col/progress bind path "f.pct" is missing (form.a.pct via <use id="u">)`,
		`warning B002 7:7 /screen/col/text if="f.shown": path "f.shown" is missing (form.a.shown via <use id="u">)`,
		`error B001 8:7 /screen/col/list each="f.rows as r": path "f.rows" is missing (want an array) (form.a.rows via <use id="u">)`,
		`warning B003 8:7 /screen/col/list bind path "f.cur" is missing (form.a.cur via <use id="u">)`,
	})
	bindJSON(t, a, `{}`)
	sameDiags(t, "missing", useDiags(a), []string{
		`error B001 4:7 /screen/col/box each="f.items as it": path "f.items" is missing (want an array) (form.a.items via <use id="u">)`,
		`error B001 8:7 /screen/col/list each="f.rows as r": path "f.rows" is missing (want an array) (form.a.rows via <use id="u">)`,
		`warning B003 12:5 /screen/use with path "form.a" (alias f) is missing`,
	})
}

// A message about a path through a with alias quotes what is written in
// the template (the path, the attribute's name and value, the alias) and
// P cut at 64 bytes on a rune boundary, then "…" (ADR 0034): each copy
// through a with path of its own says them again.
func TestViaMessagesCutWhatTheyQuote(t *testing.T) {
	name := "m" + strings.Repeat("y", 69)
	cut := "m" + strings.Repeat("y", 61) + "…" // 64 bytes with the "f." or "a." before it
	src := `<tui>
  <template id="in">
    <text>x</text>
  </template>
  <template id="t">
    <col>
      <text if="!f.` + name + `" class:` + name + `="f.k">x</text>
      <use template="in" with="f.z as ` + name + `"/>
    </col>
  </template>
  <screen id="main">
    <use id="u" template="t" with="a as f"/>
  </screen>
</tui>`
	a := useApp(t, src, `{"a": 1}`)
	sameDiags(t, "a number", useDiags(a), []string{
		`warning B002 7:7 /screen/col/text if="!f.m` + strings.Repeat("y", 60) + `…": path "f.` + cut + `" is missing (a.` + cut + ` via <use id="u">)`,
		`warning B002 7:7 /screen/col/text class:m` + strings.Repeat("y", 57) + `…="f.k": path "f.k" is missing (a.k via <use id="u">)`,
		`warning B003 8:7 /screen/col/use with path "f.z" (alias m` + strings.Repeat("y", 63) + `…) is missing (a.z via <use id="u">)`,
	})
}

// Decision 100: the if and hidden of a nested use, a disabled, and an
// open read through an enclosing use's alias are optional props while the
// alias stands for an object (null: if false, hidden false), and B002,
// with the via, when it stands for anything else.
func TestOptionalFlagsOfNestedUses(t *testing.T) {
	const src = `<tui>
  <template id="inner">
    <text>in</text>
  </template>
  <template id="outer">
    <col>
      <use template="inner" if="a.show"/>
      <use template="inner" hidden="a.hide"/>
      <button id="b" disabled="a.off" on:click="go">x</button>
    </col>
    <modal id="m" open="a.open"><text>m</text></modal>
  </template>
  <screen id="main">
    <use id="o" template="outer" with="cfg as a"/>
  </screen>
</tui>`
	a := useApp(t, src, `{"cfg": {}}`)
	sameDiags(t, "an object", useDiags(a), []string{})
	d := a.Dump(20, 6, false)
	var got []string
	for _, n := range d.Nodes {
		got = append(got, n.Tag+":"+n.ID+":"+n.Text)
	}
	if want := []string{"screen:main:", "col::", "text::in", "button:o__b:x"}; !reflect.DeepEqual(got, want) || d.Focus == nil || *d.Focus != "o__b" {
		t.Errorf("nodes %q, focus %v", got, d.Focus)
	}
	bindJSON(t, a, `{"cfg": 1}`)
	sameDiags(t, "a number", useDiags(a), []string{
		`warning B002 7:7 /screen/col/use if="a.show": path "a.show" is missing (cfg.show via <use id="o">)`,
		`warning B002 8:7 /screen/col/use hidden="a.hide": path "a.hide" is missing (cfg.hide via <use id="o">)`,
		`warning B002 9:7 /screen/col/button disabled="a.off": path "a.off" is missing (cfg.off via <use id="o">)`,
		`warning B002 11:5 /screen/modal open="a.open": path "a.open" is missing (cfg.open via <use id="o">)`,
	})
}

// Decision 16: one written element at two paths (an id-less use in the
// screen and in a col) reports once, with the first placement's path.
func TestOneDiagnosticPerWrittenElement(t *testing.T) {
	const src = `<tui>
  <template id="t">
    <text>{x.name}</text>
  </template>
  <screen id="main">
    <use template="t" with="b as x"/>
    <col><use template="t" with="b as x"/></col>
  </screen>
</tui>`
	a := useApp(t, src, `{"b": {}}`)
	sameDiags(t, "validate", useDiags(a), []string{`warning B003 3:5 /screen/text bind path "x.name" is missing (b.name via <use template="t">)`})
	if ds := a.Dump(20, 4, false).Errors; len(ds) != 1 || ds[0].Path != "/screen/text" {
		t.Errorf("dump errors %v", ds)
	}
}

// Decision 107: a use is checked once per placement, whatever the number
// of its copy's top-level elements, in the screen as in each list row.
func TestUseCheckedOncePerPlacement(t *testing.T) {
	checks := func(top int, inList bool) uint64 {
		t.Helper()
		body := strings.Repeat("<text>{b.n}</text>", top)
		use := `<use template="t" with="cfg as b" if="show" hidden="hide"/>`
		screen := use
		if inList {
			screen = `<list id="l" each="rows as r" key="r.id"><item><row>` + use + `</row></item></list>`
		}
		a := useApp(t, `<tui><template id="t">`+body+`</template><screen id="main">`+screen+`</screen></tui>`,
			`{"show": true, "hide": false, "cfg": {"n": 1}, "rows": [{"id": "a"}, {"id": "b"}, {"id": "c"}, {"id": "d"}]}`)
		before := a.placeChecks
		a.Dump(40, 10, false)
		return a.placeChecks - before
	}
	for _, inList := range []bool{false, true} {
		one, three := checks(1, inList), checks(3, inList)
		if one == 0 || one != three {
			t.Errorf("in a list %v: %d checks for one element, %d for three", inList, one, three)
		}
	}
}

// A list row's cache reads the with paths of the uses in its template
// (listPlan), so the use's B003 comes and goes with the path, also when
// the copy reads nothing through the alias.
func TestListRowsFollowTheirUsesWithPaths(t *testing.T) {
	const src = `<tui>
  <template id="t">
    <text>static</text>
  </template>
  <screen id="main">
    <list id="l" each="rows as r" key="r.id">
      <item><row><text>{r.id}</text><use template="t" with="cfg.x as c"/></row></item>
    </list>
  </screen>
</tui>`
	a := useApp(t, src, `{"rows": [{"id": "a"}, {"id": "b"}], "cfg": {}}`)
	b003 := []string{`warning B003 7:37 /screen/list/item/row/use with path "cfg.x" (alias c) is missing`}
	sameDiags(t, "missing", useDiags(a), b003)
	if err := a.Set("cfg.x", 1.0); err != nil {
		t.Fatal(err)
	}
	sameDiags(t, "set", useDiags(a), []string{})
	if err := a.Set("cfg", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	sameDiags(t, "removed", useDiags(a), b003)
}
