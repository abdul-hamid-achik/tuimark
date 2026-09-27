package host

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
)

// SPEC v0.2b §21 tests 41 and 43 (Go side): theme="auto" resolved by the
// probe's OSC 11 reply, the COLORFGBG fallback, @media (theme), and the
// reserved Set path @theme.

// 41. The lightness rule of §26.2, exactly as the vectors give it: dark
// when max + min < 1, components scaled by 16^d − 1, the alpha ignored,
// either hex case, 1 to 4 digits.
func TestThemeFromOSC11(t *testing.T) {
	for spec, want := range map[string]string{
		"rgb:ffff/ffff/ffff":       "light",
		"rgb:8080/8080/8080":       "light",
		"rgb:f/f/f":                "light",
		"rgb:fbfb/fafa/f8f8":       "light",
		"rgb:FBFB/FAFA/F8F8":       "light",
		"rgb:0000/0000/0000":       "dark",
		"rgb:7f7f/7f7f/7f7f":       "dark",
		"rgb:1e1e/1e1e/2e2e":       "dark",
		"rgba:0000/0000/0000/ffff": "dark",
		"rgb:80/80/80":             "light", // 128/255: lightness 0.502
		"rgb:7f/7f/7f":             "dark",
		"rgb:ff/0/0":               "light", // pure red: lightness exactly 0.5 is light
		"rgb:0/0/0":                "dark",
		"rgb:fffff/0/0":            "",
		"rgb:ff/ff":                "",
		"rgba:0/0/0":               "",
		"rgb:gg/00/00":             "",
		"rgb:/00/00":               "",
		"#ffffff":                  "",
		"":                         "",
	} {
		if got := ThemeFromOSC11(spec); got != want {
			t.Errorf("ThemeFromOSC11(%q) = %q, want %q", spec, got, want)
		}
	}
}

// 41. COLORFGBG: the last ;-separated field, 0-15; 0-6 and 8 dark, 7 and
// 9-15 light; anything else gives no theme.
func TestThemeFromCOLORFGBG(t *testing.T) {
	for v, want := range map[string]string{
		"15;0": "dark", "0;15": "light", "0;default;7": "light", "12;8": "dark",
		"x": "", "": "", "0;16": "", "0;-1": "", "0;": "", "7;default": "", "0;007": "",
		"15;6": "dark", "0;9": "light",
	} {
		if got := ThemeFromCOLORFGBG(v); got != want {
			t.Errorf("ThemeFromCOLORFGBG(%q) = %q, want %q", v, got, want)
		}
	}
}

// 41. The decoder turns an OSC 11 reply ended by BEL or by ST into a probe
// reply, also when split across reads, and never into a key; any other
// OSC stays a reply-less, key-less sequence.
func TestDecoderOSC11Reply(t *testing.T) {
	for _, in := range []string{"\x1b]11;rgb:ffff/ffff/ffff\a", "\x1b]11;rgb:ffff/ffff/ffff\x1b\\"} {
		for cut := 1; cut < len(in); cut++ {
			var d decoder
			ins1, reps1 := d.feed([]byte(in[:cut]), false)
			ins2, reps2 := d.feed([]byte(in[cut:]+"x"), false)
			reps := append(reps1, reps2...)
			if len(reps) != 1 || !reps[0].osc11 || reps[0].bg != "light" {
				t.Fatalf("%q cut at %d: replies %+v", in, cut, reps)
			}
			keys := append(ins1, ins2...)
			if len(keys) != 1 || keys[0].Key.Name != "x" {
				t.Fatalf("%q cut at %d: inputs %+v", in, cut, keys)
			}
		}
	}
	var d decoder
	ins, reps := d.feed([]byte("\x1b]10;rgb:0/0/0\a\x1b]11;?\a\x1b]11;bogus\x1b\\"), true)
	if len(ins) != 0 {
		t.Errorf("keys from OSC replies: %+v", ins)
	}
	if len(reps) != 2 || reps[0].bg != "" || reps[1].bg != "" {
		t.Errorf("replies %+v: an OSC 11 with another SPEC records nothing", reps)
	}
}

// 41. Only the first OSC 11 reply with an rgb:/rgba: SPEC records the
// background, and nothing counts after the DA1 sentinel.
func TestCapsRecordBackground(t *testing.T) {
	var c termCaps
	c.record(reply{osc11: true})
	c.record(reply{osc11: true, bg: "light"})
	c.record(reply{osc11: true, bg: "dark"})
	if c.bg != "light" {
		t.Errorf("bg = %q, want the first valid reply", c.bg)
	}
	c.record(reply{da1: true})
	var d termCaps
	d.record(reply{da1: true})
	d.record(reply{osc11: true, bg: "light"})
	if d.bg != "" {
		t.Errorf("a reply after DA1 counted: %q", d.bg)
	}
}

// 41. The probe plan: OSC 11 goes first when the theme is resolved from
// the terminal; under the Apple Terminal/SSH skip rule such a probe sends
// OSC 11 and DA1 only, and without theme="auto" nothing.
func TestProbePlanWithTheme(t *testing.T) {
	apple := envMap(map[string]string{"TERM": "xterm-256color", "TERM_PROGRAM": "Apple_Terminal"})
	plain := envMap(map[string]string{"TERM": "xterm-256color"})
	dumb := envMap(map[string]string{"TERM": "dumb"})
	for _, c := range []struct {
		name       string
		env        func(string) string
		tty, theme bool
		probe      bool
		queries    string
	}{
		{"auto", plain, true, true, true, queryBackground + "\x1b[?2026$p\x1b[?2027$p\x1b[c"},
		{"no auto", plain, true, false, true, "\x1b[?2026$p\x1b[?2027$p\x1b[c"},
		{"skip rule, auto", apple, true, true, true, queryBackground + "\x1b[c"},
		{"skip rule, no auto", apple, true, false, false, ""},
		{"dumb", dumb, true, true, false, ""},
		{"not a tty", plain, false, true, false, ""},
	} {
		probe, skip := probePlan(c.env, c.tty, syncAuto, c.theme)
		if probe != c.probe {
			t.Errorf("%s: probe = %v", c.name, probe)
			continue
		}
		if !probe {
			continue
		}
		s := &termSession{probe: probe, skipDECRQM: skip, themeQuery: c.theme}
		if q := s.queries(); q != c.queries {
			t.Errorf("%s: queries %q, want %q", c.name, q, c.queries)
		}
	}
	if !strings.HasPrefix(queryBackground, "\x1b]11;?\a") {
		t.Errorf("OSC 11 query %q", queryBackground)
	}
}

// 41. The probe waits for OSC 11 only when the theme is resolved from the
// terminal: TUIMARK_THEME set, @theme set to dark or light, or a document
// theme other than auto leave it out.
func TestWantsTerminalTheme(t *testing.T) {
	auto := doc(t, autoDoc)
	if !auto.wantsTerminalTheme("") {
		t.Error("theme=auto without TUIMARK_THEME")
	}
	if auto.wantsTerminalTheme("light") {
		t.Error("TUIMARK_THEME=light still queries the background")
	}
	_ = auto.Set("@theme", "dark")
	if auto.wantsTerminalTheme("") {
		t.Error("@theme dark still queries the background")
	}
	dark := doc(t, strings.Replace(autoDoc, `theme="auto"`, `theme="dark"`, 1))
	if dark.wantsTerminalTheme("") {
		t.Error("theme=dark queries the background")
	}
	_ = dark.Set("@theme", "auto")
	if !dark.wantsTerminalTheme("") {
		t.Error("@theme auto set before Run does not query the background")
	}
}

const autoDoc = `<tui version="2" theme="auto">
<style>screen { background: $bg; color: $fg; }</style>
<screen id="s" focus="#q">
  <input id="q" bind="text" on:change="chg" on:focus="hello"/>
  <input id="pw" bind="pw" secret="true" on:change="chg"/>
  <text>AUTO</text>
</screen>
</tui>`

const (
	darkBG  = "48;2;13;17;23"
	lightBG = "48;2;255;255;255"
)

// 41. With a scripted terminal: the probe's write starts with OSC 11; the
// first frame waits for the replies and is drawn in the theme the reply
// gives; a reply terminated by ST works as one ended by BEL.
func TestProbeResolvesAutoTheme(t *testing.T) {
	for _, c := range []struct {
		reply, want, dont string
	}{
		{"\x1b]11;rgb:ffff/ffff/ffff\a", lightBG, darkBG},
		{"\x1b]11;rgb:fbfb/fafa/f8f8\x1b\\", lightBG, darkBG},
		{"\x1b]11;rgb:0000/0000/0000\x1b\\", darkBG, lightBG},
		{"\x1b]11;rgba:1e1e/1e1e/2e2e/ffff\a", darkBG, lightBG},
	} {
		r := startSession(t, autoDoc, &termSession{probe: true, themeQuery: true, wait: 10 * time.Second})
		want := enterScreen + queryBackground + "\x1b[?2026$p\x1b[?2027$p\x1b[c"
		r.waitOut(t, want)
		time.Sleep(20 * time.Millisecond)
		if got := r.out.String(); got != want {
			t.Fatalf("before the reply the session wrote %q, want %q", got, want)
		}
		r.send(t, c.reply+"\x1b[?64c")
		if ev := r.next(t); ev.Action != "hello" {
			t.Fatalf("first event %+v", ev)
		}
		r.send(t, "\x03")
		if err := r.wait(t); err != nil {
			t.Fatal(err)
		}
		out := r.out.String()
		if !strings.Contains(out, c.want) || strings.Contains(out, c.dont) {
			t.Errorf("reply %q: frames in the wrong theme", c.reply)
		}
		if r.app.runAuto != "" {
			t.Errorf("the resolved theme outlived the session: %q", r.app.runAuto)
		}
	}
}

// 41. No reply within the cap: COLORFGBG decides, then dark; the first
// frame comes at the cap, never before it.
func TestProbeAutoThemeFallbacks(t *testing.T) {
	for _, c := range []struct {
		fgbg, want string
	}{{"light", lightBG}, {"dark", darkBG}, {"", darkBG}} {
		start := time.Now()
		r := startSession(t, autoDoc, &termSession{probe: true, themeQuery: true, fgbg: c.fgbg, wait: 150 * time.Millisecond})
		r.waitOut(t, "AUTO")
		if d := time.Since(start); d < 150*time.Millisecond {
			t.Errorf("first frame after %v, before the probe's cap", d)
		}
		r.next(t)
		// A late OSC 11 reply changes nothing.
		r.send(t, "\x1b]11;rgb:ffff/ffff/ffff\a\x1b[?62c")
		r.send(t, "\x03")
		if err := r.wait(t); err != nil {
			t.Fatal(err)
		}
		out := r.out.String()
		if !strings.Contains(out, c.want) {
			t.Errorf("COLORFGBG %q: no %q in the frames", c.fgbg, c.want)
		}
		if c.want == darkBG && strings.Contains(out, lightBG) {
			t.Errorf("COLORFGBG %q: a late reply changed the theme", c.fgbg)
		}
	}
}

// 43. In Run, TUIMARK_THEME beats @theme, which beats the probe; a later
// Set("@theme", "auto") takes the recorded probe result again.
func TestRunThemePrecedence(t *testing.T) {
	// @theme beats the probe: the terminal says dark, the host light.
	r := startSessionWith(t, autoDoc, &termSession{probe: true, themeQuery: true, wait: 10 * time.Second}, func(a *App) { _ = a.Set("@theme", "light") })
	r.waitOut(t, "\x1b[c")
	r.send(t, "\x1b]11;rgb:0/0/0\a\x1b[?64c")
	r.next(t)
	r.send(t, "\x03")
	if err := r.wait(t); err != nil {
		t.Fatal(err)
	}
	if out := r.out.String(); !strings.Contains(out, lightBG) || strings.Contains(out, darkBG) {
		t.Error("@theme light did not beat a dark probe")
	}
	// TUIMARK_THEME beats @theme.
	r = startSessionWith(t, autoDoc, &termSession{}, func(a *App) {
		_ = a.Set("@theme", "light")
		a.overrideTheme("dark")
	})
	r.waitOut(t, "AUTO")
	r.next(t)
	r.send(t, "\x03")
	if err := r.wait(t); err != nil {
		t.Fatal(err)
	}
	if out := r.out.String(); !strings.Contains(out, darkBG) || strings.Contains(out, lightBG) {
		t.Error("TUIMARK_THEME=dark did not beat @theme light")
	}
	// @theme set to auto during the session: the recorded background.
	r = startSession(t, strings.Replace(autoDoc, `theme="auto"`, `theme="dark"`, 1), &termSession{probe: true, themeQuery: true, wait: 10 * time.Second})
	r.waitOut(t, "\x1b[c")
	r.send(t, "\x1b]11;rgb:ffff/ffff/ffff\a\x1b[?64c")
	r.next(t)
	if strings.Contains(r.out.String(), lightBG) {
		t.Fatal("a dark document was drawn light before @theme")
	}
	if err := r.app.Set("@theme", "auto"); err != nil {
		t.Fatal(err)
	}
	r.waitOut(t, lightBG)
	r.send(t, "\x03")
	if err := r.wait(t); err != nil {
		t.Fatal(err)
	}
}

// startSessionWith is startSession with a hook run on the app before the
// session starts.
func startSessionWith(t *testing.T, src string, sess *termSession, setup func(*App)) *sessRig {
	t.Helper()
	a := doc(t, src)
	_ = a.Bind("", map[string]any{"text": "", "rows": []any{"a", "b"}})
	setup(a)
	r := &sessRig{app: a, out: &syncBuf{}, sigs: make(chan os.Signal, 1), done: make(chan error, 1), evs: make(chan Event, 64)}
	for _, act := range []string{"chg", "hello", "pick"} {
		a.On(act, func(ev Event) error { r.evs <- ev; return nil })
	}
	pr, pw := io.Pipe()
	r.in = pw
	go func() { r.done <- a.session(pr, r.out, func() (int, int) { return 20, 6 }, nil, r.sigs, sess) }()
	t.Cleanup(func() { pw.Close() })
	return r
}

// 43. Set("@theme") changes the tools' theme: Frame and Dump colors, and
// Theme(); "auto" is dark there; a theme Run resolved never reaches Dump
// or Validate (MUST 13).
func TestSetThemeReachesTools(t *testing.T) {
	a := doc(t, autoDoc)
	bg := func() css.Color { return a.Frame(10, 3).Grid.At(8, 2).BG }
	light, _ := css.ParseLiteralColor(css.Themes["light"]["bg"])
	dark, _ := css.ParseLiteralColor(css.Themes["dark"]["bg"])
	if bg() != dark || a.Theme() != "dark" {
		t.Fatalf("an auto document outside Run is dark: %v %q", bg(), a.Theme())
	}
	if err := a.Set("@theme", "light"); err != nil {
		t.Fatal(err)
	}
	if bg() != light || a.Theme() != "light" {
		t.Errorf("@theme light: %v %q", bg(), a.Theme())
	}
	// @theme beats --theme (SetTheme).
	if err := a.SetTheme("dark"); err != nil {
		t.Fatal(err)
	}
	if bg() != light {
		t.Error("--theme beat @theme")
	}
	_ = a.Set("@theme", "auto")
	if bg() != dark || a.Theme() != "dark" {
		t.Errorf("@theme auto outside Run: %v %q", bg(), a.Theme())
	}
	if err := a.Set("@theme", "blue"); err == nil || a.Theme() != "dark" {
		t.Errorf("@theme blue: %v, theme %q", err, a.Theme())
	}
	// What Run resolved reaches Frame, never Dump.
	a.resolveAuto("light", "")
	if a.Dump(10, 3, false).Grid == nil || a.Theme() != "dark" {
		t.Error("Dump saw the probed theme")
	}
	if bg() != light {
		t.Error("Frame did not use the resolved auto theme")
	}
	a.endRun()
	if bg() != dark {
		t.Error("the resolved theme outlived Run")
	}
}

// 42. @media (theme: …) and the token order: a later light :root block
// refines the base under light only; one placed before the base loses.
func TestThemeMediaTokens(t *testing.T) {
	src := `<tui version="2"><style>
:root { --brand: #e0443e; }
@media (theme: light) { :root { --brand: #c8102e; --bg: #fbfaf8; } }
@media (theme: dark) { #t { bold: true; } }
#t { color: $brand; background: $bg; }
</style><screen id="s"><text id="t">x</text></screen></tui>`
	a := doc(t, src)
	cell := func() (css.Color, css.Color, bool) {
		c := a.Frame(5, 1).Grid.At(0, 0)
		return c.FG, c.BG, c.Attrs != 0
	}
	fg, bg, bold := cell()
	if fg.String() != "#e0443e" || bg.String() != "#0d1117" || !bold {
		t.Errorf("dark: fg %v bg %v bold %v", fg, bg, bold)
	}
	_ = a.Set("@theme", "light")
	fg, bg, bold = cell()
	if fg.String() != "#c8102e" || bg.String() != "#fbfaf8" || bold {
		t.Errorf("light: fg %v bg %v bold %v", fg, bg, bold)
	}
	// The same block before the base :root loses.
	b := doc(t, strings.Replace(strings.Replace(src, ":root { --brand: #e0443e; }\n", "", 1), "@media (theme: dark)", ":root { --brand: #e0443e; }\n@media (theme: dark)", 1))
	_ = b.Set("@theme", "light")
	if c := b.Frame(5, 1).Grid.At(0, 0); c.FG.String() != "#e0443e" || c.BG.String() != "#fbfaf8" {
		t.Errorf("a light block before the base: fg %v bg %v", c.FG, c.BG)
	}
}

// 42. validate on an auto document checks both themes: a token defined
// only under @media (theme: light) is reported once, from the dark
// renders; with --theme light it is not reported.
func TestValidateAutoChecksBothThemes(t *testing.T) {
	src := `<tui version="2" theme="auto"><style>
@media (theme: light) { :root { --brand: #c8102e; } }
#t { color: $brand; }
</style><screen id="s"><text id="t">x</text></screen></tui>`
	a := doc(t, src)
	n := 0
	for _, d := range a.Validate() {
		if d.Code == "V003" && strings.Contains(d.Msg, "$brand") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("$brand reported %d times, want 1: %v", n, a.Validate())
	}
	b := doc(t, src)
	_ = b.SetTheme("light")
	for _, d := range b.Validate() {
		if strings.Contains(d.Msg, "$brand") {
			t.Errorf("--theme light: %s", d)
		}
	}
	for _, d := range doc(t, strings.Replace(src, "(theme: light)", "(theme: auto)", 1)).Validate() {
		if d.Code == "V003" && strings.Contains(d.Msg, "(theme: auto)") {
			return
		}
	}
	t.Error("@media (theme: auto) is not V003")
}

// 67 (session level). TUIMARK_LOG: one JSON object per line, start then
// caps; keys by token; while a secret input has focus, key and paste
// records are redacted and its on:change records have no value.
func TestRunLogRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.log")
	lg, err := openRunLog(path, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	r := startSession(t, autoDoc, &termSession{log: lg})
	r.next(t) // on:focus of #q
	r.send(t, "ab")
	if ev := r.next(t); ev.Value != "ab" {
		t.Fatalf("%+v", ev)
	}
	r.send(t, "\t") // focus #pw, the secret input
	r.send(t, "s3")
	r.next(t)
	r.send(t, "\x1b[200~pa\x1b[201~")
	r.next(t)
	r.send(t, "\x03")
	if err := r.wait(t); err != nil {
		t.Fatal(err)
	}
	lg.end("quit")
	recs := readLog(t, path)
	if len(recs) < 4 || recs[0]["ev"] != "start" || recs[1]["ev"] != "caps" || recs[len(recs)-1]["ev"] != "end" {
		t.Fatalf("records %v", recs)
	}
	if recs[1]["theme"] != "dark" || recs[1]["theme_from"] != "default" || recs[1]["grapheme"] != "none" || recs[1]["probe"] != false {
		t.Errorf("caps = %v", recs[1])
	}
	var keys []string
	redacted, secretChanges, frames := 0, 0, 0
	sawActionOrder := false
	for _, rec := range recs {
		switch rec["ev"] {
		case "key", "paste":
			if rec["redacted"] == true {
				redacted++
				if len(rec) != 3 {
					t.Errorf("a redacted record carries more: %v", rec)
				}
				continue
			}
			if rec["ev"] == "key" {
				keys = append(keys, rec["key"].(string))
			}
		case "action":
			if rec["source"] == "pw" {
				if rec["action"] == "chg" {
					secretChanges++
				}
				if _, ok := rec["value"]; ok || rec["redacted"] != true {
					t.Errorf("a secret input's event logged its value: %v", rec)
				}
			}
			if rec["source"] == "q" && rec["action"] == "chg" {
				sawActionOrder = rec["value"] == "ab"
			}
		case "frame":
			frames++
		}
	}
	// ctrl+c comes while the secret input has focus: redacted too.
	if strings.Join(keys, " ") != "a b tab" {
		t.Errorf("logged keys %v", keys)
	}
	if redacted != 4 || secretChanges != 2 || frames == 0 || !sawActionOrder {
		t.Errorf("redacted %d, secret changes %d, frames %d, #q change logged %v", redacted, secretChanges, frames, sawActionOrder)
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "s3") || strings.Contains(string(raw), `"pa"`) {
		t.Errorf("the log holds secret text:\n%s", raw)
	}
}

func readLog(t *testing.T, path string) []map[string]any {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []map[string]any
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatalf("line %q is not one JSON object: %v", sc.Text(), err)
		}
		if !strings.HasPrefix(sc.Text(), `{"ev":`) || !strings.Contains(sc.Text(), `,"t":`) {
			t.Errorf("line %q does not start with ev and t", sc.Text())
		}
		out = append(out, m)
	}
	return out
}

// 67. The log file is created 0600, an existing wider mode is narrowed,
// and a path in a missing directory is an error before the terminal is
// touched.
func TestRunLogFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "wide.log")
	if err := os.WriteFile(p, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, 0o644); err != nil {
		t.Fatal(err)
	}
	lg, err := openRunLog(p, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	lg.end("quit")
	st, _ := os.Stat(p)
	if st.Mode().Perm() != 0o600 {
		t.Errorf("mode %v, want 0600", st.Mode().Perm())
	}
	if b, _ := os.ReadFile(p); strings.Contains(string(b), "old") {
		t.Error("the log was not truncated")
	}
	lg, err = openRunLog(filepath.Join(dir, "new.log"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	lg.end("eof")
	if st, _ := os.Stat(filepath.Join(dir, "new.log")); st.Mode().Perm() != 0o600 {
		t.Errorf("new log mode %v", st.Mode().Perm())
	}
	// Run with a log path in a missing directory: an error, nothing written.
	t.Setenv(envLog, filepath.Join(dir, "missing", "x.log"))
	a := doc(t, autoDoc)
	var out strings.Builder
	err = a.Run(&out)
	if err == nil || !strings.Contains(err.Error(), envLog) || out.Len() != 0 {
		t.Errorf("Run = %v, wrote %q", err, out.String())
	}
	// A nil log is a no-op everywhere.
	var none *runLog
	none.write("key")
	none.key(Key{Name: "a"}, false)
	none.end("quit")
}

// 67. dump, play, and Dump() ignore TUIMARK_LOG: rendering never creates
// the file.
func TestDumpIgnoresRunLog(t *testing.T) {
	p := filepath.Join(t.TempDir(), "never.log")
	t.Setenv(envLog, p)
	a := doc(t, autoDoc)
	a.Dump(20, 3, false)
	a.Frame(20, 3)
	a.Validate()
	if _, err := os.Stat(p); err == nil {
		t.Error("a render created the TUIMARK_LOG file")
	}
}

// :focus-within (version="2") matches the focused node and each ancestor
// up to the screen, through a modal; nothing when nothing is focused.
func TestFocusWithinChain(t *testing.T) {
	src := `<tui version="2"><style>row:focus-within { bold: true; } screen:focus-within { italic: true; }</style>
<screen id="s" focus="#b"><row id="r"><button id="b" label="go"/></row><row id="o"><text>x</text></row>
<modal id="m" open="m_open"><button id="mb" label="ok"/></modal></screen></tui>`
	a := doc(t, src)
	f := a.Frame(20, 6)
	if !f.ByID["r"].Style.Bold || f.ByID["o"].Style.Bold || !f.Root.Style.Italic {
		t.Errorf("chain: r %v o %v screen %v", f.ByID["r"].Style.Bold, f.ByID["o"].Style.Bold, f.Root.Style.Italic)
	}
	_ = a.Set("m_open", true)
	f = a.Frame(20, 6)
	if f.Focus != "mb" || f.ByID["r"].Style.Bold || !f.Root.Style.Italic || !f.ByID["m"].FocusWithin {
		t.Errorf("modal chain: focus %q r %v screen %v", f.Focus, f.ByID["r"].Style.Bold, f.Root.Style.Italic)
	}
	c := doc(t, `<tui version="2"><style>screen:focus-within { italic: true; }</style><screen id="s"><text>x</text></screen></tui>`)
	if c.Frame(5, 1).Root.Style.Italic {
		t.Error(":focus-within matched with nothing focused")
	}
}
