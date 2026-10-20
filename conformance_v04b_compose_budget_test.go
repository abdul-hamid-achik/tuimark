//go:build !js

// The expansion limits build documents of 100000 elements or 16 MiB of source and
// render them; under js/wasm (4 GiB, no preemption) they run out of memory,
// and internal/parse covers the same limits there without rendering.

package tuimark_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ADR 0034, second amendment: the byte budget. Both inputs stay far below
// 100000 elements and used to take seconds and gigabytes (the review of
// phase F1, round 5); each now ends with exactly one V021, quickly.

// budgetFiles are the two inputs: (a) 2000 failing includes, placed 2000
// times, and (b) one element with 20000 attributes, included 1000 times.
func budgetFiles(which string) map[string]string {
	var b strings.Builder
	files := map[string]string{}
	switch which {
	case "a":
		b.WriteString("<fragment>")
		for i := 0; i < 2000; i++ {
			b.WriteString("<include src=\"x.tcss\"/>\n")
		}
		b.WriteString("</fragment>")
		files["leaf.tui"] = b.String()
		b.Reset()
		b.WriteString("<fragment>")
		for i := 0; i < 2000; i++ {
			b.WriteString("<include src=\"leaf.tui\"/>\n")
		}
		b.WriteString("</fragment>")
		files["mid.tui"] = b.String()
		files["app.tui"] = appWith(`<include src="mid.tui"/>`)
	case "b":
		b.WriteString("<fragment><text")
		for i := 0; i < 20000; i++ {
			fmt.Fprintf(&b, " a%d=\"\"", i)
		}
		b.WriteString(">x</text></fragment>")
		files["leaf.tui"] = b.String()
		var app strings.Builder
		app.WriteString("<tui>\n<screen id=\"s\">\n")
		for i := 0; i < 1000; i++ {
			app.WriteString("<include src=\"leaf.tui\"/>\n")
		}
		app.WriteString("</screen>\n</tui>\n")
		files["app.tui"] = app.String()
	}
	return files
}

func TestByteBudgetBoundsPlacedSource(t *testing.T) {
	for _, which := range []string{"a", "b"} {
		t.Run(which, func(t *testing.T) {
			dir := t.TempDir()
			for name, text := range budgetFiles(which) {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			start := time.Now()
			app := mustLoad(t, filepath.Join(dir, "app.tui"))
			t.Logf("Load: %v", time.Since(start))
			var v021 []string
			for _, d := range diagTexts(app.Validate()) {
				if strings.HasPrefix(d, "V021 ") {
					v021 = append(v021, d)
				}
			}
			if d := time.Since(start); d > 30*time.Second*slowdown {
				t.Errorf("took %v", d)
			}
			if len(v021) != 1 || !strings.Contains(v021[0], `exceeds the expansion limit (100000 elements or 16 MiB of placed source): it inserts nothing, and no later include or use expands`) {
				t.Errorf("V021: %v", v021)
			}
		})
	}
}
