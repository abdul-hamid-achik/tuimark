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
	// The version="2" properties (SPEC §10.2, §10.3): GridColumns is the
	// computed grid-columns (1-12, initial 1), GridMinWidth the computed
	// grid-min-width (0 when unset), Scrollbar none | auto, Bar block |
	// eighths. None of them is inherited.
	GridColumns  int
	GridMinWidth int
	Scrollbar    string
	Bar          string
}

// Initial returns the initial (unstyled) computed values.
func Initial() Style {
	return Style{Display: "flex", Visibility: "visible", Border: "none", Justify: "start", Overflow: "hidden", GridColumns: 1, Scrollbar: "none", Bar: "block"}
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
	pGridColumns
	pGridMinWidth
)

type propSpec struct {
	kind   propKind
	values []string
	// v2 marks a property only version="2" stylesheets accept (SPEC §5.1).
	v2 bool
}

// layoutGrid is the value of layout that only version="2" stylesheets
// accept (SPEC §5.1, §11.7).
const layoutGrid = "grid"

var props = map[string]propSpec{
	"layout":        {kind: pEnum, values: []string{"column", "row"}},
	"dock":          {kind: pEnum, values: []string{"top", "right", "bottom", "left"}},
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
	"align":         {kind: pEnum, values: []string{"start", "center", "end", "stretch"}},
	"justify":       {kind: pEnum, values: []string{"start", "center", "end", "space-between"}},
	"overflow":      {kind: pEnum, values: []string{"hidden", "scroll"}},
	"content-align": {kind: pEnum, values: []string{"start", "center", "end"}},
	"display":       {kind: pEnum, values: []string{"flex", "none"}},
	"color":         {kind: pColor},
	"background":    {kind: pColor},
	"border-color":  {kind: pColor},
	"title-color":   {kind: pColor},
	"border":        {kind: pEnum, values: []string{"none", "single", "double", "rounded", "thick"}},
	"bold":          {kind: pBool},
	"dim":           {kind: pBool},
	"italic":        {kind: pBool},
	"underline":     {kind: pBool},
	"reverse":       {kind: pBool},
	"wrap":          {kind: pEnum, values: []string{"wrap", "nowrap", "truncate"}},
	"visibility":    {kind: pEnum, values: []string{"visible", "hidden"}},
	// version="2" (SPEC §10.2, §10.3).
	"grid-columns":   {kind: pGridColumns, v2: true},
	"grid-min-width": {kind: pGridMinWidth, v2: true},
	"scrollbar":      {kind: pEnum, values: []string{"none", "auto"}, v2: true},
	"bar":            {kind: pEnum, values: []string{"block", "eighths"}, v2: true},
}

// PropertyOrder is every TCSS property in the order of SPEC §10.2 then
// §10.3, the order `tuimark inspect` lists them in (§15.7).
var PropertyOrder = []string{
	"layout", "grid-columns", "grid-min-width", "scrollbar", "dock",
	"width", "height", "min-width", "min-height", "max-width", "max-height",
	"flex", "gap", "padding", "margin", "align", "justify", "overflow",
	"content-align", "display",
	"color", "background", "border-color", "title-color", "border",
	"bold", "dim", "italic", "underline", "reverse", "wrap", "visibility", "bar",
}

// PropertyV2 reports whether name is a property only version="2"
// stylesheets accept.
func PropertyV2(name string) bool { return props[name].v2 }

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
		if name == "layout" {
			return strings.Join(p.values, " | ") + " | grid (version=\"2\")"
		}
		return strings.Join(p.values, " | ")
	case pGridColumns:
		return "integer 1-12"
	case pGridMinWidth:
		return "integer >= 1"
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

// CheckDecl validates a declaration's syntax (tokens are resolved later)
// in a version="1" document: CheckDeclIn with v2 false.
func CheckDecl(prop, val string) error { return CheckDeclIn(prop, val, false) }

// CheckDeclIn validates a declaration's syntax for a document of the
// given version (SPEC §5.1): a version="1" document gets V003 with the
// version hint for a version="2" property or for layout: grid.
func CheckDeclIn(prop, val string, v2 bool) error {
	val = strings.TrimSpace(val)
	if strings.HasPrefix(prop, "--") {
		return fmt.Errorf("custom property %s is only allowed in :root", prop)
	}
	p, ok := props[prop]
	if !ok {
		return fmt.Errorf("unknown property %q", prop)
	}
	if p.v2 && !v2 {
		return fmt.Errorf("property %q%s", prop, ir.VersionHint)
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
		if prop == "layout" && val == layoutGrid {
			if v2 {
				return nil
			}
			return fmt.Errorf("%s: value %q (want %s)%s", prop, val, strings.Join(p.values, " | "), ir.VersionHint)
		}
		want := strings.Join(p.values, " | ")
		if prop == "layout" && v2 {
			want += " | " + layoutGrid
		}
		return fmt.Errorf("%s: bad value %q (want %s)", prop, val, want)
	case pGridColumns:
		n, ok := wholeNumber(val)
		if !ok || n < 1 || n > 12 {
			return fmt.Errorf("grid-columns: bad value %q (want an integer 1-12)", val)
		}
	case pGridMinWidth:
		n, ok := wholeNumber(val)
		if !ok || n < 1 {
			return fmt.Errorf("grid-min-width: bad value %q (want an integer >= 1)", val)
		}
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

// maxGridMinWidth bounds a computed grid-min-width: a larger literal is
// still a valid integer >= 1, and any width that large gives one column.
const maxGridMinWidth = 1 << 30

// wholeNumber parses a plain non-negative decimal integer (digits only,
// no sign). A literal too large for maxGridMinWidth gives that bound.
func wholeNumber(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(s)
	if err != nil || n > maxGridMinWidth {
		return maxGridMinWidth, true
	}
	return n, true
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
	case "grid-columns":
		st.GridColumns, _ = wholeNumber(val)
	case "grid-min-width":
		st.GridMinWidth, _ = wholeNumber(val)
	case "scrollbar":
		st.Scrollbar = val
	case "bar":
		st.Bar = val
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

// FormatProp is the computed value of prop in st as text, the way `tuimark
// inspect` reports it (SPEC §15.7): sizes as N, N%, Nfr, or auto; boxes as
// "T R B L"; colors canonical; booleans "true"/"false". ok is false when
// the property has no computed value (an unset size, color, dock, align,
// content-align, wrap, or flex, a layout left to the tag, an unset
// grid-min-width).
func FormatProp(st Style, prop string) (string, bool) {
	str := func(s string) (string, bool) { return s, s != "" }
	size := func(s ir.Scalar) (string, bool) { return str(s.String()) }
	color := func(c Color) (string, bool) {
		if !c.IsSet() {
			return "", false
		}
		return c.String(), true
	}
	box := func(b [4]int) (string, bool) {
		return fmt.Sprintf("%d %d %d %d", b[0], b[1], b[2], b[3]), true
	}
	boolean := func(v bool) (string, bool) { return strconv.FormatBool(v), true }
	switch prop {
	case "layout":
		return str(st.Layout)
	case "grid-columns":
		return strconv.Itoa(st.GridColumns), true
	case "grid-min-width":
		if st.GridMinWidth == 0 {
			return "", false
		}
		return strconv.Itoa(st.GridMinWidth), true
	case "scrollbar":
		return str(st.Scrollbar)
	case "dock":
		return str(st.Dock)
	case "width":
		return size(st.Width)
	case "height":
		return size(st.Height)
	case "min-width":
		return size(st.MinW)
	case "min-height":
		return size(st.MinH)
	case "max-width":
		return size(st.MaxW)
	case "max-height":
		return size(st.MaxH)
	case "flex":
		if !st.FlexSet {
			return "", false
		}
		return st.FlexLit, true
	case "gap":
		return strconv.Itoa(st.Gap), true
	case "padding":
		return box(st.Pad)
	case "margin":
		return box(st.Margin)
	case "align":
		return str(st.Align)
	case "justify":
		return str(st.Justify)
	case "overflow":
		return str(st.Overflow)
	case "content-align":
		return str(st.ContentAlign)
	case "display":
		return str(st.Display)
	case "color":
		return color(st.Color)
	case "background":
		return color(st.Background)
	case "border-color":
		return color(st.BorderColor)
	case "title-color":
		return color(st.TitleColor)
	case "border":
		return str(st.Border)
	case "bold":
		return boolean(st.Bold)
	case "dim":
		return boolean(st.Dim)
	case "italic":
		return boolean(st.Italic)
	case "underline":
		return boolean(st.Underline)
	case "reverse":
		return boolean(st.Reverse)
	case "wrap":
		return str(st.Wrap)
	case "visibility":
		return str(st.Visibility)
	case "bar":
		return str(st.Bar)
	}
	return "", false
}
