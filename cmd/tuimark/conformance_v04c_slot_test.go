package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/host"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// SPEC v0.4c §24 tests of the slot (phase C2), the parts that go through
// the CLI: the validate lists of 241, 242, 244, 245, 251, 255, 257-259,
// and 263, and the dumps, classes, and ids they name. The Go API parts
// (equal frames, the charges of test 248, and test 260) are the root
// package's conformance_v04c_slot_test.go. The fixtures are
// testdata/v04c/<test>. The via entries of a fill (242's inspect, 243,
// 256) are phase C3's.

// screenTree is the screen main of file as the parser builds it, its
// fills placed: each node as KIND, #ID when it has one, :TEXT for a text,
// then its children in parentheses.
func screenTree(t *testing.T, file string) string {
	t.Helper()
	h, err := host.Load(file)
	if err != nil {
		t.Fatal(err)
	}
	var walk func(n *ir.Node) string
	walk = func(n *ir.Node) string {
		s := n.Kind
		if n.ID != "" {
			s += "#" + n.ID
		}
		if n.Kind == "text" {
			s += ":" + n.Text
		}
		if len(n.Children) > 0 {
			var kids []string
			for _, c := range n.Children {
				kids = append(kids, walk(c))
			}
			s += "(" + strings.Join(kids, " ") + ")"
		}
		return s
	}
	for _, n := range h.Document().Root.Children {
		if n.Kind == "screen" && n.ID == "main" {
			return walk(n)
		}
	}
	t.Fatalf("%s has no screen main", file)
	return ""
}

// idsOf is the id of each node of a dump that has one, in order.
func idsOf(nodes []map[string]any) []string {
	out := []string{}
	for _, n := range nodes {
		if id, _ := n["id"].(string); id != "" {
			out = append(out, id)
		}
	}
	return out
}

// 241. The slot, and a use's class: the fill is not the template's, so a
// use's class reaches only what its template's body writes.
func TestV04cSlot241Classes(t *testing.T) {
	dir := propsFixtures + "241/"
	data := dir + "d.json"
	sameLines(t, "validate", vdiags(t, dir+"app.tui", data), []string{})
	sameLines(t, "hand.tui", vdiags(t, dir+"hand.tui", data), []string{})
	nodes, _ := dumpNodes(t, dir+"app.tui", "--data", data, "--cols", "20", "--rows", "6")
	var got []string
	for _, n := range nodes {
		if n["tag"] == "text" || n["tag"] == "box" {
			cs, _ := n["classes"].([]any)
			var names []string
			for _, c := range cs {
				names = append(names, c.(string))
			}
			text, _ := n["text"].(string)
			got = append(got, n["tag"].(string)+":"+text+" "+strings.Join(names, " "))
		}
	}
	sameLines(t, "classes", got, []string{"text:a f", "text:o mine top", "box: panel p"})
	code, out, errw := runCLI("inspect", dir+"app.tui", "--data", data, "--cols", "20", "--rows", "6", "--at", "0,2", "--json")
	if code != 0 {
		t.Fatalf("inspect: exit %d %s", code, errw)
	}
	m := jsonMap(t, out)
	node := m["node"].(map[string]any)
	if node["tag"] != "box" || node["y"] != float64(2) || !reflect.DeepEqual(m["pseudo"], []any{"empty"}) {
		t.Errorf("inspect at 0,2: node %v, pseudo %v", node, m["pseudo"])
	}
}

// 242. A fill belongs where it is written: its ids are not scoped, its
// paths are the store's, and a focus= in it names its own nodes. (The
// inspect via entry of the fill is phase C3's; the source is checked.)
func TestV04cSlot242FillBelongsWhereWritten(t *testing.T) {
	dir := propsFixtures + "242/"
	data := dir + "d.json"
	sameLines(t, "validate", vdiags(t, dir+"app.tui", data), []string{})
	nodes, _ := dumpNodes(t, dir+"app.tui", "--data", data)
	sameLines(t, "ids", idsOf(nodes), []string{"main", "c__rows", "c__x", "ft", "one", "x"})
	var order []string
	for _, s := range textsOf(nodes) {
		switch s {
		case "i1", "L", "store-t", "store-r":
			order = append(order, s)
		}
	}
	sameLines(t, "texts", order, []string{"i1", "L", "store-t", "store-r"})
	code, out, errw := runCLI("inspect", dir+"app.tui", "--data", data, "--id", "x", "--json")
	if code != 0 {
		t.Fatalf("inspect: exit %d %s", code, errw)
	}
	src := jsonMap(t, out)["source"].(map[string]any)
	if src["file"] != "app.tui" || src["line"] != float64(17) || src["col"] != float64(11) {
		t.Errorf("inspect --id x: source %v", src)
	}
}

// 244. Slot diagnostics.
func TestV04cSlot244Diagnostics(t *testing.T) {
	dir := propsFixtures + "244/"
	const row = `<slot/> cannot stand where content repeats per row: its content would need the row's alias (an each template, a <table>, a <list each=…>)`
	const item = `<slot/> cannot stand inside a list <item>: a list's rows hold no focus or state from outside the list`
	sameLines(t, "slots.tui", vdiags(t, dir+"slots.tui", ""), []string{
		`error V019 slots.tui:4:5 /template/slot id= <template id="two"> has more than one <slot/> (the first is at 3:10)`,
		`error V001 slots.tui:7:47 /template/list/item/slot id= ` + item,
		`error V001 slots.tui:8:25 /template/col/slot id= ` + row,
		`error V001 slots.tui:9:73 /template/table/slot id= ` + row,
		`error V013 slots.tui:10:12 /template/text id= <text> cannot contain elements (found <slot>)`,
		`error V002 slots.tui:11:11 /template/slot id= attribute "name" is not allowed on <slot>`,
		`error V013 slots.tui:11:20 /template/slot id= <slot/> takes no content: the content of the <use> fills it`,
		`error V001 slots.tui:14:5 /screen/slot id= <slot/> is only valid inside a <template>: it marks where the content of a <use> goes`,
	})
	sameLines(t, "content.tui", vdiags(t, dir+"content.tui", ""), []string{
		`error V013 content.tui:5:26 /screen/use id= <template id="bare"> has no <slot/>`,
		`error V013 content.tui:6:25 /screen/use id= text is not allowed directly inside <use>; wrap it in <text>`,
		`error V019 content.tui:7:5 /screen/use id= <use template="missing"> names no <template>`,
		`error V013 content.tui:11:26 /screen/include id= <include> takes no content; to wrap content, make the fragment's markup a <template> with a <slot/> and place it with <use>`,
	})
	if got, want := screenTree(t, dir+"content.tui"), "screen#main(text:b box(text:y) box box)"; got != want {
		t.Errorf("content.tui: %s, want %s", got, want)
	}
	sameLines(t, "placed.tui", vdiags(t, dir+"placed.tui", ""), []string{
		`error V016 placed.tui:4:26 /screen/box/tab id=t <tab> must be a direct child of <tabs>`,
		`error L004 placed.tui:5:26 /screen/box/modal id=m <modal> must be a direct child of <screen> (and come last)`,
	})
	sameLines(t, "pinned.tui", vdiags(t, dir+"pinned.tui", ""), []string{
		`error V001 pinned.tui:3:5 /screen/slot id= tag <slot> (requires a <tui> without version)`,
	})
	// A stylesheet that names slot, as test 155 checks it for use.
	sheet := writeTemp(t, "app.tui", "<tui>\n  <style src=\"s.tcss\"/>\n  <screen id=\"main\"><text>t</text></screen>\n</tui>\n")
	if err := os.WriteFile(filepath.Join(filepath.Dir(sheet), "s.tcss"), []byte("slot { bold: true; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := vdiags(t, sheet, "")
	if len(got) != 1 || !strings.HasPrefix(got[0], "error V003 s.tcss:") || !strings.HasSuffix(got[0], ` selector "slot": <slot> is not a node and never matches`) {
		t.Errorf("slot in a stylesheet: %v", got)
	}
	sameLines(t, "item.tui", vdiags(t, dir+"item.tui", ""), []string{
		`error V001 item.tui:3:38 /template/list/item/slot id= ` + item,
	})
	if got, want := screenTree(t, dir+"item.tui"), "screen#main(list#u__m(item(text:x)) list#u__n(item(text:a)))"; got != want {
		t.Errorf("item.tui: %s, want %s", got, want)
	}
}

// 245. The ids of a fill: a template's body that writes one in a use's
// content holds it; a fill written in the document keeps its own.
func TestV04cSlot245Ids(t *testing.T) {
	dir := propsFixtures + "245/"
	data := dir + "d.json"
	sameLines(t, "validate", vdiags(t, dir+"app.tui", data), []string{
		`error V012 app.tui:14:5 /screen/use id= <use template="holder"> needs an id: its template holds ids (#in), which the use scopes as ID__in`,
		`error V004 app.tui:16:5 /screen/text id=q duplicate id "q" (first defined at 13:27)`,
	})
	nodes, _ := dumpNodes(t, dir+"app.tui", "--data", data)
	sameLines(t, "ids", idsOf(nodes), []string{"main", "q", "h__in", "q"})
	var digits []string
	for _, s := range textsOf(nodes) {
		if s == "1" || s == "2" {
			digits = append(digits, s)
		}
	}
	sameLines(t, "texts", digits, []string{"1", "2"})
}

// 251. Content only, in a use's content.
func TestV04cSlot251ContentOnly(t *testing.T) {
	dir := propsFixtures + "251/"
	const fills = `cannot be inside a <use>: its content fills a <slot/>, which holds content only`
	sameLines(t, "validate", vdiags(t, dir+"app.tui", ""), []string{
		`error V001 app.tui:4:23 /template/use/style id= <style> cannot be inside a <template>: a template holds content only`,
		`error V001 app.tui:7:5 /screen id= <screen> ` + fills,
		`error V001 app.tui:8:5 /keymap id= <keymap> ` + fills,
		`error V001 app.tui:13:12 /screen/box/template id= <template> ` + fills,
		`error V001 app.tui:14:7 /screen/bind id= <bind> ` + fills,
	})
	if got, want := screenTree(t, dir+"app.tui"), "screen#main(text:a box)"; got != want {
		t.Errorf("the screen: %s, want %s", got, want)
	}
	h, err := host.Load(dir + "app.tui")
	if err != nil {
		t.Fatal(err)
	}
	doc := h.Document()
	var screens []string
	for _, n := range doc.Root.Children {
		if n.Kind == "screen" {
			screens = append(screens, n.ID)
		}
	}
	sameLines(t, "screens", screens, []string{"main"})
	for _, k := range doc.Keymap {
		if k.KeysRaw == "x" {
			t.Errorf("the keymap has a row for x: %+v", k)
		}
	}
	if _, has := doc.IDs["inner"]; has {
		t.Errorf("there is a template inner")
	}
}

// 255. The content of a use: includes, ignored content, and cycles.
func TestV04cSlot255Content(t *testing.T) {
	dir := propsFixtures + "255/"
	const cycle = `<use template="w"> in <template id="w"> makes a cycle of templates: w → w`
	sameLines(t, "validate", vdiags(t, dir+"app.tui", ""), []string{
		`error V019 app.tui:5:7 /template/box/use id= ` + cycle,
		`error V019 app.tui:5:25 /template/box/use/use id= ` + cycle,
		`error V013 app.tui:14:26 /screen/use id= <template id="bare"> has no <slot/>`,
		`error V001 slotted.tui:2:3 /template/use/slot id= <slot/> cannot come from an <include> in the content of a <use>; write it in the content itself, placed by <include> at app.tui:10:23`,
	})
	if got, want := screenTree(t, dir+"app.tui"), "screen#main(box(text:p) text:b box(box) box)"; got != want {
		t.Errorf("app.tui: %s, want %s", got, want)
	}
	got := vdiags(t, dir+"def.tui", "")
	if len(got) != 2 || !strings.HasPrefix(got[0], `error V006 def.tui:4:26 /template/use/include id= include src="missing.tui" cannot be read: `) ||
		got[1] != `error V013 def.tui:4:26 /screen/use id= <template id="bare"> has no <slot/>` {
		t.Errorf("def.tui: %v", got)
	}
	if got, want := screenTree(t, dir+"def.tui"), "screen#main(text:b text:b)"; got != want {
		t.Errorf("def.tui: %s, want %s", got, want)
	}
}

// 257. A fill's paths in a template's body read the aliases in scope
// where it is written.
func TestV04cSlot257FillPaths(t *testing.T) {
	dir := propsFixtures + "257/"
	data := dir + "d.json"
	sameLines(t, "validate", vdiags(t, dir+"app.tui", data), []string{
		`warning B003 app.tui:9:26 /screen/box/text id= bind path "b.nope" is missing (m.nope via <use id="u">)`,
	})
	code, out, errw := runCLI("dump", dir+"app.tui", "--data", data, "--format", "json")
	if code != 0 {
		t.Fatalf("dump: exit %d %s", code, errw)
	}
	grid := jsonMap(t, out)["grid"].([]any)
	if !strings.HasPrefix(grid[0].(string), "S/x1 ") || !strings.HasPrefix(grid[1].(string), "S/x2 ") {
		t.Errorf("grid: %q", grid[:2])
	}
}

// 258. A use's if and a forwarded fill: the inner use's if prunes the
// fill with its copy.
func TestV04cSlot258IfForwarded(t *testing.T) {
	dir := propsFixtures + "258/"
	for _, c := range []struct {
		data  string
		texts []string
	}{
		{"hide.json", []string{"end"}},
		{"show.json", []string{"f", "end"}},
	} {
		sameLines(t, "validate "+c.data, vdiags(t, dir+"app.tui", dir+c.data), []string{})
		nodes, _ := dumpNodes(t, dir+"app.tui", "--data", dir+c.data)
		sameLines(t, "texts "+c.data, textsOf(nodes), c.texts)
	}
}

// 259. A fill's includes resolve where it is written, on the chain of
// includes of that place.
func TestV04cSlot259FillIncludes(t *testing.T) {
	dir := propsFixtures + "259/"
	sameLines(t, "validate", vdiags(t, dir+"app.tui", ""), []string{
		`error V006 g.tui:2:21 /screen/box/include id= include src="g.tui" is an include cycle: app.tui → g.tui → g.tui`,
	})
	if got, want := screenTree(t, dir+"app.tui"), "screen#main(box(text:p) box)"; got != want {
		t.Errorf("app.tui: %s, want %s", got, want)
	}
}

// 263. A fragment checked on its own: a use's content gets its own
// elements' checks, and the has-no-slot V013 only when the template's
// body reads no include.
func TestV04cSlot263FragmentOnItsOwn(t *testing.T) {
	dir := propsFixtures + "263/"
	kit := []string{
		`error V002 kit.tui:8:11 /use/text id= attribute "bogus" is not allowed on <text>`,
		`error V001 kit.tui:9:5 /use/zzz id= unknown tag <zzz>`,
	}
	sameLines(t, "kit.tui", vdiags(t, dir+"kit.tui", ""), kit)
	if code, _, _ := runCLI("validate", dir+"kit.tui"); code != 2 {
		t.Errorf("kit.tui: exit %d", code)
	}
	// The same without card-slot.tui: the include is not read.
	raw, err := os.ReadFile(dir + "kit.tui")
	if err != nil {
		t.Fatal(err)
	}
	sameLines(t, "kit.tui alone", vdiags(t, writeTemp(t, "kit.tui", string(raw)), ""), kit)
	sameLines(t, "kit2.tui", vdiags(t, dir+"kit2.tui", ""), []string{
		`error V013 kit2.tui:8:5 /use id= <template id="card"> has no <slot/>`,
		`error V002 kit2.tui:8:11 /use/text id= attribute "bogus" is not allowed on <text>`,
		`error V001 kit2.tui:9:5 /use/zzz id= unknown tag <zzz>`,
	})
	if code, _, _ := runCLI("validate", dir+"kit2.tui"); code != 2 {
		t.Errorf("kit2.tui: exit %d", code)
	}
}

// ir and inspect read a document with fills without failing; the shape of
// a fill in IR 0.4 and inspect's via entries are phase C3's.
func TestV04cSlotToolsReadFills(t *testing.T) {
	for _, f := range []string{"241/app.tui", "242/app.tui", "257/app.tui", "258/app.tui"} {
		if code, _, errw := runCLI("ir", propsFixtures+f); code != 0 {
			t.Errorf("ir %s: exit %d %s", f, code, errw)
		}
	}
	if code, _, errw := runCLI("inspect", propsFixtures+"258/app.tui", "--data", propsFixtures+"258/show.json", "--at", "0,0", "--json"); code != 0 {
		t.Errorf("inspect 258: exit %d %s", code, errw)
	}
}
