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
