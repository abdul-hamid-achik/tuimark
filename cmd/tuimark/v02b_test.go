package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// SPEC v0.2b §21 tests of the foundations cut through the CLI: IR 0.2
// (40), schemas (68), theme media and @theme in play (42, 43), inspect
// (66), the manifest's cells (§15.5), and fmt of class:NAME (§15.1).

const v2Fixture = `<tui version="2" theme="auto" mouse="mouse_on">
  <style>
    :root { --brand: #e0443e; }
    @media (theme: light) { :root { --brand: #c8102e; } }
    .hot { color: $brand; }
    #l > item > text.hot { color: $warn; bold: true; }
    item > text { color: $muted; }
  </style>
  <keymap>
    <bind keys="q" action="quit" label="quit"/>
    <bind keys="space" action="check-toggle" when="#l:focus" label="mark" keycap="SPC"/>
    <bind keys="2" action="switch-to" to="#b"/>
  </keymap>
  <screen id="main" focus="#l">
    <list id="l" each="rows as r" key="r.id" checked="marked">
      <item><text class="name" class:hot="r.hot" class:cold="r.cold">{r.name}</text></item>
    </list>
    <tabs id="n"><tab id="a" label="a"><text>A</text></tab><tab id="b" label="b"><text>B</text></tab></tabs>
    <table id="t" each="rows as r" key="r.id"><item class:hot="r.hot"/><column id="c-name" title="Name">{r.name}</column></table>
    <sparkline bind="hist"/>
    <hints/>
  </screen>
</tui>
`

const v2Data = `{"rows":[{"id":"a","name":"alpha","hot":true,"cold":false},{"id":"b","name":"beta","hot":false,"cold":true}],"marked":[],"hist":[1,2,3],"mouse_on":false}`

func writeV2Fixture(t *testing.T) (tui, data string) {
	t.Helper()
	dir := t.TempDir()
	tui = filepath.Join(dir, "v2.tui")
	data = filepath.Join(dir, "v2.json")
	writeFile(t, tui, v2Fixture)
	writeFile(t, data, v2Data)
	return tui, data
}

// 40 + 68. `tuimark ir` emits "0.1" for the §17 inbox and "0.2" for a
// version="2" document, each valid against its schema; IR 0.2 holds mouse
// in app, label/keycap in keymap rows, class:NAME in attrs, the new
// kinds, and a column's cell template as its text.
func TestIRVersions(t *testing.T) {
	code, out, errw := runCLI("ir", inboxFixture)
	if code != 0 {
		t.Fatalf("ir inbox: exit %d: %s", code, errw)
	}
	v1 := decodeJSON(t, []byte(out))
	if v1.(map[string]any)["version"] != "0.1" {
		t.Errorf("inbox IR version %v", v1.(map[string]any)["version"])
	}
	if errs := validateSchema(t, irV01Schema, v1); len(errs) > 0 {
		t.Errorf("inbox IR vs ir.v0.1.json: %v", errs)
	}
	tui, _ := writeV2Fixture(t)
	code, out, errw = runCLI("ir", tui)
	if code != 0 {
		t.Fatalf("ir v2: exit %d: %s\n%s", code, errw, out)
	}
	v2 := decodeJSON(t, []byte(out))
	if errs := validateSchema(t, irV02Schema, v2); len(errs) > 0 {
		t.Errorf("v2 IR vs ir.v0.2.json: %v", errs)
	}
	if errs := validateSchema(t, irV01Schema, v2); len(errs) == 0 {
		t.Error("a version=\"2\" IR validates against ir.v0.1.json")
	}
	var ir struct {
		Version string `json:"version"`
		App     struct {
			Theme, Mouse string
		} `json:"app"`
		Tokens map[string]string `json:"tokens"`
		Keymap []map[string]any  `json:"keymap"`
		Root   json.RawMessage   `json:"root"`
	}
	mustUnmarshal(t, out, &ir)
	if ir.Version != "0.2" || ir.App.Mouse != "mouse_on" || ir.App.Theme != "auto" {
		t.Errorf("version %q, app %+v", ir.Version, ir.App)
	}
	if ir.Tokens["brand"] != "#e0443e" {
		t.Errorf("tokens resolve for the tools' theme (auto is dark): brand = %q", ir.Tokens["brand"])
	}
	if ir.Keymap[0]["label"] != "quit" || ir.Keymap[1]["keycap"] != "SPC" || ir.Keymap[1]["label"] != "mark" {
		t.Errorf("keymap %v", ir.Keymap)
	}
	if _, ok := ir.Keymap[2]["label"]; ok {
		t.Errorf("an absent label is emitted: %v", ir.Keymap[2])
	}
	root := string(ir.Root)
	for _, want := range []string{`"class:hot":"r.hot"`, `"kind":"table"`, `"kind":"column"`, `"kind":"tabs"`, `"kind":"tab"`, `"kind":"sparkline"`, `"kind":"hints"`, `"text":"{r.name}"`} {
		if !strings.Contains(strings.ReplaceAll(root, " ", ""), strings.ReplaceAll(want, " ", "")) {
			t.Errorf("IR root lacks %s", want)
		}
	}
}

// 68. The dump of a version="2" document, with classes, validates against
// schema/dump.v0.2.json, and every schema file is byte-identical to its
// SPEC §23 block when TUIMARK_SPEC points at the SPEC.
func TestV2DumpValidatesAndSchemasMatchSpec(t *testing.T) {
	tui, data := writeV2Fixture(t)
	code, out, errw := runCLI("dump", tui, "--data", data, "--format", "json", "--cells", "--styles", "--cols", "30", "--rows", "6")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errw)
	}
	if !strings.Contains(out, `"classes": [`) {
		t.Errorf("a version=\"2\" dump has no classes:\n%s", out)
	}
	if errs := validateSchema(t, dumpV02Schema, decodeJSON(t, []byte(out))); len(errs) > 0 {
		t.Errorf("v2 dump vs dump.v0.2.json: %v", errs)
	}
	spec := os.Getenv("TUIMARK_SPEC")
	if spec == "" {
		t.Skip("TUIMARK_SPEC is not set: the schema files are not compared with the SPEC")
	}
	raw, err := os.ReadFile(spec)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ section, file string }{
		{"### 23.1", irV01Schema}, {"### 23.2", irV02Schema}, {"### 23.3", dumpV02Schema},
	} {
		s := string(raw)
		i := strings.Index(s, c.section)
		if i < 0 {
			t.Fatalf("no %s in the SPEC", c.section)
		}
		s = s[i:]
		start := strings.Index(s, "```json\n") + len("```json\n")
		end := strings.Index(s[start:], "\n```")
		block := s[start:start+end] + "\n"
		got, err := os.ReadFile(c.file)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != block {
			t.Errorf("%s is not byte-identical to SPEC %s", c.file, c.section)
		}
	}
}

// 42. dump --styles: a later @media (theme: light) :root block gives the
// light values with --theme light and the base values otherwise; the
// effective theme of an auto document is dark; --theme auto stays a usage
// error.
func TestDumpStylesThemeMedia(t *testing.T) {
	tui, data := writeV2Fixture(t)
	fg := func(args ...string) (string, string) {
		t.Helper()
		code, out, errw := runCLI(append([]string{"dump", tui, "--data", data, "--format", "json", "--styles", "--cols", "30", "--rows", "6"}, args...)...)
		if code != 0 {
			t.Fatalf("%v: exit %d: %s", args, code, errw)
		}
		var d struct {
			Theme  string `json:"theme"`
			Styles [][]struct {
				X  int    `json:"x"`
				FG string `json:"fg"`
			} `json:"styles"`
		}
		mustUnmarshal(t, out, &d)
		// Row 1 is beta: .hot is off, so .cold... no; beta's text is only
		// item > text ($muted). Row 0 (alpha) has .hot: #l > item >
		// text.hot wins ($warn). Row 2 onwards: the unstyled rest.
		return d.Theme, d.Styles[0][0].FG
	}
	if th, c := fg(); th != "dark" || c != "yellow" {
		t.Errorf("auto document: theme %q, row 0 fg %q", th, c)
	}
	if th, _ := fg("--theme", "light"); th != "light" {
		t.Errorf("--theme light: theme %q", th)
	}
	code, out, _ := runCLI("dump", tui, "--theme", "auto")
	if code != 1 || out != "" {
		t.Errorf("--theme auto: exit %d, stdout %q", code, out)
	}
	// The brand token: the base value in dark, the light block's in light.
	doc := filepath.Join(t.TempDir(), "brand.tui")
	writeFile(t, doc, `<tui version="2"><style>:root { --brand: #e0443e; } @media (theme: light) { :root { --brand: #c8102e; } } #t { color: $brand; }</style><screen id="s"><text id="t">x</text></screen></tui>`)
	for theme, want := range map[string]string{"dark": "#e0443e", "light": "#c8102e"} {
		code, out, errw := runCLI("dump", doc, "--format", "json", "--styles", "--cols", "3", "--rows", "1", "--theme", theme)
		if code != 0 {
			t.Fatalf("exit %d: %s", code, errw)
		}
		if !strings.Contains(out, `"fg": "`+want+`"`) {
			t.Errorf("--theme %s: no fg %s:\n%s", theme, want, out)
		}
	}
}

// 43. play: {"theme":"light"} in a script and set:@theme="light" in
// --input agree; auto is dark; a bad value exits 1 with nothing on
// stdout; @theme beats --theme.
func TestPlayThemeSteps(t *testing.T) {
	tui, data := writeV2Fixture(t)
	script := filepath.Join(t.TempDir(), "theme.ndjson")
	writeFile(t, script, `{"theme":"light"}`+"\n")
	code, fromScript, errw := runCLI("play", tui, "--data", data, "--script", script, "--styles", "--format", "json", "--cols", "30", "--rows", "6")
	if code != 0 {
		t.Fatalf("script: exit %d: %s", code, errw)
	}
	code, fromInput, errw := runCLI("play", tui, "--data", data, "--input", `set:@theme="light"`, "--styles", "--format", "json", "--cols", "30", "--rows", "6")
	if code != 0 {
		t.Fatalf("input: exit %d: %s", code, errw)
	}
	if fromScript != fromInput || !strings.Contains(fromScript, `"theme": "light"`) {
		t.Errorf("script and input disagree, or the theme is not light:\n%s\n---\n%s", fromScript, fromInput)
	}
	code, out, _ := runCLI("play", tui, "--data", data, "--input", `set:@theme="light"`, "--theme", "dark", "--styles", "--format", "json", "--cols", "30", "--rows", "6")
	if code != 0 || !strings.Contains(out, `"theme": "light"`) {
		t.Errorf("@theme did not beat --theme dark: exit %d", code)
	}
	writeFile(t, script, `{"theme":"auto"}`+"\n")
	code, out, _ = runCLI("play", tui, "--data", data, "--script", script, "--theme", "light", "--styles", "--format", "json", "--cols", "30", "--rows", "6")
	if code != 0 || !strings.Contains(out, `"theme": "dark"`) {
		t.Errorf("{\"theme\":\"auto\"} is dark in play: exit %d", code)
	}
	for _, bad := range []string{`{"theme":"blue"}`, `{"theme":1}`, `{"theme":"light","key":"q"}`} {
		writeFile(t, script, bad+"\n")
		code, out, errw := runCLI("play", tui, "--data", data, "--script", script)
		if code != 1 || out != "" || errw == "" {
			t.Errorf("%s: exit %d, stdout %q", bad, code, out)
		}
	}
	code, out, _ = runCLI("play", tui, "--data", data, "--input", `set:@theme="blue"`)
	if code != 1 || out != "" {
		t.Errorf(`set:@theme="blue": exit %d, stdout %q`, code, out)
	}
}

// 66. inspect --json: the member order; for a guarded list-row text, its
// pseudo-classes, class entries (a guard no rule names is used: false),
// the color won by #l > item > text.hot with the losing rules listed
// highest priority first, and the inherited and initial origins.
func TestInspectJSON(t *testing.T) {
	tui, data := writeV2Fixture(t)
	code, out, errw := runCLI("inspect", tui, "--data", data, "--at", "1,0", "--cols", "30", "--rows", "6", "--json")
	if code != 0 {
		t.Fatalf("exit %d: %s\n%s", code, errw, out)
	}
	order := regexp.MustCompile(`(?m)^  "(\w+)":`).FindAllStringSubmatch(out, -1)
	var keys []string
	for _, m := range order {
		keys = append(keys, m[1])
	}
	if strings.Join(keys, " ") != "cols rows ok node path cell pseudo classes style" {
		t.Errorf("members %v", keys)
	}
	var in struct {
		Node    map[string]any `json:"node"`
		Path    string         `json:"path"`
		Cell    map[string]any `json:"cell"`
		Pseudo  []string       `json:"pseudo"`
		Classes []struct {
			Name, From, Guard string
			Active, Used      bool
		} `json:"classes"`
		Style []struct {
			Prop   string  `json:"prop"`
			Value  *string `json:"value"`
			Origin string  `json:"origin"`
			Rule   *struct {
				File, Selector string
				Line, Col      int
				Specificity    int
			} `json:"rule"`
			Overridden []struct {
				Value, Origin string
				Rule          struct {
					Selector    string
					Specificity int
				}
			} `json:"overridden"`
		} `json:"style"`
	}
	mustUnmarshal(t, out, &in)
	if in.Path != "/screen#main/list#l/item[0]/text" || in.Node["tag"] != "text" || in.Node["text"] != "alpha" {
		t.Errorf("path %q node %v", in.Path, in.Node)
	}
	if in.Cell["ch"] != "l" || in.Cell["id"] != "l" {
		t.Errorf("cell %v", in.Cell)
	}
	if strings.Join(in.Pseudo, " ") != "" {
		t.Errorf("pseudo %v", in.Pseudo)
	}
	cls := ""
	for _, c := range in.Classes {
		cls += c.Name + "/" + c.From + "/" + c.Guard + "/" + map[bool]string{true: "on", false: "off"}[c.Active] + "/" + map[bool]string{true: "used", false: "unused"}[c.Used] + " "
	}
	if cls != "name/class//on/unused hot/class:hot/r.hot/on/used cold/class:cold/r.cold/off/unused " {
		t.Errorf("classes %s", cls)
	}
	if len(in.Style) != 33 || in.Style[0].Prop != "layout" || in.Style[32].Prop != "bar" {
		t.Fatalf("style has %d entries", len(in.Style))
	}
	for _, s := range in.Style {
		switch s.Prop {
		case "color":
			if s.Origin != "author" || s.Rule == nil || s.Rule.Selector != "#l > item > text.hot" || s.Rule.Specificity != 112 || *s.Value != "yellow" {
				t.Errorf("color %+v %v", s, s.Rule)
			}
			var lost []string
			for _, l := range s.Overridden {
				lost = append(lost, l.Rule.Selector+"="+l.Value)
			}
			if strings.Join(lost, " ") != ".hot=$brand item > text=$muted" {
				t.Errorf("color overrides %v", lost)
			}
		case "italic":
			if s.Origin != "inherited" || s.Rule != nil {
				t.Errorf("italic %+v", s)
			}
		case "border":
			if s.Origin != "initial" || *s.Value != "none" {
				t.Errorf("border %+v", s)
			}
		case "min-width", "layout":
			if s.Origin != "initial" || s.Value != nil {
				t.Errorf("%s %+v", s.Prop, s)
			}
		case "width":
			if s.Origin != "ua" || s.Rule.File != "<ua>" || s.Rule.Selector != "text" {
				t.Errorf("width %+v", s)
			}
		}
	}
	// The node is a dump node: it validates against the dump schema's node.
	schema := loadSchema(t, dumpV02Schema)
	nodeDef, err := resolveRef(schema, "#/$defs/node")
	if err != nil {
		t.Fatal(err)
	}
	var errs []string
	validate(schema, nodeDef, decodeJSON(t, mustJSON(t, in.Node)), "/node", &errs)
	if len(errs) > 0 {
		t.Errorf("inspect node vs dump schema: %v", errs)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// 66. inspect: attributes and style="" name themselves; --id finds the
// node; --at outside the grid, an unknown --id, both or neither flag, and
// a bad --at exit 1 with nothing on stdout; a frame with an error exits 2
// and still prints the report.
func TestInspectUsageAndOrigins(t *testing.T) {
	doc := filepath.Join(t.TempDir(), "o.tui")
	writeFile(t, doc, `<tui version="1"><style>#b { width: 10; }</style><screen id="s"><box id="b" width="30%" style="width: 5" pad="1"><text>hi</text></box></screen></tui>`)
	code, out, errw := runCLI("inspect", doc, "--id", "b", "--json")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errw)
	}
	var in struct {
		Pseudo []string
		Style  []struct {
			Prop, Origin string
			Rule         struct {
				Selector    string
				Specificity int
				Line, Col   int
			}
			Overridden []struct {
				Origin string
				Rule   struct{ Selector string }
			}
		}
	}
	mustUnmarshal(t, out, &in)
	for _, s := range in.Style {
		switch s.Prop {
		case "width":
			if s.Origin != "inline" || s.Rule.Selector != `style=""` || s.Rule.Specificity != 1000 {
				t.Errorf("width %+v", s)
			}
			var lost []string
			for _, l := range s.Overridden {
				lost = append(lost, l.Origin+":"+l.Rule.Selector)
			}
			if strings.Join(lost, " ") != `author:#b attribute:width="30%"` {
				t.Errorf("width overrides %v", lost)
			}
		case "padding":
			if s.Origin != "attribute" || s.Rule.Selector != `pad="1"` || s.Rule.Specificity != 0 || s.Rule.Line != 1 {
				t.Errorf("padding %+v", s)
			}
		}
	}
	if in.Pseudo == nil {
		t.Error("pseudo must be [] when none match")
	}
	text, _, _ := runCLI("inspect", doc, "--id", "b")
	_ = text
	code, out, _ = runCLI("inspect", doc, "--id", "b")
	if code != 0 || !strings.HasPrefix(out, "b  box  ") || !strings.Contains(out, "path: /screen#s/box#b\n") || !strings.Contains(out, `width: 5  inline  o.tui:1:`) {
		t.Errorf("text form:\n%s", out)
	}
	for _, args := range [][]string{
		{"--at", "80,0"}, {"--at", "0,24"}, {"--at", "-1,0"}, {"--at", "x,1"}, {"--at", "1"},
		{"--id", "nope"}, {"--id", "b", "--at", "0,0"}, {}, {"--id", ""},
	} {
		code, out, errw := runCLI(append([]string{"inspect", doc}, args...)...)
		if code != 1 || out != "" || errw == "" {
			t.Errorf("%v: exit %d, stdout %q", args, code, out)
		}
	}
	bad := filepath.Join(t.TempDir(), "bad.tui")
	writeFile(t, bad, `<tui version="1"><screen id="s"><text id="t">hi</text><widget/></screen></tui>`)
	code, out, _ = runCLI("inspect", bad, "--id", "t", "--json")
	if code != 2 || !strings.Contains(out, `"ok": false`) {
		t.Errorf("a frame with errors: exit %d\n%s", code, out)
	}
}

// §15.5: the manifest's cells field puts the --cells map in the JSON
// goldens.
func TestManifestCellsField(t *testing.T) {
	dir := t.TempDir()
	tui := filepath.Join(dir, "app.tui")
	writeFile(t, tui, `<tui version="1"><screen id="main"><text id="t">hi</text></screen></tui>`)
	writeManifest(t, dir, `[{"name":"e","file":"`+tui+`","sizes":["4x1"],"json":["4x1"],"frozen":false,"cells":true}]`)
	if code, out, errw := runCLI("test", dir, "--update"); code != 0 {
		t.Fatalf("exit %d: %s %s", code, out, errw)
	}
	b, err := os.ReadFile(filepath.Join(dir, "e", "4x1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"cells": [`) || !strings.Contains(string(b), `"id": "t"`) {
		t.Errorf("golden without cells:\n%s", b)
	}
	if code, out, _ := runCLI("test", dir); code != 0 {
		t.Errorf("re-run: exit %d: %s", code, out)
	}
}

// §15.1: fmt puts class:NAME attributes right after class, in source
// order, and formats a <column> body like a <text> body.
func TestFmtClassGuardsAndColumns(t *testing.T) {
	src := "<tui version=\"2\"><screen id=\"s\"><text if=\"x\" class:b=\"y\" id=\"t\" class:a=\"z\" class=\"c\">\n   hi\n</text><table id=\"tb\" each=\"xs as r\"><column title=\"N\">\n  {r.n}\n  </column></table></screen></tui>\n"
	p := filepath.Join(t.TempDir(), "f.tui")
	writeFile(t, p, src)
	code, out, errw := runCLI("fmt", p)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errw)
	}
	if !strings.Contains(out, `<text id="t" class="c" class:b="y" class:a="z" if="x">hi</text>`) {
		t.Errorf("class guards not after class:\n%s", out)
	}
	if !strings.Contains(out, `<column title="N">{r.n}</column>`) {
		t.Errorf("column body not inline:\n%s", out)
	}
}
