package lsp

import (
	"sort"
	"strconv"
	"strings"

	"github.com/abdul-hamid-achik/tuimark/internal/catalog"
	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// CompletionItemKind values the server uses.
const (
	kindFunction   = 3
	kindField      = 5
	kindClass      = 7
	kindProperty   = 10
	kindEnumMember = 20
	kindKeyword    = 14
	kindColor      = 16
	kindReference  = 18
)

type textEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

type completionItem struct {
	Label            string    `json:"label"`
	Kind             int       `json:"kind,omitempty"`
	Detail           string    `json:"detail,omitempty"`
	FilterText       string    `json:"filterText,omitempty"`
	SortText         string    `json:"sortText,omitempty"`
	InsertTextFormat int       `json:"insertTextFormat,omitempty"`
	TextEdit         *textEdit `json:"textEdit,omitempty"`
}

// completionList is the result of textDocument/completion.
type completionList struct {
	IsIncomplete bool             `json:"isIncomplete"`
	Items        []completionItem `json:"items"`
}

// completer builds the items of one request: each replaces the text from
// start (a byte offset) to the cursor, so a sigil in front of the word
// (#, ., :, $, {) is replaced or kept exactly.
type completer struct {
	text  string
	off   int
	start int
	items []completionItem
	// rng is the range of the items added while start was rngStart: one
	// request may offer a copy's every scoped id, so it is computed once,
	// not once per item.
	rng      Range
	rngStart int
	rngSet   bool
}

// at starts a completion that replaces text[start:off], keeping only the
// items whose label starts with the typed prefix.
func newCompleter(text string, start, off int) *completer {
	return &completer{text: text, off: off, start: start}
}

func (c *completer) prefix() string { return c.text[c.start:c.off] }

// add offers label (inserted as insert, or as the label when empty) when it
// starts with the typed prefix.
func (c *completer) add(label string, kind int, detail, insert string, snippet bool) {
	if !strings.HasPrefix(label, c.prefix()) {
		return
	}
	if insert == "" {
		insert = label
	}
	if !c.rngSet || c.rngStart != c.start {
		c.rng = Range{Start: positionAt(c.text, c.start), End: positionAt(c.text, c.off)}
		c.rngStart, c.rngSet = c.start, true
	}
	it := completionItem{
		Label: label, Kind: kind, Detail: detail,
		SortText: strconv.Itoa(len(c.items) + 1000)[1:],
		TextEdit: &textEdit{
			Range:   c.rng,
			NewText: insert,
		},
	}
	if snippet {
		it.InsertTextFormat = 2
	}
	c.items = append(c.items, it)
}

func (c *completer) list() *completionList {
	if c.items == nil {
		c.items = []completionItem{}
	}
	return &completionList{Items: c.items}
}

func sinceDetail(since int) string {
	switch {
	case since <= 1:
		return ""
	case since >= int(ir.LangCurrent):
		// There is no version="4" (SPEC v0.4b §5.1): an item since the
		// current language exists only in a <tui> without version.
		return currentDetail
	}
	return "version " + strconv.Itoa(since)
}

// currentDetail names the current language, a <tui> without version, in
// a completion's detail and a hover (sinceSuffix).
const currentDetail = "a <tui> without version"

// complete answers textDocument/completion (SPEC §15.12 "Completion").
func (s *Server) complete(sn *snapshot, doc *document, pos Position) *completionList {
	off := byteOffset(doc.text, pos)
	switch doc.kind {
	case kindTUI:
		return s.completeTUI(sn, doc, off)
	case kindTCSS:
		return s.completeTCSS(sn, doc, off)
	}
	return &completionList{Items: []completionItem{}}
}

// --- .tui ------------------------------------------------------------------

func (s *Server) completeTUI(sn *snapshot, doc *document, off int) *completionList {
	ctx := lexTUI(doc.text, off)
	ver := docVersionOf(doc.text)
	switch ctx.kind {
	case tuiTagName:
		c := newCompleter(doc.text, ctx.start, off)
		// A <slot/> stands only in a template's body, or in a fragment,
		// which an <include> may place in one (SPEC v0.4c §15.12, test 263).
		slot := enclosingTemplate(ctx.stack) != nil || len(ctx.stack) > 0 && ctx.stack[0].name == "fragment"
		for _, t := range s.cat.Tags {
			if t.Since <= ver && (t.Name != "slot" || slot) {
				c.add(t.Name, kindClass, sinceDetail(t.Since), "", false)
			}
		}
		return c.list()
	case tuiAttrName:
		c := newCompleter(doc.text, ctx.start, off)
		next := strings.HasPrefix(doc.text[off:], "=") // renaming an attribute that has a value
		if t := s.tag(ctx.tag); t != nil {
			for _, a := range t.Attrs {
				if a.Since > ver || a.Pattern != "" || ctx.present[a.Name] {
					continue
				}
				if next {
					c.add(a.Name, kindProperty, sinceDetail(a.Since), "", false)
				} else {
					c.add(a.Name, kindProperty, sinceDetail(a.Since), a.Name+`="$1"`, true)
				}
			}
		}
		if ctx.tag == "use" && ver == int(ir.LangCurrent) {
			// The props its template declares (SPEC v0.4c §15.12).
			s.addUseProps(c, sn, doc, ctx, next)
		}
		return c.list()
	case tuiAttrValue:
		return s.completeValue(sn, doc, ctx, off, ver)
	case tuiText:
		if i := openProp(ctx.prefix); i >= 0 && ver == int(ir.LangCurrent) {
			c := newCompleter(doc.text, ctx.start+i, off)
			addProps(c, doc, ctx)
			return c.list()
		}
		if i := strings.LastIndexByte(ctx.prefix, '{'); i >= 0 && !strings.Contains(ctx.prefix[i:], "}") {
			c := newCompleter(doc.text, ctx.start+i+1, off)
			s.addPaths(c, sn, doc, ctx, false)
			return c.list()
		}
	}
	return &completionList{Items: []completionItem{}}
}

func (s *Server) tag(name string) *catalog.Tag {
	for i := range s.cat.Tags {
		if s.cat.Tags[i].Name == name {
			return &s.cat.Tags[i]
		}
	}
	return nil
}

// attrOf is the catalog's attribute of a tag.
func (s *Server) attrOf(tag, attr string) *catalog.Attr {
	t := s.tag(tag)
	if t == nil {
		return nil
	}
	for i := range t.Attrs {
		if t.Attrs[i].Name == attr {
			return &t.Attrs[i]
		}
	}
	return nil
}

// pathAttrs are the attributes whose value is a bare path in the store
// (with a leading ! for a negation where the grammar has one).
var pathAttrs = map[string]bool{
	"bind": true, "if": true, "checked": true, "open": true, "mouse": true,
	"disabled": true, "hidden": true,
}

// templateAttrs are the attributes whose value may hold {path}.
var templateAttrs = map[string]bool{"title": true, "placeholder": true, "label": true, "short": true}

func (s *Server) completeValue(sn *snapshot, doc *document, ctx tuiCtx, off, ver int) *completionList {
	c := newCompleter(doc.text, ctx.start, off)
	attr := ctx.attr
	if i := openProp(ctx.prefix); i >= 0 && ver == int(ir.LangCurrent) {
		// A {$name} in a template's body (SPEC v0.4c §15.12).
		c.start = ctx.start + i
		addProps(c, doc, ctx)
		return c.list()
	}
	// Literal keywords of an enumerated attribute.
	if a := s.attrOf(ctx.tag, attr); a != nil {
		for _, k := range a.Keywords {
			if k.Since <= ver {
				c.add(k.Name, kindEnumMember, sinceDetail(k.Since), "", false)
			}
		}
	}
	switch {
	case strings.HasPrefix(attr, "on:"), attr == "action":
		s.addActions(c, doc.text, ver, attr != "action")
	case attr == "keys":
		// The last key of the list.
		c.start = ctx.start + strings.LastIndexByte(ctx.prefix, ',') + 1
		for c.start < off && doc.text[c.start] == ' ' {
			c.start++
		}
		s.addKeys(c)
	case attr == "to", attr == "focus", attr == "when":
		// A selector such as #list:focus: the id after the last #.
		if i := strings.LastIndexByte(ctx.prefix, '#'); i >= 0 {
			c.start = ctx.start + i
		}
		for _, id := range idsOf(doc.text) {
			c.add("#"+id, kindReference, "id", "", false)
		}
		if ver == int(ir.LangCurrent) {
			// The scoped ids of copies (SPEC v0.4b §15.12 "Composition").
			for _, id := range s.copyIDs(sn, doc) {
				c.add("#"+id, kindReference, "id", "", false)
			}
		}
	case attr == "template" && ctx.tag == "use" && ver == int(ir.LangCurrent):
		// The template ids of the expanded document.
		for _, id := range s.templateIDs(sn, doc) {
			c.add(id, kindReference, "template", "", false)
		}
	case attr == "each":
		if !strings.Contains(ctx.prefix, " ") {
			s.addPaths(c, sn, doc, ctx, true)
		}
	case attr == "class":
		c.start = ctx.start + strings.LastIndexAny(ctx.prefix, " \t\n") + 1
		for _, cl := range s.classesFor(sn, doc) {
			c.add(cl, kindClass, "class", "", false)
		}
	case pathAttrs[attr], strings.HasPrefix(attr, "class:"):
		if strings.HasPrefix(ctx.prefix, "!") {
			c.start++
		}
		s.addPaths(c, sn, doc, ctx, false)
	case templateAttrs[attr]:
		if i := strings.LastIndexByte(ctx.prefix, '{'); i >= 0 && !strings.Contains(ctx.prefix[i:], "}") {
			c.start = ctx.start + i + 1
			s.addPaths(c, sn, doc, ctx, false)
		}
	}
	return c.list()
}

// copyIDs are the scoped ids of the copies in the expanded documents
// composedDocs names, sorted, each once: the ids a use gives the elements
// of its copy (SPEC v0.4b §6.18.4). The expansion limit bounds how many
// there are and how long (ADR 0034).
func (s *Server) copyIDs(sn *snapshot, doc *document) []string {
	var out []string
	for _, a := range s.composedDocs(sn, doc) {
		for id, n := range a.doc.IDs {
			if n.InCopy() {
				out = append(out, id)
			}
		}
	}
	return uniqueSorted(out)
}

// templateIDs are the ids of the <template> elements of the expanded
// documents composedDocs names, in whichever file each is written, sorted.
func (s *Server) templateIDs(sn *snapshot, doc *document) []string {
	var out []string
	for _, a := range s.composedDocs(sn, doc) {
		walkWritten(a.doc, func(n *ir.Node) bool {
			if n.Kind == "template" && n.ID != "" {
				out = append(out, n.ID)
			}
			return true
		})
	}
	return uniqueSorted(out)
}

// addActions offers the built-in actions the version knows and the host
// actions the document names. In an on:* attribute a keymap-only built-in
// (the hyphenated ones) is V003, so it is left out.
func (s *Server) addActions(c *completer, text string, ver int, onAttr bool) {
	builtin := map[string]bool{}
	for _, a := range s.cat.Actions {
		builtin[a.Name] = true
		if a.Since <= ver && !(onAttr && a.KeymapOnly) {
			c.add(a.Name, kindFunction, "built-in", "", false)
		}
	}
	for _, name := range ownActions(text) {
		if !builtin[name] {
			c.add(name, kindFunction, "this document", "", false)
		}
	}
}

// addKeys offers the key tokens of SPEC §8.1: the named keys and ctrl+a to
// ctrl+z, without those that never match.
func (s *Server) addKeys(c *completer) {
	never := map[string]bool{}
	for _, k := range s.cat.Keys.NeverMatch {
		never[k] = true
	}
	for _, k := range s.cat.Keys.Named {
		c.add(k, kindKeyword, "", "", false)
	}
	for r := 'a'; r <= 'z'; r++ {
		if k := "ctrl+" + string(r); !never[k] {
			c.add(k, kindKeyword, "", "", false)
		}
	}
}

// classesFor are the class names a document can use: those it writes and
// those the stylesheets it loads select.
func (s *Server) classesFor(sn *snapshot, doc *document) []string {
	out := classesOf(doc.text)
	for _, p := range styleSrcsOf(doc.path, doc.text) {
		if t, ok := readText(sn, p); ok {
			out = append(out, selectorClasses(t)...)
		}
	}
	return uniqueSorted(out)
}

// addPaths offers the paths of the document's sample.json, and the fields
// of the alias of each enclosing each. Only arrays when arraysOnly.
func (s *Server) addPaths(c *completer, sn *snapshot, doc *document, ctx tuiCtx, arraysOnly bool) {
	data, ok := readSample(sn, doc.path)
	if !ok {
		return
	}
	var items []pathItem
	collectPaths("", data, 0, &items)
	for _, t := range ctx.stack {
		path, alias, ok := splitEach(t.attrs["each"])
		if !ok {
			continue
		}
		arr, ok := lookupPath(data, path).([]any)
		if !ok || len(arr) == 0 {
			continue
		}
		items = append(items, pathItem{alias, false})
		collectPaths(alias+".", arr[0], 0, &items)
	}
	for _, it := range items {
		if arraysOnly && !it.array {
			continue
		}
		detail := ""
		if it.array {
			detail = "array"
		}
		c.add(it.path, kindField, detail, "", false)
	}
}

// splitEach splits each="path as alias".
func splitEach(v string) (path, alias string, ok bool) {
	p, a, found := strings.Cut(v, " as ")
	if !found || strings.TrimSpace(p) == "" || strings.TrimSpace(a) == "" {
		return "", "", false
	}
	return strings.TrimSpace(p), strings.TrimSpace(a), true
}

// pathItem is a path of the store, and whether it holds an array.
type pathItem struct {
	path  string
	array bool
}

const (
	maxPathDepth = 6
	maxPathItems = 500
)

// collectPaths lists the dotted paths below v, objects only: an array is
// listed and not entered.
func collectPaths(prefix string, v any, depth int, out *[]pathItem) {
	m, ok := v.(map[string]any)
	if !ok || depth >= maxPathDepth {
		return
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if len(*out) >= maxPathItems {
			return
		}
		_, isArr := m[k].([]any)
		*out = append(*out, pathItem{prefix + k, isArr})
		collectPaths(prefix+k+".", m[k], depth+1, out)
	}
}

// lookupPath follows a dotted path through objects.
func lookupPath(v any, path string) any {
	for _, part := range strings.Split(path, ".") {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		if v, ok = m[part]; !ok {
			return nil
		}
	}
	return v
}

// --- .tcss -----------------------------------------------------------------

// selectorClasses are the .class names a stylesheet selects.
func selectorClasses(text string) []string {
	var out []string
	for i := 0; i < len(text); i++ {
		if text[i] != '.' || i+1 >= len(text) || !isNameByte(text[i+1]) {
			continue
		}
		j := i + 1
		for j < len(text) && isNameByte(text[j]) && text[j] != '.' && text[j] != ':' {
			j++
		}
		out = append(out, text[i+1:j])
		i = j - 1
	}
	return out
}

func (s *Server) property(name string) *catalog.Property {
	for i := range s.cat.Properties {
		if s.cat.Properties[i].Name == name {
			return &s.cat.Properties[i]
		}
	}
	return nil
}

func (s *Server) completeTCSS(sn *snapshot, doc *document, off int) *completionList {
	ctx := lexTCSS(doc.text, off)
	loaders := parsedLoaders(sn, doc.path)
	ver := loadersVersion(loaders)
	word, wstart, _ := wordAt(doc.text[:off], off)
	switch ctx.kind {
	case cssSelector:
		return s.completeSelector(doc, loaders, ver, off)
	case cssProperty:
		c := newCompleter(doc.text, wstart, off)
		if ctx.root { // a :root rule declares --tokens, which are not in the catalog
			return c.list()
		}
		for _, p := range s.cat.Properties {
			if p.Since <= ver {
				c.add(p.Name, kindProperty, sinceDetail(p.Since), p.Name+": ", false)
			}
		}
		return c.list()
	case cssValue:
		c := newCompleter(doc.text, wstart, off)
		if strings.HasPrefix(word, "$") {
			c.start = wstart + 1
			for _, t := range s.tokensFor(sn, doc, loaders) {
				c.add(t, kindColor, "token", "", false)
			}
			return c.list()
		}
		if p := s.property(ctx.prop); p != nil {
			for _, k := range p.Keywords {
				if k.Since <= ver {
					c.add(k.Name, kindEnumMember, sinceDetail(k.Since), "", false)
				}
			}
		}
		return c.list()
	case cssMedia:
		c := newCompleter(doc.text, wstart, off)
		if ctx.prop == "" {
			for _, m := range s.cat.Media {
				if m.Since <= ver {
					c.add(m.Name, kindProperty, sinceDetail(m.Since), "", false)
				}
			}
			return c.list()
		}
		for _, m := range s.cat.Media {
			if m.Name != ctx.prop {
				continue
			}
			for _, k := range m.Keywords {
				if k.Since <= ver {
					c.add(k.Name, kindEnumMember, sinceDetail(k.Since), "", false)
				}
			}
		}
		return c.list()
	}
	return &completionList{Items: []completionItem{}}
}

// completeSelector offers, by the sigil before the word: #ids, .classes,
// :pseudo-classes, @media, else tag names.
func (s *Server) completeSelector(doc *document, loaders []parsedDoc, ver, off int) *completionList {
	word, wstart, _ := wordAt(doc.text[:off], off)
	// The word includes its sigils; the part typed is what follows the last.
	start := wstart
	var sigil byte
	if i := strings.LastIndexAny(word, "#.:@"); i >= 0 {
		sigil = word[i]
		start = wstart + i + 1
	}
	c := newCompleter(doc.text, start, off)
	switch sigil {
	case '#':
		var ids []string
		for _, l := range loaders {
			ids = append(ids, idsOf(l.text)...)
			if l.expanded {
				// Its fragments' ids and its copies' scoped ids too.
				for id := range l.doc.IDs {
					ids = append(ids, id)
				}
			}
		}
		for _, id := range uniqueSorted(ids) {
			c.add(id, kindReference, "id", "", false)
		}
	case '.':
		var classes []string
		for _, l := range loaders {
			classes = append(classes, classesOf(l.text)...)
			if l.expanded {
				walkNodes(l.doc, func(n *ir.Node) {
					classes = append(classes, n.Classes...)
					for _, g := range n.ClassGuards {
						classes = append(classes, g.Name)
					}
				})
			}
		}
		for _, cl := range uniqueSorted(classes) {
			c.add(cl, kindClass, "class", "", false)
		}
	case ':':
		c.add("root", kindKeyword, "tokens", "", false)
		for _, p := range s.cat.PseudoClasses {
			if p.Since <= ver {
				c.add(p.Name, kindKeyword, sinceDetail(p.Since), "", false)
			}
		}
	case '@':
		c.add("media", kindKeyword, "", "", false)
	default:
		for _, t := range s.cat.Tags {
			if t.Since <= ver {
				c.add(t.Name, kindClass, sinceDetail(t.Since), "", false)
			}
		}
	}
	return c.list()
}

// tokensFor are the $tokens a stylesheet can use: the theme's own, and those
// declared in a :root of this stylesheet or of the others its documents
// load.
func (s *Server) tokensFor(sn *snapshot, doc *document, loaders []parsedDoc) []string {
	out := tokensOf(doc.text)
	for name := range css.Themes["dark"] {
		out = append(out, name)
	}
	for _, l := range loaders {
		for _, p := range loadedPaths(l.path, l.doc) {
			if p == doc.path {
				continue
			}
			if t, ok := readText(sn, p); ok {
				out = append(out, tokensOf(t)...)
			}
		}
	}
	return uniqueSorted(out)
}
