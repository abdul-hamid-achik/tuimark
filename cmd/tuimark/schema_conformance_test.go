package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- D8: schema/ir.v0.1.json, ir.v0.2.json, ir.v0.3.json, dump.v0.2.json --

const (
	irV01Schema   = "../../schema/ir.v0.1.json"
	irV02Schema   = "../../schema/ir.v0.2.json"
	irV03Schema   = "../../schema/ir.v0.3.json"
	dumpV02Schema = "../../schema/dump.v0.2.json"
)

func TestSchemaFilesAreValidJSON(t *testing.T) {
	for _, p := range []string{irV01Schema, irV02Schema, irV03Schema, dumpV02Schema} {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
	}
}

func TestIROutputValidatesAgainstV01Schema(t *testing.T) {
	tui, _ := writePlayFixture(t)
	code, out, errw := runCLI("ir", tui)
	if code != 0 {
		t.Fatalf("exit %d, want 0; stderr=%s", code, errw)
	}
	errs := validateSchema(t, irV01Schema, decodeJSON(t, []byte(out)))
	if len(errs) > 0 {
		t.Errorf("tuimark ir output does not validate against schema/ir.v0.1.json:\n%s", strings.Join(errs, "\n"))
	}
}

func TestDumpJSONValidatesAgainstDumpSchema(t *testing.T) {
	tui, data := writePlayFixture(t)
	for _, args := range [][]string{
		{"dump", tui, "--data", data, "--format", "json"},
		{"dump", tui, "--data", data, "--format", "json", "--cells"},
		{"dump", tui, "--data", data, "--format", "json", "--styles"},
		{"dump", tui, "--data", data, "--format", "json", "--cells", "--styles"},
	} {
		code, out, errw := runCLI(args...)
		if code != 0 {
			t.Fatalf("%v: exit %d; stderr=%s", args, code, errw)
		}
		errs := validateSchema(t, dumpV02Schema, decodeJSON(t, []byte(out)))
		if len(errs) > 0 {
			t.Errorf("%v: dump does not validate against schema/dump.v0.2.json:\n%s\n%s", args, strings.Join(errs, "\n"), out)
		}
	}
}

func TestPlayJSONValidatesAgainstDumpSchema(t *testing.T) {
	tui, data := writePlayFixture(t)
	for _, args := range [][]string{
		{"play", tui, "--data", data, "--input", "tab down", "--format", "json"},
		{"play", tui, "--data", data, "--input", "tab down", "--frames", "--format", "json"},
		{"play", tui, "--data", data, "--input", "tab down", "--cells", "--styles", "--frames", "--format", "json"},
	} {
		code, out, errw := runCLI(args...)
		if code != 0 {
			t.Fatalf("%v: exit %d; stderr=%s", args, code, errw)
		}
		errs := validateSchema(t, dumpV02Schema, decodeJSON(t, []byte(out)))
		if len(errs) > 0 {
			t.Errorf("%v: play output does not validate against schema/dump.v0.2.json:\n%s\n%s", args, strings.Join(errs, "\n"), out)
		}
	}
}

func TestGoldenManifestJSONFilesValidateAgainstDumpSchema(t *testing.T) {
	manifestDir := "../../testdata/golden"
	raw, err := os.ReadFile(filepath.Join(manifestDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var entries []goldenEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		for _, size := range e.JSON {
			p := filepath.Join(manifestDir, e.Name, size+".json")
			b, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			errs := validateSchema(t, dumpV02Schema, decodeJSON(t, b))
			if len(errs) > 0 {
				t.Errorf("%s does not validate against schema/dump.v0.2.json:\n%s", p, strings.Join(errs, "\n"))
			}
		}
	}
}

func TestDumpV02SchemaRejectsUnknownTopLevelField(t *testing.T) {
	base := decodeJSON(t, []byte(`{"cols":1,"rows":1,"ok":true,"focus":null,"errors":[],"nodes":[],"grid":["x"]}`))
	if errs := validateSchema(t, dumpV02Schema, base); len(errs) != 0 {
		t.Fatalf("a minimal valid dump must validate cleanly, got: %v", errs)
	}
	withExtra := decodeJSON(t, []byte(`{"cols":1,"rows":1,"ok":true,"focus":null,"errors":[],"nodes":[],"grid":["x"],"bogus":1}`))
	if errs := validateSchema(t, dumpV02Schema, withExtra); len(errs) == 0 {
		t.Fatal("an unknown top-level field must fail additionalProperties: false")
	}
}

// SPEC v0.3 §21 test 87 and §13.3: a version="3" document that clips
// (one L008, one L009) dumps and plays with "clipped" members that
// validate against the amended schema/dump.v0.2.json, and its text dump
// ends the clipped nodes' lines with " clipped".
func TestClippedDumpValidatesAndMarksText(t *testing.T) {
	dir := t.TempDir()
	tui := filepath.Join(dir, "clip.tui")
	writeFile(t, tui, `<tui version="3">
<screen id="main">
<text id="wide" width="10">ABCDEFGHIJKLMN</text>
<box id="short" style="height: 3"><text>one
two
three
four</text></box>
</screen>
</tui>`)
	for _, args := range [][]string{
		{"dump", tui, "--cols", "40", "--rows", "12", "--format", "json"},
		{"dump", tui, "--cols", "40", "--rows", "12", "--format", "json", "--cells", "--styles"},
		{"play", tui, "--cols", "40", "--rows", "12", "--input", "tab", "--format", "json"},
	} {
		code, out, errw := runCLI(args...)
		if code != 0 {
			t.Fatalf("%v: exit %d: %s", args, code, errw)
		}
		for _, want := range []string{`"clipped": "text"`, `"clipped": "children"`} {
			if !strings.Contains(out, want) {
				t.Errorf("%v: output lacks %s", args, want)
			}
		}
		if errs := validateSchema(t, dumpV02Schema, decodeJSON(t, []byte(out))); len(errs) > 0 {
			t.Errorf("%v: does not validate against schema/dump.v0.2.json:\n%s", args, strings.Join(errs, "\n"))
		}
	}
	code, out, errw := runCLI("dump", tui, "--cols", "40", "--rows", "12", "--format", "text")
	if code != 0 {
		t.Fatalf("text dump: exit %d: %s", code, errw)
	}
	marked := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		if f := strings.Fields(line); len(f) > 0 && strings.HasSuffix(line, " clipped") {
			marked[f[0]] = true
		}
	}
	if !marked["wide"] || !marked["short"] || len(marked) != 2 {
		t.Errorf("text dump marks %v clipped, want exactly wide and short:\n%s", marked, out)
	}
	bad := decodeJSON(t, []byte(`{"cols":1,"rows":1,"ok":true,"focus":null,"errors":[],"nodes":[{"tag":"text","x":0,"y":0,"w":1,"h":1,"clipped":"bogus"}],"grid":[" "]}`))
	if errs := validateSchema(t, dumpV02Schema, bad); len(errs) == 0 {
		t.Error(`schema/dump.v0.2.json accepts "clipped": "bogus"`)
	}
}

// SPEC v0.3 §21 test 84's IR clause: specs/fixtures/{grid,hints,table,
// tabs}.tui with only their version changed to "3" print IR "0.3" that
// validates against schema/ir.v0.3.json.
func TestVersionThreeFixturesIR(t *testing.T) {
	for _, name := range []string{"grid", "hints", "table", "tabs"} {
		raw, err := os.ReadFile(filepath.Join("../../specs/fixtures", name+".tui"))
		if err != nil {
			t.Fatal(err)
		}
		v3 := strings.Replace(string(raw), `<tui version="2"`, `<tui version="3"`, 1)
		if v3 == string(raw) {
			t.Fatalf(`%s.tui: no <tui version="2" to replace`, name)
		}
		tui := filepath.Join(t.TempDir(), name+".tui")
		writeFile(t, tui, v3)
		code, out, errw := runCLI("ir", tui)
		if code != 0 {
			t.Fatalf("%s: exit %d: %s", name, code, errw)
		}
		if !strings.Contains(out, `"version": "0.3"`) {
			t.Errorf("%s: IR is not 0.3", name)
		}
		if errs := validateSchema(t, irV03Schema, decodeJSON(t, []byte(out))); len(errs) > 0 {
			t.Errorf("%s: IR vs ir.v0.3.json: %v", name, errs)
		}
	}
}
