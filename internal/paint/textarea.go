package paint

import "github.com/abdul-hamid-achik/tuimark/internal/layout"

// textarea paints a textarea (SPEC v0.4d §12.8, §6.19.5): its visual lines
// from its offset on fill the content box, one per row, from the content
// origin, each cut at the content box (the spaces that hang past the width
// are not painted); the placeholder is painted dim at the content origin
// when the value is empty. No caret glyph is painted: while an enabled
// textarea has focus the terminal's cursor goes on the caret's cell
// (Grid.CursorOn), unless the caret's visual line is outside the content
// box, where the cursor is hidden (decisions 236 and 237).
func (p *painter) textarea(b *layout.Box, clip layout.Rect, st cellStyle) {
	c := b.Content
	if c.W <= 0 || c.H <= 0 {
		return
	}
	if b.Text == "" {
		if b.Placeholder != "" {
			ph := st
			ph.attrs |= Dim
			p.text(clip, c.X, c.Y, layout.Cut(b.Placeholder, c.W), ph)
		}
	} else {
		lines := layout.TextareaLines(b.Text, c.W)
		for row := 0; row < c.H; row++ {
			i := b.ScrollY + row
			if i < 0 || i >= len(lines) {
				if i >= len(lines) {
					break
				}
				continue
			}
			ln := lines[i]
			p.text(clip, c.X, c.Y+row, b.Text[ln.Start:ln.End], st)
		}
	}
	if b.Focused && !b.Disabled {
		if cc := layout.CaretOf(b); cc.Shown && clip.Contains(cc.X, cc.Y) {
			p.g.CursorX, p.g.CursorY, p.g.CursorOn = cc.X, cc.Y, true
		}
	}
}
