package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// SPEC v0.4b §24 tests of composition in the tools (phase F4), the parts
// that go through the CLI: 147 (inspect source), 148's amplification
// bound, 150 (fmt on fragments), 151's preview --watch bullet, 226 (a
// fragment checked on its own), and 138's inspect source bullet. The
// language server's parts (152) are internal/lsp's; 151's run bullets are
// the Glyphrun spec run_reload_fragments.

// 150. fmt formats a fragment exactly as it formats a document: the same
// text with the root renamed <tui>, renamed back; the output is canonical;
// and an <include> is never read.
func TestV04bFmtOnFragments(t *testing.T) {
	file := composeFixtures + "t133/parts/templates.tui"
	code, out, errw := runCLI("fmt", file)
	if code != 0 {
		t.Fatalf("fmt: exit %d %s", code, errw)
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	asDoc := strings.Replace(strings.Replace(string(raw), "<fragment>", "<tui>", 1), "</fragment>", "</tui>", 1)
	code, docOut, errw := runCLI("fmt", writeTemp(t, "doc.tui", asDoc))
	if code != 0 {
		t.Fatalf("fmt of the document form: exit %d %s", code, errw)
	}
	back := strings.Replace(strings.Replace(docOut, "<tui>", "<fragment>", 1), "</tui>", "</fragment>", 1)
	if out != back {
		t.Errorf("fmt of the fragment:\n%s\nwant (the document's, renamed back):\n%s", out, back)
	}
	formatted := writeTemp(t, "templates.tui", out)
	if code, out, errw := runCLI("fmt", formatted, "--check"); code != 0 {
		t.Errorf("fmt --check of fmt's output: exit %d %s%s", code, out, errw)
	}

	// An include is formatted, never read: its file does not exist.
	inc := writeTemp(t, "inc.tui", `<fragment><include src="missing.tui"/></fragment>`)
	code, out, errw = runCLI("fmt", inc)
	if code != 0 || out != "<fragment>\n  <include src=\"missing.tui\"/>\n</fragment>\n" || errw != "" {
		t.Errorf("fmt of an include: exit %d %q %q", code, out, errw)
	}
	// --write rewrites FILE only.
	if code, _, errw := runCLI("fmt", inc, "--write"); code != 0 {
		t.Fatalf("fmt --write: exit %d %s", code, errw)
	}
	entries, err := os.ReadDir(filepath.Dir(inc))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("fmt --write left %d files, want inc.tui alone", len(entries))
	}
}

// 226. validate checks a fragment on its own: test 223's kit/kit.tui
// reports nothing, with or without --data; next to it, bad.tui reports
// only the checks its elements give themselves; every other command gives
// a fragment its V005.
func TestV04bFragmentCheckedOnItsOwn(t *testing.T) {
	dir := composeFixtures + "t226/"
	for _, args := range [][]string{{}, {"--data", dir + "d.json"}} {
		code, out, errw := runCLI(append([]string{"validate", dir + "kit/kit.tui"}, args...)...)
		if code != 0 || out != "ok (fragment checked on its own)\n" {
			t.Errorf("validate kit.tui %v: exit %d %q %s", args, code, out, errw)
		}
	}
	want := "error V003 bad.tui:2:3 /include <include> needs src=\"FILE\"\n" +
		"error V001 bad.tui:10:7 /template/modal/zzz unknown tag <zzz>\n" +
		"error V002 bad.tui:11:13 /template/modal/text attribute \"bogus\" is not allowed on <text>\n"
	for _, args := range [][]string{{}, {"--data", dir + "d.json"}} {
		code, out, errw := runCLI(append([]string{"validate", dir + "kit/bad.tui"}, args...)...)
		if code != 2 || out != want {
			t.Errorf("validate bad.tui %v: exit %d\n%s%s\nwant\n%s", args, code, out, errw, want)
		}
	}
	got := vdiags(t, dir+"kit/bad.tui", dir+"d.json")
	if len(got) != 3 {
		t.Errorf("validate --json: %v", got)
	}
	code, out, _ := runCLI("dump", dir+"kit/bad.tui", "--format", "json")
	errs, _ := jsonMap(t, out)["errors"].([]any)
	if code != 2 || len(errs) != 1 || errs[0].(map[string]any)["msg"] != "a <fragment> is not a document: check it through the document that includes it" {
		t.Errorf("dump bad.tui: exit %d %s", code, out)
	}
}

// A fragment checked on its own has a document's usage errors: a --theme
// other than dark or light, and a --catalog that cannot be read, exit 1
// before anything is checked, though neither adds a check there.
func TestV04bFragmentValidateFlags(t *testing.T) {
	kit := composeFixtures + "t226/kit/kit.tui"
	for _, args := range [][]string{
		{"--theme", "auto"},
		{"--theme", ""},
		{"--catalog", filepath.Join(t.TempDir(), "nope.json")},
		{"--catalog", writeTemp(t, "cat.json", `{"actions": 3}`)},
	} {
		code, out, errw := runCLI(append([]string{"validate", kit}, args...)...)
		if code != 1 || out != "" || !strings.HasPrefix(errw, "tuimark: ") {
			t.Errorf("validate kit.tui %q: exit %d %q %q", args, code, out, errw)
		}
	}
	for _, args := range [][]string{
		{"--theme", "light"},
		{"--catalog", writeTemp(t, "cat.json", `["other"]`)},
		{"--strict"},
	} {
		code, out, errw := runCLI(append([]string{"validate", kit}, args...)...)
		if code != 0 || out != "ok (fragment checked on its own)\n" {
			t.Errorf("validate kit.tui %q: exit %d %q %q", args, code, out, errw)
		}
	}
}
