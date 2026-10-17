//go:build tuimark_verify

package layout

import (
	"fmt"
	"maps"
)

// verifying is set by the tuimark_verify build tag: the engine then also
// computes again what it would otherwise take from its memo across
// passes (scrollLayout's content height), checks the frame's checks
// against one walk over it (verifyChecks), and panics when the two
// differ. `go test -tags tuimark_verify ./...` runs the whole suite that
// way.
const verifying = true

// checkSnap is what the checks read of the engine before a Layout: the
// diagnostics already reported (seen) and the boxes L006 was reported on.
type checkSnap struct {
	seen map[string]bool
	l006 map[*Box]bool
}

func (e *Engine) snapChecks() checkSnap {
	return checkSnap{seen: maps.Clone(e.seen), l006: maps.Clone(e.l006)}
}

// verifyChecks runs the checks of this Layout (L006, L008, L009, and
// Clipped) again as one walk after layout, check over root's tree and
// then each modal's, on a second engine that starts from snap and reads
// the L003 this Layout reported, and panics unless the walk reports the
// same diagnostics, in the same order, and marks the same boxes Clipped
// as placed did. Every box's Clipped is left as placed set it.
func (e *Engine) verifyChecks(root *Box, modals []*Box, nDiags int, snap checkSnap) {
	var placed []string
	for _, d := range e.Diags[nDiags:] {
		switch d.Code {
		case "L006", "L008", "L009":
			placed = append(placed, fmt.Sprintf("%+v", d))
		}
	}
	type mark struct {
		b       *Box
		clipped string
	}
	var marks []mark
	var clear func(b *Box)
	clear = func(b *Box) {
		marks = append(marks, mark{b, b.Clipped})
		b.Clipped = ""
		for _, c := range b.Children {
			clear(c)
		}
	}
	trees := append([]*Box{root}, modals...)
	for _, t := range trees {
		clear(t)
	}
	seen := snap.seen
	if seen == nil {
		seen = map[string]bool{}
	}
	w := &Engine{File: e.File, V3: e.V3, seen: seen, l003: e.l003, l006: snap.l006}
	for _, t := range trees {
		w.check(t)
	}
	var walked []string
	for _, d := range w.Diags {
		walked = append(walked, fmt.Sprintf("%+v", d))
	}
	for _, m := range marks {
		if m.b.Clipped != m.clipped {
			panic(fmt.Sprintf("layout: %s %q is Clipped %q as placed, %q in a walk after layout", m.b.Kind, m.b.ID, m.clipped, m.b.Clipped))
		}
	}
	if fmt.Sprint(placed) != fmt.Sprint(walked) {
		panic(fmt.Sprintf("layout: the checks as placed reported\n%v\nand a walk after layout\n%v", placed, walked))
	}
}
