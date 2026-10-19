package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Regressions from the F2 review of <template> and <use> (SPEC v0.4b
// §6.18.3-§6.18.5): the probes the review ran, reduced to what each one
// checks.

// gridRows runs dump --format json and returns the grid's rows, right
// trimmed.
func gridRows(t *testing.T, args ...string) []string {
	t.Helper()
	code, out, errw := runCLI(append([]string{"dump"}, append(args, "--format", "json")...)...)
	if code != 0 && code != 2 || out == "" {
		t.Fatalf("dump %v: exit %d %s", args, code, errw)
	}
	var d struct {
		Grid []string `json:"grid"`
	}
	if err := json.Unmarshal([]byte(out), &d); err != nil {
		t.Fatal(err)
	}
	rows := make([]string, len(d.Grid))
	for i, r := range d.Grid {
		rows[i] = strings.TrimRight(r, " ")
	}
	return rows
}

// writeFiles writes name → content into one temporary directory and
// returns its path.
func writeFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// A with alias stands for a path that resolves where its use stands
// (§6.18.5): an each alias of the copy with the same name as the path's
// first segment never captures it, whether that segment is a store root,
// an each alias around the use, or reached through a nested use's with.
// What the copy writes with the each alias still reads the row, and an
// each alias written equal to the with alias itself still shadows it, with
// its V011 (test 141).
func TestV04bTemplateWithAliasesAreHygienic(t *testing.T) {
	const order = `{"order": {"id": "STORE-ORDER-ID", "lines": [{"k": "L1", "id": "ROW1-ID"}, {"k": "L2", "id": "ROW2-ID"}]}}`
	data := writeTemp(t, "d.json", order)
	cases := []struct {
		name, doc, data string
		want            []string
		diags           []string
	}{{
		name: "store root",
		doc:  `<tui><template id="T"><col each="o.lines as order" key="order.k"><text>{o.id}|{order.k}</text></col></template><screen><use template="T" with="order as o"/></screen></tui>`,
		data: data,
		want: []string{"STORE-ORDER-ID|L1", "STORE-ORDER-ID|L2"},
	}, {
		name: "each alias around the use",
		doc:  "<tui>\n<template id=\"T\"><col each=\"o.lines as r\" key=\"r.k\"><text>{o.id}|{r.k}</text></col></template>\n<screen><col each=\"rows as r\" key=\"r.k\"><use template=\"T\" with=\"r as o\"/></col></screen>\n</tui>",
		data: writeTemp(t, "d.json", `{"rows":[{"k":"A","id":"ROWA","lines":[{"k":"x"}]}]}`),
		want: []string{"ROWA|x"},
		// §6.8's V011 for the copy's each inside the use's each stands as
		// before, its message showing the alias as written.
		diags: []string{`error V011 app.tui:2:18 /screen/col/col id= each="r.lines as r": the alias "r" repeats the alias of the enclosing each="rows as r" of <col> at 3:9; pick another name, placed by <use template="T"> at app.tui:3:41`},
	}, {
		name: "nested use",
		doc:  `<tui><template id="A"><col each="o.lines as order" key="order.k"><use template="B" with="o as q"/></col></template><template id="B"><text>{q.id}|{order.k}</text></template><screen><use template="A" with="order as o"/></screen></tui>`,
		data: data,
		want: []string{"STORE-ORDER-ID|L1", "STORE-ORDER-ID|L2"},
	}, {
		name:  "test 141: an each alias equal to the with alias shadows it",
		doc:   "<tui>\n<template id=\"t2\"><box each=\"a.items as a\" key=\"a\"><text>{a}</text></box></template>\n<screen id=\"main\"><use id=\"u\" template=\"t2\" with=\"data as a\"/></screen>\n</tui>",
		data:  writeTemp(t, "d.json", `{"data": {"items": ["one", "two"]}}`),
		want:  []string{"one", "two"},
		diags: []string{`error V011 app.tui:2:19 /screen/box id= each alias "a" repeats the with alias of <use id="u"> at app.tui:3:19`},
	}}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := writeTemp(t, "app.tui", c.doc)
			rows := gridRows(t, f, "--data", c.data, "--cols", "30", "--rows", "4")
			if got := strings.Join(rows, "\n"); !strings.HasPrefix(got, strings.Join(c.want, "\n")+"\n") {
				t.Errorf("grid:\n%s\nwant first rows %q", got, c.want)
			}
			want := c.diags
			if want == nil {
				want = []string{}
			}
			sameLines(t, "validate", vdiags(t, f, c.data), want)
		})
	}
}

// A renamed each alias never surfaces: not in a diagnostic, an event's
// keys, inspect, the IR, or the dump.
func TestV04bTemplateRenamedAliasNeverSurfaces(t *testing.T) {
	doc := `<tui>
<template id="T">
  <list id="l" each="o.lines as order" key="order.k" on:select="pick">
    <item class:hot="order.hot"><text>{o.id}|{order.k}|{order.gone}</text></item>
  </list>
</template>
<screen id="main"><use id="u" template="T" with="order as o"/></screen>
</tui>`
	f := writeTemp(t, "app.tui", doc)
	data := writeTemp(t, "d.json", `{"order": {"id": "S", "lines": [{"k": "L1", "hot": true}, {"k": "L2"}]}}`)
	var outs []string
	check := func(what string, args ...string) string {
		t.Helper()
		code, out, errw := runCLI(args...)
		if code != 0 && code != 2 {
			t.Fatalf("%s: exit %d %s", what, code, errw)
		}
		for _, bad := range []string{"\x01", `\u0001`, `\x01`} {
			if strings.Contains(out+errw, bad) {
				t.Errorf("%s shows a renamed alias (%q):\n%s%s", what, bad, out, errw)
			}
		}
		outs = append(outs, out)
		return out
	}
	v := check("validate", "validate", f, "--data", data, "--json")
	if !strings.Contains(v, `bind path \"order.gone\" is missing`) {
		t.Errorf("validate: want the B003 with the path as written:\n%s", v)
	}
	check("dump", "dump", f, "--data", data, "--format", "json", "--styles")
	p := check("play", "play", f, "--data", data, "--input", "down", "--format", "json")
	if !strings.Contains(p, `"keys": {`) || !strings.Contains(p, `"order": "L2"`) {
		t.Errorf("play: want the event's keys under the written alias:\n%s", p)
	}
	check("inspect", "inspect", f, "--data", data, "--id", "u__l", "--json")
	check("inspect a row", "inspect", f, "--data", data, "--at", "0,0", "--json")
	check("ir", "ir", f)
	rows := gridRows(t, f, "--data", data, "--cols", "20", "--rows", "3")
	if rows[0] != "S|L1|" || rows[1] != "S|L2|" {
		t.Errorf("grid %q", rows)
	}
}
