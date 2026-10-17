package layout

import (
	"math/rand"
	"strings"
	"testing"
)

// TestLineCountMatchesLines checks LineCount against len(Lines) and
// MaxWidth against the widest line of Lines, for every mode, over fixed
// cases and random text mixing ASCII, spaces, newlines, wide and
// combining clusters, and control bytes.
func TestLineCountMatchesLines(t *testing.T) {
	cases := []string{
		"", "a", " ", "\n", "\n\n", "a\n", "\na", "a b c", "  lead", "trail  ",
		"averyveryverylongword", "a  b   c", "tab\there", "日本語のテキスト", "éé",
		"mixed 日本 text\nsecond line here\n\nfourth", "\x01ctl", "x\ty z", strings.Repeat("ab ", 40),
	}
	r := rand.New(rand.NewSource(7))
	alphabet := []string{"a", "b", "c", " ", " ", "\n", "日", "é", "é", "\t", "-", "😀", "xyzzy", "\x7f"}
	for i := 0; i < 2000; i++ {
		var sb strings.Builder
		for j := r.Intn(30); j > 0; j-- {
			sb.WriteString(alphabet[r.Intn(len(alphabet))])
		}
		cases = append(cases, sb.String())
	}
	for _, s := range cases {
		for _, mode := range []string{"", "nowrap", "wrap", "truncate", "truncate-start", "truncate-middle"} {
			for _, w := range []int{-1, 0, 1, 2, 3, 5, 8, 13, 40} {
				if got, want := LineCount(s, w, mode), len(Lines(s, w, mode)); got != want {
					t.Fatalf("LineCount(%q, %d, %q) = %d, want %d", s, w, mode, got, want)
				}
				b := &Box{Text: s}
				if got, want := b.lineCount(w, mode), len(Lines(s, w, mode)); got != want {
					t.Fatalf("Box.lineCount(%q, %d, %q) = %d, want %d", s, w, mode, got, want)
				}
			}
		}
		want := 0
		for _, l := range Lines(s, 0, "") {
			want = max(want, Width(l))
		}
		if got := MaxWidth(s); got != want {
			t.Fatalf("MaxWidth(%q) = %d, want %d", s, got, want)
		}
		if got := (&Box{Text: s}).textInfo().w; got != want {
			t.Fatalf("textInfo(%q).w = %d, want %d", s, got, want)
		}
	}
}

// TestTextInfoFollowsText checks that a box measures its text again when
// Text changes, also to a string of the same length.
func TestTextInfoFollowsText(t *testing.T) {
	b := &Box{Text: "abc"}
	if w := b.textInfo().w; w != 3 {
		t.Fatalf("w = %d, want 3", w)
	}
	b.Text = "a\nb"
	if ti := b.textInfo(); ti.w != 1 || ti.paras != 2 {
		t.Fatalf("after a change: w = %d, paras = %d, want 1, 2", ti.w, ti.paras)
	}
	b.Text = "日本"
	if ti := b.textInfo(); ti.w != 4 || ti.ascii {
		t.Fatalf("after a change: w = %d, ascii = %v, want 4, false", ti.w, ti.ascii)
	}
}
