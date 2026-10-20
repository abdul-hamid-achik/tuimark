package layout

import (
	"fmt"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// The layout defaults of the current language, a <tui> without version
// (SPEC v0.4b §9, §10.6, §11.2-§11.4, §11.5.2; decisions 25-31, 38, 99,
// and 109). Each rule here is read from Engine.Lang, so a document that
// writes version, and the spike root <app>, keep their tuimark 0.4.1
// layout. The built-in sheet's part of them (col and row are 1fr only
// under screen, modal, and tab, and the focus look) is
// css.UASheetCurrent; the fr freeze is frResolve (freeze.go).

// siblingFr reports whether the sibling-fr rule (SPEC §11.2) makes k,
// unsized on its parent's main axis, 1fr when a sibling there is fr. In
// a pinned document it does for every child; in the current language
// only for a viewport (a scroll, a list, a table, or a container with
// overflow: scroll), so the split between a list and an fr pane does not
// depend on the data (ADR 0031, decision 27), and every other unsized
// child stays auto, as in CSS (test 166).
func (e *Engine) siblingFr(k *Box) bool {
	return !e.Lang.V4() || k.Scrolls()
}

// markUnsized records that b has no size on its parent's main axis
// (horizontal for x) from any origin and that the sibling-fr rule did not
// make it 1fr: it is sized by its content there by §11.2 (cutCause).
func (b *Box) markUnsized(horizontal bool) {
	b.unsized |= 1 << axisIndex(horizontal)
}

// unsizedOn reports what markUnsized recorded for one axis.
func (b *Box) unsizedOn(horizontal bool) bool {
	return b.unsized&(1<<axisIndex(horizontal)) != 0
}

// hintsShrinkable reports whether hints h is shrinkable on one axis
// (SPEC v0.4b §6.12, §11.4, decision 29): in the current language on the
// axis its items are placed along (x under layout: row, the default; y
// under layout: column), so it drops the items that do not fit instead of
// being cut by its parent; never in a pinned document, where a hints is a
// leaf for shrinking.
func (e *Engine) hintsShrinkable(h *Box, horizontal bool) bool {
	return e.Lang.V4() && horizontal != hintsColumn(h)
}

// modalContentSize is the size of a modal with no width (horizontal) or
// no height in the current language (SPEC v0.4b §11.3 items 1-3,
// decision 30): its intrinsic size (cross is its size on the other axis),
// lowered to 80% of the root (full) unless a max-width/max-height in
// cells or % is written, which clampAxis applies instead, as it applies
// the min and the root's size after this.
func (e *Engine) modalContentSize(m *Box, horizontal bool, full, cross int) int {
	if horizontal {
		m.autoW = true
	} else {
		m.autoH = true
	}
	v := e.intrinsic(m, horizontal, cross)
	hi := m.Style.MaxH
	if horizontal {
		hi = m.Style.MaxW
	}
	if hi.Kind != ir.Cell && hi.Kind != ir.Pct {
		v = min(v, percent(full, ir.Scalar{Kind: ir.Pct, N: 80}))
	}
	return v
}

// modalTitleWidth is the least intrinsic width of modal m in the current
// language (SPEC v0.4b §11.3 item 2, decision 31): width(title) + 6, the
// width at which §12 paints its title whole, when it paints a border and
// its resolved title is not empty; else 0.
func (e *Engine) modalTitleWidth(m *Box) int {
	if !e.Lang.V4() || m.Kind != "modal" || m.border() == 0 || m.Title == "" {
		return 0
	}
	return clampCells(Width(m.Title) + 6)
}

// inputWidth is the intrinsic content width of input b (SPEC §11.5.2): in
// the current language max(width(placeholder) + 1, 20) whatever its value,
// so typing never moves an auto input, the nodes after it, or a modal
// sized by its content (ADR 0031, decision 99); in a pinned document
// max(width(shown), width(placeholder)) + 1.
func (e *Engine) inputWidth(b *Box) int {
	if e.Lang.V4() {
		return max(Width(b.Placeholder)+1, 20)
	}
	return max(Width(InputShown(b)), Width(b.Placeholder)) + 1
}

// reportPctOfAuto reports L002 on k, whose size s on one axis is a
// percent of parent, sized by its content on that axis (SPEC §11.3). In
// the current language, under a col or a row, the message names the size
// that fixes it (decision 109); under any other parent (a modal sized by
// its content, a box, a docked parent's kind aside) it keeps the 0.4.1
// text.
func (e *Engine) reportPctOfAuto(parent, k *Box, horizontal bool, s ir.Scalar) {
	fix := ""
	if e.Lang.V4() && (parent.Kind == "col" || parent.Kind == "row") {
		fix = fmt.Sprintf(" (%s: 1fr fills it)", axisName(horizontal))
	}
	e.report(k, ir.Error, "L002", "%s: %s is a percent of a parent sized by its content (auto); give the parent a size%s", axisName(horizontal), s, fix)
}

// cutCause is the advice of the L009 message for container n that cuts
// child c on one axis (horizontal for x; SPEC v0.4b §14, decision 109):
// in the current language, when c is an in-flow col, row, or box sized by
// its content there by §11.2 (no size on that axis from any origin, so not
// one the built-in sheet makes 1fr, and not made 1fr by the sibling-fr
// rule), the cause and the CSS fix; otherwise the 0.4.1 advice.
func (e *Engine) cutCause(n, c *Box, horizontal bool) string {
	if e.Lang.V4() && c.Style.Dock == "" && c.unsizedOn(horizontal) {
		switch c.Kind {
		case "col", "row", "box":
			return fmt.Sprintf("%s is sized by its content and does not shrink: give it %s: 1fr (or flex: 1)", clipRef(n, c), axisName(horizontal))
		}
	}
	return "give it room"
}
