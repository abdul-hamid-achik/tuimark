package parse

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// The regressions of the F2 final check that need the package's insides:
// the dedupe before formatting keeps exactly what dedupeDiags keeps, a
// use's classes are merged once, and the byte budget does not depend on
// the file name.

// osLoader is a Loader over the files under the directory of doc, as Load
// resolves them.
func osLoader(doc string) *Loader {
	dir := filepath.Dir(doc)
	return &Loader{
		Join: func(from, src string) (string, bool) {
			d := dir
			if from != "" {
				d = filepath.Dir(from)
			}
			return filepath.Join(d, src), false
		},
		Read:  os.ReadFile,
		Canon: filepath.Clean,
		Name: func(p string) string {
			r, err := filepath.Rel(dir, p)
			if err != nil {
				return p
			}
			return filepath.ToSlash(r)
		},
		Self: filepath.Clean(doc),
	}
}

// Every document of the repository with templates or includes, and copies
// that report one written element many times, gives the same diagnostics
// with and without the dedupe before formatting (builder.reported).
func TestEarlyDedupeKeepsWhatDedupeDiagsKeeps(t *testing.T) {
	type doc struct {
		name string
		src  []byte
		ld   *Loader
	}
	var docs []doc
	for _, root := range []string{"../../testdata", "../../examples"} {
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(p, ".tui") {
				return err
			}
			src, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			if strings.Contains(string(src), "<use") || strings.Contains(string(src), "<include") {
				docs = append(docs, doc{p, src, osLoader(p)})
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(docs) < 10 {
		t.Fatalf("only %d documents with composition", len(docs))
	}
	long := "a" + strings.Repeat("x", 300)
	uses := strings.Repeat("<use template=\"T\" with=\"r as x\"/>\n", 300)
	for i, body := range []string{
		`<text focusable="true" wrap="bogus">a</text><scroll stick="bottom"><text>b</text></scroll>`,
		`<col each="x.v as r"><text>{x.a}</text></col><input/><text on:click="go">c</text>`,
		`<list each="x.v as r"><item><text>{r.a}</text></item></list><text id="dup">d</text><text id="dup">e</text>`,
	} {
		src := "<tui>\n<template id=\"T\">" + body + "</template>\n<screen><col id=\"" + long + "\" each=\"rows as r\">\n" + uses + "</col></screen>\n</tui>\n"
		docs = append(docs, doc{"synthetic " + string(rune('a'+i)), []byte(src), nil})
	}
	t.Cleanup(func() { earlyDedupe = true })
	for _, d := range docs {
		earlyDedupe = true
		on := ParseWith(d.src, d.name, d.ld).Diags
		earlyDedupe = false
		off := ParseWith(d.src, d.name, d.ld).Diags
		if !reflect.DeepEqual(on, off) {
			t.Errorf("%s: with the early dedupe\n%v\nwithout\n%v", d.name, on, off)
		}
	}
}

// A use's classes land once on each element of its copy, after what the
// element has, each name once and in order (decision 101), whatever the
// nesting: they are merged when the expansion ends, from CopiedBy.
func TestNestedUseClassesEachOnceInOrder(t *testing.T) {
	d := Parse([]byte(`<tui>
<template id="A"><text class="a b">x</text><text>y</text></template>
<template id="B"><use template="A" class="b c"/></template>
<template id="C"><use template="B" class="c d a"/></template>
<screen><use template="C" class="e d"/><use template="A"/></screen>
</tui>`), "app.tui")
	if len(d.Diags) != 0 {
		t.Fatalf("diagnostics: %v", d.Diags)
	}
	var got [][]string
	for _, s := range d.Screens {
		for _, c := range s.Children {
			got = append(got, c.Classes)
		}
	}
	want := [][]string{{"a", "b", "c", "d", "e"}, {"b", "c", "d", "a", "e"}, {"a", "b"}, nil}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("classes %q, want %q", got, want)
	}
}

// B006 quotes a list's or a table's each as written, as V011 does: an
// element of a copy holds the path its use's with gives, which may be long
// and is repeated in every element that names the alias.
func TestB006QuotesTheEachAsWritten(t *testing.T) {
	path := "d." + strings.Repeat("x", 1000)
	d := Parse([]byte(`<tui>
<template id="T"><list id="l" each="a.v as r"><item><text>{r}</text></item></list><table id="t" each="a.w as r"><column>{r}</column></table></template>
<screen><use id="u" template="T" with="`+path+` as a"/></screen>
</tui>`), "app.tui")
	var got []string
	for _, x := range d.Diags {
		got = append(got, x.Code+" "+x.Msg)
	}
	want := []string{
		`B006 <list each="a.v as r"> has no key=; selection follows the index, placed by <use id="u"> at app.tui:3:9`,
		`B006 <table each="a.w as r"> has no key=; the cursor follows the row index, placed by <use id="u"> at app.tui:3:9`,
	}
	if !slices.Equal(got, want) {
		t.Errorf("diagnostics:\n%q\nwant\n%q", got, want)
	}
}

// The byte budget charges an element a copy places the descriptor of its
// placement suffix, never its location (the F2 final check): a document
// at the budget validates the same under a short and a 200-byte file
// name, and its diagnostics differ only in the file name.
func TestByteBudgetIgnoresTheFileName(t *testing.T) {
	const head, tail = `<template id="t"><input/><!--`, "--></template>"
	suffix := len(`, placed by <use template="t">`)
	tmpl := head + strings.Repeat("x", 1<<20-suffix-len(head)-len(tail)) + tail
	doc := func(tmpl string) []byte {
		return []byte("<tui>" + tmpl + "<screen>" + strings.Repeat(`<use template="t"/>`, 16) + "</screen></tui>")
	}
	longName := strings.Repeat("n", 200) + ".tui"
	masked := func(src []byte, name string) []string {
		var out []string
		for _, d := range Parse(src, name).Diags {
			out = append(out, d.Code+" "+strings.ReplaceAll(d.File+" "+d.Msg, name, "FILE"))
		}
		return out
	}
	short, long := masked(doc(tmpl), "app.tui"), masked(doc(tmpl), longName)
	if !slices.Equal(short, long) || len(short) != 16 || !strings.HasPrefix(short[0], "V012 FILE ") || !strings.Contains(short[0], `, placed by <use template="t"> at FILE:1:`) {
		t.Errorf("at the budget:\n%q\n%q", short, long)
	}
	over := doc(head + "x" + tmpl[len(head):])
	if short, long := masked(over, "app.tui"), masked(over, longName); !slices.Equal(short, long) || !slices.ContainsFunc(short, func(s string) bool { return strings.HasPrefix(s, "V021 ") }) {
		t.Errorf("one byte over the budget:\n%q\n%q", short, long)
	}
}
