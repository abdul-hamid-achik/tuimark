package main

// A minimal validator for the restricted JSON Schema subset SPEC v0.2 §23
// actually uses: "type" (including "integer" and a type list), "required",
// "properties", "additionalProperties", "items", "enum", "const",
// "minimum", "minLength", "minItems", "pattern", and "$ref" into "#/$defs".
// No schema library is added (SPEC v0.2's dependency rule allows only
// golang.org/x/term, golang.org/x/sys, and github.com/rivo/uniseg), so this
// is deliberately just enough to check §21 test 30: every JSON golden and
// every `play --format json` output validates against schema/dump.v0.2.json,
// and `tuimark ir` output validates against schema/ir.v0.1.json.

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func loadSchema(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return m
}

// resolveRef follows a "#/$defs/NAME" ref against root.
func resolveRef(root map[string]any, ref string) (map[string]any, error) {
	const prefix = "#/$defs/"
	if !strings.HasPrefix(ref, prefix) {
		return nil, fmt.Errorf("unsupported $ref %q", ref)
	}
	name := strings.TrimPrefix(ref, prefix)
	defs, _ := root["$defs"].(map[string]any)
	sub, ok := defs[name].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("$ref %q: no such def", ref)
	}
	return sub, nil
}

func jsonType(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case string:
		return "string"
	case float64:
		if x == math.Trunc(x) {
			return "integer"
		}
		return "number"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	}
	return fmt.Sprintf("%T", v)
}

// typeMatches reports whether v's JSON type satisfies want ("integer" also
// accepts a whole-number "number", matching how encoding/json decodes all
// JSON numbers as float64).
func typeMatches(want string, v any) bool {
	got := jsonType(v)
	if want == "integer" {
		return got == "integer"
	}
	if want == "number" {
		return got == "integer" || got == "number"
	}
	return got == want
}

func deepEqualJSON(a, b any) bool {
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return string(ab) == string(bb)
}

// validate checks value against schema (a node inside root), appending one
// message per violation to errs, each prefixed by an RFC-6901-ish path.
func validate(root, schema map[string]any, value any, path string, errs *[]string) {
	fail := func(format string, args ...any) {
		*errs = append(*errs, fmt.Sprintf("%s: %s", path, fmt.Sprintf(format, args...)))
	}
	if ref, ok := schema["$ref"].(string); ok {
		sub, err := resolveRef(root, ref)
		if err != nil {
			fail("%v", err)
			return
		}
		validate(root, sub, value, path, errs)
		return
	}
	if enum, ok := schema["enum"].([]any); ok {
		found := false
		for _, e := range enum {
			if deepEqualJSON(e, value) {
				found = true
				break
			}
		}
		if !found {
			fail("value %v is not one of %v", value, enum)
		}
	}
	if c, ok := schema["const"]; ok {
		if !deepEqualJSON(c, value) {
			fail("value %v != const %v", value, c)
		}
	}
	if t, ok := schema["type"]; ok {
		var wants []string
		switch tt := t.(type) {
		case string:
			wants = []string{tt}
		case []any:
			for _, x := range tt {
				wants = append(wants, x.(string))
			}
		}
		ok := false
		for _, w := range wants {
			if typeMatches(w, value) {
				ok = true
				break
			}
		}
		if !ok {
			fail("type %v does not match value %v (%s)", wants, value, jsonType(value))
			return // further object/array/string checks would be meaningless
		}
	}
	if pat, ok := schema["pattern"].(string); ok {
		s, isStr := value.(string)
		if isStr {
			re := regexp.MustCompile(pat)
			if !re.MatchString(s) {
				fail("value %q does not match pattern %q", s, pat)
			}
		}
	}
	if minLen, ok := schema["minLength"]; ok {
		if s, isStr := value.(string); isStr {
			if float64(len(s)) < minLen.(float64) {
				fail("length %d < minLength %v", len(s), minLen)
			}
		}
	}
	if min, ok := schema["minimum"]; ok {
		if n, isNum := value.(float64); isNum {
			if n < min.(float64) {
				fail("value %v < minimum %v", n, min)
			}
		}
	}
	if arr, isArr := value.([]any); isArr {
		if minItems, ok := schema["minItems"]; ok {
			if float64(len(arr)) < minItems.(float64) {
				fail("length %d < minItems %v", len(arr), minItems)
			}
		}
		if items, ok := schema["items"].(map[string]any); ok {
			for i, el := range arr {
				validate(root, items, el, fmt.Sprintf("%s/%d", path, i), errs)
			}
		}
	}
	if obj, isObj := value.(map[string]any); isObj {
		if req, ok := schema["required"].([]any); ok {
			for _, r := range req {
				name := r.(string)
				if _, present := obj[name]; !present {
					fail("missing required property %q", name)
				}
			}
		}
		props, _ := schema["properties"].(map[string]any)
		if ap, ok := schema["additionalProperties"]; ok {
			if apBool, isBool := ap.(bool); isBool && !apBool {
				var extra []string
				for k := range obj {
					if _, known := props[k]; !known {
						extra = append(extra, k)
					}
				}
				if len(extra) > 0 {
					sort.Strings(extra)
					fail("unexpected properties %v (additionalProperties: false)", extra)
				}
			}
		}
		for name, sub := range props {
			if v, present := obj[name]; present {
				validate(root, sub.(map[string]any), v, path+"/"+name, errs)
			}
		}
	}
}

// validateSchema validates value (already json.Unmarshal'd into `any`)
// against the schema at path and returns every violation found (nil means
// valid).
func validateSchema(t *testing.T, schemaPath string, value any) []string {
	t.Helper()
	schema := loadSchema(t, schemaPath)
	var errs []string
	validate(schema, schema, value, "", &errs)
	return errs
}

func decodeJSON(t *testing.T, b []byte) any {
	t.Helper()
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, b)
	}
	return v
}
