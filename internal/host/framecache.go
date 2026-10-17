package host

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
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
	f := a.render(cols, rows, theme)
	a.frameKey, a.frameLast = "", nil
	if !a.noCache && f.settled && a.last == f && a.focusReq == nil {
		a.frameKey, a.frameLast = a.frameInputs(cols, rows, theme), f
	}
	return f
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

// sortedKeys returns the keys of m in order.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
