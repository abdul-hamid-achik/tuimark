package host

import "github.com/abdul-hamid-achik/tuimark/internal/ir"

// usePruned reports whether a <use> that placed n, a top-level element of
// its copy, prunes it: its if is falsy or its hidden is truthy (SPEC v0.4b
// §6.18.3). The uses are tried outermost first, before n's own guards, in
// the scope where n stands, which is the use's. A missing if path is B002
// at the use, once per frame whatever the number of its copy's elements.
//
// F2 evaluates the guard for each top-level element of the copy; F3 makes
// it once per placement and has the list row cache read it (decision 21).
func (fb *builder) usePruned(n *ir.Node, sc *scope) bool {
	for i := len(n.CopiedBy) - 1; i >= 0; i-- {
		u := n.CopiedBy[i]
		if u.If != "" {
			if g, err := ir.ParseGuard(u.If); err == nil {
				v, found := sc.resolve(fb.a.store, g.Path)
				if !found {
					fb.report(u, ir.Warning, "B002", "if=%q: path %q is missing", u.If, g.Path)
				}
				if ir.Truthy(v) == g.Neg {
					return true
				}
			}
		}
		if fb.flag(u, "hidden", sc) {
			return true
		}
	}
	return false
}
