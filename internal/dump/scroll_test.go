package dump_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/dump"
	"github.com/abdul-hamid-achik/tuimark/internal/host"
)

// SPEC v0.2 §13.2 "scroll": every laid-out viewport (scroll, list,
// overflow: scroll container) carries its scroll state, also with nothing
// to scroll, in axis pairs and in the order x, y, w, h; no other node has
// the member.

func nodeJSON(t *testing.T, d *dump.Dump, id string) string {
	t.Helper()
	for _, n := range d.Nodes {
		if n.ID == id {
			b, err := json.Marshal(n)
			if err != nil {
				t.Fatal(err)
			}
			return string(b)
		}
	}
	t.Fatalf("no node %q", id)
	return ""
}

func TestNodeScrollMembers(t *testing.T) {
	d := dumpOf(t, `<tui version="1"><screen id="s"><col id="c">
<scroll id="empty" style="height: 3"></scroll>
<scroll id="x" axis="x" style="height: 2"><text>`+strings.Repeat("a", 58)+`</text></scroll>
<scroll id="both" axis="both" style="height: 4; width: 10; border: single"><text>`+strings.Repeat("b", 41)+`</text><text>b</text><text>c</text></scroll>
<box id="ov" style="overflow: scroll; height: 2"><text>1</text><text>2</text><text>3</text></box>
<row id="r" style="height: 1"><box id="ovrow" style="overflow: scroll; layout: row; width: 5"><text>`+strings.Repeat("r", 20)+`</text></box></row>
<text id="end">end</text>
</col></screen></tui>`, 30, 13, false)
	if len(d.Errors) != 0 {
		t.Fatalf("errors: %v", d.Errors)
	}
	for id, want := range map[string]string{
		// A viewport with no children: offset 0, extent = its content box.
		"empty": `{"id":"empty","tag":"scroll","x":0,"y":0,"w":30,"h":3,"scroll":{"y":0,"h":3}}`,
		"x":     `{"id":"x","tag":"scroll","x":0,"y":3,"w":30,"h":2,"scroll":{"x":0,"w":58}}`,
		"both":  `{"id":"both","tag":"scroll","x":0,"y":5,"w":10,"h":4,"scroll":{"x":0,"y":0,"w":41,"h":3}}`,
		"ov":    `{"id":"ov","tag":"box","x":0,"y":9,"w":30,"h":2,"scroll":{"y":0,"h":3}}`,
		// overflow: scroll scrolls on y only, also under layout: row
		// (axis= is scroll-only, SPEC v0.2 §11.4).
		"ovrow": `{"id":"ovrow","tag":"box","x":0,"y":11,"w":5,"h":1,"scroll":{"y":0,"h":1}}`,
		"c":     `{"id":"c","tag":"col","x":0,"y":0,"w":30,"h":13}`,
		"end":   `{"id":"end","tag":"text","x":0,"y":12,"w":30,"h":1,"text":"end"}`,
	} {
		if got := nodeJSON(t, d, id); got != want {
			t.Errorf("%s:\n got %s\nwant %s", id, got, want)
		}
	}
	for _, n := range d.Nodes {
		viewport := n.Tag == "scroll" || n.Tag == "list" || n.ID == "ov" || n.ID == "ovrow"
		if (n.Scroll != nil) != viewport {
			t.Errorf("%s#%s: scroll %+v", n.Tag, n.ID, n.Scroll)
		}
	}
}

// A list scrolled to its selection dumps the clamped offset it used and
// the full extent (§21 test 22); play, --frames, and the Go API all build
// their dumps with dump.Build, so the same member reaches every JSON form.
func TestNodeScrollFollowsListOffset(t *testing.T) {
	var rows []string
	for i := 0; i < 50; i++ {
		rows = append(rows, fmt.Sprintf("r%d", i))
	}
	a, err := host.Parse(strings.NewReader(`<tui version="1"><screen id="s" focus="#rows"><col>
<list id="rows" each="items as it" key="it" bind="sel"><item><text>{it}</text></item></list>
<text>footer</text>
</col></screen></tui>`))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Bind("items", rows); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		sel  string
		want string
	}{
		{"r0", `"scroll":{"y":0,"h":50}`},
		{"r9", `"scroll":{"y":3,"h":50}`},
		{"r49", `"scroll":{"y":43,"h":50}`},
	} {
		if err := a.Set("sel", c.sel); err != nil {
			t.Fatal(err)
		}
		d := a.Dump(20, 8, false)
		if got := nodeJSON(t, d, "rows"); !strings.HasSuffix(got, c.want+"}") {
			t.Errorf("sel %s: %s, want %s", c.sel, got, c.want)
		}
	}
	// The key path the play command and Run take: end moves the cursor to
	// the last row, and the next frame's dump shows the offset.
	frame := func() *dump.Dump {
		f := a.Frame(20, 8)
		return dump.Build(f.Cols, f.Rows, f.Root, f.Modals, f.Grid, f.Diags, f.Focus, false)
	}
	frame()
	a.HandleKey(host.DecodeKeys([]byte("\x1b[H"))[0]) // home
	if got := nodeJSON(t, frame(), "rows"); !strings.HasSuffix(got, `"scroll":{"y":0,"h":50}}`) {
		t.Errorf("after home: %s", got)
	}
	a.HandleKey(host.DecodeKeys([]byte("\x1b[F"))[0]) // end
	if got := nodeJSON(t, frame(), "rows"); !strings.HasSuffix(got, `"scroll":{"y":43,"h":50}}`) {
		t.Errorf("after end: %s", got)
	}
}
