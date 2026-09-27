package host

import (
	"strings"
	"testing"
)

func keyNames(ks []Key) string {
	var out []string
	for _, k := range ks {
		out = append(out, k.Name)
	}
	return strings.Join(out, " ")
}

func TestDecodeKeys(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"letter", "a", "a"},
		{"upper", "Q", "Q"},
		{"digits and punctuation", "0/?", "0 / ?"},
		{"utf8", "ñé", "ñ é"},
		{"space", " ", "space"},
		{"enter cr", "\r", "enter"},
		{"enter lf", "\n", "enter"},
		{"tab", "\t", "tab"},
		{"backspace del", "\x7f", "backspace"},
		{"ctrl+h is not backspace", "\x08", "ctrl+h"},
		{"ctrl+a", "\x01", "ctrl+a"},
		{"ctrl+c", "\x03", "ctrl+c"},
		{"ctrl+u", "\x15", "ctrl+u"},
		{"nul dropped", "\x00a", "a"},
		{"invalid utf8 dropped", "\xffa", "a"},
		{"lone esc", "\x1b", "esc"},
		{"esc esc", "\x1b\x1b", "esc esc"},
		{"alt+x is esc then x", "\x1bx", "esc x"},
		{"csi up", "\x1b[A", "up"},
		{"csi down", "\x1b[B", "down"},
		{"csi right", "\x1b[C", "right"},
		{"csi left", "\x1b[D", "left"},
		{"csi home", "\x1b[H", "home"},
		{"csi end", "\x1b[F", "end"},
		{"shift+tab", "\x1b[Z", "shift+tab"},
		{"home ~1", "\x1b[1~", "home"},
		{"home ~7", "\x1b[7~", "home"},
		{"end ~4", "\x1b[4~", "end"},
		{"end ~8", "\x1b[8~", "end"},
		{"pgup", "\x1b[5~", "pgup"},
		{"pgdn", "\x1b[6~", "pgdn"},
		{"delete", "\x1b[3~", "delete"},
		{"modified arrow", "\x1b[1;5A", "up"},
		{"ss3 up", "\x1bOA", "up"},
		{"ss3 down", "\x1bOB", "down"},
		{"ss3 right", "\x1bOC", "right"},
		{"ss3 left", "\x1bOD", "left"},
		{"ss3 home", "\x1bOH", "home"},
		{"ss3 end", "\x1bOF", "end"},
		{"ss3 f1 ignored", "\x1bOPa", "a"},
		{"unknown csi ignored", "\x1b[200~x", "x"},
		{"unknown tilde ignored", "\x1b[99~x", "x"},
		{"incomplete csi dropped", "\x1b[1;", ""},
		{"incomplete ss3 dropped", "\x1bO", ""},
		{"mixed", "ab\x1b[Bc\r", "a b down c enter"},
	}
	for _, c := range cases {
		if got := keyNames(DecodeKeys([]byte(c.in))); got != c.want {
			t.Errorf("%s: DecodeKeys(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
	// Printable keys carry their rune; named keys do not (except space).
	ks := DecodeKeys([]byte("x \r\x1b[A"))
	if ks[0].Rune != 'x' || ks[1].Rune != ' ' || ks[2].Rune != 0 || ks[3].Rune != 0 {
		t.Errorf("runes = %+v", ks)
	}
}

// Round 2, finding 4: a read that cuts a UTF-8 rune or an escape sequence
// in two loses neither half. For every split point, decoding the first
// part (keeping its incomplete tail) and then tail+second part gives the
// keys of the whole input.
func TestDecodeKeysAcrossReads(t *testing.T) {
	inputs := []string{
		"aé€😀b",
		"xy\x1b[Az",
		"\x1b[1;5A\x1b[5~\x1bOB",
		"é\x1b[Bñ\x1b\x1b[C",
		"\x1bx",       // alt+x stays esc then x
		"\x1b\x1b[D€", // esc, then left
	}
	for _, in := range inputs {
		want := keyNames(DecodeKeys([]byte(in)))
		for cut := 0; cut <= len(in); cut++ {
			first, rest := decodeKeys([]byte(in[:cut]), false)
			second, tail := decodeKeys(append(append([]byte(nil), rest...), in[cut:]...), true)
			if tail != nil {
				t.Errorf("%q cut at %d: final decode left %q", in, cut, tail)
			}
			if got := keyNames(append(first, second...)); got != want {
				t.Errorf("%q cut at %d: %q, want %q", in, cut, got, want)
			}
		}
	}
	// What stays undecoded, and what does not.
	for _, c := range []struct{ in, rest string }{
		{"a\xc3", "\xc3"},
		{"a\xe2\x82", "\xe2\x82"},
		{"a\x1b", "\x1b"},
		{"a\x1b[", "\x1b["},
		{"a\x1b[1;5", "\x1b[1;5"},
		{"a\x1bO", "\x1bO"},
		{"a\xff", ""},                           // invalid, not incomplete
		{"a\xe2x", ""},                          // invalid lead: dropped, x decoded
		{"a\x1b[1;5A", ""},                      // complete
		{"\x1b[" + strings.Repeat("1", 40), ""}, // too long to be a key: dropped
	} {
		_, rest := decodeKeys([]byte(c.in), false)
		if string(rest) != c.rest {
			t.Errorf("decodeKeys(%q) rest = %q, want %q", c.in, rest, c.rest)
		}
	}
}
