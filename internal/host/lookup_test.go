package host

import (
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// splitLookup is lookup as it was written over strings.Split, the
// reference lookup's segment walk must agree with.
func splitLookup(root any, path string) (any, bool) {
	if path == "" {
		return root, true
	}
	cur := root
	for _, seg := range strings.Split(path, ".") {
		switch c := cur.(type) {
		case map[string]any:
			v, ok := c[seg]
			if !ok {
				return nil, false
			}
			cur = v
		case []any:
			i, err := strconv.Atoi(seg)
			if err != nil || i < 0 || i >= len(c) {
				return nil, false
			}
			cur = c[i]
		default:
			return nil, false
		}
	}
	return cur, true
}

// lookup visits the segments strings.Split gives, empty ones included
// (a leading, trailing, or doubled dot names the member "").
func TestLookupWalksTheSegmentsSplitGives(t *testing.T) {
	store := map[string]any{
		"rows": []any{map[string]any{"name": "a", "": "empty"}, map[string]any{"n": 2.0}},
		"":     map[string]any{"x": "under empty", "": "empty in empty"},
		"cfg":  map[string]any{"title": "t", "deep": map[string]any{"": map[string]any{"k": true}}},
		"num":  1.0,
	}
	for _, p := range []string{
		"rows", "rows.0", "rows.0.name", "rows.1.n", "rows.2", "rows.-1", "rows.x", "rows.0.",
		".x", ".", "..", "cfg.deep..k", "cfg.deep.", "cfg..title", "num.x", "missing", "rows.01.name",
		"rows.0.name.more", "cfg.title", "a.b.c.d",
	} {
		gv, gok := lookup(store, p)
		wv, wok := splitLookup(store, p)
		if gok != wok || !reflect.DeepEqual(gv, wv) {
			t.Errorf("lookup(%q) = %v, %v; want %v, %v", p, gv, gok, wv, wok)
		}
	}
}
