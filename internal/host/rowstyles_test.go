package host

import (
	"fmt"
	"strings"
	"testing"
)

// A list row takes the styles of the row before it only when every box
// in it looks the same to the selectors (sameStyleInputs): rows that
// differ in a guarded class, in :empty, in a child an if= or display:
// none drops, or in :selected are styled on their own, exactly as a cold
// build, which shares nothing, styles every row.
func TestListRowsTakeTheirNeighboursStylesOnlyWhenTheyLookAlike(t *testing.T) {
	const doc = `<tui version="3"><style>
item.hot { bold: true; }
.gone { display: none; }
text:empty { background: red; min-width: 2; }
item > row > text.n { color: green; }
item:selected > row { italic: true; }
</style><screen id="s" focus="#l">
<list id="l" each="rows as r" key="r.id" bind="cur"><item class:hot="r.hot"><row><text class:gone="r.hide">{r.name}</text><text class="n" if="r.show">{r.n}</text></row></item></list>
</screen></tui>`
	mk := func(cold bool) *App {
		a, err := Parse(strings.NewReader(doc))
		if err != nil {
			t.Fatal(err)
		}
		rows := make([]any, 40)
		for i := range rows {
			name := "row"
			if i%7 == 3 {
				name = ""
			}
			rows[i] = map[string]any{"id": fmt.Sprint(i), "name": name, "n": float64(i),
				"hot": i%5 == 2, "hide": i%6 == 4, "show": i%4 != 1}
		}
		if err := a.Bind("", map[string]any{"rows": rows, "cur": "0"}); err != nil {
			t.Fatal(err)
		}
		a.noCache = cold
		return a
	}
	warm, cold := mk(false), mk(true)
	for step := 0; step < 12; step++ {
		fw, fc := warm.Frame(40, 30), cold.Frame(40, 30)
		if len(fw.Diags) != 0 {
			t.Fatalf("diagnostics: %v", fw.Diags)
		}
		if d := FrameDiff(fw, fc); d != "" {
			t.Fatalf("step %d: %s", step, d)
		}
		warm.HandleKey(Key{Name: "down"})
		cold.HandleKey(Key{Name: "down"})
	}
}

// A row whose cascade dropped a box under it (display: none) is never a
// model for the next row: after the drop it has no children and is
// :empty, while its styles were computed with the child there, so a
// later row that an if= (or an empty each) left with no children would
// match it and miss the :empty rules.
func TestListRowThatDroppedABoxIsNoModel(t *testing.T) {
	cases := []struct {
		name, doc string
		rows      []any
	}{{
		name: "if leaves the row empty",
		doc: `<tui version="2"><style>
.gone { display: none; }
item:empty { display: none; }
</style><screen>
<list id="l" each="rows as it" key="it.id"><item><text if="it.show" class="gone">x</text></item></list>
<text>end</text>
</screen></tui>`,
		rows: []any{
			map[string]any{"id": "a", "show": true},
			map[string]any{"id": "b", "show": true},
			map[string]any{"id": "c", "show": false},
			map[string]any{"id": "d", "show": false},
		},
	}, {
		name: "empty each one level down",
		doc: `<tui version="3"><style>
.gone { display: none; }
col:empty { height: 3; }
</style><screen>
<list id="l" each="rows as it" key="it.id"><item><col each="it.sub as s"><text class:gone="s.d">{s.name}</text></col></item></list>
</screen></tui>`,
		rows: []any{
			map[string]any{"id": "a", "sub": []any{map[string]any{"name": "x", "d": false}}},
			map[string]any{"id": "b", "sub": []any{map[string]any{"name": "y", "d": true}}},
			map[string]any{"id": "c", "sub": []any{}},
		},
	}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mk := func(cold bool) *App {
				a, err := Parse(strings.NewReader(tc.doc))
				if err != nil {
					t.Fatal(err)
				}
				if err := a.Bind("", map[string]any{"rows": tc.rows}); err != nil {
					t.Fatal(err)
				}
				a.noCache = cold
				return a
			}
			warm, cold := mk(false), mk(true)
			if d := FrameDiff(warm.Frame(20, 10), cold.Frame(20, 10)); d != "" {
				t.Errorf("warm frame differs from cold: %s", d)
			}
			if dw, dc := warm.Dump(20, 10, false), cold.Dump(20, 10, false); len(dw.Nodes) != len(dc.Nodes) {
				t.Errorf("Dump nodes: warm %d, cold %d", len(dw.Nodes), len(dc.Nodes))
			}
		})
	}
}
