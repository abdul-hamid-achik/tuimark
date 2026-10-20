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
// the placement suffix. When the alias's own path is missing, the use's
// B003 is the only report, with no B001 for an each through it
// (TestEachThroughAMissingAlias), and a flag through an alias that stands
// for something other than an object is B002.
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
		`warning B003 12:5 /screen/use with path "form.a" (alias f) is missing`,
	})
}

// The owner's ruling on the F3 erratum (decision 14): when a with alias's
// own path is missing, the use's B003 (an error under --strict) is the
// only report, and an each through the alias (on a container, a list, a
// table, or a select) has no rows and no B001. When the alias's path is
// present and an each through it is not an array, B001 reports, in the
// via format.
func TestEachThroughAMissingAlias(t *testing.T) {
	const src = `<tui>
  <template id="t">
    <col>
      <box each="f as it"><text>{it}</text></box>
      <box each="f.items as it"><text>{it}</text></box>
      <list id="l" each="f.rows as r" key="r.id"><item><text>{r.id}</text></item></list>
      <table id="tb" each="f.rows as r" key="r.id"><column>{r.id}</column></table>
      <select id="s" each="f.rows as o" key="o.id"/>
    </col>
  </template>
  <screen id="main">
    <use id="u" template="t" with="form.a as f"/>
  </screen>
</tui>`
	a := useApp(t, src, `{}`)
	sameDiags(t, "missing", useDiags(a), []string{
		`warning B003 12:5 /screen/use with path "form.a" (alias f) is missing`,
	})
	if got := texts(a); !reflect.DeepEqual(got, []string{}) {
		t.Errorf("texts %q, want no rows", got)
	}
	a.SetStrict(true)
	sameDiags(t, "missing, --strict", useDiags(a), []string{
		`error B003 12:5 /screen/use with path "form.a" (alias f) is missing`,
	})
	a.SetStrict(false)
	bindJSON(t, a, `{"form": {"a": 5}}`)
	sameDiags(t, "not an array", useDiags(a), []string{
		`error B001 4:7 /screen/col/box each="f as it": "f" is a number, not an array (form.a via <use id="u">)`,
		`error B001 5:7 /screen/col/box each="f.items as it": path "f.items" is missing (want an array) (form.a.items via <use id="u">)`,
		`error B001 6:7 /screen/col/list each="f.rows as r": path "f.rows" is missing (want an array) (form.a.rows via <use id="u">)`,
		`error B001 7:7 /screen/col/table each="f.rows as r": path "f.rows" is missing (want an array) (form.a.rows via <use id="u">)`,
		`error B001 8:7 /screen/col/select each="f.rows as o": path "f.rows" is missing (want an array) (form.a.rows via <use id="u">)`,
	})
	bindJSON(t, a, `{"form": {"a": {"items": "x", "rows": 3}}}`)
	sameDiags(t, "members not arrays", useDiags(a), []string{
		`error B001 4:7 /screen/col/box each="f as it": "f" is an object, not an array (form.a via <use id="u">)`,
		`error B001 5:7 /screen/col/box each="f.items as it": "f.items" is a string, not an array (form.a.items via <use id="u">)`,
		`error B001 6:7 /screen/col/list each="f.rows as r": "f.rows" is a number, not an array (form.a.rows via <use id="u">)`,
		`error B001 7:7 /screen/col/table each="f.rows as r": "f.rows" is a number, not an array (form.a.rows via <use id="u">)`,
		`error B001 8:7 /screen/col/select each="f.rows as o": "f.rows" is a number, not an array (form.a.rows via <use id="u">)`,
	})

	// The alias's path read in a row's scope: a row without it reports
	// the use's B003 alone; a row with it and no array there, B001.
	const rows = `<tui>
  <template id="t"><box each="f.items as it"><text>{it}</text></box></template>
  <screen id="main">
    <list id="l" each="rows as r" key="r.id"><item><use template="t" with="r.x as f"/></item></list>
  </screen>
</tui>`
	ra := useApp(t, rows, `{"rows": [{"id": 1}, {"id": 2, "x": {"items": 4}}, {"id": 3, "x": {}}]}`)
	sameDiags(t, "rows", useDiags(ra), []string{
		`error B001 2:20 /screen/list/item/box each="f.items as it": "f.items" is a number, not an array (r.x.items via <use template="t"> at app.tui:4:52)`,
		`error B001 2:20 /screen/list/item/box each="f.items as it": path "f.items" is missing (want an array) (r.x.items via <use template="t"> at app.tui:4:52)`,
		`warning B003 4:52 /screen/list/item/use with path "r.x" (alias f) is missing`,
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

// The columns and the row template of a table stand in the table's scope,
// but a with path through its row alias is read on every row (the F3
// review): `with="r as c"` on a use that places a column is no B003, and
// `with="r.sub as c"` is one B003 at the use, once, for the rows that lack
// it. It holds for a table in a copy too, whose row alias is renamed.
func TestTableUsesReadRowAliasPerRow(t *testing.T) {
	const data = `{"rows": [{"id": "a", "tag": "x", "sub": {"tag": "s"}}, {"id": "b", "tag": "y"}]}`
	table := func(with string) string {
		return `<tui>
  <template id="col"><column title="T">{c.tag}</column></template>
  <template id="row"><item class:hot="c.tag"/></template>
  <screen id="main">
    <table id="t" each="rows as r" key="r.id">
      <use template="row" with="` + with + `"/>
      <column title="N">{r.id}</column>
      <use template="col" with="` + with + `"/>
    </table>
  </screen>
</tui>`
	}
	a := useApp(t, table("r as c"), data)
	a.SetStrict(true)
	sameDiags(t, "r as c", useDiags(a), []string{})
	if got := texts(a); !reflect.DeepEqual(got, []string{"a", "x", "b", "y"}) {
		t.Errorf("texts %q", got)
	}
	a = useApp(t, table("r.sub as c"), data)
	a.SetStrict(true)
	sameDiags(t, "r.sub as c", useDiags(a), []string{
		`error B003 6:7 /screen/table/use with path "r.sub" (alias c) is missing`,
		`error B003 8:7 /screen/table/use with path "r.sub" (alias c) is missing`,
	})
	if got := texts(a); !reflect.DeepEqual(got, []string{"a", "s", "b", ""}) {
		t.Errorf("texts %q", got)
	}
	inCopy := `<tui>
  <template id="col"><column title="T">{c.tag}</column></template>
  <template id="tbl">
    <table id="t" each="f.rows as r" key="r.id">
      <column title="N">{r.id}</column>
      <use template="col" with="r as c"/>
    </table>
  </template>
  <screen id="main">
    <use id="u" template="tbl" with="d as f"/>
  </screen>
</tui>`
	a = useApp(t, inCopy, `{"d": `+data+`}`)
	a.SetStrict(true)
	sameDiags(t, "in a copy", useDiags(a), []string{})
	if got := texts(a); !reflect.DeepEqual(got, []string{"a", "x", "b", "y"}) {
		t.Errorf("texts in a copy %q", got)
	}
}

// A use whose copy places nothing (an empty template, or one whose body is
// another such use) is still checked where it stands (the F3 review): its
// if (B002), then, unless it prunes, its with paths (B003), in the scope
// of the element, row, or tab it stands in; an outer use that prunes keeps
// the inner one from being checked; and a list row's cache follows its
// paths.
func TestEmptyCopiesAreChecked(t *testing.T) {
	const src = `<tui>
  <template id="e"></template>
  <template id="outer"><use template="e" with="g.y as h"/></template>
  <screen id="main">
    <use template="e" with="nope as f" if="gone"/>
    <use template="outer" with="cfg as g" if="on"/>
    <box each="aa as r"><use template="e" with="r.x as f"/></box>
    <box each="bb as r"><use template="e" with="r.x as f"/></box>
    <list id="l" each="rows as r" key="r.id"><item><text>{r.id}</text><use template="e" with="pick.x as f"/></item></list>
    <tabs id="tb"><tab id="a" label="A"><use template="e" with="tabv as f"/></tab></tabs>
    <table id="t" each="rows as r" key="r.id"><column>{r.id}</column><use template="e" with="r.x as f"/></table>
  </screen>
</tui>`
	a := useApp(t, src, `{"aa": [{"x": 1}, {}], "bb": [{"x": 1}, {"x": 2}], "rows": [{"id": 1, "x": 1}, {"id": 2}]}`)
	sameDiags(t, "validate", useDiags(a), []string{
		`warning B002 5:5 /screen/use if="gone": path "gone" is missing`,
		`warning B002 6:5 /screen/use if="on": path "on" is missing`,
		`warning B003 7:25 /screen/box/use with path "r.x" (alias f) is missing`,
		`warning B003 9:71 /screen/list/item/use with path "pick.x" (alias f) is missing`,
		`warning B003 10:41 /screen/tabs/tab/use with path "tabv" (alias f) is missing`,
		`warning B003 11:70 /screen/table/use with path "r.x" (alias f) is missing`,
	})
	if err := a.Set("pick", map[string]any{"x": 1.0}); err != nil {
		t.Fatal(err)
	}
	if err := a.Set("on", true); err != nil {
		t.Fatal(err)
	}
	if err := a.Set("cfg", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	sameDiags(t, "set", useDiags(a), []string{
		`warning B003 3:24 /screen/use with path "g.y" (alias h) is missing (cfg.y via <use template="outer">)`,
		`warning B002 5:5 /screen/use if="gone": path "gone" is missing`,
		`warning B003 7:25 /screen/box/use with path "r.x" (alias f) is missing`,
		`warning B003 10:41 /screen/tabs/tab/use with path "tabv" (alias f) is missing`,
		`warning B003 11:70 /screen/table/use with path "r.x" (alias f) is missing`,
	})
}

// A path through a with alias that stands for a path from an each alias
// (P from a row or an element) reads the same in every each that holds
// such a use, so a use without id is also named by where it is written,
// as the placement suffix names it (the owner's decision on the F3
// review): two eaches over one template give two B003 (the review's
// two/app.tui). So is one written in an each inside a copy, whose alias
// is renamed. A use with an id is named by it, and a P from a store
// member says which data it reads: neither adds a place.
func TestViaFromAnEachAliasNamesWhereTheUseIs(t *testing.T) {
	const two = `<tui>
  <template id="b"><text>{x.missing}</text></template>
  <screen id="main">
    <box each="aa as r"><use template="b" with="r as x"/></box>
    <box each="bb as r"><use template="b" with="r as x"/></box>
  </screen>
</tui>
`
	a := useApp(t, two, `{"aa":[{}],"bb":[{}]}`)
	sameDiags(t, "two", useDiags(a), []string{
		`warning B003 2:20 /screen/box/text bind path "x.missing" is missing (r.missing via <use template="b"> at app.tui:4:25)`,
		`warning B003 2:20 /screen/box/text bind path "x.missing" is missing (r.missing via <use template="b"> at app.tui:5:25)`,
	})
	const others = `<tui>
  <template id="b"><text>{x.missing}</text></template>
  <template id="w"><box each="f.items as it"><use template="b" with="it as x"/></box></template>
  <screen id="main">
    <use id="u1" template="w" with="d as f"/>
    <list id="l" each="rows as r" key="r.id"><item><use id="k" template="b" with="r as x"/></item></list>
    <use id="s" template="b" with="cfg as x"/>
    <use template="b" with="cfg as x"/>
  </screen>
</tui>`
	a = useApp(t, others, `{"d": {"items": [{}]}, "rows": [{"id": 1}], "cfg": {}}`)
	sameDiags(t, "others", useDiags(a), []string{
		`warning B003 2:20 /screen/box/text bind path "x.missing" is missing (it.missing via <use template="b"> at app.tui:3:46)`,
		`warning B003 2:20 /screen/list/item/text bind path "x.missing" is missing (r.missing via <use id="k">)`,
		`warning B003 2:20 /screen/text bind path "x.missing" is missing (cfg.missing via <use id="s">)`,
		`warning B003 2:20 /screen/text bind path "x.missing" is missing (cfg.missing via <use template="b">)`,
	})
}

// A via is kept where its path is written (ir.ViaKey), not under the path
// it stands for: {f.x} and {g.x}, with f and g standing for one path, name
// each its own written path; a literal {a.x} beside them is a store path
// with the placement suffix; and a store flag next to a guard through an
// alias for the same path stays a store flag (B002), while the guard is
// an optional prop.
func TestViasAreKeptWhereWritten(t *testing.T) {
	const src = `<tui>
  <template id="t">
    <col>
      <text>{f.x} {g.x} {a.x}</text>
      <text if="!a.on" class:k="f.on">y</text>
    </col>
  </template>
  <screen id="main">
    <use id="u" template="t" with="a as f, a as g"/>
  </screen>
</tui>`
	a := useApp(t, src, `{"a": {}}`)
	sameDiags(t, "validate", useDiags(a), []string{
		`warning B003 4:7 /screen/col/text bind path "f.x" is missing (a.x via <use id="u">)`,
		`warning B003 4:7 /screen/col/text bind path "g.x" is missing (a.x via <use id="u">)`,
		`warning B003 4:7 /screen/col/text bind path "a.x" is missing, placed by <use id="u"> at app.tui:9:5`,
		`warning B002 5:7 /screen/col/text if="!a.on": path "a.on" is missing, placed by <use id="u"> at app.tui:9:5`,
	})
	if got := texts(a); !reflect.DeepEqual(got, []string{"  ", "y"}) {
		t.Errorf("texts %q", got)
	}

	// Every slot that holds a template string counts its paths apart
	// (ViaKey.Seg): a literal {a.N} before {f.N} in one string is a store
	// path, and only {f.N} names its via.
	const slots = `<tui>
  <template id="t">
    <col>
      <text>{a.body} {f.body}</text>
      <box border="single" height="3" title="{a.title} {f.title}"><text>z</text></box>
      <input id="i" placeholder="{a.iph} {f.iph}"/>
      <button id="b" label="{a.blabel} {f.blabel}"/>
      <button id="b2">{a.bbody} {f.bbody}</button>
      <checkbox id="c" label="{a.clabel} {f.clabel}"/>
      <select id="s" each="opts as o" key="o.id" label="{a.slabel} {f.slabel}" placeholder="{a.sph} {f.sph}"/>
      <table id="tb" each="rows as r" key="r.id"><column title="{a.ctitle} {f.ctitle}">{a.cell} {f.cell}</column></table>
      <table id="te" each="none as r" key="r.id" placeholder="{a.tph} {f.tph}"><column>{r.id}</column></table>
      <tabs id="tabs"><tab id="tt" label="{a.tlabel} {f.tlabel}" short="{a.tshort} {f.tshort}"><text>q</text></tab></tabs>
    </col>
  </template>
  <screen id="main">
    <scroll><use id="u" template="t" with="a as f"/></scroll>
  </screen>
</tui>`
	want := []string{}
	for _, s := range []struct{ at, path, name string }{
		{"4:7", "text", "body"},
		{"5:7", "box", "title"},
		{"6:7", "input", "iph"},
		{"7:7", "button", "blabel"},
		{"8:7", "button", "bbody"},
		{"9:7", "checkbox", "clabel"},
		{"10:7", "select", "sph"},
		{"10:7", "select", "slabel"},
		{"11:50", "table/column", "cell"},
		{"11:50", "table/column", "ctitle"},
		{"12:7", "table", "tph"},
		{"13:23", "tabs/tab", "tlabel"},
		{"13:23", "tabs/tab", "tshort"},
	} {
		at := "warning B003 " + s.at + " /screen/scroll/col/" + s.path + " bind path "
		want = append(want,
			at+`"a.`+s.name+`" is missing, placed by <use id="u"> at app.tui:17:13`,
			at+`"f.`+s.name+`" is missing (a.`+s.name+` via <use id="u">)`)
	}
	sa := useApp(t, slots, `{"a": {}, "opts": [{"id": 1}], "rows": [{"id": 1}], "none": []}`)
	sameDiags(t, "slots", useDiags(sa), want)
}

// What a diagnostic at an element of a copy cuts (ir.CutArgs, ir.ShownAt)
// still tells two diagnostics apart (the F3 review): two missing paths of
// one element that differ only past the 64-byte cut are two B003, two
// class guards whose names do two B002, and two with clauses whose
// aliases do two B003 at the use; what every pass of Validate repeats is
// still one.
func TestCutValuesStayApart(t *testing.T) {
	long := strings.Repeat("a", 70)
	src := `<tui>
  <template id="in"><text>x</text></template>
  <template id="t">
    <col>
      <text>{f.` + long + `1} {f.` + long + `2}</text>
      <text class:` + long + `1="f.q" class:` + long + `2="f.q">y</text>
      <use template="in" with="z as ` + long + `1, z as ` + long + `2"/>
    </col>
  </template>
  <screen id="main">
    <use id="u" template="t" with="x as f"/>
  </screen>
</tui>`
	a := useApp(t, src, `{"x": 5}`)
	cut := strings.Repeat("a", 62) + "…"
	b003 := `warning B003 5:7 /screen/col/text bind path "f.` + cut + `" is missing (x.` + cut + ` via <use id="u">)`
	b002 := `warning B002 6:7 /screen/col/text class:` + strings.Repeat("a", 58) + `…="f.q": path "f.q" is missing (x.q via <use id="u">)`
	with := `warning B003 7:7 /screen/col/use with path "z" (alias ` + strings.Repeat("a", 64) + `…) is missing`
	sameDiags(t, "validate", useDiags(a), []string{b003, b003, b002, b002, with, with})
}

// What a diagnostic quotes is cut only at an element of a copy: the same
// path written once in the screen is quoted whole, as 0.4.1 quotes it,
// and so is it in a pinned document.
func TestWrittenElementsQuoteWhole(t *testing.T) {
	path := "st.m" + strings.Repeat("y", 66) // 70 bytes
	src := `<tui>
  <template id="t">
    <text>{` + path + `}</text>
  </template>
  <screen id="main">
    <text>{` + path + `}</text>
    <use id="u" template="t"/>
  </screen>
</tui>`
	a := useApp(t, src, `{"st": {}}`)
	sameDiags(t, "validate", useDiags(a), []string{
		`warning B003 3:5 /screen/text bind path "st.m` + strings.Repeat("y", 60) + `…" is missing, placed by <use id="u"> at app.tui:7:5`,
		`warning B003 6:5 /screen/text bind path "` + path + `" is missing`,
	})
	pinned := useApp(t, `<tui version="3"><screen id="main"><text>{`+path+`}</text></screen></tui>`, `{"st": {}}`)
	sameDiags(t, "pinned", useDiags(pinned), []string{`warning B003 1:36 /screen/text bind path "` + path + `" is missing`})
}
