package kit

import (
	"fmt"
	"testing"
)

func bigRows(n int) []any {
	rows := make([]any, n)
	for i := range rows {
		rows[i] = map[string]any{
			"id": fmt.Sprintf("r%05d", i), "name": fmt.Sprintf("service-%05d", (i*7919)%n),
			"rps": float64((i * 31) % 997), "state": []string{"running", "stopped", "degraded"}[i%3],
		}
	}
	return rows
}

func benchSortPage(b *testing.B, sorted bool) {
	app := bareApp(b)
	_ = app.Bind("", map[string]any{"all": bigRows(10000), "other": 0.0})
	sp, err := NewSortPage(app, SortPageOptions{
		Source: "all", Out: "rows", Meta: "meta", Size: 50,
		Columns: []Column{{"name", "NAME"}, {"rps", "RPS"}},
	})
	if err != nil {
		b.Fatal(err)
	}
	if sorted {
		_ = sp.Cycle("name")
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = app.Set("other", float64(i)) // an unrelated write re-derives everything
	}
}

// An unrelated store write with 10k rows: what every arrow key in a table costs.
func BenchmarkSortPageUnrelatedWrite(b *testing.B)       { benchSortPage(b, false) }
func BenchmarkSortPageUnrelatedWriteSorted(b *testing.B) { benchSortPage(b, true) }

func BenchmarkSortRows10k(b *testing.B) {
	rows := bigRows(10000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		SortRows(rows, "name", false)
	}
}
