package lsp

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// Props and the slot in the language server (SPEC v0.4c §15.12, decision
// 181): completion of a <use>'s props and of {$name} in a template's
// body, hover on both, and definition from either to the name in the
// template's props. Like the lexers, it reads the text as written, so it
// works in a buffer being typed.

// attrSpan is an attribute of a start tag as written: its name and value,
// and the byte offsets where each starts.
type attrSpan struct {
	name, value           string
	nameStart, valueStart int
}

// startTagAttrs are the attributes of the start tag whose "<" is at byte
// offset start, in order, read up to its ">" or "/>", to the next "<", or
// to the end of the text: those after a cursor too. A value whose closing
// quote is missing runs to the end of its line.
func startTagAttrs(text string, start int) []attrSpan {
	if start < 0 || start >= len(text) || text[start] != '<' {
		return nil
	}
	p := start + 1
	for p < len(text) && isNameByte(text[p]) {
		p++
	}
	var out []attrSpan
	for p < len(text) {
		for p < len(text) && isSpaceByte(text[p]) {
			p++
		}
		if p >= len(text) || text[p] == '<' || text[p] == '>' || strings.HasPrefix(text[p:], "/>") {
			break
		}
		q := p
		for q < len(text) && isNameByte(text[q]) {
			q++
		}
		if q == p { // a stray character: skip it
			p++
			continue
		}
		a := attrSpan{name: text[p:q], nameStart: p, valueStart: -1}
		r := q
		for r < len(text) && isSpaceByte(text[r]) {
			r++
		}
		if r >= len(text) || text[r] != '=' {
			out = append(out, a)
			p = r
			continue
		}
		r++
		for r < len(text) && isSpaceByte(text[r]) {
			r++
		}
		if r < len(text) && (text[r] == '"' || text[r] == '\'') {
			e := strings.IndexByte(text[r+1:], text[r])
			if nl := strings.IndexByte(text[r+1:], '\n'); e < 0 || nl >= 0 && nl < e {
				e = nl
				if e < 0 {
					e = len(text) - r - 1
				}
			}
			a.value, a.valueStart = text[r+1:r+1+e], r+1
			out = append(out, a)
			p = r + 1 + e + 1
			continue
		}
		v := r
		for v < len(text) && !isSpaceByte(text[v]) && text[v] != '>' && text[v] != '/' && text[v] != '<' {
			v++
		}
		a.value, a.valueStart = text[r:v], r
		out = append(out, a)
		p = v
	}
	return out
}

// attrNamed is the first attribute of attrs named name.
func attrNamed(attrs []attrSpan, name string) (attrSpan, bool) {
	for _, a := range attrs {
		if a.name == name {
			return a, true
		}
	}
	return attrSpan{}, false
}

// propNames are the names a props value declares, in order, each once.
// The validation of the names is validate's (V003).
func propNames(value string) []string {
	var out []string
	seen := map[string]bool{}
	for _, n := range strings.Fields(value) {
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	return out
}

// propIndex is the byte offset, in value, of the first name equal to name
// among those value declares, or -1.
func propIndex(value, name string) int {
	for i := 0; i < len(value); {
		for i < len(value) && isSpaceByte(value[i]) {
			i++
		}
		j := i
		for j < len(value) && !isSpaceByte(value[j]) {
			j++
		}
		if j > i && value[i:j] == name {
			return i
		}
		i = j
	}
	return -1
}

// enclosingTemplate is the innermost <template> open at the cursor: the
// template whose body the cursor is in, whose props a {$name} there reads.
func enclosingTemplate(stack []openTag) *openTag {
	for i := len(stack) - 1; i >= 0; i-- {
		if stack[i].name == "template" {
			return &stack[i]
		}
	}
	return nil
}

// propRef is the {$name} the byte offset off is in, if any: its name and
// where the name starts.
func propRef(text string, off int) (name string, start int, ok bool) {
	word, ws, _ := wordAt(text, off)
	if !strings.HasPrefix(word, "$") || ws == 0 || text[ws-1] != '{' {
		return "", 0, false
	}
	return word[1:], ws + 1, true
}

// propsDecl is a template's props attribute as written: the file the
// template is written in, its text, and the attribute.
type propsDecl struct {
	id         string
	path, text string
	attr       attrSpan
}

// names are the props the template declares.
func (d propsDecl) names() []string { return propNames(d.attr.value) }

// location is the range of name in the props attribute, when it declares
// it.
func (s *Server) propLocation(sn *snapshot, self *document, d propsDecl, name string) (location, bool) {
	i := propIndex(d.attr.value, name)
	if i < 0 {
		return location{}, false
	}
	at := d.attr.valueStart + i
	return location{
		URI:   s.uriOfPath(sn, self, d.path),
		Range: Range{Start: positionAt(d.text, at), End: positionAt(d.text, at+len(name))},
	}, true
}

// templateProps is the props attribute of the <template> whose id is id:
// in the expanded documents composedDocs names, in whichever file the
// template is written, else, for a buffer that does not parse yet, among
// the start tags of the buffer itself. ok is false when no such template
// writes props.
func (s *Server) templateProps(sn *snapshot, doc *document, id string) (propsDecl, bool) {
	if id == "" {
		return propsDecl{}, false
	}
	for _, a := range s.composedDocs(sn, doc) {
		n := findWritten(a.doc, func(n *ir.Node) bool { return n.Kind == "template" && n.ID == id })
		if n == nil {
			continue
		}
		path, text := a.path, a.text
		if f := n.Org.FileOr(""); filepath.IsAbs(f) && f != path {
			path = f
			text, _ = readText(sn, f)
		}
		if path == doc.path {
			text = doc.text
		}
		off := byteOffset(text, fromDiag(splitLines(text), n.Line, n.Col))
		if p, ok := attrNamed(startTagAttrs(text, off), "props"); ok {
			return propsDecl{id: id, path: path, text: text, attr: p}, true
		}
		return propsDecl{}, false
	}
	// The buffer's own start tags.
	for i := 0; ; {
		k := strings.Index(doc.text[i:], "<template")
		if k < 0 {
			break
		}
		i += k
		attrs := startTagAttrs(doc.text, i)
		i += len("<template")
		if i < len(doc.text) && isNameByte(doc.text[i]) {
			continue
		}
		if v, ok := attrNamed(attrs, "id"); !ok || v.value != id {
			continue
		}
		if p, ok := attrNamed(attrs, "props"); ok {
			return propsDecl{id: id, path: doc.path, text: doc.text, attr: p}, true
		}
		return propsDecl{}, false
	}
	return propsDecl{}, false
}

// enclosingProps is the props attribute of the template whose body the
// cursor is in (ctx's stack), in the buffer, for a cursor in text or in
// an attribute value.
func enclosingProps(doc *document, ctx tuiCtx) (propsDecl, bool) {
	if ctx.kind != tuiText && ctx.kind != tuiAttrValue {
		return propsDecl{}, false
	}
	t := enclosingTemplate(ctx.stack)
	if t == nil {
		return propsDecl{}, false
	}
	p, ok := attrNamed(startTagAttrs(doc.text, t.start), "props")
	if !ok {
		return propsDecl{}, false
	}
	return propsDecl{id: t.attrs["id"], path: doc.path, text: doc.text, attr: p}, true
}

// useTemplateProps is the props attribute of the template that the <use>
// whose "<" is at tagStart names.
func (s *Server) useTemplateProps(sn *snapshot, doc *document, tagStart int) (propsDecl, []attrSpan, bool) {
	attrs := startTagAttrs(doc.text, tagStart)
	t, ok := attrNamed(attrs, "template")
	if !ok {
		return propsDecl{}, attrs, false
	}
	d, ok := s.templateProps(sn, doc, t.value)
	return d, attrs, ok
}

// propDetail is a prop's completion detail and the first line of its
// hover.
func propDetail(id string) string {
	return fmt.Sprintf("prop of <template id=%q>", id)
}

// propsText is a template's props for a hover.
func propsText(d propsDecl) string {
	names := d.names()
	if len(names) == 0 {
		return "Props: none"
	}
	for i, n := range names {
		names[i] = "`" + n + "`"
	}
	return "Props: " + strings.Join(names, ", ")
}

// addUseProps offers, on a <use> whose template is known, the props that
// template declares and the use does not write yet (the attribute being
// typed at the cursor aside), after the use's own attributes.
func (s *Server) addUseProps(c *completer, sn *snapshot, doc *document, ctx tuiCtx, next bool) {
	d, attrs, ok := s.useTemplateProps(sn, doc, ctx.tagStart)
	if !ok {
		return
	}
	written := map[string]bool{}
	for _, a := range attrs {
		if a.nameStart != ctx.start {
			written[a.name] = true
		}
	}
	for _, name := range d.names() {
		if written[name] || s.attrOf("use", name) != nil {
			continue
		}
		if next {
			c.add(name, kindProperty, propDetail(d.id), "", false)
		} else {
			c.add(name, kindProperty, propDetail(d.id), name+`="$1"`, true)
		}
	}
}

// addProps offers, after a {$ in a template's body, the props the template
// declares; c starts after the $.
func addProps(c *completer, doc *document, ctx tuiCtx) {
	d, ok := enclosingProps(doc, ctx)
	if !ok {
		return
	}
	for _, name := range d.names() {
		c.add(name, kindField, propDetail(d.id), "", false)
	}
}

// openProp is the byte offset after the "{$" of a {$name} being typed at
// the end of prefix, or -1.
func openProp(prefix string) int {
	i := strings.LastIndexByte(prefix, '{')
	if i < 0 || strings.Contains(prefix[i:], "}") || !strings.HasPrefix(prefix[i+1:], "$") {
		return -1
	}
	return i + 2
}

// hoverProp is the hover of the {$name} under the cursor, or of a prop
// attribute of a <use>.
func (s *Server) hoverProp(sn *snapshot, doc *document, word string, start, end int) []string {
	if strings.HasPrefix(word, "$") && start > 0 && doc.text[start-1] == '{' {
		d, ok := enclosingProps(doc, lexTUI(doc.text, start-1))
		if !ok || propIndex(d.attr.value, word[1:]) < 0 {
			return nil // validate's V003 says it is not declared
		}
		return []string{fmt.Sprintf("`{%s}` — %s", word, propDetail(d.id)), propsText(d)}
	}
	ctx := lexTUI(doc.text, end)
	if ctx.kind != tuiAttrName || ctx.tag != "use" || s.attrOf("use", word) != nil {
		return nil
	}
	d, _, ok := s.useTemplateProps(sn, doc, ctx.tagStart)
	if !ok || propIndex(d.attr.value, word) < 0 {
		return nil
	}
	return []string{fmt.Sprintf("`%s` — %s", word, propDetail(d.id)), propsText(d)}
}

// definitionProp resolves the {$name} under the cursor, or a prop
// attribute of a <use>, to the name in the template's props.
func (s *Server) definitionProp(sn *snapshot, doc *document, off int) []location {
	if name, at, ok := propRef(doc.text, off); ok {
		d, ok := enclosingProps(doc, lexTUI(doc.text, at-2)) // before the "{"
		if !ok {
			return nil
		}
		if l, ok := s.propLocation(sn, doc, d, name); ok {
			return []location{l}
		}
		return nil
	}
	ctx := lexTUI(doc.text, off)
	if ctx.kind != tuiAttrName || ctx.tag != "use" {
		return nil
	}
	word, _, _ := wordAt(doc.text, off)
	if word == "" || s.attrOf("use", word) != nil {
		return nil
	}
	d, _, ok := s.useTemplateProps(sn, doc, ctx.tagStart)
	if !ok {
		return nil
	}
	if l, ok := s.propLocation(sn, doc, d, word); ok {
		return []location{l}
	}
	return nil
}
