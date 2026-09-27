package tuimark_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
	"github.com/abdul-hamid-achik/tuimark/internal/dump"
)

// SPEC §21 phase 1+ (tests 10-11) and phase 2+ (tests 12-14).
//
// The stacked goldens can be regenerated with
//
//	go test -run TestStackedGoldens -update-stacked
//
// The spike goldens are frozen and have no update flag (SPEC §1 MUST NOT 8).

var updateStacked = flag.Bool("update-stacked", false, "rewrite testdata/golden/stacked/*.txt")

const (
	phStacked = "examples/stacked/app.tui"
	phSpike   = "examples/spike/inbox.tui"
	phInbox   = "examples/inbox/app.tui"
	phSample  = "examples/inbox/sample.json"
)

func phLoad(t *testing.T, path string) *tuimark.App {
	t.Helper()
	app, err := tuimark.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return app
}

// phInboxApp loads the SPEC §17 fixture and binds sample.json as the store.
func phInboxApp(t *testing.T) *tuimark.App {
	t.Helper()
	app := phLoad(t, phInbox)
	raw, err := os.ReadFile(phSample)
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	if err := app.Bind("", data); err != nil {
		t.Fatal(err)
	}
	return app
}

func phDump(t *testing.T, app *tuimark.App, cols, rows int) *tuimark.Dump {
	t.Helper()
	d, err := app.Dump(cols, rows)
	if err != nil {
		t.Fatal(err)
	}
	if !d.OK {
		t.Fatalf("dump %dx%d not ok: %v", cols, rows, d.Errors)
	}
	return d
}

func phNode(t *testing.T, d *tuimark.Dump, id string) tuimark.DumpNode {
	t.Helper()
	for _, n := range d.Nodes {
		if n.ID == id {
			return n
		}
	}
	t.Fatalf("no node #%s in the %dx%d dump", id, d.Cols, d.Rows)
	return tuimark.DumpNode{}
}

func phGeom(n tuimark.DumpNode) string { return fmt.Sprintf("%dx%d@(%d,%d)", n.W, n.H, n.X, n.Y) }

func TestStackedValidates(t *testing.T) {
	if diags := phLoad(t, phStacked).Validate(); len(diags) != 0 {
		t.Fatalf("stacked fixture: %v", diags)
	}
}

// 10. @media (max-cols: 80) stacks #body in the new 40-col fixture: the
// sidebar sits above the detail at the same x.
func TestStackedMediaStacksBody(t *testing.T) {
	app := phLoad(t, phStacked)
	for _, cols := range []int{40, 80} {
		d := phDump(t, app, cols, 24)
		side, det := phNode(t, d, "sidebar"), phNode(t, d, "detail")
		if side.X != det.X || det.Y <= side.Y || det.Y < side.Y+side.H {
			t.Errorf("%d cols: sidebar %s and detail %s are not stacked", cols, phGeom(side), phGeom(det))
		}
		if side.W != cols || det.W != cols {
			t.Errorf("%d cols: stacked panes span the width: sidebar %s detail %s", cols, phGeom(side), phGeom(det))
		}
	}
	d := phDump(t, app, 40, 24)
	for id, want := range map[string]string{
		"body":    "40x22@(0,1)",
		"sidebar": "40x5@(0,1)",
		"detail":  "40x17@(0,6)",
		"status":  "40x1@(0,23)",
	} {
		if got := phGeom(phNode(t, d, id)); got != want {
			t.Errorf("40 cols: #%s = %s, want %s", id, got, want)
		}
	}
	// Above the breakpoint the row lays out side by side.
	for _, cols := range []int{81, 120} {
		d := phDump(t, app, cols, 24)
		side, det := phNode(t, d, "sidebar"), phNode(t, d, "detail")
		if side.Y != det.Y || det.X != side.X+side.W || side.W != 32 {
			t.Errorf("%d cols: sidebar %s detail %s should be side by side", cols, phGeom(side), phGeom(det))
		}
	}
}

func TestStackedGoldens(t *testing.T) {
	app := phLoad(t, phStacked)
	for _, cols := range []int{40, 80, 120} {
		path := filepath.Join("testdata/golden/stacked", fmt.Sprintf("%dx24.txt", cols))
		got := dump.Text(phDump(t, app, cols, 24))
		if *updateStacked {
			if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if got != string(want) {
			t.Errorf("%s mismatch:\n--- got\n%s\n--- want\n%s", path, got, want)
		}
	}
}

// 11. The spike 80×24 golden still matches now that CSS exists: the
// cascade, UA sheet, and media pass run for spike documents too.
func TestSpikeGoldenAfterCSS(t *testing.T) {
	app := phLoad(t, phSpike)
	d := phDump(t, app, 80, 24)
	want, err := os.ReadFile("testdata/golden/spike/80x24.txt")
	if err != nil {
		t.Fatal(err)
	}
	if got := dump.Text(d); got != string(want) {
		t.Errorf("spike 80x24 text golden mismatch:\n%s", got)
	}
	wantJSON, err := os.ReadFile("testdata/golden/spike/80x24.json")
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := dump.JSON(d); !bytes.Equal(got, wantJSON) {
		t.Errorf("spike 80x24 JSON golden mismatch:\n%s", got)
	}
}

// 12. {folder} interpolates from sample.json.
func TestFolderInterpolates(t *testing.T) {
	d := phDump(t, phInboxApp(t), 80, 24)
	if !strings.Contains(strings.Join(d.Grid, "\n"), "mail  deploy") {
		t.Fatalf("grid lacks \"mail  deploy\":\n%s", strings.Join(d.Grid, "\n"))
	}
	if !strings.HasPrefix(d.Grid[0], "mail  deploy ") {
		t.Errorf("header row = %q", d.Grid[0])
	}
	for _, n := range d.Nodes {
		if strings.Contains(n.Text, "{") {
			t.Errorf("uninterpolated text %q", n.Text)
		}
	}
}

// 13. if="!selected_ticket": with selected_ticket null the title is gone and
// "no selection" shows.
func TestNullSelectionShowsPlaceholder(t *testing.T) {
	app := phInboxApp(t)
	d := phDump(t, app, 120, 24)
	grid := strings.Join(d.Grid, "\n")
	if !strings.Contains(grid, "login loop on staging") || strings.Contains(grid, "no selection") {
		t.Fatalf("before: selected ticket expected:\n%s", grid)
	}
	if err := app.Set("selected_ticket", nil); err != nil {
		t.Fatal(err)
	}
	d = phDump(t, app, 120, 24)
	det := phNode(t, d, "detail")
	var inDetail []string
	for y := det.Y; y < det.Y+det.H; y++ {
		inDetail = append(inDetail, string([]rune(d.Grid[y])[det.X:det.X+det.W]))
	}
	text := strings.Join(inDetail, "\n")
	if strings.Contains(text, "login loop on staging") {
		t.Errorf("the title is still in the detail pane:\n%s", text)
	}
	if !strings.Contains(text, "no selection") {
		t.Errorf("\"no selection\" is not in the detail pane:\n%s", text)
	}
	for _, n := range d.Nodes {
		if n.Text == "login loop on staging" && n.X >= det.X {
			t.Errorf("title node still laid out: %+v", n)
		}
	}
}

// 14. each="tickets as item" produces exactly three list rows.
func TestEachProducesThreeItems(t *testing.T) {
	d := phDump(t, phInboxApp(t), 80, 24)
	list := phNode(t, d, "inbox")
	var items []tuimark.DumpNode
	for _, n := range d.Nodes {
		if n.Tag == "item" {
			items = append(items, n)
		}
	}
	if len(items) != 3 {
		t.Fatalf("want 3 item nodes, got %d", len(items))
	}
	for i, want := range []string{"t-12", "t-18", "t-21"} {
		it := items[i]
		if it.Key != want {
			t.Errorf("item %d key = %q, want %q", i, it.Key, want)
		}
		if it.X != list.X || it.W != list.W || it.H != 1 || it.Y != list.Y+i {
			t.Errorf("item %d = %s inside list %s", i, phGeom(it), phGeom(list))
		}
	}
}
