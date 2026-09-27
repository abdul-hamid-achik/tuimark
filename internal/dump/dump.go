// Package dump builds the machine-readable frame snapshot (SPEC §13.2) and
// its human text form (§13.3). Agents parse the JSON; humans read the text.
package dump

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/layout"
	"github.com/abdul-hamid-achik/tuimark/internal/paint"
)

// Node is one laid-out node in the dump.
type Node struct {
	ID       string `json:"id,omitempty"`
	Tag      string `json:"tag"`
	X        int    `json:"x"`
	Y        int    `json:"y"`
	W        int    `json:"w"`
	H        int    `json:"h"`
	Text     string `json:"text,omitempty"`
	Key      string `json:"key,omitempty"`
	Focused  bool   `json:"focused,omitempty"`
	Selected bool   `json:"selected,omitempty"`
}

// Cell is one grid cell with the id of the node that owns it (--cells).
type Cell struct {
	X  int    `json:"x"`
	Y  int    `json:"y"`
	Ch string `json:"ch"`
	ID string `json:"id,omitempty"`
}

// Dump is the frame IR.
type Dump struct {
	Cols   int             `json:"cols"`
	Rows   int             `json:"rows"`
	OK     bool            `json:"ok"`
	Focus  *string         `json:"focus"`
	Errors []ir.Diagnostic `json:"errors"`
	Nodes  []Node          `json:"nodes"`
	Grid   []string        `json:"grid"`
	Cells  []Cell          `json:"cells,omitempty"`
}

// Build assembles a dump from a painted frame.
func Build(cols, rows int, root *layout.Box, modals []*layout.Box, g *paint.Grid, diags ir.Diags, focus string, cells bool) *Dump {
	d := &Dump{Cols: cols, Rows: rows, OK: !diags.HasErrors(), Errors: []ir.Diagnostic(diags), Nodes: []Node{}}
	if d.Errors == nil {
		d.Errors = []ir.Diagnostic{}
	}
	if focus != "" {
		f := focus
		d.Focus = &f
	}
	add := func(b *layout.Box) {
		b.Walk(func(n *layout.Box) {
			if !n.Laid {
				return
			}
			dn := Node{ID: n.ID, Tag: n.Tag, X: n.X, Y: n.Y, W: n.W, H: n.H, Focused: n.Focused, Selected: n.Selected, Key: n.Key}
			switch n.Kind {
			case "text", "button":
				dn.Text = n.Text
			case "input":
				if n.Secret {
					dn.Text = strings.Repeat("•", layout.Width(n.Text))
				} else {
					dn.Text = n.Text
				}
			}
			d.Nodes = append(d.Nodes, dn)
		})
	}
	if root != nil {
		add(root)
	}
	for _, m := range modals {
		add(m)
	}
	if g == nil {
		g = paint.NewGrid(cols, rows)
	}
	d.Grid = g.Lines()
	if cells {
		for y := 0; y < g.H; y++ {
			for x := 0; x < g.W; x++ {
				c := g.At(x, y)
				d.Cells = append(d.Cells, Cell{X: x, Y: y, Ch: string(c.Ch), ID: c.Owner})
			}
		}
	}
	return d
}

// JSON renders the dump as indented JSON.
func JSON(d *Dump) ([]byte, error) {
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// Text renders the human dump format (SPEC §13.3).
func Text(d *Dump) string {
	var b strings.Builder
	fmt.Fprintf(&b, "=== grid %dx%d ===\n", d.Cols, d.Rows)
	for _, l := range d.Grid {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	b.WriteString("=== nodes ===\n")
	idw, tagw := 1, 1
	for _, n := range d.Nodes {
		idw = max(idw, layout.Width(n.ID))
		tagw = max(tagw, len(n.Tag))
	}
	for _, n := range d.Nodes {
		id := n.ID
		if id == "" {
			id = "-"
		}
		line := fmt.Sprintf("%-*s  %-*s  %dx%d @(%d,%d)", idw, id, tagw, n.Tag, n.W, n.H, n.X, n.Y)
		if n.Focused {
			line += " focus"
		}
		if n.Selected {
			line += " selected"
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	b.WriteString("=== errors ===\n")
	if len(d.Errors) == 0 {
		b.WriteString("ok\n")
	}
	for _, e := range d.Errors {
		b.WriteString(e.String())
		b.WriteByte('\n')
	}
	return b.String()
}
