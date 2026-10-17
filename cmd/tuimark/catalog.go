package main

import (
	"encoding/json"
	"fmt"

	"github.com/abdul-hamid-achik/tuimark/internal/catalog"
)

// cmdCatalog is `tuimark catalog` (SPEC v0.4a §15.10): the closed
// vocabulary as one JSON object, generated from the runtime's own tables.
// It takes no flags, reads no environment variable, and exits 0, or 1 when
// stdout cannot be written.
func (c *cli) cmdCatalog(args []string) int {
	fs := c.newFlags("catalog")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return 1
	}
	if len(pos) != 0 {
		return c.fail(fmt.Errorf("tuimark catalog: takes no arguments"))
	}
	// The diagnostics' prose holds <tui> and the like: do not escape it.
	enc := json.NewEncoder(c.stdout)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(catalog.Build(version)); err != nil {
		return c.fail(err)
	}
	return 0
}
