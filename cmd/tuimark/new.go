package main

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/abdul-hamid-achik/tuimark/internal/agentsdoc"
)

// templates holds the files `tuimark new` writes (SPEC v0.4a §15.11). Two
// of them have a placeholder: @@MODULE@@ is the project's module (package)
// name, @@NAME@@ the same name in prose, and @@VERSION@@ the bare runtime
// version.
//
//go:embed newtemplate
var templates embed.FS

// templateFile is one file of a project: the name it is written under and
// the template it is made from.
type templateFile struct{ name, src string }

var (
	baseFiles = []templateFile{
		{"app.tui", "app.tui"},
		{"theme.tcss", "theme.tcss"},
		{"sample.json", "sample.json"},
	}
	goFiles = []templateFile{
		{"main.go", "main.go.txt"},
		{"go.mod", "go.mod.txt"},
	}
	tsFiles = []templateFile{
		{"parent.ts", "parent.ts"},
		{"package.json", "package.json.txt"},
	}
)

var unsafeNameChar = regexp.MustCompile(`[^a-z0-9._-]`)

// moduleName is the name of the project's Go module or package: the base
// name of the directory path, lowercased, with each character outside
// [a-z0-9._-] replaced by "-" (SPEC v0.4a §15.11); "app" if nothing is
// left of the name. Go refuses some of what that leaves, so a leading "-"
// or "." and a trailing "." are dropped, and "-app" is added to the first
// element of a name Go reserves: a standard-library package ("log",
// "time"), "std" or "cmd", or a Windows device name ("aux", "com1"),
// which Go refuses before a dot too ("aux.x" gives "aux-app.x").
func moduleName(dir string) string {
	base := filepath.Base(filepath.Clean(dir))
	if base == "." || base == ".." || base == string(filepath.Separator) || base == "" {
		return "app"
	}
	name := unsafeNameChar.ReplaceAllString(strings.ToLower(base), "-")
	name = strings.TrimRight(strings.TrimLeft(name, "-."), ".")
	if strings.Trim(name, "-._") == "" {
		return "app"
	}
	head, rest, dotted := strings.Cut(name, ".")
	if windowsDevice(head) || !dotted && (stdRoots[head] || head == "std" || head == "cmd") {
		head += "-app"
	}
	if dotted {
		return head + "." + rest
	}
	return head
}

// stdRoots are the first elements of the standard library's import paths
// (`go list std`, Go 1.22 to 1.27): a module named one of them is an
// ambiguous import. TestNewModuleNameAvoidsStd checks the list against
// the toolchain that runs the tests.
var stdRoots = map[string]bool{
	"archive": true, "bufio": true, "builtin": true, "bytes": true, "cmp": true,
	"compress": true, "container": true, "context": true, "crypto": true,
	"database": true, "debug": true, "embed": true, "encoding": true,
	"errors": true, "expvar": true, "flag": true, "fmt": true, "go": true,
	"hash": true, "html": true, "image": true, "index": true, "internal": true,
	"io": true, "iter": true, "log": true, "maps": true, "math": true,
	"mime": true, "net": true, "os": true, "path": true, "plugin": true,
	"reflect": true, "regexp": true, "runtime": true, "slices": true,
	"sort": true, "strconv": true, "strings": true, "structs": true,
	"sync": true, "syscall": true, "testing": true, "text": true, "time": true,
	"unicode": true, "unique": true, "unsafe": true, "uuid": true, "vendor": true,
	"weak": true,
}

// windowsDevice reports whether a path element is a Windows device name,
// which Go refuses in a module path on every OS.
func windowsDevice(elem string) bool {
	switch elem {
	case "con", "prn", "aux", "nul":
		return true
	}
	return len(elem) == 4 && (strings.HasPrefix(elem, "com") || strings.HasPrefix(elem, "lpt")) && elem[3] >= '0' && elem[3] <= '9'
}

// nextCommand is the last line `tuimark new` prints: how to run what it
// wrote (SPEC v0.4a §15.11).
func nextCommand(dir string, goProject, tsProject bool) string {
	cd := "cd " + shellQuote(dir) + " && "
	switch {
	case goProject:
		return cd + "go mod tidy && go run ."
	case tsProject:
		return cd + "bun install && bun parent.ts"
	}
	return cd + "tuimark run app.tui --data sample.json"
}

var shellSafe = regexp.MustCompile(`^[A-Za-z0-9_./~@%+=:,-]+$`)

// shellQuote quotes s for the printed `cd` when a shell would read it as
// anything but a plain word: a character outside shellSafe, or a first
// character that starts a tilde expansion (~), a zsh path lookup (=), a
// job spec (%), or an option (- or +).
func shellQuote(s string) string {
	if shellSafe.MatchString(s) && !strings.ContainsAny(s[:1], "~=%-+") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// projectFiles returns the files of a project in the order they are
// written and listed (the document, its stylesheet and data, the briefing,
// then the code), each with its content.
func projectFiles(dir string, goProject, tsProject bool) ([]templateFile, map[string]string, error) {
	files := append([]templateFile(nil), baseFiles...)
	files = append(files, templateFile{name: "AGENTS.md"})
	if goProject {
		files = append(files, goFiles...)
	}
	if tsProject {
		files = append(files, tsFiles...)
	}
	module := moduleName(dir)
	content := map[string]string{"AGENTS.md": agentsdoc.ProjectMarkdown()}
	for _, f := range files {
		if f.src == "" {
			continue
		}
		b, err := templates.ReadFile("newtemplate/" + f.src)
		if err != nil {
			return nil, nil, err
		}
		s := string(b)
		s = strings.ReplaceAll(s, "@@MODULE@@", module)
		s = strings.ReplaceAll(s, "@@NAME@@", module)
		s = strings.ReplaceAll(s, "@@VERSION@@", version)
		content[f.name] = s
	}
	return files, content, nil
}

// cmdNew is `tuimark new NAME [--go | --ts]` (SPEC v0.4a §15.11): it
// creates a project in which `tuimark run` works at once. NAME is a
// directory that must not exist or must be empty; otherwise, and for
// --go with --ts, it is a usage error and nothing is written. No file is
// overwritten.
func (c *cli) cmdNew(args []string) int {
	fs := c.newFlags("new")
	goProject := fs.Bool("go", false, "add main.go and go.mod: the same app as a Go program")
	tsProject := fs.Bool("ts", false, "add parent.ts and package.json: the same app as a Bun parent through @tuimark/host")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return 1
	}
	if len(pos) != 1 {
		return c.fail(fmt.Errorf("tuimark new: want exactly one NAME, got %d", len(pos)))
	}
	if *goProject && *tsProject {
		return c.fail(fmt.Errorf("tuimark new: --go and --ts are exclusive"))
	}
	dir := pos[0]
	if dir == "" {
		return c.fail(fmt.Errorf("tuimark new: NAME is empty"))
	}
	created, err := checkNewDir(dir)
	if err != nil {
		return c.fail(err)
	}
	files, content, err := projectFiles(dir, *goProject, *tsProject)
	if err != nil {
		return c.fail(err)
	}
	if err := writeProject(dir, created, files, content); err != nil {
		return c.fail(err)
	}
	for _, f := range files {
		fmt.Fprintln(c.stdout, filepath.Join(dir, f.name))
	}
	fmt.Fprintln(c.stdout, nextCommand(dir, *goProject, *tsProject))
	return 0
}

// checkNewDir reports a usage error unless dir does not exist or is an
// empty directory. created is true when it does not exist yet.
func checkNewDir(dir string) (created bool, err error) {
	st, err := os.Stat(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return true, nil
	case err != nil:
		return false, fmt.Errorf("tuimark new: %w", err)
	case !st.IsDir():
		return false, fmt.Errorf("tuimark new: %s exists and is not a directory", dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, fmt.Errorf("tuimark new: %w", err)
	}
	if len(entries) != 0 {
		return false, fmt.Errorf("tuimark new: %s exists and is not empty", dir)
	}
	return false, nil
}

// writeProject writes the files, never overwriting one. When a write fails
// part-way it removes what it wrote (and the directory, when it created
// it), so a failed `new` leaves nothing behind.
func writeProject(dir string, created bool, files []templateFile, content map[string]string) (err error) {
	if created {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("tuimark new: %w", err)
		}
	}
	var written []string
	defer func() {
		if err == nil {
			return
		}
		for _, p := range written {
			_ = os.Remove(p)
		}
		if created {
			_ = os.Remove(dir)
		}
	}()
	for _, f := range files {
		p := filepath.Join(dir, f.name)
		out, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return fmt.Errorf("tuimark new: %w", err)
		}
		written = append(written, p)
		_, werr := out.WriteString(content[f.name])
		cerr := out.Close()
		if werr != nil {
			return fmt.Errorf("tuimark new: %w", werr)
		}
		if cerr != nil {
			return fmt.Errorf("tuimark new: %w", cerr)
		}
	}
	return nil
}
