package css

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// Style is the computed style of one node.
type Style struct {
	Layout                 string // column | row | "" (tag default)
	Dock                   string
	Width, Height          ir.Scalar
	MinW, MinH, MaxW, MaxH ir.Scalar
	Flex                   float64 // nearest float64 of FlexLit (> 0 exactly when the literal is)
	FlexLit                string  // canonical literal of flex: its exact value (ir.ParseDecimalLit)
	FlexSet                bool
	// WidthUA, HeightUA, and FlexUA report that the winning width, height,
	// or flex declaration came from the built-in sheet (col/row 1fr,
	// spacer flex: 1), not from a presentational attribute, author CSS, or
	// style="". A dock ignores such defaults on its axis instead of
	// reporting V010 for sizes the author never wrote (layout.DockConflict).
	WidthUA, HeightUA, FlexUA bool
	Gap                       int
	Pad, Margin               [4]int // top right bottom left
	Align, Justify            string
	Overflow, ContentAlign    string
	Display                   string
	Color, Background         Color
	BorderColor, TitleColor   Color
	Border                    string
	Bold, Dim, Italic         bool
	Underline, Reverse        bool
	Wrap                      string
	Visibility                string
}

// Initial returns the initial (unstyled) computed values.
func Initial() Style {
	return Style{Display: "flex", Visibility: "visible", Border: "none", Justify: "start", Overflow: "hidden"}
}

// Inherited properties take the parent's computed value when not set.
var inherited = map[string]bool{
	"color": true, "bold": true, "dim": true, "italic": true, "underline": true,
	"reverse": true, "visibility": true, "wrap": true,
}

type propKind int

const (
	pEnum propKind = iota
	pSize
	pMinMax
	pNumber
	pGap
	pBox
	pColor
	pBool
)

type propSpec struct {
	kind   propKind
	values []string
}

var props = map[string]propSpec{
	"layout":        {pEnum, []string{"column", "row"}},
	"dock":          {pEnum, []string{"top", "right", "bottom", "left"}},
	"width":         {kind: pSize},
	"height":        {kind: pSize},
	"min-width":     {kind: pMinMax},
	"min-height":    {kind: pMinMax},
	"max-width":     {kind: pMinMax},
	"max-height":    {kind: pMinMax},
	"flex":          {kind: pNumber},
	"gap":           {kind: pGap},
	"padding":       {kind: pBox},
	"margin":        {kind: pBox},
	"align":         {pEnum, []string{"start", "center", "end", "stretch"}},
	"justify":       {pEnum, []string{"start", "center", "end", "space-between"}},
	"overflow":      {pEnum, []string{"hidden", "scroll"}},
	"content-align": {pEnum, []string{"start", "center", "end"}},
	"display":       {pEnum, []string{"flex", "none"}},
	"color":         {kind: pColor},
	"background":    {kind: pColor},
	"border-color":  {kind: pColor},
	"title-color":   {kind: pColor},
	"border":        {pEnum, []string{"none", "single", "double", "rounded", "thick"}},
	"bold":          {kind: pBool},
	"dim":           {kind: pBool},
	"italic":        {kind: pBool},
	"underline":     {kind: pBool},
	"reverse":       {kind: pBool},
	"wrap":          {pEnum, []string{"wrap", "nowrap", "truncate"}},
	"visibility":    {pEnum, []string{"visible", "hidden"}},
}

// PropertyNames lists the supported properties (for docs and AGENTS.md).
func PropertyNames() []string {
	out := make([]string, 0, len(props))
	for k := range props {
		out = append(out, k)
	}
	return out
}

// PropertyValues describes a property's accepted values for docs.
func PropertyValues(name string) string {
	p, ok := props[name]
	if !ok {
		return ""
	}
	switch p.kind {
	case pEnum:
		return strings.Join(p.values, " | ")
	case pSize, pMinMax:
		return "N | N% | Nfr | auto"
	case pNumber:
		return "number >= 0"
	case pGap:
		return "0-4"
	case pBox:
		return "1-4 cell values (T R B L)"
	case pColor:
		return "$token | var(--token) | ansi-name | #rgb | #rrggbb | default"
	case pBool:
		return "true | false"
	}
	return ""
}

// CheckDecl validates a declaration's syntax (tokens are resolved later).
func CheckDecl(prop, val string) error {
	val = strings.TrimSpace(val)
	if strings.HasPrefix(prop, "--") {
		return fmt.Errorf("custom property %s is only allowed in :root", prop)
	}
	p, ok := props[prop]
	if !ok {
		return fmt.Errorf("unknown property %q", prop)
	}
	if val == "" {
		return fmt.Errorf("%s: empty value", prop)
	}
	switch p.kind {
	case pEnum:
		for _, v := range p.values {
			if v == val {
				return nil
			}
		}
		return fmt.Errorf("%s: bad value %q (want %s)", prop, val, strings.Join(p.values, " | "))
	case pSize:
		if _, err := ir.ParseScalar(val); err != nil {
			return fmt.Errorf("%s: %v", prop, err)
		}
	case pMinMax:
		// SPEC §10.2 gives min-*/max-* the same value type as width/height:
		// Size (N | N% | Nfr | auto). Only cells and percent actually
		// constrain anything at layout time (§11.3 clamps min/max before
		// the fr split, so an fr bound has nothing to resolve against, and
		// auto has no floor/ceiling of its own to apply); auto and fr are
		// still accepted here and are simply no-ops, rather than rejecting
		// two of the four Size forms the SPEC's own grammar allows.
		if _, err := ir.ParseScalar(val); err != nil {
			return fmt.Errorf("%s: %v", prop, err)
		}
	case pNumber:
		if len(val) > ir.MaxDecimalLen {
			return fmt.Errorf("%s: a number has at most %d characters", prop, ir.MaxDecimalLen)
		}
		if _, ok := ir.ParseDecimal(val); !ok {
			return fmt.Errorf("%s: bad value %q (want a number >= 0)", prop, val)
		}
	case pGap:
		n, err := strconv.Atoi(val)
		if err != nil || n < 0 || n > 4 {
			return fmt.Errorf("gap: bad value %q (want one integer 0-4)", val)
		}
	case pBox:
		if _, err := parseBox(val); err != nil {
			return fmt.Errorf("%s: %v", prop, err)
		}
	case pColor:
		if err := checkColorSyntax(val); err != nil {
			return fmt.Errorf("%s: %v", prop, err)
		}
	case pBool:
		if val != "true" && val != "false" {
			return fmt.Errorf("%s: bad value %q (want true or false)", prop, val)
		}
	}
	return nil
}

func parseBox(val string) ([4]int, error) {
	fields := strings.Fields(val)
	if len(fields) < 1 || len(fields) > 4 {
		return [4]int{}, fmt.Errorf("want 1-4 cell values, got %q", val)
	}
	nums := make([]int, len(fields))
	for i, f := range fields {
		n, err := strconv.Atoi(f)
		if err != nil || n < 0 {
			return [4]int{}, fmt.Errorf("bad cell value %q (cells only, no units)", f)
		}
		nums[i] = n
	}
	switch len(nums) {
	case 1:
		return [4]int{nums[0], nums[0], nums[0], nums[0]}, nil
	case 2:
		return [4]int{nums[0], nums[1], nums[0], nums[1]}, nil
	case 3:
		return [4]int{nums[0], nums[1], nums[2], nums[1]}, nil
	}
	return [4]int{nums[0], nums[1], nums[2], nums[3]}, nil
}

// apply sets one validated declaration on st. Token resolution failures are
// returned as errors and leave st unchanged.
func apply(st *Style, prop, val string, tokens map[string]string) error {
	val = strings.TrimSpace(val)
	switch prop {
	case "layout":
		st.Layout = val
	case "dock":
		st.Dock = val
	case "width", "height", "min-width", "min-height", "max-width", "max-height":
		s, err := ir.ParseScalar(val)
		if err != nil {
			return err
		}
		switch prop {
		case "width":
			st.Width = s
		case "height":
			st.Height = s
		case "min-width":
			st.MinW = s
		case "min-height":
			st.MinH = s
		case "max-width":
			st.MaxW = s
		case "max-height":
			st.MaxH = s
		}
	case "flex":
		lit, f, ok := ir.ParseDecimalLit(val)
		if !ok {
			return fmt.Errorf("bad value %q (want a number >= 0)", val)
		}
		st.Flex, st.FlexLit, st.FlexSet = f, lit, true
	case "gap":
		st.Gap, _ = strconv.Atoi(val)
	case "padding":
		st.Pad, _ = parseBox(val)
	case "margin":
		st.Margin, _ = parseBox(val)
	case "align":
		st.Align = val
	case "justify":
		st.Justify = val
	case "overflow":
		st.Overflow = val
	case "content-align":
		st.ContentAlign = val
	case "display":
		st.Display = val
	case "color", "background", "border-color", "title-color":
		c, err := ResolveColor(val, tokens)
		if err != nil {
			return err
		}
		switch prop {
		case "color":
			st.Color = c
		case "background":
			st.Background = c
		case "border-color":
			st.BorderColor = c
		case "title-color":
			st.TitleColor = c
		}
	case "border":
		st.Border = val
	case "bold":
		st.Bold = val == "true"
	case "dim":
		st.Dim = val == "true"
	case "italic":
		st.Italic = val == "true"
	case "underline":
		st.Underline = val == "true"
	case "reverse":
		st.Reverse = val == "true"
	case "wrap":
		st.Wrap = val
	case "visibility":
		st.Visibility = val
	}
	return nil
}

// inherit copies an inherited property from parent to st.
func inherit(st *Style, parent *Style, prop string) {
	switch prop {
	case "color":
		st.Color = parent.Color
	case "bold":
		st.Bold = parent.Bold
	case "dim":
		st.Dim = parent.Dim
	case "italic":
		st.Italic = parent.Italic
	case "underline":
		st.Underline = parent.Underline
	case "reverse":
		st.Reverse = parent.Reverse
	case "visibility":
		st.Visibility = parent.Visibility
	case "wrap":
		st.Wrap = parent.Wrap
	}
}
