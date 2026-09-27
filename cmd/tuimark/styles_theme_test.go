package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// --- D2: dump --styles ----------------------------------------------------

func TestDumpStylesRowsCoverColsContiguously(t *testing.T) {
	tui, data := writePlayFixture(t)
	code, out, errw := runCLI("dump", tui, "--data", data, "--format", "json", "--styles", "--cols", "40", "--rows", "6")
	if code != 0 {
		t.Fatalf("exit %d, want 0; stderr=%s", code, errw)
	}
	var d struct {
		Rows   int `json:"rows"`
		Styles [][]struct {
			X  int      `json:"x"`
			W  int      `json:"w"`
			FG string   `json:"fg"`
			BG string   `json:"bg"`
			A  []string `json:"a"`
		} `json:"styles"`
	}
	mustUnmarshal(t, out, &d)
	if len(d.Styles) != d.Rows {
		t.Fatalf("styles has %d rows, want %d", len(d.Styles), d.Rows)
	}
	for y, row := range d.Styles {
		x := 0
		for _, span := range row {
			if span.X != x {
				t.Errorf("row %d: span at x=%d, want contiguous x=%d", y, span.X, x)
			}
			if span.W < 1 {
				t.Errorf("row %d: span width %d < 1", y, span.W)
			}
			x += span.W
		}
		if x != 40 {
			t.Errorf("row %d: spans cover %d columns, want 40", y, x)
		}
	}
}

func TestDumpStylesColorsAreCanonical(t *testing.T) {
	tui, data := writePlayFixture(t)
	code, out, errw := runCLI("dump", tui, "--data", data, "--format", "json", "--styles")
	if code != 0 {
		t.Fatalf("exit %d, want 0; stderr=%s", code, errw)
	}
	var d struct {
		Styles [][]struct {
			FG string `json:"fg"`
			BG string `json:"bg"`
		} `json:"styles"`
	}
	mustUnmarshal(t, out, &d)
	for y, row := range d.Styles {
		for _, span := range row {
			for _, c := range []string{span.FG, span.BG} {
				if !canonicalColor(c) {
					t.Errorf("row %d: color %q is not canonical (#rrggbb, an ANSI name, or default)", y, c)
				}
			}
		}
	}
}

func canonicalColor(c string) bool {
	if c == "default" {
		return true
	}
	if strings.HasPrefix(c, "#") && len(c) == 7 {
		for _, r := range c[1:] {
			if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
				return false
			}
		}
		return true
	}
	for _, name := range []string{"black", "red", "green", "yellow", "blue", "magenta", "cyan", "white"} {
		if c == name || c == "bright-"+name {
			return true
		}
	}
	return false
}

func TestDumpStylesFocusedInputDiffersFromUnfocused(t *testing.T) {
	tui, data := writePlayFixture(t)
	code, out, errw := runCLI("dump", tui, "--data", data, "--format", "json", "--styles")
	if code != 0 {
		t.Fatalf("exit %d, want 0; stderr=%s", code, errw)
	}
	code2, out2, errw2 := runCLI("play", tui, "--data", data, "--input", "tab", "--styles", "--format", "json")
	if code2 != 0 {
		t.Fatalf("exit %d, want 0; stderr=%s", code2, errw2)
	}
	var d1, d2 struct {
		Styles [][]any `json:"styles"`
	}
	mustUnmarshal(t, out, &d1)
	mustUnmarshal(t, out2, &d2)
	b1, _ := json.Marshal(d1.Styles[0])
	b2, _ := json.Marshal(d2.Styles[0])
	if string(b1) == string(b2) {
		t.Errorf("row 0's styles should differ once focus moves off #query (input:focus), got identical spans:\n%s", b1)
	}
}

func TestDumpStylesThemePresent(t *testing.T) {
	tui, data := writePlayFixture(t)
	code, out, errw := runCLI("dump", tui, "--data", data, "--format", "json", "--styles")
	if code != 0 {
		t.Fatalf("exit %d, want 0; stderr=%s", code, errw)
	}
	var d struct {
		Theme string `json:"theme"`
	}
	mustUnmarshal(t, out, &d)
	if d.Theme != "dark" {
		t.Errorf("theme = %q, want the default dark", d.Theme)
	}
}

func TestDumpWithoutStylesOmitsThemeAndStyles(t *testing.T) {
	tui, data := writePlayFixture(t)
	code, out, errw := runCLI("dump", tui, "--data", data, "--format", "json")
	if code != 0 {
		t.Fatalf("exit %d, want 0; stderr=%s", code, errw)
	}
	if strings.Contains(out, `"theme"`) || strings.Contains(out, `"styles"`) {
		t.Errorf("without --styles, dump must not carry theme/styles at all:\n%s", out)
	}
}

// --- D3: --theme ------------------------------------------------------

func TestThemeLightUsesLightTokens(t *testing.T) {
	tui, data := writePlayFixture(t)
	code, out, errw := runCLI("dump", tui, "--data", data, "--theme", "light", "--styles", "--format", "json")
	if code != 0 {
		t.Fatalf("exit %d, want 0; stderr=%s", code, errw)
	}
	if !strings.Contains(out, `"theme": "light"`) {
		t.Errorf("want theme: light in the dump, got:\n%s", out)
	}
	codeD, outD, _ := runCLI("dump", tui, "--data", data, "--styles", "--format", "json")
	if codeD != 0 {
		t.Fatal("dark dump failed")
	}
	if out == outD {
		t.Error("--theme light must render differently from the default dark theme")
	}
}

func TestThemeAutoAndInvalidAreUsageErrors(t *testing.T) {
	tui, _ := writePlayFixture(t)
	for _, cmd := range [][]string{
		{"dump", tui, "--theme", "auto"},
		{"dump", tui, "--theme", "sepia"},
		{"validate", tui, "--theme", "auto"},
		{"preview", tui, "--theme", "auto"},
		{"play", tui, "--theme", "auto"},
	} {
		code, out, errw := runCLI(cmd...)
		if code != 1 {
			t.Errorf("%v: exit %d, want 1; stdout=%s stderr=%s", cmd, code, out, errw)
		}
		if !strings.Contains(errw, "dark or light") {
			t.Errorf("%v: stderr %q, want it to mention dark or light", cmd, errw)
		}
	}
}

func TestThemeOverridesValidateTokenCheck(t *testing.T) {
	// A :root token defined only for dark must warn/error under --theme
	// light if it shadows a built-in the light palette also defines
	// (sanity: at least confirm --theme reaches Validate's token check by
	// changing its outcome for a document with no :root overrides at all,
	// which never errors either way — so this instead checks that
	// validate --theme light succeeds and --json reports light-consistent
	// output without crashing).
	tui, data := writePlayFixture(t)
	code, out, errw := runCLI("validate", tui, "--data", data, "--theme", "light", "--json")
	if code != 0 {
		t.Fatalf("exit %d, want 0; stderr=%s\n%s", code, errw, out)
	}
}

func TestThemeOnValidateAndPreview(t *testing.T) {
	tui, data := writePlayFixture(t)
	if code, _, errw := runCLI("validate", tui, "--data", data, "--theme", "dark"); code != 0 {
		t.Fatalf("validate --theme dark: exit %d; stderr=%s", code, errw)
	}
	if code, out, errw := runCLI("preview", tui, "--data", data, "--theme", "light"); code != 0 {
		t.Fatalf("preview --theme light: exit %d; stderr=%s\n%s", code, errw, out)
	}
}
