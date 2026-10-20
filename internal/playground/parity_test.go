//go:build !js

package playground

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// The playground's frames are the CLI's: for every .tui under examples/
// and kit/components/ the session's dump, at 80x24 and 120x40, in both
// themes where asked, is byte for byte what `tuimark dump --format json
// --styles --cells` prints, and validate is `tuimark validate --json`.
// Run natively (it execs the CLI); the wasm build is checked by
// cmd/tuimark-wasm/testdata/smoke.mjs.

var (
	cliOnce sync.Once
	cliPath string
	cliErr  error
	cliTmp  string
)

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// cli builds bin/tuimark once into a temp dir and returns its path.
func cli(t *testing.T) string {
	t.Helper()
	cliOnce.Do(func() {
		cliTmp, cliErr = os.MkdirTemp("", "tuimark-parity")
		if cliErr != nil {
			return
		}
		cliPath = filepath.Join(cliTmp, "tuimark")
		cmd := exec.Command("go", "build", "-o", cliPath, "./cmd/tuimark")
		cmd.Dir = repoRoot(t)
		if out, err := cmd.CombinedOutput(); err != nil {
			cliErr = errors.New("go build ./cmd/tuimark: " + err.Error() + "\n" + string(out))
		}
	})
	if cliErr != nil {
		t.Fatal(cliErr)
	}
	return cliPath
}

func TestMain(m *testing.M) {
	code := m.Run()
	if cliTmp != "" {
		os.RemoveAll(cliTmp)
	}
	os.Exit(code)
}

// run execs the CLI from the repository root, so the paths it prints
// match the ones the session sees. Exit status 2 means diagnostics.
func run(t *testing.T, args ...string) []byte {
	t.Helper()
	cmd := exec.Command(cli(t), args...)
	cmd.Dir = repoRoot(t)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	var ee *exec.ExitError
	if err != nil && !(errors.As(err, &ee) && ee.ExitCode() == 2) {
		t.Fatalf("tuimark %s: %v\n%s", strings.Join(args, " "), err, errb.String())
	}
	return out.Bytes()
}

// repoFiles reads every .tui, .tcss, and .json file under examples/ and
// kit/, keyed by repo-relative slash paths, except mock.json (the CLI's
// dump has no mock) and node_modules.
func repoFiles(t *testing.T) map[string]string {
	t.Helper()
	root := repoRoot(t)
	files := map[string]string{}
	for _, dir := range []string{"examples", "kit"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}
			switch filepath.Ext(p) {
			case ".tui", ".tcss", ".json":
			default:
				return nil
			}
			if d.Name() == "mock.json" {
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, p)
			files[filepath.ToSlash(rel)] = string(b)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return files
}

// sampleOf mirrors the data convention of resolve.
func sampleOf(files map[string]string, entry string) string {
	if p := strings.TrimSuffix(entry, ".tui") + ".sample.json"; files[p] != "" {
		return p
	}
	dir := filepath.ToSlash(filepath.Dir(entry))
	if p := dir + "/sample.json"; files[p] != "" {
		return p
	}
	return ""
}

func rawDump(t *testing.T, out string) []byte {
	t.Helper()
	var r struct {
		Dump  json.RawMessage `json:"dump"`
		Error string          `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if r.Error != "" {
		t.Fatalf("error: %s", r.Error)
	}
	var b bytes.Buffer
	if err := json.Indent(&b, r.Dump, "", "  "); err != nil {
		t.Fatal(err)
	}
	b.WriteByte('\n')
	return b.Bytes()
}

func cliDump(t *testing.T, entry, data, theme string, cols, rows int) []byte {
	args := []string{"dump", entry, "--format", "json", "--styles", "--cells", "--cols", strconv.Itoa(cols), "--rows", strconv.Itoa(rows)}
	if data != "" {
		args = append(args, "--data", data)
	}
	if theme != "" {
		args = append(args, "--theme", theme)
	}
	return run(t, args...)
}

func firstDiff(a, b []byte) string {
	la, lb := strings.Split(string(a), "\n"), strings.Split(string(b), "\n")
	for i := 0; i < len(la) && i < len(lb); i++ {
		if la[i] != lb[i] {
			return "line " + strconv.Itoa(i+1) + ":\n  playground: " + la[i] + "\n  cli:        " + lb[i]
		}
	}
	return "lengths differ: " + strconv.Itoa(len(la)) + " vs " + strconv.Itoa(len(lb)) + " lines"
}

// isFragment reports whether a .tui file's root element is <fragment>: a
// file for documents to include (kit/kit.tui), not a document to open.
func isFragment(src string) bool {
	d := xml.NewDecoder(strings.NewReader(src))
	for {
		tok, err := d.Token()
		if err != nil {
			return false
		}
		if se, ok := tok.(xml.StartElement); ok {
			return se.Name.Local == "fragment"
		}
	}
}

func TestIsFragment(t *testing.T) {
	for src, want := range map[string]bool{
		"<!-- a <tui> comment -->\n<fragment><template id=\"x\"><text>x</text></template></fragment>": true,
		"<?xml version=\"1.0\"?>\n<fragment/>":                                                        true,
		"<!-- <fragment> -->\n<tui><screen id=\"s\"/></tui>":                                          false,
		"<app><text>x</text></app>":                                                                   false,
		"":                                                                                            false,
	} {
		if got := isFragment(src); got != want {
			t.Errorf("isFragment(%q) = %v, want %v", src, got, want)
		}
	}
	if files := repoFiles(t); !isFragment(files["kit/kit.tui"]) {
		t.Error("kit/kit.tui is a fragment")
	}
}

func TestDumpParityWithCLI(t *testing.T) {
	files := repoFiles(t)
	var entries []string
	for p := range files {
		// A fragment is checked through the documents that include it
		// (examples/kit-gallery includes kit/kit.tui); opened on its own it
		// is V005 in both, but the CLI's validate checks it as a fragment.
		if strings.HasSuffix(p, ".tui") && !isFragment(files[p]) {
			entries = append(entries, p)
		}
	}
	sort.Strings(entries)
	if len(entries) < 40 {
		t.Fatalf("only %d presets found", len(entries))
	}
	themed := map[string]bool{"examples/tasks/app.tui": true, "examples/kit-gallery/app.tui": true, "kit/components/card.tui": true, "examples/pomodoro/app.tui": true}
	sizes := [][2]int{{80, 24}, {120, 40}, {80, 24}}
	for _, entry := range entries {
		entry := entry
		t.Run(entry, func(t *testing.T) {
			data := sampleOf(files, entry)
			m := NewManager()
			// The defaults must pick the CLI's data file by themselves.
			firstJSON := m.Call("open", js(t, files), js(t, Options{Entry: entry}))
			var opened struct {
				Session int `json:"session"`
			}
			if err := json.Unmarshal([]byte(firstJSON), &opened); err != nil || opened.Session == 0 {
				t.Fatalf("open: %v %.200s", err, firstJSON)
			}
			id := opened.Session
			for i, sz := range sizes {
				var out string
				if i == 0 {
					out = firstJSON
				} else {
					out = m.Call("render", js(t, id), js(t, Options{Cols: sz[0], Rows: sz[1]}))
				}
				got, want := rawDump(t, out), cliDump(t, entry, data, "", sz[0], sz[1])
				if !bytes.Equal(got, want) {
					t.Fatalf("dump at %dx%d differs from the CLI's, first at %s", sz[0], sz[1], firstDiff(got, want))
				}
			}
			if themed[entry] {
				for _, th := range []string{"light", "dark"} {
					out := m.Call("render", js(t, id), js(t, Options{Cols: 100, Rows: 30, Theme: th}))
					got, want := rawDump(t, out), cliDump(t, entry, data, th, 100, 30)
					if !bytes.Equal(got, want) {
						t.Fatalf("%s dump differs from the CLI's, first at %s", th, firstDiff(got, want))
					}
				}
			}
			// validate is validate --json, with the same file and data names.
			args := []string{"validate", entry, "--json"}
			if data != "" {
				args = append(args, "--data", data)
			}
			var want, got any
			if err := json.Unmarshal(run(t, args...), &want); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(m.Call("validate", js(t, id))), &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				g, _ := json.Marshal(got)
				w, _ := json.Marshal(want)
				t.Fatalf("validate differs:\n playground: %s\n cli:        %s", g, w)
			}
		})
	}
}
