package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/abdul-hamid-achik/tuimark/internal/host"
)

// runCLI runs the command as a fresh process would, but captures stdout and
// stderr instead of writing to the real ones, so tests can assert on both.
func runCLI(args ...string) (code int, stdout, stderr string) {
	var out, errw bytes.Buffer
	c := &cli{stdout: &out, stderr: &errw}
	code = c.run(args)
	return code, out.String(), errw.String()
}

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

const spikeFixture = "../../examples/spike/inbox.tui"
const inboxFixture = "../../examples/inbox/app.tui"
const inboxData = "../../examples/inbox/sample.json"

// --- dump -------------------------------------------------------------

func TestDumpOK(t *testing.T) {
	code, out, errw := runCLI("dump", spikeFixture, "--cols", "80", "--rows", "24")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%s", code, errw)
	}
	if !strings.Contains(out, "=== grid 80x24 ===") {
		t.Errorf("missing grid header in output:\n%s", out)
	}
}

func TestDumpJSONShapeAndGridWidth(t *testing.T) {
	for _, cols := range []int{40, 80, 120} {
		code, out, errw := runCLI("dump", spikeFixture, "--format", "json", "--cols", strconv.Itoa(cols), "--rows", "24")
		if code != 0 {
			t.Fatalf("cols=%d: exit code = %d, want 0; stderr=%s", cols, code, errw)
		}
		var d struct {
			Cols int      `json:"cols"`
			Rows int      `json:"rows"`
			OK   bool     `json:"ok"`
			Grid []string `json:"grid"`
		}
		if err := json.Unmarshal([]byte(out), &d); err != nil {
			t.Fatalf("cols=%d: dump --format json did not parse: %v\n%s", cols, err, out)
		}
		if !d.OK {
			t.Fatalf("cols=%d: dump not ok", cols)
		}
		if len(d.Grid) != d.Rows {
			t.Fatalf("cols=%d: %d grid rows, want %d", cols, len(d.Grid), d.Rows)
		}
		for y, row := range d.Grid {
			if n := utf8.RuneCountInString(row); n != d.Cols {
				t.Errorf("cols=%d: row %d has %d runes, want %d", cols, y, n, d.Cols)
			}
		}
	}
}

func TestDumpMissingFileIsIOError(t *testing.T) {
	code, _, errw := runCLI("dump", "/no/such/file.tui")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if errw == "" {
		t.Error("want an error message on stderr")
	}
}

// finding 18 / SPEC §15: "Default --cols 80 --rows 24 --format text" for
// dump. Nothing else pins the default size, so a change to fs.Int("cols",
// 80, ...) or fs.Int("rows", 24, ...) would go unnoticed.
func TestDumpDefaults(t *testing.T) {
	code, out, errw := runCLI("dump", spikeFixture)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%s", code, errw)
	}
	if !strings.HasPrefix(out, "=== grid 80x24 ===\n") {
		t.Errorf("dump with no size flags should default to 80x24, got header from:\n%s", firstLine(out))
	}
	if strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Errorf("dump with no --format flag should default to text, got what looks like JSON:\n%s", firstLine(out))
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// inboxSampleWithoutQueryStatus is examples/inbox/sample.json with the
// top-level "query" and "status" keys removed, so binding it against
// examples/inbox/app.tui leaves exactly the two documented B003s: the
// <input bind="query"> and the <text>{status}</text> paths.
const inboxSampleWithoutQueryStatus = `{
  "folder": "deploy",
  "count": 3,
  "selected": "t-12",
  "selected_ticket": {
    "id": "t-12",
    "title": "login loop on staging",
    "body": "Users hitting /login are redirected back to /login after a 302."
  },
  "tickets": [
    { "id": "t-12", "title": "login loop on staging", "prio": "P1" },
    { "id": "t-18", "title": "cannot deploy europe-west", "prio": "P1" },
    { "id": "t-21", "title": "stale feature flag cache", "prio": "P2" }
  ]
}`

// finding 16 / SPEC §15: "--data MUST be accepted from phase 0 even if
// ignored until phase 2, so agent skills do not churn." Before this test,
// nothing at the CLI level exercised --data on dump or preview: a mutation
// that made cmdDump ignore or reject the flag left `go test ./...` green.
func TestDumpAcceptsDataOnSpike(t *testing.T) {
	// The spike fixture has no bind paths, so --data must be accepted
	// without changing the dump at all (the phase-0 "MUST be accepted
	// ... even if ignored" wording).
	withData, out1, errw1 := runCLI("dump", spikeFixture, "--data", inboxData, "--format", "json")
	if withData != 0 {
		t.Fatalf("dump --data on spike: exit code = %d, want 0; stderr=%s", withData, errw1)
	}
	without, out2, errw2 := runCLI("dump", spikeFixture, "--format", "json")
	if without != 0 {
		t.Fatalf("dump without --data: exit code = %d, want 0; stderr=%s", without, errw2)
	}
	if out1 != out2 {
		t.Errorf("dump --data on a fixture with no bind paths changed the output:\n--- with --data ---\n%s\n--- without ---\n%s", out1, out2)
	}
}

func TestDumpDataBindsInbox(t *testing.T) {
	withData, out, errw := runCLI("dump", inboxFixture, "--data", inboxData)
	if withData != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%s", withData, errw)
	}
	if !strings.Contains(out, "mail  deploy") {
		t.Errorf("dump --data should bind {folder} into the header row, got:\n%s", firstLine(strings.TrimPrefix(out, "=== grid 80x24 ===\n")))
	}

	// Without --data every bind path is missing (including each="tickets
	// as item", a B001 error), so this dump is expected to exit 2 — the
	// point here is only that it must not still show the bound value.
	_, out2, _ := runCLI("dump", inboxFixture)
	if strings.Contains(out2, "mail  deploy") {
		t.Errorf("dump without --data must not have bound data:\n%s", firstLine(strings.TrimPrefix(out2, "=== grid 80x24 ===\n")))
	}
}

// finding 17 / SPEC §4 Phase 4, §13.2: "dump --cells (cell → id)" and
// "per-cell {x,y,ch,id}". This was covered at the internal/dump level but
// never through the CLI flag, so a mutation dropping *cells before it
// reached dump.Build left `go test ./...` green.
func TestDumpCellsJSON(t *testing.T) {
	code, out, errw := runCLI("dump", inboxFixture, "--data", inboxData, "--format", "json", "--cols", "80", "--rows", "24", "--cells")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%s", code, errw)
	}
	var d struct {
		Cells []struct {
			X  int    `json:"x"`
			Y  int    `json:"y"`
			Ch string `json:"ch"`
			ID string `json:"id,omitempty"`
		} `json:"cells"`
	}
	if err := json.Unmarshal([]byte(out), &d); err != nil {
		t.Fatalf("dump --cells did not parse: %v\n%s", err, out)
	}
	if len(d.Cells) != 80*24 {
		t.Fatalf("len(cells) = %d, want %d (cols*rows)", len(d.Cells), 80*24)
	}
	first := d.Cells[0]
	if first.X != 0 || first.Y != 0 || first.Ch != "m" || first.ID != "header" {
		t.Errorf("cells[0] = %+v, want {x:0 y:0 ch:m id:header}", first)
	}

	code2, out2, errw2 := runCLI("dump", inboxFixture, "--data", inboxData, "--format", "json")
	if code2 != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%s", code2, errw2)
	}
	if strings.Contains(out2, `"cells"`) {
		t.Errorf("dump without --cells must omit the cells key:\n%s", out2)
	}
}

// --- SPEC §21 tests 5-9 through the CLI --------------------------------

func TestDiagnosticExitCodesThroughCLI(t *testing.T) {
	cases := []struct {
		name, code, src string
	}{
		{"unknown tag", "V001", `<app><widget/></app>`},
		{"duplicate id", "V004", `<app><box id="a"/><box id="a"/></app>`},
		{"bad unit", "V003", `<app><box width="foo"/></app>`},
		{"unclosed xml", "V005", `<app><box></app>`},
		{"text with element", "V013", `<app><text><box/></text></app>`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			file := writeTemp(t, "doc.tui", c.src)

			code, _, _ := runCLI("validate", file)
			if code != 2 {
				t.Errorf("validate exit code = %d, want 2", code)
			}
			vcode, vout, _ := runCLI("validate", file, "--json")
			if vcode != 2 {
				t.Errorf("validate --json exit code = %d, want 2", vcode)
			}
			if !strings.Contains(vout, c.code) {
				t.Errorf("validate --json output missing %s:\n%s", c.code, vout)
			}

			dcode, _, _ := runCLI("dump", file)
			if dcode != 2 {
				t.Errorf("dump exit code = %d, want 2", dcode)
			}

			// Exit codes are uniform across every command (README): `ir`
			// must refuse to print IR for an invalid document too, not
			// only when the document fails to parse at all.
			icode, iout, ierrw := runCLI("ir", file)
			if icode != 2 {
				t.Errorf("ir exit code = %d, want 2", icode)
			}
			if iout != "" {
				t.Errorf("ir must print nothing on stdout when it exits 2, got %q", iout)
			}
			if !strings.Contains(ierrw, c.code) {
				t.Errorf("ir stderr missing %s:\n%s", c.code, ierrw)
			}
		})
	}
}

// --- validate -----------------------------------------------------------

func TestValidateJSONShape(t *testing.T) {
	code, out, errw := runCLI("validate", spikeFixture, "--json")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%s", code, errw)
	}
	var v struct {
		OK          bool   `json:"ok"`
		File        string `json:"file"`
		Diagnostics []any  `json:"diagnostics"`
		Actions     []any  `json:"actions"`
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("validate --json did not parse: %v\n%s", err, out)
	}
	if !v.OK {
		t.Error("want ok: true")
	}
	if v.Diagnostics == nil {
		t.Error("want a diagnostics array (possibly empty), got null")
	}
}

func TestValidateCatalogWarnsB004(t *testing.T) {
	catalog := writeTemp(t, "catalog.json", `["quit"]`)
	code, out, errw := runCLI("validate", inboxFixture, "--data", inboxData, "--catalog", catalog)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (B004 is a warning); stderr=%s", code, errw)
	}
	if !strings.Contains(out, "B004") {
		t.Errorf("want a B004 warning for actions missing from the catalog:\n%s", out)
	}
}

// finding 17 / SPEC §14: "B003 | bind | bind path missing (--strict
// upgrades to error)". Only internal/host had a test that SetStrict makes
// B003 an error; nothing at the CLI level did, so a mutation that dropped
// the --strict flag before it reached app.SetStrict left `go test ./...`
// green.
func TestValidateStrictUpgradesB003(t *testing.T) {
	data := writeTemp(t, "no-query-status.json", inboxSampleWithoutQueryStatus)

	code, out, errw := runCLI("validate", inboxFixture, "--data", data)
	if code != 0 {
		t.Fatalf("without --strict: exit code = %d, want 0 (B003 is a warning); stderr=%s", code, errw)
	}
	if !strings.Contains(out, "warning B003") {
		t.Errorf("without --strict, want warning B003 lines:\n%s", out)
	}
	if strings.Contains(out, "error B003") {
		t.Errorf("without --strict, B003 must not be an error:\n%s", out)
	}

	scode, sout, serrw := runCLI("validate", inboxFixture, "--data", data, "--strict")
	if scode != 2 {
		t.Fatalf("with --strict: exit code = %d, want 2; stderr=%s", scode, serrw)
	}
	if !strings.Contains(sout, "error B003") {
		t.Errorf("with --strict, want error B003 lines:\n%s", sout)
	}
	if strings.Contains(sout, "\nok\n") || strings.HasSuffix(sout, "\nok") {
		t.Errorf("with --strict and B003 present, must not print the ok line:\n%s", sout)
	}

	jcode, jout, jerrw := runCLI("validate", inboxFixture, "--data", data, "--strict", "--json")
	if jcode != 2 {
		t.Fatalf("--strict --json: exit code = %d, want 2; stderr=%s", jcode, jerrw)
	}
	var v struct {
		OK          bool `json:"ok"`
		Diagnostics []struct {
			Severity string `json:"severity"`
			Code     string `json:"code"`
		} `json:"diagnostics"`
	}
	if err := json.Unmarshal([]byte(jout), &v); err != nil {
		t.Fatalf("--strict --json did not parse: %v\n%s", err, jout)
	}
	if v.OK {
		t.Error("--strict --json: want ok=false")
	}
	found := false
	for _, d := range v.Diagnostics {
		if d.Code == "B003" {
			found = true
			if d.Severity != "error" {
				t.Errorf("B003 severity = %q, want error under --strict", d.Severity)
			}
		}
	}
	if !found {
		t.Errorf("--strict --json: no B003 diagnostic found:\n%s", jout)
	}
}

// --- fmt ------------------------------------------------------------------

func TestFmtCheckAndWrite(t *testing.T) {
	messy := `<app><box width="10" class="c" border="1" id="a"/></app>`
	file := writeTemp(t, "messy.tui", messy)

	code, _, _ := runCLI("fmt", file, "--check")
	if code != 2 {
		t.Fatalf("--check on an unformatted file: exit code = %d, want 2", code)
	}

	code, out, errw := runCLI("fmt", file)
	if code != 0 {
		t.Fatalf("plain fmt: exit code = %d, want 0; stderr=%s", code, errw)
	}
	if !strings.Contains(out, `id="a" class="c" width="10" border="1"`) {
		t.Errorf("attributes not canonically ordered:\n%s", out)
	}

	code, _, errw = runCLI("fmt", file, "--write")
	if code != 0 {
		t.Fatalf("--write: exit code = %d, want 0; stderr=%s", code, errw)
	}
	written, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(written) != out {
		t.Errorf("--write did not rewrite the file to match plain fmt output:\n--- file ---\n%s\n--- fmt ---\n%s", written, out)
	}

	code, _, _ = runCLI("fmt", file, "--check")
	if code != 0 {
		t.Fatalf("--check after --write: exit code = %d, want 0", code)
	}
}

func TestFmtV005ExitsTwo(t *testing.T) {
	file := writeTemp(t, "bad.tui", `<app><box></app>`)
	code, _, errw := runCLI("fmt", file)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(errw, "V005") {
		t.Errorf("want V005 on stderr:\n%s", errw)
	}
}

// --- preview ------------------------------------------------------------

// SPEC §15: "preview writes the text dump to stdout. If stdout is a TTY,
// it MAY also paint ANSI" — an addition, not a replacement: the plain
// `=== grid ===` text dump must always be there, with the ANSI-painted
// grid first when stdout is a TTY.
func TestRenderPreviewAlwaysIncludesTextDump(t *testing.T) {
	app, err := host.Load(spikeFixture)
	if err != nil {
		t.Fatal(err)
	}
	var tty bytes.Buffer
	ok := renderPreview(&tty, app, 40, 6, true)
	if !ok {
		t.Fatal("renderPreview reported !ok for a clean fixture")
	}
	ttyOut := tty.String()
	if !strings.Contains(ttyOut, "=== grid 40x6 ===") {
		t.Errorf("TTY preview is missing the text dump's grid header:\n%s", ttyOut)
	}
	if !strings.Contains(ttyOut, "=== nodes ===") || !strings.Contains(ttyOut, "=== errors ===") {
		t.Errorf("TTY preview is missing nodes/errors sections:\n%s", ttyOut)
	}
	if !strings.Contains(ttyOut, "\x1b[") {
		t.Errorf("TTY preview has no ANSI at all:\n%q", ttyOut)
	}

	app2, err := host.Load(spikeFixture)
	if err != nil {
		t.Fatal(err)
	}
	var plain bytes.Buffer
	renderPreview(&plain, app2, 40, 6, false)
	plainOut := plain.String()
	if strings.Contains(plainOut, "\x1b[") {
		t.Errorf("non-TTY preview must not paint ANSI:\n%q", plainOut)
	}
	// Non-TTY output is exactly dump.Text(...); TTY adds the ANSI frame in
	// front of that same text (SPEC's "also"), so it must end with it.
	if !strings.HasSuffix(ttyOut, plainOut) {
		t.Errorf("TTY output must end with the plain text dump.\n--- TTY ---\n%s\n--- plain ---\n%s", ttyOut, plainOut)
	}
}

// finding 16 / SPEC §15: --data MUST be accepted (and, per §22's agent
// loop wording, applied) by preview too, not only dump.
func TestPreviewDataBindsInbox(t *testing.T) {
	code, out, errw := runCLI("preview", inboxFixture, "--data", inboxData)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%s", code, errw)
	}
	if !strings.Contains(out, "mail  deploy") {
		t.Errorf("preview --data should bind {folder} into the header row:\n%s", out)
	}
}

// finding 18 / SPEC §15: "Default --cols 80 --rows 24 --format text" for
// preview too (preview has no --format flag, but the same default size).
// runCLI's stdout is a *bytes.Buffer, so this exercises the non-TTY path.
func TestPreviewDefaults(t *testing.T) {
	code, out, errw := runCLI("preview", spikeFixture)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%s", code, errw)
	}
	if !strings.HasPrefix(out, "=== grid 80x24 ===\n") {
		t.Errorf("preview with no size flags should default to 80x24, got header from:\n%s", firstLine(out))
	}
}

// finding 18 / SPEC §14 exit codes ("2 | validation errors"): preview must
// exit 2 on an invalid document, the same as dump and validate. Only
// renderPreview's return value was covered directly; cmdPreview's use of
// it (commands.go: "if !ok { return 2 }") had no CLI-level test.
func TestPreviewInvalidExits2(t *testing.T) {
	file := writeTemp(t, "doc.tui", `<app><widget/></app>`)
	code, out, errw := runCLI("preview", file)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2; stdout=%s stderr=%s", code, out, errw)
	}
	if !strings.Contains(out, "=== grid 80x24 ===") {
		t.Errorf("preview must still print the grid header on an invalid document:\n%s", out)
	}
	if !strings.Contains(out, "V001") {
		t.Errorf("preview output missing V001:\n%s", out)
	}
}

// --- ir ---------------------------------------------------------------

func TestIRCommand(t *testing.T) {
	code, out, errw := runCLI("ir", spikeFixture)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%s", code, errw)
	}
	var doc struct {
		Version string `json:"version"`
		Root    struct {
			Kind string `json:"kind"`
		} `json:"root"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("ir output did not parse as JSON: %v\n%s", err, out)
	}
	if doc.Version != "0.1" {
		t.Errorf("version = %q, want 0.1", doc.Version)
	}
	if doc.Root.Kind != "col" {
		t.Errorf("root.kind = %q, want col (the <app> alias)", doc.Root.Kind)
	}
}

// SPEC §13.1/§23: a node's kind is restricted to the 18-value enum, and an
// unknown tag must "fail before layout". `ir` must exit 2 and print
// nothing, not a node with a schema-invalid `"kind"`.
func TestIRRejectsUnknownKind(t *testing.T) {
	file := writeTemp(t, "doc.tui", `<app><widget id="w" width="20" height="3" border="1"/></app>`)
	code, out, errw := runCLI("ir", file)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if out != "" {
		t.Errorf("stdout = %q, want none", out)
	}
	if !strings.Contains(errw, "V001") {
		t.Errorf("stderr missing V001:\n%s", errw)
	}
}

// finding 5 / decisiones.md ronda 1: "tuimark ir sale con 2 (diagnósticos a
// stderr, nada a stdout) si el documento tiene errores" — including errors
// that live only in the document's stylesheets (V003 inside <style>/.tcss,
// V006 for a bad style src=, and the CheckTokens unknown-token diagnostics),
// which validate and dump already treat as fatal. Before the fix, cmdIR
// gated only on doc.Diags and happily printed IR + exit 0 for all three.
func TestIRExitsTwoOnStylesheetErrors(t *testing.T) {
	cases := []struct {
		name, code, src string
	}{
		{
			name: "unknown token in inline style",
			code: "V003",
			src:  `<tui version="1"><style>#t { color: $typo; }</style><screen id="main"><text id="t">x</text></screen></tui>`,
		},
		{
			name: "unknown property and bad size in inline style",
			code: "V003",
			src:  `<tui version="1"><style>#t { colr: red; width: 12px }</style><screen id="main"><text id="t">x</text></screen></tui>`,
		},
		{
			name: "unreadable style src",
			code: "V006",
			src:  `<tui version="1"><style src="nope.tcss"/><screen id="main"><text id="t">x</text></screen></tui>`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file := writeTemp(t, "doc.tui", tc.src)

			// validate must already fail this document (sanity check that
			// the fixture itself, not the CLI wiring, is what's exercised).
			vcode, _, _ := runCLI("validate", file)
			if vcode != 2 {
				t.Fatalf("validate exit code = %d, want 2 (fixture should be invalid)", vcode)
			}

			icode, iout, ierrw := runCLI("ir", file)
			if icode != 2 {
				t.Errorf("ir exit code = %d, want 2", icode)
			}
			if iout != "" {
				t.Errorf("ir must print no IR on stdout when the stylesheet has errors, got %q", iout)
			}
			if !strings.Contains(ierrw, tc.code) {
				t.Errorf("ir stderr missing %s:\n%s", tc.code, ierrw)
			}
		})
	}
}

// --- test (golden runner) ----------------------------------------------

func TestTestCommandPasses(t *testing.T) {
	// The manifest's "file" entries are repo-root relative (as `tuimark
	// test` expects when run from the repo root); chdir there for this
	// test only.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir("../.."); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(wd) })

	code, out, errw := runCLI("test", "testdata/golden")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%s\n%s", code, errw, out)
	}
	if !strings.Contains(out, "PASS spike") {
		t.Errorf("want PASS lines for the spike entry:\n%s", out)
	}
}

func TestTestCommandFailsOnMismatch(t *testing.T) {
	dir := t.TempDir()
	tui := filepath.Join(dir, "inbox.tui")
	src, err := os.ReadFile(spikeFixture)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tui, src, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "spike"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "spike", "80x24.txt"), []byte("not the real golden\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := `[{"name":"spike","file":"` + tui + `","sizes":["80x24"],"json":[],"frozen":true}]`
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	code, out, _ := runCLI("test", dir)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2\n%s", code, out)
	}
	if !strings.Contains(out, "FAIL spike") {
		t.Errorf("want a FAIL line:\n%s", out)
	}
}

// --- agents ---------------------------------------------------------------

func TestAgentsCommand(t *testing.T) {
	code, out, errw := runCLI("agents")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%s", code, errw)
	}
	if !strings.HasPrefix(out, "# Tuimark\n") {
		t.Errorf("want output to start with the AGENTS.md title:\n%s", out[:min(len(out), 80)])
	}
	if !strings.Contains(out, "## Catalog") {
		t.Errorf("missing ## Catalog section:\n%s", out)
	}
}

// V010 is a parse-pass code (SPEC §14), so `ir` must refuse a document whose
// only error is a dock+fr conflict, even on a screen that is not active and
// in a closed modal; and an unknown token in a style="" that never renders
// is still an error.
func TestIRExitsTwoOnStaticOnlyErrors(t *testing.T) {
	cases := []struct {
		name, code, src string
	}{
		{"dock and fr on an inactive screen", "V010", `<tui version="1">
  <screen id="main"><text>x</text></screen>
  <screen id="other"><box style="dock: top; height: 1fr"><text>a</text></box></screen>
</tui>`},
		{"unknown token in a closed modal", "V003", `<tui version="1">
  <screen id="main">
    <text>x</text>
    <modal id="m" open="false"><text style="color: $nope">y</text></modal>
  </screen>
</tui>`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			file := writeTemp(t, "doc.tui", c.src)
			code, out, errw := runCLI("ir", file)
			if code != 2 || out != "" || !strings.Contains(errw, c.code) {
				t.Fatalf("ir: exit %d, stdout %q, stderr %q; want exit 2, no stdout, %s", code, out, errw, c.code)
			}
			code, _, _ = runCLI("validate", file)
			if code != 2 {
				t.Fatalf("validate exit %d, want 2", code)
			}
		})
	}
}
