//go:build !js

// The expansion limits build documents of 100000 elements or 16 MiB of source and
// render them; under js/wasm (4 GiB, no preemption) they run out of memory,
// and internal/parse covers the same limits there without rendering.

package host

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ADR 0034, second amendment (the byte budget), through LoadBuffers: the two
// inputs of the round-5 review end with exactly one V021, and do not hang
// the language server.
func TestByteBudgetLoadBuffers(t *testing.T) {
	repeat := func(head, line string, n int, tail string) string {
		return head + strings.Repeat(line, n) + tail
	}
	var attrs strings.Builder
	attrs.WriteString("<fragment><text")
	for i := 0; i < 20000; i++ {
		fmt.Fprintf(&attrs, " a%d=\"\"", i)
	}
	attrs.WriteString(">x</text></fragment>")
	cases := map[string]map[string]string{
		"a": {
			"app.tui":  "<tui>\n<screen id=\"s\"><include src=\"mid.tui\"/></screen>\n</tui>\n",
			"mid.tui":  repeat("<fragment>", "<include src=\"leaf.tui\"/>\n", 2000, "</fragment>"),
			"leaf.tui": repeat("<fragment>", "<include src=\"x.tcss\"/>\n", 2000, "</fragment>"),
		},
		"b": {
			"app.tui":  repeat("<tui>\n<screen id=\"s\">\n", "<include src=\"leaf.tui\"/>\n", 1000, "</screen>\n</tui>\n"),
			"leaf.tui": attrs.String(),
		},
	}
	for name, files := range cases {
		t.Run(name, func(t *testing.T) {
			dir := writeTree(t, files)
			done := make(chan []string, 1)
			go func() {
				app, _ := LoadBuffers(filepath.Join(dir, "app.tui"), []byte(files["app.tui"]), func(string) ([]byte, bool) { return nil, false })
				var v []string
				for _, d := range app.Document().Diags {
					if d.Code == "V021" {
						v = append(v, d.Msg)
					}
				}
				done <- v
			}()
			select {
			case v := <-done:
				if len(v) != 1 {
					t.Errorf("V021: %v", v)
				}
			case <-time.After(60 * time.Second):
				t.Fatal("LoadBuffers hangs")
			}
		})
	}
}
