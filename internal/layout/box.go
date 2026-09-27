// Package layout is Tuimark's own layout engine (SPEC §11): integer cells,
// one allocation pass per container, border included in the outer rect.
//
// The host builds a Box tree (bindings already inflated, styles computed);
// Layout assigns every box an outer rectangle. Nothing here knows about
// JSON, events, or terminals.
package layout

import (
	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// Rect is an integer cell rectangle.
type Rect struct{ X, Y, W, H int }

// Intersect returns the overlap of two rectangles (possibly empty).
func (r Rect) Intersect(o Rect) Rect {
	x0, y0 := max(r.X, o.X), max(r.Y, o.Y)
	x1, y1 := min(r.X+r.W, o.X+o.W), min(r.Y+r.H, o.Y+o.H)
	if x1 < x0 {
		x1 = x0
	}
	if y1 < y0 {
		y1 = y0
	}
	return Rect{x0, y0, x1 - x0, y1 - y0}
}

// Contains reports whether the cell (x, y) is inside r.
func (r Rect) Contains(x, y int) bool {
	return x >= r.X && y >= r.Y && x < r.X+r.W && y < r.Y+r.H
}

// Box is one laid-out node.
type Box struct {
	// Identity and matching.
	Tag     string // source tag (app, col, box, item, ...)
	Kind    string // IR kind
	ID      string
	Classes []string
	// GuardOn is the truthiness of each class:NAME guard of Src, in
	// attribute order, as this frame evaluated it (SPEC §6.13); nil when
	// Src has no guards.
	GuardOn []bool
	Parent  *Box
	Src     *ir.Node

	// Pseudo-class state. FocusWithin is set on every node of the focus
	// chain (the focused node and its ancestors up to the screen, through a
	// modal) and Checked on a checked list or table row; only version="2"
	// selectors can name :focus-within and :checked (SPEC §10.1).
	Focused, Selected, Disabled bool
	FocusWithin, Checked        bool

	// Computed style (set by the host before Layout).
	Style css.Style
	// Hints and Inline are the node's attribute/style="" declarations.
	Hints, Inline []css.Decl

	// Content (resolved by the host).
	Text        string // text body, button label, or input value
	Placeholder string
	Secret      bool
	Cursor      int     // input cursor (rune index)
	Value       float64 // progress 0-100
	Axis        string  // rule/scroll axis
	Title       string
	Key         string // list item key, or the element key of a container each template child (dump; event payloads)
	Index       int    // list item index, or element index of a container each template child

	// Chan is the mark channel of a row of a list with checked and mark
	// (SPEC §6.14): the columns just inside the row's left border and
	// padding, before its content box, reserved whether or not the row is
	// checked. Mark is painted in the first width(Mark) of them, on the
	// first content row, when the row is Checked.
	Chan int
	Mark string

	// Scroll state: offset in, clamped offset and content size out.
	ScrollX, ScrollY int
	Follow           int // list: index of the item to keep visible (-1 none)
	ContentW         int
	ContentH         int

	Children []*Box

	// Geometry (outputs).
	X, Y, W, H int
	Content    Rect // inside border and padding
	Clip       Rect // visible area for this box's own painting
	Laid       bool
	Modal      bool

	autoW, autoH bool // size on this axis came from content (for L002)
}

// Outer returns the outer rectangle.
func (b *Box) Outer() Rect { return Rect{b.X, b.Y, b.W, b.H} }

// css.Element implementation.

// MatchTag returns the type used by type selectors (the IR kind, so <app>
// matches as col).
func (b *Box) MatchTag() string { return b.Kind }

// MatchID returns the id.
func (b *Box) MatchID() string { return b.ID }

// HasClass reports class membership.
func (b *Box) HasClass(c string) bool {
	for _, x := range b.Classes {
		if x == c {
			return true
		}
	}
	return false
}

// HasPseudo reports pseudo-class state.
func (b *Box) HasPseudo(p string) bool {
	switch p {
	case "focus":
		return b.Focused
	case "selected":
		return b.Selected
	case "disabled":
		return b.Disabled
	case "empty":
		return len(b.Children) == 0 && b.Text == ""
	case "focus-within":
		return b.FocusWithin
	case "checked":
		return b.Checked
	}
	return false
}

// ParentElement returns the parent for child-combinator matching.
func (b *Box) ParentElement() css.Element {
	if b.Parent == nil {
		return nil
	}
	return b.Parent
}

// Walk visits b and its descendants in document order.
func (b *Box) Walk(fn func(*Box)) {
	fn(b)
	for _, c := range b.Children {
		c.Walk(fn)
	}
}

func (b *Box) border() int {
	if b.Style.Border != "" && b.Style.Border != "none" {
		return 1
	}
	return 0
}

// frame returns border+padding on the horizontal and vertical axes.
func (b *Box) frameH() int { return 2*b.border() + b.pad(1) + b.pad(3) + b.Chan }
func (b *Box) frameV() int { return 2*b.border() + b.pad(0) + b.pad(2) }

// pad returns one padding side (top, right, bottom, left), bounded.
func (b *Box) pad(side int) int { return clampCells(b.Style.Pad[side]) }

// IsContainer reports whether the kind lays out children.
func (b *Box) IsContainer() bool {
	switch b.Kind {
	case "col", "row", "box", "scroll", "list", "item", "screen", "modal":
		return true
	}
	return false
}

// Scrolls reports whether b is a viewport: <scroll>, <list>, or a container
// with overflow: scroll (which scrolls like a <scroll> on the y axis; axis=
// is allowed only on <scroll> and <rule>, SPEC v0.2 §11.4).
func (b *Box) Scrolls() bool {
	return b.Kind == "scroll" || b.Kind == "list" || (b.Style.Overflow == "scroll" && b.IsContainer())
}

// Direction returns "row" or "column".
func (b *Box) Direction() string {
	if b.Style.Layout == "row" || b.Style.Layout == "column" {
		return b.Style.Layout
	}
	if b.Kind == "row" {
		return "row"
	}
	return "column"
}
