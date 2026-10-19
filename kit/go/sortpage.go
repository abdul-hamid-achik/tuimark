// sortpage: sort by a column (click to cycle) and paginate, for a table.
//
// Copy this file into your project and change the package name. It depends
// only on github.com/abdul-hamid-achik/tuimark.
//
// A table shows the array it is given. SortPage derives two paths from the
// full array and a small state object, so the table's `each` reads the sorted
// rows of the current page and its column titles read the arrows:
//
//	<table each="svc.rows as s" ...>
//	  <column id="svc__name" title="{svc.meta.titles.name}" on:click="table_sort">{s.name}</column>
//	<text>{svc.meta.label}</text>          1–50 of 340
//
// Out gets the rows; Meta gets {page, pages, size, total, label, has_prev,
// has_next, titles}. The state ({col, desc, page, size}) is a store object
// you can also write yourself.
package kit

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/abdul-hamid-achik/tuimark"
)

// Column is a sortable column: the field of each row it sorts by, and the
// title the header shows. A header with no sort shows the Title alone; the
// sorted one shows "Title ▲" (ascending) or "Title ▼" (descending).
type Column struct {
	Key   string
	Title string
}

// SortPageOptions configures NewSortPage.
type SortPageOptions struct {
	Source  string // path of the full array of objects
	State   string // path of the state object (default Out + "_state")
	Out     string // path to derive: the sorted rows of the current page
	Meta    string // path to derive: the page's numbers, label and column titles
	Columns []Column
	Size    int // page size when the state has none (default 50)
}

// SortPage is the sort and page state of one table.
type SortPage struct {
	app  *tuimark.App
	opts SortPageOptions
}

const (
	arrowUp   = "▲"
	arrowDown = "▼"
)

// NewSortPage registers the derived values and returns the controller.
func NewSortPage(app *tuimark.App, o SortPageOptions) (*SortPage, error) {
	if o.Source == "" || o.Out == "" || o.Meta == "" {
		return nil, fmt.Errorf("kit: SortPage needs Source, Out and Meta")
	}
	if o.State == "" {
		o.State = o.Out + "_state"
	}
	if o.Size <= 0 {
		o.Size = 50
	}
	sp := &SortPage{app: app, opts: o}
	err := errors.Join(
		app.Derive(o.Out, func(get func(string) (any, bool)) (any, error) {
			rows, _ := sp.view(get)
			return rows, nil
		}),
		app.Derive(o.Meta, func(get func(string) (any, bool)) (any, error) {
			_, meta := sp.view(get)
			return meta, nil
		}),
	)
	if err != nil {
		return nil, err
	}
	return sp, nil
}

type sortState struct {
	col  string
	desc bool
	page int
	size int
}

func (sp *SortPage) state(get func(string) (any, bool)) sortState {
	st := sortState{page: 1, size: sp.opts.Size}
	v, _ := get(sp.opts.State)
	m, _ := v.(map[string]any)
	st.col, _ = m["col"].(string)
	st.desc, _ = m["desc"].(bool)
	if n, ok := m["page"].(float64); ok && n >= 1 {
		st.page = int(n)
	}
	if n, ok := m["size"].(float64); ok && n >= 1 {
		st.size = int(n)
	}
	return st
}

// view is the current page's rows and the meta object.
func (sp *SortPage) view(get func(string) (any, bool)) ([]any, map[string]any) {
	st := sp.state(get)
	src, _ := get(sp.opts.Source)
	all, _ := src.([]any)
	rows := SortRows(all, st.col, st.desc)
	total := len(rows)
	pages := max(1, (total+st.size-1)/st.size)
	page := min(max(st.page, 1), pages)
	lo, hi := (page-1)*st.size, min(page*st.size, total)
	out := make([]any, 0, hi-lo)
	out = append(out, rows[lo:hi]...)
	titles := map[string]any{}
	for _, c := range sp.opts.Columns {
		titles[c.Key] = c.Title
		if c.Key == st.col {
			titles[c.Key] = c.Title + " " + arrow(st.desc)
		}
	}
	return out, map[string]any{
		"page": page, "pages": pages, "size": st.size, "total": total,
		"label": PageLabel(page, st.size, total), "has_prev": page > 1, "has_next": page < pages,
		"titles": titles,
	}
}

func arrow(desc bool) string {
	if desc {
		return arrowDown
	}
	return arrowUp
}

// PageLabel is "first–last of total" (an en dash), or "0 of 0" when empty.
func PageLabel(page, size, total int) string {
	if total == 0 {
		return "0 of 0"
	}
	lo := (page-1)*size + 1
	hi := min(page*size, total)
	return fmt.Sprintf("%d–%d of %d", lo, hi, total)
}

// Cycle is the click on a column header: a new column sorts ascending, the
// same column again descending, a third time back to the source order. The
// page goes back to 1.
func (sp *SortPage) Cycle(col string) error {
	known := false
	for _, c := range sp.opts.Columns {
		known = known || c.Key == col
	}
	if !known {
		return fmt.Errorf("kit: %q is not a column of %s", col, sp.opts.Out)
	}
	st := sp.state(sp.app.Get)
	switch {
	case st.col != col:
		st.col, st.desc = col, false
	case !st.desc:
		st.desc = true
	default:
		st.col, st.desc = "", false
	}
	st.page = 1
	return sp.write(st)
}

// SetPage goes to a page; the view clamps it to the pages that exist.
func (sp *SortPage) SetPage(n int) error {
	st := sp.state(sp.app.Get)
	st.page = max(n, 1)
	return sp.write(st)
}

// NextPage and PrevPage move one page, and stop at the ends.
func (sp *SortPage) NextPage() error { return sp.step(1) }
func (sp *SortPage) PrevPage() error { return sp.step(-1) }

func (sp *SortPage) step(d int) error {
	meta, _ := sp.app.Get(sp.opts.Meta)
	m, _ := meta.(map[string]any)
	page, _ := m["page"].(float64)
	pages, _ := m["pages"].(float64)
	next := min(max(int(page)+d, 1), max(int(pages), 1))
	return sp.SetPage(next)
}

// SetSize changes the page size and goes back to page 1.
func (sp *SortPage) SetSize(n int) error {
	st := sp.state(sp.app.Get)
	st.size, st.page = max(n, 1), 1
	return sp.write(st)
}

func (sp *SortPage) write(st sortState) error {
	return sp.app.Set(sp.opts.State, map[string]any{
		"col": st.col, "desc": st.desc, "page": st.page, "size": st.size,
	})
}

// SortRows is a copy of rows sorted by the field key (stable). Numbers sort
// before text and compare numerically, text compares ignoring case, booleans
// put false first, and a row without the field goes last in either direction.
// An empty key keeps the order.
func SortRows(rows []any, key string, desc bool) []any {
	out := append([]any(nil), rows...)
	if key == "" {
		return out
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, aok := sortField(out[i], key)
		b, bok := sortField(out[j], key)
		switch {
		case !aok || !bok:
			return aok && !bok // missing goes last
		case desc:
			return compareSort(a, b) > 0
		}
		return compareSort(a, b) < 0
	})
	return out
}

func sortField(row any, key string) (any, bool) {
	cur := row
	for _, seg := range strings.Split(key, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur = m[seg]
	}
	switch v := cur.(type) {
	case float64, bool:
		return v, true
	case string:
		return strings.ToLower(v), true
	}
	return nil, false
}

// compareSort orders numbers, then booleans, then text.
func compareSort(a, b any) int {
	rank := func(v any) int {
		switch v.(type) {
		case float64:
			return 0
		case bool:
			return 1
		}
		return 2
	}
	if ra, rb := rank(a), rank(b); ra != rb {
		return ra - rb
	}
	switch x := a.(type) {
	case float64:
		y := b.(float64)
		if math.IsNaN(x) || math.IsNaN(y) {
			return 0
		}
		switch {
		case x < y:
			return -1
		case x > y:
			return 1
		}
	case bool:
		y := b.(bool)
		if x != y {
			if !x {
				return -1
			}
			return 1
		}
	case string:
		return strings.Compare(x, b.(string))
	}
	return 0
}
