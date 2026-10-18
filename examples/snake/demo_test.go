package main

import (
	"bytes"
	"encoding/json"
	"testing"
)

type demoFrame struct {
	Ms   int `json:"ms"`
	Dump struct {
		OK     bool   `json:"ok"`
		Errors []any  `json:"errors"`
		Theme  string `json:"theme"`
		Grid   []string
	} `json:"dump"`
}

// The demo is the input of the docs site's animated SVG: the same bytes on
// every run, 15-120 frames, 5-12 seconds, every frame a clean dump.
func TestDemo(t *testing.T) {
	for _, theme := range []string{"", "light"} {
		var a, b bytes.Buffer
		if err := demo(&a, ".", theme, 90, 24); err != nil {
			t.Fatal(err)
		}
		if err := demo(&b, ".", theme, 90, 24); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(a.Bytes(), b.Bytes()) {
			t.Fatalf("theme %q: two runs of the demo differ", theme)
		}
		lines := bytes.Split(bytes.TrimRight(a.Bytes(), "\n"), []byte("\n"))
		if len(lines) < 15 || len(lines) > 120 {
			t.Errorf("theme %q: %d frames, want 15 to 120", theme, len(lines))
		}
		total := 0
		var last demoFrame
		for i, line := range lines {
			var f demoFrame
			if err := json.Unmarshal(line, &f); err != nil {
				t.Fatalf("frame %d: %v", i, err)
			}
			if !f.Dump.OK || len(f.Dump.Errors) != 0 {
				t.Errorf("frame %d: ok=%v errors=%v", i, f.Dump.OK, f.Dump.Errors)
			}
			if f.Ms <= 0 {
				t.Errorf("frame %d shows for %d ms", i, f.Ms)
			}
			total += f.Ms
			last = f
		}
		if total < 5000 || total > 12000 {
			t.Errorf("theme %q: %d ms in total, want 5000 to 12000", theme, total)
		}
		if last.Ms < 1000 || last.Ms > 1500 {
			t.Errorf("theme %q: the last frame is held %d ms, want 1000 to 1500", theme, last.Ms)
		}
		wantTheme := "dark"
		if theme != "" {
			wantTheme = theme
		}
		if last.Dump.Theme != wantTheme {
			t.Errorf("theme %q: dumps are %q", theme, last.Dump.Theme)
		}
	}
}

// The demo plays a real game: the snake eats several times and lives.
func TestDemoEats(t *testing.T) {
	var buf bytes.Buffer
	if err := demo(&buf, ".", "", 90, 24); err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimRight(buf.Bytes(), "\n"), []byte("\n"))
	var f demoFrame
	if err := json.Unmarshal(lines[len(lines)-1], &f); err != nil {
		t.Fatal(err)
	}
	g := joinGrid(f.Dump.Grid)
	for _, want := range []string{"PLAYING", "autopilot"} {
		if !bytes.Contains([]byte(g), []byte(want)) {
			t.Errorf("last frame lacks %q:\n%s", want, g)
		}
	}
	if bytes.Contains([]byte(g), []byte("score                0")) {
		t.Errorf("the demo ate nothing:\n%s", g)
	}
}

func joinGrid(rows []string) string {
	var b bytes.Buffer
	for _, r := range rows {
		b.WriteString(r + "\n")
	}
	return b.String()
}
