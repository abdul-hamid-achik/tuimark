// Package dump builds the machine-readable frame snapshot (SPEC §13.2) and
// its human text form (§13.3). Agents parse the JSON; humans read the text.
package dump

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
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
	// Scroll is set on every laid-out viewport (a scroll, a list, or an
	// overflow: scroll container), also when it has nothing to scroll, and
	// is nil on every other node (SPEC v0.2 §13.2).
	Scroll *Scroll `json:"scroll,omitempty"`
}

// Scroll is a viewport's scroll state (SPEC v0.2 §13.2). Its members come
// in axis pairs: Y and H are set exactly when the viewport scrolls on y,
// X and W exactly when it scrolls on x, so an offset of 0 is still
// written. X/Y is the clamped offset the frame used; W/H is the extent the
// offset scrolls over, the larger of the content-box size and the natural
// content size on that axis. The JSON members come in the order x, y, w, h.
type Scroll struct {
	X *int `json:"x,omitempty"`
	Y *int `json:"y,omitempty"`
	W *int `json:"w,omitempty"`
	H *int `json:"h,omitempty"`
}

// scrollOf returns the scroll state of a laid-out box, or nil when b is
// not a viewport. Layout has already clamped b's offsets and set its
// content extents (scrollLayout runs for every laid-out viewport, with
// children or without).
func scrollOf(b *layout.Box) *Scroll {
	if !b.Scrolls() {
		return nil
	}
	sx, sy := b.ScrollAxes()
	s := &Scroll{}
	if sx {
		x, w := b.ScrollX, b.ContentW
		s.X, s.W = &x, &w
	}
	if sy {
		y, h := b.ScrollY, b.ContentH
		s.Y, s.H = &y, &h
	}
	return s
}

// Cell is one grid column with the id of the node that owns it (--cells,
// SPEC v0.2 §13.2). A lead cell carries its grapheme cluster in Ch (" " for
// a blank cell) and W 2 when the cluster is two columns wide; the column
// after a wide lead is a continuation cell: Ch "", its lead's ID, no W.
// There is one Cell per column, so indexing cells by (x, y) works with wide
// text.
type Cell struct {
	X  int    `json:"x"`
	Y  int    `json:"y"`
	Ch string `json:"ch"`
	ID string `json:"id,omitempty"`
	W  int    `json:"w,omitempty"`
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
	// Wide is true when some row holds a complex cluster (SPEC v0.2
	// §11.5.1: two columns wide or more than one code point). Then
	// grid[y] indexed by rune no longer matches column x; use Cells.
	Wide  bool   `json:"wide,omitempty"`
	Cells []Cell `json:"cells,omitempty"`
	// Theme and Styles are set by the CLI (--styles, SPEC v0.2 §13.2,
	// §15.3) after Build: Build itself never touches the terminal or an
	// override, so it stays a pure function of a rendered frame.
	Theme  string   `json:"theme,omitempty"`
	Styles [][]Span `json:"styles,omitempty"`
}

// Span is one maximal run of adjacent columns sharing a foreground,
// background, and attribute set (SPEC v0.2 §13.2 "--styles"). Colors are
// canonical (before downsampling): "#rrggbb" lowercase, an ANSI name, or
// "default" for an unset color, since both render as the terminal default.
type Span struct {
	X  int      `json:"x"`
	W  int      `json:"w"`
	FG string   `json:"fg"`
	BG string   `json:"bg"`
	A  []string `json:"a,omitempty"`
}

// attrBits is the fixed attribute order of SPEC v0.2 §13.2 ("a" lists the
// attributes that are on, in the order bold, dim, italic, underline,
// reverse").
var attrBits = []struct {
	bit  uint8
	name string
}{
	{paint.Bold, "bold"},
	{paint.Dim, "dim"},
	{paint.Italic, "italic"},
	{paint.Underline, "underline"},
	{paint.Reverse, "reverse"},
}

// colorString renders a cell color canonically: "default" for an unset
// color (ColorNone) as well as for the terminal's own default (ColorDefault),
// since both paint as the terminal default; otherwise css.Color.String().
func colorString(c css.Color) string {
	if !c.IsSet() {
		return "default"
	}
	return c.String()
}

func spanAttrs(a uint8) []string {
	var out []string
	for _, e := range attrBits {
		if a&e.bit != 0 {
			out = append(out, e.name)
		}
	}
	return out
}

func sameAttrs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// BuildStyles partitions every row of g into maximal runs of columns that
// share a foreground, background, and attribute set (SPEC v0.2 §13.2
// "--styles"). A continuation column always carries its lead's style
// (paint.Grid's settle), so it is never a span boundary on its own.
func BuildStyles(g *paint.Grid) [][]Span {
	rows := make([][]Span, g.H)
	for y := 0; y < g.H; y++ {
		spans := []Span{}
		for x := 0; x < g.W; x++ {
			c := g.At(x, y)
			fg, bg, a := colorString(c.FG), colorString(c.BG), spanAttrs(c.Attrs)
			if n := len(spans); n > 0 {
				last := &spans[n-1]
				if last.FG == fg && last.BG == bg && sameAttrs(last.A, a) {
					last.W++
					continue
				}
			}
			spans = append(spans, Span{X: x, W: 1, FG: fg, BG: bg, A: a})
		}
		rows[y] = spans
	}
	return rows
}

// Event is one action tuimark play dispatched (SPEC v0.2 §15.4). Keys is
// always a non-nil map, so it marshals as {} rather than null; encoding/json
// sorts its members by name, matching the §13.2 serialization rule.
type Event struct {
	Step   int            `json:"step"`
	Action string         `json:"action"`
	Source string         `json:"source"`
	Keys   map[string]any `json:"keys"`
	Value  any            `json:"value"`
}

// PlayFrame is one applied step's settled frame (tuimark play --frames).
// Its Dump never carries Events or Frames of its own (SPEC v0.2 §23.3's note
// under the schema): it is built with Build, like any other dump.
type PlayFrame struct {
	Step   int     `json:"step"`
	Input  string  `json:"input"`
	Events []Event `json:"events"`
	Dump   *Dump   `json:"dump"`
}

// Play is the JSON object `tuimark play --format json` prints: a Dump plus
// the actions that fired (SPEC v0.2 §15.4). The embedded *Dump's fields are
// promoted ahead of Events and Frames by encoding/json, matching the §13.2
// top-level field order.
type Play struct {
	*Dump
	Events []Event     `json:"events"`
	Frames []PlayFrame `json:"frames,omitempty"`
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
			dn := Node{ID: n.ID, Tag: n.Tag, X: n.X, Y: n.Y, W: n.W, H: n.H, Focused: n.Focused, Selected: n.Selected, Key: n.Key, Scroll: scrollOf(n)}
			switch n.Kind {
			case "text", "button":
				dn.Text = n.Text
			case "input":
				dn.Text = layout.InputShown(n) // one • per cluster when secret
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
	d.Wide = g.HasComplex()
	if cells {
		d.Cells = make([]Cell, 0, g.W*g.H)
		for y := 0; y < g.H; y++ {
			for x := 0; x < g.W; x++ {
				c := g.At(x, y)
				dc := Cell{X: x, Y: y, Ch: c.Grapheme(), ID: c.Owner}
				switch {
				case c.Cont && x > 0:
					dc.ID = g.At(x-1, y).Owner // a continuation belongs to its lead
				case c.Wide:
					dc.W = 2
				}
				d.Cells = append(d.Cells, dc)
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
