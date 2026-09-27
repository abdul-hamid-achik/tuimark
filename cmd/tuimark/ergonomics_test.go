package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// Regression (review of 0.2b, SPEC v0.2b §8.6, §15.4): the keys and
// pastes of one text: step are dispatched each on the frame the previous
// one left, so a step gives the same events whether its keys come in one
// read or in separate steps: typing then enter sees the input's class
// guard, a paste then enter too, a list moved by down then x sees the
// list's guard, and a filter typed then down sees the list its if= just
// showed.
func TestPlayKeysOfOneStepSeeTheirEdits(t *testing.T) {
	dir := t.TempDir()
	docs := map[string]string{
		"guard": `<tui version="2"><keymap><bind keys="enter" action="go" when="#q.filled"/></keymap>
<screen id="s" focus="#q"><input id="q" bind="query" class:filled="query"/></screen></tui>`,
		"list": `<tui version="2"><keymap><bind keys="x" action="picked" when="#l.picked"/></keymap>
<screen id="s" focus="#l"><list id="l" each="rows as r" key="r" bind="cur" class:picked="cur"><item><text>{r}</text></item></list></screen></tui>`,
		"filter": `<tui version="2"><keymap><bind keys="down" action="move-next" to="#res"/></keymap>
<screen id="s" focus="#q"><input id="q" bind="query"/>
<list id="res" if="query" each="rows as r" key="r" bind="cur" on:select="picked"><item><text>{r}</text></item></list></screen></tui>`,
	}
	data := filepath.Join(dir, "d.json")
	writeFile(t, data, `{"query":"","cur":null,"rows":["a","b","c"]}`)
	for _, c := range []struct {
		doc, input, want string
	}{
		{"guard", "text:ab\r", `1 go {}`},
		{"guard", "text:ab enter", `2 go {}`},
		{"guard", "text:\x1b[200~ab\x1b[201~\r", `1 go {}`},
		{"guard", "paste:ab enter", `2 go {}`},
		{"list", "text:\x1b[Bx", `1 picked {"r":"b"}`},
		{"list", "down text:x", `2 picked {"r":"b"}`},
		{"filter", "text:x\x1b[B", `1 picked {"r":"b"}`},
		{"filter", "text:x down", `2 picked {"r":"b"}`},
	} {
		tui := filepath.Join(dir, c.doc+".tui")
		writeFile(t, tui, docs[c.doc])
		code, out, errw := runCLI("play", tui, "--data", data, "--cols", "20", "--rows", "4", "--format", "json", "--input", c.input)
		if code != 0 {
			t.Fatalf("%s %q: exit %d: %s", c.doc, c.input, code, errw)
		}
		if got := strings.Join(playEvents(t, out), "|"); got != c.want {
			t.Errorf("%s %q: events %q, want %q", c.doc, c.input, got, c.want)
		}
	}
}
