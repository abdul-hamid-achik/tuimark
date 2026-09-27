package main

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// SPEC v0.2b ergonomics through the CLI: play runs the key dispatch of
// §8.6 with the built-in actions of §8.4 and records the events they fire
// (on:select, on:change), not the actions; the dump carries checked
// (§13.2) and the text form its " checked" marker (§13.3).

const p2Fixture = `<tui version="2">
  <keymap>
    <bind keys="j" action="move-next"/>
    <bind keys="space" action="check-toggle"/>
    <bind keys="2" action="switch-to" to="#two"/>
    <bind keys="x" action="on_one" when="#one"/>
    <bind keys="x" action="on_two" when="#two"/>
  </keymap>
  <screen id="one" focus="#l">
    <list id="l" each="rows as r" key="r" checked="m" mark="✓" on:select="sel" on:change="marks">
      <item><text>{r}</text></item>
    </list>
    <row id="tags" each="rows as t"><text class="tag">{t}</text></row>
  </screen>
  <screen id="two"><text>two</text></screen>
</tui>
`

func writeP2Fixture(t *testing.T) (tui, data string) {
	t.Helper()
	dir := t.TempDir()
	tui = filepath.Join(dir, "p2.tui")
	data = filepath.Join(dir, "p2.json")
	writeFile(t, tui, p2Fixture)
	writeFile(t, data, `{"rows":["a","b"],"m":[]}`)
	return tui, data
}

// The same events as Run's loop gives for the same keys
// (internal/host TestLoopBuiltinsInOneRead), whether the keys come as
// separate steps or in one text: read.
func TestPlayBuiltinActions(t *testing.T) {
	tui, data := writeP2Fixture(t)
	want := []map[string]any{
		{"step": 1.0, "action": "sel", "source": "l", "keys": map[string]any{"r": "b"}, "value": nil},
		{"step": 2.0, "action": "marks", "source": "l", "keys": map[string]any{"r": "b"}, "value": []any{"b"}},
		{"step": 4.0, "action": "on_two", "source": "", "keys": map[string]any{}, "value": nil},
	}
	script := filepath.Join(t.TempDir(), "keys.ndjson")
	writeFile(t, script, `{"text":"j 2x"}`+"\n")
	for _, input := range []string{"j space 2 x", "script"} {
		args := []string{"play", tui, "--data", data, "--input", input, "--format", "json", "--cols", "20", "--rows", "4"}
		if input == "script" {
			args = []string{"play", tui, "--data", data, "--script", script, "--format", "json", "--cols", "20", "--rows", "4"}
		}
		code, out, errw := runCLI(args...)
		if code != 0 {
			t.Fatalf("%s: exit %d: %s", input, code, errw)
		}
		var got struct {
			Events []map[string]any `json:"events"`
			Grid   []string         `json:"grid"`
		}
		if err := json.Unmarshal([]byte(out), &got); err != nil {
			t.Fatal(err)
		}
		if input != "j space 2 x" {
			for i := range want {
				want[i]["step"] = 1.0
			}
		}
		if !reflect.DeepEqual(got.Events, want) {
			t.Errorf("%s: events\n got %v\nwant %v", input, got.Events, want)
		}
		if got.Grid[0] != "two                 " {
			t.Errorf("%s: switch-to: grid %q", input, got.Grid)
		}
	}
}

// 64. The dump's checked member (valid against dump.v0.2.json), the text
// form's " checked" after " selected", the mark in the grid, and the key
// of a container each template's generated nodes.
func TestDumpCheckedAndEachKeys(t *testing.T) {
	tui, _ := writeP2Fixture(t)
	data := filepath.Join(t.TempDir(), "checked.json")
	writeFile(t, data, `{"rows":["a","b"],"m":["a"]}`)
	code, out, errw := runCLI("dump", tui, "--data", data, "--format", "json", "--cols", "12", "--rows", "4")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errw)
	}
	if errs := validateSchema(t, dumpV02Schema, decodeJSON(t, []byte(out))); len(errs) > 0 {
		t.Errorf("dump vs dump.v0.2.json: %v", errs)
	}
	var d struct {
		Grid  []string         `json:"grid"`
		Nodes []map[string]any `json:"nodes"`
	}
	if err := json.Unmarshal([]byte(out), &d); err != nil {
		t.Fatal(err)
	}
	if d.Grid[0] != "✓ a         " || d.Grid[1] != "  b         " {
		t.Errorf("grid %q", d.Grid)
	}
	var checked, tags []string
	for _, n := range d.Nodes {
		if n["checked"] == true {
			checked = append(checked, n["key"].(string))
		}
		if n["tag"] == "text" && n["key"] != nil {
			tags = append(tags, n["key"].(string)+"="+n["text"].(string))
		}
	}
	if strings.Join(checked, " ") != "a" || strings.Join(tags, " ") != "0=a 1=b" {
		t.Errorf("checked %v, template keys %v", checked, tags)
	}
	code, out, _ = runCLI("dump", tui, "--data", data, "--cols", "12", "--rows", "4")
	if code != 0 || !strings.Contains(out, "12x1 @(0,0) selected checked\n") {
		t.Errorf("text form:\n%s", out)
	}
}

// 62. validate --catalog never reports a built-in as B004, and B007 is
// static: reported without --data.
func TestValidateBuiltinsCatalogAndB007(t *testing.T) {
	tui, data := writeP2Fixture(t)
	catalog := writeTemp(t, "catalog.json", `["sel","marks","on_one","on_two"]`)
	code, out, errw := runCLI("validate", tui, "--data", data, "--catalog", catalog)
	if code != 0 || strings.Contains(out, "B004") {
		t.Errorf("exit %d: %s%s", code, out, errw)
	}
	bad := writeTemp(t, "b007.tui", `<tui version="2"><keymap><bind keys="j" action="move-next" to="#t"/></keymap><screen id="s"><text id="t">x</text></screen></tui>`)
	code, out, _ = runCLI("validate", bad)
	if code != 2 || !strings.Contains(out, "error B007") {
		t.Errorf("exit %d: %s", code, out)
	}
}

// 68. The IR of a document with each on a container, checked, mark, and
// the built-ins validates against ir.v0.2.json; the container's each uses
// the existing each field (§13.1).
func TestIRContainerEach(t *testing.T) {
	tui, _ := writeP2Fixture(t)
	code, out, errw := runCLI("ir", tui)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errw)
	}
	if errs := validateSchema(t, irV02Schema, decodeJSON(t, []byte(out))); len(errs) > 0 {
		t.Errorf("ir vs ir.v0.2.json: %v", errs)
	}
	if !strings.Contains(out, `"each": "rows as t"`) {
		t.Errorf("the row's each is not in the IR:\n%s", out)
	}
}
