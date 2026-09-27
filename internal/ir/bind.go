package ir

import (
	"fmt"
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
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '_', c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}

// IsPath reports whether s matches the path production.
func IsPath(s string) bool {
	if s == "" {
		return false
	}
	for _, seg := range strings.Split(s, ".") {
		if !IsIdent(seg) {
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
	i := strings.Index(s, " as ")
	if i < 0 {
		return Each{}, fmt.Errorf(`each=%q must be "path as alias"`, s)
	}
	e := Each{Path: s[:i], Alias: s[i+len(" as "):]}
	if hasSpaceEdge(e.Path) || hasSpaceEdge(e.Alias) {
		return Each{}, fmt.Errorf(`each=%q must be "path as alias" with exactly one space on each side of "as" and no other spaces`, s)
	}
	if !IsPath(e.Path) {
		return Each{}, fmt.Errorf("each=%q: %q is not a path", s, e.Path)
	}
	if !IsIdent(e.Alias) {
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
	g := Guard{Path: s}
	if strings.HasPrefix(s, "!") {
		g.Neg = true
		g.Path = s[1:]
	}
	if !IsPath(g.Path) {
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
	var out []Segment
	var lit strings.Builder
	for i := 0; i < len(s); {
		if s[i] == '{' {
			end := strings.IndexByte(s[i+1:], '}')
			if end >= 0 {
				inner := s[i+1 : i+1+end]
				if !IsPath(inner) {
					if IsPath(strings.TrimSpace(inner)) {
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
