package host

import "testing"

// SPEC 0.4d test 265 and decision 265: the one normalizer of run text.
func TestNormalizeRunText(t *testing.T) {
	cases := []struct{ in, want string }{
		// The three vectors of §6.20.2.
		{"a\x1b\tb", "a    b"},
		{"a\x1b(Bb", "ab"},
		{"a\tb\x1bc", "a    b"},
		// Nothing to do.
		{"", ""},
		{"plain text", "plain text"},
		{"naïve 微信 🇺🇸", "naïve 微信 🇺🇸"},
		// LF is the hard line break and stays; CR is a control and goes.
		{"a\nb", "a\nb"},
		{"a\r\nb", "a\nb"},
		{"a\rb", "ab"},
		// Other C0 controls, DEL, and C1 controls go, each alone.
		{"a\x00b\x07c\x08d", "abcd"},
		{"a\x7fb", "ab"},
		{"a\u0085b\u009fc", "abc"},
		{"a b", "a b"}, // U+00A0 is not a control
		// Tabs are four spaces each.
		{"\t\t", "        "},
		{"x\ty", "x    y"},
		// Escapes are removed whole: CSI, OSC (BEL or ST), SS3, DCS.
		{"a\x1b[31mred\x1b[0mb", "aredb"},
		{"a\x1b]0;title\x07b", "ab"},
		{"a\x1b]0;title\x1b\\b", "ab"},
		{"a\x1bOPb", "ab"},
		{"a\x1bPdata\x1b\\b", "ab"},
		// An ESC followed by a control, a non-ASCII character, or nothing
		// is removed alone.
		{"a\x1b\nb", "a\nb"},
		{"a\x1bé", "aé"},
		{"a\x1b", "a"},
		{"\x1b\x1b", ""},
		// A sequence the text ends inside runs to the end.
		{"a\x1b[31", "a"},
		// A TAB after an ESC is not an escape's intermediate byte.
		{"a\x1b\tb\x1b\tc", "a    b    c"},
		// Invalid UTF-8 becomes U+FFFD, one per byte.
		{"a\xffb", "a�b"},
		{"a\xff\xfeb", "a��b"},
	}
	for _, c := range cases {
		if got := NormalizeRunText(c.in); got != c.want {
			t.Errorf("NormalizeRunText(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// NormalizeRunText is idempotent: its output holds nothing it removes.
func TestNormalizeRunTextIdempotent(t *testing.T) {
	for _, in := range []string{"a\x1b\tb", "x\x1b[1;2Hy\tz", "\x1b]52;c;\x07q", "\u0085\t\r\n\x7f"} {
		once := NormalizeRunText(in)
		if twice := NormalizeRunText(once); twice != once {
			t.Errorf("NormalizeRunText(%q) = %q, then %q", in, once, twice)
		}
	}
}
