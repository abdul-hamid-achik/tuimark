package layout

import (
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

// SPEC 0.4d test 270: the vectors of §11.5.2 hold for PreWrap.
func TestPreWrapVectors(t *testing.T) {
	cases := []struct {
		in   string
		n    int
		want []string
	}{
		{"    if x:", 10, []string{"    if x:"}},
		{"return  [1,   2,", 10, []string{"return  ", "[1,   2,"}},
		{"a\n\n  b", 10, []string{"a", "", "  b"}},
		{"a\n", 10, []string{"a", ""}},
		{"abcdefghijkl", 5, []string{"abcde", "fghij", "kl"}},
		{"ab      cd", 4, []string{"ab      ", "cd"}},
		{"        deep", 6, []string{"        ", "deep"}},
		{"微信 ok", 4, []string{"微信 ", "ok"}},
		{"x  ", 2, []string{"x  "}},
		{"", 10, nil},
		// The document of test 270 at 12 columns.
		{"def f(x):\n    return  [1,   2,\n            3]", 12,
			[]string{"def f(x):", "    return  ", "[1,   2,", "            ", "3]"}},
		// More.
		{"\n", 5, []string{"", ""}},
		{"   ", 2, []string{"   "}},
		{"abc def", 7, []string{"abc def"}},
		{"abc defg", 7, []string{"abc ", "defg"}},
		{"ab 微信微信微信", 4, []string{"ab ", "微信", "微信", "微信"}},
		{"微", 1, []string{"微"}}, // a cluster wider than n is a piece by itself
	}
	for _, c := range cases {
		got := PreWrap(c.in, c.n)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("PreWrap(%q, %d) = %q, want %q", c.in, c.n, got, c.want)
		}
		if n := preWrapLineCount(c.in, c.n); n != len(c.want) {
			t.Errorf("preWrapLineCount(%q, %d) = %d, want %d", c.in, c.n, n, len(c.want))
		}
	}
}

// The lines of PreWrap, joined with LF, are the text: nothing is added or
// lost, and every line but the hanging ones is at most n columns.
func TestPreWrapKeepsEveryCharacter(t *testing.T) {
	r := rand.New(rand.NewSource(270))
	alphabet := []string{"a", "b", "c", " ", " ", "  ", "\n", "微", "é", "x", "yz"}
	for i := 0; i < 2000; i++ {
		var sb strings.Builder
		for j, k := 0, r.Intn(30); j < k; j++ {
			sb.WriteString(alphabet[r.Intn(len(alphabet))])
		}
		text := sb.String()
		n := 1 + r.Intn(8)
		lines := PreWrap(text, n)
		if text == "" {
			if lines != nil {
				t.Fatalf("PreWrap(%q) = %q", text, lines)
			}
			continue
		}
		// Rejoin: hard lines come back at LF, the pieces of one hard line
		// are concatenated. Count LFs to tell them apart.
		if got, want := strings.Count(strings.Join(lines, ""), "\n"), 0; got != want {
			t.Fatalf("PreWrap(%q, %d) = %q holds an LF", text, n, lines)
		}
		flat := strings.Join(lines, "")
		if flat != strings.ReplaceAll(text, "\n", "") {
			t.Fatalf("PreWrap(%q, %d) = %q loses text", text, n, lines)
		}
		if got, want := len(lines), preWrapLineCount(text, n); got != want {
			t.Fatalf("count %d, lines %d for %q at %d", want, got, text, n)
		}
		if got := Lines(text, n, ModePreWrap); !reflect.DeepEqual(got, lines) {
			t.Fatalf("Lines(%q, %d, pre-wrap) = %q, want %q", text, n, got, lines)
		}
		if got := LineCount(text, n, ModePreWrap); got != len(lines) {
			t.Fatalf("LineCount(%q, %d, pre-wrap) = %d, want %d", text, n, got, len(lines))
		}
		b := &Box{Text: text}
		if got := b.lineCount(n, ModePreWrap); got != len(lines) {
			t.Fatalf("lineCount(%q, %d, pre-wrap) = %d, want %d", text, n, got, len(lines))
		}
		for _, l := range lines {
			// A line is wider than n only when it hangs spaces, or is one
			// cluster wider than n.
			if w := Width(strings.TrimRight(l, " ")); w > n && len([]rune(strings.TrimRight(l, " "))) > 1 {
				t.Fatalf("PreWrap(%q, %d) = %q: line %q is %d wide", text, n, lines, l, w)
			}
		}
	}
}

func TestLineWidthPreWrapLeavesOutTrailingSpaces(t *testing.T) {
	if got := LineWidth("ab      ", ModePreWrap); got != 2 {
		t.Errorf("LineWidth pre-wrap = %d, want 2", got)
	}
	if got := LineWidth("ab      ", "wrap"); got != 8 {
		t.Errorf("LineWidth wrap = %d, want 8", got)
	}
	if got := LineWidth("    ", ModePreWrap); got != 0 {
		t.Errorf("LineWidth of spaces = %d, want 0", got)
	}
}
