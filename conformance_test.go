package tuimark_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/abdul-hamid-achik/tuimark"
	"github.com/abdul-hamid-achik/tuimark/internal/dump"
)

// SPEC §21 conformance tests. Spike goldens are frozen: never regenerate
// them to make a later phase pass — add a new fixture instead.

const spike = "examples/spike/inbox.tui"

func load(t *testing.T, path string) *tuimark.App {
	t.Helper()
	app, err := tuimark.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return app
}

func dumpOf(t *testing.T, app *tuimark.App, cols, rows int) *tuimark.Dump {
	t.Helper()
	d, err := app.Dump(cols, rows)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func node(t *testing.T, d *tuimark.Dump, id string) tuimark.DumpNode {
	t.Helper()
	for _, n := range d.Nodes {
		if n.ID == id {
			return n
		}
	}
	t.Fatalf("node %q not in dump", id)
	return tuimark.DumpNode{}
}

func geom(n tuimark.DumpNode) string {
	return fmt.Sprintf("%dx%d@(%d,%d)", n.W, n.H, n.X, n.Y)
}

// 1. Parse examples/spike/inbox.tui → no diagnostics.
func TestSpikeParsesClean(t *testing.T) {
	app := load(t, spike)
	if diags := app.Validate(); len(diags) != 0 {
		t.Fatalf("want no diagnostics, got %v", diags)
	}
}

// 2 + 3. Required geometry at 80, 120, and 40 columns (SPEC §16.2).
func TestSpikeGeometry(t *testing.T) {
	cases := []struct {
		cols                  int
		inbox, detail, status string
	}{
		{80, "24x23@(0,0)", "56x23@(24,0)", "80x1@(0,23)"},
		{120, "36x23@(0,0)", "84x23@(36,0)", "120x1@(0,23)"},
		{40, "12x23@(0,0)", "28x23@(12,0)", "40x1@(0,23)"},
	}
	app := load(t, spike)
	for _, c := range cases {
		t.Run(fmt.Sprint(c.cols), func(t *testing.T) {
			d := dumpOf(t, app, c.cols, 24)
			if !d.OK {
				t.Fatalf("dump not ok: %v", d.Errors)
			}
			for id, want := range map[string]string{"inbox": c.inbox, "detail": c.detail, "status": c.status} {
				if got := geom(node(t, d, id)); got != want {
					t.Errorf("%s at %d cols: got %s, want %s", id, c.cols, got, want)
				}
			}
		})
	}
}

// §16.2: go test MUST assert the three node lines of the 80×24 text dump.
func TestSpikeTextNodeLines(t *testing.T) {
	d := dumpOf(t, load(t, spike), 80, 24)
	text := dump.Text(d)
	for _, want := range [][]string{
		{"inbox", "box", "24x23", "@(0,0)"},
		{"detail", "box", "56x23", "@(24,0)"},
		{"status", "box", "80x1", "@(0,23)"},
	} {
		found := false
		for _, line := range strings.Split(text, "\n") {
			if strings.Join(strings.Fields(line), " ") == strings.Join(want, " ") {
				found = true
			}
		}
		if !found {
			t.Errorf("text dump lacks node line %q:\n%s", strings.Join(want, " "), text)
		}
	}
}

// 4. grid[y] length equals cols for every row; no ANSI.
func TestGridShape(t *testing.T) {
	app := load(t, spike)
	for _, size := range [][2]int{{40, 24}, {80, 24}, {120, 24}, {7, 3}, {1, 1}, {200, 60}} {
		d := dumpOf(t, app, size[0], size[1])
		if len(d.Grid) != size[1] {
			t.Fatalf("%v: %d grid rows", size, len(d.Grid))
		}
		for y, row := range d.Grid {
			if n := utf8.RuneCountInString(row); n != size[0] {
				t.Errorf("%v: row %d has %d runes", size, y, n)
			}
			if strings.ContainsRune(row, 0x1b) {
				t.Errorf("%v: row %d contains ESC", size, y)
			}
		}
	}
}

// Frozen goldens (SPEC §16): text at 40/80/120 and JSON at 80.
func TestSpikeGoldens(t *testing.T) {
	app := load(t, spike)
	for _, cols := range []int{40, 80, 120} {
		want, err := os.ReadFile(filepath.Join("testdata/golden/spike", fmt.Sprintf("%dx24.txt", cols)))
		if err != nil {
			t.Fatal(err)
		}
		if got := dump.Text(dumpOf(t, app, cols, 24)); got != string(want) {
			t.Errorf("%dx24 text golden mismatch:\n--- got\n%s\n--- want\n%s", cols, got, want)
		}
	}
	want, err := os.ReadFile("testdata/golden/spike/80x24.json")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := dump.JSON(dumpOf(t, app, 80, 24))
	if !bytes.Equal(got, want) {
		t.Errorf("80x24 JSON golden mismatch:\n%s", got)
	}
	// §16.3: the first two rows.
	d := dumpOf(t, app, 80, 24)
	if d.Grid[0] != "┌"+strings.Repeat("─", 22)+"┐┌"+strings.Repeat("─", 54)+"┐" {
		t.Errorf("row 0: %q", d.Grid[0])
	}
	if !strings.HasPrefix(d.Grid[1], "│Inbox") || !strings.Contains(d.Grid[1], "││Detail") {
		t.Errorf("row 1: %q", d.Grid[1])
	}
}

func codes(diags []tuimark.Diagnostic) []string {
	var out []string
	for _, d := range diags {
		out = append(out, d.Code)
	}
	return out
}

func hasCode(diags []tuimark.Diagnostic, code string) bool {
	for _, d := range diags {
		if d.Code == code {
			return true
		}
	}
	return false
}

func parseDoc(t *testing.T, src string) *tuimark.App {
	t.Helper()
	app, err := tuimark.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	return app
}

// 5–9. Error codes (exit code 2 is covered in cmd/tuimark tests).
func TestDiagnosticCodes(t *testing.T) {
	cases := []struct {
		name, code, src string
	}{
		{"unknown tag", "V001", `<app><widget/></app>`},
		{"duplicate id", "V004", `<app><box id="a"/><box id="a"/></app>`},
		{"bad unit", "V003", `<app><box width="foo"/></app>`},
		{"unclosed", "V005", `<app><box></app>`},
		{"text with element", "V013", `<app><text><box/></text></app>`},
		{"unknown attr", "V002", `<app><box title="x"/></app>`},
		{"px unit", "V003", `<app><box width="10px"/></app>`},
		{"single quotes", "V005", `<app><box id='a'/></app>`},
		{"doctype", "V005", `<!DOCTYPE app><app/>`},
		{"entity", "V005", `<app><text>&nbsp;</text></app>`},
		{"uppercase", "V001", `<app><Box/></app>`},
		{"missing version", "V014", `<tui><screen/></tui>`},
		{"ansi", "V007", "<app><text>\x1b[31mred</text></app>"},
		{"no id on input", "V012", `<tui version="1"><screen><input/></screen></tui>`},
		{"each bad", "V011", `<tui version="1"><screen><list id="l" each="tickets"><item/></list></screen></tui>`},
		{"modal not last", "L004", `<tui version="1"><screen><modal id="m" open="true"/><box/></screen></tui>`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			diags := parseDoc(t, c.src).Validate()
			if !hasCode(diags, c.code) {
				t.Fatalf("want %s, got %v", c.code, diags)
			}
			d, _ := parseDoc(t, c.src).Dump(80, 24)
			if d.OK {
				t.Fatalf("dump should not be ok")
			}
		})
	}
}

// The JSON dump shape matches SPEC §13.2.
func TestDumpJSONShape(t *testing.T) {
	d := dumpOf(t, load(t, spike), 80, 24)
	b, _ := json.Marshal(d)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"cols", "rows", "ok", "focus", "errors", "nodes", "grid"} {
		if _, ok := m[k]; !ok {
			t.Errorf("dump JSON lacks %q", k)
		}
	}
	if m["focus"] != nil {
		t.Errorf("spike focus should be null")
	}
}

// Finding 27: the public Dump rejects negative sizes instead of returning a
// dump whose cols/rows contradict its grid; 0 is an empty, consistent grid.
func TestDumpSizeContract(t *testing.T) {
	app := parseDoc(t, `<tui version="1"><screen id="s"><text>hi</text></screen></tui>`)
	for _, sz := range [][2]int{{-5, 3}, {5, -3}, {-1, -1}} {
		if d, err := app.Dump(sz[0], sz[1]); err == nil {
			t.Errorf("Dump(%d, %d) = cols %d rows %d grid %d, want an error", sz[0], sz[1], d.Cols, d.Rows, len(d.Grid))
		}
	}
	for _, sz := range [][2]int{{0, 2}, {3, 0}, {0, 0}, {5, 3}} {
		d := dumpOf(t, app, sz[0], sz[1])
		if d.Cols != sz[0] || d.Rows != sz[1] || len(d.Grid) != d.Rows {
			t.Errorf("Dump(%d, %d): cols %d rows %d grid %d", sz[0], sz[1], d.Cols, d.Rows, len(d.Grid))
		}
		for _, line := range d.Grid {
			if utf8.RuneCountInString(line) != d.Cols {
				t.Errorf("Dump(%d, %d): %q", sz[0], sz[1], line)
			}
		}
	}
}

// Findings 16/31: a huge size is clipped in paint (SPEC §11.4), so dump and
// validate cost what the visible grid costs and still finish promptly.
func TestHugeSizesDumpPromptly(t *testing.T) {
	docs := []string{
		`<app><box width="99999999999" height="3" border="1"/></app>`,
		`<app><box id="a" width="10" height="99999999999"/></app>`,
		`<app><row><box id="a" width="1000000000"/></row></app>`,
		`<tui version="1"><style>#a { min-width: 999999999999999; background: red; }</style><screen><row><box id="a"/><box/></row></screen></tui>`,
	}
	done := make(chan string, 1)
	go func() {
		for _, src := range docs {
			app, err := tuimark.Parse(strings.NewReader(src))
			if err != nil {
				done <- err.Error()
				return
			}
			d, err := app.Dump(20, 3)
			if err != nil || len(d.Grid) != 3 || utf8.RuneCountInString(d.Grid[0]) != 20 {
				done <- fmt.Sprintf("%s: %v %+v", src, err, d)
				return
			}
			app.Validate()
		}
		done <- ""
	}()
	select {
	case msg := <-done:
		if msg != "" {
			t.Error(msg)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("dump/validate of a huge box did not finish: paint walks the logical rect")
	}
}
