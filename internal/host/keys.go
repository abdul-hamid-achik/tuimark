package host

import (
	"bytes"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Input is one unit of terminal input the decoder produces (SPEC v0.2
// §26.8): a key (§8.1 tokens), a bracketed paste (§26.7), or, in 0.2b, a
// mouse event decoded from an SGR report (§26.8, §8.5). Terminal replies
// (OSC, DCS, SOS, PM, APC, DA1, DA2, DECRPM, CPR) and X10 mouse reports
// are consumed and produce nothing.
type Input struct {
	Key     Key    // the key, when IsPaste and IsMouse are false
	IsPaste bool   // a bracketed paste
	Paste   string // the paste payload as received (at most maxPaste bytes, not normalized; HandlePaste normalizes it)
	IsMouse bool   // a mouse event (SPEC v0.2b §26.8)
	Mouse   Mouse  // the mouse event, when IsMouse is set
}

// Decoder limits (SPEC v0.2 §26.7, §26.8).
const (
	// maxCSI is the most bytes of a CSI sequence the decoder keeps, ESC [
	// and final byte included. When that many bytes hold no final byte,
	// the decoder stops buffering and discards the rest of the sequence up
	// to its final byte (discardCSI), however the reads cut it: such a
	// sequence is neither a key nor a reply. It is large enough for any
	// DA1 reply.
	maxCSI = 256
	// maxStringSeq is the most bytes of an OSC, DCS, SOS, PM, or APC
	// sequence the decoder keeps, introducer and terminator included. When
	// that many bytes hold no terminator, the decoder stops buffering and
	// discards the rest up to the terminator (discardOSC, discardST).
	maxStringSeq = 4096
	// maxPaste is how much of a paste payload is kept (1 MiB); the rest is
	// discarded up to the end marker.
	maxPaste = 1 << 20
)

// The bracketed paste markers, CSI 200 ~ and CSI 201 ~.
var (
	pasteStart = []byte("\x1b[200~")
	pasteEnd   = []byte("\x1b[201~")
)

// discardMode is what the decoder is discarding, in constant memory, once
// a sequence outgrew its bound (SPEC v0.2 §26.8).
type discardMode uint8

const (
	discardNone discardMode = iota
	discardOSC              // an OSC: up to BEL or ST
	discardST               // a DCS, SOS, PM, or APC: up to ST
	discardCSI              // a CSI: parameter and intermediate bytes up to the final byte
)

// DecodeKeys splits complete raw-mode terminal input into key tokens: a
// lone ESC at the end is the esc key, and an incomplete trailing sequence
// (a cut UTF-8 rune, an unterminated CSI/SS3/OSC, an open paste) is
// dropped. Pastes, terminal replies, and mouse reports produce no keys.
func DecodeKeys(b []byte) []Key {
	var keys []Key
	for _, in := range DecodeInput(b) {
		if !in.IsPaste && !in.IsMouse {
			keys = append(keys, in.Key)
		}
	}
	return keys
}

// DecodeInput decodes complete raw-mode terminal input into keys, pastes,
// and (SPEC v0.2b) mouse events from SGR reports, as Run decodes one read
// that the esc timeout then ends (SPEC v0.2 §26.8): a lone ESC at the end
// is the esc key, and any other incomplete trailing sequence, including a
// paste without its end marker, is dropped. `tuimark play` decodes its
// text: steps with it.
func DecodeInput(b []byte) []Input {
	var d decoder
	ins, _ := d.feed(b, true)
	return ins
}

// reply is a terminal reply the capability probe reads (SPEC v0.2 §26.2).
type reply struct {
	da1   bool // a DA1 reply, CSI ? … c: the probe's sentinel
	mode  int  // a DECRPM reply, CSI ? Pd ; Ps $ y: Pd
	value int  // and Ps
	// bg is the theme of an OSC 11 background reply, ESC ] 11 ; SPEC
	// (dark or light; "" for a SPEC that records nothing), SPEC v0.2b
	// §26.2. osc11 marks such a reply.
	osc11 bool
	bg    string
}

// decoder is the stateful input decoder behind Run's loop. A read may cut
// a UTF-8 rune, an escape sequence, or a paste end marker in two: the
// decoder keeps the incomplete tail and the next read completes it. A
// paste may span any number of reads; it ends only at its end marker. A
// sequence that outgrows its bound (maxCSI, maxStringSeq) is discarded up
// to its end without buffering.
type decoder struct {
	tail    []byte // undecoded bytes: an incomplete sequence, bytes held by guard, or in a paste a possible start of the end marker
	pasting bool   // between CSI 200 ~ and CSI 201 ~
	paste   []byte // the payload so far, at most maxPaste bytes

	discard    discardMode // discarding the rest of an over-long sequence
	discardEsc bool        // discarding a string whose last byte was ESC: a \ next is its ST

	// guard is the rest of the paste start marker, set when the esc
	// timeout decided a pending start of it (a lone ESC, ESC [, ESC [ 2,
	// ESC [ 20, ESC [ 200). When the next read brings the rest, the paste
	// starts there; bytes that are only a start of the rest are held in
	// tail until the next read or the esc timeout.
	guard []byte
}

// escPending reports whether the esc timeout must decide the decoder's
// state when nothing follows in time: an escape sequence waiting for the
// rest of its bytes, the rest of an over-long sequence being discarded, or
// bytes held by the paste marker guard. Never inside a paste.
func (d *decoder) escPending() bool {
	if d.pasting {
		return false
	}
	return d.discard != discardNone || (len(d.tail) > 0 && (d.tail[0] == 0x1b || d.guard != nil))
}

// flush decides the decoder's state as it stands, once the esc timeout
// expired: a lone ESC is the esc key; a pending CSI, SS3, OSC, DCS, SOS,
// PM, APC, or X10 report is dropped whole, producing no key; the discard
// of an over-long sequence ends; bytes held by the guard decode as usual.
// When the tail decided was a start of the paste start marker, the next
// read is guarded: if it brings the rest of the marker, a paste starts
// (SPEC v0.2 §26.7). An open paste stays open.
func (d *decoder) flush() []Input {
	if d.pasting {
		return nil
	}
	var guard []byte
	if d.guard == nil && len(d.tail) > 0 && len(d.tail) < len(pasteStart) && bytes.HasPrefix(pasteStart, d.tail) {
		guard = pasteStart[len(d.tail):]
	}
	d.guard = nil
	ins, _ := d.feed(nil, true)
	d.guard = guard
	return ins
}

// feed decodes b after the undecoded tail of earlier reads and returns the
// keys and pastes, in order, and the replies the probe reads. With final
// set (the input ended, or the esc timeout expired), nothing is kept: a
// lone ESC is the esc key, any other incomplete sequence is dropped, and a
// discard ends; a paste stays open, since only its end marker ends it.
func (d *decoder) feed(b []byte, final bool) (ins []Input, reps []reply) {
	data := b
	if len(d.tail) > 0 {
		data = append(d.tail, b...)
	}
	d.tail = nil
	i := 0
	if g := d.guard; g != nil {
		switch {
		case bytes.HasPrefix(data, g):
			i, d.pasting = len(g), true
			d.guard = nil
		case !final && len(data) < len(g) && bytes.HasPrefix(g, data):
			d.tail = append([]byte(nil), data...) // held: the next read may complete it
			return nil, nil
		default:
			d.guard = nil
		}
	}
	for i < len(data) {
		if d.pasting {
			n, done := d.pasteChunk(data[i:], final)
			i += n
			if !done {
				break
			}
			ins = append(ins, Input{IsPaste: true, Paste: string(d.paste)})
			d.pasting, d.paste = false, nil
			continue
		}
		if d.discard != discardNone {
			i += d.discardChunk(data[i:])
			continue
		}
		u := scanInput(data[i:])
		if u.n == 0 { // incomplete: the next read may complete it
			if !final {
				break
			}
			if data[i] == 0x1b && i+1 == len(data) {
				ins = append(ins, Input{Key: Key{Name: "esc"}})
			}
			i = len(data)
			break
		}
		i += u.n
		switch u.kind {
		case unitKey:
			ins = append(ins, Input{Key: u.key})
		case unitMouse:
			ins = append(ins, Input{IsMouse: true, Mouse: u.mouse})
		case unitReply:
			reps = append(reps, u.reply)
		case unitPaste:
			d.pasting = true
		}
		if u.discard != discardNone {
			// The bound cut the sequence: discard the rest. A string whose
			// kept bytes end in ESC may be one \ away from its ST.
			d.discard = u.discard
			d.discardEsc = u.discard != discardCSI && data[i-1] == 0x1b
		}
	}
	if i < len(data) {
		d.tail = append([]byte(nil), data[i:]...)
	}
	if final {
		d.discard, d.discardEsc = discardNone, false
	}
	return ins, reps
}

// discardChunk discards the rest of an over-long sequence at the start of
// p and returns how many bytes it consumed; the discard ends when the
// sequence does. A string ends at its terminator, consumed too: BEL or ST
// (ESC \) for an OSC, ST for a DCS, SOS, PM, or APC; an ESC not followed by
// \ is part of the string, as within the bound. A CSI goes on through
// parameter and intermediate bytes (0x20-0x3F) and ends at its final byte
// (0x40-0x7E), consumed too; any other byte ends it unconsumed and then
// decodes on its own, as for a CSI within the bound.
func (d *decoder) discardChunk(p []byte) int {
	if d.discard == discardCSI {
		for k, c := range p {
			if c >= 0x20 && c <= 0x3f {
				continue
			}
			d.discard = discardNone
			if c >= 0x40 && c <= 0x7e {
				return k + 1
			}
			return k
		}
		return len(p)
	}
	for k, c := range p {
		if d.discardEsc {
			d.discardEsc = false
			if c == '\\' {
				d.discard = discardNone
				return k + 1
			}
		}
		switch {
		case c == 0x1b:
			d.discardEsc = true
		case c == 0x07 && d.discard == discardOSC:
			d.discard = discardNone
			return k + 1
		}
	}
	return len(p)
}

// pasteChunk adds the paste payload at the start of p, up to the end
// marker, and reports how many bytes it consumed and whether it consumed
// the marker. Without the marker it consumes all of p except a trailing
// possible start of the marker, which the next read may complete (all of p
// when final).
func (d *decoder) pasteChunk(p []byte, final bool) (int, bool) {
	if k := bytes.Index(p, pasteEnd); k >= 0 {
		d.addPaste(p[:k])
		return k + len(pasteEnd), true
	}
	keep := 0
	if !final {
		for k := min(len(p), len(pasteEnd)-1); k > 0; k-- {
			if bytes.HasPrefix(pasteEnd, p[len(p)-k:]) {
				keep = k
				break
			}
		}
	}
	d.addPaste(p[:len(p)-keep])
	return len(p) - keep, false
}

// addPaste appends payload bytes while the payload is under maxPaste.
func (d *decoder) addPaste(p []byte) {
	if room := maxPaste - len(d.paste); room > 0 {
		d.paste = append(d.paste, p[:min(len(p), room)]...)
	}
}

type unitKind uint8

const (
	unitNone  unitKind = iota // consumed, produces nothing
	unitKey                   // a key
	unitReply                 // a reply the probe reads
	unitPaste                 // CSI 200 ~: a paste starts
	unitMouse                 // an SGR mouse report that gives an event (v0.2b)
)

// unit is one decoded item at the start of the input.
type unit struct {
	kind    unitKind
	n       int // bytes consumed; 0 means incomplete (wait for more input)
	key     Key
	mouse   Mouse
	reply   reply
	discard discardMode // the bound cut the sequence: discard its rest
}

func keyUnit(name string, r rune, n int) unit {
	return unit{kind: unitKey, n: n, key: Key{Name: name, Rune: r}}
}

// scanInput decodes the item at the start of p (len(p) > 0).
func scanInput(p []byte) unit {
	c := p[0]
	switch {
	case c == 0x1b:
		return scanEscape(p)
	case c == '\r' || c == '\n':
		return keyUnit("enter", 0, 1)
	case c == '\t':
		return keyUnit("tab", 0, 1)
	case c == 0x7f:
		return keyUnit("backspace", 0, 1)
	case c == 0:
		return unit{n: 1}
	case c < 0x20:
		return keyUnit("ctrl+"+string(rune('a'+c-1)), 0, 1)
	case c == ' ':
		return keyUnit("space", ' ', 1)
	case c < 0x80:
		return keyUnit(string(rune(c)), rune(c), 1)
	}
	if !utf8.FullRune(p) {
		return unit{}
	}
	r, size := utf8.DecodeRune(p)
	if r == utf8.RuneError && size <= 1 {
		return unit{n: 1} // invalid UTF-8: dropped
	}
	return keyUnit(string(r), r, size)
}

// scanEscape decodes the item at the start of p, which starts with ESC.
func scanEscape(p []byte) unit {
	if len(p) < 2 {
		return unit{}
	}
	switch p[1] {
	case '[':
		return scanCSI(p)
	case 'O':
		if len(p) < 3 {
			return unit{}
		}
		if name := ss3Key(p[2]); name != "" {
			return keyUnit(name, 0, 3)
		}
		return unit{n: 3}
	case ']':
		return scanString(p, true)
	case 'P', 'X', '^', '_':
		return scanString(p, false)
	}
	// ESC followed by anything else is the esc key; the next byte decodes
	// on its own (alt+x is esc then x).
	return keyUnit("esc", 0, 1)
}

func ss3Key(b byte) string {
	switch b {
	case 'A':
		return "up"
	case 'B':
		return "down"
	case 'C':
		return "right"
	case 'D':
		return "left"
	case 'H':
		return "home"
	case 'F':
		return "end"
	}
	return ""
}

// scanCSI decodes a CSI sequence at the start of p (ESC [). Parameter and
// intermediate bytes run up to the final byte, 0x40-0x7E. A sequence
// broken by any other byte is dropped up to that byte, which then decodes
// on its own. A sequence that reaches maxCSI bytes without its final byte
// is too long to be anything the runtime reads: those bytes are dropped
// and the rest is discarded up to the final byte (discardCSI).
func scanCSI(p []byte) unit {
	limit := min(len(p), maxCSI)
	j := 2
	for j < limit && p[j] >= 0x20 && p[j] <= 0x3f {
		j++
	}
	if j >= limit {
		if len(p) >= maxCSI {
			return unit{n: maxCSI, discard: discardCSI}
		}
		return unit{}
	}
	final := p[j]
	if final < 0x40 || final > 0x7e {
		return unit{n: j}
	}
	n := j + 1
	params := p[2:j]
	if final == 'M' && j == 2 {
		// X10 mouse report: CSI M plus exactly 3 raw bytes of any value.
		// Consumed whether or not the document uses the mouse: a click at
		// column 81 sends the byte q, which must never be the q key.
		if len(p) < 6 {
			return unit{}
		}
		return unit{n: 6}
	}
	var private byte
	if len(params) > 0 && params[0] >= '<' && params[0] <= '?' {
		private = params[0]
	}
	switch {
	case private == '?' && final == 'c':
		return unit{kind: unitReply, n: n, reply: reply{da1: true}}
	case private == '?' && final == 'y':
		if r, ok := parseDECRPM(params); ok {
			return unit{kind: unitReply, n: n, reply: r}
		}
		return unit{n: n}
	case private == '<' && (final == 'M' || final == 'm'):
		// An SGR mouse report: a mouse event or nothing, never a key
		// (SPEC v0.2b §26.8).
		if m, ok := parseSGRMouse(params[1:], final); ok {
			return unit{kind: unitMouse, n: n, mouse: m}
		}
		return unit{n: n}
	case private != 0:
		// DA2 (CSI > … c) and every other private-parameter sequence:
		// replies, never keys.
		return unit{n: n}
	case final == '~' && string(params) == "200":
		return unit{kind: unitPaste, n: n}
	}
	if name := csiKey(final, string(params)); name != "" {
		return keyUnit(name, 0, n)
	}
	// CPR (CSI … R), a paste end marker outside a paste, and any other CSI
	// whose final byte is not a key.
	return unit{n: n}
}

// parseSGRMouse decodes the parameters "Cb;Cx;Cy" of an SGR mouse report
// CSI < Cb ; Cx ; Cy M|m (SPEC v0.2b §26.8). Cb, Cx, and Cy are decimal,
// Cx and Cy 1-based. With b = Cb without its modifier bits 4, 8, and 16:
// b = 0 is the left button, a press on M and a release on m; b = 64 is
// wheel up and b = 65 wheel down, on M. Every other value (motion, bit 32;
// the middle and right buttons, 1 and 2; the horizontal wheel, 66 and 67;
// releases of the wheel) gives no event, and so does a parameter that is
// not a decimal number or a count other than three.
func parseSGRMouse(params []byte, final byte) (Mouse, bool) {
	parts := strings.Split(string(params), ";")
	if len(parts) != 3 {
		return Mouse{}, false
	}
	var v [3]int
	for i, p := range parts {
		if p == "" || len(p) > 9 {
			return Mouse{}, false
		}
		for _, c := range []byte(p) {
			if c < '0' || c > '9' {
				return Mouse{}, false
			}
		}
		v[i], _ = strconv.Atoi(p)
	}
	m := Mouse{X: v[1] - 1, Y: v[2] - 1}
	switch b := v[0] &^ (4 | 8 | 16); {
	case b == 0 && final == 'M':
		m.Kind = MousePress
	case b == 0 && final == 'm':
		m.Kind = MouseRelease
	case b == 64 && final == 'M':
		m.Kind = MouseWheelUp
	case b == 65 && final == 'M':
		m.Kind = MouseWheelDown
	default:
		return Mouse{}, false
	}
	return m, true
}

// parseDECRPM parses the parameters of a DECRPM reply, "?Pd;Ps$".
func parseDECRPM(params []byte) (reply, bool) {
	s := string(params)
	if !strings.HasPrefix(s, "?") || !strings.HasSuffix(s, "$") || len(s) < 2 {
		return reply{}, false
	}
	pd, ps, ok := strings.Cut(s[1:len(s)-1], ";")
	if !ok {
		return reply{}, false
	}
	mode, err := strconv.Atoi(pd)
	if err != nil || mode <= 0 {
		return reply{}, false
	}
	value, err := strconv.Atoi(ps)
	if err != nil {
		return reply{}, false
	}
	return reply{mode: mode, value: value}, true
}

// csiKey names the key a CSI sequence encodes, or "" for none. Modifier
// parameters on arrows (CSI 1 ; 5 A) still decode as the plain key.
func csiKey(final byte, params string) string {
	switch final {
	case 'A':
		return "up"
	case 'B':
		return "down"
	case 'C':
		return "right"
	case 'D':
		return "left"
	case 'H':
		return "home"
	case 'F':
		return "end"
	case 'Z':
		return "shift+tab"
	case '~':
		switch params {
		case "1", "7":
			return "home"
		case "4", "8":
			return "end"
		case "5":
			return "pgup"
		case "6":
			return "pgdn"
		case "3":
			return "delete"
		}
	}
	return ""
}

// scanString consumes an OSC (osc set; it ends at BEL or ST, ESC \) or a
// DCS, SOS, PM, or APC (it ends at ST) at the start of p; an ESC not
// followed by \ is part of the string. The decoder keeps at most
// maxStringSeq bytes of it, terminator included: when that many bytes hold
// no terminator they are dropped, producing no key, and the rest of the
// string is discarded up to its terminator (discardOSC, discardST).
func scanString(p []byte, osc bool) unit {
	limit := min(len(p), maxStringSeq)
	for k := 2; k < limit; k++ {
		if osc && p[k] == 0x07 {
			return oscUnit(p[2:k], k+1)
		}
		if p[k] == 0x1b && k+1 < limit && p[k+1] == '\\' {
			if osc {
				return oscUnit(p[2:k], k+2)
			}
			return unit{n: k + 2}
		}
	}
	if len(p) >= maxStringSeq {
		mode := discardST
		if osc {
			mode = discardOSC
		}
		return unit{n: maxStringSeq, discard: mode}
	}
	return unit{}
}

// oscUnit is a complete OSC whose body (between ESC ] and its terminator)
// is body: an OSC 11 background reply, "11;SPEC", is a reply the probe
// reads (SPEC v0.2b §26.2, §26.8); any other OSC produces nothing. Either
// way it never produces a key.
func oscUnit(body []byte, n int) unit {
	spec, ok := bytes.CutPrefix(body, []byte("11;"))
	if !ok {
		return unit{n: n}
	}
	return unit{kind: unitReply, n: n, reply: reply{osc11: true, bg: ThemeFromOSC11(string(spec))}}
}

// pasteSpaces is step 3 of the paste normalization.
var pasteSpaces = strings.NewReplacer("\r", " ", "\n", " ", "\t", " ")

// NormalizePaste returns the text a bracketed paste inserts (SPEC v0.2
// §26.7). Only the first maxPaste bytes (1 MiB) of the payload count (a
// UTF-8 sequence that limit cuts is dropped with the invalid bytes); then,
// in order: (1) invalid UTF-8 bytes are dropped; (2) each CR LF pair
// becomes one LF; (3) each CR, LF, and TAB becomes a space; (4) escape
// sequences are removed whole and then the remaining C0 and C1 controls
// and DEL (stripPasteControls).
func NormalizePaste(payload string) string {
	if len(payload) > maxPaste {
		payload = payload[:maxPaste]
	}
	s := strings.ToValidUTF8(payload, "")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = pasteSpaces.Replace(s)
	return stripPasteControls(s)
}

// stripPasteControls is step 4 of the paste normalization (SPEC v0.2
// §26.7): every escape sequence is removed whole (pasteEscapeLen), then
// every remaining C0 control (U+0000-U+001F), DEL, and C1 control
// (U+0080-U+009F). A C1 code point is removed alone: it never introduces a
// sequence (a pasted U+009E is mojibake far more often than a PM). s is
// valid UTF-8.
func stripPasteControls(s string) string {
	var out strings.Builder
	out.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			i += pasteEscapeLen(s[i:])
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r >= 0x20 && r != 0x7f && (r < 0x80 || r > 0x9f) {
			out.WriteString(s[i : i+size])
		}
		i += size
	}
	return out.String()
}

// pasteEscapeLen returns the byte length of the escape sequence at the
// start of s (s[0] is ESC), as step 4 of the paste normalization removes
// it (SPEC v0.2 §26.7), with the units of the input decoder (§26.8):
//   - CSI: ESC [, parameter and intermediate bytes 0x20-0x3F, and the final
//     byte 0x40-0x7E; any other character ends it before that character.
//   - OSC: ESC ] up to and including BEL or ST (ESC \); DCS, SOS, PM, APC:
//     ESC P, ESC X, ESC ^, ESC _ up to and including ST. An ESC not
//     followed by \ is part of the string.
//   - SS3: ESC O and the character after it, when that is 0x20-0x7E.
//   - Any other escape: ESC, intermediate bytes 0x20-0x2F, and a final
//     byte 0x30-0x7E; any other character ends it before that character.
//
// A sequence the payload ends inside runs to the end. An ESC followed by
// a control, a non-ASCII character, or nothing is removed alone.
func pasteEscapeLen(s string) int {
	if len(s) < 2 {
		return len(s)
	}
	c := s[1]
	switch {
	case c == '[':
		j := 2
		for j < len(s) && s[j] >= 0x20 && s[j] <= 0x3f {
			j++
		}
		if j < len(s) && s[j] >= 0x40 && s[j] <= 0x7e {
			j++
		}
		return j
	case c == ']' || c == 'P' || c == 'X' || c == '^' || c == '_':
		for j := 2; j < len(s); j++ {
			if c == ']' && s[j] == 0x07 {
				return j + 1
			}
			if s[j] == 0x1b && j+1 < len(s) && s[j+1] == '\\' {
				return j + 2
			}
		}
		return len(s)
	case c == 'O':
		if len(s) > 2 && s[2] >= 0x20 && s[2] <= 0x7e {
			return 3
		}
		return 2
	case c >= 0x20 && c <= 0x7e:
		j := 1
		for j < len(s) && s[j] >= 0x20 && s[j] <= 0x2f {
			j++
		}
		if j < len(s) && s[j] >= 0x30 && s[j] <= 0x7e {
			j++
		}
		return j
	}
	return 1
}

// HandlePaste delivers a bracketed paste (SPEC v0.2 §26.7) and returns the
// events to dispatch. When the focused node is an enabled input, the
// normalized text (NormalizePaste) is inserted at its cursor as one edit,
// through the same path as coalesced typing: the value changes once and,
// when the input has on:change, exactly one change event carries the
// whole new value. An empty normalized text changes nothing. Otherwise
// (nothing focused, another kind of node focused, a disabled input) the
// paste is discarded: it never reaches the keymap, a widget, or a
// built-in, so pasting a q cannot quit. Caller must not hold a.mu.
func (a *App) HandlePaste(payload string) []Event {
	text := NormalizePaste(payload)
	a.mu.Lock()
	defer a.mu.Unlock()
	b := a.focusedBox()
	if text == "" || b == nil || b.Disabled || b.Kind != "input" {
		return nil
	}
	return a.insertText(b, []rune(text))
}
