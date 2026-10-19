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
