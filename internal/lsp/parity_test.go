package lsp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/host"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// The server's diagnostics are `tuimark validate`'s (SPEC §15.12 "Which
// ones": validate's widths, themes, codes, messages, severities). The
// reference below is validate's own sequence, written out: host.Load, the
// sibling sample.json as --data, Validate, and the data-free filter.
func validateReference(t *testing.T, path string) []string {
	t.Helper()
	app, err := host.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	hasData := false
	if raw, err := os.ReadFile(filepath.Join(filepath.Dir(path), "sample.json")); err == nil {
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			t.Fatal(err)
		}
		if err := app.Bind("", v); err != nil {
			t.Fatal(err)
		}
		hasData = true
	}
	diags := app.Validate()
	if !hasData {
		var kept ir.Diags
		for _, d := range diags {
			switch d.Code {
			case "B001", "B002", "B003", "B008", "B009", "B010":
				continue
			}
			kept = append(kept, d)
		}
		diags = kept
	}
	return diagKeys(diags)
}

// diagKeys are diagnostics as sorted (code, severity, line, column,
// message, file) strings, a stylesheet named by its base name (the server
// names it by its absolute path, validate by its base name).
func diagKeys(diags ir.Diags) []string {
	var out []string
	for _, d := range diags {
		out = append(out, fmt.Sprintf("%s %s %d:%d %s %s", d.Code, d.Severity, d.Line, d.Col, d.Msg, filepath.Base(d.File)))
	}
	sort.Strings(out)
	return out
}

func TestAnalyzeIsValidate(t *testing.T) {
	tmp := writeTree(t, map[string]string{
		"auto.tui": `<tui version="2" theme="auto"><style src="auto.tcss"/><screen id="s"><text>{missing}</text></screen></tui>` + "\n",
		"auto.tcss": "@media (theme: dark) { :root { --only-dark: red; } }\n" +
			"screen { color: $only-dark; bogus-prop: 1; }\n",
		"broken.tui":   `<tui version="1"><screen id="s"><text>x</screen></tui>` + "\n",
		"errors.tui":   `<tui version="3"><style src="gone.tcss"/><screen id="s"><bogus/><text id="a">x</text><text id="a">y</text></screen></tui>` + "\n",
		"sample.json":  `{"present": 1}`,
		"wide.tui":     "<tui version=\"1\"><screen id=\"日本😀\"><text>{x}</text><bogus/></screen></tui>\n",
		"nodata/x.tui": `<tui version="1"><screen id="s"><text>{missing}</text></screen></tui>` + "\n",
	})
	// The tracked examples, listed rather than globbed: a checkout may hold
	// untracked ones.
	files := []string{
		"examples/2048/app.tui",
		"examples/agent/agent.tui",
		"examples/chess/app.tui",
		"examples/dashboard/app.tui",
		"examples/gitui/app.tui",
		"examples/host-ts/view.tui",
		"examples/http/app.tui",
		"examples/inbox/app.tui",
		"examples/logs/app.tui",
		"examples/minesweeper/app.tui",
		"examples/monitor/studio.tui",
		"examples/music/app.tui",
		"examples/pipeline/app.tui",
		"examples/pomodoro/app.tui",
		"examples/snake/app.tui",
		"examples/spike/inbox.tui",
		"examples/stacked/app.tui",
		"examples/sudoku/app.tui",
		"examples/tasks/app.tui",
		"examples/tetris/app.tui",
		"examples/unicode/app.tui",
	}
	var paths []string
	for _, f := range files {
		p, err := filepath.Abs(filepath.Join("..", "..", f))
		if err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p)
	}
	for _, f := range []string{"auto.tui", "broken.tui", "errors.tui", "wide.tui", "nodata/x.tui"} {
		paths = append(paths, filepath.Join(tmp, f))
	}
	for _, p := range paths {
		text, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		sn := &snapshot{open: map[string]string{}, byURI: map[string]*document{}}
		got := diagKeys(analyze(sn, p, string(text)).diags)
		want := validateReference(t, p)
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("%s:\nanalyze  %q\nvalidate %q", p, got, want)
		}
	}
	// The reference is not vacuous: the cases above do hold diagnostics.
	for _, f := range []string{"auto.tui", "errors.tui", "wide.tui"} {
		if len(validateReference(t, filepath.Join(tmp, f))) == 0 {
			t.Errorf("%s: no diagnostics to compare", f)
		}
	}
}
