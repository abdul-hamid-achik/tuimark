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

// notANodeTags are current-language tags (ir.KindsCurrent) that are never
// a node: the composition tags of SPEC v0.4b §6.18, expanded away before
// the tree exists. In the current language a type selector naming one is
// V003 "is not a node and never matches"; in a pinned document it is the
// unknown type of 0.4.1, with no hint (§10.1, decision 94). Each tag
// enters with the phase that adds it to ir.KindsCurrent (SPEC v0.4b F1,
// F2); none is declared ahead of it.
var notANodeTags = map[string]bool{"include": true, "fragment": true, "template": true, "use": true, "slot": true}

// generatedTags are the types a selector may name that are not tags: the
// runtime generates the node (SPEC v0.4d §6.20.4), so a document never
// writes one and the catalog does not list it. The current language accepts
// them in a selector; a pinned document gets ir.VersionHintCurrent.
var generatedTags = map[string]bool{"span": true}

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
	return parseSelectorList(src, gate{v2: v2})
}

// ParseSelectorListLang is ParseSelectorListIn for a document of language
// l (SPEC v0.4b §5.1, §10.1).
func ParseSelectorListLang(src string, l ir.Lang) ([]Selector, error) {
	return parseSelectorList(src, gateOf(l))
}

func parseSelectorList(src string, g gate) ([]Selector, error) {
	var out []Selector
	for _, part := range strings.Split(src, ",") {
		sel, err := parseSelector(part, g)
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
	return parseSelector(src, gate{v2: v2})
}

// ParseSelectorLang is ParseSelectorIn for a document of language l (SPEC
// v0.4b §5.1, §10.1): a keymap when= is checked against it.
func ParseSelectorLang(src string, l ir.Lang) (Selector, error) {
	return parseSelector(src, gateOf(l))
}

func parseSelector(src string, g gate) (Selector, error) {
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
		comp, n, err := parseCompound(text[i:], text, g)
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

func parseCompound(s, whole string, g gate) (Compound, int, error) {
	v2 := g.v2
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
		if generatedTags[c.Tag] {
			// A type only a selector names (SPEC v0.4d §6.20.4, §10.1): the
			// runtime generates the node, and the current language lets a
			// rule name it. A pinned document gets the section 5.1 message;
			// the type matches no element of the tree, only a generated
			// span (host runs).
			if !g.v4 {
				return c, 0, fmt.Errorf("selector %q: type %q%s", whole, c.Tag, ir.VersionHintCurrent)
			}
		} else if ir.IsKindV2(c.Tag) && !v2 {
			return c, 0, fmt.Errorf("selector %q: type %q%s", whole, c.Tag, ir.VersionHint)
		}
		// A tag only the current language has (SPEC v0.4b §5.1, §10.1):
		// in a pinned document it carries the hint, unless it is a tag
		// that is never a node there either (the composition tags,
		// decision 94), which stays the unknown type of 0.4.1 below.
		if ir.IsKindCurrent(c.Tag) && !g.v4 && !notANodeTags[c.Tag] {
			return c, 0, fmt.Errorf("selector %q: type %q%s", whole, c.Tag, ir.VersionHintCurrent)
		}
		if !generatedTags[c.Tag] && !ir.IsKindIn(c.Tag, v2) && !(g.v4 && ir.IsKindCurrent(c.Tag)) {
			return c, 0, fmt.Errorf("selector %q: unknown type %q (not in the tag catalog)", whole, c.Tag)
		}
		if notANodeTags[c.Tag] {
			return c, 0, fmt.Errorf("selector %q: <%s> is not a node and never matches", whole, c.Tag)
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
			// An id is [A-Za-z_][A-Za-z0-9_-]*, as markup ids are
			// (SPEC §7): `#1` could never match a node.
			if !isIdentStart(id[0]) {
				return c, 0, fmt.Errorf("selector %q: bad id %q (must start with a letter or '_')", whole, id)
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
			// classname := [A-Za-z_][A-Za-z0-9_-]* (SPEC §7): `.9x` and
			// `.-x` could never match a class.
			if !isIdentStart(cl[0]) {
				return c, 0, fmt.Errorf("selector %q: bad class name %q (must start with a letter or '_')", whole, cl)
			}
			c.Classes = append(c.Classes, cl)
		case ':':
			i++
			if i < len(s) && s[i] == ':' {
				return c, 0, fmt.Errorf("selector %q: pseudo-elements are not supported", whole)
			}
			p := readIdent()
			if p == "" {
				return c, 0, fmt.Errorf("selector %q: empty pseudo-class after ':'", whole)
			}
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
