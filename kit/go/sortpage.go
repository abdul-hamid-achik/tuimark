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
	"sort"
	"strings"
	"sync"

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

	mu    sync.Mutex // guards cache: a Derive function may run from any goroutine
	cache sortCache
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
	sp.mu.Lock()
	perm, _ := sortPerm(all, st.col, st.desc, &sp.cache)
	sp.mu.Unlock()
	total := len(all)
	pages := max(1, (total+st.size-1)/st.size)
	page := min(max(st.page, 1), pages)
	lo, hi := (page-1)*st.size, min(page*st.size, total)
	out := make([]any, 0, hi-lo)
	for _, p := range perm[lo:hi] {
		out = append(out, all[p])
	}
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
// page goes back to 1. A name that is not one of the columns does nothing and
// returns nil, so a keymap row or a click on something else can share the
// handler (an error from a handler would end Run).
func (sp *SortPage) Cycle(col string) error {
	known := false
	for _, c := range sp.opts.Columns {
		known = known || c.Key == col
	}
	if !known {
		return nil
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

// NextPage moves one page forward, and stops at the last.
func (sp *SortPage) NextPage() error { return sp.step(1) }

// PrevPage moves one page back, and stops at the first.
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
	perm, _ := sortPerm(rows, key, desc, nil)
	out := make([]any, len(rows))
	for i, p := range perm {
		out[i] = rows[p]
	}
	return out
}

// sortKey is the sortable value of one row, extracted once per sort.
type sortKey struct {
	rank int8 // 0 number, 1 boolean, 2 text, 3 missing
	n    float64
	s    string
}

// sortCache remembers the order the last sort produced. The keys are
// compared before the cache is trusted, so rows that changed in a way that
// matters to the order are sorted again, and rows that changed in other fields
// keep their order for free.
type sortCache struct {
	key  string
	desc bool
	keys []sortKey
	perm []int
}

func extractKeys(rows []any, key string) []sortKey {
	segs := strings.Split(key, ".")
	keys := make([]sortKey, len(rows))
	for i, row := range rows {
		keys[i] = sortKey{rank: 3}
		cur := row
		for _, seg := range segs {
			m, ok := cur.(map[string]any)
			if !ok {
				cur = nil
				break
			}
			cur = m[seg]
		}
		switch v := cur.(type) {
		case float64:
			keys[i] = sortKey{rank: 0, n: v}
		case bool:
			keys[i] = sortKey{rank: 1}
			if v {
				keys[i].n = 1
			}
		case string:
			keys[i] = sortKey{rank: 2, s: strings.ToLower(v)}
		}
	}
	return keys
}

// sortPerm is the order of rows (as indexes) for key, using and updating cache
// when it is not nil.
func sortPerm(rows []any, key string, desc bool, cache *sortCache) ([]int, bool) {
	perm := make([]int, len(rows))
	for i := range perm {
		perm[i] = i
	}
	if key == "" {
		return perm, true
	}
	keys := extractKeys(rows, key)
	if cache != nil && cache.key == key && cache.desc == desc && len(cache.keys) == len(keys) {
		same := true
		for i := range keys {
			if keys[i] != cache.keys[i] {
				same = false
				break
			}
		}
		if same {
			return cache.perm, true
		}
	}
	sort.SliceStable(perm, func(i, j int) bool {
		a, b := keys[perm[i]], keys[perm[j]]
		if a.rank == 3 || b.rank == 3 {
			return a.rank != 3 && b.rank == 3 // missing goes last
		}
		c := compareKeys(a, b)
		if desc {
			return c > 0
		}
		return c < 0
	})
	if cache != nil {
		*cache = sortCache{key: key, desc: desc, keys: keys, perm: perm}
	}
	return perm, false
}

func compareKeys(a, b sortKey) int {
	if a.rank != b.rank {
		return int(a.rank) - int(b.rank)
	}
	if a.rank == 2 {
		return strings.Compare(a.s, b.s)
	}
	switch {
	case a.n < b.n:
		return -1
	case a.n > b.n:
		return 1
	}
	return 0
}
