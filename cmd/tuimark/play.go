package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/abdul-hamid-achik/tuimark/internal/dump"
	"github.com/abdul-hamid-achik/tuimark/internal/host"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/play"
)

// parseInputSteps splits --input on runs of ASCII spaces (leading/trailing
// ignored) and parses each token with play.ParseStep (SPEC §15.4). The
// replay engine itself (play.Session) lives in internal/play, shared with
// tuimark test and tuimark.Play; this file keeps only --input/--script
// parsing, --frames/--strict, and output formatting.
func parseInputSteps(s string) ([]play.Step, error) {
	return play.ParseSteps(s)
}

// decodeObjectMembers decodes data as a JSON object with exact,
// case-sensitive member names, refusing a duplicate member: plain
// json.Unmarshal into a map, or a struct's field matching, would instead
// silently keep only the last of a repeated key or match a name like
// "KEY" to a "key" field (finding 24).
func decodeObjectMembers(data []byte) (map[string]json.RawMessage, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '{' {
		return nil, fmt.Errorf("want a JSON object")
	}
	members := map[string]json.RawMessage{}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key := keyTok.(string)
		if _, dup := members[key]; dup {
			return nil, fmt.Errorf("duplicate member %q", key)
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, err
		}
		members[key] = raw
	}
	if _, err := dec.Token(); err != nil { // consume '}'
		return nil, err
	}
	return members, nil
}

// decodeStringMember unmarshals a script step member that must be a JSON
// string (key, text, paste, focus).
func decodeStringMember(name string, raw json.RawMessage) (string, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", fmt.Errorf("%s: want a JSON string: %v", name, err)
	}
	return s, nil
}

// parseScriptSet decodes a script line's `"set"` member (SPEC §15.4
// example: `{"set":{"path":"filter_open","value":true}}`): exactly the
// members `path` (a string) and `value`, both required, no others, so a
// typo like `"pth"` is a usage error instead of silently defaulting
// `path` to "" (the store root) or `value` to null (finding 24).
func parseScriptSet(line string, raw json.RawMessage) (play.Step, error) {
	members, err := decodeObjectMembers(raw)
	if err != nil {
		return play.Step{}, fmt.Errorf("set: %v", err)
	}
	for name := range members {
		if name != "path" && name != "value" {
			return play.Step{}, fmt.Errorf("set: unknown member %q (want exactly \"path\" and \"value\")", name)
		}
	}
	pathRaw, hasPath := members["path"]
	valueRaw, hasValue := members["value"]
	if !hasPath || !hasValue {
		return play.Step{}, fmt.Errorf("set: wants {\"path\": ..., \"value\": ...} (both required)")
	}
	path, err := decodeStringMember("set.path", pathRaw)
	if err != nil {
		return play.Step{}, err
	}
	var value any
	vdec := json.NewDecoder(bytes.NewReader(valueRaw))
	vdec.UseNumber()
	if err := vdec.Decode(&value); err != nil {
		return play.Step{}, fmt.Errorf("set: bad JSON value: %v", err)
	}
	return play.Step{Kind: play.KindSet, Raw: line, Path: path, Value: value}, nil
}

// parseScriptResize decodes a script line's `"resize"` member (SPEC §15.4
// example: `{"resize":[60,24]}`): a JSON array of exactly 2 integers, each
// checked against §15.1's 1-1000 range, so `[30,8,5]` is a usage error
// instead of silently dropping the third element (finding 24).
func parseScriptResize(line string, raw json.RawMessage) (play.Step, error) {
	var elems []json.RawMessage
	if err := json.Unmarshal(raw, &elems); err != nil {
		return play.Step{}, fmt.Errorf("resize: want [COLS,ROWS]: %v", err)
	}
	if len(elems) != 2 {
		return play.Step{}, fmt.Errorf("resize: want exactly [COLS,ROWS] (2 elements), got %d", len(elems))
	}
	var cols, rows int
	if err := json.Unmarshal(elems[0], &cols); err != nil {
		return play.Step{}, fmt.Errorf("resize: COLS must be an integer: %v", err)
	}
	if err := json.Unmarshal(elems[1], &rows); err != nil {
		return play.Step{}, fmt.Errorf("resize: ROWS must be an integer: %v", err)
	}
	if err := checkSize(cols, rows); err != nil {
		return play.Step{}, err
	}
	return play.Step{Kind: play.KindResize, Raw: line, Cols: cols, Rows: rows}, nil
}

// parseScriptSteps reads --script FILE.ndjson: one JSON object per line,
// blank lines ignored (SPEC §15.4).
func parseScriptSteps(path string) ([]play.Step, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var steps []play.Step
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	n := 0
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		n++
		st, err := parseScriptLine(line)
		if err != nil {
			return nil, fmt.Errorf("step %d (%s): %v", n, line, err)
		}
		steps = append(steps, st)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return steps, nil
}

// scriptMembers names the step members of a --script line (SPEC v0.2b
// §15.4), for error messages.
const scriptMembers = "key, text, paste, set, focus, resize, click, wheel with at, theme"

// parseScriptCell decodes the [X,Y] of a click or wheel script step: a
// JSON array of exactly two integers (SPEC v0.2b §15.4).
func parseScriptCell(name string, raw json.RawMessage) (x, y int, err error) {
	var elems []json.RawMessage
	if err := json.Unmarshal(raw, &elems); err != nil {
		return 0, 0, fmt.Errorf("%s: want [X,Y]: %v", name, err)
	}
	if len(elems) != 2 {
		return 0, 0, fmt.Errorf("%s: want exactly [X,Y] (2 elements), got %d", name, len(elems))
	}
	if err := json.Unmarshal(elems[0], &x); err != nil {
		return 0, 0, fmt.Errorf("%s: X must be an integer: %v", name, err)
	}
	if err := json.Unmarshal(elems[1], &y); err != nil {
		return 0, 0, fmt.Errorf("%s: Y must be an integer: %v", name, err)
	}
	return x, y, nil
}

// parseScriptWheel decodes {"wheel": "up" | "down", "at": [X,Y]}, the one
// step whose object holds two members, exactly these two (SPEC v0.2b
// §15.4).
func parseScriptWheel(line string, members map[string]json.RawMessage) (play.Step, error) {
	for name := range members {
		if name != "wheel" && name != "at" {
			return play.Step{}, fmt.Errorf("wheel: unknown member %q (want exactly \"wheel\" and \"at\")", name)
		}
	}
	atRaw, ok := members["at"]
	if !ok {
		return play.Step{}, fmt.Errorf("wheel: wants {\"wheel\": \"up\" or \"down\", \"at\": [X,Y]}")
	}
	dir, err := decodeStringMember("wheel", members["wheel"])
	if err != nil {
		return play.Step{}, err
	}
	st := play.Step{Kind: play.KindWheel, Raw: line}
	switch dir {
	case "up":
		st.Dir = -1
	case "down":
		st.Dir = 1
	default:
		return play.Step{}, fmt.Errorf("wheel: want \"up\" or \"down\", got %q", dir)
	}
	if st.X, st.Y, err = parseScriptCell("at", atRaw); err != nil {
		return play.Step{}, err
	}
	return st, nil
}

// parseScriptLine decodes one --script line (SPEC v0.2b §15.4): "Each
// object holds exactly one of these members" — key, text, paste, set,
// focus, resize, click, or theme, matched exactly and case-sensitively —
// except the wheel step, whose object holds exactly wheel and at. Before
// this, plain json.Unmarshal into a struct with pointer fields matched
// member names case-insensitively (accepting "KEY"), silently dropped an
// unknown member such as "extra", and kept only the last of a duplicate
// member (finding 24).
func parseScriptLine(line string) (play.Step, error) {
	members, err := decodeObjectMembers([]byte(line))
	if err != nil {
		return play.Step{}, fmt.Errorf("bad JSON: %v", err)
	}
	if _, ok := members["wheel"]; ok {
		return parseScriptWheel(line, members)
	}
	switch len(members) {
	case 0:
		return play.Step{}, fmt.Errorf("no known step member (want one of %s)", scriptMembers)
	default:
		if len(members) > 1 {
			return play.Step{}, fmt.Errorf("more than one step member on one line")
		}
	}
	var name string
	var raw json.RawMessage
	for n, v := range members {
		name, raw = n, v
	}
	switch name {
	case "key":
		s, err := decodeStringMember("key", raw)
		if err != nil {
			return play.Step{}, err
		}
		if !ir.ValidKey(s) {
			return play.Step{}, fmt.Errorf("%q is not a valid key token", s)
		}
		return play.Step{Kind: play.KindKey, Raw: line, Key: play.KeyFromToken(s)}, nil
	case "text":
		s, err := decodeStringMember("text", raw)
		if err != nil {
			return play.Step{}, err
		}
		return play.Step{Kind: play.KindText, Raw: line, Text: s}, nil
	case "paste":
		s, err := decodeStringMember("paste", raw)
		if err != nil {
			return play.Step{}, err
		}
		return play.Step{Kind: play.KindPaste, Raw: line, Text: s}, nil
	case "focus":
		s, err := decodeStringMember("focus", raw)
		if err != nil {
			return play.Step{}, err
		}
		return play.Step{Kind: play.KindFocus, Raw: line, Focus: strings.TrimPrefix(s, "#")}, nil
	case "set":
		return parseScriptSet(line, raw)
	case "resize":
		return parseScriptResize(line, raw)
	case "click":
		// SPEC v0.2b §15.4: {"click":[X,Y]} is click:X,Y.
		x, y, err := parseScriptCell("click", raw)
		if err != nil {
			return play.Step{}, err
		}
		return play.Step{Kind: play.KindClick, Raw: line, X: x, Y: y}, nil
	case "theme":
		// SPEC v0.2b §15.4: {"theme": "dark" | "light" | "auto"} is
		// Set("@theme", value); play never probes, so auto is dark.
		s, err := decodeStringMember("theme", raw)
		if err != nil {
			return play.Step{}, err
		}
		if s != "dark" && s != "light" && s != "auto" {
			return play.Step{}, fmt.Errorf("theme: want \"dark\", \"light\", or \"auto\", got %q", s)
		}
		return play.Step{Kind: play.KindSet, Raw: line, Path: host.ThemePath, Value: s}, nil
	}
	return play.Step{}, fmt.Errorf("unknown step member %q (want one of %s)", name, scriptMembers)
}

func (c *cli) cmdPlay(args []string) int {
	fs := c.newFlags("play")
	cols := fs.Int("cols", 80, "terminal columns")
	rows := fs.Int("rows", 24, "terminal rows")
	data := fs.String("data", "", "JSON file bound as the data store")
	theme := fs.String("theme", "", "dark or light; overrides the document's theme")
	input := fs.String("input", "", "space-separated play steps")
	script := fs.String("script", "", "NDJSON file of play steps")
	format := fs.String("format", "text", "text or json")
	cells := fs.Bool("cells", false, "include per-cell ownership (json only)")
	styles := fs.Bool("styles", false, "include theme and per-row style spans (json only)")
	frames := fs.Bool("frames", false, "include one settled frame per applied step")
	strict := fs.Bool("strict", false, "treat missing bind paths (B003) as errors")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return 1
	}
	file, err := oneFile("play", pos)
	if err != nil {
		return c.fail(err)
	}
	if err := checkSize(*cols, *rows); err != nil {
		return c.fail(err)
	}
	if *format != "text" && *format != "json" {
		return c.fail(fmt.Errorf("--format must be text or json"))
	}
	if *input != "" && *script != "" {
		return c.fail(fmt.Errorf("play: --input and --script are mutually exclusive"))
	}

	var steps []play.Step
	switch {
	case *input != "":
		steps, err = parseInputSteps(*input)
	case *script != "":
		steps, err = parseScriptSteps(*script)
	}
	if err != nil {
		return c.fail(fmt.Errorf("play: %v", err))
	}

	app, err := load(file, *data)
	if err != nil {
		return c.fail(err)
	}
	if err := applyTheme(app, *theme, themeFlagSet(fs), "--theme"); err != nil {
		return c.fail(err)
	}
	app.SetStrict(*strict)

	sess := play.NewSession(app, *cols, *rows, *cells, *styles, *frames, true)
	sess.Run(steps)
	if sess.UsageErr != nil {
		return c.fail(fmt.Errorf("play: step %d (%s): %v", sess.UsageStep, sess.UsageRaw, sess.UsageErr))
	}

	if *format == "json" {
		out := &dump.Play{Dump: sess.BuildDump(), Events: sess.Events}
		if *frames {
			out.Frames = sess.StepFrames
		}
		b, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return c.fail(err)
		}
		c.stdout.Write(b)
		fmt.Fprintln(c.stdout)
		if !out.OK {
			return 2
		}
		return 0
	}

	var b strings.Builder
	if *frames {
		for _, pf := range sess.StepFrames {
			if pf.Step == 0 {
				b.WriteString("=== step 0 ===\n")
			} else {
				fmt.Fprintf(&b, "=== step %d: %s ===\n", pf.Step, pf.Input)
			}
			b.WriteString(dump.Text(pf.Dump))
			play.WriteEventsSection(&b, pf.Events)
		}
	}
	d := sess.BuildDump()
	b.WriteString(dump.Text(d))
	play.WriteEventsSection(&b, sess.Events)
	fmt.Fprint(c.stdout, b.String())
	if !d.OK {
		return 2
	}
	return 0
}

// parseSize parses "COLSxROWS" (shared with resize: and tuimark test).
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
