package host

import (
	"fmt"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

const (
	esc = "\x1b"
	csi = "\x1b["
)

// decodedNames decodes b with the decoder of the current language, as one
// read that a pause ends, and lists the key names it produced.
func decodedNames(b string) []string {
	var out []string
	for _, in := range DecodeInputLang([]byte(b), true) {
		switch {
		case in.IsPaste:
			out = append(out, "<paste>")
		case in.IsMouse:
			out = append(out, "<mouse>")
		default:
			out = append(out, in.Key.Name)
		}
	}
	return out
}

// SPEC v0.4d test 277: the decoder table of §26.14.2. Each row is the bytes
// of one read and the single key it decodes to.
func TestDecoderTable277(t *testing.T) {
	rows := []struct {
		bytes string
		key   string
	}{
		{csi + "3~", "delete"},
		{esc + "OP", "f1"},
		{csi + "P", "f1"},
		{csi + "11~", "f1"},
		{csi + "13~", "f3"},
		{csi + "1;2R", "f3"},
		{csi + "24~", "f12"},
		{csi + "24;5~", "f12"},
		{csi + "13;2u", "shift+enter"},
		{csi + "27;2;13~", "shift+enter"},
		{csi + "13;5u", "ctrl+enter"},
		{csi + "13;3u", "alt+enter"},
		{esc + "\r", "alt+enter"},
		{esc + "\n", "alt+enter"},
		{csi + "109;7u", "alt+enter"},
		{csi + "106;7u", "alt+enter"},
		{csi + "127;3u", "alt+backspace"},
		{esc + "\x7f", "alt+backspace"},
		{csi + "97;3u", "alt+a"},
		{csi + "27;3;97~", "alt+a"},
		{esc + "a", "alt+a"},
		{csi + "97;4u", "alt+A"},
		{csi + "97;7u", "ctrl+a"},
		{csi + "97;69u", "ctrl+a"},
		{csi + "97;6u", "ctrl+a"},
		{csi + "27;6;65~", "ctrl+a"},
		{csi + "97;8u", "ctrl+a"},
		{csi + "27;5;65~", "ctrl+a"},
		{csi + "127;5u", "backspace"},
		{csi + "105;5u", "tab"},
		{csi + "105;6u", "tab"},
		{csi + "27;6;73~", "tab"},
		{csi + "105;7u", "tab"},
		{csi + "109;5u", "enter"},
		{csi + "109;6u", "enter"},
		{csi + "27;5;77~", "enter"},
		{csi + "106;6u", "enter"},
		{csi + "27u", "esc"},
		{csi + "91;5u", "esc"},
		{csi + "91;6u", "esc"},
		{csi + "27;6;123~", "esc"},
		{csi + "9;2u", "shift+tab"},
		{csi + "Z", "shift+tab"},
		{csi + "1;5A", "up"},
		{csi + "57414u", "enter"},
		{csi + "57414;2u", "shift+enter"},
		{csi + "57419u", "up"},
		{csi + "1;3D", "left"},
		{csi + "1;5D", "left"},
		{csi + "57417;3u", "left"},
		// More of the keypad (decision 217), the extra vectors of §26.14.2.
		{csi + "57418u", "right"},
		{csi + "57420u", "down"},
		{csi + "57421u", "pgup"},
		{csi + "57422u", "pgdn"},
		{csi + "57423u", "home"},
		{csi + "57424u", "end"},
		{csi + "57426u", "delete"},
		{csi + "57399u", "0"},
		{csi + "57408u", "9"},
		{csi + "57409u", "."},
		{csi + "57410u", "/"},
		{csi + "57411u", "*"},
		{csi + "57412u", "-"},
		{csi + "57413u", "+"},
		{csi + "57415u", "="},
		// Other forms.
		{csi + "99;5u", "ctrl+c"},
		{csi + "2;1u", ""},
	}
	for _, r := range rows {
		got := decodedNames(r.bytes)
		if r.key == "" {
			continue
		}
		if len(got) != 1 || got[0] != r.key {
			t.Errorf("%q: got %v, want [%s]", r.bytes, got, r.key)
		}
	}
	// ctrl+j in its Kitty and modifyOtherKeys forms is the LF byte: enter
	// marked LF, and so is the byte itself.
	for _, b := range []string{"\n", csi + "106;5u", csi + "106;6u", csi + "27;5;106~"} {
		ins := DecodeInputLang([]byte(b), true)
		if len(ins) != 1 || ins[0].Key.Name != "enter" || !ins[0].Key.LF {
			t.Errorf("%q: %+v, want enter marked LF", b, ins)
		}
	}
	// CR is plain enter, with no mark.
	if ins := DecodeInputLang([]byte("\r"), true); len(ins) != 1 || ins[0].Key.LF {
		t.Errorf("CR: %+v", ins)
	}
	// Nothing: CSI 2 ~ (insert), the keypad's insert, a position report.
	for _, b := range []string{
		csi + "2~", csi + "57425u", csi + "6;3R", csi + "57416u", csi + "57427u",
		// ctrl with a key that makes text gives no key (decision 263).
		csi + "32;5u", csi + "47;5u", csi + "27;5;47~", csi + "51;5u", csi + "56;7u",
		csi + "50;6u", csi + "57400;5u", csi + "33;5u", csi + "123;5u", csi + "27;5;123~",
		csi + "60;7u", csi + "96;5u", csi + "93;6u",
		// Codes outside the table.
		csi + "233u", csi + "57441u", csi + "u", csi + ";5u", csi + "0u",
		// A bad number.
		csi + "99999999999;5u",
	} {
		if got := decodedNames(b); len(got) != 0 {
			t.Errorf("%q: got %v, want nothing", b, got)
		}
	}
}

// SPEC v0.4d §26.14.2: the word-move mark is on left and right only, and
// the key is still left or right.
func TestWordMoveMark(t *testing.T) {
	cases := []struct {
		bytes string
		key   string
		word  bool
	}{
		{csi + "1;3D", "left", true},
		{csi + "1;5D", "left", true},
		{csi + "1;6D", "left", true},
		{csi + "1;2D", "left", false},
		{csi + "D", "left", false},
		{csi + "1;3C", "right", true},
		{csi + "1;5C", "right", true},
		{csi + "57417;3u", "left", true},
		{csi + "57418;5u", "right", true},
		{csi + "57418;2u", "right", false},
		{csi + "1;3A", "up", false},
		{esc + "OD", "left", false},
	}
	for _, c := range cases {
		ins := DecodeInputLang([]byte(c.bytes), true)
		if len(ins) != 1 || ins[0].Key.Name != c.key || ins[0].Key.Word != c.word {
			t.Errorf("%q: %+v, want %s word=%v", c.bytes, ins, c.key, c.word)
		}
	}
}

// SPEC v0.4d §26.14.2: an alt key has no rune, so it is never typed.
func TestAltKeysHaveNoRune(t *testing.T) {
	for _, b := range []string{esc + "x", esc + "7", csi + "97;3u", csi + "97;4u"} {
		ins := DecodeInputLang([]byte(b), true)
		if len(ins) != 1 || ins[0].Key.Rune != 0 || !strings.HasPrefix(ins[0].Key.Name, "alt+") {
			t.Errorf("%q: %+v", b, ins)
		}
	}
	// ESC O, ESC P, and ESC X open SS3, DCS, and SOS, so alt+O, alt+P, and
	// alt+X never come out of legacy bytes.
	for _, b := range []string{esc + "OA", esc + "P1$r\x1b\\", esc + "Xjunk\x1b\\"} {
		for _, in := range DecodeInputLang([]byte(b), true) {
			if strings.HasPrefix(in.Key.Name, "alt+") {
				t.Errorf("%q: %+v", b, in)
			}
		}
	}
}

// SPEC v0.4d §26.14.2 and §26.8: a pinned document keeps tuimark 0.4.4's
// decoder, byte for byte.
func TestPinnedDecoderIsUnchanged(t *testing.T) {
	cases := []struct {
		bytes string
		want  []string
	}{
		{esc + "x", []string{"esc", "x"}},
		{esc + "\r", []string{"esc", "enter"}},
		{esc + "\x7f", []string{"esc", "backspace"}},
		{csi + "3~", []string{"delete"}},
		{esc + "OP", nil},
		{csi + "11~", nil},
		{csi + "13;2u", nil},
		{csi + "27u", nil},
		{csi + "1;5D", []string{"left"}},
		{csi + "24~", nil},
		{"\n", []string{"enter"}},
	}
	for _, c := range cases {
		var got []string
		for _, in := range DecodeInput([]byte(c.bytes)) {
			got = append(got, in.Key.Name)
			if in.Key.Word || in.Key.LF {
				t.Errorf("%q: pinned key %+v carries a mark", c.bytes, in.Key)
			}
		}
		if fmt.Sprint(got) != fmt.Sprint(c.want) {
			t.Errorf("%q: got %v, want %v", c.bytes, got, c.want)
		}
	}
}

// SPEC v0.4d §26.14.2, decision 218: ESC and a letter arrive in two reads
// within the esc timeout and are one alt key; a lone ESC is esc once the
// timeout decides it.
func TestAltAcrossReads(t *testing.T) {
	d := decoder{cur: true}
	ins, _ := d.feed([]byte(esc), false)
	if len(ins) != 0 || !d.escPending() {
		t.Fatalf("ESC alone: %+v pending=%v", ins, d.escPending())
	}
	ins, _ = d.feed([]byte("x"), false)
	if len(ins) != 1 || ins[0].Key.Name != "alt+x" {
		t.Fatalf("ESC then x: %+v", ins)
	}
	// A sequence cut in two: the rest completes it.
	ins, _ = d.feed([]byte(csi+"97;"), false)
	if len(ins) != 0 {
		t.Fatalf("cut CSI-u: %+v", ins)
	}
	ins, _ = d.feed([]byte("3u"), false)
	if len(ins) != 1 || ins[0].Key.Name != "alt+a" {
		t.Fatalf("rest of the CSI-u: %+v", ins)
	}
	// A lone ESC the timeout decided is esc.
	ins, _ = d.feed([]byte(esc), false)
	if len(ins) != 0 {
		t.Fatal(ins)
	}
	if fl := d.flush(); len(fl) != 1 || fl[0].Key.Name != "esc" {
		t.Fatalf("flush: %+v", fl)
	}
}

// SPEC v0.4d §26.14.5: under tuimark run (f12), every key read as f12 is
// the notice, modifiers included; other F keys are keys.
func TestF12NoticeCurrentLanguage(t *testing.T) {
	d := decoder{cur: true, f12: true}
	for _, b := range []string{csi + "24~", csi + "24;2~", csi + "24;5~"} {
		ins, _ := d.feed([]byte(b), true)
		if len(ins) != 1 || !ins[0].IsF12 {
			t.Errorf("%q: %+v, want the F12 notice", b, ins)
		}
	}
	ins, _ := d.feed([]byte(csi+"23~"+csi+"15~"), true)
	if len(ins) != 2 || ins[0].IsF12 || ins[0].Key.Name != "f11" || ins[1].Key.Name != "f5" {
		t.Errorf("other F keys: %+v", ins)
	}
	// A pinned decoder with f12 keeps the exact sequence only.
	p := decoder{f12: true}
	if ins, _ := p.feed([]byte(csi+"24~"), true); len(ins) != 1 || !ins[0].IsF12 {
		t.Errorf("pinned CSI 24 ~: %+v", ins)
	}
	if ins, _ := p.feed([]byte(csi+"24;2~"), true); len(ins) != 0 {
		t.Errorf("pinned CSI 24;2 ~: %+v", ins)
	}
}

// SPEC v0.4d §26.14.1: CSI ? FLAGS u is a reply, consumed as DA1 is.
func TestKittyReplyIsConsumed(t *testing.T) {
	for _, cur := range []bool{false, true} {
		d := decoder{cur: cur}
		ins, reps := d.feed([]byte(csi+"?1u"+csi+"?64;1c"), true)
		if len(ins) != 0 || len(reps) != 2 || !reps[0].kitty || !reps[1].da1 {
			t.Errorf("cur=%v: ins %+v reps %+v", cur, ins, reps)
		}
		ins, reps = d.feed([]byte(csi+"?u"), true)
		if len(ins) != 0 || len(reps) != 1 || !reps[0].kitty {
			t.Errorf("cur=%v: CSI ? u: ins %+v reps %+v", cur, ins, reps)
		}
	}
}

// Fuzz-style sweep: the decoder never panics, and CSI-u and
// modifyOtherKeys reports give a key of the current language or nothing.
func TestDecoderCSIUValid(t *testing.T) {
	for code := 0; code < 300; code++ {
		for mods := 0; mods < 260; mods += 1 {
			for _, form := range []string{"%s%d;%du", "%s27;%d;%d~"} {
				var b string
				if strings.HasSuffix(form, "u") {
					b = fmt.Sprintf(form, csi, code, mods)
				} else {
					b = fmt.Sprintf(form, csi, mods, code)
				}
				for _, in := range DecodeInputLang([]byte(b), true) {
					if !validDecodedKey(in.Key) {
						t.Fatalf("%q decoded to the invalid key %+v", b, in.Key)
					}
				}
			}
		}
	}
}

// validDecodedKey reports whether k is a key the decoder of the current
// language may produce: a token of SPEC §8.1, or a typed printable ASCII
// character (the comma included, which no keymap can bind).
func validDecodedKey(k Key) bool {
	if ir.ValidKeyLang(k.Name, ir.LangCurrent) {
		return true
	}
	return len(k.Name) == 1 && k.Rune == rune(k.Name[0]) && k.Name[0] > ' ' && k.Name[0] < 0x7f
}
