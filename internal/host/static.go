package host

import (
	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/layout"
)

// staticEl is an ir.Node seen by the cascade without any runtime state: no
// data, no inflation, no pseudo-class (:focus, :selected, :disabled, :empty
// never match).
type staticEl struct {
	n      *ir.Node
	parent *staticEl
}

func (s *staticEl) MatchTag() string { return s.n.Kind }
func (s *staticEl) MatchID() string  { return s.n.ID }
func (s *staticEl) HasPseudo(string) bool {
	return false
}

func (s *staticEl) HasClass(c string) bool {
	for _, x := range s.n.Classes {
		if x == c {
			return true
		}
	}
	return false
}

func (s *staticEl) ParentElement() css.Element {
	if s.parent == nil {
		return nil
	}
	return s.parent
}

// checkDocks reports V010 (a parse-pass code, SPEC §14) for every node of
// the document, whatever the data and whichever screen is active: other
// screens, closed modals, false if= branches, hidden nodes, and list item
// templates (not expanded). Each node's style comes from its presentational
// hints, style="", and the author rules without state pseudo-classes, at
// every validate breakpoint and under each of themes (the themes Validate
// renders). Layout keeps its own V010 as a fallback for what only a render
// can decide; Validate dedupes the two by String(). Modals are layers,
// never docked, so they are skipped like at layout time. Caller holds a.mu.
func (a *App) checkDocks(themes []string) ir.Diags {
	var out ir.Diags
	if a.doc.Root == nil {
		return out
	}
	seen := map[string]bool{}
	for _, env := range dockEnvs(themes) {
		casc := css.NewCascade(a.sheets, env)
		var walk func(n *ir.Node, parent *staticEl, ps *css.Style)
		walk = func(n *ir.Node, parent *staticEl, ps *css.Style) {
			switch n.Kind {
			case "tui":
				for _, c := range n.Children {
					walk(c, nil, nil)
				}
				return
			case "style", "keymap", "bind":
				return
			}
			el := &staticEl{n: n, parent: parent}
			st := casc.Compute(el, ps, a.toDecls(n.Hints), a.toDecls(n.Inline))
			// A table's columns and row template ignore dock (SPEC
			// §6.9.3), and so does a tab (§6.10.4): their widget
			// places them.
			if parent != nil && n.Kind != "modal" && st.Dock != "" && parent.n.Kind != "table" && parent.n.Kind != "tabs" {
				row := (&layout.Box{Kind: parent.n.Kind, Style: *ps}).Direction() == "row"
				if layout.DockConflict(st, row) {
					d := ir.At(n, a.file, ir.Error, "V010", "%s", layout.DockConflictMsg(st.Dock))
					if !seen[d.String()] {
						seen[d.String()] = true
						out = append(out, d)
					}
				}
			}
			for _, c := range n.Children {
				walk(c, el, &st)
			}
		}
		walk(a.doc.Root, nil, nil)
	}
	return out
}

// dockEnvs are the validate breakpoints (× 24 rows) under each theme.
func dockEnvs(themes []string) []css.Env {
	var out []css.Env
	for _, th := range themes {
		for _, cols := range validateCols {
			out = append(out, css.Env{Cols: cols, Rows: 24, Theme: th})
		}
	}
	return out
}
