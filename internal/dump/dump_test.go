package dump

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/layout"
	"github.com/abdul-hamid-achik/tuimark/internal/paint"
)

// frame builds a small laid-out tree by hand:
//
//	screen#main 6x3
//	  input#q 6x1 (focused, secret "abc")
//	  list#l 6x2
//	    item (selected, key t-1) > text "one"
//	    item (key t-2)           (not laid out)
//	modal#m 4x1 > button "OK"
func frame() (*layout.Box, []*layout.Box, *paint.Grid) {
	root := &layout.Box{Tag: "screen", Kind: "screen", ID: "main", W: 6, H: 3, Laid: true}
	in := &layout.Box{Tag: "input", Kind: "input", ID: "q", W: 6, H: 1, Laid: true, Focused: true, Secret: true, Text: "abc"}
	list := &layout.Box{Tag: "list", Kind: "list", ID: "l", Y: 1, W: 6, H: 2, Laid: true}
	it1 := &layout.Box{Tag: "item", Kind: "item", Y: 1, W: 6, H: 1, Laid: true, Selected: true, Key: "t-1"}
	t1 := &layout.Box{Tag: "text", Kind: "text", Y: 1, W: 3, H: 1, Laid: true, Text: "one"}
	it2 := &layout.Box{Tag: "item", Kind: "item", Key: "t-2"}
	it1.Children = []*layout.Box{t1}
	list.Children = []*layout.Box{it1, it2}
	root.Children = []*layout.Box{in, list}
	m := &layout.Box{Tag: "modal", Kind: "modal", ID: "m", X: 1, Y: 1, W: 4, H: 1, Laid: true}
	m.Children = []*layout.Box{{Tag: "button", Kind: "button", X: 1, Y: 1, W: 4, H: 1, Laid: true, Text: "OK"}}
	g := paint.NewGrid(6, 3)
	g.At(0, 0).Ch = '•'
	g.At(0, 0).Owner = "q"
	return root, []*layout.Box{m}, g
}

func TestBuild(t *testing.T) {
	root, modals, g := frame()
	d := Build(6, 3, root, modals, g, nil, "q", false)
	if d.Cols != 6 || d.Rows != 3 || !d.OK || d.Focus == nil || *d.Focus != "q" {
		t.Fatalf("header = %+v", d)
	}
	if d.Errors == nil || len(d.Errors) != 0 {
		t.Errorf("errors must be an empty list, got %#v", d.Errors)
	}
	var got []string
	for _, n := range d.Nodes {
		got = append(got, n.Tag+"#"+n.ID)
	}
	if strings.Join(got, " ") != "screen#main input#q list#l item# text# modal#m button#" {
		t.Errorf("nodes = %v (every laid-out node, modals last, unlaid skipped)", got)
	}
	if in := d.Nodes[1]; in.Text != "•••" || !in.Focused {
		t.Errorf("secret input = %+v", in)
	}
	if it := d.Nodes[3]; !it.Selected || it.Key != "t-1" {
		t.Errorf("item = %+v", it)
	}
	if d.Nodes[4].Text != "one" || d.Nodes[6].Text != "OK" {
		t.Errorf("text fields: %+v %+v", d.Nodes[4], d.Nodes[6])
	}
	if len(d.Grid) != 3 || d.Grid[0] != "•     " || d.Cells != nil {
		t.Errorf("grid = %q cells %v", d.Grid, d.Cells)
	}

	// --cells lists every cell with its owner id.
	d = Build(6, 3, root, modals, g, nil, "", true)
	if len(d.Cells) != 18 || d.Cells[0] != (Cell{X: 0, Y: 0, Ch: "•", ID: "q"}) || d.Cells[7] != (Cell{X: 1, Y: 1, Ch: " "}) {
		t.Errorf("cells = %+v", d.Cells[:8])
	}
	if d.Focus != nil {
		t.Error("no focus is null")
	}

	// ok is false only with an error-severity diagnostic.
	warn := ir.Diags{{Severity: ir.Warning, Code: "B006", Msg: "w"}}
	if d := Build(6, 3, root, nil, g, warn, "", false); !d.OK || len(d.Errors) != 1 {
		t.Errorf("warnings keep ok=true: %+v", d)
	}
	errs := append(warn, ir.Diagnostic{Severity: ir.Error, Code: "V001", Msg: "e"})
	if d := Build(6, 3, root, nil, g, errs, "", false); d.OK || len(d.Errors) != 2 {
		t.Errorf("errors make ok=false: %+v", d)
	}

	// A failed parse still produces a blank grid of the right shape.
	d = Build(4, 2, nil, nil, nil, errs, "", false)
	if len(d.Nodes) != 0 || d.Nodes == nil || len(d.Grid) != 2 || d.Grid[1] != "    " {
		t.Errorf("empty dump = %+v", d)
	}
}

func TestJSON(t *testing.T) {
	root, modals, g := frame()
	d := Build(6, 3, root, modals, g, ir.Diags{{Severity: ir.Warning, Code: "B006", Msg: "no key", Line: 2, Col: 3}}, "", false)
	b, err := JSON(d)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(b), "}\n") || !strings.HasPrefix(string(b), "{\n  \"cols\": 6,") {
		t.Errorf("JSON is indented with a trailing newline:\n%s", b)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"cols", "rows", "ok", "focus", "errors", "nodes", "grid"} {
		if _, ok := m[k]; !ok {
			t.Errorf("missing %q", k)
		}
	}
	if _, ok := m["cells"]; ok {
		t.Error("cells is omitted unless requested")
	}
	if m["focus"] != nil {
		t.Error("focus is null")
	}
	nodes := m["nodes"].([]any)
	first := nodes[3].(map[string]any)
	if _, ok := first["id"]; ok {
		t.Error("anonymous nodes omit id")
	}
	if first["selected"] != true || first["key"] != "t-1" {
		t.Errorf("item node = %v", first)
	}
	e := m["errors"].([]any)[0].(map[string]any)
	if e["severity"] != "warning" || e["code"] != "B006" || e["line"] != 2.0 || e["col"] != 3.0 {
		t.Errorf("diagnostic = %v", e)
	}
	if _, ok := e["file"]; ok {
		t.Error("empty file is omitted")
	}
	grid := m["grid"].([]any)
	if len(grid) != 3 || grid[0] != "•     " {
		t.Errorf("grid = %v", grid)
	}
}

func TestText(t *testing.T) {
	root, modals, g := frame()
	d := Build(6, 3, root, modals, g, nil, "q", false)
	want := strings.Join([]string{
		"=== grid 6x3 ===",
		"•     ",
		"      ",
		"      ",
		"=== nodes ===",
		"main  screen  6x3 @(0,0)",
		"q     input   6x1 @(0,0) focus",
		"l     list    6x2 @(0,1)",
		"-     item    6x1 @(0,1) selected",
		"-     text    3x1 @(0,1)",
		"m     modal   4x1 @(1,1)",
		"-     button  4x1 @(1,1)",
		"=== errors ===",
		"ok",
		"",
	}, "\n")
	if got := Text(d); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	d = Build(6, 3, nil, nil, g, ir.Diags{
		{Severity: ir.Error, Code: "V001", Msg: "unknown tag <widget>", File: "app.tui", Line: 3, Col: 1, Path: "/screen/widget"},
		{Severity: ir.Warning, Code: "L005", Msg: "two bottom docks"},
	}, "", false)
	text := Text(d)
	if !strings.HasSuffix(text, "=== nodes ===\n=== errors ===\nerror V001 app.tui:3:1 /screen/widget unknown tag <widget>\nwarning L005 two bottom docks\n") {
		t.Errorf("errors section:\n%s", text)
	}
}
