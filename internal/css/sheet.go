package css

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// Decl is one `prop: value` declaration.
type Decl struct {
	Prop, Value string
	Line, Col   int
	// File is the diagnostic file for a declaration that carries its own
	// (a hint or style="" converted from ir.Prop by the host, which has no
	// Sheet of its own); Cascade.Compute falls back to it when a rule has
	// no sheet file. A declaration that comes from a parsed *Sheet is
	// already attributed through that Sheet's File, so this is normally
	// left empty there.
	File string
}

// Media is a one-feature media condition.
type Media struct {
	Feature string // max-cols | min-cols | max-rows | min-rows
	N       int
}

// Matches reports whether the condition holds for a terminal size.
func (m *Media) Matches(cols, rows int) bool {
	if m == nil {
		return true
	}
	switch m.Feature {
	case "max-cols":
		return cols <= m.N
	case "min-cols":
		return cols >= m.N
	case "max-rows":
		return rows <= m.N
	case "min-rows":
		return rows >= m.N
	}
	return false
}

func (m *Media) String() string {
	if m == nil {
		return ""
	}
	return fmt.Sprintf("(%s: %d)", m.Feature, m.N)
}

// Rule is one selector block, possibly inside @media.
type Rule struct {
	Selectors []Selector
	Decls     []Decl
	Media     *Media
	Line      int
}

// Sheet is a parsed stylesheet.
type Sheet struct {
	File  string
	Rules []Rule
}

type sheetParser struct {
	src   string
	pos   int
	line  int
	col   int
	file  string
	diags ir.Diags
}

func (p *sheetParser) eof() bool { return p.pos >= len(p.src) }

func (p *sheetParser) advance(n int) {
	end := p.pos + n
	if end > len(p.src) {
		end = len(p.src)
	}
	for p.pos < end {
		r, size := utf8.DecodeRuneInString(p.src[p.pos:])
		p.pos += size
		if r == '\n' {
			p.line++
			p.col = 1
		} else {
			p.col++
		}
	}
}

func (p *sheetParser) errAt(line, col int, format string, args ...any) {
	p.diags = append(p.diags, ir.Diagnostic{
		Severity: ir.Error, Code: "V003", Msg: fmt.Sprintf(format, args...),
		File: p.file, Line: line, Col: col,
	})
}

// skip whitespace and /* comments */.
func (p *sheetParser) skip() {
	for !p.eof() {
		c := p.src[p.pos]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			p.advance(1)
			continue
		}
		if strings.HasPrefix(p.src[p.pos:], "/*") {
			end := strings.Index(p.src[p.pos+2:], "*/")
			if end < 0 {
				p.errAt(p.line, p.col, "unterminated comment")
				p.advance(len(p.src) - p.pos)
				return
			}
			p.advance(end + 4)
			continue
		}
		return
	}
}

// readUntil returns text up to (not including) one of the stop bytes,
// with comments removed.
func (p *sheetParser) readUntil(stops string) string {
	var b strings.Builder
	for !p.eof() {
		if strings.HasPrefix(p.src[p.pos:], "/*") {
			end := strings.Index(p.src[p.pos+2:], "*/")
			if end < 0 {
				p.errAt(p.line, p.col, "unterminated comment")
				p.advance(len(p.src) - p.pos)
				break
			}
			p.advance(end + 4)
			b.WriteByte(' ')
			continue
		}
		c := p.src[p.pos]
		if strings.IndexByte(stops, c) >= 0 {
			break
		}
		b.WriteByte(c)
		p.advance(1)
	}
	return b.String()
}

// ParseSheet parses TCSS source. Diagnostics use code V003.
func ParseSheet(src, file string) (*Sheet, ir.Diags) {
	// A leading UTF-8 BOM is common from Windows editors; internal/parse's
	// XML reader already strips one, so a .tcss file (which has no such
	// reader in front of it) should not choke on one either.
	src = strings.TrimPrefix(src, "\ufeff")
	return parseSheetAt(src, file, 1, 1)
}

// parseSheetAt parses source that starts at a given document position
// (inline <style> bodies report positions inside the .tui file).
func parseSheetAt(src, file string, line, col int) (*Sheet, ir.Diags) {
	p := &sheetParser{src: strings.ReplaceAll(src, "\r\n", "\n"), file: file, line: line, col: col}
	sheet := &Sheet{File: file}
	p.rules(sheet, nil)
	return sheet, p.diags
}

// ParseInlineSheet parses a <style> body located at line:col of file.
func ParseInlineSheet(src, file string, line, col int) (*Sheet, ir.Diags) {
	return parseSheetAt(src, file, line, col)
}

func (p *sheetParser) rules(sheet *Sheet, media *Media) {
	for {
		p.skip()
		if p.eof() {
			if media != nil {
				p.errAt(p.line, p.col, "unterminated @media block")
			}
			return
		}
		if p.src[p.pos] == '}' {
			if media != nil {
				p.advance(1)
				return
			}
			p.errAt(p.line, p.col, "unexpected '}'")
			p.advance(1)
			continue
		}
		if p.src[p.pos] == '@' {
			p.atRule(sheet, media)
			continue
		}
		line, col := p.line, p.col
		selText := p.readUntil("{};")
		if p.eof() || p.src[p.pos] != '{' {
			p.errAt(line, col, "expected '{' after selector %q", strings.TrimSpace(selText))
			if !p.eof() {
				p.advance(1)
			}
			continue
		}
		p.advance(1)
		sels, err := ParseSelectorList(selText)
		decls := p.block()
		if err != nil {
			p.errAt(line, col, "%v", err)
			continue
		}
		root := false
		for _, s := range sels {
			if s.Root {
				root = true
			}
		}
		if root && len(sels) > 1 {
			p.errAt(line, col, ":root must be the only selector in its rule")
			continue
		}
		var kept []Decl
		for _, d := range decls {
			if root {
				if !strings.HasPrefix(d.Prop, "--") {
					p.errAt(d.Line, d.Col, "only --token definitions are allowed in :root (got %q)", d.Prop)
					continue
				}
				name := strings.TrimPrefix(d.Prop, "--")
				if !isTokenName(name) {
					p.errAt(d.Line, d.Col, "bad token name %q", d.Prop)
					continue
				}
				if _, err := ParseLiteralColor(d.Value); err != nil {
					p.errAt(d.Line, d.Col, "token %s: %v", d.Prop, err)
					continue
				}
			} else if err := CheckDecl(d.Prop, d.Value); err != nil {
				p.errAt(d.Line, d.Col, "%v", err)
				continue
			}
			kept = append(kept, d)
		}
		sheet.Rules = append(sheet.Rules, Rule{Selectors: sels, Decls: kept, Media: media, Line: line})
	}
}

// block parses declarations up to the closing '}'.
func (p *sheetParser) block() []Decl {
	var out []Decl
	for {
		p.skip()
		if p.eof() {
			p.errAt(p.line, p.col, "unterminated declaration block")
			return out
		}
		switch p.src[p.pos] {
		case '}':
			p.advance(1)
			return out
		case ';':
			p.advance(1)
			continue
		}
		line, col := p.line, p.col
		text := p.readUntil(";{}")
		if !p.eof() && p.src[p.pos] == '{' {
			p.errAt(line, col, "nested rules are not supported")
			// Skip the nested block.
			depth := 0
			for !p.eof() {
				c := p.src[p.pos]
				p.advance(1)
				if c == '{' {
					depth++
				} else if c == '}' {
					depth--
					if depth == 0 {
						break
					}
				}
			}
			continue
		}
		colon := strings.IndexByte(text, ':')
		if colon < 0 {
			p.errAt(line, col, "expected 'property: value', got %q", strings.TrimSpace(text))
			continue
		}
		prop := strings.TrimSpace(text[:colon])
		val := strings.TrimSpace(text[colon+1:])
		if strings.Contains(val, "!important") {
			p.errAt(line, col, "!important is not supported")
			continue
		}
		out = append(out, Decl{Prop: prop, Value: val, Line: line, Col: col})
	}
}

func (p *sheetParser) atRule(sheet *Sheet, outer *Media) {
	line, col := p.line, p.col
	head := p.readUntil("{;")
	if p.eof() || p.src[p.pos] != '{' {
		p.errAt(line, col, "unsupported at-rule %q", strings.TrimSpace(head))
		if !p.eof() {
			p.advance(1)
		}
		return
	}
	p.advance(1)
	head = strings.TrimSpace(head)
	media, err := parseMedia(head)
	if err == nil && outer != nil {
		err = fmt.Errorf("nested @media is not supported")
	}
	if err != nil {
		p.errAt(line, col, "%v", err)
		// Parse and drop the block so later rules still load.
		tmp := &Sheet{}
		p.rules(tmp, &Media{})
		return
	}
	p.rules(sheet, media)
}

func parseMedia(head string) (*Media, error) {
	if !strings.HasPrefix(head, "@media") {
		name := strings.Fields(head)
		n := head
		if len(name) > 0 {
			n = name[0]
		}
		return nil, fmt.Errorf("unsupported at-rule %s (only @media)", n)
	}
	cond := strings.TrimSpace(strings.TrimPrefix(head, "@media"))
	if strings.Contains(cond, " and ") || strings.Contains(cond, " or ") || strings.Contains(cond, ",") || strings.Count(cond, "(") != 1 {
		return nil, fmt.Errorf("@media %s: exactly one feature per block (write two blocks instead of and/or)", cond)
	}
	if !strings.HasPrefix(cond, "(") || !strings.HasSuffix(cond, ")") {
		return nil, fmt.Errorf("@media %s: want (feature: N)", cond)
	}
	inner := cond[1 : len(cond)-1]
	colon := strings.IndexByte(inner, ':')
	if colon < 0 {
		return nil, fmt.Errorf("@media %s: want (feature: N)", cond)
	}
	feat := strings.TrimSpace(inner[:colon])
	switch feat {
	case "max-cols", "min-cols", "max-rows", "min-rows":
	default:
		return nil, fmt.Errorf("@media feature %q is not supported (max-cols, min-cols, max-rows, min-rows)", feat)
	}
	n, err := strconv.Atoi(strings.TrimSpace(inner[colon+1:]))
	if err != nil || n < 0 {
		return nil, fmt.Errorf("@media %s: value must be a whole number of cells", cond)
	}
	return &Media{Feature: feat, N: n}, nil
}

// ParseDeclsAll parses a style="" attribute body, the same way a
// stylesheet rule's declaration block recovers from a bad declaration
// (sheetParser.block): each `prop: value` is checked on its own, an
// invalid one is dropped and reported, and every valid one is kept. So one
// typo in style="a: 1; b: 2; c: 3" does not also drop the good b and c.
func ParseDeclsAll(src string) ([]Decl, []error) {
	var out []Decl
	var errs []error
	for _, part := range strings.Split(src, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		colon := strings.IndexByte(part, ':')
		if colon < 0 {
			errs = append(errs, fmt.Errorf("style: expected 'property: value', got %q", part))
			continue
		}
		d := Decl{Prop: strings.TrimSpace(part[:colon]), Value: strings.TrimSpace(part[colon+1:])}
		if err := CheckDecl(d.Prop, d.Value); err != nil {
			errs = append(errs, err)
			continue
		}
		out = append(out, d)
	}
	return out, errs
}

// ParseDecls parses a style="" attribute body, stopping at the first bad
// declaration (kept for callers that only want a single pass/fail result).
// Prefer ParseDeclsAll for a caller that should recover per declaration.
func ParseDecls(src string) ([]Decl, error) {
	decls, errs := ParseDeclsAll(src)
	if len(errs) > 0 {
		return nil, errs[0]
	}
	return decls, nil
}
