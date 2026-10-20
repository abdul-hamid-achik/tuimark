//go:build !js

package parse_test

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// Regressions from the F2 re-review that build large documents (too slow
// for js/wasm, whose test run is `task wasm-test`).

// treeBytes is the bytes of the ids, texts, attribute values, and
// classes of the tree under n: what `dump` and `ir` print grows with it.
func treeBytes(n *ir.Node) int {
	if n == nil {
		return 0
	}
	s := len(n.ID) + len(n.Text)
	for _, v := range n.Attrs {
		s += len(v)
	}
	for _, c := range n.Classes {
		s += len(c)
	}
	for _, c := range n.Children {
		s += treeBytes(c)
	}
	return s
}

// bounded parses src within a second and an allocation bound, and checks
// that the document it builds stays within twice the byte budget; it
// returns the document.
func bounded(t *testing.T, src string, files map[string]string) *parse.Document {
	t.Helper()
	var doc *parse.Document
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	el := within(t, 60*time.Second, func() {
		doc = parse.ParseWith([]byte(src), "app.tui", memLoader(files, "app.tui", nil))
	})
	runtime.ReadMemStats(&after)
	if el > slowdown*time.Second {
		t.Errorf("took %v", el)
	}
	if a := after.TotalAlloc - before.TotalAlloc; a > 512<<20 {
		t.Errorf("allocated %d MiB", a>>20)
	}
	if n := treeBytes(doc.Root); n > 2*parse.ExpansionByteLimit {
		t.Errorf("the document holds %d MiB", n>>20)
	}
	return doc
}

// What a copy makes beyond its template's span is charged to the
// document's byte budget (the F2 re-review, B2): the scoped-id prefix of
// every element a copy places, the bytes a with alias's path adds to each
// value that names it, and the use's class list on each top-level element
// it lands on; and the class merge goes through a set. The review's probes
// (longid_100000, nestid, longwith, cls/c_10000_100) took up to 11.7 s and
// 3.9 GB; each is now V021, or finishes, in well under a second.
func TestCopiesAreChargedWhatTheyMake(t *testing.T) {
	x := strings.Repeat("x", 100000)
	t.Run("a long use id", func(t *testing.T) {
		var b strings.Builder
		b.WriteString("<tui>\n<template id=\"T\"><col>\n")
		for i := 0; i < 10000; i++ {
			fmt.Fprintf(&b, "<text id=\"a%d\">x</text>\n", i)
		}
		fmt.Fprintf(&b, "</col></template>\n<screen><use id=\"u%s\" template=\"T\"/></screen>\n</tui>\n", x)
		doc := bounded(t, b.String(), nil)
		if got := v021(doc); len(got) != 1 || !strings.HasPrefix(got[0], "app.tui:10004:9 use template=\"T\" exceeds the expansion limit") {
			t.Errorf("V021 %q", got)
		}
	})
	t.Run("nested long use ids", func(t *testing.T) {
		var b strings.Builder
		b.WriteString("<tui>\n")
		for k := 0; k < 240; k++ {
			fmt.Fprintf(&b, "<template id=\"t%d\">", k)
			for i := 0; i < 100; i++ {
				fmt.Fprintf(&b, "<text id=\"a%d\">x</text>", i)
			}
			if k+1 < 240 {
				fmt.Fprintf(&b, "<use id=\"u%d%s\" template=\"t%d\"/>", k, x[:1000], k+1)
			}
			b.WriteString("</template>\n")
		}
		b.WriteString("<screen><use id=\"r\" template=\"t0\"/></screen>\n</tui>\n")
		doc := bounded(t, b.String(), nil)
		if got := v021(doc); len(got) != 1 {
			t.Errorf("V021 %q", got)
		}
	})
	t.Run("a long with path", func(t *testing.T) {
		var b strings.Builder
		b.WriteString("<tui>\n<template id=\"T\"><col>\n")
		for i := 0; i < 10000; i++ {
			b.WriteString("<text>{a}</text>\n")
		}
		fmt.Fprintf(&b, "</col></template>\n<screen><use template=\"T\" with=\"p%s as a\"/></screen>\n</tui>\n", strings.Repeat("q", 100000))
		doc := bounded(t, b.String(), nil)
		if got := v021(doc); len(got) != 1 || !strings.HasPrefix(got[0], "app.tui:10004:9 use template=\"T\" exceeds the expansion limit") {
			t.Errorf("V021 %q", got)
		}
	})
	t.Run("a long with path many times in one text", func(t *testing.T) {
		// One value naming the alias many times is charged before it is
		// built.
		doc := bounded(t, "<tui>\n<template id=\"T\"><text>"+strings.Repeat("{a}", 100000)+"</text></template>\n<screen><use template=\"T\" with=\"p"+x+" as a\"/></screen>\n</tui>\n", nil)
		if got := v021(doc); len(got) != 1 {
			t.Errorf("V021 %q", got)
		}
	})
	t.Run("a long class list", func(t *testing.T) {
		cls := make([]string, 10000)
		for i := range cls {
			cls[i] = fmt.Sprintf("c%d", i)
		}
		doc := bounded(t, "<tui>\n<template id=\"T\">"+strings.Repeat("<text>x</text>", 100)+"</template>\n<screen><col><use template=\"T\" class=\""+strings.Join(cls, " ")+"\"/></col></screen>\n</tui>\n", nil)
		if len(doc.Diags) != 0 {
			t.Errorf("diagnostics %v", useDiags(doc))
		}
		col := screenOf(doc).Children[0]
		if len(col.Children) != 100 {
			t.Fatalf("%d texts", len(col.Children))
		}
		for _, e := range col.Children {
			if len(e.Classes) != 10000 || e.Classes[0] != "c0" || e.Classes[9999] != "c9999" {
				t.Fatalf("%d classes", len(e.Classes))
			}
		}
		// 2000 classes on 5000 elements is 50 MB of class names: V021.
		doc = bounded(t, "<tui>\n<template id=\"T\">"+strings.Repeat("<text>x</text>", 5000)+"</template>\n<screen><col><use template=\"T\" class=\""+strings.Join(cls[:2000], " ")+"\"/></col></screen>\n</tui>\n", nil)
		if got := v021(doc); len(got) != 1 {
			t.Errorf("V021 %q", got)
		}
	})
}

// A V019 shows a template id longer than 64 bytes as its first 64 bytes,
// cut at a rune boundary, then "…" (the owner's decision in the F2
// re-review, SF1): a template with a 100 KB id holding 10 000 uses of a
// template that uses it back (probe out/longholder) printed 3 GB in 11.6 s.
func TestV019ShowsLongIDsShortened(t *testing.T) {
	long := "a" + strings.Repeat("x", 100000)
	var b strings.Builder
	fmt.Fprintf(&b, "<tui>\n<template id=%q>\n", long)
	for i := 0; i < 10000; i++ {
		b.WriteString("<use template=\"b\"/>\n")
	}
	fmt.Fprintf(&b, "</template>\n<template id=\"b\"><use template=%q/></template>\n<screen><text>x</text></screen>\n</tui>\n", long)
	doc := bounded(t, b.String(), nil)
	shown := long[:64] + "…"
	if len(doc.Diags) != 10001 {
		t.Fatalf("%d diagnostics", len(doc.Diags))
	}
	size := 0
	for _, d := range doc.Diags {
		size += len(d.Msg)
	}
	if size > 10001*400 {
		t.Errorf("messages hold %d bytes", size)
	}
	want := fmt.Sprintf("<use template=\"b\"> in <template id=%q> makes a cycle of templates: %s → b → %s", shown, shown, shown)
	if got := doc.Diags[0].Msg; got != want {
		t.Errorf("message\n%s\nwant\n%s", got, want)
	}
	if got, want := doc.Diags[10000].Msg, fmt.Sprintf("<use template=%q> in <template id=\"b\"> makes a cycle of templates: b → %s → b", shown, shown); got != want {
		t.Errorf("message\n%s\nwant\n%s", got, want)
	}
}

// The cut never splits a rune, an id of exactly 64 bytes is shown whole,
// and every V019 message shortens the ids it quotes.
func TestV019ShortenedIDFormat(t *testing.T) {
	for _, c := range []struct{ id, shown, hash string }{
		{strings.Repeat("y", 63) + "€€", strings.Repeat("y", 63) + "…", "#" + strings.Repeat("y", 63) + "…"},
		{strings.Repeat("z", 64), strings.Repeat("z", 64), "#" + strings.Repeat("z", 63) + "…"},
		{strings.Repeat("w", 62) + "€", strings.Repeat("w", 62) + "…", "#" + strings.Repeat("w", 62) + "…"},
		{"short", "short", "#short"},
	} {
		doc := parse.Parse([]byte(fmt.Sprintf("<tui>\n<template id=%q><use template=%q/></template>\n<screen><use template=%q/></screen>\n</tui>\n", c.id, c.id, "#"+c.id)), "app.tui")
		got := useDiags(doc)
		var want []string
		if !isASCII(c.id) {
			// (A template id is ASCII: V003, and the template still
			// stands, so a use of it still names it.)
			want = append(want, fmt.Sprintf("V003 2:11 bad id %q (letters, digits, '-' and '_'; must not start with a digit)", c.id))
		}
		want = append(want,
			fmt.Sprintf("V019 2:%d <use template=%q> in <template id=%q> makes a cycle of templates: %s → %s", 17+utf8.RuneCountInString(c.id), c.shown, c.shown, c.shown, c.shown),
			fmt.Sprintf("V019 3:9 <use template=%q> names no <template>; write template=%q, without #", c.hash, c.shown),
		)
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Errorf("%q:\n%s\nwant\n%s", c.id, strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
	}
}

// A written use that copies meet closing a cycle is reported once (the F2
// re-review, SF2): in probe rtcyc/app, B's copy meets C's use of B both
// through C (C → B → C) and through D (C → B → D → C). Without the
// definitions' overflow (rtcyc/app2), checkCycles reports each use on a
// cycle.
func TestCycleMetByCopiesIsReportedOncePerUse(t *testing.T) {
	files := map[string]string{
		"big.tui": texts(parse.ExpansionLimit),
		"b.tui":   "<fragment>\n<use template=\"C\"/>\n<use template=\"D\"/>\n</fragment>",
		"c.tui":   "<fragment>\n<use template=\"B\"/>\n</fragment>",
		"d.tui":   "<fragment>\n<use template=\"C\"/>\n</fragment>",
	}
	defs := "<template id=\"B\"><include src=\"b.tui\"/></template>\n<template id=\"C\"><include src=\"c.tui\"/></template>\n<template id=\"D\"><include src=\"d.tui\"/></template>\n<screen><use template=\"B\"/></screen>\n</tui>"
	for _, c := range []struct {
		name, app string
		want      []string
	}{{
		name: "met by copies",
		app:  "<tui>\n<template id=\"Z\"><include src=\"big.tui\"/></template>\n" + defs,
		want: []string{
			`V021 app.tui:2:18 include src="big.tui" in <template id="Z"> exceeds the expansion limit for template definitions (100000 elements or 16 MiB of placed source): it inserts nothing into the template's own checks, and no later include in a template definition expands`,
			`V019 c.tui:2:1 <use template="B"> in <template id="C"> makes a cycle of templates: C → B → C`,
		},
	}, {
		name: "control",
		app:  "<tui>\n" + defs,
		want: []string{
			`V019 b.tui:2:1 <use template="C"> in <template id="B"> makes a cycle of templates: B → C → B`,
			`V019 b.tui:3:1 <use template="D"> in <template id="B"> makes a cycle of templates: B → D → C → B`,
			`V019 c.tui:2:1 <use template="B"> in <template id="C"> makes a cycle of templates: C → B → C`,
			`V019 d.tui:2:1 <use template="C"> in <template id="D"> makes a cycle of templates: D → C → B → D`,
		},
	}} {
		t.Run(c.name, func(t *testing.T) {
			var doc *parse.Document
			within(t, 60*time.Second, func() {
				doc = parse.ParseWith([]byte(c.app), "app.tui", memLoader(files, "app.tui", nil))
			})
			var got []string
			for _, d := range doc.Diags.Sorted() {
				got = append(got, fmt.Sprintf("%s %s:%d:%d %s", d.Code, d.File, d.Line, d.Col, d.Msg))
			}
			if strings.Join(got, "\n") != strings.Join(c.want, "\n") {
				t.Errorf("diagnostics:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(c.want, "\n"))
			}
		})
	}
}

// The definitions' V021 is reported whether or not a use copies the
// template whose include it stopped (the F2 re-review, SF3): it stops the
// checks of every later definition, so a document whose definitions ran
// out never validates clean. In probe defb/y_used, Z2 is used, and Y's
// V002 and V006, which the control reports, are lost.
func TestDefinitionsV021IsReportedWhenTheTemplateIsUsed(t *testing.T) {
	files := map[string]string{
		"mid.tui": texts(60000),
		"bad.tui": "<fragment>\n<text bogus=\"1\">b</text>\n</fragment>",
	}
	tail := "<template id=\"Q\"><text>q</text></template>\n<template id=\"Y\"><include src=\"bad.tui\"/><include src=\"missing.tui\"/></template>\n"
	for _, c := range []struct {
		name, app string
		want      []string
		texts     int
	}{{
		name:  "used",
		app:   "<tui>\n<template id=\"Z1\"><include src=\"mid.tui\"/></template>\n<template id=\"Z2\"><include src=\"mid.tui\"/></template>\n" + tail + "<screen><use template=\"Z2\"/></screen>\n</tui>",
		want:  []string{`V021 app.tui:3:19 include src="mid.tui" in <template id="Z2"> exceeds the expansion limit for template definitions (100000 elements or 16 MiB of placed source): it inserts nothing into the template's own checks, and no later include in a template definition expands`},
		texts: 60000,
	}, {
		name:  "control",
		app:   "<tui>\n" + tail + "<screen><text>x</text></screen>\n</tui>",
		want:  []string{`V006 app.tui:3:42`, `V002 bad.tui:2:7`},
		texts: 1,
	}} {
		t.Run(c.name, func(t *testing.T) {
			var doc *parse.Document
			within(t, 60*time.Second, func() {
				doc = parse.ParseWith([]byte(c.app), "app.tui", memLoader(files, "app.tui", nil))
			})
			var got []string
			for _, d := range doc.Diags.Sorted() {
				s := fmt.Sprintf("%s %s:%d:%d", d.Code, d.File, d.Line, d.Col)
				if d.Code == "V021" {
					s += " " + d.Msg
				}
				got = append(got, s)
			}
			if strings.Join(got, "\n") != strings.Join(c.want, "\n") {
				t.Errorf("diagnostics:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(c.want, "\n"))
			}
			// It stops no placement: the copy of Z2 holds its include.
			if n := len(strings.Fields(screenTexts(doc))); n != c.texts {
				t.Errorf("%d texts, want %d", n, c.texts)
			}
		})
	}
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// An ordinary document stays far from the byte budget with the charges of
// the F2 re-review (prefix, substitution, class list, placement suffix):
// 1000 uses with ordinary ids of a template of 50 elements, 49 of them
// with ids and a with path (50 000 copied elements), leave room for an
// 11 MiB copy besides, so they cost under 5 MiB of the 16.
func TestOrdinaryCopiesAreFarFromTheByteBudget(t *testing.T) {
	var b strings.Builder
	b.WriteString("<tui>\n<template id=\"row\"><col>")
	for i := 0; i < 49; i++ {
		fmt.Fprintf(&b, "<text id=\"c%d\" class=\"cell\">{r.v}</text>", i)
	}
	b.WriteString("</col></template>\n<template id=\"fill\"><text>f</text><!--" + strings.Repeat("x", 11<<20) + "--></template>\n<screen>\n")
	for i := 0; i < 1000; i++ {
		fmt.Fprintf(&b, "<use id=\"r%d\" template=\"row\" class=\"odd\" with=\"rows as r\"/>\n", i)
	}
	b.WriteString("<use template=\"fill\"/>\n</screen>\n</tui>\n")
	var doc *parse.Document
	el := within(t, 60*time.Second, func() { doc = parse.Parse([]byte(b.String()), "app.tui") })
	if el > slowdown*time.Second {
		t.Errorf("took %v", el)
	}
	if len(doc.Diags) != 0 {
		t.Errorf("diagnostics %v", useDiags(doc)[:min(3, len(doc.Diags))])
	}
	if n := len(screenOf(doc).Children); n != 1001 {
		t.Errorf("%d children, want 1000 copies and the filler", n)
	}
}
