package paint

import (
	"strings"
	"testing"
)

// linked paints one row: two linked texts (the second with a wide cluster)
// around one unlinked, like a <text href> beside plain content.
func linked(t *testing.T) *Grid {
	t.Helper()
	a := tx("ab", "")
	a.Href = "https://example.com/a"
	w := tx("微x", "")
	w.Href = "https://example.com/a" // same href: one shared id
	c := tx("mid", "")
	root := bx("col", "", bx("row", "height: 1", a, c, w))
	return render(t, root, 10, 1)
}

func TestHrefPaintsIntoCells(t *testing.T) {
	g := linked(t)
	if len(g.Links) != 1 || g.Links[0] != (GridLink{ID: "l1", URL: "https://example.com/a"}) {
		t.Fatalf("Links = %+v, want one l1", g.Links)
	}
	// "ab" linked, "mid" not, the wide cluster and its continuation linked.
	if l := g.At(0, 0).Link; l != 1 {
		t.Errorf("cell 0,0 link = %d, want 1", l)
	}
	if l := g.At(3, 0).Link; l != 0 {
		t.Errorf("unlinked cell 3,0 link = %d, want 0", l)
	}
	w := 5 // "abmid" is 5 columns; the wide cluster leads at 5
	if !g.At(w, 0).Wide || g.At(w, 0).Ch != '微' {
		t.Fatalf("wide lead at 5 = %+v", g.At(w, 0))
	}
	if l := g.At(w+1, 0).Link; l != 1 {
		t.Errorf("continuation link = %d, want its lead's 1", l)
	}
}

// Two hrefs get ids in paint order; the same layout painted twice gives the
// same table, so a frame's link ids are stable across diffs.
func TestHrefIDsStableAndOrdered(t *testing.T) {
	a, b := tx("a", ""), tx("b", "")
	a.Href, b.Href = "https://x/1", "https://x/2"
	root := bx("col", "", bx("row", "height: 1", a, b))
	g := render(t, root, 4, 1)
	want := []GridLink{{ID: "l1", URL: "https://x/1"}, {ID: "l2", URL: "https://x/2"}}
	if len(g.Links) != 2 || g.Links[0] != want[0] || g.Links[1] != want[1] {
		t.Fatalf("Links = %+v, want %+v", g.Links, want)
	}
	g2 := render(t, root, 4, 1)
	if len(g2.Links) != 2 || g2.Links[0] != want[0] || g2.Links[1] != want[1] {
		t.Errorf("second frame Links = %+v, want the same %+v", g2.Links, want)
	}
	if g2.At(0, 0).Link != 1 || g2.At(1, 0).Link != 2 {
		t.Errorf("second frame cell links = %d,%d, want 1,2", g2.At(0, 0).Link, g2.At(1, 0).Link)
	}
}

func TestFullWithLinks(t *testing.T) {
	g := NewGrid(3, 1)
	g.Links = []GridLink{{ID: "l1", URL: "https://x"}}
	g.At(0, 0).Ch, g.At(0, 0).Link = 'a', 1
	g.At(1, 0).Ch, g.At(1, 0).Link = 'b', 1
	g.At(2, 0).Ch = 'c'
	open := "\x1b]8;id=l1;https://x\x1b\\"
	want := AutowrapOff +
		"\x1b[0;39;49m" + open + "ab" + // SGR, then the open, then the run
		"\x1b[0;39;49m" + osc8Close + "c" + // a link change is a style change (sameStyle)
		"\x1b[0m\n" + AutowrapOn
	if got := FullWith(g, Options{}); got != want {
		t.Errorf("FullWith = %q, want %q", got, want)
	}
	// A link open at the end of a line is closed before the line's reset.
	g2 := NewGrid(1, 1)
	g2.Links = g.Links
	g2.At(0, 0).Ch, g2.At(0, 0).Link = 'z', 1
	if got := FullWith(g2, Options{}); !strings.Contains(got, "z"+osc8Close+"\x1b[0m") {
		t.Errorf("line-end close missing: %q", got)
	}
	// NoLinks strips every OSC 8 byte.
	if got := FullWith(g, Options{NoLinks: true}); strings.Contains(got, "\x1b]8") {
		t.Errorf("NoLinks emitted a link: %q", got)
	}
}

func TestDiffWithLinks(t *testing.T) {
	next := NewGrid(3, 1)
	next.Links = []GridLink{{ID: "l1", URL: "https://x"}}
	next.At(0, 0).Ch, next.At(0, 0).Link = 'a', 1
	next.At(1, 0).Ch, next.At(1, 0).Link = 'b', 1
	next.At(2, 0).Ch = 'c'
	// The unlinked neighbor changes while the linked run does not: only the
	// neighbor is written, with no link bytes (the id stays stable).
	prev := NewGrid(3, 1)
	prev.Links = next.Links
	*prev.At(0, 0) = *next.At(0, 0)
	*prev.At(1, 0) = *next.At(1, 0)
	prev.At(2, 0).Ch = 'd'
	want := "\x1b[1;3H\x1b[0;39;49mc\x1b[0m\x1b[?25l"
	if got := DiffWith(prev, next, Options{}); got != want {
		t.Errorf("neighbor-only diff = %q, want %q (no link bytes)", got, want)
	}
	// A changed linked run opens after its SGR, before its text, and closes
	// by the end of the frame.
	prev.At(0, 0).Ch = 'x'
	prev.At(1, 0).Ch = 'y' // the whole linked run changes, not its first cell
	got := DiffWith(prev, next, Options{})
	if !strings.Contains(got, "\x1b[0;39;49m\x1b]8;id=l1;https://x\x1b\\ab") {
		t.Errorf("open not after SGR, before text: %q", got)
	}
	if !strings.Contains(got, osc8Close+"c") {
		t.Errorf("close not at the run's end: %q", got)
	}
	if o := (Options{NoLinks: true}); strings.Contains(DiffWith(prev, next, o), "\x1b]8") {
		t.Errorf("NoLinks emitted a link")
	}
}

func TestSanitizeURI(t *testing.T) {
	if got := SanitizeURI("https://a/b c;d\te\x7fé"); got != "https://a/bcde" {
		t.Errorf("SanitizeURI = %q, want https://a/bcde", got)
	}
	if got := SanitizeURI("\x1b]8;;\x1b\\"); got != "]8\\" {
		t.Errorf("escape sanitized to %q", got)
	}
	if got := SanitizeURI(strings.Repeat("x", MaxURI+10)); len(got) != MaxURI {
		t.Errorf("truncate = %d bytes, want %d", len(got), MaxURI)
	}
	if got := SanitizeURI(" ;\x01"); got != "" {
		t.Errorf("only separators sanitized to %q", got)
	}
}
