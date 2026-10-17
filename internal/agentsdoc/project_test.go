package agentsdoc

import (
	"strings"
	"testing"
)

// ── Project briefing (SPEC v0.4a §15.11) ────────────────────────────────

// Every projectEdit must apply exactly once to Markdown(); a change to the
// briefing that makes one stale fails here instead of leaving a repository
// path in the project's AGENTS.md.
func TestProjectEditsApply(t *testing.T) {
	md := Markdown()
	for i, e := range projectEdits {
		if n := strings.Count(md, e.Old); n != 1 {
			t.Errorf("projectEdits[%d] (%.60q) matches %d times in Markdown(), want 1", i, e.Old, n)
		}
	}
}

func TestProjectMarkdown(t *testing.T) {
	p := ProjectMarkdown()
	if p != ProjectMarkdown() {
		t.Fatal("ProjectMarkdown() is not deterministic")
	}
	// It keeps §22's block except for two lists.
	for _, want := range []string{
		"# Tuimark\n\nYou design terminal UIs by editing text documents. You do not write Go layout.\n",
		"## Allowed edits\n- *.tui\n- *.tcss\n- sample.json\n- main.go or parent.ts, when the project has one\n",
		"## Forbidden\n- Bubble Tea / lipgloss / tea.Cmd / Update()\n- inventing tags not in the catalog\n",
		"## Catalog\n", "## Loop\ntuimark validate FILE\n", "## Bindings\n", "## Actions\n",
		"### Attributes per tag", "### CSS properties", "### Key tokens", "### Diagnostic codes", "### Notes",
		"### v0.4a: tools and the Go API",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("the project briefing is missing %q", want)
		}
	}
	// It drops the maintainer section, the bin/tuimark line, and every
	// pointer into the repository.
	for _, bad := range []string{
		"## Working on the Tuimark runtime", "./bin/tuimark", "go build -o bin/tuimark",
		"internal/", "cmd/", "examples/", "testdata", "schema/", "docs/", "sdk/", "editors/", "specs/",
		"README.md", "Taskfile", "task verify", "~/notes",
	} {
		if strings.Contains(p, bad) {
			t.Errorf("the project briefing names the repository path %q", bad)
		}
	}
	if !strings.HasSuffix(p, "\n") || strings.HasSuffix(p, "\n\n") {
		t.Error("the project briefing does not end with exactly one newline")
	}
	if !strings.Contains(p, "## Reference\n\n### Attributes per tag") {
		t.Error("the Reference does not follow the Loop addition directly")
	}
	// The repository's own briefing is unchanged by all this.
	if !strings.HasPrefix(RepoMarkdown(), strings.TrimRight(Markdown(), "\n")) {
		t.Error("RepoMarkdown() must start with Markdown()")
	}
	if !strings.Contains(Markdown(), "- examples/**/sample.json\n") {
		t.Error("Markdown() lost the repository's Allowed edits")
	}
}

// §14's V005 row (test 117) and the 0.4a section's claims.
func TestV005RowAndV04aSection(t *testing.T) {
	const want = "not well-formed XML; or not a Tuimark document: a root element other than `<tui>` (or the phase-0 alias `<app>`), or a `<tui>` with no `<screen>`"
	var row string
	for _, d := range DiagCodes() {
		if d.Code == "V005" {
			row = d.When
		}
	}
	if row != want {
		t.Errorf("V005 When = %q, want %q", row, want)
	}
	md := Markdown()
	for _, w := range []string{
		"| V005 | parse | " + want + " |",
		"ok (bindings not checked: no --data)",
		"`tuimark catalog`", "`tuimark new NAME [--go | --ts]`", "`tuimark agents --project`",
		"64 MiB or more",
	} {
		if !strings.Contains(md, w) {
			t.Errorf("Markdown() is missing %q", w)
		}
	}
}

func TestDiagCodesReturnsACopy(t *testing.T) {
	a := DiagCodes()
	a[0].Code = "X"
	if DiagCodes()[0].Code == "X" {
		t.Error("DiagCodes() shares its backing array")
	}
}
