package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- D8: schema/ir.v0.1.json, schema/ir.v0.2.json, schema/dump.v0.2.json --

const (
	irV01Schema   = "../../schema/ir.v0.1.json"
	irV02Schema   = "../../schema/ir.v0.2.json"
	dumpV02Schema = "../../schema/dump.v0.2.json"
)

func TestSchemaFilesAreValidJSON(t *testing.T) {
	for _, p := range []string{irV01Schema, irV02Schema, dumpV02Schema} {
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
