package tuimark_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

// SPEC v0.4c §24 conformance tests of props, phase C1 (tests 236, 240,
// 248, and 261), the parts the Go API observes: a copy built with its props
// gives the frame of the same markup written by hand, and a prop's growth
// is charged to the expansion. The CLI parts (validate's lists, the dumps'
// texts, ir, and the catalog) are cmd/tuimark's
// conformance_v04c_props_test.go. The fixtures are testdata/v04c/<test>.

// sameFrame checks that two documents render the same frame: the same
// nodes, grid, and focus (their diagnostics may differ).
func sameFrame(t *testing.T, what string, a, b *tuimark.App, cols, rows int) {
	t.Helper()
	da, err := a.Dump(cols, rows)
	if err != nil {
		t.Fatal(err)
	}
	db, err := b.Dump(cols, rows)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(da.Nodes, db.Nodes) || !reflect.DeepEqual(da.Grid, db.Grid) || !reflect.DeepEqual(da.Focus, db.Focus) {
		t.Errorf("%s at %dx%d: the frames differ\n%v\n%s\nvs\n%v\n%s", what, cols, rows, da.Nodes, strings.Join(da.Grid, "\n"), db.Nodes, strings.Join(db.Grid, "\n"))
	}
}

// 236, 240, 261. Each copy is the markup its props make, written by hand.
func TestPropsCopiesEqualTheHandWrittenMarkup(t *testing.T) {
	for _, c := range []struct {
		test       string
		data       bool
		cols, rows int
	}{
		{"236", true, 30, 12},
		{"240", true, 20, 6},
		{"261", false, 20, 4},
	} {
		t.Run(c.test, func(t *testing.T) {
			dir := filepath.Join("testdata", "v04c", c.test)
			app, hand := filepath.Join(dir, "app.tui"), filepath.Join(dir, "hand.tui")
			if c.data {
				data := filepath.Join(dir, "d.json")
				sameFrame(t, c.test, loadWith(t, app, data), loadWith(t, hand, data), c.cols, c.rows)
				return
			}
			sameFrame(t, c.test, mustLoad(t, app), mustLoad(t, hand), c.cols, c.rows)
		})
	}
}

// 248, the prop growth: the bytes a substitution adds are charged to the
// copy, so two texts of an 8 MiB prop pass 16 MiB of placed source, and
// one does not.
func TestPropGrowthIsCharged(t *testing.T) {
	v := strings.Repeat("a", 8388608)
	write := func(texts string) string {
		p := filepath.Join(t.TempDir(), "big.tui")
		doc := "<tui>\n  <template id=\"t\" props=\"p\">" + texts + "</template>\n  <screen id=\"main\"><use template=\"t\" p=\"" + v + "\"/></screen>\n</tui>\n"
		if err := os.WriteFile(p, []byte(doc), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	app := mustLoad(t, write("<text>{$p}</text><text>{$p}</text>"))
	want := []string{`V021 big.tui:3:21 /screen/use id= use template="t" exceeds the expansion limit (100000 elements or 16 MiB of placed source): it inserts nothing, and no later include or use expands`}
	if got := diagTexts(app.Validate()); !reflect.DeepEqual(got, want) {
		t.Errorf("two texts: %v", got)
	}
	screenIsEmpty(t, app)
	one := mustLoad(t, write("<text>{$p}</text>"))
	for _, d := range one.Validate() {
		if d.Severity == "error" {
			t.Errorf("one text: %s", diagText(d))
		}
	}
}
