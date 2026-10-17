package host

import (
	"strings"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/layout"
)

// The row cache of a list with each= (0.4a, an internal cache with no
// observable effect). Resolving a list's rows reads every element of its
// array: their keys, and the item template inflated once per element,
// with every {path}, guard, and bind in it. The result depends only on
// the store members those paths read, the --strict setting (the severity
// of B003), and whether the list itself is disabled (its rows inherit
// it), so a list keeps it, as a compact prototype per box, while all of
// them are unchanged, and each frame builds its row boxes from the
// prototypes. The cursor, the checked keys, and the mark channel are
// read on every frame, as without the cache, and so is everything the
// cascade, layout, and paint do afterwards.
//
// Only a list in the document's scope (no enclosing each) is cached, and
// only when every node of its template is of a kind whose inflation reads
// nothing but the store, the element, and its attributes
// (listCacheKinds): a template holding anything else (an input, a nested
// list, a modal, a tabs, a table, a hints, which a valid document never
// puts in a row anyway) is inflated every frame.

// listCacheKinds are the kinds a cached template may hold.
var listCacheKinds = map[string]bool{
	"item": true, "text": true, "col": true, "row": true, "box": true,
	"spacer": true, "rule": true, "progress": true, "scroll": true, "sparkline": true,
}

// listCache is what the runtime keeps per list node: its plan, parsed
// once, and the rows it last resolved.
type listCache struct {
	plan *listPlan
	rows *listRows
}

// listPlan is a list node's each, key path, and item template, with deps,
// the top-level store members that resolving its rows reads (each path
// head of the template, aliases included, which only ever adds an
// invalidation), and ok, whether its rows are cached at all.
type listPlan struct {
	ok       bool
	each     ir.Each
	keyPath  string
	template *ir.Node
	deps     []string
	nodes    map[*ir.Node]*protoNode
	// scroll is set when the template holds a scroll.
	scroll bool
}

// protoNode is what every box of one template node shares, whatever its
// row: its identity, declarations, and attribute literals.
type protoNode struct {
	src           *ir.Node
	tag, kind, id string
	axis, scale   string
	hints, inline []css.Decl
	lo, hi        float64
	hasLo, hasHi  bool
	follow        int
}

// protoBox is one box of a resolved row, in preorder: what inflate
// resolved for it on its row (kids is the number of its children, which
// follow it).
type protoBox struct {
	node        *protoNode
	classes     []string
	guardOn     []bool
	title, text string
	key         string
	series      []float64
	value       float64
	index       int32
	kids        int32
	disabled    bool
}

// listRows is a list's resolved rows: the stamp of the store members it
// read, the --strict setting and the list's disabled state it was
// resolved with, each row's key (with Format(key) and, built on first
// use, its keyID), the diagnostics resolving reported (pre: the each and
// key ones, which come before the cursor's; then the rows'), and the
// prototypes of every row (start[i]:start[i+1], empty for a row whose
// item if or hidden pruned).
type listRows struct {
	stamp    []uint64
	strict   bool
	disabled bool
	keys     []any
	keyStr   []string
	ids      []string
	first    map[string]int
	pre      ir.Diags
	post     ir.Diags
	protos   []protoBox
	start    []int32
	nkids    int
}

// listPlan returns the plan of list node n, parsed once. The caller holds
// a.mu.
func (a *App) listPlan(n *ir.Node) *listPlan {
	if c := a.listRows[n]; c != nil {
		return c.plan
	}
	p := &listPlan{keyPath: n.Attrs["key"], nodes: map[*ir.Node]*protoNode{}}
	a.listRows[n] = &listCache{plan: p}
	for _, c := range n.Children {
		if c.Kind == "item" {
			p.template = c
			break
		}
	}
	e, err := ir.ParseEach(n.Each)
	if err != nil || p.template == nil {
		return p
	}
	p.each = e
	seen := map[string]bool{}
	dep := func(path string) {
		head, _, _ := strings.Cut(path, ".")
		if head != "" && !seen[head] {
			seen[head] = true
			p.deps = append(p.deps, head)
		}
	}
	guard := func(v string) {
		if g, err := ir.ParseGuard(v); err == nil {
			dep(g.Path)
		}
	}
	interp := func(v string) {
		segs, _ := ir.ParseInterp(v)
		for _, sg := range segs {
			if sg.Path != "" {
				dep(sg.Path)
			}
		}
	}
	dep(e.Path)
	if p.keyPath != "" {
		dep(p.keyPath)
	}
	ok := true
	var walk func(t *ir.Node)
	walk = func(t *ir.Node) {
		if !listCacheKinds[t.Kind] {
			ok = false
			return
		}
		if t.Kind == "scroll" {
			p.scroll = true
		}
		guard(t.If)
		guard(t.Bind)
		if t.Each != "" {
			if te, err := ir.ParseEach(t.Each); err == nil {
				dep(te.Path)
			}
		}
		interp(t.Text)
		for _, v := range t.Attrs {
			guard(v)
			interp(v)
			if te, err := ir.ParseEach(v); err == nil {
				dep(te.Path)
			}
		}
		for _, cg := range t.ClassGuards {
			guard(cg.Guard)
		}
		for _, c := range t.Children {
			walk(c)
		}
	}
	walk(p.template)
	p.ok = ok
	return p
}

// rowsOf returns the resolved rows of list node n, whose box is b, from
// the cache while its stamp, the --strict setting, and b's disabled state
// are unchanged; items is nil then. Otherwise it resolves them again and
// gives back, in items, the item boxes it inflated (nil for a pruned
// row), which this frame uses as they are. The diagnostics are not
// reported here. The caller holds a.mu.
func (fb *builder) rowsOf(n *ir.Node, p *listPlan, b *layout.Box) (r *listRows, items []*layout.Box) {
	a := fb.a
	c := a.listRows[n]
	st := a.stamp(p.deps)
	if r := c.rows; r != nil && r.strict == a.strict && r.disabled == b.Disabled && sameStamp(r.stamp, st) {
		return r, nil
	}
	sub := &builder{a: a, seen: map[string]bool{}}
	r = &listRows{stamp: st, strict: a.strict, disabled: b.Disabled}
	v, found := (*scope)(nil).resolve(a.store, p.each.Path)
	arr, isArr := v.([]any)
	if !found {
		sub.report(n, ir.Error, "B001", "each=%q: path %q is missing (want an array)", n.Each, p.each.Path)
	} else if !isArr {
		sub.report(n, ir.Error, "B001", "each=%q: %q is %s, not an array", n.Each, p.each.Path, typeName(v))
	}
	scopes := make([]scope, len(arr))
	r.keys = make([]any, len(arr))
	for i, el := range arr {
		isc := &scopes[i]
		*isc = scope{alias: p.each.Alias, value: el}
		var key any = float64(i)
		if p.keyPath != "" {
			if kv, ok := isc.resolve(a.store, p.keyPath); ok {
				key = kv
			} else {
				sub.report(n, ir.Warning, "B003", "key=%q is missing on item %d", p.keyPath, i)
			}
		}
		isc.key = key
		r.keys[i] = key
	}
	if old := c.rows; old != nil && identicalKeys(old.keys, r.keys) {
		// The same keys as the rows these replace (only what the rows
		// show changed): their texts and index still hold.
		r.keyStr, r.ids, r.first = old.keyStr, old.keyIDs(), old.first
	} else {
		r.keyStr = make([]string, len(arr))
		for i, k := range r.keys {
			r.keyStr[i] = Format(k)
		}
	}
	r.pre = sub.diags
	sub.diags = nil
	items = make([]*layout.Box, len(arr))
	r.start = make([]int32, len(arr)+1)
	for i := range arr {
		ib := sub.inflate(p.template, b, &scopes[i], true)
		items[i] = ib
		if ib != nil {
			r.protos = p.flatten(r.protos, ib, &r.nkids)
		}
		r.start[i+1] = int32(len(r.protos))
	}
	r.post = sub.diags
	r.packTexts()
	c.rows = r
	return r, items
}

// packTexts moves the texts of every prototype into one string, in
// preorder, which is the order every walk of the frame visits them, so
// that measuring and checking the rows' texts reads memory in sequence
// instead of one scattered string per row.
func (r *listRows) packTexts() {
	total := 0
	for i := range r.protos {
		total += len(r.protos[i].text)
	}
	var sb strings.Builder
	sb.Grow(total)
	for i := range r.protos {
		sb.WriteString(r.protos[i].text)
	}
	all, off := sb.String(), 0
	for i := range r.protos {
		n := len(r.protos[i].text)
		r.protos[i].text = all[off : off+n]
		off += n
	}
}

// flatten appends the prototypes of b and its descendants, in preorder,
// counting their children in *kids.
func (p *listPlan) flatten(dst []protoBox, b *layout.Box, kids *int) []protoBox {
	nd := p.nodes[b.Src]
	if nd == nil {
		nd = &protoNode{
			src: b.Src, tag: b.Tag, kind: b.Kind, id: b.ID, axis: b.Axis, scale: b.Scale,
			hints: b.Hints, inline: b.Inline, lo: b.Lo, hi: b.Hi, hasLo: b.HasLo, hasHi: b.HasHi,
			follow: b.Follow,
		}
		p.nodes[b.Src] = nd
	}
	dst = append(dst, protoBox{
		node: nd, classes: b.Classes, guardOn: b.GuardOn, title: b.Title, text: b.Text,
		key: b.Key, series: b.Series, value: b.Value, index: int32(b.Index),
		kids: int32(len(b.Children)), disabled: b.Disabled,
	})
	*kids += len(b.Children)
	for _, c := range b.Children {
		dst = p.flatten(dst, c, kids)
	}
	return dst
}

// materialize builds this frame's item boxes from the prototypes, one per
// row (nil for a pruned row), children of list box parent: every box in
// one slab, every children slice in another, each with its exact length
// as capacity, so an append to one never reaches the next.
func (r *listRows) materialize(parent *layout.Box) []*layout.Box {
	m := materializer{
		protos: r.protos,
		boxes:  make([]layout.Box, len(r.protos)),
		kids:   make([]*layout.Box, r.nkids),
	}
	items := make([]*layout.Box, len(r.keys))
	for i := range items {
		if r.start[i] < r.start[i+1] {
			items[i] = m.clone(parent)
		}
	}
	return items
}

type materializer struct {
	protos     []protoBox
	boxes      []layout.Box
	kids       []*layout.Box
	pi, bi, ki int
}

func (m *materializer) clone(parent *layout.Box) *layout.Box {
	p := &m.protos[m.pi]
	m.pi++
	b := &m.boxes[m.bi]
	m.bi++
	nd := p.node
	b.Tag, b.Kind, b.ID, b.Src, b.Parent = nd.tag, nd.kind, nd.id, nd.src, parent
	b.Hints, b.Inline, b.Axis, b.Scale = nd.hints, nd.inline, nd.axis, nd.scale
	b.Lo, b.Hi, b.HasLo, b.HasHi = nd.lo, nd.hi, nd.hasLo, nd.hasHi
	b.Follow = nd.follow
	b.Classes, b.GuardOn = p.classes, p.guardOn
	b.Title, b.Text, b.Key, b.Series, b.Value = p.title, p.text, p.key, p.series, p.value
	b.Index, b.Disabled = int(p.index), p.disabled
	if k := int(p.kids); k > 0 {
		ch := m.kids[m.ki : m.ki+k : m.ki+k]
		m.ki += k
		for j := range ch {
			ch[j] = m.clone(b)
		}
		b.Children = ch
	}
	return b
}

// identicalKeys reports whether a and b hold the same keys, in order: equal
// strings, numbers, booleans, or nulls (any other key counts as changed).
func identicalKeys(a, b []any) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		switch x := a[i].(type) {
		case string:
			if y, ok := b[i].(string); !ok || x != y {
				return false
			}
		case float64:
			if y, ok := b[i].(float64); !ok || x != y {
				return false
			}
		case bool:
			if y, ok := b[i].(bool); !ok || x != y {
				return false
			}
		case nil:
			if b[i] != nil {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// keyIDs returns each row's keyID, built once per resolved rows.
func (r *listRows) keyIDs() []string {
	if r.ids == nil {
		r.ids = make([]string, len(r.keys))
		for i, k := range r.keys {
			r.ids[i] = keyID(k)
		}
	}
	return r.ids
}

// rowOf returns the first row whose key equals v (keyEqual), or -1, from
// an index built once per resolved rows (as tableData.rowOf).
func (r *listRows) rowOf(v any) int {
	if r.first == nil {
		ids := r.keyIDs()
		r.first = make(map[string]int, len(ids))
		for i := len(ids) - 1; i >= 0; i-- {
			r.first[ids[i]] = i
		}
	}
	if i, ok := r.first[keyID(v)]; ok {
		return i
	}
	return -1
}
