package host

import (
	"testing"
	"testing/fstest"
)

// F11's clock seam asks whether the document holds a spinner; the walk
// reads the expanded tree, so a spinner an include placed counts (SPEC
// v0.4b §6.4.1, §6.18.1).
func TestSpinnerPlacedByAnIncludeIsFound(t *testing.T) {
	for _, c := range []struct {
		frag string
		want bool
	}{
		{`<progress indeterminate="true"/>`, true},
		{`<text>no spinner</text>`, false},
	} {
		mem := fstest.MapFS{
			"app.tui": {Data: []byte("<tui>\n  <screen id=\"s\"><include src=\"p.tui\"/></screen>\n</tui>\n")},
			"p.tui":   {Data: []byte("<fragment>" + c.frag + "</fragment>")},
		}
		app, err := LoadFS(mem, "app.tui")
		if err != nil {
			t.Fatal(err)
		}
		if got := app.HasSpinner(); got != c.want {
			t.Errorf("%s: HasSpinner = %v, want %v", c.frag, got, c.want)
		}
	}
}
