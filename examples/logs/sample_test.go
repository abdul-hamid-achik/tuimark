package main

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"testing"
)

// sample.json and mock.json are generated from the seeded generator, so
// they cannot drift from it: TestSampleInSync fails when they differ, and
//
//	UPDATE_SAMPLE=1 go test ./examples/logs -run TestSampleInSync
//
// rewrites them.

// sampleData is the store the opening view starts from.
func sampleData(t *testing.T) map[string]any {
	t.Helper()
	g := newFake(defaultSeed)
	es, hist := backlog(g)
	a := &app{cap: defaultBuffer}
	a.append(es)
	items := a.items()
	levels := map[string]any{lvlError: true, lvlWarn: true, lvlInfo: true, lvlDebug: true}
	visible := visibleLines(items, levels, "")
	rates := make([]any, len(hist))
	for i, h := range hist {
		rates[i] = h
	}
	return map[string]any{
		"source":      "api-gateway (demo)",
		"paused":      false,
		"mouse":       false,
		"help_open":   false,
		"notice":      "",
		"filter":      "",
		"levels":      levels,
		"lines":       items,
		"visible":     visible,
		"stats":       computeStats(items, visible),
		"state_label": "● LIVE",
		"rate_hist":   rates,
		"rate_label":  rateLabel(hist[len(hist)-1]),
	}
}

// mockData applies literal values only (tuimark run has no Go host), so
// each level toggle carries the filtered list it would produce.
func mockData(t *testing.T) map[string]any {
	t.Helper()
	s := sampleData(t)
	items := s["lines"].([]any)
	mock := map[string]any{}
	for _, name := range levelNames {
		levels := map[string]any{lvlError: true, lvlWarn: true, lvlInfo: true, lvlDebug: true}
		levels[name] = false
		visible := visibleLines(items, levels, "")
		mock["toggle_"+name] = []any{
			set("levels."+name, false),
			set("visible", visible),
			set("stats.shown", len(visible)),
		}
	}
	mock["pause"] = []any{set("paused", true), set("state_label", "‖ PAUSED")}
	mock["clear"] = []any{
		set("lines", []any{}), set("visible", []any{}),
		set("stats", map[string]any{"error": 0, "warn": 0, "info": 0, "debug": 0, "total": 0, "shown": 0, "none": true}),
	}
	mock["filter_clear"] = []any{set("filter", "")}
	mock["filter_apply"] = []any{set("notice", "")}
	mock["toggle_mouse"] = []any{set("mouse", true)}
	mock["help_open"] = []any{set("help_open", true)}
	mock["help_close"] = []any{set("help_open", false)}
	return mock
}

func set(path string, v any) map[string]any { return map[string]any{"path": path, "value": v} }

// compact writes a JSON object with one member per line and every array
// element on its own line, so the files diff and read well.
func compact(t *testing.T, v map[string]any) []byte {
	t.Helper()
	var b bytes.Buffer
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	b.WriteString("{\n")
	for i, k := range keys {
		kb, _ := json.Marshal(k)
		b.WriteString("  " + string(kb) + ": ")
		switch x := v[k].(type) {
		case []any:
			if len(x) == 0 || !isObjects(x) {
				vb, _ := json.Marshal(x)
				b.Write(vb)
				break
			}
			b.WriteString("[\n")
			for j, e := range x {
				eb, _ := json.Marshal(e)
				b.WriteString("    " + string(eb))
				if j < len(x)-1 {
					b.WriteString(",")
				}
				b.WriteString("\n")
			}
			b.WriteString("  ]")
		case map[string]any:
			if k == "levels" || k == "stats" || len(x) == 0 || !hasNested(x) {
				vb, _ := json.Marshal(x)
				b.Write(vb)
			} else {
				b.WriteString(string(compactNested(t, x)))
			}
		default:
			vb, _ := json.Marshal(x)
			b.Write(vb)
		}
		if i < len(keys)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("}\n")
	return b.Bytes()
}

func isObjects(x []any) bool {
	_, ok := x[0].(map[string]any)
	return ok
}

func hasNested(m map[string]any) bool {
	for _, v := range m {
		if _, ok := v.([]any); ok {
			return true
		}
	}
	return false
}

// compactNested writes the mock's {"action": [ {path,value}, … ]} members,
// one per line.
func compactNested(t *testing.T, m map[string]any) []byte {
	t.Helper()
	b, err := json.MarshalIndent(m, "  ", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func checkFile(t *testing.T, name string, want map[string]any, render func() []byte) {
	t.Helper()
	if os.Getenv("UPDATE_SAMPLE") != "" {
		if err := os.WriteFile(name, render(), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	raw, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	// Round-trip want through JSON so numbers compare as float64.
	var norm map[string]any
	wb, _ := json.Marshal(want)
	_ = json.Unmarshal(wb, &norm)
	if !reflect.DeepEqual(got, norm) {
		t.Errorf("%s differs from the generator's output; run UPDATE_SAMPLE=1 go test ./examples/logs -run TestSampleInSync", name)
	}
}

func TestSampleInSync(t *testing.T) {
	checkFile(t, "sample.json", sampleData(t), func() []byte { return compact(t, sampleData(t)) })
	checkFile(t, "mock.json", mockData(t), func() []byte { return compact(t, mockData(t)) })
}
