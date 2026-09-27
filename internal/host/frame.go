package host

import (
	"fmt"
	"strconv"

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
}

// Dump renders the current state at cols×rows (negative sizes clamp to 0)
// and returns its dump. It is a snapshot: the runtime state a render
// advances (focus, pending events, modal tracking, viewport offsets, list
// pages, the last frame) is put back afterwards, so dumps at several sizes
// do not depend on their order. Frame is the call that advances it.
func (a *App) Dump(cols, rows int, cells bool) *dump.Dump {
	a.mu.Lock()
	defer a.mu.Unlock()
	saved := a.saveState()
	defer a.restoreState(saved)
	f := a.render(cols, rows)
	return dump.Build(f.Cols, f.Rows, f.Root, f.Modals, f.Grid, f.Diags, f.Focus, cells)
}

// Frame renders and returns the raw frame (for the terminal loop and tests).
func (a *App) Frame(cols, rows int) *Frame {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.render(cols, rows)
}

type builder struct {
	a      *App
	diags  ir.Diags
	seen   map[string]bool
	modals []*layout.Box
	byID   map[string]*layout.Box
}

func (fb *builder) report(n *ir.Node, sev, code, format string, args ...any) {
	d := ir.At(n, fb.a.file, sev, code, format, args...)
	k := d.String()
	if fb.seen[k] {
		return
	}
	fb.seen[k] = true
	fb.diags = append(fb.diags, d)
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
func (a *App) render(cols, rows int) *Frame {
	if req := a.focusReq; req != nil && !a.doc.Spike && a.screen != req.prevScreen && a.focus == req.target {
		if !a.modalOpenOn(req, cols, rows) {
			saved := a.saveState()
			trial := a.focusReq
			f := a.renderOnce(cols, rows)
			if trial.landed {
				return f
			}
			a.restoreState(saved)
		}
		a.screen, a.focusInit, a.focus, a.focusReq = req.prevScreen, req.prevInit, req.prev, nil
	}
	return a.renderOnce(cols, rows)
}

// modalOpenOn reports whether the screen a cross-screen focus request
// leaves shows a modal in a frame built now (store changes made in the
// same tick count). The runtime state is left untouched. The caller holds
// a.mu.
func (a *App) modalOpenOn(req *focusRequest, cols, rows int) bool {
	saved := a.saveState()
	defer a.restoreState(saved)
	a.screen, a.focusInit, a.focus, a.focusReq = req.prevScreen, req.prevInit, req.prev, nil
	return len(a.renderOnce(cols, rows).Modals) > 0
}

// renderOnce builds one frame on the active screen. The caller holds a.mu.
func (a *App) renderOnce(cols, rows int) *Frame {
	cols, rows = max(cols, 0), max(rows, 0)
	f := &Frame{Cols: cols, Rows: rows}
	if a.doc.Root == nil {
		f.Diags = append(f.Diags, a.static...)
		f.Grid = paint.NewGrid(cols, rows)
		f.ByID = map[string]*layout.Box{}
		a.last = f
		return f
	}
	root := a.doc.Root
	if !a.doc.Spike {
		if len(a.doc.Screens) == 0 {
			f.Diags = append(f.Diags, a.static...)
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
	initFocus := ""
	if !a.focusInit {
		a.focusInit = true
		if f, ok := root.Attr("focus"); ok && len(f) > 1 {
			a.focus = f[1:]
			initFocus = a.focus
		}
	}
	casc := css.NewCascade(a.sheets, css.Env{Cols: cols, Rows: rows, Theme: a.doc.Theme})
	var fb *builder
	var rootBox *layout.Box
	for pass := 0; pass < 3; pass++ {
		fb = &builder{a: a, seen: map[string]bool{}, byID: map[string]*layout.Box{}}
		rootBox = fb.inflate(root, nil, nil, false)
		for _, b := range append([]*layout.Box{rootBox}, fb.modals...) {
			if b != nil {
				computeStyles(casc, b, rootBoxStyle(b, rootBox))
			}
		}
		rootBox = fb.dropUndisplayed(rootBox)
		if !a.resolveFocus(f, fb, rootBox) {
			break
		}
	}
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
	eng := &layout.Engine{File: a.file}
	if rootBox == nil {
		rootBox = &layout.Box{Tag: root.Tag, Kind: root.Kind, Style: css.Initial()}
	}
	eng.Layout(rootBox, fb.modals, cols, rows)
	f.Root, f.Modals = rootBox, fb.modals
	f.Grid = paint.Paint(rootBox, fb.modals, cols, rows)
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
	for _, ds := range []ir.Diags{a.static, casc.Diags, fb.diags, eng.Diags} {
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
}

// computeStyles runs the cascade top-down and drops display:none subtrees.
func computeStyles(c *css.Cascade, b *layout.Box, parent *css.Style) {
	b.Style = c.Compute(b, parent, b.Hints, b.Inline)
	kept := b.Children[:0]
	for _, ch := range b.Children {
		computeStyles(c, ch, &b.Style)
		if ch.Style.Display != "none" {
			kept = append(kept, ch)
		}
	}
	b.Children = kept
}

// toDecls converts a node's hints or style="" declarations for the cascade.
func (a *App) toDecls(ps []ir.Prop) []css.Decl {
	out := make([]css.Decl, len(ps))
	for i, p := range ps {
		out[i] = css.Decl{Prop: p.Name, Value: p.Value, Line: p.Line, Col: p.Col, File: a.file}
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

func focusableByDefault(kind string) bool {
	switch kind {
	case "input", "list", "button", "modal":
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
	b := &layout.Box{
		Tag: n.Tag, Kind: n.Kind, ID: n.ID, Classes: n.Classes, Parent: parent, Src: n,
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
	for i, e := range entries {
		ib := fb.inflate(e.src, b, e.sc, true)
		if ib == nil {
			continue
		}
		ib.Selected = i == sel
		ib.Key = Format(e.key)
		ib.Index = i
		b.Children = append(b.Children, ib)
	}
	if len(entries) > 0 {
		b.Follow = min(sel, len(b.Children)-1)
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
	if b.Index >= 0 && b.Kind == "item" {
		return // list items are navigated by their list
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
		a.focus = list[0].ID
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
