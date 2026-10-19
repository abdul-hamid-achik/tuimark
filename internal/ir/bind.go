package ir

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Binding grammar (SPEC §7):
//
//	ident  := [A-Za-z_][A-Za-z0-9_]*
//	path   := ident ("." ident)*
//	interp := "{" path "}"
//	each   := path " as " ident
//	if     := path | "!" path

// IsIdent reports whether s matches the ident production.
func IsIdent(s string) bool {
	return isIdent(s, false)
}

// isIdent is IsIdent; with field set it also accepts an each alias the
// parser renamed in a template's copy (MarkAlias).
func isIdent(s string, field bool) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '_', c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9' && i > 0:
		case field && c == AliasMark && i > 0:
			return allDigits(s[i+1:])
		default:
			return false
		}
	}
	return true
}

// AliasMark starts the suffix of an each alias that the parser renamed in a
// template's copy, so that a path a with alias stands for is never captured
// by an each alias of the copy with the same name (SPEC v0.4b §6.18.5). It
// is a byte that valid UTF-8 never holds: a document is valid UTF-8 (V005
// otherwise), and so is every string JSON data brings, so a renamed alias
// never meets a written name, a store root included, and Unmark never
// changes what a document or its data wrote. The grammar checks a document
// meets (IsIdent, IsPath, ParseEach, ParseGuard, ParseInterp) never accept
// it; only their Field variants do, for the node fields the parser has
// already checked as written.
const AliasMark = 0xFF

// MarkAlias is the name an each alias written as name takes when the parser
// renames it, the n-th renamed in the document.
func MarkAlias(name string, n int) string {
	return name + string([]byte{AliasMark}) + strconv.Itoa(n)
}

// Unmark is s with every renamed alias shown as written (MarkAlias): what a
// message, an event's keys, or a tool prints, so no renamed name ever
// surfaces. A string without AliasMark, which is every string a document
// or its data wrote, is returned as it is.
func Unmark(s string) string {
	i := strings.IndexByte(s, AliasMark)
	if i < 0 {
		return s
	}
	var b strings.Builder
	for i >= 0 {
		b.WriteString(s[:i])
		j := i + 1
		for j < len(s) && s[j] >= '0' && s[j] <= '9' {
			j++
		}
		s = s[j:]
		i = strings.IndexByte(s, AliasMark)
	}
	b.WriteString(s)
	return b.String()
}

// UnmarkArgs is args with every string unmarked (Unmark), for a message
// formatted from paths.
func UnmarkArgs(args []any) []any {
	cloned := false
	for i, a := range args {
		if s, ok := a.(string); ok && strings.IndexByte(s, AliasMark) >= 0 {
			if !cloned {
				args, cloned = slices.Clone(args), true
			}
			args[i] = Unmark(s)
		}
	}
	return args
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// IsPath reports whether s matches the path production.
func IsPath(s string) bool {
	return isPath(s, false)
}

// IsPathField is IsPath for a node field the parser has already checked
// as written: a path there may start with a renamed alias (MarkAlias).
func IsPathField(s string) bool {
	return isPath(s, true)
}

func isPath(s string, field bool) bool {
	if s == "" {
		return false
	}
	for _, seg := range strings.Split(s, ".") {
		if !isIdent(seg, field) {
			return false
		}
	}
	return true
}

// Each is a parsed each="path as alias".
type Each struct {
	Path  string
	Alias string
}

// ParseEach parses the each production. The grammar spells out its
// whitespace: exactly one space on each side of "as" and none around the
// path or the alias ("tickets  as item" and " tickets as item" are errors).
func ParseEach(s string) (Each, error) {
	return parseEach(s, false)
}

// ParseEachField is ParseEach for a node field the parser has already
// checked as written, where a renamed alias (MarkAlias) may stand.
func ParseEachField(s string) (Each, error) {
	return parseEach(s, true)
}

func parseEach(s string, field bool) (Each, error) {
	i := strings.Index(s, " as ")
	if i < 0 {
		return Each{}, fmt.Errorf(`each=%q must be "path as alias"`, s)
	}
	e := Each{Path: s[:i], Alias: s[i+len(" as "):]}
	if hasSpaceEdge(e.Path) || hasSpaceEdge(e.Alias) {
		return Each{}, fmt.Errorf(`each=%q must be "path as alias" with exactly one space on each side of "as" and no other spaces`, s)
	}
	if !isPath(e.Path, field) {
		return Each{}, fmt.Errorf("each=%q: %q is not a path", s, e.Path)
	}
	if !isIdent(e.Alias, field) {
		return Each{}, fmt.Errorf("each=%q: alias %q is not an identifier", s, e.Alias)
	}
	return e, nil
}

// hasSpaceEdge reports whether s starts or ends with whitespace.
func hasSpaceEdge(s string) bool {
	return s != strings.TrimSpace(s)
}

// Guard is a parsed if="path" / if="!path".
type Guard struct {
	Path string
	Neg  bool
}

// ParseGuard parses the if production: a path, or "!" immediately followed
// by a path. No whitespace is allowed anywhere (" x " and "! a" are errors).
func ParseGuard(s string) (Guard, error) {
	return parseGuard(s, false)
}

// ParseGuardField is ParseGuard for a node field the parser has already
// checked as written, where a renamed alias (MarkAlias) may stand.
func ParseGuardField(s string) (Guard, error) {
	return parseGuard(s, true)
}

func parseGuard(s string, field bool) (Guard, error) {
	g := Guard{Path: s}
	if strings.HasPrefix(s, "!") {
		g.Neg = true
		g.Path = s[1:]
	}
	if !isPath(g.Path, field) {
		return Guard{}, fmt.Errorf("%q is not a path or !path (no spaces)", s)
	}
	return g, nil
}

// Segment is a piece of an interpolated string: literal text or a {path}.
type Segment struct {
	Lit  string
	Path string
}

// ParseInterp splits s into literal and {path} segments. A "{" with no
// closing "}" on the same string is literal text. A closed brace pair must
// contain exactly a path, with no spaces ("{ folder }" is an error, like
// expressions, filters, and calls).
func ParseInterp(s string) ([]Segment, error) {
	return parseInterp(s, false)
}

// ParseInterpField is ParseInterp for a node field the parser has already
// checked as written, where a renamed alias (MarkAlias) may stand.
func ParseInterpField(s string) ([]Segment, error) {
	return parseInterp(s, true)
}

func parseInterp(s string, field bool) ([]Segment, error) {
	var out []Segment
	var lit strings.Builder
	for i := 0; i < len(s); {
		if s[i] == '{' {
			end := strings.IndexByte(s[i+1:], '}')
			if end >= 0 {
				inner := s[i+1 : i+1+end]
				if !isPath(inner, field) {
					if isPath(strings.TrimSpace(inner), field) {
						return nil, fmt.Errorf("{%s} is not a path: write {%s}, with no spaces inside the braces", inner, strings.TrimSpace(inner))
					}
					return nil, fmt.Errorf("{%s} is not a path; markup has no expressions", inner)
				}
				if lit.Len() > 0 {
					out = append(out, Segment{Lit: lit.String()})
					lit.Reset()
				}
				out = append(out, Segment{Path: inner})
				i += end + 2
				continue
			}
		}
		lit.WriteByte(s[i])
		i++
	}
	if lit.Len() > 0 {
		out = append(out, Segment{Lit: lit.String()})
	}
	return out, nil
}

// HasInterp reports whether s contains at least one {path}.
func HasInterp(s string) bool {
	segs, err := ParseInterp(s)
	if err != nil {
		return true
	}
	for _, sg := range segs {
		if sg.Path != "" {
			return true
		}
	}
	return false
}

// Truthy implements the truthiness table used by if / hidden / disabled.
func Truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case float64:
		return x != 0
	case int:
		return x != 0
	case string:
		return x != ""
	case []any:
		return len(x) > 0
	}
	return true
}

// IsGuardName reports whether s matches the guardname production of SPEC
// §7, the NAME of a class:NAME attribute: [a-z_][a-z0-9_-]*.
func IsGuardName(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '_', c >= 'a' && c <= 'z':
		case (c >= '0' && c <= '9') || c == '-':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// IsFlag reports whether s matches the flag production of SPEC §7: true,
// false, a path, or !path (hidden, disabled, open, mouse).
func IsFlag(s string) bool {
	if s == "true" || s == "false" {
		return true
	}
	_, err := ParseGuard(s)
	return err == nil
}
