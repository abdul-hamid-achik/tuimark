package layout

import (
	"fmt"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// Engine lays out one frame and collects L-diagnostics.
type Engine struct {
	File  string
	Diags ir.Diags
	memo  map[memoKey]int
	seen  map[string]bool
}

type memoKey struct {
	b          *Box
	horizontal bool
	avail      int
}

// Layout places root to fill (at most) cols×rows, then centers each open
// modal over the root. Every placed box gets Laid=true.
func (e *Engine) Layout(root *Box, modals []*Box, cols, rows int) {
	if e.memo == nil {
		e.memo = map[memoKey]int{}
		e.seen = map[string]bool{}
	}
	screen := Rect{0, 0, cols, rows}
	w := min(clampAxis(root, true, resolveRoot(root.Style.Width, cols), cols), cols)
	h := min(clampAxis(root, false, resolveRoot(root.Style.Height, rows), rows), rows)
	e.place(root, Rect{0, 0, w, h}, screen)
	for _, m := range modals {
		m.Modal = true
		// min/max apply like anywhere else; the modal never outgrows the root.
		mw := min(clampAxis(m, true, resolveModal(e, m, true, root.W, root.H), root.W), root.W)
		mh := min(clampAxis(m, false, resolveModal(e, m, false, root.H, mw), root.H), root.H)
		x := root.X + (root.W-mw)/2
		y := root.Y + (root.H-mh)/2
		e.place(m, Rect{x, y, mw, mh}, screen)
	}
}

func resolveRoot(s ir.Scalar, full int) int {
	switch s.Kind {
	case ir.Cell:
		return min(cells(s.N), full)
	case ir.Pct:
		return min(percent(full, s), full)
	}
	return full
}

// resolveModal sizes a modal on one axis: cells, % of the root, auto
// (intrinsic; cross is the size on the other axis), or 80% when unsized.
func resolveModal(e *Engine, m *Box, horizontal bool, full, cross int) int {
	s := m.Style.Height
	if horizontal {
		s = m.Style.Width
	}
	switch s.Kind {
	case ir.Cell:
		return cells(s.N)
	case ir.Pct:
		return percent(full, s)
	case ir.Auto:
		// Sized by its content: a % child on this axis is L002.
		if horizontal {
			m.autoW = true
		} else {
			m.autoH = true
		}
		return e.intrinsic(m, horizontal, cross)
	}
	return percent(full, ir.Scalar{Kind: ir.Pct, N: 80})
}

func (e *Engine) report(b *Box, sev, code, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	path, id, line, col := "", "", 0, 0
	if b != nil {
		id = b.ID
		if b.Src != nil {
			path, line, col = b.Src.Path, b.Src.Line, b.Src.Col
		}
	}
	key := code + "|" + path + "|" + id + "|" + msg
	if e.seen[key] {
		return
	}
	e.seen[key] = true
	e.Diags = append(e.Diags, ir.Diagnostic{Severity: sev, Code: code, Msg: msg, File: e.File, Line: line, Col: col, Path: path, ID: id})
}

func (e *Engine) place(b *Box, r Rect, clip Rect) {
	b.X, b.Y, b.W, b.H = r.X, r.Y, max(r.W, 0), max(r.H, 0)
	b.Laid = true
	b.Clip = clip.Intersect(b.Outer())
	bw := b.border()
	b.Content = Rect{
		X: b.X + bw + b.pad(3),
		Y: b.Y + bw + b.pad(0),
		W: max(0, b.W-b.frameH()),
		H: max(0, b.H-b.frameV()),
	}
	if !b.IsContainer() || len(b.Children) == 0 {
		return
	}
	childClip := b.Clip.Intersect(b.Content)
	switch {
	case b.Scrolls():
		e.scrollLayout(b, childClip)
	default:
		area := e.dock(b, b.Content, childClip)
		var flow []*Box
		for _, c := range b.Children {
			if c.Style.Dock == "" {
				flow = append(flow, c)
			}
		}
		e.flex(b, flow, area, childClip, false)
	}
}

func margins(b *Box, horizontal bool) (start, end int) {
	if horizontal {
		return clampCells(b.Style.Margin[3]), clampCells(b.Style.Margin[1])
	}
	return clampCells(b.Style.Margin[0]), clampCells(b.Style.Margin[2])
}

func sizeSpec(b *Box, horizontal bool) ir.Scalar {
	if horizontal {
		return b.Style.Width
	}
	return b.Style.Height
}

// mainSpec is the size spec along the parent's main axis; flex: N (N>0)
// is an alias for Nfr there. flex: 0 is an explicit grow weight of zero:
// the box keeps its own size (auto when it has none) and never joins the
// fr pool through the sibling-fr rule (SPEC §9, §11.2).
func mainSpec(b *Box, horizontal bool) ir.Scalar {
	if b.Style.FlexSet {
		if b.Style.Flex > 0 {
			return ir.Scalar{Kind: ir.Fr, N: b.Style.Flex, Lit: b.Style.FlexLit}
		}
		if s := sizeSpec(b, horizontal); s.Kind != ir.Unset {
			return s
		}
		return ir.Scalar{Kind: ir.Auto}
	}
	return sizeSpec(b, horizontal)
}

// DockConflict reports the V010 condition (SPEC §14) for a box with
// computed style st whose parent lays out as a row (parentRow) or a column:
// an fr size on the dock's own axis, or flex > 0 when the dock's axis is the
// parent's main axis (flex is a grow weight on the main axis only, §9).
// Only sizes the document wrote count (a presentational attribute, author
// CSS, style=""): V010 is a parse-pass code about the markup, so the
// built-in sheet's defaults (col/row 1fr, spacer flex: 1) never conflict;
// a docked box treats them as auto like any other fr on its axis.
func DockConflict(st css.Style, parentRow bool) bool {
	grows := st.FlexSet && st.Flex > 0 && !st.FlexUA
	switch st.Dock {
	case "top", "bottom":
		return (st.Height.Kind == ir.Fr && !st.HeightUA) || (grows && !parentRow)
	case "left", "right":
		return (st.Width.Kind == ir.Fr && !st.WidthUA) || (grows && parentRow)
	}
	return false
}

// DockConflictMsg is the V010 message for a box docked to side.
func DockConflictMsg(side string) string {
	return fmt.Sprintf("dock: %s and an fr/flex size on the same axis; docked size must be cells, %%, or auto", side)
}

// clampAxis applies min/max on one axis. Percent bounds resolve against the
// parent's content size.
func clampAxis(b *Box, horizontal bool, v, parent int) int {
	lo, hi := b.Style.MinH, b.Style.MaxH
	if horizontal {
		lo, hi = b.Style.MinW, b.Style.MaxW
	}
	res := func(s ir.Scalar) int {
		if s.Kind == ir.Pct {
			return percent(parent, s)
		}
		return cells(s.N)
	}
	if hi.Kind == ir.Cell || (hi.Kind == ir.Pct && parent >= 0) {
		v = min(v, res(hi))
	}
	if lo.Kind == ir.Cell || (lo.Kind == ir.Pct && parent >= 0) {
		v = max(v, res(lo))
	}
	return max(v, 0)
}

func hasMin(b *Box, horizontal bool) bool {
	if horizontal {
		return b.Style.MinW.Kind != ir.Unset
	}
	return b.Style.MinH.Kind != ir.Unset
}

// dock carves docked children out of area: top/bottom first, then
// left/right (SPEC §11.3 pass 1). It returns the remaining area.
func (e *Engine) dock(b *Box, area Rect, clip Rect) Rect {
	bottoms := 0
	row := b.Direction() == "row"
	// Percent sizes and percent min/max resolve against the container's
	// content box, not against what earlier docks left (SPEC §9, §11.4).
	baseW, baseH := area.W, area.H
	for pass := 0; pass < 2; pass++ {
		for _, d := range b.Children {
			side := d.Style.Dock
			vertical := side == "top" || side == "bottom"
			if side == "" || (pass == 0) != vertical {
				continue
			}
			if side == "bottom" {
				bottoms++
			}
			horizontal := !vertical
			s := sizeSpec(d, horizontal)
			if DockConflict(d.Style, row) {
				e.report(d, ir.Error, "V010", "%s", DockConflictMsg(side))
				s = ir.Scalar{Kind: ir.Auto}
			}
			ms, me := margins(d, horizontal)
			cs, ce := margins(d, !horizontal)
			full, cross, base := area.H, area.W, baseH
			if horizontal {
				full, cross, base = area.W, area.H, baseW
			}
			var size int
			switch s.Kind {
			case ir.Cell:
				size = cells(s.N)
			case ir.Pct:
				size = percent(base, s)
			default:
				size = e.intrinsic(d, horizontal, max(0, cross-cs-ce))
				// Sized by its content: a % child on this axis is L002.
				if horizontal {
					d.autoW = true
				} else {
					d.autoH = true
				}
			}
			size = clampAxis(d, horizontal, size, base)
			slot := min(size+ms+me, full)
			inner := max(0, slot-ms-me)
			crossSize := max(0, cross-cs-ce)
			var r Rect
			switch side {
			case "top":
				r = Rect{area.X + cs, area.Y + ms, crossSize, inner}
				area.Y += slot
				area.H -= slot
			case "bottom":
				r = Rect{area.X + cs, area.Y + area.H - slot + ms, crossSize, inner}
				area.H -= slot
			case "left":
				r = Rect{area.X + ms, area.Y + cs, inner, crossSize}
				area.X += slot
				area.W -= slot
			case "right":
				r = Rect{area.X + area.W - slot + ms, area.Y + cs, inner, crossSize}
				area.W -= slot
			}
			e.place(d, r, clip)
		}
	}
	if bottoms > 1 {
		e.report(b, ir.Warning, "L005", "%d children are docked to the bottom; only one status strip is expected", bottoms)
	}
	return area
}

func isStretch(align string) bool { return align == "" || align == "stretch" }

// crossSize resolves a child's border-box size on the parent's cross axis.
// mainSize is the child's main-axis size when already known (rows), else -1.
func (e *Engine) crossSize(parent, k *Box, row bool, cross, mainSize int, report bool) int {
	horizontal := !row // cross axis of a row is vertical
	s := sizeSpec(k, horizontal)
	cs, ce := margins(k, horizontal)
	avail := max(0, cross-cs-ce)
	parentAuto := parent.autoW
	if !horizontal {
		parentAuto = parent.autoH
	}
	var v int
	switch s.Kind {
	case ir.Cell:
		v = cells(s.N)
	case ir.Pct:
		if parentAuto {
			if report {
				e.report(k, ir.Error, "L002", "%s: %s is a percent of a parent sized by its content (auto); give the parent a size", axisName(horizontal), s)
			}
			v = e.intrinsic(k, horizontal, max(mainSize, 0))
		} else {
			v = percent(cross, s)
		}
	case ir.Auto:
		if isStretch(parent.Style.Align) {
			v = avail
		} else {
			v = e.intrinsic(k, horizontal, max(mainSize, 0))
			if report {
				if horizontal {
					k.autoW = true
				} else {
					k.autoH = true
				}
			}
		}
	default: // Fr or unspecified: fill the cross axis
		v = avail
	}
	return clampAxis(k, horizontal, v, cross)
}

func axisName(horizontal bool) string {
	if horizontal {
		return "width"
	}
	return "height"
}

// mainSizes allocates slots (including margins) along the main axis per
// SPEC §11.4. crossSizes must be known for column parents (auto heights of
// wrapped text depend on width) and may be nil for rows.
func (e *Engine) mainSizes(b *Box, kids []*Box, row bool, main int, crossSizes []int, unbounded, report bool) (sizes []int, anyFr bool) {
	n := len(kids)
	sizes = make([]int, n)
	if n == 0 {
		return sizes, false
	}
	horizontal := row
	specs := make([]ir.Scalar, n)
	for i, k := range kids {
		specs[i] = mainSpec(k, horizontal)
		if specs[i].Kind == ir.Fr {
			if unbounded {
				if report {
					e.report(k, ir.Error, "L001", "%s: %s inside a scrolling %s has no leftover to share; use cells or auto", axisName(horizontal), specs[i], b.Kind)
				}
				specs[i] = ir.Scalar{Kind: ir.Auto}
				continue
			}
			anyFr = true
		}
	}
	for i := range specs {
		if specs[i].Kind == ir.Unset {
			if anyFr {
				specs[i] = ir.Scalar{Kind: ir.Fr, N: 1}
			} else {
				specs[i] = ir.Scalar{Kind: ir.Auto}
			}
		}
	}
	gap := b.Style.Gap
	remain := max(0, main-gap*(n-1))
	parentAuto := b.autoH
	if horizontal {
		parentAuto = b.autoW
	}
	weights := make([]ir.Scalar, n)
	// L003 sums what cannot shrink: fixed cells and min bounds.
	fixedSum := 0
	fixedOrMin := false
	for i, k := range kids {
		ms, me := margins(k, horizontal)
		s := specs[i]
		cross := 0
		if crossSizes != nil {
			cross = crossSizes[i]
		}
		if s.Kind == ir.Pct && parentAuto {
			if report {
				e.report(k, ir.Error, "L002", "%s: %s is a percent of a parent sized by its content (auto); give the parent a size", axisName(horizontal), s)
			}
			s = ir.Scalar{Kind: ir.Auto}
		}
		var take int
		switch s.Kind {
		case ir.Cell:
			take = cells(s.N)
		case ir.Pct:
			take = percent(main, s)
		case ir.Auto:
			take = e.intrinsic(k, horizontal, cross)
			if report {
				if horizontal {
					k.autoW = true
				} else {
					k.autoH = true
				}
			}
		case ir.Fr:
			weights[i] = s
			sizes[i] = -1
			if hasMin(k, horizontal) {
				fixedSum += clampAxis(k, horizontal, 0, main) + ms + me
				fixedOrMin = true
			}
			continue
		}
		take = clampAxis(k, horizontal, take, main)
		if s.Kind == ir.Cell || hasMin(k, horizontal) {
			fixedSum += take + ms + me
			fixedOrMin = true
		}
		sizes[i] = take + ms + me
		remain -= take + ms + me
	}
	if report && fixedOrMin && fixedSum > max(0, main-gap*(n-1)) {
		e.report(b, ir.Warning, "L003", "fixed and min sizes need %d cells but the %s has %d on this axis; content is clipped", fixedSum, b.Kind, max(0, main-gap*(n-1)))
	}
	if remain < 0 {
		remain = 0
	}
	if anyFr {
		var frs []int
		var ws []ir.Scalar
		for i := range kids {
			if sizes[i] == -1 {
				frs = append(frs, i)
				ws = append(ws, weights[i])
			}
		}
		for j, take := range frSplit(remain, ws) {
			k := kids[frs[j]]
			ms, me := margins(k, horizontal)
			inner := clampAxis(k, horizontal, max(0, take-ms-me), main)
			sizes[frs[j]] = inner + ms + me
		}
	}
	for i := range sizes {
		if sizes[i] < 0 {
			sizes[i] = 0
		}
	}
	return sizes, anyFr
}

// flex allocates and places in-flow children of b inside area.
func (e *Engine) flex(b *Box, kids []*Box, area Rect, clip Rect, unbounded bool) {
	n := len(kids)
	if n == 0 {
		return
	}
	row := b.Direction() == "row"
	main, cross := area.H, area.W
	if row {
		main, cross = area.W, area.H
	}
	var crossSizes []int
	if !row {
		crossSizes = make([]int, n)
		for i, k := range kids {
			crossSizes[i] = e.crossSize(b, k, row, cross, -1, true)
		}
	}
	sizes, anyFr := e.mainSizes(b, kids, row, main, crossSizes, unbounded, true)
	if row {
		crossSizes = make([]int, n)
		for i, k := range kids {
			ms, me := margins(k, true)
			crossSizes[i] = e.crossSize(b, k, row, cross, max(0, sizes[i]-ms-me), true)
		}
	}
	gap := b.Style.Gap
	total := gap * (n - 1)
	for _, s := range sizes {
		total += s
	}
	leftover := main - total
	pos, extra, extraRem := 0, 0, 0
	if !anyFr && leftover > 0 {
		switch b.Style.Justify {
		case "center":
			pos = leftover / 2
		case "end":
			pos = leftover
		case "space-between":
			if n > 1 {
				extra = leftover / (n - 1)
				extraRem = leftover % (n - 1)
			}
		}
	}
	for i, k := range kids {
		ms, me := margins(k, row)
		cs, ce := margins(k, !row)
		boxMain := max(0, sizes[i]-ms-me)
		c := crossSizes[i]
		avail := cross - cs - ce
		co := 0
		switch b.Style.Align {
		case "center":
			co = max(0, (avail-c)/2)
		case "end":
			co = max(0, avail-c)
		}
		var r Rect
		if row {
			r = Rect{area.X + pos + ms, area.Y + cs + co, boxMain, c}
		} else {
			r = Rect{area.X + cs + co, area.Y + pos + ms, c, boxMain}
		}
		e.place(k, r, clip)
		pos += sizes[i] + gap
		if i < n-1 {
			pos += extra
			if i == n-2 {
				pos += extraRem
			}
		}
	}
}

// scrollLayout lays out a scroll or list: content keeps its natural size on
// the scroll axis and is shifted by the (clamped) offset.
func (e *Engine) scrollLayout(b *Box, clip Rect) {
	area := b.Content
	axis := b.Axis
	if b.Kind == "list" || axis == "" {
		axis = "y"
	}
	scrollY := axis == "y" || axis == "both"
	scrollX := axis == "x" || axis == "both"
	kids := b.Children
	contentW, contentH := area.W, area.H
	if scrollY {
		contentH = max(area.H, e.contentIntrinsic(b, false, area.W))
	}
	if scrollX {
		contentW = max(area.W, e.contentIntrinsic(b, true, area.H))
	}
	b.ContentW, b.ContentH = contentW, contentH
	offX, offY := b.ScrollX, b.ScrollY
	row := b.Direction() == "row"
	if b.Follow >= 0 && b.Follow < len(kids) && !row {
		crossSizes := make([]int, len(kids))
		for i, k := range kids {
			crossSizes[i] = e.crossSize(b, k, row, contentW, -1, false)
		}
		sizes, _ := e.mainSizes(b, kids, row, contentH, crossSizes, scrollY, false)
		top := 0
		for i := 0; i < b.Follow; i++ {
			top += sizes[i] + b.Style.Gap
		}
		bottom := top + sizes[b.Follow]
		if top < offY {
			offY = top
		}
		if bottom > offY+area.H {
			offY = bottom - area.H
		}
	}
	offY = max(0, min(offY, contentH-area.H))
	offX = max(0, min(offX, contentW-area.W))
	b.ScrollX, b.ScrollY = offX, offY
	virtual := Rect{area.X - offX, area.Y - offY, contentW, contentH}
	unbounded := (scrollY && !row) || (scrollX && row)
	e.flex(b, kids, virtual, clip, unbounded)
}

// intrinsic returns b's content-driven border-box size on one axis.
// avail is b's size on the other axis (used to wrap text); 0 if unknown.
func (e *Engine) intrinsic(b *Box, horizontal bool, avail int) int {
	key := memoKey{b, horizontal, avail}
	if v, ok := e.memo[key]; ok {
		return v
	}
	fh, fv := b.frameH(), b.frameV()
	frame := fv
	if horizontal {
		frame = fh
	}
	var v int
	switch b.Kind {
	case "text":
		if horizontal {
			v = MaxWidth(b.Text)
		} else if b.Style.Wrap == "wrap" && avail > 0 {
			v = len(Lines(b.Text, max(1, avail-fh), "wrap"))
		} else {
			v = len(Lines(b.Text, 0, ""))
		}
	case "input":
		if horizontal {
			v = max(Width(b.Text), Width(b.Placeholder)) + 1
		} else {
			v = 1
		}
	case "button":
		if horizontal {
			v = Width(ButtonLabel(b))
		} else {
			v = 1
		}
	case "progress":
		if horizontal {
			v = 10
		} else {
			v = 1
		}
	case "rule":
		if (b.Axis == "y") == horizontal {
			v = 1
		}
	case "spacer":
		v = 0
	default:
		if b.IsContainer() {
			inner := avail - fv
			if !horizontal {
				inner = avail - fh
			}
			v = e.contentIntrinsic(b, horizontal, max(0, inner))
		}
	}
	v += frame
	e.memo[key] = v
	return v
}

// contentIntrinsic measures a container's children (without its own
// border/padding). avail is the content size on the other axis.
func (e *Engine) contentIntrinsic(b *Box, horizontal bool, avail int) int {
	var flow []*Box
	dockMain, dockCross := 0, 0
	for _, c := range b.Children {
		side := c.Style.Dock
		if side == "" {
			flow = append(flow, c)
			continue
		}
		stacks := (side == "top" || side == "bottom") != horizontal
		s := sizeSpec(c, horizontal)
		ms, me := margins(c, horizontal)
		sz := 0
		if s.Kind == ir.Cell {
			sz = cells(s.N)
		} else if s.Kind != ir.Pct {
			sz = e.intrinsic(c, horizontal, avail)
		}
		sz += ms + me
		if stacks {
			dockMain += sz
		} else {
			dockCross = max(dockCross, sz)
		}
	}
	row := b.Direction() == "row"
	alongMain := row == horizontal
	n := len(flow)
	total := 0
	if n > 0 {
		// Sizes on the other axis, needed to measure wrapped text.
		others := make([]int, n)
		if horizontal {
			for i := range flow {
				others[i] = avail
			}
		} else if row {
			sizes, _ := e.mainSizes(b, flow, true, avail, nil, false, false)
			for i, k := range flow {
				ms, me := margins(k, true)
				others[i] = max(0, sizes[i]-ms-me)
			}
		} else {
			for i, k := range flow {
				others[i] = e.crossSize(b, k, false, avail, -1, false)
			}
		}
		for i, k := range flow {
			var s ir.Scalar
			if alongMain {
				s = mainSpec(k, horizontal)
			} else {
				s = sizeSpec(k, horizontal)
			}
			ms, me := margins(k, horizontal)
			var sz int
			switch s.Kind {
			case ir.Cell:
				sz = cells(s.N)
			case ir.Pct:
				sz = 0
			default:
				sz = e.intrinsic(k, horizontal, others[i])
			}
			sz = clampAxis(k, horizontal, sz, -1) + ms + me
			if alongMain {
				total += sz
			} else {
				total = max(total, sz)
			}
		}
		if alongMain {
			total += b.Style.Gap * (n - 1)
		}
	}
	if horizontal {
		// Top/bottom strips span the full width; left/right docks share the
		// row under them with the in-flow content.
		return max(dockCross, total+dockMain)
	}
	return max(total, dockCross) + dockMain
}

// ButtonLabel is the painted button text: "[ label ]" without a border,
// the bare label inside one.
func ButtonLabel(b *Box) string {
	if b.border() == 1 {
		return b.Text
	}
	return "[ " + b.Text + " ]"
}
