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
