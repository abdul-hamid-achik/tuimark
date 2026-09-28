// Command tuimark validates, dumps, previews, plays, and formats Tuimark
// documents.
//
//	tuimark dump     FILE [--cols 80] [--rows 24] [--format text|json] [--data FILE.json]
//	                      [--cells] [--styles] [--strict] [--theme dark|light]
//	tuimark validate FILE [--json] [--strict] [--catalog FILE] [--data FILE.json] [--theme dark|light]
//	tuimark preview  FILE [--cols 80] [--rows 24] [--data FILE.json] [--watch] [--theme dark|light]
//	                      [--color truecolor|256|16|none]
//	tuimark play     FILE [--cols 80] [--rows 24] [--data FILE.json] [--theme dark|light]
//	                      [--input STEPS | --script FILE.ndjson]
//	                      [--format text|json] [--cells] [--styles] [--frames] [--strict]
//	tuimark fmt      FILE [--write] [--check]
//	tuimark ir       FILE
//	tuimark agents
//	tuimark test     [DIR] [--update] [--allow-breaking]
//	tuimark inspect  FILE (--at X,Y | --id ID) [--cols 80] [--rows 24] [--data FILE.json]
//	                      [--theme dark|light] [--strict] [--json]
//	tuimark host     FILE [--data FILE.json] [--theme dark|light] [--reply-timeout 5s]
//	tuimark mcp                                — a Model Context Protocol server on stdin/stdout
//
// Exit codes: 0 ok, 1 I/O or crash, 2 validation errors.
package main

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// Build metadata that `tuimark version` prints. version defaults to the
// release this source tree is; release builds set all three at link time
// (see .goreleaser.yaml):
//
//	go build -ldflags "-X main.version=0.3.1 -X main.commit=abc1234 -X main.date=2026-01-02T15:04:05Z" ./cmd/tuimark
//
// A plain `go build` or `go install` leaves commit and date empty, and
// `tuimark version` then prints the version alone.
var (
	version = "0.3.1"
	commit  = ""
	date    = ""
)

// versionLine is the `tuimark version` output: "tuimark VERSION", followed
// by "(commit C, built D)" with whichever of commit and date are set.
func versionLine() string {
	line := "tuimark " + version
	var meta []string
	if commit != "" {
		meta = append(meta, "commit "+commit)
	}
	if date != "" {
		meta = append(meta, "built "+date)
	}
	if len(meta) > 0 {
		line += " (" + strings.Join(meta, ", ") + ")"
	}
	return line
}

const usage = `tuimark — a view language for terminals

usage:
  tuimark dump     FILE [--cols 80] [--rows 24] [--format text|json] [--data FILE.json]
                        [--cells] [--styles] [--strict] [--theme dark|light]
  tuimark validate FILE [--json] [--strict] [--catalog FILE] [--data FILE.json] [--theme dark|light]
  tuimark preview  FILE [--cols 80] [--rows 24] [--data FILE.json] [--watch] [--theme dark|light]
                        [--color truecolor|256|16|none]
  tuimark play     FILE [--cols 80] [--rows 24] [--data FILE.json] [--theme dark|light]
                        [--input STEPS | --script FILE.ndjson]
                        [--format text|json] [--cells] [--styles] [--frames] [--strict]
  tuimark fmt      FILE [--write] [--check]
  tuimark ir       FILE                       print the source IR as JSON
  tuimark agents                              print AGENTS.md generated from the catalog
  tuimark test     [DIR] [--update] [--allow-breaking]
                                               check dump goldens under DIR (default testdata/golden)
  tuimark inspect  FILE (--at X,Y | --id ID) [--cols 80] [--rows 24] [--data FILE.json]
                        [--theme dark|light] [--strict] [--json]
                                               explain one node: path, classes, pseudo-classes, styles
  tuimark host     FILE [--data FILE.json] [--theme dark|light] [--reply-timeout 5s]
                                               run in the terminal for a parent program (fd 3 in, fd 4 out)
  tuimark mcp                                 a Model Context Protocol server on stdin/stdout
  tuimark version

exit codes: 0 ok, 1 I/O or crash, 2 validation errors
`

// cli carries the streams every command writes to, so tests can capture
// output instead of the process's real stdout/stderr.
type cli struct {
	stdout io.Writer
	stderr io.Writer
}

func main() {
	c := &cli{stdout: os.Stdout, stderr: os.Stderr}
	os.Exit(c.run(os.Args[1:]))
}

func (c *cli) run(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(c.stderr, usage)
		return 1
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "dump":
		return c.cmdDump(rest)
	case "validate":
		return c.cmdValidate(rest)
	case "preview":
		return c.cmdPreview(rest)
	case "play":
		return c.cmdPlay(rest)
	case "fmt":
		return c.cmdFmt(rest)
	case "ir":
		return c.cmdIR(rest)
	case "agents":
		return c.cmdAgents(rest)
	case "test":
		return c.cmdTest(rest)
	case "inspect":
		return c.cmdInspect(rest)
	case "host":
		return c.cmdHost(rest)
	case "mcp":
		return c.cmdMCP(rest)
	case "version", "--version", "-v":
		fmt.Fprintln(c.stdout, versionLine())
		return 0
	case "help", "--help", "-h":
		fmt.Fprint(c.stdout, usage)
		return 0
	}
	fmt.Fprintf(c.stderr, "tuimark: unknown command %q\n\n%s", cmd, usage)
	return 1
}
