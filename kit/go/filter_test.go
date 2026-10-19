package kit

import (
	"encoding/json"
	"testing"
)

const filterRows = `[
  {"id": "deploy",   "label": "Deploy to production",      "meta": {"team": "ops"}, "n": 3},
  {"id": "rollback", "label": "Roll back the last deploy", "meta": {"team": "ops"}, "n": 12},
  {"id": "logs",     "label": "Open the logs",             "meta": {"team": "dev"}, "n": 7},
  {"id": "theme",    "label": "Toggle the theme",          "meta": {"team": "ui"},  "n": 1}
]`

func rows(t *testing.T) []any {
	t.Helper()
	var v []any
	if err := json.Unmarshal([]byte(filterRows), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func ids(items []any) []string {
	out := []string{}
	for _, it := range items {
		out = append(out, it.(map[string]any)["id"].(string))
	}
	return out
}

func TestSubstringFilter(t *testing.T) {
	r := rows(t)
	fields := []string{"label", "id"}
	for _, c := range []struct {
		query string
		want  []string
	}{
		{"", []string{"deploy", "rollback", "logs", "theme"}},
		{"DEPLOY", []string{"deploy", "rollback"}},
		{"the", []string{"rollback", "logs", "theme"}},
		{"back deploy", []string{"rollback"}}, // every word, any order
		{"deploy back", []string{"rollback"}},
		{"zzz", []string{}},
		{"  ", []string{"deploy", "rollback", "logs", "theme"}},
	} {
		if got := ids(FilterItems(r, c.query, fields, Substring)); !equal(got, c.want) {
			t.Errorf("substring %q = %v, want %v", c.query, got, c.want)
		}
	}
	// A dotted field and a numeric field.
	if got := ids(FilterItems(r, "dev", []string{"meta.team"}, Substring)); !equal(got, []string{"logs"}) {
		t.Errorf("dotted field: %v", got)
	}
	if got := ids(FilterItems(r, "12", []string{"n"}, Substring)); !equal(got, []string{"rollback"}) {
		t.Errorf("numeric field: %v", got)
	}
	if got := FilterItems(nil, "x", fields, Substring); got == nil || len(got) != 0 {
		t.Errorf("no rows should give an empty, non-nil slice: %#v", got)
	}
}

func TestFuzzyFilterOrdersByTightness(t *testing.T) {
	r := rows(t)
	fields := []string{"label"}
	// "dpl" is a subsequence of "Deploy to production" and of "Roll back the
	// last deploy"; the first starts earlier and matches a word start.
	if got := ids(FilterItems(r, "dpl", fields, Fuzzy)); !equal(got, []string{"deploy", "rollback"}) {
		t.Errorf("fuzzy dpl = %v", got)
	}
	// Consecutive letters beat scattered ones: "log" is solid in "Open the
	// logs" and scattered across "Roll back the last deploy".
	if got := ids(FilterItems(r, "log", fields, Fuzzy)); got[0] != "logs" {
		t.Errorf("fuzzy log = %v, want logs first", got)
	}
	if got := ids(FilterItems(r, "zq", fields, Fuzzy)); len(got) != 0 {
		t.Errorf("fuzzy zq = %v", got)
	}
	// Letters must come in order.
	if Match(Fuzzy, "ba", "abc") {
		t.Error(`"ba" is not a subsequence of "abc"`)
	}
	if !Match(Fuzzy, "ac", "abc") || !Match(Substring, "BC", "abc") {
		t.Error("ac / bc should match")
	}
}

func TestFilterDerivesItemsAnyCountAndTotal(t *testing.T) {
	app := bareApp(t)
	_ = app.Bind("", map[string]any{"q": "", "items": rows(t)})
	if err := Filter(app, FilterOptions{Source: "items", Query: "q", Out: "found", Fields: []string{"label"}}); err != nil {
		t.Fatal(err)
	}
	read := func() map[string]any {
		v, _ := app.Get("found")
		return v.(map[string]any)
	}
	if got := read(); got["count"] != 4.0 || got["total"] != 4.0 || got["any"] != true {
		t.Errorf("no query: %v", got)
	}
	// The query is a store path: setting it re-derives with no handler.
	_ = app.Set("q", "deploy")
	if got := read(); got["count"] != 2.0 || got["total"] != 4.0 {
		t.Errorf("query deploy: %v", got)
	}
	_ = app.Set("q", "nope")
	if got := read(); got["any"] != false || len(got["items"].([]any)) != 0 {
		t.Errorf("query nope: %v", got)
	}
	// A change of the source re-derives too.
	_ = app.Set("items", []any{map[string]any{"id": "nope-1", "label": "nope one"}})
	if got := read(); got["count"] != 1.0 || got["total"] != 1.0 {
		t.Errorf("new source: %v", got)
	}
}

func TestFilterNeedsItsPaths(t *testing.T) {
	if err := Filter(bareApp(t), FilterOptions{Source: "a"}); err == nil {
		t.Error("Filter without Query and Out should fail")
	}
}
