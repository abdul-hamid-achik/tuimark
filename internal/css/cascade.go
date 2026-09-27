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

// UASheet is the built-in stylesheet: SPEC §10.6 plus defaults for tags
// the SPEC leaves unstyled.
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
`

var uaSheet *Sheet

func init() {
	s, diags := ParseSheet(UASheet, "<ua>")
	if len(diags) > 0 {
		panic(fmt.Sprintf("built-in stylesheet: %v", diags))
	}
	uaSheet = s
}

// Env is the media/theme environment for one frame.
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
	theme := env.Theme
	if theme == "" {
		theme = "dark"
	}
	base, ok := Themes[theme]
	if !ok {
		base = Themes["dark"]
	}
	for k, v := range base {
		c.Tokens[k] = v
	}
	order := 0
	add := func(sh *Sheet, origin int) {
		for _, r := range sh.Rules {
			if !r.Media.Matches(env.Cols, env.Rows) {
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
					c.rules = append(c.rules, ruleDecl{decl: d, origin: origin, spec: sel.Specificity(), order: order, file: sh.File, sel: sel})
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
			}
		}
	}
	return st
}
