package host

import (
	"fmt"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/layout"
)

// The live builds materialize a list's rows into one of two slabs
// (listSlab), the one the frame before does not use: building a frame
// never writes a box of the frame Frame gave before it, which its caller
// still holds, and Dump, Validate, and a cold render never write either
// slab.

const slabDoc = `<tui version="3"><style>item:selected { bold: true; } .w { width: 50%; }</style><screen id="s" focus="#l">
<text style="dock: top">top {tick}</text>
<list id="l" each="rows as r" key="r.id" bind="cur"><item><row><text class="w">{r.name}</text><text>{r.n}</text></row></item></list>
</screen></tui>`

func slabApp(t *testing.T, n int) *App {
	t.Helper()
	a, err := Parse(strings.NewReader(slabDoc))
	if err != nil {
		t.Fatal(err)
	}
	rows := make([]any, n)
	for i := range rows {
		rows[i] = map[string]any{"id": fmt.Sprintf("r%03d", i), "name": strings.Repeat("ab ", i%5+1), "n": float64(i)}
	}
	if err := a.Bind("", map[string]any{"rows": rows, "cur": "r000", "tick": 0.0}); err != nil {
		t.Fatal(err)
	}
	return a
}

func firstRow(f *Frame) *layout.Box {
	var row *layout.Box
	f.Root.Walk(func(b *layout.Box) {
		if row == nil && b.Kind == "item" {
			row = b
		}
	})
	return row
}

func TestLiveBuildKeepsTheFrameBeforeIt(t *testing.T) {
	a := slabApp(t, 60)
	prev := a.Frame(40, 12)
	digest := frameDigest(prev)
	var frames []*Frame
	for i := 0; i < 8; i++ {
		// Move the cursor (and, past the first screen, the scroll offset),
		// so the rows' geometry and :selected differ from frame to frame.
		for k := 0; k < 7; k++ {
			a.HandleKey(Key{Name: "down"})
		}
		f := a.Frame(40, 12)
		if f == prev {
			t.Fatalf("step %d: the frame skip gave the frame back: this test checks nothing", i)
		}
		if d := frameDigest(prev); d != digest {
			t.Fatalf("step %d: building a frame changed the frame before it: %s", i, firstLineDiff(digest, d))
		}
		frames = append(frames, f)
		prev, digest = f, frameDigest(f)
		// Neither a Dump nor a Validate between two frames writes the
		// frame before it.
		a.Dump(40, 12, true)
		a.Dump(17, 5, false)
		a.Validate()
		if d := frameDigest(prev); d != digest {
			t.Fatalf("step %d: Dump or Validate changed the live frame: %s", i, firstLineDiff(digest, d))
		}
	}
	// The slabs are reused: two live builds apart, a row is the same box.
	if firstRow(frames[2]) != firstRow(frames[4]) || firstRow(frames[2]) == firstRow(frames[3]) {
		t.Fatalf("the live builds did not alternate between two slabs")
	}
}

// A live build that reuses a slab writes every member of every box it
// takes from it: the frame is the one a cold app builds from the same
// state, whatever the boxes held two frames before (other geometry, a
// text the engine marked clipped at another width, the engine's memo of
// another layout). Each step builds one frame; only the first frame and
// the steps that change the rows resolve them again (into fresh boxes),
// every other frame takes the slab the frame two steps back used.
func TestReusedSlabHoldsNothingStale(t *testing.T) {
	warm, cold := slabApp(t, 60), slabApp(t, 60)
	cold.noCache = true
	tick := 0.0
	next := func(a *App) { _ = a.Set("tick", tick) }
	steps := []struct {
		do         func(a *App)
		cols, rows int
	}{
		{next, 40, 12},
		{next, 25, 8}, // slab B: the long names are clipped (L008)
		{next, 40, 12},
		{next, 80, 24}, // slab B again: nothing is clipped
		{func(a *App) { a.HandleKey(Key{Name: "end"}) }, 25, 8},
		{func(a *App) { a.HandleKey(Key{Name: "home"}) }, 30, 6},
		{next, 80, 24},
		{func(a *App) { _ = a.Set("rows.3.name", "a much longer name than before") }, 40, 12},
		{next, 25, 8},
		{next, 80, 24},
		// As many bytes as the text the slab's box measured two frames
		// back, but another width: the box measures it again.
		{func(a *App) { _ = a.Set("rows.3.name", "中中中中中中中中中中") }, 40, 12},
		{next, 40, 12},
		{next, 40, 12},
		{func(a *App) { _ = a.Set("rows.3.name", "a much longer name than before") }, 40, 12},
		{next, 40, 12},
		{next, 40, 12},
		{func(a *App) { _ = a.Set("rows", []any{map[string]any{"id": "x", "name": "only", "n": 1.0}}) }, 40, 12},
		{next, 25, 8},
		{func(a *App) { a.HandleKey(Key{Name: "pgdn"}) }, 40, 12},
	}
	for i, st := range steps {
		tick++
		st.do(warm)
		st.do(cold)
		for pass := 0; pass < 2; pass++ {
			fw, fc := warm.Frame(st.cols, st.rows), cold.Frame(st.cols, st.rows)
			if d := FrameDiff(fw, fc); d != "" {
				t.Fatalf("step %d: the frame from a reused slab differs from a cold build: %s", i, d)
			}
		}
	}
}

// Boxes that look alike to every selector hold one computed style
// (layout.Box.Style is a pointer the sharing hands out), and a box whose
// state differs holds another: the selected row is bold, the rows around
// it are not, whatever they share. A cold app computes each style on its
// own, and the styles are the same.
func TestRowsShareOneStyleAndTheSelectedRowItsOwn(t *testing.T) {
	warm, cold := slabApp(t, 60), slabApp(t, 60)
	cold.noCache = true
	warm.Frame(40, 12)
	cold.Frame(40, 12)
	for i := 0; i < 2; i++ {
		warm.HandleKey(Key{Name: "down"})
		cold.HandleKey(Key{Name: "down"})
	}
	fw, fc := warm.Frame(40, 12), cold.Frame(40, 12)
	if d := FrameDiff(fw, fc); d != "" {
		t.Fatalf("shared styles differ from a cold build: %s", d)
	}
	items := func(f *Frame) []*layout.Box {
		var out []*layout.Box
		f.Root.Walk(func(b *layout.Box) {
			if b.Kind == "item" {
				out = append(out, b)
			}
		})
		return out
	}
	rows, coldRows := items(fw), items(fc)
	if len(rows) < 4 || len(rows) != len(coldRows) {
		t.Fatalf("rows: %d warm, %d cold", len(rows), len(coldRows))
	}
	if rows[0].Style != rows[1].Style || rows[0].Style != rows[3].Style {
		t.Fatalf("unselected rows do not share one style")
	}
	if !rows[2].Selected || rows[2].Style == rows[0].Style || !rows[2].Style.Bold || rows[0].Style.Bold {
		t.Fatalf("the selected row does not hold a bold style of its own")
	}
	if coldRows[0].Style == coldRows[1].Style {
		t.Fatalf("a cold build shared a style")
	}
}
