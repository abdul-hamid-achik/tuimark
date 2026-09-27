package host

import (
	"unicode/utf8"
)

// DecodeKeys splits complete raw-mode terminal input into key tokens: a
// lone ESC at the end is the esc key, and an incomplete trailing sequence
// (a cut UTF-8 rune, an unterminated CSI/SS3) is dropped. The terminal
// loop decodes with decodeKeys instead, which keeps such a tail for the
// next read.
func DecodeKeys(b []byte) []Key {
	keys, _ := decodeKeys(b, true)
	return keys
}

// maxPendingCSI bounds how many bytes of an unterminated CSI sequence are
// kept for the next read; longer garbage is dropped like before.
const maxPendingCSI = 32

// incompleteTail reports whether b, which starts at a key boundary, is an
// incomplete sequence the next read may complete: a UTF-8 lead byte whose
// rune is cut, a lone ESC, ESC '[' plus parameter/intermediate bytes with
// no final byte yet, or ESC 'O' with no third byte.
func incompleteTail(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	if b[0] != 0x1b {
		return b[0] >= 0x80 && !utf8.FullRune(b)
	}
	if len(b) == 1 {
		return true
	}
	switch b[1] {
	case 'O':
		return len(b) == 2
	case '[':
		if len(b) > maxPendingCSI {
			return false
		}
		for _, c := range b[2:] {
			if c < 0x20 || c > 0x3f {
				return false
			}
		}
		return true
	}
	return false
}

// decodeKeys splits raw-mode terminal input into key tokens. With final
// set, it decodes everything (DecodeKeys). Otherwise it stops at an
// incomplete trailing sequence (incompleteTail) and returns those bytes as
// rest, so a read that cut a rune or an escape sequence in two loses
// neither half: the caller prepends rest to the next read, or decodes it
// with final once no more input follows (a lone ESC is then the esc key).
func decodeKeys(b []byte, final bool) (out []Key, rest []byte) {
	for i := 0; i < len(b); {
		if !final && incompleteTail(b[i:]) {
			return out, b[i:]
		}
		c := b[i]
		switch {
		case c == 0x1b:
			if i+1 >= len(b) {
				out = append(out, Key{Name: "esc"})
				i++
				continue
			}
			k, n := decodeEscape(b[i:])
			if n == 0 {
				out = append(out, Key{Name: "esc"})
				i++
				continue
			}
			if k.Name != "" {
				out = append(out, k)
			}
			i += n
		case c == '\r' || c == '\n':
			out = append(out, Key{Name: "enter"})
			i++
		case c == '\t':
			out = append(out, Key{Name: "tab"})
			i++
		case c == 0x7f:
			out = append(out, Key{Name: "backspace"})
			i++
		case c == 0:
			i++
		case c < 0x20:
			out = append(out, Key{Name: "ctrl+" + string(rune('a'+c-1))})
			i++
		case c == ' ':
			out = append(out, Key{Name: "space", Rune: ' '})
			i++
		default:
			r, size := utf8.DecodeRune(b[i:])
			if r == utf8.RuneError && size <= 1 {
				i++
				continue
			}
			out = append(out, Key{Name: string(r), Rune: r})
			i += size
		}
	}
	return out, nil
}

// decodeEscape decodes a CSI/SS3 sequence at the start of b (b[0]==ESC).
// It returns the key (empty Name for ignored sequences) and bytes consumed;
// 0 means "not a sequence" (a plain esc).
func decodeEscape(b []byte) (Key, int) {
	if len(b) < 2 {
		return Key{}, 0
	}
	switch b[1] {
	case '[':
		// CSI: parameters 0x30-0x3f, intermediates 0x20-0x2f, final 0x40-0x7e.
		j := 2
		for j < len(b) && b[j] >= 0x20 && b[j] <= 0x3f {
			j++
		}
		if j >= len(b) {
			return Key{}, len(b)
		}
		final := b[j]
		params := string(b[2:j])
		n := j + 1
		switch final {
		case 'A':
			return Key{Name: "up"}, n
		case 'B':
			return Key{Name: "down"}, n
		case 'C':
			return Key{Name: "right"}, n
		case 'D':
			return Key{Name: "left"}, n
		case 'H':
			return Key{Name: "home"}, n
		case 'F':
			return Key{Name: "end"}, n
		case 'Z':
			return Key{Name: "shift+tab"}, n
		case '~':
			switch params {
			case "1", "7":
				return Key{Name: "home"}, n
			case "4", "8":
				return Key{Name: "end"}, n
			case "5":
				return Key{Name: "pgup"}, n
			case "6":
				return Key{Name: "pgdn"}, n
			case "3":
				return Key{Name: "delete"}, n
			}
		}
		return Key{}, n
	case 'O':
		if len(b) < 3 {
			return Key{}, len(b)
		}
		switch b[2] {
		case 'A':
			return Key{Name: "up"}, 3
		case 'B':
			return Key{Name: "down"}, 3
		case 'C':
			return Key{Name: "right"}, 3
		case 'D':
			return Key{Name: "left"}, 3
		case 'H':
			return Key{Name: "home"}, 3
		case 'F':
			return Key{Name: "end"}, 3
		}
		return Key{}, 3
	}
	return Key{}, 0
}
