package paint

// The spinner's glyph sets (SPEC v0.4b §12.6). Each glyph is a simple
// cluster of width 1.
var (
	// spinBraille is the set every tool paints, and Run under every color
	// profile but 16.
	spinBraille = []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}
	// spinASCII is the set of Run under the 16 profile (SPEC §26.3).
	spinASCII = []rune{'|', '/', '-', '\\'}
)

// Glyph returns the spinner glyph of phase P: S[P mod len(S)] for the
// braille set, or the ASCII set when ascii is set (SPEC v0.4b §12.6). A
// negative phase counts as 0.
func Glyph(phase int, ascii bool) rune {
	set := spinBraille
	if ascii {
		set = spinASCII
	}
	return set[max(phase, 0)%len(set)]
}

// SpinnerGlyphs returns a copy of g whose visible spinners (Grid.Spinners)
// hold the glyph of phase p, from the ASCII set when ascii is set (SPEC
// v0.4b §12.6, §26.13). g is not changed: it may be the frame the app
// keeps, or one another goroutine reads. Only the glyph of those cells
// depends on the phase, so the copy is the grid a fresh build of the same
// frame at phase p would paint.
func (g *Grid) SpinnerGlyphs(p int, ascii bool) *Grid {
	c := g.Clone()
	ch := Glyph(p, ascii)
	for _, s := range g.Spinners {
		if i := s.Y*g.W + s.X; i >= 0 && i < len(c.Cells) {
			c.Cells[i].Ch = ch
		}
	}
	return c
}
