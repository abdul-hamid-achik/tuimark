package parse

import (
	"fmt"
	"slices"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// The slot (SPEC v0.4c §6.18.10, ADR 0035, phase C2).
//
// A template's <slot/> is where each <use> of it puts its content, the
// use's fill. Neither the slot nor the use is a node: a copy replaces the
// slot by the fill, built as if its elements were written in place of the
// use (§6.18.1 step 3, sub-step 3), with the props, paths, ids, #id
// references, includes, and suffix of that place.
//
//   - A template's definition (templateBody) reads its whole body, the
//     content of the uses written in it included (useContent, decision
//     189). It judges where each <slot/> stands (slotMisplaced, decisions
//     169 and 193), and keeps its one slot, the first that no rule
//     reports (endSlots, decision 170), keyed by the template, the slot as
//     written, and the includes that placed it there (slotKey).
//   - A copy meets the slot again where the definition kept it (slot), and
//     builds there the fill of the use being copied (fill), switching to
//     the context recorded where that use is written (fillPlace). The
//     budget and depth frames (cp.frames) are not switched: a fill is
//     built in the expansion of the use that receives it (§6.18.8).
//   - The content of a use written outside every template's body is inert
//     until a copy reads it as a fill: step 1 builds nothing of it.
//
// The fill's top-level elements are recorded (fillRoot): a use's class
// reaches one of them only when the use is on its chain of placements
// (Org.Placed, decision 172), and a use's B005 hint skips the fill of the
// use (firstNodeID, decision 174).

// The messages of §6.18.10 and §6.18.2 (v0.4c).
const (
	msgSlotOutside   = `<slot/> is only valid inside a <template>: it marks where the content of a <use> goes`
	msgSlotInclude   = `<slot/> cannot come from an <include> in the content of a <use>; write it in the content itself`
	msgSlotItem      = `<slot/> cannot stand inside a list <item>: a list's rows hold no focus or state from outside the list`
	msgSlotRow       = `<slot/> cannot stand where content repeats per row: its content would need the row's alias (an each template, a <table>, a <list each=…>)`
	msgSlotContent   = `<slot/> takes no content: the content of the <use> fills it`
	msgSlotTwice     = `<template id=%q> has more than one <slot/> (the first is at %s)`
	msgUseOnlyFills  = `<%s> cannot be inside a <use>: its content fills a <slot/>, which holds content only`
	msgUseLooseText  = `text is not allowed directly inside <use>; wrap it in <text>`
	msgIncludeNoSlot = `<include> takes no content; to wrap content, make the fragment's markup a <template> with a <slot/> and place it with <use>`
)

// slotState is the composer's state for the slot.
type slotState struct {
	// inContent counts the contents of <use> elements being built in a
	// template's body as written (useContent), and contentIncl the
	// includes expanded since the innermost of them began: a slot met
	// while it is not 0 came from an include in a use's content.
	inContent, contentIncl int
	// outContent is set while the content of a <use> written outside every
	// template's body is built: as a fill, or in a fragment checked on its
	// own (decision 185's V001 reads it).
	outContent bool
	// inclAt is the chain of includes expanded since the body of the
	// template being built began (interned in inclRoots), which tells one
	// slot placed twice in one body apart (decision 193).
	inclAt    *inclStep
	inclRoots map[*RawNode]*inclStep
	// defSlots are the slots of the template's body being built as written
	// that no rule reported, in order.
	defSlots []slotRec
	// slotOK holds each template's one slot (decision 170), and receives
	// the templates that have one.
	slotOK   map[slotKey]bool
	receives map[*propDecl]bool
	// fillRoot holds the elements a fill placed in its slot's parent.
	fillRoot map[*ir.Node]bool
}

// inclStep is one include on a chain of includes inside a template's
// body: what placed a slot there.
type inclStep struct {
	parent *inclStep
	raw    *RawNode
	kids   map[*RawNode]*inclStep
}

// slotKey is a slot where a template's body places it: the template, the
// <slot/> as written, and the includes that placed it.
type slotKey struct {
	tmpl, slot *RawNode
	at         *inclStep
}

// slotRec is a slot of a template's body as written that no rule
// reported: its key, and its node, for the LOC of a second slot's V019.
type slotRec struct {
	raw  *RawNode
	at   *inclStep
	node *ir.Node
}

// fillPlace is where the content of a <use> is written: the context its
// fill is built in (the notes for the implementation of SPEC v0.4c §31.8).
// org and chain are recorded when the use is built (useNode): step 1's
// include recursion has unwound by step 3. The rest is recorded when its
// copy begins (copyTemplate), where the use stands.
type fillPlace struct {
	org   *ir.Origin
	chain []chainFile
	// The lengths of the alias scope, the template stack, the copy frames,
	// and the prop frames, where the use stands.
	nScope, nStack, nCopies, nPframes int
	prefix                            string
	inTmpl                            int
	viaTo                             *ir.Node
	inclAt                            *inclStep
	// inFill is set when the use is itself in a fill read outside every
	// template's body: the span of its content is already charged.
	inFill bool
}

// hasElement reports whether the raw element c holds an element.
func hasElement(c *RawNode) bool {
	for _, cc := range c.Children {
		if cc.Type == RawElement {
			return true
		}
	}
	return false
}

// pushIncl brings the include c onto inclAt, for an include expanded in
// a template's body (include); popIncl takes it back.
func (cp *composer) pushIncl(c *RawNode) *inclStep {
	prev := cp.inclAt
	m := &cp.inclRoots
	if prev != nil {
		m = &prev.kids
	}
	if *m == nil {
		*m = map[*RawNode]*inclStep{}
	}
	s := (*m)[c]
	if s == nil {
		s = &inclStep{parent: prev, raw: c}
		(*m)[c] = s
	}
	cp.inclAt = s
	cp.contentIncl++
	return prev
}

func (cp *composer) popIncl(prev *inclStep) {
	cp.inclAt = prev
	cp.contentIncl--
}

// toOwn moves the diagnostics reported after the first n to the template's
// own (cp.own), when a template's body is being built as written: they
// are reported whether or not a use copies it, once per written element.
func (b *builder) toOwn(n int) {
	cp := b.comp
	if !cp.selfCheck || cp.defDecl == nil || len(b.doc.Diags) == n {
		return
	}
	cp.own = append(cp.own, b.doc.Diags[n:]...)
	b.doc.Diags = b.doc.Diags[:n]
}

// contentOwn is toOwn for what an <include> in the content of a <use>
// written in a template's body reports about itself (its file cannot be
// read, is not a fragment, …): the definition reads that content, and
// may be the only one that does (decision 189, test 255). A V021 stays
// where template keeps it.
func (b *builder) contentOwn(n int) {
	cp := b.comp
	if cp.inContent == 0 || !cp.selfCheck || cp.defDecl == nil {
		return
	}
	kept := b.doc.Diags[:n]
	for _, d := range b.doc.Diags[n:] {
		if d.Code == "V021" {
			kept = append(kept, d)
		} else {
			cp.own = append(cp.own, d)
		}
	}
	b.doc.Diags = kept
}

// useContent builds the content of the <use> q, written as c, where a
// template's definition reads it (decision 189): in the use, with its
// includes expanded in the definition's budget, so that the checks of
// the body as written see it. In a fragment checked on its own, outside
// its templates, it gets the self-checks of its elements and nothing else
// (decision 195). Its character data is the use's own check
// (checkWrittenUses).
func (b *builder) useContent(q *ir.Node, c *RawNode) {
	cp := b.comp
	if !hasElement(c) {
		return
	}
	depth := b.depth
	b.depth++ // the use's own level: its content stands below it
	defer func() { b.depth = depth }()
	if cp.selfCheck {
		incl := cp.contentIncl
		cp.inContent++
		cp.contentIncl = 0
		defer func() { cp.inContent, cp.contentIncl = cp.inContent-1, incl }()
	} else {
		out := cp.outContent
		cp.outContent = true
		defer func() { cp.outContent = out }()
	}
	for _, cc := range c.Children {
		if cc.Type != RawElement {
			continue
		}
		if b.compose(q, cc, nil) {
			continue
		}
		b.attach(q, nil, b.node(cc, q))
	}
}

// takesNoElements reports whether n takes no element children: a slot in
// it is that element's V013 or V016, as an include is (§6.18.10).
func (b *builder) takesNoElements(n *ir.Node) bool {
	return n.Tag == "text" || (b.v2 && n.Tag == "column") || b.tableItem(n) ||
		(b.v2 && (n.Tag == "sparkline" || n.Tag == "hints")) || b.formNoContent(n) || leafKinds[n.Tag]
}

// noElements reports, when n takes no element children, its rule's
// diagnostic about the element c written in it, and whether it did: c is
// then dropped with what it holds (content, and a fill's elements, fill).
func (b *builder) noElements(n *ir.Node, c *RawNode) bool {
	switch {
	case n.Tag == "text":
		b.diag(ir.Error, "V013", c.Line, c.Col, n.Path, n.ID, "<text> cannot contain elements (found <%s>)", c.Name)
	case b.v2 && n.Tag == "column":
		// SPEC §6.9.1: a column's body is its cell template (text only).
		b.diagPlaced(ir.Error, "V016", c.Line, c.Col, n.Path, n.ID, "<column> holds its cell template as text, with {path} for the row's values; it cannot contain elements (found <%s>)", c.Name)
	case b.tableItem(n):
		// SPEC §6.9.1: a table's <item> is the row template, with no
		// children.
		b.diagPlaced(ir.Error, "V016", c.Line, c.Col, n.Path, n.ID, "a table's <item> is the row template: it takes class and class:NAME only, no children (found <%s>); the cells come from the <column> elements", c.Name)
	case (b.v2 && (n.Tag == "sparkline" || n.Tag == "hints")) || b.formNoContent(n):
		// SPEC §6.11, §6.12: no children (a hints' items come from the
		// keymap); a form control neither (v0.4b §6.16.1).
		b.diagPlaced(ir.Error, "V016", c.Line, c.Col, n.Path, n.ID, "<%s> takes no children (found <%s>)", n.Tag, c.Name)
	case leafKinds[n.Tag]:
		b.diag(ir.Error, "V013", c.Line, c.Col, n.Path, n.ID, "<%s> cannot contain elements (found <%s>)", n.Tag, c.Name)
	default:
		return false
	}
	return true
}

// slotIn handles the <slot/> c written in the content of n (content). It
// reports whether nothing more is to be done with c: a slot that a rule of
// §6.18.10 reports, the template's one slot in a copy, or one in an element
// that takes element children. Any other slot in an element that takes
// none falls to that element's rule.
func (b *builder) slotIn(n *ir.Node, c *RawNode) bool {
	if b.slotMisplaced(n, c) {
		return true
	}
	// A slot is judged in the body of the template that writes it, never
	// where a copy lands (decision 193): the definition took this one, so
	// it is built whatever its place in the copy takes, and the elements of
	// its fill meet that place's rules (fill). Only a table's <item> takes
	// element children in the one and not in the other.
	if !b.slotReceives(c) && b.takesNoElements(n) {
		return false
	}
	b.slot(n, c, nil)
	return true
}

// slotReceives reports whether the <slot/> c, being built in a copy, is
// its template's one slot, which receives the fill of the use copied.
func (b *builder) slotReceives(c *RawNode) bool {
	cp := b.comp
	if len(cp.copies) == 0 || cp.selfCheck {
		return false
	}
	return cp.slotOK[slotKey{cp.stack[len(cp.stack)-1].raw, c, cp.inclAt}]
}

// slotMisplaced checks the <slot/> c, a child of n, where it stands, and
// reports whether it receives nothing: a rule of §6.18.10 reported it, in
// the order they are tried (decision 169), or it is in a fragment checked
// on its own outside its templates. Its place is judged in the body of the
// template whose body holds it, as the definition reads it (decision 193):
// a copy only looks the slot up (slot). The checks of its own attributes
// and content are made wherever it is read, but in a copy.
func (b *builder) slotMisplaced(n *ir.Node, c *RawNode) bool {
	cp := b.comp
	if cp.selfCheck && cp.defDecl != nil {
		cp.defDecl.slot = true // the body has a <slot/> element (decision 167)
	}
	if len(cp.copies) > 0 && !cp.selfCheck {
		return false // judged in the definition
	}
	nDiags := len(b.doc.Diags)
	defer b.toOwn(nDiags)
	path := childPath(n, "slot")
	s := &ir.Node{Tag: "slot", Kind: "slot", Parent: n, Attrs: map[string]string{}, On: map[string]string{}, Line: c.Line, Col: c.Col, Path: path, Org: b.org}
	for _, a := range c.Attrs {
		b.attrGate(s, a, true) // a <slot> takes none: V002
	}
	for _, cc := range c.Children {
		if cc.Type == RawElement || ((cc.Type == RawText || cc.Type == RawCData) && !isXMLWhitespaceOnly(cc.Text)) {
			b.diag(ir.Error, "V013", cc.Line, cc.Col, path, "", msgSlotContent)
			break
		}
	}
	msg := ""
	switch {
	case cp.inTmpl == 0:
		if cp.selfOnly {
			return true // a fragment on its own: whether it is in a template is not known
		}
		msg = msgSlotOutside
	case cp.inContent > 0 && cp.contentIncl > 0:
		msg = msgSlotInclude
	case listItem(n) != nil:
		msg = msgSlotItem
	case isEachContainer(n) || b.eachContainer(n) != nil || n.Tag == "table" || (n.Tag == "list" && b.wrote(n, "each")):
		msg = msgSlotRow
	default:
		return false
	}
	b.diagOrg(b.org, true, ir.Error, "V001", c.Line, c.Col, path, "", msg)
	return true
}

// slot builds the <slot/> c, a child of n, that no rule reported: its node,
// kept for `tuimark ir`, and a level of the tree there. In a template's
// body as written it is recorded (endSlots); in a copy, when it is the
// template's one slot, it receives the fill of the use being copied.
func (b *builder) slot(n *ir.Node, c *RawNode, outer *ir.Node) {
	cp := b.comp
	b.checkDepth(b.depth + 1) // its own level, in the tree as composed
	s := &ir.Node{
		Tag: "slot", Kind: "slot", Parent: n,
		Attrs: map[string]string{}, On: map[string]string{},
		Line: c.Line, Col: c.Col, Path: childPath(n, "slot"),
		Org: b.wrapperOrg(),
	}
	writeInto(n, outer, s)
	if cp.selfCheck {
		cp.defSlots = append(cp.defSlots, slotRec{raw: c, at: cp.inclAt, node: s})
		return
	}
	if len(cp.copies) == 0 {
		return
	}
	s.Attrs["template"] = cp.stack[len(cp.stack)-1].node.ID
	if b.slotReceives(c) {
		b.fill(n, s, cp.copies[len(cp.copies)-1].q)
	}
}

// endSlots keeps the template t's one slot, written as c, among the slots
// its body as written holds that no rule reported (slots): the first.
// Each further one is V019, a check of the template's own (decision 170).
// A fragment checked on its own counts none of a template whose body holds
// an include, which may bring the first (decision 195).
func (b *builder) endSlots(t *ir.Node, c *RawNode, decl *propDecl, slots []slotRec) {
	cp := b.comp
	if len(slots) == 0 || (cp.selfOnly && decl.include) {
		return
	}
	first := slots[0]
	if cp.slotOK == nil {
		cp.slotOK, cp.receives = map[slotKey]bool{}, map[*propDecl]bool{}
	}
	cp.slotOK[slotKey{c, first.raw, first.at}] = true
	cp.receives[decl] = true
	for _, s := range slots[1:] {
		loc := fmt.Sprintf("%d:%d", first.node.Line, first.node.Col)
		if f, at := first.node.FileName(b.file), s.node.FileName(b.file); f != at {
			if f == "" {
				f = "<input>"
			}
			loc = f + ":" + loc
		}
		loc += ir.PlacedSuffix(first.node)
		n := len(b.doc.Diags)
		b.diagOrg(s.node.Org, false, ir.Error, "V019", s.node.Line, s.node.Col, s.node.Path, "", msgSlotTwice, shownID(t.ID), loc)
		cp.own = append(cp.own, b.doc.Diags[n:]...)
		b.doc.Diags = b.doc.Diags[:n]
	}
}

// fill builds the fill of the use q in the copy being built, at its slot
// s, a child of n: the elements of q's content, as if written in place of
// q (§6.18.10). The context of the copy is saved, that of the place where
// q is written set, and the copy's restored after, as a panic unwinds too:
// the origin and the chain of includes, the scoped-id prefix, the alias
// scope, the template stack and its onStack counts, the copy frames, the
// prop frames, inTmpl, viaTo, the includes since the template's body
// began, and outContent. The budget and depth frames stay the copy's, and
// the slot is a level of the tree.
func (b *builder) fill(n, s, q *ir.Node) {
	cp := b.comp
	info := cp.useOf[q]
	if info == nil || info.place == nil {
		return // no element in its content
	}
	pl := info.place
	org, chain, prefix, inTmpl, viaTo := b.org, cp.chain, cp.prefix, cp.inTmpl, cp.viaTo
	inclAt, out, depth := cp.inclAt, cp.outContent, b.depth
	scope := slices.Clone(cp.scope[pl.nScope:])
	stack := slices.Clone(cp.stack[pl.nStack:])
	copies := slices.Clone(cp.copies[pl.nCopies:])
	pframes := slices.Clone(cp.pframes[pl.nPframes:])
	cp.popTo(pl.nScope)
	for _, t := range stack {
		t.onStack--
	}
	cp.stack, cp.copies, cp.pframes = cp.stack[:pl.nStack], cp.copies[:pl.nCopies], cp.pframes[:pl.nPframes]
	defer func() {
		cp.popTo(pl.nScope)
		for _, a := range scope {
			cp.push(a)
		}
		for _, t := range stack {
			t.onStack++
		}
		cp.stack = append(cp.stack[:pl.nStack], stack...)
		cp.copies = append(cp.copies[:pl.nCopies], copies...)
		cp.pframes = append(cp.pframes[:pl.nPframes], pframes...)
		b.org, cp.chain, cp.prefix, cp.inTmpl, cp.viaTo = org, chain, prefix, inTmpl, viaTo
		cp.inclAt, cp.outContent, b.depth = inclAt, out, depth
	}()
	b.org, cp.chain, cp.prefix, cp.inTmpl, cp.viaTo = pl.org, pl.chain, pl.prefix, pl.inTmpl, pl.viaTo
	cp.inclAt, cp.outContent = pl.inclAt, pl.inTmpl == 0
	b.depth++ // the slot's level: the fill stands below it
	if pl.inTmpl == 0 && pl.org == nil && !pl.inFill {
		// Written in the document's own file: charged its content's span
		// when it is built, to the expansion under way (decision 177). One
		// written in a fragment or a template's body is already charged
		// with it, and so is one inside a fill read here.
		b.chargeBytes(info.raw.Inner)
	}
	start := len(n.Children)
	for _, cc := range info.raw.Children {
		if cc.Type != RawElement {
			continue
		}
		// The fill's elements follow the rules of the place where they land
		// (§6.18.10, decision 193): in an element that takes no element
		// children (a table's <item>), each is that element's diagnostic,
		// at the element, placed by where it is written (decision 176), and
		// is dropped. A forwarding slot is its own case (compose, slot).
		if cc.Name != "slot" && b.noElements(n, cc) {
			continue
		}
		if b.compose(n, cc, s) {
			continue
		}
		b.attach(n, s, b.node(cc, n))
	}
	if len(n.Children) > start && cp.fillRoot == nil {
		cp.fillRoot = map[*ir.Node]bool{}
	}
	for _, e := range n.Children[start:] {
		cp.fillRoot[e] = true
	}
}

// recordPlace records, as the use q's copy begins, where q stands: the
// context its fill is built in (fillPlace), when its content holds an
// element.
func (cp *composer) recordPlace(q *ir.Node) {
	info := cp.useOf[q]
	if info == nil || info.place == nil {
		return
	}
	pl := info.place
	pl.nScope, pl.nStack, pl.nCopies, pl.nPframes = len(cp.scope), len(cp.stack), len(cp.copies), len(cp.pframes)
	pl.prefix, pl.inTmpl, pl.viaTo, pl.inclAt, pl.inFill = cp.prefix, cp.inTmpl, cp.viaTo, cp.inclAt, cp.outContent
}

// placedBy reports whether the use q is on the chain of placements of the
// element e (Org.Placed).
func placedBy(e, q *ir.Node) bool {
	for p := e.Placer(); p != nil; p = p.Placer() {
		if p == q {
			return true
		}
	}
	return false
}

// classReaches reports whether the class of the use q reaches e, a
// top-level element of its copy (decision 172): always for an element that
// comes from the template, and, for one a fill placed, only when q is on
// its chain of placements: a fill written in the body of q's template
// comes from it, and the use's own fill, or a fill forwarded to it, does
// not.
func (cp *composer) classReaches(q, e *ir.Node) bool {
	return !cp.fillRoot[e] || placedBy(e, q)
}

// classUses is the uses whose class reaches e among those that copied it
// (CopiedBy), innermost first.
func (cp *composer) classUses(e *ir.Node) []*ir.Node {
	if !cp.fillRoot[e] {
		return e.CopiedBy
	}
	on := map[*ir.Node]bool{}
	for p := e.Placer(); p != nil; p = p.Placer() {
		on[p] = true
	}
	var out []*ir.Node
	for _, q := range e.CopiedBy {
		if on[q] {
			out = append(out, q)
		}
	}
	return out
}

// checkUseContent is the check of the content of the written use c
// against its template t (§6.18.10, decisions 167, 187, and 195): content
// toward a template with no <slot/> element is one V013 at its first
// element or character data; text directly in a use whose template has a
// slot that receives its content is V013 at that text. A fragment checked
// on its own makes neither toward a template whose body holds an include.
func (b *builder) checkUseContent(c *RawNode, t *propDecl, diag func(code string, line, col int, format string, args ...any)) {
	cp := b.comp
	if cp.selfOnly && t.include {
		return
	}
	if !t.slot {
		for _, cc := range c.Children {
			if cc.Type == RawElement || ((cc.Type == RawText || cc.Type == RawCData) && !isXMLWhitespaceOnly(cc.Text)) {
				diag("V013", cc.Line, cc.Col, msgUseNoSlot, t.id)
				return
			}
		}
		return
	}
	if !cp.receives[t] {
		return // every slot of it is reported: its content is dropped
	}
	for _, cc := range c.Children {
		if (cc.Type == RawText || cc.Type == RawCData) && !isXMLWhitespaceOnly(cc.Text) {
			diag("V013", cc.Line, cc.Col, msgUseLooseText)
			return
		}
	}
}
