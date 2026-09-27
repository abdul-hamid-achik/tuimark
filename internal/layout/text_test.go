package layout

import (
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/uniwidth"
)

// SPEC v0.2 §11.5.2 and §21 test 20: measuring, cutting, truncating, and
// wrapping work by grapheme cluster and never split one.
func TestGraphemeCutTruncate(t *testing.T) {
	for _, c := range []struct {
		s     string
		n     int
		cut   string
		trunc string
	}{
		{"微信ok", 6, "微信ok", "微信ok"},
		{"微信ok", 5, "微信o", "微信…"},
		{"微信ok", 4, "微信", "微…"}, // §21 test 20: 3 columns
		{"微信ok", 3, "微", "微…"},
		{"微信ok", 2, "微", "…"}, // 信 would straddle column 2
		{"微信ok", 1, "", "…"},  // nothing fits in one column
		{"微信ok", 0, "", ""},
		{"👍🏽👍🏽x", 3, "👍🏽", "👍🏽…"},
		{"👨\u200D👩\u200D👧 family", 4, "👨\u200D👩\u200D👧 f", "👨\u200D👩\u200D👧 …"},
		{"👨\u200D👩\u200D👧 family", 2, "👨\u200D👩\u200D👧", "…"},
		{"🇺🇸🇫🇷🇩🇪", 5, "🇺🇸🇫🇷", "🇺🇸🇫🇷…"},
		{"cafe\u0301 au lait", 4, "cafe\u0301", "caf…"},
		{"e\u0301e\u0301e\u0301", 2, "e\u0301e\u0301", "e\u0301…"},
		{"ᄒ\u1161\u11ABᄒ\u1161\u11AB", 3, "ᄒ\u1161\u11AB", "ᄒ\u1161\u11AB…"},
		{"ab\u200Bcd", 2, "ab\u200B", "a…"},
		{"\u200B", 0, "", "\u200B"}, // zero width fits in zero columns
	} {
		if got := Cut(c.s, c.n); got != c.cut {
			t.Errorf("Cut(%q, %d) = %q, want %q", c.s, c.n, got, c.cut)
		}
		if got := Truncate(c.s, c.n); got != c.trunc {
			t.Errorf("Truncate(%q, %d) = %q, want %q", c.s, c.n, got, c.trunc)
		}
		if got := Cut(c.s, c.n); c.n > 0 && Width(got) > c.n {
			t.Errorf("Cut(%q, %d) is %d columns", c.s, c.n, Width(got))
		}
	}
	for _, c := range []struct {
		s string
		w int
	}{
		{"微信ok", 6}, {"😀", 2}, {"❤", 1}, {"❤\uFE0F", 2}, {"👍🏽", 2},
		{"👨\u200D👩\u200D👧", 2}, {"🇺🇸", 2}, {"e\u0301", 1}, {"ᄒ\u1161\u11AB", 2},
		{"⸺", 2}, {"\u0301", 0}, {"\u200B", 0}, {"…─│", 3},
	} {
		if got := Width(c.s); got != c.w {
			t.Errorf("Width(%q) = %d, want %d", c.s, got, c.w)
		}
	}
	if MaxWidth("微信\nabc\n😀😀😀") != 6 {
		t.Errorf("MaxWidth = %d", MaxWidth("微信\nabc\n😀😀😀"))
	}
}

func TestGraphemeWrap(t *testing.T) {
	for _, c := range []struct {
		p     string
		width int
		want  string
	}{
		{"微信微信", 3, "微|信|微|信"}, // §21 test 20
		{"微", 1, "微"},          // a cluster wider than the width is a piece by itself
		{"微信", 1, "微|信"},
		{"微信微信", 4, "微信|微信"},
		{"ab 微信 cd", 5, "ab|微信|cd"},
		{"ab 微 cd", 5, "ab 微|cd"},
		{"👍🏽👍🏽👍🏽", 5, "👍🏽👍🏽|👍🏽"},
		{"👨\u200D👩\u200D👧👨\u200D👩\u200D👧", 3, "👨\u200D👩\u200D👧|👨\u200D👩\u200D👧"},
		{"🇺🇸🇫🇷🇩🇪", 3, "🇺🇸|🇫🇷|🇩🇪"},
		{"e\u0301e\u0301e\u0301", 2, "e\u0301e\u0301|e\u0301"},
		{"cafe\u0301 cafe\u0301", 4, "cafe\u0301|cafe\u0301"},
		{"微信", 0, "微|信"}, // a non-positive width still makes progress
	} {
		got := strings.Join(Wrap(c.p, c.width), "|")
		if got != c.want {
			t.Errorf("Wrap(%q, %d) = %q, want %q", c.p, c.width, got, c.want)
		}
	}
	if got := strings.Join(Lines("微信ok\n😀x", 3, "truncate"), "|"); got != "微…|😀x" {
		t.Errorf("Lines truncate = %q", got)
	}
	if got := strings.Join(Lines("微信 ok\nab", 4, "wrap"), "|"); got != "微信|ok|ab" {
		t.Errorf("Lines wrap = %q", got)
	}
}

// checkWrap asserts the Wrap invariants: no line is wider than the width
// unless it is one cluster, no cluster is split, and the words come back in
// order.
func checkWrap(t *testing.T, p string, width int) {
	t.Helper()
	lines := Wrap(p, width)
	if len(lines) == 0 {
		t.Fatalf("Wrap(%+q, %d) returned no line", p, width)
	}
	var clusters []string
	for _, l := range lines {
		if Width(l) > width && uniwidth.Count(l) != 1 {
			t.Fatalf("Wrap(%+q, %d): line %+q is %d columns", p, width, l, Width(l))
		}
		for _, f := range strings.Fields(l) {
			clusters = append(clusters, uniwidth.Split(f)...)
		}
	}
	var want []string
	for _, f := range strings.Fields(p) {
		want = append(want, uniwidth.Split(f)...)
	}
	if strings.Join(clusters, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("Wrap(%+q, %d) = %+q splits or loses clusters", p, width, lines)
	}
}

func FuzzWrap(f *testing.F) {
	f.Add("微信微信 ab 👨\u200D👩\u200D👧 e\u0301 🇺🇸🇫🇷", 3)
	f.Add("aaa bbb ccc", 7)
	f.Add("\u0301\u0301 x", 1)
	f.Fuzz(func(t *testing.T, p string, width int) {
		if width < 1 || width > 40 || len(p) > 200 || strings.ContainsFunc(p, func(r rune) bool { return r == '\n' }) {
			return
		}
		checkWrap(t, p, width)
		cut := Cut(p, width)
		if Width(cut) > width || !strings.HasPrefix(p, cut) {
			t.Fatalf("Cut(%+q, %d) = %+q", p, width, cut)
		}
		if tr := Truncate(p, width); Width(tr) > width {
			t.Fatalf("Truncate(%+q, %d) = %+q is %d columns", p, width, tr, Width(tr))
		}
	})
}
