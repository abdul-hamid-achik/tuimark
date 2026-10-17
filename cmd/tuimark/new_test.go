package main

// SPEC v0.4a §21 test 120: `tuimark new NAME [--go | --ts]`.

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// newProject runs `tuimark new` in a fresh temp directory and returns the
// project's directory and the command's stdout lines.
func newProject(t *testing.T, name string, flags ...string) (dir string, lines []string) {
	t.Helper()
	dir = filepath.Join(t.TempDir(), name)
	code, out, errw := runCLI(append([]string{"new", dir}, flags...)...)
	if code != 0 || errw != "" {
		t.Fatalf("new %s %v: exit %d, stderr %q", name, flags, code, errw)
	}
	return dir, strings.Split(strings.TrimSuffix(out, "\n"), "\n")
}

func readFileT(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// the files of each kind of project, in the order new lists them.
var (
	newPlain = []string{"app.tui", "theme.tcss", "sample.json", "AGENTS.md"}
	newGo    = append(append([]string(nil), newPlain...), "main.go", "go.mod")
	newTS    = append(append([]string(nil), newPlain...), "parent.ts", "package.json")
)

func TestNewWritesTheFilesAndListsThem(t *testing.T) {
	for _, c := range []struct {
		name  string
		flags []string
		files []string
		next  string
	}{
		{"plain", nil, newPlain, "cd %s && tuimark run app.tui --data sample.json"},
		{"go", []string{"--go"}, newGo, "cd %s && go mod tidy && go run ."},
		{"ts", []string{"--ts"}, newTS, "cd %s && bun install && bun parent.ts"},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir, lines := newProject(t, "proj", c.flags...)
			var want []string
			for _, f := range c.files {
				want = append(want, filepath.Join(dir, f))
				if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
					t.Errorf("%s was not written: %v", f, err)
				}
			}
			want = append(want, strings.Replace(c.next, "%s", dir, 1))
			if strings.Join(lines, "\n") != strings.Join(want, "\n") {
				t.Errorf("output:\n%s\nwant:\n%s", strings.Join(lines, "\n"), strings.Join(want, "\n"))
			}
			entries, _ := os.ReadDir(dir)
			if len(entries) != len(c.files) {
				t.Errorf("the directory holds %d entries, want %d", len(entries), len(c.files))
			}

			// AGENTS.md is the project briefing, and names no repository
			// path.
			_, briefing, _ := runCLI("agents", "--project")
			if got := readFileT(t, filepath.Join(dir, "AGENTS.md")); got != briefing {
				t.Error("AGENTS.md differs from `tuimark agents --project`")
			}
			for _, bad := range []string{"internal/", "cmd/", "examples/", "testdata", "schema/", "docs/", "sdk/", "editors/", "specs/", "README.md", "Taskfile", "bin/tuimark", "~/notes", "task verify"} {
				if strings.Contains(briefing, bad) {
					t.Errorf("the project briefing names the repository path %q", bad)
				}
			}
		})
	}
}

// The generated document passes validate --data sample.json with no
// diagnostic at 40, 80, and 120 columns under both themes.
func TestNewDocumentIsClean(t *testing.T) {
	dir, _ := newProject(t, "clean")
	doc, data := filepath.Join(dir, "app.tui"), filepath.Join(dir, "sample.json")
	for _, theme := range []string{"dark", "light"} {
		code, out, errw := runCLI("validate", doc, "--data", data, "--theme", theme)
		if code != 0 || out != "ok\n" || errw != "" {
			t.Errorf("validate --theme %s: exit %d, stdout %q, stderr %q", theme, code, out, errw)
		}
		code, out, _ = runCLI("validate", doc, "--data", data, "--theme", theme, "--strict")
		if code != 0 || out != "ok\n" {
			t.Errorf("validate --strict --theme %s: exit %d, stdout %q", theme, code, out)
		}
		for _, cols := range []string{"40", "80", "120"} {
			code, out, _ := runCLI("dump", doc, "--data", data, "--cols", cols, "--rows", "24", "--theme", theme, "--format", "json")
			if code != 0 {
				t.Errorf("dump %s columns, %s: exit %d", cols, theme, code)
			}
			var d struct {
				OK     bool              `json:"ok"`
				Errors []json.RawMessage `json:"errors"`
			}
			if err := json.Unmarshal([]byte(out), &d); err != nil {
				t.Fatal(err)
			}
			if !d.OK || len(d.Errors) != 0 {
				t.Errorf("dump %s columns, %s: ok=%v diagnostics=%s", cols, theme, d.OK, d.Errors)
			}
		}
	}
	// theme="auto": validate checks both themes, dark first.
	if !strings.Contains(readFileT(t, doc), `theme="auto"`) {
		t.Error(`the template is not theme="auto"`)
	}
	// Every action the document names is one a handler can implement: the
	// catalog lists no unregistered built-in surprise.
	code, out, _ := runCLI("validate", doc, "--data", data, "--json")
	if code != 0 {
		t.Fatalf("validate --json: exit %d", code)
	}
	var v struct {
		Actions []struct {
			Name    string `json:"name"`
			Builtin bool   `json:"builtin"`
		} `json:"actions"`
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatal(err)
	}
	var host []string
	for _, a := range v.Actions {
		if !a.Builtin {
			host = append(host, a.Name)
		}
	}
	if got := strings.Join(host, ","); got != "open,refresh,search,select" {
		t.Errorf("host actions = %s, want open,refresh,search,select", got)
	}
	// The template plays: the keymap fires its rows, and the hints show.
	code, out, errw := runCLI("play", doc, "--data", data, "--input", "enter /", "--format", "json")
	if code != 0 {
		t.Fatalf("play: exit %d, stderr %s", code, errw)
	}
	if !strings.Contains(out, `"action": "open"`) {
		t.Errorf("enter did not fire open:\n%.600s", out)
	}
}

// The files are the template's, with the name filled in.
func TestNewGoAndTSContent(t *testing.T) {
	dir, _ := newProject(t, "My App", "--go")
	gomod := readFileT(t, filepath.Join(dir, "go.mod"))
	for _, want := range []string{"module my-app\n", "go 1.22", "require github.com/abdul-hamid-achik/tuimark v" + version + "\n"} {
		if !strings.Contains(gomod, want) {
			t.Errorf("go.mod lacks %q:\n%s", want, gomod)
		}
	}
	mainGo := readFileT(t, filepath.Join(dir, "main.go"))
	for _, action := range []string{"select", "open", "search", "refresh", "quit"} {
		if !strings.Contains(mainGo, `ui.On("`+action+`"`) {
			t.Errorf("main.go registers no handler for %s", action)
		}
	}
	if strings.Contains(mainGo, "@@") || strings.Contains(gomod, "@@") {
		t.Error("a placeholder was left in a Go file")
	}

	tsDir, _ := newProject(t, "ts-app", "--ts")
	ts := readFileT(t, filepath.Join(tsDir, "parent.ts"))
	for _, want := range []string{`from "@tuimark/host"`, "spawnHost(", "ui.on(", "ui.set(", "ui.batch(", "await ui.exited"} {
		if !strings.Contains(ts, want) {
			t.Errorf("parent.ts lacks %q", want)
		}
	}
	var pkg struct {
		Name         string            `json:"name"`
		Dependencies map[string]string `json:"dependencies"`
	}
	if err := json.Unmarshal([]byte(readFileT(t, filepath.Join(tsDir, "package.json"))), &pkg); err != nil {
		t.Fatalf("package.json: %v", err)
	}
	if pkg.Name != "ts-app" || pkg.Dependencies["@tuimark/host"] == "" {
		t.Errorf("package.json = %+v", pkg)
	}
}

func TestNewModuleName(t *testing.T) {
	for in, want := range map[string]string{
		"My App":          "my-app",
		"app":             "app",
		"/tmp/x/Foo_Bar":  "foo_bar",
		"a/b/c.d-e":       "c.d-e",
		"UPPER":           "upper",
		"sp ace/ño":       "o",
		"x y z":           "x-y-z",
		".":               "app",
		"..":              "app",
		"":                "app",
		"/":               "app",
		"---":             "app",
		"my app/":         "my-app",
		"proj@v1+beta":    "proj-v1-beta",
		"tuimark/new-app": "new-app",
		"-zz":             "zz",
		"foo.":            "foo",
		".foo":            "foo",
		"log":             "log-app",
		"time":            "time-app",
		"log.v2":          "log.v2",
		"std":             "std-app",
		"cmd":             "cmd-app",
		"aux":             "aux-app",
		"aux.x":           "aux-app.x",
		"COM1":            "com1-app",
		"comx":            "comx",
	} {
		if got := moduleName(in); got != want {
			t.Errorf("moduleName(%q) = %q, want %q", in, got, want)
		}
	}
}

// A usage error writes nothing: a non-empty directory, a file, --go with
// --ts, a missing or extra NAME.
func TestNewUsageErrorsWriteNothing(t *testing.T) {
	base := t.TempDir()

	full := filepath.Join(base, "full")
	if err := os.Mkdir(full, 0o755); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(full, "app.tui")
	writeFile(t, keep, "mine")
	if code, out, errw := runCLI("new", full); code != 1 || out != "" || !strings.Contains(errw, "not empty") {
		t.Errorf("non-empty dir: exit %d, stdout %q, stderr %q", code, out, errw)
	}
	if readFileT(t, keep) != "mine" {
		t.Error("an existing file was overwritten")
	}
	if entries, _ := os.ReadDir(full); len(entries) != 1 {
		t.Errorf("the non-empty directory now holds %d entries", len(entries))
	}

	file := filepath.Join(base, "afile")
	writeFile(t, file, "x")
	if code, _, errw := runCLI("new", file); code != 1 || !strings.Contains(errw, "not a directory") {
		t.Errorf("a file: exit %d, stderr %q", code, errw)
	}

	both := filepath.Join(base, "both")
	if code, out, errw := runCLI("new", both, "--go", "--ts"); code != 1 || out != "" || !strings.Contains(errw, "exclusive") {
		t.Errorf("--go --ts: exit %d, stdout %q, stderr %q", code, out, errw)
	}
	if _, err := os.Stat(both); err == nil {
		t.Error("--go --ts created the directory")
	}

	if code, _, _ := runCLI("new"); code != 1 {
		t.Errorf("no NAME: exit %d, want 1", code)
	}
	one, two := filepath.Join(base, "one"), filepath.Join(base, "two")
	if code, _, _ := runCLI("new", one, two); code != 1 {
		t.Errorf("two NAMEs: exit %d, want 1", code)
	}
	for _, d := range []string{one, two} {
		if _, err := os.Stat(d); err == nil {
			t.Errorf("%s was created", d)
		}
	}
	if code, _, _ := runCLI("new", filepath.Join(base, "x"), "--nope"); code != 1 {
		t.Errorf("an unknown flag: exit %d, want 1", code)
	}

	// An existing empty directory is fine, and so is a nested new one.
	empty := filepath.Join(base, "empty")
	if err := os.Mkdir(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	if code, _, errw := runCLI("new", empty); code != 0 {
		t.Errorf("empty dir: exit %d, stderr %q", code, errw)
	}
	if code, _, errw := runCLI("new", filepath.Join(base, "deep", "er", "proj")); code != 0 {
		t.Errorf("nested dir: exit %d, stderr %q", code, errw)
	}
	// And a second run on the project it just made is a usage error: no
	// file is ever overwritten.
	if code, _, _ := runCLI("new", empty); code != 1 {
		t.Errorf("a second new on the same directory: exit %d, want 1", code)
	}
}

// The --go project builds with a replace to this repository, and NAME
// "My App" gives the module my-app.
func TestNewGoProjectBuilds(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a Go module")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("no go toolchain on PATH")
	}
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	dir, _ := newProject(t, "My App", "--go")
	gomod := filepath.Join(dir, "go.mod")
	if !strings.Contains(readFileT(t, gomod), "module my-app\n") {
		t.Fatalf("go.mod:\n%s", readFileT(t, gomod))
	}
	f, err := os.OpenFile(gomod, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("\nreplace github.com/abdul-hamid-achik/tuimark => " + repo + "\n"); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	env := append(os.Environ(), "GOFLAGS=-mod=mod", "GOPROXY=off", "GOSUMDB=off", "GOWORK=off")
	for _, args := range [][]string{{"mod", "tidy"}, {"vet", "./..."}, {"build", "-o", filepath.Join(dir, "app-bin"), "."}} {
		cmd := exec.Command("go", args...)
		cmd.Dir = dir
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("go %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	// gofmt-clean, like everything the repository ships.
	cmd := exec.Command("gofmt", "-l", ".")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil || len(out) != 0 {
		t.Errorf("gofmt -l: %v %s", err, out)
	}
}

// The --ts project type-checks against @tuimark/host (sdk/ts). The
// generated parent.ts is the embedded template byte for byte, which
// sdk/ts's own type-check (tsconfig.template.json, run by `task sdk-test`
// and the sdk-ts CI job) also covers; this test checks the generated copy
// with sdk/ts's TypeScript when `task sdk-test` (or `bun install` in sdk/ts)
// has installed it, and skips otherwise.
func TestNewTSProjectTypeChecks(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the TypeScript compiler")
	}
	sdk, err := filepath.Abs("../../sdk/ts")
	if err != nil {
		t.Fatal(err)
	}
	dir, _ := newProject(t, "ts-app", "--ts")
	tmpl, err := templates.ReadFile("newtemplate/parent.ts")
	if err != nil {
		t.Fatal(err)
	}
	if got := readFileT(t, filepath.Join(dir, "parent.ts")); got != string(tmpl) {
		t.Fatalf("the generated parent.ts differs from newtemplate/parent.ts, which sdk/ts type-checks")
	}
	tsc := filepath.Join(sdk, "node_modules", ".bin", "tsc")
	if _, err := os.Stat(tsc); err != nil {
		t.Skip("sdk/ts/node_modules has no tsc: run `task sdk-test` (needs bun) first")
	}
	cfg := map[string]any{
		"compilerOptions": map[string]any{
			"target":                     "ES2022",
			"module":                     "ESNext",
			"moduleResolution":           "bundler",
			"lib":                        []string{"ES2023"},
			"types":                      []string{"bun"},
			"typeRoots":                  []string{filepath.Join(sdk, "node_modules", "@types")},
			"allowImportingTsExtensions": true,
			"strict":                     true,
			"skipLibCheck":               true,
			"noEmit":                     true,
			"paths":                      map[string][]string{"@tuimark/host": {filepath.Join(sdk, "src", "index.ts")}},
		},
		"files": []string{"parent.ts"},
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tsconfig.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(tsc, "-p", "tsconfig.json")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("tsc: %v\n%s", err, out)
	}
}

// Every name Go refuses as a module path that moduleName could leave is
// rewritten: a module of each builds, offline. The standard library's
// roots come from the toolchain that runs the test, so a root a newer Go
// adds fails here until stdRoots has it.
func TestNewModuleNameAvoidsStd(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the go tool")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("no go toolchain on PATH")
	}
	out, err := exec.Command("go", "list", "std").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, pkg := range strings.Fields(string(out)) {
		root, _, _ := strings.Cut(pkg, "/")
		if !stdRoots[root] {
			t.Errorf("stdRoots lacks %q (from %q)", root, pkg)
		}
	}
	for _, name := range []string{"log", "time", "std", "cmd", "aux", "aux.x", "nul", "lpt3", "-zz", "foo.", ".foo", "sp ace/ño"} {
		dir := t.TempDir()
		mod := moduleName(name)
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module "+mod+"\n\ngo 1.22\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		src := "package main\n\nimport (\n\t\"fmt\"\n\t\"log\"\n)\n\nfunc main() { fmt.Println(); log.Print() }\n"
		if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("go", "build", "-o", os.DevNull, ".")
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOPROXY=off", "GOWORK=off")
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("new %q gives module %q, which does not build: %v\n%s", name, mod, err, b)
		}
	}
}

// The printed cd quotes a directory a shell would not read as a plain
// word.
func TestNewNextCommandQuotes(t *testing.T) {
	for in, want := range map[string]string{
		"app":      "cd app && ",
		"~root":    "cd '~root' && ",
		"=zz":      "cd '=zz' && ",
		"-zz":      "cd '-zz' && ",
		"+x":       "cd '+x' && ",
		"%1":       "cd '%1' && ",
		"a~b":      "cd a~b && ",
		"my app":   "cd 'my app' && ",
		"it's":     `cd 'it'\''s' && `,
		"/tmp/x-y": "cd /tmp/x-y && ",
	} {
		if got := nextCommand(in, false, false); !strings.HasPrefix(got, want) {
			t.Errorf("nextCommand(%q) = %q, want the prefix %q", in, got, want)
		}
	}
}
