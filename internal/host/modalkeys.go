package host

import (
	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/layout"
)

// The key and focus defaults of the current language (SPEC v0.4b §6.10.3,
// §8.1, §8.3, §8.6), in a <tui> without version only: a pinned document
// (version="1", "2", or "3", or the spike's <app>) keeps the 0.4.1 key
// dispatch exactly. Each rule is gated on Lang.V4(), never on LangCurrent.
//
//   - esc closes the top open modal that has no on:escape and whose
//     visibility path can take a false (escClose, §8.6 step 1);
//   - while a modal is open, a keymap row matches only when its effective
//     when is present and matches the modal chain (trapMatches, §8.1, §8.6
//     step 3): the modal traps the keymap;
//   - tabs is focusable by default (focusableIn, §6.10.1, §8.3), and focus
//     on its strip stays there when its active tab changes (focusWasOn,
//     §6.10.3).
//
// The dispatch itself stays the one procedure of dispatch.go (planKey),
// which Run, play, and <hints> share, so the hints follow these rules too.

// escPath returns the visibility path of modal m (SPEC v0.4b §8.6 step 1):
// its open when that is a path without "!", its bind path when it has no
// open at all. An open written as true, false, "", or !path gives it none,
// whatever its bind: open decides the modal's visibility then, and writing
// bind would not close it.
func escPath(m *layout.Box) (string, bool) {
	if m == nil || m.Src == nil {
		return "", false
	}
	if v, ok := m.Src.Attr("open"); ok {
		switch v {
		case "true", "false", "":
			return "", false
		}
		g, err := ir.ParseGuard(v)
		if err != nil || g.Neg {
			return "", false
		}
		return g.Path, true
	}
	if m.Src.Bind != "" {
		return m.Src.Bind, true
	}
	return "", false
}

// escClose reports whether esc closes the top open modal of v in the
// current language (SPEC v0.4b §8.6 step 1), and the path a false is then
// written at. The top modal must have no on:escape (planKey checks it
// first) and a visibility path (escPath) where a false can land: not a
// derived path, not inside one, and not an ancestor of one the
// recomputation could not write under false (derivedBlocked, SPEC v0.4b
// §18.2). There is no assignability check: while the modal is open its
// path resolves to a truthy value, so every value along it is an object.
// The caller holds a.mu.
func (a *App) escClose(v keyView) (string, bool) {
	if !a.doc.Lang.V4() {
		return "", false
	}
	path, ok := escPath(v.topModal())
	if !ok || a.derivedBlocked(path, false) {
		return "", false
	}
	return path, true
}

// closeOnEsc applies esc to the top modal that closes on it (escClose):
// the runtime writes false at its visibility path, whatever the path held,
// and no event fires for the key itself (SPEC v0.4b §8.6 step 1). The next
// frame no longer has the modal; it fires its on:close and gives focus
// back as SPEC §8.3 says. The caller holds a.mu.
func (a *App) closeOnEsc(path string) []Event {
	if err := a.writeStore(path, false); err != nil {
		// The store changed since the live frame (a host write in
		// between): the write cannot land, and the key does nothing.
		return nil
	}
	a.markStale()
	return nil
}

// trapped reports whether the keymap is trapped by an open modal on v
// (SPEC v0.4b §8.1): in a <tui> without version, while at least one
// modal is open.
func (a *App) trapped(v keyView) bool {
	return a.doc.Lang.V4() && len(v.modals) > 0
}

// modalChain returns the modal chain of v (SPEC v0.4b §8.1): the focused
// node and its ancestors up to and including the top modal when the
// focused node is inside the top modal; otherwise, nothing being focused
// included, the top modal alone. Only called while a modal is open.
func (v keyView) modalChain() []*layout.Box {
	top := v.topModal()
	var out []*layout.Box
	for b := v.focused; b != nil; b = b.Parent {
		out = append(out, b)
		if b == top {
			return out
		}
	}
	return []*layout.Box{top}
}

// trapMatches is whenMatches while the keymap is trapped (SPEC v0.4b §8.1,
// §8.6 step 3): a row matches only when its effective when (a <keymap
// when> included) is present and not empty, and its selector matches at
// least one node of the modal chain, each taken in turn as the subject. A
// row with no when, with when="", or with a when that is V003 (WhenSel is
// nil for all three) does not match while a modal is open.
func trapMatches(sel *css.Selector, v keyView) bool {
	if sel == nil {
		return false
	}
	for _, n := range v.modalChain() {
		if sel.Matches(n) {
			return true
		}
	}
	return false
}

// focusableIn reports the kinds that take focus without focusable="true"
// in a document of language l (SPEC §8.3): input, list, button, and modal;
// table from version="2" on; and tabs in a <tui> without version (SPEC
// v0.4b §6.10.1, §8.3), where focusable="false" takes it out.
func focusableIn(kind string, l ir.Lang) bool {
	if kind == "tabs" || kind == "checkbox" || kind == "select" {
		// SPEC v0.4b §8.3: in the current language tabs and the form
		// controls are focusable by default (a checkbox and a select exist
		// only there).
		return l.V4()
	}
	return focusableByDefault(kind)
}
