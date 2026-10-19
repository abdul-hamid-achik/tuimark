package host

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// The frame caches of 0.4a (SPEC v0.4a §21 test 125): internal, with no
// observable effect. Every one of them is checked against a cold build
// (noCache) by the equivalence tests, and against every frame the test
// suite builds by the tuimark_verify build tag (verify_on.go).
//
//   - The frame skip (liveFrame): Frame gives back the frame it built last
//     when nothing that frame was built from has changed since.
//   - The style sharing of one frame's cascade (shareStyles).
//   - A list's rows (listcache.go), kept while the store members they
//     read are unchanged.
//   - Each node's declarations as the cascade takes them (hintsOf,
//     inlineOf), built once, so equal nodes share them.

// nodeDecls is a node's presentational and style="" declarations, built
// once per node (hintsOf, inlineOf).
type nodeDecls struct {
	hints, inline []css.Decl
}

// declsOf returns n's declarations, built once per node. A slice is never
// changed once built, so boxes of the same node can share it, and the
// style sharing keys on its identity (css.DeclKey). The caller holds a.mu.
func (a *App) declsOf(n *ir.Node) *nodeDecls {
	if a.noCache {
		return &nodeDecls{a.toDecls(n.Hints), a.toDecls(n.Inline)}
	}
	d := a.decls[n]
	if d == nil {
		d = &nodeDecls{a.toDecls(n.Hints), a.toDecls(n.Inline)}
		a.decls[n] = d
	}
	return d
}

// hintsOf returns n's presentational declarations (declsOf).
func (a *App) hintsOf(n *ir.Node) []css.Decl { return a.declsOf(n).hints }

// inlineOf returns n's style="" declarations (declsOf).
func (a *App) inlineOf(n *ir.Node) []css.Decl { return a.declsOf(n).inline }

// liveFrame is Frame's build: the frame it built last, when every input
// of a frame is what that build left behind (frameInputs), else a new
// render. A render is a function of those inputs, and a render that
// settled (Frame.settled: its last pass moved neither focus nor a tab)
// leaves inputs from which the next render builds the same frame, with
// the same state and no event: the clamped offsets, cursors, and pages it
// wrote are fixed points of the same layout, and focus, the modal stack,
// and the active tabs are what it settled on. So the skip changes nothing
// a caller can see but the time. A pending focus request, and an
// unsettled render, always build. The caller holds a.mu.
func (a *App) liveFrame(cols, rows int, theme string) *Frame {
	if !a.noCache && a.last != nil && a.last == a.frameLast && a.focusReq == nil {
		if a.frameInputs(cols, rows, theme) == a.frameKey {
			return a.last
		}
	}
	f := a.renderLive(cols, rows, theme)
	a.frameKey, a.frameLast = "", nil
	if !a.noCache && f.settled && a.last == f && a.focusReq == nil {
		a.frameKey, a.frameLast = a.frameInputs(cols, rows, theme), f
	}
	return f
}

// renderLive is render for a live build (liveFrame): a list's rows go
// into its listSlab of the parity the live frame (a.last) does not use,
// so the build never writes a box of that frame, which it reads and every
// caller of Frame holds until it returns; the slab it writes held the
// boxes of the frame two builds back. Every pass and every trial render
// of this build (a focus request's, modalOpenOn) uses the same parity:
// only one of them becomes the frame. A cold app (noCache) never caches
// rows, so it allocates fresh memory as every other render does. The
// caller holds a.mu.
func (a *App) renderLive(cols, rows int, theme string) *Frame {
	a.slab = 1
	if a.last != nil && a.last.slab == 1 {
		a.slab = 2
	}
	defer func() { a.slab = 0 }()
	return a.render(cols, rows, theme)
}

// frameInputs is the fingerprint of everything a render reads besides the
// document and its stylesheets, which never change: the size and the
// effective theme, --strict, the store (by its write counters: every
// write goes through wrote), the screen, focus, the modals and their
// stack, the active and remembered tabs, every viewport offset and stick
// maximum, every list's and table's cursor state, and every input's
// state. The caller holds a.mu.
func (a *App) frameInputs(cols, rows int, theme string) string {
	b := make([]byte, 0, 256)
	num := func(n int64) { b = strconv.AppendInt(b, n, 10); b = append(b, ',') }
	str := func(s string) { num(int64(len(s))); b = append(b, s...) }
	flag := func(v bool) {
		if v {
			b = append(b, '1')
		} else {
			b = append(b, '0')
		}
	}
	num(int64(cols))
	num(int64(rows))
	str(css.EffectiveTheme(theme))
	flag(a.strict)
	num(int64(a.phase)) // SPEC v0.4b §26.13: the spinner phase is an input of the frame
	num(int64(a.storeGen))
	num(int64(a.writes))
	num(int64(a.screen))
	str(a.focus)
	flag(a.focusInit)
	b = append(b, "|m"...)
	for _, n := range a.openModals {
		str(fmt.Sprintf("%p", n))
	}
	b = append(b, "|s"...)
	for _, e := range a.modalStack {
		str(fmt.Sprintf("%p", e.node))
		str(e.ret)
	}
	b = append(b, "|o"...)
	for _, k := range sortedKeys(a.scrolls) {
		str(k)
		num(int64(a.scrolls[k][0]))
		num(int64(a.scrolls[k][1]))
	}
	b = append(b, "|k"...)
	for _, k := range sortedKeys(a.stickMax) {
		str(k)
		num(int64(a.stickMax[k]))
	}
	b = append(b, "|t"...)
	for _, k := range sortedKeys(a.tabMem) {
		str(k)
		str(a.tabMem[k])
	}
	b = append(b, "|p"...)
	for _, k := range sortedKeys(a.tabPrev) {
		str(k)
		str(a.tabPrev[k])
	}
	b = append(b, "|l"...)
	for _, k := range sortedKeys(a.lists) {
		ls := a.lists[k]
		str(k)
		num(int64(ls.index))
		num(int64(ls.page))
		str(ls.alias)
		flag(ls.each)
	}
	b = append(b, "|i"...)
	for _, k := range sortedKeys(a.inputs) {
		st := a.inputs[k]
		str(k)
		str(st.value)
		num(int64(st.cursor))
		flag(st.init)
	}
	return string(b)
}

// interpEntry is a template parsed once (segsOf).
type interpEntry struct {
	segs []ir.Segment
	ok   bool
}

// segsOf returns template s ({path} segments and literal text) parsed,
// and whether it parsed: parsed once per template text, which is always
// text of the document. The caller holds a.mu.
func (a *App) segsOf(s string) ([]ir.Segment, bool) {
	if e, ok := a.interps[s]; ok {
		return e.segs, e.ok
	}
	segs, err := ir.ParseInterp(s)
	e := interpEntry{segs, err == nil}
	if !a.noCache {
		if a.interps == nil {
			a.interps = map[string]interpEntry{}
		}
		a.interps[s] = e
	}
	return e.segs, e.ok
}

// docKinds records which widgets a document holds anywhere (every screen
// and modal): a frame of a document without them skips the steps that
// only look for them (tab activation and the tab focus rule, the hints,
// the sparkline scale groups), each of which finds nothing there and
// changes nothing.
type docKinds struct {
	doc   *parse.Document
	tabs  bool
	hints bool
	scale bool
	// spinner is set when the document holds a spinner (SPEC v0.4b
	// §6.4.1): only then does the spinner phase take part in a Run frame
	// (App.HasSpinner), so a document without one keeps the frame skip
	// as the phase moves.
	spinner bool
}

// kinds returns the docKinds of the current document, computed once per
// document. With noCache every step runs. The caller holds a.mu.
func (a *App) kinds() docKinds {
	if a.noCache {
		return docKinds{doc: a.doc, tabs: true, hints: true, scale: true, spinner: true}
	}
	if a.docKinds.doc == a.doc {
		return a.docKinds
	}
	k := docKinds{doc: a.doc}
	var walk func(n *ir.Node)
	walk = func(n *ir.Node) {
		switch n.Kind {
		case "tabs", "tab":
			k.tabs = true
		case "hints":
			k.hints = true
		case "progress":
			if n.Spinner {
				k.spinner = true
			}
		case "sparkline":
			if _, ok := n.Attr("scale"); ok {
				k.scale = true
			}
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	if a.doc != nil && a.doc.Root != nil {
		walk(a.doc.Root)
	}
	a.docKinds = k
	return k
}

// sortedKeys returns the keys of m in order.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
