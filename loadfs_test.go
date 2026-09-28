package tuimark_test

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/abdul-hamid-achik/tuimark"
)

// SPEC v0.3 §21 test 75: LoadFS paths on an fstest.MapFS.

const loadfsDoc = `<tui version="1">
  <style src="%s"/>
  <screen>
    <text>hi</text>
  </screen>
</tui>`

func mapDoc(src string) []byte {
	return []byte(strings.Replace(loadfsDoc, "%s", src, 1))
}

// v006 returns the first V006 diagnostic's message, or "" if there is
// none.
func v006(t *testing.T, app *tuimark.App) string {
	t.Helper()
	for _, d := range app.Validate() {
		if d.Code == "V006" {
			return d.Msg
		}
	}
	return ""
}

func TestLoadFSStyleNextToDocument(t *testing.T) {
	fsys := fstest.MapFS{
		"app.tui":    {Data: mapDoc("theme.tcss")},
		"theme.tcss": {Data: []byte("")},
	}
	app, err := tuimark.LoadFS(fsys, "app.tui")
	if err != nil {
		t.Fatal(err)
	}
	if msg := v006(t, app); msg != "" {
		t.Errorf("V006 = %q, want none", msg)
	}
}

func TestLoadFSStyleInSubdirectory(t *testing.T) {
	fsys := fstest.MapFS{
		"app.tui":        {Data: mapDoc("sub/theme.tcss")},
		"sub/theme.tcss": {Data: []byte("")},
	}
	app, err := tuimark.LoadFS(fsys, "app.tui")
	if err != nil {
		t.Fatal(err)
	}
	if msg := v006(t, app); msg != "" {
		t.Errorf("V006 = %q, want none", msg)
	}
}

func TestLoadFSStyleReachedByDotDotStaysInside(t *testing.T) {
	fsys := fstest.MapFS{
		"sub/app.tui": {Data: mapDoc("../theme.tcss")},
		"theme.tcss":  {Data: []byte("")},
	}
	app, err := tuimark.LoadFS(fsys, "sub/app.tui")
	if err != nil {
		t.Fatal(err)
	}
	if msg := v006(t, app); msg != "" {
		t.Errorf("V006 = %q, want none", msg)
	}
}

func TestLoadFSStyleOutsideRoot(t *testing.T) {
	fsys := fstest.MapFS{
		"app.tui":    {Data: mapDoc("../theme.tcss")},
		"theme.tcss": {Data: []byte("")}, // outside fsys's own root; unreachable either way
	}
	app, err := tuimark.LoadFS(fsys, "app.tui")
	if err != nil {
		t.Fatal(err)
	}
	msg := v006(t, app)
	if !strings.Contains(msg, "outside the file system") {
		t.Errorf("V006 = %q, want %q", msg, "outside the file system")
	}
}

func TestLoadFSAbsoluteStyleSrc(t *testing.T) {
	fsys := fstest.MapFS{
		"app.tui": {Data: mapDoc("/theme.tcss")},
	}
	app, err := tuimark.LoadFS(fsys, "app.tui")
	if err != nil {
		t.Fatal(err)
	}
	msg := v006(t, app)
	if !strings.Contains(msg, "outside the file system") {
		t.Errorf("V006 = %q, want %q", msg, "outside the file system")
	}
}

func TestLoadFSMissingStyleSrc(t *testing.T) {
	fsys := fstest.MapFS{
		"app.tui": {Data: mapDoc("theme.tcss")},
	}
	app, err := tuimark.LoadFS(fsys, "app.tui")
	if err != nil {
		t.Fatal(err)
	}
	msg := v006(t, app)
	if !strings.Contains(msg, "cannot be read") {
		t.Errorf("V006 = %q, want it to mention \"cannot be read\"", msg)
	}
}

func TestLoadFSInvalidName(t *testing.T) {
	fsys := fstest.MapFS{
		"app.tui": {Data: mapDoc("theme.tcss")},
	}
	for _, name := range []string{"../x.tui", "/x.tui"} {
		if _, err := tuimark.LoadFS(fsys, name); !errors.Is(err, fs.ErrInvalid) {
			t.Errorf("LoadFS(%q) error = %v, want it to wrap fs.ErrInvalid", name, err)
		}
	}
}

func TestLoadFSMissingName(t *testing.T) {
	fsys := fstest.MapFS{}
	_, err := tuimark.LoadFS(fsys, "missing.tui")
	if err == nil {
		t.Fatal("want an error for a missing name")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("error = %v, want it to wrap fs.ErrNotExist (the fs error, not a wrapped fs.ErrInvalid)", err)
	}
}

// SPEC v0.3 §21 test 76: LoadFS(os.DirFS(dir), base) and Load(path) with
// the same data give identical Validate() output and identical dumps at
// 80×24 and 120×30, for every example under examples/.

// exampleDoc names the .tui entry file of an examples/ subdirectory that
// is not spelled app.tui.
var exampleDoc = map[string]string{
	"agent":   "agent.tui",
	"monitor": "studio.tui",
	"spike":   "inbox.tui",
}

func TestLoadFSEqualsLoadForEveryExample(t *testing.T) {
	entries, err := os.ReadDir("examples")
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := e.Name()
		base := exampleDoc[dir]
		if base == "" {
			base = "app.tui"
		}
		path := filepath.Join("examples", dir, base)
		if _, err := os.Stat(path); err != nil {
			continue // not a document directory (none currently, but be tolerant)
		}
		found++
		t.Run(dir, func(t *testing.T) {
			viaLoad, err := tuimark.Load(path)
			if err != nil {
				t.Fatalf("Load(%q): %v", path, err)
			}
			viaFS, err := tuimark.LoadFS(os.DirFS(filepath.Join("examples", dir)), base)
			if err != nil {
				t.Fatalf("LoadFS(%q, %q): %v", dir, base, err)
			}
			dataPath := filepath.Join("examples", dir, "sample.json")
			if raw, err := os.ReadFile(dataPath); err == nil {
				var v any
				if err := json.Unmarshal(raw, &v); err != nil {
					t.Fatalf("%s: %v", dataPath, err)
				}
				if err := viaLoad.Bind("", v); err != nil {
					t.Fatalf("Load Bind: %v", err)
				}
				if err := viaFS.Bind("", v); err != nil {
					t.Fatalf("LoadFS Bind: %v", err)
				}
			}
			if !reflect.DeepEqual(viaLoad.Validate(), viaFS.Validate()) {
				t.Errorf("Validate() differs:\nLoad:   %+v\nLoadFS: %+v", viaLoad.Validate(), viaFS.Validate())
			}
			for _, sz := range [][2]int{{80, 24}, {120, 30}} {
				d1, err1 := viaLoad.Dump(sz[0], sz[1])
				d2, err2 := viaFS.Dump(sz[0], sz[1])
				if err1 != nil || err2 != nil {
					t.Fatalf("%dx%d: Dump errors %v / %v", sz[0], sz[1], err1, err2)
				}
				if !reflect.DeepEqual(d1, d2) {
					t.Errorf("%dx%d: dumps differ", sz[0], sz[1])
				}
			}
		})
	}
	if found == 0 {
		t.Fatal("no example documents found under examples/")
	}
}
