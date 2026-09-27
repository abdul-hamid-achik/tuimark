package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// --- D7: `tuimark test --update`'s golden superset check -----------------

func writeManifest(t *testing.T, dir, manifest string) {
	t.Helper()
	writeFile(t, filepath.Join(dir, "manifest.json"), manifest)
}

func TestUpdateRefusesANonSupersetJSONGolden(t *testing.T) {
	dir := t.TempDir()
	tui := filepath.Join(dir, "app.tui")
	writeFile(t, tui, `<tui version="1"><screen id="main"><text id="t">hello</text></screen></tui>`)
	writeManifest(t, dir, `[{"name":"e","file":"`+tui+`","sizes":["20x3"],"json":["20x3"],"frozen":false}]`)

	// Seed a golden whose "text" node text is "hello", then change the
	// document so the same node's text becomes something else: not a
	// superset (an existing scalar value changed).
	code, _, errw := runCLI("test", dir, "--update")
	if code != 0 {
		t.Fatalf("seed update: exit %d; stderr=%s", code, errw)
	}
	writeFile(t, tui, `<tui version="1"><screen id="main"><text id="t">bye</text></screen></tui>`)

	code, out, errw := runCLI("test", dir, "--update")
	if code != 2 {
		t.Fatalf("exit %d, want 2; stdout=%s stderr=%s", code, out, errw)
	}
	if !strings.Contains(out, "FAIL e 20x3 (json): update removes or changes") {
		t.Errorf("want the documented FAIL line naming a pointer, got:\n%s", out)
	}
	// Neither file was rewritten: the JSON golden still says "hello", and
	// the paired text golden was not rewritten either (D7's "text goldens
	// follow the JSON check of their size" rule).
	jsonGolden, err := os.ReadFile(filepath.Join(dir, "e", "20x3.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(jsonGolden), "hello") {
		t.Errorf("the JSON golden must not have been rewritten:\n%s", jsonGolden)
	}
	textGolden, err := os.ReadFile(filepath.Join(dir, "e", "20x3.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(textGolden), "hello") {
		t.Errorf("the paired text golden must not have been rewritten either:\n%s", textGolden)
	}
}

func TestUpdateAllowBreakingWritesANonSupersetGolden(t *testing.T) {
	dir := t.TempDir()
	tui := filepath.Join(dir, "app.tui")
	writeFile(t, tui, `<tui version="1"><screen id="main"><text id="t">hello</text></screen></tui>`)
	writeManifest(t, dir, `[{"name":"e","file":"`+tui+`","sizes":["20x3"],"json":["20x3"],"frozen":false}]`)

	if code, _, errw := runCLI("test", dir, "--update"); code != 0 {
		t.Fatalf("seed update: exit %d; stderr=%s", code, errw)
	}
	writeFile(t, tui, `<tui version="1"><screen id="main"><text id="t">bye</text></screen></tui>`)

	code, out, errw := runCLI("test", dir, "--update", "--allow-breaking")
	if code != 0 {
		t.Fatalf("exit %d, want 0 with --allow-breaking; stdout=%s stderr=%s", code, out, errw)
	}
	jsonGolden, err := os.ReadFile(filepath.Join(dir, "e", "20x3.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(jsonGolden), "bye") {
		t.Errorf("--allow-breaking must write the breaking change:\n%s", jsonGolden)
	}
}

// finding 25 / SPEC v0.2 §15.5 D7: "when a JSON golden already exists and
// the new JSON is not a superset of it, nothing is written for that
// size ... A JSON golden that does not exist yet is simply written."
// Only "does not exist yet" is exempt: an existing golden that cannot be
// parsed (for example, left with merge-conflict markers) must refuse the
// update, not be silently regenerated because the superset check could
// not run.
func TestUpdateRefusesAnUnparsableExistingGolden(t *testing.T) {
	dir := t.TempDir()
	tui := filepath.Join(dir, "app.tui")
	writeFile(t, tui, `<tui version="1"><screen id="main"><text id="t">hello</text></screen></tui>`)
	writeManifest(t, dir, `[{"name":"e","file":"`+tui+`","sizes":["20x3"],"json":["20x3"],"frozen":false}]`)

	// A golden left with merge-conflict markers: valid enough to exist,
	// not valid enough to decode as JSON.
	corrupt := "{\n<<<<<<< ours\n\"cols\": 20,\n=======\n\"cols\": 21,\n>>>>>>> theirs\n"
	if err := os.MkdirAll(filepath.Join(dir, "e"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "e", "20x3.json"), corrupt)
	writeFile(t, filepath.Join(dir, "e", "20x3.txt"), "unrelated stale text\n")

	code, out, errw := runCLI("test", dir, "--update")
	if code != 1 {
		t.Fatalf("exit %d, want 1 (an I/O error, per §15.5's exit status table); stdout=%s stderr=%s", code, out, errw)
	}
	if !strings.Contains(out, "FAIL e 20x3 (json)") {
		t.Errorf("want a FAIL line for the unparsable golden, got:\n%s", out)
	}

	jsonGolden, err := os.ReadFile(filepath.Join(dir, "e", "20x3.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(jsonGolden) != corrupt {
		t.Errorf("the unparsable golden must not have been overwritten:\n%s", jsonGolden)
	}
	textGolden, err := os.ReadFile(filepath.Join(dir, "e", "20x3.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(textGolden) != "unrelated stale text\n" {
		t.Errorf("the paired text golden must not have been overwritten either:\n%s", textGolden)
	}

	// --allow-breaking still skips the check entirely and regenerates it.
	code, out, errw = runCLI("test", dir, "--update", "--allow-breaking")
	if code != 0 {
		t.Fatalf("--allow-breaking: exit %d, want 0; stdout=%s stderr=%s", code, out, errw)
	}
	jsonGolden, err = os.ReadFile(filepath.Join(dir, "e", "20x3.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(jsonGolden), "<<<<<<<") {
		t.Errorf("--allow-breaking must regenerate the golden despite it being unparsable:\n%s", jsonGolden)
	}
}

func TestUpdateAcceptsASupersetChange(t *testing.T) {
	// A document whose golden is byte-identical, and does not shrink,
	// is trivially a superset of itself: --update must accept it and
	// still write (SPEC v0.2 §15.5's "new ⊇ old" holds for equal JSON).
	dir := t.TempDir()
	tui := filepath.Join(dir, "app.tui")
	writeFile(t, tui, `<tui version="1"><screen id="main"><text id="t">hello</text></screen></tui>`)
	manifestNoStyles := `[{"name":"e","file":"` + tui + `","sizes":["20x3"],"json":["20x3"],"frozen":false}]`
	writeManifest(t, dir, manifestNoStyles)

	if code, _, errw := runCLI("test", dir, "--update"); code != 0 {
		t.Fatalf("seed update: exit %d; stderr=%s", code, errw)
	}

	// Turning on the manifest's styles option only *adds* the "theme" and
	// "styles" object members: every existing field (cols, rows, nodes,
	// grid, ...) is unchanged and no array changes length, so this is a
	// superset and --update must accept it without --allow-breaking.
	writeManifest(t, dir, `[{"name":"e","file":"`+tui+`","sizes":["20x3"],"json":["20x3"],"frozen":false,"styles":true}]`)
	code, out, errw := runCLI("test", dir, "--update")
	if code != 0 {
		t.Fatalf("exit %d, want 0 for a pure member addition; stdout=%s stderr=%s", code, out, errw)
	}
	b, err := os.ReadFile(filepath.Join(dir, "e", "20x3.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"styles"`) {
		t.Errorf("the updated golden must now carry \"styles\":\n%s", b)
	}
}

func TestFrozenEntryNeverWritten(t *testing.T) {
	dir := t.TempDir()
	tui := filepath.Join(dir, "app.tui")
	writeFile(t, tui, `<tui version="1"><screen id="main"><text id="t">hello</text></screen></tui>`)
	writeManifest(t, dir, `[{"name":"e","file":"`+tui+`","sizes":["20x3"],"json":["20x3"],"frozen":true}]`)

	// No golden exists yet: --update on a frozen entry must not create one,
	// and the run must report the missing-golden compare failure, not a
	// write.
	code, out, errw := runCLI("test", dir, "--update")
	if code == 0 {
		t.Fatalf("frozen entry with no golden must not silently pass; stdout=%s stderr=%s", out, errw)
	}
	if _, err := os.Stat(filepath.Join(dir, "e", "20x3.txt")); err == nil {
		t.Error("a frozen entry must never get a golden written for it")
	}
}

// finding 26 / SPEC v0.2 §15.5: "The output has one line per golden, PASS
// NAME SIZE (text|json) or FAIL NAME SIZE (text|json): HINT." There is no
// third, "updated" form: before the fix, `--update` printed "PASS NAME
// SIZE (text|json, updated)" even when the golden was already
// byte-identical, so a tool matching the literal §15.5 line grammar would
// miss every --update PASS line.
func TestUpdatePassLineMatchesSpecGrammarExactly(t *testing.T) {
	dir := t.TempDir()
	tui := filepath.Join(dir, "app.tui")
	writeFile(t, tui, `<tui version="1"><screen id="main"><text id="t">hello</text></screen></tui>`)
	writeManifest(t, dir, `[{"name":"e","file":"`+tui+`","sizes":["20x3"],"json":["20x3"],"frozen":false}]`)

	code, out, errw := runCLI("test", dir, "--update")
	if code != 0 {
		t.Fatalf("exit %d, want 0; stderr=%s", code, errw)
	}
	grammar := regexp.MustCompile(`^PASS \S+ \S+ \((text|json)\)$`)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 PASS lines, got %d:\n%s", len(lines), out)
	}
	for _, line := range lines {
		if !grammar.MatchString(line) {
			t.Errorf("line %q does not match the §15.5 grammar PASS NAME SIZE (text|json)", line)
		}
	}

	// A second --update, with nothing changed, must print the same form.
	code, out, errw = runCLI("test", dir, "--update")
	if code != 0 {
		t.Fatalf("second --update: exit %d, want 0; stderr=%s", code, errw)
	}
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if !grammar.MatchString(line) {
			t.Errorf("second --update, line %q does not match the §15.5 grammar", line)
		}
	}
}

// finding 28 / SPEC v0.2 §28.24 ("theme, input, script, styles are the
// only additions in 0.2a"): an unknown manifest member (a typo, or the
// 0.2b-planned "cells") must be a manifest error, not silently ignored.
func TestManifestUnknownMemberIsAManifestError(t *testing.T) {
	dir := t.TempDir()
	tui := filepath.Join(dir, "app.tui")
	writeFile(t, tui, `<tui version="1"><screen id="main"><text id="t">hello</text></screen></tui>`)
	writeManifest(t, dir, `[{"name":"e","file":"`+tui+`","sizes":["20x3"],"json":["20x3"],"frozen":false,"stlyes":true}]`)

	code, out, errw := runCLI("test", dir, "--update")
	if code != 1 {
		t.Fatalf("exit %d, want 1; stdout=%s stderr=%s", code, out, errw)
	}
	if _, err := os.Stat(filepath.Join(dir, "e", "20x3.json")); err == nil {
		t.Error("a manifest decode error must not write any golden")
	}
}

// finding 28 / SPEC v0.2 §15.1 ("--cols and --rows are 1 to 1000"), §15.4
// ("resize:COLSxROWS ... each 1 to 1000"): a manifest size outside that
// range must be a manifest error too, since no other tool (dump --cols,
// play resize:) can ever render a document at that size.
func TestManifestOutOfRangeSizeIsAManifestError(t *testing.T) {
	dir := t.TempDir()
	tui := filepath.Join(dir, "app.tui")
	writeFile(t, tui, `<tui version="1"><screen id="main"><text id="t">hello</text></screen></tui>`)
	for _, size := range []string{"0x5", "-3x2", "5000x1"} {
		t.Run(size, func(t *testing.T) {
			d := t.TempDir()
			writeManifest(t, d, `[{"name":"e","file":"`+tui+`","sizes":["`+size+`"],"json":[],"frozen":false}]`)
			code, out, errw := runCLI("test", d, "--update")
			if code != 1 {
				t.Fatalf("size %s: exit %d, want 1; stdout=%s stderr=%s", size, code, out, errw)
			}
		})
	}
}

func TestManifestThemeField(t *testing.T) {
	dir := t.TempDir()
	tui := filepath.Join(dir, "app.tui")
	writeFile(t, tui, `<tui version="1"><screen id="main"><text id="t">hello</text></screen></tui>`)
	writeManifest(t, dir, `[{"name":"e","file":"`+tui+`","sizes":["20x3"],"json":["20x3"],"frozen":false,"theme":"light","styles":true}]`)

	if code, _, errw := runCLI("test", dir, "--update"); code != 0 {
		t.Fatalf("exit %d; stderr=%s", code, errw)
	}
	b, err := os.ReadFile(filepath.Join(dir, "e", "20x3.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"theme": "light"`) {
		t.Errorf("manifest theme=light + styles=true must produce a golden with theme:light:\n%s", b)
	}
	// Re-running (no --update) must pass against what was just written.
	code, out, errw := runCLI("test", dir)
	if code != 0 {
		t.Fatalf("re-run exit %d, want 0; stdout=%s stderr=%s", code, out, errw)
	}
}

// finding 27 / SPEC v0.2 §15.1 ("--theme takes dark or light ... any other
// value ... is a usage error"): a manifest entry with an explicit
// `"theme": ""` is given a theme, not lacking one, so it must be a usage
// error like `--theme ""`, not silently treated as "no theme field".
func TestManifestEmptyThemeFieldIsAManifestError(t *testing.T) {
	dir := t.TempDir()
	tui := filepath.Join(dir, "app.tui")
	writeFile(t, tui, `<tui version="1"><screen id="main"><text id="t">hello</text></screen></tui>`)
	writeManifest(t, dir, `[{"name":"e","file":"`+tui+`","sizes":["20x3"],"json":["20x3"],"frozen":false,"theme":""}]`)

	code, out, errw := runCLI("test", dir, "--update")
	if code != 1 {
		t.Fatalf("exit %d, want 1; stdout=%s stderr=%s", code, out, errw)
	}
	if !strings.Contains(out, "dark or light") {
		t.Errorf("want a dark-or-light usage error naming the entry, got:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "e", "20x3.json")); err == nil {
		t.Error("an entry that errors on theme must not have any golden written")
	}
}

// finding 29 / SPEC §21 test 29 ("manifest input, script, theme, and
// styles work"), §15.5 (manifest `script`: "a play script path;
// exclusive with input"). Before this test, nothing in the suite drove
// `tuimark test` through a manifest `script` field: only `play --script`
// itself was covered.
func TestManifestScriptField(t *testing.T) {
	dir := t.TempDir()
	tui := filepath.Join(dir, "app.tui")
	writeFile(t, tui, playFixtureTUI)
	dataPath := filepath.Join(dir, "data.json")
	writeFile(t, dataPath, playFixtureData)
	scriptPath := filepath.Join(dir, "steps.ndjson")
	writeFile(t, scriptPath, `{"key":"tab"}`+"\n"+`{"key":"down"}`+"\n")
	writeManifest(t, dir, `[{"name":"e","file":"`+tui+`","data":"`+dataPath+`","sizes":["40x6"],"json":["40x6"],"frozen":false,"script":"`+scriptPath+`"}]`)

	if code, out, errw := runCLI("test", dir, "--update"); code != 0 {
		t.Fatalf("exit %d; stdout=%s stderr=%s", code, out, errw)
	}
	textGolden, err := os.ReadFile(filepath.Join(dir, "e", "40x6.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(textGolden), "=== events ===") {
		t.Errorf("a `script`-driven entry's golden must be play output (with an events section):\n%s", textGolden)
	}
	jsonGolden, err := os.ReadFile(filepath.Join(dir, "e", "40x6.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(jsonGolden), `"events"`) {
		t.Errorf("a `script`-driven entry's JSON golden must carry \"events\":\n%s", jsonGolden)
	}
	// Re-running (no --update) must reproduce the same play output.
	code, out, errw := runCLI("test", dir)
	if code != 0 {
		t.Fatalf("re-run exit %d, want 0; stdout=%s stderr=%s", code, out, errw)
	}
}

// finding 29: `input` and `script` are exclusive (SPEC §15.5), and a
// manifest entry setting both must be a manifest error, exit 1, with no
// golden written — the manifest-level equivalent of `play`'s own
// --input/--script exclusivity check.
func TestManifestInputAndScriptAreMutuallyExclusive(t *testing.T) {
	dir := t.TempDir()
	tui := filepath.Join(dir, "app.tui")
	writeFile(t, tui, playFixtureTUI)
	dataPath := filepath.Join(dir, "data.json")
	writeFile(t, dataPath, playFixtureData)
	scriptPath := filepath.Join(dir, "steps.ndjson")
	writeFile(t, scriptPath, `{"key":"tab"}`+"\n")
	writeManifest(t, dir, `[{"name":"e","file":"`+tui+`","data":"`+dataPath+`","sizes":["40x6"],"json":["40x6"],"frozen":false,"input":"tab","script":"`+scriptPath+`"}]`)

	code, out, errw := runCLI("test", dir, "--update")
	if code != 1 {
		t.Fatalf("exit %d, want 1; stdout=%s stderr=%s", code, out, errw)
	}
	if !strings.Contains(out, "input and script are mutually exclusive") {
		t.Errorf("want the mutual-exclusivity error, got:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "e", "40x6.json")); err == nil {
		t.Error("a manifest error must not write any golden")
	}
}

func TestManifestInputField(t *testing.T) {
	dir := t.TempDir()
	tui := filepath.Join(dir, "app.tui")
	writeFile(t, tui, playFixtureTUI)
	dataPath := filepath.Join(dir, "data.json")
	writeFile(t, dataPath, playFixtureData)
	writeManifest(t, dir, `[{"name":"e","file":"`+tui+`","data":"`+dataPath+`","sizes":["40x6"],"json":["40x6"],"frozen":false,"input":"tab down"}]`)

	if code, out, errw := runCLI("test", dir, "--update"); code != 0 {
		t.Fatalf("exit %d; stdout=%s stderr=%s", code, out, errw)
	}
	textGolden, err := os.ReadFile(filepath.Join(dir, "e", "40x6.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(textGolden), "=== events ===") {
		t.Errorf("an `input`-driven entry's golden must be play output (with an events section):\n%s", textGolden)
	}
	jsonGolden, err := os.ReadFile(filepath.Join(dir, "e", "40x6.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(jsonGolden), `"events"`) {
		t.Errorf("an `input`-driven entry's JSON golden must carry \"events\":\n%s", jsonGolden)
	}
	// Re-running (no --update) must reproduce the same play output.
	code, out, errw := runCLI("test", dir)
	if code != 0 {
		t.Fatalf("re-run exit %d, want 0; stdout=%s stderr=%s", code, out, errw)
	}
}
