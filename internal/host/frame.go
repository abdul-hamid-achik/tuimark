package host

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/dump"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/layout"
	"github.com/abdul-hamid-achik/tuimark/internal/paint"
	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// Frame is one rendered frame.
type Frame struct {
	Cols, Rows int
	Root       *layout.Box
	Modals     []*layout.Box
	Grid       *paint.Grid
	Diags      ir.Diags
	Focus      string
	Focusables []*layout.Box
	ByID       map[string]*layout.Box
	// Theme is the frame's effective theme, dark or light (SPEC §26.4).
	Theme string
	// V2 is set when the document is version="2": dumps of the frame
	// carry the nodes' classes (SPEC §13.2).
	V2 bool
	// Mouse is the value of the document's mouse attribute on this frame
	// (SPEC v0.2b §8.5): mouse events act only while it is true.
	Mouse bool
}

// Dump returns the frame's dump (SPEC §13.2), with the --cells map when
// cells is set.
func (f *Frame) Dump(cells bool) *dump.Dump {
	return dump.BuildWith(f.Cols, f.Rows, f.Root, f.Modals, f.Grid, f.Diags, f.Focus, dump.Options{Cells: cells, V2: f.V2})
}

// Dump renders the current state at cols×rows (negative sizes clamp to 0)
// and returns its dump. It is a snapshot: the runtime state a render
// advances (focus, pending events, modal tracking, viewport offsets, list
// pages, the last frame) is put back afterwards, so dumps at several sizes
// do not depend on their order. Frame is the call that advances it. The
// theme is the tools' (SPEC §26.4): @theme, else the document's, auto as
// dark; never TUIMARK_THEME or a theme Run probed (MUST 13).
func (a *App) Dump(cols, rows int, cells bool) *dump.Dump {
	a.mu.Lock()
	defer a.mu.Unlock()
	saved := a.saveState()
	defer a.restoreState(saved)
	return a.render(cols, rows, a.toolTheme()).Dump(cells)
}

// Frame renders and returns the raw frame (for the terminal loop and
// tests). Its theme is frameTheme: while Run runs, TUIMARK_THEME and the
// theme Run resolved for auto take part.
func (a *App) Frame(cols, rows int) *Frame {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.dirty = false // the live frame is fresh again (TakeDirty)
	return a.render(cols, rows, a.frameTheme())
}

type builder struct {
	a      *App
	diags  ir.Diags
	seen   map[string]bool
	modals []*layout.Box
	byID   map[string]*layout.Box
	// tables holds the tables inflated in step 1, for step 7.
	tables map[*layout.Box]*tableFrame
	// rc is the state of the render this builder's pass belongs to.
	rc *renderCtx
	// chain holds the source nodes of the focus chain (the focused node
	// and its ancestors): their boxes match :focus-within (SPEC §10.1).
	chain map[*ir.Node]bool
	// scopes holds the scope each tab was inflated in, for its content
	// (step 3).
	scopes map[*layout.Box]*scope
}

// renderCtx is what one render keeps across its passes (SPEC §18 step 4
// repeats steps 1-4): the focus when it started and whether it is the
// screen's first frame, and the tabs whose activation focus rule
// (§6.10.3) it has already applied.
type renderCtx struct {
	focusStart string
	initial    bool
	tabDone    map[string]bool
}

func (fb *builder) report(n *ir.Node, sev, code, format string, args ...any) {
	fb.add(ir.At(n, fb.a.file, sev, code, format, args...))
}

// render runs the whole pipeline. The caller holds a.mu.
//
// A focus request that switched screens (setFocus) is a no-op while the
// screen it leaves has a modal open in this frame, including a modal
// opened in the same tick: focus stays trapped in the modal (decisiones).
// Otherwise it is tried on the target's screen first. When the target
// cannot take focus there (resolveFocus did not mark the request landed),
// the request is a no-op too (SPEC §8.3). Either way everything the trial
// render changed is put back (pending events, modal tracking, list and
// input state), the screen, focus, and focusInit return to their values
// before the request, and the frame is built on the original screen.
func (a *App) render(cols, rows int, theme string) *Frame {
	if req := a.focusReq; req != nil && !a.doc.Spike && a.screen != req.prevScreen && a.focus == req.target {
		if !a.modalOpenOn(req, cols, rows, theme) {
			saved := a.saveState()
			trial := a.focusReq
			f := a.renderOnce(cols, rows, theme)
			if trial.landed {
				return f
			}
			a.restoreState(saved)
		}
		a.screen, a.focusInit, a.focus, a.focusReq = req.prevScreen, req.prevInit, req.prev, nil
		// Its tab activations are undone with it (SPEC §6.10.3).
		a.undoTabActs(req.acts)
	}
	return a.renderOnce(cols, rows, theme)
}

// modalOpenOn reports whether the screen a cross-screen focus request
// leaves shows a modal in a frame built now (store changes made in the
// same tick count). The runtime state is left untouched. The caller holds
// a.mu.
func (a *App) modalOpenOn(req *focusRequest, cols, rows int, theme string) bool {
	saved := a.saveState()
	defer a.restoreState(saved)
	a.screen, a.focusInit, a.focus, a.focusReq = req.prevScreen, req.prevInit, req.prev, nil
	return len(a.renderOnce(cols, rows, theme).Modals) > 0
}

// renderOnce builds one frame on the active screen under an effective
// theme (dark or light), in the order of SPEC §18:
//
//  1. inflate the active screen and its open modals (if, hidden, each,
//     text, class guards; the tab nodes of a tabs but not their content);
//  2. cascade, and drop display: none subtrees;
//  3. activate one tab per tabs, outermost first, and build its labels and
//     content (activateTabs);
//  4. resolve focus, repeating steps 1-4 while focus or an active tab
//     changes, at most three rounds as in v1;
//  5. build the items of each hints from the key dispatch on this tree
//     (buildHints);
//  6. measure and allocate (§11);
//  7. generate, cascade, and place the visible rows of each table
//     (placeTableRows);
//  8. paint (§12).
//
// "The frame" of §6.12 and §8.4 is the tree after step 4. Steps 2, 3, 5,
// and 7 are one cascade, with the frame's media and theme; the order only
// fixes when each node exists. The caller holds a.mu.
func (a *App) renderOnce(cols, rows int, theme string) *Frame {
	cols, rows = max(cols, 0), max(rows, 0)
	theme = css.EffectiveTheme(theme)
	f := &Frame{Cols: cols, Rows: rows, Theme: theme, V2: a.doc.V2}
	static := a.staticFor(theme)
	if a.doc.Root == nil {
		f.Diags = append(f.Diags, static...)
		f.Grid = paint.NewGrid(cols, rows)
		f.ByID = map[string]*layout.Box{}
		a.last = f
		return f
	}
	root := a.doc.Root
	if !a.doc.Spike {
		if len(a.doc.Screens) == 0 {
			f.Diags = append(f.Diags, static...)
			f.Grid = paint.NewGrid(cols, rows)
			f.ByID = map[string]*layout.Box{}
			a.last = f
			return f
		}
		if a.screen >= len(a.doc.Screens) {
			a.screen = 0
		}
		root = a.doc.Screens[a.screen]
	}
	rc := &renderCtx{focusStart: a.focus, initial: !a.focusInit, tabDone: map[string]bool{}}
	initFocus := ""
	if !a.focusInit {
		a.focusInit = true
		if f, ok := root.Attr("focus"); ok && len(f) > 1 {
			a.focus = f[1:]
			initFocus = a.focus
		}
	}
	casc := css.NewCascade(a.sheets, css.Env{Cols: cols, Rows: rows, Theme: theme})
	var fb *builder
	var rootBox *layout.Box
	for pass := 0; pass < 3; pass++ {
		fb = &builder{a: a, seen: map[string]bool{}, byID: map[string]*layout.Box{}, rc: rc, chain: a.focusChainIR()}
		rootBox = fb.inflate(root, nil, nil, false) // step 1
		rootBox = fb.cascade(casc, rootBox)         // step 2
		fb.activateTabs(casc, rootBox)              // step 3
		if !a.resolveFocus(f, fb, rootBox) {
			break // step 4
		}
	}
	a.tabPrev = fb.activeTabs(rootBox)
	// screen@focus gives initial focus like any other focus change: on:focus
	// fires once (resolveFocus reports the case where it had to pick another
	// target).
	if initFocus != "" && a.focus == initFocus {
		if b := fb.byID[a.focus]; b != nil && b.Src != nil {
			if act, ok := b.Src.On["focus"]; ok {
				a.pending = append(a.pending, Event{Action: act, Source: a.focus, Keys: map[string]any{}})
			}
		}
	}
	a.trackModals(fb)
	f.Mouse = fb.mouseOn()
	fb.buildHints(casc, rootBox) // step 5
	eng := &layout.Engine{File: a.file}
	if rootBox == nil {
		rootBox = &layout.Box{Tag: root.Tag, Kind: root.Kind, Style: css.Initial()}
	}
	eng.Layout(rootBox, fb.modals, cols, rows) // step 6
	fb.placeTableRows(casc, rootBox)           // step 7
	f.Root, f.Modals = rootBox, fb.modals
	f.Grid = paint.Paint(rootBox, fb.modals, cols, rows) // step 8
	f.ByID = fb.byID
	f.Focus = a.focus
	// Remember scroll offsets clamped by layout.
	rootBox.Walk(func(b *layout.Box) { a.rememberScroll(b) })
	for _, m := range fb.modals {
		m.Walk(func(b *layout.Box) { a.rememberScroll(b) })
	}
	// The static token check and the cascade can report the same V003.
	var diags ir.Diags
	seen := map[string]bool{}
	for _, ds := range []ir.Diags{static, casc.Diags, fb.diags, eng.Diags} {
		for _, d := range ds {
			if k := d.String(); !seen[k] {
				seen[k] = true
				diags = append(diags, d)
			}
		}
	}
	f.Diags = diags.Sorted()
	a.last = f
	return f
}

// cascade is step 2 of SPEC §18: it computes the style of the screen tree
// and of each open modal (a modal inherits from its screen) and drops
// display: none subtrees, the screen root and the modals included.
func (fb *builder) cascade(casc *css.Cascade, rootBox *layout.Box) *layout.Box {
	for _, b := range append([]*layout.Box{rootBox}, fb.modals...) {
		if b != nil {
			computeStyles(casc, b, rootBoxStyle(b, rootBox))
		}
	}
	return fb.dropUndisplayed(rootBox)
}

// focusChainIR returns the source nodes of the focus chain: the focused
// node and its ancestors up to the screen (a modal's parent is its
// screen). Their boxes match :focus-within (SPEC §10.1), which inflate
// marks as it creates them, so that the content a tab activation inflates
// in step 3 and the ancestors cascaded before it agree. A focused node is
// never inside a list row or an each template, so each source node of the
// chain has at most one box. With nothing focused it is empty. When the
// focused node turns out not to be in the frame, focus moves and the
// next pass marks the new chain. The caller holds a.mu.
func (a *App) focusChainIR() map[*ir.Node]bool {
	n := a.focusTarget(a.focus)
	if a.focus == "" || n == nil {
		return nil
	}
	m := map[*ir.Node]bool{}
	for p := n; p != nil && p.Kind != "tui"; p = p.Parent {
		m[p] = true
	}
	return m
}

// dropUndisplayed applies display: none to the boxes computeStyles cannot
// drop from a parent: the modals (styled as separate trees) and the screen
// root itself. A display:none root renders an empty frame, with no modals.
func (fb *builder) dropUndisplayed(root *layout.Box) *layout.Box {
	if root != nil && root.Style.Display == "none" {
		fb.modals = nil
		return nil
	}
	kept := fb.modals[:0]
	for _, m := range fb.modals {
		if m.Style.Display != "none" {
			kept = append(kept, m)
		}
	}
	fb.modals = kept
	return root
}

func rootBoxStyle(b, root *layout.Box) *css.Style {
	if b == root || root == nil {
		return nil
	}
	return &root.Style
}

func (a *App) rememberScroll(b *layout.Box) {
	// Every viewport keeps its clamped offset. For lists this means moving
	// the selection only scrolls when the item would leave the viewport.
	if b.Scrolls() && b.ID != "" {
		a.scrolls[b.ID] = [2]int{b.ScrollX, b.ScrollY}
	}
	if b.Kind == "list" && b.ID != "" {
		if ls := a.lists[b.ID]; ls != nil {
			ls.page = max(1, b.Content.H)
		}
	}
	if b.Kind == "table" && b.ID != "" && b.Laid {
		// P = max(1, V), the body viewport (SPEC §6.9.2).
		if ls := a.lists[b.ID]; ls != nil {
			ls.page = max(1, b.View)
		}
	}
}

// computeStyles runs the cascade top-down and drops display:none subtrees.
// A list keeps following the same row: its Follow moves with the child it
// names, or becomes -1 when display: none dropped that row.
func computeStyles(c *css.Cascade, b *layout.Box, parent *css.Style) {
	b.Style = c.Compute(subjectOf(b), parent, b.Hints, b.Inline)
	var follow *layout.Box
	if b.Kind == "list" && b.Follow >= 0 && b.Follow < len(b.Children) {
		follow = b.Children[b.Follow]
	}
	kept := b.Children[:0]
	for _, ch := range b.Children {
		computeStyles(c, ch, &b.Style)
		if ch.Style.Display != "none" {
			kept = append(kept, ch)
		}
	}
	b.Children = kept
	if follow != nil {
		b.Follow = -1
		for i, ch := range kept {
			if ch == follow {
				b.Follow = i
				break
			}
		}
	}
}

// subjectOf is the element the cascade matches as the subject when it
// computes b's own style. While a tab's own style is computed, :empty and
// :focus-within never match that tab: its style, display included, is
// needed before its content exists (SPEC §10.1, §18 step 2). Everywhere
// else, as a parent in a child selector for instance, it matches normally.
func subjectOf(b *layout.Box) css.Element {
	if b.Kind == "tab" {
		return tabSubject{b}
	}
	return b
}

// tabSubject is a tab box as the subject of its own cascade.
type tabSubject struct{ *layout.Box }

func (t tabSubject) HasPseudo(p string) bool {
	if p == "empty" || p == "focus-within" {
		return false
	}
	return t.Box.HasPseudo(p)
}

// toDecls converts a node's hints or style="" declarations for the cascade.
func (a *App) toDecls(ps []ir.Prop) []css.Decl {
	out := make([]css.Decl, len(ps))
	for i, p := range ps {
		out[i] = css.Decl{Prop: p.Name, Value: p.Value, Line: p.Line, Col: p.Col, File: a.file, Attr: p.Attr}
	}
	return out
}

// flag evaluates hidden/disabled/open: "true", "false", path, or !path.
func (fb *builder) flag(n *ir.Node, attr string, sc *scope) bool {
	v, ok := n.Attr(attr)
	if !ok {
		return false
	}
	switch v {
	case "true":
		return true
	case "false", "":
		return false
	}
	g, err := ir.ParseGuard(v)
	if err != nil {
		return false
	}
	val, found := sc.resolve(fb.a.store, g.Path)
	if !found {
		fb.report(n, ir.Warning, "B002", "%s=%q: path %q is missing", attr, v, g.Path)
	}
	return ir.Truthy(val) != g.Neg
}

// missingBind reports B003 for a path absent from the store (an error with
// --strict).
func (fb *builder) missingBind(n *ir.Node, path string) {
	sev := ir.Warning
	if fb.a.strict {
		sev = ir.Error
	}
	fb.report(n, sev, "B003", "bind path %q is missing", path)
}

// interp resolves {path} segments; missing paths render empty (B003).
func (fb *builder) interp(n *ir.Node, s string, sc *scope) string {
	if fb.a.doc.Spike || s == "" {
		return s
	}
	segs, err := ir.ParseInterp(s)
	if err != nil {
		return s
	}
	out := ""
	for _, sg := range segs {
		if sg.Path == "" {
			out += sg.Lit
			continue
		}
		v, ok := sc.resolve(fb.a.store, sg.Path)
		if !ok {
			fb.missingBind(n, sg.Path)
			continue
		}
		clean, _ := parse.StripControl(Format(v), true)
		out += clean
	}
	return out
}

// focusableByDefault reports the kinds that take focus without
// focusable="true" (SPEC §8.3): input, list, button, modal, and, in
// version="2" documents (the only ones that can hold one), table.
func focusableByDefault(kind string) bool {
	switch kind {
	case "input", "list", "button", "modal", "table":
		return true
	}
	return false
}

// inflate builds the Box tree for n: if/hidden prune, each expands, text
// resolves. Modals are collected into fb.modals.
func (fb *builder) inflate(n *ir.Node, parent *layout.Box, sc *scope, inItem bool) *layout.Box {
	a := fb.a
	switch n.Kind {
	case "tui", "style", "keymap", "bind":
		return nil
	}
	if n.If != "" {
		g, err := ir.ParseGuard(n.If)
		if err == nil {
			v, found := sc.resolve(a.store, g.Path)
			if !found {
				fb.report(n, ir.Warning, "B002", "if=%q: path %q is missing", n.If, g.Path)
			}
			if ir.Truthy(v) == g.Neg {
				return nil
			}
		}
	}
	if fb.flag(n, "hidden", sc) {
		return nil
	}
	if n.Kind == "modal" {
		open := false
		if v, ok := n.Attr("open"); ok {
			open = fb.flag(n, "open", sc)
			_ = v
		} else if n.Bind != "" {
			v, found := sc.resolve(a.store, n.Bind)
			if !found {
				fb.missingBind(n, n.Bind)
			}
			open = ir.Truthy(v)
		}
		if !open {
			return nil
		}
	}
	classes, guardOn := fb.classes(n, sc)
	b := &layout.Box{
		Tag: n.Tag, Kind: n.Kind, ID: n.ID, Classes: classes, GuardOn: guardOn, Parent: parent, Src: n,
		Hints: a.toDecls(n.Hints), Inline: a.toDecls(n.Inline), Follow: -1, Index: -1,
		Axis: n.Attrs["axis"],
	}
	if n.ID != "" && !inItem {
		fb.byID[n.ID] = b
	}
	// disabled covers the whole subtree, like a disabled <fieldset>.
	b.Disabled = fb.flag(n, "disabled", sc) || (parent != nil && parent.Disabled)
	b.Title = fb.interp(n, n.Attrs["title"], sc)
	if n.ID != "" && n.ID == a.focus && !inItem {
		b.Focused = true
	}
	if !inItem && fb.chain[n] {
		b.FocusWithin = true
	}
	switch n.Kind {
	case "text":
		b.Text = fb.interp(n, n.Text, sc)
	case "button":
		if l, ok := n.Attr("label"); ok {
			b.Text = l
		} else {
			b.Text = n.Text
		}
	case "input":
		b.Placeholder = fb.interp(n, n.Attrs["placeholder"], sc)
		b.Secret = n.Attrs["secret"] == "true"
		st := a.input(n.ID)
		if n.Bind != "" {
			v, found := sc.resolve(a.store, n.Bind)
			if !found {
				fb.missingBind(n, n.Bind)
			}
			b.Text = Format(v)
		} else {
			b.Text = st.value
		}
		b.Text, _ = parse.StripControl(b.Text, false)
		if !st.init {
			st.init = true
			st.cursor = len([]rune(b.Text))
		}
		st.cursor = min(max(st.cursor, 0), len([]rune(b.Text)))
		b.Cursor = st.cursor
	case "progress":
		if n.Bind != "" {
			v, found := sc.resolve(a.store, n.Bind)
			if !found {
				fb.missingBind(n, n.Bind)
			}
			if f, ok := v.(float64); ok {
				b.Value = f
			} else if found && v != nil {
				fb.report(n, ir.Warning, "B003", "progress bind %q is %s, want a number 0-100", n.Bind, typeName(v))
			}
		} else if s, ok := n.Attr("value"); ok {
			b.Value, _ = strconv.ParseFloat(s, 64)
		}
	}
	// Viewports (scroll, list, overflow: scroll) keep their offset; styles are
	// not computed yet, so restore whatever was remembered for this id.
	if off, ok := a.scrolls[n.ID]; ok && n.ID != "" && !inItem {
		b.ScrollX, b.ScrollY = off[0], off[1]
	}
	if n.Kind == "list" {
		fb.inflateList(n, b, sc)
		return b
	}
	switch n.Kind {
	case "table":
		// Its columns are its header cells; its row template and the
		// columns' cell templates are resolved on every row, whose visible
		// part is generated after layout (§18 step 7).
		fb.inflateTable(n, b, sc)
		return b
	case "sparkline":
		fb.inflateSparkline(n, b, sc)
		return b
	case "hints":
		// No children: a hints' items come from the keymap (step 5).
		return b
	case "tabs":
		if m, ok := n.Attr("mark"); ok {
			if w := layout.Width(m); w >= 1 && w <= 2 {
				b.Mark = m
			}
		}
	case "tab":
		// Step 1 creates the tab nodes of a tabs but not their content:
		// only the active tab's content is inflated, in step 3, in the
		// scope the tab was inflated in.
		if sc != nil {
			if fb.scopes == nil {
				fb.scopes = map[*layout.Box]*scope{}
			}
			fb.scopes[b] = sc
		}
		return b
	case "col", "row", "box":
		if n.Each != "" && a.doc.V2 {
			fb.inflateEach(n, b, sc)
			return b
		}
	}
	for _, c := range n.Children {
		cb := fb.inflate(c, b, sc, inItem)
		if cb == nil {
			continue
		}
		if c.Kind == "modal" {
			// Out of flow (a layer above the screen), but still the screen's
			// child for selector matching: screen > modal matches.
			fb.modals = append(fb.modals, cb)
			continue
		}
		b.Children = append(b.Children, cb)
	}
	return b
}

// inflateEach inflates the template of a col, row, or box with each=
// (SPEC §6.8): for each element of the array, in array order, every
// template child is inflated once, in document order, with the alias bound
// to the element. The container itself was inflated in the enclosing
// scope (its if, hidden, disabled, title, and class guards); it is not
// repeated. A missing path or a value that is not an array is B001 and
// leaves the container without children; an empty array too, so it then
// matches :empty. Each generated top-level node carries the element's key
// (key= resolved on the element, else its index; a key path missing on
// the element gives the index and B003); their descendants do not.
func (fb *builder) inflateEach(n *ir.Node, b *layout.Box, sc *scope) {
	a := fb.a
	e, err := ir.ParseEach(n.Each)
	if err != nil {
		return
	}
	v, found := sc.resolve(a.store, e.Path)
	arr, isArr := v.([]any)
	if !found {
		fb.report(n, ir.Error, "B001", "each=%q: path %q is missing (want an array)", n.Each, e.Path)
		return
	}
	if !isArr {
		fb.report(n, ir.Error, "B001", "each=%q: %q is %s, not an array", n.Each, e.Path, typeName(v))
		return
	}
	keyPath := n.Attrs["key"]
	for i, el := range arr {
		isc := &scope{alias: e.Alias, value: el, parent: sc}
		var key any = float64(i)
		if keyPath != "" {
			if kv, ok := isc.resolve(a.store, keyPath); ok {
				key = kv
			} else {
				sev := ir.Warning
				if a.strict {
					sev = ir.Error
				}
				fb.report(n, sev, "B003", "key=%q is missing on element %d (its key is the index)", keyPath, i)
			}
		}
		isc.key = key
		for _, c := range n.Children {
			// The template holds nothing focusable or addressed by id
			// (SPEC §6.8 item 8): it is inflated like a list row.
			cb := fb.inflate(c, b, isc, true)
			if cb == nil || c.Kind == "modal" {
				continue
			}
			cb.Key = Format(key)
			cb.Index = i
			b.Children = append(b.Children, cb)
		}
	}
}

// checkedKeys reads the checked= array of a list or table (SPEC §6.14): a
// missing path counts as [] and reports B003; a value that is not an array
// is B008 (an error), counts as [] for display, and ok is false, so the
// check-* actions never overwrite it.
func (fb *builder) checkedKeys(n *ir.Node, path string, sc *scope) (keys []any, ok bool) {
	v, found := sc.resolve(fb.a.store, path)
	if !found {
		fb.missingBind(n, path)
		return nil, true
	}
	arr, isArr := v.([]any)
	if !isArr {
		fb.report(n, ir.Error, "B008", "checked=%q: %q is %s, not an array of row keys (shown as none checked; check-toggle, check-all, and check-none leave it alone)", path, path, typeName(v))
		return nil, false
	}
	return arr, true
}

// hasKey reports whether some element of keys equals k (SPEC §6.14: the
// same JSON type and the same value formatted as text).
func hasKey(keys []any, k any) bool {
	for _, x := range keys {
		if keyEqual(x, k) {
			return true
		}
	}
	return false
}

// keyID is a row key's identity for SPEC §6.14 equality: its JSON type
// and its value formatted as text. keyID(a) == keyID(b) exactly when
// keyEqual(a, b), since a type name never holds a NUL.
func keyID(v any) string { return typeName(v) + "\x00" + Format(v) }

// keySet is a set of row keys by keyID: membership in O(1), where hasKey
// scans the array and formats both sides of every comparison.
type keySet map[string]struct{}

func newKeySet(keys []any) keySet {
	s := make(keySet, len(keys))
	for _, k := range keys {
		s[keyID(k)] = struct{}{}
	}
	return s
}

// has reports whether k equals some key of s (keyEqual).
func (s keySet) has(k any) bool {
	if len(s) == 0 {
		return false
	}
	_, ok := s[keyID(k)]
	return ok
}

// checkedCache is the key set of a list's or table's checked array, with
// the store stamp it was built at.
type checkedCache struct {
	stamp []uint64
	set   keySet
}

// checkedSet returns keys, the checked array of list or table node n read
// at path this frame, as a keySet, so :checked costs O(1) per row. In the
// document's scope it is kept per node while the store member that path
// is under is unchanged (a Set of it, a check-* action, or a whole-store
// write builds it again), so a frame that changed nothing there does not
// read the array; inside an each scope it is built every frame. The
// caller holds a.mu.
func (fb *builder) checkedSet(n *ir.Node, path string, keys []any, sc *scope) keySet {
	if len(keys) == 0 {
		return nil
	}
	if sc != nil {
		return newKeySet(keys)
	}
	a := fb.a
	head, _, _ := strings.Cut(path, ".")
	st := a.stamp([]string{head})
	if c := a.checkedSets[n]; c != nil && sameStamp(c.stamp, st) {
		return c.set
	}
	s := newKeySet(keys)
	a.checkedSets[n] = &checkedCache{stamp: st, set: s}
	return s
}

// classes returns an element's classes (SPEC §6.13): its class names in
// order, then the names of its truthy class:NAME guards in attribute
// order, without repeats. A guard's path is resolved in the element's
// scope (aliases of enclosing each included); a missing path is B002 and
// counts as null, as for if (class:x="path" adds nothing, class:x="!path"
// adds x). Without guards (every version="1" element) it is the class
// list itself. on is the truthiness of each guard.
func (fb *builder) classes(n *ir.Node, sc *scope) (out []string, on []bool) {
	if len(n.ClassGuards) == 0 {
		return n.Classes, nil
	}
	out = append([]string(nil), n.Classes...)
	on = make([]bool, len(n.ClassGuards))
	for i, cg := range n.ClassGuards {
		if !fb.guard(n, "class:"+cg.Name, cg.Guard, sc) {
			continue
		}
		on[i] = true
		dup := false
		for _, c := range out {
			if c == cg.Name {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, cg.Name)
		}
	}
	return out, on
}

// guard evaluates a path or !path guard written in attribute attr; a
// missing path is B002 (warning) and its value counts as null before the
// ! applies, as for if, hidden, disabled, open, and mouse (SPEC §7): path
// is false and !path is true.
func (fb *builder) guard(n *ir.Node, attr, v string, sc *scope) bool {
	g, err := ir.ParseGuard(v)
	if err != nil {
		return false
	}
	val, found := sc.resolve(fb.a.store, g.Path)
	if !found {
		fb.report(n, ir.Warning, "B002", "%s=%q: path %q is missing", attr, v, g.Path)
	}
	return ir.Truthy(val) != g.Neg
}

func typeName(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case bool:
		return "a bool"
	case float64:
		return "a number"
	case string:
		return "a string"
	case []any:
		return "an array"
	case map[string]any:
		return "an object"
	}
	return fmt.Sprintf("%T", v)
}

func (a *App) input(id string) *inputState {
	st, ok := a.inputs[id]
	if !ok {
		st = &inputState{}
		a.inputs[id] = st
	}
	return st
}

func (a *App) list(id string) *listState {
	st, ok := a.lists[id]
	if !ok {
		st = &listState{}
		a.lists[id] = st
	}
	return st
}

func keyEqual(a, b any) bool { return Format(a) == Format(b) && typeName(a) == typeName(b) }

// inflateList expands each= (or static items) and marks the selection.
func (fb *builder) inflateList(n *ir.Node, b *layout.Box, sc *scope) {
	a := fb.a
	ls := a.list(n.ID)
	if n.ID == "" {
		ls = &listState{}
	}
	var template *ir.Node
	var statics []*ir.Node
	for _, c := range n.Children {
		if c.Kind == "item" {
			if template == nil {
				template = c
			}
			statics = append(statics, c)
		}
	}
	type entry struct {
		sc  *scope
		key any
		src *ir.Node
	}
	var entries []entry
	if n.Each != "" {
		e, err := ir.ParseEach(n.Each)
		if err != nil || template == nil {
			return
		}
		v, found := sc.resolve(a.store, e.Path)
		arr, isArr := v.([]any)
		if !found {
			fb.report(n, ir.Error, "B001", "each=%q: path %q is missing (want an array)", n.Each, e.Path)
		} else if !isArr {
			fb.report(n, ir.Error, "B001", "each=%q: %q is %s, not an array", n.Each, e.Path, typeName(v))
		}
		keyPath := n.Attrs["key"]
		ls.alias, ls.each = e.Alias, true
		for i, el := range arr {
			isc := &scope{alias: e.Alias, value: el, parent: sc}
			var key any = float64(i)
			if keyPath != "" {
				if kv, ok := isc.resolve(a.store, keyPath); ok {
					key = kv
				} else {
					fb.report(n, ir.Warning, "B003", "key=%q is missing on item %d", keyPath, i)
				}
			}
			isc.key = key
			entries = append(entries, entry{isc, key, template})
		}
	} else {
		ls.each = false
		for i, s := range statics {
			var key any = float64(i)
			if s.ID != "" {
				key = s.ID
			}
			entries = append(entries, entry{sc, key, s})
		}
	}
	ls.keys = ls.keys[:0]
	for _, e := range entries {
		ls.keys = append(ls.keys, e.key)
	}
	sel := ls.index
	if n.Bind != "" {
		v, found := sc.resolve(a.store, n.Bind)
		if !found {
			fb.missingBind(n, n.Bind)
		}
		if found && v != nil {
			for i, k := range ls.keys {
				if keyEqual(k, v) {
					sel = i
					break
				}
			}
		}
	}
	if len(entries) == 0 {
		sel = 0
	} else {
		sel = min(max(sel, 0), len(entries)-1)
	}
	ls.index = sel
	// SPEC §6.14 (version="2"): the checked row keys, and the mark channel
	// every row reserves when the list has checked and mark.
	var checked keySet
	chanW, mark := 0, ""
	if a.doc.V2 {
		if path, ok := n.Attr("checked"); ok && ir.IsPath(path) {
			keys, _ := fb.checkedKeys(n, path, sc)
			checked = fb.checkedSet(n, path, keys, sc)
			if m, ok := n.Attr("mark"); ok {
				if w := layout.Width(m); w >= 1 && w <= 2 {
					chanW, mark = w+1, m
				}
			}
		}
	}
	for i, e := range entries {
		ib := fb.inflate(e.src, b, e.sc, true)
		if ib == nil {
			continue
		}
		ib.Selected = i == sel
		ib.Checked = checked.has(e.key)
		ib.Chan, ib.Mark = chanW, mark
		ib.Key = Format(e.key)
		ib.Index = i
		b.Children = append(b.Children, ib)
	}
	// The list follows its cursor row (SPEC §6.9.3, §8.4). Follow is an
	// index into the laid-out children, which skip the rows whose item if
	// or hidden pruned; a cursor row without an item (still a row, SPEC
	// v0.2b §6.14) has nothing to follow. computeStyles keeps it on the
	// same child when display: none drops rows.
	for j, c := range b.Children {
		if c.Index == sel {
			b.Follow = j
			break
		}
	}
}

// isFocusable reports whether a laid-out box can take focus.
func isFocusable(b *layout.Box) bool {
	if b.ID == "" || b.Disabled || b.Style.Visibility == "hidden" {
		return false
	}
	if v, ok := b.Src.Attr("focusable"); ok {
		return v == "true"
	}
	return focusableByDefault(b.Kind)
}

func collectFocusables(b *layout.Box, out *[]*layout.Box) {
	if b == nil {
		return
	}
	if b.Index >= 0 {
		// List items are navigated by their list; nothing in a container
		// each template takes focus (SPEC §6.8 item 8).
		return
	}
	if isFocusable(b) {
		*out = append(*out, b)
	}
	for _, c := range b.Children {
		collectFocusables(c, out)
	}
}

// resolveFocus picks a valid focus target; it returns true when focus
// changed and the tree must be rebuilt (so :focus rules apply).
//
// Modals trap focus in the top one (the last open in document order). The
// modal stack records, for each modal that became the top one, where focus
// was before it: closing it gives focus back there (no on:focus). A focus
// request made in the same tick as a modal closes wins over that restore
// when its target can take focus; the restore target is its fallback. A
// request that moves focus into a modal in the tick it opens still records
// the focus from before the request as the modal's return target.
func (a *App) resolveFocus(f *Frame, fb *builder, root *layout.Box) bool {
	before := a.focus
	// A pending focus request (action="focus", Set("@focus")) is decided by
	// the first pass; fallback is where focus goes if it cannot land.
	req := a.focusReq
	a.focusReq = nil
	if req != nil && a.focus != req.target {
		// A request that focus left behind: its tab activations are
		// undone, and a pass without them decides (SPEC §6.10.3).
		if a.undoTabActs(req.acts) {
			return true
		}
		req = nil
	}
	fallback := ""
	if req != nil {
		fallback = req.prev
	}
	if ret, ok := a.popModals(fb.modals); ok {
		if req != nil {
			fallback = ret
		} else {
			a.focus = ret
		}
	}
	var list []*layout.Box
	if n := len(fb.modals); n > 0 {
		top := fb.modals[n-1]
		if !a.inModalStack(top.Src) {
			ret := a.focus
			if req != nil {
				ret = fallback
			}
			a.modalStack = append(a.modalStack, modalEntry{node: top.Src, ret: ret})
		}
		for _, c := range top.Children {
			collectFocusables(c, &list)
		}
		if len(list) == 0 && isFocusable(top) {
			list = append(list, top)
		}
	} else {
		collectFocusables(root, &list)
	}
	f.Focusables = list
	focusable := func(id string) bool {
		for _, b := range list {
			if b.ID == id {
				return true
			}
		}
		return false
	}
	// A focus request only lands on a node that can take focus in this
	// frame: focusable, not hidden or disabled, inside the modal trap.
	// Anything else is a no-op: focus stays where it was (or where a modal
	// that closed gives it back) and no on:focus fires (SPEC §8.3).
	if req != nil {
		if focusable(req.target) {
			req.landed = true
			// A request that activated tabs (action="focus" only) gives
			// their on:select events, outermost first, before its
			// target's on:focus (SPEC §6.10.3).
			if req.fire {
				a.pending = append(a.pending, tabSelects(req.acts)...)
			}
			if req.fire && req.target != req.prev {
				if b := fb.byID[req.target]; b != nil && b.Src != nil {
					if act, ok := b.Src.On["focus"]; ok {
						a.pending = append(a.pending, Event{Action: act, Source: req.target, Keys: map[string]any{}})
					}
				}
			}
			return a.focus != before
		}
		a.focus = fallback
		// The activations of a request that cannot land are undone with
		// it (SPEC §6.10.3); the frame is then built again without them,
		// and that pass decides where focus goes.
		if a.undoTabActs(req.acts) {
			return true
		}
	}
	// A tab change moves focus that was on the strip, in the old tab, or
	// nowhere into the new tab (SPEC §6.10.3); focus that lands this way
	// fires on:focus, as initial focus does.
	if id, ok := fb.tabFocus(root, list); ok && id != a.focus {
		a.focus = id
		if b := fb.byID[id]; b != nil && b.Src != nil && id != before {
			if act, ok := b.Src.On["focus"]; ok {
				a.pending = append(a.pending, Event{Action: act, Source: id, Keys: map[string]any{}})
			}
		}
		return true
	}
	if focusable(a.focus) {
		return a.focus != before
	}
	prev := a.focus
	a.focus = ""
	if len(fb.modals) == 0 && (prev == "" || fb.byID[prev] == nil) {
		if sf, ok := a.currentScreenFocus(); ok {
			for _, b := range list {
				if b.ID == sf {
					a.focus = sf
				}
			}
		}
	}
	if a.focus == "" && len(list) > 0 {
		// On a screen's first frame, the first tabs whose active tab's
		// focus= can take focus comes before the first focusable node
		// (SPEC §6.10.3).
		if fb.rc != nil && fb.rc.initial && len(fb.modals) == 0 {
			a.focus = initialTabFocus(root, list)
		}
		if a.focus == "" {
			a.focus = list[0].ID
		}
	}
	if a.focus != before {
		if b := fb.byID[a.focus]; b != nil && b.Src != nil {
			if act, ok := b.Src.On["focus"]; ok {
				a.pending = append(a.pending, Event{Action: act, Source: a.focus, Keys: map[string]any{}})
			}
		}
		return true
	}
	return false
}

func (a *App) currentScreenFocus() (string, bool) {
	if a.doc.Spike || len(a.doc.Screens) == 0 {
		return "", false
	}
	if f, ok := a.doc.Screens[a.screen].Attr("focus"); ok && len(f) > 1 {
		return f[1:], true
	}
	return "", false
}

// popModals takes the modals that are no longer open off the modal stack.
// A modal still open above a closed one inherits the closed one's return
// target (its own pointed into the closed modal). When the top of the stack
// closed, it returns where focus goes back to: the return target of the
// lowest modal of the closed run at the top, which lies outside every
// modal that closed (stacked modals closing in one frame restore
// deterministically).
func (a *App) popModals(open []*layout.Box) (ret string, restore bool) {
	if len(a.modalStack) == 0 {
		return "", false
	}
	isOpen := make(map[*ir.Node]bool, len(open))
	for _, m := range open {
		isOpen[m.Src] = true
	}
	kept := a.modalStack[:0]
	for _, e := range a.modalStack {
		if !isOpen[e.node] {
			if !restore {
				ret, restore = e.ret, true
			}
			continue
		}
		if restore {
			e.ret, restore = ret, false
		}
		kept = append(kept, e)
	}
	a.modalStack = kept
	return ret, restore
}

func (a *App) inModalStack(n *ir.Node) bool {
	for _, e := range a.modalStack {
		if e.node == n {
			return true
		}
	}
	return false
}

// trackModals fires on:open for the modals that opened (document order)
// and on:close for those that closed, from the top down (reverse document
// order), so events for modals that change in the same frame come in a
// fixed order.
func (a *App) trackModals(fb *builder) {
	now := make([]*ir.Node, 0, len(fb.modals))
	isOpen := make(map[*ir.Node]bool, len(fb.modals))
	for _, m := range fb.modals {
		now = append(now, m.Src)
		isOpen[m.Src] = true
	}
	was := make(map[*ir.Node]bool, len(a.openModals))
	for _, n := range a.openModals {
		was[n] = true
	}
	for _, m := range fb.modals {
		if m.ID == "" || was[m.Src] {
			continue
		}
		if act, ok := m.Src.On["open"]; ok {
			a.pending = append(a.pending, Event{Action: act, Source: m.ID, Keys: map[string]any{}})
		}
	}
	for i := len(a.openModals) - 1; i >= 0; i-- {
		n := a.openModals[i]
		if n.ID == "" || isOpen[n] {
			continue
		}
		if act, ok := n.On["close"]; ok {
			a.pending = append(a.pending, Event{Action: act, Source: n.ID, Keys: map[string]any{}})
		}
	}
	a.openModals = now
}

// TakePending returns and clears queued lifecycle events.
func (a *App) TakePending() []Event {
	a.mu.Lock()
	defer a.mu.Unlock()
	p := a.pending
	a.pending = nil
	return p
}
