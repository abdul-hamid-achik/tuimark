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
//
// Exit codes: 0 ok, 1 I/O or crash, 2 validation errors.
package main

import (
	"fmt"
	"io"
	"os"
)

const version = "0.2.0-a"

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
	case "version", "--version", "-v":
		fmt.Fprintln(c.stdout, "tuimark", version)
		return 0
	case "help", "--help", "-h":
		fmt.Fprint(c.stdout, usage)
		return 0
	}
	fmt.Fprintf(c.stderr, "tuimark: unknown command %q\n\n%s", cmd, usage)
	return 1
}
