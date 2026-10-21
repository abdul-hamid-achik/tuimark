package host

import (
	"strings"
	"unicode/utf8"
)

// runTab is what a TAB becomes in run text (SPEC v0.4d §6.20.2, decisions
// 208 and 265): Tuimark has no tab stops anywhere, so four spaces.
const runTab = "    "

// NormalizeRunText is the one normalizer of run text (SPEC v0.4d §6.20.2,
// decision 265). Run text is the text of a run of a `<text runs>`, and,
// in the later phases of 0.4d, a committed line, a textarea's value as the
// host wrote it, and a paste into a textarea once its CR LF and CR are LF.
// In this order:
//
//  1. escape sequences are removed, each whole, with the units of step (4)
//     of §26.7's paste normalization (pasteEscapeLen), where an ESC
//     followed by a control, a non-ASCII character, or nothing is removed
//     alone;
//  2. each TAB becomes four spaces;
//  3. every remaining control but LF is removed, each alone: the C0
//     controls (U+0000-U+001F), a CR included, DEL, and the C1 controls
//     (U+0080-U+009F).
//
// So "a ESC TAB b" is "a    b" (the ESC before the TAB is removed alone),
// "a ESC ( B b" is "ab", and "a TAB b ESC c" is "a    b". Escapes come
// first because a TAB made spaces first would become an escape's
// intermediate bytes, which take the next character with it. LF is the
// hard line break and is kept; no `{...}` is read. A byte that is not
// valid UTF-8 becomes U+FFFD, as StripControl writes it for a bound value
// (the SPEC does not say; this keeps one rule for the host's data).
//
// A paste into an input keeps NormalizePaste's order (step 3 before step
// 4), and other bound data keeps StripControl; neither goes through this.
func NormalizeRunText(s string) string {
	if !needsRunNormalization(s) {
		return s
	}
	// Step 1: escape sequences, whole.
	if strings.IndexByte(s, 0x1b) >= 0 {
		var b strings.Builder
		b.Grow(len(s))
		for i := 0; i < len(s); {
			if s[i] == 0x1b {
				i += pasteEscapeLen(s[i:])
				continue
			}
			b.WriteByte(s[i])
			i++
		}
		s = b.String()
	}
	// Steps 2 and 3, in one pass over the text the escapes left: a TAB is
	// four spaces, LF stays, any other control goes.
	var out strings.Builder
	out.Grow(len(s))
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == '\t':
			out.WriteString(runTab)
		case r == '\n':
			out.WriteByte('\n')
		case r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f):
			// removed
		case r == utf8.RuneError && size == 1:
			out.WriteRune(utf8.RuneError)
		default:
			out.WriteString(s[i : i+size])
		}
		i += size
	}
	return out.String()
}

// needsRunNormalization reports whether s holds anything NormalizeRunText
// changes: a control other than LF, DEL, the lead byte of a C1 control
// (0xC2, conservatively), or a byte that is not valid UTF-8.
func needsRunNormalization(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < 0x20 && c != '\n') || c == 0x7f || c == 0xc2 {
			return true
		}
	}
	return !utf8.ValidString(s)
}
