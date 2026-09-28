package parse

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
)

// tokensFor resolves the theme + :root token table for doc the way
// cmd/tuimark's `ir` command does, reading any external style src=
// relative to dir. Media conditions are ignored (SPEC §13.1: "tokens = the
// theme tokens overridden by :root tokens of the loaded stylesheets (media
// ignored)").
func tokensFor(t *testing.T, doc *Document, dir string) map[string]string {
	t.Helper()
	theme := doc.Theme
	if theme == "" {
		theme = "dark"
	}
	base, ok := css.Themes[theme]
	if !ok {
		base = css.Themes["dark"]
	}
	tokens := make(map[string]string, len(base))
	for k, v := range base {
		tokens[k] = v
	}
	for _, s := range doc.Styles {
		var sheet *css.Sheet
		if s.Src != "" {
			raw, err := os.ReadFile(filepath.Join(dir, s.Src))
			if err != nil {
				t.Fatalf("read style src %q: %v", s.Src, err)
			}
			sheet, _ = css.ParseSheet(string(raw), s.Src)
		} else {
			sheet, _ = css.ParseInlineSheet(s.Body, "", s.BodyLine, s.BodyCol)
		}
		for _, r := range sheet.Rules {
			for _, sel := range r.Selectors {
				if !sel.Root {
					continue
				}
				for _, d := range r.Decls {
					tokens[strings.TrimPrefix(d.Prop, "--")] = d.Value
				}
			}
		}
	}
	return tokens
}

func allExampleFiles(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob("../../examples/*/*.tui")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no examples/*/*.tui files found")
	}
	return files
}

// --- A small JSON-Schema (2020-12) subset validator ------------------------
//
// Supports exactly what schema/ir.v0.1.json uses: type, required,
// properties, additionalProperties (bool or schema), items, enum, const,
// and $ref into #/$defs. That is enough to validate the IR; it is not a
// general-purpose validator.

type schemaValidator struct {
	root map[string]any
}

func (v *schemaValidator) resolve(schema map[string]any) map[string]any {
	if ref, ok := schema["$ref"].(string); ok {
		const prefix = "#/$defs/"
		name := strings.TrimPrefix(ref, prefix)
		defs, _ := v.root["$defs"].(map[string]any)
		def, _ := defs[name].(map[string]any)
		return def
	}
	return schema
}

func jsonTypeOf(data any) string {
	switch data.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case float64:
		return "number"
	case string:
		return "string"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	}
	return fmt.Sprintf("%T", data)
}

func (v *schemaValidator) validate(schema map[string]any, data any, path string) []string {
	schema = v.resolve(schema)
	if schema == nil {
		return []string{fmt.Sprintf("%s: unresolved schema", path)}
	}
	var errs []string
	if want, ok := schema["const"]; ok {
		if !reflect.DeepEqual(want, data) {
			errs = append(errs, fmt.Sprintf("%s: want const %v, got %v", path, want, data))
		}
	}
	if rawEnum, ok := schema["enum"].([]any); ok {
		found := false
		for _, e := range rawEnum {
			if reflect.DeepEqual(e, data) {
				found = true
				break
			}
		}
		if !found {
			errs = append(errs, fmt.Sprintf("%s: %v not in enum %v", path, data, rawEnum))
		}
	}
	if want, ok := schema["type"].(string); ok {
		if got := jsonTypeOf(data); got != want {
			errs = append(errs, fmt.Sprintf("%s: want type %s, got %s", path, want, got))
			return errs // deeper checks are meaningless against the wrong type
		}
	}
	if obj, ok := data.(map[string]any); ok {
		if rawReq, ok := schema["required"].([]any); ok {
			for _, r := range rawReq {
				key, _ := r.(string)
				if _, present := obj[key]; !present {
					errs = append(errs, fmt.Sprintf("%s: missing required %q", path, key))
				}
			}
		}
		props, _ := schema["properties"].(map[string]any)
		addl, hasAddl := schema["additionalProperties"]
		for k, val := range obj {
			if propSchema, ok := props[k].(map[string]any); ok {
				errs = append(errs, v.validate(propSchema, val, path+"."+k)...)
				continue
			}
			if !hasAddl {
				continue
			}
			switch a := addl.(type) {
			case bool:
				if !a {
					errs = append(errs, fmt.Sprintf("%s: additional property %q not allowed", path, k))
				}
			case map[string]any:
				errs = append(errs, v.validate(a, val, path+"."+k)...)
			}
		}
	}
	if items, ok := schema["items"].(map[string]any); ok {
		if arr, ok := data.([]any); ok {
			for i, el := range arr {
				errs = append(errs, v.validate(items, el, fmt.Sprintf("%s[%d]", path, i))...)
			}
		}
	}
	return errs
}

// Every example's IR validates against the schema of its version (SPEC
// v0.2b §21 test 68): IR "0.1" against schema/ir.v0.1.json, IR "0.2" (a
// version="2" document such as examples/monitor) against ir.v0.2.json.
func TestIRValidatesAgainstSchema(t *testing.T) {
	schemas := map[string]map[string]any{}
	for version, path := range map[string]string{"0.1": "../../schema/ir.v0.1.json", "0.2": "../../schema/ir.v0.2.json", "0.3": "../../schema/ir.v0.3.json"} {
		schemaRaw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var schema map[string]any
		if err := json.Unmarshal(schemaRaw, &schema); err != nil {
			t.Fatalf("schema is not valid JSON: %v", err)
		}
		schemas[version] = schema
	}

	for _, f := range allExampleFiles(t) {
		f := f
		t.Run(f, func(t *testing.T) {
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			doc := Parse(src, f)
			if doc.Diags.HasErrors() {
				t.Fatalf("%s has diagnostics, want a clean example: %v", f, doc.Diags)
			}
			tokens := tokensFor(t, doc, filepath.Dir(f))
			irDoc := BuildIR(doc, tokens)
			b, err := json.Marshal(irDoc)
			if err != nil {
				t.Fatal(err)
			}
			var data any
			if err := json.Unmarshal(b, &data); err != nil {
				t.Fatal(err)
			}
			schema := schemas[irDoc.Version]
			if schema == nil {
				t.Fatalf("%s: IR version %q has no schema", f, irDoc.Version)
			}
			v := &schemaValidator{root: schema}
			if errs := v.validate(schema, data, "$"); len(errs) > 0 {
				t.Errorf("%s: IR does not validate against schema/ir.v%s.json:\n%s\n--- IR ---\n%s", f, irDoc.Version, strings.Join(errs, "\n"), b)
			}
		})
	}
}
