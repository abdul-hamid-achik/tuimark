package paint

import "github.com/abdul-hamid-achik/tuimark/internal/layout"

// checkbox paints a checkbox (SPEC v0.4b §12.5): from its content origin,
// its glyph, [x] or [ ], then, when its label is not empty, a space and
// the label cut to max(0, W − 4) columns by its computed wrap, W being its
// content width. Everything is clipped at the content box (so at W = 3
// only the glyph shows) and painted with the node's computed style; its
// content-align is ignored.
func (p *painter) checkbox(b *layout.Box, clip layout.Rect, st cellStyle) {
	c := b.Content
	if c.W <= 0 || c.H <= 0 {
		return
	}
	p.text(clip, c.X, c.Y, layout.CheckboxShown(b, c.W), st)
}

// selectBox paints a select, open or closed (SPEC v0.4b §12.5): with W its
// content width, the shown text cut to max(0, W − 2) columns at the
// content origin, dim when it is the placeholder and in the node's style
// otherwise, then, when W ≥ 1, ▾ at content x + W − 1. Column W − 2 is the
// space between them. Its content-align is ignored.
func (p *painter) selectBox(b *layout.Box, clip layout.Rect, st cellStyle) {
	c := b.Content
	if c.W <= 0 || c.H <= 0 {
		return
	}
	s, dim := layout.SelectShown(b, c.W)
	ts := st
	if dim {
		ts.attrs |= Dim
	}
	p.text(clip, c.X, c.Y, s, ts)
	p.set(clip, c.X+c.W-1, c.Y, '▾', st)
}
