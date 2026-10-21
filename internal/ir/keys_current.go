package ir

import "strings"

// The key tokens 0.4d adds to the current language (SPEC v0.4d §8.1,
// decision 215): delete, f1 to f12, alt+ a letter (either case) or a digit,
// alt+enter, alt+backspace, shift+enter, and ctrl+enter. ValidKey and
// NamedKeys stay tuimark 0.4.4's list, which a document that writes
// version keeps (decision 202); ValidKeyLang is the check of a document's
// own language.

// NamedKeysCurrent are the named tokens only the current language accepts,
// in the order of SPEC §8.1.
var NamedKeysCurrent = []string{
	"delete",
	"f1", "f2", "f3", "f4", "f5", "f6", "f7", "f8", "f9", "f10", "f11", "f12",
	"shift+enter", "ctrl+enter", "alt+enter", "alt+backspace",
}

// NamedKeysAll is every named token of the current language, in the order
// of SPEC §8.1: tuimark 0.4.4's NamedKeys with the 0.4d tokens in place.
// `tuimark catalog` lists it as keys.named, and AGENTS.md prints it.
var NamedKeysAll = []string{
	"enter", "esc", "tab", "backspace", "space", "delete",
	"up", "down", "left", "right", "home", "end", "pgup", "pgdn",
	"f1", "f2", "f3", "f4", "f5", "f6", "f7", "f8", "f9", "f10", "f11", "f12",
	"shift+tab", "shift+enter", "ctrl+enter", "alt+enter", "alt+backspace",
}

// KeysAltChars is the characters alt+ takes (SPEC v0.4d §8.1, §15.10).
const KeysAltChars = "a-z A-Z 0-9"

// KeysCurrentSince is the language number the 0.4d tokens carry in the
// catalog (named_since, alt_since): the current language is 4 whatever
// the cut (SPEC v0.4d §5.1, decision 201).
const KeysCurrentSince = Lang4

// IsKeyCurrent reports whether tok is a key token only the current
// language accepts: delete, f1 to f12, alt+ an ASCII letter or digit,
// alt+enter, alt+backspace, shift+enter, or ctrl+enter. `alt+` a
// punctuation character, alt+space, shift+ a letter, ctrl+ a named key,
// alt+ any other named key, and anything after f12 are not tokens.
func IsKeyCurrent(tok string) bool {
	switch tok {
	case "delete", "shift+enter", "ctrl+enter", "alt+enter", "alt+backspace":
		return true
	}
	if rest, ok := strings.CutPrefix(tok, "alt+"); ok {
		return len(rest) == 1 && isAltChar(rest[0])
	}
	if rest, ok := strings.CutPrefix(tok, "f"); ok {
		switch len(rest) {
		case 1:
			return rest[0] >= '1' && rest[0] <= '9'
		case 2:
			return rest == "10" || rest == "11" || rest == "12"
		}
	}
	return false
}

func isAltChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

// ValidKeyLang reports whether tok is a key token of a document of
// language l: tuimark 0.4.4's tokens (ValidKey), plus the 0.4d tokens in
// the current language (SPEC v0.4d §8.1).
func ValidKeyLang(tok string, l Lang) bool {
	return ValidKey(tok) || (l.V4() && IsKeyCurrent(tok))
}

// UnknownKeyList is the list of tokens an unknown-key V003 gives (SPEC
// v0.4d §8.1, decision 224): the current language lists the 0.4d tokens, a
// pinned document keeps tuimark 0.4.4's list.
func UnknownKeyList(l Lang) string {
	if l.V4() {
		return "a-z 0-9 enter esc tab backspace space delete arrows home end pgup pgdn f1-f12 ctrl+x alt+x shift+tab shift+enter ctrl+enter alt+enter alt+backspace"
	}
	return "a-z 0-9 enter esc tab backspace space arrows home end pgup pgdn ctrl+x shift+tab"
}
