package layout

import (
	"fmt"
	"math"
	"sync/atomic"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// Engine lays out one frame and collects L-diagnostics.
type Engine struct {
	File  string
	Diags ir.Diags
	// V3 is set for a version="3" document (SPEC v0.3 §5.1): Layout also
	// reports the clipping warnings L008 and L009 (§14, §30.2).
	V3 bool
	// Lang is the document's language (SPEC v0.4b §5.1): the rules 0.4b
	// changes are read from it, so a pinned document keeps its 0.4.1
	// layout. V3 stays the flag of the version="3" rules, which hold for
	// Lang3 and the current language alike. 0 is read as Lang1.
	Lang ir.Lang
	// memo holds the intrinsic sizes that do not fit a box's own memo
	// slots (Box.memo); gen is this engine's generation (generation).
	memo map[memoKey]int
	gen  uint64
	seen map[string]bool
	// Scratch is the memory this engine's Layout calls cut their work
	// lists from (nil: the engine makes its own on first use).
	Scratch *Scratch
	// l003 and l006 record, for this frame, the boxes L003 and L006 were
	// reported on (SPEC v0.3 §14): L009 does not repeat either.
	l003, l006 map[*Box]bool
	// held counts the boxes being placed that hold their subtree's
	// checks (holdChecks).
	held int
}

type memoKey struct {
	b          *Box
	horizontal bool
	avail      int
}

// Layout places root to fill (at most) cols×rows, then centers each open
// modal over the root. Every placed box gets Laid=true. Viewports that can
// never show part of their content are reported as L006 (SPEC §11.3 pass
// 7), and in a version="3" document cut text and children as L008 and
// L009 (check), each box once it and its subtree are placed (placed),
// so the checks read the final layout.
func (e *Engine) Layout(root *Box, modals []*Box, cols, rows int) {
	if e.memo == nil {
		e.memo = map[memoKey]int{}
		e.seen = map[string]bool{}
	}
	e.scratch().reset()
	nDiags := len(e.Diags)
	var snap checkSnap
	if verifying {
		snap = e.snapChecks()
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
	// place checked root's tree and each modal's (placed).
	if verifying {
		e.verifyChecks(root, modals, nDiags, snap)
	}
}

// check reports L006 (checkViewport) and, in a version="3" document,
// L008 and L009 (checkTextClip, checkChildrenClip; SPEC v0.3 §14, §30.2)
// for every laid-out box under b, in one walk, children first: when L009
// looks at a box's children, L006 has already been decided for each of
// them, as L009 needs (it does not repeat L006, nor L003, which place
// recorded). The diagnostics come out in another order than one walk per
// code would give, which changes nothing: the frame sorts them stably by
// file, line, column, and code, and two that compare equal there come
// from the same source element, whose boxes are never nested in each
// other, so they keep their order.
//
// Layout runs these checks as it places each box (placed), in the same
// order, and runs check itself only for a subtree it could not check
// that way (holdChecks); a tuimark_verify build checks the result
// against this walk over the whole frame (verifyChecks).
func (e *Engine) check(b *Box) {
	for _, c := range b.Children {
		e.check(c)
	}
	e.checkBox(b)
}

// checkBox is check for b alone, its children already checked.
func (e *Engine) checkBox(b *Box) {
	if !b.Laid {
		return
	}
	e.checkViewport(b)
	if e.V3 {
		e.checkTextClip(b)
		e.checkChildrenClip(b)
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
// (intrinsic; cross is the size on the other axis), or, when unsized, 80%
// in a pinned document and its content size capped in the current
// language (modalContentSize).
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
	if e.Lang.V4() {
		// Sized by its content, capped (SPEC v0.4b §11.3).
		return e.modalContentSize(m, horizontal, full, cross)
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
		switch code {
		case "L003":
			if e.l003 == nil {
				e.l003 = map[*Box]bool{}
			}
			e.l003[b] = true
		case "L006":
			if e.l006 == nil {
				e.l006 = map[*Box]bool{}
			}
			e.l006[b] = true
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
		e.placed(b, false)
		return
	}
	childClip := b.Clip.Intersect(b.Content)
	held := false
	switch {
	case b.Kind == "table":
		// Its own layout (SPEC §6.9.3): layout, gap between rows, align,
		// and justify do not apply; its rows come after layout.
		held = e.holdChecks()
		e.tableLayout(b, childClip)
	case b.Kind == "tabs":
		// The label strip and the active tab below it (SPEC §6.10.4).
		held = e.holdChecks()
		e.tabsLayout(b, childClip)
	case b.Kind == "hints":
		// The items that fit, in order (SPEC §6.12).
		held = e.holdChecks()
		e.hintsLayout(b, childClip)
	case b.Scrolls():
		// Also with no children: the offset clamps to 0 and the content
		// extent is the content box (SPEC §13.2, scroll).
		held = e.scrollLayout(b, childClip)
	case len(b.Children) == 0:
	default:
		if b.IsGrid() || !placesInOrder(b.Children) {
			held = e.holdChecks()
		}
		flow := inFlow(b.Children)
		area := e.dock(b, b.Content, childClip)
		e.flex(b, flow, area, childClip)
	}
	e.placed(b, held)
}

// holdChecks makes the boxes placed from now on wait for their checks
// (placed), and reports true: their parent does not place its children
// in their order (placesInOrder is false, it lays them out as a grid, or
// it is a table, a tabs, or a hints, which place theirs their own way),
// so it checks its whole subtree, in order, once it is placed.
func (e *Engine) holdChecks() bool {
	e.held++
	return true
}

// placed runs the checks of box b (check), which place has just placed
// with its whole subtree: when no ancestor of b holds the checks
// (holdChecks), b's own when it did not hold them either (its children
// were checked as each was placed, in their order), or its whole
// subtree's when it did. Every box of a tree is then checked in the
// order check walks it, children first, with what it reads final: a
// box's checks read its own geometry and scroll state, its children's,
// its ancestors' (all set before b's children are placed), and which of
// its children L006 and itself L003 were reported on (both decided by
// then). A list of 20 000 rows is then checked as it is placed, while
// its boxes are still in the cache, instead of in one more walk of it.
func (e *Engine) placed(b *Box, held bool) {
	if held {
		e.held--
		if e.held == 0 {
			e.check(b)
		}
		return
	}
	if e.held == 0 {
		e.checkBox(b)
	}
}

// placesInOrder reports whether a container that is not a grid places
// children kids in their order: dock (carve) places the ones docked to
// the top or the bottom, then the ones docked to the left or the right,
// each in their order, and flex the ones in flow after them, so it does
// when kids are those three groups, in that order (each may be empty).
func placesInOrder(kids []*Box) bool {
	group := 0
	for _, c := range kids {
		g := 1 // as carve: docked, not to the top or the bottom
		switch c.Style.Dock {
		case "":
			g = 2
		case "top", "bottom":
			g = 0
		}
		if g < group {
			return false
		}
		group = g
	}
	return true
}

// inFlow returns the children that are not docked, in order: kids itself
// when none is (flex only reads the list).
func inFlow(kids []*Box) []*Box {
	for i, c := range kids {
		if c.Style.Dock == "" {
			continue
		}
		flow := append([]*Box(nil), kids[:i]...)
		for _, c := range kids[i+1:] {
			if c.Style.Dock == "" {
				flow = append(flow, c)
			}
		}
		return flow
	}
	return kids
}

// columnGap and rowGap are a container's gap between siblings placed side
// by side (x) or stacked (y): in a version="3" document (v3), the
// axis-specific longhand (SPEC v0.3 §10.2, v0.3b); otherwise the plain
// gap, which is what every v0.2b rule already meant. mainGap picks
// between them for a container whose main axis is horizontal (a row
// layout) or vertical (a column layout); table columns, tabs labels, and
// a hint item's keycap+label are always laid out along x, so their
// callers use columnGap directly.
func columnGap(st *css.Style, v3 bool) int {
	if v3 {
		return st.ColumnGap
	}
	return st.Gap
}

func rowGap(st *css.Style, v3 bool) int {
	if v3 {
		return st.RowGap
	}
	return st.Gap
}

func mainGap(st *css.Style, v3, horizontal bool) int {
	if horizontal {
		return columnGap(st, v3)
	}
	return rowGap(st, v3)
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
	return dockConflict(&st, parentRow)
}

// dockConflict is DockConflict without copying the style.
func dockConflict(st *css.Style, parentRow bool) bool {
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
// left/right (SPEC §11.3 pass 1), and places them. It returns the
// remaining area.
func (e *Engine) dock(b *Box, area Rect, clip Rect) Rect {
	return e.carve(b, area, clip, true)
}

// dockSize is the size of docked child d of a container (parentRow when
// it lays out as a row) on the axis it takes from the container
// (horizontal for left/right), margins excluded, and whether its content
// gave it: a cell count, a percent of base (the container's content box
// on that axis), else its intrinsic size with cross as its size on the
// other axis (a size that conflicts with the dock, V010, is auto); then
// clamped by its min and max.
func (e *Engine) dockSize(d *Box, horizontal, parentRow bool, base, cross int) (int, bool) {
	s := sizeSpec(d, horizontal)
	if dockConflict(d.Style, parentRow) {
		s = ir.Scalar{Kind: ir.Auto}
	}
	auto := false
	var size int
	switch s.Kind {
	case ir.Cell:
		size = cells(s.N)
	case ir.Pct:
		size = percent(base, s)
	default:
		size, auto = e.intrinsic(d, horizontal, cross), true
	}
	return clampAxis(d, horizontal, size, base), auto
}

// carve is dock; with place false it only computes the remaining area,
// without placing the docked children or reporting anything.
func (e *Engine) carve(b *Box, area Rect, clip Rect, place bool) Rect {
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
			if place && dockConflict(d.Style, row) {
				e.report(d, ir.Error, "V010", "%s", DockConflictMsg(side))
			}
			ms, me := margins(d, horizontal)
			cs, ce := margins(d, !horizontal)
			full, cross, base := area.H, area.W, baseH
			if horizontal {
				full, cross, base = area.W, area.H, baseW
			}
			size, auto := e.dockSize(d, horizontal, row, base, max(0, cross-cs-ce))
			if auto && place {
				// Sized by its content: a % child on this axis is L002.
				if horizontal {
					d.autoW = true
				} else {
					d.autoH = true
				}
			}
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
			if place {
				e.place(d, r, clip)
			}
		}
	}
	if place && bottoms > 1 {
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
				e.reportPctOfAuto(parent, k, horizontal, s)
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
	sizes = e.ints(n)
	if n == 0 {
		return sizes, false
	}
	horizontal := row
	var specBuf, weightBuf [smallList]ir.Scalar
	specs := e.scalars(&specBuf, n)
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
			if anyFr && e.siblingFr(kids[i]) {
				specs[i] = ir.Scalar{Kind: ir.Fr, N: 1}
			} else {
				specs[i] = ir.Scalar{Kind: ir.Auto}
				if report {
					// Sized by its content by §11.2: L009 names the cause
					// when it cuts this child (cutCause).
					kids[i].markUnsized(horizontal)
				}
			}
		}
	}
	gap := mainGap(b.Style, e.V3, horizontal)
	remain := max(0, main-gap*(n-1))
	parentAuto := b.autoH
	if horizontal {
		parentAuto = b.autoW
	}
	weights := e.scalars(&weightBuf, n)
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
				e.reportPctOfAuto(b, k, horizontal, s)
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
		if e.Lang.V4() {
			// min and max take part in the split (SPEC v0.4b §11.4).
			frResolve(kids, frs, weights, sizes, remain, horizontal, main)
		} else {
			// A pinned document clamps each share after one split, and
			// nothing is shared again (SPEC v0.4b §11.4, §29 item 23).
			for j, take := range frSplit(remain, ws) {
				k := kids[frs[j]]
				ms, me := margins(k, horizontal)
				inner := clampAxis(k, horizontal, max(0, take-ms-me), main)
				sizes[frs[j]] = inner + ms + me
			}
		}
	}
	for i := range sizes {
		if sizes[i] < 0 {
			sizes[i] = 0
		}
	}
	return sizes, anyFr
}

// scratch returns the engine's Scratch, made on first use.
func (e *Engine) scratch() *Scratch {
	if e.Scratch == nil {
		e.Scratch = &Scratch{}
	}
	return e.Scratch
}

// ints returns a zeroed []int of length (and capacity) n for a size list
// of this Layout call, cut from its Scratch, so laying out many
// containers, or one of many children, allocates nothing per container.
func (e *Engine) ints(n int) []int { return e.scratch().ints.take(n) }

// smallList is the length up to which a container's work lists live on
// the stack of the function that builds them (the arrays below), and
// beyond which they are cut from the Scratch.
const smallList = 8

// scalars returns a zeroed []ir.Scalar of length n: buf's first n
// elements when n fits it, else one cut from the Scratch.
func (e *Engine) scalars(buf *[smallList]ir.Scalar, n int) []ir.Scalar {
	if n <= smallList {
		*buf = [smallList]ir.Scalar{}
		return buf[:n:n]
	}
	return e.scratch().scalars.take(n)
}

// boxList returns an empty []*Box with room for n: buf when n fits it,
// else one cut from the Scratch. Appending up to n boxes never moves it.
func (e *Engine) boxList(buf *[smallList]*Box, n int) []*Box {
	if n <= smallList {
		return buf[:0:smallList]
	}
	return e.scratch().boxes.take(n)[:0]
}

// intList is ints for a list only the calling function reads: buf's
// first n elements when n fits it, else one cut from the Scratch.
func (e *Engine) intList(buf *[smallList]int, n int) []int {
	if n <= smallList {
		*buf = [smallList]int{}
		return buf[:n:n]
	}
	return e.ints(n)
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
		crossSizes = e.ints(n)
		for i, k := range kids {
			crossSizes[i] = e.crossSize(b, k, row, cross, -1, !crossUnbounded, report)
		}
	}
	sizes, anyFr = e.mainSizes(b, kids, row, main, crossSizes, mainUnbounded, true, report)
	if row {
		crossSizes = e.ints(n)
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
	e.flexWith(b, kids, area, clip, nil)
}

// allocation is what allocate returns.
type allocation struct {
	sizes, cross []int
	anyFr        bool
}

// flexWith is flex with the allocation of kids in area already made (with
// diagnostics), or nil to make it.
func (e *Engine) flexWith(b *Box, kids []*Box, area Rect, clip Rect, pre *allocation) {
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
	var sizes, crossSizes []int
	var anyFr bool
	if pre != nil {
		sizes, crossSizes, anyFr = pre.sizes, pre.cross, pre.anyFr
	} else {
		sizes, crossSizes, anyFr = e.allocate(b, kids, main, cross, true)
	}
	gap := mainGap(b.Style, e.V3, row)
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
// the scroll axis and is shifted by the (clamped) offset. Docked children
// are part of that content: they are carved out of the content extent,
// as contentIntrinsic counts them, and scroll with it (SPEC §11.3 pass 1);
// the in-flow children are laid out in what is left. It reports whether
// it held the checks of b's subtree (holdChecks), before placing any
// child: b does not place its children in their order (placesInOrder),
// or is a grid.
func (e *Engine) scrollLayout(b *Box, clip Rect) (held bool) {
	area := b.Content
	scrollX, scrollY := b.ScrollAxes()
	var kidBuf [smallList]*Box
	kids := e.boxList(&kidBuf, len(b.Children))
	follow := -1 // b.Follow as an index into kids
	for i, c := range b.Children {
		if c.Style.Dock != "" {
			continue
		}
		if i == b.Follow {
			follow = len(kids)
		}
		kids = append(kids, c)
	}
	if b.IsGrid() || !placesInOrder(b.Children) {
		held = e.holdChecks()
	}
	contentW, contentH := area.W, area.H
	if scrollY {
		ci, ok := e.measuredContentH(b)
		if !ok {
			ci = e.contentIntrinsic(b, false, area.W)
		} else if verifying {
			if v := e.contentIntrinsic(b, false, area.W); v != ci {
				panic(fmt.Sprintf("layout: the measured content height of %s %q is %d, measured again %d", b.Kind, b.ID, ci, v))
			}
		}
		contentH = max(area.H, ci)
	}
	if scrollX {
		contentW = max(area.W, e.contentIntrinsic(b, true, area.H))
	}
	b.ContentW, b.ContentH = contentW, contentH
	offX, offY := b.ScrollX, b.ScrollY
	row := b.Direction() == "row"
	var pre *allocation
	if follow >= 0 && !row {
		// The same allocation flex makes below, in the part of the content
		// extent the docks leave (the same size as the area flex gets).
		// When b stretches its children across (its align is stretch) and
		// does not lay out as a grid (where flex allocates nothing), flex
		// takes it as it is instead of allocating again. Its diagnostics
		// and auto marks then come before the docked children are placed,
		// which changes nothing: they concern b and its in-flow children,
		// the frame sorts diagnostics stably by position and code, and
		// placing the docked children reads neither. Under stretch the
		// cross sizes mark no child auto, so every intrinsic size the
		// allocation measures sees the marks a silent allocation would.
		// Otherwise a cross size marks an auto-width child before its
		// height is measured, which a percent width inside it reads, so
		// this allocation stays silent and flex makes its own.
		flowArea := e.carve(b, Rect{0, 0, contentW, contentH}, Rect{}, false)
		reuse := !b.IsGrid() && isStretch(b.Style.Align)
		sizes, cross, anyFr := e.allocate(b, kids, flowArea.H, flowArea.W, reuse)
		if reuse {
			pre = &allocation{sizes, cross, anyFr}
		}
		top := flowArea.Y
		g := mainGap(b.Style, e.V3, false)
		for i := 0; i < follow; i++ {
			top += sizes[i] + g
		}
		bottom := top + sizes[follow]
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
	e.flexWith(b, kids, e.dock(b, virtual, clip), clip, pre)
	return held
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
	if b.Kind == "hints" {
		// A hints is a leaf for shrinking in a pinned document (its items
		// never shrink, SPEC §6.12), and shrinkable along its items' axis
		// in the current language (v0.4b).
		return e.hintsShrinkable(b, horizontal)
	}
	if !b.IsContainer() {
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
	e.memoOf(b)
	shift := 2 * axisIndex(horizontal)
	if b.shrink>>shift&1 != 0 {
		return b.shrink>>shift&2 != 0
	}
	v := false
	for _, c := range b.Children {
		if e.shrinkable(c, horizontal) {
			v = true
			break
		}
	}
	bits := uint8(1)
	if v {
		bits = 3
	}
	b.shrink |= bits << shift
	return v
}

// checkViewport reports L006 (SPEC §11.4) once for laid-out viewport V
// when it can never show part of its content on one of its scroll axes A
// (y first): (a) its content box is 0 cells on A while its content is not
// empty there, or (b) its outer rect sticks out, on A, of the content box
// of an ancestor below the nearest ancestor that scrolls on A. Clipping by
// an ancestor that scrolls on A is by design and never reported.
func (e *Engine) checkViewport(v *Box) {
	sx, sy := v.ScrollAxes()
	if sy {
		if msg := e.neverShown(v, false); msg != "" {
			e.report(v, ir.Warning, "L006", "%s", msg)
			return
		}
	}
	if sx {
		if msg := e.neverShown(v, true); msg != "" {
			e.report(v, ir.Warning, "L006", "%s", msg)
		}
	}
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

// clipRef names a child for the L009 message (SPEC v0.3 §14): #id, else
// its tag and a 0-based index among its Laid siblings sharing that tag
// (omitted when it is the only one), the way `tuimark inspect` names an
// unidentified node.
func clipRef(c *Box) string {
	if c.ID != "" {
		return "#" + c.ID
	}
	if c.Parent == nil {
		return c.Tag
	}
	k, count := 0, 0
	for _, s := range c.Parent.Children {
		if !s.Laid || s.Tag != c.Tag {
			continue
		}
		if s == c {
			k = count
		}
		count++
	}
	if count > 1 {
		return fmt.Sprintf("%s[%d]", c.Tag, k)
	}
	return c.Tag
}

// checkTextClip reports L008: a painted text (not visibility: hidden, not
// a generated node — a table cell, tab label, or hint item, all Fixed,
// SPEC §6.9.4, §6.10.4, §6.12) that loses content without saying so.
// Case (a) loses columns: its computed wrap is nowrap (the initial value,
// "", or the literal "nowrap"), that is not document-written, and its
// widest line does not fit its content box. Case (b) loses lines: its
// lines after wrapping and truncation outnumber its content box's rows,
// and overflow: hidden is not document-written on it. Either, both, or
// neither may apply; each has its own message and silencing rule, and
// both mark Clipped "text" (SPEC §14, ADR 0015 Enmienda).
func (e *Engine) checkTextClip(n *Box) {
	if n.Kind != "text" || n.Fixed || n.Style.Visibility == "hidden" {
		return
	}
	mode := n.Style.Wrap
	// truncate-start and truncate-middle (version="3", v0.3b) mark their
	// cut with an ellipsis exactly as truncate does, so they never
	// produce L008a either (SPEC §11.5.2).
	nowrap := mode != "wrap" && mode != "truncate" && mode != "truncate-start" && mode != "truncate-middle"
	if nowrap && !n.Style.WrapWritten {
		if w, c := n.textInfo().w, n.Content.W; w > c {
			if e.clipFirst(n, "L008a") {
				e.report(n, ir.Warning, "L008", "text is %d columns wide but its content box has %d; set wrap: truncate, or write wrap: nowrap to keep the cut", w, c)
			}
			n.Clipped = "text"
		}
	}
	if !n.Style.OverflowWritten {
		lines := n.lineCount(n.Content.W, mode)
		if r := n.Content.H; lines > r {
			if e.clipFirst(n, "L008b") {
				e.report(n, ir.Warning, "L008", "text has %d lines but its content box has %d rows; give it room, or write overflow: hidden to keep the cut", lines, r)
			}
			n.Clipped = "text"
		}
	}
}

// checkChildrenClip reports L009: a container (n) that cuts a child, in
// flow or docked, on an axis it does not itself scroll. Not reported for
// a list, a table, or a hints (they show part of their content by
// design), for a node L003 already reported this frame (its own message
// already explains the cut), or when overflow: hidden is document-written
// on n. A child with visibility: hidden or an empty rect does not count,
// and a child L006 already reported this frame is skipped (its own
// warning explains it), leaving the next child in document order a
// candidate. CHILD in the message is the first child so cut.
func (e *Engine) checkChildrenClip(n *Box) {
	if n.Style.OverflowWritten {
		return
	}
	switch n.Kind {
	case "list", "table", "hints":
		return
	}
	if e.l003 != nil && e.l003[n] {
		return
	}
	sx, sy := n.ScrollAxes()
	for _, c := range n.Children {
		if !c.Laid || c.W <= 0 || c.H <= 0 || c.Style.Visibility == "hidden" {
			continue
		}
		if e.l006 != nil && e.l006[c] {
			continue
		}
		if !sx {
			lo, hi := n.Content.X, n.Content.X+n.Content.W
			if c.X < lo || c.X+c.W > hi {
				e.reportClip(n, c, "x", max(0, min(c.X+c.W, hi)-max(c.X, lo)), c.W)
				return
			}
		}
		if !sy {
			lo, hi := n.Content.Y, n.Content.Y+n.Content.H
			if c.Y < lo || c.Y+c.H > hi {
				e.reportClip(n, c, "y", max(0, min(c.Y+c.H, hi)-max(c.Y, lo)), c.H)
				return
			}
		}
	}
}

// reportClip reports L009 on n for its child c cut on axis ("x" or "y"),
// shown of total cells, and marks n Clipped "children".
func (e *Engine) reportClip(n, c *Box, axis string, shown, total int) {
	if e.clipFirst(n, "L009") {
		e.report(n, ir.Warning, "L009", "cuts %s on %s: %d of %d cells shown; %s, or write overflow: hidden to keep the cut", clipRef(c), axis, shown, total, e.cutCause(c, axis == "x"))
	}
	n.Clipped = "children"
}

// clipFirst reports whether n is the first node of its source element to
// meet this clipping condition in the frame (kind is "L008a", "L008b", or
// "L009"). The rows of one template share a source element and give one
// diagnostic whatever their numbers (SPEC v0.3 §14), while each of them
// still gets Clipped. report's own dedup keys on the message, which
// differs from row to row, so it cannot do this.
func (e *Engine) clipFirst(n *Box, kind string) bool {
	path, line, col := "", 0, 0
	if n.Src != nil {
		path, line, col = n.Src.Path, n.Src.Line, n.Src.Col
	}
	key := fmt.Sprintf("clip|%s|%s|%d|%d|%s", kind, path, line, col, n.ID)
	if e.seen[key] {
		return false
	}
	e.seen[key] = true
	return true
}

// intrinsic returns b's content-driven border-box size on one axis.
// avail is b's size on the other axis (used to wrap text); 0 if unknown.
func (e *Engine) intrinsic(b *Box, horizontal bool, avail int) int {
	if v, ok := e.memoGet(b, horizontal, avail); ok {
		return v
	}
	v := e.measure(b, horizontal, avail)
	e.memoPut(b, horizontal, avail, v)
	return v
}

// engineGen hands out the generations of engine runs (Engine.gen).
var engineGen atomic.Uint64

// generation returns this engine's generation: the stamp that makes its
// memo on a box (Box.memoGen) valid for it alone, as its maps would be.
func (e *Engine) generation() uint64 {
	if e.gen == 0 {
		e.gen = engineGen.Add(1)
	}
	return e.gen
}

// memoOf returns b's memo for this engine run, cleared when b holds
// another run's.
func (e *Engine) memoOf(b *Box) {
	if g := e.generation(); b.memoGen != g {
		b.memoGen, b.memoSet, b.shrink = g, 0, 0
	}
}

// fits32 reports whether v fits a memo slot.
func fits32(v int) bool { return v >= math.MinInt32 && v <= math.MaxInt32 }

func axisIndex(horizontal bool) int {
	if horizontal {
		return 1
	}
	return 0
}

// memoGet returns the intrinsic size this engine run computed for b on
// one axis at avail, if any: the first value computed for that key, as a
// map keyed by (b, horizontal, avail) holds it.
func (e *Engine) memoGet(b *Box, horizontal bool, avail int) (int, bool) {
	e.memoOf(b)
	if fits32(avail) {
		ax := axisIndex(horizontal)
		for i, s := range &b.memo[ax] {
			if b.memoSet&(1<<(2*ax+i)) != 0 && int(s.avail) == avail {
				return int(s.v), true
			}
		}
	}
	v, ok := e.memo[memoKey{b, horizontal, avail}]
	return v, ok
}

// pureContentV is a bit of Box.memoSet, above the slot bits:
// contentIntrinsic sets it when it measures b down (horizontal false)
// and that measure reads none of b's auto marks (autoW, autoH), which
// layout sets between two measures of b: b does not lay out as a grid
// and none of its in-flow children has a percent width (crossSize and
// mainSizes read the marks for those alone). Everything else such a
// measure reads is b's and its children's styles, which do not change
// during a run, and the intrinsic sizes this run already remembered,
// which it reads again, so measuring b down again at the same width
// gives the same value.
const pureContentV = 1 << 4

// measuredContentH returns contentIntrinsic(b, false, b.Content.W) when
// this engine run has already measured it and measuring it again gives
// the same value (pureContentV): b's parent measures b's height at b's
// width (intrinsic(b, false, b.W)), which is measure's
// contentIntrinsic(b, false, max(0, b.W-frameH)) plus frameV, and
// max(0, b.W-frameH) is b.Content.W. A list of 20 000 rows is then
// walked once to measure its content, not twice.
func (e *Engine) measuredContentH(b *Box) (int, bool) {
	e.memoOf(b)
	if b.memoSet&pureContentV == 0 {
		return 0, false
	}
	v, ok := e.memoGet(b, false, b.W)
	if !ok {
		return 0, false
	}
	return v - b.frameV(), true
}

// memoPut remembers v for (b, horizontal, avail): in a free slot of b
// when both fit one, else in the engine's map.
func (e *Engine) memoPut(b *Box, horizontal bool, avail, v int) {
	if fits32(avail) && fits32(v) {
		ax := axisIndex(horizontal)
		for i := range b.memo[ax] {
			if bit := uint8(1) << (2*ax + i); b.memoSet&bit == 0 {
				b.memo[ax][i] = memoSlot{avail: int32(avail), v: int32(v)}
				b.memoSet |= bit
				return
			}
		}
	}
	if e.memo == nil {
		e.memo = map[memoKey]int{}
	}
	e.memo[memoKey{b, horizontal, avail}] = v
}

// measure computes intrinsic's value.
func (e *Engine) measure(b *Box, horizontal bool, avail int) int {
	fh, fv := b.frameH(), b.frameV()
	frame := fv
	if horizontal {
		frame = fh
	}
	var v int
	switch b.Kind {
	case "text":
		if horizontal {
			v = b.textInfo().w
		} else if b.Style.Wrap == "wrap" && avail > 0 {
			v = b.lineCount(max(1, avail-fh), "wrap")
		} else {
			v = b.textInfo().paras
		}
	case "input":
		if horizontal {
			v = e.inputWidth(b)
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
			if b.IsSpinner() {
				v = 1 // SPEC v0.4b §11.5: a spinner is one cell until given a size
			}
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
		v = tableIntrinsic(b, horizontal, e.V3)
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
		v = hintsIntrinsic(b, horizontal, e.V3)
	default:
		if b.IsContainer() {
			inner := avail - fv
			if !horizontal {
				inner = avail - fh
			}
			v = e.contentIntrinsic(b, horizontal, max(0, inner))
		}
	}
	if horizontal {
		// A modal's title counts in the current language (SPEC v0.4b
		// §11.3 item 2): 0 otherwise.
		return max(v+frame, e.modalTitleWidth(b))
	}
	return v + frame
}

// contentIntrinsic measures a container's children (without its own
// border/padding). avail is the content size on the other axis.
//
// Measured down (horizontal false), avail is the content width, and the
// in-flow children, a grid's included, are measured at the width the
// left and right docks leave, carved as dock carves them (SPEC §11.3
// pass 1, §11.7): wrapped text and a grid's columns get the width they
// are laid out at. A left or right dock's own height is measured at its
// own width.
func (e *Engine) contentIntrinsic(b *Box, horizontal bool, avail int) int {
	var flowBuf [smallList]*Box
	flow := e.boxList(&flowBuf, len(b.Children))
	dockMain, dockCross := 0, 0
	row := b.Direction() == "row"
	flowAvail := avail
	for _, c := range b.Children {
		side := c.Style.Dock
		if side == "" {
			flow = append(flow, c)
			continue
		}
		stacks := (side == "top" || side == "bottom") != horizontal
		s := sizeSpec(c, horizontal)
		ms, me := margins(c, horizontal)
		other := avail
		if !horizontal && (side == "left" || side == "right") {
			w, _ := e.dockSize(c, true, row, avail, 0)
			ws, we := margins(c, true)
			slot := min(w+ws+we, flowAvail)
			flowAvail -= slot
			other = max(0, slot-ws-we)
		}
		sz := 0
		if s.Kind == ir.Cell {
			sz = cells(s.N)
		} else if s.Kind != ir.Pct {
			sz = e.intrinsic(c, horizontal, other)
		}
		sz += ms + me
		if stacks {
			dockMain += sz
		} else {
			dockCross = max(dockCross, sz)
		}
	}
	alongMain := row == horizontal
	n := len(flow)
	total := 0
	grid := b.IsGrid()
	pctW := false // an in-flow child has a percent width (pureContentV)
	if grid {
		// SPEC §11.7 item 6: the grid's own measure of its in-flow
		// children; docked children stack around it as in any container.
		total = e.gridIntrinsic(b, flow, horizontal, flowAvail)
		n = 0
	}
	if n > 0 {
		// Sizes on the other axis, needed to measure wrapped text.
		var otherBuf [smallList]int
		others := e.intList(&otherBuf, n)
		if horizontal {
			for i := range flow {
				others[i] = flowAvail
			}
		} else if row {
			sizes, _ := e.mainSizes(b, flow, true, flowAvail, nil, false, false, false)
			for i, k := range flow {
				ms, me := margins(k, true)
				others[i] = max(0, sizes[i]-ms-me)
			}
		} else {
			for i, k := range flow {
				others[i] = e.crossSize(b, k, false, flowAvail, -1, false, false)
			}
		}
		for i, k := range flow {
			if !horizontal && k.Style.Width.Kind == ir.Pct {
				pctW = true
			}
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
			total += mainGap(b.Style, e.V3, row) * (n - 1)
		}
	}
	if horizontal {
		// Top/bottom strips span the full width; left/right docks share the
		// row under them with the in-flow content.
		return max(dockCross, total+dockMain)
	}
	if !grid && !pctW {
		e.memoOf(b)
		b.memoSet |= pureContentV
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
