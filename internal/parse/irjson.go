package parse

import (
	"strings"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// IR is the source IR document printed by `tuimark ir` (SPEC §13.1): the
// parsed-and-cascaded shape agents read to understand what a document
// means before layout ever runs.
type IR struct {
	Version     string            `json:"version"`
	App         IRApp             `json:"app"`
	Stylesheets []string          `json:"stylesheets"`
	Tokens      map[string]string `json:"tokens"`
	Keymap      []IRBind          `json:"keymap"`
	Root        *IRNode           `json:"root"`
}

// IRApp is the document's top-level identity (SPEC §13.1's "app" object).
// Title and Focus come from the main (first) <screen>; v1-only. Mouse is
// the version="2" mouse attribute as written, omitted when absent (IR 0.2).
type IRApp struct {
	Title string `json:"title"`
	Theme string `json:"theme"`
	Focus string `json:"focus"`
	Mouse string `json:"mouse,omitempty"`
}

// IRBind is one <keymap><bind> row. Label and Keycap (IR 0.2) are
// present exactly when the row writes them.
type IRBind struct {
	Keys   string  `json:"keys"`
	Action string  `json:"action"`
	When   string  `json:"when,omitempty"`
	To     string  `json:"to,omitempty"`
	Label  *string `json:"label,omitempty"`
	Keycap *string `json:"keycap,omitempty"`
}

// IRNode is one element of the source IR tree.
//
// Attrs holds every attribute *not* already surfaced as a structured field:
// id, class, bind, each, if, style, and the presentational hints (width,
// height, gap, pad, border, wrap) live in ID/Classes/Bind/Each/If/Style
// instead, so Attrs never repeats them. pad is folded into Style as
// "padding"; border="1"/"0" are folded in as "single"/"none" (build.go
// already normalizes both before the node reaches here).
type IRNode struct {
	ID       string            `json:"id,omitempty"`
	Kind     string            `json:"kind"`
	Classes  []string          `json:"classes"`
	Attrs    map[string]string `json:"attrs"`
	Bind     string            `json:"bind"`
	Each     string            `json:"each"`
	If       string            `json:"if"`
	On       map[string]string `json:"on"`
	Style    map[string]string `json:"style"`
	Text     string            `json:"text"`
	Children []*IRNode         `json:"children"`
}

// structuralAttrs are raw attributes represented by a structured IRNode
// field instead of Attrs. on:* attributes are excluded by prefix, below.
var structuralAttrs = map[string]bool{
	"id": true, "class": true, "bind": true, "each": true, "if": true,
	"style": true, "width": true, "height": true, "gap": true, "pad": true,
	"border": true, "wrap": true,
}

// BuildIR converts a parsed Document into the source IR (SPEC §13.1):
// version "0.1" for a version="1" document and the spike, "0.2" for a
// version="2" document (IR 0.2 adds the six kinds, app.mouse, and the
// keymap's label and keycap; class:NAME stays in attrs, §13.1), and "0.3"
// for a version="3" document (IR 0.3 has exactly the shape of IR 0.2 in
// 0.3a, SPEC v0.3 §23.4).
//
// tokens is the resolved theme token table: the built-in theme's tokens
// overridden by any :root custom properties in the document's stylesheets
// (media conditions ignored). internal/parse cannot load style src= files
// itself (that needs a directory to resolve against, which is a host
// concern), so callers that read from disk — cmd/tuimark's `ir` command,
// internal/host — compute tokens and pass them in.
func BuildIR(doc *Document, tokens map[string]string) *IR {
	version := ir.Version
	switch {
	case doc.V3:
		version = ir.VersionV3
	case doc.V2:
		version = ir.VersionV2
	}
	out := &IR{
		Version:     version,
		App:         IRApp{Theme: doc.Theme, Mouse: doc.Mouse},
		Stylesheets: stylesheetPaths(doc.Styles),
		Tokens:      copyTokens(tokens),
		Keymap:      buildKeymap(doc.Keymap),
	}
	if len(doc.Screens) > 0 {
		s := doc.Screens[0]
		out.App.Title, _ = s.Attr("title")
		out.App.Focus, _ = s.Attr("focus")
	}
	if doc.Root != nil {
		out.Root = buildIRNode(doc.Root)
	}
	return out
}

func stylesheetPaths(styles []StyleRef) []string {
	out := []string{}
	for _, s := range styles {
		if s.Src != "" {
			out = append(out, s.Src)
		}
	}
	return out
}

func copyTokens(tokens map[string]string) map[string]string {
	out := make(map[string]string, len(tokens))
	for k, v := range tokens {
		out[k] = v
	}
	return out
}

func buildKeymap(kb []KeyBind) []IRBind {
	out := []IRBind{}
	for _, k := range kb {
		// The top-level keymap array carries each row's effective when
		// (its own when= when it wrote one, else its <keymap>'s, SPEC
		// v0.3b §8.1, §13.1); the root tree keeps the keymap node and its
		// bind nodes with their own when= in attrs, as written.
		e := IRBind{Keys: k.KeysRaw, Action: k.Action, When: k.EffectiveWhen}
		if k.To != "" {
			e.To = "#" + k.To
		}
		if k.HasLabel {
			l := k.Label
			e.Label = &l
		}
		if k.HasKeycap {
			c := k.Keycap
			e.Keycap = &c
		}
		out = append(out, e)
	}
	return out
}

func buildIRNode(n *ir.Node) *IRNode {
	classes := n.Classes
	if classes == nil {
		classes = []string{}
	}
	on := n.On
	if on == nil {
		on = map[string]string{}
	}
	children := make([]*IRNode, 0, len(n.Children))
	for _, c := range n.Children {
		children = append(children, buildIRNode(c))
	}
	return &IRNode{
		ID: n.ID, Kind: n.Kind, Classes: classes, Attrs: nodeAttrs(n),
		Bind: n.Bind, Each: n.Each, If: n.If, On: on,
		Style: nodeStyle(n), Text: n.Text, Children: children,
	}
}

// nodeAttrs is every attribute as written, minus the ones already
// represented by a structured field (see structuralAttrs) and on:* events.
func nodeAttrs(n *ir.Node) map[string]string {
	out := map[string]string{}
	for k, v := range n.Attrs {
		if structuralAttrs[k] || strings.HasPrefix(k, "on:") {
			continue
		}
		out[k] = v
	}
	return out
}

// nodeStyle merges presentational attributes (already normalized: pad ->
// padding, border 1/0 -> single/none) with style="" declarations, which
// take precedence, matching the cascade's presentation-hints-then-inline
// precedence.
func nodeStyle(n *ir.Node) map[string]string {
	out := map[string]string{}
	for _, p := range n.Hints {
		out[p.Name] = p.Value
	}
	for _, p := range n.Inline {
		out[p.Name] = p.Value
	}
	return out
}
