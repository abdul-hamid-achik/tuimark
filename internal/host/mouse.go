package host

import (
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/layout"
)

// The mouse of SPEC v0.2b §8.5 and §26.11: <tui mouse="…"> turns it on,
// evaluated on every frame; only the left button and the wheel act. Events
// are resolved against the live frame (the frame on the screen) with the
// hit test of §8.5; a left press followed by a left release over the same
// hit identity is a click, which acts on the release; a wheel report acts
// at once.

// MouseKind is the kind of a decoded mouse event (SPEC v0.2b §26.8).
type MouseKind uint8

// The mouse events the decoder produces.
const (
	MousePress     MouseKind = iota + 1 // left button pressed
	MouseRelease                        // left button released
	MouseWheelUp                        // wheel up
	MouseWheelDown                      // wheel down
)

// String is the kind as TUIMARK_LOG writes it (SPEC v0.2b §26.12).
func (k MouseKind) String() string {
	switch k {
	case MousePress:
		return "press"
	case MouseRelease:
		return "release"
	case MouseWheelUp:
		return "wheel-up"
	case MouseWheelDown:
		return "wheel-down"
	}
	return ""
}

// Mouse is one mouse event at the 0-based cell (X, Y).
type Mouse struct {
	Kind MouseKind
	X, Y int
}

// hitIdentity is the hit identity of a mouse event (SPEC v0.2b §8.5 step
// 4): its layer (the source node of the screen, or of the modal) and the
// steps from that layer's root down to the hit node. Boxes are rebuilt
// every frame, so the identity is this path, never a box.
type hitIdentity struct {
	layer *ir.Node
	path  []hitStep
}

// hitStep is one step of a hit identity's path: a node's child index in
// the laid-out tree, except that a row of a list or a table is its row,
// its index in the bound array and its key. A table lays out only its
// visible rows, so a child index there names a viewport slot, which
// another row takes when the offset moves; the key tells rows apart when
// the host reorders the array between a press and a release.
type hitStep struct {
	index int
	key   string
	row   bool
}

func (h *hitIdentity) equal(o *hitIdentity) bool {
	if h == nil || o == nil || h.layer != o.layer || len(h.path) != len(o.path) {
		return false
	}
	for i := range h.path {
		if h.path[i] != o.path[i] {
			return false
		}
	}
	return true
}

// mouseOn evaluates the document's mouse attribute for this frame (SPEC
// v0.2b §8.5): a flag, false by default; a missing path is B002 and its
// value counts as null, so mouse="path" is off and mouse="!path" is on.
// Only a version="2" document can have it.
func (fb *builder) mouseOn() bool {
	d := fb.a.doc
	if !d.V2 || d.Mouse == "" || d.Root == nil {
		return false
	}
	return fb.flag(d.Root, "mouse", nil)
}

// MouseOn reports whether the live frame has the mouse on: what Run
// compares with the terminal's mouse modes (SPEC v0.2b §26.11).
func (a *App) MouseOn() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.last != nil && a.last.Mouse
}

// DropPress forgets a pending left press: a resize drops it (SPEC v0.2b
// §26.11).
func (a *App) DropPress() {
	a.mu.Lock()
	a.press = nil
	a.mu.Unlock()
}

// KeyRun splits the key run off the front of ins, whose first input is a
// key: the keys up to the next paste, or up to the next mouse event while
// the live frame has the mouse on, and the inputs after the run. While the
// mouse is off, a mouse event is dropped (SPEC v0.2b §8.5, §26.11), so it
// does not end the run either: the printable keys of one read stay one
// edit with one on:change (§8.6, §15.4, the v0.1 coalescing), as they
// were in 0.2a. A dropped event forgets a pending press, as HandleMouse
// does. The mouse is read once, when the run starts. Run's loop and
// play's text: step (and so every caller that walks decoded input) use
// it.
func (a *App) KeyRun(ins []Input) (keys []Key, rest []Input) {
	on := a.MouseOn()
	dropped := false
	n := 0
	for ; n < len(ins); n++ {
		in := ins[n]
		if in.IsPaste || (in.IsMouse && on) {
			break
		}
		if in.IsMouse {
			dropped = true
			continue
		}
		keys = append(keys, in.Key)
	}
	if dropped {
		a.DropPress()
	}
	return keys, ins[n:]
}

// HandleMouse applies one mouse event to the live frame (SPEC v0.2b §8.5)
// and returns the events to dispatch, in order. on reports whether the
// live frame had the mouse on: while it is off every event is dropped
// (including those still in flight after the modes were turned off), and
// so are events outside the grid and, while a modal is open, events
// outside the top modal. A left press records its hit identity; the next
// left release ends it, and clicks when both identities match. A wheel
// report acts at once. The caller must not hold a.mu.
func (a *App) HandleMouse(m Mouse) (evs []Event, on bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	f := a.last
	if f == nil || !f.Mouse {
		a.press = nil
		return nil, false
	}
	h, root, id := a.hit(f, m.X, m.Y)
	switch m.Kind {
	case MousePress:
		// Another left press replaces a pending one; an ignored press
		// leaves none.
		a.press = id
		return nil, true
	case MouseRelease:
		// A release ends the pending press either way.
		p := a.press
		a.press = nil
		if h == nil || !p.equal(id) {
			return nil, true
		}
		return a.click(f, h, root), true
	case MouseWheelUp, MouseWheelDown:
		if h == nil {
			return nil, true
		}
		dir := 1
		if m.Kind == MouseWheelUp {
			dir = -1
		}
		return a.wheel(h, root, m.X, m.Y, dir), true
	}
	return nil, true
}

// hit runs the hit test of SPEC v0.2b §8.5 on f for cell (x, y): nil when
// the cell is outside the grid, or outside the top modal while one is open
// (focus is trapped there); else the hit node (HitNode, steps 2 and 3),
// the root of its layer (the screen, or the modal), and the hit identity
// (step 4). The caller holds a.mu.
func (a *App) hit(f *Frame, x, y int) (h, root *layout.Box, id *hitIdentity) {
	grid := layout.Rect{W: f.Cols, H: f.Rows}
	if !grid.Contains(x, y) {
		return nil, nil, nil
	}
	if n := len(f.Modals); n > 0 {
		top := f.Modals[n-1]
		if !top.Laid || !top.Outer().Intersect(grid).Contains(x, y) {
			return nil, nil, nil
		}
	}
	h = HitNode(f, x, y)
	if h == nil {
		return nil, nil, nil
	}
	var path []hitStep
	root = h
	for b := h; ; b = b.Parent {
		root = b
		if b.Modal || b.Parent == nil {
			break
		}
		if rowWidget(b) != nil {
			path = append(path, hitStep{index: b.Index, key: b.Key, row: true})
			continue
		}
		k := -1
		for i, c := range b.Parent.Children {
			if c == b {
				k = i
				break
			}
		}
		path = append(path, hitStep{index: k})
	}
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return h, root, &hitIdentity{layer: root.Src, path: path}
}

// stopsWalk is rule 0 of the click and wheel walks (SPEC v0.2b §8.5): a
// node that is disabled or has visibility: hidden stops the walk, and
// nothing happens.
func stopsWalk(b *layout.Box) bool {
	return b.Disabled || b.Style.Visibility == "hidden"
}

// inCycle reports whether b is in f's focus cycle (which respects the
// modal trap).
func inCycle(f *Frame, b *layout.Box) bool {
	if b.ID == "" {
		return false
	}
	for _, c := range f.Focusables {
		if c == b {
			return true
		}
	}
	return false
}

// rowWidget returns the list or table b is a row of, or nil.
func rowWidget(b *layout.Box) *layout.Box {
	if b.Kind != "item" || b.Parent == nil {
		return nil
	}
	if p := b.Parent; p.Kind == "list" || p.Kind == "table" {
		return p
	}
	return nil
}

// click applies a click whose press and release had the same hit identity
// (SPEC v0.2b §8.5): walking up from the hit node h to its layer's root,
// inclusive, the first rule that applies decides, and the walk stops
// there:
//
//  0. a disabled or visibility: hidden node: nothing happens;
//  1. a tab label: a user activation of its tab;
//  2. a row of a list or a table: the widget takes focus when it is in the
//     focus cycle and not focused (its on:focus), then its cursor moves to
//     that row as a user move (on:select when it changed);
//  3. a column with on:click: that action fires; focus does not move;
//  4. a node with on:click: it takes focus when it is in the focus cycle
//     and not focused (on:focus), then its on:click fires;
//  5. a node in the focus cycle: focus moves to it (on:focus), or stays.
//
// The caller holds a.mu.
func (a *App) click(f *Frame, h, root *layout.Box) []Event {
	for b := h; b != nil; b = b.Parent {
		if stopsWalk(b) {
			return nil
		}
		switch {
		case b.Role == layout.RoleTabLabel:
			return a.activateTab(b.Parent, b.For)
		case rowWidget(b) != nil:
			w := rowWidget(b)
			var evs []Event
			if inCycle(f, w) && a.focus != w.ID {
				evs = append(evs, a.focusTo(w.ID)...)
			}
			return append(evs, a.moveList(w, func(_, _, _ int) int { return b.Index })...)
		case b.Kind == "column":
			if act, ok := b.Src.On["click"]; ok {
				return []Event{{Action: act, Source: b.ID, Keys: map[string]any{}}}
			}
		case ownClick(b) != "":
			var evs []Event
			if inCycle(f, b) && a.focus != b.ID {
				evs = append(evs, a.focusTo(b.ID)...)
			}
			return append(evs, Event{Action: ownClick(b), Source: b.ID, Keys: map[string]any{}})
		case inCycle(f, b):
			if a.focus != b.ID {
				return a.focusTo(b.ID)
			}
			return nil
		}
		if b == root {
			break
		}
	}
	return nil
}

// wheel applies one wheel report, dir −1 for up and +1 for down, at cell
// (x, y) (SPEC v0.2b §8.5): walking up from the hit node h as a click does
// (rule 0 applies), the first of these decides:
//
//  1. a tab label, or the tabs node itself when the cell is on its strip
//     row: move-prev (up) or move-next (down) on that tabs;
//  2. a list or a table with at least one row, or a row of one: the
//     cursor moves by dir as a user move, clamped, with on:select when it
//     changed; focus does not move;
//  3. a viewport other than a list or a table, with an id or without one:
//     its offset moves by one row on y, or on x when it scrolls only on x,
//     clamped.
//
// The caller holds a.mu.
func (a *App) wheel(h, root *layout.Box, x, y, dir int) []Event {
	move := "move-next"
	if dir < 0 {
		move = "move-prev"
	}
	for b := h; b != nil; b = b.Parent {
		if stopsWalk(b) {
			return nil
		}
		switch {
		case b.Role == layout.RoleTabLabel:
			return a.activateTab(b.Parent, stepTab(b.Parent, move))
		case b.Kind == "tabs" && onStrip(b, x, y):
			return a.activateTab(b, stepTab(b, move))
		case rowWidget(b) != nil && a.rows(rowWidget(b)) > 0:
			return a.moveList(rowWidget(b), func(i, _, _ int) int { return i + dir })
		case (b.Kind == "list" || b.Kind == "table") && a.rows(b) > 0:
			return a.moveList(b, func(i, _, _ int) int { return i + dir })
		case b.Scrolls() && b.Kind != "list" && b.Kind != "table":
			// With an id or without one: a viewport without one keeps the
			// offset the wheel gives it under its element (offsetKey).
			a.moveViewport(b, move)
			return nil
		}
		if b == root {
			break
		}
	}
	return nil
}

// ownClick is the on:click action of b's own element, "" without one. A
// table body cell carries its column as its source node, but the
// column's on:click belongs to the header cell (rule 3), never to the
// cells: only a node whose source is an element of its own kind has one.
func ownClick(b *layout.Box) string {
	if b.Src == nil || b.Src.Kind != b.Kind || b.Kind == "column" {
		return ""
	}
	return b.Src.On["click"]
}

// onStrip reports whether cell (x, y) lies on the strip row of tabs t: the
// first row of its content box (SPEC v0.2b §6.10.4), not its border.
func onStrip(t *layout.Box, x, y int) bool {
	c := t.Content
	return y == c.Y && x >= c.X && x < c.X+c.W
}
