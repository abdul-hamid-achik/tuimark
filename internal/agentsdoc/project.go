package agentsdoc

import "strings"

// projectEdit is one exact replacement that turns the briefing of
// Markdown() into the project briefing (SPEC v0.4a §15.11). Every Old must
// occur in Markdown() exactly once; TestProjectEditsApply fails when a
// change to the briefing makes one stale, so the project briefing cannot
// quietly keep a repository path.
type projectEdit struct{ Old, New string }

var projectEdits = []projectEdit{
	// The two lists §15.11 replaces: "Allowed edits" becomes the project's
	// files and "Forbidden" loses the repository paths.
	{"## Allowed edits\n- *.tui\n- *.tcss\n- examples/**/sample.json\n\n",
		"## Allowed edits\n- *.tui\n- *.tcss\n- sample.json\n- main.go or parent.ts, when the project has one\n\n"},
	{"- internal/**\n- cmd/** except flag wiring\n", ""},
	// The line about building ./bin/tuimark from the repository.
	{"`tuimark` in the Loop above is `./bin/tuimark` after `go build -o bin/tuimark ./cmd/tuimark`, or `go run ./cmd/tuimark` without building.\n\n", ""},
	// Pointers into the repository.
	{" (validated by `schema/ir.v0.2.json`)", ""},
	{", validated by `schema/ir.v0.3.json` (the same shape as IR 0.2 in 0.3a)", " (the same shape as IR 0.2 in 0.3a)"},
	{"`examples/host-ts` is a Bun parent: it passes", "A parent written for Bun passes"},
	{" (the SPEC itself is not edited; see README.md's \"Language notes\" section for the same points in more detail)", ""},
	{"the SPEC §17 inbox fixture's `theme.tcss` (`list:focus`/`input:focus { border-color: $focus; }`) has", "a stylesheet rule such as `list:focus`/`input:focus { border-color: $focus; }` has"},
	{" (as `examples/agent`'s `#composer`/`#transcript` and `examples/dashboard`'s bordered panels do)", ""},
}

// ProjectMarkdown is the project briefing that `tuimark new` writes as a
// project's AGENTS.md and `tuimark agents --project` prints (SPEC v0.4a
// §15.11): the briefing of Markdown() adapted to a project outside the
// repository. It keeps §22's block except for two lists: "Allowed edits"
// becomes the project's files and "Forbidden" loses the repository paths.
// The Reference and the Notes follow, without the line about building
// bin/tuimark and without any pointer into the repository. It is the same
// text for every project, with or without main.go or parent.ts.
func ProjectMarkdown() string {
	s := Markdown()
	for _, e := range projectEdits {
		s = strings.Replace(s, e.Old, e.New, 1)
	}
	return s
}
