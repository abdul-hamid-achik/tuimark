package host

import (
	"strings"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/layout"
)

// The <tabs> widget of SPEC v0.2b §6.10, in the frame order of §18:
//
//   - step 1 (inflate) creates the tabs box and one box per tab child that
//     if and hidden do not prune, without their content;
//   - step 2 cascades them and drops the tabs whose display is none, which
//     leaves the visible tabs;
//   - step 3 (activateTabs), for each tabs, outermost first, picks the
//     active tab, drops the other tab boxes, creates one label per visible
//     tab, inflates the active tab's content, and cascades both;
//   - step 4 (resolveFocus) applies the focus rules of §6.10.3;
//   - step 6 (layout) picks the label tier and lays out the strip and the
//     active tab below it.
//
// Without bind the runtime remembers the active tab per tabs id (tabMem);
// the active tab of every tabs of the last frame (tabPrev) tells the next
// frame which tabs changed. Widgets inside inactive tabs keep their state
// because every widget state is kept by id.

// activateTabs is step 3 of SPEC §18: every tabs of the screen and of the
// open modals, outermost first, gets its active tab (activate).
func (fb *builder) activateTabs(casc *css.Cascade, root *layout.Box) {
	var walk func(b *layout.Box)
	walk = func(b *layout.Box) {
		if b.Kind == "tabs" {
			fb.activate(casc, b)
		}
		for _, c := range b.Children {
			walk(c)
		}
	}
	if root != nil {
		walk(root)
	}
	for _, m := range fb.modals {
		walk(m)
	}
}

// activate picks the active tab of tabs t among its visible tabs (its tab
// children after step 2, in document order), drops the other tab boxes
// (and anything else the document put there, which is V016), creates one
// label per visible tab, then the active tab, as t's children, inflates
// the active tab's content in the scope the tab was inflated in, and
// cascades the labels (children of t: tabs > text, .tab-label) and that
// content. A label is a generated text: class tab-label, the tab's id as
// its key, :selected for the active tab, :disabled for a disabled tab.
func (fb *builder) activate(casc *css.Cascade, t *layout.Box) {
	var visible []*layout.Box
	for _, c := range t.Children {
		if c.Kind == "tab" {
			visible = append(visible, c)
		}
	}
	active := fb.pickTab(t, visible)
	t.TabList, t.TabActive = visible, active
	kids := make([]*layout.Box, 0, len(visible)+1)
	for _, v := range visible {
		full, _ := v.Src.Attr("label")
		short, ok := v.Src.Attr("short")
		if !ok || short == "" {
			short = full
		}
		l := &layout.Box{
			Tag: "text", Kind: "text", Role: layout.RoleTabLabel, Parent: t, For: v,
			Classes: []string{"tab-label"}, Key: v.ID, Text: full, Full: full, Short: short,
			Selected: v == active, Disabled: v.Disabled, Fixed: true, Mark: t.Mark,
			Follow: -1, Index: -1,
		}
		l.Style = casc.Compute(l, &t.Style, nil, nil)
		kids = append(kids, l)
		if v != active && fb.byID[v.ID] == v {
			delete(fb.byID, v.ID)
		}
	}
	if active != nil {
		sc := fb.scopes[active]
		for _, c := range active.Src.Children {
			cb := fb.inflate(c, active, sc, false)
			if cb == nil || c.Kind == "modal" {
				continue
			}
			computeStyles(casc, cb, &active.Style)
			if cb.Style.Display != "none" {
				active.Children = append(active.Children, cb)
			}
		}
		kids = append(kids, active)
	}
	t.Children = kids
}

// pickTab returns the active tab of tabs t among its visible tabs (SPEC
// §6.10.2), or nil when none is visible. With bind: the visible tab whose
// id equals the bound value (a string); the first visible tab when the
// value is null, when the path is missing (B003), and for any other value
// (B010, a warning); the store is never written for these fallbacks.
// Without bind: the tab the runtime remembers for this tabs id when it is
// visible, else the first visible tab, which does not change what is
// remembered.
func (fb *builder) pickTab(t *layout.Box, visible []*layout.Box) *layout.Box {
	if len(visible) == 0 {
		return nil
	}
	a, n := fb.a, t.Src
	if n.Bind != "" {
		v, found := fb.tabsScope(t).resolve(a.store, n.Bind)
		if !found {
			fb.missingBind(n, n.Bind)
			return visible[0]
		}
		if v == nil {
			return visible[0]
		}
		if s, ok := v.(string); ok {
			for _, vt := range visible {
				if vt.ID == s {
					return vt
				}
			}
		}
		fb.report(n, ir.Warning, "B010", "bind=%q holds %s, which is not the id of a visible tab of #%s; the first visible tab, #%s, is active", n.Bind, jsonText(v), n.ID, visible[0].ID)
		return visible[0]
	}
	if id, ok := a.tabMem[t.ID]; ok && t.ID != "" {
		for _, vt := range visible {
			if vt.ID == id {
				return vt
			}
		}
	}
	return visible[0]
}

// tabsScope is the scope tabs t was inflated in: the scope of its tabs
// (every tab of t shares it), or the store.
func (fb *builder) tabsScope(t *layout.Box) *scope {
	for _, c := range t.Children {
		if sc, ok := fb.scopes[c]; ok {
			return sc
		}
	}
	return nil
}

// activeTabs maps the id of every tabs in the frame (the screen and the
// open modals) to its active tab's id ("" without one): what the next
// frame compares against (SPEC §6.10.3, "between two consecutive frames").
func (fb *builder) activeTabs(root *layout.Box) map[string]string {
	out := map[string]string{}
	visit := func(r *layout.Box) {
		if r == nil {
			return
		}
		r.Walk(func(b *layout.Box) {
			if b.Kind == "tabs" && b.ID != "" {
				id := ""
				if b.TabActive != nil {
					id = b.TabActive.ID
				}
				out[b.ID] = id
			}
		})
	}
	visit(root)
	for _, m := range fb.modals {
		visit(m)
	}
	return out
}

// tabFocus applies the activation rule of SPEC §6.10.3 to this frame and
// returns the node focus moves to and whether a rule applied. For the
// first tabs, in document order, whose active tab changed from A to B
// since the last frame (a tabs that was not in it has no previous active
// tab), while focus, when this render started, was on the tabs node
// itself, on A or inside it, or nowhere: B's focus= target when it can
// take focus; else the first node of the focus cycle inside B; else the
// §8.3 rule (the screen's focus= target, else the first focusable node).
// list is the frame's focus cycle. Each tabs gets the rule once per
// render. The caller holds a.mu.
func (fb *builder) tabFocus(root *layout.Box, list []*layout.Box) (string, bool) {
	a, rc := fb.a, fb.rc
	var hit *layout.Box
	visit := func(r *layout.Box) {
		if r == nil || hit != nil {
			return
		}
		r.Walk(func(b *layout.Box) {
			if hit != nil || b.Kind != "tabs" || b.ID == "" || b.TabActive == nil || rc.tabDone[b.ID] {
				return
			}
			prev, ok := a.tabPrev[b.ID]
			if !ok || prev == b.TabActive.ID {
				return
			}
			rc.tabDone[b.ID] = true
			if a.focusWasOn(b.ID, prev, rc.focusStart) {
				hit = b
			}
		})
	}
	visit(root)
	for _, m := range fb.modals {
		visit(m)
	}
	if hit == nil {
		return "", false
	}
	in := func(id string) bool {
		for _, c := range list {
			if c.ID == id {
				return true
			}
		}
		return false
	}
	tab := hit.TabActive
	if f, ok := tab.Src.Attr("focus"); ok && len(f) > 1 && in(f[1:]) {
		return f[1:], true
	}
	for _, c := range list {
		if within(c, tab) {
			return c.ID, true
		}
	}
	if sf, ok := a.currentScreenFocus(); ok && len(fb.modals) == 0 && in(sf) {
		return sf, true
	}
	if len(list) > 0 {
		return list[0].ID, true
	}
	return "", true
}

// focusWasOn reports whether focus (the id focused when the render
// started) was, in the last frame, on the tabs node tabs, on its tab
// prevTab or inside it, or nowhere (SPEC §6.10.3). The caller holds a.mu.
func (a *App) focusWasOn(tabs, prevTab, focus string) bool {
	if focus == "" {
		return true
	}
	if focus == tabs {
		return true
	}
	if a.last == nil || prevTab == "" {
		return false
	}
	for b := a.last.ByID[focus]; b != nil; b = b.Parent {
		if b.Kind == "tab" && b.ID == prevTab {
			return true
		}
	}
	return false
}

// within reports whether b is anc or inside it.
func within(b, anc *layout.Box) bool {
	for ; b != nil; b = b.Parent {
		if b == anc {
			return true
		}
	}
	return false
}

// initialTabFocus is the initial focus a tabs supplies (SPEC §6.10.3): the
// focus= target of the active tab of the first tabs of the screen, in
// document order, whose target can take focus (is in list); "" when none.
func initialTabFocus(root *layout.Box, list []*layout.Box) string {
	found := ""
	if root == nil {
		return ""
	}
	root.Walk(func(b *layout.Box) {
		if found != "" || b.Kind != "tabs" || b.TabActive == nil {
			return
		}
		f, ok := b.TabActive.Src.Attr("focus")
		if !ok || len(f) < 2 {
			return
		}
		for _, c := range list {
			if c.ID == f[1:] {
				found = c.ID
				return
			}
		}
	})
	return found
}

// enabledTabs is the enabled visible tabs of tabs t, in document order.
func enabledTabs(t *layout.Box) []*layout.Box {
	var out []*layout.Box
	for _, v := range t.TabList {
		if !v.Disabled {
			out = append(out, v)
		}
	}
	return out
}

// stepTab returns the tab a move-* built-in (or left/right, or the wheel)
// activates on tabs t (SPEC §8.4): the next or previous enabled visible
// tab after the active one, wrapping, or the first or last. nil when no
// visible tab is enabled.
func stepTab(t *layout.Box, action string) *layout.Box {
	en := enabledTabs(t)
	if len(en) == 0 {
		return nil
	}
	switch action {
	case "move-first":
		return en[0]
	case "move-last":
		return en[len(en)-1]
	}
	// A visible tab exists (en is not empty), so one is active.
	cur := 0
	for i, v := range t.TabList {
		if v == t.TabActive {
			cur = i
		}
	}
	n := len(t.TabList)
	dir := 1
	if action == "move-prev" {
		dir = -1
	}
	// The next enabled tab in that direction, wrapping; the active tab
	// itself last (k = n), when it is the only enabled one.
	for k := 1; k <= n; k++ {
		if v := t.TabList[((cur+dir*k)%n+n)%n]; !v.Disabled {
			return v
		}
	}
	return nil
}

// activateTab is a user activation of tab tab of tabs t (SPEC §6.10.2): a
// key, a built-in action, the wheel, or a click. With bind it writes the
// tab's id (a string) to the bound path; without, the runtime remembers
// it. When the active tab changed, t's on:select fires with value = the
// tab's id; the next frame then applies the focus rule of §6.10.3. The
// caller holds a.mu.
func (a *App) activateTab(t, tab *layout.Box) []Event {
	if t == nil || tab == nil || t.Src == nil {
		return nil
	}
	n := t.Src
	if n.Bind != "" {
		if cur, ok := lookup(a.store, n.Bind); !ok || !isString(cur, tab.ID) {
			if root, err := assign(a.store, n.Bind, tab.ID); err == nil {
				a.store = root
				a.wrote(n.Bind)
				a.dirty = true
			}
		}
	} else if a.tabMem[t.ID] != tab.ID {
		a.tabMem[t.ID] = tab.ID
		a.dirty = true
	}
	if t.TabActive == tab {
		return nil
	}
	a.dirty = true
	if act, ok := n.On["select"]; ok {
		return []Event{{Action: act, Source: t.ID, Keys: map[string]any{}, Value: tab.ID}}
	}
	return nil
}

// tabAct is one tab a focus request activated (SPEC §6.10.3), with what it
// changed, so that the request can be undone when it cannot land.
type tabAct struct {
	tabs, tab string // ids
	path      string // the tabs' bind path; "" when the tab is remembered
	old       any    // the bound value before, when hadOld
	hadOld    bool
	oldMem    string // the remembered tab before, when hadMem
	hadMem    bool
	fire      string // the tabs' on:select action, "" without one
}

// openTabsFor activates, outermost first, the inactive tabs that hold the
// node id, for a focus request (SPEC §6.10.3): a tab counts as active when
// its tabs is in the live frame with that tab active, or, for a tabs not
// in the live frame, when its bound value or remembered tab is that tab.
// It returns what it changed. The caller holds a.mu.
func (a *App) openTabsFor(id string) []tabAct {
	n := a.doc.IDs[id]
	var tabs []*ir.Node
	for p := n; p != nil; p = p.Parent {
		if p != n && p.Kind == "tab" && p.Parent != nil && p.Parent.Kind == "tabs" && p.ID != "" && p.Parent.ID != "" {
			tabs = append(tabs, p)
		}
	}
	var out []tabAct
	for i := len(tabs) - 1; i >= 0; i-- {
		tab := tabs[i]
		tn := tab.Parent
		if a.tabActiveNow(tn, tab) {
			continue
		}
		act := tabAct{tabs: tn.ID, tab: tab.ID, path: tn.Bind, fire: tn.On["select"]}
		if tn.Bind != "" {
			act.old, act.hadOld = lookup(a.store, tn.Bind)
			root, err := assign(a.store, tn.Bind, tab.ID)
			if err != nil {
				continue
			}
			a.store = root
			a.wrote(tn.Bind)
		} else {
			act.oldMem, act.hadMem = a.tabMem[tn.ID]
			a.tabMem[tn.ID] = tab.ID
		}
		out = append(out, act)
	}
	return out
}

// tabActiveNow reports whether tab is the active tab of tabs tn now: in
// the live frame when tn is in it, else by its bound value or remembered
// tab. The caller holds a.mu.
func (a *App) tabActiveNow(tn, tab *ir.Node) bool {
	v := a.liveView()
	if b := v.byID[tn.ID]; b != nil && b.Kind == "tabs" && v.inFrame(b) {
		return b.TabActive != nil && b.TabActive.ID == tab.ID
	}
	if tn.Bind != "" {
		cur, ok := lookup(a.store, tn.Bind)
		return ok && isString(cur, tab.ID)
	}
	return a.tabMem[tn.ID] == tab.ID
}

// undoTabActs undoes the activations of a focus request that did not
// land, innermost first (SPEC §6.10.3): each remembered tab is put back,
// and each bound path is written back to its old value only while it
// still holds the id the activation wrote (a value the host set in between
// stays). No on:select fires. It reports whether anything changed. The
// caller holds a.mu.
func (a *App) undoTabActs(acts []tabAct) bool {
	changed := false
	for i := len(acts) - 1; i >= 0; i-- {
		act := acts[i]
		if act.path == "" {
			if act.hadMem {
				changed = changed || a.tabMem[act.tabs] != act.oldMem
				a.tabMem[act.tabs] = act.oldMem
			} else if _, ok := a.tabMem[act.tabs]; ok {
				delete(a.tabMem, act.tabs)
				changed = true
			}
			continue
		}
		cur, ok := lookup(a.store, act.path)
		if !ok || !isString(cur, act.tab) {
			continue
		}
		if act.hadOld {
			if root, err := assign(a.store, act.path, act.old); err == nil {
				a.store = root
			}
		} else {
			unassign(a.store, act.path)
		}
		a.wrote(act.path)
		changed = true
	}
	return changed
}

// unassign removes the last member of path from its parent object, when
// there is one.
func unassign(root any, path string) {
	parent, last := root, path
	if i := strings.LastIndexByte(path, '.'); i >= 0 {
		p, ok := lookup(root, path[:i])
		if !ok {
			return
		}
		parent, last = p, path[i+1:]
	}
	if m, ok := parent.(map[string]any); ok {
		delete(m, last)
	}
}

// tabSelects returns the on:select events of the tabs a landed focus
// request activated, outermost first, each with its tab's id as value
// (SPEC §6.10.3).
func tabSelects(acts []tabAct) []Event {
	var out []Event
	for _, act := range acts {
		if act.fire != "" {
			out = append(out, Event{Action: act.fire, Source: act.tabs, Keys: map[string]any{}, Value: act.tab})
		}
	}
	return out
}

// isString reports whether v is the string s.
func isString(v any, s string) bool {
	x, ok := v.(string)
	return ok && x == s
}
