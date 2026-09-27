package main

import (
	"fmt"

	"github.com/abdul-hamid-achik/tuimark/internal/agentsdoc"
)

func (c *cli) cmdAgents(args []string) int {
	fs := c.newFlags("agents")
	repo := fs.Bool("repo", false, "print the Tuimark repository's own AGENTS.md (adds the maintainer section)")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return 1
	}
	if len(pos) != 0 {
		return c.fail(fmt.Errorf("tuimark agents: takes no arguments"))
	}
	if *repo {
		fmt.Fprint(c.stdout, agentsdoc.RepoMarkdown())
		return 0
	}
	fmt.Fprint(c.stdout, agentsdoc.Markdown())
	return 0
}
