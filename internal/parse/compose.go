package parse

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// Loader reads the files an <include> names (SPEC v0.4b §6.18.2). The
// parser cannot touch a file system itself: the host passes the one its
// document came from, an OS directory under Load, an fs.FS under LoadFS,
// the editor's buffers under LoadBuffers. Paths are the loader's own (an OS
// path, or a slash-separated path inside an fs.FS); a file's path is "" for
// the document itself.
type Loader struct {
	// Join resolves src, as written in the file at path from ("" is the
	// document), against that file's directory. outside reports a src the
	// loader refuses (an absolute path, or one that climbs above the root,
	// under an fs.FS); resolved is then only fit for the message.
	Join func(from, src string) (resolved string, outside bool)
	// Read returns the file at a resolved path.
	Read func(resolved string) ([]byte, error)
	// Canon is a resolved path's identity: two includes of the same file
	// have the same Canon, whatever their spelling.
	Canon func(resolved string) string
	// Name is the name diagnostics give a file at a resolved path: its
	// path relative to the document's directory, with "/" separators
	// (SPEC v0.4b §6.18.6).
	Name func(resolved string) string
	// Self is the document's own Canon ("" when it has no path, as under
	// Parse): an include of the document itself is a cycle.
	Self string
	// SelfPath is the document's own resolved path in the loader's file
	// system ("" when it has none), for Same.
	SelfPath string
	// Same, when not nil, reports whether the files at two resolved paths
	// are the same file on disk (os.SameFile), however they are spelled:
	// a symlinked directory gives a fragment a second path to itself (ADR
	// 0034). nil, or false, leaves the identity to Canon alone, as for an
	// fs.FS that has no file identity.
	Same func(a, b string) bool
}

// ExpansionLimit is the most elements the expansions of one document may
// insert (ADR 0034): each element an <include> (later also a <use>)
// inserts, at every depth, once per copy, and one for each include (use)
// expanded, a written one at the top of the document included. Text does
// not count. Counting the includes too is what bounds a fan-out of
// fragments that hold no element.
const ExpansionLimit = 100000

// ExpansionByteLimit is the most source an expansion may place in one
// document, in bytes (ADR 0034, second amendment): each expansion is
// charged, before it builds anything, the size of the source it places (an
// include, its fragment's file; a use, its template's span). The work of an
// expansion is linear in the bytes it places, so this bounds it for any
// content, which the count of elements does not.
const ExpansionByteLimit = 16 << 20

// expFrame is an expansion under way: the innermost one reports V021.
type expFrame struct{}

// expansionOverflow is the panic that unwinds an expansion to the frame
// that inserted one element too many.
type expansionOverflow struct{ frame *expFrame }

// composer is the state of composition while one document is built.
type composer struct {
	ld *Loader
	// chain holds the files being expanded, the document first: the
	// include cycle check walks it (SPEC v0.4b §6.18.2 check 2).
	chain []chainFile
	// selfOnly is set when a fragment is checked on its own (FragmentDiags):
	// an <include> gets only its own checks, and no file is read.
	selfOnly bool
	// used is whether an <include> was expanded (some diagnostics are then
	// reported once per written element).
	used bool
	// docOrg is the origin of an <include> written in the document: its
	// file, placed by nothing.
	docOrg *ir.Origin
	// frames are the expansions under way, outermost first; count is the
	// number of elements they have inserted (ExpansionLimit); dead is set
	// once one went over: no later include expands (ADR 0034).
	frames []*expFrame
	count  int
	bytes  int
	dead   bool
	// files caches each fragment read and parsed, by Canon: an expansion
	// may place a file many times, and reading and parsing it again each
	// time would make the work grow with the copies, not the files.
	files map[string]*fragFile
	// templating is the state of <template> and <use> (expand.go).
	templating
}

// fragFile is a fragment read once: its parsed root, or why it has none.
type fragFile struct {
	root    *RawNode
	readErr error
	syntax  *SyntaxError
	size    int // the file's size in bytes
}

// chainFile is one file on the chain of includes that leads to the
// <include> being expanded.
type chainFile struct {
	path, canon, name string
	// real is the file's resolved path in the loader's file system, for
	// Same ("" for a document that has none).
	real string
}

// chargeBytes charges the expansion under way n bytes of placed source.
// Going over ExpansionByteLimit unwinds to the innermost expansion, like
// passing ExpansionLimit does. F2's <use> charges its template's source
// span through it.
func (b *builder) chargeBytes(n int) {
	cp := b.comp
	if cp == nil || cp.dead || len(cp.frames) == 0 {
		return
	}
	cp.bytes += n
	if cp.bytes > ExpansionByteLimit {
		panic(&expansionOverflow{frame: cp.frames[len(cp.frames)-1]})
	}
}

// countElement counts an element built inside an expansion. The one that
// would pass ExpansionLimit unwinds to the innermost expansion, which
// inserts nothing and reports V021 (ADR 0034).
func (b *builder) countElement() {
	cp := b.comp
	if cp == nil || cp.dead || len(cp.frames) == 0 {
		return
	}
	cp.count++
	if cp.count > ExpansionLimit {
		panic(&expansionOverflow{frame: cp.frames[len(cp.frames)-1]})
	}
}

// docName is how the document is named in a message that names files.
func (b *builder) docName() string {
	if b.file == "" {
		return "<input>"
	}
	return b.file
}

// wrapperOrg is the origin of an <include> element written where the
// builder is: the file it is in, placed by what placed that file's elements.
func (b *builder) wrapperOrg() *ir.Origin {
	if b.org != nil {
		return b.org
	}
	if b.comp.docOrg == nil {
		b.comp.docOrg = &ir.Origin{File: b.file}
	}
	return b.comp.docOrg
}

// errInc reports an error about an <include> element itself: at the
// include, in its file, with no placement suffix (SPEC v0.4b §6.18.6).
func (b *builder) errInc(w *ir.Node, code, format string, args ...any) {
	b.diagOrg(w.Org, false, ir.Error, code, w.Line, w.Col, w.Path, w.ID, format, args...)
}

// attach adds an element built from n's content to n's tree. A node of a
// known kind joins n.Children (and the as-written list of the include it
// came from, outer, or of n); an unknown one is validated and kept out
// (SPEC §1).
func (b *builder) attach(n, outer, child *ir.Node) {
	if !b.tagKnown(child.Tag) {
		b.doc.dropped = append(b.doc.dropped, child)
		if b.droppedIn == nil {
			b.droppedIn = map[*ir.Node][]droppedAt{}
		}
		b.droppedIn[n] = append(b.droppedIn[n], droppedAt{at: len(n.Children), node: child})
		return
	}
	n.Children = append(n.Children, child)
	switch {
	case outer != nil:
		outer.Children = append(outer.Children, child)
	case n.Source != nil:
		n.Source = append(n.Source, child)
	}
}

// written appends w to the as-written children of n, or of the include
// that holds it.
func writeInto(n, outer, w *ir.Node) {
	if outer != nil {
		outer.Children = append(outer.Children, w)
		return
	}
	if n.Source == nil {
		n.Source = append([]*ir.Node{}, n.Children...)
	}
	n.Source = append(n.Source, w)
}

// include expands the <include> element c, a child of n (SPEC v0.4b
// §6.18.2): the elements of its fragment are built in n, as if written
// there, and an include node that holds them is kept for `tuimark ir`. outer
// is the include whose fragment holds c, nil when c is written in n's own
// file at n's own level.
func (b *builder) include(n *ir.Node, c *RawNode, outer *ir.Node) {
	cp := b.comp
	cp.used = true
	w := &ir.Node{
		Tag: "include", Kind: "include", Parent: n,
		Attrs: map[string]string{}, On: map[string]string{},
		Line: c.Line, Col: c.Col, Path: childPath(n, "include"),
		Org: b.wrapperOrg(),
	}
	for _, a := range c.Attrs {
		w.Attrs[a.Name] = a.Value
		w.Order = append(w.Order, a.Name)
	}
	writeInto(n, outer, w)
	for _, a := range c.Attrs {
		b.attr(w, a)
	}
	// An include takes no content: Tuimark has no slots.
	for _, cc := range c.Children {
		bad := cc.Type == RawElement || ((cc.Type == RawText || cc.Type == RawCData) && !isXMLWhitespaceOnly(cc.Text))
		if bad {
			b.diagOrg(w.Org, false, ir.Error, "V013", cc.Line, cc.Col, w.Path, "", "<include> takes no content; Tuimark has no slots")
			break
		}
	}
	src := w.Attrs["src"]
	if src == "" {
		b.errInc(w, "V003", `<include> needs src="FILE"`)
		return
	}
	if cp.selfOnly || cp.dead {
		return
	}
	cur := cp.chain[len(cp.chain)-1]
	var resolved string
	var outside bool
	if cp.ld != nil {
		resolved, outside = cp.ld.Join(cur.path, src)
	}
	// 1. Outside the file system.
	if outside {
		b.errInc(w, "V006", "include src=%q is outside the file system; root the fs.FS at a directory that holds both files, or copy the fragment inside it", src)
		return
	}
	var canon, name string
	if cp.ld != nil {
		canon, name = cp.ld.Canon(resolved), cp.ld.Name(resolved)
	}
	// 2. A cycle: the file is already being expanded on this chain, by
	// path or, when the loader can tell, as the same file on disk.
	if cp.ld != nil {
		for _, e := range cp.chain {
			if (e.canon != "" && e.canon == canon) || (cp.ld.Same != nil && e.real != "" && cp.ld.Same(e.real, resolved)) {
				names := make([]string, 0, len(cp.chain)+1)
				for _, f := range cp.chain {
					names = append(names, f.name)
				}
				names = append(names, name)
				b.errInc(w, "V006", "include src=%q is an include cycle: %s", src, strings.Join(names, " → "))
				return
			}
		}
	}
	// 3. A stylesheet.
	if strings.HasSuffix(src, ".tcss") {
		b.errInc(w, "V020", `include src=%q is a stylesheet: load it with <style src=%q>`, src, src)
		return
	}
	// 4. Unreadable.
	if cp.ld == nil {
		b.errInc(w, "V006", "include src=%q cannot be read: no file system to read it from", src)
		return
	}
	ff := cp.files[canon]
	if ff == nil {
		ff = &fragFile{}
		data, err := cp.ld.Read(resolved)
		if err != nil {
			ff.readErr = err
		} else {
			b.doc.Fragments = append(b.doc.Fragments, FragmentRef{Path: resolved, Name: name})
			ff.size = len(data)
			ff.root, _, _, ff.syntax = ParseXML(data)
		}
		if cp.files == nil {
			cp.files = map[string]*fragFile{}
		}
		cp.files[canon] = ff
	}
	if ff.readErr != nil {
		b.errInc(w, "V006", "include src=%q cannot be read: %v", src, unwrapOrSelf(ff.readErr))
		return
	}
	// 5. Not well-formed, or against §5: V005 in that file.
	root, serr := ff.root, ff.syntax
	fo := &ir.Origin{File: name, Placed: w}
	if serr != nil {
		b.diagOrg(fo, false, ir.Error, "V005", serr.Line, serr.Col, "", "", "%s", serr.Msg)
		return
	}
	// 6. Not a fragment.
	if root.Name != "fragment" {
		b.errInc(w, "V020", "include src=%q is not a fragment: its root is <%s>, and an included file's root is <fragment>", src, root.Name)
		return
	}
	b.expandFragment(n, w, outer, root, fo, chainFile{path: resolved, canon: canon, name: name, real: resolved}, src, ff.size)
}

// expandFragment builds the elements of a fragment in n (SPEC v0.4b
// §6.18.2). An expansion that passes ExpansionLimit is undone: the include
// inserts nothing, whatever it had inserted is discarded, and it reports
// V021; the includes around it keep what they inserted, and none expands
// after it (ADR 0034).
func (b *builder) expandFragment(n, w, outer *ir.Node, root *RawNode, fo *ir.Origin, file chainFile, src string, size int) {
	cp := b.comp
	saved := b.org
	chainLen := len(cp.chain)
	nChildren, nDropped, nDroppedIn := len(n.Children), len(b.doc.dropped), len(b.droppedIn[n])
	nStyles, nDiags, count, bytes := len(b.doc.Styles), len(b.doc.Diags), cp.count, cp.bytes
	nScope := len(cp.scope)
	nList := len(n.Source)
	if outer != nil {
		nList = len(outer.Children)
	}
	fr := &expFrame{}
	cp.frames = append(cp.frames, fr)
	defer func() {
		cp.frames = cp.frames[:len(cp.frames)-1]
		r := recover()
		if r == nil {
			return
		}
		if o, ok := r.(*expansionOverflow); !ok || o.frame != fr {
			panic(r)
		}
		b.org = saved
		cp.chain = cp.chain[:chainLen]
		cp.scope = cp.scope[:nScope] // an element of a copy left mid-build
		n.Children = n.Children[:nChildren]
		if outer != nil {
			outer.Children = outer.Children[:nList]
		} else {
			n.Source = n.Source[:nList]
		}
		b.doc.dropped = b.doc.dropped[:nDropped]
		if b.droppedIn != nil {
			b.droppedIn[n] = b.droppedIn[n][:nDroppedIn]
		}
		for _, st := range b.doc.Styles[nStyles:] {
			delete(b.styleSeen, styleKey(st.File, st.Line, st.Col))
		}
		b.doc.Styles = b.doc.Styles[:nStyles]
		b.doc.Diags = b.doc.Diags[:nDiags]
		w.Children = nil
		cp.count, cp.bytes = count, bytes
		cp.dead = true
		b.errInc(w, "V021", "include src=%q exceeds the expansion limit (%d elements or %d MiB of placed source): it inserts nothing, and no later include or use expands", src, ExpansionLimit, ExpansionByteLimit>>20)
	}()
	b.chargeBytes(size) // before anything is built
	b.countElement()    // the include itself, so it is the one that reports
	b.org = fo
	cp.chain = append(cp.chain, file)
	b.fragmentRoot(root)
	for _, fc := range root.Children {
		if fc.Type != RawElement {
			continue
		}
		if b.compose(n, fc, w) {
			continue
		}
		b.attach(n, w, b.node(fc, n))
	}
	cp.chain = cp.chain[:len(cp.chain)-1]
	b.org = saved
}

// fragmentRoot checks the root element of a fragment: it takes no
// attribute (version has its own message), and no character data other than
// whitespace (SPEC v0.4b §6.18.2). The diagnostics are in the fragment's
// file, which is b.org.
func (b *builder) fragmentRoot(root *RawNode) {
	for _, a := range root.Attrs {
		if a.Name == "version" {
			b.diag(ir.Error, "V002", a.Line, a.Col, "/", "", "a <fragment> takes the language of the document that includes it; remove version")
			continue
		}
		b.diag(ir.Error, "V002", a.Line, a.Col, "/", "", "attribute %q is not allowed on <fragment>", a.Name)
	}
	for _, c := range root.Children {
		if (c.Type == RawText || c.Type == RawCData) && !isXMLWhitespaceOnly(c.Text) {
			b.diag(ir.Error, "V013", c.Line, c.Col, "/", "", "text is not allowed directly inside <fragment>; wrap it in <text>")
			break
		}
	}
}

// FragmentDiags checks a file whose root is <fragment> on its own (SPEC
// v0.4b §6.18.2, §15.1, decision 20): on every element, only the checks an
// element gets from itself (an unknown tag, and the attribute and text
// rules), plus the root's own. Nothing that depends on a place, an id, or
// the data is reported, and an <include> gets only its own checks, its file
// unread. doc is the Document Parse returned for the file (FragmentRoot).
func FragmentDiags(doc *Document) ir.Diags {
	out := &Document{File: doc.File, IDs: map[string]*ir.Node{}, Lang: ir.LangCurrent, V2: true, V3: true}
	b := &builder{doc: out, file: docBase(doc.File), v2: true, v3: true, lang: ir.LangCurrent, comp: &composer{selfOnly: true}}
	if doc.Raw == nil {
		return nil
	}
	b.fragmentRoot(doc.Raw)
	holder := &ir.Node{Tag: "fragment", Kind: "fragment", Attrs: map[string]string{}, On: map[string]string{}, Path: "/"}
	for _, c := range doc.Raw.Children {
		if c.Type != RawElement {
			continue
		}
		if b.compose(holder, c, nil) {
			continue
		}
		b.attach(holder, nil, b.node(c, holder))
	}
	return dedupeDiags(out.Diags).Sorted()
}

// dedupeDiags keeps the first of the diagnostics equal in severity, code,
// message, file, line, and column: one written element placed more than
// once reports once (SPEC v0.4b §6.18.6), with the path and id of its first
// placement.
func dedupeDiags(ds ir.Diags) ir.Diags {
	type key struct {
		sev, code, msg, file string
		line, col            int
	}
	seen := make(map[key]bool, len(ds))
	out := ds[:0:0]
	for _, d := range ds {
		k := key{d.Severity, d.Code, d.Msg, d.File, d.Line, d.Col}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, d)
	}
	return out
}

// unwrapOrSelf is errors.Unwrap(err), or err itself when there is nothing
// to unwrap: a "cannot be read" message drops the redundant "open PATH:"
// prefix of an os or fs error.
func unwrapOrSelf(err error) error {
	if u := errors.Unwrap(err); u != nil {
		return u
	}
	return err
}

// dupID reports the V004 of the second occurrence n of an id first defined
// by prev, in document order (SPEC v0.4b §6.18.4). Without composition the
// message is the one of 0.4.1. With it, a repeat that is one written
// element placed twice is reported at the innermost include that placed the
// second occurrence, and the message names the file of the first
// occurrence when it is not the one reported, and what placed it.
func (b *builder) dupID(n, prev *ir.Node) {
	docFile := b.file
	nFile, pFile := n.FileName(docFile), prev.FileName(docFile)
	if n.Org == nil && prev.Org == nil {
		b.diagOrg(nil, false, ir.Error, "V004", n.Line, n.Col, n.Path, n.ID, "duplicate id %q (first defined at %d:%d)", n.ID, prev.Line, prev.Col)
		return
	}
	// The element and the file where the diagnostic stands.
	at, atFile := n, nFile
	if nFile == pFile && n.Line == prev.Line && n.Col == prev.Col && n.Org != nil && n.Org.Placed != nil {
		at = outermostNewPlacer(n, prev)
		atFile = at.FileName(docFile)
	}
	loc := fmt.Sprintf("%d:%d", prev.Line, prev.Col)
	if pFile != atFile {
		f := pFile
		if f == "" {
			f = "<input>"
		}
		loc = f + ":" + loc
	}
	msg := fmt.Sprintf("duplicate id %q (first defined at %s%s)", n.ID, loc, ir.PlacedSuffix(prev))
	path := n.Path
	if at != n {
		// Test 137 expects the path of the include here; §6.18.6 says the
		// element's own path. Provisional until the draft decides.
		path = at.Path
	}
	b.diagOrg(at.Org, false, ir.Error, "V004", at.Line, at.Col, path, n.ID, "%s", msg)
}

// outermostNewPlacer is the include that placed the second occurrence n of
// a written element and did not place the first, prev (SPEC v0.4b §6.18.4):
// walking out from the innermost include of n, the first whose written
// element (file, line, column) is not on prev's chain of includes. For a
// fragment included three times through one fragment that includes it, the
// innermost include is the same written element every time, so it is the
// outer one that tells the copies apart. It falls back to the innermost
// include when every one is on both chains.
func outermostNewPlacer(n, prev *ir.Node) *ir.Node {
	type site struct {
		file      string
		line, col int
	}
	seen := map[site]bool{}
	for p := prev.Placer(); p != nil; p = p.Placer() {
		seen[site{p.FileName(""), p.Line, p.Col}] = true
	}
	for p := n.Placer(); p != nil; p = p.Placer() {
		if !seen[site{p.FileName(""), p.Line, p.Col}] {
			return p
		}
	}
	return n.Org.Placed
}

// docBase is how diagnostics name a document: its base name ("" for none).
func docBase(file string) string {
	if file == "" {
		return ""
	}
	return filepath.Base(file)
}
