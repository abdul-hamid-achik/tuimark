//go:build !js

package parse_test

import (
	"fmt"
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
// second include reports V021 (review probe cap/t9d). That V021 stops
// nothing in the document: an include and a use after it still expand.
func TestDefinitionsBudgetNeverStopsThePlacements(t *testing.T) {
	files := map[string]string{
		"frag9.tui": "<fragment><text>" + strings.Repeat("x", 9<<20) + "</text></fragment>",
		"one.tui":   "<fragment><text>included</text></fragment>",
	}
	const app = "<tui>\n<template id=\"T\"><include src=\"frag9.tui\"/></template>\n<template id=\"U\"><include src=\"frag9.tui\"/></template>\n<template id=\"S\"><text>copied</text></template>\n<screen id=\"main\">\n<include src=\"one.tui\"/><use template=\"S\"/>\n</screen>\n</tui>"
	var doc *parse.Document
	within(t, 60*time.Second, func() {
		doc = parse.ParseWith([]byte(app), "app.tui", memLoader(files, "app.tui", nil))
	})
	want := `app.tui:3:18 include src="frag9.tui" exceeds the expansion limit (100000 elements or 16 MiB of placed source): it inserts nothing, and no later include or use expands`
	if got := v021(doc); len(got) != 1 || got[0] != want {
		t.Errorf("V021 %q, want %q", got, want)
	}
	if got := screenTexts(doc); got != "included copied" {
		t.Errorf("screen texts %q, want the include's and the copy's", got)
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
