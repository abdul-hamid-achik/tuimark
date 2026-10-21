package layout

import (
	"math/rand"
	"strings"
	"testing"
)

// SPEC 0.4d test 264 and 269: RunLines gives the lines Lines gives, as
// pieces of the joined text, in every wrap mode (decision 209).
func TestRunLinesAreTheLinesOfLines(t *testing.T) {
	r := rand.New(rand.NewSource(209))
	alphabet := []string{"a", "bc", "def", " ", " ", "  ", "\n", "微", "é", "x", "yz", " ", "é", "🇺🇸"}
	modes := []string{"", "nowrap", "wrap", "truncate", "truncate-start", "truncate-middle", ModePreWrap}
	for i := 0; i < 4000; i++ {
		var sb strings.Builder
		for j, k := 0, r.Intn(24); j < k; j++ {
			sb.WriteString(alphabet[r.Intn(len(alphabet))])
		}
		text := sb.String()
		width := r.Intn(9) // 0 included
		for _, mode := range modes {
			want := Lines(text, width, mode)
			got := RunLines(text, width, mode)
			if len(got) != len(want) {
				t.Fatalf("%q at %d, %s: %d lines, Lines gives %d (%q)", text, width, mode, len(got), len(want), want)
			}
			for li, line := range got {
				var b strings.Builder
				for _, p := range line {
					b.WriteString(p.Text)
					// A piece from a run is a substring of text at its Off,
					// but for the joining space, whose Off is the first
					// space it stands for.
					if p.Off >= 0 {
						if p.Text == " " && !strings.HasPrefix(text[p.Off:], " ") {
							if r, _ := firstRune(text[p.Off:]); !isSpaceRune(r) {
								t.Fatalf("%q at %d, %s: the joining space at %d stands for %q", text, width, mode, p.Off, text[p.Off:])
							}
						} else if !strings.HasPrefix(text[p.Off:], p.Text) {
							t.Fatalf("%q at %d, %s: piece %q is not at %d", text, width, mode, p.Text, p.Off)
						}
					}
				}
				if b.String() != want[li] {
					t.Fatalf("%q at %d, %s: line %d is %q, Lines gives %q", text, width, mode, li, b.String(), want[li])
				}
			}
		}
	}
}

func TestRunLinesOffsets(t *testing.T) {
	// Test 264: "Use go build and then check the go.mod file" wrapped at 20.
	text := "Use go build and then check the go.mod file"
	lines := RunLines(text, 20, "wrap")
	if len(lines) != 3 {
		t.Fatalf("%d lines: %v", len(lines), lines)
	}
	// Line 0 is "Use go build and": words at 0, 4, 7, 13, joined by spaces
	// that stand for the offsets 3, 6, 12.
	wantOff := []int{0, 3, 4, 6, 7, 12, 13}
	wantTxt := []string{"Use", " ", "go", " ", "build", " ", "and"}
	if len(lines[0]) != len(wantOff) {
		t.Fatalf("line 0: %v", lines[0])
	}
	for i, p := range lines[0] {
		if p.Off != wantOff[i] || p.Text != wantTxt[i] {
			t.Errorf("line 0 piece %d = %+v, want %q at %d", i, p, wantTxt[i], wantOff[i])
		}
	}

	// Test 269: the ellipsis comes from no run.
	got := RunLines("abcdefgh", 6, "truncate")
	if len(got) != 1 || len(got[0]) != 2 || got[0][0] != (Piece{"abcde", 0}) || got[0][1] != (Piece{"…", -1}) {
		t.Errorf("truncate: %v", got)
	}
	got = RunLines("abcdefgh", 6, "truncate-middle")
	if len(got) != 1 || len(got[0]) != 3 || got[0][0] != (Piece{"abc", 0}) || got[0][1] != (Piece{"…", -1}) || got[0][2] != (Piece{"gh", 6}) {
		t.Errorf("truncate-middle: %v", got)
	}
	got = RunLines("abcdefgh", 6, "truncate-start")
	if len(got) != 1 || len(got[0]) != 2 || got[0][0] != (Piece{"…", -1}) || got[0][1] != (Piece{"defgh", 3}) {
		t.Errorf("truncate-start: %v", got)
	}

	// pre-wrap: one piece per line, at its offset.
	got = RunLines("  a  b", 10, ModePreWrap)
	if len(got) != 1 || len(got[0]) != 1 || got[0][0] != (Piece{"  a  b", 0}) {
		t.Errorf("pre-wrap: %v", got)
	}
	got = RunLines("ab\n\ncd", 10, ModePreWrap)
	if len(got) != 3 || got[2][0] != (Piece{"cd", 4}) {
		t.Errorf("pre-wrap lines: %v", got)
	}
}

func TestTextRunsAt(t *testing.T) {
	r := &TextRuns{Spans: []RunSpan{{End: 4}, {End: 12}, {End: 32}}}
	for _, c := range []struct{ off, want int }{{0, 0}, {3, 0}, {4, 1}, {11, 1}, {12, 2}, {31, 2}, {99, 2}} {
		if got := r.At(c.off); got != c.want {
			t.Errorf("At(%d) = %d, want %d", c.off, got, c.want)
		}
	}
	if (*TextRuns)(nil).At(0) != -1 || (&TextRuns{}).At(0) != -1 {
		t.Error("no run is -1")
	}
}

func TestPiecesWidth(t *testing.T) {
	if got := PiecesWidth([]Piece{{"ab      ", 0}}, ModePreWrap); got != 2 {
		t.Errorf("pre-wrap = %d", got)
	}
	if got := PiecesWidth([]Piece{{"ab", 0}, {" ", 2}, {"微", 3}}, "wrap"); got != 5 {
		t.Errorf("wrap = %d", got)
	}
}

func firstRune(s string) (rune, int) {
	for i, r := range s {
		return r, i
	}
	return 0, 0
}

func isSpaceRune(r rune) bool { return strings.ContainsRune(" \t\n\v\f\r\u0085 ", r) || r == 0x2028 }
