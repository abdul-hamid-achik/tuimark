package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// SPEC v0.2b tables through the CLI: play moves a table's cursor with the
// keys a focused table consumes and records on:select with {alias: key}
// (§6.9.2, §8.2), the dump with the visible rows validates against
// schema/dump.v0.2.json (68), and inspect explains a body cell: its
// column's classes and guards, and the rule that won its color (66).

const p3Fixture = `<tui version="2">
  <style>
    .hot { color: yellow; }
    #procs > item > text.hot { color: red; }
    item > text { color: green; }
    #procs { border: single; scrollbar: auto; }
  </style>
  <screen id="main" focus="#procs">
    <table id="procs" each="rows as p" key="p.pid" bind="cur" on:select="moved" placeholder="none">
      <item class:blocked="p.protected"/>
      <column id="c-pid" title="PID" width="4">{p.pid}</column>
      <column id="c-cpu" class="num" title="CPU" class:hot="p.hot" class:spare="p.hot">{p.cpu}</column>
    </table>
  </screen>
</tui>
`

const p3Data = `{"cur":null,"rows":[{"pid":1,"cpu":"5","hot":false,"protected":true},{"pid":2,"cpu":"97","hot":true,"protected":false},{"pid":3,"cpu":"0","hot":false,"protected":false},{"pid":4,"cpu":"1","hot":false,"protected":false}]}`

func writeP3Fixture(t *testing.T) (tui, data string) {
	t.Helper()
	dir := t.TempDir()
	tui, data = filepath.Join(dir, "p3.tui"), filepath.Join(dir, "p3.json")
	writeFile(t, tui, p3Fixture)
	writeFile(t, data, p3Data)
	return tui, data
}

func TestPlayTable(t *testing.T) {
	tui, data := writeP3Fixture(t)
	code, out, errw := runCLI("play", tui, "--data", data, "--input", "down end up home up", "--format", "json", "--cols", "12", "--rows", "6")
	if code != 0 {
		t.Fatalf("exit %d: %s\n%s", code, errw, out)
	}
	var p struct {
		Grid   []string `json:"grid"`
		Events []struct {
			Step   int
			Action string
			Source string
			Keys   map[string]any
		} `json:"events"`
	}
	mustUnmarshal(t, out, &p)
	var evs []string
	for _, e := range p.Events {
		evs = append(evs, e.Action+" "+e.Source+" "+strings.TrimSuffix(strings.TrimPrefix(jsonOf(t, e.Keys), "{"), "}"))
	}
	if got := strings.Join(evs, "\n"); got != "moved procs \"p\":2\nmoved procs \"p\":4\nmoved procs \"p\":3\nmoved procs \"p\":1" {
		t.Errorf("events:\n%s", got)
	}
	// Outer 6 rows: header + V = 3; the cursor is back on row 0.
	if p.Grid[1] != "│PID CPU   ┃" || p.Grid[2] != "│1   5     ┃" || p.Grid[4] != "│3   0     │" {
		t.Errorf("grid %q", p.Grid)
	}
	if errs := validateSchema(t, dumpV02Schema, decodeJSON(t, []byte(out))); len(errs) > 0 {
		t.Errorf("play output vs dump.v0.2.json: %v", errs)
	}
}

func TestInspectTableCell(t *testing.T) {
	tui, data := writeP3Fixture(t)
	code, out, errw := runCLI("inspect", tui, "--data", data, "--at", "5,3", "--cols", "12", "--rows", "6", "--json")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errw)
	}
	var in struct {
		Node    map[string]any `json:"node"`
		Path    string         `json:"path"`
		Classes []struct {
			Name, From, Guard string
			Active, Used      bool
		} `json:"classes"`
		Style []struct {
			Prop       string  `json:"prop"`
			Value      *string `json:"value"`
			Rule       *struct{ Selector string }
			Overridden []struct{ Value string }
		} `json:"style"`
	}
	mustUnmarshal(t, out, &in)
	if in.Path != "/screen#main/table#procs/item[1]/text[1]" || in.Node["text"] != "97" {
		t.Errorf("path %q node %v", in.Path, in.Node)
	}
	var cls []string
	for _, c := range in.Classes {
		cls = append(cls, c.Name+"/"+c.From+"/"+c.Guard+"/"+map[bool]string{true: "on", false: "off"}[c.Active]+"/"+map[bool]string{true: "used", false: "unused"}[c.Used])
	}
	if got := strings.Join(cls, " "); got != "num/class//on/unused hot/class:hot/p.hot/on/used spare/class:spare/p.hot/on/unused" {
		t.Errorf("classes %s", got)
	}
	for _, s := range in.Style {
		if s.Prop == "color" && (s.Rule == nil || s.Rule.Selector != "#procs > item > text.hot" || *s.Value != "red" || len(s.Overridden) != 2) {
			t.Errorf("color %+v", s)
		}
	}
	// The header cell of the same column: static classes only, guards off.
	_, out, _ = runCLI("inspect", tui, "--data", data, "--id", "c-cpu", "--cols", "12", "--rows", "6", "--json")
	mustUnmarshal(t, out, &in)
	if in.Node["text"] != "CPU" || len(in.Classes) != 3 || in.Classes[1].Active || in.Classes[2].Active {
		t.Errorf("header %v %+v", in.Node, in.Classes)
	}
}

func jsonOf(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
