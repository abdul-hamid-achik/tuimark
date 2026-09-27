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
)

// stepKind is one parsed `play` step (SPEC v0.2 §15.4).
type stepKind int

const (
	stepKey stepKind = iota
	stepText
	stepPaste
	stepSet
	stepFocus
	stepResize
)

// playStep is one step of a play session, already parsed and validated.
// raw is "the step as written" (SPEC §15.4): the --input token, or the
// --script line with surrounding whitespace removed, used verbatim in
// `events`/`frames` output and in a usage-error message.
type playStep struct {
	kind  stepKind
	raw   string
	key   host.Key
	text  string // stepText, stepPaste
	path  string // stepSet
	value any    // stepSet, decoded JSON
	focus string // stepFocus
	cols  int    // stepResize
	rows  int    // stepResize
}

// reservedStep reports the usage error for a mouse step of SPEC v0.2b
// §15.4 this build does not run yet, or "" when tok/member is not one of
// them. The `theme` script step is a step (Set("@theme", value)).
func reservedStep(name string) string {
	switch name {
	case "click", "wheel-up", "wheel-down", "wheel":
		return fmt.Sprintf("%q is a 0.2b mouse step, not implemented in this build", name)
	}
	return ""
}

// parseInputSteps splits --input on runs of ASCII spaces (leading/trailing
// ignored) and parses each token (SPEC §15.4).
func parseInputSteps(s string) ([]playStep, error) {
	var toks []string
	for _, f := range strings.Split(s, " ") {
		if f != "" {
			toks = append(toks, f)
		}
	}
	steps := make([]playStep, 0, len(toks))
	for i, tok := range toks {
		st, err := parseInputToken(tok)
		if err != nil {
			return nil, fmt.Errorf("step %d (%s): %v", i+1, tok, err)
		}
		steps = append(steps, st)
	}
	return steps, nil
}

func parseInputToken(tok string) (playStep, error) {
	switch {
	case strings.HasPrefix(tok, "text:"):
		return playStep{kind: stepText, raw: tok, text: tok[len("text:"):]}, nil
	case strings.HasPrefix(tok, "paste:"):
		return playStep{kind: stepPaste, raw: tok, text: tok[len("paste:"):]}, nil
	case strings.HasPrefix(tok, "set:"):
		return parseSetToken(tok)
	case strings.HasPrefix(tok, "focus:"):
		return playStep{kind: stepFocus, raw: tok, focus: strings.TrimPrefix(tok[len("focus:"):], "#")}, nil
	case strings.HasPrefix(tok, "resize:"):
		return parseResizeToken(tok)
	case strings.HasPrefix(tok, "click:"), strings.HasPrefix(tok, "wheel-up:"), strings.HasPrefix(tok, "wheel-down:"):
		name, _, _ := strings.Cut(tok, ":")
		return playStep{}, fmt.Errorf("%s", reservedStep(name))
	}
	if !ir.ValidKey(tok) {
		return playStep{}, fmt.Errorf("not a valid key token")
	}
	return playStep{kind: stepKey, raw: tok, key: keyFromToken(tok)}, nil
}

func parseSetToken(tok string) (playStep, error) {
	rest := tok[len("set:"):]
	idx := strings.IndexByte(rest, '=')
	if idx < 0 {
		return playStep{}, fmt.Errorf("set: wants PATH=JSON")
	}
	path, jsonText := rest[:idx], rest[idx+1:]
	var v any
	if err := json.Unmarshal([]byte(jsonText), &v); err != nil {
		return playStep{}, fmt.Errorf("bad JSON value: %v", err)
	}
	return playStep{kind: stepSet, raw: tok, path: path, value: v}, nil
}

func parseResizeToken(tok string) (playStep, error) {
	rest := tok[len("resize:"):]
	cols, rows, err := parseSize(rest)
	if err != nil {
		return playStep{}, err
	}
	if err := checkSize(cols, rows); err != nil {
		return playStep{}, err
	}
	return playStep{kind: stepResize, raw: tok, cols: cols, rows: rows}, nil
}

// keyFromToken maps a validated (ir.ValidKey) key token to the host.Key the
// terminal decoder would have produced for it (SPEC §26.8): only "space"
// among the named tokens is printable. `ctrl+i`, `ctrl+j`, and `ctrl+m`
// are their own bytes (0x09, 0x0a, 0x0d), which internal/host/keys.go's
// scanInput decodes as `tab`/`enter`/`enter` before the generic
// `ctrl+<letter>` case ever runs (finding 31, SPEC §8.1: "ctrl+i, ctrl+j,
// and ctrl+m arrive as tab/enter and never match"), so a play key step
// for one of them must deliver that same key, never the bound-but-
// unreachable `ctrl+i`/`ctrl+j`/`ctrl+m` token, or `play` could dispatch
// an event Run can never fire for the same input.
func keyFromToken(tok string) host.Key {
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
func parseScriptSet(line string, raw json.RawMessage) (playStep, error) {
	members, err := decodeObjectMembers(raw)
	if err != nil {
		return playStep{}, fmt.Errorf("set: %v", err)
	}
	for name := range members {
		if name != "path" && name != "value" {
			return playStep{}, fmt.Errorf("set: unknown member %q (want exactly \"path\" and \"value\")", name)
		}
	}
	pathRaw, hasPath := members["path"]
	valueRaw, hasValue := members["value"]
	if !hasPath || !hasValue {
		return playStep{}, fmt.Errorf("set: wants {\"path\": ..., \"value\": ...} (both required)")
	}
	path, err := decodeStringMember("set.path", pathRaw)
	if err != nil {
		return playStep{}, err
	}
	var value any
	vdec := json.NewDecoder(bytes.NewReader(valueRaw))
	vdec.UseNumber()
	if err := vdec.Decode(&value); err != nil {
		return playStep{}, fmt.Errorf("set: bad JSON value: %v", err)
	}
	return playStep{kind: stepSet, raw: line, path: path, value: value}, nil
}

// parseScriptResize decodes a script line's `"resize"` member (SPEC §15.4
// example: `{"resize":[60,24]}`): a JSON array of exactly 2 integers, each
// checked against §15.1's 1-1000 range, so `[30,8,5]` is a usage error
// instead of silently dropping the third element (finding 24).
func parseScriptResize(line string, raw json.RawMessage) (playStep, error) {
	var elems []json.RawMessage
	if err := json.Unmarshal(raw, &elems); err != nil {
		return playStep{}, fmt.Errorf("resize: want [COLS,ROWS]: %v", err)
	}
	if len(elems) != 2 {
		return playStep{}, fmt.Errorf("resize: want exactly [COLS,ROWS] (2 elements), got %d", len(elems))
	}
	var cols, rows int
	if err := json.Unmarshal(elems[0], &cols); err != nil {
		return playStep{}, fmt.Errorf("resize: COLS must be an integer: %v", err)
	}
	if err := json.Unmarshal(elems[1], &rows); err != nil {
		return playStep{}, fmt.Errorf("resize: ROWS must be an integer: %v", err)
	}
	if err := checkSize(cols, rows); err != nil {
		return playStep{}, err
	}
	return playStep{kind: stepResize, raw: line, cols: cols, rows: rows}, nil
}

// parseScriptSteps reads --script FILE.ndjson: one JSON object per line,
// blank lines ignored (SPEC §15.4).
func parseScriptSteps(path string) ([]playStep, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var steps []playStep
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

// parseScriptLine decodes one --script line (SPEC §15.4): "Each object
// holds exactly one of these members" — key, text, paste, set, focus,
// resize, or (0.2b) theme, matched exactly and case-sensitively, with the
// click/wheel mouse steps rejected as not implemented yet. Before this, plain json.Unmarshal into a
// struct with pointer fields matched member names case-insensitively
// (accepting "KEY"), silently dropped an unknown member such as "extra",
// and kept only the last of a duplicate member (finding 24).
func parseScriptLine(line string) (playStep, error) {
	members, err := decodeObjectMembers([]byte(line))
	if err != nil {
		return playStep{}, fmt.Errorf("bad JSON: %v", err)
	}
	// A reserved member name is a usage error even alongside another
	// member (the §15.4 example `{"wheel":"up","at":[X,Y]}}` has two), so
	// this check runs before the exactly-one-member count below.
	for name := range members {
		if r := reservedStep(name); r != "" {
			return playStep{}, fmt.Errorf("%s", r)
		}
	}
	switch len(members) {
	case 0:
		return playStep{}, fmt.Errorf("no known step member (want one of key, text, paste, set, focus, resize, theme)")
	default:
		if len(members) > 1 {
			return playStep{}, fmt.Errorf("more than one step member on one line")
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
			return playStep{}, err
		}
		if !ir.ValidKey(s) {
			return playStep{}, fmt.Errorf("%q is not a valid key token", s)
		}
		return playStep{kind: stepKey, raw: line, key: keyFromToken(s)}, nil
	case "text":
		s, err := decodeStringMember("text", raw)
		if err != nil {
			return playStep{}, err
		}
		return playStep{kind: stepText, raw: line, text: s}, nil
	case "paste":
		s, err := decodeStringMember("paste", raw)
		if err != nil {
			return playStep{}, err
		}
		return playStep{kind: stepPaste, raw: line, text: s}, nil
	case "focus":
		s, err := decodeStringMember("focus", raw)
		if err != nil {
			return playStep{}, err
		}
		return playStep{kind: stepFocus, raw: line, focus: strings.TrimPrefix(s, "#")}, nil
	case "set":
		return parseScriptSet(line, raw)
	case "resize":
		return parseScriptResize(line, raw)
	case "theme":
		// SPEC v0.2b §15.4: {"theme": "dark" | "light" | "auto"} is
		// Set("@theme", value); play never probes, so auto is dark.
		s, err := decodeStringMember("theme", raw)
		if err != nil {
			return playStep{}, err
		}
		if s != "dark" && s != "light" && s != "auto" {
			return playStep{}, fmt.Errorf("theme: want \"dark\", \"light\", or \"auto\", got %q", s)
		}
		return playStep{kind: stepSet, raw: line, path: host.ThemePath, value: s}, nil
	}
	return playStep{}, fmt.Errorf("unknown step member %q (want one of key, text, paste, set, focus, resize, theme)", name)
}

// playSession replays steps against app through the same primitives Run's
// loop uses (Frame, HandleKeyRun, HandlePaste, Dispatch, TakePending,
// Focus), without a TTY (SPEC v0.2 §15.4).
type playSession struct {
	app           *host.App
	cols, rows    int
	frame         *host.Frame
	events        []dump.Event // cumulative, in order, for the top-level "events"
	curStepEvents []dump.Event // this step's own events, for --frames
	quit          bool
	cellsFlag     bool
	stylesFlag    bool
	stepFrames    []dump.PlayFrame // only collected with --frames
	collectFrames bool

	// usageErr, when set, is a step's Set/@focus error (bad path, unknown
	// screen): the session stops right there, exactly like a quit, but
	// cmdPlay reports it as a usage error (SPEC §15.4 "Errors") instead of
	// printing a dump.
	usageErr  error
	usageStep int
	usageRaw  string
}

func newPlaySession(app *host.App, cols, rows int, cells, styles, frames bool) *playSession {
	return &playSession{
		app: app, cols: cols, rows: rows,
		events: []dump.Event{}, cellsFlag: cells, stylesFlag: styles, collectFrames: frames,
	}
}

func (s *playSession) draw() { s.frame = s.app.Frame(s.cols, s.rows) }

// dispatchAll dispatches evs, then TakePending in rounds, redrawing between
// rounds, for at most 8 rounds (the v0.1 loop; SPEC §15.4 point 3).
func (s *playSession) dispatchAll(step int, evs []host.Event) {
	for round := 0; round < 8; round++ {
		for _, ev := range evs {
			de := dump.Event{Step: step, Action: ev.Action, Source: ev.Source, Keys: ev.Keys, Value: ev.Value}
			if de.Keys == nil {
				de.Keys = map[string]any{}
			}
			s.events = append(s.events, de)
			s.curStepEvents = append(s.curStepEvents, de)
			quit, _ := s.app.Dispatch(ev) // play registers no handlers: err is always nil
			if quit {
				s.quit = true
				return
			}
		}
		evs = s.app.TakePending()
		if len(evs) == 0 {
			return
		}
		s.draw()
	}
}

// settle draws and runs the events a render queued, exactly as Run's loop
// settles after applying one step (SPEC §15.4 point 3).
func (s *playSession) settle(step int) {
	s.draw()
	s.dispatchAll(step, nil)
}

// handleKeys applies keys through HandleKeyRun's coalescing, dispatching
// each run's events and redrawing when they fired or focus moved so a later
// key in the same step sees the fresh frame (mirrors internal/host/run.go's
// handleKeys).
func (s *playSession) handleKeys(step int, keys []host.Key) {
	for len(keys) > 0 && !s.quit {
		focus := s.app.Focus()
		evs, n := s.app.HandleKeyRun(keys)
		keys = keys[n:]
		s.dispatchAll(step, evs)
		if s.quit {
			return
		}
		if len(evs) > 0 || s.app.Focus() != focus {
			s.draw()
		}
	}
}

// handleText decodes str with the §26.8 decoder and applies the resulting
// keys and pastes in order, exactly as Run's handleInputs walks one read
// (SPEC v0.2 §15.4 `text:STR`, §28.21: "text: goes through it in one
// read"; events must be exactly what Run would dispatch). A run of
// consecutive keys is coalesced through handleKeys (the v0.1 one-edit
// behavior for printable characters into a focused input); each paste is
// its own handlePaste call.
func (s *playSession) handleText(step int, str string) {
	ins := host.DecodeInput([]byte(str))
	for len(ins) > 0 && !s.quit {
		if ins[0].IsPaste {
			s.handlePaste(step, ins[0].Paste)
			ins = ins[1:]
			continue
		}
		n := 1
		for n < len(ins) && !ins[n].IsPaste {
			n++
		}
		keys := make([]host.Key, n)
		for i := range keys {
			keys[i] = ins[i].Key
		}
		ins = ins[n:]
		s.handleKeys(step, keys)
	}
}

func (s *playSession) handlePaste(step int, payload string) {
	focus := s.app.Focus()
	evs := s.app.HandlePaste(payload)
	s.dispatchAll(step, evs)
	if s.quit {
		return
	}
	if len(evs) > 0 || s.app.Focus() != focus {
		s.draw()
	}
}

// buildDump builds the dump of the session's current frame with this
// session's --cells/--styles flags.
func (s *playSession) buildDump() *dump.Dump {
	return frameDump(s.frame, s.cellsFlag, s.stylesFlag)
}

// snapshot records this step's PlayFrame when --frames is set.
func (s *playSession) snapshot(step int, input string) {
	if !s.collectFrames {
		return
	}
	evs := s.curStepEvents
	if evs == nil {
		evs = []dump.Event{}
	}
	s.stepFrames = append(s.stepFrames, dump.PlayFrame{Step: step, Input: input, Events: evs, Dump: s.buildDump()})
}

// run replays step 0 (the first live frame) and then every step in order,
// stopping early on quit (SPEC §15.4).
func (s *playSession) run(steps []playStep) {
	s.settle(0)
	s.snapshot(0, "")
	for i, st := range steps {
		if s.quit || s.usageErr != nil {
			break
		}
		s.curStepEvents = nil
		stepNum := i + 1
		switch st.kind {
		case stepKey:
			s.handleKeys(stepNum, []host.Key{st.key})
		case stepText:
			s.handleText(stepNum, st.text)
		case stepPaste:
			s.handlePaste(stepNum, st.text)
		case stepSet:
			if err := s.app.Set(st.path, st.value); err != nil {
				s.usageErr, s.usageStep, s.usageRaw = err, stepNum, st.raw
				return
			}
		case stepFocus:
			if err := s.app.Set("@focus", st.focus); err != nil {
				s.usageErr, s.usageStep, s.usageRaw = err, stepNum, st.raw
				return
			}
		case stepResize:
			s.cols, s.rows = st.cols, st.rows
		}
		if !s.quit {
			s.settle(stepNum)
		}
		s.snapshot(stepNum, st.raw)
	}
}

func formatEventLine(e dump.Event) string {
	src := e.Source
	if src == "" {
		src = "-"
	}
	keysJSON, _ := json.Marshal(e.Keys)
	valJSON, _ := json.Marshal(e.Value)
	return fmt.Sprintf("%d %s %s %s %s", e.Step, e.Action, src, keysJSON, valJSON)
}

func writeEventsSection(b *strings.Builder, evs []dump.Event) {
	b.WriteString("=== events ===\n")
	if len(evs) == 0 {
		b.WriteString("none\n")
		return
	}
	for _, e := range evs {
		b.WriteString(formatEventLine(e))
		b.WriteByte('\n')
	}
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

	var steps []playStep
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

	sess := newPlaySession(app, *cols, *rows, *cells, *styles, *frames)
	sess.run(steps)
	if sess.usageErr != nil {
		return c.fail(fmt.Errorf("play: step %d (%s): %v", sess.usageStep, sess.usageRaw, sess.usageErr))
	}

	if *format == "json" {
		play := &dump.Play{Dump: sess.buildDump(), Events: sess.events}
		if *frames {
			play.Frames = sess.stepFrames
		}
		b, err := json.MarshalIndent(play, "", "  ")
		if err != nil {
			return c.fail(err)
		}
		c.stdout.Write(b)
		fmt.Fprintln(c.stdout)
		if !play.OK {
			return 2
		}
		return 0
	}

	var b strings.Builder
	if *frames {
		for _, pf := range sess.stepFrames {
			if pf.Step == 0 {
				b.WriteString("=== step 0 ===\n")
			} else {
				fmt.Fprintf(&b, "=== step %d: %s ===\n", pf.Step, pf.Input)
			}
			b.WriteString(dump.Text(pf.Dump))
			writeEventsSection(&b, pf.Events)
		}
	}
	d := sess.buildDump()
	b.WriteString(dump.Text(d))
	writeEventsSection(&b, sess.events)
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
