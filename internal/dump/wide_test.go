package dump_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/rivo/uniseg"

	"github.com/abdul-hamid-achik/tuimark/internal/dump"
	"github.com/abdul-hamid-achik/tuimark/internal/host"
	"github.com/abdul-hamid-achik/tuimark/internal/uniwidth"
)

func dumpOf(t *testing.T, doc string, cols, rows int, cells bool) *dump.Dump {
	t.Helper()
	a, err := host.Parse(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	return a.Dump(cols, rows, cells)
}

func jsonOf(t *testing.T, d *dump.Dump) string {
	t.Helper()
	b, err := dump.JSON(d)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// §21 test 18: the §13.2 example, byte for byte.
func TestWideExample(t *testing.T) {
	d := dumpOf(t, `<app><text id="name">微信ok</text></app>`, 6, 1, true)
	want := `{
  "cols": 6,
  "rows": 1,
  "ok": true,
  "focus": null,
  "errors": [],
  "nodes": [
    {
      "tag": "app",
      "x": 0,
      "y": 0,
      "w": 6,
      "h": 1
    },
    {
      "id": "name",
      "tag": "text",
      "x": 0,
      "y": 0,
      "w": 6,
      "h": 1,
      "text": "微信ok"
    }
  ],
  "grid": [
    "微信ok"
  ],
  "wide": true,
  "cells": [
    {
      "x": 0,
      "y": 0,
      "ch": "微",
      "id": "name",
      "w": 2
    },
    {
      "x": 1,
      "y": 0,
      "ch": "",
      "id": "name"
    },
    {
      "x": 2,
      "y": 0,
      "ch": "信",
      "id": "name",
      "w": 2
    },
    {
      "x": 3,
      "y": 0,
      "ch": "",
      "id": "name"
    },
    {
      "x": 4,
      "y": 0,
      "ch": "o",
      "id": "name"
    },
    {
      "x": 5,
      "y": 0,
      "ch": "k",
      "id": "name"
    }
  ]
}
`
	if got := jsonOf(t, d); got != want {
		t.Errorf("dump:\n%s\nwant:\n%s", got, want)
	}
	// Without --cells, "wide" still says the grid is not one rune per column.
	if got := jsonOf(t, dumpOf(t, `<app><text id="name">微信ok</text></app>`, 6, 1, false)); !strings.Contains(got, `"wide": true`) || strings.Contains(got, `"cells"`) {
		t.Errorf("dump without cells:\n%s", got)
	}
}

const mixed = `<tui version="1"><screen id="s"><col>
<box id="panel" border="single" title="微信 title"><text id="t">한국어 텍스트</text></box>
<text id="fam">👨\u200D👩\u200D👧 family e` + "\u0301" + ` 🇺🇸</text>
<input id="pw" secret="true" bind="pw"/>
<button id="ok">确定</button>
</col></screen></tui>`

// MUST 6 and §13.2: every grid row is exactly cols columns wide, cells has
// one entry per column, continuation cells have ch "" and their lead's id,
// and "wide" is set.
func TestWideGridColumnsAndCells(t *testing.T) {
	for _, cols := range []int{7, 12, 13, 20, 41} {
		d := dumpOf(t, mixed, cols, 8, true)
		if !d.Wide {
			t.Errorf("%d cols: wide is not set", cols)
		}
		if len(d.Grid) != 8 || len(d.Cells) != cols*8 {
			t.Fatalf("%d cols: %d rows, %d cells", cols, len(d.Grid), len(d.Cells))
		}
		for y, row := range d.Grid {
			var b strings.Builder
			w := 0
			for x := 0; x < cols; x++ {
				c := d.Cells[y*cols+x]
				if c.X != x || c.Y != y {
					t.Fatalf("cell %d is (%d,%d)", y*cols+x, c.X, c.Y)
				}
				if c.Ch == "" {
					lead := d.Cells[y*cols+x-1]
					if lead.W != 2 || c.ID != lead.ID || c.W != 0 {
						t.Fatalf("%d cols (%d,%d): continuation %+v after %+v", cols, x, y, c, lead)
					}
					continue
				}
				if cw := uniwidth.ClusterWidth(c.Ch); (c.W == 2) != (cw == 2) || cw == 0 || uniwidth.Count(c.Ch) != 1 {
					t.Fatalf("%d cols (%d,%d): cell %+v, cluster width %d", cols, x, y, c, cw)
				}
				w += uniwidth.ClusterWidth(c.Ch)
				b.WriteString(c.Ch)
			}
			if w != cols || row != b.String() {
				t.Fatalf("%d cols: row %d %q is %d columns; cells give %q", cols, y, row, w, b.String())
			}
			for _, r := range row {
				if r < 0x20 || (r >= 0x7f && r <= 0x9f) {
					t.Fatalf("control character in grid row %q", row)
				}
			}
		}
	}
}

// A secret input dumps one • per cluster of its value (§12.1, §21 test 21).
func TestSecretDumpsOneBulletPerCluster(t *testing.T) {
	a, err := host.Parse(strings.NewReader(mixed))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Set("pw", "a👍🏽e"+"\u0301"+"微"); err != nil {
		t.Fatal(err)
	}
	d := a.Dump(20, 8, false)
	for _, n := range d.Nodes {
		if n.ID == "pw" && n.Text != "••••" {
			t.Errorf("secret text = %q, want one bullet per cluster", n.Text)
		}
	}
	if !strings.Contains(strings.Join(d.Grid, "\n"), "••••") {
		t.Errorf("grid:\n%s", strings.Join(d.Grid, "\n"))
	}
}

// §21 test 18: an ASCII (and box-drawing) document has no "wide"; nor does
// one whose non-ASCII glyphs are simple clusters of width 1.
func TestNoWideForSimpleClusters(t *testing.T) {
	for _, doc := range []string{
		`<app><box border="1"><text id="t">plain ascii</text></box></app>`,
		`<app><text id="t">ñandú — café • … ±</text></app>`,
	} {
		d := dumpOf(t, doc, 30, 4, true)
		if d.Wide || strings.Contains(jsonOf(t, d), `"wide"`) {
			t.Errorf("%s: wide set", doc)
		}
		for _, c := range d.Cells {
			if c.W != 0 || c.Ch == "" {
				t.Errorf("%s: cell %+v", doc, c)
			}
		}
		for y, row := range d.Grid {
			if n := len([]rune(row)); n != 30 {
				t.Errorf("row %d has %d runes", y, n)
			}
		}
	}
}

// §21 test 17: dumps do not change when uniseg's process-wide ambiguous
// width is 2.
func TestDumpIgnoresUnisegAmbiguousWidth(t *testing.T) {
	doc := `<tui version="1"><screen id="s"><col><box border="rounded" title="… ± §"><text>• ─ … ° ± § ¶ ① α</text></box>` +
		`<text>微信ok ❤ ❤` + "\uFE0F" + `</text></col></screen></tui>`
	before := jsonOf(t, dumpOf(t, doc, 20, 5, true))
	saved := uniseg.EastAsianAmbiguousWidth
	uniseg.EastAsianAmbiguousWidth = 2
	defer func() { uniseg.EastAsianAmbiguousWidth = saved }()
	if after := jsonOf(t, dumpOf(t, doc, 20, 5, true)); after != before {
		t.Errorf("dump depends on uniseg.EastAsianAmbiguousWidth:\n%s\n---\n%s", before, after)
	}
	var d dump.Dump
	if err := json.Unmarshal([]byte(before), &d); err != nil || len(d.Cells) != 100 {
		t.Errorf("round trip: %v, %d cells", err, len(d.Cells))
	}
}
