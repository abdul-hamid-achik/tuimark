package parse

import (
	"slices"
	"strings"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// Props (SPEC v0.4c §6.18.9, ADR 0035, phase C1).
//
// A template declares its props (`<template id="T" props="title max">`),
// each <use> of it passes them as attributes, and its body reads them as
// {$name}. A {$name} is replaced when the copy is built, as text, before
// anything else reads the value: here, while node, content, and useNode
// build an element of a copy, from the props of the use whose copy is
// being built (propFrame, pushed by copyTemplate). The written raw tree
// is never cloned, so every map keyed by a raw node (cp.cyclic,
// cp.metCycle, useInfo.raw), the span charge, and the checks made as
// written key on the element as written.
//
// Where an element stands decides what a {$name} in it means (propMode):
//
//   - in a template's body as written (the definition), it gets the
//     template's own checks (a malformed or undeclared reference, one
//     where a prop cannot stand, one inside braces), reported once, with
//     no suffix, whether or not the template is used; the value is
//     otherwise not checked as written (decision 162);
//   - in a copy, it is replaced, and the value is checked as written by
//     hand, a diagnostic there naming the use that wrote the text
//     (decisions 161 and 194);
//   - outside every template's body, it is the V003 of a {$name} outside
//     a template, reported instead of any other diagnostic about the value.
//
// The checks of a <use> against its template (decision 187), which read
// only an attribute's name and whether the use has content, are made once
// per written use after step 2 of §6.18.1 (checkWrittenUses).

// propMode is where the element being built stands, for its {$name}.
type propMode uint8

const (
	// pmNone: a document that writes version, where {$…} means what it
	// meant in tuimark 0.4.1.
	pmNone propMode = iota
	// pmOutside: outside every template's body.
	pmOutside
	// pmUnknown: a fragment checked on its own, outside its templates:
	// whether the fragment is included in a template's body is not known
	// there, so a {$name} gets only the checks that do not depend on a
	// template (§15.1: a malformed reference, one where a prop cannot
	// stand, one inside braces), and its value is otherwise muted
	// (decision 103: a fragment on its own gets only what holds wherever
	// it is placed).
	pmUnknown
	// pmDef: a template's body as written.
	pmDef
	// pmCopy: an element of a copy.
	pmCopy
)

// propMode is where the element being built stands.
func (b *builder) propMode() propMode {
	cp := b.comp
	switch {
	case cp == nil:
		return pmNone
	case cp.selfCheck:
		return pmDef
	case len(cp.pframes) > 0:
		return pmCopy
	case cp.selfOnly:
		return pmUnknown
	}
	return pmOutside
}

// The messages of §6.18.9 and §6.18.3 (v0.4c).
const (
	msgPropSeparator   = `props=%q: separate the names with one space`
	msgPropInvalid     = `invalid prop name %q in props: want [a-z_][a-z0-9_-]*`
	msgPropTwice       = `prop %q is declared twice`
	msgPropUseOwn      = `%q cannot be a prop: <use> takes %s="…" for itself`
	msgPropUndeclared  = `{$%s} names no prop of <template id=%q>; declare it in props="…"`
	msgPropMalformed   = `malformed prop reference %q: want {$name}, name [a-z_][a-z0-9_-]*`
	msgPropPath        = `{$%s} cannot stand in %s: a prop is text; pass a path with with="…"`
	msgPropOther       = `{$%s} cannot stand in %s; write the value itself`
	msgPropInsidePath  = `{$%s} cannot stand inside {…}: a prop is text; pass a path with with="…"`
	msgPropInsideText  = `{$%s} cannot stand inside {…}: a prop is text; write the value without braces`
	msgPropOutside     = `{$%s} is outside a <template>: props exist only in a template's body`
	msgPropPairPath    = `prop %q makes the pair %q: a prop is text; pass a path with with="…"`
	msgPropPairText    = `prop %q makes the pair %q: a prop is text; write the value without braces`
	msgUsePropAttr     = `attribute %q is not allowed on <use>: a <use> is not a node; give this copy a static class="…" and style that class, put the attribute on the template's element, or declare it in the template's props`
	msgUseNoSlot       = `<template id=%q> has no <slot/>`
	propSeparatorSpace = " "
)

// pref is a prop reference in a value as written: a `{$` that has a `}`
// after it, up to that `}` (§6.18.9, decision 158).
type pref struct {
	start, end int // the reference is s[start:end], braces included
	name       string
	ok         bool // a well-formed {$name}
}

// scanRefs returns the prop references of s, in order.
func scanRefs(s string) []pref {
	var out []pref
	for i := 0; ; {
		k := strings.Index(s[i:], "{$")
		if k < 0 {
			return out
		}
		k += i
		j := strings.IndexByte(s[k+2:], '}')
		if j < 0 {
			return out
		}
		j += k + 2
		name := s[k+2 : j]
		out = append(out, pref{start: k, end: j + 1, name: name, ok: ir.IsGuardName(name)})
		i = j + 1
	}
}

// hasPropRef reports whether s holds a well-formed {$name}.
func hasPropRef(s string) bool {
	if !strings.Contains(s, "{$") {
		return false
	}
	for _, r := range scanRefs(s) {
		if r.ok {
			return true
		}
	}
	return false
}

// insideBraces returns, for each reference of s, whether it stands inside
// a pair of braces of s as written, nil when none does (decision 186):
// braces pair as §7 says, a `{` and the first `}` after it, each
// well-formed reference read as one unit that holds no brace (a malformed
// one is text, whose braces pair as any other).
func insideBraces(s string, refs []pref) []bool {
	if !strings.ContainsAny(s, "{}") {
		return nil
	}
	type brace struct {
		at   int
		open bool
	}
	var bs []brace
	r := 0
	for i := 0; i < len(s); i++ {
		if r < len(refs) && i == refs[r].start {
			r++
			if refs[r-1].ok {
				i = refs[r-1].end - 1
				continue
			}
		}
		switch s[i] {
		case '{':
			bs = append(bs, brace{i, true})
		case '}':
			bs = append(bs, brace{i, false})
		}
	}
	var in []bool
	for k := 0; k < len(bs); k++ {
		if !bs[k].open {
			continue
		}
		m := k + 1
		for m < len(bs) && bs[m].open {
			m++
		}
		if m == len(bs) {
			break
		}
		p, q := bs[k].at, bs[m].at
		for x, rf := range refs {
			if rf.start > p && rf.end <= q {
				if in == nil {
					in = make([]bool, len(refs))
				}
				in[x] = true
			}
		}
		k = m
	}
	return in
}

// bracePairs returns the pairs of braces of s as §7 reads them: a `{` and
// the first `}` after it, the indexes of both.
func bracePairs(s string) [][2]int {
	var out [][2]int
	for i := 0; i < len(s); i++ {
		if s[i] != '{' {
			continue
		}
		j := strings.IndexByte(s[i+1:], '}')
		if j < 0 {
			break
		}
		j += i + 1
		out = append(out, [2]int{i, j})
		i = j
	}
	return out
}

// propPlace is what a {$name} in an attribute called name is (§6.18.9,
// decision 156): a place where it may stand, an attribute that takes a
// path, or any other.
type propPlace uint8

const (
	plText propPlace = iota
	plPath
	plOther
)

// propTextAttrs are the attributes of the closed list where a {$name} may
// stand, besides the action names of the on:* attributes.
var propTextAttrs = map[string]bool{
	"title": true, "placeholder": true, "label": true, "short": true,
	"width": true, "height": true, "gap": true, "pad": true, "border": true,
	"max": true, "min": true, "mark": true, "keycap": true, "value": true,
	"wrap": true, "axis": true, "priority": true, "scale": true,
	"class": true, "style": true,
}

// propPathAttrs are the attributes that take a path (and a <use>'s with).
var propPathAttrs = map[string]bool{
	"bind": true, "each": true, "key": true, "checked": true, "if": true,
	"hidden": true, "disabled": true, "open": true, "with": true,
	"runs": true, // SPEC v0.4d §6.20.1
}

func placeOf(name string) propPlace {
	switch {
	case name == "", propTextAttrs[name], strings.HasPrefix(name, "on:"):
		return plText
	case propPathAttrs[name], strings.HasPrefix(name, ClassGuardPrefix):
		return plPath
	}
	return plOther
}

// isTemplate7 reports whether the attribute name of a tag ("" for its
// body) is a template of §7, where a {path} may stand (decision 191).
func isTemplate7(tag, name string) bool {
	switch name {
	case "title", "placeholder":
		return true
	case "label":
		return tag == "button" || tag == "checkbox" || tag == "select" || tag == "tab"
	case "short":
		return tag == "tab"
	case "":
		return tag == "text" || tag == "button" || tag == "column"
	}
	return false
}

// useOwnAttrs are a <use>'s own attributes, which cannot be props.
var useOwnAttrs = map[string]bool{"id": true, "template": true, "with": true, "if": true, "hidden": true, "class": true}

// propShaped reports whether name, an attribute of a <use>, could be a
// prop: a prop name (§7) that is not one of the use's own.
func propShaped(name string) bool {
	return ir.IsGuardName(name) && !useOwnAttrs[name]
}

// propDecl is what a template declares and what its body as written
// holds, for its copies and the checks of its uses.
type propDecl struct {
	id    string
	names map[string]bool
	// sites are the values of the body where a copy's text lands that
	// hold a reference: what decision 160 checks in each copy.
	sites []propSite
	// slot and include say whether the body as written holds a <slot>
	// element (a use's content then has somewhere to go, decision 187),
	// and an <include> (decision 195).
	slot, include bool
}

// propSite is one value of a template's body that holds a reference.
type propSite struct {
	s    string
	refs []pref
	t7   bool // a template of §7
}

// pwriter is a <use>'s prop attribute that wrote, as literal text, part
// of a passed value: where a brace it brings is reported (decision 160),
// and what a diagnostic at a value that holds its text names (decision
// 194). lvl is the use's depth in the copies (0 for one written outside
// every copy): the outermost writer has the lowest.
type pwriter struct {
	use       *ir.Node
	line, col int
	name      string
	lvl       int
}

// prun is a run of a value's text that ends at end, written by w (nil for
// the template's own text).
type prun struct {
	end int
	w   *pwriter
}

// pval is a value with the writer of each of its runs.
type pval struct {
	s    string
	runs []prun
}

func (v *pval) add(end int, w *pwriter) {
	if n := len(v.runs); n > 0 && v.runs[n-1].w == w {
		v.runs[n-1].end = end
		return
	}
	v.runs = append(v.runs, prun{end, w})
}

// outer is the outermost writer of v's text, nil when a use wrote none.
func (v *pval) outer() *pwriter {
	var best *pwriter
	for _, r := range v.runs {
		if r.w != nil && (best == nil || r.w.lvl < best.lvl) {
			best = r.w
		}
	}
	return best
}

// writerAt is the writer of the byte at i.
func (v *pval) writerAt(i int) *pwriter {
	for _, r := range v.runs {
		if i < r.end {
			return r.w
		}
	}
	return nil
}

// propFrame is the props of the copy being built: the values its use
// passes, of the props its template declares.
type propFrame struct {
	decl *propDecl
	vals map[string]*pval
	q    *ir.Node // the use whose copy it is
}

func (fr *propFrame) val(r pref) *pval {
	if !r.ok {
		return nil
	}
	return fr.vals[r.name]
}

// subst is s, a value of the template's body with the references refs,
// as the copy holds it: each reference replaced by the value passed, or
// by nothing. removed is set for an attribute (attr) whose whole value is
// one reference to a prop the use does not pass: it is not in the copy
// (decision 163). lit is the writer of s's own text (nil for an
// element's value, the use for one of its prop attributes).
func (fr *propFrame) subst(s string, refs []pref, lit *pwriter, attr bool) (out pval, removed bool) {
	if len(refs) == 1 && refs[0].start == 0 && refs[0].end == len(s) {
		v := fr.val(refs[0])
		if v == nil {
			return pval{}, attr
		}
		return *v, false
	}
	var sb strings.Builder
	text := func(t string) {
		if t != "" {
			sb.WriteString(t)
			out.add(sb.Len(), lit)
		}
	}
	at := 0
	for _, r := range refs {
		text(s[at:r.start])
		if v := fr.val(r); v != nil && v.s != "" {
			off := sb.Len()
			sb.WriteString(v.s)
			for _, run := range v.runs {
				out.add(off+run.end, run.w)
			}
		}
		at = r.end
	}
	text(s[at:])
	out.s = sb.String()
	return out, false
}

// pstate is what one value that holds a reference becomes.
type pstate uint8

const (
	psAsIs    pstate = iota // as written: no reference, or checked no further
	psMuted                 // as written, its diagnostics muted (outside a template)
	psHidden                // a template's body as written: not checked (decision 162)
	psIgnored               // where a prop cannot stand, or inside braces: ignored (decision 157)
	psRemoved               // a whole value whose prop is not passed: not in the copy
	psSubst                 // substituted in a copy
	// psPlace: an attribute of a copy that its element does not take
	// where the copy lands, though the template's definition took it: its
	// V002 carries the copy's suffix (erratum E1).
	psPlace
)

// pattr is what a value that holds a reference becomes.
type pattr struct {
	st pstate
	v  string
	// by is the use a diagnostic at a substituted value names (decision
	// 194); w the outermost writer of its text, nil when none; held is
	// set for a value of a copy that held a reference.
	by   *ir.Node
	w    *pwriter
	held bool
}

// propAt is a place a diagnostic is redirected or muted at (builder.at,
// builder.mute).
type propAt struct {
	on        bool
	file      string
	line, col int
	by        *ir.Node
}

// propNode is what an element's references become: one entry per
// attribute as written, and its body's.
type propNode struct {
	attrs   []pattr
	body    pattr
	hasBody bool
	def     bool // built in a template's body as written
}

// propsState is the composer's state for props.
type propsState struct {
	// defDecl is the template whose body is being built as written, and
	// own the template's own prop diagnostics that body gave: reported
	// whether or not a use copies the template (template).
	defDecl *propDecl
	own     ir.Diags
	// pframes are the props of the copies being built, innermost last.
	pframes []*propFrame
	// ignored holds the attributes of an element of a copy that were
	// ignored where a prop cannot stand: they count as written for the
	// checks that require them (wrote, decision 157).
	ignored map[*ir.Node][]string
	// noLoad holds a <style> or an <include> whose src holds a {$name}
	// outside a template: it loads nothing (decision 184).
	noLoad map[*ir.Node]bool
	// muted holds the attributes of an element outside a template whose
	// {$name} is that value's only diagnostic: a check of the element that
	// reads one reports nothing about it (mutedAttr).
	muted map[*ir.Node][]string
	// took holds the attributes holding a `{$` that an element of a
	// template's definition takes where it is written (erratum E1).
	took map[rawAttrRef]bool
	// wuses are the <use> elements as written, in a template's body or
	// outside every template (checkWrittenUses); firstAt the node of each
	// one's first placement in a copy, firstLog the order they were
	// recorded in (a discarded expansion takes them back).
	wuses    []writtenUse
	firstAt  map[*RawNode]*ir.Node
	firstLog []*RawNode
	// fragDecls are the templates a fragment checked on its own writes,
	// the first of each id (FragmentDiags).
	fragDecls map[string]*propDecl
}

// rawAttrRef is the attribute i of a raw element.
type rawAttrRef struct {
	r *RawNode
	i int
}

// writtenUse is a <use> as written.
type writtenUse struct {
	raw  *RawNode
	q    *ir.Node  // its node where it is written
	encl *propDecl // the template whose body holds it, nil outside
	// unknown is set outside the templates of a fragment checked on its
	// own (pmUnknown).
	unknown bool
}

// wrote reports whether n writes the attribute name, an attribute ignored
// where a prop cannot stand included: it counts as written for the
// checks that require it on its own element (decision 157).
func (b *builder) wrote(n *ir.Node, name string) bool {
	if _, ok := n.Attrs[name]; ok {
		return true
	}
	return b.comp != nil && slices.Contains(b.comp.ignored[n], name)
}

// noID reports whether n has no id for V012: none written, nor one
// ignored where a prop cannot stand (decision 157).
func (b *builder) noID(n *ir.Node) bool {
	return n.ID == "" && !(b.comp != nil && slices.Contains(b.comp.ignored[n], "id"))
}

// ignoredAttr reports whether n's attribute name was ignored where a prop
// cannot stand (decision 157).
func (b *builder) ignoredAttr(n *ir.Node, name string) bool {
	return b.comp != nil && slices.Contains(b.comp.ignored[n], name)
}

// mutedAttr reports whether n's attribute name held a {$name} outside a
// template: its V003 is reported instead of any other diagnostic about
// that value, so a check that reads it reports nothing (§6.18.9).
func (b *builder) mutedAttr(n *ir.Node, name string) bool {
	return b.comp != nil && slices.Contains(b.comp.muted[n], name)
}

// substBy is the use a diagnostic names when a check reported at another
// attribute of the element whose attributes are being checked reads the
// attribute name, substituted in a copy (erratum E4), nil otherwise.
func (b *builder) substBy(name string) *ir.Node {
	pn, r := b.pnode, b.praw
	if pn == nil || r == nil {
		return nil
	}
	for i, a := range r.Attrs {
		if a.Name == name && i < len(pn.attrs) && pn.attrs[i].st == psSubst {
			return pn.attrs[i].by
		}
	}
	return nil
}

// orgFile is the name of the file being built.
func (b *builder) orgFile() string {
	return b.org.FileOr(b.file)
}

// ownDiag reports a template's own prop check (§6.18.9): at the value, in
// the file being built, with no suffix. Inside a template's body as
// written it is kept apart (cp.own), so that it is reported whether or
// not a use copies the template.
func (b *builder) ownDiag(line, col int, path, id, format string, args ...any) {
	cp := b.comp
	n := len(b.doc.Diags)
	b.diag(ir.Error, "V003", line, col, path, id, format, args...)
	if cp.defDecl != nil && len(b.doc.Diags) > n {
		cp.own = append(cp.own, b.doc.Diags[n:]...)
		b.doc.Diags = b.doc.Diags[:n]
	}
}

// propValue decides what the value s, written in the attribute name of
// the element tag at path ("" for its body), becomes where the element
// stands (mode), and reports what §6.18.9 says of it there.
func (b *builder) propValue(tag, path, id, name, s string, line, col int, mode propMode) pattr {
	if !strings.Contains(s, "{$") {
		return pattr{}
	}
	refs := scanRefs(s)
	if len(refs) == 0 {
		return pattr{}
	}
	cp := b.comp
	switch mode {
	case pmOutside:
		var seen []string
		for _, r := range refs {
			if !r.ok || slices.Contains(seen, r.name) {
				continue
			}
			seen = append(seen, r.name)
			// It depends on where the element stands, so on an element an
			// include placed it names the include (§6.18.6, erratum E2).
			b.diagPlaced(ir.Error, "V003", line, col, path, id, msgPropOutside, r.name)
		}
		if seen == nil {
			return pattr{} // no {$name}: §7 reads it, as before
		}
		return pattr{st: psMuted}
	case pmDef, pmUnknown:
		// A template's own checks; in a fragment checked on its own,
		// outside its templates, those that do not depend on a template
		// (§15.1), the value otherwise muted.
		d := cp.defDecl
		if mode == pmUnknown {
			d = nil
		}
		if pl := placeOf(name); pl != plText {
			// Where a prop cannot stand: one diagnostic, the attribute
			// ignored (decision 157).
			switch r := refs[0]; {
			case !r.ok:
				b.ownDiag(line, col, path, id, msgPropMalformed, s[r.start:r.end])
			case pl == plPath:
				b.ownDiag(line, col, path, id, msgPropPath, r.name, name)
			default:
				b.ownDiag(line, col, path, id, msgPropOther, r.name, name)
			}
			return pattr{st: psIgnored}
		}
		t7 := isTemplate7(tag, name)
		in := insideBraces(s, refs)
		for k, r := range refs {
			switch {
			case !r.ok:
				b.ownDiag(line, col, path, id, msgPropMalformed, s[r.start:r.end])
			case in != nil && in[k]:
				if t7 {
					b.ownDiag(line, col, path, id, msgPropInsidePath, r.name)
				} else {
					b.ownDiag(line, col, path, id, msgPropInsideText, r.name)
				}
			case d != nil && !d.names[r.name]:
				b.ownDiag(line, col, path, id, msgPropUndeclared, r.name, d.id)
			}
		}
		if in != nil {
			return pattr{st: psIgnored}
		}
		if mode == pmUnknown {
			return pattr{st: psMuted}
		}
		if d != nil {
			d.sites = append(d.sites, propSite{s: s, refs: refs, t7: t7})
		}
		return pattr{st: psHidden}
	case pmCopy:
		fr := cp.pframes[len(cp.pframes)-1]
		if placeOf(name) != plText || insideBraces(s, refs) != nil {
			// Reported by the template's definition: ignored here.
			return pattr{st: psIgnored, held: true}
		}
		v, removed := fr.subst(s, refs, nil, name != "")
		if removed {
			return pattr{st: psRemoved, held: true}
		}
		if g := len(v.s) - len(s); g > 0 {
			b.chargeBytes(g) // decision 177
		}
		p := pattr{st: psSubst, v: v.s, by: fr.q, w: v.outer(), held: true}
		if p.w != nil {
			p.by = p.w.use
		}
		return p
	}
	return pattr{}
}

// propNode decides what the references of the element n, built from r,
// become, and reports what §6.18.9 says of them. It is nil when none of
// its attributes, nor its body, holds a reference.
func (b *builder) propNode(n *ir.Node, r *RawNode) *propNode {
	cp := b.comp
	mode := b.propMode()
	if mode == pmDef && r.Name == "slot" && cp.defDecl != nil {
		cp.defDecl.slot = true
	}
	var pn *propNode
	id := ""
	start := func() {
		pn = &propNode{def: mode == pmDef}
		if v, ok := rawAttr(r, "id"); ok && !strings.Contains(v, "{$") {
			id = cp.prefix + v // the id a diagnostic names
		}
	}
	for i, a := range r.Attrs {
		if !strings.Contains(a.Value, "{$") {
			continue
		}
		if !b.attrGate(n, a, false) {
			// Checked no further (decision 157). Where the attributes an
			// element takes depend on its place, the definition judges
			// them as written, and a copy that lands where the element does
			// not take one reports its V002 with the copy's suffix (E1).
			if mode == pmCopy && cp.took[rawAttrRef{r, i}] && n.Org != nil {
				if pn == nil {
					start()
				}
				if pn.attrs == nil {
					pn.attrs = make([]pattr, len(r.Attrs))
				}
				pn.attrs[i] = pattr{st: psPlace, by: n.Org.Placed}
				// A whole value that is one reference to a prop the use does
				// not pass, or that names no prop, is not in the copy
				// (decision 163), so its place rejects nothing there.
				if refs := scanRefs(a.Value); len(refs) == 1 && refs[0].start == 0 && refs[0].end == len(a.Value) && cp.pframes[len(cp.pframes)-1].val(refs[0]) == nil {
					pn.attrs[i] = pattr{st: psRemoved, held: true}
				}
			}
			continue
		}
		if mode == pmDef {
			if cp.took == nil {
				cp.took = map[rawAttrRef]bool{}
			}
			cp.took[rawAttrRef{r, i}] = true
		}
		if pn == nil {
			start()
		}
		if pn.attrs == nil {
			pn.attrs = make([]pattr, len(r.Attrs))
		}
		pn.attrs[i] = b.propValue(n.Tag, n.Path, id, a.Name, a.Value, a.Line, a.Col, mode)
		switch p := pn.attrs[i]; {
		case p.st == psMuted:
			if cp.muted == nil {
				cp.muted = map[*ir.Node][]string{}
			}
			cp.muted[n] = append(cp.muted[n], a.Name)
			if a.Name == "src" && (n.Tag == "style" || n.Tag == "include") {
				if cp.noLoad == nil {
					cp.noLoad = map[*ir.Node]bool{}
				}
				cp.noLoad[n] = true
			}
		case p.st == psIgnored:
			if cp.ignored == nil {
				cp.ignored = map[*ir.Node][]string{}
			}
			cp.ignored[n] = append(cp.ignored[n], a.Name)
		}
	}
	if r.Name == "text" || r.Name == "button" || r.Name == "column" {
		if body, line, col, ok := propBody(r); ok {
			if pn == nil {
				start()
			}
			pn.body = b.propValue(n.Tag, n.Path, id, "", body, line, col, mode)
			pn.hasBody = pn.body.st != psAsIs
		}
	}
	if pn == nil || mode != pmCopy {
		return pn
	}
	// The use a diagnostic at the element names (decision 194).
	var best *pwriter
	held := false
	for i := 0; i <= len(pn.attrs); i++ {
		p := &pn.body
		if i < len(pn.attrs) {
			p = &pn.attrs[i]
		}
		held = held || p.held
		if p.w != nil && (best == nil || p.w.lvl < best.lvl) {
			best = p.w
		}
	}
	if held {
		n.PropBy = cp.pframes[len(cp.pframes)-1].q
		if best != nil {
			n.PropBy = best.use
		}
	}
	return pn
}

// propBody is the body of a text, a button, or a column as content reads
// it, when it holds a `{$`: its character data joined, and where the
// first of it starts. A body of one run of character data, the common
// case, is read in place.
func propBody(r *RawNode) (body string, line, col int, ok bool) {
	var first *RawNode
	runs := 0
	for _, c := range r.Children {
		if c.Type != RawText && c.Type != RawCData {
			continue
		}
		if first == nil {
			first = c
		}
		runs++
	}
	if first == nil {
		return "", 0, 0, false
	}
	if runs == 1 {
		body = first.Text
	} else {
		var sb strings.Builder
		for _, c := range r.Children {
			if c.Type == RawText || c.Type == RawCData {
				sb.WriteString(c.Text)
			}
		}
		body = sb.String()
	}
	return body, first.Line, first.Col, strings.Contains(body, "{$")
}

// rawAttr is the value of r's attribute name as written.
func rawAttr(r *RawNode, name string) (string, bool) {
	for _, a := range r.Attrs {
		if a.Name == name {
			return a.Value, true
		}
	}
	return "", false
}

// inAttrs puts the attribute i of the element n, a, in n.Attrs as the
// element holds it while it is built, and reports whether a goes there as
// written instead: an attribute ignored, removed, or hidden is not there,
// and a substituted one holds its value.
func (pn *propNode) inAttrs(n *ir.Node, i int, a RawAttr) bool {
	if i >= len(pn.attrs) {
		return true
	}
	switch p := pn.attrs[i]; p.st {
	case psRemoved, psIgnored, psHidden:
		return false
	case psSubst:
		n.Attrs[a.Name] = p.v
		n.Order = append(n.Order, a.Name)
		return false
	}
	return true
}

// check is the attribute i as attr checks it, and whether it does: a
// substituted value is checked as written by hand, its diagnostics naming
// the use that wrote it; one outside a template is checked with its
// diagnostics muted.
func (pn *propNode) check(b *builder, i int, a RawAttr) (RawAttr, bool) {
	if i >= len(pn.attrs) {
		return a, true
	}
	switch p := pn.attrs[i]; p.st {
	case psRemoved, psIgnored, psHidden:
		return a, false
	case psSubst:
		a.Value = p.v
		b.at = propAt{on: true, file: b.orgFile(), line: a.Line, col: a.Col, by: p.by}
	case psMuted:
		b.mute = propAt{on: true, file: b.orgFile(), line: a.Line, col: a.Col}
	case psPlace:
		b.at = propAt{on: true, file: b.orgFile(), line: a.Line, col: a.Col, by: p.by}
	}
	return a, true
}

// body is the body content checks, and whether it checks it.
func (p *pattr) body(b *builder, n *ir.Node, body string, line, col int) (string, bool) {
	switch p.st {
	case psHidden, psIgnored:
		if b.comp.selfCheck {
			n.Text = NormalizeText(body) // as written, for `tuimark ir`
		}
		return body, false
	case psSubst:
		b.at = propAt{on: true, file: b.orgFile(), line: line, col: col, by: p.by}
		return p.v, true
	case psMuted:
		b.mute = propAt{on: true, file: b.orgFile(), line: line, col: col}
	}
	return body, true
}

// restore gives an element of a template's body as written the
// attributes no check read (decisions 157 and 162), as written, for
// `tuimark ir` (§13.1: a template node keeps its body as written).
func (pn *propNode) restore(b *builder, n *ir.Node, r *RawNode) {
	if !pn.def {
		return
	}
	for i, p := range pn.attrs {
		if p.st != psHidden && p.st != psIgnored {
			continue
		}
		a := r.Attrs[i]
		n.Attrs[a.Name] = a.Value
		n.Order = append(n.Order, a.Name)
		switch {
		case a.Name == "bind":
			n.Bind = a.Value
		case a.Name == "runs":
			n.Runs = a.Value
		case a.Name == "each":
			n.Each = a.Value
		case a.Name == "if":
			n.If = a.Value
		case a.Name == "class":
			n.Classes = append(n.Classes, strings.Fields(a.Value)...)
		case strings.HasPrefix(a.Name, "on:"):
			n.On[strings.TrimPrefix(a.Name, "on:")] = a.Value
		}
	}
}

// templateProps checks the <template> element t's own attributes, built
// from c, and returns what it declares (§6.18.9). Its id and props are
// not in its body: a {$name} in them is outside a template (decision
// 190), and a props that holds one declares nothing.
func (b *builder) templateProps(t *ir.Node, c *RawNode) *propDecl {
	d := &propDecl{id: t.ID}
	for _, a := range c.Attrs {
		muted := false
		if strings.Contains(a.Value, "{$") && b.attrGate(t, a, false) {
			muted = b.propValue(t.Tag, t.Path, t.ID, a.Name, a.Value, a.Line, a.Col, pmOutside).st == psMuted
		}
		if muted {
			b.mute = propAt{on: true, file: b.orgFile(), line: a.Line, col: a.Col}
		}
		b.attr(t, a)
		b.mute.on = false
		if a.Name == "props" && !muted && b.attrGate(t, a, false) {
			b.declare(t, a, d)
		}
	}
	return d
}

// declare reads a template's props (§6.18.9, decision 153): the names
// separated by one space, each a prop name, written once, and not one a
// <use> takes for itself. The separator's error comes first, then those
// of the names, in their order; every other name is declared.
func (b *builder) declare(t *ir.Node, a RawAttr, d *propDecl) {
	v := a.Value
	err := func(format string, args ...any) {
		b.diag(ir.Error, "V003", a.Line, a.Col, t.Path, t.ID, format, args...)
	}
	if v != "" && (strings.HasPrefix(v, propSeparatorSpace) || strings.HasSuffix(v, propSeparatorSpace) ||
		strings.Contains(v, "  ") || strings.ContainsAny(v, "\t\n\r")) {
		err(msgPropSeparator, v)
	}
	names := strings.FieldsFunc(v, func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' || r == '\r' })
	twice := map[string]bool{}
	for _, name := range names {
		switch {
		case !ir.IsGuardName(name):
			err(msgPropInvalid, name)
		case useOwnAttrs[name]:
			err(msgPropUseOwn, name, name)
		case d.names[name]:
			if !twice[name] {
				twice[name] = true
				err(msgPropTwice, name)
			}
		default:
			if d.names == nil {
				d.names = map[string]bool{}
			}
			d.names[name] = true
		}
	}
}

// useProps decides what the attributes of the <use> q, built from c,
// become (§6.18.3, §6.18.9): its own attributes, where a {$name} is
// checked as in any element (and, in template, with, id, if, or hidden,
// makes the use insert nothing), and its prop-shaped ones, which a copy
// passes on to its template when the template declares them (info.props).
// It returns nil when no attribute holds a reference.
func (b *builder) useProps(q *ir.Node, c *RawNode, info *useInfo) *propNode {
	cp := b.comp
	mode := b.propMode()
	var pn *propNode
	get := func() *propNode {
		if pn == nil {
			pn = &propNode{attrs: make([]pattr, len(c.Attrs)), def: mode == pmDef}
		}
		return pn
	}
	var t *propDecl
	var fr *propFrame
	if mode == pmCopy {
		fr = cp.pframes[len(cp.pframes)-1]
		if name, ok := rawAttr(c, "template"); ok && !strings.Contains(name, "{$") {
			if tt := cp.tmpls[name]; tt != nil {
				t = tt.decl
			}
		}
	}
	id := ""
	if v, ok := rawAttr(c, "id"); ok && !strings.Contains(v, "{$") {
		id = cp.prefix + v
	}
	for i, a := range c.Attrs {
		if useOwnAttrs[a.Name] {
			p := b.propValue("use", q.Path, id, a.Name, a.Value, a.Line, a.Col, mode)
			if p.st == psAsIs {
				continue
			}
			if a.Name != "class" {
				info.bad = true // inserts nothing (decision 157)
			}
			get().attrs[i] = p
			continue
		}
		if !propShaped(a.Name) {
			continue
		}
		w := &pwriter{use: q, line: a.Line, col: a.Col, name: a.Name, lvl: info.lvl}
		switch {
		case mode == pmOutside:
			// Passed as written, unless it holds a {$name} (decision 188).
			v := &pval{s: a.Value}
			v.add(len(a.Value), w)
			if a.Value == "" {
				v.runs = nil
			}
			info.props = append(info.props, useProp{name: a.Name, v: v, no: hasPropRef(a.Value)})
		case mode == pmCopy:
			// Substituted, as the copy holds it (§13.1), and passed when
			// the template declares it.
			decl := t != nil && t.names[a.Name]
			refs := scanRefs(a.Value)
			if len(refs) == 0 {
				if decl {
					v := &pval{s: a.Value}
					if a.Value != "" {
						v.add(len(a.Value), w)
					}
					info.props = append(info.props, useProp{name: a.Name, v: v})
				}
				continue
			}
			if decl && insideBraces(a.Value, refs) != nil {
				continue // reported as written (checkWrittenUses): not passed
			}
			v, removed := fr.subst(a.Value, refs, w, true)
			if removed {
				get().attrs[i] = pattr{st: psRemoved} // decision 164
				continue
			}
			if g := len(v.s) - len(a.Value); g > 0 {
				b.chargeBytes(g) // decision 177
			}
			get().attrs[i] = pattr{st: psSubst, v: v.s}
			if decl {
				info.props = append(info.props, useProp{name: a.Name, v: &v})
			}
		}
	}
	return pn
}

// touched reports whether the attribute name of the use c holds a
// reference that this use does not read as written.
func (pn *propNode) touched(c *RawNode, name string) bool {
	if pn == nil {
		return false
	}
	for i, a := range c.Attrs {
		if a.Name == name {
			return pn.attrs[i].st != psAsIs
		}
	}
	return false
}

// useProp is a prop-shaped attribute of a <use>: its value, passed on
// when its template declares it, unless no is set.
type useProp struct {
	name string
	v    *pval
	no   bool
}

// recordUse records the <use> c, built as q, for checkWrittenUses: as
// written in a template's body or outside every template, and, in a copy,
// as the first placement of the written use.
func (b *builder) recordUse(c *RawNode, q *ir.Node) {
	cp := b.comp
	switch mode := b.propMode(); mode {
	case pmDef:
		cp.wuses = append(cp.wuses, writtenUse{raw: c, q: q, encl: cp.defDecl})
	case pmOutside, pmUnknown:
		cp.wuses = append(cp.wuses, writtenUse{raw: c, q: q, unknown: mode == pmUnknown})
	case pmCopy:
		if cp.firstAt == nil {
			cp.firstAt = map[*RawNode]*ir.Node{}
		}
		if cp.firstAt[c] == nil {
			cp.firstAt[c] = q
			cp.firstLog = append(cp.firstLog, c)
		}
	}
}

// forgetUses takes back what recordUse recorded after the first nUses
// written uses and nFirst placements: those of an expansion that is
// discarded.
func (cp *composer) forgetUses(nUses, nFirst int) {
	cp.wuses = cp.wuses[:nUses]
	for _, c := range cp.firstLog[nFirst:] {
		delete(cp.firstAt, c)
	}
	cp.firstLog = cp.firstLog[:nFirst]
}

// pushProps makes the props the use q passes to its template t those of
// the copy about to be built, after the braces that their values would
// pair are checked (decision 160). popProps takes them back.
func (b *builder) pushProps(q *ir.Node, t *tmpl) {
	cp := b.comp
	fr := &propFrame{decl: t.decl, q: q}
	if info := cp.useOf[q]; info != nil && t.decl != nil {
		for _, p := range info.props {
			if p.no || !t.decl.names[p.name] {
				continue
			}
			if fr.vals == nil {
				fr.vals = map[string]*pval{}
			}
			fr.vals[p.name] = p.v
		}
	}
	if fr.decl == nil {
		fr.decl = &propDecl{}
	}
	b.checkBraces(fr)
	cp.pframes = append(cp.pframes, fr)
}

// pseg is a run of a site's value as a copy holds it: the template's own
// text (prop "") or the value of a prop.
type pseg struct {
	start, end int
	prop       string
	v          *pval
}

// render is the site's value as the copy fr holds it, with its runs.
func (fr *propFrame) render(site *propSite) (string, []pseg) {
	var sb strings.Builder
	var segs []pseg
	at := 0
	for _, r := range site.refs {
		sb.WriteString(site.s[at:r.start])
		if v := fr.val(r); v != nil && v.s != "" {
			off := sb.Len()
			sb.WriteString(v.s)
			segs = append(segs, pseg{off, sb.Len(), r.name, v})
		}
		at = r.end
	}
	sb.WriteString(site.s[at:])
	return sb.String(), segs
}

// checkBraces applies decision 160 to the copy fr is about to build: in
// each value of the template's body where its text lands, a pair of
// braces one of which a prop's value brings is V003, once for each prop
// that brings a brace to such a pair, at the <use> attribute where the
// brace is written; every prop that brings a brace into such a value is
// then not passed, in the whole copy.
func (b *builder) checkBraces(fr *propFrame) {
	braced := false
	for _, v := range fr.vals {
		if strings.ContainsAny(v.s, "{}") {
			braced = true
			break
		}
	}
	if !braced {
		return
	}
	// Each prop is reported once, with the first pair it brings a brace
	// to, at each attribute that writes one of its braces (decision 160),
	// over every value of the body.
	drop := map[string]bool{}
	first := map[string]string{}
	done := map[*pwriter]bool{}
	for i := range fr.decl.sites {
		site := &fr.decl.sites[i]
		s, segs := fr.render(site)
		if len(segs) == 0 {
			continue
		}
		type hit struct {
			prop string
			pair [2]int
			w    *pwriter
		}
		var hits []hit
		for _, p := range bracePairs(s) {
			for _, at := range p {
				for _, sg := range segs {
					if at >= sg.start && at < sg.end {
						hits = append(hits, hit{sg.prop, p, sg.v.writerAt(at - sg.start)})
					}
				}
			}
		}
		if len(hits) == 0 {
			continue
		}
		for _, sg := range segs {
			if strings.ContainsAny(s[sg.start:sg.end], "{}") {
				drop[sg.prop] = true
			}
		}
		for _, h := range hits {
			if _, ok := first[h.prop]; !ok {
				first[h.prop] = s[h.pair[0] : h.pair[1]+1]
			}
		}
		for _, h := range hits {
			if h.w == nil || done[h.w] {
				continue
			}
			done[h.w] = true
			b.pairDiag(h.w, first[h.prop], site.t7)
		}
	}
	for p := range drop {
		delete(fr.vals, p)
	}
}

// pairDiag reports the pair a prop's value makes (decision 160) at the
// <use> attribute w, where the brace is written, with that use's own
// placement suffix. The pair is cut as §6.18.6 cuts a value.
func (b *builder) pairDiag(w *pwriter, pair string, t7 bool) {
	format := msgPropPairText
	if t7 {
		format = msgPropPairPath
	}
	q := w.use
	if q.InCopy() || len(pair) <= ir.MaxShownID {
		b.diagOrg(q.Org, true, ir.Error, "V003", w.line, w.col, q.Path, q.ID, format, w.name, pair)
		return
	}
	n := len(b.doc.Diags)
	b.diagOrg(q.Org, true, ir.Error, "V003", w.line, w.col, q.Path, q.ID, format, w.name, ir.ShownPath(pair))
	if len(b.doc.Diags) > n {
		b.doc.Diags[n] = ir.WithWhole(b.doc.Diags[n], pair)
	}
}

// checkWrittenUses makes the checks of each <use> as written against its
// template (§6.18.3, decision 187), once per written use, with no
// suffix, whether or not it expands: the V002 of a prop-shaped attribute
// that the template does not declare, the checks of the value of one it
// declares when the value holds a reference (the template's own checks in
// a template's body, the V003 of a {$name} outside a template elsewhere),
// and the V013 of content toward a template with no slot. lookup finds a
// template by id; a use whose template is not known there gets none of
// them. Their path is the use's at its first placement, else where it is
// written.
func (b *builder) checkWrittenUses(lookup func(string) *propDecl) {
	cp := b.comp
	for _, wu := range cp.wuses {
		c := wu.raw
		name, ok := rawAttr(c, "template")
		if !ok || name == "" || strings.Contains(name, "{$") {
			continue
		}
		t := lookup(name)
		if t == nil {
			continue
		}
		at := wu.q
		if f := cp.firstAt[c]; f != nil {
			at = f
		}
		org := wu.q.Org
		diag := func(code string, line, col int, format string, args ...any) {
			b.diagOrg(org, false, ir.Error, code, line, col, at.Path, at.ID, format, args...)
		}
		for _, a := range c.Attrs {
			if !propShaped(a.Name) {
				continue
			}
			if !t.names[a.Name] {
				diag("V002", a.Line, a.Col, msgUsePropAttr, a.Name)
				continue
			}
			refs := scanRefs(a.Value)
			if len(refs) == 0 {
				continue
			}
			switch {
			case wu.encl != nil || wu.unknown:
				// In a fragment checked on its own, outside its templates,
				// only the checks that do not depend on a template (§15.1).
				in := insideBraces(a.Value, refs)
				for k, r := range refs {
					switch {
					case !r.ok:
						diag("V003", a.Line, a.Col, msgPropMalformed, a.Value[r.start:r.end])
					case in != nil && in[k]:
						diag("V003", a.Line, a.Col, msgPropInsideText, r.name)
					case wu.encl != nil && !wu.encl.names[r.name]:
						diag("V003", a.Line, a.Col, msgPropUndeclared, r.name, wu.encl.id)
					}
				}
			default:
				var seen []string
				for _, r := range refs {
					if r.ok && !slices.Contains(seen, r.name) {
						seen = append(seen, r.name)
						// With the suffix of the include that placed the
						// use, if any (erratum E2).
						b.diagOrg(org, true, ir.Error, "V003", a.Line, a.Col, at.Path, at.ID, msgPropOutside, r.name)
					}
				}
			}
		}
		b.checkUseContent(c, t, diag) // its content (v0.4c, slot.go)
	}
}
