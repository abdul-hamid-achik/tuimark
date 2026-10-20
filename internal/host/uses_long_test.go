//go:build !js

package host

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// ADR 0034 for what phase F3 reports about copies (too slow for js/wasm,
// whose test run is `task wasm-test`): a long with path, a long use id,
// or a use in many list rows gives a validate output at most ten times
// its input (document and data), within a second (scaled by slowdown under
// the race detector). A message about a path
// through a with alias shows the path the alias stands for, and the use's
// id, cut at 64 bytes; a use's own B002 and B003 are formatted once per
// pass, however many rows place it.
func TestCopyMessagesStayLinear(t *testing.T) {
	long := "p" + strings.Repeat("x", 100000)
	texts := func(format string) string {
		var b strings.Builder
		for i := 0; i < 100; i++ {
			fmt.Fprintf(&b, format, i)
		}
		return b.String()
	}
	rows := func(n int) string {
		var b strings.Builder
		b.WriteString(`{"rows": [`)
		for i := 0; i < n; i++ {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, `{"id": %d}`, i)
		}
		b.WriteString("]}")
		return b.String()
	}
	// many is 150 uses of template t, each with its own with path aN (so
	// each copy's message is its own), and data that gives each aN the
	// JSON value v: about 15 MB of copies, within the expansion budget.
	const copies = 150
	many := func(t string) string {
		var b strings.Builder
		for i := 0; i < copies; i++ {
			fmt.Fprintf(&b, `<use template="%s" with="a%d as f"/>`, t, i)
		}
		return b.String()
	}
	// placedBy is many("t") with each use given the id prefix+N.
	placedBy := func(prefix string) string {
		var b strings.Builder
		for i := 0; i < copies; i++ {
			fmt.Fprintf(&b, `<use id="%s%d" template="t" with="a%d as f"/>`, prefix, i, i)
		}
		return b.String()
	}
	manyData := func(v string) string {
		var b strings.Builder
		b.WriteString("{")
		for i := 0; i < copies; i++ {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, `"a%d": %s`, i, v)
		}
		b.WriteString("}")
		return b.String()
	}
	for _, c := range []struct {
		name, src, data string
		want            map[string]int // diagnostics per code
	}{
		{"B003 through a long with path",
			`<tui><template id="t"><col>` + texts(`<text>{f.m%d}</text>`) + `</col></template><screen id="main"><use id="u" template="t" with="` + long + ` as f"/></screen></tui>`,
			`{"` + long + `": {}}`, map[string]int{"B003": 100}},
		{"B002 through a long with path",
			`<tui><template id="t"><col>` + texts(`<text if="f.m%d">x</text>`) + `</col></template><screen id="main"><use id="u" template="t" with="` + long + ` as f"/></screen></tui>`,
			`{"` + long + `": 1}`, map[string]int{"B002": 100}},
		{"B001 through a long with path",
			`<tui><template id="t"><col>` + texts(`<box each="f.m%d as it"><text>{it}</text></box>`) + `</col></template><screen id="main"><use id="u" template="t" with="` + long + ` as f"/></screen></tui>`,
			`{"` + long + `": {}}`, map[string]int{"B001": 100}},
		{"a long with path that is missing",
			`<tui><template id="t"><col>` + texts(`<text>{f.m%d}</text>`) + `</col></template><screen id="main"><use id="u" template="t" with="` + long + ` as f"/></screen></tui>`,
			`{}`, map[string]int{"B003": 1}},
		{"a long use id",
			`<tui><template id="t"><col>` + texts(`<text>{f.m%d}</text>`) + `</col></template><screen id="main"><use id="u` + long + `" template="t" with="a as f"/></screen></tui>`,
			`{"a": {}}`, map[string]int{"B003": 100}},
		{"a nested use under a long outer path",
			`<tui><template id="in"><col>` + texts(`<text>{f.m%d}</text>`) + `</col></template><template id="out"><use id="z" template="in" with="a.z as f"/></template><screen id="main"><use id="o" template="out" with="` + long + ` as a"/></screen></tui>`,
			`{"` + long + `": {"z": {}}}`, map[string]int{"B003": 100}},
		{"a nested use's with path under a long outer path",
			`<tui><template id="in"><col>` + texts(`<text>{f.m%d}</text>`) + `</col></template><template id="out"><use id="z" template="in" with="a.z as f" if="a.on"/></template><screen id="main"><use id="o" template="out" with="` + long + ` as a"/></screen></tui>`,
			`{"` + long + `": {"on": true}}`, map[string]int{"B003": 1}},
		{"a use with a long with path in many rows",
			`<tui><template id="t"><text>{a.n}</text></template><screen id="main"><list id="l" each="rows as r" key="r.id"><item><use template="t" with="r.` + long[:10000] + ` as a" if="r.` + long[:10000] + `"/></item></list></screen></tui>`,
			rows(20000), map[string]int{"B002": 1}},
		// What a message quotes as written in the template (a path, an
		// attribute, a class name, an alias) is cut as P is, since every
		// copy through its own with path says it again.
		{"a long written path in many copies",
			`<tui><template id="t"><text>{f.` + long + `}</text></template><screen id="main"><col>` + many("t") + `</col></screen></tui>`,
			manyData("{}"), map[string]int{"B003": copies}},
		{"a long written flag in many copies",
			`<tui><template id="t"><text if="f.` + long + `">x</text></template><screen id="main"><col>` + many("t") + `</col></screen></tui>`,
			manyData("1"), map[string]int{"B002": copies}},
		{"a long written each in many copies",
			`<tui><template id="t"><box each="f.` + long + ` as it"><text>{it}</text></box></template><screen id="main"><col>` + many("t") + `</col></screen></tui>`,
			manyData("{}"), map[string]int{"B001": copies}},
		{"a long class name in many copies",
			`<tui><template id="t"><text class:` + long + `="f.x">x</text></template><screen id="main"><col>` + many("t") + `</col></screen></tui>`,
			manyData("1"), map[string]int{"B002": copies, "L009": 1}},
		{"a long alias in many copies",
			`<tui><template id="in"><text>x</text></template><template id="t"><use template="in" with="f.z as ` + long + `"/></template><screen id="main"><col>` + many("t") + `</col></screen></tui>`,
			manyData("{}"), map[string]int{"B003": copies, "L009": 1}},
		{"a long with path of an empty copy in many copies",
			`<tui><template id="e"></template><template id="t"><use template="e" with="f.` + long + ` as g"/></template><screen id="main"><col>` + many("t") + `</col></screen></tui>`,
			manyData("{}"), map[string]int{"B003": copies}},
		{"a long guard of an empty copy in many copies",
			`<tui><template id="e"></template><template id="t"><use template="e" if="f.` + long + `"/></template><screen id="main"><col>` + many("t") + `</col></screen></tui>`,
			manyData("1"), map[string]int{"B002": copies}},
		// A via whose P starts with an each alias names where its use is
		// written, so each of these 150 eaches reports its own B003.
		{"a long written path through an each alias in many eaches",
			`<tui><template id="t"><text>{f.` + long + `}</text></template><screen id="main"><col>` + strings.Repeat(`<box each="rows as r"><use template="t" with="r as f"/></box>`, copies) + `</col></screen></tui>`,
			`{"rows": [{}]}`, map[string]int{"B003": copies}},
		// Such a via also names what placed its use: here 150 uses with
		// long ids, each placing a template whose each holds the use.
		{"a via naming its use's long-id placer in many copies",
			`<tui><template id="in"><text>{x.` + long + `}</text></template><template id="t"><box each="f.rows as it"><use template="in" with="it as x"/></box></template><screen id="main"><col>` + placedBy(long[:1000]) + `</col></screen></tui>`,
			manyData(`{"rows": [{}]}`), map[string]int{"B003": copies}},
		{"a use's missing with path in many rows",
			`<tui><template id="t"><text>{a.n}</text></template><screen id="main"><list id="l" each="rows as r" key="r.id"><item><use template="t" with="r.` + long[:10000] + ` as a"/></item></list></screen></tui>`,
			rows(20000), map[string]int{"B003": 1}},
	} {
		t.Run(c.name, func(t *testing.T) {
			start := time.Now()
			a := useApp(t, c.src, c.data)
			ds := a.Validate()
			el := time.Since(start)
			out := 0
			got := map[string]int{}
			for _, d := range ds {
				out += len(d.String())
				got[d.Code]++
			}
			if in := len(c.src) + len(c.data); out > 10*in {
				t.Errorf("%d bytes of diagnostics for %d of input", out, in)
			}
			if el > slowdown*time.Second {
				t.Errorf("took %v", el)
			}
			if fmt.Sprint(got) != fmt.Sprint(c.want) {
				t.Errorf("diagnostics %v, want %v", got, c.want)
			}
		})
	}
}

// Every diagnostic at an element of a copy that quotes a value (a store
// path, an attribute, an action, a store value) cuts it at 64 bytes:
// each copy reports it with its own placement suffix, so 150 uses of a
// template that quotes a 100 KB path printed 15 MB before (the F3
// review). Each code reachable at a copy that quotes such a value is
// checked here, through the uses' placements alone (no with). B007 is
// not: it is reported at a keymap row, and a <keymap> cannot be inside a
// <template> (V001), so no copy holds one.
func TestCopyDiagnosticsQuoteValuesShortened(t *testing.T) {
	long := "p" + strings.Repeat("x", 100000)
	value := strings.Repeat("v", 100000)
	const copies = 150
	var uses strings.Builder
	for i := 0; i < copies; i++ {
		fmt.Fprintf(&uses, `<use id="u%d" template="t"/>`, i)
	}
	doc := func(body string) string {
		return `<tui><template id="t">` + body + `</template><screen id="main"><scroll>` + uses.String() + `</scroll></screen></tui>`
	}
	for _, c := range []struct {
		code, body, data string
	}{
		{"B001", `<box each="st.` + long + ` as it"><text>{it}</text></box>`, `{"st": {}}`},
		{"B002", `<text if="st.` + long + `">x</text>`, `{"st": {}}`},
		{"B003", `<text>{st.` + long + `}</text>`, `{"st": {}}`},
		{"B003", `<box each="rows as r" key="r.` + long + `"><text>x</text></box>`, `{"rows": [{}]}`},
		{"B004", `<button id="b" on:click="` + long + `">x</button>`, `{}`},
		{"B005", `<tabs id="tb"><tab id="a" label="A" focus="#` + long + `"><text>a</text></tab></tabs>`, `{}`},
		{"B005", `<modal id="m" open="false" focus="#` + long + `"><text>m</text></modal>`, `{}`},
		{"B006", `<list id="l" each="st.` + long + ` as r"><item><text>x</text></item></list>`, `{"st": {"` + long + `": []}}`},
		{"B008", `<list id="l" each="rows as r" key="r.id" checked="st.` + long + `"><item><text>x</text></item></list>`, `{"rows": [{"id": "a"}], "st": {"` + long + `": 5}}`},
		{"B009", `<sparkline bind="st.` + long + `"/>`, `{"st": {"` + long + `": "x"}}`},
		{"B010", `<tabs id="tb" bind="st.` + long + `"><tab id="a" label="A"><text>a</text></tab></tabs>`, `{"st": {"` + long + `": "` + value + `"}}`},
		{"B011", `<checkbox id="c" bind="st.` + long + `"/>`, `{"st": {"` + long + `": 5}}`},
		{"B012", `<select id="s" bind="st.` + long + `" each="opts as o" key="o.id" label="{o.n}"/>`, `{"opts": [{"id": "a", "n": "A"}], "st": {"` + long + `": "` + value + `"}}`},
	} {
		t.Run(c.code, func(t *testing.T) {
			src := doc(c.body)
			start := time.Now()
			a := useApp(t, src, c.data)
			ds := a.Validate()
			if c.code == "B004" {
				ds = append(ds, a.CheckCatalog(map[string]bool{})...)
			}
			el := time.Since(start)
			out, n := 0, 0
			for _, d := range ds {
				out += len(d.String())
				if d.Code == c.code {
					n++
				}
			}
			if in := len(src) + len(c.data); out > 10*in {
				t.Errorf("%d bytes of diagnostics for %d of input", out, in)
			}
			if el > slowdown*time.Second {
				t.Errorf("took %v", el)
			}
			if n != copies {
				t.Errorf("%d %s, want one per copy", n, c.code)
			}
		})
	}
}
