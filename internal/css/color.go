// Package css implements TCSS v1 (SPEC §10): a reduced, cell-based CSS with
// type/#id/.class/child/pseudo selectors, theme tokens, one-feature @media
// blocks, and a cascade that computes a typed style per node.
package css

import (
	"fmt"
	"strconv"
	"strings"
)

// ColorKind distinguishes unset, terminal-default, 16-color, and RGB colors.
type ColorKind uint8

const (
	ColorNone ColorKind = iota // not set: inherit / leave the cell alone
	ColorDefault
	ColorANSI
	ColorRGB
)

// Color is a resolved terminal color.
type Color struct {
	Kind    ColorKind
	Index   uint8 // ANSI 0-15
	R, G, B uint8
}

// IsSet reports whether the color was specified.
func (c Color) IsSet() bool { return c.Kind != ColorNone }

func (c Color) String() string {
	switch c.Kind {
	case ColorDefault:
		return "default"
	case ColorANSI:
		return ansiNames[c.Index]
	case ColorRGB:
		return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
	}
	return ""
}

var ansiNames = []string{
	"black", "red", "green", "yellow", "blue", "magenta", "cyan", "white",
	"bright-black", "bright-red", "bright-green", "bright-yellow",
	"bright-blue", "bright-magenta", "bright-cyan", "bright-white",
}

// RequiredTokens must exist after the theme is applied (SPEC §10.4).
var RequiredTokens = []string{"bg", "fg", "muted", "accent", "ok", "warn", "danger", "border", "focus", "panel"}

// Themes are the built-in token sets selectable with <tui theme="...">.
var Themes = map[string]map[string]string{
	"dark": {
		"bg": "#0d1117", "fg": "#e6edf3", "muted": "#8b949e", "accent": "cyan",
		"ok": "green", "warn": "yellow", "danger": "red", "border": "#30363d",
		"focus": "cyan", "panel": "#161b22",
	},
	"light": {
		"bg": "#ffffff", "fg": "#1f2328", "muted": "#656d76", "accent": "blue",
		"ok": "green", "warn": "yellow", "danger": "red", "border": "#d0d7de",
		"focus": "blue", "panel": "#f6f8fa",
	},
}

// ParseLiteralColor parses a color that is not a token reference.
func ParseLiteralColor(s string) (Color, error) {
	s = strings.TrimSpace(s)
	if s == "default" {
		return Color{Kind: ColorDefault}, nil
	}
	for i, n := range ansiNames {
		if s == n {
			return Color{Kind: ColorANSI, Index: uint8(i)}, nil
		}
	}
	if strings.HasPrefix(s, "#") {
		hex := s[1:]
		if len(hex) == 3 {
			hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
		}
		if len(hex) == 6 {
			v, err := strconv.ParseUint(hex, 16, 32)
			if err == nil {
				return Color{Kind: ColorRGB, R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v)}, nil
			}
		}
	}
	return Color{}, fmt.Errorf("bad color %q (want $token, var(--token), an ANSI name, #rgb, #rrggbb, or default)", s)
}

// TokenRef returns the token name if s is `$name` or `var(--name)`.
func TokenRef(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "$") {
		return s[1:], true
	}
	if strings.HasPrefix(s, "var(--") && strings.HasSuffix(s, ")") {
		return strings.TrimSpace(s[6 : len(s)-1]), true
	}
	return "", false
}

func isTokenName(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c == '-' || c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9' && i > 0)) {
			return false
		}
	}
	return true
}

// checkColorSyntax validates a color value without resolving tokens.
func checkColorSyntax(v string) error {
	if name, ok := TokenRef(v); ok {
		if !isTokenName(name) {
			return fmt.Errorf("bad token reference %q", v)
		}
		return nil
	}
	_, err := ParseLiteralColor(v)
	return err
}

// ResolveColor resolves a (possibly token) color against a token table.
func ResolveColor(v string, tokens map[string]string) (Color, error) {
	if name, ok := TokenRef(v); ok {
		val, ok := tokens[name]
		if !ok {
			return Color{}, fmt.Errorf("unknown token $%s", name)
		}
		if _, nested := TokenRef(val); nested {
			return Color{}, fmt.Errorf("token $%s refers to another token; tokens must be literal colors", name)
		}
		return ParseLiteralColor(val)
	}
	return ParseLiteralColor(v)
}
