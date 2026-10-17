package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/abdul-hamid-achik/tuimark/internal/lsp"
)

// cmdLSP is `tuimark lsp` (SPEC v0.4a §15.12): a Language Server Protocol
// server on stdin/stdout for .tui and .tcss files. It takes no arguments
// and no flags, and reads no environment variable. The exit code is the
// server's: 0 after `shutdown` and `exit` (or the end of input), else 1.
func (c *cli) cmdLSP(args []string) int {
	fs := c.newFlags("lsp")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return 1
	}
	if len(pos) != 0 {
		return c.fail(fmt.Errorf("tuimark lsp: takes no arguments"))
	}
	// As in mcp: a client that closes its end of stdout ends the server
	// with exit 1 and a "tuimark: " line, not with SIGPIPE.
	signal.Ignore(syscall.SIGPIPE)
	return lsp.Serve(os.Stdin, c.stdout, c.stderr, lsp.Options{Version: version})
}
