package parse

import (
	"path/filepath"
	"sort"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// A committed template (SPEC v0.4d §26.15.4, decision 245) is the copy of a
// <use> without id, with the commit's with and props, placed at the root of
// the active screen. The runtime builds it by expanding such a use in a
// document made of the original's templates and includes, and a screen that
// holds the use alone: the one place that knows how a copy is built.

// CommitLine is the line of the synthetic <use> and of its attributes, so a
// diagnostic about the use is told from one about the document. A prop
// attribute i has column i+1, the use itself column 0.
const CommitLine = -7

// CommitProp is one prop a commit passes.
type CommitProp struct{ Name, Value string }

// CommitDocument expands a <use template="id" with="with" NAME="VALUE"…/>
// in a screen of doc's templates and includes. props are written in the
// order given (the caller sorts them), each at column i+1. The document is
// the last of its screens; its diagnostics tell what the expansion of the
// use found, at CommitLine. doc must be a <tui> without version.
func CommitDocument(doc *Document, ld *Loader, id, with string, props []CommitProp) *Document {
	raw := doc.Raw
	tui := &RawNode{Type: RawElement, Name: "tui", Attrs: raw.Attrs, Line: raw.Line, Col: raw.Col, Span: raw.Span}
	keep := func(c *RawNode) bool {
		return c.Type == RawElement && (c.Name == "template" || c.Name == "include")
	}
	for _, c := range raw.Children {
		switch {
		case keep(c):
			tui.Children = append(tui.Children, c)
		case c.Type == RawElement && c.Name == "screen":
			sc := *c
			sc.Children = nil
			for _, g := range c.Children {
				if keep(g) {
					sc.Children = append(sc.Children, g)
				}
			}
			tui.Children = append(tui.Children, &sc)
		}
	}
	use := &RawNode{Type: RawElement, Name: "use", SelfClosing: true, Line: CommitLine}
	use.Attrs = append(use.Attrs, RawAttr{Name: "template", Value: id, Line: CommitLine})
	if with != "" {
		use.Attrs = append(use.Attrs, RawAttr{Name: "with", Value: with, Line: CommitLine})
	}
	for i, p := range props {
		use.Attrs = append(use.Attrs, RawAttr{Name: p.Name, Value: p.Value, Line: CommitLine, Col: i + 1})
	}
	tui.Children = append(tui.Children, &RawNode{Type: RawElement, Name: "screen", Children: []*RawNode{use}, Line: raw.Line + 1})
	out := &Document{File: doc.File, IDs: map[string]*ir.Node{}, Lang: ir.Lang1}
	b := &builder{doc: out, file: filepath.Base(doc.File), lang: ir.Lang1}
	if doc.File == "" {
		b.file = ""
	}
	return b.build(out, tui, nil, nil, doc.File, ld)
}

// SortProps returns props as CommitProps sorted by name, the order a
// commit reports its first error in.
func SortProps(props map[string]string) []CommitProp {
	out := make([]CommitProp, 0, len(props))
	for n, v := range props {
		out = append(out, CommitProp{n, v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
