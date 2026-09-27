package host

import (
	"math"
	"strconv"
	"strings"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/layout"
)

// The <hints> widget of SPEC v0.2b §6.12 and the <sparkline> of §6.11.
//
// A keymap row with a non-empty label is a hint row. A hints with
// scope="active" (the default) shows the hint rows that one of their keys
// would fire now: the key dispatch of §8.6 (planKey), run without side
// effects on the frame after step 4 of §18, reaches its keymap step and
// selects that row. scope="all" shows every hint row. Each item is a
// generated anonymous row holding two generated texts, the keycap (class
// hint-key) and the label (class hint-label); layout keeps the items that
// fit (§6.12 "Layout").

// hintItem is one item of a hints: the key text it shows and its label.
type hintItem struct {
	keycap, label string
}

// keyOf is the key a key token delivers (SPEC §8.1, §15.4): a printable
// character carries its rune (space included), a named key or ctrl+X
// only its name.
func keyOf(tok string) Key {
	if tok == "space" {
		return Key{Name: "space", Rune: ' '}
	}
	for _, k := range ir.NamedKeys {
		if tok == k {
			return Key{Name: tok}
		}
	}
	if strings.HasPrefix(tok, "ctrl+") {
		return Key{Name: tok}
	}
	r := []rune(tok)[0]
	return Key{Name: string(r), Rune: r}
}

// neverArrives reports the key tokens that never reach a keymap: the
// decoder turns their bytes into tab and enter (SPEC §8.1).
func neverArrives(tok string) bool {
	return tok == "ctrl+i" || tok == "ctrl+j" || tok == "ctrl+m"
}

// hintItems returns the items of a hints on v (SPEC §6.12): the hint rows
// in keymap order; with all, every one, its keycap being its keycap= else
// its first key token as written; otherwise the rows that one of their
// keys would fire now, its keycap being its keycap= else the first of its
// keys, in keys= order, that would fire it. The caller holds a.mu.
func (a *App) hintItems(v keyView, all bool) []hintItem {
	var out []hintItem
	for i := range a.doc.Keymap {
		kb := &a.doc.Keymap[i]
		if kb.Label == "" || len(kb.Keys) == 0 {
			continue
		}
		if all {
			keycap := kb.Keys[0]
			if kb.HasKeycap {
				keycap = kb.Keycap
			}
			out = append(out, hintItem{keycap, kb.Label})
			continue
		}
		for _, tok := range kb.Keys {
			if !ir.ValidKey(tok) || neverArrives(tok) {
				continue
			}
			if p := a.planKey(v, keyOf(tok)); p.step == stepKeymap && p.row == i {
				keycap := tok
				if kb.HasKeycap {
					keycap = kb.Keycap
				}
				out = append(out, hintItem{keycap, kb.Label})
				break
			}
		}
	}
	return out
}

// buildHints is step 5 of SPEC §18: it builds the items of each hints of
// the screen and of the open modals from the key dispatch run on this
// frame (the tree after step 4), and cascades them: the item row as a
// child of its hints (hints > row), its texts as children of the row.
// Generated nodes ignore their own display and box properties (§6.12),
// so none of them is dropped.
func (fb *builder) buildHints(casc *css.Cascade, root *layout.Box) {
	var hs []*layout.Box
	collect := func(r *layout.Box) {
		if r == nil {
			return
		}
		r.Walk(func(b *layout.Box) {
			if b.Kind == "hints" {
				hs = append(hs, b)
			}
		})
	}
	collect(root)
	for _, m := range fb.modals {
		collect(m)
	}
	if len(hs) == 0 {
		return
	}
	a := fb.a
	var focused *layout.Box
	if a.focus != "" {
		focused = fb.byID[a.focus]
	}
	v := keyView{focused: focused, root: root, modals: fb.modals, byID: fb.byID}
	items := map[bool][]hintItem{}
	for _, h := range hs {
		all := h.Src != nil && h.Src.Attrs["scope"] == "all"
		its, ok := items[all]
		if !ok {
			its = a.hintItems(v, all)
			items[all] = its
		}
		h.Children = h.Children[:0]
		for _, it := range its {
			row := &layout.Box{Tag: "row", Kind: "row", Role: layout.RoleHintItem, Parent: h, Fixed: true, Disabled: h.Disabled, Follow: -1, Index: -1}
			key := &layout.Box{Tag: "text", Kind: "text", Role: layout.RoleHintKey, Parent: row, Classes: []string{"hint-key"}, Text: it.keycap, Fixed: true, Disabled: h.Disabled, Follow: -1, Index: -1}
			label := &layout.Box{Tag: "text", Kind: "text", Role: layout.RoleHintLabel, Parent: row, Classes: []string{"hint-label"}, Text: it.label, Fixed: true, Disabled: h.Disabled, Follow: -1, Index: -1}
			row.Children = []*layout.Box{key, label}
			row.Style = casc.Compute(row, &h.Style, nil, nil)
			key.Style = casc.Compute(key, &row.Style, nil, nil)
			label.Style = casc.Compute(label, &row.Style, nil, nil)
			h.Children = append(h.Children, row)
		}
	}
}

// inflateSparkline resolves a sparkline's bind (SPEC §6.11) into
// b.Series, one value per array element, NaN for a gap (null, or an
// element that is not a number: B009 once per node and frame), and its
// min= and max=. A missing path is B003 and a value that is not an array
// B009 (a warning); both paint nothing.
func (fb *builder) inflateSparkline(n *ir.Node, b *layout.Box, sc *scope) {
	if s, ok := n.Attr("min"); ok {
		if f, err := strconv.ParseFloat(s, 64); err == nil && isFinite(f) {
			b.Lo, b.HasLo = f, true
		}
	}
	if s, ok := n.Attr("max"); ok {
		if f, err := strconv.ParseFloat(s, 64); err == nil && isFinite(f) {
			b.Hi, b.HasHi = f, true
		}
	}
	if n.Bind == "" {
		return
	}
	v, found := sc.resolve(fb.a.store, n.Bind)
	if !found {
		fb.missingBind(n, n.Bind)
		return
	}
	arr, ok := v.([]any)
	if !ok {
		fb.report(n, ir.Warning, "B009", "bind=%q is %s, not an array of numbers; the sparkline paints nothing", n.Bind, typeName(v))
		return
	}
	b.Series = make([]float64, len(arr))
	bad := -1
	for i, el := range arr {
		switch x := el.(type) {
		case float64:
			b.Series[i] = x
		case nil:
			b.Series[i] = math.NaN()
		default:
			b.Series[i] = math.NaN()
			if bad < 0 {
				bad = i
			}
		}
	}
	if bad >= 0 {
		fb.report(n, ir.Warning, "B009", "bind=%q: element %d is %s, not a number or null; it is a gap", n.Bind, bad, typeName(arr[bad]))
	}
}

func isFinite(f float64) bool { return !math.IsNaN(f) && !math.IsInf(f, 0) }
