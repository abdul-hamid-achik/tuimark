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
