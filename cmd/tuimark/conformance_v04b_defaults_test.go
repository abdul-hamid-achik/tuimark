package main

import (
	"testing"
)

// SPEC v0.4b §24 conformance tests of the layout defaults (phase F5), the
// parts only the CLI observes: inspect's origin of the built-in rules and
// validate's exit code. The geometry and the diagnostics are the root
// package's conformance_v04b_defaults_test.go.

const v04bDefaults = "../../testdata/v04b/"

// f5Inspect runs inspect --id ID --json on a fixture at cols×rows and
// returns its style entries by property.
func f5Inspect(t *testing.T, file, id, cols, rows string) map[string]f5Prop {
	t.Helper()
	code, out, errw := runCLI("inspect", file, "--id", id, "--cols", cols, "--rows", rows, "--json")
	if code != 0 {
		t.Fatalf("inspect %s --id %s: exit %d: %s", file, id, code, errw)
	}
	var in struct {
		Style []f5Prop `json:"style"`
	}
	mustUnmarshal(t, out, &in)
	props := map[string]f5Prop{}
	for _, p := range in.Style {
		props[p.Prop] = p
	}
	return props
}

type f5Prop struct {
	Prop   string `json:"prop"`
	Value  any    `json:"value"`
	Origin string `json:"origin"`
	Rule   *struct {
		File        string `json:"file"`
		Selector    string `json:"selector"`
		Specificity int    `json:"specificity"`
	} `json:"rule"`
}

// 165, last bullet: #top's height comes from the current language's
// built-in col/row rule, and #nav has no width at all.
func TestV04bInspectColRowDefaults(t *testing.T) {
	file := v04bDefaults + "165/panes.tui"
	h := f5Inspect(t, file, "top", "30", "8")["height"]
	const sel = "screen > col, screen > row, modal > col, modal > row, tab > col, tab > row"
	if h.Value != "1fr" || h.Origin != "ua" || h.Rule == nil || h.Rule.Selector != sel || h.Rule.Specificity != 2 {
		t.Errorf("#top height: %+v %+v", h, h.Rule)
	}
	w, ok := f5Inspect(t, file, "nav", "30", "8")["width"]
	if !ok || w.Value != nil || w.Origin != "initial" || w.Rule != nil {
		t.Errorf("#nav width: %+v", w)
	}
	// The twin: both come from 0.4.1's col/row rule.
	h = f5Inspect(t, "../../specs/fixtures/v04b/twins/panes.tui", "top", "30", "8")["height"]
	if h.Value != "1fr" || h.Origin != "ua" || h.Rule == nil || h.Rule.Selector != "row" {
		t.Errorf("twin #top height: %+v %+v", h, h.Rule)
	}
}

// §10.6's focus look: a focused button is reversed by the built-in
// button:focus rule in the current language (test 158's #yes), and not in
// its twin.
func TestV04bInspectButtonFocusLook(t *testing.T) {
	r := f5Inspect(t, v04bDefaults+"158/confirm.tui", "yes", "40", "12")["reverse"]
	if r.Value != "true" || r.Origin != "ua" || r.Rule == nil || r.Rule.Selector != "button:focus, checkbox:focus, select:focus" || r.Rule.Specificity != 11 {
		t.Errorf("#yes reverse: %+v %+v", r, r.Rule)
	}
	r = f5Inspect(t, "../../specs/fixtures/v04b/twins/confirm.tui", "yes", "40", "12")["reverse"]
	if r.Value == "true" || r.Origin == "ua" {
		t.Errorf("twin #yes reverse: %+v", r)
	}
}

// 167: validate exits 2 on the L002 error; 169's L009 alone is a warning
// and exits 0.
func TestV04bValidateExitCodes(t *testing.T) {
	for _, c := range []struct {
		file string
		code int
	}{
		{v04bDefaults + "167/pct.tui", 2},
		{"../../specs/fixtures/v04b/twins/pct.tui", 0},
		{v04bDefaults + "169/twopane.tui", 0},
	} {
		if code, out, errw := runCLI("validate", c.file); code != c.code {
			t.Errorf("validate %s: exit %d, want %d\n%s%s", c.file, code, c.code, out, errw)
		}
	}
}
