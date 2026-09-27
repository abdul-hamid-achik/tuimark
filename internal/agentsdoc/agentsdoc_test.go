package agentsdoc

import (
	"os"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// TestAGENTSDoesNotDrift asserts the repo's AGENTS.md is exactly
// Markdown()'s output, so the two can never fall out of sync: regenerate
// it with `tuimark agents --repo > AGENTS.md` instead of hand-editing.
func TestAGENTSDoesNotDrift(t *testing.T) {
	want := RepoMarkdown()
	got, err := os.ReadFile("../../AGENTS.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Errorf("AGENTS.md is out of date; regenerate it with `tuimark agents --repo > AGENTS.md`.\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}

func TestMarkdownIsDeterministic(t *testing.T) {
	a := Markdown()
	b := Markdown()
	if a != b {
		t.Fatal("Markdown() is not deterministic across calls")
	}
}

func TestMarkdownContainsSpec22Sections(t *testing.T) {
	md := Markdown()
	for _, want := range []string{
		"# Tuimark", "## Allowed edits", "## Forbidden", "## Catalog",
		"## Loop", "## Bindings", "## Actions",
		"### Attributes per tag", "### CSS properties", "### Key tokens", "### Diagnostic codes",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("Markdown() is missing section %q", want)
		}
	}
}

// extractSpec22 extracts the fenced code block under a SPEC document's
// "## 22. AGENTS.md (copy into the repo)" heading (its content only, not
// the ``` fence lines).
func extractSpec22(t *testing.T, spec string) string {
	t.Helper()
	lines := strings.Split(spec, "\n")
	start := -1
	for i, l := range lines {
		if strings.HasPrefix(l, "## 22.") {
			start = i
			break
		}
	}
	if start < 0 {
		t.Fatal("the SPEC has no '## 22.' heading")
	}
	open := -1
	for i := start; i < len(lines); i++ {
		if lines[i] == "```" {
			open = i
			break
		}
	}
	if open < 0 {
		t.Fatal("no fenced block found under SPEC §22")
	}
	closeLine := -1
	for i := open + 1; i < len(lines); i++ {
		if lines[i] == "```" {
			closeLine = i
			break
		}
	}
	if closeLine < 0 {
		t.Fatal("SPEC §22 fenced block is not closed")
	}
	return strings.Join(lines[open+1:closeLine], "\n") + "\n"
}

// spec22Block returns the frozen copy of the SPEC §22 block kept in
// testdata/spec22.md. The SPEC itself is maintained outside this
// repository; when TUIMARK_SPEC names a copy of it, the frozen block must
// still match that document's §22.
func spec22Block(t *testing.T) string {
	t.Helper()
	frozen, err := os.ReadFile("testdata/spec22.md")
	if err != nil {
		t.Fatal(err)
	}
	if path := os.Getenv("TUIMARK_SPEC"); path != "" {
		spec, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("TUIMARK_SPEC: %v", err)
		}
		if got := extractSpec22(t, string(spec)); got != string(frozen) {
			t.Fatalf("testdata/spec22.md differs from §22 of %s:\n--- SPEC ---\n%s\n--- frozen ---\n%s", path, got, frozen)
		}
	}
	return string(frozen)
}

// TestAGENTSStartsWithSpec22Verbatim: SPEC §3 says "AGENTS.md copy of §22"
// and the §22 heading itself says "(copy into the repo)". Markdown()'s own
// generated reference sections may follow it (§4 Phase 4: "generate
// AGENTS.md from the catalog"), but the §22 text itself is not up for
// reflowing, reordering, or otherwise editing (regression test for the
// Catalog block once being flattened onto one line).
func TestAGENTSStartsWithSpec22Verbatim(t *testing.T) {
	want := spec22Block(t)
	got := Markdown()
	if !strings.HasPrefix(got, want) {
		t.Errorf("Markdown() does not start with SPEC §22 verbatim.\n--- SPEC §22 ---\n%s\n--- Markdown() prefix ---\n%s", want, got[:min(len(got), len(want)+200)])
	}
}

// TestRuntimeSectionFollowsNotes: the "## Working on the Tuimark runtime"
// section is for agents changing the Go runtime itself, as opposed to
// authoring a .tui/.tcss/sample.json UI against it; it must exist, must
// come after "### Notes" (the last of the authoring-oriented sections), and
// must lead with a sentence explaining that split before covering the gate
// commands, package boundaries, hard rules, how to add things, and the
// Glyphrun conventions used under specs/glyphrun/.
func TestRuntimeSectionFollowsNotes(t *testing.T) {
	md := RepoMarkdown()

	notesIdx := strings.Index(md, "### Notes")
	runtimeIdx := strings.Index(md, "## Working on the Tuimark runtime")
	if notesIdx < 0 {
		t.Fatal("Markdown() is missing \"### Notes\"")
	}
	if runtimeIdx < 0 {
		t.Fatal("Markdown() is missing \"## Working on the Tuimark runtime\"")
	}
	if runtimeIdx < notesIdx {
		t.Errorf("\"## Working on the Tuimark runtime\" (at %d) must come after \"### Notes\" (at %d)", runtimeIdx, notesIdx)
	}

	runtimeSection := md[runtimeIdx:]
	for _, want := range []string{
		// lead sentence: this section is for a different audience than the rest of the file
		"is written for UI-authoring tasks",
		"editing the Go runtime itself",
		// gate commands
		"### Gate",
		"gofmt -l .",
		"go vet ./...",
		"go test ./...",
		"go test -race ./internal/host/",
		"./bin/tuimark test",
		"glyph run specs/glyphrun/*.yml --format md",
		"task verify",
		// package boundaries
		"### Package boundaries",
		"golden-manifest runner",
		"internal/ir",
		"internal/parse",
		"internal/css",
		"internal/layout",
		"internal/paint",
		"internal/dump",
		"internal/host",
		"internal/agentsdoc",
		"uses only the root `tuimark` package",
		"internal/layout` is imported by `internal/paint`, `internal/dump`, and `internal/host`",
		// hard rules
		"### Hard rules",
		"It is maintained outside this repository",
		"testdata/golden/spike/**` is frozen",
		"examples/inbox/{app.tui,theme.tcss,sample.json}` are verbatim SPEC §17 fixtures",
		"Closed vocabulary",
		"No Bubble Tea, Lipgloss",
		"No new dependencies",
		"a design-decision entry in the project notes",
		// documentation boundary (ADR: docs/ is the public site only)
		"### Documentation boundary",
		"`docs/` is the public Tuimark website",
		"~/notes/projects/tuimark/",
		// adding things
		"### Adding things",
		"A diagnostic code.",
		"A CSS property.",
		"A tag attribute.",
		"go run ./cmd/tuimark agents --repo > AGENTS.md",
		// glyphrun conventions
		"### Glyphrun conventions",
		"glyph spec verify <spec> --stamp",
		"Assertions wait for state",
		"--demo-workspace",
		"snapshot: <name>",
		// pointers
		"README.md",
	} {
		if !strings.Contains(runtimeSection, want) {
			t.Errorf("Markdown()'s \"## Working on the Tuimark runtime\" section is missing %q", want)
		}
	}
}

// TestKindGroupsMatchIRKinds: kindGroups (the §22 five-line grouping) must
// flatten to exactly ir.Kinds, so the Catalog block and the runtime's own
// tag catalog cannot silently drift apart.
func TestKindGroupsMatchIRKinds(t *testing.T) {
	var flat []string
	for _, g := range kindGroups {
		flat = append(flat, g...)
	}
	if strings.Join(flat, " ") != strings.Join(ir.Kinds, " ") {
		t.Errorf("kindGroups flattened = %v, want ir.Kinds = %v", flat, ir.Kinds)
	}
}

// TestNotesSectionCoversClarifications: the generated "### Notes" section
// must exist after the reference tables and must keep stating each
// documented clarification of behavior the SPEC leaves loose or silent
// (body trimming, modal open vs bind precedence, cross-axis stretch inside
// a modal, focused-list key handling, list rows not being focusable, Dump
// being a side-effect-free snapshot, Run and signals, built-in quit, and
// escape latency). It fails if the section is removed, reordered before
// the reference tables, or edited to drop one of these points.
func TestNotesSectionCoversClarifications(t *testing.T) {
	md := Markdown()

	refIdx := strings.Index(md, "### Diagnostic codes")
	notesIdx := strings.Index(md, "### Notes")
	if refIdx < 0 {
		t.Fatal("Markdown() is missing \"### Diagnostic codes\"")
	}
	if notesIdx < 0 {
		t.Fatal("Markdown() is missing \"### Notes\"")
	}
	if notesIdx < refIdx {
		t.Errorf("\"### Notes\" (at %d) must come after the reference tables (\"### Diagnostic codes\" at %d)", notesIdx, refIdx)
	}

	for _, want := range []string{
		// <text>/<button> body trimming
		"trimmed of XML whitespace",
		"leading/trailing blank lines are dropped",
		// modal open vs bind precedence
		"`open` wins outright and `bind` is ignored",
		// cross-axis stretch inside a modal
		"stretches to the modal's full remaining height",
		"on the row alone shrinks the buttons",
		// focused list key handling
		"consumes `up`, `down`, `home`, `end`, `pgup`, and `pgdn`",
		"never implicit for a list",
		// list rows, Dump snapshot, signals, built-in quit, esc latency
		"nothing takes focus",
		"side-effect-free snapshot",
		"non-nil error naming the signal",
		"fires the built-in `quit` action",
		"about 25ms",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("Markdown()'s Notes section is missing %q", want)
		}
	}
}

// TestNoDocsDecisionsReference guards the documentation boundary: design
// decisions live in the maintainer's notes, never in the repo's docs/, which
// is reserved for the public website.
func TestNoDocsDecisionsReference(t *testing.T) {
	if strings.Contains(Markdown(), "docs/decisions") {
		t.Fatal("AGENTS.md must not point at docs/decisions.md; decisions live outside the repo")
	}
}

// TestUserBriefingHasNoMaintainerSection: `tuimark agents` prints Markdown()
// for anyone authoring UIs, so it must not carry the repository-only
// maintainer section or the maintainer's notes location; those live only in
// the repository's AGENTS.md (RepoMarkdown).
func TestUserBriefingHasNoMaintainerSection(t *testing.T) {
	md := Markdown()
	for _, bad := range []string{"## Working on the Tuimark runtime", "~/notes/"} {
		if strings.Contains(md, bad) {
			t.Errorf("Markdown() (the `tuimark agents` output) contains %q", bad)
		}
	}
	if !strings.HasPrefix(RepoMarkdown(), strings.TrimRight(md, "\n")) {
		t.Error("RepoMarkdown() must start with Markdown()")
	}
}
