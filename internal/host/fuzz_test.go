package host

import (
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// FuzzRender checks the invariants every dump must keep, whatever the
// document: no panic, grid[y] has exactly cols runes, no control
// characters, and ok=false exactly when an error was reported.
func FuzzRender(f *testing.F) {
	for _, p := range []string{"../../examples/spike/inbox.tui", "../../examples/inbox/app.tui", "../../examples/stacked/app.tui"} {
		if src, err := os.ReadFile(p); err == nil {
			f.Add(string(src))
		}
	}
	f.Add(v1(`<col><text>{a}</text><list id="l" each="rows as r"><item><text>{r}</text></item></list><input id="q" bind="a"/></col><modal id="m" open="true" title="t"><button id="b">x</button></modal>`))
	f.Add(v1(`<row style="gap: 4; padding: 1 2 3 4"><box style="dock: left; width: 50%"/><scroll axis="both"><text wrap="wrap">a b c</text></scroll><progress value="50"/><rule axis="y"/><spacer/></row>`))
	f.Fuzz(func(t *testing.T, src string) {
		a := doc(t, src)
		_ = a.Bind("", map[string]any{"a": "x", "rows": []any{"1", "2"}})
		for _, size := range [][2]int{{40, 10}, {1, 1}, {0, 0}} {
			d := a.Dump(size[0], size[1], false)
			if len(d.Grid) != size[1] {
				t.Fatalf("%v: %d rows", size, len(d.Grid))
			}
			for _, row := range d.Grid {
				if utf8.RuneCountInString(row) != size[0] {
					t.Fatalf("%v: row %q", size, row)
				}
				if strings.IndexFunc(row, func(r rune) bool { return r < 0x20 || (r >= 0x7f && r <= 0x9f) }) >= 0 {
					t.Fatalf("%v: control character in %q", size, row)
				}
			}
			if d.OK == ir.Diags(d.Errors).HasErrors() {
				t.Fatalf("ok=%v with errors %v", d.OK, d.Errors)
			}
		}
		for _, k := range DecodeKeys([]byte("\t\x1b[Bab\r\x1b\x7f")) {
			a.HandleKey(k)
			a.Frame(40, 10)
		}
		_ = a.Validate()
	})
}
