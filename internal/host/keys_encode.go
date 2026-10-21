package host

import (
	"strconv"
	"strings"
)

// EncodeKey is the bytes a terminal with the keyboard kb sends for the key
// token tok, which `tuimark play` delivers to the decoder of the current
// language as one read (SPEC v0.4d §15.4, "Keys and the keyboard", decision
// 221). kb is KeyboardKitty ("" too) or KeyboardLegacy.
//
// Under Kitty (flag 1) a key has the CSI-u form for esc, for a key held
// with ctrl or alt, and for a modified enter or backspace (CSI 27 u,
// CSI 97 ; 5 u for ctrl+a, CSI 120 ; 3 u for alt+x, CSI 13 ; 2 u for
// shift+enter), and the legacy bytes otherwise (a, CR for enter, CSI A for
// up, CSI 3 ~ for delete, CSI Z for shift+tab), with CSI P for f1, as a
// Kitty terminal sends a key that makes no text. Under legacy it has the
// legacy bytes, ESC O P for f1, ESC x for alt+x, ESC CR for alt+enter, and
// ESC DEL for alt+backspace. ok is false for a token the keyboard cannot
// send (shift+enter, ctrl+enter, alt+O, alt+P, and alt+X under legacy) and
// for a string that is no token.
//
// ctrl+i, ctrl+j, and ctrl+m are their own bytes, TAB, LF, and CR, under
// both keyboards (SPEC §8.1): they arrive as tab and enter, and the LF
// byte reaches a focused textarea as a line break (§26.14.4).
func EncodeKey(tok, kb string) (string, bool) {
	kitty := kb != KeyboardLegacy
	csi := func(s string) (string, bool) { return "\x1b[" + s, true }
	switch tok {
	case "space":
		return " ", true
	case "enter":
		return "\r", true
	case "tab":
		return "\t", true
	case "backspace":
		return "\x7f", true
	case "esc":
		if kitty {
			return csi("27u")
		}
		return "\x1b", true
	case "up":
		return csi("A")
	case "down":
		return csi("B")
	case "right":
		return csi("C")
	case "left":
		return csi("D")
	case "home":
		return csi("H")
	case "end":
		return csi("F")
	case "pgup":
		return csi("5~")
	case "pgdn":
		return csi("6~")
	case "delete":
		return csi("3~")
	case "shift+tab":
		return csi("Z")
	case "shift+enter":
		if kitty {
			return csi("13;2u")
		}
		return "", false
	case "ctrl+enter":
		if kitty {
			return csi("13;5u")
		}
		return "", false
	case "alt+enter":
		if kitty {
			return csi("13;3u")
		}
		return "\x1b\r", true
	case "alt+backspace":
		if kitty {
			return csi("127;3u")
		}
		return "\x1b\x7f", true
	case "ctrl+i":
		return "\t", true
	case "ctrl+j":
		return "\n", true
	case "ctrl+m":
		return "\r", true
	}
	if n, ok := fKeyNumber(tok); ok {
		return fKeyBytes(n, kitty), true
	}
	if rest, ok := strings.CutPrefix(tok, "ctrl+"); ok {
		if len(rest) != 1 || rest[0] < 'a' || rest[0] > 'z' {
			return "", false
		}
		if kitty {
			return csi(strconv.Itoa(int(rest[0])) + ";5u")
		}
		return string(rune(rest[0] - 'a' + 1)), true
	}
	if rest, ok := strings.CutPrefix(tok, "alt+"); ok {
		if len(rest) != 1 || !isAltByte(rest[0]) {
			return "", false
		}
		c := rest[0]
		if kitty {
			// alt+shift+a is alt+A: Kitty reports the base key, a.
			if c >= 'A' && c <= 'Z' {
				return csi(strconv.Itoa(int(c-'A'+'a')) + ";4u")
			}
			return csi(strconv.Itoa(int(c)) + ";3u")
		}
		if unsendableLegacy(tok) {
			return "", false
		}
		return "\x1b" + string(rune(c)), true
	}
	if len(tok) == 1 && tok[0] > ' ' && tok[0] < 0x7f {
		return tok, true
	}
	return "", false
}

// fKeyNumber parses f1 to f12.
func fKeyNumber(tok string) (int, bool) {
	rest, ok := strings.CutPrefix(tok, "f")
	if !ok || rest == "" || rest[0] == '0' || len(rest) > 2 {
		return 0, false
	}
	n, err := strconv.Atoi(rest)
	if err != nil || n < 1 || n > 12 {
		return 0, false
	}
	return n, true
}

// fKeyBytes are the bytes of F key n: f1 to f4 are CSI P, CSI Q, CSI 13 ~,
// and CSI S under Kitty and ESC O P to ESC O S under legacy; f5 to f12 are
// CSI N ~ with N 15, 17 to 21, 23, and 24 in both.
func fKeyBytes(n int, kitty bool) string {
	if n <= 4 {
		if !kitty {
			return "\x1bO" + string(rune('P'+n-1))
		}
		return [...]string{"\x1b[P", "\x1b[Q", "\x1b[13~", "\x1b[S"}[n-1]
	}
	codes := [...]int{15, 17, 18, 19, 20, 21, 23, 24}
	return "\x1b[" + strconv.Itoa(codes[n-5]) + "~"
}
