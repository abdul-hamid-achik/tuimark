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
	"unicode/utf8"
)

// Version is the source IR version `tuimark ir` emits for version="1"
// documents and the spike (SPEC §13.1).
const Version = "0.1"

// VersionV2 is the source IR version `tuimark ir` emits for version="2"
// documents (SPEC §13.1, §23.2).
const VersionV2 = "0.2"

// VersionV3 is the source IR version `tuimark ir` emits for version="3"
// documents (SPEC v0.3 §5.1, §13.1, §23.4). A version="3" document adds no
// tag, attribute, or keymap member over version="2" in 0.3a, so IR 0.3 has
// exactly the shape of IR 0.2 (schema/ir.v0.3.json).
const VersionV3 = "0.3"

// VersionHint ends the message of every V001, V002, and V003 that a
// version="2" item gets in a version="1" document (SPEC §5.1).
const VersionHint = ` (requires version="2")`

// VersionHintV3 ends the message of every V002 and V003 that a
// version="3" item gets in a version="1" or version="2" document (SPEC
// v0.3b §5.1).
const VersionHintV3 = ` (requires version="3")`

// Kinds is the closed tag vocabulary (SPEC §6). Anything else is V001.
var Kinds = []string{
	"tui", "style", "keymap", "bind", "screen",
	"col", "row", "box", "scroll", "spacer",
	"text", "rule",
	"list", "item", "input", "button", "progress",
	"modal",
}

// KindsV2 are the tags only a <tui version="2"> document accepts (SPEC
// §5.1, §6): 23 kinds in all with Kinds.
var KindsV2 = []string{"table", "column", "tabs", "tab", "sparkline", "hints"}

// IsKindV2 reports whether tag is one of the version="2" tags.
func IsKindV2(tag string) bool {
	for _, k := range KindsV2 {
		if k == tag {
			return true
		}
	}
	return false
}

// IsKindIn reports whether tag is in the catalog of a document of the
// given version: Kinds, plus KindsV2 when v2.
func IsKindIn(tag string, v2 bool) bool {
	return IsKind(tag) || (v2 && IsKindV2(tag))
}

// BuiltinActionsV2 are the hyphenated built-in actions of SPEC §8.4,
// accepted in keymap rows of version="2" documents only (ADR 0006).
var BuiltinActionsV2 = []string{
	"move-next", "move-prev", "move-first", "move-last", "move-page-down", "move-page-up",
	"check-toggle", "check-all", "check-none", "switch-to",
}

// IsBuiltinActionV2 reports whether name is a hyphenated built-in action.
func IsBuiltinActionV2(name string) bool {
	for _, a := range BuiltinActionsV2 {
		if a == name {
			return true
		}
	}
	return false
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
	Tag     string
	Kind    string
	ID      string
	Classes []string
	Attrs   map[string]string // every attribute as written, in source form
	Order   []string          // attribute names in source order
	Bind    string
	Each    string
	If      string
	On      map[string]string // event name (without "on:") -> action
	// ClassGuards are the class:NAME attributes of a version="2" element,
	// in attribute order (SPEC §6.13).
	ClassGuards []ClassGuard
	Hints       []Prop // presentational attributes (width, pad, ...) as CSS declarations
	Inline      []Prop // style="" declarations
	Text        string
	Children    []*Node
	Parent      *Node
	// Source is the children of this element as they were written, when
	// they include composition (SPEC v0.4b §6.18): the elements of
	// Children plus the <include> nodes (and, later, <template> and
	// <use> nodes) between them, each include holding the nodes it
	// inserted in its own Children. nil when the element's children are
	// exactly Children. Only `tuimark ir` reads it: layout, the cascade,
	// the host, and the dump see the expanded Children.
	Source []*Node

	// Spinner is set by the parser on a <progress> of the current
	// language that is a spinner: indeterminate="true", with neither bind
	// nor value (SPEC v0.4b §6.4.1). In every other case, a pinned
	// document's indeterminate included, the progress is a bar.
	Spinner bool

	// Single is set by the parser on a <list> or <table> of the current
	// language that writes single="true" (SPEC v0.4b §6.14.1): its checked
	// path then holds one row key, or null, instead of an array. In every
	// other case, a pinned document's single included, the widget is a
	// multi-select as in 0.4.1.
	Single bool

	Line, Col int
	Path      string
	// Org says where an element of an expanded document was written and
	// which <include> placed it (SPEC v0.4b §6.18.6). nil for an element
	// written in the document itself and placed by nothing, which is every
	// element of a document without composition.
	Org *Origin

	// CopiedBy is set on a top-level element of a <use>'s copy (SPEC
	// v0.4b §6.18.3): the uses whose copy it is a top-level element of,
	// innermost first. Their static class is already in Classes; their if
	// and hidden decide, each frame, whether the element is in it. nil
	// for every other element.
	CopiedBy []*Node
	// Written is what an element of a copy (or a <use> in one) wrote,
	// before its paths went through the with aliases in scope and its #id
	// references were resolved (SPEC v0.4b §6.18.4, §6.18.5, decision
	// 18): `tuimark ir` shows it, and the messages that name a path as
	// written read it. nil outside copies.
	Written *Written
	// With is a <use> node's with clauses (SPEC v0.4b §6.18.5), each path
	// already resolved where the use stands (through the aliases in scope
	// there); nil for every other node and for a use whose with is V011.
	With []Each
	// Vias says how the paths of an element of a copy (or of a <use> in
	// one) that went through a with alias did (SPEC v0.4b §6.18.5), by
	// the path as it stands after the substitution: what the runtime
	// resolves, and what its binding diagnostics name. nil for every
	// node with no such path, which is every node outside copies.
	Vias map[string]*Via
}

// Via is a path of an element of a copy that went through a with alias
// of Use (SPEC v0.4b §6.18.5): Written is the path as written in the
// template, and Base the path the alias stands for, resolved where Use
// stands (the path of its clause in Use.With). The path itself is Base
// followed by Written's other segments.
type Via struct {
	Written, Base string
	Use           *Node
}

// ViaOf is how the path p of n (as it stands after the substitution)
// went through a with alias, or nil when it went through none.
func (n *Node) ViaOf(p string) *Via {
	if n == nil || n.Vias == nil {
		return nil
	}
	return n.Vias[p]
}

// Suffix is what a B001, B002, or B003 about the path p, which went
// through v, ends with in place of the placement suffix (SPEC v0.4b
// §6.18.5 "Messages"): ` (P via <use id="U">)`, with P the path p
// stands for (ShownPath) and the use named as a placement suffix names
// it (`<use template="T">` without id).
func (v *Via) Suffix(p string) string {
	return " (" + ShownPath(p) + " via " + v.Use.describePlacer() + ")"
}

// ShownPath is a path a diagnostic quotes away from where it is written,
// the P of a path through a with alias (Via.Suffix): every renamed alias
// shown as written (Unmark) and cut as ShownID cuts an id, in time
// bounded by MaxShownID however long p is. A path holds at most one
// renamed alias, at its head: one AliasMark byte and the digits of an
// int.
func ShownPath(p string) string {
	const room = MaxShownID + 22
	if len(p) > room {
		// Unmarked, the first room bytes keep more than MaxShownID bytes
		// (a mark and its digits are at most 21), so ShownID cuts them
		// where it would cut the whole path.
		p = p[:room]
	}
	return ShownID(Unmark(p))
}

// Written is an element of a copy as written (Node.Written).
type Written struct {
	Attrs                map[string]string
	Bind, Each, If, Text string
	Classes              []string
}

// Origin is where an element of an expanded document was written (SPEC
// v0.4b §6.18.6): the file's name as diagnostics name it, and the
// innermost <include> element that placed it. Every element placed by the
// same include in the same file shares one Origin.
type Origin struct {
	// File is the display name of the file the element is written in.
	File string
	// Placed is the node of the innermost <include> that placed the
	// element, whose own Org says where that include is written. nil
	// when nothing placed the element (a document element that stands
	// in its own file, or a node of a fragment checked on its own).
	Placed *Node
}

// FileName is the display name of the file n is written in: its origin's,
// else def (the document's own name).
func (n *Node) FileName(def string) string {
	if n == nil {
		return def
	}
	return n.Org.FileOr(def)
}

// placer is the include that placed the elements of o, nil for no origin
// or an origin nothing placed.
func (o *Origin) placer() *Node {
	if o == nil {
		return nil
	}
	return o.Placed
}

// Placer is the innermost include that placed the node, nil if none; the
// next one out is the Placer of that include.
func (n *Node) Placer() *Node {
	if n == nil {
		return nil
	}
	return n.Org.placer()
}

// FileOr is the file name o says, or def for no origin (an element of the
// document's own text).
func (o *Origin) FileOr(def string) string {
	if o != nil {
		return o.File
	}
	return def
}

// PlacedSuffix is the suffix of a diagnostic about an element placed by an
// include: `, placed by <include> at FILE:LINE:COL` (SPEC v0.4b §6.18.6),
// naming the line the author wrote. It is "" for an element nothing
// placed.
func PlacedSuffix(n *Node) string {
	if n == nil {
		return ""
	}
	return n.Org.PlacedSuffix()
}

// PlacedSuffix is the placement suffix of an element written where o says
// (see the function of the same name); "" for no origin or none placed.
func (o *Origin) PlacedSuffix() string {
	if o == nil || o.Placed == nil {
		return ""
	}
	return o.PlacedBy() + " at " + o.Placed.Loc(true)
}

// PlacedBy is the part of o's placement suffix that does not depend on
// where the files are or what they are called: `, placed by <use …>`,
// without " at FILE:LINE:COL". It is what the expansion budget charges an
// element a copy places (the F2 final check): the same document under a
// longer file name, or with its uses further right, is charged the same.
func (o *Origin) PlacedBy() string {
	if o == nil || o.Placed == nil {
		return ""
	}
	return ", placed by " + o.Placed.describePlacer()
}

// InCopy reports whether n is an element of a copy: a <use> is among the
// elements that placed it (its Placer chain). A diagnostic about such an
// element quotes every id written outside the element through ShownID
// (ShownIDAt), since one written element there is reported once per copy;
// an element written once, and every element of a pinned document, keeps
// the text 0.4.1 gives.
func (n *Node) InCopy() bool {
	for p := n.Placer(); p != nil; p = p.Placer() {
		if p.Kind == "use" {
			return true
		}
	}
	return false
}

// ShownIDAt is id as a diagnostic about the node at quotes it: ShownID(id)
// for an element of a copy (InCopy) and for a <use> (a composition
// diagnostic, which no pinned document has), id whole otherwise, as 0.4.1
// quotes it.
func ShownIDAt(at *Node, id string) string {
	if at != nil && (at.Kind == "use" || at.InCopy()) {
		return ShownID(id)
	}
	return id
}

// describePlacer names an <include> or a <use> the way a placement suffix
// does, its id or template id as ShownID shows it.
func (n *Node) describePlacer() string {
	if n.ID != "" {
		return fmt.Sprintf("<%s id=%q>", n.Tag, ShownID(n.ID))
	}
	if t, ok := n.Attrs["template"]; ok && n.Tag == "use" {
		return fmt.Sprintf("<use template=%q>", ShownID(t))
	}
	return "<" + n.Tag + ">"
}

// MaxShownID is the longest id a composition diagnostic shows whole when
// it quotes an id (or an include's file) written somewhere other than its
// own site (the owner's decisions in the F2 re-review): a longer one is
// shown as its first MaxShownID bytes, cut at a rune boundary, then "…"
// (ShownID). It keeps what many diagnostics print about one long id
// linear in the document: the placement suffix, V019, V012, V021, V006,
// V011, and the reasons of an id that names no node.
const MaxShownID = 64

// ShownID is id as a composition diagnostic shows it (MaxShownID).
func ShownID(id string) string {
	if len(id) <= MaxShownID {
		return id
	}
	cut := MaxShownID
	for cut > 0 && !utf8.RuneStart(id[cut]) {
		cut--
	}
	return id[:cut] + "…"
}

// Loc is the node's position in the file it is written in as LINE:COL, or
// FILE:LINE:COL with file (an empty file is written <input>, as a
// diagnostic's location is).
func (n *Node) Loc(file bool) string {
	if !file {
		return fmt.Sprintf("%d:%d", n.Line, n.Col)
	}
	f := n.FileName("")
	if f == "" {
		f = "<input>"
	}
	return fmt.Sprintf("%s:%d:%d", f, n.Line, n.Col)
}

// ClassGuard is one class:NAME="guard" attribute: the class NAME is on the
// element while the guard (path or !path) is truthy (SPEC §6.13).
type ClassGuard struct {
	Name  string
	Guard string
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
	// Attr is the presentational attribute as written (`pad="1"`), for a
	// declaration that comes from one; "" for style="" declarations.
	Attr string
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

// At builds a diagnostic located at a node. file is the document's name;
// an element written in a fragment is reported in its own file, and one an
// include placed names the include (SPEC v0.4b §6.18.6).
func At(n *Node, file, sev, code, format string, args ...any) Diagnostic {
	d := Diagnostic{Severity: sev, Code: code, Msg: fmt.Sprintf(format, UnmarkArgs(args)...), File: file}
	if n != nil {
		d.Line, d.Col, d.Path, d.ID = n.Line, n.Col, n.Path, n.ID
		d.File = n.FileName(file)
		d.Msg += PlacedSuffix(n)
	}
	return d
}

// AtSuffix is At with suffix in place of the placement suffix: a message
// that already names its placement (SPEC v0.4b §6.18.6), such as the
// ` (P via <use id="U">)` of a path through a with alias, or one about a
// <use> itself, which carries none ("" adds nothing).
func AtSuffix(n *Node, file, sev, code, suffix, format string, args ...any) Diagnostic {
	d := Diagnostic{Severity: sev, Code: code, Msg: fmt.Sprintf(format, UnmarkArgs(args)...) + suffix, File: file}
	if n != nil {
		d.Line, d.Col, d.Path, d.ID = n.Line, n.Col, n.Path, n.ID
		d.File = n.FileName(file)
	}
	return d
}
