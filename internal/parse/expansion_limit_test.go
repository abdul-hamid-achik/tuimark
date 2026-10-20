package parse_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// ADR 0034: an expansion that would insert more than ExpansionLimit
// elements is undone, reports V021 at the include being expanded, and no
// later include expands.

func texts(n int) string {
	return "<fragment>" + strings.Repeat("<text>a</text>", n) + "</fragment>"
}

func v021(doc *parse.Document) []string {
	var out []string
	for _, d := range doc.Diags {
		if d.Code == "V021" {
			out = append(out, fmt.Sprintf("%s:%d:%d %s", d.File, d.Line, d.Col, d.Msg))
		}
	}
	return out
}

func screenChildren(doc *parse.Document) int {
	return len(doc.Root.Children[0].Children)
}

// The boundary: the include counts one, so a fragment of ExpansionLimit-1
// elements makes exactly the limit, which is fine; one more is V021 and the
// include inserts nothing.
func TestExpansionLimitBoundary(t *testing.T) {
	if parse.ExpansionLimit != 100000 {
		t.Fatalf("ExpansionLimit = %d", parse.ExpansionLimit)
	}
	const app = "<tui>\n<screen>\n<include src=\"big.tui\"/>\n</screen>\n</tui>"
	doc := parse.ParseWith([]byte(app), "app.tui", memLoader(map[string]string{"big.tui": texts(parse.ExpansionLimit - 1)}, "app.tui", nil))
	if got := v021(doc); len(got) != 0 || screenChildren(doc) != parse.ExpansionLimit-1 {
		t.Errorf("at the limit: V021 %v, %d children", got, screenChildren(doc))
	}
	doc = parse.ParseWith([]byte(app), "app.tui", memLoader(map[string]string{"big.tui": texts(parse.ExpansionLimit)}, "app.tui", nil))
	want := `app.tui:3:1 include src="big.tui" exceeds the expansion limit (100000 elements or 16 MiB of placed source): it inserts nothing, and no later include or use expands`
	if got := v021(doc); len(got) != 1 || got[0] != want {
		t.Errorf("over the limit: V021 %v, want %s", got, want)
	}
	if n := screenChildren(doc); n != 0 {
		t.Errorf("the include inserted %d elements, want none", n)
	}
	// The include stays in the source IR, empty.
	if inc := doc.Root.Children[0].Source; len(inc) != 1 || inc[0].Kind != "include" || len(inc[0].Children) != 0 {
		t.Errorf("source: %+v", inc)
	}
}

// The enclosing include keeps what it had inserted, a later include does
// not expand, and there is no second V021.
func TestExpansionLimitKeepsTheEnclosingAndStopsTheRest(t *testing.T) {
	files := map[string]string{
		"a.tui":   `<fragment><text>1</text><text>2</text><include src="big.tui"/><text>3</text></fragment>`,
		"big.tui": texts(parse.ExpansionLimit),
		"c.tui":   `<fragment><text>late</text><include src="big.tui"/></fragment>`,
	}
	doc := parse.ParseWith([]byte("<tui>\n<screen>\n<include src=\"a.tui\"/>\n<include src=\"c.tui\"/>\n<include src=\"big.tui\"/>\n</screen>\n</tui>"), "app.tui", memLoader(files, "app.tui", nil))
	got := v021(doc)
	if len(got) != 1 || !strings.HasPrefix(got[0], `a.tui:1:39 include src="big.tui" exceeds`) {
		t.Fatalf("V021 %v", got)
	}
	// 1, 2, and 3 stay (the first two were inserted before big.tui, the
	// third after it); c.tui and the last include expand nothing.
	var txt []string
	for _, c := range doc.Root.Children[0].Children {
		txt = append(txt, c.Text)
	}
	if strings.Join(txt, "") != "123" {
		t.Errorf("texts %v, want 1 2 3", txt)
	}
	// The later includes report nothing else for it.
	if len(doc.Diags) != 1 {
		t.Errorf("diagnostics: %s", diagsOf(doc))
	}
}

// A fan-out of plain files, no symlinks: thirty fragments that include the
// next twice each. It returns at once with exactly one V021, at the include
// that was expanding when the limit went.
func TestExpansionLimitFanOut(t *testing.T) {
	files := map[string]string{"f30.tui": "<fragment><text>x</text></fragment>"}
	for i := 0; i < 30; i++ {
		files[fmt.Sprintf("f%d.tui", i)] = fmt.Sprintf("<fragment><include src=\"f%d.tui\"/><include src=\"f%d.tui\"/></fragment>", i+1, i+1)
	}
	start := time.Now()
	doc := parse.ParseWith([]byte("<tui><screen><include src=\"f0.tui\"/></screen></tui>"), "app.tui", memLoader(files, "app.tui", nil))
	if d := time.Since(start); d > 5*slowdown*time.Second {
		t.Errorf("took %v", d)
	}
	got := v021(doc)
	if len(got) != 1 {
		t.Errorf("V021 %v, want exactly one", got)
	}
	// What stays plus the includes that expanded is at most the limit.
	if n := screenChildren(doc); n <= 0 || n >= parse.ExpansionLimit {
		t.Errorf("%d elements inserted", n)
	}
}

// Includes count too, so a fan-out of fragments that hold no element at all
// is bounded: thirty levels of two includes each, a leaf of <fragment/>.
func TestExpansionLimitCountsIncludes(t *testing.T) {
	files := map[string]string{"f30.tui": "<fragment/>"}
	for i := 0; i < 30; i++ {
		files[fmt.Sprintf("f%d.tui", i)] = fmt.Sprintf("<fragment><include src=\"f%d.tui\"/><include src=\"f%d.tui\"/></fragment>", i+1, i+1)
	}
	start := time.Now()
	doc := parse.ParseWith([]byte("<tui><screen><include src=\"f0.tui\"/></screen></tui>"), "app.tui", memLoader(files, "app.tui", nil))
	if d := time.Since(start); d > 5*slowdown*time.Second {
		t.Errorf("took %v", d)
	}
	if got := v021(doc); len(got) != 1 {
		t.Errorf("V021 %v, want exactly one", got)
	}
}

// The byte budget (ADR 0034, second amendment): each include is charged its
// fragment's size before anything is built. A fragment of exactly 1 MiB
// placed 16 times is exactly the budget and fine; the 17th placement is
// V021 and inserts nothing, and the ones before it stay.
func TestExpansionByteBudgetBoundary(t *testing.T) {
	if parse.ExpansionByteLimit != 16<<20 {
		t.Fatalf("ExpansionByteLimit = %d", parse.ExpansionByteLimit)
	}
	const head, tail = "<fragment><text>a</text><!--", "--></fragment>"
	frag := head + strings.Repeat("x", 1<<20-len(head)-len(tail)) + tail
	if len(frag) != 1<<20 {
		t.Fatalf("fragment is %d bytes", len(frag))
	}
	place := func(n int) *parse.Document {
		return parse.ParseWith([]byte("<tui><screen>"+strings.Repeat("<include src=\"f.tui\"/>", n)+"</screen></tui>"), "app.tui", memLoader(map[string]string{"f.tui": frag}, "app.tui", nil))
	}
	doc := place(16)
	if got := v021(doc); len(got) != 0 || screenChildren(doc) != 16 {
		t.Errorf("at the budget: V021 %v, %d children", got, screenChildren(doc))
	}
	doc = place(17)
	if got := v021(doc); len(got) != 1 || !strings.Contains(got[0], `exceeds the expansion limit (100000 elements or 16 MiB of placed source): it inserts nothing, and no later include or use expands`) {
		t.Errorf("over the budget: V021 %v", got)
	}
	if n := screenChildren(doc); n != 16 {
		t.Errorf("%d children, want the 16 placed before the 17th", n)
	}
}
