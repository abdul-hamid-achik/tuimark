package lsp

import (
	"regexp"
	"strings"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// location is an LSP Location.
type location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

// definition answers textDocument/definition (SPEC §15.12 "Definition"):
// an #id (in a selector, to, when, or focus) to its element, a .class to
// every element that has it, <style src> to its file, and a $token to its
// declarations. It returns null when the cursor is on none of these.
func (s *Server) definition(sn *snapshot, doc *document, pos Position) []location {
	off := byteOffset(doc.text, pos)
	var locs []location
	switch doc.kind {
	case kindTUI:
		locs = s.definitionTUI(sn, doc, off)
	case kindTCSS:
		locs = s.definitionTCSS(sn, doc, off)
	}
	if len(locs) == 0 {
		return nil
	}
	return locs
}

func (s *Server) uriOfPath(sn *snapshot, path string) string {
	for _, d := range sn.byURI {
		if d.path == path {
			return d.uri
		}
	}
	return pathToURI(path)
}

// nodeLocation is the range of an element's start tag name in the file at
// path, whose text is text.
func (s *Server) nodeLocation(sn *snapshot, path, text string, n *ir.Node) location {
	lines := splitLines(text)
	start := fromDiag(lines, n.Line, n.Col)
	end := start
	if n.Line >= 1 && n.Line <= len(lines) {
		end.Character = tokenEnd(lines[start.Line], start.Character)
	}
	return location{URI: s.uriOfPath(sn, path), Range: Range{Start: start, End: end}}
}

func (s *Server) definitionTUI(sn *snapshot, doc *document, off int) []location {
	ctx := lexTUI(doc.text, off)
	if ctx.kind != tuiAttrValue {
		return nil
	}
	value := ctx.prefix + doc.text[off:off+valueRest(doc.text, off)]
	switch {
	case ctx.tag == "style" && ctx.attr == "src":
		if doc.path == "" || value == "" {
			return nil
		}
		p := resolveSrc(doc.path, value)
		if _, ok := readText(sn, p); !ok {
			return nil
		}
		return []location{{URI: s.uriOfPath(sn, p)}}
	case ctx.attr == "to", ctx.attr == "focus", ctx.attr == "when":
		word, _, _ := wordAt(doc.text, off)
		i := strings.LastIndexByte(word, '#')
		if i < 0 {
			return nil
		}
		id, _, _ := strings.Cut(word[i+1:], ":")
		if n := parseText(doc).IDs[id]; n != nil {
			return []location{s.nodeLocation(sn, doc.path, doc.text, n)}
		}
	}
	return nil
}

// valueRest is the length of the rest of an attribute value after byte
// offset off: up to the closing quote.
func valueRest(text string, off int) int {
	n := 0
	for off+n < len(text) && text[off+n] != '"' && text[off+n] != '\'' && text[off+n] != '\n' {
		n++
	}
	return n
}

func (s *Server) definitionTCSS(sn *snapshot, doc *document, off int) []location {
	word, wstart, wend := wordAt(doc.text, off)
	if word == "" {
		return nil
	}
	loaders := parsedLoaders(sn, doc.path)
	// A selector: the sigil the cursor's name follows, within the word.
	if ctx := lexTCSS(doc.text, wend); ctx.kind == cssSelector {
		i := strings.LastIndexAny(word[:max(0, min(len(word), off-wstart))], "#.")
		if i < 0 {
			return nil
		}
		sigil, name := word[i], word[i+1:]
		if j := strings.IndexAny(name, "#.:"); j >= 0 {
			name = name[:j]
		}
		var locs []location
		for _, l := range loaders {
			switch sigil {
			case '#':
				if n := l.doc.IDs[name]; n != nil {
					locs = append(locs, s.nodeLocation(sn, l.path, l.text, n))
				}
			case '.':
				walkNodes(l.doc, func(n *ir.Node) {
					if hasClass(n, name) {
						locs = append(locs, s.nodeLocation(sn, l.path, l.text, n))
					}
				})
			}
		}
		return locs
	}
	// A $token or var(--token): the declarations of --token.
	var name string
	switch {
	case strings.HasPrefix(word, "$"):
		name = word[1:]
	case strings.HasPrefix(word, "--"):
		name = word[2:]
	default:
		return nil
	}
	return s.tokenDeclarations(sn, doc, loaders, name)
}

// tokenDeclarations are the `--name:` declarations in this stylesheet and
// in the others the documents that load it load.
func (s *Server) tokenDeclarations(sn *snapshot, doc *document, loaders []parsedDoc, name string) []location {
	files := map[string]string{doc.path: doc.text}
	order := []string{doc.path}
	for _, l := range loaders {
		for _, p := range loadedPaths(l.path, l.doc) {
			if _, seen := files[p]; seen {
				continue
			}
			if t, ok := readText(sn, p); ok {
				files[p] = t
				order = append(order, p)
			}
		}
	}
	re := regexp.MustCompile(`--` + regexp.QuoteMeta(name) + `\s*:`)
	var locs []location
	for _, p := range order {
		text := files[p]
		for _, m := range re.FindAllStringIndex(text, -1) {
			start := positionAt(text, m[0])
			end := positionAt(text, m[0]+2+len(name))
			locs = append(locs, location{URI: s.uriOfPath(sn, p), Range: Range{Start: start, End: end}})
		}
	}
	return locs
}

var ownActionRE = regexp.MustCompile(`\b(?:on:[a-z]+|action)\s*=\s*"([^"]+)"`)

// ownActions are the action names a .tui writes in on:* attributes and
// keymap rows, in text order and without repeats.
func ownActions(text string) []string {
	return uniqueSorted(submatches(ownActionRE, text))
}

// parseText is a parse of the document's current text, for its ids and
// elements; it checks nothing else.
func parseText(doc *document) *parse.Document {
	return parse.Parse([]byte(doc.text), doc.path)
}
