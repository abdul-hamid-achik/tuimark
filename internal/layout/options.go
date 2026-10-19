package layout

// The option list of an open <select> (SPEC v0.4b §6.17.4, §18 step 7a):
// a generated list that no pass of §11 lays out. It is placed after the
// screen and every open modal, from its select's visible rect, in the
// modal layer above all of them, and no layout diagnostic reports it, its
// rows, or their texts (§11.3).

// PlaceOptions places option list l, whose style is computed, for a
// select whose visible rect (its outer rect clipped to its clip rect and
// to the grid) is sel, with n options whose widest label is lw columns
// wide, on a grid of cols × rows (SPEC v0.4b §6.17.4), and returns V, the
// number of rows it shows. With hb and vb its horizontal and vertical
// border plus padding:
//
//   - w = max(sel.W, lw + hb), clamped by its min-width and max-width,
//     each only when it is a cell count or a percentage of cols, then
//     w = min(w, cols), and x = max(0, min(sel.X, cols − w));
//   - h0 = n + vb, clamped by its min-height and max-height, each only
//     when it is a cell count or a percentage of rows; with below = rows −
//     (sel.Y + sel.H) and above = sel.Y, the list is below the select (y =
//     sel.Y + sel.H, h = h0) when h0 ≤ below, else above it (h = min(h0,
//     above), y = sel.Y − h) when above > below, else below it, cut to
//     what is left (h = below);
//   - V = max(0, h − vb).
//
// Its own width, height, flex, dock, margin, layout, gap, align, justify,
// overflow, and display are ignored. It is Modal, so it clears what lies
// under it (§12.5), and its clip is the grid cut by its rect. The caller
// sets its offset (ScrollY) and content height and places its rows
// (PlaceOptionRow).
func PlaceOptions(l *Box, sel Rect, n, lw, cols, rows int) int {
	hb, vb := l.frameH(), l.frameV()
	w := clampAxis(l, true, max(sel.W, lw+hb), cols)
	w = max(min(w, cols), 0)
	x := max(0, min(sel.X, cols-w))
	h0 := clampAxis(l, false, n+vb, rows)
	below, above := max(rows-(sel.Y+sel.H), 0), max(sel.Y, 0)
	var y, h int
	switch {
	case h0 <= below:
		y, h = sel.Y+sel.H, h0
	case above > below:
		h = min(h0, above)
		y = sel.Y - h
	default:
		y, h = sel.Y+sel.H, below
	}
	l.X, l.Y, l.W, l.H = x, y, w, h
	l.Laid, l.Modal = true, true
	l.Clip = Rect{W: cols, H: rows}.Intersect(l.Outer())
	bw := l.border()
	l.Content = Rect{
		X: l.X + bw + l.pad(3),
		Y: l.Y + bw + l.pad(0),
		W: max(0, l.W-hb),
		H: max(0, l.H-vb),
	}
	return max(0, h-vb)
}

// OptionsOffset is the offset of an option list showing v of its n rows
// with its cursor at cursor, from the offset o it had (SPEC v0.4b §6.17.4,
// as a table's follows its cursor, §6.9.3): o becomes the cursor when the
// cursor is above the rows shown, and cursor − v + 1 when it is below
// them; it is then clamped to [0, max(0, n − v)].
func OptionsOffset(o, cursor, v, n int) int {
	if v > 0 {
		if cursor < o {
			o = cursor
		} else if cursor >= o+v {
			o = cursor - v + 1
		}
	}
	return min(max(o, 0), max(0, n-v))
}

// PlaceOptionRow places row, the item of option i of option list l (and
// its text, row.Children[0]), at (content x, content y + i − o), Wc × 1
// with Wc l's content width and o its offset (ScrollY): rows and their
// texts ignore their own sizes, spacing, borders, dock, layout, and
// display, as a table's rows do (§6.9.3). Both are clipped at l's content
// box.
func PlaceOptionRow(l, row *Box, i int) {
	c := l.Content
	clip := l.Clip.Intersect(c)
	row.X, row.Y, row.W, row.H = c.X, c.Y+i-l.ScrollY, c.W, 1
	row.Laid = true
	row.Clip = clip.Intersect(row.Outer())
	row.Content = row.Outer()
	for _, t := range row.Children {
		t.X, t.Y, t.W, t.H = row.X, row.Y, row.W, 1
		t.Laid = true
		t.Content = t.Outer()
		t.Clip = row.Clip
	}
}
