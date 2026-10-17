package layout

import (
	"fmt"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
)

// An arena hands out zeroed slices that stay put until reset, and one
// round's chunks are merged so the same round fits one chunk next time.
func TestArenaHandsOutZeroedSlicesThatStayPut(t *testing.T) {
	var a arena[int]
	first := a.take(3)
	for i := range first {
		first[i] = 7
	}
	big := a.take(arenaChunk) // a second chunk: first must not move
	big[0] = 1
	if first[0] != 7 || first[2] != 7 || len(a.chunks) != 2 {
		t.Fatalf("first %v, %d chunks", first, len(a.chunks))
	}
	a.reset(false)
	if len(a.chunks) != 1 || len(a.chunks[0]) != 2*arenaChunk {
		t.Fatalf("after reset: %d chunks", len(a.chunks))
	}
	again := a.take(3)
	if again[0] != 0 || again[1] != 0 || again[2] != 0 || cap(again) != 3 {
		t.Fatalf("a reused slice is not zeroed: %v (cap %d)", again, cap(again))
	}
	var p arena[*Box]
	ps := p.take(2)
	ps[0], ps[1] = &Box{}, &Box{}
	p.reset(true)
	if p.chunks[0][0] != nil || p.chunks[0][1] != nil {
		t.Fatalf("a reset pointer arena still holds what the round left")
	}
}

// scratchTree is a column holding a scroll of many texts of varied
// widths (a long list's rows), a row of several boxes sharing it by
// percent and fr, and a docked status line.
func scratchTree(rows int) *Box {
	var items []*Box
	for i := 0; i < rows; i++ {
		items = append(items, n("item", "", txt(strings.Repeat("ab ", i%7+1)+fmt.Sprint(i), "")))
	}
	list := n("scroll#s", "height: 1fr", items...)
	list.ScrollY = rows / 3
	var cells []*Box
	for i := 0; i < 12; i++ {
		decl := "width: 1fr"
		if i%3 == 0 {
			decl = "width: 10%"
		}
		cells = append(cells, n("box", decl, txt(strings.Repeat("x", i+1), "")))
	}
	return n("col", "", txt("status", "dock: bottom"), n("row", "height: 3", cells...), list)
}

func scratchGeometry(root *Box) string {
	var sb strings.Builder
	root.Walk(func(b *Box) {
		fmt.Fprintf(&sb, "%s %v %v %v %d %d %d %v\n", b.Kind, b.Outer(), b.Content, b.Clip, b.ContentW, b.ContentH, b.ScrollY, b.Laid)
	})
	return sb.String()
}

// One Scratch serving Layout after Layout, at other sizes and over other
// trees, lays out every tree as an engine with its own memory does.
func TestReusedScratchLaysOutAsFreshMemory(t *testing.T) {
	var s Scratch
	for i, st := range []struct{ rows, cols, h int }{
		{3000, 80, 24}, {40, 30, 10}, {5000, 120, 40}, {3000, 80, 24}, {9, 20, 6},
	} {
		lay := func(sc *Scratch) (string, string) {
			root := scratchTree(st.rows)
			c := css.NewCascade(nil, css.Env{Cols: st.cols, Rows: st.h})
			styleTree(c, root, nil)
			e := &Engine{File: "t.tui", V3: true, Scratch: sc}
			e.Layout(root, nil, st.cols, st.h)
			return scratchGeometry(root), fmt.Sprint(e.Diags)
		}
		gw, dw := lay(&s)
		gc, dc := lay(nil)
		if gw != gc || dw != dc {
			t.Fatalf("step %d: a reused Scratch lays out otherwise than fresh memory: %s", i, firstDiff(gw, gc))
		}
	}
}

func firstDiff(a, b string) string {
	al, bl := strings.Split(a, "\n"), strings.Split(b, "\n")
	for i := range al {
		if i >= len(bl) || al[i] != bl[i] {
			y := ""
			if i < len(bl) {
				y = bl[i]
			}
			return fmt.Sprintf("line %d:\n  %s\n  %s", i, al[i], y)
		}
	}
	return "diagnostics differ"
}
