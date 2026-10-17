package ir

import "testing"

func TestEditDistance(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"", "", 0}, {"abc", "abc", 0}, {"", "abc", 3}, {"abc", "", 3},
		{"tex", "text", 1}, {"text", "tetx", 1}, // transposition
		{"colr", "color", 1}, {"kitten", "sitting", 3}, {"bx", "box", 1},
		{"padding", "pading", 1}, {"ab", "ba", 1}, {"abc", "xyz", 3},
	} {
		if got := EditDistance(c.a, c.b); got != c.want {
			t.Errorf("EditDistance(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
		if got := EditDistance(c.b, c.a); got != c.want {
			t.Errorf("EditDistance(%q, %q) = %d, want %d (symmetry)", c.b, c.a, got, c.want)
		}
	}
}

func TestSuggest(t *testing.T) {
	cands := []string{"box", "col", "row", "text", "list", "item"}
	for _, c := range []struct {
		name, want string
	}{
		{"tex", "text"},   // distance 1, len 3: limit is < 2
		{"txet", "text"},  // transposition
		{"bx", "box"},     // len 2: limit is < 2, distance 1
		{"lst", "list"},   // 1
		{"itm", "item"},   // 1
		{"b", ""},         // too short: limit is < 1
		{"div", ""},       // too far
		{"zzzzzz", ""},    // far
		{"box", ""},       // exact: not a typo of itself
		{"colum", "col"},  // distance 2, len 5: limit is < 3
		{"rw", "row"},     // 1
		{"ro", "row"},     // 1
		{"tx", ""},        // distance 2 to text, len 2: limit is < 2
		{"textt", "text"}, // 1
	} {
		if got := Suggest(c.name, cands); got != c.want {
			t.Errorf("Suggest(%q) = %q, want %q", c.name, got, c.want)
		}
	}
	// A tie goes to the first candidate in order.
	if got := Suggest("cax", []string{"cat", "cab", "car"}); got != "cat" {
		t.Errorf("tie-break = %q, want cat", got)
	}
	// A strictly closer later candidate beats an earlier, farther one.
	if got := Suggest("cart", []string{"cert9", "cat", "card"}); got != "cat" {
		t.Errorf("closest = %q", got)
	}
	if got := Suggest("anything", nil); got != "" {
		t.Errorf("no candidates = %q", got)
	}
}
