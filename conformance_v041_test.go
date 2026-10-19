package tuimark_test

import (
	"testing"
)

// v0.4.1: version on <tui> is optional. A <tui> without it is the current
// language, version="3": no V014, the version="2" and version="3"
// vocabulary is accepted, and L008 and the dump's clipped apply.
func TestMissingVersionIsTheCurrentLanguage(t *testing.T) {
	const src = `<tui>
<keymap when="#t"><bind keys="j" action="move-next" label="next"/></keymap>
<screen id="main">
<table id="t" each="rows as r" key="r.k" height="3"><column title="V">{r.v}</column></table>
<scroll id="log" stick="bottom" height="2"><text>a</text></scroll>
<text id="wide" width="10">ABCDEFGHIJKLMN</text>
<hints/>
</screen>
</tui>`
	app := parseDoc(t, src)
	bindJSON(t, app, "rows", `[{"k":0,"v":"x"}]`)
	codes := map[string]int{}
	for _, d := range app.Validate() {
		codes[d.Code]++
	}
	if codes["L008"] != 1 || len(codes) != 1 {
		t.Errorf("diagnostics = %v, want exactly one L008 (the wide text)", codes)
	}
	d, err := app.Dump(80, 24)
	if err != nil {
		t.Fatal(err)
	}
	if n := dumpNode(d, "wide"); n == nil || n.Clipped != "text" {
		t.Errorf("wide = %+v, want clipped \"text\"", n)
	}
}

// v0.4.1: an explicit version keeps its meaning, so a version="1" document
// still reports the version="2" vocabulary, and a value other than 1, 2, or
// 3 is still V003 and read as version="1".
func TestExplicitVersionsKeepTheirMeaning(t *testing.T) {
	for _, c := range []struct {
		version string
		want    []string
	}{
		{"1", []string{"V001"}},
		{"4", []string{"V003", "V001"}},
		{"", []string{"V003", "V001"}},
	} {
		src := `<tui version="` + c.version + `"><screen><hints/></screen></tui>`
		got := map[string]bool{}
		for _, d := range parseDoc(t, src).Validate() {
			got[d.Code] = true
		}
		for _, code := range c.want {
			if !got[code] {
				t.Errorf("version=%q: diagnostics %v, want %s", c.version, got, code)
			}
		}
	}
}
