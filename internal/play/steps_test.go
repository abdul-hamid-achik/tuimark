package play

import (
	"reflect"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/host"
)

// ParseStep's accepted grammar (SPEC §15.4): a key token, or one of the
// prefixed forms. These cases pin what each form does with its edge
// inputs, including the ones the grammar is lenient about.
func TestParseStepAccepted(t *testing.T) {
	for _, c := range []struct {
		tok  string
		want Step
	}{
		{"enter", Step{Kind: KindKey, Raw: "enter", Key: host.Key{Name: "enter"}, Tok: "enter"}},
		{"a", Step{Kind: KindKey, Raw: "a", Key: host.Key{Name: "a", Rune: 'a'}, Tok: "a"}},
		{"text:", Step{Kind: KindText, Raw: "text:"}}, // empty text is a step that types nothing
		{"text:hi", Step{Kind: KindText, Raw: "text:hi", Text: "hi"}},
		{"paste:a=b", Step{Kind: KindPaste, Raw: "paste:a=b", Text: "a=b"}},
		{"focus:q", Step{Kind: KindFocus, Raw: "focus:q", Focus: "q"}},
		{"focus:#q", Step{Kind: KindFocus, Raw: "focus:#q", Focus: "q"}},
		{"focus:", Step{Kind: KindFocus, Raw: "focus:"}},                    // empty: clears the focus request
		{"focus:##q", Step{Kind: KindFocus, Raw: "focus:##q", Focus: "#q"}}, // one leading # only
		{"resize:80x24", Step{Kind: KindResize, Raw: "resize:80x24", Cols: 80, Rows: 24}},
		{"resize:1x1", Step{Kind: KindResize, Raw: "resize:1x1", Cols: 1, Rows: 1}},
		{"resize:1000x1000", Step{Kind: KindResize, Raw: "resize:1000x1000", Cols: 1000, Rows: 1000}},
		{"resize:+80x24", Step{Kind: KindResize, Raw: "resize:+80x24", Cols: 80, Rows: 24}}, // strconv.Atoi takes a sign
		{"click:3,4", Step{Kind: KindClick, Raw: "click:3,4", X: 3, Y: 4}},
		{"click:-1,0", Step{Kind: KindClick, Raw: "click:-1,0", X: -1}}, // range is judged when applied
		{"click:007,000", Step{Kind: KindClick, Raw: "click:007,000", X: 7}},
		{"wheel-up:5,6", Step{Kind: KindWheel, Raw: "wheel-up:5,6", X: 5, Y: 6, Dir: -1}},
		{"wheel-down:0,0", Step{Kind: KindWheel, Raw: "wheel-down:0,0", Dir: 1}},
		{"set:a=1", Step{Kind: KindSet, Raw: "set:a=1", Path: "a", Value: float64(1)}},
		{`set:a.b="x=y"`, Step{Kind: KindSet, Raw: `set:a.b="x=y"`, Path: "a.b", Value: "x=y"}}, // the first = splits
		{"set:a=null", Step{Kind: KindSet, Raw: "set:a=null", Path: "a"}},
		// SPEC v0.4d §15.4: append:PATH=JSON, split at the first =, as set:.
		{`append:log="x=y"`, Step{Kind: KindAppend, Raw: `append:log="x=y"`, Path: "log", Value: "x=y"}},
		{`append:m={"text":"hi"}`, Step{Kind: KindAppend, Raw: `append:m={"text":"hi"}`, Path: "m", Value: map[string]any{"text": "hi"}}},
		{"append:a.b=null", Step{Kind: KindAppend, Raw: "append:a.b=null", Path: "a.b"}},
		{"append:=1", Step{Kind: KindAppend, Raw: "append:=1", Value: float64(1)}}, // the root: judged when applied
		{`set:a=[1,{"b":true}]`, Step{Kind: KindSet, Raw: `set:a=[1,{"b":true}]`, Path: "a", Value: []any{float64(1), map[string]any{"b": true}}}},
		{"set:@theme=\"light\"", Step{Kind: KindSet, Raw: "set:@theme=\"light\"", Path: "@theme", Value: "light"}},
		{"set:=1", Step{Kind: KindSet, Raw: "set:=1", Value: float64(1)}}, // an empty path parses; Set refuses it when applied
	} {
		t.Run(c.tok, func(t *testing.T) {
			got, err := ParseStep(c.tok)
			if err != nil {
				t.Fatalf("ParseStep(%q): %v", c.tok, err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("ParseStep(%q) = %+v, want %+v", c.tok, got, c.want)
			}
		})
	}
}

func TestParseStepRejected(t *testing.T) {
	for _, c := range []struct{ tok, want string }{
		{"", "not a valid key token"},
		{"eggs", "not a valid key token"},
		{"Enter", "not a valid key token"}, // key names are lowercase
		{"ctrl+A", "not a valid key token"},
		{"ctrl+ab", "not a valid key token"},
		{"ctrl+1", "not a valid key token"},
		{",", "not a valid key token"}, // the keys= list separator binds nothing
		{"é", "not a valid key token"},
		{"wheel:1,2", "not a valid key token"}, // no such prefix
		{"Text:hi", "not a valid key token"},   // prefixes are case-sensitive
		{"Append:a=1", "not a valid key token"},

		{"append:", "append: wants PATH=JSON"},
		{"append:log", "append: wants PATH=JSON"},
		{"append:log=", "bad JSON value: unexpected end of JSON input"},
		{"append:log={", "bad JSON value: unexpected end of JSON input"},

		{"resize:", `bad size ""`},
		{"resize:80", `bad size "80" (want COLSxROWS)`},
		{"resize:80x", `bad size "80x": strconv.Atoi: parsing "": invalid syntax`},
		{"resize:x24", `bad size "x24": strconv.Atoi: parsing "": invalid syntax`},
		{"resize:80x24x", `bad size "80x24x"`},
		{"resize:axb", `bad size "axb"`},
		{"resize:0x24", "--cols and --rows must be between 1 and 1000 (got 0x24)"},
		{"resize:24x0", "--cols and --rows must be between 1 and 1000 (got 24x0)"},
		{"resize:-1x5", "--cols and --rows must be between 1 and 1000 (got -1x5)"},
		{"resize:1001x1", "--cols and --rows must be between 1 and 1000 (got 1001x1)"},
		{"resize:1x1001", "--cols and --rows must be between 1 and 1000 (got 1x1001)"},

		{"click:", "click: want X,Y"},
		{"click:1", "click: want X,Y"},
		{"click:,", `click: "" is not a decimal integer`},
		{"click:1,", `click: "" is not a decimal integer`},
		{"click:,2", `click: "" is not a decimal integer`},
		{"click:a,2", `click: "a" is not a decimal integer`},
		{"click:1,b", `click: "b" is not a decimal integer`},
		{"click:1,2,3", `click: "2,3" is not a decimal integer`},
		{"click:+1,2", `click: "+1" is not a decimal integer`},
		{"click:--1,2", `click: "--1" is not a decimal integer`},
		{"click:-,2", `click: "-" is not a decimal integer`},
		{"click:1.5,2", `click: "1.5" is not a decimal integer`},
		{"click:1234567890,0", `click: "1234567890" is not a decimal integer`}, // more than 9 digits
		{"wheel-up:", "wheel-up: want X,Y"},
		{"wheel-up:1;2", "wheel-up: want X,Y"},
		{"wheel-down:x,1", `wheel-down: "x" is not a decimal integer`},

		{"set:", "set: wants PATH=JSON"},
		{"set:a", "set: wants PATH=JSON"},
		{"set:a=", "bad JSON value: unexpected end of JSON input"},
		{"set:a={bad", "bad JSON value"},
		{"set:a=hello", "bad JSON value"}, // a string needs its quotes
		{"set:a=1 2", "bad JSON value"},
		{"set:a='x'", "bad JSON value"},
	} {
		t.Run(c.tok, func(t *testing.T) {
			st, err := ParseStep(c.tok)
			if err == nil {
				t.Fatalf("ParseStep(%q) = %+v, want an error containing %q", c.tok, st, c.want)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("ParseStep(%q): %q, want it to contain %q", c.tok, err, c.want)
			}
			if !reflect.DeepEqual(st, Step{}) {
				t.Errorf("ParseStep(%q) returned %+v with its error, want the zero Step", c.tok, st)
			}
		})
	}
}

func TestParseStepsSplitsOnSpaces(t *testing.T) {
	// Only ASCII spaces separate: a tab stays inside its token.
	tabbed, err := ParseSteps("  down   enter  text:hi\tthere ")
	if err != nil || len(tabbed) != 3 || tabbed[2].Text != "hi\tthere" {
		t.Errorf("a tab inside a token: %+v, %v", tabbed, err)
	}

	steps, err := ParseSteps("  down   enter  text:hi ")
	if err != nil {
		t.Fatal(err)
	}
	var raws []string
	for _, s := range steps {
		raws = append(raws, s.Raw)
	}
	if want := []string{"down", "enter", "text:hi"}; !reflect.DeepEqual(raws, want) {
		t.Errorf("steps = %q, want %q", raws, want)
	}
	for _, empty := range []string{"", "   "} {
		steps, err := ParseSteps(empty)
		if err != nil || len(steps) != 0 {
			t.Errorf("ParseSteps(%q) = %v, %v, want no steps", empty, steps, err)
		}
	}
}

// A failing --input token is reported as "step N (TOKEN): reason", counting
// from 1 over the tokens left after the split.
func TestParseStepsErrorNumbersTheStep(t *testing.T) {
	_, err := ParseSteps("down  bogus enter")
	if err == nil || err.Error() != "step 2 (bogus): not a valid key token" {
		t.Errorf("err = %v", err)
	}
	_, err = ParseSteps("down resize:0x1")
	if err == nil || err.Error() != "step 2 (resize:0x1): --cols and --rows must be between 1 and 1000 (got 0x1)" {
		t.Errorf("err = %v", err)
	}
}

// text:buy eggs splits at the space, and "eggs" is not a key. The error
// keeps its usual text and, only when the failing token follows a text: or
// paste: step, adds why (SPEC §15.4: --script carries embedded spaces).
func TestParseStepsHintsAtTextWithSpaces(t *testing.T) {
	const hint = "; --input splits steps at spaces, so text with a space needs --script FILE.ndjson"
	for _, c := range []struct{ in, want string }{
		{"text:buy eggs enter", "step 2 (eggs): not a valid key token" + hint},
		{"paste:buy eggs", "step 2 (eggs): not a valid key token" + hint},
		{"down text:a b-c", "step 3 (b-c): not a valid key token" + hint},
		// Not after text:/paste:, or not a key-token failure: no hint.
		{"down eggs", "step 2 (eggs): not a valid key token"},
		{"eggs text:buy", "step 1 (eggs): not a valid key token"},
		{"text:buy click:a,b", `step 2 (click:a,b): click: "a" is not a decimal integer`},
		{"text:buy resize:0x1", "step 2 (resize:0x1): --cols and --rows must be between 1 and 1000 (got 0x1)"},
	} {
		_, err := ParseSteps(c.in)
		if err == nil || err.Error() != c.want {
			t.Errorf("ParseSteps(%q) error = %v, want %q", c.in, err, c.want)
		}
	}
	// ParseStep itself (what Play and the MCP tool call per step) never adds it.
	if _, err := ParseStep("eggs"); err == nil || err.Error() != "not a valid key token" {
		t.Errorf("ParseStep(eggs) error = %v", err)
	}
}

func TestKeyFromToken(t *testing.T) {
	for _, c := range []struct {
		tok  string
		want host.Key
	}{
		{"space", host.Key{Name: "space", Rune: ' '}},
		{"enter", host.Key{Name: "enter"}},
		{"esc", host.Key{Name: "esc"}},
		{"tab", host.Key{Name: "tab"}},
		{"shift+tab", host.Key{Name: "shift+tab"}},
		{"backspace", host.Key{Name: "backspace"}},
		{"up", host.Key{Name: "up"}},
		{"pgdn", host.Key{Name: "pgdn"}},
		{"home", host.Key{Name: "home"}},
		{"ctrl+a", host.Key{Name: "ctrl+a"}},
		{"ctrl+c", host.Key{Name: "ctrl+c"}},
		{"ctrl+z", host.Key{Name: "ctrl+z"}},
		// ctrl+i/j/m arrive from a terminal as tab/enter (SPEC §8.1).
		{"ctrl+i", host.Key{Name: "tab"}},
		{"ctrl+j", host.Key{Name: "enter"}},
		{"ctrl+m", host.Key{Name: "enter"}},
		{"q", host.Key{Name: "q", Rune: 'q'}},
		{"Q", host.Key{Name: "Q", Rune: 'Q'}},
		{"/", host.Key{Name: "/", Rune: '/'}},
		{"7", host.Key{Name: "7", Rune: '7'}},
	} {
		if got := KeyFromToken(c.tok); got != c.want {
			t.Errorf("KeyFromToken(%q) = %+v, want %+v", c.tok, got, c.want)
		}
	}
}

// Every token ParseStep accepts as a key gets the key KeyFromToken gives.
func TestParseStepKeyUsesKeyFromToken(t *testing.T) {
	for _, tok := range []string{"space", "ctrl+i", "ctrl+x", "x", "pgup"} {
		st, err := ParseStep(tok)
		if err != nil || st.Kind != KindKey || st.Key != KeyFromToken(tok) {
			t.Errorf("ParseStep(%q) = %+v, %v", tok, st, err)
		}
	}
}

// --- Session -------------------------------------------------------------

const sessionDoc = `<tui version="1">
  <screen id="s" focus="#q">
    <col>
      <input id="q" on:submit="go"/>
      <text>{n}</text>
    </col>
  </screen>
</tui>`

func newSessionApp(t *testing.T) *host.App {
	t.Helper()
	app, err := host.Parse(strings.NewReader(sessionDoc))
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Set("n", float64(0)); err != nil {
		t.Fatal(err)
	}
	return app
}

func runSteps(t *testing.T, app *host.App, cols, rows int, in string) *Session {
	t.Helper()
	steps, err := ParseSteps(in)
	if err != nil {
		t.Fatal(err)
	}
	sess := NewSession(app, cols, rows, false, false, true, true)
	sess.Run(steps)
	return sess
}

// Typing into the focused input and pressing enter fires on:submit with
// the typed value, at the step that pressed enter; --frames-style
// snapshots hold one frame for step 0 and one per applied step.
func TestSessionTypesAndSubmits(t *testing.T) {
	sess := runSteps(t, newSessionApp(t), 20, 4, "text:hi enter")
	if sess.UsageErr != nil || sess.Quit || sess.Err != nil {
		t.Fatalf("usage %v quit %v err %v", sess.UsageErr, sess.Quit, sess.Err)
	}
	if len(sess.Events) != 1 {
		t.Fatalf("events = %+v, want one", sess.Events)
	}
	if ev := sess.Events[0]; ev.Step != 2 || ev.Action != "go" || ev.Source != "q" || ev.Value != "hi" {
		t.Errorf("event = %+v, want step 2, action go from #q with value hi", ev)
	}
	if len(sess.StepFrames) != 3 {
		t.Fatalf("%d frames, want 3 (step 0 and two steps)", len(sess.StepFrames))
	}
	for i, in := range []string{"", "text:hi", "enter"} {
		if f := sess.StepFrames[i]; f.Step != i || f.Input != in {
			t.Errorf("frame %d = step %d input %q, want input %q", i, f.Step, f.Input, in)
		}
	}
}

// A click or wheel outside the current grid is a usage error that stops
// the session at that step; a resize changes the grid later cells are
// judged against.
func TestSessionMouseCellMustBeInsideTheGrid(t *testing.T) {
	sess := runSteps(t, newSessionApp(t), 20, 4, "resize:10x2 click:15,1 text:x")
	if sess.UsageErr == nil || sess.UsageStep != 2 || sess.UsageRaw != "click:15,1" ||
		sess.UsageErr.Error() != "cell 15,1 is outside the 10x2 grid" {
		t.Errorf("usage error = %v at step %d (%q)", sess.UsageErr, sess.UsageStep, sess.UsageRaw)
	}
	if len(sess.StepFrames) != 2 { // step 0 and the resize; the failed step has no frame
		t.Errorf("%d frames, want 2", len(sess.StepFrames))
	}
	for _, in := range []string{"click:20,0", "wheel-down:0,4", "wheel-up:-1,0"} {
		if sess := runSteps(t, newSessionApp(t), 20, 4, in); sess.UsageErr == nil || sess.UsageStep != 1 {
			t.Errorf("%s: usage error = %v, want one at step 1", in, sess.UsageErr)
		}
	}
	if sess := runSteps(t, newSessionApp(t), 20, 4, "click:19,3 wheel-down:0,0"); sess.UsageErr != nil {
		t.Errorf("cells inside the grid: %v", sess.UsageErr)
	}
}

// set: writes the store before the next step; a path the store cannot take
// ends the session as a usage error naming the step.
func TestSessionSetAndFocus(t *testing.T) {
	sess := runSteps(t, newSessionApp(t), 20, 4, "set:n=7 focus:#q")
	if sess.UsageErr != nil {
		t.Fatalf("usage error: %v", sess.UsageErr)
	}
	if got, _ := newSessionApp(t).Get("n"); got != float64(0) {
		t.Errorf("a fresh app's n = %v", got)
	}
	if !strings.Contains(strings.Join(sess.BuildDump().Grid, "\n"), "7") {
		t.Errorf("set:n=7 is not in the final frame:\n%s", strings.Join(sess.BuildDump().Grid, "\n"))
	}

	sess = runSteps(t, newSessionApp(t), 20, 4, "set:n.x.y=1 down")
	if sess.UsageErr == nil || sess.UsageStep != 1 || sess.UsageRaw != "set:n.x.y=1" {
		t.Errorf("set through a number: usage %v at step %d (%q)", sess.UsageErr, sess.UsageStep, sess.UsageRaw)
	}
	// An id that names no focusable node is not an error: the request is
	// simply not honored.
	sess = runSteps(t, newSessionApp(t), 20, 4, "focus:nope")
	if sess.UsageErr != nil {
		t.Errorf("focus on an unknown id: usage error %v", sess.UsageErr)
	}
}

// The built-in quit ends the session and later steps do not run.
func TestSessionQuitStopsReplay(t *testing.T) {
	sess := runSteps(t, newSessionApp(t), 20, 4, "ctrl+c text:x enter")
	if !sess.Quit {
		t.Fatalf("ctrl+c did not quit: %+v", sess.Events)
	}
	for _, ev := range sess.Events {
		if ev.Action == "go" {
			t.Errorf("a step after quit ran: %+v", ev)
		}
	}
}
