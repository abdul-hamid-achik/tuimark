package host

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/dump"
	"github.com/abdul-hamid-achik/tuimark/internal/layout"
)

// InspectTarget selects the node `tuimark inspect` explains (SPEC v0.2b
// §15.7): the hit node of cell (X, Y) when At is set, else the first
// laid-out node, in document order, with id ID.
type InspectTarget struct {
	At   bool
	X, Y int
	ID   string
}

// Inspection explains one node of a frame (SPEC v0.2b §15.7). Its JSON
// members follow the §15.7 order once the CLI wraps it with cols, rows,
// and ok.
type Inspection struct {
	Box     *layout.Box `json:"-"`
	Node    dump.Node   `json:"node"`
	Path    string      `json:"path"`
	Cell    *dump.Cell  `json:"cell,omitempty"`
	Pseudo  []string    `json:"pseudo"`
	Classes []ClassInfo `json:"classes"`
	Style   []StyleInfo `json:"style"`
}

// ClassInfo is one class the node could have: from "class" (its class
// attribute), "class:NAME" (a guard, with the guard as written), or
// "generated" (a class the runtime gives a generated node). Active tells
// whether the class is on the node now; Used whether some selector of the
// loaded stylesheets or of the built-in sheet names .NAME, under any
// @media.
type ClassInfo struct {
	Name   string `json:"name"`
	From   string `json:"from"`
	Guard  string `json:"guard,omitempty"`
	Active bool   `json:"active"`
	Used   bool   `json:"used"`
}

// RuleInfo is where a declaration comes from: the rule's file ("<ua>" for
// the built-in sheet, the document for attributes and style=""), its
// position, its selector as written (the attribute as written, or
// style=""), its specificity (0 for attributes, 1000 for style=""), and
// its @media condition as written, omitted outside @media.
type RuleInfo struct {
	File        string `json:"file"`
	Line        int    `json:"line"`
	Col         int    `json:"col"`
	Selector    string `json:"selector"`
	Specificity int    `json:"specificity"`
	Media       string `json:"media,omitempty"`
}

// LostDecl is a declaration of a property that matched the node and lost.
type LostDecl struct {
	Value  string   `json:"value"`
	Origin string   `json:"origin"`
	Rule   RuleInfo `json:"rule"`
}

// StyleInfo is one property of the node's computed style: its value (nil
// when it has none), its origin (ua, attribute, author, inline, inherited,
// or initial), the winning rule (nil for inherited and initial), and the
// declarations it overrode, highest priority first.
type StyleInfo struct {
	Prop       string     `json:"prop"`
	Value      *string    `json:"value"`
	Origin     string     `json:"origin"`
	Rule       *RuleInfo  `json:"rule,omitempty"`
	Overridden []LostDecl `json:"overridden"`
}

// InspectError is a usage error of `tuimark inspect`: a cell outside the
// grid or an id that no laid-out node has.
type InspectError struct{ Msg string }

func (e *InspectError) Error() string { return e.Msg }

// Inspect explains one node of frame f, a frame this app rendered
// (SPEC v0.2b §15.7).
func (a *App) Inspect(f *Frame, t InspectTarget) (*Inspection, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	var b *layout.Box
	var cell *dump.Cell
	if t.At {
		if t.X < 0 || t.Y < 0 || t.X >= f.Cols || t.Y >= f.Rows {
			return nil, &InspectError{fmt.Sprintf("--at %d,%d is outside the %dx%d grid", t.X, t.Y, f.Cols, f.Rows)}
		}
		b = HitNode(f, t.X, t.Y)
		c := dump.CellAt(f.Grid, t.X, t.Y)
		cell = &c
	} else {
		b = firstWithID(f, t.ID)
		if b == nil {
			return nil, &InspectError{fmt.Sprintf("--id %s: no laid-out node has that id", t.ID)}
		}
	}
	if b == nil {
		return nil, &InspectError{"the frame has no node to inspect"}
	}
	in := &Inspection{Box: b, Node: dump.NodeOf(b, f.V2), Path: LayoutPath(f, b), Cell: cell}
	in.Pseudo = pseudoOf(b, f.V2)
	in.Classes = a.classInfo(b)
	in.Style = a.styleInfo(f, b)
	return in, nil
}

// HitNode is the hit node of cell (x, y) of f, steps 2 and 3 of the SPEC
// v0.2b §8.5 hit test (without the modal filter of step 1): start at the
// node whose id owns the cell in the frame's cell-owner map, the last such
// laid-out node in document order whose clipped outer rect contains the
// cell (an id inside a list item template repeats), or, for a cell with no
// owner, at the root of the layer that contains it (the top modal, else
// the screen); then descend through the last laid-out child without an id
// whose clipped outer rect contains the cell, while there is one. Inside
// the top modal's outer rect (clipped to the grid) only the top modal and
// the nodes inside it own cells, as focus is trapped there: an owner under
// it, which shows through when the modal does not paint its rect
// (visibility: hidden), counts as none, so the start is the top modal.
func HitNode(f *Frame, x, y int) *layout.Box {
	var top *layout.Box
	if n := len(f.Modals); n > 0 {
		m := f.Modals[n-1]
		grid := layout.Rect{W: f.Cols, H: f.Rows}
		if m.Laid && m.Outer().Intersect(grid).Contains(x, y) {
			top = m
		}
	}
	var s *layout.Box
	if owner := dump.CellAt(f.Grid, x, y).ID; owner != "" {
		find := func(b *layout.Box) {
			if b.Laid && b.ID == owner && b.Clip.Contains(x, y) {
				s = b
			}
		}
		if top != nil {
			top.Walk(find)
		} else {
			eachLaid(f, find)
		}
	}
	if s == nil {
		s = f.Root
		if top != nil {
			s = top
		}
	}
	for s != nil {
		var next *layout.Box
		for _, c := range s.Children {
			if c.ID == "" && c.Laid && c.Clip.Contains(x, y) {
				next = c
			}
		}
		if next == nil {
			break
		}
		s = next
	}
	return s
}

// eachLaid visits every laid-out node of f in document order: the screen
// tree, then the modals.
func eachLaid(f *Frame, fn func(*layout.Box)) {
	visit := func(root *layout.Box) {
		if root == nil {
			return
		}
		root.Walk(func(b *layout.Box) {
			if b.Laid {
				fn(b)
			}
		})
	}
	visit(f.Root)
	for _, m := range f.Modals {
		visit(m)
	}
}

// firstWithID is the first laid-out node of f, in document order, with id.
func firstWithID(f *Frame, id string) *layout.Box {
	var found *layout.Box
	eachLaid(f, func(b *layout.Box) {
		if found == nil && id != "" && b.ID == id {
			found = b
		}
	})
	return found
}

// LayoutPath is a node's layout path (SPEC v0.2b §15.7): "/" and one
// segment per node from the root of its layer (the screen, or app for the
// spike; a modal's path goes through its screen) down to the node. A
// segment is the tag, then #id when the node has one; a node without id
// whose parent has more than one laid-out child of its tag also gets [k],
// its 0-based index among them (a modal is counted among the modals).
func LayoutPath(f *Frame, b *layout.Box) string {
	var segs []string
	for n := b; n != nil; n = n.Parent {
		seg := n.Tag
		switch {
		case n.ID != "":
			seg += "#" + n.ID
		case n.Parent != nil:
			siblings := n.Parent.Children
			if n.Modal {
				siblings = f.Modals
			}
			k, count := 0, 0
			for _, s := range siblings {
				if !s.Laid || s.Tag != n.Tag {
					continue
				}
				if s == n {
					k = count
				}
				count++
			}
			if count > 1 {
				seg += "[" + strconv.Itoa(k) + "]"
			}
		}
		segs = append(segs, seg)
	}
	for i, j := 0, len(segs)-1; i < j; i, j = i+1, j-1 {
		segs[i], segs[j] = segs[j], segs[i]
	}
	return "/" + strings.Join(segs, "/")
}

// pseudoOf lists the pseudo-classes that match b, in the order focus,
// focus-within, selected, checked, disabled, empty. A version="1"
// document has no :focus-within or :checked (SPEC §5.1), so they are
// listed only for version="2" documents.
func pseudoOf(b *layout.Box, v2 bool) []string {
	out := []string{}
	for _, p := range []string{"focus", "focus-within", "selected", "checked", "disabled", "empty"} {
		if (p == "focus-within" || p == "checked") && !v2 {
			continue
		}
		if b.HasPseudo(p) {
			out = append(out, p)
		}
	}
	return out
}

// classInfo lists every class b could have: its class names, then its
// class:NAME guards in attribute order, then (for a generated node) the
// classes the runtime gave it. The caller holds a.mu.
func (a *App) classInfo(b *layout.Box) []ClassInfo {
	sheets := append(append([]*css.Sheet(nil), a.sheets...), css.UA())
	used := func(name string) bool { return css.NamesClass(sheets, name) }
	out := []ClassInfo{}
	if b.Src == nil {
		for _, c := range b.Classes {
			out = append(out, ClassInfo{Name: c, From: "generated", Active: true, Used: used(c)})
		}
		return out
	}
	for _, c := range b.Src.Classes {
		out = append(out, ClassInfo{Name: c, From: "class", Active: true, Used: used(c)})
	}
	for i, g := range b.Src.ClassGuards {
		on := i < len(b.GuardOn) && b.GuardOn[i]
		out = append(out, ClassInfo{Name: g.Name, From: "class:" + g.Name, Guard: g.Guard, Active: on, Used: used(g.Name)})
	}
	return out
}

// originNames are the §15.7 origin names of the cascade's origins.
var originNames = map[int]string{css.OriginUA: "ua", css.OriginHint: "attribute", css.OriginAuthor: "author", css.OriginInline: "inline"}

func ruleOf(d css.DeclSource) RuleInfo {
	return RuleInfo{File: d.File, Line: d.Line, Col: d.Col, Selector: d.Selector, Specificity: d.Specificity, Media: d.Media}
}

// styleInfo explains every TCSS property of b, in the order of SPEC §10.2
// then §10.3, re-running the frame's cascade on it. The caller holds a.mu.
func (a *App) styleInfo(f *Frame, b *layout.Box) []StyleInfo {
	casc := css.NewCascade(a.sheets, css.Env{Cols: f.Cols, Rows: f.Rows, Theme: f.Theme})
	var parent *css.Style
	if b.Parent != nil {
		parent = &b.Parent.Style
	}
	_, trace := casc.Explain(subjectOf(b), parent, b.Hints, b.Inline)
	out := make([]StyleInfo, 0, len(css.PropertyOrder))
	for _, prop := range css.PropertyOrder {
		si := StyleInfo{Prop: prop, Origin: "initial", Overridden: []LostDecl{}}
		if v, ok := css.FormatProp(b.Style, prop); ok {
			si.Value = &v
		}
		if pt := trace[prop]; pt != nil {
			switch {
			case pt.Winner != nil:
				si.Origin = originNames[pt.Winner.Origin]
				r := ruleOf(*pt.Winner)
				si.Rule = &r
				for _, l := range pt.Losers {
					si.Overridden = append(si.Overridden, LostDecl{Value: l.Value, Origin: originNames[l.Origin], Rule: ruleOf(l)})
				}
			case pt.Inherited:
				si.Origin = "inherited"
			}
		}
		out = append(out, si)
	}
	return out
}
