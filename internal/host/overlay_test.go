package host

import (
	"os"
	"path/filepath"
	"testing"
)

const overlayDoc = `<tui version="1"><style src="../theme.tcss"/><screen id="s"><text>x</text></screen></tui>` + "\n"

// LoadBuffers is Load over open buffers (SPEC v0.4a §15.12): a stylesheet
// that is open is read from the buffer, one that is not from disk, and a
// src climbs with .. as Load's does.
func TestLoadBuffersReadsOpenStylesheets(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	sheet := filepath.Join(dir, "theme.tcss")
	if err := os.WriteFile(sheet, []byte("screen { color: red; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	docPath := filepath.Join(dir, "sub", "app.tui")
	abs, _ := filepath.Abs(sheet)

	// From disk.
	app, read := LoadBuffers(docPath, []byte(overlayDoc), func(string) ([]byte, bool) { return nil, false })
	if len(read) != 1 || read[0].Path != abs || string(read[0].Text) != "screen { color: red; }\n" {
		t.Fatalf("read = %+v", read)
	}
	if d := app.Validate(); len(d) != 0 {
		t.Errorf("disk: %v", d)
	}

	// From the open buffer, whose text disk does not have: the diagnostic
	// names the stylesheet by its absolute path, the document by its base name.
	app, read = LoadBuffers(docPath, []byte(overlayDoc), func(p string) ([]byte, bool) {
		if p == abs {
			return []byte("screen { bogus-prop: 1; }\n"), true
		}
		return nil, false
	})
	if len(read) != 1 || string(read[0].Text) != "screen { bogus-prop: 1; }\n" {
		t.Fatalf("read = %+v", read)
	}
	diags := app.Validate()
	if len(diags) != 1 || diags[0].Code != "V003" || diags[0].File != abs || diags[0].Line != 1 || diags[0].Col != 10 {
		t.Errorf("buffer: %v", diags)
	}

	// A document diagnostic keeps the base name.
	app, _ = LoadBuffers(docPath, []byte(`<tui version="1"><screen id="s"><bogus/></screen></tui>`), func(string) ([]byte, bool) { return nil, false })
	if d := app.Validate(); len(d) != 1 || d[0].File != "app.tui" {
		t.Errorf("document: %v", d)
	}

	// A stylesheet that cannot be read is V006 on the document, as Load says.
	if err := os.Remove(sheet); err != nil {
		t.Fatal(err)
	}
	app, read = LoadBuffers(docPath, []byte(overlayDoc), func(string) ([]byte, bool) { return nil, false })
	d := app.Validate()
	if len(read) != 0 || len(d) != 1 || d[0].Code != "V006" || d[0].File != "app.tui" {
		t.Errorf("missing: read %+v, %v", read, d)
	}
	if err := os.WriteFile(docPath, []byte(overlayDoc), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(docPath)
	if err != nil {
		t.Fatal(err)
	}
	if want := loaded.Validate(); len(want) != 1 || want[0].String() != d[0].String() {
		t.Errorf("Load says %v, LoadBuffers says %v", want, d)
	}
}
