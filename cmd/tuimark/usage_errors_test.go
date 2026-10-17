package main

import (
	"os"
	"strings"
	"testing"
)

// `fmt FILE --write --check` used to run only the check and say nothing
// about the ignored --write. The combination is a usage error (exit 1),
// raised before the file is read, and leaves the file as it was.
func TestFmtWriteAndCheckAreMutuallyExclusive(t *testing.T) {
	const unformatted = "<tui   version=\"1\"><screen><text>hi</text></screen></tui>"
	file := writeTemp(t, "a.tui", unformatted)
	for _, args := range [][]string{
		{"fmt", file, "--write", "--check"},
		{"fmt", file, "--check", "--write"},
		{"fmt", "no-such-file.tui", "--write", "--check"}, // refused before the file is read
	} {
		code, out, errw := runCLI(args...)
		if code != 1 || out != "" || errw != "tuimark: fmt: --write and --check are mutually exclusive\n" {
			t.Errorf("%v: exit %d stdout %q stderr %q, want the mutual-exclusion usage error", args, code, out, errw)
		}
	}
	if got, err := os.ReadFile(file); err != nil || string(got) != unformatted {
		t.Errorf("the file changed: %q (%v)", got, err)
	}
	// Each flag alone still works.
	if code, _, _ := runCLI("fmt", file, "--check"); code != 2 {
		t.Errorf("fmt --check on an unformatted file: exit %d, want 2", code)
	}
	if code, _, errw := runCLI("fmt", file, "--write"); code != 0 {
		t.Errorf("fmt --write: exit %d, want 0; stderr=%s", code, errw)
	}
}

// `tuimark version` takes no arguments, like agents and mcp: an extra one
// (in any of the three spellings) is a usage error, not a quiet exit 0.
func TestVersionTakesNoArguments(t *testing.T) {
	for _, args := range [][]string{
		{"version", "extra"},
		{"--version", "extra"},
		{"-v", "--json"},
		{"version", "a", "b"},
	} {
		code, out, errw := runCLI(args...)
		if code != 1 || out != "" || errw != "tuimark: tuimark version: takes no arguments\n" {
			t.Errorf("%v: exit %d stdout %q stderr %q, want the no-arguments usage error", args, code, out, errw)
		}
	}
}

// The usage text lists every flag a command takes, `agents --repo`
// included (cmdAgents defines it), and `agents` still rejects arguments.
func TestUsageMentionsAgentsRepo(t *testing.T) {
	if !strings.Contains(usage, "tuimark agents   [--repo]") {
		t.Errorf("usage text is missing agents' --repo flag:\n%s", usage)
	}
	code, out, errw := runCLI("agents", "--repo")
	if code != 0 || !strings.Contains(out, "Working on the Tuimark runtime") || errw != "" {
		t.Errorf("agents --repo: exit %d, stderr %q", code, errw)
	}
	if code, _, _ := runCLI("agents", "extra"); code != 1 {
		t.Errorf("agents extra: exit %d, want 1", code)
	}
}
