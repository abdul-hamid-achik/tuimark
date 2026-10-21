package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/catalog"
	"github.com/abdul-hamid-achik/tuimark/internal/host"
	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// SPEC v0.4c §24 tests of props (phase C1), the parts that go through the
// CLI: the validate lists of 236-240, 252-254, 261, and 262, the dumps'
// texts, and the ir bullets. The Go API parts (equal dumps, the prop
// growth of test 248) are the root package's conformance_v04c_props_test.go.
// The fixtures are testdata/v04c/<test>.

const propsFixtures = "../../testdata/v04c/"

// irRoot runs ir on file, checks it against schema/ir.v0.4.json, and
// returns its root.
func irRoot(t *testing.T, file string) map[string]any {
	t.Helper()
	code, out, errw := runCLI("ir", file)
	if code != 0 {
		t.Fatalf("ir %s: exit %d %s", file, code, errw)
	}
	if errs := validateSchema(t, irV04Schema, decodeJSON(t, []byte(out))); len(errs) != 0 {
		t.Errorf("ir %s vs ir.v0.4.json: %v", file, errs)
	}
	return jsonMap(t, out)["root"].(map[string]any)
}

// parsedIRRoot is the root of the IR the parser builds for file, for a
// document with errors, on which ir exits 2 (§15.1, decision 114).
func parsedIRRoot(t *testing.T, file string) map[string]any {
	t.Helper()
	h, err := host.Load(file)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(parse.BuildIR(h.Document(), nil))
	if err != nil {
		t.Fatal(err)
	}
	return jsonMap(t, string(raw))["root"].(map[string]any)
}

// textsOf is the text of each text node of a dump, "" for one without.
func textsOf(nodes []map[string]any) []string {
	out := []string{}
	for _, n := range nodes {
		if n["tag"] == "text" {
			s, _ := n["text"].(string)
			out = append(out, s)
		}
	}
	return out
}

// attrsOf is the attrs member of an IR node.
func attrsOf(n map[string]any) map[string]any {
	return n["attrs"].(map[string]any)
}

// 236. A prop passed, not passed, and empty.
func TestV04cProps236PassedNotPassedEmpty(t *testing.T) {
	dir := propsFixtures + "236/"
	sameLines(t, "validate", vdiags(t, dir+"app.tui", dir+"d.json"), []string{})
	root := irRoot(t, dir+"app.tui")
	screen := kids(t, root)[1]
	uses := kids(t, screen)
	if len(uses) != 3 {
		t.Fatalf("screen: %v", uses)
	}
	box := func(i int) (map[string]any, []map[string]any) {
		b := kids(t, uses[i])[0]
		return b, kids(t, b)
	}
	// The first copy: the prop is substituted, the path stays as written.
	b0, c0 := box(0)
	if attrsOf(b0)["title"] != "CPU" || c0[0]["text"] != "CPU: {v}" || attrsOf(c0[1])["max"] != "100" {
		t.Errorf("first copy: %v %v", b0, c0)
	}
	if !reflect.DeepEqual(b0["classes"], []any{"b", "hot"}) {
		t.Errorf("first copy's classes: %v", b0["classes"])
	}
	// The second: an unpassed whole-value prop removes the attribute.
	b1, c1 := box(1)
	if _, has := attrsOf(b1)["title"]; has {
		t.Errorf("second copy's box has a title: %v", b1)
	}
	if _, has := attrsOf(c1[1])["max"]; has {
		t.Errorf("second copy's sparkline has a max: %v", c1[1])
	}
	// The third: an empty value is passed.
	b2, _ := box(2)
	if title, has := attrsOf(b2)["title"]; !has || title != "" {
		t.Errorf("third copy's box: %v", b2)
	}
	// The template keeps its body as written.
	tmpl := kids(t, root)[0]
	if attrsOf(tmpl)["props"] != "title max tone" {
		t.Errorf("template: %v", tmpl)
	}
	tb := kids(t, tmpl)[0]
	if attrsOf(tb)["title"] != "{$title}" || kids(t, tb)[0]["text"] != "{$title}: {v}" || attrsOf(kids(t, tb)[1])["max"] != "{$max}" {
		t.Errorf("template body: %v", tb)
	}
}

// 237. The template's own errors, reported once, with no suffix, in a
// template no use copies; and the <template> element's own attributes,
// which are outside its body.
func TestV04cProps237OwnErrors(t *testing.T) {
	dir := propsFixtures + "237/"
	sameLines(t, "validate", vdiags(t, dir+"app.tui", ""), []string{
		`error V003 app.tui:2:20 /template id=a props="ok  Bad ok id": separate the names with one space`,
		`error V003 app.tui:2:20 /template id=a invalid prop name "Bad" in props: want [a-z_][a-z0-9_-]*`,
		`error V003 app.tui:2:20 /template id=a prop "ok" is declared twice`,
		`error V003 app.tui:2:20 /template id=a "id" cannot be a prop: <use> takes id="…" for itself`,
		`error V003 app.tui:3:10 /template/box id= {$nope} names no prop of <template id="a">; declare it in props="…"`,
		`error V003 app.tui:3:26 /template/box id= {$ok} cannot stand in focusable; write the value itself`,
		`error V003 app.tui:4:13 /template/box/text id= malformed prop reference "{$Ok}": want {$name}, name [a-z_][a-z0-9_-]*`,
		`error V003 app.tui:5:14 /template/box/input id= {$ok} cannot stand in id; write the value itself`,
		`error V003 app.tui:5:25 /template/box/input id= {$ok} cannot stand in bind: a prop is text; pass a path with with="…"`,
		`error V003 app.tui:9:11 /screen/text id= {$ok} is outside a <template>: props exist only in a template's body`,
	})
	sameLines(t, "own.tui", vdiags(t, dir+"own.tui", ""), []string{
		`error V003 own.tui:2:13 /template id={$x} {$x} is outside a <template>: props exist only in a template's body`,
		`error V003 own.tui:2:23 /template id={$x} {$y} is outside a <template>: props exist only in a template's body`,
	})
}

// 238. The use's side: undeclared names, and pairs that a value makes.
func TestV04cProps238UseSide(t *testing.T) {
	dir := propsFixtures + "238/"
	sameLines(t, "validate", vdiags(t, dir+"app.tui", dir+"d.json"), []string{
		`error V002 app.tui:4:33 /screen/use id= attribute "width" is not allowed on <use>: a <use> is not a node; give this copy a static class="…" and style that class, put the attribute on the template's element, or declare it in the template's props`,
		`error V002 app.tui:4:43 /screen/use id= attribute "class:x" is not allowed on <use>: a <use> is not a node; give this copy a static class="…" and style that class, or put the attribute on the template's element`,
		`error V002 app.tui:4:55 /screen/use id= attribute "on:click" is not allowed on <use>: a <use> is not a node; give this copy a static class="…" and style that class, or put the attribute on the template's element`,
		`error V019 app.tui:5:5 /screen/use id= <use template="nope"> names no <template>`,
		`error V003 app.tui:6:23 /screen/use id= prop "label" makes the pair "{n}": a prop is text; pass a path with with="…"`,
		`error V003 app.tui:7:23 /screen/use id= prop "label" makes the pair "{n: {v}": a prop is text; pass a path with with="…"`,
	})
	code, _, _ := runCLI("dump", dir+"app.tui", "--data", dir+"d.json", "--cols", "20", "--rows", "4", "--format", "json")
	if code != 2 {
		t.Errorf("dump: exit %d, want 2", code)
	}
	nodes, _ := dumpNodes(t, dir+"app.tui", "--data", dir+"d.json", "--cols", "20", "--rows", "4")
	if got, want := textsOf(nodes), []string{"a: 7", ": 7", ": 7", "x}: 7"}; !reflect.DeepEqual(got, want) {
		t.Errorf("texts %q, want %q", got, want)
	}
}

// handMsg is the message validate gives the one element of a hand-written
// document.
func handMsg(t *testing.T, doc string) string {
	t.Helper()
	got := vdiags(t, writeTemp(t, "hand.tui", doc), "")
	if len(got) != 1 {
		t.Fatalf("hand-written: %v", got)
	}
	_, msg, _ := strings.Cut(got[0], " id= ")
	return msg
}

// 239. A substituted value is checked in the copy, at the template's
// attribute, with the suffix of the use that wrote it.
func TestV04cProps239CheckedInTheCopy(t *testing.T) {
	dir := propsFixtures + "239/"
	class := handMsg(t, "<tui>\n  <screen id=\"main\"><sparkline class=\"9x\" bind=\"h\"/></screen>\n</tui>")
	max := handMsg(t, "<tui>\n  <screen id=\"main\"><sparkline bind=\"h\" max=\"lots\"/></screen>\n</tui>")
	if class != `bad class name "9x"` {
		t.Errorf("hand-written class: %q", class)
	}
	sameLines(t, "validate", vdiags(t, dir+"app.tui", dir+"d.json"), []string{
		`error V003 app.tui:3:16 /screen/sparkline id= ` + class + `, placed by <use id="b"> at app.tui:7:5`,
		`error V003 app.tui:3:41 /screen/sparkline id= ` + max + `, placed by <use id="a"> at app.tui:6:5`,
		`error V003 app.tui:3:41 /screen/sparkline id= ` + max + `, placed by <use id="b"> at app.tui:7:5`,
	})
	// No copy carries a class.
	nodes, _ := dumpNodes(t, dir+"app.tui", "--data", dir+"d.json", "--cols", "20", "--rows", "4")
	n := 0
	for _, x := range nodes {
		if x["tag"] == "sparkline" {
			n++
			if cs, has := x["classes"]; has && len(cs.([]any)) != 0 {
				t.Errorf("a sparkline has classes %v", cs)
			}
		}
	}
	if n != 3 {
		t.Errorf("%d sparklines, want 3", n)
	}
}

// 240. Forwarding props, through a use in a template's body.
func TestV04cProps240Forwarding(t *testing.T) {
	dir := propsFixtures + "240/"
	sameLines(t, "validate", vdiags(t, dir+"app.tui", dir+"d.json"), []string{})
	nodes, _ := dumpNodes(t, dir+"app.tui", "--data", dir+"d.json", "--cols", "20", "--rows", "6")
	if got, want := textsOf(nodes), []string{"x=1", "x (n)=2", "=1", "()=2"}; !reflect.DeepEqual(got, want) {
		t.Errorf("texts %q, want %q", got, want)
	}
	root := irRoot(t, dir+"app.tui")
	second := kids(t, kids(t, root)[2])[1] // the screen's second use
	inner := kids(t, kids(t, second)[0])   // its copy's col
	if len(inner) != 2 || inner[0]["kind"] != "use" || inner[1]["kind"] != "use" {
		t.Fatalf("the second pair copy: %v", inner)
	}
	if _, has := attrsOf(inner[0])["label"]; has {
		t.Errorf("the first inner use has a label: %v", inner[0])
	}
	if attrsOf(inner[1])["label"] != " ()" {
		t.Errorf("the second inner use: %v", inner[1])
	}
}

// 252. Braces and props: a prop inside braces as written, and a pair a
// forwarded value makes, reported at the outermost use.
func TestV04cProps252Braces(t *testing.T) {
	dir := propsFixtures + "252/"
	sameLines(t, "validate", vdiags(t, dir+"app.tui", ""), []string{
		`error V003 app.tui:4:13 /template/col/text id= {$p} cannot stand inside {…}: a prop is text; pass a path with with="…"`,
		`error V003 app.tui:5:13 /template/col/text id= {$p} cannot stand inside {…}: a prop is text; pass a path with with="…"`,
		`error V003 app.tui:6:13 /template/col/text id= {$k} cannot stand inside {…}: a prop is text; pass a path with with="…"`,
		`error V003 app.tui:13:24 /screen/use id= prop "label" makes the pair "{n}": a prop is text; pass a path with with="…"`,
		`error V003 app.tui:14:23 /screen/use id= {$q} is outside a <template>: props exist only in a template's body`,
	})
	nodes, _ := dumpNodes(t, dir+"app.tui", "--cols", "20", "--rows", "8")
	if got, want := textsOf(nodes), []string{"", "t", "", "}", "}"}; !reflect.DeepEqual(got, want) {
		t.Errorf("texts %q, want %q", got, want)
	}
}

// 253. A prop where it cannot stand, in a used template: one diagnostic
// each, and nothing follows from them.
func TestV04cProps253CannotStand(t *testing.T) {
	dir := propsFixtures + "253/"
	sameLines(t, "validate", vdiags(t, dir+"app.tui", ""), []string{
		`error V003 app.tui:5:14 /template/col/input id= {$x} cannot stand in id; write the value itself`,
		`error V003 app.tui:6:18 /template/col/sparkline id= {$x} cannot stand in bind: a prop is text; pass a path with with="…"`,
		`error V003 app.tui:7:25 /template/col/use id= {$x} cannot stand in if: a prop is text; pass a path with with="…"`,
		`error V003 app.tui:8:16 /template/col/include id= {$x} cannot stand in src; write the value itself`,
	})
	nodes, _ := dumpNodes(t, dir+"app.tui", "--cols", "20", "--rows", "8")
	if got, want := textsOf(nodes), []string{"kept", "kept"}; !reflect.DeepEqual(got, want) {
		t.Errorf("texts %q, want %q", got, want)
	}
	// No file is read.
	h, err := host.Load(dir + "app.tui")
	if err != nil {
		t.Fatal(err)
	}
	if fs := h.Document().Fragments; len(fs) != 0 {
		t.Errorf("fragments read: %v", fs)
	}
}

// 254. The checks of a <use> written in a template's body, made as
// written, once per written use.
func TestV04cProps254UseInATemplate(t *testing.T) {
	const style = `attribute "style" is not allowed on <use>: a <use> is not a node; give this copy a static class="…" and style that class, put the attribute on the template's element, or declare it in the template's props`
	sameLines(t, "validate", vdiags(t, propsFixtures+"254/app.tui", ""), []string{
		`error V002 app.tui:6:25 /template/col/use id= ` + style,
		`error V002 app.tui:6:35 /template/col/use id= attribute "class:z" is not allowed on <use>: a <use> is not a node; give this copy a static class="…" and style that class, or put the attribute on the template's element`,
		`error V013 app.tui:7:28 /template/col/use id= <template id="bare"> has no <slot/>`,
		`error V002 app.tui:12:35 /screen/col/use id= ` + style,
		`error V013 app.tui:13:28 /screen/col/use id= <template id="bare"> has no <slot/>`,
	})
}

// 261. Pairs that several props make.
func TestV04cProps261SeveralProps(t *testing.T) {
	dir := propsFixtures + "261/"
	const path = `pass a path with with="…"`
	sameLines(t, "validate", vdiags(t, dir+"app.tui", ""), []string{
		`error V003 app.tui:10:23 /screen/use id= prop "a" makes the pair "{x}": a prop is text; ` + path,
		`error V003 app.tui:10:29 /screen/use id= prop "b" makes the pair "{x}": a prop is text; ` + path,
		`error V003 app.tui:10:36 /screen/use id= prop "c" makes the pair "{p}": a prop is text; ` + path,
		`error V003 app.tui:10:44 /screen/use id= prop "d" makes the pair "{q}": a prop is text; ` + path,
		`error V003 app.tui:10:52 /screen/use id= prop "e" makes the pair "{x}": a prop is text; ` + path,
		`error V003 app.tui:10:64 /screen/use id= prop "s" makes the pair "{x}": a prop is text; write the value without braces`,
	})
	sameLines(t, "hand.tui", vdiags(t, dir+"hand.tui", ""), []string{})
	// ir exits 2 on these errors and prints no IR (§15.1), so the IR is
	// the one the parser builds (as decision 114 reads test 146).
	if code, out, _ := runCLI("ir", dir+"app.tui"); code != 2 || out != "" {
		t.Errorf("ir: exit %d, stdout %q", code, out)
	}
	root := parsedIRRoot(t, dir+"app.tui")
	use := kids(t, kids(t, root)[1])[0]
	texts := kids(t, kids(t, use)[0])
	if len(texts) != 3 {
		t.Fatalf("the copy: %v", texts)
	}
	if title, has := attrsOf(texts[0])["title"]; !has || title != " " {
		t.Errorf("the first text: %v", texts[0])
	}
	if style, has := texts[2]["style"].(map[string]any); !has || len(style) != 0 {
		t.Errorf("the last text has a style: %v", texts[2])
	}
}

// 262. A bad value through forwarding names the use where it is written;
// a message that names its placement gets no suffix.
func TestV04cProps262Forwarded(t *testing.T) {
	dir := propsFixtures + "262/"
	const max = `max="lots": want a number such as 0, -5, or 0.25`
	sameLines(t, "validate", vdiags(t, dir+"app.tui", dir+"d.json"), []string{
		`error V003 app.tui:4:27 /screen/col/sparkline id= ` + max + `, placed by <use id="a"> at app.tui:12:5`,
		`error V003 app.tui:4:27 /screen/col/sparkline id= ` + max + `, placed by <use id="b"> at app.tui:13:5`,
		`warning B003 app.tui:5:7 /screen/col/text id= bind path "v.x" is missing (data.x via <use template="k">)`,
		`warning B003 app.tui:5:7 /screen/col/text id= bind path "v.x" is missing (data.x via <use id="c">)`,
	})
	if got := handMsg(t, "<tui>\n  <screen id=\"main\"><sparkline bind=\"h\" max=\"lots\"/></screen>\n</tui>"); got != max {
		t.Errorf("hand-written max: %q", got)
	}
}

// 250, the corner cases of decision 151: a {$name} in a src outside every
// template loads nothing, and a use's template that holds one in a
// template's body is V003. The files {$x}.tcss and {$y}.tui exist, and
// are written at run time (a file name with braces is no fixture).
func TestV04cProps250Corners(t *testing.T) {
	dir := t.TempDir()
	write := func(name, s string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	write("{$x}.tcss", "text { bold: true; }\n")
	write("{$y}.tui", "<fragment>\n  <text>y</text>\n</fragment>\n")
	src := write("src.tui", "<tui>\n  <style src=\"{$x}.tcss\"/>\n  <screen id=\"main\">\n    <include src=\"{$y}.tui\"/>\n  </screen>\n</tui>")
	sameLines(t, "src.tui", vdiags(t, src, ""), []string{
		`error V003 src.tui:2:10 /style id= {$x} is outside a <template>: props exist only in a template's body`,
		`error V003 src.tui:4:14 /screen/include id= {$y} is outside a <template>: props exist only in a template's body`,
	})
	h, err := host.Load(src)
	if err != nil {
		t.Fatal(err)
	}
	if doc := h.Document(); len(doc.Styles) != 0 || len(doc.Fragments) != 0 || len(h.Sheets()) != 0 {
		t.Errorf("loaded: styles %v, fragments %v, %d sheets", doc.Styles, doc.Fragments, len(h.Sheets()))
	}
	if nodes, _ := dumpNodes(t, src, "--cols", "10", "--rows", "2"); len(nodes) != 1 {
		t.Errorf("the include inserted %v", nodes)
	}
	tu := write("tu.tui", "<tui>\n  <template id=\"u\">\n    <col><use template=\"{$x}\"/></col>\n  </template>\n  <screen id=\"main\"><text>t</text></screen>\n</tui>")
	sameLines(t, "tu.tui", vdiags(t, tu, ""), []string{
		`error V003 tu.tui:3:15 /template/col/use id= {$x} cannot stand in template; write the value itself`,
	})
}

// 249, the catalog bullets of C1: props on template, and use's open_attrs.
func TestV04cPropsCatalog(t *testing.T) {
	c := catalog.Build("test")
	open := 0
	for _, tag := range c.Tags {
		if tag.OpenAttrs != "" {
			open++
			if tag.Name != "use" || tag.OpenAttrs != "props" {
				t.Errorf("%s has open_attrs %q", tag.Name, tag.OpenAttrs)
			}
		}
		if tag.Name != "template" {
			continue
		}
		found := false
		for _, a := range tag.Attrs {
			if a.Name == "props" {
				found = true
				if a.Since != 4 || len(a.Keywords) != 0 {
					t.Errorf("template's props: %+v", a)
				}
			}
		}
		if !found {
			t.Errorf("template has no props: %+v", tag.Attrs)
		}
	}
	if open != 1 {
		t.Errorf("%d tags with open_attrs, want 1", open)
	}
	code, out, _ := runCLI("catalog")
	if code != 0 || !strings.Contains(out, `"open_attrs": "props"`) {
		t.Errorf("catalog: exit %d, open_attrs missing", code)
	}
}

// Regressions of C1's implementation choices, beyond the draft's tests.
func TestV04cPropsImplementation(t *testing.T) {
	write := func(dir, name, s string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	// Decision 194: a diagnostic at an element names the outermost writer
	// among its substituted values, else the use that copied it.
	t.Run("element suffix", func(t *testing.T) {
		f := writeTemp(t, "a.tui", "<tui>\n  <template id=\"b\" props=\"t c\"><button title=\"{$t}\" class=\"{$c}\">x</button></template>\n  <template id=\"fw\" props=\"t\"><use template=\"b\" t=\"{$t}\" c=\"k\"/></template>\n  <screen id=\"main\">\n    <use template=\"b\" t=\"T\" c=\"k\"/>\n    <use template=\"fw\" t=\"U\"/>\n    <use template=\"fw\"/>\n  </screen>\n</tui>\n")
		const v012 = `error V012 a.tui:2:32 /screen/button id= <button> is focusable and needs an id, placed by `
		sameLines(t, "validate", vdiags(t, f, ""), []string{
			v012 + `<use template="b"> at a.tui:5:5`,
			v012 + `<use template="fw"> at a.tui:6:5`,
			v012 + `<use template="b"> at a.tui:3:31`,
		})
	})
	// Decision 160: a prop is reported once, with the first pair it brings
	// a brace to, however many values of the body hold it.
	t.Run("one pair per prop", func(t *testing.T) {
		f := writeTemp(t, "a.tui", "<tui>\n  <template id=\"d\" props=\"p\"><col><text>{$p}</text><text style=\"{$p}\">x</text></col></template>\n  <screen id=\"main\"><use template=\"d\" p=\"{a}\"/></screen>\n</tui>\n")
		sameLines(t, "validate", vdiags(t, f, ""), []string{
			`error V003 a.tui:3:39 /screen/use id= prop "p" makes the pair "{a}": a prop is text; pass a path with with="…"`,
		})
	})
	// A fragment checked on its own: outside its templates a {$name} is not
	// known to be outside one (it may be included in a template's body),
	// while its templates and the uses of them are checked. Included
	// outside every template, the same {$name} is V003.
	t.Run("fragment", func(t *testing.T) {
		dir := t.TempDir()
		frag := write(dir, "frag.tui", "<fragment>\n  <template id=\"f\" props=\"x\"><text>{$x}{$z}</text></template>\n  <text>{$y}</text>\n  <use template=\"f\" x=\"1\" w=\"2\"><text>c</text></use>\n  <use template=\"g\" w=\"2\"/>\n</fragment>\n")
		inc := write(dir, "inc.tui", "<tui>\n  <screen id=\"main\">\n    <include src=\"frag.tui\"/>\n  </screen>\n</tui>\n")
		const w = `attribute "w" is not allowed on <use>: a <use> is not a node; give this copy a static class="…" and style that class, put the attribute on the template's element, or declare it in the template's props`
		sameLines(t, "frag.tui", vdiags(t, frag, ""), []string{
			`error V003 frag.tui:2:36 /template/text id= {$z} names no prop of <template id="f">; declare it in props="…"`,
			`error V002 frag.tui:4:27 /use id= ` + w,
			`error V013 frag.tui:4:33 /use id= <template id="f"> has no <slot/>`,
		})
		sameLines(t, "inc.tui", vdiags(t, inc, ""), []string{
			`error V003 frag.tui:2:36 /screen/template/text id= {$z} names no prop of <template id="f">; declare it in props="…"`,
			`error V003 frag.tui:3:9 /screen/text id= {$y} is outside a <template>: props exist only in a template's body, placed by <include> at inc.tui:3:5`,
			`error V002 frag.tui:4:27 /screen/use id= ` + w,
			`error V013 frag.tui:4:33 /screen/use id= <template id="f"> has no <slot/>`,
			`error V019 frag.tui:5:3 /screen/use id= <use template="g"> names no <template>`,
		})
	})
	// A fragment that an include places in a template's body reads the
	// template's props, and so does a list's row template.
	t.Run("include and rows", func(t *testing.T) {
		dir := t.TempDir()
		write(dir, "part.tui", "<fragment>\n  <text title=\"{$t}\">{$t}!</text>\n</fragment>\n")
		app := write(dir, "b.tui", "<tui>\n  <template id=\"k\" props=\"t lab\">\n    <col>\n      <include src=\"part.tui\"/>\n      <list id=\"l\" each=\"items as r\" key=\"r.id\"><item><text>{$lab}: {r.name}</text></item></list>\n    </col>\n  </template>\n  <screen id=\"main\">\n    <use id=\"u\" template=\"k\" t=\"T\" lab=\"L\"/>\n  </screen>\n</tui>\n")
		data := write(dir, "b.json", `{"items":[{"id":1,"name":"a"},{"id":2,"name":"b"}]}`)
		sameLines(t, "validate", vdiags(t, app, data), []string{})
		nodes, _ := dumpNodes(t, app, "--data", data, "--cols", "20", "--rows", "5")
		if got, want := textsOf(nodes), []string{"T!", "L: a", "L: b"}; !reflect.DeepEqual(got, want) {
			t.Errorf("texts %q, want %q", got, want)
		}
	})
}

// The fixes of the C1 review (S1-S5, errata E1 and E4, nit N4), each with
// the input that showed it.
func TestV04cPropsReviewFixes(t *testing.T) {
	// S1, §15.1: a fragment checked on its own gives a {$name} outside its
	// templates the checks that do not depend on a template (a malformed
	// reference, one where a prop cannot stand, one inside braces), its use
	// values included; a well-formed one in a text place is not reported.
	t.Run("fragment on its own", func(t *testing.T) {
		f := writeTemp(t, "k.tui", "<fragment>\n  <text id=\"{$p}\" title=\"{{$q}}\">z</text>\n  <list id=\"l\" each=\"{$e}\" key=\"k\"><item><text>a</text></item></list>\n  <text>{$Bad}</text>\n  <text>{$ok}</text>\n  <keymap><bind keys=\"{$k}\" action=\"quit\"/></keymap>\n  <template id=\"t\" props=\"p\"><text>{$p}</text></template>\n  <use template=\"t\" p=\"{$Bad}\"/>\n</fragment>\n")
		sameLines(t, "validate", vdiags(t, f, ""), []string{
			`error V003 k.tui:2:9 /text id= {$p} cannot stand in id; write the value itself`,
			`error V003 k.tui:2:19 /text id= {$q} cannot stand inside {…}: a prop is text; pass a path with with="…"`,
			`error V003 k.tui:3:16 /list id=l {$e} cannot stand in each: a prop is text; pass a path with with="…"`,
			`error V003 k.tui:4:9 /text id= malformed prop reference "{$Bad}": want {$name}, name [a-z_][a-z0-9_-]*`,
			`error V003 k.tui:6:17 /keymap/bind id= {$k} cannot stand in keys; write the value itself`,
			`error V003 k.tui:8:21 /use id= malformed prop reference "{$Bad}": want {$name}, name [a-z_][a-z0-9_-]*`,
		})
	})
	// S2: outside a template, a value's {$name} V003 is reported instead of
	// any other diagnostic about it, the keymap's and focus='s included.
	t.Run("outside replaces the rest", func(t *testing.T) {
		f := writeTemp(t, "a.tui", "<tui>\n  <keymap when=\"{$w}\"><bind keys=\"{$k}\" action=\"{$a}\" to=\"#{$x}\"/><bind keys=\"q\" action=\"quit\" when=\"{$v}\"/></keymap>\n  <screen id=\"main\" focus=\"#{$f}\"><text>t</text></screen>\n</tui>\n")
		const out = ` is outside a <template>: props exist only in a template's body`
		sameLines(t, "validate", vdiags(t, f, ""), []string{
			`error V003 a.tui:2:11 /keymap id= {$w}` + out,
			`error V003 a.tui:2:29 /keymap/bind id= {$k}` + out,
			`error V003 a.tui:2:41 /keymap/bind id= {$a}` + out,
			`error V003 a.tui:2:55 /keymap/bind id= {$x}` + out,
			`error V003 a.tui:2:96 /keymap/bind id= {$v}` + out,
			`error V003 a.tui:3:21 /screen id=main {$f}` + out,
		})
	})
	// S3: an ignored checked counts as written for what requires it (mark
	// needs checked) and is absent for the V018 its own presence triggers.
	t.Run("ignored checked", func(t *testing.T) {
		f := writeTemp(t, "a.tui", "<tui>\n  <template id=\"t\" props=\"x\"><list id=\"l\" checked=\"{$x}\" mark=\"*\"><item><text>a</text></item></list></template>\n  <screen id=\"main\"><use id=\"a\" template=\"t\" x=\"q\"/><use id=\"b\" template=\"t\"/></screen>\n</tui>\n")
		sameLines(t, "validate", vdiags(t, f, ""), []string{
			`error V003 a.tui:2:43 /template/list id=l {$x} cannot stand in checked: a prop is text; pass a path with with="…"`,
		})
	})
	// S4, §6.18.6: a diagnostic at the text of a body whose element's
	// attribute held a {$name} carries the copy's suffix, once per copy.
	t.Run("body of a copy", func(t *testing.T) {
		f := writeTemp(t, "a.tui", "<tui>\n  <template id=\"t\" props=\"ttl\"><text title=\"{$ttl}\">{ bad }</text></template>\n  <screen id=\"main\">\n    <use template=\"t\" ttl=\"A\"/>\n    <use template=\"t\" ttl=\"B\"/>\n  </screen>\n</tui>\n")
		const msg = `error V003 a.tui:2:53 /screen/text id= { bad } is not a path: write {bad}, with no spaces inside the braces, placed by <use template="t"> at `
		sameLines(t, "validate", vdiags(t, f, ""), []string{msg + `a.tui:4:5`, msg + `a.tui:5:5`})
	})
	// E4: a check reported at another attribute of the element that reads
	// a substituted value names the use that wrote it.
	t.Run("combination check", func(t *testing.T) {
		f := writeTemp(t, "a.tui", "<tui>\n  <template id=\"t\" props=\"x\"><progress indeterminate=\"true\" value=\"{$x}\"/></template>\n  <screen id=\"main\"><use id=\"a\" template=\"t\" x=\"50\"/><use id=\"b\" template=\"t\"/></screen>\n</tui>\n")
		sameLines(t, "validate", vdiags(t, f, ""), []string{
			`error V003 a.tui:2:40 /screen/progress id= indeterminate="true" cannot be combined with bind or value, placed by <use id="a"> at a.tui:3:21`,
		})
	})
	// E1: an attribute its element takes in the definition but not where a
	// copy lands is that copy's V002, with its suffix; the definition's own
	// checks of the {$name} in it stand. A whole value of one {$name} that
	// names no prop reads as unset: the attribute is not in the copy, so
	// its place rejects nothing (the width).
	t.Run("place-dependent attribute", func(t *testing.T) {
		f := writeTemp(t, "a.tui", "<tui>\n  <template id=\"row\" props=\"c t\"><item class=\"{$c}\" title=\"{$t}\" width=\"{$nope}\"/></template>\n  <template id=\"cell\" props=\"act\"><text on:click=\"{$act}\">c</text></template>\n  <screen id=\"main\">\n    <table id=\"tb\" each=\"rows as r\" key=\"r.id\"><column title=\"N\">{r.name}</column><use template=\"row\" c=\"hot\" t=\"T\"/></table>\n    <list id=\"ls\" each=\"rows as r\" key=\"r\"><item><use template=\"cell\" act=\"go\"/></item></list>\n  </screen>\n</tui>\n")
		got := vdiags(t, f, "")
		sameLines(t, "validate", got, []string{
			`error V002 a.tui:2:53 /screen/table/item id= attribute "title" is not allowed on <item>, placed by <use template="row"> at a.tui:5:83`,
			`error V003 a.tui:2:66 /template/item id= {$nope} names no prop of <template id="row">; declare it in props="…"`,
			`error V002 a.tui:3:41 /screen/list/item/text id= attribute "on:click" is not allowed inside a list <item> (on <text>): list rows are navigated by their list; handle the row with the list's on:select, or a keymap <bind keys="..." action="..." when="#ls:focus"/> (its event identifies the selected row), placed by <use template="cell"> at a.tui:6:50`,
		})
	})
	// The same with the props not passed: each whole-value attribute is
	// removed from the copy (decision 163), so no place rejects it, and the
	// document is valid.
	t.Run("place-dependent attribute, not passed", func(t *testing.T) {
		f := writeTemp(t, "a.tui", "<tui>\n  <template id=\"row\" props=\"c t w\"><item class=\"{$c}\" title=\"{$t}\" width=\"{$w}\"/></template>\n  <template id=\"cell\" props=\"act\"><text on:click=\"{$act}\">c</text></template>\n  <screen id=\"main\">\n    <table id=\"tb\" each=\"rows as r\" key=\"r.id\"><column title=\"N\">{r.name}</column><use template=\"row\" c=\"hot\"/></table>\n    <list id=\"ls\" each=\"rows as r\" key=\"r\"><item><use template=\"cell\"/></item></list>\n  </screen>\n</tui>\n")
		sameLines(t, "validate", vdiags(t, f, ""), []string{})
	})
	// N4, decision 186: only a well-formed {$name} is one unit; the braces
	// of a malformed one pair as §7 says, so {$p} is not inside a pair.
	t.Run("malformed braces pair", func(t *testing.T) {
		f := writeTemp(t, "a.tui", "<tui>\n  <template id=\"t\" props=\"p\"><text title=\"{a {$Bad} {$p} b}\">x</text></template>\n  <screen id=\"main\"><text>t</text></screen>\n</tui>\n")
		sameLines(t, "validate", vdiags(t, f, ""), []string{
			`error V003 a.tui:2:36 /template/text id= malformed prop reference "{$Bad}": want {$name}, name [a-z_][a-z0-9_-]*`,
		})
	})
}
