// Package ir holds the source IR shared by the parser, the stylesheet engine,
// the layout engine, and the dump: nodes, sizes, and diagnostics.
package ir

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
)

// Version is the source IR version emitted by `tuimark ir`.
const Version = "0.1"

// Kinds is the closed tag vocabulary (SPEC §6). Anything else is V001.
var Kinds = []string{
	"tui", "style", "keymap", "bind", "screen",
	"col", "row", "box", "scroll", "spacer",
	"text", "rule",
	"list", "item", "input", "button", "progress",
	"modal",
}

// SpikeKinds is the phase-0 vocabulary accepted under an <app> root.
var SpikeKinds = []string{"app", "col", "row", "box", "text"}

// SpikeAttrs is the phase-0 attribute whitelist (SPEC §6.7).
var SpikeAttrs = []string{"id", "class", "width", "height", "gap", "pad", "border"}

// IsKind reports whether tag is in the v1 catalog.
func IsKind(tag string) bool {
	for _, k := range Kinds {
		if k == tag {
			return true
		}
	}
	return false
}

// Node is one element of a parsed document.
type Node struct {
	// Tag is the source tag name; Kind is the IR kind. They differ only for
	// the spike alias <app>, whose kind is "col".
	Tag      string
	Kind     string
	ID       string
	Classes  []string
	Attrs    map[string]string // every attribute as written, in source form
	Order    []string          // attribute names in source order
	Bind     string
	Each     string
	If       string
	On       map[string]string // event name (without "on:") -> action
	Hints    []Prop            // presentational attributes (width, pad, ...) as CSS declarations
	Inline   []Prop            // style="" declarations
	Text     string
	Children []*Node
	Parent   *Node

	Line, Col int
	Path      string
}

// Attr returns an attribute value and whether it was present.
func (n *Node) Attr(name string) (string, bool) {
	if n == nil || n.Attrs == nil {
		return "", false
	}
	v, ok := n.Attrs[name]
	return v, ok
}

// Prop is one CSS declaration carried by a node (from an attribute or style="").
type Prop struct {
	Name, Value string
	Line, Col   int
}

// ScalarKind is the unit of a Size.
type ScalarKind int

const (
	// Unset means no size was specified on this axis.
	Unset ScalarKind = iota
	Cell
	Pct
	Fr
	Auto
)

// Scalar is a size value: N cells, N%, Nfr, or auto.
//
// Lit is the canonical decimal literal the value was parsed from (leading
// integer zeros and trailing fractional zeros dropped: "007.50" is "7.5"),
// and it is the exact value: layout does its §11.4 arithmetic on it, never
// on N. N is the nearest float64, kept for quick comparisons; a literal too
// large for a float64 has N = +Inf, and a positive literal too small for
// one still has N > 0 (math.SmallestNonzeroFloat64), so the sign of N is
// always the sign of the literal. A Scalar built in code (Lit == "") is
// exactly N.
type Scalar struct {
	Kind ScalarKind
	N    float64
	Lit  string
}

func (s Scalar) String() string {
	switch s.Kind {
	case Cell:
		return s.num(0)
	case Pct:
		return s.num(-1) + "%"
	case Fr:
		return s.num(-1) + "fr"
	case Auto:
		return "auto"
	}
	return ""
}

// num formats the number: the literal when there is one, else N (prec is
// strconv's: 0 for a whole number of cells, -1 for the shortest form).
// Unlike int(N), strconv.FormatFloat does not depend on the platform for
// values beyond the int range.
func (s Scalar) num(prec int) string {
	if s.Lit != "" {
		return s.Lit
	}
	return strconv.FormatFloat(s.N, 'f', prec, 64)
}

// Rat returns the exact value of s: its literal, or N when it has none
// (see FloatRat).
func (s Scalar) Rat() *big.Rat {
	if s.Lit != "" {
		if r, ok := new(big.Rat).SetString(s.Lit); ok {
			return r
		}
	}
	return FloatRat(s.N)
}

// IsWhole reports whether s is a whole number, decided on its literal
// (no fractional digits) rather than on N.
func (s Scalar) IsWhole() bool {
	if s.Lit != "" {
		return !strings.Contains(s.Lit, ".")
	}
	return s.N == math.Trunc(s.N)
}

// FloatRat returns the exact value of a number written in code (1fr, 80%,
// a flex weight with no literal): the shortest decimal that round-trips
// the float64, so 0.1 is 1/10, not 0.1000000000000000055…. NaN,
// infinities, and values <= 0 are 0.
func FloatRat(n float64) *big.Rat {
	if math.IsNaN(n) || math.IsInf(n, 0) || n <= 0 {
		return new(big.Rat)
	}
	r, ok := new(big.Rat).SetString(strconv.FormatFloat(n, 'f', -1, 64))
	if !ok {
		return new(big.Rat).SetFloat64(n)
	}
	return r
}

// ParseScalar parses `N | N% | Nfr | auto`. Anything else (px, em, ch, vw,
// negative numbers, fractions of a cell) is an error. Whether a cell count
// is whole and whether an fr weight is zero are decided on the literal, not
// on its float64: a 20-digit cell count is a whole number (layout clamps
// every size to 2^40 cells), 1.00000000000000000001 is not, and a tiny
// positive weight that underflows a float64 is still > 0.
func ParseScalar(s string) (Scalar, error) {
	s = strings.TrimSpace(s)
	if s == "auto" {
		return Scalar{Kind: Auto}, nil
	}
	num, kind := s, Cell
	switch {
	case strings.HasSuffix(s, "%"):
		num, kind = strings.TrimSuffix(s, "%"), Pct
	case strings.HasSuffix(s, "fr"):
		num, kind = strings.TrimSuffix(s, "fr"), Fr
	}
	if len(num) > MaxDecimalLen {
		return Scalar{}, fmt.Errorf("bad size %q: a number has at most %d characters", clip(s), MaxDecimalLen)
	}
	lit, f, ok := ParseDecimalLit(num)
	if !ok {
		return Scalar{}, fmt.Errorf("bad size %q (want N, N%%, Nfr, or auto)", clip(s))
	}
	sc := Scalar{Kind: kind, N: f, Lit: lit}
	if kind == Cell && !sc.IsWhole() {
		return Scalar{}, fmt.Errorf("bad size %q: cells are integers", clip(s))
	}
	if kind == Fr && lit == "0" {
		return Scalar{}, fmt.Errorf("bad size %q: fr weight must be > 0", clip(s))
	}
	return sc, nil
}

// clip shortens a value quoted in a diagnostic.
func clip(s string) string {
	const max = 40
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// MaxDecimalLen bounds the length of a number literal (sizes, flex,
// value). Exact arithmetic on a literal costs time quadratic in its length
// on every frame, so, like the nesting-depth and 2^40-cell bounds, this is
// a robustness backstop no real document comes near; a longer literal is
// V003.
const MaxDecimalLen = 1000

// ParseDecimal parses a plain non-negative decimal: digits with at most one
// inner '.', nothing else. It rejects what strconv.ParseFloat would also
// accept but markup must not: signs, exponents, hex floats, "inf", "NaN".
// The float64 is ParseDecimalLit's.
func ParseDecimal(s string) (float64, bool) {
	_, f, ok := ParseDecimalLit(s)
	return f, ok
}

// ParseDecimalLit is ParseDecimal that also returns the canonical literal,
// the exact value: leading integer zeros, trailing fractional zeros, and a
// then-empty fraction are dropped, so "007.50" is "7.5" and "2.0" is "2".
// The float64 is the nearest one, except that a literal too large for a
// float64 gives +Inf and a positive literal too small for one gives
// math.SmallestNonzeroFloat64: its sign is always the literal's. A literal
// longer than MaxDecimalLen is rejected.
func ParseDecimalLit(s string) (lit string, f float64, ok bool) {
	dot := strings.IndexByte(s, '.')
	if s == "" || dot == 0 || dot == len(s)-1 || len(s) > MaxDecimalLen {
		return "", 0, false
	}
	for i := 0; i < len(s); i++ {
		if (s[i] < '0' || s[i] > '9') && i != dot {
			return "", 0, false
		}
	}
	whole, frac := s, ""
	if dot > 0 {
		whole, frac = s[:dot], s[dot+1:]
	}
	whole = strings.TrimLeft(whole, "0")
	if whole == "" {
		whole = "0"
	}
	lit = whole
	if frac = strings.TrimRight(frac, "0"); frac != "" {
		lit += "." + frac
	}
	f, err := strconv.ParseFloat(lit, 64)
	if err != nil && !errors.Is(err, strconv.ErrRange) {
		return "", 0, false
	}
	if f == 0 && lit != "0" {
		f = math.SmallestNonzeroFloat64
	}
	return lit, f, true
}

// DecimalCmp compares the canonical literal lit (from ParseDecimalLit)
// with the whole number n exactly: -1, 0, or +1.
func DecimalCmp(lit string, n int64) int {
	r, ok := new(big.Rat).SetString(lit)
	if !ok {
		return 1
	}
	return r.Cmp(new(big.Rat).SetInt64(n))
}

// Severity levels.
const (
	Error   = "error"
	Warning = "warning"
)

// Diagnostic is the single diagnostic shape used by every pass (SPEC §14).
type Diagnostic struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Msg      string `json:"msg"`
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
	Col      int    `json:"col,omitempty"`
	Path     string `json:"path,omitempty"`
	ID       string `json:"id,omitempty"`
}

func (d Diagnostic) String() string {
	loc := d.File
	if d.Line > 0 {
		if loc == "" {
			loc = "<input>"
		}
		loc = fmt.Sprintf("%s:%d:%d", loc, d.Line, d.Col)
	}
	parts := []string{d.Severity, d.Code}
	if loc != "" {
		parts = append(parts, loc)
	}
	if d.Path != "" {
		parts = append(parts, d.Path)
	}
	return strings.Join(parts, " ") + " " + d.Msg
}

// Diags is an ordered diagnostic collection.
type Diags []Diagnostic

// HasErrors reports whether any diagnostic has error severity.
func (ds Diags) HasErrors() bool {
	for _, d := range ds {
		if d.Severity == Error {
			return true
		}
	}
	return false
}

// Sorted returns a copy ordered by file position, then code, for stable output.
func (ds Diags) Sorted() Diags {
	out := append(Diags(nil), ds...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Col != b.Col {
			return a.Col < b.Col
		}
		return a.Code < b.Code
	})
	return out
}

// At builds a diagnostic located at a node.
func At(n *Node, file, sev, code, format string, args ...any) Diagnostic {
	d := Diagnostic{Severity: sev, Code: code, Msg: fmt.Sprintf(format, args...), File: file}
	if n != nil {
		d.Line, d.Col, d.Path, d.ID = n.Line, n.Col, n.Path, n.ID
	}
	return d
}
