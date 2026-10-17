package host

import "testing"

// rowOf answers its first question by a walk of the keys and every later
// one from an index: both give the first row whose key equals v
// (keyEqual), or -1, for keys and values of every kind, a string never
// equal to the number it spells and -0 equal to 0.
func TestRowOfWalkAndIndexAgree(t *testing.T) {
	keys := []any{"a", 1.0, "1", true, nil, "a", 0.0, "true", map[string]any{"k": "v"}, []any{1.0}, -0.0, false, ""}
	values := []any{"a", 1.0, "1", true, nil, 0.0, -0.0, "true", map[string]any{"k": "v"}, []any{1.0}, false, "", "zz", 2.0, "null", map[string]any{}}
	want := func(v any) int {
		for i, k := range keys {
			if keyEqual(k, v) {
				return i
			}
		}
		return -1
	}
	for _, first := range values {
		r := &listRows{keys: keys}
		if got := r.rowOf(first); got != want(first) {
			t.Fatalf("first question %#v: row %d, want %d", first, got, want(first))
		}
		for _, v := range values {
			if got := r.rowOf(v); got != want(v) {
				t.Fatalf("after %#v, %#v: row %d, want %d", first, v, got, want(v))
			}
		}
	}
}
