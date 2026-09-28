// Package play is the headless replay engine of SPEC §15.4: it serves
// tuimark play, tuimark test, and (SPEC v0.3 §18.1) tuimark.Play, so
// their parity is guaranteed by construction (one engine, one place the
// steps are applied). cmd/tuimark keeps only flag parsing, --script
// parsing, --frames/--strict, and output formatting; tuimark.go keeps
// only the PlayOptions/PlayResult translation.
package play

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/abdul-hamid-achik/tuimark/internal/host"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// Kind is one parsed play step's kind (SPEC v0.2 §15.4).
type Kind int

const (
	KindKey Kind = iota
	KindText
	KindPaste
	KindSet
	KindFocus
	KindResize
	KindClick // click:X,Y or {"click":[X,Y]} (SPEC v0.2b §15.4)
	KindWheel // wheel-up:X,Y, wheel-down:X,Y, or {"wheel":…,"at":[X,Y]}
)

// Step is one parsed, validated step of a play session. Raw is "the step
// as written" (SPEC §15.4): the --input token, the --script line, or one
// of Play's steps, used verbatim in events/frames output and in an error
// message.
type Step struct {
	Kind  Kind
	Raw   string
	Key   host.Key
	Text  string // KindText, KindPaste
	Path  string // KindSet
	Value any    // KindSet, decoded JSON
	Focus string // KindFocus
	Cols  int    // KindResize
	Rows  int    // KindResize
	X, Y  int    // KindClick, KindWheel: the 0-based cell
	Dir   int    // KindWheel: -1 up, +1 down
}

// checkSize is the 1-1000 range every size in the SPEC's tools shares
// (--cols/--rows, resize:, Play's opts.Cols/opts.Rows).
func checkSize(cols, rows int) error {
	if cols <= 0 || rows <= 0 || cols > 1000 || rows > 1000 {
		return fmt.Errorf("--cols and --rows must be between 1 and 1000 (got %dx%d)", cols, rows)
	}
	return nil
}

// parseCoord parses one coordinate of a mouse step: a decimal integer
// (SPEC v0.2b §15.4). Whether the cell lies inside the grid is decided
// when the step is applied, against the size in effect then.
func parseCoord(s string) (int, error) {
	digits := strings.TrimPrefix(s, "-")
	if digits == "" || len(digits) > 9 || strings.Trim(digits, "0123456789") != "" {
		return 0, fmt.Errorf("%q is not a decimal integer", s)
	}
	return strconv.Atoi(s)
}

// parseCell parses the "X,Y" of click:X,Y, wheel-up:X,Y, and
// wheel-down:X,Y (SPEC v0.2b §15.4).
func parseCell(s string) (x, y int, err error) {
	xs, ys, ok := strings.Cut(s, ",")
	if !ok {
		return 0, 0, fmt.Errorf("want X,Y")
	}
	if x, err = parseCoord(xs); err != nil {
		return 0, 0, err
	}
	if y, err = parseCoord(ys); err != nil {
		return 0, 0, err
	}
	return x, y, nil
}

// ParseStep parses one step of the §15.4 --input grammar: a key token, or
// text:/paste:/set:/focus:/resize:/click:/wheel-up:/wheel-down:. It never
// splits tok on spaces, so a caller that wants tuimark play's --input
// splitting does that first (ParseStep is also what tuimark.Play parses
// each of its variadic steps with).
func ParseStep(tok string) (Step, error) {
	switch {
	case strings.HasPrefix(tok, "text:"):
		return Step{Kind: KindText, Raw: tok, Text: tok[len("text:"):]}, nil
	case strings.HasPrefix(tok, "paste:"):
		return Step{Kind: KindPaste, Raw: tok, Text: tok[len("paste:"):]}, nil
	case strings.HasPrefix(tok, "set:"):
		return parseSetToken(tok)
	case strings.HasPrefix(tok, "focus:"):
		return Step{Kind: KindFocus, Raw: tok, Focus: strings.TrimPrefix(tok[len("focus:"):], "#")}, nil
	case strings.HasPrefix(tok, "resize:"):
		return parseResizeToken(tok)
	case strings.HasPrefix(tok, "click:"), strings.HasPrefix(tok, "wheel-up:"), strings.HasPrefix(tok, "wheel-down:"):
		// SPEC v0.2b §15.4: a left click (press and release) or one wheel
		// report at cell (X, Y), 0-based as in the dump.
		name, cell, _ := strings.Cut(tok, ":")
		x, y, err := parseCell(cell)
		if err != nil {
			return Step{}, fmt.Errorf("%s: %v", name, err)
		}
		st := Step{Kind: KindClick, Raw: tok, X: x, Y: y}
		switch name {
		case "wheel-up":
			st.Kind, st.Dir = KindWheel, -1
		case "wheel-down":
			st.Kind, st.Dir = KindWheel, 1
		}
		return st, nil
	}
	if !ir.ValidKey(tok) {
		return Step{}, fmt.Errorf("not a valid key token")
	}
	return Step{Kind: KindKey, Raw: tok, Key: KeyFromToken(tok)}, nil
}

// ParseSteps splits s on runs of ASCII spaces (leading/trailing ignored,
// SPEC §15.4's --input grammar) and parses each token with ParseStep.
func ParseSteps(s string) ([]Step, error) {
	var toks []string
	for _, f := range strings.Split(s, " ") {
		if f != "" {
			toks = append(toks, f)
		}
	}
	steps := make([]Step, 0, len(toks))
	for i, tok := range toks {
		st, err := ParseStep(tok)
		if err != nil {
			return nil, fmt.Errorf("step %d (%s): %v", i+1, tok, err)
		}
		steps = append(steps, st)
	}
	return steps, nil
}

func parseSetToken(tok string) (Step, error) {
	rest := tok[len("set:"):]
	idx := strings.IndexByte(rest, '=')
	if idx < 0 {
		return Step{}, fmt.Errorf("set: wants PATH=JSON")
	}
	path, jsonText := rest[:idx], rest[idx+1:]
	var v any
	if err := json.Unmarshal([]byte(jsonText), &v); err != nil {
		return Step{}, fmt.Errorf("bad JSON value: %v", err)
	}
	return Step{Kind: KindSet, Raw: tok, Path: path, Value: v}, nil
}

// parseSize parses "COLSxROWS" (resize: and the CLI's own flags share
// this grammar; cmd/tuimark keeps its own copy for its size flags).
func parseSize(s string) (cols, rows int, err error) {
	parts := strings.SplitN(s, "x", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("bad size %q (want COLSxROWS)", s)
	}
	cols, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("bad size %q: %v", s, err)
	}
	rows, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("bad size %q: %v", s, err)
	}
	return cols, rows, nil
}

func parseResizeToken(tok string) (Step, error) {
	rest := tok[len("resize:"):]
	cols, rows, err := parseSize(rest)
	if err != nil {
		return Step{}, err
	}
	if err := checkSize(cols, rows); err != nil {
		return Step{}, err
	}
	return Step{Kind: KindResize, Raw: tok, Cols: cols, Rows: rows}, nil
}

// KeyFromToken maps a validated (ir.ValidKey) key token to the host.Key
// the terminal decoder would have produced for it (SPEC §26.8): only
// "space" among the named tokens is printable. `ctrl+i`, `ctrl+j`, and
// `ctrl+m` are their own bytes (0x09, 0x0a, 0x0d), which
// internal/host/keys.go's scanInput decodes as `tab`/`enter`/`enter`
// before the generic `ctrl+<letter>` case ever runs (SPEC §8.1: "ctrl+i,
// ctrl+j, and ctrl+m arrive as tab/enter and never match"), so a play key
// step for one of them must deliver that same key, never the
// bound-but-unreachable `ctrl+i`/`ctrl+j`/`ctrl+m` token, or play could
// dispatch an event Run can never fire for the same input.
func KeyFromToken(tok string) host.Key {
	if tok == "space" {
		return host.Key{Name: "space", Rune: ' '}
	}
	switch tok {
	case "ctrl+i":
		return host.Key{Name: "tab"}
	case "ctrl+j", "ctrl+m":
		return host.Key{Name: "enter"}
	}
	for _, k := range ir.NamedKeys {
		if tok == k {
			return host.Key{Name: tok}
		}
	}
	if strings.HasPrefix(tok, "ctrl+") {
		return host.Key{Name: tok}
	}
	r := []rune(tok)[0]
	return host.Key{Name: string(r), Rune: r}
}
