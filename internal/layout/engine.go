package layout

import (
	"fmt"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// Engine lays out one frame and collects L-diagnostics.
type Engine struct {
	File   string
	Diags  ir.Diags
	memo   map[memoKey]int
	shrink map[axisKey]bool
	seen   map[string]bool
}

type memoKey struct {
	b          *Box
	horizontal bool
	avail      int
}

type axisKey struct {
	b          *Box
	horizontal bool
}

// Layout places root to fill (at most) cols×rows, then centers each open
// modal over the root. Every placed box gets Laid=true. Viewports that can
// never show part of their content are reported last, as L006 (SPEC §11.3
// pass 7).
func (e *Engine) Layout(root *Box, modals []*Box, cols, rows int) {
	if e.memo == nil {
		e.memo = map[memoKey]int{}
		e.seen = map[string]bool{}
	}
	if e.shrink == nil {
		e.shrink = map[axisKey]bool{}
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
	e.checkViewports(root)
	for _, m := range modals {
		e.checkViewports(m)
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
		X: b.X + bw + b.pad(3) + b.Chan,
		Y: b.Y + bw + b.pad(0),
		W: max(0, b.W-b.frameH()),
		H: max(0, b.H-b.frameV()),
	}
	if !b.IsContainer() {
		return
	}
	childClip := b.Clip.Intersect(b.Content)
	switch {
	case b.Kind == "table":
		// Its own layout (SPEC §6.9.3): layout, gap between rows, align,
		// and justify do not apply; its rows come after layout.
		e.tableLayout(b, childClip)
	case b.Kind == "tabs":
		// The label strip and the active tab below it (SPEC §6.10.4).
		e.tabsLayout(b, childClip)
	case b.Kind == "hints":
		// The items that fit, in order (SPEC §6.12).
		e.hintsLayout(b, childClip)
	case b.Scrolls():
		// Also with no children: the offset clamps to 0 and the content
		// extent is the content box (SPEC §13.2, scroll).
		e.scrollLayout(b, childClip)
	case len(b.Children) == 0:
	default:
		area := e.dock(b, b.Content, childClip)
		var flow []*Box
		for _, c := range b.Children {
			if c.Style.Dock == "" {
				flow = append(flow, c)
			}
		}
		e.flex(b, flow, area, childClip)
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

// authorFr returns the declaration the document wrote that gives b an fr
// weight along the parent's main axis, for a message ("flex: N" or
// "height: Nfr"), or "" when b has none: its weight, if any, comes from
// the built-in sheet (col/row 1fr, spacer flex: 1). Only sizes the
// document wrote count, as for V010 (DockConflict).
func authorFr(b *Box, horizontal bool) string {
	if b.Style.FlexSet && b.Style.Flex > 0 && !b.Style.FlexUA {
		return "flex: " + b.Style.FlexLit
	}
	s, ua := b.Style.Height, b.Style.HeightUA
	if horizontal {
		s, ua = b.Style.Width, b.Style.WidthUA
	}
	if s.Kind == ir.Fr && !ua {
		return axisName(horizontal) + ": " + s.String()
	}
	return ""
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
// fit is set when the cross allocation is bounded (the parent does not
// scroll on its cross axis) and this is layout, not measuring: then an auto
// child that is shrinkable on the cross axis is capped to the available
// cross size under a non-stretch align (SPEC §11.4 auto-fit rule 4).
func (e *Engine) crossSize(parent, k *Box, row bool, cross, mainSize int, fit, report bool) int {
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
			if fit && e.shrinkable(k, horizontal) {
				v = min(v, avail)
			}
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
//
// unbounded is set when b is a viewport distributing its content along one
// of its own scroll axes: children keep their natural size there. fit
// turns on auto-fit for bounded allocations (layout; measuring passes
// false and stays exactly v0.1): an auto child that is shrinkable on this
// axis is deferred until fixed cells, percents, and the other auto
// children are served, and then takes its intrinsic size capped by what is
// left, in document order, before the fr split.
func (e *Engine) mainSizes(b *Box, kids []*Box, row bool, main int, crossSizes []int, unbounded, fit, report bool) (sizes []int, anyFr bool) {
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
				// L001 (SPEC v0.2 §11.3): no leftover to share along a
				// viewport's own scroll axis, so the child is auto there.
				// A weight from the built-in sheet (col/row 1fr, spacer
				// flex: 1) is not reported, as for V010: the document
				// never wrote it, and <scroll><col>…</col></scroll> is
				// valid.
				if decl := authorFr(k, horizontal); report && decl != "" {
					e.report(k, ir.Error, "L001", "%s inside a scrolling %s has no leftover to share; use cells or auto", decl, b.Kind)
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
	// Auto-fit (SPEC §11.4): shrinkable auto children, in document order,
	// with their intrinsic sizes.
	var deferred, deferredIntr []int
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
			// Sized by its content, capped or not: still auto for L002.
			if report {
				if horizontal {
					k.autoW = true
				} else {
					k.autoH = true
				}
			}
			if fit && !unbounded && e.shrinkable(k, horizontal) {
				deferred = append(deferred, i)
				deferredIntr = append(deferredIntr, take)
				if hasMin(k, horizontal) {
					// Only the min cannot shrink.
					fixedSum += clampAxis(k, horizontal, 0, main) + ms + me
					fixedOrMin = true
				}
				continue
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
	// Deferred children: content size capped by what is left, then min/max
	// (a min larger than what is left overflows, as in v0.1). The first may
	// leave 0 to the next; sharing space is what fr is for.
	for j, i := range deferred {
		k := kids[i]
		ms, me := margins(k, horizontal)
		size := clampAxis(k, horizontal, min(deferredIntr[j], max(0, remain-ms-me)), main)
		sizes[i] = size + ms + me
		remain = max(0, remain-sizes[i])
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

// allocate sizes the in-flow children of b in a main×cross area (SPEC
// §11.4): the slots along b's main axis and the border-box sizes across it.
// An axis on which b itself scrolls is unbounded; the others are bounded
// and auto-fit applies there.
func (e *Engine) allocate(b *Box, kids []*Box, main, cross int, report bool) (sizes, crossSizes []int, anyFr bool) {
	n := len(kids)
	row := b.Direction() == "row"
	mainUnbounded, crossUnbounded := b.scrollsOn(row), b.scrollsOn(!row)
	if !row {
		crossSizes = make([]int, n)
		for i, k := range kids {
			crossSizes[i] = e.crossSize(b, k, row, cross, -1, !crossUnbounded, report)
		}
	}
	sizes, anyFr = e.mainSizes(b, kids, row, main, crossSizes, mainUnbounded, true, report)
	if row {
		crossSizes = make([]int, n)
		for i, k := range kids {
			ms, me := margins(k, true)
			crossSizes[i] = e.crossSize(b, k, row, cross, max(0, sizes[i]-ms-me), !crossUnbounded, report)
		}
	}
	return sizes, crossSizes, anyFr
}

// flex allocates and places in-flow children of b inside area; a grid
// container places them with the grid of SPEC §11.7 instead.
func (e *Engine) flex(b *Box, kids []*Box, area Rect, clip Rect) {
	n := len(kids)
	if n == 0 {
		return
	}
	if b.IsGrid() {
		e.grid(b, kids, area, clip)
		return
	}
	row := b.Direction() == "row"
	main, cross := area.H, area.W
	if row {
		main, cross = area.W, area.H
	}
	sizes, crossSizes, anyFr := e.allocate(b, kids, main, cross, true)
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
	scrollX, scrollY := b.ScrollAxes()
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
		// The same allocation flex makes below, without diagnostics.
		sizes, _, _ := e.allocate(b, kids, contentH, contentW, false)
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
	e.flex(b, kids, virtual, clip)
}

// ScrollAxes reports the axes b scrolls on (SPEC §11.4): for a scroll, its
// axis= attribute (y by default, x, or both); y for a list and for an
// overflow: scroll container, which cannot take axis= (it is V002 there).
// A value the parser rejected (V003) scrolls on y, the default, so every
// viewport scrolls on at least one axis. Boxes that are not viewports
// scroll on neither.
func (b *Box) ScrollAxes() (x, y bool) {
	if !b.Scrolls() {
		return false, false
	}
	if b.Kind == "scroll" {
		switch b.Axis {
		case "x":
			return true, false
		case "both":
			return true, true
		}
	}
	return false, true
}

// scrollsOn reports whether b scrolls on the horizontal (x) or vertical (y)
// axis.
func (b *Box) scrollsOn(horizontal bool) bool {
	x, y := b.ScrollAxes()
	if horizontal {
		return x
	}
	return y
}

// shrinkable reports whether b is shrinkable on one axis (SPEC §11.4): a
// viewport on its scroll axes, and any container, a viewport included on
// its other axes, holding a child that is (in flow or docked; modals are
// never children). Leaves never are. Computed bottom-up once per frame.
func (e *Engine) shrinkable(b *Box, horizontal bool) bool {
	if !b.IsContainer() || b.Kind == "hints" {
		// A hints is a leaf for shrinking: its items never shrink (SPEC
		// §6.12).
		return false
	}
	if b.scrollsOn(horizontal) {
		return true
	}
	if b.IsGrid() {
		// A grid container is shrinkable only on the scroll axes of a
		// viewport (SPEC §11.4, §11.7 item 7): its row heights come from
		// intrinsic sizes.
		return false
	}
	key := axisKey{b, horizontal}
	if v, ok := e.shrink[key]; ok {
		return v
	}
	v := false
	for _, c := range b.Children {
		if e.shrinkable(c, horizontal) {
			v = true
			break
		}
	}
	e.shrink[key] = v
	return v
}

// checkViewports reports L006 (SPEC §11.4) once per laid-out viewport V
// under b that can never show part of its content on one of its scroll
// axes A: (a) its content box is 0 cells on A while its content is not
// empty there, or (b) its outer rect sticks out, on A, of the content box
// of an ancestor below the nearest ancestor that scrolls on A. Clipping by
// an ancestor that scrolls on A is by design and never reported.
func (e *Engine) checkViewports(b *Box) {
	b.Walk(func(v *Box) {
		if !v.Laid {
			return
		}
		sx, sy := v.ScrollAxes()
		for _, horizontal := range []bool{false, true} {
			if (horizontal && !sx) || (!horizontal && !sy) {
				continue
			}
			if msg := e.neverShown(v, horizontal); msg != "" {
				e.report(v, ir.Warning, "L006", "%s", msg)
				return
			}
		}
	})
}

// neverShown returns the L006 message for viewport v on one axis, or "".
func (e *Engine) neverShown(v *Box, horizontal bool) string {
	unit, size, axis := "rows", "height", "y"
	pos, length := v.Y, v.H
	content, natural := v.Content.H, v.ContentH
	if v.Kind == "table" {
		// A table's viewport is its body, its content its n rows (SPEC
		// §6.9.3).
		content, natural = v.View, v.Rows
	}
	if horizontal {
		unit, size, axis = "columns", "width", "x"
		pos, length = v.X, v.W
		content, natural = v.Content.W, v.ContentW
	}
	// scrollLayout sets the extent to max(content box, natural content), so
	// with a 0-cell content box it is the natural content extent.
	if content == 0 && natural > 0 {
		return fmt.Sprintf("%s gets 0 %s on its scroll axis but has content there, so it can never show it; give it a cells, %%, or fr %s, or more room in its parent", v.Kind, unit, size)
	}
	for p := v.Parent; p != nil && !p.scrollsOn(horizontal); p = p.Parent {
		lo, hi := p.Content.Y, p.Content.Y+p.Content.H
		if horizontal {
			lo, hi = p.Content.X, p.Content.X+p.Content.W
		}
		if pos < lo || pos+length > hi {
			return fmt.Sprintf("%s is clipped on %s by %s, which does not scroll, so part of its content can never be shown; give it a cells, %%, or fr %s, or more room in %s", v.Kind, axis, describe(p), size, describe(p))
		}
	}
	return ""
}

// describe names a box for a message: its tag, plus #id when it has one.
func describe(b *Box) string {
	if b.ID != "" {
		return b.Tag + "#" + b.ID
	}
	return b.Tag
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
			v = max(Width(InputShown(b)), Width(b.Placeholder)) + 1
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
	case "table":
		v = tableIntrinsic(b, horizontal)
	case "sparkline":
		// SPEC §6.11: the array's length across (0 when it is not an
		// array), one row down.
		if horizontal {
			v = clampCells(len(b.Series))
		} else {
			v = 1
		}
	case "tabs":
		inner := avail - fv
		if !horizontal {
			inner = avail - fh
		}
		v = e.tabsIntrinsic(b, horizontal, max(0, inner))
	case "hints":
		v = hintsIntrinsic(b, horizontal)
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
	if b.IsGrid() {
		// SPEC §11.7 item 6: the grid's own measure of its in-flow
		// children; docked children stack around it as in any container.
		total = e.gridIntrinsic(b, flow, horizontal, avail)
		n = 0
	}
	if n > 0 {
		// Sizes on the other axis, needed to measure wrapped text.
		others := make([]int, n)
		if horizontal {
			for i := range flow {
				others[i] = avail
			}
		} else if row {
			sizes, _ := e.mainSizes(b, flow, true, avail, nil, false, false, false)
			for i, k := range flow {
				ms, me := margins(k, true)
				others[i] = max(0, sizes[i]-ms-me)
			}
		} else {
			for i, k := range flow {
				others[i] = e.crossSize(b, k, false, avail, -1, false, false)
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
