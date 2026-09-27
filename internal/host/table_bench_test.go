package host

import (
	"fmt"
	"strings"
	"testing"
)

// benchTable is SPEC v0.2b §21 test 73's document: a 1000-row, 7-column
// table, rendered at 200×60.
const benchTable = `<tui version="2"><style>#t { border: rounded; scrollbar: auto; } .num { content-align: end; }</style>
<screen id="s" focus="#t">
<table id="t" each="procs as p" key="p.pid" bind="cur" checked="marked" mark="▸" gap="1">
  <item class:blocked="p.protected"/>
  <column id="c-pid" class="num" title="PID" width="7">{p.pid}</column>
  <column id="c-name" title="Name" width="1fr" style="min-width: 10; max-width: 44">{p.name}</column>
  <column id="c-user" title="User">{p.user}</column>
  <column id="c-cpu" class="num" title="CPU%" class:hot="p.hot">{p.cpu}</column>
  <column id="c-mem" class="num" title="MEM">{p.mem}</column>
  <column id="c-thr" class="num" title="THR">{p.threads}</column>
  <column id="c-cmd" title="Command" width="30%">{p.cmd}</column>
</table>
</screen></tui>`

func benchApp(b *testing.B) *App {
	b.Helper()
	a := docB(b, benchTable)
	procs := make([]any, 1000)
	for i := range procs {
		procs[i] = map[string]any{
			"pid": float64(1000 + i), "name": fmt.Sprintf("process-%04d", i), "user": "root",
			"cpu": float64(i%100) / 3, "mem": fmt.Sprintf("%dM", i%512), "threads": float64(i % 64),
			"cmd": fmt.Sprintf("/usr/bin/process-%04d --flag", i), "hot": i%7 == 0, "protected": i%11 == 0,
		}
	}
	if err := a.Bind("", map[string]any{"procs": procs, "cur": float64(1500), "marked": []any{}}); err != nil {
		b.Fatal(err)
	}
	return a
}

// BenchmarkFrame is SPEC v0.2b §21 test 73 (a SHOULD, recorded, not a
// gate): one frame of a 1000-row, 7-column table at 200×60, with a warm
// measure cache (only the cursor moved since the last frame) and with a
// cold one (the rows are resolved again, as after a Set of the array).
func BenchmarkFrame(b *testing.B) {
	b.Run("warm", func(b *testing.B) {
		a := benchApp(b)
		a.Frame(200, 60)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = a.Set("cur", float64(1000+i%1000))
			a.Frame(200, 60)
		}
	})
	b.Run("cold", func(b *testing.B) {
		a := benchApp(b)
		a.Frame(200, 60)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			a.mu.Lock()
			a.wrote("procs")
			a.mu.Unlock()
			a.Frame(200, 60)
		}
	})
}

// docB is doc for benchmarks.
func docB(b *testing.B, src string) *App {
	b.Helper()
	a, err := Parse(strings.NewReader(src))
	if err != nil {
		b.Fatal(err)
	}
	return a
}
