package play

import (
	"fmt"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/host"
)

// The performance benchmarks of SPEC v0.4a §21 test 125 (§31.2 item 20):
// on a settled session at 80×24, one input per iteration, each must take
// under 16 ms per operation. They are benchmarks, not timed assertions:
// `task bench` runs them and prints the numbers. An iteration is exactly
// what a play step costs after step 0: the input (handleKeys, or the
// Batch an Every tick applies) and the settle that follows it.

// benchRows is the size of the each array of every benchmark.
const benchRows = 20000

// benchList is case (a): a list of benchRows rows with each and key, the
// cursor bound, focused from the start.
const benchList = `<tui version="3"><screen id="s" focus="#l">
<list id="l" each="rows as r" key="r.id" bind="cur"><item><text>{r.name} {r.n}</text></item></list>
</screen></tui>`

// benchTable is case (b): a table of benchRows rows and four columns.
const benchTable = `<tui version="3"><screen id="s" focus="#t">
<table id="t" each="rows as r" key="r.id" bind="cur">
<column title="ID" width="8">{r.id}</column><column title="Name" width="1fr">{r.name}</column>
<column title="N" width="6">{r.n}</column><column title="Tag">{r.tag}</column></table>
</screen></tui>`

// benchTick is case (d): the list of case (a) under a status line that a
// periodic one-path Batch (what an Every tick applies) rewrites.
const benchTick = `<tui version="3"><screen id="s" focus="#l">
<text id="clock" style="dock: top">tick {tick}</text>
<list id="l" each="rows as r" key="r.id" bind="cur"><item><text>{r.name} {r.n}</text></item></list>
</screen></tui>`

func benchData(n int) map[string]any {
	rows := make([]any, n)
	for i := range rows {
		rows[i] = map[string]any{"id": fmt.Sprintf("r%05d", i), "name": fmt.Sprintf("row number %d", i), "n": float64(i), "tag": "x"}
	}
	return map[string]any{"rows": rows, "cur": "r00000", "tick": float64(0)}
}

// benchSession loads src over benchRows rows and settles it at 80×24
// (step 0 of a play session).
func benchSession(b *testing.B, src string) *Session {
	b.Helper()
	a, err := host.Parse(strings.NewReader(src))
	if err != nil {
		b.Fatal(err)
	}
	if err := a.Bind("", benchData(benchRows)); err != nil {
		b.Fatal(err)
	}
	s := NewSession(a, 80, 24, false, false, false, true)
	s.Run(nil)
	return s
}

// benchDown replays down once per iteration, as a play step does: the
// key (with the redraw its cursor move asks for) and the settle. Before
// the cursor reaches the last row it is put back on the first, outside
// the timer, so every timed down moves it (and, past the first screen,
// scrolls).
func benchDown(b *testing.B, src string) {
	s := benchSession(b, src)
	down := KeyFromToken("down")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i > 0 && i%(benchRows-1) == 0 {
			b.StopTimer()
			if err := s.App.Set("cur", "r00000"); err != nil {
				b.Fatal(err)
			}
			s.settle(i)
			b.StartTimer()
		}
		s.handleKeys(i+1, []host.Key{down})
		s.settle(i + 1)
	}
}

// BenchmarkListDown20k is case (a): down in a list of 20 000 rows.
func BenchmarkListDown20k(b *testing.B) { benchDown(b, benchList) }

// BenchmarkTableDown20k is case (b): down in a table of 20 000 rows and
// four columns.
func BenchmarkTableDown20k(b *testing.B) { benchDown(b, benchTable) }

// BenchmarkDeriveFilterText20k is case (c): a text: keystroke into an
// input whose bind a Derive filters over 20 000 rows.
//
// TODO(0.4a): Derive is being added to the runtime loop in parallel; write
// this case on top of it (an input bound to "q", a Derive of
// "visible" = rows filtered by q, a list over visible, one text: step per
// iteration) once the API lands.
func BenchmarkDeriveFilterText20k(b *testing.B) {
	b.Skip("TODO(0.4a): needs the Derive API (SPEC v0.4a §21 test 125, case c)")
}

// BenchmarkBatchTick20k is case (d): a one-path Batch, as an Every tick
// applies it, on a store holding 20 000 rows, and the settle that draws
// it.
func BenchmarkBatchTick20k(b *testing.B) {
	s := benchSession(b, benchTick)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tick := float64(i + 1)
		if err := s.App.Batch(func(bt *host.Batch) error { return bt.Set("tick", tick) }); err != nil {
			b.Fatal(err)
		}
		s.settle(i + 1)
	}
}

// BenchmarkListRowSet20k is not one of the cases: a Set of one row's name
// under the list of case (a), and the settle that draws it. The list's
// rows read that member of the store, so their cache is filled again on
// every iteration: the cost of a frame that misses it (what a Derive
// rewriting the list's array on every keystroke pays, case c).
func BenchmarkListRowSet20k(b *testing.B) {
	s := benchSession(b, benchList)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := s.App.Set("rows.5.name", fmt.Sprintf("renamed %d", i)); err != nil {
			b.Fatal(err)
		}
		s.settle(i + 1)
	}
}

// BenchmarkBatchOnly20k is the Batch of case (d) alone, without the
// redraw: the cost of the atomic apply itself.
func BenchmarkBatchOnly20k(b *testing.B) {
	s := benchSession(b, benchTick)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tick := float64(i + 1)
		if err := s.App.Batch(func(bt *host.Batch) error { return bt.Set("tick", tick) }); err != nil {
			b.Fatal(err)
		}
	}
}
