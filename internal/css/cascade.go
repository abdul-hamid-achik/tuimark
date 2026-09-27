package css

import (
	"fmt"
	"sort"
	"strings"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// Origins, lowest priority first. Presentational attributes (width="30%",
// pad="1", ...) sit between the built-in sheet and author CSS, like HTML
// presentational hints; style="" beats every selector (specificity 1000).
const (
	OriginUA = iota
	OriginHint
	OriginAuthor
	OriginInline
)

// UASheet is the built-in stylesheet, SPEC §10.6 as a whole: the v1
// defaults, then the 0.2b rules for the version="2" tags, which never
// match in a version="1" document (it cannot contain those tags). The
// built-in sheet is exempt from the version gate (SPEC §5.1).
const UASheet = `
screen { layout: column; width: 100%; height: 100%; }
col    { layout: column; width: 1fr; height: 1fr; }
row    { layout: row;    width: 1fr; height: 1fr; }
box    { layout: column; }
text   { width: auto; height: auto; }
scroll { layout: column; }
list   { layout: column; }
item   { layout: column; }
modal  { layout: column; border: single; }
spacer { flex: 1; }
input, button, progress, rule { width: auto; height: auto; }
list > item:selected { reverse: true; }
table  { layout: column; wrap: truncate; }
table > item:selected { reverse: true; }
tabs   { layout: column; }
tab    { layout: column; }
sparkline { width: auto; height: auto; }
hints  { layout: row; gap: 2; }
hints > row { gap: 1; }
`

// UAFile is the file name of the built-in sheet in diagnostics and in
// `tuimark inspect` (SPEC §15.7).
const UAFile = "<ua>"

var uaSheet *Sheet

// UA returns the parsed built-in sheet (read-only).
func UA() *Sheet { return uaSheet }

// EffectiveTheme maps a theme name to the built-in theme it selects:
// "light" is light; "dark", "", "auto" (outside Run, SPEC §10.4), and
// anything else are dark.
func EffectiveTheme(name string) string {
	if name == "light" {
		return "light"
	}
	return "dark"
}

func init() {
	s, diags := ParseSheetIn(UASheet, UAFile, true)
	if len(diags) > 0 {
		panic(fmt.Sprintf("built-in stylesheet: %v", diags))
	}
	uaSheet = s
}

// Env is the media/theme environment for one frame. Theme is the frame's
// effective theme, dark or light ("" is dark).
type Env struct {
	Cols, Rows int
	Theme      string
}

type ruleDecl struct {
	decl   Decl
	origin int
	spec   int
	order  int
	file   string
	sel    Selector
	media  *Media
}

// Cascade computes styles for one frame: media-filtered rules plus tokens.
type Cascade struct {
	rules  []ruleDecl // author + UA, flattened per selector
	Tokens map[string]string
	Diags  ir.Diags
	seen   map[string]bool
}

// NewCascade prepares the rule set for a terminal size and theme.
func NewCascade(sheets []*Sheet, env Env) *Cascade {
	c := &Cascade{Tokens: map[string]string{}, seen: map[string]bool{}}
	for k, v := range Themes[EffectiveTheme(env.Theme)] {
		c.Tokens[k] = v
	}
	order := 0
	// Token order (SPEC §10.4): the base set of the effective theme, then
	// every :root declaration of every rule whose @media matches, in
	// document order, a later declaration of a token replacing an earlier
	// one.
	add := func(sh *Sheet, origin int) {
		for _, r := range sh.Rules {
			if !r.Media.MatchesEnv(env) {
				continue
			}
			for _, sel := range r.Selectors {
				if sel.Root {
					for _, d := range r.Decls {
						c.Tokens[strings.TrimPrefix(d.Prop, "--")] = d.Value
					}
					continue
				}
				for _, d := range r.Decls {
					order++
					c.rules = append(c.rules, ruleDecl{decl: d, origin: origin, spec: sel.Specificity(), order: order, file: sh.File, sel: sel, media: r.Media})
				}
			}
		}
	}
	add(uaSheet, OriginUA)
	for _, sh := range sheets {
		add(sh, OriginAuthor)
	}
	for _, t := range RequiredTokens {
		if _, ok := c.Tokens[t]; !ok {
			c.Diags = append(c.Diags, ir.Diagnostic{Severity: ir.Error, Code: "V003", Msg: fmt.Sprintf("required theme token $%s is missing", t)})
		}
	}
	return c
}

// Compute returns the computed style of el. hints and inline hold the
// node's presentational attributes and style="" declarations.
func (c *Cascade) Compute(el Element, parent *Style, hints, inline []Decl) Style {
	return c.compute(el, parent, hints, inline, nil)
}

// DeclSource is one declaration that matched a node, as `tuimark inspect`
// reports it (SPEC §15.7): its value as written, its origin, and where it
// comes from.
type DeclSource struct {
	Value  string
	Origin int // OriginUA, OriginHint, OriginAuthor, OriginInline
	File   string
	Line   int
	Col    int
	// Selector is the rule's selector as written, the attribute as written
	// (`width="30%"`) for a presentational attribute, or `style=""`.
	Selector    string
	Specificity int
	Media       string // the @media condition as written; "" outside @media
}

// PropTrace explains one property of a computed style: the declaration
// that won (nil when none applied), the ones that matched and lost
// (highest priority first), and whether the value was inherited.
type PropTrace struct {
	Winner    *DeclSource
	Losers    []DeclSource
	Inherited bool
}

// Explain is Compute that also returns, per property, the declarations
// that decided it (`tuimark inspect`, SPEC §15.7). Declarations whose
// value does not resolve (an unknown token) apply nothing and are left
// out, as they are from the computed style.
func (c *Cascade) Explain(el Element, parent *Style, hints, inline []Decl) (Style, map[string]*PropTrace) {
	trace := map[string]*PropTrace{}
	st := c.compute(el, parent, hints, inline, trace)
	return st, trace
}

func (rd ruleDecl) source() DeclSource {
	ds := DeclSource{Value: rd.decl.Value, Origin: rd.origin, File: rd.file, Line: rd.decl.Line, Col: rd.decl.Col, Specificity: rd.spec}
	switch rd.origin {
	case OriginHint:
		ds.Selector = rd.decl.Attr
	case OriginInline:
		ds.Selector = `style=""`
	default:
		ds.Selector = rd.sel.Text
		if rd.media != nil {
			ds.Media = rd.media.Text
		}
	}
	return ds
}

func (c *Cascade) compute(el Element, parent *Style, hints, inline []Decl, trace map[string]*PropTrace) Style {
	var matched []ruleDecl
	for _, rd := range c.rules {
		if rd.sel.Matches(el) {
			matched = append(matched, rd)
		}
	}
	for i, d := range hints {
		matched = append(matched, ruleDecl{decl: d, origin: OriginHint, order: i, file: d.File})
	}
	for i, d := range inline {
		matched = append(matched, ruleDecl{decl: d, origin: OriginInline, spec: 1000, order: i, file: d.File})
	}
	sort.SliceStable(matched, func(i, j int) bool {
		a, b := matched[i], matched[j]
		if a.origin != b.origin {
			return a.origin < b.origin
		}
		if a.spec != b.spec {
			return a.spec < b.spec
		}
		return a.order < b.order
	})
	st := Initial()
	set := map[string]bool{}
	for _, rd := range matched {
		if err := apply(&st, rd.decl.Prop, rd.decl.Value, c.Tokens); err != nil {
			key := fmt.Sprintf("%s:%d:%d:%s", rd.file, rd.decl.Line, rd.decl.Col, err)
			if !c.seen[key] {
				c.seen[key] = true
				c.Diags = append(c.Diags, ir.Diagnostic{Severity: ir.Error, Code: "V003", Msg: fmt.Sprintf("%s: %v", rd.decl.Prop, err), File: rd.file, Line: rd.decl.Line, Col: rd.decl.Col})
			}
			continue
		}
		set[rd.decl.Prop] = true
		if trace != nil {
			pt := trace[rd.decl.Prop]
			if pt == nil {
				pt = &PropTrace{}
				trace[rd.decl.Prop] = pt
			}
			if pt.Winner != nil {
				pt.Losers = append([]DeclSource{*pt.Winner}, pt.Losers...)
			}
			src := rd.source()
			pt.Winner = &src
		}
		// matched is in ascending priority, so the last write wins.
		switch rd.decl.Prop {
		case "width":
			st.WidthUA = rd.origin == OriginUA
		case "height":
			st.HeightUA = rd.origin == OriginUA
		case "flex":
			st.FlexUA = rd.origin == OriginUA
		}
	}
	if parent != nil {
		for prop := range inherited {
			if !set[prop] {
				inherit(&st, parent, prop)
				if trace != nil {
					trace[prop] = &PropTrace{Inherited: true}
				}
			}
		}
	}
	return st
}

// IsInherited reports whether prop takes its parent's computed value when
// no declaration sets it.
func IsInherited(prop string) bool { return inherited[prop] }
