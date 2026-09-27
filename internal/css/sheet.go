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
	// Attr is the presentational attribute as written (`pad="1"`) for a
	// declaration the host converted from one; "" otherwise.
	Attr string
}

// Media is a one-feature media condition.
type Media struct {
	Feature string // max-cols | min-cols | max-rows | min-rows | theme (version="2")
	N       int
	Value   string // the theme feature's value: dark | light
	// Text is the condition as written, trimmed: "(max-cols: 80)".
	Text string
}

// Matches reports whether the condition holds for a terminal size. A
// theme condition never holds here: use MatchesEnv.
func (m *Media) Matches(cols, rows int) bool {
	return m.MatchesEnv(Env{Cols: cols, Rows: rows})
}

// MatchesEnv reports whether the condition holds for a frame's size and
// effective theme (SPEC §10.5: theme matches when the frame's effective
// theme, dark or light, equals the value).
func (m *Media) MatchesEnv(env Env) bool {
	if m == nil {
		return true
	}
	switch m.Feature {
	case "max-cols":
		return env.Cols <= m.N
	case "min-cols":
		return env.Cols >= m.N
	case "max-rows":
		return env.Rows <= m.N
	case "min-rows":
		return env.Rows >= m.N
	case "theme":
		return EffectiveTheme(env.Theme) == m.Value
	}
	return false
}

// IsTheme reports whether m is a theme condition.
func (m *Media) IsTheme() bool { return m != nil && m.Feature == "theme" }

func (m *Media) String() string {
	if m == nil {
		return ""
	}
	if m.Feature == "theme" {
		return fmt.Sprintf("(theme: %s)", m.Value)
	}
	return fmt.Sprintf("(%s: %d)", m.Feature, m.N)
}

// Rule is one selector block, possibly inside @media.
type Rule struct {
	Selectors []Selector
	// Text is the selector list as written: its selectors, each trimmed,
	// joined by ", " (`tuimark inspect` reports it, SPEC §15.7).
	Text  string
	Decls []Decl
	Media *Media
	Line  int
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
	v2    bool // the document that loads the sheet is version="2" (SPEC §5.1)
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

// ParseSheet parses TCSS source loaded by a version="1" document.
// Diagnostics use code V003.
func ParseSheet(src, file string) (*Sheet, ir.Diags) { return ParseSheetIn(src, file, false) }

// ParseSheetIn parses TCSS source for a document of the given version: a
// stylesheet has no version of its own and is checked against the version
// of the document that loads it (SPEC §5.1).
func ParseSheetIn(src, file string, v2 bool) (*Sheet, ir.Diags) {
	// A leading UTF-8 BOM is common from Windows editors; internal/parse's
	// XML reader already strips one, so a .tcss file (which has no such
	// reader in front of it) should not choke on one either.
	src = strings.TrimPrefix(src, "\ufeff")
	return parseSheetAt(src, file, 1, 1, v2)
}

// parseSheetAt parses source that starts at a given document position
// (inline <style> bodies report positions inside the .tui file).
func parseSheetAt(src, file string, line, col int, v2 bool) (*Sheet, ir.Diags) {
	p := &sheetParser{src: strings.ReplaceAll(src, "\r\n", "\n"), file: file, line: line, col: col, v2: v2}
	sheet := &Sheet{File: file}
	p.rules(sheet, nil)
	return sheet, p.diags
}

// ParseInlineSheet parses a <style> body of a version="1" document located
// at line:col of file.
func ParseInlineSheet(src, file string, line, col int) (*Sheet, ir.Diags) {
	return parseSheetAt(src, file, line, col, false)
}

// ParseInlineSheetIn is ParseInlineSheet for a document of the given
// version.
func ParseInlineSheetIn(src, file string, line, col int, v2 bool) (*Sheet, ir.Diags) {
	return parseSheetAt(src, file, line, col, v2)
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
		sels, err := ParseSelectorListIn(selText, p.v2)
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
			} else if err := CheckDeclIn(d.Prop, d.Value, p.v2); err != nil {
				p.errAt(d.Line, d.Col, "%v", err)
				continue
			}
			kept = append(kept, d)
		}
		texts := make([]string, len(sels))
		for i, s := range sels {
			texts[i] = s.Text
		}
		sheet.Rules = append(sheet.Rules, Rule{Selectors: sels, Text: strings.Join(texts, ", "), Decls: kept, Media: media, Line: line})
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
	media, err := parseMedia(head, p.v2)
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

func parseMedia(head string, v2 bool) (*Media, error) {
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
	case "theme":
		// SPEC §10.5: version="2" only; the value is dark or light, never
		// auto (auto is never an effective theme).
		if !v2 {
			return nil, fmt.Errorf("@media feature %q%s", feat, ir.VersionHint)
		}
		val := strings.TrimSpace(inner[colon+1:])
		if val != "dark" && val != "light" {
			return nil, fmt.Errorf("@media %s: the theme feature takes dark or light", cond)
		}
		return &Media{Feature: feat, Value: val, Text: cond}, nil
	default:
		if v2 {
			return nil, fmt.Errorf("@media feature %q is not supported (max-cols, min-cols, max-rows, min-rows, theme)", feat)
		}
		return nil, fmt.Errorf("@media feature %q is not supported (max-cols, min-cols, max-rows, min-rows)", feat)
	}
	n, err := strconv.Atoi(strings.TrimSpace(inner[colon+1:]))
	if err != nil || n < 0 {
		return nil, fmt.Errorf("@media %s: value must be a whole number of cells", cond)
	}
	return &Media{Feature: feat, N: n, Text: cond}, nil
}

// ParseDeclsAll parses a style="" attribute body, the same way a
// stylesheet rule's declaration block recovers from a bad declaration
// (sheetParser.block): each `prop: value` is checked on its own, an
// invalid one is dropped and reported, and every valid one is kept. So one
// typo in style="a: 1; b: 2; c: 3" does not also drop the good b and c.
// The declarations are checked for a version="1" document.
func ParseDeclsAll(src string) ([]Decl, []error) { return ParseDeclsAllIn(src, false) }

// ParseDeclsAllIn is ParseDeclsAll for a document of the given version.
func ParseDeclsAllIn(src string, v2 bool) ([]Decl, []error) {
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
		if err := CheckDeclIn(d.Prop, d.Value, v2); err != nil {
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
