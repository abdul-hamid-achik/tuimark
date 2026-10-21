package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/catalog"
)

// SPEC v0.4c §24 tests of the tools (phase C3): 235, 243, 246, 247, 249's
// catalog bullet, 250, 256, and the via entry of 242 (C2 closed the rest
// of 242). 249's language-server bullet is internal/lsp's
// conformance_v04c_test.go. The fixtures are testdata/v04c/<test>.

// viaIs checks the source member of inspect --json with args against
// want, members and their order included (SPEC §15.7).
func viaIs(t *testing.T, want string, args ...string) {
	t.Helper()
	got, code := inspectSource(t, args...)
	if code != 0 || got != compactJSON(t, want) {
		t.Errorf("inspect %v: exit %d\n got %s\nwant %s", args, code, got, compactJSON(t, want))
	}
}

// inspectTail is the last n lines of inspect's text output.
func inspectTail(t *testing.T, n int, args ...string) []string {
	t.Helper()
	code, out, errw := runCLI(append([]string{"inspect"}, args...)...)
	if code != 0 {
		t.Fatalf("inspect %v: exit %d %s", args, code, errw)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < n {
		return lines
	}
	return lines[len(lines)-n:]
}

// sameDumps checks that two documents give byte-identical JSON dumps with
// the extra args.
func sameDumps(t *testing.T, a, b string, args ...string) {
	t.Helper()
	dump := func(f string) string {
		code, out, errw := runCLI(append([]string{"dump", f, "--format", "json"}, args...)...)
		if code != 0 {
			t.Fatalf("dump %s %v: exit %d %s", f, args, code, errw)
		}
		return out
	}
	if da, db := dump(a), dump(b); da != db {
		t.Errorf("dumps of %s and %s differ with %v", a, b, args)
	}
}

// validateOK checks that validate reports nothing and prints ok.
func validateOK(t *testing.T, file string, args ...string) {
	t.Helper()
	code, out, errw := runCLI(append([]string{"validate", file}, args...)...)
	if code != 0 || out != "ok\n" {
		t.Errorf("validate %s %v: exit %d %q %s", file, args, code, out, errw)
	}
}

// irChild finds the child of an IR node with kind and id ("" for none).
func irChild(t *testing.T, n map[string]any, kind, id string) map[string]any {
	t.Helper()
	for _, c := range n["children"].([]any) {
		m := c.(map[string]any)
		if mid, _ := m["id"].(string); m["kind"] == kind && mid == id {
			return m
		}
	}
	t.Fatalf("no %s %q under %s %v", kind, id, n["kind"], n["id"])
	return nil
}

// onlyChild is the only child of an IR node.
func onlyChild(t *testing.T, n map[string]any) map[string]any {
	t.Helper()
	kids := n["children"].([]any)
	if len(kids) != 1 {
		t.Fatalf("%s %v has %d children, want 1", n["kind"], n["id"], len(kids))
	}
	return kids[0].(map[string]any)
}

// irWalk calls fn for every node of an IR tree.
func irWalk(n map[string]any, fn func(map[string]any)) {
	fn(n)
	for _, c := range n["children"].([]any) {
		irWalk(c.(map[string]any), fn)
	}
}

// 235. The detail tab of examples/containers, rewritten with k-kv, k-stat,
// and a k-card with a slot, equals the tab written by hand; a scroll in a
// fill keeps its id, and inspect names the slot that holds it.
func TestV04cTools235DetailTab(t *testing.T) {
	dir := propsFixtures + "235/"
	data := dir + "d.json"
	validateOK(t, dir+"app.tui", "--data", data)
	validateOK(t, dir+"hand.tui", "--data", data)
	for _, cols := range []string{"80", "120"} {
		sameDumps(t, dir+"app.tui", dir+"hand.tui", "--data", data, "--cols", cols, "--rows", "24")
	}
	code, out, errw := runCLI("dump", dir+"app.tui", "--data", data, "--cols", "80", "--rows", "24", "--format", "json")
	if code != 0 {
		t.Fatalf("dump: exit %d %s", code, errw)
	}
	m := jsonMap(t, out)
	if m["focus"] != "logs-view" {
		t.Errorf("focus %v", m["focus"])
	}
	var nodes []map[string]any
	for _, n := range m["nodes"].([]any) {
		nodes = append(nodes, n.(map[string]any))
	}
	sameLines(t, "ids", idsOf(nodes), []string{"main", "nav", "detail", "detail__page", "detail__top", "detail__stats", "dtabs", "logs", "logs-view"})
	grid := m["grid"].([]any)
	if got := string([]rune(grid[2].(string))[29:34]); got != "▃▅▇▄█" {
		t.Errorf("grid[2][29:34] = %q", got)
	}
	if got := string([]rune(grid[4].(string))[28:33]); got != "▃▃▃▃▃" {
		t.Errorf("grid[4][28:33] = %q", got)
	}
	// In ir, the Memory copy's sparkline has max 100, the CPU copy's none.
	maxes := map[string]any{}
	irWalk(irRoot(t, dir+"app.tui"), func(n map[string]any) {
		if n["kind"] != "use" || attrsOf(n)["template"] != "k-stat" {
			return
		}
		title := attrsOf(n)["title"].(string)
		irWalk(n, func(s map[string]any) {
			if s["kind"] == "sparkline" {
				maxes[title] = attrsOf(s)["max"]
			}
		})
	})
	if !reflect.DeepEqual(maxes, map[string]any{"CPU": nil, "Memory": "100"}) {
		t.Errorf("sparkline max by copy: %v", maxes)
	}

	// A scroll in a fill keeps its id.
	validateOK(t, dir+"fill.tui", "--data", data)
	if _, focus := dumpNodes(t, dir+"fill.tui", "--data", data); focus != "logs-view" {
		t.Errorf("fill.tui focus %v", focus)
	}
	viaIs(t, `{"file": "fill.tui", "line": 28, "col": 15, "via": [{"slot": "", "template": "k-card", "file": "kit.tui", "line": 21, "col": 7}]}`, dir+"fill.tui", "--data", data, "--id", "logs-view")
	sameLines(t, "inspect fill.tui text", inspectTail(t, 2, dir+"fill.tui", "--data", data, "--id", "logs-view"), []string{
		`source fill.tui:28:15`,
		`via <slot/> of <use template="k-card"> kit.tui:21:7`,
	})
}

// 242, its inspect bullet: the input x of the fill is placed by the slot of
// the copy c.
func TestV04cTools242FillVia(t *testing.T) {
	dir := propsFixtures + "242/"
	viaIs(t, `{"file": "app.tui", "line": 17, "col": 11, "via": [{"slot": "c", "template": "card", "file": "app.tui", "line": 8, "col": 7}]}`, dir+"app.tui", "--data", dir+"d.json", "--id", "x")
	sameLines(t, "inspect --id x text", inspectTail(t, 2, dir+"app.tui", "--data", dir+"d.json", "--id", "x"), []string{
		`source app.tui:17:11`,
		`via <slot/> of <use id="c" template="card"> app.tui:8:7`,
	})
}

// 243. Forwarding a slot: outer's fill goes through inner's slot, with
// neither use's id, class, or alias.
func TestV04cTools243ForwardedSlot(t *testing.T) {
	dir := propsFixtures + "243/"
	data := dir + "d.json"
	sameLines(t, "validate", vdiags(t, dir+"app.tui", data), []string{})
	sameDumps(t, dir+"app.tui", dir+"hand.tui", "--data", data, "--cols", "30", "--rows", "8")
	code, out, errw := runCLI("dump", dir+"app.tui", "--data", data, "--cols", "30", "--rows", "8", "--format", "json")
	if code != 0 {
		t.Fatalf("dump: exit %d %s", code, errw)
	}
	var got []string
	for _, x := range jsonMap(t, out)["nodes"].([]any) {
		n := x.(map[string]any)
		s, _ := n["tag"].(string)
		if id, _ := n["id"].(string); id != "" {
			s += "#" + id
		}
		if txt, _ := n["text"].(string); txt != "" {
			s += ":" + txt
		}
		if cl, _ := n["classes"].([]any); len(cl) > 0 {
			var names []string
			for _, c := range cl {
				names = append(names, c.(string))
			}
			s += "." + strings.Join(names, ".")
		}
		got = append(got, s)
	}
	sameLines(t, "nodes", got, []string{"screen#main", "col.out.oc", "text:T", "box.in.ic", "text:Name.head", "input#name:ada"})
	viaIs(t, `{"file": "app.tui", "line": 18, "col": 7, "via": [{"slot": "o", "template": "outer", "file": "app.tui", "line": 12, "col": 9}, {"slot": "o__i", "template": "inner", "file": "app.tui", "line": 4, "col": 7}]}`, dir+"app.tui", "--data", data, "--id", "name")
	sameLines(t, "inspect --id name text", inspectTail(t, 3, dir+"app.tui", "--data", data, "--id", "name"), []string{
		`source app.tui:18:7`,
		`via <slot/> of <use id="o" template="outer"> app.tui:12:9`,
		`via <slot/> of <use id="o__i" template="inner"> app.tui:4:7`,
	})
	// Any cell of the head text, which outer's body writes in inner's fill.
	for _, at := range []string{"0,1", "3,1", "29,1"} {
		viaIs(t, `{"file": "app.tui", "line": 11, "col": 9, "via": [{"slot": "o__i", "template": "inner", "file": "app.tui", "line": 4, "col": 7}, {"use": "o", "template": "outer", "file": "app.tui", "line": 17, "col": 5}]}`, dir+"app.tui", "--data", data, "--cols", "30", "--rows", "8", "--at", at)
	}
}

// 246. Props and the slot in IR 0.4: a definition keeps props, {$name},
// and its use's content as written; a copy is substituted, and its slot
// holds the fill, with from.
func TestV04cTools246IR(t *testing.T) {
	root := irRoot(t, propsFixtures+"246/app.tui") // validated against ir.v0.4.json
	raw, err := os.ReadFile("../../schema/ir.v0.4.json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"select", "slot"`) {
		t.Errorf(`schema/ir.v0.4.json has no "slot" in its kind enum`)
	}
	exact := func(what string, n map[string]any, want map[string]any) {
		t.Helper()
		if got := attrsOf(n); !reflect.DeepEqual(got, want) {
			t.Errorf("%s attrs %v, want %v", what, got, want)
		}
	}
	empty := func(what string, n map[string]any) {
		t.Helper()
		if kids := n["children"].([]any); len(kids) != 0 {
			t.Errorf("%s has %d children", what, len(kids))
		}
	}
	from := func(what string, n map[string]any, want string) {
		t.Helper()
		if n["from"] != want {
			t.Errorf("%s from %v, want %s", what, n["from"], want)
		}
	}
	card := irChild(t, root, "template", "card")
	exact("template card", card, map[string]any{"props": "title"})
	box := onlyChild(t, card)
	if box["kind"] != "box" {
		t.Fatalf("card's child is %v", box["kind"])
	}
	exact("card's box", box, map[string]any{"title": "{$title}"})
	slot := onlyChild(t, box)
	if slot["kind"] != "slot" {
		t.Fatalf("card's box holds %v", slot["kind"])
	}
	exact("card's slot", slot, map[string]any{})
	empty("card's slot", slot)

	use := onlyChild(t, irChild(t, root, "template", "wrap"))
	if use["kind"] != "use" || use["id"] != nil {
		t.Fatalf("wrap's child is %v %v", use["kind"], use["id"])
	}
	exact("wrap's use", use, map[string]any{"template": "card", "title": "W"})
	fw := onlyChild(t, use)
	if fw["kind"] != "slot" {
		t.Fatalf("wrap's use holds %v", fw["kind"])
	}
	exact("wrap's slot", fw, map[string]any{})
	empty("wrap's slot", fw)

	c := irChild(t, irChild(t, root, "screen", "main"), "use", "c")
	exact("use c", c, map[string]any{"template": "card", "title": "Hi", "with": "m as t"})
	cbox := onlyChild(t, c)
	if cbox["kind"] != "box" {
		t.Fatalf("c's child is %v", cbox["kind"])
	}
	exact("c's box", cbox, map[string]any{"title": "Hi"})
	from("c's box", cbox, "app.tui:3")
	cslot := onlyChild(t, cbox)
	if cslot["kind"] != "slot" {
		t.Fatalf("c's box holds %v", cslot["kind"])
	}
	exact("c's slot", cslot, map[string]any{"template": "card"})
	from("c's slot", cslot, "app.tui:3")
	txt := onlyChild(t, cslot)
	if txt["kind"] != "text" || txt["text"] != "{t}" {
		t.Errorf("c's slot holds %v %v", txt["kind"], txt["text"])
	}
	from("the fill's text", txt, "app.tui:8")
}

// 247. fmt with props and a slot: the attributes keep the author's order
// after id and class, a use with content is a container, and a slot or a
// use without content self-closes.
func TestV04cTools247Fmt(t *testing.T) {
	dir := propsFixtures + "247/"
	want, err := os.ReadFile(dir + "want.tui")
	if err != nil {
		t.Fatal(err)
	}
	code, out, errw := runCLI("fmt", dir+"in.tui")
	if code != 0 || out != string(want) {
		t.Errorf("fmt: exit %d %s\n got:\n%s\nwant:\n%s", code, errw, out, want)
	}
	if code, _, errw := runCLI("fmt", "--check", dir+"want.tui"); code != 0 {
		t.Errorf("fmt --check of the output: exit %d %s", code, errw)
	}
}

// 249, the catalog bullet: slot, props on template, use's open_attrs, and
// nothing else; current 4, catalog 1, 41 diagnostics; the output
// validates against schema/catalog.v1.json.
func TestV04cTools249Catalog(t *testing.T) {
	raw, c := catalogOutput(t)
	if errs := validateSchema(t, catalogSchema, decodeJSON(t, []byte(raw))); len(errs) != 0 {
		t.Fatalf("catalog vs catalog.v1.json: %v", errs)
	}
	if c.Current != 4 || c.Catalog != 1 || len(c.Diagnostics) != 41 {
		t.Errorf("current %d, catalog %d, %d diagnostics", c.Current, c.Catalog, len(c.Diagnostics))
	}
	var slot *catalog.Tag
	for i, tag := range c.Tags {
		switch tag.Name {
		case "slot":
			slot = &c.Tags[i]
		case "template":
			found := false
			for _, a := range tag.Attrs {
				if a.Name == "props" {
					found = a.Since == 4 && len(a.Keywords) == 0
				}
			}
			if !found {
				t.Errorf("template's attrs %+v hold no {props, since 4, keywords []}", tag.Attrs)
			}
		}
		if tag.OpenAttrs != "" && (tag.Name != "use" || tag.OpenAttrs != "props") {
			t.Errorf("%s has open_attrs %q", tag.Name, tag.OpenAttrs)
		}
		if tag.Name == "use" && tag.OpenAttrs != "props" {
			t.Errorf("use has open_attrs %q", tag.OpenAttrs)
		}
	}
	if slot == nil || slot.Since != 4 || len(slot.Attrs) != 0 {
		t.Errorf("slot: %+v", slot)
	}
	found := false
	for _, tag := range jsonMap(t, raw)["tags"].([]any) {
		found = found || reflect.DeepEqual(tag, map[string]any{"name": "slot", "since": float64(4), "attrs": []any{}})
	}
	if !found {
		t.Errorf(`catalog has no {"name": "slot", "since": 4, "attrs": []}`)
	}
}

// 250, its third bullet: {$x} where a prop cannot stand, or anywhere
// outside a template, and in a template that no use copies, is an error.
// When TUIMARK_BASE044 names a tuimark 0.4.4 binary, each case also gets
// the code the SPEC lists for 0.4.4.
func TestV04cTools250NothingValidBecomesValid(t *testing.T) {
	type spot struct {
		name, elem string
		// v044 is the code tuimark 0.4.4 gives outside a template, and
		// v044tmpl in an unused template ("" when the same).
		v044, v044tmpl string
		// top is set for an element that stands in <tui>, not a screen.
		top, noTmpl bool
	}
	const x = "{$x}"
	list := func(attrs string) string {
		return `<list id="l" ` + attrs + `><item><text>a</text></item></list>`
	}
	spots := []spot{
		{name: "text", elem: `<text>X</text>`, v044: "V003"},
		{name: "button body", elem: `<button id="b">X</button>`, v044: "V003"},
		{name: "column body", elem: `<table id="tb" each="rows as r" key="r.k"><column>X</column></table>`, v044: "V003"},
		{name: "title", elem: `<box title="X"/>`, v044: "V003"},
		{name: "placeholder", elem: `<input id="i" placeholder="X"/>`, v044: "V003"},
		{name: "button label", elem: `<button id="b" label="X"/>`, v044: "V003"},
		{name: "checkbox label", elem: `<checkbox id="c" label="X"/>`, v044: "V003"},
		{name: "select label", elem: `<select id="s" each="rows as r" key="r.k" label="X"/>`, v044: "V003"},
		{name: "tab label", elem: `<tabs id="ts"><tab id="t1" label="X"/></tabs>`, v044: "V003"},
		{name: "short", elem: `<tabs id="ts"><tab id="t1" label="a" short="X"/></tabs>`, v044: "V003"},
		{name: "width", elem: `<box width="X"/>`, v044: "V003"},
		{name: "height", elem: `<box height="X"/>`, v044: "V003"},
		{name: "gap", elem: `<box gap="X"/>`, v044: "V003"},
		{name: "pad", elem: `<box pad="X"/>`, v044: "V003"},
		{name: "border", elem: `<box border="X"/>`, v044: "V003"},
		{name: "max", elem: `<sparkline bind="h" max="X"/>`, v044: "V003"},
		{name: "min", elem: `<sparkline bind="h" min="X"/>`, v044: "V003"},
		{name: "mark", elem: list(`each="rows as r" key="r.k" checked="sel" mark="X"`), v044: "V003"},
		{name: "keycap", elem: `<keymap><bind keys="q" action="quit" keycap="X"/></keymap>`, v044: "V003", v044tmpl: "V001", top: true},
		{name: "value", elem: `<progress value="X"/>`, v044: "V003"},
		{name: "wrap", elem: `<text wrap="X">a</text>`, v044: "V003"},
		{name: "axis", elem: `<scroll axis="X"><text>a</text></scroll>`, v044: "V003"},
		{name: "priority", elem: `<table id="tb" each="rows as r" key="r.k"><column priority="X">{r.k}</column></table>`, v044: "V003"},
		{name: "scale", elem: `<sparkline bind="h" scale="X"/>`, v044: "V003"},
		{name: "class", elem: `<box class="X"/>`, v044: "V003"},
		{name: "style", elem: `<box style="X"/>`, v044: "V003"},
		{name: "on:click", elem: `<button id="b" on:click="X">a</button>`, v044: "V003"},
		{name: "bind", elem: `<input id="i" bind="X"/>`, v044: "V003"},
		{name: "each", elem: list(`each="X" key="r.k"`), v044: "V011"},
		{name: "key", elem: list(`each="rows as r" key="X"`), v044: "V003"},
		{name: "checked", elem: list(`each="rows as r" key="r.k" checked="X"`), v044: "V003"},
		{name: "if", elem: `<box if="X"/>`, v044: "V011"},
		{name: "hidden", elem: `<box hidden="X"/>`, v044: "V003"},
		{name: "disabled", elem: `<box disabled="X"/>`, v044: "V003"},
		{name: "open", elem: `<modal id="m" open="X"><text>a</text></modal>`, v044: "V003"},
		{name: "class:NAME", elem: `<box class:a="X"/>`, v044: "V015"},
		{name: "with", elem: `<use template="t" with="X"/>`, v044: "V011"},
		{name: "use template", elem: `<use template="X"/>`, v044: "V019", noTmpl: true},
		{name: "id", elem: `<box id="X"/>`, v044: "V003"},
		{name: "focus", elem: `<tabs id="ts"><tab id="t1" label="a" focus="X"/></tabs>`, v044: "V003"},
	}
	base := os.Getenv("TUIMARK_BASE044")
	errorCodes := func(bin, file string) []string {
		var out []byte
		if bin == "" {
			_, o, _ := runCLI("validate", file, "--json")
			out = []byte(o)
		} else {
			out, _ = exec.Command(bin, "validate", file, "--json").Output()
		}
		var v struct {
			Diagnostics []struct{ Severity, Code, Msg string }
		}
		if err := json.Unmarshal(out, &v); err != nil {
			t.Fatalf("validate %s: %v\n%s", file, err, out)
		}
		var codes []string
		for _, d := range v.Diagnostics {
			if d.Severity == "error" {
				codes = append(codes, d.Code+" "+d.Msg)
			}
		}
		return codes
	}
	for _, s := range spots {
		elem := strings.Replace(s.elem, "X", x, 1)
		docs := map[string]string{}
		lib := `  <template id="t"><text>t</text></template>` + "\n"
		if s.top {
			docs["outside"] = "<tui>\n" + lib + "  " + elem + "\n  <screen id=\"main\"><text>a</text></screen>\n</tui>\n"
		} else {
			docs["outside"] = "<tui>\n" + lib + "  <screen id=\"main\">\n    " + elem + "\n  </screen>\n</tui>\n"
		}
		if !s.noTmpl {
			docs["in an unused template"] = "<tui>\n" + lib + "  <template id=\"u\">\n    " + elem + "\n  </template>\n  <screen id=\"main\"><text>a</text></screen>\n</tui>\n"
		}
		for where, doc := range docs {
			f := writeTemp(t, "app.tui", doc)
			// The error is about the {$x}, but for the <keymap> that a
			// template cannot hold.
			errs := errorCodes("", f)
			about := len(errs) > 0
			for _, e := range errs {
				about = about && (strings.Contains(e, x) || s.top && where != "outside" && strings.HasPrefix(e, "V001 <keymap>"))
			}
			if !about {
				t.Errorf("%s %s: errors %v\n%s", s.name, where, errs, doc)
			}
			if base == "" {
				continue
			}
			want := s.v044
			if where != "outside" && s.v044tmpl != "" {
				want = s.v044tmpl
			}
			found := false
			errs = errorCodes(base, f)
			for _, e := range errs {
				found = found || strings.HasPrefix(e, want+" ")
			}
			if !found {
				t.Errorf("%s %s: tuimark 0.4.4 gives %v, want a %s\n%s", s.name, where, errs, want, doc)
			}
		}
	}
}

// 250, its second bullet: a pinned document's {$x} in a <text> keeps the
// diagnostic tuimark 0.4.4 gives it.
func TestV04cTools250Pinned(t *testing.T) {
	for _, v := range []string{"1", "2", "3"} {
		f := writeTemp(t, "v.tui", "<tui version=\""+v+"\">\n  <screen id=\"main\">\n    <text>{$x}</text>\n  </screen>\n</tui>\n")
		sameLines(t, "version "+v, vdiags(t, f, ""), []string{
			`error V003 v.tui:3:11 /screen/text id= {$x} is not a path; markup has no expressions`,
		})
	}
}

// 256. A use and an include written in a fill come before the slot that
// holds the fill in via, and the use's id is not scoped by the receiving
// use.
func TestV04cTools256FillVia(t *testing.T) {
	dir := propsFixtures + "256/"
	sameLines(t, "validate", vdiags(t, dir+"app.tui", ""), []string{})
	viaIs(t, `{"file": "app.tui", "line": 3, "col": 21, "via": [{"use": "k", "template": "kv", "file": "app.tui", "line": 6, "col": 7}, {"slot": "c", "template": "card", "file": "app.tui", "line": 2, "col": 28}]}`, dir+"app.tui", "--id", "k__t")
	viaIs(t, `{"file": "f.tui", "line": 2, "col": 3, "via": [{"include": "f.tui", "file": "app.tui", "line": 7, "col": 7}, {"slot": "c", "template": "card", "file": "app.tui", "line": 2, "col": 28}]}`, dir+"app.tui", "--id", "ft")
	sameLines(t, "inspect --id ft text", inspectTail(t, 3, dir+"app.tui", "--id", "ft"), []string{
		`source f.tui:2:3`,
		`via <include src="f.tui"> app.tui:7:7`,
		`via <slot/> of <use id="c" template="card"> app.tui:2:28`,
	})
}
