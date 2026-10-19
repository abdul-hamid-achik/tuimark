package ir

import "strings"

// NamedKeys are the non-character key tokens (SPEC §8.1).
var NamedKeys = []string{
	"enter", "esc", "tab", "backspace", "space", "up", "down", "left", "right",
	"home", "end", "pgup", "pgdn", "shift+tab",
}

// ValidKey reports whether tok is a key token: a named key, ctrl+<a-z>, or a
// single printable ASCII character (a-z, 0-9, and punctuation such as "/").
func ValidKey(tok string) bool {
	for _, k := range NamedKeys {
		if tok == k {
			return true
		}
	}
	if strings.HasPrefix(tok, "ctrl+") {
		rest := tok[5:]
		return len(rest) == 1 && rest[0] >= 'a' && rest[0] <= 'z'
	}
	return len(tok) == 1 && tok[0] > ' ' && tok[0] < 0x7f && tok[0] != ','
}

// NeverMatchKeys are the ctrl tokens whose bytes (0x09, 0x0a, 0x0d) the
// terminal decoder turns into tab and enter before a ctrl+<letter> key
// ever exists, so a keymap row never sees them (SPEC §8.1, §26.8). They
// stay valid key tokens and valid play steps, which deliver tab or enter
// (decision 82); from KeysRejectedSince on, a <bind> that writes one is
// V003 (SPEC v0.4b §8.1).
var NeverMatchKeys = []string{"ctrl+i", "ctrl+j", "ctrl+m"}

// KeysRejectedSince is the first language in which NeverMatchKeys are
// V003 in a keymap (SPEC v0.4b §8.1): the catalog's keys.rejected_since
// (§15.10). It stays 4 when a later cut moves the current language.
const KeysRejectedSince = Lang4

// ArrivesAs reports the key a NeverMatchKeys token arrives as (tab or
// enter), and whether tok is one of them.
func ArrivesAs(tok string) (string, bool) {
	switch tok {
	case "ctrl+i":
		return "tab", true
	case "ctrl+j", "ctrl+m":
		return "enter", true
	}
	return "", false
}

// SplitKeys splits keys="q,ctrl+c" into tokens.
func SplitKeys(s string) []string {
	var out []string
	for _, k := range strings.Split(s, ",") {
		k = strings.TrimSpace(k)
		if k != "" {
			out = append(out, k)
		}
	}
	return out
}
