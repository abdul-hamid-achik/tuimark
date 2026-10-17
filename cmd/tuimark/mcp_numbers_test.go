package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// A line is "not JSON" (-32700, SPEC §15.8) only when its syntax is wrong,
// not when it holds a number a float64 cannot hold: 1e400 and a
// 400-digit integer are valid JSON, whether they sit in a member the
// server does not read, are echoed as an "id", or reach the command as
// the digits of an integer argument.
func TestMCPNumbersBeyondFloat64AreStillJSON(t *testing.T) {
	huge := strings.Repeat("9", 400)
	f := mcpFixtureFile
	// handle runs one line through mcpHandleLine and returns the response
	// line exactly as runMCPServer would write it.
	handle := func(l string) string {
		t.Helper()
		resp, answer := mcpHandleLine([]byte(l))
		if !answer {
			t.Fatalf("%s: no answer", l)
		}
		b, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("%s: marshal: %v", l, err)
		}
		return string(b)
	}
	for _, c := range []struct{ name, l string }{
		{"ignored member 1e400", `{"jsonrpc":"2.0","id":1,"method":"ping","extra":1e400}`},
		{"ignored member 400 digits", `{"jsonrpc":"2.0","id":1,"method":"ping","extra":` + huge + `}`},
		{"ignored nested 1e400", `{"jsonrpc":"2.0","id":1,"method":"ping","params":{"a":[1e400,-1e400]}}`},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got, want := handle(c.l), `{"jsonrpc":"2.0","id":1,"result":{}}`; got != want {
				t.Errorf("got %s, want %s", got, want)
			}
		})
	}
	t.Run("id 1e400 is echoed as written", func(t *testing.T) {
		got := handle(`{"jsonrpc":"2.0","id":1e400,"method":"ping"}`)
		if want := `{"jsonrpc":"2.0","id":1e400,"result":{}}`; got != want {
			t.Errorf("got %s, want %s", got, want)
		}
	})
	t.Run("id of 400 digits is echoed as written", func(t *testing.T) {
		got := handle(`{"jsonrpc":"2.0","id":` + huge + `,"method":"ping"}`)
		if want := `{"jsonrpc":"2.0","id":` + huge + `,"result":{}}`; got != want {
			t.Errorf("got %.80s..., want the id echoed", got)
		}
	})
	t.Run("id 1e400 on an invalid request is echoed", func(t *testing.T) {
		got := handle(`{"id":1e400}`)
		if want := `{"jsonrpc":"2.0","id":1e400,"error":{"code":-32600,"message":"invalid request"}}`; got != want {
			t.Errorf("got %s, want %s", got, want)
		}
	})
	t.Run("cols 1e400 reaches the flag parser as its exact decimal text (§15.8)", func(t *testing.T) {
		got := handle(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"tuimark_dump","arguments":{"file":"` + f + `","cols":1e400}}}`)
		var resp struct {
			Result struct {
				IsError bool `json:"isError"`
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"result"`
			Error *rpcErrInfo `json:"error"`
		}
		if err := json.Unmarshal([]byte(got), &resp); err != nil || resp.Error != nil {
			t.Fatalf("got %s, want a tool result", got)
		}
		want, wantErr := cliResult("dump", f, "--cols", "1"+strings.Repeat("0", 400), "--format", "json")
		if !wantErr || !resp.Result.IsError || len(resp.Result.Content) != 1 || resp.Result.Content[0].Text != want {
			t.Errorf("got %s, want isError with %q (the CLI's own result for the same digits)", got, want)
		}
	})
	t.Run("a line that is not JSON is still -32700", func(t *testing.T) {
		want := `{"jsonrpc":"2.0","id":null,"error":{"code":-32700,"message":"parse error"}}`
		for _, l := range []string{`not json`, `{"jsonrpc":"2.0","id":1e400,`, `{"id":1e400}x`, `{"id":01}`, `{"id":1e}`} {
			if got := handle(l); got != want {
				t.Errorf("%s: got %s, want %s", l, got, want)
			}
		}
	})
}
