package play

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/abdul-hamid-achik/tuimark/internal/host"
)

// ParseCommit decodes the object of a commit: step (SPEC v0.4d §15.4),
// {"lines": […], "template": "T", "with": "W", "props": {"P": "V"}}. Each
// member is optional here (Commit says which are needed), but they are
// exact: an unknown or repeated member, or one of the wrong JSON type, is
// an error.
func ParseCommit(data []byte) (host.Commit, error) {
	var c host.Commit
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return c, fmt.Errorf("commit: bad JSON: %v", err)
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return c, fmt.Errorf("commit: want a JSON object")
	}
	seen := map[string]bool{}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return c, fmt.Errorf("commit: bad JSON: %v", err)
		}
		key := keyTok.(string)
		if seen[key] {
			return c, fmt.Errorf("commit: duplicate member %q", key)
		}
		seen[key] = true
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return c, fmt.Errorf("commit: bad JSON: %v", err)
		}
		switch key {
		case "lines":
			if err := json.Unmarshal(raw, &c.Lines); err != nil {
				return c, fmt.Errorf("commit: lines: want a JSON array")
			}
		case "template":
			if err := json.Unmarshal(raw, &c.Template); err != nil {
				return c, fmt.Errorf("commit: template: want a JSON string")
			}
		case "with":
			if err := json.Unmarshal(raw, &c.With); err != nil {
				return c, fmt.Errorf("commit: with: want a JSON string")
			}
		case "props":
			if err := json.Unmarshal(raw, &c.Props); err != nil {
				return c, fmt.Errorf("commit: props: want a JSON object of strings")
			}
		default:
			return c, fmt.Errorf("commit: unknown member %q (want lines, template, with, props)", key)
		}
	}
	if _, err := dec.Token(); err != nil { // the closing brace
		return c, fmt.Errorf("commit: bad JSON: %v", err)
	}
	if dec.More() {
		return c, fmt.Errorf("commit: bad JSON: trailing data")
	}
	return c, nil
}
