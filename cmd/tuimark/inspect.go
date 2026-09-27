package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"strconv"
	"strings"

	"github.com/abdul-hamid-achik/tuimark/internal/dump"
	"github.com/abdul-hamid-achik/tuimark/internal/host"
	"github.com/abdul-hamid-achik/tuimark/internal/layout"
)

// inspectJSON is the JSON output of `tuimark inspect --json` (SPEC v0.2b
// §15.7), members in the order cols, rows, ok, node, path, cell, pseudo,
// classes, style.
type inspectJSON struct {
	Cols    int              `json:"cols"`
	Rows    int              `json:"rows"`
	OK      bool             `json:"ok"`
	Node    dump.Node        `json:"node"`
	Path    string           `json:"path"`
	Cell    *dump.Cell       `json:"cell,omitempty"`
	Pseudo  []string         `json:"pseudo"`
	Classes []host.ClassInfo `json:"classes"`
	Style   []host.StyleInfo `json:"style"`
}

// parseAt parses --at X,Y: two 0-based decimal cell coordinates.
func parseAt(s string) (x, y int, err error) {
	xs, ys, ok := strings.Cut(s, ",")
	if !ok {
		return 0, 0, fmt.Errorf("--at %q: want X,Y", s)
	}
	for _, p := range []string{xs, ys} {
		if p == "" || strings.TrimLeft(p, "0123456789") != "" {
			return 0, 0, fmt.Errorf("--at %q: want X,Y (0-based decimal cells)", s)
		}
	}
	if x, err = strconv.Atoi(xs); err == nil {
		y, err = strconv.Atoi(ys)
	}
	if err != nil {
		return 0, 0, fmt.Errorf("--at %q: %v", s, err)
	}
	return x, y, nil
}

// cmdInspect is `tuimark inspect` (SPEC v0.2b §15.7): it renders one
// frame exactly as dump does with the same flags and explains one node of
// it. It is deterministic (MUST 13): no terminal, no TUIMARK_* variable.
func (c *cli) cmdInspect(args []string) int {
	fs := c.newFlags("inspect")
	at := fs.String("at", "", "X,Y: explain the node hit at this 0-based cell")
	id := fs.String("id", "", "explain the laid-out node with this id")
	cols := fs.Int("cols", 80, "terminal columns")
	rows := fs.Int("rows", 24, "terminal rows")
	data := fs.String("data", "", "JSON file bound as the data store")
	theme := fs.String("theme", "", "dark or light; overrides the document's theme")
	strict := fs.Bool("strict", false, "treat missing bind paths (B003) as errors")
	asJSON := fs.Bool("json", false, "print the report as JSON")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return 1
	}
	file, err := oneFile("inspect", pos)
	if err != nil {
		return c.fail(err)
	}
	if err := checkSize(*cols, *rows); err != nil {
		return c.fail(err)
	}
	atSet, idSet := false, false
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "at":
			atSet = true
		case "id":
			idSet = true
		}
	})
	if atSet == idSet {
		return c.fail(errors.New("inspect: give exactly one of --at X,Y and --id ID"))
	}
	target := host.InspectTarget{ID: *id}
	if atSet {
		x, y, err := parseAt(*at)
		if err != nil {
			return c.fail(fmt.Errorf("inspect: %v", err))
		}
		target = host.InspectTarget{At: true, X: x, Y: y}
	} else if *id == "" {
		return c.fail(errors.New("inspect: --id wants an id"))
	}
	app, err := load(file, *data)
	if err != nil {
		return c.fail(err)
	}
	if err := applyTheme(app, *theme, themeFlagSet(fs), "--theme"); err != nil {
		return c.fail(err)
	}
	app.SetStrict(*strict)
	f := app.Frame(*cols, *rows)
	in, err := app.Inspect(f, target)
	if err != nil {
		return c.fail(fmt.Errorf("inspect: %v", err))
	}
	ok := !f.Diags.HasErrors()
	if *asJSON {
		out := inspectJSON{Cols: f.Cols, Rows: f.Rows, OK: ok, Node: in.Node, Path: in.Path, Cell: in.Cell, Pseudo: in.Pseudo, Classes: in.Classes, Style: in.Style}
		b, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return c.fail(err)
		}
		c.stdout.Write(append(b, '\n'))
	} else {
		fmt.Fprint(c.stdout, inspectText(in))
	}
	if !ok {
		return 2
	}
	return 0
}

// inspectText is the informative text form of an inspection (SPEC v0.2b
// §15.7): the node line of §13.3, then path, cell, pseudo, one line per
// class, and one line per property whose origin is not initial, with its
// winning rule and, indented under it, the rules it overrode.
func inspectText(in *host.Inspection) string {
	var b strings.Builder
	b.WriteString(dump.NodeLine(in.Node, max(1, layout.Width(in.Node.ID)), len(in.Node.Tag)) + "\n")
	b.WriteString("path: " + in.Path + "\n")
	if in.Cell != nil {
		fmt.Fprintf(&b, "cell: %d,%d %q", in.Cell.X, in.Cell.Y, in.Cell.Ch)
		if in.Cell.ID != "" {
			b.WriteString(" id " + in.Cell.ID)
		}
		if in.Cell.W == 2 {
			b.WriteString(" w 2")
		}
		b.WriteString("\n")
	}
	if len(in.Pseudo) == 0 {
		b.WriteString("pseudo: -\n")
	} else {
		b.WriteString("pseudo: :" + strings.Join(in.Pseudo, " :") + "\n")
	}
	for _, c := range in.Classes {
		fmt.Fprintf(&b, "class .%s from %s", c.Name, c.From)
		if c.Guard != "" {
			fmt.Fprintf(&b, "=%q", c.Guard)
		}
		if c.Active {
			b.WriteString(" active")
		} else {
			b.WriteString(" inactive")
		}
		if !c.Used {
			b.WriteString(" (no rule names it)")
		}
		b.WriteString("\n")
	}
	for _, s := range in.Style {
		if s.Origin == "initial" {
			continue
		}
		v := "(none)"
		if s.Value != nil {
			v = *s.Value
		}
		fmt.Fprintf(&b, "%s: %s  %s", s.Prop, v, s.Origin)
		if s.Rule != nil {
			b.WriteString("  " + ruleText(*s.Rule))
		}
		b.WriteString("\n")
		for _, l := range s.Overridden {
			fmt.Fprintf(&b, "    overrides %s  %s  %s\n", l.Value, l.Origin, ruleText(l.Rule))
		}
	}
	return b.String()
}

func ruleText(r host.RuleInfo) string {
	file := r.File
	if file == "" {
		file = "<input>"
	}
	s := fmt.Sprintf("%s:%d:%d %s (specificity %d)", file, r.Line, r.Col, r.Selector, r.Specificity)
	if r.Media != "" {
		s += " @media " + r.Media
	}
	return s
}
