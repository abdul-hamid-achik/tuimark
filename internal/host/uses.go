package host

import "github.com/abdul-hamid-achik/tuimark/internal/ir"

// placement is one place a <use> stands in a frame: the use, and the
// scope its copy's top-level elements are inflated in (the use's own: a
// list row's, an each element's, or the document's). Within one pass of
// a builder, inflation meets each (use, scope) pair at one place only.
type placement struct {
	use *ir.Node
	sc  *scope
}

// usePruned reports whether a <use> that placed n, a top-level element of
// its copy, prunes it: its if is falsy or its hidden is truthy (SPEC v0.4b
// §6.18.3). The uses are tried outermost first, before n's own guards, in
// the scope where n stands, which is the use's.
//
// Each use is checked once per placement (decision 107): the first of its
// copy's top-level elements that inflation meets in a scope checks it
// (placeUse), and the others reuse what it decided. Its guard is never
// pushed onto the elements, so a missing path is one B002 at the use, and
// a list row's cache reads the guard's paths and the with paths as it
// reads the row's own (listPlan).
func (fb *builder) usePruned(n *ir.Node, sc *scope) bool {
	for i := len(n.CopiedBy) - 1; i >= 0; i-- {
		u := n.CopiedBy[i]
		k := placement{u, sc}
		pruned, seen := fb.placed[k]
		if !seen {
			pruned = fb.placeUse(u, sc)
			if fb.placed == nil {
				fb.placed = map[placement]bool{}
			}
			fb.placed[k] = pruned
		}
		if pruned {
			return true
		}
	}
	return false
}

// placeUse checks the use u where it stands, in sc, and reports whether
// it prunes its copy. Its if and hidden come first (a missing if path is
// B002 at the use); a pruned copy is in the frame nowhere, so nothing
// else of it is checked. Then each with clause whose path does not
// resolve is one B003 at the use (SPEC v0.4b §6.18.5, decision 14), an
// error under --strict: `with path "P" (alias x) is missing`. The paths
// of the copy that go through that alias report nothing of their own
// (pathMiss). A diagnostic about a use names no placement, as one the
// parser reports about it does: a use in a copy says where its paths
// come from with ` (P via <use …>)` instead, and one whose paths are the
// store's says the same thing in every copy, which is reported once.
//
// What it reports about one use is the same text at every place the use
// stands (its paths, as written and as the aliases make them, are the
// use's), so it is formatted once per pass (useSaid): a use in every row
// of a long list costs one message, whatever its paths' length.
func (fb *builder) placeUse(u *ir.Node, sc *scope) bool {
	fb.a.placeChecks++
	if fb.useGuard(u, "if", u.If, sc, false) || fb.useGuard(u, "hidden", u.Attrs["hidden"], sc, true) {
		return true
	}
	for i, cl := range u.With {
		if _, found := sc.resolve(fb.a.store, cl.Path); found || fb.said(u, i) {
			continue
		}
		if ok, via := fb.pathMiss(u, "with", i, cl.Path, sc, missValue); ok {
			fb.reportPath(u, via, cl.Path, fb.bindSev(), "B003", "with path %q (alias %s) is missing", shownPath(cl.Path, via), ir.ShownAt(u, cl.Alias))
			fb.saidNow(u, i)
		}
	}
	return false
}

// useGuard evaluates the if (or, flag, the hidden) of the use u, written
// v, in sc: whether it prunes the copy. A missing path is B002 at the use,
// null, and through a with alias an optional prop (pathMiss).
func (fb *builder) useGuard(u *ir.Node, attr, v string, sc *scope, flag bool) bool {
	switch {
	case v == "":
		return false
	case flag && (v == "true" || v == "false"):
		return v == "true"
	}
	g, err := ir.ParseGuardField(v)
	if err != nil {
		return false
	}
	val, found := sc.resolve(fb.a.store, g.Path)
	which := -1
	if flag {
		which = -2
	}
	if !found && !fb.said(u, which) {
		if ok, via := fb.pathMiss(u, attr, 0, g.Path, sc, missFlag); ok {
			fb.reportPath(u, via, g.Path, ir.Warning, "B002", "%s=%q: path %q is missing", attr, writtenAttr(u, attr, v, via), shownPath(g.Path, via))
			fb.saidNow(u, which)
		}
	}
	// A truthy if keeps the copy (path, or !path over a falsy value), a
	// truthy hidden prunes it.
	return (ir.Truthy(val) != g.Neg) == flag
}

// useMsg is one message placeUse may report about a use: its if (-1), its
// hidden (-2), or the with clause of that index.
type useMsg struct {
	use   *ir.Node
	which int
}

// said reports whether this pass already reported message which about u.
func (fb *builder) said(u *ir.Node, which int) bool {
	return fb.useSaid[useMsg{u, which}]
}

// saidNow records that message which about u is reported.
func (fb *builder) saidNow(u *ir.Node, which int) {
	if fb.useSaid == nil {
		fb.useSaid = map[useMsg]bool{}
	}
	fb.useSaid[useMsg{u, which}] = true
}

// missKind is how a path that does not resolve reads (SPEC §7, v0.4b
// §6.18.5): a value ({path}, bind, key, checked, a with path, an each
// array) is B003 (B001 for an each), and a flag or a guard (if, hidden,
// disabled, open, class:NAME) is B002 and null.
type missKind uint8

const (
	missValue missKind = iota
	missFlag
)

// pathMiss decides whether the path p of n, written in slot as its seg-th
// path (ir.ViaKey), which does not resolve in sc, reports its B001, B002,
// or B003, and returns how p went through a with alias (nil for none),
// which the message then names (reportPath).
// A path through an alias whose own path does not resolve reports
// nothing: its use's B003 is the only report (decision 14), and an each
// through it has no rows and no B001 (the owner's ruling on the F3
// erratum). A flag or a guard through an alias that stands for an object
// is an optional prop (decision 100): missing, it is null, with no B002.
// A flag through an alias that stands for anything else is B002, a value
// B003, and an each B001, as anywhere.
func (fb *builder) pathMiss(n *ir.Node, slot string, seg int, p string, sc *scope, kind missKind) (bool, *ir.Via) {
	via := n.ViaOf(slot, seg)
	if via == nil {
		return true, nil
	}
	base, found := sc.resolve(fb.a.store, via.Base)
	if !found {
		return false, via
	}
	if _, obj := base.(map[string]any); obj && kind == missFlag {
		return false, via
	}
	return true, via
}

// badEach reports the B001 of n's each, whose path p resolved in sc to v,
// not an array, or did not resolve (found false): the each as written and
// p through a with alias name it as written (SPEC v0.4b §6.18.5). A path
// through an alias whose own path is missing reports nothing (pathMiss).
func (fb *builder) badEach(n *ir.Node, p string, v any, found bool, sc *scope) {
	via := n.ViaOf("each", 0)
	each := n.Each
	if via != nil && n.Written != nil {
		each = n.Written.Each
	}
	if !found {
		if ok, _ := fb.pathMiss(n, "each", 0, p, sc, missValue); ok {
			fb.reportPath(n, via, p, ir.Error, "B001", "each=%q: path %q is missing (want an array)", each, shownPath(p, via))
		}
		return
	}
	fb.reportPath(n, via, p, ir.Error, "B001", "each=%q: %q is %s, not an array", each, shownPath(p, via), typeName(v))
}

// missingKey reports the B003 of a key= path k of n missing on row i, in
// the row's scope sc; format quotes key= and names the row.
func (fb *builder) missingKey(n *ir.Node, k string, sc *scope, sev, format string, i int) {
	if ok, via := fb.pathMiss(n, "key", 0, k, sc, missValue); ok {
		fb.reportPath(n, via, k, sev, "B003", format, writtenAttr(n, "key", k, via), i)
	}
}

// reportPath reports a B001, B002, or B003 about the path p of n. One that
// went through a with alias (via) ends with ` (P via <use id="U">)` (SPEC
// v0.4b §6.18.5 "Messages"), which names its placement, so it carries no
// placement suffix (§6.18.6); a diagnostic about a <use> carries none
// either (placeUse). Any other carries the placement suffix (report).
func (fb *builder) reportPath(n *ir.Node, via *ir.Via, p string, sev, code, format string, args ...any) {
	switch {
	case via != nil:
		fb.add(ir.AtSuffix(n, fb.a.file, sev, code, via.Suffix(p), format, args...))
	case n.Kind == "use":
		fb.add(ir.AtSuffix(n, fb.a.file, sev, code, "", format, args...))
	default:
		fb.report(n, sev, code, format, args...)
	}
}

// shownPath is the path p of a message: as written when it went through
// a with alias (via), else p itself. At an element of a copy, which every
// via is, the message cuts it (ir.At, ir.CutQuoted).
func shownPath(p string, via *ir.Via) string {
	if via != nil {
		return via.Written
	}
	return p
}

// writtenAttr is the value of n's attribute attr a message quotes: as
// written when the path in it went through a with alias (via), else v,
// the value the runtime reads (cut at a copy, as shownPath).
func writtenAttr(n *ir.Node, attr, v string, via *ir.Via) string {
	if via != nil && n.Written != nil {
		if w, ok := n.Written.Attrs[attr]; ok {
			return w
		}
	}
	return v
}

// diagKey is what makes two diagnostics one (the dedupe of a frame's and
// of Validate's diagnostics): the whole diagnostic as 0.4.1 compared it
// (String, with its path), except in a document that expanded an
// <include> or a <use> (Composed), where one written element may stand
// at several paths and diagnostics equal in severity, code, message,
// file, line, and column are one, kept with the path and id of the first
// placement (SPEC v0.4b §6.18.6, decision 16). The message already holds
// what tells two placements apart: the placement suffix, or the
// ` (P via <use …>)` of a path through a with alias.
func (a *App) diagKey(d ir.Diagnostic) string {
	if a.doc.Composed {
		d.Path = "" // a copy: d is the caller's diagnostic unchanged
	}
	return d.String()
}

// bindSev is the severity of B003: a warning, an error under --strict.
func (fb *builder) bindSev() string {
	if fb.a.strict {
		return ir.Error
	}
	return ir.Warning
}
