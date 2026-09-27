package host

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

func keyNames(ks []Key) string {
	var out []string
	for _, k := range ks {
		out = append(out, k.Name)
	}
	return strings.Join(out, " ")
}

// inputNames renders inputs as key names, and a paste as paste(%q).
func inputNames(ins []Input) string {
	var out []string
	for _, in := range ins {
		if in.IsPaste {
			out = append(out, fmt.Sprintf("paste(%q)", in.Paste))
			continue
		}
		if in.IsMouse {
			// An SGR report is a mouse event since 0.2b (SPEC §26.8).
			out = append(out, fmt.Sprintf("mouse(%s@%d,%d)", in.Mouse.Kind, in.Mouse.X, in.Mouse.Y))
			continue
		}
		out = append(out, in.Key.Name)
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
		{"unknown csi ignored", "\x1b[99mx", "x"},
		{"unknown tilde ignored", "\x1b[99~x", "x"},
		{"incomplete csi dropped", "\x1b[1;", ""},
		{"incomplete ss3 dropped", "\x1bO", ""},
		{"mixed", "ab\x1b[Bc\r", "a b down c enter"},
		// SPEC v0.2 §26.8: replies and mouse reports are never keys.
		{"osc bel", "a\x1b]11;rgb:0000/0000/0000\x07b", "a b"},
		{"osc st", "a\x1b]11;rgb:ffff/ffff/ffff\x1b\\b", "a b"},
		{"osc with q and esc-less text", "\x1b]0;quit q\x07q", "q"},
		{"dcs", "\x1bP>|xterm(390)\x1b\\x", "x"},
		{"dcs does not end at bel", "\x1bPa\x07q\x1b\\z", "z"},
		{"sos", "\x1bXq\x1b\\z", "z"},
		{"pm", "\x1b^q\x1b\\z", "z"},
		{"apc", "\x1b_Gi=1;OK\x1b\\z", "z"},
		{"da1", "\x1b[?62;22c", ""},
		{"da1 long", "\x1b[?64;1;2;6;9;15;16;17;18;21;22;28;29c", ""},
		{"da2", "\x1b[>1;10;0c", ""},
		{"decrpm", "\x1b[?2026;2$y\x1b[?2027;0$y", ""},
		{"cpr", "\x1b[12;40R", ""},
		{"sgr mouse press", "\x1b[<0;81;5M", ""},
		{"sgr mouse release", "\x1b[<0;81;5m", ""},
		{"x10 mouse with q", "\x1b[M q%x", "x"},
		{"x10 mouse raw bytes", "\x1b[M\xff\x1b\x03y", "y"},
		{"stray paste end", "\x1b[201~z", "z"},
		{"private csi is not a key", "\x1b[?1A\x1b[>5~", ""},
		{"broken csi resumes at the byte", "\x1b[1\x01", "ctrl+a"},
		{"csi broken by esc", "\x1b[12\x1b[A", "up"},
		// A paste is not a key; one without its end marker is dropped.
		{"paste", "a\x1b[200~q\x1b[201~b", "a b"},
		{"open paste dropped", "\x1b[200~x", ""},
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

// feedSplit decodes in as reads cut at the given offsets (ascending),
// the last read final.
func feedSplit(in string, cuts ...int) string {
	var d decoder
	var out []Input
	prev := 0
	for _, c := range cuts {
		ins, _ := d.feed([]byte(in[prev:c]), false)
		out = append(out, ins...)
		prev = c
	}
	ins, _ := d.feed([]byte(in[prev:]), true)
	return inputNames(append(out, ins...))
}

// Round 2, finding 4, and SPEC v0.2 §21 test 31: a read that cuts a UTF-8
// rune, an escape sequence, a reply, a mouse report, or a paste marker in
// two loses neither half. For every split point, and byte by byte,
// decoding gives the inputs of the whole.
func TestDecodeKeysAcrossReads(t *testing.T) {
	inputs := []string{
		"aé€😀b",
		"xy\x1b[Az",
		"\x1b[1;5A\x1b[5~\x1bOB",
		"é\x1b[Bñ\x1b\x1b[C",
		"\x1bx",       // alt+x stays esc then x
		"\x1b\x1b[D€", // esc, then left
		"a\x1b]11;rgb:1e1e/1e1e/2e2e\x1b\\b\x1b]0;t\x07c",
		"q\x1bP>|WezTerm 2024\x1b\\q\x1b_Gi=31;OK\x1b\\q",
		"\x1b[?2026;2$y\x1b[?2027;1$yk\x1b[?62;22c",
		"\x1b[>1;4000;15cj\x1b[3;7Rk",
		"\x1b[<0;81;5Mz\x1b[<64;3;3m",
		"\x1b[M q%x\x1b[Mqqqy",
		"a\x1b[200~p\r\nq\x1b[201~b",
		"\x1b[200~\x1b[20\x1b[201\x1b[201~",
		"\x1b[200~x\x1b[201~\x1b[200~y\x1b[201~",
	}
	for _, in := range inputs {
		want := feedSplit(in)
		for cut := 0; cut <= len(in); cut++ {
			if got := feedSplit(in, cut); got != want {
				t.Errorf("%q cut at %d: %q, want %q", in, cut, got, want)
			}
		}
		var every []int
		for i := 1; i < len(in); i++ {
			every = append(every, i)
		}
		if got := feedSplit(in, every...); got != want {
			t.Errorf("%q byte by byte: %q, want %q", in, got, want)
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
		{"a\x1b]11;rgb:00", "\x1b]11;rgb:00"},
		{"a\x1b]11;x\x1b", "\x1b]11;x\x1b"},
		{"a\x1bPq", "\x1bPq"},
		{"a\x1b[Mq", "\x1b[Mq"},
		{"a\x1b[?62;2", "\x1b[?62;2"},
		{"a\xff", ""},                               // invalid, not incomplete
		{"a\xe2x", ""},                              // invalid lead: dropped, x decoded
		{"a\x1b[1;5A", ""},                          // complete
		{"\x1b[" + strings.Repeat("1", maxCSI), ""}, // too long: nothing buffered, the rest is discarded
		{"\x1b[" + strings.Repeat("1", maxCSI-3), "\x1b[" + strings.Repeat("1", maxCSI-3)},
		{"\x1b]" + strings.Repeat("q", maxStringSeq), ""}, // likewise for a string sequence
	} {
		var d decoder
		d.feed([]byte(c.in), false)
		if string(d.tail) != c.rest {
			t.Errorf("feed(%q) tail = %q, want %q", c.in, d.tail, c.rest)
		}
	}
	// A CSI that reaches maxCSI bytes without a final byte is discarded up
	// to its final byte, however the reads cut it (finding 13): none of
	// its bytes is a key; decoding resumes after the final byte.
	long := "\x1b[" + strings.Repeat("1", maxCSI) + "Az"
	for _, cut := range []int{0, 10, maxCSI - 1, maxCSI, maxCSI + 1, maxCSI + 2} {
		if got := feedSplit(long, cut); got != "z" {
			t.Errorf("long CSI cut at %d: %q", cut, got)
		}
	}
}

// §21 test 31: an unterminated OSC (or DCS, SOS, PM, APC) produces no key:
// past 4096 bytes nothing more is buffered and the rest is discarded up to
// its terminator (finding 12), or until the esc timeout expires or the
// input ends; decoding then resumes with the next byte.
func TestDecoderStringBounds(t *testing.T) {
	junk := strings.Repeat("q", maxStringSeq-2) // with ESC ], exactly 4096 bytes
	for _, intro := range []string{"\x1b]", "\x1bP", "\x1bX", "\x1b^", "\x1b_"} {
		in := intro + junk + "ab"
		if got := feedSplit(in); got != "" {
			t.Errorf("%q…: %q, want no key from the bytes after 4096", intro, got)
		}
		if got := feedSplit(in, 1000, 3000, 4095, 4096, 4097); got != "" {
			t.Errorf("%q… across reads: %q", intro, got)
		}
		if got := feedSplit(in + "\x1b\\ab"); got != "a b" {
			t.Errorf("%q… terminated after 4096 bytes: %q, want the bytes after ST decoded", intro, got)
		}
		// Terminated within 4096 bytes: consumed whole.
		ok := intro + junk[:len(junk)-2] + "\x1b\\ab"
		if len(ok) != maxStringSeq+2 {
			t.Fatalf("len %d", len(ok))
		}
		if got := feedSplit(ok, 2048); got != "a b" {
			t.Errorf("%q… terminated at 4096: %q", intro, got)
		}
		// The esc timeout drops a pending one whole.
		var d decoder
		ins, _ := d.feed([]byte("x"+intro+"quit q"), false)
		if inputNames(ins) != "x" || !d.escPending() {
			t.Fatalf("%q: %q, pending %v", intro, inputNames(ins), d.escPending())
		}
		if got := inputNames(d.flush()); got != "" {
			t.Errorf("%q: the esc timeout decoded %q", intro, got)
		}
		ins, _ = d.feed([]byte("y"), false)
		if inputNames(ins) != "y" || len(d.tail) != 0 {
			t.Errorf("%q: after the timeout %q, tail %q", intro, inputNames(ins), d.tail)
		}
	}
	// A pending X10 report is dropped whole by the esc timeout too.
	var d decoder
	d.feed([]byte("\x1b[Mq"), false)
	if got := inputNames(d.flush()); got != "" {
		t.Errorf("X10: the esc timeout decoded %q", got)
	}
	// A lone ESC is still the esc key.
	d.feed([]byte("\x1b"), false)
	if got := inputNames(d.flush()); got != "esc" {
		t.Errorf("lone ESC flushed as %q", got)
	}
}

// §21 test 31 and §26.2: DA1 and DECRPM replies reach the probe, in order,
// and never produce keys.
func TestDecoderReplies(t *testing.T) {
	var d decoder
	ins, reps := d.feed([]byte("a\x1b[?2026;2$yb\x1b[?2027;4$y\x1b[?64;1;2c\x1b[?1;$y\x1b[?2027$yc"), false)
	if got := inputNames(ins); got != "a b c" {
		t.Errorf("keys = %q", got)
	}
	want := []reply{{mode: 2026, value: 2}, {mode: 2027, value: 4}, {da1: true}}
	if fmt.Sprint(reps) != fmt.Sprint(want) {
		t.Errorf("replies = %+v, want %+v", reps, want)
	}
	var c termCaps
	for _, r := range append(reps, reply{mode: 2026, value: 0}) {
		c.record(r)
	}
	if !c.done || c.sync != 2 || c.grapheme != 4 {
		t.Errorf("caps = %+v (a reply after DA1 must be ignored)", c)
	}
	if !c.syncSupported() || c.turnGraphemeOn() || c.graphemeActive() {
		t.Errorf("caps interpretation = %+v", c)
	}
	for ps, want := range map[int][2]bool{0: {false, false}, 1: {false, true}, 2: {true, true}, 3: {false, true}, 4: {false, false}} {
		c := termCaps{grapheme: ps}
		if c.turnGraphemeOn() != want[0] || c.graphemeActive() != want[1] {
			t.Errorf("2027 Ps=%d: on %v active %v", ps, c.turnGraphemeOn(), c.graphemeActive())
		}
		c = termCaps{sync: ps}
		if c.syncSupported() != (ps == 1 || ps == 2) {
			t.Errorf("2026 Ps=%d: sync %v", ps, c.syncSupported())
		}
	}
}

// §26.7: a paste is the bytes between the markers, across reads, never
// keys; it ends only at its end marker (the esc timeout does not end it);
// the payload keeps 1 MiB and drops the rest up to the marker.
func TestDecoderPaste(t *testing.T) {
	var d decoder
	ins, _ := d.feed([]byte("a\x1b[200~q\r\n\x1b[A\x1b"), false)
	if got := inputNames(ins); got != "a" || !d.pasting || d.escPending() {
		t.Fatalf("open paste: %q pasting %v escPending %v", got, d.pasting, d.escPending())
	}
	if got := inputNames(d.flush()); got != "" || !d.pasting {
		t.Fatalf("the esc timeout ended a paste: %q", got)
	}
	ins, _ = d.feed([]byte("[20"), false)
	if len(ins) != 0 || string(d.tail) != "\x1b[20" {
		t.Fatalf("marker start: %q tail %q", inputNames(ins), d.tail)
	}
	ins, _ = d.feed([]byte("1~b"), false)
	if got := inputNames(ins); got != `paste("q\r\n\x1b[A") b` {
		t.Errorf("paste = %s", got)
	}
	// A false start of the marker is payload.
	ins, _ = d.feed([]byte("\x1b[200~x\x1b[20"), false)
	ins2, _ := d.feed([]byte("0~y\x1b[201~"), false)
	if got := inputNames(append(ins, ins2...)); got != `paste("x\x1b[200~y")` {
		t.Errorf("false start = %s", got)
	}
	// The 1 MiB cap: the rest is discarded up to the end marker.
	big := strings.Repeat("é", maxPaste/2) // exactly maxPaste bytes
	var d2 decoder
	ins, _ = d2.feed([]byte("\x1b[200~"+big[:1000]), false)
	ins2, _ = d2.feed([]byte(big[1000:]+"zzzz"), false)
	ins3, _ := d2.feed([]byte("zz\x1b[201~k"), false)
	all := append(append(ins, ins2...), ins3...)
	if len(all) != 2 || !all[0].IsPaste || all[0].Paste != big || all[1].Key.Name != "k" {
		t.Errorf("capped paste: %d inputs, paste %d bytes", len(all), len(all[0].Paste))
	}
	// A cap that cuts a UTF-8 sequence: the cut bytes are dropped by the
	// normalization.
	odd := "a" + big
	if got := NormalizePaste(odd); len(got) != maxPaste-1 || !strings.HasPrefix(got, "aé") {
		t.Errorf("cut rune: %d bytes", len(got))
	}
}

func TestNormalizePaste(t *testing.T) {
	cases := []struct{ in, want string }{
		{"hello", "hello"},
		{"a\nb", "a b"},
		{"a\r\nb", "a b"},
		{"a\r\n\r\nb", "a  b"},
		{"a\rb\tc", "a b c"},
		{"a\x1b[31mred\x1b[0m", "ared"},
		{"x\x1b]0;title\x07y", "xy"},
		{"q\x03\x7f\x00z", "qz"},
		{"c1\u0085\u009bend", "c1end"},
		{"bad\xff\xfeutf8", "badutf8"},
		{"\r\n", " "},
		{"\x1b", ""},
		{"👍🏽 é", "👍🏽 é"},
	}
	for _, c := range cases {
		if got := NormalizePaste(c.in); got != c.want {
			t.Errorf("NormalizePaste(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// FuzzDecoder: the decoder never panics, a read boundary never changes
// what it decodes, and replies wrapped around keys never add keys.
func FuzzDecoder(f *testing.F) {
	for _, s := range []string{
		"ab\x1b[A\x1b]11;rgb:0/0/0\x07\x1b[?2026;2$y\x1b[?62c",
		"\x1b[M q%\x1b[<0;1;1Mq\x1bP1$r0m\x1b\\",
		"\x1b[200~paste\r\n\x1b[201~z\x1b[201~",
		"\x1b\x1b\x1bO\x1b[1;5A\xe2\x82\xac\xff",
		"\x1b_" + strings.Repeat("x", 5000),
	} {
		f.Add(s, uint16(3))
	}
	f.Fuzz(func(t *testing.T, s string, cut uint16) {
		if len(s) > 1<<14 {
			return
		}
		whole := feedSplit(s)
		c := int(cut) % (len(s) + 1)
		if got := feedSplit(s, c); got != whole {
			t.Fatalf("%q cut at %d: %q, whole %q", s, c, got, whole)
		}
		// The same input with replies between every decoded unit decodes
		// to the same inputs, when the input does not end inside a
		// sequence that would swallow them.
		var d decoder
		ins, _ := d.feed([]byte(s), true)
		for _, in := range ins {
			if !in.IsPaste && !in.IsMouse && in.Key.Name == "" {
				t.Fatalf("empty key from %q", s)
			}
		}
		noise := "\x1b[?2027;2$y\x1b]11;rgb:1/2/3\x1b\\\x1b[M q#\x1b[<35;9;9M\x1b[?64;1c"
		if got := feedSplit(noise + s); got != whole {
			t.Fatalf("replies before %q changed it: %q, want %q", s, got, whole)
		}
	})
}

// BenchmarkBracketedPaste decodes a 2 MiB paste in terminal-sized reads
// (half of it past the 1 MiB cap) and normalizes the kept payload: linear
// in the input, whatever the read size.
func BenchmarkBracketedPaste(b *testing.B) {
	payload := []byte("\x1b[200~" + strings.Repeat("lorem ipsum\r\ndolor\tsit é ", 2<<20/24) + "\x1b[201~")
	for i := 0; i < b.N; i++ {
		var d decoder
		var got []Input
		for off := 0; off < len(payload); off += 4096 {
			ins, _ := d.feed(payload[off:min(off+4096, len(payload))], false)
			got = append(got, ins...)
		}
		if len(got) != 1 || len(got[0].Paste) != maxPaste {
			b.Fatalf("%d inputs", len(got))
		}
		_ = NormalizePaste(got[0].Paste)
	}
}

// Finding 12 (SPEC v0.2 §26.8 bounds): an OSC, DCS, SOS, PM, or APC longer
// than 4096 bytes keeps at most 4096 bytes, then is discarded up to its
// terminator (BEL or ST for OSC, ST for the others), in constant memory,
// however the reads cut it: no byte of it ever becomes a key, and the key
// after the terminator still decodes.
func TestDecoderLongStringDiscarded(t *testing.T) {
	long := strings.Repeat("q", 5000)
	cases := []struct{ name, in string }{
		{"osc 52 bel", "\x1b]52;c;" + long + "\x07z"},
		{"osc 52 st", "\x1b]52;c;" + long + "\x1b\\z"},
		{"osc base64", "\x1b]52;c;" + strings.Repeat("cXVpdCBx", 750) + "\x07z"},
		{"dcs st", "\x1bP" + long + "\x1b\\z"},
		{"dcs ignores bel", "\x1bP" + long + "\x07q\x1b\\z"},
		{"sos", "\x1bX" + long + "\x1b\\z"},
		{"pm", "\x1b^" + long + "\x1b\\z"},
		{"apc", "\x1b_G" + long + "\x1b\\z"},
		{"esc inside the discarded run", "\x1b]" + long + "\x1b[Aq\x1b\x1b\\z"},
		// ST split by the 4096-byte bound: ESC is byte 4096, \ byte 4097.
		{"st across the bound", "\x1b_" + strings.Repeat("q", maxStringSeq-3) + "\x1b\\z"},
		{"bel right after the bound", "\x1b]" + strings.Repeat("q", maxStringSeq-2) + "\x07z"},
	}
	for _, c := range cases {
		if got := feedSplit(c.in); got != "z" {
			t.Errorf("%s: %q, want only z", c.name, got)
		}
		for _, cut := range []int{1, 1000, maxStringSeq - 1, maxStringSeq, maxStringSeq + 1, len(c.in) - 3, len(c.in) - 2, len(c.in) - 1} {
			if got := feedSplit(c.in, cut); got != "z" {
				t.Errorf("%s cut at %d: %q, want only z", c.name, cut, got)
			}
		}
		var several []int
		for _, cut := range []int{700, 2100, 4095, 4097, 4200} {
			if cut < len(c.in) {
				several = append(several, cut)
			}
		}
		if got := feedSplit(c.in, several...); got != "z" {
			t.Errorf("%s in several reads: %q", c.name, got)
		}
	}
	// Nothing past the bound is buffered.
	var d decoder
	ins, _ := d.feed([]byte("x\x1b]52;c;"+long), false)
	if inputNames(ins) != "x" || len(d.tail) != 0 || !d.escPending() {
		t.Fatalf("discarding: %q, tail %d bytes, pending %v", inputNames(ins), len(d.tail), d.escPending())
	}
	// The esc timeout ends a discarded string like a pending one.
	if got := inputNames(d.flush()); got != "" {
		t.Errorf("the esc timeout decoded %q", got)
	}
	ins, _ = d.feed([]byte("y"), false)
	if got := inputNames(ins); got != "y" {
		t.Errorf("after the timeout: %q", got)
	}
	// Input that ends inside a discarded string (DecodeInput) produces
	// nothing more.
	if got := keyNames(DecodeKeys([]byte("a\x1bP" + long))); got != "a" {
		t.Errorf("unterminated long DCS: %q", got)
	}
}

// Finding 13 (SPEC v0.2 §26.8 bounds): a CSI keeps at most 256 bytes;
// past that it is discarded up to its final byte (consumed too), however
// the reads cut it. Its parameter bytes and final never become keys; such a
// CSI is neither a key nor a reply.
func TestDecoderLongCSIDiscarded(t *testing.T) {
	da1 := "\x1b[?" + strings.Repeat("1;", 150) + "c" // 304 bytes
	for _, cuts := range [][]int{nil, {100}, {maxCSI - 1}, {maxCSI}, {maxCSI + 1}, {100, 200, 300}} {
		var d decoder
		var ins []Input
		var reps []reply
		prev := 0
		in := da1 + "z"
		for _, c := range append(cuts, len(in)) {
			i, r := d.feed([]byte(in[prev:c]), c == len(in))
			ins, reps = append(ins, i...), append(reps, r...)
			prev = c
		}
		if got := inputNames(ins); got != "z" || len(reps) != 0 {
			t.Errorf("304-byte DA1 cut at %v: keys %q replies %+v, want only z", cuts, got, reps)
		}
	}
	// The bound is exact: 256 bytes is still a reply, 257 is discarded.
	var d decoder
	_, reps := d.feed([]byte("\x1b[?"+strings.Repeat("1", maxCSI-4)+"c"), false)
	if len(reps) != 1 || !reps[0].da1 {
		t.Errorf("256-byte DA1: replies %+v", reps)
	}
	ins, reps := d.feed([]byte("\x1b[?"+strings.Repeat("1", maxCSI-3)+"cz"), false)
	if inputNames(ins) != "z" || len(reps) != 0 {
		t.Errorf("257-byte DA1: %q %+v", inputNames(ins), reps)
	}
	// Any byte outside 0x20-0x7E ends the discarded CSI and decodes on its
	// own, as it does for a short broken CSI.
	if got := feedSplit("\x1b[" + strings.Repeat("1", 300) + "\x01z"); got != "ctrl+a z" {
		t.Errorf("broken long CSI: %q", got)
	}
	if got := feedSplit("\x1b["+strings.Repeat("1", 300)+"\x1b[Az", 290); got != "up z" {
		t.Errorf("long CSI broken by ESC: %q", got)
	}
	// The esc timeout ends it too; nothing is buffered meanwhile.
	d = decoder{}
	ins, _ = d.feed([]byte("\x1b["+strings.Repeat("1", 300)), false)
	if len(ins) != 0 || len(d.tail) != 0 || !d.escPending() {
		t.Fatalf("discarding CSI: %q tail %d pending %v", inputNames(ins), len(d.tail), d.escPending())
	}
	if got := inputNames(d.flush()); got != "" {
		t.Errorf("esc timeout: %q", got)
	}
	ins, _ = d.feed([]byte("A"), false)
	if got := inputNames(ins); got != "A" {
		t.Errorf("after the timeout: %q", got)
	}
}

// Finding 14 (SPEC v0.2 §26.7, §26.8): the esc timeout decides a pending
// sequence (a lone ESC is esc, anything else is dropped), but when that
// sequence was the start of the paste start marker CSI 200 ~ and the next
// read brings the rest of the marker, the paste starts there: the payload
// never becomes keys, so its q cannot quit.
func TestDecoderPasteMarkerSplitByTimeout(t *testing.T) {
	const marker = "\x1b[200~"
	for k := 1; k < len(marker); k++ {
		var d decoder
		ins, _ := d.feed([]byte("a"+marker[:k]), false)
		if inputNames(ins) != "a" || !d.escPending() {
			t.Fatalf("%q: %q pending %v", marker[:k], inputNames(ins), d.escPending())
		}
		want := ""
		if k == 1 {
			want = "esc" // a lone ESC is the esc key at the timeout
		}
		if got := inputNames(d.flush()); got != want {
			t.Errorf("%q flushed as %q, want %q", marker[:k], got, want)
		}
		ins, _ = d.feed([]byte(marker[k:]+"abc xq\x1b[201~z"), false)
		if got := inputNames(ins); got != `paste("abc xq") z` {
			t.Errorf("%q | timeout | rest: %s", marker[:k], got)
		}
		// The rest of the marker may itself come in pieces, each within
		// the esc timeout.
		d = decoder{}
		d.feed([]byte(marker[:k]), false)
		d.flush()
		rest := marker[k:] + "q\x1b[201~"
		var all []Input
		for i := 0; i < len(rest); i++ {
			ins, _ := d.feed([]byte(rest[i:i+1]), false)
			all = append(all, ins...)
		}
		if got := inputNames(all); got != `paste("q")` {
			t.Errorf("%q | timeout | rest byte by byte: %s", marker[:k], got)
		}
	}
	// Anything else after the timeout decodes as usual.
	for _, c := range []struct{ before, after, want string }{
		{"\x1b", "q", "q"},
		{"\x1b", "[A", "[ A"},
		{"\x1b[2", "x", "x"},
		{"\x1b[20", "0x", "0 x"},
		{"\x1b", "\x1b[A", "up"},
		{"\x1b[1;5", "A", "A"}, // not the paste marker: dropped at the timeout
	} {
		var d decoder
		d.feed([]byte(c.before), false)
		d.flush()
		ins, _ := d.feed([]byte(c.after), false)
		ins = append(ins, d.flush()...)
		if got := inputNames(ins); got != c.want {
			t.Errorf("%q | timeout | %q: %q, want %q", c.before, c.after, got, c.want)
		}
	}
	// A prefix of the rest is held until the next read or the esc timeout,
	// which decodes it as keys.
	var d decoder
	d.feed([]byte("\x1b"), false)
	d.flush()
	ins, _ := d.feed([]byte("[2"), false)
	if len(ins) != 0 || !d.escPending() {
		t.Fatalf("held rest: %q pending %v", inputNames(ins), d.escPending())
	}
	if got := inputNames(d.flush()); got != "[ 2" {
		t.Errorf("held rest at the timeout: %q", got)
	}
	// The guard covers only the read right after the timeout.
	d = decoder{}
	d.feed([]byte("\x1b"), false)
	d.flush()
	d.feed([]byte("a"), false)
	if ins, _ = d.feed([]byte("[200~q"), false); inputNames(ins) != "[ 2 0 0 ~ q" {
		t.Errorf("a later read: %q", inputNames(ins))
	}
}

// Finding 15 (SPEC v0.2 §26.7 step 4): whole escape sequences leave a
// paste, string sequences included (DCS, SOS, PM, APC bodies do not stay
// as text); C1 controls are removed one by one, never as introducers.
func TestNormalizePasteSequences(t *testing.T) {
	cases := []struct{ in, want string }{
		{"a\x1bPq#0;2;0;0;0\x1b\\b", "ab"},
		{"a\x1b_Gpayload\x1b\\b", "ab"},
		{"a\x1bXsos\x1b\\b", "ab"},
		{"a\x1b^pm\x1b\\b", "ab"},
		{"a\x1b]0;t\x1b\\b", "ab"},
		{"a\x1b]0;t\x07b", "ab"},
		{"a\x1bPdcs\x07still\x1b\\b", "ab"}, // BEL ends only an OSC
		{"a\x1b]0;x\x1b[31my\x07b", "ab"},   // an ESC inside a string is part of it
		{"a\x1bPno terminator", "a"},
		{"a\x1b[12", "a"},
		{"a\x1b[1;2\x01b", "ab"},   // a CSI broken by another character ends before it
		{"a\x1b[1é", "aé"},         // ... which then stands on its own
		{"a\x1b[1\x1b[31mb", "ab"}, // an ESC starts a new sequence
		{"a\x1b[?25lb", "ab"},
		{"a\x1b(Bb", "ab"}, // ESC, intermediates, final
		{"a\x1b#8b", "ab"}, // DECALN
		{"a\x1b7b\x1b8", "ab"},
		{"a\x1bOAb", "ab"}, // SS3 and its character
		{"a\x1b\x1b[31mb", "ab"},
		{"a\x1bé", "aé"}, // a lone ESC: the next character stays
		{"a\x1b", "a"},
		{"a\x1b\x03b", "ab"},
		{"a\u0090dcs\u009cb", "adcsb"}, // 8-bit C1: removed one by one
		{"c1\u0085\u009bend", "c1end"},
		{"a\x1b]0;x\r\ny\x07b", "ab"}, // CR LF became spaces first, inside the OSC
	}
	for _, c := range cases {
		if got := NormalizePaste(c.in); got != c.want {
			t.Errorf("NormalizePaste(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// FuzzNormalizePaste: the normalized text is valid UTF-8 without any C0
// or C1 control, DEL, or ESC, and normalizing it again changes nothing.
func FuzzNormalizePaste(f *testing.F) {
	for _, s := range []string{"a\x1bPq\x1b\\b", "x\x1b[1;2\x01y\r\n\t", "\u009b\u0090\x1b]0;t\x07é", "\x1b(B\x1bOA\x1b\x1b"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		got := NormalizePaste(s)
		if !utf8.ValidString(got) {
			t.Fatalf("%q: invalid UTF-8 %q", s, got)
		}
		for _, r := range got {
			if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
				t.Fatalf("%q: control %U left in %q", s, r, got)
			}
		}
		if again := NormalizePaste(got); again != got {
			t.Fatalf("%q: not idempotent: %q then %q", s, got, again)
		}
	})
}
