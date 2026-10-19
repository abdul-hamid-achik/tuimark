package parse_test

import (
	"fmt"
	"io/fs"
	"path"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// memLoader is a Loader over an in-memory file system, the way LoadFS
// resolves: slash paths, "" for the document at doc. reads counts the files
// read.
func memLoader(files map[string]string, doc string, reads *[]string) *parse.Loader {
	fsys := fstest.MapFS{}
	for name, text := range files {
		fsys[name] = &fstest.MapFile{Data: []byte(text)}
	}
	dir := path.Dir(doc)
	return &parse.Loader{
		Join: func(from, src string) (string, bool) {
			if path.IsAbs(src) {
				return src, true
			}
			d := dir
			if from != "" {
				d = path.Dir(from)
			}
			j := path.Join(d, src)
			return j, j == ".." || strings.HasPrefix(j, "../")
		},
		Read: func(p string) ([]byte, error) {
			if reads != nil {
				*reads = append(*reads, p)
			}
			return fs.ReadFile(fsys, p)
		},
		Canon: path.Clean,
		Name: func(p string) string {
			if dir == "." {
				return p
			}
			return strings.TrimPrefix(p, dir+"/")
		},
		Self: path.Clean(doc),
	}
}

func diagsOf(doc *parse.Document) string {
	var out []string
	for _, d := range doc.Diags {
		out = append(out, fmt.Sprintf("%s %s:%d:%d %s", d.Code, d.File, d.Line, d.Col, d.Msg))
	}
	return strings.Join(out, "\n")
}

// A pinned document never reads the file an include names (decision 23):
// the include is the V001 of its hint and nothing else.
func TestPinnedDocumentNeverReadsAnInclude(t *testing.T) {
	var reads []string
	files := map[string]string{"x.tui": "<fragment><text>a</text></fragment>"}
	for _, v := range []string{"1", "2", "3"} {
		doc := parse.ParseWith([]byte(`<tui version="`+v+`"><screen><include src="x.tui"/></screen></tui>`), "app.tui", memLoader(files, "app.tui", &reads))
		want := "V001 app.tui:1:" + fmt.Sprint(len(`<tui version="`+v+`"><screen>`)+1) + " tag <include> (requires a <tui> without version)"
		if got := diagsOf(doc); got != want {
			t.Errorf("version=%s: %s\nwant %s", v, got, want)
		}
		if len(doc.Root.Children[0].Children) != 0 {
			t.Errorf("version=%s: the include inserted nodes", v)
		}
	}
	if len(reads) != 0 {
		t.Errorf("a pinned document read %v", reads)
	}
	if doc := parse.ParseWith([]byte(`<app><include src="x.tui"/></app>`), "app.tui", memLoader(files, "app.tui", &reads)); len(reads) != 0 || !strings.Contains(diagsOf(doc), "unknown tag <include>") {
		t.Errorf("the spike root read %v: %s", reads, diagsOf(doc))
	}
}

// Parse has no loader: an include is V006, and nothing is read.
func TestParseWithoutALoader(t *testing.T) {
	doc := parse.Parse([]byte(`<tui><screen><include src="x.tui"/></screen></tui>`), "app.tui")
	if got := diagsOf(doc); !strings.HasPrefix(got, `V006 app.tui:1:14 include src="x.tui" cannot be read: `) {
		t.Errorf("%s", got)
	}
}

// An include stands where its fragment's elements may stand (SPEC v0.4b
// §6.18.1): the checks of the expanded parent apply, and the expanded
// document has no node for the include.
func TestIncludePlacementByExpandedParent(t *testing.T) {
	files := map[string]string{
		"tabs.tui":   `<fragment><tab id="t1" label="one"><text>1</text></tab><tab id="t2" label="two"><text>2</text></tab></fragment>`,
		"cols.tui":   `<fragment><column title="N">{r.n}</column><item class="x"/></fragment>`,
		"item.tui":   `<fragment><item><text>{r}</text></item></fragment>`,
		"screen.tui": `<fragment><screen id="s"><text>a</text></screen></fragment>`,
		"binds.tui":  `<fragment><bind keys="a" action="go"/><bind keys="b" action="stop"/></fragment>`,
		"text.tui":   `<fragment><text>a</text></fragment>`,
	}
	for _, c := range []struct {
		name, src string
		want      string // diagnostics
	}{
		{"tabs", `<tui><screen><tabs id="nav"><include src="tabs.tui"/></tabs></screen></tui>`, ""},
		{"table", `<tui><screen><table id="tb" each="rows as r" key="r.n"><include src="cols.tui"/></table></screen></tui>`, ""},
		{"list", `<tui><screen><list id="l" each="rs as r" key="r"><include src="item.tui"/></list></screen></tui>`, ""},
		{"screen under tui", `<tui><include src="screen.tui"/></tui>`, ""},
		{"keymap", `<tui><keymap><include src="binds.tui"/></keymap><screen/></tui>`, ""},
		{"screen under col", `<tui><screen><col><include src="screen.tui"/></col></screen></tui>`,
			"V001 screen.tui:1:11 <screen> must be a direct child of <tui>, placed by <include> at app.tui:1:19"},
		{"text under tabs", `<tui><screen><tabs id="nav"><include src="text.tui"/></tabs></screen></tui>`,
			"V016 text.tui:1:11 <tabs> children must be <tab> elements, got <text>, placed by <include> at app.tui:1:29\nV017 app.tui:1:14 <tabs> needs at least one <tab>"},
	} {
		t.Run(c.name, func(t *testing.T) {
			doc := parse.ParseWith([]byte(c.src), "app.tui", memLoader(files, "app.tui", nil))
			got := strings.Join(strings.Split(diagsOf(doc), "\n"), "\n")
			// Ignore the screen/V005 noise of documents that hold none.
			var keep []string
			for _, l := range strings.Split(got, "\n") {
				if l != "" && !strings.Contains(l, "document has no <screen>") {
					keep = append(keep, l)
				}
			}
			if strings.Join(keep, "\n") != c.want {
				t.Errorf("diagnostics:\n%s\nwant\n%s", strings.Join(keep, "\n"), c.want)
			}
			var walk func(n *ir.Node)
			walk = func(n *ir.Node) {
				if n.Kind == "include" {
					t.Errorf("an include node in the expanded tree at %s", n.Path)
				}
				for _, ch := range n.Children {
					walk(ch)
				}
			}
			walk(doc.Root)
		})
	}
}

// V004 stands at the second occurrence in document order, across files,
// whether that occurrence is an element of the tree or of an unknown tag's
// subtree (which is validated and kept out of the tree).
func TestDuplicateIDAcrossFilesInDocumentOrder(t *testing.T) {
	files := map[string]string{
		"w.tui": `<fragment><widget><text id="d"/></widget></fragment>`,
		"t.tui": `<fragment><text id="d">b</text></fragment>`,
	}
	// The text is first in the document, the unknown widget's child second;
	// the child's line in its own file is smaller than the text's line.
	doc := parse.ParseWith([]byte("<tui>\n<screen>\n<text id=\"d\">a</text>\n<include src=\"w.tui\"/>\n</screen>\n</tui>"), "app.tui", memLoader(files, "app.tui", nil))
	var v004 []string
	for _, d := range doc.Diags {
		if d.Code == "V004" {
			v004 = append(v004, fmt.Sprintf("%s:%d:%d %s", d.File, d.Line, d.Col, d.Msg))
		}
	}
	// (The widget is V001 too; the elements inside it are validated all the
	// same.)
	if want := `w.tui:1:19 duplicate id "d" (first defined at app.tui:3:1)`; len(v004) != 1 || v004[0] != want {
		t.Errorf("V004: %v, want %s", v004, want)
	}
	// An element of the document that repeats an id from a fragment is
	// reported in the document, with the file of the first occurrence.
	doc = parse.ParseWith([]byte("<tui>\n<screen>\n<include src=\"t.tui\"/>\n<text id=\"d\">a</text>\n</screen>\n</tui>"), "app.tui", memLoader(files, "app.tui", nil))
	var got []string
	for _, d := range doc.Diags {
		if d.Code == "V004" {
			got = append(got, fmt.Sprintf("%s:%d:%d %s", d.File, d.Line, d.Col, d.Msg))
		}
	}
	want := `app.tui:4:1 duplicate id "d" (first defined at t.tui:1:11, placed by <include> at app.tui:3:1)`
	if len(got) != 1 || got[0] != want {
		t.Errorf("V004: %v, want %s", got, want)
	}
}

// Includes nest, each resolving against its own file; the same fragment
// may be included twice when no include of it is on its own chain; the
// chain names the files in a cycle; files are listed once, in read order.
func TestNestedIncludesAndFragmentList(t *testing.T) {
	files := map[string]string{
		"parts/a.tui":      `<fragment><include src="deep/b.tui"/><text>a</text></fragment>`,
		"parts/deep/b.tui": `<fragment><text>b</text><include src="../c.tui"/></fragment>`,
		"parts/c.tui":      `<fragment><text>c</text></fragment>`,
	}
	var reads []string
	doc := parse.ParseWith([]byte(`<tui><screen><include src="parts/a.tui"/><include src="parts/c.tui"/></screen></tui>`), "app.tui", memLoader(files, "app.tui", &reads))
	if got := diagsOf(doc); got != "" {
		t.Errorf("diagnostics: %s", got)
	}
	var texts []string
	for _, c := range doc.Root.Children[0].Children {
		texts = append(texts, c.Text)
	}
	if strings.Join(texts, "") != "bcac" {
		t.Errorf("texts %v, want b c a c", texts)
	}
	var frags []string
	for _, f := range doc.Fragments {
		frags = append(frags, f.Name)
	}
	if strings.Join(frags, " ") != "parts/a.tui parts/deep/b.tui parts/c.tui" {
		t.Errorf("fragments: %v", frags)
	}
	// The IR keeps the includes as written.
	b := parse.BuildIR(doc, nil)
	screen := b.Root.Children[0]
	if len(screen.Children) != 2 || screen.Children[0].Kind != "include" || screen.Children[0].Children[0].Kind != "include" || screen.Children[0].Children[0].From != "parts/a.tui:1" {
		t.Errorf("IR: %+v", screen.Children)
	}
}

// The check order of SPEC v0.4b §6.18.2, one case each.
func TestIncludeCheckOrder(t *testing.T) {
	files := map[string]string{
		"s.tcss": "x",
		"a.tui":  `<fragment><include src="a.tui"/></fragment>`,
	}
	for _, c := range []struct{ src, want string }{
		{"../up.tui", `V006 app.tui:1:14 include src="../up.tui" is outside the file system; root the fs.FS at a directory that holds both files, or copy the fragment inside it`},
		{"s.tcss", `V020 app.tui:1:14 include src="s.tcss" is a stylesheet: load it with <style src="s.tcss">`},
		{"nope.tcss", `V020 app.tui:1:14 include src="nope.tcss" is a stylesheet: load it with <style src="nope.tcss">`},
		{"a.tui", `V006 a.tui:1:11 include src="a.tui" is an include cycle: app.tui → a.tui → a.tui`},
	} {
		doc := parse.ParseWith([]byte(`<tui><screen><include src="`+c.src+`"/></screen></tui>`), "app.tui", memLoader(files, "app.tui", nil))
		if got := diagsOf(doc); !strings.HasPrefix(got, c.want) {
			t.Errorf("src=%q:\n%s\nwant\n%s", c.src, got, c.want)
		}
	}
}

// An include's own attributes and content: only src, no slots.
func TestIncludeAttributesAndContent(t *testing.T) {
	files := map[string]string{"x.tui": `<fragment><text>a</text></fragment>`}
	doc := parse.ParseWith([]byte(`<tui><screen><include src="x.tui" id="i" class:a="b"><text>no</text></include></screen></tui>`), "app.tui", memLoader(files, "app.tui", nil))
	got := diagsOf(doc)
	for _, s := range []string{
		`V002 app.tui:1:35 attribute "id" is not allowed on <include>`,
		`V002 app.tui:1:42 attribute "class:a" is not allowed on <include>`,
		`V013 app.tui:1:54 <include> takes no content; Tuimark has no slots`,
	} {
		if !strings.Contains(got, s) {
			t.Errorf("missing %q in:\n%s", s, got)
		}
	}
	// The fragment is still inserted: the include's own errors do not stop it.
	if n := len(doc.Root.Children[0].Children); n != 1 {
		t.Errorf("%d nodes inserted, want 1", n)
	}
}

// A fragment is named by its path relative to the document's directory;
// the document by its base name.
func TestFragmentNames(t *testing.T) {
	files := map[string]string{"ui/kit/k.tui": `<fragment><text bogus="1">a</text></fragment>`}
	doc := parse.ParseWith([]byte(`<tui><screen><include src="kit/k.tui"/></screen></tui>`), "ui/app.tui", memLoader(files, "ui/app.tui", nil))
	if got := diagsOf(doc); !strings.HasPrefix(got, `V002 kit/k.tui:1:17 attribute "bogus"`) || strings.Contains(got, "placed by") {
		t.Errorf("%s", got)
	}
}

// A self-check (an attribute the element's own tag rejects) is reported
// once per written element however many times a fragment is placed, with
// no placement suffix; a check that depends on the place is reported once
// per placement, with it (SPEC v0.4b §6.18.6).
func TestDiagnosticsOncePerWrittenElement(t *testing.T) {
	files := map[string]string{"r.tui": `<fragment><text bogus="1">a</text><button label="x"/></fragment>`}
	doc := parse.ParseWith([]byte("<tui><screen>\n<include src=\"r.tui\"/>\n<include src=\"r.tui\"/>\n</screen></tui>"), "app.tui", memLoader(files, "app.tui", nil))
	got := diagsOf(doc)
	want := strings.Join([]string{
		`V002 r.tui:1:17 attribute "bogus" is not allowed on <text>`,
		`V012 r.tui:1:35 <button> is focusable and needs an id, placed by <include> at app.tui:2:1`,
		`V012 r.tui:1:35 <button> is focusable and needs an id, placed by <include> at app.tui:3:1`,
	}, "\n")
	if got != want {
		t.Errorf("diagnostics:\n%s\nwant\n%s", got, want)
	}
}
