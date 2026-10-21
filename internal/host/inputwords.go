package host

import "unicode"

// The word moves of a focused input (SPEC v0.4d §6.19.3, §8.6 step 2,
// decisions 217 and 223): alt+b and alt+f, alt+backspace, and left and
// right held with alt or ctrl, which the decoder marks (Key.Word) and which
// act as alt+b and alt+f. An input is one line, so the definitions of a
// textarea's words apply to it without line breaks: a word is a sequence of
// characters that are not space characters (unicode.IsSpace), judged on
// each grapheme cluster by its first code point.

// inputKeyName is the key an input acts on: left and right marked with alt
// or ctrl act as alt+b and alt+f. Only the decoder of the current language
// sets the mark, so a pinned document never sees it.
func inputKeyName(k Key) string {
	if k.Word {
		switch k.Name {
		case "left":
			return "alt+b"
		case "right":
			return "alt+f"
		}
	}
	return k.Name
}

// spaceCluster reports whether the cluster between bounds[i] and
// bounds[i+1] is a space character.
func spaceCluster(runes []rune, bounds []int, i int) bool {
	return unicode.IsSpace(runes[bounds[i]])
}

// clusterAt returns the index in bounds of the boundary c (the cursor, at a
// boundary after inputCursor clamped it).
func clusterAt(bounds []int, c int) int {
	for i, b := range bounds {
		if b >= c {
			return i
		}
	}
	return len(bounds) - 1
}

// wordStart is where alt+b moves from the cursor c: the start of the word
// before it. It skips the space characters right before the cursor, then
// the characters before those up to the previous space character or the
// start of the line (alt+backspace deletes what it skips).
func wordStart(runes []rune, bounds []int, c int) int {
	i := clusterAt(bounds, c)
	for i > 0 && spaceCluster(runes, bounds, i-1) {
		i--
	}
	for i > 0 && !spaceCluster(runes, bounds, i-1) {
		i--
	}
	return bounds[i]
}

// wordEnd is where alt+f moves from the cursor c: the end of the word
// after it. It skips the space characters after the cursor, then the
// characters of the word up to the next space character or the end of the
// line.
func wordEnd(runes []rune, bounds []int, c int) int {
	n := len(bounds) - 1
	i := clusterAt(bounds, c)
	for i < n && spaceCluster(runes, bounds, i) {
		i++
	}
	for i < n && !spaceCluster(runes, bounds, i) {
		i++
	}
	return bounds[i]
}
