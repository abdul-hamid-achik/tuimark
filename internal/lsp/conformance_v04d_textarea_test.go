package lsp

import (
	"path/filepath"
	"strings"
	"testing"
)

// SPEC v0.4d §6.19, §15.12 (test 310, the textarea parts): the language
// server completes the tag textarea and its attributes in a <tui> without
// version only, completes textarea as a type in a stylesheet, hovers it,
// and reports the diagnostics of test 285 as validate does.
func TestD3LSPTextarea(t *testing.T) {
	dir := writeTree(t, map[string]string{"t.tcss": "text { }\n"})
	app := filepath.Join(dir, "app.tui")
	sheet := filepath.Join(dir, "t.tcss")
	c := startServer(t, Options{})
	c.initialize()
	cur := "<tui>\n  <style src=\"t.tcss\"/>\n  <screen id=\"main\"><text id=\"r\"/></screen>\n</tui>\n"
	c.open(app, "tuimark", cur)
	for _, tc := range []completionCase{
		{name: "tag", path: filepath.Join(dir, "a.tui"), lang: "tuimark", text: `<tui><screen id="s"><texta|`,
			want: []string{"textarea"}},
		{name: "tag after <", path: filepath.Join(dir, "b.tui"), lang: "tuimark", text: `<tui><screen id="s"><|`,
			want: []string{"textarea", "input", "checkbox"}},
		{name: "not in version 3", path: filepath.Join(dir, "c.tui"), lang: "tuimark", text: `<tui version="3"><screen id="s"><|`,
			want: []string{"input"}, absent: []string{"textarea"}},
		{name: "not in version 1", path: filepath.Join(dir, "d.tui"), lang: "tuimark", text: `<tui version="1"><screen id="s"><|`,
			want: []string{"input"}, absent: []string{"textarea"}},
		{name: "attributes", path: filepath.Join(dir, "e.tui"), lang: "tuimark", text: `<tui><screen id="s"><textarea |`,
			want: []string{"bind", "placeholder", "on:change", "on:submit", "id", "disabled", "on:click"}, absent: []string{"secret", "each", "label"}},
	} {
		text := strings.Replace(tc.text, "|", "", 1)
		uri := c.open(tc.path, tc.lang, text)
		c.change(uri, 2, text)
		got := completionLabels(t, c, uri, positionAt(text, strings.IndexByte(tc.text, '|')))
		for _, w := range tc.want {
			if !got[w] {
				t.Errorf("%s: %v, missing %q", tc.name, keys(got), w)
			}
		}
		for _, a := range tc.absent {
			if got[a] {
				t.Errorf("%s: %q offered", tc.name, a)
			}
		}
	}

	// A type selector in a stylesheet the current language loads.
	text := "text > texta"
	uri := c.open(sheet, "tcss", text)
	c.change(uri, 2, text)
	if got := completionLabels(t, c, uri, positionAt(text, len(text))); !got["textarea"] {
		t.Errorf("textarea as a type in a stylesheet: %v", keys(got))
	}

	// The diagnostics of test 285, as validate reports them.
	bad := `<tui><screen id="main"><textarea bind="d"/><textarea id="s" secret="true"/><textarea id="t">hi</textarea></screen></tui>`
	p := c.nextPublish(c.open(filepath.Join(dir, "bad.tui"), "tuimark", bad))
	var msgs []string
	for _, d := range p.Diagnostics {
		msgs = append(msgs, d.Code+" "+d.Message)
	}
	for _, want := range []string{
		"V012 <textarea> is focusable and needs an id",
		`V002 attribute "secret" is not allowed on <textarea>`,
		"V013 text is not allowed directly inside <textarea>; its text is the value at its bind path",
	} {
		found := false
		for _, m := range msgs {
			if strings.HasPrefix(m, want) {
				found = true
			}
		}
		if !found {
			t.Errorf("no %q in %q", want, msgs)
		}
	}
	// A pinned document: the section 5.1 message.
	pinned := `<tui version="3"><screen id="main"><textarea id="t"/></screen></tui>`
	p = c.nextPublish(c.open(filepath.Join(dir, "pin.tui"), "tuimark", pinned))
	if len(p.Diagnostics) != 1 || p.Diagnostics[0].Code != "V001" || p.Diagnostics[0].Message != "tag <textarea> (requires a <tui> without version)" {
		t.Errorf("pinned textarea: %+v", p.Diagnostics)
	}
	// Hover names the tag and its attributes with their language.
	hov := "<tui>\n  <screen id=\"main\"><textarea id=\"c\" bind=\"d\" placeholder=\"m\"/></screen>\n</tui>\n"
	huri := c.open(filepath.Join(dir, "hov.tui"), "tuimark", hov)
	for _, tc := range []struct{ mark, want string }{{"<textarea", "textarea"}, {`placeholder=`, "placeholder"}} {
		i := strings.Index(hov, tc.mark)
		if got := hoverAt(t, c, huri, positionAt(hov, i+2)); !strings.Contains(got, tc.want) || got == "" {
			t.Errorf("hover on %q: %q", tc.mark, got)
		}
	}
	_ = c.shutdown()
}
