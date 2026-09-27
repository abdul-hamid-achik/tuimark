package host

import (
	"strings"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/layout"
	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// The <table> widget of SPEC v0.2b §6.9, in the frame order of §18:
//
//   - step 1 (inflateTable) resolves the rows of the each array on every
//     row, visible or not (their keys, the row template's guards, and
//     each column's cell texts and guards), the cursor, the checked keys,
//     and the header cells, which are the column nodes titled in the
//     table's scope;
//   - step 2 cascades the table and its columns and drops the columns
//     whose display is none, which leaves the visible ones;
//   - step 6 (layout) sizes the columns and the body viewport and moves
//     the offset so that the cursor row is visible;
//   - step 7 (placeTableRows) generates the visible rows and their body
//     cells, places them, and cascades them.
//
// The table's cursor is a listState kept by its id, as a list's is, so
// the key dispatch, the built-in actions, and the event payloads treat
// both widgets alike.

// tableCache is what the runtime keeps per table node: its parsed plan,
// and the rows it last resolved with the store stamp they depend on.
type tableCache struct {
	plan *tablePlan
	data *tableData
}

// tablePlan is a table node's templates, parsed once: its each, key
// path, row template, columns (in document order) with their cell
// templates, and deps, the top-level store members that its rows read
// outside the alias (the each array included), for the cache stamp.
type tablePlan struct {
	each     ir.Each
	ok       bool // each parsed
	keyPath  string
	item     *ir.Node
	cols     []*ir.Node
	colIndex map[*ir.Node]int
	segs     [][]ir.Segment
	deps     []string
}

// tableData is what a table reads from every row of its each array (SPEC
// §6.9.2, §6.9.4): the number of rows, their keys, the row template's
// guards, and per column (plan order) the resolved cell texts, the
// guards, and the measure (the widest cell text), with the diagnostics
// that resolving them reported. Resolving reads every row, so a table
// keeps it while the store paths it read are unchanged (§6.9.4 "Cost";
// the stamp is taken by stamp) and the --strict setting is the same.
type tableData struct {
	stamp   []uint64
	strict  bool
	rows    int
	keys    []any
	rowOn   [][]bool
	texts   [][]string
	on      [][][]bool
	measure []int
	diags   ir.Diags
}

// tableFrame is a table of the frame being built, between step 1 and
// step 7: its plan and rows, its checked keys, and its cursor.
type tableFrame struct {
	plan    *tablePlan
	data    *tableData
	checked []any
	sel     int
}

// tablePlan parses the templates of table node n once. The caller holds
// a.mu.
func (a *App) tablePlan(n *ir.Node) *tablePlan {
	if c := a.tables[n]; c != nil {
		return c.plan
	}
	p := &tablePlan{keyPath: n.Attrs["key"], colIndex: map[*ir.Node]int{}}
	if n.Each != "" {
		if e, err := ir.ParseEach(n.Each); err == nil {
			p.each, p.ok = e, true
		}
	}
	for _, c := range n.Children {
		switch {
		case c.Kind == "item" && p.item == nil:
			p.item = c
		case c.Kind == "column":
			segs, _ := ir.ParseInterp(c.Text)
			p.colIndex[c] = len(p.cols)
			p.cols = append(p.cols, c)
			p.segs = append(p.segs, segs)
		}
	}
	seen := map[string]bool{}
	dep := func(path string, rowScope bool) {
		head, _, _ := strings.Cut(path, ".")
		if head == "" || (rowScope && head == p.each.Alias) || seen[head] {
			return
		}
		seen[head] = true
		p.deps = append(p.deps, head)
	}
	if p.ok {
		dep(p.each.Path, false)
		if p.keyPath != "" {
			dep(p.keyPath, true)
		}
		guards := func(n *ir.Node) {
			if n == nil {
				return
			}
			for _, cg := range n.ClassGuards {
				if g, err := ir.ParseGuard(cg.Guard); err == nil {
					dep(g.Path, true)
				}
			}
		}
		guards(p.item)
		for j, c := range p.cols {
			for _, sg := range p.segs[j] {
				if sg.Path != "" {
					dep(sg.Path, true)
				}
			}
			guards(c)
		}
	}
	a.tables[n] = &tableCache{plan: p}
	return p
}

// stamp is the store state a table's rows depend on: the whole-store
// generation, then the generation of each of deps. The caller holds a.mu.
func (a *App) stamp(deps []string) []uint64 {
	out := make([]uint64, 0, len(deps)+1)
	out = append(out, a.storeGen)
	for _, d := range deps {
		out = append(out, a.segGen[d])
	}
	return out
}

func sameStamp(a, b []uint64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// tableRows returns the rows of table node n (SPEC §6.9.2, §6.9.4) and
// reports their diagnostics into the frame: from the cache while the
// store paths they read are unchanged, else resolved again. A table in an
// enclosing each scope (which is V001) is resolved every frame. The
// caller holds a.mu.
func (fb *builder) tableRows(n *ir.Node, p *tablePlan, sc *scope) *tableData {
	a := fb.a
	c := a.tables[n]
	var td *tableData
	if sc == nil && c.data != nil && c.data.strict == a.strict && sameStamp(c.data.stamp, a.stamp(p.deps)) {
		td = c.data
	} else {
		td = fb.resolveTable(n, p, sc)
		if sc == nil {
			td.stamp = a.stamp(p.deps)
			c.data = td
		}
	}
	for _, d := range td.diags {
		fb.add(d)
	}
	return td
}

// add appends a diagnostic to the frame once.
func (fb *builder) add(d ir.Diagnostic) {
	k := d.String()
	if fb.seen[k] {
		return
	}
	fb.seen[k] = true
	fb.diags = append(fb.diags, d)
}

// resolveTable resolves every row of table node n: a missing each path or
// a value that is not an array is B001 and gives no rows; a key= path
// missing on a row gives its index and B003, per row; the row template's
// guards and each column's cell template and guards are resolved on every
// row, visible or not, and a missing path reports B002 (guard) or B003
// (cell template) once per template or guard and path, so the
// diagnostics never depend on the scroll offset. The caller holds a.mu.
func (fb *builder) resolveTable(n *ir.Node, p *tablePlan, sc *scope) *tableData {
	a := fb.a
	sub := &builder{a: a, seen: map[string]bool{}}
	k := len(p.cols)
	td := &tableData{strict: a.strict, texts: make([][]string, k), on: make([][][]bool, k), measure: make([]int, k)}
	if !p.ok {
		return td
	}
	v, found := sc.resolve(a.store, p.each.Path)
	arr, isArr := v.([]any)
	switch {
	case !found:
		sub.report(n, ir.Error, "B001", "each=%q: path %q is missing (want an array)", n.Each, p.each.Path)
	case !isArr:
		sub.report(n, ir.Error, "B001", "each=%q: %q is %s, not an array", n.Each, p.each.Path, typeName(v))
	}
	rows := len(arr)
	td.rows = rows
	td.keys = make([]any, rows)
	for j, c := range p.cols {
		td.texts[j] = make([]string, rows)
		if len(c.ClassGuards) > 0 {
			td.on[j] = make([][]bool, rows)
		}
	}
	if p.item != nil && len(p.item.ClassGuards) > 0 {
		td.rowOn = make([][]bool, rows)
	}
	keySev := ir.Warning
	if a.strict {
		keySev = ir.Error
	}
	for i, el := range arr {
		isc := &scope{alias: p.each.Alias, value: el, parent: sc}
		var key any = float64(i)
		if p.keyPath != "" {
			if kv, ok := isc.resolve(a.store, p.keyPath); ok {
				key = kv
			} else {
				sub.report(n, keySev, "B003", "key=%q is missing on row %d (its key is the index)", p.keyPath, i)
			}
		}
		isc.key = key
		td.keys[i] = key
		if td.rowOn != nil {
			td.rowOn[i] = sub.guardsOn(p.item, isc)
		}
		for j, c := range p.cols {
			text := sub.interpSegs(c, p.segs[j], isc)
			td.texts[j][i] = text
			if w := layout.MaxWidth(text); w > td.measure[j] {
				td.measure[j] = w
			}
			if td.on[j] != nil {
				td.on[j][i] = sub.guardsOn(c, isc)
			}
		}
	}
	td.diags = sub.diags
	return td
}

// guardsOn evaluates the class:NAME guards of n in scope sc, in attribute
// order (SPEC §6.13; a missing path is B002 and counts as null, as for
// if).
func (fb *builder) guardsOn(n *ir.Node, sc *scope) []bool {
	on := make([]bool, len(n.ClassGuards))
	for i, cg := range n.ClassGuards {
		on[i] = fb.guard(n, "class:"+cg.Name, cg.Guard, sc)
	}
	return on
}

// interpSegs is interp over a template parsed once: literal text and the
// value of each {path} in scope sc, formatted as text without control
// characters; a missing path renders empty and reports B003.
func (fb *builder) interpSegs(n *ir.Node, segs []ir.Segment, sc *scope) string {
	if len(segs) == 1 && segs[0].Path == "" {
		return segs[0].Lit
	}
	var b strings.Builder
	for _, sg := range segs {
		if sg.Path == "" {
			b.WriteString(sg.Lit)
			continue
		}
		v, ok := sc.resolve(fb.a.store, sg.Path)
		if !ok {
			fb.missingBind(n, sg.Path)
			continue
		}
		clean, _ := parse.StripControl(Format(v), true)
		b.WriteString(clean)
	}
	return b.String()
}

// guardClasses is the class list of a generated table node whose classes
// come from n (the row template, or a column for its body cells): n's
// class names, then the names of its guards that on marks truthy, in
// attribute order, without repeats (SPEC §6.9.4, §6.13).
func guardClasses(n *ir.Node, on []bool) []string {
	if n == nil {
		return nil
	}
	if len(on) == 0 {
		return n.Classes
	}
	out := append([]string(nil), n.Classes...)
	for i, cg := range n.ClassGuards {
		if i >= len(on) || !on[i] {
			continue
		}
		dup := false
		for _, c := range out {
			if c == cg.Name {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, cg.Name)
		}
	}
	return out
}

// inflateTable is step 1 of SPEC §18 for table node n, inflated as box b
// in scope sc (SPEC §6.9): its placeholder and header cells (the column
// nodes: static classes, title resolved in the table's scope, measure),
// its rows, and its cursor. With bind, the cursor is the first row, in
// array order, whose key equals the bound value (same JSON type, same
// text); when none does, when the value is null, or when the path is
// missing (B003), the cursor keeps its previous index. It is clamped to
// [0, n − 1], or 0 without rows. The caller holds a.mu.
func (fb *builder) inflateTable(n *ir.Node, b *layout.Box, sc *scope) {
	a := fb.a
	p := a.tablePlan(n)
	td := fb.tableRows(n, p, sc)
	b.Placeholder = fb.interp(n, n.Attrs["placeholder"], sc)
	for j, c := range p.cols {
		cb := &layout.Box{
			Tag: c.Tag, Kind: c.Kind, ID: c.ID, Classes: c.Classes, Parent: b, Src: c,
			Hints: a.toDecls(c.Hints), Inline: a.toDecls(c.Inline), Follow: -1, Index: -1,
			Fixed: true, Disabled: b.Disabled,
		}
		cb.Title = fb.interp(c, c.Attrs["title"], sc)
		cb.Text = cb.Title
		cb.Measure = max(layout.MaxWidth(cb.Text), td.measure[j])
		if c.ID != "" {
			fb.byID[c.ID] = cb
		}
		b.Children = append(b.Children, cb)
	}
	ls := a.list(n.ID)
	if n.ID == "" {
		ls = &listState{}
	}
	ls.alias, ls.each = p.each.Alias, true
	ls.keys = append(ls.keys[:0], td.keys...)
	sel := ls.index
	if n.Bind != "" {
		v, found := sc.resolve(a.store, n.Bind)
		if !found {
			fb.missingBind(n, n.Bind)
		}
		if found && v != nil {
			for i, k := range td.keys {
				if keyEqual(k, v) {
					sel = i
					break
				}
			}
		}
	}
	if td.rows == 0 {
		sel = 0
	} else {
		sel = min(max(sel, 0), td.rows-1)
	}
	ls.index = sel
	// SPEC §6.14: the checked row keys, and the mark channel of every
	// row, the header row's too (§6.9.3).
	var checked []any
	if path, ok := n.Attr("checked"); ok && ir.IsPath(path) {
		checked, _ = fb.checkedKeys(n, path, sc)
		if m, ok := n.Attr("mark"); ok {
			if w := layout.Width(m); w >= 1 && w <= 2 {
				b.MarkChan, b.Mark = w+1, m
			}
		}
	}
	b.Rows = td.rows
	b.Follow = -1
	if td.rows > 0 {
		b.Follow = sel
	}
	if fb.tables == nil {
		fb.tables = map[*layout.Box]*tableFrame{}
	}
	fb.tables[b] = &tableFrame{plan: p, data: td, checked: checked, sel: sel}
}

// placeTableRows is step 7 of SPEC §18: after layout, for each laid-out
// table (the screen's, then the modals'), it generates rows o to
// min(o + V, n) − 1 and their body cells, places them (§6.9.3), and
// cascades them, with the frame's media and theme.
func (fb *builder) placeTableRows(casc *css.Cascade, root *layout.Box) {
	var tables []*layout.Box
	collect := func(r *layout.Box) {
		if r == nil {
			return
		}
		r.Walk(func(b *layout.Box) {
			if b.Kind == "table" && b.Laid && fb.tables[b] != nil {
				tables = append(tables, b)
			}
		})
	}
	collect(root)
	for _, m := range fb.modals {
		collect(m)
	}
	for _, t := range tables {
		fb.placeRows(casc, t)
	}
}

// placeRows generates, places, and cascades the visible rows of table t
// (SPEC §6.9.3, §6.9.4). A row is a generated item: the row template's
// classes and truthy guards, :selected on the cursor row, :checked when
// its key is checked, and the key of its row. Its body cells are
// generated texts, one per visible column: the column's cell template
// resolved on the row, the column's classes and its guards truthy on the
// row. They inherit from their row and the table, never from their
// column; a cell whose display is none is dropped and leaves its cell
// blank. Rows and cells get their geometry from the table (they ignore
// their own sizes, borders, and spacing).
func (fb *builder) placeRows(casc *css.Cascade, t *layout.Box) {
	tf := fb.tables[t]
	p, td := tf.plan, tf.data
	var cols []*layout.Box
	for _, col := range layout.TableColumns(t) {
		if _, ok := p.colIndex[col.Src]; ok {
			cols = append(cols, col)
		}
	}
	last := min(t.ScrollY+t.View, td.rows)
	for i := max(t.ScrollY, 0); i < last; i++ {
		var rowOn []bool
		if td.rowOn != nil {
			rowOn = td.rowOn[i]
		}
		row := &layout.Box{
			Tag: "item", Kind: "item", Parent: t, Src: p.item, Follow: -1, Index: i,
			Fixed: true, Disabled: t.Disabled, Mark: t.Mark,
			Classes: guardClasses(p.item, rowOn), GuardOn: rowOn,
			Selected: i == tf.sel, Checked: hasKey(tf.checked, td.keys[i]), Key: Format(td.keys[i]),
		}
		for _, col := range cols {
			j := p.colIndex[col.Src]
			var on []bool
			if td.on[j] != nil {
				on = td.on[j][i]
			}
			row.Children = append(row.Children, &layout.Box{
				Tag: "text", Kind: "text", Parent: row, Src: col.Src, Follow: -1, Index: -1,
				Fixed: true, Disabled: row.Disabled, Text: td.texts[j][i],
				Classes: guardClasses(col.Src, on), GuardOn: on,
			})
		}
		layout.PlaceTableRow(t, row, cols, i)
		computeStyles(casc, row, &t.Style)
		t.Children = append(t.Children, row)
	}
}
