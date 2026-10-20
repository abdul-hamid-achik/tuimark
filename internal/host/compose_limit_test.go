//go:build !js

package host

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ADR 0034 through LoadBuffers (the LSP's loader): thirty plain files that
// include the next twice each do not hang it; the expansion stops at the
// limit with one V021.
func TestExpansionLimitLoadBuffers(t *testing.T) {
	files := map[string]string{
		"app.tui": "<tui>\n  <screen id=\"s\"><include src=\"f0.tui\"/></screen>\n</tui>\n",
		"f30.tui": "<fragment><text>x</text></fragment>",
	}
	for i := 0; i < 30; i++ {
		files[fmt.Sprintf("f%d.tui", i)] = fmt.Sprintf("<fragment><include src=\"f%d.tui\"/><include src=\"f%d.tui\"/></fragment>", i+1, i+1)
	}
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
		if len(v) != 1 || v[0] != `include src="f30.tui" exceeds the expansion limit (100000 elements or 16 MiB of placed source): it inserts nothing, and no later include or use expands` {
			t.Errorf("V021: %v", v)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("LoadBuffers hangs on the fan-out")
	}
}

// A copy that goes over the limit is undone with the empty copies its
// uses recorded (ir.Node.EmptyCopies): the use of an empty template in
// it, which inserts nothing as the rest of it does, reports nothing.
func TestExpansionLimitUndoesEmptyCopies(t *testing.T) {
	src := "<tui>\n  <template id=\"e\"></template>\n" +
		"  <template id=\"big\"><use template=\"e\" with=\"nope as f\"/>" + strings.Repeat("<text>x</text>", 100001) + "</template>\n" +
		"  <screen id=\"main\"><use template=\"big\"/></screen>\n</tui>\n"
	a := useApp(t, src, `{}`)
	var got []string
	for _, d := range a.Validate() {
		got = append(got, d.Code)
	}
	if fmt.Sprint(got) != "[V021]" {
		t.Errorf("diagnostics %v, want only the V021", got)
	}
}

// ADR 0034: includes count toward the limit, so a fan-out whose leaf holds
// no element is bounded too, under LoadBuffers as under Load.
func TestExpansionLimitCountsIncludesLoadBuffers(t *testing.T) {
	files := map[string]string{
		"app.tui": "<tui>\n  <screen id=\"s\"><include src=\"f0.tui\"/></screen>\n</tui>\n",
		"f30.tui": "<fragment/>",
	}
	for i := 0; i < 30; i++ {
		files[fmt.Sprintf("f%d.tui", i)] = fmt.Sprintf("<fragment><include src=\"f%d.tui\"/><include src=\"f%d.tui\"/></fragment>", i+1, i+1)
	}
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
	case <-time.After(30 * time.Second):
		t.Fatal("LoadBuffers hangs on the fan-out of empty fragments")
	}
}
