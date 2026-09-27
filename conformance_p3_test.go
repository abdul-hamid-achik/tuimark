package tuimark_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

// SPEC v0.2b §21 conformance tests of the table phase through the public
// API: the §6.9.5 literal fixture (50), and scrollbar and bar: eighths in
// a version="2" document (60, 61). Keys, the offset, the cache, and the
// diagnostics are covered in internal/host (table_test.go), the column
// arithmetic in internal/layout, the paint vectors in internal/paint, and
// the golden (manifest entry "table", 30x5) by `tuimark test`.

// 50. The §6.9.5 fixture dumps exactly the grid and the nodes given there,
// with focus "t" and "wide": true.
func TestTableSpecFixture(t *testing.T) {
	app, err := tuimark.Load("specs/fixtures/table.tui")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("specs/fixtures/table.json")
	if err != nil {
		t.Fatal(err)
	}
	var data any
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	if err := app.Bind("", data); err != nil {
		t.Fatal(err)
	}
	d, err := app.Dump(30, 5)
	if err != nil {
		t.Fatal(err)
	}
	wantGrid := []string{
		"  PID Name                CPU%",
		"    1 launchd              0.5",
		"    7 微信                  12",
		"   42 a-very-long-proces… 3.25",
		"                              ",
	}
	if strings.Join(d.Grid, "\n") != strings.Join(wantGrid, "\n") {
		t.Errorf("grid:\n%s\nwant\n%s", strings.Join(d.Grid, "\n"), strings.Join(wantGrid, "\n"))
	}
	wantNodes := []string{
		`{"id":"main","tag":"screen","x":0,"y":0,"w":30,"h":5}`,
		`{"id":"t","tag":"table","x":0,"y":0,"w":30,"h":4,"focused":true,"scroll":{"y":0,"h":3}}`,
		`{"id":"c-pid","tag":"column","x":0,"y":0,"w":5,"h":1,"text":"PID","classes":["num"]}`,
		`{"id":"c-name","tag":"column","x":6,"y":0,"w":19,"h":1,"text":"Name"}`,
		`{"id":"c-cpu","tag":"column","x":26,"y":0,"w":4,"h":1,"text":"CPU%","classes":["num"]}`,
		`{"tag":"item","x":0,"y":1,"w":30,"h":1,"key":"1"}`,
		`{"tag":"text","x":0,"y":1,"w":5,"h":1,"text":"1","classes":["num"]}`,
		`{"tag":"text","x":6,"y":1,"w":19,"h":1,"text":"launchd"}`,
		`{"tag":"text","x":26,"y":1,"w":4,"h":1,"text":"0.5","classes":["num"]}`,
		`{"tag":"item","x":0,"y":2,"w":30,"h":1,"key":"7","selected":true}`,
		`{"tag":"text","x":0,"y":2,"w":5,"h":1,"text":"7","classes":["num"]}`,
		`{"tag":"text","x":6,"y":2,"w":19,"h":1,"text":"微信"}`,
		`{"tag":"text","x":26,"y":2,"w":4,"h":1,"text":"12","classes":["num"]}`,
		`{"tag":"item","x":0,"y":3,"w":30,"h":1,"key":"42"}`,
		`{"tag":"text","x":0,"y":3,"w":5,"h":1,"text":"42","classes":["num"]}`,
		`{"tag":"text","x":6,"y":3,"w":19,"h":1,"text":"a-very-long-process-name"}`,
		`{"tag":"text","x":26,"y":3,"w":4,"h":1,"text":"3.25","classes":["num"]}`,
	}
	var got []string
	for _, n := range d.Nodes {
		b, err := json.Marshal(n)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, string(b))
	}
	if strings.Join(got, "\n") != strings.Join(wantNodes, "\n") {
		t.Errorf("nodes:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(wantNodes, "\n"))
	}
	if d.Focus == nil || *d.Focus != "t" || !d.Wide || !d.OK || len(d.Errors) != 0 {
		t.Errorf("focus %v wide %v ok %v errors %v", d.Focus, d.Wide, d.OK, d.Errors)
	}
}

// 60, 61. In a version="2" document, scrollbar: auto paints the thumb on a
// list's right border and bar: eighths paints eighths; version="1"
// documents cannot name either (V003, the version gate) and paint as in
// 0.2a.
func TestScrollbarAndEighthsThroughTheAPI(t *testing.T) {
	app, err := tuimark.Parse(strings.NewReader(`<tui version="2"><style>
#l { border: single; scrollbar: auto; height: 6; }
#p { bar: eighths; width: 10; }
</style><screen id="s"><list id="l" each="xs as x" key="x"><item><text>{x}</text></item></list><progress id="p" value="33"/></screen></tui>`))
	if err != nil {
		t.Fatal(err)
	}
	var xs []any
	for i := 0; i < 20; i++ {
		xs = append(xs, float64(i))
	}
	_ = app.Bind("xs", xs)
	d, err := app.Dump(8, 7)
	if err != nil {
		t.Fatal(err)
	}
	// track 4, view 4, content 20: length floor(16/20) → 1, at offset 0.
	if d.Grid[1] != "│0     ┃" || d.Grid[2] != "│1     │" || d.Grid[6] != "███▎░░░░" {
		t.Errorf("grid %q", d.Grid)
	}
}
