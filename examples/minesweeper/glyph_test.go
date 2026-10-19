package main

import "testing"

// Nothing on the board or in the panel is wider than one column per
// character: the faces and glyphs are text, not emoji (which are two
// columns wide and would shift the cells), in every look.
func TestEverythingIsOneColumnPerCharacter(t *testing.T) {
	for _, g := range []string{glyphHidden, glyphFlag, glyphQuestion, glyphMine, glyphWrong, glyphEmpty} {
		if n := len([]rune(g)); n != 2 {
			t.Errorf("cell glyph %q is %d characters, want 2", g, n)
		}
	}
	// The faces are ASCII, two one-column characters: the text faces such as
	// U+263A are emoji code points that a font can draw two columns wide.
	seen := map[string]bool{}
	for _, state := range []string{"ready", "playing", "paused", "lost", "won"} {
		f := faceOf(state)
		if len(f) != 2 {
			t.Errorf("face for %s is %q: want two characters", state, f)
		}
		for _, c := range f {
			if c < 0x20 || c > 0x7e {
				t.Errorf("face for %s is %q: %q is not printable ASCII", state, f, c)
			}
		}
		seen[f] = true
	}
	if len(seen) != 4 { // ready and playing share one
		t.Errorf("the faces %v do not tell lost, won, paused, and playing apart", seen)
	}
	for _, b := range layoutBoards[:1] {
		for _, look := range layoutLooks {
			r := newRigOn(t, b, 5)
			r.arrange(look)
			res := r.playAt(100, 30)
			if res.Dump.Wide {
				t.Errorf("%s %s: a cluster is wider than one column:\n%s", b.level, look, gridOf(res))
			}
		}
	}
}
