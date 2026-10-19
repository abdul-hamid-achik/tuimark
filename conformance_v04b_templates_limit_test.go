//go:build !js

// These tests build and render documents of up to 100000 elements; under
// js/wasm (4 GiB, no preemption) they run out of memory, and
// internal/parse covers the same limits there without rendering.

package tuimark_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

// ADR 0034 for uses, through the public API: thirty levels of templates,
// each copying the next twice (2^30 elements written out), load, validate,
// and dump at once, with exactly one V021, at the innermost use that was
// expanding (the use of t30 in t29, on line 31).
func TestUseExpansionLimitFanOutRenders(t *testing.T) {
	var b strings.Builder
	b.WriteString("<tui>\n")
	for i := 0; i < 30; i++ {
		fmt.Fprintf(&b, "<template id=\"t%d\"><use template=\"t%d\"/><use template=\"t%d\"/></template>\n", i, i+1, i+1)
	}
	b.WriteString("<template id=\"t30\"><text>x</text></template>\n<screen id=\"s\"><use template=\"t0\"/></screen>\n</tui>\n")
	start := time.Now()
	app, err := tuimark.Parse(strings.NewReader(b.String()))
	if err != nil {
		t.Fatal(err)
	}
	var v021 []string
	for _, d := range diagTexts(app.Validate()) {
		if strings.HasPrefix(d, "V021 ") {
			v021 = append(v021, d)
		}
	}
	if _, err := app.Dump(80, 24); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 20*time.Second {
		t.Errorf("parse, validate, and dump took %v", d)
	}
	if len(v021) != 1 || !strings.Contains(v021[0], ":31:41 /screen/use ") || !strings.Contains(v021[0], `use template="t30" exceeds the expansion limit (100000 elements or 16 MiB of placed source): it inserts nothing, and no later include or use expands`) {
		t.Errorf("V021: %v", v021)
	}
}
