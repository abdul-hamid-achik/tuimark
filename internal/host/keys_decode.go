package host

import (
	"strconv"
	"strings"
)

// The decoder of the current language (SPEC v0.4d §26.14.2, decisions 201,
// 217, 218, and 263). One decoder reads every form a terminal sends a key
// in, whatever the probe found: the legacy bytes, Kitty's CSI-u with flag
// 1, and xterm's modifyOtherKeys (CSI 27 ; MODS ; CODE ~). A document that
// writes version keeps tuimark 0.4.4's decoder, byte for byte: every
// function here is reached only through the decoder's cur flag.

// Modifier bits of a CSI MODS parameter: MODS is 1 plus the bit set. The
// bits for super, hyper, meta, caps lock, and num lock are ignored.
const (
	modShift = 1 << iota
	modAlt
	modCtrl
)

// keypadKeys maps Kitty's keypad code points (57399 to 57427, flag 1) to
// the keys the keypad sent in tuimark 0.4.5 (SPEC §26.14.2). An empty name
// is a code that gives nothing: the separator (57416), insert (57425), and
// begin (57427).
var keypadKeys = map[int]string{
	57399: "0", 57400: "1", 57401: "2", 57402: "3", 57403: "4",
	57404: "5", 57405: "6", 57406: "7", 57407: "8", 57408: "9",
	57409: ".", 57410: "/", 57411: "*", 57412: "-", 57413: "+",
	57414: "enter", 57415: "=", 57416: "",
	57417: "left", 57418: "right", 57419: "up", 57420: "down",
	57421: "pgup", 57422: "pgdn", 57423: "home", 57424: "end",
	57425: "", 57426: "delete", 57427: "",
}

// codeBase names the base key of a Kitty or modifyOtherKeys code point:
// 9 tab, 13 enter, 27 esc, 32 space, 127 backspace, a printable ASCII
// character itself, a keypad code as the key it was; any other code gives
// nothing (ok false).
func codeBase(code int) (string, bool) {
	switch {
	case code == 9:
		return "tab", true
	case code == 13:
		return "enter", true
	case code == 27:
		return "esc", true
	case code == 32:
		return "space", true
	case code == 127:
		return "backspace", true
	case code > 32 && code < 127:
		return string(rune(code)), true
	}
	if name, ok := keypadKeys[code]; ok && name != "" {
		return name, true
	}
	return "", false
}

// decodeCode is the key a CSI-u or modifyOtherKeys report stands for:
// code is the code point and modParam the MODS parameter (1 when absent).
func decodeCode(code, modParam int) (Key, bool) {
	base, ok := codeBase(code)
	if !ok {
		return Key{}, false
	}
	return resolveKey(base, modParam)
}

// isASCIILetter reports whether s is one ASCII letter.
func isASCIILetter(s string) bool {
	return len(s) == 1 && (s[0] >= 'a' && s[0] <= 'z' || s[0] >= 'A' && s[0] <= 'Z')
}

// makesText reports whether the base key makes text: space or one
// printable ASCII character (SPEC §26.14.2, step 4 of the modifier list).
func makesText(base string) bool {
	return base == "space" || (len(base) == 1 && base[0] > ' ' && base[0] < 0x7f)
}

// resolveKey applies the modifier rules of SPEC §26.14.2 to a base key and
// a MODS parameter and returns the first token of §8.1 in the list the
// SPEC gives, or no key:
//
//  1. the modifiers and the base key as they are;
//  2. without shift;
//  3. without shift and alt;
//  4. the base key alone, unless ctrl is held and the base key makes text
//     (space or a printable ASCII character): then the key gives nothing
//     (decision 263).
//
// Before that list these pairs are rewritten: with ctrl, a letter's
// upper-case code is its lower-case one (the modifier bits unchanged), and
// the code of { with shift is that of [; ctrl with i, m, [, or j is tab,
// enter, esc, or the LF byte (enter marked LF), the shift and the ctrl
// dropped and the alt kept; shift with alt and a letter, without ctrl, is
// alt+ the upper-case letter. left and right keep whether alt or ctrl was
// held (Key.Word).
func resolveKey(base string, modParam int) (Key, bool) {
	m := 0
	if modParam > 1 {
		m = modParam - 1
	}
	shift, alt, ctrl := m&modShift != 0, m&modAlt != 0, m&modCtrl != 0
	word := (base == "left" || base == "right") && (alt || ctrl)
	lf := false
	if ctrl {
		if len(base) == 1 && base[0] >= 'A' && base[0] <= 'Z' {
			base = strings.ToLower(base)
		}
		if base == "{" && shift {
			base = "["
		}
		switch base {
		case "i":
			base, shift, ctrl = "tab", false, false
		case "m":
			base, shift, ctrl = "enter", false, false
		case "[":
			base, shift, ctrl = "esc", false, false
		case "j":
			base, shift, ctrl, lf = "enter", false, false, true
		}
	} else if shift && alt && isASCIILetter(base) {
		base, shift = strings.ToUpper(base), false
	}
	tryOrder := [3][3]bool{{shift, alt, ctrl}, {false, alt, ctrl}, {false, false, ctrl}}
	for _, t := range tryOrder {
		if k, ok := tokenKey(base, t[0], t[1], t[2]); ok {
			k.Word = word
			k.LF = lf && k.Name == "enter"
			return k, true
		}
	}
	if ctrl && makesText(base) {
		return Key{}, false
	}
	k, ok := tokenKey(base, false, false, false)
	if ok {
		k.Word = word
		k.LF = lf && k.Name == "enter"
	}
	return k, ok
}

// tokenKey is the key whose token is the base key with exactly the given
// modifiers, when that is a token of the current language (SPEC §8.1):
// tab or enter with shift; a letter or a digit, enter, or backspace with
// alt; a lower-case letter or enter with ctrl; the base key alone. A key
// typed as text carries its rune.
func tokenKey(base string, shift, alt, ctrl bool) (Key, bool) {
	n := 0
	for _, b := range []bool{shift, alt, ctrl} {
		if b {
			n++
		}
	}
	switch {
	case n == 0:
		return plainKey(base)
	case n > 1:
		return Key{}, false
	case shift:
		if base == "tab" || base == "enter" {
			return Key{Name: "shift+" + base}, true
		}
	case alt:
		if base == "enter" || base == "backspace" || (len(base) == 1 && isAltByte(base[0])) {
			return Key{Name: "alt+" + base}, true
		}
	case ctrl:
		if base == "enter" || (len(base) == 1 && base[0] >= 'a' && base[0] <= 'z') {
			return Key{Name: "ctrl+" + base}, true
		}
	}
	return Key{}, false
}

func isAltByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

// plainKey is the key with no modifier named by base: a named key, or one
// printable ASCII character (the key typed).
func plainKey(base string) (Key, bool) {
	switch base {
	case "enter", "esc", "tab", "backspace", "delete", "up", "down", "left", "right",
		"home", "end", "pgup", "pgdn",
		"f1", "f2", "f3", "f4", "f5", "f6", "f7", "f8", "f9", "f10", "f11", "f12":
		return Key{Name: base}, true
	case "space":
		return Key{Name: "space", Rune: ' '}, true
	}
	if len(base) == 1 && base[0] > ' ' && base[0] < 0x7f {
		return Key{Name: base, Rune: rune(base[0])}, true
	}
	return Key{}, false
}

// csiNumbers parses the parameters of a CSI: separated by ';', each with
// any ':'-separated sub-parameters ignored. An empty parameter is
// defaultNum; a parameter that is not a decimal number makes ok false.
// The count is bounded by maxCSI, so a parameter list is short.
const defaultNum = -1

func csiNumbers(params []byte) (nums []int, ok bool) {
	if len(params) == 0 {
		return nil, true
	}
	for _, field := range strings.Split(string(params), ";") {
		first, _, _ := strings.Cut(field, ":")
		if first == "" {
			nums = append(nums, defaultNum)
			continue
		}
		if len(first) > 9 {
			return nil, false
		}
		n, err := strconv.Atoi(first)
		if err != nil || n < 0 {
			return nil, false
		}
		nums = append(nums, n)
	}
	return nums, true
}

// modsOf is the MODS parameter at nums[i], 1 when absent or empty.
func modsOf(nums []int, i int) int {
	if i < len(nums) && nums[i] > 0 {
		return nums[i]
	}
	return 1
}

// specialKeys maps the N of CSI N ~ to the key it stands for in the
// current language: home, delete, end, pgup, pgdn, and the F keys. 2
// (insert), and every other N, give nothing.
var specialKeys = map[int]string{
	1: "home", 3: "delete", 4: "end", 5: "pgup", 6: "pgdn", 7: "home", 8: "end",
	11: "f1", 12: "f2", 13: "f3", 14: "f4", 15: "f5",
	17: "f6", 18: "f7", 19: "f8", 20: "f9", 21: "f10",
	23: "f11", 24: "f12",
}

// csiKeyCur decodes the key a CSI sequence of the current language
// encodes (SPEC §26.14.2), with no private marker, and whether it is one.
// The caller has already taken the replies, the mouse reports, and the
// paste markers.
func csiKeyCur(final byte, params []byte) (Key, bool) {
	nums, ok := csiNumbers(params)
	if !ok {
		return Key{}, false
	}
	first := defaultNum
	if len(nums) > 0 {
		first = nums[0]
	}
	switch final {
	case 'u':
		if len(nums) == 0 || first <= 0 {
			return Key{}, false
		}
		return decodeCode(first, modsOf(nums, 1))
	case '~':
		if first == 27 && len(nums) >= 3 {
			// modifyOtherKeys: CSI 27 ; MODS ; CODE ~.
			return decodeCode(nums[2], modsOf(nums, 1))
		}
		if base, ok := specialKeys[first]; ok {
			return resolveKey(base, modsOf(nums, 1))
		}
		return Key{}, false
	case 'A', 'B', 'C', 'D', 'H', 'F':
		base := map[byte]string{'A': "up", 'B': "down", 'C': "right", 'D': "left", 'H': "home", 'F': "end"}[final]
		return resolveKey(base, modsOf(nums, 1))
	case 'Z':
		return Key{Name: "shift+tab"}, true
	case 'P', 'Q', 'S':
		// CSI P, CSI Q, CSI S, and CSI 1 ; MODS P to S.
		if len(nums) != 0 && !(len(nums) >= 2 && first == 1) {
			return Key{}, false
		}
		base := map[byte]string{'P': "f1", 'Q': "f2", 'S': "f4"}[final]
		return resolveKey(base, modsOf(nums, 1))
	case 'R':
		// CSI 1 ; MODS R is f3 with modifiers. CSI R and CSI N ; M R with N
		// other than 1 stay cursor position reports, which give nothing.
		if len(nums) == 2 && first == 1 {
			return resolveKey("f3", modsOf(nums, 1))
		}
	}
	return Key{}, false
}

// ss3KeyCur is the key ESC O final stands for in the current language: the
// arrows, home, end (as in tuimark 0.4.4), and f1 to f4.
func ss3KeyCur(b byte) (string, bool) {
	switch b {
	case 'A':
		return "up", true
	case 'B':
		return "down", true
	case 'C':
		return "right", true
	case 'D':
		return "left", true
	case 'H':
		return "home", true
	case 'F':
		return "end", true
	case 'P':
		return "f1", true
	case 'Q':
		return "f2", true
	case 'R':
		return "f3", true
	case 'S':
		return "f4", true
	}
	return "", false
}

// altKeyCur decodes ESC followed by the byte c, in the current language
// (SPEC §26.14.2, decision 218): a letter or a digit is alt+ it, CR and LF
// are alt+enter, DEL is alt+backspace. ESC O, ESC P, and ESC X open SS3,
// DCS, and SOS, so alt+O, alt+P, and alt+X never come out of legacy bytes
// (the caller has taken those before it asks). Anything else is not an alt
// key.
func altKeyCur(c byte) (Key, bool) {
	switch {
	case isAltByte(c):
		return Key{Name: "alt+" + string(rune(c))}, true
	case c == '\r' || c == '\n':
		return Key{Name: "alt+enter"}, true
	case c == 0x7f:
		return Key{Name: "alt+backspace"}, true
	}
	return Key{}, false
}
