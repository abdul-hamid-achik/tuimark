//go:build !js

package parse_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// Regressions from the F2 review that build large documents (too slow for
// js/wasm, whose test run is `task wasm-test`).

// within runs f, failing the test when it takes longer than limit; a run
// that hangs fails at the limit instead of stalling the suite.
func within(t *testing.T, limit time.Duration, f func()) time.Duration {
	t.Helper()
	done := make(chan time.Duration, 1)
	go func() {
		start := time.Now()
		f()
		done <- time.Since(start)
	}()
	select {
	case d := <-done:
		return d
	case <-time.After(limit):
		t.Fatalf("still running after %v", limit)
		return 0
	}
}

// A lookup of an alias costs the same however many aliases are in scope:
// a use with 50 000 with clauses, copying a template that uses another
// with 20 000 more (each checked against the aliases in scope), fanned out
// to 4096 leaves that each read 100 paths (review probe hugewith2, 27.7 s
// before the alias index).
func TestWithScopeLookupDoesNotGrowWithTheScope(t *testing.T) {
	clauses := func(prefix string, n int) string {
		parts := make([]string, n)
		for i := range parts {
			parts[i] = fmt.Sprintf("%sp%d as %sa%d", prefix, i, prefix, i)
		}
		return strings.Join(parts, ", ")
	}
	var b strings.Builder
	b.WriteString("<tui>\n")
	fmt.Fprintf(&b, "<template id=\"top\"><use template=\"t0\" with=\"%s\"/></template>\n", clauses("y", 20000))
	for i := 0; i < 12; i++ {
		fmt.Fprintf(&b, "<template id=\"t%d\"><use template=\"t%d\"/><use template=\"t%d\"/></template>\n", i, i+1, i+1)
	}
	b.WriteString("<template id=\"t12\"><text")
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&b, " class:c%d=\"ga%05d\"", i, i)
	}
	b.WriteString(">{xa1.v}{ya2.v}</text></template>\n")
	fmt.Fprintf(&b, "<screen><use template=\"top\" with=\"%s\"/></screen>\n</tui>", clauses("x", 50000))
	src := []byte(b.String())
	var d *parse.Document
	el := within(t, 60*time.Second, func() { d = parse.Parse(src, "app.tui") })
	if el > 5*time.Second {
		t.Errorf("took %v", el)
	}
	if len(d.Diags) != 0 {
		t.Errorf("diagnostics: %v", useDiags(d)[:min(3, len(d.Diags))])
	}
	if n := len(screenOf(d).Children); n != 4096 {
		t.Errorf("%d texts, want 4096", n)
	}
	if got := screenOf(d).Children[0].Text; got != "{xp1.v}{yp2.v}" {
		t.Errorf("text %q", got)
	}
}

// ring is n templates, each holding a use of the next, the last of the
// first.
func ring(n int) string {
	var b strings.Builder
	b.WriteString("<tui>\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "<template id=\"t%d\"><use template=\"t%d\"/></template>\n", i, (i+1)%n)
	}
	b.WriteString("<screen id=\"main\"><text>x</text></screen>\n</tui>")
	return b.String()
}

// V019 on a long cycle (the owner's decision on the F2 review): one per
// use on it, each naming the first 8 templates from the template holding
// the use, then how many more, then that template again; and the whole
// check stays near-linear on a ring (review probe ring10000: 15.7 s and
// 990 MB of output before).
func TestV019LongCycleIsShortenedAndFast(t *testing.T) {
	var doc *parse.Document
	el := within(t, 60*time.Second, func() { doc = parse.Parse([]byte(ring(10000)), "app.tui") })
	if el > time.Second {
		t.Errorf("took %v", el)
	}
	got := useDiags(doc)
	if len(got) != 10000 {
		t.Fatalf("%d diagnostics, want one V019 per use", len(got))
	}
	for i, want := range map[int]string{
		0:    `V019 2:19 <use template="t1"> in <template id="t0"> makes a cycle of templates: t0 → t1 → t2 → t3 → t4 → t5 → t6 → t7 → … (9992 more) → t0`,
		9999: `V019 10001:22 <use template="t0"> in <template id="t9999"> makes a cycle of templates: t9999 → t0 → t1 → t2 → t3 → t4 → t5 → t6 → … (9992 more) → t9999`,
	} {
		if got[i] != want {
			t.Errorf("diagnostic %d:\n%s\nwant\n%s", i, got[i], want)
		}
	}
}

// V019 stays near-linear where the shortest cycles are long and the
// component is not a ring (the F2 review, follow-up): a ring of 10 000
// with a chord (11.6 s and 746 MB of output before, 1.25 s with a full
// search per use), and 9 layers of 100 templates, each using every
// template of the next layer (every shortest cycle has 9 templates, so
// each use's capped search explores 7 layers until the budget runs out).
// Each use is named by a simple cycle through it.
func TestV019FallbackIsFast(t *testing.T) {
	chord := strings.Replace(ring(10000), `<template id="t0"><use template="t1"/>`, `<template id="t0"><use template="t1"/><use template="t5000"/>`, 1)
	var layered strings.Builder
	layered.WriteString("<tui>\n")
	for l := 0; l < 9; l++ {
		for k := 0; k < 100; k++ {
			fmt.Fprintf(&layered, `<template id="l%d_%d">`, l, k)
			for j := 0; j < 100; j++ {
				fmt.Fprintf(&layered, `<use template="l%d_%d"/>`, (l+1)%9, j)
			}
			layered.WriteString("</template>\n")
		}
	}
	layered.WriteString("<screen id=\"main\"><text>x</text></screen>\n</tui>")
	for _, c := range []struct {
		name, src string
		uses      int
		first     string
	}{
		{"ring with a chord", chord, 10001, `V019 2:19 <use template="t1"> in <template id="t0"> makes a cycle of templates: t0 → t1 → t2 → t3 → t4 → t5 → t6 → t7 → … (9992 more) → t0`},
		{"dense layers", layered.String(), 90000, `V019 2:21 <use template="l1_0"> in <template id="l0_0"> makes a cycle of templates: l0_0 → l1_0 → l2_0 → l3_0 → l4_0 → l5_0 → l6_0 → l7_0 → … (1 more) → l0_0`},
	} {
		t.Run(c.name, func(t *testing.T) {
			var doc *parse.Document
			el := within(t, 60*time.Second, func() { doc = parse.Parse([]byte(c.src), "app.tui") })
			if el > time.Second {
				t.Errorf("took %v", el)
			}
			got := useDiags(doc)
			if len(got) != c.uses || got[0] != c.first {
				t.Errorf("%d diagnostics, the first %q; want %d, the first %q", len(got), got[0], c.uses, c.first)
			}
		})
	}
}

// A cycle of 8 templates is named whole, as before; one of 9 names 8 and
// "(1 more)".
func TestV019CycleOfEightAndNine(t *testing.T) {
	got := useDiags(parse.Parse([]byte(ring(8)), "app.tui"))
	if want := `V019 2:19 <use template="t1"> in <template id="t0"> makes a cycle of templates: t0 → t1 → t2 → t3 → t4 → t5 → t6 → t7 → t0`; len(got) != 8 || got[0] != want {
		t.Errorf("8: %q\nwant %s first", got, want)
	}
	got = useDiags(parse.Parse([]byte(ring(9)), "app.tui"))
	if want := `V019 4:19 <use template="t3"> in <template id="t2"> makes a cycle of templates: t2 → t3 → t4 → t5 → t6 → t7 → t8 → t0 → … (1 more) → t2`; len(got) != 9 || got[2] != want {
		t.Errorf("9: %q\nwant %s third", got, want)
	}
	// A component that is not a single ring keeps the shortest cycle: a
	// chord from t0 to t5 in a ring of 12 makes t0's first use close a
	// cycle of 12, its second (the chord) one of 8.
	src := strings.Replace(ring(12), `<template id="t0"><use template="t1"/>`, `<template id="t0"><use template="t1"/><use template="t5"/>`, 1)
	got = useDiags(parse.Parse([]byte(src), "app.tui"))
	for _, want := range []string{
		`V019 2:19 <use template="t1"> in <template id="t0"> makes a cycle of templates: t0 → t1 → t2 → t3 → t4 → t5 → t6 → t7 → … (4 more) → t0`,
		`V019 2:39 <use template="t5"> in <template id="t0"> makes a cycle of templates: t0 → t5 → t6 → t7 → t8 → t9 → t10 → t11 → t0`,
	} {
		if !slices.Contains(got, want) {
			t.Errorf("chord: %q\nwant %s", got, want)
		}
	}
}

// A template is charged to the document's budget once per copy (ADR 0034,
// the F2 review): building its body as written, for its own checks, is
// charged to the definitions' budget instead. A template that includes a
// 9 MiB fragment (or 60 000 elements) and is used once is not V021 (review
// probes cap/t9, cap/t60: the definition and the copy used to add up past
// the limit).
func TestTemplateDefinitionIsNotChargedToTheDocument(t *testing.T) {
	files := map[string]string{
		"frag9.tui":   "<fragment><text>" + strings.Repeat("x", 9<<20) + "</text></fragment>",
		"frag60k.tui": "<fragment>" + strings.Repeat("<text>x</text>", 60000) + "</fragment>",
	}
	for _, c := range []struct {
		name, frag string
		texts      int
	}{{"9 MiB", "frag9.tui", 2}, {"60 000 elements", "frag60k.tui", 60001}} {
		t.Run(c.name, func(t *testing.T) {
			app := fmt.Sprintf("<tui>\n<template id=\"T\"><include src=%q/></template>\n<screen id=\"main\">\n<scroll><use template=\"T\"/></scroll>\n<text>after</text></screen>\n</tui>", c.frag)
			var doc *parse.Document
			within(t, 60*time.Second, func() {
				doc = parse.ParseWith([]byte(app), "app.tui", memLoader(files, "app.tui", nil))
			})
			if got := v021(doc); len(got) != 0 {
				t.Errorf("V021 %v", got)
			}
			if n := len(strings.Fields(screenTexts(doc))); n != c.texts {
				t.Errorf("%d texts, want %d", n, c.texts)
			}
		})
	}
}

// The definitions share one budget, so every body built as written stays
// bounded: two unused templates that include 9 MiB each pass it, and the
// second include reports V021 (review probe cap/t9d), with a message of
// its own: it concerns the template's own checks, and no later include in
// a template definition expands (W's, below). It stops nothing in the
// document: an include and a use after it still expand.
func TestDefinitionsBudgetNeverStopsThePlacements(t *testing.T) {
	files := map[string]string{
		"frag9.tui": "<fragment><text>" + strings.Repeat("x", 9<<20) + "</text></fragment>",
		"one.tui":   "<fragment><text>included</text></fragment>",
	}
	const app = "<tui>\n<template id=\"T\"><include src=\"frag9.tui\"/></template>\n<template id=\"U\"><include src=\"frag9.tui\"/></template>\n<template id=\"W\"><include src=\"one.tui\"/></template>\n<template id=\"S\"><text>copied</text></template>\n<screen id=\"main\">\n<include src=\"one.tui\"/><use template=\"S\"/>\n</screen>\n</tui>"
	var doc *parse.Document
	within(t, 60*time.Second, func() {
		doc = parse.ParseWith([]byte(app), "app.tui", memLoader(files, "app.tui", nil))
	})
	want := `app.tui:3:18 include src="frag9.tui" in <template id="U"> exceeds the expansion limit for template definitions (100000 elements or 16 MiB of placed source): it inserts nothing into the template's own checks, and no later include in a template definition expands`
	if got := v021(doc); len(got) != 1 || got[0] != want {
		t.Errorf("V021 %q, want %q", got, want)
	}
	if got := screenTexts(doc); got != "included copied" {
		t.Errorf("screen texts %q, want the include's and the copy's", got)
	}
	// W's include, as written, inserts nothing.
	found := false
	for _, n := range doc.Root.Source {
		if n.Kind == "template" && n.ID == "W" {
			found = true
			if inc := n.Source; len(inc) != 1 || inc[0].Kind != "include" || len(inc[0].Children) != 0 {
				t.Errorf("W as written: %v", inc)
			}
		}
	}
	if !found {
		t.Error("no template W in the document as written")
	}
}

// A cycle that checkCycles cannot see, because the include that closes it
// is in a template body left unexpanded once the definitions' budget ran
// out, is reported where a copy meets it, instead of stopping silently
// (the F2 review): A's fragment uses A, and B's fragment uses A, whose
// body uses B.
func TestCycleThroughAnUnexpandedIncludeIsReported(t *testing.T) {
	files := map[string]string{
		"big.tui":   texts(parse.ExpansionLimit),
		"loopA.tui": `<fragment><text>a</text><use template="A"/></fragment>`,
		"loopB.tui": `<fragment><text>b</text><use template="C"/></fragment>`,
	}
	const app = "<tui>\n<template id=\"T\"><include src=\"big.tui\"/></template>\n<template id=\"A\"><include src=\"loopA.tui\"/></template>\n<template id=\"B\"><include src=\"loopB.tui\"/></template>\n<template id=\"C\"><use template=\"B\"/></template>\n<screen id=\"main\"><use template=\"A\"/><use id=\"c\" template=\"C\"/></screen>\n</tui>"
	var doc *parse.Document
	within(t, 60*time.Second, func() {
		doc = parse.ParseWith([]byte(app), "app.tui", memLoader(files, "app.tui", nil))
	})
	sameDiags := []string{
		`V019 loopA.tui:1:25 <use template="A"> in <template id="A"> makes a cycle of templates: A → A`,
		`V019 loopB.tui:1:25 <use template="C"> in <template id="B"> makes a cycle of templates: B → C → B`,
		`V021 app.tui:2:18 include src="big.tui" in <template id="T"> exceeds the expansion limit for template definitions (100000 elements or 16 MiB of placed source): it inserts nothing into the template's own checks, and no later include in a template definition expands`,
	}
	var got []string
	for _, d := range doc.Diags {
		got = append(got, fmt.Sprintf("%s %s:%d:%d %s", d.Code, d.File, d.Line, d.Col, d.Msg))
	}
	if strings.Join(got, "\n") != strings.Join(sameDiags, "\n") {
		t.Errorf("diagnostics:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(sameDiags, "\n"))
	}
	if got := screenTexts(doc); got != "a b" {
		t.Errorf("screen texts %q", got)
	}
}

// A copy that a V021 discards makes no template used: a template copied
// only inside it reports its own diagnostics, as an unused template does
// (the F2 review: used stayed set after the copy was undone, and the
// template's own V002 was never reported).
func TestDiscardedCopyLeavesItsTemplatesUnused(t *testing.T) {
	app := "<tui>\n<template id=\"A\"><text bogus=\"1\">a</text></template>\n<template id=\"X\"><use template=\"A\"/>" +
		strings.Repeat("<rule/>", parse.ExpansionLimit) + "</template>\n<screen id=\"main\"><use template=\"X\"/></screen>\n</tui>"
	var doc *parse.Document
	within(t, 60*time.Second, func() { doc = parse.Parse([]byte(app), "app.tui") })
	var got []string
	for _, d := range doc.Diags {
		got = append(got, fmt.Sprintf("%s %d:%d", d.Code, d.Line, d.Col))
	}
	want := []string{"V002 2:24", "V021 4:19"}
	if strings.Join(got, ", ") != strings.Join(want, ", ") {
		t.Errorf("diagnostics %q, want %q", got, want)
	}
}
