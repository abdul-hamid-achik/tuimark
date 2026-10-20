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
// its input (document and data), within a second. A message about a path
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
			if el > time.Second {
				t.Errorf("took %v", el)
			}
			if fmt.Sprint(got) != fmt.Sprint(c.want) {
				t.Errorf("diagnostics %v, want %v", got, c.want)
			}
		})
	}
}
