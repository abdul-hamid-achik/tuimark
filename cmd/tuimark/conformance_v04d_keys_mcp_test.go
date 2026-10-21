package main

import (
	"fmt"
	"reflect"
	"testing"
)

// SPEC v0.4d §15.8: tuimark_play takes keyboard, "kitty" or "legacy", and
// passes it as --keyboard; each result is what the command prints.
func TestD2MCPKeyboard(t *testing.T) {
	var tool *mcpTool
	for i := range mcpTools {
		if mcpTools[i].Name == "tuimark_play" {
			tool = &mcpTools[i]
		}
	}
	props := mcpSchema(*tool)["properties"].(map[string]any)
	kb, ok := props["keyboard"].(map[string]any)
	if !ok || !reflect.DeepEqual(kb["enum"], []string{"kitty", "legacy"}) || kb["type"] != "string" {
		t.Errorf("keyboard property = %v", props["keyboard"])
	}
	for _, c := range []struct{ kb, step string }{{"legacy", "shift+enter"}, {"kitty", "shift+enter"}, {"legacy", "alt+enter"}} {
		params := fmt.Sprintf(`{"name":"tuimark_play","arguments":{"file":%q,"data":%q,"steps":[%q],"keyboard":%q}}`, d2K1, d2AB, c.step, c.kb)
		got, isErr := mcpCall(t, params)
		want, wantErr := cliResult("play", d2K1, "--data", d2AB, "--input", c.step, "--keyboard", c.kb, "--format", "json")
		if got != want || isErr != wantErr {
			t.Errorf("%s %s: MCP %q (%v), CLI %q (%v)", c.kb, c.step, got, isErr, want, wantErr)
		}
	}
	params := fmt.Sprintf(`{"name":"tuimark_play","arguments":{"file":%q,"steps":["shift+enter"],"keyboard":"legacy"}}`, d2K1)
	got, isErr := mcpCall(t, params)
	wantMsg := "tuimark: play: step 1 (shift+enter): key \"shift+enter\" cannot be sent by a legacy terminal (--keyboard legacy)\n"
	if !isErr || got != wantMsg {
		t.Errorf("legacy shift+enter: %q %v", got, isErr)
	}
	params = fmt.Sprintf(`{"name":"tuimark_play","arguments":{"file":%q,"keyboard":7}}`, d2K1)
	if got, isErr := mcpCall(t, params); !isErr || got != "tuimark: keyboard: want string\n" {
		t.Errorf("keyboard 7: %q %v", got, isErr)
	}
}
