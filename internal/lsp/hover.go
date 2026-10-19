package lsp

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/abdul-hamid-achik/tuimark/internal/catalog"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// hoverResult is the result of textDocument/hover.
type hoverResult struct {
	Contents markupContent `json:"contents"`
	Range    *Range        `json:"range,omitempty"`
}

type markupContent struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

func markdown(parts ...string) markupContent {
	return markupContent{Kind: "markdown", Value: strings.Join(parts, "\n\n")}
}

var codeRE = regexp.MustCompile(`^[A-Z][0-9]{3}$`)

// hover answers textDocument/hover (SPEC §15.12 "Hover"): the catalog's
// text for the tag, attribute, property, keyword, pseudo-class, or
// diagnostic code under the cursor. The catalog (SPEC §15.10) holds no
// prose, so the text is made of its fields: a property's value syntax, an
// attribute's keywords, a tag's attributes, a diagnostic's pass and the
// situation it reports, each with the document version that introduced it.
func (s *Server) hover(sn *snapshot, doc *document, pos Position) *hoverResult {
	off := byteOffset(doc.text, pos)
	word, start, end := wordAt(doc.text, off)
	var parts []string
	if word != "" {
		switch doc.kind {
		case kindTUI:
			parts = s.hoverTUI(doc, word, start, end)
		case kindTCSS:
			parts = s.hoverTCSS(doc, word, start, end)
		}
		if codeRE.MatchString(word) {
			parts = append(parts, s.diagnosticText(word)...)
		}
	}
	// A diagnostic published on the cursor explains itself too.
	s.mu.Lock()
	published := s.lastDiags[doc.uri]
	s.mu.Unlock()
	for _, d := range published {
		if pos.Line == d.Range.Start.Line && pos.Character >= d.Range.Start.Character && pos.Character <= d.Range.End.Character {
			parts = append(parts, s.diagnosticText(d.Code)...)
		}
	}
	if len(parts) == 0 {
		return nil
	}
	return &hoverResult{Contents: markdown(parts...)}
}

func (s *Server) diagnosticText(code string) []string {
	for _, d := range s.cat.Diagnostics {
		if d.Code == code {
			return []string{fmt.Sprintf("**%s** — diagnostic (%s pass)", d.Code, d.Pass), d.When}
		}
	}
	return nil
}

func (s *Server) hoverTUI(doc *document, word string, start, end int) []string {
	ctx := lexTUI(doc.text, end)
	switch ctx.kind {
	case tuiTagName:
		if t := s.tag(strings.TrimPrefix(word, "/")); t != nil {
			return tagText(*t)
		}
	case tuiAttrName:
		if a := s.attrOf(ctx.tag, word); a != nil {
			return attrText(ctx.tag, *a)
		}
	case tuiAttrValue:
		a := s.attrOf(ctx.tag, ctx.attr)
		if a == nil {
			return nil
		}
		for _, k := range a.Keywords {
			if k.Name == word {
				return append([]string{fmt.Sprintf("`%s` — value of `%s` on `<%s>`%s", k.Name, a.Name, ctx.tag, sinceSuffix(k.Since))}, attrText(ctx.tag, *a)...)
			}
		}
		return attrText(ctx.tag, *a)
	}
	return nil
}

func sinceSuffix(since int) string {
	switch {
	case since <= 1:
		return ""
	case since >= int(ir.LangCurrent):
		return " (" + currentDetail + ")"
	}
	return fmt.Sprintf(" (version %d)", since)
}

func tagText(t catalog.Tag) []string {
	names := make([]string, 0, len(t.Attrs))
	for _, a := range t.Attrs {
		names = append(names, "`"+a.Name+"`")
	}
	return []string{
		fmt.Sprintf("`<%s>` — tag%s", t.Name, sinceSuffix(t.Since)),
		"Attributes: " + strings.Join(names, ", "),
	}
}

func attrText(tag string, a catalog.Attr) []string {
	out := []string{fmt.Sprintf("`%s` — attribute of `<%s>`%s", a.Name, tag, sinceSuffix(a.Since))}
	if len(a.Keywords) > 0 {
		out = append(out, "Values: "+keywordList(a.Keywords))
	}
	return out
}

func keywordList(ks []catalog.Item) string {
	names := make([]string, 0, len(ks))
	for _, k := range ks {
		n := "`" + k.Name + "`"
		if k.Since > 1 {
			n += fmt.Sprintf(" (version %d)", k.Since)
		}
		names = append(names, n)
	}
	return strings.Join(names, " | ")
}

func (s *Server) hoverTCSS(doc *document, word string, start, end int) []string {
	ctx := lexTCSS(doc.text, end)
	switch ctx.kind {
	case cssProperty:
		if p := s.property(strings.TrimSpace(word)); p != nil {
			return propertyText(*p)
		}
	case cssValue:
		p := s.property(ctx.prop)
		if p == nil {
			return nil
		}
		for _, k := range p.Keywords {
			if k.Name == word {
				return append([]string{fmt.Sprintf("`%s` — value of `%s`%s", k.Name, p.Name, sinceSuffix(k.Since))}, propertyText(*p)...)
			}
		}
		return propertyText(*p)
	case cssSelector:
		if i := strings.LastIndexByte(word, ':'); i >= 0 {
			for _, p := range s.cat.PseudoClasses {
				if p.Name == word[i+1:] {
					return []string{fmt.Sprintf("`:%s` — pseudo-class%s", p.Name, sinceSuffix(p.Since))}
				}
			}
		}
		if t := s.tag(word); t != nil {
			return tagText(*t)
		}
	case cssMedia:
		for _, m := range s.cat.Media {
			if m.Name == word {
				out := []string{fmt.Sprintf("`%s` — @media feature%s", m.Name, sinceSuffix(m.Since))}
				if len(m.Keywords) > 0 {
					out = append(out, "Values: "+keywordList(m.Keywords))
				}
				return out
			}
		}
	}
	return nil
}

func propertyText(p catalog.Property) []string {
	return []string{
		fmt.Sprintf("`%s` — property%s", p.Name, sinceSuffix(p.Since)),
		"Syntax: " + p.Syntax,
	}
}
