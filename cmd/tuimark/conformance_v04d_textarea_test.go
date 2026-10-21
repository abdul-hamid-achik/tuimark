package main

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// SPEC v0.4d §21 tests of phase D3 (the textarea) through the CLI: the
// dump (284), IR (273), the pinned messages (274), and the catalog (309).
// The Go API half is conformance_v04d_textarea*_test.go at the root.

const taDir = runsFixtures + "t284/"

// 284, through the CLI: the dump of t.tui, its members in the order of
// SPEC §13.2 (caret last), and the schema.
func TestD3Textarea284Dump(t *testing.T) {
	code, out, errw := runCLI("dump", taDir+"t.tui", "--data", taDir+"d.json", "--cols", "30", "--rows", "6", "--format", "json")
	if code != 0 {
		t.Fatalf("exit %d %s", code, errw)
	}
	if errs := validateSchema(t, dumpV02Schema, decodeJSON(t, []byte(out))); len(errs) != 0 {
		t.Errorf("dump vs dump.v0.2.json: %v", errs)
	}
	m := jsonMap(t, out)
	n := nodeOfMap(t, m, "composer")
	want := map[string]any{
		"id": "composer", "tag": "textarea", "x": 0.0, "y": 1.0, "w": 30.0, "h": 1.0, "focused": true,
		"scroll": map[string]any{"y": 0.0, "h": 1.0},
		"caret":  map[string]any{"line": 0.0, "col": 0.0, "x": 0.0, "y": 1.0},
	}
	if !reflect.DeepEqual(n, want) {
		t.Errorf("composer: %v", n)
	}
	// The caret comes after every other member.
	i, j := strings.Index(out, `"scroll"`), strings.Index(out, `"caret"`)
	if i < 0 || j < i || strings.Index(out[j:], `"id": "status"`) < 0 {
		t.Errorf("caret is not last in the node:\n%s", out)
	}
	// No other node has a caret, and an unfocused textarea has none.
	if strings.Count(out, `"caret"`) != 1 {
		t.Errorf("%d carets", strings.Count(out, `"caret"`))
	}
	other := strings.Replace(readFile(t, taDir+"t.tui"), `focus="#composer"`, "", 1)
	code, out, _ = runCLI("dump", writeTemp(t, "u.tui", other), "--cols", "30", "--rows", "6", "--format", "json")
	if code != 0 && code != 2 {
		t.Fatalf("exit %d", code)
	}
	if strings.Contains(out, `"caret"`) && jsonMap(t, out)["focus"] == nil {
		t.Errorf("an unfocused textarea has a caret:\n%s", out)
	}
	// The text dump names it, focused.
	code, out, _ = runCLI("dump", taDir+"t.tui", "--data", taDir+"d.json", "--cols", "30", "--rows", "6")
	if code != 0 || !strings.Contains(out, "composer  textarea  30x1 @(0,1) focus") {
		t.Errorf("text dump:\n%s", out)
	}
}

// 273, the textarea part: a node of kind textarea, with bind in bind and
// placeholder in attrs, valid against schema/ir.v0.4.json (irRoot checks
// it).
func TestD3Textarea273IR(t *testing.T) {
	root := irRoot(t, taDir+"t.tui")
	var ta map[string]any
	for _, screen := range kids(t, root) {
		for _, k := range kids(t, screen) {
			if k["kind"] == "textarea" {
				ta = k
			}
		}
	}
	if ta == nil {
		t.Fatalf("no textarea in %v", kids(t, root))
	}
	if ta["id"] != "composer" || ta["bind"] != "draft" || !reflect.DeepEqual(attrsOf(ta), map[string]any{"placeholder": "Message"}) {
		t.Errorf("textarea node: %v", ta)
	}
	if on, _ := ta["on"].(map[string]any); on["change"] != "typed" || on["submit"] != "send" {
		t.Errorf("events: %v", ta["on"])
	}
	if c, _ := ta["children"].([]any); len(c) != 0 {
		t.Errorf("children: %v", c)
	}
}

// 274, the textarea parts, and the whole list of nine errors at the
// positions of the SPEC.
func TestD3Pinned274(t *testing.T) {
	got := vdiags(t, runsFixtures+"t274/pin.tui", "")
	want := []struct{ pos, msg string }{
		{":2:10 ", `selector "text > span.code": type "span" (requires a <tui> without version)`},
		{":2:43 ", `selector "textarea": type "textarea" (requires a <tui> without version)`},
		{":4:5 ", `tag <textarea> (requires a <tui> without version)`},
		{":5:11 ", `attribute "runs" on <text> (requires a <tui> without version)`},
		{":5:20 ", `wrap: value "pre-wrap" (want wrap | nowrap | truncate | truncate-start | truncate-middle) (requires a <tui> without version)`},
		{":6:5 ", `unknown tag <span>; use <text>`},
		{":8:11 ", `key "delete" (requires a <tui> without version)`},
		{":8:11 ", `key "alt+x" (requires a <tui> without version)`},
		{":8:11 ", `key "shift+enter" (requires a <tui> without version)`},
	}
	if len(got) != len(want) {
		t.Fatalf("%d diagnostics: %v", len(got), got)
	}
	codes := []string{"V003", "V003", "V001", "V002", "V003", "V001", "V003", "V003", "V003"}
	for i, w := range want {
		if !strings.HasPrefix(got[i], "error "+codes[i]+" ") || !strings.Contains(got[i], w.pos) || !strings.HasSuffix(got[i], w.msg) {
			t.Errorf("diagnostic %d = %s, want %s%s at%s", i+1, got[i], codes[i], w.msg, w.pos)
		}
	}
	// A pinned textarea is checked no further: its own attributes and
	// content add nothing.
	src := "<tui version=\"2\"><screen id=\"main\"><textarea secret=\"true\">x</textarea></screen></tui>\n"
	got = vdiags(t, writeTemp(t, "p.tui", src), "")
	if len(got) != 1 || !strings.HasSuffix(got[0], `tag <textarea> (requires a <tui> without version)`) {
		t.Errorf("pinned textarea: %v", got)
	}
}

// 309, the textarea parts: the tag with since 4 and the attributes of
// §6.15, each since 4; the catalog validates against its schema (the rest
// of the test is phases D1 and D2's words).
func TestD3Catalog309(t *testing.T) {
	code, out, errw := runCLI("catalog")
	if code != 0 {
		t.Fatalf("exit %d %s", code, errw)
	}
	if errs := validateSchema(t, "../../schema/catalog.v1.json", decodeJSON(t, []byte(out))); len(errs) != 0 {
		t.Errorf("catalog vs catalog.v1.json: %v", errs)
	}
	var c struct {
		Tags []struct {
			Name  string `json:"name"`
			Since int    `json:"since"`
			Attrs []struct {
				Name  string `json:"name"`
				Since int    `json:"since"`
			} `json:"attrs"`
		} `json:"tags"`
		Diagnostics []struct {
			Code string `json:"code"`
		} `json:"diagnostics"`
	}
	if err := json.Unmarshal([]byte(out), &c); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tag := range c.Tags {
		if tag.Name != "textarea" {
			continue
		}
		found = true
		if tag.Since != 4 {
			t.Errorf("textarea since %d", tag.Since)
		}
		var names []string
		for _, a := range tag.Attrs {
			names = append(names, a.Name)
			if a.Since != 4 {
				t.Errorf("textarea attribute %s since %d", a.Name, a.Since)
			}
		}
		want := []string{"bind", "border", "class", "class:NAME", "disabled", "focusable", "gap", "height", "hidden", "id", "if", "on:change", "on:click", "on:focus", "on:submit", "pad", "placeholder", "style", "title", "width"}
		if !reflect.DeepEqual(names, want) {
			t.Errorf("textarea attributes %v, want %v", names, want)
		}
	}
	if !found {
		t.Error("the catalog has no textarea")
	}
	if len(c.Diagnostics) != 42 {
		t.Errorf("%d diagnostics, want 42", len(c.Diagnostics))
	}
}

// fmt writes a textarea as a self-closing leaf (SPEC v0.4d §15.6).
func TestD3Textarea284Fmt(t *testing.T) {
	src := "<tui>\n<screen id=\"main\">\n<textarea id=\"c\" bind=\"d\"></textarea>\n</screen>\n</tui>\n"
	code, out, errw := runCLI("fmt", writeTemp(t, "f.tui", src))
	if code != 0 {
		t.Fatalf("exit %d %s", code, errw)
	}
	if !strings.Contains(out, `<textarea id="c" bind="d"/>`) {
		t.Errorf("fmt:\n%s", out)
	}
}
