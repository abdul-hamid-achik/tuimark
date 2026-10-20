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
	"testing/fstest"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

// ADR 0034 through the public loaders: a fan-out of plain files (thirty
// fragments that include the next twice each, so 2^30 elements) finishes
// at once, with exactly one V021, and its dump and validate are fast too.
func TestExpansionLimitFanOutThroughLoadFS(t *testing.T) {
	mem := fstest.MapFS{
		"app.tui": {Data: []byte("<tui>\n  <screen id=\"s\">\n    <include src=\"f0.tui\"/>\n  </screen>\n</tui>\n")},
		"f30.tui": {Data: []byte("<fragment><text>x</text></fragment>")},
	}
	for i := 0; i < 30; i++ {
		mem[fmt.Sprintf("f%d.tui", i)] = &fstest.MapFile{Data: []byte(fmt.Sprintf("<fragment>\n<include src=\"f%d.tui\"/>\n<include src=\"f%d.tui\"/>\n</fragment>", i+1, i+1))}
	}
	start := time.Now()
	app, err := tuimark.LoadFS(mem, "app.tui")
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
	if d := time.Since(start); d > 20*time.Second*slowdown {
		t.Errorf("load, validate, and dump took %v", d)
	}
	if len(v021) != 1 || !strings.HasPrefix(v021[0], "V021 f29.tui:") || !strings.Contains(v021[0], `include src="f30.tui" exceeds the expansion limit (100000 elements or 16 MiB of placed source): it inserts nothing, and no later include or use expands`) {
		t.Errorf("V021: %v", v021)
	}
}

// ADR 0034: includes count toward the limit, so thirty levels of two
// includes each over a leaf of <fragment/> (plain files) give exactly one
// V021 under Load, and quickly.
func TestExpansionLimitCountsIncludesLoad(t *testing.T) {
	dir := t.TempDir()
	write := func(name, text string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("app.tui", appWith(`<include src="f0.tui"/>`))
	write("f30.tui", "<fragment/>")
	for i := 0; i < 30; i++ {
		write(fmt.Sprintf("f%d.tui", i), fmt.Sprintf("<fragment><include src=\"f%d.tui\"/><include src=\"f%d.tui\"/></fragment>", i+1, i+1))
	}
	start := time.Now()
	app := mustLoad(t, filepath.Join(dir, "app.tui"))
	var v021 []string
	for _, d := range diagTexts(app.Validate()) {
		if strings.HasPrefix(d, "V021 ") {
			v021 = append(v021, d)
		}
	}
	if d := time.Since(start); d > 20*time.Second*slowdown {
		t.Errorf("took %v", d)
	}
	if len(v021) != 1 {
		t.Errorf("V021: %v", v021)
	}
}
