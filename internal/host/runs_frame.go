package host

import (
	"fmt"
	"slices"
	"strings"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/layout"
)

// runsState is what a builder keeps for the <text runs> of its pass.
type runsState struct {
	// boxes are the boxes of the texts with runs the pass inflated, for
	// styleRuns after the cascade.
	boxes []*layout.Box
	// bad holds the texts whose B013 this pass has reported: it is
	// reported once per element and frame (SPEC v0.4d §6.20.3).
	bad map[*ir.Node]bool
	// looks memoizes the look of a span by the style of its text and its
	// classes (styleRuns): boxes that share a computed style are the same
	// to every selector.
	looks map[spanKey]*css.Style
}

type spanKey struct {
	parent  *css.Style
	classes string
}

// inflateRuns is step 1 for a <text runs="path"> (SPEC v0.4d §6.20): it
// resolves the path in the text's scope and gives b the joined text of the
// runs and the runs themselves. The value is an array of runs, a string
// (one run), or null (no run); a missing path is B003 and the text is
// empty, and any other value or element is B013, once per element, with
// the bad run left out (or painted without classes when only its class is
// bad).
func (fb *builder) inflateRuns(n *ir.Node, b *layout.Box, sc *scope) {
	v, found := sc.resolve(fb.a.store, n.Runs)
	if !found {
		if ok, via := fb.pathMiss(n, "runs", 0, n.Runs, sc, missValue); ok {
			fb.reportPath(n, via, n.Runs, fb.bindSev(), "B003", "runs path %q is missing", shownPath(n.Runs, via))
		}
		return
	}
	via := n.ViaOf("runs", 0)
	bad := func(format string, args ...any) {
		if fb.runs.bad[n] {
			return
		}
		if fb.runs.bad == nil {
			fb.runs.bad = map[*ir.Node]bool{}
		}
		fb.runs.bad[n] = true
		args = append([]any{shownPath(n.Runs, via)}, args...)
		fb.reportPath(n, via, n.Runs, ir.Warning, "B013", format, args...)
	}
	var rb runBuilder
	switch x := v.(type) {
	case nil:
	case string:
		rb.add(0, x, nil)
	case []any:
		for i, el := range x {
			switch r := el.(type) {
			case string:
				rb.add(i, r, nil)
			case map[string]any:
				text, ok := r["text"].(string)
				if !ok {
					bad("runs=%q: run %d has no string \"text\"", i)
					continue
				}
				var classes []string
				if cv, has := r["class"]; has && cv != nil {
					cs, isStr := cv.(string)
					if isStr {
						classes, isStr = runClasses(cs)
					}
					if !isStr {
						bad("runs=%q: run %d has a \"class\" that is not a list of class names", i)
						classes = nil
					}
				}
				rb.add(i, text, classes)
			default:
				bad("runs=%q: run %d is %s, not a string or an object", i, runTypeName(el))
			}
		}
	default:
		bad("runs=%q holds %s, not an array of runs, a string, or null", runTypeName(v))
	}
	b.Text = rb.text.String()
	if len(rb.spans) > 0 {
		b.Runs = &layout.TextRuns{Spans: rb.spans}
		fb.runs.boxes = append(fb.runs.boxes, b)
	}
}

// runBuilder joins the texts of runs.
type runBuilder struct {
	text  strings.Builder
	spans []layout.RunSpan
}

// add appends the run index i with the given text (normalized as run
// text, SPEC v0.4d §6.20.2) and classes. A run whose text is empty has no
// cells and is left out.
func (rb *runBuilder) add(i int, text string, classes []string) {
	text = NormalizeRunText(text)
	if text == "" {
		return
	}
	rb.text.WriteString(text)
	rb.spans = append(rb.spans, layout.RunSpan{End: rb.text.Len(), Index: i, Classes: classes})
}

// runClasses splits a run's class value into class names, each once, in
// order (SPEC v0.4d §6.20.2): ok is false when one is not a class name
// (§7 classname, [A-Za-z_][A-Za-z0-9_-]*).
func runClasses(s string) (names []string, ok bool) {
	for _, c := range strings.Fields(s) {
		if !isClassName(c) {
			return nil, false
		}
		if !slices.Contains(names, c) {
			names = append(names, c)
		}
	}
	return names, true
}

func isClassName(s string) bool {
	if s == "" || (s[0] >= '0' && s[0] <= '9') || s[0] == '-' {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c == '-' || c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}

// runTypeName is the TYPE of a B013 message (SPEC v0.4d §6.20.3).
func runTypeName(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case bool:
		return "a boolean"
	case float64:
		return "a number"
	case string:
		return "a string"
	case []any:
		return "an array"
	case map[string]any:
		return "an object"
	}
	return fmt.Sprintf("%T", v)
}

// spanElement is the generated span of a run as the cascade sees it
// (SPEC v0.4d §6.20.4): type span, no id, the run's classes, no state, and
// the text as its parent, so `text > span`, `#reply > span.code`, and
// `#t:focus > span` match, while no pseudo-class matches the span itself.
type spanElement struct {
	parent  *layout.Box
	classes []string
}

func (s *spanElement) MatchTag() string       { return "span" }
func (s *spanElement) MatchID() string        { return "" }
func (s *spanElement) HasClass(c string) bool { return slices.Contains(s.classes, c) }
func (s *spanElement) HasPseudo(string) bool  { return false }
func (s *spanElement) ParentElement() css.Element {
	return s.parent
}

// spanProps are the look properties a span styles (SPEC v0.4d §6.20.4);
// every other property is accepted on a span and ignored.
var spanProps = [...]string{"color", "background", "bold", "dim", "italic", "underline", "reverse"}

// spanLook computes the style a run's cells have: the text's own style
// with each of the seven look properties a rule sets on the span replaced
// (SPEC v0.4d §6.20.4). background is not inherited, so a span that no rule
// gives a background has its text's, as it has its text's value of every
// other property no rule sets on it.
func spanLook(c *css.Cascade, text *layout.Box, classes []string) *css.Style {
	st, trace := c.Explain(&spanElement{parent: text, classes: classes}, text.Style, nil, nil)
	out := *text.Style
	for _, p := range spanProps {
		if t := trace[p]; t != nil && t.Winner != nil {
			css.CopyLook(&out, &st, p)
		}
	}
	return &out
}

// runInfo is the run member of the inspection of b for --at x,y (SPEC
// v0.4d §6.20.6): the run that painted that cell, nil when b has no runs
// or the cell is not one a run painted (after the text, the ellipsis, an
// empty row). It asks the layout the way the painter does
// (layout.Box.RunAt), then explains the span with the cascade again, as
// styleInfo explains a node. The caller holds a.mu.
func (a *App) runInfo(f *Frame, b *layout.Box, x, y int) *RunInfo {
	if b.Runs == nil || b.Fixed || !b.Laid {
		return nil
	}
	i := b.RunAt(b.Style.Wrap, x, y)
	if i < 0 {
		return nil
	}
	sp := b.Runs.Spans[i]
	start := 0
	if i > 0 {
		start = b.Runs.Spans[i-1].End
	}
	out := &RunInfo{Index: sp.Index, Text: b.Text[start:sp.End], Classes: append([]string{}, sp.Classes...), Style: []StyleInfo{}}
	casc := css.NewCascadeLang(a.sheets, css.Env{Cols: f.Cols, Rows: f.TermRows, Theme: f.Theme}, f.Lang)
	st, trace := casc.Explain(&spanElement{parent: b, classes: sp.Classes}, b.Style, nil, nil)
	for _, prop := range css.PropertyOrder {
		if !slices.Contains(spanProps[:], prop) {
			continue
		}
		si := StyleInfo{Prop: prop, Origin: "inherited", Overridden: []LostDecl{}}
		from := *b.Style
		if pt := trace[prop]; pt != nil && pt.Winner != nil {
			from = st
			si.Origin = originNames[pt.Winner.Origin]
			r := ruleOf(*pt.Winner)
			si.Rule = &r
			for _, l := range pt.Losers {
				si.Overridden = append(si.Overridden, LostDecl{Value: l.Value, Origin: originNames[l.Origin], Rule: ruleOf(l)})
			}
		}
		if v, ok := css.FormatProp(from, prop); ok {
			si.Value = &v
		}
		out.Style = append(out.Style, si)
	}
	return out
}

// classKey names a list of classes, without building a string for the
// common cases of none and one.
func classKey(classes []string) string {
	switch len(classes) {
	case 0:
		return ""
	case 1:
		return classes[0]
	}
	return strings.Join(classes, " ")
}

// styleRuns gives the runs of every text with runs the look of their span
// (SPEC v0.4d §6.20.4), after the cascade of the frame: a text's runs with
// the same classes share one computation, and so do the texts that share a
// computed style.
func (fb *builder) styleRuns(c *css.Cascade) {
	for _, b := range fb.runs.boxes {
		if b.Style == nil {
			continue
		}
		for i := range b.Runs.Spans {
			sp := &b.Runs.Spans[i]
			key := spanKey{b.Style, classKey(sp.Classes)}
			st, ok := fb.runs.looks[key]
			if !ok {
				st = spanLook(c, b, sp.Classes)
				if fb.runs.looks == nil {
					fb.runs.looks = map[spanKey]*css.Style{}
				}
				fb.runs.looks[key] = st
			}
			sp.Style = st
		}
	}
}
