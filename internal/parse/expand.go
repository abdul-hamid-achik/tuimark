package parse

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// <template> and <use> (SPEC v0.4b §6.18.3-§6.18.5, phase F2).
//
// Expansion follows the three steps of §6.18.1:
//
//  1. While the document is built (node, content), every <include> is
//     replaced by its fragment's elements (compose.go). A <template> that
//     stands in <tui> or <screen> is kept out of the tree as a ghost entry
//     of its parent (droppedAt), with its body built as written, getting
//     only the checks an element gets from itself (decision 20). A <use>
//     is a ghost entry too, waiting for step 3.
//  2. expandUses collects the templates, in document order, from the tree
//     that step 1 left (an include undone by V021 took its templates with
//     it), and reports the uses that close a cycle of templates (V019).
//  3. expandIn walks the tree in document order and replaces each waiting
//     use by a copy of its template's body, built where the use stands, as
//     if written there. A use inside a copy expands where it stands, while
//     the copy is built.
//
// A copy's elements are built from the template's raw elements: their ids
// scoped by the use's id (§6.18.4), their paths through the use's with
// aliases (§6.18.5), their #id references resolved in the copy (§6.18.4).
// What they wrote is kept in Node.Written for `tuimark ir` (decision 18).
// Each copy is an expansion of ADR 0034: it charges its template's source
// span and counts itself and every element it builds, and one that goes
// over a limit inserts nothing and reports V021.

// ghostKind says what a droppedAt entry is: the subtree of an unknown tag,
// or a <template> or a <use> standing in its parent, which is never a node.
type ghostKind uint8

const (
	ghostNone ghostKind = iota
	ghostTemplate
	ghostUse
)

// templating is the composer's state for <template> and <use>.
type templating struct {
	// tmplOf holds each <template> node step 1 may collect.
	tmplOf map[*ir.Node]*tmpl
	// tmpls are the collected templates by id (the first in document
	// order), and order all of them in document order.
	tmpls map[string]*tmpl
	order []*tmpl
	// useOf holds what a <use> node needs to expand.
	useOf map[*ir.Node]*useInfo
	// pending is set when a use waits for step 3; expanding while step 3
	// runs (a use then expands where it stands).
	pending, expanding bool
	// selfCheck is set while a template's body is built as written: its
	// elements get only the checks an element gets from itself, and a
	// <use> in it does not expand. defining is that template's node.
	selfCheck bool
	defining  *ir.Node
	// inTmpl counts the template bodies (as written, or copied) being
	// built: what a template may not hold is V001 inside one.
	inTmpl int
	// prefix is the id prefix of the copy being built: "U__" for the copy
	// of <use id="U"> (U scoped itself), "" outside copies.
	prefix string
	// scope is the aliases in scope in the copies being built, innermost
	// last: the with aliases of their uses, and the each aliases of their
	// elements, which shadow them (§6.18.5). byName indexes it: the
	// positions in scope of each name, innermost last, so a lookup costs
	// the same however many aliases are in scope. heads counts the with
	// aliases in scope by the first segment of the path they stand for
	// (an each alias of that name is renamed, see copyContent). push and
	// popTo keep the three in step.
	scope  []alias
	byName map[string][]int
	heads  map[string]int
	// renamed counts the each aliases renamed so far (ir.MarkAlias).
	renamed int
	// viaTo is the element of a copy (or the <use> in one) whose paths
	// are being substituted: substedAt records on it each path that goes
	// through a with alias (ir.Node.Vias), where it is written. nil
	// records nothing.
	viaTo *ir.Node
	// charge charges the expansion under way the bytes a substitution
	// adds (builder.chargeBytes; nil charges nothing).
	charge func(n int)
	// sufOrg and sufPrefix are the origin and prefix chargePlaced last
	// measured, sufCharge what it charges for them.
	sufOrg    *ir.Origin
	sufPrefix string
	sufCharge int
	// copies are the copies being built, outermost first; stack their
	// templates.
	copies []*copyFrame
	stack  []*tmpl
	// usedLog holds the templates a copy made used, in order (markUsed).
	usedLog []*tmpl
	// cyclic holds the raw <use> elements that close a cycle of templates:
	// reported once (V019), they never expand.
	cyclic map[*RawNode]bool
	// metCycle holds the raw <use> elements a copy met closing a cycle
	// that cyclic missed (expandUse): each is reported once, however many
	// copies meet it (the F2 re-review), and never expands.
	metCycle map[*RawNode]bool
	// classed holds the elements a use's class was appended to, in
	// classedOrder (copyTemplate): uniqueClasses leaves each name on
	// them once when the expansion ends.
	classed      map[*ir.Node]bool
	classedOrder []*ir.Node
	// useIDs and tmplIDs hold the first <use> and <template> of each id in
	// the id space (checkIDs); written maps an id an element of a template
	// writes to its first scoped id in document order (notANode).
	useIDs, tmplIDs map[string]*ir.Node
	written         map[string]string
}

// tmpl is a <template> standing in <tui> or <screen>.
type tmpl struct {
	node *ir.Node // its node, holding its body as written
	raw  *RawNode
	file string // the name of the file it is written in
	// chain is the chain of includes at the template: an include in its
	// body resolves against its file, and its cycle check walks it.
	chain []chainFile
	// self is what building its body as written reported: the checks an
	// element gets from itself, reported only when no use copies it (a
	// copy reports them itself, at the copy's place).
	self ir.Diags
	used bool
	// onStack counts its copies being built (a copy inside one of them is
	// a cycle).
	onStack int
	// firstID is the first id of its body in document order (V012 of a
	// use without id), found once.
	firstID string
	scanned bool
	// uses are the <use> nodes of its body as written, at any depth.
	uses []*ir.Node
}

// useInfo is what a <use> node needs to expand.
type useInfo struct {
	raw *RawNode
	// bad is set by a V011 on the use itself (its with, or its if): it
	// inserts nothing (decision 15).
	bad bool
	// depth is the use's own level in the tree `tuimark ir` prints, where
	// it holds its copy: the copy's elements stand one level below it
	// (checkDepth).
	depth int
}

// alias is an alias in scope in a copy: a with alias (use is its <use>,
// path what it stands for), or an each alias of an element of the copy
// (use nil), which shadows any alias of the same name around it. An each
// alias the parser renamed has its new name in path (ir.MarkAlias).
type alias struct {
	name, path string
	use        *ir.Node
	// eachRel is set for a with alias whose path starts with an each
	// alias in scope at its use (ir.Via.EachRel).
	eachRel bool
}

// push brings a into scope, innermost.
func (t *templating) push(a alias) {
	if t.byName == nil {
		t.byName, t.heads = map[string][]int{}, map[string]int{}
	}
	t.byName[a.name] = append(t.byName[a.name], len(t.scope))
	if a.use != nil {
		head, _, _ := strings.Cut(a.path, ".")
		t.heads[head]++
	}
	t.scope = append(t.scope, a)
}

// popTo takes the aliases pushed after the first n out of scope.
func (t *templating) popTo(n int) {
	for i := len(t.scope) - 1; i >= n; i-- {
		a := t.scope[i]
		s := t.byName[a.name]
		t.byName[a.name] = s[:len(s)-1]
		if a.use != nil {
			head, _, _ := strings.Cut(a.path, ".")
			t.heads[head]--
		}
	}
	t.scope = t.scope[:n]
}

// copyFrame is a copy being built: its elements whose focus= names an
// #id that no copy inside it has resolved.
type copyFrame struct {
	refs []*ir.Node
}

// notInTemplate are the tags a template's body may not hold, at any depth
// (SPEC v0.4b §6.18.3).
var notInTemplate = map[string]bool{
	"tui": true, "app": true, "fragment": true, "screen": true,
	"style": true, "keymap": true, "bind": true, "template": true,
}

// useAttrError is the V002 of an attribute a <use> does not take (SPEC
// v0.4b §6.18.3, ADR 0031).
const useAttrError = `attribute %q is not allowed on <use>: a <use> is not a node; give this copy a static class="…" and style that class, or put the attribute on the template's element`

// compose builds the composition element c, a child of n: an <include>,
// a <template>, or a <use>, or an element a template may not hold inside
// one, which is V001 and dropped. outer is the include or the use whose
// elements c is at the top of, nil when c is written in n. It reports
// whether c was one of them.
func (b *builder) compose(n *ir.Node, c *RawNode, outer *ir.Node) bool {
	cp := b.comp
	if cp.inTmpl > 0 && notInTemplate[c.Name] {
		// Reported instead of the placement message, wherever the
		// template is copied: a self-check, with no placement suffix.
		b.diag(ir.Error, "V001", c.Line, c.Col, childPath(n, c.Name), "", "<%s> cannot be inside a <template>: a template holds content only", c.Name)
		return true
	}
	switch c.Name {
	case "include":
		b.include(n, c, outer)
	case "template":
		b.template(n, c, outer)
	case "use":
		b.use(n, c, outer)
	default:
		return false
	}
	return true
}

// ghostEntry records a <template> or a <use> where it stands in n.
func (b *builder) ghostEntry(n, g *ir.Node, kind ghostKind) {
	if b.droppedIn == nil {
		b.droppedIn = map[*ir.Node][]droppedAt{}
	}
	b.droppedIn[n] = append(b.droppedIn[n], droppedAt{at: len(n.Children), node: g, ghost: kind})
}

// template builds the <template> element c, a child of n (SPEC v0.4b
// §6.18.3): its node, kept for `tuimark ir` with its body as written, and,
// when it stands in <tui> or <screen> with an id, a template that step 2
// collects.
func (b *builder) template(n *ir.Node, c *RawNode, outer *ir.Node) {
	cp := b.comp
	t := &ir.Node{
		Tag: "template", Kind: "template", Parent: n,
		Attrs: map[string]string{}, On: map[string]string{},
		Line: c.Line, Col: c.Col, Path: childPath(n, "template"),
		Org: b.wrapperOrg(),
	}
	for _, a := range c.Attrs {
		t.Attrs[a.Name] = a.Value
		t.Order = append(t.Order, a.Name)
	}
	t.ID = t.Attrs["id"]
	writeInto(n, outer, t)
	for _, a := range c.Attrs {
		b.attr(t, a)
	}
	collect := true
	if !cp.selfOnly && n.Tag != "tui" && n.Tag != "screen" {
		b.diagOrg(b.org, true, ir.Error, "V001", t.Line, t.Col, t.Path, t.ID, "<template> must be a direct child of <tui> or <screen>")
		collect = false
	}
	if t.ID == "" {
		b.diag(ir.Error, "V012", t.Line, t.Col, t.Path, "", "<template> needs an id, which <use template> names")
		collect = false
	}
	for _, cc := range c.Children {
		if (cc.Type == RawText || cc.Type == RawCData) && !isXMLWhitespaceOnly(cc.Text) {
			b.diag(ir.Error, "V013", cc.Line, cc.Col, t.Path, t.ID, "text is not allowed directly inside <template>; wrap it in <text>")
			break
		}
	}
	nDiags := len(b.doc.Diags)
	b.templateBody(t, c)
	if cp.selfOnly || !collect {
		return // never copied: its body's checks stand
	}
	// The definitions' V021 (an include of the body as written, which the
	// definitions' budget stopped) is reported whether or not a use copies
	// the template (the F2 re-review): what it stopped is every later
	// definition's checks, so a document whose definitions ran out never
	// validates clean. The other checks wait for step 3 (expandUses).
	var self ir.Diags
	kept := b.doc.Diags[:nDiags]
	for _, d := range b.doc.Diags[nDiags:] {
		if d.Code == "V021" {
			kept = append(kept, d)
		} else {
			self = append(self, d)
		}
	}
	b.doc.Diags, b.reported = kept, nil // set aside: forgotten by reported too
	if cp.tmplOf == nil {
		cp.tmplOf = map[*ir.Node]*tmpl{}
	}
	cp.tmplOf[t] = &tmpl{
		node: t, raw: c, file: b.org.FileOr(b.file),
		chain: slices.Clip(slices.Clone(cp.chain)), self: self,
	}
	b.ghostEntry(n, t, ghostTemplate)
}

// templateBody builds the body of the template c as written, in t: only
// the checks an element gets from itself (decision 20), an <include> in it
// expanded (§6.18.1 step 1), a <use> in it not.
func (b *builder) templateBody(t *ir.Node, c *RawNode) {
	cp := b.comp
	saved, savedDef := cp.selfCheck, cp.defining
	cp.selfCheck, cp.defining = true, t
	cp.inTmpl++
	b.depth++ // the template's own level, in the tree as written
	defer func() {
		cp.selfCheck, cp.defining = saved, savedDef
		cp.inTmpl--
		b.depth--
	}()
	for _, cc := range c.Children {
		if cc.Type != RawElement {
			continue
		}
		if b.compose(t, cc, nil) {
			continue
		}
		b.attach(t, nil, b.node(cc, t))
	}
}

// use builds the <use> element c, a child of n (SPEC v0.4b §6.18.3): its
// node, and, while step 3 runs, its copy where it stands; before, it waits
// for step 3.
func (b *builder) use(n *ir.Node, c *RawNode, outer *ir.Node) {
	cp := b.comp
	if n.Tag == "keymap" {
		b.diag(ir.Error, "V001", c.Line, c.Col, childPath(n, "use"), "", "a <template> holds no <bind> rows; share keymap rows with <include>")
		return
	}
	b.checkDepth(b.depth + 1) // its own level, in the tree as composed
	q := b.useNode(n, c, outer)
	if cp.selfOnly || cp.selfCheck {
		return // a template's body as written, or a fragment on its own
	}
	b.ghostEntry(n, q, ghostUse)
	if cp.expanding {
		b.expandUse(n, q)
		return
	}
	cp.pending = true
}

// useNode makes the node of the <use> element c, a child of n, and checks
// it: its attributes (decision 9), its content, and its with (§6.18.5).
// In a copy, its own paths resolve where it stands: its if and hidden,
// and the paths of its with.
func (b *builder) useNode(n *ir.Node, c *RawNode, outer *ir.Node) *ir.Node {
	cp := b.comp
	q := &ir.Node{
		Tag: "use", Kind: "use", Parent: n,
		Attrs: map[string]string{}, On: map[string]string{},
		Line: c.Line, Col: c.Col, Path: childPath(n, "use"),
		Org: b.wrapperOrg(),
	}
	for _, a := range c.Attrs {
		q.Attrs[a.Name] = a.Value
		q.Order = append(q.Order, a.Name)
	}
	b.chargePlaced() // as for any element a copy places (node)
	if id, ok := q.Attrs["id"]; ok {
		q.ID = cp.prefix + id
	}
	writeInto(n, outer, q)
	info := &useInfo{raw: c, depth: b.depth + 1}
	allowed := TagAttrsCurrent["use"]
	for _, a := range c.Attrs {
		if !allowed[a.Name] {
			b.attrErr(q, a, "V002", useAttrError, a.Name)
			continue
		}
		b.attr(q, a)
	}
	if _, ok := q.Attrs["if"]; ok && q.If == "" {
		info.bad = true // V011 (attr)
	}
	if id, ok := q.Attrs["id"]; ok && !isIDName(id) {
		// V003 (attr), the use's only diagnostic: a bad id scopes nothing,
		// so the use inserts nothing, as with a malformed with (the F2
		// review), and is no V012 either.
		info.bad = true
	}
	for _, cc := range c.Children {
		if cc.Type == RawElement || ((cc.Type == RawText || cc.Type == RawCData) && !isXMLWhitespaceOnly(cc.Text)) {
			b.diag(ir.Error, "V013", cc.Line, cc.Col, q.Path, q.ID, "<use> takes no content; Tuimark has no slots")
			break
		}
	}
	var clauses []ir.Each
	if w, ok := q.Attrs["with"]; ok {
		var msg string
		if clauses, msg = parseWith(w); msg != "" {
			b.diag(ir.Error, "V011", q.Line, q.Col, q.Path, q.ID, "%s", msg)
			info.bad = true
		}
	}
	if len(cp.copies) > 0 {
		q.Written = &ir.Written{Attrs: maps.Clone(q.Attrs), If: q.If, Classes: slices.Clone(q.Classes)}
		prev := cp.viaTo
		cp.viaTo = q
		defer func() { cp.viaTo = prev }()
		if q.If != "" {
			q.If = cp.substGuard("if", q.If)
			q.Attrs["if"] = q.If
		}
		if v, ok := q.Attrs["hidden"]; ok {
			q.Attrs["hidden"] = cp.substFlag("hidden", v)
		}
		for i := range clauses {
			clauses[i].Path = cp.substAt("with", i, clauses[i].Path)
		}
	}
	q.With = clauses
	if cp.useOf == nil {
		cp.useOf = map[*ir.Node]*useInfo{}
	}
	cp.useOf[q] = info
	return q
}

// parseWith parses with="path as alias, path as alias" (SPEC v0.4b
// §6.18.5): clauses of §7's each production separated by exactly ", ".
// msg is the V011 message when it is malformed or repeats an alias.
func parseWith(w string) (clauses []ir.Each, msg string) {
	seen := map[string]bool{}
	for _, part := range strings.Split(w, ", ") {
		e, err := ir.ParseEach(part)
		if err != nil {
			return nil, fmt.Sprintf(`with=%q: want "path as alias", separated by ", "`, w)
		}
		if seen[e.Alias] {
			return nil, fmt.Sprintf("with alias %q appears twice", e.Alias)
		}
		seen[e.Alias] = true
		clauses = append(clauses, e)
	}
	return clauses, ""
}

// errUse reports an error about a <use> itself: at the use, in its file,
// with no placement suffix (SPEC v0.4b §6.18.6), as for an include.
func (b *builder) errUse(q *ir.Node, code, format string, args ...any) {
	b.diagOrg(q.Org, false, ir.Error, code, q.Line, q.Col, q.Path, q.ID, format, args...)
}

// expandUses runs steps 2 and 3 of SPEC v0.4b §6.18.1 on the built
// document.
func (b *builder) expandUses() {
	cp := b.comp
	if cp == nil || b.doc.Root == nil {
		return
	}
	if len(cp.tmplOf) > 0 {
		cp.tmpls = map[string]*tmpl{}
		b.collectTemplates(b.doc.Root)
		b.checkCycles()
	}
	if cp.pending {
		cp.expanding = true
		b.expandIn(b.doc.Root)
		cp.expanding = false
	}
	cp.uniqueClasses()
	for _, t := range cp.order {
		if !t.used {
			// No use copied it: its elements get only the checks an
			// element gets from itself (decision 20).
			b.doc.Diags = append(b.doc.Diags, t.self...)
		}
	}
}

// collectTemplates collects the templates standing in n's subtree, in
// document order (step 2).
func (b *builder) collectTemplates(n *ir.Node) {
	cp := b.comp
	extra := b.droppedIn[n]
	j := 0
	for i := 0; i <= len(n.Children); i++ {
		for ; j < len(extra) && extra[j].at == i; j++ {
			switch e := extra[j]; e.ghost {
			case ghostTemplate:
				if t := cp.tmplOf[e.node]; t != nil {
					cp.order = append(cp.order, t)
					if cp.tmpls[t.node.ID] == nil {
						cp.tmpls[t.node.ID] = t
					}
				}
			case ghostNone:
				b.collectTemplates(e.node)
			}
		}
		if i < len(n.Children) {
			b.collectTemplates(n.Children[i])
		}
	}
}

// expandIn replaces each use waiting in n's subtree by its copy, in
// document order (step 3): n's children are built again, each copy's
// elements in its use's place.
func (b *builder) expandIn(n *ir.Node) {
	extra := b.droppedIn[n]
	waiting := false
	for _, e := range extra {
		if e.ghost == ghostUse {
			waiting = true
			break
		}
	}
	if !waiting {
		j := 0
		for i := 0; i <= len(n.Children); i++ {
			for ; j < len(extra) && extra[j].at == i; j++ {
				if extra[j].ghost == ghostNone {
					b.expandIn(extra[j].node)
				}
			}
			if i < len(n.Children) {
				b.expandIn(n.Children[i])
			}
		}
		return
	}
	old := n.Children
	n.Children = make([]*ir.Node, 0, len(old))
	b.droppedIn[n] = make([]droppedAt, 0, len(extra))
	j := 0
	for i := 0; i <= len(old); i++ {
		for ; j < len(extra) && extra[j].at == i; j++ {
			e := extra[j]
			e.at = len(n.Children)
			b.droppedIn[n] = append(b.droppedIn[n], e)
			switch e.ghost {
			case ghostUse:
				b.expandUse(n, e.node)
			case ghostNone:
				b.expandIn(e.node)
			}
		}
		if i < len(old) {
			n.Children = append(n.Children, old[i])
			b.expandIn(old[i])
		}
	}
}

// expandUse replaces the use q, a child of n, by a copy of its template's
// body, or reports why it inserts nothing (SPEC v0.4b §6.18.3-§6.18.5,
// decision 15).
func (b *builder) expandUse(n, q *ir.Node) {
	cp := b.comp
	info := cp.useOf[q]
	if info == nil || info.bad || cp.doc.dead {
		return
	}
	name, ok := q.Attrs["template"]
	if !ok || name == "" {
		b.errUse(q, "V019", `<use> needs template="ID"`)
		return
	}
	if cp.cyclic[info.raw] {
		return // V019, reported once by checkCycles
	}
	t := cp.tmpls[name]
	if t == nil {
		hint := ""
		if strings.HasPrefix(name, "#") {
			hint = fmt.Sprintf("; write template=%q, without #", shownID(name[1:]))
		}
		b.errUse(q, "V019", "<use template=%q> names no <template>%s", shownID(name), hint)
		return
	}
	if t.onStack > 0 {
		// A cycle checkCycles did not see, through an include that a
		// template's body as written left unexpanded (the definitions'
		// budget ran out): reported here, never stopped silently (the F2
		// review). The copies being built name it, from the template
		// whose copy holds q back to t; it never expands. A written use
		// (one raw element, its fragment parsed once) is reported once,
		// by the first copy that meets it.
		if !cp.metCycle[info.raw] {
			if cp.metCycle == nil {
				cp.metCycle = map[*RawNode]bool{}
			}
			cp.metCycle[info.raw] = true
			b.errUse(q, "V019", "<use template=%q> in <template id=%q> makes a cycle of templates: %s", shownID(name), shownID(cp.stack[len(cp.stack)-1].node.ID), cp.stackCycle(t))
		}
		return
	}
	if q.ID == "" && b.eachContainer(q) == nil {
		if x := b.firstID(t); x != "" {
			x = shownID(x) // written in the template, not at the use
			b.errUse(q, "V012", "<use template=%q> needs an id: its template holds ids (#%s), which the use scopes as ID__%s", shownID(name), x, x)
			return
		}
	}
	if len(q.With) > 0 {
		inScope := b.aliasesAround(q)
		for _, cl := range q.With {
			if inScope(cl.Alias) {
				b.errUse(q, "V011", "with alias %q repeats an alias in scope", cl.Alias)
				return
			}
		}
	}
	b.copyTemplate(n, q, t)
}

// aliasesAround returns whether a name is an alias in scope where the use
// q stands: an each alias of an element around it, or an alias of a copy
// being built, a with alias of an enclosing use included. The each aliases
// around it are collected once, so a with of many clauses costs one walk;
// a renamed alias counts by its written name (ir.MarkAlias).
func (b *builder) aliasesAround(q *ir.Node) func(string) bool {
	cp := b.comp
	around := map[string]bool{}
	for p := q.Parent; p != nil; p = p.Parent {
		if p.Each == "" {
			continue
		}
		if e, err := ir.ParseEachField(p.Each); err == nil {
			around[ir.Unmark(e.Alias)] = true
		}
	}
	return func(name string) bool {
		return around[name] || len(cp.byName[name]) > 0
	}
}

// copyTemplate builds the copy of t's body that the use q places in n.
// It is an expansion of ADR 0034: one that goes over a limit is undone,
// inserts nothing, and reports V021, and nothing expands after it.
func (b *builder) copyTemplate(n, q *ir.Node, t *tmpl) {
	cp := b.comp
	cp.used = true
	savedOrg, savedChain, savedPrefix := b.org, cp.chain, cp.prefix
	nChildren, nDroppedIn, nDropped, nEmpty := len(n.Children), len(b.droppedIn[n]), len(b.doc.dropped), len(n.EmptyCopies)
	fr := &expFrame{bg: cp.active()}
	nDiags, count, bytes := len(b.doc.Diags), fr.bg.count, fr.bg.bytes
	nScope, nStack, nCopies, inTmpl := len(cp.scope), len(cp.stack), len(cp.copies), cp.inTmpl
	nUsed, savedDepth := len(cp.usedLog), b.depth
	cp.frames = append(cp.frames, fr)
	defer func() {
		cp.frames = cp.frames[:len(cp.frames)-1]
		b.org, cp.chain, cp.prefix, b.depth = savedOrg, savedChain, savedPrefix, savedDepth
		cp.popTo(nScope)
		for _, s := range cp.stack[nStack:] {
			s.onStack--
		}
		cp.stack, cp.copies, cp.inTmpl = cp.stack[:nStack], cp.copies[:nCopies], inTmpl
		r := recover()
		if r == nil {
			return
		}
		o, ok := r.(*expansionOverflow)
		if !ok || o.frame != fr {
			panic(r)
		}
		n.Children = n.Children[:nChildren]
		n.EmptyCopies = n.EmptyCopies[:nEmpty]
		q.Children = nil
		b.doc.dropped = b.doc.dropped[:nDropped]
		if b.droppedIn != nil {
			b.droppedIn[n] = b.droppedIn[n][:nDroppedIn]
		}
		b.cutDiags(nDiags)
		cp.unuse(nUsed) // a discarded copy makes no template used
		fr.bg.count, fr.bg.bytes = count, bytes
		fr.bg.dead = true
		b.errUse(q, "V021", "%s", overflowMsg(fmt.Sprintf("use template=%q", shownID(q.Attrs["template"])), o))
	}()
	b.chargeBytes(t.raw.Span) // before anything is built
	b.countElement()          // the use itself, so it is the one that reports
	// Step 3 may build a copy outside the recursion that built n: its
	// elements stand one level below the use, wherever it is built.
	b.depth = cp.useOf[q].depth
	b.org = &ir.Origin{File: t.file, Placed: q}
	cp.chain = t.chain
	prefix := cp.prefix
	if q.ID != "" {
		prefix = q.ID + "__"
	}
	cp.prefix = prefix
	for _, cl := range q.With {
		cp.push(alias{name: cl.Alias, path: cl.Path, use: q, eachRel: eachRelAt(q, cl.Path)})
	}
	cp.stack = append(cp.stack, t)
	t.onStack++
	frame := &copyFrame{}
	cp.copies = append(cp.copies, frame)
	cp.inTmpl++
	for _, c := range t.raw.Children {
		if c.Type != RawElement {
			continue
		}
		if b.compose(n, c, q) {
			continue
		}
		b.attach(n, q, b.node(c, n))
	}
	// The top-level elements of the copy, those of a use at the top of
	// the body included: the use's class goes on each, after what they
	// already have (their own, then an inner use's), each name once
	// (decision 101), and its if and hidden decide whether they are in
	// the frame.
	tops := n.Children[nChildren:]
	classes := len(q.Attrs["class"])
	for _, e := range tops {
		// Charged the use's class list on each element it lands on (the
		// F2 re-review). The classes are merged once, when the expansion
		// ends (uniqueClasses, from CopiedBy), so a class costs the same
		// at every level of nested uses (the F2 final check).
		if len(q.Classes) > 0 {
			b.chargeBytes(classes)
			if !cp.classed[e] {
				if cp.classed == nil {
					cp.classed = map[*ir.Node]bool{}
				}
				cp.classed[e] = true
				cp.classedOrder = append(cp.classedOrder, e)
			}
		}
		e.CopiedBy = append(e.CopiedBy, q)
	}
	// A use whose copy places nothing here is still checked where it
	// stands (ir.Node.EmptyCopies): an inner use's empty copy is under q
	// too, and q's own, when nothing else of it is.
	for i := nEmpty; i < len(n.EmptyCopies); i++ {
		n.EmptyCopies[i] = append(n.EmptyCopies[i], q)
	}
	if len(tops) == 0 && len(n.EmptyCopies) == nEmpty {
		n.EmptyCopies = append(n.EmptyCopies, []*ir.Node{q})
	}
	left := resolveRefs(frame.refs, q, tops, prefix)
	if nCopies > 0 && len(left) > 0 {
		outer := cp.copies[nCopies-1]
		outer.refs = append(outer.refs, left...)
	}
	cp.markUsed(t)
}

// stackCycle names the cycle a use of t closes while the copies on the
// stack are built, t among them: from the template whose copy holds the
// use (the top of the stack) to t, then the templates copied inside t's
// copy, back to the top (cycleText). The templates on the stack are
// distinct, so the cycle is simple.
func (cp *composer) stackCycle(t *tmpl) string {
	top := len(cp.stack) - 1
	k := top
	for cp.stack[k] != t {
		k--
	}
	names := []string{cp.stack[top].node.ID}
	for _, s := range cp.stack[k:top] {
		names = append(names, s.node.ID)
	}
	return cycleText(names[:min(len(names), cycleShown)], max(0, len(names)-cycleShown))
}

// chargePlaced charges an element a copy places (the F2 re-review): its
// scoped-id prefix, id or not, and the descriptor of its placement suffix
// as a diagnostic shows it (ir.Origin.PlacedBy, `, placed by <use …>`),
// less the bytes of the prefix the descriptor already shows (the id of
// the use that scoped it). Together they bound the ids a copy makes,
// nested uses with long ids included, and what every diagnostic about one
// of its elements may print, whether or not one is printed. The
// location, " at FILE:LINE:COL", is not charged (the F2 final check): the
// same bytes validate the same under any file name, and the file system
// bounds a name as the element cap bounds how often it is printed.
func (b *builder) chargePlaced() {
	cp := b.comp
	if cp == nil || len(cp.copies) == 0 {
		return
	}
	if cp.sufOrg != b.org || cp.sufPrefix != cp.prefix {
		cp.sufOrg, cp.sufPrefix = b.org, cp.prefix
		n := len(cp.prefix) + len(b.org.PlacedBy())
		if p := b.org.Placed; p != nil && p.Kind == "use" && p.ID != "" && cp.prefix == p.ID+"__" {
			n -= len(strings.TrimSuffix(shownID(p.ID), "…"))
		}
		cp.sufCharge = n
	}
	b.chargeBytes(cp.sufCharge)
}

// uniqueClasses gives every element a use's class lands on the classes of
// the uses that copied it (decision 101): its own, then an inner use's,
// then an outer one's (CopiedBy, innermost first), each name once, at its
// first place. One pass when the expansion ends costs what the classes
// cost, where a set rebuilt at every level of nested uses cost the square
// of their depth (the F2 final check).
func (cp *composer) uniqueClasses() {
	seen := map[string]bool{}
	for _, e := range cp.classedOrder {
		n := len(e.Classes)
		for _, q := range e.CopiedBy {
			n += len(q.Classes)
		}
		clear(seen)
		out := make([]string, 0, n)
		add := func(cs []string) {
			for _, c := range cs {
				if !seen[c] {
					seen[c] = true
					out = append(out, c)
				}
			}
		}
		add(e.Classes)
		for _, q := range e.CopiedBy {
			add(q.Classes)
		}
		e.Classes = slices.Clip(out)
	}
	cp.classed, cp.classedOrder = nil, nil
}

// markUsed records that a use copied t; unuse(n) takes back what was
// recorded after the first n, for the copies a V021 discards.
func (cp *composer) markUsed(t *tmpl) {
	if !t.used {
		t.used = true
		cp.usedLog = append(cp.usedLog, t)
	}
}

func (cp *composer) unuse(n int) {
	for _, t := range cp.usedLog[n:] {
		t.used = false
	}
	cp.usedLog = cp.usedLog[:n]
}

// resolveRefs resolves the #x references of a copy, made by the use q
// (SPEC v0.4b §6.18.4): one that names the local id of an element of the
// copy names that element. It returns the others, which resolve as if
// written in q's place. A use without id makes no local ids.
func resolveRefs(refs []*ir.Node, q *ir.Node, tops []*ir.Node, prefix string) []*ir.Node {
	if len(refs) == 0 || q.ID == "" {
		return refs
	}
	local := map[string]bool{}
	var walk func(ns []*ir.Node)
	walk = func(ns []*ir.Node) {
		for _, x := range ns {
			if strings.HasPrefix(x.ID, prefix) {
				local[x.ID[len(prefix):]] = true
			}
			walk(x.Children)
		}
	}
	walk(tops)
	var left []*ir.Node
	for _, r := range refs {
		if x := r.Attrs["focus"][1:]; local[x] {
			r.Attrs["focus"] = "#" + prefix + x
		} else {
			left = append(left, r)
		}
	}
	return left
}

// copyContent builds what an element of a copy holds, after its paths
// went through the aliases in scope (SPEC v0.4b §6.18.5): its own paths
// resolve in the scope around it, its key and what it holds inside its
// each alias, which shadows a with alias of the same name. It keeps what
// the element wrote in n.Written.
func (b *builder) copyContent(n *ir.Node, r *RawNode) {
	cp := b.comp
	w := &ir.Written{Attrs: maps.Clone(n.Attrs), Bind: n.Bind, Each: n.Each, If: n.If, Classes: slices.Clone(n.Classes)}
	n.Written = w
	depth := len(cp.scope)
	prev := cp.viaTo
	cp.viaTo = n // the paths substituted below are n's (substedAt)
	defer func() { cp.viaTo = prev }()
	if n.Bind != "" {
		n.Bind = cp.substAt("bind", 0, n.Bind)
		n.Attrs["bind"] = n.Bind
	}
	if n.If != "" {
		n.If = cp.substGuard("if", n.If)
		n.Attrs["if"] = n.If
	}
	for _, k := range [...]string{"hidden", "disabled", "open"} {
		if v, ok := n.Attrs[k]; ok {
			n.Attrs[k] = cp.substFlag(k, v)
		}
	}
	if v, ok := n.Attrs["checked"]; ok {
		n.Attrs["checked"] = cp.substAt("checked", 0, v)
	}
	for _, k := range [...]string{"title", "placeholder", "label", "short"} {
		if v, ok := n.Attrs[k]; ok && !(k == "label" && n.Tag == "select") {
			n.Attrs[k] = cp.substText(k, v)
		}
	}
	for i := range n.ClassGuards {
		g := &n.ClassGuards[i]
		g.Guard = cp.substGuard(ClassGuardPrefix+g.Name, g.Guard)
		n.Attrs[ClassGuardPrefix+g.Name] = g.Guard
	}
	if n.Each != "" {
		if e, err := ir.ParseEach(n.Each); err == nil {
			// The alias is renamed when a with alias in scope stands for a
			// path that starts with its name: inside this each, that path
			// still resolves where its use stands (§6.18.5), so it must not
			// meet this alias at run time. A path written with the alias
			// takes the new name through subst; messages and events show
			// the written one (ir.Unmark), and n.Written keeps it.
			name := e.Alias
			if cp.heads[name] > 0 {
				cp.renamed++
				name = ir.MarkAlias(e.Alias, cp.renamed)
			}
			cp.grow(cp.growth(e.Path) + len(name) - len(e.Alias))
			n.Each = cp.substedAt("each", 0, e.Path) + " as " + name
			n.Attrs["each"] = n.Each
			if a := cp.lookup(e.Alias); a != nil && a.use != nil {
				// Reported at the element; the copy is still inserted,
				// and inside this each its own alias is the innermost.
				b.diagOrg(n.Org, false, ir.Error, "V011", n.Line, n.Col, n.Path, n.ID, "each alias %q repeats the with alias of %s at %s", e.Alias, describeUse(a.use), a.use.Loc(true))
			}
			al := alias{name: e.Alias}
			if name != e.Alias {
				al.path = name
			}
			cp.push(al)
		}
	}
	// Per row, inside the each alias: a key, and a select's option label.
	if v, ok := n.Attrs["key"]; ok {
		n.Attrs["key"] = cp.substAt("key", 0, v)
	}
	if v, ok := n.Attrs["label"]; ok && n.Tag == "select" {
		n.Attrs["label"] = cp.substText("label", v)
	}
	if f, ok := n.Attrs["focus"]; ok && strings.HasPrefix(f, "#") && isIDName(f[1:]) {
		fr := cp.copies[len(cp.copies)-1]
		fr.refs = append(fr.refs, n)
	}
	b.content(n, r)
	cp.viaTo = n // content built n's children, each of them its own target
	w.Text = n.Text
	n.Text = cp.substText("", n.Text)
	cp.popTo(depth)
}

// describeUse names a <use> as a placement suffix does.
func describeUse(u *ir.Node) string {
	if u.ID != "" {
		return fmt.Sprintf("<use id=%q>", shownID(u.ID))
	}
	return fmt.Sprintf("<use template=%q>", shownID(u.Attrs["template"]))
}

// lookup returns the innermost alias in scope called name, or nil.
func (t *templating) lookup(name string) *alias {
	s := t.byName[name]
	if len(s) == 0 {
		return nil
	}
	return &t.scope[s[len(s)-1]]
}

// substAt is the path p, written in slot as its seg-th path (ir.ViaKey),
// as it resolves in the copies being built: a path whose first segment is
// a with alias in scope (not shadowed by an each alias) stands for the
// alias's path followed by the other segments, and one whose first
// segment is a renamed each alias takes its new name. The bytes it adds
// are charged to the expansion under way (grow).
func (t *templating) substAt(slot string, seg int, p string) string {
	t.grow(t.growth(p))
	return t.substedAt(slot, seg, p)
}

// substedAt is substAt, charging nothing. A path that went through a with
// alias is recorded on viaTo (ir.Node.Vias) under where it is written
// (slot, seg), not under what it becomes, so two paths written apart that
// stand for one path keep their own: its binding diagnostics name it as
// written and say which use's alias it went through (SPEC v0.4b
// §6.18.5), and the runtime reads its flags as optional props (decision
// 100).
func (t *templating) substedAt(slot string, seg int, p string) string {
	if len(t.scope) == 0 {
		return p
	}
	head, rest, more := strings.Cut(p, ".")
	a := t.lookup(head)
	if a == nil || a.path == "" {
		return p
	}
	out := a.path
	if more {
		out = a.path + "." + rest
	}
	if a.use != nil && t.viaTo != nil {
		if t.viaTo.Vias == nil {
			t.viaTo.Vias = map[ir.ViaKey]*ir.Via{}
		}
		t.viaTo.Vias[ir.ViaKey{Slot: slot, Seg: seg}] = &ir.Via{Written: p, Base: a.path, Use: a.use, EachRel: a.eachRel}
	}
	return out
}

// eachRelAt reports whether path p, a with path of the use q resolved
// where q stands, starts with an each alias in scope there: that of an
// element around q (a container each, a list, or a table, the copy's own
// elements included, whose renamed alias p then holds).
func eachRelAt(q *ir.Node, p string) bool {
	head, _, _ := strings.Cut(p, ".")
	for n := q.Parent; n != nil; n = n.Parent {
		if n.Each == "" {
			continue
		}
		if e, err := ir.ParseEachField(n.Each); err == nil && e.Alias == head {
			return true
		}
	}
	return false
}

// growth is len(subst(p)) - len(p), computed without building it.
func (t *templating) growth(p string) int {
	if len(t.scope) == 0 {
		return 0
	}
	head, _, _ := strings.Cut(p, ".")
	a := t.lookup(head)
	if a == nil || a.path == "" {
		return 0
	}
	return len(a.path) - len(head)
}

// grow charges the expansion under way the n bytes a substitution added
// to a value, when it added any (ADR 0034, the F2 re-review): a with
// alias standing for a long path makes every value that names it longer,
// once per copy, which the template's span does not account for.
func (t *templating) grow(n int) {
	if n > 0 && t.charge != nil {
		t.charge(n)
	}
}

// substGuard is substAt for path or !path, written in attribute slot.
func (t *templating) substGuard(slot, g string) string {
	if rest, neg := strings.CutPrefix(g, "!"); neg {
		return "!" + t.substAt(slot, 0, rest)
	}
	return t.substAt(slot, 0, g)
}

// substFlag is substAt for true, false, path, or !path, written in
// attribute slot.
func (t *templating) substFlag(slot, f string) string {
	if f == "true" || f == "false" {
		return f
	}
	return t.substGuard(slot, f)
}

// substText is substAt for each {path} of a template string written in
// slot ("" for an element's text), its k-th path as segment k. A string
// that is not a valid template (V003) is left as written. What the whole
// string grows by is charged before it is built (grow).
func (t *templating) substText(slot, s string) string {
	if len(t.scope) == 0 || !strings.Contains(s, "{") {
		return s
	}
	segs, err := ir.ParseInterp(s)
	if err != nil {
		return s
	}
	n := 0
	for _, sg := range segs {
		if sg.Path != "" {
			n += t.growth(sg.Path)
		}
	}
	t.grow(n)
	var out strings.Builder
	k := 0
	for _, sg := range segs {
		if sg.Path == "" {
			out.WriteString(sg.Lit)
			continue
		}
		out.WriteString("{" + t.substedAt(slot, k, sg.Path) + "}")
		k++
	}
	return out.String()
}

// firstID is the first id of t's body in document order, a <use>'s id
// included, or "" (V012 of a use without id, SPEC v0.4b §6.18.4).
func (b *builder) firstID(t *tmpl) string {
	if !t.scanned {
		t.scanned = true
		t.firstID = firstWrittenID(written(t.node))
	}
	return t.firstID
}

// written is n's children as written: with the <include> and <use> nodes
// in their places.
func written(n *ir.Node) []*ir.Node {
	if n.Source != nil {
		return n.Source
	}
	return n.Children
}

func firstWrittenID(ns []*ir.Node) string {
	for _, x := range ns {
		if x.Kind != "include" && x.ID != "" {
			return x.ID
		}
		if x.Kind == "use" {
			continue
		}
		if id := firstWrittenID(written(x)); id != "" {
			return id
		}
	}
	return ""
}

// usesIn appends the <use> nodes of a body as written, at any depth.
func usesIn(out []*ir.Node, ns []*ir.Node) []*ir.Node {
	for _, x := range ns {
		if x.Kind == "use" {
			out = append(out, x)
			continue
		}
		out = usesIn(out, written(x))
	}
	return out
}

// ghostID records the id of a <use> or a <template> (checkIDs): it is in
// the id space, and names no node (SPEC v0.4b §6.18.4).
func (b *builder) ghostID(n *ir.Node) bool {
	cp := b.comp
	if cp == nil {
		return false
	}
	var m *map[string]*ir.Node
	switch n.Kind {
	case "use":
		m = &cp.useIDs
	case "template":
		m = &cp.tmplIDs
	default:
		return false
	}
	if *m == nil {
		*m = map[string]*ir.Node{}
	}
	if _, ok := (*m)[n.ID]; !ok {
		(*m)[n.ID] = n
	}
	return true
}

// sameIDCopies reports whether two elements with one id were scoped by two
// <use> elements with the same id: such copies are not compared, since
// their shared prefix is already V004 (SPEC v0.4b §6.18.4).
func sameIDCopies(n, prev *ir.Node) bool {
	a, c := idOwner(n), idOwner(prev)
	return a != nil && c != nil && a != c && a.ID == c.ID
}

// idOwner is the innermost <use> with an id that placed n, or nil.
func idOwner(n *ir.Node) *ir.Node {
	for p := n.Placer(); p != nil; p = p.Placer() {
		if p.Kind == "use" && p.ID != "" {
			return p
		}
	}
	return nil
}

// notANode is idTarget's reason for an id that names no node, in a
// document with composition (SPEC v0.4b §6.18.4): a <use>'s, a
// <template>'s, or missing, with the scoped id of an element of a template
// that writes it.
func (b *builder) notANode(id string) string {
	cp := b.comp
	if q := cp.useIDs[id]; q != nil {
		if first := firstNodeID(q.Children); first != "" {
			return "is a <use>, not a node; name a node of its copy, such as #" + shownID(first)
		}
		return "is a <use>, not a node, and its copy has no id"
	}
	if cp.tmplIDs[id] != nil {
		return "is a <template>, which is never in the frame"
	}
	if cp.written == nil {
		cp.written = map[string]string{}
		b.walkAll(b.doc.Root, false, func(n *ir.Node, dropped bool) {
			if dropped || n.Written == nil {
				return
			}
			if w := n.Attrs["id"]; w != "" && w != n.ID {
				if _, ok := cp.written[w]; !ok {
					cp.written[w] = n.ID
				}
			}
		})
	}
	if s := cp.written[id]; s != "" {
		return idMissing + "; ids in a <template> are scoped by their <use>: #" + shownID(s)
	}
	return idMissing
}

// firstNodeID is the first id of a node in a copy as written, in document
// order (a <use>'s own id is not a node's).
func firstNodeID(ns []*ir.Node) string {
	for _, x := range ns {
		if x.Kind != "use" && x.Kind != "include" && x.ID != "" {
			return x.ID
		}
		if id := firstNodeID(x.Children); id != "" {
			return id
		}
	}
	return ""
}
