package dump

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/layout"
	"github.com/abdul-hamid-achik/tuimark/internal/paint"
)

// A <text href> frame dumps its linked cells with the frame's link index,
// which schema/dump.v0.2.json's cell accepts (an additive "link" member).
func TestDumpCellLink(t *testing.T) {
	zs := &css.Style{}
	tx := &layout.Box{Tag: "text", Kind: "text", Style: zs, W: 2, H: 1, Laid: true, Text: "hi", Href: "https://x"}
	root := &layout.Box{Tag: "screen", Kind: "screen", ID: "m", Style: zs, W: 2, H: 1, Laid: true, Children: []*layout.Box{tx}}
	g := paint.NewGrid(2, 1)
	g.Links = []paint.GridLink{{ID: "l1", URL: "https://x"}}
	for x, r := range []rune{'h', 'i'} {
		c := g.At(x, 0)
		c.Ch, c.Owner, c.Link = r, "m", 1
	}
	if dc := CellAt(g, 0, 0); dc.Link != 1 {
		t.Fatalf("CellAt link = %d, want 1", dc.Link)
	}
	d := BuildWith(2, 1, root, nil, g, nil, "", Options{Cells: true})
	j, err := JSON(d)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(j), `"link": 1`) {
		t.Errorf("dump cells carry no link index:\n%s", j)
	}
	// The schema's cell object declares the additive member.
	s, err := os.ReadFile("../../schema/dump.v0.2.json")
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Defs struct {
			Cell struct {
				Props map[string]json.RawMessage `json:"properties"`
			} `json:"cell"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(s, &schema); err != nil {
		t.Fatal(err)
	}
	if _, ok := schema.Defs.Cell.Props["link"]; !ok {
		t.Error("schema cell has no link property")
	}
	// A cell outside a link omits it.
	bare := paint.NewGrid(1, 1)
	if got, _ := json.Marshal(CellAt(bare, 0, 0)); strings.Contains(string(got), "link") {
		t.Errorf("unlinked cell marshals a link: %s", got)
	}
}
