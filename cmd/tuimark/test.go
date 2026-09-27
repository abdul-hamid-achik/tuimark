package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/abdul-hamid-achik/tuimark/internal/dump"
)

// goldenEntry is one row of testdata/golden/manifest.json.
type goldenEntry struct {
	Name   string   `json:"name"`
	File   string   `json:"file"`
	Data   string   `json:"data,omitempty"`
	Sizes  []string `json:"sizes"`
	JSON   []string `json:"json"`
	Frozen bool     `json:"frozen"`
}

func parseSize(s string) (cols, rows int, err error) {
	parts := strings.SplitN(s, "x", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("bad size %q (want COLSxROWS)", s)
	}
	cols, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("bad size %q: %v", s, err)
	}
	rows, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("bad size %q: %v", s, err)
	}
	return cols, rows, nil
}

// compareOrUpdate compares got against the golden file at path, or (when
// update is true) writes it. ok is true on a match or a successful write;
// hint is a short diff for a mismatch.
func compareOrUpdate(path, got string, update bool) (ok bool, hint string, err error) {
	if update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return false, "", err
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			return false, "", err
		}
		return true, "", nil
	}
	want, err := os.ReadFile(path)
	if err != nil {
		return false, "", err
	}
	if string(want) == got {
		return true, "", nil
	}
	return false, diffHint(string(want), got), nil
}

func diffHint(want, got string) string {
	wl := strings.Split(want, "\n")
	gl := strings.Split(got, "\n")
	n := len(wl)
	if len(gl) < n {
		n = len(gl)
	}
	for i := 0; i < n; i++ {
		if wl[i] != gl[i] {
			return fmt.Sprintf("line %d: want %q, got %q", i+1, wl[i], gl[i])
		}
	}
	if len(wl) != len(gl) {
		return fmt.Sprintf("line count: want %d, got %d", len(wl), len(gl))
	}
	return "differs"
}

// cmdTest runs the golden dump comparisons driven by DIR/manifest.json
// (default testdata/golden). See README.md for the manifest format.
func (c *cli) cmdTest(args []string) int {
	fs := c.newFlags("test")
	update := fs.Bool("update", false, "write goldens for entries that are not frozen")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return 1
	}
	dir := "testdata/golden"
	switch len(pos) {
	case 0:
	case 1:
		dir = pos[0]
	default:
		return c.fail(fmt.Errorf("tuimark test: want at most one DIR, got %d", len(pos)))
	}
	manifestPath := filepath.Join(dir, "manifest.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return c.fail(err)
	}
	var entries []goldenEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return c.fail(fmt.Errorf("%s: %w", manifestPath, err))
	}

	ioErr := false
	mismatch := false
	for _, e := range entries {
		app, err := load(e.File, e.Data)
		if err != nil {
			fmt.Fprintf(c.stdout, "FAIL %s: %v\n", e.Name, err)
			ioErr = true
			continue
		}
		jsonSizes := map[string]bool{}
		for _, s := range e.JSON {
			jsonSizes[s] = true
		}
		for _, size := range e.Sizes {
			cols, rows, err := parseSize(size)
			if err != nil {
				fmt.Fprintf(c.stdout, "FAIL %s %s: %v\n", e.Name, size, err)
				ioErr = true
				continue
			}
			d := app.Dump(cols, rows, false)
			doUpdate := *update && !e.Frozen

			textPath := filepath.Join(dir, e.Name, size+".txt")
			c.reportGolden(e.Name, size, "text", textPath, dump.Text(d), doUpdate, &mismatch, &ioErr)

			if jsonSizes[size] {
				gotJSON, err := dump.JSON(d)
				if err != nil {
					fmt.Fprintf(c.stdout, "FAIL %s %s (json): %v\n", e.Name, size, err)
					ioErr = true
					continue
				}
				jsonPath := filepath.Join(dir, e.Name, size+".json")
				c.reportGolden(e.Name, size, "json", jsonPath, string(gotJSON), doUpdate, &mismatch, &ioErr)
			}
		}
	}
	if ioErr {
		return 1
	}
	if mismatch {
		return 2
	}
	return 0
}

func (c *cli) reportGolden(name, size, kind, path, got string, doUpdate bool, mismatch, ioErr *bool) {
	ok, hint, err := compareOrUpdate(path, got, doUpdate)
	switch {
	case err != nil:
		fmt.Fprintf(c.stdout, "FAIL %s %s (%s): %v\n", name, size, kind, err)
		*ioErr = true
	case ok && doUpdate:
		fmt.Fprintf(c.stdout, "PASS %s %s (%s, updated)\n", name, size, kind)
	case ok:
		fmt.Fprintf(c.stdout, "PASS %s %s (%s)\n", name, size, kind)
	default:
		fmt.Fprintf(c.stdout, "FAIL %s %s (%s): %s\n", name, size, kind, hint)
		*mismatch = true
	}
}
