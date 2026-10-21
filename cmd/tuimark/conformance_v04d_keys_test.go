package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// SPEC v0.4d §21 tests of phase D2 (the keys): 274's key parts, 276 to 282,
// and 312's input parts, through the CLI. The fixtures are
// testdata/v04d/t<NNN>; the Go API and the pty are in the root package and
// internal/host.

const (
	d2K277  = "../../testdata/v04d/t277/k277.tui"
	d2Rows  = "../../testdata/v04d/t277/sample.json"
	d2K1    = "../../testdata/v04d/t278/k1.tui"
	d2AB    = "../../testdata/v04d/t278/ab.json"
	d2Raise = "\x1b"
)

// d2Script writes the --script file of one {"text": ...} step per element
// and returns its path.
func d2Script(t *testing.T, texts ...string) string {
	t.Helper()
	var b strings.Builder
	for _, s := range texts {
		line, err := json.Marshal(map[string]string{"text": s})
		if err != nil {
			t.Fatal(err)
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	path := filepath.Join(t.TempDir(), "steps.ndjson")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// d2Event is one event of play's JSON output.
type d2Event struct {
	Step   int    `json:"step"`
	Action string `json:"action"`
	Source string `json:"source"`
	Value  any    `json:"value"`
}

// d2Play runs play with args and returns its events.
func d2Play(t *testing.T, args ...string) []d2Event {
	t.Helper()
	code, out, errw := runCLI(append([]string{"play"}, append(args, "--format", "json")...)...)
	if code != 0 {
		t.Fatalf("play %v: exit %d %s%s", args, code, out, errw)
	}
	var res struct {
		Events []d2Event `json:"events"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("play %v: %v\n%s", args, err, out)
	}
	return res.Events
}

// d2Actions lists the actions of events as "step:action".
func d2Actions(evs []d2Event) []string {
	out := []string{}
	for _, e := range evs {
		out = append(out, fmt.Sprintf("%d:%s", e.Step, e.Action))
	}
	return out
}

// SPEC v0.4d test 277: the decoder table. The document binds every token to
// an action named after it, and a focused empty list consumes no key.
func TestD2Decoder277(t *testing.T) {
	csi := "\x1b["
	rows := []struct {
		bytes  []string
		action string
	}{
		{[]string{csi + "3~"}, "delete"},
		{[]string{"\x1bOP", csi + "P", csi + "11~"}, "f1"},
		{[]string{csi + "13~", csi + "1;2R"}, "f3"},
		{[]string{csi + "24~", csi + "24;5~"}, "f12"},
		{[]string{csi + "13;2u", csi + "27;2;13~"}, "shift_enter"},
		{[]string{csi + "13;5u"}, "ctrl_enter"},
		{[]string{csi + "13;3u", "\x1b\r", "\x1b\n", csi + "109;7u", csi + "106;7u"}, "alt_enter"},
		{[]string{csi + "127;3u", "\x1b\x7f"}, "alt_backspace"},
		{[]string{csi + "97;3u", csi + "27;3;97~", "\x1ba"}, "alt_a"},
		{[]string{csi + "97;4u"}, "alt_A"},
		{[]string{csi + "97;7u"}, "ctrl_a"},
		{[]string{csi + "97;69u"}, "ctrl_a"},
		{[]string{csi + "97;6u", csi + "27;6;65~", csi + "97;8u", csi + "27;5;65~"}, "ctrl_a"},
		{[]string{csi + "127;5u"}, "backspace"},
		{[]string{csi + "105;5u", csi + "105;6u", csi + "27;6;73~", csi + "105;7u"}, "tab"},
		{[]string{csi + "109;5u", csi + "109;6u", csi + "27;5;77~", csi + "106;6u"}, "enter"},
		{[]string{csi + "27u", csi + "91;5u", csi + "91;6u", csi + "27;6;123~"}, "esc"},
		{[]string{csi + "9;2u", csi + "Z"}, "shift_tab"},
		{[]string{csi + "1;5A"}, "up"},
		{[]string{csi + "57414u"}, "enter"},
		{[]string{csi + "57414;2u"}, "shift_enter"},
		{[]string{csi + "57419u"}, "up"},
		{[]string{csi + "1;3D", csi + "1;5D", csi + "57417;3u"}, "left"},
	}
	var steps []string
	var want []string
	for _, r := range rows {
		for _, b := range r.bytes {
			steps = append(steps, b)
			want = append(want, fmt.Sprintf("%d:%s", len(steps), r.action))
		}
	}
	got := d2Actions(d2Play(t, d2K277, "--data", d2Rows, "--script", d2Script(t, steps...)))
	if !reflect.DeepEqual(got, want) {
		t.Errorf("events\n got %v\nwant %v", got, want)
	}

	// Nothing fires for these, even with rows for the characters.
	nothing := []string{
		csi + "2~", csi + "57425u", csi + "6;3R",
		csi + "32;5u", csi + "47;5u", csi + "27;5;47~", csi + "51;5u", csi + "56;7u", csi + "50;6u", csi + "57400;5u",
	}
	if got := d2Actions(d2Play(t, d2K277, "--data", d2Rows, "--script", d2Script(t, nothing...))); len(got) != 0 {
		t.Errorf("nothing was expected, got %v", got)
	}

	// CSI 99 ; 5 u with no ctrl+c row records quit.
	evs := d2Play(t, d2K277, "--data", d2Rows, "--script", d2Script(t, csi+"99;5u"))
	if got := d2Actions(evs); !reflect.DeepEqual(got, []string{"1:quit"}) {
		t.Errorf("ctrl+c: %v", got)
	}
}

// SPEC v0.4d test 278: ESC and a letter. The 0.4.4 column is the same
// document with version="3" written, which keeps the 0.4.4 decoder.
func TestD2EscAndALetter278(t *testing.T) {
	pinned := writeTemp(t, "k1-v3.tui", strings.Replace(readFile(t, d2K1), "<tui>", `<tui version="3">`, 1))
	cases := []struct {
		text   string
		cur    []string // 0.4d, k1.tui as written: "action=value"
		pinned []string // 0.4.4, and 0.4d with version="3" written
	}{
		{"\x1bx", nil, []string{`e="ab"`, `ch="abx"`}},
		{"\x1b\r", nil, []string{`e="ab"`, `sub="ab"`}},
		{"\x1b\x7f", []string{`ch=""`}, []string{`e="ab"`, `ch="a"`}},
		{"\x1b", []string{`e="ab"`}, []string{`e="ab"`}},
	}
	for _, c := range cases {
		for _, doc := range []struct {
			file string
			want []string
		}{{d2K1, c.cur}, {pinned, c.pinned}} {
			evs := d2Play(t, doc.file, "--data", d2AB, "--script", d2Script(t, c.text))
			got := []string{}
			for _, e := range evs {
				v, _ := json.Marshal(e.Value)
				got = append(got, fmt.Sprintf("%s=%s", e.Action, v))
			}
			want := doc.want
			if want == nil {
				want = []string{}
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%s %q: %v, want %v", filepath.Base(doc.file), c.text, got, want)
			}
		}
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// SPEC v0.4d test 276: the key tokens of the current language.
func TestD2KeyTokens276(t *testing.T) {
	doc := func(keys string) string {
		return writeTemp(t, "k.tui", `<tui><screen id="s"><text>x</text></screen><keymap><bind keys="`+keys+`" action="a"/></keymap></tui>`)
	}
	code, out, errw := runCLI("validate", doc("delete,f1,f12,alt+a,alt+Z,alt+0,alt+enter,alt+backspace,shift+enter,ctrl+enter"))
	if code != 0 || out != "ok (bindings not checked: no --data)\n" {
		t.Errorf("valid tokens: exit %d %q %s", code, out, errw)
	}
	const list = "a-z 0-9 enter esc tab backspace space delete arrows home end pgup pgdn f1-f12 ctrl+x alt+x shift+tab shift+enter ctrl+enter alt+enter alt+backspace"
	for _, bad := range []string{"f13", "f0", "alt+!", "alt+space", "shift+a", "ctrl+delete", "alt+f1"} {
		code, out, _ := runCLI("validate", doc(bad))
		want := fmt.Sprintf("unknown key %q (see the key token list: %s)", bad, list)
		if code != 2 || !strings.Contains(out, "V003") || !strings.Contains(out, want) {
			t.Errorf("%s: exit %d %q, want %q", bad, code, out, want)
		}
	}
	// ctrl+i, ctrl+j, and ctrl+m keep their 0.4b messages.
	for tok, as := range map[string]string{"ctrl+i": "tab", "ctrl+j": "enter", "ctrl+m": "enter"} {
		code, out, _ := runCLI("validate", doc(tok))
		want := fmt.Sprintf("key %q never reaches a keymap: it arrives as %s (write %s)", tok, as, as)
		if code != 2 || !strings.Contains(out, want) {
			t.Errorf("%s: exit %d %q, want %q", tok, code, out, want)
		}
	}
}

// SPEC v0.4d test 274, the key parts: the messages of a pinned document.
func TestD2PinnedKeyMessages274(t *testing.T) {
	src := "<tui version=\"3\">\n  <screen id=\"main\"><text>x</text></screen>\n  <keymap><bind keys=\"delete,alt+x,shift+enter\" action=\"zz\"/></keymap>\n</tui>\n"
	path := writeTemp(t, "pin.tui", src)
	code, out, _ := runCLI("validate", path)
	var want []string
	for _, k := range []string{"delete", "alt+x", "shift+enter"} {
		want = append(want, fmt.Sprintf("V003 pin.tui:3:11: key %q (requires a <tui> without version)", k))
	}
	if code != 2 {
		t.Errorf("exit %d", code)
	}
	for _, w := range want {
		if !strings.Contains(out, strings.Replace(w, "V003 pin.tui:3:11: ", "", 1)) {
			t.Errorf("validate output lacks %q:\n%s", w, out)
		}
	}
	// An unknown key still lists 0.4.4's tokens there.
	path = writeTemp(t, "pin2.tui", strings.Replace(src, "delete,alt+x,shift+enter", "alt+!", 1))
	_, out, _ = runCLI("validate", path)
	if !strings.Contains(out, `unknown key "alt+!" (see the key token list: a-z 0-9 enter esc tab backspace space arrows home end pgup pgdn ctrl+x shift+tab)`) {
		t.Errorf("pinned unknown key:\n%s", out)
	}
	// JSON gives the position as test 274 states: line 3, column 11.
	_, out, _ = runCLI("validate", writeTemp(t, "pin.tui", src), "--json")
	var res struct {
		Diagnostics []struct {
			Code string `json:"code"`
			Line int    `json:"line"`
			Col  int    `json:"col"`
			Msg  string `json:"msg"`
		} `json:"diagnostics"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Diagnostics) != 3 {
		t.Fatalf("diagnostics: %s", out)
	}
	for i, k := range []string{"delete", "alt+x", "shift+enter"} {
		d := res.Diagnostics[i]
		if d.Code != "V003" || d.Line != 3 || d.Col != 11 || d.Msg != fmt.Sprintf("key %q (requires a <tui> without version)", k) {
			t.Errorf("diagnostic %d = %+v", i, d)
		}
	}
}

// SPEC v0.4d test 279: --keyboard legacy.
func TestD2KeyboardLegacy279(t *testing.T) {
	code, out, errw := runCLI("play", d2K1, "--data", d2AB, "--input", "shift+enter", "--keyboard", "legacy")
	if code != 1 || out != "" || errw != "tuimark: play: step 1 (shift+enter): key \"shift+enter\" cannot be sent by a legacy terminal (--keyboard legacy)\n" {
		t.Errorf("legacy shift+enter: exit %d %q %q", code, out, errw)
	}
	for _, args := range [][]string{{"--input", "shift+enter"}, {"--input", "shift+enter", "--keyboard", "kitty"}} {
		if code, out, errw := runCLI(append([]string{"play", d2K1, "--data", d2AB}, args...)...); code != 0 {
			t.Errorf("%v: exit %d %q %q", args, code, out, errw)
		}
	}
	// The same events where both can send the key.
	script := []string{"alt+a", "delete", "f1", "esc", "ctrl+a", "enter", "alt+enter", "alt+backspace"}
	steps := strings.Join(script, " ")
	kitty := d2Play(t, d2K277, "--data", d2Rows, "--input", steps)
	legacy := d2Play(t, d2K277, "--data", d2Rows, "--input", steps, "--keyboard", "legacy")
	if !reflect.DeepEqual(kitty, legacy) || len(kitty) != len(script) {
		t.Errorf("kitty %v, legacy %v", d2Actions(kitty), d2Actions(legacy))
	}
	// --keyboard other is a usage error.
	if code, _, errw := runCLI("play", d2K1, "--keyboard", "other"); code != 1 || !strings.Contains(errw, "--keyboard must be kitty or legacy") {
		t.Errorf("--keyboard other: exit %d %q", code, errw)
	}
	// A pinned document: a key step naming a 0.4d token is a usage error,
	// and --keyboard is ignored.
	pinned := writeTemp(t, "p.tui", `<tui version="3"><screen id="s"><text>x</text></screen></tui>`)
	code, out, errw = runCLI("play", pinned, "--input", "delete")
	if code != 1 || out != "" || errw != "tuimark: play: step 1 (delete): key \"delete\" (requires a <tui> without version)\n" {
		t.Errorf("pinned delete: exit %d %q %q", code, out, errw)
	}
	if code, _, errw := runCLI("play", pinned, "--input", "enter", "--keyboard", "legacy"); code != 0 {
		t.Errorf("pinned with --keyboard legacy: exit %d %q", code, errw)
	}
}

// SPEC v0.4d test 280: hints and producible keys.
func TestD2Hints280(t *testing.T) {
	src := func(scope string) string {
		return `<tui>
  <screen id="main"><hints` + scope + `/></screen>
  <keymap>
    <bind keys="shift+enter,alt+enter" action="nl" label="newline"/>
    <bind keys="ctrl+enter" action="go" label="send"/>
  </keymap>
</tui>`
	}
	hintsRow := func(args ...string) string {
		code, out, errw := runCLI(args...)
		if code != 0 {
			t.Fatalf("%v: exit %d %s", args, code, errw)
		}
		return out
	}
	active := writeTemp(t, "h.tui", src(""))
	all := writeTemp(t, "ha.tui", src(` scope="all"`))
	if out := hintsRow("dump", active); !strings.Contains(out, "shift+enter newline  ctrl+enter send") {
		t.Errorf("dump: %s", out)
	}
	if out := hintsRow("play", active, "--keyboard", "legacy"); !strings.Contains(out, "alt+enter newline") || strings.Contains(out, "send") {
		t.Errorf("play legacy: %s", out)
	}
	if out := hintsRow("play", all, "--keyboard", "legacy"); !strings.Contains(out, "alt+enter newline  ctrl+enter send") {
		t.Errorf("play legacy scope=all: %s", out)
	}
	if out := hintsRow("dump", all); !strings.Contains(out, "shift+enter newline  ctrl+enter send") {
		t.Errorf("dump scope=all: %s", out)
	}
	cap := writeTemp(t, "hc.tui", strings.Replace(src(""), `<bind keys="shift+enter,alt+enter"`, `<bind keycap="⇧↵" keys="shift+enter,alt+enter"`, 1))
	for _, args := range [][]string{{"dump", cap}, {"play", cap, "--keyboard", "legacy"}} {
		if out := hintsRow(args...); !strings.Contains(out, "⇧↵ newline") {
			t.Errorf("%v: %s", args, out)
		}
	}
}

// SPEC v0.4d test 281: input's new keys, and no ctrl+w or ctrl+k.
func TestD2InputKeys281(t *testing.T) {
	doc := writeTemp(t, "i.tui", `<tui><screen id="main"><input id="q" bind="q" on:change="ch"/></screen><keymap><bind keys="ctrl+w" action="cw"/><bind keys="ctrl+k" action="ck"/></keymap></tui>`)
	data := writeTemp(t, "d.json", `{"q": "foo bar"}`)
	vals := func(steps string, dataPath string) []string {
		out := []string{}
		for _, e := range d2Play(t, doc, "--data", dataPath, "--input", steps) {
			v, _ := json.Marshal(e.Value)
			out = append(out, e.Action+"="+string(v))
		}
		return out
	}
	// alt+b twice, alt+f, delete, then alt+backspace.
	if got, want := vals("alt+b alt+b alt+f delete alt+backspace", data), []string{`ch="foobar"`, `ch="bar"`}; !reflect.DeepEqual(got, want) {
		t.Errorf("word keys: %v, want %v", got, want)
	}
	if got, want := vals("alt+backspace", data), []string{`ch="foo "`}; !reflect.DeepEqual(got, want) {
		t.Errorf("alt+backspace at the end: %v, want %v", got, want)
	}
	if got, want := vals("alt+backspace", writeTemp(t, "e.json", `{"q": "foobar"}`)), []string{`ch=""`}; !reflect.DeepEqual(got, want) {
		t.Errorf("alt+backspace on one word: %v, want %v", got, want)
	}
	// ctrl+w and ctrl+k rows fire with the input focused (test 278's k1.tui
	// records cw with value ab).
	evs := d2Play(t, d2K1, "--data", d2AB, "--input", "ctrl+w")
	if len(evs) != 1 || evs[0].Action != "cw" || evs[0].Value != "ab" {
		t.Errorf("ctrl+w: %+v", evs)
	}
	if got := vals("ctrl+k", data); !reflect.DeepEqual(got, []string{`ck="foo bar"`}) {
		t.Errorf("ctrl+k: %v", got)
	}
}

// SPEC v0.4d test 282: an f12 row fires in play.
func TestD2F12Row282(t *testing.T) {
	doc := writeTemp(t, "f.tui", `<tui><screen id="main"><text>x</text></screen><keymap><bind keys="f12" action="dbg"/><bind keys="f2" action="two"/></keymap></tui>`)
	got := d2Actions(d2Play(t, doc, "--input", "f12 f2"))
	if !reflect.DeepEqual(got, []string{"1:dbg", "2:two"}) {
		t.Errorf("events %v", got)
	}
}

// SPEC v0.4d test 312, the input parts: word moves on left and right.
func TestD2WordMoves312(t *testing.T) {
	csi := "\x1b["
	data := writeTemp(t, "d.json", `{"q": "foo bar"}`)
	last := func(file string, steps ...string) string {
		evs := d2Play(t, file, "--data", data, "--script", d2Script(t, append(steps, "X")...))
		if len(evs) == 0 {
			return ""
		}
		for _, e := range evs {
			if e.Action != "ch" {
				t.Errorf("%q fired %s", steps, e.Action)
			}
		}
		v, _ := evs[len(evs)-1].Value.(string)
		return v
	}
	cases := []struct {
		steps []string
		want  string
	}{
		{[]string{csi + "1;3D"}, "foo Xbar"},
		{[]string{csi + "1;5D"}, "foo Xbar"},
		{[]string{csi + "1;6D"}, "foo Xbar"},
		{[]string{csi + "57417;3u"}, "foo Xbar"},
		{[]string{csi + "D"}, "foo baXr"},
		{[]string{csi + "32;5u"}, "foo barX"},
		{[]string{csi + "51;5u"}, "foo barX"},
		{[]string{csi + "27;5;47~"}, "foo barX"},
		{[]string{csi + "H", csi + "1;5C"}, "fooX bar"},
	}
	for _, c := range cases {
		if got := last(d2K1, c.steps...); got != c.want {
			t.Errorf("%q then X: %q, want %q", c.steps, got, c.want)
		}
	}
	// Under --keyboard legacy the steps give the same results: they are bytes.
	for _, c := range cases[:3] {
		evs := d2Play(t, d2K1, "--data", data, "--keyboard", "legacy", "--script", d2Script(t, c.steps[0], "X"))
		if v, _ := evs[len(evs)-1].Value.(string); v != c.want {
			t.Errorf("legacy %q: %q", c.steps, v)
		}
	}
	// With version="3" written, ctrl+left moves one cluster, as 0.4.5 does.
	pinned := writeTemp(t, "k1-v3.tui", strings.Replace(readFile(t, d2K1), "<tui>", `<tui version="3">`, 1))
	if got := last(pinned, csi+"1;5D"); got != "foo baXr" {
		t.Errorf("version=3 ctrl+left: %q, want %q", got, "foo baXr")
	}
	// With the input not focused and rows for left and right, the same
	// bytes fire left and right (test 277's k277.tui).
	evs := d2Play(t, d2K277, "--data", d2Rows, "--script", d2Script(t, csi+"1;3D", csi+"1;5C"))
	if got := d2Actions(evs); !reflect.DeepEqual(got, []string{"1:left", "2:right"}) {
		t.Errorf("rows: %v", got)
	}
}
