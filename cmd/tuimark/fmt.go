package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

func (c *cli) cmdFmt(args []string) int {
	fs := c.newFlags("fmt")
	write := fs.Bool("write", false, "rewrite the file in place")
	check := fs.Bool("check", false, "check whether the file is already formatted (no write)")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return 1
	}
	file, err := oneFile("fmt", pos)
	if err != nil {
		return c.fail(err)
	}
	src, err := os.ReadFile(file)
	if err != nil {
		return c.fail(err)
	}
	formatted, ferr := parse.Format(src)
	if ferr != nil {
		d := ir.Diagnostic{Severity: ir.Error, Code: "V005", File: filepath.Base(file), Msg: ferr.Error()}
		if se, ok := ferr.(*parse.SyntaxError); ok {
			d.Line, d.Col = se.Line, se.Col
			d.Msg = se.Msg
		}
		fmt.Fprintln(c.stderr, d.String())
		return 2
	}
	switch {
	case *check:
		if string(src) == formatted {
			return 0
		}
		fmt.Fprintln(c.stdout, file)
		return 2
	case *write:
		if err := os.WriteFile(file, []byte(formatted), 0o644); err != nil {
			return c.fail(err)
		}
		return 0
	default:
		fmt.Fprint(c.stdout, formatted)
		return 0
	}
}
