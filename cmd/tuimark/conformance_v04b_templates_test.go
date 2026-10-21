package main

import (
	"encoding/json"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/host"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// SPEC v0.4b §24 tests of <template> and <use> (phase F2), the parts that
// go through the CLI: 133, 138-140, 142, 143, 145, 146, 148, 153-155, and
// 217, and a <select> in a template. The Go API parts are the root
// package's conformance_v04b_templates_test.go. The fixtures are
// testdata/v04b/t133 ... t217 and tselect; a variant of a fixture is
// written from its text with one change.

// vdiags runs validate --json on file (with --data when data is not
// empty) and returns its diagnostics as "SEV CODE FILE:L:C PATH id=ID MSG".
func vdiags(t *testing.T, file, data string) []string {
	t.Helper()
	args := []string{"validate", file, "--json"}
	if data != "" {
		args = append(args, "--data", data)
	}
	_, out, errw := runCLI(args...)
	var v struct {
		Diagnostics []ir.Diagnostic `json:"diagnostics"`
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("validate %s: %v\n%s%s", file, err, out, errw)
	}
	got := []string{}
	for _, d := range v.Diagnostics {
		got = append(got, d.Severity+" "+d.Code+" "+d.File+":"+strconv.Itoa(d.Line)+":"+strconv.Itoa(d.Col)+" "+d.Path+" id="+d.ID+" "+d.Msg)
	}
	return got
}

// dumpNodes runs dump --format json with the extra args and returns its
// nodes and focus (a document with errors still dumps, with exit 2).
func dumpNodes(t *testing.T, args ...string) ([]map[string]any, any) {
	t.Helper()
	code, out, errw := runCLI(append([]string{"dump"}, append(args, "--format", "json")...)...)
	if code != 0 && code != 2 || out == "" {
		t.Fatalf("dump %v: exit %d %s", args, code, errw)
	}
	m := jsonMap(t, out)
	var nodes []map[string]any
	for _, n := range m["nodes"].([]any) {
		nodes = append(nodes, n.(map[string]any))
	}
	return nodes, m["focus"]
}

// variant writes the text of fixture with old replaced by new as
// app.tui in a temporary directory.
func variant(t *testing.T, fixture, old, new string) string {
	t.Helper()
	raw, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), old) {
		t.Fatalf("%s holds no %q", fixture, old)
	}
	return writeTemp(t, "app.tui", strings.Replace(string(raw), old, new, 1))
}

func sameLines(t *testing.T, what string, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s:\n got %s\nwant %s", what, strings.Join(got, "\n     "), strings.Join(want, "\n     "))
	}
}

// 133. Expansion equals hand expansion: no diagnostic, byte-identical
// dumps and plays at two sizes, and the play ends on the second copy's
// input with no event.
func TestV04bTemplate133HandExpansion(t *testing.T) {
	dir := composeFixtures + "t133/"
	data := dir + "d.json"
	for _, f := range []string{"app.tui", "flat.tui"} {
		sameLines(t, "validate "+f, vdiags(t, dir+f, data), []string{})
	}
	for _, size := range [][2]string{{"80", "24"}, {"40", "12"}} {
		run := func(f string, cmd ...string) string {
			code, out, errw := runCLI(append(cmd, dir+f, "--data", data, "--cols", size[0], "--rows", size[1], "--format", "json")...)
			if code != 0 {
				t.Fatalf("%v %s: exit %d %s", cmd, f, code, errw)
			}
			return out
		}
		dumpArgs := []string{"dump", "--cells", "--styles"}
		if a, b := run("app.tui", dumpArgs...), run("flat.tui", dumpArgs...); a != b {
			t.Errorf("%sx%s: dump differs from the hand expansion", size[0], size[1])
		}
		playArgs := []string{"play", "--input", "text:9 tab text:8"}
		a, b := run("app.tui", playArgs...), run("flat.tui", playArgs...)
		if a != b {
			t.Errorf("%sx%s: play differs from the hand expansion", size[0], size[1])
		}
		p := decodePlay(t, a)
		if p.Focus == nil || *p.Focus != "shipping__line__in" || len(p.Events) != 0 {
			t.Errorf("play: focus %v, events %v", p.Focus, p.Events)
		}
	}
}

// 138. Scoped ids, focus, events, and a selector naming a scoped id. The
// inspect source bullet is phase F4's.
func TestV04bTemplate138ScopedIDs(t *testing.T) {
	dir := composeFixtures + "t138/"
	app, data := dir+"app.tui", dir+"d.json"
	sameLines(t, "validate", vdiags(t, app, data), []string{})
	nodes, focus := dumpNodes(t, app, "--data", data)
	var ids []string
	for _, n := range nodes {
		if id, ok := n["id"].(string); ok {
			ids = append(ids, id)
		} else if n["tag"] != "text" {
			t.Errorf("a node without id that is not a text: %v", n)
		}
	}
	if want := []string{"main", "bill__row", "bill__in", "ship__row", "ship__in"}; !reflect.DeepEqual(ids, want) || focus != "bill__in" {
		t.Errorf("ids %v focus %v", ids, focus)
	}
	_, out, _ := runCLI("play", app, "--data", data, "--input", "text:ab enter tab text:c enter ctrl+s", "--format", "json")
	var p struct {
		Events json.RawMessage `json:"events"`
	}
	if err := json.Unmarshal([]byte(out), &p); err != nil {
		t.Fatal(err)
	}
	var got, want any
	_ = json.Unmarshal(p.Events, &got)
	_ = json.Unmarshal([]byte(`[{"step": 2, "action": "save", "source": "bill__in", "keys": {}, "value": "ab"},
		{"step": 5, "action": "save", "source": "ship__in", "keys": {}, "value": "c"},
		{"step": 6, "action": "save_all", "source": "ship__in", "keys": {}, "value": "c"}]`), &want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("events %s", p.Events)
	}
	props := f5Inspect(t, app, "bill__in", "80", "24")
	if b := props["bold"]; b.Value != "true" || b.Origin != "author" || b.Rule == nil || b.Rule.Selector != "#bill__in" {
		t.Errorf("bold: %+v", b)
	}
}

// 139. V004 and V012 with uses. The draft lists four diagnostics and says
// the uses of card report nothing; the runtime reports, as it has for a
// written <box focusable="true"> without id since 0.1 (validate of
// <tui><screen id="m"><box focusable="true"><text>c</text></box></screen></tui>
// gives it, with and without version="3"), a V012 at the copied box, once
// per use (placement suffix), reported as an erratum and pinned here (the
// F2 review confirmed 0.4.1's V012). The focus cycle bullet goes with
// them, since focus is held by id.
func TestV04bTemplate139V004AndV012(t *testing.T) {
	app := composeFixtures + "t139/app.tui"
	sameLines(t, "validate", vdiags(t, app, ""), []string{
		`error V004 app.tui:4:7 /screen/row/input id=bill__in duplicate id "bill__in" (first defined at 11:5)`,
		`error V012 app.tui:8:5 /screen/box id= <box> is focusable and needs an id, placed by <use template="card"> at app.tui:16:5`,
		`error V012 app.tui:8:5 /screen/box id= <box> is focusable and needs an id, placed by <use template="card"> at app.tui:17:5`,
		`error V004 app.tui:13:5 /screen/use id=bill duplicate id "bill" (first defined at 12:5)`,
		`error V004 app.tui:14:5 /screen/box id=field duplicate id "field" (first defined at 2:3)`,
		`error V012 app.tui:15:5 /screen/use id= <use template="field"> needs an id: its template holds ids (#row), which the use scopes as ID__row`,
	})
	host, err := host.Load(app)
	if err != nil {
		t.Fatal(err)
	}
	boxes := 0
	for _, n := range host.Document().Root.Children[0].Children {
		if n.Tag == "box" && n.ID == "" {
			boxes++
		}
	}
	if boxes != 2 {
		t.Errorf("%d boxes without id, want the two copies of card", boxes)
	}
}

// 140. V019: a cycle reported at each use on it, a missing template, a
// value written with #, and a use without template.
func TestV04bTemplate140V019(t *testing.T) {
	app := composeFixtures + "t140/app.tui"
	sameLines(t, "validate", vdiags(t, app, ""), []string{
		`error V019 app.tui:4:5 /template/use id= <use template="b"> in <template id="a"> makes a cycle of templates: a → b → a`,
		`error V019 app.tui:8:5 /template/use id= <use template="a"> in <template id="b"> makes a cycle of templates: b → a → b`,
		`error V019 app.tui:11:5 /template/use id= <use template="c"> in <template id="c"> makes a cycle of templates: c → c`,
		`error V019 app.tui:15:5 /screen/use id= <use template="nope"> names no <template>`,
		`error V019 app.tui:16:5 /screen/use id= <use template="#a"> names no <template>; write template="a", without #`,
		`error V019 app.tui:17:5 /screen/use id= <use> needs template="ID"`,
	})
	nodes, _ := dumpNodes(t, app)
	if len(nodes) != 2 || nodes[0]["tag"] != "screen" || nodes[1]["tag"] != "text" || nodes[1]["text"] != "a" {
		t.Errorf("nodes %v, want the screen and the text a", nodes)
	}
}

// 142. A #x in a template names the element of its own copy.
func TestV04bTemplate142References(t *testing.T) {
	dir := composeFixtures + "t142/"
	app, data := dir+"app.tui", dir+"d.json"
	sameLines(t, "validate", vdiags(t, app, data), []string{})
	nodes, focus := dumpNodes(t, app, "--data", data)
	label := false
	for _, n := range nodes {
		if n["tag"] == "text" && n["text"] == "One" && n["key"] == "p__one" {
			label = true
		}
	}
	if focus != "p__q" || !label {
		t.Errorf("focus %v, tab label keyed p__one: %v", focus, label)
	}
	code, out, errw := runCLI("play", app, "--data", data, "--input", "set:ask=true enter", "--format", "json")
	p := decodePlay(t, out)
	if code != 0 || len(p.Events) != 1 || p.Events[0].Step != 2 || p.Events[0].Action != "no" || p.Events[0].Source != "kill__no" ||
		len(p.Events[0].Keys) != 0 || p.Events[0].Value != nil || p.Focus == nil || *p.Focus != "kill__no" {
		t.Errorf("play: exit %d events %+v focus %v %s", code, p.Events, p.Focus, errw)
	}
	// The draft's message ends at "does not exist"; §6.18.6 appends the
	// placement suffix to a diagnostic about a copied element, which the
	// runtime does (an erratum of the test).
	got := vdiags(t, variant(t, app, `focus="#no"`, `focus="#zz"`), data)
	if len(got) != 1 || !strings.HasPrefix(got[0], `error B005 app.tui:3:5 /screen/modal id=kill__dlg focus="#zz" refers to an id that does not exist`) {
		t.Errorf("focus=#zz: %v", got)
	}
}

// 143. B005 for the ids of a use and a template, and for an id only a
// template writes.
func TestV04bTemplate143B005(t *testing.T) {
	sameLines(t, "validate", vdiags(t, composeFixtures+"t143/app.tui", composeFixtures+"t138/d.json"), []string{
		`error B005 app.tui:11:5 /keymap/bind id= to="#bill" is a <use>, not a node; name a node of its copy, such as #bill__row`,
		`error B005 app.tui:12:5 /keymap/bind id= to="#field" is a <template>, which is never in the frame`,
		`error B005 app.tui:13:5 /keymap/bind id= to="#in" refers to an id that does not exist; ids in a <template> are scoped by their <use>: #bill__in`,
		`error B005 app.tui:14:5 /keymap/bind id= when="#bill" refers to #bill, which is a <use>, not a node; name a node of its copy, such as #bill__row`,
	})
}

// 145. A use in a list item copies into each row; in an each template its
// ids are V004 and it needs no id.
func TestV04bTemplate145ListAndEach(t *testing.T) {
	dir := composeFixtures + "t145/"
	app, data := dir+"app.tui", dir+"d.json"
	sameLines(t, "validate", vdiags(t, app, data), []string{})
	nodes, _ := dumpNodes(t, app, "--data", data)
	var rows []string
	key := ""
	for _, n := range nodes {
		if n["tag"] == "item" {
			key, _ = n["key"].(string)
		}
		if n["tag"] == "text" && n["id"] == "c__t" {
			rows = append(rows, key+":"+n["text"].(string))
		}
	}
	if !reflect.DeepEqual(rows, []string{"a:A", "b:B"}) {
		t.Errorf("rows %v", rows)
	}
	const list = "    <list id=\"l\" each=\"rows as r\" key=\"r.id\">\n      <item><use id=\"c\" template=\"cell\" with=\"r as x\"/></item>\n    </list>"
	codes := func(lines []string) (out []string) {
		for _, l := range lines {
			f := strings.Fields(l)
			out = append(out, f[1]+" "+f[2])
		}
		return out
	}
	each := variant(t, app, list, `    <box each="rows as r"><use id="c" template="cell" with="r as x"/></box>`)
	sameLines(t, "each, use with id", codes(vdiags(t, each, data)), []string{"V004 app.tui:3:5", "V004 app.tui:9:27"})
	each = variant(t, app, list, `    <box each="rows as r"><use template="cell" with="r as x"/></box>`)
	sameLines(t, "each, use without id", codes(vdiags(t, each, data)), []string{"V004 app.tui:3:5"})
	form := vdiags(t, variant(t, app, `template="cell"`, `template="form"`), "")
	if len(form) != 1 || !strings.HasPrefix(form[0], "error V001 app.tui:6:5 ") || !strings.HasSuffix(form[0], `, placed by <use id="c"> at app.tui:10:13`) {
		t.Errorf("an input copied into a row: %v", form)
	}
}

// 146. A document that writes version reads no composition tag.
func TestV04bTemplate146Pinned(t *testing.T) {
	app := composeFixtures + "t146/app.tui"
	for _, v := range []string{"1", "2", "3"} {
		f := variant(t, app, `version="3"`, `version="`+v+`"`)
		sameLines(t, "version="+v, vdiags(t, f, ""), []string{
			"error V001 app.tui:2:3 /template id=t tag <template> (requires a <tui> without version)",
			"error V001 app.tui:4:5 /screen/include id= tag <include> (requires a <tui> without version)",
			"error V001 app.tui:5:5 /screen/use id=u tag <use> (requires a <tui> without version)",
		})
		// ir exits 2 on these errors, so the IR is built from the loaded
		// document, as test 137's is.
		h, err := host.Load(f)
		if err != nil {
			t.Fatal(err)
		}
		irOut, err := json.Marshal(parse.BuildIR(h.Document(), nil))
		if err != nil {
			t.Fatal(err)
		}
		for _, k := range []string{`"include"`, `"template"`, `"use"`} {
			if strings.Contains(string(irOut), `"kind":`+k) {
				t.Errorf("version=%s: the IR has a %s node", v, k)
			}
		}
	}
	got := vdiags(t, variant(t, app, `<tui version="3">`, `<tui>`), "")
	if len(got) != 1 || !strings.HasPrefix(got[0], `error V006 app.tui:4:5 /screen/include id= include src="missing.tui" cannot be read: `) {
		t.Errorf("without version: %v", got)
	}
}

// 148, the bullets about templates and uses: the tree keeps them as
// written, with the copies under their uses.
func TestV04bTemplate148IR(t *testing.T) {
	code, out, errw := runCLI("ir", composeFixtures+"t133/app.tui")
	if code != 0 {
		t.Fatalf("ir: exit %d %s", code, errw)
	}
	if errs := validateSchema(t, irV04Schema, decodeJSON(t, []byte(out))); len(errs) != 0 {
		t.Errorf("IR vs ir.v0.4.json: %v", errs)
	}
	m := jsonMap(t, out)
	if m["version"] != "0.4" {
		t.Errorf("version %v", m["version"])
	}
	root := m["root"].(map[string]any)
	inc := kids(t, root)[0]
	ts := kids(t, inc)
	if inc["kind"] != "include" || len(ts) != 2 || ts[0]["kind"] != "template" || ts[0]["from"] != "parts/templates.tui:2" || ts[1]["from"] != "parts/templates.tui:8" {
		t.Fatalf("include: %v", inc)
	}
	box := kids(t, ts[0])[0]
	if box["id"] != "box" || box["from"] != "parts/templates.tui:3" {
		t.Errorf("address's col: %v", box)
	}
	if u := kids(t, box)[1]; u["kind"] != "use" || u["id"] != "line" || len(kids(t, u)) != 0 {
		t.Errorf("the use in address: %v", u)
	}
	screen := kids(t, root)[1]
	billing := kids(t, screen)[1]
	if billing["id"] != "billing" || billing["kind"] != "use" || !reflect.DeepEqual(billing["attrs"], map[string]any{"template": "address", "with": "order.billing as a"}) {
		t.Errorf("billing: %v", billing)
	}
	if _, has := billing["from"]; has {
		t.Errorf("billing has from")
	}
	copyBox := kids(t, billing)
	if len(copyBox) != 1 || copyBox[0]["id"] != "billing__box" || copyBox[0]["from"] != "parts/templates.tui:3" {
		t.Fatalf("billing's copy: %v", copyBox)
	}
	cs := kids(t, copyBox[0])
	if cs[0]["text"] != "{a.city}" || cs[1]["id"] != "billing__line" {
		t.Errorf("billing__box's children: %v", cs)
	}
	in := kids(t, cs[1])
	if len(in) != 1 || in[0]["id"] != "billing__line__in" || in[0]["bind"] != "f" || in[0]["from"] != "parts/templates.tui:9" {
		t.Errorf("billing__line's copy: %v", in)
	}
	if code, out, _ := runCLI("ir", composeFixtures+"t133/flat.tui"); code != 0 || strings.Contains(out, `"from"`) {
		t.Errorf("flat.tui: exit %d, from in %s", code, out)
	}
}

// 153. Where a template may stand, and what a template and a use hold
// (as revised by SPEC v0.4c: the V002 of a name that could be a prop, and
// the V013 of content toward a template with no slot).
func TestV04bTemplate153PlacementAndContent(t *testing.T) {
	app := composeFixtures + "t153/app.tui"
	sameLines(t, "validate", vdiags(t, app, ""), []string{
		`error V001 app.tui:4:7 /screen/col/template id=a <template> must be a direct child of <tui> or <screen>`,
		`error V002 app.tui:6:30 /screen/use id=u attribute "style" is not allowed on <use>: a <use> is not a node; give this copy a static class="…" and style that class, put the attribute on the template's element, or declare it in the template's props`,
		`error V013 app.tui:6:40 /screen/use id=u <template id="b"> has no <slot/>`,
		`error V001 app.tui:9:5 /screen/screen id= <screen> cannot be inside a <template>: a template holds content only`,
		`error V012 app.tui:12:3 /template id= <template> needs an id, which <use template> names`,
	})
	nodes, _ := dumpNodes(t, app)
	var texts []string
	for _, n := range nodes {
		if n["tag"] == "text" {
			texts = append(texts, n["text"].(string))
		}
	}
	if !reflect.DeepEqual(texts, []string{"ok"}) {
		t.Errorf("texts %v", texts)
	}
	text := writeTemp(t, "app.tui", "<tui>\n  <template id=\"t\">x<text>y</text></template>\n  <screen id=\"main\"><use template=\"t\"/></screen>\n</tui>")
	sameLines(t, "text in a template", vdiags(t, text, ""), []string{
		`error V013 app.tui:2:20 /template id=t text is not allowed directly inside <template>; wrap it in <text>`,
	})
	if nodes, _ := dumpNodes(t, text); len(nodes) != 2 || nodes[1]["text"] != "y" {
		t.Errorf("the copy: %v", nodes)
	}
	keymap := writeTemp(t, "app.tui", "<tui>\n  <template id=\"t\"><text>y</text></template>\n  <keymap><use template=\"t\"/></keymap>\n  <screen id=\"main\"/>\n</tui>")
	sameLines(t, "a use in a keymap", vdiags(t, keymap, ""), []string{
		`error V001 app.tui:3:11 /keymap/use id= a <template> holds no <bind> rows; share keymap rows with <include>`,
	})
	// 0.4c: style is not a name a use takes for itself, so a template may
	// declare it: the use passes it, and its V002 goes. (The variant with
	// a <slot/> in b is phase C2's.)
	props := variant(t, composeFixtures+"t153/app.tui", `<template id="b">`, `<template id="b" props="style">`)
	sameLines(t, "props=\"style\"", vdiags(t, props, ""), []string{
		`error V001 app.tui:4:7 /screen/col/template id=a <template> must be a direct child of <tui> or <screen>`,
		`error V013 app.tui:6:40 /screen/use id=u <template id="b"> has no <slot/>`,
		`error V001 app.tui:9:5 /screen/screen id= <screen> cannot be inside a <template>: a template holds content only`,
		`error V012 app.tui:12:3 /template id= <template> needs an id, which <use template> names`,
	})
}

// 154. An unused template gets only the checks its elements give
// themselves.
func TestV04bTemplate154Unused(t *testing.T) {
	data := writeTemp(t, "d.json", "{}")
	got := vdiags(t, composeFixtures+"t154/app.tui", data)
	if len(got) != 2 || !strings.HasPrefix(got[0], "error V002 app.tui:3:18 ") || !strings.HasPrefix(got[1], "error V001 app.tui:4:5 /template/zzz id= unknown tag <zzz>") {
		t.Errorf("validate: %v", got)
	}
	// The V003 its own attributes give, an inline style="" included.
	own := writeTemp(t, "app.tui", "<tui>\n  <template id=\"t\">\n    <text style=\"colr: red\" width=\"abc\">hi</text>\n  </template>\n  <screen id=\"main\"><text>s</text></screen>\n</tui>")
	got = vdiags(t, own, data)
	if len(got) != 2 || !strings.HasPrefix(got[0], "error V003 app.tui:3:11 ") || !strings.Contains(got[0], `unknown property "colr"`) || !strings.HasPrefix(got[1], "error V003 app.tui:3:29 ") {
		t.Errorf("own V003: %v", got)
	}
}

// 155. Selectors: a composition tag is never a node, a use's id matches
// nothing, and scoped ids are ordinary ids.
func TestV04bTemplate155Selectors(t *testing.T) {
	for _, tag := range []string{"use", "include", "fragment", "template"} {
		f := writeTemp(t, "app.tui", "<tui>\n  <style>"+tag+" { bold: true; }</style>\n  <screen id=\"main\"/>\n</tui>")
		got := vdiags(t, f, "")
		want := `selector "` + tag + `": <` + tag + `> is not a node and never matches`
		if len(got) != 1 || !strings.Contains(got[0], " V003 ") || !strings.HasSuffix(got[0], want) {
			t.Errorf("%s: %v", tag, got)
		}
	}
	when := writeTemp(t, "app.tui", "<tui>\n  <keymap><bind keys=\"x\" action=\"go\" when=\"use\"/></keymap>\n  <screen id=\"main\"/>\n</tui>")
	if got := vdiags(t, when, ""); len(got) != 1 || !strings.Contains(got[0], " V003 ") || !strings.Contains(got[0], `<use> is not a node and never matches`) {
		t.Errorf(`when="use": %v`, got)
	}
	app, data := composeFixtures+"t138/app.tui", composeFixtures+"t138/d.json"
	rules := `<style>#bill { bold: true; } #bill__row > text { bold: true; } #main > row { italic: true; }</style>`
	f := variant(t, app, `<style>#bill__in { bold: true; }</style>`, rules)
	sameLines(t, "validate", vdiags(t, f, data), []string{})
	code, out, errw := runCLI("dump", f, "--data", data, "--styles", "--format", "json", "--cols", "20", "--rows", "4")
	if code != 0 {
		t.Fatalf("dump: exit %d %s", code, errw)
	}
	var d struct {
		Nodes []struct {
			ID   string `json:"id"`
			Tag  string `json:"tag"`
			Text string `json:"text"`
			Y    int    `json:"y"`
		} `json:"nodes"`
		Styles [][]struct {
			X, W int
			A    []string
		} `json:"styles"`
	}
	if err := json.Unmarshal([]byte(out), &d); err != nil {
		t.Fatal(err)
	}
	attrs := func(y int) []string {
		var a []string
		for _, s := range d.Styles[y] {
			if s.X == 0 {
				a = s.A
			}
		}
		return a
	}
	// Each label starts its row at x 0: its span there holds its look.
	seen := 0
	for _, n := range d.Nodes {
		if n.Tag != "text" {
			continue
		}
		a := strings.Join(attrs(n.Y), " ")
		seen++
		switch n.Text {
		case "Billing":
			if a != "bold italic" {
				t.Errorf("Billing: %q, want bold italic", a)
			}
		case "Shipping":
			if a != "italic" {
				t.Errorf("Shipping: %q, want italic", a)
			}
		}
	}
	if seen != 2 {
		t.Errorf("%d texts, want the two labels", seen)
	}
}

// 217. A static class on a use lands on the top-level elements of its
// copy, after their own and an inner use's, each name once.
func TestV04bTemplate217UseClass(t *testing.T) {
	dir := composeFixtures + "t217/"
	cls := dir + "cls.tui"
	sameLines(t, "validate", vdiags(t, cls, ""), []string{})
	nodes, _ := dumpNodes(t, cls, "--cols", "20", "--rows", "6")
	var got []string
	for _, n := range nodes[1:] {
		b, _ := json.Marshal(n["classes"])
		got = append(got, n["tag"].(string)+string(b))
	}
	sameLines(t, "classes", got, []string{
		`button["k-btn","danger","wide"]`, `text["k-note","inner","danger","wide"]`, `box["k-box","danger","wide"]`, `textnull`,
		`button["k-btn","danger"]`, `text["k-note","inner"]`, `box["k-box"]`, `textnull`,
	})
	run := func(f string) string {
		code, out, errw := runCLI("dump", f, "--styles", "--format", "json", "--cols", "20", "--rows", "6")
		if code != 0 {
			t.Fatalf("dump %s: exit %d %s", f, code, errw)
		}
		return out
	}
	base := run(cls)
	if run(dir+"flat.tui") != base {
		t.Errorf("the hand expansion's dump differs")
	}
	guard := variant(t, cls, `<use id="b" template="pair"/>`, `<use id="b" template="pair" class:x="p"/>`)
	sameLines(t, "class:x", vdiags(t, guard, ""), []string{
		`error V002 app.tui:16:33 /screen/use id=b attribute "class:x" is not allowed on <use>: a <use> is not a node; give this copy a static class="…" and style that class, or put the attribute on the template's element`,
	})
	if nodes, _ := dumpNodes(t, guard, "--cols", "20", "--rows", "6"); len(nodes) != 9 || !reflect.DeepEqual(nodes[5]["classes"], []any{"k-btn", "danger"}) {
		t.Errorf("the copy with class:x: %v", nodes)
	}
	if run(variant(t, cls, `<use id="b" template="pair"/>`, `<use id="b" template="pair" class=""/>`)) != base {
		t.Errorf(`class="" changed the dump`)
	}
}

// A <select> in a template: its bind, each, and placeholder go through the
// with alias, its label stays in its own each alias, and the copy works as
// the hand expansion does.
func TestV04bTemplateSelectInACopy(t *testing.T) {
	dir := composeFixtures + "tselect/"
	data := dir + "d.json"
	sameLines(t, "validate", vdiags(t, dir+"app.tui", data), []string{})
	play := func(f string) string {
		code, out, errw := runCLI("play", dir+f, "--data", data, "--input", "enter down enter", "--format", "json", "--cols", "30", "--rows", "8", "--cells", "--styles")
		if code != 0 {
			t.Fatalf("play %s: exit %d %s", f, code, errw)
		}
		return out
	}
	out := play("app.tui")
	if out != play("flat.tui") {
		t.Errorf("the copy plays differently from the hand expansion")
	}
	p := decodePlay(t, out)
	if len(p.Events) != 1 || p.Events[0].Action != "picked" || p.Events[0].Source != "tz__s" || p.Events[0].Value != "b" ||
		!reflect.DeepEqual(p.Events[0].Keys, map[string]any{"o": "b"}) {
		t.Errorf("events %+v", p.Events)
	}
	opened, _ := dumpNodes(t, dir+"app.tui", "--data", data, "--cols", "30", "--rows", "8")
	if len(opened) != 2 || opened[1]["id"] != "tz__s" {
		t.Errorf("closed select: %v", opened)
	}
	_, out, _ = runCLI("play", dir+"app.tui", "--data", data, "--input", "enter", "--format", "json", "--cols", "30", "--rows", "8")
	var options []string
	for _, n := range decodePlay(t, out).Nodes {
		if n.Tag == "text" {
			options = append(options, n.Text)
		}
	}
	if !reflect.DeepEqual(options, []string{"Alpha", "Beta"}) {
		t.Errorf("options %v", options)
	}
}

// SPEC v0.4b §24 tests of the binding semantics of copies (phase F3): 141,
// 144, 149, 218, and 227. The Go API parts of 141 and 144 are the root
// package's conformance_v04b_templates_test.go.

// strictDiags runs validate --json --strict on file with data and returns
// its diagnostics as vdiags does.
func strictDiags(t *testing.T, file, data string) []string {
	t.Helper()
	_, out, errw := runCLI("validate", file, "--json", "--strict", "--data", data)
	var v struct {
		Diagnostics []ir.Diagnostic `json:"diagnostics"`
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("validate %s: %v\n%s%s", file, err, out, errw)
	}
	got := []string{}
	for _, d := range v.Diagnostics {
		got = append(got, d.Severity+" "+d.Code+" "+d.File+":"+strconv.Itoa(d.Line)+":"+strconv.Itoa(d.Col)+" "+d.Path+" id="+d.ID+" "+d.Msg)
	}
	return got
}

// 141. with: a missing with path is one B003 at the use (an error under
// --strict), and the copy's paths through it, the nested with included,
// report nothing; the messages of paths through an alias name the path it
// stands for and the use; the V011 bullets. The Play/Get bullet and the
// use-site aliases bullet are the root package's.
func TestV04bTemplate141With(t *testing.T) {
	dir := composeFixtures + "t141/"
	app := dir + "app.tui"
	sameLines(t, "validate", vdiags(t, app, dir+"d.json"), []string{
		`warning B003 app.tui:16:5 /screen/use id=ship with path "order.ship" (alias a) is missing`,
	})
	sameLines(t, "--strict", strictDiags(t, app, dir+"d.json"), []string{
		`error B003 app.tui:16:5 /screen/use id=ship with path "order.ship" (alias a) is missing`,
	})
	inputsOf := func(nodes []map[string]any) []string {
		var out []string
		for _, n := range nodes {
			if n["tag"] == "input" {
				out = append(out, n["id"].(string))
			}
		}
		return out
	}
	nodes, focus := dumpNodes(t, app, "--data", dir+"d.json")
	if got := inputsOf(nodes); !reflect.DeepEqual(got, []string{"bill__zip__in", "ship__zip__in"}) || focus != "bill__zip__in" {
		t.Errorf("inputs %v, focus %v", got, focus)
	}
	sameLines(t, "both", vdiags(t, app, dir+"both.json"), []string{
		`warning B003 app.tui:4:7 /screen/col/row/text id= bind path "f.label" is missing (order.bill.zip.label via <use id="bill__zip">)`,
		`warning B003 app.tui:4:7 /screen/col/row/text id= bind path "f.label" is missing (order.ship.zip.label via <use id="ship__zip">)`,
		`warning B003 app.tui:5:7 /screen/col/row/input id=bill__zip__in bind path "f.value" is missing (order.bill.zip.value via <use id="bill__zip">)`,
		`warning B003 app.tui:5:7 /screen/col/row/input id=ship__zip__in bind path "f.value" is missing (order.ship.zip.value via <use id="ship__zip">)`,
	})

	// V011: one at the use, which inserts nothing (the dump keeps the
	// inputs of bill and ship only).
	const last = "  </screen>"
	for with, msg := range map[string]string{
		"x as f,y as g":   `with="x as f,y as g": want "path as alias", separated by ", "`,
		"x as f,  y as g": `with="x as f,  y as g": want "path as alias", separated by ", "`,
		"x as f, ":        `with="x as f, ": want "path as alias", separated by ", "`,
		"x as f,, y as g": `with="x as f,, y as g": want "path as alias", separated by ", "`,
		"":                `with="": want "path as alias", separated by ", "`,
		"x as f, y as f":  `with alias "f" appears twice`,
	} {
		v := variant(t, app, last, "    <use id=\"u\" template=\"field\" with=\""+with+"\"/>\n"+last)
		sameLines(t, "with="+with, vdiags(t, v, ""), []string{`error V011 app.tui:17:5 /screen/use id=u ` + msg})
		if nodes, _ := dumpNodes(t, v); !reflect.DeepEqual(inputsOf(nodes), []string{"bill__zip__in", "ship__zip__in"}) {
			t.Errorf("with=%q inserted something: %v", with, inputsOf(nodes))
		}
	}
	list := writeTemp(t, "app.tui", `<tui><template id="t"><text>{r.city}</text></template><screen id="main"><list id="l" each="rows as r" key="r.id"><item><use template="t" with="r.addr as r"/></item></list></screen></tui>`)
	if got := vdiags(t, list, ""); len(got) != 1 || !strings.HasPrefix(got[0], "error V011 ") || !strings.HasSuffix(got[0], ` with alias "r" repeats an alias in scope`) {
		t.Errorf("a with alias repeating a row alias: %v", got)
	}
	raw, err := os.ReadFile(app)
	if err != nil {
		t.Fatal(err)
	}
	src := strings.Replace(string(raw), "  <screen id=\"main\">", "  <template id=\"t2\"><box each=\"a.items as a\"><text>{a}</text></box></template>\n  <screen id=\"main\">", 1)
	each := writeTemp(t, "app.tui", strings.Replace(src, last, "    <use id=\"u\" template=\"t2\" with=\"data as a\"/>\n"+last, 1))
	sameLines(t, "each alias", vdiags(t, each, ""), []string{
		`error V011 app.tui:14:21 /screen/box id= each alias "a" repeats the with alias of <use id="u"> at app.tui:18:5`,
	})
	nodes, _ = dumpNodes(t, each, "--data", writeTemp(t, "d.json", `{"data": {"items": ["p", "q"]}}`))
	var got []string
	for _, n := range nodes {
		if s, ok := n["text"].(string); ok && n["tag"] == "text" && (s == "p" || s == "q") {
			got = append(got, s)
		}
	}
	if !reflect.DeepEqual(got, []string{"p", "q"}) {
		t.Errorf("the copy of t2: texts %v", got)
	}
}

// 144. if on a use: the copy is in the frame while the guard holds, and a
// missing path is one B002 at the use, read where the use stands (b is
// not in scope).
func TestV04bTemplate144UseIf(t *testing.T) {
	dir := composeFixtures + "t144/"
	app, data := dir+"app.tui", dir+"d.json"
	sameLines(t, "validate", vdiags(t, app, data), []string{})
	nodesOf := func(what string, args ...string) {
		t.Helper()
		want := args[len(args)-1]
		_, out, _ := runCLI(append([]string{args[0], app, "--data", data, "--cols", "10", "--rows", "3", "--format", "json"}, args[1:len(args)-1]...)...)
		var d struct {
			Nodes any `json:"nodes"`
		}
		var w any
		if err := json.Unmarshal([]byte(out), &d); err != nil {
			t.Fatal(err)
		}
		_ = json.Unmarshal([]byte(want), &w)
		if !reflect.DeepEqual(d.Nodes, w) {
			t.Errorf("%s: nodes %v", what, d.Nodes)
		}
	}
	nodesOf("dump", "dump", `[{"id": "main", "tag": "screen", "x": 0, "y": 0, "w": 10, "h": 3}, {"tag": "text", "x": 0, "y": 0, "w": 10, "h": 1, "text": "after"}]`)
	nodesOf("play", "play", "--input", "set:show=true", `[{"id": "main", "tag": "screen", "x": 0, "y": 0, "w": 10, "h": 3}, {"id": "top__msg", "tag": "text", "x": 0, "y": 0, "w": 10, "h": 1, "text": "hi"}, {"tag": "text", "x": 0, "y": 1, "w": 10, "h": 1, "text": "after"}]`)
	v := variant(t, app, `if="show"`, `if="b.text"`)
	sameLines(t, "if=b.text", vdiags(t, v, data), []string{`warning B002 app.tui:6:5 /screen/use id=top if="b.text": path "b.text" is missing`})
	if nodes, _ := dumpNodes(t, v, "--data", data, "--cols", "10", "--rows", "3"); len(nodes) != 2 {
		t.Errorf("if=b.text: the copy is in the frame: %v", nodes)
	}
}

// 149. Once per written element: the V002 of the template's text and the
// B003 of the two copies through b, which say the same thing, are one
// each; no V012, since the template has no id.
func TestV04bTemplate149OncePerElement(t *testing.T) {
	dir := composeFixtures + "t149/"
	sameLines(t, "validate", vdiags(t, dir+"app.tui", dir+"d.json"), []string{
		`warning B003 app.tui:3:5 /screen/text id= bind path "x.name" is missing (b.name via <use template="t">)`,
		`error V002 app.tui:3:11 /screen/text id= attribute "bogus" is not allowed on <text>`,
	})
}

// 218. Optional flags through with: a flag read through an alias that
// stands for an object is null when missing, open included, while text
// keeps B003; a missing alias path is the use's B003; an alias that
// stands for anything else gives each flag its B002; a store flag is an
// ordinary one.
func TestV04bTemplate218OptionalFlags(t *testing.T) {
	dir := composeFixtures + "t218/"
	opt := dir + "opt.tui"
	texts := func(data string) []string {
		t.Helper()
		nodes, _ := dumpNodes(t, opt, "--data", data, "--cols", "30", "--rows", "6")
		var out []string
		for _, n := range nodes {
			switch n["tag"] {
			case "text":
				b, _ := json.Marshal(n["classes"])
				out = append(out, n["text"].(string)+" "+string(b))
			case "modal":
				out = append(out, "modal "+n["id"].(string))
			}
		}
		return out
	}
	sameLines(t, "validate", vdiags(t, opt, dir+"opt.json"), []string{})
	sameLines(t, "dump", texts(dir+"opt.json"), []string{`Email ["k-label"]`, `We never share it null`, `Email! null`})

	data := func(more string) string {
		return writeTemp(t, "opt.json", `{"form": {"email": {"label": "Email", "help": "We never share it"`+more+`}}}`)
	}
	withError := data(`, "error": "Not an email"`)
	sameLines(t, "error", vdiags(t, opt, withError), []string{})
	sameLines(t, "error dump", texts(withError), []string{`Email ["k-label","invalid"]`, `Not an email null`, `Email! null`})
	open := data(`, "open": true`)
	sameLines(t, "open", vdiags(t, opt, open), []string{})
	sameLines(t, "open dump", texts(open), []string{`Email ["k-label"]`, `We never share it null`, `Email! null`, `modal hint__m`, `We never share it null`})

	sameLines(t, "no help", vdiags(t, opt, writeTemp(t, "opt.json", `{"form": {"email": {"label": "Email"}}}`)), []string{
		`warning B003 opt.tui:5:7 /screen/col/text id= bind path "f.help" is missing (form.email.help via <use id="email">)`,
	})
	sameLines(t, "{}", vdiags(t, opt, writeTemp(t, "opt.json", `{}`)), []string{
		`warning B003 opt.tui:14:5 /screen/use id=email with path "form.email" (alias f) is missing`,
		`warning B003 opt.tui:15:5 /screen/use id=hint with path "form.email" (alias f) is missing`,
	})
	sameLines(t, "a string", vdiags(t, opt, writeTemp(t, "opt.json", `{"form": {"email": "x"}}`)), []string{
		`warning B002 opt.tui:4:7 /screen/col/text id= class:invalid="f.error": path "f.error" is missing (form.email.error via <use id="email">)`,
		`warning B003 opt.tui:4:7 /screen/col/text id= bind path "f.label" is missing (form.email.label via <use id="email">)`,
		`warning B002 opt.tui:5:7 /screen/col/text id= if="!f.error": path "f.error" is missing (form.email.error via <use id="email">)`,
		`warning B003 opt.tui:5:7 /screen/col/text id= bind path "f.help" is missing (form.email.help via <use id="email">)`,
		`warning B002 opt.tui:6:7 /screen/col/text id= if="f.error": path "f.error" is missing (form.email.error via <use id="email">)`,
		`warning B002 opt.tui:7:7 /screen/col/text id= hidden="f.meta.hide": path "f.meta.hide" is missing (form.email.meta.hide via <use id="email">)`,
		`warning B003 opt.tui:7:7 /screen/col/text id= bind path "f.label" is missing (form.email.label via <use id="email">)`,
		`warning B002 opt.tui:11:5 /screen/modal id=hint__m open="f.open": path "f.open" is missing (form.email.open via <use id="hint">)`,
	})
	raw, err := os.ReadFile(opt)
	if err != nil {
		t.Fatal(err)
	}
	store := writeTemp(t, "opt.tui", strings.Replace(string(raw), `hidden="f.meta.hide"`, `hidden="debug"`, 1))
	sameLines(t, "a store flag", vdiags(t, store, dir+"opt.json"), []string{
		`warning B002 opt.tui:7:7 /screen/col/text id= hidden="debug": path "debug" is missing, placed by <use id="email"> at opt.tui:14:5`,
	})
}

// 227. A use's guard in a list row: each row is built again when the
// guard flips (if, and hidden the other way round), and a missing guard
// path is one B002 at the use, not one per row.
func TestV04bTemplate227UseGuardInRow(t *testing.T) {
	dir := composeFixtures + "t227/"
	rows, data := dir+"rows.tui", dir+"rows.json"
	plain := []string{"alpha               ", "beta                "}
	shown := []string{"alpha [new]         ", "beta [old]          "}
	check := func(what, file string) {
		t.Helper()
		code, out, errw := runCLI("play", file, "--data", data, "--cols", "20", "--rows", "2", "--frames", "--input", "set:show=true set:show=false", "--format", "json")
		if code != 0 {
			t.Fatalf("%s: play exit %d %s", what, code, errw)
		}
		p := decodePlay(t, out)
		if len(p.Events) != 0 || len(p.Frames) != 3 {
			t.Fatalf("%s: events %v, %d frames", what, p.Events, len(p.Frames))
		}
		for i, want := range [][]string{plain, shown, plain} {
			f := p.Frames[i].Dump
			badges := 0
			for _, n := range f.Nodes {
				if n.Tag == "text" && reflect.DeepEqual(n.Classes, []string{"k-badge"}) {
					badges++
				}
			}
			if !reflect.DeepEqual(f.Grid, want) || badges != 2*(i%2) {
				t.Errorf("%s step %d: grid %q, %d badges", what, i, f.Grid, badges)
			}
		}
	}
	check("if", rows)
	check("hidden", variant(t, rows, `if="show"`, `hidden="!show"`))
	sameLines(t, "no show", vdiags(t, rows, writeTemp(t, "rows.json", `{"rows": [{"id": "a", "name": "alpha", "tag": "new"}, {"id": "b", "name": "beta", "tag": "old"}]}`)), []string{
		`warning B002 rows.tui:10:11 /screen/list/item/row/use id= if="show": path "show" is missing`,
	})
}
