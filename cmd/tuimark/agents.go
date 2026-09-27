package main

import (
	"fmt"

	"github.com/abdul-hamid-achik/tuimark/internal/agentsdoc"
)

func (c *cli) cmdAgents(args []string) int {
	fs := c.newFlags("agents")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return 1
	}
	if len(pos) != 0 {
		return c.fail(fmt.Errorf("tuimark agents: takes no arguments"))
	}
	fmt.Fprint(c.stdout, agentsdoc.Markdown())
	return 0
}
