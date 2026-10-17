package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A manifest entry's name is its golden directory (SPEC §15.5), so it must
// be one clean path segment no earlier entry uses. Otherwise a non-frozen
// entry could make --update write over a frozen entry's goldens (a
// duplicate name) or outside DIR altogether ("../x", "a/b", "a\b").
func TestGoldenManifestNamesAreSingleUniqueSegments(t *testing.T) {
	for _, bad := range []struct{ name, why string }{
		{"", "empty"},
		{".", "dot"},
		{"..", "dotdot"},
		{"../outside", "traversal"},
		{"a/b", "slash"},
		{"a/", "trailing slash"},
		{"/abs", "absolute"},
		{`a\\b`, "backslash"}, // JSON escape: the name is a\b
		{"./a", "unclean"},
	} {
		t.Run(bad.why, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, "g")
			tui := filepath.Join(root, "app.tui")
			mkdir(t, dir)
			writeFile(t, tui, `<tui version="1"><screen id="main"><text id="t">hello</text></screen></tui>`)
			writeManifest(t, dir, `[{"name":"`+bad.name+`","file":"`+tui+`","sizes":["20x3"],"json":["20x3"],"frozen":false}]`)

			code, out, errw := runCLI("test", dir, "--update")
			if code != 1 || !strings.HasPrefix(out, "FAIL "+strings.ReplaceAll(bad.name, `\\`, `\`)+": manifest entry name ") {
				t.Errorf("exit %d, want 1 and a manifest-name FAIL line; stdout=%q stderr=%q", code, out, errw)
			}
			// Nothing was written anywhere under root but the manifest and the document.
			var written []string
			filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
				if err == nil && !d.IsDir() {
					written = append(written, filepath.Base(p))
				}
				return nil
			})
			if len(written) != 2 {
				t.Errorf("--update wrote files for a bad name: %v", written)
			}
		})
	}
}

func TestGoldenManifestDuplicateNameCannotOverwriteAFrozenEntry(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "g")
	mkdir(t, dir)
	frozenDoc := filepath.Join(root, "frozen.tui")
	otherDoc := filepath.Join(root, "other.tui")
	writeFile(t, frozenDoc, `<tui version="1"><screen id="main"><text id="t">frozen</text></screen></tui>`)
	writeFile(t, otherDoc, `<tui version="1"><screen id="main"><text id="t">other</text></screen></tui>`)
	// Seed the frozen entry's goldens through a non-frozen manifest, then freeze it.
	writeManifest(t, dir, `[{"name":"e","file":"`+frozenDoc+`","sizes":["20x3"],"json":["20x3"]}]`)
	if code, _, errw := runCLI("test", dir, "--update"); code != 0 {
		t.Fatalf("seed: exit %d; %s", code, errw)
	}
	before := map[string]string{}
	for _, ext := range []string{".txt", ".json"} {
		b, err := os.ReadFile(filepath.Join(dir, "e", "20x3"+ext))
		if err != nil {
			t.Fatal(err)
		}
		before[ext] = string(b)
	}
	// "E" differs from "e" only in case: the same directory on a
	// case-insensitive file system.
	for _, dup := range []string{"e", "E"} {
		writeManifest(t, dir, `[{"name":"e","file":"`+frozenDoc+`","sizes":["20x3"],"json":["20x3"],"frozen":true},`+
			`{"name":"`+dup+`","file":"`+otherDoc+`","sizes":["20x3"],"json":["20x3"],"frozen":false}]`)
		code, out, errw := runCLI("test", dir, "--update", "--allow-breaking")
		if code != 1 || !strings.Contains(out, "PASS e 20x3 (json)") ||
			!strings.Contains(out, "FAIL "+dup+": manifest entry name is already used by an earlier entry") {
			t.Errorf("dup %q: exit %d, want 1 with the frozen entry passing and the duplicate refused; stdout=%q stderr=%q", dup, code, out, errw)
		}
		for ext, want := range before {
			b, err := os.ReadFile(filepath.Join(dir, "e", "20x3"+ext))
			if err != nil || string(b) != want {
				t.Errorf("dup %q: the frozen golden 20x3%s changed (%v)", dup, ext, err)
			}
		}
	}
}

func mkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}
