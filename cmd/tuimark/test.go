package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	iofs "io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/abdul-hamid-achik/tuimark/internal/dump"
	"github.com/abdul-hamid-achik/tuimark/internal/host"
)

// goldenEntry is one row of testdata/golden/manifest.json (SPEC v0.2 §15.5).
// Theme is *string, not string, so an explicit `"theme": ""` (a usage
// error, like `--theme ""`: finding 27) can be told apart from the field
// being absent altogether (which keeps the document's own theme).
type goldenEntry struct {
	Name   string   `json:"name"`
	File   string   `json:"file"`
	Data   string   `json:"data,omitempty"`
	Sizes  []string `json:"sizes"`
	JSON   []string `json:"json"`
	Frozen bool     `json:"frozen"`
	Theme  *string  `json:"theme,omitempty"`  // "dark" or "light", as --theme
	Input  string   `json:"input,omitempty"`  // play steps (§15.4); exclusive with Script
	Script string   `json:"script,omitempty"` // a play script path; exclusive with Input
	Styles bool     `json:"styles,omitempty"` // JSON goldens include theme and styles
	Cells  bool     `json:"cells,omitempty"`  // JSON goldens include cells (SPEC v0.2b §15.5)
}

// theme returns the entry's theme value and whether the field was given
// at all (finding 27: an explicit "" is given, not absent).
func (e goldenEntry) theme() (value string, set bool) {
	if e.Theme == nil {
		return "", false
	}
	return *e.Theme, true
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

// --- ordered JSON values, for the superset check (D7) -----------------

// ojVal is a JSON value decoded with object member order preserved
// (encoding/json's map[string]any does not), so a superset violation is
// reported at "the first offending location in document order" (SPEC v0.2
// §15.5).
type ojVal struct {
	kind   byte // 'o' object, 'a' array, 's' scalar
	obj    []ojPair
	arr    []ojVal
	scalar any
}

type ojPair struct {
	key string
	val ojVal
}

func decodeOrdered(data []byte) (ojVal, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	return decodeOrderedValue(dec)
}

func decodeOrderedValue(dec *json.Decoder) (ojVal, error) {
	tok, err := dec.Token()
	if err != nil {
		return ojVal{}, err
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return ojVal{kind: 's', scalar: tok}, nil
	}
	switch delim {
	case '{':
		var obj []ojPair
		for dec.More() {
			keyTok, err := dec.Token()
			if err != nil {
				return ojVal{}, err
			}
			val, err := decodeOrderedValue(dec)
			if err != nil {
				return ojVal{}, err
			}
			obj = append(obj, ojPair{keyTok.(string), val})
		}
		if _, err := dec.Token(); err != nil { // consume '}'
			return ojVal{}, err
		}
		return ojVal{kind: 'o', obj: obj}, nil
	case '[':
		var arr []ojVal
		for dec.More() {
			val, err := decodeOrderedValue(dec)
			if err != nil {
				return ojVal{}, err
			}
			arr = append(arr, val)
		}
		if _, err := dec.Token(); err != nil { // consume ']'
			return ojVal{}, err
		}
		return ojVal{kind: 'a', arr: arr}, nil
	}
	return ojVal{}, fmt.Errorf("unexpected JSON delimiter %v", delim)
}

func withSeg(path []string, seg string) []string {
	out := make([]string, len(path)+1)
	copy(out, path)
	out[len(path)] = seg
	return out
}

// jsonPointer renders path as an RFC 6901 JSON Pointer.
func jsonPointer(path []string) string {
	if len(path) == 0 {
		return ""
	}
	rep := strings.NewReplacer("~", "~0", "/", "~1")
	var b strings.Builder
	for _, s := range path {
		b.WriteByte('/')
		b.WriteString(rep.Replace(s))
	}
	return b.String()
}

// supersetOK reports whether newV is a superset of oldV (SPEC v0.2 §15.5):
// scalars equal (same JSON type and value); every member of an old object
// exists in the new object with a new value that is a superset of the old
// one (the new object may add members); arrays have the same length and are
// a superset element by element. It returns the JSON Pointer of the first
// offending location in document order (oldV's own member/element order).
func supersetOK(oldV, newV ojVal, path []string) (bool, string) {
	if oldV.kind != newV.kind {
		return false, jsonPointer(path)
	}
	switch oldV.kind {
	case 's':
		if oldV.scalar != newV.scalar {
			return false, jsonPointer(path)
		}
		return true, ""
	case 'o':
		nmap := make(map[string]ojVal, len(newV.obj))
		for _, p := range newV.obj {
			nmap[p.key] = p.val
		}
		for _, p := range oldV.obj {
			nv, ok := nmap[p.key]
			if !ok {
				return false, jsonPointer(withSeg(path, p.key))
			}
			if ok2, ptr := supersetOK(p.val, nv, withSeg(path, p.key)); !ok2 {
				return false, ptr
			}
		}
		return true, ""
	case 'a':
		if len(oldV.arr) != len(newV.arr) {
			return false, jsonPointer(path)
		}
		for i := range oldV.arr {
			if ok2, ptr := supersetOK(oldV.arr[i], newV.arr[i], withSeg(path, strconv.Itoa(i))); !ok2 {
				return false, ptr
			}
		}
		return true, ""
	}
	return false, jsonPointer(path)
}

// --- tuimark test -------------------------------------------------------

// cmdTest runs the golden dump comparisons driven by DIR/manifest.json
// (default testdata/golden). See README.md for the manifest format.
func (c *cli) cmdTest(args []string) int {
	fs := c.newFlags("test")
	update := fs.Bool("update", false, "write goldens for entries that are not frozen")
	allowBreaking := fs.Bool("allow-breaking", false, "with --update, skip the JSON superset check (SPEC §15.5 D7)")
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
	// Strict decoding (finding 28): a typo like "stlyes" is a manifest
	// error (exit 1), not silently ignored. §28.24: "theme, input, script,
	// styles are the only additions in 0.2a"; 0.2b adds "cells" (§15.5).
	var entries []goldenEntry
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&entries); err != nil {
		return c.fail(fmt.Errorf("%s: %w", manifestPath, err))
	}

	ioErr := false
	mismatch := false
	for _, e := range entries {
		if e.Input != "" && e.Script != "" {
			fmt.Fprintf(c.stdout, "FAIL %s: input and script are mutually exclusive\n", e.Name)
			ioErr = true
			continue
		}
		hasPlay := e.Input != "" || e.Script != ""
		var steps []playStep
		if hasPlay {
			var perr error
			if e.Input != "" {
				steps, perr = parseInputSteps(e.Input)
			} else {
				steps, perr = parseScriptSteps(e.Script)
			}
			if perr != nil {
				fmt.Fprintf(c.stdout, "FAIL %s: %v\n", e.Name, perr)
				ioErr = true
				continue
			}
		}
		freshPerSize := hasPlay || e.Styles || e.Cells
		themeVal, themeSet := e.theme()

		// A plain (non-play, non-styles) entry loads once: App.Dump is a
		// snapshot, so dumping several sizes on the same App does not
		// depend on their order (SPEC v0.2 §26.4's Dump/Validate row).
		var sharedApp *host.App
		if !freshPerSize {
			app, err := load(e.File, e.Data)
			if err != nil {
				fmt.Fprintf(c.stdout, "FAIL %s: %v\n", e.Name, err)
				ioErr = true
				continue
			}
			if err := applyTheme(app, themeVal, themeSet, fmt.Sprintf("manifest entry %q theme", e.Name)); err != nil {
				fmt.Fprintf(c.stdout, "FAIL %s: %v\n", e.Name, err)
				ioErr = true
				continue
			}
			sharedApp = app
		}

		jsonSizes := map[string]bool{}
		for _, s := range e.JSON {
			jsonSizes[s] = true
		}
		for _, size := range e.Sizes {
			cols, rows, err := parseSize(size)
			if err == nil {
				// finding 28: a manifest size must be in the same 1-1000
				// range `--cols`/`--rows` and `resize:` enforce, so a
				// document is never rendered at a size no other tool can
				// reach (0x5, a negative dimension, or over 1000).
				err = checkSize(cols, rows)
			}
			if err != nil {
				fmt.Fprintf(c.stdout, "FAIL %s %s: %v\n", e.Name, size, err)
				ioErr = true
				continue
			}

			var d *dump.Dump
			var events []dump.Event
			isPlay := false

			if freshPerSize {
				app, err := load(e.File, e.Data)
				if err != nil {
					fmt.Fprintf(c.stdout, "FAIL %s %s: %v\n", e.Name, size, err)
					ioErr = true
					continue
				}
				if err := applyTheme(app, themeVal, themeSet, fmt.Sprintf("manifest entry %q theme", e.Name)); err != nil {
					fmt.Fprintf(c.stdout, "FAIL %s %s: %v\n", e.Name, size, err)
					ioErr = true
					continue
				}
				if hasPlay {
					sess := newPlaySession(app, cols, rows, e.Cells, e.Styles, false)
					sess.run(steps)
					if sess.usageErr != nil {
						fmt.Fprintf(c.stdout, "FAIL %s %s: play step %d (%s): %v\n", e.Name, size, sess.usageStep, sess.usageRaw, sess.usageErr)
						ioErr = true
						continue
					}
					d = sess.buildDump()
					events = sess.events
					isPlay = true
				} else {
					d = frameDump(app.Frame(cols, rows), e.Cells, e.Styles)
				}
			} else {
				d = sharedApp.Dump(cols, rows, false)
			}

			var gotText string
			if isPlay {
				var tb strings.Builder
				tb.WriteString(dump.Text(d))
				writeEventsSection(&tb, events)
				gotText = tb.String()
			} else {
				gotText = dump.Text(d)
			}

			doUpdate := *update && !e.Frozen
			textPath := filepath.Join(dir, e.Name, size+".txt")
			jsonPath := filepath.Join(dir, e.Name, size+".json")

			if !jsonSizes[size] {
				c.reportGolden(e.Name, size, "text", textPath, gotText, doUpdate, &mismatch, &ioErr)
				continue
			}

			var gotJSON []byte
			if isPlay {
				play := &dump.Play{Dump: d, Events: events}
				b, err := json.MarshalIndent(play, "", "  ")
				if err != nil {
					fmt.Fprintf(c.stdout, "FAIL %s %s (json): %v\n", e.Name, size, err)
					ioErr = true
					continue
				}
				gotJSON = append(b, '\n')
			} else {
				b, err := dump.JSON(d)
				if err != nil {
					fmt.Fprintf(c.stdout, "FAIL %s %s (json): %v\n", e.Name, size, err)
					ioErr = true
					continue
				}
				gotJSON = b
			}

			// The superset check (SPEC v0.2 §15.5 D7). --allow-breaking skips
			// it entirely ("it has no effect without --update"). Without
			// it, an existing golden that cannot be read or is not valid
			// JSON must refuse the write (finding 25), not be silently
			// regenerated: only "does not exist yet" is exempt.
			if doUpdate && !*allowBreaking {
				old, err := os.ReadFile(jsonPath)
				switch {
				case errors.Is(err, iofs.ErrNotExist):
					// No existing golden: "simply written" (§15.5).
				case err != nil:
					fmt.Fprintf(c.stdout, "FAIL %s %s (json): cannot read the existing golden for the superset check: %v\n", e.Name, size, err)
					ioErr = true
					continue
				default:
					oldV, err1 := decodeOrdered(old)
					if err1 != nil {
						fmt.Fprintf(c.stdout, "FAIL %s %s (json): existing golden is not valid JSON, refusing to overwrite it: %v\n", e.Name, size, err1)
						ioErr = true
						continue
					}
					newV, err2 := decodeOrdered(gotJSON)
					if err2 != nil {
						fmt.Fprintf(c.stdout, "FAIL %s %s (json): internal error re-parsing the new dump: %v\n", e.Name, size, err2)
						ioErr = true
						continue
					}
					if ok, ptr := supersetOK(oldV, newV, nil); !ok {
						fmt.Fprintf(c.stdout, "FAIL %s %s (json): update removes or changes %s\n", e.Name, size, ptr)
						mismatch = true
						continue
					}
				}
			}

			c.reportGolden(e.Name, size, "json", jsonPath, string(gotJSON), doUpdate, &mismatch, &ioErr)
			c.reportGolden(e.Name, size, "text", textPath, gotText, doUpdate, &mismatch, &ioErr)
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

// reportGolden prints exactly the line grammar SPEC v0.2 §15.5 specifies:
// "PASS NAME SIZE (text|json)" or "FAIL NAME SIZE (text|json): HINT".
// Before the fix, a successful --update printed a third, undocumented
// "PASS NAME SIZE (text|json, updated)" form (finding 26), which a tool
// matching the literal §15.5 pattern would miss.
func (c *cli) reportGolden(name, size, kind, path, got string, doUpdate bool, mismatch, ioErr *bool) {
	ok, hint, err := compareOrUpdate(path, got, doUpdate)
	switch {
	case err != nil:
		fmt.Fprintf(c.stdout, "FAIL %s %s (%s): %v\n", name, size, kind, err)
		*ioErr = true
	case ok:
		fmt.Fprintf(c.stdout, "PASS %s %s (%s)\n", name, size, kind)
	default:
		fmt.Fprintf(c.stdout, "FAIL %s %s (%s): %s\n", name, size, kind, hint)
		*mismatch = true
	}
}
