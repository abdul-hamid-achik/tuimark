package main

import (
	"os"
	"strings"
	"testing"
)

// SPEC v0.4b §24, phase F7: the CLI parts of tests 211, 213, 214, and 216
// (a button's label as a template, §6.4.2). The Go API parts are the root
// package's conformance_v04b_button_test.go; both read
// testdata/v04b/t211/btn.tui.

const (
	btnFixture = "../../testdata/v04b/t211/btn.tui"
	btnJSON    = "../../testdata/v04b/t211/btn.json"
)

func TestV04bButtonLabelValidate(t *testing.T) {
	// 211: nothing to report with the data; without it, bindings are not
	// checked (213).
	code, out, errw := runCLI("validate", btnFixture, "--data", btnJSON)
	if code != 0 || strings.TrimSpace(out) != "ok" {
		t.Errorf("--data: exit %d, %q (%s)", code, out, errw)
	}
	code, out, _ = runCLI("validate", btnFixture)
	if code != 0 || strings.TrimSpace(out) != "ok (bindings not checked: no --data)" {
		t.Errorf("no data: exit %d, %q", code, out)
	}

	// 213: one B003 at the button, an error with --strict.
	data := writeTemp(t, "d.json", `{"no": "Can\ncel"}`)
	code, out, _ = runCLI("validate", btnFixture, "--data", data)
	if code != 0 || !strings.Contains(out, `warning B003 btn.tui:4:7 /screen/row/button`) || strings.Count(out, "B003") != 1 {
		t.Errorf("missing n: exit %d, %q", code, out)
	}
	code, out, _ = runCLI("validate", btnFixture, "--data", data, "--strict")
	if code != 2 || !strings.Contains(out, `error B003 btn.tui:4:7 /screen/row/button bind path "n" is missing`) {
		t.Errorf("missing n, --strict: exit %d, %q", code, out)
	}
}

func TestV04bButtonLabelPinned(t *testing.T) {
	raw, err := os.ReadFile(btnFixture)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"1", "2", "3"} {
		src := strings.Replace(string(raw), "<tui>", `<tui version="`+v+`">`, 1)
		doc := writeTemp(t, "btn.tui", src)
		code, out, _ := runCLI("validate", doc, "--data", btnJSON)
		const msg = "{path} in a <button> label (requires a <tui> without version)"
		if code != 2 ||
			!strings.Contains(out, "error V003 btn.tui:4:24 /screen/row/button "+msg) ||
			!strings.Contains(out, "error V003 btn.tui:5:7 /screen/row/button "+msg) ||
			strings.Count(out, "V003") != 2 || strings.Contains(out, "B003") {
			t.Errorf("version=%s: exit %d, %q", v, code, out)
		}
	}
}

func TestV04bButtonLabelIR(t *testing.T) {
	code, out, errw := runCLI("ir", btnFixture)
	if code != 0 {
		t.Fatalf("ir: exit %d: %s", code, errw)
	}
	for _, s := range []string{`"version": "0.4"`, `"label": "Delete {n} tasks"`, `"text": "{no}"`} {
		if !strings.Contains(out, s) {
			t.Errorf("ir lacks %s", s)
		}
	}
}
