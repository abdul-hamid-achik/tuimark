package parse_test

import (
	"strings"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// The cost of an element's attributes is linear in their number: a document
// with one element of 20000 unknown attributes (each a V002 with its
// suggestion) is read in well under a second (the review of phase F1,
// round 5). The ceiling is generous so CI does not flake; it was 0.12 s a
// placement before the suggestion candidates were kept per tag.
func TestHugeElementIsLinear(t *testing.T) {
	src := hugeElement(20000)
	start := time.Now()
	doc := parse.Parse(src, "a.tui")
	if d := time.Since(start); d > 3*slowdown*time.Second {
		t.Errorf("20000 attributes took %v", d)
	}
	n := 0
	for _, d := range doc.Diags {
		if d.Code == "V002" && strings.Contains(d.Msg, `attribute "a`) {
			n++
		}
	}
	if n != 20000 {
		t.Errorf("%d V002, want 20000", n)
	}
	// Doubling the attributes about doubles the time: not quadratic.
	t1 := timeParse(hugeElement(10000))
	t2 := timeParse(hugeElement(40000))
	if t2 > 12*t1+50*time.Millisecond {
		t.Errorf("10000 attributes took %v, 40000 took %v: not linear", t1, t2)
	}
}

func timeParse(src []byte) time.Duration {
	start := time.Now()
	parse.Parse(src, "a.tui")
	return time.Since(start)
}

// Many <style> elements are recorded in linear time and in order, each
// place once.
func TestManyStylesAreLinear(t *testing.T) {
	var b strings.Builder
	b.WriteString("<tui><screen/>")
	for i := 0; i < 20000; i++ {
		b.WriteString("<style>text { bold: true; }</style>")
	}
	b.WriteString("</tui>")
	start := time.Now()
	doc := parse.Parse([]byte(b.String()), "a.tui")
	if d := time.Since(start); d > 3*slowdown*time.Second {
		t.Errorf("20000 styles took %v", d)
	}
	if len(doc.Styles) != 20000 {
		t.Errorf("%d styles", len(doc.Styles))
	}
	for i := 1; i < len(doc.Styles); i++ {
		if doc.Styles[i].Col <= doc.Styles[i-1].Col {
			t.Fatalf("styles out of order at %d", i)
		}
	}
}
