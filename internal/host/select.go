package host

import (
	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/layout"
)

// The <select> of SPEC v0.4b §6.17 (phase F9) at run time, in the frame
// order of §18:
//
//   - step 1 (inflateSelect) resolves every option's key and label, open
//     or closed, the chosen option, and the shown text;
//   - step 4 (closeOptionsIn) closes the open option list when its select
//     is not the focused node, is disabled, has no option, or is not in
//     the frame;
//   - step 7a (placeOptions) generates the open list and the rows its
//     offset shows, cascades them, and places them above the screen and
//     every modal, or closes the list when the select's visible rect is
//     empty;
//   - step 8 paints the list after the modals (paint.PaintFrame).
//
// A select exists only in a <tui> without version: the parser rejects it
// in a document that writes version.

// selectState is what the runtime keeps per select id: the key chosen
// without bind (mem, when has), and what the last render resolved, which
// the key dispatch, the mouse, and the event payloads read: the option
// keys, the chosen option's index (-1 for none), and the each alias.
type selectState struct {
	mem    any
	has    bool
	keys   []any
	chosen int
	alias  string
}

// optionList is the open option list (SPEC v0.4b §6.17.3): at most one is
// open, that of the select whose id is id ("" when none is), with its
// cursor and offset, and view, V, the rows its last placement showed (the
// page of pgup and pgdn).
type optionList struct {
	id     string
	cursor int
	offset int
	view   int
}

// selectOptions is a select's options as one frame resolved them: their
// keys and labels in array order, and the width of the widest label.
type selectOptions struct {
	keys   []any
	labels []string
	lw     int
	alias  string
}

// selectState returns the state of select id, created on first use. The
// caller holds a.mu.
func (a *App) selectState(id string) *selectState {
	st := a.selects[id]
	if st == nil {
		st = &selectState{chosen: -1}
		a.selects[id] = st
	}
	return st
}

// inflateSelect resolves select n for this frame (SPEC v0.4b §6.17.2):
// its options, the chosen one, and the shown text. b.Text is the chosen
// option's label, and b.Placeholder the placeholder only when no option
// is chosen (layout.SelectShown paints it dim); b.Rows is n, the number
// of options, and b.Measure the widest of the placeholder and the labels
// (§11.5.2). A select inside a list item or a container each template is
// V001 (§6.8 item 12): it is shown with its placeholder only, and its
// options are not resolved, so it reports nothing more. The caller holds
// a.mu.
func (fb *builder) inflateSelect(n *ir.Node, b *layout.Box, sc *scope, nested bool) {
	a := fb.a
	ph := oneLineLabel(fb.interp(n, n.Attrs["placeholder"], sc))
	b.Measure = layout.Width(ph)
	if nested || n.ID == "" {
		b.Placeholder = ph
		return
	}
	o := fb.resolveOptions(n, sc)
	st := a.selectState(n.ID)
	st.keys, st.alias = o.keys, o.alias
	st.chosen = fb.chosenOption(n, sc, o, st)
	if st.chosen >= 0 {
		b.Text = o.labels[st.chosen]
	} else {
		b.Placeholder = ph
	}
	b.Rows = len(o.keys)
	b.Measure = max(b.Measure, o.lw)
	if a.opts.id == n.ID {
		fb.open = o
	}
}

// resolveOptions resolves the options of select n (SPEC v0.4b §6.17.2):
// the elements of its each array, in order (a missing path or a value
// that is not an array is B001 and gives none). An option's key is key=
// resolved on its element, or its index when the path is missing there
// (B003 per option, an error under --strict, as a table row's key). Its
// label is label= resolved with the alias in scope (a missing path is
// B003, once per path, and the empty string), or, without label, its key
// formatted as text (decision 51); each LF a resolved value brings
// becomes a space. The caller holds a.mu.
func (fb *builder) resolveOptions(n *ir.Node, sc *scope) *selectOptions {
	a := fb.a
	o := &selectOptions{}
	e, err := ir.ParseEach(n.Each)
	if err != nil {
		return o
	}
	o.alias = e.Alias
	v, found := sc.resolve(a.store, e.Path)
	arr, isArr := v.([]any)
	switch {
	case !found:
		fb.report(n, ir.Error, "B001", "each=%q: path %q is missing (want an array)", n.Each, e.Path)
	case !isArr:
		fb.report(n, ir.Error, "B001", "each=%q: %q is %s, not an array", n.Each, e.Path, typeName(v))
	}
	keyPath, hasKey := n.Attr("key")
	label, hasLabel := n.Attr("label")
	var segs []ir.Segment
	if hasLabel {
		if s, ok := a.segsOf(label); ok {
			segs = s
		} else {
			segs = []ir.Segment{{Lit: label}}
		}
	}
	keySev := ir.Warning
	if a.strict {
		keySev = ir.Error
	}
	o.keys = make([]any, len(arr))
	o.labels = make([]string, len(arr))
	for i, el := range arr {
		isc := &scope{alias: e.Alias, value: el, parent: sc}
		var key any = float64(i)
		if hasKey && keyPath != "" {
			if kv, ok := isc.resolve(a.store, keyPath); ok {
				key = kv
			} else {
				fb.report(n, keySev, "B003", "key=%q is missing on option %d (its key is the index)", keyPath, i)
			}
		}
		isc.key = key
		o.keys[i] = key
		var text string
		if hasLabel {
			text = fb.interpSegs(n, segs, isc)
		} else {
			text = cleanText(Format(key))
		}
		text = oneLineLabel(text)
		o.labels[i] = text
		o.lw = max(o.lw, layout.Width(text))
	}
	return o
}

// chosenOption is the index of select n's chosen option, or -1 (SPEC
// v0.4b §6.17.2). With bind it is the first option whose key equals the
// bound value (keyEqual: the same JSON type and the same value formatted
// as text); none is chosen for null, for a missing path (B003), and for a
// value that is no option's key (B012, a warning, also with no option).
// The store is never written for these. Without bind it is the first
// option whose key equals the key the runtime remembers for the select,
// none until a pick.
func (fb *builder) chosenOption(n *ir.Node, sc *scope, o *selectOptions, st *selectState) int {
	if n.Bind == "" {
		if !st.has {
			return -1
		}
		return optionOf(o.keys, st.mem)
	}
	v, found := sc.resolve(fb.a.store, n.Bind)
	if !found {
		fb.missingBind(n, n.Bind)
		return -1
	}
	if v == nil {
		return -1
	}
	i := optionOf(o.keys, v)
	if i < 0 {
		fb.report(n, ir.Warning, "B012", "bind=%q holds %s, which is no option's key; nothing is chosen, and a pick writes over it", n.Bind, jsonText(v))
	}
	return i
}

// optionOf is the index of the first key equal to v (keyEqual), or -1.
func optionOf(keys []any, v any) int {
	for i, k := range keys {
		if keyEqual(k, v) {
			return i
		}
	}
	return -1
}

// selectValue is the chosen option's key of select b, as a keymap row's
// payload carries it (SPEC v0.4b §8.2, decision 65), or nil when none is
// chosen: the option whose key equals the bound value now (or the key
// remembered without bind), among the options of the last render. The
// caller holds a.mu.
func (a *App) selectValue(b *layout.Box) any {
	st := a.selects[b.ID]
	if st == nil {
		return nil
	}
	var v any
	if b.Src != nil && b.Src.Bind != "" {
		cur, found := lookup(a.store, b.Src.Bind)
		if !found || cur == nil {
			return nil
		}
		v = cur
	} else {
		if !st.has {
			return nil
		}
		v = st.mem
	}
	if i := optionOf(st.keys, v); i >= 0 {
		return deepCopyJSON(st.keys[i])
	}
	return nil
}

// openOptions opens the option list of select b (SPEC v0.4b §6.17.3):
// its cursor is the chosen option's index, or 0 with none chosen, and its
// offset follows the cursor when the list is placed. Opening fires
// nothing. The caller holds a.mu.
func (a *App) openOptions(b *layout.Box) {
	cursor := 0
	if st := a.selects[b.ID]; st != nil && st.chosen >= 0 {
		cursor = st.chosen
	}
	a.opts = optionList{id: b.ID, cursor: cursor}
	a.markStale()
}

// closeOptions closes the open option list, if any, without change. The
// caller holds a.mu.
func (a *App) closeOptions() {
	if a.opts.id == "" {
		return
	}
	a.opts = optionList{}
	a.markStale()
}

// optionsOpen reports whether the open option list belongs to b, the
// focused node of a key view: only then does it take the keys (SPEC v0.4b
// §8.6 step 0).
func (a *App) optionsOpen(b *layout.Box) bool {
	return a.opts.id != "" && b != nil && b.Kind == "select" && b.ID == a.opts.id
}

// moveOptions moves the open list's cursor to to(cursor, n, P), clamped
// to [0, n − 1], with P = max(1, V) (SPEC v0.4b §6.17.3). It writes
// nothing and fires nothing. The caller holds a.mu.
func (a *App) moveOptions(b *layout.Box, to func(cursor, n, page int) int) {
	st := a.selects[b.ID]
	if st == nil || len(st.keys) == 0 {
		return
	}
	n := len(st.keys)
	c := min(max(to(a.opts.cursor, n, max(1, a.opts.view)), 0), n-1)
	if c != a.opts.cursor {
		a.opts.cursor = c
		a.markStale()
	}
}

// optionsKey applies key k, which the open option list of the focused
// select b takes (SPEC v0.4b §8.6 step 0): up, down, home, end, pgup, and
// pgdn move its cursor; enter and space pick the cursor option; esc
// closes it without change; any other key does nothing. The caller holds
// a.mu.
func (a *App) optionsKey(b *layout.Box, k Key) []Event {
	switch k.Name {
	case "up":
		a.moveOptions(b, func(c, _, _ int) int { return c - 1 })
	case "down":
		a.moveOptions(b, func(c, _, _ int) int { return c + 1 })
	case "home":
		a.moveOptions(b, func(_, _, _ int) int { return 0 })
	case "end":
		a.moveOptions(b, func(_, n, _ int) int { return n - 1 })
	case "pgup":
		a.moveOptions(b, func(c, _, p int) int { return c - p })
	case "pgdn":
		a.moveOptions(b, func(c, _, p int) int { return c + p })
	case "enter", "space":
		return a.pickOption(b, a.opts.cursor)
	case "esc":
		a.closeOptions()
	}
	return nil
}

// pickOption picks option i of select b (SPEC v0.4b §6.17.3): the list
// closes; when the option's key K differs from the bound value (or,
// without bind, from the remembered key) by keyEqual, K is written with
// its JSON type to the bind path, which is created when missing (or
// remembered), and on:change fires with {source: b's id, keys: {alias:
// K}, value: K}. When K equals the current value nothing is written and
// nothing fires; a B012 value is written over. A bind path that cannot be
// written (a value along it present and not an object) or that is derived
// (§18.2) writes nothing and fires nothing. The caller holds a.mu.
func (a *App) pickOption(b *layout.Box, i int) []Event {
	a.closeOptions()
	st := a.selects[b.ID]
	if st == nil || i < 0 || i >= len(st.keys) {
		return nil
	}
	k := st.keys[i]
	if b.Src != nil && b.Src.Bind != "" {
		path := b.Src.Bind
		if cur, found := lookup(a.store, path); found && keyEqual(cur, k) {
			return nil
		}
		if !canAssign(a.store, path) || a.derivedBlocked(path, k) {
			return nil
		}
		if err := a.writeStore(path, deepCopyJSON(k)); err != nil {
			return nil
		}
	} else {
		if st.has && keyEqual(st.mem, k) {
			return nil
		}
		st.mem, st.has = deepCopyJSON(k), true
	}
	a.markStale()
	if b.Src == nil {
		return nil
	}
	if act, ok := b.Src.On["change"]; ok {
		return []Event{{Action: act, Source: b.ID, Keys: map[string]any{st.alias: deepCopyJSON(k)}, Value: deepCopyJSON(k)}}
	}
	return nil
}

// closeOptionsIn is the end of SPEC v0.4b §18 step 4: the open option
// list closes, without change and without an event, when its select is
// not the focused node, is disabled, has no option, or is not in the
// frame (§6.17.3). The caller holds a.mu.
func (fb *builder) closeOptionsIn(root *layout.Box) {
	a := fb.a
	if a.opts.id == "" {
		return
	}
	b := fb.byID[a.opts.id]
	v := keyView{root: root, modals: fb.modals}
	if b == nil || b.Kind != "select" || a.focus != b.ID || b.Disabled || b.Rows == 0 || !v.inFrame(b) {
		a.closeOptions()
	}
}

// placeOptions is SPEC v0.4b §18 step 7a: when a select's option list is
// open, it generates the list (a list, the child of its select for
// selectors and inheritance, with no id and no classes) and the rows its
// offset shows (item nodes, :selected on the cursor row and :checked on
// the chosen option's, each holding a text with the label), cascades
// them, and places them above the screen and every modal (§6.17.4). The
// list closes instead, before it is placed, when the select's visible
// rect is empty; that frame's hints were built with it open, so it is not
// kept for the frame skip. It returns the list, or nil. The caller holds
// a.mu.
func (fb *builder) placeOptions(casc *css.Cascade, f *Frame) *layout.Box {
	a := fb.a
	if a.opts.id == "" {
		return nil
	}
	sel := fb.byID[a.opts.id]
	o := fb.open
	grid := layout.Rect{W: f.Cols, H: f.Rows}
	if sel == nil || o == nil || !sel.Laid {
		a.closeOptions()
		f.settled = false
		return nil
	}
	vis := sel.Clip.Intersect(sel.Outer()).Intersect(grid)
	if vis.W <= 0 || vis.H <= 0 {
		a.closeOptions()
		f.settled = false
		return nil
	}
	n := len(o.keys)
	st := a.selects[sel.ID]
	// A host write while open changes n: the cursor stays where it was,
	// clamped to [0, n − 1] (§6.17.3).
	a.opts.cursor = min(max(a.opts.cursor, 0), n-1)
	l := &layout.Box{Tag: "list", Kind: "list", Parent: sel, Follow: -1, Index: -1}
	// Computed once, with a placeholder child, so :empty never matches
	// the list (its rows come after its geometry).
	l.Children = []*layout.Box{{Tag: "item", Kind: "item", Parent: l, Follow: -1, Index: -1}}
	l.Style = newStyle(casc.Compute(l, sel.Style, nil, nil))
	l.Children = nil
	v := layout.PlaceOptions(l, vis, n, o.lw, f.Cols, f.Rows)
	a.opts.view = v
	a.opts.offset = layout.OptionsOffset(a.opts.offset, a.opts.cursor, v, n)
	l.ScrollY, l.ContentH = a.opts.offset, max(v, n)
	chosen := -1
	if st != nil {
		chosen = st.chosen
	}
	for i := a.opts.offset; i < min(a.opts.offset+v, n); i++ {
		row := &layout.Box{
			Tag: "item", Kind: "item", Parent: l, Follow: -1, Index: i, Fixed: true,
			Selected: i == a.opts.cursor, Checked: i == chosen, Key: Format(o.keys[i]),
		}
		text := &layout.Box{Tag: "text", Kind: "text", Parent: row, Follow: -1, Index: -1, Fixed: true, Text: o.labels[i]}
		row.Children = []*layout.Box{text}
		row.Style = newStyle(casc.Compute(row, l.Style, nil, nil))
		text.Style = newStyle(casc.Compute(text, row.Style, nil, nil))
		layout.PlaceOptionRow(l, row, i)
		l.Children = append(l.Children, row)
	}
	return l
}
