package host

import (
	"time"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/paint"
)

// The internal hooks `tuimark run` adds to RunHost's (SPEC v0.4a §15.9,
// "Internal hooks"): the overlay painted after each frame, the F12
// notice, the second ctrl+c, and the loop-side app swap of a reload.
// None of them is public API (§18): only cmd/tuimark's `run` sets them,
// and the zero HostHooks leaves Run exactly as it was.

// ctrlCWindow is how close two ctrl+c presses must be for the second to
// end a `tuimark run` session (HostHooks.CtrlCTwice, SPEC v0.4a §15.9
// "Always an exit"); a variable only so a test can narrow it.
var ctrlCWindow = time.Second

// The looks of the overlay rows: the panel's diagnostics bold and
// reversed, the strip's entries reversed, both in the terminal's default
// colors, so they read the same under every color profile.
const (
	panelAttrs = paint.Reverse | paint.Bold
	stripAttrs = paint.Reverse
)

// HostSwap is one reload `tuimark run` hands the loop on HostHooks.Swap
// (SPEC v0.4a §15.9 "Reload"). The loop takes it between inputs, as it
// takes a settle: it moves the session into App (unless App is nil), runs
// Applied, and settles, so the first frame of the new app is drawn,
// diffed with the last frame of the old one, and its lifecycle events
// run.
type HostSwap struct {
	// App is the app the session continues with: a new app built from the
	// reloaded files, which nothing else uses. nil keeps the current app
	// (a reload that failed): only Applied runs, and the frame is drawn
	// again.
	App *App
	// KeepStore moves the current app's store into App. When it is false,
	// App keeps its own store (--data changed, and App has the new data
	// bound at the root).
	KeepStore bool
	// Applied, when not nil, runs on the loop's goroutine once the swap is
	// applied, before the frame is drawn: the strip's "reload" line and
	// the panel's new contents show in that frame.
	Applied func()
}

// overlay returns a copy of g with top painted over its first rows and
// bottom over its last rows, bottom last, so a row both reach shows the
// bottom one (SPEC v0.4a §15.9: "where the panel and the strip overlap,
// the strip wins"). g itself, the frame's own grid that hit tests and
// dumps read, is left as it is.
func overlay(g *paint.Grid, top, bottom []string) *paint.Grid {
	c := g.Clone()
	for i, s := range top {
		c.PaintRow(i, s, panelAttrs)
	}
	for i, s := range bottom {
		c.PaintRow(c.H-len(bottom)+i, s, stripAttrs)
	}
	return c
}

// takeOver moves a running session's state from old, the app the session
// leaves, into a, a new app built from the reloaded files (SPEC v0.4a
// §15.9 "Success"). An app's document never changes after Load: a reload
// is a new app, and this is what carries over.
//
//   - The store, when keepStore is set; otherwise a keeps the one it has.
//   - The themes the session chose or resolved: the host's @theme, the
//     tools' theme, TUIMARK_THEME, and what theme="auto" resolved to.
//   - By id and tag (an id the new document has, on an element with the
//     same tag): the active screen, and, when the screen is kept, the
//     focus and the open modals with the focus each one gives back; list
//     and table cursors; viewport offsets and the stick="bottom" state;
//     an input's text and cursor; the active tab of a tabs without bind,
//     and the active tab of every tabs in the last frame, so the first
//     frame's tab rule sees no change where there is none.
//   - The last frame, which hit tests use until the next one is drawn and
//     which the first frame compares with.
//
// Everything else starts as on a first frame: no queued or kept
// lifecycle event, no pending press or focus request, and a's caches,
// which no frame has filled, are empty. So the first frame of a fires
// lifecycle events only for what changed: a modal open in both fires no
// on:open, and focus that lands somewhere else because its id is gone
// fires on:focus there. Only the loop's goroutine calls it, with neither
// app's lock held.
func (a *App) takeOver(old *App, keepStore bool) {
	old.mu.Lock()
	defer old.mu.Unlock()
	a.mu.Lock()
	defer a.mu.Unlock()
	if keepStore {
		a.store = old.store
		a.wrote("")
	}
	a.hostTheme, a.flagTheme, a.runEnvTheme, a.runAuto = old.hostTheme, old.flagTheme, old.runEnvTheme, old.runAuto

	kept := func(id string) *ir.Node {
		if id == "" {
			return nil
		}
		o, n := old.doc.IDs[id], a.doc.IDs[id]
		if o == nil || n == nil || o.Tag != n.Tag {
			return nil
		}
		return n
	}

	screenKept := false
	switch {
	case old.doc.Root == nil || a.doc.Root == nil:
	case old.doc.Spike || a.doc.Spike:
		// The spike's <app> root is no screen: the root is kept when
		// both documents are spikes.
		screenKept = old.doc.Spike && a.doc.Spike
	case old.screen >= 0 && old.screen < len(old.doc.Screens):
		if n := kept(old.doc.Screens[old.screen].ID); n != nil {
			for i, s := range a.doc.Screens {
				if s == n {
					a.screen, screenKept = i, true
				}
			}
		}
	}
	if screenKept {
		a.focusInit = old.focusInit
		if kept(old.focus) != nil {
			a.focus = old.focus
		}
		for _, n := range old.openModals {
			if m := kept(n.ID); m != nil {
				a.openModals = append(a.openModals, m)
			}
		}
		for _, e := range old.modalStack {
			if m := kept(e.node.ID); m != nil {
				a.modalStack = append(a.modalStack, modalEntry{node: m, ret: e.ret})
			}
		}
	}
	for id, ls := range old.lists {
		if kept(id) != nil {
			c := *ls
			c.keys = append([]any(nil), ls.keys...)
			a.lists[id] = &c
		}
	}
	for id, st := range old.inputs {
		if kept(id) != nil {
			c := *st
			a.inputs[id] = &c
		}
	}
	// An offset the wheel gave a viewport without an id is kept under a
	// key naming its old element (offsetKey), which no id matches: it
	// starts at 0 again.
	for k, off := range old.scrolls {
		if kept(k) != nil {
			a.scrolls[k] = off
		}
	}
	for id, m := range old.stickMax {
		if kept(id) != nil {
			a.stickMax[id] = m
		}
	}
	for id, tab := range old.tabMem {
		if kept(id) != nil {
			a.tabMem[id] = tab
		}
	}
	for id, tab := range old.tabPrev {
		if kept(id) != nil {
			a.tabPrev[id] = tab
		}
	}
	a.last = old.last
}

// leaveSession releases what a running session claimed on an app it no
// longer drives (an app a reload replaced, or the app it ends on when it
// is not the one Run was called on): the Run mode, and the themes the
// session resolved, which Dump and Validate never see (MUST 13).
func (a *App) leaveSession() {
	a.exitMode()
	a.mu.Lock()
	a.runAuto, a.runEnvTheme = "", ""
	a.mu.Unlock()
}
