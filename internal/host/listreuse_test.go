package host

import (
	"fmt"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/layout"
)

// A live build whose list rows are the ones the slab it takes was last
// materialized from reuses the slab's boxes (listRows.reuse) instead of
// writing each one whole: it puts back only what the steps after
// inflation wrote (the layout's outputs, an item's parent, :selected, and
// :checked), and writes every box whole when display: none dropped a
// child of one of them in that frame.

const reuseDoc = `<tui version="3" mouse="true"><style>
item:selected { bold: true; }
item:checked { italic: true; }
.w { width: 50%; }
@media (max-cols: 30) { .n { display: none; } }
</style><screen id="s" focus="#l">
<text style="dock: top">top {tick}</text>
<list id="l" each="rows as r" key="r.id" bind="cur" checked="sel" mark="*"><item><row><text class="w">{r.name}</text><text class="n">{r.n}</text><scroll style="height: 1; width: 6"><text>{r.n}</text><text>{r.name}</text></scroll></row></item></list>
</screen></tui>`

func reuseApp(t *testing.T, n int) *App {
	t.Helper()
	a, err := Parse(strings.NewReader(reuseDoc))
	if err != nil {
		t.Fatal(err)
	}
	rows := make([]any, n)
	for i := range rows {
		rows[i] = map[string]any{"id": fmt.Sprintf("r%03d", i), "name": strings.Repeat("ab ", i%5+1), "n": float64(i)}
	}
	if err := a.Bind("", map[string]any{"rows": rows, "cur": "r000", "sel": []any{}, "tick": 0.0}); err != nil {
		t.Fatal(err)
	}
	return a
}

// Each step builds one live frame, from rows resolved once at the start
// (no step changes what they read), so from the fourth frame on every
// build takes a slab materialized from the same rows: the frame is the
// one a cold app builds from the same state, whatever the reused boxes
// held two frames before (a :checked row whose list has no checked keys
// now, a child display: none dropped at another width, geometry, an
// offset the wheel gave a row's scroll).
func TestReusedRowsAreTheRowsWrittenWhole(t *testing.T) {
	warm, cold := reuseApp(t, 40), reuseApp(t, 40)
	cold.noCache = true
	tick := 0.0
	next := func(a *App) { _ = a.Set("tick", tick) }
	set := func(path string, v any) func(a *App) {
		return func(a *App) { _ = a.Set(path, v) }
	}
	key := func(name string) func(a *App) {
		return func(a *App) { a.HandleKey(Key{Name: name}) }
	}
	// wheel turns the wheel down over the scroll of the second row shown.
	wheel := func(a *App) {
		var sc *layout.Box
		n := 0
		a.Frame(40, 12).Root.Walk(func(b *layout.Box) {
			if b.Kind == "scroll" && b.Laid {
				if n++; n == 2 {
					sc = b
				}
			}
		})
		if sc == nil {
			t.Fatal("no scroll in the second row")
		}
		a.HandleMouse(Mouse{Kind: MouseWheelDown, X: sc.Content.X, Y: sc.Content.Y})
	}
	steps := []struct {
		do         func(a *App)
		cols, rows int
	}{
		{next, 40, 12},
		{next, 40, 12},
		{next, 40, 12},
		{set("sel", []any{"r002"}), 40, 12},
		{next, 40, 12},
		// No checked keys: the row loop writes no row's :checked, and the
		// reused row 2 was checked.
		{set("sel", []any{}), 40, 12},
		{next, 40, 12},
		// Narrow: display: none drops every row's .n, in one slab.
		{next, 25, 8},
		{next, 40, 12},
		// That slab's rows lost a child: written whole again.
		{next, 40, 12},
		{next, 40, 12},
		{key("down"), 40, 12},
		{key("down"), 80, 24},
		{key("end"), 25, 8},
		{key("home"), 40, 12},
		{wheel, 40, 12},
		{next, 40, 12},
		{next, 40, 12},
		{next, 30, 12},
		{next, 31, 12},
		{next, 40, 12},
	}
	for i, st := range steps {
		tick++
		st.do(warm)
		st.do(cold)
		fw, fc := warm.Frame(st.cols, st.rows), cold.Frame(st.cols, st.rows)
		if d := FrameDiff(fw, fc); d != "" {
			t.Fatalf("step %d: the frame from reused rows differs from a cold build: %s", i, d)
		}
	}
	// The steps above went through the reuse: two live builds apart, a
	// box no step writes after inflation keeps what it holds.
	a := reuseApp(t, 5)
	for i := 0; i < 3; i++ {
		a.Frame(40, 12)
		_ = a.Set("tick", float64(i+1))
	}
	f := a.Frame(40, 12)
	firstRow(f).Tag = "seen"
	_ = a.Set("tick", 10.0)
	a.Frame(40, 12)
	_ = a.Set("tick", 11.0)
	if g := a.Frame(40, 12); firstRow(g) != firstRow(f) || firstRow(g).Tag != "seen" {
		t.Fatalf("a build from unchanged rows did not reuse the slab's boxes")
	}
	// Rows resolved again are written whole.
	_ = a.Set("rows.0.name", "other")
	a.Frame(40, 12)
	_ = a.Set("tick", 12.0)
	if g := a.Frame(40, 12); firstRow(g).Tag == "seen" {
		t.Fatalf("a build from rows resolved again kept a box member of the rows before")
	}
}
