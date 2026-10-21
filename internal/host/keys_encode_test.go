package host

import (
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// allTokens lists every key token of the current language (SPEC v0.4d
// §8.1) plus the three that never arrive and the typed characters.
func allTokens() []string {
	toks := append([]string(nil), ir.NamedKeysAll...)
	for c := 'a'; c <= 'z'; c++ {
		toks = append(toks, "ctrl+"+string(c))
	}
	for _, c := range "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789" {
		toks = append(toks, "alt+"+string(c))
	}
	for c := '!'; c < 0x7f; c++ {
		if c != ',' {
			toks = append(toks, string(c))
		}
	}
	return toks
}

// SPEC v0.4d §15.4, decision 221: every token, encoded for a keyboard and
// decoded by the decoder of the current language, is that key again (the
// three tokens that never arrive are tab and enter), and a token a legacy
// terminal cannot send has no bytes there.
func TestEncodeDecodeRoundTrip(t *testing.T) {
	for _, kb := range []string{KeyboardKitty, KeyboardLegacy} {
		for _, tok := range allTokens() {
			b, ok := EncodeKey(tok, kb)
			if !ok {
				if kb != KeyboardLegacy || !UnsendableLegacy(tok) {
					t.Errorf("%s: %q has no bytes", kb, tok)
				}
				continue
			}
			if kb == KeyboardLegacy && UnsendableLegacy(tok) {
				t.Errorf("legacy: %q has bytes %q", tok, b)
			}
			want := tok
			switch tok {
			case "ctrl+i":
				want = "tab"
			case "ctrl+j", "ctrl+m":
				want = "enter"
			}
			ins := DecodeInputLang([]byte(b), true)
			if len(ins) != 1 || ins[0].Key.Name != want {
				t.Errorf("%s: %q -> %q -> %+v, want %s", kb, tok, b, ins, want)
				continue
			}
			if (tok == "ctrl+j") != ins[0].Key.LF {
				t.Errorf("%s: %q LF mark = %v", kb, tok, ins[0].Key.LF)
			}
			if (len(tok) == 1 || tok == "space") != (ins[0].Key.Rune != 0) {
				t.Errorf("%s: %q rune %q", kb, tok, ins[0].Key.Rune)
			}
		}
	}
	for _, tok := range []string{"shift+enter", "ctrl+enter", "alt+O", "alt+P", "alt+X"} {
		if _, ok := EncodeKey(tok, KeyboardLegacy); ok {
			t.Errorf("legacy sends %q", tok)
		}
		if _, ok := EncodeKey(tok, KeyboardKitty); !ok {
			t.Errorf("kitty cannot send %q", tok)
		}
	}
	for _, tok := range []string{"", "f0", "f13", "alt+!", "alt+space", "shift+a", "ctrl+delete", "alt+f1", "ctrl+1", "nope"} {
		if _, ok := EncodeKey(tok, KeyboardKitty); ok {
			t.Errorf("%q has bytes", tok)
		}
	}
}
