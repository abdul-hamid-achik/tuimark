// filter: keep a filtered copy of an array in the store, with counts.
//
// Copy this file into your project and change the package name. It depends
// only on github.com/abdul-hamid-achik/tuimark. palette.go calls Filter, so
// copy this file with it.
//
// Markup has no expressions, so it cannot filter a list. Filter registers a
// Derive: the runtime recomputes the result inside every store write, so a
// list bound to it follows the query as the user types, with no handler.
package kit

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/abdul-hamid-achik/tuimark"
)

// Mode is how a query matches.
type Mode int

const (
	// Substring: every word of the query must appear, in any order, in the
	// fields of an item, ignoring case. Items keep their order.
	Substring Mode = iota
	// Fuzzy: the query's letters must appear in order in one field
	// ("dpl" matches "Deploy"). Items are ordered by how tight the match is
	// (an earlier start and fewer gaps score higher), then by their order.
	Fuzzy
)

// FilterOptions configures Filter.
type FilterOptions struct {
	Source string   // path of the array of objects, e.g. "palette.commands"
	Query  string   // path of the query string, e.g. "palette.query"
	Out    string   // path to derive, e.g. "found"; it must not be Source or Query
	Fields []string // fields to match, each a key or a dotted path ("owner.name")
	Mode   Mode
}

// Filter derives Out from Source and Query. Out is an object:
//
//	{"items": [...matching items...], "any": true, "count": 2, "total": 5}
//
// An empty query matches everything. Items are the original objects, so a
// list can show any of their fields. A non-array Source gives no items.
func Filter(app *tuimark.App, o FilterOptions) error {
	if o.Source == "" || o.Query == "" || o.Out == "" {
		return fmt.Errorf("kit: Filter needs Source, Query and Out")
	}
	return app.Derive(o.Out, func(get func(string) (any, bool)) (any, error) {
		q, _ := get(o.Query)
		query, _ := q.(string)
		src, _ := get(o.Source)
		rows, _ := src.([]any)
		items := FilterItems(rows, query, o.Fields, o.Mode)
		return map[string]any{"items": items, "any": len(items) > 0, "count": len(items), "total": len(rows)}, nil
	})
}

// FilterItems is the pure part of Filter: the items of rows that match query
// in the given fields, as Filter returns them. It never returns nil.
func FilterItems(rows []any, query string, fields []string, mode Mode) []any {
	words := strings.Fields(strings.ToLower(query))
	type hit struct {
		item  any
		score int
		idx   int
	}
	var hits []hit
	for i, row := range rows {
		score, ok := 0, true
		if len(words) > 0 {
			switch mode {
			case Fuzzy:
				score, ok = fuzzyScore(row, strings.Join(words, ""), fields)
			default:
				ok = containsAll(row, words, fields)
			}
		}
		if ok {
			hits = append(hits, hit{row, score, i})
		}
	}
	if mode == Fuzzy {
		sort.SliceStable(hits, func(a, b int) bool { return hits[a].score > hits[b].score })
	}
	out := make([]any, len(hits))
	for i, h := range hits {
		out[i] = h.item
	}
	return out
}

// Match reports whether query matches the single string hay in mode.
func Match(mode Mode, query, hay string) bool {
	rows := []any{map[string]any{"v": hay}}
	return len(FilterItems(rows, query, []string{"v"}, mode)) == 1
}

func containsAll(row any, words, fields []string) bool {
	var parts []string
	for _, f := range fields {
		parts = append(parts, strings.ToLower(fieldText(row, f)))
	}
	hay := strings.Join(parts, " ")
	for _, w := range words {
		if !strings.Contains(hay, w) {
			return false
		}
	}
	return true
}

// fuzzyScore is the best score of the query over the fields, and whether any
// field matches. Higher is better.
func fuzzyScore(row any, query string, fields []string) (int, bool) {
	best, found := 0, false
	for _, f := range fields {
		if s, ok := subsequenceScore(strings.ToLower(fieldText(row, f)), []rune(query)); ok && (!found || s > best) {
			best, found = s, true
		}
	}
	return best, found
}

// subsequenceScore matches q as a subsequence of hay. Each matched letter
// scores 10, +8 when it follows the previous one directly, +6 at the start of
// a word, and the whole is reduced by the position of the first match.
func subsequenceScore(hay string, q []rune) (int, bool) {
	score, qi, prev, first := 0, 0, -2, -1
	runes := []rune(hay)
	for i, r := range runes {
		if qi == len(q) {
			break
		}
		if r != q[qi] {
			continue
		}
		score += 10
		if i == prev+1 {
			score += 8
		}
		if i == 0 || !(unicode.IsLetter(runes[i-1]) || unicode.IsDigit(runes[i-1])) {
			score += 6
		}
		if first < 0 {
			first = i
		}
		prev = i
		qi++
	}
	if qi < len(q) {
		return 0, false
	}
	return score - first, true
}

// fieldText is the text of a field of an object: a string as it is, a number
// or a bool formatted, anything else empty. path may be dotted.
func fieldText(row any, path string) string {
	cur := row
	for _, seg := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur = m[seg]
	}
	switch v := cur.(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64) // 1000000, not 1e+06
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case bool:
		return strconv.FormatBool(v)
	}
	return ""
}
