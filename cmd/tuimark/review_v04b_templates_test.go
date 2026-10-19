package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
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

// A guarded <use> in a <table> prunes what it placed (§6.18.3: no node of
// its copy is in the frame): a pruned column is absent altogether (no
// header cell, no body cells, no width, no say in the header row, no
// B003 from its cell template), a pruned row template leaves the rows
// without its classes, and flipping the guard with play's set: brings
// them back, the column measured again (review probes ifuse, ifcol).
func TestV04bTemplateGuardedUseInATable(t *testing.T) {
	doc := `<tui>
<template id="colsT"><column title="B">{r.b}{r.gone}</column></template>
<template id="itemT"><item class="hot"/></template>
<style>.hot { bold: true; }</style>
<screen id="main">
<table id="tb" each="rs as r" key="r.a"><column>{r.a}</column><use template="colsT" if="show"/><use template="itemT" hidden="!show"/></table>
</screen>
</tui>`
	f := writeTemp(t, "app.tui", doc)
	off := writeTemp(t, "d.json", `{"show": false, "rs": [{"a": "1", "b": "wide-value"}]}`)
	on := writeTemp(t, "d.json", `{"show": true, "rs": [{"a": "1", "b": "wide-value"}]}`)
	summary := func(out string) (cols, items []string, errs []string) {
		t.Helper()
		var d struct {
			Nodes []struct {
				Tag     string   `json:"tag"`
				Text    string   `json:"text"`
				W       int      `json:"w"`
				Y       int      `json:"y"`
				Classes []string `json:"classes"`
			} `json:"nodes"`
			Errors []struct {
				Code string `json:"code"`
				Msg  string `json:"msg"`
			} `json:"errors"`
		}
		if err := json.Unmarshal([]byte(out), &d); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		for _, n := range d.Nodes {
			switch n.Tag {
			case "column":
				cols = append(cols, n.Text+"/"+strconv.Itoa(n.W))
			case "item":
				items = append(items, strconv.Itoa(n.Y)+":"+strings.Join(n.Classes, ","))
			}
		}
		for _, e := range d.Errors {
			errs = append(errs, e.Code+" "+e.Msg)
		}
		return cols, items, errs
	}
	_, out, _ := runCLI("dump", f, "--data", off, "--format", "json", "--cols", "20", "--rows", "3")
	cols, items, errs := summary(out)
	// One untitled column: no header row (so no column node is laid out),
	// the row at y 0 with one cell, no class hot.
	sameLines(t, "pruned columns", cols, nil)
	sameLines(t, "pruned row template", items, []string{"0:"})
	sameLines(t, "pruned diagnostics", errs, nil)
	if rows := gridRows(t, f, "--data", off, "--cols", "20", "--rows", "3"); rows[0] != "1" {
		t.Errorf("grid %q, want the row first, with no header", rows)
	}
	_, out, _ = runCLI("dump", f, "--data", on, "--format", "json", "--cols", "20", "--rows", "3")
	cols, items, errs = summary(out)
	sameLines(t, "columns", cols, []string{"/1", "B/10"})
	sameLines(t, "row template", items, []string{"1:hot"})
	if len(errs) != 1 || !strings.HasPrefix(errs[0], `B003 bind path "r.gone" is missing`) {
		t.Errorf("diagnostics %q", errs)
	}
	// The guard flips in play: the same frame shows the column, measured
	// for its rows, and the row template.
	code, out, errw := runCLI("play", f, "--data", off, "--input", "set:show=true", "--format", "json", "--cols", "20", "--rows", "3")
	if code != 0 {
		t.Fatalf("play: exit %d %s", code, errw)
	}
	cols, items, _ = summary(out)
	sameLines(t, "after set:", cols, []string{"/1", "B/10"})
	sameLines(t, "row template after set:", items, []string{"1:hot"})
	code, out, errw = runCLI("play", f, "--data", on, "--input", "set:show=false", "--format", "json", "--cols", "20", "--rows", "3")
	if code != 0 {
		t.Fatalf("play: exit %d %s", code, errw)
	}
	cols, items, errs = summary(out)
	sameLines(t, "after set:false", cols, nil)
	sameLines(t, "row template after set:false", items, []string{"0:"})
	sameLines(t, "diagnostics after set:false", errs, nil)
}

// A use whose id is V003 inserts nothing, as one with a malformed with,
// and that V003 is its only diagnostic: id="" is not also the V012 of a
// use without id (review probe rob/c14). A valid id still scopes the copy.
func TestV04bTemplateUseWithABadID(t *testing.T) {
	f := writeTemp(t, "app.tui", `<tui><template id="t"><text id="x">x</text></template><screen id="m"><use id="" template="t"/><use id="a b" template="t"/><use id="1" template="t"/><use id="ok" template="t"/></screen></tui>`)
	sameLines(t, "validate", vdiags(t, f, ""), []string{
		`error V003 app.tui:1:75 /screen/use id= bad id "" (letters, digits, '-' and '_'; must not start with a digit)`,
		`error V003 app.tui:1:100 /screen/use id=a b bad id "a b" (letters, digits, '-' and '_'; must not start with a digit)`,
		`error V003 app.tui:1:128 /screen/use id=1 bad id "1" (letters, digits, '-' and '_'; must not start with a digit)`,
	})
	_, out, _ := runCLI("dump", f, "--format", "json", "--cols", "10", "--rows", "3")
	var d struct {
		Nodes []struct {
			ID string `json:"id"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal([]byte(out), &d); err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, n := range d.Nodes {
		ids = append(ids, n.ID)
	}
	sameLines(t, "nodes", ids, []string{"m", "ok__x"})
}

// In the current language, the HTML names of composition point at
// <template>/<use> and <include> (the F2 review); a document that writes
// version keeps 0.4.1's hint (review probe comp).
func TestV04bTemplateCompositionHints(t *testing.T) {
	const body = "\n<screen><component/><slot/><import/></screen>\n</tui>"
	sameLines(t, "current", vdiags(t, writeTemp(t, "app.tui", "<tui>"+body), ""), []string{
		`error V001 app.tui:2:9 /screen/component id= unknown tag <component>; write it once in a <template id=…> and place it with <use template=…>`,
		`error V001 app.tui:2:21 /screen/slot id= unknown tag <slot>; a <use> takes no content: write the markup in a <template id=…> and place it with <use template=…>`,
		`error V001 app.tui:2:28 /screen/import id= unknown tag <import>; use <include src=…> for markup, <style src=…> for a stylesheet`,
	})
	const old = "Tuimark has no components or includes; only <style src> loads another file"
	for _, v := range []string{"1", "2", "3"} {
		sameLines(t, "version="+v, vdiags(t, writeTemp(t, "app.tui", `<tui version="`+v+`">`+body), ""), []string{
			`error V001 app.tui:2:9 /screen/component id= unknown tag <component>; ` + old,
			`error V001 app.tui:2:21 /screen/slot id= unknown tag <slot>; ` + old,
			`error V001 app.tui:2:28 /screen/import id= unknown tag <import>; ` + old,
		})
	}
}

// `tuimark ir` at the expansion depth limit (the F2 review): a chain of
// element-less uses whose text lands at level 256 prints its IR, 256
// levels deep; a chain of 6000 is one V021 (exit 2), where the IR used to
// nest past what encoding/json reads back and the command failed (exit 1).
func TestV04bIRAtTheExpansionDepthLimit(t *testing.T) {
	chain := func(n int) string {
		var b strings.Builder
		b.WriteString("<tui>\n")
		for i := 0; i < n; i++ {
			b.WriteString(`<template id="t` + strconv.Itoa(i) + `"><use template="t` + strconv.Itoa(i+1) + `"/></template>` + "\n")
		}
		b.WriteString(`<template id="t` + strconv.Itoa(n) + `"><text>x</text></template>` + "\n")
		b.WriteString(`<screen><use template="t0"/></screen>` + "\n</tui>")
		return b.String()
	}
	code, out, errw := runCLI("ir", writeTemp(t, "app.tui", chain(252)))
	if code != 0 {
		t.Fatalf("ir at the limit: exit %d %s", code, errw)
	}
	type node struct {
		Kind     string `json:"kind"`
		Children []node `json:"children"`
	}
	var doc struct {
		Root node `json:"root"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}
	var depth func(n node) int
	depth = func(n node) int {
		d := 0
		for _, c := range n.Children {
			d = max(d, depth(c))
		}
		return d + 1
	}
	if d := depth(doc.Root); d != 256 {
		t.Errorf("ir at the limit nests %d levels, want 256", d)
	}
	code, out, errw = runCLI("ir", writeTemp(t, "app.tui", chain(6000)))
	if code != 2 || !strings.Contains(out+errw, `error V021 app.tui:254:21 /screen/use use template="t253" exceeds the expansion depth limit of 256 levels`) {
		t.Errorf("ir past the limit: exit %d\n%s%s", code, out, errw)
	}
}
