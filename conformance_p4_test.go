package tuimark_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

// SPEC v0.2b §21 conformance tests of the tabs, hints, sparkline, and grid
// phase through the public API: the literal fixtures of §6.10.5 (54),
// §6.11 (56), §6.12 (57), and §11.7 (58), which `tuimark test` also pins
// (manifest entries "tabs", "hints", "hints-filter", and "grid"). The
// focus rules, the dispatch, and the diagnostics are covered in
// internal/host and internal/parse, the arithmetic in internal/layout and
// internal/paint.

// loadFixture loads specs/fixtures/NAME.tui with NAME.json bound.
func loadFixture(t *testing.T, name string) *tuimark.App {
	t.Helper()
	app, err := tuimark.Load("specs/fixtures/" + name + ".tui")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("specs/fixtures/" + name + ".json")
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
	return app
}

// compactNodes is each dump node as compact JSON.
func compactNodes(t *testing.T, d *tuimark.Dump) []string {
	t.Helper()
	var out []string
	for _, n := range d.Nodes {
		b, err := json.Marshal(n)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, string(b))
	}
	return out
}

func sameLines(t *testing.T, what string, got, want []string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("%s:\n%s\nwant\n%s", what, strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// 54. The §6.10.5 fixture: the strip row at 40, 30, 20, 12, and 8 columns
// (tiers 1, 2, 3, 3, 3), and the nodes at 40x3.
func TestTabsSpecFixture(t *testing.T) {
	app := loadFixture(t, "tabs")
	for cols, want := range map[int]string{
		40: " 1 overview  ▸7 processes   8 settings  ",
		30: " 1 ovr  ▸7 proc   8 cfg       ",
		20: "▸‹ 7 processes ›    ",
		12: "▸‹ 7 proc › ",
		8:  "▸‹ 7 pr…",
	} {
		d, err := app.Dump(cols, 3)
		if err != nil {
			t.Fatal(err)
		}
		if d.Grid[0] != want || !d.OK {
			t.Errorf("%d cols: grid[0] %q, want %q (errors %v)", cols, d.Grid[0], want, d.Errors)
		}
	}
	d, err := app.Dump(40, 3)
	if err != nil {
		t.Fatal(err)
	}
	sameLines(t, "nodes", compactNodes(t, d), []string{
		`{"id":"main","tag":"screen","x":0,"y":0,"w":40,"h":3}`,
		`{"id":"nav","tag":"tabs","x":0,"y":0,"w":40,"h":2}`,
		`{"tag":"text","x":0,"y":0,"w":11,"h":1,"text":"1 overview","key":"overview","classes":["tab-label"]}`,
		`{"tag":"text","x":13,"y":0,"w":12,"h":1,"text":"7 processes","key":"processes","selected":true,"classes":["tab-label"]}`,
		`{"tag":"text","x":27,"y":0,"w":11,"h":1,"text":"8 settings","key":"settings","classes":["tab-label"]}`,
		`{"id":"processes","tag":"tab","x":0,"y":1,"w":40,"h":1}`,
		`{"tag":"text","x":0,"y":1,"w":40,"h":1,"text":"processes panel"}`,
	})
	sameLines(t, "grid", d.Grid, []string{
		" 1 overview  ▸7 processes   8 settings  ",
		"processes panel                         ",
		"                                        ",
	})
}

// 56. The §6.11 vectors through a document: min and max, a null gap, the
// last W values right-aligned, a flat series, and H = 2.
func TestSparklineSpecVectors(t *testing.T) {
	for _, c := range []struct {
		attrs, data string
		w, h        int
		want        []string
	}{
		{`min="0" max="8"`, `[0, 1, 4, 8, null, 9]`, 6, 1, []string{" ▁▄█ █"}},
		{``, `[3, 7, 1, 9, 4]`, 8, 1, []string{"   ▂▆ █▃"}},
		{``, `[5, 5, 5]`, 3, 1, []string{"▄▄▄"}},
		{`min="0" max="100"`, `[10, 20, 30, 40, 50, 60, 70, 80, 90, 100, 0]`, 4, 1, []string{"▆▇█ "}},
		{`min="0" max="16" height="2"`, `[0, 2, 4, 6, 8, 10, 12, 14, 16]`, 9, 2, []string{"     ▂▄▆█", " ▂▄▆█████"}},
	} {
		app, err := tuimark.Parse(strings.NewReader(`<tui version="2"><screen id="s"><sparkline bind="h" ` + c.attrs + `/></screen></tui>`))
		if err != nil {
			t.Fatal(err)
		}
		var h any
		_ = json.Unmarshal([]byte(c.data), &h)
		_ = app.Bind("h", h)
		d, err := app.Dump(c.w, c.h)
		if err != nil {
			t.Fatal(err)
		}
		sameLines(t, c.data, d.Grid, c.want)
	}
}

// 57. The §6.12 fixture: grid[3] at 40 and 20 columns with #items
// focused, and at 40 after focus:#filter (the input consumes q, /, space,
// and ctrl+a; esc now matches its when).
func TestHintsSpecFixture(t *testing.T) {
	app := loadFixture(t, "hints")
	for cols, want := range map[int]string{
		40: "q quit  / filter  space mark  ^A all    ",
		20: "q quit  / filter    ",
	} {
		d, err := app.Dump(cols, 4)
		if err != nil {
			t.Fatal(err)
		}
		if d.Grid[3] != want || d.Focus == nil || *d.Focus != "items" {
			t.Errorf("%d cols: grid[3] %q, want %q", cols, d.Grid[3], want)
		}
		if d.Grid[0] != "filter"+strings.Repeat(" ", cols-6) || !strings.HasPrefix(d.Grid[1], "alpha") || !strings.HasPrefix(d.Grid[2], "beta") {
			t.Errorf("%d cols: rows 0-2 %q", cols, d.Grid[:3])
		}
	}
	if err := app.Set("@focus", "#filter"); err != nil {
		t.Fatal(err)
	}
	d, err := app.Dump(40, 4)
	if err != nil {
		t.Fatal(err)
	}
	if d.Grid[3] != "esc cancel"+strings.Repeat(" ", 30) {
		t.Errorf("after focus:#filter: grid[3] %q", d.Grid[3])
	}
}

// 58. The §11.7 fixture at 20 and 30 columns: the grid and the geometry.
func TestGridSpecFixture(t *testing.T) {
	app := loadFixture(t, "grid")
	d, err := app.Dump(20, 5)
	if err != nil {
		t.Fatal(err)
	}
	sameLines(t, "grid at 20", d.Grid, []string{
		"alpha     beta      ",
		"                    ",
		"gamma     delta     ",
		"          +1        ",
		"                    ",
	})
	var geo []string
	for _, n := range d.Nodes {
		if n.ID == "g" || n.Tag == "col" {
			b, _ := json.Marshal(n)
			geo = append(geo, string(b))
		}
	}
	sameLines(t, "nodes at 20", geo, []string{
		`{"id":"g","tag":"box","x":0,"y":0,"w":20,"h":4}`,
		`{"tag":"col","x":0,"y":0,"w":9,"h":1,"key":"a"}`,
		`{"tag":"col","x":10,"y":0,"w":10,"h":1,"key":"b"}`,
		`{"tag":"col","x":0,"y":2,"w":9,"h":2,"key":"c"}`,
		`{"tag":"col","x":10,"y":2,"w":10,"h":2,"key":"d"}`,
	})
	d, err = app.Dump(30, 5)
	if err != nil {
		t.Fatal(err)
	}
	sameLines(t, "grid at 30", d.Grid, []string{
		"alpha     beta      gamma     ",
		"                              ",
		"delta                         ",
		"+1                            ",
		"                              ",
	})
	if !d.OK || len(d.Errors) != 0 {
		t.Errorf("errors %v", d.Errors)
	}
}
