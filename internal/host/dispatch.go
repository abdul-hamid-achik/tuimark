package host

import (
	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/layout"
	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// The key dispatch of SPEC §8.6, written out once. Run and play reach it
// through HandleKey and HandleKeyRun on the live frame; <hints> runs the
// same procedure, without side effects, on the frame being built (§6.12,
// §18 step 5). planKey decides which step takes a key and never changes
// anything; handleKey applies what planKey decided.

// keyView is the frame the dispatch runs on: the live frame (liveView),
// or the frame being built after step 4 of SPEC §18. "The frame" of §8.4
// is its node tree: the screen root and the open modals, after if,
// hidden, display: none, and tab activation.
type keyView struct {
	focused *layout.Box            // the focused node; nil when nothing is focused
	root    *layout.Box            // the screen (nil when it is display: none)
	modals  []*layout.Box          // the open modals, top last
	byID    map[string]*layout.Box // the frame's nodes by id (outside list rows and templates)
}

// liveView is the live frame, the frame on the screen. The caller holds
// a.mu.
func (a *App) liveView() keyView {
	if a.last == nil {
		return keyView{}
	}
	return keyView{focused: a.focusedBox(), root: a.last.Root, modals: a.last.Modals, byID: a.last.ByID}
}

func (v keyView) topModal() *layout.Box {
	if len(v.modals) == 0 {
		return nil
	}
	return v.modals[len(v.modals)-1]
}

// chain returns the focus chain of SPEC §8.1: the focused node and its
// ancestors up to and including the screen (a modal's parent is its
// screen). With nothing focused, it is the top open modal, if any, then
// the screen.
func (v keyView) chain() []*layout.Box {
	start := v.focused
	if start == nil {
		if start = v.topModal(); start == nil {
			start = v.root
		}
	}
	var out []*layout.Box
	for b := start; b != nil; b = b.Parent {
		out = append(out, b)
	}
	return out
}

// inFrame reports whether b is in the frame: reachable from the screen
// root or an open modal through the children the frame kept (nodes
// dropped by display: none, inactive tabs, and nodes of other screens are
// not).
func (v keyView) inFrame(b *layout.Box) bool {
	for b != nil {
		if b == v.root {
			return true
		}
		if b.Kind == "modal" {
			for _, m := range v.modals {
				if m == b {
					return true
				}
			}
			return false
		}
		p := b.Parent
		if p == nil {
			return false
		}
		found := false
		for _, c := range p.Children {
			if c == b {
				found = true
				break
			}
		}
		if !found {
			return false
		}
		b = p
	}
	return false
}

// keyStep is the step of SPEC §8.6 that takes a key.
type keyStep int

const (
	stepNone       keyStep = iota // nothing takes the key: it does nothing
	stepEscape                    // 1: the top modal's on:escape fires
	stepWidget                    // 2: the focused, enabled widget consumes it
	stepKeymap                    // 3: a keymap row matches and fires
	stepBuiltinKey                // 4: tab, shift+tab, or ctrl+c
)

// keyPlan is what the dispatch decided for one key.
type keyPlan struct {
	step keyStep
	// row is the index in the keymap of the row that fires (stepKeymap).
	row int
	// target is the node a built-in action of §8.4 acts on (stepKeymap on
	// such a row; nil for switch-to, whose target is not in the frame).
	target *layout.Box
}

// planKey runs the key dispatch of SPEC §8.6 for k on v without side
// effects and returns the step that takes k:
//
//  1. esc, while the top open modal has on:escape: that action;
//  2. a focused, enabled node consumes the keys of its kind (consumes);
//  3. the keymap rows in document order: the first row whose keys hold k,
//     whose when matches (§8.1: the focused node in a version="1"
//     document, the focus chain in a version="2" one), and, for a
//     built-in of §8.4, whose target matches at run time;
//  4. tab and shift+tab cycle focus, ctrl+c fires quit.
//
// Anything else does nothing. The caller holds a.mu.
func (a *App) planKey(v keyView, k Key) keyPlan {
	if k.Name == "esc" {
		if m := v.topModal(); m != nil && m.Src != nil {
			if _, ok := m.Src.On["escape"]; ok {
				return keyPlan{step: stepEscape}
			}
		}
	}
	if f := v.focused; f != nil && !f.Disabled && a.consumes(f, k) {
		return keyPlan{step: stepWidget}
	}
	for i := range a.doc.Keymap {
		kb := &a.doc.Keymap[i]
		if !containsKey(kb.Keys, k.Name) || !a.whenMatches(kb.WhenSel, v) {
			continue
		}
		if ir.IsBuiltinActionV2(kb.Action) {
			// A version="1" document cannot name one (V003): the row
			// never runs. In version="2" the row matches only when its
			// target does (§8.4); otherwise dispatch goes on with the
			// next rows, exactly as for a row whose when fails.
			if !a.doc.V2 {
				continue
			}
			t, ok := a.builtinTarget(v, kb)
			if !ok {
				continue
			}
			return keyPlan{step: stepKeymap, row: i, target: t}
		}
		return keyPlan{step: stepKeymap, row: i}
	}
	switch k.Name {
	case "tab", "shift+tab", "ctrl+c":
		return keyPlan{step: stepBuiltinKey}
	}
	return keyPlan{}
}

func containsKey(keys []string, name string) bool {
	for _, k := range keys {
		if k == name {
			return true
		}
	}
	return false
}

// whenMatches reports whether a keymap row's when selector matches on v
// (SPEC §8.1, ADR 0008). A row without when always matches. In a
// version="1" document when must match the focused node, and nothing
// matches while nothing is focused. In a version="2" document it matches
// when the selector matches at least one node of the focus chain, each
// taken in turn as the subject (with > checked against that node's
// parents as usual).
func (a *App) whenMatches(sel *css.Selector, v keyView) bool {
	if sel == nil {
		return true
	}
	if !a.doc.V2 {
		return v.focused != nil && sel.Matches(v.focused)
	}
	for _, n := range v.chain() {
		if sel.Matches(n) {
			return true
		}
	}
	return false
}

// consumes reports whether the focused, enabled node b consumes k (SPEC
// §8.6 step 2):
//
//   - an input: every single-character key and space (typed), backspace,
//     ctrl+h, ctrl+u, left, right, home, end, ctrl+a, ctrl+e, and enter
//     when it has on:submit;
//   - a list with at least one row: up, down, home, end, pgup, pgdn;
//   - any other viewport: up, down, left, right, pgup, pgdn, home, end;
//   - after those, any node other than an input or a list that has
//     on:click: enter and space, which fire it.
//
// (A table consumes what a list does, and a focusable tabs left and
// right; they arrive with those widgets.) The caller holds a.mu.
func (a *App) consumes(b *layout.Box, k Key) bool {
	switch b.Kind {
	case "input":
		if isPrintable(k) {
			return true
		}
		switch k.Name {
		case "backspace", "ctrl+h", "ctrl+u", "left", "right", "home", "end", "ctrl+a", "ctrl+e":
			return true
		case "enter":
			_, ok := b.Src.On["submit"]
			return ok
		}
		return false
	case "list":
		if a.rows(b) > 0 {
			switch k.Name {
			case "up", "down", "home", "end", "pgup", "pgdn":
				return true
			}
		}
		// A list never fires on:click from the keyboard (v1).
		return false
	}
	if b.Scrolls() {
		switch k.Name {
		case "up", "down", "left", "right", "pgup", "pgdn", "home", "end":
			return true
		}
	}
	if (k.Name == "enter" || k.Name == "space") && b.Src != nil {
		_, ok := b.Src.On["click"]
		return ok
	}
	return false
}

// rows is the number of rows of a list (its each array's elements, or
// its static items), as the last render inflated them.
func (a *App) rows(b *layout.Box) int {
	if ls := a.lists[b.ID]; ls != nil {
		return len(ls.keys)
	}
	return 0
}

// handleKey runs the key dispatch of SPEC §8.6 for k on the live frame and
// returns the events to dispatch, in order. The caller holds a.mu.
func (a *App) handleKey(k Key) []Event {
	v := a.liveView()
	p := a.planKey(v, k)
	switch p.step {
	case stepEscape:
		m := v.topModal()
		return []Event{{Action: m.Src.On["escape"], Source: m.ID, Keys: map[string]any{}}}
	case stepWidget:
		return a.widgetKey(v.focused, k)
	case stepKeymap:
		kb := &a.doc.Keymap[p.row]
		switch {
		case kb.Action == "focus":
			return a.focusTo(kb.To)
		case ir.IsBuiltinActionV2(kb.Action):
			return a.runBuiltin(kb, p.target)
		}
		// A host action (or quit) fired by a keymap row: the focused node
		// is its source, never the node its when matched over the focus
		// chain (§8.2).
		return []Event{a.eventFor(kb.Action, v.focused)}
	case stepBuiltinKey:
		switch k.Name {
		case "tab":
			return a.cycleFocus(1)
		case "shift+tab":
			return a.cycleFocus(-1)
		case "ctrl+c":
			return []Event{{Action: "quit", Source: a.focus, Keys: map[string]any{}}}
		}
	}
	return nil
}

// widgetKey applies a key the focused widget b consumes (consumes is
// true) and returns its events. The caller holds a.mu.
func (a *App) widgetKey(b *layout.Box, k Key) []Event {
	switch b.Kind {
	case "input":
		if isPrintable(k) {
			return a.insertText(b, []rune{k.Rune})
		}
		// The cursor stands between grapheme clusters: left, right, and
		// backspace move over or delete a whole cluster (SPEC v0.2 §12.1).
		st, runes, bounds := a.inputCursor(b)
		changed := false
		switch k.Name {
		case "backspace", "ctrl+h":
			if st.cursor > 0 {
				p := boundBefore(bounds, st.cursor)
				runes = append(runes[:p], runes[st.cursor:]...)
				st.cursor = p
				changed = true
			}
		case "ctrl+u":
			if len(runes) > 0 {
				runes, st.cursor, changed = nil, 0, true
			}
		case "left":
			st.cursor = boundBefore(bounds, st.cursor)
		case "right":
			st.cursor = boundAfter(bounds, st.cursor)
		case "home", "ctrl+a":
			st.cursor = 0
		case "end", "ctrl+e":
			st.cursor = len(runes)
		case "enter":
			return []Event{a.eventFor(b.Src.On["submit"], b)}
		}
		if changed {
			a.setInputValue(b, string(runes))
			if act, ok := b.Src.On["change"]; ok {
				return []Event{a.eventFor(act, b)}
			}
		}
		return nil
	case "list":
		switch k.Name {
		case "up":
			return a.moveList(b, func(i, _, _ int) int { return i - 1 })
		case "down":
			return a.moveList(b, func(i, _, _ int) int { return i + 1 })
		case "home":
			return a.moveList(b, func(_, _, _ int) int { return 0 })
		case "end":
			return a.moveList(b, func(_, n, _ int) int { return n - 1 })
		case "pgup":
			return a.moveList(b, func(i, _, p int) int { return i - p })
		case "pgdn":
			return a.moveList(b, func(i, _, p int) int { return i + p })
		}
		return nil
	}
	if b.Scrolls() {
		off := a.scrolls[b.ID]
		page := max(1, b.Content.H)
		switch k.Name {
		case "up":
			off[1]--
		case "down":
			off[1]++
		case "left":
			off[0]--
		case "right":
			off[0]++
		case "pgup":
			off[1] -= page
		case "pgdn":
			off[1] += page
		case "home":
			off[1] = 0
		case "end":
			off[1] = b.ContentH
		default:
			return a.clickKey(b)
		}
		off[0], off[1] = max(0, off[0]), max(0, off[1])
		a.scrolls[b.ID] = off
		return nil
	}
	return a.clickKey(b)
}

// clickKey fires b's on:click for enter or space.
func (a *App) clickKey(b *layout.Box) []Event {
	if act, ok := b.Src.On["click"]; ok {
		return []Event{{Action: act, Source: b.ID, Keys: map[string]any{}}}
	}
	return nil
}

// moveList moves a list's cursor as a user move: to(index, rows, page)
// gives the new index, which is clamped and never wraps (page is P =
// max(1, content height)). When the index changed, the new row's key,
// with its JSON type, is written to bind and on:select fires; the list
// follows its cursor in the next frame. The caller holds a.mu.
func (a *App) moveList(b *layout.Box, to func(index, rows, page int) int) []Event {
	ls := a.lists[b.ID]
	if ls == nil || len(ls.keys) == 0 {
		return nil
	}
	n := len(ls.keys)
	idx := min(max(to(ls.index, n, max(1, ls.page)), 0), n-1)
	if idx == ls.index {
		return nil
	}
	ls.index = idx
	if b.Src.Bind != "" {
		if root, err := assign(a.store, b.Src.Bind, ls.keys[idx]); err == nil {
			a.store = root
		}
	}
	if act, ok := b.Src.On["select"]; ok {
		return []Event{a.eventFor(act, b)}
	}
	return nil
}

// builtinTarget resolves the target of a keymap row naming a built-in of
// SPEC §8.4 and reports whether the row matches at run time. The target is
// the node named by to=, else the focused node. For every action but
// switch-to it must be in the frame, not disabled, and compatible;
// switch-to names an inactive tab or another screen, which are never in
// the frame, so only compatibility applies to it. The caller holds a.mu.
func (a *App) builtinTarget(v keyView, kb *parse.KeyBind) (*layout.Box, bool) {
	if kb.Action == "switch-to" {
		return nil, a.switchable(kb.To)
	}
	t := v.focused
	if kb.To != "" {
		t = v.byID[kb.To]
	}
	if t == nil || t.Disabled || !v.inFrame(t) {
		return nil, false
	}
	return t, a.compatible(kb.Action, t)
}

// isMove reports whether action is one of the move-* built-ins.
func isMove(action string) bool {
	switch action {
	case "move-next", "move-prev", "move-first", "move-last", "move-page-down", "move-page-up":
		return true
	}
	return false
}

// compatible reports whether t can take a built-in action (SPEC §8.4):
// move-* needs a list with at least one row or a viewport that is not a
// list; check-* a list that has checked=, whose value is an array or
// missing (never B008), with at least one row. (A table, and a tabs with
// an enabled visible tab, arrive with those widgets.) The caller holds
// a.mu.
func (a *App) compatible(action string, t *layout.Box) bool {
	switch {
	case isMove(action):
		switch t.Kind {
		case "list":
			return a.rows(t) > 0
		case "table", "tabs":
			return false
		}
		return t.Scrolls()
	case action == "check-toggle" || action == "check-all" || action == "check-none":
		if t.Kind != "list" || a.rows(t) == 0 {
			return false
		}
		_, ok := a.checkedArray(t)
		return ok
	}
	return false
}

// checkedArray returns the checked= array of a list (SPEC §6.14): the
// array, or nil when the path is missing (it counts as []); ok is false
// when the list has no checked= or its value is present and not an array
// (B008), which the runtime never overwrites. A list is never inside an
// each template, so its path resolves against the store. The caller holds
// a.mu.
func (a *App) checkedArray(b *layout.Box) (keys []any, ok bool) {
	if !a.doc.V2 || b.Src == nil {
		return nil, false
	}
	path, has := b.Src.Attr("checked")
	if !has || !ir.IsPath(path) {
		return nil, false
	}
	v, found := lookup(a.store, path)
	if !found {
		return nil, true
	}
	arr, isArr := v.([]any)
	return arr, isArr
}

// switchable reports whether switch-to can act on the node id (SPEC §8.4):
// a screen. (A visible, enabled tab whose tabs is in the frame arrives with
// the tabs widget.) The caller holds a.mu.
func (a *App) switchable(id string) bool {
	n := a.doc.IDs[id]
	return n != nil && n.Kind == "screen"
}

// runBuiltin applies a built-in action of SPEC §8.4 to the target
// builtinTarget resolved and returns the events it fires: on:select when a
// cursor moved, on:change when the checked array changed. A matching row
// consumes its key even when nothing changes. The caller holds a.mu.
func (a *App) runBuiltin(kb *parse.KeyBind, t *layout.Box) []Event {
	switch kb.Action {
	case "switch-to":
		// As Set("@screen", id) does.
		prev := a.screen
		a.switchScreen(kb.To)
		if a.screen != prev {
			a.dirty = true
		}
		return nil
	case "check-toggle", "check-all", "check-none":
		return a.check(t, kb.Action)
	}
	if t.Kind == "list" {
		// The cursor moved without an event when the list has no
		// on:select: the live frame is stale all the same.
		if ls := a.lists[t.ID]; ls != nil {
			prev := ls.index
			defer func() {
				if ls.index != prev {
					a.dirty = true
				}
			}()
		}
		switch kb.Action {
		case "move-next":
			return a.moveList(t, func(i, _, _ int) int { return i + 1 })
		case "move-prev":
			return a.moveList(t, func(i, _, _ int) int { return i - 1 })
		case "move-first":
			return a.moveList(t, func(_, _, _ int) int { return 0 })
		case "move-last":
			return a.moveList(t, func(_, n, _ int) int { return n - 1 })
		case "move-page-down":
			return a.moveList(t, func(i, _, p int) int { return i + p })
		case "move-page-up":
			return a.moveList(t, func(i, _, p int) int { return i - p })
		}
		return nil
	}
	a.moveViewport(t, kb.Action)
	return nil
}

// moveViewport moves the offset of a viewport that is not a list on its
// scroll axis (y, or x when it scrolls only on x): by one for
// move-next/move-prev, to 0 or its maximum for move-first/move-last, by
// its content-box size for move-page-*, clamped to [0, maximum] of the
// live frame. The caller holds a.mu.
func (a *App) moveViewport(b *layout.Box, action string) {
	off := a.scrolls[b.ID]
	sx, sy := b.ScrollAxes()
	axis, size, extent := 1, b.Content.H, b.ContentH
	if sx && !sy {
		axis, size, extent = 0, b.Content.W, b.ContentW
	}
	most := max(0, extent-size)
	cur := min(max(off[axis], 0), most)
	switch action {
	case "move-next":
		cur++
	case "move-prev":
		cur--
	case "move-first":
		cur = 0
	case "move-last":
		cur = most
	case "move-page-down":
		cur += max(1, size)
	case "move-page-up":
		cur -= max(1, size)
	}
	cur = min(max(cur, 0), most)
	if cur != off[axis] {
		off[axis] = cur
		a.scrolls[b.ID] = off
		a.dirty = true
	}
}

// check applies check-toggle, check-all, or check-none to a list (SPEC
// §6.14). With K the key of the cursor row and R the array: check-toggle
// removes every element equal to K when there is one, else appends K with
// its JSON type; check-all appends, in data order, every row key (over all
// rows of the array, not only the visible ones) that equals no element of
// R, keeping the existing elements, stale keys included; check-none makes
// R empty. When the new array differs from the old one, it is written to
// the path and on:change fires with the new array as its value; otherwise
// nothing is written and nothing fires. The caller holds a.mu.
func (a *App) check(b *layout.Box, action string) []Event {
	ls := a.lists[b.ID]
	old, ok := a.checkedArray(b)
	if ls == nil || len(ls.keys) == 0 || !ok {
		return nil
	}
	next := []any{}
	switch action {
	case "check-toggle":
		k := ls.keys[min(max(ls.index, 0), len(ls.keys)-1)]
		if hasKey(old, k) {
			for _, x := range old {
				if !keyEqual(x, k) {
					next = append(next, x)
				}
			}
		} else {
			next = append(append(next, old...), k)
		}
	case "check-all":
		next = append(next, old...)
		for _, k := range ls.keys {
			if !hasKey(next, k) {
				next = append(next, k)
			}
		}
	}
	if sameKeys(old, next) {
		return nil
	}
	root, err := assign(a.store, b.Src.Attrs["checked"], next)
	if err != nil {
		return nil
	}
	a.store = root
	a.dirty = true
	if act, ok := b.Src.On["change"]; ok {
		ev := a.eventFor(act, b)
		ev.Value = append([]any{}, next...)
		return []Event{ev}
	}
	return nil
}

// sameKeys reports whether two checked arrays are equal: the same length
// and, element by element, the same JSON type and value.
func sameKeys(a, b []any) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !keyEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

// TakeDirty reports, and clears, whether a key changed runtime state that
// the live frame shows without firing an event or moving focus (a cursor,
// an offset, a checked array, the screen), so Run and play redraw before
// the next key of the same read is dispatched on that frame.
func (a *App) TakeDirty() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	d := a.dirty
	a.dirty = false
	return d
}
