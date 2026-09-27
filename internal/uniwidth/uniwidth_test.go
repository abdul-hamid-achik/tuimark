package uniwidth

import (
	"math/rand"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/rivo/uniseg"
)

// The normative test vectors of SPEC v0.2 §11.5.1, row by row (§21 test 16).
var vectors = []struct {
	name    string
	text    string
	w       int
	complex bool
}{
	{"a", "a", 1, false},
	{"box drawing (Ambiguous)", "─", 1, false},
	{"ellipsis (Ambiguous)", "…", 1, false},
	{"CJK", "微", 2, true},
	{"emoji", "\U0001F600", 2, true},
	{"heart, no emoji presentation", "❤", 1, false},
	{"heart + VS16", "❤\uFE0F", 2, true},
	{"thumbs up + skin tone", "\U0001F44D\U0001F3FD", 2, true},
	{"family ZWJ sequence", "\U0001F468\u200D\U0001F469\u200D\U0001F467", 2, true},
	{"flag", "\U0001F1FA\U0001F1F8", 2, true},
	{"e + combining acute", "e\u0301", 1, true},
	{"Hangul jamo L V T", "ᄒ\u1161\u11AB", 2, true},
	{"two-em dash (capped)", "⸺", 2, true},
	{"three-em dash (capped)", "⸻", 2, true},
	{"combining acute alone", "\u0301", 0, false},
	{"zero width space", "\u200B", 0, false},
	// rw rule 3: an Extended_Pictographic code point without
	// Emoji_Presentation is 2 when its East_Asian_Width is W or F, as
	// every terminal draws it; a later U+FE0E still makes the cluster 1.
	{"wavy dash (EAW W, text presentation)", "\u3030", 2, true},
	{"wavy dash + VS15", "\u3030\uFE0E", 1, true},
	{"wavy dash + VS16", "\u3030\uFE0F", 2, true},
	{"circled ideograph secret (EAW W)", "\u3299", 2, true},
}

// eawWidePictographs are the Extended_Pictographic code points without
// Emoji_Presentation whose East_Asian_Width is W (Unicode 15.0.0): the
// only code points where W(c) differs from uniseg v0.4.7's width (SPEC
// v0.2 \u00A711.5.1). A cluster they lead counts 2, where uniseg says 1, unless
// a later U+FE0E or U+FE0F sets its width (then both agree).
var eawWidePictographs = []rune{0x3030, 0x303D, 0x3297, 0x3299, 0x1F202, 0x1F237, 0x1F260, 0x1F261, 0x1F262, 0x1F263, 0x1F264, 0x1F265}

// oracleWidth is the \u00A711.5.1 oracle for one cluster: the width uniseg
// reported for it (with EastAsianAmbiguousWidth = 1), capped at 2, and 2
// for a cluster led by one of eawWidePictographs without a variation
// selector.
func oracleWidth(c string, unisegWidth int) int {
	r0, _ := utf8.DecodeRuneInString(c)
	for _, p := range eawWidePictographs {
		if r0 == p && !strings.ContainsAny(c, "\uFE0E\uFE0F") {
			return 2
		}
	}
	return min(unisegWidth, 2)
}

func TestVectors(t *testing.T) {
	for _, v := range vectors {
		cs := Split(v.text)
		if len(cs) != 1 || cs[0] != v.text {
			t.Errorf("%s: %q segments as %q, want one cluster", v.name, v.text, cs)
			continue
		}
		if got := ClusterWidth(v.text); got != v.w {
			t.Errorf("%s: W(%q) = %d, want %d", v.name, v.text, got, v.w)
		}
		if got := Width(v.text); got != v.w {
			t.Errorf("%s: Width(%q) = %d, want %d", v.name, v.text, got, v.w)
		}
		if v.w > 0 {
			if got := Complex(v.text, v.w); got != v.complex {
				t.Errorf("%s: Complex(%q) = %v, want %v", v.name, v.text, got, v.complex)
			}
		}
	}
	// The glyphs the runtime paints itself are simple clusters of width 1.
	for _, g := range []string{"…", "•", "█", "░", "◆", "●", "▸", "┌", "┐", "└", "┘", "─", "│", "╔", "═", "║", "╭", "╯", "┏", "━", "┃"} {
		if ClusterWidth(g) != 1 || Complex(g, 1) {
			t.Errorf("%q must be a simple cluster of width 1", g)
		}
	}
}

func TestWidthCountSplitPrefix(t *testing.T) {
	cases := []struct {
		s     string
		w, n  int
		split string
	}{
		{"", 0, 0, ""},
		{"hello", 5, 5, "h|e|l|l|o"},
		{"ñandú─", 6, 6, "ñ|a|n|d|ú|─"},
		{"微信ok", 6, 4, "微|信|o|k"},
		{"a👍🏽b", 4, 3, "a|👍🏽|b"},
		{"🇺🇸🇫🇷", 4, 2, "🇺🇸|🇫🇷"},
		{"🇺🇸🇫", 4, 2, "🇺🇸|🇫"},
		{"cafe\u0301!", 5, 5, "c|a|f|e\u0301|!"},
		{"\u0301x", 1, 2, "\u0301|x"},
		{"a\tb", 2, 3, "a|\t|b"},
		{"a\r\nb", 2, 3, "a|\r\n|b"},
		{"x\u200By", 2, 3, "x|\u200B|y"},
	}
	for _, c := range cases {
		if got := Width(c.s); got != c.w {
			t.Errorf("Width(%q) = %d, want %d", c.s, got, c.w)
		}
		if got := Count(c.s); got != c.n {
			t.Errorf("Count(%q) = %d, want %d", c.s, got, c.n)
		}
		if got := strings.Join(Split(c.s), "|"); got != c.split {
			t.Errorf("Split(%q) = %q, want %q", c.s, got, c.split)
		}
	}
	for _, c := range []struct {
		s        string
		n        int
		bytes, w int
	}{
		{"hello", 3, 3, 3},
		{"hello", 9, 5, 5},
		{"hello", 0, 0, 0},
		{"hello", -1, 0, 0},
		{"微信ok", 3, 3, 2},      // 信 would straddle column 3
		{"微信ok", 4, 6, 4},      // both CJK clusters
		{"微信ok", 1, 0, 0},      // nothing fits
		{"ab\u200Bc", 2, 5, 2}, // a trailing zero-width cluster joins the prefix
	} {
		b, w := Prefix(c.s, c.n)
		if b != c.bytes || w != c.w {
			t.Errorf("Prefix(%q, %d) = (%d, %d), want (%d, %d)", c.s, c.n, b, w, c.bytes, c.w)
		}
	}
	var got []string
	Each("abc", func(c string, _ int) bool {
		got = append(got, c)
		return len(got) < 2
	})
	if strings.Join(got, "") != "ab" {
		t.Errorf("Each must stop when fn returns false: %q", got)
	}
	if c, rest, w, st := Next("", 7); c != "" || rest != "" || w != 0 || st != 7 {
		t.Error("Next on an empty string")
	}
}

// oracle is the cluster width uniseg v0.4.7 reports with
// EastAsianAmbiguousWidth = 1, capped at 2, with the documented
// exceptions of oracleWidth (SPEC §11.5.1: "Tests may use uniseg as that
// oracle").
func oracle(s string) []int {
	saved := uniseg.EastAsianAmbiguousWidth
	uniseg.EastAsianAmbiguousWidth = 1
	defer func() { uniseg.EastAsianAmbiguousWidth = saved }()
	var out []int
	state := -1
	for s != "" {
		var c string
		var w int
		c, s, w, state = uniseg.FirstGraphemeClusterInString(s, state)
		out = append(out, oracleWidth(c, w))
	}
	return out
}

func widths(s string) []int {
	var out []int
	Each(s, func(_ string, w int) bool {
		out = append(out, w)
		return true
	})
	return out
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Every code point, alone and as the lead of the clusters that exercise
// the W(c) rules, measures as the uniseg oracle says.
func TestOracleEveryCodePoint(t *testing.T) {
	if testing.Short() {
		t.Skip("exhaustive")
	}
	saved := uniseg.EastAsianAmbiguousWidth
	uniseg.EastAsianAmbiguousWidth = 1
	defer func() { uniseg.EastAsianAmbiguousWidth = saved }()
	tails := []string{"", "\uFE0E", "\uFE0F", "\u0301", "\u200D\U0001F600", "\u1161", "\u093F", "\U0001F3FD"}
	// Heads put each code point after another cluster: after an Other lead
	// (ASCII or not), the runtime decides the boundary without uniseg
	// unless the code point is Extend, ZWJ, or SpacingMark.
	heads := []string{"a", "\u5FAE", "\u0600", "\U0001F468\u200D", "\u2500"}
	bad := 0
	for r := rune(0); r <= 0x10FFFF && bad < 20; r++ {
		if r >= 0xD800 && r <= 0xDFFF {
			continue
		}
		var cases []string
		for _, tail := range tails {
			cases = append(cases, string(r)+tail)
		}
		for _, head := range heads {
			cases = append(cases, head+string(r))
		}
		for _, s := range cases {
			var want []int
			state := -1
			for rest := s; rest != ""; {
				var c string
				var w int
				c, rest, w, state = uniseg.FirstGraphemeClusterInString(rest, state)
				want = append(want, oracleWidth(c, w))
			}
			if got := widths(s); !equalInts(got, want) {
				t.Errorf("%U in %+q: widths %v, oracle %v", r, s, got, want)
				bad++
			}
		}
	}
}

// Segmentation (with the ASCII fast path) is uniseg's, and widths match
// the oracle, on random mixes of the scripts Tuimark cares about.
func TestSegmentationMatchesUniseg(t *testing.T) {
	atoms := []string{"a", "Z", " ", "~", "\t", "\n", "\r", "\r\n", "é", "e\u0301", "\u0301", "\u200B", "\u200D",
		"\uFE0F", "\uFE0E", "微", "信", "한", "ᄒ", "\u1161", "\u11AB", "😀", "👍", "\U0001F3FD",
		"\U0001F1FA", "\U0001F1F8", "❤", "…", "─", "⸺", "\u0600", "क", "\u093F", "\u094D", "\xff", "©",
		// East Asian Ambiguous code points of every UTF-8 length, which the
		// runtime hands uniseg only as stand-ins (SPEC v0.2 §11.5.1), and
		// the other Grapheme_Cluster_Break classes.
		"±", "①", "\uFFFD", "\U000F0000", "\U0010FFFD", "\xe4\xb8", "〰", "㊙", "가", "각", "\u11A8", "ᄀ",
		"\u0903", "\U0001F3FB", "\u00AD", "\u2028", "\U000E0001", "\U000E0100", "®", "\u0E33"}
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 5000; i++ {
		var b strings.Builder
		for n := rng.Intn(12); n > 0; n-- {
			b.WriteString(atoms[rng.Intn(len(atoms))])
		}
		checkAgainstUniseg(t, b.String())
	}
	// Clusters longer than the windows Next copies for uniseg (32 and 256
	// bytes), with ambiguous code points before, inside, and after them.
	for _, n := range []int{10, 15, 16, 31, 40, 127, 128, 200, 300} {
		marks := strings.Repeat("\u0301", n)
		for _, s := range []string{"±" + marks + "…", "a" + marks + "±\u0301", "\u0600±" + marks, "…" + marks + "\u093F±", "\xff" + marks + "\xff", "👨\u200D" + strings.Repeat("\U0001F3FB", n/2) + "\u200D👩±"} {
			checkAgainstUniseg(t, s)
		}
	}
}

func checkAgainstUniseg(t *testing.T, s string) {
	t.Helper()
	var want []string
	g := uniseg.NewGraphemes(s)
	for g.Next() {
		want = append(want, g.Str())
	}
	got := Split(s)
	if strings.Join(got, "|") != strings.Join(want, "|") || len(got) != len(want) {
		t.Fatalf("Split(%+q) = %+q, uniseg %+q", s, got, want)
	}
	// Next, step by step with its state, segments the same way.
	var steps []string
	for rest, state := s, -1; rest != ""; {
		var c string
		c, rest, _, state = Next(rest, state)
		steps = append(steps, c)
	}
	if strings.Join(steps, "|") != strings.Join(want, "|") || len(steps) != len(want) {
		t.Fatalf("Next over %+q = %+q, uniseg %+q", s, steps, want)
	}
	ws := widths(s)
	if o := oracle(s); !equalInts(ws, o) {
		t.Fatalf("widths(%+q) = %v, oracle %v", s, ws, o)
	}
	sum := 0
	for _, w := range ws {
		sum += w
	}
	if Width(s) != sum || Count(s) != len(got) {
		t.Fatalf("Width/Count(%+q) = %d/%d, want %d/%d", s, Width(s), Count(s), sum, len(got))
	}
	if !utf8.ValidString(s) {
		return
	}
	for _, c := range got {
		if w := ClusterWidth(c); w < 0 || w > 2 {
			t.Fatalf("W(%+q) = %d", c, w)
		}
	}
}

// §21 test 17: widths do not change when another package sets uniseg's
// process-wide ambiguous width to 2.
func TestAmbiguousWidthGlobalIsIgnored(t *testing.T) {
	texts := []string{"…─│•█░◆●▸", "ñandú", "微信ok", "❤ ❤\uFE0F", "¡±①α", "é", "🇺🇸", "a⸺b"}
	before := make([]int, len(texts))
	for i, s := range texts {
		before[i] = Width(s)
	}
	saved := uniseg.EastAsianAmbiguousWidth
	uniseg.EastAsianAmbiguousWidth = 2
	defer func() { uniseg.EastAsianAmbiguousWidth = saved }()
	for i, s := range texts {
		if got := Width(s); got != before[i] {
			t.Errorf("Width(%q) = %d with EastAsianAmbiguousWidth = 2, %d before", s, got, before[i])
		}
	}
	if uniseg.StringWidth("…") != 2 {
		t.Fatal("the test did not change uniseg's global")
	}
	for _, v := range vectors {
		if got := ClusterWidth(v.text); got != v.w {
			t.Errorf("%s: W = %d with EastAsianAmbiguousWidth = 2, want %d", v.name, got, v.w)
		}
	}
}

// The runtime never reads uniseg.EastAsianAmbiguousWidth (SPEC v0.2
// §11.5.1, ADR 0002): measuring text while another goroutine writes that
// process-wide variable is not a data race. The strings hand uniseg East
// Asian Ambiguous code points (and invalid bytes, which decode to U+FFFD)
// in every position where its width lookup would read the variable. Run
// with -race to check.
func TestUnisegGlobalNeverRead(t *testing.T) {
	texts := []string{
		"±\u0301x", "\u0600…─", "\xff\u0301", "\U000F0000\u093F", "a\u0301…", "①\u200D②", "\uFFFD\u0301",
		"é\u0301 ñ\u0301", "\u0600\U0010FFFD", "…─│•█░◆●▸", "微—…é❤\uFE0F", strings.Repeat("±\u0301", 40),
		"x" + strings.Repeat("\u0301", 70) + "±", "±" + strings.Repeat("\u0301", 200) + "…\u0301",
	}
	want := make([]int, len(texts))
	for i, s := range texts {
		want[i] = Width(s)
	}
	saved := uniseg.EastAsianAmbiguousWidth
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
				uniseg.EastAsianAmbiguousWidth = 1 + i%2
			}
		}
	}()
	for n := 0; n < 200; n++ {
		for i, s := range texts {
			if got := Width(s); got != want[i] {
				t.Errorf("Width(%+q) = %d while uniseg's global changes, %d before", s, got, want[i])
			}
			Split(s)
			Prefix(s, 3)
		}
	}
	close(stop)
	<-done
	uniseg.EastAsianAmbiguousWidth = saved
}

// Every code point in the ambiguous table is one whose uniseg width reads
// EastAsianAmbiguousWidth, and every such code point is in the table; the
// stand-ins the runtime segments in their place never read it.
func TestAmbiguousTable(t *testing.T) {
	if testing.Short() {
		t.Skip("exhaustive")
	}
	saved := uniseg.EastAsianAmbiguousWidth
	defer func() { uniseg.EastAsianAmbiguousWidth = saved }()
	lone := func(r rune, global int) int {
		uniseg.EastAsianAmbiguousWidth = global
		return uniseg.StringWidth(string(r))
	}
	for r := rune(0); r <= 0x10FFFF; r++ {
		if r >= 0xD800 && r <= 0xDFFF {
			continue
		}
		if reads := lone(r, 1) != lone(r, 2); reads != in(ambiguous, r) || reads != standsIn(r) {
			t.Fatalf("%U: uniseg reads the global: %v; in the ambiguous table: %v; standsIn: %v", r, reads, in(ambiguous, r), standsIn(r))
		}
	}
	for n, s := range standIn {
		if s == "" {
			continue
		}
		r, size := utf8.DecodeRuneInString(s)
		if size != n || in(ambiguous, r) || Joiner(r) || RuneWidth(r) == 0 {
			t.Errorf("stand-in %+q for length %d: size %d, ambiguous %v, joiner %v", s, n, size, in(ambiguous, r), Joiner(r))
		}
	}
}

func FuzzSegmentation(f *testing.F) {
	for _, v := range vectors {
		f.Add(v.text)
	}
	f.Add("微信ok👨\u200D👩\u200D👧🇺🇸e\u0301\t\r\n\xff")
	f.Fuzz(func(t *testing.T, s string) {
		checkAgainstUniseg(t, s)
		b, w := Prefix(s, 3)
		if w > 3 || Width(s[:b]) != w || !strings.HasPrefix(s, s[:b]) {
			t.Fatalf("Prefix(%+q, 3) = %d, %d", s, b, w)
		}
	})
}

func BenchmarkWidthASCII(b *testing.B) {
	s := strings.Repeat("hello world ", 8)
	for i := 0; i < b.N; i++ {
		_ = Width(s)
	}
}

func BenchmarkWidthMixed(b *testing.B) {
	s := strings.Repeat("hello 微信 ok 👍🏽 é ", 4)
	for i := 0; i < b.N; i++ {
		_ = Width(s)
	}
}
