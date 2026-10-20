package host

import (
	"strings"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/layout"
)

// The form controls of SPEC v0.4b (phase F8) at run time: the <checkbox>
// (§6.16) and single="true" on a list or a table (§6.14.1). Both exist
// only in a <tui> without version: the parser rejects them, and leaves
// ir.Node.Single unset, in a document that writes version.

// inflateCheckbox resolves a checkbox for this frame (SPEC v0.4b §6.16):
// its label into b.Text, and whether it is checked into b.Checked, which
// :checked, the dump's checked, and the glyph read.
func (fb *builder) inflateCheckbox(n *ir.Node, b *layout.Box, sc *scope) {
	b.Text = fb.checkboxLabel(n, sc)
	b.Checked = fb.checkboxChecked(n, sc)
}

// checkboxLabel is a checkbox's label (SPEC v0.4b §6.16.1): a template
// resolved every frame in the checkbox's scope, like a version="3" tab
// label. A missing path is B003 and resolves to the empty string, and
// each LF a resolved value brings becomes a space, since a checkbox is
// one row. An absent label is the empty string.
func (fb *builder) checkboxLabel(n *ir.Node, sc *scope) string {
	text := n.Attrs["label"]
	if text == "" {
		return ""
	}
	segs, ok := fb.a.segsOf(text)
	if !ok {
		return text
	}
	var b strings.Builder
	for _, sg := range segs {
		if sg.Path == "" {
			b.WriteString(sg.Lit)
			continue
		}
		v, found := sc.resolve(fb.a.store, sg.Path)
		if !found {
			fb.missingBind(n, sg.Path)
			continue
		}
		b.WriteString(oneLineLabel(cleanText(Format(v))))
	}
	return b.String()
}

// checkboxChecked is what a checkbox shows (SPEC v0.4b §6.16.2). With
// bind, true is checked; false and null are unchecked; a missing path is
// unchecked and B003; any other value is B011 (a warning) and unchecked.
// Showing it never writes the store. Without bind, it is the value kept
// by the checkbox's id (App.checks), unchecked until a toggle.
func (fb *builder) checkboxChecked(n *ir.Node, sc *scope) bool {
	if n.Bind == "" {
		return fb.a.checks[n.ID]
	}
	v, found := sc.resolve(fb.a.store, n.Bind)
	if !found {
		fb.missingBind(n, n.Bind)
		return false
	}
	switch v := v.(type) {
	case bool:
		return v
	case nil:
		return false
	}
	fb.report(n, ir.Warning, "B011", "bind=%q holds %s, not true, false, or null (shown unchecked; a toggle writes true over it)", n.Bind, typeName(v))
	return false
}

// checkboxValue is the value of checkbox b as its toggle and a keymap
// row's payload see it (SPEC v0.4b §6.16.2, §8.2): what the store holds
// at its bind path now (true is checked, anything else, a B011 value
// included, is not), or the value kept by its id without bind. A checkbox
// is never inside a list item or an each template (V001), so its path
// resolves against the store. The caller holds a.mu.
func (a *App) checkboxValue(b *layout.Box) bool {
	if b.Src == nil || b.Src.Bind == "" {
		return a.checks[b.ID]
	}
	v, _ := lookup(a.store, b.Src.Bind)
	return v == true
}

// toggleCheckbox toggles checkbox b (SPEC v0.4b §6.16.2): the new value is
// true when it is unchecked (a B011 value included) and false when it is
// checked; with bind it is written to the path, which is created when
// missing, and without bind it is kept by the checkbox's id. Then on:change
// fires with {source: its id, keys: {}, value: the new boolean}. A path
// that cannot be written (a value along it present and not an object) or
// that is derived (§18.2) changes nothing and fires nothing; the key that
// asked is consumed either way. The caller holds a.mu.
func (a *App) toggleCheckbox(b *layout.Box) []Event {
	next := !a.checkboxValue(b)
	if b.Src != nil && b.Src.Bind != "" {
		path := b.Src.Bind
		if !canAssign(a.store, path) || a.derivedBlocked(path, next) {
			return nil
		}
		if err := a.writeStore(path, next); err != nil {
			return nil
		}
	} else {
		a.checks[b.ID] = next
	}
	// The frame shows the value, and when, class guards, and if may read
	// it (SPEC v0.4b §8.6 "Inputs of one read", decision 106).
	a.markStale()
	if b.Src == nil {
		return nil
	}
	if act, ok := b.Src.On["change"]; ok {
		return []Event{{Action: act, Source: b.ID, Keys: map[string]any{}, Value: next}}
	}
	return nil
}

// singleValue reads the checked= value of a single="true" list or table
// (SPEC v0.4b §6.14.1): one key, or nil for null and for a missing path
// (which counts as null). ok is false when the widget has no checked=,
// when its value is an array or an object (B008), which the runtime never
// overwrites, and when the path is missing because a value along it is
// present and not an object, which the runtime could not write: the
// check-* actions and space do nothing on such a widget. A list or a table
// is never inside an each template, so its path resolves against the
// store. The caller holds a.mu.
func (a *App) singleValue(b *layout.Box) (v any, ok bool) {
	if b.Src == nil {
		return nil, false
	}
	path, has := b.Src.Attr("checked")
	if !has || !ir.IsPathField(path) {
		return nil, false
	}
	v, found := lookup(a.store, path)
	if !found {
		return nil, canAssign(a.store, path)
	}
	switch v.(type) {
	case []any, map[string]any:
		return nil, false
	}
	return v, true
}

// isSingle reports whether b is a list or a table with single="true"
// (SPEC v0.4b §6.14.1).
func isSingle(b *layout.Box) bool {
	return (b.Kind == "list" || b.Kind == "table") && b.Src != nil && b.Src.Single
}

// singleCompatible is §8.4's compatibility of a single widget with a
// check-* action: a value that is a key or null, or missing where the
// runtime can write it, at least one row, never check-all, and a path
// that is not derived (§18.2). The caller holds a.mu.
func (a *App) singleCompatible(action string, t *layout.Box) bool {
	if action == "check-all" || a.rows(t) == 0 {
		return false
	}
	_, ok := a.singleValue(t)
	// The value written is a row key or null, a scalar.
	return ok && !a.derivedBlocked(t.Src.Attrs["checked"], "")
}

// checkSingle applies check-toggle or check-none to a single widget (SPEC
// v0.4b §6.14.1), with K the key of the cursor row and V the value:
// check-toggle leaves V alone when it equals K (a radio button is not
// unchecked by choosing it again) or when K is an array or an object (such
// a value would be B008), and otherwise makes V the key K, with its JSON
// type; check-none makes V null. check-all never applies. When V changed,
// it is written and on:change fires with the new value (a key or null);
// otherwise nothing is written and nothing fires. The caller holds a.mu.
func (a *App) checkSingle(b *layout.Box, action string) []Event {
	ls := a.lists[b.ID]
	old, ok := a.singleValue(b)
	if ls == nil || len(ls.keys) == 0 || !ok {
		return nil
	}
	var next any
	switch action {
	case "check-toggle":
		k := ls.keys[min(max(ls.index, 0), len(ls.keys)-1)]
		switch k.(type) {
		case []any, map[string]any:
			return nil
		}
		if keyEqual(old, k) {
			return nil
		}
		next = deepCopyJSON(k)
	case "check-none":
		if old == nil {
			return nil
		}
	default:
		return nil
	}
	// writeStore refuses a path that is derived (SPEC v0.4a §18.2):
	// nothing changes and on:change does not fire.
	if err := a.writeStore(b.Src.Attrs["checked"], next); err != nil {
		return nil
	}
	a.markStale()
	if act, ok := b.Src.On["change"]; ok {
		ev := a.eventFor(act, b)
		ev.Value = deepCopyJSON(next)
		return []Event{ev}
	}
	return nil
}
