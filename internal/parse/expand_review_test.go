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
