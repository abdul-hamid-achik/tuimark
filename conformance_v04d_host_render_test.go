package tuimark_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

// Appending in place must be seen by every cache that reads an array: a
// list, a table, and a container with each, rendered after every append,
// through Play's frames and through Dump. The array starts with spare
// capacity, so the first appends go into the store's own backing array.
// (SPEC v0.4d §18.3: "the store appends in place".)
func TestD5AppendRendersEveryRow(t *testing.T) {
	const src = `<tui>
  <screen id="a">
    <list id="l" each="rows as r" key="r.id" height="12"><item><text>l-{r.id}</text></item></list>
    <table id="t" each="rows as r" key="r.id" height="12"><column title="N">t-{r.id}</column></table>
    <col id="c" each="rows as r"><text>c-{r.id}</text></col>
  </screen>
</tui>`
	const rowsJSON = `{"rows": [{"id": 1}, {"id": 2}, {"id": 3}]}`
	check := func(t *testing.T, grid []string, n int, where string) {
		t.Helper()
		g := strings.Join(grid, "\n")
		for i := 1; i <= n; i++ {
			for _, p := range []string{"l-", "t-", "c-"} {
				if !strings.Contains(g, fmt.Sprintf("%s%d", p, i)) {
					t.Errorf("%s: %s%d missing with %d rows:\n%s", where, p, i, n, g)
				}
			}
		}
		if strings.Contains(g, fmt.Sprintf("l-%d", n+1)) {
			t.Errorf("%s: a row beyond %d is shown:\n%s", where, n, g)
		}
	}

	t.Run("Play frames", func(t *testing.T) {
		app := parseDoc(t, src)
		bindJSON(t, app, "", rowsJSON)
		var steps []string
		for i := 4; i <= 9; i++ {
			steps = append(steps, fmt.Sprintf(`append:rows={"id":%d}`, i))
		}
		res, err := app.Play(tuimark.PlayOptions{NoHandlers: true, Frames: true, Cols: 40, Rows: 40}, steps...)
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Frames) != len(steps)+1 {
			t.Fatalf("%d frames, want %d", len(res.Frames), len(steps)+1)
		}
		for i, f := range res.Frames {
			check(t, f.Dump.Grid, 3+i, fmt.Sprintf("frame %d", i))
		}
	})

	t.Run("Append and Dump", func(t *testing.T) {
		app := parseDoc(t, src)
		bindJSON(t, app, "", rowsJSON)
		for n := 3; n <= 9; n++ {
			d, err := app.Dump(40, 40)
			if err != nil {
				t.Fatal(err)
			}
			check(t, d.Grid, n, fmt.Sprintf("dump with %d rows", n))
			if err := app.Append("rows", map[string]any{"id": float64(n + 1)}); err != nil {
				t.Fatal(err)
			}
		}
	})

	t.Run("a rolled-back batch leaves the frames alone", func(t *testing.T) {
		app := parseDoc(t, src)
		bindJSON(t, app, "", rowsJSON)
		if _, err := app.Dump(40, 40); err != nil {
			t.Fatal(err)
		}
		err := app.Batch(func(b *tuimark.Batch) error {
			_ = b.Append("rows", map[string]any{"id": 4.0})
			_ = b.Append("rows", map[string]any{"id": 5.0})
			return b.Set("@theme", "nope")
		})
		if err == nil {
			t.Fatal("expected the batch to fail")
		}
		d, err := app.Dump(40, 40)
		if err != nil {
			t.Fatal(err)
		}
		check(t, d.Grid, 3, "after the rolled-back batch")
		if err := app.Append("rows", map[string]any{"id": 4.0}); err != nil {
			t.Fatal(err)
		}
		d, _ = app.Dump(40, 40)
		check(t, d.Grid, 4, "after an append that follows it")
	})
}
