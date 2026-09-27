// Package parse turns a .tui document into the source IR.
//
// The XML subset (SPEC §5) is small and strict enough that a purpose-built
// tokenizer is simpler than bending encoding/xml: attribute names such as
// on:select are not namespaces, attribute values must be double-quoted, and
// every diagnostic needs an exact line and column.
package parse

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// RawType is the kind of a lossless tree node.
type RawType int

const (
	RawElement RawType = iota
	RawText
	RawComment
	RawCData
)

// RawAttr is one attribute as written.
type RawAttr struct {
	Name, Value string
	Line, Col   int
}

// RawNode is a lossless XML tree node. `tuimark fmt` prints this tree; the
// IR builder validates it.
type RawNode struct {
	Type        RawType
	Name        string
	Attrs       []RawAttr
	Children    []*RawNode
	Text        string // decoded character data, comment body, or CDATA body
	SelfClosing bool
	Line, Col   int
}

// SyntaxError is a V005 well-formedness failure.
type SyntaxError struct {
	Line, Col int
	Msg       string
}

func (e *SyntaxError) Error() string { return fmt.Sprintf("%d:%d: %s", e.Line, e.Col, e.Msg) }

type scanner struct {
	src   string
	pos   int
	line  int
	col   int
	depth int // element nesting depth (bounds O(depth) work per node)
}

// maxNestingDepth bounds the raw-tree depth. Position/Path strings and the
// recursive descent both cost O(depth) per node, so an unbounded document
// can exhaust memory long before it exhausts patience (a real .tui document
// is shallow; this is a robustness backstop, reported as V005).
const maxNestingDepth = 256

func (s *scanner) eof() bool { return s.pos >= len(s.src) }

func (s *scanner) peek() byte {
	if s.eof() {
		return 0
	}
	return s.src[s.pos]
}

func (s *scanner) has(prefix string) bool { return strings.HasPrefix(s.src[s.pos:], prefix) }

// advance moves over n bytes, keeping line/col (col counts runes).
func (s *scanner) advance(n int) {
	end := s.pos + n
	if end > len(s.src) {
		end = len(s.src)
	}
	for s.pos < end {
		r, size := utf8.DecodeRuneInString(s.src[s.pos:])
		s.pos += size
		if r == '\n' {
			s.line++
			s.col = 1
		} else {
			s.col++
		}
	}
}

func (s *scanner) errf(line, col int, format string, args ...any) *SyntaxError {
	return &SyntaxError{Line: line, Col: col, Msg: fmt.Sprintf(format, args...)}
}

func isXMLSpaceByte(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

func (s *scanner) skipSpace() {
	for !s.eof() {
		switch s.peek() {
		case ' ', '\t', '\n', '\r':
			s.advance(1)
		default:
			return
		}
	}
}

// skipSpaceHad is skipSpace that also reports whether it moved.
func (s *scanner) skipSpaceHad() bool {
	start := s.pos
	s.skipSpace()
	return s.pos > start
}

// isNameStartRune and isNameRune approximate the XML NameStartChar/NameChar
// productions with Go's Unicode tables: any letter or '_' starts a name
// (tags and attributes are further restricted to lowercase ASCII by SPEC
// §5, checked by the IR builder, not the tokenizer); combining marks,
// digits, '-', '.', ':', and the middle dot continue one.
func isNameStartRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r)
}

func isNameRune(r rune) bool {
	if isNameStartRune(r) || unicode.IsDigit(r) || r == '-' || r == '.' || r == ':' || r == 0xB7 {
		return true
	}
	return unicode.In(r, unicode.Mn, unicode.Mc)
}

func (s *scanner) name() string {
	start := s.pos
	if s.eof() {
		return ""
	}
	r, size := utf8.DecodeRuneInString(s.src[s.pos:])
	if !isNameStartRune(r) {
		return ""
	}
	s.advance(size)
	for !s.eof() {
		r, size = utf8.DecodeRuneInString(s.src[s.pos:])
		if !isNameRune(r) {
			break
		}
		s.advance(size)
	}
	return s.src[start:s.pos]
}

// ParseXML tokenizes src into a lossless tree. It returns the single root
// element plus any comments that surround it (for fmt).
func ParseXML(src []byte) (root *RawNode, prolog, epilog []*RawNode, err *SyntaxError) {
	text := string(src)
	text = strings.TrimPrefix(text, "\ufeff")
	if !utf8.ValidString(text) {
		return nil, nil, nil, &SyntaxError{Line: 1, Col: 1, Msg: "document is not valid UTF-8"}
	}
	// XML 1.0 §2.11 end-of-line handling: CRLF and a lone CR (one not
	// followed by LF) are both a single LF. CRLF goes first so it does not
	// become two line breaks.
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	s := &scanner{src: text, line: 1, col: 1}

	// Prolog: optional XML declaration, whitespace, comments.
	if s.has("<?xml") && (len(s.src) > 5 && (isXMLSpaceByte(s.src[5]) || s.src[5] == '?')) {
		if err := s.xmlDecl(); err != nil {
			return nil, nil, nil, err
		}
	}
	for {
		s.skipSpace()
		if s.has("<!--") {
			c, err := s.comment()
			if err != nil {
				return nil, nil, nil, err
			}
			prolog = append(prolog, c)
			continue
		}
		break
	}
	if s.eof() {
		return nil, nil, nil, s.errf(s.line, s.col, "document has no root element")
	}
	if s.peek() != '<' {
		return nil, nil, nil, s.errf(s.line, s.col, "text before the root element")
	}
	root, err = s.element()
	if err != nil {
		return nil, nil, nil, err
	}
	for {
		s.skipSpace()
		if s.eof() {
			break
		}
		if s.has("<!--") {
			c, err := s.comment()
			if err != nil {
				return nil, nil, nil, err
			}
			epilog = append(epilog, c)
			continue
		}
		if s.peek() == '<' {
			return nil, nil, nil, s.errf(s.line, s.col, "a document must have a single root element")
		}
		return nil, nil, nil, s.errf(s.line, s.col, "text after the root element")
	}
	return root, prolog, epilog, nil
}

// xmlDecl parses the optional declaration '<?xml' VersionInfo EncodingDecl?
// SDDecl? S? '?>' (XML 1.0 §2.8). Only version is required; SPEC §5 asks
// for UTF-8, so a declared encoding must say so. It is called only when the
// input has already matched '<?xml' followed by S or '?'.
func (s *scanner) xmlDecl() *SyntaxError {
	line, col := s.line, s.col
	if strings.Index(s.src[s.pos:], "?>") < 0 {
		return s.errf(line, col, "unterminated XML declaration")
	}
	s.advance(5) // '<?xml'
	if s.eof() || !isXMLSpaceByte(s.peek()) {
		return s.errf(line, col, "malformed XML declaration: expected whitespace and version=\"1.x\" after '<?xml'")
	}
	s.skipSpace()
	name, val, ok, err := s.xmlDeclAttr()
	if err != nil {
		return err
	}
	if !ok || name != "version" {
		return s.errf(line, col, "malformed XML declaration: expected version=\"1.x\"")
	}
	if !isVersionNum(val) {
		return s.errf(line, col, "malformed XML declaration: unsupported version %q (want 1.<digits>, such as 1.0)", val)
	}
	hadSpace := s.skipSpaceHad()
	if hadSpace && s.has("encoding") {
		if name, val, ok, err = s.xmlDeclAttr(); err != nil {
			return err
		} else if ok && name == "encoding" {
			if !strings.EqualFold(val, "utf-8") {
				return s.errf(line, col, "encoding must be UTF-8 (SPEC §5), got %q", val)
			}
			hadSpace = s.skipSpaceHad()
		}
	}
	if hadSpace && s.has("standalone") {
		if name, val, ok, err = s.xmlDeclAttr(); err != nil {
			return err
		} else if ok && name == "standalone" && val != "yes" && val != "no" {
			return s.errf(line, col, `standalone must be "yes" or "no", got %q`, val)
		}
	}
	s.skipSpace()
	if !s.has("?>") {
		return s.errf(s.line, s.col, "malformed XML declaration: expected '?>'")
	}
	s.advance(2)
	return nil
}

// isVersionNum reports whether v matches XML 1.0 §2.8
// VersionNum ::= '1.' [0-9]+.
func isVersionNum(v string) bool {
	if len(v) < 3 || v[:2] != "1." {
		return false
	}
	for i := 2; i < len(v); i++ {
		if v[i] < '0' || v[i] > '9' {
			return false
		}
	}
	return true
}

// xmlDeclAttr reads one `name="value"` (or '...') pseudo-attribute. ok is
// false (with err nil) when there is no name at the current position at
// all, so the caller can tell "nothing here" from "malformed".
func (s *scanner) xmlDeclAttr() (name, val string, ok bool, err *SyntaxError) {
	name = s.name()
	if name == "" {
		return "", "", false, nil
	}
	s.skipSpace()
	if s.peek() != '=' {
		return "", "", false, s.errf(s.line, s.col, "malformed XML declaration: expected '=' after %q", name)
	}
	s.advance(1)
	s.skipSpace()
	q := s.peek()
	if q != '"' && q != '\'' {
		return "", "", false, s.errf(s.line, s.col, "malformed XML declaration: expected a quoted value for %q", name)
	}
	s.advance(1)
	end := strings.IndexByte(s.src[s.pos:], q)
	if end < 0 {
		return "", "", false, s.errf(s.line, s.col, "malformed XML declaration: unterminated value for %q", name)
	}
	val = s.src[s.pos : s.pos+end]
	s.advance(end + 1)
	return name, val, true, nil
}

// illegalInXML reports whether r may never appear in an XML 1.0 document
// (XML 1.0 §2.2 Char), even inside a comment: a C0/C1 control other than
// tab/LF/CR, or DEL. Higher illegal code points (lone surrogates,
// U+FFFE/FFFF) cannot occur in the UTF-8 this scanner already validated.
func illegalInXML(r rune) bool {
	if r == '\t' || r == '\n' || r == '\r' {
		return false
	}
	return r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f)
}

// firstIllegalRune returns the first illegalInXML rune in s and its byte
// offset, or (0, -1) when s has none.
func firstIllegalRune(s string) (rune, int) {
	for i, r := range s {
		if illegalInXML(r) {
			return r, i
		}
	}
	return 0, -1
}

// advancePos returns the line:col reached after text, starting at line:col.
func advancePos(line, col int, text string) (int, int) {
	for _, r := range text {
		if r == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return line, col
}

func (s *scanner) comment() (*RawNode, *SyntaxError) {
	line, col := s.line, s.col
	s.advance(4)
	end := strings.Index(s.src[s.pos:], "-->")
	if end < 0 {
		return nil, s.errf(line, col, "unterminated comment")
	}
	body := s.src[s.pos : s.pos+end]
	if strings.Contains(body, "--") {
		return nil, s.errf(line, col, `"--" is not allowed inside a comment`)
	}
	if strings.HasSuffix(body, "-") {
		return nil, s.errf(line, col, "a comment may not end with '--->' (add a space before the closing '-->')")
	}
	if r, off := firstIllegalRune(body); off >= 0 {
		eline, ecol := advancePos(line, col+4, body[:off])
		return nil, s.errf(eline, ecol, "character U+%04X is not allowed in XML, even inside a comment", r)
	}
	s.advance(end + 3)
	return &RawNode{Type: RawComment, Text: body, Line: line, Col: col}, nil
}

// markupError reports the constructs the subset forbids.
func (s *scanner) markupError() *SyntaxError {
	switch {
	case s.has("<!DOCTYPE"), s.has("<!doctype"):
		return s.errf(s.line, s.col, "DTD/DOCTYPE is not allowed")
	case s.has("<?"):
		return s.errf(s.line, s.col, "processing instructions are not allowed")
	case s.has("<!"):
		return s.errf(s.line, s.col, "declarations are not allowed")
	}
	return nil
}

func (s *scanner) element() (*RawNode, *SyntaxError) {
	if err := s.markupError(); err != nil {
		return nil, err
	}
	line, col := s.line, s.col
	s.depth++
	if s.depth > maxNestingDepth {
		s.depth--
		return nil, s.errf(line, col, "elements nested deeper than %d levels", maxNestingDepth)
	}
	defer func() { s.depth-- }()
	s.advance(1) // <
	name := s.name()
	if name == "" {
		return nil, s.errf(line, col, "expected a tag name after '<'")
	}
	n := &RawNode{Type: RawElement, Name: name, Line: line, Col: col}
	seen := map[string]bool{}
	for {
		hadSpace := false
		for !s.eof() && strings.IndexByte(" \t\n\r", s.peek()) >= 0 {
			s.advance(1)
			hadSpace = true
		}
		if s.eof() {
			return nil, s.errf(line, col, "unclosed <%s> (end of input inside the start tag)", name)
		}
		if s.has("/>") {
			s.advance(2)
			n.SelfClosing = true
			return n, nil
		}
		if s.peek() == '>' {
			s.advance(1)
			break
		}
		if !hadSpace {
			return nil, s.errf(s.line, s.col, "expected whitespace before attribute in <%s>", name)
		}
		aline, acol := s.line, s.col
		an := s.name()
		if an == "" {
			return nil, s.errf(aline, acol, "bad attribute in <%s>", name)
		}
		s.skipSpace()
		if s.peek() != '=' {
			return nil, s.errf(aline, acol, "attribute %q in <%s> has no value (write %s=\"...\")", an, name, an)
		}
		s.advance(1)
		s.skipSpace()
		switch s.peek() {
		case '"':
		case '\'':
			return nil, s.errf(s.line, s.col, "attribute %q must use double quotes", an)
		default:
			return nil, s.errf(s.line, s.col, "attribute %q value must be double-quoted", an)
		}
		s.advance(1)
		end := strings.IndexByte(s.src[s.pos:], '"')
		if end < 0 {
			return nil, s.errf(aline, acol, "unterminated value for attribute %q", an)
		}
		raw := s.src[s.pos : s.pos+end]
		if strings.ContainsRune(raw, '<') {
			return nil, s.errf(aline, acol, "'<' is not allowed in attribute values (use &lt;)")
		}
		vline, vcol := s.line, s.col
		val, err := decodeEntities(raw, vline, vcol)
		if err != nil {
			return nil, err
		}
		s.advance(end + 1)
		if seen[an] {
			return nil, s.errf(aline, acol, "duplicate attribute %q in <%s>", an, name)
		}
		seen[an] = true
		n.Attrs = append(n.Attrs, RawAttr{Name: an, Value: val, Line: aline, Col: acol})
	}
	// Content.
	for {
		if s.eof() {
			return nil, s.errf(line, col, "unclosed <%s>", name)
		}
		switch {
		case s.has("</"):
			cline, ccol := s.line, s.col
			s.advance(2)
			cn := s.name()
			s.skipSpace()
			if s.peek() != '>' {
				return nil, s.errf(cline, ccol, "malformed closing tag </%s", cn)
			}
			s.advance(1)
			if cn != name {
				return nil, s.errf(cline, ccol, "closing tag </%s> does not match <%s> opened at %d:%d", cn, name, line, col)
			}
			return n, nil
		case s.has("<!--"):
			c, err := s.comment()
			if err != nil {
				return nil, err
			}
			n.Children = append(n.Children, c)
		case s.has("<![CDATA["):
			cline, ccol := s.line, s.col
			s.advance(9)
			end := strings.Index(s.src[s.pos:], "]]>")
			if end < 0 {
				return nil, s.errf(cline, ccol, "unterminated CDATA section")
			}
			n.Children = append(n.Children, &RawNode{Type: RawCData, Text: s.src[s.pos : s.pos+end], Line: cline, Col: ccol})
			s.advance(end + 3)
		case s.peek() == '<':
			child, err := s.element()
			if err != nil {
				return nil, err
			}
			n.Children = append(n.Children, child)
		default:
			tline, tcol := s.line, s.col
			end := strings.IndexByte(s.src[s.pos:], '<')
			if end < 0 {
				end = len(s.src) - s.pos
			}
			raw := s.src[s.pos : s.pos+end]
			if i := strings.Index(raw, "]]>"); i >= 0 {
				eline, ecol := advancePos(tline, tcol, raw[:i])
				return nil, s.errf(eline, ecol, "']]>' is not allowed in character data (XML 1.0 §2.4)")
			}
			txt, err := decodeEntities(raw, tline, tcol)
			if err != nil {
				return nil, err
			}
			n.Children = append(n.Children, &RawNode{Type: RawText, Text: txt, Line: tline, Col: tcol})
			s.advance(end)
		}
	}
}

var entities = map[string]string{"lt": "<", "gt": ">", "amp": "&", "quot": `"`, "apos": "'"}

// decodeEntities resolves the five predefined entities and rejects anything
// else, including numeric character references.
func decodeEntities(raw string, line, col int) (string, *SyntaxError) {
	if !strings.Contains(raw, "&") {
		return raw, nil
	}
	var b strings.Builder
	l, c := line, col
	for i := 0; i < len(raw); {
		ch := raw[i]
		if ch == '&' {
			end := strings.IndexByte(raw[i:], ';')
			if end < 0 {
				return "", &SyntaxError{Line: l, Col: c, Msg: "bare '&' (write &amp;)"}
			}
			name := raw[i+1 : i+end]
			rep, ok := entities[name]
			if !ok {
				return "", &SyntaxError{Line: l, Col: c, Msg: fmt.Sprintf("entity &%s; is not allowed (only &lt; &gt; &amp; &quot; &apos;)", name)}
			}
			b.WriteString(rep)
			i += end + 1
			c += end + 1
			continue
		}
		r, size := utf8.DecodeRuneInString(raw[i:])
		b.WriteRune(r)
		i += size
		if r == '\n' {
			l++
			c = 1
		} else {
			c++
		}
	}
	return b.String(), nil
}
