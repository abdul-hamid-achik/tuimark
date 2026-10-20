package lsp

import (
	"path/filepath"
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

// uriOfPath is the URI of the file at path: self's own when it is self's
// file (an untitled buffer has no path, and is only ever self), else the
// URI of the open buffer for it, else a file: URI. A buffer with no path
// is never matched by one.
func (s *Server) uriOfPath(sn *snapshot, self *document, path string) string {
	if self != nil && self.path == path {
		return self.uri
	}
	if path == "" {
		return ""
	}
	for _, d := range sn.byURI {
		if d.path == path {
			return d.uri
		}
	}
	return pathToURI(path)
}

// nodeLocation is the range of an element's start tag name in the file at
// path, whose text is text.
func (s *Server) nodeLocation(sn *snapshot, self *document, path, text string, n *ir.Node) location {
	lines := splitLines(text)
	start := fromDiag(lines, n.Line, n.Col)
	end := start
	if n.Line >= 1 && n.Line <= len(lines) {
		end.Character = tokenEnd(lines[start.Line], start.Character)
	}
	return location{URI: s.uriOfPath(sn, self, path), Range: Range{Start: start, End: end}}
}

func (s *Server) definitionTUI(sn *snapshot, doc *document, off int) []location {
	ctx := lexTUI(doc.text, off)
	if ctx.kind != tuiAttrValue {
		return nil
	}
	value := ctx.prefix + doc.text[off:off+valueRest(doc.text, off)]
	// Composition is the current language's, which a fragment always is
	// (SPEC v0.4b §15.12 "Definition").
	composed := docVersionOf(doc.text) == int(ir.LangCurrent)
	switch {
	case ctx.tag == "style" && ctx.attr == "src", composed && ctx.tag == "include" && ctx.attr == "src":
		if doc.path == "" || value == "" {
			return nil
		}
		p := resolveSrc(doc.path, value)
		if _, ok := readText(sn, p); !ok {
			return nil
		}
		return []location{{URI: s.uriOfPath(sn, doc, p)}}
	case composed && ctx.tag == "use" && ctx.attr == "template":
		// The <template id> in whichever file holds it.
		for _, a := range s.composedDocs(sn, doc) {
			if n := findWritten(a.doc, func(n *ir.Node) bool { return n.Kind == "template" && n.ID == value }); n != nil {
				return []location{s.locationIn(sn, doc, a, n)}
			}
		}
	case ctx.attr == "to", ctx.attr == "focus", ctx.attr == "when":
		word, _, _ := wordAt(doc.text, off)
		i := strings.LastIndexByte(word, '#')
		if i < 0 {
			return nil
		}
		id, _, _ := strings.Cut(word[i+1:], ":")
		if composed {
			// An element, a copy's included (its scoped id names the
			// template's element it was copied from), else a <use>.
			for _, a := range s.composedDocs(sn, doc) {
				if n := a.doc.IDs[id]; n != nil {
					return []location{s.locationIn(sn, doc, a, n)}
				}
				if n := findWritten(a.doc, func(n *ir.Node) bool { return n.Kind == "use" && n.ID == id }); n != nil {
					return []location{s.locationIn(sn, doc, a, n)}
				}
			}
		}
		if n := parseText(doc).IDs[id]; n != nil {
			return []location{s.nodeLocation(sn, doc, doc.path, doc.text, n)}
		}
	}
	return nil
}

// composedDocs are the analyses whose expanded documents a definition or
// a completion in doc reads: doc's own, or, for a fragment, those of the
// documents that read it (SPEC v0.4b §15.12).
func (s *Server) composedDocs(sn *snapshot, doc *document) []*analysis {
	a := s.analysisOf(sn, doc.path, doc.text)
	if !a.fragment {
		return []*analysis{a}
	}
	if doc.path == "" {
		return nil
	}
	return s.includers(sn, doc.path)
}

// locationIn is the location of an element of a's expanded document, in
// the file it is written in.
func (s *Server) locationIn(sn *snapshot, self *document, a *analysis, n *ir.Node) location {
	return s.writtenLocation(sn, self, a.path, a.text, n)
}

// writtenLocation is the location of an element of the document at path,
// whose text is text, in the file it is written in: the loader names a
// fragment by its absolute path, and the document by its base name, or
// by nothing for an element written in it.
func (s *Server) writtenLocation(sn *snapshot, self *document, path, text string, n *ir.Node) location {
	if f := n.Org.FileOr(""); filepath.IsAbs(f) && f != path {
		path = f
		text, _ = readText(sn, f)
	}
	return s.nodeLocation(sn, self, path, text, n)
}

// findWritten is the first element of the document as written, in
// document order, that match accepts: its <template>, <use>, and
// <include> elements included, which only the written children
// (ir.Node.Source) hold.
func findWritten(doc *parse.Document, match func(*ir.Node) bool) *ir.Node {
	var found *ir.Node
	walkWritten(doc, func(n *ir.Node) bool {
		if match(n) {
			found = n
			return false
		}
		return true
	})
	return found
}

// walkWritten calls fn for every element of the document as written, in
// document order, until it returns false.
func walkWritten(doc *parse.Document, fn func(*ir.Node) bool) {
	var walk func(n *ir.Node) bool
	walk = func(n *ir.Node) bool {
		if n == nil {
			return true
		}
		if !fn(n) {
			return false
		}
		kids := n.Children
		if n.Source != nil {
			kids = n.Source
		}
		for _, c := range kids {
			if !walk(c) {
				return false
			}
		}
		return true
	}
	walk(doc.Root)
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
					locs = append(locs, s.writtenLocation(sn, doc, l.path, l.text, n))
				}
			case '.':
				walkNodes(l.doc, func(n *ir.Node) {
					if hasClass(n, name) {
						locs = append(locs, s.writtenLocation(sn, doc, l.path, l.text, n))
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
			locs = append(locs, location{URI: s.uriOfPath(sn, doc, p), Range: Range{Start: start, End: end}})
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
