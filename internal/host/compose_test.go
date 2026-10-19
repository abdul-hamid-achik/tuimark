package host

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

// SPEC v0.4b §6.18 (phase F1): includes through the loaders of the host.

func diagLines(a *App) []string {
	var out []string
	for _, d := range a.Validate() {
		out = append(out, fmt.Sprintf("%s %s:%d:%d %s", d.Code, d.File, d.Line, d.Col, d.Msg))
	}
	return out
}

func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, text := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// The three loaders give a document the same diagnostics, with the same
// file names, and a fragment's stylesheet is named by its path relative to
// the document's directory (decision 4) while the document's own keeps its
// base name.
func TestIncludeThroughEveryLoader(t *testing.T) {
	files := map[string]string{
		"ui/app.tui":        "<tui>\n  <style src=\"theme.tcss\"/>\n  <include src=\"kit/ui.tui\"/>\n</tui>\n",
		"ui/kit/ui.tui":     "<fragment>\n  <style src=\"theme.tcss\"/>\n  <screen id=\"s\"><text id=\"t\" bogus=\"1\">x</text></screen>\n</fragment>\n",
		"ui/theme.tcss":     "text { color: $nope; }\n",
		"ui/kit/theme.tcss": "text { bold: bogus; }\n",
	}
	dir := writeTree(t, files)
	want := []string{
		// Sorted by file name, in byte order.
		`V003 kit/theme.tcss:1:8 bold: bad value "bogus" (want true or false)`,
		`V002 kit/ui.tui:3:31 attribute "bogus" is not allowed on <text>`,
		`V003 theme.tcss:1:8 color: unknown token $nope`,
	}
	byLoad, err := Load(filepath.Join(dir, "ui/app.tui"))
	if err != nil {
		t.Fatal(err)
	}
	mem := fstest.MapFS{}
	for name, text := range files {
		mem[name] = &fstest.MapFile{Data: []byte(text)}
	}
	byFS, err := LoadFS(mem, "ui/app.tui")
	if err != nil {
		t.Fatal(err)
	}
	for name, a := range map[string]*App{"Load": byLoad, "LoadFS": byFS} {
		if got := diagLines(a); strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Errorf("%s:\n%s\nwant\n%s", name, strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
	}
	// Files: the document, the fragment, and both stylesheets (for --watch).
	got := byLoad.Files()
	var names []string
	for _, f := range got {
		names = append(names, strings.TrimPrefix(f, dir+string(filepath.Separator)))
	}
	if strings.Join(names, " ") != strings.Join([]string{"ui/app.tui", "ui/kit/ui.tui", "ui/theme.tcss", "ui/kit/theme.tcss"}, " ") {
		t.Errorf("Files: %v", names)
	}
}

// LoadBuffers reads a fragment from the editor's open buffer when there is
// one, names it by its absolute path (the language server routes by file),
// and keeps fragments out of the stylesheets it returns.
func TestLoadBuffersReadsFragmentsFromBuffers(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"app.tui":   "<tui>\n  <screen id=\"s\"><include src=\"x.tui\"/></screen>\n</tui>\n",
		"x.tui":     "<fragment><text>on disk</text></fragment>",
		"look.tcss": "text { bold: true; }\n",
	})
	abs, _ := filepath.Abs(filepath.Join(dir, "x.tui"))
	app, sheets := LoadBuffers(filepath.Join(dir, "app.tui"), []byte("<tui>\n  <screen id=\"s\"><include src=\"x.tui\"/></screen>\n</tui>\n"), func(p string) ([]byte, bool) {
		if p == abs {
			return []byte("<fragment><text bogus=\"1\">in the editor</text></fragment>"), true
		}
		return nil, false
	})
	var got []string
	for _, d := range app.Validate() {
		got = append(got, d.Code+" "+d.File)
	}
	if strings.Join(got, "\n") != "V002 "+abs {
		t.Errorf("diagnostics %v, want V002 in %s", got, abs)
	}
	if len(sheets) != 0 {
		t.Errorf("stylesheets read: %v", sheets)
	}
	if n := app.Document().Root.Children[0].Children[0].Text; n != "in the editor" {
		t.Errorf("the fragment text is %q", n)
	}
}

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
		if len(v) != 1 || v[0] != `include src="f30.tui" exceeds the expansion limit of 100000 elements: it inserts nothing, and no later include or use expands` {
			t.Errorf("V021: %v", v)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("LoadBuffers hangs on the fan-out")
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
