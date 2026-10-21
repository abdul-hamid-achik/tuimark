package host

import (
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// FuzzDecoderCurrent checks the decoder of the current language (SPEC
// v0.4d §26.14.2) on arbitrary bytes, split into reads at arbitrary
// points: it never panics; a key it produces is a token of §8.1, a typed
// character, or one of the ctrl+ names of the C0 bytes that 0.4.4 already
// produced; an alt key has no rune; the marks sit only where they belong;
// and the pinned decoder never produces a mark.
func FuzzDecoderCurrent(f *testing.F) {
	for _, s := range []string{
		"\x1b[97;5u", "\x1b[27;6;65~", "\x1b[13;2u", "\x1b[1;5D", "\x1bx", "\x1b\r", "\x1b\x7f",
		"\x1b[24~", "\x1b[24;2~", "\x1bOP", "\x1b[1;2R", "\x1b[?1u", "\x1b[57417;3u", "\x1b[200~x\x1b[201~",
		"\x1b[<0;3;4M", "\x1b[M abc", "\n", "\x1b[999999999999;5u", "\x1b[97:65;3:1;97u",
	} {
		f.Add([]byte(s), uint8(3))
	}
	f.Fuzz(func(t *testing.T, b []byte, cut uint8) {
		for _, cur := range []bool{false, true} {
			d := decoder{cur: cur, f12: cut%2 == 0}
			var ins []Input
			split := int(cut) % (len(b) + 1)
			got, _ := d.feed(b[:split], false)
			ins = append(ins, got...)
			got, _ = d.feed(b[split:], true)
			ins = append(ins, got...)
			for _, in := range ins {
				if in.IsPaste || in.IsMouse || in.IsF12 {
					continue
				}
				k := in.Key
				if !cur && (k.Word || k.LF) {
					t.Fatalf("the pinned decoder marked %+v for %q", k, b)
				}
				if k.Word && k.Name != "left" && k.Name != "right" {
					t.Fatalf("Word on %+v for %q", k, b)
				}
				if k.LF && k.Name != "enter" {
					t.Fatalf("LF on %+v for %q", k, b)
				}
				if strings.HasPrefix(k.Name, "alt+") && k.Rune != 0 {
					t.Fatalf("alt key with a rune: %+v for %q", k, b)
				}
				if !cur {
					continue
				}
				if !ir.ValidKeyLang(k.Name, ir.LangCurrent) && !typedKey(k) && !c0CtrlName(k.Name) {
					t.Fatalf("not a key of the current language: %+v for %q", k, b)
				}
				if d.f12 && k.Name == "f12" {
					t.Fatalf("f12 reached the dispatch under run: %q", b)
				}
			}
		}
	})
}

// typedKey reports whether k is a typed character: a rune, the name its
// text, or space.
func typedKey(k Key) bool {
	if k.Rune == 0 {
		return false
	}
	return k.Name == string(k.Rune) || (k.Name == "space" && k.Rune == ' ')
}

// c0CtrlName reports whether name is the ctrl+ name the C0 bytes get: 0.4.4
// names 0x1c to 0x1f with the characters after z, which no token names.
func c0CtrlName(name string) bool {
	rest, ok := strings.CutPrefix(name, "ctrl+")
	return ok && len([]rune(rest)) == 1
}
