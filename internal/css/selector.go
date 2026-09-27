package css

import (
	"fmt"
	"strings"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// Compound is one simple-selector sequence: type? #id? .class* :pseudo*.
type Compound struct {
	Tag     string
	ID      string
	Classes []string
	Pseudos []string
}

// Selector is a chain of compounds joined by the child combinator. The last
// compound is the subject.
type Selector struct {
	Parts []Compound
	Root  bool // the :root pseudo-selector (token definitions only)
	Text  string
}

// Pseudos allowed in v1.
var allowedPseudos = map[string]bool{"focus": true, "selected": true, "disabled": true, "empty": true}

// v2Pseudos are the pseudo-classes only version="2" documents accept, in
// stylesheets and in keymap when= selectors (SPEC §5.1, §10.1).
var v2Pseudos = map[string]bool{"checked": true, "focus-within": true}

// nonRenderedTags are catalog tags (SPEC §6.1) that never become a laid-out
// Box: host.inflate builds no node for them, and the cascade never matches
// or inherits through them (the screen, not <tui>, is the root of the
// matched/inherited tree). A type selector naming one, such as `tui { ... }`
// or `tui > screen { ... }`, would otherwise parse without error and then
// silently never match anything.
var nonRenderedTags = map[string]bool{"tui": true, "style": true, "keymap": true, "bind": true}

// Specificity: type=1, class=10, id=100, pseudo=+10 (SPEC §10.1).
func (s Selector) Specificity() int {
	n := 0
	for _, c := range s.Parts {
		if c.Tag != "" {
			n++
		}
		if c.ID != "" {
			n += 100
		}
		n += 10 * len(c.Classes)
		n += 10 * len(c.Pseudos)
	}
	return n
}

// Element is what selectors match against.
type Element interface {
	MatchTag() string
	MatchID() string
	HasClass(string) bool
	HasPseudo(string) bool
	ParentElement() Element
}

// Matches reports whether sel matches el.
func (s Selector) Matches(el Element) bool {
	if s.Root || len(s.Parts) == 0 {
		return false
	}
	cur := el
	for i := len(s.Parts) - 1; i >= 0; i-- {
		if cur == nil || !s.Parts[i].matches(cur) {
			return false
		}
		if i > 0 {
			cur = cur.ParentElement()
		}
	}
	return true
}

func (c Compound) matches(el Element) bool {
	if c.Tag != "" && c.Tag != el.MatchTag() {
		return false
	}
	if c.ID != "" && c.ID != el.MatchID() {
		return false
	}
	for _, cl := range c.Classes {
		if !el.HasClass(cl) {
			return false
		}
	}
	for _, p := range c.Pseudos {
		if !el.HasPseudo(p) {
			return false
		}
	}
	return true
}

// ParseSelectorList parses `a, b > c` into selectors (version="1").
func ParseSelectorList(src string) ([]Selector, error) { return ParseSelectorListIn(src, false) }

// ParseSelectorListIn is ParseSelectorList for a document of the given
// version: a version="2" document also accepts the six new type
// selectors and :checked and :focus-within (SPEC §5.1).
func ParseSelectorListIn(src string, v2 bool) ([]Selector, error) {
	var out []Selector
	for _, part := range strings.Split(src, ",") {
		sel, err := ParseSelectorIn(part, v2)
		if err != nil {
			return nil, err
		}
		out = append(out, sel)
	}
	return out, nil
}

// ParseSelector parses one selector (no commas) of a version="1" document.
func ParseSelector(src string) (Selector, error) { return ParseSelectorIn(src, false) }

// ParseSelectorIn parses one selector (no commas) for a document of the
// given version.
func ParseSelectorIn(src string, v2 bool) (Selector, error) {
	text := strings.TrimSpace(src)
	if text == "" {
		return Selector{}, fmt.Errorf("empty selector")
	}
	if text == ":root" {
		return Selector{Root: true, Text: text}, nil
	}
	sel := Selector{Text: text}
	i := 0
	expectCompound := true
	sawSpace := false
	for i < len(text) {
		c := text[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			sawSpace = true
			i++
			continue
		case c == '>':
			if expectCompound {
				return Selector{}, fmt.Errorf("selector %q: '>' needs a selector on both sides", text)
			}
			expectCompound = true
			sawSpace = false
			i++
			continue
		case c == '*':
			return Selector{}, fmt.Errorf("selector %q: the universal selector * is not supported", text)
		case c == '&':
			return Selector{}, fmt.Errorf("selector %q: nesting (&) is not supported", text)
		case c == '[':
			return Selector{}, fmt.Errorf("selector %q: attribute selectors are not supported", text)
		case c == '+' || c == '~':
			return Selector{}, fmt.Errorf("selector %q: sibling combinators are not supported", text)
		}
		if !expectCompound {
			if sawSpace {
				return Selector{}, fmt.Errorf("selector %q: the descendant combinator (space) is not supported; use '>'", text)
			}
		}
		comp, n, err := parseCompound(text[i:], text, v2)
		if err != nil {
			return Selector{}, err
		}
		sel.Parts = append(sel.Parts, comp)
		i += n
		expectCompound = false
		sawSpace = false
	}
	if expectCompound {
		return Selector{}, fmt.Errorf("selector %q ends with a combinator", text)
	}
	return sel, nil
}

func parseCompound(s, whole string, v2 bool) (Compound, int, error) {
	var c Compound
	i := 0
	readIdent := func() string {
		start := i
		for i < len(s) && (s[i] == '-' || s[i] == '_' || (s[i] >= 'a' && s[i] <= 'z') || (s[i] >= 'A' && s[i] <= 'Z') || (s[i] >= '0' && s[i] <= '9')) {
			i++
		}
		return s[start:i]
	}
	if i < len(s) && isIdentStart(s[i]) {
		c.Tag = readIdent()
		if ir.IsKindV2(c.Tag) && !v2 {
			return c, 0, fmt.Errorf("selector %q: type %q%s", whole, c.Tag, ir.VersionHint)
		}
		if !ir.IsKindIn(c.Tag, v2) {
			return c, 0, fmt.Errorf("selector %q: unknown type %q (not in the tag catalog)", whole, c.Tag)
		}
		if nonRenderedTags[c.Tag] {
			return c, 0, fmt.Errorf("selector %q: <%s> is not rendered and never matches; style screen (or its descendants) instead", whole, c.Tag)
		}
	}
	for i < len(s) {
		switch s[i] {
		case '#':
			i++
			id := readIdent()
			if id == "" {
				return c, 0, fmt.Errorf("selector %q: '#' needs an id", whole)
			}
			if c.ID != "" {
				return c, 0, fmt.Errorf("selector %q: more than one #id in a compound", whole)
			}
			c.ID = id
		case '.':
			i++
			cl := readIdent()
			if cl == "" {
				return c, 0, fmt.Errorf("selector %q: '.' needs a class name", whole)
			}
			c.Classes = append(c.Classes, cl)
		case ':':
			i++
			if i < len(s) && s[i] == ':' {
				return c, 0, fmt.Errorf("selector %q: pseudo-elements are not supported", whole)
			}
			p := readIdent()
			if i < len(s) && s[i] == '(' || strings.HasPrefix(p, "nth-") {
				return c, 0, fmt.Errorf("selector %q: :%s is not supported", whole, p)
			}
			if p == "root" {
				return c, 0, fmt.Errorf("selector %q: :root must stand alone", whole)
			}
			if v2Pseudos[p] && !v2 {
				return c, 0, fmt.Errorf("selector %q: pseudo-class :%s%s", whole, p, ir.VersionHint)
			}
			if !allowedPseudos[p] && !v2Pseudos[p] {
				if v2 {
					return c, 0, fmt.Errorf("selector %q: pseudo-class :%s is not supported (use :focus :selected :disabled :empty :checked :focus-within)", whole, p)
				}
				return c, 0, fmt.Errorf("selector %q: pseudo-class :%s is not supported (use :focus :selected :disabled :empty)", whole, p)
			}
			c.Pseudos = append(c.Pseudos, p)
		default:
			if i == 0 {
				return c, 0, fmt.Errorf("selector %q: unexpected %q", whole, string(s[i]))
			}
			return c, i, nil
		}
	}
	if i == 0 {
		return c, 0, fmt.Errorf("selector %q: empty compound", whole)
	}
	return c, i, nil
}

func isIdentStart(b byte) bool {
	return b == '_' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}
