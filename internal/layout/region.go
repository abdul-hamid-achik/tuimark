package layout

import "github.com/abdul-hamid-achik/tuimark/internal/ir"

// The inline region (SPEC v0.4d §26.15.2, decision 242): with
// Engine.Inline the root is the active screen laid out as a live region
// at the terminal's width and at the height of its content, capped, and
// the layers (modals, the open option list) are laid out against that
// region as against a screen of its size (decision 249).

// RegionHeight is the height of the inline region for root at width cols
// in a terminal of rows rows (SPEC v0.4d §26.15.2):
//
//   - the screen's content height, measured as a modal sized by its
//     content is (a % child on that axis is L002, a col or row child of
//     the screen is 1fr);
//   - replaced by a height in cells that the document writes on the
//     screen, which is the only kind that counts (the built-in
//     height: 100%, and a written %, fr, or auto height, are unwritten);
//   - capped by a max-height in cells and by the terminal's rows;
//   - raised to a min-height in cells, never past the terminal's rows.
func (e *Engine) RegionHeight(root *Box, cols, rows int) int {
	root.autoH = true
	h := e.intrinsic(root, false, cols)
	if root.Style.Height.Kind == ir.Cell {
		h = cells(root.Style.Height.N)
	}
	if root.Style.MaxH.Kind == ir.Cell {
		h = min(h, cells(root.Style.MaxH.N))
	}
	h = min(h, rows)
	if root.Style.MinH.Kind == ir.Cell {
		h = min(max(h, cells(root.Style.MinH.N)), rows)
	}
	return max(h, 0)
}

// LayoutRoot lays b out as a root of its own (SPEC v0.4d §26.15.4,
// decisions 244 and 245): from the cell (0, 0), cols wide, as high as its
// content, with no cap, and returns that height. A committed line or the
// copy of a committed template is laid out so, after the cascade has
// styled it as a child of the active screen.
func (e *Engine) LayoutRoot(b *Box, cols int) int {
	if e.memo == nil {
		e.memo = map[memoKey]int{}
		e.seen = map[string]bool{}
	}
	e.scratch().reset()
	b.autoH = true
	h := e.intrinsic(b, false, cols)
	r := Rect{0, 0, cols, h}
	e.place(b, r, r)
	return h
}
